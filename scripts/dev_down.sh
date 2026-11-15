#!/usr/bin/env bash
# Stop what scripts/dev_up.sh started, using the pid files in .dev/.
#   bash scripts/dev_down.sh            stop agent, web, backend; stop the db container
#   bash scripts/dev_down.sh --keep-db  leave PostgreSQL running
# Data is kept in the pgdata Docker volume. Processes not started by dev_up are never touched.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEV="$ROOT/.dev"
KEEP_DB=0
[ "${1:-}" = "--keep-db" ] && KEEP_DB=1

stop() {
  local name="$1" file="$DEV/$1.pid" pid
  [ -f "$file" ] || { echo "skip   $name (no pid file)"; return; }
  pid="$(cat "$file")"
  if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null
    echo "stop   $name (pid $pid)"
  else
    echo "gone   $name (pid $pid was not running)"
  fi
  rm -f "$file"
}

stop agent
stop web
stop backend

if [ -f "$DEV/db.started" ]; then
  if [ "$KEEP_DB" = 1 ]; then
    echo "keep   db (--keep-db)"
  else
    (cd "$ROOT/backend" && docker compose stop db) && echo "stop   db (container stopped, data kept)"
    rm -f "$DEV/db.started"
  fi
else
  echo "skip   db (not started by dev_up)"
fi
