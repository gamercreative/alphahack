"""Live API tests: targets, medication, candidates and enquiry lists (one test per case).

Each test creates its own enquiry, asserts the exact HTTP status and the exact error code.
"""


def _ids(body):
    """Enquiry ids of a list response (rows carry "id", "enquiry_id" or a nested "enquiry")."""
    ids = set()
    for row in body.get("data", []):
        ident = row.get("id") or row.get("enquiry_id") or (row.get("enquiry") or {}).get("id")
        ids.add(ident)
    return ids


def test_staff_targets_are_approved(live):
    enquiry = live.new_enquiry(who="a")
    E = enquiry["id"]
    B = live.BRANCH
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/targets", "a",
                             {"branch_ids": [B["b"], B["c"]]})
    assert status == 201, body
    assert len(body["data"]) == 2, body
    for target in body["data"]:
        assert target["status"] == "approved", target
        assert target["follow_ups"] == 0, target


def test_own_branch_rejected(live):
    enquiry = live.new_enquiry(who="a")
    E = enquiry["id"]
    B = live.BRANCH
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/targets", "a", {"branch_ids": [B["a"]]})
    assert status == 400 and live.code(body) == "validation_failed", (status, body)


def test_unknown_branch_404(live):
    enquiry = live.new_enquiry(who="a")
    E = enquiry["id"]
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/targets", "a", {"branch_ids": ["branch-nope"]})
    assert status == 404 and live.code(body) == "not_found", (status, body)


def test_empty_list_rejected(live):
    enquiry = live.new_enquiry(who="a")
    E = enquiry["id"]
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/targets", "a", {"branch_ids": []})
    assert status == 400 and live.code(body) == "validation_failed", (status, body)


def test_duplicate_in_one_call_creates_one(live):
    enquiry = live.new_enquiry(who="a")
    E = enquiry["id"]
    B = live.BRANCH
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/targets", "a",
                             {"branch_ids": [B["b"], B["b"]]})
    assert status == 201, body
    detail = live.detail(E, "a")
    assert len(detail["targets"]) == 1, detail["targets"]


def test_same_branch_twice_409(live):
    enquiry = live.new_enquiry(who="a")
    E = enquiry["id"]
    B = live.BRANCH
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/targets", "a", {"branch_ids": [B["b"]]})
    assert status == 201, body
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/targets", "a", {"branch_ids": [B["b"]]})
    assert status == 409 and live.code(body) == "conflict", (status, body)
    detail = live.detail(E, "a")
    assert len(detail["targets"]) == 1, detail["targets"]


def test_targets_on_closed_enquiry_409(live):
    enquiry = live.new_enquiry(who="a")
    E = enquiry["id"]
    B = live.BRANCH
    status, body = live.close(E, "cancelled", who="a")
    assert status == 200, body
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/targets", "a", {"branch_ids": [B["b"]]})
    assert status == 409 and live.code(body) == "conflict", (status, body)


def test_detail_shape(live):
    enquiry = live.new_enquiry(who="a")
    E = enquiry["id"]
    detail = live.detail(E, "a")
    assert set(detail.keys()) == {"enquiry", "medication", "targets", "messages", "events", "runs", "hitl_count"}, \
        sorted(detail.keys())
    events = live.events(E, "a")
    assert events, "no events on a new enquiry"
    first = events[0]
    assert first["type"] == "enquiry_created", first
    assert first["actor_type"] == "staff", first
    assert first["actor_id"] == "staff-hamra", first


def test_set_medication_updates_and_logs(live):
    enquiry = live.new_enquiry(who="a", medication_id=None)
    E = enquiry["id"]
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/medication", "a",
                             {"medication_id": live.MED_625})
    assert status == 200, body
    detail = live.detail(E, "a")
    assert detail["enquiry"]["medication_id"] == live.MED_625, detail["enquiry"]
    last = live.last_event(E, "medication_set", "a")
    assert last["inverse"]["op"] == "set_medication", last


def test_set_medication_unknown_404(live):
    enquiry = live.new_enquiry(who="a", medication_id=live.MED_1G)
    E = enquiry["id"]
    status, body = live.call("POST", f"/api/v1/enquiries/{E}/medication", "a",
                             {"medication_id": "med-nope"})
    assert status == 404 and live.code(body) == "not_found", (status, body)
    detail = live.detail(E, "a")
    assert detail["enquiry"]["medication_id"] == live.MED_1G, detail["enquiry"]


def test_candidates_exclude_requester_and_respect_limit(live):
    enquiry = live.new_enquiry(who="a")
    E = enquiry["id"]
    status, body = live.call("GET", f"/api/v1/enquiries/{E}/candidates?limit=2", "a")
    assert status == 200, body
    items = body["data"]
    assert len(items) <= 2, items
    for item in items:
        assert item["branch_id"] != live.BRANCH["a"], item
        assert {"branch_id", "distance_km", "has_reported_stock"} <= set(item.keys()), item


def test_list_outgoing_contains_own(live):
    canary_b = live.new_enquiry(who="b")["id"]
    enquiry = live.new_enquiry(who="a")
    E = enquiry["id"]
    assert E in live.enquiry_ids("a", scope="outgoing"), "own enquiry missing from staff a list"
    outgoing_b = live.enquiry_ids("b", scope="outgoing")
    assert canary_b in outgoing_b
    assert E not in outgoing_b, "enquiry created by a leaked into outgoing list of b"


def test_list_incoming_after_delivery(live):
    canary_c = live.canary_in_inbox("c")
    E, _ = live.sent_enquiry(("b",))
    assert E in live.enquiry_ids("b", scope="incoming"), "delivered enquiry missing from incoming list of b"
    incoming_c = live.enquiry_ids("c", scope="incoming")
    assert canary_c in incoming_c
    assert E not in incoming_c, "enquiry not addressed to c appears in incoming list of c"


def test_list_limit_is_capped_at_200(live):
    live.new_enquiry(who="a")
    status, body = live.call("GET", "/api/v1/enquiries?limit=100000", "a")
    assert status == 200, body
    assert body["limit"] == 200, body["limit"]
    status, body = live.call("GET", "/api/v1/enquiries?limit=1", "a")
    assert status == 200, body
    assert len(body["data"]) == 1, body["data"]
