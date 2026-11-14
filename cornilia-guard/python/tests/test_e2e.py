"""End-to-end: real Go gateway binary + real signed client + guard."""
import os
import secrets
import socket
import subprocess
import time
import urllib.request
from pathlib import Path

import pytest

from cornilia_guard import GatewayClient, GatewayError, GuardConfig, ToolGuard
from example_tools import TOOLS

GO_DIR = Path(__file__).resolve().parents[2] / "go"
TOKEN_KEY, SERVICE_KEY = secrets.token_bytes(32), secrets.token_bytes(32)


def _free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


@pytest.fixture(scope="module")
def server(tmp_path_factory):
    binary = tmp_path_factory.mktemp("bin") / "demo"
    subprocess.run(["go", "build", "-o", str(binary), "./cmd/demo"], cwd=GO_DIR, check=True)
    env = {**os.environ, "CORNILIA_TOKEN_KEY": TOKEN_KEY.hex(), "CORNILIA_SERVICE_KEY": SERVICE_KEY.hex()}
    port = _free_port()
    proc = subprocess.Popen([str(binary), "serve", "-addr", f"127.0.0.1:{port}"], env=env,
                            stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
    for _ in range(50):
        try:
            socket.create_connection(("127.0.0.1", port), 0.1).close()
            break
        except OSError:
            time.sleep(0.1)

    def mint(user="u_42", scopes="events:read,bookings:read,bookings:write", budget=20):
        return subprocess.run([str(binary), "mint", "-user", user, "-scopes", scopes, "-budget", str(budget)],
                              env=env, check=True, capture_output=True, text=True).stdout.strip()

    yield f"http://127.0.0.1:{port}", mint
    proc.terminate()
    audit = proc.stderr.read()
    print("\n--- gateway audit log ---\n" + audit[-3000:])


def client(server, **kw):
    url, mint = server
    return GatewayClient(url, SERVICE_KEY, mint(**kw))


def test_injection_scenario(server):
    """Model reads an event whose description tells it to book 10 seats."""
    approvals = []
    g = ToolGuard(TOOLS, client(server), approver=lambda t, a, r: approvals.append(a) or True)

    r = g.execute("search_events", {"query": "jazz"})
    assert r.ok and r.flagged  # injection text detected and labelled

    # Model obeys the injection with 10 seats: schema says max 6.
    assert not g.execute("create_booking", {"event_id": "ev_2", "seats": 10}).ok
    # Within schema but more seats than exist: Go returns a clean conflict.
    r = g.execute("create_booking", {"event_id": "ev_2", "seats": 3})
    assert "conflict" in r.content
    # Legit booking goes through only via the approver.
    assert g.execute("create_booking", {"event_id": "ev_2", "seats": 1}).ok
    assert len(approvals) == 2


def test_retry_cannot_double_book(server):
    g = ToolGuard(TOOLS, client(server, user="u_7"), approver=lambda *a: True)
    g.execute("create_booking", {"event_id": "ev_1", "seats": 2})
    g.execute("create_booking", {"event_id": "ev_1", "seats": 2})  # loop/retry, same key
    r = g.execute("list_my_bookings", {})
    assert r.content.count('"event_id"') == 1


def test_bypassing_python_guard_still_blocked(server):
    """Pretend Cornilia's guard is buggy and sends raw requests."""
    c = client(server, scopes="events:read")
    with pytest.raises(GatewayError) as e:
        c.call("bookings.list_mine", {})                       # not in token scopes
    assert e.value.status == 403
    with pytest.raises(GatewayError) as e:
        c.call("events.search", {"query": "x", "user_id": "u_1"})  # smuggled field
    assert e.value.status == 400
    with pytest.raises(GatewayError) as e:
        c.call("db.exec", {"sql": "DROP TABLE events"})         # no such op
    assert e.value.status == 404


def test_users_are_isolated(server):
    a = ToolGuard(TOOLS, client(server, user="u_alice"), approver=lambda *x: True)
    b = ToolGuard(TOOLS, client(server, user="u_bob"))
    a.execute("create_booking", {"event_id": "ev_3", "seats": 1})
    assert '"bookings": []' in b.execute("list_my_bookings", {}).content


def test_server_budget(server):
    c = client(server, budget=2)
    c.call("events.search", {"query": "a"})
    c.call("events.search", {"query": "b"})
    with pytest.raises(GatewayError) as e:
        c.call("events.search", {"query": "c"})
    assert e.value.status == 429


def test_wrong_service_key_rejected(server):
    url, mint = server
    c = GatewayClient(url, secrets.token_bytes(32), mint())
    with pytest.raises(GatewayError) as e:
        c.call("events.search", {"query": "a"})
    assert e.value.status == 401


def test_agent_token_rejected_on_normal_api(server):
    url, mint = server
    req = urllib.request.Request(url + "/api/agent/run", method="POST",
                                 headers={"Authorization": "Agent " + mint()})
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with pytest.raises(urllib.error.HTTPError) as e:
        opener.open(req)
    assert e.value.code == 403
