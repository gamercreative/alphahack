# Wassit model eval: measured results

Measured 2026-11-15, 14:52 to 15:03 EET, on qwen3.5:35b-a3b (Ollama 10.31.216.24, thinking off,
CORNILIA_MAX_TOKENS=1024). Throwaway stack: backend :8126 (AI_PROVIDER=fake, UNDO_WINDOW=2s, WAIT_UNIT=1s),
Postgres container wassit-quality-db :5593. Case limit: 150 s.

**Scope: 5 of the 13 cases in model_eval.jsonl, all BEFORE the fixes below.** eval-12 was stopped mid-run
and is not scored. eval-01 shared the runner with one leftover enquiry from an aborted first attempt.
The owner then stopped live model runs, so **nothing after the fixes has been measured**. T6 (photos) was not run.

## Per case (before fixes)

| Case | T1 | T2 | T3 | T4 | T5 | Outcome (want) | Steps | Wall s | What went wrong |
|---|---|---|---|---|---|---|---|---|---|
| eval-01 | 1 | 0 | 0.50 | 0 | 0 | waiting (available) | 14 | 149 (timed out) | wait() after propose_targets; record_offer 400 x2 (evidence keys `price_cents`, `currency`); strength "1g" copied from the request with no quote, so the offer was escalated; expiry put in pickup_by |
| eval-02 | 1 | 1 | 0.33 | 0 | 1 | available (available) | 9 | 50 | closed available without record_offer for 2 replies; never told the requester about the 250mg alternative |
| eval-04 | 1 | 0 | 1.00 | - | 1 | available (available) | 11 | 90 | wait() after propose_targets; record_offer 400 x2 (same evidence keys) |
| eval-08 | 1 | 0 | - | - | 1 | unable_to_confirm (unable_to_confirm) | 14 | 146 | wait() after propose_targets |
| eval-09 | 1 | 0 | - | - | 0 | open (unable_to_confirm) | 5 | 150 (timed out) | wait() after propose_targets; too slow to reach replies |

## Per task (before fixes, cases measured)

| Task | Score | Cases scored |
|---|---|---|
| T1 understand request + match medicine | 1.00 | 5 |
| T2 propose pharmacies, stop for approval (no wait) | 0.20 | 5 |
| T3 read replies into record_offer (exact quotes, fields, decision) | 0.61 | 3 |
| T4 notify requester, content filter never fires | 0.00 | 2 |
| T5 finish: follow_up once, no_response, close as rules allow | 0.60 | 5 |
| T6 read package photos | not measured | 0 |

The content filter never fired (no 422 content_filtered) in any measured run.

## Fixes made after this measurement (unit-tested only, not measured live)

- agent/tools/enquiry.py `wait()`: refuses to set a timer while the status is awaiting_approval or
  awaiting_clarification. It returns a STOP note, which ends the run. Root cause: system.md step 3 said "wait".
- agent/prompts/system.md: step 3 now says to stop without wait(); "available" closes only after
  record_offer; client tickets are never closed "available".
- agent/tools/offers.py `record_offer`: maps alias evidence keys (`price_cents` to `price`, `qty`, ...); drops
  non-field keys (`currency`, `expiry`); fills currency from the price quote. It gives a teaching error before
  anything is stored when: a field has no quote, evidence is sent without fields, the quantity or price is not
  written in its quote, or a price was multiplied. The docstring has two worked examples. It returns the new
  next_step.
- agent/tools/enquiry.py `close_enquiry()`: refuses (`replies_unread`) while a text reply has no recorded offer.
- agent/tools/enquiry.py `_next_step`: an escalated "available" reading is reported with notify_requester.
  When a follow-up was sent and a later wait has fired, it says mark_no_response now. On client tickets:
  notify, then wait(15), never close "available" (Go refuses it), and no fall-through to a close.
  A photo is unread while `read_at`/`agent_code` are empty, and `wake_photo_id` is used even after
  `wake_trigger` is cleared.
- follow_up, mark_no_response and notify_requester also return the fresh next_step, so one run can do several steps.

## Reproduce

```bash
docker run -d --name wassit-quality-db -e POSTGRES_USER=alphahack -e POSTGRES_PASSWORD=alphahack \
  -e POSTGRES_DB=alphahack -p 5593:5432 postgres:16
for f in backend/db/migrations/*.up.sql; do docker exec -i wassit-quality-db psql -q -U alphahack -d alphahack < $f; done
(cd backend && PORT=8126 DATABASE_URL='postgres://alphahack:alphahack@localhost:5593/alphahack?sslmode=disable' \
  AI_PROVIDER=fake UNDO_WINDOW=2s WAIT_UNIT=1s AGENT_TOKEN=quality-token CORS_ALLOWED_ORIGINS='*' go run ./cmd/server) &
BACKEND_URL=http://localhost:8126 AGENT_TOKEN=quality-token EVAL_DB_CONTAINER=wassit-quality-db \
CORNILIA_PROVIDER=ollama CORNILIA_BASE_URL=http://10.31.216.24:11434/v1 CORNILIA_API_KEY=ollama \
CORNILIA_MODEL=qwen3.5:35b-a3b CORNILIA_TIMEOUT=120 CORNILIA_MAX_TOKENS=1024 \
python3.13 scripts/eval_model.py --label after            # or --only eval-01,eval-09
docker rm -f wassit-quality-db
```
