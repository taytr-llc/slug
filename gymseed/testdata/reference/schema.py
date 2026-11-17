"""Closed finite-fixture codecs. Old Set44 receipts are matched to pinned immutable
source values after recursively canonicalizing fields; this is NOT a general
Set44 authority validator. Crypto/new DTO checks are independently implemented.
"""
import re,json,struct
from common import *
from crypto import publication,verify_merkle,Reader,parse_tx
FIELDS={
'Pin':'version source_store_id source_pin publisher_gym_id publisher_account_id publisher_source_authority_id publisher_chain_id network genesis floor_height floor_hash publisher_public_key checkpoint_public_key funding_public_key funding_script rules_id',
'Info':'version store_id pin created_at_ms event_count funding_height funding_hash funding_complete funding_generation funding_restart_count',
'Declaration':'version event_key store_id source_store_id event snapshot_digest receipt_digest publisher_chain_id network genesis floor_height floor_hash rules_id publisher_public_key checkpoint_public_key',
'SignedDeclaration':'version declaration signature',
'Outpoint':'txid index','FundingCoin':'outpoint value_satoshis script block_height block_hash coinbase',
'Publication':'version raw_tx txid coin built_at_ms dispatch_count first_dispatch_at_ms last_dispatch_at_ms state',
'Cursor':'generation height hash scan_digest complete_to_height complete_to_hash','Tip':'height hash',
'Anchor':'height hash header txid raw_tx_digest raw_tx_size tx_count tx_index output_index payload merkle_branch',
'ScanRestart':'generation previous observed_at_ms observed_tip replacement_hash',
'Target':'height hash header observed_at_ms observed_tip','SignedCheckpoint':'version network genesis height hash signature',
'Finality':'checkpoint observed_at_ms observed_tip target_descendants',
'Conflict':'kind observed_at_ms declaration checkpoint expected_height expected_hash observed_hash observed_tip',
'Observation':'observed_at_ms chain_observed_at_ms tip node_state target_descendants current_finality',
'Record':'version store_id event_key source_receipt declaration prepared_at_ms publication cursor scan_restarts anchor target checkpoint finality conflicts conflict_overflow incident_at_ms observation',
'Status':'version store_id event_key event exists phase publication_state scan_height scan_tip_height anchor_height anchor_hash target_height target_hash target_descendants checkpoint_present finalized_at_ms conflict_count conflict_overflow incident_at_ms prepared_at_ms observed_at_ms chain_observed_at_ms node_state funding_state current_finality escalation_due scan_generation funding_generation',
'HistoryPage':'version store_id after_event_key next_event_key has_more observed_at_ms events',
'Marker':'application version store_id pin','Init':'version store_id pin',
'Metadata':'version store_id pin created_at_ms clock_ms event_count funding_cursor funding_restarts funding_complete last_funding_tip',
'SeedRequest':'id command view_id saturday after_event_key limit job_id',
'SeedResponse':'id ok view store_id snapshot status record history job error',
'SeedSnapshot':'version state error view store_id pin created_at_ms publisher_account_id node_state active_job_id queued_job_id last_job_id',
'SeedJob':'version job_id kind state view store_id event_key saturday error result',
'Rules':'version target floor carrier finality max_step_blocks max_scan_restarts fee_satoshis'}
FIELDS={k:v.split() for k,v in FIELDS.items()}
CAPS={'Pin':16384,'Info':20480,'Declaration':4096,'SignedDeclaration':5120,'ScanRestart':1024,'SignedCheckpoint':4096,'Conflict':8192,'Record':229376,'Status':8192,'Marker':20480,'Metadata':40960,'Init':20480,'HistoryPage':262144,'SeedRequest':8192,'SeedResponse':262144,'SeedJob':245760,'SeedSnapshot':16384}
ERRORS='invalid_request unauthorized forbidden stale_view busy outcome_unknown seed_unavailable seed_setup_incomplete seed_corrupt seed_capacity seed_clock seed_calendar seed_created_too_late seed_source_incomplete seed_not_found seed_not_prepared seed_node_offline seed_waiting_for_funds seed_scan_incomplete seed_conflict seed_target_unavailable seed_not_mature seed_cancelled'.split()
class Refusal(ValueError):pass
def req(x,why='invalid_request'):
 if not x:raise Refusal(why)
def integer(v,lo=0,hi=SAFE):req(type(v)is int and lo<=v<=hi)
def boolean(v):req(type(v)is bool)
def string(v):req(type(v)is str and v.isascii())
def enum(v,values):string(v);req(v in values)
def hexval(v,n=64,empty=False):string(v);req((empty and v=='') or bool(re.fullmatch('[0-9a-f]{%d}'%n,v)))
def time(v,zero=False):integer(v,0 if zero else 1,DATE_MAX)
def exact(v,keys):req(type(v)is dict and set(v)==set(keys));return {k:v[k] for k in keys}
def like(v,t):
 if type(t)is dict:
  z=exact(v,t);return {k:like(z[k],t[k]) for k in t}
 if type(t)is list:
  req(type(v)is list and len(v)==len(t));return [like(x,y) for x,y in zip(v,t)]
 req(type(v)is type(t));return v
def source_receipt(v):
 for r in RECEIPTS:
  try:
   c=like(v,r)
   if c==r:return clone(r)
  except Refusal:pass
 raise Refusal('invalid_request')
def view(v):
 for f in VIEWS:
  try:
   c=like(v,f['current_view'])
   if c==f['current_view']:return c
  except Refusal:pass
 raise Refusal('invalid_request')
def val_event(v):
 req(type(v)is dict and 'saturday'in v)
 try:e=event(v['saturday'])
 except(ValueError,TypeError):raise Refusal('invalid_request')
 req(like(v,e)==e);return e
BLOCK_INDEX=None

def headerhash(h):
 global BLOCK_INDEX
 hexval(h,160)
 if BLOCK_INDEX is None:BLOCK_INDEX=json.loads((HERE/'fixtures/normal_work.json').read_bytes())
 req(h in BLOCK_INDEX);return BLOCK_INDEX[h]['work_hash']
def validate(kind,v):
 v=exact(v,FIELDS[kind]);cap=CAPS.get(kind,262144);req(len(J(v))+(1 if kind in ['SeedRequest','SeedResponse'] else 0)<=cap)
 if 'version'in v:integer(v['version'],1,1)
 if kind=='Rules':req(v==RULES)
 elif kind=='Pin':req(like(v,PIN)==PIN);v=clone(PIN)
 elif kind in ('Info','Marker','Init','Metadata'):
  hexval(v['store_id']);req(v['store_id']==STORE_ID);v['pin']=validate('Pin',v['pin'])
  if kind=='Marker':req(v['application']=='slug-candidate-seeds')
  if kind in ('Info','Metadata'):
   time(v['created_at_ms']);integer(v['event_count'],0,256)
  if kind=='Info':
   integer(v['funding_height'],-1,I32);hexval(v['funding_hash'],empty=True);req((v['funding_height']==-1)==(v['funding_hash']==''));boolean(v['funding_complete']);integer(v['funding_generation'],0,16);integer(v['funding_restart_count'],0,16);req(v['funding_generation']==v['funding_restart_count']);req(not v['funding_complete'] or v['funding_height']>=0)
  if kind=='Metadata':
   time(v['clock_ms']);req(v['clock_ms']>=v['created_at_ms']);v['funding_cursor']=validate('Cursor',v['funding_cursor']);req(type(v['funding_restarts'])is list and len(v['funding_restarts'])<=16);v['funding_restarts']=[validate('ScanRestart',x) for x in v['funding_restarts']];boolean(v['funding_complete']);req(v['funding_cursor']['generation']==len(v['funding_restarts']));
   if v['last_funding_tip']is not None:v['last_funding_tip']=validate('Tip',v['last_funding_tip'])
   if v['funding_complete']:req(v['last_funding_tip']is not None and v['funding_cursor']['height']==v['last_funding_tip']['height'] and v['funding_cursor']['hash']==v['last_funding_tip']['hash'])
 elif kind=='Declaration':
  for k in ['event_key','store_id','source_store_id','snapshot_digest','receipt_digest','publisher_chain_id','genesis','floor_hash','rules_id','publisher_public_key','checkpoint_public_key']:hexval(v[k])
  v['event']=val_event(v['event']);req(v['event_key']==event_key(v['event']));integer(v['network'],NET,NET);integer(v['floor_height'],0,0)
  for k in ['source_store_id','genesis','floor_hash','publisher_public_key']:req(v[k]==PIN[k])
  # Alternate canonical StoreID/rules/snapshot/receipt remain authenticated conflict evidence.
 elif kind=='SignedDeclaration':
  v['declaration']=validate('Declaration',v['declaration']);hexval(v['signature'],128);req(ed_verify(DECL_PUB,domain('candidate seed declaration')+lp(J(v['declaration'])),raw(v['signature'])))
 elif kind=='Outpoint':hexval(v['txid']);integer(v['index'],0,U32)
 elif kind=='FundingCoin':
  v['outpoint']=validate('Outpoint',v['outpoint']);integer(v['value_satoshis'],1,MAXSAT);req(v['script']==FUND_SCRIPT.hex());integer(v['block_height'],0,I32);hexval(v['block_hash']);boolean(v['coinbase'])
 elif kind=='Publication':
  v['coin']=validate('FundingCoin',v['coin']);string(v['raw_tx']);req(0<len(v['raw_tx'])<=2048 and len(v['raw_tx'])%2==0 and re.fullmatch('[0-9a-f]+',v['raw_tx']));hexval(v['txid']);time(v['built_at_ms']);integer(v['dispatch_count'],0,U32);time(v['first_dispatch_at_ms'],True);time(v['last_dispatch_at_ms'],True);enum(v['state'],['retained','unknown','submitted'])
  rr=Reader(raw(v['raw_tx']));tx=parse_tx(rr);req(rr.p==len(rr.b) and len(tx['outputs'])==2);script=raw(tx['outputs'][0]['script']);req(len(script)==75 and script[:11]==b'\x6a\x49SLUGSEED\x01');expected=publication(v['coin'],script[11:43].hex(),script[43:75].hex());req(v['raw_tx']==expected['raw_tx'] and v['txid']==expected['txid'])
  if v['dispatch_count']==0:req(v['state']=='retained' and v['first_dispatch_at_ms']==v['last_dispatch_at_ms']==0)
  else:req(v['state']!='retained' and v['built_at_ms']<=v['first_dispatch_at_ms']<=v['last_dispatch_at_ms'])
 elif kind=='Cursor':
  integer(v['generation'],0,16);integer(v['height'],-1,I32);integer(v['complete_to_height'],-1,I32);hexval(v['hash'],empty=True);hexval(v['complete_to_hash'],empty=True);hexval(v['scan_digest']);req((v['height']==-1)==(v['hash']==''));req((v['complete_to_height']==-1)==(v['complete_to_hash']==''));req(v['complete_to_height']<=v['height']);
  if v['height']==v['complete_to_height']:req(v['hash']==v['complete_to_hash'])
 elif kind=='Tip':integer(v['height'],0,I32);hexval(v['hash'])
 elif kind=='Anchor':
  integer(v['height'],0,I32);hexval(v['hash']);req(headerhash(v['header'])==v['hash']);hexval(v['txid']);hexval(v['raw_tx_digest']);integer(v['raw_tx_size'],1,4000000);integer(v['tx_count'],1,1000000);integer(v['tx_index'],0,v['tx_count']-1);integer(v['output_index'],0,U32);hexval(v['payload'],146);req(raw(v['payload'])[:9]==b'SLUGSEED\x01');req(type(v['merkle_branch'])is list and len(v['merkle_branch'])<=20)
  for h in v['merkle_branch']:hexval(h)
  req(verify_merkle(v['txid'],v['tx_index'],v['tx_count'],v['merkle_branch'],raw(v['header'])[36:68][::-1].hex()))
 elif kind=='ScanRestart':
  integer(v['generation'],1,16);v['previous']=validate('Cursor',v['previous']);req(v['previous']['height']>=0 and v['generation']==v['previous']['generation']+1);time(v['observed_at_ms']);v['observed_tip']=validate('Tip',v['observed_tip']);hexval(v['replacement_hash'],empty=True)
  req((v['observed_tip']['height']<v['previous']['height'] and v['replacement_hash']=='') or(v['observed_tip']['height']>=v['previous']['height'] and v['replacement_hash']!='' and v['replacement_hash']!=v['previous']['hash']))
 elif kind=='Target':
  integer(v['height'],1,I32);hexval(v['hash']);req(headerhash(v['header'])==v['hash']);time(v['observed_at_ms']);v['observed_tip']=validate('Tip',v['observed_tip']);req(v['observed_tip']['height']>=v['height'])
 elif kind=='SignedCheckpoint':
  integer(v['network'],NET,NET);req(v['genesis']==GENESIS);integer(v['height'],1,I32);hexval(v['hash']);req(v['hash']!='00'*32);hexval(v['signature'],128);req(ed_verify(CHECK_PUB,checkpoint_pre(v),raw(v['signature'])))
 elif kind=='Finality':
  v['checkpoint']=validate('SignedCheckpoint',v['checkpoint']);time(v['observed_at_ms']);v['observed_tip']=validate('Tip',v['observed_tip']);integer(v['target_descendants'],60,I32);req(v['target_descendants']==v['observed_tip']['height']-v['checkpoint']['height'])
 elif kind=='Conflict':
  enum(v['kind'],['declaration','checkpoint','chain']);time(v['observed_at_ms']);integer(v['expected_height'],0,I32);hexval(v['expected_hash'],empty=True);hexval(v['observed_hash'],empty=True)
  for k,typ in [('declaration','SignedDeclaration'),('checkpoint','SignedCheckpoint'),('observed_tip','Tip')]:
   if v[k]is not None:v[k]=validate(typ,v[k])
  if v['kind']=='chain':req(v['declaration']is None and v['checkpoint']is None and v['observed_tip']is not None and v['expected_hash']!='' and v['expected_hash']!=v['observed_hash']);req(v['observed_hash']!='' or v['observed_tip']['height']<v['expected_height'])
  else:req(v[v['kind']]is not None and v['observed_tip']is None and v['checkpoint' if v['kind']=='declaration' else 'declaration']is None and v['expected_height']==0 and v['expected_hash']==v['observed_hash']=='')
 elif kind=='Observation':
  time(v['observed_at_ms'],True);time(v['chain_observed_at_ms'],True);req(v['chain_observed_at_ms']<=v['observed_at_ms']);enum(v['node_state'],['unobserved','online','offline','unavailable']);integer(v['target_descendants'],0,I32);boolean(v['current_finality']);req((v['tip']is None)==(v['chain_observed_at_ms']==0))
  if v['tip']is not None:v['tip']=validate('Tip',v['tip'])
  if v['node_state']=='unobserved':req(v['observed_at_ms']==0 and v['tip']is None)
  if v['node_state']=='online':req(v['chain_observed_at_ms']==v['observed_at_ms'] and v['tip']is not None)
  if v['tip']is None:req(v['target_descendants']==0 and not v['current_finality'])
 elif kind=='Record':
  req(v['store_id']==STORE_ID);hexval(v['event_key']);v['source_receipt']=source_receipt(v['source_receipt']);v['declaration']=validate('SignedDeclaration',v['declaration']);d=declaration(v['source_receipt']);req(v['declaration']['declaration']==d and v['event_key']==d['event_key']);time(v['prepared_at_ms']);req(v['prepared_at_ms']>=v['source_receipt']['completed_at_ms']);v['cursor']=validate('Cursor',v['cursor']);v['observation']=validate('Observation',v['observation']);time(v['incident_at_ms'],True)
  for k,t in [('publication','Publication'),('anchor','Anchor'),('target','Target'),('checkpoint','SignedCheckpoint'),('finality','Finality')]:
   if v[k]is not None:v[k]=validate(t,v[k])
  for k,t,limit in [('scan_restarts','ScanRestart',16),('conflicts','Conflict',8)]:
   req(type(v[k])is list and len(v[k])<=limit);v[k]=[validate(t,x) for x in v[k]]
  boolean(v['conflict_overflow']);req(not v['conflict_overflow'] or len(v['conflicts'])==8);req(v['cursor']['generation']==len(v['scan_restarts']))
  for i,x in enumerate(v['scan_restarts']):req(x['generation']==i+1)
  if v['cursor']['height']==-1:req(v['cursor']['scan_digest']==scan_initial(v['event_key'],v['cursor']['generation']))
  if v['publication']is not None:
   p=v['publication'];expected=publication(p['coin'],v['event_key'],decl_id(d));req(p['raw_tx']==expected['raw_tx'] and p['txid']==expected['txid'] and p['built_at_ms']>=v['prepared_at_ms'])
  if v['anchor']is not None:
   a=v['anchor'];req(a['payload']==(b'SLUGSEED\x01'+raw(v['event_key'])+raw(decl_id(d))).hex());req(v['cursor']['height']==v['cursor']['complete_to_height']==a['height'] and v['cursor']['hash']==v['cursor']['complete_to_hash']==a['hash'])
  if v['target']is not None:
   req(v['anchor']is not None);req(v['target']['height']==v['anchor']['height']+1 and raw(v['target']['header'])[4:36][::-1].hex()==v['anchor']['hash'])
  if v['checkpoint']is not None:req(v['target']is not None and v['checkpoint']['height']==v['target']['height'] and v['checkpoint']['hash']==v['target']['hash'])
  if v['finality']is not None:req(v['checkpoint']is not None and v['finality']['checkpoint']==v['checkpoint'])
  if v['observation']['current_finality']:req(v['finality']is not None and not v['conflicts'] and not v['conflict_overflow'] and v['observation']['target_descendants']>=60)
  seen=set()
  for c in v['conflicts']:
   key=sha(J({k:x for k,x in c.items()if k!='observed_at_ms'}));req(key not in seen);seen.add(key)
   if c['kind']=='declaration':req(c['declaration']['declaration']['event_key']==v['event_key'] and c['declaration']['declaration']!=d)
   elif c['kind']=='checkpoint':req(v['target']is not None and c['checkpoint']['height']==v['target']['height'] and c['checkpoint']['hash']!=v['target']['hash'])
   else:req(any(x is not None and x['height']==c['expected_height']and x['hash']==c['expected_hash']for x in [v['anchor'],v['target']]))
  if v['target']is None:req(v['observation']['target_descendants']==0)
  if v['incident_at_ms']:req(v['incident_at_ms']>=d['event']['incident_deadline_ms'])
 elif kind=='Status':
  req(v['store_id']==STORE_ID);v['event']=val_event(v['event']);req(v['event_key']==event_key(v['event']));boolean(v['exists']);enum(v['phase'],['absent','prepared','waiting_anchor','waiting_target','provisional','finalized','conflict']);enum(v['publication_state'],['none','retained','unknown','submitted']);integer(v['scan_height'],-1,I32);integer(v['scan_tip_height'],-1,I32)
  for k in ['anchor_height','target_height','target_descendants']:integer(v[k],0,I32)
  for k in ['anchor_hash','target_hash']:hexval(v[k],empty=True)
  for k in ['checkpoint_present','conflict_overflow','current_finality','escalation_due']:boolean(v[k])
  for k in ['finalized_at_ms','incident_at_ms','prepared_at_ms','observed_at_ms','chain_observed_at_ms']:time(v[k],True)
  integer(v['conflict_count'],0,8);integer(v['scan_generation'],0,16);integer(v['funding_generation'],0,16);enum(v['node_state'],['unobserved','online','offline','unavailable']);enum(v['funding_state'],['unobserved','scanning','ready','waiting_for_funds','unavailable']);req(v['chain_observed_at_ms']<=v['observed_at_ms']);req(v['escalation_due']==(v['observed_at_ms']>=v['event']['incident_deadline_ms'] and (v['finalized_at_ms']==0 or v['conflict_count']>0 or v['conflict_overflow'])))
  req(v['exists']==(v['phase']!='absent'))
  if not v['exists']:
   for k in ['prepared_at_ms','finalized_at_ms','incident_at_ms','anchor_height','target_height','target_descendants','conflict_count','scan_generation']:req(v[k]==0)
   req(v['scan_height']==v['scan_tip_height']==-1 and not v['checkpoint_present'] and not v['current_finality'] and not v['conflict_overflow'] and v['publication_state']=='none' and v['anchor_hash']==v['target_hash']=='')
  if v['current_finality']:req(v['checkpoint_present'] and v['finalized_at_ms']>0 and v['target_descendants']>=60 and v['conflict_count']==0 and not v['conflict_overflow'])
 elif kind=='HistoryPage':
  req(v['store_id']==STORE_ID);hexval(v['after_event_key'],empty=True);hexval(v['next_event_key'],empty=True);boolean(v['has_more']);time(v['observed_at_ms']);req(type(v['events'])is list and len(v['events'])<=8);v['events']=[validate('Status',x) for x in v['events']];req(all(x['exists'] and x['observed_at_ms']==v['observed_at_ms'] for x in v['events']));keys=[(x['event']['cutoff_ms'],x['event_key']) for x in v['events']];req(keys==sorted(set(keys)));req(v['next_event_key']==(v['events'][-1]['event_key'] if v['has_more'] and v['events'] else ''));req(not v['has_more'] or bool(v['events']))
 elif kind=='SeedRequest':
  integer(v['id'],1,SAFE);enum(v['command'],['seed_'+s for s in ['snapshot','status','record','history','prepare','publish','reconcile','attest','job','cancel']]);hexval(v['view_id'],empty=True);hexval(v['after_event_key'],empty=True);hexval(v['job_id'],empty=True);string(v['saturday']);integer(v['limit'],0,8);c=v['command'][5:]
  if c=='snapshot':req(all(v[k]=='' for k in ['view_id','saturday','after_event_key','job_id']) and v['limit']==0)
  else:
   req(v['view_id']!='')
   if c in ['status','record','prepare','publish','reconcile','attest']:
    try:dateparts(v['saturday'])
    except ValueError:raise Refusal('invalid_request')
    req(v['after_event_key']==v['job_id']=='' and v['limit']==0)
   elif c=='history':req(v['saturday']==v['job_id']=='' and v['limit']>=1)
   else:req(v['job_id']!='' and v['saturday']==v['after_event_key']=='' and v['limit']==0)
 elif kind=='SeedSnapshot':
  enum(v['state'],['ready','busy','setup_incomplete','unavailable','failed']);string(v['error']);hexval(v['store_id'],empty=True);hexval(v['publisher_account_id'],empty=True);time(v['created_at_ms'],True);enum(v['node_state'],['ready','offline'])
  for k in ['active_job_id','queued_job_id','last_job_id']:hexval(v[k],empty=True)
  if v['state']in ['ready','busy','failed']:
   v['view']=view(v['view']);v['pin']=validate('Pin',v['pin']);req(v['store_id']==STORE_ID and v['publisher_account_id']==PIN['publisher_account_id'] and v['created_at_ms']>0)
   req(v['error']in ['seed_unavailable','seed_corrupt','seed_clock'] if v['state']=='failed' else v['error']=='')
   if v['state']=='ready':req(v['active_job_id']==v['queued_job_id']=='')
   req(not v['active_job_id'] or v['active_job_id']!=v['queued_job_id'])
  else:
   req(v['pin']is None and v['created_at_ms']==0 and v['store_id']==v['publisher_account_id']==v['active_job_id']==v['queued_job_id']==v['last_job_id']=='' and v['node_state']=='offline')
   if v['state']=='setup_incomplete':v['view']=view(v['view']);req(v['error']=='seed_setup_incomplete')
   else:req(v['view']is None and v['error']=='seed_unavailable')
 elif kind=='SeedJob':
  hexval(v['job_id']);enum(v['kind'],['prepare','publish','reconcile','attest']);enum(v['state'],['queued','running','completed','failed','cancelled']);v['view']=view(v['view']);req(v['store_id']==STORE_ID);hexval(v['event_key']);req(v['event_key']==event_key(event(v['saturday'])));string(v['error'])
  if v['state']=='completed':req(v['error']=='');v['result']=validate('Record',v['result']);req(v['result']['event_key']==v['event_key'])
  else:
   req(v['result']is None)
   if v['state']in ['queued','running']:req(v['error']=='')
   elif v['state']=='cancelled':req(v['error']=='seed_cancelled')
   else:req(v['error']in ERRORS)
 elif kind=='SeedResponse':
  integer(v['id'],1,SAFE);boolean(v['ok']);string(v['error']);hexval(v['store_id'],empty=True);names=['snapshot','status','record','history','job'];kinds=['SeedSnapshot','Status','Record','HistoryPage','SeedJob']
  if not v['ok']:req(v['error']in ERRORS and v['view']is None and v['store_id']=='' and all(v[x]is None for x in names))
  else:
   req(v['error']=='' and sum(v[x]is not None for x in names)==1)
   for k,t in zip(names,kinds):
    if v[k]is not None:v[k]=validate(t,v[k])
   if v['snapshot']is not None and v['snapshot']['state']=='unavailable':req(v['view']is None and v['store_id']=='')
   else:
    v['view']=view(v['view']);setup=v['snapshot']is not None and v['snapshot']['state']=='setup_incomplete';req(v['store_id']==('' if setup else STORE_ID))
    for k in ['snapshot','job']:
     if v[k]is not None:req(v[k]['view']==v['view'] and v[k]['store_id']==v['store_id'])
 return v

def parse(kind,b):
 req(type(b)is bytes and 0<len(b) and len(b)+(1 if kind in ['SeedRequest','SeedResponse'] else 0)<=CAPS.get(kind,262144))
 def pairs(ps):
  d={}
  for k,v in ps:req(k not in d);d[k]=v
  return d
 def number(s):req(bool(re.fullmatch('0|[1-9][0-9]*|-1',s)));return int(s)
 try:v=json.loads(b.decode('utf8'),object_pairs_hook=pairs,parse_int=number,parse_float=lambda s:req(False),parse_constant=lambda s:req(False))
 except(ValueError,UnicodeError,RecursionError):raise Refusal('invalid_request')
 def pre(x,depth=0):
  if type(x)in(dict,list):req(depth+1<=16)
  if type(x)is dict:
   req(len(x)<=64)
   for k,y in x.items():req(len(k)<=64);pre(y,depth+1)
  elif type(x)is list:
   req(len(x)<=20)
   for y in x:pre(y,depth+1)
  elif type(x)is str:req(x.isascii())
 pre(v)
 try:return validate(kind,v)
 except (TypeError,KeyError,OverflowError,ValueError,IndexError) as e:
  if isinstance(e,Refusal):raise
  raise Refusal('invalid_request')

def correlate(request,response,current_view,expected_job=None,terminal_digest=None,created_at_ms=None):
 q=validate('SeedRequest',request);r=validate('SeedResponse',response);req(q['id']==r['id'])
 if not r['ok']:return r
 c=q['command'][5:];slot=c if c in ['snapshot','status','record','history'] else 'job';req(r[slot]is not None)
 unavailable=slot=='snapshot' and r[slot]['state']=='unavailable'
 if not unavailable:req(r['view']==current_view and (q['view_id']=='' or q['view_id']==current_view['view_id']))
 if slot=='snapshot':
  if created_at_ms is not None and r['snapshot']['pin']is not None:req(r['snapshot']['created_at_ms']==created_at_ms)
 elif slot in ['status','record']:
  e=r[slot]['event'] if slot=='status' else r[slot]['source_receipt']['input']['event'];req(e['saturday']==q['saturday'])
 elif slot=='history':req(r['history']['after_event_key']==q['after_event_key'] and len(r['history']['events'])<=q['limit'])
 else:
  j=r['job']
  if c in ['prepare','publish','reconcile','attest']:req(j['kind']==c and j['saturday']==q['saturday'] and j['state']=='queued')
  else:req(j['job_id']==q['job_id'])
  if expected_job is not None:
   for k in ['job_id','kind','view','store_id','event_key','saturday']:
    if k in expected_job and expected_job[k] is not None:req(j[k]==expected_job[k])
   if expected_job.get('state')=='running':req(j['state']!='queued')
   if expected_job.get('state')in ['completed','failed','cancelled']:req(j['state']==expected_job['state'])
  if terminal_digest is not None:req(sha(J(j))==terminal_digest)
 return r
