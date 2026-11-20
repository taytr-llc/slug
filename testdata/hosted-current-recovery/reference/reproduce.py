#!/usr/bin/env python3
"""Reproduce frozen public Set71 references; requires the paired wallet checkout."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

sys.dont_write_bytecode = True
here = Path(__file__).resolve().parent
repo = here.parents[2]
wallet = repo.parent / 'slugwallet'
expected = {
    'provider': here.parent/'provider.json',
    'public': wallet/'wallet/testdata/hosted-current-recovery/public.json',
    'current': wallet/'recoveryvault/testdata/hosted-current-recovery/current.json',
}
sha = lambda raw: hashlib.sha256(raw).hexdigest()
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output', required=True, type=Path,
                    help='New directory; existing paths are never replaced')
args = parser.parse_args()
args.output.mkdir(parents=False, exist_ok=False)
inputs = here/'preparation/inputs'
env = dict(os.environ, PYTHONDONTWRITEBYTECODE='1')
results = []
for family in ('provider', 'public', 'current'):
    lock = here/(family+'-producer-lock-v1.json')
    output = args.output/(family+'.json')
    cmd = [sys.executable, '-B', str(here/'source'/('generate_'+family+'_reference.py')),
           '--execute', '--approved-producer-lock', str(lock),
           '--approved-lock-sha256', sha(lock.read_bytes()), '--output', str(output)]
    if family == 'current':
        cmd += ['--inputs', str(inputs), '--input-lock', str(here/'preparation/input-lock-v1.json')]
    else:
        cmd += ['--input-root', str(inputs)]
    with (args.output/(family+'.log')).open('xb') as log:
        result = subprocess.run(cmd, env=env, stdout=log, stderr=subprocess.STDOUT)
    matches = output.exists() and output.read_bytes() == expected[family].read_bytes()
    row = dict(family=family, exit_code=result.returncode, exact_match=matches)
    if output.exists():
        row['sha256'] = sha(output.read_bytes())
    results.append(row)
    print(json.dumps(row), flush=True)
with (args.output/'reproduction.json').open('x') as stream:
    json.dump(results, stream, indent=2)
    stream.write('\n')
raise SystemExit(0 if all(row['exit_code']==0 and row['exact_match'] for row in results) else 1)
