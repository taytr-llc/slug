#!/usr/bin/env python3
"""Guarded source-only until root approves exact inputs. No product imports."""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import struct
import sys
import time

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
SOURCES = {'authority_model.py', 'generate_reference.py', 'case-plan.json', 'README.md'}


def need(value, message):
    if not value:
        raise RuntimeError(message)


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def input_check(root, catalog):
    need(root.is_dir() and not root.is_symlink(), 'input root')
    expected = {}
    for row in catalog['files']:
        rel = Path(row['path'])
        need(not rel.is_absolute() and '..' not in rel.parts and row['path'] not in expected,
             'input path')
        path = root / rel
        need(path.is_file() and not path.is_symlink() and
             path.stat().st_size == row['bytes'] and sha(path) == row['sha256'], 'input hash')
        expected[row['path']] = row['sha256']
    need(not any(p.is_symlink() for p in root.rglob('*')), 'input links')
    need({str(p.relative_to(root)) for p in root.rglob('*') if p.is_file()} == set(expected),
         'input membership')
    return expected


def produce(A, B, E, root, recipe, plan):
    started = time.monotonic()
    def bounded():
        need(time.monotonic()-started < 120, 'reference CPU deadline')
    E.selftest()  # Unchanged Set45 RFC8032/secp known-answer expectations.
    legacy = json.loads((root/'legacy/public-core-v1.json').read_bytes())
    source_inputs = []
    for version in ('v1', 'v2'):
        legacy_name = version+'/'+recipe['inherited_fixture_names'][version]
        record = json.loads((root/'actual'/version/'admission.json').read_bytes())
        expected = json.loads((root/'actual'/version/'expected-pin.json').read_bytes())
        need(type(record['sequence']) is int and record['sequence'] == 1, 'inherited sequence one')
        need(B.J(record) == (root/'actual'/version/'admission.json').read_bytes().rstrip(b'\n'),
             'original nested record bytes')
        source_inputs.append((version, expected['source'], record, legacy_name))
    model = A.Model(B, E, recipe, [x[1] for x in source_inputs])
    need(legacy['literals']['public_log_key'] == model.excluded['ownership_log'] and
         legacy['literals']['checkpoint_key'] == model.excluded['seed_checkpoint'] and
         legacy['literals']['rule_id'] == model.rule, 'inherited independent literals')
    positives, refusals, raw_cases, guards, matrix = [], [], [], [], []
    fixtures = {}

    def positive(family, name, value, ids=None):
        bounded()
        row = dict(family=family, name=name, value=copy.deepcopy(value),
                   canonical_hex=B.J(value).hex(), canonical_sha256=B.sha(B.J(value)))
        if ids is not None:
            row['ids'] = ids
        positives.append(row)

    def refuse(name, family, pin, selected, value, check):
        bounded()
        try:
            check()
        except B.Refused as error:
            refusals.append(dict(name=name, family=family, expected_pin=copy.deepcopy(pin),
                                 selected_profile=selected, value=copy.deepcopy(value),
                                 expected='refuse', reason=str(error)))
        else:
            raise RuntimeError('negative accepted: '+name)

    for descriptor, source, record, legacy_name in source_inputs:
        for label in recipe['profile_labels']:
            bounded()
            pin = model.pin(source, label)
            version, pid = pin['version'], model.pin_id(pin, pin['version'])
            name = descriptor+'/'+label
            positive('pin', name, pin, dict(pin_id=pid))
            body = dict(version=1, pin_id=pid, admission_sequence=1,
                        previous_admission_id=B.ZERO,
                        record_digest=B.digest('SLUG fighter ownership admission record', record).hex(),
                        record=copy.deepcopy(record))
            admission = model.sign(pin, 'admission', body)
            aid = model.verify(pin, version, 'admission', admission)
            positive('admission', name, admission, dict(admission_id=aid))
            if version == 1:
                for family, value in (('pin', pin), ('admission', admission)):
                    old = next(x for x in legacy['positives'] if x['family'] == family and
                               x['name'] == legacy_name)
                    need(B.J(value).hex() == old['canonical_hex'], 'exact inherited v1 '+family)
            height, block_hash = 100, '51'*32
            cp = dict(version=1, network=source['network'], genesis=source['genesis'],
                      height=height, hash=block_hash, signature='')
            cp_preimage = (b'SLUG signed checkpoint\0'+struct.pack('<II', 1, source['network'])+
                           bytes.fromhex(source['genesis'])[::-1]+struct.pack('<I', height)+
                           bytes.fromhex(block_hash)[::-1])
            cp['signature'] = E.ed_sign(model.seed_for(pin, 'checkpoint'), cp_preimage).hex()
            need(E.ed_verify(bytes.fromhex(pin['checkpoint_key']), cp_preimage,
                             bytes.fromhex(cp['signature'])), 'checkpoint role verification')
            positive('checkpoint', name, cp, dict(signing_bytes_hex=cp_preimage.hex()))
            pubs, previous = [], B.ZERO
            tip = dict(height=recipe['modeled_chain_height'], hash=('%02x'%recipe['modeled_chain_hash_byte'])*32)
            payloads = [('admission', dict(admission_sequence=1, admission_id=aid)),
                        ('checkpoint', dict(checkpoint=cp)),
                        ('chain', dict(height=tip['height'], hash=tip['hash'], previous_chain_entry_id=B.ZERO))]
            for n, (kind, payload) in enumerate(payloads, 1):
                body = dict(version=1, pin_id=pid, sequence=n, previous_id=previous,
                            attested_at_ms=recipe['base_ms']+n, kind=kind, payload=payload)
                pub = model.sign(pin, 'publication', body)
                previous = model.verify(pin, version, 'publication', pub)
                pubs.append(pub)
                positive('publication', name+'/'+kind, pub, dict(publication_id=previous))
            birth = B.Model.initial(model, pid, admission, record['authorization']['public_key'])
            # Existing independent conditional model supplies a recipient point;
            # neither this transition nor its modeled chain is new chain proof.
            old_final = next(x['value'] for x in legacy['positives'] if x['family'] == 'state' and
                             x['name'] == legacy_name+'/final')
            cp_id = model.verify(pin, version, 'publication', pubs[1])
            state = B.Model.transferred(model, pid, birth, '61'*32, cp_id, tip,
                                        old_final['owner_public_key'])
            owner_map, map_id, prefix = B.Model.ownership_map(model, pid, 1, state)
            replay = dict(pin_id=pid, admission_sequence=1, admission_head=aid,
                          publication_sequence=3, publication_head=previous,
                          chain_height=tip['height'], chain_hash=tip['hash'],
                          ownership_map_id=map_id, finalized_prefix_id=prefix, conflict_id='')
            head = dict(version=1, pin_id=pid, admission_sequence=1, admission_head=aid,
                        publication_sequence=3, publication_head=previous,
                        chain_height=tip['height'], chain_hash=tip['hash'],
                        replay_digest=B.digest('SLUG fighter ownership replay', replay).hex(),
                        finalized_prefix_id=prefix, conflict_id='')
            fb = dict(version=1, pin_id=pid, sequence=1, previous_body_id=B.ZERO,
                      admission_sequence=1, admission_head=aid, publication_sequence=3,
                      publication_head=previous, chain_height=tip['height'], chain_hash=tip['hash'],
                      ownership_map_id=map_id, finalized_prefix_id=prefix)
            frame = model.sign(pin, 'frame', fb)
            frame_id = model.verify(pin, version, 'frame', frame)
            positive('frame', name, frame, dict(frame_id=frame_id,
                     body_id=B.digest('SLUG fighter ownership finality body', fb).hex()))
            noon = recipe['base_ms']+1000
            first = dict(request_start_ms=noon-2, response_end_ms=noon-1,
                         instance_id=('%02x'%recipe['instance_id_byte'])*32,
                         revision=recipe['revision'], height=tip['height'], hash=tip['hash'])
            second = copy.deepcopy(first)
            second.update(request_start_ms=noon, response_end_ms=noon+1)
            intent = dict(version=1, pin_id=pid, event_id=('%02x'%recipe['event_id_byte'])*32,
                          cutoff_ms=noon, first=first, second=second, accepted_admission_sequence=1,
                          publication_sequence=3, publication_head=previous, finality_sequence=1,
                          finality_body_head=B.digest('SLUG fighter ownership finality body', fb).hex(),
                          prior_cutoff_id=B.ZERO)
            boundary = model.sign(pin, 'boundary', dict(version=1, pin_id=pid, intent=intent))
            positive('boundary', name, boundary,
                     dict(boundary_id=model.verify(pin, version, 'boundary', boundary)))
            cutoff_body = dict(version=1, pin_id=pid, event_id=intent['event_id'], cutoff_ms=noon,
                               boundary=boundary, admission_sequence=1, admission_head=aid,
                               head=head, ownership_digest=map_id)
            cutoff = model.sign(pin, 'cutoff', cutoff_body)
            positive('cutoff', name, cutoff,
                     dict(cutoff_id=model.verify(pin, version, 'cutoff', cutoff)))
            fixtures[name] = dict(pin=pin, admission=admission, publication=pubs[0],
                                  boundary=boundary, cutoff=cutoff, frame=frame,
                                  checkpoint=cp, checkpoint_preimage=cp_preimage.hex(),
                                  conditional_state=state, conditional_map=owner_map)
            guard = dict(application='slug-manifest-ownership', version=version, pin=pin,
                         authority_id=source['authority_id'], publication_id=pin['publication_id'])
            for reader in (1, 2):
                guards.append(dict(name=name+'/reader'+str(reader), expected_profile=reader,
                     filename='SLUG-MANIFEST-OWNERSHIP-V%d.json'%version,
                     value=guard, accepted=reader == version,
                     scope='profile/guard codec only; not filesystem-open or mutation proof'))

    # Cross-profile: both wrong parser selection and valid signing under a
    # different profile fail at the independently supplied expected source.
    for descriptor in ('v1', 'v2'):
        old = fixtures[descriptor+'/fixture_v1']
        new = fixtures[descriptor+'/operator_v2_distinct']
        for expected, foreign in ((old, new), (new, old)):
            pin, foreign_pin = expected['pin'], foreign['pin']
            name = descriptor+'/profile%d-rejects%d'%(pin['version'], foreign_pin['version'])
            refuse(name+'/pin', 'pin', pin, pin['version'], foreign_pin,
                   lambda p=foreign_pin, v=pin['version']: model.pin_id(p, v))
            for family in A.DOMAINS:
                value = foreign[family]
                refuse(name+'/'+family, family, pin, pin['version'], value,
                       lambda p=pin, f=family, x=value: model.verify(p, p['version'], f, x))
        for family in A.DOMAINS:
            pin = new['pin']; value = copy.deepcopy(new[family]); body = copy.deepcopy(value); body.pop('signature')
            right_role = A.DOMAINS[family][0]
            wrong_role = next(r for r in A.ROLES if r != right_role)
            bad = model.sign(pin, family, body, wrong_role)
            refuse(descriptor+'/'+family+'/wrong-role', family, pin, 2, bad,
                   lambda f=family, x=bad, p=pin: model.verify(p, 2, f, x))
            body['version'] = True
            bad = model.sign(pin, family, body)
            refuse(descriptor+'/'+family+'/boolean-version', family, pin, 2, bad,
                   lambda f=family, x=bad, p=pin: model.verify(p, 2, f, x))
            body['version'] = 2
            bad = model.sign(pin, family, body)
            refuse(descriptor+'/'+family+'/statement-version-stays-one', family, pin, 2, bad,
                   lambda f=family, x=bad, p=pin: model.verify(p, 2, f, x))
        for role in A.ROLES:
            for fixture_name, key in model.excluded.items():
                bad = copy.deepcopy(new['pin']); bad[role+'_key'] = key
                refuse(descriptor+'/'+role+'/excludes-'+fixture_name, 'pin', new['pin'], 2, bad,
                       lambda x=bad: model.pin_id(x, 2))
        for value in (0, 3, True):
            bad = copy.deepcopy(new['pin']); bad['version'] = value
            refuse(descriptor+'/pin-version-'+str(value), 'pin', new['pin'], 2, bad,
                   lambda x=bad: model.pin_id(x, 2))
        # Same actual signing key does not eliminate the pin/profile boundary.
        shared = fixtures[descriptor+'/operator_v2_shared']
        value = shared['admission']
        refuse(descriptor+'/same-admission-key-foreign-pin', 'admission', new['pin'], 2, value,
               lambda p=new['pin'], x=value: model.verify(p, 2, 'admission', x))
        for expected, foreign in ((old, new), (new, old)):
            valid = E.ed_verify(bytes.fromhex(expected['pin']['checkpoint_key']),
                                bytes.fromhex(foreign['checkpoint_preimage']),
                                bytes.fromhex(foreign['checkpoint']['signature']))
            need(not valid, 'foreign checkpoint role accepted')
            matrix.append(dict(name=descriptor+'/checkpoint-profile%d-rejects%d'%(
                          expected['pin']['version'], foreign['pin']['version']),
                          expected_key=expected['pin']['checkpoint_key'],
                          checkpoint=foreign['checkpoint'], valid=False))
        raw = B.J(new['pin'])
        variants = [('canonical', raw, True), ('newline', raw+b'\n', False),
                    ('duplicate-version', raw.replace(b'"version":2', b'"version":2,"version":2', 1), False),
                    ('boolean-version', raw.replace(b'"version":2', b'"version":true', 1), False),
                    ('float-version', raw.replace(b'"version":2', b'"version":2.0', 1), False),
                    ('unknown-member', raw[:-1]+b',"operator":true}', False)]
        for label, raw_value, wanted in variants:
            try:
                model.parse_pin(raw_value, 2)
                actual = True
            except B.Refused:
                actual = False
            need(actual == wanted, 'raw case '+label)
            raw_cases.append(dict(name=descriptor+'/'+label, selected_profile=2,
                                  raw_hex=raw_value.hex(), valid=wanted))
    counts = dict(positives=len(positives), refusals=len(refusals), raw_cases=len(raw_cases),
                  guards=len(guards), checkpoint_matrix=len(matrix))
    need(counts == plan['expected_counts'], 'exact case inventory')
    bounded()
    return dict(version=1, scope=plan['scope'],
                literals=dict(rule_id=model.rule, known_fixture_keys=model.excluded,
                              synthetic_role_keys=model.keys),
                positives=positives, refusals=refusals, raw_cases=raw_cases,
                guards=guards, checkpoint_matrix=matrix,
                conditional_compositions=fixtures, counts=counts)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--execute', action='store_true')
    parser.add_argument('--approved-lock', type=Path)
    parser.add_argument('--approved-lock-sha256')
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    if not args.execute:
        print('Prepared source only; generation requires root-approved exact lock.')
        return
    need(args.approved_lock is not None and args.output is not None, 'explicit lock/output')
    need(not args.approved_lock.is_symlink() and sha(args.approved_lock) == args.approved_lock_sha256,
         'approved lock hash')
    lock = json.loads(args.approved_lock.read_bytes())
    need(lock.get('approved_by') == 'root' and lock.get('execution_authorized') is True, 'approval')
    catalog_path, root = HERE/'input-catalog.json', HERE/'inputs'
    need(sha(catalog_path) == lock['input_catalog_sha256'], 'input catalog hash')
    need(set(lock['sources']) == SOURCES, 'source inventory')
    before = {name: sha(HERE/name) for name in SOURCES}
    need(before == lock['sources'], 'source hashes')
    catalog = json.loads(catalog_path.read_bytes())
    need(catalog['public_only'] is True, 'public-only inputs')
    inputs_before = input_check(root, catalog)
    output = args.output.resolve()
    need(output != HERE and HERE not in output.parents, 'output outside producer/input tree')
    output.mkdir(parents=True, exist_ok=False)
    try:
        B = load('set73_independent_core', root/'inherited/core_model.py')
        E = load('set45_independent_crypto', root/'inherited/crypto.py')
        A = load('set74_independent_authority', HERE/'authority_model.py')
        recipe = json.loads((root/'recipe.json').read_bytes())
        plan = json.loads((HERE/'case-plan.json').read_bytes())
        result = produce(A, B, E, root, recipe, plan)
        result['provenance'] = dict(input_catalog_sha256=sha(catalog_path),
                                    producer_lock_sha256=args.approved_lock_sha256)
        raw = B.J(result)+b'\n'
        need(len(raw) <= recipe['output_max_bytes'], 'output byte bound')
        need(before == {name:sha(HERE/name) for name in SOURCES}, 'source unchanged')
        need(inputs_before == input_check(root, catalog), 'inputs unchanged')
        need(sha(catalog_path) == lock['input_catalog_sha256'] and
             sha(args.approved_lock) == args.approved_lock_sha256, 'catalog/lock unchanged')
        with (output/'authority-v2.json').open('xb') as out:
            out.write(raw)
        report = dict(status='passed', source_unchanged=True, inputs_unchanged=True,
                      bytes=len(raw), sha256=hashlib.sha256(raw).hexdigest(), counts=result['counts'],
                      normal_work_calls=0, product_output_calls=0, network_calls=0)
    except BaseException as error:
        report = dict(status='failed', error_type=type(error).__name__, error=str(error))
        with (output/'report.json').open('x') as out:
            json.dump(report, out, sort_keys=True, indent=2); out.write('\n')
        raise
    with (output/'report.json').open('x') as out:
        json.dump(report, out, sort_keys=True, indent=2); out.write('\n')
    print(json.dumps(report, sort_keys=True))


if __name__ == '__main__':
    main()
