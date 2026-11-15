"""Lead-owned self-check of the shared `live` fixture: CONTRACT acceptance check 1 (manual flow)."""

import time


def test_kit_manual_flow_cancel_in_window_then_close_available(live):
    canary_c = live.canary_in_inbox("c")
    enquiry = live.new_enquiry()
    targets = live.add_targets(enquiry["id"], ("b", "c"))
    assert targets["b"]["status"] == "approved" and targets["c"]["status"] == "approved"

    queued = live.send(enquiry["id"])
    assert len(queued) == 2 and {m["status"] for m in queued} == {"queued"}

    time.sleep(0.4 * live.undo_window)
    c_message = next(m for m in queued if m["target_id"] == targets["c"]["id"])
    status, body = live.call("POST", f"/api/v1/messages/{c_message['id']}/cancel", "a", {})
    assert status == 200 and body["status"] == "cancelled"

    live.wait_sent(enquiry["id"])
    assert enquiry["id"] in live.inbox_ids("b")
    inbox_c = live.inbox_ids("c")
    assert canary_c in inbox_c
    assert enquiry["id"] not in inbox_c  # cancelled inside the window: C never sees it
    assert live.target(enquiry["id"], "c")["status"] == "cancelled"

    status, body = live.reply(targets["b"]["id"], "b", "available", quantity=2)
    assert status == 200, body
    assert live.target(enquiry["id"], "b")["status"] == "available"

    status, body = live.close(enquiry["id"], "available", branch="b")
    assert status == 200, body
    closed = live.detail(enquiry["id"])["enquiry"]
    assert closed["status"] == "closed" and closed["outcome"] == "available"
    assert closed["outcome_branch_id"] == live.BRANCH["b"]
    assert live.stats_row(enquiry["id"])["outcome"] == "available"


def test_kit_error_code_helper(live):
    status, body = live.call("GET", "/api/v1/me")
    assert status == 401 and live.code(body) == "unauthorized"
    assert "request_id" in body["error"]
