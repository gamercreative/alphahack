"""Auth contract checks (CONTRACT.md "Auth"): staff login, bearer tokens, agent credential, error envelope.

Each test is one attack or one contract case. Exact status and exact error code are asserted.
"""

import re

TOKEN_RE = re.compile(r"^[0-9a-f]{64}$")


def test_login_returns_token_staff_and_branch(live):
    status, body = live.raw("POST", "/api/v1/auth/login", body={"email": "a@wassit.demo", "pin": "1234"})
    assert status == 200, body
    assert TOKEN_RE.match(body["token"]), body["token"]
    assert body["staff"]["email"] == "a@wassit.demo"
    assert body["branch"]["id"] == "branch-hamra"


def test_login_wrong_pin_401(live):
    status, body = live.raw("POST", "/api/v1/auth/login", body={"email": "a@wassit.demo", "pin": "9999"})
    assert status == 401, body
    assert live.code(body) == "unauthorized", body


def test_login_unknown_email_401(live):
    status, body = live.raw("POST", "/api/v1/auth/login", body={"email": "zz@wassit.demo", "pin": "1234"})
    assert status == 401, body
    assert live.code(body) == "unauthorized", body


def test_login_missing_pin_400(live):
    status, body = live.raw("POST", "/api/v1/auth/login", body={"email": "a@wassit.demo"})
    assert status == 400, body
    assert live.code(body) == "validation_failed", body


def test_two_logins_give_different_tokens_both_valid(live):
    s1, b1 = live.raw("POST", "/api/v1/auth/login", body={"email": "a@wassit.demo", "pin": "1234"})
    s2, b2 = live.raw("POST", "/api/v1/auth/login", body={"email": "a@wassit.demo", "pin": "1234"})
    assert s1 == 200 and s2 == 200, (b1, b2)
    assert b1["token"] != b2["token"]
    status1, me1 = live.raw("GET", "/api/v1/me", authorization=f"Bearer {b1['token']}")
    status2, me2 = live.raw("GET", "/api/v1/me", authorization=f"Bearer {b2['token']}")
    assert status1 == 200, me1
    assert status2 == 200, me2


def test_me_returns_own_staff_and_branch(live):
    status, body = live.call("GET", "/api/v1/me", "b")
    assert status == 200, body
    assert body["staff"]["id"] == "staff-achrafieh"
    assert body["branch"]["id"] == "branch-achrafieh"


def test_me_without_header_401(live):
    status, body = live.call("GET", "/api/v1/me", None)
    assert status == 401, body
    assert live.code(body) == "unauthorized", body


def test_me_junk_bearer_401(live):
    status, body = live.raw("GET", "/api/v1/me", authorization="Bearer junk")
    assert status == 401, body
    assert live.code(body) == "unauthorized", body


def test_me_basic_scheme_401(live):
    status, body = live.raw("GET", "/api/v1/me", authorization="Basic abc")
    assert status == 401, body
    assert live.code(body) == "unauthorized", body


def test_me_agent_credential_403_agent_forbidden(live):
    status, body = live.call("GET", "/api/v1/me", "agent")
    assert status == 403, body
    assert live.code(body) == "agent_forbidden", body


def test_agent_route_staff_token_403_forbidden(live):
    status, body = live.call("GET", "/api/v1/agent/wakeups", "a")
    assert status == 403, body
    assert live.code(body) == "forbidden", body


def test_agent_route_wrong_token_401(live):
    status, body = live.raw("GET", "/api/v1/agent/wakeups", authorization="Agent wrong")
    assert status == 401, body
    assert live.code(body) == "unauthorized", body


def test_agent_route_without_header_401(live):
    status, body = live.call("GET", "/api/v1/agent/wakeups", None)
    assert status == 401, body
    assert live.code(body) == "unauthorized", body


def test_agent_route_valid_token_200(live):
    status, body = live.call("GET", "/api/v1/agent/wakeups", "agent")
    assert status == 200, body
    assert isinstance(body.get("data"), list), body


def test_error_envelope_shape(live):
    status, body = live.call("GET", "/api/v1/me", None)
    assert status == 401, body
    assert isinstance(body, dict) and isinstance(body.get("error"), dict), body
    err = body["error"]
    assert set(err.keys()) >= {"code", "message", "request_id"}, err
    for key in ("code", "message", "request_id"):
        assert isinstance(err[key], str) and err[key], (key, err)
