#!/usr/bin/env python3
"""Explicit reviewed-lock entrypoint. Do not import/execute before approval."""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import sys
sys.dont_write_bytecode = True

SOURCE_NAMES={'downstream_model.py','generate_reference.py','sizing_model.py','case-plan.json','README.md'}
sha=lambda b:hashlib.sha256(b).hexdigest()
pretty=lambda v:(json.dumps(v,ensure_ascii=False,indent=2,allow_nan=False)+'\n').encode('utf-8')


def load_unique(raw):
    def pairs(items):
        out={}
        for k,v in items:
            if k in out:raise ValueError('duplicate key')
            out[k]=v
        return out
    return json.loads(raw,object_pairs_hook=pairs,parse_constant=lambda _:(_ for _ in ()).throw(ValueError('constant')))


def module(name,path):
    spec=importlib.util.spec_from_file_location(name,path)
    value=importlib.util.module_from_spec(spec);sys.modules[name]=value;spec.loader.exec_module(value);return value


def checked_inputs(root,catalog):
    result={}
    for row in catalog['inputs']:
        path=Path(row['path'])
        if path.is_absolute() or '..' in path.parts:raise ValueError('input path')
        p=root/path
        if p.is_symlink() or not p.is_file():raise ValueError('input file')
        b=p.read_bytes()
        if sha(b)!=row['sha256'] or len(b)!=row['bytes']:raise ValueError('input changed: '+str(path))
        result[str(path)]=sha(b)
    found={str(p.relative_to(root))for p in root.rglob('*')if p.is_file()}
    if found!=set(result):raise ValueError('input inventory is not exact')
    return result


def generate(M,C,read):
    recipe=read('recipe.json');up=read('upstream/owned-consumers-v1.json')
    e=up['contexts'][recipe['upstream_context']]['pin']
    cases=[x for x in up['positives']if x['family']=='snapshot' and x['name']==recipe['upstream_snapshot']]
    if len(cases)!=1:raise ValueError('one original independent snapshot')
    original=cases[0]
    if M.canonical(original['value']).hex()!=original['canonical_hex'] or M.sha(M.canonical(original['value']))!=original['sha256']:raise ValueError('upstream bytes')
    snapshot=copy.deepcopy(original['value'])
    oldseed=read('old-public/seed-pin.json');oldcard=read('old-public/card-rules.json');oldpin=read('old-public/card-pin.json')['value']
    if recipe['fixture_publisher_gym']!=oldseed['publisher_gym_id']:raise ValueError('inherited zero256 fixture publisher identity')
    pins,physical,seeds=M.assemble_pins(e,recipe,C)
    if pins['seed']['rules_id']!=oldseed['rules_id'] or pins['seed']['publisher_public_key']!=oldseed['publisher_public_key'] or pins['card']['rules_id']!=oldcard['id'] or pins['card']['rules_id']!=oldpin['rules_id']:raise ValueError('inherited rule/key reproduction')
    receipt=M.make_receipt(snapshot,recipe['capture_offset_ms']);decl=M.declaration(pins['seed'],receipt)
    signed=M.sign_body('seed declaration',decl,seeds['candidate seed declaration fixture'],C,'declaration')
    record=M.prepared_seed(pins['seed'],receipt,signed)
    card,card_input,inputid=M.card_ids(pins['card'],snapshot['event'],receipt['snapshot_digest'],decl['event_key'],recipe['target_height'],recipe['target_hash'])
    supplied=M.conditional_result_source(pins['bout'],read('old-public/bout-record.json'),receipt,recipe,(card,inputid),up)
    statement=M.result_statement(pins['result'],supplied)
    signed_result=M.sign_body('result statement',statement,seeds['candidate result statement fixture'],C,'statement')
    career=M.career(pins['result'],snapshot['event']['cutoff_ms'],recipe)
    codec=[];compositions=[];negative=[];raw=[]
    def row(family,name,value,scope='closed-codec'):
        b=M.canonical(value)
        return dict(family=family,name=name,scope=scope,value=value,canonical_hex=b.hex(),sha256=sha(b))
    for family,pin in pins.items():codec.append(row(family+'_pin','candidate',pin))
    codec.extend([row('event_receipt','modeled-original-capture',receipt),row('seed_declaration','prepared',decl),row('seed_signed_declaration','prepared',signed),row('seed_record','prepared',record)])
    for family,pin in pins.items():
        marker=dict(format='slug-owned-'+family,version=1,store_id=M.digest(family+' store',pin),pin=pin)
        x=row(family+'_marker','v1',marker,'private-marker-composition');x['file_hex']=(M.canonical(marker)+b'\n').hex();x['file_sha256']=sha(bytes.fromhex(x['file_hex']));compositions.append(x)
    for label,value in [('event receipt',receipt),('seed declaration',decl),('seed record',record),('card input',card_input),('result statement',statement)]:
        x=row('domain',label,value,'byte-composition-only' if label=='result statement' else 'domain-composition');x.update(preimage_hex=M.preimage(label,value).hex(),digest=M.digest(label,value));compositions.append(x)
    for family,body,signed_value,body_field in [('seed declaration',decl,signed,'declaration'),('result statement',statement,signed_result,'statement')]:
        message=bytes.fromhex(M.digest(family+' signature',body));pub=pins['seed' if family.startswith('seed')else'result']['publisher_public_key']
        if not C.ed_verify(bytes.fromhex(pub),message,bytes.fromhex(signed_value['signature'])):raise ValueError('signature selfcheck')
        x=row('signature',family,signed_value,'signature-composition-only' if family.startswith('result') else 'signed-declaration');x.update(message_hex=message.hex(),public_key=pub);compositions.append(x)
        seed=seeds['candidate seed declaration fixture' if family.startswith('seed') else 'candidate result statement fixture']
        for mode in recipe['signature_wrong_message_cases']:
            bad=copy.deepcopy(signed_value)
            if mode=='hexadecimal-display':bad['signature']=C.ed_sign(seed,message.hex().encode('ascii')).hex()
            elif mode=='inherited-family':
                label='candidate seed declaration' if family.startswith('seed')else'candidate result statement'
                bad['signature']=C.ed_sign(seed,M.old_domain(label)+M.lp(M.canonical(body))).hex()
            else:bad[body_field]['publisher_chain_id']='00'*32
            try:M.verify_signed(family,body_field,bad,pub,C)
            except M.Refused:pass
            else:raise AssertionError('accepted signature mutation '+mode)
            negative.append(row('signature',family+'/'+mode,bad,'signature-only-refusal'))
    seed_payload=b'SLUGSEED\x01'+bytes.fromhex(decl['event_key'])+bytes.fromhex(M.digest('seed declaration',decl))
    result_payload=b'SLUGRSLT\x01'+bytes.fromhex(statement['result_key'])+bytes.fromhex(M.digest('result statement',statement))
    compositions.append(dict(family='carrier',name='seed',scope='byte-composition-only',payload_hex=seed_payload.hex(),bytes=len(seed_payload)))
    compositions.append(dict(family='carrier',name='result',scope='byte-composition-only',payload_hex=result_payload.hex(),bytes=len(result_payload)))
    compositions.append(dict(family='identities',name='owned-card-and-result',scope='domain-composition',event_id=snapshot['event']['event_id'],seed_event_key=decl['event_key'],card_id=card,card_input_id=inputid,result_key=statement['result_key']))
    compositions.append(row('result_source','opaque-conditional-shape',supplied,'NOT-a-semantic-parser-positive; no-authenticated-booking-or-simulation-claim'))
    compositions.append(row('career_transition','exact-prefixes',career,'private-marker-and-activation-composition'))
    # Exercise the inherited custom SignedCheckpoint ordered JSON separately.
    cp=dict(version=1,network=pins['seed']['network'],genesis=pins['seed']['genesis'],height=recipe['target_height'],hash=recipe['target_hash'])
    cp_pre=b'SLUG signed checkpoint\0'+M.u32(1)+M.u32(cp['network'])+bytes.fromhex(cp['genesis'])[::-1]+M.u32(cp['height'])+bytes.fromhex(cp['hash'])[::-1]
    cp['signature']=C.ed_sign(seeds['candidate seed checkpoint fixture'],cp_pre).hex()
    checkpoint=row('checkpoint','modeled-custom-json',cp,'encoding-and-fixture-signature-only; no-PoW-or-finality');checkpoint.update(message_hex=cp_pre.hex(),public_key=pins['seed']['checkpoint_public_key']);compositions.append(checkpoint)
    for family,pin in pins.items():
        for mode in ['version_boolean','source_id','rules_id','nested_source','unknown','missing','order']:
            bad=copy.deepcopy(pin)
            if mode=='version_boolean':bad['version']=True
            elif mode in ['source_id','rules_id']:bad['source_store_id'if mode=='source_id'else'rules_id']='00'*32
            elif mode=='nested_source':bad['source_pin']['version']=2
            elif mode=='unknown':bad['unrecognized']=0
            elif mode=='missing':del bad['rules_id']
            else:bad=dict(reversed(list(bad.items())))
            try:M.validate_pin(family,bad,pins)
            except M.Refused:pass
            else:raise AssertionError('accepted pin mutation')
            negative.append(row(family+'_pin',mode,bad,'exact-context-refusal'))
    for mode in ['missing-activation','wrong-store','wrong-intent','timestamp-order','old-marker-lf','prepared-has-activation','legacy-format']:
        bad=copy.deepcopy(career)
        if mode=='missing-activation':bad['ready']['career']=None
        elif mode=='wrong-store':bad['ready']['career']['result_store_id']='00'*32
        elif mode=='wrong-intent':bad['ready']['career_intent_digest']='00'*32
        elif mode=='timestamp-order':bad['ready']['career']['activated_at_ms']=bad['ready']['career']['result_created_at_ms']-1
        elif mode=='old-marker-lf':bad['intent']['old_marker_sha256']=sha(M.canonical(bad['v1']))
        elif mode=='prepared-has-activation':bad['prepared']['career']=copy.deepcopy(bad['ready']['career'])
        else:bad['ready']['format']='slug-result'
        try:M.verify_career(pins['result'],bad,recipe,snapshot['event']['cutoff_ms'])
        except M.Refused:pass
        else:raise AssertionError('accepted career mutation')
        negative.append(row('career_transition',mode,bad,'exact-prefix-refusal'))
    for family,value in [('seed_pin',pins['seed']),('checkpoint',cp)]:
        canonical=M.canonical(value)
        for name,b,valid in [('canonical',canonical,True),('leading_space',b' '+canonical,False),('trailing_lf',canonical+b'\n',False),('trailing_value',canonical+b'{}',False),('reordered',M.canonical(dict(reversed(list(value.items())))),False),('duplicate_version',canonical.replace(b'{',b'{"version":1,',1),False)]:
            raw.append(dict(family=family,name=name,scope='closed-canonical-shape-only',valid=valid,raw_hex=b.hex()))
    return dict(version=1,scope='independent finite byte compositions; conditional public inputs; no Go outputs, complete-chain assertion, runtime authority, matching or simulation oracle',upstream_sha256=sha((read.root/'upstream/owned-consumers-v1.json').read_bytes()),contexts=dict(pins=pins,event_pin=e,event=snapshot['event']),codec_positives=codec,compositions=compositions,negatives=negative,raw_cases=raw)


def main():
    p=argparse.ArgumentParser();p.add_argument('--execute',action='store_true');p.add_argument('--input-root',required=True);p.add_argument('--approved-producer-lock',required=True);p.add_argument('--approved-lock-sha256',required=True);p.add_argument('--output',required=True);a=p.parse_args()
    if not a.execute:raise SystemExit('explicit --execute required')
    here=Path(__file__).resolve().parent;root=Path(a.input_root).resolve();output=Path(a.output).resolve();lockraw=Path(a.approved_producer_lock).read_bytes()
    if sha(lockraw)!=a.approved_lock_sha256:raise SystemExit('approved lock hash mismatch')
    lock=load_unique(lockraw)
    if lock.get('approved_by')!='root' or lock.get('execution_authorized')is not True:raise SystemExit('root execution approval required')
    if set(lock.get('sources',{}))!=SOURCE_NAMES:raise SystemExit('exact owned source set required')
    source_before={name:sha((here/name).read_bytes())for name in sorted(SOURCE_NAMES)}
    if source_before!=lock['sources']:raise SystemExit('source hash mismatch')
    catalograw=(here/'input-catalog.json').read_bytes()
    if sha(catalograw)!=lock['input_catalog_sha256']:raise SystemExit('input catalog hash mismatch')
    catalog=load_unique(catalograw);input_before=checked_inputs(root,catalog)
    if output.exists() or output==root or root in output.parents:raise SystemExit('exclusive external output required')
    output.mkdir(parents=True,exist_ok=False)
    report=dict(passed=False,lock_sha256=sha(lockraw),python=sys.version,source_before=source_before,input_before=input_before)
    try:
        M=module('owned_downstream_model',here/'downstream_model.py');C=module('owned_downstream_old_crypto',root/'crypto/crypto.py');S=module('owned_downstream_sizes',here/'sizing_model.py')
        C.selftest()  # Published RFC8032 and secp256k1 primitive checks precede generation.
        def read(path):return load_unique((root/path).read_bytes())
        read.root=root
        value=generate(M,C,read);value['budgets']=S.compose(value['codec_positives'])
        raw=pretty(value);(output/'owned-downstream-v1.json').write_bytes(raw)
        source_after={name:sha((here/name).read_bytes())for name in sorted(SOURCE_NAMES)};input_after=checked_inputs(root,catalog)
        if source_after!=source_before or input_after!=input_before:raise ValueError('producer inputs changed during generation')
        report.update(passed=True,source_unchanged=True,input_unchanged=True,output_sha256=sha(raw),output_bytes=len(raw),counts={k:len(value[k])for k in ['codec_positives','compositions','negatives','raw_cases','budgets']})
    except BaseException as e:
        report['failure']=type(e).__name__+': '+str(e);raise
    finally:(output/'report.json').write_bytes(pretty(report))
    print(json.dumps({k:report[k]for k in ['passed','output_sha256','output_bytes','counts']}))

if __name__=='__main__':main()
