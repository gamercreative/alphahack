# Swarm Bug Hunt Report — 2026-11-15

- **Target Environment:** `http://localhost:8114` (`UNDO_WINDOW=2s`, `WAIT_UNIT=200ms`, `AGENT_TOKEN=dev-agent-token`)
- **Database:** `postgres://alphahack:alphahack@localhost:5574/alphahack?sslmode=disable`
- **Base Commit:** `529a9a6` (`test/full-2026-11-15`)
- **Swarm Topology:**
  - **Agent A (`3d5a2b4a-8edb-40aa-9eeb-759b176948b9`):** State Machine & Concurrency Race Fuzzer
  - **Agent B (`893611cc-d390-4717-8428-e0042e72710b`):** Auth, Cross-Pharmacy Boundaries (IDOR), & Input Validation Auditor
  - **Agent C (`f87e321a-1b84-4a52-947d-612a40cfdf85`):** Agent Runner, Tools, Wakeup Queue, & SSE Stream Verifier

---

## Executive Summary of Newly Discovered Bugs (Beyond `BUG-01`..`BUG-15`)

| Swarm ID | Severity | Agent | Area | Summary | Root Cause |
| --- | --- | --- | --- | --- | --- |
| **SWARM-01** (`SM-01`) | **High** | Agent A | Outbox / Follow-Up Cancel | Cancelling a queued `follow_up` message inside the undo window leaves `target.follow_ups = 1` with no sent follow-up, permanently deadlocking the target (`follow_up` -> 409, `no-response` -> 409 forever, agent `close unable_to_confirm` -> 409). | `backend/internal/services/enq_outbox.go:120-128` (`CancelMessage` only updates target state for `OutboxKindEnquiry` and never decrements `t.FollowUps` when `m.Kind == OutboxKindFollowUp`). |
| **SWARM-02** (`SM-02` / `SM-07`) | **High** | Agent A | Undo / Late Reply / Race | Undoing `messages_queued` after a pharmacy has already replied (or racing `Reply` vs. `Undo(messages_queued)`) overwrites the replied target to `cancelled`; undoing `marked_no_response` after retraction resurrects a `cancelled` target back to `sent`. | `backend/internal/services/enq_outbox.go:347-350, 515-527` (`saveTargetStatus` and `undoCancelMessages` do not check current `t.Status`) + `enq_replies.go:45-68` (`Reply` reads `t` before `GetEnquiryForUpdate`). |
| **SWARM-03** (`SM-03` / `BUG-C3`) | **High** | Agent A, C | Undo / Reopen | Closing or cancelling an agent enquiry and undoing `enquiry_closed` (`reopen`) loses `wake_trigger` and `next_wake_at`, permanently stranding the reopened agent enquiry. | `backend/internal/services/enq_enquiries.go:630-641` & `enq_outbox.go:351-355` (`reopen` inverse does not restore `wake_trigger` or wake the agent). |
| **SWARM-04** (`SM-04` / `SM-05`) | **High** | Agent A | Clarification & Targets State Machine | Calling `AddTargets` (agent) while `status == "awaiting_clarification"` overwrites `status` to `"awaiting_approval"` so `Clarify` fails with `409`; conversely, `Clarify` sets `status = "open"` even when targets are `"proposed"`, or `ApproveTargets` sets `status = "open"` even when targets are `"sent"`. | `backend/internal/services/enq_enquiries.go:513-519`, `enq_outbox.go:25-91`, `enq_replies.go:186-198, 272-279`. |
| **SWARM-05** (`SM-09`) | **Medium** | Agent A | Close vs. Outbox | Closing an enquiry with any outcome (`cancelled`, `unable_to_confirm`, `available`) leaves queued `outbox_messages` in the queue, blocks `CancelMessage`/`Undo` with `409`, and `DispatchDue` delivers them after `enquiry_closed`. | `backend/internal/services/enq_enquiries.go:546-645` (`CloseEnquiry` does not cancel pending queued messages) & `enq_outbox.go:380-405` (`DispatchDue` does not skip closed/cancelled enquiries). |
| **SWARM-06** (`SM-10`) | **Medium** | Agent A | Undo Window Expiry | `CancelMessage` and `Undo(messages_queued)` only check `m.Status == "queued"`, allowing cancellation up to 2s after `send_after` has expired (before the 2s `RunDispatcher` tick). | `backend/internal/services/enq_outbox.go:117-120, 506` (missing `!s.now().Before(m.SendAfter)` / `send_after` expiry check). |
| **SWARM-07** (`B-01` / `B-02` / `BUG-C10`) | **High** | Agent B, C | Cross-Pharmacy Access Control (Rule 5) | Target pharmacy whose message was cancelled inside the undo window (`status = "cancelled"`) or is only `proposed`/`approved`/queued-in-window can still read `GET /api/v1/enquiries/{id}`, `GET /api/v1/enquiries/{id}/stream`, and `GET /api/v1/enquiries?scope=incoming`. | `backend/internal/services/enq_enquiries.go:163-168`, `enq_stats.go:69-74`, and `backend/internal/queries/sql/list_incoming_enquiries.sql`. |
| **SWARM-08** (`B-05` / `BUG-C1`) | **Medium** | Agent A, B, C | Agent Runs & Tool Calls | `StartRun` (`POST /api/v1/agent/runs`) succeeds on `closed`/`cancelled` enquiries and allows concurrent duplicate runs; `RecordToolCall` (`POST /api/v1/agent/runs/{id}/tool-calls`) accepts tool calls on finished runs, whitespace-only `name`, and negative `duration_ms`. | `backend/internal/services/enq_agentapi.go:33-60, 114-141`. |
| **SWARM-09** (`B-06`) | **High** | Agent B | Integer Overflow (`500`) | 64-bit Go `int` values `> 2,147,483,647` (`math.MaxInt32`) in `Reply` (`quantity`), `FinishRun` (`input_tokens`, `output_tokens`, `steps`), and `RecordToolCall` (`duration_ms`) overflow PostgreSQL `INTEGER` (`INT4`) and return `500 internal_error`. | `backend/internal/services/enq_replies.go:82`, `enq_agentapi.go:82, 114`. |
| **SWARM-10** (`B-07`) | **High** | Agent B | Null Byte (`\u0000`) `500` | UTF-8 NUL bytes (`\u0000` in JSON or `%00` in URL paths) reach PostgreSQL and trigger SQLSTATE `22021` (`500 internal_error`) on `/api/v1/auth/login`, `/api/v1/enquiries`, etc. | `backend/internal/services/contracts.go:132` (`clean()` does not strip `\x00`) & `services/errors.go` (`FromStore` does not map `22021` / NUL bytes). |
| **SWARM-11** (`BUG-C5` / `BUG-C9`) | **Medium** | Agent C | Agent Scripted Policy & Runner | `scripted.py` `_choose_medication` drops strength from `raw_request` when `clarification_answer` only gives the medicine name; `runner.py` marks `scripted` runs `completed` even when a terminal tool returns an error dict. | `agent/scripted.py:119` & `agent/runner.py:207-229`. |

---

## Agent A Detailed Report — State Machine & Concurrency Race Fuzzing

### 1. Follow-Up Cancel Deadlock (`SWARM-01` / `SM-01`)
- **Reproduction (`curl` / Python against `:8114`):**
  ```python
  # 1. Create enquiry, add branch-achrafieh, send, wait until delivered ("sent")
  # 2. Queue follow-up:
  POST /api/v1/targets/{tid}/follow-up -> 200 OK (fu_msg.status="queued", target.follow_ups=1)
  # 3. Cancel follow-up inside undo window:
  POST /api/v1/messages/{fu_msg.id}/cancel -> 200 OK (fu_msg.status="cancelled", target.follow_ups=1)
  # 4. Wait > 5 * WAIT_UNIT:
  POST /api/v1/targets/{tid}/follow-up -> 409 conflict ("a follow-up was already sent...")
  POST /api/v1/targets/{tid}/no-response -> 409 conflict ("the follow-up was sent too recently...")
  ```
- **Root Cause:** `CancelMessage` (`backend/internal/services/enq_outbox.go:120-128`) only updates the target when `m.Kind == queries.OutboxKindEnquiry` and never decrements `t.FollowUps` when `m.Kind == queries.OutboxKindFollowUp`.

### 2. `Reply` vs. `Undo` & `Reply` vs. `FollowUp` Races (`SWARM-02` / `SM-02` / `SM-07` / `SM-11`)
- **Reproduction:**
  - Racing `POST /api/v1/targets/{tid}/reply` (`available`, `quantity=2`) against `POST /api/v1/events/{mq_ev_id}/undo` returns `200 OK` on **both** requests in 6/6 attempts because `Reply` reads `t` before acquiring `GetEnquiryForUpdate(ctx, t.EnquiryID)` (`enq_replies.go:45-52`), and `undoCancelMessages` (`enq_outbox.go:515-527`) overwrites `t.Status` to `cancelled` without checking whether the target has already replied.
  - Undoing `marked_no_response` (`enq_outbox.go:347-350`) calls `saveTargetStatus(ctx, q, inv.TargetID, "sent")` without checking `t.Status == "no_response"`, overwriting a `available` reply (`BUG-02`) or resurrecting a `cancelled` target (`SM-07`).

### 3. Clarification, Approval, and Send State Machine Transitions (`SWARM-04` / `SM-04` / `SM-05`)
- **Reproduction:**
  - `POST /api/v1/enquiries/{id}/clarification-request` sets `status = "awaiting_clarification"`. If `POST /api/v1/enquiries/{id}/send` (`BUG-05`) or agent `POST /api/v1/enquiries/{id}/targets` (`SM-04`) is called, `status` changes to `"waiting"` or `"awaiting_approval"`, causing `POST /api/v1/enquiries/{id}/clarify` to fail with `409 no clarification is pending` even though `clarification_question != ""` and `clarification_answer == ""`.

### 4. Close vs. Outbox & Reopen (`SWARM-03` / `SWARM-05` / `SWARM-06`)
- **Reproduction:**
  - Closing an enquiry while outbox messages are still `queued` leaves them in `outbox_messages`; `DispatchDue` (`enq_outbox.go:380-405`) subsequently delivers them and logs `message_sent` on a `closed`/`cancelled` enquiry.
  - Undoing `enquiry_closed` (`reopen`) on an agent enquiry restores `status` but leaves `wake_trigger = ""` and `next_wake_at = nil`, stranding the agent enquiry.
  - Calling `CancelMessage` or `Undo(messages_queued)` after `send_after` has passed (`now > send_after`) but before the 2s `RunDispatcher` tick still treats the message as `queued` inside the window instead of expired/sent.

---

## Agent B Detailed Report — Auth, Cross-Pharmacy Boundaries (IDOR), & Input Validation Audit

### 1. Unauthenticated & Agent-Credential Endpoint Sweep (`BUG-06` Neighborhood)
- All customer, prescription, refill, cart, and order routes in `backend/internal/handlers/pharmacy.go:17-31` are mounted without `h.auth(staffOnly, ...)`:
  - `GET /api/v1/customers`, `GET /api/v1/customers/{id}`, `PATCH /api/v1/customers/{id}`
  - `GET /api/v1/customers/{id}/prescriptions`, `POST /api/v1/customers/{id}/prescriptions`
  - `GET /api/v1/customers/{id}/refills`, `POST /api/v1/customers/{id}/refills`, `PATCH /api/v1/refills/{id}`
  - `POST /api/v1/carts`, `GET /api/v1/carts/{id}`, `POST /api/v1/carts/{id}/items`, `DELETE /api/v1/carts/{id}/items/{medication_id}`, `POST /api/v1/carts/{id}/checkout`
  - `GET /api/v1/customers/{id}/orders`, `GET /api/v1/orders/{id}`
- Both anonymous callers (`401 unauthorized` expected) and `Authorization: Agent dev-agent-token` (`403 agent_forbidden` expected per `CONTRACT.md` Rule 8) currently receive `200`/`201`.

### 2. Cross-Pharmacy IDOR & Access Control (`SWARM-07`)
- **Reproduction:**
  - When Pharmacy A adds Pharmacy B as a target (`proposed` or `approved`), or sends and cancels B's message inside the undo window (`cancelled`), Pharmacy B has never received a delivered message (`deliveredToTarget(msgs, t.ID) == false`), yet B receives `200 OK` on `GET /api/v1/enquiries/{id}`, `GET /api/v1/enquiries/{id}/stream`, and sees the enquiry in `GET /api/v1/enquiries?scope=incoming` (`list_incoming_enquiries.sql` checks `t.status NOT IN ('proposed','approved')`, which matches `'cancelled'` and queued `'sent'`).

### 3. Integer Overflow & Null Byte `500 internal_error` (`SWARM-09` & `SWARM-10`)
- **Reproduction:**
  - `POST /api/v1/targets/{id}/reply` with `{"kind": "available", "quantity": 3000000000}` -> `500 internal_error`.
  - `PATCH /api/v1/agent/runs/{id}` with `{"status": "completed", "input_tokens": 3000000000, "output_tokens": 1, "steps": 1, "error": ""}` -> `500 internal_error`.
  - `POST /api/v1/agent/runs/{id}/tool-calls` with `{"name": "t", "input": {}, "output": "", "is_error": false, "duration_ms": 3000000000}` -> `500 internal_error`.
  - `POST /api/v1/auth/login` with `{"email": "a\u0000@wassit.demo", "pin": "1234"}` -> `500 internal_error`.

---

## Agent C Detailed Report — Agent Runner, Tools, Wakeup Queue & SSE Verifier

### 1. Wakeup Queue & `StartRun` Validation (`BUG-08` & `SWARM-08`)
- `StartRun` (`backend/internal/services/enq_agentapi.go:33-60`) does not validate `trigger` (`created`, `clarified`, `approved`, `reply`, `timer`) or non-empty `model`, does not reject `isFinal(e)` (`closed`/`cancelled` enquiries), and does not lock the enquiry row in `InTx`.
- `FinishRun` (`backend/internal/services/enq_agentapi.go:78-101`) overwrites already-finished runs (`status != "running"`, `BUG-04`).

### 2. Agent Tools, Scripted Policy, & Runner (`BUG-09`..`BUG-12` & `SWARM-11`)
- `agent/scripted.py`:
  - `_strengths()` (`BUG-09`) fails to divide `mcg` by `1000`, strips `x 0` from `x 0.5g` via `_QUANTITY_PATTERNS`, and matches bare numbers without units (`2 3elab`).
  - `_choose_medication()` (`SWARM-11` / `BUG-C5`) uses `wanted = _strengths(answer if answer else raw)` instead of `_strengths(answer) or _strengths(raw)`, dropping the strength from `raw_request` when `clarification_answer` only specifies the medication name.
- `agent/runner.py`:
  - `make_tracer()` (`BUG-11`) only checks `'"error"' in text`, missing Cornilia's `System_Error:` prefix when a terminal tool fails argument validation.
  - `run_scripted()` (`SWARM-11` / `BUG-C9`) records a run as `completed` even if the terminal tool call returned an error dict `{"error": ...}`.
- `agent/tools/_http.py`, `catalogue.py`, `enquiry.py`:
  - `_http.call()` (`BUG-10`) only catches `HTTPError` and `URLError`, letting `TimeoutError` / `ConnectionError` / `OSError` escape.
  - `search_medications()`, `find_candidates()`, `propose_targets()`, and `_http.tool_error()` (`BUG-12`) can return payloads `>= 2,000` characters when catalogue/branch fields or backend error messages are long.
