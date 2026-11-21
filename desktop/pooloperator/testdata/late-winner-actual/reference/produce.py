"""Independent byte-only public composition; no product imports or key access."""
from pathlib import Path
import copy
import hashlib
import json
import struct
import sys


def raw(value):
    return json.dumps(value, separators=(',', ':'), ensure_ascii=True).encode()


def framed(label, value):
    body = raw(value)
    return hashlib.sha256(label.encode() + b'\0v1\0' + struct.pack('<I', len(body)) + body).hexdigest()


def build(root):
    recipe = json.loads((root / 'recipe.json').read_bytes())
    source_bytes = (root / recipe['source']).read_bytes()
    assert hashlib.sha256(source_bytes).hexdigest() == recipe['source_sha256']
    source = json.loads(source_bytes)
    assert len(source['positives']) == 3 and recipe['source_positive_index'] == 0
    original = source['positives'][0]
    assert raw(original['setup']).hex() == original['canonical_hex']
    assert framed('SLUG pool operator setup', original['setup']) == original['setup_id']
    setup = copy.deepcopy(original['setup'])
    for field, value in recipe['changes'].items():
        group, name = field.split('.')
        setup[group][name] = value
    profile = framed('SLUG fighter ownership pin', setup['authority'])
    source_id = hashlib.sha256(b'SLUG pool service source\0v1\0' + raw(setup['pool'])).hexdigest()
    config_id = framed('SLUG operator pool settlement config', setup['settlement'])
    setup['wallet_guard'].update(operator_profile_id=profile, source_binding_id=source_id, settlement_config_id=config_id)
    setup['miners'].update(operator_profile_id=profile, source_id=source_id, settlement_config_id=config_id)
    setup['schedule']['settlement_binding_id'] = framed('SLUG pool settlement binding', dict(config=setup['settlement'], source=setup['pool']))
    assert setup['authority'] == original['setup']['authority']
    assert setup['settlement']['destinations'] == original['setup']['settlement']['destinations']
    assert setup['settlement']['change_script'] == original['setup']['settlement']['change_script']
    assert setup['pool']['policy'] == original['setup']['pool']['policy']
    assert setup['pool']['pool_target'] == original['setup']['pool']['pool_target']
    assert setup['pool']['max_works'] <= setup['round_limits']['max_rounds'] == setup['settlement']['max_rounds']
    assert setup['pool']['max_submissions'] > recipe['expected_scenario']['ordinary_intake']
    assert min(setup['pool']['max_submissions'], setup['round_limits']['max_rounds'], setup['settlement']['max_rounds'], (setup['settlement']['max_batches'] + 1) // 2) == 4
    assert 2 * recipe['expected_scenario']['late_block_rows'] - 1 <= setup['settlement']['max_batches']
    sid = framed('SLUG pool operator setup', setup)
    return dict(version=1, scope=recipe['scope'], setup=setup, canonical_hex=raw(setup).hex(), setup_id=sid,
                intent_hex=(raw(setup) + b'\n').hex(),
                ready_hex=(raw(dict(version=1, runtime_id=setup['runtime_id'], setup_id=sid)) + b'\n').hex(),
                miner_config_id=framed('SLUG pool miner enrollment', setup['miners']),
                expected_scenario=recipe['expected_scenario'],
                reservation_scope='row/batch scalar crosschecks only; full N=5/R=3 byte bound requires independent two-axis-v1 reference')


if __name__ == '__main__':
    print(json.dumps(build(Path(__file__).resolve().parent), indent=2))
