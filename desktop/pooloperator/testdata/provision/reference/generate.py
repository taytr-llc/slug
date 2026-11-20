#!/usr/bin/env python3
"""Execute only an explicitly approved frozen public reference producer."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys

sys.dont_write_bytecode = True
BASE = Path(__file__).resolve().parent


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    sys.modules[name] = value
    spec.loader.exec_module(value)
    return value


def verify(lock):
    if lock.get('version') != 1 or lock.get('approved_by') != 'root':
        raise ValueError('reviewed root source lock required')
    for row in lock['inputs']:
        p = BASE / row['path']
        if not p.is_file() or p.stat().st_size != row['bytes'] or digest(p) != row['sha256']:
            raise ValueError('frozen source mismatch: '+row['path'])


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--execute', action='store_true')
    ap.add_argument('--lock', required=True)
    ap.add_argument('--lock-sha', required=True)
    ap.add_argument('--output', required=True)
    a = ap.parse_args()
    if not a.execute:
        raise ValueError('explicit execution required')
    lock_path, output = Path(a.lock), Path(a.output)
    if not output.is_absolute() or output.exists() or digest(lock_path) != a.lock_sha:
        raise ValueError('exact lock and fresh absolute output required')
    lock = json.loads(lock_path.read_bytes())
    verify(lock)
    def load(name): return json.loads((BASE/'inputs'/name).read_bytes())
    R = module('provision_inherited_recovery', BASE/'inputs/inherited/recovery/reference.py')
    E = module('provision_inherited_crypto', BASE/'inputs/inherited/crypto.py')
    C = module('provision_certificate', BASE/'certificate.py')
    P = module('provision_model', BASE/'model.py')
    recipe = load('recipe.json')
    inherited = dict(setup=load('public-setup-v1.json'), wallet=load('public-wallet16.json'), authority=load('public-authority-v2.json'), recovery=load('published-recovery-v1.json'), seed_recipe=load('authority-seed-recipe.json'))
    model = P.Model(recipe, inherited, R, E)
    result = P.build_vectors(model, C.build(E, recipe['certificate']))
    result['provenance'] = dict(producer_lock_sha256=a.lock_sha, input_catalog_sha256=digest(BASE/'input-catalog.json'))
    raw = json.dumps(result, separators=(',', ':'), ensure_ascii=True).encode('ascii')+b'\n'
    if len(raw) > recipe['max_output_bytes']:
        raise ValueError('finite output cap')
    verify(lock)
    with output.open('xb') as f:
        f.write(raw)
        f.flush()
        os.fsync(f.fileno())
    print(json.dumps(dict(output=str(output), bytes=len(raw), sha256=hashlib.sha256(raw).hexdigest(), counts=result['counts']), separators=(',', ':')))


if __name__ == '__main__':
    main()
