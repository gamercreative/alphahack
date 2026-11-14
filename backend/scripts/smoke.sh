#!/usr/bin/env bash
# Smoke test: curls every endpoint of a running server and checks status codes
# and the error format. Usage: BASE_URL=http://localhost:8080 scripts/smoke.sh
set -uo pipefail

BASE="${BASE_URL:-http://localhost:8080}"
PASS=0
FAIL=0
BODY=""

# req METHOD PATH EXPECTED_STATUS [JSON_BODY] [EXPECTED_ERROR_CODE]
req() {
  local method="$1" path="$2" want="$3" data="${4:-}" code="${5:-}"
  local out status
  if [ -n "$data" ]; then
    out=$(curl -sS -m 600 -X "$method" "$BASE$path" -H 'Content-Type: application/json' -d "$data" -w $'\n%{http_code}')
  else
    out=$(curl -sS -m 600 -X "$method" "$BASE$path" -w $'\n%{http_code}')
  fi
  status="${out##*$'\n'}"
  BODY="${out%$'\n'*}"
  local ok=1
  [ "$status" = "$want" ] || ok=0
  if [ -n "$code" ]; then
    case "$BODY" in
      *"\"error\":{\"code\":\"$code\""*"\"request_id\":\""*) ;;
      *) ok=0 ;;
    esac
  fi
  if [ "$ok" = 1 ]; then
    PASS=$((PASS + 1))
    printf 'PASS %-6s %-45s %s\n' "$method" "$path" "$status"
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL %-6s %-45s got %s want %s %s\n     %s\n' "$method" "$path" "$status" "$want" "$code" "$BODY"
  fi
}

# stream PATH JSON_BODY: POST and expect a 200 event stream holding start and done events.
stream() {
  local path="$1" data="$2" out status
  out=$(curl -sS -N -m 600 -X POST "$BASE$path" -H 'Content-Type: application/json' -d "$data" -w $'\n%{http_code}')
  status="${out##*$'\n'}"
  BODY="${out%$'\n'*}"
  case "$status|$BODY" in
    200\|*"event: start"*"event: delta"*"event: done"*)
      PASS=$((PASS + 1))
      printf 'PASS %-6s %-45s %s (start, delta, done)\n' POST "$path" "$status" ;;
    *)
      FAIL=$((FAIL + 1))
      printf 'FAIL %-6s %-45s got %s, want an event stream with start, delta and done\n     %s\n' POST "$path" "$status" "$BODY" ;;
  esac
}

# first_id: the first "id" value in $BODY. id_after KEY: the id of the object under KEY.
first_id() { printf '%s' "$BODY" | sed -n 's/^[^"]*"id":"\([^"]*\)".*/\1/p' | head -n 1; }
id_after() { printf '%s' "$BODY" | sed -n "s/.*\"$1\":{\"id\":\"\([^\"]*\)\".*/\1/p" | head -n 1; }

# body_has LABEL SUBSTRING: checks that the last response $BODY contains SUBSTRING.
body_has() {
  local label="$1" sub="$2"
  case "$BODY" in
    *"$sub"*)
      PASS=$((PASS + 1))
      printf 'PASS %-6s %-45s contains %s\n' CHECK "$label" "$sub" ;;
    *)
      FAIL=$((FAIL + 1))
      printf 'FAIL %-6s %-45s missing %s\n     %s\n' CHECK "$label" "$sub" "$BODY" ;;
  esac
}

echo "== smoke test against $BASE"

req GET /healthz 200

# agents
req POST /v1/agents 400 '{}' validation_failed
req POST /v1/agents 400 '{"name":"x","bogus":1}' bad_request
req POST /v1/agents 400 '{not json' bad_request
req POST /v1/agents 201 '{"name":"Smoke Agent","system_prompt":"You are a terse test agent."}'
AGENT_ID=$(first_id)
req GET /v1/agents 200
req GET "/v1/agents?limit=abc" 400 '' bad_request
req GET "/v1/agents/$AGENT_ID" 200
req PATCH "/v1/agents/$AGENT_ID" 200 '{"name":"Smoke Agent v2"}'
req PATCH "/v1/agents/$AGENT_ID" 400 '{}' validation_failed
req GET /v1/agents/does-not-exist 404 '' not_found
req PATCH /v1/agents/does-not-exist 404 '{"name":"x"}' not_found
req DELETE /v1/agents/does-not-exist 404 '' not_found

# conversations
req POST /v1/conversations 400 '{}' validation_failed
req POST /v1/conversations 404 '{"agent_id":"does-not-exist"}' not_found
req POST /v1/conversations 201 "{\"agent_id\":\"$AGENT_ID\",\"title\":\"smoke\"}"
CONV_ID=$(first_id)
req GET /v1/conversations 200
req GET "/v1/conversations?agent_id=$AGENT_ID" 200
req GET "/v1/conversations/$CONV_ID" 200
req GET /v1/conversations/does-not-exist 404 '' not_found

# tools + chat
req GET /v1/tools 200
req POST "/v1/conversations/$CONV_ID/messages" 400 '{"content":"   "}' validation_failed
req POST /v1/conversations/does-not-exist/messages 404 '{"content":"hi"}' not_found
req POST "/v1/conversations/$CONV_ID/messages" 201 '{"content":"Say hello in three words."}'
RUN_ID=$(id_after run)
req POST "/v1/conversations/$CONV_ID/messages" 201 '{"content":"/tool get_current_time {}"}'
stream "/v1/conversations/$CONV_ID/messages/stream" '{"content":"Count from one to three."}'
req POST "/v1/conversations/$CONV_ID/messages/stream" 400 '{"content":""}' validation_failed
req POST /v1/conversations/does-not-exist/messages/stream 404 '{"content":"hi"}' not_found
req GET "/v1/conversations/$CONV_ID/messages" 200
req GET /v1/conversations/does-not-exist/messages 404 '' not_found
req GET "/v1/conversations/$CONV_ID/runs" 200
req GET /v1/conversations/does-not-exist/runs 404 '' not_found
req GET "/v1/runs/$RUN_ID" 200
req GET /v1/runs/does-not-exist 404 '' not_found

# router-level errors
req GET /no/such/route 404 '' not_found
req DELETE /healthz 405 '' method_not_allowed

# cleanup (also proves cascade deletes)
req DELETE "/v1/conversations/$CONV_ID" 204
req GET "/v1/conversations/$CONV_ID" 404 '' not_found
req GET "/v1/runs/$RUN_ID" 404 '' not_found
req DELETE "/v1/agents/$AGENT_ID" 204
req GET "/v1/agents/$AGENT_ID" 404 '' not_found

# B2C pharmacy
req GET /api/v1/branches 200
body_has /api/v1/branches branch-byblos
body_has /api/v1/branches branch-hamra
body_has /api/v1/branches branch-baabda
req GET /api/v1/branches/branch-byblos 200
req GET /api/v1/branches/branch-byblos/inventory 200
req GET "/api/v1/medications?q=concor" 200
req GET /api/v1/medications/med-concor-5 200
body_has /api/v1/medications/med-concor-5 '"branch_id":"branch-byblos"'
body_has /api/v1/medications/med-concor-5 '"available_quantity":0'
body_has /api/v1/medications/med-concor-5 '"branch_id":"branch-hamra"'
body_has /api/v1/medications/med-concor-5 '"available_quantity":25'
req GET /api/v1/customers 200
req GET /api/v1/customers/cust-nadia 200
req GET /api/v1/customers/cust-nadia/prescriptions 200
req GET /api/v1/customers/cust-nadia/refills 200
req POST /api/v1/carts 201 '{"customer_id":"cust-nadia","branch_id":"branch-byblos"}'
NADIA_CART_ID=$(id_after cart)
req POST "/api/v1/carts/$NADIA_CART_ID/items" 409 '{"medication_id":"med-augmentin-1g","quantity":1}' conflict
body_has "/api/v1/carts/$NADIA_CART_ID/items" contraindication_detected
req POST "/api/v1/carts/$NADIA_CART_ID/items" 409 '{"medication_id":"med-lexotanil-3","quantity":1}' conflict
body_has "/api/v1/carts/$NADIA_CART_ID/items" controlled_substance
req POST /api/v1/carts 201 '{"customer_id":"cust-karim","branch_id":"branch-hamra"}'
KARIM_CART_ID=$(id_after cart)
req POST "/api/v1/carts/$KARIM_CART_ID/items" 409 '{"medication_id":"med-brufen-400","quantity":1}' conflict
body_has "/api/v1/carts/$KARIM_CART_ID/items" contraindication_detected
req POST "/api/v1/carts/$NADIA_CART_ID/items" 200 '{"medication_id":"med-glucophage-1000","prescription_id":"rx-nadia-glucophage","quantity":1}'
req POST "/api/v1/carts/$NADIA_CART_ID/checkout" 201 '{"fulfillment_type":"delivery","delivery_address":"Rue des Croisés, Byblos"}'
body_has "/api/v1/carts/$NADIA_CART_ID/checkout" '"batch_id":"batch-byblos-glucophage-a"'
NADIA_ORDER_ID=$(id_after order)
req GET /api/v1/customers/cust-nadia/orders 200
req GET "/api/v1/orders/$NADIA_ORDER_ID" 200

echo "== $PASS passed, $FAIL failed"
[ "$FAIL" = 0 ]
