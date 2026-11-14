"""Wassit runner: polls the backend for wakeups and runs one loop per enquiry.

Usage:
  python runner.py                       loop forever, poll every 3s, model policy
  python runner.py --once                process the current wakeups, then exit
  python runner.py --policy scripted     no model: the deterministic policy in scripted.py

The policy comes from --policy, else env WASSIT_POLICY, else model.
"""

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

AGENT_DIR = Path(__file__).resolve().parent
TOOLS_DIR = AGENT_DIR / "tools"
PROMPT_PATH = AGENT_DIR / "prompts" / "system.md"
POLL_SECONDS = 3
DEFAULT_MODEL = "qwen3.5:35b-a3b"
DEFAULT_MAX_STEPS = 12
MAX_STEPS_MESSAGE = "max steps reached; left open for a human"
POLICIES = ("model", "scripted")
_policy_override: str | None = None  # set from --policy in main()

# Cornilia reads CORNILIA_TOOLS_DIR when it is imported, so set the default first
os.environ.setdefault("CORNILIA_TOOLS_DIR", str(TOOLS_DIR))

sys.path.append(str(AGENT_DIR))  # usage.py
sys.path.append(str(TOOLS_DIR))  # enquiry.py and _http.py

import scripted  # noqa: E402
import usage  # noqa: E402
from cornilia import settings  # noqa: E402
from cornilia.context_managers import CacheModelContextManager  # noqa: E402
from cornilia.errors import MaxStepsError  # noqa: E402
from cornilia.main import CreateProvider, CreateTools  # noqa: E402
from cornilia.models import OpenAIModel  # noqa: E402


def http_call(method: str, path: str, body: dict | None = None) -> dict:
    """Call the backend with the agent header. Raises on HTTP errors."""
    base = os.environ.get("BACKEND_URL", "http://localhost:8080").rstrip("/")
    token = os.environ.get("AGENT_TOKEN", "dev-agent-token")
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(
        base + path,
        data=data,
        method=method,
        headers={"Authorization": f"Agent {token}", "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            raw = r.read()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"backend {method} {path} returned {e.code}") from e


def fetch_wakeups() -> list[dict]:
    return http_call("GET", "/api/v1/agent/wakeups").get("data", [])


def current_policy() -> str:
    """--policy wins, then env WASSIT_POLICY, then model."""
    choice = _policy_override or os.environ.get("WASSIT_POLICY", "").strip().lower() or "model"
    return choice if choice in POLICIES else "model"


def model_name_from_env() -> str:
    return os.environ.get("CORNILIA_MODEL", "").strip() or DEFAULT_MODEL


def max_steps_from_env() -> int:
    return int(os.environ.get("CORNILIA_MAX_STEPS", "").strip() or DEFAULT_MAX_STEPS)


def load_enquiry_state(enquiry_id: str) -> str:
    """Compact enquiry state as JSON text. Uses tools/enquiry.py get_enquiry, else the raw trimmed JSON."""
    try:
        from enquiry import get_enquiry  # agent/tools/enquiry.py
    except Exception:
        return _raw_enquiry_state(enquiry_id)
    try:
        fn = getattr(get_enquiry, "ToolFunction", None) or get_enquiry
        result = fn(enquiry_id=enquiry_id)
        if isinstance(result, str):
            return result
        return json.dumps(result, ensure_ascii=False, separators=(",", ":"))
    except Exception:
        return _raw_enquiry_state(enquiry_id)


def _raw_enquiry_state(enquiry_id: str) -> str:
    raw = http_call("GET", f"/api/v1/enquiries/{enquiry_id}")
    return json.dumps(raw, ensure_ascii=False)[:3000]


def build_prompt(trigger: str, state_json: str) -> str:
    return (
        f"Trigger: {trigger}\n"
        f"Enquiry state (JSON):\n{state_json}\n"
        "Decide the next step. End this run by calling wait() or close_enquiry() "
        "(or ask_requester when the request is ambiguous)."
    )


def build_model(provider, tools, model_name, tracer):
    """A fresh model per run, built the same way as cornilia.main.CreateModel."""
    system_prompt = PROMPT_PATH.read_text(encoding="utf-8")
    return OpenAIModel(
        model_name,
        provider=provider,
        context_manager=CacheModelContextManager(system_prompt),  # renders $date and $time
        tools=tools,
        response_configs=settings.MODEL_RESPONSE_CONFIG,
        max_steps=max_steps_from_env(),
        max_retries=settings.MAX_RETRIES,
        retry_max_wait=settings.RETRY_MAX_WAIT,
        tool_tracer=tracer,
    )


def make_tracer(run_id: str):
    def tracer(name, arguments, result, seconds):
        args = arguments
        if isinstance(args, str):
            try:
                args = json.loads(args)
            except ValueError:
                args = {"raw": args}
        if args is None:
            args = {}
        if not isinstance(args, dict):
            args = {"value": args}
        text = result if isinstance(result, str) else json.dumps(result, ensure_ascii=False, default=str)
        is_error = '"error"' in text
        duration_ms = int(seconds * 1000)
        try:
            http_call(
                "POST",
                f"/api/v1/agent/runs/{run_id}/tool-calls",
                {
                    "name": name,
                    "input": args,
                    "output": text[:4000],
                    "is_error": is_error,
                    "duration_ms": duration_ms,
                },
            )
        except Exception as e:
            print(f"[run {run_id}] could not record tool call {name}: {e}")
        flag = " ERROR" if is_error else ""
        print(f"[run {run_id}] tool {name} {duration_ms}ms{flag}")

    return tracer


def finish_run(run_id: str, status: str, error: str, input_tokens: int = 0,
               output_tokens: int = 0, steps: int = 0) -> None:
    body = {
        "status": status,
        "input_tokens": input_tokens,
        "output_tokens": output_tokens,
        "steps": steps,
        "error": error,
    }
    try:
        http_call("PATCH", f"/api/v1/agent/runs/{run_id}", body)
    except Exception as e:
        print(f"[run {run_id}] could not finish run: {e}")


def run_scripted(run_id: str, enquiry_id: str) -> tuple[str, str, int]:
    """The deterministic policy: no model, tokens 0, steps = tool calls. Returns (status, error, steps)."""
    status, error = "completed", ""
    steps = 0
    tracer = make_tracer(run_id)

    def call(name: str, **kwargs) -> dict:
        nonlocal steps
        steps += 1
        started = time.perf_counter()
        try:
            result = scripted.run_tool(name, **kwargs)
        except Exception as e:
            result = {"error": str(e)[:300]}
        tracer(name, kwargs, result, time.perf_counter() - started)
        return result

    try:
        state = http_call("GET", f"/api/v1/enquiries/{enquiry_id}")
        scripted.decide(state, call)
    except Exception as e:
        status, error = "failed", str(e)[:500]
    return status, error, steps


def process_wake(wake: dict) -> str:
    """Run one loop for one wakeup, with the model or the scripted policy. Returns the run status."""
    enquiry_id = str(wake.get("enquiry_id", ""))
    trigger = str(wake.get("trigger", ""))
    policy = current_policy()
    model_name = model_name_from_env()

    run = http_call(
        "POST",
        "/api/v1/agent/runs",
        {"enquiry_id": enquiry_id, "trigger": trigger, "model": model_name},
    )
    run_id = str(run["id"])
    print(f"[run {run_id}] enquiry {enquiry_id} trigger {trigger} policy {policy}")

    if policy == "scripted":
        status, error, steps = run_scripted(run_id, enquiry_id)
        finish_run(run_id, status, error, steps=steps)
        print(f"[run {run_id}] {status}")
        return status

    status, error = "completed", ""
    meter = None
    try:
        state_json = load_enquiry_state(enquiry_id)
        provider = CreateProvider()
        meter = usage.attach(provider)
        tools = CreateTools()
        model = build_model(provider, tools, model_name, make_tracer(run_id))
        model.MultiShotMessage(build_prompt(trigger, state_json))
    except MaxStepsError:
        status, error = "failed", MAX_STEPS_MESSAGE
    except Exception as e:
        status, error = "failed", str(e)[:500]
    finally:
        finish_run(
            run_id,
            status,
            error,
            input_tokens=meter.input_tokens if meter else 0,
            output_tokens=meter.output_tokens if meter else 0,
            steps=meter.calls if meter else 0,
        )

    print(f"[run {run_id}] {status}")
    return status


def process_wakeups() -> None:
    for wake in fetch_wakeups():
        try:
            process_wake(wake)
        except Exception as e:
            print(f"wake for enquiry {wake.get('enquiry_id')} failed: {e}")


def main(argv: list[str]) -> int:
    global _policy_override
    parser = argparse.ArgumentParser(description="Wassit runner")
    parser.add_argument("--once", action="store_true", help="process the current wakeups, then exit")
    parser.add_argument("--policy", choices=POLICIES, default=None, help="model (default) or scripted")
    args = parser.parse_args(argv)
    env_policy = os.environ.get("WASSIT_POLICY", "").strip().lower()
    if env_policy and env_policy not in POLICIES:
        print(f"WASSIT_POLICY must be one of {', '.join(POLICIES)}, got {env_policy!r}")
        return 2
    _policy_override = args.policy

    if args.once:
        try:
            process_wakeups()
        except Exception as e:
            print(f"could not read wakeups: {e}")
            return 1
        return 0

    print(f"runner polling every {POLL_SECONDS}s, policy {current_policy()}")
    while True:
        try:
            process_wakeups()
        except KeyboardInterrupt:
            raise
        except Exception as e:
            print(f"poll failed: {e}")
        time.sleep(POLL_SECONDS)


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except KeyboardInterrupt:
        raise SystemExit(0)
