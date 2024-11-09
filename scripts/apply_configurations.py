import os
import yaml
import jinja2
from pathlib import Path

# Load the YAML configuration
with open('/pxe-server/templates/os_templates.yaml', 'r') as file:
    os_templates = yaml.safe_load(file)

with open('/pxe-server/templates/machine_templates.yaml', 'r') as file:
    machine_templates = yaml.safe_load(file)

# Define the configuration directory
config_dir = Path('/pxe-server/http/configs')
config_dir.mkdir(parents=True, exist_ok=True)

# Function to render Jinja templates
def render_template(template_file, output_file, context):
    template_loader = jinja2.FileSystemLoader(searchpath="/pxe-server/templates")
    template_env = jinja2.Environment(loader=template_loader)
    template = template_env.get_template(template_file)
    output = template.render(context)
    with open(output_file, 'w') as f:
        f.write(output)

# Apply core OS configurations
for os_name, config in os_templates.items():
    template_file = config['config_template']
    output_file = config_dir / f"{os_name}.cfg"
    render_template(template_file, output_file, config)

    # Render OS-specific configuration files
    if 'preseed_template' in config:
        preseed_template = config['preseed_template']
        preseed_output = Path(f'/pxe-server/http/preseed/{os_name}.preseed')
        render_template(preseed_template, preseed_output, config)

    if 'kickstart_template' in config:
        kickstart_template = config['kickstart_template']
        kickstart_output = Path(f'/pxe-server/http/kickstart/{os_name}.ks')
        render_template(kickstart_template, kickstart_output, config)

    if 'cloud_init_template' in config:
        cloud_init_template = config['cloud_init_template']
        cloud_init_output = Path(f'/pxe-server/http/cloud-init/{os_name}.yaml')
        render_template(cloud_init_template, cloud_init_output, config)

# Apply machine-specific configurations
for machine in machine_templates['machines']:
    mac_address = machine['mac_address']
    os_name = machine['os']
    output_file = config_dir / f"{mac_address}.cfg"
    render_template(f"{os_name}.j2", output_file, machine)

    # Render machine-specific configuration files
    if 'preseed_template' in os_templates[os_name]:
        preseed_template = os_templates[os_name]['preseed_template']
        preseed_output = Path(f'/pxe-server/http/preseed/{mac_address}.preseed')
        render_template(preseed_template, preseed_output, machine)

    if 'kickstart_template' in os_templates[os_name]:
        kickstart_template = os_templates[os_name]['kickstart_template']
        kickstart_output = Path(f'/pxe-server/http/kickstart/{mac_address}.ks')
        render_template(kickstart_template, kickstart_output, machine)

    if 'cloud_init_template' in os_templates[os_name]:
        cloud_init_template = os_templates[os_name]['cloud_init_template']
        cloud_init_output = Path(f'/pxe-server/http/cloud-init/{mac_address}.yaml')
        render_template(cloud_init_template, cloud_init_output, machine)
