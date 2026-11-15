"""Known bugs, pinned. Lead-owned.

Each test states the CORRECT behaviour, fails today, and is marked xfail(strict=True) so that:
- the suite stays green while the bug exists;
- the day the bug is fixed the test XPASSes, strict mode turns that into a failure, and whoever
  fixed it removes the marker (the test then guards the fix).

Never "fix" one of these by weakening the assertion. Ids match docs/TEST_REPORT_2026-11-15.md.
"""

import time

import pytest

from conftest import KnownBugFailure, assert_known_bug


def _wake(live, enquiry_id):
    return live.wake_triggers(enquiry_id)


def _no_response(live, enquiry_id, target_id):
    """Follow up once, wait the required time, mark the target no_response."""
    status, body = live.call("POST", f"/api/v1/targets/{target_id}/follow-up", "a", {})
    assert status == 200, body
    live.wait_sent(enquiry_id)
    time.sleep(5 * live.wait_unit + 0.6)
    status, body = live.call("POST", f"/api/v1/targets/{target_id}/no-response", "a", {})
    assert status == 200, body


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason="BUG-01: re-adding a pharmacy whose target was cancelled returns 500 "
                          "(UNIQUE(enquiry_id, branch_id) violation is not handled)")
def test_bug01_readd_pharmacy_after_cancel_in_window(live):
    enquiry = live.new_enquiry()
    targets = live.add_targets(enquiry["id"], ("b", "c"))
    queued = live.send(enquiry["id"])
    c_message = next(m for m in queued if m["target_id"] == targets["c"]["id"])
    status, _ = live.call("POST", f"/api/v1/messages/{c_message['id']}/cancel", "a", {})
    assert status == 200
    assert live.target(enquiry["id"], "c")["status"] == "cancelled"

    status, body = live.call("POST", f"/api/v1/enquiries/{enquiry['id']}/targets", "a",
                             {"branch_ids": [live.BRANCH["c"]]})

    assert_known_bug(status == 201, f"re-adding a cancelled pharmacy must work, got {status} {live.code(body)}")
    assert_known_bug(live.target(enquiry["id"], "c")["status"] == "approved")


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason="BUG-01: re-adding a pharmacy after an undo-after-send retraction returns 500")
def test_bug01_readd_pharmacy_after_retraction(live):
    enquiry_id, _ = live.sent_enquiry(("b",))
    queued_event = live.last_event(enquiry_id, "messages_queued")
    status, _ = live.call("POST", f"/api/v1/events/{queued_event['id']}/undo", "a", {})
    assert status == 200
    assert live.target(enquiry_id, "b")["status"] == "cancelled"

    status, body = live.call("POST", f"/api/v1/enquiries/{enquiry_id}/targets", "a",
                             {"branch_ids": [live.BRANCH["b"]]})

    assert_known_bug(status == 201, f"re-adding a retracted pharmacy must work, got {status} {live.code(body)}")
    assert_known_bug(live.target(enquiry_id, "b")["status"] == "approved")


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason="BUG-02: undoing marked_no_response after the pharmacy replied resets the "
                          "target to 'sent' and discards a human pharmacist's reply (rules 2, 4)")
def test_bug02_undo_no_response_must_not_erase_a_late_reply(live):
    enquiry_id, targets = live.sent_enquiry(("b",))
    _no_response(live, enquiry_id, targets["b"]["id"])
    no_response_event = live.last_event(enquiry_id, "marked_no_response")
    status, _ = live.reply(targets["b"]["id"], "b", "available", quantity=2)
    assert status == 200
    assert live.target(enquiry_id, "b")["status"] == "available"

    status, body = live.call("POST", f"/api/v1/events/{no_response_event['id']}/undo", "a", {})

    assert_known_bug((status, live.code(body)) == (409, "conflict"),
                     "the inverse no longer applies: the target has replied")
    assert_known_bug(live.target(enquiry_id, "b")["status"] == "available")


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason="BUG-03: cancelling an enquiry leaves its queued messages in the outbox; "
                          "they are delivered after the enquiry was cancelled (rule 5)")
def test_bug03_cancelled_enquiry_must_not_deliver_queued_messages(live):
    canary_b = live.canary_in_inbox("b")
    enquiry = live.new_enquiry()
    live.add_targets(enquiry["id"], ("b",))
    live.send(enquiry["id"])
    time.sleep(0.4 * live.undo_window)
    status, _ = live.close(enquiry["id"], "cancelled")  # inside the undo window: nothing was delivered yet
    assert status == 200

    time.sleep(1.15 * live.undo_window + 2.5)  # give the dispatcher time to (wrongly) deliver

    inbox_b = live.inbox_ids("b")
    assert canary_b in inbox_b
    assert_known_bug([m["status"] for m in live.messages(enquiry["id"], kind="enquiry")] == ["cancelled"])
    assert_known_bug(enquiry["id"] not in inbox_b)


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason="BUG-04: a finished agent run can be finished again; tokens, status and "
                          "cost_micros are overwritten, so the cost report can be rewritten")
def test_bug04_finished_run_cannot_be_finished_again(live):
    enquiry = live.new_enquiry(handled_by="agent", medication_id=None)
    status, run = live.call("POST", "/api/v1/agent/runs", "agent",
                            {"enquiry_id": enquiry["id"], "trigger": "created", "model": "scripted"})
    assert status == 201
    first = {"status": "completed", "input_tokens": 1000, "output_tokens": 500, "steps": 3, "error": ""}
    status, finished = live.call("PATCH", f"/api/v1/agent/runs/{run['id']}", "agent", first)
    assert status == 200
    cost = finished["cost_micros"]

    second = {"status": "failed", "input_tokens": 9, "output_tokens": 9, "steps": 9, "error": "again"}
    status, body = live.call("PATCH", f"/api/v1/agent/runs/{run['id']}", "agent", second)

    assert_known_bug((status, live.code(body)) == (409, "conflict"))
    assert_known_bug(live.stats_row(enquiry["id"])["cost_micros"] == cost)


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason="BUG-05: sending while a clarification question is pending moves the enquiry "
                          "to 'waiting'; the requester can then never answer (clarify -> 409)")
def test_bug05_pending_question_survives_a_send(live):
    enquiry = live.new_enquiry(handled_by="agent")
    path = f"/api/v1/enquiries/{enquiry['id']}"
    status, _ = live.call("POST", f"{path}/clarification-request", "agent", {"question": "Which strength?"})
    assert status == 200
    live.add_targets(enquiry["id"], ("b",), who="a")

    send_status, _ = live.call("POST", f"{path}/send", "a", {})
    clarify_status, _ = live.call("POST", f"{path}/clarify", "a", {"answer": "1g tablets"})

    # Either the send is refused while a question is pending, or the question can still be answered.
    assert_known_bug(send_status == 409 or clarify_status == 200, f"({send_status}, {clarify_status})")


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason="BUG-06 (rule 8): customer, allergy, prescription, refill, order and cart data "
                          "is served with no credential at all, and to the agent credential")
@pytest.mark.parametrize("path", [
    "/api/v1/customers",
    "/api/v1/customers/cust-karim",
    "/api/v1/customers/cust-karim/prescriptions",
    "/api/v1/customers/cust-karim/refills",
    "/api/v1/customers/cust-karim/orders",
    "/api/v1/carts/cart-rami-1",
])
def test_bug06_patient_data_needs_a_credential(live, path):
    status, body = live.call("GET", path)  # anonymous

    assert_known_bug((status, live.code(body)) == (401, "unauthorized"))


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason="BUG-06 (rule 8): the agent credential can read customers and allergies")
def test_bug06_agent_credential_cannot_read_customers(live):
    status, body = live.call("GET", "/api/v1/customers", "agent")

    assert_known_bug(status in (401, 403), f"agent credential read patient data: {status}")


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason="BUG-07: a reply of kind 'available' with quantity 0 is accepted; the enquiry "
                          "can then be closed 'available' for zero stock")
def test_bug07_available_reply_needs_a_positive_quantity(live):
    enquiry_id, targets = live.sent_enquiry(("b",))

    status, body = live.reply(targets["b"]["id"], "b", "available", quantity=0)

    assert_known_bug((status, live.code(body)) == (400, "validation_failed"))
    assert_known_bug(live.target(enquiry_id, "b")["status"] == "sent")


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason="BUG-08: POST /agent/runs accepts an unknown trigger (and a missing model), "
                          "creates a run that never finishes, and clears the enquiry's wake-up")
def test_bug08_run_with_unknown_trigger_is_rejected_and_keeps_the_wake(live):
    enquiry = live.new_enquiry(handled_by="agent", medication_id=None)
    assert _wake(live, enquiry["id"]) == ["created"]

    status, body = live.call("POST", "/api/v1/agent/runs", "agent",
                             {"enquiry_id": enquiry["id"], "trigger": "bogus", "model": "scripted"})

    assert_known_bug((status, live.code(body)) == (400, "validation_failed"))
    assert_known_bug(_wake(live, enquiry["id"]) == ["created"])
