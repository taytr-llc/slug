"""Finite atomic-boundary reference model, not a filesystem/account/node simulator.
Network facts supplied below refer only to independently built fixture headers.
No model branch is a public authority-import API. Fault steps model either durable
outcome explicitly; they do not claim actual crash, lease or network execution.
"""
from common import *
from schema import Refusal,req,validate
from crypto import publication
BLOCKS=json.loads((HERE/'fixtures/block_index.json').read_bytes())
CHAIN={b['height']:b for b in BLOCKS['blocks'] if b['name'].startswith('chain-')}
BYNAME={b['name']:b for b in BLOCKS['blocks']}
C=RECEIPTS[0]['input']['event']['cutoff_ms'];DEADLINE=RECEIPTS[0]['input']['event']['incident_deadline_ms'];CREATED=C-10000

def tip(height):return dict(height=height,hash=GENESIS if height==0 else CHAIN[height]['hash'])
def phase(r):
 if r is None:return 'absent'
 if r['conflicts']or r['conflict_overflow']:return 'conflict'
 for key,name in [('finality','finalized'),('target','provisional'),('anchor','waiting_target'),('publication','waiting_anchor')]:
  if r[key]is not None:return name
 return 'prepared'
def status(r,now,funding='unobserved',fundgen=0):
 e=event('2026-11-21') if r is None else r['source_receipt']['input']['event'];p=None if r is None else r['publication'];a=None if r is None else r['anchor'];t=None if r is None else r['target'];f=None if r is None else r['finality'];o=None if r is None else r['observation'];conf=False if r is None else bool(r['conflicts']or r['conflict_overflow'])
 return dict(version=1,store_id=STORE_ID,event_key=event_key(e),event=e,exists=r is not None,phase=phase(r),publication_state='none' if p is None else p['state'],scan_height=-1 if r is None else r['cursor']['height'],scan_tip_height=-1 if o is None or o['tip']is None else o['tip']['height'],anchor_height=0 if a is None else a['height'],anchor_hash='' if a is None else a['hash'],target_height=0 if t is None else t['height'],target_hash='' if t is None else t['hash'],target_descendants=0 if o is None else o['target_descendants'],checkpoint_present=r is not None and r['checkpoint']is not None,finalized_at_ms=0 if f is None else f['observed_at_ms'],conflict_count=0 if r is None else len(r['conflicts']),conflict_overflow=False if r is None else r['conflict_overflow'],incident_at_ms=0 if r is None else r['incident_at_ms'],prepared_at_ms=0 if r is None else r['prepared_at_ms'],observed_at_ms=now,chain_observed_at_ms=0 if o is None else o['chain_observed_at_ms'],node_state='unobserved' if o is None else o['node_state'],funding_state='unobserved' if r is None else funding,current_finality=False if o is None else o['current_finality'],escalation_due=now>=e['incident_deadline_ms'] and(f is None or conf),scan_generation=0 if r is None else r['cursor']['generation'],funding_generation=0 if r is None else fundgen)

def conflict_key(c):
 q={k:v for k,v in c.items() if k!='observed_at_ms'};return sha(J(q))
class State:
 def __init__(self,created=CREATED):
  self.created=created;self.clock=created;self.r=None;self.reservations=[];self.funding_generation=0;self.funding_state='unobserved';self.poison=False;self.sent=0;self.last_work=[]
 def snapshot(self):return dict(created_at_ms=self.created,clock_ms=self.clock,record_digest='' if self.r is None else record_digest(self.r),record=clone(self.r),reservations=clone(self.reservations),funding_generation=self.funding_generation,funding_state=self.funding_state,poisoned=self.poison,network_send_count=self.sent)
 def before(self,now,authorized=True):
  self.last_work=[];req(authorized,'unauthorized');req(not self.poison,'seed_unavailable');req(now>=self.clock,'seed_clock');self.clock=now;self.incident()
 def incident(self):
  if self.r and self.clock>=self.r['source_receipt']['input']['event']['incident_deadline_ms'] and (self.r['finality']is None or self.r['conflicts']or self.r['conflict_overflow']) and not self.r['incident_at_ms']:self.r['incident_at_ms']=self.clock
 def add_conflict(self,c):
  req(self.r is not None,'seed_not_prepared');key=conflict_key(c)
  if all(conflict_key(x)!=key for x in self.r['conflicts']):
   if len(self.r['conflicts'])==8:self.r['conflict_overflow']=True
   else:self.r['conflicts'].append(c)
  self.r['observation']['current_finality']=False;self.incident()
 def call(self,op,now,**kw):
  if op=='reopen':self.poison=False;self.last_work=[];return
  self.last_work=[];req(kw.get('authorized',True),'unauthorized')
  if op in ['build','arm','ack','attest']:req(kw.get('fixture','zero256')=='zero256' and not kw.get('held',False) and kw.get('fresh',True),'forbidden')
  self.before(now,kw.get('authorized',True))
  if op=='prepare':
   if self.r is not None:return
   req(self.created<C,'seed_created_too_late');req(kw.get('complete',True),'seed_source_incomplete');req(now>=RECEIPTS[0]['completed_at_ms'],'seed_clock');self.r=record();self.r['prepared_at_ms']=now
  elif op=='status':pass
  else:
   req(self.r is not None,'seed_not_prepared')
   if op in ['build','arm','ack','attest']:
    req(kw.get('fixture','zero256')=='zero256' and not kw.get('held',False) and kw.get('fresh',True),'forbidden')
    req(not self.r['conflicts']and not self.r['conflict_overflow'],'seed_conflict')
   if op=='build':
    if self.r['anchor']is not None:return
    if self.r['publication']is not None:return
    req(kw.get('online',True),'seed_node_offline');req(kw.get('inventory_complete',True),'seed_scan_incomplete');req(kw.get('segwit',True),'seed_not_mature');req(kw.get('has_funds',True),'seed_waiting_for_funds')
    coin=clone(BLOCKS['coin']);req(coin['outpoint']not in self.reservations,'seed_waiting_for_funds');p=publication(coin,self.r['event_key'],decl_id(self.r['declaration']['declaration']))
    self.r['publication']=dict(version=1,raw_tx=p['raw_tx'],txid=p['txid'],coin=coin,built_at_ms=now,dispatch_count=0,first_dispatch_at_ms=0,last_dispatch_at_ms=0,state='retained');self.reservations.append(coin['outpoint']);self.funding_state='ready'
   elif op=='arm':
    if self.r['anchor']is not None:return
    req(self.r['publication']is not None,'seed_not_prepared');req(kw.get('unspent',True),'seed_waiting_for_funds');p=self.r['publication'];req(p['dispatch_count']<U32,'seed_capacity');p['dispatch_count']+=1;p['first_dispatch_at_ms']=p['first_dispatch_at_ms']or now;p['last_dispatch_at_ms']=now;p['state']='unknown'
    if kw.get('cancel_after_intent'):raise Refusal('outcome_unknown')
    self.sent+=1
    if kw.get('lost_reply'):raise Refusal('outcome_unknown')
   elif op=='ack':
    p=self.r['publication'];req(p is not None and p['dispatch_count']>0,'seed_not_prepared');req(kw.get('txid',p['txid'])==p['txid'],'outcome_unknown');p['state']='submitted'
   elif op=='scan':
    req(kw.get('online',True),'seed_node_offline');heights=kw['heights'];req(len(set(heights))<=8,'seed_capacity');req(len(heights)==len(set(heights)),'invalid_request');cur=self.r['cursor'];
    if self.r['anchor']is None:
     for h in heights:
      req(h==cur['height']+1,'seed_scan_incomplete');hash_=GENESIS if h==0 else CHAIN[h]['hash'];cur['height']=h;cur['hash']=hash_;cur['scan_digest']=scan_step(cur['scan_digest'],h,hash_);self.last_work.append(h)
      if h==2 and kw.get('find_anchor',True):
       self.r['anchor']=clone(BLOCKS['anchor']);cur['complete_to_height']=h;cur['complete_to_hash']=hash_;break
    self.online(now,kw.get('tip_height',65))
   elif op=='target':
    req(self.r['anchor']is not None,'seed_target_unavailable');req(kw.get('online',True),'seed_node_offline');t=CHAIN[3];self.last_work=[2,3]
    if self.r['target']is None:self.r['target']=dict(height=3,hash=t['hash'],header=t['header'],observed_at_ms=now,observed_tip=tip(kw.get('tip_height',65)))
    self.online(now,kw.get('tip_height',65))
   elif op in ['checkpoint','attest']:
    req(self.r['target']is not None,'seed_target_unavailable');t=self.r['target'];cp=checkpoint(t['height'],t['hash'])
    if kw.get('wrong_height'):cp=checkpoint(t['height']+1,CHAIN[t['height']+1]['hash']);raise Refusal('invalid_request')
    if op=='attest':req(kw.get('online',True),'seed_node_offline');req(kw.get('tip_height',65)-t['height']>=60,'seed_not_mature')
    if self.r['checkpoint']is None:self.r['checkpoint']=cp
    if op=='attest':self.last_work=[2,3];self.online(now,kw.get('tip_height',65))
   elif op=='online':
    self.last_work=[2,3] if self.r['target']is not None else [];self.online(now,kw.get('tip_height',65))
   elif op=='offline':
    o=self.r['observation'];o['observed_at_ms']=now;o['node_state']='offline'
   elif op=='event_restart':
    req(self.r['anchor']is None,'seed_conflict');cur=self.r['cursor'];req(cur['height']>=0,'seed_scan_incomplete');req(cur['generation']<16,'seed_capacity');g=cur['generation']+1
    x=dict(generation=g,previous=clone(cur),observed_at_ms=now,observed_tip=dict(height=1,hash=BYNAME['fork-pre-anchor']['hash']),replacement_hash=BYNAME['fork-pre-anchor']['hash']);self.r['scan_restarts'].append(x);self.r['cursor']=cursor(self.r['event_key'],g)
   elif op=='funding_restart':
    req(self.funding_generation<16,'seed_capacity');self.funding_generation+=1;self.funding_state='scanning'
   elif op=='mixed_uncommitted_batch':pass
   elif op=='declaration_conflict':
    d=clone(self.r['declaration']['declaration']);d['snapshot_digest']=label('conflicting snapshot '+str(kw.get('number',0)));c=dict(kind='declaration',observed_at_ms=now,declaration=signed(d),checkpoint=None,expected_height=0,expected_hash='',observed_hash='',observed_tip=None);self.add_conflict(c)
   elif op=='checkpoint_conflict':
    req(self.r['target']is not None,'seed_target_unavailable');c=dict(kind='checkpoint',observed_at_ms=now,declaration=None,checkpoint=checkpoint(3,BYNAME['fork-target']['hash']),expected_height=0,expected_hash='',observed_hash='',observed_tip=None);self.add_conflict(c)
   elif op=='chain_conflict':
    req(self.r['target']is not None,'seed_target_unavailable');short=kw.get('short',False);c=dict(kind='chain',observed_at_ms=now,declaration=None,checkpoint=None,expected_height=3,expected_hash=self.r['target']['hash'],observed_hash='' if short else BYNAME['fork-target']['hash'],observed_tip=tip(2) if short else dict(height=3,hash=BYNAME['fork-target']['hash']));self.add_conflict(c);self.r['observation']=dict(observed_at_ms=now,chain_observed_at_ms=now,tip=clone(c['observed_tip']),node_state='online',target_descendants=max(0,c['observed_tip']['height']-3),current_finality=False)
   elif op=='unrelated_spent_input':pass
   elif op=='cancel_before_dispatch':raise Refusal('seed_cancelled')
   else:raise AssertionError(op)
  self.incident()
  if kw.get('unknown_commit'):self.poison=True;raise Refusal('outcome_unknown')
 def online(self,now,height):
  t=self.r['target'];desc=0 if t is None else max(0,height-t['height']);eligible=t is not None and self.r['checkpoint']is not None and desc>=60 and not(self.r['conflicts']or self.r['conflict_overflow']);o=dict(observed_at_ms=now,chain_observed_at_ms=now,tip=tip(height),node_state='online',target_descendants=desc,current_finality=eligible);self.r['observation']=o
  if eligible and self.r['finality']is None:self.r['finality']=dict(checkpoint=clone(self.r['checkpoint']),observed_at_ms=now,observed_tip=tip(height),target_descendants=desc)

# Trace expectations are atomic reference states. Runtime may combine boundaries;
# consumers must not treat internal names as additional public API requirements.
def scenarios():
 pre=[('prepare',{}),('build',{}),('arm',{}),('ack',{}),('scan',{'heights':[0,1,2]}),('target',{})]
 runs=[('ordinary_publication_finality',pre+[('checkpoint',{}),('online',{'tip_height':62}),('online',{'tip_height':63}),('offline',{}),('online',{'tip_height':62}),('online',{'tip_height':65}),('status',{'at':DEADLINE})]),
 ('lost_reply_reopen_exact_retransmit',[('prepare',{}),('build',{}),('arm',{'lost_reply':True}),('reopen',{}),('arm',{}),('ack',{})]),
 ('cancel_after_armed_intent',[('prepare',{}),('build',{}),('arm',{'cancel_after_intent':True}),('reopen',{}),('arm',{'held':True}),('arm',{}),('ack',{})]),
 ('durable_write_unknown',[('prepare',{'unknown_commit':True}),('status',{}),('reopen',{}),('prepare',{}),('build',{'unknown_commit':True}),('reopen',{}),('build',{}),('status',{})]),
 ('waiting_and_authority',[('prepare',{}),('build',{'fixture':'incrementing256'}),('build',{'held':True}),('build',{'fresh':False}),('build',{'online':False}),('build',{'inventory_complete':False}),('build',{'segwit':False}),('build',{'has_funds':False}),('build',{}),('arm',{'unspent':False})]),
 ('unanchored_restart_preserves_publication',[('prepare',{}),('build',{}),('scan',{'heights':[0,1],'find_anchor':False}),('mixed_uncommitted_batch',{}),('event_restart',{}),('scan',{'heights':[0,1,2]}),('funding_restart',{}),('target',{}),('unrelated_spent_input',{})]),
 ('checkpoint_before_mature',pre+[('checkpoint',{}),('attest',{'tip_height':62}),('attest',{'tip_height':63}),('checkpoint',{}),('attest',{'tip_height':65})]),
 ('checkpoint_wrong_height',pre+[('checkpoint',{'wrong_height':True}),('checkpoint',{})]),
 ('target_reorg_conflict',pre+[('attest',{}),('chain_conflict',{}),('online',{}),('status',{'at':DEADLINE})]),
 ('shorter_coherent_tip_conflict',pre+[('chain_conflict',{'short':True})]),
 ('authenticated_competing_checkpoint',pre+[('checkpoint_conflict',{}),('checkpoint_conflict',{}),('arm',{})]),
 ('equivocation_overflow',[('prepare',{})]+[('declaration_conflict',{'number':i}) for i in range(9)]+[('declaration_conflict',{'number':0}),('status',{'at':DEADLINE})]),
 ('late_first_finality_observes_incident',pre+[('checkpoint',{}),('online',{'at':DEADLINE,'tip_height':63}),('status',{})]),
 ('late_first_attestation_observes_incident',pre+[('attest',{'at':DEADLINE,'tip_height':63}),('status',{})]),
 ('deadline_prepare_replay_observes_incident',[('prepare',{}),('prepare',{'at':DEADLINE}),('prepare',{})]),
 ('deadline_anchored_publish_noop_observes_incident',pre+[('build',{'at':DEADLINE}),('arm',{}),('status',{})]),
 ('forbidden_deadline_observation_does_not_write',pre+[('build',{'at':DEADLINE,'held':True}),('status',{'at':DEADLINE+1})]),
 ('absent_deadline_no_allocation',[('status',{'at':DEADLINE-1}),('status',{'at':DEADLINE}),('status',{'at':DEADLINE+1})]),
 ('deadline_latches_actual_observation',[('prepare',{}),('status',{'at':DEADLINE-1}),('status',{'at':DEADLINE+7}),('offline',{})]),
 ('initialization_late_refusal',[('prepare',{'created':C})]),
 ('authorization_precedes_clock',[('status',{'at':C+1000,'authorized':False}),('prepare',{}),('status',{'at':C-1})]),
 ('restart_capacity',[('prepare',{}),('build',{})]+sum(([('scan',{'heights':[0,1],'find_anchor':False}),('event_restart',{})]for _ in range(17)),[])),
 ('funding_restart_capacity',pre+[('funding_restart',{})for _ in range(17)])]
 out=[];states={}
 for name,actions in runs:
  s=State(actions[0][1].get('created',CREATED));steps=[];now=C+100
  for i,(op,kw0)in enumerate(actions):
   kw=dict(kw0);now=kw.pop('at',max(now+1,s.clock+1));kw.pop('created',None);before=s.snapshot();error=''
   try:s.call(op,now,**kw)
   except Refusal as e:error=str(e)
   after=s.snapshot()
   if after['record']is not None:
    validate('Record',after['record']);rid=record_digest(after['record']);states.setdefault(rid,after.pop('record'))
   else:after.pop('record')
   st=status(s.r,s.clock,s.funding_state,s.funding_generation);validate('Status',st)
   steps.append(dict(operation=op,now_ms=now,arguments=kw,error=error,expected=after,status=st,distinct_modeled_block_heights=s.last_work,unchanged_record=before['record_digest']==after['record_digest']))
  out.append(dict(name=name,kind='modeled_atomic_boundaries_not_runtime_execution',steps=steps))
 return out,states
