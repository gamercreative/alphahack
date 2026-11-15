"""End to end: the real Wassit runner (scripted policy, no model) against the live backend. Lead-owned.

Each test creates its own agent enquiry, runs `agent/runner.py --once --policy scripted` as a
subprocess the way scripts/demo_flow.py does, and plays the human parts (approve, reply) over HTTP.
CONTRACT acceptance checks 2 and 6, plus the runner's exit codes when the backend is unusable.

The runner processes every pending wake-up on the backend, so run this file against the dedicated
test stack, not in parallel with other live tests.
"""

import os
import subprocess
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
AGENT_PY = ROOT / "agent" / ".venv" / "bin" / "python"
RUNNER = ROOT / "agent" / "runner.py"
LETTER = {"branch-hamra": "a", "branch-achrafieh": "b", "branch-baabda": "c",
          "branch-verdun": "d", "branch-byblos": "e"}


def run_agent(base_url, agent_token):
    """One runner pass. Returns the CompletedProcess."""
    env = dict(os.environ, BACKEND_URL=base_url, AGENT_TOKEN=agent_token, WASSIT_POLICY="scripted")
    return subprocess.run([str(AGENT_PY), str(RUNNER), "--once", "--policy", "scripted"],
                          cwd=str(ROOT), env=env, capture_output=True, text=True, timeout=120)


def agent_pass(live):
    proc = run_agent(live.base, live.agent_token)
    assert proc.returncode == 0, f"runner exited {proc.returncode}: {proc.stdout[-600:]} {proc.stderr[-600:]}"
    return proc


def wake(live, enquiry_id):
    return live.wake_triggers(enquiry_id)


def approve_all(live, enquiry_id):
    """The requester approves every proposed target. Returns the targets ordered as the backend lists them."""
    targets = live.detail(enquiry_id)["targets"]
    proposed = [t["id"] for t in targets if t["status"] == "proposed"]
    assert proposed, f"nothing to approve: {[t['status'] for t in targets]}"
    status, body = live.call("POST", f"/api/v1/enquiries/{enquiry_id}/targets/approve", "a",
                             {"target_ids": proposed})
    assert status == 200, body
    return targets


def run_until_closed(live, enquiry_id, rounds=8):
    """Keep giving the agent turns (its timers are 5 wait units) until the enquiry is closed."""
    for _ in range(rounds):
        live.wait_sent(enquiry_id)
        time.sleep(5 * live.wait_unit + 0.6)
        agent_pass(live)
        if live.detail(enquiry_id)["enquiry"]["status"] == "closed":
            return
    raise AssertionError(f"enquiry not closed after {rounds} agent passes: "
                         f"{[(e['type'], e['actor_type']) for e in live.events(enquiry_id)]}")


def test_e2e_agent_flow_ends_unable_to_confirm_and_never_available(live):
    """Acceptance check 2: unavailable + alternative + silence -> notify, one follow-up, no_response, unable_to_confirm."""
    enquiry = live.new_enquiry(handled_by="agent", medication_id=None, raw_request="need augmentin 1g tabs x2")
    enquiry_id = enquiry["id"]

    agent_pass(live)  # run 1: match the medicine, propose pharmacies, stop for the human
    detail = live.detail(enquiry_id)
    assert detail["enquiry"]["medication_id"] == live.MED_1G
    assert detail["enquiry"]["status"] == "awaiting_approval"
    assert len(detail["targets"]) == 3 and {t["status"] for t in detail["targets"]} == {"proposed"}
    assert detail["messages"] == [], "nothing may be sent before a human approves"
    assert detail["hitl_count"] == 0

    targets = approve_all(live, enquiry_id)
    assert live.detail(enquiry_id)["hitl_count"] == 1
    agent_pass(live)  # run 2: send to the approved pharmacies
    live.wait_sent(enquiry_id)
    assert {t["status"] for t in live.detail(enquiry_id)["targets"]} == {"sent"}

    unavailable, alternative, silent = targets
    status, body = live.reply(unavailable["id"], LETTER[unavailable["branch_id"]], "unavailable")
    assert status == 200, body
    status, body = live.reply(alternative["id"], LETTER[alternative["branch_id"]], "alternative", text="625mg only")
    assert status == 200, body

    run_until_closed(live, enquiry_id)

    detail = live.detail(enquiry_id)
    by_id = {t["id"]: t for t in detail["targets"]}
    assert detail["enquiry"]["outcome"] == "unable_to_confirm"
    assert detail["enquiry"]["outcome_branch_id"] is None
    assert by_id[unavailable["id"]]["status"] == "unavailable"
    assert by_id[alternative["id"]]["status"] == "alternative_offered"  # an option, never a match (rule 1)
    assert by_id[silent["id"]]["status"] == "no_response"  # silence is never "unavailable" (rule 3)
    assert by_id[silent["id"]]["follow_ups"] == 1  # exactly one follow-up
    assert len([m for m in detail["messages"] if m["kind"] == "follow_up"]) == 1
    assert len([m for m in detail["messages"] if m["kind"] == "notification"]) >= 1  # A was told about the option
    assert detail["hitl_count"] == 1  # one human approval, nothing else

    human_only = {"reply_received", "targets_approved", "clarified"}
    assert [e["type"] for e in detail["events"] if e["actor_type"] == "agent" and e["type"] in human_only] == []
    assert [e for e in detail["events"] if e["type"] == "enquiry_closed"][0]["actor_type"] == "agent"

    assert len(detail["runs"]) >= 3 and {r["status"] for r in detail["runs"]} == {"completed"}
    row = live.stats_row(enquiry_id)
    assert row["outcome"] == "unable_to_confirm" and row["hitl_count"] == 1
    assert row["agent_runs"] == len(detail["runs"]) and row["agent_steps"] > 0
    assert row["cost_micros"] == 0  # the scripted policy uses no tokens


def test_e2e_agent_closes_available_only_after_a_pharmacist_says_so(live):
    enquiry_id = live.new_enquiry(handled_by="agent", medication_id=live.MED_1G)["id"]
    agent_pass(live)
    targets = approve_all(live, enquiry_id)
    agent_pass(live)
    live.wait_sent(enquiry_id)
    assert live.detail(enquiry_id)["enquiry"]["status"] == "waiting"  # no reply yet: the agent has not closed

    winner = targets[0]
    status, body = live.reply(winner["id"], LETTER[winner["branch_id"]], "available", quantity=2)
    assert status == 200, body
    assert wake(live, enquiry_id) == ["reply"]
    agent_pass(live)

    closed = live.detail(enquiry_id)["enquiry"]
    assert (closed["status"], closed["outcome"]) == ("closed", "available")
    assert closed["outcome_branch_id"] == winner["branch_id"]


def test_e2e_ambiguous_request_asks_the_requester(live):
    """Acceptance check 6: "need augmentin" (two strengths in the catalogue) -> ask, hitl +1, run ends."""
    enquiry_id = live.new_enquiry(handled_by="agent", medication_id=None, raw_request="need augmentin")["id"]

    agent_pass(live)

    detail = live.detail(enquiry_id)
    assert detail["enquiry"]["status"] == "awaiting_clarification"
    assert detail["enquiry"]["clarification_question"] != ""
    assert detail["enquiry"]["medication_id"] is None  # it did not guess a strength (rules 1, 7)
    assert detail["targets"] == [] and detail["messages"] == []
    assert [r["status"] for r in detail["runs"]] == ["completed"]
    assert wake(live, enquiry_id) == []  # the run ended and nothing wakes the agent until the human answers

    status, body = live.call("POST", f"/api/v1/enquiries/{enquiry_id}/clarify", "a", {"answer": "1g tablets"})
    assert status == 200, body
    assert live.detail(enquiry_id)["hitl_count"] == 1
    assert wake(live, enquiry_id) == ["clarified"]


def test_e2e_agent_pass_with_nothing_to_do_exits_0_and_changes_nothing(live):
    enquiry_id = live.new_enquiry()["id"]  # manual: never the agent's business
    before = len(live.events(enquiry_id))

    agent_pass(live)

    detail = live.detail(enquiry_id)
    assert len(detail["events"]) == before and detail["runs"] == []


def test_e2e_runner_exits_1_when_the_backend_is_down():
    proc = run_agent("http://127.0.0.1:9", "irrelevant")

    assert proc.returncode == 1, proc.stdout + proc.stderr
    assert "Traceback" not in proc.stderr


def test_e2e_runner_with_a_wrong_agent_token_exits_1_and_keeps_the_wake(live):
    enquiry_id = live.new_enquiry(handled_by="agent", medication_id=None)["id"]

    proc = run_agent(live.base, "definitely-not-the-agent-token")

    assert proc.returncode == 1, proc.stdout + proc.stderr
    assert wake(live, enquiry_id) == ["created"]  # nothing was consumed
    assert live.detail(enquiry_id)["runs"] == []
