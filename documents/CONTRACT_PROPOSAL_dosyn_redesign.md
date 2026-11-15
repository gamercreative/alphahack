# CONTRACT proposal: DOSYN redesign (clients, schedules, tickets, dose rules, photos)

Status: **approved by the owner 2026-11-15 with the recommended answers; CONTRACT.md amended.** Already being built (no rule change needed): government reference + documents (0007), staff stock PUT, content filter on agent text (enforces today's rule 1), agent tool `check_reference`. Phase 3B (clients, schedules, tickets, equivalence, photos) follows. Written 2026-11-15
after the owner approved the Phase 1 frontend. Nothing here changes `CONTRACT.md` until approved.

Phase 1 runs every client feature on `web/wassit-demo.js` (the demo adapter). This document turns
each adapter method into a backend need, and lists the `CONTRACT.md` changes that make it legal.

---

## 1. Key design choice: client tickets reuse the enquiry machinery

A client ticket **is** an enquiry, sent by a new non-participating pharmacy row called
**DOSYN client desk** (`branch-client-desk`, `participating=false`) with one desk staff account
(`desk@wassit.demo`). Everything already built is reused unchanged:

- candidates and ranking, targets, outbox with undo window, follow-up, no_response
- `POST /targets/{id}/reply` (rule 2: only target pharmacy staff reply)
- structured offers with quoted evidence decided by Go (`services/enq_offers.go`)
- `enquiry_events` audit trail and undo, SSE stream, agent runs and costs

New tables only hold what the enquiry model lacks: the client, the schedule, the link from a
ticket to its enquiry, the government reference, package photos and dose checks.

Pharmacies see the request as coming from "DOSYN client desk": never the client's name, phone or id.

---

## 2. CONTRACT.md changes the owner must approve

### Rule 1 (today: "never substitutes; an alternative never counts as a match")
Proposed text:
> 1. The agent never diagnoses, prescribes, recommends or substitutes. A pharmacy's offer of a
>    different strength counts as a match **only when the Go dose rules accept it as an
>    equivalent**: same active ingredient, same dosage form, whole units that add up exactly to
>    the requested strength per dose (2 x 500mg = 1 x 1000mg; 625mg can never make 1000mg), and
>    the government reference lists the offered product. Anything else is shown to the client as
>    an option and never counts as a match. The agent can report what a pharmacy offered; only
>    the rules can mark an equivalent.

### Rule 8 (today: "customers, carts, orders, prescriptions, refills out of scope")
Proposed text:
> 8. Clients may have an account (display name, phone, PIN), scheduled medicines and tickets.
>    Pharmacies never see client identity. Out of scope and never read by the agent: the
>    `customers` table, carts, orders, prescriptions, refill subscriptions, and any
>    contraindication, allergy or chronic-condition field.

### New rules
> 9. **Content filter.** Every outbound text written by the agent (notifications, clarification
>    questions, outbox bodies, ticket notes) passes a Go check before it is stored: no
>    recommendation phrases (EN/AR/FR/Arabizi list in code) and no medicine name other than the
>    ticket's medicine or one a pharmacy offered. A blocked text returns 422 `content_filtered`
>    with a teaching message and opens a human review event. The agent never sends it.
>
> 10. **Photos are evidence, not proof.** A package photo is checked by Go (product code against
>    the government reference, expiry after the pickup window). A photo that cannot be checked
>    goes to a human. The UI always says that a photo alone does not prove a box is genuine.
>
> 11. **Best offer.** Computed in Go from verified offers only (decision `matches`, including
>    rule-accepted equivalents, plus a `verified` photo): lowest total price first, then the
>    latest expiry. The model never ranks offers.

### Changes to existing rules
- **Rule 2:** add "Only staff at the target branch can upload a package photo for that target."
- **Rule 4:** `enquiry_events.actor_type` gains `client`. Every event row also gets a derived
  **decider** in API responses: `rules` | `human` | `pharmacy` | `client` | `wassit`
  (`wassit` = relayed or read only; the API never says the AI decided).
- **Rule 6:** a client-desk enquiry can close `available` at a branch only if that branch has a
  verified offer (see rule 11). Reserving does exactly that.
- **Agent wake triggers:** add `scheduled` (a schedule opened a new ticket).
- **Target approval (needs a decision, see section 6, Q2):** for client-desk enquiries, Go
  auto-approves agent-proposed targets when every proposed branch is in the ranked candidate
  list; otherwise they wait for the desk staff.

---

## 3. Data (new migrations; each also updates db/schema.dbml + models.go)

**0008_clients**
- `clients`: id, display_name, phone UNIQUE, pin_hash (sha256 hex like staff), created_at
- `client_sessions`: token_hash PK, client_id FK, expires_at (12h)
- `enquiry_events.actor_type` CHECK adds `client`
- seed: `branch-client-desk` (participating=false, no lat/lng), staff `desk@wassit.demo` PIN 1234
  (role `reviewer`), demo client "Maya" phone `+96170000000` PIN `1234`

**0007_gov_reference** (being built now: owner OK'd the no-rule-change scope; seeded rows clearly labelled `source = 'Demo seed, not real MoPH data'`)
- `gov_products`: code PK (13-digit), reg_no, brand_name, ingredient, strength, dosage_form,
  official_price_cents, source, document_id FK NULL, created_at; UNIQUE(ingredient, strength, dosage_form, brand_name)
- `reference_documents`: id, name, content_type, size_bytes, sha256, uploaded_by_staff_id, storage_path, created_at
- seed: the 18 rows already in `wassit-demo.js` (11 catalogue products + 7 half/other strengths)

**0009_schedules_tickets**
- `medicine_schedules`: id, client_id FK, medication_id FK, quantity CHECK 1..10, every CHECK
  (week|2weeks|month), weekday CHECK 0..6, pickup_window, no_bulk bool default true,
  status CHECK (active|paused) default active, next_run_at, created_at
- `client_tickets`: id, client_id FK, enquiry_id FK UNIQUE, schedule_id FK NULL, mode CHECK
  (once|scheduled), pickup_window, status CHECK (open|reserved|closed) default open,
  reserved_branch_id FK NULL, created_at
- no_bulk means one ticket per cycle with that cycle's quantity; cycles are never merged.

**0010_offer_equivalence_photos**
- `target_offers` adds: unit_strength text default '' (strength of one unit offered),
  expiry_month text default '' (`YYYY-MM`), equivalent bool default false, gov_code text default '',
  total_cents bigint NULL (computed in Go = price_cents x quantity)
- `dose_checks`: id, offer_id FK, requested text, offered text, checks jsonb
  (`[{label, ok}]`, labels fixed), accepted bool, gov_code, created_at
- `package_photos`: id, target_id FK, branch_id FK, storage_path, content_type, size_bytes,
  typed_code text, typed_expiry_month text, status CHECK (verified|flagged|mismatch|rejected),
  gov_match bool, decided_by CHECK (rules|human), note, reviewed_by_staff_id NULL, created_at

Files (photos, documents) are stored on disk under `UPLOAD_DIR` (env, default `./uploads`),
max 5 MB photos / 10 MB documents, image/jpeg, image/png, application/pdf only.

---

## 4. Routes: one per demo adapter method

Auth header for clients: `Authorization: Bearer <client token>`. The middleware looks the token up
in `staff_sessions` first, then `client_sessions`, and sets `Actor{Type: "client", ID}`.
New error code `content_filtered` (422). Client-only route + staff token → 403 `forbidden`.
Agent credential on any client route → 403 `agent_forbidden`.

| Demo adapter method | Route | Who | Request | Response (key fields) |
| --- | --- | --- | --- | --- |
| (login) | POST /api/v1/client/auth/login | public | `{phone, pin}` | `{token, client:{id, display_name}}` |
| (me) | GET /api/v1/client/me | client | | `{client}` |
| `medFromApi` | GET /api/v1/medications (exists) | public | | unchanged |
| `createTicket` | POST /api/v1/client/tickets | client | `{medication_id, quantity, pickup_window}` | 201 ticket (below). Creates the client-desk enquiry with handled_by=agent, raw_request built from catalogue fields only, wake `created` |
| `listTickets` | GET /api/v1/client/tickets | client | `?status=` | `{data:[{id, medication:{brand_name, strength, dosage_form}, quantity, mode, pickup_window, status, created_at, working, best_offer}]}` |
| `getTicket` + `events` + `isWorking` | GET /api/v1/client/tickets/{id} | client (owner) | | `{ticket, medication, branches:[{id,name}], working, thread:[{id, at, kind, decider, branch_id, text, data}], best_offer}` |
| (live updates) | GET /api/v1/client/tickets/{id}/stream | client (owner) | `?access_token=` | SSE `ticket_event` per new thread row |
| `bestOffer` | part of ticket detail | client | | `best_offer:{branch_id, total_cents, expiry_month, offer_id, excluded:[{branch_id, reason}]}` or null |
| `reserve` | POST /api/v1/client/tickets/{id}/reserve | client (owner) | `{branch_id}` | ticket; 409 unless branch has a verified offer. Queues a notification to that branch and closes the enquiry `available` there |
| `ticketsForBranch` | GET /api/v1/inbox (exists) | staff | `?source=client` | inbox items gain `requester_kind: pharmacy\|client`, `pickup_window` |
| `branchThread` | GET /api/v1/enquiries/{id} (exists) | staff (target) | | target branch sees only its own target, offer, photos (no other pharmacies) |
| `pharmacyReply` | POST /api/v1/targets/{id}/reply (exists) | staff (target) | `{kind, quantity?, text}` | unchanged; agent then reads it with `record_offer` |
| (offer read) | POST /api/v1/targets/{id}/offers (exists) | staff, agent | adds `unit_strength?, expiry_month?` + evidence for them | offer + `dose_check` when strength differs |
| `pharmacyPhoto` | POST /api/v1/targets/{id}/photos | staff (target) | multipart: `photo`, `code`, `expiry_month` | 201 `{id, status, gov_match, expiry_month, decided_by, note}` |
| (human photo review) | POST /api/v1/photos/{id}/review | staff role reviewer | `{status: verified\|rejected, note}` | photo, event decided_by human |
| (photo file) | GET /api/v1/photos/{id}/file | staff (target or reviewer), client (ticket owner) | | image bytes |
| `listSchedules` | GET /api/v1/client/schedules | client | | `{data:[{id, medication, quantity, every, weekday, pickup_window, no_bulk, status, next_run_at, ticket_ids}]}` |
| `createSchedule` | POST /api/v1/client/schedules | client | `{medication_id, quantity, every, weekday, pickup_window}` | 201 schedule (first ticket opens at next_run_at) |
| `togglePause` | PATCH /api/v1/client/schedules/{id} | client (owner) | `{status: active\|paused}` | schedule |
| `runSchedule` | POST /api/v1/client/schedules/{id}/run | client (owner) | | 201 ticket (demo "run this cycle now"; 409 if a ticket for this cycle is still open: no bulk) |
| (scheduler) | background goroutine in main.go, 30s ticker | system | | opens a ticket when next_run_at passed, wake `scheduled`, advances next_run_at |
| `govReference` / `govLookup` | GET /api/v1/reference/products | public, staff, agent | `?q=&ingredient=&strength=&form=` | `{data:[{code, reg_no, brand_name, ingredient, strength, dosage_form, official_price_cents, source}]}` |
| (add row) | POST /api/v1/reference/products | staff role reviewer | row fields + `document_id?` | 201 row |
| `addReferenceDoc` | POST /api/v1/reference/documents | staff | multipart `document` | 201 `{id, name, size_bytes, created_at}` (no automatic parsing; a reviewer adds rows) |
| `listReferenceDocs` | GET /api/v1/reference/documents | staff | | `{data:[...]}` |
| `getStock` | GET /api/v1/branches/{id}/inventory (exists) | public | | unchanged |
| `setStock` | PUT /api/v1/branches/{id}/inventory/{medication_id} | staff (own branch only) | `{quantity_on_hand}` | batch row. Agent: 403 (rule 2: no tool marks stock) |
| `reset` | none (demo only) | | | |

Thread `kind` values returned by the ticket detail (same as the demo adapter, so the UI swap is a
data-source change only): `client_request`, `wassit_note`, `outreach`, `pharmacy_reply`,
`declined`, `no_answer`, `offer`, `dose_check`, `photo_check`, `best_offer`, `reserved`.
They are built in Go from `enquiry_events`, `outbox_messages`, `target_offers`, `dose_checks`
and `package_photos`; nothing new is stored for the thread itself.

---

## 5. Go rules and agent tools

**Dose equivalence** (`services/enq_dose.go`, next to `enq_offers.go`, pure function + table tests):
`EvaluateEquivalence(requested Medication, offered OfferInput, ref GovLookup) DoseCheck`.
Checks, in order, with fixed labels: Same active ingredient · Same form · Total strength matches
(`requested / unit` is a whole number 2..4 and `quantity` covers it) · Listed in government
reference. All four true → `accepted`, offer `equivalent=true`, decision may be `matches`.
Tests: 2x500=1000 accepted; 625 for 1000 rejected; inhaler 50mcg x2 for 100mcg accepted only if
listed; different ingredient rejected; unlisted product rejected.

**Photo check** (`services/enq_photos.go`): typed code must equal a `gov_products.code` for the
offer's product; typed expiry must be after the pickup window; missing or mismatched → `flagged`
(decided_by rules), waits for a reviewer.

**Best offer** (`services/client_tickets.go`): rule 11, pure function + tests.

**Content filter** (`services/content_filter.go`): rule 9, pure function + attack tests
(recommendation phrases in EN/AR/FR/Arabizi, other medicine names, zero-width tricks).

**Agent tools** (agent/tools/, results < 2k chars):
- `record_offer` (exists): add `unit_strength`, `expiry_month` with quotes.
- `check_reference(ingredient, strength, form)` → GET /reference/products (read only; the result
  is information for the model, the decision stays in Go).
- `notify_requester` (exists): for client-desk enquiries the text goes to the client ticket
  thread, through the content filter.
- No photo tool and no stock tool for the agent.
System prompt: add the client-desk case (never name or describe the client; relay, read, compare).

**Checks before Phase 3 is done:** `make check`, `make check-queries`, `scripts/smoke.sh` (every
new route), agent pytest, `python3.13 scripts/check_frontend_sync.py`, and one browser run:
client → Wassit → two pharmacies reply (one exact, one 2 x half strength) → photos → best offer
→ reserve.

---

## 6. Decisions needed from the owner

1. **Approve the rule 1, rule 8 and new rules 9 to 11 text** above (and the small changes to rules 2, 4 and 6).
2. **Target approval for client tickets.** Recommended: Go auto-approves agent proposals that are
   all in the ranked candidate list (fast demo). Alternative: the client approves the pharmacies
   in the chat (one more step, more human-in-the-loop).
3. **Barcode and expiry reading.** Recommended for the hackathon: the pharmacist types the code
   and expiry next to the photo, and Go checks them (no new dependency; Go's standard library has
   no barcode decoder). Later: a vision model reads the photo and Go still decides.
4. **Clients table.** Recommended: a new minimal `clients` table, so the agent can never reach
   the allergy and chronic-condition fields on the existing `customers` table.
5. **Shared server.** Phase 3 needs migrations 0006 to 0010 applied and the 8113 server
   restarted with new code. Both need your OK at that time.
