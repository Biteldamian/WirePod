import re
import strconv # This will be a custom helper or we'll use Python's built-ins directly

# Placeholder for strconv, as Python's strconv is not a direct equivalent for Go's.
# We'll use Python's int(), float(), str() directly in parsing.

class AutonomousCommand:
    """
    Represents the definition of an autonomous command.
    This is a Python equivalent of the Go struct for conceptual alignment.
    """
    def __init__(self, command: str, description: str, expected_params: list[str], param_types: list[str]):
        self.command = command
        self.description = description
        self.expected_params = expected_params
        self.param_types = param_types

# This list would ideally be loaded from a shared config or defined consistently
# with the Go version if both systems need to know about all commands.
# For now, defining a subset relevant to what VectorBrain's LLM might output.
VALID_AUTONOMOUS_COMMANDS_PY = [
    AutonomousCommand("CMD_SAY_TEXT", "Makes Vector say text.", ["textToSay"], ["string"]),
    AutonomousCommand("CMD_DRIVE_WHEELS", "Drives wheels.", ["leftWheelSpeed", "rightWheelSpeed", "durationMs"], ["float", "float", "int"]),
    AutonomousCommand("CMD_TURN_IN_PLACE", "Turns Vector.", ["angleRad", "speedRadPerSec", "direction"], ["float", "float", "int"]),
    AutonomousCommand("CMD_MOVE_HEAD", "Moves head.", ["angleRad"], ["float"]),
    AutonomousCommand("CMD_MOVE_LIFT", "Moves lift.", ["heightMm"], ["float"]), # Assuming heightMm, though controller used height_ratio
    AutonomousCommand("CMD_PLAY_ANIMATION", "Plays animation.", ["animationName"], ["string"]),
    AutonomousCommand("CMD_PLAY_ANIMATION_WI", "Plays animation while idle/speaking.", ["animationName"], ["string"]),
    AutonomousCommand("CMD_TAKE_PHOTO_AND_CONTINUE", "Gets new photo and continues loop.", [], []),
    AutonomousCommand("CMD_STOP_AUTONOMOUS_MODE", "Stops autonomous mode.", [], []),
]

def get_command_def(command_name: str) -> AutonomousCommand | None:
    for cmd_def in VALID_AUTONOMOUS_COMMANDS_PY:
        if cmd_def.command == command_name:
            return cmd_def
    return None

def parse_llm_command_string(llm_output: str) -> tuple[str | None, dict | None, str | None]:
    """
    Parses an LLM command string like "CMD_DRIVE_WHEELS(50, 50, 1000)".
    Returns (command_name, params_dict, error_message).
    """
    llm_output = llm_output.strip()

    # Regex to capture command name and parameters within parentheses
    match = re.fullmatch(r"([A-Z_0-9]+)\s*\((.*)\)", llm_output)

    if not match:
        # Handle commands with no parentheses (no arguments)
        if re.fullmatch(r"([A-Z_0-9]+)", llm_output):
            command_name = llm_output
            cmd_def = get_command_def(command_name)
            if cmd_def and not cmd_def.expected_params:
                return command_name, {}, None
            elif cmd_def:
                return None, None, f"Command '{command_name}' expects parameters but none were given in parentheses."
            else:
                return None, None, f"Unknown command or invalid format (no parentheses for potential args): {llm_output}"
        return None, None, f"Invalid command format: '{llm_output}'. Expected CMD_NAME(params...)"

    command_name, params_str = match.groups()
    params_str = params_str.strip()

    cmd_def = get_command_def(command_name)
    if not cmd_def:
        return None, None, f"Unknown command: {command_name}"

    parsed_params = {}

    if not params_str and cmd_def.expected_params: # Expects params but got empty parentheses
        return None, None, f"Command '{command_name}' expects {len(cmd_def.expected_params)} parameters, but received empty parentheses."
    if not params_str and not cmd_def.expected_params: # No params expected, empty parentheses are fine
        return command_name, parsed_params, None
    if params_str and not cmd_def.expected_params: # Params provided but none expected
        return None, None, f"Command '{command_name}' expects no parameters, but received: {params_str}"

    # Split parameters by comma, but be careful about commas inside strings
    # A more robust parser might use ast.literal_eval if params were Python literals,
    # but LLM output is less predictable. This simple split works for numbers/simple strings.
    # For strings with commas, the LLM must quote them, and we'd need a CSV-like parser.
    # For now, assuming simple comma separation.
    param_values_str = [p.strip() for p in params_str.split(',')]

    if len(param_values_str) != len(cmd_def.expected_params):
        return None, None, f"Command '{command_name}': expected {len(cmd_def.expected_params)} parameters ({', '.join(cmd_def.expected_params)}), got {len(param_values_str)} values: [{params_str}]"

    for i, param_name in enumerate(cmd_def.expected_params):
        value_str = param_values_str[i]
        param_type = "string"  # Default type
        if i < len(cmd_def.param_types):
            param_type = cmd_def.param_types[i]

        try:
            if param_type == "int":
                parsed_params[param_name] = int(value_str)
            elif param_type == "float":
                parsed_params[param_name] = float(value_str)
            elif param_type == "string":
                # Strip quotes if LLM happens to add them
                if (value_str.startswith('"') and value_str.endswith('"')) or \
                   (value_str.startswith("'") and value_str.endswith("'")):
                    parsed_params[param_name] = value_str[1:-1]
                else:
                    parsed_params[param_name] = value_str
            elif param_type == "bool":
                if value_str.lower() in ['true', '1', 'yes']:
                    parsed_params[param_name] = True
                elif value_str.lower() in ['false', '0', 'no']:
                    parsed_params[param_name] = False
                else:
                    raise ValueError("Invalid boolean value")
            else:
                return None, None, f"Unsupported parameter type '{param_type}' for '{param_name}' in command '{command_name}'"
        except ValueError as e:
            return None, None, f"Error parsing parameter '{param_name}' for command '{command_name}': expected {param_type}, got '{value_str}'. Details: {e}"

    return command_name, parsed_params, None

if __name__ == '__main__':
    test_strings = [
        "CMD_SAY_TEXT(Hello world)",
        "CMD_DRIVE_WHEELS(50.5, -50.0, 1000)",
        "CMD_TURN_IN_PLACE(1.57, 1.0, 0)",
        "CMD_MOVE_HEAD(0.5)",
        "CMD_MOVE_LIFT(45.0)",
        "CMD_PLAY_ANIMATION(happy)",
        "CMD_TAKE_PHOTO_AND_CONTINUE()", # With parentheses
        "CMD_TAKE_PHOTO_AND_CONTINUE",   # Without parentheses
        "CMD_STOP_AUTONOMOUS_MODE",
        "CMD_UNKNOWN_COMMAND(param1)",
        "CMD_DRIVE_WHEELS(50, 50)", # Wrong number of params
        "CMD_DRIVE_WHEELS(50, fifty, 1000)", # Wrong type
        "CMD_SAY_TEXT(\"Hello, world with comma\")",
        "CMD_SAY_TEXT('Single quoted string')",
        "CMD_DRIVE_WHEELS(50.0,50.0,1000 )", # Spacing
    ]

    for s in test_strings:
        name, params, err = parse_llm_command_string(s)
        if err:
            print(f"Input: '{s}' -> Error: {err}")
        else:
            print(f"Input: '{s}' -> Name: {name}, Params: {params}")

    # Test case for command expecting params but getting none in parens
    name, params, err = parse_llm_command_string("CMD_DRIVE_WHEELS()")
    if err:
        print(f"Input: 'CMD_DRIVE_WHEELS()' -> Error: {err}")
    else:
        print(f"Input: 'CMD_DRIVE_WHEELS()' -> Name: {name}, Params: {params}")

    # Test case for command expecting no params but getting some
    name, params, err = parse_llm_command_string("CMD_TAKE_PHOTO_AND_CONTINUE(unexpected)")
    if err:
        print(f"Input: 'CMD_TAKE_PHOTO_AND_CONTINUE(unexpected)' -> Error: {err}")
    else:
        print(f"Input: 'CMD_TAKE_PHOTO_AND_CONTINUE(unexpected)' -> Name: {name}, Params: {params}")
```
