"""D75 study utilities. No simulations or fitting at import."""
import datetime,gzip,hashlib,importlib.util,json,os,pathlib,sys
sys.dont_write_bytecode=True
ROOT=pathlib.Path(__file__).resolve().parent;OLD=ROOT.parent/'set42-fight-reference';DATA=ROOT/'data'
PROTOCOL='b1e3a27c6dad5956b98dd340efd313998324d656a4d23d7d04b40d3fcc3424d6'
RESOURCE_AMENDMENT='38ae55d7b796afda0868fd1d2b6ee96ba02630f2c8c189c575b7386505b9e91c'
ORIGINAL_PINS='c838eb52bb6638d88f9fa1f7d13da3bc3ab77a7dba2c735eb78024b461958883'
CLASSES=('light','welter','middle','light_heavy','heavy');POPS=('prime-xp0','mixed-age-xp0','prime-xpvar','mixed-age-xpvar');PANELS=('natural','close','d52-edge');ASOF=1800000000000;WEEK=604800000

def sha(raw):return hashlib.sha256(raw).hexdigest()
def filehash(path):
 h=hashlib.sha256()
 with open(path,'rb') as f:
  while True:
   b=f.read(1<<20)
   if not b:break
   h.update(b)
 return h.hexdigest()
def jsonbytes(v):return json.dumps(v,ensure_ascii=False,separators=(',',':'),allow_nan=False).encode()
def write_json(path,v):
 import resources
 raw=(json.dumps(v,indent=2,ensure_ascii=False,allow_nan=False)+'\n').encode('utf-8')
 resources.enforce(len(raw))
 with open(path,'xb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 resources.enforce()
def read_json(path):return json.loads(path.read_bytes())
def now():return datetime.datetime.now(datetime.timezone.utc).isoformat()
def u32(n):return n.to_bytes(4,'little')
def u64(n):return n.to_bytes(8,'little')
def frame(s):b=s.encode('ascii');return u32(len(b))+b

def check_pins():
 assert filehash(ROOT/'PROTOCOL.md')==PROTOCOL
 assert filehash(ROOT/'RESOURCE_AMENDMENT.md')==RESOURCE_AMENDMENT
 assert filehash(ROOT/'original-input-pins.json')==ORIGINAL_PINS
 for p,h in read_json(ROOT/'original-input-pins.json').items():assert filehash(OLD/p)==h,p
 frozen=read_json(OLD/'corpus-freeze.json')
 for p,h in frozen['files'].items():assert filehash(OLD/p)==h,p
 return {'protocol':PROTOCOL,'resource_amendment':RESOURCE_AMENDMENT,'originals':read_json(ROOT/'original-input-pins.json'),'reference_files':frozen['files']}
PINS=check_pins()
spec=importlib.util.spec_from_file_location('d75_frozen_bout',OLD/'reference_bout.py');B=importlib.util.module_from_spec(spec);sys.modules[spec.name]=B;spec.loader.exec_module(B)

def gzwrite(path):return gzip.GzipFile(filename='',mode='wb',fileobj=open(path,'xb'),mtime=0)
def rows(path):
 with gzip.open(path,'rt',encoding='utf-8') as f:
  for line in f:yield json.loads(line)
class Stream:
 def __init__(self,key,prefix=b'',budget=None):self.key=key;self.prefix=prefix;self.counter=0;self.budget=budget
 def word(self):
  if self.budget is not None:
   self.budget[0]+=1
   if self.budget[0]>self.budget[1]:raise ValueError('global random-word cap')
  r=int.from_bytes(hashlib.sha256(self.key+self.prefix+u64(self.counter)).digest()[:8],'big');self.counter+=1;return r
 def uniform(self,n):
  assert 0<n<1<<64
  threshold=(1<<64)%n
  for _ in range(256):
   w=self.word()
   if w>=threshold:return w%n
  raise ValueError('uniform rejection cap')
 def shuffle(self,values):
  for i in range(len(values)-1,0,-1):j=self.uniform(i+1);values[i],values[j]=values[j],values[i]
