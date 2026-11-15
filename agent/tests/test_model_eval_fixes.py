"""Behaviours added after the real-model eval (scripts/eval_model.py): the tools do the work, not the prompt."""
import pytest

import _http
import enquiry
import offers


class Backend:
    """Fake _http.call keyed by (method, path suffix); records every call."""

    def __init__(self, routes):
        self.routes = routes
        self.calls = []

    def __call__(self, method, path, body=None, query=None):
        self.calls.append((method, path, body))
        for (m, suffix), answer in self.routes.items():
            if m == method and path.endswith(suffix):
                return answer(body) if callable(answer) else answer
        return {}


@pytest.fixture
def backend(monkeypatch):
    def install(routes):
        b = Backend(routes)
        monkeypatch.setattr(_http, "call", b)
        return b
    return install


def _detail(status="waiting", targets=(), events=(), next_wake_at=None, desk=False, messages=(), photos=None):
    d = {"enquiry": {"id": "e1", "status": status, "raw_request": "Augmentin 1000mg", "quantity": 1,
                     "next_wake_at": next_wake_at,
                     "requester_branch_id": "branch-client-desk" if desk else "branch-hamra"},
         "medication": {"brand_name": "Augmentin", "strength": "1000mg", "dosage_form": "tablet"},
         "targets": list(targets), "messages": list(messages), "events": list(events)}
    if photos is not None:
        d["photos"] = photos
    return d


def _t(tid, status, text="", branch="Baabda", follow_ups=0):
    return {"id": tid, "branch_id": "branch-" + branch.lower(), "branch_name": branch, "status": status,
            "reply_text": text, "follow_ups": follow_ups}


# ---------------------------------------------------------------- wait()

@pytest.mark.parametrize("status", ["awaiting_approval", "awaiting_clarification"])
def test_wait_while_a_human_must_answer_sets_no_timer_and_ends_the_run(backend, status):
    b = backend({("GET", "/enquiries/E1"): {"enquiry": {"id": "E1", "status": status}}})
    out = enquiry.wait.function("E1", minutes=5)
    assert out["ok"] is True and out["skipped"] is True and "STOP" in out["note"]
    assert [c[0] for c in b.calls] == ["GET"]  # never POST /wait


def test_wait_when_pharmacies_are_pending_sets_the_timer(backend):
    b = backend({("GET", "/enquiries/E1"): {"enquiry": {"id": "E1", "status": "waiting"}},
                 ("POST", "/wait"): {"status": "waiting", "next_wake_at": "t"}})
    assert enquiry.wait.function("E1", minutes=5)["next_wake_at"] == "t"
    assert b.calls[-1][:2] == ("POST", "/api/v1/enquiries/E1/wait")


# ---------------------------------------------------------------- close_enquiry()

def test_close_refuses_while_a_text_reply_is_unread(backend):
    detail = _detail(targets=[_t("t1", "available", "eh 3ande 2 3elab"), _t("t2", "unavailable", "la2", "Verdun")])
    b = backend({("GET", "/enquiries/e1"): detail, ("GET", "/offers"): {"data": [{"target_id": "t2", "decision": "rejected"}]}})
    out = enquiry.close_enquiry.function("e1", outcome="available", branch_id="branch-baabda")
    assert out["code"] == "replies_unread" and "record_offer for target t1" in out["error"]
    assert not any(c[0] == "POST" for c in b.calls)


def test_close_goes_through_when_every_reply_is_read(backend):
    detail = _detail(targets=[_t("t1", "available", "eh 3ande")])
    b = backend({("GET", "/enquiries/e1"): detail, ("GET", "/offers"): {"data": [{"target_id": "t1", "decision": "matches"}]},
                 ("POST", "/close"): {"status": "closed"}})
    assert enquiry.close_enquiry.function("e1", outcome="available", branch_id="branch-baabda")["ok"] is True
    assert b.calls[-1][:2] == ("POST", "/api/v1/enquiries/e1/close")


# ---------------------------------------------------------------- next_step

def test_escalated_available_reading_is_reported_not_closed():
    out = enquiry._compact_enquiry(_detail(targets=[_t("t1", "available", "fi bas lazem wasfe")]), {"t1": "escalated"})
    assert "notify_requester naming Baabda" in out["next_step"] and "NOT a match" in out["next_step"]
    assert "outcome 'available'" not in out["next_step"]


def test_escalated_already_notified_moves_on_to_close():
    detail = _detail(targets=[_t("t1", "available", "fi bas lazem wasfe")],
                     messages=[{"kind": "notification", "body": "Baabda has it but asks for a prescription"}])
    assert "unable_to_confirm" in enquiry._compact_enquiry(detail, {"t1": "escalated"})["next_step"]


def test_chase_after_follow_up_and_fired_wait_marks_no_response_now():
    events = [{"seq": 5, "type": "follow_up_queued"}, {"seq": 7, "type": "wait_set"}]
    out = enquiry._compact_enquiry(_detail(targets=[_t("t1", "sent", follow_ups=1)], events=events))
    assert out["next_step"].startswith("Followed up and waited with no reply: mark_no_response t1")
    assert "_waited_after_follow_up" not in out


def test_chase_after_follow_up_without_wait_waits():
    events = [{"seq": 7, "type": "wait_set"}, {"seq": 9, "type": "follow_up_queued"}]
    out = enquiry._compact_enquiry(_detail(targets=[_t("t1", "sent", follow_ups=1)], events=events))
    assert out["next_step"] == "Followed up already, but no wait since: call wait(5) now."
    pending_timer = _detail(targets=[_t("t1", "sent", follow_ups=1)], next_wake_at="2099-01-01T00:00:00Z",
                            events=[{"seq": 5, "type": "follow_up_queued"}, {"seq": 7, "type": "wait_set"}])
    assert "wait(5)" in enquiry._compact_enquiry(pending_timer)["next_step"]


def test_client_desk_never_told_to_close_available_even_with_a_verified_offer():
    out = enquiry._compact_enquiry(_detail(desk=True, targets=[_t("t1", "available", "3ande 500mg")]), {"t1": "matches"})
    assert "close_enquiry outcome 'available'" not in out["next_step"]
    assert "notify_requester on the client ticket naming Baabda" in out["next_step"] and "wait(15)" in out["next_step"]


def test_client_desk_after_notifying_waits_instead_of_closing():
    detail = _detail(desk=True, targets=[_t("t1", "available", "3ande")],
                     messages=[{"kind": "notification", "body": "Baabda has it"}])
    for decision in ("matches", None):
        step = enquiry._compact_enquiry(detail, {"t1": decision} if decision else {})["next_step"]
        assert "wait(15)" in step and "unable_to_confirm" not in step


def test_photo_without_agent_reading_is_unread_whatever_its_status():
    photos = [{"id": "p1", "status": "flagged", "read_at": None, "agent_code": ""},
              {"id": "p2", "status": "flagged", "read_at": "2026-11-15T10:00:00Z", "agent_code": "628"}]
    out = enquiry._compact_enquiry(_detail(desk=True, targets=[_t("t1", "available", "")], photos=photos))
    assert out["photos_to_read"] == ["p1"]


def test_wake_photo_id_is_read_even_after_wake_trigger_was_cleared():
    detail = _detail(desk=True, targets=[_t("t1", "available", "")])
    detail["wake_photo_id"] = "p9"
    assert enquiry._compact_enquiry(detail)["next_step"].startswith("read_package_photo(photo_id) for photo p9")


# ---------------------------------------------------------------- chaining

def test_follow_up_returns_the_fresh_next_step(backend):
    detail = _detail(targets=[_t("t1", "sent", follow_ups=1), _t("t2", "sent", "", "Verdun")])
    detail["messages"] = [{"target_id": "t1", "kind": "enquiry", "status": "sent"},
                          {"target_id": "t2", "kind": "enquiry", "status": "sent"}]
    backend({("POST", "/follow-up"): {"status": "queued", "enquiry_id": "e1"}, ("GET", "/enquiries/e1"): detail})
    out = enquiry.follow_up.function("t1")
    assert out["next_step"].startswith("Still silent: follow_up t2")


# ---------------------------------------------------------------- record_offer()

def test_record_offer_maps_alias_keys_and_drops_non_field_quotes(backend):
    b = backend({("POST", "/offers"): {"id": "o1", "decision": "matches", "reasons": []}})
    out = offers.record_offer.function(
        target_id="T1", confidence=0.9, in_stock=True, quantity=2, price_cents=1200, currency="USD",
        evidence={"in_stock": "eh 3ande", "quantity": "2 3elab", "price_cents": "12$", "currency": "USD", "expiry": "08/2027"})
    body = b.calls[0][2]
    assert body["evidence"] == {"in_stock": "eh 3ande", "quantity": "2 3elab", "price": "12$"}
    assert out["decision"] == "matches" and sorted(out["ignored_evidence"]) == ["currency", "expiry"]


def test_record_offer_evidence_without_fields_teaches_before_posting(backend):
    b = backend({})
    out = offers.record_offer.function(target_id="T1", confidence=0.9, evidence={"in_stock": "eh 3ande"})
    assert out["code"] == "validation_failed" and "set no field" in out["error"] and "in_stock=true" in out["error"]
    assert b.calls == []


def test_record_offer_field_without_quote_is_refused_not_invented(backend):
    b = backend({})
    out = offers.record_offer.function(target_id="T1", confidence=0.9, in_stock=True, strength="1g",
                                       evidence={"in_stock": "eh 3ande"})
    assert "strength set without a quote" in out["error"] and "Never copy" in out["error"]
    assert b.calls == []


def test_record_offer_quantity_must_be_written_in_the_quote(backend):
    b = backend({("POST", "/offers"): {"id": "o1", "decision": "matches"}})
    out = offers.record_offer.function(target_id="T1", confidence=0.9, in_stock=True, quantity=2,
                                       evidence={"in_stock": "متوفر", "quantity": "علبتين"})
    assert "does not write that number" in out["error"] and b.calls == []
    offers.record_offer.function(target_id="T1", confidence=0.9, in_stock=True, quantity=2,
                                 evidence={"in_stock": "متوفر", "quantity": "٢ علب"})  # Arabic-Indic digits count
    assert b.calls and b.calls[0][2]["quantity"] == 2


def test_record_offer_price_must_be_as_written_and_currency_is_read_from_the_quote(backend):
    b = backend({("POST", "/offers"): {"id": "o1", "decision": "matches"}})
    out = offers.record_offer.function(target_id="T1", confidence=0.9, in_stock=True, price_cents=2400,
                                       evidence={"in_stock": "eh 3ande", "price": "12$ el 3elbe"})
    assert "do not multiply" in out["error"] and b.calls == []
    offers.record_offer.function(target_id="T1", confidence=0.9, in_stock=True, price_cents=800,
                                 evidence={"in_stock": "متوفر", "price": "٨ دولار"})
    assert b.calls[0][2]["currency"] == "USD" and b.calls[0][2]["price_cents"] == 800
    out = offers.record_offer.function(target_id="T1", confidence=0.9, in_stock=True, price_cents=900,
                                       evidence={"in_stock": "fi", "price": "9"})
    assert "currency is required" in out["error"]


def test_record_offer_returns_next_step_when_the_backend_names_the_enquiry(backend):
    detail = _detail(targets=[_t("t1", "unavailable", "la2 ma fi")])
    backend({("POST", "/offers"): {"id": "o1", "decision": "rejected", "enquiry_id": "e1"},
             ("GET", "/enquiries/e1"): detail, ("GET", "/offers"): {"data": [{"target_id": "t1", "decision": "rejected"}]}})
    out = offers.record_offer.function(target_id="t1", confidence=1, in_stock=False, evidence={"in_stock": "la2 ma fi"})
    assert out["next_step"] == "Every pharmacy replied unavailable: close_enquiry outcome 'unavailable'."
