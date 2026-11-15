# Testing

One command for everything offline: `bash scripts/verify_all.sh`. It prints a PASS / FAIL / SKIP
table and keeps each check's full output in `.dev/logs/verify/`. Live checks run only when
`BACKEND_URL` is set.

## Checks

| Check | Command (from the repo root) | Needs | What it proves |
| --- | --- | --- | --- |
| Backend build, vet, unit tests | `(cd backend && make check)` | Go | The code compiles, passes `go vet`, and handler, service and rule tests pass (dose rules, best offer, content filter, auth matrix). DB integration tests skip without `TEST_DATABASE_URL` |
| SQL and layering rules | `(cd backend && make check-queries)` | bash; a migrated database for the prepare step | One statement per file in `internal/queries/sql/`, no `SELECT *`, positional params, only `internal/queries` touches the DB |
| Guard (Go) | `(cd cornilia-guard/go && go test ./...)` | Go | The signed tool-call gateway |
| Agent | `(cd agent && .venv/bin/python -m pytest -q)` | `agent/.venv` | Tools, runner, scripted policy, offers and reference tools. No network, no model |
| Guard (Python) | `(cd cornilia-guard/python && ../../agent/.venv/bin/python -m pytest -q)` | `agent/.venv` | The Python guard package |
| Repo tests | `agent/.venv/bin/python -m pytest -q tests` | `agent/.venv` | Repo layout, script `--help`, OpenAPI route list (`tests/`), plus `tests/api` and `tests/e2e` suites |
| Frontend sync, static | `python3.13 scripts/check_frontend_sync.py --static` | Python | Every `apiCall` / `apiAudio` / `EventSource` URL in `web/app.js` is a registered backend route, and every snake_case field the UI reads exists as a Go JSON tag |
| Frontend sync, live | `python3.13 scripts/check_frontend_sync.py --backend http://localhost:8080` | running backend | The above, plus every GET the frontend makes returns 2xx with the demo logins, and one `POST /api/v1/speech`. Read-only. Default backend is `http://localhost:8113` |
| Demo readiness | `PYTHON=python3.13 BACKEND_URL=http://localhost:8080 bash scripts/seed_demo.sh` | running backend | Health, the demo logins, stats totals. Read-only |
| Endpoint smoke | `(cd backend && BASE_URL=http://localhost:8080 make smoke)` | running backend | Every endpoint answers with the expected status and the error envelope. Writes data, so use a throwaway database |
| Acceptance | `python3.13 scripts/acceptance.py` | running backend | The acceptance checks of `CONTRACT.md` |
| Agent cases | `python3.13 scripts/run_cases.py`, then `scripts/show_results.py` | running backend | The cases in `agent/cases/cases.jsonl`; committed tables in `agent/cases/results*.md` |
| Model path | `bash scripts/test_ollama.sh` | Ollama on the LAN | Both API shapes (native and `/v1`) answer, and an Arabizi alternative is not read as a match |
| Demo adapter smoke | open <http://localhost:5500/#client/home>, send one request, reload | static web server | The adapter (`web/wassit-demo.js`) persists a ticket in `localStorage` key `dosyn-demo-v2` with no backend. Manual; flow in `docs/DEMO.md` |

Run `--help` on the Python scripts for their flags. AGENTS.md rule: no benchmarks or long model
runs during the event; tests mock the model.

## Known state on 2026-11-15

`bash scripts/verify_all.sh` reported two failures on the working tree:

| Check | Failure | Likely cause |
| --- | --- | --- |
| `backend-queries` | "every internal/queries/sql/*.sql prepares against the schema (6 of 95 failing)" | the prepare step ran against a database that is behind the migrations (0006, 0007). Run `make migrate` on that database and repeat |
| `repo-pytest` | `tests/test_openapi_routes.py::test_code_has_63_routes`: 71 routes in code, 63 expected | the test constant predates the offers, speech, stock and reference routes |

Everything else passed (Go build, vet, tests; guard; agent pytest; static frontend sync).

## DB integration tests on a throwaway container

`backend/internal/queries/integration_test.go` connects to `TEST_DATABASE_URL` (a migrated
database) and skips when it is unset. Use a separate compose project and port so the dev and
demo databases (5432, 5573) are never touched:

```bash
cd backend
export COMPOSE_PROJECT_NAME=wassit_test DB_PORT=5574
docker compose up -d --wait db
export TEST_DATABASE_URL="postgres://alphahack:alphahack@localhost:5574/alphahack?sslmode=disable"
DATABASE_URL="$TEST_DATABASE_URL" ./scripts/migrate.sh up     # works without host psql
go test ./...
docker compose down -v      # removes only the wassit_test project and its volume
```

Keep `COMPOSE_PROJECT_NAME` exported for the last line; without it `down -v` would target the
default project and delete the dev database volume.

## Results on record

`docs/TEST_REPORT_2026-11-15.md` (full QA run), `logs/fix_report_2026-11-15.md`,
`logs/swarm_bug_hunt_2026-11-15.md`.
