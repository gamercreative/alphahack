import io
import json
import urllib.error
import urllib.request

import pytest

import _http
import catalogue
import enquiry

LIMIT = 2000  # CONTRACT "Agent tools": every tool result is under 2,000 characters


class KnownBugFailure(AssertionError):
    """Raised only by the pinned bug assertion in an xfail test so setup failures fail loudly."""


def assert_known_bug(condition, message=""):
    if not condition:
        raise KnownBugFailure(message)


class FakeCall:
    """Replaces _http.call. Records every request and returns canned answers in order."""

    def __init__(self, *responses):
        self.responses = list(responses)
        self.calls = []

    def __call__(self, method, path, body=None, query=None):
        self.calls.append((method, path, body, query))
        if not self.responses:
            return {}
        return self.responses.pop(0)


@pytest.fixture
def fake(monkeypatch):
    def install(*responses):
        f = FakeCall(*responses)
        monkeypatch.setattr(_http, "call", f)
        return f
    return install


class Resp:
    def __init__(self, body=b"{}"):
        self._body = body

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False

    def read(self):
        return self._body


def capture_urlopen(monkeypatch, body=b"{}"):
    """Replaces urllib.request.urlopen; records the Request it was given."""
    seen = {}

    def fake_open(req, timeout=None):
        seen["req"] = req
        seen["timeout"] = timeout
        return Resp(body)
    monkeypatch.setattr(urllib.request, "urlopen", fake_open)
    return seen


def raising_urlopen(monkeypatch, exc):
    def boom(req, timeout=None):
        raise exc
    monkeypatch.setattr(urllib.request, "urlopen", boom)


# ---------- Part A: _http.call against a fake urlopen ----------

def test_call_sends_json_body_and_content_type(monkeypatch):
    monkeypatch.setenv("BACKEND_URL", "http://backend.test")
    seen = capture_urlopen(monkeypatch)
    _http.call("POST", "/api/v1/enquiries/E1/medication", body={"a": 1})
    req = seen["req"]
    assert req.data == b'{"a": 1}'
    assert json.loads(req.data) == {"a": 1}
    assert req.get_header("Content-type") == "application/json"
    assert req.get_method() == "POST"
    assert req.full_url == "http://backend.test" + "/api/v1/enquiries/E1/medication"


def test_call_get_has_no_body_and_no_content_type(monkeypatch):
    monkeypatch.setenv("BACKEND_URL", "http://backend.test")
    seen = capture_urlopen(monkeypatch)
    _http.call("GET", "/api/v1/medications")
    req = seen["req"]
    assert req.data is None
    assert req.has_header("Content-type") is False
    assert req.get_method() == "GET"
    assert req.full_url == "http://backend.test/api/v1/medications"


def test_call_reads_agent_token_from_env(monkeypatch):
    monkeypatch.setenv("AGENT_TOKEN", "tok-xyz")
    seen = capture_urlopen(monkeypatch)
    _http.call("GET", "/api/v1/medications")
    assert seen["req"].get_header("Authorization") == "Agent tok-xyz"


def test_call_reads_backend_url_from_env(monkeypatch):
    monkeypatch.setenv("BACKEND_URL", "http://example.test:9")
    seen = capture_urlopen(monkeypatch)
    _http.call("GET", "/api/v1/medications")
    assert seen["req"].full_url.startswith("http://example.test:9")
    assert seen["req"].full_url == "http://example.test:9/api/v1/medications"


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason='BUG-10: _http.call lets a read timeout escape as an exception instead of returning an error dict (CONTRACT: errors teach)')
def test_call_read_timeout_returns_error_dict(monkeypatch):
    raising_urlopen(monkeypatch, TimeoutError("timed out"))
    try:
        out = _http.call("GET", "/api/v1/medications")
    except TimeoutError as exc:
        raise KnownBugFailure(f"_http.call let TimeoutError escape: {exc}") from exc
    assert_known_bug(isinstance(out, dict) and "error" in out)


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason='BUG-10: _http.call lets a connection reset escape as an exception instead of returning an error dict')
def test_call_connection_reset_returns_error_dict(monkeypatch):
    raising_urlopen(monkeypatch, ConnectionResetError("connection reset by peer"))
    try:
        out = _http.call("GET", "/api/v1/medications")
    except ConnectionResetError as exc:
        raise KnownBugFailure(f"_http.call let ConnectionResetError escape: {exc}") from exc
    assert_known_bug(isinstance(out, dict) and "error" in out)


def test_call_http_500_plain_text(monkeypatch):
    def boom(req, timeout=None):
        raise urllib.error.HTTPError(req.full_url, 500, "Internal Server Error", {}, io.BytesIO(b"boom"))
    monkeypatch.setattr(urllib.request, "urlopen", boom)
    out = _http.call("GET", "/api/v1/medications")
    assert out["error"]["code"] == "http_500"
    assert out == {"error": {"code": "http_500", "message": "boom"}}


def test_call_http_409_envelope_is_returned_as_is(monkeypatch):
    envelope = {"error": {"code": "conflict", "message": "no"}}

    def boom(req, timeout=None):
        raise urllib.error.HTTPError(req.full_url, 409, "Conflict", {}, io.BytesIO(json.dumps(envelope).encode()))
    monkeypatch.setattr(urllib.request, "urlopen", boom)
    out = _http.call("POST", "/api/v1/enquiries/E1/close", body={"outcome": "available"})
    assert out == envelope


# ---------- Part B: tool result size under 2,000 characters ----------

@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason='BUG-12: search_medications does not cap its result (CONTRACT: tool results < 2k chars)')
def test_search_medications_result_under_2k(fake):
    long = "x" * 300
    rows = [{"id": f"{long}{i}", "brand_name": long, "active_ingredient": long,
             "strength": long, "dosage_form": long} for i in range(20)]
    fake({"data": rows})
    out = catalogue.search_medications("augmentin")
    size = len(json.dumps(out))
    assert_known_bug(size < LIMIT, f"search_medications result is {size} characters")


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason='BUG-12: find_candidates does not cap its result (CONTRACT: tool results < 2k chars)')
def test_find_candidates_result_under_2k(fake):
    long = "n" * 300
    rows = [{"branch_id": f"B{i}", "name": f"{long}{i}", "distance_km": 1.5,
             "has_reported_stock": True, "reported_quantity": 4} for i in range(10)]
    fake({"data": rows})
    out = enquiry.find_candidates("E1", limit=10)
    size = len(json.dumps(out))
    assert_known_bug(size < LIMIT, f"find_candidates result is {size} characters")


def test_propose_targets_result_under_2k(fake):
    rows = [{"target_id": f"T{i}", "branch_id": f"B{i}", "status": "proposed"} for i in range(10)]
    fake({"data": rows})
    out = enquiry.propose_targets("E1", [f"B{i}" for i in range(10)])
    size = len(json.dumps(out))
    assert size < LIMIT, f"propose_targets result is {size} characters"


def test_get_enquiry_without_targets_under_2k(fake):
    fake({
        "id": "E1",
        "status": "awaiting_clarification",
        "raw_request": "r" * 1000,
        "clarification_question": "q" * 1000,
        "clarification_answer": "a" * 1000,
        "targets": [],
        "messages": [],
    })
    out = enquiry.get_enquiry("E1")
    size = len(json.dumps(out))
    assert size < LIMIT, f"get_enquiry (no targets) result is {size} characters"


def test_get_enquiry_with_ten_targets_under_2k(fake):
    targets = [{"id": f"T{i}", "branch_id": f"B{i}", "branch_name": f"Pharmacy {i}",
                "status": "unavailable", "reply_quantity": 0, "reply_text": "z" * 1000,
                "follow_ups": 0} for i in range(10)]
    fake({
        "id": "E1",
        "status": "sent",
        "raw_request": "r" * 1000,
        "medication": {"brand_name": "Augmentin", "strength": "1g", "dosage_form": "tablet"},
        "targets": targets,
        "messages": [],
    })
    out = enquiry.get_enquiry("E1")
    size = len(json.dumps(out))
    assert size < LIMIT, f"get_enquiry (ten targets) result is {size} characters"


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason='BUG-12: a long backend error message is returned uncut by the tools (CONTRACT: tool results < 2k chars)')
def test_tool_error_message_under_2k(fake):
    fake({"error": {"code": "conflict", "message": "m" * 5000}})
    out = enquiry.close_enquiry("E1", "unable_to_confirm")
    size = len(json.dumps(out))
    assert out["code"] == "conflict"
    assert_known_bug(size < LIMIT, f"close_enquiry error result is {size} characters")
