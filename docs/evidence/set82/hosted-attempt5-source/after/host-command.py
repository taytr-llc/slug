#!/usr/bin/env python3
"""Bounded, explicitly scoped SSH transport for the disposable SLUG fixture."""
import argparse
import os
from pathlib import Path
import shlex
import subprocess
import time
import json

p = argparse.ArgumentParser()
p.add_argument('--service', choices=['node', 'pool'], required=True)
p.add_argument('--output', type=Path, required=True)
p.add_argument('--timeout', type=int, default=60)
p.add_argument('command', nargs=argparse.REMAINDER)
a = p.parse_args()
assert a.command and a.command[0] == '--' and len(a.command) > 1
assert 1 <= a.timeout <= 1080
assert a.output.is_absolute() and a.output.parent.is_dir()
os.umask(0o077)
env = os.environ.copy()
env.pop('RAILWAY_TOKEN', None)
env.pop('RAILWAY_API_TOKEN', None)
service = {'node': 'c7ba8d73-fe01-418e-8f7b-14a141da8043',
           'pool': 'dedabe18-acd1-4dfe-a775-8bd02f799717'}[a.service]
cmd = ['railway', 'ssh', '--project', '62fdb666-7f67-4cf3-98f0-6177f099b580',
       '--environment', '3fd7da79-0f2f-4077-bf29-e7035d62ac57',
       '--service', service, '--', shlex.join(a.command[1:])]
started = time.time()
with a.output.open('xb') as stream, Path(str(a.output)+'.stderr').open('xb') as err:
    result = subprocess.run(cmd, stdout=stream, stderr=err, env=env, timeout=a.timeout)
report = {'service': service, 'started': started, 'elapsed': time.time()-started,
          'exit_code': result.returncode, 'output': str(a.output)}
Path(str(a.output)+'.status.json').write_text(json.dumps(report, indent=2)+'\n')
print(json.dumps(report))
raise SystemExit(result.returncode)
