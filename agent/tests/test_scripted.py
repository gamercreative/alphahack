import sys
from pathlib import Path

AGENT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AGENT_DIR))

import scripted  # noqa: E402


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


def test_ambiguous_request_asks_requester_with_strengths():
    rec = Recorder({"search_medications": AUGMENTIN_MATCHES})

    names = scripted.decide(state(raw="need augmentin", medication=False), rec)

    assert names == ["search_medications", "ask_requester"]
    question = rec.kwargs_of("ask_requester")[0]["question"]
    assert "1000mg" in question and "625mg" in question
    assert rec.kwargs_of("search_medications")[0]["query"] == "need augmentin"


def test_one_strength_match_sets_medication_then_proposes_nearest_three():
    rec = Recorder({
        "search_medications": AUGMENTIN_MATCHES,
        "find_candidates": {"candidates": [
            {"branch_id": "b1", "name": "A"}, {"branch_id": "b2", "name": "B"},
            {"branch_id": "b3", "name": "C"}, {"branch_id": "b4", "name": "D"},
        ]},
    })

    names = scripted.decide(state(raw="need augmentin 1g tabs x2", medication=False), rec)

    assert names == ["search_medications", "set_medication", "find_candidates", "propose_targets", "wait"]
    assert rec.kwargs_of("set_medication")[0] == {"enquiry_id": "enq-1", "medication_id": "med-1000"}
    assert rec.kwargs_of("propose_targets")[0]["branch_ids"] == ["b1", "b2", "b3"]
    assert rec.kwargs_of("wait")[0] == {"enquiry_id": "enq-1", "minutes": 5}


def test_clarification_answer_picks_matching_strength():
    rec = Recorder({"search_medications": AUGMENTIN_MATCHES})

    scripted.decide(state(raw="need augmentin", medication=False,
                          question="Which strength?", answer="the 625mg one"), rec)

    assert rec.kwargs_of("set_medication")[0]["medication_id"] == "med-625"


def test_unanswered_clarification_does_nothing():
    rec = Recorder({"search_medications": AUGMENTIN_MATCHES})

    names = scripted.decide(state(raw="need augmentin", medication=False,
                                  question="Which strength?", answer=""), rec)

    assert names == []


def test_closed_enquiry_does_nothing():
    rec = Recorder()
    assert scripted.decide(state(status="closed", targets=[target("t1", "b1", "A", "sent")]), rec) == []
    assert rec.calls == []


def test_approved_targets_are_sent():
    rec = Recorder()
    st = state(status="awaiting_approval", targets=[target("t1", "b1", "A", "approved")])

    names = scripted.decide(st, rec)

    assert names == ["send_enquiries", "wait"]


def test_proposed_targets_wait_for_approval():
    rec = Recorder()
    names = scripted.decide(state(targets=[target("t1", "b1", "A", "proposed")]), rec)
    assert names == ["wait"]


def test_available_reply_closes_available_at_that_branch():
    rec = Recorder()
    st = state(targets=[
        target("t1", "b1", "A", "available", reply_text="2 in stock"),
        target("t2", "b2", "B", "sent"),
    ])

    names = scripted.decide(st, rec)

    assert names == ["close_enquiry"]
    assert rec.kwargs_of("close_enquiry")[0] == {"enquiry_id": "enq-1", "outcome": "available", "branch_id": "b1"}


def test_alternative_is_notified_then_closed_unable_to_confirm():
    # state 1: B unavailable, C offered an alternative (not yet told), D silent after its follow-up
    first = state(targets=[
        target("tb", "b2", "Pharmacy B", "unavailable"),
        target("tc", "b3", "Pharmacy C", "alternative_offered", reply_text="625mg only"),
        target("td", "b4", "Pharmacy D", "no_response", follow_ups=1),
    ])
    rec1 = Recorder()
    names1 = scripted.decide(first, rec1)

    assert names1 == ["notify_requester", "wait"]
    message = rec1.kwargs_of("notify_requester")[0]["message"]
    assert "Pharmacy C" in message and "625mg only" in message
    assert "Not a match" in message

    # state 2: the requester has been told about C, so the enquiry can close
    second = state(
        targets=first["targets"],
        messages=[msg("tc", "notification", body=message)],
    )
    rec2 = Recorder()
    names2 = scripted.decide(second, rec2)

    assert names2 == ["close_enquiry"]
    assert rec2.kwargs_of("close_enquiry")[0] == {
        "enquiry_id": "enq-1", "outcome": "unable_to_confirm",
    }


def test_all_unavailable_closes_unavailable():
    rec = Recorder()
    st = state(targets=[target("t1", "b1", "A", "unavailable"), target("t2", "b2", "B", "unavailable")])
    assert scripted.decide(st, rec) == ["close_enquiry"]
    assert rec.kwargs_of("close_enquiry")[0]["outcome"] == "unavailable"


def test_silent_sent_target_gets_one_follow_up():
    rec = Recorder()
    st = state(targets=[target("t1", "b1", "A", "sent", follow_ups=0)],
               messages=[msg("t1", "enquiry", status="sent")])

    names = scripted.decide(st, rec)

    assert names == ["follow_up", "wait"]
    assert rec.kwargs_of("follow_up")[0] == {"target_id": "t1"}


def test_silent_after_follow_up_is_marked_no_response():
    rec = Recorder()
    st = state(targets=[target("t1", "b1", "A", "sent", follow_ups=1)],
               messages=[msg("t1", "enquiry", status="sent"), msg("t1", "follow_up", status="sent")])

    names = scripted.decide(st, rec)

    assert names == ["mark_no_response", "wait"]


def test_pending_sent_target_just_waits():
    rec = Recorder()
    st = state(targets=[target("t1", "b1", "A", "sent", follow_ups=1)],
               messages=[msg("t1", "enquiry", status="sent"), msg("t1", "follow_up", status="queued")])

    assert scripted.decide(st, rec) == ["wait"]


def test_failed_candidate_lookup_waits_instead_of_closing():
    rec = Recorder({"find_candidates": {"error": "backend down", "code": "internal"}})

    names = scripted.decide(state(targets=[]), rec)

    assert names == ["find_candidates", "wait"]
