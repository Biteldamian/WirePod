from flask import Flask, jsonify
import os

# Attempt to import routes blueprint
try:
    from .routes import vector_routes
    routes_imported = True
except ImportError:
    print("Warning: vector_routes blueprint not found. Ensure routes/vector_routes.py exists.")
    routes_imported = False

app = Flask(__name__)

# Register Blueprints if imported successfully
if routes_imported:
    app.register_blueprint(vector_routes.bp)
else:
    print("Vector routes blueprint not registered. API endpoints under /vector will not be available.")

@app.route('/')
def hello_world():
    """A simple route to confirm the server is running."""
    return jsonify(message='Hello, VectorBrain is alive and processing!')

@app.route('/health')
def health_check():
    """Health check endpoint."""
    return jsonify(status="healthy", message="VectorBrain server is up and running."), 200

if __name__ == '__main__':
    # Configuration for host and port
    # Use environment variables or a config file for more flexibility
    host = os.environ.get('VB_HOST', '0.0.0.0')
    port = int(os.environ.get('VB_PORT', 5001)) # Flask expects port to be an int
    debug_mode = os.environ.get('VB_DEBUG', 'True').lower() in ('true', '1', 't')

    print(f"Starting VectorBrain server on {host}:{port} (Debug: {debug_mode})")
    if not routes_imported:
        print("Note: vector_routes.py was not imported. Vector control API endpoints will be missing.")

    app.run(debug=debug_mode, host=host, port=port)
```
