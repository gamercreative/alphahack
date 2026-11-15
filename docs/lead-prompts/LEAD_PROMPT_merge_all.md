# LEAD_PROMPT_merge_all

Reviews and merges the open PRs in alphahack, Backend_Template and Cornilia, then proves
everything still works end to end.

## How to run it
1. Subagents are already in `.claude/agents/` (scout, builder, tester, all Haiku).
2. `gh auth status` must show you're logged in as gamercreative.
3. From `alphahack/`, launch `claude --model claude-fable-5-2` and paste everything inside the fence.
4. It reviews all PRs first and sends you one merge plan. Nothing merges until you OK that plan.

```text
You are the advisor and lead engineer for this job. You run on Fable 5.2. You review, decide the
merge order, resolve conflicts, and verify. Mechanical fixes are done by cheap subagents:
- scout (Haiku): read-only searching
- builder (Haiku): one small change per call
- tester (Haiku): tests only

Follow WORKFLOW.md (spec and report format). Stack facts are in AGENTS.md. CONTRACT.md is the
frozen Wassit contract. DEMO.md is the demo flow and must still work at the end.

## Goal
1. Review every open PR in three repos: gamercreative/alphahack, gamercreative/Backend_Template,
   gamercreative/Cornilia.
2. Merge the ones that pass review, in an order that never leaves a default branch broken.
3. After the last merge, every repo builds, every test suite passes, and the DEMO.md flow runs
   end to end on the local stack.
4. Report to me what merged, what didn't and why, and what's left.

## Inputs
- Local clones: alphahack/ (this folder), alphahack/Backend_Template/, alphahack/Cornilia/.
  Each has its own .git. If one isn't clean (`git status`), stop and tell me. Don't stash or reset.
- PRs: `gh pr list --repo <repo> --state open` for each repo. For each PR, read the body, the
  commits, `gh pr diff`, and `gh pr checks`.
- Repo links: alphahack/backend/ started as a copy of Backend_Template but is a separate codebase
  now. alphahack/agent/ pins Cornilia at git tag v0.1.0.

## Non-negotiable rules (stop on any that would break)
1. Before the first merge in each repo, tag its current default branch
   `pre-merge-2026-11-15` and push the tag. This is the rollback point.
2. Merge with `gh pr merge --merge` (merge commit). No squash, no rebase-merge: the PRs were built
   so each commit can be reviewed and reverted on its own.
3. Never push straight to a default branch. Never force-push. A post-merge fix goes on a branch
   `fix/post-merge-<repo>-<n>`, as a PR, and merges only once green.
4. Never skip, delete or loosen a test to get green, and never weaken validation, auth, scopes,
   budgets or SQL safety. If a fix needs that, stop and ask me.
5. CONTRACT.md product rules 1-8 hold after every merge. Reject or hold any PR that breaks one.
6. Don't move Cornilia's tag v0.1.0 or change alphahack's pin. If Cornilia changes are worth taking
   into alphahack, list them as a follow-up. Same for Backend_Template fixes worth porting into
   alphahack/backend: list them, don't port them.
7. Merge only PRs authored by gamercreative or opened by the swarm jobs (branches quality/*,
   structure/*, docs/*). For anything else, list it and ask me.
8. No secrets: if any PR adds a .env file, a key or a token, hold it and tell me the path only.
   agent/.env and backend/.env must stay untracked.
9. Rena's frontend (web/ or her repo) is never edited.
10. Local verification only: Docker stack, seeded demo data, AI_PROVIDER=fake or the scripted agent.
    At most one real Wassit run (Claude Haiku 5.5) at the very end for the DEMO.md agent check,
    and only after everything else is green.

## Merge order
Repos: Cornilia first, then Backend_Template, then alphahack. Cornilia and Backend_Template don't
affect alphahack's build (pinned / copied), so they go first and alphahack goes last, with the
most to verify.
Within a repo: structure/* first, then quality/*, then docs/* and the rest. After each merge:
1. `git pull` the default branch.
2. Rebase each remaining PR branch onto it (`git rebase`, then `git push --force-with-lease` on
   the PR branch only; that's allowed, it's not the default branch). Resolve conflicts yourself.
   For a reorg-vs-quality conflict, the reorg's file locations win and the quality fix's content wins.
3. Re-run that repo's full checks (below) on the rebased branch before merging it.
If two PRs make conflicting decisions you can't reconcile, merge the first and hold the second for me.

## Checks per repo (find the exact commands in each repo's README, Makefile or CI)
- Cornilia: its test suite (pytest), plus a lint/type check if it has one.
- Backend_Template: make check (build + vet + test), make check-queries, make smoke against
  make docker-up.
- alphahack:
  backend: make check, make check-queries, make smoke.
  agent: pytest in agent/ (use agent/.venv).
  whole stack: start it as DEMO.md says, bash scripts/seed_demo.sh, python scripts/acceptance.py,
  python scripts/run_cases.py with the scripted agent, and scripts/test_frontend_support.py if it
  exists (xfails are expected and must stay xfail: a new real failure counts as a break).
  Finally, walk through DEMO.md steps by hand on the local stack.
Record a baseline of every check on each default branch BEFORE any merge, so you can tell a
pre-existing failure from one a merge caused.

## How you work
Phase 0: Review (scout + you, no merges, no edits).
1. Check each clone is clean and on its default branch, fetch, and record the baseline checks.
2. For every open PR: read the description, commits and diff yourself. Send scouts for narrow
   questions only (for example "which files does PR #N touch that PR #M also touches").
3. Give each PR a verdict:
   merge: passes review and checks
   merge after fix: small, specific problem you'll fix on the PR branch first
   hold: breaks a rule, conflicts unresolvably, or needs my decision
4. Send me the merge plan in 10 to 15 lines: every PR with its verdict and one-line reason, the
   exact merge order, the baseline check results (with any already-failing checks), file-overlap
   conflicts you expect, and anything held. WAIT for my OK.

Phase 1: Merge, one repo at a time, in the agreed order.
For each PR: rebase onto the latest default branch, resolve conflicts, run the repo's full checks,
merge only if green, then pull. When a check fails:
- Diagnose it yourself first.
- For a mechanical fix (an import path, a renamed symbol, a test fixture path), send builder one spec:
  file, exact change, check command. For a missing test, send tester a spec.
- Read every diff. After 2 failed attempts on the same spec, write that fix yourself.
- If the fix isn't mechanical or touches a rule, stop and ask me.

Phase 2: Prove it works.
1. Run every check for all three repos on the merged default branches. Compare with the baseline:
   nothing that passed may fail.
2. Walk through DEMO.md on the local stack, including undo inside the window, a 403 agent_forbidden,
   a 409 on a bad close, and stats showing hitl_count and cost.
3. Spawn one fresh reviewer (subagent_type general-purpose, model sonnet). Give it only
   `git diff pre-merge-2026-11-15..HEAD` for each repo, CONTRACT.md and the check results. Ask it to
   find broken behaviour, weakened checks and rule breaks. Fix (through a fix/ PR) or answer every point.
4. Only then, if I've said yes, do the one real Wassit run for DEMO.md's agent check and report its cost.

## Cost rules
- Use scout for any search that needs more than two reads. Read line ranges, not whole files.
- Don't re-read files you already reviewed unless they changed.
- After a small fix, run the changed package; run the full suite before each merge.
- No benchmarks. Fake provider and scripted agent until Phase 2 step 4.

## Reporting and stop points
After each repo: 3 to 5 lines covering what merged, checks passing, anything held.
Final report:
1. A table per repo: PR | verdict | merged commit or reason held.
2. Baseline vs final check results.
3. DEMO.md walkthrough result.
4. Follow-ups: Cornilia changes worth a new tag, Backend_Template fixes worth porting, held PRs.
5. Rollback: `git revert -m 1 <merge commit>` per PR, or reset to tag pre-merge-2026-11-15 (only with my OK).
Add one line to alphahack's PLAN.md progress log, through the last fix/ or docs PR, not a direct push.
Stop and ask me before: the first merge (the plan OK), merging anything held, any action on a
default branch other than a PR merge, deleting branches or tags, adding a dependency, or a paid model run.
```
