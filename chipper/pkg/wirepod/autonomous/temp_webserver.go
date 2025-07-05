package webserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kercre123/wire-pod/chipper/pkg/logger"
	"github.com/kercre123/wire-pod/chipper/pkg/scripting" // Assuming this is the correct path
	"github.com/kercre123/wire-pod/chipper/pkg/vars"
	"github.com/kercre123/wire-pod/chipper/pkg/wirepod/localization" // Assuming this is the correct path
	processreqs "github.com/kercre123/wire-pod/chipper/pkg/wirepod/preqs" // Assuming this is the correct path
	botsetup "github.com/kercre123/wire-pod/chipper/pkg/wirepod/setup" // Assuming this is the correct path
)

var SttInitFunc func() error // This might be better placed in a shared utility or context package

// DisableCachingAndSniffing is a middleware that sets headers to prevent caching and content sniffing.
func DisableCachingAndSniffing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

// certHandler serves certificate files.
func certHandler(w http.ResponseWriter, r *http.Request) {
	requestedFile := strings.TrimPrefix(r.URL.Path, "/session-certs/")
	if requestedFile == "" {
		http.Error(w, "no file requested", http.StatusBadRequest)
		return
	}
	// Sanitize file path to prevent directory traversal
	requestedFile = path.Clean(requestedFile)
	if strings.Contains(requestedFile, "..") {
		http.Error(w, "invalid file path", http.StatusBadRequest)
		return
	}
	filePath := filepath.Join(vars.SessionCertPath, requestedFile)
	http.ServeFile(w, r, filePath)
}

// anyEmpty checks if any of the provided strings are empty.
func anyEmpty(strs ...string) bool {
	for _, s := range strs {
		if s == "" {
			return true
		}
	}
	return false
}

// isValidLanguage checks if a language is valid for a given STT service.
func isValidLanguage(lang string, validModels []string) bool {
	for _, model := range validModels {
		if lang == model {
			return true
		}
	}
	return false
}

// isDownloadedLanguage checks if a language model is downloaded.
func isDownloadedLanguage(lang string, downloadedModels []string) bool {
	for _, model := range downloadedModels {
		if lang == model {
			return true
		}
	}
	return false
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Content-Type", "application/json") // Default to JSON

	switch strings.TrimPrefix(r.URL.Path, "/api/") {
	case "add_custom_intent":
		handleAddCustomIntent(w, r)
	case "edit_custom_intent":
		handleEditCustomIntent(w, r)
	case "get_custom_intents_json":
		handleGetCustomIntentsJSON(w)
	case "remove_custom_intent":
		handleRemoveCustomIntent(w, r)
	case "set_weather_api":
		handleSetWeatherAPI(w, r)
	case "get_weather_api":
		handleGetWeatherAPI(w)
	case "set_kg_api":
		handleSetKGAPI(w, r)
	case "get_kg_api":
		handleGetKGAPI(w)
	case "set_autonomous_mode_settings":
		handleSetAutonomousModeSettings(w, r)
	case "get_autonomous_mode_settings":
		handleGetAutonomousModeSettings(w)
	case "set_stt_info":
		handleSetSTTInfo(w, r)
	case "get_download_status":
		handleGetDownloadStatus(w)
	case "get_stt_info":
		handleGetSTTInfo(w)
	case "get_config":
		handleGetConfig(w)
	case "get_logs":
		handleGetLogs(w)
	case "get_debug_logs":
		handleGetDebugLogs(w)
	case "is_running":
		handleIsRunning(w)
	case "delete_chats":
		handleDeleteChats(w)
	case "get_ota_update_status": // Assuming this was the intended name from a previous context
		handleGetOTAUpdateStatus(w, r) // Changed from handleGetOTA to match a common pattern
	case "get_version_info":
		handleGetVersionInfo(w)
	case "generate_certs":
		handleGenerateCerts(w)
	case "is_api_v3": // Example, keep if needed
		fmt.Fprintf(w, `{"status": "it is!"}`)
	default:
		http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
	}
}

func handleAddCustomIntent(w http.ResponseWriter, r *http.Request) {
	var intent vars.CustomIntent
	if err := json.NewDecoder(r.Body).Decode(&intent); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}
	if anyEmpty(intent.Name, intent.Description, intent.Intent) || len(intent.Utterances) == 0 {
		http.Error(w, `{"error": "missing required field (name, description, utterances, and intent are required)"}`, http.StatusBadRequest)
		return
	}
	intent.LuaScript = strings.TrimSpace(intent.LuaScript)
	if intent.LuaScript != "" {
		if err := scripting.ValidateLuaScript(intent.LuaScript); err != nil {
			http.Error(w, `{"error": "lua validation error: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
	}
	vars.CustomIntentsExist = true
	vars.CustomIntents = append(vars.CustomIntents, intent)
	saveCustomIntents()
	fmt.Fprint(w, `{"status": "Intent added successfully."}`)
}

func handleEditCustomIntent(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Number int `json:"number"`
		vars.CustomIntent
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}
	if request.Number < 1 || request.Number > len(vars.CustomIntents) {
		http.Error(w, `{"error": "invalid intent number"}`, http.StatusBadRequest)
		return
	}
	intent := &vars.CustomIntents[request.Number-1]
	// Only update fields if they are provided in the request
	if request.Name != "" {
		intent.Name = request.Name
	}
	if request.Description != "" {
		intent.Description = request.Description
	}
	if len(request.Utterances) != 0 {
		intent.Utterances = request.Utterances
	}
	if request.Intent != "" {
		intent.Intent = request.Intent
	}
	// For nested structs, check if the parent is non-nil or if specific fields are set
	if request.Params.ParamName != "" || request.Params.ParamValue != "" {
		if request.Params.ParamName != "" {
			intent.Params.ParamName = request.Params.ParamName
		}
		if request.Params.ParamValue != "" {
			intent.Params.ParamValue = request.Params.ParamValue
		}
	}
	if request.Exec != "" {
		intent.Exec = request.Exec
	}
	if request.LuaScript != "" {
		intent.LuaScript = request.LuaScript
		if err := scripting.ValidateLuaScript(intent.LuaScript); err != nil {
			http.Error(w, `{"error": "lua validation error: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
	} else { // If LuaScript is explicitly empty in request, clear it
		intent.LuaScript = ""
	}

	if len(request.ExecArgs) != 0 {
		intent.ExecArgs = request.ExecArgs
	} else if request.ExecArgs == nil && r.ContentLength > 0 { // Check if it was present in JSON as null or empty array
		// Decide if you want to clear ExecArgs if an empty array is passed, or only if explicitly nil
		// For now, let's assume an empty array in request means clear it.
		intent.ExecArgs = []string{}
	}

	intent.IsSystemIntent = request.IsSystemIntent // Update boolean field

	saveCustomIntents()
	fmt.Fprint(w, `{"status": "Intent edited successfully."}`)
}

func handleGetCustomIntentsJSON(w http.ResponseWriter) {
	if !vars.CustomIntentsExist || len(vars.CustomIntents) == 0 {
		// Return an empty JSON array if no intents exist, instead of an error
		fmt.Fprint(w, "[]")
		return
	}
	customIntentJSONFile, err := os.ReadFile(vars.CustomIntentsPath)
	if err != nil {
		http.Error(w, `{"error": "could not read custom intents file"}`, http.StatusInternalServerError)
		logger.Println(err)
		return
	}
	w.Write(customIntentJSONFile)
}

func handleRemoveCustomIntent(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Number int `json:"number"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}
	if request.Number < 1 || request.Number > len(vars.CustomIntents) {
		http.Error(w, `{"error": "invalid intent number"}`, http.StatusBadRequest)
		return
	}
	vars.CustomIntents = append(vars.CustomIntents[:request.Number-1], vars.CustomIntents[request.Number:]...)
	if len(vars.CustomIntents) == 0 {
		vars.CustomIntentsExist = false
	}
	saveCustomIntents()
	fmt.Fprint(w, `{"status": "Intent removed successfully."}`)
}

func handleSetWeatherAPI(w http.ResponseWriter, r *http.Request) {
	var config struct {
		Enable   bool   `json:"enable"` // Added Enable field
		Provider string `json:"provider"`
		Key      string `json:"key"`
		Unit     string `json:"unit"` // Added unit
	}
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}
	vars.APIConfig.Weather.Enable = config.Enable
	if config.Enable {
		if config.Provider == "" {
			http.Error(w, `{"error": "provider is required if weather API is enabled"}`, http.StatusBadRequest)
			return
		}
		vars.APIConfig.Weather.Key = strings.TrimSpace(config.Key)
		vars.APIConfig.Weather.Provider = config.Provider
		vars.APIConfig.Weather.Unit = config.Unit // Save the unit
	} else {
		// Clear other fields if disabled
		vars.APIConfig.Weather.Key = ""
		vars.APIConfig.Weather.Provider = ""
		vars.APIConfig.Weather.Unit = ""
	}
	vars.WriteConfigToDisk()
	fmt.Fprint(w, `{"status": "Changes successfully applied."}`)
}

func handleGetWeatherAPI(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(vars.APIConfig.Weather)
}

func handleSetKGAPI(w http.ResponseWriter, r *http.Request) {
	// Decode into a temporary struct to avoid partially updating the global config on error
	var kgConfig vars.ApiConfigKnowledge // Assuming ApiConfigKnowledge is the type of APIConfig.Knowledge
	if err := json.NewDecoder(r.Body).Decode(&kgConfig); err != nil {
		http.Error(w, `{"error": "invalid request body: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	vars.APIConfig.Knowledge = kgConfig // Update the global config
	vars.WriteConfigToDisk()
	fmt.Fprint(w, `{"status": "Changes successfully applied."}`)
}

func handleGetKGAPI(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(vars.APIConfig.Knowledge)
}

func handleSetAutonomousModeSettings(w http.ResponseWriter, r *http.Request) {
	var autoConfig vars.ApiConfigAutonomousMode // Assuming this is the type of APIConfig.AutonomousMode
	if err := json.NewDecoder(r.Body).Decode(&autoConfig); err != nil {
		http.Error(w, `{"error": "invalid request body: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	vars.APIConfig.AutonomousMode = autoConfig
	vars.WriteConfigToDisk()
	fmt.Fprint(w, `{"status": "Autonomous mode settings successfully applied."}`)
}

func handleGetAutonomousModeSettings(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(vars.APIConfig.AutonomousMode)
}

func handleSetSTTInfo(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Language string `json:"language"`
		Service  string `json:"provider"` // Allow setting STT service via API
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Update STT Service if provided
	if request.Service != "" {
		// Potentially validate request.Service against a list of known/supported services
		vars.APIConfig.STT.Service = request.Service
	}

	currentService := vars.APIConfig.STT.Service
	if currentService == "vosk" {
		if !isValidLanguage(request.Language, localization.ValidVoskModels) {
			http.Error(w, `{"error": "language not valid for Vosk"}`, http.StatusBadRequest)
			return
		}
		if !isDownloadedLanguage(request.Language, vars.DownloadedVoskModels) && request.Language != "" {
			go localization.DownloadVoskModel(request.Language) // Ensure this doesn't block
			fmt.Fprint(w, `{"status": "downloading language model..."}`)
			return
		}
	} else if currentService == "whisper.cpp" {
		// Assuming ValidVoskModels also contains whisper.cpp compatible language codes, or use a different list
		if !isValidLanguage(request.Language, localization.ValidVoskModels) && request.Language != "" {
			http.Error(w, `{"error": "language not valid for whisper.cpp"}`, http.StatusBadRequest)
			return
		}
	} else if currentService != "" && request.Language != "" {
		// For other services, or if language is set for a non-model based STT
		// just set the language.
	} else if request.Language == "" && currentService == "" {
		// Both are empty, nothing to do or error
		http.Error(w, `{"error": "STT service and language are empty"}`, http.StatusBadRequest)
		return
	}


	if request.Language != "" {
		vars.APIConfig.STT.Language = request.Language
	}
	vars.APIConfig.PastInitialSetup = true // Setting language implies setup is past initial
	vars.WriteConfigToDisk()

	// Reload STT only if SttInitFunc is set (meaning it's a model-based STT that needs reloading)
	if SttInitFunc != nil && (currentService == "vosk" || currentService == "whisper.cpp") {
		if err := SttInitFunc(); err != nil {
			logger.Println("Error reloading STT models: " + err.Error())
			http.Error(w, `{"error": "Failed to reload STT models: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		// processreqs.ReloadVosk() needs to be callable or its logic moved
		logger.Println("Reloaded voice processor successfully")
	}
	fmt.Fprint(w, `{"status": "STT settings applied successfully."}`)
}

func handleGetDownloadStatus(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json") // Changed to JSON
	status := localization.DownloadStatus
	fmt.Fprintf(w, `{"status": "%s"}`, status)
	if status == "success" || strings.Contains(status, "error") {
		localization.DownloadStatus = "not downloading"
	}
}

func handleGetSTTInfo(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(vars.APIConfig.STT)
}

func handleGetConfig(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(vars.APIConfig)
}

func handleGetLogs(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(logger.LogList))
}

func handleGetDebugLogs(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(logger.LogTrayList))
}

func handleIsRunning(w http.ResponseWriter) {
	fmt.Fprintf(w, `{"status": "running"}`)
}

func handleDeleteChats(w http.ResponseWriter) {
	vars.RememberedChats = nil // Clears the chats
	// Optionally, save this empty state to disk if chats are persisted
	logger.Println("All conversation history deleted.")
	fmt.Fprint(w, `{"status": "All conversation history has been deleted."}`)
}

func handleGetOTAUpdateStatus(w http.ResponseWriter, r *http.Request) {
	// Placeholder for OTA update status logic
	// You'll need to implement how OTA updates are checked and their status reported
	// For now, returning a placeholder:
	otaStatus := botsetup.GetOTAUpdateStatus()
	json.NewEncoder(w).Encode(otaStatus)
}

func handleGetVersionInfo(w http.ResponseWriter) {
	// Assuming vars.CommitSHA holds the commit SHA and you have a way to get a version string
	// For example, read from a version file or have it set at build time
	version := "unknown"
	versionData, err := os.ReadFile(vars.VersionFile)
	if err == nil {
		version = strings.TrimSpace(string(versionData))
	}
	info := struct {
		Version   string `json:"version"`
		CommitSHA string `json:"commit_sha"`
	}{
		Version:   version,
		CommitSHA: vars.CommitSHA,
	}
	json.NewEncoder(w).Encode(info)
}

func handleGenerateCerts(w http.ResponseWriter) {
	err := botsetup.CreateCertCombo()
	if err != nil {
		http.Error(w, `{"error": "failed to generate certificates: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, `{"status": "Certificates generated successfully."}`)
}

func saveCustomIntents() {
	customIntentJSONFile, _ := json.MarshalIndent(vars.CustomIntents, "", "  ") // Indent for readability
	os.WriteFile(vars.CustomIntentsPath, customIntentJSONFile, 0644)
}

func StartWebServer() {
	// These registrations should ideally be done once.
	// If StartWebServer can be called multiple times, ensure handlers are not registered repeatedly
	// or use a router that handles this better.
	// For http.DefaultServeMux, repeated calls to HandleFunc will replace previous handlers for the same pattern.

	// Assuming these are safe to call if already registered, or this function is called once.
	botsetup.RegisterSSHAPI()
	botsetup.RegisterBLEAPI()
	scripting.RegisterScriptingAPI() // Ensure this is correctly placed

	http.HandleFunc("/api/", apiHandler)
	http.HandleFunc("/session-certs/", certHandler)

	var webRootPath string
	if runtime.GOOS == "darwin" && vars.Packaged {
		appPath, _ := os.Executable()
		// Frameworks are typically inside Contents/Frameworks relative to the executable in a .app bundle
		webRootPath = filepath.Join(filepath.Dir(appPath), "..", "Frameworks", "chipper", "webroot")
	} else if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		webRootPath = filepath.Join(vars.AndroidPath, "static", "webroot")
	} else {
		webRootPath = "./webroot"
	}

	// Check if webroot exists, provide a clear log message if not.
	if _, err := os.Stat(webRootPath); os.IsNotExist(err) {
		logger.Println("Error: Webroot directory not found at", webRootPath)
		// Depending on severity, you might os.Exit(1) or try a fallback
	} else {
		logger.Println("Serving web content from:", webRootPath)
	}

	webRoot := http.FileServer(http.Dir(webRootPath))
	http.Handle("/", DisableCachingAndSniffing(webRoot))

	logger.Println("Starting webserver at port " + vars.WebPort + " (http://localhost:" + vars.WebPort + ")")
	if err := http.ListenAndServe(":"+vars.WebPort, nil); err != nil {
		logger.Println("Error binding to " + vars.WebPort + ": " + err.Error())
		if vars.Packaged {
			logger.ErrMsg("FATAL: Wire-pod was unable to bind to port " + vars.WebPort + ". Another process is likely using it. Exiting.")
		}
		// os.Exit(1) // This will terminate the application. Consider if this is the desired behavior.
	}
}

// Ensure vars.ApiConfigKnowledge and vars.ApiConfigAutonomousMode types are defined in the vars package.
// For example, in chipper/pkg/vars/config.go:
// type ApiConfigKnowledge struct { ... fields ... }
// type ApiConfigAutonomousMode struct { ... fields ... }
// And then in apiConfig struct:
// Knowledge       ApiConfigKnowledge `json:"knowledge"`
// AutonomousMode  ApiConfigAutonomousMode `json:"autonomous_mode"`
//
// Placeholder for GetOTAUpdateStatus, replace with actual implementation
// package botsetup
// func GetOTAUpdateStatus() map[string]string {
//    return map[string]string{"status": "not implemented"}
// }
//
// Placeholder for RegisterScriptingAPI
// package scripting
// func RegisterScriptingAPI() {
//    http.HandleFunc("/api-lua/", func(w http.ResponseWriter, r *http.Request) { /* ... */ })
// }
