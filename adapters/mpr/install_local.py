#!/usr/bin/env python3
"""Install a local, opt-in MPR/Gas City connection for this checkout."""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--city', type=Path, required=True, help='Gas City root directory containing .gc and the HQ Beads database')
parser.add_argument('--target', default='mayor', help='Agent/session target within that city (default: mayor)')
parser.add_argument('--repo', action='append', default=[])
args = parser.parse_args()
city = args.city.resolve(strict=True)
artifacts = city / '.gc/maintainer-pr-review'
if not artifacts.is_dir():
    parser.error(f'MPR artifacts not found: {artifacts}')
if not re.fullmatch(r'[A-Za-z0-9_./-]+', args.target):
    parser.error('Invalid session template')
tools = {}
for name in ['bd', 'gc', 'gh', 'systemctl']:
    tools[name] = shutil.which(name)
    if not tools[name]: parser.error(f'{name} is required')
# Discover the selected city's database namespace before any installation writes.
prefix_result = subprocess.run([tools['bd'], 'config', 'get', 'issue_prefix', '--json'],
                               cwd=city, capture_output=True, text=True, check=True,
                               env={**os.environ, 'GC_CITY_ROOT': str(city)})
issue_prefix = json.loads(prefix_result.stdout).get('value')
if not isinstance(issue_prefix, str) or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]*', issue_prefix):
    parser.error('The selected city must have a valid Beads issue_prefix')
base = Path(os.environ.get('XDG_DATA_HOME', Path.home()/'.local/share')) / 'hold-court/mpr'
base.mkdir(parents=True, exist_ok=True)
repos = set(args.repo)
if not repos and (base/'config.json').exists():
    repos.update(json.loads((base/'config.json').read_text())['repos'])
if not repos:
    for marker in artifacts.glob('*/pr-*/hold-notice.json'):
        metadata = marker.parent/'runs/latest/metadata.json'
        try:
            match = re.match(r'https://github.com/([^/]+/[^/]+)/pull/', json.loads(metadata.read_text())['url'])
            if match: repos.add(match[1])
        except (OSError, ValueError, KeyError): pass
if not repos: parser.error('No repositories found; provide --repo owner/repo')
for name in ['export.py', 'consumer.py']:
    shutil.copyfile(ROOT/'adapters/mpr'/name, base/name)
config = dict(artifact_root=str(artifacts), repos=sorted(repos), feed=str(base/'feed'),
              status=str(base/'status.json'), execution='agent-handoff')
consumer = dict(repos=sorted(repos), city_root=str(city), target=args.target, issue_prefix=issue_prefix,
                feed=str(base/'feed'), rulings=str(base/'rulings'), spool=str(base/'requests'),
                **{name:tools[name] for name in ['bd','gc','gh']})
for name, content in [('config.json',config),('consumer.json',consumer)]:
    (base/name).write_text(json.dumps(content,indent=2)+'\n')
(base/'rulings').mkdir(exist_ok=True)
(base/'requests').mkdir(exist_ok=True)
# Only read operations occur before the connection is configured.
subprocess.run([sys.executable,str(base/'export.py'),'--config',str(base/'config.json')],check=True)
units = Path(os.environ.get('XDG_CONFIG_HOME', Path.home()/'.config'))/'systemd/user'
units.mkdir(parents=True,exist_ok=True)
for name, command, interval in [
    ('hold-court-mpr-feed',[sys.executable,str(base/'export.py'),'--config',str(base/'config.json')],'5min'),
    ('hold-court-mpr-worker',[sys.executable,str(base/'consumer.py'),'worker','--config',str(base/'consumer.json')],'15s')]:
    argv = ' '.join(json.dumps(arg) for arg in command)
    (units/(name+'.service')).write_text(f'[Unit]\nDescription={name}\n\n[Service]\nType=oneshot\nExecStart={argv}\nTimeoutStartSec=20min\nUMask=0077\n')
    (units/(name+'.timer')).write_text(f'[Unit]\nDescription={name}\n\n[Timer]\nOnStartupSec=30s\nOnUnitInactiveSec={interval}\nUnit={name}.service\n\n[Install]\nWantedBy=timers.target\n')
subprocess.run([tools['systemctl'],'--user','daemon-reload'],check=True)
subprocess.run([tools['systemctl'],'--user','enable','--now','hold-court-mpr-feed.timer','hold-court-mpr-worker.timer'],check=True)
hook = [sys.executable,str(base/'consumer.py'),'enqueue','--config',str(base/'consumer.json')]
description = f'Saving sends a task to {args.target}. Discuss requests analysis and a reply here. Other choices authorize the specified PR action on the reviewed head; your required response to the hold guides the agent, which writes appropriate messages and asks here when intent is unclear. Verbatim wording is used only when you explicitly request it.'
text = '\n'.join([f'feed = {json.dumps(config["feed"])}',f'rulings = {json.dumps(consumer["rulings"])}',
                  f'on_ruling = {json.dumps(hook)}',f'consumer_description = {json.dumps(description)}',''])
configuration = ROOT/'holdcourt.toml'
if configuration.exists():
    backup = base/'holdcourt.before-connect.toml'
    if not backup.exists(): shutil.copyfile(configuration,backup)
configuration.write_text(text)
common = Path(subprocess.check_output(['git','rev-parse','--git-common-dir'],cwd=ROOT,text=True).strip())
if not common.is_absolute(): common = ROOT/common
exclude = common/'info/exclude'
with exclude.open('a') as stream: stream.write('\n# Local Hold Court connection\n/holdcourt.toml\n/feed\n/rulings\n')
print(f'Connected to {args.target}. Start/restart with make run. Only newly confirmed decisions are dispatched; legacy trial rulings stay inert.')
