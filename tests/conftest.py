"""Shared fixtures for the live Wassit tests (tests/api, tests/e2e). Owned by the lead: do not edit.

The live tests talk to a running local backend over HTTP. They are skipped unless WASSIT_LIVE_URL
is set, so the normal suite (`pytest -q tests`) stays green without a server.

Start the test stack (see docs/TEST_REPORT_2026-11-15.md), then:
    WASSIT_LIVE_URL=http://localhost:8114 agent/.venv/bin/python -m pytest -q tests

Env:
    WASSIT_LIVE_URL      backend base URL (no default: unset means skip)
    AGENT_TOKEN          agent credential (default dev-agent-token, the documented local dev value)
    WASSIT_UNDO_WINDOW   the server's UNDO_WINDOW in seconds (default 2)
    WASSIT_WAIT_UNIT     the server's WAIT_UNIT in seconds (default 0.2)
    WASSIT_PRICE_IN      the server's AGENT_PRICE_IN_PER_MTOK (default 0.10)
    WASSIT_PRICE_OUT     the server's AGENT_PRICE_OUT_PER_MTOK (default 0.50)

Rules for tests that use the `live` fixture:
- every test creates its own enquiry (live.new_enquiry / live.sent_enquiry) and asserts on that
  enquiry only: never on global totals, never on another test's data;
- assert the exact HTTP status and the exact error code (live.code(body)).
"""

import json
import os
import time
import urllib.error
import urllib.request
import uuid

import pytest

PIN = "1234"  # seeded demo PIN (migration 0005), public demo data
STAFF = {"a": "a@wassit.demo", "b": "b@wassit.demo", "c": "c@wassit.demo",
         "d": "d@wassit.demo", "e": "e@wassit.demo"}
BRANCH = {"a": "branch-hamra", "b": "branch-achrafieh", "c": "branch-baabda",
          "d": "branch-verdun", "e": "branch-byblos"}
MED_1G = "med-augmentin-1g"
MED_625 = "med-augmentin-625"


class KnownBugFailure(AssertionError):
    """Raised only by the pinned bug assertion in an xfail test so setup failures fail loudly."""


def assert_known_bug(condition, message=""):
    """Assert the target invariant of a pinned xfail test; raises KnownBugFailure on failure."""
    if not condition:
        raise KnownBugFailure(message)


class Live:
    """A tiny client for the Wassit API. `who` is None (anonymous), "agent", or a staff letter a..e."""

    BRANCH = BRANCH
    STAFF = STAFF
    MED_1G = MED_1G
    MED_625 = MED_625

    def __init__(self, base):
        self.base = base.rstrip("/")
        self.agent_token = os.environ.get("AGENT_TOKEN", "dev-agent-token")
        self.undo_window = float(os.environ.get("WASSIT_UNDO_WINDOW", "2"))
        self.wait_unit = float(os.environ.get("WASSIT_WAIT_UNIT", "0.2"))
        self.price_in = float(os.environ.get("WASSIT_PRICE_IN", "0.10"))
        self.price_out = float(os.environ.get("WASSIT_PRICE_OUT", "0.50"))
        self._tokens = {}
        self._inbox_canaries = {}  # staff letter -> delivered uncancelled enquiry_id in that branch's inbox
        self._agent_enquiries = []  # agent enquiries created by the running test, cancelled at teardown

    # ------------------------------------------------------------------ HTTP

    def raw(self, method, path, authorization=None, body=None, data=None, headers=None):
        """Lowest level: explicit Authorization header value. Returns (status, parsed_json_or_{"raw": text}).

        body: a JSON-serialisable value. data: raw bytes to send instead (for malformed-JSON tests).
        """
        hdrs = {"Accept": "application/json"}
        if authorization is not None:
            hdrs["Authorization"] = authorization
        payload = None
        if data is not None:
            payload = data
            hdrs["Content-Type"] = "application/json"
        elif body is not None:
            payload = json.dumps(body).encode("utf-8")
            hdrs["Content-Type"] = "application/json"
        if headers:
            hdrs.update(headers)
        request = urllib.request.Request(self.base + path, data=payload, method=method, headers=hdrs)
        try:
            with urllib.request.urlopen(request, timeout=15) as resp:
                return resp.status, self._parse(resp.read())
        except urllib.error.HTTPError as err:
            return err.code, self._parse(err.read())

    @staticmethod
    def _parse(raw):
        if not raw:
            return {}
        try:
            return json.loads(raw)
        except ValueError:
            return {"raw": raw.decode("utf-8", "replace")}

    def token(self, who):
        """The raw staff session token for a letter a..e (logs in once per session)."""
        if who not in self._tokens:
            status, body = self.raw("POST", "/api/v1/auth/login", body={"email": STAFF[who], "pin": PIN})
            assert status == 200, f"login {who} failed: {status} {body}"
            self._tokens[who] = body["token"]
        return self._tokens[who]

    def auth(self, who):
        """The Authorization header value for who: None, "Agent <token>" or "Bearer <token>"."""
        if who is None:
            return None
        if who == "agent":
            return f"Agent {self.agent_token}"
        return f"Bearer {self.token(who)}"

    def call(self, method, path, who=None, body=None):
        """Call the API as `who`. Returns (status, json)."""
        return self.raw(method, path, authorization=self.auth(who), body=body)

    @staticmethod
    def code(body):
        """The error code of an error response, or "" when the body is not an error envelope."""
        err = body.get("error") if isinstance(body, dict) else None
        return err.get("code", "") if isinstance(err, dict) else ""

    # ------------------------------------------------------------------ builders (they assert success)

    def new_enquiry(self, who="a", handled_by="manual", medication_id=MED_1G, quantity=2, raw_request=None):
        """Create an enquiry as staff `who`. Returns the enquiry dict (has "id", "status", ...)."""
        body = {"raw_request": raw_request or f"test {uuid.uuid4().hex[:10]} augmentin 1g x2",
                "quantity": quantity, "handled_by": handled_by}
        if medication_id is not None:
            body["medication_id"] = medication_id
        status, enquiry = self.call("POST", "/api/v1/enquiries", who, body)
        assert status == 201, f"create enquiry failed: {status} {enquiry}"
        if handled_by == "agent":
            self._agent_enquiries.append((enquiry["id"], who))
        return enquiry

    def detail(self, enquiry_id, who="a"):
        """GET the enquiry detail: {"enquiry", "medication", "targets", "messages", "events", "runs", "hitl_count"}."""
        status, body = self.call("GET", f"/api/v1/enquiries/{enquiry_id}", who)
        assert status == 200, f"get enquiry failed: {status} {body}"
        return body

    def add_targets(self, enquiry_id, branches=("b", "c"), who="a"):
        """Add target pharmacies by staff letter. Returns {letter: target dict}. Staff -> approved, agent -> proposed."""
        status, body = self.call("POST", f"/api/v1/enquiries/{enquiry_id}/targets", who,
                                 {"branch_ids": [BRANCH[b] for b in branches]})
        assert status == 201, f"add targets failed: {status} {body}"
        by_branch = {t["branch_id"]: t for t in body["data"]}
        return {b: by_branch[BRANCH[b]] for b in branches}

    def send(self, enquiry_id, who="a"):
        """Queue one message per approved target. Returns the list of queued message dicts."""
        status, body = self.call("POST", f"/api/v1/enquiries/{enquiry_id}/send", who, {})
        assert status == 200, f"send failed: {status} {body}"
        return body["data"]

    def wait_sent(self, enquiry_id, who="a", timeout=None):
        """Poll until no message of the enquiry is still queued (the dispatcher ticks every 2s). Returns the detail."""
        deadline = time.monotonic() + (timeout if timeout is not None else self.undo_window + 8)
        while True:
            detail = self.detail(enquiry_id, who)
            if not any(m["status"] == "queued" for m in detail["messages"]):
                return detail
            assert time.monotonic() < deadline, f"messages still queued after the timeout: {detail['messages']}"
            time.sleep(0.25)

    def sent_enquiry(self, branches=("b", "c"), who="a", handled_by="manual"):
        """New manual enquiry + targets + send + wait until delivered. Returns (enquiry_id, {letter: target dict})."""
        enquiry = self.new_enquiry(who=who, handled_by=handled_by)
        self.add_targets(enquiry["id"], branches, who)
        self.send(enquiry["id"], who)
        self.wait_sent(enquiry["id"], who)
        return enquiry["id"], {b: self.target(enquiry["id"], b, who) for b in branches}

    def reply(self, target_id, who, kind, quantity=None, text=None):
        """Reply to a target as staff `who`. Returns (status, body): it does NOT assert, so tests can check errors."""
        body = {"kind": kind}
        if quantity is not None:
            body["quantity"] = quantity
        if text is not None:
            body["text"] = text
        return self.call("POST", f"/api/v1/targets/{target_id}/reply", who, body)

    def close(self, enquiry_id, outcome, who="a", branch=None):
        """Close an enquiry. `branch` is a staff letter. Returns (status, body): it does NOT assert."""
        body = {"outcome": outcome}
        if branch is not None:
            body["branch_id"] = BRANCH[branch]
        return self.call("POST", f"/api/v1/enquiries/{enquiry_id}/close", who, body)

    # ------------------------------------------------------------------ readers

    def target(self, enquiry_id, letter, who="a"):
        """The current target dict of the enquiry for a staff letter (fresh read)."""
        for t in self.detail(enquiry_id, who)["targets"]:
            if t["branch_id"] == BRANCH[letter]:
                return t
        raise AssertionError(f"enquiry {enquiry_id} has no target for {letter}")

    def messages(self, enquiry_id, who="a", kind=None):
        """Messages of the enquiry, oldest first; optionally only one kind (enquiry|follow_up|retraction|notification)."""
        msgs = sorted(self.detail(enquiry_id, who)["messages"], key=lambda m: m["created_at"])
        return [m for m in msgs if kind is None or m["kind"] == kind]

    def events(self, enquiry_id, who="a", type=None):
        """Events of the enquiry ordered by seq; optionally only one type."""
        evs = sorted(self.detail(enquiry_id, who)["events"], key=lambda e: e["seq"])
        return [e for e in evs if type is None or e["type"] == type]

    def last_event(self, enquiry_id, type, who="a"):
        """The newest event of that type; fails the test when there is none."""
        evs = self.events(enquiry_id, who, type)
        assert evs, f"no {type} event on enquiry {enquiry_id}"
        return evs[-1]

    def stats_row(self, enquiry_id, who="a"):
        """The /api/v1/stats row of one enquiry (stats are newest first, 200 per page)."""
        status, body = self.call("GET", "/api/v1/stats?limit=200", who)
        assert status == 200, f"stats failed: {status} {body}"
        for row in body["data"]:
            if row["enquiry_id"] == enquiry_id:
                return row
        raise AssertionError(f"enquiry {enquiry_id} not in the first stats page")

    def _all_rows(self, path, who):
        """Every row of a paginated list (pages of 200), so a "not in the list" check cannot be fooled by paging."""
        rows, offset, sep = [], 0, "&" if "?" in path else "?"
        while True:
            status, body = self.call("GET", f"{path}{sep}limit=200&offset={offset}", who)
            assert status == 200, f"GET {path} failed: {status} {body}"
            rows.extend(body["data"])
            if len(body["data"]) < 200:
                return rows
            offset += 200

    def inbox_ids(self, who):
        """Set of enquiry ids in the whole inbox of staff `who` (every page)."""
        rows = self._all_rows("/api/v1/inbox", who)
        assert all("enquiry_id" in row for row in rows), "inbox rows must carry enquiry_id"
        return {row["enquiry_id"] for row in rows}

    def canary_in_inbox(self, who):
        """Return the id of a live, uncancelled delivered enquiry sitting in `who`'s inbox."""
        canary_id = self._inbox_canaries.get(who)
        if canary_id is None or canary_id not in self.inbox_ids(who):
            sender = "a" if who != "a" else "b"
            canary_id, _ = self.sent_enquiry((who,), who=sender)
            self._inbox_canaries[who] = canary_id
        assert canary_id in self.inbox_ids(who), f"canary {canary_id} missing from inbox of {who}"
        return canary_id

    def enquiry_ids(self, who, scope="outgoing"):
        """Set of enquiry ids in the whole outgoing or incoming list of staff `who` (every page)."""
        rows = self._all_rows(f"/api/v1/enquiries?scope={scope}", who)
        assert all("id" in row for row in rows), "enquiry list rows must carry id"
        return {row["id"] for row in rows}

    def wake_triggers(self, enquiry_id):
        """The agent wake-up triggers currently pending for one enquiry, e.g. ["created"] or [].

        The backend returns only the 10 oldest pending wake-ups, so this fails the test when the list is
        full: a wake-up could then be hidden and an empty answer would prove nothing.
        """
        status, body = self.call("GET", "/api/v1/agent/wakeups", "agent")
        assert status == 200, f"wakeups failed: {status} {body}"
        assert len(body["data"]) < 10, "10 pending wake-ups: the list is capped, this check would be blind"
        return [w["trigger"] for w in body["data"] if w["enquiry_id"] == enquiry_id]


@pytest.fixture(scope="session")
def live():
    """The live API client. Skips the test when WASSIT_LIVE_URL is not set; fails when the server is down."""
    base = os.environ.get("WASSIT_LIVE_URL", "").strip()
    if not base:
        pytest.skip("WASSIT_LIVE_URL not set: live Wassit tests need a running local backend")
    client = Live(base)
    try:
        status, _ = client.raw("GET", "/healthz")
    except OSError as err:
        pytest.fail(f"WASSIT_LIVE_URL={base} is set but the backend is not reachable: {err}")
    assert status == 200, f"backend at {base} is not healthy: {status}"
    # GET /agent/wakeups returns only the 10 oldest pending wake-ups. A backlog left by another tool
    # (a stopped runner, acceptance.py) would hide the wake-ups these tests create and assert on.
    status, body = client.call("GET", "/api/v1/agent/wakeups", "agent")
    assert status == 200, f"agent credential rejected: {status} {body}"
    if len(body["data"]) >= 10:
        pytest.fail("the backend has 10 or more pending agent wake-ups; the live tests need a database "
                    "without a wake-up backlog (use the dedicated test stack, or let a runner drain it)")
    return client


@pytest.fixture(autouse=True)
def _cancel_agent_enquiries(request):
    """After each live test, cancel the agent enquiries it created, so their pending wake-ups do not
    pile up: the wake-up list is capped at the 10 oldest, and a backlog would hide later tests' wakes."""
    yield
    if "live" not in request.fixturenames:
        return
    client = request.getfixturevalue("live")
    while client._agent_enquiries:
        enquiry_id, who = client._agent_enquiries.pop()
        status, body = client.close(enquiry_id, "cancelled", who=who)
        assert status in (200, 409), f"teardown could not cancel {enquiry_id}: {status} {body}"  # 409 = already closed
