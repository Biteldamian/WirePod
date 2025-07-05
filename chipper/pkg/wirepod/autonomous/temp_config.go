package vars

import (
	"encoding/json"
	"os"

	"github.com/kercre123/wire-pod/chipper/pkg/logger"
)

// a way to create a JSON configuration for wire-pod, rather than the use of env vars

var ApiConfigPath = "./apiConfig.json" // This path might need adjustment if vars package is moved/used elsewhere

var APIConfig apiConfig

type apiConfig struct {
	Weather struct {
		Enable   bool   `json:"enable"`
		Provider string `json:"provider"`
		Key      string `json:"key"`
		Unit     string `json:"unit"`
	} `json:"weather"`
	Knowledge struct {
		Enable                 bool    `json:"enable"`
		Provider               string  `json:"provider"`
		Key                    string  `json:"key"`
		ID                     string  `json:"id"`
		Model                  string  `json:"model"`
		IntentGraph            bool    `json:"intentgraph"`
		RobotName              string  `json:"robotName"`
		OpenAIPrompt           string  `json:"openai_prompt"`
		OpenAIVoice            string  `json:"openai_voice"`
		OpenAIVoiceWithEnglish bool    `json:"openai_voice_with_english"`
		SaveChat               bool    `json:"save_chat"`
		CommandsEnable         bool    `json:"commands_enable"`
		Endpoint               string  `json:"endpoint"`
		TopP                   float32 `json:"top_p"`
		Temperature            float32 `json:"temp"`
	} `json:"knowledge"`
	AutonomousMode struct {
		Enable            bool   `json:"enable"`
		LLMProvider       string `json:"llm_provider"` // e.g., "openai", "together", "custom"
		LLMKey            string `json:"llm_key"`
		Model             string `json:"model"` // e.g., "gpt-4o-mini", "llama3-70b-chat"
		SystemPrompt      string `json:"system_prompt"`
		MaxLoopIterations int    `json:"max_loop_iterations"`
	} `json:"autonomous_mode"`
	STT struct {
		Service  string `json:"provider"`
		Language string `json:"language"`
	} `json:"STT"`
	Server struct {
		// false for ip, true for escape pod
		EPConfig bool   `json:"epconfig"`
		Port     string `json:"port"`
	} `json:"server"`
	HasReadFromEnv   bool `json:"hasreadfromenv"`
	PastInitialSetup bool `json:"pastinitialsetup"`
}

func WriteConfigToDisk() {
	logger.Println("Configuration changed, writing to disk")
	writeBytes, _ := json.MarshalIndent(APIConfig, "", "  ") // Indent for readability
	os.WriteFile(ApiConfigPath, writeBytes, 0644)
}

func CreateConfigFromEnv() {
	// if no config exists, create it
	if os.Getenv("WEATHERAPI_ENABLED") == "true" {
		APIConfig.Weather.Enable = true
		APIConfig.Weather.Provider = os.Getenv("WEATHERAPI_PROVIDER")
		APIConfig.Weather.Key = os.Getenv("WEATHERAPI_KEY")
		APIConfig.Weather.Unit = os.Getenv("WEATHERAPI_UNIT")
	} else {
		APIConfig.Weather.Enable = false
	}
	if os.Getenv("KNOWLEDGE_ENABLED") == "true" {
		APIConfig.Knowledge.Enable = true
		APIConfig.Knowledge.Provider = os.Getenv("KNOWLEDGE_PROVIDER")
		if os.Getenv("KNOWLEDGE_PROVIDER") == "houndify" {
			APIConfig.Knowledge.ID = os.Getenv("KNOWLEDGE_ID")
		}
		APIConfig.Knowledge.Key = os.Getenv("KNOWLEDGE_KEY")
	} else {
		APIConfig.Knowledge.Enable = false
	}

	// Initialize AutonomousMode with defaults if not set by env (though likely won't be)
	if APIConfig.AutonomousMode.SystemPrompt == "" {
		APIConfig.AutonomousMode.SystemPrompt = "You are Vector, an autonomous robot. Observe your surroundings and decide on the best action. Available commands: CMD_DRIVE_WHEELS(leftSpeed, rightSpeed, durationMs), CMD_TURN_IN_PLACE(angleRad, speedRadPerSec, direction), CMD_SAY_TEXT(text), CMD_MOVE_HEAD(angleRad), CMD_MOVE_LIFT(heightMm), CMD_TAKE_PHOTO_AND_CONTINUE(), CMD_STOP_AUTONOMOUS_MODE()."
	}
	if APIConfig.AutonomousMode.MaxLoopIterations == 0 {
		APIConfig.AutonomousMode.MaxLoopIterations = 100
	}
	APIConfig.AutonomousMode.Enable = false // Default to disabled

	WriteSTT() // Ensure STT settings are also handled
	APIConfig.HasReadFromEnv = true
	writeBytes, _ := json.MarshalIndent(APIConfig, "", "  ")
	os.WriteFile(ApiConfigPath, writeBytes, 0644)
}

func WriteSTT() {
	// was not part of the original code, so this is its own function
	// launched if stt not found in config
	APIConfig.STT.Service = os.Getenv("STT_SERVICE")
	if os.Getenv("STT_SERVICE") == "vosk" || os.Getenv("STT_SERVICE") == "whisper.cpp" {
		APIConfig.STT.Language = os.Getenv("STT_LANGUAGE")
	}
}

func ReadConfig() {
	if _, err := os.Stat(ApiConfigPath); err != nil {
		logger.Println("No API config found, creating from environment variables...")
		CreateConfigFromEnv()
		logger.Println("API config JSON created")
	} else {
		// read config
		configBytes, err := os.ReadFile(ApiConfigPath)
		if err != nil {
			APIConfig.Knowledge.Enable = false
			APIConfig.Weather.Enable = false
			APIConfig.AutonomousMode.Enable = false
			logger.Println("Failed to read API config file")
			logger.Println(err)
			return
		}
		err = json.Unmarshal(configBytes, &APIConfig)
		if err != nil {
			APIConfig.Knowledge.Enable = false
			APIConfig.Weather.Enable = false
			APIConfig.AutonomousMode.Enable = false
			logger.Println("Failed to unmarshal API config JSON")
			logger.Println(err)
			return
		}
		// stt service is the only thing controlled by shell
		// This logic might need adjustment if STT_SERVICE env var is meant to override saved config on each run
		if APIConfig.STT.Service == "" || APIConfig.STT.Service != os.Getenv("STT_SERVICE") && os.Getenv("STT_SERVICE") != "" {
			logger.Println("STT Service in config is blank or different from ENV var, updating from ENV var.")
			WriteSTT()
		}
		if !APIConfig.HasReadFromEnv {
			// This condition might be redundant if CreateConfigFromEnv sets it
			if APIConfig.Server.Port != os.Getenv("DDL_RPC_PORT") && os.Getenv("DDL_RPC_PORT") != "" {
				APIConfig.HasReadFromEnv = true
			}
			// Ensure PastInitialSetup is true if certain critical parts are configured
			if APIConfig.STT.Service != "" && APIConfig.Server.Port != "" {
				APIConfig.PastInitialSetup = true
			}
		}

		if APIConfig.Knowledge.Model == "meta-llama/Llama-2-70b-chat-hf" {
			logger.Println("Setting Together model to Llama3")
			APIConfig.Knowledge.Model = "meta-llama/Llama-3-70b-chat-hf"
		}

		// Initialize AutonomousMode with defaults if loading from an older config file that doesn't have it
		if APIConfig.AutonomousMode.SystemPrompt == "" {
			APIConfig.AutonomousMode.SystemPrompt = "You are Vector, an autonomous robot. Observe your surroundings and decide on the best action. Available commands: CMD_DRIVE_WHEELS(leftSpeed, rightSpeed, durationMs), CMD_TURN_IN_PLACE(angleRad, speedRadPerSec, direction), CMD_SAY_TEXT(text), CMD_MOVE_HEAD(angleRad), CMD_MOVE_LIFT(heightMm), CMD_TAKE_PHOTO_AND_CONTINUE(), CMD_STOP_AUTONOMOUS_MODE()."
		}
		if APIConfig.AutonomousMode.MaxLoopIterations == 0 {
			APIConfig.AutonomousMode.MaxLoopIterations = 100
		}
		// Note: APIConfig.AutonomousMode.Enable will default to false (zero-value for bool) if not in JSON

		writeBytes, _ := json.MarshalIndent(APIConfig, "", "  ") // Indent for readability
		os.WriteFile(ApiConfigPath, writeBytes, 0644)
		logger.Println("API config successfully read")
	}
}
