package processreqs

import (
	"strings"

	"github.com/kercre123/wire-pod/chipper/pkg/logger"
	"github.com/kercre123/wire-pod/chipper/pkg/vars"
	"github.com/kercre123/wire-pod/chipper/pkg/vtt"
	sr "github.com/kercre123/wire-pod/chipper/pkg/wirepod/speechrequest"
	ttr "github.com/kercre123/wire-pod/chipper/pkg/wirepod/ttr"
	"github.com/kercre123/wire-pod/chipper/pkg/wirepod/autonomous" // Import the new package
)

func (s *Server) ProcessIntentGraph(req *vtt.IntentGraphRequest) (*vtt.IntentGraphResponse, error) {
	var successMatched bool
	speechReq := sr.ReqToSpeechRequest(req)
	var transcribedText string

	if !isSti { // Assuming isSti is defined elsewhere, for STT vs STI choice
		var err error
		transcribedText, err = sttHandler(speechReq) // Assuming sttHandler is defined
		if err != nil {
			ttr.IntentPass(req, "intent_system_noaudio", "voice processing error: "+err.Error(), map[string]string{"error": err.Error()}, true)
			return nil, nil
		}
		if strings.TrimSpace(transcribedText) == "" {
			ttr.IntentPass(req, "intent_system_noaudio", "", map[string]string{}, false)
			return nil, nil
		}

		// Check for autonomous mode trigger phrases BEFORE general intent processing
		// This is a simplified check. Ideally, intent matching should return the specific intent.
		lowerTranscribedText := strings.ToLower(transcribedText)
		autonomousModePhrases := []string{"start autonomous mode", "enter autonomous mode", "go autonomous", "begin autonomous mode", "initiate autonomous sequence"}
		triggeredAutonomous := false
		for _, phrase := range autonomousModePhrases {
			if strings.Contains(lowerTranscribedText, phrase) {
				triggeredAutonomous = true
				break
			}
		}

		if triggeredAutonomous && vars.APIConfig.AutonomousMode.Enable {
			logger.Println("Autonomous mode triggered by phrase: ", transcribedText)
			robot, err := vars.GetRobot(req.Device) // Get the robot instance
			if err != nil {
				logger.Println("Error getting robot instance for autonomous mode:", err)
				ttr.IntentPass(req, "intent_system_error", "could not get robot instance", map[string]string{"error": err.Error()}, true)
				return nil, nil
			}

			// Acknowledge and start loop in a goroutine
			go func() {
				// Optional: Send an immediate acknowledgment to the user if possible through current stream
				// This is tricky as the main intent response path might also try to send something.
				// For now, AutonomousLLMLoop will handle initial voice feedback.
				autonomous.AutonomousLLMLoop(robot, req.Session, req.Device)
			}()

			// Send a minimal, non-interfering response to satisfy the original intent request,
			// or find a way to indicate that this stream is now handled differently.
			// For now, let's send a simple acknowledgment intent.
			ttr.IntentPass(req, "intent_greeting_hello", "Starting autonomous mode.", map[string]string{}, false)
			logger.Println("Bot " + speechReq.Device + " autonomous mode initiated.")
			return nil, nil // Autonomous loop handles further interaction
		}

		// If not autonomous mode, proceed with normal intent matching
		successMatched = ttr.ProcessTextAll(req, transcribedText, vars.IntentList, speechReq.IsOpus)

	} else { // STI path
		intent, slots, err := stiHandler(speechReq) // Assuming stiHandler is defined
		if err != nil {
			if err.Error() == "inference not understood" {
				logger.Println("Bot " + speechReq.Device + " No intent was matched by STI")
				ttr.IntentPass(req, "intent_system_unmatched", "voice processing error", map[string]string{"error": err.Error()}, true)
				return nil, nil
			}
			logger.Println(err)
			ttr.IntentPass(req, "intent_system_noaudio", "voice processing error", map[string]string{"error": err.Error()}, true)
			return nil, nil
		}
		// Note: Autonomous mode trigger via STI would need specific intent handling in STI model
		ttr.ParamCheckerSlotsEnUS(req, intent, slots, speechReq.IsOpus, speechReq.Device)
		logger.Println("Bot " + speechReq.Device + " STI request served.")
		return nil, nil
	}

	if !successMatched { // Only if not STT successMatched and not autonomous mode
		if vars.APIConfig.Knowledge.IntentGraph && vars.APIConfig.Knowledge.Enable {
			logger.Println("Making LLM (KnowledgeGraph) request for device " + req.Device + "...")
			_, err := ttr.StreamingKGSim(req, req.Device, transcribedText, false) // false indicates it's not a KG-specific loop start
			if err != nil {
				logger.Println("LLM error: " + err.Error())
				logger.LogUI("LLM error: " + err.Error())
				// Fallback to standard unmatched intent
				ttr.IntentPass(req, "intent_system_unmatched", transcribedText, map[string]string{"error": err.Error()}, true)
				// ttr.KGSim(req.Device, "There was an error getting a response from the L L M. Check the logs in the web interface.")
			}
			logger.Println("Bot " + speechReq.Device + " KG request served.")
			return nil, nil
		}
		logger.Println("No intent was matched (after STT).")
		ttr.IntentPass(req, "intent_system_unmatched", transcribedText, map[string]string{"transcribed_text": transcribedText}, true)
		return nil, nil
	}

	logger.Println("Bot " + speechReq.Device + " STT intent request served.")
	return nil, nil
}

// Note: Ensure that isSti, sttHandler, and stiHandler are properly defined and initialized
// in your actual processreqs package.
// Also, vars.GetRobot() needs to be accessible and working.
// The intent "intent_greeting_hello" is used as a placeholder for acknowledgment;
// a more specific "intent_autonomous_mode_started" could be created if desired.
// This simplified check for autonomous mode phrases might lead to false positives if these phrases
// appear in other contexts. A refactor of ttr.ProcessTextAll to return the matched intent name
// would be a more robust solution for triggering specific Go functions based on matched intents.
