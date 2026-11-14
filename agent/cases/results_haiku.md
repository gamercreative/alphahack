# Wassit case results

- Run (UTC): 2026-11-14 12:37:18
- Cases file: /Users/akramseifeddine/Documents/alphahack/agent/cases/cases.jsonl
- Policy: model; max rounds: 25; backend: http://localhost:8080
- PLACEHOLDER cases: 20 of 20 (not the owner's real cases yet)

| Case | Expected | Outcome | Match | HITL | Runs | Steps | Cost (micros) | Rounds | Elapsed (s) | Error |
|---|---|---|---|---|---|---|---|---|---|---|
| case-01 | available | available | yes | 2 | 4 | 13 | 5576 | 5 | 29.3 |  |
| case-02 | available | available | yes | 2 | 4 | 14 | 6032 | 5 | 28.2 |  |
| case-03 | unable_to_confirm | none (waiting) | no | 2 | 6 | 19 | 8889 | 25 | 108.1 |  |
| case-04 | available | available | yes | 1 | 3 | 10 | 4223 | 4 | 22.6 |  |
| case-05 | available | unavailable | no | 1 | 3 | 10 | 4210 | 4 | 24.3 |  |
| case-06 | available | available | yes | 1 | 3 | 10 | 4295 | 4 | 29.6 |  |
| case-07 | available | available | yes | 1 | 3 | 13 | 5432 | 4 | 30.0 |  |
| case-08 | available | available | yes | 1 | 3 | 10 | 4295 | 4 | 22.3 |  |
| case-09 | available | available | yes | 1 | 3 | 13 | 5484 | 4 | 29.1 |  |
| case-10 | available | unable_to_confirm | no | 1 | 4 | 15 | 6599 | 7 | 43.9 |  |
| case-11 | available | unable_to_confirm | no | 1 | 5 | 17 | 7447 | 9 | 56.4 |  |
| case-12 | unable_to_confirm | unable_to_confirm | yes | 1 | 4 | 15 | 6969 | 8 | 45.8 |  |
| case-13 | unable_to_confirm | unable_to_confirm | yes | 1 | 4 | 14 | 6162 | 7 | 43.0 |  |
| case-14 | unable_to_confirm | unable_to_confirm | yes | 1 | 5 | 16 | 7164 | 15 | 82.0 |  |
| case-15 | unable_to_confirm | unable_to_confirm | yes | 1 | 5 | 15 | 6790 | 16 | 78.7 |  |
| case-16 | unable_to_confirm | none (awaiting_clarification) | no | 0 | 1 | 4 | 1812 | 25 | 98.9 |  |
| case-17 | unavailable | unavailable | yes | 1 | 3 | 10 | 4260 | 4 | 26.2 |  |
| case-18 | unavailable | unavailable | yes | 1 | 3 | 10 | 4182 | 4 | 22.1 |  |
| case-19 | unavailable | unavailable | yes | 1 | 3 | 10 | 4269 | 4 | 24.9 |  |
| case-20 | unavailable | unavailable | yes | 1 | 3 | 10 | 4212 | 4 | 30.6 |  |

**Totals:** matches 15/20; total HITL 22; total steps 248; total cost $0.108302 (108302 micros).
