#!/usr/bin/env python3
"""Gated Set66 independent producer; refuses before root execution approval.

No expected bytes are generated merely by importing this module or asking for
help. The lock must bind these scripts, recipes, contract, original public
inputs and repository-relative independent dependencies before normal work.
"""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import sys
import time
import traceback

sys.dont_write_bytecode = True
from evidence_schema import CONTRACT_SHA256, Refusal, canonical, need
from reference_evidence import Reference
import cases

HERE = Path(__file__).resolve().parent


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def pretty(value):
    return (json.dumps(value, ensure_ascii=True, indent=2, allow_nan=False) + '\n').encode('ascii')


def verify_lock(path):
    lock_raw = path.read_bytes()
    lock = json.loads(lock_raw)
    need(lock['version'] == 1 and lock['execution_authorized_by_root'] is True)
    need(lock['contract_sha256'] == CONTRACT_SHA256)
    for relative, value in lock['files'].items():
        path = HERE / relative
        need(path.resolve().is_relative_to(HERE))
        raw = path.read_bytes()
        need(len(raw) == value['bytes'] and sha(raw) == value['sha256'])
    need(json.loads((HERE / 'case-plan.json').read_bytes()) == cases.plan())
    return lock, sha(lock_raw)


def actual_inputs():
    manifest = json.loads((HERE / 'actual-input-manifest.json').read_bytes())
    base = HERE / 'actual-inputs'
    for relative, row in manifest['files'].items():
        raw = (base / relative).read_bytes()
        need(len(raw) == row['bytes'] and sha(raw) == row['sha256'])
    independent = json.loads((base / 'provenance/independent-expected-inputs.json').read_bytes())
    selected = {group['Name']: group for group in independent['expected']}
    output = {}
    for name in cases.ACTUAL_NAMES:
        version, fixture_role = name.split('/')
        fixture = fixture_role.split('-')[0]
        path = base / name
        fields = {key: json.loads((path / (key + '.json')).read_bytes())
                  for key in ('admission', 'capture', 'rows', 'expected-pin')}
        expected = fields['expected-pin']
        # Expected trust predates envelope assembly, with its own independently
        # retained marker/accepted-entry provenance, not capture-derived trust.
        need(expected == selected[version]['Pins'][fixture])
        for key, value in fields.items():
            need((path / (key + '.json')).read_bytes() == canonical(value))
        output[name] = (expected, cases.construct(fields['admission'], fields['capture'], fields['rows']))
    return output


class CountedWork:
    def __init__(self, primitive):
        self.primitive, self.calls = primitive, 0

    def hash(self, header):
        self.calls += 1
        return self.primitive.hash(header)


def negative(reference, oracle, name, expected, raw, calls_expected):
    before = oracle.calls
    try:
        reference.verify(expected, raw, oracle)
    except (Refusal, ValueError, AssertionError) as error:
        need(oracle.calls - before == calls_expected)
        return dict(name=name, expected='reject', work_calls=calls_expected,
                    expected_pin=expected, raw=raw.decode('ascii'), file_bytes=len(raw),
                    file_sha256=sha(raw), refusal_category=getattr(error, 'code', 'invalid_evidence'))
    raise AssertionError('negative unexpectedly accepted: ' + name)


def generate(repo_root, lock_hash):
    reference = Reference(repo_root)
    actual = actual_inputs()
    positive_cases = cases.positives(reference, actual)
    result = dict(version=1, contract_sha256=CONTRACT_SHA256, execution_lock_sha256=lock_hash,
                  scope='Conditional supplied-evidence consistency; actual public source inputs and explicitly modeled histories, no admission-time/award/completeness/ownership authentication',
                  dependency_manifest_sha256=sha((HERE/'dependency-manifest.json').read_bytes()),
                  actual_input_manifest_sha256=sha((HERE/'actual-input-manifest.json').read_bytes()),
                  case_plan_sha256=sha((HERE/'case-plan.json').read_bytes()), positives=[], negatives=[])
    with reference.admission.Argon() as primitive:
        oracle = CountedWork(primitive)
        for name, (expected, archive, provenance) in positive_cases.items():
            raw = canonical(archive)
            before = oracle.calls
            report = reference.verify(expected, raw, oracle)
            need(oracle.calls == before + 1)
            result['positives'].append(dict(name=name, provenance=provenance,
                                            expected_pin=expected, canonical=raw.decode('ascii'),
                                            report=report))
        expected, base = actual['v2/zero256-unawarded']
        for name in cases.RAW_CONTROLS:
            raw = cases.raw_mutation(name, base, positive_cases)
            result['negatives'].append(negative(reference, oracle, 'raw/'+name, expected, raw, 0))
        for name in cases.SHAPE_CONTROLS:
            expected, raw = cases.shape_mutation(name, reference, actual, positive_cases)
            result['negatives'].append(negative(reference, oracle, 'shape/'+name, expected, raw, 0))
        for version in ('v1', 'v2'):
            expected, raw = cases.work_mismatch(reference, actual, version)
            result['negatives'].append(negative(reference, oracle, 'work/'+version+'/claimed_zero_hash', expected, raw, 1))
        need(len(result['positives']) == cases.plan()['expected_positive_count'])
        need(len(result['negatives']) == cases.plan()['expected_negative_count'])
        need(len(primitive.hashes) == 6)
        result['normal_work'] = dict(unique_headers_recomputed=len(primitive.hashes), logical_calls=oracle.calls,
                                     implementation='Unchanged standalone x/crypto/argon2.IDKey; imports no SLUG package; literal production parameters; downloads disabled',
                                     exact_header_results=[primitive.hashes[k] for k in sorted(primitive.hashes)],
                                     signature_implementation='Unchanged archived independent Python secp256k1 and canonical low-S DER validation',
                                     supplied_verified_hashes_trusted=False)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo-root', type=Path)
    parser.add_argument('--lock', type=Path, required=True)
    parser.add_argument('--attempt', type=Path, required=True)
    parser.add_argument('--check', type=Path, help='Compare regenerated exact corpus bytes; never replace the expected file')
    args = parser.parse_args()
    # Preserve every attempt and fail before generating if its name is reused.
    args.attempt.mkdir(parents=True, exist_ok=False)
    start = time.monotonic()
    source = args.attempt/'source';source.mkdir()
    for name in ('evidence_schema.py','reference_evidence.py','cases.py','generate_reference.py',
                 'case-plan.json','contract.md','dependency-manifest.json','actual-input-manifest.json'):
        (source/name).write_bytes((HERE/name).read_bytes())
    (args.attempt/'execution-lock.json').write_bytes(args.lock.read_bytes())
    try:
        _, lock_hash = verify_lock(args.lock)
        result = generate(args.repo_root, lock_hash)
        raw = pretty(result)
        (args.attempt/'reference-v1.json').write_bytes(raw)
        if args.check is not None:
            need(args.check.read_bytes() == raw)
        verify_lock(args.lock)
        report = dict(passed=True, seconds=time.monotonic()-start, bytes=len(raw), sha256=sha(raw),
                      positives=len(result['positives']), negatives=len(result['negatives']),
                      normal_work_unique_headers=result['normal_work']['unique_headers_recomputed'],
                      execution_lock_sha256=lock_hash, python=sys.version,
                      scope=result['scope'])
        (args.attempt/'report.json').write_bytes(pretty(report))
        print(json.dumps(report))
    except BaseException as error:
        (args.attempt/'failure.txt').write_text(traceback.format_exc())
        (args.attempt/'report.json').write_bytes(pretty(dict(passed=False,seconds=time.monotonic()-start,
                                                           error_type=type(error).__name__,error=str(error))))
        raise


if __name__ == '__main__':
    main()
