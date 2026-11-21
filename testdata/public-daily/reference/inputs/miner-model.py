"""Independent public-byte model. No Go, keys, filesystem or service authority."""
import copy, hashlib, json, re, struct
TIME=253402300799999
TTL=2592000000
P=2**256-2**32-977

def raw(v): return json.dumps(v,ensure_ascii=True,separators=(',',':')).encode('ascii')
def h(domain,v):
 b=raw(v); return hashlib.sha256(domain.encode()+b'\0v1\0'+struct.pack('<I',len(b))+b).hexdigest()
def hx(v,n=32): return isinstance(v,str) and bool(re.fullmatch('[0-9a-f]{%d}'%(n*2),v)) and v!='00'*n
def integer(v,lo,hi): return type(v) is int and lo<=v<=hi
def key(k):
 if not hx(k,33) or k[:2] not in ('02','03'): return False
 x=int(k[2:],16); y=pow((x*x*x+7)%P,(P+1)//4,P)
 return x<P and (y*y-x*x*x-7)%P==0

def config(c):
 if type(c)!=dict or list(c)!=['version','credential_id','runtime_id','operator_profile_id','source_id','settlement_config_id','enrollments']: return False
 if type(c['version']) is not int or c['version']!=1 or not all(hx(c[k]) for k in list(c)[1:6]): return False
 es=c['enrollments']
 if type(es)!=list or not 1<=len(es)<=64:return False
 last='';keys=set();scripts=set()
 for e in es:
  if type(e)!=dict or list(e)!=['public_key','gym_id','payee_script']:return False
  if not key(e['public_key']) or not hx(e['gym_id']) or e['gym_id']<=last or not isinstance(e['payee_script'],str) or not re.fullmatch('0014[0-9a-f]{40}',e['payee_script']):return False
  if e['public_key'] in keys or e['payee_script'] in scripts:return False
  last=e['gym_id'];keys.add(e['public_key']);scripts.add(e['payee_script'])
 return len(raw(c))+1<=65536

def state(s):
 if type(s)!=dict or list(s)!=['version','config','revision','clock_ms','credentials','history'] or type(s['version']) is not int or s['version']!=1 or not config(s['config']):return False
 if not integer(s['revision'],1,2**32-1) or not integer(s['clock_ms'],1,TIME):return False
 cs,hs=s['credentials'],s['history']
 if type(cs)!=list or len(cs)>128 or type(hs)!=list or len(hs)>4096 or len(hs)>s['revision']-1:return False
 digests=set();reuse=set();pairs=set()
 for x in hs:
  if type(x)!=dict or list(x)!=['token_hash','reuse_hash'] or not hx(x['token_hash']) or not hx(x['reuse_hash']) or x['token_hash'] in digests or x['reuse_hash'] in reuse:return False
  digests.add(x['token_hash']);reuse.add(x['reuse_hash']);pairs.add((x['token_hash'],x['reuse_hash']))
 gyms={e['gym_id'] for e in s['config']['enrollments']};last='';used=set();total=0
 for x in cs:
  if type(x)!=dict or list(x)!=['gym_id','audience','generation','token_hash','reuse_hash','issued_at_ms','expires_at_ms','revoked_at_ms']:return False
  order=x['gym_id']+':'+x['audience']
  if x['gym_id'] not in gyms or x['audience'] not in ('pool','admission') or order<=last or not integer(x['generation'],1,4096):return False
  if not integer(x['issued_at_ms'],1,s['clock_ms']) or not integer(x['expires_at_ms'],x['issued_at_ms']+1,min(TIME,x['issued_at_ms']+TTL)):return False
  if not (type(x['revoked_at_ms']) is int and (x['revoked_at_ms']==0 or x['issued_at_ms']<=x['revoked_at_ms']<=s['clock_ms'])):return False
  pair=(x['token_hash'],x['reuse_hash'])
  if pair not in pairs or pair in used:return False
  used.add(pair);total+=x['generation'];last=order
 if total!=len(hs):return False
 return len(raw(s))<=1048576

def decode(b,fn):
 try:
  if not b or len(b)>1048576 or any(c>127 for c in b):return False
  def obj(pairs):
   d={}
   for k,v in pairs:
    if k in d:raise ValueError('duplicate')
    d[k]=v
   return d
  v=json.loads(b,object_pairs_hook=obj)
  return raw(v)==b and fn(v)
 except (ValueError,TypeError,KeyError,OverflowError):return False

def generate():
 c=dict(version=1,credential_id='11'*32,runtime_id='22'*32,operator_profile_id='33'*32,source_id='44'*32,settlement_config_id='55'*32,enrollments=[dict(public_key='0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798',gym_id='66'*32,payee_script='0014'+'77'*20)])
 cid=h('SLUG pool miner enrollment',c)
 def hashes(token,aud):return dict(token_hash=h('SLUG pool miner credential',dict(config_id=cid,audience=aud,token=token)),reuse_hash=h('SLUG pool miner token reuse',dict(config_id=cid,token=token)))
 s=dict(version=1,config=c,revision=1,clock_ms=1000,credentials=[],history=[])
 pos=[]
 def add(name,kind,v):
  assert (config if kind=='config' else state)(v),name
  pos.append(dict(name=name,kind=kind,value=copy.deepcopy(v),canonical_hex=raw(v).hex(),digest=h('SLUG pool miner enrollment',v) if kind=='config' else ''))
 add('config-one','config',c);add('empty','state',s)
 hp=hashes('88'*32,'pool');ha=hashes('99'*32,'admission')
 cr=dict(gym_id='66'*32,audience='pool',generation=1,**hp,issued_at_ms=1000,expires_at_ms=2000,revoked_at_ms=0)
 s['revision']=2;s['credentials']=[cr];s['history']=[hp];add('active','state',s)
 s['clock_ms']=2000;add('expired','state',s)
 cr['revoked_at_ms']=2000;s['revision']=3;add('revoked','state',s)
 cr2=dict(gym_id='66'*32,audience='admission',generation=1,**ha,issued_at_ms=2000,expires_at_ms=2000+TTL,revoked_at_ms=0)
 s['credentials'].insert(0,cr2);s['history'].append(ha);s['revision']=4;add('both-audiences','state',s)
 h2=hashes('aa'*32,'pool');cr.update(h2,generation=2,issued_at_ms=2000,expires_at_ms=3000,revoked_at_ms=0);s['history'].append(h2);s['revision']=5;add('rotation-history','state',s)
 s['revision']=2**32-1;s['clock_ms']=TIME;add('last-revision-time','state',s)
 neg=[]
 def bad(name,kind,base,change):
  v=copy.deepcopy(base);before=raw(v);change(v);assert raw(v)!=before,name
  assert not (config if kind=='config' else state)(v),name
  neg.append(dict(name=name,kind=kind,raw_hex=raw(v).hex()))
 for field in ('credential_id','runtime_id','operator_profile_id','source_id','settlement_config_id'):
  bad('zero-'+field,'config',c,lambda v,f=field:v.__setitem__(f,'00'*32))
 bad('version-bool','config',c,lambda v:v.__setitem__('version',True));bad('empty-enrollment','config',c,lambda v:v.__setitem__('enrollments',[]));bad('null-enrollment','config',c,lambda v:v.__setitem__('enrollments',None))
 bad('invalid-key','config',c,lambda v:v['enrollments'][0].__setitem__('public_key','02'+'ff'*32));bad('duplicate-enrollment','config',c,lambda v:v['enrollments'].append(copy.deepcopy(v['enrollments'][0])))
 good=pos[6]['value']
 for name,change in [('clock-zero',lambda v:v.__setitem__('clock_ms',0)),('revision-zero',lambda v:v.__setitem__('revision',0)),('revision-over',lambda v:v.__setitem__('revision',2**32)),('null-history',lambda v:v.__setitem__('history',None)),('null-credentials',lambda v:v.__setitem__('credentials',None)),('missing-used-hash',lambda v:v['history'].pop()),('duplicate-used-hash',lambda v:v['history'].append(copy.deepcopy(v['history'][0]))),('generation-mismatch',lambda v:v['credentials'][0].__setitem__('generation',2)),('unknown-audience',lambda v:v['credentials'][0].__setitem__('audience','account')),('uncovered-gym',lambda v:v['credentials'][0].__setitem__('gym_id','fe'*32)),('future-issuance',lambda v:v['credentials'][0].__setitem__('issued_at_ms',2001)),('expiry-not-after-issue',lambda v:v['credentials'][0].__setitem__('expires_at_ms',2000)),('over-lifetime',lambda v:v['credentials'][0].__setitem__('expires_at_ms',2001+TTL)),('future-revoke',lambda v:v['credentials'][0].__setitem__('revoked_at_ms',2001)),('reordered-credentials',lambda v:v['credentials'].reverse())]:bad(name,'state',good,change)
 b=raw(c)
 for name,x in [('trailing',b+b' '),('newline',b+b'\n'),('duplicate',b.replace(b'"version":1',b'"version":1,"version":1',1)),('omitted',b.replace(b'"version":1,',b'',1)),('reordered',raw(dict(reversed(list(c.items()))))),('exponent',b.replace(b'"version":1',b'"version":1e0',1)),('nonascii',b.replace(b'"version"',b'"versi\xc3\xb3n"',1))]:
  assert not decode(x,config);neg.append(dict(name=name,kind='config',raw_hex=x.hex()))
 tokens=[dict(token=t,audience=a,config_id=cid,**hashes(t,a)) for t,a in [('88'*32,'pool'),('88'*32,'admission'),('99'*32,'admission')]]
 return dict(version=1,scope='independent grammar and digests only; modeled opaque binding IDs',positives=pos,refusals=neg,tokens=tokens)

if __name__=='__main__':
 import sys
 print(json.dumps(generate(),separators=(',',':')))
