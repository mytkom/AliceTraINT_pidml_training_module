from flask import Flask, request, jsonify
import json

app = Flask(__name__)

# Fake database state to ensure we only send the task once
task_sent = False

@app.route('/training-machines/<int:machine_id>/training-task', methods=['GET'])
def get_task(machine_id):
    global task_sent
    if not task_sent:
        print(f"\n[SERVER] Machine {machine_id} requested a task. Sending Task #1...")
        task_sent = True
        return jsonify({
            "ID": 1,
            "AODFiles": [
                {"Path": "data/LHC23k4g-535069-from-001-to-006.root"}
            ],
            "Configuration": {
                "subset_size": 50000,
                # --- NEW KEYS FOR PDI INTEGRATION ---
                "input_file": "data/LHC23k4g-535069-from-001-to-006.root",
                "use_gpu": False,
                "max_epochs": 1, # Set to 1 for fast testing
                # ------------------------------------
                "embed_hidden": 16,
                "d_model": 16,
                "ff_hidden": 32,
                "pool_hidden": 16,
                "num_heads": 1,
                "num_blocks": 1,
                "start_lr": 0.001,
                "dropout": 0.0,
                "bs": 1024, # Reduced batch size for stability
                "undersample": False
            }
        })
    return '', 404

@app.route('/training-tasks/<int:tt_id>/status', methods=['POST'])
def update_status(tt_id):
    status_map = {0: "Failed", 1: "Queued", 2: "Training", 3: "Benchmarking", 4: "Completed"}
    status_code = request.json.get('Status')
    status_name = status_map.get(status_code, "Unknown")
    print(f"[SERVER] Task {tt_id} status updated to: {status_name} ({status_code})")
    return '', 200

@app.route('/training-tasks/<int:tt_id>/training-task-results', methods=['POST'])
def upload_result(tt_id):
    file_name = request.form.get('name')
    file_type = request.form.get('file-type')
    description = request.form.get('description')
    print(f"[SERVER] Task {tt_id} received result: {file_name} (Type: {file_type}, Desc: {description})")
    return '', 201

if __name__ == '__main__':
    print("AliceTraINT Mock Server (PDI V2 compatible) running on http://localhost:8080")
    print("Press Ctrl+C to stop.")
    app.run(port=8080, debug=False)
