# DOSYN / Wassit

[![CI](https://github.com/gamercreative/alphahack/actions/workflows/ci.yml/badge.svg)](https://github.com/gamercreative/alphahack/actions/workflows/ci.yml)

An AI pharmacist agent that finds a scarce medicine across many pharmacies at once.
**AI reads, Go rules decide, humans approve.** Built in 48 hours for AlphaHack 2026.

In Lebanon a scarce medicine means a weekly hunt: calls, walks, arguments about strengths and
prices, and no way to tell if a box is genuine. DOSYN removes the hunt. Wassit is its agent, for
clients and for pharmacists.

## What Wassit does

1. **The client asks once, or on a schedule** (for example every Friday). No bulk buying.
2. **Wassit picks the nearest pharmacies and messages them at once.** Pharmacies see the
   "DOSYN client desk", never the client.
3. **Pharmacists reply in free text**: Arabic, Arabizi, French or English. Wassit reads each
   reply into a structured offer and keeps the quoted words as evidence.
4. **Go rules decide what matches**, including dose equivalents: 2 x 500mg = 1 x 1000mg only if
   the active ingredient and form are the same and the product is in the government reference.
5. **Package photos are checked in Go** against the reference code and the expiry. Wassit adds
   its own reading of the photo. A photo alone is never proof.
6. **The best verified offer comes back in a ticket**: lowest total, then latest expiry. The
   model never ranks offers.
7. **Every event shows who decided**: Rules, Human or Pharmacy (Wassit only relays). Never
   "AI decided".

### Where a human approves

| Step | Who | Enforced by |
| --- | --- | --- |
| Which pharmacies are contacted (pharmacist enquiry) | the requesting pharmacist approves the list | `POST /api/v1/enquiries/{id}/targets/approve` |
| Which pharmacies are contacted (client ticket) | Go approves only the ranked nearest pharmacies; anything else waits | `deskAutoApprove` in `backend/internal/services/client_tickets.go` |
| Any message leaving the system | an undo window before the outbox sends it | `UNDO_WINDOW`, `POST /api/v1/messages/{id}/cancel` |
| "Available" or "unavailable", and photo uploads | only a human at the target pharmacy; the agent gets 403 | route tags in `backend/internal/handlers/` |
| An unclear or mismatched photo | a human reviewer | `POST /api/v1/photos/{id}/review` |
| Reserving the best offer | the client | `POST /api/v1/client/tickets/{id}/reserve` |

The full rule list is in [CONTRACT.md](CONTRACT.md). Where each rule is enforced in code:
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#where-each-product-rule-is-enforced).

## Live vs demo

Everything below runs on the Go backend and PostgreSQL. Nothing runs on the old browser demo
adapter any more (it was removed). The only demo part is the seeded data.

| Feature | Status | Where |
| --- | --- | --- |
| Pharmacist-to-pharmacist enquiries | live | `/api/v1/enquiries/*`, `/api/v1/targets/*` |
| Client login | live | `POST /api/v1/client/auth/login` (phone + PIN) |
| Client tickets (thread, best offer, reserve) | live | `/api/v1/client/tickets/*` |
| Schedules (for example every Friday) | live | `/api/v1/client/schedules/*` and a scheduler in the backend |
| Package photos with the agent's reading | live | `/api/v1/targets/{id}/photos`, `/api/v1/photos/*` |
| Structured offers with quoted evidence | live | `/api/v1/targets/{id}/offers` |
| Dose equivalence | live | `backend/internal/services/enq_dose.go` |
| Agent runs with steps, tokens and cost | live | `agent/runner.py`, `/api/v1/agent/*`, `/api/v1/stats` |
| Outbox with undo window | live | `UNDO_WINDOW` |
| Live updates | live | SSE on enquiries and client tickets |
| Government reference, stock update, content filter | live | `/api/v1/reference/*`, `PUT /api/v1/branches/{id}/inventory/{medication_id}` |
| Speech | live | `POST /api/v1/speech` (macOS `say`) |
| Pharmacies, catalogue, demo accounts | **demo seed** | migrations `0003`, `0005`, `0008` |
| Government reference rows | **demo seed**, labelled "Demo seed, not real MoPH data" | migration `0007` |

## Benchmarks

20 **placeholder** cases in `agent/cases/cases.jsonl`, replayed by `scripts/run_cases.py` with
scripted pharmacists. Details, misses and caveats: [docs/BENCHMARKS.md](docs/BENCHMARKS.md).

| Model / policy | Matches | Wall time | Cost |
| --- | --- | --- | --- |
| `qwen3.5:35b-a3b`, local Ollama | **16/20** | 26.2 min | $0 (own GPU) |
| `claude-haiku-5-5` | 15/20 | 14.6 min | $0.108 |
| scripted, no model | 14/20 | 12.1 min | $0 |

The runs did not use identical code: guards for small models were added before the qwen run
(see BENCHMARKS.md). Live runs on 2026-11-15 with `qwen3.5:35b-a3b` on the current code:
case-01 closed `available` in 66 s, case-14 closed `unable_to_confirm` in 151 s, both as expected.

## Quickstart

Needs Docker Desktop, Go 1.25 and `python3.13`. No model and no API key: the backend runs with
`AI_PROVIDER=fake` and the agent has a scripted policy.

```bash
bash scripts/dev_up.sh        # Postgres 5432, migrations, backend 8080, web 5500
curl localhost:8080/healthz
bash scripts/dev_down.sh      # stop what dev_up started
```

Then set `window.VITE_API_BASE_URL` in `web/index.html` to `http://localhost:8080` (the committed
value is the demo backend on the event LAN) and open <http://localhost:5500/#client/home>.

- Client: phone `+96170000000`, PIN `1234`.
- Pharmacists: `a@wassit.demo` to `e@wassit.demo`, PIN `1234`.
- Client desk reviewer (photo reviews): `desk@wassit.demo`, PIN `1234`.

Add the agent: create `agent/.venv` (below), then `bash scripts/dev_up.sh --with-agent`.
Step by step, with expected output and troubleshooting: [docs/REPRODUCE.md](docs/REPRODUCE.md).

### Run it by hand

```bash
ollama serve                                                   # model: qwen3.5:35b-a3b
(cd backend && cp -n .env.example .env && make up && make migrate && make run)   # API on :8080
(cd agent && python3.13 -m venv .venv && .venv/bin/pip install "cornilia @ git+https://github.com/gamercreative/Cornilia.git@v0.1.0" pytest)
(cd agent && .venv/bin/python runner.py)                       # the agent, polls the backend
python3.13 scripts/demo_flow.py                                # narrated pharmacist demo
python3.13 -m http.server 5500 --directory web                 # web frontend on :5500
```

Model URLs: Cornilia wants `CORNILIA_BASE_URL=http://<ollama-host>:11434/v1` (ends in `/v1`); the
backend wants `OLLAMA_BASE_URL=http://<ollama-host>:11434` (no `/v1`).

### Test it

```bash
bash scripts/verify_all.sh                                     # every offline check, pass/fail table
BACKEND_URL=http://localhost:8080 bash scripts/verify_all.sh   # plus read-only live checks
```

CI runs on every push and pull request: Go build, vet and tests; agent and repo pytest; JS syntax;
frontend/backend route sync; shell syntax ([.github/workflows/ci.yml](.github/workflows/ci.yml)).

## Architecture

```
web/ (static HTML/JS) --REST + SSE--> backend/ (Go 1.25) --> PostgreSQL 16
agent/ (Python, Cornilia) --REST, own token--> backend/
agent/ --OpenAI-compatible API--> Ollama qwen3.5:35b-a3b
```

The agent acts only through the backend's HTTP API with its own credential. All product rules
live in Go. Diagrams and the data model: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Documentation

| Read this | For |
| --- | --- |
| [docs/REPRODUCE.md](docs/REPRODUCE.md) | fresh clone to running demo, env vars, ports, troubleshooting |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | components, flow diagrams, data model, where each rule is enforced |
| [docs/API.md](docs/API.md) | every route, who may call it, auth headers, error envelope |
| [docs/TESTING.md](docs/TESTING.md) | every check and what it proves |
| [docs/BENCHMARKS.md](docs/BENCHMARKS.md) | model runs on the 20 placeholder cases |
| [docs/DEMO.md](docs/DEMO.md) | judge demo run order |
| [docs/STRUCTURE.md](docs/STRUCTURE.md) | repo layout and where a new file goes |
| [docs/TEST_REPORT_2026-11-15.md](docs/TEST_REPORT_2026-11-15.md) | full test report of 2026-11-15 |
| [CONTRACT.md](CONTRACT.md) | product spec and rules |
| [backend/openapi.yaml](backend/openapi.yaml) | API schemas |
| [agent/README.md](agent/README.md) | the agent runner |
| [AGENTS.md](AGENTS.md), [backend/AGENTS.md](backend/AGENTS.md) | stack facts and rules for AI contributors |
| [docs/PLAN.md](docs/PLAN.md), [docs/WORKFLOW.md](docs/WORKFLOW.md), [docs/lead-prompts/](docs/lead-prompts/) | how the build was planned and split across agents |

## Submission

- [documents/DOSYN_Wassit_Submission.pdf](documents/DOSYN_Wassit_Submission.pdf): the submission document
- [documents/jury/](documents/jury/): material for the jury
- [documents/SUBMISSION.md](documents/SUBMISSION.md): one page for judges

## Project structure

```
agent/           Python agent: Cornilia tools, prompt, cases, tests
backend/         Go API + PostgreSQL: cmd/, internal/, db/migrations/, openapi.yaml
cornilia-guard/  signed tool-call layer (Go gateway, Python package); not mounted yet
scripts/         dev_up, dev_down, verify_all; drivers against a live backend
tests/           repo-level tests (layout, OpenAPI routes, live API, end to end)
web/             static frontend (index.html, app.js, wassit-ui.js)
docs/            the docs above, plus lead-prompts/, research/, archive/, assets/
documents/       submission material
```
