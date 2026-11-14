"""The only place in the agent that talks HTTP to the backend.

When the cornilia-guard agent gateway replaces the AGENT_TOKEN header scheme, only this file changes.
Tools call `_http.call(...)` (not the function imported by name) so tests can monkeypatch it.
"""

import json
import os
import urllib.error
import urllib.parse
import urllib.request

TIMEOUT_SECONDS = 15


def _base_url() -> str:
    return os.environ.get("BACKEND_URL", "http://localhost:8080").rstrip("/")


def _token() -> str:
    return os.environ.get("AGENT_TOKEN", "dev-agent-token")


def _parse(raw: bytes) -> dict:
    if not raw:
        return {}
    try:
        value = json.loads(raw)
    except ValueError:
        return {"error": {"code": "bad_response", "message": "The backend returned a body that is not JSON."}}
    return value if isinstance(value, dict) else {"data": value}


def call(method: str, path: str, body: dict | None = None, query: dict | None = None) -> dict:
    """Calls the backend. Returns the parsed JSON body, or {"error": {"code", "message"}} on failure."""
    url = _base_url() + path
    if query:
        clean = {key: value for key, value in query.items() if value is not None}
        if clean:
            url += "?" + urllib.parse.urlencode(clean)
    data = json.dumps(body).encode("utf-8") if body is not None else None
    headers = {"Accept": "application/json", "Authorization": f"Agent {_token()}"}
    if data is not None:
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT_SECONDS) as resp:
            return _parse(resp.read())
    except urllib.error.HTTPError as e:
        raw = b""
        try:
            raw = e.read()
        except Exception:
            pass
        try:
            parsed = json.loads(raw) if raw else {}
        except ValueError:
            parsed = {}
        if isinstance(parsed, dict) and isinstance(parsed.get("error"), dict):
            return parsed
        text = raw.decode("utf-8", "replace")[:300] or str(e.reason or "")
        return {"error": {"code": f"http_{e.code}", "message": text}}
    except urllib.error.URLError as e:
        return {"error": {"code": "backend_unreachable", "message": f"Could not reach the backend: {e.reason}"}}


def tool_error(out: dict) -> dict | None:
    """Turns a backend error body into the small dict the model sees, or None when the call succeeded."""
    err = out.get("error")
    if err is None:
        return None
    if isinstance(err, dict):
        return {"error": str(err.get("message", "")), "code": str(err.get("code", ""))}
    return {"error": str(err), "code": ""}
