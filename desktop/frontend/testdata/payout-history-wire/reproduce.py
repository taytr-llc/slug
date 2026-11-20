#!/usr/bin/env python3
import argparse,hashlib,json,subprocess,sys
from pathlib import Path
sys.dont_write_bytecode=True
p=argparse.ArgumentParser();p.add_argument('--core-bundle',required=True);p.add_argument('--out',required=True);a=p.parse_args()
root=Path(__file__).resolve().parent;core=Path(a.core_bundle).resolve()
lock=json.loads((root/'producer-lock.json').read_bytes())
for name,want in lock['bundle_files'].items():
    raw=(root/name).read_bytes()
    if len(raw)!=want['bytes'] or hashlib.sha256(raw).hexdigest()!=want['sha256']:raise SystemExit('wire binding mismatch: '+name)
raw=(core/'producer-lock.json').read_bytes()
if hashlib.sha256(raw).hexdigest()!=lock['core_lock_sha256']:raise SystemExit('core lock mismatch')
for name,want in json.loads(raw)['bundle_files'].items():
    b=(core/name).read_bytes()
    if len(b)!=want['bytes'] or hashlib.sha256(b).hexdigest()!=want['sha256']:raise SystemExit('core binding mismatch: '+name)
subprocess.run([sys.executable,'-B',str(root/'producer/generate_wire.py'),'--core-bundle',str(core),'--out',a.out],check=True)
