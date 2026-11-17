#!/usr/bin/env python3
"""Independent integer model from the approved English contract, never Go output."""
import copy, hashlib, importlib.util, json, pathlib, re, sys
sys.dont_write_bytecode=True
HERE=pathlib.Path(__file__).resolve().parent
CONTRACT='abb5b54edd921855b81dc5b99ef29b2ac94cadc9296dcd2981af4b57ad25040f'
Q=1000000; DATE_MAX=8640000000000000; ENGINE='slug-fighter-bout-candidate-v1'
ATTRS=('power','speed','chin','stamina','skill'); LABELS=('form-a','form-b','chin-a','chin-b','combat')
INPUT_DOMAIN=b'SLUG fighter bout input\0v1\0'; RANDOM_DOMAIN=b'SLUG fighter bout random\0v1\0'; RESULT_DOMAIN=b'SLUG fighter bout result\0v1\0'
STRIKES=('jab','cross','hook','uppercut','body'); DAMAGE=(1200000,2300000,2800000,3300000,1800000)
MIX={'slugger':(25,25,25,15,10),'outfighter':(50,20,10,5,15),'boxer_puncher':(35,25,20,10,10),'pressure':(30,20,25,10,15),'boxer':(40,25,15,8,12)}

def canonical(v):return json.dumps(v,ensure_ascii=False,separators=(',',':'),allow_nan=False).encode('utf-8')
def sha(b):return hashlib.sha256(b).hexdigest()
def load(name,path):
 spec=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(spec);sys.modules[name]=m;spec.loader.exec_module(m);return m

def check_inputs():
 pins=json.loads((HERE/'input_hashes.json').read_bytes())
 for rel,h in pins.items():
  if sha((HERE/rel).read_bytes())!=h:raise ValueError('input hash changed: '+rel)
 if pins['inputs/set42-fight-contract-draft.md']!=CONTRACT:raise ValueError('unapproved model')
 return pins
PINS=check_inputs(); MANIFEST=load('bout_independent_manifest',HERE/'inputs/manifest/reference_manifest.py'); CONDITION=load('bout_independent_condition',HERE/'inputs/condition/reference_condition.py')

def R(n,d):
 if type(n)!=int or type(d)!=int or d<=0:raise ValueError('arithmetic')
 v,r=divmod(abs(n),d);v+=2*r>d or (2*r==d and v%2==1)
 return -v if n<0 else v

def mul(x,y):return R(x*y,Q)
def div(x,y):return R(x*Q,y)
def clamp(x,a,b):return min(max(x,a),b)
def integer(v,lo,hi):return type(v)==int and lo<=v<=hi
def hid(v):return type(v)==str and re.fullmatch('[0-9a-f]{64}',v)!=None

def validate_input(i):
 if type(i)!=dict or set(i)!=set(('version','engine_id','mode','seed_kind','as_of_ms','seed','a','b')):raise ValueError('input fields')
 if not integer(i['version'],1,1) or i['engine_id']!=ENGINE or i['mode']!='standard' or i['seed_kind']!='explicit' or not hid(i['seed']) or not integer(i['as_of_ms'],1,DATE_MAX):raise ValueError('input selectors')
 fighters=[]
 for key in ('a','b'):
  f=i[key]
  if type(f)!=dict or set(f)!=set(('prospect_id','manifest','born_at_ms','pro_xp','training_skill')):raise ValueError('fighter fields')
  if not hid(f['prospect_id']) or not integer(f['born_at_ms'],1,i['as_of_ms']) or not integer(f['pro_xp'],0,15) or not integer(f['training_skill'],0,5):raise ValueError('fighter values')
  m=MANIFEST.parse_manifest(canonical(f['manifest']))
  fighters.append({'prospect_id':f['prospect_id'],'manifest':m,'born_at_ms':f['born_at_ms'],'pro_xp':f['pro_xp'],'training_skill':f['training_skill']})
 if not fighters[0]['prospect_id']<fighters[1]['prospect_id'] or fighters[0]['manifest']['physical']['weight_class']!=fighters[1]['manifest']['physical']['weight_class']:raise ValueError('pair')
 return {'version':1,'engine_id':ENGINE,'mode':'standard','seed_kind':'explicit','as_of_ms':i['as_of_ms'],'seed':i['seed'],'a':fighters[0],'b':fighters[1]}

def new_input(as_of,seed,a,b):
 a,b=sorted((a,b),key=lambda f:f['prospect_id'])
 return validate_input({'version':1,'engine_id':ENGINE,'mode':'standard','seed_kind':'explicit','as_of_ms':as_of,'seed':seed,'a':a,'b':b})

def strict_json(raw,limit):
 if type(raw)!=bytes or not 1<=len(raw)<=limit:raise ValueError('bytes')
 def pairs(items):
  out={}
  for k,v in items:
   if k in out:raise ValueError('duplicate')
   out[k]=v
  return out
 def bad(_):raise ValueError('number')
 def number(s):
  if not re.fullmatch('0|-?[1-9][0-9]*',s) or len(s)>20:raise ValueError('integer')
  return int(s)
 out=json.loads(raw.decode('utf-8'),object_pairs_hook=pairs,parse_int=number,parse_float=bad,parse_constant=bad)
 def depth(v,n):
  if isinstance(v,(dict,list)):
   if n>8:raise ValueError('depth')
   for child in (v.values() if isinstance(v,dict) else v):depth(child,n+1)
 depth(out,1)
 return out

def parse_input(raw):
 v=validate_input(strict_json(raw,32768))
 if canonical(v)!=raw:raise ValueError('canonical')
 return v

def input_id(i):return sha(INPUT_DOMAIN+canonical(validate_input(i)))
def stream_message(identifier,label,counter):
 if not hid(identifier) or label not in LABELS or not integer(counter,0,(1<<64)-1):raise ValueError('stream')
 b=label.encode('ascii');return RANDOM_DOMAIN+bytes.fromhex(identifier)+len(b).to_bytes(4,'little')+b+counter.to_bytes(8,'little')

class Streams:
 def __init__(self,identifier):self.identifier=identifier;self.counts=dict.fromkeys(LABELS,0);self.total=0
 def word(self,label):
  if self.total>=65536:raise ValueError('stream exhausted')
  n=self.counts[label];message=stream_message(self.identifier,label,n);self.counts[label]+=1;self.total+=1
  return int.from_bytes(hashlib.sha256(message).digest()[:8],'big')
 def uniform(self,label,n):return uniform_words(n,lambda:self.word(label))

def uniform_words(n,nextword):
 if type(n)!=int or not 0<n<1<<64:raise ValueError('uniform')
 threshold=(1<<64)%n
 for _ in range(256):
  v=nextword()
  if not integer(v,0,(1<<64)-1):raise ValueError('word')
  if v>=threshold:return v%n
 raise ValueError('stream exhausted')

def initial(aged,xp,training,form,jitter):
 eff={k:mul(aged[k]*100000,Q+form) for k in ATTRS};eff['skill']+=(xp+training)*Q
 return {'form_q':form,'jitter_q':jitter,'effective':eff,'max_stamina_q':60*Q+mul(eff['stamina'],1400000),'ko_threshold_q':mul(70*Q+mul(eff['chin'],450000),jitter)}

def state(init):return {'stamina_q':init['max_stamina_q'],'damage_taken_q':0,'thrown':0,'landed':0,'significant':0,'damage_dealt_q':0,'knockdowns':0,'control_q':0,'round_knockdowns':0}
def snapshots(states):return [s.copy() for s in states]

def _tick(number,states,initials,manifests,uniform,physical=(True,True,True)):
 # Private primitive seam; public simulate always supplies exact Streams.
 def chance(p):
  if not 0<=p<=Q:raise AssertionError('chance range')
  return uniform(Q)<p
 wa=[mul(initials[n]['effective']['speed'],450000+mul(550000,div(states[n]['stamina_q'],initials[n]['max_stamina_q']))) for n in (0,1)]
 actor=0 if chance(div(wa[0],sum(wa))) else 1;other=1-actor
 a,d=states[actor],states[other];ai,di=initials[actor],initials[other];am,dm=manifests[actor],manifests[other]
 ticket=uniform(100);total=0
 for strike,portion in enumerate(MIX[am['style']]):
  total+=portion
  if ticket<total:break
 name=STRIKES[strike];power=strike in (1,2,3);a['thrown']+=1;a['stamina_q']=max(0,a['stamina_q']-(1150000 if power else 800000))
 ap,dp=am['physical'],dm['physical']
 rangeq=(2500*clamp(ap['reach_in']-dp['reach_in'],-8,8) if physical[0] else 0)+(1250*clamp(ap['height_in']-dp['height_in'],-8,8) if physical[1] else 0)
 mass=Q+1000*clamp(ap['weight_lb']-dp['weight_lb'],-40,40) if physical[2] else Q
 p=300000+mul(ai['effective']['skill']-di['effective']['speed'],2200)
 if ap['stance']!=dp['stance']:p+=20000
 p+=rangeq;p=mul(p,720000+mul(280000,div(a['stamina_q'],ai['max_stamina_q'])))
 p+=mul(max(0,550000-div(d['stamina_q'],di['max_stamina_q'])),100000);p=clamp(p,60000,780000)
 hit=chance(p);damage=0;sig=False;kd=False;stop=''
 if not hit:a['control_q']+=500000
 else:
  a['landed']+=1;factor=600000+uniform(850001)
  damage=mul(mul(mul(DAMAGE[strike],650000+R(ai['effective']['power'],100)),factor),mass)
  if strike==0:a['control_q']+=500000
  if strike==4:d['stamina_q']=max(0,d['stamina_q']-mul(damage,900000))
  sig=power or damage>=1600000
  if sig:a['significant']+=1
  a['damage_dealt_q']+=damage;d['damage_taken_q']+=damage
  if sig:
   v=div(d['damage_taken_q'],di['ko_threshold_q'])
   pk=clamp(8000+mul(max(0,damage-3000000),18000)+mul(mul(v,v),220000)-mul(di['effective']['chin'],120),0,450000)
   kd=chance(pk)
   if kd:
    d['round_knockdowns']+=1;a['knockdowns']+=1;a['control_q']+=3000000;d['damage_taken_q']+=6000000;a['damage_dealt_q']+=6000000
    if d['round_knockdowns']>=3:stop='tko_three_knockdowns'
   if not stop:
    v=div(d['damage_taken_q'],di['ko_threshold_q'])
    if v>=Q:stop='ko'
    elif v>=850000 and chance(100000+mul(v-850000,1200000)):stop='tko_referee'
 return {'number':number,'actor':actor,'strike':name,'hit':hit,'damage_q':damage,'significant':sig,'knockdown':kd,'stoppage':stop,'states':snapshots(states)}

def _recover(states,initials,uniform):
 for s,i in zip(states,initials):
  recovery=mul(i['max_stamina_q'],100000+mul(R(i['effective']['stamina'],100),100000))
  s['stamina_q']=min(i['max_stamina_q'],s['stamina_q']+recovery);s['damage_taken_q']=mul(s['damage_taken_q'],970000)
 stopped=[]
 for s,i in zip(states,initials):
  stopped.append(uniform(Q)<350000 if div(s['damage_taken_q'],i['ko_threshold_q'])>=970000 else False)
 return {'states':snapshots(states),'corner_stopped':stopped}

def scores(s):
 v={'damage_tenths':R(s['damage_dealt_q'],100000),'knockdown_tenths':100*s['knockdowns'],'accuracy_tenths':R(250*s['landed'],s['thrown']) if s['thrown'] else 0,'volume_tenths':6*s['significant'],'control_tenths':R(7*s['control_q'],2*Q)}
 v['total_tenths']=sum(v.values());return v

def simulate(inp,physical=(True,True,True)):
 # physical is reference-only private counterfactual control, not public API.
 i=validate_input(inp);identifier=sha(INPUT_DOMAIN+canonical(i));rng=Streams(identifier);fs=[i['a'],i['b']]
 cond=[CONDITION.project(1,f['manifest']['dna'],f['born_at_ms'],i['as_of_ms'],f['pro_xp']) for f in fs]
 forms=[rng.uniform('form-a',200001)-100000,rng.uniform('form-b',200001)-100000]
 jitters=[920000+rng.uniform('chin-a',160001),920000+rng.uniform('chin-b',160001)]
 init=[initial(c['aged_attributes_tenths'],f['pro_xp'],f['training_skill'],fo,ji) for f,c,fo,ji in zip(fs,cond,forms,jitters)]
 states=[state(x) for x in init];uniform=lambda n:rng.uniform('combat',n);rounds=[];winner='';method=''
 for rn in range(1,4):
  for s in states:s['round_knockdowns']=0
  ticks=[]
  for tn in range(1,61):
   t=_tick(tn,states,init,[f['manifest'] for f in fs],uniform,physical);ticks.append(t)
   if t['stoppage']:winner=fs[t['actor']]['prospect_id'];method=t['stoppage'];break
  end=snapshots(states);recovery=None
  if not method and rn<3:
   recovery=_recover(states,init,uniform);stopped=recovery['corner_stopped']
   if all(stopped):method='double_corner_stoppage'
   elif any(stopped):method='tko_corner';winner=fs[1-stopped.index(True)]['prospect_id']
  rounds.append({'number':rn,'ticks':ticks,'end':end,'recovery':recovery})
  if method:break
 score=[scores(s) for s in states]
 if not method:
  x,y=[s['total_tenths'] for s in score];method='decision_draw' if x==y else 'decision'
  if x!=y:winner=fs[0 if x>y else 1]['prospect_id']
 result={'version':1,'engine_id':ENGINE,'input_id':identifier,'initial':init,'rounds':rounds,'final':snapshots(states),'winner':winner,'method':method,'end_round':rn,'end_tick':tn,'scores':score,'draw_counts':{label.replace('-','_'):rng.counts[label] for label in LABELS}}
 if len(canonical(result))>196608:raise AssertionError('result bound')
 return result

def digest(result):return sha(RESULT_DOMAIN+canonical(result))
def parse_result(inp,raw):
 actual=strict_json(raw,196608);expected=simulate(inp)
 if raw!=canonical(expected):raise ValueError('result mismatch')
 return expected
