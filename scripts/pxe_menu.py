import os
import yaml
import jinja2
from pathlib import Path

# Load the YAML configuration
with open('/pxe-server/templates/machine_templates.yaml', 'r') as file:
    machine_templates = yaml.safe_load(file)

# Define the PXE menu directory
menu_dir = Path('/pxe-server/tftpboot/pxelinux.cfg')
menu_dir.mkdir(parents=True, exist_ok=True)

# Function to render PXE menu
def render_pxe_menu(mac_address, os_name):
    template_loader = jinja2.FileSystemLoader(searchpath="/pxe-server/templates")
    template_env = jinja2.Environment(loader=template_loader)
    template = template_env.get_template('pxe_menu.j2')
    output = template.render(mac_address=mac_address, os_name=os_name)
    output_file = menu_dir / f"{mac_address}"
    with open(output_file, 'w') as f:
        f.write(output)

# Generate PXE menu for each machine
for machine in machine_templates['machines']:
    mac_address = machine['mac_address']
    os_name = machine['os']
    render_pxe_menu(mac_address, os_name)

# Generate default PXE menu
default_menu_file = menu_dir / 'default'
render_template('default_pxe_menu.j2', default_menu_file, {})
