#!/usr/bin/env python3.13
"""Wassit acceptance checks (CONTRACT.md) against a LIVE backend. Stdlib only.

Run after the backend is up and seeded (for example `make docker-up`, with
UNDO_WINDOW=3s so the outbox dispatcher, which ticks every 2s, is quick enough):

    python3.13 scripts/acceptance.py            # run every check
    python3.13 scripts/acceptance.py --help     # this text, no network calls

Environment:
    BACKEND_URL   backend base URL                  (default http://localhost:8080)
    AGENT_TOKEN   agent credential, sent as
                  "Authorization: Agent <token>"    (default dev-agent-token)
    UNDO_WAIT     seconds to wait for the outbox dispatcher (default 5)

Each assertion prints one line: "PASS check N: ..." or "FAIL check N: ...".
A failed step does not stop the other checks. Steps that depend on a missing id
are skipped with a FAIL line.

Exit status: 0 when every check passed, 1 when any check failed.
"""

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request


def _env_float(name, default):
    raw = os.environ.get(name, str(default))
    try:
        return float(raw)
    except ValueError:
        sys.exit(f"{name} must be a number, got {raw!r}")


BACKEND_URL = os.environ.get("BACKEND_URL", "http://localhost:8080").rstrip("/")
AGENT_TOKEN = os.environ.get("AGENT_TOKEN", "dev-agent-token")
UNDO_WAIT = _env_float("UNDO_WAIT", 5)
NO_RESPONSE_WAIT = float(os.environ.get("NO_RESPONSE_WAIT", "11"))  # 5 x WAIT_UNIT (2s) + margin
HTTP_TIMEOUT = 15

# Demo seed (see CONTRACT.md "Seed"): one staff per branch, PIN 1234.
PIN = "1234"
STAFF_EMAIL = {
    "A": "a@wassit.demo",
    "B": "b@wassit.demo",
    "C": "c@wassit.demo",
    "D": "d@wassit.demo",
}
BRANCH = {
    "A": "branch-hamra",
    "B": "branch-achrafieh",
    "C": "branch-baabda",
    "D": "branch-verdun",
}
MED_AUGMENTIN_1G = "med-augmentin-1g"

_tokens = {}  # staff letter -> token (None when login failed)
_MISSING = object()
RESULTS = []  # one bool per printed line
AGENT_ENQUIRIES = []  # agent enquiries created by checks 2 and 6, read by check 7


# ---------------------------------------------------------------- HTTP


def _parse(raw):
    if not raw:
        return {}
    try:
        return json.loads(raw)
    except ValueError:
        return {"raw": raw.decode("utf-8", "replace")}


def _login(who):
    status, body = req("POST", "/api/v1/auth/login", None,
                       {"email": STAFF_EMAIL[who], "pin": PIN})
    token = field(body, "token") if status == 200 else None
    return token if isinstance(token, str) else None


def req(method, path, who, body=None):
    """Call the backend. who: None (no Authorization), "agent", or "A".."D" (staff).

    Returns (status, parsed_json). Network failures return (0, {"error": {...}}).
    """
    headers = {"Accept": "application/json"}
    if who == "agent":
        headers["Authorization"] = f"Agent {AGENT_TOKEN}"
    elif who is not None:
        if who not in _tokens:
            _tokens[who] = _login(who)
        if _tokens[who]:
            headers["Authorization"] = f"Bearer {_tokens[who]}"
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(BACKEND_URL + path, data=data,
                                     method=method, headers=headers)
    try:
        with urllib.request.urlopen(request, timeout=HTTP_TIMEOUT) as resp:
            return resp.status, _parse(resp.read())
    except urllib.error.HTTPError as err:
        return err.code, _parse(err.read())
    except OSError as err:  # URLError, timeouts, refused connections
        return 0, {"error": {"code": "network", "message": str(err)}}


# ---------------------------------------------------------------- body helpers


def lookup(body, key):
    """Value of `key` at top level, or inside body["enquiry"] (detail wrapper)."""
    if isinstance(body, dict):
        if key in body:
            return body[key]
        inner = body.get("enquiry")
        if isinstance(inner, dict) and key in inner:
            return inner[key]
    return _MISSING


def field(body, key):
    value = lookup(body, key)
    return None if value is _MISSING else value


def rows(body, key=None):
    """List payload: the body itself if it is a list, body[key], body[key]["data"], or body["data"]."""
    if isinstance(body, list):
        return body
    if not isinstance(body, dict):
        return None
    if key is not None:
        value = lookup(body, key)
        if isinstance(value, list):
            return value
        if isinstance(value, dict) and isinstance(value.get("data"), list):
            return value["data"]
    if isinstance(body.get("data"), list):
        return body["data"]
    return None


def _id(body):
    return body.get("id") if isinstance(body, dict) else None


def snippet(body, limit=600):
    text = json.dumps(body, ensure_ascii=False, default=str)
    return text if len(text) <= limit else text[:limit] + "...(truncated)"


# ---------------------------------------------------------------- output and assertions


def emit(n, ok, msg):
    RESULTS.append(bool(ok))
    print(f"{'PASS' if ok else 'FAIL'} check {n}: {msg}", flush=True)
    return bool(ok)


def abort(n, why):
    return emit(n, False, f"could not continue ({why}); remaining steps of this check skipped")


def expect(n, label, resp, want, code=None, check=None):
    """One line. HTTP status must equal `want`; error.code must equal `code` when given;
    `check(body)` returns a problem string or None. Returns the body on PASS, None on FAIL."""
    status, body = resp
    problems = []
    if status != want:
        problems.append(f"expected HTTP {want}, got {status}")
    if code is not None:
        err = body.get("error") if isinstance(body, dict) else None
        got = err.get("code") if isinstance(err, dict) else None
        if got != code:
            problems.append(f"expected error.code {code!r}, got {got!r}")
    if check is not None and not problems:
        problem = check(body)
        if problem:
            problems.append(problem)
    if problems:
        why = "; ".join(problems)
        emit(n, False, f"{label}: {why}; status={status} body={snippet(body)}")
        return None
    emit(n, True, label)
    return body


def detail(eid):
    return req("GET", f"/api/v1/enquiries/{eid}", "A")


def target_ids(eid):
    """{branch_id: target_id} read from the enquiry detail."""
    _, body = detail(eid)
    out = {}
    for t in rows(body, "targets") or []:
        if isinstance(t, dict):
            out[t.get("branch_id")] = t.get("id")
    return out


def assert_statuses(n, label, eid, want):
    """want: {branch_id: target status}. The enquiry's target set must match exactly."""
    status, body = detail(eid)
    if status != 200:
        return emit(n, False, f"{label}: GET enquiry expected HTTP 200, got {status}; body={snippet(body)}")
    targets = rows(body, "targets")
    if targets is None:
        return emit(n, False, f"{label}: no targets list in enquiry detail; body={snippet(body)}")
    got = {t.get("branch_id"): t.get("status") for t in targets if isinstance(t, dict)}
    if got == want:
        return emit(n, True, f"{label}: {got}")
    return emit(n, False, f"{label}: expected target statuses {want}, got {got}; body={snippet(body)}")


def assert_field(n, label, eid, key, want):
    status, body = detail(eid)
    if status != 200:
        return emit(n, False, f"{label}: GET enquiry expected HTTP 200, got {status}; body={snippet(body)}")
    got = field(body, key)
    if got == want:
        return emit(n, True, f"{label}: {key}={got!r}")
    return emit(n, False, f"{label}: expected {key}={want!r}, got {got!r}; body={snippet(body)}")


def wake_check(n, label, eid, want_trigger):
    """want_trigger None means the enquiry must NOT be in the wakeups list."""
    status, body = req("GET", "/api/v1/agent/wakeups", "agent")
    if status != 200:
        return emit(n, False, f"{label}: GET /agent/wakeups expected HTTP 200, got {status}; body={snippet(body)}")
    items = rows(body, "data")
    if items is None:
        return emit(n, False, f"{label}: wakeups has no data list; body={snippet(body)}")
    triggers = [w.get("trigger") for w in items if isinstance(w, dict) and w.get("enquiry_id") == eid]
    if want_trigger is None:
        if not triggers:
            return emit(n, True, label)
        return emit(n, False, f"{label}: expected no wakeup for {eid}, got triggers {triggers}")
    if want_trigger in triggers:
        return emit(n, True, f"{label} (trigger {want_trigger!r})")
    return emit(n, False, f"{label}: expected trigger {want_trigger!r}, got {triggers}; body={snippet(body)}")


def inbox_has(n, label, who, eid, want_present):
    status, body = req("GET", "/api/v1/inbox", who)
    if status != 200:
        return emit(n, False, f"{label}: GET /inbox expected HTTP 200, got {status}; body={snippet(body)}")
    items = rows(body)
    if items is None:
        return emit(n, False, f"{label}: inbox has no data list; body={snippet(body)}")
    present = any(isinstance(i, dict) and i.get("enquiry_id") == eid for i in items)
    if present == want_present:
        return emit(n, True, label)
    return emit(n, False, f"{label}: expected present={want_present}, got present={present}; body={snippet(body)}")


def create_enquiry(who, raw_request, quantity, handled_by, medication_id=None):
    body = {"raw_request": raw_request, "quantity": quantity, "handled_by": handled_by}
    if medication_id is not None:
        body["medication_id"] = medication_id
    return req("POST", "/api/v1/enquiries", who, body)


def _two_queued(body):
    items = rows(body) or []
    queued = [m for m in items if isinstance(m, dict) and m.get("status") == "queued"]
    if len(items) == 2 and len(queued) == 2:
        return None
    return f"expected exactly 2 queued messages in data, got {len(items)} items with {len(queued)} queued"


def _outcome_is(want):
    def check(body):
        got = field(body, "outcome")
        return None if got == want else f"expected outcome {want!r}, got {got!r}"
    return check


# ---------------------------------------------------------------- checks


def check1_manual_flow():
    n = 1
    body = expect(n, "A creates manual enquiry (augmentin 1g x2, medication set) -> 201",
                  create_enquiry("A", "augmentin 1g x2", 2, "manual", MED_AUGMENTIN_1G), 201)
    eid = _id(body)
    if not eid:
        return abort(n, "no enquiry id")

    expect(n, "A targets achrafieh + baabda -> 201",
           req("POST", f"/api/v1/enquiries/{eid}/targets", "A",
               {"branch_ids": [BRANCH["B"], BRANCH["C"]]}), 201)
    assert_statuses(n, "both targets are approved", eid,
                    {BRANCH["B"]: "approved", BRANCH["C"]: "approved"})
    ids = target_ids(eid)
    tb, tc = ids.get(BRANCH["B"]), ids.get(BRANCH["C"])
    if not tb or not tc:
        return abort(n, f"target ids missing: {ids}")

    sent = expect(n, "A sends -> 200 with two queued messages",
                  req("POST", f"/api/v1/enquiries/{eid}/send", "A"), 200, check=_two_queued)
    if sent is None:
        return abort(n, "send did not return two queued messages")
    msg_c = {m.get("target_id"): m.get("id") for m in rows(sent) or []}.get(tc)
    if not msg_c:
        return abort(n, f"no queued message for C's target {tc}")

    expect(n, "A cancels C's queued message inside the undo window -> 200",
           req("POST", f"/api/v1/messages/{msg_c}/cancel", "A"), 200)
    time.sleep(UNDO_WAIT)

    inbox_has(n, "C inbox has no item for the enquiry (cancelled message never sent)", "C", eid, False)
    inbox_has(n, "B inbox has the enquiry (positive control)", "B", eid, True)
    expect(n, "B replies available, quantity 2 -> 200",
           req("POST", f"/api/v1/targets/{tb}/reply", "B", {"kind": "available", "quantity": 2}), 200)
    expect(n, "A closes available at achrafieh -> 200 with outcome available",
           req("POST", f"/api/v1/enquiries/{eid}/close", "A",
               {"outcome": "available", "branch_id": BRANCH["B"]}), 200, check=_outcome_is("available"))


def check2_agent_api_flow():
    n = 2
    body = expect(n, "A creates handled_by=agent 'need augmentin 1g tabs x2' (no medication) -> 201",
                  create_enquiry("A", "need augmentin 1g tabs x2", 2, "agent"), 201)
    eid = _id(body)
    if not eid:
        return abort(n, "no enquiry id")
    AGENT_ENQUIRIES.append(eid)

    wake_check(n, "agent wakeups include it", eid, "created")
    body = expect(n, "agent starts run (trigger created, model test) -> 201",
                  req("POST", "/api/v1/agent/runs", "agent",
                      {"enquiry_id": eid, "trigger": "created", "model": "test"}), 201)
    run_id = _id(body)
    if not run_id:
        return abort(n, "no run id")
    wake_check(n, "wakeups no longer include it once the run starts", eid, None)

    expect(n, "agent sets medication augmentin 1g -> 200",
           req("POST", f"/api/v1/enquiries/{eid}/medication", "agent",
               {"medication_id": MED_AUGMENTIN_1G}), 200)
    expect(n, "agent GET candidates -> 200 with a non-empty list",
           req("GET", f"/api/v1/enquiries/{eid}/candidates?limit=5&radius_km=15", "agent"), 200,
           check=lambda b: None if rows(b) else f"expected a non-empty candidates list, got {snippet(b, 200)}")

    expect(n, "agent proposes B, C, D -> 201",
           req("POST", f"/api/v1/enquiries/{eid}/targets", "agent",
               {"branch_ids": [BRANCH["B"], BRANCH["C"], BRANCH["D"]]}), 201)
    assert_statuses(n, "three targets are proposed", eid,
                    {BRANCH["B"]: "proposed", BRANCH["C"]: "proposed", BRANCH["D"]: "proposed"})
    assert_field(n, "enquiry is awaiting_approval", eid, "status", "awaiting_approval")

    expect(n, "agent sends with nothing approved -> 409",
           req("POST", f"/api/v1/enquiries/{eid}/send", "agent"), 409)
    expect(n, "agent waits 5 minutes -> 200",
           req("POST", f"/api/v1/enquiries/{eid}/wait", "agent", {"minutes": 5}), 200)

    body = expect(n, "agent completes run with token usage -> 200",
                  req("PATCH", f"/api/v1/agent/runs/{run_id}", "agent",
                      {"status": "completed", "input_tokens": 1200, "output_tokens": 300, "steps": 4}), 200)
    if body is not None:
        cost = field(body, "cost_micros")
        if isinstance(cost, int) and not isinstance(cost, bool):
            emit(n, True, f"run cost_micros is an integer: {cost} "
                          f"(>0 only when AGENT_PRICE_* are set; not asserted)")
        else:
            emit(n, False, f"expected integer cost_micros on run, got {cost!r}; body={snippet(body)}")

    ids = target_ids(eid)
    tb, tc, td = ids.get(BRANCH["B"]), ids.get(BRANCH["C"]), ids.get(BRANCH["D"])
    if not (tb and tc and td):
        return abort(n, f"target ids missing: {ids}")

    expect(n, "A approves all three targets -> 200",
           req("POST", f"/api/v1/enquiries/{eid}/targets/approve", "A",
               {"target_ids": [tb, tc, td]}), 200)
    assert_statuses(n, "all three targets approved", eid,
                    {BRANCH["B"]: "approved", BRANCH["C"]: "approved", BRANCH["D"]: "approved"})
    wake_check(n, "wakeups include trigger approved", eid, "approved")

    expect(n, "agent sends the approved targets -> 200",
           req("POST", f"/api/v1/enquiries/{eid}/send", "agent"), 200)
    time.sleep(UNDO_WAIT)

    expect(n, "B replies unavailable -> 200",
           req("POST", f"/api/v1/targets/{tb}/reply", "B", {"kind": "unavailable"}), 200)
    expect(n, "C replies alternative '625mg only' -> 200",
           req("POST", f"/api/v1/targets/{tc}/reply", "C",
               {"kind": "alternative", "text": "625mg only"}), 200)
    expect(n, "agent notifies requester about C's alternative -> 200",
           req("POST", f"/api/v1/enquiries/{eid}/notify", "agent",
               {"body": "Baabda offers 625mg only (not a match)"}), 200)
    expect(n, "agent follows up D once -> 200",
           req("POST", f"/api/v1/targets/{td}/follow-up", "agent"), 200)
    time.sleep(UNDO_WAIT + NO_RESPONSE_WAIT)  # follow-up delivered + one full wait
    expect(n, "agent marks D no_response -> 200",
           req("POST", f"/api/v1/targets/{td}/no-response", "agent"), 200)
    assert_statuses(n, "no target is available (B unavailable, C alternative_offered, D no_response)",
                    eid, {BRANCH["B"]: "unavailable", BRANCH["C"]: "alternative_offered",
                          BRANCH["D"]: "no_response"})
    expect(n, "agent closes available at C (only an alternative) -> 409 conflict",
           req("POST", f"/api/v1/enquiries/{eid}/close", "agent",
               {"outcome": "available", "branch_id": BRANCH["C"]}), 409, code="conflict")
    expect(n, "agent closes unable_to_confirm -> 200",
           req("POST", f"/api/v1/enquiries/{eid}/close", "agent",
               {"outcome": "unable_to_confirm"}), 200, check=_outcome_is("unable_to_confirm"))


def check3_agent_forbidden():
    n = 3
    body = expect(n, "A creates manual enquiry for the agent-403 setup -> 201",
                  create_enquiry("A", "augmentin 1g x2", 2, "manual", MED_AUGMENTIN_1G), 201)
    eid = _id(body)
    if not eid:
        return abort(n, "no enquiry id")
    expect(n, "A proposes achrafieh target (approved by staff) -> 201",
           req("POST", f"/api/v1/enquiries/{eid}/targets", "A", {"branch_ids": [BRANCH["B"]]}), 201)
    tb = target_ids(eid).get(BRANCH["B"])
    if not tb:
        return abort(n, "target id missing")

    expect(n, "agent marks stock available via reply -> 403 agent_forbidden",
           req("POST", f"/api/v1/targets/{tb}/reply", "agent",
               {"kind": "available", "quantity": 2}), 403, code="agent_forbidden")
    expect(n, "agent approves a target via targets/approve -> 403 agent_forbidden",
           req("POST", f"/api/v1/enquiries/{eid}/targets/approve", "agent",
               {"target_ids": [tb]}), 403, code="agent_forbidden")
    expect(n, "agent answers the requester's clarification via clarify -> 403 agent_forbidden",
           req("POST", f"/api/v1/enquiries/{eid}/clarify", "agent",
               {"answer": "1g, 2 boxes"}), 403, code="agent_forbidden")


def check4_closing_rules():
    n = 4
    body = expect(n, "A creates manual enquiry for the closing rules -> 201",
                  create_enquiry("A", "augmentin 1g x2", 2, "manual", MED_AUGMENTIN_1G), 201)
    eid = _id(body)
    if not eid:
        return abort(n, "no enquiry id")
    expect(n, "A targets achrafieh + baabda -> 201",
           req("POST", f"/api/v1/enquiries/{eid}/targets", "A",
               {"branch_ids": [BRANCH["B"], BRANCH["C"]]}), 201)
    expect(n, "A sends to both targets -> 200",
           req("POST", f"/api/v1/enquiries/{eid}/send", "A"), 200)
    time.sleep(UNDO_WAIT)
    assert_statuses(n, "B and C are sent (delivered)", eid,
                    {BRANCH["B"]: "sent", BRANCH["C"]: "sent"})
    ids = target_ids(eid)
    tb, tc = ids.get(BRANCH["B"]), ids.get(BRANCH["C"])
    if not tb or not tc:
        return abort(n, f"target ids missing: {ids}")

    expect(n, "A closes available at achrafieh before any reply -> 409 conflict",
           req("POST", f"/api/v1/enquiries/{eid}/close", "A",
               {"outcome": "available", "branch_id": BRANCH["B"]}), 409, code="conflict")
    expect(n, "B replies unavailable -> 200",
           req("POST", f"/api/v1/targets/{tb}/reply", "B", {"kind": "unavailable"}), 200)
    expect(n, "A follows up C once -> 200",
           req("POST", f"/api/v1/targets/{tc}/follow-up", "A"), 200)
    time.sleep(UNDO_WAIT + NO_RESPONSE_WAIT)  # follow-up delivered + one full wait
    expect(n, "A marks C no_response -> 200",
           req("POST", f"/api/v1/targets/{tc}/no-response", "A"), 200)
    expect(n, "A closes unavailable while C is no_response -> 409 conflict",
           req("POST", f"/api/v1/enquiries/{eid}/close", "A", {"outcome": "unavailable"}), 409,
           code="conflict")
    expect(n, "A closes unable_to_confirm -> 200",
           req("POST", f"/api/v1/enquiries/{eid}/close", "A",
               {"outcome": "unable_to_confirm"}), 200, check=_outcome_is("unable_to_confirm"))


def check5_undo_after_send():
    n = 5
    body = expect(n, "A creates manual enquiry for the undo check -> 201",
                  create_enquiry("A", "augmentin 1g x2", 2, "manual", MED_AUGMENTIN_1G), 201)
    eid = _id(body)
    if not eid:
        return abort(n, "no enquiry id")
    expect(n, "A targets achrafieh -> 201",
           req("POST", f"/api/v1/enquiries/{eid}/targets", "A", {"branch_ids": [BRANCH["B"]]}), 201)
    tb = target_ids(eid).get(BRANCH["B"])
    if not tb:
        return abort(n, "target id missing")
    expect(n, "A sends to achrafieh -> 200",
           req("POST", f"/api/v1/enquiries/{eid}/send", "A"), 200)
    time.sleep(UNDO_WAIT)

    _, d = detail(eid)
    queued = next((e for e in rows(d, "events") or []
                   if isinstance(e, dict) and e.get("type") == "messages_queued"), None)
    if not queued or not queued.get("id"):
        return abort(n, f"no messages_queued event; events={snippet(rows(d, 'events'))}")
    ev_id = queued["id"]

    expect(n, "A undoes the messages_queued event -> 200",
           req("POST", f"/api/v1/events/{ev_id}/undo", "A"), 200)

    _, d = detail(eid)
    retraction = [m for m in rows(d, "messages") or []
                  if isinstance(m, dict) and m.get("kind") == "retraction" and m.get("target_id") == tb]
    emit(n, bool(retraction),
         "undo queues a retraction message for B's target" if retraction else
         f"expected a retraction message for B's target; messages={snippet(rows(d, 'messages'))}")
    assert_statuses(n, "B's target is cancelled after undo", eid, {BRANCH["B"]: "cancelled"})

    events = rows(d, "events") or []
    undone = [e for e in events if isinstance(e, dict) and e.get("type") == "event_undone"]
    emit(n, bool(undone),
         "event_undone is recorded in the events" if undone else
         f"expected an event_undone event; events={snippet(events)}")
    original = next((e for e in events if isinstance(e, dict) and e.get("id") == ev_id), None)
    undone_at = original.get("undone_at") if original else None
    emit(n, undone_at is not None,
         "original messages_queued event has undone_at set" if undone_at is not None else
         f"expected undone_at on original event {ev_id}, got {undone_at!r}")


def check6_clarification():
    n = 6
    body = expect(n, "A creates agent enquiry 'need augmentin' -> 201",
                  create_enquiry("A", "need augmentin", 1, "agent"), 201)
    eid = _id(body)
    if not eid:
        return abort(n, "no enquiry id")
    AGENT_ENQUIRIES.append(eid)

    expect(n, "agent asks the requester (clarification-request) -> 200",
           req("POST", f"/api/v1/enquiries/{eid}/clarification-request", "agent",
               {"question": "1g or 625mg, and how many?"}), 200)
    assert_field(n, "enquiry is awaiting_clarification", eid, "status", "awaiting_clarification")
    expect(n, "A answers the clarification -> 200",
           req("POST", f"/api/v1/enquiries/{eid}/clarify", "A", {"answer": "1g, 2 boxes"}), 200)
    assert_field(n, "enquiry is open again after clarify", eid, "status", "open")
    assert_field(n, "hitl_count is 1 after one clarification", eid, "hitl_count", 1)
    wake_check(n, "wakeups include trigger clarified", eid, "clarified")


def check7_stats():
    n = 7
    status, body = req("GET", "/api/v1/stats", "A")
    body = expect(n, "A GET stats -> 200", (status, body), 200)
    if body is None:
        return abort(n, "no stats payload")
    stat_rows = rows(body, "enquiries") or rows(body, "stats") or rows(body)
    if stat_rows is None:
        return abort(n, f"no per-enquiry list in stats; body={snippet(body)}")

    emit(n, lookup(body, "totals") is not _MISSING,
         "stats has totals" if lookup(body, "totals") is not _MISSING else
         f"expected a totals key in stats; body={snippet(body)}")

    if not AGENT_ENQUIRIES:
        emit(n, False, "no agent enquiries were recorded by checks 2 and 6, nothing to verify")
    for eid in AGENT_ENQUIRIES:
        row = next((r for r in stat_rows if isinstance(r, dict) and r.get("enquiry_id") == eid), None)
        if row is None:
            emit(n, False, f"agent enquiry {eid} is missing from stats; body={snippet(body)}")
            continue
        missing = [k for k in ("hitl_count", "cost_micros") if k not in row]
        if missing:
            emit(n, False, f"agent enquiry {eid} stats row lacks {missing}; row={snippet(row)}")
        else:
            emit(n, True, f"agent enquiry {eid} in stats with hitl_count={row['hitl_count']} "
                          f"cost_micros={row['cost_micros']}")


def check_extra():
    n = "extra"
    expect(n, "anonymous GET /me (no Authorization) -> 401 unauthorized",
           req("GET", "/api/v1/me", None), 401, code="unauthorized")
    expect(n, "staff token on agent wakeups -> 403 forbidden",
           req("GET", "/api/v1/agent/wakeups", "A"), 403, code="forbidden")


CHECKS = [
    (1, check1_manual_flow),
    (2, check2_agent_api_flow),
    (3, check3_agent_forbidden),
    (4, check4_closing_rules),
    (5, check5_undo_after_send),
    (6, check6_clarification),
    (7, check7_stats),
    ("extra", check_extra),
]


def main(argv=None):
    parser = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.parse_args(argv)

    print(f"Wassit acceptance against {BACKEND_URL} (UNDO_WAIT={UNDO_WAIT:g}s)", flush=True)
    for label, fn in CHECKS:
        try:
            fn()
        except Exception as err:  # one broken check must not hide the others
            emit(label, False, f"unexpected error in {fn.__name__}: {type(err).__name__}: {err}")

    passed = sum(RESULTS)
    failed = len(RESULTS) - passed
    print(f"SUMMARY: {passed} passed, {failed} failed", flush=True)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
