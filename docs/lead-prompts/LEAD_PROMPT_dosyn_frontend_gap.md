# LEAD_PROMPT_dosyn_frontend_gap

Checks whether the Wassit backend can support everything in Rena's DOSYN prototype. The output is
a gap report and a contract proposal. Nothing gets built in this job.

## How to run it
1. Subagents are already in `.claude/agents/` (scout, builder, tester, all Haiku). This job uses scout and tester only.
2. The prototype summary is saved at `documents/DOSYN_PROTOTYPE.md`.
3. If Rena's frontend code is on this machine, replace `<FRONTEND_PATH>` below with its folder. If not, delete that line and the lead judges against the summary only.
4. From `alphahack/`, launch `claude --model claude-fable-5-2` and paste everything inside the fence.
5. Teammate contract: the frontend codes against `backend/openapi.yaml`. Endpoint changes go through the lead and CONTRACT.md, never ad hoc. The frontend repo belongs to Rena and is never edited.

```text
You are the advisor and lead engineer for this job. You run on Fable 5.2. You plan, write specs,
review, and verify. The actual work is done by cheap subagents:
- scout (Haiku): read-only searching
- tester (Haiku): tests only
Do not spawn builder. This job changes no application code.

Follow WORKFLOW.md (spec and report format). Stack facts, ports, env vars and make targets are in
AGENTS.md. CONTRACT.md is the frozen Wassit contract; read all of it before anything else.
DEMO.md has the exact start commands. Your output files are documents/FRONTEND_GAP_REPORT.md and
documents/CONTRACT_PROPOSAL_dosyn.md.

## Goal
1. For every feature in documents/DOSYN_PROTOTYPE.md, decide whether the current backend in
   backend/ supports it, and prove it with evidence.
2. If frontend code exists at <FRONTEND_PATH>, list every API call it makes and check each one
   against backend/openapi.yaml AND the real handlers: method, path, auth header, request fields,
   response fields read, status codes handled.
3. Rank the gaps by what blocks the prototype's first end-to-end milestone: verify one pilot
   pharmacy, load its stock, send it a client enquiry using the quantity from a reviewed
   prescription, show the pharmacist's timestamped response in the client view.
4. Propose the contract changes that close those gaps. I decide what gets built.

## Inputs
- Backend: backend/ (Go), backend/openapi.yaml, backend/db/schema.dbml, backend/db/migrations/0001-0005.
- Existing checks: make check, make smoke (backend/scripts/smoke.sh), scripts/acceptance.py, scripts/seed_demo.sh.
- Prototype: documents/DOSYN_PROTOTYPE.md
- Frontend: <FRONTEND_PATH>
- Never edited, reference only: Backend_Template/, Cornilia/, cornilia-guard/, web/, the frontend repo.

## Non-negotiable rules (reject any verdict or proposal that breaks one)
1. Read-only on code. You may create only: the two report files, one probe test file
   (scripts/test_frontend_support.py), and one line in PLAN.md's progress log. Every other file
   stays untouched.
2. Every verdict cites evidence: an openapi.yaml line, a handler/service/sql file:line, a migration
   column, or a curl you ran. No evidence means "Unknown", never "Supported".
3. "Supported" means proven against the running local stack with seeded data. Code that exists but
   wasn't exercised is "Exists, not proven".
4. Frontend mock or demo data never counts as backend support. The prototype's "What's still
   simulated" list is a list of backend needs, not features.
5. CONTRACT.md product rules 1-7 are fixed: no diagnosing or substituting, only a human pharmacist
   at the target branch replies, no reply is no_response, every change is an event with its inverse,
   outbound goes through the outbox with an undo window, closing rules live in Go, the agent works
   only from DB data. No proposal may loosen them. Rule 8 (scope: customers, carts, orders,
   prescriptions, refills are out of scope) is the owner's call. Where the prototype needs something
   rule 8 excludes, mark it "Conflicts with rule 8" and give me options with their costs. Don't pick one.
6. Verify only against the local Docker stack and seeded demo data. AI_PROVIDER=fake or the scripted
   agent. No paid model calls without my OK.
7. No secrets in either report: env var names only, tokens trimmed.
8. No commits or pushes without my OK.

## Feature checklist
Verify every row. The hypotheses come from a quick read of the repo. Treat them as things to
disprove, not answers.

| ID | Prototype feature | Backend pieces to check | Hypothesis |
| --- | --- | --- | --- |
| F1 | Client log in (demo profile) and sign up | client auth, client table, login/signup routes | Missing: only staff email+PIN login exists |
| F2 | Pharmacist signs up with pharmacy name + town, sees Pending verification; pre-approved demo branch | branch/staff create route, verification status column, approval route | Missing: no signup, no verification status on branches |
| F3 | My medicines: only prescribed AND paid items | prescriptions, orders, payment status | Template has customers/prescriptions/orders routes; payment status absent; conflicts with rule 8 |
| F4 | Check availability from a reviewed prescription; quantity read-only; stop if quantity needs review | prescriptions.status (pending/verified), enquiry created on behalf of a client | Enquiries need a staff requester; no client-originated enquiry; conflicts with rule 8 |
| F5 | Pharmacy list + interactive map | GET /api/v1/branches with lat, lng, participating | Supported |
| F6 | Branch-by-branch responses with exact medicine, quantity, branch, time checked | GET /api/v1/enquiries/{id}: targets, reply_quantity, replied_at, medication | Supported for a staff requester only |
| F7 | Client enquiry history | enquiries listed per client | Missing for clients; staff scope=outgoing exists |
| F8 | Documents: personal folder, case folders, PDF/image upload, preview | multipart upload, private storage, folder tables | Missing |
| F9 | Pharmacy office: response queue, client + network enquiries, approvals, notifications | /inbox, /enquiries?scope=, /targets/approve, outbox kind=notification, a notifications list route | Network side supported; client enquiries and a notifications list missing |
| F10 | Stockroom: searchable inventory in condition folders, edit stock counts, In stock / Out of stock / Unknown | GET /branches/{id}/inventory, a write route for inventory_batches, medications.therapeutic_class, a defined status rule | Read exists; write route unclear (check commit_batch_stock.sql); "Unknown" has no defined meaning yet |
| F11 | Agent matches and routes an exact enquiry, tracks progress, audit trail | agent API, enquiry_events, runs, tool calls, SSE stream | Supported (acceptance 70/70 per PLAN.md) |
| F12 | Ask the network from a zero-stock branch: linked enquiry, same medicine and quantity | staff enquiry create + a link to the originating client enquiry | Create supported; link field missing |
| F13 | Receiving pharmacist confirms; timeout shown as Unable to confirm | reply staff-only (rule 2), follow-up -> no_response -> close unable_to_confirm | Supported; check that the follow-up-then-no_response path matches what the UI calls a "timeout" |
| F14 | Light/dark, EN/AR/FR with RTL | stable error codes the frontend can translate | Frontend-only, provided the frontend translates error codes, not messages |
| F15 | Future inventory import: Import -> Review uncertain matches -> Apply | none yet | Not built; only list the tables/routes it would need |

## How you work
Phase 0: Recon (scout only, up to 8 in parallel). One question each:
1. Every route registered (handlers.go, main.go) with its auth tag (public, staff, agent-ok,
   agent-only), compared with the paths in openapi.yaml. Flag routes in one but not the other.
2. Columns on branches, staff, medications and inventory_batches (migrations 0002 and 0004): lat/lng,
   participating, any status or verification field, therapeutic_class, updated_at, expiry, recalled.
3. Every write path to inventory_batches: sql files, queries, services, and whether any route exposes it.
4. Template customer/prescription/refill/cart/order routes: which exist, what they return, and
   whether any enquiry or agent code reads them (rule 8).
5. Auth: login, sessions, any signup, role or approval field, any client/customer credential.
6. Notifications: outbox kind=notification, and any route that lists notifications for a branch or client.
7. File upload or storage: any multipart handler, storage config, file tables.
8. Frontend (only if <FRONTEND_PATH> is set): every $fetch/useFetch/fetch/axios call with method, path,
   body fields, response fields read, headers; and where its mock data lives.
Read the openapi.yaml sections behind each finding yourself. Scouts find; you judge.

Then send me a plan of 8 to 10 lines: frontend found or not, your first verdict count
(supported / partial / missing / conflicts), the rule conflicts, whether the stack boots, and anything
you can't verify. WAIT for my OK before Phase 1.

Phase 1: Verify.
1. Start the stack the way DEMO.md and AGENTS.md describe (make docker-up, or make up && make migrate
   && make run with AI_PROVIDER=fake), then run bash scripts/seed_demo.sh. Demo staff PIN is 1234 (seed 0005).
2. Run the existing checks first and record the results: make check, make smoke, python scripts/acceptance.py.
   A pre-existing failure is recorded, not fixed.
3. Send tester ONE spec:
   TESTS frontend-support: file scripts/test_frontend_support.py (pytest against the running stack,
   base URL from env, same style as scripts/acceptance.py). One test per F-row, named
   test_F05_branches_have_coordinates and so on. Each asserts the exact status and the fields the
   prototype needs. Rows you expect to be missing are marked
   pytest.mark.xfail(strict=True, reason="gap F<id>: <what is missing>"), so the suite prints the gap
   list and fails loudly the day a gap gets closed. F14 and F15 get no test.
   Run it yourself. A row is "Supported" only when its test passes for real.
4. If there is frontend code: for each call, check it against the handler code, not just openapi.yaml.
   Record every field-name, type or status mismatch.

Phase 2: Write the reports yourself, not a subagent.
documents/FRONTEND_GAP_REPORT.md:
1. Verdict (5 lines max): can the backend support the prototype today, can it support the first
   milestone, and what blocks it.
2. Support matrix: F-id | feature | verdict (Supported / Exists, not proven / Partial / Missing /
   Conflicts with rule N / Frontend-only) | evidence | what's missing.
3. Frontend call check (only with frontend code): call | in openapi? | handler? | mismatches.
4. Milestone path: the 4 milestone steps, each with the exact endpoints in order and the gaps on that
   path, ranked by what blocks the demo first.
5. Rule conflicts: for each, option A (keep the rule, frontend changes) and option B (change the scope:
   new tables, routes and safety work it needs, and size S/M/L). No recommendation in this section.
6. Unknowns, and why.
documents/CONTRACT_PROPOSAL_dosyn.md: in CONTRACT.md's own section format (Data, Endpoints, Auth,
Ownership map, Acceptance checks). Cover only the gaps on the milestone path. Tag anything that
touches rule 8 "needs owner decision". Every proposed endpoint gets an acceptance check with
concrete input and exact expected status. Do not edit CONTRACT.md.

Phase 3: Fresh read. Spawn one general-purpose agent (model sonnet). Give it only the two reports,
CONTRACT.md, openapi.yaml and DOSYN_PROTOTYPE.md, not your notes. Ask it to find wrong verdicts,
missing gaps, and proposals that would break rules 1-7. Fix or answer every point.

## Cost rules
- Use scout for any search that needs more than two reads. Read line ranges, not whole files.
- openapi.yaml is 72KB: read only the path sections you need.
- Don't re-read files you already reviewed unless they changed.
- No benchmarks and no paid model runs. Fake provider and scripted agent only.

## Reporting and stop points
Deliver to me, in this order:
1. Paths of the two reports and the probe test file, plus the probe results (passed / xfailed).
2. A brief (8 lines max): the top 3 blockers for the milestone, the decisions only I can make
   (rule 8 first), your recommended order, and your honest view of the biggest risk.
3. An offer, not an action: turn the approved proposal into a build lead prompt (W2 freeze, W3 slices).
Add one line to PLAN.md's progress log: "- <date> DOSYN frontend gap check: <verdict in one line>".
Stop and ask me before: editing any other existing file, committing, pushing, running migrations
against anything other than the local stack, calling a paid model API, or adding a dependency.
```
