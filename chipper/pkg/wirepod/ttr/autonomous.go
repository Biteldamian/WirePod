package wirepod_ttr

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fforchino/vector-go-sdk/pkg/vector"
	"github.com/fforchino/vector-go-sdk/pkg/vectorpb"
	"github.com/kercre123/wire-pod/chipper/pkg/logger"
	"github.com/kercre123/wire-pod/chipper/pkg/vars"
	"github.com/sashabaranov/go-openai"
)

// Global variable to store the Vector robot instance for autonomous mode
var autonomousRobot *vector.Vector
var autonomousCtx context.Context
var autonomousCancel context.CancelFunc
var autonomousRunning bool = false

// LLM client
var llmClient *openai.Client

// Configuration for autonomous mode
var autonomousConfig struct {
	LLMModel             string
	LLMPrompt            string
	CycleInterval        time.Duration
	ImageResolutionHigh  bool
	MaxConversationHistory int
}

// Constants for commands
const (
	CmdSayText       = "sayText"
	CmdDriveWheels   = "driveWheels"
	CmdTurnInPlace   = "turnInPlace"
	CmdMoveLift      = "moveLift"
	CmdSetLiftHeight = "setLiftHeight"
	CmdMoveHead      = "moveHead"
	CmdSetHeadAngle  = "setHeadAngle"
	CmdPlayAnim      = "playAnimation"
	CmdGetImage      = "getCameraImage" // LLM might request this, loop handles it
)

// InitializeLLMClient sets up the LLM client based on global wire-pod config
func InitializeLLMClient() error {
	// Default configuration
	autonomousConfig.LLMModel = openai.GPT4oMini // Default to GPT-4o Mini
	autonomousConfig.LLMPrompt = "You are Vector, an autonomous robot. You will be given images from your camera. Respond with commands to interact with your environment or speak. Available commands: {{sayText||text_to_say}}, {{driveWheels||left_speed_mmps,right_speed_mmps,duration_ms}}, {{turnInPlace||angle_deg,speed_dps}}, {{moveLift||speed_dps}}, {{setLiftHeight||height_0_to_1}}, {{moveHead||speed_dps}}, {{setHeadAngle||angle_deg}}, {{playAnimation||anim_alias}}, {{getCameraImage||now}}. Example: 'I see a person. {{sayText||Hello!}} {{driveWheels||50,50,1000}}'"
	autonomousConfig.CycleInterval = 10 * time.Second // How often to take a picture and query LLM
	autonomousConfig.ImageResolutionHigh = false      // Low-res for speed by default
	autonomousConfig.MaxConversationHistory = 10     // Number of past user/assistant messages to keep

	// Override with specific autonomous mode settings from vars.APIConfig if they exist
	// For now, we use the general Knowledge Graph settings as a base.
	// TODO: Add specific [autonomous] section to apiConfig.json in vars/config.go

	if vars.APIConfig.Knowledge.Provider == "openai" {
		llmClient = openai.NewClient(vars.APIConfig.Knowledge.Key)
		if vars.APIConfig.Knowledge.Model != "" { // If a model is specified in general config
			autonomousConfig.LLMModel = vars.APIConfig.Knowledge.Model
		}
	} else if vars.APIConfig.Knowledge.Provider == "together" {
		conf := openai.DefaultConfig(vars.APIConfig.Knowledge.Key)
		conf.BaseURL = "https://api.together.xyz/v1"
		llmClient = openai.NewClientWithConfig(conf)
		if vars.APIConfig.Knowledge.Model == "" {
			autonomousConfig.LLMModel = "meta-llama/Llama-3-70b-chat-hf" // Default for Together
		} else {
			autonomousConfig.LLMModel = vars.APIConfig.Knowledge.Model
		}
	} else if vars.APIConfig.Knowledge.Provider == "custom" {
		conf := openai.DefaultConfig(vars.APIConfig.Knowledge.Key)
		conf.BaseURL = vars.APIConfig.Knowledge.Endpoint
		llmClient = openai.NewClientWithConfig(conf)
		if vars.APIConfig.Knowledge.Model != "" {
			autonomousConfig.LLMModel = vars.APIConfig.Knowledge.Model
		} else {
			// Requires a model to be specified for custom endpoints if not defaulting to GPT-4o Mini
			logger.Println("Autonomous Mode: Custom LLM provider configured but no model specified. Defaulting to gpt-4o-mini if compatible, otherwise might fail.")
		}
	} else {
		logger.Println("Autonomous Mode: LLM provider not configured or not supported.")
		return errors.New("LLM provider not configured or supported for autonomous mode")
	}

	// Use a specific autonomous prompt if available in config, otherwise use default
	// TODO: Add vars.APIConfig.Autonomous.Prompt
	// if vars.APIConfig.Autonomous.Prompt != "" {
	// autonomousConfig.LLMPrompt = vars.APIConfig.Autonomous.Prompt
	// }

	logger.Println("Autonomous Mode: LLM Client Initialized. Provider:", vars.APIConfig.Knowledge.Provider, "Model:", autonomousConfig.LLMModel)
	return nil
}

// StartAutonomousMode is called when the intent "Hey Vector, start autonomous mode" is triggered.
func StartAutonomousMode(bot *vector.Vector, _ context.Context, intentParams map[string]string) error {
	if autonomousRunning {
		logger.Println("Autonomous mode is already running.")
		// DoSayText("I'm already in autonomous mode.", bot) // Requires behavior control
		return errors.New("autonomous mode is already running")
	}

	logger.Println("Starting Autonomous Mode for robot:", bot.Cfg.SerialNo)
	autonomousRobot = bot
	autonomousCtx, autonomousCancel = context.WithCancel(context.Background())
	autonomousRunning = true

	if llmClient == nil {
		if err := InitializeLLMClient(); err != nil {
			autonomousRunning = false
			logger.Println("Failed to initialize LLM client for autonomous mode:", err)
			// DoSayText("I couldn't set up my brain for autonomous mode.", bot) // Requires behavior control
			return err
		}
	}

	// Announce start of autonomous mode
	go func() {
		startChan := make(chan bool)
		stopChan := make(chan bool)
		BControl(autonomousRobot, autonomousCtx, startChan, stopChan) // Use the one from kgsim.go for consistency
		select {
		case <-startChan:
			logger.Println("AutonomousMode: Behavior control acquired for startup message.")
			DoSayText("Starting autonomous mode. I will now observe and act.", autonomousRobot)
			stopChan <- true // Release behavior control
			logger.Println("AutonomousMode: Behavior control released after startup message.")
			// Start the main loop only after the initial message is successfully delivered and control released.
			go AutonomousLLMLoop()
		case <-time.After(5 * time.Second):
			logger.Println("AutonomousMode: Timeout acquiring behavior control for startup message.")
			autonomousRunning = false // Critical failure, can't start
			autonomousCancel()      // Cancel the context if loop was prematurely started by mistake
			// Not starting the loop.
		case <-autonomousCtx.Done():
			logger.Println("AutonomousMode: Startup cancelled.")
			// Not starting the loop
		}
	}()

	return nil
}

// StopAutonomousMode stops the autonomous operation
func StopAutonomousMode() error {
	if !autonomousRunning {
		logger.Println("Autonomous mode is not running.")
		return errors.New("autonomous mode is not running")
	}
	logger.Println("Stopping Autonomous Mode")
	if autonomousCancel != nil {
		autonomousCancel()
	}
	autonomousRunning = false

	if autonomousRobot != nil {
		// Announce stop of autonomous mode
		// Use a new, short-lived context for this final message.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		startChan := make(chan bool)
		stopChan := make(chan bool)
		// Use BControl with the new short-lived context
		BControl(autonomousRobot, ctx, startChan, stopChan)
		select {
		case <-startChan:
			logger.Println("AutonomousMode: Behavior control acquired for shutdown message.")
			DoSayText("Autonomous mode stopped.", autonomousRobot)
			stopChan <- true // Release behavior control
			logger.Println("AutonomousMode: Behavior control released after shutdown message.")
		case <-time.After(5 * time.Second):
			logger.Println("AutonomousMode: Timeout acquiring behavior control for shutdown message.")
		case <-ctx.Done(): // Handles overall timeout for this operation
			logger.Println("AutonomousMode: Shutdown message context done.")
		}
	}
	autonomousRobot = nil
	return nil
}

// AutonomousLLMLoop is the main operational loop
func AutonomousLLMLoop() {
	logger.Println("AutonomousLLMLoop started for robot:", autonomousRobot.Cfg.SerialNo)

	var messages []openai.ChatCompletionMessage
	systemMessage := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: autonomousConfig.LLMPrompt,
	}
	messages = append(messages, systemMessage)

	ticker := time.NewTicker(autonomousConfig.CycleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-autonomousCtx.Done():
			logger.Println("AutonomousLLMLoop: Context cancelled, exiting loop.")
			return
		case <-ticker.C:
			if !autonomousRunning {
				logger.Println("AutonomousLLMLoop: autonomousRunning is false, exiting.")
				return
			}
			performAutonomousCycle(&messages)
		}
	}
}

func performAutonomousCycle(messages *[]openai.ChatCompletionMessage) {
	logger.Println("AutonomousLLMLoop: Starting new cycle.")

	// 1. Get camera image
	imageData, err := getCameraImage(autonomousRobot, autonomousCtx, autonomousConfig.ImageResolutionHigh)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Println("AutonomousLLMLoop: Image capture cancelled.")
			return
		}
		logger.Println("AutonomousLLMLoop: Error getting camera image:", err)
		return // Skip this cycle on error
	}

	// 2. Prepare prompt for LLM
	// TODO: Add other sensor data if needed (e.g., battery, is_picked_up)
	currentMessageContent := []openai.ChatMessagePart{
		{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL:    fmt.Sprintf("data:image/jpeg;base64,%s", imageData),
				Detail: openai.ImageURLDetailAuto, // Let OpenAI decide, or use config
			},
		},
		{
			Type: openai.ChatMessagePartTypeText,
			Text: "Current view. What should I do?", // TODO: Make this more dynamic or configurable
		},
	}

	// Append current observation as a user message
	userObservationMessage := openai.ChatCompletionMessage{
		Role:         openai.ChatMessageRoleUser,
		MultiContent: currentMessageContent,
	}
	activeMessages := append(*messages, userObservationMessage)


	// 3. Send to LLM
	if llmClient == nil {
		logger.Println("AutonomousLLMLoop: LLM client is not initialized. Attempting re-initialization.")
		if initErr := InitializeLLMClient(); initErr != nil {
			logger.Println("AutonomousLLMLoop: Failed to re-initialize LLM client:", initErr)
			return // Skip cycle
		}
	}

	llmRequest := openai.ChatCompletionRequest{
		Model:     autonomousConfig.LLMModel,
		Messages:  activeMessages, // Send the current conversation history + new observation
		MaxTokens: 250,          // Configurable?
		// Temperature, TopP can also be made configurable via autonomousConfig
	}

	logger.Println("AutonomousLLMLoop: Sending request to LLM. Model:", llmRequest.Model, "NumMessages:", len(llmRequest.Messages))
	resp, err := llmClient.CreateChatCompletion(autonomousCtx, llmRequest)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Println("AutonomousLLMLoop: LLM request cancelled.")
			return
		}
		logger.Println("AutonomousLLMLoop: Error from LLM:", err)
		// Consider specific error handling, e.g., for context length errors.
		return // Skip cycle on error
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		logger.Println("AutonomousLLMLoop: LLM returned no content.")
		return // Skip cycle
	}

	llmResponseText := resp.Choices[0].Message.Content
	logger.Println("AutonomousLLMLoop: LLM Response:", llmResponseText)

	// Add user observation and LLM response to conversation history
	*messages = append(*messages, userObservationMessage)
	*messages = append(*messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: llmResponseText,
	})

	// Trim message history
	if len(*messages) > autonomousConfig.MaxConversationHistory {
		// Keep system prompt + last N messages
		keepFrom := len(*messages) - (autonomousConfig.MaxConversationHistory -1) // -1 because system prompt is 1
		if keepFrom < 1 { keepFrom = 1} // Should always keep system prompt

		// Create a new slice, starting with the system prompt, then the tail of the messages
		newMessages := []openai.ChatCompletionMessage{(*messages)[0]} // Keep the system prompt
		newMessages = append(newMessages, (*messages)[keepFrom:]...)
    *messages = newMessages
	}


	// 4. Parse and Execute Commands
	startBehaviorCtrl := make(chan bool)
	stopBehaviorCtrl := make(chan bool)

	// Use a new context for this specific set of actions to allow cancellation if autonomous mode is stopped
	// This actionCtx should also have a timeout to prevent actions from running indefinitely if behavior control is lost
	actionCtx, actionCancel := context.WithTimeout(autonomousCtx, 30*time.Second) // Example: 30s timeout for all actions in a cycle
	defer actionCancel()

	BControl(autonomousRobot, actionCtx, startBehaviorCtrl, stopBehaviorCtrl)

	select {
	case <-startBehaviorCtrl:
		logger.Println("AutonomousLLMLoop: Behavior control acquired for LLM commands.")
		// Use the new ParseAutonomousActions which returns []AutonomousRobotAction
		parsedActions := ParseAutonomousActions(llmResponseText)
		ExecuteAutonomousCommands(actionCtx, autonomousRobot, parsedActions, stopBehaviorCtrl)
	case <-time.After(10 * time.Second): // Timeout for acquiring behavior control itself
		logger.Println("AutonomousLLMLoop: Timeout acquiring behavior control for LLM commands.")
	case <-actionCtx.Done(): // Handles timeout for actions or if autonomousCtx was cancelled
		logger.Println("AutonomousLLMLoop: Action context done (cancelled or timed out) before/during command execution.")
	case <-autonomousCtx.Done(): // If the main autonomous mode is stopped
		logger.Println("AutonomousLLMLoop: Autonomous mode stopped while waiting for behavior control for commands.")
	}
	// Ensure actionCancel is called if not using defer, or rely on defer.
	// stopBehaviorCtrl is signaled by ExecuteAutonomousCommands or by BControl if actionCtx is done.
}

// getCameraImage captures an image from Vector's camera
func getCameraImage(robot *vector.Vector, ctx context.Context, highRes bool) (string, error) {
	if robot == nil || robot.Conn == nil {
		return "", errors.New("getCameraImage: robot connection not available")
	}
	logger.Println("getCameraImage: Capturing image (HighRes:", highRes, ")")

	captureReq := &vectorpb.CaptureSingleImageRequest{
		EnableHighResolution: highRes,
	}

	resp, err := robot.Conn.CaptureSingleImage(ctx, captureReq)
	if err != nil {
		logger.Println("getCameraImage: Error capturing image:", err)
		return "", err
	}

	imgBase64 := base64.StdEncoding.EncodeToString(resp.Data)
	logger.Println("getCameraImage: Image captured and encoded.")
	return imgBase64, nil
}

// AutonomousRobotAction defines the structure for parsed commands specific to autonomous mode
type AutonomousRobotAction struct {
    Command   string
    Parameter string
}

// ExecuteAutonomousCommands takes a list of parsed AutonomousRobotAction and executes them
func ExecuteAutonomousCommands(ctx context.Context, robot *vector.Vector, actions []AutonomousRobotAction, stopBehaviorCtrl chan bool) {
	defer func() {
		select {
		case stopBehaviorCtrl <- true:
			logger.Println("ExecuteAutonomousCommands: Behavior control release signaled.")
		case <-ctx.Done():
			logger.Println("ExecuteAutonomousCommands: Context done before signaling behavior control release.")
		default:
			logger.Println("ExecuteAutonomousCommands: Behavior control channel likely already handled by BControl due to context.")
		}
	}()

	if robot == nil || robot.Conn == nil {
		logger.Println("ExecuteAutonomousCommands: Robot connection not available.")
		return
	}

	for _, action := range actions {
		select {
		case <-ctx.Done():
			logger.Println("ExecuteAutonomousCommands: Context cancelled, stopping further actions.")
			return
		default:
			// Proceed
		}

		logger.Println("ExecuteAutonomousCommands: Executing:", action.Command, "Params:", action.Parameter)
		switch action.Command {
		case CmdSayText:
			if err := DoSayText(action.Parameter, robot); err != nil { // Using DoSayText from kgsim_cmds
				logger.Println("Error executing SayText:", err)
			}
		case CmdDriveWheels: // Params: "left_mmps,right_mmps,duration_ms"
			params := strings.Split(action.Parameter, ",")
			if len(params) == 3 {
				leftSpeed, errL := parseFloat(strings.TrimSpace(params[0]))
				rightSpeed, errR := parseFloat(strings.TrimSpace(params[1]))
				durationMs, errD := parseInt(strings.TrimSpace(params[2]))
				if errL == nil && errR == nil && errD == nil && durationMs > 0 {
					logger.Println(fmt.Sprintf("Driving wheels: left=%.2f mmps, right=%.2f mmps, duration=%d ms", leftSpeed, rightSpeed, durationMs))
					_, err := robot.Conn.DriveWheels(
						ctx,
						&vectorpb.DriveWheelsRequest{
							LeftWheelMmps:  float32(leftSpeed),
							RightWheelMmps: float32(rightSpeed),
							DurationMs:     uint32(durationMs),
						},
					)
					if err != nil {
						if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "rpc error: code = Canceled") {
							logger.Println("Error executing DriveWheels:", err)
						}
					} else {
						select {
						case <-time.After(time.Duration(durationMs) * time.Millisecond):
						case <-ctx.Done():
							logger.Println("DriveWheels wait interrupted by context cancellation.")
							return
						}
					}
				} else {
					logger.Println("Invalid parameters for driveWheels:", action.Parameter, errL, errR, errD)
				}
			} else {
				logger.Println("Incorrect number of parameters for driveWheels:", action.Parameter)
			}
		case CmdTurnInPlace: // Params: "angle_degrees,speed_dps"
			params := strings.Split(action.Parameter, ",")
			if len(params) == 2 {
				angleDeg, errA := parseFloat(strings.TrimSpace(params[0]))
				speedDps, errS := parseFloat(strings.TrimSpace(params[1]))
				if errA == nil && errS == nil {
					angleRad := float32(angleDeg * (math.Pi / 180.0))
					speedRadPerSec := float32(speedDps * (math.Pi / 180.0))
					logger.Println(fmt.Sprintf("Turning in place: angle=%.2f deg, speed=%.2f dps", angleDeg, speedDps))
					_, err := robot.Conn.TurnInPlace(ctx, &vectorpb.TurnInPlaceRequest{
						AngleRad:         angleRad,
						SpeedRadPerSec:   speedRadPerSec,
						IsAbsolute:       0,
						ToleranceRad:     float32(10.0 * (math.Pi / 180.0)),
					})
					if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "rpc error: code = Canceled") {
						logger.Println("Error executing TurnInPlace:", err)
					}
				} else {
					logger.Println("Invalid parameters for turnInPlace:", action.Parameter, errA, errS)
				}
			} else {
				logger.Println("Incorrect number of parameters for turnInPlace:", action.Parameter)
			}
		case CmdMoveLift:    // Param: "speed_dps" (positive for up, negative for down)
			speedDps, err := parseFloat(strings.TrimSpace(action.Parameter))
			if err == nil {
				speedRadPerSec := float32(speedDps * (math.Pi / 180.0))
				logger.Println(fmt.Sprintf("Moving lift at speed: %.2f dps", speedDps))
				_, err := robot.Conn.MoveLift(ctx, &vectorpb.MoveLiftRequest{SpeedRadPerSec: speedRadPerSec})
				if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "rpc error: code = Canceled") {
					logger.Println("Error executing MoveLift:", err)
				}
			} else {
				logger.Println("Invalid parameters for moveLift:", action.Parameter, err)
			}
		case CmdSetLiftHeight: // Param: "height_ratio_0_to_1"
			heightRatio, err := parseFloat(strings.TrimSpace(action.Parameter))
			if err == nil {
				if heightRatio < 0.0 { heightRatio = 0.0 }
				if heightRatio > 1.0 { heightRatio = 1.0 }
				logger.Println(fmt.Sprintf("Setting lift height to: %.2f", heightRatio))
				// vector.LiftMaxHeightMM is not directly available; using a typical value or making it configurable.
				// From SDK source: const liftTravelMM = 31.0; MinLiftHeightMM = 32.0; MaxLiftHeightMM = MinLiftHeightMM + liftTravelMM = 63.0
				// So, height is absolute from 32 to 63. Ratio needs to map to this.
				// Height = MinLiftHeight + ratio * (MaxLiftHeight - MinLiftHeight)
				const minLiftSDKMM float32 = 32.0
				const maxLiftSDKMM float32 = 63.0
				targetHeightMM := minLiftSDKMM + float32(heightRatio)*(maxLiftSDKMM-minLiftSDKMM)

				_, err := robot.Conn.SetLiftHeight(ctx, &vectorpb.SetLiftHeightRequest{HeightMm: targetHeightMM})
				if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "rpc error: code = Canceled") {
					logger.Println("Error executing SetLiftHeight:", err)
				}
			} else {
				logger.Println("Invalid parameters for setLiftHeight:", action.Parameter, err)
			}
		case CmdMoveHead:    // Param: "speed_dps"
			speedDps, err := parseFloat(strings.TrimSpace(action.Parameter))
			if err == nil {
				speedRadPerSec := float32(speedDps * (math.Pi / 180.0))
				logger.Println(fmt.Sprintf("Moving head at speed: %.2f dps", speedDps))
				_, err := robot.Conn.MoveHead(ctx, &vectorpb.MoveHeadRequest{SpeedRadPerSec: speedRadPerSec})
				if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "rpc error: code = Canceled") {
					logger.Println("Error executing MoveHead:", err)
				}
			} else {
				logger.Println("Invalid parameters for moveHead:", action.Parameter, err)
			}
		case CmdSetHeadAngle: // Param: "angle_degrees"
			angleDeg, err := parseFloat(strings.TrimSpace(action.Parameter))
			if err == nil {
				angleRad := float32(angleDeg * (math.Pi / 180.0))
				// Constants from vector-go-sdk/pkg/vector/robot.go
				const minHeadAngleRad float32 = -0.38397244 // RADIANS_PER_DEGREE * -22.0
		const maxHeadAngleRad float32 = 0.78539816  // RADIANS_PER_DEGREE * 45.0
				if angleRad < minHeadAngleRad { angleRad = minHeadAngleRad }
				if angleRad > maxHeadAngleRad { angleRad = maxHeadAngleRad }
				logger.Println(fmt.Sprintf("Setting head angle to: %.2f deg (%.2f rad)", angleDeg, angleRad))
				_, err := robot.Conn.SetHeadAngle(ctx, &vectorpb.SetHeadAngleRequest{AngleRad: angleRad, MaxSpeedRadPerSec: float32(100.0 * math.Pi / 180.0)})
				if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "rpc error: code = Canceled") {
					logger.Println("Error executing SetHeadAngle:", err)
				}
			} else {
				logger.Println("Invalid parameters for setHeadAngle:", action.Parameter, err)
			}
		case CmdPlayAnim: // Param: "animation_alias_or_trigger_name"
			animName := getAnimationName(action.Parameter) // Uses kgsim_cmds animationMap
			if animName != "" {
				logger.Println("Playing animation:", animName)
				_, err := robot.Conn.PlayAnimation(
					ctx,
					&vectorpb.PlayAnimationRequest{
						Animation: &vectorpb.Animation{Name: animName},
						Loops:     1,
					},
				)
				if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "rpc error: code = Canceled") {
					logger.Println("Error executing PlayAnimation:", err)
				}
			} else {
				logger.Println("Unknown animation alias for playAnimation:", action.Parameter)
			}
		case CmdGetImage:
			logger.Println("LLM requested getCameraImage. Next cycle will provide a new image.")
		default:
			logger.Println("Unknown command in ExecuteAutonomousCommands:", action.Command)
		}
		select {
		case <-time.After(200 * time.Millisecond):
		case <-ctx.Done():
			logger.Println("Command delay interrupted by context cancellation.")
			return
		}
	}
	logger.Println("ExecuteAutonomousCommands: Finished all actions in list.")
}

// Helper function to parse integer from string
func parseInt(s string) (int, error) {
	val, err := strconv.Atoi(s)
	if err != nil {
		fVal, fErr := strconv.ParseFloat(s, 32)
		if fErr == nil {
			return int(fVal), nil
		}
		return 0, err
	}
	return val, nil
}

// Helper function to parse float from string
func parseFloat(s string) (float64, error) {
	val, err := strconv.ParseFloat(s, 64)
	return val, err
}

// ParseAutonomousActions parses the LLM's response text for commands.
func ParseAutonomousActions(inputText string) []AutonomousRobotAction {
    var actions []AutonomousRobotAction

    cmdRegex := regexp.MustCompile(`\{\{([^|}]+)\|\|([^}]+)\}\}`)

    lastIndex := 0
    foundMatches := cmdRegex.FindAllStringSubmatchIndex(inputText, -1)

    for _, matchIndices := range foundMatches {
        fullMatchStart := matchIndices[0]
        fullMatchEnd := matchIndices[1]
        cmdStart := matchIndices[2]
        cmdEnd := matchIndices[3]
        paramStart := matchIndices[4]
        paramEnd := matchIndices[5]

        if fullMatchStart > lastIndex {
            textBefore := strings.TrimSpace(inputText[lastIndex:fullMatchStart])
            if textBefore != "" {
                actions = append(actions, AutonomousRobotAction{Command: CmdSayText, Parameter: textBefore})
            }
        }

        command := strings.TrimSpace(inputText[cmdStart:cmdEnd])
        parameter := strings.TrimSpace(inputText[paramStart:paramEnd])
        actions = append(actions, AutonomousRobotAction{Command: command, Parameter: parameter})

        lastIndex = fullMatchEnd
    }

    if lastIndex < len(inputText) {
        textAfter := strings.TrimSpace(inputText[lastIndex:])
        if textAfter != "" {
            actions = append(actions, AutonomousRobotAction{Command: CmdSayText, Parameter: textAfter})
        }
    }

    if len(foundMatches) == 0 && strings.TrimSpace(inputText) != "" {
        actions = append(actions, AutonomousRobotAction{Command: CmdSayText, Parameter: strings.TrimSpace(inputText)})
    }

    return actions
}
