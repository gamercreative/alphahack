"""Access control: a pharmacy that is not the requester cannot change someone else's enquiry.

A (Hamra) is the requester, B (Achrafieh) is a contacted target, D (Verdun) is not involved.
Every test asserts the exact HTTP status and the exact error code.
"""

import pytest

MUTATING_ROUTES = [
    "medication", "candidates", "targets", "send", "notify", "close",
    "cancel", "follow_up", "no_response", "undo", "clarify", "approve",
]

# Route name -> (method, path, body) for the scene ids.
ROUTE_TABLE = {
    "medication": ("POST", "/api/v1/enquiries/{E}/medication", {"medication_id": "med-augmentin-625"}),
    "candidates": ("GET", "/api/v1/enquiries/{E}/candidates", None),
    "targets": ("POST", "/api/v1/enquiries/{E}/targets", {"branch_ids": ["branch-baabda"]}),
    "send": ("POST", "/api/v1/enquiries/{E}/send", {}),
    "notify": ("POST", "/api/v1/enquiries/{E}/notify", {"body": "x"}),
    "close": ("POST", "/api/v1/enquiries/{E}/close", {"outcome": "cancelled"}),
    "cancel": ("POST", "/api/v1/messages/{msg}/cancel", {}),
    "follow_up": ("POST", "/api/v1/targets/{target}/follow-up", {}),
    "no_response": ("POST", "/api/v1/targets/{target}/no-response", {}),
    "undo": ("POST", "/api/v1/events/{event}/undo", {}),
    "clarify": ("POST", "/api/v1/enquiries/{E}/clarify", {"answer": "x"}),
    "approve": ("POST", "/api/v1/enquiries/{E}/targets/approve", {"target_ids": ["{target}"]}),
}


def _route(scene, name):
    """Resolve a route name to (method, path, body) using the scene ids."""
    method, path, body = ROUTE_TABLE[name]
    path = path.format(E=scene["E"], msg=scene["msg"], target=scene["target"], event=scene["event"])
    if body is not None:
        body = _fill(body, scene)
    return method, path, body


def _fill(value, scene):
    """Replace the {target} placeholder inside a JSON body with the scene target id."""
    if isinstance(value, dict):
        return {k: _fill(v, scene) for k, v in value.items()}
    if isinstance(value, list):
        return [_fill(v, scene) for v in value]
    if value == "{target}":
        return scene["target"]
    return value


@pytest.fixture(scope="module")
def scene(live):
    """Build one requester enquiry (A) sent to B, shared by every test in this module."""
    E, t = live.sent_enquiry(("b",))
    msg = live.messages(E, kind="enquiry")[0]
    ev = live.last_event(E, "messages_queued")
    return dict(E=E, target=t["b"]["id"], msg=msg["id"], event=ev["id"])


# ---------------------------------------------------------------- uninvolved pharmacy (D)


@pytest.mark.parametrize("route", sorted(ROUTE_TABLE))
def test_uninvolved_pharmacy_is_forbidden(live, scene, route):
    method, path, body = _route(scene, route)
    status, resp = live.call(method, path, "d", body)
    assert status == 403, f"{route}: expected 403, got {status} {resp}"
    assert live.code(resp) == "forbidden", f"{route}: expected code forbidden, got {resp}"


# ---------------------------------------------------------------- target pharmacy (B) acting as requester


@pytest.mark.parametrize("route", sorted(ROUTE_TABLE))
def test_target_pharmacy_cannot_change_requesters_enquiry(live, scene, route):
    method, path, body = _route(scene, route)
    status, resp = live.call(method, path, "b", body)
    assert status == 403, f"{route}: expected 403, got {status} {resp}"
    assert live.code(resp) == "forbidden", f"{route}: expected code forbidden, got {resp}"


# ---------------------------------------------------------------- agent on a manual enquiry


@pytest.mark.parametrize("route", [
    "medication", "candidates", "targets", "send", "notify", "close",
    "cancel", "follow_up", "no_response", "undo",
])
def test_agent_cannot_act_on_manual_enquiry(live, scene, route):
    method, path, body = _route(scene, route)
    status, resp = live.call(method, path, "agent", body)
    assert status == 403, f"{route}: expected 403, got {status} {resp}"
    assert live.code(resp) == "forbidden", f"{route}: expected code forbidden, got {resp}"


# ---------------------------------------------------------------- anonymous caller


@pytest.mark.parametrize("route", sorted(ROUTE_TABLE))
def test_anonymous_is_unauthorized(live, scene, route):
    method, path, body = _route(scene, route)
    status, resp = live.raw(method, path, authorization=None, body=body)
    assert status == 401, f"{route}: expected 401, got {status} {resp}"
    assert live.code(resp) == "unauthorized", f"{route}: expected code unauthorized, got {resp}"


# ---------------------------------------------------------------- read access


def test_uninvolved_pharmacy_cannot_read(live, scene):
    status, resp = live.call("GET", f"/api/v1/enquiries/{scene['E']}", "d")
    assert status == 403, f"expected 403, got {status} {resp}"
    assert live.code(resp) == "forbidden", f"expected code forbidden, got {resp}"


def test_target_pharmacy_can_read(live, scene):
    status, resp = live.call("GET", f"/api/v1/enquiries/{scene['E']}", "b")
    assert status == 200, f"expected 200, got {status} {resp}"
    assert resp["enquiry"]["id"] == scene["E"]


# ---------------------------------------------------------------- nothing changed (must stay last)


def test_zz_nothing_changed(live, scene):
    E = scene["E"]
    detail = live.detail(E, "a")
    assert detail["enquiry"]["status"] == "waiting", detail["enquiry"]
    b = live.target(E, "b", "a")
    assert b["status"] == "sent", b
    assert detail["enquiry"]["medication_id"] == live.MED_1G, detail["enquiry"]
    assert len(detail["targets"]) == 1, detail["targets"]
    forbidden_types = {
        "enquiry_closed", "message_cancelled", "follow_up_queued",
        "marked_no_response", "event_undone", "requester_notified",
    }
    found = sorted({e["type"] for e in live.events(E, "a") if e["type"] in forbidden_types})
    assert found == [], f"unexpected mutation events on the enquiry: {found}"
