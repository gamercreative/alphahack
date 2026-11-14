# Wassit benchmarks

The 20 cases in `agent/cases/cases.jsonl` are **placeholders**, written for the demo until the owner
supplies the real ones. Each case is replayed by `scripts/run_cases.py`: requester A creates an agent
enquiry, scripted pharmacists reply (or stay silent), and the agent runs until the enquiry closes or
25 rounds pass. The server runs with `UNDO_WINDOW=3s WAIT_UNIT=2s`, so timers are compressed.

| Run | Date (UTC) | Model / policy | Matches | HITL | Steps | Tokens | Wall time | Cost |
|---|---|---|---|---|---|---|---|---|
| [qwen](agent/cases/results_qwen.md) | 2026-11-14 22:06 | `qwen3.5:35b-a3b` on LAN Ollama 192.168.1.108 | **16/20** | 22 | 193 | 529k | 26.2 min | $0 (own GPU) |
| [haiku](agent/cases/results_haiku.md) | 2026-11-14 12:20 | `claude-haiku-5-5` (Anthropic) | 15/20 | 22 | 248 | ~1.1M | 14.6 min | $0.108 |
| [scripted](agent/cases/results_scripted.md) | 2026-11-14 12:00 | no model (`--policy scripted`) | 14/20 | 44 | 318 | 0 | 12.1 min | $0 |

Steps are model calls (or tool calls for the scripted policy). The Haiku token count also includes
two demo enquiries made during the same run window. Haiku cost uses $0.10 in / $0.50 out per million tokens.

## The code is not identical across runs

Before the qwen run, three guards were added in code (commit "Agent guards for small local models"):

1. `search_medications` retries word by word when the full phrase finds nothing, and flags the one match with the requested strength.
2. The runner ends a run right after a successful `wait`, `close_enquiry` or `ask_requester` (qwen kept calling tools after them).
3. The backend refuses a second clarification while one is pending, and refuses an agent `unable_to_confirm` close while a contacted pharmacy is still unanswered.

Haiku ran without these guards, so a rerun of Haiku on the current code would be the fair comparison.

## Where each run missed

| Case | Expected | qwen | Haiku | Why |
|---|---|---|---|---|
| case-03 (Arabic) | unable_to_confirm | ok | still waiting | Arabic brand name; the word-by-word search fixed it for qwen |
| case-05 | available | unavailable | unavailable | the pharmacy scripted "available" was not among the 3 nearest asked |
| case-08 | available | stuck on a clarification | ok | qwen asked a question the case file never answers |
| case-10, case-11 | available | unable_to_confirm | unable_to_confirm | same as case-05 |
| case-16 | unable_to_confirm | ok | stuck on a clarification | Haiku asked a question the case file never answers |

Cases 05, 10 and 11 are mostly about how the placeholder cases are written.

## Full agent flow (acceptance check 2), one run each

| Model | Result | Runs | Tokens | Time |
|---|---|---|---|---|
| qwen3.5:35b-a3b (LAN) | passed: alternative flagged as not a match, follow-up, no_response, closed unable_to_confirm | 6 | 42k | 188 s |
| claude-haiku-5-5 | passed, same steps | 5 | 63k | 79 s |

Other checks on the same code: `go test ./...` green, `scripts/smoke.sh` 83/83, `scripts/acceptance.py` 70/70.

Reproduce: start the backend (DEMO.md), then
`CORNILIA_PROVIDER=ollama CORNILIA_BASE_URL=http://192.168.1.108:11434/v1 CORNILIA_API_KEY=ollama CORNILIA_MODEL=qwen3.5:35b-a3b python3.13 scripts/run_cases.py --policy model`
