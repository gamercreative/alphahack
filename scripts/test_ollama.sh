#!/usr/bin/env bash
# Tests the LAN Ollama box through both paths the Wassit pipeline uses:
#   1. Go backend  -> native  POST /api/chat            (OLLAMA_BASE_URL, no /v1)
#   2. Cornilia    -> OpenAI  POST /v1/chat/completions (CORNILIA_BASE_URL, ends in /v1)
#   3. The judgement the agent must get right: an Arabizi "alternative" reply is NOT a match.
# Usage: bash scripts/test_ollama.sh            (defaults below)
#        MODEL=gemma4:31b bash scripts/test_ollama.sh
set -uo pipefail

OLLAMA="${OLLAMA:-http://10.31.216.24:11434}"
MODEL="${MODEL:-qwen3.5:35b-a3b}"
PY=python3
pass=0; fail=0
ok()  { echo "  PASS  $1"; pass=$((pass+1)); }
bad() { echo "  FAIL  $1"; fail=$((fail+1)); }

SYS='You are Wassit, an admin coordinator for pharmacists. Use tools to find exact medicines. Never substitute a medicine.'
TOOLS='[{"type":"function","function":{"name":"search_medications","description":"Search the medicine catalogue. Returns up to 5 exact products.","parameters":{"type":"object","properties":{"query":{"type":"string","description":"Brand or active ingredient"},"form":{"type":"string","enum":["tablet","capsule","syrup","injection"]}},"required":["query"]}}}]'
ASK='Pharmacist request: need augmentin 1g tabs x2'

echo "== 0. Reachability ($OLLAMA, model $MODEL)"
if curl -sf -m 5 "$OLLAMA/api/version" >/dev/null; then ok "server answers: $(curl -s -m 5 "$OLLAMA/api/version")"; else bad "server unreachable"; exit 1; fi
if curl -s -m 5 "$OLLAMA/api/tags" | grep -q "\"name\":\"$MODEL\""; then ok "model $MODEL is pulled"; else bad "model $MODEL not found"; exit 1; fi

echo "== 1. Backend path: native /api/chat with tools (cold load included)"
BODY=$($PY -c "import json,sys;print(json.dumps({'model':'$MODEL','stream':False,'think':False,'options':{'num_ctx':16384},'messages':[{'role':'system','content':'''$SYS'''},{'role':'user','content':'''$ASK'''}],'tools':json.loads('''$TOOLS''')}))")
R=$(curl -s -m 300 -w '\n%{time_total}' "$OLLAMA/api/chat" -H 'Content-Type: application/json' -d "$BODY")
T=$(echo "$R" | tail -1); J=$(echo "$R" | sed '$d')
echo "$J" | $PY -c "
import json,sys
j=json.load(sys.stdin); m=j.get('message',{}); tc=m.get('tool_calls') or []
ld=j.get('load_duration',0)/1e9; ev=j.get('eval_count',0); ed=j.get('eval_duration',1)/1e9
pe=j.get('prompt_eval_count',0); pd=j.get('prompt_eval_duration',1)/1e9
print('  tool_calls:', json.dumps([c['function'] for c in tc]) or 'none')
print(f'  load {ld:.1f}s | prompt {pe} tok @ {pe/pd:.0f} tok/s | gen {ev} tok @ {ev/ed:.1f} tok/s | wall $T s')
sys.exit(0 if tc and tc[0]['function']['name']=='search_medications' else 1)
" && ok "native tool call" || bad "native tool call (see output above)"

echo "== 2. Cornilia path: /v1/chat/completions with tools (model now warm)"
BODY=$($PY -c "import json;print(json.dumps({'model':'$MODEL','max_tokens':512,'messages':[{'role':'system','content':'''$SYS'''},{'role':'user','content':'''$ASK'''}],'tools':json.loads('''$TOOLS''')}))")
R=$(curl -s -m 300 -w '\n%{time_total}' "$OLLAMA/v1/chat/completions" -H 'Content-Type: application/json' -d "$BODY")
T=$(echo "$R" | tail -1); J=$(echo "$R" | sed '$d')
echo "$J" | $PY -c "
import json,sys
j=json.load(sys.stdin); c=j['choices'][0]; tc=c['message'].get('tool_calls') or []
print('  finish:', c.get('finish_reason'), '| tool_calls:', json.dumps([t['function'] for t in tc]) or 'none')
print('  usage:', j.get('usage'), '| wall $T s')
sys.exit(0 if tc else 1)
" && ok "OpenAI-compatible tool call" || bad "OpenAI-compatible tool call"

echo "== 3. Judgement: Arabizi alternative reply must NOT count as a match (3 runs)"
for i in 1 2 3; do
BODY=$($PY -c "import json;print(json.dumps({'model':'$MODEL','stream':False,'think':False,'format':{'type':'object','properties':{'counts_as_match':{'type':'boolean'},'classification':{'type':'string','enum':['exact','alternative','unavailable','unclear']},'note_for_requester':{'type':'string'}},'required':['counts_as_match','classification','note_for_requester']},'messages':[{'role':'system','content':'''$SYS Classify a pharmacy reply to an enquiry. Reply JSON only.'''},{'role':'user','content':'Requested: Augmentin 1g tablet x2. Pharmacy C replied: \"eh fi 3ena augmentin bas 625 mg, 3 3elab\"'}]}))")
curl -s -m 300 "$OLLAMA/api/chat" -H 'Content-Type: application/json' -d "$BODY" | $PY -c "
import json,sys
a=json.loads(json.load(sys.stdin)['message']['content'])
print('  run $i:', a)
sys.exit(0 if (not a['counts_as_match']) and a['classification']=='alternative' else 1)
" && ok "run $i classified as alternative" || bad "run $i misclassified"
done

echo "== 4. End to end: Wassit runner (Cornilia -> $OLLAMA) on the live backend"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if ! curl -sf -m 3 "${BACKEND_URL:-http://localhost:8080}/healthz" >/dev/null; then
  echo "  SKIP  backend not running (cd backend && make docker-up && bash scripts/seed_demo.sh), then rerun"
elif [ ! -x "$ROOT/agent/.venv/bin/python" ]; then
  echo "  SKIP  agent/.venv missing (see agent/README.md steps 1-2), then rerun"
else
  ( export CORNILIA_PROVIDER=ollama CORNILIA_BASE_URL="$OLLAMA/v1" CORNILIA_MODEL="$MODEL" \
           CORNILIA_TIMEOUT=300 CORNILIA_MAX_TOKENS=2048 CORNILIA_TRACE=1 WASSIT_POLICY=model \
           PATH="$ROOT/agent/.venv/bin:$PATH"
    cd "$ROOT" && python3.13 scripts/demo_flow.py --no-pause --drive-agent ) \
    && ok "demo flow with the real model" || bad "demo flow with the real model (read the trace above)"
fi

echo
echo "== Result: $pass passed, $fail failed"
echo "Pipeline settings for this box:"
echo "  agent (Cornilia): CORNILIA_BASE_URL=$OLLAMA/v1 CORNILIA_MODEL=$MODEL CORNILIA_TIMEOUT=300"
echo "  backend on host : OLLAMA_BASE_URL=$OLLAMA OLLAMA_MODEL=$MODEL OLLAMA_NUM_CTX=16384 (in backend/.env)"
echo "  backend in Docker: export DOCKER_OLLAMA_BASE_URL=$OLLAMA before 'make docker-up'"
