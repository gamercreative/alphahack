import pytest

import _http
import offers


class FakeCall:
    """Replaces _http.call: records requests, returns canned answers in order."""

    def __init__(self, *responses):
        self.responses = list(responses)
        self.calls = []

    def __call__(self, method, path, body=None, query=None):
        self.calls.append((method, path, body, query))
        return self.responses.pop(0) if self.responses else {}


@pytest.fixture
def fake(monkeypatch):
    def install(*responses):
        f = FakeCall(*responses)
        monkeypatch.setattr(_http, "call", f)
        return f
    return install


def test_record_offer_posts_fields_and_returns_small_verdict(fake):
    f = fake({"id": "o1", "decision": "escalated", "decided_by": "rules",
              "reasons": ["strength 625mg differs from the requested 1000mg; a substitution needs prescriber approval"],
              "source_text": "3ande bas 625, ta3e abel l 6, cash"})
    out = offers.record_offer.function(
        target_id="T 1", evidence={"in_stock": "3ande", "strength": "625", "payment": "cash", "pickup_by": ""},
        confidence=0.85, in_stock=True, strength="625mg", payment="cash",
    )
    method, path, body, query = f.calls[0]
    assert (method, path, query) == ("POST", "/api/v1/targets/T%201/offers", None)
    assert body["evidence"] == {"in_stock": "3ande", "strength": "625", "payment": "cash"}  # empty quotes dropped
    assert body["confidence"] == 0.85 and body["in_stock"] is True and body["strength"] == "625mg"
    assert body["quantity"] is None and body["needs_prescription"] is None
    assert body["currency"] == "" and body["pickup_by"] == ""
    assert out["offer_id"] == "o1" and out["decision"] == "escalated"
    assert "prescriber approval" in out["reasons"][0]
    assert "source_text" not in out  # result stays small


def test_record_offer_passes_backend_error_to_the_model(fake):
    fake({"error": {"code": "conflict", "message": "this pharmacy has not replied yet; offers are read only from a pharmacist's reply"}})
    out = offers.record_offer.function(target_id="T1", evidence={"in_stock": "fi"}, confidence=0.9, in_stock=True)
    assert out == {"error": "this pharmacy has not replied yet; offers are read only from a pharmacist's reply", "code": "conflict"}


def test_record_offer_caps_reasons(fake):
    fake({"id": "o2", "decision": "escalated", "reasons": [f"r{i}" * 300 for i in range(10)]})
    out = offers.record_offer.function(target_id="T1", evidence={"payment": "cash"}, confidence=0.5, payment="cash")
    assert len(out["reasons"]) == 6 and all(len(r) <= 200 for r in out["reasons"])


def test_record_offer_schema_requires_target_evidence_confidence():
    params = offers.record_offer.get_schema()["function"]["parameters"]
    assert params["required"] == ["target_id", "evidence", "confidence"]
    assert params["properties"]["evidence"]["type"] == "object"
    assert params["properties"]["payment"]["enum"] == ["cash", "card", "any"]


def test_record_offer_sends_numeric_quotes_as_text(fake):
    f = fake({"id": "o3", "decision": "matches", "reasons": []})
    offers.record_offer.function(target_id="T1", evidence={"quantity": 2, "in_stock": "fi", "price": None, "payment": "  "},
                                 confidence=0.9, in_stock=True, quantity=2)
    body = f.calls[0][2]
    assert body["evidence"] == {"quantity": "2", "in_stock": "fi"}


def test_record_offer_rejects_non_object_evidence_without_calling(fake):
    f = fake()
    out = offers.record_offer.function(target_id="T1", evidence="3ande", confidence=0.9, in_stock=True)
    assert out["code"] == "validation_failed" and "evidence must be an object" in out["error"]
    assert f.calls == []


def test_record_offer_never_sends_a_decision(fake):
    # The rules decide; the tool has no way to pass decision or decided_by.
    f = fake({"id": "o4", "decision": "escalated", "reasons": ["x"]})
    params = offers.record_offer.get_schema()["function"]["parameters"]["properties"]
    assert "decision" not in params and "decided_by" not in params
    offers.record_offer.function(target_id="T1", evidence={"in_stock": "fi"}, confidence=0.9, in_stock=True)
    assert "decision" not in f.calls[0][2] and "decided_by" not in f.calls[0][2]
