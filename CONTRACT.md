# CONTRACT: Wassit (DRAFT, waiting for the owner's OK; frozen at W2, then change only through the lead)

## Idea in one line
A pharmacist-to-pharmacist medicine availability enquiry system that works fully by hand, plus
Wassit, an agent that runs the same workflow through the same API with its own credential,
inside a guarded state machine (code decides what is allowed, the model decides what to do next).

## Product rules (reject any diff that breaks one)
1. The agent never diagnoses, prescribes, recommends or substitutes. An alternative offered by a
   pharmacy is shown to the requester as an option and never counts as a match.
2. Only a human pharmacist at the target branch can reply available/unavailable/alternative.
   Agent credentials get 403 `agent_forbidden` on reply, approve and clarify. No tool marks stock available.
3. No reply is `no_response`, never `unavailable`.
4. Every state change writes an `enquiry_events` row (actor_type staff | agent | system) and, where
   reversible, its inverse. Undo runs the stored inverse.
5. Outbound messages go through `outbox_messages` with `send_after = now + UNDO_WINDOW` (default 60s).
   Cancel inside the window means the target never sees it.
6. Closing rules live in Go: `available` needs >= 1 target `available`; `unavailable` needs every
   target replied `unavailable`; otherwise `unable_to_confirm` or `cancelled`.
7. Agent works only from DB catalogue and pharmacy data. Ambiguous request -> ask the requester.
8. Out of scope, never read by the agent: customers, carts, orders, prescriptions, refills,
   contraindication/allergy fields.

## Loop owner + model
- Cornilia-driven. The backend never calls Cornilia.
- `agent/runner.py` polls `GET /api/v1/agent/wakeups` every 3s, one enquiry at a time. Per wake:
  create run record -> Cornilia one-shot (system prompt + trigger + compact enquiry state) -> tools
  until `wait()` or `close_enquiry()` -> PATCH run with tokens, steps, status. Cost computed in Go.
- `CORNILIA_MAX_STEPS=12`. Hitting it: run `failed`, nothing closed, enquiry stays open for a human.
- Model `qwen3.5:35b-a3b` via Ollama, thinking off. Cornilia pinned at git tag `v0.1.0`.
- Token usage: Cornilia v0.1.0 drops `response.usage` (providers.py returns the ChatCompletion,
  models.py reads only `choices`). Decision pending (see "Open decisions").
- Wake triggers: `created` (handled_by=agent), `clarified`, `approved`, `reply`, `timer` (next_wake_at passed).

## Data (migration 0004_enquiries; plus db/schema.dbml + internal/queries/models.go)
Reuse: `branches` = participating pharmacies, `medications` = catalogue, `inventory_batches` = reported stock.
Ids are TEXT UUIDs like the template. Timestamps are timestamptz default now().

- `ALTER TABLE branches ADD lat double precision, ADD lng double precision, ADD participating boolean NOT NULL DEFAULT true`
  (existing pharmacy SQL selects explicit columns, so existing endpoints are unchanged)
- `staff`: id, branch_id FK branches, full_name, email UNIQUE, pin_hash (hex sha256 of pin, stdlib), role default 'pharmacist', created_at
- `staff_sessions`: token_hash PK (hex sha256 of token), staff_id FK, expires_at
- `enquiries`: id, requester_branch_id FK, requester_staff_id FK, raw_request, medication_id FK NULL,
  quantity int CHECK > 0, status CHECK (open|awaiting_clarification|awaiting_approval|waiting|closed|cancelled),
  outcome CHECK (''|available|unavailable|unable_to_confirm|cancelled) default '', outcome_branch_id FK NULL,
  handled_by CHECK (manual|agent), clarification_question default '', clarification_answer default '',
  next_wake_at NULL, created_at, updated_at, closed_at NULL
- `enquiry_targets`: id, enquiry_id FK, branch_id FK, UNIQUE(enquiry_id, branch_id),
  status CHECK (proposed|approved|sent|available|unavailable|alternative_offered|no_response|cancelled),
  reply_quantity int NULL, reply_text default '', replied_by_staff_id FK NULL, replied_at NULL, follow_ups int default 0
- `outbox_messages`: id, enquiry_id FK, target_id FK NULL, kind CHECK (enquiry|follow_up|retraction|notification),
  body, status CHECK (queued|sent|cancelled), send_after, sent_at NULL, created_by_actor (e.g. `staff:<id>`, `agent`)
- `enquiry_events`: id, seq bigserial UNIQUE, enquiry_id FK, actor_type CHECK (staff|agent|system), actor_id,
  type, payload jsonb, inverse jsonb NULL, undone_at NULL, undone_by_event_id NULL, created_at
- `enquiry_agent_runs`: id, enquiry_id FK, trigger, model, input_tokens int default 0, output_tokens int default 0,
  steps int default 0, cost_micros bigint default 0, status CHECK (running|completed|failed), error default '',
  started_at, finished_at NULL
- `enquiry_agent_tool_calls`: id, seq bigserial, run_id FK, name, input jsonb, output, is_error bool, duration_ms int, created_at

Seed (0005_enquiry_seed): lat/lng for every seeded branch (Beirut area), one staff per branch with demo
PIN `1234`, emails `pharmacist@<branch-slug>.demo` (Pharmacy A/B/C/D logins for the demo).

### Event types (payload / inverse)
`enquiry_created`, `medication_set` (inverse: previous medication_id), `clarification_requested`,
`clarified`, `targets_proposed` (inverse: delete proposed targets), `targets_approved`,
`messages_queued` (inverse: cancel queued messages; after sent: queue retraction + target cancelled),
`message_cancelled`, `message_sent` (system), `follow_up_queued`, `marked_no_response` (inverse: back to sent),
`reply_received`, `requester_notified`, `wait_set`, `enquiry_closed`, `event_undone`.
HITL count per enquiry = count of `clarified` + `targets_approved` events.

## Auth
- Staff: `POST /api/v1/auth/login {email, pin}` -> 200 `{token, staff, branch}`. 30 random bytes, hex;
  stored hashed; 12h expiry. `Authorization: Bearer <token>` on staff routes. Missing/bad -> 401 `unauthorized`.
- Agent: `Authorization: Agent <AGENT_TOKEN>` (env, constant-time compare). Same header shape as
  cornilia-guard's agentgw so the guard can replace it later.
- Middleware (handlers layer; services never import net/http) puts an `services.Actor{Type, ID, BranchID}` in
  the context. Routes are tagged staff-only, agent-ok or agent-only. Agent credential on a staff-only
  route -> 403 `agent_forbidden`. Staff token on an agent-only route -> 403 `forbidden`.
- New error codes in services/contracts.go: `unauthorized` (401), `forbidden` (403), `agent_forbidden` (403).

## Endpoints (JSON snake_case; errors `{"error":{"code","message","request_id"}}`)
Reused unchanged: `GET /api/v1/medications?q=`, `GET /api/v1/branches`, `GET /api/v1/branches/{id}/inventory`.

| Route | Who | Body -> result |
| --- | --- | --- |
| POST /api/v1/auth/login | public | `{email, pin}` -> `{token, staff, branch}` |
| GET /api/v1/me | staff | `{staff, branch}` |
| POST /api/v1/enquiries | staff | `{raw_request, medication_id?, quantity, handled_by}` -> 201 enquiry |
| GET /api/v1/enquiries?scope=outgoing\|incoming&status= | staff | list |
| GET /api/v1/enquiries/{id} | staff, agent | enquiry + targets + messages + events + run summaries |
| POST /api/v1/enquiries/{id}/medication | staff, agent | `{medication_id}` |
| POST /api/v1/enquiries/{id}/clarification-request | agent | `{question}` -> awaiting_clarification |
| POST /api/v1/enquiries/{id}/clarify | staff (requester) | `{answer}` -> open, wake `clarified` |
| GET /api/v1/enquiries/{id}/candidates?limit=5&radius_km=5 | staff, agent | ranked in Go: participating, not requester, has unexpired unrecalled stock >= qty preferred, haversine km, stock `updated_at` freshness |
| POST /api/v1/enquiries/{id}/targets | staff, agent | `{branch_ids}`; staff -> approved; agent -> proposed + awaiting_approval |
| POST /api/v1/enquiries/{id}/targets/approve | staff (requester) | `{target_ids}` -> approved, wake `approved` |
| POST /api/v1/enquiries/{id}/send | staff, agent | one queued outbox msg per approved target; targets -> sent; status waiting |
| POST /api/v1/messages/{id}/cancel | staff, agent | only while queued, else 409 |
| POST /api/v1/targets/{id}/follow-up | staff, agent | max 1 per target, only if sent, else 409 |
| POST /api/v1/targets/{id}/no-response | staff, agent | only if sent and follow_ups = 1, else 409 |
| GET /api/v1/inbox | staff | targets for my branch whose enquiry message is sent |
| POST /api/v1/targets/{id}/reply | staff (target branch only) | `{kind: available\|unavailable\|alternative, quantity?, text?}`, wake `reply` |
| POST /api/v1/enquiries/{id}/notify | staff, agent | `{body}` -> notification outbox msg to requester |
| POST /api/v1/enquiries/{id}/wait | agent | `{minutes: 5\|15\|30}` -> next_wake_at, status waiting |
| POST /api/v1/enquiries/{id}/close | staff, agent | `{outcome, branch_id?}`; rule 6 -> 409 `conflict` with a teaching message |
| POST /api/v1/events/{id}/undo | staff, agent | runs inverse; 409 if not reversible or already undone |
| GET /api/v1/enquiries/{id}/stream | staff | SSE (`httpx.NewSSE`), event `enquiry_event` per new row |
| GET /api/v1/agent/wakeups | agent | `{data:[{enquiry_id, trigger}]}` handled_by=agent, oldest first |
| POST /api/v1/agent/runs | agent | `{enquiry_id, trigger, model}` -> 201 run |
| PATCH /api/v1/agent/runs/{id} | agent | `{status, input_tokens, output_tokens, steps, error}`; Go sets cost_micros |
| POST /api/v1/agent/runs/{id}/tool-calls | agent | `{name, input, output, is_error, duration_ms}` |
| GET /api/v1/stats | staff | per enquiry: outcome, hitl_count, agent_steps, cost_micros; totals |

Background (goroutine started in main.go, 2s ticker): outbox dispatcher marks queued -> sent when
send_after passed, logs a `message_sent` system event. Env: `UNDO_WINDOW=60s`, `AGENT_TOKEN`,
`AGENT_PRICE_IN_PER_MTOK`, `AGENT_PRICE_OUT_PER_MTOK` (USD per million tokens; cost_micros =
tokens x price, in micro-dollars).

## Agent tools (agent/tools/, Cornilia @tool, results < 2k chars, errors that teach)
All HTTP goes through `agent/tools/_http.py` (`call(method, path, body=None) -> dict`, adds the Agent
header, returns the backend's error body on 4xx so the model learns). The guard replaces this file later.

| Tool | Endpoint |
| --- | --- |
| get_enquiry(enquiry_id) | GET /enquiries/{id} (compacted) |
| search_medications(query, form: Literal[...] \| None = None) -> top 5 | GET /medications?q= |
| set_medication(enquiry_id, medication_id) | POST /enquiries/{id}/medication |
| ask_requester(enquiry_id, question) | POST /enquiries/{id}/clarification-request |
| find_candidates(enquiry_id, limit: int = 5) | GET /enquiries/{id}/candidates |
| propose_targets(enquiry_id, branch_ids) | POST /enquiries/{id}/targets |
| send_enquiries(enquiry_id) | POST /enquiries/{id}/send |
| follow_up(target_id) | POST /targets/{id}/follow-up |
| mark_no_response(target_id) | POST /targets/{id}/no-response |
| notify_requester(enquiry_id, message) | POST /enquiries/{id}/notify |
| wait(enquiry_id, minutes: Literal[5, 15, 30]) | POST /enquiries/{id}/wait |
| close_enquiry(enquiry_id, outcome: Literal["available","unavailable","unable_to_confirm"], branch_id: str \| None = None) | POST /enquiries/{id}/close |

System prompt (`agent/prompts/system.md`): Wassit is an admin coordinator for pharmacists, not a
clinician; rules 1, 3, 7 in plain words; replies may be Arabic, English, French or Arabizi; an
alternative strength/form/brand is reported with notify_requester and is never a match; when unsure,
ask the requester; end every run with wait() or close_enquiry().

## Acceptance checks
1. Manual: A logs in, creates enquiry, targets B and C, sends; A cancels C's message inside the window,
   C's inbox stays empty; B replies available qty 2; A closes available at B.
2. Agent: A creates handled_by=agent "need augmentin 1g tabs x2". Agent matches medicine, proposes, waits;
   A approves; agent sends; B unavailable; C alternative "625mg only"; D silent. Agent notifies A about C,
   follows up D once, marks D no_response, closes unable_to_confirm. Never available.
3. Agent credential on reply, approve, clarify -> 403 agent_forbidden (one test per route).
4. Close available with no available target -> 409; close unavailable with a no_response target -> 409.
5. Undo after send: retraction queued, target cancelled, both in events.
6. "need augmentin" -> clarification asked, hitl_count increments, run ends.
7. GET /api/v1/stats shows hitl_count and cost for every agent enquiry.
8. agent/cases/cases.jsonl (20 lines: raw_request, quantity, scripted target replies) + scripts/run_cases.py
   prints the results table. Placeholders clearly marked until the owner supplies real cases.

## Ownership map
Lead (shared, frozen in W2): CONTRACT.md, db/migrations/0004*, 0005*, db/schema.dbml,
internal/queries/models.go, internal/queries/querier.go, internal/services/contracts.go,
internal/handlers/handlers.go, cmd/server/main.go, internal/config/config.go, openapi.yaml, .env.example.

Backend slices (each owns `internal/queries/sql/*_<slice>*.sql`, `internal/queries/<slice>.go`,
`internal/services/<slice>.go` + `_test.go`, `internal/handlers/<slice>.go` + `_test.go`, its smoke lines):
`auth` | `enquiries` (create/get/list/medication/candidates/targets/close) | `replies` (inbox/reply/clarify/approve) |
`outbox` (send/cancel/follow-up/no-response/dispatcher/undo) | `agentapi` (wakeups/runs/tool-calls/wait/notify/clarification-request) | `stats` (stats + SSE).

Agent: agent/tools/_http.py | agent/tools/enquiry.py | agent/tools/catalogue.py | agent/runner.py |
agent/prompts/system.md | agent/cases/ + scripts/run_cases.py. `web/` belongs to Rena: never edited.

## Out of scope
Customers, carts, orders, prescriptions, refills, contraindication/allergy fields; real SMS/WhatsApp
delivery (the outbox "sent" state is the delivery); multi-enquiry concurrency in the runner.

## Open decisions (owner)
1. Token usage: (a, recommended) capture it inside agent/ by wrapping Cornilia's provider
   (`OpenAIProvider.OneShotMessage` returns the ChatCompletion with `usage`), no Cornilia core edit;
   or (b) add a general `usage_tracer` hook to Cornilia core, next to the existing `tool_tracer`.
2. Guard: agentgw is not in backend/ yet, so v1 uses the AGENT_TOKEN header scheme behind `_http.py`.
