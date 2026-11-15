"""Concurrency: two identical or conflicting requests fired at the same moment. Lead-owned.

For every pair exactly one request may win; the loser gets 409 conflict, and the stored state has no
duplicate effect (one reply, one follow-up message, one HITL count, one message per target).
Each test repeats the race several times, because a race that is lost once in ten is still a bug.
"""

import threading

import pytest

from conftest import KnownBugFailure, assert_known_bug

ATTEMPTS = 6


def race(*calls):
    """Run the callables at the same instant on separate threads. Returns their (status, body) results in order."""
    barrier = threading.Barrier(len(calls))
    results = [None] * len(calls)

    def worker(index, fn):
        barrier.wait()
        results[index] = fn()

    threads = [threading.Thread(target=worker, args=(i, fn)) for i, fn in enumerate(calls)]
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join(30)
    assert all(result is not None for result in results), "a racing request did not finish"
    return results


def statuses(results):
    return sorted(status for status, _ in results)


def loser_code(live, results):
    return [live.code(body) for status, body in results if status != 200 and status != 201]


@pytest.mark.xfail(strict=True, raises=KnownBugFailure,
                   reason="BUG-15: two simultaneous replies to one target are both accepted (the status "
                          "check runs before the enquiry lock); two reply_received events, last write wins")
def test_race_two_replies_to_one_target_only_one_wins(live):
    enquiry_id, targets = live.sent_enquiry(("b", "c", "d", "e"))
    for letter in ("b", "c", "d", "e"):
        target_id = targets[letter]["id"]

        results = race(lambda: live.reply(target_id, letter, "available", quantity=2),
                       lambda: live.reply(target_id, letter, "unavailable"))

        assert_known_bug(statuses(results) == [200, 409], f"{letter}: {statuses(results)}")
        assert_known_bug(loser_code(live, results) == ["conflict"])
    replies = live.events(enquiry_id, type="reply_received")
    assert_known_bug(len(replies) == 4, f"{len(replies)} reply_received events for 4 targets")


def test_race_two_closes_only_one_wins(live):
    for _ in range(ATTEMPTS):
        enquiry_id = live.new_enquiry()["id"]

        results = race(lambda: live.close(enquiry_id, "unable_to_confirm"),
                       lambda: live.close(enquiry_id, "cancelled"))

        assert statuses(results) == [200, 409]
        assert loser_code(live, results) == ["conflict"]
        assert len(live.events(enquiry_id, type="enquiry_closed")) == 1


def test_race_two_undos_of_one_event_only_one_wins(live):
    for _ in range(ATTEMPTS):
        enquiry_id = live.new_enquiry()["id"]
        live.add_targets(enquiry_id, ("b", "c"))
        event = [e for e in live.events(enquiry_id) if e["type"].startswith("targets_")][-1]
        undo = lambda: live.call("POST", f"/api/v1/events/{event['id']}/undo", "a", {})  # noqa: E731

        results = race(undo, undo)

        assert statuses(results) == [200, 409]
        assert loser_code(live, results) == ["conflict"]
        assert len(live.events(enquiry_id, type="event_undone")) == 1
        assert live.detail(enquiry_id)["targets"] == []


def test_race_two_sends_queue_one_message_per_target(live):
    for _ in range(ATTEMPTS):
        enquiry_id = live.new_enquiry()["id"]
        live.add_targets(enquiry_id, ("b", "c"))
        send = lambda: live.call("POST", f"/api/v1/enquiries/{enquiry_id}/send", "a", {})  # noqa: E731

        results = race(send, send)

        assert statuses(results) == [200, 409]
        assert loser_code(live, results) == ["conflict"]
        assert len(live.messages(enquiry_id, kind="enquiry")) == 2  # one per target, not four
        live.close(enquiry_id, "cancelled")


def test_race_two_adds_of_the_same_pharmacy_only_one_wins(live):
    for _ in range(ATTEMPTS):
        enquiry_id = live.new_enquiry()["id"]
        add = lambda: live.call("POST", f"/api/v1/enquiries/{enquiry_id}/targets", "a",  # noqa: E731
                                {"branch_ids": [live.BRANCH["b"]]})

        results = race(add, add)

        assert statuses(results) == [201, 409], statuses(results)
        assert len(live.detail(enquiry_id)["targets"]) == 1


def test_race_two_approvals_count_one_human_step(live):
    for _ in range(ATTEMPTS):
        enquiry_id = live.new_enquiry(handled_by="agent")["id"]
        proposed = live.add_targets(enquiry_id, ("b", "c"), who="agent")
        ids = [proposed["b"]["id"], proposed["c"]["id"]]
        approve = lambda: live.call("POST", f"/api/v1/enquiries/{enquiry_id}/targets/approve", "a",  # noqa: E731
                                    {"target_ids": ids})

        results = race(approve, approve)

        assert statuses(results) == [200, 409]
        assert loser_code(live, results) == ["conflict"]
        assert live.detail(enquiry_id)["hitl_count"] == 1  # one approval, counted once


def test_race_two_follow_ups_send_one(live):
    enquiry_id, targets = live.sent_enquiry(("b", "c", "d", "e"))
    for letter in ("b", "c", "d", "e"):
        follow_up = lambda: live.call("POST", f"/api/v1/targets/{targets[letter]['id']}/follow-up", "a", {})  # noqa: E731

        results = race(follow_up, follow_up)

        assert statuses(results) == [200, 409], f"{letter}: {statuses(results)}"
        assert loser_code(live, results) == ["conflict"]
        assert live.target(enquiry_id, letter)["follow_ups"] == 1
    assert len(live.messages(enquiry_id, kind="follow_up")) == 4  # one per target (CONTRACT: max 1 per target)


def test_race_two_cancels_of_one_message_only_one_wins(live):
    for _ in range(ATTEMPTS):
        enquiry_id = live.new_enquiry()["id"]
        live.add_targets(enquiry_id, ("b",))
        message = live.send(enquiry_id)[0]
        cancel = lambda: live.call("POST", f"/api/v1/messages/{message['id']}/cancel", "a", {})  # noqa: E731

        results = race(cancel, cancel)

        assert statuses(results) == [200, 409]
        assert loser_code(live, results) == ["conflict"]
        assert len(live.events(enquiry_id, type="message_cancelled")) == 1


def test_race_reply_against_close_leaves_a_consistent_enquiry(live):
    """A reply and a close at the same instant: whichever wins, a closed enquiry never gains a reply afterwards."""
    for _ in range(3):
        enquiry_id, targets = live.sent_enquiry(("b",))

        reply, close = race(lambda: live.reply(targets["b"]["id"], "b", "available", quantity=1),
                            lambda: live.close(enquiry_id, "unable_to_confirm"))

        assert close[0] == 200, close
        assert reply[0] in (200, 409), reply
        events = [e["type"] for e in live.events(enquiry_id)]
        if reply[0] == 200:  # the reply got in first: it must be logged before the close
            assert events.index("reply_received") < events.index("enquiry_closed")
        else:
            assert live.code(reply[1]) == "conflict" and "reply_received" not in events
