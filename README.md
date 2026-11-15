# DOSYN / Wassit (AlphaHack 2026)

DOSYN removes the weekly hunt for a scarce medicine. Wassit is its AI pharmacist chatbot, for
clients and for pharmacists. Code decides what is allowed; the model only decides what to do next.

## How it works

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

## 60-second quickstart

Needs Docker Desktop, Go 1.25 and `python3.13`. No model and no API key are required.

```bash
bash scripts/dev_up.sh        # Postgres 5432, migrations 0001-0007, backend 8080, web 5500
curl localhost:8080/healthz
bash scripts/dev_down.sh      # stop what dev_up started
```

Then set `window.VITE_API_BASE_URL` in `web/index.html` to `http://localhost:8080` (the committed
value is the live demo backend on the event LAN) and open <http://localhost:5500/#client/home>.
Demo logins: `a@wassit.demo` to `e@wassit.demo`, PIN `1234`.

Add the agent: create `agent/.venv` (below), then `bash scripts/dev_up.sh --with-agent`.
Step by step, with expected output and troubleshooting: [docs/REPRODUCE.md](docs/REPRODUCE.md).

## Documentation

| Read this | For |
| --- | --- |
| [docs/REPRODUCE.md](docs/REPRODUCE.md) | fresh clone to running demo, env vars, ports, troubleshooting |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | components, flow diagrams, data model, where each rule is enforced |
| [docs/API.md](docs/API.md) | every route, who may call it, auth headers, error envelope |
| [docs/TESTING.md](docs/TESTING.md) | every check and what it proves; DB integration tests |
| [docs/DEMO.md](docs/DEMO.md) | judge demo run order |
| [CONTRACT.md](CONTRACT.md) | product spec and rules |
| [documents/SUBMISSION.md](documents/SUBMISSION.md) | one page for judges |
| [documents/CONTRACT_PROPOSAL_dosyn_redesign.md](documents/CONTRACT_PROPOSAL_dosyn_redesign.md) | Phase 3B design (client backend), not built |
| [documents/STATUS_2026-11-15.md](documents/STATUS_2026-11-15.md) | status on 2026-11-15 |
| [docs/STRUCTURE.md](docs/STRUCTURE.md) | repo layout and where a new file goes |
| [AGENTS.md](AGENTS.md), [backend/AGENTS.md](backend/AGENTS.md) | stack facts and rules for AI contributors |
| [agent/README.md](agent/README.md) | the agent runner |
| [backend/openapi.yaml](backend/openapi.yaml) | API schemas |

## What is live vs demo

| Feature | Status | Where |
| --- | --- | --- |
| Pharmacist-to-pharmacist enquiries | live | Go backend + PostgreSQL |
| Agent runs with steps and cost | live | `agent/runner.py`, `/api/v1/agent/*`, `/api/v1/stats` |
| Outbox with undo window | live | `UNDO_WINDOW` |
| Live updates | live | SSE, `GET /api/v1/enquiries/{id}/stream` |
| Structured offers with quoted evidence | live | `/api/v1/targets/{id}/offers` |
| Speech | live | `POST /api/v1/speech` (macOS `say`) |
| Government reference, stock update, content filter | live | `/api/v1/reference/*`, `PUT /api/v1/branches/{id}/inventory/{medication_id}` |
| Client accounts | demo adapter | `web/wassit-demo.js`, `localStorage` key `dosyn-demo-v2` |
| Client tickets | demo adapter | same |
| Schedules | demo adapter | same |
| Photo checks | demo adapter | same |

The demo adapter is labelled in the UI. Its backend is designed (Phase 3B), not built.

## Model

Ollama `qwen3.5:35b-a3b`. On the event LAN the host is `http://10.31.216.24:11434`.

- Cornilia: `CORNILIA_BASE_URL=http://10.31.216.24:11434/v1` (ends in `/v1`)
- Backend: `OLLAMA_BASE_URL=http://10.31.216.24:11434` (no `/v1`)

## Run it by hand

All commands run from the repo root.

```bash
ollama serve                                                   # model: qwen3.5:35b-a3b
(cd backend && cp -n .env.example .env && make up && make migrate && make run)   # API on :8080
(cd agent && python3.13 -m venv .venv && .venv/bin/pip install "cornilia @ git+https://github.com/gamercreative/Cornilia.git@v0.1.0" pytest)
(cd agent && .venv/bin/python runner.py)                       # the agent, polls the backend
bash scripts/seed_demo.sh                                      # read-only check: health, logins, stats
python3.13 scripts/demo_flow.py                                # narrated demo
python3.13 -m http.server 5500 --directory web                 # web frontend on :5500
```

## Test it

```bash
bash scripts/verify_all.sh                                     # every offline check, pass/fail table
BACKEND_URL=http://localhost:8080 bash scripts/verify_all.sh   # plus read-only live checks
```

Individual commands and what each proves: [docs/TESTING.md](docs/TESTING.md).

## Project structure

```
agent/           Python enquiry agent (Cornilia tools, prompt, cases, tests)
backend/         Go API + PostgreSQL (cmd/, internal/, db/, scripts/)
cornilia-guard/  signed tool-call layer (go/ gateway, python/ package)
scripts/         dev_up, dev_down, verify_all; drivers against a live backend: demo, acceptance, case runner, seed
tests/           repo-level smoke tests
web/             static frontend (index.html, app.js, wassit-ui.js, wassit-demo.js)
docs/            REPRODUCE, ARCHITECTURE, API, TESTING, DEMO, STRUCTURE, PLAN, WORKFLOW, lead-prompts/, research/, archive/
```

Where a new file goes, and why the layout is this way: [docs/STRUCTURE.md](docs/STRUCTURE.md).
