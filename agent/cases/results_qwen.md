# Wassit case results

- Run (UTC): 2026-11-14 22:34:07
- Cases file: /Users/akramseifeddine/Documents/alphahack/agent/cases/cases.jsonl
- Policy: model; max rounds: 25; backend: http://localhost:8080
- PLACEHOLDER cases: 20 of 20 (not the owner's real cases yet)

| Case | Expected | Outcome | Match | HITL | Runs | Steps | Cost (micros) | Rounds | Elapsed (s) | Error |
|---|---|---|---|---|---|---|---|---|---|---|
| case-01 | available | available | yes | 2 | 4 | 10 | 0 | 4 | 75.4 |  |
| case-02 | available | available | yes | 2 | 4 | 10 | 0 | 5 | 71.9 |  |
| case-03 | unable_to_confirm | unable_to_confirm | yes | 2 | 6 | 16 | 0 | 14 | 149.2 |  |
| case-04 | available | available | yes | 1 | 3 | 8 | 0 | 4 | 59.2 |  |
| case-05 | available | unavailable | no | 1 | 3 | 8 | 0 | 4 | 55.4 |  |
| case-06 | available | available | yes | 1 | 3 | 8 | 0 | 3 | 63.0 |  |
| case-07 | available | available | yes | 1 | 3 | 8 | 0 | 3 | 54.2 |  |
| case-08 | available | none (awaiting_clarification) | no | 0 | 1 | 2 | 0 | 25 | 114.9 |  |
| case-09 | available | available | yes | 1 | 3 | 8 | 0 | 4 | 53.6 |  |
| case-10 | available | unable_to_confirm | no | 1 | 4 | 11 | 0 | 6 | 91.8 |  |
| case-11 | available | unable_to_confirm | no | 1 | 4 | 11 | 0 | 6 | 95.9 |  |
| case-12 | unable_to_confirm | unable_to_confirm | yes | 1 | 4 | 11 | 0 | 10 | 85.5 |  |
| case-13 | unable_to_confirm | unable_to_confirm | yes | 1 | 4 | 12 | 0 | 8 | 85.4 |  |
| case-14 | unable_to_confirm | unable_to_confirm | yes | 1 | 5 | 13 | 0 | 18 | 123.3 |  |
| case-15 | unable_to_confirm | unable_to_confirm | yes | 1 | 5 | 14 | 0 | 18 | 129.0 |  |
| case-16 | unable_to_confirm | unable_to_confirm | yes | 1 | 4 | 11 | 0 | 10 | 93.3 |  |
| case-17 | unavailable | unavailable | yes | 1 | 3 | 8 | 0 | 4 | 44.0 |  |
| case-18 | unavailable | unavailable | yes | 1 | 3 | 8 | 0 | 4 | 39.3 |  |
| case-19 | unavailable | unavailable | yes | 1 | 3 | 8 | 0 | 4 | 43.7 |  |
| case-20 | unavailable | unavailable | yes | 1 | 3 | 8 | 0 | 4 | 42.8 |  |

**Totals:** matches 16/20; total HITL 22; total steps 193; total cost $0.000000 (0 micros).
