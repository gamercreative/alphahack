#!/usr/bin/env bash
# Switch the Wassit agent runner between the local model and the Claude API, then run it.
#   scripts/agent_model.sh local                          # Ollama qwen3.5:35b-a3b on the LAN host
#   ANTHROPIC_API_KEY=sk-... scripts/agent_model.sh claude  # Claude (default claude-haiku-5-5)
#   CLAUDE_MODEL=claude-sonnet-5-5 ANTHROPIC_API_KEY=... scripts/agent_model.sh claude
# Stops any running agent/runner.py first, then runs the runner in the foreground (Ctrl-C stops it).
# The key is read from the environment only; it is never written to disk by this script.
# BACKEND_URL defaults to the live demo backend; AGENT_TOKEN defaults to the running backend's token.
set -euo pipefail
MODE="${1:-}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export BACKEND_URL="${BACKEND_URL:-http://localhost:8113}"

if [ -z "${AGENT_TOKEN:-}" ]; then
  PORT="${BACKEND_URL##*:}"; PORT="${PORT%%/*}"
  SERVER_PID="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | head -1 || true)"
  if [ -n "$SERVER_PID" ]; then
    AGENT_TOKEN="$(ps eww -p "$SERVER_PID" | tr ' ' '\n' | sed -n 's/^AGENT_TOKEN=//p' | head -1)"
  fi
fi
[ -n "${AGENT_TOKEN:-}" ] || { echo "AGENT_TOKEN not set and not found on the backend at $BACKEND_URL" >&2; exit 1; }
export AGENT_TOKEN

case "$MODE" in
  local)
    export CORNILIA_PROVIDER=ollama
    export CORNILIA_BASE_URL="${OLLAMA_URL:-http://10.31.216.24:11434}/v1"
    export CORNILIA_API_KEY=ollama
    export CORNILIA_MODEL="${LOCAL_MODEL:-qwen3.5:35b-a3b}"
    export CORNILIA_MAX_TOKENS="${CORNILIA_MAX_TOKENS:-1024}"
    ;;
  claude)
    [ -n "${ANTHROPIC_API_KEY:-}" ] || { echo "Set ANTHROPIC_API_KEY first (it stays in your shell only)." >&2; exit 1; }
    export CORNILIA_PROVIDER=openai
    export CORNILIA_BASE_URL=https://api.anthropic.com/v1/
    export CORNILIA_API_KEY="$ANTHROPIC_API_KEY"
    export CORNILIA_MODEL="${CLAUDE_MODEL:-claude-haiku-5-5}"
    export CORNILIA_MAX_TOKENS="${CORNILIA_MAX_TOKENS:-2048}"
    ;;
  *)
    echo "usage: scripts/agent_model.sh local|claude" >&2
    exit 2
    ;;
esac
export CORNILIA_TIMEOUT="${CORNILIA_TIMEOUT:-120}"
export WASSIT_VISION_URL="${WASSIT_VISION_URL:-${OLLAMA_URL:-http://10.31.216.24:11434}}"

for pid in $(pgrep -f "runner.py" || true); do
  if lsof -a -p "$pid" -d cwd -Fn 2>/dev/null | grep -q "$ROOT/agent"; then
    echo "stopping running agent runner (pid $pid)"
    kill "$pid" 2>/dev/null || true
  fi
done
sleep 1

echo "agent runner: model=$CORNILIA_MODEL provider=$CORNILIA_PROVIDER backend=$BACKEND_URL (photos: gemma4:31b on the LAN host)"
cd "$ROOT/agent"
exec .venv/bin/python runner.py
