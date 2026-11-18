"""Independent closed codecs. Context 'core' derives source/name/condition facts;
context 'transport' is the cheap pipe boundary, not a source-authority verifier.
Old receipt core acceptance is deliberately finite: the retained exact receipt,
not a claimed generic replacement for the frozen Set44 journal validator.
"""
from common import *
FIELDS={
'Rules':'version scope record_basis classes band_min_numerator band_min_denominator band_max_numerator band_max_denominator widen_to_five condition_version rating cutoff_eligibility max_entrants max_bouts',
'Pin':'version source_store_id source_pin rules_id',
'Info':'version store_id pin source_created_at_ms created_at_ms event_count',
'ProfessionalRecord':'wins losses draws','ReasonCounts':' '.join(REASONS),
'Counts':'total_count eligible_count paired_count unmatched_count reasons','GymCounts':'gym_id counts',
'Entrant':'prospect_id gym_id account_id display_name class rating_hundredths cutoff_eligibility professional_record state reason bout_id',
'Bout':'bout_id class corner_a corner_b seed','Plan':'version card_id input_id seed record_basis counts bout_count gyms entrants bouts',
'Record':'version store_id card_id source source_digest plan created_at_ms','Status':'version store_id card_id event exists record_digest created_at_ms observed_at_ms counts bout_count source usable',
'BuildResult':'version record status replayed','ReadResult':'version record status','HistoryPage':'version store_id after_card_id next_card_id has_more observed_at_ms cards',
'CardSource':'version store_id event_key source_receipt declaration anchor target finality',
'CardState':'version status source_digest','CardView':'version store_id pin created_at_ms participant_account_id participant_gym_id observed_at_ms states',
'Marker':'application version store_id pin','Metadata':'version store_id pin source_created_at_ms created_at_ms clock_ms event_count','Init':'version store_id pin source_created_at_ms',
'CardRequest':'id command view_id saturday after_card_id limit job_id','CardResponse':'id ok view store_id snapshot status record history job error',
'CardSnapshot':'version state error view store_id pin source_created_at_ms created_at_ms active_job_id queued_job_id last_job_id','CardJob':'version job_id kind state view store_id card_id saturday error result'}
FIELDS={k:v.split() for k,v in FIELDS.items()}
CAPS=dict(Pin=20480,Info=24576,Marker=24576,Metadata=24576,Init=24576,CardSource=163840,CardState=12288,CardView=122880,Plan=16384,Record=196608,Status=16384,HistoryPage=139264,BuildResult=229376,ReadResult=229376,CardRequest=8192,CardResponse=262144,CardJob=245760,CardSnapshot=49152)
ERRORS='invalid_request unauthorized forbidden stale_view busy outcome_unknown card_unavailable card_corrupt card_capacity card_clock card_calendar card_not_found card_source_incomplete card_source_conflict card_source_changed card_cancelled card_setup_incomplete'.split()
class Refusal(ValueError):pass
def req(b,why='invalid_request'):
 if not b:raise Refusal(why)
def integer(v,lo=0,hi=SAFE):req(type(v)is int and lo<=v<=hi)
def boolean(v):req(type(v)is bool)
def string(v):req(type(v)is str and v.isascii())
def enum(v,choices):string(v);req(v in choices)
def hx(v,n=64,empty=False):string(v);req(empty and v=='' or bool(re.fullmatch('[0-9a-f]{%d}'%n,v)))
def time(v,zero=False):integer(v,0 if zero else 1,DATE_MAX)
def exact(v,keys):req(type(v)is dict and set(v)==set(keys));return {k:v[k] for k in keys}
def array(v,n,minimum=0):req(type(v)is list and minimum<=len(v)<=n)
def ordered(v,t,path='',signed_paths=frozenset()):
 """Template keys/order with type-exact scalars; bounded finite old shapes."""
 if type(t)is dict:
  v=exact(v,t);return {k:ordered(v[k],t[k],path+'/'+k,signed_paths) for k in t}
 if type(t)is list:
  # Retained old template uses exactly two participants/captures and ten slots.
  bounds={'participants':(2,2),'captures':(2,2),'slots':(10,10),'assigned_at_ms':(10,10),'members':(0,5),'merkle_branch':(0,20)}
  lo,hi=bounds.get(path.split('/')[-1],(len(t),len(t)));array(v,hi,lo)
  if not t:req(v==[]);return []
  return [ordered(x,t[min(i,len(t)-1)],path+'/'+str(i),signed_paths) for i,x in enumerate(v)]
 if t is None:req(v is None);return None
 req(type(v)is type(t))
 if type(v)is int:
  integer(v,-1 if path in signed_paths else 0)
  if path.endswith('_ms'):time(v,True)
 elif type(v)is str:string(v)
 return v

def val_event(v):
 req(type(v)is dict and 'saturday'in v)
 try:e=event(v['saturday'])
 except(ValueError,TypeError):raise Refusal()
 req(ordered(v,e)==e);return e
def view(v):
 # Exact fixture authority tuple, allowing future authorized view/generation only
 # in semantic protocol controls by explicitly supplying a new expected view.
 for f in VIEWS:
  try:
   x=ordered(v,f['current_view']);hx(x['view_id']);integer(x['generation'],1,SAFE)
   req(all(x[k]==f['current_view'][k] for k in x if k not in ['view_id','generation']));return x
  except Refusal:pass
 raise Refusal()
def seed_status(v):
 t=SEED_STATUSES['finalized'];v=ordered(v,t,signed_paths=frozenset(['/scan_height','/scan_tip_height']))
 req(v['store_id']==SEED_ID);v['event']=val_event(v['event']);req(v['event_key']==event_key(v['event']))
 enum(v['phase'],['absent','prepared','waiting_anchor','waiting_target','provisional','finalized','conflict']);enum(v['publication_state'],['none','retained','unknown','submitted'])
 for k in ['scan_height','scan_tip_height']:integer(v[k],-1,I32)
 for k in ['anchor_height','target_height','target_descendants']:integer(v[k],0,I32)
 for k in ['anchor_hash','target_hash']:hx(v[k],empty=True)
 for k in ['finalized_at_ms','incident_at_ms','prepared_at_ms','observed_at_ms','chain_observed_at_ms']:time(v[k],True)
 integer(v['conflict_count'],0,8);integer(v['scan_generation'],0,16);integer(v['funding_generation'],0,16)
 enum(v['node_state'],['unobserved','online','offline','unavailable']);enum(v['funding_state'],['unobserved','scanning','ready','waiting_for_funds','unavailable'])
 req(v['chain_observed_at_ms']<=v['observed_at_ms']);req(v['exists']==(v['phase']!='absent'))
 req(v['escalation_due']==(v['observed_at_ms']>=v['event']['incident_deadline_ms'] and (not v['finalized_at_ms'] or v['conflict_count']>0 or v['conflict_overflow'])))
 if not v['exists']:
  req(all(v[k]==0 for k in ['prepared_at_ms','finalized_at_ms','incident_at_ms','anchor_height','target_height','target_descendants','conflict_count','scan_generation']))
  req(v['scan_height']==v['scan_tip_height']==-1 and not v['checkpoint_present'] and not v['current_finality'] and not v['conflict_overflow'] and v['publication_state']=='none' and v['anchor_hash']==v['target_hash']=='')
 if v['current_finality']:req(v['checkpoint_present'] and v['finalized_at_ms']>0 and v['target_descendants']>=60 and not v['conflict_count'] and not v['conflict_overflow'])
 return v
WORK=load('inputs/seed45/normal_work.json')
def source(v,context):
 v=exact(v,FIELDS['CardSource']);integer(v['version'],1,1);req(v['store_id']==SEED_ID);hx(v['event_key'])
 v['source_receipt']=ordered(v['source_receipt'],SOURCE['source_receipt']);r=v['source_receipt'];e=val_event(r['input']['event']);req(v['event_key']==event_key(e))
 req(r['snapshot_digest']==sha(domain('candidate cutoff input')+lp(J(r['input']))));req(e['cutoff_ms']<=r['completed_at_ms'])
 req(len(J(r))<=126976)
 ps=r['input']['participants'];req([p['participant_id'] for p in ps]==[g for g,_ in GYMS])
 allids=[]
 for p,cap in zip(ps,r['captures']):
  g=p['participant_id'];ep=next(x for x in SOURCE_PIN['participants'] if x['stable_pin']['identity']['gym_id']==g);sp=ep['stable_pin'];oldp=next(x for x in SOURCE['source_receipt']['input']['participants'] if x['participant_id']==g)
  req(p['owner']==dict(ownership_basis='candidate_assignment',account_authority_id=sp['account_authority_id'],account_id=sp['account_id'],gym_id=g))
  req(p['entry_store_id']==oldp['entry_store_id'] and p['roster']['store_id']==ep['stable_store_id'] and p['roster']['cutoff_ms']==e['cutoff_ms'])
  ent=p['entry'];req(ent['store_id']==p['entry_store_id'] and ent['event']==oldp['entry']['event'] and ent['entry_id']==oldp['entry']['entry_id'])
  req(cap['participant_id']==g and e['cutoff_ms']<=cap['captured_at_ms']<=r['completed_at_ms'])
  for name,sid in [('stable_fence',ep['stable_store_id']),('entry_fence',p['entry_store_id'])]:
   f=cap[name];req(f['version']==1 and f['store_id']==sid and f['cutoff_ms']==e['cutoff_ms'] and e['cutoff_ms']<=f['observed_at_ms']<=cap['captured_at_ms'])
  ids=[];classes=[]
  for m in p['members']:
   a=m['assignment'];hx(a['prospect_id']);hx(a['work_hash']);req(a['gym_id']==g and a['descriptor_id']==sp['source']['descriptor_id']);req(0<a['born_at_ms']<=a['accepted_at_ms']<e['cutoff_ms'])
   for key in ['manifest_digest','portrait_digest','source_record_digest']:hx(a[key])
   ids.append(a['prospect_id']);classes.append(m['class']);enum(m['class'],CLASSES);enum(m['eligibility'],['eligible','retired','not_rostered']);integer(m['slot'],1,10)
   con=m['condition_at_cutoff'];req(con['seed']==a['work_hash'] and con['born_at_ms']==a['born_at_ms'] and con['as_of_ms']==e['cutoff_ms'] and con['pro_xp']==0);integer(con['rating_hundredths'],0,100000)
  req(len(ids)==len(set(ids)) and len(classes)==len(set(classes)));allids+=ids
 req(len(allids)==len(set(allids)))

 v['declaration']=ordered(v['declaration'],SOURCE['declaration']);d=v['declaration']['declaration'];hx(v['declaration']['signature'],128)
 req(d['event']==e and d['store_id']==SEED_ID and d['source_store_id']==SOURCE_ID and d['event_key']==v['event_key'] and d['snapshot_digest']==r['snapshot_digest'] and d['receipt_digest']==sha(domain('candidate cutoff receipt')+lp(J(r))))
 for k in ['publisher_chain_id','network','genesis','floor_height','floor_hash','rules_id','publisher_public_key','checkpoint_public_key']:req(d[k]==SEED_PIN[k])
 for k in ['anchor','target','finality']:v[k]=ordered(v[k],SOURCE[k])
 a,t,f=v['anchor'],v['target'],v['finality'];cp=f['checkpoint']
 for x in [a,t]:integer(x['height'],0,I32);hx(x['hash']);hx(x['header'],160)
 for k in ['txid','raw_tx_digest']:hx(a[k])
 integer(a['raw_tx_size'],1,4000000);integer(a['tx_count'],1,1000000);integer(a['tx_index'],0,a['tx_count']-1);integer(a['output_index'],0,U32)
 hx(a['payload'],146)
 for h in a['merkle_branch']:hx(h)
 req(a['payload']==(b'SLUGSEED\x01'+raw(v['event_key'])+raw(sha(domain('candidate seed declaration id')+lp(J(d))))).hex())
 req(t['height']==a['height']+1 and raw(t['header'])[4:36][::-1].hex()==a['hash'])
 req(cp['version']==1 and cp['network']==NET and cp['genesis']==GENESIS and cp['height']==t['height'] and cp['hash']==t['hash']);hx(cp['signature'],128)
 for x in [t,f]:time(x['observed_at_ms']);integer(x['observed_tip']['height'],0,I32);hx(x['observed_tip']['hash'])
 req(r['completed_at_ms']<=t['observed_at_ms']<=f['observed_at_ms']);req(t['observed_tip']['height']>=t['height']);integer(f['target_descendants'],60,I32);req(f['observed_tip']['height']-t['height']==f['target_descendants'])
 # Cheap transport verifies hashes/crossbindings, never name/condition/signature/work.
 if context=='core':
  req(r==SOURCE['source_receipt']) # finite frozen authority-shaped receipt domain
  req(CRYPTO.ed_verify(raw(SEED_PIN['publisher_public_key']),domain('candidate seed declaration')+lp(J(d)),raw(v['declaration']['signature'])))
  pre=b'SLUG signed checkpoint\0'+u32(cp['version'])+u32(cp['network'])+raw(cp['genesis'])[::-1]+u32(cp['height'])+raw(cp['hash'])[::-1]
  req(CRYPTO.ed_verify(raw(SEED_PIN['checkpoint_public_key']),pre,raw(cp['signature'])))
  for x in [a,t]:req(x['header'] in WORK and WORK[x['header']]['work_hash']==x['hash'])
  req(CRYPTO.verify_merkle(a['txid'],a['tx_index'],a['tx_count'],a['merkle_branch'],raw(a['header'])[36:68][::-1].hex()))
 return v

def validate(kind,v,context='core'):
 req(context in ['core','transport']);v=exact(v,FIELDS[kind])
 req(len(J(v))+(kind in ['CardRequest','CardResponse'])<=CAPS.get(kind,262144))
 if 'version'in v:integer(v['version'],1,1)
 def val(t,x):return validate(t,x,context)
 if kind=='Rules':req(ordered(v,RULES)==RULES);v=clone(RULES)
 elif kind=='Pin':req(ordered(v,PIN)==PIN);v=clone(PIN)
 elif kind in ['Info','Marker','Metadata','Init']:
  req(v['store_id']==STORE_ID);v['pin']=val('Pin',v['pin'])
  if kind=='Marker':req(v['application']=='SLUG candidate cards')
  else:
   time(v['source_created_at_ms'])
   if kind!='Init':
    time(v['created_at_ms']);req(v['created_at_ms']>=v['source_created_at_ms']);integer(v['event_count'],0,256)
   if kind=='Metadata':time(v['clock_ms']);req(v['clock_ms']>=v['created_at_ms'])
 elif kind=='ProfessionalRecord':
  for x in v.values():integer(x,0,0)
 elif kind=='ReasonCounts':
  for x in v.values():integer(x,0,10)
 elif kind=='Counts':
  for k in ['total_count','eligible_count','paired_count','unmatched_count']:integer(v[k],0,10)
  v['reasons']=val('ReasonCounts',v['reasons']);req(v['total_count']==v['paired_count']+v['unmatched_count'] and v['eligible_count']<=v['total_count'] and v['paired_count']<=v['eligible_count'] and sum(v['reasons'].values())==v['unmatched_count'])
 elif kind=='GymCounts':req(v['gym_id'] in GYM_ACCOUNT);v['counts']=val('Counts',v['counts']);req(v['counts']['total_count']<=5)
 elif kind=='Entrant':
  hx(v['prospect_id']);req(v['gym_id']in GYM_ACCOUNT and v['account_id']==GYM_ACCOUNT[v['gym_id']]);string(v['display_name']);req(1<=len(v['display_name'])<=128);enum(v['class'],CLASSES);integer(v['rating_hundredths'],0,100000);enum(v['cutoff_eligibility'],['eligible','retired','not_rostered']);req(v['cutoff_eligibility']!='eligible' or v['rating_hundredths']>0);v['professional_record']=val('ProfessionalRecord',v['professional_record']);enum(v['state'],['paired','unmatched']);hx(v['bout_id'],empty=True)
  if v['state']=='paired':req(v['reason']=='' and v['bout_id']!='' and v['cutoff_eligibility']=='eligible')
  else:enum(v['reason'],REASONS);req(v['bout_id']=='')
 elif kind=='Bout':
  for k in ['bout_id','corner_a','corner_b','seed']:hx(v[k])
  enum(v['class'],CLASSES);req(v['corner_a']<v['corner_b'])
 elif kind=='Plan':
  for k in ['card_id','input_id','seed']:hx(v[k])
  req(v['record_basis']==BASIS);v['counts']=val('Counts',v['counts']);integer(v['bout_count'],0,5);array(v['gyms'],2,2);array(v['entrants'],10);array(v['bouts'],5)
  v['gyms']=[val('GymCounts',x) for x in v['gyms']];v['entrants']=[val('Entrant',x) for x in v['entrants']];v['bouts']=[val('Bout',x) for x in v['bouts']]
  inputs=[dict(prospect_id=e['prospect_id'],gym_id=e['gym_id'],account_id=e['account_id'],display_name=e['display_name'],class_=e['class'],rating_hundredths=e['rating_hundredths'],cutoff_eligibility=e['cutoff_eligibility']) for e in v['entrants']]
  try:expected=match(inputs,v['card_id'],v['input_id'],v['seed'])
  except ValueError:raise Refusal()
  req(v==expected)
 elif kind=='CardSource':v=source(v,context)
 elif kind=='CardState':
  v['status']=seed_status(v['status']);hx(v['source_digest'],empty=True);req(bool(v['source_digest'])==bool(v['status']['finalized_at_ms']))
 elif kind=='CardView':
  req(v['store_id']==SEED_ID);req(ordered(v['pin'],SEED_PIN)==SEED_PIN);v['pin']=clone(SEED_PIN);req(v['participant_gym_id']in GYM_ACCOUNT and v['participant_account_id']==GYM_ACCOUNT[v['participant_gym_id']]);time(v['created_at_ms']);time(v['observed_at_ms']);req(v['created_at_ms']<=v['observed_at_ms']);array(v['states'],8);v['states']=[val('CardState',x) for x in v['states']];ds=[x['status']['event']['saturday'] for x in v['states']];req(ds==sorted(set(ds)));req(all(x['status']['observed_at_ms']==v['observed_at_ms'] for x in v['states']))
 elif kind=='Record':
  req(v['store_id']==STORE_ID);v['source']=val('CardSource',v['source']);s=v['source'];req(v['card_id']==card_id(s['source_receipt']['input']['event']) and v['source_digest']==source_digest(s));v['plan']=val('Plan',v['plan']);time(v['created_at_ms']);req(v['created_at_ms']>=s['finality']['observed_at_ms']);req(v['plan']['card_id']==v['card_id']);i=sha(input_pre(s));req(v['plan']['input_id']==i and v['plan']['seed']==sha(domain('candidate card seed')+raw(i)+raw(s['target']['hash'])))
  if context=='core':req(v['plan']==derive(s))
  else:
   # Available snapshot fields can be correlated without reprojecting their facts.
   members={(p['participant_id'],m['assignment']['prospect_id']):(p,m) for p in s['source_receipt']['input']['participants'] for m in p['members']}
   req(len(members)==len(v['plan']['entrants']))
   for e in v['plan']['entrants']:
    req((e['gym_id'],e['prospect_id'])in members);p,m=members[e['gym_id'],e['prospect_id']];req(e['account_id']==p['owner']['account_id'] and e['class']==m['class'] and e['rating_hundredths']==m['condition_at_cutoff']['rating_hundredths'] and e['cutoff_eligibility']==m['eligibility'])
 elif kind=='Status':
  req(v['store_id']==STORE_ID);v['event']=val_event(v['event']);req(v['card_id']==card_id(v['event']));boolean(v['exists']);boolean(v['usable']);hx(v['record_digest'],empty=True);time(v['created_at_ms'],True);time(v['observed_at_ms']);v['counts']=val('Counts',v['counts']);integer(v['bout_count'],0,5);req(v['counts']['paired_count']==2*v['bout_count']);v['source']=val('CardState',v['source']);s=v['source']['status'];req(s['event']==v['event'] and s['observed_at_ms']<=v['observed_at_ms']);req(v['created_at_ms']<=v['observed_at_ms'])
  if not v['exists']:req(v['record_digest']=='' and v['created_at_ms']==v['bout_count']==0 and v['counts']==counts([]) and not v['usable'])
  else:req(v['record_digest']!='' and v['created_at_ms']>0 and s['finalized_at_ms']>0)
  if v['usable']:req(v['exists'] and s['finalized_at_ms']>0 and not s['conflict_count'] and not s['conflict_overflow'])
 elif kind in ['BuildResult','ReadResult']:
  v['record']=val('Record',v['record']);v['status']=val('Status',v['status']);r,st=v['record'],v['status'];req(st['exists'] and st['card_id']==r['card_id'] and st['record_digest']==record_digest(r) and st['created_at_ms']==r['created_at_ms'] and st['counts']==r['plan']['counts'] and st['bout_count']==r['plan']['bout_count']);req(st['source']['source_digest']==r['source_digest']);req(st['usable']==(not st['source']['status']['conflict_count'] and not st['source']['status']['conflict_overflow']))
  ss=st['source']['status'];sr=r['source'];req(ss['event_key']==sr['event_key'] and ss['anchor_height']==sr['anchor']['height'] and ss['anchor_hash']==sr['anchor']['hash'] and ss['target_height']==sr['target']['height'] and ss['target_hash']==sr['target']['hash'] and ss['finalized_at_ms']==sr['finality']['observed_at_ms'])
  if kind=='BuildResult':boolean(v['replayed'])
 elif kind=='HistoryPage':
  req(v['store_id']==STORE_ID);hx(v['after_card_id'],empty=True);hx(v['next_card_id'],empty=True);boolean(v['has_more']);time(v['observed_at_ms']);array(v['cards'],8);v['cards']=[val('Status',x) for x in v['cards']];req(all(x['exists'] and x['observed_at_ms']==v['observed_at_ms'] for x in v['cards']));ids=[(x['event']['cutoff_ms'],x['card_id']) for x in v['cards']];req(ids==sorted(set(ids)));req(v['after_card_id'] not in [x['card_id'] for x in v['cards']]);req(not v['has_more'] or len(ids)>0);req(v['next_card_id']==(v['cards'][-1]['card_id'] if v['has_more'] else ''))
 elif kind=='CardRequest':
  integer(v['id'],1,SAFE);enum(v['command'],['card_'+x for x in ['snapshot','status','record','history','build','job','cancel']]);hx(v['view_id'],empty=True);hx(v['after_card_id'],empty=True);hx(v['job_id'],empty=True);integer(v['limit'],0,8);string(v['saturday']);c=v['command'][5:]
  if c=='snapshot':req(v['view_id']==v['saturday']==v['after_card_id']==v['job_id']=='' and v['limit']==0)
  else:
   req(v['view_id']!='')
   if c in ['status','record','build']:
    try:dateparts(v['saturday'])
    except ValueError:raise Refusal()
    req(v['after_card_id']==v['job_id']=='' and v['limit']==0)
   elif c=='history':req(v['saturday']==v['job_id']=='' and v['limit']>=1)
   else:req(v['job_id']!='' and v['saturday']==v['after_card_id']=='' and v['limit']==0)
 elif kind=='CardSnapshot':
  enum(v['state'],['ready','busy','setup_incomplete','unavailable','failed']);string(v['error']);hx(v['store_id'],empty=True)
  for k in ['active_job_id','queued_job_id','last_job_id']:hx(v[k],empty=True)
  time(v['source_created_at_ms'],True);time(v['created_at_ms'],True);ids=[v[k] for k in ['active_job_id','queued_job_id','last_job_id'] if v[k]];req(len(ids)==len(set(ids)))
  req(not(v['active_job_id'] and v['queued_job_id']));req(not(v['active_job_id'] or v['queued_job_id']) or v['state']=='busy')
  if v['state'] in ['ready','busy','failed']:
   v['view']=view(v['view']);v['pin']=val('Pin',v['pin']);req(v['store_id']==STORE_ID and 0<v['source_created_at_ms']<=v['created_at_ms']);req(v['error']in ['card_unavailable','card_corrupt','card_clock','card_source_changed'] if v['state']=='failed' else v['error']=='')
  else:
   req(v['pin']is None and not ids and v['store_id']=='' and v['source_created_at_ms']==v['created_at_ms']==0)
   if v['state']=='unavailable':req(v['view']is None and v['error']=='card_unavailable')
   else:v['view']=view(v['view']);req(v['error']=='card_setup_incomplete')
 elif kind=='CardJob':
  hx(v['job_id']);req(v['kind']=='build');enum(v['state'],['queued','running','completed','failed','cancelled']);v['view']=view(v['view']);req(v['store_id']==STORE_ID);req(v['card_id']==card_id(event(v['saturday'])));string(v['error'])
  if v['state']=='completed':req(v['error']=='');v['result']=val('BuildResult',v['result']);req(v['result']['record']['card_id']==v['card_id'])
  else:
   req(v['result']is None)
   if v['state']in ['queued','running']:req(v['error']=='')
   elif v['state']=='cancelled':req(v['error']=='card_cancelled')
   else:req(v['error']in ERRORS)
 elif kind=='CardResponse':
  integer(v['id'],1,SAFE);boolean(v['ok']);string(v['error']);hx(v['store_id'],empty=True);names=['snapshot','status','record','history','job'];types=['CardSnapshot','Status','ReadResult','HistoryPage','CardJob']
  if not v['ok']:req(v['error']in ERRORS and v['view']is None and v['store_id']=='' and all(v[k]is None for k in names))
  else:
   req(v['error']=='' and sum(v[k]is not None for k in names)==1)
   for k,t in zip(names,types):
    if v[k]is not None:v[k]=validate(t,v[k],'transport')
   snap=v['snapshot']
   if snap is not None and snap['state']=='unavailable':req(v['view']is None and v['store_id']=='')
   else:
    v['view']=view(v['view']);req(v['store_id']==('' if snap is not None and snap['state']=='setup_incomplete' else STORE_ID))
    for k in ['snapshot','job']:
     if v[k]is not None:req(v[k]['view']==v['view'] and v[k]['store_id']==v['store_id'])
 return v

def parse(kind,b,context='core'):
 req(type(b)is bytes and 0<len(b) and len(b)+(kind in ['CardRequest','CardResponse'])<=CAPS.get(kind,262144))
 def pairs(ps):
  d={}
  for k,v in ps:req(k not in d);d[k]=v
  return d
 def number(s):req(bool(re.fullmatch('0|[1-9][0-9]*|-1',s)));return int(s)
 try:v=json.loads(b.decode('utf8'),object_pairs_hook=pairs,parse_int=number,parse_float=lambda s:req(False),parse_constant=lambda s:req(False))
 except(ValueError,UnicodeError,RecursionError):raise Refusal()
 def pre(x,d=0):
  if type(x)in [dict,list]:req(d+1<=16)
  if type(x)is dict:
   for k,y in x.items():req(len(k)<=64);pre(y,d+1)
  elif type(x)is list:
   req(len(x)<=20)
   for y in x:pre(y,d+1)
  elif type(x)is str:string(x)
 pre(v)
 try:return validate(kind,v,context)
 except(TypeError,KeyError,OverflowError,ValueError,IndexError):raise Refusal()

def correlate(q,r,current_view,expected_job=None,terminal_digest=None,created_at_ms=CREATED):
 q=validate('CardRequest',q,'transport');r=validate('CardResponse',r,'transport');req(q['id']==r['id'])
 if not r['ok']:return r
 c=q['command'][5:];slot=c if c in ['snapshot','status','record','history'] else 'job';req(r[slot]is not None)
 if not(slot=='snapshot' and r['snapshot']['state']=='unavailable'):req(r['view']==current_view and (not q['view_id'] or q['view_id']==current_view['view_id']))
 if slot=='snapshot':
  s=r[slot]
  if s['pin']is not None:req(s['source_created_at_ms']==SOURCE_CREATED and s['created_at_ms']==created_at_ms)
 elif slot in ['status','record']:
  e=r[slot]['event'] if slot=='status' else r[slot]['status']['event'];req(e['saturday']==q['saturday'])
 elif slot=='history':req(r[slot]['after_card_id']==q['after_card_id'] and len(r[slot]['cards'])<=q['limit'])
 else:
  j=r['job']
  if c=='build':req(j['state']=='queued' and j['saturday']==q['saturday'])
  else:req(j['job_id']==q['job_id'])
  if expected_job is not None:
   req(all(j[k]==expected_job[k] for k in ['job_id','kind','card_id','saturday']))
   if 'view'in expected_job:req(j['view']==expected_job['view'])
   req(not(expected_job['state']=='running' and j['state']=='queued'))
   if expected_job['state']in ['completed','failed','cancelled']:req(j['state']==expected_job['state'])
  if terminal_digest is not None:req(sha(J(j))==terminal_digest)
 return r
