import os
import requests
import yaml
from pathlib import Path

# Load the YAML configuration
with open('/pxe-server/templates/os_templates.yaml', 'r') as file:
    os_templates = yaml.safe_load(file)

# Define the download directory
download_dir = Path('/pxe-server/http/images')
download_dir.mkdir(parents=True, exist_ok=True)

# Function to download files
def download_file(url, dest_path):
    if not dest_path.exists():
        print(f"Downloading {url} to {dest_path}")
        response = requests.get(url, stream=True)
        if response.status_code == 200:
            with open(dest_path, 'wb') as f:
                for chunk in response.iter_content(chunk_size=8192):
                    f.write(chunk)
        else:
            print(f"Failed to download {url}: HTTP {response.status_code}")
    else:
        print(f"{dest_path} already exists, skipping download.")

# Download images and configurations
for os_name, config in os_templates.items():
    kernel_path = download_dir / f"{os_name}-{config['kernel']}"
    initrd_path = download_dir / f"{os_name}-{config['initrd']}"
    iso_path = download_dir / f"{os_name}.iso"

    download_file(config['image_url'], kernel_path)
    download_file(config['image_url'], initrd_path)
    download_file(config['iso_url'], iso_path)

    # Download OS-specific configuration files
    if 'preseed_template' in config:
        preseed_path = Path(f'/pxe-server/http/preseed/{os_name}.preseed')
        preseed_path.parent.mkdir(parents=True, exist_ok=True)
        download_file(config['preseed_template'], preseed_path)

    if 'kickstart_template' in config:
        kickstart_path = Path(f'/pxe-server/http/kickstart/{os_name}.ks')
        kickstart_path.parent.mkdir(parents=True, exist_ok=True)
        download_file(config['kickstart_template'], kickstart_path)

    if 'cloud_init_template' in config:
        cloud_init_path = Path(f'/pxe-server/http/cloud-init/{os_name}.yaml')
        cloud_init_path.parent.mkdir(parents=True, exist_ok=True)
        download_file(config['cloud_init_template'], cloud_init_path)
