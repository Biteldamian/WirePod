from flask import Flask, request, jsonify
import os
import sys

# Adjust import path to reach the autonomous_loop module
# This assumes 'VectorBrain' is the root of where you run python,
# or that VectorBrain is in PYTHONPATH
try:
    from autonomous_loop.loop import start_autonomous_loop, stop_autonomous_loop, _autonomous_loop_running
except ImportError:
    # If running server/main.py directly, Python might not see parent 'autonomous_loop'
    # Add parent directory of 'server' to path, which should be 'VectorBrain'
    sys.path.append(os.path.join(os.path.dirname(__file__), '..'))
    try:
        from autonomous_loop.loop import start_autonomous_loop, stop_autonomous_loop, _autonomous_loop_running
    except ImportError as e:
        print(f"Error importing autonomous_loop.loop: {e}")
        print("Ensure that the VectorBrain directory is in your PYTHONPATH or you are running from VectorBrain directory.")
        # Define dummy functions if import fails, so server can still start for basic testing
        def start_autonomous_loop(robot_esn, wirepod_ip): print("Dummy start_autonomous_loop"); return False
        def stop_autonomous_loop(): print("Dummy stop_autonomous_loop"); return False
        _autonomous_loop_running = False


app = Flask(__name__)

# Try to import config, with fallback for basic server operation if missing
try:
    import config # Assumes config.py is in VectorBrain root, and VectorBrain is in PYTHONPATH or running from there
except ImportError:
    print("WARNING: VectorBrain/config.py not found or import error. Using fallback configurations for server.")
    class config: # type: ignore
        WIREPOD_IP = os.environ.get("WIREPOD_IP", "192.168.1.100")
        ROBOT_ESN = os.environ.get("ROBOT_ESN", None)
        VECTOR_BRAIN_SERVER_HOST = os.environ.get("VECTOR_BRAIN_SERVER_HOST", "0.0.0.0")
        VECTOR_BRAIN_SERVER_PORT = int(os.environ.get("VECTOR_BRAIN_SERVER_PORT", 5001))
        # We need a path for the prompt file for the __main__ block, even in fallback
        _BASE_DIR_FALLBACK = os.path.dirname(os.path.abspath(__file__)) # server directory
        DEFAULT_SYSTEM_PROMPT_PATH = os.path.join(os.path.dirname(_BASE_DIR_FALLBACK), 'autonomous_loop', 'default_system_prompt.txt')


@app.route('/')
def index():
    return "VectorBrain Server is running. Use API endpoints to control autonomous mode."

@app.route('/api/start_autonomous_mode', methods=['POST'])
def handle_start_autonomous_mode():
    data = request.get_json()
    if not data:
        return jsonify({"status": "error", "message": "Request body must be JSON."}), 400

    # Use ESN from request, fallback to config, then error if none
    robot_esn = data.get('robot_esn', config.ROBOT_ESN)
    if not robot_esn:
        return jsonify({"status": "error", "message": "Missing 'robot_esn' in request and not set in config."}), 400

    # Use wirepod_ip from request, fallback to config
    wirepod_ip_to_use = data.get('wirepod_ip', config.WIREPOD_IP)


    print(f"API: Received request to start autonomous mode for ESN: {robot_esn} on WirePod: {wirepod_ip_to_use}")

    if _autonomous_loop_running:
        return jsonify({"status": "warning", "message": "Autonomous loop is already running."}), 200 # Or 409 Conflict

    if start_autonomous_loop(robot_esn=robot_esn, wirepod_ip=wirepod_ip_to_use):
        return jsonify({"status": "success", "message": f"Autonomous loop started for robot {robot_esn}."}), 200
    else:
        return jsonify({"status": "error", "message": f"Failed to start autonomous loop for robot {robot_esn}."}), 500


@app.route('/api/stop_autonomous_mode', methods=['POST'])
def handle_stop_autonomous_mode():
    print("API: Received request to stop autonomous mode.")
    if not _autonomous_loop_running:
        return jsonify({"status": "warning", "message": "Autonomous loop is not currently running."}), 200

    if stop_autonomous_loop():
        return jsonify({"status": "success", "message": "Autonomous loop stopping sequence initiated."}), 200
    else:
        # This might happen if stop_autonomous_loop had an issue (e.g., thread never started)
        return jsonify({"status": "error", "message": "Could not stop autonomous loop (already stopped or error)."}), 500

@app.route('/api/autonomous_status', methods=['GET'])
def handle_autonomous_status():
    if _autonomous_loop_running:
        return jsonify({"status": "running", "message": "Autonomous loop is active."}), 200
    else:
        return jsonify({"status": "stopped", "message": "Autonomous loop is not active."}), 200

if __name__ == '__main__':
    # Ensure the default prompt file exists if using the default path from config
    # This check is more for convenience during direct execution of main.py
    # The loop.py itself also has a fallback if the prompt file is missing.
    prompt_file_path_from_config = config.DEFAULT_SYSTEM_PROMPT_PATH
    if not os.path.isabs(prompt_file_path_from_config):
        # If path in config is relative, assume it's relative to VectorBrain root
        prompt_file_path_from_config = os.path.join(os.path.dirname(__file__), '..', prompt_file_path_from_config)

    if not os.path.exists(prompt_file_path_from_config):
        print(f"Warning: System prompt file not found at configured path: {prompt_file_path_from_config}")
        print("Attempting to create a basic dummy prompt file there for server to run.")
        try:
            os.makedirs(os.path.dirname(prompt_file_path_from_config), exist_ok=True)
            with open(prompt_file_path_from_config, "w") as f:
                f.write("""You are Vector. Default prompt.
CMD_SAY_TEXT(textToSay: string)
CMD_STOP_AUTONOMOUS_MODE()
Respond with ONLY ONE command string.""")
            print(f"Dummy prompt file created at {prompt_file_path_from_config}")
        except Exception as e:
            print(f"Error creating dummy prompt file: {e}")


    server_host = config.VECTOR_BRAIN_SERVER_HOST
    server_port = config.VECTOR_BRAIN_SERVER_PORT

    print(f"Starting VectorBrain Flask server on http://{server_host}:{server_port} ...")
    # Setting debug=False for a slightly cleaner default, can be enabled via config if needed
    app.run(host=server_host, port=server_port, debug=False)
```

**To make this runnable, I also need to create the dummy system prompt file that `loop.py` (and now `main.py` for its `if __name__ == '__main__'` block) tries to load.**
