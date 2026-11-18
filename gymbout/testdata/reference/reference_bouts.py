#!/usr/bin/env python3
"""Finite Set47 composition/canonical/protocol corpus; no Go execution/imports."""
from common import *
import schema as S

def replace_at(v,path,value,remove=False):
 v=clone(v);keys=path.strip('/').split('/');p=v
 for k in keys[:-1]:p=p[int(k)]if isinstance(p,list)else p[k]
 k=int(keys[-1])if isinstance(p,list)else keys[-1]
 if remove:del p[k]
 else:p[k]=clone(value)
 return v

def materialize(base,recipe):
 v=clone(base);op=recipe['op']
 if op=='identity':return J(v)
 if op=='set':return J(replace_at(v,recipe['path'],recipe['value']))
 if op=='set_many':
  for edit in recipe['edits']:v=replace_at(v,edit['path'],edit['value'])
  return J(v)
 if op=='remove':return J(replace_at(v,recipe['path'],None,True))
 if op=='reverse':
  keys=recipe['path'].strip('/').split('/');p=v
  for k in keys:p=p[int(k)]if isinstance(p,list)else p[k]
  p.reverse();return J(v)
 if op=='whitespace_reversed':
  def rev(x):return {k:rev(x[k])for k in reversed(x)}if type(x)is dict else[rev(y)for y in x]if type(x)is list else x
  return (' \n\t'+json.dumps(rev(v),ensure_ascii=True,indent=1)+'\r\n').encode('ascii')
 if op=='duplicate_top':
  b=J(v);key=recipe.get('key',next(iter(v)));return b[:-1]+b','+J(key)+b':'+J(v[key])+b'}'
 if op=='escaped_key_duplicate':
  b=J(v);key=next(iter(v));escaped='\\u%04x'%ord(key[0])+key[1:];return b[:-1]+b',"'+escaped.encode()+b'":'+J(v[key])+b'}'
 if op=='replace_once':
  b=J(v);a=recipe['old'].encode();req=b.count(a);assert req>0;return b.replace(a,recipe['new'].encode(),1)
 if op=='pad':
  b=J(v);assert len(b)<=recipe['bytes'];return b+b' '*(recipe['bytes']-len(b))
 raise ValueError('unknown finite mutation recipe')

def fixtures():
 out=[]
 for x in composition_sources():
  inp=derive_input(x['source']);res=ENGINE.simulate(inp);r=make_record(x['pin'],x['source'],res,x['run_at_ms']);x=clone(x)
  x.update(input=inp,input_id=ENGINE.input_id(inp),result=res,result_digest=ENGINE.digest(res),record=r,record_digest=record_digest(r),canonical_source=J(x['source']).decode(),canonical_input=J(inp).decode(),canonical_result=J(res).decode(),canonical_record=J(r).decode())
  out.append(x)
 return out

def dtos_for(fs):
 out=[]
 def add(kind,name,v,f=0,context='core'):
  out.append(dict(kind=kind,name=name,fixture_index=f,context=context,value=clone(v),canonical=J(v).decode(),sha256=sha(J(v))))
 f=fs[0];p=f['pin'];s=f['source'];r=f['record'];n=r['created_at_ms'];st=status(p,s,r,n);rr=dict(version=1,record=r,status=st,replayed=False)
 add('Rules','compiled',RULES)
 for i in [0,1]:
  f=fs[i];p=f['pin'];s=f['source'];r=f['record'];n=r['created_at_ms'];st=status(p,s,r,n)
  add('Pin','authority-'+str(i),p,i);info=dict(version=1,store_id=store_id(p),pin=p,source_created_at_ms=f['source_store_created_at_ms'],created_at_ms=f['store_created_at_ms'],bout_count=0);add('Info','empty-'+str(i),info,i)
  add('Marker','authority-'+str(i),dict(application='SLUG candidate bouts',version=1,store_id=store_id(p),pin=p),i)
  add('Init','prepared-'+str(i),{k:info[k]for k in ['version','store_id','pin','source_created_at_ms']},i)
  add('Metadata','empty-'+str(i),{**{k:info[k]for k in ['version','store_id','pin','source_created_at_ms','created_at_ms']},'clock_ms':f['store_created_at_ms'],'bout_count':0},i)
 for i,f in enumerate(fs):
  p=f['pin'];s=f['source'];r=f['record'];n=r['created_at_ms'];st=status(p,s,r,n)
  add('BoutSource',f['name'],s,i);add('Record',f['name'],r,i)
  add('Status','saved/'+f['name'],st,i);add('Status','unsaved/'+f['name'],status(p,s,None,n),i)
  add('RunResult','inserted/'+f['name'],dict(version=1,record=r,status=st,replayed=False),i)
  add('ReadResult','saved/'+f['name'],dict(version=1,record=r,status=st),i)
 f=fs[0];p=f['pin'];s=f['source'];r=f['record'];n=r['created_at_ms'];st=status(p,s,r,n);cf=status(p,s,r,n+1,True)
 for kind,v in [('Bout',s['bout']),('BoutRef',dict(bout=s['bout'],source_digest=source_digest(s))),('SavedBout',saved(s,r)),('BoutState',source_state(s,n))]:add(kind,'first',v)
 add('Status','historical_conflict',cf);add('RunResult','historical_conflict',dict(version=1,record=r,status=cf,replayed=True));add('ReadResult','historical_conflict',dict(version=1,record=r,status=cf))
 false=clone(st);false['saved']['usable']=False;add('Status','standalone_false_healthy',false)
 late=clone(st);later=s['event']['incident_deadline_ms']+10;late['observed_at_ms']=late['source']['observed_at_ms']=later;late['source']['incident_at_ms']=later-1;add('Status','historical_incident_late_finality',late)
 absent=source_state(s,n);absent.update(card_exists=False,card_record_digest='',card_source_digest='',card_created_at_ms=0,finalized_at_ms=0,chain_observed_at_ms=0,target_descendants=0,current_finality=False,node_state='unobserved',usable=False)
 add('BoutState','absent',absent);add('CardStatus','absent',dict(version=1,store_id=store_id(p),card_id=s['card_id'],observed_at_ms=n,source=absent,bouts=[]))
 add('CardStatus','completed_empty',dict(version=1,store_id=store_id(p),card_id=s['card_id'],observed_at_ms=n,source=source_state(s,n),bouts=[]))
 for order in [0,1]:
  selected=fs[1+order*5:6+order*5];f=selected[0];p=f['pin'];s=f['source'];n=max(x['run_at_ms']for x in selected)
  bs=[saved(x['source'],x['record'])for x in selected]
  add('CardStatus','five_saved_attribution_'+str(order),dict(version=1,store_id=store_id(p),card_id=s['card_id'],observed_at_ms=n,source=source_state(s,n),bouts=bs),1+order*5)
  owner=s['corners'][0]['owner']
  add('BoutView','five_refs_attribution_'+str(order),dict(version=1,store_id=card_store_id(p['source_pin']),pin=p['source_pin'],created_at_ms=f['source_store_created_at_ms'],participant_account_id=owner['account_id'],participant_gym_id=owner['gym_id'],observed_at_ms=n,state=source_state(s,n),bouts=[dict(bout=x['source']['bout'],source_digest=source_digest(x['source']))for x in selected]),1+order*5)
 f=fs[0];p=f['pin'];s=f['source'];r=f['record'];n=r['created_at_ms'];st=status(p,s,r,n)
 for vi,v in enumerate(f['views']):
  ready=dict(version=1,state='ready',error='',view=v,store_id=store_id(p),pin=p,source_created_at_ms=f['source_store_created_at_ms'],created_at_ms=f['store_created_at_ms'],active_job_id='',queued_job_id='',last_job_id='');add('BoutSnapshot','ready-'+str(vi),ready,context='transport')
  setup=clone(ready);setup.update(state='setup_incomplete',error='bout_setup_incomplete',pin=None,store_id='',source_created_at_ms=0,created_at_ms=0);add('BoutSnapshot','setup-'+str(vi),setup,context='transport')
  queued=dict(version=1,job_id=label('job/'+str(vi)),kind='run',state='queued',view=v,store_id=store_id(p),card_id=s['card_id'],saturday=s['event']['saturday'],bout_id=s['bout']['bout_id'],error='',result=None)
  for state_ in ['queued','running','completed','failed','cancelled']:
   j=clone(queued);j['state']=state_
   if state_=='completed':j['result']=dict(version=1,record=r,status=st,replayed=False)
   if state_=='failed':j['error']='outcome_unknown'
   if state_=='cancelled':j['error']='bout_cancelled'
   add('BoutJob',state_+'-'+str(vi),j,context='transport')
  busy=clone(ready);busy.update(state='busy',queued_job_id=queued['job_id']);add('BoutSnapshot','busy-'+str(vi),busy,context='transport')
  for error in ['bout_unavailable','bout_corrupt','bout_clock','bout_source_changed']:
   failed=clone(ready);failed.update(state='failed',error=error);add('BoutSnapshot','failed-'+error+'-'+str(vi),failed,context='transport')
 add('BoutSnapshot','unavailable',dict(version=1,state='unavailable',error='bout_unavailable',view=None,store_id='',pin=None,source_created_at_ms=0,created_at_ms=0,active_job_id='',queued_job_id='',last_job_id=''),context='transport')
 add('BoutResponse','unavailable_snapshot',dict(id=1,ok=True,view=None,store_id='',snapshot=clone(out[-1]['value']),card_status=None,status=None,record=None,job=None,error=''),context='transport')
 return out

def codec_cases(ds,fs):
 out=[]
 def add(i,name,recipe,accept,context=None):
  d=ds[i];ctx=context or d['context'];b=materialize(d['value'],recipe)
  out.append(dict(name=d['kind']+'/'+d['name']+'/'+name,kind=d['kind'],fixture_index=d['fixture_index'],context=ctx,base=i,recipe=recipe,input_sha256=sha(b),input_bytes=len(b),accept=accept,canonical_ref=i if accept and recipe['op']in ['identity','whitespace_reversed','pad']else None,canonical=J(json.loads(b)).decode()if accept and recipe['op']=='set_many'else None))
 # Each representative has exact bytes and a canonical whitespace/order control.
 for i,d in enumerate(ds):
  add(i,'canonical',dict(op='identity'),True)
  if d['kind']not in ['Record','RunResult','ReadResult']:add(i,'ordered_whitespace',dict(op='whitespace_reversed'),True)
  if d['kind']in ['Record','RunResult','ReadResult']and d['fixture_index']!=0:continue
  add(i,'missing_first_field',dict(op='remove',path='/'+next(iter(d['value']))),False)
  add(i,'duplicate_first_field',dict(op='duplicate_top'),False)
  add(i,'unknown_key',dict(op='set',path='/unexpected',value=0),False)
  add(i,'boolean_version',dict(op='set',path='/version',value=True),False)
 # Type and semantic controls on one representative, not every full transcript.
 def idx(k,n):return next(i for i,d in enumerate(ds)if d['kind']==k and d['name']==n)
 si=idx('BoutSource',fs[0]['name']);ri=idx('Record',fs[0]['name']);st=idx('Status','saved/'+fs[0]['name']);run=idx('RunResult','inserted/'+fs[0]['name'])
 for name,path,value in [('wrong_source_store','/store_id','0'*64),('source_birth_after_cutoff','/corners/0/member/assignment/born_at_ms',fs[0]['source']['event']['cutoff_ms']+1),('own_not_rostered','/corners/0/member/slot',0),('wrong_booked_seed','/bout/seed','0'*64),('wrong_corner','/bout/corner_a','f'*64),('duplicate_owner','/corners/1/owner',fs[0]['source']['corners'][0]['owner']),('wrong_rating','/corners/0/member/condition_at_cutoff/rating_hundredths',1),('null_corners','/corners',None),('bool_slot','/corners/0/member/slot',True)]:add(si,name,dict(op='set',path=path,value=value),False)
 for name,path,value in [('wrong_input_seed','/input/seed','0'*64),('current_time_replaces_cutoff','/input/as_of_ms',fs[0]['run_at_ms']),('nonzero_xp','/input/a/pro_xp',1),('training_bonus','/input/b/training_skill',1),('source_digest_drift','/source_digest','0'*64),('result_digest_drift','/result_digest','0'*64),('negative_unrelated','/result/end_tick',-1),('bool_form','/result/initial/0/form_q',False),('form_outside','/result/initial/0/form_q',-100001),('null_rounds','/result/rounds',None),('creation_before_card','/created_at_ms',fs[0]['source']['card_created_at_ms']-1)]:add(ri,name,dict(op='set',path=path,value=value),False)
 add(ri,'escaped_key_duplicate',dict(op='escaped_key_duplicate'),False)
 for n,repl in [('negative_zero','-0'),('exponent','1e0'),('float','1.0')]:add(ri,n,dict(op='replace_once',old='"version":1',new='"version":'+repl),False)
 add(run,'full_false_healthy',dict(op='set',path='/status/saved/usable',value=False),False)
 add(st,'negative_seed_sentinel_not_in_projection',dict(op='set',path='/source/target_descendants',value=-1),False)
 for k,n in [('BoutSnapshot','ready-0'),('BoutJob','completed-0')]:
  i=idx(k,n);add(i,'null_wrong_payload',dict(op='set',path='/view',value=None),False,context='transport')
 # Rehashed random-draw accounting cannot be authenticated by a cheap parser.
 # It is structurally bounded but disagrees with unchanged engine execution.
 altered=clone(fs[0]['result']);altered['draw_counts']['combat']+=1
 recipe=dict(op='set_many',edits=[dict(path='/result/draw_counts/combat',value=altered['draw_counts']['combat']),dict(path='/result_digest',value=sha(ENGINE.RESULT_DOMAIN+J(altered)))])
 add(ri,'rehashed_draw_count_full_engine_refuses',recipe,False,'core')
 add(ri,'rehashed_draw_count_transport_only',recipe,True,'transport')
 # Standalone caps and framed helper caps differ by the required LF byte.
 for kind in ['BoutSource','Record','BoutRequest','BoutResponse']:
  i=next(i for i,d in enumerate(ds)if d['kind']==kind and(kind!='BoutResponse'or d['name']=='protocol-0'))
  cap=S.CAPS[kind]-(kind in ['BoutRequest','BoutResponse'])
  add(i,'raw_cap_exact',dict(op='pad',bytes=cap),True)
  add(i,'raw_cap_plus_one',dict(op='pad',bytes=cap+1),False)
 # Extra arrays never gain permission merely from a familiar field suffix.
 add(si,'unrooted_array_suffix',dict(op='set',path='/unexpected',value={'corners':[]}),False)
 # Exact numeric spellings stay strict even when Python value equality would coerce.
 for i,d in enumerate(ds):
  if d['kind']=='BoutSnapshot'and d['value']['state']in ['setup_incomplete','unavailable']:
   for field in ['source_created_at_ms','created_at_ms']:add(i,'false_'+field,dict(op='set',path='/'+field,value=False),False)
  if d['kind']=='BoutResponse'and d['value']['snapshot']is not None and d['value']['snapshot']['state']in ['setup_incomplete','unavailable']:
   for field in ['source_created_at_ms','created_at_ms']:add(i,'wrapped_false_'+field,dict(op='set',path='/snapshot/'+field,value=False),False)
 altered=clone(fs[0]['result']);altered['version']=True
 recipe=dict(op='set_many',edits=[dict(path='/result/version',value=True),dict(path='/result_digest',value=sha(ENGINE.RESULT_DOMAIN+J(altered)))])
 for context in ['core','transport']:add(ri,'rehashed_boolean_result_version_'+context,recipe,False,context)
 # Rebind all cheap commitments so the named manifest field bound is exercised.
 def manifest_recipe(path,value):
  rec=clone(fs[0]['record']);rec=replace_at(rec,'/input/a/manifest/'+path,value)
  man=rec['input']['a']['manifest']
  if path in ['name/first','name/last']:man['name']['display']=man['name']['first']+' '+man['name']['last']
  rec['source']['corners'][0]['member']['assignment']['manifest_digest']=sha(MANIFEST.DIGEST_DOMAIN+J(man));rec['source_digest']=source_digest(rec['source']);rec['input_id']=sha(ENGINE.INPUT_DOMAIN+J(rec['input']));rec['result']['input_id']=rec['input_id'];rec['result_digest']=sha(ENGINE.RESULT_DOMAIN+J(rec['result']))
  return dict(op='set_many',edits=[dict(path='/'+k,value=rec[k])for k in ['source','source_digest','input','input_id','result','result_digest']])
 for name,path,value,accept in [('first64','name/first','Q'*64,True),('first65','name/first','Q'*65,False),('last65','name/last','Q'*65,False),('empty_first','name/first','',False),('display130','name/display','Q'*130,False),('portrait_skin6','portrait/skin',6,False),('height81','physical/height_in',81,False),('attribute101','dna/attributes/power',101,False)]:
  add(ri,'rehashed_manifest_'+name,manifest_recipe(path,value),accept,'transport')
 for field,value in [('aged_attributes_tenths/power',1001),('league_attributes_tenths/power',1001),('age_weeks',91),('prime_remaining_weeks',53),('career_remaining_ms',91*WEEK+1)]:add(si,'condition_bound_'+field,dict(op='set',path='/corners/0/member/condition_at_cutoff/'+field,value=value),False,'transport')
 vi=idx('BoutView','five_refs_attribution_0');vv=ds[vi]['value']
 add(vi,'duplicate_bout_id',dict(op='set',path='/bouts/1/bout/bout_id',value=vv['bouts'][0]['bout']['bout_id']),False)
 add(vi,'cross_class_duplicate_prospect',dict(op='set',path='/bouts/1/bout/corner_a',value=vv['bouts'][0]['bout']['corner_a']),False)
 for size in [1,3]:add(si,'corners_length_'+str(size),dict(op='set',path='/corners',value=(fs[0]['source']['corners']*2)[:size]),False)
 qi=next(i for i,d in enumerate(ds)if d['kind']=='BoutRequest'and d['value']['command']=='bout_snapshot');add(qi,'unused_view_not_empty',dict(op='set',path='/view_id',value=fs[0]['views'][0]['view_id']),False)
 bi=idx('BoutSnapshot','busy-0');bj=ds[bi]['value']['queued_job_id']
 add(bi,'simultaneous_active_queued',dict(op='set',path='/active_job_id',value=label('another job')),False)
 add(bi,'last_alias_queued',dict(op='set',path='/last_job_id',value=bj),False)
 return out

def response_value(q,v,p,k,value):return dict(id=q['id'],ok=True,view=clone(v),store_id=store_id(p),snapshot=None,card_status=None,status=None,record=None,job=None,error='',**{})|{k:clone(value)}
def protocol_cases(ds,fs):
 out=[];f=fs[0];p=f['pin'];s=f['source'];r=f['record'];n=r['created_at_ms']
 def d(k,name):return next(x['value']for x in ds if x['kind']==k and x['name']==name)
 def add(name,q,response,current,accept,expected=None,terminal=''):
  out.append(dict(name=name,fixture_index=0,current_view=clone(current),authority_pin=clone(p),source_created_at_ms=f['source_store_created_at_ms'],created_at_ms=f['store_created_at_ms'],request=clone(q),response=clone(response),expected_job=clone(expected),terminal_digest=terminal,accept=accept))
 for vi,v in enumerate(f['views']):
  for command in ['bout_snapshot','bout_card_status','bout_status','bout_record','bout_run','bout_job','bout_cancel']:
   q=dict(id=10+vi*20,command=command,view_id=''if command=='bout_snapshot'else v['view_id'],saturday=s['event']['saturday']if command in ['bout_card_status','bout_status','bout_record','bout_run']else'',bout_id=s['bout']['bout_id']if command in ['bout_status','bout_record','bout_run']else'',job_id=label('job/'+str(vi))if command in ['bout_job','bout_cancel']else'')
   k={'bout_snapshot':'snapshot','bout_card_status':'card_status','bout_status':'status','bout_record':'record','bout_run':'job','bout_job':'job','bout_cancel':'job'}[command]
   val=d('BoutSnapshot','ready-'+str(vi))if k=='snapshot'else dict(version=1,store_id=store_id(p),card_id=s['card_id'],observed_at_ms=n,source=source_state(s,n),bouts=[saved(s,r)])if k=='card_status'else d('Status','saved/'+f['name'])if k=='status'else d('ReadResult','saved/'+f['name'])if k=='record'else d('BoutJob',('queued-'if command=='bout_run'else'completed-')+str(vi))
   rsp=response_value(q,v,p,k,val);ex=d('BoutJob','running-'+str(vi))if command in ['bout_job','bout_cancel']else None
   add(str(vi)+'/'+command+'/valid',q,rsp,v,True,ex)
   wrong=clone(rsp);wrong['id']+=1;add(str(vi)+'/'+command+'/request_id',q,wrong,v,False,ex)
   if k!='snapshot':
    wrong=clone(rsp);wrong['view']=clone(f['views'][1-vi]);add(str(vi)+'/'+command+'/wrong_view',q,wrong,v,False,ex)
   if command=='bout_run':
    wrong=clone(rsp);wrong['job']=d('BoutJob','completed-'+str(vi));add(str(vi)+'/begin_completed_not_ack',q,wrong,v,False)
   if command=='bout_job':
    wrong=clone(rsp);wrong['job']=d('BoutJob','queued-'+str(vi));add(str(vi)+'/running_regression',q,wrong,v,False,ex)
    term=clone(val);add(str(vi)+'/terminal_repeat',q,rsp,v,True,term,sha(J(term)))
    wrong=clone(rsp);wrong['job']['result']['status']['observed_at_ms']+=1;wrong['job']['result']['status']['source']['observed_at_ms']+=1;add(str(vi)+'/terminal_observation_changed',q,wrong,v,False,term,sha(J(term)))
    wrong=clone(rsp);wrong['job']['bout_id']=label('other bout');add(str(vi)+'/job_tuple_changed',q,wrong,v,False,ex)
    add(str(vi)+'/snapshot_discovered_completed',q,rsp,v,True)
    wrong=clone(rsp);wrong['job']=d('BoutJob','running-'+str(vi));add(str(vi)+'/terminal_regression',q,wrong,v,False,term,sha(J(term)))
   if command=='bout_snapshot':
    wrong=clone(rsp);wrong['snapshot']['created_at_ms']+=1;add(str(vi)+'/snapshot_creation_changed',q,wrong,v,False)
    setup=response_value(q,v,p,'snapshot',d('BoutSnapshot','setup-'+str(vi)));setup['store_id']='';add(str(vi)+'/setup_snapshot',q,setup,v,True)
   if command=='bout_record':
    wrong=clone(rsp);wr=wrong['record'];wr['record']['created_at_ms']=s['card_created_at_ms'];wr['status']['saved']['created_at_ms']=s['card_created_at_ms'];wr['status']['saved']['record_digest']=record_digest(wr['record']);add(str(vi)+'/record_before_journal_created',q,wrong,v,False)
   if command=='bout_status':
    wrong=clone(rsp);st=wrong['status'];st['observed_at_ms']=st['source']['observed_at_ms']=s['card_created_at_ms'];st['saved']['created_at_ms']=s['card_created_at_ms'];add(str(vi)+'/status_before_known_journal_creation',q,wrong,v,False)
   if command in ['bout_status','bout_record','bout_run']:
    wrongq=clone(q);wrongq['view_id']=label('retired initiating view');add(str(vi)+'/'+command+'/old_request_view',wrongq,rsp,v,False,ex)
    err=dict(id=q['id'],ok=False,view=clone(v),store_id=store_id(p),snapshot=None,card_status=None,status=None,record=None,job=None,error='outcome_unknown');add(str(vi)+'/'+command+'/typed_unknown_no_payload',q,err,v,True)
 return out

def calendar_cases():
 rows=[]
 for d in range(cal['days'](2026,1,1),cal['days'](2100,1,1)):
  if (d+4)%7!=6:continue
  y,m,day=cal['civil'](d);date='%04d-%02d-%02d'%(y,m,day)
  c,n=cutoff(date),deadline(date)
  rows.append(dict(saturday=date,cutoff_ms=c,incident_deadline_ms=n,hours_to_next=(n-c)//3600000))
 return rows

def identities_cases(fs):
 out=[]
 for f in fs:
  s=f['source'];inp=f['input'];changed=clone(s)
  changed['card_record_digest']=label('different immutable evidence/'+f['name'])
  changed['card_source_digest']=label('different omitted evidence/'+f['name'])
  changed['card_created_at_ms']+=1;changed['finalized_at_ms']+=1
  other=derive_input(changed)
  assert other==inp and source_digest(changed)!=source_digest(s)
  later=clone(f['record']);later['created_at_ms']+=1
  out.append(dict(name=f['name'],scope='pure_recipe_invariance_not_permission_to_rewrite_retained_source',source_digest=source_digest(s),changed_evidence_source=changed,changed_source_digest=source_digest(changed),input_id=f['input_id'],changed_input_id=ENGINE.input_id(other),input_bytes_unchanged=True,result_bytes_unchanged=True,record_digest=f['record_digest'],later_creation_record_digest=record_digest(later),later_run_does_not_replace_as_of=s['event']['cutoff_ms']))
 return out

def portrait_cases(fs,ds):
 f=fs[0];p=f['pin'];r=f['record'];s=f['source'];cases=[]
 svg='<svg xmlns="http://www.w3.org/2000/svg"></svg>'
 def add(name,reply,view,accept,cache=False,selected=True):
  cases.append(dict(name=name,fixture_index=0,current_view=clone(view),selected_record_fixture=0 if selected else None,source_created_at_ms=f['source_store_created_at_ms'],created_at_ms=f['store_created_at_ms'],cache_hit=cache,requires_fresh_status=True,response=clone(reply),accept=accept,scope='structural_envelope_and_selected_tuple_only; SVG_is_deliberate_placeholder_not_renderer_or_digest_evidence'))
 def dto(kind,name,value):ds.append(dict(kind=kind,name=name,fixture_index=0,context='transport',value=clone(value),canonical=J(value).decode(),sha256=sha(J(value))))
 for vi,v in enumerate(f['views']):
  for side,c in enumerate(s['corners']):
   a=c['member']['assignment'];pt=dict(version=1,view=clone(v),store_id=store_id(p),card_id=r['card_id'],saturday=s['event']['saturday'],bout_id=r['bout_id'],prospect_id=a['prospect_id'],record_digest=record_digest(r),input_id=r['input_id'],result_digest=r['result_digest'],descriptor_id=a['descriptor_id'],manifest_digest=a['manifest_digest'],portrait_digest=a['portrait_digest'],svg=svg)
   response=dict(version=1,ok=True,portrait=pt,status=status(p,s,r,r['created_at_ms']+1),error='')
   dto('BoutPortrait','structural_only/'+str(vi)+'/'+str(side),pt);dto('BoutPortraitResponse','structural_only/'+str(vi)+'/'+str(side),response)
   for cache in [False,True]:add('fresh_status/'+str(vi)+'/'+str(side)+'/'+str(cache),response,v,True,cache)
   conflict=clone(response);conflict['status']=status(p,s,r,r['created_at_ms']+2,True);add('historical_conflict/'+str(vi)+'/'+str(side),conflict,v,True,True)
   for field in ['input_id','result_digest','record_digest','manifest_digest','portrait_digest']:
    wrong=clone(response);wrong['portrait'][field]=label('wrong '+field);add('wrong_'+field+'/'+str(vi)+'/'+str(side),wrong,v,False,True)
   wrong=clone(response);wrong['status']=None;add('cache_hit_missing_fresh_status/'+str(vi)+'/'+str(side),wrong,v,False,True)
   wrong=clone(response);wrong['portrait']['view']=clone(f['views'][1-vi]);add('cache_hit_old_view/'+str(vi)+'/'+str(side),wrong,v,False,True)
   add('no_selected_tuple/'+str(vi)+'/'+str(side),response,v,False,True,False)
 for error in ['busy','stale_view','outcome_unknown','bout_not_found']:
  response=dict(version=1,ok=False,portrait=None,status=None,error=error);dto('BoutPortraitResponse','error/'+error,response);add('typed_'+error,response,f['views'][0],True,True)
 return cases

def generate():
 from reference_state import scenarios,initialization
 fs=fixtures();ds=dtos_for(fs);ps=protocol_cases(ds,fs)
 for i,c in enumerate(ps):
  if not c['accept']:continue
  for kind,key in [('BoutRequest','request'),('BoutResponse','response')]:
   v=c[key];ds.append(dict(kind=kind,name='protocol-'+str(i),fixture_index=c['fixture_index'],context='transport',value=clone(v),canonical=J(v).decode(),sha256=sha(J(v))))
 portraits=portrait_cases(fs,ds);cs=codec_cases(ds,fs)
 bad_dates=['2025-12-27','2100-01-02','2026-11-20','2026-2-07','2026-02-30','2026-00-03','2026-13-05','2026-11-21T12:00:00','2026-11-21 ']
 return dict(version=1,core_contract_sha256=CORE,helper_contract_sha256=HELPER,dependency_freeze_sha256=INDEX['inputs/contracts/set47-contract-dependency-freeze.json']['sha256'],final_dependency_source_sha256=INDEX['inputs/contracts/set47-final-dependency-source.json']['sha256'],input_index_sha256=sha((HERE/'input_index.json').read_bytes()),candidate_only=True,new47_runtime_read=False,expected_values_from_Go=False,rules=dict(value=RULES,id=RULE_ID),ast_extractions=EXTRACTIONS,actual_source_context=dict(original_status=load('inputs/actual46/initial_status.json'),retained_conflict_status=load('inputs/actual46/conflict_status.json'),new_run_authorized_by_reference=False,bout_journal_times_are_modeled=True),compositions=fs,dtos=ds,codec_cases=cs,protocol_cases=ps,portrait_cases=portraits,calendar_cases=calendar_cases(),calendar_refusals=bad_dates,identity_cases=identities_cases(fs),state_cases=scenarios(fs[1]),initialization_cases=initialization())
if __name__=='__main__':
 obj=generate();(HERE/'bout_v1.json').write_bytes(pretty(obj));print(json.dumps({'compositions':len(obj['compositions']),'dtos':len(obj['dtos']),'codec_cases':len(obj['codec_cases']),'protocol_cases':len(obj['protocol_cases']),'sha256':sha((HERE/'bout_v1.json').read_bytes())}))
