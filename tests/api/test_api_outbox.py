"""Outbox rules (CONTRACT rules 3 and 5): send queue, undo window, follow-up, no-response, notify.

Each test builds its own enquiry and asserts on that enquiry only. Every error case asserts the
exact HTTP status and the exact error code.
"""

import time


def _cancel(live, message_id, who="a"):
    return live.call("POST", f"/api/v1/messages/{message_id}/cancel", who, {})


def _follow_up(live, target_id, who="a"):
    return live.call("POST", f"/api/v1/targets/{target_id}/follow-up", who, {})


def _no_response(live, target_id, who="a"):
    return live.call("POST", f"/api/v1/targets/{target_id}/no-response", who, {})


def test_send_queues_one_message_per_target(live):
    enquiry = live.new_enquiry()
    targets = live.add_targets(enquiry["id"], ("b", "c"))

    queued = live.send(enquiry["id"])
    assert len(queued) == 2
    assert {m["target_id"] for m in queued} == {targets["b"]["id"], targets["c"]["id"]}
    for m in queued:
        assert m["kind"] == "enquiry"
        assert m["status"] == "queued"
        assert m["created_by_actor"] == "staff:staff-hamra"

    detail = live.detail(enquiry["id"])
    assert detail["enquiry"]["status"] == "waiting"

    live.wait_sent(enquiry["id"])
    assert live.target(enquiry["id"], "b")["status"] == "sent"
    assert live.target(enquiry["id"], "c")["status"] == "sent"


def test_send_without_targets_409(live):
    enquiry = live.new_enquiry()
    status, body = live.call("POST", f"/api/v1/enquiries/{enquiry['id']}/send", "a", {})
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_queued_message_not_in_inbox(live):
    canary_b = live.canary_in_inbox("b")
    enquiry = live.new_enquiry()
    live.add_targets(enquiry["id"], ("b",))
    queued = live.send(enquiry["id"])
    assert len(queued) == 1 and queued[0]["status"] == "queued"

    time.sleep(0.4 * live.undo_window)
    assert [m["status"] for m in live.messages(enquiry["id"], kind="enquiry")] == ["queued"]
    inbox_b = live.inbox_ids("b")
    assert canary_b in inbox_b
    assert enquiry["id"] not in inbox_b


def test_dispatcher_delivers(live):
    enquiry = live.new_enquiry()
    targets = live.add_targets(enquiry["id"], ("b",))
    queued = live.send(enquiry["id"])
    message_id = queued[0]["id"]
    assert targets["b"]["id"] == queued[0]["target_id"]

    live.wait_sent(enquiry["id"])
    message = next(m for m in live.messages(enquiry["id"]) if m["id"] == message_id)
    assert message["status"] == "sent"
    assert message["sent_at"] is not None

    sent_events = live.events(enquiry["id"], type="message_sent")
    assert sent_events, "no message_sent event"
    assert sent_events[-1]["actor_type"] == "system"

    assert enquiry["id"] in live.inbox_ids("b")


def test_cancel_in_window_target_never_sees_it(live):
    canary_c = live.canary_in_inbox("c")
    enquiry = live.new_enquiry()
    targets = live.add_targets(enquiry["id"], ("b", "c"))
    queued = live.send(enquiry["id"])
    c_message = next(m for m in queued if m["target_id"] == targets["c"]["id"])

    time.sleep(0.4 * live.undo_window)
    status, body = _cancel(live, c_message["id"])
    assert status == 200, body
    assert body["status"] == "cancelled"
    assert live.target(enquiry["id"], "c")["status"] == "cancelled"
    assert live.events(enquiry["id"], type="message_cancelled")

    live.wait_sent(enquiry["id"])
    inbox_c = live.inbox_ids("c")
    assert canary_c in inbox_c
    assert enquiry["id"] not in inbox_c
    assert enquiry["id"] in live.inbox_ids("b")


def test_cancel_twice_409(live):
    enquiry = live.new_enquiry()
    live.add_targets(enquiry["id"], ("b",))
    queued = live.send(enquiry["id"])
    message_id = queued[0]["id"]

    status, body = _cancel(live, message_id)
    assert status == 200, body

    status, body = _cancel(live, message_id)
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_cancel_after_sent_409(live):
    enquiry = live.new_enquiry()
    live.add_targets(enquiry["id"], ("b",))
    queued = live.send(enquiry["id"])
    message = queued[0]

    time.sleep(0.4 * live.undo_window)
    assert live.messages(enquiry["id"], kind="enquiry")[0]["status"] == "queued"

    time.sleep(0.75 * live.undo_window)  # total elapsed >= 1.15 * undo_window
    live.wait_sent(enquiry["id"])

    status, body = _cancel(live, message["id"])
    assert status == 409, body
    assert live.code(body) == "conflict"

    still = next(m for m in live.messages(enquiry["id"]) if m["id"] == message["id"])
    assert still["status"] == "sent"


def test_cancel_unknown_404(live):
    status, body = _cancel(live, "nope")
    assert status == 404, body
    assert live.code(body) == "not_found"


def test_follow_up_before_delivery_409(live):
    enquiry = live.new_enquiry()
    targets = live.add_targets(enquiry["id"], ("b",))
    live.send(enquiry["id"])

    status, body = _follow_up(live, targets["b"]["id"])
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_follow_up_once(live):
    enquiry_id, targets = live.sent_enquiry(("b",))
    target_id = targets["b"]["id"]

    status, body = _follow_up(live, target_id)
    assert status == 200, body

    assert live.target(enquiry_id, "b")["follow_ups"] == 1
    follow_ups = live.messages(enquiry_id, kind="follow_up")
    assert len(follow_ups) == 1
    assert live.events(enquiry_id, type="follow_up_queued")


def test_follow_up_twice_409(live):
    enquiry_id, targets = live.sent_enquiry(("b",))
    target_id = targets["b"]["id"]

    status, body = _follow_up(live, target_id)
    assert status == 200, body

    status, body = _follow_up(live, target_id)
    assert status == 409, body
    assert live.code(body) == "conflict"
    assert live.target(enquiry_id, "b")["follow_ups"] == 1


def test_no_response_without_follow_up_409(live):
    enquiry_id, targets = live.sent_enquiry(("b",))

    status, body = _no_response(live, targets["b"]["id"])
    assert status == 409, body
    assert live.code(body) == "conflict"
    assert live.target(enquiry_id, "b")["status"] == "sent"


def test_no_response_too_soon_409(live):
    enquiry_id, targets = live.sent_enquiry(("b",))
    target_id = targets["b"]["id"]

    status, body = _follow_up(live, target_id)
    assert status == 200, body
    live.wait_sent(enquiry_id)

    status, body = _no_response(live, target_id)
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_no_response_after_wait(live):
    enquiry_id, targets = live.sent_enquiry(("b",))
    target_id = targets["b"]["id"]

    status, body = _follow_up(live, target_id)
    assert status == 200, body
    live.wait_sent(enquiry_id)

    time.sleep(5 * live.wait_unit + 0.6)

    status, body = _no_response(live, target_id)
    assert status == 200, body
    assert live.target(enquiry_id, "b")["status"] == "no_response"
    assert live.events(enquiry_id, type="marked_no_response")


def test_no_response_twice_409(live):
    enquiry_id, targets = live.sent_enquiry(("b",))
    target_id = targets["b"]["id"]

    status, body = _follow_up(live, target_id)
    assert status == 200, body
    live.wait_sent(enquiry_id)
    time.sleep(5 * live.wait_unit + 0.6)

    status, body = _no_response(live, target_id)
    assert status == 200, body

    status, body = _no_response(live, target_id)
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_notify(live):
    enquiry = live.new_enquiry()
    status, body = live.call("POST", f"/api/v1/enquiries/{enquiry['id']}/notify", "a", {"body": "hello"})
    assert status == 200, body

    notifications = live.messages(enquiry["id"], kind="notification")
    assert len(notifications) == 1
    assert live.events(enquiry["id"], type="requester_notified")


def test_notify_empty_body_400(live):
    enquiry = live.new_enquiry()
    status, body = live.call("POST", f"/api/v1/enquiries/{enquiry['id']}/notify", "a", {"body": ""})
    assert status == 400, body
    assert live.code(body) == "validation_failed"


def test_notify_1001_chars_400(live):
    enquiry = live.new_enquiry()
    status, body = live.call("POST", f"/api/v1/enquiries/{enquiry['id']}/notify", "a", {"body": "x" * 1001})
    assert status == 400, body
    assert live.code(body) == "validation_failed"
