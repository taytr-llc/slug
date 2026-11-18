"""Closed Set50 core-value and cheap transport grammar. Parsing grants no authority."""
from common import *
class Refusal(ValueError):pass
def req(v,m='invalid'):
 if not v:raise Refusal(m)
def integer(v,lo=0,hi=SAFE):req(type(v)is int and lo<=v<=hi,'integer')
def boolean(v):req(type(v)is bool,'boolean')
def string(v,lo=0,hi=512):req(type(v)is str and v.isascii()and lo<=len(v)<=hi,'string')
def hx(v,empty=False):req(type(v)is str and ((empty and v=='')or re.fullmatch('[0-9a-f]{64}',v)is not None),'hex')
def time(v,zero=False):integer(v,0 if zero else 1,DATE_MAX)
def enum(v,values):req(type(v)is str and v in values,'enum')
def exact(v,keys):req(type(v)is dict and set(v)==set(keys),'fields');return {k:v[k]for k in keys}
def array(v,lo,hi):req(type(v)is list and lo<=len(v)<=hi,'array')
def ordered(v,t,path='',signed=None):
 if type(t)is dict:return {k:ordered(exact(v,t)[k],t[k],path+'/'+k,signed)for k in t}
 if type(t)is list:
  array(v,len(t),len(t));return [ordered(x,t[i],path+'/'+str(i),signed)for i,x in enumerate(v)]
 req(type(v)is type(t),'type')
 if type(v)is int:
  lo,hi=(signed or {}).get(path,(0,SAFE));integer(v,lo,hi)
  if path.endswith('_ms'):time(v,True)
 elif type(v)is str:string(v,0,8192)
 return v
# Exact frozen result shape, independent of future Set50 Go.
RESULT_TEMPLATE=clone(OLD['cases'][0]['response']['job']['result']['engine_result'])
# This template requires a non-null full-round recovery for nested type grammar.
if RESULT_TEMPLATE['rounds'][0]['recovery']is None:
 RESULT_TEMPLATE=next(clone(x['response']['job']['result']['engine_result'])for x in OLD['cases']if x['response']['job']['result']['engine_result']['rounds'][0]['recovery']is not None)
CAPS={'Result':196608,'CareerPreviewFighter':8448,'CareerPreviewPair':24576,'CareerPreviewRecipe':4096,'CareerPreviewResult':229376,'CareerPreviewJob':231424,'CareerPreviewSnapshot':4096,'CareerPreviewRequest':4096,'CareerPreviewResponse':262144,'CareerPreviewPortraitResponse':98304}
# Reuse only the two exact source-pinned cheap engine functions; no Set47 module import/bootstrap.
exec(compile((ROOT/'inputs/engine_transport_excerpt.py.txt').read_text(),str(ROOT/'inputs/engine_transport_excerpt.py.txt'),'exec'),globals())
# New-context wrapper preserves the existing Preview-v1 structural acceptance grammar.
# The exact47 excerpt remains a pinned reference, not an authority to tighten old controls.
prior47_engine_result=engine_result

def engine_result(v,inp,context):
 v=exact(v,RESULT_TEMPLATE);integer(v['version'],1,1);req(v['engine_id']==B.ENGINE and v['input_id']==B.input_id(inp))
 array(v['initial'],2,2);v['initial']=[ordered(x,RESULT_TEMPLATE['initial'][i],signed={'/form_q':(-100000,100000)})for i,x in enumerate(v['initial'])]
 minima=dict(power=720000,speed=540000,chin=630000,stamina=630000,skill=810000)
 for x in v['initial']:
  integer(x['jitter_q'],920000,1080000);integer(x['max_stamina_q'],60000000,214000000);integer(x['ko_threshold_q'],64000000,130000000)
  for k,n in x['effective'].items():integer(n,minima[k],130000000 if k=='skill'else 110000000)
 def st(x):
  integer(x['stamina_q'],0,214000000)
  for k in ['damage_taken_q','damage_dealt_q']:integer(x[k],0,2699999999)
  for k in ['thrown','landed','significant','knockdowns']:integer(x[k],0,180)
  integer(x['control_q'],0,630000000);integer(x['round_knockdowns'],0,3);req(x['significant']<=x['landed']<=x['thrown'])
 array(v['rounds'],1,3);out=[]
 for ri,r in enumerate(v['rounds']):
  r=exact(r,['number','ticks','end','recovery']);integer(r['number'],ri+1,ri+1);array(r['ticks'],1,60);last=ri==len(v['rounds'])-1
  if not last:req(len(r['ticks'])==60)
  r['ticks']=[ordered(t,RESULT_TEMPLATE['rounds'][0]['ticks'][0])for t in r['ticks']]
  for ti,t in enumerate(r['ticks']):
   integer(t['number'],ti+1,ti+1);integer(t['actor'],0,1);enum(t['strike'],B.STRIKES);enum(t['stoppage'],['','ko','tko_referee','tko_three_knockdowns']);integer(t['damage_q'],0,8999999)
   req(t['hit']or(t['damage_q']==0 and not t['significant']and not t['knockdown']and t['stoppage']==''));req(not t['knockdown']or t['significant']);req(not t['stoppage']or(last and ti==len(r['ticks'])-1 and t['stoppage']==v['method']))
   for x in t['states']:st(x)
  r['end']=ordered(r['end'],RESULT_TEMPLATE['rounds'][0]['end']);req(r['end']==r['ticks'][-1]['states']);needrec=len(r['ticks'])==60 and ri<2 and not r['ticks'][-1]['stoppage'];req(needrec==(r['recovery']is not None))
  if r['recovery']is not None:
   r['recovery']=ordered(r['recovery'],RESULT_TEMPLATE['rounds'][0]['recovery'])
   for x in r['recovery']['states']:st(x)
   if not last:req(not any(r['recovery']['corner_stopped']))
  out.append(r)
 v['rounds']=out;req(sum(len(r['ticks'])for r in out)<=180);v['final']=ordered(v['final'],RESULT_TEMPLATE['final']);last=out[-1];req(v['final']==(last['recovery']['states']if last['recovery']else last['end']))
 v['scores']=ordered(v['scores'],RESULT_TEMPLATE['scores']);req(v['scores']==[B.scores(x)for x in v['final']]);
 for score in v['scores']:
  for key,hi in [('damage_tenths',26999),('knockdown_tenths',18000),('accuracy_tenths',250),('volume_tenths',1080),('control_tenths',2205)]:integer(score[key],0,hi)
 v['draw_counts']=ordered(v['draw_counts'],RESULT_TEMPLATE['draw_counts'])
 for x in v['draw_counts'].values():integer(x,1,65536)
 req(sum(v['draw_counts'].values())<=65536);enum(v['method'],METHODS);hx(v['winner'],True);integer(v['end_round'],1,3);integer(v['end_tick'],1,60);req(v['end_round']==len(out)and v['end_tick']==len(last['ticks']));a,b=inp['a']['prospect_id'],inp['b']['prospect_id'];m=v['method'];tick=last['ticks'][-1]
 if m in ['decision','decision_draw']:
  req(len(out)==3 and len(last['ticks'])==60 and last['recovery']is None and not tick['stoppage']);x,y=[s['total_tenths']for s in v['scores']];req(m==('decision_draw'if x==y else'decision')and v['winner']==(''if x==y else a if x>y else b))
 elif m in ['double_corner_stoppage','tko_corner']:
  req(len(out)<3 and len(last['ticks'])==60 and last['recovery']is not None);stopped=last['recovery']['corner_stopped'];req(any(stopped));req(m==('double_corner_stoppage'if all(stopped)else'tko_corner'));req(v['winner']==(''if all(stopped)else[b,a][stopped.index(True)]))
 else:req(last['recovery']is None and tick['stoppage']==m and tick['hit']and tick['significant']and v['winner']==[a,b][tick['actor']])
 if context=='core':req(v==B.simulate(inp))
 req(len(J(v))<=196608);return v

ERRORS='invalid_request unauthorized forbidden stale_view busy stable_corrupt stable_clock stable_conflict stable_sync_required outcome_unknown preview_not_found preview_class_mismatch preview_capacity preview_cancelled preview_unavailable preview_observation_unknown career_invalid career_inactive career_unfenced career_closed career_poisoned career_cancelled career_corrupt'.split()
UNAVAILABLE=['','preview_unavailable','career_inactive','career_unfenced','career_closed','career_poisoned','career_corrupt']
FIGHTER_FIELDS='assignment manifest totals condition'.split();PAIR_FIELDS='version status activation career_digest as_of_ms fighters'.split();RESULT_FIELDS='version pair recipe seed engine_result engine_result_digest'.split()
JOB_FIELDS='version job_id state view store_id engine_version engine_id activation_id fighter_a_id fighter_b_id error result'.split();SNAP_FIELDS='version state error view store_id engine_version engine_id activation active_job_id queued_job_id last_job_id'.split()
REQUEST_FIELDS='id version command view_id fighter_a_id fighter_b_id job_id'.split();COMMANDS=['career_preview_snapshot','career_preview_begin','career_preview_job','career_preview_cancel']
def total(v):
 v=exact(v,'wins losses draws xp base_points'.split())
 for k,x in v.items():integer(x,0,(1<<32)-1)
 n=v['wins']+v['losses']+v['draws'];base=10*v['wins']+4*v['draws'];req(n<=256 and v['xp']==min(15,n)and base<=v['base_points']<=base+4*v['wins']and(v['base_points']-base)%4==0,'totals');return v

def activation(v):
 v=exact(v,'version policy_id activation_id result_store_id result_created_at_ms source_created_at_ms activated_at_ms'.split());integer(v['version'],1,1)
 for k in ['policy_id','activation_id','result_store_id']:hx(v[k])
 for k in ['result_created_at_ms','source_created_at_ms','activated_at_ms']:time(v[k])
 req(v['source_created_at_ms']<=v['result_created_at_ms']<=v['activated_at_ms'],'activation time');a,_=C.activation(v['result_store_id'],v['result_created_at_ms'],v['source_created_at_ms'],v['activated_at_ms']);req(v==a,'activation');req(len(J(v))<=512);return v

def view(v):
 t=OLD['cases'][0]['view'];v=ordered(v,t);integer(v['version'],1,1);integer(v['generation'],1)
 for k in ['view_id','account_authority_id','account_id','transport_trust_id']:hx(v[k])
 p=v['authority_pin'];integer(p['version'],5,5);req(p['network']==t['authority_pin']['network']and p['genesis']==t['authority_pin']['genesis']and p['descriptor_id']==t['authority_pin']['descriptor_id'])
 hx(p['authority_id']);req(p['authority_id']!=v['account_authority_id']);req(re.fullmatch(r'http://127\.0\.0\.1:[1-9][0-9]{0,4}',v['authority_origin'])is not None);port=int(v['authority_origin'].rsplit(':',1)[1]);req(port<=65535)
 trust=sha(b'SLUG manifest transport trust\0v1\0'+lp(v['authority_origin'].encode())+bytes([1])+(0).to_bytes(4,'little'));req(v['transport_trust_id']==trust);req(len(J(v))<=1024);return v

def status(v):
 v=ordered(v,OLD['cases'][0]['status']);integer(v['version'],1,1);hx(v['store_id']);p=v['pin'];req(any(J(p)==J(x['status']['pin'])for x in OLD['cases']),'finite fixture pin');req(v['store_id']==stable_id(p));integer(v['checkpoint'],0,1000);integer(v['observed_source_count'],v['checkpoint'],1000);req(v['caught_up']==(v['checkpoint']==v['observed_source_count']));integer(v['operation_count'],0,4096);integer(v['roster_revision'],0,v['operation_count']);array(v['slots'],10,10)
 for i in v['slots']:hx(i,True)
 req(len([i for i in v['slots']if i])==len(set(i for i in v['slots']if i)));hx(v['pending_operation_id'],True);hx(v['last_operation_id'],True);req((v['operation_count']==0)==(v['last_operation_id']=='')and(not v['pending_operation_id']or v['pending_operation_id']==v['last_operation_id']));time(v['last_as_of_ms'],True);req(len(J(v))<=4096);return v

def pair(v,owner=None):
 v=exact(v,PAIR_FIELDS);integer(v['version'],1,1);v['status']=status(v['status']);v['activation']=activation(v['activation']);hx(v['career_digest']);time(v['as_of_ms']);req(v['status']['last_as_of_ms']==v['as_of_ms']>=v['activation']['activated_at_ms']);array(v['fighters'],2,2);out=[]
 template=OLD['cases'][0]['response']['job']['result']['pair']['fighters'][0]
 for f in v['fighters']:
  f=exact(f,FIGHTER_FIELDS);a=ordered(f['assignment'],template['assignment']);integer(a['version'],1,1);integer(a['sequence'],1,v['status']['checkpoint'])
  for k in ['prospect_id','gym_id','work_hash','descriptor_id','manifest_digest','portrait_digest','source_record_digest']:hx(a[k])
  req(a['gym_id']==v['status']['pin']['identity']['gym_id']and a['descriptor_id']==v['status']['pin']['source']['descriptor_id']);req(1<=a['born_at_ms']<=a['accepted_at_ms']<=v['as_of_ms'] and a['accepted_at_ms']<a['born_at_ms']+300000)
  # Bounded independent manifest/condition arithmetic is allowed in transport; no PoW or simulation.
  m=B.MANIFEST.parse_manifest(B.MANIFEST.canonical(ordered(f['manifest'],template['manifest'],signed={'/portrait/shoulder_jitter':(-6,6)})));req(m['dna']['seed']==a['work_hash']and B.MANIFEST.digest_manifest(B.MANIFEST.canonical(m))==a['manifest_digest']);tot=total(f['totals']);c=B.CONDITION.project(1,m['dna'],a['born_at_ms'],v['as_of_ms'],tot['xp']);req(J(ordered(f['condition'],c))==J(c),'condition')
  f=dict(assignment=a,manifest=m,totals=tot,condition=c);req(len(J(f))<=8448);out.append(f)
 v['fighters']=out;ids=[f['assignment']['prospect_id']for f in out];req(ids[0]<ids[1]and out[0]['manifest']['physical']['weight_class']==out[1]['manifest']['physical']['weight_class'],'pair');req(len(J(v))<=24576)
 if owner is not None:
  req(v['activation']==owner['activation']);s=C.snapshot(owner['activation'],owner['events'],ids,v['as_of_ms'],'current',owner['clock_ms']);req(s['error']==''and s['digest']==v['career_digest']and[f['totals']for f in out]==[t['totals']for t in s['totals']],'trusted modeled prefix')
 return v

def result(v,owner=None):
 v=exact(v,RESULT_FIELDS);integer(v['version'],1,1);v['pair']=pair(v['pair'],owner);r=recipe(v['pair']);req(J(ordered(v['recipe'],r))==J(r),'recipe');v['recipe']=r;req(len(J(r))<=4096);s=seed(r);req(v['seed']==s,'seed');inp=input_for(v['pair'],s);v['engine_result']=engine_result(v['engine_result'],inp,'transport');req(v['engine_result_digest']==B.digest(v['engine_result']),'result digest');req(len(J(v))<=229376);return v

def job(v):
 v=exact(v,JOB_FIELDS);integer(v['version'],1,1);v['view']=view(v['view']);integer(v['engine_version'],1,1);req(v['engine_id']==B.ENGINE)
 for k in ['job_id','store_id','activation_id','fighter_a_id','fighter_b_id']:hx(v[k])
 req(v['fighter_a_id']<v['fighter_b_id']);enum(v['state'],['queued','running','done','failed','cancelled']);enum(v['error'],['']+ERRORS)
 if v['state']=='done':
  req(v['error']=='');v['result']=result(v['result']);p=v['result']['pair'];req(v['activation_id']==p['activation']['activation_id']and v['store_id']==p['status']['store_id']and[v['fighter_a_id'],v['fighter_b_id']]==[x['assignment']['prospect_id']for x in p['fighters']]);req(v['view']['account_authority_id']==p['status']['pin']['account_authority_id']and v['view']['account_id']==p['status']['pin']['account_id']and v['view']['authority_pin']==p['status']['pin']['source'])
 else:
  req(v['result']is None);req(v['error']==''if v['state']in['queued','running']else v['error']=='preview_cancelled'if v['state']=='cancelled'else v['error']in ERRORS and v['error']!='outcome_unknown');req(len(J(v))<=2048)
 metadata=clone(v);metadata['result']=None;req(len(J(metadata))<=2048);req(len(J(v))<=231424);return v

def snapshot(v):
 v=exact(v,SNAP_FIELDS);integer(v['version'],1,1);enum(v['state'],['idle','busy','ready','failed','cancelled','unavailable']);enum(v['error'],['']+ERRORS)
 if v['state']=='unavailable':req(v['view']is None and v['activation']is None and v['engine_version']==0 and all(v[k]==''for k in ['store_id','engine_id','active_job_id','queued_job_id','last_job_id'])and v['error']in UNAVAILABLE);integer(v['engine_version'],0,0)
 else:
  v['view']=view(v['view']);v['activation']=activation(v['activation']);hx(v['store_id']);integer(v['engine_version'],1,1);req(v['engine_id']==B.ENGINE)
  for k in ['active_job_id','queued_job_id','last_job_id']:hx(v[k],True)
  ids=[v[k]for k in ['active_job_id','queued_job_id','last_job_id']if v[k]];req(len(ids)==len(set(ids))and not(v['active_job_id']and v['queued_job_id']));req(not(v['active_job_id']or v['queued_job_id'])or v['state']=='busy')
  active=bool(v['active_job_id']or v['queued_job_id']);last=bool(v['last_job_id']);state=v['state'];err=v['error']
  if state=='idle':req(not active and not last and err=='')
  elif state=='busy':req(active and not last and err=='')
  elif state=='ready':req(not active and last and err=='')
  else:req(not active and last and err in ERRORS and err!='outcome_unknown' and(state!='cancelled'or err=='preview_cancelled'))
 req(len(J(v))<=4096);return v

def request(v):
 req(type(v)is dict and set(v)<=set(REQUEST_FIELDS)and all(k in v for k in ['id','version','command']));integer(v['id'],1);integer(v['version'],1,1);enum(v['command'],COMMANDS)
 fields={'career_preview_snapshot':[],'career_preview_begin':['view_id','fighter_a_id','fighter_b_id'],'career_preview_job':['view_id','job_id'],'career_preview_cancel':['view_id','job_id']}[v['command']];req(set(v)==set(['id','version','command']+fields))
 for k in fields:hx(v[k])
 if v['command']=='career_preview_begin':req(v['fighter_a_id']<v['fighter_b_id'])
 return {k:v[k]for k in REQUEST_FIELDS if k in v}

def response(v):
 req(type(v)is dict and set(v)<=set('id version ok snapshot job error'.split())and all(k in v for k in ['id','version','ok']));integer(v['id'],1);integer(v['version'],1,1);boolean(v['ok'])
 if not v['ok']:req(set(v)==set(['id','version','ok','error']));enum(v['error'],ERRORS);return {k:v[k]for k in ['id','version','ok','error']}
 field='snapshot'if'snapshot'in v else'job';req(set(v)==set(['id','version','ok',field]));return dict(id=v['id'],version=1,ok=True,**{field:snapshot(v[field])if field=='snapshot'else job(v[field])})

def portrait_response(v):
 req(type(v)is dict and set(v)<=set(['version','ok','portrait','error']));integer(v.get('version'),1,1);boolean(v.get('ok'))
 if not v['ok']:req(set(v)==set(['version','ok','error']));enum(v['error'],ERRORS);return v
 req(set(v)==set(['version','ok','portrait']));p=exact(v['portrait'],'version view store_id job_id prospect_id activation_id career_digest result_digest descriptor_id manifest_digest portrait_digest svg'.split());integer(p['version'],1,1);p['view']=view(p['view'])
 for k in p:
  if k not in ['version','view','svg']:hx(p[k])
 string(p['svg'],1,32768);req(p['portrait_digest']==sha(P.DOMAIN+p['svg'].encode()));req(len(J(v))<=98304);return dict(version=1,ok=True,portrait=p)

VALIDATORS={'CareerPreviewPair':pair,'CareerPreviewResult':result,'CareerPreviewJob':job,'CareerPreviewSnapshot':snapshot,'CareerPreviewRequest':request,'CareerPreviewResponse':response,'CareerPreviewPortraitResponse':portrait_response}
def validate(kind,v,owner=None):
 req(kind in VALIDATORS);out=VALIDATORS[kind](v,owner)if kind in ['CareerPreviewPair','CareerPreviewResult']else VALIDATORS[kind](v);req(len(J(out))<=CAPS[kind]);return out

def parse(kind,raw,owner=None):
 # NDJSON frame bounds include the one terminating LF; public parse input excludes it.
 req(type(raw)is bytes and 0<len(raw)<=CAPS[kind]-(1 if kind in ['CareerPreviewRequest','CareerPreviewResponse']else 0),'bytes');
 def pairs(values):
  out={}
  for k,v in values:req(k not in out,'duplicate');out[k]=v
  return out
 def bad(_):raise Refusal('number')
 def number(s):req(re.fullmatch('0|-?[1-9][0-9]*',s)is not None and len(s)<=20,'number');return int(s)
 try:v=json.loads(raw.decode('utf-8'),object_pairs_hook=pairs,parse_int=number,parse_float=bad,parse_constant=bad)
 except(UnicodeError,json.JSONDecodeError,ValueError,OverflowError)as e:raise Refusal(str(e))
 def depth(v,n=1):
  if type(v)in(dict,list):
   req(n<=12,'depth')
   for x in(v.values()if type(v)is dict else v):depth(x,n+1)
 depth(v);return validate(kind,v,owner)

def correlate(reqv,resp,current,authority,known=None,terminal_digest=None):
 q=request(reqv);r=response(resp);req(q['id']==r['id']);
 if not r['ok']:return r
 if q['command']=='career_preview_snapshot':
  req('snapshot'in r);s=r['snapshot']
  if s['state']=='unavailable':return r
  req(current is not None and s['view']==current and s['store_id']==authority['store_id']and s['activation']==authority['activation']);return r
 req('job'in r and current is not None and q['view_id']==current['view_id']);j=r['job'];req(j['view']==current and j['store_id']==authority['store_id']and j['activation_id']==authority['activation']['activation_id'])
 if q['command']=='career_preview_begin':req(j['state']=='queued'and[j['fighter_a_id'],j['fighter_b_id']]==[q['fighter_a_id'],q['fighter_b_id']])
 else:req(j['job_id']==q['job_id']and known is not None)
 if known is not None:
  if 'snapshot'in known:
   ss=snapshot(known['snapshot']);req(ss['view']==current and ss['store_id']==authority['store_id']and ss['activation']==authority['activation']and j['job_id']in[ss[k]for k in ['active_job_id','queued_job_id','last_job_id']]and j['job_id']!='')
  else:
   req(all(j[k]==known[k]for k in ['job_id','view','store_id','activation_id','fighter_a_id','fighter_b_id']));req(not(known['state']=='running'and j['state']=='queued'))
   if known['state']in ['done','failed','cancelled']:req(J(j)==J(job(clone(known))),'known terminal immutable')
 if terminal_digest is not None:req(sha(J(j))==terminal_digest,'terminal immutable')
 return r


def correlate_portrait(value,current,known,fresh_job):
 r=portrait_response(value);req(r['ok']);j=job(fresh_job);req(j['state']=='done'and j['view']==current);p=r['portrait'];req(p['view']==current and p['job_id']==j['job_id']and p['store_id']==j['store_id']and p['activation_id']==j['activation_id'])
 req(all(j[k]==known[k]for k in ['job_id','view','store_id','activation_id','fighter_a_id','fighter_b_id']),'selected job tuple');full=j['result'];req(sha(J(full))==known['captured_result_digest']);req(p['career_digest']==full['pair']['career_digest']and p['result_digest']==full['engine_result_digest']and p['descriptor_id']==full['pair']['status']['pin']['source']['descriptor_id'])
 members=[x for x in full['pair']['fighters']if x['assignment']['prospect_id']==p['prospect_id']];req(len(members)==1);a=members[0]['assignment'];req(p['manifest_digest']==a['manifest_digest']and p['portrait_digest']==a['portrait_digest']and p['svg']==known['svg']);return r
