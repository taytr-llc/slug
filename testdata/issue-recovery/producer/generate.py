#!/usr/bin/env python3
"""Root-admitted independent producer. Never import copied documentary models."""
import argparse
import hashlib
import importlib
import json
import os
from pathlib import Path
import stat
import sys
import time

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parent
BASE_COMMIT = 'fdd40d8edd9a685613f1f667f0074e56ec4e7da5'
CONTRACT = 'f3e019a840d6b69d853350763364a3fc41f10ea899f00ed6f164db5d58a95610'
PLAN = '568251bda8d3589874c8dccd0652442c1be7b3b218f40518f70606ad1279c99d'
MODULES = ('core', 'fixtures', 'states', 'outbox', 'lifecycle', 'wire', 'framing', 'bounds')
UNBOUND_METADATA = {'producer-lock.proposed.json', 'source-ready.json'}
MAX_INPUT = 32 << 20
MAX_OUTPUT = 16 << 20


def require(okay, message):
    if not okay:
        raise ValueError(message)


def read_exact(path, maximum):
    # Identity-bracket every read; regular files only, no symlink traversal.
    require(path.is_absolute() and path.parent.resolve() == path.parent, 'noncanonical-input-parent')
    before = path.lstat()
    require(stat.S_ISREG(before.st_mode) and before.st_size <= maximum, 'input-kind-size')
    flags = os.O_RDONLY | getattr(os, 'O_NOFOLLOW', 0)
    fd = os.open(str(path), flags)
    try:
        opened = os.fstat(fd)
        require((before.st_dev, before.st_ino, before.st_size) == (opened.st_dev, opened.st_ino, opened.st_size), 'input-replaced-at-open')
        chunks, size = [], 0
        while True:
            raw = os.read(fd, min(65536, maximum + 1 - size))
            if not raw:
                break
            chunks.append(raw)
            size += len(raw)
            require(size <= maximum, 'input-grew')
        after = os.fstat(fd)
    finally:
        os.close(fd)
    final = path.lstat()
    signature = lambda s: (s.st_dev, s.st_ino, s.st_size, s.st_mtime_ns, s.st_ctime_ns)
    require(signature(before) == signature(opened) == signature(after) == signature(final), 'input-changed-during-read')
    return b''.join(chunks)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def document(raw):
    def pairs(items):
        out = {}
        for k, v in items:
            require(k not in out, 'duplicate-lock-field')
            out[k] = v
        return out
    return json.loads(raw.decode('utf-8'), object_pairs_hook=pairs)


def admission(lock_path, lock_raw):
    lock = document(lock_raw)
    require(lock.get('version') == 1 and lock.get('execution_authorized') is True, 'root-execution-authorization-required')
    require(lock.get('base_commit') == BASE_COMMIT and lock.get('contract_sha256') == CONTRACT
            and lock.get('plan_sha256') == PLAN, 'selection-binding')
    require(lock.get('limits') == dict(seconds=120, log_bytes=1 << 20, output_bytes=MAX_OUTPUT, input_bytes=MAX_INPUT), 'execution-limits')
    require(type(lock.get('files')) is list and lock['files'], 'empty-lock')
    files, total = {}, 0
    for item in lock['files']:
        require(set(item) == {'path', 'bytes', 'sha256'}, 'lock-entry-fields')
        rel = Path(item['path'])
        require(not rel.is_absolute() and rel.as_posix() == item['path'] and rel.parts
                and all(p not in ('', '.', '..') for p in rel.parts), 'lock-relative-path')
        require(item['path'] not in files and item['path'] not in UNBOUND_METADATA, 'lock-duplicate-or-self')
        require(type(item['bytes']) is int and 0 <= item['bytes'] <= MAX_INPUT, 'lock-byte-count')
        total += item['bytes']
        require(total <= MAX_INPUT, 'total-input-bound')
        files[item['path']] = item
    for mandatory in ['generate.py', 'recipe.json', 'input-catalog.json', 'README.md',
                      'inputs/set79/reference-v1.json', 'inputs/manifest-job/manifest_job_v1.json',
                      'inputs/name-v2/independent/reference-v2.json',
                      'inputs/selection/ISSUE_RECOVERY_CONTRACT.md', 'inputs/selection/REFERENCE_PLAN.md'] + [x + '.py' for x in MODULES]:
        require(mandatory in files, 'required-input-absent:' + mandatory)
    ignored = set(UNBOUND_METADATA)
    if lock_path.parent == ROOT:
        ignored.add(lock_path.name)
    actual = set()
    for path in ROOT.rglob('*'):
        require(not path.is_symlink(), 'input-tree-symlink')
        if path.is_dir():
            continue
        require(path.is_file(), 'input-tree-nonregular')
        rel = path.relative_to(ROOT).as_posix()
        if rel not in ignored:
            actual.add(rel)
    require(actual == set(files), 'incomplete-input-inventory')
    return lock, files


def verify(files, stop):
    values = {}
    for rel in sorted(files):
        require(time.monotonic() <= stop, 'producer-time-bound')
        item = files[rel]
        raw = read_exact(ROOT / rel, MAX_INPUT)
        require(len(raw) == item['bytes'] and digest(raw) == item['sha256'], 'input-identity:' + rel)
        values[rel] = raw
    require(digest(values['inputs/selection/ISSUE_RECOVERY_CONTRACT.md']) == CONTRACT, 'contract-copy')
    require(digest(values['inputs/selection/REFERENCE_PLAN.md']) == PLAN, 'plan-copy')
    catalog = document(values['input-catalog.json'])
    require(catalog['base_commit'] == BASE_COMMIT, 'catalog-base-commit')
    require(len(catalog['files']) == len({x['path'] for x in catalog['files']}), 'catalog-duplicate')
    require({x['path'] for x in catalog['files']} == {k for k in files if k.startswith('inputs/')}, 'catalog-completeness')
    for item in catalog['files']:
        raw = values[item['path']]
        require(item['bytes'] == len(raw) and item['sha256'] == digest(raw), 'catalog-identity')
    return values


def produce(values):
    # Local producer leaves are imported only after every locked input is read
    # and checked. Documentary inherited Python remains plain pinned bytes.
    modules = {name: importlib.import_module(name) for name in MODULES}
    for name, module in modules.items():
        require(Path(module.__file__).resolve() == ROOT / (name + '.py'), 'foreign-producer-module')
    inherited = document(values['inputs/set79/reference-v1.json'])
    manifest = document(values['inputs/manifest-job/manifest_job_v1.json'])
    name_v2_reference = document(values['inputs/name-v2/independent/reference-v2.json'])
    recipe = document(values['recipe.json'])
    require(recipe['base_commit'] == BASE_COMMIT, 'recipe-commit')
    fixtures = modules['fixtures'].produce_fixtures(inherited, manifest, recipe, name_v2_reference)
    states = modules['states'].produce_states(fixtures['matching_families'])
    outbox = modules['outbox'].produce_outbox(fixtures['matching_families'], inherited)
    lifecycle = modules['lifecycle'].produce_lifecycle(fixtures['matching_families'])
    wire = modules['wire'].produce_wire(fixtures, inherited, recipe)
    framing = modules['framing'].produce_framing(fixtures, states, wire)
    bounds = modules['bounds'].produce_bounds(fixtures)
    counts = dict(profiles=len(fixtures['profiles']), profile_cases=len(fixtures['profile_cases']),
                  wrapper_cases=len(fixtures['wrapper_cases']), wrappers=len(fixtures['wrappers']),
                  mode_boundaries=len(fixtures['mode_boundaries']), matching_families=len(fixtures['matching_families']),
                  conditional_transport=len(fixtures['conditional_transport']), remote=len(fixtures['remote']),
                  inherited_anchors=len(fixtures['inherited_anchors']), state_pairs=len(states['state_pairs']),
                  state_refusals=len(states['state_refusals']), response_cases=len(states['response_cases']),
                  outbox_cases=len(outbox['cases']), outbox_request_rows=len(outbox['request_rows']),
                  inherited_entry_anchors=len(outbox['inherited_entry_anchors']), lifecycle_traces=len(lifecycle['traces']),
                  authority_refusals=len(lifecycle['authority_refusals']), wire_responses=len(wire['response_cases']),
                  wire_errors=len(wire['error_cases']), framing_targets=len(framing['targets']), raw_cases=len(framing['cases']),
                  source_bounds=len(bounds['source']), registry_bounds=len(bounds['registry']), count_bounds=len(bounds['counts']),
                  outbox_bounds=len(bounds['outbox']), checked_arithmetic=len(bounds['checked_arithmetic']))
    require(counts == recipe['expected_case_counts'], 'declared-case-counts')
    output = dict(version=1, base_commit=BASE_COMMIT, contract_sha256=CONTRACT, plan_sha256=PLAN,
                  scope='Independent conditional byte/state/capacity model. No product imports, live crypto/PoW, network, storage sync, lease, signing or payout authority.',
                  source_provenance=document(values['input-catalog.json']), counts=counts,
                  fixtures=fixtures, states=states, outbox=outbox, lifecycle=lifecycle, wire=wire, framing=framing, bounds=bounds,
                  exclusions=['No invented idempotency in older modes', 'No migration or existing V2 unidentified-hold adoption',
                              'No new payout arithmetic, fee/minimum/finality/daily-due policy',
                              'No runtime mutex/process/crash qualification', 'No inferred continuous or public-host readiness'])
    raw = modules['core'].canon(output) + b'\n'
    require(0 < len(raw) <= MAX_OUTPUT, 'output-byte-bound')
    return raw, counts


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--lock', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    lock_path, output_path = Path(args.lock), Path(args.output)
    require(lock_path.is_absolute() and output_path.is_absolute(), 'absolute-paths-required')
    require(output_path.parent.resolve() == output_path.parent and output_path.parent.is_dir(), 'output-parent')
    require(ROOT not in output_path.parents and not output_path.exists() and not output_path.is_symlink(), 'fresh-external-output-required')
    lock_raw = read_exact(lock_path, 1 << 20)
    lock, files = admission(lock_path, lock_raw)
    stop = time.monotonic() + lock['limits']['seconds']
    counts = None
    try:
        values = verify(files, stop)
        raw, counts = produce(values)
        require(time.monotonic() <= stop, 'producer-time-bound')
        # Bind exact input bytes before creation as well as in final cleanup.
        verify(files, stop)
        require(read_exact(lock_path, 1 << 20) == lock_raw, 'lock-changed')
        fd = os.open(str(output_path), os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, 'O_NOFOLLOW', 0), 0o600)
        try:
            offset = 0
            while offset < len(raw):
                count = os.write(fd, raw[offset:offset + 65536])
                require(count > 0, 'short-output-write')
                offset += count
            os.fsync(fd)
        finally:
            os.close(fd)
        require(read_exact(output_path, MAX_OUTPUT) == raw, 'output-readback')
    finally:
        # Recheck even if a modeled case or output write fails. A partial output
        # is retained and never reported qualified; root also brackets inputs.
        verify(files, float('inf'))
        require(read_exact(lock_path, 1 << 20) == lock_raw, 'lock-changed-at-exit')
        admission(lock_path, lock_raw)
    print(json.dumps(dict(status='PASS', bytes=len(raw), sha256=digest(raw), counts=counts), separators=(',', ':')))


if __name__ == '__main__':
    main()
