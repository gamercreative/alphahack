"""UNDO contract tests: POST /api/v1/events/{id}/undo runs the stored inverse (CONTRACT rule 4).

Each test builds its own enquiry and asserts on that enquiry only. 409 = not reversible or already undone.
"""

import time


def _undo(live, ev):
    """Undo one event as staff a."""
    return live.call("POST", f"/api/v1/events/{ev['id']}/undo", "a", {})


def _last_prefixed(live, enquiry_id, prefix):
    """The newest event whose type starts with prefix; fails when there is none."""
    evs = [e for e in live.events(enquiry_id) if e["type"].startswith(prefix)]
    assert evs, f"no {prefix}* event on enquiry {enquiry_id}"
    return evs[-1]


def test_undo_enquiry_created_409(live):
    enquiry = live.new_enquiry()
    created = live.last_event(enquiry["id"], "enquiry_created")
    status, body = _undo(live, created)
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_undo_unknown_event_404(live):
    status, body = live.call("POST", "/api/v1/events/nope/undo", "a", {})
    assert status == 404, body
    assert live.code(body) == "not_found"


def test_undo_medication_set_restores_previous(live):
    enquiry = live.new_enquiry(medication_id=live.MED_1G)
    eid = enquiry["id"]
    status, body = live.call("POST", f"/api/v1/enquiries/{eid}/medication", "a",
                             {"medication_id": live.MED_625})
    assert status == 200, body

    ms = live.last_event(eid, "medication_set")
    status, body = _undo(live, ms)
    assert status == 200, body
    assert live.detail(eid)["enquiry"]["medication_id"] == live.MED_1G


def test_undo_writes_event_undone_and_marks_original(live):
    enquiry = live.new_enquiry(medication_id=live.MED_1G)
    eid = enquiry["id"]
    status, body = live.call("POST", f"/api/v1/enquiries/{eid}/medication", "a",
                             {"medication_id": live.MED_625})
    assert status == 200, body

    ms = live.last_event(eid, "medication_set")
    status, body = _undo(live, ms)
    assert status == 200, body

    undone = live.last_event(eid, "event_undone")
    assert undone["payload"]["undone_event_id"] == ms["id"]

    original = next(e for e in live.events(eid, type="medication_set") if e["id"] == ms["id"])
    assert original["undone_at"] is not None


def test_undo_twice_409(live):
    enquiry = live.new_enquiry(medication_id=live.MED_1G)
    eid = enquiry["id"]
    status, body = live.call("POST", f"/api/v1/enquiries/{eid}/medication", "a",
                             {"medication_id": live.MED_625})
    assert status == 200, body

    ms = live.last_event(eid, "medication_set")
    status, body = _undo(live, ms)
    assert status == 200, body

    status, body = _undo(live, ms)
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_undo_event_undone_409(live):
    enquiry = live.new_enquiry(medication_id=live.MED_1G)
    eid = enquiry["id"]
    status, body = live.call("POST", f"/api/v1/enquiries/{eid}/medication", "a",
                             {"medication_id": live.MED_625})
    assert status == 200, body

    ms = live.last_event(eid, "medication_set")
    status, body = _undo(live, ms)
    assert status == 200, body

    undone = live.last_event(eid, "event_undone")
    status, body = _undo(live, undone)
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_undo_targets_removes_them(live):
    enquiry = live.new_enquiry()
    eid = enquiry["id"]
    live.add_targets(eid, ("b", "c"))

    targets_ev = _last_prefixed(live, eid, "targets_")
    assert targets_ev["inverse"]["op"] == "delete_targets"

    status, body = _undo(live, targets_ev)
    assert status == 200, body
    assert live.detail(eid)["targets"] == []


def test_undo_send_inside_window(live):
    canary_b = live.canary_in_inbox("b")
    enquiry = live.new_enquiry()
    eid = enquiry["id"]
    live.add_targets(eid, ("b", "c"))
    live.send(eid)

    time.sleep(0.4 * live.undo_window)
    queued = live.last_event(eid, "messages_queued")
    status, body = _undo(live, queued)
    assert status == 200, body

    msgs = live.messages(eid)
    assert len(msgs) == 2, msgs
    assert all(m["status"] == "cancelled" for m in msgs), msgs
    assert live.target(eid, "b")["status"] == "approved"
    assert live.target(eid, "c")["status"] == "approved"
    assert live.detail(eid)["enquiry"]["status"] == "open"

    time.sleep(1.15 * live.undo_window + 2.5)
    inbox_b = live.inbox_ids("b")
    assert canary_b in inbox_b
    assert eid not in inbox_b


def test_undo_send_after_delivery_queues_retraction(live):
    eid, _ = live.sent_enquiry(("b",))

    queued = live.last_event(eid, "messages_queued")
    status, body = _undo(live, queued)
    assert status == 200, body

    retractions = live.messages(eid, kind="retraction")
    assert len(retractions) == 1, retractions
    assert live.target(eid, "b")["status"] == "cancelled"
    assert live.events(eid, type="event_undone")

    live.wait_sent(eid)
    retractions = live.messages(eid, kind="retraction")
    assert len(retractions) == 1, retractions
    assert retractions[0]["status"] == "sent"


def test_undo_close_reopens(live):
    eid, _ = live.sent_enquiry()

    status, body = live.close(eid, "unable_to_confirm")
    assert status == 200, body

    closed = live.last_event(eid, "enquiry_closed")
    status, body = _undo(live, closed)
    assert status == 200, body

    enquiry = live.detail(eid)["enquiry"]
    assert enquiry["status"] == "waiting"
    assert enquiry["outcome"] == ""
    assert enquiry["closed_at"] is None


def test_undo_no_response_returns_to_sent(live):
    eid, targets = live.sent_enquiry(("b",))
    target_b = targets["b"]

    status, body = live.call("POST", f"/api/v1/targets/{target_b['id']}/follow-up", "a", {})
    assert status == 200, body
    live.wait_sent(eid)
    time.sleep(5 * live.wait_unit + 0.6)

    status, body = live.call("POST", f"/api/v1/targets/{target_b['id']}/no-response", "a", {})
    assert status == 200, body

    marked = live.last_event(eid, "marked_no_response")
    status, body = _undo(live, marked)
    assert status == 200, body
    assert live.target(eid, "b")["status"] == "sent"


def test_undo_on_closed_enquiry_409(live):
    enquiry = live.new_enquiry()
    eid = enquiry["id"]
    live.add_targets(eid, ("b",))

    targets_ev = _last_prefixed(live, eid, "targets_")

    status, body = live.close(eid, "cancelled")
    assert status == 200, body

    status, body = _undo(live, targets_ev)
    assert status == 409, body
    assert live.code(body) == "conflict"
