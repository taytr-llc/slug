"""Authoring-only minimal extraction; original inputs are immutable and hash-pinned.
No project/runtime source or outputs are read. Ordinary consumers use input_index.
"""
from pathlib import Path
import hashlib,json,subprocess
ROOT=Path(__file__).resolve().parent;WORK=ROOT.parent
sha=lambda b:hashlib.sha256(b).hexdigest()
J=lambda x:json.dumps(x,ensure_ascii=True,separators=(',',':'),allow_nan=False).encode()
P=lambda x:(json.dumps(x,indent=2,ensure_ascii=True)+'\n').encode()
index={}
def put(dest,origin,data=None,pointer=None,expected=None):
 b=origin.read_bytes();assert expected is None or sha(b)==expected,(origin,sha(b))
 v=b if data is None else data;p=ROOT/dest;p.parent.mkdir(parents=True,exist_ok=True)
 if p.exists():assert p.read_bytes()==v,p
 else:p.write_bytes(v)
 index[dest]={'origin':str(origin),'origin_sha256':sha(b),'json_pointer':pointer,'extraction':'byte_exact' if data is None else 'canonical_JSON_without_LF','sha256':sha(v),'bytes':len(v)}
for name,h in [('set46-card-core-contract-draft.md','0280887a3a5b1d20862253760bc655649b7bad23dec20c8fe523a1463436bbee'),('set46-helper-contract-draft.md','394747421cd8ca39cc0f6f255c709178be4e324ab9d7a927bf67e2696df47b5a'),('set46-contract-freeze.json','c5f992094df47de09e15a882fcba3cff47b295078ba8506e2200134517ae1ad0'),('set46-candidate-exception-approval.json','14c6074ed36351f979bf8f3e9c7e456489155e1a5ab3b56f909ae1998bccd007'),('set46-contract-bounds-formula.json','a82b6be80d1d35c43b0a9084db3fc5619d5bdd9dc24e3bfd721b3abba5b9916e')]:put('inputs/contracts/'+name,WORK/name,expected=h)
put('inputs/contracts/CANDIDATE_CARD_EXCEPTION.md',WORK/'slug/CANDIDATE_CARD_EXCEPTION.md',expected='30e5c6ec200797a6c6012f9437a13cf9f2e0379e3b5e561b0e93300d21ff9f56')
OLD=WORK/'set45-seed-reference';put('inputs/seed45/freeze.json',OLD/'freeze.json',expected='decbef0a5a69aac9827817994307b65a82f4a0daa20e654574072fe3f480b5db')
put('inputs/seed45/file_index.json',OLD/'file_index.json',expected='246411a21b609f19f4881c3693130b7cd94d0751357405e07d2906297c44babc')
old_index=json.loads((OLD/'file_index.json').read_bytes())['files']
p=OLD/'seed_v1.json';b=p.read_bytes();assert sha(b)=='2b8d78e8ef665e372543eb87bb5d25a975026a0ae946b15a871ac97bbf2153f0';c=json.loads(b)
for kind,names in [('Pin',['candidate']),('Record',['prepared','finalized','conflict','empty','five_class_synthetic']),('Status',['prepared','finalized','conflict','absent_due_no_allocation'])]:
 for name in names:
  xs=[(i,x) for i,x in enumerate(c['dtos']) if x['kind']==kind and x['name']==name];assert len(xs)==1;i,x=xs[0];assert J(x['value']).decode()==x['canonical'];put('inputs/seed45/'+kind+'-'+name+'.json',p,J(x['value']),'/dtos/%d/value'%i)
for key in ['fixture_keys','crypto']:
 # Only signature fixture metadata needed; crypto is too broad and intentionally omitted.
 if key=='fixture_keys':put('inputs/seed45/fixture_keys.json',p,J(c[key]),'/'+key)
for name in ['index.json','pin.json','views.json','receipt-0.json','receipt-1.json','receipt-10.json','America_Los_Angeles.tzif']:
 put('inputs/source44/'+name,OLD/'inputs/source44'/name)
for name in ['crypto.py','common.py']:
 put('inputs/seed45/'+name,OLD/name)
put('inputs/seed45/normal_work.json',OLD/'fixtures/normal_work.json')
# Original normal-work evidence is reused, not recomputed or claimed a consensus chain.
put('inputs/seed45/verification.json',OLD/'verification.json')
MAN=WORK/'fighter-manifest-reference'
for name in ['reference_manifest.py','tables_v1.json','baseline/reference_dna.py','baseline/dna_v1.json']:
 put('inputs/manifest/'+name,MAN/name)
put('inputs/condition/reference_condition.py',WORK/'fighter-condition-reference/reference_condition.py')
for dest,expected in {'inputs/manifest/reference_manifest.py':'9fb1a814047f2329066bb19596e910ac7c4c7aadfce4b8f994e02dab81e51b4f','inputs/manifest/tables_v1.json':'9f9380b708cae73b8d2641eaf04ecefb3a3b7fa9dd1b25b1f234f6511780b8c7','inputs/condition/reference_condition.py':'924152a89241e9fa6d60aa27ba19666d3a3f65fdf53bd3c0cf6f89c8db4e946c'}.items():assert index[dest]['sha256']==expected
for dest,m in index.items():
 origin=Path(m['origin'])
 if origin.is_relative_to(OLD) and str(origin.relative_to(OLD)) in old_index:assert m['origin_sha256']==old_index[str(origin.relative_to(OLD))]['sha256']
# Read only an exact pre46 committed encoder, never the working-tree runtime.
commit='ca84a62d178386646aef119cbe2edbb6ad38f508';oldpath='gymseed/json.go'
b=subprocess.check_output(['git','-C',str(WORK/'slug'),'show',commit+':'+oldpath]);assert sha(b)=='379406059c507139ca63baa27cc567b4bce55a77ca0121e97438350778350eb7'
dest='inputs/encoding/pre46-gymseed-json.go';p=ROOT/dest;p.parent.mkdir(parents=True,exist_ok=True)
if p.exists():assert p.read_bytes()==b
else:p.write_bytes(b)
index[dest]=dict(origin='git:'+str(WORK/'slug')+'@'+commit+':'+oldpath,origin_sha256=sha(b),json_pointer=None,extraction='byte_exact_committed_blob',sha256=sha(b),bytes=len(b))
(ROOT/'input_index.json').write_bytes(P(index))
print('minimal pinned inputs',len(index),'bytes',sum(v['bytes'] for v in index.values()),'index',sha(P(index)))
