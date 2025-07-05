import requests
import json
import os
import time # For potential delays or timeouts

# Configuration - Loaded from environment variables with defaults
# Ensure these are set in your environment or a .env file for production
WIREPOD_IP = os.getenv("WIREPOD_IP")
if not WIREPOD_IP:
    print("Warning: WIREPOD_IP environment variable not set. Defaulting to 'localhost'.")
    WIREPOD_IP = "localhost"

WIREPOD_PORT = os.getenv("WIREPOD_PORT", "8080") # Default WirePod web server port

# Determine if WirePod is using HTTPS (e.g., if ep.crt/ep.key are used by WirePod)
# This might need to be a configurable variable itself. For now, assume HTTP.
# If WirePod is set up with proper HTTPS and a known CA, use "https".
# If self-signed without host verification, requests might need `verify=False` (not recommended for production).
WIREPOD_SCHEME = os.getenv("WIREPOD_SCHEME", "http")
BASE_URL = f"{WIREPOD_SCHEME}://{WIREPOD_IP}:{WIREPOD_PORT}"

# Authentication: WirePod's /api/ endpoints defined in sdkapp/server.go do not show explicit token authentication.
# They seem to rely on identifying the robot via the 'serial' in the JSON payload.
# If specific authentication is discovered later, this needs to be updated.
# VECTOR_TOKEN = os.getenv("VECTOR_HTTP_TOKEN", None)

# --- Helper Function ---
def _make_request_to_sdkapp(method: str, endpoint_path: str, payload: dict = None, esn_in_payload: bool = True, esn: str = None):
    """
    Internal helper to make HTTP requests to WirePod's /api/ (sdkapp) endpoints.
    Args:
        method (str): HTTP method (POST, GET).
        endpoint_path (str): The specific API path (e.g., "say_text", "drive_wheels").
        payload (dict, optional): The JSON payload for POST requests or query params for GET.
        esn_in_payload (bool): If true, 'serial' is expected in the payload for POST or as a query param for GET.
        esn (str, optional): ESN of the robot, used if esn_in_payload is True.
    """
    url = f"{BASE_URL}/api/{endpoint_path}"
    headers = {"Content-Type": "application/json"}

    # if VECTOR_TOKEN: # If token auth were used
    #     headers["Authorization"] = f"Bearer {VECTOR_TOKEN}"

    current_payload = {}
    if payload: # Make a copy to avoid modifying the original dict if esn is added
        current_payload = payload.copy()


    if esn_in_payload and esn:
        current_payload['serial'] = esn
    elif esn_in_payload and 'serial' not in current_payload:
        message = f"Error: ESN must be provided in payload for {endpoint_path}"
        print(message)
        return False, {"error": message}

    # Add a small delay to avoid overwhelming WirePod if commands are sent rapidly
    time.sleep(0.05) # 50ms delay

    try:
        print(f"[vector_http_client] Requesting {method.upper()} {url} with data: {json.dumps(current_payload)}")
        if method.upper() == 'POST':
            response = requests.post(url, headers=headers, json=current_payload, timeout=15) # General timeout
        elif method.upper() == 'GET':
            # For GET, current_payload dict is converted to query params by 'params' arg in requests.get
            response = requests.get(url, headers=headers, params=current_payload, timeout=15)
        else:
            return False, {"error": f"Unsupported HTTP method: {method}"}

        response.raise_for_status()

        try:
            return True, response.json()
        except json.JSONDecodeError:
            if response.text: # If response is not JSON but status was OK (e.g. 200 OK with plain text "done")
                return True, {"message": response.text.strip()}
            return True, {"message": "Request successful, no JSON content."} # Should ideally not happen for GETs returning data

    except requests.exceptions.HTTPError as e:
        error_message = f"HTTP Error for ESN {esn} to {url}: {e}."
        try:
            error_details = e.response.json() # type: ignore
            error_message += f" Details: {error_details}"
        except json.JSONDecodeError:
            error_message += f" Raw response: {e.response.text if e.response else 'No response text'}" # type: ignore
        print(error_message)
        return False, {"error": str(e), "details": e.response.text if e.response else "No response body"} # type: ignore
    except requests.exceptions.RequestException as e:
        print(f"Request Exception for ESN {esn} to {url}: {e}")
        return False, {"error": str(e)}
    except Exception as e:
        print(f"An unexpected error occurred in _make_request_to_sdkapp for ESN {esn} to {url}: {e}")
        return False, {"error": f"Unexpected error: {str(e)}"}

# --- Vector Command Functions ---

def say_text(esn: str, text: str):
    """ Commands Vector to say the given text. """
    if not esn: return False, {"error": "ESN not provided for say_text"}
    if not text: return False, {"error": "Text not provided for say_text"}

    # print(f"[vector_http_client] Vector ({esn}) say: '{text}'") # Logging done in _make_request
    return _make_request_to_sdkapp("POST", "say_text", payload={"text": text}, esn=esn)

def drive_wheels(esn: str, left_speed_mmps: float, right_speed_mmps: float, duration_ms: int):
    """ Commands Vector to drive its wheels. """
    if not esn: return False, {"error": "ESN not provided for drive_wheels"}

    # print(f"[vector_http_client] Vector ({esn}) drive: L={left_speed_mmps}, R={right_speed_mmps}, D={duration_ms}ms")
    payload = {
        "left": left_speed_mmps,  # sdkapp/server.go expects these keys
        "right": right_speed_mmps,
        "dur": duration_ms
    }
    return _make_request_to_sdkapp("POST", "drive_wheels", payload=payload, esn=esn)

def turn_in_place(esn: str, angle_degrees: float, speed_dps: float, is_absolute: bool = False, tolerance_deg: float = 10.0):
    """
    Commands Vector to turn in place.
    sdkapp/server.go's /api/turn_in_place expects angle in degrees.
    Payload: {"serial":"00xxxxxx", "angle": angle_deg_string, "speed": speed_dps_string, "is_absolute": "0" or "1", "tolerance": tol_deg_string}
    """
    if not esn: return False, {"error": "ESN not provided for turn_in_place"}

    # print(f"[vector_http_client] Vector ({esn}) turn_in_place: Angle={angle_degrees}deg, Speed={speed_dps}dps, Abs={is_absolute}, Tol={tolerance_deg}deg")
    payload = {
        "angle": str(angle_degrees),
        "speed": str(speed_dps),
        "is_absolute": "1" if is_absolute else "0",
        "tolerance": str(tolerance_deg)
    }
    return _make_request_to_sdkapp("POST", "turn_in_place", payload=payload, esn=esn)

def move_lift(esn: str, speed_dps: float):
    """
    Moves Vector's lift continuously at the specified speed.
    sdkapp/server.go /api/move_lift payload: {"serial":"00xxxxxx", "speed": speed_dps_as_string}
    """
    if not esn: return False, {"error": "ESN not provided for move_lift"}

    # print(f"[vector_http_client] Vector ({esn}) move_lift: Speed={speed_dps}dps")
    payload = {"speed": str(speed_dps)} # sdkapp expects speed as string
    return _make_request_to_sdkapp("POST", "move_lift", payload=payload, esn=esn)

def set_lift_height(esn: str, height_ratio: float):
    """
    Sets Vector's lift to a specific height (0.0 to 1.0).
    sdkapp/server.go /api/move_lift (also used for set height) payload: {"serial":"00xxxxxx", "height": height_0_to_1_as_string}
    """
    if not esn: return False, {"error": "ESN not provided for set_lift_height"}
    if not (0.0 <= height_ratio <= 1.0):
        print(f"[vector_http_client] Warning: height_ratio {height_ratio} for {esn} is outside 0.0-1.0. Clamping.")
        height_ratio = max(0.0, min(1.0, height_ratio))

    # print(f"[vector_http_client] Vector ({esn}) set_lift_height to ratio: {height_ratio}")
    payload = {"height": str(height_ratio)} # sdkapp expects height as string for ratio
    return _make_request_to_sdkapp("POST", "move_lift", payload=payload, esn=esn) # Uses the same "move_lift" endpoint

def move_head(esn: str, speed_dps: float):
    """
    Moves Vector's head continuously at the specified speed.
    sdkapp/server.go /api/move_head payload: {"serial":"00xxxxxx", "speed": speed_dps_as_string}
    """
    if not esn: return False, {"error": "ESN not provided for move_head"}

    # print(f"[vector_http_client] Vector ({esn}) move_head: Speed={speed_dps}dps")
    payload = {"speed": str(speed_dps)} # sdkapp expects speed as string
    return _make_request_to_sdkapp("POST", "move_head", payload=payload, esn=esn)

def set_head_angle(esn: str, angle_degrees: float):
    """
    Sets Vector's head to a specific angle.
    sdkapp/server.go /api/move_head (also used for set angle) payload: {"serial":"00xxxxxx", "angle": angle_degrees_as_string}
    """
    if not esn: return False, {"error": "ESN not provided for set_head_angle"}
    min_head_angle_deg = -22.0
    max_head_angle_deg = 45.0
    if not (min_head_angle_deg <= angle_degrees <= max_head_angle_deg):
        print(f"[vector_http_client] Warning: head_angle {angle_degrees} for {esn} is outside {min_head_angle_deg}-{max_head_angle_deg}. Clamping.")
        angle_degrees = max(min_head_angle_deg, min(max_head_angle_deg, angle_degrees))

    # print(f"[vector_http_client] Vector ({esn}) set_head_angle to: {angle_degrees} degrees")
    payload = {"angle": str(angle_degrees)} # sdkapp expects angle as string
    return _make_request_to_sdkapp("POST", "move_head", payload=payload, esn=esn) # Uses the same "move_head" endpoint

def play_animation(esn: str, animation_name: str):
    """
    Commands Vector to play a specific animation (not a trigger).
    sdkapp/server.go /api/play_animation payload: {"serial":"00xxxxxx", "animation": animation_name}
    """
    if not esn: return False, {"error": "ESN not provided for play_animation"}
    if not animation_name: return False, {"error": "Animation name not provided"}

    # print(f"[vector_http_client] Vector ({esn}) play_animation: Name={animation_name}")
    payload = {"animation": animation_name}
    return _make_request_to_sdkapp("POST", "play_animation", payload=payload, esn=esn)

def play_animation_trigger(esn: str, trigger_name: str):
    """
    Commands Vector to play a specific animation trigger.
    sdkapp/server.go /api/play_animation_trigger payload: {"serial":"00xxxxxx", "trigger": trigger_name}
    """
    if not esn: return False, {"error": "ESN not provided for play_animation_trigger"}
    if not trigger_name: return False, {"error": "Animation trigger name not provided"}

    # print(f"[vector_http_client] Vector ({esn}) play_animation_trigger: Name={trigger_name}")
    payload = {"trigger": trigger_name} # sdkapp uses "trigger" for this endpoint
    return _make_request_to_sdkapp("POST", "play_animation_trigger", payload=payload, esn=esn)

def get_camera_image_b64(esn: str):
    """
    Requests a single camera image from Vector.
    This function remains a placeholder as a direct HTTP endpoint for single image capture
    is not evident in the analyzed sdkapp/server.go. This functionality might require
    a new endpoint in WirePod or a different mechanism (like WebSockets or image push from Vector).
    """
    if not esn: return False, {"error": "ESN not provided for get_camera_image_b64"}
    print(f"[vector_http_client] Vector ({esn}) get_camera_image_b64: This function is a placeholder and requires a suitable WirePod HTTP endpoint.")
    return False, {"error": "get_camera_image_b64 not implemented via HTTP. Requires a dedicated WirePod endpoint."}

def get_vector_status(esn: str):
    """
    Requests status from a specific Vector.
    The /api/get_robot_status endpoint in sdkapp/server.go returns a list of all robots.
    This function will make that request and then filter for the specified ESN.
    """
    if not esn: return False, {"error": "ESN not provided for get_vector_status"}

    # print(f"[vector_http_client] Vector ({esn}) get_status")
    # The endpoint /api/get_robot_status does not take 'serial' in payload for GET
    # It returns all robots. We filter client-side.
    success, response_data = _make_request_to_sdkapp("GET", "get_robot_status", payload=None, esn_in_payload=False)

    if success:
        if isinstance(response_data, list):
            for robot_status in response_data:
                if isinstance(robot_status, dict) and robot_status.get("serial") == esn:
                    return True, robot_status
            return False, {"error": f"Status for ESN {esn} not found in WirePod's list."}
        else: # Should be a list based on sdkapp/server.go
            return False, {"error": "Unexpected status response format from WirePod (expected a list).", "raw_response": response_data}
    return False, response_data # Return error from _make_request_to_sdkapp

# Example usage (for testing this module directly)
if __name__ == '__main__':
    if not WIREPOD_IP or WIREPOD_IP == "localhost": # Check if it's still the default
        print(f"WARNING: WIREPOD_IP is set to '{WIREPOD_IP}'. Ensure this is correct or set the environment variable.")

    test_esn = os.getenv("VB_TEST_ESN")
    if not test_esn:
        print("ERROR: VB_TEST_ESN environment variable not set.")
        print("Please set export VB_TEST_ESN=your_vector_esn before running this test.")
    else:
        print(f"--- Testing Vector HTTP Client against WirePod at {BASE_URL} (ESN: {test_esn}) ---")

        success, response = say_text(test_esn, "Vector Brain H. T. T. P. client test.")
        print(f"Say Text -> Success: {success}, Response: {response}\n")
        if not success : time.sleep(1) # Wait a bit if first command failed, maybe server is slow
        time.sleep(3)


        success, response = drive_wheels(test_esn, 30, 30, 1000) # Drive forward
        print(f"Drive Wheels (forward) -> Success: {success}, Response: {response}\n")
        time.sleep(2)

        success, response = drive_wheels(test_esn, -30, -30, 1000) # Drive backward
        print(f"Drive Wheels (backward) -> Success: {success}, Response: {response}\n")
        time.sleep(2)

        success, response = turn_in_place(test_esn, 90, 45) # Turn 90 deg left
        print(f"Turn In Place (90 deg left) -> Success: {success}, Response: {response}\n")
        time.sleep(4) # Turns can take a moment

        success, response = set_lift_height(test_esn, 0.8)
        print(f"Set Lift Height (0.8) -> Success: {success}, Response: {response}\n")
        time.sleep(3)

        success, response = set_lift_height(test_esn, 0.1)
        print(f"Set Lift Height (0.1) -> Success: {success}, Response: {response}\n")
        time.sleep(3)

        success, response = set_head_angle(test_esn, 30) # Head up
        print(f"Set Head Angle (30 deg) -> Success: {success}, Response: {response}\n")
        time.sleep(3)

        success, response = set_head_angle(test_esn, -15) # Head slightly down
        print(f"Set Head Angle (-15 deg) -> Success: {success}, Response: {response}\n")
        time.sleep(3)

        # Test with a known animation trigger name from vector-go-sdk/pkg/vectorpb/response_codes.go
        # Example: anim_greeting_goodmorning_01, anim_pounce_success_03
        anim_to_play = "anim_greeting_goodmorning_01"
        success, response = play_animation(test_esn, anim_to_play)
        print(f"Play Animation ('{anim_to_play}') -> Success: {success}, Response: {response}\n")
        time.sleep(5) # Animations can take time

        trigger_to_play = "anim_feedback_goodrobot_01"
        success, response = play_animation_trigger(test_esn, trigger_to_play)
        print(f"Play Animation Trigger ('{trigger_to_play}') -> Success: {success}, Response: {response}\n")
        time.sleep(5)


        success, status = get_vector_status(test_esn)
        print(f"Get Status -> Success: {success}, Status: {status}\n")
        time.sleep(1)

        success, img_status = get_camera_image_b64(test_esn) # Expected to fail gracefully or return placeholder error
        print(f"Get Camera Image (expect placeholder/error) -> Success: {success}, Status: {img_status}\n")

        print("--- Test complete ---")
```
