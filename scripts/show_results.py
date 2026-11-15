#!/usr/bin/env python3
"""30-second results view: the latest agent enquiry's timeline, then the stats table.

Usage: python3.13 scripts/show_results.py [enquiry_id]   (default: latest closed agent enquiry)
Env: BACKEND_URL (default http://localhost:8080). Read-only.
"""
import json
import os
import sys
import urllib.error
import urllib.request

B = os.environ.get("BACKEND_URL", "http://localhost:8080")


def req(method, path, token=None, body=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = json.dumps(body).encode() if body is not None else None
    try:
        with urllib.request.urlopen(urllib.request.Request(B + path, data=data, method=method, headers=headers), timeout=10) as r:
            return json.loads(r.read() or b"{}")
    except (urllib.error.HTTPError, urllib.error.URLError, TimeoutError, OSError) as err:
        print(f"error: could not reach backend at {B}: {err}", file=sys.stderr)
        raise SystemExit(1)


tok = req("POST", "/api/v1/auth/login", body={"email": "a@wassit.demo", "pin": "1234"})["token"]
stats = req("GET", "/api/v1/stats?limit=200", tok)
rows = stats["data"]
agent_rows = [r for r in rows if r["handled_by"] == "agent" and r["status"] == "closed" and r["agent_runs"] > 0]

eid = sys.argv[1] if len(sys.argv) > 1 else (agent_rows[0]["enquiry_id"] if agent_rows else None)
if eid:
    d = req("GET", f"/api/v1/enquiries/{eid}", tok)
    e = d["enquiry"]
    print(f"\n=== Wassit timeline: \"{e['raw_request']}\"  ->  {e['status']} / {e['outcome'] or '-'}\n")
    for ev in d["events"]:
        p = ev.get("payload") or {}
        detail = p.get("kind") or p.get("outcome") or p.get("question") or p.get("body") or ""
        undone = "  (undone)" if ev.get("undone_at") else ""
        print(f"  {ev['seq']:>5}  {ev['actor_type']:<6}  {ev['type']:<24} {str(detail)[:70]}{undone}")
    runs = d["runs"]
    print(f"\n  HITL {d['hitl_count']} | agent runs {len(runs)} | steps {sum(r['steps'] for r in runs)} | "
          f"tokens {sum(r['input_tokens'] + r['output_tokens'] for r in runs)} | "
          f"cost ${sum(r['cost_micros'] for r in runs) / 1e6:.4f}")

print("\n=== Agent enquiries (from GET /api/v1/stats)\n")
print(f"  {'request':<38} {'outcome':<18} {'HITL':>4} {'runs':>4} {'steps':>5} {'cost $':>8}")
for r in agent_rows[:15]:
    print(f"  {r['raw_request'][:38]:<38} {r['outcome'] or r['status']:<18} {r['hitl_count']:>4} "
          f"{r['agent_runs']:>4} {r['agent_steps']:>5} {r['cost_micros'] / 1e6:>8.4f}")
t = stats["totals"]
print(f"\n  totals: {t['agent_enquiries']} agent enquiries | HITL {t['hitl_count']} | steps {t['agent_steps']} | "
      f"cost ${t['cost_micros'] / 1e6:.4f}\n")
