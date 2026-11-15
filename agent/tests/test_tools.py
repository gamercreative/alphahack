import io
import json
import urllib.error
import urllib.request

import pytest

import _http
import catalogue
import enquiry
from cornilia.tools import Tool


class FakeCall:
    """Replaces _http.call. Records every request and returns canned answers in order."""

    def __init__(self, *responses):
        self.responses = list(responses)
        self.calls = []

    def __call__(self, method, path, body=None, query=None):
        self.calls.append((method, path, body, query))
        if not self.responses:
            return {}
        return self.responses.pop(0)


@pytest.fixture
def fake(monkeypatch):
    def install(*responses):
        f = FakeCall(*responses)
        monkeypatch.setattr(_http, "call", f)
        return f
    return install


MEDS = {"data": [
    {"id": "m1", "brand_name": "Augmentin", "active_ingredient": "amoxicillin/clavulanate",
     "strength": "1g", "dosage_form": "tablet", "contraindication_tags": ["penicillin"],
     "rx_required": True, "controlled": False, "unit_price_cents": 1200},
    {"id": "m2", "brand_name": "Augmentin", "active_ingredient": "amoxicillin/clavulanate",
     "strength": "625mg", "dosage_form": "syrup", "contraindication_tags": [],
     "rx_required": True, "controlled": False, "unit_price_cents": 900},
]}


def test_search_medications_strips_sensitive_fields_and_returns_safe_shape(fake):
    f = fake(MEDS)
    out = catalogue.search_medications("augmentin")
    assert f.calls == [("GET", "/api/v1/medications", None, {"q": "augmentin", "limit": 20})]
    assert len(out["matches"]) == 2
    assert out["count"] == 2
    for m in out["matches"]:
        assert set(m) == {"medication_id", "brand_name", "active_ingredient", "strength", "dosage_form"}
    dumped = json.dumps(out)
    for banned in ("contraindication", "rx_required", "controlled", "unit_price", "price"):
        assert banned not in dumped


def test_search_medications_filters_by_form(fake):
    fake(MEDS)
    out = catalogue.search_medications("augmentin", form="syrup")
    assert [m["medication_id"] for m in out["matches"]] == ["m2"]
    assert out["count"] == 1


def test_search_medications_caps_at_five(fake):
    many = {"data": [{"id": f"m{i}", "brand_name": "X", "active_ingredient": "x", "strength": "1",
                      "dosage_form": "tablet"} for i in range(12)]}
    fake(many)
    out = catalogue.search_medications("x")
    assert len(out["matches"]) == 5


def test_search_medications_empty_returns_hint(fake):
    fake({"data": []})
    out = catalogue.search_medications("zzzz")
    assert out == {
        "matches": [],
        "hint": "No catalogue match. Ask the requester with ask_requester; never invent a medicine.",
    }


def test_search_medications_form_filter_with_no_hits_returns_hint(fake):
    fake(MEDS)
    out = catalogue.search_medications("augmentin", form="cream")
    assert out["matches"] == []
    assert "hint" in out


def test_search_medications_backend_error_is_passed_through(fake):
    fake({"error": {"code": "bad_request", "message": "q is too short", "request_id": "r1"}})
    assert catalogue.search_medications("a") == {"error": "q is too short", "code": "bad_request"}


DETAIL = {
    "id": "E1",
    "status": "waiting",
    "raw_request": "need augmentin 1g tabs x2",
    "quantity": 2,
    "medication_id": "m1",
    "medication": {"brand_name": "Augmentin", "strength": "1g", "dosage_form": "tablet"},
    "clarification_question": "",
    "clarification_answer": "",
    "outcome": "",
    "targets": [
        {"id": "T1", "branch_id": "B", "branch_name": "Pharmacy B", "status": "available",
         "reply_quantity": 2, "reply_text": "x" * 2000, "follow_ups": 0},
        {"id": "T2", "branch_id": "C", "branch_name": "Pharmacy C", "status": "alternative_offered",
         "reply_quantity": None, "reply_text": "625mg only", "follow_ups": 0},
        {"id": "T3", "branch_id": "D", "branch_name": "Pharmacy D", "status": "sent",
         "reply_quantity": None, "reply_text": "", "follow_ups": 1},
    ] + [
        {"id": f"TX{i}", "branch_id": f"BX{i}", "branch_name": f"Pharmacy X{i}", "status": "sent",
         "reply_quantity": None, "reply_text": "", "follow_ups": 0}
        for i in range(6)
    ],
    "messages": [
        {"id": "g1", "target_id": "T1", "kind": "enquiry", "status": "sent", "created_at": "2026-11-14T10:00:00Z"},
        {"id": "g2", "target_id": "T3", "kind": "enquiry", "status": "sent", "created_at": "2026-11-14T10:00:00Z"},
        {"id": "g3", "target_id": "T3", "kind": "follow_up", "status": "sent", "created_at": "2026-11-14T11:00:00Z"},
        {"id": "g4", "target_id": None, "kind": "notification", "status": "sent", "created_at": "2026-11-14T11:05:00Z"},
    ],
}


def test_get_enquiry_compacts_detail(fake):
    f = fake(DETAIL)
    out = enquiry.get_enquiry("E1")
    assert f.calls[0][:2] == ("GET", "/api/v1/enquiries/E1")
    assert out["enquiry_id"] == "E1"
    assert out["medication"] == "Augmentin 1g tablet"
    assert out["clarification"] is None
    assert out["notified_requester"] == 1
    by_id = {t["target_id"]: t for t in out["targets"]}
    assert by_id["T1"]["pharmacy"] == "Pharmacy B"
    assert by_id["T1"]["message"] == "sent"
    assert by_id["T3"]["message"] == "sent"
    assert by_id["T2"]["message"] is None
    assert len(by_id["T1"]["reply_text"]) <= 303
    assert len(json.dumps(out)) < 2000


def test_get_enquiry_keeps_clarification(fake):
    d = dict(DETAIL, clarification_question="Which strength?", clarification_answer="1g")
    fake(d)
    out = enquiry.get_enquiry("E1")
    assert out["clarification"] == {"question": "Which strength?", "answer": "1g"}


def test_get_enquiry_stays_under_limit_with_many_targets(fake):
    targets = [{"id": f"T{i}", "branch_name": f"Pharmacy {i}" * 5, "status": "no_response",
                "reply_quantity": None, "reply_text": "y" * 1000, "follow_ups": 1} for i in range(20)]
    fake(dict(DETAIL, targets=targets, messages=[], raw_request="z" * 3000))
    out = enquiry.get_enquiry("E1")
    assert len(json.dumps(out)) < 2000
    assert len(out["raw_request"]) <= 303
    assert out["targets_omitted"] > 0
    assert len(out["targets"]) + out["targets_omitted"] == 20


def test_propose_targets_posts_branch_ids(fake):
    f = fake({"proposed": [{"target_id": "T9", "branch_id": "B", "status": "proposed"}]})
    out = enquiry.propose_targets("E1", ["B"])
    assert f.calls == [("POST", "/api/v1/enquiries/E1/targets", {"branch_ids": ["B"]}, None)]
    assert out["proposed"] == [{"target_id": "T9", "branch_id": "B", "status": "proposed"}]
    assert out["note"].startswith("Stop now") and "Do not call wait()" in out["note"]


def test_close_enquiry_passes_409_message_through(fake):
    f = fake({}, {"error": {"code": "conflict", "message": "Close as available needs one target with status available.",
                            "request_id": "r2"}})  # first answer: close_enquiry reads the state (no unread replies)
    out = enquiry.close_enquiry("E1", outcome="available")
    assert f.calls[0][:2] == ("GET", "/api/v1/enquiries/E1")
    assert f.calls[1][:3] == ("POST", "/api/v1/enquiries/E1/close", {"outcome": "available", "branch_id": None})
    assert out == {"error": "Close as available needs one target with status available.", "code": "conflict"}


def test_set_medication_posts_medication_id(fake):
    f = fake({})
    enquiry.set_medication("E1", "m1")
    assert f.calls[0][:3] == ("POST", "/api/v1/enquiries/E1/medication", {"medication_id": "m1"})


def test_ask_requester_returns_run_ends_note(fake):
    f = fake({"status": "awaiting_clarification"})
    out = enquiry.ask_requester("E1", "Which strength?")
    assert f.calls[0][:3] == ("POST", "/api/v1/enquiries/E1/clarification-request", {"question": "Which strength?"})
    assert out["ok"] is True
    assert out["status"] == "awaiting_clarification"
    assert out["note"].startswith("STOP")


def test_find_candidates_clamps_limit_and_maps_fields(fake):
    f = fake({"candidates": [{"branch_id": "B", "name": "Pharmacy B", "distance_km": 1.2,
                              "has_reported_stock": True, "reported_quantity": 4}]})
    out = enquiry.find_candidates("E1", limit=99)
    assert f.calls[0][:2] == ("GET", "/api/v1/enquiries/E1/candidates")
    assert f.calls[0][3] == {"limit": 10}
    assert out["candidates"][0] == {"branch_id": "B", "name": "Pharmacy B", "distance_km": 1.2,
                                    "has_reported_stock": True, "reported_quantity": 4}


def test_send_wait_notify_follow_up_no_response_close_paths(fake):
    f = fake({"queued": 2}, {"status": "waiting"}, {}, {}, {}, {}, {})
    assert enquiry.send_enquiries("E1")["queued"] == 2
    assert enquiry.wait("E1", minutes=15)["ok"] is True
    enquiry.notify_requester("E1", "Pharmacy C offers 625mg only")
    enquiry.follow_up("T3")
    enquiry.mark_no_response("T3")
    enquiry.close_enquiry("E1", outcome="available", branch_id="B")
    # wait() and close_enquiry() read the state first (GET); notify returns the fresh next_step (GET).
    posts = [c for c in f.calls if c[0] == "POST"]
    assert [c[1] for c in posts] == [
        "/api/v1/enquiries/E1/send",
        "/api/v1/enquiries/E1/wait",
        "/api/v1/enquiries/E1/notify",
        "/api/v1/targets/T3/follow-up",
        "/api/v1/targets/T3/no-response",
        "/api/v1/enquiries/E1/close",
    ]
    assert posts[1][2] == {"minutes": 15}
    assert posts[2][2] == {"body": "Pharmacy C offers 625mg only"}
    assert posts[5][2] == {"outcome": "available", "branch_id": "B"}


def test_backend_unreachable_maps_url_error(monkeypatch):
    def boom(*args, **kwargs):
        raise urllib.error.URLError("connection refused")
    monkeypatch.setattr(urllib.request, "urlopen", boom)
    out = _http.call("GET", "/api/v1/medications")
    assert out["error"]["code"] == "backend_unreachable"
    assert "connection refused" in out["error"]["message"]


def test_http_error_returns_backend_error_body(monkeypatch):
    body = json.dumps({"error": {"code": "agent_forbidden", "message": "Agents cannot reply.", "request_id": "r3"}}).encode()

    def boom(req, timeout=None):
        raise urllib.error.HTTPError(req.full_url, 403, "Forbidden", {}, io.BytesIO(body))
    monkeypatch.setattr(urllib.request, "urlopen", boom)
    out = _http.call("POST", "/api/v1/targets/T1/reply", body={"kind": "available"})
    assert out == {"error": {"code": "agent_forbidden", "message": "Agents cannot reply.", "request_id": "r3"}}


def test_http_error_without_json_body_maps_to_http_status(monkeypatch):
    def boom(req, timeout=None):
        raise urllib.error.HTTPError(req.full_url, 502, "Bad Gateway", {}, io.BytesIO(b"upstream down"))
    monkeypatch.setattr(urllib.request, "urlopen", boom)
    out = _http.call("GET", "/api/v1/medications")
    assert out == {"error": {"code": "http_502", "message": "upstream down"}}


def test_request_sends_agent_authorization_header(monkeypatch):
    seen = {}

    class Resp:
        def __enter__(self):
            return self

        def __exit__(self, *a):
            return False

        def read(self):
            return b""

    def fake_open(req, timeout=None):
        seen["auth"] = req.get_header("Authorization")
        seen["timeout"] = timeout
        seen["url"] = req.full_url
        return Resp()
    monkeypatch.setattr(urllib.request, "urlopen", fake_open)
    monkeypatch.setenv("AGENT_TOKEN", "test-token")
    monkeypatch.setenv("BACKEND_URL", "http://backend.test")
    assert _http.call("GET", "/api/v1/medications", query={"q": "a", "limit": 20}) == {}
    assert seen["auth"] == "Agent test-token"
    assert seen["timeout"] == 15
    assert seen["url"] == "http://backend.test/api/v1/medications?q=a&limit=20"


def test_no_reply_approve_or_mark_available_tools():
    for module in (catalogue, enquiry):
        names = [v.name for v in vars(module).values() if isinstance(v, Tool)]
        assert names, f"{module.__name__} defines no tools"
        for name in names:
            assert not any(word in name for word in ("reply", "approve", "mark_available")), name
        for attr in dir(module):
            assert not any(word in attr for word in ("reply", "approve", "mark_available")), attr


def test_all_twelve_tools_are_defined():
    names = {v.name for module in (catalogue, enquiry) for v in vars(module).values() if isinstance(v, Tool)}
    assert names == {
        "search_medications", "get_enquiry", "set_medication", "ask_requester", "find_candidates",
        "propose_targets", "send_enquiries", "follow_up", "mark_no_response", "notify_requester",
        "wait", "close_enquiry",
    }


def test_inventory_shield_policy_flags_and_fefo_order():
    batches = [
        {"id": "b-recalled", "batch_number": "REC-1", "expiry_date": "2027-01-15T00:00:00Z",
         "quantity_on_hand": 10, "quantity_reserved": 0, "is_recalled": True},
        {"id": "b-expired", "batch_number": "EXP-1", "expiry_date": "2025-12-01T00:00:00Z",
         "quantity_on_hand": 10, "quantity_reserved": 0, "is_recalled": False},
        {"id": "b-later", "batch_number": "AUG-2801", "expiry_date": "2028-01-31T00:00:00Z",
         "quantity_on_hand": 2, "quantity_reserved": 0, "is_recalled": False},
        {"id": "b-sooner", "batch_number": "AUG-2704", "expiry_date": "2027-04-30T00:00:00Z",
         "quantity_on_hand": 5, "quantity_reserved": 1, "is_recalled": False},
    ]
    shield = catalogue.evaluate_inventory_shield(batches, active_refills=4, requested_qty=3, chronic_eligible=True)
    assert [b["batch_number"] for b in shield["fefo_batches"]] == ["AUG-2704", "AUG-2801"]
    assert shield["total_available"] == 6
    assert shield["chronic_reserved"] == 4
    assert shield["surplus_for_peers"] == 2
    assert set(shield["policy_flags"]) == {
        "RARE_PROTECT",
        "CHRONIC_BUFFER",
        "EXCHANGE_ONLY",
        "NEAR_EXPIRY_PUSH",
    }
    assert len(json.dumps(shield)) < 2000


def test_normalize_intake_query_extracts_franco_tokens_and_form():
    tokens, form = catalogue.normalize_intake_query("badde augmentin 1g 2 3elab habb")
    assert tokens == ["augmentin"]
    assert form == "tablet"
