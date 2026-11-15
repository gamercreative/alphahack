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
# Demo staff a/b/c/d@wassit.demo, PIN 1234. AGENT_TOKEN must match the server's AGENT_TOKEN.
AGENT_TOKEN="${AGENT_TOKEN:-dev-agent-token}"

# wreq AUTH METHOD PATH EXPECTED_STATUS [JSON_BODY] [EXPECTED_ERROR_CODE]
# AUTH is the full Authorization header value ("Bearer <token>" or "Agent <token>"), "" for none.
wreq() {
  local auth="$1" method="$2" path="$3" want="$4" data="${5:-}" code="${6:-}"
  local out status
  local args=(-sS -m 600 -X "$method" "$BASE$path" -H 'Content-Type: application/json')
  if [ -n "$auth" ]; then args+=(-H "Authorization: $auth"); fi
  if [ -n "$data" ]; then args+=(-d "$data"); fi
  out=$(curl "${args[@]}" -w $'\n%{http_code}')
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

# json_field KEY: the top-level field KEY of $BODY (empty if missing or not JSON).
json_field() { printf '%s' "$BODY" | python3 -c 'import json,sys; print(json.load(sys.stdin).get(sys.argv[1], ""))' "$1" 2>/dev/null; }

req POST /api/v1/auth/login 200 '{"email":"a@wassit.demo","pin":"1234"}'
TOKEN_A=$(json_field token)
req POST /api/v1/auth/login 200 '{"email":"b@wassit.demo","pin":"1234"}'
TOKEN_B=$(json_field token)

wreq "Bearer $TOKEN_A" GET /api/v1/customers 200
wreq "Bearer $TOKEN_A" GET /api/v1/customers/cust-nadia 200
wreq "Bearer $TOKEN_A" GET /api/v1/customers/cust-nadia/prescriptions 200
wreq "Bearer $TOKEN_A" GET /api/v1/customers/cust-nadia/refills 200
wreq "Bearer $TOKEN_A" POST /api/v1/carts 201 '{"customer_id":"cust-nadia","branch_id":"branch-byblos"}'
NADIA_CART_ID=$(id_after cart)
wreq "Bearer $TOKEN_A" POST "/api/v1/carts/$NADIA_CART_ID/items" 409 '{"medication_id":"med-augmentin-1g","quantity":1}' conflict
body_has "/api/v1/carts/$NADIA_CART_ID/items" contraindication_detected
wreq "Bearer $TOKEN_A" POST "/api/v1/carts/$NADIA_CART_ID/items" 409 '{"medication_id":"med-lexotanil-3","quantity":1}' conflict
body_has "/api/v1/carts/$NADIA_CART_ID/items" controlled_substance
wreq "Bearer $TOKEN_A" POST /api/v1/carts 201 '{"customer_id":"cust-karim","branch_id":"branch-hamra"}'
KARIM_CART_ID=$(id_after cart)
wreq "Bearer $TOKEN_A" POST "/api/v1/carts/$KARIM_CART_ID/items" 409 '{"medication_id":"med-brufen-400","quantity":1}' conflict
body_has "/api/v1/carts/$KARIM_CART_ID/items" contraindication_detected
wreq "Bearer $TOKEN_A" POST "/api/v1/carts/$NADIA_CART_ID/items" 200 '{"medication_id":"med-glucophage-1000","prescription_id":"rx-nadia-glucophage","quantity":1}'
wreq "Bearer $TOKEN_A" POST "/api/v1/carts/$NADIA_CART_ID/checkout" 201 '{"fulfillment_type":"delivery","delivery_address":"Rue des Croisés, Byblos"}'
body_has "/api/v1/carts/$NADIA_CART_ID/checkout" '"batch_id":"batch-byblos-glucophage-a"'
NADIA_ORDER_ID=$(id_after order)
wreq "Bearer $TOKEN_A" GET /api/v1/customers/cust-nadia/orders 200
wreq "Bearer $TOKEN_A" GET "/api/v1/orders/$NADIA_ORDER_ID" 200

echo "== Wassit"

wreq "Bearer $TOKEN_A" GET /api/v1/me 200
wreq "Bearer $TOKEN_A" POST /api/v1/enquiries 201 '{"raw_request":"smoke: panadol","medication_id":"med-panadol-advance","quantity":1,"handled_by":"manual"}'
ENQ_ID=$(json_field id)
wreq "Bearer $TOKEN_A" GET "/api/v1/enquiries/$ENQ_ID" 200
wreq "Bearer $TOKEN_A" GET "/api/v1/enquiries/$ENQ_ID/candidates" 200
wreq "Bearer $TOKEN_A" POST "/api/v1/enquiries/$ENQ_ID/targets" 201 '{"branch_ids":["branch-achrafieh"]}'
wreq "Bearer $TOKEN_A" POST "/api/v1/enquiries/$ENQ_ID/send" 200
wreq "Bearer $TOKEN_B" GET /api/v1/inbox 200
wreq "Bearer $TOKEN_A" POST "/api/v1/enquiries/$ENQ_ID/close" 200 '{"outcome":"unable_to_confirm"}'
wreq "Bearer $TOKEN_A" GET /api/v1/stats 200

# Agent credential on a staff-only route, and no credential at all.
wreq "Agent $AGENT_TOKEN" POST "/api/v1/enquiries/$ENQ_ID/clarify" 403 '{"answer":"smoke"}' agent_forbidden
wreq "" GET /api/v1/me 401 '' unauthorized

echo "== Offers + speech"

# data_id: the first id under "data" in $BODY.
data_id() { printf '%s' "$BODY" | python3 -c 'import json,sys; d=json.load(sys.stdin).get("data") or [{}]; print(d[0].get("id",""))' 2>/dev/null; }

wreq "Bearer $TOKEN_A" POST /api/v1/enquiries 201 '{"raw_request":"smoke: augmentin 1g x2","medication_id":"med-augmentin-1g","quantity":2,"handled_by":"manual"}'
OFFER_ENQ_ID=$(json_field id)
wreq "Bearer $TOKEN_A" POST "/api/v1/enquiries/$OFFER_ENQ_ID/targets" 201 '{"branch_ids":["branch-achrafieh"]}'
OFFER_TARGET_ID=$(data_id)
wreq "Bearer $TOKEN_A" POST "/api/v1/enquiries/$OFFER_ENQ_ID/send" 200
OFFER_BODY='{"in_stock":true,"strength":"625mg","payment":"cash","evidence":{"in_stock":"3ande","strength":"625","payment":"cash"},"confidence":0.85}'
wreq "Bearer $TOKEN_A" POST "/api/v1/targets/$OFFER_TARGET_ID/offers" 400 '{"in_stock":true}' validation_failed
wreq "Bearer $TOKEN_A" POST "/api/v1/targets/$OFFER_TARGET_ID/offers" 409 "$OFFER_BODY" conflict
wreq "Bearer $TOKEN_B" POST "/api/v1/targets/$OFFER_TARGET_ID/offers" 403 "$OFFER_BODY" forbidden
wreq "Agent $AGENT_TOKEN" POST "/api/v1/targets/$OFFER_TARGET_ID/offers" 403 "$OFFER_BODY" forbidden
wreq "" POST "/api/v1/targets/$OFFER_TARGET_ID/offers" 401 "$OFFER_BODY" unauthorized

# The reply needs the enquiry delivered (send_after = now + UNDO_WINDOW). Wait up to SMOKE_UNDO_WAIT seconds.
delivered=0
for _ in $(seq 1 "${SMOKE_UNDO_WAIT:-15}"); do
  BODY=$(curl -sS -m 10 "$BASE/api/v1/inbox" -H "Authorization: Bearer $TOKEN_B")
  case "$BODY" in *"$OFFER_TARGET_ID"*) delivered=1; break ;; esac
  sleep 1
done
if [ "$delivered" = 1 ]; then
  wreq "Bearer $TOKEN_B" POST "/api/v1/targets/$OFFER_TARGET_ID/reply" 200 '{"kind":"alternative","text":"3ande bas 625, ta3e abel l 6, cash"}'
  wreq "Bearer $TOKEN_A" POST "/api/v1/targets/$OFFER_TARGET_ID/offers" 201 "$OFFER_BODY"
  OFFER_ID=$(json_field id)
  body_has "offer decision" '"decision":"escalated"'
  body_has "offer decided_by" '"decided_by":"rules"'
  wreq "Bearer $TOKEN_A" GET "/api/v1/enquiries/$OFFER_ENQ_ID/offers" 200
  body_has "/api/v1/enquiries/{id}/offers" "\"id\":\"$OFFER_ID\""
  wreq "Bearer $TOKEN_B" GET "/api/v1/enquiries/$OFFER_ENQ_ID/offers" 403 '' forbidden
  wreq "Bearer $TOKEN_A" GET "/api/v1/enquiries/$OFFER_ENQ_ID" 200
  body_has "offer events" '"type":"offer_decided"'
else
  printf 'SKIP offers after reply: enquiry not delivered within %ss (server UNDO_WINDOW too long; set SMOKE_UNDO_WAIT)\n' "${SMOKE_UNDO_WAIT:-15}"
fi
wreq "Bearer $TOKEN_A" POST "/api/v1/enquiries/$OFFER_ENQ_ID/close" 200 '{"outcome":"unable_to_confirm"}'

# Speech answers audio bytes, so check status and size instead of the JSON body.
speech_check() {
  local auth="$1" want="$2" data="$3" args out status size
  args=(-sS -m 60 -o /dev/null -w '%{http_code} %{size_download}' -X POST "$BASE/api/v1/speech" -H 'Content-Type: application/json' -d "$data")
  if [ -n "$auth" ]; then args+=(-H "Authorization: $auth"); fi
  out=$(curl "${args[@]}")
  status="${out%% *}"; size="${out##* }"
  if [ "$status" = "$want" ] && { [ "$want" != 200 ] || [ "$size" -gt 44 ]; }; then
    PASS=$((PASS + 1)); printf 'PASS %-6s %-45s %s (%s bytes)\n' POST /api/v1/speech "$status" "$size"
  else
    FAIL=$((FAIL + 1)); printf 'FAIL %-6s %-45s got %s (%s bytes) want %s\n' POST /api/v1/speech "$status" "$size" "$want"
  fi
}
speech_check "Bearer $TOKEN_A" 200 '{"text":"Hello, do you have Augmentin 1g?","speaker":"agent"}'
speech_check "Agent $AGENT_TOKEN" 200 '{"text":"مرحبا، عندكن أوغمنتين؟","speaker":"pharmacy"}'
speech_check "Bearer $TOKEN_A" 400 '{"text":"hi","lang":"de"}'
speech_check "" 401 '{"text":"hi"}'

echo "== $PASS passed, $FAIL failed"
[ "$FAIL" = 0 ]
