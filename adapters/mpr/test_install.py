"""Exercise installation against an arbitrary city with all commands stubbed."""
import json
import os
from pathlib import Path
import runpy
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


class InstallTests(unittest.TestCase):
    def test_discovers_selected_city_prefix(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            checkout = root/'checkout'
            scripts = checkout/'adapters/mpr'
            scripts.mkdir(parents=True)
            (checkout/'.git/info').mkdir(parents=True)
            for name in ['install_local.py', 'export.py', 'consumer.py']:
                shutil.copyfile(Path(__file__).with_name(name), scripts/name)
            city = root/'arbitrary-city-name'
            (city/'.gc/maintainer-pr-review').mkdir(parents=True)
            data = root/'data'
            calls = []

            def run(argv, **kwargs):
                calls.append((argv, kwargs))
                return subprocess.CompletedProcess(argv, 0, json.dumps({'value':'custom'}))

            with patch.dict(os.environ, {'XDG_DATA_HOME':str(data), 'XDG_CONFIG_HOME':str(root/'config')}), \
                 patch.object(sys, 'argv', ['install_local.py', '--city', str(city), '--target', 'reviewer', '--repo', 'owner/repo']), \
                 patch('shutil.which', side_effect=lambda name: '/tools/'+name), \
                 patch('subprocess.run', side_effect=run), \
                 patch('subprocess.check_output', return_value='.git\n'):
                runpy.run_path(str(scripts/'install_local.py'), run_name='__main__')
            config = json.loads((data/'hold-court/mpr/consumer.json').read_text())
            self.assertEqual(config['issue_prefix'], 'custom')
            self.assertEqual(config['city_root'], str(city))
            self.assertEqual(config['target'], 'reviewer')
            self.assertEqual(calls[0][0], ['/tools/bd', 'config', 'get', 'issue_prefix', '--json'])
            self.assertEqual(calls[0][1]['cwd'], city)
            self.assertIn('annotations guide the agent', (checkout/'holdcourt.toml').read_text())
