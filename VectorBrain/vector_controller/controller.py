import requests
import json
import time
from typing import Optional, Dict, Any, Union, List

import sys
import os
# Add parent directory to sys.path to allow direct import of config
sys.path.append(os.path.join(os.path.dirname(__file__), '..'))
try:
    import config
except ImportError:
    print("CRITICAL: config.py not found. Ensure it exists in the VectorBrain root directory.")
    class config: # type: ignore
        WIREPOD_IP = "192.168.1.100" # Fallback
        ROBOT_ESN = None # Fallback
        # WIREPOD_PORT is not directly in config, assumed to be part of base URL construction or default 8080

REQUEST_TIMEOUT = 10  # Seconds
DEFAULT_WI ispod_PORT = "8080" # Retaining this if WIREPOD_PORT isn't explicitly in config.py

class VectorWebSDKController:
    def __init__(self, wirepod_ip: Optional[str] = None, robot_serial: Optional[str] = None):
        """
        Initializes the controller.
        Uses settings from config.py if parameters are not provided.
        Args:
            wirepod_ip: The IP address of the WirePod server.
            robot_serial: The ESN of the Vector robot to control.
        """
        self.wirepod_ip = wirepod_ip if wirepod_ip is not None else config.WIREPOD_IP
        # Assuming WirePod runs on port 8080 if not specified otherwise or part of a full URL in config
        # A more robust config might have WIREPOD_BASE_URL or separate WIREPOD_PORT.
        # For now, constructing with default port if only IP is in config.
        if ":" in self.wirepod_ip: # If port is included in wirepod_ip from config
            self.base_url = f"http://{self.wirepod_ip}"
        else:
            self.base_url = f"http://{self.wirepod_ip}:{DEFAULT_WI ispod_PORT}"

        self.robot_serial = robot_serial if robot_serial is not None else config.ROBOT_ESN
        self.session = requests.Session()

        if not self.robot_serial:
            print("Attempting to auto-detect robot ESN from WirePod...")
            status = self._get_robot_status_internal()
            if status and status.get("robots") and isinstance(status["robots"], list) and len(status["robots"]) > 0:
                first_robot = status["robots"][0]
                if isinstance(first_robot, dict) and first_robot.get("esn"):
                    self.robot_serial = first_robot["esn"]
                    print(f"Auto-detected robot ESN: {self.robot_serial}")
                else:
                    print(f"Could not auto-detect robot ESN from sdk_info (unexpected robot data structure): {first_robot}")
            else:
                print(f"Could not fetch robot list from sdk_info to auto-detect ESN. Status: {status}")

        print(f"VectorWebSDKController initialized for WirePod at {self.base_url}, Robot ESN: {self.robot_serial if self.robot_serial else 'Not specified'}")


    def _make_request(self, method: str, endpoint: str, payload: Optional[Dict[str, Any]] = None, params: Optional[Dict[str, Any]] = None, add_esn_to_payload: bool = True) -> Optional[Dict[str, Any]]:
        """
        Internal helper to make HTTP requests to WirePod.
        """
        url = f"{self.base_url}{endpoint}"
        headers = {"Content-Type": "application/json", "Accept": "application/json"}

        current_payload = payload.copy() if payload else {}

        # Add ESN to payload if required and available, and not already present
        if add_esn_to_payload and self.robot_serial and 'serial' not in current_payload and 'esn' not in current_payload:
            current_payload['serial'] = self.robot_serial

        try:
            if method.upper() == "GET":
                response = self.session.get(url, headers=headers, params=params, timeout=REQUEST_TIMEOUT)
            elif method.upper() == "POST":
                response = self.session.post(url, headers=headers, json=current_payload, params=params, timeout=REQUEST_TIMEOUT)
            else:
                print(f"Unsupported HTTP method: {method}")
                return None

            response.raise_for_status() # Raises an exception for bad status codes (4xx or 5xx)

            if response.content:
                try:
                    return response.json()
                except json.JSONDecodeError:
                    # If response is not JSON but request was successful (e.g. 204 No Content)
                    if 200 <= response.status_code < 300:
                        return {"status": "success", "message": "Request successful, no JSON content returned."}
                    print(f"Failed to decode JSON response from {endpoint}. Response text: {response.text[:200]}")
                    return None
            return {"status": "success", "message": "Request successful, no content."} # For 204 or empty body

        except requests.exceptions.RequestException as e:
            print(f"Error making request to {url}: {e}")
            return None
        except Exception as e:
            print(f"An unexpected error occurred with request to {url}: {e}")
            return None

    def _get_robot_status_internal(self) -> Optional[Dict[str, Any]]:
        """
        Internal function to get basic status, used for ESN auto-detection.
        Endpoint based on `getRobotsAjax` in `app.js` which calls `/api/get_sdk_info`.
        """
        return self._make_request("GET", "/api/get_sdk_info", add_esn_to_payload=False)

    def get_robot_status(self) -> Optional[Dict[str, Any]]:
        """
        Gets the general status/info of the robot(s) from WirePod.
        """
        return self._get_robot_status_internal()

    def say_text(self, text: str) -> bool:
        """
        Makes Vector say the given text.
        Endpoint based on `sayText()` in `app.js` which calls `/api/say_text`.
        Payload needs: {"text": text, "serial": this.currentEsn}
        """
        if not self.robot_serial:
            print("Error: Robot ESN not set. Cannot send say_text command.")
            return False
        payload = {"text": text, "serial": self.robot_serial}
        response = self._make_request("POST", "/api/say_text", payload=payload)
        return response is not None and response.get("status") != "error" # Assuming success if no error

    def play_animation(self, animation_name: str) -> bool:
        """
        Makes Vector play a specific animation.
        Endpoint based on `playAnimation()` in `app.js` which calls `/api/play_animation`.
        Payload needs: {"animation": animation_name, "serial": this.currentEsn}
        """
        if not self.robot_serial:
            print("Error: Robot ESN not set. Cannot send play_animation command.")
            return False
        payload = {"animation": animation_name, "serial": self.robot_serial}
        response = self._make_request("POST", "/api/play_animation", payload=payload)
        return response is not None and response.get("status") != "error"

    def play_animation_trigger(self, trigger_name: str) -> bool:
        """
        Makes Vector play an animation trigger.
        Endpoint based on `playAnimationTrigger()` in `app.js` -> `/api/play_anim_trigger`.
        Payload: {"trigger": trigger_name, "serial": this.currentEsn }
        """
        if not self.robot_serial:
            print("Error: Robot ESN not set. Cannot send play_animation_trigger command.")
            return False
        payload = {"trigger": trigger_name, "serial": self.robot_serial}
        response = self._make_request("POST", "/api/play_anim_trigger", payload=payload)
        return response is not None and response.get("status") != "error"


    def drive_wheels(self, left_wheel_speed: float, right_wheel_speed: float, duration_ms: int) -> bool:
        """
        Drives Vector's wheels.
        Endpoint based on `driveWheels()` in `app.js` -> `/api/drive_wheels`.
        Payload: {"lws": lw, "rws": rw, "lws2": lw, "rws2": rw, "dur": duration, "serial": serial}
        Note: app.js sends lws/rws and lws2/rws2 the same. We'll do the same.
        """
        if not self.robot_serial:
            print("Error: Robot ESN not set. Cannot send drive_wheels command.")
            return False
        payload = {
            "lws": left_wheel_speed,
            "rws": right_wheel_speed,
            "lws2": left_wheel_speed, # Matching app.js behavior
            "rws2": right_wheel_speed, # Matching app.js behavior
            "dur": duration_ms,
            "serial": self.robot_serial
        }
        response = self._make_request("POST", "/api/drive_wheels", payload=payload)
        return response is not None and response.get("status") != "error"

    def move_head(self, angle_rad: float) -> bool:
        """
        Moves Vector's head.
        Endpoint based on `moveHead()` in `app.js` -> `/api/move_head`.
        Payload: {"angle": speed, "serial": serial} - app.js seems to send angle as 'speed' param name.
        Let's assume the API actually expects "angle_rad" or similar based on SDK.
        For now, matching app.js which might be a bug or simplification there.
        The Go SDK uses AngleRad for MoveHeadRequest.
        Let's use a more descriptive payload key "angle_rad" and hope the API handles it or adjust later.
        If not, we might need to stick to "angle" as per app.js
        """
        if not self.robot_serial:
            print("Error: Robot ESN not set. Cannot send move_head command.")
            return False
        # Using "angle_rad" as a more descriptive key. If this fails, try "angle".
        payload = {"angle_rad": angle_rad, "serial": self.robot_serial}
        response = self._make_request("POST", "/api/move_head", payload=payload)
        return response is not None and response.get("status") != "error"

    def move_lift(self, height_ratio: float) -> bool:
        """
        Moves Vector's lift. height_ratio is 0.0 to 1.0.
        Endpoint based on `moveLift()` in `app.js` -> `/api/move_lift`.
        Payload: {"height": speed, "serial": serial} - app.js sends height_ratio as 'speed' param name.
        Similar to move_head, using a more descriptive "height_ratio".
        """
        if not self.robot_serial:
            print("Error: Robot ESN not set. Cannot send move_lift command.")
            return False
        payload = {"height_ratio": height_ratio, "serial": self.robot_serial} # Using "height_ratio"
        response = self._make_request("POST", "/api/move_lift", payload=payload)
        return response is not None and response.get("status") != "error"

    def get_camera_image_b64(self) -> Optional[str]:
        """
        Requests a single camera image from Vector, expecting a base64 encoded string.
        The endpoint `/api/get_cam_image` is a guess based on common patterns
        and `getImage()` in `util.js` which draws to canvas.
        We need an endpoint that returns the image data directly.
        If WirePod's `/api/get_cam_image` returns JSON like {"image": "base64string"}, this will work.
        """
        if not self.robot_serial:
            print("Error: Robot ESN not set. Cannot get camera image.")
            return None
        # This endpoint might require the serial in the query params or payload
        params = {"serial": self.robot_serial, "_": int(time.time() * 1000)} # Cache buster
        response = self._make_request("GET", "/api/get_cam_image", params=params, add_esn_to_payload=False)
        if response and "image" in response:
            return response["image"] # Assuming {"image": "base64_encoded_string"}
        elif response:
            print(f"Camera image response did not contain 'image' field. Response: {response}")
        return None

    def set_eye_color(self, hue: float, saturation: float) -> bool:
        """
        Sets Vector's eye color.
        Endpoint: /api/set_custom_eyecolor
        Payload: {"hue": hue_value, "sat": sat_value, "serial": esn}
        """
        if not self.robot_serial:
            print("Error: Robot ESN not set. Cannot set eye color.")
            return False
        payload = {"hue": hue, "sat": saturation, "serial": self.robot_serial}
        response = self._make_request("POST", "/api/set_custom_eyecolor", payload=payload)
        return response is not None and response.get("status") != "error"

    # Add more methods here as needed, e.g., for:
    # - TurnInPlace (if there's a specific endpoint, otherwise use DriveWheels)
    # - Getting other sensor data if exposed by web SDK

# Example Usage (for testing this module directly):
if __name__ == '__main__':
    wirepod_ip_address = input("Enter WirePod IP Address: ")
    if not wirepod_ip_address:
        print("WirePod IP address is required.")
    else:
        # First, try to auto-detect ESN
        controller = VectorWebSDKController(wirepod_ip_address)

        if not controller.robot_serial:
            print("Could not auto-detect robot ESN. Please ensure a robot is connected to WirePod.")
            # robot_esn_manual = input("Enter Robot ESN (optional, if auto-detect failed): ")
            # if robot_esn_manual:
            #    controller.robot_serial = robot_esn_manual
            # else:
            #    print("Cannot proceed without robot ESN.")
            #    exit()

        if controller.robot_serial:
            print(f"Using WirePod at {controller.base_url} for robot {controller.robot_serial}")

            status = controller.get_robot_status()
            if status:
                print("\nRobot Status:")
                print(json.dumps(status, indent=2))
            else:
                print("\nFailed to get robot status.")

            # Test say command
            # print("\nAttempting to make Vector say 'Hello from Vector Brain'")
            # if controller.say_text("Hello from Vector Brain"):
            #     print("Say text command sent successfully.")
            # else:
            #     print("Failed to send say text command.")
            # time.Sleep(3)

            # Test animation command
            # print("\nAttempting to play 'happy' animation trigger (anim_feedback_happy_01)")
            # if controller.play_animation_trigger("anim_feedback_happy_01"): # Triggers are often direct animation names
            #     print("Play animation trigger command sent successfully.")
            # else:
            #     print("Failed to send play animation trigger command.")
            # time.Sleep(3)

            # Test drive command
            # print("\nAttempting to drive wheels forward")
            # if controller.drive_wheels(left_wheel_speed=50, right_wheel_speed=50, duration_ms=1000):
            #     print("Drive wheels command sent successfully.")
            # else:
            #     print("Failed to send drive wheels command.")
            # time.Sleep(2)

            # Test camera image
            print("\nAttempting to get camera image...")
            b64_image = controller.get_camera_image_b64()
            if b64_image:
                print(f"Successfully received base64 image data (first 100 chars): {b64_image[:100]}...")
                # To save it:
                # import base64
                # image_data = base64.b64decode(b64_image)
                # with open("vector_cam_image.jpg", "wb") as f:
                #     f.write(image_data)
                # print("Saved image to vector_cam_image.jpg")
            else:
                print("Failed to get camera image.")
        else:
            print("Could not initialize controller with a robot ESN.")
