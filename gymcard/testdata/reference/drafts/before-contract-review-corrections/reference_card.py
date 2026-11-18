"""Independent Set46 expected bytes; finite mutation recipes, no Go outputs."""
import argparse,itertools
from schema import *

def pointer(v,p):
 if p=='':return v
 for k in p.split('/')[1:]:
  k=k.replace('~1','/').replace('~0','~');v=v[int(k)] if type(v)is list else v[k]
 return v

def edit(v,p,op,value=None):
 if p=='':
  req(op=='set');return clone(value)
 parts=p.split('/');parent=pointer(v,'/'.join(parts[:-1]));k=parts[-1].replace('~1','/').replace('~0','~');k=int(k) if type(parent)is list else k
 if op=='set':parent[k]=clone(value)
 elif op=='delete':del parent[k]
 elif op=='append':parent[k].append(clone(value))
 elif op=='reverse':parent[k].reverse()
 else:raise AssertionError(op)
 return v

def reverse_objects(v):
 if type(v)is dict:return {k:reverse_objects(v[k]) for k in reversed(v)}
 if type(v)is list:return [reverse_objects(x) for x in v]
 return v

def materialize(base,recipe):
 v=clone(base)
 for x in recipe.get('edits',[]):v=edit(v,x['path'],x['op'],x.get('value'))
 style=recipe.get('style','canonical')
 if style=='reversed_whitespace':b=json.dumps(reverse_objects(v),indent=1,ensure_ascii=True).encode()
 else:b=J(v)
 if 'raw'in recipe:b=recipe['raw'].encode('ascii')
 if 'replace_once'in recipe:
  old,new=recipe['replace_once'];req(old.encode()in b);b=b.replace(old.encode(),new.encode(),1)
 if 'pad_bytes'in recipe:
  req(recipe['pad_bytes']>=len(b));b+=b' '*(recipe['pad_bytes']-len(b))
 return b

def snapshot(f,st='ready'):
 blank=st in ['setup_incomplete','unavailable'];return dict(version=1,state=st,error={'setup_incomplete':'card_setup_incomplete','unavailable':'card_unavailable','failed':'card_corrupt'}.get(st,''),view=None if st=='unavailable' else clone(f['current_view']),store_id='' if blank else STORE_ID,pin=None if blank else clone(PIN),source_created_at_ms=0 if blank else SOURCE_CREATED,created_at_ms=0 if blank else CREATED,active_job_id='',queued_job_id=label('job/'+f['fixture']) if st=='busy' else '',last_job_id='')
def job(f,st='queued',r=None):
 return dict(version=1,job_id=label('job/'+f['fixture']),kind='build',state=st,view=clone(f['current_view']),store_id=STORE_ID,card_id=card_id(E),saturday=E['saturday'],error={'failed':'outcome_unknown','cancelled':'card_cancelled'}.get(st,''),result=clone(r) if st=='completed' else None)
def request(cmd,f=None,saturday=E['saturday']):
 return dict(id=1,command='card_'+cmd,view_id='' if cmd=='snapshot' else f['current_view']['view_id'],saturday=saturday if cmd in ['status','record','build'] else '',after_card_id='',limit=8 if cmd=='history' else 0,job_id=label('job/'+f['fixture']) if cmd in ['job','cancel'] else '')
def response(slot,v,f):
 r=dict(id=1,ok=True,view=clone(f['current_view']),store_id=STORE_ID,snapshot=None,status=None,record=None,history=None,job=None,error='');r[slot]=clone(v)
 if slot=='snapshot' and v['state']in ['unavailable','setup_incomplete']:r['view']=clone(v['view']);r['store_id']=''
 return r

def matcher_cases():
 # Input rows are standalone synthetic matcher controls, not admitted records.
 full=clone(SOURCE);full['source_receipt']=clone(RECEIPTS[2]);base=source_inputs(full);original_base=clone(base);out=[]
 # The old structural five-class receipt repeats the same full IDs across gyms.
 # Distinct matcher IDs are openly synthetic, never passed as admitted receipts.
 for x in base:x['prospect_id']=label('synthetic prospect/'+x['gym_id']+'/'+x['prospect_id'])
 def add(name,inputs):
  c=label('matcher/card/'+name);i=label('matcher/input/'+name);s=label('matcher/seed/'+name)
  try:p=match(inputs,c,i,s);validate('Plan',p);accept=True
  except(ValueError,Refusal):p=None;accept=False
  out.append(dict(name=name,scope='synthetic_standalone_matcher_no_source_authority',inputs=inputs,card_id=c,input_id=i,seed=s,accept=accept,plan=p,canonical=J(p).decode() if p else None))
 add('empty',[]);add('old_five_class_duplicate_source_ids',original_base);add('five_classes_all_paired',base);add('one_gym_five',base[:5])
 # Both source participant lists are five rows in their source order, flattened.
 for ai,a in enumerate(['absent','eligible','retired','not_rostered']):
  for bi,b in enumerate(['absent','eligible','retired','not_rostered']):
   for ratio in ['equal','exact_double','outside']:
    xs=[]
    for n,e in enumerate([a,b]):
     if e=='absent':continue
     x=clone(next(x for x in base if x['gym_id']==GYMS[n][0] and x['class_']=='light'));x['cutoff_eligibility']=e;x['rating_hundredths']=10000 if n==0 or ratio=='equal' else 20000 if ratio=='exact_double' else 20001;xs.append(x)
    add('eligibility/%s/%s/%s'%(a,b,ratio),xs)
 # One combined card exposes all five unmatched reasons simultaneously.
 xs=clone(base)
 for x in xs:
  g=next(i for i,(gid,_) in enumerate(GYMS) if gid==x['gym_id']);c=x['class_']
  if c=='light' and g==0:x['cutoff_eligibility']='retired'
  if c=='welter' and g==0:x['cutoff_eligibility']='not_rostered'
  if c=='middle':x['rating_hundredths']=10000 if g==0 else 20001
 xs=[x for x in xs if not(x['class_']=='light_heavy' and x['gym_id']==GYMS[1][0])]
 add('all_reasons_and_one_pair',xs)
 # Sorted corner identity is independent of input sequence; Plan array order is strict.
 add('five_reversed_input_derivation',list(reversed(base)))
 for name,change in [('duplicate_id',lambda a:a[1].update(prospect_id=a[0]['prospect_id'])),('duplicate_gym_class',lambda a:a[1].update(class_=a[0]['class_'])),('foreign_gym',lambda a:a[0].update(gym_id=label('foreign'))),('eligible_zero',lambda a:a[0].update(rating_hundredths=0)),('bool_rating',lambda a:a[0].update(rating_hundredths=True)),('overrating',lambda a:a[0].update(rating_hundredths=100001))]:
  x=clone(base);change(x);add(name,x)
 return out

def band_cases():
 out=[]
 pairs=[(1,1),(1,2),(1,3),(2,3),(49999,100000),(50000,100000),(50001,100000),(100000,100000),(10000,19999),(10000,20000),(10000,20001)]
 pairs+= [(b,a) for a,b in pairs if a!=b]
 pairs += [(0,1),(-1,1),(True,1),(1.0,1),(1,100001),('1',1),(None,1)]
 for a,b in pairs:
  try:v=band(a,b);ok=True
  except ValueError:v=None;ok=False
  out.append(dict(a=a,b=b,valid=ok,paired=v,max_product=2*min(a,b) if ok else None))
 return out

def corpus():
 dtos=[]
 def dto(kind,name,value,context='core'):
  v=validate(kind,value,context);b=J(v);d=dict(kind=kind,name=name,context=context,value=v,canonical=b.decode(),sha256=sha(b));dtos.append(d);return len(dtos)-1
 ids={}
 def add(k,n,v,context='core'):i=dto(k,n,v,context);ids[k+'/'+n]=i;return i
 add('Rules','compiled',RULES);add('Pin','candidate',PIN)
 add('Info','empty',dict(version=1,store_id=STORE_ID,pin=PIN,source_created_at_ms=SOURCE_CREATED,created_at_ms=CREATED,event_count=0))
 add('Marker','candidate',dict(application='SLUG candidate cards',version=1,store_id=STORE_ID,pin=PIN))
 add('Init','prepared',dict(version=1,store_id=STORE_ID,pin=PIN,source_created_at_ms=SOURCE_CREATED))
 add('Metadata','empty',dict(version=1,store_id=STORE_ID,pin=PIN,source_created_at_ms=SOURCE_CREATED,created_at_ms=CREATED,clock_ms=CREATED,event_count=0))
 add('CardSource','retained_finalized',SOURCE);add('CardState','complete',state());add('CardState','unfinalized',state('prepared'));add('CardState','absent',state('absent_due_no_allocation'));add('CardState','conflicted',state('conflict'))
 r=record();plan=r['plan'];add('Record','retained_source_three_bouts',r);add('Plan','retained_source_three_bouts',plan)
 for k,v in [('ProfessionalRecord',plan['entrants'][0]['professional_record']),('ReasonCounts',plan['counts']['reasons']),('Counts',plan['counts']),('GymCounts',plan['gyms'][0]),('Entrant',plan['entrants'][0]),('Bout',plan['bouts'][0])]:add(k,'first',v)
 add('Status','existing',status(r));add('Status','absent',status(None));conf=status(r,state('conflict'));add('Status','historical_conflict',conf)
 off=state();off['status']['node_state']='offline';off['status']['current_finality']=False;add('CardState','offline_historical',off);add('Status','offline_usable',status(r,off))
 shallow=clone(off);shallow['status']['node_state']='online';shallow['status']['target_descendants']=1;shallow['status']['scan_tip_height']=4;add('CardState','shallow_historical',shallow);add('Status','shallow_usable',status(r,shallow))
 br=dict(version=1,record=r,status=status(r),replayed=False);rr=dict(version=1,record=r,status=status(r));add('BuildResult','inserted',br);replay=clone(br);replay['replayed']=True;replay['status']=status(r,state(now=CREATED+1),CREATED+1);add('BuildResult','replayed',replay)
 add('BuildResult','conflict_replay',dict(version=1,record=r,status=conf,replayed=True));add('ReadResult','existing',rr)
 hist=dict(version=1,store_id=STORE_ID,after_card_id='',next_card_id='',has_more=False,observed_at_ms=CREATED,cards=[status(r)]);add('HistoryPage','one',hist);h=clone(hist);h['cards']=[];add('HistoryPage','empty',h)
 mats=matcher_cases()
 for m in mats:
  if m['accept']:add('Plan','matcher/'+m['name'],m['plan'])
 for f in VIEWS:
  g=f['participant_id'];acc=GYM_ACCOUNT[g];add('CardView',f['fixture']+'/complete',dict(version=1,store_id=SEED_ID,pin=SEED_PIN,created_at_ms=SOURCE_CREATED,participant_account_id=acc,participant_gym_id=g,observed_at_ms=CREATED,states=[state()]))
  for st in ['ready','busy','setup_incomplete','unavailable','failed']:add('CardSnapshot',f['fixture']+'/'+st,snapshot(f,st),'transport')
  for st in ['queued','running','completed','failed','cancelled']:add('CardJob',f['fixture']+'/'+st,job(f,st,br),'transport')
  for cmd in ['snapshot','status','record','history','build','job','cancel']:
   q=request(cmd,f);add('CardRequest',f['fixture']+'/'+cmd,q,'transport')
   slot=cmd if cmd in ['snapshot','status','record','history'] else 'job';v={'snapshot':snapshot(f),'status':status(r),'record':rr,'history':hist,'build':job(f),'job':job(f,'completed',br),'cancel':job(f,'cancelled')}[cmd]
   add('CardResponse',f['fixture']+'/'+cmd,response(slot,v,f),'transport')
 # Canonical repeated terminal must preserve all observation evidence bytes.
 codecs=[]
 def codec(base,name,recipe,accept,context=None):
  b=dtos[base];ctx=context or b['context'];rawb=materialize(b['value'],recipe)
  try:v=parse(b['kind'],rawb,ctx);actual=True;canonical=J(v)
  except Refusal:actual=False;canonical=None
  assert actual==accept,(b['kind'],name,ctx,actual)
  x=dict(name=b['kind']+'/'+b['name']+'/'+name,kind=b['kind'],context=ctx,base=base,recipe=recipe,input_bytes=len(rawb),input_sha256=sha(rawb),accept=accept,canonical_sha256=sha(canonical) if canonical else None)
  if canonical is not None:x['canonical_ref']=base if canonical.decode()==b['canonical'] else None;x['canonical']=canonical.decode() if x['canonical_ref']is None else None
  codecs.append(x)
 # For every DTO, one canonical positive, nested order/whitespace positive and
 # first-level missing/unknown/duplicate/scalar refusal, finite and reproducible.
 for i,b in enumerate(dtos):
  codec(i,'canonical',{},True);codec(i,'ordered_whitespace',{'style':'reversed_whitespace'},True)
  key=next(iter(b['value']));codec(i,'missing_'+key,{'edits':[dict(op='delete',path='/'+key)]},False)
  codec(i,'unknown',{'edits':[dict(op='set',path='/unknown',value=0)]},False)
  text=J(b['value']).decode();dup='{'+J(key).decode()+':'+J(b['value'][key]).decode()+','+text[1:];codec(i,'duplicate_key',{'raw':dup},False)
  if 'version'in b['value']:
   for value in [True,0,2]:codec(i,'version_'+str(value),{'edits':[dict(op='set',path='/version',value=value)]},False)
   for name,token in [('exponent','1e0'),('float','1.0'),('negative_zero','-0')]:codec(i,name,{'replace_once':['"version":1','"version":'+token]},False)
 # Specific paired arithmetic, identity and array invariants.
 p=ids['Plan/retained_source_three_bouts'];rec=ids['Record/retained_source_three_bouts']
 mutations=[('entrants_reverse','/entrants','reverse',None),('bouts_reverse','/bouts','reverse',None),('gyms_reverse','/gyms','reverse',None),('nonzero_record','/entrants/0/professional_record/wins','set',1),('reason_wrong','/entrants/0/reason','set','outside_rating_band'),('counts_wrong','/counts/paired_count','set',4),('null_bouts','/bouts','set',None),('duplicate_id','/entrants/1/prospect_id','set',plan['entrants'][0]['prospect_id']),('corner_reversal','/bouts/0/corner_a','set',plan['bouts'][0]['corner_b']),('bad_seed','/bouts/0/seed','set',label('wrong'))]
 for name,path,op,value in mutations:codec(p,name,{'edits':[dict(op=op,path=path,value=value)]},False)
 codec(rec,'changed_derived_name_core',{'edits':[dict(op='set',path='/plan/entrants/0/display_name',value='Other Valid Name')]},False,'core')
 codec(rec,'changed_name_cheap_context',{'edits':[dict(op='set',path='/plan/entrants/0/display_name',value='Other Valid Name')]},True,'transport')
 codec(p,'local_name_context',{'edits':[dict(op='set',path='/entrants/0/display_name',value='Other Valid Name')]},True)
 for path,value in [('/source/target/height',4),('/source/finality/target_descendants',59),('/created_at_ms',C),('/source_digest',label('bad')),('/source/anchor/height',-1),('/source/finality/observed_at_ms',C),('/source/source_receipt/input/participants/0/members/0/slot_assigned_at_ms',False),('/source/declaration/signature','00'*64)]:
  codec(rec,'source_'+path.replace('/','_'),{'edits':[dict(op='set',path=path,value=value)]},False)
 cs=ids['CardState/absent']
 codec(cs,'signed_sentinels_positive',{},True)
 for path in ['/status/anchor_height','/status/target_height','/status/target_descendants']:
  codec(cs,'wrong_signed_'+path.split('/')[-1],{'edits':[dict(op='set',path=path,value=-1)]},False)
 for f in VIEWS:
  base=ids['CardSnapshot/'+f['fixture']+'/busy'];q=dtos[base]['value']['queued_job_id']
  for name,edits in [('both_active_queued',[dict(op='set',path='/active_job_id',value=label('other'))]),('same_last_queued',[dict(op='set',path='/last_job_id',value=q)]),('ready_with_queued',[dict(op='set',path='/state',value='ready')])]:codec(base,name,{'edits':edits},False)
 # LF belongs to helper frame budget; standalone core budget excludes LF.
 for k,n in [('CardRequest',VIEWS[0]['fixture']+'/snapshot'),('CardResponse',VIEWS[0]['fixture']+'/snapshot'),('Plan','retained_source_three_bouts')]:
  i=ids[k+'/'+n];cap=CAPS[k];lf=int(k in ['CardRequest','CardResponse']);codec(i,'exact_cap',{'pad_bytes':cap-lf},True);codec(i,'one_over_cap',{'pad_bytes':cap-lf+1},False)
 protocols=[]
 def pc(name,f,q,z,accept,expected=None,terminal=None):
  try:correlate(q,z,f['current_view'],expected,terminal);actual=True
  except(ValueError,TypeError,KeyError):actual=False
  assert actual==accept,(name,actual)
  protocols.append(dict(name=f['fixture']+'/'+name,fixture=f['fixture'],current_view=f['current_view'],authority_pin=PIN,source_created_at_ms=SOURCE_CREATED,created_at_ms=CREATED,request=q,response=z,expected_job=expected,terminal_digest=terminal,accept=accept))
 for f in VIEWS:
  for cmd in ['snapshot','status','record','history','build','job','cancel']:
   q=request(cmd,f);z=dtos[ids['CardResponse/'+f['fixture']+'/'+cmd]]['value'];pc(cmd+'/valid',f,q,z,True)
   bad=clone(z);bad['id']+=1;pc(cmd+'/wrong_id',f,q,bad,False)
   if bad['view']is not None:
    bad=clone(z);bad['view']['view_id']=label('stale');pc(cmd+'/wrong_view',f,q,bad,False)
  for st in ['setup_incomplete','unavailable','failed','busy']:pc('snapshot/'+st,f,request('snapshot',f),response('snapshot',snapshot(f,st),f),True)
  for st in ['running','completed','failed','cancelled']:pc('begin_nonqueued/'+st,f,request('build',f),response('job',job(f,st,br),f),False)
  jq=job(f);jr=job(f,'running');jc=job(f,'completed',br);qd=request('job',f)
  pc('queued_to_running',f,qd,response('job',jr,f),True,jq)
  pc('running_to_queued',f,qd,response('job',jq,f),False,jr)
  pc('skip_running',f,qd,response('job',jc,f),True,jq)
  pc('repeat_terminal',f,qd,response('job',jc,f),True,jc,sha(J(jc)))
  bad=clone(jc);bad['result']['replayed']=True;pc('terminal_replayed_changed',f,qd,response('job',bad,f),False,jc,sha(J(jc)))
  bad=clone(jc);bad['result']['status']['observed_at_ms']+=1;pc('terminal_observation_changed',f,qd,response('job',bad,f),False,jc,sha(J(jc)))
  pc('terminal_regression',f,qd,response('job',jq,f),False,jc,sha(J(jc)))
  bad=clone(jq);bad['saturday']='2026-11-28';bad['card_id']=card_id(event(bad['saturday']));pc('discovered_wrong_tuple',f,qd,response('job',bad,f),False,jq)
  for err in ERRORS:
   z=dict(id=1,ok=False,view=None,store_id='',snapshot=None,status=None,record=None,history=None,job=None,error=err);pc('error/'+err,f,request('build',f),z,True)
 # Full validated-source versus recipe-only identity controls.
 identities=[]
 def identity(name,s,context):
  p=derive(s);identities.append(dict(name=name,scope=context,source=s,source_digest=source_digest(s),card_id=p['card_id'],input_id=p['input_id'],seed=p['seed'],bout_ids=[b['bout_id'] for b in p['bouts']],bout_seeds=[b['seed'] for b in p['bouts']]))
 identity('original',SOURCE,'validated_retained_source')
 s=clone(SOURCE);s['finality']['observed_at_ms']+=1;validate('CardSource',s);identity('later_first_finality_observation',s,'validated_retained_source')
 s=clone(SOURCE);s['target']['observed_at_ms']+=1;validate('CardSource',s);identity('later_target_observation',s,'validated_retained_source')
 # Different source receipt evidence requires a new matching chain declaration.
 # These are only byte-preimage sensitivity controls, never accepted CardSource.
 s=clone(SOURCE);s['finality']['checkpoint']['signature']='11'*64;identity('certificate_bytes_only_recipe',s,'preimage_only_invalid_signature')
 s=clone(SOURCE);s['target']['hash']=label('alternate target');identity('different_target_recipe',s,'preimage_only_missing_matching_chain')
 preimages=dict(rules=RULE_PRE.hex(),store=STORE_PRE.hex(),card=card_pre(E).hex(),source=(domain('candidate card source')+lp(J(SOURCE))).hex(),input=input_pre(SOURCE).hex(),seed=(domain('candidate card seed')+raw(plan['input_id'])+raw(SOURCE['target']['hash'])).hex(),bouts=[dict(bout_id=b['bout_id'],preimage=bout_pre(plan['input_id'],b['class'],b['corner_a'],b['corner_b']).hex(),seed_preimage=(domain('candidate card bout seed')+raw(plan['seed'])+raw(b['bout_id'])).hex()) for b in plan['bouts']],record=(domain('candidate card record')+lp(J(r))).hex())
 from reference_state import scenarios,initialization,history_cases
 return dict(version=1,core_contract_sha256=CORE,helper_contract_sha256=HELPER,input_index_sha256=sha((HERE/'input_index.json').read_bytes()),candidate_only=True,rules=dict(value=RULES,id=RULE_ID),pin=dict(value=PIN,store_id=STORE_ID),views=VIEWS,source_provenance=dict(receipt='unchanged_six_genuine_admission_inputs_with_modeled_cutoff_history',block='unchanged_normal_work_wire_fixture_not_validating_node_chain',synthetic='five_class_matcher_controls_not_admitted_records',core_receipt_validator='finite_retained_exact_receipt_domain'),preimages=preimages,dtos=dtos,codec_cases=codecs,protocol_cases=protocols,matcher_cases=mats,band_cases=band_cases(),identity_cases=identities,calendar=calendar_rows(),scenarios=scenarios(),initialization=initialization(),history=history_cases(),bounds=load('inputs/contracts/set46-contract-bounds-formula.json'))

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--generate',action='store_true');a=ap.parse_args();c=corpus();b=pretty(c);p=HERE/'card_v1.json'
 if a.generate:p.write_bytes(b)
 else:assert p.read_bytes()==b,'frozen expected corpus differs'
 print('card reference',sha(b),'bytes',len(b),'DTOs',len(c['dtos']),'codec',len(c['codec_cases']),'protocol',len(c['protocol_cases']),'matcher',len(c['matcher_cases']),'calendar',len(c['calendar']))
if __name__=='__main__':main()
