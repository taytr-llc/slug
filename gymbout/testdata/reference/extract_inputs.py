#!/usr/bin/env python3
"""Minimal immutable pre47 inputs. Never imports project/runtime or simulates."""
import hashlib,json,pathlib
W=pathlib.Path('/Users/taytrrip/slug');OUT=pathlib.Path(__file__).resolve().parent
sha=lambda b:hashlib.sha256(b).hexdigest()
def J(x):return json.dumps(x,ensure_ascii=False,separators=(',',':'),allow_nan=False).replace('&','\\u0026').replace('<','\\u003c').replace('>','\\u003e').encode()
index={}
def put(rel,b,origin,original,pointer=None):
 p=OUT/rel;p.parent.mkdir(parents=True,exist_ok=True)
 if p.exists():assert p.read_bytes()==b,('existing input differs',rel)
 else:p.write_bytes(b)
 index[rel]=dict(origin=str(origin),origin_sha256=sha(original),json_pointer=pointer,extraction='byte_exact' if pointer is None else 'canonical_JSON_without_LF',sha256=sha(b),bytes=len(b))
def copy(rel,origin,pin=None):
 p=W/origin;b=p.read_bytes()
 if pin:assert sha(b)==pin,(origin,sha(b))
 put(rel,b,p,b)
def extract(rel,origin,pointer,pin=None):
 p=W/origin;b=p.read_bytes()
 if pin:assert sha(b)==pin,(origin,sha(b))
 x=json.loads(b)
 for k in pointer.strip('/').split('/'):
  x=x[int(k)] if isinstance(x,list) else x[k]
 put(rel,J(x),p,b,pointer)
freeze=json.loads((W/'set47-contract-freeze.json').read_bytes())
assert sha((W/'set47-contract-freeze.json').read_bytes())=='e023ddb4c043ae09b6edd826e0fbd1765d3e8bd02c34bcf0ce0ed10205d8c2fe'
for n,h in freeze['contract_sha256'].items():copy('inputs/contracts/'+n,n,h)
for n in ['set47-contract-freeze.json','set47-contract-bounds-formula.json','set47-reference-plan.md']:
 copy('inputs/contracts/'+n,n)
copy('inputs/engine42/corpus-freeze.json','set42-fight-reference/corpus-freeze.json')
ef=json.loads((W/'set42-fight-reference/corpus-freeze.json').read_bytes())
for source,target in [('reference_bout.py','inputs/engine42/reference_bout.py'),('inputs/manifest/reference_manifest.py','inputs/manifest/reference_manifest.py'),('inputs/manifest/tables_v1.json','inputs/manifest/tables_v1.json'),('inputs/manifest/baseline/reference_dna.py','inputs/manifest/baseline/reference_dna.py'),('inputs/manifest/baseline/dna_v1.json','inputs/manifest/baseline/dna_v1.json'),('inputs/condition/reference_condition.py','inputs/condition/reference_condition.py')]:
 copy(target,'set42-fight-reference/'+source,ef['files'][source])
old='set42-fight-reference/bout_v1.json';pin=ef['files']['bout_v1.json']
# Frozen old positive controls, chosen by method coverage, no new seed search.
for i in [0,2,5,6,52,53,54]:extract('inputs/engine42/case-%02d.json'%i,old,'/cases/%d'%i,pin)
extract('inputs/engine42/primitive.json',old,'/primitive',pin)
copy('inputs/card46/freeze.json','set46-card-reference/freeze.json','895bf574fd1a3de5fbb213bbbc84ca27b64e7c6790585000c4d7b117ea2563f1')
copy('inputs/card46/file_index.json','set46-card-reference/file_index.json')
ci=json.loads((W/'set46-card-reference/file_index.json').read_bytes())
for source,target in [('common.py','inputs/card46/common.py'),('inputs/seed45/common.py','inputs/calendar/common.py'),('inputs/source44/America_Los_Angeles.tzif','inputs/calendar/America_Los_Angeles.tzif'),('inputs/contracts/CANDIDATE_CARD_EXCEPTION.md','inputs/contracts/CANDIDATE_CARD_EXCEPTION.md'),('inputs/contracts/set46-candidate-exception-approval.json','inputs/contracts/set46-candidate-exception-approval.json')]:
 copy(target,'set46-card-reference/'+source,ci['files'][source]['sha256'])
cv='set46-card-reference/card_v1.json';ch=ci['corpus_sha256']
for name,pointer in [('pin','/pin'),('rules','/rules'),('views','/views'),('source','/dtos/6/value'),('record','/dtos/11/value'),('absent_card_state','/dtos/9/value')]:extract('inputs/card46/'+name+'.json',cv,pointer,ch)
copy('inputs/card46/modeled-five-class-seed.json','set46-card-reference/inputs/seed45/Record-five_class_synthetic.json',ci['files']['inputs/seed45/Record-five_class_synthetic.json']['sha256'])
actual='artifacts/set46/card-testclock-626145732/public-card-result.json';ah='b4349aab352a8e33400cf6d03e1c57beb680f72add2fcc18cbccd8ba37f17282'
for name,pointer in [('card_record','/build/record'),('initial_status','/build/status'),('conflict_status','/read/status'),('snapshot','/snapshot')]:extract('inputs/actual46/'+name+'.json',actual,pointer,ah)
replay='artifacts/set46/card-native-replay-2410442048/public-result.json';rh='fe650c9b6f0e89cbf10c7d97d88fbbfe75c61758d288fbbbb65e6681ad42a3a2'
for i in range(2):extract('inputs/actual46/view-%d.json'%i,replay,'/rows/%d/snapshot/view'%i,rh)
# Public schema5 evidence is retained for identity/provenance, no new proof run.
for i in range(3):extract('inputs/actual46/admission-%d.json'%i,replay,'/admission_receipts/%d'%i,rh)
for name,src,pin in [('chain-report.json','artifacts/set46/independent-attempt1/chain-report.json','1080c41658b65186049c874c6cd3ab367e3566a35c3c8b89df43753146472fd9'),('card-report.json','artifacts/set46/independent-attempt1/card-report.json','dd0ade793b96661fca6b12f86a99b9380d7dd5db30f14636ebfcd0e20698518a'),('admission-proof-addendum.json','set46-admission-proof-addendum.json','eb493df2d23e33038917ebe27f83fd6db07680dd322123ca9107b74939afa8f1')]:copy('inputs/actual46/'+name,src,pin)
b=(json.dumps(index,indent=2)+'\n').encode();p=OUT/'input_index.json'
if p.exists():assert p.read_bytes()==b,'input index differs'
else:p.write_bytes(b)
print(json.dumps(dict(status='passed',files=len(index),bytes=sum(m['bytes']for m in index.values()),input_index_sha256=sha(b),engine_executed=False,new47_runtime_read=False)))
