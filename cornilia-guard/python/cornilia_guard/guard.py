"""ToolGuard: every model tool call in Cornilia goes through execute().

Order of checks (cheap and local first, the network last):
  1. halted / budget        -> stops runaway loops
  2. known tool             -> the model can't invent tools
  3. parse + strict schema  -> no extra fields, right types
  4. repeat detection       -> same call again and again = loop
  5. approval gate          -> writes after untrusted data need a human yes
  6. signed gateway call    -> Go re-checks EVERYTHING (scope, budget, args)
  7. wrap output            -> marks result untrusted, flags injection text
"""
from __future__ import annotations

import hashlib
import json
import logging
from dataclasses import dataclass, field
from typing import Any, Callable, Literal, Protocol

from pydantic import ValidationError

from .client import GatewayError
from .policy import Risk, StrictArgs, ToolSpec
from .sanitize import wrap_untrusted

log = logging.getLogger("cornilia.guard")

MAX_RAW_ARGS = 16_000  # chars of JSON the model may send for one call


class Client(Protocol):
    def call(self, op: str, args: dict[str, Any], idempotency_key: str | None = None) -> Any: ...


# approver(tool, validated_args, reason) -> True to allow
Approver = Callable[[ToolSpec, StrictArgs, str], bool]


def deny_all(tool: ToolSpec, args: StrictArgs, reason: str) -> bool:
    return False


def ask_in_terminal(tool: ToolSpec, args: StrictArgs, reason: str) -> bool:
    print(f"\n[approval needed] {tool.name} {args.model_dump_json()}\n  reason: {reason}")
    return input("  allow? [y/N] ").strip().lower() == "y"


@dataclass
class GuardConfig:
    max_calls: int = 25                 # per agent run (Go has its own budget too)
    max_identical_calls: int = 2        # same tool + same args
    max_consecutive_errors: int = 3     # then halt the run
    max_output_chars: int = 6000
    approve_writes: Literal["always", "when_tainted"] = "always"


@dataclass
class ToolResult:
    content: str          # what goes back to the model
    ok: bool
    blocked: bool = False
    flagged: bool = False  # output looked like a prompt injection


@dataclass
class ToolGuard:
    tools: list[ToolSpec]
    client: Client
    approver: Approver = deny_all
    config: GuardConfig = field(default_factory=GuardConfig)

    def __post_init__(self):
        self._by_name = {t.name: t for t in self.tools}
        if len(self._by_name) != len(self.tools):
            raise ValueError("duplicate tool names")
        self.calls = 0
        self.consecutive_errors = 0
        self.tainted = False   # becomes True once untrusted data entered the context
        self.halted = False
        self._seen: dict[str, int] = {}
        self.events: list[dict[str, Any]] = []  # local audit trail

    # ---- for the harness ----
    def schemas(self) -> list[dict[str, Any]]:
        return [t.schema() for t in self.tools]

    @property
    def should_stop(self) -> bool:
        return self.halted

    def execute(self, name: str, raw_args: str | dict[str, Any] | None) -> ToolResult:
        """Never raises. Always returns something safe to hand to the model."""
        try:
            res = self._execute(name, raw_args)
        except _Blocked as b:
            res = ToolResult(f"[blocked] {b}", ok=False, blocked=True)
        except Exception:  # bug in a tool spec etc. Don't leak internals to the model.
            log.exception("guard internal error")
            self._error()
            res = ToolResult("[error] internal error in tool layer", ok=False)
        self.events.append({"tool": name, "ok": res.ok, "blocked": res.blocked,
                            "flagged": res.flagged, "call_no": self.calls})
        return res

    # ---- internals ----
    def _execute(self, name: str, raw_args: str | dict[str, Any] | None) -> ToolResult:
        if self.halted:
            raise _Blocked("this run was halted after repeated failures; stop and report to the user")
        if self.calls >= self.config.max_calls:
            self.halted = True
            raise _Blocked("tool call budget for this run is used up; stop and answer with what you have")

        spec = self._by_name.get(name)
        if spec is None:
            self._error()
            raise _Blocked(f"unknown tool {name!r}. Available: {', '.join(sorted(self._by_name))}")

        args_dict = self._parse(raw_args)
        try:
            args = spec.args_model.model_validate(args_dict)
        except ValidationError as e:
            self._error()
            msgs = "; ".join(f"{'.'.join(map(str, x['loc'])) or 'args'}: {x['msg']}" for x in e.errors()[:5])
            return ToolResult(f"[invalid arguments] {msgs}", ok=False)

        payload = args.model_dump(mode="json", exclude_none=True)
        fp = hashlib.sha256(f"{spec.name}|{json.dumps(payload, sort_keys=True)}".encode()).hexdigest()
        self._seen[fp] = self._seen.get(fp, 0) + 1
        if self._seen[fp] > self.config.max_identical_calls:
            self._error()
            raise _Blocked("you already made this exact call; use the earlier result instead of repeating it")

        reason = self._approval_reason(spec)
        if reason and not self.approver(spec, args, reason):
            raise _Blocked(f"{spec.name} needs user approval ({reason}) and it was not given. "
                           "Ask the user directly instead of retrying.")

        # Same logical action -> same key, so a retry or a loop can't double-book.
        idem = fp[:40] if spec.risk >= Risk.WRITE else None
        self.calls += 1
        try:
            data = self.client.call(spec.op, payload, idem)
        except GatewayError as e:
            self._error()
            return ToolResult(f"[tool error] {e.code}: {e.message}", ok=False)

        self.consecutive_errors = 0
        self.tainted = True
        text, flagged = wrap_untrusted(spec.name, data, self.config.max_output_chars)
        if flagged:
            log.warning("possible prompt injection in output of %s", spec.name)
        return ToolResult(text, ok=True, flagged=flagged)

    def _parse(self, raw: str | dict[str, Any] | None) -> dict[str, Any]:
        if raw is None or raw == "":
            return {}
        if isinstance(raw, dict):
            return raw
        if len(raw) > MAX_RAW_ARGS:
            self._error()
            raise _Blocked("arguments too large")
        try:
            val = json.loads(raw)
        except json.JSONDecodeError:
            self._error()
            raise _Blocked("arguments must be a JSON object") from None
        if not isinstance(val, dict):
            self._error()
            raise _Blocked("arguments must be a JSON object")
        return val

    def _approval_reason(self, spec: ToolSpec) -> str | None:
        if spec.risk == Risk.DESTRUCTIVE:
            return "irreversible action"
        if spec.risk == Risk.WRITE:
            if self.config.approve_writes == "always":
                return "changes data"
            if self.tainted:
                return "changes data after reading untrusted content"
        return None

    def _error(self) -> None:
        self.consecutive_errors += 1
        if self.consecutive_errors >= self.config.max_consecutive_errors:
            self.halted = True


class _Blocked(Exception):
    pass
