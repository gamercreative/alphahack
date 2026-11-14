# Wassit case results

- Run (UTC): 2026-11-14 12:20:31
- Cases file: /Users/akramseifeddine/Documents/alphahack/agent/cases/cases.jsonl
- Policy: scripted; max rounds: 25; backend: http://localhost:8080
- PLACEHOLDER cases: 20 of 20 (not the owner's real cases yet)

| Case | Expected | Outcome | Match | HITL | Runs | Steps | Cost (micros) | Rounds | Elapsed (s) | Error |
|---|---|---|---|---|---|---|---|---|---|---|
| case-01 | available | available | yes | 2 | 4 | 13 | 0 | 6 | 16.6 |  |
| case-02 | available | available | yes | 2 | 4 | 10 | 0 | 5 | 13.3 |  |
| case-03 | unable_to_confirm | none (open) | no | 25 | 25 | 99 | 0 | 25 | 78.9 |  |
| case-04 | available | available | yes | 1 | 3 | 9 | 0 | 5 | 13.3 |  |
| case-05 | available | unavailable | no | 1 | 3 | 11 | 0 | 5 | 13.3 |  |
| case-06 | available | available | yes | 1 | 3 | 9 | 0 | 5 | 13.3 |  |
| case-07 | available | available | yes | 1 | 3 | 9 | 0 | 5 | 13.3 |  |
| case-08 | available | none (awaiting_clarification) | no | 0 | 1 | 3 | 0 | 25 | 78.3 |  |
| case-09 | available | available | yes | 1 | 3 | 9 | 0 | 5 | 13.3 |  |
| case-10 | available | unable_to_confirm | no | 1 | 6 | 18 | 0 | 17 | 52.4 |  |
| case-11 | available | unable_to_confirm | no | 1 | 6 | 17 | 0 | 17 | 52.3 |  |
| case-12 | unable_to_confirm | unable_to_confirm | yes | 1 | 6 | 20 | 0 | 18 | 55.6 |  |
| case-13 | unable_to_confirm | unable_to_confirm | yes | 1 | 6 | 17 | 0 | 16 | 49.0 |  |
| case-14 | unable_to_confirm | unable_to_confirm | yes | 1 | 7 | 17 | 0 | 21 | 65.4 |  |
| case-15 | unable_to_confirm | unable_to_confirm | yes | 1 | 7 | 20 | 0 | 21 | 65.5 |  |
| case-16 | unable_to_confirm | none (awaiting_clarification) | no | 0 | 1 | 3 | 0 | 25 | 78.2 |  |
| case-17 | unavailable | unavailable | yes | 1 | 3 | 8 | 0 | 5 | 13.4 |  |
| case-18 | unavailable | unavailable | yes | 1 | 3 | 8 | 0 | 5 | 13.3 |  |
| case-19 | unavailable | unavailable | yes | 1 | 3 | 9 | 0 | 5 | 13.3 |  |
| case-20 | unavailable | unavailable | yes | 1 | 3 | 9 | 0 | 5 | 13.4 |  |

**Totals:** matches 14/20; total HITL 44; total steps 318; total cost $0.000000 (0 micros).
