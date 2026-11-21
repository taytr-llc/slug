#!/usr/bin/env python3
"""Root-admitted independent Set79 producer. Authoring does not execute this file."""
import sys
sys.dont_write_bytecode = True
import argparse
import hashlib
import json
import os
from pathlib import Path
import stat

from core import *
from composition import produce_composition, credentials
from payments import produce_payments, frame_bounds
from transport import produce_transport, money_expected
from outbox import produce_outbox

ROOT=Path(__file__).resolve().parent

def verify_lock(lock):
    require(lock.get('approved_by')=='root','execution not approved')
    require(lock.get('execution_authorized') is True,'execution not authorized')
    require(lock.get('version')==1 and lock.get('max_output_bytes')==16<<20,'lock bounds')
    files=lock.get('files');require(type(files) is list and 1<=len(files)<=128,'lock inventory')
    seen=set()
    for row in files:
        name=row['path'];require(type(name) is str and name not in seen,'inventory duplicate');seen.add(name)
        path=ROOT/name;require(not Path(name).is_absolute() and '..' not in Path(name).parts,'inventory path')
        require(path.resolve()==path and stat.S_ISREG(path.lstat().st_mode),'inventory regular file')
        require(path.stat().st_size==row['bytes'] and 0<row['bytes']<=8<<20,'inventory size')
        require(sha(path.read_bytes())==row['sha256'],'input/source changed')
    for required in ('generate.py','core.py','composition.py','transport.py','payments.py','outbox.py','recipe.json','copied-inputs.json'):
        require(required in seen,'missing producer dependency')
    for name in ('contract.md','test-plan.md','root-selection.json','frame-bounds.md'):
        require('inputs/'+name in seen,'missing documentary input')
    return seen

def read_inputs(directory,seen):
    require(directory.resolve()==ROOT/'inputs','selected input directory')
    catalog=json.loads((ROOT/'copied-inputs.json').read_bytes())
    for row in catalog['files']:
        require(row['input'] in seen,'catalog omitted from execution lock')
        raw=(ROOT/row['input']).read_bytes();require(len(raw)==row['bytes'] and sha(raw)==row['sha256'],'catalog mismatch')
    names={'provision':'provision.json','daily_wallet':'daily-wallet.json','daily_scheduler':'daily-scheduler.json','admission_actual':'admission-actual.json','capacity':'capacity.json','identities':'identities.json','miner_old':'miner-v1.json','pool_share':'pool-share.json','manifest_admission':'manifest-admission.json','payout_history':'payout-history.json','remote':'remote-v1.json','recipients':'recipients.json','daily_actual':'daily-actual.json','daily_wallet_recipe':'daily-wallet-recipe.json'}
    return {key:json.loads((directory/name).read_bytes()) for key,name in names.items()}

def raw_cases(compositions,credentials_result,transport_result,payments_result):
    c=compositions['positives'][1]
    # These check only canonical grammar/selected literal equality, not a second
    # complete implementation of every inherited semantic/cryptographic parser.
    targets=[('policy',c['policy'],65536,'optional_lf'),('setup',c['setup'],262144,'strict'),
             ('credential-config',credentials_result['config'],65536,'strict'),
             ('credential-state',credentials_result['cases'][2]['value'],1048576,'strict'),
             ('remote-config',transport_result['remote_positives'][0]['config'],262144,'strict'),
             ('local-credential',transport_result['local_credential_cases'][0]['value'],2048,'file_lf'),
             ('client',transport_result['wire_identities'][0]['client'],262144,'strict'),
             ('work-response',transport_result['envelope_cases'][0]['value'],(1<<24)+(64<<10),'http'),
             ('receipt-response',next(r['value'] for r in transport_result['envelope_cases'] if r['name']=='coinbase-only-submission'),64<<10,'http'),
             ('error-response',transport_result['error_cases'][0]['value'],64<<10,'http'),
             ('balance-response',payments_result['balances'][0]['value'],65536,'http'),
             ('page-response',payments_result['pages'][0]['value'],131072,'http'),
             ('page-request',payments_result['requests'][0]['value'],1024,'http')]
    out=[]
    for name,value,cap,mode in targets:
        core=canon(value);expected=core
        def check(raw):
            require(0<len(raw)<=cap,'size')
            if mode=='http':raw=raw.strip(b' \t\r\n')
            parsed=parse(raw,cap,final_lf=mode=='file_lf',optional_lf=mode=='optional_lf')
            require(canon(parsed)==expected,'selected canonical shape')
        positive=core+(b'\n' if mode in ('file_lf','http') else b'')
        rows=[('canonical',positive,True),('leading-space',b' '+positive,mode=='http'),('extra-LF',positive+b'\n',mode in ('http','optional_lf'))]
        if mode=='optional_lf':rows.append(('two-LFs',core+b'\n\n',False))
        first=next(iter(value));dup=canon({first:value[first]})[1:-1]
        missing=clone(value);missing.pop(next(reversed(missing)))
        invalid=[('unknown-field',b'{"unexpected":0,'+core[1:]),('duplicate-field',b'{'+dup+b','+core[1:]),('missing-field',canon(missing)),('reordered-field',canon(dict(reversed(list(value.items()))))),('inner-space',core.replace(b',',b', ',1)),('second-value',core+b'{}'),('null',b'null')]
        for suffix,raw in invalid:rows.append((suffix,raw+(b'\n' if mode=='file_lf' else b''),False))
        for suffix,raw,valid in rows:
            require(accepted(check,raw)==valid,'raw mutation: '+name+'-'+suffix)
            out.append(dict(name=name+'-'+suffix,family=name,framing=mode,valid=valid,scope='canonical selected fixture grammar only',raw_hex=raw.hex()))
        # Compressed exact-bound HTTP padding tests avoid duplicating large
        # templates in output; root/Go consumers expand this literal recipe.
        if mode=='http':
            require(len(core)<=cap)
            at=core+b' '*(cap-len(core));require(accepted(check,at) and not accepted(check,at+b' '))
            out.append(dict(name=name+'-exact-byte-cap',family=name,framing=mode,valid=True,scope='canonical selected fixture with permitted outer HTTP padding',raw_recipe=dict(prefix_hex=core.hex(),padding_hex='20',padding_count=cap-len(core)),byte_count=cap))
            out.append(dict(name=name+'-above-byte-cap',family=name,framing=mode,valid=False,scope='predecode size refusal',raw_recipe=dict(prefix_hex=core.hex(),padding_hex='20',padding_count=cap+1-len(core)),byte_count=cap+1))
    return out

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--inputs',required=True);parser.add_argument('--lock',required=True);parser.add_argument('--out',required=True)
    args=parser.parse_args();lock_path=Path(args.lock);output_path=Path(args.out)
    require(lock_path.is_absolute() and output_path.is_absolute() and not output_path.exists(),'explicit fresh paths')
    require(ROOT not in output_path.parents,'outputs outside immutable reference root')
    lock_bytes=lock_path.read_bytes();lock=json.loads(lock_bytes);seen=verify_lock(lock)
    recipe=json.loads((ROOT/'recipe.json').read_bytes());inputs=read_inputs(Path(args.inputs),seen)
    require(recipe['version']==1 and recipe['execution']['max_output_bytes']==lock['max_output_bytes'])
    selection=json.loads((ROOT/'inputs/root-selection.json').read_bytes())
    require(selection['approved_by']=='root' and selection['producer_execution_authorized'] is False)
    for source,name in [('public-daily-contract.md','contract.md'),('public-daily-test-plan.md','test-plan.md')]:
        require(sha((ROOT/'inputs'/name).read_bytes())==selection['source_documents'][source],'selected contract changed')
    compositions,_=produce_composition(inputs,recipe)
    credential_result=credentials(compositions['positives'][1]['setup']['miners'])
    transport_result,work_cases=produce_transport(inputs,compositions)
    require(transport_result['error_status_map']['stale_capture']==recipe['daily_stale_capture_http_status'])
    expected=money_expected(transport_result['remote_positives'][0]['daily_client'])
    payments_result=produce_payments(expected)
    outbox_result=produce_outbox(work_cases)
    bounds=frame_bounds()
    for name,value in recipe['frame_cross_check'].items():
        require(bounds['widths'][name]==value,'independent frame cross-check: '+name)
    raws=raw_cases(compositions,credential_result,transport_result,payments_result)
    counts=dict(compositions=len(compositions['positives']),policy_cases=len(compositions['policy_cases']),credential_cases=len(credential_result['cases']),wire_identities=len(transport_result['wire_identities']),envelope_cases=len(transport_result['envelope_cases']),remote_cases=len(transport_result['remote_cases']),local_credential_cases=len(transport_result['local_credential_cases']),balance_cases=len(payments_result['balances']),page_cases=len(payments_result['pages']),request_cases=len(payments_result['requests']),traversals=len(payments_result['traversals']),raw_cases=len(raws))
    counts['error_cases']=len(transport_result['error_cases'])
    for name in ('config_cases','marker_cases','scan_cases','entry_cases','journal_cases','issue_transitions'):
        counts['outbox_'+name]=len(outbox_result[name])
    for name,value in recipe['expected_counts'].items():require(counts[name]==value,'frozen case count: '+name)
    result=dict(version=1,scope=recipe['scope'],source_provenance=[dict(path=r['path'],bytes=r['bytes'],sha256=r['sha256']) for r in lock['files']],
                counts=counts,composition=compositions,credentials=credential_result,transport=transport_result,payments=payments_result,outbox=outbox_result,raw_cases=raws,frame_bounds=bounds,
                exclusions=['No Go-derived expected values','No proof, signature, key or certificate generation','No production signer/session/wallet authority from modeled public identities','No observed crash durability or financial state from conditional traces','No unlimited service or resolution of unknown issuance'])
    raw=json.dumps(result,indent=2,ensure_ascii=True,allow_nan=False).encode('ascii')+b'\n'
    require(len(raw)<=lock['max_output_bytes'],'output bound')
    verify_lock(lock);require(lock_path.read_bytes()==lock_bytes,'execution lock changed')
    with output_path.open('xb') as stream:
        stream.write(raw);stream.flush();os.fsync(stream.fileno())
    verify_lock(lock);require(lock_path.read_bytes()==lock_bytes,'execution lock changed after output')
    print(json.dumps(dict(bytes=len(raw),sha256=sha(raw),counts=counts),sort_keys=True))

if __name__=='__main__':main()
