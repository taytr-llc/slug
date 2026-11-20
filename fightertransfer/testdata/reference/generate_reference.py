#!/usr/bin/env python3
"""Source-only until exact root-approved input/producer lock authorizes execution."""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import sys

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
SOURCE_NAMES = {'carrier_model.py', 'generate_reference.py', 'case-plan.json', 'README.md'}


def need(v, message):
    if not v:
        raise RuntimeError(message)


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def canonical(v):
    return json.dumps(v, ensure_ascii=False, separators=(',', ':'), allow_nan=False).encode()


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    m = importlib.util.module_from_spec(spec)
    sys.modules[name] = m
    spec.loader.exec_module(m)
    return m


def main():
    p = argparse.ArgumentParser()
    p.add_argument('--execute', action='store_true')
    p.add_argument('--input-root', type=Path, default=HERE/'inputs')
    p.add_argument('--input-catalog', type=Path, default=HERE/'input-catalog.json')
    p.add_argument('--approved-producer-lock', type=Path)
    p.add_argument('--approved-lock-sha256')
    p.add_argument('--output', type=Path)
    args = p.parse_args()
    if not args.execute:
        print('Source prepared only; no model imports or expected-output generation.')
        return
    need(args.approved_producer_lock is not None and args.output is not None,
         'explicit lock/output required')
    need(sha(args.approved_producer_lock) == args.approved_lock_sha256, 'lock hash')
    lock = json.loads(args.approved_producer_lock.read_bytes())
    need(lock.get('approved_by') == 'root' and lock.get('execution_authorized') is True, 'approval')
    need(lock['input_catalog_sha256'] == sha(args.input_catalog), 'input catalog')
    need(set(lock['sources']) == SOURCE_NAMES, 'exact producer source set')
    before = {n: sha(HERE/n) for n in SOURCE_NAMES}
    need(before == lock['sources'], 'source binding')
    catalog = json.loads(args.input_catalog.read_bytes())
    expected = {}
    for row in catalog['files']:
        rel = Path(row['path']); need(not rel.is_absolute() and '..' not in rel.parts, 'input path')
        path = args.input_root/rel
        need(path.is_file() and not path.is_symlink(), 'input type')
        need(path.stat().st_size == row['bytes'] and sha(path) == row['sha256'], 'input bytes')
        need(row['path'] not in expected, 'duplicate input')
        expected[row['path']] = row['sha256']
    actual = {str(x.relative_to(args.input_root)) for x in args.input_root.rglob('*') if x.is_file()}
    need(actual == set(expected), 'exact input set')
    args.output.mkdir(mode=0o700, parents=True, exist_ok=False)
    # No derivation, signing or old module import occurs before approved guards.
    M = load('set73_independent_carrier', HERE/'carrier_model.py')
    R = load('set73_old_recovery', args.input_root/'inherited/recovery/reference.py')
    E = load('set73_old_ecdsa', args.input_root/'inherited/ecdsa/reference_authorizations.py')
    published = json.loads((args.input_root/'published-recovery-v1.json').read_bytes())
    recipe = json.loads((args.input_root/'recipe.json').read_bytes())
    model = M.Model(R,E,published,recipe)
    positives = []
    for fixture in recipe['fixtures']:
        for count in (1,16):
            positives.append(model.make(fixture,count))
    for variant in ('refund','nonminimal_outer','high_s_outer','extra_stack','non_rfc_gym'):
        positives.append(model.make('zero256',variant=variant))
    positives.append(model.make('zero256',payer='incrementing256',suffix='third-party-payer'))
    positives.append(model.make('zero256',suffix='different-inputs-competing-intent'))
    positives.append(model.make('zero256',variant='large_unrelated_parent_output'))
    for row in positives:
        result=model.validate(row)
        need(result['transfer_id']==row['transfer_id'] and result['is_transfer']==row['is_transfer'], 'positive semantic expectation')
        need(result['intent_bytes']==129 and result['envelope_bytes']==202 and result['redeem_bytes']==240, 'fixed byte sizes')
        row['validated']=result
        row['canonical_sha256']=hashlib.sha256(canonical({k:v for k,v in row.items() if k!='canonical_sha256'})).hexdigest()
    base=positives[0]
    same=[r for r in positives if r['name'].startswith('zero256-1-') and r['name'].endswith('-base') and '-large_unrelated_parent_output-' not in r['name']]
    need(len({r['transfer_id'] for r in same})==1, 'semantic identity ignores outer/gym signature representation and refund form')
    rfc=model.sign_gym('zero256',bytes.fromhex(base['intent']))
    need(rfc.hex()==base['envelope'][-128:], 'deterministic signature reproduction')
    alternative=next(r for r in positives if '-non_rfc_gym-' in r['name'])
    need(alternative['envelope'][-128:]!=rfc.hex(), 'modeled alternative signature differs')
    need(alternative['redeem']!=base['redeem'], 'nonconforming commitment cannot be reconstructed by RFC writer')
    negatives=[]

    def evidence(row):
        return {key: row[key] for key in ('expected_pin','expected_prior','sender_gym',
                'prevouts','funding_transactions','commit_raw','spend_raw')}

    def refuse(name,row):
        try:
            model.validate(row)
        except M.Rejected as e:
            negatives.append(dict(name=name,expected='refuse',reason=str(e),value=evidence(row)))
        else:
            raise RuntimeError('negative accepted: '+name)

    for name,field,value in [('foreign_pin','expected_pin',recipe['alternate_pin_id']),
                             ('foreign_prior','expected_prior',recipe['alternate_prior_state_id']),
                             ('foreign_gym','sender_gym',base['recipient_gym'])]:
        row=copy.deepcopy(base);row[field]=value;refuse(name,row)
    # Re-sign funding legs after semantic mutations; no mere signature mismatch
    # stands in for the intended template or compact-gym-signature refusal.
    mutations=['commit_version','commit_locktime','commit_sequence','commit_fee','commit_change_dust',
               'commit_locator','return_program','reveal_version','reveal_locktime','reveal_sequence',
               'reveal_return','reveal_locator','extra_spend_output','gym_r_zero','gym_r_order',
               'gym_s_zero','gym_s_order','gym_s_high','zero_pin','zero_prior','self_recipient',
               'bad_recipient_point','funding_digest','envelope_tag','first_funding_key']
    for name in mutations:
        row=copy.deepcopy(base);c=M.parse_tx(bytes.fromhex(row['commit_raw']));s=M.parse_tx(bytes.fromhex(row['spend_raw']))
        redeem=bytearray(M.pushes(s['inputs'][0]['script'])[-1])
        if name=='commit_version':c['version']=2
        elif name=='commit_locktime':c['locktime']=1
        elif name=='commit_sequence':c['inputs'][0]['sequence']=0
        elif name=='commit_fee':c['outputs'][2]['atoms']-=1
        elif name=='commit_change_dust':c['outputs'][2]['atoms']=999
        elif name=='commit_locator':c['outputs'][1]['script']=c['outputs'][1]['script'][:-1]+bytes([c['outputs'][1]['script'][-1]^1])
        elif name=='return_program':c['outputs'][2]['script']=b'\x51\x20'+bytes(32)
        elif name=='reveal_version':s['version']=2
        elif name=='reveal_locktime':s['locktime']=1
        elif name=='reveal_sequence':s['inputs'][0]['sequence']=0
        elif name=='reveal_return':s['outputs'][0]['atoms']-=1
        elif name=='reveal_locator':s['outputs'][1]['script']=s['outputs'][1]['script'][:-1]+bytes([s['outputs'][1]['script'][-1]^1])
        elif name=='extra_spend_output':s['outputs'].append(dict(atoms=0,script=b'\x6a\x00'))
        elif name=='gym_r_zero':redeem[140:172]=bytes(32)
        elif name=='gym_r_order':redeem[140:172]=E.ORDER.to_bytes(32,'big')
        elif name=='gym_s_zero':redeem[172:204]=bytes(32)
        elif name=='gym_s_order':redeem[172:204]=E.ORDER.to_bytes(32,'big')
        elif name=='gym_s_high':redeem[172:204]=(E.ORDER-int.from_bytes(redeem[172:204],'big')).to_bytes(32,'big')
        elif name=='zero_pin':redeem[11:43]=bytes(32);row['expected_pin']='00'*32
        elif name=='zero_prior':redeem[43:75]=bytes(32);row['expected_prior']='00'*32
        elif name=='self_recipient':redeem[75:108]=bytes.fromhex(row['sender_gym'])
        elif name=='bad_recipient_point':redeem[75:108]=b'\x02'+bytes.fromhex('ff'*32)
        elif name=='funding_digest':redeem[108]^=1
        elif name=='envelope_tag':redeem[2]^=1
        elif name=='first_funding_key':redeem[206:239]=bytes.fromhex(row['recipient_gym'])
        if bytes(redeem)!=M.pushes(s['inputs'][0]['script'])[-1]:
            c['outputs'][0]['script']=b'\xa9\x14'+M.h160(bytes(redeem))+b'\x87'
        model.resign(row,c,s,bytes(redeem));refuse(name,row)
    for name,raw in [('appended',bytes.fromhex(base['spend_raw'])+b'\x00'),
                     ('truncated',bytes.fromhex(base['spend_raw'])[:-1])]:
        row=copy.deepcopy(base);row['spend_raw']=raw.hex();refuse(name,row)
    row=copy.deepcopy(base)
    parent=M.parse_tx(bytes.fromhex(row['funding_transactions'][0]),previous=True)
    parent['outputs'][0]['atoms']+=1
    row['funding_transactions'][0]=M.serialize(parent).hex()
    changed_hash=M.D(M.serialize(parent,False)).hex()
    row['prevouts'][0]['hash_wire']=changed_hash
    c=M.parse_tx(bytes.fromhex(row['commit_raw']));s=M.parse_tx(bytes.fromhex(row['spend_raw']))
    c['inputs'][0]['hash_wire']=changed_hash
    # Retain the old amount in the independent claimed facts/signature input.
    # Actual full-parent bytes now disagree; Go's evidence-only API rejects the
    # fixed fee or BIP143 signature without trusting row.prevouts metadata.
    model.resign(row,c,s)
    refuse('parent_selected_output_mismatch',row)
    unavailable=[]
    for name in ('missing_parent','wrong_parent_hash'):
        row=copy.deepcopy(base)
        if name=='missing_parent':row['funding_transactions']=[]
        else:
            parent=M.parse_tx(bytes.fromhex(row['funding_transactions'][0]),previous=True)
            parent['locktime']=1
            row['funding_transactions'][0]=M.serialize(parent).hex()
        try:model.validate(row)
        except M.Unavailable as e:unavailable.append(dict(name=name,expected='unavailable',reason=str(e),value=evidence(row)))
        else:raise RuntimeError('unavailable evidence accepted '+name)
    tuple_cases=[]
    for name,coins in [('zero_count',[]),('seventeen',copy.deepcopy(positives[1]['prevouts'])+[copy.deepcopy(base['prevouts'][0])]),
                       ('duplicate',[copy.deepcopy(base['prevouts'][0])]*2),
                       ('unsorted',list(reversed(copy.deepcopy(positives[1]['prevouts']))))]:
        try:model.funding_tuple(coins)
        except M.Rejected as e:tuple_cases.append(dict(name=name,expected='refuse',reason=str(e),prevouts=coins))
        else:raise RuntimeError('tuple negative accepted '+name)
    report=dict(version=1,notice='Independent mathematical carrier vectors; full modeled parent transactions and Pin/State only. Parent TxIDs/selected outputs are checked, not parent input signatures or chain existence. No actual chain/admission/finality, consensus engine, restore, clock or custody claim.',
                input_catalog_sha256=sha(args.input_catalog),producer_lock_sha256=args.approved_lock_sha256,
                positives=positives,negatives=negatives,unavailable=unavailable,tuple_refusals=tuple_cases,
                expected_counts=dict(positives=12,negatives=31,unavailable=2,tuple_refusals=4))
    need(len(positives)==12 and len(negatives)==31 and len(unavailable)==2 and len(tuple_cases)==4, 'finite case plan counts')
    for name,digest in expected.items():need(sha(args.input_root/name)==digest,'input changed')
    need({n:sha(HERE/n) for n in SOURCE_NAMES}==before,'producer changed')
    raw=canonical(report)+b'\n';(args.output/'carrier-v1.json').write_bytes(raw)
    (args.output/'report.json').write_text(json.dumps(dict(passed=True,source_unchanged=True,input_unchanged=True,
        source_hashes=before,output_sha256=hashlib.sha256(raw).hexdigest(),output_bytes=len(raw),
        counts=report['expected_counts'],python=sys.version),indent=2)+'\n')
    print(json.dumps(dict(passed=True,output=str(args.output/'carrier-v1.json'),sha256=hashlib.sha256(raw).hexdigest(),counts=report['expected_counts'])))


if __name__=='__main__':
    main()
