#!/usr/bin/env python3.13
"""Wassit live demo driver: narrated, step by step, against a LIVE backend. Stdlib only.

Run after the backend is up and seeded (see DEMO.md):

    python3.13 scripts/demo_flow.py                  # pause for Enter between steps
    python3.13 scripts/demo_flow.py --no-pause       # run straight through
    python3.13 scripts/demo_flow.py --drive-agent    # run agent/runner.py --once as the agent
    python3.13 scripts/demo_flow.py --help           # this text, no network calls

Parts: 1 manual flow, 2 agent flow (Wassit), 3 undo after send, 4 stats table.

Environment:
    BACKEND_URL    backend base URL                   (default http://localhost:8080)
    WASSIT_POLICY  agent policy for --drive-agent     (default scripted; or model)
    UNDO_WAIT      seconds to wait for the outbox     (default 5)

Staff logins: a@ (Hamra, requester), b@ (Achrafieh), c@ (Baabda), d@ (Verdun), PIN 1234.
Nothing is deleted. Each step prints one line of narration, then the event timeline.
"""

import argparse
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
AGENT_PY = ROOT / "agent" / ".venv" / "bin" / "python"
RUNNER = ROOT / "agent" / "runner.py"

BACKEND_URL = os.environ.get("BACKEND_URL", "http://localhost:8080").rstrip("/")
HTTP_TIMEOUT = 15
POLL_SECONDS = 1.5
PIN = "1234"
STAFF_EMAIL = {"A": "a@wassit.demo", "B": "b@wassit.demo", "C": "c@wassit.demo", "D": "d@wassit.demo"}
BRANCH = {"A": "branch-hamra", "B": "branch-achrafieh", "C": "branch-baabda", "D": "branch-verdun"}
MED_AUGMENTIN_1G = "med-augmentin-1g"

# Set in main() from the command line.
NO_PAUSE = False
DRIVE_AGENT = False
POLICY = "scripted"
AGENT_TIMEOUT = 900
UNDO_WAIT = 5.0

_tokens = {}


class DemoError(Exception):
    """A step failed in a way the presenter should see; the demo stops."""


# ---------------------------------------------------------------- HTTP


def _parse(raw):
    if not raw:
        return {}
    try:
        return json.loads(raw)
    except ValueError:
        return {"raw": raw.decode("utf-8", "replace")}


def call(method, path, who=None, body=None):
    """Returns (status, parsed body). who: None (anonymous) or "A".."D" (staff bearer)."""
    headers = {"Accept": "application/json"}
    if who is not None:
        headers["Authorization"] = f"Bearer {token(who)}"
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(BACKEND_URL + path, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as resp:
            return resp.status, _parse(resp.read())
    except urllib.error.HTTPError as err:
        return err.code, _parse(err.read())
    except OSError as err:
        raise DemoError(f"backend not reachable at {BACKEND_URL}: {err}") from err


def token(who):
    if who not in _tokens:
        status, body = call("POST", "/api/v1/auth/login", None,
                            {"email": STAFF_EMAIL[who], "pin": PIN})
        if status != 200 or not isinstance(body, dict) or not body.get("token"):
            raise DemoError(f"login failed for {STAFF_EMAIL[who]} (HTTP {status}); "
                            "run scripts/seed_demo.sh to see what is wrong")
        _tokens[who] = body["token"]
    return _tokens[who]


def ok(method, path, who=None, body=None, want=(200,)):
    status, data = call(method, path, who, body)
    if status not in want:
        err = data.get("error") if isinstance(data, dict) else None
        msg = err.get("message") if isinstance(err, dict) else str(data)[:200]
        raise DemoError(f"{method} {path} -> HTTP {status}: {msg}")
    return data


def items(body):
    """List payload: a bare list, or {"data": [...]}."""
    if isinstance(body, list):
        return body
    if isinstance(body, dict) and isinstance(body.get("data"), list):
        return body["data"]
    return []


# ---------------------------------------------------------------- narration


def banner(title):
    print(f"\n{'=' * 72}\n{title}\n{'=' * 72}", flush=True)


def step(title, line):
    print(f"\n-- {title}\n   {line}", flush=True)


def pause(prompt):
    if NO_PAUSE:
        print(f"   (no pause) {prompt}", flush=True)
        return
    try:
        input(f"   [Enter] {prompt} ")
    except EOFError:  # stdin closed: carry on rather than hang
        print(flush=True)


def short(value, limit=70):
    text = json.dumps(value, ensure_ascii=False, separators=(",", ":")) if value is not None else ""
    return text if len(text) <= limit else text[:limit - 3] + "..."


class Watch:
    """Prints each new enquiry event once, in seq order."""

    def __init__(self, eid):
        self.eid = eid
        self.last_seq = 0

    def poll(self):
        d = ok("GET", f"/api/v1/enquiries/{self.eid}", "A")
        for ev in sorted(d.get("events") or [], key=lambda e: e.get("seq", 0)):
            if ev.get("seq", 0) > self.last_seq:
                self.last_seq = ev["seq"]
                undone = "  (undone)" if ev.get("undone_at") else ""
                print(f"   #{ev['seq']:<4} {ev.get('actor_type', ''):<6} {ev.get('type', ''):<22} "
                      f"{short(ev.get('payload'))}{undone}", flush=True)
        return d


def targets(d):
    return d.get("targets") or []


def target_for(d, branch_id):
    return next((t for t in targets(d) if t.get("branch_id") == branch_id), None)


def message_for(d, target_id, kind="enquiry"):
    return next((m for m in d.get("messages") or []
                 if m.get("target_id") == target_id and m.get("kind") == kind), None)


def inbox_has(who, eid):
    return any(i.get("enquiry_id") == eid for i in items(ok("GET", "/api/v1/inbox", who)))


def wait_until(watch, pred, what, timeout=60, drive=False):
    deadline = time.monotonic() + timeout
    while True:
        d = watch.poll()
        if pred(d):
            return d
        if drive:
            run_agent_once()
        if time.monotonic() > deadline:
            raise DemoError(f"timed out after {timeout}s waiting for {what}")
        time.sleep(POLL_SECONDS)


def run_agent_once():
    """Runs one agent pass: wakeups in, runs out. Blocking; the model call happens here."""
    if not AGENT_PY.exists():
        raise DemoError(f"agent venv not found at {AGENT_PY}; "
                        "create it: cd agent && python3.13 -m venv .venv && "
                        ".venv/bin/pip install \"cornilia @ git+https://github.com/gamercreative/Cornilia.git@v0.1.0\" pytest")
    env = dict(os.environ, BACKEND_URL=BACKEND_URL, WASSIT_POLICY=POLICY)
    rc = subprocess.run([str(AGENT_PY), str(RUNNER), "--once", "--policy", POLICY],
                        cwd=str(ROOT), env=env).returncode
    if rc != 0:
        print(f"   (agent runner exited {rc}; the demo keeps polling)", flush=True)


def new_enquiry(who, raw, quantity, handled_by, medication_id=None):
    body = {"raw_request": raw, "quantity": quantity, "handled_by": handled_by}
    if medication_id:
        body["medication_id"] = medication_id
    return ok("POST", "/api/v1/enquiries", who, body, want=(201,))


# ---------------------------------------------------------------- parts


def part1_manual():
    banner("Part 1: manual flow (no agent)")
    step("A writes the enquiry by hand",
         "Augmentin 1g x2 from the catalogue, handled by a person. Hamra is the requester.")
    pause("A creates the enquiry")
    eid = new_enquiry("A", "Augmentin 1g x2", 2, "manual", MED_AUGMENTIN_1G)["id"]
    watch = Watch(eid)

    step("A asks Achrafieh and Baabda", "Both are staff targets, so they are approved at once.")
    pause("A adds the two targets")
    ok("POST", f"/api/v1/enquiries/{eid}/targets", "A",
       {"branch_ids": [BRANCH["B"], BRANCH["C"]]}, want=(201,))
    d = watch.poll()
    tb, tc = target_for(d, BRANCH["B"]), target_for(d, BRANCH["C"])

    step("A sends", "One message per target goes into the outbox with a short undo window.")
    pause("A sends")
    ok("POST", f"/api/v1/enquiries/{eid}/send", "A")
    d = wait_until(watch, lambda x: message_for(x, tc["id"]) is not None, "queued messages")
    msg_c = message_for(d, tc["id"])

    step("A changes its mind about Baabda",
         "Cancelling inside the undo window means Baabda never sees the message.")
    pause("A cancels Baabda's message")
    ok("POST", f"/api/v1/messages/{msg_c['id']}/cancel", "A")
    d = wait_until(watch, lambda x: (message_for(x, tb["id"]) or {}).get("status") == "sent",
                   "Achrafieh's message to be sent")
    print(f"   Baabda inbox: {'empty' if not inbox_has('C', eid) else 'NOT EMPTY (cancel failed)'}",
          flush=True)

    step("Achrafieh replies", "Only a human pharmacist at the target branch can answer.")
    pause("B replies available, quantity 2")
    ok("POST", f"/api/v1/targets/{tb['id']}/reply", "B",
       {"kind": "available", "quantity": 2}, want=(200,))
    watch.poll()

    step("A closes the enquiry", "Code checks the rule: available needs one available target.")
    pause("A closes as available at Achrafieh")
    ok("POST", f"/api/v1/enquiries/{eid}/close", "A",
       {"outcome": "available", "branch_id": BRANCH["B"]})
    d = watch.poll()
    print(f"   outcome: {d['enquiry']['outcome']} at Achrafieh", flush=True)


def part2_agent():
    banner("Part 2: agent flow (Wassit)")
    step("A asks Wassit in messy shorthand",
         "handled_by agent: the agent must match the medicine, propose pharmacies and wait.")
    pause("A creates the agent enquiry")
    eid = new_enquiry("A", "badde augmentin 1g 2 3elab", 2, "agent")["id"]
    watch = Watch(eid)
    if DRIVE_AGENT:
        print(f"   driving the agent: {AGENT_PY} runner.py --once --policy {POLICY}", flush=True)
    else:
        print("   now run the agent: cd agent && .venv/bin/python runner.py", flush=True)

    approved = replied = False
    deadline = time.monotonic() + AGENT_TIMEOUT
    while True:
        d = watch.poll()
        if d["enquiry"]["status"] == "closed":
            break
        if time.monotonic() > deadline:
            raise DemoError(f"enquiry still open after {AGENT_TIMEOUT}s; check the runner terminal")

        proposed = [t for t in targets(d) if t.get("status") == "proposed"]
        if proposed and not approved:
            step("Wassit proposes pharmacies",
                 "Nothing is sent until a human approves the targets.")
            pause("A approves")
            ok("POST", f"/api/v1/enquiries/{eid}/targets/approve", "A",
               {"target_ids": [t["id"] for t in proposed]})
            approved = True
            print("   approved", flush=True)

        live = [t for t in targets(d) if t.get("status") not in ("proposed", "cancelled")]
        delivered = bool(live) and all(
            t.get("status") == "sent" and (message_for(d, t["id"]) or {}).get("status") == "sent"
            for t in live)
        if delivered and not replied:
            step("Delivered; the pharmacies answer",
                 "B says unavailable, C offers an alternative (not a match), D stays silent.")
            pause("B, C and D respond")
            tb, tc = target_for(d, BRANCH["B"]), target_for(d, BRANCH["C"])
            if tb:
                ok("POST", f"/api/v1/targets/{tb['id']}/reply", "B", {"kind": "unavailable"})
            if tc:
                ok("POST", f"/api/v1/targets/{tc['id']}/reply", "C",
                   {"kind": "alternative", "text": "625mg only"})
            replied = True

        if DRIVE_AGENT:
            run_agent_once()
        time.sleep(POLL_SECONDS)

    banner("Outcome")
    print(f"   enquiry {eid[:8]} closed: outcome={d['enquiry']['outcome']}", flush=True)
    for t in targets(d):
        print(f"   {t.get('branch_name', t.get('branch_id'))}: {t.get('status')}", flush=True)
    print(f"   hitl_count: {d.get('hitl_count', 0)} (approvals and clarifications)", flush=True)


def part3_undo():
    banner("Part 3: undo after send")
    step("A sends a manual enquiry to Verdun", "Then A undoes the send.")
    pause("A creates and sends")
    eid = new_enquiry("A", "Augmentin 1g x2", 2, "manual", MED_AUGMENTIN_1G)["id"]
    watch = Watch(eid)
    ok("POST", f"/api/v1/enquiries/{eid}/targets", "A", {"branch_ids": [BRANCH["D"]]}, want=(201,))
    td = target_for(watch.poll(), BRANCH["D"])
    ok("POST", f"/api/v1/enquiries/{eid}/send", "A")
    d = wait_until(watch, lambda x: (message_for(x, td["id"]) or {}).get("status") == "sent",
                   "Verdun's message to be sent")

    queued = next(e for e in d.get("events") or []
                  if e.get("type") == "messages_queued" and not e.get("undone_at"))
    step("A undoes the send", "The stored inverse runs: a retraction goes to Verdun.")
    pause("A undoes messages_queued")
    ok("POST", f"/api/v1/events/{queued['id']}/undo", "A")
    time.sleep(UNDO_WAIT)
    d = watch.poll()
    retraction = next((m for m in d.get("messages") or []
                       if m.get("kind") == "retraction" and m.get("target_id") == td["id"]), None)
    if retraction is None:
        raise DemoError("no retraction message after undo")
    print(f"   retraction: kind=retraction status={retraction.get('status')} "
          f"body={short(retraction.get('body'), 90)}", flush=True)
    print(f"   Verdun target: {target_for(d, BRANCH['D']).get('status')}", flush=True)


def part4_stats():
    banner("Part 4: stats")
    step("Every number above is in the stats", "Cost is computed in Go from the run's tokens.")
    body = ok("GET", "/api/v1/stats?limit=50", "A")
    print(f"\n   {'enquiry':<32} {'handled_by':<10} {'outcome':<18} {'hitl':>4} {'steps':>5} {'cost USD':>11}")
    for r in body.get("data") or []:
        label = f"{r.get('enquiry_id', '')[:8]} {str(r.get('raw_request', ''))[:22]}"
        print(f"   {label:<32} {r.get('handled_by', ''):<10} {(r.get('outcome') or '-'):<18} "
              f"{r.get('hitl_count', 0):>4} {r.get('agent_steps', 0):>5} "
              f"{(r.get('cost_micros', 0) or 0) / 1_000_000:>11.6f}")
    t = body.get("totals") or {}
    print(f"\n   totals: enquiries={t.get('enquiries', 0)} agent={t.get('agent_enquiries', 0)} "
          f"hitl={t.get('hitl_count', 0)} steps={t.get('agent_steps', 0)} "
          f"cost_usd={(t.get('cost_micros', 0) or 0) / 1_000_000:.6f}", flush=True)


# ---------------------------------------------------------------- main


def main(argv=None):
    global NO_PAUSE, DRIVE_AGENT, POLICY, AGENT_TIMEOUT, UNDO_WAIT
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--no-pause", action="store_true",
                        help="do not wait for Enter between steps")
    parser.add_argument("--drive-agent", action="store_true",
                        help="run agent/runner.py --once as the agent (needs agent/.venv)")
    parser.add_argument("--policy", choices=["scripted", "model"],
                        default=os.environ.get("WASSIT_POLICY", "scripted"),
                        help="agent policy passed to the runner (default: WASSIT_POLICY or scripted)")
    parser.add_argument("--timeout", type=int, default=900,
                        help="seconds to wait for the agent to close the enquiry (default 900)")
    args = parser.parse_args(argv)

    try:
        UNDO_WAIT = float(os.environ.get("UNDO_WAIT", "5"))
    except ValueError:
        parser.error("UNDO_WAIT must be a number of seconds")
    NO_PAUSE, DRIVE_AGENT, POLICY, AGENT_TIMEOUT = (
        args.no_pause, args.drive_agent, args.policy, args.timeout)

    if DRIVE_AGENT and not AGENT_PY.exists():
        print(f"error: agent venv not found at {AGENT_PY} (see DEMO.md, prerequisites)", file=sys.stderr)
        return 1

    try:
        part1_manual()
        part2_agent()
        part3_undo()
        part4_stats()
    except DemoError as err:
        print(f"\nSTOPPED: {err}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("\nstopped", file=sys.stderr)
        return 130
    print("\nDemo finished.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
