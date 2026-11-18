"""Closed Set48 core DTO codecs; full semantics and cheap transport separate.
No engine, signing, RPC or proof hash. Compact sources do not prove omitted rows.
"""
from common import *
import crypto_result as R
FIELDS={
'Pin':'version source_store_id source_pin publisher_gym_id publisher_account_id publisher_source_authority_id publisher_chain_id network genesis floor_height floor_hash publisher_public_key checkpoint_public_key funding_public_key funding_script rules_id',
'Info':'version store_id pin source_created_at_ms created_at_ms result_count funding_height funding_hash funding_complete funding_generation funding_restart_count',
'ResultSummary':'winner method end_round end_tick',
'ResultSource':'version store_id store_created_at_ms record_digest source_digest input_id result_digest created_at_ms source summary',
'Statement':'version result_key store_id source_store_id source publisher_chain_id network genesis rules_id publisher_public_key checkpoint_public_key',
'SignedStatement':'version statement signature',
'Publication':'version raw_tx txid coin built_at_ms dispatch_count first_dispatch_at_ms last_dispatch_at_ms state',
'Inclusion':'version generation anchor observed_at_ms observed_tip',
'PriorInclusion':'version inclusion orphaned_at_ms observed_tip replacement_hash',
'Finality':'checkpoint inclusion observed_at_ms observed_tip descendants',
'Conflict':'kind observed_at_ms expected_height expected_hash observed_hash observed_tip checkpoint',
'Observation':'observed_at_ms chain_observed_at_ms tip node_state descendants current_finality',
'Record':'version store_id result_key statement prepared_at_ms publication cursor scan_restarts inclusion prior_inclusions checkpoint finality equivocation conflicts conflict_overflow observation',
'Status':'version store_id result_key card_id bout_id event exists phase record_digest source_record_digest input_id result_digest prepared_at_ms publication_state txid scan_height scan_tip_height scan_generation inclusion_height inclusion_hash inclusion_generation prior_inclusion_count descendants checkpoint_present finalized_at_ms conflict_count conflict_overflow observed_at_ms chain_observed_at_ms node_state funding_state current_finality source',
'ActionResult':'version record status','ReadResult':'version record status',
'Tip':'height hash','Outpoint':'txid index','FundingCoin':'outpoint value_satoshis script block_height block_hash coinbase',
'Anchor':'height hash header txid raw_tx_digest raw_tx_size tx_count tx_index output_index payload merkle_branch',
'Cursor':'generation height hash scan_digest complete_to_height complete_to_hash',
'ScanRestart':'generation previous observed_at_ms observed_tip replacement_hash',
'SignedCheckpoint':'version network genesis height hash signature',
'ProofSummary':'version result_key card_id bout_id record_digest source_record_digest input_id result_digest txid sha256 bytes header_count block_count finalized_at_ms observed_tip',
'Marker':'application version store_id pin','Init':'version store_id pin source_created_at_ms',
'Metadata':'version store_id pin source_created_at_ms created_at_ms clock_ms result_count funding_cursor funding_restarts funding_complete last_funding_tip',
'Reservation':'version result_key txid','IndexEntry':'result_key saturday bout_id record_digest',
'ProofHeader':'height hash header','ProofBlock':'height hash raw_block',
'PublicProof':'version scope pin result status bout card observed_tip result_finality_tip source_finality_tip headers blocks'}
FIELDS={k:v.split()for k,v in FIELDS.items()}
CAPS=dict(Pin=32768,Info=36864,ResultSummary=2048,ResultSource=20480,Statement=24576,SignedStatement=25600,Publication=4096,Anchor=2048,Inclusion=3072,PriorInclusion=4096,Finality=8192,ScanRestart=1024,Conflict=2048,Record=196608,Status=16384,ActionResult=229376,ReadResult=229376,ProofSummary=2048,Marker=36864,Init=36864,Metadata=65536,Tip=256,Outpoint=256,FundingCoin=1024,Cursor=512,Observation=1024,SignedCheckpoint=4096,Reservation=256,IndexEntry=512,ProofHeader=512,ProofBlock=8000200,PublicProof=25165824)
PINS=[R.candidate_pin(BOUT_PIN)]
METHODS=INHERITED.METHODS
req=INHERITED.req;integer=INHERITED.integer;boolean=INHERITED.boolean;string=INHERITED.string;hx=INHERITED.hx;time=INHERITED.time;enum=INHERITED.enum;array=INHERITED.array

def hexstr(v,n=None,lo=0,hi=8192):
 string(v,lo,hi);req(re.fullmatch('[0-9a-f]*',v)is not None and len(v)%2==0)
 if n is not None:req(len(v)==2*n)
def fields(k,v):
 req(type(v)is dict and set(v)==set(FIELDS[k]));v={f:v[f]for f in FIELDS[k]}
 if 'version'in v:integer(v['version'],1,1)
 return v
def pin(v):
 p=INHERITED.ordered(v,PINS[0]);req(p==PINS[0]);return p

def summary(v,source=None):
 v=fields('ResultSummary',v);hx(v['winner'],True);enum(v['method'],METHODS);integer(v['end_round'],1,3);integer(v['end_tick'],1,60)
 draw=v['method']in ['decision_draw','double_corner_stoppage'];req((v['winner']=='')==draw)
 if source and not draw:req(v['winner']in [source['bout']['corner_a'],source['bout']['corner_b']])
 return v

def source(v,p,context):
 v=fields('ResultSource',v);req(v['store_id']==p['source_store_id']);time(v['store_created_at_ms']);time(v['created_at_ms']);req(v['created_at_ms']>=v['store_created_at_ms'])
 for k in ['record_digest','source_digest','input_id','result_digest']:hx(v[k])
 v['source']=INHERITED.source(v['source'],p['source_pin']['source_pin'],context);s=v['source'];req(v['created_at_ms']>=max(s['card_created_at_ms'],s['finalized_at_ms']));req(v['source_digest']==OLD.source_digest(s));v['summary']=summary(v['summary'],s)
 return v

def signed_statement(v,p,context):
 v=fields('SignedStatement',v);v['statement']=validate('Statement',v['statement'],p,context);hexstr(v['signature'],64)
 if context!='transport':req(CRYPTO.ed_verify(raw(p['publisher_public_key']),R.signing_bytes(v['statement']),raw(v['signature'])))
 return v

def tip(v):
 v=fields('Tip',v);integer(v['height'],0,I32);hx(v['hash']);return v

def coin(v,p):
 v=fields('FundingCoin',v);v['outpoint']=fields('Outpoint',v['outpoint']);hx(v['outpoint']['txid']);integer(v['outpoint']['index'],0,U32)
 integer(v['value_satoshis'],1,21000000*100000000);req(v['script']==p['funding_script']);integer(v['block_height'],0,I32);hx(v['block_hash']);boolean(v['coinbase']);return v

def publication(v,p,payload=None,context='core'):
 v=fields('Publication',v);hexstr(v['raw_tx'],lo=2,hi=2048);hx(v['txid']);v['coin']=coin(v['coin'],p);req(v['coin']['value_satoshis']>=1294)
 for k in ['built_at_ms','first_dispatch_at_ms','last_dispatch_at_ms']:time(v[k],k!='built_at_ms')
 integer(v['dispatch_count'],0,U32);enum(v['state'],['built','dispatch_unknown','submitted'])
 if v['dispatch_count']==0:req(v['first_dispatch_at_ms']==v['last_dispatch_at_ms']==0 and v['state']=='built')
 else:req(v['built_at_ms']<=v['first_dispatch_at_ms']<=v['last_dispatch_at_ms']and v['state']!='built')
 # Metadata transport can inspect public serialization/txid but never re-sign.
 if context=='core':
  reader=CRYPTO.Reader(raw(v['raw_tx']));tx=CRYPTO.parse_tx(reader);req(reader.p==len(raw(v['raw_tx']))and tx['txid']==v['txid']);outputs=tx['outputs'];req(len(outputs)==2 and outputs[0]['value']==0)
  script=raw(outputs[0]['script']);req(script[:2]==b'\x6a\x49'and len(script)==75 and script[2:11]==b'SLUGRSLT\x01'and script[11:43]!=bytes(32)and script[43:]!=bytes(32))
  payload=script[2:]if payload is None else payload;req(script[2:]==payload);req(R.validate_publication(v['coin'],payload,v['raw_tx'])['txid']==v['txid'])
 return v

def cursor(v):
 v=fields('Cursor',v);integer(v['generation'],0,16);integer(v['height'],-1,I32);integer(v['complete_to_height'],-1,I32);hx(v['hash'],v['height']==-1);hx(v['complete_to_hash'],v['complete_to_height']==-1);hx(v['scan_digest'])
 req((v['hash']=='')==(v['height']==-1));req((v['complete_to_hash']=='')==(v['complete_to_height']==-1));req(v['complete_to_height']<=v['height'])
 if v['complete_to_height']==v['height']:req(v['complete_to_hash']==v['hash'])
 return v

def anchor(v,p,context="core"):
 v=fields('Anchor',v)
 for k in ['height','raw_tx_size','tx_count','tx_index','output_index']:integer(v[k],0,U32)
 req(1<=v['height']<=I32 and 1<=v['raw_tx_size']<=1024 and 1<=v['tx_count']<=1000000 and v['tx_index']<v['tx_count']and v['output_index']==0)
 for k in ['hash','txid','raw_tx_digest']:hx(v[k])
 hexstr(v['header'],80);hexstr(v['payload'],73);req(raw(v['payload']).startswith(b'SLUGRSLT\x01'));array(v['merkle_branch'],0,20)
 for h in v['merkle_branch']:hx(h)
 if context=='core':
  root=raw(v['header'])[36:68][::-1].hex();req(CRYPTO.verify_merkle(v['txid'],v['tx_index'],v['tx_count'],v['merkle_branch'],root))
 return v

def inclusion(v,p,context="core"):
 v=fields('Inclusion',v);integer(v['generation'],0,16);v['anchor']=anchor(v['anchor'],p,context);time(v['observed_at_ms']);v['observed_tip']=tip(v['observed_tip']);req(v['anchor']['height']<=v['observed_tip']['height']);
 if v['anchor']['height']==v['observed_tip']['height']:req(v['anchor']['hash']==v['observed_tip']['hash'])
 return v

def checkpoint(v,p,context):
 v=fields('SignedCheckpoint',v);req(v['network']==p['network']and v['genesis']==p['genesis']);integer(v['height'],1,I32);hx(v['hash']);hexstr(v['signature'],64)
 if context=='core':req(CRYPTO.ed_verify(raw(p['checkpoint_public_key']),R.checkpoint_bytes(v),raw(v['signature'])))
 return v

def observation(v):
 v=fields('Observation',v)
 for k in ['observed_at_ms','chain_observed_at_ms']:time(v[k],True)
 req(v['chain_observed_at_ms']<=v['observed_at_ms']);enum(v['node_state'],['unknown','online','offline']);integer(v['descendants'],0,I32);boolean(v['current_finality'])
 if v['tip']is not None:v['tip']=tip(v['tip'])
 else:req(v['chain_observed_at_ms']==v['descendants']==0 and not v['current_finality'])
 if v['current_finality']:req(v['chain_observed_at_ms']>0 and v['descendants']>=60 and v['node_state']=='online')
 return v

def record(v,p,context):
 v=fields('Record',v);req(v['store_id']==R.store_id(p));hx(v['result_key']);v['statement']=signed_statement(v['statement'],p,context);s=v['statement']['statement'];req(v['result_key']==s['result_key']);time(v['prepared_at_ms']);req(v['prepared_at_ms']>=max(s['source']['created_at_ms'],s['source']['store_created_at_ms']))
 times=[v['prepared_at_ms']];payload=R.carrier(s)
 if v['publication']is not None:
  v['publication']=publication(v['publication'],p,payload,context);pub=v['publication'];req(pub['built_at_ms']>=v['prepared_at_ms']);times +=[pub['built_at_ms'],pub['last_dispatch_at_ms']]
 v['cursor']=cursor(v['cursor']);array(v['scan_restarts'],0,16);v['scan_restarts']=[validate('ScanRestart',x,p,context)for x in v['scan_restarts']]
 req(v['cursor']['generation']==len(v['scan_restarts']))
 for i,x in enumerate(v['scan_restarts']):req(x['generation']==i+1 and x['previous']['generation']==i);times.append(x['observed_at_ms'])
 if v['cursor']['height']==-1:req(v['cursor']['scan_digest']==R.scan_initial(p,v['result_key'],v['cursor']['generation'])['scan_digest'])
 if v['inclusion']is not None:
  v['inclusion']=inclusion(v['inclusion'],p,context);inc=v['inclusion'];a=inc['anchor'];req(v['publication']is not None and inc['observed_at_ms']>=v['prepared_at_ms']);req(a['payload']==payload.hex()and a['txid']==pub['txid']and a['raw_tx_digest']==sha(raw(pub['raw_tx']))and a['raw_tx_size']==len(raw(pub['raw_tx'])));times.append(inc['observed_at_ms'])
 array(v['prior_inclusions'],0,16);v['prior_inclusions']=[validate('PriorInclusion',x,p,context)for x in v['prior_inclusions']]
 for i,x in enumerate(v['prior_inclusions']):
  req(x['inclusion']['generation']==i and x['inclusion']['anchor']['payload']==payload.hex());req(v['publication']is not None and x['inclusion']['anchor']['txid']==pub['txid']);req(x['inclusion']['anchor']['raw_tx_digest']==sha(raw(pub['raw_tx']))and x['inclusion']['anchor']['raw_tx_size']==len(raw(pub['raw_tx'])));times.append(x['orphaned_at_ms'])
 if v['inclusion']is not None:req(v['inclusion']['generation']==len(v['prior_inclusions']))
 if v['checkpoint']is not None:
  v['checkpoint']=checkpoint(v['checkpoint'],p,context);req(v['inclusion']is not None);req(v['checkpoint']['height']==a['height']and v['checkpoint']['hash']==a['hash'])
 if v['finality']is not None:
  v['finality']=validate('Finality',v['finality'],p,context);req(v['finality']['checkpoint']==v['checkpoint']and v['finality']['inclusion']==v['inclusion']);times.append(v['finality']['observed_at_ms'])
 if v['equivocation']is not None:
  v['equivocation']=signed_statement(v['equivocation'],p,'transport'if context=='transport'else'evidence');eq=v['equivocation']['statement'];req(eq['result_key']==v['result_key']and eq!=s)
 array(v['conflicts'],0,8);v['conflicts']=[validate('Conflict',x,p,context)for x in v['conflicts']];boolean(v['conflict_overflow']);times +=[x['observed_at_ms']for x in v['conflicts']]
 if v['conflict_overflow']:req(len(v['conflicts'])==8 or v['equivocation']is not None)
 v['observation']=observation(v['observation']);obs=v['observation']
 if obs['observed_at_ms']:req(max(times)<=obs['observed_at_ms'])
 else:req(v['publication']is None and not v['scan_restarts']and v['inclusion']is None and not v['conflicts']and v['equivocation']is None)
 if obs['current_finality']:req(v['finality']is not None and not v['conflicts']and not v['conflict_overflow']and v['equivocation']is None)
 return v

def source_status(v,p):return INHERITED.status_value(v,p['source_pin'])

def status(v,p):
 v=fields('Status',v);req(v['store_id']==R.store_id(p));v['source']=source_status(v['source'],p);ss=v['source'];saved=ss['saved'];v['event']=INHERITED.event(v['event'],p['source_pin']['source_pin']);req(v['event']==ss['source']['event']);req(v['card_id']==ss['card_id']and v['bout_id']==saved['bout']['bout_id']);req(v['result_key']==R.result_key(p,v['card_id'],v['bout_id']));req(v['observed_at_ms']==ss['observed_at_ms']);time(v['observed_at_ms'])
 for k in ['exists','checkpoint_present','conflict_overflow','current_finality']:boolean(v[k])
 enum(v['phase'],['absent','prepared','provisional','finalized','conflict']);enum(v['publication_state'],['none','built','dispatch_unknown','submitted']);enum(v['node_state'],['unknown','online','offline']);enum(v['funding_state'],['unknown','scanning','available','waiting_for_funds'])
 for k in ['record_digest','source_record_digest','input_id','result_digest','txid','inclusion_hash']:hx(v[k],True)
 for k in ['prepared_at_ms','finalized_at_ms','chain_observed_at_ms']:time(v[k],True);req(v[k]<=v['observed_at_ms'])
 for k in ['scan_height','scan_tip_height']:integer(v[k],-1,I32)
 for k in ['scan_generation','inclusion_generation','prior_inclusion_count']:integer(v[k],0,16)
 for k in ['inclusion_height','descendants']:integer(v[k],0,I32)
 integer(v['conflict_count'],0,8);req((v['publication_state']=='none')==(v['txid']==''));req((v['inclusion_height']==0)==(v['inclusion_hash']==''))
 if not v['exists']:
  req(v['phase']=='absent'and v['publication_state']=='none'and v['scan_height']==v['scan_tip_height']==-1)
  req(all(v[k]==''for k in ['record_digest','source_record_digest','input_id','result_digest','txid','inclusion_hash']))
  req(all(v[k]==0 for k in ['prepared_at_ms','scan_generation','inclusion_height','inclusion_generation','prior_inclusion_count','descendants','finalized_at_ms','conflict_count','chain_observed_at_ms']))
  req(not any(v[k]for k in ['checkpoint_present','conflict_overflow','current_finality']))
 else:
  req(all(v[k]!=''for k in ['record_digest','source_record_digest','input_id','result_digest']));time(v['prepared_at_ms']);req(v['phase']!='absent');req(saved['exists'])
  req(v['source_record_digest']==saved['record_digest']and v['input_id']==saved['input_id']and v['result_digest']==saved['result_digest'])
 if v['finalized_at_ms']:req(v['checkpoint_present']and v['inclusion_height']>0 and v['phase']in ['finalized','conflict'])
 if v['phase']=='finalized':req(v['finalized_at_ms']>0)
 if v['phase']=='prepared':req(v['publication_state']in ['none','built']and v['inclusion_height']==v['finalized_at_ms']==0)
 if v['phase']=='provisional':req(v['publication_state']in ['dispatch_unknown','submitted']or v['inclusion_height']>0)
 if v['exists']:req(v['prepared_at_ms']>=saved['created_at_ms'])
 if v['current_finality']:req(v['finalized_at_ms']>0 and v['descendants']>=60 and v['chain_observed_at_ms']>0 and v['node_state']=='online'and v['phase']!='conflict')
 if v['conflict_count']or v['conflict_overflow']or(v['exists']and not saved['usable']):req(v['phase']=='conflict')
 return v

def phase(r,ss):
 if r['conflicts']or r['conflict_overflow']or r['equivocation']is not None or not ss['saved']['usable']:return'conflict'
 if r['finality']is not None:return'finalized'
 if r['inclusion']is not None or(r['publication']is not None and r['publication']['dispatch_count']):return'provisional'
 return'prepared'

def status_from_record(p,r,ss,funding='unknown'):
 inc=r['inclusion']if r else None;pub=r['publication']if r else None;ob=r['observation']if r else dict(observed_at_ms=0,chain_observed_at_ms=0,tip=None,node_state='unknown',descendants=0,current_finality=False);saved=ss['saved'];fin=r['finality']if r else None
 return dict(version=1,store_id=R.store_id(p),result_key=R.result_key(p,ss['card_id'],saved['bout']['bout_id']),card_id=ss['card_id'],bout_id=saved['bout']['bout_id'],event=clone(ss['source']['event']),exists=r is not None,phase=phase(r,ss)if r else'absent',record_digest=R.record_digest(p,r)if r else'',source_record_digest=r['statement']['statement']['source']['record_digest']if r else'',input_id=r['statement']['statement']['source']['input_id']if r else'',result_digest=r['statement']['statement']['source']['result_digest']if r else'',prepared_at_ms=r['prepared_at_ms']if r else 0,publication_state=pub['state']if pub else'none',txid=pub['txid']if pub else'',scan_height=r['cursor']['height']if r else-1,scan_tip_height=ob['tip']['height']if ob['tip']else-1,scan_generation=r['cursor']['generation']if r else 0,inclusion_height=inc['anchor']['height']if inc else 0,inclusion_hash=inc['anchor']['hash']if inc else'',inclusion_generation=inc['generation']if inc else 0,prior_inclusion_count=len(r['prior_inclusions'])if r else 0,descendants=ob['descendants'],checkpoint_present=r['checkpoint']is not None if r else False,finalized_at_ms=fin['observed_at_ms']if fin else 0,conflict_count=len(r['conflicts'])if r else 0,conflict_overflow=r['conflict_overflow']if r else False,observed_at_ms=ss['observed_at_ms'],chain_observed_at_ms=ob['chain_observed_at_ms'],node_state=ob['node_state'],funding_state=funding,current_finality=ob['current_finality'],source=clone(ss))

def validate(kind,value,p=None,context='core'):
 req(context in ['core','transport','evidence']);v=clone(value);p=PINS[0]if p is None else pin(p)
 if kind=='Pin':v=pin(v)
 elif kind=='ResultSummary':v=summary(v)
 elif kind=='ResultSource':v=source(v,p,'transport'if context=='evidence'else context)
 elif kind=='Statement':
  v=fields(kind,v);v['source']=source(v['source'],p,'transport'if context=='evidence'else context);req(v==R.statement(p,v['source']))
 elif kind=='SignedStatement':v=signed_statement(v,p,context)
 elif kind=='Publication':v=publication(v,p,context=context)
 elif kind=='Tip':v=tip(v)
 elif kind=='Outpoint':v=fields(kind,v);hx(v['txid']);integer(v['index'],0,U32)
 elif kind=='FundingCoin':v=coin(v,p)
 elif kind=='Cursor':v=cursor(v)
 elif kind=='Anchor':v=anchor(v,p,context)
 elif kind=='Inclusion':v=inclusion(v,p,context)
 elif kind=='ScanRestart':
  v=fields(kind,v);integer(v['generation'],1,16);v['previous']=cursor(v['previous']);req(v['generation']==v['previous']['generation']+1 and v['previous']['height']>=0);time(v['observed_at_ms']);v['observed_tip']=tip(v['observed_tip']);hx(v['replacement_hash'],True);req((v['replacement_hash']=='')==(v['observed_tip']['height']<v['previous']['height']));req(v['replacement_hash']!=v['previous']['hash'])
 elif kind=='PriorInclusion':
  v=fields(kind,v);v['inclusion']=inclusion(v['inclusion'],p,context);time(v['orphaned_at_ms']);req(v['orphaned_at_ms']>=v['inclusion']['observed_at_ms']);v['observed_tip']=tip(v['observed_tip']);hx(v['replacement_hash'],True);req((v['replacement_hash']=='')==(v['observed_tip']['height']<v['inclusion']['anchor']['height']));req(v['replacement_hash']!=v['inclusion']['anchor']['hash'])
 elif kind=='SignedCheckpoint':v=checkpoint(v,p,context)
 elif kind=='Finality':
  v=fields(kind,v);v['checkpoint']=checkpoint(v['checkpoint'],p,context);v['inclusion']=inclusion(v['inclusion'],p,context);time(v['observed_at_ms']);req(v['observed_at_ms']>=v['inclusion']['observed_at_ms']);v['observed_tip']=tip(v['observed_tip']);integer(v['descendants'],60,I32);a=v['inclusion']['anchor'];req(v['descendants']==v['observed_tip']['height']-a['height']);req(v['checkpoint']['height']==a['height']and v['checkpoint']['hash']==a['hash'])
 elif kind=='Conflict':
  v=fields(kind,v);enum(v['kind'],['source_chain_conflict','checkpoint_chain_conflict','checkpoint_equivocation','statement_equivocation']);time(v['observed_at_ms']);integer(v['expected_height'],0,I32);hx(v['expected_hash'],True);hx(v['observed_hash'],True)
  if v['observed_tip']is not None:v['observed_tip']=tip(v['observed_tip'])
  if v['checkpoint']is not None:v['checkpoint']=checkpoint(v['checkpoint'],p,context)
  if v['kind']=='checkpoint_equivocation':req(v['checkpoint']is not None and v['checkpoint']['height']==v['expected_height']and v['checkpoint']['hash']==v['observed_hash']and v['observed_hash']!=v['expected_hash'])
  elif v['kind']in ['source_chain_conflict','checkpoint_chain_conflict']:req(v['observed_tip']is not None and v['expected_hash']and v['observed_hash']!=v['expected_hash']and v['checkpoint']is None)
  else:req(v['checkpoint']is None)
 elif kind=='Observation':v=observation(v)
 elif kind=='Record':v=record(v,p,context)
 elif kind=='Status':v=status(v,p)
 elif kind in ['ActionResult','ReadResult']:
  v=fields(kind,v);v['record']=record(v['record'],p,context);v['status']=status(v['status'],p);req(v['status']==status_from_record(p,v['record'],v['status']['source'],v['status']['funding_state']));req(v['record']['prepared_at_ms']<=v['status']['observed_at_ms']);req(v['record']['observation']['observed_at_ms']<=v['status']['observed_at_ms'])
  src=v['record']['statement']['statement']['source'];bs=v['status']['source'];saved=bs['saved'];state=bs['source'];compact=src['source']
  req(saved['source_digest']==src['source_digest']and saved['created_at_ms']==src['created_at_ms']and saved['bout']==compact['bout'])
  req(all(saved[k]==src['summary'][k]for k in ['winner','method','end_round','end_tick']))
  req(all(state[k]==compact[k]for k in ['card_id','card_record_digest','card_source_digest','card_created_at_ms','event','finalized_at_ms']))
 elif kind=='Info':
  v=fields(kind,v);req(v['pin']==p and v['store_id']==R.store_id(p));v['pin']=pin(v['pin']);time(v['source_created_at_ms']);time(v['created_at_ms']);req(v['source_created_at_ms']<=v['created_at_ms']);integer(v['result_count'],0,256);integer(v['funding_height'],-1,I32);hx(v['funding_hash'],v['funding_height']==-1);req((v['funding_hash']=='')==(v['funding_height']==-1));boolean(v['funding_complete']);integer(v['funding_generation'],0,16);integer(v['funding_restart_count'],0,16);req(v['funding_generation']==v['funding_restart_count']);req(not v['funding_complete']or v['funding_height']>=0)
 elif kind in ['Marker','Init','Metadata']:
  v=fields(kind,v);v['pin']=pin(v['pin']);req(v['pin']==p and v['store_id']==R.store_id(p))
  if kind=='Marker':req(v['application']=='slug-candidate-results')
  else:time(v['source_created_at_ms'])
  if kind=='Metadata':
   time(v['created_at_ms']);time(v['clock_ms']);req(v['source_created_at_ms']<=v['created_at_ms']<=v['clock_ms']);integer(v['result_count'],0,256);v['funding_cursor']=cursor(v['funding_cursor']);array(v['funding_restarts'],0,16);v['funding_restarts']=[validate('ScanRestart',x,p,context)for x in v['funding_restarts']];boolean(v['funding_complete']);req(v['funding_cursor']['generation']==len(v['funding_restarts']))
   if v['last_funding_tip']is not None:v['last_funding_tip']=tip(v['last_funding_tip'])
   if v['funding_complete']:req(v['last_funding_tip']==dict(height=v['funding_cursor']['height'],hash=v['funding_cursor']['hash']))
 elif kind=='Reservation':v=fields(kind,v);hx(v['result_key']);hx(v['txid'])
 elif kind=='IndexEntry':
  v=fields(kind,v)
  for k in ['result_key','bout_id','record_digest']:hx(v[k])
  cutoff(v['saturday']);req(re.fullmatch('[0-9]{4}-[0-9]{2}-[0-9]{2}',v['saturday'])is not None)
 elif kind=='ProofSummary':
  v=fields(kind,v)
  for k in ['result_key','card_id','bout_id','record_digest','source_record_digest','input_id','result_digest','txid','sha256']:hx(v[k])
  req(v['result_key']==R.result_key(p,v['card_id'],v['bout_id']));integer(v['bytes'],1,24*1024*1024);integer(v['header_count'],1,512);integer(v['block_count'],1,3);time(v['finalized_at_ms'],True);v['observed_tip']=tip(v['observed_tip'])
 else:raise Refusal('unknown kind '+kind)
 req(len(J(v))<=CAPS[kind]);return v

def _pairs(pairs):
 d={}
 for k,v in pairs:
  if k in d:raise Refusal('duplicate key')
  d[k]=v
 return d

def parse(kind,b,p=None,context='core'):
 if isinstance(b,str):b=b.encode('utf8')
 req(type(b)is bytes and 0<len(b)<=CAPS[kind]);
 def num(s):
  if not re.fullmatch('-?(0|[1-9][0-9]*)',s)or s=='-0':raise Refusal('integer grammar')
  n=int(s)
  if abs(n)>SAFE:raise Refusal('safe integer')
  return n
 def nofloat(s):raise Refusal('noninteger')
 try:v=json.loads(b.decode('utf8'),object_pairs_hook=_pairs,parse_int=num,parse_float=nofloat,parse_constant=nofloat)
 except(UnicodeError,json.JSONDecodeError,RecursionError)as e:raise Refusal('JSON')from e
 def walk(x,depth=0):
  if isinstance(x,(dict,list)):
   req(depth+1<=20)
   for y in x.values()if isinstance(x,dict)else x:walk(y,depth+1)
  elif type(x)is str:req(x.isascii())
 walk(v)
 try:return validate(kind,v,p,context)
 except(AssertionError,KeyError,TypeError,OverflowError,ValueError,struct.error)as e:
  if isinstance(e,Refusal):raise
  raise Refusal('invalid_request')from e

def canonical(kind,v,p=None,context='core'):return J(validate(kind,v,p,context))
