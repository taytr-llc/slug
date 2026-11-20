"""Independent Set47 recipes; imports only exact pinned pre47 Python sources."""
import ast,copy,hashlib,importlib.util,json,pathlib,re,struct,sys,types
sys.dont_write_bytecode=True
HERE=pathlib.Path(__file__).resolve().parent
CORE='f73db38d7860b6fba7d57370fbbb58ad24377082a253ad234e57a22385f0997b'
HELPER='eabdf5e294fe26b6acb72ac67ff3c87b4764a366422414a1c202f8fa6622b691'
sha=lambda b:hashlib.sha256(b).hexdigest();clone=copy.deepcopy;raw=bytes.fromhex
u32=lambda n:struct.pack('<I',n);lp=lambda b:u32(len(b))+b
domain=lambda s:('SLUG '+s+'\0v1\0').encode('ascii')
label=lambda s:sha(('Set47 independent modeled '+s).encode('ascii'))
def J(x):return json.dumps(x,ensure_ascii=False,separators=(',',':'),allow_nan=False).replace('&','\\u0026').replace('<','\\u003c').replace('>','\\u003e').encode()
pretty=lambda x:(json.dumps(x,ensure_ascii=True,indent=2,allow_nan=False)+'\n').encode('ascii')
DATE_MAX=8640000000000000;SAFE=2**53-1;I32=2**31-1;WEEK=604800000
INDEX=json.loads((HERE/'input_index.json').read_bytes())
for p,m in INDEX.items():
 b=(HERE/p).read_bytes();assert sha(b)==m['sha256'] and len(b)==m['bytes'],p
assert INDEX['inputs/contracts/set47-core-contract-draft.md']['sha256']==CORE
assert INDEX['inputs/contracts/set47-helper-contract-draft.md']['sha256']==HELPER
def load(p):return json.loads((HERE/p).read_bytes())
def module(name,p):
 s=importlib.util.spec_from_file_location(name,HERE/p);m=importlib.util.module_from_spec(s);sys.modules[name]=m;s.loader.exec_module(m);return m
MANIFEST=module('bout47_frozen_manifest','inputs/manifest/reference_manifest.py');DNA=MANIFEST.DNA
EXTRACTIONS={}
def functions(p,names,ns):
 b=(HERE/p).read_bytes();tree=ast.parse(b);nodes=[n for n in tree.body if isinstance(n,(ast.FunctionDef,ast.ClassDef))and n.name in names]
 assert {n.name for n in nodes}==set(names),p
 selected=b''.join(lp(n.name.encode('ascii'))+lp(ast.get_source_segment(b.decode('utf-8'),n).encode('utf-8'))for n in nodes)
 EXTRACTIONS[p]=dict(source_sha256=sha(b),names=sorted(names),selected_source_sha256=sha(selected),selection_encoding='source-order LE32-length/name + LE32-length/exact-source-segment; AST allowlist only')
 exec(compile(ast.Module(body=nodes,type_ignores=[]),p,'exec'),ns)
cond={'json':json,'re':re,'DNA':DNA,'VERSION':1,'PROJECTOR':'slug-fighter-condition-candidate-v1','WEEK_MS':WEEK,'INT64_MAX':2**63-1,'ATTRIBUTES':('power','speed','chin','stamina','skill'),'LOSS':dict(power=20,speed=40,chin=30,stamina=30,skill=10),'RATING':dict(power=10,speed=25,chin=2,stamina=4,skill=8)}
functions('inputs/condition/reference_condition.py',{'round_even','aged_tenths','validate_dna','project'},cond)
CONDITION=types.SimpleNamespace(project=cond['project']);condition=cond['project']
# Literal constant nodes plus the explicitly named pure functions/classes only.
ENGINE_NAMES={'canonical','sha','R','mul','div','clamp','integer','hid','validate_input','new_input','strict_json','parse_input','input_id','stream_message','Streams','uniform_words','initial','state','snapshots','_tick','_recover','scores','simulate','digest','parse_result'}
ENGINE_CONSTANTS={'Q','DATE_MAX','ENGINE','ATTRS','LABELS','INPUT_DOMAIN','RANDOM_DOMAIN','RESULT_DOMAIN','STRIKES','DAMAGE','MIX'}
e={'json':json,'re':re,'hashlib':hashlib,'MANIFEST':MANIFEST,'CONDITION':CONDITION}
b=(HERE/'inputs/engine42/reference_bout.py').read_bytes();tree=ast.parse(b);cn=[]
for n in tree.body:
 if isinstance(n,ast.Assign)and all(isinstance(t,ast.Name)and t.id in ENGINE_CONSTANTS for t in n.targets):cn.append(n)
assert {t.id for n in cn for t in n.targets}==ENGINE_CONSTANTS
exec(compile(ast.Module(body=cn,type_ignores=[]),'frozen42-literal-constants','exec'),e)
functions('inputs/engine42/reference_bout.py',ENGINE_NAMES,e);EXTRACTIONS['inputs/engine42/reference_bout.py']['constant_names']=sorted(ENGINE_CONSTANTS)
ENGINE=types.SimpleNamespace(**e)
cal={'re':re,'I32':I32}
functions('inputs/calendar/common.py',{'days','civil','dateparts','sunday','noon_ms','cutoff','deadline'},cal)
cutoff=cal['cutoff'];deadline=cal['deadline']
CLASSES=['light','welter','middle','light_heavy','heavy'];BASIS='candidate_zero_record_no_results_authority'
CARD_RULES=load('inputs/card46/rules.json');CARD_RULE_ID=CARD_RULES['id']
RULES=dict(version=1,scope='two_fixture_booked_candidate_bout',record_basis=BASIS,card_rules_id=CARD_RULE_ID,engine_version=1,engine_id=ENGINE.ENGINE,mode='standard',seed_kind='explicit',rounds=3,as_of='event_cutoff',seed='booked_bout_seed',pro_xp=0,training_skill=0,new_run_requires_usable_card=True,saved_replay_immutable=True)
RULE_ID=sha(domain('candidate bout rules')+lp(J(RULES)))
def card_store_id(p):return sha(domain('candidate card store')+lp(J(p)))
def candidate_pin(cp):return dict(version=1,source_store_id=card_store_id(cp),source_pin=clone(cp),rules_id=RULE_ID)
def store_id(p):return sha(domain('candidate bout store')+lp(J(p)))
def card_id(cp,event):return sha(domain('candidate card event')+raw(card_store_id(cp))+raw(event['event_id']))
def identities(cp,event,snapshot_digest,event_key,height,target_hash,cls,a,b):
 cid=card_id(cp,event)
 ip=domain('candidate card input')+raw(cid)+raw(snapshot_digest)+raw(event_key)+u32(height)+raw(target_hash)+raw(CARD_RULE_ID)
 iid=sha(ip);ps=sha(domain('candidate card seed')+raw(iid)+raw(target_hash))
 bp=domain('candidate card bout')+raw(iid)+u32(CLASSES.index(cls))+raw(a)+raw(b);bid=sha(bp)
 return dict(card_id=cid,input_id=iid,plan_seed=ps,bout=dict(bout_id=bid,**{'class':cls},corner_a=a,corner_b=b,seed=sha(domain('candidate card bout seed')+raw(ps)+raw(bid))),input_preimage=ip.hex(),bout_preimage=bp.hex())
def source_digest(s):return sha(domain('candidate booked bout source')+lp(J(s)))
def record_digest(r):return sha(domain('candidate bout record')+lp(J(r)))
def source_projection(cp,r,bout):
 src=r['source'];rec=src['source_receipt'];ps=rec['input']['participants'];corners=[]
 for pid in [bout['corner_a'],bout['corner_b']]:
  selected=[dict(owner=clone(p['owner']),member=clone(m))for p in ps for m in p['members']if m['assignment']['prospect_id']==pid];assert len(selected)==1;corners+=selected
 s=dict(version=1,store_id=card_store_id(cp),card_id=r['card_id'],card_record_digest=sha(domain('candidate card record')+lp(J(r))),card_source_digest=r['source_digest'],card_created_at_ms=r['created_at_ms'],event=clone(rec['input']['event']),snapshot_digest=rec['snapshot_digest'],seed_event_key=src['event_key'],target_height=src['target']['height'],target_hash=src['target']['hash'],finalized_at_ms=src['finality']['observed_at_ms'],bout=clone(bout),corners=corners)
 return s
def derive_input(s):
 fs=[]
 for c in s['corners']:
  m=c['member'];a=m['assignment'];man=MANIFEST.generate(a['work_hash'])
  assert sha(MANIFEST.DIGEST_DOMAIN+MANIFEST.canonical(man))==a['manifest_digest']
  assert man['physical']['weight_class']==m['class']==s['bout']['class']
  assert condition(1,man['dna'],a['born_at_ms'],s['event']['cutoff_ms'],0)==m['condition_at_cutoff']
  assert m['eligibility']=='eligible'and m['condition_at_cutoff']['phase']!='retired'
  fs.append(dict(prospect_id=a['prospect_id'],manifest=man,born_at_ms=a['born_at_ms'],pro_xp=0,training_skill=0))
 assert [f['prospect_id']for f in fs]==[s['bout']['corner_a'],s['bout']['corner_b']]
 return ENGINE.new_input(s['event']['cutoff_ms'],s['bout']['seed'],*fs)
def make_record(pin,s,result,created):
 inp=derive_input(s);return dict(version=1,store_id=store_id(pin),card_id=s['card_id'],bout_id=s['bout']['bout_id'],source=clone(s),source_digest=source_digest(s),input=inp,input_id=ENGINE.input_id(inp),result=clone(result),result_digest=ENGINE.digest(result),created_at_ms=created)

def source_state(s,now,conflict=False,offline=False):
 return dict(version=1,card_id=s['card_id'],event=clone(s['event']),card_exists=True,card_record_digest=s['card_record_digest'],card_source_digest=s['card_source_digest'],card_created_at_ms=s['card_created_at_ms'],finalized_at_ms=s['finalized_at_ms'],conflict_count=int(conflict),conflict_overflow=False,incident_at_ms=0,escalation_due=now>=s['event']['incident_deadline_ms']and conflict,chain_observed_at_ms=s['finalized_at_ms'],target_descendants=60,current_finality=not(conflict or offline),node_state='offline'if offline else'online',observed_at_ms=now,usable=not conflict)
def saved(s,r=None,usable=True):
 return dict(bout=clone(s['bout']),source_digest=source_digest(s),exists=r is not None,record_digest=record_digest(r)if r else'',created_at_ms=r['created_at_ms']if r else 0,input_id=r['input_id']if r else'',result_digest=r['result_digest']if r else'',winner=r['result']['winner']if r else'',method=r['result']['method']if r else'',end_round=r['result']['end_round']if r else 0,end_tick=r['result']['end_tick']if r else 0,usable=bool(r and usable))
def status(pin,s,r,now,conflict=False):return dict(version=1,store_id=store_id(pin),card_id=s['card_id'],observed_at_ms=now,source=source_state(s,now,conflict),saved=saved(s,r,not conflict))
def event_for(cp,saturday):
 sp=cp['source_pin'];ep=sp['source_pin'];participant=ep['participants'][0];c=cutoff(saturday);n=deadline(saturday);u64=lambda v:struct.pack('<Q',v)
 pre=domain('candidate Saturday event')+raw(participant['namespace_id'])+raw(participant['calendar_id'])+raw(participant['rules_id'])+saturday.encode()+u64(c);eid=sha(pre)
 pre=domain('candidate cutoff event')+raw(sp['source_store_id'])+raw(eid)+u64(c)+u64(n)
 return dict(version=1,event_id=sha(pre),source_event_id=eid,store_id=sp['source_store_id'],namespace_id=participant['namespace_id'],calendar_id=participant['calendar_id'],rules_id=ep['rules_id'],saturday=saturday,cutoff_ms=c,incident_deadline_ms=n)
def event_key_for(cp,ev):
 sp=cp['source_pin'];return sha(domain('candidate seed event')+u32(sp['network'])+raw(sp['genesis'])+raw(sp['source_store_id'])+raw(ev['event_id'])+raw(sp['publisher_public_key']))

def composition_sources():
 actual=load('inputs/actual46/card_record.json');snap=load('inputs/actual46/snapshot.json');cp=snap['pin'];rpt=load('inputs/actual46/card-report.json');ad=load('inputs/actual46/admission-proof-addendum.json')
 assert rpt['status']==ad['status']=='passed'and ad['all_entered_admissions_independently_verified']
 assert rpt['input_sha256']==ad['baseline_input_sha256']==INDEX['inputs/actual46/card_record.json']['origin_sha256']
 assert ad['input_sha256']==INDEX['inputs/actual46/admission-0.json']['origin_sha256']
 assert card_store_id(cp)==actual['store_id']==rpt['card_store_id']
 assert sha(domain('candidate card record')+lp(J(actual)))==rpt['record_digest']
 assert len(actual['plan']['bouts'])==1
 s=source_projection(cp,actual,actual['plan']['bouts'][0])
 out=[dict(name='genuine_set46_middle',provenance='genuine_public_fixture_admissions_and_chain_independently_verified; original_booking_now_conflicted; expected_transcript_not_executed_authority',card_pin=cp,pin=candidate_pin(cp),source=s,source_store_created_at_ms=snap['created_at_ms'],store_created_at_ms=s['card_created_at_ms']+1,run_at_ms=s['card_created_at_ms']+2,views=[load('inputs/actual46/view-%d.json'%i)for i in range(2)])]
 # Five independently generated physical classes already existed in frozen45.
 # Distinct synthetic prospect IDs are explicit fixture values, never admitted.
 base=load('inputs/card46/modeled-five-class-seed.json');cp=load('inputs/card46/pin.json')['value'];event=clone(base['source_receipt']['input']['event']);c=event['cutoff_ms'];ps=base['source_receipt']['input']['participants'];models={}
 for order in [0,1]:
  for ci,cls in enumerate(CLASSES):
   corners=[]
   for corner in [0,1]:
    p=ps[corner^order];m=clone(next(m for m in p['members']if m['class']==cls));a=m['assignment'];man=MANIFEST.generate(a['work_hash'])
    a['prospect_id']=('%064x'%(1+ci*2+corner));a['gym_id']=p['owner']['gym_id']
    # Order0 prime; order1 late decline, still eligible at the fixed cutoff.
    age=0 if order==0 else man['dna']['prime_weeks']+man['dna']['decline_weeks']-1
    a['born_at_ms']=c-age*WEEK-2;a['accepted_at_ms']=a['born_at_ms']+1
    a['source_record_digest']=label('unadmitted assignment/'+str(order)+'/'+cls+'/'+str(corner))
    m['condition_at_cutoff']=condition(1,man['dna'],a['born_at_ms'],c,0);m['slot']=ci+1;m['slot_assigned_at_ms']=c-1;m['eligibility']='eligible'
    corners.append(dict(owner=clone(p['owner']),member=m))
   sn=label('modeled snapshot/'+str(order));ek=event_key_for(cp,event);h=114;th=label('modeled target/'+str(order));a,b=[z['member']['assignment']['prospect_id']for z in corners]
   ids=identities(cp,event,sn,ek,h,th,cls,a,b)
   s=dict(version=1,store_id=card_store_id(cp),card_id=ids['card_id'],card_record_digest=label('modeled card record/'+str(order)),card_source_digest=label('modeled omitted card source/'+str(order)),card_created_at_ms=c+1000,event=clone(event),snapshot_digest=sn,seed_event_key=ek,target_height=h,target_hash=th,finalized_at_ms=c+999,bout=ids['bout'],corners=corners)
   later=max(z['member']['assignment']['born_at_ms']+(MANIFEST.generate(z['member']['assignment']['work_hash'])['dna']['prime_weeks']+MANIFEST.generate(z['member']['assignment']['work_hash'])['dna']['decline_weeks'])*WEEK for z in corners)+1
   out.append(dict(name='modeled_'+cls+'_attribution_'+str(order),provenance='structural_model_only_no_admission_or_card_provenance; omitted_evidence_commitments_are_explicit_fixture_labels',card_pin=clone(cp),pin=candidate_pin(cp),source=s,source_store_created_at_ms=c+500,store_created_at_ms=c+1500,run_at_ms=max(c+2000,later)if order else c+2000,views=[clone(x['current_view'])for x in load('inputs/card46/views.json')]))
 return out
