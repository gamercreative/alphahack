"""Agent human-in-the-loop flow: CONTRACT rule 2 and "HITL count = clarified + targets_approved".

Every test builds its own enquiry (handled_by=agent) and asserts on that enquiry only.
"""

P_FMT = "/api/v1/enquiries/{}"


def agent_enquiry(live):
    """A fresh enquiry handled by the agent. Returns its id."""
    return live.new_enquiry(handled_by="agent", medication_id=live.MED_1G)["id"]


def wake(live, E):
    return live.wake_triggers(E)


def start(live, E, trigger):
    """Start an agent run on E for the given trigger. Asserts 201."""
    status, body = live.call("POST", "/api/v1/agent/runs", "agent",
                             {"enquiry_id": E, "trigger": trigger, "model": "scripted"})
    assert status == 201, body
    return body


def ask(live, E, who="agent"):
    """The agent asks the requester a clarification question. Returns (status, body)."""
    return live.call("POST", P_FMT.format(E) + "/clarification-request", who,
                     {"question": "Which strength?"})


def answer(live, E, who):
    """Staff `who` answers the pending clarification. Returns (status, body)."""
    return live.call("POST", P_FMT.format(E) + "/clarify", who, {"answer": "1g tablets"})


def approve(live, E, target_ids, who):
    """Approve the given target ids as `who`. Returns (status, body)."""
    return live.call("POST", P_FMT.format(E) + "/targets/approve", who, {"target_ids": target_ids})


def asked(live):
    """A started enquiry whose agent has asked the requester a question. Returns the enquiry id."""
    E = agent_enquiry(live)
    start(live, E, "created")
    status, body = ask(live, E)
    assert status == 200, body
    return E


def sent_flow(live):
    """Agent proposes b and c, staff a approves both, agent sends, and the messages are delivered.

    Returns (enquiry id, {letter: target dict}) with the targets in status "sent".
    """
    E = agent_enquiry(live)
    targets = live.add_targets(E, ("b", "c"), who="agent")
    status, body = approve(live, E, [targets["b"]["id"], targets["c"]["id"]], "a")
    assert status == 200, body
    status, body = live.call("POST", P_FMT.format(E) + "/send", "agent", {})
    assert status == 200, body
    live.wait_sent(E)
    return E, targets


# ---------------------------------------------------------------- clarification


def test_agent_asks_requester(live):
    E = agent_enquiry(live)
    start(live, E, "created")
    status, body = ask(live, E)
    assert status == 200, body
    d = live.detail(E)
    assert d["enquiry"]["status"] == "awaiting_clarification"
    assert d["enquiry"]["clarification_question"] == "Which strength?"
    assert wake(live, E) == []


def test_second_question_409(live):
    E = asked(live)
    status, body = ask(live, E)
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_staff_cannot_ask_403(live):
    E = agent_enquiry(live)
    start(live, E, "created")
    status, body = ask(live, E, who="a")
    assert status == 403, body
    assert live.code(body) == "forbidden"


def test_requester_answers(live):
    E = asked(live)
    status, body = answer(live, E, "a")
    assert status == 200, body
    d = live.detail(E)
    assert d["enquiry"]["status"] == "open"
    assert d["enquiry"]["clarification_answer"] == "1g tablets"
    assert wake(live, E) == ["clarified"]
    assert d["hitl_count"] == 1


def test_agent_cannot_answer_403_agent_forbidden(live):
    E = asked(live)
    status, body = answer(live, E, "agent")
    assert status == 403, body
    assert live.code(body) == "agent_forbidden"
    d = live.detail(E)
    assert d["enquiry"]["status"] == "awaiting_clarification"
    assert d["hitl_count"] == 0


def test_other_pharmacy_cannot_answer_403(live):
    E = asked(live)
    status, body = answer(live, E, "d")
    assert status == 403, body
    assert live.code(body) == "forbidden"


def test_answer_without_question_409(live):
    E = agent_enquiry(live)
    status, body = answer(live, E, "a")
    assert status == 409, body
    assert live.code(body) == "conflict"


# ---------------------------------------------------------------- target approval


def test_agent_targets_are_proposed_not_approved(live):
    E = agent_enquiry(live)
    targets = live.add_targets(E, ("b", "c"), who="agent")
    assert targets["b"]["status"] == "proposed"
    assert targets["c"]["status"] == "proposed"
    d = live.detail(E)
    assert d["enquiry"]["status"] == "awaiting_approval"


def test_agent_cannot_send_unapproved_409(live):
    E = agent_enquiry(live)
    live.add_targets(E, ("b", "c"), who="agent")
    status, body = live.call("POST", P_FMT.format(E) + "/send", "agent", {})
    assert status == 409, body
    assert live.code(body) == "conflict"
    assert live.detail(E)["messages"] == []


def test_agent_cannot_approve_403_agent_forbidden(live):
    E = agent_enquiry(live)
    targets = live.add_targets(E, ("b", "c"), who="agent")
    status, body = approve(live, E, [targets["b"]["id"], targets["c"]["id"]], "agent")
    assert status == 403, body
    assert live.code(body) == "agent_forbidden"
    assert live.target(E, "b")["status"] == "proposed"
    assert live.target(E, "c")["status"] == "proposed"


def test_requester_approves(live):
    E = agent_enquiry(live)
    targets = live.add_targets(E, ("b", "c"), who="agent")
    status, body = approve(live, E, [targets["b"]["id"], targets["c"]["id"]], "a")
    assert status == 200, body
    assert live.target(E, "b")["status"] == "approved"
    assert live.target(E, "c")["status"] == "approved"
    assert live.detail(E)["enquiry"]["status"] == "open"
    assert "approved" in wake(live, E)
    assert live.detail(E)["hitl_count"] == 1


def test_approve_twice_409(live):
    E = agent_enquiry(live)
    targets = live.add_targets(E, ("b", "c"), who="agent")
    ids = [targets["b"]["id"], targets["c"]["id"]]
    status, body = approve(live, E, ids, "a")
    assert status == 200, body
    status, body = approve(live, E, ids, "a")
    assert status == 409, body
    assert live.code(body) == "conflict"
    assert live.detail(E)["hitl_count"] == 1


def test_other_pharmacy_cannot_approve_403(live):
    E = agent_enquiry(live)
    targets = live.add_targets(E, ("b", "c"), who="agent")
    status, body = approve(live, E, [targets["b"]["id"], targets["c"]["id"]], "d")
    assert status == 403, body
    assert live.code(body) == "forbidden"


# ---------------------------------------------------------------- HITL count


def test_hitl_counts_both(live):
    E = asked(live)
    status, body = answer(live, E, "a")
    assert status == 200, body
    targets = live.add_targets(E, ("b", "c"), who="agent")
    status, body = approve(live, E, [targets["b"]["id"], targets["c"]["id"]], "a")
    assert status == 200, body
    assert live.detail(E)["hitl_count"] == 2
    assert live.stats_row(E)["hitl_count"] == 2


# ---------------------------------------------------------------- send and reply


def test_agent_send_after_approval(live):
    E = agent_enquiry(live)
    targets = live.add_targets(E, ("b", "c"), who="agent")
    status, body = approve(live, E, [targets["b"]["id"], targets["c"]["id"]], "a")
    assert status == 200, body
    status, body = live.call("POST", P_FMT.format(E) + "/send", "agent", {})
    assert status == 200, body
    live.wait_sent(E)
    messages = live.messages(E)
    assert messages, "no messages were created"
    assert {m["created_by_actor"] for m in messages} == {"agent"}
    assert live.target(E, "b")["status"] == "sent"
    assert live.target(E, "c")["status"] == "sent"


def test_reply_wakes_agent(live):
    E, targets = sent_flow(live)
    start(live, E, "approved")
    status, body = live.reply(targets["b"]["id"], "b", "unavailable")
    assert status == 200, body
    assert wake(live, E) == ["reply"]


def test_agent_cannot_close_available_without_reply_409(live):
    E, _ = sent_flow(live)
    status, body = live.close(E, "available", who="agent", branch="b")
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_agent_cannot_close_while_pending_409(live):
    E, _ = sent_flow(live)
    status, body = live.close(E, "unable_to_confirm", who="agent")
    assert status == 409, body
    assert live.code(body) == "conflict"


def test_agent_cannot_cancel_403(live):
    E = agent_enquiry(live)
    status, body = live.close(E, "cancelled", who="agent")
    assert status == 403, body
    assert live.code(body) == "forbidden"
