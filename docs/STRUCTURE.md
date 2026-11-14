# Repository structure

How this repo is laid out, why, and where a new file goes.

## Tree (top two levels)

```
alphahack/
  README.md            start here: what the project is and how to run it
  AGENTS.md            stack facts and rules for every AI model on the project (CLAUDE.md imports it)
  CLAUDE.md            Claude Code entry: @AGENTS.md plus where the plans and prompts are
  CONTRACT.md          frozen Wassit spec: endpoints, product rules, event types, seed
  agent/               Python enquiry agent on Cornilia
    runner.py  scripted.py  usage.py   entry point, scripted fallback policy, token usage
    tools/             Cornilia tools (one file per area; _*.py are helpers, not loaded)
    prompts/           system prompt
    cases/             test cases and committed result tables
    tests/             pytest, no network, no model
  backend/             Go API + PostgreSQL (copied from Backend_Template)
    cmd/server/        entry point
    internal/          ai, config, handlers, http, queries, services
    db/                migrations and schema.dbml
    scripts/           migrate, smoke, check_queries
  cornilia-guard/      security layer: signed tool calls from the agent to the backend
    go/                Go gateway module (agentgw, cmd/demo)
    python/            cornilia_guard package, example tools, tests
  scripts/             repo-level drivers against a live backend: demo, acceptance, case runner, seed
  tests/               repo-level smoke tests (layout, script --help, doc links)
  docs/
    PLAN.md  WORKFLOW.md  DEMO.md      build order, subagent workflow, judge demo order
    STRUCTURE.md       this file
    lead-prompts/      paste-in lead prompts and agents/ (portable copies of .claude/agents)
    research/          background research notes
    archive/           superseded documents kept for history
  .claude/agents/      the Haiku subagents Claude Code loads (scout, builder, tester)
```

`Cornilia/` and `Backend_Template/` may sit next to these on a local machine. They are
separate git repos, ignored by `.gitignore`, and never edited from here.

## Decision record (2026-11-15)

**Context.** The code folders already follow their stack's conventions: `backend/` uses
Go's `cmd/` + `internal/` with the strict handlers -> services -> queries layering that
`scripts/check_queries.sh` enforces, `agent/` follows Cornilia's tools-folder layout, and
`cornilia-guard/` keeps a Go module and a Python package side by side. The mess was at the
root: eight markdown files, a `documents/` folder holding an outdated contract draft, a
`claude-agents/` folder whose purpose was only clear from one prompt, no README, and a file
name with spaces and parentheses.

**Options considered.**
1. Full restructure into `apps/` + `packages/` (monorepo style). Rejected: it moves every
   code path, breaks `agent/.venv` paths baked into scripts, and conflicts with open work.
2. Code moves inside `backend/` (for example splitting the 86 files in
   `internal/queries/sql/` by domain). Rejected for now: the one-file-per-statement rule and
   the query checker depend on that folder, and the open audit PR edits backend files.
3. **Chosen: tidy the root only.** Code paths stay as they are. Docs move into `docs/`, with
   subfolders by purpose. A README is added. `CONTRACT.md` stays at the root because backend
   code comments and the acceptance scripts cite it by that name.

**Cost.** Eleven file renames (all detected by git at 98 to 100 percent similarity) and
path edits in CLAUDE.md, docs/WORKFLOW.md, the two lead prompts, and one docstring line and
one error message in `scripts/demo_flow.py`. No shims were needed, because nothing imports
a doc.

**Rule for new files.**
- Agent code -> `agent/` (a tool -> `agent/tools/<area>.py`, a test -> `agent/tests/`).
- Backend code -> `backend/`, following `backend/AGENTS.md` (SQL -> one file in
  `internal/queries/sql/`, schema change -> new migration).
- A script you run against a live backend -> `scripts/`. A script for one component goes in
  that component's own `scripts/`.
- A doc people read -> `docs/` (a research note -> `docs/research/`, a superseded doc ->
  `docs/archive/`, a lead prompt -> `docs/lead-prompts/`). Only entry docs that tools look
  for by name (README, AGENTS, CLAUDE, CONTRACT) live at the root.
- Paths in docs are written from the repo root (`scripts/demo_flow.py`), as commands are run
  from the root.
