import sys
from pathlib import Path
from types import SimpleNamespace

import pytest

AGENT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AGENT_DIR))

import runner  # noqa: E402
import usage  # noqa: E402
from cornilia.errors import MaxStepsError  # noqa: E402


def fake_response(prompt_tokens, completion_tokens):
    return SimpleNamespace(
        usage=SimpleNamespace(prompt_tokens=prompt_tokens, completion_tokens=completion_tokens)
    )


class FakeProvider:
    def __init__(self, responses):
        self.responses = list(responses)

    def OneShotMessage(self, prompt, model_name, tools, response_configs):
        return self.responses.pop(0)


def test_usage_attach_sums_two_calls():
    provider = FakeProvider([fake_response(100, 20), fake_response(50, 5)])
    meter = usage.attach(provider)

    first = provider.OneShotMessage([], "m", [], {})
    second = provider.OneShotMessage([], "m", [], {})

    assert first.usage.prompt_tokens == 100
    assert second.usage.completion_tokens == 5
    assert meter.input_tokens == 150
    assert meter.output_tokens == 25
    assert meter.calls == 2


def test_usage_missing_usage_counts_as_zero():
    provider = FakeProvider([SimpleNamespace(usage=None)])
    meter = usage.attach(provider)
    provider.OneShotMessage([], "m", [], {})
    assert meter.input_tokens == 0
    assert meter.output_tokens == 0
    assert meter.calls == 1


class FakeHttp:
    def __init__(self):
        self.calls = []

    def __call__(self, method, path, body=None):
        self.calls.append((method, path, body))
        if method == "POST" and path == "/api/v1/agent/runs":
            return {"id": "run-1"}
        return {}

    def find(self, method, path_suffix):
        return [c for c in self.calls if c[0] == method and c[1].endswith(path_suffix)]


class FakeModel:
    """Calls the provider twice and the tracer once, then returns "done"."""

    def __init__(self, provider, tracer, raise_error=None):
        self.provider = provider
        self.tracer = tracer
        self.raise_error = raise_error

    def MultiShotMessage(self, prompt):
        if self.raise_error is not None:
            raise self.raise_error
        self.provider.OneShotMessage([], "m", [], {})
        self.provider.OneShotMessage([], "m", [], {})
        self.tracer("search_medications", {"query": "augmentin"}, '{"data": []}', 0.25)
        return "done"


@pytest.fixture
def patched(monkeypatch):
    http = FakeHttp()
    monkeypatch.setattr(runner, "http_call", http)
    monkeypatch.setattr(runner, "load_enquiry_state", lambda enquiry_id: '{"id":"enq-1"}')
    monkeypatch.setattr(runner, "CreateTools", lambda: {})
    monkeypatch.setattr(
        runner,
        "CreateProvider",
        lambda: FakeProvider([fake_response(120, 30), fake_response(80, 10)]),
    )
    return http


def test_run_completes_and_patches_token_sums(monkeypatch, patched):
    monkeypatch.setattr(
        runner,
        "build_model",
        lambda provider, tools, model_name, tracer: FakeModel(provider, tracer),
    )

    status = runner.process_wake({"enquiry_id": "enq-1", "trigger": "created"})

    assert status == "completed"
    create = patched.find("POST", "/api/v1/agent/runs")
    assert create and create[0][2]["enquiry_id"] == "enq-1"

    tool_posts = patched.find("POST", "/api/v1/agent/runs/run-1/tool-calls")
    assert len(tool_posts) == 1
    tool_body = tool_posts[0][2]
    assert tool_body["name"] == "search_medications"
    assert tool_body["input"] == {"query": "augmentin"}
    assert tool_body["is_error"] is False
    assert tool_body["duration_ms"] == 250

    patches = patched.find("PATCH", "/api/v1/agent/runs/run-1")
    assert len(patches) == 1
    payload = patches[0][2]
    assert payload["status"] == "completed"
    assert payload["input_tokens"] == 200
    assert payload["output_tokens"] == 40
    assert payload["steps"] == 2
    assert payload["error"] == ""


def test_max_steps_marks_run_failed(monkeypatch, patched):
    monkeypatch.setattr(
        runner,
        "build_model",
        lambda provider, tools, model_name, tracer: FakeModel(
            provider, tracer, raise_error=MaxStepsError(12)
        ),
    )

    status = runner.process_wake({"enquiry_id": "enq-1", "trigger": "reply"})

    assert status == "failed"
    patches = patched.find("PATCH", "/api/v1/agent/runs/run-1")
    assert len(patches) == 1
    payload = patches[0][2]
    assert payload["status"] == "failed"
    assert payload["error"] == "max steps reached; left open for a human"


def test_other_error_marks_run_failed_with_message(monkeypatch, patched):
    monkeypatch.setattr(
        runner,
        "build_model",
        lambda provider, tools, model_name, tracer: FakeModel(
            provider, tracer, raise_error=RuntimeError("boom " * 200)
        ),
    )

    status = runner.process_wake({"enquiry_id": "enq-1", "trigger": "timer"})

    assert status == "failed"
    payload = patched.find("PATCH", "/api/v1/agent/runs/run-1")[0][2]
    assert payload["status"] == "failed"
    assert len(payload["error"]) == 500
