#!/usr/bin/env bash
# Scripted DOSYN client run (Phase 3B), no model: the agent's steps are made
# with the agent token. Needs a server with AI_PROVIDER=fake and a short
# UNDO_WINDOW (e.g. 2s), migrations 0001-0010 applied, and fresh seed data.
#   BASE_URL=http://localhost:8124 AGENT_TOKEN=dev-agent-token scripts/smoke_client.sh
# client login -> ticket -> agent proposes 2 targets (auto-approved) -> send ->
# B replies (exact match), C replies (2 x half strength) -> agent records offers
# -> dose check accepted for C -> photos (C types a wrong code) -> best offer = B,
# C excluded -> reserve B -> enquiry closed available at B. Plus schedules.
set -uo pipefail

BASE="${BASE_URL:-http://localhost:8080}"
AGENT="Agent ${AGENT_TOKEN:-dev-agent-token}"
PASS=0
FAIL=0
BODY=""
STATUS=""
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

ok() { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n     %s\n' "$1" "$BODY"; }

# call AUTH METHOD PATH [JSON] : sets BODY and STATUS
call() {
  local auth="$1" method="$2" path="$3" data="${4:-}" out
  local args=(-sS -m 60 -X "$method" "$BASE$path" -H 'Content-Type: application/json')
  [ -n "$auth" ] && args+=(-H "Authorization: $auth")
  [ -n "$data" ] && args+=(-d "$data")
  out=$(curl "${args[@]}" -w $'\n%{http_code}')
  STATUS="${out##*$'\n'}"
  BODY="${out%$'\n'*}"
}

# expect AUTH METHOD PATH WANT_STATUS [JSON] [WANT_ERROR_CODE]
expect() {
  call "$1" "$2" "$3" "${5:-}"
  local label="$2 $3 -> $4${6:+ $6}"
  if [ "$STATUS" = "$4" ] && { [ -z "${6:-}" ] || [[ "$BODY" == *"\"code\":\"$6\""* ]]; }; then ok "$label"; else bad "$label (got $STATUS)"; fi
}

# py EXPR: evaluates EXPR with d = parsed $BODY
py() { printf '%s' "$BODY" | python3 -c "import json,sys; d=json.load(sys.stdin); print($1)" 2>/dev/null; }

check() { if [ "$2" = "$3" ]; then ok "$1 = $3"; else bad "$1: got '$2', want '$3'"; fi; }

# photo AUTH TARGET_ID CODE EXPIRY : uploads a 1x1 PNG
photo() {
  local out
  out=$(curl -sS -m 60 -X POST "$BASE/api/v1/targets/$2/photos" -H "Authorization: $1" \
    -F "photo=@$TMP/box.png;type=image/png" --form-string "code=$3" --form-string "expiry_month=$4" -w $'\n%{http_code}')
  STATUS="${out##*$'\n'}"
  BODY="${out%$'\n'*}"
}
python3 -c 'import base64,sys; open(sys.argv[1],"wb").write(base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=="))' "$TMP/box.png"

echo "== DOSYN client run against $BASE"

# --- client auth
expect "" POST /api/v1/client/auth/login 401 '{"phone":"+96170000000","pin":"0000"}' unauthorized
expect "" POST /api/v1/client/auth/login 200 '{"phone":"+96170000000","pin":"1234"}'
CLIENT="Bearer $(py 'd["token"]')"
expect "$CLIENT" GET /api/v1/client/me 200
check "client display_name" "$(py 'd["client"]["display_name"]')" "Maya"
call "" POST /api/v1/auth/login '{"email":"a@wassit.demo","pin":"1234"}'; STAFF_A="Bearer $(py 'd["token"]')"
call "" POST /api/v1/auth/login '{"email":"b@wassit.demo","pin":"1234"}'; STAFF_B="Bearer $(py 'd["token"]')"
call "" POST /api/v1/auth/login '{"email":"c@wassit.demo","pin":"1234"}'; STAFF_C="Bearer $(py 'd["token"]')"
call "" POST /api/v1/auth/login '{"email":"desk@wassit.demo","pin":"1234"}'; DESK="Bearer $(py 'd["token"]')"
expect "$STAFF_A" GET /api/v1/client/me 403 '' forbidden
expect "$AGENT" GET /api/v1/client/tickets 403 '' agent_forbidden
expect "$CLIENT" GET /api/v1/me 403 '' forbidden
expect "$CLIENT" GET /api/v1/inbox 403 '' forbidden

# --- ticket
expect "$CLIENT" POST /api/v1/client/tickets 400 '{"medication_id":"med-augmentin-1g","quantity":2,"pickup_window":"call Maya on +96170000000"}' validation_failed
expect "$CLIENT" POST /api/v1/client/tickets 400 '{"medication_id":"med-augmentin-1g","quantity":11,"pickup_window":"Today before 18:00"}' validation_failed
expect "$CLIENT" POST /api/v1/client/tickets 201 '{"medication_id":"med-augmentin-1g","quantity":2,"pickup_window":"Today before 18:00"}'
TICKET=$(py 'd["id"]'); ENQ=$(py 'd["enquiry_id"]')
expect "$CLIENT" GET "/api/v1/client/tickets?status=open" 200
expect "$CLIENT" GET "/api/v1/client/tickets/$TICKET" 200
check "first thread row" "$(py 'd["thread"][0]["kind"]')" "client_request"
check "ticket working" "$(py 'd["working"]')" "True"

expect "$AGENT" GET /api/v1/agent/wakeups 200
check "wakeup for the ticket" "$(py "[w['trigger'] for w in d['data'] if w['enquiry_id']=='$ENQ'][0]")" "created"
expect "$AGENT" GET "/api/v1/enquiries/$ENQ" 200
check "raw_request has no client data" "$(py '"Maya" in d["enquiry"]["raw_request"] or "+961" in d["enquiry"]["raw_request"]')" "False"
check "requester is the desk" "$(py 'd["enquiry"]["requester_branch_id"]')" "branch-client-desk"
expect "$AGENT" GET "/api/v1/enquiries/$ENQ/candidates?limit=20" 200
check "desk never a candidate" "$(py '"branch-client-desk" in [c["branch_id"] for c in d["data"]]')" "False"

# --- agent proposes 2 targets -> Go auto-approves (client desk)
expect "$AGENT" POST "/api/v1/enquiries/$ENQ/targets" 201 '{"branch_ids":["branch-achrafieh","branch-baabda"]}'
check "targets auto-approved" "$(py '",".join(sorted(set(t["status"] for t in d["data"])))')" "approved"
expect "$AGENT" GET "/api/v1/enquiries/$ENQ" 200
check "targets_approved by rules" "$(py "[e['actor_id'] for e in d['events'] if e['type']=='targets_approved'][0]")" "rules"
check "wake approved" "$(py 'd["enquiry"]["wake_trigger"]')" "approved"
expect "$AGENT" POST "/api/v1/enquiries/$ENQ/send" 200

# wait for the dispatcher (UNDO_WINDOW)
for _ in $(seq 1 "${SMOKE_UNDO_WAIT:-15}"); do
  call "$STAFF_B" GET /api/v1/inbox
  [[ "$BODY" == *"$ENQ"* ]] && [[ "$(curl -sS "$BASE/api/v1/inbox" -H "Authorization: $STAFF_C")" == *"$ENQ"* ]] && break
  sleep 1
done
call "$STAFF_B" GET /api/v1/inbox
TB=$(py "[i['target_id'] for i in d['data'] if i['enquiry_id']=='$ENQ'][0]")
check "inbox requester_kind" "$(py "[i['requester_kind'] for i in d['data'] if i['enquiry_id']=='$ENQ'][0]")" "client"
check "inbox pickup_window" "$(py "[i['pickup_window'] for i in d['data'] if i['enquiry_id']=='$ENQ'][0]")" "Today before 18:00"
check "inbox requester name" "$(py "[i['requester_branch_name'] for i in d['data'] if i['enquiry_id']=='$ENQ'][0]")" "DOSYN client desk"
call "$STAFF_C" GET /api/v1/inbox
TC=$(py "[i['target_id'] for i in d['data'] if i['enquiry_id']=='$ENQ'][0]")

# --- replies + offers
expect "$STAFF_B" POST "/api/v1/targets/$TB/reply" 200 '{"kind":"available","quantity":2,"text":"eh 3ande 2 3elab 12$ exp 08/2027"}'
expect "$AGENT" POST "/api/v1/targets/$TB/offers" 201 '{"in_stock":true,"quantity":2,"price_cents":1200,"currency":"USD","evidence":{"in_stock":"3ande","quantity":"2 3elab","price":"12$"},"confidence":0.9}'
check "B offer decision" "$(py 'd["decision"]')" "matches"
expect "$STAFF_C" POST "/api/v1/targets/$TC/reply" 200 '{"kind":"available","quantity":4,"text":"3ande 500mg bas 4 3elab 7$ exp 03/2027"}'
expect "$AGENT" POST "/api/v1/targets/$TC/offers" 201 '{"in_stock":true,"strength":"500mg","quantity":4,"price_cents":700,"currency":"USD","evidence":{"in_stock":"3ande","strength":"500mg","quantity":"4 3elab","price":"7$"},"confidence":0.9}'
check "C offer decision (rules-accepted equivalent)" "$(py 'd["decision"]')" "matches"
expect "$CLIENT" GET "/api/v1/client/tickets/$TICKET" 200
check "dose_check accepted" "$(py "[r['data']['accepted'] for r in d['thread'] if r['kind']=='dose_check'][0]")" "True"
check "no best offer before photos" "$(py 'd["best_offer"]')" "None"
expect "$CLIENT" POST "/api/v1/client/tickets/$TICKET/reserve" 409 '{"branch_id":"branch-achrafieh"}' conflict
expect "$AGENT" POST "/api/v1/enquiries/$ENQ/close" 409 '{"outcome":"available","branch_id":"branch-achrafieh"}' conflict

# --- photos (C types the 625mg code: a different product -> mismatch)
photo "$AGENT" "$TB" 6280000000001 2027-08; check "agent cannot upload a photo" "$STATUS" "403"
photo "$STAFF_C" "$TB" 6280000000001 2027-08; check "other branch cannot upload a photo" "$STATUS" "403"
photo "$STAFF_B" "$TB" 6280000000001 2027-08; check "B photo upload" "$STATUS" "201"
check "B photo status" "$(py 'd["status"]')" "verified"
PB=$(py 'd["id"]')
photo "$STAFF_C" "$TC" 6280000000002 2027-03; check "C photo upload" "$STATUS" "201"
check "C photo status" "$(py 'd["status"]')" "mismatch"
PC=$(py 'd["id"]')
expect "$AGENT" GET "/api/v1/enquiries/$ENQ" 200
check "agent sees photos" "$(py 'len(d["photos"])')" "2"
check "wake photo" "$(py 'd["wake_photo_id"]')" "$PC"
expect "$AGENT" POST "/api/v1/photos/$PB/agent-reading" 200 '{"code":"6280000000001","expiry_month":"2027-08","confidence":0.92}'
check "agent reading agrees -> still verified" "$(py 'd["status"]')" "verified"
expect "$STAFF_B" POST "/api/v1/photos/$PB/agent-reading" 403 '{"code":"x","confidence":0.9}' forbidden
expect "$STAFF_B" POST "/api/v1/photos/$PC/review" 403 '{"status":"verified"}' forbidden
expect "$CLIENT" GET "/api/v1/photos/$PB/file" 200
expect "$AGENT" GET "/api/v1/photos/$PB/file" 200
expect "$STAFF_A" GET "/api/v1/photos/$PB/file" 404 '' not_found

# --- best offer + reserve
expect "$CLIENT" GET "/api/v1/client/tickets/$TICKET" 200
check "best offer branch" "$(py 'd["best_offer"]["branch_id"]')" "branch-achrafieh"
check "best offer total" "$(py 'd["best_offer"]["total_cents"]')" "2400"
check "C excluded reason" "$(py "[x['reason'] for x in d['best_offer']['excluded'] if x['branch_id']=='branch-baabda'][0]")" "Photo did not match the government reference"
check "ticket no longer working" "$(py 'd["working"]')" "False"
expect "$CLIENT" POST "/api/v1/client/tickets/$TICKET/reserve" 409 '{"branch_id":"branch-baabda"}' conflict
expect "$CLIENT" POST "/api/v1/client/tickets/$TICKET/reserve" 200 '{"branch_id":"branch-achrafieh"}'
check "ticket reserved" "$(py 'd["status"]')" "reserved"
expect "$AGENT" GET "/api/v1/enquiries/$ENQ" 200
check "enquiry closed" "$(py 'd["enquiry"]["status"]+"/"+d["enquiry"]["outcome"]+"/"+d["enquiry"]["outcome_branch_id"]')" "closed/available/branch-achrafieh"
check "hold notification queued to B" "$(py "len([m for m in d['messages'] if m['kind']=='notification' and m['target_id']=='$TB'])")" "1"
expect "$CLIENT" GET "/api/v1/client/tickets/$TICKET" 200
check "last thread rows" "$(py '",".join(r["kind"] for r in d["thread"][-2:])')" "reserved,wassit_note"
expect "$CLIENT" POST "/api/v1/client/tickets/$TICKET/reserve" 409 '{"branch_id":"branch-achrafieh"}' conflict
call "$CLIENT" POST /api/v1/client/auth/login '{"phone":"+96170000000","pin":"1234"}'
expect "$STAFF_B" GET "/api/v1/client/tickets/$TICKET" 403 '' forbidden
STREAM=$(curl -sS -N -m 3 "$BASE/api/v1/client/tickets/$TICKET/stream?access_token=${CLIENT#Bearer }" 2>/dev/null)
if [[ "$STREAM" == *"event: ticket_event"* ]]; then ok "ticket stream sends ticket_event"; else BODY="$STREAM"; bad "ticket stream"; fi

# --- photo review (desk reviewer) on a flagged photo of a second ticket is covered by smoke.sh

# --- schedules
expect "$CLIENT" POST /api/v1/client/schedules 400 '{"medication_id":"med-concor-5","quantity":1,"every":"daily","weekday":5,"pickup_window":"Tomorrow morning"}' validation_failed
expect "$CLIENT" POST /api/v1/client/schedules 201 '{"medication_id":"med-concor-5","quantity":1,"every":"week","weekday":5,"pickup_window":"Tomorrow morning"}'
SCHED=$(py 'd["id"]')
check "next run is a Friday 09:00 Beirut" "$(py '__import__("datetime").datetime.fromisoformat(d["next_run_at"].replace("Z","+00:00")).astimezone(__import__("datetime").timezone(__import__("datetime").timedelta(hours=2))).strftime("%a %H") in ("Fri 09","Fri 10")')" "True"
expect "$CLIENT" POST "/api/v1/client/schedules/$SCHED/run" 201
STICKET=$(py 'd["id"]')
check "scheduled ticket mode" "$(py 'd["mode"]')" "scheduled"
expect "$CLIENT" POST "/api/v1/client/schedules/$SCHED/run" 409 '' conflict
expect "$AGENT" GET /api/v1/agent/wakeups 200
check "wake scheduled" "$(py "[w['trigger'] for w in d['data'] if w['enquiry_id']!='$ENQ'][-1]")" "scheduled"
expect "$CLIENT" PATCH "/api/v1/client/schedules/$SCHED" 200 '{"status":"paused"}'
check "schedule paused" "$(py 'd["status"]')" "paused"
expect "$CLIENT" GET /api/v1/client/schedules 200
check "schedule lists its ticket" "$(py "'$STICKET' in d['data'][0]['ticket_ids']")" "True"
call "$CLIENT" GET "/api/v1/client/tickets/$STICKET"; SENQ=$(py 'd["ticket"]["enquiry_id"]')
expect "$AGENT" POST /api/v1/agent/runs 201 "{\"enquiry_id\":\"$SENQ\",\"trigger\":\"scheduled\",\"model\":\"fake\"}"
expect "$DESK" POST "/api/v1/enquiries/$SENQ/close" 200 '{"outcome":"cancelled"}'

echo "== $PASS passed, $FAIL failed"
[ "$FAIL" = 0 ]
