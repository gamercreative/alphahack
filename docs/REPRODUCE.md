# Reproduce: fresh clone to running demo (macOS)

Every command runs from the repo root unless a `cd` is shown. Nothing here needs a cloud key:
the backend runs with `AI_PROVIDER=fake` and the agent can run with the scripted policy.

## 1. Prerequisites

| Tool | Version | Used for | Check |
| --- | --- | --- | --- |
| Docker Desktop | any recent, with `docker compose` v2 | PostgreSQL 16 (`postgres:16-alpine`) | `docker compose version` |
| Go | 1.25 | backend | `go version` |
| Python | 3.13 (`python3.13`; Cornilia needs >= 3.10, macOS system 3.9 is too old) | agent, scripts, static web server | `python3.13 --version` |
| curl, lsof | macOS built-ins | health checks, port checks | |
| Ollama | optional | model `qwen3.5:35b-a3b` for the model policy | `ollama list` |
| psql | optional | not needed, see Troubleshooting | |
| node | optional | JS syntax check in `scripts/verify_all.sh` | |

## 2. Port map

| Port | Process | Notes |
| --- | --- | --- |
| 5432 | PostgreSQL (Docker) | change the host port with `DB_PORT` (compose) |
| 8080 | Go backend, default `PORT` | what `scripts/dev_up.sh` uses |
| 5500 | static web (`python3.13 -m http.server`) | also in `.claude/launch.json` |
| 11434 | Ollama | event LAN host: `http://10.31.216.24:11434` |
| 8113 | Go backend, live demo instance | the default in `web/index.html` and `scripts/check_frontend_sync.py` |
| 5573 | PostgreSQL of the live demo instance | do not reuse while the demo is running |
| 8000 | Cornilia HTTP server | not used: the agent runs as `agent/runner.py` |

## 3. Environment variables

### Backend (`backend/.env`, copied from `backend/.env.example`; real env vars win)

| Variable | Default | Demo value | Meaning |
| --- | --- | --- | --- |
| `PORT` | 8080 | 8113 on the live demo | HTTP listen port |
| `DATABASE_URL` | none, required | `postgres://alphahack:alphahack@localhost:5432/alphahack?sslmode=disable` | PostgreSQL |
| `AI_PROVIDER` | anthropic if `ANTHROPIC_API_KEY` is set, else fake | `fake` | provider for the template chat loop (`/v1/...`), not for Wassit |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | `http://10.31.216.24:11434` | no `/v1` (native `/api/chat`) |
| `OLLAMA_MODEL` | `qwen3.5:35b-a3b` | same | |
| `OLLAMA_NUM_CTX` | 0 | 16384 or more with tools | context window |
| `UNDO_WINDOW` | 60s | `3s` | outbox delay; cancel inside it and the message is never delivered |
| `WAIT_UNIT` | 1m | `2s` | length of one agent `wait()` minute |
| `AGENT_TOKEN` | `dev-agent-token` in the example file; empty disables the agent | same | `Authorization: Agent <token>` |
| `AGENT_PRICE_IN_PER_MTOK` / `AGENT_PRICE_OUT_PER_MTOK` | 0.10 / 0.50 | same | run cost, USD per million tokens |
| `SPEECH_PROVIDER` | azure if `AZURE_SPEECH_KEY` is set, else `say` on macOS, else fake | `say` | `POST /api/v1/speech` |
| `UPLOAD_DIR` | `./uploads` | same | reference documents go to `UPLOAD_DIR/reference` |
| `REQUEST_TIMEOUT`, `CORS_ALLOWED_ORIGINS`, `LOG_LEVEL` | 300s, `*`, info | same | |

### Agent (`agent/.env`, copied from `agent/.env.example`; loaded by `runner.py`, real env vars win)

| Variable | Default | Demo value |
| --- | --- | --- |
| `BACKEND_URL` | `http://localhost:8080` | the backend you started |
| `AGENT_TOKEN` | `dev-agent-token` | must equal the backend's |
| `WASSIT_POLICY` | `model` | `scripted` runs with no model |
| `CORNILIA_PROVIDER` | `ollama` | `ollama` |
| `CORNILIA_BASE_URL` | `http://localhost:11434/v1` | `http://10.31.216.24:11434/v1` (ends in `/v1`) |
| `CORNILIA_API_KEY` | `ollama` | `ollama` |
| `CORNILIA_MODEL` | Cornilia default `qwen3.5:9b`; the runner defaults to `qwen3.5:35b-a3b` | `qwen3.5:35b-a3b` |
| `CORNILIA_MAX_STEPS` | 12 in the runner | 12 |
| `CORNILIA_TOOLS_DIR` | `agent/tools` (set by the runner) | leave unset |

### Web

One line near the top of `web/index.html`: `window.VITE_API_BASE_URL`. The committed default is
`http://10.31.216.148:8113` (the live demo backend on the event LAN). For a local run set it to
`http://localhost:8080`. Do not commit that change during the event.

### Scripts

| Script | Variable | Default |
| --- | --- | --- |
| `scripts/seed_demo.sh` | `BACKEND_URL`, `PYTHON` | `http://localhost:8080`, `python3` (use `PYTHON=python3.13`) |
| `scripts/check_frontend_sync.py` | `--backend URL` | `http://localhost:8113` |
| `backend/scripts/smoke.sh` | `BASE_URL` | `http://localhost:8080` |
| `scripts/test_ollama.sh` | `OLLAMA`, `MODEL` | `http://10.31.216.24:11434`, `qwen3.5:35b-a3b` |

## 4. Fast path (one command)

```bash
bash scripts/dev_up.sh              # db 5432, migrations, backend 8080, web 5500
bash scripts/dev_up.sh --with-agent # same, plus agent/runner.py (scripted policy unless WASSIT_POLICY=model)
bash scripts/dev_down.sh            # stop what dev_up started (add --keep-db to leave Postgres up)
```

`dev_up.sh` refuses to start if 8080 or 5500 is busy, keeps pid files in `.dev/` and logs in
`.dev/logs/`. It does not create `backend/.env`; it passes `PORT`, `DATABASE_URL`,
`AI_PROVIDER=fake` and `AGENT_TOKEN=dev-agent-token` in the environment.

## 5. Step by step

| # | Command | Expected output |
| --- | --- | --- |
| 1 | `git clone <repo> && cd alphahack` | repo root with `backend/`, `agent/`, `web/` |
| 2 | `(cd backend && cp -n .env.example .env)` | `backend/.env` exists (git-ignored) |
| 3 | `(cd backend && make up)` | `Container ...-db-1  Healthy` |
| 4 | `(cd backend && make migrate)` | `apply  0001_init` through `apply  0007_gov_reference`, then `migrate up: done`. A second run prints `skip ... (already applied)` |
| 5 | `(cd backend && make run)` (keep this terminal open) | server log line with the listen port |
| 6 | `curl localhost:8080/healthz` | HTTP 200 JSON |
| 7 | `PYTHON=python3.13 bash scripts/seed_demo.sh` | health ok, demo logins `a` to `d` at `@wassit.demo` succeed (PIN `1234`), stats totals. Read-only |
| 8 | `(cd agent && python3.13 -m venv .venv && .venv/bin/pip install "cornilia @ git+https://github.com/gamercreative/Cornilia.git@v0.1.0" pytest)` | pip ends with `Successfully installed ... cornilia-0.1.0 ...` |
| 9 | `(cd agent && .venv/bin/python -m pytest -q)` | all tests pass, no network used |
| 10 | `(cd agent && .venv/bin/python runner.py --policy scripted)` or, with Ollama, `(cd agent && cp -n .env.example .env)`, edit to the Ollama lines, then `.venv/bin/python runner.py` | the runner polls `GET /api/v1/agent/wakeups` every 3 s |
| 11 | `python3.13 -m http.server 5500 --directory web` | `Serving HTTP on :: port 5500` |
| 12 | set `window.VITE_API_BASE_URL` in `web/index.html` to `http://localhost:8080`, open <http://localhost:5500/#client/home> | DOSYN client home; client data is the labelled demo adapter |
| 13 | `python3.13 scripts/demo_flow.py` (add `--drive-agent --policy scripted` to run without step 10) | narrated pharmacist enquiry demo, see `docs/DEMO.md` |

For demo timing start the backend with `UNDO_WINDOW=3s WAIT_UNIT=2s`, as the live instance does.

Model check before using the model policy: `bash scripts/test_ollama.sh` (three PASS lines per
path: backend native API, Cornilia OpenAI API, and the Arabizi "alternative is not a match" case).

## 6. Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| `psql: command not found`, or you have no `psql` on the host | not installed | Nothing to do. `backend/scripts/migrate.sh` falls back by itself: with `DATABASE_URL` set it runs `psql` in a throwaway `postgres:16` container (localhost is rewritten to `host.docker.internal`); with `DATABASE_URL` unset it uses `docker compose exec db psql`. `make migrate` exports nothing, so it takes the compose path |
| Backend exits at start | `DATABASE_URL` missing or Postgres not up | `(cd backend && make up)`, check `backend/.env` |
| `dev_up: port 8080 is busy` | another backend, or `make docker-up` app container | stop it, or run the backend by hand with `PORT=<other>` |
| `bind: address already in use` on 5432 | a local Postgres | `DB_PORT=5433 docker compose up -d --wait db` and use port 5433 in `DATABASE_URL` |
| Web shows network errors | `window.VITE_API_BASE_URL` points at the LAN demo host | set it to your backend URL |
| 401 `unauthorized` | staff token expired (12 h) or missing | log in again |
| Agent gets 401 or 403 | `AGENT_TOKEN` differs between backend and agent, or is empty on the backend | set both to the same value |
| Agent replies time out with Ollama | wrong base URL shape | Cornilia: ends in `/v1`. Backend: no `/v1` |
| `ModuleNotFoundError: cornilia` | system `python3` used | use `agent/.venv/bin/python` |
| Undo window too long for a demo | default 60 s | `UNDO_WINDOW=3s` |
| Client tickets vanish | demo adapter data is in `localStorage` key `dosyn-demo-v2` | expected; clear the key to reset the demo |
| `POST /api/v1/speech` returns 409 | speech is off | `SPEECH_PROVIDER=say` on macOS, or `fake` |

## 7. Reset

`(cd backend && make down)` stops containers and keeps data. To wipe the database, remove the
`pgdata` volume yourself (`docker compose down -v` in `backend/`); this deletes all data.
