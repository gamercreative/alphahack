#!/usr/bin/env bash
# Run every offline check in order and print a pass/fail table.
#   bash scripts/verify_all.sh
#   BACKEND_URL=http://localhost:8080 bash scripts/verify_all.sh   also run the read-only live checks
#   BACKEND_URL=... RUN_SMOKE=1 bash scripts/verify_all.sh         also run backend/scripts/smoke.sh (WRITES data)
# Offline checks need no database, no model and no network. Exit code 0 = no FAIL rows.
# Full output of each check is kept in .dev/logs/verify/<name>.log.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOGS="$ROOT/.dev/logs/verify"
mkdir -p "$LOGS"
PY="$ROOT/agent/.venv/bin/python"
NAMES=(); RESULTS=(); NOTES=()

record() { NAMES+=("$1"); RESULTS+=("$2"); NOTES+=("$3"); printf '%-5s %s %s\n' "$2" "$1" "$3"; }

# run NAME DIR COMMAND...: runs the command in DIR, records PASS or FAIL.
run() {
  local name="$1" dir="$2"; shift 2
  local log="$LOGS/$name.log" start=$SECONDS
  if (cd "$dir" && "$@") >"$log" 2>&1; then
    record "$name" PASS "($((SECONDS - start))s)"
  else
    record "$name" FAIL "see .dev/logs/verify/$name.log"
  fi
}
skip() { record "$1" SKIP "$2"; }
have() { command -v "$1" >/dev/null 2>&1; }

echo "== offline checks =="
if have go; then
  run backend-build   "$ROOT/backend" go build ./...
  run backend-vet     "$ROOT/backend" go vet ./...
  run backend-test    "$ROOT/backend" go test ./...
  run guard-go-test   "$ROOT/cornilia-guard/go" go test ./...
else
  skip backend-build "go not installed"; skip backend-vet "go not installed"
  skip backend-test "go not installed";  skip guard-go-test "go not installed"
fi
run backend-queries "$ROOT/backend" ./scripts/check_queries.sh
if [ -x "$PY" ]; then
  run agent-pytest    "$ROOT/agent" "$PY" -m pytest -q
  run guard-py-pytest "$ROOT/cornilia-guard/python" "$PY" -m pytest -q
  run repo-pytest     "$ROOT" "$PY" -m pytest -q tests
else
  skip agent-pytest "agent/.venv missing"; skip guard-py-pytest "agent/.venv missing"
  skip repo-pytest "agent/.venv missing"
fi
if have python3.13; then
  run frontend-sync-static "$ROOT" python3.13 scripts/check_frontend_sync.py --static
else
  skip frontend-sync-static "python3.13 not installed"
fi
shell_syntax() { local f rc=0; for f in scripts/*.sh backend/scripts/*.sh; do bash -n "$f" || rc=1; done; return $rc; }
run shell-syntax "$ROOT" shell_syntax
if have node; then
  js_syntax() { local f rc=0; for f in web/*.js; do node --check "$f" || rc=1; done; return $rc; }
  run web-js-syntax "$ROOT" js_syntax
else
  skip web-js-syntax "node not installed"
fi

echo "== live checks =="
if [ -n "${BACKEND_URL:-}" ]; then
  run live-seed-check    "$ROOT" env BACKEND_URL="$BACKEND_URL" PYTHON=python3.13 bash scripts/seed_demo.sh
  run live-frontend-sync "$ROOT" python3.13 scripts/check_frontend_sync.py --backend "$BACKEND_URL"
  if [ "${RUN_SMOKE:-}" = 1 ]; then
    run live-smoke "$ROOT/backend" env BASE_URL="$BACKEND_URL" ./scripts/smoke.sh
  else
    skip live-smoke "writes data; set RUN_SMOKE=1 to include"
  fi
else
  skip live-seed-check "BACKEND_URL not set"; skip live-frontend-sync "BACKEND_URL not set"
  skip live-smoke "BACKEND_URL not set"
fi

echo
printf '%-22s %-6s %s\n' CHECK RESULT NOTE
printf '%-22s %-6s %s\n' ---------------------- ------ ----
fails=0
for i in "${!NAMES[@]}"; do
  printf '%-22s %-6s %s\n' "${NAMES[$i]}" "${RESULTS[$i]}" "${NOTES[$i]}"
  [ "${RESULTS[$i]}" = FAIL ] && fails=$((fails + 1))
done
echo
if [ "$fails" = 0 ]; then echo "verify_all: no failures"; else echo "verify_all: $fails check(s) failed"; fi
[ "$fails" = 0 ]
