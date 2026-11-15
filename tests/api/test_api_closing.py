"""Closing an enquiry: CONTRACT rule 6 (outcome preconditions) and the caller authorisation rules.

Every test builds its own enquiry and asserts exact status and error code on that enquiry only.
"""

import time


def test_close_available_with_available_reply(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "available", quantity=2)
    assert status == 200, body

    status, body = live.close(E, "available", branch="b")
    assert status == 200, body

    detail = live.detail(E)
    assert detail["enquiry"]["status"] == "closed"
    assert detail["enquiry"]["outcome"] == "available"
    assert detail["enquiry"]["outcome_branch_id"] == live.BRANCH["b"]
    assert detail["enquiry"]["closed_at"] is not None
    assert live.last_event(E, "enquiry_closed") is not None
    assert live.stats_row(E)["outcome"] == "available"


def test_close_available_before_any_reply_409(live):
    E, t = live.sent_enquiry(("b", "c"))

    status, body = live.close(E, "available", branch="b")
    assert status == 409, body
    assert live.code(body) == "conflict"

    assert live.detail(E)["enquiry"]["status"] == "waiting"


def test_close_available_at_alternative_branch_409(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "alternative", text="625mg only")
    assert status == 200, body

    status, body = live.close(E, "available", branch="b")
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_close_available_at_non_target_409(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "available", quantity=2)
    assert status == 200, body

    status, body = live.close(E, "available", branch="d")
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_close_available_at_unavailable_branch_409(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "available", quantity=2)
    assert status == 200, body
    status, body = live.reply(t["c"]["id"], "c", "unavailable")
    assert status == 200, body

    status, body = live.close(E, "available", branch="c")
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_close_unavailable_all_unavailable(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "unavailable")
    assert status == 200, body
    status, body = live.reply(t["c"]["id"], "c", "unavailable")
    assert status == 200, body

    status, body = live.close(E, "unavailable")
    assert status == 200, body
    assert live.detail(E)["enquiry"]["outcome"] == "unavailable"


def test_close_unavailable_with_pending_target_409(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "unavailable")
    assert status == 200, body

    status, body = live.close(E, "unavailable")
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_close_unavailable_with_alternative_409(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "unavailable")
    assert status == 200, body
    status, body = live.reply(t["c"]["id"], "c", "alternative", text="625mg only")
    assert status == 200, body

    status, body = live.close(E, "unavailable")
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_close_unavailable_with_no_response_409(live):
    E, t = live.sent_enquiry(("b",))
    status, body = live.call("POST", f"/api/v1/targets/{t['b']['id']}/follow-up", "a", {})
    assert status == 200, body
    live.wait_sent(E)
    time.sleep(5 * live.wait_unit + 0.6)
    status, body = live.call("POST", f"/api/v1/targets/{t['b']['id']}/no-response", "a", {})
    assert status == 200, body

    status, body = live.close(E, "unavailable")
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_close_unavailable_without_targets_409(live):
    E = live.new_enquiry()["id"]

    status, body = live.close(E, "unavailable")
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_close_unable_to_confirm_by_staff(live):
    E, t = live.sent_enquiry(("b", "c"))

    status, body = live.close(E, "unable_to_confirm")
    assert status == 200, body
    enquiry = live.detail(E)["enquiry"]
    assert enquiry["status"] == "closed"
    assert enquiry["outcome"] == "unable_to_confirm"


def test_close_cancelled_by_staff(live):
    E, t = live.sent_enquiry(("b", "c"))

    status, body = live.close(E, "cancelled")
    assert status == 200, body
    enquiry = live.detail(E)["enquiry"]
    assert enquiry["status"] == "cancelled"
    assert enquiry["outcome"] == "cancelled"


def test_close_twice_409(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.close(E, "unable_to_confirm")
    assert status == 200, body

    status, body = live.close(E, "cancelled")
    assert status == 409, body
    assert live.code(body) == "conflict"
    assert live.detail(E)["enquiry"]["outcome"] == "unable_to_confirm"


def test_close_by_other_pharmacy_403(live):
    E, t = live.sent_enquiry(("b", "c"))

    status, body = live.close(E, "cancelled", who="d")
    assert status == 403, body
    assert live.code(body) == "forbidden"
    assert live.detail(E)["enquiry"]["status"] == "waiting"


def test_close_by_target_pharmacy_403(live):
    E, t = live.sent_enquiry(("b", "c"))

    status, body = live.close(E, "cancelled", who="b")
    assert status == 403, body
    assert live.code(body) == "forbidden"


def test_agent_cannot_close_manual_enquiry_403(live):
    E, t = live.sent_enquiry(("b", "c"))

    status, body = live.close(E, "cancelled", who="agent")
    assert status == 403, body
    assert live.code(body) == "forbidden"


def test_close_anonymous_401(live):
    E, t = live.sent_enquiry(("b", "c"))

    status, body = live.raw("POST", f"/api/v1/enquiries/{E}/close", body={"outcome": "cancelled"})
    assert status == 401, body
    assert live.code(body) == "unauthorized"
