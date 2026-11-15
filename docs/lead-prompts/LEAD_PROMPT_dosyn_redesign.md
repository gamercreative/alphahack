# LEAD_PROMPT_dosyn_redesign

Redesign the DOSYN frontend around Wassit as an AI pharmacist chatbot for clients and pharmacists,
freeze the UI contract, then build the backend to match. Paste everything inside the fence into a
new Claude Code session started in `alphahack/`.

```text
You are the lead engineer for the DOSYN redesign. Read AGENTS.md, CLAUDE.md, CONTRACT.md,
docs/PLAN.md and docs/WORKFLOW.md first. Delegate small, fully specified pieces to the subagents in
.claude/agents/ (scout, builder, tester); you plan, review and verify.

## Product (owner's words, cleaned up)
DOSYN removes the weekly pain of hunting for a scarce medicine. Example: a patient needs a
medicine every Friday and is not allowed to buy in bulk, so every week they message many
pharmacies, argue about strengths and prices, and check the box. Wassit, an AI agent shown as a
pharmacist, does this for them:
1. The client searches the catalogue, picks a medicine and either requests it once or schedules
   it (for example every Friday, bulk not allowed, so Wassit keeps searching every cycle).
2. Wassit messages several pharmacies at once, describing only what is needed (drug, strength,
   form, quantity, pickup window), never the patient's identity.
3. Pharmacies answer in free text (Arabic, Arabizi, French, English). Wassit reads each reply into
   a structured offer with quoted evidence. Dose equivalents are allowed only when they are the
   same active ingredient and form and the total matches (2 x 500mg = 1 x 1g box), and only if
   the government health reference confirms the product. The Go rules decide this, not the model.
4. Pharmacies send package photos. The system checks them (barcode/product code against the
   government reference, expiry date) and flags unclear ones for a human. A photo alone does not
   prove a box is genuine; say so in the UI.
5. Wassit returns the best valid offer: lowest total price plus the latest expiry, from verified
   offers only.
6. Client and pharmacy each have a ticket page (a chat thread per request) where Wassit sits in the
   middle. The client sees every update: not answered, declined, a dose debate rejected, offer
   matched, photo verified.
7. Uploaded official documents (government price list or product leaflets) are allowed so the
   rules can compare against them.
Content filter: Wassit never suggests, recommends or substitutes a medicine on its own. It only
relays, reads and compares what the client asked for and what pharmacies offered. Anything off
the client's request goes to a human.

## Phase 1: frontend redesign (web/ only; do this first and get the owner's OK)
- Wassit is a visual pharmacist character (avatar, white coat, name tag) with chatbot speech
  bubbles, and a subtle "working" animation while it contacts pharmacies, waits and reads replies.
- Client section: a medicine search engine over the catalogue (GET /api/v1/medications). Tapping
  a medicine opens Wassit's chat (not the old enquiry form); Wassit asks quantity, once or
  scheduled, and pickup window, then shows the outreach live as bubbles (rings of pharmacies,
  replies, structured offer cards with highlighted evidence, photo checks, best price + expiry).
- Client folders stay, and a new Scheduled medicines folder shows each schedule (medicine, dose,
  quantity, day/interval, next run, "no bulk" flag) and its ticket history.
- Pharmacist section: same chat style. Wassit tells the pharmacist about incoming requests and
  helps them reply, update stock and upload package photos; keep the existing folder layout
  (inbox, stockroom, enquiries, stats).
- Ticket pages for both sides, with the decider shown on every event (Rules, Human, Pharmacy;
  never "AI decided"). Keep the existing offer card, Read offer form and Play call (voice stays
  as it is, macOS `say`; do not extend voice).
- Keep everything that already talks to the live API working. Where the backend does not support
  something yet, use a clearly labelled demo adapter in app.js so Phase 3 can swap it out.
- Bilingual English/Arabic with proper RTL; dark and light; phone width.
- Done when: the owner approves the look in the browser pane, and
  `python3.13 scripts/check_frontend_sync.py` still passes for every live call.

## Phase 2: UI contract (no code)
List every demo adapter from Phase 1 as a backend need: route, request, response fields, who may
call it. Write it as documents/CONTRACT_PROPOSAL_dosyn_redesign.md. It needs CONTRACT.md changes
the owner must approve before Phase 3: rule 1 (the agent never substitutes) must allow
rules-checked dose equivalents, and rule 8 (customers, prescriptions, refills out of scope) must
allow clients, scheduled medicines and tickets. Stop and ask the owner.

## Phase 3: backend to match (backend/ and agent/)
Follow backend/AGENTS.md layering and SQL rules exactly. Likely pieces: client accounts and
sessions; scheduled_medicines (+ a job that opens a ticket each cycle); client tickets linked to
the existing enquiries/targets/outbox; dose-equivalence rules in Go next to
services/enq_offers.go; a government reference table (seeded demo data, clearly labelled, plus
document upload) for product codes and official prices; package photo upload with barcode check
and expiry; best-offer ranking (price + expiry, verified only); a content-filter check on outbound
agent text. Add agent tools for the new steps. Every new route goes in openapi.yaml and smoke.sh.
Done when make check, make check-queries, smoke, agent pytest and the sync script all pass, and a
full client -> Wassit -> two pharmacies -> best offer run works in the browser.

## Current state (2026-11-15)
- Branch fix/audit-and-swarm-2026-11-15, nothing committed this session. Uncommitted and working:
  structured offers (migration 0006_target_offers, services/enq_offers.go, POST
  /api/v1/targets/{id}/offers, GET /api/v1/enquiries/{id}/offers), speech (internal/ai/speech.go,
  POST /api/v1/speech), agent tool agent/tools/offers.py, the offer card, Read offer form and Play
  call in web/app.js, scripts/check_frontend_sync.py, .claude/launch.json (web on 5500).
- Live stack: web on 5500 (launch config dosyn-web), backend `go run` on 8113 (DB on 5573,
  AI_PROVIDER=fake), agent runner -> 8113 with remote Ollama. 8113 runs OLD code: restarting it
  and applying migration 0006 needs the owner's OK. The 8080 server is stale; ignore it.
- Brief: ~/Downloads/Cortisone Supply Agent — Concept & Pitch Brief.pdf. Rena's original
  prototype: ~/Downloads/DOSYN-developer-handoff.zip.
- Never commit, push, delete data or restart shared servers without the owner's OK.
```
