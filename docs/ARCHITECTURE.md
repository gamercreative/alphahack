# Architecture

Code decides what is allowed. The model only decides what to do next.

## Components

| Component | Path | Tech | Role | Port |
| --- | --- | --- | --- | --- |
| Backend | `backend/` | Go 1.25, stdlib router, pgx, hand-written SQL | REST API, all product rules, outbox dispatcher, SSE | 8080 (demo 8113) |
| Database | `backend/db/` | PostgreSQL 16 in Docker, migrations 0001 to 0007 | storage | 5432 (demo 5573) |
| Agent | `agent/` | Python 3.13, Cornilia v0.1.0 | polls wakeups, one tool loop per enquiry, acts only over HTTP with its own credential | none (client only) |
| Model | external | Ollama `qwen3.5:35b-a3b`, thinking off | chooses the next tool call | 11434 |
| Web | `web/` | static HTML, CSS, JS | pharmacist UI on the live API; client UI on the demo adapter | 5500 |
| Guard | `cornilia-guard/` | Go gateway + Python package | signed tool calls; not mounted yet, the agent uses `AGENT_TOKEN` | |

```mermaid
flowchart LR
  subgraph Browser
    UI[web/app.js + wassit-ui.js]
    DEMO[web/wassit-demo.js<br/>localStorage dosyn-demo-v2]
  end
  UI -- "REST + SSE, Bearer token" --> API[Go backend]
  UI -. "client tickets, schedules, photos (demo)" .-> DEMO
  RUN[agent/runner.py + Cornilia] -- "REST, Agent token" --> API
  RUN -- "OpenAI API /v1" --> OLL[Ollama qwen3.5:35b-a3b]
  API --> PG[(PostgreSQL 16)]
```

Backend layering, enforced by `backend/scripts/check_queries.sh`:
`handlers -> services -> queries -> PostgreSQL`, and `services -> ai`. Only `internal/queries`
touches the database; `internal/services` never imports `net/http`.

## Flow 1: pharmacist enquiry (live)

```mermaid
sequenceDiagram
  participant A as Pharmacist A (requester)
  participant API as Go backend
  participant OB as Outbox dispatcher (2s ticker)
  participant B as Pharmacist B (target)
  A->>API: POST /api/v1/enquiries
  A->>API: POST /enquiries/{id}/targets {branch_ids}
  A->>API: POST /enquiries/{id}/send
  API->>API: outbox row, send_after = now + UNDO_WINDOW
  opt inside the undo window
    A->>API: POST /messages/{id}/cancel (target never sees it)
  end
  OB->>API: queued -> sent, event message_sent
  B->>API: GET /api/v1/inbox
  B->>API: POST /targets/{id}/reply {kind, quantity, text}
  API-->>A: SSE enquiry_event (GET /enquiries/{id}/stream)
  A->>API: POST /enquiries/{id}/close {outcome, branch_id}
  API->>API: closing rules, 409 conflict if broken
```

## Flow 2: agent wake loop (live)

```mermaid
sequenceDiagram
  participant R as agent/runner.py
  participant API as Go backend
  participant M as Model (or scripted policy)
  loop every 3 s
    R->>API: GET /api/v1/agent/wakeups
    API-->>R: [{enquiry_id, trigger}]
    R->>API: POST /api/v1/agent/runs
    R->>M: system prompt + trigger + compact enquiry state
    loop until wait() or close_enquiry(), max 12 steps
      M-->>R: tool call
      R->>API: tool HTTP call (agent/tools/_http.py)
      R->>API: POST /agent/runs/{id}/tool-calls
    end
    R->>API: PATCH /agent/runs/{id} (tokens, steps, status)
    API->>API: cost_micros computed in Go
  end
```

Wake triggers: `created`, `clarified`, `approved`, `reply`, `timer`, `scheduled`. Hitting the
step limit marks the run `failed` and leaves the enquiry open for a human.

## Flow 3: offer and rules decision

```mermaid
flowchart TD
  REPLY[Pharmacist free-text reply<br/>Arabic, Arabizi, French, English] --> READ[Agent reads it into a structured offer<br/>with quoted evidence and confidence]
  READ --> POST[POST /api/v1/targets/id/offers]
  POST --> RULES{Go rules}
  RULES -- same product --> MATCH[decision: matches]
  RULES -- different strength --> DOSE{Dose equivalence<br/>same ingredient, same form,<br/>whole units add up exactly,<br/>listed in gov reference}
  DOSE -- yes --> MATCH
  DOSE -- no --> REJ[decision: rejected<br/>shown as an option, never a match]
  RULES -- unclear or low confidence --> ESC[decision: escalated to a human]
  MATCH --> PHOTO{Photo check: typed code vs reference,<br/>expiry vs pickup window}
  PHOTO -- verified --> BEST[Best offer: lowest total, then latest expiry]
  PHOTO -- unchecked or mismatch --> ESC
```

`target_offers.decision` is `matches | escalated | rejected` and is set in Go. The model never
ranks offers and never marks an equivalent.

## Flow 4: client ticket (demo adapter today, Phase 3B design)

```mermaid
sequenceDiagram
  participant C as Client (web)
  participant D as Demo adapter (wassit-demo.js)
  participant API as Go backend (Phase 3B, not built)
  C->>D: search, pick medicine and quantity, send one request
  D->>D: create ticket in localStorage (dosyn-demo-v2)
  D-->>C: simulated outreach, offers, equivalence, photo checks, best offer
  Note over D,API: Phase 3B replaces each adapter method with an apiCall.<br/>Tickets become enquiries from the "DOSYN client desk",<br/>so pharmacies never see client identity.
```

The UI labels this data as demo. Design: `documents/CONTRACT_PROPOSAL_dosyn_redesign.md`.

## Data model

Source: `backend/db/migrations/` (authoritative) and `backend/db/schema.dbml`.

| Group | Tables | Migration |
| --- | --- | --- |
| Template chat loop | `agents`, `conversations`, `runs`, `messages`, `tool_calls` | 0001 |
| Pharmacy base | `branches`, `medications`, `inventory_batches`, `customers`, `prescriptions`, `carts`, `cart_items`, `orders`, `order_items`, `refill_subscriptions` | 0002, seed 0003 |
| Wassit enquiries | `staff`, `staff_sessions`, `enquiries`, `enquiry_targets`, `outbox_messages`, `enquiry_events`, `enquiry_agent_runs`, `enquiry_agent_tool_calls`; `branches` gains `lat`, `lng`, `participating` | 0004, seed 0005 |
| Offers | `target_offers` (structured offer per target, evidence, confidence, `decision`) | 0006 |
| Government reference | `gov_products`, `reference_documents` | 0007 |

Chain: `enquiries -> enquiry_targets -> target_offers`; `enquiries -> outbox_messages`;
`enquiries -> enquiry_events` (append-only, with a stored inverse where reversible);
`enquiries -> enquiry_agent_runs -> enquiry_agent_tool_calls`. Ids are text UUIDs.
Column lists for the 0004 tables are in `CONTRACT.md`, section Data.

Enquiry status: `open | awaiting_clarification | awaiting_approval | waiting | closed | cancelled`.
Target status: `proposed | approved | sent | available | unavailable | alternative_offered | no_response | cancelled`.

## Where each product rule is enforced

Rule numbers are from `CONTRACT.md`.

| Rule | Enforced in | How |
| --- | --- | --- |
| 1. No substitution; equivalents only by rules | `backend/internal/services/enq_dose.go` (`EvaluateEquivalence`), `enq_offers.go`; `agent/prompts/system.md` | Go compares ingredient, form and exact unit sums and looks the product up in the reference; the prompt repeats the rule but is not relied on |
| 2. Only a human at the target branch replies | route tags in `backend/internal/handlers/` (`h.auth(staffOnly, ...)` on reply, approve, clarify, stock, uploads); `services/enq_replies.go` | agent credential gets 403 `agent_forbidden`; no agent tool marks stock available |
| 3. Silence is `no_response` | `services/enq_outbox.go` (follow-up, no-response) | only allowed after one follow-up; never `unavailable` |
| 4. Every change is an event with an inverse; derived decider | `services/enquiry_common.go`, `enq_outbox.go` (undo) | `enquiry_events` rows; `POST /events/{id}/undo` runs the stored inverse |
| 5. Outbox with undo window | `services/enq_outbox.go`, dispatcher started in `backend/cmd/server/main.go` | `send_after = now + UNDO_WINDOW` |
| 6. Closing rules | `services/enq_enquiries.go` | 409 `conflict` with a teaching message |
| 7. Agent works from DB data; asks when ambiguous | `agent/tools/catalogue.py`, `agent/tools/enquiry.py` (`ask_requester`) | tools only reach the backend API |
| 8. Client identity hidden; customers out of scope | no agent tool reads customer routes (`agent/tools/`); Phase 3B "client desk" design | |
| 9. Content filter | `services/content_filter.go` | 422 `content_filtered`, nothing stored or sent |
| 10. Photo is evidence, not proof | UI text and demo adapter (`web/wassit-ui.js`, `web/wassit-demo.js`); reference lookup in `services/reference.go` | photo upload route is Phase 3B |
| 11. Best offer in Go | `services/client_best.go` (`PickBestOffer`) | verified offers only: lowest total, then latest expiry |

Other fixed points: auth middleware and route tags in `backend/internal/handlers/`
(`enq_auth.go`, `enquiry_common.go`); error codes in `services/contracts.go`; speech in
`services/enq_speech.go`; stock update in `services/stock.go`; agent HTTP in
`agent/tools/_http.py`; token usage wrapper in `agent/usage.py`; scripted fallback policy in
`agent/scripted.py`.
