"""Set46 independent input pins, exact integer recipes and inherited pure functions.
No project/runtime imports. AST subsets execute only the explicitly named frozen
calendar/condition functions; original source bytes stay intact in inputs.
"""
import ast,copy,hashlib,importlib.util,json,re,struct,sys
from pathlib import Path
sys.dont_write_bytecode=True
HERE=Path(__file__).resolve().parent
def J(v):
 # Established pre46 json.Marshal: HTML ASCII escaped, DEL literal, C0 escaped.
 s=json.dumps(v,ensure_ascii=False,separators=(',',':'),allow_nan=False)
 return s.replace('&','\\u0026').replace('<','\\u003c').replace('>','\\u003e').encode('utf-8')
pretty=lambda v:(json.dumps(v,ensure_ascii=True,indent=2,allow_nan=False)+'\n').encode('ascii')
sha=lambda b:hashlib.sha256(b).hexdigest()
clone=copy.deepcopy;raw=bytes.fromhex;u32=lambda n:struct.pack('<I',n);u64=lambda n:struct.pack('<Q',n)
lp=lambda b:u32(len(b))+b
domain=lambda s:('SLUG '+s+'\0v1\0').encode('ascii')
label=lambda s:sha(('Set46 independent modeled '+s).encode('ascii'))
SAFE=2**53-1;DATE_MAX=8640000000000000;I32=2**31-1;U32=2**32-1
INDEX=json.loads((HERE/'input_index.json').read_bytes())
for p,m in INDEX.items():
 b=(HERE/p).read_bytes();assert len(b)==m['bytes'] and sha(b)==m['sha256'],p
CORE='0280887a3a5b1d20862253760bc655649b7bad23dec20c8fe523a1463436bbee';HELPER='394747421cd8ca39cc0f6f255c709178be4e324ab9d7a927bf67e2696df47b5a'
assert INDEX['inputs/contracts/set46-card-core-contract-draft.md']['sha256']==CORE
assert INDEX['inputs/contracts/set46-helper-contract-draft.md']['sha256']==HELPER
def load(p):return json.loads((HERE/p).read_bytes())
def module(name,p):
 spec=importlib.util.spec_from_file_location(name,HERE/p);m=importlib.util.module_from_spec(spec);sys.modules[name]=m;spec.loader.exec_module(m);return m
CRYPTO=module('card_inherited_crypto','inputs/seed45/crypto.py')
MANIFEST=module('card_inherited_manifest','inputs/manifest/reference_manifest.py')
DNA=MANIFEST.DNA
SEED_PIN=load('inputs/seed45/Pin-candidate.json');SEED_ID=sha(domain('candidate seed store')+lp(J(SEED_PIN)))
SOURCE_PIN=load('inputs/source44/pin.json');SOURCE_ID=SEED_PIN['source_store_id'];NET=SEED_PIN['network'];GENESIS=SEED_PIN['genesis'];DECL_PUB=raw(SEED_PIN['publisher_public_key'])
VIEWS=load('inputs/source44/views.json');GYMS=sorted((p['stable_pin']['identity']['gym_id'],p['stable_pin']['account_id']) for p in SOURCE_PIN['participants']);GYM_ACCOUNT=dict(GYMS)
RECEIPTS=[load('inputs/source44/receipt-%d.json'%i) for i in [0,1,10]]
SEED_RECORDS={n:load('inputs/seed45/Record-'+n+'.json') for n in ['prepared','finalized','conflict','empty','five_class_synthetic']}
SEED_STATUSES={n:load('inputs/seed45/Status-'+n+'.json') for n in ['prepared','finalized','conflict','absent_due_no_allocation']}
# Deliberate exact source subset; no old generator/corpus entrypoint executes.
CAL_NAMES={'days','civil','dateparts','sunday','noon_ms','cutoff','deadline','event','event_pre','event_key'}
tree=ast.parse((HERE/'inputs/seed45/common.py').read_bytes());nodes=[n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name in CAL_NAMES];assert {n.name for n in nodes}==CAL_NAMES
exec(compile(ast.Module(body=nodes,type_ignores=[]),'pinned-calendar-functions','exec'),globals())
COND_NAMES={'round_even','aged_tenths','validate_dna','project'}
ns={'json':json,'re':re,'DNA':DNA,'VERSION':1,'PROJECTOR':'slug-fighter-condition-candidate-v1','WEEK_MS':604800000,'INT64_MAX':2**63-1,'ATTRIBUTES':('power','speed','chin','stamina','skill'),'LOSS':dict(power=20,speed=40,chin=30,stamina=30,skill=10),'RATING':dict(power=10,speed=25,chin=2,stamina=4,skill=8)}
tree=ast.parse((HERE/'inputs/condition/reference_condition.py').read_bytes());nodes=[n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name in COND_NAMES];assert {n.name for n in nodes}==COND_NAMES
exec(compile(ast.Module(body=nodes,type_ignores=[]),'pinned-condition-functions','exec'),ns);condition=ns['project']
CLASSES=['light','welter','middle','light_heavy','heavy'];BASIS='candidate_zero_record_no_results_authority'
REASONS=['retired_at_cutoff','not_rostered_at_cutoff','no_opponent_in_class','opponent_ineligible_at_cutoff','outside_rating_band']
RULES=dict(version=1,scope='two_fixture_candidate',record_basis=BASIS,classes=CLASSES,band_min_numerator=1,band_min_denominator=2,band_max_numerator=2,band_max_denominator=1,widen_to_five=False,condition_version=1,rating='D65',cutoff_eligibility=True,max_entrants=10,max_bouts=5)
RULE_PRE=domain('candidate card rules')+lp(J(RULES));RULE_ID=sha(RULE_PRE)
PIN=dict(version=1,source_store_id=SEED_ID,source_pin=SEED_PIN,rules_id=RULE_ID)
STORE_PRE=domain('candidate card store')+lp(J(PIN));STORE_ID=sha(STORE_PRE)
def card_pre(e):return domain('candidate card event')+raw(STORE_ID)+raw(e['event_id'])
def card_id(e):return sha(card_pre(e))
def source_digest(s):return sha(domain('candidate card source')+lp(J(s)))
def record_digest(r):return sha(domain('candidate card record')+lp(J(r)))
def source_projection(r):return dict(version=1,**{k:clone(r[k]) for k in ['store_id','event_key','source_receipt','declaration','anchor','target','finality']})
SOURCE=source_projection(SEED_RECORDS['finalized']);E=SOURCE['source_receipt']['input']['event'];C=E['cutoff_ms'];SOURCE_CREATED=C-1000;CREATED=C+2000

def input_pre(s):return domain('candidate card input')+raw(card_id(s['source_receipt']['input']['event']))+raw(s['source_receipt']['snapshot_digest'])+raw(s['event_key'])+u32(s['target']['height'])+raw(s['target']['hash'])+raw(RULE_ID)
def bout_pre(i,cls,a,b):return domain('candidate card bout')+raw(i)+u32(CLASSES.index(cls))+raw(a)+raw(b)
def source_inputs(s):
 out=[]
 for p in s['source_receipt']['input']['participants']:
  for m in p['members']:
   a=m['assignment'];manifest=MANIFEST.generate(a['work_hash'])
   assert sha(MANIFEST.DIGEST_DOMAIN+MANIFEST.canonical(manifest))==a['manifest_digest']
   assert manifest['physical']['weight_class']==m['class']
   assert condition(1,manifest['dna'],a['born_at_ms'],s['source_receipt']['input']['event']['cutoff_ms'],0)==m['condition_at_cutoff']
   out.append(dict(prospect_id=a['prospect_id'],gym_id=p['participant_id'],account_id=p['owner']['account_id'],display_name=manifest['name']['display'],class_=m['class'],rating_hundredths=m['condition_at_cutoff']['rating_hundredths'],cutoff_eligibility=m['eligibility']))
 return out

def band(a,b):
 if type(a)is not int or type(b)is not int or not 1<=a<=100000 or not 1<=b<=100000:raise ValueError('invalid_request')
 return 2*min(a,b)>=max(a,b)
def counts(es):
 reasons={k:sum(e['reason']==k for e in es) for k in REASONS}
 return dict(total_count=len(es),eligible_count=sum(e['cutoff_eligibility']=='eligible' for e in es),paired_count=sum(e['state']=='paired' for e in es),unmatched_count=sum(e['state']=='unmatched' for e in es),reasons=reasons)
def match(inputs,card,input_id,seed):
 """Pure local table; callers must label whether inputs came from a valid source."""
 if len(inputs)>10 or len({e['prospect_id'] for e in inputs})!=len(inputs):raise ValueError('invalid_request')
 if len({(e['gym_id'],e['class_']) for e in inputs})!=len(inputs):raise ValueError('invalid_request')
 for e in inputs:
  if e['gym_id'] not in GYM_ACCOUNT or e['account_id']!=GYM_ACCOUNT[e['gym_id']] or e['class_'] not in CLASSES or e['cutoff_eligibility'] not in ['eligible','retired','not_rostered'] or type(e['rating_hundredths']) is not int or not 0<=e['rating_hundredths']<=100000 or (e['cutoff_eligibility']=='eligible' and e['rating_hundredths']==0):raise ValueError('invalid_request')
 es=[];bs=[]
 for e in sorted(inputs,key=lambda x:(CLASSES.index(x['class_']),x['gym_id'],x['prospect_id'])):
  others=[x for x in inputs if x['class_']==e['class_'] and x['gym_id']!=e['gym_id']];o=others[0] if others else None;elig=e['cutoff_eligibility'];reason=''
  if elig=='retired':reason=REASONS[0]
  elif elig=='not_rostered':reason=REASONS[1]
  elif o is None:reason=REASONS[2]
  elif o['cutoff_eligibility']!='eligible':reason=REASONS[3]
  elif not band(e['rating_hundredths'],o['rating_hundredths']):reason=REASONS[4]
  bid=''
  if not reason:
   a,b=sorted([e['prospect_id'],o['prospect_id']]);bid=sha(bout_pre(input_id,e['class_'],a,b))
   if not any(x['bout_id']==bid for x in bs):bs.append(dict(bout_id=bid,class_=e['class_'],corner_a=a,corner_b=b,seed=sha(domain('candidate card bout seed')+raw(seed)+raw(bid))))
  row=dict(prospect_id=e['prospect_id'],gym_id=e['gym_id'],account_id=e['account_id'],display_name=e['display_name'],class_=e['class_'],rating_hundredths=e['rating_hundredths'],cutoff_eligibility=elig,professional_record=dict(wins=0,losses=0,draws=0),state='unmatched' if reason else 'paired',reason=reason,bout_id=bid)
  es.append({('class' if k=='class_' else k):v for k,v in row.items()})
 bs=[{('class' if k=='class_' else k):v for k,v in b.items()} for b in bs]
 return dict(version=1,card_id=card,input_id=input_id,seed=seed,record_basis=BASIS,counts=counts(es),bout_count=len(bs),gyms=[dict(gym_id=g,counts=counts([e for e in es if e['gym_id']==g])) for g,_ in GYMS],entrants=es,bouts=bs)
def derive(s):
 i=sha(input_pre(s));seed=sha(domain('candidate card seed')+raw(i)+raw(s['target']['hash']));return match(source_inputs(s),card_id(s['source_receipt']['input']['event']),i,seed)
def record(s=SOURCE,now=CREATED):return dict(version=1,store_id=STORE_ID,card_id=card_id(s['source_receipt']['input']['event']),source=clone(s),source_digest=source_digest(s),plan=derive(s),created_at_ms=now)
def state(kind='finalized',now=CREATED,s=SOURCE):
 st=clone(SEED_STATUSES[kind]);st['observed_at_ms']=now
 if st['finalized_at_ms']:st['finalized_at_ms']=s['finality']['observed_at_ms']
 st['escalation_due']=now>=st['event']['incident_deadline_ms'] and (not st['finalized_at_ms'] or bool(st['conflict_count']) or st['conflict_overflow'])
 return dict(version=1,status=st,source_digest=source_digest(s) if st['finalized_at_ms'] else '')
def status(r=None,source=None,now=CREATED):
 src=clone(source if source is not None else state(now=now));e=src['status']['event'];conf=src['status']['conflict_count'] or src['status']['conflict_overflow']
 return dict(version=1,store_id=STORE_ID,card_id=card_id(e),event=e,exists=r is not None,record_digest=record_digest(r) if r else '',created_at_ms=r['created_at_ms'] if r else 0,observed_at_ms=now,counts=clone(r['plan']['counts']) if r else counts([]),bout_count=r['plan']['bout_count'] if r else 0,source=src,usable=bool(r and src['source_digest']==r['source_digest'] and src['status']['finalized_at_ms'] and not conf))
def calendar_rows():
 out=[]
 for z in range(days(2026,1,1),days(2100,1,1)):
  if (z+4)%7==6:
   s='%04d-%02d-%02d'%civil(z);e=event(s);out.append(dict(saturday=s,event=e,card_id=card_id(e),card_preimage=card_pre(e).hex(),gap_hours=(e['incident_deadline_ms']-e['cutoff_ms'])//3600000))
 return out
