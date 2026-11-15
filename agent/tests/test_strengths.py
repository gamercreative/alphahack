import sys
from pathlib import Path

import pytest

AGENT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AGENT_DIR))
sys.path.insert(0, str(AGENT_DIR / "tools"))

import _http  # noqa: E402
import catalogue  # noqa: E402
import scripted  # noqa: E402


class KnownBugFailure(AssertionError):
    """Raised only by the pinned bug assertion in an xfail test so setup failures fail loudly."""


def assert_known_bug(condition, message=""):
    if not condition:
        raise KnownBugFailure(message)


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


CATALOGUE_ROWS = {"data": [
    {"id": "med-augmentin-1g", "brand_name": "Augmentin", "active_ingredient": "amoxicillin/clavulanate",
     "strength": "1000mg", "dosage_form": "tablet", "contraindication_tags": [],
     "rx_required": True, "controlled": False, "unit_price_cents": 1200},
    {"id": "med-augmentin-625", "brand_name": "Augmentin", "active_ingredient": "amoxicillin/clavulanate",
     "strength": "625mg", "dosage_form": "tablet", "contraindication_tags": [],
     "rx_required": True, "controlled": False, "unit_price_cents": 900},
]}


def eight_rows():
    rows = []
    for i in range(8):
        rows.append({"id": f"med-augmentin-row-{i}", "brand_name": "Augmentin",
                     "active_ingredient": "amoxicillin/clavulanate", "strength": f"{250 + i}mg",
                     "dosage_form": "tablet"})
    return {"data": rows}


# 1. milligram parsing in catalogue

@pytest.mark.parametrize("text, expected", [
    ("Augmentin 1g", {1000.0}),
    ("1000mg", {1000.0}),
    ("625 mg", {625.0}),
    ("0.5g", {500.0}),
    ("0,5 g", {500.0}),
    ("100mcg", {0.1}),
    ("augmentin", set()),
    ("2 boxes", set()),
    ("500mg/125mg", {500.0, 125.0}),
])
def test_strengths_mg(text, expected):
    assert catalogue._strengths_mg(text) == expected


# 2. one gram is 1000 mg, never 625 mg

def test_one_gram_equals_1000mg_but_not_625():
    assert catalogue._strengths_mg("1g") == catalogue._strengths_mg("1000mg")
    assert catalogue._strengths_mg("1g").isdisjoint(catalogue._strengths_mg("625mg"))


# 3. scripted and catalogue parse strengths the same way

@pytest.mark.parametrize("text", [
    "augmentin 1g",
    "augmentin 1000mg",
    "augmentin 625mg",
    pytest.param('ventolin 100mcg', marks=pytest.mark.xfail(strict=True, raises=KnownBugFailure, reason='BUG-09: scripted._strengths does not convert mcg to mg (100mcg is read as 100mg), catalogue._strengths_mg does')),
    pytest.param('x 0.5g', marks=pytest.mark.xfail(strict=True, raises=KnownBugFailure, reason="BUG-09: scripted's quantity pattern eats 'x 0' of 'x 0.5g', so 0.5g is read as 5g")),
])
def test_scripted_and_catalogue_agree(text):
    assert_known_bug({float(v) for v in scripted._strengths(text)} == catalogue._strengths_mg(text))


# 4. quantity words never become strengths in scripted

@pytest.mark.parametrize("text", [
    "augmentin 1g x2",
    "augmentin 1g 2 boxes",
    pytest.param('badde augmentin 1g 2 3elab', marks=pytest.mark.xfail(strict=True, raises=KnownBugFailure, reason='BUG-09: scripted._strengths counts the bare quantity numbers of the demo phrase (2, 3) as strengths')),
])
def test_scripted_ignores_quantity_words(text):
    assert_known_bug(scripted._strengths(text) == scripted._strengths("augmentin 1g"))


# 5. search_medications strength handling

def test_search_strength_match_picks_exact_strength(fake):
    fake(CATALOGUE_ROWS)
    out = catalogue.search_medications("augmentin 1g")
    assert out["strength_match"] == "med-augmentin-1g"


def test_search_no_strength_in_query_gives_no_strength_match(fake):
    fake(CATALOGUE_ROWS)
    out = catalogue.search_medications("augmentin")
    assert "strength_match" not in out


def test_search_unknown_strength_gives_no_strength_match(fake):
    fake(CATALOGUE_ROWS)
    out = catalogue.search_medications("augmentin 2g")
    assert "strength_match" not in out


def test_search_returns_at_most_five(fake):
    fake(eight_rows())
    out = catalogue.search_medications("augmentin")
    assert len(out["matches"]) == 5
    assert out["count"] == 5


def test_search_form_filter_is_exact(fake):
    fake(CATALOGUE_ROWS)
    out = catalogue.search_medications("augmentin", form="syrup")
    assert out["matches"] == []
