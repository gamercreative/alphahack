"""Live API checks for GET /api/v1/stats and GET /api/v1/enquiries/{id}/stream (SSE).

Each test builds its own enquiry and asserts on that enquiry only (the totals check, case 4,
is the one deliberate exception and reads the first stats page).
"""

import json
import urllib.request


def open_stream(live, enquiry_id, who="a"):
    req = urllib.request.Request(f"{live.base}/api/v1/enquiries/{enquiry_id}/stream",
        headers={"Authorization": live.auth(who), "Accept": "text/event-stream"})
    return urllib.request.urlopen(req, timeout=8)


def next_event(resp):
    """Returns (event_name, data_dict) for the next SSE frame."""
    name, data = None, None
    while True:
        line = resp.readline().decode().rstrip("\n")
        if line.startswith("event:"): name = line[6:].strip()
        elif line.startswith("data:"): data = json.loads(line[5:].strip())
        elif line == "" and name is not None: return name, data


def test_stats_row_for_manual_enquiry(live):
    enquiry = live.new_enquiry(handled_by="manual")
    row = live.stats_row(enquiry["id"])
    assert row["handled_by"] == "manual"
    assert row["hitl_count"] == 0
    assert row["agent_runs"] == 0
    assert row["agent_steps"] == 0
    assert row["cost_micros"] == 0
    assert row["outcome"] == ""


def test_stats_agent_credential_403(live):
    status, body = live.call("GET", "/api/v1/stats", "agent")
    assert status == 403, body
    assert live.code(body) == "agent_forbidden"


def test_stats_anonymous_401(live):
    status, body = live.call("GET", "/api/v1/stats", None)
    assert status == 401, body
    assert live.code(body) == "unauthorized"


def test_stats_totals_match_rows_on_page(live):
    # The only test that reads global totals: it checks the envelope against the rows of the same page.
    status, body = live.call("GET", "/api/v1/stats?limit=200", "a")
    assert status == 200, body
    rows = body["data"]
    totals = body["totals"]
    assert totals["enquiries"] == len(rows)
    assert totals["cost_micros"] == sum(row["cost_micros"] for row in rows)
    assert totals["hitl_count"] == sum(row["hitl_count"] for row in rows)


def test_stream_replays_events_in_order(live):
    enquiry = live.new_enquiry(who="a")
    live.add_targets(enquiry["id"], ("b",), "a")
    resp = open_stream(live, enquiry["id"], "a")
    try:
        assert resp.headers["Content-Type"].startswith("text/event-stream")
        first_name, first = next_event(resp)
        assert first_name == "enquiry_event"
        assert first["type"] == "enquiry_created"
        second_name, second = next_event(resp)
        assert second_name == "enquiry_event"
        assert second["seq"] > first["seq"]
    finally:
        resp.close()


def test_stream_pushes_new_event(live):
    enquiry = live.new_enquiry(who="a")
    resp = open_stream(live, enquiry["id"], "a")
    try:
        name, first = next_event(resp)
        assert name == "enquiry_event" and first["type"] == "enquiry_created"
        status, body = live.call("POST", f"/api/v1/enquiries/{enquiry['id']}/notify", "a", {"body": "hi"})
        assert status == 200, body
        name, pushed = next_event(resp)
        assert name == "enquiry_event"
        assert pushed["type"] == "requester_notified"
    finally:
        resp.close()


def test_stream_other_pharmacy_403(live):
    enquiry = live.new_enquiry(who="a")
    status, body = live.call("GET", f"/api/v1/enquiries/{enquiry['id']}/stream", "d")
    assert status == 403, body
    assert live.code(body) == "forbidden"


def test_stream_agent_403_agent_forbidden(live):
    enquiry = live.new_enquiry(who="a")
    status, body = live.call("GET", f"/api/v1/enquiries/{enquiry['id']}/stream", "agent")
    assert status == 403, body
    assert live.code(body) == "agent_forbidden"


def test_stream_anonymous_401(live):
    enquiry = live.new_enquiry(who="a")
    status, body = live.call("GET", f"/api/v1/enquiries/{enquiry['id']}/stream", None)
    assert status == 401, body
    assert live.code(body) == "unauthorized"


def test_stream_target_pharmacy_can_watch(live):
    enquiry_id, _ = live.sent_enquiry(("b",))
    resp = open_stream(live, enquiry_id, "b")
    try:
        name, first = next_event(resp)
        assert name == "enquiry_event"
        assert first["type"] == "enquiry_created"
    finally:
        resp.close()
