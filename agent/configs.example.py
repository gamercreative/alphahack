# Copy to agent/configs.py (gitignored). Cornilia reads these names; configs.py wins over env vars.
from cornilia.providers import ProviderConfigs

PROVIDER_CONFIGS = ProviderConfigs(
    name="ollama",
    base_url="http://localhost:11434/v1",
    api_key="ollama",
    timeout=300,
)
MODEL_NAME = "qwen3.5:35b-a3b"
MODEL_RESPONSE_CONFIG = {"max_tokens": 1024, "reasoning_effort": "none"}
TOOLS_DIR = "tools"
MAX_STEPS = 12
