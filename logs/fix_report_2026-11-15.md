# Wassit QA, Swarm Bug Hunt, and Remediation Report (2026-11-15)

- **Branch:** `fix/audit-and-swarm-2026-11-15` (cut from `test/full-2026-11-15` at `529a9a6`)
- **Primary Test Target:** `http://localhost:8114` (PostgreSQL `5574`, `UNDO_WINDOW=2s`, `WAIT_UNIT=200ms`, `AGENT_TOKEN=dev-agent-token`)

---

## 1. Pinned Bugs Fixed (`BUG-01` through `BUG-15`)

| ID | Summary | Root Cause & Fix | Verification |
| --- | --- | --- | --- |
| `BUG-01` | Re-adding a cancelled/retracted pharmacy returned `500` (`UNIQUE(enquiry_id, branch_id)`) | `backend/internal/services/enq_enquiries.go` (`AddTargets`): detect previously `cancelled` target rows and reactivate via `UpdateEnquiryTarget` (resetting `FollowUps: 0` and reply fields) instead of `CreateEnquiryTarget`. | `tests/api/test_api_known_bugs.py` (2 tests) — `PASSED` |
| `BUG-02` | Undoing `marked_no_response` after a late reply erased the pharmacist's reply | `backend/internal/services/enq_outbox.go` (`Undo` -> `case "set_target_status"`): load target and return `409 conflict` if `t.Status != queries.TargetNoResponse`. | `tests/api/test_api_known_bugs.py` — `PASSED` |
| `BUG-03` | Cancelling an enquiry inside the undo window left queued messages to be delivered | `backend/internal/services/enq_enquiries.go` (`CloseEnquiry`): cancel all `queued` outbox messages (and `cancelled` enquiry targets when outcome is `cancelled`). `enq_outbox.go` (`DispatchDue`): lock enquiry (`GetEnquiryForUpdate`), re-read message, and cancel queued messages on finalized enquiries. | `tests/api/test_api_known_bugs.py` — `PASSED` |
| `BUG-04` | Finished agent runs could be `PATCH`ed again, overwriting tokens/cost/status | `backend/internal/services/enq_agentapi.go` (`FinishRun`): inside `InTx` with `GetEnquiryForUpdate`, verify `existing.Status == queries.RunStatusRunning`, else `409 conflict`. | `tests/api/test_api_known_bugs.py` — `PASSED` |
| `BUG-05` | Sending while clarification was pending moved status to `waiting` and broke `clarify` | `backend/internal/services/enq_outbox.go` (`Send`): reject with `409 conflict` when `awaiting_clarification` or `ClarificationQuestion != "" && ClarificationAnswer == ""`. `enq_replies.go` (`Clarify`): also allow answering when `ClarificationQuestion != "" && ClarificationAnswer == ""`. | `tests/api/test_api_known_bugs.py` — `PASSED` |
| `BUG-06` | B2C customer/prescription/refill/cart/order routes were open to anonymous & agent callers | `backend/internal/handlers/pharmacy.go` (`staffPharmacy`, `registerPharmacy`): wrap all 15 patient/cart/order routes with `h.auth(staffOnly, fn)` when `h.enq != nil` (preserving `pharmacy_test.go` unit tests). Updated `backend/scripts/smoke.sh` to send `Bearer $TOKEN_A`. | `tests/api/test_api_known_bugs.py` (7 parametrized/direct tests) + `smoke.sh` — `PASSED` |
| `BUG-07` | `Reply` accepted `kind="available"` with `quantity=0` | `backend/internal/services/enq_replies.go` (`Reply`): require `in.Quantity != nil && *in.Quantity >= 1` when `in.Kind == "available"`. | `tests/api/test_api_known_bugs.py` — `PASSED` |
| `BUG-08` | `POST /agent/runs` accepted invalid `trigger` or empty `model` and cleared `wake_trigger` | `backend/internal/services/enq_agentapi.go` (`StartRun`): validate `trigger` (`created\|clarified\|approved\|reply\|timer`) and non-empty `model` before `ClearEnquiryWake`. | `tests/api/test_api_known_bugs.py` — `PASSED` |
| `BUG-09` | `agent/scripted.py` failed on `100mcg`, `x 0.5g`, and `badde augmentin 1g 2 3elab` | `agent/scripted.py` (`_QUANTITY_PATTERNS`, `_STRENGTH`, `QUANTITY_WORDS`): require strength unit (`mcg\|mg\|g`), convert `mcg` to `mg`, avoid eating `x 0` in `x 0.5g`, and strip `3elab`/`elab`/`hab`/Arabic pack words. | `agent/tests/test_strengths.py` — `PASSED` |
| `BUG-10` | `agent/tools/_http.py` let `TimeoutError`/`ConnectionResetError` escape instead of returning error dict | `agent/tools/_http.py` (`call`): catch `(urllib.error.URLError, TimeoutError, ConnectionError, OSError)` and return `{"error": {"code": "backend_unreachable", ...}}`. | `agent/tests/test_http_and_limits.py` — `PASSED` |
| `BUG-11` | `agent/runner.py` treated Cornilia `System_Error` on terminal tools as success and ended the run | `agent/runner.py` (`make_tracer`): set `is_error = '"error"' in text or text.startswith("System_Error")`. | `agent/tests/test_known_bugs.py` — `PASSED` |
| `BUG-12` | `search_medications`, `find_candidates`, and `tool_error` could exceed 2,000 JSON chars | `agent/tools/catalogue.py`, `agent/tools/enquiry.py`, `agent/tools/_http.py`: truncate long strings and trim trailing list items so JSON stays `< 1900` chars (`tool_error` caps message at 500 chars). | `agent/tests/test_http_and_limits.py` — `PASSED` |
| `BUG-13` | `scripts/show_results.py` printed raw Python traceback when backend was unreachable | `scripts/show_results.py` (`req`): catch network/HTTP/OS errors, print a clean message to `stderr`, and exit `1`. | `tests/test_scripts_cli.py` — `PASSED` |
| `BUG-14` | `scripts/run_cases.py` overwrote tracked `agent/cases/results.md` on every run | `scripts/run_cases.py` (`main`): add `--output` flag (default `None`, prints to stdout only unless `--output` is passed). | `tests/test_scripts_cli.py` — `PASSED` |
| `BUG-15` | Two concurrent replies to one target could both succeed | `backend/internal/services/enq_replies.go` (`Reply`): lock enquiry via `GetEnquiryForUpdate` and re-read target via `GetEnquiryTarget` before checking `t.Status`. | `tests/api/test_api_races.py` — `PASSED` |

---

## 2. Phase 2 Swarm Findings Fixed

| ID | Summary | Fix |
| --- | --- | --- |
| `SWARM-01` | Cancelling a queued `follow_up` message did not decrement `target.follow_ups` | `backend/internal/services/enq_outbox.go` (`CancelMessage`): decrement `t.FollowUps` when cancelling `OutboxKindFollowUp`. |
| `SWARM-02` | `Undo` of `messages_queued` could retract/cancel a target that had already replied | `backend/internal/services/enq_outbox.go` (`undoCancelMessages`): return `409 conflict` if target already has `available`/`unavailable`/`alternative_offered`. |
| `SWARM-03` | Reopening a closed agent enquiry via undo left `wake_trigger=""` | `backend/internal/services/enq_outbox.go` (`Undo` -> `case undoOpReopen`): set `e.WakeTrigger = WakeCreated` for agent enquiries when appropriate. |
| `SWARM-04` | Agent could propose targets while enquiry was `awaiting_clarification` | `backend/internal/services/enq_enquiries.go` (`AddTargets`): return `409 conflict` if `a.IsAgent() && e.Status == queries.EnquiryAwaitingClarification`. |
| `SWARM-05` | `DispatchDue` ran without locking enquiry/re-checking message status | `backend/internal/services/enq_outbox.go` (`DispatchDue`): lock enquiry, re-read message, skip non-queued, cancel queued on finalized enquiries. |
| `SWARM-06` | `CancelMessage` / `undoCancelMessages` relied only on dispatcher tick instead of `send_after` | `backend/internal/services/enq_outbox.go` (`CancelMessage`, `undoCancelMessages`): check `s.now().Before(m.SendAfter)`. `backend/cmd/server/main.go`: reduced dispatcher tick from `2s` to `250ms`. |
| `SWARM-07` | Cancelled targets could still read enquiry details and event streams | `backend/internal/services/enq_enquiries.go` (`GetEnquiryDetail`), `enq_stats.go` (`EventsSince`): require `t.Status != queries.TargetCancelled`. |
| `SWARM-08` | `StartRun` allowed starting runs on closed enquiries; `RecordToolCall` allowed recording on finished runs or negative `duration_ms` | `backend/internal/services/enq_agentapi.go` (`StartRun`, `RecordToolCall`): enforce `!isFinal(e)`, `run.Status == queries.RunStatusRunning`, non-empty cleaned `name`, and `0 <= DurationMs <= math.MaxInt32`. |
| `SWARM-09` | Large integer inputs (`input_tokens`, `output_tokens`, `steps`, `duration_ms`, `quantity`) overflowed PG `INT4` (`500`) | `backend/internal/services/enq_agentapi.go`, `enq_replies.go`: bound to `math.MaxInt32` / `maxEnquiryQuantity`. |
| `SWARM-10` | Null bytes (`\x00`) in JSON strings caused PostgreSQL `22021` `500` errors | `backend/internal/services/contracts.go` (`clean`, `FromStore`): strip `\x00` in `clean()` and map `"invalid byte sequence"` to `400 validation_failed`. |
| `SWARM-11` | `scripted._choose_medication` lost `raw_request` strength when clarification answer gave only medicine name | `agent/scripted.py` (`_choose_medication`): use `wanted = _strengths(answer) or _strengths(raw)`. |

---

## 3. Verification Matrix (Post-Fix, All Strict, Zero `xfail`)

- `cd backend && make check`: **PASS** (`go build ./...`, `go vet ./...`, `go test ./...`)
- `cd backend && TEST_DATABASE_URL='postgres://alphahack:alphahack@localhost:5574/alphahack?sslmode=disable' go test ./... -count=1`: **PASS** (all 6 Go packages)
- `cd backend && COMPOSE_PROJECT_NAME=testfull make check-queries`: **PASS** (86 SQL files + layering checks)
- `cd agent && .venv/bin/python -m pytest -q`: **130 passed in 0.26s** (0 xfailed)
- `WASSIT_LIVE_URL=http://localhost:8114 agent/.venv/bin/python -m pytest -q tests`: **273 passed in 219.14s** (0 xfailed)
- `BASE_URL=http://localhost:8114 bash backend/scripts/smoke.sh`: **83 passed, 0 failed**
- `BACKEND_URL=http://localhost:8114 agent/.venv/bin/python scripts/acceptance.py`: **exit 0** (all 7 checks + extra auth checks passed)
- `BACKEND_URL=http://localhost:8114 agent/.venv/bin/python scripts/demo_flow.py --no-pause --drive-agent --policy scripted`: **exit 0**

---

## 4. PostgreSQL `5573` Cleanup SQL (Not Executed — For Manual Use If Needed)

> Note: No test or swarm agent in this session touched port `8113` or `5573`. If you want to clear stale test enquiries from an earlier `5573` database, run:

```sql
BEGIN;
DELETE FROM enquiry_agent_tool_calls;
DELETE FROM enquiry_agent_runs;
DELETE FROM outbox_messages;
DELETE FROM enquiry_events;
DELETE FROM enquiry_targets;
DELETE FROM enquiries;
COMMIT;
```
