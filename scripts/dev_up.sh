#!/usr/bin/env bash
# Start a local DOSYN / Wassit stack on the DEFAULT ports with one command.
#   Postgres 5432 (Docker)  ->  migrations  ->  Go backend 8080  ->  static web 5500
#
#   bash scripts/dev_up.sh               db + migrations + backend + web
#   bash scripts/dev_up.sh --no-web      skip the static web server
#   bash scripts/dev_up.sh --with-agent  also start agent/runner.py (needs agent/.venv)
#
# State lives in .dev/ (git-ignored): pid files in .dev/*.pid, logs in .dev/logs/.
# Stop everything this script started with: bash scripts/dev_down.sh
#
# Environment (all optional; real values win over the defaults below):
#   AI_PROVIDER    default fake (no model needed)      UNDO_WINDOW  default 60s
#   AGENT_TOKEN    default dev-agent-token             WAIT_UNIT    default 1m
#   WASSIT_POLICY  agent policy with --with-agent: model | scripted (default scripted)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEV="$ROOT/.dev"
LOGS="$DEV/logs"
BACKEND_PORT=8080
WEB_PORT=5500
DB_URL="postgres://alphahack:alphahack@localhost:5432/alphahack?sslmode=disable"
WITH_WEB=1
WITH_AGENT=0

for arg in "$@"; do
  case "$arg" in
    --no-web) WITH_WEB=0 ;;
    --with-agent) WITH_AGENT=1 ;;
    -h|--help) sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "dev_up: unknown flag $arg (try --help)" >&2; exit 2 ;;
  esac
done

need() { command -v "$1" >/dev/null 2>&1 || { echo "dev_up: missing '$1'. See docs/REPRODUCE.md, Prerequisites." >&2; exit 1; }; }
busy() { lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }

need docker; need go; need curl; need lsof
[ "$WITH_WEB" = 0 ] || need python3.13

if busy "$BACKEND_PORT"; then
  echo "dev_up: port $BACKEND_PORT is busy. Stop that process (or run scripts/dev_down.sh) and retry." >&2; exit 1
fi
if [ "$WITH_WEB" = 1 ] && busy "$WEB_PORT"; then
  echo "dev_up: port $WEB_PORT is busy. Stop that process, or pass --no-web." >&2; exit 1
fi

mkdir -p "$LOGS"

echo "[1/5] PostgreSQL 16 on :5432 (docker compose, waits until healthy)"
(cd "$ROOT/backend" && docker compose up -d --wait db)
touch "$DEV/db.started"

echo "[2/5] Migrations (through the db container, so no host psql is needed)"
(cd "$ROOT/backend" && env -u DATABASE_URL ./scripts/migrate.sh up)

echo "[3/5] Backend on :$BACKEND_PORT (log: .dev/logs/backend.log)"
(cd "$ROOT/backend" && go build -o "$DEV/server" ./cmd/server)
(
  cd "$ROOT/backend"
  export PORT="$BACKEND_PORT" DATABASE_URL="$DB_URL"
  export AI_PROVIDER="${AI_PROVIDER:-fake}" AGENT_TOKEN="${AGENT_TOKEN:-dev-agent-token}"
  nohup "$DEV/server" >"$LOGS/backend.log" 2>&1 &
  echo $! >"$DEV/backend.pid"
)
ok=0
for _ in $(seq 1 30); do
  if curl -fsS -m 2 "http://localhost:$BACKEND_PORT/healthz" >/dev/null 2>&1; then ok=1; break; fi
  if ! kill -0 "$(cat "$DEV/backend.pid")" 2>/dev/null; then break; fi
  sleep 1
done
if [ "$ok" != 1 ]; then
  echo "dev_up: backend did not become healthy. Last log lines:" >&2
  tail -n 20 "$LOGS/backend.log" >&2 || true
  exit 1
fi

if [ "$WITH_WEB" = 1 ]; then
  echo "[4/5] Web on :$WEB_PORT (log: .dev/logs/web.log)"
  nohup python3.13 -m http.server "$WEB_PORT" --directory "$ROOT/web" >"$LOGS/web.log" 2>&1 &
  echo $! >"$DEV/web.pid"
else
  echo "[4/5] Web skipped (--no-web)"
fi

if [ "$WITH_AGENT" = 1 ]; then
  PY="$ROOT/agent/.venv/bin/python"
  if [ -x "$PY" ]; then
    echo "[5/5] Agent runner (log: .dev/logs/agent.log)"
    (
      cd "$ROOT/agent"
      export BACKEND_URL="http://localhost:$BACKEND_PORT" AGENT_TOKEN="${AGENT_TOKEN:-dev-agent-token}"
      export WASSIT_POLICY="${WASSIT_POLICY:-scripted}"
      nohup "$PY" runner.py >"$LOGS/agent.log" 2>&1 &
      echo $! >"$DEV/agent.pid"
    )
  else
    echo "[5/5] Agent skipped: agent/.venv is missing. Create it as in docs/REPRODUCE.md, step 5." >&2
  fi
else
  echo "[5/5] Agent skipped (pass --with-agent to start it)"
fi

cat <<MSG

Stack is up.
  Backend   http://localhost:$BACKEND_PORT/healthz
  Web       http://localhost:$WEB_PORT/#client/home $([ "$WITH_WEB" = 1 ] || echo "(not started)")
  Logs      $LOGS
  Stop      bash scripts/dev_down.sh

Note: web/index.html sets window.VITE_API_BASE_URL. If it points at another host,
change that one line to http://localhost:$BACKEND_PORT for a fully local run.
Demo logins: a@wassit.demo .. e@wassit.demo, PIN 1234.
MSG
