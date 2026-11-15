"""next_step rules for structured offers: read every text reply with record_offer before notify/close/wait."""
import pytest

import _http
import enquiry


def _detail(status="waiting", targets=()):
    return {
        "enquiry": {"id": "e1", "status": status, "raw_request": "Zithromax 500mg, 2 boîtes", "quantity": 2},
        "medication": {"brand_name": "Zithromax", "strength": "500mg", "dosage_form": "tablet"},
        "targets": list(targets),
        "messages": [],
    }


def _target(tid, status, text="", branch="Baabda"):
    return {"id": tid, "branch_id": "branch-" + branch.lower(), "branch_name": branch, "status": status, "reply_text": text}


@pytest.fixture
def backend(monkeypatch):
    def install(detail, offers=()):
        calls = []

        def call(method, path, body=None, query=None):
            calls.append((method, path))
            return {"data": list(offers)} if path.endswith("/offers") else detail
        monkeypatch.setattr(_http, "call", call)
        return calls
    return install


def test_reply_without_offer_asks_for_record_offer_first(backend):
    calls = backend(_detail(targets=[_target("t1", "alternative_offered", "only 250mg in stock")]))
    out = enquiry.get_enquiry.fn("e1") if hasattr(enquiry.get_enquiry, "fn") else enquiry.get_enquiry("e1")
    assert out["next_step"].startswith("record_offer for target t1 (Baabda)")
    assert "exact quotes" in out["next_step"] and "the rules decide" in out["next_step"]
    assert ("GET", "/api/v1/enquiries/e1/offers") in calls


def test_different_strength_adds_check_reference_hint(backend):
    backend(_detail(targets=[_target("t1", "alternative_offered", "only 250mg in stock")]))
    step = enquiry._compact_enquiry(_detail(targets=[_target("t1", "alternative_offered", "only 250mg in stock")]))["next_step"]
    assert "check_reference(ingredient, '250mg', form)" in step and "information only" in step


def test_same_strength_has_no_reference_hint():
    step = enquiry._compact_enquiry(_detail(targets=[_target("t1", "available", "yes 2 boxes of 500 mg")]))["next_step"]
    assert step.startswith("record_offer for target t1") and "check_reference" not in step


def test_gram_and_mg_are_the_same_strength():
    assert enquiry._other_strength("Augmentin 1000mg tablet", "we have the 1g") is None
    assert enquiry._other_strength("Augmentin 1000mg tablet", "only 625 mg") == "625mg"


def test_after_offers_recorded_old_logic_applies():
    targets = [_target("t1", "alternative_offered", "only 250mg in stock"), _target("t2", "unavailable", "ma 3anna", "Verdun")]
    out = enquiry._compact_enquiry(_detail(targets=targets), {"t1": "rejected", "t2": "rejected"})
    assert out["targets"][0]["offer"] == "rejected"
    assert "record_offer" not in out["next_step"] and "notify_requester" in out["next_step"]


def test_second_target_is_next_when_first_has_offer():
    targets = [_target("t1", "alternative_offered", "only 250mg"), _target("t2", "unavailable", "ma 3anna", "Verdun")]
    step = enquiry._compact_enquiry(_detail(targets=targets), {"t1": "rejected"})["next_step"]
    assert step.startswith("record_offer for target t2 (Verdun)")


def test_escalated_available_reply_is_not_closed_as_available():
    out = enquiry._compact_enquiry(_detail(targets=[_target("t1", "available", "3anna 625mg")]), {"t1": "escalated"})
    assert "outcome 'available'" not in out["next_step"]


def test_rules_match_on_alternative_notifies_and_never_closes_available():
    # Go Close only accepts a target whose status is available, so an alternative must not be closed as available.
    out = enquiry._compact_enquiry(_detail(targets=[_target("t1", "alternative_offered", "4 boxes of 250mg")]), {"t1": "matches"})
    assert "notify_requester naming Baabda" in out["next_step"] and "dose equivalent" in out["next_step"]
    assert "close_enquiry outcome 'available'" not in out["next_step"]


def _desk(targets, **extra):
    detail = _detail(targets=targets)
    detail["enquiry"]["requester_branch_id"] = "branch-client-desk"
    detail.update(extra)
    return detail


def test_client_desk_closes_available_only_with_a_rules_accepted_offer():
    targets = [_target("t1", "available", "3anna 2 boxes 500mg")]
    step = enquiry._compact_enquiry(_desk(targets), {"t1": "matches"})["next_step"]
    assert "close_enquiry outcome 'available' with branch_id branch-baabda" in step
    out = enquiry._compact_enquiry(_desk([_target("t1", "available", "")]))
    assert out["client_desk"] is True
    assert "notify_requester on the client ticket naming Baabda" in out["next_step"] and "Do NOT close as available" in out["next_step"]
    assert "no advice" in out["next_step"]


def test_client_desk_unverified_and_already_notified_falls_through_without_close_available():
    detail = _desk([_target("t1", "available", "")], messages=[{"kind": "notification", "body": "Baabda says yes"}])
    assert "outcome 'available'" not in enquiry._compact_enquiry(detail)["next_step"]


def test_client_desk_never_asks_identity():
    detail = _desk([])
    detail["medication"] = None
    step = enquiry._compact_enquiry(detail)["next_step"]
    assert "Never ask who the client is" in step


def test_photo_wake_reads_the_photo_first():
    targets = [_target("t1", "available", "yes 500mg")]
    out = enquiry._compact_enquiry(_desk(targets, photos=[{"id": "p1", "status": "verified"}, {"id": "p2", "status": "pending"}]),
                                   {"t1": "matches"})
    assert out["photos_to_read"] == ["p2"]
    assert out["next_step"].startswith("read_package_photo(photo_id) for photo p2")
    detail = _desk(targets)
    detail["enquiry"].update(wake_trigger="photo", wake_photo_id="p9")
    assert enquiry._compact_enquiry(detail, {"t1": "matches"})["next_step"].startswith("read_package_photo(photo_id) for photo p9")


def test_decided_photos_do_not_block():
    out = enquiry._compact_enquiry(_desk([_target("t1", "available", "")], photos=[{"id": "p1", "status": "verified"}]))
    assert "photos_to_read" not in out and "read_package_photo" not in out["next_step"]


def _sent(tid, follow_ups, branch="Achrafieh"):
    return {**_target(tid, "sent", "", branch), "follow_ups": follow_ups}


def _delivered(*tids):
    return [{"target_id": t, "kind": "enquiry", "status": "sent", "created_at": "1"} for t in tids]


def test_case14_ending_follow_up_then_no_response_then_close():
    replied = [_target("t1", "alternative_offered", "only 250mg in stock"), _target("t2", "unavailable", "", "Verdun")]
    offers = {"t1": "rejected"}
    # alternative not yet reported: notify, then chase the silent pharmacy in the same run
    d = _detail(targets=replied + [_sent("t3", 0)]); d["messages"] = _delivered("t3")
    step = enquiry._compact_enquiry(d, offers)["next_step"]
    assert "notify_requester naming Baabda" in step and "in the same run: Still silent: follow_up t3" in step
    # reported: follow up
    d["messages"] = _delivered("t3") + [{"kind": "notification", "body": "Baabda offered 250mg"}]
    assert enquiry._compact_enquiry(d, offers)["next_step"].startswith("Still silent: follow_up t3")
    # followed up once: wait, then mark_no_response
    d["targets"] = replied + [_sent("t3", 1)]
    assert "mark_no_response t3" in enquiry._compact_enquiry(d, offers)["next_step"]
    # nobody is `sent` any more and none is available: unable_to_confirm
    d["targets"] = replied + [_target("t3", "no_response", "", "Achrafieh")]
    assert enquiry._compact_enquiry(d, offers)["next_step"].endswith("close_enquiry outcome 'unable_to_confirm'.")


def test_all_unavailable_closes_unavailable():
    d = _detail(targets=[_target("t1", "unavailable", ""), _target("t2", "unavailable", "", "Verdun")])
    assert "outcome 'unavailable'" in enquiry._compact_enquiry(d)["next_step"]


def test_reply_without_text_needs_no_offer_and_no_offers_request(backend):
    calls = backend(_detail(targets=[_target("t1", "available", "")]))
    out = enquiry.get_enquiry.fn("e1") if hasattr(enquiry.get_enquiry, "fn") else enquiry.get_enquiry("e1")
    assert "close_enquiry outcome 'available'" in out["next_step"]
    assert all(not path.endswith("/offers") for _, path in calls)


@pytest.mark.parametrize("status", ["awaiting_approval", "waiting"])
def test_never_suggests_wait_while_proposed(status):
    step = enquiry._compact_enquiry(_detail(status=status, targets=[_target("t1", "proposed")]))["next_step"]
    assert "Stop now" in step and "Do not call wait()" in step and "wait(5)" not in step


def test_no_targets_step_does_not_end_with_wait():
    assert "then wait" not in enquiry._compact_enquiry(_detail(targets=[]))["next_step"]
