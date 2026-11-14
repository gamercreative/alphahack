# AGENTS.md — AlphaHack hackathon project

Context for every AI model and chat (Claude, Codex, Cursor, Gemini, local models) working on
this project. Read all of it before writing code. The same text works as `CLAUDE.md`: either
copy it, or make `CLAUDE.md` a one-liner that says `@AGENTS.md`.

> Last verified against both repos on GitHub: 2026-11-13 (Cornilia tag `v0.1.0`,
> Backend_Template `main`). If something here disagrees with the code, the code wins:
> say so in your reply and suggest a fix to this file.

---

## 1. The event and the goal

- **Event:** AlphaHack, a 48 hour hackathon that started **2026-11-13**.
- **What we are building:** an AI agent with a fully operational backend and database.
- **Owner:** akram (GitHub `gamercreative`). Local workspace on the Mac:
  `/Users/akram/Documents/code/agentic_harness` (also holds `hermes-manager` and `scripts`).
- **Priority order for every decision:** a working demo > clean code > extra features.
  Ship the smallest thing that works end to end, then improve it.

## 2. The idea (fill this in)

> **TODO (owner):** replace this block with the hackathon idea.
>
> - Problem / who it is for:
> - What the agent does in one sentence:
> - The demo flow (3 to 5 steps the judges will see):
> - Tools the agent needs (each one = one Cornilia tool or one backend endpoint):
> - Data we store (new tables, if any):
> - Judging criteria / track:
> - Out of scope:

Until this section is filled in, ask the owner before inventing product features.

---

## 3. Architecture

Two of the owner's repos, used together over **HTTP only** (no shared code, no shared DB access):

| Part | Repo | Language | Role | Port |
| --- | --- | --- | --- | --- |
| Brain | `gamercreative/Cornilia` | Python ≥ 3.10 | Agent loop, tools, model provider | 8000 (optional HTTP server) |
| Storage + API | `gamercreative/Backend_Template` | Go 1.25 | REST API, PostgreSQL, run and tool-call history | 8080 |
| Database | Postgres 16 (Docker) | SQL | Data | 5432 |
| Model | Ollama on the host | — | `qwen3.5:35b-a3b` | 11434 |

**Recommended split:** Cornilia is the brain, the backend is storage and API. Cornilia tools
call the backend's REST endpoints with `httpx`/`requests`; the backend never imports Cornilia.

Note that the backend also has its **own** agent loop (`POST /v1/conversations/{id}/messages`
runs a model with Go tools and stores every run and tool call). So there are two valid ways
to run the agent, pick one per feature and say which:

1. **Cornilia-driven (default):** Cornilia runs the loop; its tools read and write the backend.
2. **Backend-driven:** the backend runs the loop with Go tools in
   `internal/services/tools.go`; useful when you want every step persisted and streamed (SSE).

### Where hackathon code lives

- Cornilia's own rules forbid event-specific code in its core (`src/cornilia/`). **Never put
  hackathon tools, prompts or data in Cornilia's `src/`.** Put them in this project's folder
  (for example `tools/` next to a `configs.py`) and install Cornilia as a pinned package.
- Backend_Template is a starter: copy or fork it for the project and extend it following its
  own `AGENTS.md` (layering and SQL rules below).

---

## 4. Cornilia (the brain)

**Install** (the Mac's system `python3` is 3.9.6, which is too old; use 3.12):

```bash
python3.12 -m venv .venv && source .venv/bin/activate
pip install "cornilia[server] @ git+https://github.com/gamercreative/Cornilia.git@v0.1.0"
# or, inside a Cornilia clone: pip install -e '.[dev,server]'   (18/18 tests pass with pytest)
```

Extras that exist: `[server]` (fastapi, uvicorn, pydantic) and `[dev]` (pytest). There is
**no** `[web]` extra and no `examples/web_tools` on `main`. Only runtime dependency: `openai>=1.58`.

**Run:**

```bash
cornilia                                  # terminal chat (same as python -m cornilia)
uvicorn cornilia.server:app --port 8000   # HTTP API, run from the folder with configs.py and tools
```

HTTP API (`src/cornilia/server.py`, conversations kept in memory, lost on restart):
`POST /one-shot {message, model_name?, system_prompt?}`,
`POST /multi-shot {message, conversation_hash?, model_name?}`,
`GET /multi-shot/{conversation_hash}`.

**Configuration** (`src/cornilia/settings.py`). Environment variables, or a gitignored
`configs.py` in the folder you run from (copy `configs.example.py`); `configs.py` wins.

| Variable | Default | Notes |
| --- | --- | --- |
| `CORNILIA_PROVIDER` | `ollama` | `ollama` or `openai` presets |
| `CORNILIA_BASE_URL` | `http://localhost:11434/v1` | **ends in `/v1`** (OpenAI-compatible) |
| `CORNILIA_API_KEY` | `ollama` | `OPENAI_API_KEY` for the openai preset |
| `CORNILIA_MODEL` | `qwen3.5:9b` | **set to `qwen3.5:35b-a3b`** to match the backend |
| `CORNILIA_MAX_TOKENS` | 512 (ollama preset) | raise for long answers |
| `CORNILIA_TIMEOUT` | 120 s | use 300 when thinking is on |
| `CORNILIA_TOOLS_DIR` | none | folders joined with `:`; every `Tool` at top level of a `.py` is loaded |
| `CORNILIA_SYSTEM_PROMPT` / `_FILE` | none | supports `$date $time $datetime $weekday` |
| `CORNILIA_MAX_STEPS` | 10 | tool-loop limit per turn |
| `CORNILIA_TRACE` | off | `1` prints every tool call |
| `CORNILIA_PLANNER_MODEL` | none | setting it turns on the planner/executor team |

Gemini works through `CORNILIA_PROVIDER=openai` with
`CORNILIA_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai/`.

**Write a tool** (one file per area in `tools/`; files starting with `_` are helpers, not loaded):

```python
from typing import Annotated, Literal
from cornilia.tools import tool

@tool
def count_words(text: Annotated[str, "The text to count the words of"]) -> dict:
    """Counts the words and characters in a piece of text."""
    return {"words": len(text.split()), "characters": len(text)}
```

Name, docstring and type hints become the schema. `Literal[...]` becomes an enum; a
`TypedDict` describes a nested object. For full control, subclass `Tool`
(see `examples/tools/tax_calculator.py`).

**Rules for tools (from Cornilia's benchmark on small models):**

- Let the model choose and the tools compute: arithmetic, comparing, ranking, filtering,
  sorting and validation happen in code, and the tool returns the finished answer.
- Keep results small (about 2,000 characters). Big data goes to a file or comes back in pages.
- Few parameters, sensible defaults, explicit filters such as `state: Literal["open","closed","all"]`.
- Errors teach: say what was wrong and what to try next (the text goes back to the model).
- Thinking stays off by default. Turn it on only for one role that needs exact logic.
- Keep the conversation append-only (the local prompt cache depends on it).

---

## 5. Backend_Template (storage + API)

Go 1.25, stdlib router, pgx, PostgreSQL 16, hand-written SQL. No ORM, no framework, **no auth**.

**Run:**

```bash
cp .env.example .env          # DATABASE_URL is required; the server exits without Postgres
make docker-up                # db + migrations + app in Docker, waits until healthy
# or on the host: make up && make migrate && make run
curl localhost:8080/healthz
```

Other targets: `make test`, `make check` (build + vet + test), `make check-queries`,
`make smoke` (curls every endpoint), `make docker-logs`, `make down`.

**Endpoints** (contract in `openapi.yaml`, JSON is snake_case):

| Method + path | Body / notes |
| --- | --- |
| `GET /healthz` | |
| `POST/GET /v1/agents`, `GET/PATCH/DELETE /v1/agents/{id}` | create: `{name, system_prompt?, model?}` |
| `POST/GET /v1/conversations`, `GET/DELETE /v1/conversations/{id}` | create: `{agent_id, title?}` |
| `GET /v1/conversations/{id}/messages` | history |
| `POST /v1/conversations/{id}/messages` | `{content}` runs the backend's agent loop |
| `POST /v1/conversations/{id}/messages/stream` | same turn as server-sent events |
| `GET /v1/conversations/{id}/runs`, `GET /v1/runs/{id}` | run + tool-call trace |
| `GET /v1/tools` | the backend's Go tools |

Lists return `{"data": [...], "limit": n, "offset": n}`. Create → 201, delete → 204.
Every error is `{"error": {"code", "message", "request_id"}}`.

**Data model** (`db/schema.dbml`): `agents` → `conversations` → `runs` → `tool_calls`, and
`messages` (role `user|assistant`) linked to a conversation and a run. Text UUID ids.

**Key env vars** (`.env.example`): `PORT=8080`,
`DATABASE_URL=postgres://alphahack:alphahack@localhost:5432/alphahack?sslmode=disable`,
`AI_PROVIDER` (`anthropic`, `ollama` or `fake`; default anthropic if `ANTHROPIC_API_KEY` is
set, else fake), `ANTHROPIC_MODEL`, `OLLAMA_BASE_URL=http://localhost:11434` (**no `/v1`**,
it uses Ollama's native `/api/chat`), `OLLAMA_MODEL=qwen3.5:35b-a3b`, `OLLAMA_NUM_CTX`
(use 16384+ with tools), `REQUEST_TIMEOUT=300s`, `CORS_ALLOWED_ORIGINS=*`.
Under `make docker-up`, the container ignores `.env` for `DATABASE_URL`, `PORT` and
`OLLAMA_BASE_URL` and reaches Ollama at `host.docker.internal:11434`.

**Rules (strict, enforced by `scripts/check_queries.sh`):**

- Layering: `handlers → services → queries → PostgreSQL`, and `services → ai`.
  Only `internal/queries` touches the DB; only `internal/ai` talks to model providers.
- One SQL statement per file in `internal/queries/sql/`, positional params, no `SELECT *`,
  never build SQL with string concatenation.
- Schema change = new migration in `db/migrations/` + `db/schema.dbml` + `models.go`.
- New endpoint: SQL file → `Querier` method → service → handler + route → `openapi.yaml`
  → `scripts/smoke.sh`.
- New backend tool: add an `ai.Tool` to `services.DefaultTools()` in `internal/services/tools.go`.
- Before calling a change done: `go build ./... && go vet ./... && go test ./...`.

---

## 6. Connecting them

Pattern (proved in an audit on 2026-11-13: a Cornilia tool created and listed an agent through
the backend). The proof file was on an **unpushed** local branch, so recreate it rather than
assuming it exists:

```python
# tools/backend.py
import json, os, urllib.request
from typing import Annotated
from cornilia.tools import tool

BACKEND_URL = os.environ.get("BACKEND_URL", "http://localhost:8080")

def _call(method: str, path: str, body: dict | None = None) -> dict:
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BACKEND_URL + path, data=data, method=method,
                                 headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            return json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        return json.loads(e.read() or b"{}")  # the backend's error body teaches the model

@tool
def list_agents(limit: Annotated[int, "How many to return"] = 20) -> dict:
    """Lists the agents stored in the backend."""
    out = _call("GET", f"/v1/agents?limit={limit}")
    return {"agents": [{"id": a["id"], "name": a["name"]} for a in out.get("data", [])]}
```

Gotchas when wiring them:

- Base URLs differ on purpose: Cornilia wants `.../v1`, the backend's `OLLAMA_BASE_URL` does not.
- Model names: set `CORNILIA_MODEL=qwen3.5:35b-a3b` so both use the same model.
- Ports: Cornilia 8000, backend 8080, Postgres 5432, Ollama 11434. No clashes.
- No auth on the backend: fine for a local demo, never expose 8080 publicly as is.
- Cornilia's HTTP server keeps conversations in memory; persist anything important through
  the backend.

## 7. Hackathon run sheet

```bash
# terminal 1: model
ollama serve            # and once: ollama pull qwen3.5:35b-a3b

# terminal 2: backend + database
cd Backend_Template && cp -n .env.example .env && make docker-up
curl localhost:8080/healthz

# terminal 3: the agent
cd <project folder> && source .venv/bin/activate
export BACKEND_URL=http://localhost:8080 CORNILIA_MODEL=qwen3.5:35b-a3b \
       CORNILIA_TOOLS_DIR=tools CORNILIA_TRACE=1
cornilia                       # or: uvicorn cornilia.server:app --port 8000
```

---

## 8. Working rules for any AI on this project

- **Read before writing.** Each repo has its own `AGENTS.md`; follow it inside that repo.
  Never edit `AGENTS.md`, `CLAUDE.md` or `.github/CODEOWNERS` inside the Cornilia repo.
- **Small, demo-first steps.** One concern per change, and keep "fresh clone, install, run" working.
- **No secrets in git.** Keys live in `.env` (backend) or `configs.py` / env vars (Cornilia).
- **Ask before** pushing to `main`, upgrading dependencies, deleting data, or anything that
  touches production.
- **No benchmarks or long model runs during the event.** Mock the LLM in tests
  (the backend has `AI_PROVIDER=fake`).
- **Errors are data.** When something fails twice, write down what and where, and move on.
- **Keep this file current.** When the idea, a port, an env var or a command changes,
  update the matching section here in the same change.
