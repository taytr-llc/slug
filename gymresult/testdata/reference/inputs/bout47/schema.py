"""Contract-derived bounded codecs. Full core and cheap transport are distinct.
No source provenance is granted. Pin acceptance is the two retained fixture
installations; arbitrary input Pins are deliberately outside this finite oracle.
"""
from common import *
FIELDS={
'Rules':'version scope record_basis card_rules_id engine_version engine_id mode seed_kind rounds as_of seed pro_xp training_skill new_run_requires_usable_card saved_replay_immutable',
'BoutCorner':'owner member','BoutSource':'version store_id card_id card_record_digest card_source_digest card_created_at_ms event snapshot_digest seed_event_key target_height target_hash finalized_at_ms bout corners',
'BoutState':'version card_id event card_exists card_record_digest card_source_digest card_created_at_ms finalized_at_ms conflict_count conflict_overflow incident_at_ms escalation_due chain_observed_at_ms target_descendants current_finality node_state observed_at_ms usable',
'BoutRef':'bout source_digest','BoutView':'version store_id pin created_at_ms participant_account_id participant_gym_id observed_at_ms state bouts',
'Pin':'version source_store_id source_pin rules_id','Info':'version store_id pin source_created_at_ms created_at_ms bout_count',
'Record':'version store_id card_id bout_id source source_digest input input_id result result_digest created_at_ms',
'SavedBout':'bout source_digest exists record_digest created_at_ms input_id result_digest winner method end_round end_tick usable',
'Status':'version store_id card_id observed_at_ms source saved','CardStatus':'version store_id card_id observed_at_ms source bouts',
'RunResult':'version record status replayed','ReadResult':'version record status',
'Marker':'application version store_id pin','Metadata':'version store_id pin source_created_at_ms created_at_ms clock_ms bout_count','Init':'version store_id pin source_created_at_ms',
'Bout':'bout_id class corner_a corner_b seed',
'BoutRequest':'id command view_id saturday bout_id job_id',
'BoutResponse':'id ok view store_id snapshot card_status status record job error',
'BoutSnapshot':'version state error view store_id pin source_created_at_ms created_at_ms active_job_id queued_job_id last_job_id',
'BoutJob':'version job_id kind state view store_id card_id saturday bout_id error result',
'BoutPortrait':'version view store_id card_id saturday bout_id prospect_id record_digest input_id result_digest descriptor_id manifest_digest portrait_digest svg',
'BoutPortraitResponse':'version ok portrait status error'}
FIELDS={k:v.split()for k,v in FIELDS.items()}
CAPS=dict(Pin=24576,Info=28672,Marker=28672,Metadata=28672,Init=28672,BoutSource=16384,BoutCorner=8192,BoutState=4096,BoutView=32768,BoutRef=2048,Bout=2048,Input=16384,Result=196608,Record=237568,SavedBout=2048,Status=8192,CardStatus=16384,RunResult=249856,ReadResult=249856,BoutRequest=8192,BoutResponse=262144,BoutJob=253952,BoutSnapshot=73728,BoutPortrait=49152,BoutPortraitResponse=49152,Rules=4096)
ERRORS='invalid_request unauthorized forbidden stale_view busy outcome_unknown bout_unavailable bout_setup_incomplete bout_corrupt bout_capacity bout_clock bout_calendar bout_not_found bout_source_incomplete bout_source_conflict bout_source_changed bout_cancelled'.split()
METHODS='decision decision_draw ko tko_referee tko_three_knockdowns tko_corner double_corner_stoppage'.split()
SOURCES=composition_sources();CARD_PINS=[x['card_pin']for x in SOURCES[:2]];PINS=[candidate_pin(p)for p in CARD_PINS]
VIEWS=[x for s in [SOURCES[0],SOURCES[1]]for x in s['views']]
MEMBER_TEMPLATE=SOURCES[0]['source']['corners'][0]['member'];OWNER_TEMPLATE=SOURCES[0]['source']['corners'][0]['owner']
INPUT_TEMPLATE=json.loads(load('inputs/engine42/case-00.json')['canonical_input']);RESULT_TEMPLATE=json.loads(load('inputs/engine42/case-00.json')['canonical_result'])
class Refusal(ValueError):pass
def req(b,why='invalid_request'):
 if not b:raise Refusal(why)
def integer(v,lo=0,hi=SAFE):req(type(v)is int and lo<=v<=hi)
def boolean(v):req(type(v)is bool)
def string(v,lo=0,hi=512):req(type(v)is str and v.isascii()and lo<=len(v.encode('ascii'))<=hi)
def hx(v,empty=False):string(v,0,64);req((empty and v=='')or re.fullmatch('[0-9a-f]{64}',v)is not None)
def time(v,zero=False):integer(v,0 if zero else 1,DATE_MAX)
def enum(v,values):string(v);req(v in values)
def exact(v,keys):req(type(v)is dict and set(v)==set(keys));return {k:v[k]for k in keys}
def fields(k,v):
 v=exact(v,FIELDS[k])
 if 'version'in v:integer(v['version'],1,1)
 return v
def array(v,lo,hi):req(type(v)is list and lo<=len(v)<=hi)
def ordered(v,t,path='',signed=None):
 if type(t)is dict:return {k:ordered(exact(v,t)[k],t[k],path+'/'+k,signed)for k in t}
 if type(t)is list:
  array(v,len(t),len(t));return [ordered(x,t[i],path+'/'+str(i),signed)for i,x in enumerate(v)]
 req(type(v)is type(t))
 if type(v)is int:
  lo,hi=(signed or {}).get(path,(0,SAFE));integer(v,lo,hi)
  if path.endswith('_ms'):time(v,True)
 elif type(v)is str:string(v,0,8192)
 return v

def pin(v):
 for p in PINS:
  try:x=ordered(v,p);req(x==p);return x
  except Refusal:pass
 raise Refusal()
def cp(v):
 for p in CARD_PINS:
  try:x=ordered(v,p);req(x==p);return x
  except Refusal:pass
 raise Refusal()
def view(v):
 for t in VIEWS:
  try:
   x=ordered(v,t);hx(x['view_id']);integer(x['generation'],1);req(all(x[k]==t[k]for k in t if k not in ['view_id','generation']));return x
  except Refusal:pass
 raise Refusal()
def event(v,p):
 req(type(v)is dict and 'saturday'in v)
 try:e=event_for(p,v['saturday'])
 except(ValueError,TypeError,OverflowError):raise Refusal()
 req(ordered(v,e)==e);return e

def bout(v):
 v=fields('Bout',v)
 for k in ['bout_id','corner_a','corner_b','seed']:hx(v[k])
 enum(v['class'],CLASSES);req(v['corner_a']<v['corner_b']);return v

def condition_shape(c,a,ev):
 req(c['version']==c['dna_version']==1 and c['projector_id']=='slug-fighter-condition-candidate-v1'and c['seed']==a['work_hash']and c['born_at_ms']==a['born_at_ms']and c['as_of_ms']==ev['cutoff_ms']and c['pro_xp']==0 and c['phase']in ['prime','decline'])
 integer(c['age_weeks'],0,90);req(c['age_weeks']==(ev['cutoff_ms']-a['born_at_ms'])//WEEK)
 integer(c['elapsed_decline_weeks'],0,38);integer(c['prime_remaining_weeks'],0,52)
 integer(c['prime_remaining_ms'],0,52*WEEK);integer(c['career_remaining_ms'],1,91*WEEK)
 for k in ['aged_attributes_tenths','league_attributes_tenths']:
  for x in c[k].values():integer(x,1,1000)
 req(c['aged_attributes_tenths']==c['league_attributes_tenths'])
 req(c['rating_hundredths']==sum(cond['RATING'][k]*x for k,x in c['league_attributes_tenths'].items()))
 if c['phase']=='prime':req(c['elapsed_decline_weeks']==0 and c['prime_remaining_weeks']>0 and c['prime_remaining_ms']>0)
 else:req(c['prime_remaining_weeks']==c['prime_remaining_ms']==0)

def manifest_shape(m):
 """Inherited closed field bounds only, with no PRNG or generator invocation."""
 integer(m['manifest_version'],1,1);req(m['generator_id']==MANIFEST.GENERATOR and m['table_hash']==MANIFEST.TABLE_HASH)
 d=m['dna'];integer(d['dna_version'],1,1);hx(d['seed']);enum(d['rarity'],[t[1]for t in DNA.TIERS])
 tier=next(t for t in DNA.TIERS if t[1]==d['rarity']);integer(d['talent_budget'],tier[2],tier[3])
 for x in d['attributes'].values():integer(x,1,100)
 req(sum(d['attributes'].values())==d['talent_budget']);integer(d['prime_weeks'],26,52);integer(d['decline_weeks'],13,39)
 n=m['name'];string(n['first'],1,64);string(n['last'],1,64);string(n['display'],1,129);req(n['display']==n['first']+' '+n['last'])
 p=m['physical'];integer(p['height_in'],62,80);integer(p['reach_in'],60,84);integer(p['weight_lb'],128,268);req(-2<=p['reach_in']-p['height_in']<=4)
 enum(p['stance'],['orthodox','southpaw']);enum(p['weight_class'],CLASSES);req(p['weight_class']==CLASSES[sum(p['weight_lb']>b for b in [145,170,195,225])]);enum(m['style'],MANIFEST.TABLES['style']['priority'])
 p=m['portrait'];integer(p['portrait_version'],1,1)
 for k in ['skin','hair_style','hair_color','facial_hair','head_shape','mouth','brow','background']:integer(p[k],0,MANIFEST.LABEL_BOUNDS['portrait.'+k]-1)
 enum(p['scar'],['none','left_brow','right_cheek']);integer(p['shoulder_jitter'],-6,6);req(len(J(m))<=8192)

def corner(v,p,ev,context):
 v=fields('BoutCorner',v);o=ordered(v['owner'],OWNER_TEMPLATE);m=ordered(v['member'],MEMBER_TEMPLATE)
 req(o['ownership_basis']=='candidate_assignment');a=m['assignment'];req(a['version']==1);integer(a['sequence'],1,1000)
 for k in ['prospect_id','gym_id','work_hash','descriptor_id','manifest_digest','portrait_digest','source_record_digest']:hx(a[k])
 time(a['born_at_ms']);time(a['accepted_at_ms']);req(a['born_at_ms']<=a['accepted_at_ms']<=ev['cutoff_ms'])
 integer(m['slot'],1,10);time(m['slot_assigned_at_ms']);req(a['accepted_at_ms']<=m['slot_assigned_at_ms']<ev['cutoff_ms']);req(m['eligibility']=='eligible');enum(m['class'],CLASSES)
 matches=[ep['stable_pin']for ep in p['source_pin']['source_pin']['participants']if ep['stable_pin']['account_id']==o['account_id']]
 req(len(matches)==1);sp=matches[0];req(o['account_authority_id']==sp['account_authority_id']and o['gym_id']==sp['identity']['gym_id']==a['gym_id']and a['descriptor_id']==sp['source']['descriptor_id'])
 c=m['condition_at_cutoff'];condition_shape(c,a,ev)
 if context=='core':
  man=MANIFEST.generate(a['work_hash']);req(sha(MANIFEST.DIGEST_DOMAIN+MANIFEST.canonical(man))==a['manifest_digest']);req(man['physical']['weight_class']==m['class']);req(condition(1,man['dna'],a['born_at_ms'],ev['cutoff_ms'],0)==c)
 return dict(owner=o,member=m)
def source(v,p,context):
 v=fields('BoutSource',v);req(v['version']==1 and v['store_id']==card_store_id(p));v['event']=event(v['event'],p)
 for k in ['card_id','card_record_digest','card_source_digest','snapshot_digest','seed_event_key','target_hash']:hx(v[k])
 time(v['card_created_at_ms']);time(v['finalized_at_ms']);req(v['event']['cutoff_ms']<=v['finalized_at_ms']<=v['card_created_at_ms']);integer(v['target_height'],1,I32)
 v['bout']=bout(v['bout']);array(v['corners'],2,2);v['corners']=[corner(c,p,v['event'],context)for c in v['corners']]
 a,b=[c['member']['assignment']['prospect_id']for c in v['corners']];req([a,b]==[v['bout']['corner_a'],v['bout']['corner_b']]);req(v['corners'][0]['owner']['gym_id']!=v['corners'][1]['owner']['gym_id']);req(all(c['member']['class']==v['bout']['class']for c in v['corners']))
 ra,rb=[c['member']['condition_at_cutoff']['rating_hundredths']for c in v['corners']];req(2*min(ra,rb)>=max(ra,rb));req(v['seed_event_key']==event_key_for(p,v['event']))
 ids=identities(p,v['event'],v['snapshot_digest'],v['seed_event_key'],v['target_height'],v['target_hash'],v['bout']['class'],a,b);req(v['card_id']==ids['card_id']and v['bout']==ids['bout'])
 req(len(J(v))<=CAPS['BoutSource']);return v

def state(v,p):
 v=fields('BoutState',v);req(v['version']==1);v['event']=event(v['event'],p);req(v['card_id']==card_id(p,v['event']));time(v['observed_at_ms']);n=v['observed_at_ms']
 for k in ['card_exists','conflict_overflow','escalation_due','current_finality','usable']:boolean(v[k])
 for k in ['card_record_digest','card_source_digest']:hx(v[k],not v['card_exists'])
 for k in ['card_created_at_ms','finalized_at_ms','incident_at_ms','chain_observed_at_ms']:time(v[k],True);req(v[k]<=n)
 integer(v['conflict_count'],0,8);req(not v['conflict_overflow']or v['conflict_count']==8);integer(v['target_descendants'],0,I32);enum(v['node_state'],['unobserved','online','offline','unavailable'])
 req(v['escalation_due']==(n>=v['event']['incident_deadline_ms']and(not v['finalized_at_ms']or v['conflict_count']>0 or v['conflict_overflow'])))
 if v['incident_at_ms']:req(v['event']['incident_deadline_ms']<=v['incident_at_ms'])
 if not v['chain_observed_at_ms']:req(v['target_descendants']==0 and not v['current_finality'])
 if v['current_finality']:req(v['finalized_at_ms']>0 and v['chain_observed_at_ms']>0 and v['target_descendants']>=60 and not v['conflict_count']and not v['conflict_overflow'])
 if v['card_exists']:req(0<v['finalized_at_ms']<=v['card_created_at_ms']<=n)
 else:req(v['card_record_digest']==v['card_source_digest']==''and v['card_created_at_ms']==0 and not v['usable'])
 if v['usable']:req(v['card_exists']and v['finalized_at_ms']>0 and not v['conflict_count']and not v['conflict_overflow'])
 return v

def engine_input(v,s,context):
 v=ordered(v,INPUT_TEMPLATE,signed={'/a/manifest/portrait/shoulder_jitter':(-6,6),'/b/manifest/portrait/shoulder_jitter':(-6,6)})
 req(v['version']==1 and v['engine_id']==ENGINE.ENGINE and v['mode']=='standard'and v['seed_kind']=='explicit'and v['as_of_ms']==s['event']['cutoff_ms']and v['seed']==s['bout']['seed'])
 for k,c in zip(['a','b'],s['corners']):
  f=v[k];a=c['member']['assignment'];m=f['manifest'];manifest_shape(m);req(f['prospect_id']==a['prospect_id']and f['born_at_ms']==a['born_at_ms']and f['pro_xp']==f['training_skill']==0);req(m['dna']['seed']==a['work_hash']and m['physical']['weight_class']==s['bout']['class']);req(sha(MANIFEST.DIGEST_DOMAIN+J(m))==a['manifest_digest']);req(m['manifest_version']==1 and m['generator_id']=='slug-fighter-manifest-candidate-v1'and m['table_hash']==MANIFEST.TABLE_HASH)
 if context=='core':req(v==derive_input(s))
 req(len(J(v))<=CAPS['Input']);return v

def bounded_engine_state(v,init):
 integer(v['stamina_q'],0,init['max_stamina_q'])
 for k in ['damage_taken_q','damage_dealt_q']:integer(v[k],0,2700000000-1)
 for k in ['thrown','landed','significant','knockdowns']:integer(v[k],0,180)
 integer(v['control_q'],0,630000000);integer(v['round_knockdowns'],0,3)
 req(v['significant']<=v['landed']<=v['thrown'])
 return v

def engine_result(v,inp,context):
 v=exact(v,RESULT_TEMPLATE);integer(v['version'],1,1);req(v['engine_id']==ENGINE.ENGINE and v['input_id']==sha(ENGINE.INPUT_DOMAIN+J(inp)))
 array(v['initial'],2,2);v['initial']=[ordered(x,RESULT_TEMPLATE['initial'][i],signed={'/form_q':(-100000,100000)})for i,x in enumerate(v['initial'])]
 for x in v['initial']:
  integer(x['jitter_q'],920000,1080000);integer(x['max_stamina_q'],60000000,214000000);integer(x['ko_threshold_q'],64000000,130000000)
  for k,value in x['effective'].items():integer(value,1,130000000 if k=='skill'else 110000000)
 array(v['rounds'],1,3);rounds=[]
 for ri,r in enumerate(v['rounds']):
  r=exact(r,['number','ticks','end','recovery']);integer(r['number'],ri+1,ri+1);array(r['ticks'],1,60)
  r['ticks']=[ordered(t,RESULT_TEMPLATE['rounds'][0]['ticks'][0])for t in r['ticks']]
  for ti,t in enumerate(r['ticks']):
   integer(t['number'],ti+1,ti+1);integer(t['actor'],0,1);enum(t['strike'],ENGINE.STRIKES);enum(t['stoppage'],['','ko','tko_referee','tko_three_knockdowns']);req(t['hit']or(t['damage_q']==0 and not t['significant']and not t['knockdown']and t['stoppage']==''));req(not t['stoppage']or(ri==len(v['rounds'])-1 and ti==len(r['ticks'])-1))
   integer(t['damage_q'],0,8999999)
   for side,x in enumerate(t['states']):bounded_engine_state(x,v['initial'][side])
  r['end']=ordered(r['end'],RESULT_TEMPLATE['rounds'][0]['end']);req(r['end']==r['ticks'][-1]['states'])
  if r['recovery']is not None:
   r['recovery']=ordered(r['recovery'],RESULT_TEMPLATE['rounds'][0]['recovery']);req(ri<2 and len(r['ticks'])==60 and not r['ticks'][-1]['stoppage'])
   for side,x in enumerate(r['recovery']['states']):bounded_engine_state(x,v['initial'][side])
  if ri<len(v['rounds'])-1:req(r['recovery']is not None and not any(r['recovery']['corner_stopped'])and len(r['ticks'])==60)
  rounds.append(r)
 v['rounds']=rounds;v['final']=ordered(v['final'],RESULT_TEMPLATE['final']);req(v['final']==(rounds[-1]['recovery']['states']if rounds[-1]['recovery']else rounds[-1]['end']))
 v['scores']=ordered(v['scores'],RESULT_TEMPLATE['scores']);req(v['scores']==[ENGINE.scores(s)for s in v['final']]);v['draw_counts']=ordered(v['draw_counts'],RESULT_TEMPLATE['draw_counts']);req(sum(v['draw_counts'].values())<=65536)
 for k in ['form_a','form_b','chin_a','chin_b']:integer(v['draw_counts'][k],1,256)
 integer(v['draw_counts']['combat'],3*sum(len(r['ticks'])for r in rounds),65536)
 enum(v['method'],METHODS);hx(v['winner'],True);integer(v['end_round'],1,3);integer(v['end_tick'],1,60);req(v['end_round']==len(rounds)and v['end_tick']==len(rounds[-1]['ticks']));a,b=inp['a']['prospect_id'],inp['b']['prospect_id'];last=rounds[-1];m=v['method']
 if m in ['decision','decision_draw']:
  req(len(rounds)==3 and len(last['ticks'])==60 and last['recovery']is None and not last['ticks'][-1]['stoppage']);x,y=[s['total_tenths']for s in v['scores']];req(m==('decision_draw'if x==y else'decision')and v['winner']==(''if x==y else a if x>y else b))
 elif m in ['double_corner_stoppage','tko_corner']:
  req(last['recovery']is not None);st=last['recovery']['corner_stopped'];req(any(st));req(m==('double_corner_stoppage'if all(st)else'tko_corner'));req(v['winner']==(''if all(st)else[b,a][st.index(True)]))
 else:req(last['recovery']is None and last['ticks'][-1]['stoppage']==m and v['winner']==[a,b][last['ticks'][-1]['actor']])
 if context=='core':req(v==ENGINE.simulate(inp))
 req(len(J(v))<=CAPS['Result']);return v

def record(v,p,context):
 v=fields('Record',v);req(v['version']==1 and v['store_id']==store_id(p));v['source']=source(v['source'],p['source_pin'],context);s=v['source'];req(v['card_id']==s['card_id']and v['bout_id']==s['bout']['bout_id']and v['source_digest']==source_digest(s));v['input']=engine_input(v['input'],s,context);req(v['input_id']==sha(ENGINE.INPUT_DOMAIN+J(v['input'])));v['result']=engine_result(v['result'],v['input'],context);req(v['result_digest']==sha(ENGINE.RESULT_DOMAIN+J(v['result'])));time(v['created_at_ms']);req(v['created_at_ms']>=s['card_created_at_ms']);return v

def saved_bout(v):
 v=fields('SavedBout',v);v['bout']=bout(v['bout']);hx(v['source_digest']);boolean(v['exists']);boolean(v['usable'])
 for k in ['record_digest','input_id','result_digest']:hx(v[k],not v['exists'])
 hx(v['winner'],True);time(v['created_at_ms'],not v['exists']);integer(v['end_round'],0,3);integer(v['end_tick'],0,60)
 if not v['exists']:req(all(v[k]==''for k in ['record_digest','input_id','result_digest','winner','method'])and v['created_at_ms']==v['end_round']==v['end_tick']==0 and not v['usable'])
 else:
  enum(v['method'],METHODS);req(v['end_round']>0 and v['end_tick']>0);req(v['winner']in ['',v['bout']['corner_a'],v['bout']['corner_b']]);req((v['winner']=='')==(v['method']in ['decision_draw','double_corner_stoppage']))
 return v

def status_value(v,p,kind='Status'):
 v=fields(kind,v);req(v['version']==1 and v['store_id']==store_id(p));time(v['observed_at_ms']);v['source']=state(v['source'],p['source_pin']);s=v['source'];req(v['card_id']==s['card_id']and v['observed_at_ms']==s['observed_at_ms'])
 if kind=='Status':v['saved']=saved_bout(v['saved']);bs=[v['saved']];req(s['card_exists'])
 else:array(v['bouts'],0,5);v['bouts']=[saved_bout(b)for b in v['bouts']];bs=v['bouts'];req(s['card_exists']or not bs);req([CLASSES.index(b['bout']['class'])for b in bs]==sorted(set(CLASSES.index(b['bout']['class'])for b in bs)))
 req(len({b['bout']['bout_id']for b in bs})==len(bs));req(len({x for b in bs for x in [b['bout']['corner_a'],b['bout']['corner_b']]})==2*len(bs))
 for b in bs:
  if b['exists']:req(s['card_created_at_ms']<=b['created_at_ms']<=v['observed_at_ms'])
  if b['usable']:req(s['usable'])
 return v

def result(v,p,context,kind):
 v=fields(kind,v);req(v['version']==1);v['record']=record(v['record'],p,context);r=v['record'];v['status']=status_value(v['status'],p);st=v['status'];s=st['source'];req(st['card_id']==r['card_id']);req(st['saved']==saved(r['source'],r,s['usable']));req(s['card_exists']and all(s[k]==r['source'][k]for k in ['card_id','card_record_digest','card_source_digest','card_created_at_ms','event','finalized_at_ms']))
 if kind=='RunResult':boolean(v['replayed'])
 return v
def snapshot(v,p):
 v=fields('BoutSnapshot',v);req(v['version']==1);time(v['source_created_at_ms'],True);time(v['created_at_ms'],True);enum(v['state'],['ready','busy','setup_incomplete','unavailable','failed']);s=v['state'];string(v['error'])
 if s=='unavailable':req(v==dict(version=1,state='unavailable',error='bout_unavailable',view=None,store_id='',pin=None,source_created_at_ms=0,created_at_ms=0,active_job_id='',queued_job_id='',last_job_id=''));return v
 v['view']=view(v['view'])
 if s=='setup_incomplete':req(v['pin']is None and v['store_id']==v['active_job_id']==v['queued_job_id']==v['last_job_id']==''and v['source_created_at_ms']==v['created_at_ms']==0 and v['error']=='bout_setup_incomplete');return v
 v['pin']=pin(v['pin']);req(v['pin']==p and v['store_id']==store_id(p));time(v['source_created_at_ms']);time(v['created_at_ms']);req(v['source_created_at_ms']<=v['created_at_ms'])
 for k in ['active_job_id','queued_job_id','last_job_id']:hx(v[k],True)
 ids=[v[k]for k in ['active_job_id','queued_job_id','last_job_id']if v[k]];req(len(set(ids))==len(ids));req(not(v['active_job_id']and v['queued_job_id']));req(s=='busy'or not(v['active_job_id']or v['queued_job_id']))
 if s=='failed':enum(v['error'],['bout_unavailable','bout_corrupt','bout_clock','bout_source_changed'])
 else:req(v['error']=='')
 return v

def job(v,p,context):
 v=fields('BoutJob',v);req(v['version']==1 and v['kind']=='run'and v['store_id']==store_id(p));hx(v['job_id']);hx(v['bout_id']);v['view']=view(v['view']);ev=event_for(p['source_pin'],v['saturday']);req(v['card_id']==card_id(p['source_pin'],ev));enum(v['state'],['queued','running','completed','failed','cancelled'])
 if v['state']in ['queued','running']:req(v['result']is None and v['error']=='')
 elif v['state']=='completed':req(v['error']=='');v['result']=result(v['result'],p,context,'RunResult');req(v['result']['record']['bout_id']==v['bout_id']and v['result']['record']['card_id']==v['card_id']and v['result']['record']['source']['event']['saturday']==v['saturday'])
 else:req(v['result']is None);enum(v['error'],[e for e in ERRORS if e!='bout_setup_incomplete']);req(v['state']!='cancelled'or v['error']=='bout_cancelled');req(v['error']!='outcome_unknown'or v['state']=='failed')
 return v

def request(v):
 v=fields('BoutRequest',v);integer(v['id'],1)
 matrix={'bout_snapshot':[],'bout_card_status':['view_id','saturday'],'bout_status':['view_id','saturday','bout_id'],'bout_record':['view_id','saturday','bout_id'],'bout_run':['view_id','saturday','bout_id'],'bout_job':['view_id','job_id'],'bout_cancel':['view_id','job_id']}
 enum(v['command'],matrix)
 for k in ['view_id','saturday','bout_id','job_id']:
  if k not in matrix[v['command']]:req(v[k]=='')
  elif k=='saturday':cutoff(v[k])
  else:hx(v[k])
 return v

def response(v,p,context):
 v=fields('BoutResponse',v);integer(v['id'],1);boolean(v['ok']);string(v['error']);names=['snapshot','card_status','status','record','job'];present=[k for k in names if v[k]is not None]
 if not v['ok']:
  req(not present);enum(v['error'],ERRORS)
  if v['view']is None:req(v['store_id']=='')
  else:v['view']=view(v['view']);req(v['store_id']in ['',store_id(p)])
  return v
 req(v['error']==''and len(present)==1);k=present[0]
 if k=='snapshot':
  v[k]=snapshot(v[k],p);req(v['view']==v[k]['view']and v['store_id']==v[k]['store_id']);v['view']=clone(v[k]['view']);return v
 v['view']=view(v['view']);req(v['store_id']==store_id(p))
 if k=='card_status':v[k]=status_value(v[k],p,'CardStatus')
 elif k=='status':v[k]=status_value(v[k],p)
 elif k=='record':v[k]=result(v[k],p,context,'ReadResult')
 else:v[k]=job(v[k],p,context);req(v[k]['view']==v['view'])
 return v

def portrait(v,p):
 v=fields('BoutPortrait',v);req(v['version']==1 and v['store_id']==store_id(p));v['view']=view(v['view']);ev=event_for(p['source_pin'],v['saturday']);req(v['card_id']==card_id(p['source_pin'],ev))
 for k in ['bout_id','prospect_id','record_digest','input_id','result_digest','descriptor_id','manifest_digest','portrait_digest']:hx(v[k])
 string(v['svg'],1,32768);req(len(J(v['svg']))<=32768);return v

def validate(kind,v,p,context='core'):
 req(context in ['core','transport']);p=pin(p)
 if kind=='Rules':req(ordered(v,RULES)==RULES);return clone(RULES)
 if kind=='Pin':return pin(v)
 if kind=='Bout':return bout(v)
 if kind=='BoutSource':return source(v,p['source_pin'],context)
 if kind=='BoutState':return state(v,p['source_pin'])
 if kind=='BoutRef':v=fields(kind,v);v['bout']=bout(v['bout']);hx(v['source_digest']);return v
 if kind=='BoutView':
  v=fields(kind,v);v['pin']=cp(v['pin']);req(v['version']==1 and v['store_id']==card_store_id(p['source_pin'])and v['pin']==p['source_pin']);time(v['created_at_ms']);time(v['observed_at_ms']);v['state']=state(v['state'],p['source_pin']);req(v['observed_at_ms']==v['state']['observed_at_ms']>=v['created_at_ms']);array(v['bouts'],0,5);v['bouts']=[validate('BoutRef',b,p,context)for b in v['bouts']];req(v['state']['card_exists']or not v['bouts']);req([CLASSES.index(b['bout']['class'])for b in v['bouts']]==sorted(set(CLASSES.index(b['bout']['class'])for b in v['bouts'])));req(len({b['bout']['bout_id']for b in v['bouts']})==len(v['bouts']));req(len({x for b in v['bouts']for x in [b['bout']['corner_a'],b['bout']['corner_b']]})==2*len(v['bouts']));req(any(ep['stable_pin']['account_id']==v['participant_account_id']and ep['stable_pin']['identity']['gym_id']==v['participant_gym_id']for ep in p['source_pin']['source_pin']['source_pin']['participants']));return v
 if kind in ['Info','Marker','Metadata','Init']:
  v=fields(kind,v);v['pin']=pin(v['pin']);req(v['version']==1 and v['store_id']==store_id(p)and v['pin']==p)
  if kind=='Marker':req(v['application']=='SLUG candidate bouts');return v
  time(v['source_created_at_ms'])
  if kind=='Init':return v
  time(v['created_at_ms']);req(v['source_created_at_ms']<=v['created_at_ms']);integer(v['bout_count'],0,256)
  if kind=='Metadata':time(v['clock_ms']);req(v['clock_ms']>=v['created_at_ms'])
  return v
 if kind=='Record':return record(v,p,context)
 if kind=='SavedBout':return saved_bout(v)
 if kind in ['Status','CardStatus']:return status_value(v,p,kind)
 if kind in ['ReadResult','RunResult']:return result(v,p,context,kind)
 if kind=='BoutSnapshot':return snapshot(v,p)
 if kind=='BoutJob':return job(v,p,context)
 if kind=='BoutRequest':return request(v)
 if kind=='BoutResponse':return response(v,p,context)
 if kind=='BoutPortrait':return portrait(v,p)
 if kind=='BoutPortraitResponse':
  v=fields(kind,v);req(v['version']==1);boolean(v['ok'])
  if not v['ok']:req(v['portrait']is None and v['status']is None);enum(v['error'],[x for x in ERRORS if x!='bout_setup_incomplete']);return v
  req(v['error']=='');v['portrait']=portrait(v['portrait'],p);v['status']=status_value(v['status'],p);pt,st=v['portrait'],v['status'];sb=st['saved'];req(sb['exists']and pt['card_id']==st['card_id']and pt['saturday']==st['source']['event']['saturday']and pt['bout_id']==sb['bout']['bout_id']and pt['prospect_id']in [sb['bout']['corner_a'],sb['bout']['corner_b']]);req(all(pt[k]==sb[k]for k in ['record_digest','input_id','result_digest']));return v
 raise Refusal('unknown kind')

def parse(kind,b,p,context='core'):
 req(type(b)is bytes and 0<len(b)<=CAPS[kind]-(kind in ['BoutRequest','BoutResponse']));req(b.isascii())
 def pairs(xs):
  r={}
  for k,v in xs:string(k,1,64);req(k not in r);r[k]=v
  return r
 def bad(s):raise Refusal('number')
 def number(s):req(re.fullmatch('0|-?[1-9][0-9]*',s)is not None and len(s)<=17);return int(s)
 try:v=json.loads(b,object_pairs_hook=pairs,parse_int=number,parse_float=bad,parse_constant=bad)
 except(ValueError,UnicodeError,RecursionError):raise Refusal()
 def bound(x,path='',depth=1):
  if isinstance(x,(dict,list)):
   req(depth<=16)
   if type(x)is list:
    permitted=array_paths(kind)
    normalized=re.sub(r'/[0-9]+(?=/|$)','/*',path)
    req(normalized in permitted and len(x)<=permitted[normalized])
    for i,y in enumerate(x):bound(y,path+'/'+str(i),depth+1)
   else:
    req(len(x)<=32)
    for k,y in x.items():bound(y,path+'/'+k,depth+1)
  elif type(x)is int:
   if x<0:
    # Exact roots only: Record or Record under Run/Read, Job/Response wrappers.
    # Kind-specific root grammar; no suffix-based signed-field permission.
    prefix={'Record':[''],'RunResult':['/record'],'ReadResult':['/record'],'BoutJob':['/result/record'],'BoutResponse':['/record/record','/job/result/record']}.get(kind,[])
    allowed={z+'/input/'+side+'/manifest/portrait/shoulder_jitter':(-6,6)for z in prefix for side in ['a','b']}
    allowed.update({z+'/result/initial/'+str(i)+'/form_q':(-100000,100000)for z in prefix for i in range(2)})
    req(path in allowed);integer(x,*allowed[path])
   else:integer(x)
  elif type(x)is str:string(x,0,32768)
  else:req(x is None or type(x)is bool)
 bound(v)
 try:r=validate(kind,v,p,context)
 except(AssertionError,KeyError,IndexError,TypeError,ValueError,OverflowError):raise Refusal()
 req(len(J(r))<=CAPS[kind]-(kind in ['BoutRequest','BoutResponse']));return r

def array_paths(kind):
 """Exact typed root paths only; numeric components normalize to '*'."""
 paths={}
 def pin_at(z):paths[z+'/source_pin/source_pin/source_pin/participants']=2
 def record_at(z):
  paths[z+'/source/corners']=2
  paths[z+'/result/initial']=2;paths[z+'/result/rounds']=3
  paths[z+'/result/rounds/*/ticks']=60
  for q in ['/result/rounds/*/ticks/*/states','/result/rounds/*/end','/result/rounds/*/recovery/states','/result/rounds/*/recovery/corner_stopped','/result/final','/result/scores']:paths[z+q]=2
 if kind in ['Pin']:pin_at('')
 if kind in ['Info','Marker','Metadata','Init','BoutSnapshot']:pin_at('/pin')
 if kind=='BoutSource':paths['/corners']=2
 if kind=='BoutView':paths['/pin/source_pin/source_pin/participants']=2;paths['/bouts']=5
 if kind=='CardStatus':paths['/bouts']=5
 if kind=='Record':record_at('')
 if kind in ['RunResult','ReadResult']:record_at('/record')
 if kind=='BoutJob':record_at('/result/record')
 if kind=='BoutResponse':
  pin_at('/snapshot/pin');paths['/card_status/bouts']=5
  record_at('/record/record');record_at('/job/result/record')
 return paths

def correlate(q,r,p,current,source_created,created,expected_job=None,terminal_digest=''):
 q=request(q);r=response(r,p,'transport');req(r['id']==q['id'])
 if not r['ok']:
  if r['view']is not None:req(r['view']==current)
  return r
 k={'bout_snapshot':'snapshot','bout_card_status':'card_status','bout_status':'status','bout_record':'record','bout_run':'job','bout_job':'job','bout_cancel':'job'}[q['command']];req(r[k]is not None)
 if k=='snapshot':
  ss=r[k]
  if ss['state']!='unavailable':req(ss['view']==current)
  if ss['pin']is not None:req(ss['source_created_at_ms']==source_created and ss['created_at_ms']==created)
  return r
 req(q['view_id']==current['view_id']and r['view']==current and r['store_id']==store_id(p))
 if k=='job':
  j=r[k];req(j['view']==current)
  if q['command']=='bout_run':req(j['state']=='queued'and j['saturday']==q['saturday']and j['bout_id']==q['bout_id'])
  else:req(j['job_id']==q['job_id'])
  if expected_job:
   for f in ['job_id','kind','view','store_id','card_id','saturday','bout_id']:req(j[f]==expected_job[f])
   req(not(expected_job['state']=='running'and j['state']=='queued'))
   if expected_job['state']in ['completed','failed','cancelled']:req(J(j)==J(expected_job))
  if terminal_digest:req(sha(J(j))==terminal_digest)
  payload=j['result']
 else:
  payload=r[k]
  st=payload['status']if k=='record'else payload;req(st['source']['event']['saturday']==q['saturday'])
  if k!='card_status':req(st['saved']['bout']['bout_id']==q['bout_id'])
 if payload:
  st=payload['status']if k in ['record','job']else payload
  req(st['observed_at_ms']>=created)
  if st['source']['card_exists']:req(st['source']['card_created_at_ms']>=source_created)
  for sb in st['bouts']if k=='card_status'else[st['saved']]:
   if sb['exists']:req(sb['created_at_ms']>=created)
  if k in ['record','job']:req(payload['record']['created_at_ms']>=created and payload['record']['source']['card_created_at_ms']>=source_created)
 return r

def correlate_portrait(reply,p,current,selected_record,source_created,created):
 """Structural selected-tuple/fresh-status check only; does not authenticate SVG bytes."""
 v=validate('BoutPortraitResponse',reply,p,'transport')
 if not v['ok']:return v
 req(selected_record is not None);pt=v['portrait'];r=selected_record
 req(pt['view']==current and pt['card_id']==r['card_id']and pt['bout_id']==r['bout_id']and pt['saturday']==r['source']['event']['saturday'])
 req(pt['record_digest']==record_digest(r)and pt['input_id']==r['input_id']and pt['result_digest']==r['result_digest'])
 matches=[c['member']['assignment']for c in r['source']['corners']if c['member']['assignment']['prospect_id']==pt['prospect_id']];req(len(matches)==1)
 req(all(pt[k]==matches[0][k]for k in ['descriptor_id','manifest_digest','portrait_digest']))
 checked=result(dict(version=1,record=clone(r),status=clone(v['status'])),p,'transport','ReadResult')
 req(checked['record']['created_at_ms']>=created and checked['record']['source']['card_created_at_ms']>=source_created)
 return v
