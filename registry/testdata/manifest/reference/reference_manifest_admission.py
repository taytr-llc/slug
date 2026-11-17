#!/usr/bin/env python3
"""Independent schema5 candidate receipts from pinned Python references + Argon2id.

No project Go package/runtime/output is used. The sole Go program is the copied
standalone standard-library/x/crypto Argon primitive with literal normal params.
All authority times/eligibility observations are public hypothetical fixtures,
not evidence of actual issuance, canonical inclusion, admission or ownership.
"""
import argparse
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess
import sys
import tempfile

sys.dont_write_bytecode=True
HERE=Path(__file__).resolve().parent
CONTRACT='d51178db1decf13ad09986802bb3eb7861827d372a7f35e99c13adc739420f27'
MAX_RECORD=65536
MAX_PAGE=8
MAX_PAGE_BYTES=MAX_PAGE*MAX_RECORD+MAX_PAGE+1
RECORD_FIELDS=('version','sequence','context','proof','verified','authorization','job','birth','eligibility','static_binding')
MARKER_FIELDS=('application','version','network','genesis','authority_id','descriptor_id','descriptor')
META_FIELDS=('version','network','genesis','authority_id','descriptor_id','descriptor','last_sequence','job_count','last_clock_ms')
ARGON_MODULE_HASHES={
 'argon/go.mod':'a6f877804daa32c034e5c8056c67681f6f77d97f45d6d909f49b9a00ec821e09',
 'argon/go.sum':'bff0eacd6a478f51dbac17b3ea35dff6a617bb776f34074f48c81b97a9ce33d5',
}


def sha(raw):return hashlib.sha256(raw).hexdigest()
def canon(value):return json.dumps(value,ensure_ascii=True,separators=(',',':'),allow_nan=False).encode('ascii')
def pretty(value):return (json.dumps(value,ensure_ascii=True,indent=2,allow_nan=False)+'\n').encode('ascii')
def le32(value):return struct.pack('<I',value)
def same(a,b):return json.dumps(a,sort_keys=True,allow_nan=False)==json.dumps(b,sort_keys=True,allow_nan=False)
def load(name,path):
 spec=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m


def inputs():
 provenance=json.loads((HERE/'input_hashes.json').read_bytes())
 for path,entry in provenance.items():
  if sha((HERE/path).read_bytes())!=entry['sha256']:raise ValueError('pinned input changed: '+path)
 for path,digest in ARGON_MODULE_HASHES.items():
  if sha((HERE/path).read_bytes())!=digest:raise ValueError('standalone Argon dependency contract changed: '+path)
 assert provenance['inputs/manifest-admission-contract.md']['sha256']==CONTRACT
 return provenance


PROVENANCE=inputs()
J=load('admission_independent_jobs',HERE/'inputs/job_reference/reference_manifest_jobs.py')
P=load('admission_independent_portraits',HERE/'inputs/portrait_reference/reference_portraits.py')
M=P.MANIFEST
DNA=M.DNA
AUTH=J.AUTH
TPL=J.TPL


class Argon:
 """Build only the copied standalone primitive; all dependency downloads off."""
 def __enter__(self):
  self.tmp=tempfile.TemporaryDirectory(prefix='slug-manifest-admission-argon-')
  self.binary=Path(self.tmp.name)/'argon-oracle'
  self.env=os.environ.copy()
  self.env.update(GOPROXY='off',GOSUMDB='off',GOTOOLCHAIN='local',GOWORK='off',GOFLAGS='-mod=readonly')
  self.env.setdefault('GOCACHE',str(Path(tempfile.gettempdir())/'slug-manifest-admission-go-build'))
  go=self.env.get('GO') or shutil.which('go')
  if not go:raise ValueError('locally installed Go is required for standalone Argon primitive')
  subprocess.run([go,'build','-o',str(self.binary),'argon_oracle.go'],cwd=HERE/'argon',env=self.env,check=True,capture_output=True,text=True,timeout=120)
  self.hashes={}
  return self
 def __exit__(self,*_):self.tmp.cleanup()
 def hash(self,header):
  if not re.fullmatch(r'[0-9a-f]{160}',header):raise ValueError('exact header hex')
  if header not in self.hashes:
   output=subprocess.run([str(self.binary),header],cwd=HERE/'argon',env=self.env,check=True,capture_output=True,text=True,timeout=60)
   records=json.loads(output.stdout)
   if len(records)!=1:raise ValueError('unexpected Argon result count')
   record=records[0];digest=bytes.fromhex(record['digest'])
   if record['header']!=header or len(digest)!=32 or record['work_hash']!=digest[::-1].hex():raise ValueError('Argon result/header/display mismatch')
   self.hashes[header]=record
  return copy.deepcopy(self.hashes[header])


def target(bits):
 exponent=bits>>24;mantissa=bits&0x7fffff
 if bits&0x800000 or not mantissa:raise ValueError('nonpositive target')
 result=(mantissa>>(8*(3-exponent))) if exponent<=3 else mantissa<<(8*(exponent-3))
 if not 0<result<(1<<256):raise ValueError('target range')
 return result


def search(oracle,proof,winner):
 prefix=bytes.fromhex(proof['header'])[:76];attempts=[]
 bits=struct.unpack('<I',prefix[72:76])[0]
 for nonce in range(16):
  header=(prefix+le32(nonce)).hex();value=oracle.hash(header)
  wins=int(value['work_hash'],16)<=target(bits)
  attempts.append({'nonce':nonce,**value,'meets_block_target':wins})
  if wins==winner:
   result=copy.deepcopy(proof);result['header']=header
   return result,value,attempts
 raise AssertionError('bounded public fixture search found no requested work class')


def verified(job,proof,work_hash):
 c=job['context'];assert proof['version']==2 and proof['job_id']==job['id'] and proof['gym_id']==job['gym_id']
 assert int(work_hash,16)<=int(c['prospect_target'],16)
 return {'version':2,'verification':'supplied-context','network':c['network'],'genesis':c['genesis'],'prospect_id':sha(J.PROOF.ID_DOMAIN+le32(c['network'])+bytes.fromhex(c['genesis'])+bytes.fromhex(work_hash)),'policy_id':J.PROOF.policy_id(c).hex(),'gym_id':job['gym_id'],'work_hash':work_hash,'parent_hash':c['parent_hash'],'parent_height':c['parent_height'],'dna':DNA.derive_dna(work_hash)}


def appearance(work_hash):
 manifest=M.generate(work_hash);raw=M.canonical(manifest)
 assert M.parse_manifest(raw)==manifest
 svg=P.render(raw);stats=P.validate_xml(svg,'Portrait preview of '+manifest['name']['display'])
 binding={'version':1,'descriptor_id':J.descriptor_id(J.CANDIDATE),'manifest_digest':M.digest_manifest(raw),'portrait_digest':sha(P.DOMAIN+svg)}
 return manifest,raw,svg,binding,stats


def signature(job,proof,scalar):
 digest=J.OLD.admission_digest(job,proof,J.PROOF.policy_id(job['context']))
 r,s=AUTH.sign(scalar,digest)
 assert AUTH.verify(AUTH.multiply(scalar),digest,r,s)
 return {'version':2,'public_key':job['public_key'],'signature':AUTH.der_signature(r,s).hex()},digest.hex()


def parse_signature(encoded):
 if not isinstance(encoded,str) or encoded!=encoded.lower() or len(encoded)>144:raise ValueError('signature shape')
 raw=bytes.fromhex(encoded)
 if len(raw)<8 or raw[0]!=0x30 or raw[1]!=len(raw)-2:raise ValueError('DER sequence')
 pos=2;values=[]
 for _ in range(2):
  if pos+2>len(raw) or raw[pos]!=2:raise ValueError('DER integer')
  size=raw[pos+1];part=raw[pos+2:pos+2+size];pos+=2+size
  if not part or part[0]&128 or len(part)!=size:raise ValueError('DER sign/length')
  values.append(int.from_bytes(part,'big'))
 if pos!=len(raw) or AUTH.der_signature(*values)!=raw:raise ValueError('noncanonical DER')
 r,s=values
 if not 1<=r<AUTH.ORDER or not 1<=s<=AUTH.ORDER//2:raise ValueError('scalar/low-S')
 return r,s


def proof_binding(job,proof):
 """Independent header/coinbase/merkle checks for this exact fixture family."""
 J.exact_fields(proof,('version','gym_id','header','coinbase','merkle_branch','job_id'))
 if proof['version']!=2 or proof['gym_id']!=job['gym_id'] or proof['job_id']!=job['id']:raise ValueError('proof job/gym binding')
 for key in ('gym_id','header','coinbase','job_id'):
  if not isinstance(proof[key],str) or proof[key]!=proof[key].lower() or not re.fullmatch('[0-9a-f]+',proof[key]):raise ValueError('canonical proof hex')
 header=bytes.fromhex(proof['header']);coinbase=bytes.fromhex(proof['coinbase']);c=job['context']
 if len(header)!=80 or not 1<=len(coinbase)<=16384:raise ValueError('proof byte bounds')
 if header[4:36]!=bytes.fromhex(c['parent_hash'])[::-1] or struct.unpack('<I',header[72:76])[0]!=c['expected_bits']:raise ValueError('header context')
 stamp=struct.unpack('<I',header[68:72])[0]
 if not c['min_timestamp']<=stamp<=c['max_timestamp']:raise ValueError('header time')
 if J.commitment(job) not in coinbase:raise ValueError('job commitment')
 if not isinstance(proof['merkle_branch'],list) or len(proof['merkle_branch'])>20:raise ValueError('branch bound')
 merkle=TPL.sha256d(coinbase)
 for value in proof['merkle_branch']:
  J.hex32(value);sibling=bytes.fromhex(value)[::-1]
  if merkle==sibling:raise ValueError('equal sibling')
  merkle=TPL.sha256d(merkle+sibling)
 if merkle!=header[36:68]:raise ValueError('merkle binding')


def validate_record(record,pin,jobs,meta,known_hashes):
 """Check generated/mutated fixture links; not a general trusted store/parser."""
 J.exact_fields(record,RECORD_FIELDS)
 if record['version']!=5 or type(record['sequence']) is not int or not 1<=record['sequence']<=meta['last_sequence']:raise ValueError('record version/sequence')
 job=record['job'];J.validate_job(job)
 if job['id'] not in jobs or not same(job,jobs[job['id']]):raise ValueError('own issued job')
 if job['authority_id']!=pin['authority_id'] or job['descriptor_id']!=pin['descriptor_id'] or job['context']['network']!=pin['network'] or job['context']['genesis']!=pin['genesis']:raise ValueError('authority pin')
 if not same(record['context'],job['context']):raise ValueError('context identity')
 proof=record['proof'];proof_binding(job,proof)
 auth=record['authorization'];J.exact_fields(auth,('version','public_key','signature'))
 if auth['version']!=2 or auth['public_key']!=job['public_key']:raise ValueError('authorization key')
 r,s=parse_signature(auth['signature']);_,point=J.public_key(auth['public_key'])
 digest=J.OLD.admission_digest(job,proof,J.PROOF.policy_id(job['context']))
 if not AUTH.verify(point,digest,r,s):raise ValueError('authorization signature')
 if proof['header'] not in known_hashes:raise ValueError('unknown fixture work input')
 actual=verified(job,proof,known_hashes[proof['header']]['work_hash'])
 if not same(record['verified'],actual):raise ValueError('verified mismatch')
 birth=record['birth'];J.exact_fields(birth,('rule_version','born_at_ms','accepted_at_ms'))
 if birth['rule_version']!=1 or birth['born_at_ms']!=job['issued_at_ms'] or type(birth['accepted_at_ms']) is not int or not job['issued_at_ms']<=birth['accepted_at_ms']<job['expires_at_ms'] or birth['accepted_at_ms']>meta['last_clock_ms']:raise ValueError('birth interval')
 e=record['eligibility'];J.exact_fields(e,('rule_version','source','hash','height'))
 if e['rule_version']!=2:raise ValueError('eligibility version')
 if e['source']=='current-parent':
  if e['hash']!=job['context']['parent_hash'] or e['height']!=job['context']['parent_height']:raise ValueError('parent eligibility')
 elif e['source']=='canonical-winning-block':
  if e['hash']!=actual['work_hash'] or e['height']!=job['context']['parent_height']+1 or int(actual['work_hash'],16)>target(job['context']['expected_bits']):raise ValueError('winning eligibility')
 else:raise ValueError('eligibility source')
 _,_,_,binding,_=appearance(actual['work_hash'])
 if not same(record['static_binding'],binding):raise ValueError('static binding')
 if len(canon(record))>MAX_RECORD:raise ValueError('record byte bound')


def maximum_proof(job):
 c=job['context'];script=TPL.script_number_push(c['parent_height']+1)+b'\x08'+struct.pack('<Q',17)
 payout=b'\x76\xa9\x14'+TPL.PAYOUT_HASH160+b'\x88\xac'
 def pad(count):return b'\x6a\x4d'+struct.pack('<H',count)+b'\xa5'*count
 def build(second):return TPL.transaction([(bytes(32),0xffffffff,script,0xffffffff)],[(1000000000,payout),(0,J.commitment(job)),(0,pad(8000)),(0,pad(second))],None)[1]
 initial=build(1000);second=1000+(16384-len(initial));coinbase=build(second)
 assert len(coinbase)==16384 and 0<second<10000
 siblings=[TPL.sha256d(b'SLUG manifest admission fixture sibling\x00v1\x00'+le32(i)) for i in range(20)]
 merkle=TPL.sha256d(coinbase)
 for sibling in siblings:
  assert merkle!=sibling;merkle=TPL.sha256d(merkle+sibling)
 header=le32(4)+bytes.fromhex(c['parent_hash'])[::-1]+merkle+le32(c['min_timestamp'])+le32(c['expected_bits'])+le32(0)
 return {'version':2,'gym_id':job['gym_id'],'header':header.hex(),'coinbase':coinbase.hex(),'merkle_branch':[s[::-1].hex() for s in siblings],'job_id':job['id']}


def storage_preflight(raw,limit,depth_limit,marker=False):
 if not isinstance(raw,bytes) or not 0<len(raw)<=limit or not raw.isascii():raise ValueError('stored JSON size/encoding')
 depth=0;quoted=False;escaped=False
 for byte in raw:
  if quoted:
   if escaped:escaped=False
   elif byte==92:escaped=True
   elif byte==34:quoted=False
  elif byte==34:quoted=True
  elif byte in (123,91):
   depth+=1
   if depth>depth_limit:raise ValueError('stored JSON depth')
  elif byte in (125,93):
   depth-=1
   if depth<0:raise ValueError('stored JSON depth')
 if quoted or depth:raise ValueError('unclosed JSON')
 parsed=J.decoder().decode(raw.decode('ascii'))
 # Convert bounded ParsedInts to ordinary JSON numbers, then enforce exact bytes.
 expected=canon(parsed)+(b'\n' if marker else b'')
 if raw!=expected:raise ValueError('noncanonical stored bytes')
 return parsed


def transport_cases(kind,value,fields,limit,depth):
 marker=kind=='marker';raw=canon(value)+(b'\n' if marker else b'');cases=[]
 def add(name,data,accepted=False):
  cases.append({'name':kind+'_'+name,'input_hex':data.hex(),'accepted_encoding':accepted})
 add('canonical',raw,True)
 for field in fields:
  for mutation in ('missing','null','duplicate','case_alias','escaped_duplicate'):
   changed=copy.deepcopy(value)
   if mutation in ('duplicate','escaped_duplicate'):
    key=canon(field) if mutation=='duplicate' else ('"\\u%04x'%ord(field[0])+field[1:]+'"').encode()
    data=b'{'+key+b':'+canon(value[field])+b','+raw[1:]
   else:
    if mutation=='missing':del changed[field]
    elif mutation=='null':changed[field]=None
    else:changed[field.upper()]=changed.pop(field)
    data=canon(changed)+(b'\n' if marker else b'')
   add(field+'_'+mutation,data)
 for name,data in [('empty',b''),('null',b'null'),('array',b'[]'),('pretty',pretty(value)),('reordered',canon(dict(reversed(list(value.items()))))+(b'\n' if marker else b'')),('leading_space',b' '+raw),('extra_newline',raw+b'\n'),('unknown',canon({**value,'extra':1})+(b'\n' if marker else b'')),('trailing',raw+b'{}'),('invalid_utf8',raw+b'\xff'),('over_size',raw+b' '*(limit+1-len(raw))),('too_deep',b'{"x":'*(depth+1)+b'0'+b'}'*(depth+1))]:add(name,data)
 if marker:add('missing_terminal_lf',raw[:-1])
 for case in cases:
  try:
   data=bytes.fromhex(case['input_hex']);parsed=storage_preflight(data,limit,depth,marker)
   J.exact_fields(parsed,fields)
   # These transport mutations retain the same intended fixture row. Typed
   # canonical field order and required nonnull values must also be preserved.
   if not same(parsed,value) or data!=raw:raise ValueError('typed canonical fixture row differs')
   accepted=True
  except (ValueError,TypeError,KeyError,IndexError):accepted=False
  assert accepted==case['accepted_encoding'],case['name']
 return cases


def corpus(oracle):
 assert pretty(J.corpus())==(HERE/'inputs/job_reference/manifest_job_v1.json').read_bytes()
 original_portraits=json.loads((HERE/'inputs/portrait_reference/portraits_v1.json').read_bytes())
 for case in original_portraits['cases']:
  assert P.render(M.canonical(case['manifest'])).decode()==case['svg']
  assert sha(P.DOMAIN+case['svg'].encode())==case['svg_digest']
 descriptor=J.CANDIDATE.copy();descriptor_id=J.descriptor_id(descriptor)
 base=J.OLD_TEMPLATES[0];context=base['job']['context'].copy();authority=bytes(range(64,96)).hex()
 job_a=J.new_job(context,authority,descriptor_id,base['job']['public_key'],base['job']['challenge'],base['job']['issued_at_ms'])
 old_b=J.OLD_JOBS[1]['job'];job_b=J.new_job(context,authority,descriptor_id,old_b['public_key'],old_b['challenge'],job_a['issued_at_ms'])
 jobs=[job_a,job_b];jobmap={j['id']:j for j in jobs}
 pin={'version':5,'network':context['network'],'genesis':context['genesis'],'authority_id':authority,'descriptor_id':descriptor_id}
 marker={'application':'slug-prospect-registry',**pin,'descriptor':descriptor}
 specifications=[('coinbase-only-current-parent',0,job_a,1,False,'current-parent'),('dependent-legacy-current-parent',1,job_a,1,False,'current-parent'),('witness-canonical-winner',2,job_a,1,True,'canonical-winning-block'),('second-key-witness-current-parent',2,job_b,2,False,'current-parent'),('maximum-proof-current-parent',None,job_a,1,False,'current-parent')]
 cases=[];record_rows=[];sequence_rows=[]
 for sequence,(name,index,job,scalar,winner,source) in enumerate(specifications,1):
  if index is None:
   candidate=None;template=None;proof=maximum_proof(job);metadata={}
  else:
   original=J.OLD_TEMPLATES[index];template=copy.deepcopy(original['template']);candidate,metadata=J.candidate(job,template,original['payout_address'],original['extra_nonce']);proof=candidate['proof']
  proof,work,attempts=search(oracle,proof,winner);proof_binding(job,proof)
  if candidate is not None:
   candidate['proof']=copy.deepcopy(proof);candidate['block_hex']=proof['header']+candidate['block_hex'][160:]
  v=verified(job,proof,work['work_hash']);auth,digest=signature(job,proof,scalar)
  m,mb,svg,binding,stats=appearance(work['work_hash']);assert m['dna']==v['dna']
  eligibility={'rule_version':2,'source':source,'hash':work['work_hash'] if winner else context['parent_hash'],'height':context['parent_height']+(1 if winner else 0)}
  birth={'rule_version':1,'born_at_ms':job['issued_at_ms'],'accepted_at_ms':job['issued_at_ms']+1001*sequence}
  record={'version':5,'sequence':sequence,'context':context.copy(),'proof':proof,'verified':v,'authorization':auth,'job':job,'birth':birth,'eligibility':eligibility,'static_binding':binding}
  raw=canon(record);assert len(raw)<=MAX_RECORD
  cases.append({'name':name,'public_test_scalar':scalar,'job':job,'template':template,'candidate':candidate,'proof':proof,'authorization':auth,'expected_authorization_digest':digest,'expected_verified':v,'expected_record':record,'record_canonical':raw.decode(),'record_bytes':len(raw),'manifest':m,'manifest_canonical':mb.decode(),'svg':svg.decode(),'svg_sha256':sha(svg),'static_binding':binding,'nonce':attempts[-1]['nonce'],'attempts':attempts,'meets_block_target':winner,'block_target_hex':target(context['expected_bits']).to_bytes(32,'big').hex(),'eligibility_input':eligibility,'accepted_at_ms':birth['accepted_at_ms'],'coinbase_bytes':len(bytes.fromhex(proof['coinbase'])),'merkle_siblings':len(proof['merkle_branch']),'full_template_fixture':index is not None,'actual_issuance_verified':False,'actual_canonicality_verified':False,'actual_admission_verified':False,**metadata,**stats})
  record_rows.append({'key':'record:'+v['prospect_id'],'value':raw.decode()});sequence_rows.append({'key_hex':(b'sequence:'+struct.pack('>Q',sequence)).hex(),'value':v['prospect_id']})
 meta={**pin,'descriptor':descriptor,'last_sequence':len(cases),'job_count':len(jobs),'last_clock_ms':cases[-1]['accepted_at_ms']}
 for case in cases:validate_record(case['expected_record'],pin,jobmap,meta,oracle.hashes)
 mutations=[];base_record=cases[0]['expected_record']
 def mutate(name,path,value):
  record=copy.deepcopy(base_record);part=record
  for key in path[:-1]:part=part[key]
  part[path[-1]]=value
  try:validate_record(record,pin,jobmap,meta,oracle.hashes)
  except (ValueError,TypeError,KeyError,IndexError):pass
  else:raise AssertionError('mutation accepted: '+name)
  mutations.append({'name':name,'record':record,'accepted':False})
 for name,path,value in [('version',['version'],4),('zero_sequence',['sequence'],0),('sequence_above_meta',['sequence'],6),('birth_rule',['birth','rule_version'],2),('birth_not_issue',['birth','born_at_ms'],job_a['issued_at_ms']+1),('accepted_before_issue',['birth','accepted_at_ms'],job_a['issued_at_ms']-1),('accepted_at_expiry',['birth','accepted_at_ms'],job_a['expires_at_ms']),('accepted_above_clock',['birth','accepted_at_ms'],meta['last_clock_ms']+1),('eligibility_rule',['eligibility','rule_version'],1),('eligibility_source',['eligibility','source'],'side-branch'),('eligibility_hash',['eligibility','hash'],'00'*32),('eligibility_height',['eligibility','height'],context['parent_height']+1),('binding_version',['static_binding','version'],2),('binding_descriptor',['static_binding','descriptor_id'],'00'*32),('manifest_digest',['static_binding','manifest_digest'],'00'*32),('portrait_digest',['static_binding','portrait_digest'],'00'*32),('verified_hash',['verified','work_hash'],'00'*32),('verified_id',['verified','prospect_id'],'00'*32),('verified_dna',['verified','dna','attributes','power'],0),('foreign_job',['job'],job_b),('context_chain',['context','genesis'],'00'*32),('authorization_key',['authorization','public_key'],job_b['public_key']),('proof_uppercase',['proof','header'],base_record['proof']['header'].upper()),('proof_job',['proof','job_id'],'00'*32)]:mutate(name,path,value)
 forged=copy.deepcopy(base_record);forged['eligibility']={'rule_version':2,'source':'canonical-winning-block','hash':base_record['verified']['work_hash'],'height':context['parent_height']+1}
 try:validate_record(forged,pin,jobmap,meta,oracle.hashes)
 except ValueError:pass
 else:raise AssertionError('nonwinner accepted as winning eligibility')
 mutations.append({'name':'nonwinner_winning_claim','record':forged,'accepted':False})
 r,s=parse_signature(base_record['authorization']['signature'])
 mutate('high_s_authorization',['authorization','signature'],AUTH.der_signature(r,AUTH.ORDER-s).hex())
 alternate_authorizations=[]
 for case in (cases[0],cases[3]):
  # k=1 is deliberately public fixture material, NEVER a production signer.
  # Its distinct valid low-S signature proves retry equality need not include
  # signature bytes; the authority still retains its original signature.
  scalar=case['public_test_scalar'];digest=bytes.fromhex(case['expected_authorization_digest'])
  r=AUTH.GENERATOR[0]%AUTH.ORDER;s=(int.from_bytes(digest,'big')+r*scalar)%AUTH.ORDER
  if s>AUTH.ORDER//2:s=AUTH.ORDER-s
  assert AUTH.verify(AUTH.multiply(scalar),digest,r,s)
  alternate={'version':2,'public_key':case['job']['public_key'],'signature':AUTH.der_signature(r,s).hex()}
  assert alternate!=case['authorization']
  alternate_authorizations.append({'name':case['name']+'-equivalent-signature','proof':case['proof'],'authorization':alternate,'public_test_nonce':1,'expected_original_record_sequence':case['expected_record']['sequence'],'expected_original_authorization':case['authorization']})
 trust_variant=copy.deepcopy(base_record);trust_variant['birth']['accepted_at_ms']+=1
 validate_record(trust_variant,pin,jobmap,meta,oracle.hashes)
 storage_inputs={'marker':transport_cases('marker',marker,MARKER_FIELDS,4096,2),'metadata':transport_cases('metadata',meta,META_FIELDS,4096,2),'record':transport_cases('record',base_record,RECORD_FIELDS,MAX_RECORD,4)}
 canonical_marker=canon(marker)+b'\n';canonical_meta=canon(meta)
 assert len(canonical_marker)<=4096 and len(canonical_meta)<=4096
 expected_info={'pin':pin,'descriptor':descriptor,'records':len(cases),'jobs':len(jobs),'read_only':False}
 pin_cases=[{'name':'exact-pin','pin':pin.copy(),'canonical_shape':True,'matches_expected_authority':True}]
 for name,key,value,shape in [('other-authority','authority_id','ff'*32,True),('zero-authority-shape','authority_id','00'*32,True),('other-chain','genesis','01'*32,True),('other-network','network',1,True),('unsupported-descriptor','descriptor_id','00'*32,True),('old-schema','version',4,False),('uppercase-authority','authority_id',authority.upper(),False),('short-authority','authority_id',authority[:-1],False),('zero-genesis','genesis','00'*32,False)]:
  other=pin.copy();other[key]=value;pin_cases.append({'name':name,'pin':other,'canonical_shape':shape,'matches_expected_authority':False})
 return {'oracle_version':1,'candidate_fixture_only':True,'approved_contract_sha256':CONTRACT,'no_actual_issuance_canonicality_or_admission':True,'provenance':PROVENANCE,'descriptor':descriptor,'pin':pin,'marker':marker,'marker_canonical':canonical_marker.decode(),'issued_jobs':[{'job':j,'canonical':canon(j).decode(),'job_id_preimage':J.job_preimage(j).hex(),'commitment_script':J.commitment(j).hex()} for j in jobs],'public_fixture_random_stream_hex':authority+job_a['challenge']+job_b['challenge'],'issued_at_ms':job_a['issued_at_ms'],'cases':cases,'metadata':meta,'metadata_canonical':canonical_meta.decode(),'expected_info':expected_info,'database_rows':{'meta':canonical_meta.decode(),'jobs':[{'key':'job:'+j['id'],'value':canon(j).decode()} for j in jobs],'records':record_rows,'sequences':sequence_rows},'storage_inputs':storage_inputs,'record_mutations':mutations,'alternate_authorizations':alternate_authorizations,'pin_cases':pin_cases,'trust_limit_case':{'name':'self_consistent_accepted_time_rewrite','record':trust_variant,'passes_local_consistency_checks':True,'authenticates_actual_acceptance':False,'explanation':'Unsigned work/auth/binding cannot prove the local authority timestamp; trusted storage and explicit configured pin remain necessary.'},'validation':{'records':len(cases),'issued_jobs':len(jobs),'winning_records':sum(c['meets_block_target'] for c in cases),'nonwinning_records':sum(not c['meets_block_target'] for c in cases),'witness_records':sum(c['candidate'] is not None and c['name'].find('witness')>=0 for c in cases),'maximum_coinbase_bytes':max(c['coinbase_bytes'] for c in cases),'maximum_merkle_siblings':max(c['merkle_siblings'] for c in cases),'maximum_record_bytes':max(c['record_bytes'] for c in cases),'argon_attempts':len(oracle.hashes),'argon_memory_kib':256*1024,'argon_parallelism':1,'argon_time':1,'storage_input_cases':sum(len(c) for c in storage_inputs.values()),'record_mutations':len(mutations),'alternate_authorizations':len(alternate_authorizations),'pin_cases':len(pin_cases),'old_job_corpus_exact':True,'old_portrait_cases_exact':len(original_portraits['cases']),'max_page_size':MAX_PAGE,'max_page_bytes':MAX_PAGE_BYTES,'actual_node_or_store_contacted':False,'production_activation':False}}


def main():
 parser=argparse.ArgumentParser(description=__doc__);group=parser.add_mutually_exclusive_group(required=True);group.add_argument('--generate',action='store_true');group.add_argument('--check',action='store_true');args=parser.parse_args()
 with Argon() as oracle:result=corpus(oracle)
 raw=pretty(result);path=HERE/'manifest_admission_v1.json'
 if args.generate:path.write_bytes(raw)
 elif path.read_bytes()!=raw:raise SystemExit('FAIL: independent corpus regeneration differs')
 print(json.dumps({'result':'pass','mode':'generate' if args.generate else 'check','corpus_sha256':sha(raw),'validation':result['validation']},sort_keys=True))


if __name__=='__main__':main()
