import os
import yaml
import jinja2

# Load the YAML configuration
with open('./templates/os_templates.yaml', 'r') as file:
    os_templates = yaml.safe_load(file)

with open('./templates/machine_templates.yaml', 'r') as file:
    machine_templates = yaml.safe_load(file)

# Define the Jinja2 template for the README
readme_template = """
# PXE Boot Server

This project provides a Docker image that sets up a PXE Boot server to support both UEFI and BIOS booting. It includes configurations for multiple operating systems (Ubuntu, Talos OS, Debian, Raspbian) and allows for machine-specific configurations using YAML and Jinja2 templates.

## How to Use

### Docker

1. **Build the Docker Image:**
   ```sh
   docker build -t pxe-server .
   ```

2. **Run the Docker Container:**
   ```sh
   docker run -d --name pxe-server -p 69:69/udp -p 80:80 -p 8080:8080 -p 8081:8081 -v <nfs_mount>:/pxe-server/nfs/mnt pxe-server
   ```

### Kubernetes

1. **Create a Persistent Volume Claim (PVC) for NFS:**
   ```yaml
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: pxe-server-pvc
   spec:
     accessModes:
       - ReadWriteMany
     storageClassName: <nfs-storage-class>
     resources:
       requests:
         storage: 10Gi
   ```

2. **Deploy the PXE Server:**
   ```yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: pxe-server
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: pxe-server
     template:
       metadata:
         labels:
           app: pxe-server
       spec:
         containers:
         - name: pxe-server
           image: pxe-server:latest
           ports:
           - containerPort: 69
             protocol: UDP
           - containerPort: 80
           - containerPort: 8080
           - containerPort: 8081
           volumeMounts:
           - mountPath: /pxe-server/nfs/mnt
             name: nfs-volume
         volumes:
         - name: nfs-volume
           persistentVolumeClaim:
             claimName: pxe-server-pvc
   ```

3. **Expose the Deployment:**
   ```yaml
   apiVersion: v1
   kind: Service
   metadata:
     name: pxe-server
   spec:
     selector:
       app: pxe-server
     ports:
     - name: tftp
       port: 69
       targetPort: 69
       protocol: UDP
     - name: http
       port: 80
       targetPort: 80
     - name: healthz
       port: 8080
       targetPort: 8080
     - name: metrics
       port: 8081
       targetPort: 8081
     type: LoadBalancer
   ```

## Configuration

### Available OS Templates

{% for os_name, config in os_templates.items() %}
- **{{ os_name }}:**
  - Kernel: {{ config['kernel'] }}
  - Initrd: {{ config['initrd'] }}
  - Configuration Template: {{ config['config_template'] }}
  - Preseed Template: {{ config.get('preseed_template', 'N/A') }}
  - Kickstart Template: {{ config.get('kickstart_template', 'N/A') }}
  - Cloud-Init Template: {{ config.get('cloud_init_template', 'N/A') }}
  - Image URL: {{ config['image_url'] }}
  - ISO URL: {{ config['iso_url'] }}
{% endfor %}

### Available Machine Templates

{% for machine in machine_templates['machines'] %}
- **MAC Address: {{ machine['mac_address'] }}**
  - OS: {{ machine['os'] }}
  - Static IP: {{ machine['static_ip'] }}
  - Gateway: {{ machine['gateway'] }}
  - DNS Servers: {{ machine['dns_servers'] | join(', ') }}
  - Partitions:
    {% for partition in machine['partitions'] %}
    - Disk: {{ partition['disk'] }}
      - Mount Point: {{ partition['mount_point'] }}
      - Size: {{ partition['size'] }}
    {% endfor %}
  - SSH Keys:
    {% for key in machine['ssh_keys'] %}
    - {{ key }}
    {% endfor %}
  - Default User: {{ machine['default_user'] }}
  - Default Password: {{ machine['default_password'] }}
{% endfor %}
"""

# Define the Jinja2 template environment
template_loader = jinja2.FileSystemLoader(searchpath="./.github/workflows")
template_env = jinja2.Environment(loader=template_loader)
template = template_env.from_string(readme_template)

# Render the README content
readme_content = template.render(os_templates=os_templates, machine_templates=machine_templates)

# Write the README content to README.md
with open('README.md', 'w') as readme_file:
    readme_file.write(readme_content)

# Generate additional documentation files
docs_template = """
# PXE Boot Server Documentation

## Overview

This project provides a Docker image that sets up a PXE Boot server to support both UEFI and BIOS booting. It includes configurations for multiple operating systems (Ubuntu, Talos OS, Debian, Raspbian) and allows for machine-specific configurations using YAML and Jinja2 templates.

## Configuration

### OS Templates

{% for os_name, config in os_templates.items() %}
- **{{ os_name }}:**
  - Kernel: {{ config['kernel'] }}
  - Initrd: {{ config['initrd'] }}
  - Configuration Template: {{ config['config_template'] }}
  - Preseed Template: {{ config.get('preseed_template', 'N/A') }}
  - Kickstart Template: {{ config.get('kickstart_template', 'N/A') }}
  - Cloud-Init Template: {{ config.get('cloud_init_template', 'N/A') }}
  - Image URL: {{ config['image_url'] }}
  - ISO URL: {{ config['iso_url'] }}
{% endfor %}

### Machine Templates

{% for machine in machine_templates['machines'] %}
- **MAC Address: {{ machine['mac_address'] }}**
  - OS: {{ machine['os'] }}
  - Static IP: {{ machine['static_ip'] }}
  - Gateway: {{ machine['gateway'] }}
  - DNS Servers: {{ machine['dns_servers'] | join(', ') }}
  - Partitions:
    {% for partition in machine['partitions'] %}
    - Disk: {{ partition['disk'] }}
      - Mount Point: {{ partition['mount_point'] }}
      - Size: {{ partition['size'] }}
    {% endfor %}
  - SSH Keys:
    {% for key in machine['ssh_keys'] %}
    - {{ key }}
    {% endfor %}
  - Default User: {{ machine['default_user'] }}
  - Default Password: {{ machine['default_password'] }}
{% endfor %}
"""

# Render the documentation content
docs_content = template.render(os_templates=os_templates, machine_templates=machine_templates)

# Write the documentation content to docs/index.md
with open('docs/index.md', 'w') as docs_file:
    docs_file.write(docs_content)
