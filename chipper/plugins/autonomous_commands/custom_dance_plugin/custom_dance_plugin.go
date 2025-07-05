package main

import (
	"fmt"
	"time"

	"github.com/fforchino/vector-go-sdk/pkg/vector"
	"github.com/fforchino/vector-go-sdk/pkg/vectorpb"
	// "github.com/kercre123/wire-pod/chipper/pkg/logger" // Uncomment if specific logging is needed
)

// CommandName is the command string the LLM would use (e.g., "CMD_CUSTOM_DANCE")
var CommandName string = "CMD_CUSTOM_DANCE"

// ExpectedParams defines the parameters this command expects from the LLM.
// Example: ["style", "durationSeconds"]
var ExpectedParams []string = []string{"style", "durationSeconds"}

// Description for display or for the LLM prompt.
var Description string = "Performs a custom dance. Style can be 'wiggle' or 'spin'. Duration is in seconds."

// Execute is the function that will be called by the autonomous loop if this plugin is registered and matched.
// It receives the robot instance and the parsed parameters from the LLM.
// The main autonomous.go would need to be modified to load and call such plugins.
func Execute(robot *vector.Vector, params map[string]interface{}) error {
	// Example using global logger, or pass logger instance if framework supports it
	// logger.Println("Executing CMD_CUSTOM_DANCE plugin")

	style, styleOk := params["style"].(string)

	// LLM might send numbers as float64, so we need to handle that conversion robustly.
	var durationSec float64
	durationInput, durOk := params["durationSeconds"]
	if durOk {
		switch v := durationInput.(type) {
		case float64:
			durationSec = v
		case float32:
			durationSec = float64(v)
		case int:
			durationSec = float64(v)
		case int32:
			durationSec = float64(v)
		case int64:
			durationSec = float64(v)
		default:
			return fmt.Errorf("parameter 'durationSeconds' is of an unexpected type: %T", durationInput)
		}
	} else {
        return fmt.Errorf("parameter 'durationSeconds' not provided or of wrong type for CMD_CUSTOM_DANCE. Got: %+v", params)
    }


	if !styleOk {
		return fmt.Errorf("parameter 'style' not provided or of wrong type for CMD_CUSTOM_DANCE. Got: %+v", params)
	}


	// Using robot.Ctx which should be the context from the robot's instance, managed by the autonomous loop
	ctx := robot.Ctx

	if style == "wiggle" {
		// Example: robot.Conn.SayText(ctx, &vectorpb.SayTextRequest{Text: "Wiggle time!", UseVectorVoice: true})
		startTime := time.Now()
		for time.Since(startTime).Seconds() < durationSec {
			_, err := robot.Conn.DriveWheels(ctx, &vectorpb.DriveWheelsRequest{LeftWheelMmps: 50, RightWheelMmps: -50, DurationMs: 200})
			if err != nil { return fmt.Errorf("wiggle drive error: %w", err) }
			time.Sleep(200 * time.Millisecond) // Ensure this sleep doesn't exceed context timeout in a real scenario
			if ctx.Err() != nil { return ctx.Err() }


			_, err = robot.Conn.DriveWheels(ctx, &vectorpb.DriveWheelsRequest{LeftWheelMmps: -50, RightWheelMmps: 50, DurationMs: 200})
			if err != nil { return fmt.Errorf("wiggle drive error: %w", err) }
			time.Sleep(200 * time.Millisecond)
			if ctx.Err() != nil { return ctx.Err() }
		}
	} else if style == "spin" {
		// Example: robot.Conn.SayText(ctx, &vectorpb.SayTextRequest{Text: "Spinning!", UseVectorVoice: true})
		_, err := robot.Conn.DriveWheels(ctx, &vectorpb.DriveWheelsRequest{LeftWheelMmps: 100, RightWheelMmps: -100, DurationMs: uint32(durationSec * 1000)})
		if err != nil { return fmt.Errorf("spin drive error: %w", err) }
		// The DriveWheels with DurationMs is non-blocking in terms of this function call,
		// but the robot will be busy. A sleep here simulates waiting for the action.
		// In a real plugin integrated into the loop, the loop would wait or the action would be truly async.
		time.Sleep(time.Duration(durationSec * float64(time.Second)))
		if ctx.Err() != nil { return ctx.Err() }
	} else {
		return fmt.Errorf("unknown dance style: %s", style)
	}
	return nil
}

// Init function (optional, if the plugin needs setup when loaded by a new plugin framework).
// func Init() error {
//    // logger.Println("Custom Dance Plugin Initialized by Autonomous Framework")
//    return nil
// }

// To be runnable as a standalone Go program for testing (won't work as a .so plugin directly this way)
// func main() {
//    fmt.Println("This is a plugin, not meant to be run directly without a loader.")
//    fmt.Printf("Command: %s\nDescription: %s\nParams: %v\n", CommandName, Description, ExpectedParams)
// }
