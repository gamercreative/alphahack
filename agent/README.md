# Wassit agent

Runs the enquiry agent: polls the backend for wakeups and runs one Cornilia loop per enquiry.

1. `cd agent && python3.13 -m venv .venv && source .venv/bin/activate`
2. `pip install "cornilia @ git+https://github.com/gamercreative/Cornilia.git@v0.1.0" pytest`
3. Optional: `cp configs.example.py configs.py` to override the Cornilia settings.
4. Start Ollama with `qwen3.5:35b-a3b` pulled, and the backend on port 8080.
5. Env vars: `BACKEND_URL` (default `http://localhost:8080`), `AGENT_TOKEN` (default `dev-agent-token`).
6. Optional Cornilia env: `CORNILIA_MODEL=qwen3.5:35b-a3b`, `CORNILIA_MAX_STEPS=12`, `CORNILIA_TRACE=1`.
7. Loop forever: `python runner.py`
8. Process the current wakeups once: `python runner.py --once`
9. Tests (no network, no model): `python -m pytest -q tests/test_runner.py`
10. `CORNILIA_TOOLS_DIR` defaults to `agent/tools`; `prompts/system.md` is the system prompt.
