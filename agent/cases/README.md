# Wassit case set

**These cases are PLACEHOLDERS.** Every line has `"placeholder": true`. They were written to
exercise the agent (ambiguity, four languages, all four outcomes), not taken from the owner's
real demand. Replace them with the 20 real cases from the owner, then set `placeholder` to
`false` (or remove the field) for those lines.

## Files

- `cases.jsonl`: one JSON object per line (20 lines). Read by `scripts/run_cases.py`.
- `results.md`: written by `scripts/run_cases.py` on every run (the last results table).

## Fields

| Field | Meaning |
| --- | --- |
| `id` | Case id, `case-01` to `case-20`. Used with `--only`. |
| `placeholder` | `true` while the case is not one of the owner's real cases. |
| `raw_request` | The text the requester (A, Hamra, `a@wassit.demo`) types. Any language. |
| `quantity` | Units requested. Sent as the enquiry `quantity`. For ambiguous cases it is the quantity after clarification. |
| `clarify_answer` | What A answers when the agent asks the requester a clarification question. `null` means the request is clear and the agent should not ask. |
| `replies` | Scripted human replies, one key per branch (`branch-achrafieh`, `branch-baabda`, `branch-verdun`, `branch-byblos`). Each value is the body the branch's pharmacist posts to `/api/v1/targets/{id}/reply` once the enquiry message was sent to that branch: `{"kind": "available", "quantity": n}`, `{"kind": "unavailable"}` or `{"kind": "alternative", "text": "..."}`. `null` means the branch never replies (silence). |
| `expected_outcome` | The enquiry outcome the closing rules require: `available`, `unavailable` or `unable_to_confirm`. |

## Mix in this placeholder set

- 3 ambiguous requests that need a clarification (`case-01`, `case-02`, `case-03`).
- 10 that end `available`, 4 that end `unavailable` (every contacted branch replies unavailable),
  6 that end `unable_to_confirm` (3 with an alternative offer, 3 with silence).
- Languages: English, French, Arabic, Arabizi (for example `badde augmentin 1g 2 3elab`).
- Medications: only the seeded catalogue (Panadol Advance 500mg, Brufen 400mg, Nexium 20mg,
  Augmentin 1000mg and 625mg, Zithromax 500mg, Glucophage 1000mg, Concor 5mg, Lipitor 20mg,
  Ventolin inhaler 100mcg).

An alternative (other strength, form or brand) is never a match. It is reported to the requester
and the enquiry cannot close `available` on it.

## Run

```bash
python3.13 scripts/run_cases.py                    # all cases, scripted policy
python3.13 scripts/run_cases.py --only case-06     # one case
```

The runner drives the backend and `agent/runner.py` (`--once`) for each case. It writes
`agent/cases/results.md`.
