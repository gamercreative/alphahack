import os
import sys
from pathlib import Path

import pytest

AGENT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AGENT_DIR))

import runner  # noqa: E402


@pytest.fixture(autouse=True)
def reset_runner_state(monkeypatch):
    """Every test starts with no --policy override, no WASSIT_POLICY, and no network."""
    monkeypatch.setattr(runner, "_policy_override", None)
    monkeypatch.delenv("WASSIT_POLICY", raising=False)

    def no_network(method, path, body=None):
        raise AssertionError(f"unexpected network call: {method} {path}")

    monkeypatch.setattr(runner, "http_call", no_network)


def record_fetch(monkeypatch, wakeups):
    calls = []

    def fake_fetch():
        calls.append("fetch")
        return wakeups

    monkeypatch.setattr(runner, "fetch_wakeups", fake_fetch)
    return calls


def record_process(monkeypatch, behaviour=None):
    seen = []

    def fake_process(wake):
        seen.append(wake)
        if behaviour is not None:
            return behaviour(wake)
        return "completed"

    monkeypatch.setattr(runner, "process_wake", fake_process)
    return seen


def test_once_with_no_wakeups_exits_0(monkeypatch):
    record_fetch(monkeypatch, [])
    seen = record_process(monkeypatch)

    assert runner.main(["--once"]) == 0
    assert seen == []


def test_once_processes_every_wakeup(monkeypatch):
    wakes = [
        {"enquiry_id": "enq-1", "trigger": "created"},
        {"enquiry_id": "enq-2", "trigger": "reply"},
    ]
    record_fetch(monkeypatch, wakes)
    seen = record_process(monkeypatch)

    assert runner.main(["--once"]) == 0
    assert seen == wakes


def test_once_backend_down_exits_1(monkeypatch):
    def refused(method, path, body=None):
        raise OSError("connection refused")

    monkeypatch.setattr(runner, "http_call", refused)
    seen = record_process(monkeypatch)

    assert runner.main(["--once"]) == 1
    assert seen == []


def test_once_agent_token_rejected_exits_1(monkeypatch):
    def rejected(method, path, body=None):
        raise RuntimeError("backend GET /api/v1/agent/wakeups returned 401")

    monkeypatch.setattr(runner, "http_call", rejected)
    seen = record_process(monkeypatch)

    assert runner.main(["--once"]) == 1
    assert seen == []


def test_bad_policy_env_exits_2(monkeypatch):
    monkeypatch.setenv("WASSIT_POLICY", "banana")
    fetch_calls = record_fetch(monkeypatch, [{"enquiry_id": "enq-1"}])
    record_process(monkeypatch)

    assert runner.main(["--once"]) == 2
    assert fetch_calls == []


def test_bad_policy_flag_exits_2(monkeypatch):
    fetch_calls = record_fetch(monkeypatch, [])
    record_process(monkeypatch)

    with pytest.raises(SystemExit) as excinfo:
        runner.main(["--policy", "banana"])

    assert excinfo.value.code == 2
    assert fetch_calls == []


def test_policy_flag_beats_env(monkeypatch):
    monkeypatch.setenv("WASSIT_POLICY", "model")
    record_fetch(monkeypatch, [])
    record_process(monkeypatch)

    assert runner.main(["--once", "--policy", "scripted"]) == 0
    assert runner.current_policy() == "scripted"


def test_policy_env_beats_default(monkeypatch):
    monkeypatch.setenv("WASSIT_POLICY", "scripted")
    record_fetch(monkeypatch, [])
    record_process(monkeypatch)

    assert runner.main(["--once"]) == 0
    assert runner.current_policy() == "scripted"


def test_policy_default_is_model(monkeypatch):
    monkeypatch.delenv("WASSIT_POLICY", raising=False)
    record_fetch(monkeypatch, [])
    record_process(monkeypatch)

    assert runner.main(["--once"]) == 0
    assert runner.current_policy() == "model"


def test_one_failing_wake_does_not_stop_the_next(monkeypatch):
    def first_fails(wake):
        if wake["enquiry_id"] == "enq-1":
            raise RuntimeError("model exploded")
        return "completed"

    monkeypatch.setattr(
        runner,
        "fetch_wakeups",
        lambda: [{"enquiry_id": "enq-1", "trigger": "created"},
                 {"enquiry_id": "enq-2", "trigger": "reply"}],
    )
    seen = record_process(monkeypatch, first_fails)

    runner.process_wakeups()

    assert [w["enquiry_id"] for w in seen] == ["enq-1", "enq-2"]


def test_model_name_default_and_env(monkeypatch):
    monkeypatch.delenv("CORNILIA_MODEL", raising=False)
    assert runner.model_name_from_env() == "qwen3.5:35b-a3b"

    monkeypatch.setenv("CORNILIA_MODEL", "abc")
    assert runner.model_name_from_env() == "abc"

    monkeypatch.setenv("CORNILIA_MODEL", "  ")
    assert runner.model_name_from_env() == "qwen3.5:35b-a3b"


def test_max_steps_default_and_env(monkeypatch):
    monkeypatch.delenv("CORNILIA_MAX_STEPS", raising=False)
    assert runner.max_steps_from_env() == 12

    monkeypatch.setenv("CORNILIA_MAX_STEPS", "5")
    assert runner.max_steps_from_env() == 5


def test_load_dotenv_sets_missing_keys_only(monkeypatch, tmp_path):
    env_file = tmp_path / "test.env"
    env_file.write_text(
        "A_TEST_KEY=from_file\n"
        "# a comment line\n"
        "\n"
        "B_TEST_KEY=from_file\n"
    )
    monkeypatch.setenv("A_TEST_KEY", "from_env")
    # setenv then delenv: records the original (absent) state so teardown removes the key
    monkeypatch.setenv("B_TEST_KEY", "placeholder")
    monkeypatch.delenv("B_TEST_KEY")

    runner._load_dotenv(env_file)

    assert os.environ["A_TEST_KEY"] == "from_env"
    assert os.environ["B_TEST_KEY"] == "from_file"


def test_load_dotenv_missing_file_is_silent(tmp_path):
    runner._load_dotenv(tmp_path / "nope")


def test_build_prompt_contains_trigger_and_state():
    prompt = runner.build_prompt("reply", '{"id":"e1"}')

    assert "Trigger: reply" in prompt
    assert '{"id":"e1"}' in prompt
    assert "close_enquiry" in prompt


def test_wake_with_run_create_failure_writes_no_patch(monkeypatch):
    monkeypatch.delenv("CORNILIA_MODEL", raising=False)
    calls = []

    def create_run_fails(method, path, body=None):
        calls.append((method, path, body))
        if method == "POST" and path == "/api/v1/agent/runs":
            raise RuntimeError("backend POST /api/v1/agent/runs returned 500")
        return {}

    monkeypatch.setattr(runner, "http_call", create_run_fails)

    with pytest.raises(RuntimeError, match=r"^backend POST /api/v1/agent/runs returned 500$"):
        runner.process_wake({"enquiry_id": "enq-1", "trigger": "created"})

    assert [c[0] for c in calls] == ["POST"]
    assert [c for c in calls if c[0] == "PATCH"] == []
