#!/usr/bin/env python3
"""Independent synthetic transport bindings, NEVER admitted/proof evidence."""
import argparse,collections,json,pathlib
import reference_bout as B
ROOT=pathlib.Path(__file__).resolve().parent
PINS={'preview_inputs/stable_v1.json':'29f6a1302c2e04ade612fe2775c085c65b8bf33587c010060b727833182d2057','preview_inputs/portraits_v1.json':'03df75cbf923a6dfc8fcac78e98d704641c0edf65ce563cf307f20a52f250670'}

def framed(domain,raw):return domain+len(raw).to_bytes(4,'little')+raw

def generate():
 for p,h in PINS.items():assert B.sha((ROOT/p).read_bytes())==h
 stable=json.loads((ROOT/'preview_inputs/stable_v1.json').read_bytes());portraits=json.loads((ROOT/'preview_inputs/portraits_v1.json').read_bytes());old=B.MANIFEST.generate
 examples=json.loads((ROOT/'inputs/manifest/manifests_v1.json').read_bytes())['cases'];byclass=collections.defaultdict(list)
 for x in examples:
  assert old(x['seed'])==x['manifest'];assert B.MANIFEST.digest_manifest(x['canonical'].encode())==x['digest']
  byclass[x['manifest']['physical']['weight_class']].append(x)
 imgs={x['seed']:x for x in portraits['cases']};out=[]
 for pi,p in enumerate(stable['pins']):
  pin=p['pin'];sid=p['expected_id'];assert B.sha(framed(b'SLUG candidate stable\0v1\0',B.canonical(pin)))==sid
  for phase in ('prime','retired'):
   candidates=byclass[('middle','welter')[pi]][:2];assert len(candidates)==2
   born=1700000000000;asof=born if phase=='prime' else born+max(x['manifest']['dna']['prime_weeks']+x['manifest']['dna']['decline_weeks'] for x in candidates)*604800000
   label=p['name']+'-'+phase;items=[];image_evidence=[]
   for n,x in enumerate(candidates,1):
    m=x['manifest'];im=imgs[x['seed']];assert im['manifest']==m and im['manifest_digest']==x['digest'];assert B.sha(b'SLUG fighter portrait SVG\0v1\0'+im['svg'].encode('ascii'))==im['svg_digest']
    pid=B.sha(b'SLUG synthetic preview transport prospect\0v1\0'+label.encode()+bytes([n]));source=B.sha(b'SLUG synthetic preview transport source record\0v1\0'+label.encode()+bytes([n]))
    assignment={'version':1,'sequence':n,'prospect_id':pid,'gym_id':pin['identity']['gym_id'],'work_hash':x['seed'],'born_at_ms':born,'accepted_at_ms':born,'descriptor_id':pin['source']['descriptor_id'],'manifest_digest':x['digest'],'portrait_digest':im['svg_digest'],'source_record_digest':source}
    cond=B.CONDITION.project(1,m['dna'],born,asof,0);items.append({'assignment':assignment,'manifest':m,'condition':cond});image_evidence.append({'prospect_id':pid,'svg':im['svg'],'portrait_digest':im['svg_digest']})
   items.sort(key=lambda x:x['assignment']['prospect_id']);status={'version':1,'store_id':sid,'pin':pin,'checkpoint':2,'observed_source_count':2,'caught_up':True,'roster_revision':0,'slots':['']*10,'operation_count':0,'pending_operation_id':'','last_operation_id':'','last_as_of_ms':asof};pair={'version':1,'status':status,'as_of_ms':asof,'fighters':items}
   bindings=[];conditions=[]
   for it in items:
    a=it['assignment'];raw=B.canonical(it['condition']);cd=B.sha(framed(b'SLUG fight preview condition\0v1\0',raw));bindings.append({'prospect_id':a['prospect_id'],'source_record_digest':a['source_record_digest'],'manifest_digest':a['manifest_digest'],'condition_digest':cd});conditions.append({'prospect_id':a['prospect_id'],'canonical':raw.decode(),'digest':cd})
   recipe={'version':1,'engine_version':1,'engine_id':B.ENGINE,'network':pin['source']['network'],'genesis':pin['source']['genesis'],'store_id':sid,'source_authority_id':pin['source']['authority_id'],'descriptor_id':pin['source']['descriptor_id'],'as_of_ms':asof,'rounds':3,'training':[0,0],'fighters':bindings};rawrecipe=B.canonical(recipe);seed=B.sha(framed(b'SLUG fight preview\0v1\0',rawrecipe));fs=[{'prospect_id':it['assignment']['prospect_id'],'manifest':it['manifest'],'born_at_ms':born,'pro_xp':0,'training_skill':0} for it in items];inp=B.new_input(asof,seed,*fs);result=B.simulate(inp)
   wrapper={'version':1,'pair':pair,'recipe':recipe,'seed':seed,'engine_result':result,'engine_result_digest':B.digest(result)}
   origin='http://127.0.0.1:18499';trust=B.sha(framed(b'SLUG manifest transport trust\0v1\0',origin.encode())+bytes([1])+(0).to_bytes(4,'little'))
   view={'version':1,'view_id':B.sha(b'SLUG synthetic preview view\0v1\0'+label.encode()),'account_authority_id':pin['account_authority_id'],'account_id':pin['account_id'],'generation':1,'authority_pin':pin['source'],'authority_origin':origin,'transport_trust_id':trust}
   job={'version':1,'job_id':B.sha(b'SLUG synthetic preview job\0v1\0'+label.encode()),'state':'done','view':view,'store_id':sid,'engine_version':1,'engine_id':B.ENGINE,'fighter_a_id':fs[0]['prospect_id'],'fighter_b_id':fs[1]['prospect_id'],'error':'','result':wrapper};response={'id':1,'version':1,'ok':True,'job':job}
   snapshot={'version':1,'state':'ready','error':'','view':view,'store_id':sid,'engine_version':1,'engine_id':B.ENGINE,'active_job_id':'','queued_job_id':'','last_job_id':job['job_id']}
   out.append({'name':label,'synthetic_transport_only':True,'status':status,'view':view,'snapshot_response':{'id':1,'version':1,'ok':True,'snapshot':snapshot},'response':response,'canonical_response':B.canonical(response).decode(),'canonical_input':B.canonical(inp).decode(),'input_id':B.input_id(inp),'canonical_recipe':rawrecipe.decode(),'seed':seed,'conditions':conditions,'engine_result_digest':B.digest(result),'portrait_evidence':image_evidence})
   assert len(B.canonical(response))+1<262144 and len(B.canonical(wrapper))<=229376
 return {'version':1,'scope':'synthetic transport bindings only; no source record/proof/admission/ownership claim','model_contract_sha256':B.CONTRACT,'integration_contract_sha256':B.PINS['inputs/set42-preview-integration-draft.md'],'input_sha256':{p:B.sha((ROOT/p).read_bytes()) for p in ('preview_inputs/stable_v1.json','preview_inputs/portraits_v1.json')},'cases':out}

if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--check',action='store_true');a=p.parse_args();raw=B.canonical(generate())+b'\n';target=ROOT/'preview_transport_v1.json'
 if a.check:assert target.read_bytes()==raw
 else:target.write_bytes(raw)
 print(json.dumps({'status':'passed','cases':4,'sha256':B.sha(raw),'bytes':len(raw)}))
