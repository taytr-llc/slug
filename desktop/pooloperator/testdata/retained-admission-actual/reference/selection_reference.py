#!/usr/bin/env python3
"""Independent Set81 selection derivation. Root alone executes an approved lock.

Only standard-library imports. No product/old-producer import, subprocess,
network, runtime fixture, private key or expected output read.
"""
import argparse
import copy
import hashlib
import json
import os
from pathlib import Path
import stat
import struct

HERE = Path(__file__).resolve().parent
OLD = '0f0890e529b64c10ed000673aab7861b9bd526057cc3c2bbbe05b868028f3d10'
ATTACHMENT = 'a48a854d20298fced726b816d11690aa45e9f4184e3b161cbc7c258b74897386'
MAX_INPUT = 1 << 20


def need(ok, message):
    if not ok:
        raise ValueError(message)


def canonical(value):
    return json.dumps(value, ensure_ascii=True, allow_nan=False,
                      separators=(',', ':')).encode('ascii')


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def frame(label, body, version='v1'):
    need(type(body) is bytes and len(body) <= 0xffffffff, 'frame size')
    return (label.encode('ascii') + b'\0' + version.encode('ascii') + b'\0'
            + struct.pack('<I', len(body)) + body)


def identity(label, value, version='v1'):
    return digest(frame(label, canonical(value), version))


def parsed(raw):
    def pairs(items):
        out = {}
        for key, value in items:
            need(key not in out, 'duplicate JSON key')
            out[key] = value
        return out
    def invalid(_):
        raise ValueError('noninteger JSON number')
    return json.loads(raw.decode('ascii'), object_pairs_hook=pairs,
                      parse_float=invalid, parse_constant=invalid)


def read_public(path, cap):
    need(path.is_file() and not path.is_symlink(), 'regular public input')
    size = path.stat().st_size
    need(0 < size <= cap, 'input size')
    raw = path.read_bytes()
    need(len(raw) == size, 'input changed while reading')
    return raw


def input_value(name, expected):
    raw = read_public(HERE / 'inputs' / name, MAX_INPUT)
    need(digest(raw) == expected, 'input identity: ' + name)
    return parsed(raw)


def differences(before, after, prefix=''):
    need(type(before) is type(after), 'unchanged types')
    if isinstance(before, dict):
        need(list(before) == list(after), 'unchanged ordered fields')
        return [p for key in before for p in differences(
            before[key], after[key], prefix + ('.' if prefix else '') + key)]
    if isinstance(before, list):
        need(len(before) == len(after), 'unchanged list size')
        return [p for i, (a, b) in enumerate(zip(before, after))
                for p in differences(a, b, prefix + '[' + str(i) + ']')]
    return [] if before == after else [prefix]


def selected(base):
    """Restate the published frame grammar; never execute the old producer."""
    s, c, worker = base['setup'], base['remote'], base['worker']
    sid = digest(b'SLUG pool share daily source\0v2\0' + canonical(s['pool']))
    need(canonical(s['pool']) == canonical(c['source']) == canonical(worker['source']),
         'same original full source')
    need(sid == base['source_id'] == s['wallet_guard']['source_binding_id'],
         'unchanged original source ID')
    need(canonical(s['settlement']) == canonical(c['settlement']), 'same settlement')
    profile = dict(version=1, base_setup_id=base['setup_id'],
                   source_config_id=sid, max_requests=s['pool']['max_works'])
    pid = identity('SLUG pool issue recovery profile', profile)
    setup = dict(version=1, base=s, issue_profile=profile)
    remote = dict(version=1, base=c, issue_profile=profile)
    setup_id = identity('SLUG pool issue recovery setup', setup)
    remote_id = identity('SLUG pool issue recovery attachment', remote)
    trust = frame('SLUG pool issue recovery transport',
                  (worker['transport_trust_id'] + pid).encode('ascii'))
    wrapped_worker = dict(version=1, base=worker, issue_profile=profile,
                          settlement_config_id=s['miners']['settlement_config_id'],
                          recipient_script=c['expected_local']['recipient']['recipient']['script'],
                          transport_trust_id=digest(trust))
    frames = []
    for name, value, label in [
        ('profile', profile, 'SLUG pool issue recovery profile'),
        ('setup', setup, 'SLUG pool issue recovery setup'),
        ('remote', remote, 'SLUG pool issue recovery attachment')
    ]:
        raw = canonical(value)
        frames.append(dict(name=name, value=value, bytes=len(raw),
                           canonical_hex=raw.hex(), id=identity(label, value),
                           preimage_hex=frame(label, raw).hex()))
    intent = dict(version=1, application='slug-remote-pool-issue-recovery',
                  config_id=remote_id, config=remote)
    ready = dict(version=1, application=intent['application'], config_id=remote_id,
                 intent_sha256=digest(canonical(intent) + b'\n'))
    operator_ready = dict(version=1, runtime_id=s['runtime_id'], setup_id=setup_id,
                          issue_profile_id=pid)
    for name, value in [('operator-intent', setup), ('operator-ready', operator_ready),
                        ('remote-intent', intent), ('remote-ready', ready)]:
        raw = canonical(value) + b'\n'
        frames.append(dict(name=name, value=value, bytes=len(raw),
                           canonical_hex=raw.hex(), framing='exact-final-lf'))
    for audience, token in [('pool', '4' * 64), ('admission', '5' * 64)]:
        inner = dict(version=2, config_id=base['remote_id'],
                     gym_id=c['expected_local']['recipient']['config']['identity']['gym_id'],
                     audience=audience, generation=1,
                     expires_at_ms=1794790800000, token=token)
        value = dict(version=1, base=inner, issue_profile_id=pid)
        raw = canonical(value) + b'\n'
        frames.append(dict(name='synthetic-credential-' + audience, value=value,
                           bytes=len(raw), canonical_hex=raw.hex(), framing='exact-final-lf',
                           scope='published shape only; scenario enrolls original session separately'))
    need(len(frames) == 9, 'nine original frames')
    return dict(profile=profile, profile_id=pid, setup=setup, setup_id=setup_id,
                remote=remote, remote_id=remote_id, worker=wrapped_worker,
                transport_trust_id=wrapped_worker['transport_trust_id'],
                trust_preimage_hex=trust.hex(), frames=frames)


def economic_row(recipe):
    e = recipe['economics']
    need(e['accepted_rewards'] == e['native_inputs'] == 2 and
         e['positive_payees'] == e['native_change_outputs'] == 1,
         'two rewards and complete two-input payout')
    # n=2 and two native22-byte output scripts each have one-byte lengths.
    # Input: outpoint36 + empty-script length1 + sequence4.
    # Witness: marker/flag2 + each stack count1/signature length1/max73/key length1/key33.
    n = e['native_inputs']
    stripped = 4 + 1 + n * (36 + 1 + 4) + 1 + 2 * (8 + 1 + 22) + 4
    witness = 2 + n * (1 + 1 + 73 + 1 + 33)
    weight = 4 * stripped + witness
    vsize = (weight + 3) // 4
    gross = e['accepted_rewards'] * e['subsidy_atoms_per_reward']
    numerator = gross * e['miner_numerator']
    need(numerator % e['allocation_denominator'] == 0, 'exact miner integer')
    payment = numerator // e['allocation_denominator']
    fee = (vsize * e['atoms_per_kvb'] + 999) // 1000
    change = gross - payment - fee
    need((stripped, witness, weight, vsize) == (154, 220, 836, 209), 'hand shape anchor')
    need((payment, fee, change) == (e['expected_payment_atoms'], e['expected_fee_atoms'],
                                  e['expected_change_atoms']), 'hand economic anchors')
    return dict(accepted_rewards=2, credits=2, batches=1, input_count=2,
                positive_payees=1, output_count=2, gross_atoms=gross,
                miner_atoms=payment, operator_gross_atoms=gross-payment,
                fee_atoms=fee, change_atoms=change, other_gym_atoms=0,
                shape=dict(stripped_bytes=stripped, witness_bytes=witness,
                           raw_bytes=stripped+witness, weight=weight, vsize=vsize))


def build():
    recipe = parsed(read_public(HERE / 'selection-recipe.json', 16384))
    old = input_value('actual-reference.json', OLD)
    attachment = input_value('path-c-reference.json', ATTACHMENT)
    need(canonical(old['base_actual']) == canonical(attachment), 'qualified base attachment')
    need(canonical(old['fixture']) == canonical(attachment['fixture']), 'qualified fixture')
    need(old['ordinary_maximum'] == attachment['ordinary_maximum'] == 2, 'original true cap2')
    need(canonical(old['economics']) == canonical(attachment['economics']), 'original economics')
    need(attachment['setup_hex'] == canonical(attachment['setup']).hex() and
         identity('SLUG pool operator setup', attachment['setup'], 'v2') == attachment['setup_id'],
         'original base setup frame')
    need(attachment['remote_hex'] == canonical(attachment['remote']).hex() and
         identity('SLUG remote pool attachment', attachment['remote'], 'v2') == attachment['remote_id'],
         'original base remote frame')
    need(attachment['worker_hex'] == canonical(attachment['worker']).hex(), 'original worker frame')
    inherited = selected(attachment)
    for key, value in inherited.items():
        need(canonical(old[key]) == canonical(value), 'original selected identity/frame: ' + key)

    change = recipe['semantic_change']
    need(change['path'] == 'setup.base.maintenance_interval_ms' and
         attachment['setup']['maintenance_interval_ms'] == change['before'] == 30000 and
         change['existing_minimum'] == change['after'] == 1000 and
         change['existing_maximum'] == 60000, 'one existing supported selection')
    base = copy.deepcopy(attachment)
    base['setup']['maintenance_interval_ms'] = change['after']
    need(differences(attachment['setup'], base['setup']) == ['maintenance_interval_ms'],
         'no other semantic setup mutation')
    base['setup_hex'] = canonical(base['setup']).hex()
    base['setup_id'] = identity('SLUG pool operator setup', base['setup'], 'v2')
    need(differences(attachment, base) == ['setup.maintenance_interval_ms', 'setup_hex', 'setup_id'],
         'only changed base copies and derived serialization/ID')
    value = selected(base)
    for key in ['profile_id', 'setup_id', 'remote_id', 'transport_trust_id']:
        need(value[key] != old[key], 'required enclosing identity changed: ' + key)
    need(value['profile']['max_requests'] == old['profile']['max_requests'] == 8,
         'unchanged finite request budget')

    target = recipe['unchanged_target']
    bits = int(target['compact_bits'], 16)
    exponent, coefficient = bits >> 24, bits & 0x7fffff
    need(bits == 0x207fffff and bits & 0x800000 == 0 and exponent == 32,
         'unchanged positive normal target')
    decoded = (coefficient << (8 * (exponent-3))).to_bytes(32, 'big').hex()
    need(decoded == target['hex'] == base['block_target'] and
         target['path'] == 'setup.base.pool.pool_target' and
         target['required_accepted_rewards'] == 2 and
         target['one_reward_is_not_a_passing_substitute'] is True,
         'second canonical pool reward is mandatory')
    for source in [base['setup']['pool'], base['remote']['source'], base['worker']['source']]:
        need(source['pool_target'] == decoded and
             source['policy']['prospect_target'] == target['prospect_target'] == 'f' * 64,
             'all embedded unchanged targets')
    econ = economic_row(recipe)
    prior_two = [row for row in attachment['economics'] if row['accepted_rewards'] == 2]
    need(len(prior_two) == 1 and canonical(econ) == canonical(prior_two[0]),
         'independent two-reward arithmetic equals inherited row')
    need(base['setup']['node']['fee']['mode'] == 'fixed' and
         base['setup']['node']['fee']['atoms_per_kvb'] == recipe['economics']['atoms_per_kvb'],
         'original fixed fee policy')
    return dict(version=1, scope=recipe['scope'],
                inputs=dict(original_selection=OLD, original_attachment=ATTACHMENT),
                fixture=base['fixture'], base_actual=base, **value,
                economics=base['economics'], selected_economics=econ, ordinary_maximum=2,
                required_accepted_rewards=2, target_assertion=target,
                semantic_change=change, runtime_normalization=recipe['runtime_normalization'],
                exclusions=recipe['exclusions'])


def verify_lock(path):
    raw = read_public(path, MAX_INPUT)
    lock = parsed(raw)
    need(lock.get('version') == 1 and lock.get('execution_authorized') is True and
         lock.get('approved_by') == 'root', 'explicit root execution admission')
    before = {}
    need(type(lock.get('files')) is list and 1 <= len(lock['files']) <= 32, 'bounded lock inventory')
    for row in lock['files']:
        relative = Path(row['path'])
        need(not relative.is_absolute() and '..' not in relative.parts and
             str(relative) == row['path'] and row['path'] not in before, 'unique relative lock path')
        for i in range(1, len(relative.parts)+1):
            need(not (HERE / Path(*relative.parts[:i])).is_symlink(), 'no symlink input')
        content = read_public(HERE / relative, MAX_INPUT)
        need(len(content) == row['bytes'] and digest(content) == row['sha256'], 'locked file identity')
        before[row['path']] = content
    required = {'selection_reference.py', 'selection-recipe.json', 'input-catalog.json',
                'inputs/actual-reference.json', 'inputs/path-c-reference.json',
                'inputs/RETAINED_ADMISSION_ACTUAL_PLAN.md', 'inputs/implementation-selection.json'}
    need(required.issubset(before), 'all actual derivation inputs locked')
    catalog = parsed(before['input-catalog.json'])
    for entry in catalog['files']:
        need(entry['path'] in before and len(before[entry['path']]) == entry['bytes'] and
             digest(before[entry['path']]) == entry['sha256'], 'complete catalog identities')
    return raw, before


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--lock', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    lock_path = Path(args.lock)
    output = Path(args.output)
    need(lock_path.is_absolute() and output.is_absolute() and output.parent.is_dir() and
         not output.exists() and not output.is_symlink(), 'fresh absolute output and lock')
    need(HERE not in output.resolve().parents and output.resolve() != HERE, 'external output')
    lock_raw, before = verify_lock(lock_path)
    def unchanged():
        for name, original in before.items():
            need(read_public(HERE / name, MAX_INPUT) == original, 'source changed')
        need(read_public(lock_path, MAX_INPUT) == lock_raw, 'approved lock changed')
    try:
        raw = canonical(build()) + b'\n'
        need(len(raw) <= 1 << 20, 'output maximum1MiB')
        unchanged()
        flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
        if hasattr(os, 'O_NOFOLLOW'):
            flags |= os.O_NOFOLLOW
        descriptor = os.open(output, flags, 0o600)
        with os.fdopen(descriptor, 'wb') as stream:
            need(stat.S_ISREG(os.fstat(stream.fileno()).st_mode), 'regular output')
            need(stream.write(raw) == len(raw), 'complete output')
            stream.flush()
            os.fsync(stream.fileno())
        unchanged()
        print(json.dumps(dict(passed=True, bytes=len(raw), sha256=digest(raw),
                              frames=9, required_accepted_rewards=2)))
    finally:
        unchanged()


if __name__ == '__main__':
    main()
