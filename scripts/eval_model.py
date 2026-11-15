#!/usr/bin/env python3.13
"""Model-quality eval for Wassit: plays agent/cases/model_eval.jsonl against a LIVE throwaway backend
with the real model, then scores the model's tasks (T1-T5) from the stored runs, tool calls and offers.

It reuses scripts/run_cases.py for the human side (login, clarify, approve, scripted replies) and the
runner call; it only adds a per-case deadline and the scoring. Reads tool calls and offers straight
from the throwaway Postgres container (docker exec psql), so never point it at a shared database.

Usage:
    BACKEND_URL=http://localhost:8126 AGENT_TOKEN=quality-token EVAL_DB_CONTAINER=wassit-quality-db \
    CORNILIA_PROVIDER=ollama CORNILIA_BASE_URL=http://10.31.216.24:11434/v1 CORNILIA_API_KEY=ollama \
    CORNILIA_MODEL=qwen3.5:35b-a3b CORNILIA_TIMEOUT=120 CORNILIA_MAX_TOKENS=1024 \
    python3.13 scripts/eval_model.py [--only eval-01,eval-02] [--case-timeout 150] [--label before]

Writes the scorecard to agent/cases/model_eval_results.md (appends a section per --label run) and the
raw per-case records to agent/cases/model_eval_raw.json is NOT done; everything is in the markdown.
"""

import argparse
import json
import os
import re
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

os.environ.setdefault("BACKEND_URL", "http://localhost:8126")
os.environ.setdefault("AGENT_TOKEN", "quality-token")
sys.path.insert(0, str(Path(__file__).resolve().parent))
import run_cases as rc  # noqa: E402

CASES = rc.AGENT_DIR / "cases" / "model_eval.jsonl"
RESULTS = rc.AGENT_DIR / "cases" / "model_eval_results.md"
DB_CONTAINER = os.environ.get("EVAL_DB_CONTAINER", "wassit-quality-db")
if os.environ["BACKEND_URL"].rstrip("/").endswith((":8113", ":8080")):
    sys.exit("refusing to run the eval against a shared backend; use your throwaway stack")


# ---------------------------------------------------------------- DB reads (throwaway container only)


def sql_json(query: str, *params: str):
    """Runs one SELECT that returns a single json value; params are bound by psql -v (quoted)."""
    args = ["docker", "exec", "-i", DB_CONTAINER, "psql", "-U", "alphahack", "-d", "alphahack", "-tAq"]
    for i, p in enumerate(params, 1):
        args += ["-v", f"p{i}={p}"]
    out = subprocess.run(args, input=query, capture_output=True, text=True, timeout=30)
    text = out.stdout.strip()
    return json.loads(text) if text else None


def tool_calls(eid: str) -> list[dict]:
    q = """SELECT coalesce(json_agg(x ORDER BY x.started_at, x.seq), '[]') FROM (
             SELECT r.id AS run_id, r.trigger, r.status AS run_status, r.steps, r.started_at, tc.seq,
                    tc.name, tc.input, tc.output, tc.is_error
             FROM enquiry_agent_runs r JOIN enquiry_agent_tool_calls tc ON tc.run_id = r.id
             WHERE r.enquiry_id = :'p1') x;"""
    return sql_json(q, eid) or []


def runs(eid: str) -> list[dict]:
    q = """SELECT coalesce(json_agg(json_build_object('trigger', trigger, 'status', status, 'steps', steps,
             'error', error, 'secs', extract(epoch FROM coalesce(finished_at, now()) - started_at))
             ORDER BY started_at), '[]') FROM enquiry_agent_runs WHERE enquiry_id = :'p1';"""
    return sql_json(q, eid) or []


def offers(eid: str) -> list[dict]:
    q = """SELECT coalesce(json_agg(json_build_object('target_id', target_id, 'branch_id', branch_id,
             'source_text', source_text, 'extracted_by', extracted_by, 'in_stock', in_stock, 'strength', strength,
             'quantity', quantity, 'price_cents', price_cents, 'evidence', evidence, 'decision', decision,
             'reasons', reasons) ORDER BY created_at), '[]') FROM target_offers WHERE enquiry_id = :'p1';"""
    return sql_json(q, eid) or []


def medication_id(eid: str):
    return sql_json("SELECT to_json(medication_id) FROM enquiries WHERE id = :'p1';", eid)


# ---------------------------------------------------------------- play one case


def play(case: dict, deadline_s: int) -> dict:
    started = time.monotonic()
    rec = {"id": case["id"], "error": "", "rounds": 0, "timed_out": False}
    rc.login_all()
    status, body = rc.req("POST", "/api/v1/enquiries", rc.REQUESTER, {
        "raw_request": case["raw_request"], "quantity": case["quantity"], "handled_by": "agent"})
    if status != 201:
        rec["error"] = f"create {status}: {rc.snippet(body)}"
        return rec
    eid = rec["eid"] = body["id"]
    replied: set = set()
    try:
        while True:
            left = deadline_s - (time.monotonic() - started)
            if left <= 5:
                rec["timed_out"] = True
                break
            rec["rounds"] += 1
            rc.RUNNER_TIMEOUT = int(left)
            rc.run_runner("model")
            view = rc.enquiry_view(eid)
            if view.get("status") in rc.CLOSED_STATUSES:
                break
            rc.act(case, eid, view, replied)
            time.sleep(2)
    except rc.CaseError as e:
        rec["error"] = str(e)
        rec["timed_out"] = "timed out" in str(e)
    rec["wall_s"] = round(time.monotonic() - started, 1)
    rec["view"] = rc.enquiry_view(eid)
    rec["calls"] = tool_calls(eid)
    rec["runs"] = runs(eid)
    rec["offers"] = offers(eid)
    rec["medication_id"] = medication_id(eid)
    return rec


# ---------------------------------------------------------------- scoring

_NUM = re.compile(r"(\d+(?:\.\d+)?)")


def _norm(s) -> str:
    return " ".join(str(s or "").lower().split())


def _ok_call(c: dict) -> bool:
    return not c.get("is_error")


def _err_code(c: dict) -> str:
    try:
        return str(json.loads(c.get("output") or "{}").get("code") or "error")
    except ValueError:
        return "error"


def expected_outcome(case: dict, targets: list, offer_by_target: dict) -> str:
    """The outcome the Go rules allow for the targets the model actually contacted."""
    contacted = [t for t in targets if t.get("status") not in ("cancelled", "proposed", "approved")]
    if not contacted:
        return case["expected_outcome"]
    kinds = []
    for t in contacted:
        reply = case["replies"].get(t.get("branch_id"))
        exp = case.get("expect", {}).get(t.get("branch_id"), {})
        dec = (offer_by_target.get(t.get("id")) or {}).get("decision")
        if reply and reply["kind"] == "available" and (exp.get("decision") == ["matches"] or dec == "matches"):
            return "available"
        kinds.append(reply["kind"] if reply else "silent")
    return "unavailable" if all(k == "unavailable" for k in kinds) else "unable_to_confirm"


def score(case: dict, rec: dict) -> dict:
    calls = rec.get("calls", [])
    view = rec.get("view", {})
    targets = [t for t in rc.items(view, "targets")]
    agent_offers = [o for o in rec.get("offers", []) if o.get("extracted_by") == "agent"]
    last_offer = {}
    for o in agent_offers:
        last_offer[o["target_id"]] = o
    s = {"id": case["id"], "notes": []}

    # T1: medicine matched; clarification asked iff expected.
    asked = sum(1 for c in calls if c["name"] == "ask_requester" and _ok_call(c))
    want_ask = case.get("clarify_answer") is not None
    med_ok = rec.get("medication_id") == case.get("expected_medication")
    s["T1"] = 1.0 if (med_ok and (asked >= 1) == want_ask and asked <= 1) else 0.0
    if not med_ok:
        s["notes"].append(f"med {rec.get('medication_id')} != {case.get('expected_medication')}")
    if (asked >= 1) != want_ask:
        s["notes"].append(f"ask_requester x{asked}, expected {'1' if want_ask else '0'}")

    # T2: candidates looked up, 1-3 pharmacies proposed, no wait() in a run after propose_targets.
    proposed_ok = any(c["name"] == "propose_targets" and _ok_call(c) for c in calls)
    found = any(c["name"] == "find_candidates" and _ok_call(c) for c in calls)
    wait_after_propose = 0
    by_run: dict = {}
    for c in calls:
        by_run.setdefault(c["run_id"], []).append(c)
    for seq in by_run.values():
        proposed_here = False
        for c in seq:
            if c["name"] == "propose_targets" and _ok_call(c):
                proposed_here = True
            elif c["name"] == "wait" and proposed_here:
                wait_after_propose += 1
    n_targets = len(targets)
    s["T2"] = 1.0 if (proposed_ok and found and 1 <= n_targets <= 3 and not wait_after_propose) else 0.0
    if wait_after_propose:
        s["notes"].append(f"wait() after propose_targets x{wait_after_propose}")
    if not (1 <= n_targets <= 3):
        s["notes"].append(f"{n_targets} targets")

    # T3: every replied target read into an offer with exact quotes and the expected fields/decision.
    t3_checks, t3_pass = 0, 0
    ref_calls = [c for c in calls if c["name"] == "check_reference" and _ok_call(c)]
    offer_errors = [c for c in calls if c["name"] == "record_offer" and c.get("is_error")]
    for t in targets:
        reply = case["replies"].get(t.get("branch_id"))
        if not reply or not reply.get("text") or t.get("status") in ("sent", "proposed", "approved", "cancelled", "no_response"):
            continue
        exp = case.get("expect", {}).get(t["branch_id"], {})
        t3_checks += 1
        o = last_offer.get(t["id"])
        label = t["branch_id"].replace("branch-", "")
        if not o:
            s["notes"].append(f"{label}: no offer recorded")
            continue
        bad = []
        reply_n = _norm(reply["text"])
        for k, v in (o.get("evidence") or {}).items():
            if _norm(v) not in reply_n:
                bad.append(f"quote {k}={v!r} not in reply")
        if "quantity" in exp and o.get("quantity") not in exp["quantity"]:
            bad.append(f"quantity {o.get('quantity')} not in {exp['quantity']}")
        if "price_cents" in exp and o.get("price_cents") not in exp["price_cents"]:
            bad.append(f"price {o.get('price_cents')} not in {exp['price_cents']}")
        if "decision" in exp and o.get("decision") not in exp["decision"]:
            bad.append(f"decision {o.get('decision')} not in {exp['decision']} ({'; '.join(o.get('reasons') or [])[:120]})")
        if exp.get("check_reference"):
            nums = set(_NUM.findall(reply["text"]))
            if not any(set(_NUM.findall(json.dumps(c.get("input")))) & nums for c in ref_calls):
                bad.append("check_reference not called for the offered strength")
        if bad:
            s["notes"].append(f"{label}: " + "; ".join(bad))
        else:
            t3_pass += 1
    s["T3"] = (t3_pass / t3_checks) if t3_checks else None
    s["offer_errors"] = len(offer_errors)
    if offer_errors:
        s["notes"].append(f"record_offer errors x{len(offer_errors)}: {_err_code(offer_errors[0])}")

    # T4: requester notified when an alternative/escalated offer exists; the content filter never fires.
    notifies = [c for c in calls if c["name"] == "notify_requester"]
    filtered = [c for c in calls if c.get("is_error") and "content_filtered" in (c.get("output") or "")]
    need_notify = any(t.get("status") == "alternative_offered" for t in targets) or any(
        o.get("decision") == "escalated" for o in last_offer.values())
    if need_notify or notifies:
        ok_notify = any(_ok_call(c) for c in notifies)
        s["T4"] = 1.0 if ((ok_notify or not need_notify) and not filtered) else 0.0
        if need_notify and not ok_notify:
            s["notes"].append("requester never notified about an alternative/escalated offer")
    else:
        s["T4"] = None
    if filtered:
        s["notes"].append(f"content_filtered x{len(filtered)}: {json.dumps(filtered[0].get('input'), ensure_ascii=False)[:120]}")

    # T5: closed with the outcome the rules allow; silent pharmacies chased once then no_response.
    want = expected_outcome(case, [{"id": t["id"], "branch_id": t["branch_id"], "status": t["status"]} for t in targets],
                            last_offer)
    outcome = view.get("outcome") or ""
    t5 = outcome == want
    if not t5:
        s["notes"].append(f"outcome {outcome or view.get('status')} != {want}")
    fu = {}
    for c in calls:
        if c["name"] == "follow_up" and _ok_call(c):
            tid = (c.get("input") or {}).get("target_id")
            fu[tid] = fu.get(tid, 0) + 1
    # Closing 'available' early is allowed while others are silent; otherwise each silent one is chased once.
    silent = [] if outcome == "available" else [
        t for t in targets if case["replies"].get(t.get("branch_id")) is None and t.get("status") != "cancelled"]
    for t in silent:
        if fu.get(t["id"], 0) != 1 or (outcome and t.get("status") != "no_response"):
            t5 = False
            s["notes"].append(f"{t['branch_id'].replace('branch-', '')}: follow_up x{fu.get(t['id'], 0)}, status {t.get('status')}")
    s["T5"] = 1.0 if t5 else 0.0
    s["outcome"], s["want"] = outcome or view.get("status", ""), want

    s["steps"] = sum(r.get("steps") or 0 for r in rec.get("runs", []))
    s["runs"] = len(rec.get("runs", []))
    s["failed_runs"] = sum(1 for r in rec.get("runs", []) if r.get("status") == "failed")
    errs = [c for c in calls if c.get("is_error")]
    s["errors"] = len(errs)
    s["error_codes"] = sorted({_err_code(c) for c in errs})
    s["wall_s"] = rec.get("wall_s", 0)
    if rec.get("timed_out"):
        s["notes"].append("TIMED OUT")
    if rec.get("error"):
        s["notes"].append(rec["error"][:160])
    return s


# ---------------------------------------------------------------- report


def _fmt(v):
    return "-" if v is None else f"{v:.2f}"


def render(scores: list, label: str) -> str:
    lines = [f"## Run: {label} ({datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M UTC')}, "
             f"model {os.environ.get('CORNILIA_MODEL', '?')})", "",
             "| Case | T1 | T2 | T3 | T4 | T5 | Outcome (want) | Runs | Steps | Errors | Wall s | Notes |",
             "|---|---|---|---|---|---|---|---|---|---|---|---|"]
    for s in scores:
        notes = rc.cell("; ".join(s["notes"]), 300)
        lines.append(f"| {s['id']} | {_fmt(s['T1'])} | {_fmt(s['T2'])} | {_fmt(s['T3'])} | {_fmt(s['T4'])} | "
                     f"{_fmt(s['T5'])} | {s['outcome']} ({s['want']}) | {s['runs']} ({s['failed_runs']} failed) | "
                     f"{s['steps']} | {s['errors']} {','.join(s['error_codes'])} | {s['wall_s']} | {notes} |")
    lines += ["", "| Task | Score | Cases scored |", "|---|---|---|"]
    names = {"T1": "understand request + match medicine", "T2": "propose pharmacies, stop for approval",
             "T3": "read replies into record_offer", "T4": "notify requester (filter never fires)",
             "T5": "finish: follow_up, no_response, close"}
    for t, name in names.items():
        vals = [s[t] for s in scores if s[t] is not None]
        avg = sum(vals) / len(vals) if vals else None
        lines.append(f"| {t} {name} | {_fmt(avg)} | {len(vals)} |")
    total_steps = sum(s["steps"] for s in scores)
    lines += ["", f"Totals: steps {total_steps}, tool errors {sum(s['errors'] for s in scores)}, "
              f"record_offer errors {sum(s['offer_errors'] for s in scores)}, "
              f"wall {sum(s['wall_s'] for s in scores):.0f}s.", ""]
    return "\n".join(lines)


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--cases", type=Path, default=CASES)
    ap.add_argument("--only", default="", help="comma-separated case ids")
    ap.add_argument("--case-timeout", type=int, default=150)
    ap.add_argument("--label", default="run")
    ap.add_argument("--output", type=Path, default=RESULTS)
    args = ap.parse_args(argv)
    cases = [json.loads(line) for line in args.cases.read_text(encoding="utf-8").splitlines() if line.strip()]
    if args.only:
        keep = set(args.only.split(","))
        cases = [c for c in cases if c["id"] in keep]
    scores = []
    for case in cases:
        print(f"[{case['id']}] running", flush=True)
        try:
            rec = play(case, args.case_timeout)
            s = score(case, rec)
        except Exception as e:  # one broken case must not stop the scorecard
            s = {"id": case["id"], "T1": 0.0, "T2": 0.0, "T3": None, "T4": None, "T5": 0.0, "outcome": "error",
                 "want": case["expected_outcome"], "runs": 0, "failed_runs": 0, "steps": 0, "errors": 0,
                 "error_codes": [], "offer_errors": 0, "wall_s": 0, "notes": [f"{type(e).__name__}: {e}"[:200]]}
        print(f"[{case['id']}] T1={s['T1']} T2={s['T2']} T3={s['T3']} T4={s['T4']} T5={s['T5']} "
              f"{s['outcome']}/{s['want']} steps={s['steps']} {s['wall_s']}s :: {'; '.join(s['notes'])}", flush=True)
        scores.append(s)
    section = render(scores, args.label)
    print(section)
    head = "# Wassit model eval (agent/cases/model_eval.jsonl, scripts/eval_model.py)\n\n"
    old = args.output.read_text(encoding="utf-8") if args.output.exists() else head
    args.output.write_text(old.rstrip() + "\n\n" + section, encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
