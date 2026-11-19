#!/usr/bin/env python3
"""Five fixed normal-Argon evaluations; no mining/search or SLUG Go imports.

Python derives every nonce-adjusted byte, target comparison and duplicate key.
A tiny standalone x/crypto oracle supplies only Argon2id(header,header).
Default checks existing JSON; --write creates it exclusively before adapter code.
"""
import argparse
import copy
import hashlib
import json
import os
import importlib.util
from pathlib import Path
import struct
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
SOURCE = ROOT/'fighterdescriptor/testdata/reference/manifest_job_v1.json'
SOURCE_SHA = '34b9484db90fd6d9ca9d704ee1ac76843a717bddaeb38194df1932f77fe754ad'
DOMAIN = b'SLUG pool submission\x00v1\x00'
SPECS = (('coinbase-only', 0), ('coinbase-only', 1),
         ('coinbase-only', 0xffffffff), ('witness-odd-two-level-dependent', 0),
         ('modeled-all-three-miss', 0))
HELPER = SOURCE.with_name('reference_manifest_jobs.py')
HELPER_SHA = '2beaf985ae5f7851df4c945104e1c16559915dfb4ef23d6dfa4c019c0e8a4159'
HELPER_INPUT_INDEX_SHA = 'd23e9a1c12f4a10256e167b94d768e6ae35f24a6f42a3430b430916b1fdaba7b'
FLAGS = 'all=-d=ssa/generic_cse/off,ssa/lowered_cse/off'

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def compact(bits):
    size, word = bits >> 24, bits & 0x7fffff
    assert not bits & 0x800000
    return word >> (8*(3-size)) if size <= 3 else word << (8*(size-3))

def generate():
    original = SOURCE.read_bytes()
    assert sha(original) == SOURCE_SHA
    source = json.loads(original)
    candidates = {v['name']: v for v in source['candidates']}
    assert sha(HELPER.read_bytes()) == HELPER_SHA
    assert sha(HELPER.with_name('input_hashes.json').read_bytes()) == HELPER_INPUT_INDEX_SHA
    spec=importlib.util.spec_from_file_location('set52_frozen_manifest_reference',HELPER)
    helper=importlib.util.module_from_spec(spec);spec.loader.exec_module(helper)
    # Supplied-context model only: recompute every affected immutable identity
    # and coinbase/merkle/header byte with the existing independent Python helpers.
    modeled=copy.deepcopy(candidates['coinbase-only']);modeled['name']='modeled-all-three-miss'
    context=copy.deepcopy(modeled['job']['context'])
    context['prospect_target']=f'{1:064x}';context['expected_bits']=0x01010000
    old=modeled['job']
    modeled['job']=helper.new_job(context,old['authority_id'],old['descriptor_id'],old['public_key'],old['challenge'],old['issued_at_ms'])
    modeled['template']['context']=copy.deepcopy(context)
    modeled['template']['block']['bits']='01010000'
    modeled['template']['block']['target']=f'{1:064x}'
    modeled['expected'],_=helper.candidate(modeled['job'],modeled['template'],modeled['payout_address'],modeled['extra_nonce'])
    candidates[modeled['name']]=modeled
    headers = []
    for name, nonce in SPECS:
        base = candidates[name]['expected']
        header = bytes.fromhex(base['proof']['header'])
        block = bytes.fromhex(base['block_hex'])
        assert len(header) == 80 and block[:80] == header
        assert header[76:80] == bytes(4)
        headers.append(header[:76] + struct.pack('<I', nonce))
    env = dict(os.environ, GOENV='off', GOTOOLCHAIN='local', GOFLAGS='-mod=readonly',
               GOEXPERIMENT='', GOCACHEPROG='', GOWORK='off', GOPROXY='off', GOSUMDB='off',
               GOMAXPROCS='4', CGO_ENABLED='0', GOOS='darwin', GOARCH='arm64',
               GOCACHE='/private/tmp/slug-go-build', GOMODCACHE='/private/tmp/slug-desktop-modcache')
    assert subprocess.check_output(['go','env','GOVERSION'],env=env,text=True,timeout=20).strip() == 'go1.27.4'
    with tempfile.TemporaryDirectory(prefix='set52-argon-') as tmp:
        oracle = Path(tmp)/'argon'
        subprocess.run(['go','build','-gcflags='+FLAGS,'-o',str(oracle),'argon_oracle.go'],
                       cwd=HERE/'argon',env=env,check=True,timeout=60)
        values = json.loads(subprocess.check_output([str(oracle),*[h.hex() for h in headers]],
                                                   env=env,timeout=30))
    assert len(values) == len(SPECS)
    cases = []
    for (name, nonce), header, value in zip(SPECS, headers, values):
        base = candidates[name]
        assert value['header'] == header.hex()
        digest = bytes.fromhex(value['digest'])
        assert len(digest) == 32 and value['work_hash'] == digest[::-1].hex()
        number = int.from_bytes(digest, 'little')
        assert 0 < number < (1<<256)-1
        prospect_target = int(base['job']['context']['prospect_target'],16)
        bits = int.from_bytes(header[72:76], 'little')
        assert bits == base['job']['context']['expected_bits']
        block_target = compact(bits)
        changed = copy.deepcopy(base['expected'])
        changed['proof']['header'] = header.hex()
        changed['block_hex'] = (header + bytes.fromhex(base['expected']['block_hex'])[80:]).hex()
        # Nonce bytes alone change. Coinbase, witness bytes, branches, payout,
        # fees, job/context and all candidate accounting remain frozen.
        unchanged = copy.deepcopy(changed)
        unchanged['proof']['header'] = base['expected']['proof']['header']
        unchanged['block_hex'] = base['expected']['block_hex']
        assert unchanged == base['expected']
        assert bytes.fromhex(changed['block_hex'])[80:] == bytes.fromhex(base['expected']['block_hex'])[80:]
        preimage = DOMAIN + header
        targets=[('one',1)] if name=='modeled-all-three-miss' else [('digest_minus_one',number-1),('digest',number),('digest_plus_one',number+1)]
        if name=='modeled-all-three-miss':assert number>1 and prospect_target==block_target==1
        cases.append(dict(name=name+'-nonce-'+str(nonce),source_candidate=name,nonce=nonce,
                          header=header.hex(),argon_digest=digest.hex(),work_hash=digest[::-1].hex(),
                          submission_key_preimage=preimage.hex(),submission_key=sha(preimage),
                          prospect_target=f'{prospect_target:064x}',block_target=f'{block_target:064x}',
                          qualifies_prospect=number<=prospect_target,qualifies_block=number<=block_target,
                          share_targets=[dict(relation=label,target=f'{target:064x}',meets_target=number<=target)
                                         for label,target in targets],
                          qualifying_candidate=changed if any(number<=v for _,v in targets) or number<=prospect_target or number<=block_target else None))
    return dict(version=1,scope='Independent fixed-header normal-PoW adapter vectors; no pool or chain acceptance',
                provenance=dict(source_path=SOURCE.relative_to(ROOT).as_posix(),source_sha256=SOURCE_SHA,
                                modeled_candidate_helper_sha256=HELPER_SHA,
                                modeled_candidate_helper_input_index_sha256=HELPER_INPUT_INDEX_SHA,
                                source_candidate_bytes='existing independently derived Python ManifestJob fixtures',
                                oracle='standalone x/crypto/argon2; no SLUG Go imports',
                                oracle_inputs_sha256={n:sha((HERE/'argon'/n).read_bytes()) for n in ['argon_oracle.go','go.mod','go.sum']}),
                argon2=dict(variant='id',version=19,memory_kib=262144,iterations=1,parallelism=1,length=32,password='exact80header',salt='exact80header'),
                work_hash_encoding='Argon digest bytes reversed, lowercase hex; numeric comparison is digest interpreted little-endian',
                duplicate_key_encoding='SHA256 of exact ASCII domain including both NUL bytes followed by exact80header; ordinary SHA256 hex, no byte reversal',
                independence='Neither work hash nor duplicate key takes pool target, pool session, or observation time as input. Fixed prospect/block targets come only from unchanged source.',
                limitations=['Synthetic public candidate templates/targets; normal Argon parameters do not establish live-chain validity or authorization.',
                             'Four unchanged-template rows have an all-ones prospect target; fifth row is explicitly modeled supplied context with prospect/block/pool targets one, never live-node-valid issuance.',
                             'Exactly five declared headers per generation; no nonce search. Pool target never alters a prepared candidate.'],
                sources=[{k:v[k] for k in ('name','job','template','payout_address','extra_nonce','expected')} for v in candidates.values() if v['name'] in {n for n,_ in SPECS}],
                cases=cases)

if __name__ == '__main__':
    p=argparse.ArgumentParser();p.add_argument('--write',action='store_true');args=p.parse_args()
    raw=(json.dumps(generate(),indent=2)+'\n').encode();target=HERE/'pool_share_v1.json'
    if args.write:
        with target.open('xb') as f:f.write(raw)
    else:
        assert target.read_bytes()==raw,'reference mismatch'
    print(json.dumps({'cases':len(SPECS),'share_target_cases':13,'sha256':sha(raw)}))
