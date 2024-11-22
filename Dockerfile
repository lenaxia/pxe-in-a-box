FROM debian:bookworm-slim

# Install necessary packages
RUN apt-get update && \
    apt-get install -y tftpd-hpa nginx python3 python3-pip python3.11-venv && \
    python3 -m venv /pxe-server/venv && \
    /pxe-server/venv/bin/pip3 install -r /pxe-server/requirements.txt && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Create necessary directories
RUN mkdir -p /pxe-server/tftpboot/bios /pxe-server/tftpboot/uefi /pxe-server/tftpboot/pxelinux.cfg \
    /pxe-server/http/images /pxe-server/http/configs /pxe-server/http/machine-specific \
    /pxe-server/http/preseed /pxe-server/http/kickstart /pxe-server/http/cloud-init \
    /pxe-server/scripts /pxe-server/templates

# Copy configuration files and scripts
COPY templates/os_templates.yaml templates/machine_templates.yaml /pxe-server/templates/
COPY scripts/download_images.py scripts/apply_configurations.py scripts/pxe_menu.py scripts/healthz.py scripts/metrics.py /pxe-server/scripts/
COPY templates/default_pxe_menu.j2 templates/ubuntu.j2 templates/talos_os.j2 templates/debian.j2 templates/raspbian.j2 templates/ubuntu_preseed.j2 templates/debian_preseed.j2 templates/talos_kickstart.j2 templates/cloud_init.j2 /pxe-server/templates/
COPY nginx.conf /etc/nginx/nginx.conf
COPY scripts/entrypoint.sh /pxe-server/scripts/

# Configure TFTP server
RUN mkdir -p /etc/xinetd.d && \
    echo "service tftp\n{\n    socket_type = dgram\n    protocol = udp\n    wait = yes\n    user = nobody\n    server = /usr/sbin/in.tftpd\n    server_args = -s /pxe-server/tftpboot/\n    disable = no\n}" > /etc/xinetd.d/tftp

# Expose necessary ports
EXPOSE 69/udp 80 8080 8081

# Set the entrypoint
ENTRYPOINT ["/bin/bash", "/pxe-server/scripts/entrypoint.sh"]
