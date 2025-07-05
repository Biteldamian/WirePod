package autonomous

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fforchino/vector-go-sdk/pkg/vector"
	"github.com/fforchino/vector-go-sdk/pkg/vectorpb"
	"github.com/kercre123/wire-pod/chipper/pkg/logger" // Assuming logger path
)

// AutonomousCommand defines the structure for commands Vector can execute in autonomous mode.
type AutonomousCommand struct {
	Command         string   // e.g., CMD_DRIVE_WHEELS
	Description     string   // Description of what the command does
	ExpectedParams  []string // Ordered list of parameter names, e.g., ["leftWheelSpeed", "rightWheelSpeed", "durationMs"]
	ParamTypes      []string // Optional: "int", "float", "string", "bool" to help with parsing/validation
	SDKFunction     func(robot *vector.Vector, params map[string]interface{}, ctx logger.Logger) error // Direct mapping to an SDK call
}

// ValidAutonomousCommands lists all commands available in autonomous mode.
var ValidAutonomousCommands []AutonomousCommand

func init() {
	ValidAutonomousCommands = []AutonomousCommand{
		{
			Command:        "CMD_DRIVE_WHEELS",
			Description:    "Drives Vector's wheels. Speeds are in mm/s, duration in ms.",
			ExpectedParams: []string{"leftWheelSpeed", "rightWheelSpeed", "durationMs"},
			ParamTypes:     []string{"float", "float", "int"},
			SDKFunction: func(robot *vector.Vector, params map[string]interface{}, ctx logger.Logger) error {
				lw, lok := params["leftWheelSpeed"].(float32)
				rw, rok := params["rightWheelSpeed"].(float32)
				dur, dok := params["durationMs"].(uint32) // DurationMs is uint32 in DriveWheelsRequest
				if !lok || !rok || !dok {
					return fmt.Errorf("invalid parameters for CMD_DRIVE_WHEELS. Got: %+v", params)
				}
				// For simplicity, setting LeftWheelMmps2 and RightWheelMmps2 the same.
				// Adjust if separate control for lws/rws (speed change over duration) is needed.
				_, err := robot.Conn.DriveWheels(
					robot.Ctx, // Assuming robot.Ctx is the parent context for the robot instance
					&vectorpb.DriveWheelsRequest{
						LeftWheelMmps:  lw,
						RightWheelMmps: rw,
						LeftWheelMmps2: lw, // Using same speed for simplicity for now
						RightWheelMmps2:rw,  // Using same speed for simplicity for now
						DurationMs:     dur,
					},
				)
				return err
			},
		},
		{
			Command:        "CMD_TURN_IN_PLACE",
			Description:    "Turns Vector in place. Angle in radians, speed in rad/s, accel in rad/s^2. Direction: 0 for clockwise, 1 for counter-clockwise.",
			ExpectedParams: []string{"angleRad", "speedRadPerSec", "accelRadPerSecSq", "direction"},
			ParamTypes:     []string{"float", "float", "float", "int"},
			SDKFunction: func(robot *vector.Vector, params map[string]interface{}, ctx logger.Logger) error {
				angle, angOk := params["angleRad"].(float32)
				speed, speedOk := params["speedRadPerSec"].(float32)
				accel, accelOk := params["accelRadPerSecSq"].(float32)
				dirInt, dirOk := params["direction"].(int32) // Assuming direction comes as int
				if !angOk || !speedOk || !accelOk || !dirOk {
					return fmt.Errorf("invalid parameters for CMD_TURN_IN_PLACE. Got: %+v", params)
				}
				// TurnInPlaceRequest uses int32 for direction, but SDK examples might use specific consts.
				// Here, we'll assume 0 = CLOCKWISE, 1 = COUNTER_CLOCKWISE as per plan.
				// The vectorpb.TurnInPlaceRequest_Direction enum is not directly used with integers in the Go SDK typically.
				// We might need to adjust if the SDK expects a specific enum or if 0/1 isn't standard.
				// For now, this is a direct interpretation. A wrapper in ExecuteSDKCommand might be better.
				_, err := robot.Conn.TurnInPlace(
					robot.Ctx,
					&vectorpb.TurnInPlaceRequest{
						AngleRad:         angle,
						SpeedRadPerSec:   speed,
						AccelRadPerSecSq: accel,
						Direction:        vectorpb.TurnInPlaceRequest_Direction(dirInt), // Casting to the enum type
						IsAbsolute:       0, // Relative turn
					},
				)
				return err
			},
		},
		{
			Command:        "CMD_SAY_TEXT",
			Description:    "Makes Vector say the provided text.",
			ExpectedParams: []string{"textToSay"},
			ParamTypes:     []string{"string"},
			SDKFunction: func(robot *vector.Vector, params map[string]interface{}, ctx logger.Logger) error {
				text, ok := params["textToSay"].(string)
				if !ok {
					return fmt.Errorf("invalid parameters for CMD_SAY_TEXT. Expected string. Got: %+v", params)
				}
				_, err := robot.Conn.SayText(
					robot.Ctx,
					&vectorpb.SayTextRequest{
						Text:           text,
						UseVectorVoice: true,
						DurationScalar: 1.0,
					},
				)
				return err
			},
		},
		{
			Command:        "CMD_MOVE_HEAD",
			Description:    "Moves Vector's head to a specific angle in radians.",
			ExpectedParams: []string{"angleRad"},
			ParamTypes:     []string{"float"},
			SDKFunction: func(robot *vector.Vector, params map[string]interface{}, ctx logger.Logger) error {
				angle, ok := params["angleRad"].(float32)
				if !ok {
					return fmt.Errorf("invalid parameters for CMD_MOVE_HEAD. Expected float. Got: %+v", params)
				}
				_, err := robot.Conn.MoveHead(
					robot.Ctx,
					&vectorpb.MoveHeadRequest{
						AngleRad:          angle,
						MaxSpeedRadPerSec: 2.0, // A sensible default, can be made a param
						DurationSec:       0,   // Let SDK determine duration based on speed and angle
						IdTag:             0,   // Not critical for simple moves
					},
				)
				return err
			},
		},
		{
			Command:        "CMD_MOVE_LIFT",
			Description:    "Moves Vector's lift to a specific height in millimeters (0.0 to 1.0 relative to max height, or absolute mm). For simplicity, using absolute mm from 0-~92mm.",
			ExpectedParams: []string{"heightMm"},
			ParamTypes:     []string{"float"}, // Changed to float to allow fractional mm, though SDK takes float32
			SDKFunction: func(robot *vector.Vector, params map[string]interface{}, ctx logger.Logger) error {
				height, ok := params["heightMm"].(float32)
				if !ok {
					return fmt.Errorf("invalid parameters for CMD_MOVE_LIFT. Expected float. Got: %+v", params)
				}
				_, err := robot.Conn.MoveLift(
					robot.Ctx,
					&vectorpb.MoveLiftRequest{
						HeightMm:          height,
						MaxSpeedRadPerSec: 2.0, // A sensible default, can be made a param
						DurationSec:       0,   // Let SDK determine duration
						IdTag:             0,
					},
				)
				return err
			},
		},
		{
			Command:        "CMD_TAKE_PHOTO_AND_CONTINUE",
			Description:    "Commands the autonomous loop to capture a new photo and send it with the next LLM prompt. This is a signal to the loop, not a direct robot hardware action beyond image capture (which is handled by the loop's getCameraImage).",
			ExpectedParams: []string{},
			ParamTypes:     []string{},
			SDKFunction:    nil, // No direct SDK function for this loop controller
		},
		{
			Command:        "CMD_STOP_AUTONOMOUS_MODE",
			Description:    "Commands the autonomous loop to stop and exit autonomous mode.",
			ExpectedParams: []string{},
			ParamTypes:     []string{},
			SDKFunction:    nil, // No direct SDK function for this loop controller
		},
	}
}

// ParseAutonomousCommand parses the LLM output string (e.g., "CMD_DRIVE_WHEELS(100, -100, 1000)")
// into a command name and a map of its parameters.
func ParseAutonomousCommand(llmOutput string) (commandName string, params map[string]interface{}, err error) {
	llmOutput = strings.TrimSpace(llmOutput)
	if !strings.HasSuffix(llmOutput, ")") {
		return "", nil, fmt.Errorf("command string does not end with ')'")
	}

	parts := strings.SplitN(llmOutput, "(", 2)
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("invalid command format, missing '('")
	}

	commandName = strings.TrimSpace(parts[0])
	rawParamsString := strings.TrimSuffix(parts[1], ")")
	params = make(map[string]interface{})

	var targetCommand *AutonomousCommand
	for _, cmd := range ValidAutonomousCommands {
		if cmd.Command == commandName {
			targetCommand = &cmd
			break
		}
	}

	if targetCommand == nil {
		return commandName, nil, fmt.Errorf("unknown command: %s", commandName)
	}

	// Handle commands with no parameters
	if rawParamsString == "" && len(targetCommand.ExpectedParams) == 0 {
		return commandName, params, nil
	}
    if rawParamsString == "" && len(targetCommand.ExpectedParams) > 0 {
        return commandName, nil, fmt.Errorf("command %s expects parameters, but none were provided", commandName)
    }


	paramValuesStr := strings.Split(rawParamsString, ",")

	if len(paramValuesStr) != len(targetCommand.ExpectedParams) {
		return commandName, nil, fmt.Errorf("command %s: expected %d parameters, got %d. Input: '%s'",
			commandName, len(targetCommand.ExpectedParams), len(paramValuesStr), rawParamsString)
	}

	for i, paramName := range targetCommand.ExpectedParams {
		valueStr := strings.TrimSpace(paramValuesStr[i])
		paramType := "string" // Default type
		if i < len(targetCommand.ParamTypes) {
			paramType = targetCommand.ParamTypes[i]
		}

		switch paramType {
		case "int":
			val, errConv := strconv.ParseInt(valueStr, 10, 32) // Using 32-bit int for typical SDK params
			if errConv != nil {
				return commandName, nil, fmt.Errorf("error parsing parameter '%s' for command %s: expected int, got '%s'", paramName, commandName, valueStr)
			}
			params[paramName] = int32(val) // Store as int32 or uint32 as appropriate
			if paramName == "durationMs" { // Specific case for DriveWheels
				params[paramName] = uint32(val)
			}
		case "float":
			val, errConv := strconv.ParseFloat(valueStr, 32)
			if errConv != nil {
				return commandName, nil, fmt.Errorf("error parsing parameter '%s' for command %s: expected float, got '%s'", paramName, commandName, valueStr)
			}
			params[paramName] = float32(val)
		case "string":
			// Remove surrounding quotes if LLM adds them, e.g. for CMD_SAY_TEXT("Hello there")
			if strings.HasPrefix(valueStr, "\"") && strings.HasSuffix(valueStr, "\"") {
				valueStr = strings.TrimSuffix(strings.TrimPrefix(valueStr, "\""), "\"")
			}
			if strings.HasPrefix(valueStr, "'") && strings.HasSuffix(valueStr, "'") {
				valueStr = strings.TrimSuffix(strings.TrimPrefix(valueStr, "'"), "'")
			}
			params[paramName] = valueStr
		case "bool":
			val, errConv := strconv.ParseBool(valueStr)
			if errConv != nil {
				return commandName, nil, fmt.Errorf("error parsing parameter '%s' for command %s: expected bool, got '%s'", paramName, commandName, valueStr)
			}
			params[paramName] = val
		default:
			return commandName, nil, fmt.Errorf("unknown parameter type '%s' for parameter '%s' in command %s", paramType, paramName, commandName)
		}
	}

	return commandName, params, nil
}

// GetCommandByName returns the AutonomousCommand struct for a given command name.
func GetCommandByName(name string) *AutonomousCommand {
	for _, cmd := range ValidAutonomousCommands {
		if cmd.Command == name {
			return &cmd
		}
	}
	return nil
}
