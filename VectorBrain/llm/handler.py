import os
from openai import OpenAI, APITimeoutError, APIConnectionError, APIStatusError
from typing import List, Dict, Optional, Any

# Assuming config.py is in the parent directory (VectorBrain root)
import sys
import os
# Add parent directory to sys.path to allow direct import of config
sys.path.append(os.path.join(os.path.dirname(__file__), '..'))
try:
    import config
except ImportError:
    print("CRITICAL: config.py not found. Ensure it exists in the VectorBrain root directory.")
    # Define fallbacks if config import fails, so the module can still be imported for basic structure check
    class config: # type: ignore
        LLM_API_BASE_URL = "http://localhost:8000/v1"
        LLM_API_KEY = "not-needed"
        LLM_MODEL_NAME = "local-model"


class LLMHandler:
    def __init__(self,
                 api_base_url: Optional[str] = None,
                 api_key: Optional[str] = None,
                 model_name: Optional[str] = None):
        """
        Initializes the LLM Handler.
        Uses settings from config.py if parameters are not provided.
        """
        _api_base_url = api_base_url if api_base_url is not None else config.LLM_API_BASE_URL
        _api_key = api_key if api_key is not None else config.LLM_API_KEY
        self.model_name = model_name if model_name is not None else config.LLM_MODEL_NAME

        self.client = OpenAI(
            base_url=_api_base_url,
            api_key=_api_key,
        )
        print(f"LLM Handler initialized for model: {self.model_name} at {_api_base_url}")

    def get_llm_command(
        self,
        system_prompt: str,
        conversation_history: Optional[List[Dict[str, Any]]] = None, # List of messages like {"role": "user/assistant", "content": ...}
        image_base64: Optional[str] = None,
        temperature: float = 0.6,
        max_tokens: int = 150
    ) -> Optional[str]:
        """
        Gets a command from the LLM based on the provided context and image.

        Args:
            system_prompt: The system prompt defining the LLM's role and available commands.
            conversation_history: A list of previous user/assistant messages to provide context.
                                  Each message is a dict: e.g., {"role": "user", "content": "What do you see?"}
                                  or for images: {"role": "user", "content": [{"type": "text", "text": "..."}, {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,..."}}]}
            image_base64: Optional base64 encoded string of the camera image.
            temperature: The sampling temperature for the LLM.
            max_tokens: The maximum number of tokens for the LLM to generate.

        Returns:
            The raw command string from the LLM, or None if an error occurs.
        """
        messages = [{"role": "system", "content": system_prompt}]

        if conversation_history:
            messages.extend(conversation_history)

        # Prepare the current user message, potentially with an image
        current_user_content: List[Dict[str, Any]] = []
        if image_base64:
            # Assuming JPEG format for the base64 image data
            image_url_data = f"data:image/jpeg;base64,{image_base64}"
            current_user_content.append({"type": "text", "text": "Analyze the current camera view and decide the next command."})
            current_user_content.append({
                "type": "image_url",
                "image_url": {"url": image_url_data, "detail": "auto"} # detail can be low, high, auto
            })
            print("LLM Handler: Preparing prompt with image.")
        else:
            current_user_content.append({"type": "text", "text": "No new camera image available or image not supported by model. Decide the next command based on context."})
            print("LLM Handler: Preparing prompt without image.")

        messages.append({"role": "user", "content": current_user_content})

        # For debugging the prompt structure:
        # print("\n--- LLM Request Messages ---")
        # for msg in messages:
        #     if msg["role"] == "user" and isinstance(msg["content"], list):
        #         print(f"Role: {msg['role']}")
        #         for part in msg["content"]:
        #             if part["type"] == "text":
        #                 print(f"  Text: {part['text']}")
        #             elif part["type"] == "image_url":
        #                 print(f"  Image URL: {part['image_url']['url'][:50]}...") # Print start of b64
        #     else:
        #         print(f"Role: {msg['role']}, Content: {str(msg['content'])[:200]}") # Print start of content
        # print("---------------------------\n")


        try:
            print(f"LLM Handler: Sending request to model {self.model_name}...")
            completion = self.client.chat.completions.create(
                model=self.model_name,
                messages=messages,
                temperature=temperature,
                max_tokens=max_tokens,
                # stream=False, # For simplicity in this example, not streaming
            )

            if completion.choices and len(completion.choices) > 0:
                response_content = completion.choices[0].message.content
                print(f"LLM Handler: Received response: {response_content}")
                return response_content.strip() if response_content else None
            else:
                print("LLM Handler: No choices returned in completion.")
                return None

        except APITimeoutError:
            print("LLM Handler: OpenAI API request timed out.")
            return None
        except APIConnectionError as e:
            print(f"LLM Handler: OpenAI API connection error: {e}")
            return None
        except APIStatusError as e:
            print(f"LLM Handler: OpenAI API returned an API Error: {e.status_code} - {e.message}")
            return None
        except Exception as e:
            print(f"LLM Handler: An unexpected error occurred: {e}")
            return None

# Example Usage (for testing this module directly):
if __name__ == '__main__':
    # Configure these based on your local LLM setup
    # For Llamafile or other OpenAI-compatible servers:
    API_BASE = os.getenv("LLM_API_BASE_URL", "http://localhost:8081/v1") # Llamafile often runs on 8081
    API_KEY = os.getenv("LLM_API_KEY", "sk-no-key-required")
    MODEL_NAME = os.getenv("LLM_MODEL_NAME", "llamafile") # The actual model name might be ignored by local server

    print(f"Attempting to connect to LLM at: {API_BASE} with model: {MODEL_NAME}")

    handler = LLMHandler(api_base_url=API_BASE, api_key=API_KEY, model_name=MODEL_NAME)

    # Simple test without image
    test_system_prompt = "You are a helpful assistant. Respond with a short greeting."
    print("\n--- Test 1: Simple text prompt ---")
    response = handler.get_llm_command(system_prompt=test_system_prompt)
    if response:
        print(f"LLM Response (Text Only): {response}")
    else:
        print("Failed to get response from LLM (Text Only).")

    # Test with a placeholder for image (if your model supports vision)
    # To test with a real image, you'd load an image, base64 encode it.
    # For example:
    # try:
    #     with open("path_to_your_test_image.jpg", "rb") as image_file:
    #         import base64
    #         test_image_b64 = base64.b64encode(image_file.read()).decode('utf-8')
    #     print("\n--- Test 2: Text and Image prompt ---")
    #     vision_system_prompt = "You are an image analysis assistant. Describe the image in one sentence."
    #     response_vision = handler.get_llm_command(system_prompt=vision_system_prompt, image_base64=test_image_b64)
    #     if response_vision:
    #         print(f"LLM Response (Vision): {response_vision}")
    #     else:
    #         print("Failed to get response from LLM (Vision). Ensure your model supports vision and the image path is correct.")
    # except FileNotFoundError:
    #     print("\nSkipping vision test: Test image not found. Create a 'path_to_your_test_image.jpg' to run.")
    # except ImportError:
    #     print("\nSkipping vision test: `base64` module not found (should be standard).")

    print("\n--- Test 3: Command generation prompt (no image) ---")
    command_system_prompt = """
You are Vector, a robot. Respond with ONLY ONE command.
Available commands:
CMD_SAY_TEXT(textToSay: string)
CMD_DRIVE_WHEELS(leftWheelSpeed: float, rightWheelSpeed: float, durationMs: int)
CMD_STOP_AUTONOMOUS_MODE()
If you want to speak, use CMD_SAY_TEXT.
If you want to stop, use CMD_STOP_AUTONOMOUS_MODE.
What is your next command?
"""
    # Simulating conversation history
    conv_history = [
        {"role": "user", "content": [{"type": "text", "text": "Hello Vector!"}]},
        {"role": "assistant", "content": "CMD_SAY_TEXT(Hello human!)"}
    ]
    # New user part of the prompt (without image for this test)
    # The get_llm_command will wrap this in the final user message structure.
    # For this test, we are essentially providing the "user" part of the prompt as empty,
    # relying on the system prompt and history. Or, we could add a specific query.
    # Let's add a simple query to make it more realistic for this test.
    conv_history.append({"role": "user", "content": [{"type": "text", "text": "What should we do next?"}]})


    response_command = handler.get_llm_command(
        system_prompt=command_system_prompt,
        conversation_history=None, # Pass the conv_history directly to the messages list construction
                                    # The function signature expects it as a separate param.
                                    # Let's modify call or function. For now, simpler:
                                    # The function `get_llm_command` now appends a new user message based on image or lack thereof.
                                    # So, conversation_history should be prior turns.
    )
    # To test with history:
    # response_command = handler.get_llm_command(system_prompt=command_system_prompt, conversation_history=conv_history)


    if response_command:
        print(f"LLM Command Response: {response_command}")
    else:
        print("Failed to get command response from LLM.")

```
