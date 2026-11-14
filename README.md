# Wassit (AlphaHack 2026)

Wassit is a pharmacy out-of-stock enquiry agent built for the AlphaHack hackathon. A Go
backend with PostgreSQL stores enquiries, pharmacies and every agent run. A Python agent
on Cornilia handles each enquiry by contacting pharmacies through the backend.

- Product spec: [CONTRACT.md](CONTRACT.md)
- Stack facts and rules for AI contributors: [AGENTS.md](AGENTS.md)
- Demo run order: [docs/DEMO.md](docs/DEMO.md)

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
```

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
docs/            PLAN, WORKFLOW, DEMO, STRUCTURE, lead-prompts/, research/, archive/
```

Where a new file goes, and why the layout is this way: [docs/STRUCTURE.md](docs/STRUCTURE.md).
