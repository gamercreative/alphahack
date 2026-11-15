#!/usr/bin/env bash
# Run ALL cases (agent/cases/cases.jsonl) on the real model on a throwaway stack and write a scored table.
#   scripts/eval20.sh            results in .dev/eval20/result.md
# Starts a throwaway Postgres (port 5594) + backend (port 8127, AI_PROVIDER=fake), runs the case with
# the real agent (Cornilia + Ollama), and prints every enquiry event (with its decider), every agent
# API call, every offer the rules decided, and the final outcome. Ctrl-C stops and cleans up.
# Never touches the live demo (8113 / 5573).
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${EVAL_DIR:-$ROOT/.dev/eval20}"
mkdir -p "$OUT"
DBPORT=5595 PORT=8128 DB=wassit-eval20-db
export BACKEND_URL="http://localhost:$PORT" AGENT_TOKEN="watch-token"
export CORNILIA_PROVIDER="${CORNILIA_PROVIDER:-ollama}" CORNILIA_BASE_URL="${CORNILIA_BASE_URL:-http://10.31.216.24:11434/v1}"
export CORNILIA_API_KEY="${CORNILIA_API_KEY:-ollama}" CORNILIA_MODEL="${CORNILIA_MODEL:-qwen3.5:35b-a3b}"
export CORNILIA_TIMEOUT="${CORNILIA_TIMEOUT:-120}" CORNILIA_MAX_TOKENS="${CORNILIA_MAX_TOKENS:-1024}"

SERVER_PID="" CASES_PID=""
cleanup() {
  [ -n "$CASES_PID" ] && kill "$CASES_PID" 2>/dev/null
  pkill -f "runner.py --once" -P "$CASES_PID" 2>/dev/null
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null
  lsof -tiTCP:$PORT -sTCP:LISTEN | xargs kill 2>/dev/null
  docker rm -f "$DB" >/dev/null 2>&1
  echo "cleaned up (db $DB, backend :$PORT)"
}
trap cleanup EXIT INT TERM

echo "== model: $CORNILIA_MODEL at $CORNILIA_BASE_URL   all cases"
docker rm -f "$DB" >/dev/null 2>&1
docker run --rm -d --name "$DB" -e POSTGRES_USER=alphahack -e POSTGRES_PASSWORD=alphahack -e POSTGRES_DB=alphahack -p $DBPORT:5432 postgres:16 >/dev/null
until docker exec "$DB" pg_isready -U alphahack >/dev/null 2>&1; do sleep 1; done
sleep 1
for f in "$ROOT"/backend/db/migrations/*.up.sql; do
  docker exec -i "$DB" psql -U alphahack -d alphahack -v ON_ERROR_STOP=1 -q < "$f" >/dev/null || { echo "migration failed: $f"; exit 1; }
done
echo "== db ready, migrations applied"
(cd "$ROOT/backend" && PORT=$PORT DATABASE_URL="postgres://alphahack:alphahack@localhost:$DBPORT/alphahack?sslmode=disable" \
  AI_PROVIDER=fake UNDO_WINDOW=2s WAIT_UNIT=1s AGENT_TOKEN=$AGENT_TOKEN CORS_ALLOWED_ORIGINS='*' \
  go run ./cmd/server > "$OUT/server.log" 2>&1) &
SERVER_PID=$!
until curl -sf "localhost:$PORT/healthz" >/dev/null; do sleep 1; done
echo "== backend up on :$PORT"

python3.13 "$ROOT/scripts/run_cases.py" --policy model --max-rounds 25 --output "$OUT/result.md" 2>&1 | tee "$OUT/cases.log"
