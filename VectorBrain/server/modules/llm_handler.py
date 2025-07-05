import os
import json # For potential future use if LLM returns JSON
import re   # For parsing commands from LLM string output

# Configuration for LLM (placeholders, use environment variables or a config system)
LLM_PROVIDER = os.getenv("VB_LLM_PROVIDER", "stub") # 'openai', 'local_ollama', 'huggingface_api', 'stub'
OPENAI_API_KEY = os.getenv("VB_OPENAI_API_KEY", "your_openai_api_key_here")
OPENAI_MODEL_NAME = os.getenv("VB_OPENAI_MODEL", "gpt-4o-mini") # e.g., gpt-4, gpt-3.5-turbo

# TODO: Add configuration for other providers if needed

# Placeholder for conversation history (in-memory, will be lost on server restart)
conversation_history = []
MAX_HISTORY_TURNS = 5 # Number of (user + assistant) message pairs to keep

# System prompt defining Vector's role, capabilities, and command format
DEFAULT_SYSTEM_PROMPT = """You are Vector, an autonomous robot.
You will receive observations, including text descriptions of images from your camera or other sensor data.
Your goal is to explore, interact, and respond intelligently.
Output your actions as a sequence of commands in the format: {{command_name||parameter_key1=value1,parameter_key2=value2,...}}
If a command takes no parameters, use {{command_name||}}.
If a command takes a single unnamed parameter (like sayText), use {{command_name||text_value}}.

Available commands and their parameters:
- {{sayText||text_to_say}}
- {{driveWheels||left_speed_mmps=float,right_speed_mmps=float,duration_ms=int}} (duration_ms > 0)
- {{turnInPlace||angle_degrees=float,speed_dps=float,is_absolute=bool_0_or_1,tolerance_deg=float}} (positive angle for CCW/left)
- {{moveLift||speed_dps=float}} (positive up, starts continuous movement; use 0 to stop or use setLiftHeight)
- {{setLiftHeight||height_ratio=float}} (0.0 is lowest, 1.0 is highest)
- {{moveHead||speed_dps=float}} (positive up, starts continuous movement; use 0 to stop or use setHeadAngle)
- {{setHeadAngle||angle_degrees=float}} (-22 (down) to 45 (up) degrees)
- {{playAnimation||animation_name=string}} (e.g., anim_greeting_goodmorning_01)
- {{playAnimationTrigger||trigger_name=string}} (e.g., UserSeen, ExploreLookAround)
- {{getCameraImage||}} (Use this if you need a new image to make a decision. No parameters.)
- {{getVectorStatus||}} (Use this to get current battery, etc. No parameters.)

Example: If you see a person, you might respond: {{sayText||Hello there!}} {{playAnimation||anim_greeting_goodmorning_01}}
If you want to move towards something: {{driveWheels||left_speed_mmps=50,right_speed_mmps=50,duration_ms=1000}}

Think step-by-step. You can output multiple commands.
If you are unsure what to do, you can say so, or ask for a new image with {{getCameraImage||}}.
"""

def _parse_llm_response_to_commands(llm_response_text: str) -> list:
    """
    Parses the LLM's text response to extract commands.
    Expected format: {{command_name||param_key=value,param_key2=value2}} or {{command_name||single_value}}
    """
    commands = []
    # Regex to find {{command||params}} blocks
    # It captures: 1=command_name, 2=parameters_string (optional)
    cmd_regex = re.compile(r"\{\{([a-zA-Z0-9_]+)\|\|(.*?)\}\}")

    for match in cmd_regex.finditer(llm_response_text):
        command_name = match.group(1).strip()
        params_str = match.group(2).strip()
        params_dict = {}

        if params_str: # If there are parameters
            # Check if it's key-value pairs or a single value
            if '=' in params_str:
                try:
                    param_pairs = params_str.split(',')
                    for pair in param_pairs:
                        key_value = pair.split('=', 1)
                        if len(key_value) == 2:
                            key = key_value[0].strip()
                            value_str = key_value[1].strip()
                            # Attempt to convert value to int, float, or bool
                            if value_str.lower() == 'true':
                                params_dict[key] = True
                            elif value_str.lower() == 'false':
                                params_dict[key] = False
                            elif '.' in value_str:
                                try:
                                    params_dict[key] = float(value_str)
                                except ValueError:
                                    params_dict[key] = value_str # Keep as string if float conversion fails
                            else:
                                try:
                                    params_dict[key] = int(value_str)
                                except ValueError:
                                    params_dict[key] = value_str # Keep as string if int conversion fails
                        else: # Malformed pair
                            print(f"[llm_handler] Warning: Malformed parameter pair '{pair}' for command '{command_name}'")
                except Exception as e:
                    print(f"[llm_handler] Error parsing key-value parameters for '{command_name}': {e}. Params string: '{params_str}'")
                    # Fallback: treat entire params_str as a single unnamed parameter if parsing fails badly
                    params_dict["_raw"] = params_str
            else:
                # Single unnamed parameter (e.g., for sayText)
                # The key for this single param can be conventional, e.g. "text" for sayText, or just "value"
                # For now, we'll let the route handler expect specific keys or handle a generic one.
                # Let's assume for sayText, the route handler expects 'text'.
                if command_name == "sayText":
                     params_dict["text"] = params_str
                else: # For other commands expecting single value but not key-value
                    params_dict["value"] = params_str # Generic single value key

        commands.append({
            "command": command_name,
            "params": params_dict
        })

    # Handle text outside commands as sayText if no other commands were parsed
    if not commands and llm_response_text.strip() and not cmd_regex.search(llm_response_text):
        commands.append({
            "command": "sayText",
            "params": {"text": llm_response_text.strip()}
        })

    return commands


def get_llm_decision(image_description: str = None, sensor_data: dict = None, user_text_input: str = None) -> list:
    """
    Gets a decision from the LLM based on current input.
    In a real implementation, this would involve API calls to an LLM.
    For now, it returns stubbed command sequences.

    Args:
        image_description (str, optional): Text description of a camera image.
        sensor_data (dict, optional): Other sensor data from Vector.
        user_text_input (str, optional): Direct text input from a user (e.g., for debugging).

    Returns:
        list: A list of command dictionaries, e.g.,
              [{'command': 'sayText', 'params': {'text': 'Hello world'}}, ...]
    """
    global conversation_history

    # Construct the user's part of the prompt for the LLM
    current_prompt_parts = []
    if user_text_input:
        current_prompt_parts.append(f"User says: \"{user_text_input}\"")
    if image_description:
        current_prompt_parts.append(f"You see: \"{image_description}\"")
    if sensor_data:
        current_prompt_parts.append(f"Current sensor status: {json.dumps(sensor_data)}")

    if not current_prompt_parts:
        current_prompt_parts.append("You have no new specific observations. What should you do next based on past interactions or general goals?")

    user_turn_content = " ".join(current_prompt_parts)

    # Prepare messages for LLM (system prompt + history + current user turn)
    messages_for_llm = [{"role": "system", "content": DEFAULT_SYSTEM_PROMPT}]
    for turn in conversation_history:
        messages_for_llm.append({"role": "user", "content": turn["user"]})
        messages_for_llm.append({"role": "assistant", "content": turn["assistant"]})
    messages_for_llm.append({"role": "user", "content": user_turn_content})

    llm_response_text = ""

    # --- Actual LLM Call Would Go Here ---
    if LLM_PROVIDER == "openai":
        # Ensure OPENAI_API_KEY is set
        # Example:
        # try:
        #     client = openai.OpenAI(api_key=OPENAI_API_KEY)
        #     chat_completion = client.chat.completions.create(
        #         model=OPENAI_MODEL_NAME,
        #         messages=messages_for_llm
        #     )
        #     llm_response_text = chat_completion.choices[0].message.content
        # except Exception as e:
        #     print(f"[llm_handler] OpenAI API Error: {e}")
        #     llm_response_text = "{{sayText||I had a problem thinking with OpenAI.}}"
        pass # Placeholder, not making actual calls in this stub
    elif LLM_PROVIDER == "stub":
        print(f"[llm_handler] Using STUB LLM. User prompt: {user_turn_content}")
        if "image" in user_turn_content.lower() or "see" in user_turn_content.lower():
            llm_response_text = "{{sayText||I acknowledge the image. What next?}} {{getCameraImage||}}"
        elif "hello" in user_turn_content.lower() or "hi" in user_turn_content.lower():
            llm_response_text = "{{sayText||Hello to you too from stub!}} {{playAnimation||animation_name=anim_greeting_goodmorning_01}}"
        elif "battery" in user_turn_content.lower():
            llm_response_text = "{{getVectorStatus||}} {{sayText||Checking my battery...}}"
        elif "drive" in user_turn_content.lower():
            llm_response_text = "{{driveWheels||left_speed_mmps=50,right_speed_mmps=50,duration_ms=1000}}"
        else:
            llm_response_text = "{{sayText||I'm using my stub brain today.}} {{playAnimation||animation_name=anim_ ενεργοποίηση}}" # anim_ ενεργοποίηση is a guess for a thinking/processing anim
    else:
        print(f"[llm_handler] LLM Provider '{LLM_PROVIDER}' not implemented in this stub.")
        llm_response_text = "{{sayText||My LLM provider is not configured in the stub handler.}}"

    if not llm_response_text: # Fallback if LLM call failed or provider not stubbed
         llm_response_text = "{{sayText||I'm not sure what to do. Can I get a new image?}} {{getCameraImage||}}"

    print(f"[llm_handler] LLM Response ({LLM_PROVIDER}): {llm_response_text}")

    # Update conversation history
    conversation_history.append({"user": user_turn_content, "assistant": llm_response_text})
    if len(conversation_history) > MAX_HISTORY_TURNS:
        # Keep only the last MAX_HISTORY_TURNS (user, assistant) pairs
        conversation_history = conversation_history[-MAX_HISTORY_TURNS:]

    # Parse the LLM response text into command structures
    parsed_commands = _parse_llm_response_to_commands(llm_response_text)

    print(f"[llm_handler] Parsed commands: {parsed_commands}")
    return parsed_commands


if __name__ == '__main__':
    print("--- Testing LLM Handler (Stubbed) ---")

    # Test 1: Simulate seeing an image
    cmds1 = get_llm_decision(image_description="a red ball on a blue carpet")
    # Expected: should acknowledge image and maybe ask for new one or an action.

    # Test 2: Simulate user text input
    cmds2 = get_llm_decision(user_text_input="hello vector, how are you?")
    # Expected: should respond to hello.

    # Test 3: Simulate sensor data
    cmds3 = get_llm_decision(sensor_data={"battery_level": 0.85, "is_on_charger": True})
    # Expected: might comment on battery or charger status.

    # Test 4: Drive command
    cmds4 = get_llm_decision(user_text_input="drive forward a little bit")

    # Test 5: Malformed or complex response (for parser robustness)
    print("\n--- Testing Parser with complex/malformed input ---")
    test_response_str = "Okay, I will do that. {{sayText||I am moving forward now.}} {{driveWheels||left_speed_mmps=50,right_speed_mmps=50,duration_ms=1000}} Then I will {{playAnimation||animation_name=anim_pounce_success_03}} and also {{malformed||param1=true,param2}}. Finally, some more text."
    parsed_complex = _parse_llm_response_to_commands(test_response_str)
    print(f"Parsed complex string: {parsed_complex}")
    # Expected: sayText, driveWheels, playAnimation, and another sayText for "Then I will", "and also", "Finally, some more text."
    # The regex parser in _parse_llm_response_to_commands will only pick up valid {{command||params}} blocks.
    # Text outside will be ignored by this specific regex based parser.
    # The updated _parse_llm_response_to_commands will try to handle text outside as sayText if no commands are found.
    # The current _parse_llm_response_to_commands will only extract the valid commands.

    print(f"\nFinal conversation history (simplified for stub): {conversation_history}")
```
