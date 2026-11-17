#!/usr/bin/env python3
"""Freeze independently generated full bouts and labelled primitive boundaries."""
import argparse,collections,copy,itertools,json,pathlib,time
import reference_bout as B
ROOT=pathlib.Path(__file__).resolve().parent

def check_accounting(inp,out):
 sums=[dict(thrown=0,landed=0,significant=0,damage_dealt_q=0,knockdowns=0,control_q=0) for _ in (0,1)]
 for ri,rd in enumerate(out['rounds'],1):
  assert rd['number']==ri
  for ti,t in enumerate(rd['ticks'],1):
   assert t['number']==ti
   s=sums[t['actor']];s['thrown']+=1
   if t['hit']:
    s['landed']+=1;s['significant']+=int(t['significant']);s['damage_dealt_q']+=t['damage_q']
    if t['strike']=='jab':s['control_q']+=500000
   else:s['control_q']+=500000;assert t['damage_q']==0 and not t['significant'] and not t['knockdown']
   if t['knockdown']:s['knockdowns']+=1;s['damage_dealt_q']+=6000000;s['control_q']+=3000000
   for j in (0,1):
    assert all(t['states'][j][k]==v for k,v in sums[j].items())
    assert 0<=t['states'][j]['stamina_q']<=out['initial'][j]['max_stamina_q']
  assert rd['end']==rd['ticks'][-1]['states']
  if rd['recovery'] is not None:assert ri<3 and len(rd['ticks'])==60 and not rd['ticks'][-1]['stoppage']
 assert out['final']==(out['rounds'][-1]['recovery']['states'] if out['rounds'][-1]['recovery'] else out['rounds'][-1]['end'])
 for s,score in zip(out['final'],out['scores']):assert score['total_tenths']==sum(v for k,v in score.items() if k!='total_tenths') and score==B.scores(s)
 assert out['input_id']==B.input_id(inp) and sum(out['draw_counts'].values())<=65536
 if out['method']=='decision_draw':assert out['scores'][0]['total_tenths']==out['scores'][1]['total_tenths'] and not out['winner']
 if out['method'] not in ('decision_draw','double_corner_stoppage'):assert out['winner'] in (inp['a']['prospect_id'],inp['b']['prospect_id'])

def primitive_cases():
 rounding=[]
 for den in (1,2,3,10,100,100000,1000000):
  for n in (-7*den-1,-5*den//2,-den//2,-1,0,1,den//2,3*den//2,7*den+1):
   # Independent rational oracle using Decimal for signed ties.
   from decimal import Decimal,localcontext,ROUND_HALF_EVEN
   with localcontext() as c:c.prec=80;expected=int((Decimal(n)/Decimal(den)).to_integral_value(rounding=ROUND_HALF_EVEN))
   assert B.R(n,den)==expected
   rounding.append({'numerator':n,'denominator':den,'expected':expected})
 limits=[]
 for total in (65535,65536):
  rng=B.Streams('00'*32);rng.total=total;rng.counts['combat']=total
  try:word=rng.word('combat');error=False
  except ValueError:word=0;error=True
  limits.append({'total_before':total,'combat_before':total,'expected_error':error,'word_decimal':str(word),'total_after':rng.total,'combat_after':rng.counts['combat']})
 streams=[]
 for label in B.LABELS:
  for counter in (0,1,255,65535):
   msg=B.stream_message('00'*32,label,counter);d=B.sha(msg)
   streams.append({'input_id':'00'*32,'label':label,'counter':counter,'message_hex':msg.hex(),'sha256':d,'word_decimal':str(int(d[:16],16))})
 uniforms=[]
 for n,words in [(1,[0]),(3,[0,1]),(100,[0,15,16]),(1000000,[0,(1<<64)-1]),((1<<64)-1,[0,1]),(3,[0]*256)]:
  seq=iter(words);count=[0]
  def word():count[0]+=1;return next(seq)
  try:value=B.uniform_words(n,word);err=False
  except ValueError:value=0;err=True
  uniforms.append({'bound_decimal':str(n),'words_decimal':[str(v) for v in words],'expected_error':err,'expected':value,'consumed':count[0]})
 init=[]
 for values in itertools.product((8,1000),(6,1000),(7,1000),(7,1000),(9,1000)):
  aged=dict(zip(B.ATTRS,values))
  for xp,tr,form,jit in itertools.product((0,15),(0,5),(-100000,0,100000),(920000,1000000,1080000)):
   init.append({'aged_tenths':aged.copy(),'pro_xp':xp,'training_skill':tr,'form_q':form,'jitter_q':jit,'expected':B.initial(aged,xp,tr,form,jit)})
 assert len(init)==1152
 base=B.initial(dict.fromkeys(B.ATTRS,500),0,0,0,B.Q);man={'style':'slugger','physical':{'height_in':70,'reach_in':70,'weight_lb':180,'stance':'orthodox'}}
 ticks=[]
 specifications=[('miss',[0,80,999999],{}),('nonsignificant_above_threshold',[0,0,0,0],{'damage_taken_q':100000000}),('hit_no_kd',[0,80,0,850000,999999],{}),('three_knockdowns_first',[0,80,0,850000,0],{'damage_taken_q':90000000,'round_knockdowns':2}),('ko',[0,80,0,850000,999999],{'damage_taken_q':100000000}),('referee',[0,80,0,850000,999999,0],{'damage_taken_q':80000000}),('referee_declines',[0,80,0,850000,999999,999999],{'damage_taken_q':80000000}),('body',[0,95,0,850000,999999],{}),('zero_stamina',[0,0,999999],{})]
 for name,draws,updates in specifications:
  states=[B.state(base),B.state(base)];states[1].update(updates)
  if name=='zero_stamina':states[0]['stamina_q']=states[1]['stamina_q']=0
  original=copy.deepcopy(states);seq=iter(draws);bounds=[]
  def uniform(n):bounds.append(n);v=next(seq);assert 0<=v<n;return v
  out=B._tick(1,states,[base,base],[man,man],uniform)
  assert len(bounds)==len(draws)
  ticks.append({'name':name,'initial':[base,base],'states':original,'manifests':[man,man],'uniform_values':draws,'uniform_bounds':bounds,'expected':out})
 recover=[]
 for name,eligible,draws in [('none',[False,False],[]),('a_only',[True,False],[0]),('b_only',[False,True],[0]),('both_stop',[True,True],[0,0]),('both_decline',[True,True],[999999,999999]),('a_stop_b_declines',[True,True],[0,999999])]:
  states=[B.state(base),B.state(base)]
  for n in (0,1):
   states[n]['stamina_q']=0;states[n]['damage_taken_q']=base['ko_threshold_q'] if eligible[n] else 0
  before=copy.deepcopy(states);seq=iter(draws);bounds=[]
  def uniform(n):bounds.append(n);return next(seq)
  out=B._recover(states,[base,base],uniform);assert len(bounds)==len(draws)
  recover.append({'name':name,'initial':[base,base],'states':before,'uniform_values':draws,'uniform_bounds':bounds,'expected':out})
 scorecases=[]
 for dmg,landed,thrown,control in [(0,0,0,0),(50000,1,100,1000000),(150000,3,100,3000000),(249999,1,3,999999),(250000,2,3,1000001)]:
  s=B.state(base);s.update(damage_dealt_q=dmg,landed=landed,thrown=thrown,control_q=control)
  scorecases.append({'state':s,'expected':B.scores(s)})
 physical=[]
 for reach,height,weight in itertools.product((-9,-8,-7,0,7,8,9),(-9,-8,-7,0,7,8,9),(-41,-40,-39,0,39,40,41)):
  rq=2500*B.clamp(reach,-8,8)+1250*B.clamp(height,-8,8);mass=B.Q+1000*B.clamp(weight,-40,40)
  physical.append({'reach_gap':reach,'height_gap':height,'weight_gap':weight,'range_hit_q':rq,'mass_damage_q':mass,'unfatigued_equal_skill_hit_q':300000+rq,'uppercut_damage_at_power50_factor1_q':B.mul(B.mul(3300000,1150000),mass)})
 chance=[{'probability':p,'ticket':t,'bound':B.Q,'expected':t<p,'consumed':1} for p,t in itertools.product((0,1,B.Q-1,B.Q),(0,B.Q-1))]
 decisions=[{'a_total_tenths':a,'b_total_tenths':b,'expected_method':'decision_draw' if a==b else 'decision','expected_winner_index':-1 if a==b else 0 if a>b else 1} for a,b in ((0,0),(1,1),(100,100),(100,99),(99,100))]
 for row in recover:
  a,b=row['expected']['corner_stopped'];row['expected_terminal_method']='double_corner_stoppage' if a and b else 'tko_corner' if a or b else '';row['expected_winner_index']=-1 if a==b else 1 if a else 0
 return {'stream_limits':limits,'physical':physical,'chance':chance,'decision_comparison':decisions,'rounding':rounding,'stream_words':streams,'uniform_rejection':uniforms,'initial_grid':init,'ticks':ticks,'recovery':recover,'scores':scorecases}

def cases():
 source=json.loads((ROOT/'inputs/manifest/manifests_v1.json').read_bytes())
 print('manifest corpus keys',list(source),flush=True)
 manifests=[x['manifest'] for x in source['cases']]
 byclass=collections.defaultdict(list)
 for m in manifests:byclass[m['physical']['weight_class']].append(m)
 records=[];methods=collections.Counter()
 for n,m in enumerate(manifests):
  pool=byclass[m['physical']['weight_class']];other=pool[(pool.index(m)+1)%len(pool)]
  born=1700000000000;week=0 if n%3==0 else (max(m['dna']['prime_weeks'],other['dna']['prime_weeks'])+7 if n%3==1 else max(x['dna']['prime_weeks']+x['dna']['decline_weeks'] for x in (m,other)))
  fs=[{'prospect_id':B.sha(b'SLUG reference fighter\0'+n.to_bytes(4,'little')+bytes([j])),'manifest':v,'born_at_ms':born,'pro_xp':n%16 if j==0 else 0,'training_skill':n%6 if j==1 else 0} for j,v in enumerate((m,other))]
  inp=B.new_input(born+week*604800000,B.sha(b'SLUG reference bout\0'+n.to_bytes(4,'little')),*fs)
  out=B.simulate(inp);check_accounting(inp,out);assert B.new_input(inp['as_of_ms'],inp['seed'],inp['b'],inp['a'])==inp
  records.append({'name':'manifest_'+str(n),'input':inp,'canonical_input':B.canonical(inp).decode(),'input_id':out['input_id'],'canonical_result':B.canonical(out).decode(),'result_digest':B.digest(out)})
  methods[out['method']]+=1
 # Boundary dates reuse valid manifests and avoid any hand-edited phenotype.
 for name,born,now in [('maximum_date',B.DATE_MAX,B.DATE_MAX),('minimum_birth',1,B.DATE_MAX)]:
  inp=copy.deepcopy(records[0]['input']);inp['as_of_ms']=now
  for key in ('a','b'):inp[key]['born_at_ms']=born;inp[key]['pro_xp']=15;inp[key]['training_skill']=5
  out=B.simulate(inp);check_accounting(inp,out)
  records.append({'name':name,'input':inp,'canonical_input':B.canonical(inp).decode(),'input_id':out['input_id'],'canonical_result':B.canonical(out).decode(),'result_digest':B.digest(out)});methods[out['method']]+=1
 selected=json.loads((ROOT/'selected_public_cases.json').read_bytes())
 for name,row in selected['cases'].items():
  inp=row['input'];out=B.simulate(inp);check_accounting(inp,out)
  records.append({'name':name,'input':inp,'canonical_input':B.canonical(inp).decode(),'input_id':out['input_id'],'canonical_result':B.canonical(out).decode(),'result_digest':B.digest(out)});methods[out['method']]+=1
 return records,dict(methods)

def negatives(base):
 cases=[]
 def add(name,mutator):
  i=copy.deepcopy(base);mutator(i)
  try:B.validate_input(i)
  except (ValueError,KeyError,TypeError,OverflowError):pass
  else:raise AssertionError('accepted '+name)
  cases.append({'name':name,'input':i,'accepted':False})
 for k in base:
  add('missing_'+k,lambda i,k=k:i.pop(k));add('null_'+k,lambda i,k=k:i.__setitem__(k,None))
 for k,v in [('version',2),('version',True),('engine_id','latest'),('mode','title'),('seed_kind','future'),('as_of_ms',0),('as_of_ms',B.DATE_MAX+1),('as_of_ms',1.0),('seed','a'*63),('seed','AA'*32),('extra',0)]:add(k+'_'+str(v),lambda i,k=k,v=v:i.__setitem__(k,v))
 for side in ('a','b'):
  for k,v in [('born_at_ms',0),('born_at_ms',B.DATE_MAX),('pro_xp',16),('pro_xp',-1),('pro_xp',True),('training_skill',6),('training_skill',-1),('prospect_id','FF'*32),('manifest',None),('extra',0)]:add(side+'_'+k+'_'+str(v),lambda i,side=side,k=k,v=v:i[side].__setitem__(k,v))
 different=B.MANIFEST.generate('00'*32)
 k=0
 while different['physical']['weight_class']==base['a']['manifest']['physical']['weight_class']:
  k+=1;different=B.MANIFEST.generate(f'{k:064x}')
 add('different_class',lambda i:i['b'].__setitem__('manifest',different))
 add('duplicate_identity',lambda i:i['b'].__setitem__('prospect_id',i['a']['prospect_id']))
 add('reversed_identity',lambda i:(i.__setitem__('a',base['b']),i.__setitem__('b',base['a'])))
 add('mutated_attribute',lambda i:i['a']['manifest']['dna']['attributes'].__setitem__('power',100 if i['a']['manifest']['dna']['attributes']['power']!=100 else 99))
 add('mutated_style',lambda i:i['a']['manifest'].__setitem__('style','wrong'))
 raw=B.canonical(base)
 malformed=[('empty',b''),('whitespace',b' '+raw),('newline',raw+b'\n'),('trailing',raw+b'{}'),('duplicate',raw.replace(b'"version":1',b'"version":1,"version":1',1)),('escaped_key',raw.replace(b'"version"',b'"versi\\u006fn"',1)),('float',raw.replace(b'"version":1',b'"version":1.0',1)),('exponent',raw.replace(b'"version":1',b'"version":1e0',1)),('negative_zero',raw.replace(b'"version":1',b'"version":-0',1)),('invalid_utf8',b'\xff'),('oversize',b' '*32769)]
 codecs=[]
 for name,data in malformed:
  try:B.parse_input(data)
  except (ValueError,KeyError,TypeError,OverflowError,UnicodeError):pass
  else:raise AssertionError('codec accepted '+name)
  codecs.append({'name':name,'raw_hex':data.hex(),'accepted':False})
 return cases,codecs

def generate():
 start=time.monotonic();records,methods=cases();primitive=primitive_cases();bad,codec=negatives(records[0]['input'])
 out={'version':1,'contract_sha256':B.CONTRACT,'provenance':'independent Python integers and pinned previous independent Python generation, no Go input/output','cases':records,'primitive':primitive,'invalid_inputs':bad,'input_codec_cases':codec,'result_mutations':[{'name':n,'path':p,'value':v} for n,p,v in [('wrong_input',['input_id'],'00'*32),('wrong_engine',['engine_id'],'other'),('wrong_winner',['winner'],'00'*32),('wrong_score',['scores',0,'total_tenths'],99999),('wrong_tick',['rounds',0,'ticks',0,'number'],2),('wrong_count',['draw_counts','combat'],0),('missing_rounds',['rounds'],[]),('null_rounds',['rounds'],None),('extra_field',['extra'],0)]],'coverage':{'complete_cases':len(records),'methods':methods,'styles':sorted({f['manifest']['style'] for r in records for f in (r['input']['a'],r['input']['b'])}),'classes':sorted({r['input']['a']['manifest']['physical']['weight_class'] for r in records}),'rare_paths_label':'private tick/recovery primitives separately cover every finish, not a claim these are canonical complete bouts'}}
 print(json.dumps({'elapsed':time.monotonic()-start,'cases':len(records),'methods':methods}),flush=True)
 return out

if __name__=='__main__':
 parser=argparse.ArgumentParser();parser.add_argument('--check',action='store_true');args=parser.parse_args();out=generate();raw=B.canonical(out)+b'\n';path=ROOT/'bout_v1.json'
 if args.check:assert path.read_bytes()==raw,'corpus changed'
 else:path.write_bytes(raw)
 print(json.dumps({'status':'passed','sha256':B.sha(raw),'bytes':len(raw)}))
