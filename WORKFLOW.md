# WORKFLOW.md: building with subagents

Reusable workflows for any model leading the AlphaHack build. Goal: fast and cheap. The lead
plans, briefs and integrates; subagents read code and write code. Stack facts live in
`AGENTS.md`, the build order in `PLAN.md`, the frozen spec in `CONTRACT.md`.

## The five rules that save the most tokens

1. **The lead never reads big files.** It reads `CONTRACT.md`, short reports and the diffs it
   reviews. Anything that needs a repo search goes to `scout`.
2. **One owner per file.** Parallel agents never edit the same file. Shared files are frozen
   first (W2), then only the lead touches them.
3. **Briefs carry the context.** A subagent starts cold, so the brief names exact files, the
   contract section, and the done check. It does not say "explore the repo".
4. **Reports are short.** Every subagent returns the report format below, at most ~15 lines.
   No pasted code, no file dumps.
5. **Resume, don't respawn.** To fix a builder's slice, send a message to that same agent (it
   keeps its context). A new agent re-reads everything.

Skip subagents entirely for a change of 1 to 3 files the lead already understands.

## Roles

Claude Code setup: the lead runs on **Fable 5.2** (`claude --model claude-fable-5-2`) and plans,
writes specs, reviews diffs and runs the tests. Code is written by three **Haiku** subagents defined
in `.claude/agents/`:

| Agent | File | Does | Never |
| --- | --- | --- | --- |
| Lead | (you) | specs, CONTRACT.md, shared files, diff review, final test runs | bulk reading, repo searches |
| `scout` | `.claude/agents/scout.md` | read-only search; returns path + line range + 1-3 line excerpt | edits, opinions |
| `builder` | `.claude/agents/builder.md` | ONE file or ONE function from an exact spec, runs its check | touches unnamed files, weakens a check |
| `tester` | `.claude/agents/tester.md` | tests only, one per case (attack cases named after the attack) | changes non-test code, fixes bugs it finds |

Haiku is fast but literal: a builder spec must name the exact file, signatures, types, SQL text,
error messages and the check command. Paste reference snippets instead of describing them.
The "backend slice", "tool builder", "prompt writer" and "verifier" jobs below are all done by
`builder` or `tester` calls, one file each.

## Builder spec template (copy, fill, send)

```
AGENT: builder | tester

READ ONLY: <exact files>, CONTRACT.md §<section>. Follow <repo>/AGENTS.md rules.
OWN (only edit these): <exact files/folders>
TASK: <one feature, 1-3 sentences>
DONE WHEN: <command(s) that must pass, e.g. `cd backend && make check` or `pytest agent/tests/test_x.py`>
DON'T: touch files you don't own, add dependencies, run benchmarks or long model runs, edit Backend_Template/ or Cornilia/.
IF BLOCKED: stop and report what is missing; don't work around the contract.
REPORT: use the report format, max 15 lines.
```

## Report format (every subagent)

```
STATUS: done | blocked | partial
CHANGED: <files, one line each>
CHECK: <command run> -> <pass/fail, last 20 lines>
CONTRACT GAPS: <anything the contract didn't cover, or "none">
NEXT: <one line, optional>
```

## W1: Scaffold (2 builders in parallel, once)

- **A, backend:** `cp -R Backend_Template backend`, `cp -n .env.example .env`, set
  `OLLAMA_MODEL`/`AI_PROVIDER` per CONTRACT, `make docker-up`, curl `/healthz`. Done: healthz OK
  and `make check` passes.
- **B, agent:** create `agent/` with a Python 3.12 venv, `pip install "cornilia[server] @ git+https://github.com/gamercreative/Cornilia.git@v0.1.0"`,
  `configs.py` from `Cornilia/configs.example.py` (model `qwen3.5:35b-a3b`), `tools/backend.py`
  from AGENTS.md §6, `prompts/system.md` placeholder. Done: `list_agents` works from `cornilia`
  with `CORNILIA_TRACE=1`.
- **Lead:** git init/commit the green baseline.

## W2: Freeze the shared files (1 builder, sequential)

The lead writes (or sends one-file `builder` specs, in sequence, for) every file that more than one slice would touch, from CONTRACT:
`db/migrations/0002_<name>.up/.down.sql`, `db/schema.dbml`, `internal/queries/models.go`,
`internal/queries/querier.go` (method signatures), Store interfaces and constructor list in
`internal/services/contracts.go`, `Deps` + `Register` in `internal/handlers/handlers.go`, wiring in
`cmd/server/main.go`, and `openapi.yaml`. Handlers may be stubs. Done: `make check` and
`make check-queries` pass. From here on these files belong to the lead.

This mirrors the template's own pattern: `contracts.go` is "owned by the orchestrator" and
workers implement the constructors listed there.

## W3: Feature fan-out (parallel, the main build)

Spawn in one message, all in the background, one agent per row:

| Agent | Owns |
| --- | --- |
| Backend slice `<domain>` (one per domain) | `internal/queries/sql/*_<domain>*.sql`, `internal/queries/<domain>.go`, `internal/services/<domain>.go` + test, `internal/handlers/<domain>.go` + test, its lines in `scripts/smoke.sh` |
| Backend Go tools (only if backend-driven) | `internal/services/tools.go` + test |
| Tool builder `<area>` (one per file) | `agent/tools/<area>.py`, `agent/tests/test_<area>.py` |
| Prompt writer | `agent/prompts/system.md` |
| UI builder (optional) | `web/` |

Each row is a group of one-file builder specs (SQL file, then queries, then service, then handler),
plus one tester spec per group. Specs with no shared file run in parallel.

Rules for this wave:
- Tool builders code against the CONTRACT endpoints, not the running backend; they mock HTTP in
  tests so they don't wait on the backend slices.
- Tools follow Cornilia's small-model rules: the tool computes, results under ~2k chars,
  `Literal` filters, errors that teach (AGENTS.md §4).
- Backend tests use the in-memory Store fakes and `AI_PROVIDER=fake`; no real model in tests.
- While they run, the lead does nothing token-heavy. It waits for the completion notices.

After the wave the lead reads the reports and every diff (`git diff -- <owned files>`). Wrong diff:
send a corrected, more specific spec. Two failed attempts on the same spec: the lead writes it. Contract gaps are decided by the lead, written
into CONTRACT.md, and sent back to the owner by message.

## W4: Integrate and verify (the lead, repeat after each wave)

The lead runs, in order, `cd backend && make check && make check-queries && make docker-up && make smoke`,
then `cd agent && pytest`, then each acceptance check from CONTRACT.md. Note pass/fail per
command and the first error line of each failure.

The lead routes each failure to the agent that owns the file (resume it with a message). Two
failures on the same thing: write it down in PLAN.md's log and choose a simpler path.

## W5: Demo hardening (2 builders in parallel)

- **Seed + script:** `scripts/seed_demo.sh` (curls that create demo data) and `DEMO.md` with
  the exact commands and chat messages, in demo order.
- **Fallback:** a recorded run (asciinema or screen recording) and an `AI_PROVIDER=fake` path so
  the demo survives a slow or broken model.

## W6: Small change or bug fix (no fan-out)

1. If the cause is unknown, one Scout finds it ("where does X happen? file:line, 5 lines max").
2. The lead or the owning builder fixes it. 3. One verifier run on the affected checks.

## Cost rules

- Use `scout` for any search that needs more than two reads. Read line ranges, not whole files.
- Don't re-read files you already reviewed unless they changed.
- Run only the changed package after a small change (`go test ./internal/services/...`,
  `pytest -q agent/tests/test_x.py`); run the full suite at the end of each slice.
- Report to the owner in 3-5 lines per slice. Commit after each green slice; ask before pushing.

## Ready-made lead prompts

- `LEAD_PROMPT_cornilia_guard.md`: secure DB access for agent tools (Cornilia -> signed request ->
  Go agent gateway -> Postgres). Needs `cornilia-guard/` in the root and the owner's OK after Phase 0.

## Lead checklist per wave

- [ ] CONTRACT.md is current and every brief cites its section
- [ ] Ownership has no overlaps
- [ ] All parallel agents spawned in one message, in the background
- [ ] Reports read; gaps decided; fixes sent to the original owners
- [ ] Verifier green, then commit, then one line in PLAN.md's progress log
