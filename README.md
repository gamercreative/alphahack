# DOSYN / Wassit (AlphaHack 2026)

DOSYN removes the weekly hunt for a scarce medicine. Wassit is its AI pharmacist chatbot, for
clients and for pharmacists.

How it works:

1. The client searches the catalogue and asks once, or on a schedule (for example every Friday).
   There is no bulk buying.
2. Wassit messages several pharmacies at once. Pharmacies never see who the client is.
3. Pharmacies reply in free text: Arabic, Arabizi, French or English. Wassit reads each reply into
   a structured offer and keeps the quoted words as evidence.
4. Go rules decide what matches, including dose equivalents. 2 x 500mg = 1 x 1000mg only if the
   active ingredient and form are the same and the product is listed in the government reference.
5. Package photos are checked against the reference. A photo alone is not proof.
6. The best verified offer comes back in a ticket: lowest total, then latest expiry.
7. Every event in the ticket shows who decided: Rules, Human or Pharmacy. Never "AI decided".

- Product spec: [CONTRACT.md](CONTRACT.md)
- One page for judges: [documents/SUBMISSION.md](documents/SUBMISSION.md)
- Stack facts and rules for AI contributors: [AGENTS.md](AGENTS.md)
- Demo run order: [docs/DEMO.md](docs/DEMO.md)

## What is live vs demo

**Live (Go backend + PostgreSQL + agent):**

- Pharmacist-to-pharmacist enquiries
- Agent runs, with steps and cost
- Outbox with undo
- Live updates over SSE
- Structured offers with quoted evidence
- Speech (macOS `say`)
- New: government reference, stock update, content filter

**Demo adapter (`web/wassit-demo.js`, clearly labelled in the UI, data in localStorage):**

- Client accounts
- Client tickets
- Schedules
- Photo checks

The approved backend design for the demo parts is in
[documents/CONTRACT_PROPOSAL_dosyn_redesign.md](documents/CONTRACT_PROPOSAL_dosyn_redesign.md) (Phase 3B).
It is designed, not built.

## Model

Ollama `qwen3.5:35b-a3b`. On the event LAN the host is `http://10.31.216.24:11434`.

- Cornilia: `CORNILIA_BASE_URL=http://10.31.216.24:11434/v1` (ends in `/v1`)
- Backend: `OLLAMA_BASE_URL=http://10.31.216.24:11434` (no `/v1`)

## Run it

All commands run from the repo root. The full judge run order, with the demo timing
variables, is in [docs/DEMO.md](docs/DEMO.md).

```bash
ollama serve                                                   # model: qwen3.5:35b-a3b
(cd backend && cp -n .env.example .env && make up && make migrate && make run)   # API on :8080
(cd agent && python3.13 -m venv .venv && .venv/bin/pip install "cornilia @ git+https://github.com/gamercreative/Cornilia.git@v0.1.0" pytest)
(cd agent && .venv/bin/python runner.py)                       # the agent, polls the backend
bash scripts/seed_demo.sh                                      # read-only check: health, logins, stats
python3.13 scripts/demo_flow.py                                # narrated demo
python3.13 -m http.server 5500 --directory web                 # web frontend on :5500
```

The web frontend is static. `web/index.html` sets the backend URL (`window.VITE_API_BASE_URL`,
near the top). Point it at your backend, for example `http://localhost:8080`, then open
<http://localhost:5500/#client/home>.

Details are in [agent/README.md](agent/README.md) and [backend/AGENTS.md](backend/AGENTS.md).

## Test it

```bash
(cd backend && go build ./... && go vet ./... && go test ./...)
(cd cornilia-guard/go && go test ./...)
(cd agent && .venv/bin/python -m pytest -q)
(cd cornilia-guard/python && ../../agent/.venv/bin/python -m pytest -q)
agent/.venv/bin/python -m pytest -q tests                      # repo layout smoke tests
```

## Project structure

```
agent/           Python enquiry agent (Cornilia tools, prompt, cases, tests)
backend/         Go API + PostgreSQL (cmd/, internal/, db/, scripts/)
cornilia-guard/  signed tool-call layer (go/ gateway, python/ package)
scripts/         drivers against a live backend: demo, acceptance, case runner, seed
tests/           repo-level smoke tests
web/             static frontend (index.html, app.js, wassit-ui.js, wassit-demo.js)
docs/            PLAN, WORKFLOW, DEMO, STRUCTURE, lead-prompts/, research/, archive/
```

Where a new file goes, and why the layout is this way: [docs/STRUCTURE.md](docs/STRUCTURE.md).
