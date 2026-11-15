import json

import pytest

import _http
import reference


class FakeCall:
    """Replaces _http.call. Records every request and returns canned answers in order."""

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


def row(code, strength, extra=None):
    r = {"code": code, "reg_no": "DEMO-0001", "brand_name": "Augmentin", "ingredient": "Amoxicillin/Clavulanate",
         "strength": strength, "dosage_form": "tablet", "official_price_cents": 1200,
         "source": "Demo seed, not real MoPH data", "document_id": None, "created_at": "2026-11-15T00:00:00Z"}
    r.update(extra or {})
    return r


def test_check_reference_urlencodes_params_and_returns_compact_rows(fake):
    f = fake({"data": [row("6280000000001", "1000mg")], "limit": 5, "offset": 0})
    out = reference.check_reference.function("Amoxicillin/Clavulanate", "1000mg", "tablet")
    assert f.calls == [(
        "GET",
        "/api/v1/reference/products?ingredient=Amoxicillin%2FClavulanate&limit=5&strength=1000mg&form=tablet",
        None,
        None,
    )]
    assert out["products"] == [{"code": "6280000000001", "brand_name": "Augmentin", "strength": "1000mg",
                                "dosage_form": "tablet", "official_price_cents": 1200}]
    assert out["note"] == "Information only; the Go rules decide equivalence."
    assert out["count"] == 1


def test_check_reference_optional_filters_left_out(fake):
    f = fake({"data": []})
    out = reference.check_reference.function("Ibuprofen")
    assert f.calls[0][1] == "/api/v1/reference/products?ingredient=Ibuprofen&limit=5"
    assert out["products"] == [] and "hint" in out


def test_check_reference_caps_at_five_and_stays_small(fake):
    fake({"data": [row(f"62800000000{i:02d}", f"{i}mg") for i in range(12)]})
    out = reference.check_reference.function("Amoxicillin/Clavulanate")
    assert len(out["products"]) == 5
    assert len(json.dumps(out)) < 2000
    for p in out["products"]:
        assert set(p) == {"code", "brand_name", "strength", "dosage_form", "official_price_cents"}


def test_check_reference_passes_backend_error_through(fake):
    fake({"error": {"code": "validation_failed", "message": "search filters must be at most 200 characters"}})
    out = reference.check_reference.function("x" * 300)
    assert out == {"error": "search filters must be at most 200 characters", "code": "validation_failed"}


def test_check_reference_requires_ingredient(fake):
    f = fake()
    out = reference.check_reference.function("  ")
    assert out["code"] == "validation_failed"
    assert f.calls == []


def test_check_reference_is_a_cornilia_tool():
    from cornilia.tools import Tool

    assert isinstance(reference.check_reference, Tool)
    assert reference.check_reference.name == "check_reference"
