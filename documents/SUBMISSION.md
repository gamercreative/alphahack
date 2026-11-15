# DOSYN / Wassit: submission (AlphaHack 2026)

## Problem

In Lebanon a scarce medicine means a weekly hunt. The patient calls or walks to pharmacy after
pharmacy. Pharmacists phone each other. Nobody knows who has stock, at what price, or whether the
box is genuine. People who can afford it buy in bulk, which makes the shortage worse.

## Solution

DOSYN removes the hunt. Wassit is an AI pharmacist chatbot for clients and pharmacists.

- The client searches the catalogue and asks once, or on a schedule (for example every Friday).
  No bulk buying.
- Wassit messages several pharmacies at once, without the client's identity.
- Pharmacies reply in free text (Arabic, Arabizi, French, English). Wassit turns each reply into a
  structured offer with the quoted words as evidence.
- Go rules decide matches and dose equivalents. Package photos are checked.
- The best verified offer comes back in a ticket. Every event shows who decided: Rules, Human or
  Pharmacy. Never "AI decided".

## Demo flow

1. Client searches Augmentin, picks 1000mg, 2 boxes, and sends one request.
2. The ticket shows outreach, a decline, an offer with highlighted quotes, one dose equivalent
   accepted and one rejected, a verified photo, a blurry photo sent to a human, and "no response".
3. The best verified offer appears. The client reserves it.
4. A pharmacist replies in Arabizi and uploads a photo. That pharmacy becomes the best offer.
5. A weekly schedule (every Friday, no bulk) and the Arabic right-to-left UI.

Step by step: [docs/DEMO.md](../docs/DEMO.md).

## Architecture

- **Go API + PostgreSQL.** Stores enquiries, offers, events, outbox and agent runs. All rules live here.
- **Cornilia agent with Ollama** (`qwen3.5:35b-a3b`). Python. Polls the backend and acts only
  through its HTTP API, with its own credential.
- **Static web** (`web/`). Plain HTML, CSS and JS. Live updates over SSE.

Code decides what is allowed. The model only decides what to do next.

## Safety rules (from [CONTRACT.md](../CONTRACT.md))

- **Rule 1.** The agent never diagnoses, prescribes, recommends or substitutes. Only the Go dose rules can accept an equivalent (2 x 500mg = 1 x 1000mg, same ingredient and form, listed in the government reference).
- **Rule 2.** Only a human pharmacist at the target pharmacy can say available or unavailable, or upload a photo. The agent gets 403.
- **Rule 3.** No reply is recorded as "no response", never as "unavailable".
- **Rule 8.** Pharmacies never see the client's identity. The agent never reads prescriptions, allergies or conditions.
- **Rule 9.** Every text the agent writes passes a Go content filter. Recommendations and other medicine names are blocked.
- **Rule 10.** A photo is evidence, not proof. Code and expiry are checked in Go. Anything unclear goes to a human.
- **Rule 11.** The best offer is computed in Go from verified offers only: lowest total, then latest expiry. The model never ranks.

## What is live vs demo

**Live:** pharmacist-to-pharmacist enquiries, agent runs, outbox with undo, SSE, structured offers,
speech (macOS `say`), and new: government reference, stock update, content filter.

**Demo adapter** (`web/wassit-demo.js`, clearly labelled, localStorage): client accounts, client
tickets, schedules, photo checks.

## What's next (Phase 3B)

Move the demo adapter to the backend: client accounts, tickets as enquiries from a "DOSYN client
desk", schedules, dose equivalence in Go, and photo checks. The design is approved and written in
[CONTRACT_PROPOSAL_dosyn_redesign.md](CONTRACT_PROPOSAL_dosyn_redesign.md). It is not built yet.

## How to run

See [README.md](../README.md).
