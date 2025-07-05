# VectorBrain Server

This server acts as the "brain" for an Anki Vector robot, enabling advanced autonomous capabilities
driven by a Large Language Model (LLM). It runs on a separate computer and communicates with
Vector (running WirePod) via HTTP requests to WirePod's SDK-like endpoints.

## Purpose

- Offload complex processing (LLM inference, tool use, vector databases) from Vector/WirePod.
- Enable richer interactions and decision-making for Vector.
- Provide a platform for integrating various AI tools and services.

## Setup (Placeholder)

- Python 3.9+
- Dependencies: Flask/FastAPI, requests (more to be added)
- Configuration for WirePod endpoint and LLM service.

## Running (Placeholder)

```bash
python server/app.py
```

## Architecture (Conceptual)

1.  **Vector/WirePod**: Captures sensor data (e.g., camera images), executes commands.
2.  **VectorBrain Server (this project)**:
    *   Receives data from Vector (mechanism TBD, could be push or pull).
    *   Processes data with an LLM.
    *   Orchestrates actions, potentially using external tools or databases.
    *   Sends commands back to Vector via WirePod's HTTP interface.
3.  **LLM**: The language model providing the core intelligence.

## Modules

-   `server/app.py`: Main server application (Flask/FastAPI).
-   `server/routes/`: Defines API endpoints for the server.
    -   `vector_routes.py`: Handles incoming requests to command Vector.
-   `server/modules/`: Contains core logic.
    -   `vector_http_client.py`: Client to communicate with WirePod's HTTP SDK endpoints.
    -   `llm_handler.py`: Interface for LLM interactions.
    -   (Future: `tool_manager.py`, `db_handler.py`, etc.)
```
