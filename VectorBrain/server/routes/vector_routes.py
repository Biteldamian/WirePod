from flask import Blueprint, request, jsonify

# Import the Vector HTTP client module
# Assuming vector_http_client.py is in server/modules/
try:
    from ..modules import vector_http_client
    client_imported = True
except ImportError:
    print("ERROR in vector_routes.py: Could not import vector_http_client. Ensure it exists in modules folder.")
    # Define a dummy client if import fails, so server can still start but routes will show error
    class DummyVectorClient:
        def __getattr__(self, name):
            def method(*args, **kwargs):
                print(f"DummyVectorClient: {name} called, but real client failed to import.")
                return False, {"error": "vector_http_client module not imported correctly."}
            return method
    vector_http_client = DummyVectorClient()
    client_imported = False


bp = Blueprint('vector_api', __name__, url_prefix='/vector') # Renamed blueprint for clarity

def _handle_vector_action(action_func, esn, required_params=None, data_source=None):
    """Helper to reduce boilerplate in route handlers."""
    if not client_imported:
        return jsonify({"error": "Vector HTTP client module not loaded on server."}), 503

    if data_source is None:
        data_source = request.get_json()

    if not data_source and required_params: # If data_source is None (GET) but params were expected via JSON body
        return jsonify({"error": "Request body is missing or not JSON"}), 400

    params_to_pass = {}
    if required_params:
        if not data_source: # Should have been caught above if required_params is not None
             return jsonify({"error": "Request body is missing or not JSON"}), 400
        missing_params = [p for p in required_params if p not in data_source]
        if missing_params:
            return jsonify({"error": f"Missing required parameters: {', '.join(missing_params)}"}), 400
        for p in required_params:
            params_to_pass[p] = data_source[p]

    # Call the actual client function
    # success, response_data = action_func(esn, **data_source if data_source else {}) # Unpack data as kwargs
    success, response_data = action_func(esn, **params_to_pass)


    if success:
        return jsonify(response_data), 200 # response_data from client might already be a dict
    else:
        # response_data likely contains an "error" key from the client
        status_code = 502 # Bad Gateway (if error communicating with WirePod)
        if isinstance(response_data, dict) and "client_error" in response_data:
            status_code = 400 # Bad Request (if client-side validation failed before HTTP call)
        return jsonify(response_data), status_code


@bp.route('/<string:esn>/say_text', methods=['POST'])
def say_text_route(esn):
    return _handle_vector_action(vector_http_client.say_text, esn, required_params=['text'])

@bp.route('/<string:esn>/drive_wheels', methods=['POST'])
def drive_wheels_route(esn):
    return _handle_vector_action(vector_http_client.drive_wheels, esn,
                                 required_params=['left_speed_mmps', 'right_speed_mmps', 'duration_ms'])

@bp.route('/<string:esn>/turn_in_place', methods=['POST'])
def turn_in_place_route(esn):
    data = request.get_json()
    # Optional params for turn_in_place
    is_absolute = data.get('is_absolute', False)
    tolerance_deg = data.get('tolerance_deg', 10.0)

    # Create a temporary data source that includes defaults for optional params
    # to pass to _handle_vector_action, which expects all its required_params
    # to be in the data_source it uses.
    # However, vector_http_client.turn_in_place handles defaults itself.
    # So we can directly call it or adapt _handle_vector_action.

    # Let's call directly for more control over optional params
    if not client_imported:
        return jsonify({"error": "Vector HTTP client module not loaded on server."}), 503
    if not data or not all(k in data for k in ("angle_degrees", "speed_dps")):
        return jsonify({"error": "Missing required parameters: angle_degrees, speed_dps"}), 400

    success, response_data = vector_http_client.turn_in_place(
        esn,
        data['angle_degrees'],
        data['speed_dps'],
        is_absolute=data.get('is_absolute', False), # Get with default
        tolerance_deg=data.get('tolerance_deg', 10.0) # Get with default
    )
    if success:
        return jsonify(response_data), 200
    else:
        return jsonify(response_data), 502


@bp.route('/<string:esn>/move_lift', methods=['POST'])
def move_lift_route(esn):
    return _handle_vector_action(vector_http_client.move_lift, esn, required_params=['speed_dps'])

@bp.route('/<string:esn>/set_lift_height', methods=['POST'])
def set_lift_height_route(esn):
    return _handle_vector_action(vector_http_client.set_lift_height, esn, required_params=['height_ratio'])

@bp.route('/<string:esn>/move_head', methods=['POST'])
def move_head_route(esn):
    return _handle_vector_action(vector_http_client.move_head, esn, required_params=['speed_dps'])

@bp.route('/<string:esn>/set_head_angle', methods=['POST'])
def set_head_angle_route(esn):
    return _handle_vector_action(vector_http_client.set_head_angle, esn, required_params=['angle_degrees'])

@bp.route('/<string:esn>/play_animation', methods=['POST'])
def play_animation_route(esn):
    return _handle_vector_action(vector_http_client.play_animation, esn, required_params=['animation_name'])

@bp.route('/<string:esn>/play_animation_trigger', methods=['POST'])
def play_animation_trigger_route(esn):
    return _handle_vector_action(vector_http_client.play_animation_trigger, esn, required_params=['trigger_name'])

@bp.route('/<string:esn>/get_status', methods=['GET'])
def get_status_route(esn):
    # GET requests don't have a JSON body, params are in query string
    # _handle_vector_action needs to be adapted or call client directly
    if not client_imported:
        return jsonify({"error": "Vector HTTP client module not loaded on server."}), 503

    success, response_data = vector_http_client.get_vector_status(esn)
    if success:
        return jsonify(response_data), 200
    else:
        return jsonify(response_data), 502 # Or appropriate error code

@bp.route('/<string:esn>/get_camera_image', methods=['GET'])
def get_camera_image_route(esn):
    if not client_imported:
        return jsonify({"error": "Vector HTTP client module not loaded on server."}), 503

    success, response_data = vector_http_client.get_camera_image_b64(esn)
    if success:
        # Assuming response_data is {"image_base64": "..."}
        return jsonify(response_data), 200
    else:
        # This is expected to fail with current client if WirePod endpoint doesn't exist
        return jsonify(response_data), 501 # Not Implemented (by WirePod for HTTP GET)
```
