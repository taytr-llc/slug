#!/usr/bin/env python3
"""One bounded public reproduction, no imports of reference models."""
import argparse
import hashlib
from pathlib import Path
import subprocess
import sys

def main():
    p=argparse.ArgumentParser();p.add_argument('--python',default=sys.executable)
    p.add_argument('--approved-producer-lock',type=Path,required=True)
    p.add_argument('--approved-lock-sha256',required=True)
    p.add_argument('--expected',type=Path,required=True)
    p.add_argument('--output',type=Path,required=True)
    args=p.parse_args();root=Path(__file__).resolve().parent
    cmd=[args.python,str(root/'generate_reference.py'),'--execute','--input-root',str(root/'inputs'),'--input-catalog',str(root/'input-catalog.json'),'--approved-producer-lock',str(args.approved_producer_lock),'--approved-lock-sha256',args.approved_lock_sha256,'--output',str(args.output)]
    result=subprocess.run(cmd,timeout=600,check=False)
    if result.returncode:raise SystemExit(result.returncode)
    actual=(args.output/'wallet-v1.json').read_bytes();expected=args.expected.read_bytes()
    if actual!=expected:raise SystemExit('independent bytes differ; preserve both attempts')
    print('byte-identical '+hashlib.sha256(actual).hexdigest())

if __name__=='__main__':main()
