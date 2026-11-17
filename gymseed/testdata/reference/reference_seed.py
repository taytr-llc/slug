#!/usr/bin/env python3
"""Independent Set45 expected corpus. Read only frozen contracts/extracts and our
own crypto/work fixtures; never imports new Go code or consumes its outputs."""
import argparse,datetime,hashlib,json,re,struct,sys
from decimal import Decimal,InvalidOperation
from zoneinfo import ZoneInfo
from common import *
from crypto import selftest,merkle,verify_merkle,parse_block,publication
from schema import *
from reference_state import scenarios,status,phase,BLOCKS,CHAIN,C,CREATED,DEADLINE,tip
from extra_vectors import initialization,selection,carrier_cases,fold_cases

# Finite input-recipe grammar. JSON Pointers use RFC6901 ~0/~1 escaping.
def tokens(pointer):
 req(pointer==''or pointer.startswith('/'));return [x.replace('~1','/').replace('~0','~')for x in pointer.split('/')[1:]]
def locate(v,path):
 cur=v
 for s in path:cur=cur[int(s)] if type(cur)is list else cur[s]
 return cur
def mutate(value,operations):
 v=clone(value)
 for op in operations:
  path=tokens(op['path']);req(path)
  parent=locate(v,path[:-1]);key=int(path[-1])if type(parent)is list else path[-1]
  if op['op']=='set':parent[key]=clone(op['value'])
  elif op['op']=='delete':del parent[key]
  elif op['op']=='append':locate(v,path).append(clone(op['value']))
  else:raise AssertionError('recipe operation')
 return v
def reverse_objects(v):
 if type(v)is dict:return {k:reverse_objects(v[k])for k in reversed(v)}
 if type(v)is list:return [reverse_objects(x)for x in v]
 return v
def materialize(case,dtos):
 if 'raw_hex'in case:return raw(case['raw_hex'])
 base=dtos[case['dto']]['value'];v=mutate(base,case.get('mutations',[]));fmt=case.get('format','canonical')
 if fmt=='canonical':b=J(v)
 elif fmt=='reversed_whitespace':b=(' \n'+json.dumps(reverse_objects(v),ensure_ascii=True,indent=1)+'\t\n').encode('ascii')
 elif fmt=='escaped_first_key':
  b=J(v);k=next(iter(v));old=J(k);escaped=('"\\u%04x%s"'%(ord(k[0]),k[1:])).encode();b=b.replace(old,escaped,1)
 elif fmt=='duplicate_first_key':
  b=J(v);k=next(iter(v));b=b[:-1]+b','+J(k)+b':'+J(v[k])+b'}'
 elif fmt=='duplicate_escaped_key':
  b=J(v);k=next(iter(v));escaped=('"\\u%04x%s"'%(ord(k[0]),k[1:])).encode();b=b[:-1]+b','+escaped+b':'+J(v[k])+b'}'
 elif fmt=='pad_to_cap':
  b=J(v);n=case['bytes'];req(len(b)<=n);b=b+b' '*(n-len(b))
 else:raise AssertionError(fmt)
 if 'replace_first'in case:
  a,z=case['replace_first'];assert a.encode()in b;b=b.replace(a.encode(),z.encode(),1)
 return b

def calendar_checks(rows):
 tzbytes=(HERE/'inputs/source44/America_Los_Angeles.tzif').read_bytes();assert tzbytes[:5]==b'TZif2'and tzbytes.endswith(b'\nPST8PDT,M3.2.0,M11.1.0\n')
 with (HERE/'inputs/source44/America_Los_Angeles.tzif').open('rb')as f:tz=ZoneInfo.from_file(f,key='Pinned/Los_Angeles')
 old=[]
 for row in rows:
  s=row['saturday'];y,m,d=map(int,s.split('-'));base=datetime.datetime(y,m,d,12,tzinfo=tz);nxt=base+datetime.timedelta(days=7);C0=int(base.timestamp())*1000;N=int(nxt.timestamp())*1000
  assert C0==row['cutoff_ms'] and N==row['incident_deadline_ms'];nextdate=nxt.strftime('%Y-%m-%d')
  old.append(dict(saturday=s,cutoff_ms=C0,incident_deadline_ms=N,deadline_saturday=nextdate,hours_to_deadline=(N-C0)//3600000,upcoming_before=s,upcoming_at=nextdate if nextdate[:4]!='2100' else 'event_calendar',event=row['event']))
 # Full retained44 calendar checksum verifies our independently reconstructed old shape.
 # Its last upcoming-at field is explicitly inspected/pinned in the minimal sample.
 old[-1]['upcoming_at']=SOURCE_INDEX['calendar_samples'][-1]['upcoming_at']
 assert len(old)==SOURCE_INDEX['calendar_count']and sha(J(old))==SOURCE_INDEX['calendar_canonical_sha256']
 for sample in SOURCE_INDEX['calendar_samples']:
  i=sample['original_index'];assert old[i]=={k:v for k,v in sample.items()if k!='original_index'}
 return dict(count=len(rows),tzif_sha256=sha(tzbytes),posix_tail='PST8PDT,M3.2.0,M11.1.0',original44_calendar_sha256=sha(J(old)),method='integer Gregorian days + frozen future rules; cross-checked all rows using ZoneInfo.from_file of the exact same TZif bytes, no host timezone database')
def rpc_amount(s):
 if type(s)is not str or len(s)>128 or not re.fullmatch(r'-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?',s):raise ValueError('amount')
 exp=re.search('[eE]([+-]?[0-9]+)$',s)
 if exp and abs(int(exp[1]))>32:raise ValueError('amount')
 # Exact integer arithmetic: Decimal is used only to parse the finite decimal token.
 sign=-1 if s.startswith('-')else 1
 if sign<0:raise ValueError('amount')
 mantissa=re.split('[eE]',s)[0];pieces=mantissa.split('.');digits=pieces[0]+(pieces[1]if len(pieces)==2 else '');power=(int(exp[1])if exp else 0)+8-(len(pieces[1])if len(pieces)==2 else 0);n=int(digits)
 if power<0:
  div=10**(-power)
  if n%div:raise ValueError('fractional satoshi')
  n//=div
 else:n*=10**power
 if n>MAXSAT:raise ValueError('amount')
 return n

def corpus():
 selftest();rows=calendar();calcheck=calendar_checks(rows);scens,states=scenarios();dtos=[]
 def dto(kind,name,value):
  value=validate(kind,value);row=dict(kind=kind,name=name,value=value,canonical=J(value).decode(),sha256=sha(J(value)))
  if kind=='Record':row['record_digest']=record_digest(value)
  dtos.append(row);return len(dtos)-1
 ix={}
 for k,v in [('Rules',RULES),('Pin',PIN),('Declaration',declaration(RECEIPTS[0])),('SignedDeclaration',signed(declaration(RECEIPTS[0]))),('Outpoint',BLOCKS['coin']['outpoint']),('FundingCoin',BLOCKS['coin']),('Cursor',cursor(event_key(event('2026-11-21')))),('Tip',tip(65)),('Anchor',BLOCKS['anchor']),('Anchor',BLOCKS['maximum_anchor'])]:ix[k]=dto(k,'candidate'if k!='Anchor'else ('maximum_foreign'if v==BLOCKS['maximum_anchor']else 'foreign_first'),v)
 info=dict(version=1,store_id=STORE_ID,pin=PIN,created_at_ms=CREATED,event_count=0,funding_height=-1,funding_hash='',funding_complete=False,funding_generation=0,funding_restart_count=0);ix['Info']=dto('Info','created',info)
 for kind,v in [('Marker',dict(application='slug-candidate-seeds',version=1,store_id=STORE_ID,pin=PIN)),('Init',dict(version=1,store_id=STORE_ID,pin=PIN)),('Metadata',dict(version=1,store_id=STORE_ID,pin=PIN,created_at_ms=CREATED,clock_ms=CREATED,event_count=0,funding_cursor={**cursor('',0),'scan_digest':scan_initial('',0,True)},funding_restarts=[],funding_complete=False,last_funding_tip=None))]:ix[kind]=dto(kind,'created',v)
 # Retain every atomic record once; trace steps refer to its domain digest.
 record_rows=[]
 for digest,r in states.items():record_rows.append(dict(digest=digest,canonical=J(r).decode()))
 byphase={}
 for r in states.values():byphase.setdefault(phase(r),r)
 for p,r in byphase.items():ix['Record/'+p]=dto('Record',p,r);ix['Status/'+p]=dto('Status',p,status(r,max(C+1000,r['observation']['observed_at_ms'],r['incident_at_ms']),'ready'))
 for n,r in [('empty',record(RECEIPTS[1])),('five_class_synthetic',record(RECEIPTS[2]))]:dto('Record',n,r)
 finals=next(r for r in states.values()if r['finality']is not None and not r['conflicts'])
 for k in ['publication','target','checkpoint','finality','observation']:
  typ={'publication':'Publication','target':'Target','checkpoint':'SignedCheckpoint','finality':'Finality','observation':'Observation'}[k];ix[typ]=dto(typ,'candidate',finals[k])
 restart=next(r['scan_restarts'][-1]for r in states.values()if len(r['scan_restarts'])==16);ix['ScanRestart']=dto('ScanRestart','generation16',restart)
 conflict=next(r['conflicts'][0]for r in states.values()if r['conflicts']);ix['Conflict']=dto('Conflict','candidate',conflict)
 alternates=[]
 for field,value in [('rules_id',label('unsupported alternate rules')),('checkpoint_public_key',ed_public(H(domain('different candidate checkpoint policy fixture'))).hex()),('publisher_chain_id',label('different publication chain policy'))]:
  alt=clone(declaration(RECEIPTS[0]));alt[field]=value;statement=signed(alt);alternates.append((field,statement));ix['SignedDeclaration/alternate/'+field]=dto('SignedDeclaration','authenticated_alternate_'+field+'_is_conflict_evidence',statement)
  conflict_record=clone(byphase['prepared']);conflict_record['conflicts']=[dict(kind='declaration',observed_at_ms=C+1000,declaration=statement,checkpoint=None,expected_height=0,expected_hash='',observed_hash='',observed_tip=None)];dto('Record','retained_authenticated_alternate_'+field,conflict_record)
 absent=status(None,DEADLINE);ix['Status/absent']=dto('Status','absent_due_no_allocation',absent)
 history=dict(version=1,store_id=STORE_ID,after_event_key='',next_event_key='',has_more=False,observed_at_ms=C+1000,events=[status(byphase['prepared'],C+1000)]);ix['HistoryPage']=dto('HistoryPage','one_terminal',history)
 dto('HistoryPage','empty_terminal',{**history,'events':[]})
 # Full eight-status page is constructed from deterministic dates, not eight imported receipts.
 eight=[]
 for row in rows[40:48]:
  st=status(None,row['cutoff_ms']+1);st.update(event=row['event'],event_key=row['event_key'],exists=True,phase='prepared',prepared_at_ms=row['cutoff_ms']+1,observed_at_ms=rows[48]['cutoff_ms'],escalation_due=rows[48]['cutoff_ms']>=row['incident_deadline_ms']);eight.append(st)
 dto('HistoryPage','maximum_eight_more',dict(version=1,store_id=STORE_ID,after_event_key='',next_event_key=eight[-1]['event_key'],has_more=True,observed_at_ms=rows[48]['cutoff_ms'],events=eight))
 def request(cmd,view_,**kw):return dict(id=1,command='seed_'+cmd,view_id=''if cmd=='snapshot'else view_['view_id'],saturday='2026-11-21'if cmd in ['status','record','prepare','publish','reconcile','attest']else'',after_event_key='',limit=8 if cmd=='history'else 0,job_id=kw.get('job_id',''))
 def response(view_,slot,value):
  r=dict(id=1,ok=True,view=clone(view_),store_id=STORE_ID,snapshot=None,status=None,record=None,history=None,job=None,error='');r[slot]=value
  if slot=='snapshot'and value['state']in['setup_incomplete','unavailable']:r['store_id']=''
  if slot=='snapshot'and value['state']=='unavailable':r['view']=None
  return r
 protocol=[]
 def pc(name,q,r,v,accepted=True,expected=None,terminal=None,created=None):
  try:canonical=J(correlate(q,r,v,expected,terminal,created)).decode();ok=True
  except(Refusal,ValueError,TypeError,KeyError):ok=False;canonical=None
  assert ok==accepted,(name,ok,accepted)
  protocol.append(dict(name=name,request=q,response=r,current_view=v,expected_job=expected,terminal_digest=terminal,expected_created_at_ms=created,accepted=accepted,canonical_sha256=sha(canonical.encode()) if canonical is not None else None))
 for f in VIEWS:
  view_=f['current_view'];name=f['fixture'];jid=label('job '+name);sn=dict(version=1,state='ready',error='',view=view_,store_id=STORE_ID,pin=PIN,created_at_ms=CREATED,publisher_account_id=PIN['publisher_account_id'],node_state='offline',active_job_id='',queued_job_id='',last_job_id='')
  for state in ['ready','busy','setup_incomplete','unavailable','failed']:
   s=clone(sn);s['state']=state
   if state=='busy':s['active_job_id']=jid
   elif state=='failed':s['error']='seed_clock'
   elif state in ['setup_incomplete','unavailable']:
    s.update(error='seed_setup_incomplete'if state=='setup_incomplete'else'seed_unavailable',pin=None,created_at_ms=0,store_id='',publisher_account_id='');s['view']=None if state=='unavailable'else view_
   dto('SeedSnapshot',name+'/'+state,s);q=request('snapshot',view_);r=response(view_,'snapshot',s);dto('SeedResponse',name+'/snapshot/'+state,r);pc(name+'/snapshot/'+state,q,r,view_,created=CREATED)
   if state in ['ready','busy','failed']:
    changed=clone(r);changed['snapshot']['created_at_ms']+=1;pc(name+'/snapshot/'+state+'/changed_creation',q,changed,view_,False,created=CREATED)
  for cmd in ['status','record','history']:
   q=request(cmd,view_);value=status(finals,C+1000)if cmd=='status'else finals if cmd=='record'else history;r=response(view_,cmd,clone(value));dto('SeedRequest',name+'/'+cmd,q);dto('SeedResponse',name+'/'+cmd,r);pc(name+'/'+cmd,q,r,view_)
   bad=clone(r);bad['id']=2;pc(name+'/'+cmd+'/wrong_id',q,bad,view_,False)
   bad=clone(r);bad['view']=next(x['current_view']for x in VIEWS if x['fixture']!=name);pc(name+'/'+cmd+'/other_view',q,bad,view_,False)
  for kind in ['prepare','publish','reconcile','attest']:
   q=request(kind,view_);j=dict(version=1,job_id=label(name+'/'+kind),kind=kind,state='queued',view=view_,store_id=STORE_ID,event_key=event_key(event('2026-11-21')),saturday='2026-11-21',error='',result=None);dto('SeedRequest',name+'/'+kind,q);dto('SeedJob',name+'/'+kind+'/queued',j);r=response(view_,'job',j);pc(name+'/'+kind+'/ack',q,r,view_)
   for badstate in ['running','failed','cancelled']:
    bad=clone(j);bad['state']=badstate;bad['error']='seed_cancelled'if badstate=='cancelled'else'seed_unavailable'if badstate=='failed'else'';pc(name+'/'+kind+'/nonqueued_ack/'+badstate,q,response(view_,'job',bad),view_,False)
   running=clone(j);running['state']='running';pc(name+'/'+kind+'/running_to_queued',request('job',view_,job_id=j['job_id']),response(view_,'job',j),view_,False,running)
   for terminal,error in [('completed',''),('failed','outcome_unknown'),('failed','seed_waiting_for_funds'),('cancelled','seed_cancelled')]:
    end=clone(j);end.update(state=terminal,error=error,result=clone(finals)if terminal=='completed'else None);query=request('job',view_,job_id=j['job_id']);reply=response(view_,'job',end);dto('SeedJob',name+'/'+kind+'/'+terminal+'/'+error,end);pc(name+'/'+kind+'/'+terminal+'/'+error,query,reply,view_,expected=j)
    if terminal=='completed':
     pc(name+'/'+kind+'/completed_begin_ack',q,reply,view_,False)
     digest=sha(J(end));pc(name+'/'+kind+'/terminal_repeat',query,reply,view_,expected=end,terminal=digest)
     changed=clone(reply);changed['job']['result']['observation']['observed_at_ms']+=1;changed['job']['result']['observation']['node_state']='offline';pc(name+'/'+kind+'/terminal_changed_observation',query,changed,view_,False,end,digest)
     wrong=clone(reply);wrong['job']['kind']='publish'if kind!='publish'else'prepare';pc(name+'/'+kind+'/wrong_kind',query,wrong,view_,False,j)
     wrong=clone(reply);wrong['job']['state']='running';wrong['job']['result']=None;pc(name+'/'+kind+'/terminal_regression',query,wrong,view_,False,end,digest)
  # Unknown-kind discovery is bounded to an exact same-view snapshot tuple.
  expected=dict(job_id=j['job_id'],kind=None,view=view_,store_id=STORE_ID,event_key=j['event_key'],saturday=j['saturday']);pc(name+'/snapshot_discovered_kind',request('job',view_,job_id=j['job_id']),response(view_,'job',j),view_,expected=expected)
  for error in ERRORS:
   r=dict(id=1,ok=False,view=None,store_id='',snapshot=None,status=None,record=None,history=None,job=None,error=error);pc(name+'/error/'+error,request('status',view_),r,view_)
 # Recipes avoid duplicating full canonical records per mutation. Each is executed
 # here and must meet its independently contract-selected accepted bit.
 cases=[]
 def cc(i,name,accepted,**kw):
  case=dict(kind=dtos[i]['kind'],name=dtos[i]['name']+'/'+name,dto=i,accepted=accepted,**kw);b=materialize(case,dtos)
  try:norm=parse(case['kind'],b);ok=True;canonical=J(norm).decode()
  except Refusal:ok=False;canonical=None
  assert ok==accepted,(case['kind'],case['name'],ok,accepted)
  case['input_bytes']=len(b);case['input_sha256']=sha(b);case['expected_dto']=i if accepted else None;case['canonical_sha256']=sha(canonical.encode()) if accepted else None;cases.append(case)
 for i,row in enumerate(dtos):
  if row['kind']in ['Marker','Init','Metadata','Rules']:continue
  cc(i,'canonical',True);cc(i,'reversed_whitespace',True,format='reversed_whitespace');cc(i,'escaped_first_key',True,format='escaped_first_key');cc(i,'duplicate',False,format='duplicate_first_key');cc(i,'duplicate_decoded',False,format='duplicate_escaped_key');first=next(iter(row['value']));cc(i,'missing',False,mutations=[dict(op='delete',path='/'+first)]);cc(i,'unknown',False,mutations=[dict(op='set',path='/not_a_field',value=0)])
  if 'version'in row['value']:
   cc(i,'version_boolean',False,mutations=[dict(op='set',path='/version',value=True)]);cc(i,'version2',False,mutations=[dict(op='set',path='/version',value=2)]);cc(i,'negative_zero',False,replace_first=['"version":1','"version":-0']);cc(i,'exponent',False,replace_first=['"version":1','"version":1e0'])
  if row['kind']in CAPS:
   cap=CAPS[row['kind']]-(1 if row['kind']in ['SeedRequest','SeedResponse'] else 0);cc(i,'exact_transport_cap',True,format='pad_to_cap',bytes=cap);cc(i,'over_transport_cap',False,format='pad_to_cap',bytes=cap+1)
 for key,field,value in [('Info','funding_height',-2),('Info','funding_height',False),('Info','funding_generation',17),('Cursor','height',-2),('Cursor','generation',17),('FundingCoin','value_satoshis',MAXSAT+1),('FundingCoin','value_satoshis',True),('FundingCoin','coinbase',0),('Publication','dispatch_count',U32+1),('Publication','raw_tx','00'),('Target','height',0),('Finality','target_descendants',59),('SignedCheckpoint','signature','00'*64),('SignedDeclaration','signature','00'*64),('Observation','chain_observed_at_ms',DATE_MAX),('Status/absent','exists',True),('Pin','publisher_gym_id','00'*32),('Pin','floor_height',1),('Declaration','event_key','00'*32),('Declaration','publisher_public_key','00'*32),('SeedJob','result',None)]:
  if key=='SeedJob':continue
  cc(ix[key],'bad_'+field,False,mutations=[dict(op='set',path='/'+field,value=value)])
 for field,statement in alternates:
  cc(ix['Record/prepared'],'alternate_'+field+'_not_supported_record',False,mutations=[dict(op='set',path='/declaration',value=statement)])
 for field in ['source_store_id','publisher_public_key']:
  invalid=clone(declaration(RECEIPTS[0]));invalid[field]=label('different signed '+field);cc(ix['SignedDeclaration'],'signed_changed_'+field,False,mutations=[dict(op='set',path='/declaration',value=invalid),dict(op='set',path='/signature',value=signed(invalid)['signature'])])
 cc(ix['SignedCheckpoint'],'wrong_wire_order_signature',False,mutations=[dict(op='set',path='/signature',value=ed_sign(CHECK_SEED,b'SLUG signed checkpoint\0'+u32(1)+u32(NET)+raw(GENESIS)+u32(finals['target']['height'])+raw(finals['target']['hash'])).hex())])
 # Canonical scalar refusal in deep old-source fields, where Python bool==0 must never pass.
 cc(ix['Anchor'],'merkle_branch_21',False,mutations=[dict(op='set',path='/merkle_branch',value=['00'*32]*21)])
 cc(ix['Record/prepared'],'scan_restarts_17',False,mutations=[dict(op='set',path='/scan_restarts',value=[dtos[ix['ScanRestart']]['value']]*17)])
 cc(ix['Record/prepared'],'conflicts_9',False,mutations=[dict(op='set',path='/conflicts',value=[dtos[ix['Conflict']]['value']]*9)])
 cc(ix['HistoryPage'],'history_9',False,mutations=[dict(op='set',path='/events',value=history['events']*9)])
 cc(ix['Record/prepared'],'nested_boolean_timestamp',False,mutations=[dict(op='set',path='/source_receipt/input/participants/0/roster/assigned_at_ms/9',value=False)])
 amounts=[]
 for s in ['0','1e-8','0.00000001','1.23456789','21000000','2.1e7','21000000.00000000','0e32','1e-9','-0','-1','21000000.00000001','1e33','1e-33','01','NaN','1.' ,'1'+'0'*128]:
  try:n=rpc_amount(s);ok=True
  except ValueError:n=None;ok=False
  amounts.append(dict(token=s,accepted=ok,satoshis=n))
 # Coinbase off-by-one and deterministic display-order selection are formula facts.
 funding=[]
 for T in [98,99,100,101]:
  for height in [0,1,2]:funding.append(dict(tip_height=T,coin_height=height,coinbase=True,mature=T+1-height>=100))
 merkle_cases=[]
 for n in [1,2,3,5,8,17]:
  txids=[label('merkle %d'%i)for i in range(n)]
  for i in range(n):root,branch=merkle(txids,i);assert verify_merkle(txids[i],i,n,branch,root);merkle_cases.append(dict(txids=txids,index=i,root=root,branch=branch))
 return dict(version=1,candidate_fixture_only=True,core_contract_sha256=CORE,helper_contract_sha256=HELPER,input_index_sha256=sha((HERE/'input_hashes.json').read_bytes()),rules=dict(value=RULES,canonical=J(RULES).decode(),preimage=RULE_PRE.hex(),id=RULE_ID),pin=dict(value=PIN,canonical=J(PIN).decode(),store_preimage=STORE_PRE.hex(),store_id=STORE_ID,publication_chain_preimage=CHAIN_PRE.hex()),fixture_keys=dict(publisher_seed=DECL_SEED.hex(),publisher_public=DECL_PUB.hex(),checkpoint_seed=CHECK_SEED.hex(),checkpoint_public=CHECK_PUB.hex(),funding_scalar='01'.zfill(64),funding_public=FUND_PUB.hex(),funding_script=FUND_SCRIPT.hex(),scope='Published fixture material only, never production/operator authority'),calendar=dict(verification=calcheck,rows=rows,invalid_dates=['2025-12-27','2100-01-02','2026-11-20','2026-02-30','2026-1-03','0000-00-00'],signed_height_sentinels=['Cursor.height','Cursor.complete_to_height','Info.funding_height','Status.scan_height','Status.scan_tip_height']),crypto=dict(declaration=declaration(RECEIPTS[0]),declaration_id_preimage=(domain('candidate seed declaration id')+lp(J(declaration(RECEIPTS[0])))).hex(),declaration_id=decl_id(declaration(RECEIPTS[0])),declaration_signature_preimage=(domain('candidate seed declaration')+lp(J(declaration(RECEIPTS[0])))).hex(),signed_declaration=signed(declaration(RECEIPTS[0])),event_key_preimage=event_pre(event('2026-11-21')).hex(),checkpoint=finals['checkpoint'],checkpoint_preimage=checkpoint_pre(finals['checkpoint']).hex(),publication=BLOCKS['publication'],merkle_cases=merkle_cases),block_fixture_index='fixtures/block_index.json',dtos=dtos,codec_cases=cases,protocol_cases=protocol,record_states=record_rows,scenarios=scens,rpc_amount_cases=amounts,funding_maturity_cases=funding,funding_selection_cases=selection(),initialization_cases=initialization(),carrier_cases=carrier_cases(),scan_fold_cases=fold_cases(),bounds=dict(formula_report='inputs/contracts/shape_bounds.json',formula_report_sha256=INPUTS['inputs/contracts/shape_bounds.json']['sha256'],largest_canonical_dto_by_kind={k:max(len(d['canonical']) for d in dtos if d['kind']==k)for k in sorted(set(d['kind']for d in dtos))},helper_frame_lf_bytes=1,limits=CAPS,claim='Formula-only cap report and finite actual samples; not a claim every independent cap maximum is simultaneously a valid admitted record.'),coverage_limits=['No new45 Go source/output read. Expected values derive from frozen contracts, minimal immutable44 extracts and independent arithmetic/crypto only.','Old complete receipts carry modeled cutoff histories atop six previously proof-verified admissions; they are not actual authority closed-event execution. Synthetic five-class receipt is explicitly structural.','Normal-work block bodies have synthetic input references and are not a validating-node admitted chain; full consensus, maturity, funding/dispatch and real reorg tests remain runtime obligations.','Atomic state and initialization cases model durable branches; no process SIGKILL, LevelDB, account lease, network dispatch, GUI or public mining is performed.','Compact Anchor alone does not prove carrier payload: independent full-body inspection is separately required.','Codec validator recognizes the retained three44 receipt shapes/values; it is a finite reference, not a replacement for general production validation.'])

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--check',action='store_true');args=ap.parse_args();c=corpus();b=pretty(c);path=HERE/'seed_v1.json'
 if args.check:assert path.read_bytes()==b,'seed_v1.json differs'
 else:path.write_bytes(b)
 print('seed corpus:',len(c['dtos']),'DTOs,',len(c['codec_cases']),'codec cases,',len(c['protocol_cases']),'protocol cases,',len(c['scenarios']),'scenarios;',len(b),'bytes;',sha(b))
if __name__=='__main__':main()
