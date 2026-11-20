#!/usr/bin/env python3
"""Reproduce the immutable, independently reviewed marker corpus."""
from pathlib import Path
import hashlib
import os
import subprocess
import sys
import tempfile

root = Path(__file__).resolve().parent
expected = "3d31e7f195fa20456978739258f5517fbe2a4d97d93b379c9b8ade941daa3147"
approved = "b99be7d00028248890870bd677c9bfa220a1c3b1c51da725a148d23d054c6dcc"
raw = (root / "setup-v1.json").read_bytes()
if hashlib.sha256(raw).hexdigest() != expected:
    raise SystemExit("reference corpus hash mismatch")
env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1")
with tempfile.TemporaryDirectory(prefix="slug-restored-setup-reference-") as d:
    output = Path(d) / "setup-v1.json"
    subprocess.run([
        sys.executable, str(root / "generate_setup_reference.py"), "--execute",
        "--input-root", str(root / "inputs"),
        "--approved-producer-lock", str(root / "producer-lock.approved.json"),
        "--approved-lock-sha256", approved, "--output", str(output),
    ], check=True, env=env, timeout=30)
    if output.read_bytes() != raw:
        raise SystemExit("reproduced marker bytes differ")
print("PASS: independently generated marker bytes reproduced exactly")
