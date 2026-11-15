# DOSYN / Wassit demo

Two demos. The 3-minute judge demo of the new UI comes first. The pharmacist enquiry demo
(terminal, 5 to 7 minutes) is below it.

## 3-minute judge demo (new UI)

Setup: backend running (see Terminal layout below), then
`python3.13 -m http.server 5500 --directory web`. `web/index.html` must point at the backend URL.
Client tickets, schedules and photo checks run on the demo adapter (`web/wassit-demo.js`,
labelled in the UI). Pharmacist enquiries are live.

**Client (about 1.5 min)**

1. Open <http://localhost:5500/#client/home>.
2. Search "Augmentin". Pick 1000mg, 2 boxes, "Just once", "Today before 18:00". Press Send.
3. Watch the ticket fill in:
   - outreach to several pharmacies at once
   - a decline
   - an offer card with the pharmacy's own words highlighted as quotes
   - a dose equivalent accepted by the rules (2 x 500mg = 1 x 1000mg)
   - a dose equivalent rejected by the rules
   - a verified photo
   - a blurry photo sent to a human
   - "no response" for the silent pharmacy (never "unavailable")
   - the best verified offer
4. Press Reserve.

Say: "Every line shows who decided: Rules, Human or Pharmacy. It never says the AI decided."

**Pharmacist (about 1 min)**

1. Switch to Pharmacist view. Choose Hamra (A).
2. Open Client tickets, then open the ticket. The client's identity is not shown.
3. Reply: `eh 3ande 2 3elab, 10$ el 3elbe, expiry 09/2027`
4. Upload a package photo.
5. Hamra becomes the best offer.

Say: "The reply was free-text Arabizi. Wassit read it into an offer. The rules ranked it."

**Scheduled medicines (about 20 s)**

Back on "Ask Wassit", pick Concor 5mg, 1 box, then "Every week", "Friday" and a pickup time, and Send. A ticket opens for this cycle. Open "Scheduled medicines": the card shows FRI, "No bulk", the next search time and the ticket history.

**Arabic (about 10 s)**

Switch the language to العربية. The whole UI turns right to left.

---

## Pharmacist enquiry demo: judge run order (5 to 7 minutes)

Wassit is a pharmacist-to-pharmacist medicine availability enquiry system. A person can run it
by hand; Wassit (an agent) runs the same workflow through the same API. Code decides what is
allowed. The model only decides what to do next. The agent never says "available".

### Prerequisites

- Docker Desktop running (Postgres 16 for the backend).
- Ollama on the host with the model: `ollama pull qwen3.5:35b-a3b`.
- Python 3.13 (`python3.13 --version`).
- Agent venv, once:
  `cd agent && python3.13 -m venv .venv && .venv/bin/pip install "cornilia @ git+https://github.com/gamercreative/Cornilia.git@v0.1.0" pytest`
- Backend config, once: `cd backend && cp -n .env.example .env`.

### Terminal layout

| Terminal | Command (from the project root unless shown) |
| --- | --- |
| 1: model | `ollama serve` |
| 2: backend | `cd backend && make up && make migrate`, then `UNDO_WINDOW=3s WAIT_UNIT=2s AGENT_TOKEN=dev-agent-token AGENT_PRICE_IN_PER_MTOK=0.2 AGENT_PRICE_OUT_PER_MTOK=0.8 make run` |
| 3: agent | `cd agent && source .venv/bin/activate && python runner.py` (polls every 3s) |
| 4: demo | `python3.13 scripts/demo_flow.py` |

Before the audience arrives: `bash scripts/seed_demo.sh` (checks health, the four logins, and
the stats totals). It only reads.

Terminal 3 is not needed if you use `python3.13 scripts/demo_flow.py --drive-agent`, which runs
one agent pass per poll itself.

### The four parts

Press Enter to move between steps. Each step prints one narration line and an event timeline.

1. **Manual flow (about 1.5 min).** A asks Achrafieh and Baabda by hand, sends, and cancels
   Baabda inside the undo window. Say: "Cancel inside the window and the target never sees it."
   The output shows `Baabda inbox: empty`. B replies available, A closes available.
2. **Agent flow (about 3 min).** A types "badde augmentin 1g 2 3elab" with handled_by agent.
   Say: "Wassit matches the medicine and proposes pharmacies, but a human approves before anything
   is sent." A approves. B replies unavailable, C offers "625mg only" (shown as an option, not a
   match), D stays silent. Wassit follows up D once, then closes unable_to_confirm. Say: "The
   agent cannot close available. Only a pharmacist's reply can."
3. **Undo after send (about 1 min).** A sends a manual enquiry to Verdun, then undoes it.
   Say: "Undo runs the stored inverse: a retraction goes out and the target is cancelled."
   The output shows the retraction and the `event_undone` event in the timeline.
4. **Stats (about 30 s).** One table: enquiry, handled_by, outcome, hitl, steps, cost USD.
   Say: "Every human approval is counted, and every agent run has a cost."

Where to look:

- **hitl_count:** the Part 2 outcome block, the stats table, and `hitl_count` in
  `GET /api/v1/enquiries/{id}`. It counts `clarified` and `targets_approved` events that are not undone.
- **Undo:** Part 3 (retraction and `event_undone` in the timeline), and the `undone_at` mark on the
  original `messages_queued` event.
- **Cost:** Part 4 (per enquiry, and totals). Cost is micro-dollars in the API and USD in the table.

### Agent evaluation (optional, after the demo)

- 20 test cases with the model: `python3.13 scripts/run_cases.py --policy model`
- The same 20 cases with the fallback policy: `python3.13 scripts/run_cases.py --policy scripted`

Both print one results table.

### If the model is slow or down

Use the scripted policy. It runs the same tools through the same API without a model, so every
state change, rule and count still happens. It is deterministic, so the outcome is the same every time:

```bash
WASSIT_POLICY=scripted python3.13 scripts/demo_flow.py --drive-agent --policy scripted
```

Say so plainly on stage: "The model is slow today, so this run uses the scripted policy. The rules,
the API and the numbers are the same."

If the backend is down, see the start commands printed by `bash scripts/seed_demo.sh`.

### Logins (PIN 1234 for all)

| Email | Pharmacy | Role in the demo |
| --- | --- | --- |
| a@wassit.demo | Hamra | Requester (A) |
| b@wassit.demo | Achrafieh | Target (B) |
| c@wassit.demo | Baabda | Target (C), gives an alternative |
| d@wassit.demo | Verdun | Target (D), stays silent |
| e@wassit.demo | Byblos | Not used in the demo |

### Notes for the presenter

- The backend has no auth beyond staff PINs and the agent token. Run it on your laptop only.
- Demo timing uses `UNDO_WINDOW=3s` and `WAIT_UNIT=2s`. Production defaults are 60s and 1m.
- Stop all terminals with Ctrl-C. Data stays in the Docker volume; `make down` keeps it too.
