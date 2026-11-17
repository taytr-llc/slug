#!/usr/bin/env python3
"""Independent set41 namespace/assignment/condition expectations.

Actual set40 public candidate receipts are INPUTS, never golden expectations.
The frozen set37 Python oracle verifies every normal-PoW proof, signature, job,
DNA, birth structure and binding first. It cannot authenticate historical node
observations or authority time. The frozen set35 integer oracle derives current
condition; Python hashlib and the approved set41 framing derive new digests.
No SLUG Go runtime, private wallet, network or operator key is used.
"""
import argparse, hashlib, importlib.util, json, struct, sys
from pathlib import Path
sys.dont_write_bytecode=True
HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[2]
CONTRACT='f977e8a7cef5ab470e5019f4f1b9d88cdb4fd03d5ee1feb6acc32321389d84d2'
PUBLIC_HASH='c5b30b106fd55935e0483b0a1ac28f3d67d8301076186e64de549bcddf055020'

def sha(raw): return hashlib.sha256(raw).hexdigest()
def canon(value): return json.dumps(value,ensure_ascii=True,separators=(',',':'),allow_nan=False).encode('ascii')
def load(name,path):
    spec=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(spec);sys.modules[name]=m;spec.loader.exec_module(m);return m

def ordered(value,shape):
    if isinstance(shape,dict):
        assert set(value)==set(shape)
        return {k:ordered(value[k],v) for k,v in shape.items()}
    if isinstance(shape,list):
        return [ordered(v,shape[0]) if shape else v for v in value]
    return value

def generate():
    source=HERE/'inputs/public-candidate-records.json';raw=source.read_bytes();assert sha(raw)==PUBLIC_HASH
    public=json.loads(raw);assert public['candidate_fixture_only'] and len(public['records'])==6
    references={
        'registry/testdata/manifest/reference/reference_manifest_admission.py':'ab65e2c63fb288c3d117416825edc44a8a5596f31e54b50f12f476b96c883bbc',
        'registry/testdata/manifest/reference/manifest_admission_v1.json':'d32a8653c76c643addb3df091ca8d7dfdba66c2486f2ca305e718113f71d5794',
        'fightercondition/testdata/reference/condition_v1.json':'4f5be20e0e6ceb6b2ef4ee4063fffd2691fecb7a71d984f4624eb97162a9dbfa',
        'fightercondition/testdata/reference/reference_condition.py':'924152a89241e9fa6d60aa27ba19666d3a3f65fdf53bd3c0cf6f89c8db4e946c',
        'gymaccount/testdata/reference_enrollment.py':'24cbfdbaa84f8c106a6ce0b2b7128f9bdc5082e09d41d528d4a27efc576d3571',
    }
    for path,h in references.items():assert sha((ROOT/path).read_bytes())==h,path
    oracle=load('stable_independent_admission',ROOT/'registry/testdata/manifest/reference/reference_manifest_admission.py')
    condition=load('stable_independent_condition',ROOT/'fightercondition/testdata/reference/reference_condition.py')
    enrollment=load('stable_independent_enrollment',ROOT/'gymaccount/testdata/reference_enrollment.py')
    published={f['name']:f for f in enrollment.FIXTURES}
    shapes=json.loads((ROOT/'registry/testdata/manifest/reference/manifest_admission_v1.json').read_bytes())
    shape=shapes['cases'][0]['expected_record']
    jobs={r['record']['job']['id']:r['record']['job'] for r in public['records']}
    meta={'last_sequence':max(r['record']['sequence'] for r in public['records']),'last_clock_ms':max(r['record']['birth']['accepted_at_ms'] for r in public['records'])}
    pins=[];cases=[];seen=set()
    with oracle.Argon() as argon:
        for row in public['records']:
            r=ordered(row['record'],shape);v=row['view'];f=published[row['fixture']]
            assert r['job']['public_key']==f['gym_public_key'] and r['job']['gym_id']==f['gym_id']
            work=argon.hash(r['proof']['header']);oracle.validate_record(r,v['authority_pin'],jobs,meta,{r['proof']['header']:work})
            manifest,_,_,binding,_=oracle.appearance(work['work_hash'])
            identity={'network':enrollment.NETWORK,'genesis':enrollment.GENESIS,'profile':enrollment.PROFILE,'root_public_key':f['root_public_key'],'gym_public_key':f['gym_public_key'],'gym_id':f['gym_id']}
            pin={'version':1,'account_authority_id':v['account_authority_id'],'account_id':v['account_id'],'identity':identity,'source':ordered(v['authority_pin'],shapes['pin']),'condition_version':1,'projector_id':condition.PROJECTOR}
            p=canon(pin);preimage=b'SLUG candidate stable\x00v1\x00'+struct.pack('<I',len(p))+p
            if row['fixture'] not in seen:
                pins.append({'name':row['fixture'],'pin':pin,'canonical':p.decode(),'id_preimage':preimage.hex(),'expected_id':sha(preimage)});seen.add(row['fixture'])
            record_raw=canon(r);digest_preimage=b'SLUG stable source record\x00v1\x00'+struct.pack('<I',len(record_raw))+record_raw
            assignment={'version':1,'sequence':r['sequence'],'prospect_id':r['verified']['prospect_id'],'gym_id':r['verified']['gym_id'],'work_hash':r['verified']['work_hash'],'born_at_ms':r['birth']['born_at_ms'],'accepted_at_ms':r['birth']['accepted_at_ms'],'descriptor_id':binding['descriptor_id'],'manifest_digest':binding['manifest_digest'],'portrait_digest':binding['portrait_digest'],'source_record_digest':sha(digest_preimage)}
            born=assignment['born_at_ms'];dna=r['verified']['dna'];week=condition.WEEK_MS
            times=sorted({assignment['accepted_at_ms'],born+week-1,born+week,born+dna['prime_weeks']*week-1,born+dna['prime_weeks']*week,born+(dna['prime_weeks']+1)*week,born+(dna['prime_weeks']+dna['decline_weeks'])*week-1,born+(dna['prime_weeks']+dna['decline_weeks'])*week})
            projected=[{'as_of_ms':t,'expected':condition.project(1,dna,born,t,0)} for t in times]
            cases.append({'fixture':row['fixture'],'record':r,'record_canonical':record_raw.decode(),'assignment':assignment,'manifest':manifest,'conditions':projected})
        count=len(argon.hashes)
    old=json.loads((ROOT/'fightercondition/testdata/reference/condition_v1.json').read_bytes())
    xp0=[{'name':x['name'],'dna':old['dna_cases'][x['dna_index']]['dna'],'born_at_ms':x['born_at_ms'],'as_of_ms':x['as_of_ms'],'expected':x['expected']} for x in old['cases'] if x['pro_xp']==0]
    return {'version':1,'candidate_fixture_only':True,'approved_contract_sha256':CONTRACT,'input_sha256':PUBLIC_HASH,'reference_sha256':references,'normal_argon_verifications':count,'pins':pins,'cases':cases,'frozen_xp0_condition_count':len(xp0),'limits':['Historical eligibility/time is trusted input, not independently authenticated.','Candidate assignment is not portable ownership or production activation.']}

def main():
    p=argparse.ArgumentParser();p.add_argument('--check',action='store_true');args=p.parse_args();result=generate();raw=(json.dumps(result,indent=2,ensure_ascii=True)+'\n').encode('ascii');out=HERE/'stable_v1.json'
    if args.check:assert out.read_bytes()==raw
    else:out.write_bytes(raw)
    print(json.dumps({'status':'passed','mode':'check' if args.check else 'generate','sha256':sha(raw),'pins':len(result['pins']),'records':len(result['cases']),'normal_argon_verifications':result['normal_argon_verifications'],'xp0_conditions':result['frozen_xp0_condition_count']}))
if __name__=='__main__':main()
