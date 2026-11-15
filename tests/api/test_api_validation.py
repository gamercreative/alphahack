"""Input validation and routing attacks on the enquiry API (tester-owned).

Every case asserts the exact HTTP status and the exact error code. Each test creates its own data
and asserts only on that data.
"""

import pytest

BASE_BODY = {"raw_request": "x", "quantity": 1, "handled_by": "manual"}
# Just over the 1 MiB body limit (1,048,576 bytes). The server answers 400 and closes the connection
# without reading the rest, so a much larger upload can see a connection reset instead of the 400
# (finding F-03 in docs/TEST_REPORT_2026-11-15.md). 1 KiB over the limit fits in the socket buffer,
# which makes the 400 deterministic.
OVERSIZED_TARGET = 1_048_576 + 1_024


def _body(**changes):
    body = dict(BASE_BODY)
    body.update(changes)
    return body


def _without(key):
    return {k: v for k, v in BASE_BODY.items() if k != key}


# A. POST /api/v1/enquiries with one bad field each (as staff "a").
@pytest.mark.parametrize(
    "body, status, code",
    [
        pytest.param(_body(quantity=0), 400, "validation_failed", id="quantity_zero"),
        pytest.param(_body(quantity=-1), 400, "validation_failed", id="quantity_negative"),
        pytest.param(_body(quantity=1001), 400, "validation_failed", id="quantity_over_1000"),
        pytest.param(_body(quantity="2"), 400, "bad_request", id="quantity_string"),
        pytest.param(_without("raw_request"), 400, "validation_failed", id="raw_request_missing"),
        pytest.param(_body(raw_request="   "), 400, "validation_failed", id="raw_request_blank"),
        pytest.param(_body(raw_request="x" * 1001), 400, "validation_failed", id="raw_request_1001_chars"),
        pytest.param(_body(handled_by="robot"), 400, "validation_failed", id="handled_by_robot"),
        pytest.param(_body(medication_id="med-nope"), 404, "not_found", id="medication_unknown"),
        pytest.param(_body(extra=1), 400, "bad_request", id="unknown_field"),
    ],
)
def test_create_enquiry_rejects(live, body, status, code):
    got_status, got_body = live.call("POST", "/api/v1/enquiries", "a", body)
    assert got_status == status, got_body
    assert live.code(got_body) == code, got_body


# B. Malformed, empty and oversized raw bodies (as staff "a").
_PREFIX = b'{"raw_request":"'
_SUFFIX = b'","quantity":1,"handled_by":"manual"}'
_OVERSIZED = _PREFIX + b"a" * (OVERSIZED_TARGET - len(_PREFIX) - len(_SUFFIX)) + _SUFFIX
assert len(_OVERSIZED) == OVERSIZED_TARGET


@pytest.mark.parametrize(
    "data",
    [
        pytest.param(b'{"raw_request": ', id="malformed_json"),
        pytest.param(b"", id="empty_body"),
        pytest.param(_OVERSIZED, id="oversized_body"),
    ],
)
def test_create_enquiry_rejects_bad_bodies(live, data):
    status, body = live.raw("POST", "/api/v1/enquiries", authorization=live.auth("a"), data=data)
    assert status == 400, body
    assert live.code(body) == "bad_request", body


# C. GET /api/v1/enquiries with invalid query parameters (as staff "a").
@pytest.mark.parametrize(
    "query, code",
    [
        pytest.param("limit=abc", "bad_request", id="limit_abc"),
        pytest.param("limit=-1", "bad_request", id="limit_negative"),
        pytest.param("offset=-1", "bad_request", id="offset_negative"),
        pytest.param("scope=sideways", "validation_failed", id="scope_sideways"),
    ],
)
def test_list_rejects(live, query, code):
    status, body = live.call("GET", f"/api/v1/enquiries?{query}", "a")
    assert status == 400, body
    assert live.code(body) == code, body


# D. Routing: unknown ids, wrong methods and unknown paths (as staff "a").
@pytest.mark.parametrize(
    "method, path, status, code",
    [
        pytest.param("GET", "/api/v1/enquiries/nope", 404, "not_found", id="unknown_enquiry_id"),
        pytest.param("DELETE", "/api/v1/enquiries", 405, "method_not_allowed", id="delete_collection"),
        pytest.param("GET", "/api/v1/nope", 404, "not_found", id="unknown_route"),
    ],
)
def test_routing(live, method, path, status, code):
    got_status, body = live.call(method, path, "a")
    assert got_status == status, body
    assert live.code(body) == code, body


# E. Sub-routes of a fresh enquiry with invalid input (as staff "a").
@pytest.mark.parametrize(
    "method, suffix, body, status, code",
    [
        pytest.param("GET", "/candidates?limit=-1", None, 400, "bad_request", id="candidates_limit_negative"),
        pytest.param("GET", "/candidates?radius_km=abc", None, 400, "bad_request", id="candidates_radius_abc"),
        pytest.param("POST", "/close", {"outcome": "bogus"}, 400, "validation_failed", id="close_bogus_outcome"),
        pytest.param("POST", "/close", {"outcome": "available"}, 400, "validation_failed",
                     id="close_available_without_branch"),
    ],
)
def test_enquiry_subroutes_reject(live, method, suffix, body, status, code):
    enquiry_id = live.new_enquiry()["id"]
    got_status, got_body = live.call(method, f"/api/v1/enquiries/{enquiry_id}{suffix}", "a", body)
    assert got_status == status, got_body
    assert live.code(got_body) == code, got_body


# F. A rejected create must not write anything (as staff "a").
def test_rejected_create_writes_nothing(live):
    status, before = live.call("GET", "/api/v1/enquiries?limit=200", "a")
    assert status == 200, before
    ids_before = {row["id"] for row in before["data"]}

    status, body = live.call("POST", "/api/v1/enquiries", "a", _body(quantity=0))
    assert status == 400, body
    assert live.code(body) == "validation_failed", body

    status, after = live.call("GET", "/api/v1/enquiries?limit=200", "a")
    assert status == 200, after
    ids_after = {row["id"] for row in after["data"]}
    assert len(ids_after) == len(ids_before), (len(ids_before), len(ids_after))
    assert ids_after == ids_before
