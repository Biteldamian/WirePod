package autonomous

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fforchino/vector-go-sdk/pkg/vector"
	"github.com/fforchino/vector-go-sdk/pkg/vectorpb"
	"github.com/kercre123/wire-pod/chipper/pkg/logger"
	"github.com/kercre123/wire-pod/chipper/pkg/vars"
	"github.com/sashabaranov/go-openai"
)

// getCameraImage captures a single high-resolution image from Vector's camera.
func getCameraImage(robot *vector.Vector, ctx context.Context) ([]byte, error) {
	logger.Println("AutonomousMode: Attempting to capture image...")
	_, err := robot.Conn.EnableMirrorMode(ctx, &vectorpb.EnableMirrorModeRequest{Enable: true})
	if err != nil {
		logger.Println("AutonomousMode: Failed to enable mirror mode:", err)
		// Not returning error immediately, as image capture might still work
	}
	defer robot.Conn.EnableMirrorMode(ctx, &vectorpb.EnableMirrorModeRequest{Enable: false})

	// Adding a small delay as sometimes enabling mirror mode and immediately capturing might not give the latest frame.
	time.Sleep(100 * time.Millisecond)

	resp, err := robot.Conn.CaptureSingleImage(ctx, &vectorpb.CaptureSingleImageRequest{EnableHighResolution: true})
	if err != nil {
		logger.Println("AutonomousMode: Failed to capture single image:", err)
		return nil, fmt.Errorf("failed to capture image: %w", err)
	}

	if len(resp.Data) == 0 {
		logger.Println("AutonomousMode: Captured image data is empty.")
		return nil, fmt.Errorf("captured image data is empty")
	}

	logger.Println("AutonomousMode: Image captured successfully, size:", len(resp.Data))
	return resp.Data, nil
}

// TODO: Implement ExecuteSDKCommand
// TODO: Implement AutonomousLLMLoop
// TODO: Adapt BControl and InterruptKGSimWhenTouchedOrWaked

// Placeholder for logger if not passed explicitly or available globally in this context
// var autonomousLogger logger.Logger = logger.NewLogger("autonomous") // Example

// BehaviorControl adapted from TTR
// BControl attempts to gain behavior control for Vector.
// It sends a ControlRequest and waits for a ControlGrantedResponse.
// start chan is sent true when control is gained.
// stop chan is used to signal the release of behavior control.
func BControl(robot *vector.Vector, ctx context.Context, start chan bool, stop chan bool, priority vectorpb.ControlRequest_Priority) {
	controlRequest := &vectorpb.BehaviorControlRequest{
		RequestType: &vectorpb.BehaviorControlRequest_ControlRequest{
			ControlRequest: &vectorpb.ControlRequest{
				Priority: priority,
			},
		},
	}

	go func() {
		r, err := robot.Conn.BehaviorControl(ctx)
		if err != nil {
			logger.Println("AutonomousMode: BehaviorControl stream error:", err)
			// Consider how to signal this failure to the main loop
			return
		}

		if err := r.Send(controlRequest); err != nil {
			logger.Println("AutonomousMode: BehaviorControl send error:", err)
			return
		}

		grantReceived := false
		for !grantReceived {
			ctrlresp, err := r.Recv()
			if err != nil {
				logger.Println("AutonomousMode: BehaviorControl recv error:", err)
				// If the context is cancelled (e.g. autonomous mode stopping), this error is expected.
				if ctx.Err() != nil {
					logger.Println("AutonomousMode: BehaviorControl context cancelled while waiting for grant.")
					return
				}
				return // Or attempt to signal error
			}
			if ctrlresp.GetControlGrantedResponse() != nil {
				logger.Println("AutonomousMode: Behavior control granted.")
				grantReceived = true
				select {
				case start <- true:
				case <-ctx.Done(): // Ensure we don't block if context is cancelled
					logger.Println("AutonomousMode: Context cancelled after behavior control granted but before start signaled.")
					// Release control if granted but loop is stopping
					r.Send(&vectorpb.BehaviorControlRequest{RequestType: &vectorpb.BehaviorControlRequest_ControlRelease{ControlRelease: &vectorpb.ControlRelease{}}})
					return
				}
				break
			} else if ctrlresp.GetControlLostResponse() != nil {
				logger.Println("AutonomousMode: Behavior control lost unexpectedly while waiting for grant.")
				return // Or attempt to signal error and stop
			}
		}

		// Wait for stop signal or context cancellation
		select {
		case <-stop:
			logger.Println("AutonomousMode: Releasing behavior control (stop signal).")
		case <-ctx.Done():
			logger.Println("AutonomousMode: Releasing behavior control (context cancelled).")
		}

		if err := r.Send(
			&vectorpb.BehaviorControlRequest{
				RequestType: &vectorpb.BehaviorControlRequest_ControlRelease{
					ControlRelease: &vectorpb.ControlRelease{},
				},
			},
		); err != nil {
			logger.Println("AutonomousMode: Error sending control release:", err)
		}
		logger.Println("AutonomousMode: Behavior control released.")
	}()
}

// InterruptAutonomousModeWhenTouchedOrWaked monitors for touch or wake word events to interrupt autonomous mode.
// stopLoop chan is sent true if an interruption event occurs.
// monitorCtx is the context for this monitoring goroutine.
func InterruptAutonomousModeWhenTouchedOrWaked(robot *vector.Vector, monitorCtx context.Context, stopLoop chan<- bool) {
	eventStream, err := robot.Conn.EventStream(
		monitorCtx,
		&vectorpb.EventRequest{
			ListType: &vectorpb.EventRequest_WhiteList{
				WhiteList: &vectorpb.FilterList{
					// Use RobotState for touch data, and WakeWord for voice interruption.
					// TouchDetectionEvent is another option if it provides simpler pressed/released state.
					// For now, using RobotState as it's confirmed in context for touch.
					List: []string{"robot_state", "wake_word"},
				},
			},
		},
	)
	if err != nil {
		logger.Println("AutonomousMode: Failed to create event stream for interruption:", err)
		return
	}
	logger.Println("AutonomousMode: Interrupt monitor started (touch/wake word).")

	var lastTouchValue uint32 = 0
	var touchInitialized bool = false
	const touchThreshold uint32 = 50 // How much change in raw touch value triggers an interrupt

	for {
		select {
		case <-monitorCtx.Done():
			logger.Println("AutonomousMode: Interrupt monitor stopping (context cancelled).")
			if eventStream != nil {
				eventStream.CloseSend() // Ensure stream is closed
			}
			return
		default:
			if eventStream == nil { // Should not happen if initial check passed
				logger.Println("AutonomousMode: Event stream is nil, cannot receive events for interruption.")
				return
			}
			resp, err := eventStream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "transport is closing") {
					logger.Println("AutonomousMode: Event stream closed or context cancelled for interrupt monitor.")
				} else {
					logger.Println("AutonomousMode: Error receiving from event stream for interruption:", err)
				}
				return // Stop monitoring on stream error or closure
			}

			event := resp.GetEvent()
			if event == nil {
				continue
			}

			switch event.EventType.(type) {
			case *vectorpb.Event_RobotState:
				touchData := event.GetRobotState().GetTouchData()
				if touchData != nil {
					currentTouchValue := touchData.GetRawTouchValue()
					if !touchInitialized {
						lastTouchValue = currentTouchValue
						touchInitialized = true
						logger.Println(fmt.Sprintf("AutonomousMode: Initial touch value: %d", lastTouchValue))
					} else {
						// Check for significant change from the last known value
						if currentTouchValue > lastTouchValue+touchThreshold || (lastTouchValue > currentTouchValue+touchThreshold && lastTouchValue > touchThreshold) { // handles press and release if sensitive enough
							logger.Println(fmt.Sprintf("AutonomousMode: Interrupting loop (source: touch sensor change from %d to %d).", lastTouchValue, currentTouchValue))
							select {
							case stopLoop <- true:
							default:
							}
							return // Stop monitoring
						}
						// Update lastTouchValue only if it's not a spike that would cause immediate re-trigger.
						// This simple logic might need refinement for noisy sensors.
						// A rolling average or debounce could be more robust.
						// For now, only update if it's a settled new value after a potential spike.
						// Or simply update it always: lastTouchValue = currentTouchValue
						// Let's update it to reflect the current baseline.
						lastTouchValue = currentTouchValue
					}
				}
			case *vectorpb.Event_WakeWord:
				logger.Println("AutonomousMode: Interrupting loop (source: wake word).")
				select {
				case stopLoop <- true:
				default:
				}
				return // Stop monitoring
			}
		}
	}
}

// ExecuteSDKCommand takes a command name and parameters, and executes the corresponding Vector SDK action.
// robot.Ctx should be used by SDKFunction for calls like robot.Conn.DriveWheels(robot.Ctx, ...)
// cmdCtx is the context for the execution of this specific command, can be used for timeouts if needed.
func ExecuteSDKCommand(robot *vector.Vector, commandName string, params map[string]interface{}, cmdCtx context.Context) error {
	logger.Println(fmt.Sprintf("AutonomousMode: Executing command: %s with params: %+v", commandName, params))

	cmdDef := GetCommandByName(commandName)
	if cmdDef == nil {
		return fmt.Errorf("unknown command: %s", commandName)
	}

	if cmdDef.SDKFunction != nil {
		// Pass a logger to SDKFunction if it needs one, otherwise remove `logger.NoLogger`
		// For now, assuming SDKFunction can use the global logger or doesn't need one.
		// Need to ensure robot.Ctx is correctly set up and passed if SDKFunction uses it.
		// The plan uses robot.Ctx which is part of the robot struct.
		// The cmdCtx passed to ExecuteSDKCommand is for the specific command execution.
		err := cmdDef.SDKFunction(robot, params, logger.Logger{}) // Pass a dummy logger for now
		if err != nil {
			logger.Println(fmt.Sprintf("AutonomousMode: Error executing SDKFunction for %s: %v", commandName, err))
			return err
		}
		logger.Println(fmt.Sprintf("AutonomousMode: Command %s executed successfully via SDKFunction.", commandName))
		return nil
	}

	// The GetCommandByName check already handles unknown commands.
	// If cmdDef is not nil but SDKFunction is, it's a setup error for that command.
	// The call cmdDef.SDKFunction() will panic if SDKFunction is nil.
	// So, an explicit check for nil SDKFunction can be added before calling it if desired,
	// but the current structure relies on GetCommandByName finding a fully defined command.
	// For now, if GetCommandByName returns a command, we assume SDKFunction is valid.
	// If it's nil, the cmdDef.SDKFunction call itself will indicate the problem (panic).
	// A more graceful way would be:
	if cmdDef.SDKFunction == nil {
		return fmt.Errorf("command %s is defined but has no SDKFunction assigned", commandName)
	}
	// This line is already present above the commented out switch:
	// err := cmdDef.SDKFunction(robot, params, logger.Logger{})
	// So, the switch is indeed not needed. The function will return based on SDKFunction's result.
	// The original return fmt.Errorf from the default case of the switch is now effectively covered by
	// the GetCommandByName check and the SDKFunction call itself (or the nil check above).
	// No change needed here if the SDKFunction is expected to be called directly as it is.
	// The previous return was inside a commented out switch, so it was not active.
	// The actual active part is the call to cmdDef.SDKFunction.
	// The function will return the error from cmdDef.SDKFunction or nil.
	// If GetCommandByName returned nil, that error is already handled.
	// The structure is fine.
	return nil // Should be unreachable if SDKFunction is always called and returns its own error. The actual return is from SDKFunction.
}

// AutonomousLLMLoop is the main loop for Vector's autonomous operation.
func AutonomousLLMLoop(robot *vector.Vector, robotSessionID string, robotDeviceID string) {
	logger.Println(fmt.Sprintf("AutonomousMode: Starting LLM Loop for robot %s (Session: %s)", robotDeviceID, robotSessionID))

	if !vars.APIConfig.AutonomousMode.Enable {
		logger.Println("AutonomousMode: Aborting, feature is not enabled in config.")
		// Optionally, inform the user via voice that the mode isn't enabled.
		robot.Conn.SayText(robot.Ctx, &vectorpb.SayTextRequest{Text: "Autonomous mode is not enabled in my settings.", UseVectorVoice: true, DurationScalar: 1.0})
		return
	}

	// Create a context for the entire autonomous loop operation, cancellable for graceful shutdown
	loopCtx, cancelLoop := context.WithCancel(robot.Ctx) // Inherit from robot's main context
	defer cancelLoop()                                   // Ensure cancellation on exit

	// Behavior Control
	behaviorControlStart := make(chan bool)
	behaviorControlStop := make(chan bool) // Used to signal BControl to release
	go BControl(robot, loopCtx, behaviorControlStart, behaviorControlStop, vectorpb.ControlRequest_OVERRIDE_BEHAVIORS)

	select {
	case <-behaviorControlStart:
		logger.Println("AutonomousMode: Behavior control successfully acquired.")
	case <-time.After(10 * time.Second): // Timeout for acquiring behavior control
		logger.Println("AutonomousMode: Timeout acquiring behavior control. Exiting.")
		robot.Conn.SayText(robot.Ctx, &vectorpb.SayTextRequest{Text: "I couldn't get control of my behaviors. Exiting autonomous mode.", UseVectorVoice: true, DurationScalar: 1.0})
		return
	case <-loopCtx.Done(): // This case might be hit if robot.Ctx was already cancelled
		logger.Println("AutonomousMode: Loop context cancelled while acquiring behavior control. Exiting.")
		return
	}

	// Ensure behavior control is released when AutonomousLLMLoop exits, for any reason.
	defer func() {
		logger.Println("AutonomousMode: Ensuring behavior control is released.")
		select {
		case behaviorControlStop <- true:
			logger.Println("AutonomousMode: Sent stop signal for behavior control.")
		case <-time.After(2 * time.Second): // Don't block forever if BControl goroutine is stuck
			logger.Println("AutonomousMode: Timeout sending stop signal for behavior control (it might have already stopped or context cancelled).")
		}
	}()


	// Interruption Monitoring
	interruptChan := make(chan bool, 1)
	monitorCtx, cancelMonitor := context.WithCancel(loopCtx) // monitorCtx is child of loopCtx
	defer cancelMonitor() // Ensure monitor goroutine is cleaned up
	go InterruptAutonomousModeWhenTouchedOrWaked(robot, monitorCtx, interruptChan)

	// LLM Client Initialization
	var llmClient *openai.Client
	provider := vars.APIConfig.AutonomousMode.LLMProvider
	apiKey := vars.APIConfig.AutonomousMode.LLMKey
	modelName := vars.APIConfig.AutonomousMode.Model
	systemPrompt := vars.APIConfig.AutonomousMode.SystemPrompt

	if apiKey == "" {
		logger.Println("AutonomousMode: LLM API key is not set. Exiting.")
		robot.Conn.SayText(loopCtx, &vectorpb.SayTextRequest{Text: "My LLM API key isn't set. I can't run autonomous mode.", UseVectorVoice: true, DurationScalar: 1.0})
		return
	}

	switch provider {
	case "openai":
		llmClient = openai.NewClient(apiKey)
	case "together":
		config := openai.DefaultConfig(apiKey)
		config.BaseURL = "https://api.together.xyz/v1"
		llmClient = openai.NewClientWithConfig(config)
	case "custom":
		customEndpoint := vars.APIConfig.AutonomousMode.Endpoint // Assuming custom endpoint specific to autonomous mode
		if customEndpoint == "" {
            // Fallback to general knowledge endpoint if specific one not set
            customEndpoint = vars.APIConfig.Knowledge.Endpoint
        }
		if customEndpoint == "" {
			logger.Println("AutonomousMode: Custom LLM provider selected but endpoint is not set. Exiting.")
			robot.Conn.SayText(loopCtx, &vectorpb.SayTextRequest{Text: "My custom LLM endpoint isn't set. Exiting autonomous mode.", UseVectorVoice: true, DurationScalar: 1.0})
			return
		}
		config := openai.DefaultConfig(apiKey)
		config.BaseURL = customEndpoint
		llmClient = openai.NewClientWithConfig(config)
	default:
		logger.Println(fmt.Sprintf("AutonomousMode: Unsupported LLM provider '%s'. Exiting.", provider))
		robot.Conn.SayText(loopCtx, &vectorpb.SayTextRequest{Text: fmt.Sprintf("LLM provider %s is not supported. Exiting autonomous mode.", provider), UseVectorVoice: true, DurationScalar: 1.0})
		return
	}

	if modelName == "" {
		logger.Println("AutonomousMode: LLM model name is not set. Exiting.")
		robot.Conn.SayText(loopCtx, &vectorpb.SayTextRequest{Text: "My LLM model name isn't set. Exiting autonomous mode.", UseVectorVoice: true, DurationScalar: 1.0})
		return
	}
	if systemPrompt == "" {
		logger.Println("AutonomousMode: System prompt is not set. Using a default.")
		systemPrompt = "You are Vector, an autonomous robot. Observe the image and issue a single command from the available list to interact with your environment or navigate. Available commands: CMD_DRIVE_WHEELS(leftSpeed, rightSpeed, durationMs), CMD_TURN_IN_PLACE(angleRad, speedRadPerSec, direction), CMD_SAY_TEXT(text), CMD_MOVE_HEAD(angleRad), CMD_MOVE_LIFT(heightMm), CMD_TAKE_PHOTO_AND_CONTINUE(), CMD_STOP_AUTONOMOUS_MODE(). Respond with ONLY the command string."
	}

	// Initial "wake up" animation or sound
	robot.Conn.SayText(loopCtx, &vectorpb.SayTextRequest{Text: "Entering autonomous mode.", UseVectorVoice: true, DurationScalar: 1.0})
	robot.Conn.PlayAnimation(loopCtx, &vectorpb.PlayAnimationRequest{Animation: &vectorpb.Animation{Name: "anim_wakeup_getout_01"}, Loops: 1})
	time.Sleep(1 * time.Second)

	// Main Loop
	for i := 0; i < vars.APIConfig.AutonomousMode.MaxLoopIterations; i++ {
		logger.Println(fmt.Sprintf("AutonomousMode: Loop iteration %d / %d", i+1, vars.APIConfig.AutonomousMode.MaxLoopIterations))

		select {
		case <-interruptChan:
			logger.Println("AutonomousMode: Interrupt signal received. Stopping loop.")
			robot.Conn.SayText(loopCtx, &vectorpb.SayTextRequest{Text: "Autonomous mode interrupted.", UseVectorVoice: true, DurationScalar: 1.0})
			goto endLoop
		case <-loopCtx.Done():
			logger.Println("AutonomousMode: Loop context cancelled externally. Stopping loop.")
			robot.Conn.SayText(loopCtx, &vectorpb.SayTextRequest{Text: "Autonomous mode stopped.", UseVectorVoice: true, DurationScalar: 1.0})
			goto endLoop
		default:
			// Continue
		}

		imageData, err := getCameraImage(robot, loopCtx)
		if err != nil {
			logger.Println("AutonomousMode: Failed to get camera image:", err)
			robot.Conn.PlayAnimation(loopCtx, &vectorpb.PlayAnimationRequest{Animation: &vectorpb.Animation{Name: "anim_feedback_facepalmerror_01"}, Loops: 1})
			if i == 0 { // Critical on first attempt
				robot.Conn.SayText(loopCtx, &vectorpb.SayTextRequest{Text: "Failed to get initial camera view. Exiting.", UseVectorVoice: true, DurationScalar: 1.0})
				goto endLoop
			}
			imageData = nil // Try to continue without image if not first iter
		}

		var chatMessages []openai.ChatCompletionMessage
		chatMessages = append(chatMessages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		})

		userMessageParts := []openai.ChatMessagePart{
			{Type: openai.ChatMessagePartTypeText, Text: "Current view is attached. What is the next command?"},
		}
		if imageData != nil && (strings.Contains(modelName, "gpt-4") || strings.Contains(modelName, "vision") || strings.Contains(modelName, "4o")) {
			imgBase64 := base64.StdEncoding.EncodeToString(imageData)
			imageURL := fmt.Sprintf("data:image/jpeg;base64,%s", imgBase64)
			userMessageParts = append(userMessageParts, openai.ChatMessagePart{
				Type: openai.ChatMessagePartTypeImageURL,
				ImageURL: &openai.ChatMessageImageURL{URL: imageURL, Detail: openai.ImageURLDetailAuto},
			})
			logger.Println("AutonomousMode: Image prepared for LLM.")
		} else if imageData == nil {
             userMessageParts[0].Text = "Failed to capture image. What is the next command based on prior actions or general knowledge?"
        } else {
			logger.Println("AutonomousMode: Model may not support images, or image support not explicitly coded. Sending text prompt only.")
			userMessageParts[0].Text = "No current image available (model might not support it). What is the next command based on prior actions or general knowledge?"
		}
		chatMessages = append(chatMessages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, MultiContent: userMessageParts})

		aiRequest := openai.ChatCompletionRequest{
			Model:       modelName,
			Messages:    chatMessages,
			MaxTokens:   150,
			Temperature: 0.6,
		}

		logger.Println("AutonomousMode: Sending request to LLM...")
		resp, err := llmClient.CreateChatCompletion(loopCtx, aiRequest)
		if err != nil {
			logger.Println("AutonomousMode: LLM request failed:", err)
			robot.Conn.PlayAnimation(loopCtx, &vectorpb.PlayAnimationRequest{Animation: &vectorpb.Animation{Name: "anim_feedback_connectionerror_01"}, Loops: 1})
			time.Sleep(2 * time.Second)
			continue
		}

		if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
			logger.Println("AutonomousMode: LLM returned empty response.")
			robot.Conn.PlayAnimation(loopCtx, &vectorpb.PlayAnimationRequest{Animation: &vectorpb.Animation{Name: "anim_knowledgegraph_noanswer_01"}, Loops: 1})
			time.Sleep(1 * time.Second)
			continue
		}
		llmCommandOutput := strings.TrimSpace(resp.Choices[0].Message.Content)
		logger.Println(fmt.Sprintf("AutonomousMode: LLM response: '%s'", llmCommandOutput))

		parsedCommandName, parsedParams, err := ParseAutonomousCommand(llmCommandOutput)
		if err != nil {
			logger.Println("AutonomousMode: Failed to parse LLM command:", err, ". Raw output:", llmCommandOutput)
			robot.Conn.PlayAnimation(loopCtx, &vectorpb.PlayAnimationRequest{Animation: &vectorpb.Animation{Name: "anim_knowledgegraph_confused_01"}, Loops: 1})
			// Optionally send error back to LLM for correction?
			continue
		}

		if parsedCommandName == "CMD_TAKE_PHOTO_AND_CONTINUE" {
			logger.Println("AutonomousMode: Received CMD_TAKE_PHOTO_AND_CONTINUE.")
			robot.Conn.PlayAnimation(loopCtx, &vectorpb.PlayAnimationRequest{Animation: &vectorpb.Animation{Name: "anim_phototaken_01"}, Loops: 1})
			continue
		}
		if parsedCommandName == "CMD_STOP_AUTONOMOUS_MODE" {
			logger.Println("AutonomousMode: Received CMD_STOP_AUTONOMOUS_MODE. Exiting loop.")
			robot.Conn.SayText(loopCtx, &vectorpb.SayTextRequest{Text: "Stopping autonomous mode.", UseVectorVoice: true, DurationScalar: 1.0})
			goto endLoop
		}

		cmdExecCtx, cancelCmdExec := context.WithTimeout(loopCtx, 20*time.Second)
		err = ExecuteSDKCommand(robot, parsedCommandName, parsedParams, cmdExecCtx)
		cancelCmdExec()
		if err != nil {
			logger.Println(fmt.Sprintf("AutonomousMode: Error executing command %s: %v", parsedCommandName, err))
			robot.Conn.PlayAnimation(loopCtx, &vectorpb.PlayAnimationRequest{Animation: &vectorpb.Animation{Name: "anim_feedback_facepalmerror_01"}, Loops: 1})
			// Potentially give LLM feedback about the error.
		} else {
			logger.Println(fmt.Sprintf("AutonomousMode: Command %s executed.", parsedCommandName))
			// robot.Conn.PlayAnimation(loopCtx, &vectorpb.PlayAnimationRequest{Animation: &vectorpb.Animation{Name: "anim_feedback_affirmative_01"}, Loops: 1})
		}
		time.Sleep(200 * time.Millisecond) // Small pause
	}

endLoop:
	logger.Println("AutonomousMode: LLM Loop finished or exited.")
	robot.Conn.PlayAnimation(loopCtx, &vectorpb.PlayAnimationRequest{Animation: &vectorpb.Animation{Name: "anim_goodbye_01"}, Loops: 1})
	// Release of behavior control is handled by the defer statement earlier.
	// Cancel the interrupt monitor explicitly if it hasn't already stopped due to loopCtx cancellation.
	cancelMonitor()
	logger.Println("AutonomousMode: AutonomousLLMLoop function finished.")
}
