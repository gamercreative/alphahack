# Test report 2026-11-15: backend, agent and end-to-end scripts

Branch `test/full-2026-11-15`, cut from main `cc7ae6a`. Tests only: no application file changed.
Scope: `backend/` (Go API, Postgres, migrations, outbox dispatcher, SSE, agent API), `agent/` (runner and
tools) and `scripts/`. Out of scope: `Backend_Template/`, `Cornilia/`, `cornilia-guard/`, `web/`.
Expected behaviour comes from CONTRACT.md first, then `backend/openapi.yaml`, then the code.

## 1. Verdict on demo safety

**The demo as written in docs/DEMO.md is safe to run.** The four parts pass end to end with the scripted
policy and with the LAN model, acceptance is 70/70, smoke 83/83, and every contract rule the demo shows
(rules 1 to 6) is proven by tests that go red when the rule is removed (section 6).

Stay on the script. Two bugs sit one step off it:

- **BUG-01:** re-adding a pharmacy that was cancelled (Part 1 cancels Baabda) or retracted (Part 3 undoes
  Verdun) returns `500 internal_error`. Do not re-add a pharmacy on stage.
- **BUG-03:** cancelling a whole enquiry inside the undo window still delivers its queued messages. The line
  "cancel inside the window and the target never sees it" holds for cancelling a message or undoing the
  send, not for cancelling the enquiry.

Before the demo: use a database without old open agent enquiries (they keep waking the agent, section 9
R-4), and if you run the agent on the LAN model keep `CORNILIA_PROVIDER=ollama` and `CORNILIA_API_KEY=ollama`.

## 2. Results

| Suite | Command (section 3) | Result |
| --- | --- | --- |
| Go, normal | `make check` | pass: 707 test runs pass, 17 skip (need `TEST_DATABASE_URL`), 0 fail |
| Go, with integration database | `TEST_DATABASE_URL=... go test ./...` | pass: 757 pass, 0 skip, 0 fail |
| SQL and layering rules | `make check-queries` | pass |
| Agent unit | `cd agent && pytest -q` | 118 passed, 10 xfailed (pinned bugs) |
| Root tests, no server | `pytest -q tests` | 22 passed, 250 skipped (live tests), 1 xfailed |
| Root tests, live | `WASSIT_LIVE_URL=... pytest -q tests` | 255 passed, 18 xfailed |
| Existing live scripts (baseline, unchanged) | `smoke.sh`, `acceptance.py`, `demo_flow.py` | 83/83, 70/70, 4 parts pass |

Baseline before this job (main `cc7ae6a`): Go 547 pass / 11 skip (566 with the integration database); agent 45;
root 7; smoke 83/83; acceptance 70/70.
New tests: 191 Go test runs, 83 agent tests, 266 root tests (250 of them live).
Bugs pinned: **15** (2 high, 6 medium, 7 low by demo risk), as 28 strict expected-failures.

## 3. How to run everything

```bash
# 1. Normal suite (no server, no database). Must stay green.
cd backend && make check && make check-queries        # check-queries needs the compose db up
cd agent && .venv/bin/python -m pytest -q
agent/.venv/bin/python -m pytest -q tests              # from the repo root; live tests skip

# 2. Go integration tests (real Postgres, their own rows, cleaned up)
cd backend && TEST_DATABASE_URL='postgres://alphahack:alphahack@localhost:5432/alphahack?sslmode=disable' go test ./internal/queries/ -count=1

# 3. Live suite: a dedicated local stack with short timers
cd backend && COMPOSE_PROJECT_NAME=testfull DB_PORT=5574 docker compose up -d --wait db
COMPOSE_PROJECT_NAME=testfull DATABASE_URL= ./scripts/migrate.sh up
PORT=8114 DATABASE_URL='postgres://alphahack:alphahack@localhost:5574/alphahack?sslmode=disable' \
  AI_PROVIDER=fake UNDO_WINDOW=2s WAIT_UNIT=200ms AGENT_TOKEN=dev-agent-token \
  AGENT_PRICE_IN_PER_MTOK=0.10 AGENT_PRICE_OUT_PER_MTOK=0.50 go run ./cmd/server
# in another terminal, from the repo root (about 5.5 minutes):
WASSIT_LIVE_URL=http://localhost:8114 agent/.venv/bin/python -m pytest -q tests
```

If the server uses other timers or prices, tell the tests: `WASSIT_UNDO_WINDOW` (seconds), `WASSIT_WAIT_UNIT`
(seconds), `WASSIT_PRICE_IN`, `WASSIT_PRICE_OUT`. `AGENT_TOKEN` is read from the environment.

Rules of the live suite (enforced by `tests/conftest.py`):
- it skips without `WASSIT_LIVE_URL` and fails loudly if the URL is set but the server is down;
- every test creates its own enquiry and asserts on it only; agent enquiries are cancelled at teardown;
- it refuses to start if the backend already has 10 or more pending agent wake-ups (section 9 R-2);
- `tests/e2e` and one test in `tests/test_scripts_cli.py` start the real runner, which processes every
  pending wake-up on that backend: do not run two live sessions against one backend at the same time.

A pinned bug is `@pytest.mark.xfail(strict=True, reason="BUG-nn ...")`. When the bug is fixed the test
passes, strict mode reports that as a failure, and the fixer removes the marker. List them with
`pytest -rx`. No Go test needed a build tag: every pinned bug is on the Python side or reachable over HTTP.

## 4. Coverage by layer

| Layer | Files | What it proves |
| --- | --- | --- |
| Go unit, handlers | `backend/internal/handlers/enq_authmatrix_test.go` (new, 160 subtests) | every one of the 26 guarded routes rejects a missing, unknown, malformed or wrong-kind credential with the exact status and code |
| Go integration, SQL | `backend/internal/queries/enquiry_sql_integration_test.go` (new) | 9 CHECK constraints and the target UNIQUE constraint by SQLSTATE and name; wake-up, due-outbox, stats and inbox queries |
| Live API (black box) | `tests/api/test_api_*.py` (13 files, 233 tests) | handler + service + SQL together, per route: exact status and error code, state after the call, events written |
| Live concurrency | `tests/api/test_api_races.py` (9) | two simultaneous requests: one winner, one 409, no duplicate effect |
| Live end to end | `tests/e2e/test_scripted_flow.py` (6) | the real runner (scripted policy) through acceptance checks 2 and 6; runner exit codes |
| Agent unit | `agent/tests/test_{strengths,http_and_limits,runner_main,scripted_branches,known_bugs}.py` (83) | strength parsing, HTTP helper, result size, runner entry point, every scripted branch |
| Static | `tests/test_openapi_routes.py` (7) | code routes = openapi routes (63 = 63); all 27 CONTRACT routes exist with the stated auth |
| Scripts | `tests/test_scripts_cli.py` (11) | exit codes of the demo scripts with the backend down and up |

Go statement coverage of `go test`: ai 93.8%, config 94.0%, http 93.5%, handlers 55.6%, services 33.1%,
queries 3.5% without the integration database and 71.7% with it. The services number does not count the
live suite, which is where the enquiry service is exercised. I tried to measure it with a
coverage-instrumented server binary; it wrote no data on shutdown, so there is no number for it here.

### Coverage by route (the 27 CONTRACT routes)

Every route is covered for auth by the Go matrix and for cross-pharmacy access by `test_api_access.py`
where it applies. "Files" names the live files that test its behaviour (prefix `tests/api/test_api_`).

| Route | Files |
| --- | --- |
| POST /auth/login, GET /me | auth |
| POST /enquiries, GET /enquiries | validation, targets |
| GET /enquiries/{id} | targets, access, agent_runs |
| POST /enquiries/{id}/medication | targets, undo, access |
| POST /enquiries/{id}/clarification-request, /clarify | agent_flow, known_bugs (BUG-05) |
| GET /enquiries/{id}/candidates | targets, validation |
| POST /enquiries/{id}/targets | targets, agent_flow, races, known_bugs (BUG-01) |
| POST /enquiries/{id}/targets/approve | agent_flow, races |
| POST /enquiries/{id}/send | outbox, agent_flow, races |
| POST /messages/{id}/cancel | outbox, races |
| POST /targets/{id}/follow-up, /no-response | outbox, replies, races |
| GET /inbox | outbox, undo, known_bugs (BUG-03) |
| POST /targets/{id}/reply | replies, races, known_bugs (BUG-07, BUG-15 in races) |
| POST /enquiries/{id}/notify | outbox, stats_sse |
| POST /enquiries/{id}/wait | agent_runs |
| POST /enquiries/{id}/close | closing, validation, races |
| POST /events/{id}/undo | undo, races, known_bugs (BUG-02) |
| GET /enquiries/{id}/stream | stats_sse |
| GET /agent/wakeups, POST /agent/runs, PATCH /agent/runs/{id}, POST .../tool-calls | agent_runs, known_bugs (BUG-04, BUG-08) |
| GET /stats | stats_sse, agent_runs, closing |

Deliberately not tested: the template's `/v1` chat routes and the pharmacy cart, order and refill logic
(outside the Wassit contract, already unit-tested, checked here only for who can reach them), real model
quality, load, the frontend, cornilia-guard.

## 5. Bugs, ranked by demo risk

Each has a test that states the correct behaviour and fails today. Repro steps use the demo logins
(A = a@wassit.demo and so on, PIN 1234) against any local backend.

### High

**BUG-01: re-adding a cancelled or retracted pharmacy returns 500.**
Repro: A creates an enquiry, adds Achrafieh and Baabda, sends, cancels Baabda's message inside the undo
window, then `POST /api/v1/enquiries/{id}/targets {"branch_ids":["branch-baabda"]}` -> `500 internal_error`.
Same after undoing a delivered send (the retraction path). Cause: the cancelled target row still holds
`UNIQUE(enquiry_id, branch_id)`, the service treats the branch as free and inserts, and the unique violation
is not handled. Tests: `test_bug01_readd_pharmacy_after_cancel_in_window`, `test_bug01_readd_pharmacy_after_retraction`.

**BUG-03: a cancelled enquiry still delivers its queued messages (rule 5).**
Repro: A creates, adds Achrafieh, sends, and inside the undo window closes with `{"outcome":"cancelled"}`
-> 200. A few seconds later the message is `sent` and the cancelled enquiry is in B's inbox. Closing does
not touch the outbox and the dispatcher does not check the enquiry status.
Test: `test_bug03_cancelled_enquiry_must_not_deliver_queued_messages`.

### Medium

**BUG-02: undoing a no-response mark erases a pharmacist's later reply (rules 2, 4).**
Repro: send to B; follow up; wait; mark no_response; B replies `available`; A undoes the
`marked_no_response` event -> 200 and B's target is back to `sent`, so "close available" now fails with 409.
The stored inverse is applied without checking that the target is still `no_response`.
Test: `test_bug02_undo_no_response_must_not_erase_a_late_reply`.

**BUG-15: two simultaneous replies to one target are both accepted.**
Repro: two `POST /targets/{id}/reply` requests at the same instant from the target pharmacy, one
`available`, one `unavailable` -> both 200, two `reply_received` events, the last write wins. Reproduced
20 times out of 20. The status check reads the target before the enquiry lock is taken.
Test: `test_race_two_replies_to_one_target_only_one_wins`.

**BUG-05: sending while a clarification question is pending strands the question.**
Repro: agent enquiry; agent posts a clarification request; A adds a target and sends -> status `waiting`;
`POST /clarify` -> `409 no clarification is pending`. The requester can never answer.
Test: `test_bug05_pending_question_survives_a_send`.

**BUG-11: a terminal tool that fails inside Cornilia is recorded as a completed run (model policy only).**
When the model calls `close_enquiry`, `wait` or `ask_requester` with invalid JSON or a wrong argument name,
Cornilia returns the text `System_Error: ...`; `agent/runner.py` only treats text containing `"error"` as a
failure, ends the run and records `completed`, although nothing was closed. Tests:
`agent/tests/test_known_bugs.py::test_bug11_*`. A fix exists on the local branch `final/2026-11-15`.

**BUG-06: patient data needs no credential (rule 8).**
Repro: `curl http://localhost:8080/api/v1/customers` with no header -> 200 with names, phone numbers,
allergies and chronic conditions; the same for `/customers/{id}/prescriptions`, `/refills`, `/orders` and
`/carts/{id}`, and with the agent credential. The agent's tools never call these routes (proved), but
nothing stops a caller. The servers listen on all interfaces. Tests: `test_bug06_*` (7).

**BUG-13: `scripts/show_results.py` prints a raw traceback when the backend is down.**
Repro: `BACKEND_URL=http://127.0.0.1:9 python3.13 scripts/show_results.py`. Test:
`test_show_results_backend_down_exits_nonzero`.

### Low

| Id | Bug | Repro | Test |
| --- | --- | --- | --- |
| BUG-04 | a finished agent run can be finished again; tokens, status and cost are overwritten | `PATCH /agent/runs/{id}` twice with different numbers: both 200, cost changes | `test_bug04_finished_run_cannot_be_finished_again` |
| BUG-07 | a reply `available` with quantity 0 is accepted | B replies `{"kind":"available","quantity":0}` -> 200 | `test_bug07_available_reply_needs_a_positive_quantity` |
| BUG-08 | `POST /agent/runs` accepts an unknown trigger or no model, creates a run that never finishes and clears the wake-up | `{"enquiry_id":..., "trigger":"bogus"}` -> 201 | `test_bug08_run_with_unknown_trigger_is_rejected_and_keeps_the_wake` |
| BUG-09 | the scripted policy parses strengths differently from the catalogue tool: `100mcg` is read as 100 mg, `x 0.5g` as 5 g, and the bare numbers in "badde augmentin 1g 2 3elab" count as strengths | `scripted._strengths("ventolin 100mcg")` -> `{100.0}`; catalogue gives `{0.1}` | `agent/tests/test_strengths.py` (3 xfail params) |
| BUG-10 | the tools' HTTP helper raises on a read timeout or connection reset instead of returning an error dict | `_http.call` with `urlopen` raising `TimeoutError` | `test_call_read_timeout_returns_error_dict`, `test_call_connection_reset_returns_error_dict` |
| BUG-12 | tool results can exceed the contract's 2,000 characters (`search_medications`, `find_candidates`, long backend error messages) | long strings from the backend pass through uncut: 8,035, 4,076 and 5,033 characters | `agent/tests/test_http_and_limits.py` (3) |
| BUG-14 | `scripts/run_cases.py` rewrites the tracked `agent/cases/results.md` on every run, with no flag to write elsewhere | run it once, `git status` | `test_run_cases_does_not_modify_tracked_results_file` |

## 6. Proof for each CONTRACT rule

"Mutation" = I broke the rule in a throwaway copy of the code, rebuilt, and the named tests went red
(section 8). Rules without a mutation are proved by exact-status tests only.

| Rule | Proved by | Mutation |
| --- | --- | --- |
| 1. Never substitute; an alternative is an option, never a match | `replies::test_alternative_is_an_option_not_a_match`, `closing::test_close_available_at_alternative_branch_409`, `agent/tests/test_scripted_branches.py::test_alternative_never_closes_available`, `::test_requested_strength_not_in_catalogue_never_substitutes`, e2e flow (alternative ends `alternative_offered`, outcome `unable_to_confirm`) | M2, M12, M13 |
| 2. Only a human at the target pharmacy replies; agent gets 403 `agent_forbidden` on reply, approve, clarify | Go auth matrix (9 staff-only routes), `replies::test_reply_by_agent_403_agent_forbidden`, `::test_reply_by_other_pharmacy_403`, `::test_reply_by_requester_403`, `agent_flow::test_agent_cannot_answer_...`, `::test_agent_cannot_approve_...`, e2e (no agent event of a human-only type) | M1, M5, M9. Gap: BUG-15 |
| 3. No reply is `no_response`, never `unavailable` | `outbox::test_no_response_*` (4), `replies::test_silence_is_no_response_not_unavailable`, `closing::test_close_unavailable_with_no_response_409`, scripted `test_all_no_response_is_not_unavailable`, e2e | M6 |
| 4. Every state change writes an event and, where reversible, its inverse; undo runs it | `undo` (12 tests: restore, event_undone, twice -> 409, non-reversible -> 409), events asserted in targets, outbox, replies, closing; races (one `event_undone`) | M4. Gap: BUG-02 |
| 5. Outbound messages wait `UNDO_WINDOW`; cancel inside it means the target never sees it | `outbox::test_queued_message_not_in_inbox`, `::test_cancel_in_window_target_never_sees_it`, `::test_cancel_after_sent_409`, `undo::test_undo_send_inside_window`, `::test_undo_send_after_delivery_queues_retraction`; fault injection F1 (queue survives a restart) | M3. Gap: BUG-03 |
| 6. Closing rules live in Go | `closing` (17 tests, the whole outcome matrix), `agent_flow::test_agent_cannot_close_*` (3) | M2 |
| 7. Agent works only from catalogue and pharmacy data; ambiguous -> ask | e2e `test_e2e_ambiguous_request_asks_the_requester`, scripted `test_two_strengths_no_strength_in_request_asks`, `::test_no_catalogue_match_asks_requester`, `agent/tests/test_strengths.py` | M12. Note: on the backend nothing checks that a target is a candidate; with a model, "ask when ambiguous" is prompt-only |
| 8. Agent never reads customers, carts, orders, prescriptions | existing `agent/tests/test_tools.py` (exactly 12 tools, none for those routes; sensitive fields stripped). Not enforced by the backend: BUG-06 | none |

## 7. Contract, openapi and code mismatches

Routes agree everywhere: 63 routes in code and in `openapi.yaml`, all 27 CONTRACT routes exist with the
stated auth (`tests/test_openapi_routes.py`). The differences are in behaviour:

| # | Mismatch |
| --- | --- |
| 1 | Event type `targets_added` (staff adds targets) is written by the code and is not in CONTRACT's event list. |
| 2 | `?access_token=<staff token>` is accepted on every guarded route when there is no Authorization header. Not in CONTRACT. Tokens in URLs end up in logs. |
| 3 | `POST /agent/runs`: CONTRACT lists `{enquiry_id, trigger, model}`; the code accepts a missing model and any trigger (BUG-08). |
| 4 | Agent tools: CONTRACT says results under 2k characters; only `get_enquiry` enforces it (BUG-12). `_http.call` has an extra `query=` parameter not in CONTRACT. |
| 5 | SSE: CONTRACT says one `enquiry_event` per new row. The stream replays every event from the first on each connect, ignores `Last-Event-ID`, sends no heartbeat, and ends without a final event at `REQUEST_TIMEOUT` (300 s). |
| 6 | Stats: "totals" are the sum of the current page (at most 200 rows), not of all enquiries. |
| 7 | Inbox: as CONTRACT says, it lists targets whose enquiry message was sent. So a retracted or cancelled target stays in the pharmacy's inbox, and replying to it returns 409. |
| 8 | A body over 1 MiB gets `400 bad_request`, not 413; with a much larger upload the client can see a connection reset instead of the 400. |
| 9 | `GET /enquiries?status=bogus` returns 200 with the filter ignored; `limit` above 200 is silently capped. |
| 10 | Cancelling an already cancelled message answers "message already sent; undo ...": the status is right (409), the text is wrong. |
| 11 | The `Bearer` scheme is matched case-sensitively: `bearer <token>` gets 401. |
| 12 | A run object has a top-level string field `error`; the error envelope uses the same key for an object. Clients must check the type. |
| 13 | `make migrate-down` (reverts 0005) fails as soon as one enquiry exists: enquiries reference the seeded staff with no ON DELETE rule. On an empty database the full down and up round trip is clean. |
| 14 | AGENTS.md and the scripts set `CORNILIA_TIMEOUT`; the pinned Cornilia never reads it (fixed 120 s). |

## 8. Test quality

**Mutation checks: 14 of 14 killed.** Each was applied to a copy under the scratchpad, never to the repo.

| # | Break | Tests that went red |
| --- | --- | --- |
| M1 | no `agent_forbidden` on staff-only routes | 6: agent answer, agent approve, agent reply, `/me`, `/stats`, `/stream` |
| M2 | close `available` without an available reply | 4 in closing and agent_flow |
| M3 | cancel allowed after the message was sent | 3 in outbox and races |
| M4 | undo allowed twice | 2 in undo and races |
| M5 | any pharmacy may reply for a target | 2 in replies |
| M6 | no_response without waiting | 1 in outbox |
| M7 | any pharmacy may change another's enquiry | 27 in access and closing |
| M8 | unlimited follow-ups | 2 in outbox and races |
| M9 | staff token accepted on agent-only routes | Go `TestAuthMatrixStaffOnAgentOnlyRoutes` |
| M10 | wake-ups include closed enquiries (SQL) | Go `TestIntegrationAgentWakeups` |
| M11 | HITL counts undone events (SQL) | Go `TestIntegrationStatsRow` |
| M12 | `strength_match` offered without a strength | 6 in agent tests |
| M13 | scripted policy treats an alternative as available | 3 in agent tests |
| M14 | runner exits 0 when the backend is down | 2 in `test_runner_main.py` |

**Flakiness.** The live suite ran four times: twice on the wave 1 files (225 passed, 15 xfailed both times)
and twice on the final set (255 passed, 18 xfailed). Go ran three times with `-race -shuffle=on`; the agent
and offline root suites three times each; all identical. Two tests were flaky when first written and were
fixed, not retried:
- the oversized-body test sent 2.1 MB and sometimes got a connection reset; it now sends 1 KiB over the
  limit, which is deterministic (the reset itself is mismatch 8);
- a wake-up assertion depended on how many agent enquiries earlier tests had left open (the wake-up list
  is capped at 10); the shared fixture now cancels each test's agent enquiries and refuses a backlog.

**Fault injection (run by hand on the test stack).**

| Fault | Result |
| --- | --- |
| Backend stopped (SIGTERM) while a message is queued, restarted 3 s later | stops in 0.02 s; after restart the old session token still works and the queued message is delivered once |
| Database container restarted under a running backend | requests during the restart get `500 internal_error`; the API recovers by itself; a message queued before the restart is delivered once |
| Database stopped | `/healthz` answers 500 "database unreachable" |
| Backend stopped for 8 s under a looping runner | the runner keeps polling and processes a new enquiry after the backend returns; Ctrl-C exits 0 |
| Backend down or wrong agent token for `runner.py --once` | exit 1, no traceback, the wake-up is kept |
| Migrations up, down in reverse, up again on an empty database | clean (23 tables, 0, 23) |
| `0005` down with one enquiry present | fails on a foreign key (mismatch 13) |

**Review.** Every tester file was scanned for weak patterns (status ranges, `or` in asserts, skips, bare
4xx without a code) with no hits, and I read the access, closing, outbox and undo files in full.

## 9. Risks without a pinned test

- **R-1: a run that never finishes strands the enquiry.** Starting a run clears the wake-up. If the runner
  dies or the backend goes away before the run is finished, the run stays `running` and nothing wakes the
  enquiry until a human approves, clarifies or replies. CONTRACT only covers the max-steps case.
- **R-2: wake-ups are capped at the 10 oldest.** A backlog of enquiries whose runs cannot start would hide
  newer ones.
- **R-3: cancel against the dispatcher.** Cancel reads the message without a row lock and the update has
  no "still queued" condition, so a cancel landing inside the dispatcher's transaction could mark a sent
  message cancelled. The window is microseconds; I could not reproduce it.
- **R-4: stale agent enquiries cost model calls.** In one measured demo flow on the LAN model, a leftover
  open agent enquiry woke on its timer six times.

## 10. Ownership

Lead (written by me): `tests/conftest.py`, `tests/api/test_api_kit_selfcheck.py`, `test_api_known_bugs.py`,
`test_api_races.py`, `tests/e2e/test_scripted_flow.py`, `agent/tests/test_known_bugs.py`, this report.
One Haiku tester per file for the other 19 test files. After a tester finished I made these edits: xfail
markers for confirmed bugs, the notify status (200) in the SSE test, the oversized-body size, one
parameter removed from a scripted-branch test, and a duplicated comment in the Go SQL test.
