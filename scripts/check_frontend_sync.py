#!/usr/bin/env python3
"""Checkpoint: is web/ (DOSYN frontend) in sync with backend/ (Wassit Go API)?

1. Routes: every apiCall(METHOD, path), apiAudio(METHOD, path) and EventSource URL in
   web/app.js must match a route registered with mux.HandleFunc in backend/internal/handlers.
2. Fields: every snake_case field the UI reads off an API object must exist as a
   `json:"..."` tag in backend Go code (or be listed in FRONTEND_ONLY below).
3. Live (unless --static): log in as the demo pharmacies and call every GET the
   frontend makes against BACKEND_URL (default http://localhost:8113), expecting 2xx.
   Also POST /api/v1/speech once (no data is written): audio bytes, or 409 when speech is off.

Usage: python3.13 scripts/check_frontend_sync.py [--static] [--backend URL]
Exit code 0 = in sync, 1 = gaps found. Read-only: it never writes data.
"""
from __future__ import annotations

import argparse
import json
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
APP_JS = ROOT / "web" / "app.js"
HANDLERS = ROOT / "backend" / "internal"

# Fields the UI computes itself or reads from its own demo data, not from the API.
FRONTEND_ONLY = {"expiry_date"}

CALL_RE = re.compile(r"(apiCall|apiAudio|apiUpload)\(\s*'([A-Z]+)'\s*,\s*([`'])(.*?)\3", re.S)
ROUTE_RE = re.compile(r'mux\.HandleFunc\("([A-Z]+) ([^"]+)"')
FIELD_RE = re.compile(
    r"\b(?:detail|enquiry|enq|e|t|tgt|target|m|msg|ev|evt|event|run|r|row|res|me|item|b|branch|"
    r"med|c|cand|s|stat|totals|o|offer)\??\.([a-z]+_[a-z_]+)\b"
)
JSON_TAG_RE = re.compile(r'json:"([a-z_]+)')


def frontend_calls(src: str) -> list[tuple[int, str, str]]:
    out = []
    for m in CALL_RE.finditer(src):
        line = src.count("\n", 0, m.start()) + 1
        out.append((line, m.group(2), m.group(4)))
    for m in re.finditer(r"\$\{API_BASE_URL\}(/api/v1/[^`?]+/stream)", src):
        out.append((src.count("\n", 0, m.start()) + 1, "GET", m.group(1)))
    return out


def to_template(path: str) -> str:
    path = path.split("?", 1)[0]
    return re.sub(r"\$\{[^}]+\}", "{}", path)


def backend_routes() -> set[tuple[str, str]]:
    routes = set()
    for f in HANDLERS.rglob("*.go"):
        if f.name.endswith("_test.go"):
            continue
        for m in ROUTE_RE.finditer(f.read_text()):
            routes.add((m.group(1), re.sub(r"\{[^}]+\}", "{}", m.group(2))))
    return routes


def backend_json_fields() -> set[str]:
    fields = set()
    for f in HANDLERS.rglob("*.go"):
        if not f.name.endswith("_test.go"):
            fields.update(JSON_TAG_RE.findall(f.read_text()))
    return fields


def http(method: str, url: str, body: dict | None = None, token: str | None = None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Accept", "application/json")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            raw = r.read()
            return r.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        raw = e.read()
        try:
            return e.code, json.loads(raw)
        except ValueError:
            return e.code, {"raw": raw[:200].decode(errors="replace")}


def speech_check(base: str, token: str) -> list[str]:
    req = urllib.request.Request(base + "/api/v1/speech", method="POST",
                                 data=json.dumps({"text": "Sync check", "speaker": "agent"}).encode())
    req.add_header("Content-Type", "application/json")
    req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            ctype, size = r.headers.get("Content-Type", ""), len(r.read())
        ok = r.status == 200 and ctype.startswith("audio/") and size > 0
        print(f"  live {'ok ' if ok else 'BAD'} {r.status} POST /api/v1/speech -> {ctype}, {size} bytes")
        return [] if ok else [f"live: POST /api/v1/speech returned {ctype} ({size} bytes), want audio"]
    except urllib.error.HTTPError as e:
        if e.code == 409:
            print("  live ok  409 POST /api/v1/speech (speech is off on this backend; UI shows a notice)")
            return []
        print(f"  live BAD {e.code} POST /api/v1/speech")
        return [f"live: POST /api/v1/speech -> {e.code}"]


def live_check(base: str, calls: list[tuple[int, str, str]]) -> list[str]:
    gaps = []
    status, login = http("POST", base + "/api/v1/auth/login", {"email": "a@wassit.demo", "pin": "1234"})
    if status != 200 or not login.get("token"):
        return [f"live: login a@wassit.demo failed ({status}): {login}"]
    token = login["token"]
    branch_id = (login.get("branch") or {}).get("id", "branch-hamra")

    _, outgoing = http("GET", base + "/api/v1/enquiries?scope=outgoing&limit=1", token=token)
    enquiry_id = ((outgoing.get("data") or [{}])[0]).get("id", "")

    subs = {"branchId": branch_id, "enquiryId": enquiry_id}
    seen = set()
    for line, method, path in calls:
        if method != "GET" or path.endswith("/stream"):
            continue
        concrete = re.sub(r"\$\{encodeURIComponent\((\w+)\)\}", lambda m: subs.get(m.group(1), "x"), path)
        if concrete in seen or "${" in concrete:
            continue
        seen.add(concrete)
        status, body = http("GET", base + concrete, token=token)
        ok = 200 <= status < 300
        print(f"  live {'ok ' if ok else 'BAD'} {status} GET {concrete}  (app.js:{line})")
        if not ok:
            gaps.append(f"live: GET {concrete} -> {status} {body.get('error', body)}")
    if any(p == "/api/v1/speech" for _, _, p in calls):
        gaps += speech_check(base, token)
    return gaps


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--static", action="store_true", help="skip the live checks")
    ap.add_argument("--backend", default="http://localhost:8113")
    args = ap.parse_args()

    src = APP_JS.read_text()
    calls = frontend_calls(src)
    routes = backend_routes()
    gaps: list[str] = []

    print(f"1. routes: {len(calls)} frontend calls vs {len(routes)} backend routes")
    for line, method, path in calls:
        tmpl = to_template(path)
        ok = (method, tmpl) in routes
        print(f"  {'ok ' if ok else 'BAD'} {method:6} {tmpl}  (app.js:{line})")
        if not ok:
            gaps.append(f"route: {method} {tmpl} (app.js:{line}) has no backend route")

    fields = set(FIELD_RE.findall(src))
    known = backend_json_fields()
    missing = sorted(f for f in fields if f not in known and f not in FRONTEND_ONLY)
    print(f"2. fields: {len(fields)} snake_case fields read by the UI; {len(missing)} unknown to the backend")
    for f in missing:
        print(f"  BAD {f}")
        gaps.append(f"field: UI reads '{f}' but no backend JSON tag has it")

    if not args.static:
        print(f"3. live: {args.backend}")
        gaps += live_check(args.backend.rstrip("/"), calls)

    print()
    if gaps:
        print(f"NOT IN SYNC: {len(gaps)} gap(s)")
        for g in gaps:
            print(" -", g)
        return 1
    print("IN SYNC: every frontend call has a route, every field read exists, live GETs return 2xx")
    return 0


if __name__ == "__main__":
    sys.exit(main())
