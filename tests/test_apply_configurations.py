import os
import yaml
import jinja2
from pathlib import Path
import unittest
from unittest.mock import patch, mock_open, MagicMock
from scripts.apply_configurations import render_template, apply_configurations

class TestApplyConfigurations(unittest.TestCase):

    @patch('builtins.open', new_callable=mock_open, read_data='template content')
    @patch('jinja2.FileSystemLoader')
    @patch('jinja2.Environment')
    def test_render_template(self, mock_env, mock_loader, mock_open):
        mock_template = MagicMock()
        mock_template.render.return_value = 'rendered content'
        mock_env.return_value.get_template.return_value = mock_template

        context = {'key': 'value'}
        output_file = Path('/tmp/output.cfg')
        render_template('template.j2', output_file, context)

        mock_loader.assert_called_once_with(searchpath="/pxe-server/templates")
        mock_env.assert_called_once_with(loader=mock_loader.return_value)
        mock_template.render.assert_called_once_with(context)
        mock_open.assert_called_once_with(output_file, 'w')
        mock_open.return_value.write.assert_called_once_with('rendered content')

    @patch('builtins.open', new_callable=mock_open, read_data='os templates content')
    @patch('builtins.open', new_callable=mock_open, read_data='machine templates content')
    @patch('scripts.apply_configurations.render_template')
    def test_apply_configurations(self, mock_render_template, mock_open_os, mock_open_machine):
        os_templates = {
            'os1': {
                'config_template': 'os1.cfg.j2',
                'preseed_template': 'os1.preseed.j2',
                'kickstart_template': 'os1.ks.j2',
                'cloud_init_template': 'os1.yaml.j2'
            }
        }
        machine_templates = {
            'machines': [
                {
                    'mac_address': '00:11:22:33:44:55',
                    'os': 'os1'
                }
            ]
        }

        mock_open_os.return_value.__enter__.return_value.read.return_value = yaml.dump(os_templates)
        mock_open_machine.return_value.__enter__.return_value.read.return_value = yaml.dump(machine_templates)

        apply_configurations()

        expected_calls = [
            (('os1.cfg.j2', Path('/pxe-server/http/configs/os1.cfg'), os_templates['os1']), {}),
            (('os1.preseed.j2', Path('/pxe-server/http/preseed/os1.preseed'), os_templates['os1']), {}),
            (('os1.ks.j2', Path('/pxe-server/http/kickstart/os1.ks'), os_templates['os1']), {}),
            (('os1.yaml.j2', Path('/pxe-server/http/cloud-init/os1.yaml'), os_templates['os1']), {}),
            (('os1.j2', Path('/pxe-server/http/configs/00:11:22:33:44:55.cfg'), machine_templates['machines'][0]), {}),
            (('os1.preseed.j2', Path('/pxe-server/http/preseed/00:11:22:33:44:55.preseed'), machine_templates['machines'][0]), {}),
            (('os1.ks.j2', Path('/pxe-server/http/kickstart/00:11:22:33:44:55.ks'), machine_templates['machines'][0]), {}),
            (('os1.yaml.j2', Path('/pxe-server/http/cloud-init/00:11:22:33:44:55.yaml'), machine_templates['machines'][0]), {})
        ]
        mock_render_template.assert_has_calls([call(*args, **kwargs) for args, kwargs in expected_calls])

if __name__ == '__main__':
    unittest.main()
