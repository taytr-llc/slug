"""Pinned old independent arithmetic only; no Go, network, keys, work verifier or journal."""
import copy,hashlib,importlib.util,json,pathlib,re,struct,sys
sys.dont_write_bytecode=True
ROOT=pathlib.Path(__file__).resolve().parent;REPO=ROOT/'inputs/repo'
def sha(b):return hashlib.sha256(b).hexdigest()
def J(v):
 s=json.dumps(v,ensure_ascii=False,separators=(',',':'),allow_nan=False)
 for a,b in [('&','\\u0026'),('<','\\u003c'),('>','\\u003e'),('\u2028','\\u2028'),('\u2029','\\u2029')]:s=s.replace(a,b)
 return s.encode('utf-8')
def clone(v):return copy.deepcopy(v)
def lp(b):return struct.pack('<I',len(b))+b
def load(name,p):
 spec=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(spec);sys.modules[name]=m;spec.loader.exec_module(m);return m
INDEX=json.loads((ROOT/'input_index.json').read_bytes())
for n,row in INDEX['inputs'].items():
 p=ROOT/n;assert p.is_file()and not p.is_symlink()and len(p.read_bytes())==row['bytes']and sha(p.read_bytes())==row['sha256'],n
assert sha((ROOT/'inputs/contracts/set50-career-preview-contract.md').read_bytes())==INDEX['contract_sha256']=='5cf347a9e402ae46e4682728733dc637d997a1f702d11ddd65c9f8ebd04efeeb'
B=load('set50_inherited_engine',REPO/'fighterbout/testdata/reference/reference_bout.py')
C=load('set50_inherited_prefix',REPO/'internal/careerstate/testdata/reference/generate.py')
F=load('set50_inherited_arithmetic',REPO/'fightercareer/testdata/reference/generate.py')
P=load('set50_inherited_portrait',REPO/'fighterportrait/testdata/reference/reference_portraits.py')
OLD=json.loads((REPO/'fighterbout/testdata/reference/preview_transport_v1.json').read_bytes())
ENGINE=B;SAFE=9007199254740991;DATE_MAX=8640000000000000
COND_DOMAIN=b'SLUG career fight preview condition\0v1\0';SEED_DOMAIN=b'SLUG career fight preview\0v1\0'
METHODS='decision decision_draw ko tko_referee tko_three_knockdowns tko_corner double_corner_stoppage'.split()
def condition_digest(c):return sha(COND_DOMAIN+lp(J(c)))
def recipe(p):
 s=p['status'];return dict(version=1,engine_version=1,engine_id=B.ENGINE,network=s['pin']['source']['network'],genesis=s['pin']['source']['genesis'],store_id=s['store_id'],source_authority_id=s['pin']['source']['authority_id'],descriptor_id=s['pin']['source']['descriptor_id'],activation_id=p['activation']['activation_id'],career_digest=p['career_digest'],as_of_ms=p['as_of_ms'],rounds=3,training=[0,0],fighters=[dict(prospect_id=f['assignment']['prospect_id'],source_record_digest=f['assignment']['source_record_digest'],manifest_digest=f['assignment']['manifest_digest'],condition_digest=condition_digest(f['condition']),totals=clone(f['totals']))for f in p['fighters']])
def seed(r):return sha(SEED_DOMAIN+lp(J(r)))
def input_for(p,s):return B.new_input(p['as_of_ms'],s,*[dict(prospect_id=f['assignment']['prospect_id'],manifest=f['manifest'],born_at_ms=f['assignment']['born_at_ms'],pro_xp=f['totals']['xp'],training_skill=0)for f in p['fighters']])
def stable_id(p):return sha(b'SLUG candidate stable\0v1\0'+lp(J(p)))
