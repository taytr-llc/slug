#!/usr/bin/env python3
"""One bounded actual native lifetime; root drives normal UI Quit via CUA."""
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import time

W = Path('/Users/taytrrip/slug')
A = W / 'artifacts/set83/ui-actual-attempt4'
B = W / 'artifacts/set83/remote-package/SLUG Remote Pool Pilot.app/Contents/MacOS/SLUGPilot'
H = B.parent.parent / 'Resources/bin/slugpilot'
P = W / 'artifacts/set83/ui-actual-attempt1/client-inputs'
S = json.loads((A / 'execution.json').read_text())
assert hashlib.sha256(B.read_bytes()).hexdigest() == '9ffffcd47239522e1e0960192d3423267d096187325c16b5bf09ca72c5c10cfd'
assert hashlib.sha256(H.read_bytes()).hexdigest() == '8c61109cd45107c5cc565769d9a2b414b8ab39c8e6774f3bfaf9e948e4e50f82'
assert time.time_ns() // 1000000 < S['deadline_at_ms'] - 90000
args = [str(B), '--daily-recoverable-issue', '--daily-wallet-helper', '--daily-wallet-helper-version=2',
        '--data-dir', str(W / 'artifacts/set82/hosted-actual-attempt5/owned/owned-gym'),
        '--remote-attachment-root', str(W / 'artifacts/set83/ui-actual-attempt3/remote-attachment'),
        '--remote-config', str(P / 'remote-config.json'), '--remote-node-rpc', str(P / 'remote-rpc.json'),
        '--remote-pool-credential', str(P / 'pool.credential'),
        '--remote-admission-credential', str(P / 'admission.credential'), '--candidate-name-version=1']
r = dict(version=1, started_at_ms=time.time_ns() // 1000000, deadline_at_ms=S['deadline_at_ms'],
         source_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
         app_sha256=hashlib.sha256(B.read_bytes()).hexdigest(), signals=[], joined=False)
with (A / 'public/native-window.stdout').open('xb') as out, (A / 'public/native-window.stderr').open('xb') as err:
    c = subprocess.Popen(args, stdin=subprocess.DEVNULL, stdout=out, stderr=err, start_new_session=True)
    r.update(pid=c.pid, pgid=c.pid)
    (A / 'public/native-window-start.json').write_text(json.dumps(r, indent=2) + '\n')
    try:
        rc = c.wait(timeout=max(1, (S['deadline_at_ms'] - 45000) / 1000 - time.time()))
    except subprocess.TimeoutExpired:
        os.killpg(c.pid, signal.SIGTERM); r['signals'].append('TERM')
        try:
            rc = c.wait(timeout=30)
        except subprocess.TimeoutExpired:
            os.killpg(c.pid, signal.SIGKILL); r['signals'].append('KILL'); rc = c.wait(timeout=10)
    r.update(joined=True, exit_code=rc, ended_at_ms=time.time_ns() // 1000000,
             natural_exit=rc == 0 and not r['signals'])
    (A / 'public/native-window-process.json').write_text(json.dumps(r, indent=2) + '\n')
print(json.dumps(r))
raise SystemExit(0 if r['natural_exit'] else 1)
