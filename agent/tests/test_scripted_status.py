import sys
from pathlib import Path

import pytest

AGENT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AGENT_DIR))

import runner  # noqa: E402

ENQUIRY = {"id": "enq-1", "status": "open", "requester": "Ada", "pharmacy": None}


class FakeHttp:
    """GET returns a minimal enquiry; every POST (tool-call trace) returns an empty body."""

    def __init__(self):
        self.calls = []

    def __call__(self, method, path, body=None):
        self.calls.append((method, path, body))
        if method == "GET" and path == "/api/v1/enquiries/enq-1":
            return dict(ENQUIRY)
        return {}


@pytest.fixture
def http(monkeypatch):
    fake = FakeHttp()
    monkeypatch.setattr(runner, "http_call", fake)
    return fake


def _policy_calling(tool, **kwargs):
    """A fake scripted.decide that makes exactly one call to `tool`."""

    def decide(state, call):
        call(tool, **kwargs)

    return decide


def _tool_result(result):
    def run_tool(name, **kwargs):
        return result

    return run_tool


def test_scripted_run_ending_with_wait_is_completed(monkeypatch, http):
    monkeypatch.setattr(runner.scripted, "decide", _policy_calling("wait", minutes=30))
    monkeypatch.setattr(runner.scripted, "run_tool", _tool_result({"ok": True}))

    status, error, steps = runner.run_scripted("run-1", "enq-1")

    assert status == "completed"
    assert error == ""
    assert steps == 1


def test_scripted_run_ending_with_close_enquiry_is_completed(monkeypatch, http):
    monkeypatch.setattr(
        runner.scripted,
        "decide",
        _policy_calling("close_enquiry", enquiry_id="enq-1", reason="done"),
    )
    monkeypatch.setattr(runner.scripted, "run_tool", _tool_result({"ok": True}))

    status, error, steps = runner.run_scripted("run-1", "enq-1")

    assert status == "completed"
    assert error == ""
    assert steps == 1


def test_scripted_run_ending_with_ask_requester_is_completed(monkeypatch, http):
    monkeypatch.setattr(
        runner.scripted,
        "decide",
        _policy_calling("ask_requester", enquiry_id="enq-1", question="Which strength?"),
    )
    monkeypatch.setattr(runner.scripted, "run_tool", _tool_result({"ok": True}))

    status, error, steps = runner.run_scripted("run-1", "enq-1")

    assert status == "completed"
    assert error == ""
    assert steps == 1


def test_scripted_run_with_failing_terminal_tool_does_not_raise_run_ended(monkeypatch, http):
    monkeypatch.setattr(runner.scripted, "decide", _policy_calling("close_enquiry", enquiry_id="enq-1"))
    monkeypatch.setattr(runner.scripted, "run_tool", _tool_result({"error": "x"}))

    # Must return normally: a failed terminal tool must not raise RunEnded.
    status, error, steps = runner.run_scripted("run-1", "enq-1")

    assert status == "completed"
    assert error == ""
    assert steps == 1


def test_scripted_run_real_exception_is_failed(monkeypatch, http):
    def decide(state, call):
        raise RuntimeError("boom")

    monkeypatch.setattr(runner.scripted, "decide", decide)
    monkeypatch.setattr(runner.scripted, "run_tool", _tool_result({"ok": True}))

    status, error, steps = runner.run_scripted("run-1", "enq-1")

    assert status == "failed"
    assert error == "boom"
    assert steps == 0
