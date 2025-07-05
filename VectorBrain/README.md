# VectorBrain - External Control Server for Vector Robot

VectorBrain is a Python-based server designed to run on a local computer, providing advanced processing capabilities and autonomous control for an Anki Vector robot connected via WirePod. It leverages a local Large Language Model (LLM) for decision-making and interacts with Vector through WirePod's web SDK endpoints.

This approach allows for more complex behaviors, integration with external tools and knowledge bases (like ChromaDB), and offloads heavy processing from Vector and the WirePod instance itself.

## High-Level Architecture

1.  **VectorBrain Server (This Project):**
    *   Runs on a separate computer (typically where the LLM is hosted).
    *   Hosts a web server (e.g., Flask/FastAPI) to manage the autonomous loop and potentially offer a control/monitoring UI.
    *   **LLM Interaction Module:** Interfaces with a local LLM to get action commands based on sensory input and a system prompt.
    *   **Vector Control Module:** Sends commands to Vector by making HTTP requests to the WirePod instance's web SDK endpoints.
    *   **Autonomous Loop Logic:** Manages the perception-action cycle (get sensor data, query LLM, parse command, execute command).
    *   **(Future) Tool Integration:** Can be extended to use external tools (web search, calculators).
    *   **(Future) Persistent Memory:** Can integrate with vector stores (ChromaDB) or other databases for long-term memory.

2.  **WirePod Instance:**
    *   Runs on its usual hardware (e.g., Raspberry Pi, server).
    *   Serves as the bridge to Vector.
    *   Exposes web SDK endpoints that `VectorBrain` calls to control Vector and retrieve data (e.g., camera images).
    *   (Future) May have a dedicated, simplified API endpoint for more efficient communication with `VectorBrain`.

3.  **Vector Robot:**
    *   Connects to the WirePod instance.
    *   Executes commands received from WirePod (which are relayed from `VectorBrain`).

## Communication Flow

`LLM (Local)` <-> `VectorBrain Server (Python)` <-> `WirePod (HTTP Web SDK)` <-> `Vector Robot`

## Key Components (in this repository)

*   **`server/`**: Contains the web server application (e.g., Flask/FastAPI) that exposes API endpoints to start/stop autonomous mode and potentially for monitoring.
*   **`llm/`**: Houses the logic for interacting with the chosen local LLM (e.g., loading models, formatting prompts, processing responses).
*   **`vector_controller/`**: Module responsible for communicating with Vector via WirePod's web SDK. It translates internal commands into HTTP requests to WirePod.
*   **`autonomous_loop/`**: Contains the core decision-making loop, command parsing, and orchestration of the different modules.
*   **`static/` & `templates/`**: (Optional) For any web UI associated with the `VectorBrain` server.
*   **`config.py` / `config.ini` / `config.yaml` (To be created):** For configuring WirePod IP address, LLM model paths, API keys, etc.
*   **`requirements.txt`**: Python dependencies.
*   **`README.md`**: This file.

## Setup and Running

1.  **Prerequisites:**
    *   Python 3.8+ installed.
    *   A running WirePod instance accessible on your network, with a Vector robot connected to it.
    *   A local LLM server (e.g., Llamafile, Ollama with an OpenAI-compatible API, or a custom solution) or access to a remote LLM API (like OpenAI).

2.  **Clone the Repository:**
    ```bash
    # Assuming VectorBrain is part of a larger repository structure
    # git clone <repository_url>
    # cd <repository_url>/VectorBrain
    # If VectorBrain is standalone:
    # git clone <vectorbrain_repository_url>
    # cd VectorBrain
    ```

3.  **Install Dependencies:**
    Create a virtual environment (recommended):
    ```bash
    python -m venv venv
    source venv/bin/activate  # On Windows: venv\Scripts\activate
    ```
    Install required packages:
    ```bash
    pip install -r requirements.txt
    ```

4.  **Configuration (`config.py`):**
    *   Create a `VectorBrain/config.py` file (or copy `config.py.example` if provided).
    *   Edit `config.py` to set:
        *   `WIREPOD_IP`: The IP address of your WirePod server.
        *   `ROBOT_ESN`: (Optional) The ESN of the specific Vector you want to control. If not set, the system may try to use the first robot found via WirePod.
        *   `LLM_PROVIDER`: "openai_compatible", "openai_direct" (future), or "custom".
        *   `LLM_API_BASE_URL`: The base URL for your LLM's API (e.g., "http://localhost:8081/v1" for Llamafile, "https://api.openai.com/v1" for OpenAI).
        *   `LLM_API_KEY`: Your LLM API key (e.g., OpenAI key, or "sk-no-key-required" for many local servers).
        *   `LLM_MODEL_NAME`: The specific model name your LLM server uses (e.g., "gpt-4o-mini", "llamafile-default-model").
        *   `VECTOR_BRAIN_SERVER_PORT`: Port for the VectorBrain Flask server (default is 5001).
        *   `DEFAULT_SYSTEM_PROMPT_PATH`: Path to the system prompt file (default: "autonomous_loop/default_system_prompt.txt").
        *   `MAX_LOOP_ITERATIONS`: Default maximum iterations for the autonomous loop.
        *   `LOOP_DELAY_SECONDS`: Delay between loop iterations.

5.  **System Prompt:**
    *   Review and customize `autonomous_loop/default_system_prompt.txt` to tailor Vector's behavior, personality, and available commands.

6.  **Run the VectorBrain Server:**
    Navigate to the `VectorBrain` root directory in your terminal (ensure virtual environment is active).
    ```bash
    python server/main.py
    ```
    The server will start, typically on `http://0.0.0.0:5001` (or the port you configured).

7.  **Control Autonomous Mode:**
    *   **Start:** Send a POST request to `http://<VectorBrain_IP>:<Port>/api/start_autonomous_mode` with a JSON payload:
        ```json
        {
            "robot_esn": "00xxxxxx", // Your Vector's ESN
            "wirepod_ip": "192.168.x.y" // Your WirePod's IP (can be omitted if configured in config.py and matches)
        }
        ```
        You can use tools like `curl` or Postman:
        ```bash
        curl -X POST -H "Content-Type: application/json" \
             -d '{"robot_esn": "00xxxxxx", "wirepod_ip": "192.168.x.y"}' \
             http://localhost:5001/api/start_autonomous_mode
        ```
    *   **Stop:** Send a POST request to `http://<VectorBrain_IP>:<Port>/api/stop_autonomous_mode`:
        ```bash
        curl -X POST http://localhost:5001/api/stop_autonomous_mode
        ```
    *   **Status:** Send a GET request to `http://<VectorBrain_IP>:<Port>/api/autonomous_status`.

## Future Development

*   Implementing robust sensor data retrieval from WirePod (especially efficient camera streaming).
*   Adding tool use capabilities for the LLM.
*   Integrating a persistent memory solution (e.g., ChromaDB).
*   Developing a simple web UI for monitoring and basic control.
*   Creating dedicated API endpoints on WirePod for more streamlined communication.
*   Implementing a more sophisticated state management for Vector within `VectorBrain`.
