"""Live API tests for the agent endpoints (CONTRACT "Loop owner"): wakeups, runs, tool calls, wait.

Every test creates its own enquiry and asserts on that enquiry only.
"""

import time

import pytest

RUNS = "/api/v1/agent/runs"
FIN = {"status": "completed", "input_tokens": 1000, "output_tokens": 500, "steps": 3, "error": ""}


def agent_enquiry(live):
    """A new enquiry handled by the agent (it wakes the agent on creation)."""
    return live.new_enquiry(handled_by="agent", medication_id=None)["id"]


def wake(live, E):
    return live.wake_triggers(E)


def start(live, E):
    """Start a scripted run for enquiry E as the agent. Asserts 201 and returns the run body."""
    status, body = live.call("POST", RUNS, "agent",
                             {"enquiry_id": E, "trigger": "created", "model": "scripted"})
    assert status == 201, f"start run failed: {status} {body}"
    return body


def expected_cost(live):
    return round(1000 * live.price_in + 500 * live.price_out)


# ------------------------------------------------------------------ wakeups

def test_agent_enquiry_wakes_with_created(live):
    E = agent_enquiry(live)
    assert wake(live, E) == ["created"]


def test_manual_enquiry_never_wakes(live):
    E = live.new_enquiry()["id"]
    status, body = live.call("GET", "/api/v1/agent/wakeups", "agent")
    assert status == 200, f"wakeups failed: {status} {body}"
    assert E not in [w.get("enquiry_id") for w in body["data"]]
    assert wake(live, E) == []


# ------------------------------------------------------------------ run start

def test_run_start_returns_running_and_clears_wake(live):
    E = agent_enquiry(live)
    run = start(live, E)
    assert run["status"] == "running"
    assert run["trigger"] == "created"
    assert run["cost_micros"] == 0
    assert wake(live, E) == []


def test_run_start_unknown_enquiry_404(live):
    status, body = live.call("POST", RUNS, "agent",
                             {"enquiry_id": "nope", "trigger": "created", "model": "scripted"})
    assert status == 404
    assert live.code(body) == "not_found"


def test_run_start_by_staff_403(live):
    E = agent_enquiry(live)
    status, body = live.call("POST", RUNS, "a",
                             {"enquiry_id": E, "trigger": "created", "model": "scripted"})
    assert status == 403
    assert live.code(body) == "forbidden"


# ------------------------------------------------------------------ run finish

def test_finish_run_sets_cost(live):
    E = agent_enquiry(live)
    run = start(live, E)
    status, body = live.call("PATCH", f"{RUNS}/{run['id']}", "agent", FIN)
    assert status == 200, body
    assert body["status"] == "completed"
    assert body["cost_micros"] == expected_cost(live)
    assert body["finished_at"] is not None


def test_finish_run_shows_in_stats(live):
    E = agent_enquiry(live)
    run = start(live, E)
    status, body = live.call("PATCH", f"{RUNS}/{run['id']}", "agent", FIN)
    assert status == 200, body
    row = live.stats_row(E)
    assert row["agent_runs"] == 1
    assert row["agent_steps"] == 3
    assert row["input_tokens"] == 1000
    assert row["output_tokens"] == 500
    assert row["cost_micros"] == expected_cost(live)


@pytest.mark.parametrize(
    "attack, override",
    [
        ("status_bogus", {"status": "bogus"}),
        ("status_running", {"status": "running"}),
        ("input_tokens_negative", {"input_tokens": -5}),
        ("steps_negative", {"steps": -1}),
    ],
    ids=["status_bogus", "status_running", "input_tokens_negative", "steps_negative"],
)
def test_finish_run_rejects(live, attack, override):
    E = agent_enquiry(live)
    run = start(live, E)
    status, body = live.call("PATCH", f"{RUNS}/{run['id']}", "agent", {**FIN, **override})
    assert status == 400, f"{attack}: {status} {body}"
    assert live.code(body) == "validation_failed"


def test_finish_unknown_run_404(live):
    status, body = live.call("PATCH", f"{RUNS}/nope", "agent", FIN)
    assert status == 404
    assert live.code(body) == "not_found"


def test_finish_run_by_staff_403(live):
    E = agent_enquiry(live)
    run = start(live, E)
    status, body = live.call("PATCH", f"{RUNS}/{run['id']}", "a", FIN)
    assert status == 403
    assert live.code(body) == "forbidden"


# ------------------------------------------------------------------ tool calls

TOOL_CALL = {"name": "search_medications", "input": {"query": "augmentin"},
             "output": "{}", "is_error": False, "duration_ms": 12}


def test_tool_call_recorded(live):
    E = agent_enquiry(live)
    run = start(live, E)
    status, body = live.call("POST", f"{RUNS}/{run['id']}/tool-calls", "agent", TOOL_CALL)
    assert status == 201, body


def test_tool_call_without_name_400(live):
    E = agent_enquiry(live)
    run = start(live, E)
    payload = {k: v for k, v in TOOL_CALL.items() if k != "name"}
    status, body = live.call("POST", f"{RUNS}/{run['id']}/tool-calls", "agent", payload)
    assert status == 400
    assert live.code(body) == "validation_failed"


def test_tool_call_unknown_run_404(live):
    status, body = live.call("POST", f"{RUNS}/nope/tool-calls", "agent", TOOL_CALL)
    assert status == 404
    assert live.code(body) == "not_found"


# ------------------------------------------------------------------ wait

def test_wait_sets_timer_wake(live):
    E = agent_enquiry(live)
    start(live, E)
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/wait", "agent", {"minutes": 5})
    assert status == 200, body
    detail = live.detail(E)
    assert detail["enquiry"]["status"] == "waiting"
    assert detail["enquiry"]["next_wake_at"] is not None
    assert wake(live, E) == []
    time.sleep(5 * live.wait_unit + 0.6)
    assert wake(live, E) == ["timer"]


def test_wait_rejects_other_minutes(live):
    E = agent_enquiry(live)
    start(live, E)
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/wait", "agent", {"minutes": 7})
    assert status == 400
    assert live.code(body) == "validation_failed"


def test_wait_by_staff_403(live):
    E = agent_enquiry(live)
    start(live, E)
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/wait", "a", {"minutes": 5})
    assert status == 403
    assert live.code(body) == "forbidden"


# ------------------------------------------------------------------ visibility

def test_agent_cannot_read_manual_enquiry(live):
    E = live.new_enquiry()["id"]
    status, body = live.call("GET", f"/api/v1/enquiries/{E}", "agent")
    assert status == 403
    assert live.code(body) == "forbidden"
