from flask import Flask, jsonify

app = Flask(__name__)

# Initialize metrics
tftp_requests = 0
http_requests = 0
active_machines = 0

@app.route('/metrics', methods=['GET'])
def metrics():
    return jsonify(
        tftp_requests=tftp_requests,
        http_requests=http_requests,
        active_machines=active_machines
    ), 200

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8081)
