#!/usr/bin/env python3.13
"""Wassit case runner: plays each case in agent/cases/cases.jsonl against a LIVE backend. Stdlib only.

For every case:
  1. log in as A (requester) and B, C, D, E (branch staff, PIN 1234)
  2. A creates a handled_by=agent enquiry
  3. loop up to --max-rounds times:
       a. run `agent/.venv/bin/python agent/runner.py --once --policy <policy>`
          (--once processes every current wakeup, so other enquiries may run too)
       b. read the enquiry; stop when it is closed or cancelled
       c. act as the humans: A clarifies when asked, A approves proposed targets,
          and each target branch replies (scripted) once its enquiry message was sent
       d. sleep 3s
  4. read outcome, hitl_count and runs from the enquiry, steps and cost_micros from the stats row

Usage:
    python3.13 scripts/run_cases.py                       # all cases, scripted policy
    python3.13 scripts/run_cases.py --only case-06        # one case
    python3.13 scripts/run_cases.py --policy model --max-rounds 8

Environment:
    BACKEND_URL   backend base URL          (default http://localhost:8080)
    AGENT_TOKEN   agent credential, sent as "Authorization: Agent <token>" (default dev-agent-token)

Prints a markdown table and writes the same table to agent/cases/results.md.
Errors in one case are recorded in the table; the other cases still run.
Exit status: 0 when the run finished (see the table for matches); non-zero for a bad cases file or arguments.
"""

import argparse
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
AGENT_DIR = REPO_ROOT / "agent"
RUNNER = [str(AGENT_DIR / ".venv" / "bin" / "python"), str(AGENT_DIR / "runner.py")]
DEFAULT_CASES = AGENT_DIR / "cases" / "cases.jsonl"
RESULTS_PATH = AGENT_DIR / "cases" / "results.md"

BACKEND_URL = os.environ.get("BACKEND_URL", "http://localhost:8080").rstrip("/")
AGENT_TOKEN = os.environ.get("AGENT_TOKEN", "dev-agent-token")
HTTP_TIMEOUT = 15
RUNNER_TIMEOUT = 600
ROUND_SLEEP = 3
POLICIES = ("scripted", "model")
REQUIRED_KEYS = ("id", "raw_request", "quantity", "replies", "expected_outcome")
CLOSED_STATUSES = ("closed", "cancelled")

# Demo seed (CONTRACT.md "Seed"): one staff per branch, PIN 1234.
PIN = "1234"
STAFF_EMAIL = {
    "A": "a@wassit.demo",
    "B": "b@wassit.demo",
    "C": "c@wassit.demo",
    "D": "d@wassit.demo",
    "E": "e@wassit.demo",
}
REQUESTER = "A"
BRANCH_STAFF = {
    "branch-achrafieh": "B",
    "branch-baabda": "C",
    "branch-verdun": "D",
    "branch-byblos": "E",
}

_tokens = {}  # staff letter -> token, filled by login_all()


class CaseError(Exception):
    """Ends one case early. It is recorded in the table, not fatal to the run."""


# ---------------------------------------------------------------- HTTP


def _parse(raw):
    if not raw:
        return {}
    try:
        return json.loads(raw)
    except ValueError:
        return {"raw": raw.decode("utf-8", "replace")}


def req(method, path, who, body=None):
    """Call the backend. who: None (no Authorization), "agent", or "A".."E" (staff).

    Returns (status, parsed_json). Network failures return (0, {"error": {...}}).
    """
    headers = {"Accept": "application/json"}
    if who == "agent":
        headers["Authorization"] = f"Agent {AGENT_TOKEN}"
    elif who is not None and _tokens.get(who):
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


def expect_ok(resp, label):
    status, body = resp
    if not 200 <= status < 300:
        raise CaseError(f"{label} expected HTTP 2xx, got {status}: {snippet(body)}")
    return body


def snippet(body, limit=300):
    text = json.dumps(body, ensure_ascii=False, default=str)
    return text if len(text) <= limit else text[:limit] + "...(truncated)"


def items(body, *keys):
    """List payload: the body itself when it is a list, else the first body[key] that is a
    list or {"data": [...]}. Returns only dict items."""
    if isinstance(body, list):
        return [x for x in body if isinstance(x, dict)]
    if isinstance(body, dict):
        for key in keys:
            value = body.get(key)
            if isinstance(value, dict) and isinstance(value.get("data"), list):
                value = value["data"]
            if isinstance(value, list):
                return [x for x in value if isinstance(x, dict)]
    return []


# ---------------------------------------------------------------- backend helpers


def login_all():
    _tokens.clear()
    for who, email in STAFF_EMAIL.items():
        status, body = req("POST", "/api/v1/auth/login", None, {"email": email, "pin": PIN})
        token = body.get("token") if status == 200 and isinstance(body, dict) else None
        if not isinstance(token, str) or not token:
            raise CaseError(f"login as {who} failed with HTTP {status}")
        _tokens[who] = token


def enquiry_view(eid):
    """Enquiry detail as one flat dict (top level merged with the optional "enquiry" wrapper)."""
    status, body = req("GET", f"/api/v1/enquiries/{eid}", REQUESTER)
    if status != 200 or not isinstance(body, dict):
        raise CaseError(f"GET enquiry expected HTTP 200, got {status}: {snippet(body)}")
    view = dict(body)
    if isinstance(body.get("enquiry"), dict):
        view.update(body["enquiry"])
    return view


def stats_row(eid):
    status, body = req("GET", "/api/v1/stats", REQUESTER)
    if status != 200:
        raise CaseError(f"GET stats expected HTTP 200, got {status}: {snippet(body)}")
    for row in items(body, "enquiries", "stats", "data"):
        if row.get("enquiry_id") == eid:
            return row
    return None


def run_runner(policy):
    cmd = RUNNER + ["--once", "--policy", policy]
    try:
        proc = subprocess.run(cmd, cwd=str(AGENT_DIR), env=os.environ.copy(),
                              capture_output=True, text=True, timeout=RUNNER_TIMEOUT)
    except subprocess.TimeoutExpired:
        raise CaseError(f"runner timed out after {RUNNER_TIMEOUT}s")
    except OSError as err:
        raise CaseError(f"could not start runner ({RUNNER[0]}): {err}")
    if proc.returncode != 0:
        lines = (proc.stdout + proc.stderr).strip().splitlines()
        tail = lines[-1] if lines else "no output"
        raise CaseError(f"runner exited {proc.returncode}: {tail}")


# ---------------------------------------------------------------- the human side


def act(case, eid, view, replied):
    """Clarify, approve and reply the way the people in the demo would. Reads one snapshot."""
    if view.get("status") == "awaiting_clarification" and case.get("clarify_answer"):
        expect_ok(req("POST", f"/api/v1/enquiries/{eid}/clarify", REQUESTER,
                      {"answer": case["clarify_answer"]}), "clarify")

    proposed = [t["id"] for t in items(view, "targets")
                if t.get("status") == "proposed" and t.get("id")]
    if proposed:
        expect_ok(req("POST", f"/api/v1/enquiries/{eid}/targets/approve", REQUESTER,
                      {"target_ids": proposed}), "approve targets")

    sent_msgs = {m.get("target_id") for m in items(view, "messages")
                 if m.get("kind") == "enquiry" and m.get("status") == "sent"}
    for t in items(view, "targets"):
        tid, branch = t.get("id"), t.get("branch_id")
        if not tid or tid in replied:
            continue
        if t.get("status") != "sent" or tid not in sent_msgs:
            continue
        reply = case["replies"].get(branch)
        if reply is None:  # null = this branch never replies
            continue
        staff = BRANCH_STAFF.get(branch)
        if staff is None:
            raise CaseError(f"no staff login for branch {branch}")
        expect_ok(req("POST", f"/api/v1/targets/{tid}/reply", staff, dict(reply)),
                  f"reply from {branch}")
        replied.add(tid)


# ---------------------------------------------------------------- one case


def fill_from_backend(rec, eid):
    view = enquiry_view(eid)
    rec["status"] = str(view.get("status", ""))
    rec["outcome"] = str(view.get("outcome") or "")
    hitl = view.get("hitl_count")
    rec["hitl"] = hitl if isinstance(hitl, int) and not isinstance(hitl, bool) else 0
    rec["runs"] = len(items(view, "runs", "agent_runs"))
    row = stats_row(eid)
    if row:
        steps = row.get("agent_steps", row.get("steps", 0))
        cost = row.get("cost_micros", 0)
        rec["steps"] = steps if isinstance(steps, int) else 0
        rec["cost_micros"] = cost if isinstance(cost, int) else 0


def run_case(case, policy, max_rounds):
    started = time.monotonic()
    rec = {"id": case["id"], "expected": case["expected_outcome"], "outcome": "",
           "status": "", "match": False, "hitl": 0, "runs": 0, "steps": 0,
           "cost_micros": 0, "rounds": 0, "elapsed": 0.0, "error": ""}
    eid = None
    try:
        login_all()
        status, body = req("POST", "/api/v1/enquiries", REQUESTER, {
            "raw_request": case["raw_request"],
            "quantity": case["quantity"],
            "handled_by": "agent",
        })
        if status != 201 or not isinstance(body, dict) or not body.get("id"):
            raise CaseError(f"create enquiry expected HTTP 201, got {status}: {snippet(body)}")
        eid = body["id"]

        replied = set()
        for rnd in range(1, max_rounds + 1):
            rec["rounds"] = rnd
            run_runner(policy)
            view = enquiry_view(eid)
            rec["status"] = str(view.get("status", ""))
            if view.get("status") in CLOSED_STATUSES:
                break
            act(case, eid, view, replied)
            if rnd < max_rounds:
                time.sleep(ROUND_SLEEP)
    except CaseError as err:
        rec["error"] = str(err)
    except Exception as err:  # one broken case must not stop the table
        rec["error"] = f"{type(err).__name__}: {err}"

    if eid is not None:
        try:
            fill_from_backend(rec, eid)
        except Exception as err:
            rec["error"] = (rec["error"] + "; " if rec["error"] else "") + \
                f"read-back failed: {type(err).__name__}: {err}"

    rec["match"] = bool(rec["outcome"]) and rec["outcome"] == rec["expected"]
    rec["elapsed"] = time.monotonic() - started
    return rec


# ---------------------------------------------------------------- input and output


def load_cases(path):
    cases = []
    with open(path, encoding="utf-8") as fh:
        for n, line in enumerate(fh, 1):
            line = line.strip()
            if not line:
                continue
            try:
                case = json.loads(line)
            except ValueError as err:
                sys.exit(f"{path}:{n}: invalid JSON: {err}")
            if not isinstance(case, dict):
                sys.exit(f"{path}:{n}: each line must be a JSON object")
            missing = [k for k in REQUIRED_KEYS if k not in case]
            if missing:
                sys.exit(f"{path}:{n}: missing keys {missing}")
            if not isinstance(case["replies"], dict):
                sys.exit(f"{path}:{n}: replies must be an object keyed by branch id")
            cases.append(case)
    return cases


def cell(text, limit=160):
    s = " ".join(str(text).split()).replace("|", "/")
    return s if len(s) <= limit else s[:limit - 3] + "..."


def render(results, policy, max_rounds, cases_path, placeholder_count):
    head = ["Case", "Expected", "Outcome", "Match", "HITL", "Runs", "Steps",
            "Cost (micros)", "Rounds", "Elapsed (s)", "Error"]
    lines = [
        "# Wassit case results",
        "",
        f"- Run (UTC): {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M:%S')}",
        f"- Cases file: {cases_path}",
        f"- Policy: {policy}; max rounds: {max_rounds}; backend: {BACKEND_URL}",
    ]
    if placeholder_count:
        lines.append(f"- PLACEHOLDER cases: {placeholder_count} of {len(results)} "
                     "(not the owner's real cases yet)")
    lines += ["", f"| {' | '.join(head)} |", "|" + "---|" * len(head)]
    for r in results:
        outcome = r["outcome"] or f"none ({r['status'] or 'unknown'})"
        lines.append(
            f"| {r['id']} | {r['expected']} | {cell(outcome, 60)} | "
            f"{'yes' if r['match'] else 'no'} | {r['hitl']} | {r['runs']} | {r['steps']} | "
            f"{r['cost_micros']} | {r['rounds']} | {r['elapsed']:.1f} | {cell(r['error'])} |"
        )
    matches = sum(1 for r in results if r["match"])
    total_hitl = sum(r["hitl"] for r in results)
    total_steps = sum(r["steps"] for r in results)
    total_micros = sum(r["cost_micros"] for r in results)
    lines += [
        "",
        f"**Totals:** matches {matches}/{len(results)}; total HITL {total_hitl}; "
        f"total steps {total_steps}; total cost ${total_micros / 1e6:.6f} "
        f"({total_micros} micros).",
        "",
    ]
    return "\n".join(lines)


def main(argv=None):
    parser = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--cases", type=Path, default=DEFAULT_CASES,
                        help="JSONL case file (default agent/cases/cases.jsonl)")
    parser.add_argument("--policy", choices=POLICIES, default="scripted",
                        help="runner policy passed to --policy (default scripted)")
    parser.add_argument("--max-rounds", type=int, default=25,
                        help="runner rounds per case before giving up (default 12)")
    parser.add_argument("--only", metavar="CASE_ID", default=None,
                        help="run only this case id, e.g. case-06")
    args = parser.parse_args(argv)
    if args.max_rounds < 1:
        parser.error("--max-rounds must be at least 1")

    try:
        cases = load_cases(args.cases)
    except OSError as err:
        sys.exit(f"could not read cases file: {err}")
    if args.only:
        cases = [c for c in cases if c["id"] == args.only]
        if not cases:
            sys.exit(f"no case with id {args.only!r} in {args.cases}")
    if not Path(RUNNER[0]).exists():
        print(f"warning: runner python not found at {RUNNER[0]}; every case will fail", flush=True)

    print(f"Wassit cases against {BACKEND_URL}: {len(cases)} case(s), policy={args.policy}, "
          f"max rounds={args.max_rounds}", flush=True)
    results = []
    for case in cases:
        print(f"[{case['id']}] running", flush=True)
        rec = run_case(case, args.policy, args.max_rounds)
        verdict = "match" if rec["match"] else "MISMATCH"
        print(f"[{case['id']}] {rec['outcome'] or 'no outcome'} (expected {rec['expected']}): "
              f"{verdict}{'; error: ' + rec['error'] if rec['error'] else ''}", flush=True)
        results.append(rec)

    placeholders = sum(1 for c in cases if c.get("placeholder") is True)
    table = render(results, args.policy, args.max_rounds, args.cases, placeholders)
    print()
    print(table)
    RESULTS_PATH.write_text(table, encoding="utf-8")
    print(f"wrote {RESULTS_PATH}", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
