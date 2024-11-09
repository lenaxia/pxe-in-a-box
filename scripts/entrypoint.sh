#!/bin/bash

# Start the TFTP and HTTP servers
service xinetd start
service nginx start

# Run the Flask application
/pxe-server/venv/bin/python /pxe-server/scripts/metrics.py &
/pxe-server/venv/bin/python /pxe-server/scripts/healthz.py &
/pxe-server/venv/bin/python /pxe-server/scripts/apply_configurations.py

# Start TFTP and HTTP servers
service xinetd start
service nginx start

# Run Python scripts to download images, apply configurations, and generate PXE menu
python3 /pxe-server/scripts/download_images.py
python3 /pxe-server/scripts/apply_configurations.py
python3 /pxe-server/scripts/pxe_menu.py

# Start health and metrics endpoints
python3 /pxe-server/scripts/healthz.py &
python3 /pxe-server/scripts/metrics.py &

# Keep the container running
tail -f /dev/null
