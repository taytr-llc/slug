"""Finite journal contract model, NOT filesystem/crash/lease execution evidence.
Only explicit successful local observations are modeled as clock writes. Source
journal observation/incident write internals remain inherited test obligations.
"""
from common import *

class Store:
 def __init__(self):self.rows={};self.clock=CREATED;self.poison=False;self.closed=False;self.source=clone(SOURCE);self.source_created=SOURCE_CREATED;self.overlay='healthy';self.extra_count=0
 def inspect(self):return dict(clock_ms=self.clock,event_count=len(self.rows)+self.extra_count,poisoned=self.poison,closed=self.closed,source_overlay=self.overlay,record_digests={k:record_digest(v) for k,v in sorted(self.rows.items())},created_at_ms={k:v['created_at_ms'] for k,v in sorted(self.rows.items())})
 def call(self,op,**args):
  now=args.get('now',CREATED+10);a=args.get('auth','current');r=self.rows.get(card_id(E));mut=False
  if op=='reopen':self.poison=False;self.closed=False;return dict(error='',record=None,replayed=None)
  if op=='close':self.closed=True;return dict(error='',record=None,replayed=None)
  if op=='overlay':self.overlay=args['value'];return dict(error='',record=None,replayed=None)
  if op=='capacity':self.extra_count=256-len(self.rows);return dict(error='',record=None,replayed=None)
  if op=='source_changed':self.source_created+=1;return dict(error='',record=None,replayed=None)
  error=''
  if self.closed:error='card_unavailable'
  elif self.poison:error='card_unavailable'
  elif a in ['foreign','mining']:error='forbidden'
  elif a in ['stale','revoked']:error='stale_view'
  elif a=='expired':error='unauthorized'
  elif a=='cancelled':error='card_cancelled'
  elif self.source_created!=SOURCE_CREATED:error='card_source_changed'
  elif now<self.clock or now<SOURCE['finality']['observed_at_ms']:error='card_clock'
  elif op in ['read','build'] and r is not None:
   # Existing immutable replay bypasses new capacity/healthy-source predicate.
   pass
  elif op=='read':error='card_not_found'
  elif op=='build':
   if self.overlay=='incomplete':error='card_source_incomplete'
   elif self.overlay in ['conflict','overflow']:error='card_source_conflict'
   elif self.extra_count+len(self.rows)>=256:error='card_capacity'
   elif args.get('final_auth')in ['revoked','expired']:error='stale_view' if args['final_auth']=='revoked' else 'unauthorized'
   elif args.get('final_overlay')in ['conflict','overflow']:error='card_source_conflict'
   elif args.get('final_source_changed'):error='card_source_changed'
   elif args.get('final_now',now)<now:error='card_clock'
   elif args.get('cancel_before_sync'):error='card_cancelled'
   else:
    now=args.get('final_now',now);r=record(now=now);mut=True
    if args.get('sync')=='unknown_not_written':self.poison=True;error='outcome_unknown';r=None
    else:self.rows[r['card_id']]=r;self.clock=now
    if args.get('sync')=='unknown_written':self.poison=True;error='outcome_unknown'
    if args.get('after_sync_error'):error='outcome_unknown'
  elif op!='status':error='invalid_request'
  if not error and args.get('final_auth')in ['revoked','expired']:error='outcome_unknown' if mut else ('stale_view' if args['final_auth']=='revoked' else 'unauthorized')
  if not error and args.get('final_source_changed'):error='outcome_unknown' if mut else 'card_source_changed'
  if not error and r is not None and args.get('final_overlay')in ['conflict','overflow']:self.overlay=args['final_overlay']
  if error:return dict(error=error,record=None,replayed=None)
  self.clock=max(self.clock,now)
  if args.get('observation_unknown'):self.poison=True;return dict(error='outcome_unknown',record=None,replayed=None)
  usable=bool(r and self.overlay not in ['conflict','overflow','incomplete'])
  return dict(error='',record=clone(r) if op in ['build','read'] else None,replayed=(not mut) if op=='build' else None,exists=bool(r),usable=usable)

def scenarios():
 cases={
 'first_repeat_reopen_held':[('build',{}),('build',{'now':CREATED+20}),('reopen',{}),('build',{'auth':'held','now':CREATED+21})],
 'conflict_before_and_historical':[('overlay',{'value':'conflict'}),('build',{}),('overlay',{'value':'healthy'}),('build',{}),('overlay',{'value':'conflict'}),('build',{'now':CREATED+20}),('read',{'now':CREATED+21})],
 'offline_and_shallow_historical':[('overlay',{'value':'offline'}),('build',{}),('overlay',{'value':'shallow'}),('build',{'now':CREATED+20})],
 'incomplete_is_not_empty':[('overlay',{'value':'incomplete'}),('build',{}),('read',{})],
 'capacity_new_refused':[('capacity',{}),('build',{})],
 'capacity_existing_replay':[('build',{}),('capacity',{}),('build',{'now':CREATED+20})],
 'unknown_written_recovery':[('build',{'sync':'unknown_written'}),('read',{}),('reopen',{}),('read',{'now':CREATED+20}),('build',{'now':CREATED+21})],
 'unknown_unwritten_recovery':[('build',{'sync':'unknown_not_written'}),('reopen',{}),('read',{}),('build',{'now':CREATED+20})],
 'post_sync_revocation_unknown':[('build',{'after_sync_error':True}),('build',{'auth':'held','now':CREATED+20})],
 'clock_rollback':[('build',{'now':CREATED+20}),('read',{'now':CREATED+19}),('reopen',{}),('build',{'now':CREATED+19}),('read',{'now':CREATED+20})],
 'source_creation_loss':[('build',{}),('source_changed',{}),('read',{'now':CREATED+20})],
 'pre_sync_rechecks':[('build',{'final_auth':'revoked'}),('build',{'final_overlay':'conflict'}),('build',{'final_source_changed':True}),('build',{'cancel_before_sync':True}),('build',{'final_now':CREATED+9}),('build',{})],
 'read_observation_uncertainty':[('build',{}),('read',{'observation_unknown':True,'now':CREATED+20}),('reopen',{}),('read',{'now':CREATED+21})],
 'no_implicit_build':[('status',{}),('read',{}),('reopen',{}),('status',{}),('close',{}),('build',{})],
 'replay_final_original_view':[('build',{}),('build',{'final_auth':'revoked','now':CREATED+20}),('read',{'final_auth':'expired','now':CREATED+21}),('build',{'final_overlay':'conflict','now':CREATED+22})],
 'original_authorization':[('build',{'auth':a}) for a in ['foreign','mining','stale','revoked','expired','cancelled','held']]
 }
 out=[]
 for name,steps in cases.items():
  m=Store();rows=[]
  for op,args in steps:
   before=m.inspect();result=m.call(op,**args);rows.append(dict(operation=op,arguments=args,before=before,result=result,after=m.inspect()))
  out.append(dict(name=name,scope='journal_transition_model_no_actual_crash_or_account_lease',steps=rows))
 return out

def initialization():
 out=[]
 for mode in ['create','open','resume_prepared']:
  for shape in ['absent','empty_directory','marker_only','recognized_init','ready_empty','ready_populated','unknown_db','missing_marker','wrong_pin','symlink']:
   allow=(mode=='create' and shape in ['absent','empty_directory']) or (mode=='open' and shape in ['ready_empty','ready_populated']) or(mode=='resume_prepared' and shape in ['absent','empty_directory','marker_only','recognized_init','ready_empty'])
   out.append(dict(mode=mode,shape=shape,accept=allow,preserve_existing_time=allow and shape.startswith('ready'),fresh_sample=allow and not shape.startswith('ready'),scope='structural_initialization_model'))
 for mode in ['create','open','resume_prepared']:out.append(dict(mode=mode,shape='cheap_pow_valid_root',accept=False,filesystem_effects=False,scope='profile_preflight_obligation'))
 return out

def history_cases():
 rows=calendar_rows()[:256];index=[dict(card_id=x['card_id'],cutoff_ms=x['event']['cutoff_ms']) for x in rows];out=[]
 for after,limit in [('',1),('',8),(index[7]['card_id'],8),(index[247]['card_id'],8),(index[254]['card_id'],8),(index[255]['card_id'],8),(label('unknown cursor'),8),('',0),('',9)]:
  error='';start=0
  if limit not in range(1,9):error='invalid_request'
  elif after:
   found=[i for i,x in enumerate(index) if x['card_id']==after]
   if not found:error='card_not_found'
   else:start=found[0]+1
  selected=[] if error else index[start:start+limit];more=not error and start+len(selected)<len(index)
  out.append(dict(after_card_id=after,limit=limit,error=error,selected=selected,has_more=more,next_card_id=selected[-1]['card_id'] if more else '',full_record_validations=len(selected),scope='bounded_audited_ID_date_index_selection_model_not_256_trusted_source_records'))
 return dict(index=index,cases=out,selected_corruption=dict(after_card_id=index[247]['card_id'],limit=8,corrupt_selected_offset=3,error='card_corrupt',returned_payload=False),cancel_between_rows=dict(after_card_id='',limit=8,cancel_after_validations=1,error='card_cancelled',returned_payload=False))
