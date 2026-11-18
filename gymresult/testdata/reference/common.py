"""Set48 independent primitives. Only pinned pre48 Python and public fixtures."""
import ast,copy,hashlib,importlib.util,json,pathlib,re,struct,sys,types
sys.dont_write_bytecode=True
HERE=pathlib.Path(__file__).resolve().parent
sha=lambda b:hashlib.sha256(b).hexdigest();clone=copy.deepcopy;raw=bytes.fromhex
u32=lambda n:struct.pack('<I',n);u64=lambda n:struct.pack('<Q',n);lp=lambda b:u32(len(b))+b
domain=lambda s:('SLUG '+s+'\0v1\0').encode('ascii')
label=lambda s:sha(('Set48 independent modeled '+s).encode('ascii'))
def J(x):
 return json.dumps(x,ensure_ascii=False,separators=(',',':'),allow_nan=False).replace('&','\\u0026').replace('<','\\u003c').replace('>','\\u003e').replace('\u2028','\\u2028').replace('\u2029','\\u2029').encode('utf8')
pretty=lambda x:(json.dumps(x,ensure_ascii=True,indent=2,allow_nan=False)+'\n').encode('ascii')
DATE_MAX=8640000000000000;SAFE=2**53-1;I32=2**31-1;U32=2**32-1;WEEK=604800000
class Refusal(ValueError):pass
INDEX=json.loads((HERE/'input_index.json').read_bytes())
for p,m in INDEX.items():
 b=(HERE/p).read_bytes();assert sha(b)==m['sha256']and len(b)==m['bytes'],p
assert INDEX['inputs/contracts/set48-core-contract-draft.md']['sha256']=='982126ad4678814ec7a6bedab9c4604b8c25c9955f1cd3e869c046bf37360eb3'
def load(p):return json.loads((HERE/p).read_bytes())
def module(name,p):
 s=importlib.util.spec_from_file_location(name,HERE/p);m=importlib.util.module_from_spec(s);sys.modules[name]=m;s.loader.exec_module(m);return m
EXTRACTIONS={}
def functions(p,names,ns):
 b=(HERE/p).read_bytes();tree=ast.parse(b);nodes=[n for n in tree.body if isinstance(n,(ast.FunctionDef,ast.ClassDef))and n.name in names]
 assert {n.name for n in nodes}==set(names),p
 selected=b''.join(lp(n.name.encode())+lp(ast.get_source_segment(b.decode(),n).encode())for n in nodes)
 EXTRACTIONS[p]=dict(source_sha256=sha(b),names=sorted(names),selected_source_sha256=sha(selected),selection_encoding='source-order LE32-length/name + LE32-length/exact-source-segment; AST allowlist only')
 exec(compile(ast.Module(body=nodes,type_ignores=[]),p,'exec'),ns)
MANIFEST=module('result48_frozen_manifest','inputs/manifest/reference_manifest.py');DNA=MANIFEST.DNA
cond={'json':json,'re':re,'DNA':DNA,'VERSION':1,'PROJECTOR':'slug-fighter-condition-candidate-v1','WEEK_MS':WEEK,'INT64_MAX':2**63-1,'ATTRIBUTES':('power','speed','chin','stamina','skill'),'LOSS':dict(power=20,speed=40,chin=30,stamina=30,skill=10),'RATING':dict(power=10,speed=25,chin=2,stamina=4,skill=8)}
functions('inputs/condition/reference_condition.py',{'round_even','aged_tenths','validate_dna','project'},cond)
CONDITION=types.SimpleNamespace(project=cond['project']);condition=cond['project']
# Engine is used only by an explicit independent source-provenance check, never codecs.
ENGINE_NAMES={'canonical','sha','R','mul','div','clamp','integer','hid','validate_input','new_input','strict_json','parse_input','input_id','stream_message','Streams','uniform_words','initial','state','snapshots','_tick','_recover','scores','simulate','digest','parse_result'}
ENGINE_CONSTANTS={'Q','DATE_MAX','ENGINE','ATTRS','LABELS','INPUT_DOMAIN','RANDOM_DOMAIN','RESULT_DOMAIN','STRIKES','DAMAGE','MIX'}
e={'json':json,'re':re,'hashlib':hashlib,'MANIFEST':MANIFEST,'CONDITION':CONDITION}
tree=ast.parse((HERE/'inputs/engine42/reference_bout.py').read_bytes());cn=[n for n in tree.body if isinstance(n,ast.Assign)and all(isinstance(t,ast.Name)and t.id in ENGINE_CONSTANTS for t in n.targets)]
assert {t.id for n in cn for t in n.targets}==ENGINE_CONSTANTS
exec(compile(ast.Module(body=cn,type_ignores=[]),'frozen42-constants','exec'),e)
functions('inputs/engine42/reference_bout.py',ENGINE_NAMES,e);EXTRACTIONS['inputs/engine42/reference_bout.py']['constant_names']=sorted(ENGINE_CONSTANTS);ENGINE=types.SimpleNamespace(**e)
cal={'re':re,'I32':I32};functions('inputs/calendar/common.py',{'days','civil','dateparts','sunday','noon_ms','cutoff','deadline'},cal);cutoff=cal['cutoff'];deadline=cal['deadline']
CLASSES=['light','welter','middle','light_heavy','heavy'];BASIS='candidate_zero_record_no_results_authority'
CARD_RULES=load('inputs/card46/rules.json');CARD_RULE_ID=CARD_RULES['id']
old={**globals()}
functions('inputs/bout47/common.py',{'card_store_id','card_id','identities','source_digest','record_digest','source_projection','derive_input','event_for','event_key_for'},old)
OLD=types.SimpleNamespace(**{k:old[k]for k in ['card_store_id','card_id','identities','source_digest','record_digest','source_projection','derive_input','event_for','event_key_for']})
BOUT_RECORD=load('inputs/actual47/bout_record.json');BOUT_SNAPSHOT=load('inputs/actual47/bout_snapshot.json');CARD_RECORD=load('inputs/actual47/card_record.json');CARD_PIN=BOUT_SNAPSHOT['pin']['source_pin'];BOUT_PIN=BOUT_SNAPSHOT['pin']
VIEWS=[load('inputs/actual47/view0.json'),load('inputs/actual47/view1.json')]
# Reuse explicit inherited validator functions, not prior generation/main code.
ns={**old,'store_id':lambda p:sha(domain('candidate bout store')+lp(J(p))),'Refusal':Refusal,'PINS':[BOUT_PIN],'CARD_PINS':[CARD_PIN],'VIEWS':VIEWS,'MEMBER_TEMPLATE':BOUT_RECORD['source']['corners'][0]['member'],'OWNER_TEMPLATE':BOUT_RECORD['source']['corners'][0]['owner']}
tree=ast.parse((HERE/'inputs/bout47/schema.py').read_bytes())
for n in tree.body:
 if isinstance(n,ast.Assign)and any(isinstance(t,ast.Name)and t.id in ['FIELDS','CAPS','METHODS']for t in n.targets):exec(compile(ast.Module(body=[n],type_ignores=[]),'frozen47-fields','exec'),ns)
functions('inputs/bout47/schema.py',{'req','integer','boolean','string','hx','time','enum','exact','fields','array','ordered','pin','cp','view','event','bout','condition_shape','manifest_shape','corner','source','state','saved_bout','status_value'},ns)
INHERITED=types.SimpleNamespace(**ns)
CRYPTO=module('result48_frozen_crypto','inputs/seed45/crypto.py')
