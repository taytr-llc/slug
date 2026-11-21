#!/usr/bin/env python3
"""Independent, public-only actual fixture derivation. No import-time effects."""
import argparse
import hashlib
import json
import os
from pathlib import Path
from core import canon, sha, clone, require, ident, source_id, settlement_id, scheduler_id, profile_id, daily_trust, manifest_trust

HERE = Path(__file__).resolve().parent

def bounded(path, limit):
    raw = path.read_bytes()
    require(0 < len(raw) <= limit, 'input bound')
    return raw

def selected_json(name, limit=4<<20):
    return json.loads(bounded(HERE/name, limit))

def verify_sources():
    lock = selected_json('source-ready.json', 1<<20)
    for row in lock['files']:
        raw = bounded(HERE/row['path'], 4<<20)
        require(len(raw) == row['bytes'] and sha(raw) == row['sha256'], 'frozen source input changed')
    return lock

def shape(inputs):
    # Native empty scriptSig: 36-byte outpoint +1 length +4 sequence.
    # One native payee + native change: each 8+1+22 bytes.
    # Two stack elements + one signature of at most73 bytes + compressed33-byte key.
    stripped = 4+1+41*inputs+1+31*2+4
    witness = 2+109*inputs
    weight = 4*stripped+witness
    return dict(stripped_bytes=stripped,witness_bytes=witness,raw_bytes=stripped+witness,weight=weight,vsize=(weight+3)//4)

def produce():
    recipe = selected_json('recipe.json', 65536)
    corpus = selected_json('inputs/public-daily.json')
    local = selected_json('inputs/fixture-public.json', 65536)
    row = next(x for x in corpus['composition']['positives'] if x['name']==recipe['selected_composition'])
    require(ident('SLUG pool operator setup','v2',row['setup'])==row['setup_id'], 'inherited exact setup')
    require(local['fixture']==recipe['selected_fixture'] and local['expected_local']['recipient']['config']['fixture']==recipe['selected_fixture'], 'selected existing unheld wallet')
    s = clone(row['setup'])
    require(s['pool']['version']==2 and s['pool']['policy']==local['expected_local']['recipient']['config']['source']['policy'], 'original local policy')
    require(s['pool']['policy']['prospect_target']==recipe['expected_prospect_target'], 'inherited prospect target')
    require(s['pool']['policy']['network']==recipe['expected_network'] and s['pool']['policy']['genesis']==recipe['expected_genesis'], 'original normal chain')
    for k,v in recipe['source_limits'].items(): require(s['pool'][k]==v, 'fixed source limit')
    require(s['round_limits']==recipe['round_limits'], 'fixed round limits')
    for k,v in recipe['settlement_limits'].items(): require(s['settlement'][k]==v, 'fixed financial limit')
    # Bitcoin compact positive target: 23-bit coefficient *256^(exponent-3).
    bits = int(recipe['bits_hex'],16); exponent = bits>>24; coefficient = bits&0x7fffff
    require(bits&0x800000==0 and exponent==32 and coefficient==0x7fffff, 'selected compact target')
    target = coefficient << (8*(exponent-3))
    require(0 < target < (1<<255), 'normal compact target')
    s['pool']['pool_target']=target.to_bytes(32,'big').hex()
    original_local = clone(local['expected_local'])
    recipients={r['config']['identity']['gym_id']:r for r in local['recipients']}
    require(len(recipients)==2 and len(s['miners']['enrollments'])==2, 'exact two configured gyms')
    for en,dest in zip(s['miners']['enrollments'],s['settlement']['destinations']):
        r=recipients[en['gym_id']]
        require(en['gym_id']==dest['gym_id'] and en['public_key']==r['config']['identity']['gym_public_key'], 'original gym identity')
        en['payee_script']=r['recipient']['script'];dest['script']=en['payee_script']
        require(en['payee_script']!=s['settlement']['change_script'], 'separate operator funding')
    sid=source_id(s['pool']);cid=settlement_id(s['settlement']);pid=profile_id(s['authority'])
    s['wallet_guard'].update(source_binding_id=sid,settlement_config_id=cid)
    s['miners'].update(source_id=sid,settlement_config_id=cid)
    s['schedule']['settlement_binding_id']=scheduler_id(s['settlement'],s['pool'])
    require(s['wallet_guard']['operator_profile_id']==pid and s['miners']['operator_profile_id']==pid, 'unchanged exact profile')
    setup_id=ident('SLUG pool operator setup','v2',s)
    base_remote=clone(corpus['transport']['remote_positives'][0]['config'])
    base_remote.update(connection_id=sha(recipe['connection_recipe'].encode('ascii')),operator_pin=s['authority'],source=s['pool'],settlement=s['settlement'],miners=s['miners'],expected_local=original_local,**recipe['outbox_limits'])
    remote_id=ident('SLUG remote pool attachment','v2',base_remote)
    key=original_local['recipient']['config']['identity']['gym_public_key']
    client=dict(version=2,url=base_remote['origin'],token='97'*32,ca_certificate=base_remote['ca_certificate'],public_key=key,source=s['pool'],settlement_config_id=cid,recipient_script=original_local['recipient']['recipient']['script'])
    _, trust=daily_trust(client);_, admission=manifest_trust(base_remote['origin'],base_remote['ca_certificate'])
    worker=dict(version=2,source=s['pool'],public_key=key,authority_origin=base_remote['origin'],transport_trust_id=trust,admission_origin=base_remote['origin'],admission_trust_id=admission,**recipe['outbox_limits'])
    expect=[]
    require(s['node']['fee']['mode']=='fixed' and s['node']['fee']['atoms_per_kvb']==recipe['atoms_per_kvb'], 'fixed real fee policy')
    for n in recipe['reward_counts']:
        require(n in (1,2), 'finite Stop race')
        sizes=shape(n);gross=n*recipe['subsidy_atoms'];payment=gross*recipe['miner_numerator']//recipe['allocation_denominator'];fee=(sizes['vsize']*recipe['atoms_per_kvb']+999)//1000
        expect.append(dict(accepted_rewards=n,credits=n,batches=1,input_count=n,positive_payees=1,output_count=2,gross_atoms=gross,miner_atoms=payment,operator_gross_atoms=gross-payment,fee_atoms=fee,change_atoms=gross-payment-fee,other_gym_atoms=0,shape=sizes))
    require(base_remote['expected_local']==local['expected_local'], 'local binding never repinned')
    return dict(version=1,scope=recipe['scope'],base_composition_setup_id=row['setup_id'],base_reference_sha256=sha(bounded(HERE/'inputs/public-daily.json',4<<20)),fixture=local,setup=s,setup_hex=canon(s).hex(),setup_id=setup_id,source_id=sid,settlement_config_id=cid,settlement_binding_id=s['schedule']['settlement_binding_id'],operator_profile_id=pid,wallet_guard_id=ident('SLUG operator pool wallet guard','v1',s['wallet_guard']),miner_config_id=ident('SLUG pool miner enrollment','v2',s['miners']),remote=base_remote,remote_id=remote_id,remote_hex=canon(base_remote).hex(),worker=worker,worker_hex=canon(worker).hex(),block_target=s['pool']['pool_target'],compact_bits=recipe['bits_hex'],ordinary_maximum=recipe['ordinary_maximum'],economics=expect,exclusions=['No real keys','No block/proof outputs','No product Go expected output','No original wallet/source/marker mutation','No claim dynamic TLS/time template is a precomputed actual identity'])

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--output',required=True);args=ap.parse_args()
    lock=verify_sources();value=produce();verify_sources()
    raw=json.dumps(value,ensure_ascii=True,separators=(',',':')).encode('ascii')+b'\n'
    require(len(raw)<=262144,'finite output')
    path=Path(args.output);require(path.is_absolute() and path.parent.is_dir() and not path.exists(),'fresh absolute output')
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
    print(json.dumps(dict(bytes=len(raw),sha256=hashlib.sha256(raw).hexdigest(),source_ready_sha256=sha((HERE/'source-ready.json').read_bytes()))))

if __name__=='__main__':main()
