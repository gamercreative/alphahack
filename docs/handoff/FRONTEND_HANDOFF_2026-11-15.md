# Frontend handoff: building against the Wassit backend (2026-11-15)

For: the frontend engineer. Questions: akram (repo owner, GitHub `gamercreative`).

Every shape below was captured from a real local run of the backend (fake model, seed data) on
2026-11-15, with both open pull requests applied. The full machine contract is
`backend/openapi.yaml` (OpenAPI 3.0.3); this guide is the human map of it.

## 1. TL;DR

- **Nothing you've built breaks.** Neither recent PR changed a route, a request or response schema, auth,
  CORS, env var names or `openapi.yaml` (PR #1, PR #2 diffs; see section 8).
- Three new **409** error messages can now reach a UI: `safety_data_unreadable:` on cart add/checkout,
  and two agent-only refusals that change how agent-run enquiries move through their statuses (section 8).
- Everything you need to build the staff app is in sections 2 to 7: run it, log in, the enquiry
  lifecycle, every endpoint with real JSON, live updates over SSE, and error handling.

Terms used below:
- **Wassit**: the product. Pharmacists ask other pharmacies whether a medicine is in stock.
- **Enquiry**: one such question ("need Augmentin 1g x2"), created by the **requester** pharmacy.
- **Target**: one pharmacy contacted for an enquiry. Each has its own status.
- **Agent**: the AI assistant that can run an enquiry for the requester (`handled_by: "agent"`).
  It uses the same API with its own credential. A human still approves targets and answers questions.
- **Outbox / undo window**: messages to pharmacies are queued and only go out after
  `UNDO_WINDOW` (default 60s), so they can be cancelled first.
- **Event**: every state change writes an event row. The detail view and the live stream are built from them.
- **HITL count**: "human in the loop" count, the number of times a human had to step in on an enquiry.

## 2. Run the backend locally

Requirements: Docker, Go 1.25. No model is needed for UI work: `AI_PROVIDER=fake` is the default
when no Anthropic key is set (`backend/.env.example:14`).

```bash
cd backend
cp -n .env.example .env
make up          # Postgres 16 in Docker on :5432
make migrate     # schema + seed data (migrations 0003 and 0005 hold the seed)
UNDO_WINDOW=3s WAIT_UNIT=2s AGENT_TOKEN=dev-agent-token make run   # API on :8080
curl localhost:8080/healthz    # {"status":"ok"}
```

(Commands from `docs/DEMO.md:21` after PR #2; it was `DEMO.md` at the root before.) The short
`UNDO_WINDOW` makes queued messages go out after 3s instead of 60s, which is easier for UI work.
All-in-Docker alternative: `make docker-up` (`backend/Makefile:13`).

To watch the agent drive enquiries while you build, run the agent in a second terminal
(`docs/DEMO.md:22`) or drive a scripted demo with `python3.13 scripts/demo_flow.py --drive-agent`
(`docs/DEMO.md:28`). Ask akram if you need it; it needs a model.

**Seed logins** (PIN `1234` for all, `backend/db/migrations/0005_enquiry_seed.up.sql:1-2`, `docs/DEMO.md:78-86`):

| Email | Pharmacy (branch id) |
| --- | --- |
| `a@wassit.demo` | Hamra (`branch-hamra`), the usual requester |
| `b@wassit.demo` | Achrafieh (`branch-achrafieh`) |
| `c@wassit.demo` | Baabda (`branch-baabda`) |
| `d@wassit.demo` | Verdun (`branch-verdun`) |
| `e@wassit.demo` | Byblos (`branch-byblos`) |

Log in as two pharmacies in two browser profiles to see both sides of an enquiry.

**Env vars that matter to you** (names only; `backend/.env.example`):
`PORT` (default 8080), `CORS_ALLOWED_ORIGINS` (default `*`), `UNDO_WINDOW` (outbox delay),
`WAIT_UNIT` (length of one agent "wait minute"), `AGENT_TOKEN` (agent credential; empty disables
agent routes), `AGENT_PRICE_IN_PER_MTOK` / `AGENT_PRICE_OUT_PER_MTOK` (without them, `cost_micros`
in stats is always 0).

## 3. Conventions (all routes)

- Base URL `http://localhost:8080`. JSON is snake_case. Ids are strings (UUIDs, or readable ids for seed rows such as `branch-hamra`).
- Timestamps are RFC 3339 with offset. Nullable fields come back as `null`, never missing.
- Money is integer cents (`unit_price_cents`, `subtotal_cents`, `total_cents`). Agent cost is
  `cost_micros` (millionths of a US dollar).
- Lists: `{"data": [...], "limit": 50, "offset": 0}`; `?limit=` 1 to 200 (default 50), `?offset=`
  (`openapi.yaml:1297-1304`). Exceptions: `candidates` and `agent/wakeups` return `{"data": [...]}` only;
  `stats` returns `{"data": [...], "totals": {...}}`.
- Create returns **201**, delete returns **204**, everything else **200**.
- **Errors** are always `{"error": {"code", "message", "request_id"}}` (`backend/internal/handlers/handlers.go:138-160`).
  Branch on `code`; show `message` (it is written to be read by a person). Codes and statuses
  (`backend/internal/services/contracts.go:28-37`, `handlers.go:162-179`):

  | code | status | typical cause |
  | --- | --- | --- |
  | `validation_failed` | 400 | bad body or query, e.g. "raw_request must be 1 to 1000 characters" |
  | `unauthorized` | 401 | missing, bad or expired token |
  | `forbidden` | 403 | right login, wrong pharmacy (e.g. replying for another branch) |
  | `agent_forbidden` | 403 | agent credential on a human-only route |
  | `not_found` | 404 | unknown id or route |
  | `conflict` | 409 | business rule refused it; the message says why and what to do |
  | `upstream_error` | 502 | model provider failed (chat routes only) |
  | `internal_error` | 500 | bug |

- **CORS** (verified with a preflight): `Access-Control-Allow-Origin: *` by default,
  allowed headers `Content-Type, Authorization, X-Request-ID`, methods `GET, POST, PATCH, DELETE, OPTIONS`,
  exposed header `X-Request-ID` (`backend/internal/http/httpx.go:174-205`, `helpers.go:13`).

## 4. Auth

- `POST /api/v1/auth/login` `{email, pin}` → 200 `{token, staff, branch}`. Wrong PIN → 401
  `unauthorized` "wrong email or PIN".
- Send `Authorization: Bearer <token>` on every `/api/v1/...` staff route. Tokens last **12 hours**
  (CONTRACT.md:74). On any 401, send the user back to login. There is no refresh and no logout endpoint.
- `GET /api/v1/me` → `{staff, branch}`. Use it on app load to restore a session.
- **SSE exception:** the browser `EventSource` can't set headers, so the live stream also accepts
  `?access_token=<token>` (`backend/internal/handlers/enquiry_common.go:31-33`). This fallback is not
  in `openapi.yaml`.
- The pharmacy routes (`branches`, `medications`, `customers`, `carts`, `orders`, `refills`) and the
  generic `/v1/...` chat routes need **no auth** today (`backend/internal/handlers/pharmacy.go:12-31`).
  See section 11.

Login response (token redacted):

```json
{"token": "<64 hex chars>",
 "staff": {"id": "staff-hamra", "branch_id": "branch-hamra", "full_name": "Pharmacist Hamra (A)",
           "email": "a@wassit.demo", "role": "pharmacist", "created_at": "..."},
 "branch": {"id": "branch-hamra", "name": "Cornilia Pharmacy Hamra", "city": "Beirut - Hamra",
            "address": "...", "phone": "+961 1 350 456", "supports_delivery": true, "created_at": "..."}}
```

## 5. The enquiry lifecycle (what screens need to show)

**Enquiry `status`** (`openapi.yaml`, enquiries list `status` enum; CONTRACT.md:44):
`open` → (`awaiting_clarification` ↔ `open`) → `awaiting_approval` → `waiting` → `closed` | `cancelled`.

| status | meaning | who acts next |
| --- | --- | --- |
| `open` | created, or the clarification was answered | requester, or the agent |
| `awaiting_clarification` | the agent asked the requester a question (`clarification_question`) | **requester** answers |
| `awaiting_approval` | the agent proposed pharmacies to contact | **requester** approves |
| `waiting` | messages sent, waiting for replies or for the agent's next wake-up (`next_wake_at`) | target pharmacies |
| `closed` | finished with an `outcome` | none |
| `cancelled` | cancelled | none |

**`outcome`** (set when closed): `available` (with `outcome_branch_id`), `unavailable`,
`unable_to_confirm`, `cancelled`; `""` while open. Closing rules are enforced by the backend
(CONTRACT.md:18-19): `available` needs at least one target that replied available; `unavailable`
needs every target to have replied unavailable; otherwise it's `unable_to_confirm`. A wrong close
returns 409 with an explanation, which is safe to show as is.

**Target `status`**: `proposed` (agent suggested) → `approved` → `sent` → `available` |
`unavailable` | `alternative_offered` | `no_response` | `cancelled`. "No reply" is `no_response`,
never `unavailable`. An `alternative_offered` reply (e.g. "625mg only") is an option for the
requester and never counts as a match (CONTRACT.md:9-13; the `alternative` reply kind).

**Outbox message** (`messages[]` in the detail): `kind` `enquiry | follow_up | retraction | notification`,
`status` `queued → sent | cancelled`, `send_after` (when it will go out). While `queued`, show a
Cancel button with a countdown to `send_after`. `notification` messages (no `target_id`) are notes
from the agent to the requester, e.g. "C has 625mg only".

**Manual flow (requester A, target B), as verified:**
1. `POST /api/v1/enquiries` `{raw_request, medication_id?, quantity, handled_by: "manual"}` → 201 enquiry.
2. `GET /api/v1/medications?q=augmentin` to pick the medicine; `GET /api/v1/enquiries/{id}/candidates?limit=5`
   for ranked nearby pharmacies.
3. `POST /api/v1/enquiries/{id}/targets` `{branch_ids: [...]}` → 201, targets are `approved` straight away for staff.
4. `POST /api/v1/enquiries/{id}/send` → one `queued` message per target; enquiry → `waiting`.
   Optional: `POST /api/v1/messages/{id}/cancel` inside the window (the target never sees it).
5. B: `GET /api/v1/inbox` → B's waiting requests; `POST /api/v1/targets/{target_id}/reply`
   `{kind: "available" | "unavailable" | "alternative", quantity?, text?}`.
6. A: `POST /api/v1/enquiries/{id}/close` `{outcome: "available", branch_id: "branch-achrafieh"}`.

**Agent flow (`handled_by: "agent"`):** A creates the enquiry and then only does the human steps:
- Answers questions: when `status` is `awaiting_clarification`, show `clarification_question` and
  send `POST /api/v1/enquiries/{id}/clarify` `{answer}`.
- Approves pharmacies: when `status` is `awaiting_approval`, show targets with `status: "proposed"` and
  send `POST /api/v1/enquiries/{id}/targets/approve` `{target_ids: [...]}`.
- Watches progress through the live stream (section 7). Staff can still cancel, undo or close.

**Undo:** events with a non-null `inverse` can be undone with `POST /api/v1/events/{id}/undo` → 200
with a new `event_undone` event. A second undo → 409 "this event was already undone".
Undoing `messages_queued` after sending queues a retraction and cancels the target (CONTRACT.md:67, 152).

## 6. Endpoint reference

Who: **S** = staff Bearer token, **A** = agent token, **–** = no auth. Body fields with `?` are optional.
All verified on the local run unless marked.

### Staff app (Wassit)

| Method + path | Who | Body / query | Returns |
| --- | --- | --- | --- |
| POST `/api/v1/auth/login` | – | `{email, pin}` | `{token, staff, branch}` |
| GET `/api/v1/me` | S | | `{staff, branch}` |
| GET `/api/v1/enquiries` | S | `?scope=outgoing\|incoming&status=<status>` + limit/offset | list of Enquiry |
| POST `/api/v1/enquiries` | S | `{raw_request (1-1000 chars), medication_id?, quantity ≥1, handled_by: manual\|agent}` | 201 Enquiry |
| GET `/api/v1/enquiries/{id}` | S, A | | EnquiryDetail (below) |
| POST `/api/v1/enquiries/{id}/medication` | S, A | `{medication_id}` | Enquiry |
| POST `/api/v1/enquiries/{id}/clarify` | S (requester) | `{answer}` | Enquiry, status back to `open` |
| GET `/api/v1/enquiries/{id}/candidates` | S, A | `?limit=5&radius_km=15` | `{data: [Candidate]}` |
| POST `/api/v1/enquiries/{id}/targets` | S, A | `{branch_ids: [...]}` | 201 `{data: [Target]}` |
| POST `/api/v1/enquiries/{id}/targets/approve` | S (requester) | `{target_ids: [...]}` | `{data: [Target]}` |
| POST `/api/v1/enquiries/{id}/send` | S, A | none | `{data: [OutboxMessage]}` |
| POST `/api/v1/messages/{id}/cancel` | S, A | none | OutboxMessage (409 if no longer queued) |
| POST `/api/v1/targets/{id}/follow-up` | S, A | none | max one per target, only when `sent`, else 409 |
| POST `/api/v1/targets/{id}/no-response` | S, A | none | only when `sent` and after one follow-up, else 409 |
| GET `/api/v1/inbox` | S | limit/offset | list of InboxItem (requests waiting on my pharmacy) |
| POST `/api/v1/targets/{id}/reply` | S (target pharmacy only) | `{kind, quantity?, text?}` | Target |
| POST `/api/v1/enquiries/{id}/notify` | S, A | `{body}` | OutboxMessage (`kind: notification`) |
| POST `/api/v1/enquiries/{id}/close` | S, A | `{outcome, branch_id?}` | Enquiry |
| POST `/api/v1/events/{id}/undo` | S, A | none | Event (`event_undone`) |
| GET `/api/v1/enquiries/{id}/stream` | S | `?access_token=` allowed | SSE, see section 7 |
| GET `/api/v1/stats` | S | | `{data: [per enquiry], totals}` |

Agent-only routes (the agent process calls these; a staff token gets 403 `forbidden`; you don't need them):
`POST /enquiries/{id}/clarification-request`, `POST /enquiries/{id}/wait`, `GET /agent/wakeups`,
`POST /agent/runs`, `PATCH /agent/runs/{id}`, `POST /agent/runs/{id}/tool-calls`
(`backend/internal/handlers/enq_replies.go:14`, `enq_agentapi.go:12-16`).

**EnquiryDetail** (trimmed to one item per list):

```json
{"enquiry": {"id": "a4f3…", "requester_branch_id": "branch-hamra", "requester_staff_id": "staff-hamra",
   "raw_request": "need augmentin", "medication_id": "med-augmentin-1g", "quantity": 2,
   "status": "waiting", "outcome": "", "outcome_branch_id": null, "handled_by": "agent",
   "clarification_question": "Which strength?", "clarification_answer": "1g tablets",
   "wake_trigger": "approved", "next_wake_at": "…", "created_at": "…", "updated_at": "…", "closed_at": null},
 "medication": {"id": "med-augmentin-1g", "brand_name": "Augmentin", "active_ingredient": "Amoxicillin/Clavulanate",
   "dosage_form": "tablet", "strength": "1000mg"},
 "targets": [{"id": "349c…", "enquiry_id": "a4f3…", "branch_id": "branch-baabda", "branch_name": "Cornilia Pharmacy Baabda",
   "status": "sent", "reply_quantity": null, "reply_text": "", "replied_by_staff_id": null,
   "replied_at": null, "follow_ups": 0, "created_at": "…"}],
 "messages": [{"id": "46eb…", "enquiry_id": "a4f3…", "target_id": "349c…", "kind": "enquiry",
   "body": "Availability enquiry from Cornilia Pharmacy Hamra: do you have Augmentin 1000mg tablet x2? …",
   "status": "queued", "send_after": "…", "sent_at": null, "created_by_actor": "agent", "created_at": "…"}],
 "events": [{"id": "a75f…", "seq": 8, "enquiry_id": "a4f3…", "actor_type": "staff", "actor_id": "staff-hamra",
   "type": "enquiry_created", "payload": {"quantity": 2, "handled_by": "agent", "raw_request": "need augmentin",
   "medication_id": null}, "inverse": null, "undone_at": null, "undone_by_event_id": null, "created_at": "…"}],
 "runs": [],
 "hitl_count": 2}
```

`medication` is `null` until one is set. `actor_type` is `staff | agent | system`; use it to label
timeline entries ("Wassit agent", a pharmacist, or the system). Event `type`s (CONTRACT.md:64-69):
`enquiry_created`, `medication_set`, `clarification_requested`, `clarified`, `targets_proposed`,
`targets_approved`, `messages_queued`, `message_cancelled`, `message_sent`, `follow_up_queued`,
`marked_no_response`, `reply_received`, `requester_notified`, `wait_set`, `enquiry_closed`, `event_undone`.

**InboxItem:** `{target_id, target_status, follow_ups, enquiry_id, requester_branch_id,
requester_branch_name, medication_id, medication_name, raw_request, quantity, sent_at}`.

**Candidate:** `{branch_id, name, city, distance_km, reported_quantity, has_reported_stock, stock_updated_at}`.

**Stats row:** `{enquiry_id, raw_request, handled_by, status, outcome, hitl_count, agent_runs,
agent_steps, input_tokens, output_tokens, cost_micros, created_at}`; `totals`:
`{enquiries, agent_enquiries, hitl_count, agent_runs, agent_steps, input_tokens, output_tokens, cost_micros}`.

### Pharmacy catalogue and retail (no auth)

| Method + path | Body / query | Returns |
| --- | --- | --- |
| GET `/api/v1/branches`, `/api/v1/branches/{id}` | | Branch `{id, name, city, address, phone, supports_delivery, created_at}` |
| GET `/api/v1/branches/{id}/inventory` | | batches, soonest expiry first |
| GET `/api/v1/medications` | `?q=` search + limit/offset | Medication `{id, brand_name, active_ingredient, therapeutic_class, dosage_form, strength, rx_required, controlled, chronic_eligible, unit_price_cents, contraindication_tags, description, created_at}` |
| GET `/api/v1/medications/{id}` | | `{medication, branch_availability: [{branch_id, branch_name, city, available_quantity, earliest_expiry, …}]}` |
| GET/PATCH `/api/v1/customers[/{id}]` | | Customer `{id, full_name, phone, email, address, allergies[], chronic_conditions[], created_at}` |
| GET/POST `/api/v1/customers/{id}/prescriptions` | | new prescriptions start `pending` |
| GET/POST `/api/v1/customers/{id}/refills`, PATCH `/api/v1/refills/{id}` | | chronic refill subscriptions |
| POST `/api/v1/carts` | `{customer_id, branch_id}` | 201 `{cart, items, subtotal_cents}` (creates or loads the active cart) |
| GET `/api/v1/carts/{id}` | | same CartDetail |
| POST `/api/v1/carts/{id}/items` | `{medication_id, quantity ≥1, prescription_id?}` | CartDetail; 409 on safety or stock rules |
| DELETE `/api/v1/carts/{id}/items/{medication_id}` | | CartDetail |
| POST `/api/v1/carts/{id}/checkout` | `{fulfillment_type: pickup\|delivery, delivery_address?}` | 201 `{order, items}` |
| GET `/api/v1/customers/{id}/orders`, `/api/v1/orders/{id}` | | orders with batch-allocated lines |

Cart 409 messages start with a machine-readable prefix, then a colon: `rx_required:`,
`controlled_substance:`, `contraindication_detected:`, `insufficient_stock:`, and now
`safety_data_unreadable:` (section 8). Show the text after the prefix, or map the prefix to your own
copy. Also seen: `cart is empty`.

### Generic agent chat (`/v1`, no auth)

From the original backend template: agents, conversations, messages and runs. Use it only if you
build a free-form chat screen. `POST /v1/agents {name, system_prompt?, model?}`,
`POST /v1/conversations {agent_id, title?}`, `POST /v1/conversations/{id}/messages {content}` →
201 `{user_message, assistant_message, run, tool_calls}`; `…/messages/stream` is the same turn as SSE
with events `start`, `delta`, `thinking`, `tool_call`, `tool_result`, `done`, `error`
(`openapi.yaml:199-219`; the streaming route was not run). List, get and delete routes are in `openapi.yaml:26-282`.

## 7. Live updates (SSE)

```js
const es = new EventSource(`${API}/api/v1/enquiries/${id}/stream?access_token=${token}`);
es.addEventListener("enquiry_event", (e) => applyEvent(JSON.parse(e.data)));
```

- One event named `enquiry_event` per event row; `data` is the same Event object as in `events[]`.
  Verified: the stream starts by sending the enquiry's existing events (from `seq` 8 on, in order),
  then new ones as they happen.
- Dedupe by `id` (or keep the highest `seq`) when you merge with the detail you already fetched.
- No token → 401; agent token → 403 `agent_forbidden` (`openapi.yaml:1160-1162`).
- The simplest correct UI: on each event, refetch `GET /api/v1/enquiries/{id}`. Events tell you
  *that* something changed; the detail gives you the current state.

## 8. What changed in the last two PRs (and what it means for the UI)

Both PRs are **open, not merged** (section 14). Neither adds, removes or renames an endpoint,
field, header or env var, and `openapi.yaml` is untouched (commit list, section 14).

**Visible behaviour changes (non-breaking):**

| Endpoint | Change | Before | After | Source |
| --- | --- | --- | --- | --- |
| POST `/api/v1/carts/{id}/items`, POST `/api/v1/carts/{id}/checkout` | Unreadable medication or customer tag data blocks the item | 200/201, sale went through as if there were no contraindications | **409** `conflict`, message `safety_data_unreadable: medication contraindication tags are not a list of strings` (or `…customer allergy or condition tags…`) | PR #1, 2f964e6, `backend/internal/services/pharmacy.go:478-497` |
| POST `/api/v1/enquiries/{id}/close` with `unable_to_confirm` (agent token only) | Agent can't give up while a contacted pharmacy is still pending (`proposed`, `approved` or `sent`) | 200, closed | **409** `conflict` "cannot close yet: a contacted pharmacy has not answered…" Staff can still close early. | bb7229d (in PR #2), `backend/internal/services/enq_enquiries.go:610-618` |
| POST `/api/v1/enquiries/{id}/clarification-request` (agent token only) | Agent can't ask a second question while one is waiting | 200, question overwritten | **409** `conflict` "a question is already waiting for the requester…" | bb7229d (in PR #2), `backend/internal/services/enq_replies.go:141-143` |

What this means for you:
- The safety 409 only happens with corrupt catalogue or customer data. If your cart UI already shows
  409 messages, nothing to do; the item just can't be sold until the data is fixed.
- The two agent 409s are seen by the agent, not by your UI. What you'll notice: an agent enquiry
  stays `waiting` longer, through a follow-up and `no_response` on silent pharmacies before it closes,
  and the clarification question shown to the requester no longer gets replaced mid-question.

**Not visible through the API, listed so you know it was checked:**
- A chat turn on `/v1/conversations/{id}/messages` now gives up after 30 s if the database hangs
  while saving, instead of hanging forever (PR #1, af2b140, `backend/internal/services/chat.go:20-24`). Not run (see section 13).

## 9. Repo and dev-setup changes (PR #2)

Only docs moved. Code folders, `backend/openapi.yaml` and the root `CONTRACT.md` stay where they are.

| Old path | New path |
| --- | --- |
| `PLAN.md`, `WORKFLOW.md`, `DEMO.md` | `docs/PLAN.md`, `docs/WORKFLOW.md`, `docs/DEMO.md` |
| `LEAD_PROMPT_*.md` | `docs/lead-prompts/` |
| `documents/CONTRACT.md` (old draft) | `docs/archive/CONTRACT-draft-pre-W2.md`. Use the root `CONTRACT.md`, not this one |
| `documents/Shelf Guard vs. …md` | `docs/research/shelf-guard-vs-farmanco-evidence-check-2026-11.md` |

New: a root `README.md` and `docs/STRUCTURE.md` ("where new files go"). A `web/` folder for the
frontend is planned in `docs/PLAN.md:18` but does not exist yet. Run commands, ports
(API 8080, Postgres 5432) and seed data are unchanged.

## 10. Safe to ignore

- The cornilia-guard gateway fixes (422 `idempotency_key_reused`, client JSON checks; PR #1, 03900a9, e3878ae):
  that gateway is a separate demo on port 8088 and nothing in the backend or agent uses it yet.
- Outbox dispatcher error logging (PR #1, af5dfcf) is server logs only.
- New tests in PR #1 (enquiry auth matrix, staff login and session expiry queries) and PR #2's layout smoke tests.
- Agent-side changes in bb7229d (`agent/`): how the AI searches the catalogue and ends a run.

## 11. Known issues and deferred items that touch the frontend

From PR #1, section "Deferred":
- **Pharmacy and `/v1` routes have no auth**, including customer data, prescriptions and checkout.
  Adding auth would change the public API. If it lands, expect `Authorization: Bearer` on those
  routes plus updates to `openapi.yaml`. Not decided.
- **Login has no rate limit** and PINs are short; expect a lockout error on login later.
- **The staff token is accepted as `?access_token=`** (needed for EventSource). Planned replacement:
  short-lived, stream-only tokens, which would change how you open the stream.

Not documented in `openapi.yaml` today: the `access_token` query fallback, and CORS.

## 12. Open questions for you

1. Which screens are you building: staff Wassit app only, or the retail pharmacy (carts, orders) too?
   That decides whether the "no auth on pharmacy routes" item matters to you now.
2. Do you want typed clients generated from `backend/openapi.yaml`? It's OpenAPI 3.0.3 and covers every route.
3. Live updates: will you use `EventSource` (needs `?access_token=`) or a fetch-based SSE client
   that can send the `Authorization` header?
4. What dev origin do you use (e.g. `localhost:3000`)? CORS allows `*` today; a fixed list would need
   `CORS_ALLOWED_ORIGINS`.

## 13. Unverified

- The 30 s chat save timeout (section 8) was not reproduced at runtime: it needs a database that
  hangs mid-request. It is covered by `backend/internal/services/chat_persist_test.go` (PR #1).
- `POST /v1/conversations/{id}/messages/stream` event names come from `openapi.yaml:199-219`; that route was not run.
- `follow-up` and `no-response` 409 rules come from CONTRACT.md:100-101 and `openapi.yaml`; they were not run.

## 14. Appendix

**PRs** (both open as of 2026-11-15):
- PR #1 https://github.com/gamercreative/alphahack/pull/1, branch `quality/2026-11-14-audit`:
  b4ea6a0 test: cover the enquiry route auth matrix at the handler level ·
  e3878ae fix(guard): raise GatewayError for non-JSON or non-object 2xx bodies ·
  2f964e6 fix(pharmacy): fail closed when safety tag data cannot be read ·
  03900a9 fix(agentgw): reject an Idempotency-Key reused with different arguments ·
  af5dfcf fix(enquiry): log outbox dispatcher errors instead of dropping them ·
  c8cf1fe test: integration-test staff login and session expiry queries ·
  af2b140 fix(chat): bound the post-model writes with a timeout
- PR #2 https://github.com/gamercreative/alphahack/pull/2, branch `structure/2026-11-15-reorg`:
  bb7229d Agent guards for small local models (…agent cannot close while a pharmacy is pending) ·
  7e6a786 test: add layout smoke tests · bde88ed refactor(structure): move process docs into docs/ ·
  6386305 refactor(structure): group lead prompts under docs/lead-prompts/ ·
  a39935b refactor(structure): move documents/ into docs/research and docs/archive ·
  9c80f89 docs: add README and docs/STRUCTURE.md · ab92509 docs(structure): correct the cost line
  (bb7229d is not on GitHub's `main` yet, so it shows up in PR #2's diff.)

**How this was verified:** a fresh clone with both PR branches merged locally, its own Postgres
container on port 5544 (`COMPOSE_PROJECT_NAME=handoff DB_PORT=5544 make up && make migrate`) and the
API on port 8090 with `AI_PROVIDER=fake`, a test-only `AGENT_TOKEN`, `UNDO_WINDOW=2s`. A stdlib Python
script walked: login (good and bad PIN), `/me` with no token, a Bearer token and an agent token;
branches; medication search and detail; the full manual flow (create, candidates, targets, send,
cancel inside the window, inbox for B and for C, reply as agent/wrong branch/B, wrong close then
correct close, detail, list); the full agent flow (create, wakeups, clarification asked twice,
clarify by agent and by staff, propose, approve, send, early close by agent, wait, notify, undo
twice, run start/tool call/finish, stats); cart create, add item and checkout with a medication's
tag data corrupted on purpose, then restored and checked out (201); `/v1` agent, conversation and
chat message; an unknown route; then `curl -N` on the SSE stream with `?access_token=` and without a
token, and a CORS preflight `OPTIONS` from `Origin: http://localhost:3000`. Every status and shape
quoted in this guide came from that run, except the items in section 13.
