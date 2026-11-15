import sys
from pathlib import Path

AGENT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AGENT_DIR))

import pytest  # noqa: E402
import scripted  # noqa: E402

TERMINAL = {"wait", "close_enquiry", "ask_requester"}

# the arguments that matter for each tool; enquiry_id is always "enq-1" and is left out
KEY_ARGS = {
    "search_medications": ("query",),
    "set_medication": ("medication_id",),
    "ask_requester": (),
    "find_candidates": ("limit",),
    "propose_targets": ("branch_ids",),
    "send_enquiries": (),
    "follow_up": ("target_id",),
    "mark_no_response": ("target_id",),
    "notify_requester": (),
    "wait": ("minutes",),
    "close_enquiry": ("outcome", "branch_id"),
}


class Recorder:
    """Stands in for the tool-call function. Records each call and returns canned results by tool name."""

    def __init__(self, results=None):
        self.results = results or {}
        self.calls = []

    def __call__(self, name, **kwargs):
        self.calls.append((name, kwargs))
        return self.results.get(name, {})

    @property
    def names(self):
        return [name for name, _ in self.calls]

    def kwargs_of(self, name):
        return [kw for n, kw in self.calls if n == name]


def summary(rec):
    """The exact ordered list of (tool name, key arguments) that were recorded."""
    out = []
    for name, kwargs in rec.calls:
        out.append((name, {k: kwargs[k] for k in KEY_ARGS[name] if k in kwargs}))
    return out


def match(medication_id, strength, form="tablet"):
    return {"medication_id": medication_id, "brand_name": "Augmentin", "strength": strength, "dosage_form": form}


AUGMENTIN_MATCHES = {"matches": [match("med-1000", "1000mg"), match("med-625", "625mg")], "count": 2}


def target(target_id, branch_id, name, status, follow_ups=0, reply_text=""):
    return {
        "id": target_id,
        "branch_id": branch_id,
        "branch_name": name,
        "status": status,
        "follow_ups": follow_ups,
        "reply_text": reply_text,
    }


def msg(target_id, kind, status="sent", body=""):
    return {"id": f"m-{target_id}-{kind}", "target_id": target_id, "kind": kind, "status": status, "body": body}


def state(raw="need augmentin", status="open", medication=True, targets=(), messages=(),
          question="", answer=""):
    return {
        "enquiry": {
            "id": "enq-1",
            "status": status,
            "raw_request": raw,
            "quantity": 2,
            "clarification_question": question,
            "clarification_answer": answer,
        },
        "medication": {"id": "med-1000", "brand_name": "Augmentin", "strength": "1000mg"} if medication else None,
        "targets": list(targets),
        "messages": list(messages),
        "events": [],
        "runs": [],
        "hitl_count": 0,
    }


# Case builders: each returns (state, canned tool results). Shared by the named tests and the
# terminal-tool test, so the same states are checked in both places.

def case_no_catalogue_match():
    return (state(raw="need augmentin", medication=False),
            {"search_medications": {"matches": [], "count": 0}})


def case_search_error():
    return (state(raw="need augmentin", medication=False),
            {"search_medications": {"error": "catalogue unavailable", "code": "internal"}})


def case_two_strengths_no_strength():
    return (state(raw="need augmentin", medication=False),
            {"search_medications": AUGMENTIN_MATCHES})


def case_strength_not_in_catalogue():
    return (state(raw="augmentin 2g", medication=False),
            {"search_medications": AUGMENTIN_MATCHES})


def case_pending_clarification():
    return (state(raw="need augmentin", medication=False, question="Which strength?", answer=""),
            {"search_medications": AUGMENTIN_MATCHES})


def case_no_candidates():
    return (state(targets=[]), {"find_candidates": {"candidates": []}})


def case_candidates_error():
    return (state(targets=[]), {"find_candidates": {"error": "backend down", "code": "internal"}})


def case_five_candidates():
    return (state(targets=[]), {"find_candidates": {"candidates": [
        {"branch_id": "b1", "name": "A"}, {"branch_id": "b2", "name": "B"},
        {"branch_id": "b3", "name": "C"}, {"branch_id": "b4", "name": "D"},
        {"branch_id": "b5", "name": "E"},
    ]}})


def case_proposed_waiting():
    return (state(targets=[target("t1", "b1", "A", "proposed")]), {})


def case_alternative_notified_and_silent():
    return (state(
        targets=[
            target("tc", "b3", "Pharmacy C", "alternative_offered", reply_text="625mg only"),
            target("td", "b4", "Pharmacy D", "no_response", follow_ups=1),
        ],
        messages=[msg("tc", "notification",
                      body="Pharmacy C offers an alternative: 625mg only. Not a match; tell us if acceptable.")],
    ), {})


def case_all_no_response():
    return (state(targets=[
        target("t1", "b1", "A", "no_response", follow_ups=1),
        target("t2", "b2", "B", "no_response", follow_ups=1),
    ]), {})


def case_available_beats_everything():
    return (state(targets=[
        target("t1", "b1", "A", "available", reply_text="2 in stock"),
        target("t2", "b2", "B", "unavailable"),
        target("t3", "b3", "C", "sent"),
    ]), {})


def case_alternative_and_unavailable():
    return (state(targets=[
        target("tc", "b3", "Pharmacy C", "alternative_offered", reply_text="625mg only"),
        target("tb", "b2", "Pharmacy B", "unavailable"),
    ]), {})


# 1
def test_closed_enquiry_makes_no_calls():
    rec = Recorder()
    scripted.decide(state(status="closed", targets=[target("t1", "b1", "A", "sent")]), rec)
    assert summary(rec) == []


# 2
def test_cancelled_enquiry_makes_no_calls():
    rec = Recorder()
    scripted.decide(state(status="cancelled", targets=[target("t1", "b1", "A", "sent")]), rec)
    assert summary(rec) == []


# 3
def test_no_catalogue_match_asks_requester():
    st, results = case_no_catalogue_match()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [
        ("search_medications", {"query": "need"}),
        ("search_medications", {"query": "augmentin"}),
        ("ask_requester", {}),
    ]


# 4
def test_search_error_waits():
    st, results = case_search_error()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [
        ("search_medications", {"query": "need"}),
        ("search_medications", {"query": "augmentin"}),
        ("wait", {"minutes": 5}),
    ]


# 5
def test_two_strengths_no_strength_in_request_asks():
    st, results = case_two_strengths_no_strength()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [
        ("search_medications", {"query": "need"}),
        ("search_medications", {"query": "augmentin"}),
        ("ask_requester", {}),
    ]
    question = rec.kwargs_of("ask_requester")[0]["question"]
    assert "1000mg" in question and "625mg" in question


# 6
def test_requested_strength_not_in_catalogue_never_substitutes():
    st, results = case_strength_not_in_catalogue()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [
        ("search_medications", {"query": "augmentin"}),
        ("ask_requester", {}),
    ]


# 7
def test_pending_clarification_makes_no_calls():
    st, results = case_pending_clarification()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == []


# 8
def test_no_candidates_closes_unable_to_confirm():
    st, results = case_no_candidates()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [
        ("find_candidates", {"limit": 5}),
        ("close_enquiry", {"outcome": "unable_to_confirm"}),
    ]


# 9
def test_candidates_error_waits():
    st, results = case_candidates_error()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [
        ("find_candidates", {"limit": 5}),
        ("wait", {"minutes": 5}),
    ]


# 10
def test_proposes_at_most_three():
    st, results = case_five_candidates()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [
        ("find_candidates", {"limit": 5}),
        ("propose_targets", {"branch_ids": ["b1", "b2", "b3"]}),
        ("wait", {"minutes": 5}),
    ]


# 11
def test_proposed_targets_wait_for_human():
    st, results = case_proposed_waiting()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [("wait", {"minutes": 5})]


# 12
def test_alternative_and_silence_closes_unable_to_confirm():
    st, results = case_alternative_notified_and_silent()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [("close_enquiry", {"outcome": "unable_to_confirm"})]


# 13
def test_all_no_response_is_not_unavailable():
    st, results = case_all_no_response()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [("close_enquiry", {"outcome": "unable_to_confirm"})]


# 14
def test_available_beats_everything():
    st, results = case_available_beats_everything()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [("close_enquiry", {"outcome": "available", "branch_id": "b1"})]


# 15
def test_alternative_never_closes_available():
    st, results = case_alternative_and_unavailable()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert summary(rec) == [("notify_requester", {}), ("wait", {"minutes": 5})]
    assert "Pharmacy C" in rec.kwargs_of("notify_requester")[0]["message"]
    assert not [kw for kw in rec.kwargs_of("close_enquiry") if kw.get("outcome") == "available"]


# 16
@pytest.mark.parametrize("case", [
    case_no_catalogue_match,
    case_search_error,
    case_two_strengths_no_strength,
    case_strength_not_in_catalogue,
    # case_pending_clarification is not here on purpose: that state makes no call at all, which
    # test_pending_clarification_makes_no_calls asserts exactly.
    case_no_candidates,
    case_candidates_error,
    case_five_candidates,
    case_proposed_waiting,
    case_alternative_notified_and_silent,
    case_all_no_response,
    case_available_beats_everything,
    case_alternative_and_unavailable,
], ids=lambda f: f.__name__)
def test_every_branch_ends_with_terminal_tool_or_nothing(case):
    st, results = case()
    rec = Recorder(results)
    scripted.decide(st, rec)
    assert rec.names, "no calls recorded; every run must end with wait, close_enquiry or ask_requester"
    assert rec.names[-1] in TERMINAL, f"last call was {rec.names[-1]!r}, calls: {summary(rec)}"
