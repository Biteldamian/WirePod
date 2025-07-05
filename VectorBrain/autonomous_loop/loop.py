import time
import base64
import threading # For running the loop in a separate thread if called from web server

# Assuming these modules are in ../llm and ../vector_controller relative to this file's location
# Adjust import paths if your project structure is different or if using a proper package structure.
try:
    from llm.handler import LLMHandler
    from vector_controller.controller import VectorWebSDKController
    from autonomous_loop.command_parser import parse_llm_command_string, get_command_def
except ImportError: # Fallback for direct execution or different pathing
    print("Attempting fallback imports for loop.py")
    from ..llm.handler import LLMHandler
    from ..vector_controller.controller import VectorWebSDKController
    from .command_parser import parse_llm_command_string, get_command_def


import os
import sys
# Add parent directory to sys.path to allow direct import of config
sys.path.append(os.path.join(os.path.dirname(__file__), '..'))
try:
    import config
except ImportError:
    print("CRITICAL: config.py not found in VectorBrain/autonomous_loop/loop.py. Ensure it exists in the VectorBrain root directory.")
    # Define fallbacks if config import fails
    class config: # type: ignore
        WIREPOD_IP = "192.168.1.100"
        ROBOT_ESN = None
        LLM_API_BASE_URL = "http://localhost:8081/v1"
        LLM_API_KEY = "sk-no-key-required"
        LLM_MODEL_NAME = "llamafile"
        DEFAULT_SYSTEM_PROMPT_PATH = "autonomous_loop/default_system_prompt.txt" # Relative to VectorBrain root
        MAX_LOOP_ITERATIONS = 20
        LOOP_DELAY_SECONDS = 0.5

# Global variable to control the loop externally
_autonomous_loop_running = False
_autonomous_loop_thread = None

def load_system_prompt(file_path: str) -> str:
    try:
        with open(file_path, 'r') as f:
            return f.read()
    except FileNotFoundError:
        print(f"Warning: System prompt file not found at {file_path}. Using a basic default.")
        return """You are Vector, an autonomous robot.
Available Commands:
CMD_SAY_TEXT(textToSay: string)
CMD_DRIVE_WHEELS(leftWheelSpeed: float, rightWheelSpeed: float, durationMs: int)
CMD_TAKE_PHOTO_AND_CONTINUE()
CMD_STOP_AUTONOMOUS_MODE()
Respond with ONLY ONE command string."""

SYSTEM_PROMPT = load_system_prompt(DEFAULT_SYSTEM_PROMPT_FILE)

def run_autonomous_loop(robot_esn: str, wirepod_ip: str):
    """
    Main autonomous loop for Vector.
    """
    global _autonomous_loop_running
    if not _autonomous_loop_running: # Should be set by start_autonomous_loop
        print("Autonomous loop started directly but not enabled. Exiting.")
        return

    print(f"Starting autonomous loop for Vector ESN: {robot_esn} via WirePod: {wirepod_ip}")

    # Initialize controller and LLM handler
    # Note: VectorWebSDKController will try to auto-detect ESN if robot_esn is None,
    # but here we assume it's provided to start_autonomous_loop.
    vector_ctrl = VectorWebSDKController(wirepod_ip=wirepod_ip, robot_serial=robot_esn)
    if not vector_ctrl.robot_serial: # Check if ESN was actually set or found
        print(f"Error: Could not confirm robot ESN '{robot_esn}' with WirePod at {wirepod_ip}. Exiting autonomous loop.")
        _autonomous_loop_running = False
        return

    llm_handler = LLMHandler(api_base_url=LLM_API_BASE_URL, api_key=LLM_API_KEY, model_name=LLM_MODEL_NAME)

    conversation_history = [] # Optional: To maintain some context for the LLM

    # Initial "wake up" or acknowledgement
    vector_ctrl.say_text("Autonomous mode activated.")
    # Consider playing an animation trigger that's known to be safe/short
    # vector_ctrl.play_animation_trigger("anim_keepalive_eyesonly_level1_01") # Example
    time.sleep(1)

    for i in range(MAX_LOOP_ITERATIONS):
        if not _autonomous_loop_running:
            print("Autonomous loop externally stopped.")
            break

        print(f"\n--- Loop Iteration {i + 1} / {MAX_LOOP_ITERATIONS} ---")

        # 1. Get camera image
        print("Requesting camera image...")
        image_b64 = vector_ctrl.get_camera_image_b64()
        if image_b64:
            print("Camera image received.")
            # Optionally save for debugging:
            # with open(f"vector_image_iter_{i+1}.jpg", "wb") as f:
            #     f.write(base64.b64decode(image_b64))
        else:
            print("Failed to get camera image for this iteration.")
            # Decide if we should retry, or inform LLM. For now, LLM will be told no image.

        # 2. Get LLM command
        print("Querying LLM for next command...")
        # The LLMHandler's get_llm_command now constructs the user message based on image availability
        llm_command_str = llm_handler.get_llm_command(
            system_prompt=SYSTEM_PROMPT,
            conversation_history=conversation_history, # Pass previous turns if desired
            image_base64=image_b64
        )

        if not llm_command_str:
            print("LLM did not return a command. Skipping this iteration.")
            time.sleep(LOOP_DELAY_SECONDS)
            continue

        print(f"LLM response: '{llm_command_str}'")

        # Add LLM's raw response to history as "assistant" turn BEFORE parsing
        # This helps if LLM fails to format correctly, it can see its mistake
        if conversation_history is not None: # Ensure history is being used
             conversation_history.append({"role": "assistant", "content": llm_command_str})
             # Optional: Prune history to keep it from growing too large
             if len(conversation_history) > 10: # Keep last 5 exchanges (10 messages)
                 conversation_history = conversation_history[-10:]


        # 3. Parse LLM command
        command_name, params, error_msg = parse_llm_command_string(llm_command_str)

        if error_msg:
            print(f"Command parsing error: {error_msg}")
            vector_ctrl.say_text(f"I had a problem understanding that command: {error_msg.split(':')[0]}.") # Say first part of error
            # vector_ctrl.play_animation_trigger("anim_feedback_confused_01")
            time.sleep(LOOP_DELAY_SECONDS)
            continue

        if not command_name: # Should be caught by error_msg, but as a safeguard
            print("No command name parsed.")
            time.sleep(LOOP_DELAY_SECONDS)
            continue

        print(f"Parsed command: {command_name}, Params: {params}")

        # 4. Execute command
        if command_name == "CMD_STOP_AUTONOMOUS_MODE":
            print("CMD_STOP_AUTONOMOUS_MODE received. Stopping loop.")
            vector_ctrl.say_text("Stopping autonomous mode now.")
            # vector_ctrl.play_animation_trigger("anim_goodbye_01")
            _autonomous_loop_running = False # Signal to stop
            break
        elif command_name == "CMD_TAKE_PHOTO_AND_CONTINUE":
            print("CMD_TAKE_PHOTO_AND_CONTINUE received. Will get fresh image next iteration.")
            # vector_ctrl.play_animation_trigger("anim_phototaken_01")
            # No action needed here, loop will get new image.
        else:
            cmd_def = get_command_def(command_name)
            if not cmd_def: # Should have been caught by parser, but double check
                print(f"Unknown command '{command_name}' after parsing. This should not happen.")
                continue

            print(f"Executing: {command_name}")
            success = False
            if command_name == "CMD_SAY_TEXT":
                success = vector_ctrl.say_text(params.get("textToSay", ""))
            elif command_name == "CMD_DRIVE_WHEELS":
                success = vector_ctrl.drive_wheels(
                    params.get("leftWheelSpeed", 0.0),
                    params.get("rightWheelSpeed", 0.0),
                    params.get("durationMs", 0)
                )
            elif command_name == "CMD_TURN_IN_PLACE":
                # The controller might need adjustment if it doesn't have turn_in_place
                # For now, assuming it's implemented or we use drive_wheels for turns.
                # Let's assume we need to implement it in controller, or use drive_wheels.
                # For simplicity here, let's map to differential drive if no direct turn command in controller
                angle_rad = params.get("angleRad", 0.0)
                turn_speed = 50 # mm/s, arbitrary
                turn_duration_ms = int(abs(angle_rad) * 500) # Very rough estimate for duration
                if angle_rad > 0: # Clockwise for this example (adjust based on your definition)
                    success = vector_ctrl.drive_wheels(turn_speed, -turn_speed, turn_duration_ms)
                else: # Counter-clockwise
                    success = vector_ctrl.drive_wheels(-turn_speed, turn_speed, turn_duration_ms)
                print(f"Executed CMD_TURN_IN_PLACE via drive_wheels (angle: {angle_rad})")
            elif command_name == "CMD_MOVE_HEAD":
                success = vector_ctrl.move_head(params.get("angleRad", 0.0))
            elif command_name == "CMD_MOVE_LIFT":
                 # controller.py expects height_ratio (0-1). LLM prompt gives heightMm (0-92).
                 # We need to convert or update one of them. Let's convert here.
                height_mm = params.get("heightMm", 0.0)
                height_ratio = height_mm / 92.0 # Approximate max lift height
                height_ratio = max(0.0, min(1.0, height_ratio)) # Clamp to 0-1
                success = vector_ctrl.move_lift(height_ratio)
            elif command_name == "CMD_PLAY_ANIMATION":
                success = vector_ctrl.play_animation_trigger(params.get("animationName", "")) # Assuming animationName is a trigger
            elif command_name == "CMD_PLAY_ANIMATION_WI":
                 # For WI, it's often a trigger too. If it's a general animation name,
                 # controller needs to handle it, or we map it here.
                success = vector_ctrl.play_animation_trigger(params.get("animationName", "") + "_WI") # Placeholder for WI version
                print(f"Note: CMD_PLAY_ANIMATION_WI executed as trigger: {params.get('animationName', '')}_WI. Verify trigger names.")

            if success:
                print(f"Command {command_name} executed successfully.")
                # vector_ctrl.play_animation_trigger("anim_feedback_affirmative_01")
            else:
                print(f"Command {command_name} failed or controller reported failure.")
                vector_ctrl.say_text(f"I had a problem with the command: {command_name.replace('CMD_', '').replace('_', ' ').lower()}")
                # vector_ctrl.play_animation_trigger("anim_feedback_facepalmerror_01")

        # Add the executed command (or intent) and its outcome to conversation history
        # This part is conceptual for LLM to learn from its actions.
        # For now, we added the raw LLM command earlier. We could add a "system_feedback" message.
        # e.g., conversation_history.append({"role": "system", "content": f"Executed {command_name}. Success: {success}"})


        time.sleep(LOOP_DELAY_SECONDS)

    print("Autonomous loop finished.")
    if _autonomous_loop_running: # If loop finished due to max iterations
        vector_ctrl.say_text("Maximum loop iterations reached. Exiting autonomous mode.")
    _autonomous_loop_running = False # Ensure it's marked as not running

def start_autonomous_loop(robot_esn: str, wirepod_ip: str):
    global _autonomous_loop_running, _autonomous_loop_thread
    if _autonomous_loop_running:
        print("Autonomous loop is already running.")
        return False

    _autonomous_loop_running = True
    # Run the loop in a new thread so it doesn't block the caller (e.g., the web server)
    _autonomous_loop_thread = threading.Thread(target=run_autonomous_loop, args=(robot_esn, wirepod_ip))
    _autonomous_loop_thread.daemon = True # Allow main program to exit even if thread is running
    _autonomous_loop_thread.start()
    print(f"Autonomous loop thread started for {robot_esn}.")
    return True

def stop_autonomous_loop():
    global _autonomous_loop_running
    if not _autonomous_loop_running:
        print("Autonomous loop is not running.")
        return False

    print("Requesting autonomous loop to stop...")
    _autonomous_loop_running = False
    # Wait for the thread to finish if it's running
    if _autonomous_loop_thread and _autonomous_loop_thread.is_alive():
        _autonomous_loop_thread.join(timeout=5.0) # Wait up to 5 seconds
        if _autonomous_loop_thread.is_alive():
            print("Warning: Autonomous loop thread did not stop cleanly after 5 seconds.")
    print("Autonomous loop marked as stopped.")
    return True

if __name__ == '__main__':
    # This is a basic test.
    # You would typically call start_autonomous_loop from your web server (main.py)

    # Create a dummy prompt file for testing if it doesn't exist
    if not os.path.exists(DEFAULT_SYSTEM_PROMPT_FILE):
        print(f"Creating dummy prompt file: {DEFAULT_SYSTEM_PROMPT_FILE}")
        with open(DEFAULT_SYSTEM_PROMPT_FILE, "w") as f:
            f.write("""You are Vector. Your goal is to explore.
Available Commands:
CMD_SAY_TEXT(textToSay: string)
CMD_DRIVE_WHEELS(leftWheelSpeed: float, rightWheelSpeed: float, durationMs: int)
CMD_TAKE_PHOTO_AND_CONTINUE()
CMD_STOP_AUTONOMOUS_MODE()
Respond with ONLY ONE command string. What is your next command based on the image?""")

    print("Attempting to start autonomous loop...")
    # Replace with your Vector's ESN and WirePod IP if not using defaults or auto-detect
    # For testing, you might need to manually input these or have a config.
    test_wirepod_ip = input(f"Enter WirePod IP (default: {DEFAULT_WIREPOD_IP}): ") or DEFAULT_WIREPOD_IP
    test_robot_esn = input(f"Enter Robot ESN (leave blank for auto-detect by controller): ") or None


    if start_autonomous_loop(robot_esn=test_robot_esn, wirepod_ip=test_wirepod_ip):
        print("Autonomous loop initiated. It will run in a separate thread.")
        print(f"Max iterations: {MAX_LOOP_ITERATIONS}. Loop delay: {LOOP_DELAY_SECONDS}s.")
        print("Press Ctrl+C to stop this script (and attempt to stop the loop).")
        try:
            while _autonomous_loop_running:
                time.sleep(1) # Keep main thread alive while loop runs
        except KeyboardInterrupt:
            print("\nKeyboard interrupt received. Stopping autonomous loop...")
            stop_autonomous_loop()
        finally:
            if _autonomous_loop_running: # Ensure it's stopped if loop was still marked running
                stop_autonomous_loop()
            print("Main script finished.")
    else:
        print("Failed to start autonomous loop (maybe already running or ESN issue).")

```
