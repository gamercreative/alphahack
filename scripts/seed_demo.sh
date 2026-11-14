#!/usr/bin/env bash
# Wassit demo check: backend health, the four demo logins, and stats totals.
# Read-only: it logs in and reads. It never deletes or writes data.
#
#   bash scripts/seed_demo.sh
#   BACKEND_URL=http://localhost:8080 bash scripts/seed_demo.sh
#
# Environment:
#   BACKEND_URL   backend base URL (default http://localhost:8080)
#   PYTHON        python used to read JSON replies (default python3)
set -euo pipefail

BACKEND_URL="${BACKEND_URL:-http://localhost:8080}"
BACKEND_URL="${BACKEND_URL%/}"
PYTHON="${PYTHON:-python3}"
PIN="1234"
DEMO_USERS="a b c d"

# jget KEY.PATH: reads a JSON document on stdin, prints the value ("" when missing).
jget() {
  "$PYTHON" -c '
import json, sys
v = json.load(sys.stdin)
for k in sys.argv[1].split("."):
    v = v.get(k) if isinstance(v, dict) else None
print("" if v is None else v)
' "$1"
}

# login EMAIL: POST /api/v1/auth/login. Sets RESP_STATUS and RESP_BODY.
login() {
  local resp
  resp=$(curl -sS --max-time 10 -X POST "$BACKEND_URL/api/v1/auth/login" \
    -H 'Content-Type: application/json' -H 'Accept: application/json' \
    -d "{\"email\":\"$1\",\"pin\":\"$PIN\"}" -w $'\n%{http_code}') || resp=$'\n000'
  RESP_STATUS="${resp##*$'\n'}"
  RESP_BODY="${resp%$'\n'*}"
}

# 1. backend health
if ! curl -fsS --max-time 5 "$BACKEND_URL/healthz" >/dev/null 2>&1; then
  cat <<'EOF'
Backend is not healthy (GET /healthz failed).

Start it from the project root, in terminal 2:

  cd backend && make up && make migrate

Then start the server with the demo environment:

  cd backend
  UNDO_WINDOW=3s WAIT_UNIT=2s AGENT_TOKEN=dev-agent-token AGENT_PRICE_IN_PER_MTOK=0.2 AGENT_PRICE_OUT_PER_MTOK=0.8 make run

Then run this script again.
EOF
  exit 1
fi
echo "backend healthy at $BACKEND_URL"

# 2. demo logins (PIN is the demo PIN; tokens are never printed)
printf '\n%-32s %-22s %-6s %s\n' "PHARMACY" "EMAIL" "PIN" "LOGIN"
failed=0
token_a=""
for who in $DEMO_USERS; do
  email="$who@wassit.demo"
  login "$email"
  if [ "$RESP_STATUS" = "200" ]; then
    name=$(printf '%s' "$RESP_BODY" | jget branch.name)
    printf '%-32s %-22s %-6s %s\n' "$name" "$email" "$PIN" "ok"
    if [ "$who" = "a" ]; then
      token_a=$(printf '%s' "$RESP_BODY" | jget token)
    fi
  else
    printf '%-32s %-22s %-6s %s\n' "-" "$email" "$PIN" "FAILED (HTTP $RESP_STATUS)"
    failed=1
  fi
done

# 3. stats totals (staff route, requester A)
echo
if [ -z "$token_a" ]; then
  echo "stats: skipped, no token for a@wassit.demo"
  exit 1
fi
stats=$(curl -sS --max-time 10 -H "Authorization: Bearer $token_a" \
  -H 'Accept: application/json' "$BACKEND_URL/api/v1/stats?limit=200") || stats="{}"
printf '%s' "$stats" | "$PYTHON" -c '
import json, sys
t = (json.load(sys.stdin) or {}).get("totals") or {}
micros = t.get("cost_micros", 0) or 0
print("stats totals:")
for key in ("enquiries", "agent_enquiries", "hitl_count", "agent_runs", "agent_steps"):
    print(f"  {key:<17} {t.get(key, 0)}")
print("  " + "cost_usd".ljust(17) + f" {micros / 1_000_000:.6f}")
'

if [ "$failed" -ne 0 ]; then
  echo
  echo "one or more logins failed: check that the backend is migrated and seeded (make migrate)"
  exit 1
fi
echo
echo "seed check passed"
