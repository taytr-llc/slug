#!/usr/bin/env python3
"""Verify frozen source/input/dependency hashes, then run the independent model."""
import argparse,hashlib,json,subprocess,sys
from pathlib import Path
sys.dont_write_bytecode=True
p=argparse.ArgumentParser();p.add_argument('--repo',required=True);p.add_argument('--out',required=True);a=p.parse_args()
root=Path(__file__).resolve().parent
lock=json.loads((root/'producer-lock.json').read_bytes())
for name,want in lock['bundle_files'].items():
    raw=(root/name).read_bytes()
    if len(raw)!=want['bytes'] or hashlib.sha256(raw).hexdigest()!=want['sha256']:raise SystemExit('bundle binding mismatch: '+name)
for name,want in lock['repository_dependencies'].items():
    raw=(Path(a.repo)/name).read_bytes()
    if hashlib.sha256(raw).hexdigest()!=want:raise SystemExit('repository dependency mismatch: '+name)
subprocess.run([sys.executable,'-B',str(root/'producer/generate.py'),'--out',a.out],check=True)
