#!/usr/bin/env python3
"""Independent public fixture composition; no Go, node, real key or signature.

This program is frozen before execution. Scalar1..64 are deliberately published
test identities. It produces no accepted receipt, mined proof or payment txid.
"""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import runpy
import struct


def canonical(v):
    return json.dumps(v, separators=(',', ':'), ensure_ascii=True).encode('ascii')


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def framed(domain, v):
    raw = canonical(v)
    assert len(raw) <= 65536
    return digest(domain + struct.pack('<I', len(raw)) + raw)


def native_shape(n, payees):
    def compact_size(v):
        assert 0 <= v <= 65535
        return 1 if v < 253 else 3
    outputs = payees + 1
    stripped = 4 + compact_size(n) + 41*n + compact_size(outputs) + 31*outputs + 4
    witness = 2 + 109*n
    weight = 4*stripped + witness
    assert 1 <= n <= 1464 and 1 <= payees <= 256 and weight <= 400000
    return {'stripped_bytes': stripped, 'witness_bytes': witness,
            'raw_bytes': stripped+witness, 'weight': weight, 'vsize': (weight+3)//4}


def main():
    p = argparse.ArgumentParser()
    p.add_argument('--inputs', required=True)
    p.add_argument('--recipe', required=True)
    p.add_argument('--out', required=True)
    args = p.parse_args()
    directory = Path(args.inputs)
    recipe = json.loads(Path(args.recipe).read_bytes())
    assert recipe['version'] == 1
    for row in recipe['frozen_inputs']:
        name = Path(row['path']).name
        raw = (directory/name).read_bytes()
        assert len(raw) == row['bytes'] and digest(raw) == row['sha256'], name
    inherited = json.loads((directory/'setup-v1.json').read_bytes())['setup']
    wallet = json.loads((directory/'wallet-daily-v1.json').read_bytes())
    zero = next(v['binding'] for v in wallet['positives'] if v['fixture'] == 'zero256')
    for key in ('root_public_key', 'funding_public_key', 'funding_script', 'derivation_id'):
        assert inherited['wallet_guard'][key] == zero['guard'][key]
    crypto = runpy.run_path(str(directory/'crypto.py'))
    gym = runpy.run_path(str(directory/'gym-reference.py'))
    spec = recipe['accounts']
    assert (spec['count'],spec['public_scalar_start'],spec['public_scalar_end']) == (64,1,64)
    source = copy.deepcopy(inherited['pool'])
    source['version'] = 2
    source.update(recipe['source_maxima'])
    source_id = digest(recipe['source_domain_ascii'].encode('ascii') + canonical(source))
    accounts = []
    for scalar in range(1,65):
        pub = crypto['pubkey'](scalar)
        assert pub == gym['public_key'](scalar)
        identity = digest(spec['gym_domain_ascii'].encode('ascii') + struct.pack('<I',source['policy']['network']) + bytes.fromhex(source['policy']['genesis']) + pub)
        assert identity == gym['gym_id'](source['policy'],pub)
        script = (b'\x00\x14' + crypto['hash160'](pub)).hex()
        token = digest(spec['public_token_domain_ascii'].encode('ascii')+struct.pack('<I',scalar))
        accounts.append({'index':scalar-1, 'published_scalar':scalar, 'public_key':pub.hex(),
                         'gym_id':identity, 'payee_script':script, 'public_fixture_token':token})
    assert len({a['gym_id'] for a in accounts}) == 64
    assert len({a['payee_script'] for a in accounts}) == 64
    assert all(a['payee_script'] != inherited['wallet_guard']['funding_script'] for a in accounts)
    config = copy.deepcopy(inherited['settlement'])
    config['version'] = 2
    config['destinations'] = [{'gym_id':a['gym_id'],'script':a['payee_script']} for a in sorted(accounts,key=lambda a:a['gym_id'])]
    config.update(recipe['settlement_maxima'])
    config_id = framed(recipe['config_domain_ascii'].encode('ascii'),config)
    guard = copy.deepcopy(inherited['wallet_guard'])
    guard['source_binding_id'] = source_id
    guard['settlement_config_id'] = config_id
    binding = {'version':2,'wallet_owner_id':guard['wallet_owner_id'],'operator_profile_id':guard['operator_profile_id'],
               'source':source,'settlement':config,'guard':guard}
    counts,econ = recipe['counts'],recipe['economics']
    assert counts['first_rewards']==1 and counts['second_rewards']==720 and counts['all_rewards']==721
    a_shape,b_shape = native_shape(1,1),native_shape(721,64)
    prior_shapes=json.loads((directory/'native84-v1.json').read_bytes())['cases']
    for n,payees,expected in ((1,1,a_shape),(721,64,b_shape)):
        matches=[v for v in prior_shapes if v['inputs']==n and v['payees']==payees and v['change']]
        assert len(matches)==1 and matches[0]['result']=='ok' and matches[0]['dimensions']==expected
    rate=econ['fee_rate_atoms_per_kvb']
    a_fee=(rate*a_shape['vsize']+999)//1000
    b_fee=(rate*b_shape['vsize']+999)//1000
    miner_per_reward=econ['reward_atoms']*econ['miner_numerator']//econ['miner_denominator']
    assert miner_per_reward*econ['miner_denominator']==econ['reward_atoms']*econ['miner_numerator']
    operator_per_reward=econ['reward_atoms']-miner_per_reward
    a_change=operator_per_reward-a_fee
    b_change=a_change+720*operator_per_reward-b_fee
    payments=[]
    for a in accounts:
        new_rewards=sum(1 for i in range(720) if i%64==a['index'])
        first_rewards=int(a['index']==0)
        payments.append({'gym_id':a['gym_id'],'script':a['payee_script'],'first_rewards':first_rewards,
                         'second_rewards':new_rewards,'first_atoms':first_rewards*miner_per_reward,
                         'second_atoms':new_rewards*miner_per_reward,'total_atoms':(first_rewards+new_rewards)*miner_per_reward})
    payments.sort(key=lambda v:v['gym_id'])
    total=sum(v['total_atoms'] for v in payments)
    assert (a_fee,b_fee,a_change,b_change,total)==(econ['first_fee_atoms'],econ['second_fee_atoms'],econ['first_change_atoms'],econ['second_change_atoms'],econ['total_miner_atoms'])
    node=copy.deepcopy(inherited['node'])
    assert node['fee']['mode']=='fixed' and node['fee']['atoms_per_kvb']==rate and b_fee<=node['fee']['max_fee_atoms']
    result={'version':1,'scope':recipe['scope'],'authority':inherited['authority'],'binding':binding,
            'round_limits':recipe['round_limits'],'node':node,'source_id':source_id,'config_id':config_id,
            'guard_hex':canonical(guard).hex(),'guard_digest':framed(recipe['guard_domain_ascii'].encode('ascii'),guard),
            'binding_hex':canonical(binding).hex(),'accounts':accounts,'counts':counts,'days':recipe['days'],
            'first_shape':a_shape,'second_shape':b_shape,'economics':econ,'payments':payments}
    raw=json.dumps(result,indent=2,ensure_ascii=True).encode('ascii')+b'\n'
    assert len(raw)<1<<20
    with open(args.out,'xb') as f:f.write(raw)


if __name__=='__main__':
    main()
