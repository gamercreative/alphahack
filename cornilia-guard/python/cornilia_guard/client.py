"""Signed HTTP client from Cornilia to the Go agent gateway (stdlib only)."""
from __future__ import annotations

import hashlib
import hmac
import json
import re
import secrets
import time
import urllib.error
import urllib.request
from typing import Any

PREFIX = "/agent/v1/ops/"
_SIG_DOMAIN = b"cornilia-req-v1"
_OP_RE = re.compile(r"^[a-z_]+(\.[a-z_]+)*$")


class GatewayError(Exception):
    """An error the gateway returned. `message` is safe to show the model."""

    def __init__(self, status: int, code: str, message: str):
        super().__init__(f"{status} {code}: {message}")
        self.status, self.code, self.message = status, code, message


def canonical_string(method: str, path: str, ts: str, nonce: str, body: bytes) -> str:
    return "\n".join([method, path, ts, nonce, hashlib.sha256(body).hexdigest()])


def sign_request(service_key: bytes, method: str, path: str, ts: str, nonce: str, body: bytes) -> str:
    # Same layout as Go's mac(): domain, NUL byte, message.
    msg = _SIG_DOMAIN + b"\x00" + canonical_string(method, path, ts, nonce, body).encode()
    return hmac.new(service_key, msg, hashlib.sha256).hexdigest()


class GatewayClient:
    """One client per agent session. `agent_token` is the opaque delegation token
    Go minted for this run; Cornilia never sees the key that signs it."""

    def __init__(self, base_url: str, service_key: bytes, agent_token: str,
                 timeout: float = 10.0, use_env_proxy: bool = False):
        if len(service_key) < 32:
            raise ValueError("service key must be at least 32 bytes")
        self.base_url = base_url.rstrip("/")
        self._key = service_key
        self._token = agent_token
        self.timeout = timeout
        handlers = [] if use_env_proxy else [urllib.request.ProxyHandler({})]
        self._opener = urllib.request.build_opener(*handlers)

    def call(self, op: str, args: dict[str, Any], idempotency_key: str | None = None) -> Any:
        if not _OP_RE.match(op):
            raise ValueError(f"bad op name {op!r}")
        path = PREFIX + op
        body = json.dumps(args, separators=(",", ":"), ensure_ascii=False).encode()
        ts, nonce = str(int(time.time())), secrets.token_urlsafe(18)
        headers = {
            "Content-Type": "application/json",
            "Authorization": f"Agent {self._token}",
            "X-Cornilia-Timestamp": ts,
            "X-Cornilia-Nonce": nonce,
            "X-Cornilia-Signature": sign_request(self._key, "POST", path, ts, nonce, body),
        }
        if idempotency_key:
            headers["Idempotency-Key"] = idempotency_key
        req = urllib.request.Request(self.base_url + path, data=body, method="POST", headers=headers)
        try:
            with self._opener.open(req, timeout=self.timeout) as resp:
                payload = json.loads(resp.read())
        except urllib.error.HTTPError as e:
            try:
                err = json.loads(e.read()).get("error") or {}
            except Exception:
                err = {}
            raise GatewayError(e.code, err.get("code", "http_error"), err.get("message", "request failed")) from None
        except (urllib.error.URLError, TimeoutError) as e:
            raise GatewayError(0, "unreachable", "backend unreachable") from e
        if not payload.get("ok"):
            raise GatewayError(200, "bad_response", "backend returned an error")
        return payload.get("data")
