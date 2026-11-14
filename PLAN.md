# PLAN.md: from idea to demo

The start-up plan for AlphaHack once the theme is known. Read `AGENTS.md` for the stack facts
(ports, env vars, commands) and `WORKFLOW.md` for how to split the work across subagents. This
file only says **what to do, in what order**.

> T0 = the moment the idea is written into `AGENTS.md` §2. Times below are targets, not limits.
> Priority for every call: a working demo > clean code > extra features.

## Target layout (created in Step 2)

```
alphahack/
  AGENTS.md  PLAN.md  WORKFLOW.md  CONTRACT.md   # docs (CONTRACT.md is written in Step 1)
  Backend_Template/  Cornilia/                   # untouched references, never edited
  backend/          # copy of Backend_Template, the product's API + DB
  agent/            # the product's brain: configs.py, tools/*.py, prompts/system.md, .venv
  web/              # optional demo UI (only if the demo needs more than a terminal)
```

## Step 0: lock the idea (T0 to T0+30m, lead + owner)

1. Fill in `AGENTS.md` §2 with the owner: problem, one-sentence agent, demo flow (3 to 5 steps),
   tools, data, judging criteria, out of scope.
2. Turn the demo flow into **acceptance checks**: one line per step, each one testable with a
   curl or a single chat message. These are the definition of done.

## Step 1: write the contract (T0+30m to T0+1h, lead only)

Write `CONTRACT.md` (template below). This is the one document every subagent reads, so it
replaces re-reading the repos. Decide here, not later:

- **Loop owner** per feature: Cornilia-driven (default) or backend-driven (when the demo needs
  every step stored and streamed over SSE). See `AGENTS.md` §3.
- **Model**: local Ollama `qwen3.5:35b-a3b` for both, or Anthropic on the backend
  (`AI_PROVIDER=anthropic`) when quality matters more than running offline.
- **Ownership map**: which subagent owns which files. No file has two owners.

## Step 2: scaffold (T0+1h to T0+2h, see WORKFLOW W1)

`backend/` boots with `make docker-up` and answers `/healthz`. `agent/` runs `cornilia` with one
tool that calls the backend (`AGENTS.md` §6). Commit this as the green baseline.

## Step 3: freeze the shared files (T0+2h to T0+3h, see WORKFLOW W2)

One agent writes every shared file at once (migration, schema, models, Store interfaces, route
stubs, `openapi.yaml`) so the parallel builders never touch the same file. `make check` passes.

## Step 4: build in parallel (T0+3h to T0+20h, see WORKFLOW W3)

Fan out: backend slices, Cornilia tools, system prompt, optional UI. Each builder ships its slice
with tests and reports in five lines. The lead integrates after each wave.

## Step 5: end-to-end demo (T0+20h to T0+28h, see WORKFLOW W4 and W5)

Run every acceptance check against the real stack. Fix only what breaks the demo. Seed demo data
with a script, write `DEMO.md` (exact commands and messages to type), and record a fallback run.

## Step 6: polish and pitch (remaining time)

Only now: nicer UI, extra tools, README, slides. Freeze features 6 hours before the deadline;
after that, fixes only. Stop all model-heavy work (no benchmarks) during judging.

## CONTRACT.md template

```markdown
# CONTRACT (frozen at <time>; change only through the lead)
## Idea in one line
## Acceptance checks          # numbered, each a curl or chat message + expected result
## Loop owner + model         # Cornilia-driven | backend-driven, which model, thinking on/off where
## Data                       # new tables: name, columns, types, FKs
## Endpoints                  # METHOD /path -> request JSON -> response JSON, status codes
## Agent tools                # name, args (with Literal choices), what it returns (<2k chars), which endpoint it calls
## Ownership map              # file or folder -> owner role; shared files -> lead
## Out of scope
```

## Record keeping

After each step, add one line to the bottom of this file (`- <time> Step N done: <result>`) and
log the milestone in the owner's Notion Command Center.

## Progress log

- 2026-11-13 Plan and workflows written; waiting for the theme.
- 2026-11-14 Wassit Phase 0 done: backend/ copied from Backend_Template, recon complete, CONTRACT.md drafted; waiting for owner OK before W2.
