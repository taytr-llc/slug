#!/usr/bin/env python3
"""Unexecuted independent public-reader producer. Requires a root-approved lock.

This is not a DB loader: synthetic row descriptions are byte-layout vectors,
not valid ciphertext, wallet ownership, encryption or live capture handles.
"""
import argparse
import json
from pathlib import Path
import struct
import sys
sys.dont_write_bytecode = True

from common import Rejected, canonical, own, sha
from public_rows import account_row, cursors, IMPORTED
from public_inventory import read_inventory, PURPOSES, SCHEMAS
from size_model import size_report

INPUT_LOCK_SHA256='f7809a938be24d9a320beeff9f8ed09fb2a43f9245864ca7ab0d4f5bb7a16dd9'
SOURCE_NAMES=('common.py','public_rows.py','public_inventory.py','size_model.py','generate_public_reference.py','public-case-plan.json')


def preflight(inputs, lock_path, lock_sha):
    raw = lock_path.read_bytes()
    if sha(raw) != lock_sha:
        raise SystemExit('unapproved producer lock')
    lock = json.loads(raw)
    if lock.get('approved_by') != 'root' or lock.get('execution_authorized') is not True:
        raise SystemExit('producer execution not authorized')
    source_root = Path(__file__).resolve().parent
    if set(lock['sources']) != set(SOURCE_NAMES):
        raise SystemExit('incorrect reviewed source set')
    for name in SOURCE_NAMES:
        data = (source_root/name).read_bytes()
        if sha(data) != lock['sources'][name]:
            raise SystemExit('producer source differs from reviewed lock')
    input_raw=(inputs.parent/'input-lock-v1.json').read_bytes()
    if sha(input_raw)!=INPUT_LOCK_SHA256 or lock['input_lock_sha256']!=INPUT_LOCK_SHA256:
        raise SystemExit('input lock differs from reviewed bytes')
    input_lock=json.loads(input_raw)
    data = (inputs.parent/input_lock['catalog']).read_bytes()
    if sha(data) != input_lock['catalog_sha256']:
        raise SystemExit('input catalog differs from lock')
    catalog = json.loads(data)
    if len(catalog['files'])!=input_lock['file_count'] or input_lock['file_count']!=75:
        raise SystemExit('input count mismatch')
    seen=set();total=0
    for row in catalog['files']:
        if row['path'] in seen:raise SystemExit('duplicate input')
        seen.add(row['path'])
        path = inputs / row['path']
        if not path.resolve().is_relative_to(inputs.resolve()):
            raise SystemExit('input escaped frozen root')
        data = path.read_bytes()
        if len(data) != row['bytes'] or sha(data) != row['sha256']:
            raise SystemExit('input differs from reviewed lock')
        total+=len(data)
    if total!=input_lock['input_bytes']:raise SystemExit('input byte count mismatch')
    data = (inputs/'fixture-recipe.json').read_bytes()
    if sha(data) != input_lock['recipe_sha256']:
        raise SystemExit('input recipe differs from lock')
    recipe=json.loads(data)
    if sha((inputs/recipe['paths']['contract']).read_bytes())!=input_lock['contract_sha256']:
        raise SystemExit('contract differs from lock')
    return recipe, seen


def original_manifest(actual):
    original = own(actual)
    for a in original['accounts']:
        a['next_external'] = a['next_internal'] = 0
    return original


def layout(original, nexts, imported='absent', private=b'PRIVATE-SPAN-SENTINEL'):
    rows = []
    for i, purpose in enumerate(PURPOSES):
        default = account_row(b'PUBLIC-ENC-SENTINEL', private, *nexts[2*i:2*i+2])
        imp = account_row(b'',b'',0,0,b'imported')
        rows.append(dict(key_hex=struct.pack('<II',purpose,0).hex(),is_bucket=True,
                         schema_hex=SCHEMAS[i],accounts=[
                             dict(key_hex=struct.pack('<I',0).hex(),row_hex=default.hex()),
                             dict(key_hex=struct.pack('<I',IMPORTED).hex(),row_hex=imp.hex())],
                         imported_index=imported))
    return dict(manager_version=9,derivation_mode=1,recovery_version=1,watch_only=False,
                network=original['network'],genesis=original['genesis'],
                public_manifest=own(original),public_root=original['root_public_key'],scopes=rows)


def row_cases():
    out=[]
    def add(name,raw,account,accepted):
        entry=dict(name=name,account=account,row_hex=raw.hex(),accepted=accepted,
                   evidence='synthetic binary layout only; spans are inert public bytes')
        try:
            result=cursors(raw,account)
        except Rejected:
            if accepted:raise
        else:
            if not accepted:raise AssertionError(name+' unexpectedly valid')
            entry['cursors']=result
        out.append(entry)
    base=account_row(b'p',b'k',0,0,b'')
    add('minimal_default',base,0,True)
    add('ordinary_default',account_row(b'p'*32,b'k'*48,1,1),0,True)
    add('both_at4096',account_row(b'p',b'k',4096,4096),0,True)
    add('mixed_zero4096',account_row(b'p',b'k',0,4096),0,True)
    add('required_imported_metadata',account_row(b'',b'',0,0,b'imported'),IMPORTED,True)
    add('exact65536_mixed_row',account_row(b'p',b'k'*65510,4096,4096,b''),0,True)
    add('opaque_nontext_name_span',account_row(b'p',b'k',1,2,b'\xff\xfe'),0,True)
    add('empty',b'',0,False);add('short_header',b'\0'*4,0,False)
    add('unsupported_account_type',b'\x01'+base[1:],0,False)
    add('body_length_short',base[:1]+struct.pack('<I',len(base)-6)+base[5:],0,False)
    add('body_length_long',base[:1]+struct.pack('<I',len(base)-4)+base[5:],0,False)
    add('public_length_overflow',base[:5]+b'\xff'*4+base[9:],0,False)
    add('private_length_overflow',base[:10]+b'\xff'*4+base[14:],0,False)
    add('name_length_overflow',base[:-4]+b'\xff'*4,0,False)
    add('trailing_body',base[:1]+struct.pack('<I',len(base)-4)+base[5:]+b'!',0,False)
    add('missing_default_public',account_row(b'',b'k',0,0),0,False)
    add('missing_default_private',account_row(b'p',b'',0,0),0,False)
    add('external4097',account_row(b'p',b'k',4097,0),0,False)
    add('internal4097',account_row(b'p',b'k',0,4097),0,False)
    add('imported_public_nonempty',account_row(b'p',b'',0,0),IMPORTED,False)
    add('imported_private_nonempty',account_row(b'',b'k',0,0),IMPORTED,False)
    add('imported_cursor_nonzero',account_row(b'',b'',0,1),IMPORTED,False)
    add('foreign_account_number',base,1,False)
    add('row65537',account_row(b'p',b'k'*65511,0,0,b''),0,False)
    return out


def inventory_cases(originals, actuals):
    out=[]
    def add(name,fixture,state,accepted):
        entry=dict(name=name,fixture=fixture,layout=state,accepted=accepted,
                   evidence='synthetic bounded bucket layout with separately inherited public identity')
        try:
            manifest,counts=read_inventory(state,originals[fixture])
        except Rejected:
            if accepted:raise
        else:
            if not accepted:raise AssertionError(name+' unexpectedly valid')
            entry.update(manifest=manifest,manifest_canonical_utf8=canonical(manifest).decode(),
                         manifest_sha256=sha(canonical(manifest)),modeled_reads=counts)
        out.append(entry)
    for fixture in ('zero256','incrementing256'):
        original=originals[fixture]
        actual=[a[k] for a in actuals[fixture]['accounts'] for k in ('next_external','next_internal')]
        for suffix,values,probe,private in [
            ('initial_absent_imports',[0]*8,'absent',b'private'),
            ('actual_native84_1_1',actual,'empty',b'private'),
            ('all4096',[4096]*8,'empty',b'private'),
            ('mixed_cursors',[0,1,2,3,4095,4096,17,29],'absent',b'private'),
            ('opaque_private_content',actual,'empty',b'\x00\xff'*128)]:
            add(fixture+'_'+suffix,fixture,layout(original,values,probe,private),True)
    original=originals['zero256'];base=layout(original,[0]*8)
    def bad(name,mutate):
        state=own(base);mutate(state);add(name,'zero256',state,False)
    for version in (8,10,11,12):
        bad('schema'+str(version),lambda s,n=version:s.update(manager_version=n))
    bad('legacy_derivation',lambda s:s.update(derivation_mode=0))
    bad('profile2_recovery_version',lambda s:s.update(recovery_version=2))
    bad('watch_only',lambda s:s.update(watch_only=True))
    bad('foreign_network',lambda s:s.update(network=1))
    bad('foreign_genesis',lambda s:s.update(genesis='f'*64))
    bad('other_fixture_public_manifest',lambda s:s.update(public_manifest=own(originals['incrementing256'])))
    bad('nested_cursor_boolean_false',lambda s:s['public_manifest']['accounts'][0].update(next_external=False))
    bad('public_root_mismatch',lambda s:s.update(public_root=originals['incrementing256']['root_public_key']))
    bad('missing_scope',lambda s:s['scopes'].pop())
    bad('fifth_scope_refusal_witness',lambda s:s['scopes'].append(dict(s['scopes'][0],key_hex=struct.pack('<II',87,0).hex())))
    bad('foreign_coin',lambda s:s['scopes'][1].update(key_hex=struct.pack('<II',49,1).hex()))
    bad('short_scope_key',lambda s:s['scopes'][0].update(key_hex='00'))
    bad('scope_is_value',lambda s:s['scopes'][0].update(is_bucket=False))
    bad('wrong_mixed49_schema',lambda s:s['scopes'][1].update(schema_hex='0303'))
    bad('schema_order_reversed',lambda s:s['scopes'][1].update(schema_hex='0304'))
    bad('missing_default',lambda s:s['scopes'][0]['accounts'].pop(0))
    bad('missing_required_imported_metadata',lambda s:s['scopes'][0]['accounts'].pop(1))
    bad('third_account_refusal_witness',lambda s:s['scopes'][0]['accounts'].append(dict(key_hex='01000000',row_hex=s['scopes'][0]['accounts'][0]['row_hex'])))
    bad('short_account_key',lambda s:s['scopes'][0]['accounts'][0].update(key_hex='00'))
    bad('nonempty_imported_index',lambda s:s['scopes'][0].update(imported_index='present'))
    bad('missing_parent_index_bucket',lambda s:s['scopes'][0].update(imported_index='parent_missing'))
    bad('mixed_row_malformed',lambda s:s['scopes'][0]['accounts'][0].update(row_hex='00'))
    bad('imported_private_material',lambda s:s['scopes'][0]['accounts'][1].update(row_hex=account_row(b'',b'k',0,0).hex()))
    bad('imported_index_cursor',lambda s:s['scopes'][0]['accounts'][1].update(row_hex=account_row(b'',b'',1,0).hex()))
    return out


def generate(inputs,recipe):
    actuals={f:json.loads((inputs/recipe['paths']['actual_manifests'][f]).read_bytes()) for f in recipe['fixture_order']}
    originals={f:original_manifest(actuals[f]) for f in recipe['fixture_order']}
    inherited=json.loads((inputs/recipe['paths']['old_recovery']).read_bytes())
    if inherited['fixture_only'] is not True:raise AssertionError('nonfixture input')
    old={row['name']:row['export']['manifest'] for row in inherited['cases']}
    for fixture,manifest in originals.items():
        if canonical(manifest)!=canonical(original_manifest(old[fixture])):
            raise AssertionError('public identity differs from independent old recovery corpus')
    return dict(version=1,scope='Independent public-row and bounded inventory consistency only; no live capture or private cryptographic evidence',
                inherited_actual_manifests=actuals,rows=row_cases(),inventories=inventory_cases(originals,actuals),
                size_composition=size_report(recipe,lambda name:json.loads((inputs/name).read_bytes())),
                runtime_only_checks=['one committed walletdb View','exact retained Manager owner','no private-retention Get',
                                     'private ciphertext span never copied/decrypted/retained','context join before close',
                                     'closed/busy owner rejection','real public ciphertext decryption','opaque CaptureID owner binding'])


def main():
    p=argparse.ArgumentParser();p.add_argument('--execute',action='store_true');p.add_argument('--input-root',required=True,type=Path)
    p.add_argument('--approved-producer-lock',required=True,type=Path)
    p.add_argument('--approved-lock-sha256',required=True);p.add_argument('--output',required=True,type=Path)
    args=p.parse_args()
    if not args.execute:raise SystemExit('explicit --execute required')
    if args.output.exists():raise SystemExit('refusing to replace an outcome')
    recipe,seen=preflight(args.input_root,args.approved_producer_lock,args.approved_lock_sha256)
    for key in ('old_recovery','rsa','descriptor','contract'):
        if recipe['paths'][key] not in seen:raise SystemExit('recipe input not locked')
    for name in recipe['paths']['actual_manifests'].values():
        if name not in seen:raise SystemExit('manifest input not locked')
    result=generate(args.input_root,recipe)
    raw=json.dumps(result,ensure_ascii=False,allow_nan=False,indent=2).encode()+b'\n'
    preflight(args.input_root,args.approved_producer_lock,args.approved_lock_sha256)
    with args.output.open('xb') as f:f.write(raw)


if __name__=='__main__':main()
