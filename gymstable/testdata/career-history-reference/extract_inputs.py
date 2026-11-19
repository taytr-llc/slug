#!/usr/bin/env python3
"""One-time exact public extraction, not history expected-output generation.
No private journal/key, runtime, network, engine, Argon or signing is accessed.
"""
from pathlib import Path
import hashlib, json
ROOT=Path(__file__).resolve().parent
W=ROOT.parent; R=W/'slug'; P=W/'artifacts/set49/career-chain-attempt3'
def sha(b): return hashlib.sha256(b).hexdigest()
def encode(v): return (json.dumps(v,ensure_ascii=False,indent=2)+'\n').encode()
def pointer(v,path):
    for k in path.strip('/').split('/') if path else []:
        v=v[int(k)] if isinstance(v,list) else v[k.replace('~1','/').replace('~0','~')]
    return v
rows={}
def put(name,data,origin,role,path=None):
    target=ROOT/name;target.parent.mkdir(parents=True,exist_ok=True)
    if target.exists(): assert target.read_bytes()==data, name+' already differs; preserve predecessor first'
    else: target.write_bytes(data)
    rows[name]={'sha256':sha(data),'bytes':len(data),'origin':str(origin),'origin_sha256':sha(origin.read_bytes()),'role':role}
    if path is not None: rows[name].update(json_pointer=path,extraction_encoding='json.dumps ensure_ascii=False indent=2 + LF; original insertion order preserved')
def copy(name,origin,role): put(name,origin.read_bytes(),origin,role)
def extract(name,origin,path,role): put(name,encode(pointer(json.loads(origin.read_bytes()),path)),origin,role,path)
for name in ['set51-fighter-history-contract.md','set51-history-contract-freeze.json','set51-history-contract-independent-review.json','set51-history-bound-independent-review.json','set51-history-reference-intake.md']:
    copy('inputs/contracts/'+name,W/name,'Frozen Set51 contract/review/provenance; not new output')
for name in ['CAREER_ACCOUNTING.md','CAREER_CONSUMERS.md','FIGHTER_CAREER.md','ADR_002_CAREER_FINALITY.md']:
    copy('inputs/contracts/'+name,R/name,'Existing approved prose')
prior=R/'internal/careerstate/testdata/reference'
for name in ['generate.py','corpus.json','input_index.json','README.md']:
    copy('inputs/prior-career/'+name,prior/name,'Frozen independent career equations/finite modeled oracle; no new Go input')
for p in sorted((prior/'inputs').iterdir()):
    if p.is_file(): copy('inputs/prior-career/inputs/'+p.name,p,'Transitive pinned old career producer input')
copy('inputs/prior-arithmetic.py',R/'fightercareer/testdata/reference/generate.py','Frozen independent standard arithmetic; history uses standard only')
for name in ['gymstable/types.go','gymstable/codec.go','desktop/pilot/manifest_types.go','desktop/pilot/stable_validation.go','desktop/pilot/manifest_validation.go','gymaccount/types.go','gymstable/audit.go','prospectservice/manifest_config.go','internal/careerstate/state.go']:
    src=R/name
    if src.exists(): copy('inputs/grammar/'+name.replace('/','__')+'.txt',src,'Existing inherited DTO/validation grammar only; not new Go outputs')
actual=W/'set49-actual-careers-attempt3.json';report=W/'artifacts/set49/independent-careers-attempt3/result-report.json'
a=json.loads(actual.read_bytes());q=json.loads(report.read_bytes());assert a['status']==q['status']=='passed'
for name in ['public-career-native-evidence.json','public-result-native-evidence.json','public-result-proof-finalized60.json','public-result-proof-conflict.json']:
    assert sha((P/name).read_bytes())==a['public_evidence_sha256'][name]
for field in ['status','scope','public_evidence_sha256','career_evidence_checked','career_rows','candidate_awarded_bouts','source_files_unchanged','original_tree_unchanged','automatic_admissions']:
    extract('inputs/extracted/actual-'+field+'.json',actual,'/'+field,'Historical Set49 runner provenance, not reexecution')
for field in ['status','scope','career','proofs','evidence_sha256','limitations','verifier_sha256','source_verifier_sha256']:
    if field in q: extract('inputs/extracted/independent-'+field+'.json',report,'/'+field,'Previously independent public proof/career verification provenance; not newly verified')
c=P/'public-career-native-evidence.json';native=P/'public-result-native-evidence.json'
for field in ['activation','finality','normal_pow','tagged_clock','automatic_admissions','scope','result_key']:
    extract('inputs/extracted/career-'+field+'.json',c,'/'+field,'Genuine previously executed candidate public evidence; scoped clock')
cv=json.loads(c.read_bytes())
for stage,i in [('provisional',0),('winner',4),('conflict',8),('held',9),('loser',10)]:
    row=cv['rows'][i];pid=row['prospect_id']
    for field in ['stage','awarded','expected_totals','held','prospect_id']:
        extract('inputs/extracted/'+stage+'-'+field+'.json',c,f'/rows/{i}/'+field,'Genuine public history phase/totals facts; not new source authority')
    extract('inputs/extracted/'+stage+'-view.json',c,f'/rows/{i}/view','Historical original public View (not a current session)')
    found=False
    for j,response in enumerate(row['inventory']):
        for k,item in enumerate(response['page']['source']['items']):
            if item['assignment']['prospect_id']==pid:
                base=f'/rows/{i}/inventory/{j}/page/source'
                extract('inputs/extracted/'+stage+'-status.json',c,base+'/status','Genuine captured Stable status, original source identity')
                extract('inputs/extracted/'+stage+'-assignment.json',c,base+f'/items/{k}/assignment','Genuine consumed assignment, previously verified public admission; no proof rerun')
                found=True;break
        if found: break
    assert found,stage
for name,key in [('finalized','finalized'),('conflict','conflict'),('provisional','first_inclusion')]:
    extract('inputs/extracted/result-'+name+'.json',native,'/'+key+'/record','Genuine retained public Result record; no new result/engine/chain validation')
index={'version':1,'status':'inputs_frozen_before_new_reference_math','ownership':{'checkpoint_design_audit':['common.py','history_math.py','reference_history.py','extract_inputs.py','input_index.json','generate.py','verify_reference.py','check_reproducibility.py','README.md','file_index.json','fragments/history.json','inputs/**'],'checkpoint_tool':['schema_wire.py','reference_wire.py','state_models.py','fragments/wire.json','fragments/state.json']},'inputs':rows,'origins_are_read_only':True,'limits':['No private database or secret read. Published fixture identities are historical public inputs.','Genuine one-bout finality reuses Set49 independent verification; new histories/IDs/pages are independently derived here.','Additional count/tie/capacity histories are modeled only; no new engine, proof, signing or mining.','No new runtime source or outputs are oracle inputs.'],'extractor_sha256':sha(Path(__file__).read_bytes())}
p=ROOT/'input_index.json';data=encode(index)
if p.exists(): assert p.read_bytes()==data,'input manifest differs; preserve first'
else:p.write_bytes(data)
print(json.dumps({'status':'extracted','inputs':len(rows),'bytes':sum(x['bytes'] for x in rows.values()),'input_index_sha256':sha(data)}))
