"""Route consistency: Go handler registrations vs backend/openapi.yaml vs CONTRACT.md.

Stdlib + pytest only. No network, no backend, no yaml library: openapi.yaml is
parsed line by line. Path params are normalised (`{id}` and `{medication_id}`
both become `{}`) before any comparison.
"""

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
HANDLERS = ROOT / "backend" / "internal" / "handlers"
OPENAPI = ROOT / "backend" / "openapi.yaml"
CONTRACT = ROOT / "CONTRACT.md"

GUARDED_PREFIXES = (
    "/api/v1/enquiries",
    "/api/v1/targets",
    "/api/v1/messages",
    "/api/v1/events",
    "/api/v1/agent",
    "/api/v1/inbox",
    "/api/v1/stats",
    "/api/v1/me",
)

HANDLE_RE = re.compile(r'HandleFunc\("(GET|POST|PATCH|DELETE|PUT) (/[^"]+)"')
# Only a wrapper call counts as auth: a bare level name without h.auth( is not a guard.
AUTH_RE = re.compile(r"h\.auth\((staffOnly|agentOK|agentOnly)\b")
AUTH_MAP = {"staffOnly": "staff", "agentOK": "either", "agentOnly": "agent"}

OAS_PATH_RE = re.compile(r"^  (/\S*):\s*$")
OAS_METHOD_RE = re.compile(r"^    (get|post|patch|delete|put):\s*$")

CONTRACT_ROW_RE = re.compile(r"^\| (GET|POST|PATCH) ")


def norm(path):
    return re.sub(r"\{[^}]*\}", "{}", path)


def code_routes():
    """{(METHOD, path): auth} for every non-test handler registration."""
    routes = {}
    for go in sorted(HANDLERS.glob("*.go")):
        if go.name.endswith("_test.go"):
            continue
        for line in go.read_text(encoding="utf-8").splitlines():
            if line.lstrip().startswith("//"):  # commented-out registrations do not count
                continue
            m = HANDLE_RE.search(line)
            if not m:
                continue
            a = AUTH_RE.search(line)
            auth = AUTH_MAP[a.group(1)] if a else "none"
            routes[(m.group(1), m.group(2))] = auth
    return routes


def openapi_routes():
    """Set of (METHOD, path) declared under `paths:` in openapi.yaml."""
    out = set()
    path = None
    for line in OPENAPI.read_text(encoding="utf-8").splitlines():
        m = OAS_PATH_RE.match(line)
        if m:
            path = m.group(1)
            continue
        if line and not line[0].isspace():  # a new top-level key ends the paths block
            path = None
            continue
        m = OAS_METHOD_RE.match(line)
        if m and path:
            out.add((m.group(1).upper(), path))
    return out


def contract_routes():
    """List of (METHOD, path, who) from the CONTRACT.md endpoint table."""
    rows = []
    for line in CONTRACT.read_text(encoding="utf-8").splitlines():
        if not CONTRACT_ROW_RE.match(line):
            continue
        # Escaped pipes (`\|`) inside a path must not split the cell.
        cells = line.replace("\\|", "\x00").split("|")
        method, raw = cells[1].split()[0], cells[1].split()[1]
        path = raw.split("?")[0]
        who = cells[2].strip()
        rows.append((method, path, who))
    return rows


def openapi_block(path):
    """Lines of the openapi path item for `path` (up to the next path or top-level key)."""
    lines = OPENAPI.read_text(encoding="utf-8").splitlines()
    key = f"  {path}:"
    assert key in lines, f"openapi.yaml has no path key {key!r}"
    start = lines.index(key)
    block = []
    for line in lines[start + 1:]:
        if re.match(r"^  /\S", line) or (line and not line[0].isspace()):
            break
        block.append(line)
    return block


def expected_auth(who):
    w = who.lower()
    if w == "public":
        return "none"
    if "agent only" in w or w == "agent":
        return "agent"
    if w.startswith("staff") and "agent" in w:
        return "either"
    if w.startswith("staff"):
        return "staff"
    return None  # unknown Who wording: reported as a mismatch


# ---------------------------------------------------------------- 1


def test_code_and_openapi_routes_match():
    """No hardcoded count: the code and openapi.yaml must list the same (method, path) set."""
    code = {(m, norm(p)) for (m, p) in code_routes()}
    spec = {(m, norm(p)) for (m, p) in openapi_routes()}
    assert code, "no HandleFunc registrations parsed from backend/internal/handlers/*.go"
    assert spec, "no path+method parsed from backend/openapi.yaml"
    only_code = sorted(f"{m} {p}" for m, p in code - spec)
    only_spec = sorted(f"{m} {p}" for m, p in spec - code)
    assert not only_code and not only_spec, (
        f"routes differ ({len(code)} in code, {len(spec)} in openapi.yaml)\n"
        f"  in code but not in openapi.yaml ({len(only_code)}):\n    " + "\n    ".join(only_code or ["-"]) + "\n"
        f"  in openapi.yaml but not in code ({len(only_spec)}):\n    " + "\n    ".join(only_spec or ["-"])
    )


# ---------------------------------------------------------------- 2


def test_every_code_route_is_in_openapi():
    code = {(m, norm(p)) for (m, p) in code_routes()}
    spec = {(m, norm(p)) for (m, p) in openapi_routes()}
    missing = sorted(code - spec)
    assert code - spec == set(), f"code routes missing from openapi.yaml: {missing}"


# ---------------------------------------------------------------- 3


def test_every_openapi_route_is_in_code():
    code = {(m, norm(p)) for (m, p) in code_routes()}
    spec = {(m, norm(p)) for (m, p) in openapi_routes()}
    extra = sorted(spec - code)
    assert spec - code == set(), f"openapi.yaml routes with no handler: {extra}"


# ---------------------------------------------------------------- 4


def test_every_contract_route_exists_in_code():
    code = {(m, norm(p)) for (m, p) in code_routes()}
    contract = {(m, norm(p)) for (m, p, _who) in contract_routes()}
    assert contract, "CONTRACT.md endpoint table parsed to zero rows"
    missing = sorted(contract - code)
    assert contract - code == set(), f"CONTRACT.md routes with no handler: {missing}"


# ---------------------------------------------------------------- 5


def test_contract_who_matches_code_auth():
    code = {(m, norm(p)): a for (m, p), a in code_routes().items()}
    rows = contract_routes()
    assert rows, "CONTRACT.md endpoint table parsed to zero rows"

    mismatches = []
    for method, path, who in rows:
        key = (method, norm(path))
        want = expected_auth(who)
        if key not in code:
            mismatches.append(f"{method} {path}: no code route")
        elif want is None:
            mismatches.append(f"{method} {path}: unknown Who {who!r}")
        elif code[key] != want:
            mismatches.append(f"{method} {path}: Who {who!r} expects {want!r}, code has {code[key]!r}")
    assert mismatches == [], "CONTRACT.md Who disagrees with code auth:\n  " + "\n  ".join(mismatches)


# ---------------------------------------------------------------- 6


def test_enquiry_routes_all_have_an_auth_wrapper():
    # Segment-boundary match: "/api/v1/me" must not capture "/api/v1/medications" (public).
    def guarded_path(p):
        return any(p == g or p.startswith(g + "/") for g in GUARDED_PREFIXES)

    guarded = {k: a for k, a in code_routes().items() if guarded_path(k[1])}
    assert guarded, "no guarded-prefix routes found in code"
    unwrapped = sorted(f"{m} {p}" for (m, p), a in guarded.items() if a == "none")
    assert unwrapped == [], f"guarded-prefix routes with no auth wrapper: {unwrapped}"


# ---------------------------------------------------------------- 7


def test_openapi_declares_security_for_guarded_routes():
    samples = ["/api/v1/me", "/api/v1/agent/wakeups", "/api/v1/stats"]
    missing = [p for p in samples if not any(re.match(r"^\s+security:", l) for l in openapi_block(p))]
    assert missing == [], f"openapi.yaml path items with no security: line: {missing}"
