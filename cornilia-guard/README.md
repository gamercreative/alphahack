# cornilia-guard

A security layer between Cornilia's tool calls and the Go backend.
The model never touches the database. It can only ask for **named operations**,
and Go decides whether each request is allowed.

```
user ──> Go API ──(mints token: user, scopes, budget)──> Cornilia (Python)
                                                          │ model picks a tool
                                                          ▼
                                                  ToolGuard (Python, fast fail)
                                                          │ signed request + token
                                                          ▼
                       Go agent gateway /agent/v1/ops/<op>  (makes the final decision)
                                                          │ fixed parameterized SQL
                                                          ▼
                                                      Postgres
```

## Two keys, two jobs

| Key | Who holds it | Proves |
|---|---|---|
| `CORNILIA_TOKEN_KEY` | **Go only** | the token (user, scopes, budget, expiry) came from Go and wasn't edited |
| `CORNILIA_SERVICE_KEY` | Go + Cornilia | the request came from Cornilia, wasn't changed in transit, and isn't a replay |

Cornilia can't widen its own permissions, because it never has the token key.

## What each layer stops

| Attack | Python guard | Go gateway |
|---|---|---|
| Model invents a tool / raw SQL | unknown tool | no such op (404); there is no "run SQL" op |
| Model passes `user_id` to act as someone else | `extra="forbid"` | `DisallowUnknownFields`; user always = token `sub` |
| Injection in DB/web data says "book 10 seats" | output wrapped + flagged; writes need approval | scope check, `Validate()` ranges |
| Agent loops forever | repeat detection, call budget, halt after 3 errors | per-session budget → 429 |
| Retry double-applies a write | stable idempotency key per action | `Idempotency-Key` required for writes, replayed response |
| Someone else on the network calls the gateway | none | HMAC request signature, ±30s window, nonce cache |
| Token edited to add `bookings:write` | none | token HMAC fails |
| DB error leaks schema to the model | generic message | `internal error` out; real error only in the audit log |
| Tool calls an endpoint that starts another agent run | none | `RejectAgentCredentials` on the normal API |

Go doesn't trust anything the Python side checked. The Python checks give the
model fast, readable errors; Go makes the final decision.

## Breaking the Go → Python → Go loop

1. Agent credentials (`Authorization: Agent …` or a Cornilia signature) only
   work on `/agent/v1/ops/*`. Everything else is wrapped in
   `RejectAgentCredentials`, so a tool can never reach the route that starts a
   new Cornilia run.
2. Gateway handlers run their query and return. They never call Cornilia.
3. The budget lives in the token and is counted by Go per session, so a buggy
   or bypassed guard still stops at N calls.

So it is always exactly one hop out and one hop back. No recursion is possible.

## Plugging into Cornilia

```python
from cornilia_guard import GatewayClient, ToolGuard, SYSTEM_RULES, ask_in_terminal
from example_tools import TOOLS, run_tool_calls

client = GatewayClient("http://backend:8080", SERVICE_KEY, agent_token_from_go)
guard = ToolGuard(TOOLS, client, approver=ask_in_terminal)

messages = [{"role": "system", "content": base_prompt + "\n" + SYSTEM_RULES}, ...]
resp = provider.chat(messages, tools=guard.schemas())       # any provider
messages += run_tool_calls(guard, resp.tool_calls)
if guard.should_stop: ...  # end the run
```

In production the `approver` is a callback into your UI. For example, Go sends a
"confirm booking?" card to the user, and the run continues only on yes.

## Adding a new database tool (both sides)

1. **Go**: an args struct with `Validate()` and `agentgw.Register(g, "x.y", "x:read", RiskRead, handler)`.
   The handler uses `$1` placeholders and `c.Sub` for the user. Never use string-built SQL.
2. **Python**: a `StrictArgs` model with the same limits plus a `ToolSpec(name, "x.y", Model, Risk.READ, desc)`.
3. **Go route that starts Cornilia**: add the scope to `MintSession(...)` only for features that need it.

## Run

```bash
cd go && go test -race ./...
cd python && python3 -m pytest -q tests     # includes e2e against the real Go binary
```

## Before production

- Store idempotency keys in Postgres (`UNIQUE (user_id, idempotency_key)`) instead of memory,
  and the nonce/budget maps in Redis if you run more than one gateway instance.
- Use a separate DB role for the gateway with only the grants its ops need.
  That's the last wall if a handler has a bug.
- Rotate keys by accepting two keys during the switch.
- Ship the audit log somewhere you can search it.
