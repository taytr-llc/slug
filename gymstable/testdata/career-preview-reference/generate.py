"""Independent Set50 compositions, finite codecs and full protocol envelopes."""
import argparse,contextlib,copy,datetime,json,pathlib,sys,time
from common import *
import schema as S

BASE=C.C;WEEK=604800000
CASE_SPECS=[('zero',0,[],0),('xp1-versus-zero',0,[('a','ko',-1)],0),('unequal-xp',0,[('a','decision',-3),('b','loss',-2),('b','decision',-1),('b','ko',0)],0),('cap15',0,[('a','ko',-i)for i in range(16)]+[('b','loss',-i)for i in range(15)],0),('standard-draw-xp',1,[('a','draw',-1)],0),('retained-after-conflict',1,[('a','decision',-1)],0),('before-equal-after',0,[('a','ko',-1),('b','draw',0),('a','loss',1)],0),('same-ms-prefix-first',0,[('a','decision',0)],0),('same-ms-prefix-extended',0,[('a','decision',0),('b','ko',0)],0),('retired-practice',1,[('a','ko',-1)],100*WEEK),('other-gym-zero',1,[],0),('other-gym-cap',1,[('b','decision',-i)for i in range(16)],0),('unrelated-event-prefix',0,[('unrelated','ko',-1)],0)]
def make_event(a,n,t,pid,gym,account,kind):
 opponent=C.ident(70000+n);win='draw'if kind=='draw'else'b'if kind=='loss'else'a';method='decision_draw'if kind=='draw'else'decision'if kind=='loss'else kind
 e=C.event(a,n,t,int(pid,16),int(opponent,16),win,method);e['corner_a']=dict(prospect_id=pid,account_id=account,gym_id=gym);e['corner_b']=dict(prospect_id=opponent,account_id=C.ident(80000+n),gym_id=C.ident(90000+n))
 # Independent agreement with the separately frozen standard arithmetic producer.
 deltas=F.award(win,method,'standard');req=[C.delta(e,k)for k in ['corner_a','corner_b']];assert deltas==req
 return e

def compose(name,pi,event_specs,retired):
 old=OLD['cases'][pi*2];status=clone(old['status']);fighters=clone(old['response']['job']['result']['pair']['fighters']);born=BASE-4*WEEK;asof=BASE+retired
 # Synthetic assignments retain declared original birth throughout this composition.
 for f in fighters:f['assignment'].update(born_at_ms=born,accepted_at_ms=born+1)
 aid=sha(b'SLUG modeled Set50 result owner\0'+bytes([pi]));act,actpre=C.activation(aid,BASE-20000,BASE-30000,BASE-10000)
 ids=[f['assignment']['prospect_id']for f in fighters];events=[]
 for n,(who,kind,delta)in enumerate(event_specs,1):events.append(make_event(act,n,asof+delta,(ids[0 if who=='a'else 1]if who!='unrelated'else C.ident(60000+n)),status['pin']['identity']['gym_id'],status['pin']['account_id'],kind))
 clock=max([asof]+[e['effective_at_ms']for e in events]);snap=C.snapshot(act,events,ids,asof,'current',clock);assert snap['error']==''
 status['last_as_of_ms']=asof;parts=[]
 for i,f in enumerate(fighters):
  a=f['assignment'];m=f['manifest'];tot=snap['totals'][i]['totals'];condition=B.CONDITION.project(1,m['dna'],born,asof,tot['xp']);parts.append(dict(assignment=a,manifest=m,totals=tot,condition=condition))
 pair=dict(version=1,status=status,activation=act,career_digest=snap['digest'],as_of_ms=asof,fighters=parts);r=recipe(pair);pre=SEED_DOMAIN+lp(J(r));seedvalue=sha(pre);inp=input_for(pair,seedvalue);engine=B.simulate(inp);result=dict(version=1,pair=pair,recipe=r,seed=seedvalue,engine_result=engine,engine_result_digest=B.digest(engine))
 owner=dict(activation=act,events=events,clock_ms=clock);S.validate('CareerPreviewResult',clone(result),owner)
 reversed_input=B.new_input(asof,seedvalue,inp['b'],inp['a']);assert B.canonical(reversed_input)==B.canonical(inp)
 conditions=[dict(prospect_id=f['assignment']['prospect_id'],canonical=J(f['condition']).decode(),preimage=(COND_DOMAIN+lp(J(f['condition']))).hex(),digest=condition_digest(f['condition']))for f in parts]
 # Portraits use the old independent typed renderer; no browser or native renderer is executed.
 portraits=[]
 for f in parts:
  svg=P.render(B.MANIFEST.canonical(f['manifest'])).decode();assert sha(P.DOMAIN+svg.encode())==f['assignment']['portrait_digest'];portraits.append(dict(prospect_id=f['assignment']['prospect_id'],svg=svg,portrait_digest=f['assignment']['portrait_digest']))
 return dict(name=name,provenance='MODELED authority/index/events and synthetic Preview-v1 assignments; no admission, signature, finality or live session claim',fixture=old['name'].split('-')[0],owner=owner,activation_preimage=actpre.hex(),current_snapshot=snap,pair=pair,canonical_pair=J(pair).decode(),conditions=conditions,recipe=r,canonical_recipe=J(r).decode(),seed_preimage=pre.hex(),seed=seedvalue,input=inp,canonical_input=B.canonical(inp).decode(),input_id=B.input_id(inp),result=result,canonical_result=J(result).decode(),engine_result_digest=B.digest(engine),captured_result_digest=sha(J(result)),reversed_selection_same_input=True,portrait_evidence=portraits,source_conflict_retains_award=(name=='retained-after-conflict'))

def surfaces(c,index):
 pi=1 if c['fixture']=='incrementing256'else 0;v=clone(OLD['cases'][pi*2]['view']);v['view_id']=sha(b'SLUG modeled Set50 view\0'+bytes([pi]));p=c['pair'];ids=[f['assignment']['prospect_id']for f in p['fighters']]
 done=dict(version=1,job_id=sha(b'SLUG modeled Set50 job\0'+c['name'].encode()),state='done',view=v,store_id=p['status']['store_id'],engine_version=1,engine_id=B.ENGINE,activation_id=p['activation']['activation_id'],fighter_a_id=ids[0],fighter_b_id=ids[1],error='',result=c['result'])
 snapshot=dict(version=1,state='ready',error='',view=v,store_id=done['store_id'],engine_version=1,engine_id=B.ENGINE,activation=p['activation'],active_job_id='',queued_job_id='',last_job_id=done['job_id'])
 req=dict(id=index+1,version=1,command='career_preview_job',view_id=v['view_id'],job_id=done['job_id']);response=dict(id=index+1,version=1,ok=True,job=done)
 return v,done,snapshot,req,response

def dtos(compositions):
 out=[]
 def add(kind,name,value):
  v=S.validate(kind,clone(value));out.append(dict(kind=kind,name=name,value=v,canonical=J(v).decode(),sha256=sha(J(v))))
 for i,c in enumerate(compositions):
  v,j,s,q,r=surfaces(c,i)
  for kind,value in [('CareerPreviewPair',c['pair']),('CareerPreviewResult',c['result']),('CareerPreviewJob',j),('CareerPreviewSnapshot',s),('CareerPreviewRequest',q),('CareerPreviewResponse',r)]:add(kind,c['name'],value)
  for state in ['queued','running','failed','cancelled']:
   a=clone(j);a.update(state=state,result=None,error=''if state in ['queued','running']else'preview_cancelled'if state=='cancelled'else'career_inactive');add('CareerPreviewJob',c['name']+'/'+state,a)
  a=clone(s);a.update(state='busy',active_job_id=j['job_id'],last_job_id='');add('CareerPreviewSnapshot',c['name']+'/busy',a)
  a=clone(s);a.update(state='idle',last_job_id='');add('CareerPreviewSnapshot',c['name']+'/idle',a)
  for cmd,fields in [('career_preview_snapshot',{}),('career_preview_begin',dict(view_id=v['view_id'],fighter_a_id=j['fighter_a_id'],fighter_b_id=j['fighter_b_id'])),('career_preview_cancel',dict(view_id=v['view_id'],job_id=j['job_id']))]:add('CareerPreviewRequest',c['name']+'/'+cmd,dict(id=i+1,version=1,command=cmd,**fields))
  for im in c['portrait_evidence']:
   f=next(x for x in c['pair']['fighters']if x['assignment']['prospect_id']==im['prospect_id']);a=f['assignment'];pr=dict(version=1,view=v,store_id=j['store_id'],job_id=j['job_id'],prospect_id=a['prospect_id'],activation_id=j['activation_id'],career_digest=pval(c,'career_digest'),result_digest=c['engine_result_digest'],descriptor_id=a['descriptor_id'],manifest_digest=a['manifest_digest'],portrait_digest=a['portrait_digest'],svg=im['svg']);add('CareerPreviewPortraitResponse',c['name']+'/'+a['prospect_id'],dict(version=1,ok=True,portrait=pr))
 # Structural boundary only: this parsed public Pair does not assert a matching trusted owner prefix.
 boundary=clone(compositions[0]['pair']);f=boundary['fighters'][0];f['totals']=dict(wins=0,losses=256,draws=0,xp=15,base_points=0);f['condition']=B.CONDITION.project(1,f['manifest']['dna'],f['assignment']['born_at_ms'],boundary['as_of_ms'],15);add('CareerPreviewPair','structural-total-256-no-authority-claim',boundary)
 for code in S.UNAVAILABLE:
  add('CareerPreviewSnapshot','unavailable/'+code,dict(version=1,state='unavailable',error=code,view=None,store_id='',engine_version=0,engine_id='',activation=None,active_job_id='',queued_job_id='',last_job_id=''))
 for code in S.ERRORS:add('CareerPreviewResponse','refusal/'+code,dict(id=1,version=1,ok=False,error=code))
 return out

def pval(c,k):return c['pair'][k]
def pathset(v,path,value,delete=False):
 at=v
 for k in path[:-1]:at=at[k]
 if delete:del at[path[-1]]
 else:at[path[-1]]=value

def materialize(case,values):
 b=case['base'];v=clone(values[b]['value']);rec=case['recipe']
 if 'set'in rec:pathset(v,rec['path'],rec['set'])
 for op in rec.get('mutations',[]):pathset(v,op['path'],op['set'])
 if rec.get('recompute_scores'):v['engine_result']['scores']=[B.scores(x)for x in v['engine_result']['final']]
 if rec.get('rehash_engine'):v['engine_result_digest']=B.digest(v['engine_result'])
 if rec.get('delete'):pathset(v,rec['path'],None,True)
 if 'add'in rec:v[rec['add']]=rec['value']
 if rec.get('reverse_keys'):
  def rev(x):return {k:rev(x[k])for k in reversed(x)}if type(x)is dict else[rev(y)for y in x]if type(x)is list else x
  v=rev(v)
 raw=J(v)
 if rec.get('pretty'):raw=json.dumps(v,indent=1).encode()
 if 'replace_once'in rec:
  a,z=rec['replace_once'];assert a.encode()in raw;raw=raw.replace(a.encode(),z.encode(),1)
 if 'raw_hex'in rec:raw=bytes.fromhex(rec['raw_hex'])
 if 'append'in rec:raw+=rec['append'].encode()
 if 'pad_to'in rec:raw+=b' '*(rec['pad_to']-len(raw))
 return raw

def codec_cases(values):
 out=[]
 def add(name,b,recipe,accept):
  c=dict(name=name,base=b,recipe=recipe,accept=accept);raw=materialize(c,values);c.update(input_sha256=sha(raw),input_bytes=len(raw))
  try:v=S.parse(values[b]['kind'],raw);actual=True;canonical=J(v).decode()
  except(S.Refusal,ValueError,TypeError,KeyError,OverflowError,IndexError,RecursionError):actual=False;canonical=None
  assert actual==accept,(name,actual,accept)
  if accept:c['canonical_sha256']=sha(canonical.encode());c['canonical_ref']=b
  out.append(c)
 bases={kind:next(i for i,v in enumerate(values)if v['kind']==kind)for kind in S.VALIDATORS}
 for kind,b in bases.items():
  for name,recipe in [('canonical',{}),('reordered',{'reverse_keys':True}),('whitespace',{'pretty':True})]:add(kind+'/'+name,b,recipe,True)
  if kind in ['CareerPreviewRequest','CareerPreviewResponse']:
   add(kind+'/exact-frame-limit',b,dict(pad_to=S.CAPS[kind]-1),True);add(kind+'/LF-over-frame-limit',b,dict(pad_to=S.CAPS[kind]),False)
  add(kind+'/extra',b,dict(add='extra',value=0),False);add(kind+'/duplicate',b,dict(replace_once=['"version":1','"version":1,"version":1']),False);add(kind+'/negative-zero',b,dict(replace_once=['"version":1','"version":-0']),False);add(kind+'/bool-version',b,dict(path=['version'],set=True),False);add(kind+'/exponent',b,dict(replace_once=['"version":1','"version":1e0']),False);add(kind+'/trailing-token',b,dict(append='{}'),False);add(kind+'/invalid-utf8',b,dict(raw_hex='ff'),False);add(kind+'/oversize',b,dict(pad_to=S.CAPS[kind]+1),False);add(kind+'/deep-root',b,dict(raw_hex=(b'['*13+b'0'+b']'*13).hex()),False)
  for key in values[b]['value']:add(kind+'/missing-'+key,b,dict(path=[key],delete=True),False)
 b=bases['CareerPreviewResult'];v=values[b]['value'];p=v['pair'];badtotal=clone(p['fighters'][0]['totals']);badtotal['xp']=16
 for label,path,value in [('pair-version-bool',['pair','version'],True),('fighters-extra',['pair','fighters'],p['fighters']+[p['fighters'][0]]),('slots-extra',['pair','status','slots'],['']*11),('xp16',['pair','fighters',0,'totals'],badtotal),('condition-xp',['pair','fighters',0,'condition','pro_xp'],15),('condition-birth',['pair','fighters',0,'condition','born_at_ms'],p['as_of_ms']),('activation-id',['pair','activation','activation_id'],'f'*64),('career-digest-unbound',['pair','career_digest'],'f'*64),('recipe-total-unbound',['recipe','fighters',0,'totals','wins'],1),('training',['recipe','training'],[0,0,0]),('seed',['seed'],'f'*64),('input-id',['engine_result','input_id'],'f'*64),('result-digest',['engine_result_digest'],'f'*64),('engine-version-bool',['engine_result','version'],True),('rounds-extra',['engine_result','rounds'],v['engine_result']['rounds']*4),('round-negative',['engine_result','end_round'],-1),('tick-array-extra',['engine_result','rounds',0,'ticks'],v['engine_result']['rounds'][0]['ticks']*61),('form-outside',['engine_result','initial',0,'form_q'],-100001),('bool-count',['engine_result','draw_counts','combat'],True),('fighter-reversal',['pair','fighters'],list(reversed(p['fighters']))),('wrong-source-gym',['pair','fighters',0,'assignment','gym_id'],'f'*64)]:add('semantic/'+label,b,dict(path=path,set=value),False)
 # Recomputed outer digests isolate inherited transcript predicates, rather than a stale digest.
 for label,path,value in [('effective-power-low',['initial',0,'effective','power'],719999),('effective-speed-low',['initial',0,'effective','speed'],539999),('effective-chin-low',['initial',0,'effective','chin'],629999),('effective-stamina-low',['initial',0,'effective','stamina'],629999),('effective-skill-low',['initial',0,'effective','skill'],809999),('required-recovery-absent',['rounds',0,'recovery'],None),('initial-signed-overflow',['initial',0,'form_q'],2147483648),('missing-final-stoppage',['rounds',len(v['engine_result']['rounds'])-1,'ticks',len(v['engine_result']['rounds'][-1]['ticks'])-1,'significant'],False)]:
  add('isolated-engine/'+label,b,dict(path=['engine_result']+path,set=value,rehash_engine=True),False)
 add('isolated-engine/knockdown-not-significant',b,dict(mutations=[dict(path=['engine_result','rounds',0,'ticks',0,'hit'],set=True),dict(path=['engine_result','rounds',0,'ticks',0,'knockdown'],set=True),dict(path=['engine_result','rounds',0,'ticks',0,'significant'],set=False)],rehash_engine=True),False)
 ri=len(v['engine_result']['rounds'])-1;ti=len(v['engine_result']['rounds'][-1]['ticks'])-1
 add('isolated-engine/score-rounds-to-forbidden-27000',b,dict(mutations=[dict(path=['engine_result']+path+['damage_dealt_q'],set=2699999999)for path in [['rounds',ri,'ticks',ti,'states',0],['rounds',ri,'end',0],['final',0]]],recompute_scores=True,rehash_engine=True),False)
 add('assignment/accepted-at-birth-window-edge',b,dict(path=['pair','fighters',0,'assignment','accepted_at_ms'],set=p['fighters'][0]['assignment']['born_at_ms']+300000),False)
 add('assignment/accepted-before-birth',b,dict(path=['pair','fighters',0,'assignment','accepted_at_ms'],set=p['fighters'][0]['assignment']['born_at_ms']-1),False)
 for suffix,path,value in [('wins-width',['wins'],1<<32),('xp-bool',['xp'],True),('sum-over-256',['losses'],257),('impossible-points',['base_points'],1)]:
  add('totals/'+suffix,b,dict(path=['pair','fighters',0,'totals']+path,set=value),False)
 # Old and new public types must not cross-decode; no legacy expected bytes are changed.
 oldresult=OLD['cases'][0]['response']['job']['result'];add('cross-type/legacy-result',b,dict(raw_hex=J(oldresult).hex()),False)
 oldpair=oldresult['pair'];add('cross-type/legacy-pair',bases['CareerPreviewPair'],dict(raw_hex=J(oldpair).hex()),False)
 for state in ['idle','busy','ready','failed','cancelled']:
  sb=bases['CareerPreviewSnapshot'];base=clone(values[sb]['value']);base.update(state=state,active_job_id='',queued_job_id='',last_job_id='',error='')
  # Every state has one isolated impossible projection.
  if state=='idle':base['last_job_id']='f'*64
  elif state=='busy':pass
  elif state=='ready':pass
  elif state=='failed':base['last_job_id']='f'*64
  else:base.update(last_job_id='f'*64,error='career_inactive')
  add('snapshot/state-'+state,sb,dict(raw_hex=J(base).hex()),False)
 sb=bases['CareerPreviewSnapshot'];add('snapshot/unknown-unavailable-error',sb,dict(raw_hex=J(dict(version=1,state='unavailable',error='busy',view=None,store_id='',engine_version=0,engine_id='',activation=None,active_job_id='',queued_job_id='',last_job_id='')).hex()),False)
 jb=next(i for i,r in enumerate(values)if r['kind']=='CareerPreviewJob'and r['value']['state']=='cancelled');add('job/cancelled-wrong-error',jb,dict(path=['error'],set='career_cancelled'),False)
 boundary=next(i for i,r in enumerate(values)if r['name']=='structural-total-256-no-authority-claim');add('totals/exact-256',boundary,{},True);add('totals/exact-257-with-coherent-XP15',boundary,dict(path=['fighters',0,'totals','losses'],set=257),False)
 # Deep duplicate escaped alias, not merely a raw spelling collision.
 add('semantic/escaped-duplicate',b,dict(replace_once=['"career_digest":','"career\\u005fdigest":"'+p['career_digest']+'","career_digest":']),False)
 return out

def protocol_cases(compositions):
 out=[]
 def add(name,q,r,current,authority,known=None,terminal=None,accept=True):
  row=dict(name=name,request=q,response=r,current_view=current,authority=authority,known=known,terminal_digest=terminal,accept=accept)
  try:S.correlate(clone(q),clone(r),clone(current),clone(authority),clone(known),terminal);actual=True
  except(S.Refusal,ValueError,TypeError,KeyError,OverflowError,IndexError):actual=False
  assert actual==accept,(name,actual,accept);out.append(row)
 for i,c in enumerate([compositions[1],compositions[4]]):
  v,j,s,q,r=surfaces(c,i);auth=dict(store_id=j['store_id'],activation=c['pair']['activation']);queued=clone(j);queued.update(state='queued',result=None);running=clone(queued);running['state']='running';begin=dict(id=q['id'],version=1,command='career_preview_begin',view_id=v['view_id'],fighter_a_id=j['fighter_a_id'],fighter_b_id=j['fighter_b_id']);prefix=c['name']+'/'
  add(prefix+'begin-queued',begin,dict(id=q['id'],version=1,ok=True,job=queued),v,auth)
  add(prefix+'begin-done-refused',begin,r,v,auth,accept=False)
  add(prefix+'running-done',q,r,v,auth,running)
  add(prefix+'snapshot-recovery',q,r,v,auth,{'snapshot':s})
  add(prefix+'unsolicited-without-correlation',q,r,v,auth,accept=False)
  add(prefix+'terminal-replay',q,r,v,auth,j,sha(J(j)))
  add(prefix+'terminal-derived-commitment',q,r,v,auth,j)
  add(prefix+'terminal-regresses-running',q,dict(id=q['id'],version=1,ok=True,job=running),v,auth,j,accept=False)
  changed=clone(r);changed['job']['result']['pair']['status']['observed_source_count']=3;changed['job']['result']['pair']['status']['caught_up']=False
  add(prefix+'terminal-status-replacement',q,changed,v,auth,j,sha(J(j)),False)
  add(prefix+'terminal-status-replacement-without-separate-digest',q,changed,v,auth,j,accept=False)
  add(prefix+'running-queued',q,dict(id=q['id'],version=1,ok=True,job=queued),v,auth,running,accept=False)
  for label,rv,cv,av in [('response-id',dict(r,id=r['id']+1),v,auth),('stale-view',r,dict(v,generation=v['generation']+1),auth),('wrong-store',r,v,dict(auth,store_id='f'*64)),('wrong-activation',r,v,dict(auth,activation=C.activation('f'*64,BASE-20000,BASE-30000,BASE-10000)[0]))]:add(prefix+label,q,rv,cv,av,running,accept=False)
  snapreq=dict(id=q['id'],version=1,command='career_preview_snapshot');snapresp=dict(id=q['id'],version=1,ok=True,snapshot=s);add(prefix+'snapshot-pinned',snapreq,snapresp,v,auth);add(prefix+'snapshot-no-authority',snapreq,snapresp,None,auth,accept=False)
 unavailable=dict(version=1,state='unavailable',error='career_inactive',view=None,store_id='',engine_version=0,engine_id='',activation=None,active_job_id='',queued_job_id='',last_job_id='')
 add('bootstrap-unavailable',dict(id=1,version=1,command='career_preview_snapshot'),dict(id=1,version=1,ok=True,snapshot=unavailable),None,None)
 return out

def owner_cases(compositions):
 out=[];c=compositions[6];p=c['pair'];owner=c['owner']
 for name,mut,accept in [('original-prefix',None,True),('different-requested-prefix','ids',False),('future-event-included','future',False),('same-ms-later-prefix','later',False),('wrong-activation','activation',False),('insufficient-owner-clock','clock',False)]:
  value=clone(p);o=clone(owner)
  if mut=='ids':value['career_digest']=C.snapshot(o['activation'],o['events'],[p['fighters'][0]['assignment']['prospect_id']],p['as_of_ms'],'current',o['clock_ms'])['digest']
  elif mut=='future':value['career_digest']=C.snapshot(o['activation'],o['events'],[f['assignment']['prospect_id']for f in p['fighters']],p['as_of_ms']+1,'current',o['clock_ms'])['digest']
  elif mut=='later':o['events'].append(make_event(o['activation'],99,p['as_of_ms'],p['fighters'][0]['assignment']['prospect_id'],p['status']['pin']['identity']['gym_id'],p['status']['pin']['account_id'],'decision'))
  elif mut=='activation':o['activation']=C.activation('f'*64,BASE-20000,BASE-30000,BASE-10000)[0]
  elif mut=='clock':o['clock_ms']=p['as_of_ms']-1
  try:S.validate('CareerPreviewPair',clone(value),o);actual=True
  except(S.Refusal,ValueError,AssertionError,KeyError):actual=False
  assert actual==accept,(name,actual,accept);out.append(dict(name=name,pair=value,modeled_owner=o,accept=accept,context='Exact captured authority snapshot comparison, not a later-current-owner refresh of an existing private token'))
 # Explicit double-XP input cannot substitute for the exact recipe-derived input.
 c=compositions[1];correct=c['input'];bad=clone(correct);bad['a']['pro_xp']*=2
 assert bad['a']['pro_xp']!=correct['a']['pro_xp'] and B.input_id(bad)!=c['input_id']
 out.append(dict(name='double-xp-input',modeled_only=True,correct_input_id=c['input_id'],wrong_input=bad,wrong_input_id=B.input_id(bad),accept=False,predicate='InputID must equal original-manifest/original-birth/Totals.XP exactly once'))
 return out

def portrait_protocol_cases(compositions,values):
 out=[]
 for c in [compositions[1],compositions[4]]:
  v,j,ss,q,r=surfaces(c,0);im=c['portrait_evidence'][0];resp=next(clone(x['value'])for x in values if x['kind']=='CareerPreviewPortraitResponse'and x['name']==c['name']+'/'+im['prospect_id']);known=dict(captured_result_digest=c['captured_result_digest'],svg=im['svg'],**{k:clone(j[k])for k in ['job_id','view','store_id','activation_id','fighter_a_id','fighter_b_id']})
  def add(name,response=resp,current=v,committed=known,fresh=j,accept=True):
   try:S.correlate_portrait(clone(response),clone(current),clone(committed),clone(fresh));actual=True
   except(S.Refusal,ValueError,KeyError,TypeError):actual=False
   assert actual==accept,(name,actual,accept);out.append(dict(name=c['name']+'/'+name,response=response,current_view=current,known=committed,fresh_job=fresh,accept=accept))
  add('render-fresh');add('cache-hit-fresh');add('stale-view',current=dict(v,generation=v['generation']+1),accept=False);add('lost-whole-result',committed=dict(known,captured_result_digest='f'*64),accept=False)
  for key in ['job_id','store_id','activation_id','career_digest','result_digest','prospect_id','descriptor_id','manifest_digest','portrait_digest']:
   changed=clone(resp);changed['portrait'][key]='f'*64;add('wrong-'+key,response=changed,accept=False)
  changed=clone(resp);changed['portrait']['svg']+=' ';changed['portrait']['portrait_digest']=sha(P.DOMAIN+changed['portrait']['svg'].encode());add('rehashed-foreign-svg',response=changed,accept=False)
  changed=clone(j);changed.update(state='running',result=None);add('not-terminal',fresh=changed,accept=False)
  changed=clone(j);changed['job_id']='e'*64;reply=clone(resp);reply['portrait']['job_id']='e'*64;add('same-result-different-selected-job',fresh=changed,response=reply,accept=False)
  missing=clone(known);del missing['job_id'];add('missing-selected-job',committed=missing,accept=False)
 return out

def build():
 compositions=[compose(*spec)for spec in CASE_SPECS];values=dtos(compositions)
 # Expected transcript is derived by frozen B.simulate; owner checks above validate the prefix, not a second engine run.
 # Every transport path below runs under engine, file/source-read, process/Argon and network traps.
 import builtins,os,socket,subprocess
 saved=[]
 def forbidden(*a,**kw):raise AssertionError('transport invoked forbidden engine/source/process/network work')
 for obj,key in [(B,'simulate'),(B,'parse_result'),(builtins,'open'),(pathlib.Path,'open'),(pathlib.Path,'read_bytes'),(pathlib.Path,'read_text'),(socket,'socket'),(socket,'create_connection'),(subprocess,'Popen'),(os,'system')]:
  saved.append((obj,key,getattr(obj,key)));setattr(obj,key,forbidden)
 try:
  for row in values:assert J(S.parse(row['kind'],row['canonical'].encode()))==row['canonical'].encode()
  codecs=codec_cases(values);protocols=protocol_cases(compositions);portraits=portrait_protocol_cases(compositions,values);owners=owner_cases(compositions)
 finally:
  for obj,key,old in reversed(saved):setattr(obj,key,old)
 # The whole bounded prefix contributes even when no selected totals change.
 zero,unrelated=compositions[0],compositions[-1]
 assert [f['totals']for f in zero['pair']['fighters']]==[f['totals']for f in unrelated['pair']['fighters']]
 assert zero['pair']['career_digest']!=unrelated['pair']['career_digest'] and zero['seed']!=unrelated['seed']
 from state_models import cases as state_cases
 return dict(version=1,scope='Independent modeled CareerPreview reference; no actual authorization/admission/finality, runtime or GUI acceptance',contract_sha256=INDEX['contract_sha256'],input_index_sha256=sha((ROOT/'input_index.json').read_bytes()),compositions=compositions,dtos=values,codec_cases=codecs,protocol_cases=protocols,portrait_protocol_cases=portraits,owner_cases=owners,state_cases=state_cases(),transport_simulation_trap_passed=True,transport_forbidden_work_traps=['simulate','parse_result','file/source reads','process/Argon invocation','network/RPC'],unrelated_event_changes_prefix_seed_only=True)

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--write',type=pathlib.Path);ap.add_argument('--check',type=pathlib.Path);a=ap.parse_args();start=time.monotonic();value=build();raw=J(value)+b'\n'
 if a.write:
  with a.write.open('xb')as f:f.write(raw)
 if a.check:assert a.check.read_bytes()==raw,'complete reference bytes differ'
 print(json.dumps(dict(status='passed',bytes=len(raw),sha256=sha(raw),compositions=len(value['compositions']),dtos=len(value['dtos']),codecs=len(value['codec_cases']),protocols=len(value['protocol_cases']),portraits=len(value['portrait_protocol_cases']),owners=len(value['owner_cases']),states=len(value['state_cases']),seconds=round(time.monotonic()-start,3))))
if __name__=='__main__':main()
