import os

# WirePod Configuration
WIREPOD_IP = os.environ.get("WIREPOD_IP", "192.168.1.100") # Default if not set in ENV
# ROBOT_ESN can be set here if you typically control one specific robot,
# otherwise, it can be specified in the API call to start autonomous mode.
# If None, the vector_controller will attempt to use the first robot it finds from WirePod's /api/get_sdk_info
ROBOT_ESN = os.environ.get("ROBOT_ESN", None)

# LLM Configuration
# LLM_PROVIDER can be "openai_compatible" (for local servers like Llamafile, Ollama's OpenAI API, etc.)
# or "openai_direct" (for actual OpenAI API - not fully implemented in handler for this yet but planned)
# or "custom" (if you have a different LLM interaction logic)
LLM_PROVIDER = os.environ.get("LLM_PROVIDER", "openai_compatible")

LLM_API_BASE_URL = os.environ.get("LLM_API_BASE_URL", "http://localhost:8081/v1") # e.g., for Llamafile
# For OpenAI direct, it would be "https://api.openai.com/v1"
# For Ollama with OpenAI compatibility: "http://localhost:11434/v1" (or your Ollama port)

LLM_API_KEY = os.environ.get("LLM_API_KEY", "sk-no-key-required") # For local, often not needed. For OpenAI, your actual key.

LLM_MODEL_NAME = os.environ.get("LLM_MODEL_NAME", "llamafile-default") # Adjust to your specific model identifier
# For OpenAI: "gpt-4o-mini", "gpt-4-vision-preview", etc.
# For Ollama: "llava:latest", "llama3:latest", etc. (ensure the model is pulled and running)


# VectorBrain Server Configuration
VECTOR_BRAIN_SERVER_HOST = os.environ.get("VECTOR_BRAIN_SERVER_HOST", "0.0.0.0")
VECTOR_BRAIN_SERVER_PORT = int(os.environ.get("VECTOR_BRAIN_SERVER_PORT", 5001))

# Autonomous Loop Configuration
# Determine the base path for relative paths (assuming config.py is in VectorBrain root)
_BASE_DIR = os.path.dirname(os.path.abspath(__file__))
DEFAULT_SYSTEM_PROMPT_PATH = os.environ.get(
    "DEFAULT_SYSTEM_PROMPT_PATH",
    os.path.join(_BASE_DIR, "autonomous_loop", "default_system_prompt.txt")
)

MAX_LOOP_ITERATIONS = int(os.environ.get("MAX_LOOP_ITERATIONS", 30))
LOOP_DELAY_SECONDS = float(os.environ.get("LOOP_DELAY_SECONDS", 0.5))


# --- How to use this config.py ---
# 1. Set environment variables for any settings you want to override.
# 2. Or, directly modify the default values in this file.
# 3. The application modules (server/main.py, autonomous_loop/loop.py, llm/handler.py)
#    will need to be updated to import and use these settings from `config`.

# Example of how other modules would import and use this:
# from config import WIREPOD_IP, LLM_API_BASE_URL
# print(f"Connecting to WirePod at {WIREPOD_IP}")

if __name__ == '__main__':
    # Print out current configuration for verification
    print("--- VectorBrain Configuration ---")
    print(f"WIREPOD_IP: {WIREPOD_IP}")
    print(f"ROBOT_ESN: {ROBOT_ESN if ROBOT_ESN else 'Not set (will try auto-detect or require in API call)'}")
    print(f"LLM_PROVIDER: {LLM_PROVIDER}")
    print(f"LLM_API_BASE_URL: {LLM_API_BASE_URL}")
    print(f"LLM_API_KEY: {'********' if LLM_API_KEY and LLM_API_KEY != 'sk-no-key-required' else LLM_API_KEY}")
    print(f"LLM_MODEL_NAME: {LLM_MODEL_NAME}")
    print(f"VECTOR_BRAIN_SERVER_HOST: {VECTOR_BRAIN_SERVER_HOST}")
    print(f"VECTOR_BRAIN_SERVER_PORT: {VECTOR_BRAIN_SERVER_PORT}")
    print(f"DEFAULT_SYSTEM_PROMPT_PATH: {DEFAULT_SYSTEM_PROMPT_PATH}")
    print(f"MAX_LOOP_ITERATIONS: {MAX_LOOP_ITERATIONS}")
    print(f"LOOP_DELAY_SECONDS: {LOOP_DELAY_SECONDS}")
    print("-----------------------------")

    if not os.path.exists(DEFAULT_SYSTEM_PROMPT_PATH):
        print(f"Warning: System prompt file not found at the configured path: {DEFAULT_SYSTEM_PROMPT_PATH}")
        print("The autonomous loop might use a very basic internal default if this file is missing.")
```

Now, the other Python modules (`server/main.py`, `autonomous_loop/loop.py`, `llm/handler.py`, `vector_controller/controller.py`) need to be updated to import and use these settings from `config.py` instead of their internal defaults or environment variable lookups.

I'll proceed to update those files.
