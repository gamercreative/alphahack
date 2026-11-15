# LEAD_PROMPT_wrapup

Pulls together what the recent swarm jobs did, lists what's left, merges every PR in alphahack,
Backend_Template and Cornilia, and runs the full test suite across all three.

## How to run it
1. Subagents are already in `.claude/agents/` (scout, builder, tester, all Haiku).
2. `gh auth status` must show you're logged in as gamercreative.
3. From `alphahack/`, launch `claude --model claude-fable-5-2` and paste everything inside the fence.
4. It stops once: after the summary and merge plan. Reply "go" (or with changes) to start merging.

```text
You are the advisor and lead engineer for this job. You run on Fable 5.2. You read, summarize,
decide the merge order, resolve conflicts, and verify. Mechanical work is done by cheap subagents:
- scout (Haiku): read-only searching
- builder (Haiku): one small change per call
- tester (Haiku): tests only

Follow WORKFLOW.md (spec and report format). Stack facts are in AGENTS.md. CONTRACT.md is the
frozen Wassit contract. DEMO.md is the demo flow and must work at the end.
Your output file is documents/STATUS_2026-11-15.md.

## Goal
1. Summarize what the recent jobs did (code-quality audit, structure reorg, frontend handoff,
   DOSYN frontend gap check, and anything else since 2026-11-13): what was asked, what was
   delivered, what was deferred, and what is waiting on me.
2. Turn that into one ranked to-do list.
3. Merge every open PR in gamercreative/alphahack, gamercreative/Backend_Template and
   gamercreative/Cornilia, in an order that never leaves a default branch broken.
4. Run the full test suite for all three repos plus the DEMO.md flow, and prove nothing that passed
   before now fails.
5. Finish with an updated to-do list that reflects the merged state.

## Sources for the summary (read the end of each first; that's where the results are)
- alphahack: PLAN.md progress log, CONTRACT.md (Decisions section), DEMO.md, every LEAD_PROMPT_*.md
  (what each job was asked to do), everything in documents/ and docs/ (gap report, contract
  proposal, handoff report, DOSYN_PROTOTYPE.md), agent/cases/results*.md.
- All three repos: `gh pr list --state all --search "created:>=2026-11-13"`, and each PR's body
  (deferred items and follow-ups live there); `git log --since=2026-11-13 --oneline --all`;
  local branches not pushed (`git branch -vv`); uncommitted work (`git status` in each clone).
- Claude Code session logs for this folder, if present: the newest 8 .jsonl files under
  ~/.claude/projects/ whose folder name matches this repo's path. Read only each session's last
  assistant messages (the final reports). Never copy a token, key or .env value out of them.
For each job, compare what its LEAD_PROMPT asked for with what was actually delivered. A gap
there is a to-do item.

## Non-negotiable rules (stop on any that would break)
1. Before the first merge in each repo, tag its default branch `pre-merge-2026-11-15` and push the
   tag. This is the rollback point.
2. Merge with `gh pr merge --merge` (merge commit). No squash, no rebase-merge, so each commit
   stays revertable on its own.
3. Never push straight to a default branch. Never force-push one. Fixes go on `fix/post-merge-<repo>-<n>`
   as a PR and merge once green. Force-with-lease is allowed only on PR branches you just rebased.
4. Never skip, delete or loosen a test to get green, and never weaken validation, auth, scopes,
   budgets or SQL safety. If a fix needs that, stop and ask me.
5. CONTRACT.md product rules 1-8 hold after every merge. A PR that breaks one is held, not merged.
6. Don't move Cornilia's tag v0.1.0 or change alphahack's pin. Cornilia changes worth taking and
   Backend_Template fixes worth porting into alphahack/backend go on the to-do list, not into code.
7. Merge PRs authored by gamercreative or opened by the swarm jobs (quality/*, structure/*, docs/*,
   fix/*). Anything else: list it and ask me.
8. No secrets: a PR that adds a .env file, key or token is held; tell me the path only.
   agent/.env and backend/.env stay untracked.
9. Rena's frontend (web/ or her repo) is never edited.
10. Local verification only: Docker stack, seeded demo data, AI_PROVIDER=fake or the scripted
    agent. At most one real Wassit run (Claude Haiku 5.5), at the very end, only with my yes.

## Full test, per repo (confirm the exact commands from each repo's README, Makefile or CI)
- Cornilia: pytest, plus lint/type check if configured.
- Backend_Template: make check, make check-queries, make smoke against make docker-up.
- alphahack: backend make check, make check-queries, make smoke; pytest in agent/ (agent/.venv);
  then the whole stack as DEMO.md says: bash scripts/seed_demo.sh, python scripts/acceptance.py,
  python scripts/run_cases.py with the scripted agent, and scripts/test_frontend_support.py if it
  exists (its xfails must stay xfail; a new real failure is a break). Then walk through DEMO.md by hand.
Run all of this on each default branch BEFORE any merge and record it as the baseline.

## How you work
Phase 0: Summarize and plan (scout + you; no merges, no edits except the STATUS file).
1. Check each clone (alphahack/, alphahack/Backend_Template/, alphahack/Cornilia/) is on its default
   branch. If one has uncommitted work, don't stash or reset: list it in the summary.
2. Send scouts (up to 8 in parallel) for narrow lookups across the sources above, for example
   "list every deferred item in PR bodies with its PR number" or "which documents/ files are not
   committed". Read the important ones yourself.
3. Record the baseline full test.
4. Review every open PR: description, commits, `gh pr diff`, `gh pr checks`. Verdict per PR:
   merge / merge after fix (state the fix) / hold (state the reason).
5. Uncommitted outputs from earlier jobs (reports, probe tests, PLAN.md lines): propose one PR
   `docs/wrapup-2026-11-15` that collects them, so they merge like everything else.
6. Write documents/STATUS_2026-11-15.md, sections 1 to 4:
   1. What happened: one short paragraph per job: asked, delivered, deferred, links (PRs, files).
   2. Decisions waiting on me: each with the options and what it blocks (for example CONTRACT.md
      rule 8 vs the DOSYN client side).
   3. To-do, ranked: demo blockers first, then pilot-milestone gaps, then cleanup and follow-ups.
      Each item: what, why, size (S/M/L), source.
   4. Merge plan: every PR with its verdict and reason, the exact order, expected file overlaps,
      and the baseline test results (including anything already failing).
Then send me a 10 to 15 line version of sections 2 to 4. WAIT for my OK.

Phase 1: Merge.
Order across repos: Cornilia, then Backend_Template, then alphahack (the first two don't affect
alphahack's build). Inside a repo: structure/* first, then quality/*, then fix/*, then docs/*.
For each PR:
1. Rebase onto the latest default branch. Resolve conflicts yourself: for reorg vs quality, the
   reorg's file locations win and the quality fix's content wins.
2. Run that repo's full test. Merge only if it's green against the baseline. Then pull.
3. On a failure: diagnose it yourself. For a mechanical fix (import path, renamed symbol, fixture
   path), send builder one spec (file, exact change, check command); for a missing test, send tester
   a spec. Read every diff. After 2 failed attempts on the same spec, write the fix yourself.
   A fix that isn't mechanical or touches a rule: stop and ask me.
4. If two PRs make decisions you can't reconcile, merge the first and hold the second for me.
After each repo, send me 3 to 5 lines: what merged, test status, anything held.

Phase 2: Full test and fresh review.
1. Run the full test for all three repos on the merged default branches and compare with the
   baseline. Nothing that passed may fail.
2. Walk through DEMO.md on the local stack, including undo inside the window, a 403
   agent_forbidden, a 409 on a bad close, and stats showing hitl_count and cost.
3. Spawn one fresh reviewer (subagent_type general-purpose, model sonnet). Give it only
   `git diff pre-merge-2026-11-15..HEAD` per repo, CONTRACT.md and the test results. Ask it for
   broken behaviour, weakened checks and rule breaks. Fix (through a fix/ PR) or answer every point.
4. Ask me whether to do the one real Wassit run. If yes, run it and report the cost.

Phase 3: Close out.
Finish documents/STATUS_2026-11-15.md with:
5. Merge results: a table per repo (PR | verdict | merge commit or reason held).
6. Test results: baseline vs final, per command.
7. Updated to-do: the Phase 0 list with merged items ticked, new items from the merges added.
8. Rollback: `git revert -m 1 <merge commit>` per PR, or reset to tag pre-merge-2026-11-15 (only with my OK).
Add one line to PLAN.md's progress log. Commit the STATUS file and that PLAN.md line on
docs/status-2026-11-15 and open a PR. Merge it only with my OK.

## Cost rules
- Use scout for any search that needs more than two reads. Read line ranges and file tails, not whole files.
- Session logs are large: read only their last assistant messages.
- Don't re-read files you already reviewed unless they changed.
- After a small fix, run the changed package; run the full test before each merge.
- No benchmarks. Fake provider and scripted agent until Phase 2 step 4.

## Reporting and stop points
Stop and ask me before: the first merge (the Phase 0 OK), merging anything held, any action on a
default branch other than a PR merge, deleting branches or tags, adding a dependency, the paid
Wassit run, and merging the final STATUS PR.
Final message to me: the STATUS file path, 5 lines on where things stand, and the top 3 to-do items.
```
