"""Known agent-side bugs, pinned. Lead-owned.

Each test states the CORRECT behaviour, fails today and is xfail(strict=True): the suite stays green
while the bug exists, and the test XPASSes (which strict mode reports as a failure) once it is fixed,
so the marker gets removed and the test then guards the fix. Ids match docs/TEST_REPORT_2026-11-15.md.
"""

import sys
from pathlib import Path
from typing import Annotated

import pytest
from openai.types.chat import ChatCompletion

AGENT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AGENT_DIR))

import runner  # noqa: E402
from cornilia.tools import tool  # noqa: E402


class KnownBugFailure(AssertionError):
    """Raised only by the pinned bug assertion in an xfail test so setup failures fail loudly."""


def assert_known_bug(condition, message=""):
    if not condition:
        raise KnownBugFailure(message)


def _completion(message):
    return ChatCompletion.model_validate({
        "id": "c1", "object": "chat.completion", "created": 0, "model": "m",
        "choices": [{"index": 0, "finish_reason": "tool_calls" if message.get("tool_calls") else "stop",
                     "message": dict({"role": "assistant", "content": None}, **message)}],
        "usage": {"prompt_tokens": 50, "completion_tokens": 5, "total_tokens": 55},
    })


def _tool_call(name, arguments):
    return _completion({"tool_calls": [{"id": "call_1", "type": "function",
                                        "function": {"name": name, "arguments": arguments}}]})


class _Provider:
    """Returns the scripted completions in order; counts the calls."""
    name = "fake"

    def __init__(self, items):
        self.items = list(items)
        self.calls = 0

    def OneShotMessage(self, prompt, model_name, tools, response_configs):
        self.calls += 1
        return self.items.pop(0)


class _Http:
    def __init__(self):
        self.calls = []

    def __call__(self, method, path, body=None):
        self.calls.append((method, path, body))
        return {"id": "run-1"} if (method, path) == ("POST", "/api/v1/agent/runs") else {}

    def bodies(self, method, suffix):
        return [c[2] for c in self.calls if c[0] == method and c[1].endswith(suffix)]


@pytest.fixture
def closed_with():
    """A real Cornilia `close_enquiry` tool that records the outcomes it was really called with."""
    outcomes = []

    @tool
    def close_enquiry(outcome: Annotated[str, "The outcome"]) -> dict:
        """Closes the enquiry."""
        outcomes.append(outcome)
        return {"ok": True, "status": "closed"}

    return outcomes, {"close_enquiry": close_enquiry}


def _run(monkeypatch, provider, tools):
    http = _Http()
    monkeypatch.setattr(runner, "http_call", http)
    monkeypatch.setattr(runner, "load_enquiry_state", lambda enquiry_id: '{"id":"enq-1"}')
    monkeypatch.setattr(runner, "CreateProvider", lambda: provider)
    monkeypatch.setattr(runner, "CreateTools", lambda: tools)
    monkeypatch.setattr(runner, "_policy_override", None)
    monkeypatch.delenv("WASSIT_POLICY", raising=False)
    monkeypatch.delenv("CORNILIA_MAX_STEPS", raising=False)
    runner.process_wake({"enquiry_id": "enq-1", "trigger": "reply"})
    return http


def test_bug11_terminal_tool_with_bad_json_does_not_end_the_run(monkeypatch, closed_with):
    outcomes, tools = closed_with
    provider = _Provider([_tool_call("close_enquiry", "{not json"), _completion({"content": "done"})])

    http = _run(monkeypatch, provider, tools)

    assert outcomes == []  # the tool never ran
    assert_known_bug(http.bodies("POST", "/tool-calls")[0]["is_error"] is True)
    assert_known_bug(provider.calls == 2, "the model must get the error back and be called again")


def test_bug11_terminal_tool_with_wrong_argument_does_not_end_the_run(monkeypatch, closed_with):
    outcomes, tools = closed_with
    provider = _Provider([_tool_call("close_enquiry", '{"wrong_name": "x"}'), _completion({"content": "done"})])

    http = _run(monkeypatch, provider, tools)

    assert outcomes == []
    assert_known_bug(http.bodies("POST", "/tool-calls")[0]["is_error"] is True)
    assert_known_bug(provider.calls == 2)


def test_terminal_tool_success_ends_the_run(monkeypatch, closed_with):
    """The guard for the two tests above: a real success still ends the run after one model call."""
    outcomes, tools = closed_with
    provider = _Provider([_tool_call("close_enquiry", '{"outcome": "unable_to_confirm"}'),
                          _completion({"content": "done"})])

    http = _run(monkeypatch, provider, tools)

    assert outcomes == ["unable_to_confirm"]
    assert http.bodies("POST", "/tool-calls")[0]["is_error"] is False
    assert provider.calls == 1
    assert http.bodies("PATCH", "/api/v1/agent/runs/run-1")[0]["status"] == "completed"
