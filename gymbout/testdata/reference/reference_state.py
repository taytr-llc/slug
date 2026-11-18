"""Finite specification state models, not executed storage/account/crash tests."""
from common import *
class Model:
 def __init__(self,f):self.f=f;self.clock=f['store_created_at_ms'];self.saved={};self.count=0;self.poison=False;self.closed=False;self.total_engine=0
 def call(self,op,**kw):
  engine=0;out={'operation':op,'error':'','engine_calls':0,'inserted':False,'replayed':False,'record_digest':'','observed_at_ms':self.clock,'count':self.count}
  def done(error=''):
   out.update(error=error,engine_calls=engine,count=self.count,observed_at_ms=self.clock,poisoned=self.poison);return out
  if op=='close':self.closed=True;return done()
  if op=='reopen':self.closed=False;self.poison=False;return done()
  if self.closed:return done('bout_unavailable')
  if self.poison:return done('bout_unavailable')
  if not kw.get('authorized',True):return done('unauthorized')
  if kw.get('cancel_before',False):return done('bout_cancelled')
  now=kw.get('now',self.clock);bid=kw.get('bout_id',self.f['source']['bout']['bout_id']);exists=bid in self.saved
  if not kw.get('source_same',True):return done('bout_source_changed')
  if now<self.clock or now<self.f['source']['card_created_at_ms']:return done('bout_clock')
  if op=='status':self.clock=now;out.update(exists=exists,usable=exists and kw.get('usable',True));return done('outcome_unknown'if kw.get('after_observation_fail')else'')
  if op=='read':
   self.clock=now
   if not exists:return done('bout_not_found')
   if kw.get('after_observation_fail'):return done('outcome_unknown')
   out.update(record_digest=self.saved[bid],usable=kw.get('usable',True));return done()
  assert op=='run'
  if exists:
   self.clock=now
   if kw.get('after_observation_fail'):return done('outcome_unknown')
   out.update(replayed=True,record_digest=self.saved[bid],usable=kw.get('usable',True));return done()
  if not kw.get('booked',True):return done('bout_not_found')
  if not kw.get('complete',True):return done('bout_source_incomplete')
  if not kw.get('usable',True):return done('bout_source_conflict')
  if self.count>=256:return done('bout_capacity')
  engine=1;self.total_engine+=1
  if kw.get('cancel_during_engine'):return done('bout_cancelled')
  if not kw.get('final_authorized',True):return done('unauthorized')
  if not kw.get('final_same',True):return done('bout_source_changed')
  if not kw.get('final_usable',True):return done('bout_source_conflict')
  if kw.get('write_fail'):
   self.poison=True
   if kw.get('write_reached_disk',False):self.saved[bid]=self.f['record_digest'];self.count+=1;self.clock=now
   return done('outcome_unknown')
  self.saved[bid]=self.f['record_digest'];self.count+=1;self.clock=now
  if kw.get('after_commit_fail'):return done('outcome_unknown')
  out.update(inserted=True,record_digest=self.saved[bid],usable=True);return done()

def scenarios(f):
 n=f['run_at_ms'];base=[('run',{'now':n}),('status',{'now':n+1}),('read',{'now':n+2}),('run',{'now':n+3}),('close',{}),('reopen',{}),('run',{'now':n+4,'usable':False}),('read',{'now':n+5,'usable':False})]
 specs=[('saved_replay_conflict_reopen',base,0),('new_conflict_no_engine',[('run',{'now':n,'usable':False})],0),('old_view_refuses',[('run',{'now':n,'authorized':False})],0),('cancel_before',[('run',{'now':n,'cancel_before':True})],0),('cancel_engine',[('run',{'now':n,'cancel_during_engine':True})],0),('final_revoked',[('run',{'now':n,'final_authorized':False})],0),('final_source_changed',[('run',{'now':n,'final_same':False})],0),('final_conflict',[('run',{'now':n,'final_usable':False})],0),('unknown_committed_query',[('run',{'now':n,'after_commit_fail':True}),('read',{'now':n+1}),('run',{'now':n+2})],0),('unknown_disk_reopen',[('run',{'now':n,'write_fail':True,'write_reached_disk':True}),('read',{'now':n+1}),('reopen',{}),('read',{'now':n+2})],0),('unknown_no_disk_reopen',[('run',{'now':n,'write_fail':True}),('reopen',{}),('read',{'now':n+1})],0),('clock_rollback',[('run',{'now':n}),('read',{'now':n-1})],0),('source_incarnation_changed',[('run',{'now':n}),('read',{'now':n+1,'source_same':False})],0),('observation_unknown_is_not_run',[('status',{'now':n,'after_observation_fail':True}),('read',{'now':n+1})],0),('capacity_256_new_refused',[('run',{'now':n})],256),('capacity_255_insert_then_replay',[('run',{'now':n}),('run',{'now':n+1}),('run',{'now':n+2,'bout_id':label('other booked')})],255)]
 out=[]
 for name,steps,count in specs:
  m=Model(f);m.count=count;rows=[]
  for op,kw in steps:rows.append(dict(action=op,args=kw,expected=m.call(op,**kw)))
  out.append(dict(name=name,scope='modeled_state_only_no_actual_sync_or_auth_execution',initial_saved_count=count,steps=rows,total_engine_calls=m.total_engine))
 return out

def initialization():
 out=[]
 states=['absent','real_empty','marker_only','recognized_init','ready_empty','ready_nonempty','wrong_pin','wrong_source_created','symlink','unknown_files','bad_marker','ready_missing_db']
 for op in ['create','resume_prepared','open_ready']:
  for state in states:
   good=state in (['absent','real_empty']if op=='create'else['absent','real_empty','marker_only','recognized_init','ready_empty']if op=='resume_prepared'else['ready_empty','ready_nonempty'])
   out.append(dict(operation=op,state=state,accept=good,preserves_first_created_at=good and state in ['ready_empty','ready_nonempty'],samples_created_after_durable_intent=good and state not in ['ready_empty','ready_nonempty'],publishes_partial_store=False,scope='recognized_prefix_model_not_crash_execution'))
 return out
