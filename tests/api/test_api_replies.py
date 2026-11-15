"""Replies from target pharmacies: CONTRACT rules 1, 2, 3 (reply kinds, guards, silence vs no response).

Every test builds its own enquiry and asserts exact status and error code.
"""

import time


def _silence_then_no_response(live):
    """Send to B and C, B follows up, B stays silent past the no-response window, then staff marks B no-response."""
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.call("POST", f"/api/v1/targets/{t['b']['id']}/follow-up", "a", {})
    assert status == 200, body
    live.wait_sent(E)
    time.sleep(5 * live.wait_unit + 0.6)
    status, body = live.call("POST", f"/api/v1/targets/{t['b']['id']}/no-response", "a", {})
    assert status == 200, body
    return E, t


def test_available_reply_recorded(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "available", quantity=2)
    assert status == 200, body
    target = live.target(E, "b")
    assert target["status"] == "available"
    assert target["reply_quantity"] == 2
    assert target["replied_by_staff_id"] == "staff-achrafieh"
    assert target["replied_at"] is not None
    ev = live.last_event(E, "reply_received")
    assert ev["actor_type"] == "staff"


def test_unavailable_reply_recorded(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "unavailable")
    assert status == 200, body
    assert live.target(E, "b")["status"] == "unavailable"


def test_alternative_is_an_option_not_a_match(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "alternative", text="625mg only")
    assert status == 200, body
    target = live.target(E, "b")
    assert target["status"] == "alternative_offered"
    assert target["status"] != "available"
    assert target["reply_text"] == "625mg only"


def test_alternative_without_text_400(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "alternative")
    assert status == 400, body
    assert live.code(body) == "validation_failed"
    assert live.target(E, "b")["status"] == "sent"


def test_unknown_kind_400(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "maybe")
    assert status == 400, body
    assert live.code(body) == "validation_failed"


def test_negative_quantity_400(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "available", quantity=-3)
    assert status == 400, body
    assert live.code(body) == "validation_failed"


def test_reply_by_other_pharmacy_403(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "d", "available", quantity=1)
    assert status == 403, body
    assert live.code(body) == "forbidden"
    assert live.target(E, "b")["status"] == "sent"


def test_reply_by_requester_403(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "a", "available", quantity=1)
    assert status == 403, body
    assert live.code(body) == "forbidden"


def test_reply_by_agent_403_agent_forbidden(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "agent", "available", quantity=1)
    assert status == 403, body
    assert live.code(body) == "agent_forbidden"
    assert live.target(E, "b")["status"] == "sent"


def test_reply_anonymous_401(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.raw("POST", f"/api/v1/targets/{t['b']['id']}/reply", body={"kind": "available", "quantity": 1})
    assert status == 401, body
    assert live.code(body) == "unauthorized"


def test_reply_before_delivery_409(live):
    E = live.new_enquiry()
    targets = live.add_targets(E["id"], ("b",))
    live.send(E["id"])
    status, body = live.reply(targets["b"]["id"], "b", "available", quantity=1)
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_second_reply_409(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "available", quantity=2)
    assert status == 200, body
    status, body = live.reply(t["b"]["id"], "b", "unavailable")
    assert status == 409, body
    assert live.code(body) == "conflict"
    assert live.target(E, "b")["status"] == "available"


def test_reply_after_close_409(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.close(E, "unable_to_confirm")
    assert status == 200, body
    status, body = live.reply(t["b"]["id"], "b", "available", quantity=1)
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_reply_unknown_target_404(live):
    status, body = live.reply("nope", "b", "available", quantity=1)
    assert status == 404, body
    assert live.code(body) == "not_found"


def test_silence_is_no_response_not_unavailable(live):
    E, t = _silence_then_no_response(live)
    target = live.target(E, "b")
    assert target["status"] == "no_response"
    assert target["status"] != "unavailable"


def test_late_reply_after_no_response_is_accepted(live):
    E, t = _silence_then_no_response(live)
    status, body = live.reply(t["b"]["id"], "b", "available", quantity=1)
    assert status == 200, body
    assert live.target(E, "b")["status"] == "available"


def test_reply_does_not_touch_other_target(live):
    E, t = live.sent_enquiry(("b", "c"))
    status, body = live.reply(t["b"]["id"], "b", "available", quantity=1)
    assert status == 200, body
    assert live.target(E, "c")["status"] == "sent"
