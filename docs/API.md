# API reference

Source of truth: `backend/openapi.yaml` (schemas) and the `mux.HandleFunc` registrations in
`backend/internal/handlers/` (71 routes). This page is the map; read the OpenAPI file for full
field lists. JSON is snake_case. Base URL: `http://localhost:8080` (live demo: port 8113).

## Auth

| Caller | Header | How to get it |
| --- | --- | --- |
| public | none | |
| staff | `Authorization: Bearer <token>` | `POST /api/v1/auth/login {email, pin}`; 32 random bytes, stored hashed, 12 h expiry |
| agent | `Authorization: Agent <AGENT_TOKEN>` | backend env `AGENT_TOKEN`; empty disables the agent |

Routes are tagged in code as `staffOnly`, `agentOK` (staff or agent) or `agentOnly`.

## Errors

Every error has one shape:

```json
{"error": {"code": "conflict", "message": "what was wrong and what to try", "request_id": "..."}}
```

| Status | Code | When |
| --- | --- | --- |
| 401 | `unauthorized` | missing or bad credential |
| 403 | `agent_forbidden` | agent credential on a staff-only route (reply, approve, clarify, stock, uploads) |
| 403 | `forbidden` | staff token on an agent-only route, or wrong branch |
| 409 | `conflict` | state rule broken (closing rules, cancel after send, second follow-up, undo twice, speech off) |
| 422 | `content_filtered` | agent-written text failed the Go content filter; nothing is sent |

Validation and not-found errors use the same envelope; codes are listed per route in `openapi.yaml`.
Lists return `{"data": [...], "limit": n, "offset": n}`. Create returns 201, delete 204.

## Health

| Method | Path | Who | Result |
| --- | --- | --- | --- |
| GET | `/healthz` | public | liveness and database reachability |

## Wassit: auth and identity

| Method | Path | Who | Body -> result |
| --- | --- | --- | --- |
| POST | `/api/v1/auth/login` | public | `{email, pin}` -> `{token, staff, branch}` |
| GET | `/api/v1/me` | staff | `{staff, branch}` |

## Wassit: enquiries

| Method | Path | Who | Body -> result | Notable errors |
| --- | --- | --- | --- | --- |
| POST | `/api/v1/enquiries` | staff | `{raw_request, medication_id?, quantity, handled_by: manual\|agent}` -> 201 enquiry | |
| GET | `/api/v1/enquiries?scope=outgoing\|incoming&status=` | staff | list for my branch | |
| GET | `/api/v1/enquiries/{id}` | staff, agent | enquiry + targets + messages + events + run summaries | |
| POST | `/api/v1/enquiries/{id}/medication` | staff, agent | `{medication_id}` | |
| POST | `/api/v1/enquiries/{id}/clarification-request` | agent only | `{question}` -> `awaiting_clarification` | 422 `content_filtered` |
| POST | `/api/v1/enquiries/{id}/clarify` | staff (requester) | `{answer}` -> `open`, wake `clarified` | 403 `agent_forbidden` |
| GET | `/api/v1/enquiries/{id}/candidates?limit=5&radius_km=15` | staff, agent | branches ranked in Go | |
| POST | `/api/v1/enquiries/{id}/targets` | staff, agent | `{branch_ids}`; staff -> approved, agent -> proposed | |
| POST | `/api/v1/enquiries/{id}/targets/approve` | staff (requester) | `{target_ids}`, wake `approved` | 403 `agent_forbidden` |
| POST | `/api/v1/enquiries/{id}/send` | staff, agent | one queued outbox message per approved target | |
| POST | `/api/v1/enquiries/{id}/notify` | staff, agent | `{body}` -> notification to the requester | 422 `content_filtered` |
| POST | `/api/v1/enquiries/{id}/wait` | agent only | `{minutes: 5\|15\|30}` -> `next_wake_at` | |
| POST | `/api/v1/enquiries/{id}/close` | staff, agent | `{outcome, branch_id?}` | 409 `conflict` (closing rules) |
| GET | `/api/v1/enquiries/{id}/stream` | staff | SSE, event `enquiry_event` per new row | |
| GET | `/api/v1/enquiries/{id}/offers` | staff, agent | structured offers of the enquiry | |

## Wassit: targets, outbox, undo

| Method | Path | Who | Body -> result | Notable errors |
| --- | --- | --- | --- | --- |
| GET | `/api/v1/inbox` | staff | enquiries waiting for my branch (message already sent) | |
| POST | `/api/v1/targets/{id}/reply` | staff of the target branch | `{kind: available\|unavailable\|alternative, quantity?, text?}`, wake `reply` | 403 `agent_forbidden` |
| POST | `/api/v1/targets/{id}/offers` | staff, agent | `{confidence (0..1, required), in_stock?, quantity?, price_cents?, pickup_by?, payment?: cash\|card\|any, needs_prescription?, evidence?: {field: quoted words}}` -> offer with a Go `decision` | |
| POST | `/api/v1/targets/{id}/follow-up` | staff, agent | max one per target, only if sent | 409 |
| POST | `/api/v1/targets/{id}/no-response` | staff, agent | only if sent and one follow-up was made | 409 |
| POST | `/api/v1/messages/{id}/cancel` | staff, agent | only while queued | 409 |
| POST | `/api/v1/events/{id}/undo` | staff, agent | runs the stored inverse | 409 if not reversible or already undone |

## Wassit: agent bookkeeping, stats, speech

| Method | Path | Who | Body -> result |
| --- | --- | --- | --- |
| GET | `/api/v1/agent/wakeups` | agent only | `{data: [{enquiry_id, trigger}]}`, oldest first |
| POST | `/api/v1/agent/runs` | agent only | `{enquiry_id, trigger, model}` -> 201 run |
| PATCH | `/api/v1/agent/runs/{id}` | agent only | `{status, input_tokens, output_tokens, steps, error}`; Go computes `cost_micros` |
| POST | `/api/v1/agent/runs/{id}/tool-calls` | agent only | `{name, input, output, is_error, duration_ms}` |
| GET | `/api/v1/stats` | staff | per enquiry: outcome, `hitl_count`, agent steps, cost; totals |
| POST | `/api/v1/speech` | staff, agent | `{text (1..600 chars), lang?: ar\|en\|fr}` -> audio bytes; 409 when speech is off |

## Catalogue, stock, government reference

| Method | Path | Who | Body -> result |
| --- | --- | --- | --- |
| GET | `/api/v1/branches`, `/api/v1/branches/{id}` | public | pharmacy branches |
| GET | `/api/v1/branches/{id}/inventory` | public | inventory batches, FEFO order |
| PUT | `/api/v1/branches/{id}/inventory/{medication_id}` | staff, own branch | `{quantity_on_hand: 0..1000000}`; agent gets 403 `agent_forbidden` |
| GET | `/api/v1/medications?q=`, `/api/v1/medications/{id}` | public | search; detail with sellable stock per branch |
| GET | `/api/v1/reference/products` | public | search the government reference list (information only) |
| POST | `/api/v1/reference/products` | staff | add a reference product |
| GET | `/api/v1/reference/documents` | staff | uploaded reference documents, newest first |
| POST | `/api/v1/reference/documents` | staff | upload PDF, PNG or JPEG, max 10 MB, stored under `UPLOAD_DIR/reference` |

## Pharmacy template routes (out of Wassit scope, never read by the agent)

All are registered through the `h.staffPharmacy` wrapper in `backend/internal/handlers/` (see the wrapper for the exact credential check).

| Method | Path |
| --- | --- |
| GET | `/api/v1/customers`, `/api/v1/customers/{id}` |
| PATCH | `/api/v1/customers/{id}` |
| GET, POST | `/api/v1/customers/{id}/prescriptions`, `/api/v1/customers/{id}/refills` |
| PATCH | `/api/v1/refills/{id}` |
| POST | `/api/v1/carts`, `/api/v1/carts/{id}/items`, `/api/v1/carts/{id}/checkout` |
| GET | `/api/v1/carts/{id}`, `/api/v1/customers/{id}/orders`, `/api/v1/orders/{id}` |
| DELETE | `/api/v1/carts/{id}/items/{medication_id}` |

## Backend template chat loop (no auth, not used by Wassit)

| Method | Path | Body / notes |
| --- | --- | --- |
| POST, GET | `/v1/agents` | create `{name, system_prompt?, model?}`; list |
| GET, PATCH, DELETE | `/v1/agents/{id}` | |
| POST, GET | `/v1/conversations` | create `{agent_id, title?}`; list |
| GET, DELETE | `/v1/conversations/{id}` | |
| GET | `/v1/conversations/{id}/messages` | history |
| POST | `/v1/conversations/{id}/messages` | `{content}` runs the backend loop with `AI_PROVIDER` |
| POST | `/v1/conversations/{id}/messages/stream` | same turn as SSE |
| GET | `/v1/conversations/{id}/runs`, `/v1/runs/{id}` | run and tool-call trace |
| GET | `/v1/tools` | the backend's Go tools |

## Not built (Phase 3B)

Client accounts, client tickets, schedules and photo checks have no routes yet. Their design is in
`documents/CONTRACT_PROPOSAL_dosyn_redesign.md`; the UI uses `web/wassit-demo.js` until then.
