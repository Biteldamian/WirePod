# Vector Autonomous Mode

This directory contains the core logic for enabling LLM-driven autonomous operation for Vector within the WirePod ecosystem.

## Overview

The Autonomous Mode allows Vector to perceive its environment using its camera, send this visual information (and potentially other sensor data in the future) to a Large Language Model (LLM), receive action commands from the LLM, and execute those commands. This creates a loop where Vector can make decisions and interact with its surroundings based on LLM guidance.

## How It Works

1.  **Activation**:
    *   Autonomous mode is triggered by a specific voice intent (e.g., "Hey Vector, start autonomous mode").
    *   This intent is defined in `chipper/intent-data/en-US.json` (and potentially other language files) as `intent_start_autonomous_mode`.
    *   The `chipper/pkg/wirepod/preqs/intent_graph.go` (or `intent.go`) handles this intent and initiates the `AutonomousLLMLoop`.

2.  **Configuration (`chipper/pkg/vars/config.go`)**:
    *   Settings for autonomous mode are managed in `apiConfig.json` and accessed via `vars.APIConfig.AutonomousMode`.
    *   Key settings include:
        *   `Enable`: Toggles the entire feature.
        *   `LLMProvider`, `LLMKey`, `Model`: For specifying the LLM service (OpenAI, Together, Custom) and credentials.
        *   `SystemPrompt`: A crucial prompt that instructs the LLM on its role, available commands, and expected output format.
        *   `MaxLoopIterations`: A safety limit for how many cycles the autonomous loop will run.
    *   These settings can be configured via the WirePod web UI.

3.  **Core Loop (`autonomous.go:AutonomousLLMLoop`)**:
    *   **Behavior Control**: The loop first acquires behavior control over Vector to enable direct SDK commands. This is released upon exiting the loop.
    *   **Interruption Handling**: The loop can be interrupted by voice (wake word) or touch, providing a way to stop autonomous operation manually.
    *   **Main Cycle**:
        1.  **Image Capture (`getCameraImage`)**: Vector captures an image from its camera.
        2.  **LLM Prompting**: The image (if captured and model supports it) is base64 encoded and sent to the configured LLM along with the system prompt and a query for the next action.
        3.  **Command Reception**: The LLM responds with a command string (e.g., `CMD_DRIVE_WHEELS(50, 50, 1000)`).
        4.  **Command Parsing (`commands.go:ParseAutonomousCommand`)**: The LLM's response is parsed to extract the command name and its parameters.
        5.  **Command Execution (`ExecuteSDKCommand`)**: The parsed command is executed using the `vector-go-sdk`. This involves calling specific robot functions like `DriveWheels`, `TurnInPlace`, `SayText`, etc.
        6.  **Loop Continuation/Termination**:
            *   The loop continues for `MaxLoopIterations`.
            *   The command `CMD_TAKE_PHOTO_AND_CONTINUE` signals the loop to get a new image and re-prompt the LLM.
            *   The command `CMD_STOP_AUTONOMOUS_MODE` or an interruption event will terminate the loop.

4.  **Available Commands (`commands.go`)**:
    *   A predefined set of `AutonomousCommand` structs defines the actions Vector can take. Each command specifies:
        *   `Command`: The string identifier (e.g., `CMD_DRIVE_WHEELS`).
        *   `Description`: What the command does.
        *   `ExpectedParams`: The names of parameters the command takes.
        *   `ParamTypes`: The data types of these parameters (e.g., "int", "float", "string").
        *   `SDKFunction`: A direct Go function that executes the command using the Vector SDK.
    *   Current commands include driving, turning, speaking, moving head/lift, and loop control.
    *   The system prompt sent to the LLM includes a list of these available commands and their expected parameter formats.

## File Structure

*   **`autonomous.go`**: Contains the main `AutonomousLLMLoop`, `getCameraImage`, `ExecuteSDKCommand`, and helper functions for behavior control and interruption.
*   **`commands.go`**: Defines the `AutonomousCommand` struct, the list of `ValidAutonomousCommands`, and the `ParseAutonomousCommand` function for interpreting LLM responses.
*   **`README.md`**: This file.

## Future Enhancements (Potential)

*   Integration of more sensor data (e.g., audio, distance sensor) into the LLM prompt.
*   More sophisticated state management within the loop.
*   Memory for the LLM to recall previous actions or observations.
*   Ability for the LLM to request specific information (e.g., "Is there an obstacle?").
*   User-defined autonomous "tasks" or "scripts" driven by the LLM.
*   Expansion via a plugin system, allowing new autonomous capabilities or command handlers to be added modularly.

## Safety

Autonomous operation requires caution.
*   Always supervise Vector when this mode is active, especially during initial testing.
*   Test in a clear, safe environment where Vector cannot fall or cause damage.
*   Start with low `MaxLoopIterations` and simple, short-duration commands.
*   Be aware of the physical limits of the robot.
*   Use the interruption features (touch, wake word) if Vector behaves unexpectedly.
The system prompt should be carefully crafted to prevent the LLM from issuing dangerous or nonsensical sequences of commands.
