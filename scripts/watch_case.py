#!/usr/bin/env python3.13
"""Live printer for scripts/watch_case.sh: follows the newest enquiry on BACKEND_URL and prints
events (with the decider), agent API calls from the server log, and rules-decided offers."""
from __future__ import annotations

import json
import os
import re
import sys
import time
import urllib.request

BASE = os.environ.get("BACKEND_URL", "http://localhost:8127")
LOG_PATH, CASES_PID = sys.argv[1], int(sys.argv[2])
LIMIT_S = int(os.environ.get("WATCH_LIMIT_S", "600"))
C = {"agent": "\033[36m", "rules": "\033[35m", "human": "\033[33m", "pharmacy": "\033[32m",
     "system": "\033[90m", "call": "\033[34m", "bold": "\033[1m", "end": "\033[0m"}
# Agent-side calls worth showing (the watcher's own polling GETs are filtered out).
CALL_RE = re.compile(r"method=(\w+) path=(\S+) status=(\d+)")
SHOW = re.compile(r"/(medication|clarification-request|candidates|targets|send|follow-up|no-response|"
                  r"notify|wait|close|offers|reference/products|photos|agent-reading)")


def http(method, path, body=None, token=None):
    req = urllib.request.Request(BASE + path, method=method,
                                 data=json.dumps(body).encode() if body is not None else None)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    with urllib.request.urlopen(req, timeout=10) as r:
        return json.loads(r.read() or b"{}")


def alive(pid):
    try:
        os.kill(pid, 0)
        return True
    except OSError:
        return False


def decider(ev):
    if ev.get("actor_type") == "agent":
        return "agent", "Wassit · relayed"
    if ev.get("actor_type") == "system" and ev.get("actor_id") == "rules":
        return "rules", "Rules"
    if ev.get("actor_type") == "system":
        return "system", "System"
    if ev.get("type") == "reply_received":
        return "pharmacy", "Pharmacy"
    return "human", "Human"


def short(payload):
    if isinstance(payload, str):
        try:
            payload = json.loads(payload)
        except ValueError:
            return payload[:160]
    keep = {k: v for k, v in (payload or {}).items() if k not in ("message_ids", "target_ids")}
    return json.dumps(keep, ensure_ascii=False)[:220]


def main():
    token = None
    while token is None and alive(CASES_PID):
        try:
            token = http("POST", "/api/v1/auth/login", {"email": "a@wassit.demo", "pin": "1234"})["token"]
        except Exception:
            time.sleep(1)
    start, enquiry_id, seen, offers_seen, log_pos = time.time(), None, set(), set(), 0
    print(f"{C['bold']}watching {BASE} (limit {LIMIT_S}s){C['end']}")
    while time.time() - start < LIMIT_S:
        # agent API calls from the server log
        try:
            with open(LOG_PATH, encoding="utf-8", errors="replace") as fh:
                fh.seek(log_pos)
                chunk = fh.read()
                log_pos = fh.tell()
            for line in chunk.splitlines():
                m = CALL_RE.search(line)
                if m and SHOW.search(m.group(2)) and not (m.group(1) == "GET" and m.group(2).endswith("/offers")):
                    if m.group(1) == "GET" and "/candidates" not in m.group(2) and "reference" not in m.group(2):
                        continue
                    print(f"  {C['call']}call  {m.group(1):5} {m.group(2)} -> {m.group(3)}{C['end']}")
        except FileNotFoundError:
            pass
        try:
            if not enquiry_id:
                data = http("GET", "/api/v1/enquiries?scope=outgoing&limit=1", token=token).get("data", [])
                if data:
                    enquiry_id = data[0]["id"]
                    print(f"{C['bold']}enquiry {enquiry_id}: \"{data[0]['raw_request']}\" x{data[0]['quantity']}{C['end']}")
            if enquiry_id:
                detail = http("GET", f"/api/v1/enquiries/{enquiry_id}", token=token)
                for ev in detail.get("events", []):
                    if ev["id"] in seen:
                        continue
                    seen.add(ev["id"])
                    key, label = decider(ev)
                    print(f"{C[key]}#{ev['seq']:<3} {label:17} {ev['type']:22} {short(ev.get('payload'))}{C['end']}")
                for off in http("GET", f"/api/v1/enquiries/{enquiry_id}/offers", token=token).get("data", []):
                    if off["id"] in offers_seen:
                        continue
                    offers_seen.add(off["id"])
                    print(f"{C['rules']}      OFFER {off.get('decision')} (decided by {off.get('decided_by')}): "
                          f"\"{off.get('source_text', '')[:80]}\" -> {'; '.join(off.get('reasons') or [])[:200]}{C['end']}")
                enq = detail.get("enquiry", {})
                if enq.get("status") in ("closed", "cancelled"):
                    runs = detail.get("runs", [])
                    print(f"{C['bold']}CLOSED: outcome={enq.get('outcome')} branch={enq.get('outcome_branch_id')} "
                          f"runs={len(runs)} steps={sum(r.get('steps', 0) for r in runs)} "
                          f"tokens={sum(r.get('input_tokens', 0) for r in runs)}/{sum(r.get('output_tokens', 0) for r in runs)} "
                          f"elapsed={time.time() - start:.0f}s{C['end']}")
                    return
        except Exception as err:  # keep watching through transient errors
            print(f"  (poll error: {err})")
        if not alive(CASES_PID):
            print(f"{C['bold']}case runner finished (enquiry not closed){C['end']}")
            return
        time.sleep(2)
    print(f"{C['bold']}time limit reached{C['end']}")


if __name__ == "__main__":
    main()
