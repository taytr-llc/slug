#!/usr/bin/env python3
"""Independent set36 descriptor/Job2/candidate oracle, Python stdlib only.

Expected protocol bytes never use project Go code/output. Existing independent
Python references are copied unchanged and provide context, secp256k1, old-job
parity and transaction primitives. No Argon, real key, node, network or issuer.
"""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import struct
import sys

sys.dont_write_bytecode=True
HERE=Path(__file__).resolve().parent
REPO=HERE/'inputs/reference_repo'
CONTRACT_HASH='6a761c2e78081178e45c493953b78759f721ad3772a4e1fe684f38ec5fb0cd66'
DESCRIPTOR_DOMAIN=b'SLUG fighter issuance descriptor\x00v1\x00'
JOB_DOMAIN=b'SLUG mining job\x00v2\x00'
LIFETIME=300000
MAX_I64=(1<<63)-1
DESC_FIELDS=('version','dna_version','manifest_version','generator_id','manifest_table_hash','manifest_contract_hash','portrait_trait_version','renderer_version','renderer_id','renderer_contract_hash')
CTX_FIELDS=('version','network','genesis','dna_version','prospect_target','parent_hash','parent_height','expected_bits','min_timestamp','max_timestamp')
JOB_FIELDS=('version','rule_version','id','authority_id','descriptor_id','context','public_key','gym_id','challenge','issued_at_ms','expires_at_ms')
OLD_FIELDS=('version','rule_version','id','context','public_key','gym_id','challenge','issued_at_ms','expires_at_ms')
HASH_FIELDS=('manifest_table_hash','manifest_contract_hash','renderer_contract_hash')
VERSIONS=('version','dna_version','manifest_version','portrait_trait_version','renderer_version')
CANDIDATE={
 'version':1,'dna_version':1,'manifest_version':1,
 'generator_id':'slug-fighter-manifest-candidate-v1',
 'manifest_table_hash':'72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44',
 'manifest_contract_hash':'ba8d33dbad3722eed6b4406ef120f946d7e06e1ea04305cbe8648889ba169e80',
 'portrait_trait_version':1,'renderer_version':1,
 'renderer_id':'slug-fighter-portrait-candidate-v1',
 'renderer_contract_hash':'8ae3c8dbe92fbef719880227e107cb98cbfd6419d984cad08ebb5d5e0a9c0df5',
}


def sha(data):return hashlib.sha256(data).digest()
def canon(value):return json.dumps(value,ensure_ascii=True,separators=(',',':'),allow_nan=False).encode('ascii')
def pretty(value):return (json.dumps(value,ensure_ascii=True,indent=2,allow_nan=False)+'\n').encode('ascii')
def same(left,right):return json.dumps(left,sort_keys=True,allow_nan=False)==json.dumps(right,sort_keys=True,allow_nan=False)
def le32(value):return struct.pack('<I',value)
def le64(value):return struct.pack('<Q',value)


def module(name,relative):
 spec=importlib.util.spec_from_file_location(name,REPO/relative)
 value=importlib.util.module_from_spec(spec);spec.loader.exec_module(value);return value


def inputs():
 provenance=json.loads((HERE/'input_hashes.json').read_bytes())
 for path,item in provenance.items():
  if sha((HERE/path).read_bytes()).hex()!=item['sha256']:raise ValueError('pinned input changed: '+path)
 assert provenance['inputs/manifest-job-contract.md']['sha256']==CONTRACT_HASH
 assert provenance['inputs/fighter-manifest-contract.md']['sha256']==CANDIDATE['manifest_contract_hash']
 assert provenance['inputs/portrait-renderer-contract.md']['sha256']==CANDIDATE['renderer_contract_hash']
 assert sha(b'SLUG fighter manifest tables\x00v1\x00'+(HERE/'inputs/tables_v1.json').read_bytes()).hex()==CANDIDATE['manifest_table_hash']
 return provenance


PROVENANCE=inputs()
AUTH=module('independent_manifest_job_auth','gymauth/testdata/reference_authorizations.py')
PROOF=module('independent_manifest_job_context','prospect/testdata/reference_proofs.py')
OLD=module('independent_original_job','prospectjob/testdata/reference_jobs.py')
TPL=module('independent_original_template','prospectwork/testdata/reference_templates.py')
OLD_JOBS=json.loads((REPO/'prospectjob/testdata/jobs_v1.json').read_bytes())
OLD_TEMPLATES=json.loads((REPO/'prospectwork/testdata/templates_v1.json').read_bytes())


class ParsedInt(int):
 def __new__(cls,text):
  if len(text)>20 or not re.fullmatch(r'-?(0|[1-9][0-9]*)',text):raise ValueError('bounded integer token')
  obj=int.__new__(cls,text);obj.literal=text;return obj


def integer(value,minimum,maximum,unsigned=False):
 if type(value) not in (int,ParsedInt) or not minimum<=value<=maximum:raise ValueError('integer range/type')
 if unsigned and isinstance(value,ParsedInt) and value.literal.startswith('-'):raise ValueError('unsigned token sign')
 return int(value)


def hex32(value,normalize=False,nonzero=False):
 pattern=r'[0-9a-fA-F]{64}' if normalize else r'[0-9a-f]{64}'
 if not isinstance(value,str) or not re.fullmatch(pattern,value):raise ValueError('hash shape')
 raw=bytes.fromhex(value)
 if nonzero and not any(raw):raise ValueError('zero context hash')
 return raw.hex()


def exact_fields(value,fields):
 if not isinstance(value,dict) or set(value)!=set(fields):raise ValueError('exact required schema')


def normalize_descriptor(d):
 exact_fields(d,DESC_FIELDS)
 result={key:d[key] for key in DESC_FIELDS}
 for key in VERSIONS:result[key]=integer(d[key],1,(1<<32)-1,unsigned=True)
 if result['version']!=1:raise ValueError('unknown descriptor format')
 for key in ('generator_id','renderer_id'):
  if not isinstance(d[key],str) or not re.fullmatch(r'[A-Za-z0-9_-]{1,96}',d[key]):raise ValueError('identifier shape')
 for key in HASH_FIELDS:result[key]=hex32(d[key],normalize=True)
 if len(canon(result))>2048:raise ValueError('descriptor output size')
 return result


def descriptor_bytes(d):
 result=normalize_descriptor(d)
 if not same(d,result):raise ValueError('noncanonical descriptor value')
 return canon(result)


def descriptor_id(d):return sha(DESCRIPTOR_DOMAIN+descriptor_bytes(d)).hex()


def resolve(identifier,production=False):
 hex32(identifier)
 if production:raise ValueError('production unavailable')
 if identifier!=descriptor_id(CANDIDATE):raise ValueError('unsupported descriptor')
 return CANDIDATE.copy()


def preflight(raw,maximum,depth_limit):
 if not isinstance(raw,bytes) or not 1<=len(raw)<=maximum or not raw.isascii():raise ValueError('JSON byte bound/encoding')
 depth=0;quoted=False;escaped=False
 for byte in raw:
  if quoted:
   if escaped:escaped=False
   elif byte==92:escaped=True
   elif byte==34:quoted=False
  elif byte==34:quoted=True
  elif byte in (91,93):raise ValueError('no arrays')
  elif byte==123:
   depth+=1
   if depth>depth_limit:raise ValueError('JSON depth')
  elif byte==125:
   depth-=1
   if depth<0:raise ValueError('JSON depth')
 if quoted or depth:raise ValueError('incomplete JSON')


def decoder():
 def pairs(items):
  result={}
  for key,value in items:
   if key in result:raise ValueError('decoded duplicate key')
   result[key]=value
  return result
 def forbidden(_):raise ValueError('fraction/exponent/nonfinite')
 return json.JSONDecoder(object_pairs_hook=pairs,parse_int=ParsedInt,parse_float=forbidden,parse_constant=forbidden)


def decode(raw,maximum,depth_limit):
 preflight(raw,maximum,depth_limit)
 text=raw.decode('ascii');value=decoder().decode(text)
 if not isinstance(value,dict):raise ValueError('root must be object')
 return value


def parse_descriptor(raw):
 value=decode(raw,2048,1)
 if b'\\' in raw:raise ValueError('descriptor escaped spelling')
 if descriptor_bytes(value)!=raw:raise ValueError('noncanonical descriptor bytes')
 return normalize_descriptor(value)


def public_key(value,normalize=False):
 if not isinstance(value,str) or not re.fullmatch(r'0[23][0-9a-fA-F]{64}',value):raise ValueError('compressed key')
 if not normalize and value!=value.lower():raise ValueError('noncanonical public key')
 raw=bytes.fromhex(value);x=int.from_bytes(raw[1:],'big')
 if x>=AUTH.FIELD:raise ValueError('public x coordinate')
 y2=(x*x*x+7)%AUTH.FIELD;y=pow(y2,(AUTH.FIELD+1)//4,AUTH.FIELD)
 if y*y%AUTH.FIELD!=y2:raise ValueError('point not on curve')
 if y%2!=raw[0]%2:y=AUTH.FIELD-y
 return raw.hex(),(x,y)


def normalize_context(context):
 exact_fields(context,CTX_FIELDS)
 c={key:context[key] for key in CTX_FIELDS}
 for key in ('version','network','dna_version','parent_height','expected_bits','min_timestamp','max_timestamp'):
  c[key]=integer(c[key],0,(1<<32)-1,unsigned=True)
 if c['version']!=1 or c['dna_version']!=1 or not c['network'] or not c['expected_bits'] or c['parent_height']>2147483646 or c['max_timestamp']<c['min_timestamp']:raise ValueError('context semantics')
 for key in ('genesis','prospect_target','parent_hash'):c[key]=hex32(c[key],normalize=True,nonzero=True)
 return c


def job_preimage(job):
 c=job['context']
 result=(JOB_DOMAIN+le32(job['version'])+le32(job['rule_version'])+bytes.fromhex(job['authority_id'])+bytes.fromhex(job['descriptor_id'])
  +le32(c['version'])+le32(c['network'])+bytes.fromhex(c['genesis'])+le32(c['dna_version'])+bytes.fromhex(c['prospect_target'])+bytes.fromhex(c['parent_hash'])
  +le32(c['parent_height'])+le32(c['expected_bits'])+le32(c['min_timestamp'])+le32(c['max_timestamp'])+bytes.fromhex(job['public_key'])+bytes.fromhex(job['gym_id'])+bytes.fromhex(job['challenge'])+le64(job['issued_at_ms'])+le64(job['expires_at_ms']))
 assert len(result)==328
 return result


def new_job(context,authority_id,descriptor_id_,pub,challenge,issued,require_support=True):
 # require_support=False is PRIVATE oracle counterfactual math, never a runtime
 # API or an accepted job. Corpus labels and refusal checks enforce this split.
 authority_id=hex32(authority_id,normalize=True);descriptor_id_=hex32(descriptor_id_,normalize=True)
 resolved=resolve(descriptor_id_) if require_support else None
 c=normalize_context(context)
 if resolved is not None and resolved['dna_version']!=c['dna_version']:raise ValueError('DNA linkage')
 pub,_=public_key(pub,normalize=True);challenge=hex32(challenge,normalize=True)
 issued=integer(issued,1,MAX_I64-LIFETIME)
 gym=AUTH.gym_id(c,bytes.fromhex(pub))
 if int(gym,16)==0:raise ValueError('existing zero gym refusal')
 job={'version':2,'rule_version':1,'id':'','authority_id':authority_id,'descriptor_id':descriptor_id_,'context':c,'public_key':pub,'gym_id':gym,'challenge':challenge,'issued_at_ms':issued,'expires_at_ms':issued+LIFETIME}
 job['id']=sha(job_preimage(job)).hex()
 return job


def validate_job(job):
 exact_fields(job,JOB_FIELDS)
 if integer(job['version'],0,(1<<32)-1,unsigned=True)!=2 or integer(job['rule_version'],0,(1<<32)-1,unsigned=True)!=1:raise ValueError('job version')
 integer(job['expires_at_ms'],1,MAX_I64)
 value=new_job(job['context'],job['authority_id'],job['descriptor_id'],job['public_key'],job['challenge'],job['issued_at_ms'])
 if not same(job,value):raise ValueError('noncanonical job or immutable mismatch')
 return value


def context_span(raw):
 text=raw.decode('ascii');d=decoder();pos=0
 def ws(p):
  while p<len(text) and text[p] in ' \t\r\n':p+=1
  return p
 pos=ws(pos)+1
 while True:
  pos=ws(pos)
  if text[pos]=='}':break
  key,pos=d.raw_decode(text,pos);pos=ws(pos)
  if text[pos]!=':':raise ValueError('colon')
  pos=ws(pos+1);start=pos;_,pos=d.raw_decode(text,pos)
  if key=='context':return raw[start:pos]
  pos=ws(pos)
  if text[pos]==',':pos+=1
  elif text[pos]!='}':raise ValueError('delimiter')
 raise ValueError('missing context')


def parse_job(raw):
 value=decode(raw,8192,2)
 if len(context_span(raw))>4096:raise ValueError('nested context byte bound')
 return validate_job(value)


def commitment(job):
 return OLD.commitment_script(job,PROOF.policy_id(job['context']))


def candidate(job,template,payout_address,extra_nonce):
 # Only used with independently regenerated synthetic template inputs below.
 validate_job(job)
 c=job['context'];block=template['block'];txs=block['transactions']
 assert all(c[k]==template['context'][k] for k in CTX_FIELDS if k!='max_timestamp')
 assert c['min_timestamp']<=block['curtime']<=c['max_timestamp']
 assert payout_address==TPL.base58check(0x53,TPL.PAYOUT_HASH160)
 payout=b'\x76\xa9\x14'+TPL.PAYOUT_HASH160+b'\x88\xac'
 script=TPL.script_number_push(c['parent_height']+1)+b'\x08'+le64(extra_nonce)+bytes.fromhex(block['coinbaseaux']['flags'])
 outputs=[(block['coinbasevalue'],payout),(0,commitment(job))]
 witness=any(tx['hash']!=tx['txid'] for tx in txs)
 if witness:
  witness_root,_=TPL.merkle([bytes(32)]+[bytes.fromhex(tx['hash'])[::-1] for tx in txs])
  digest=TPL.sha256d(witness_root+bytes(32))
  assert block['default_witness_commitment']==digest.hex()
  outputs.append((0,b'\x6a\x24\xaa\x21\xa9\xed'+digest))
 full,stripped=TPL.transaction([(bytes(32),0xffffffff,script,0xffffffff)],outputs,[[bytes(32)]] if witness else None)
 root,branch=TPL.merkle([TPL.sha256d(stripped)]+[bytes.fromhex(tx['txid'])[::-1] for tx in txs])
 header=le32(block['version'])+bytes.fromhex(c['parent_hash'])[::-1]+root+le32(block['curtime'])+le32(c['expected_bits'])+le32(0)
 encoded=header+TPL.compact_size(len(txs)+1)+full+b''.join(bytes.fromhex(tx['data']) for tx in txs)
 base=len(header)+len(TPL.compact_size(len(txs)+1))+len(stripped)+sum((tx['weight']-len(bytes.fromhex(tx['data'])))//3 for tx in txs)
 proof={'version':2,'gym_id':job['gym_id'],'header':header.hex(),'coinbase':stripped.hex(),'merkle_branch':branch,'job_id':job['id']}
 result={'version':1,'job_id':job['id'],'block_hex':encoded.hex(),'proof':proof,'payout_address':payout_address,'coinbase_value':block['coinbasevalue'],'transaction_count':len(txs)+1,'weight':3*base+len(encoded),'sigop_cost':4}
 return result,{'expected_merkle_root':root[::-1].hex(),'expected_coinbase_txid':TPL.sha256d(stripped)[::-1].hex(),'expected_serialized_size':len(encoded),'expected_base_size':base,'expected_payout_script':payout.hex()}


def old_parity():
 AUTH.self_check()
 assert TPL.golden_records()==OLD_TEMPLATES
 for item in OLD_JOBS:
  j=item['job'];pre=OLD.job_preimage(j);policy=PROOF.policy_id(j['context'])
  assert pre.hex()==item['job_id_preimage'] and sha(pre).hex()==j['id']
  assert OLD.commitment_script(j,policy).hex()==item['commitment_script']
  digest=OLD.admission_digest(j,item['proof'],policy)
  assert digest.hex()==item['expected_digest']
  scalar=int(item['test_only_private_key'],16);r,s=AUTH.sign(scalar,digest)
  assert AUTH.der_signature(r,s).hex()==item['authorization']['signature']
  assert AUTH.verify(AUTH.multiply(scalar),digest,r,s)
 # Do not execute old Argon-backed golden_records(): this slice proves codecs,
 # commitment/signature bytes and unmined block assembly, not qualifying work.
 return {'old_jobs':len(OLD_JOBS),'old_templates':len(OLD_TEMPLATES),'old_job_preimages_commitments_authorizations_equal':True,'old_template_objects_equal':True,'old_input_bytes_unchanged':True,'old_production_argon_fixtures_preserved_not_rehashed':True}


def negative_or_positive_inputs(base_descriptor,base_job):
 descriptor_inputs=[];job_inputs=[]
 def add(collection,name,raw,parser):
  try:value=parser(raw)
  except (ValueError,TypeError,KeyError,IndexError,UnicodeError,json.JSONDecodeError):value=None
  collection.append({'name':name,'input_hex':raw.hex(),'accepted':value is not None,'expected':value})
 db=descriptor_bytes(base_descriptor);jb=canon(base_job)
 add(descriptor_inputs,'canonical',db,parse_descriptor)
 for key in DESC_FIELDS:
  for kind in ('missing','null','duplicate','case_alias'):
   if kind=='duplicate':raw=b'{'+canon(key)+b':'+canon(base_descriptor[key])+b','+db[1:]
   else:
    d=base_descriptor.copy()
    if kind=='missing':del d[key]
    elif kind=='null':d[key]=None
    else:d[key.upper()]=d.pop(key)
    raw=canon(d)
   add(descriptor_inputs,key+'_'+kind,raw,parse_descriptor)
 for name,raw in [('empty',b''),('null',b'null'),('array',b'[]'),('pretty',pretty(base_descriptor)),('reverse_order',canon(dict(reversed(list(base_descriptor.items()))))),('leading_space',b' '+db),('newline',db+b'\n'),('trailing_object',db+b'{}'),('unknown',db[:-1]+b',"extra":1}'),('escaped_name',db.replace(b'"version"',b'"ver\\u0073ion"',1)),('escaped_value',db.replace(b'slug-fighter',b'\\u0073lug-fighter',1)),('escaped_duplicate',b'{"ver\\u0073ion":1,'+db[1:]),('depth2',db.replace(b'"version":1',b'"version":{"x":1}',1)),('at_limit_padding_noncanonical',db+b' '*(2048-len(db))),('over_limit',db+b' '*(2049-len(db))),('invalid_utf8',db+b'\xff')]:add(descriptor_inputs,name,raw,parse_descriptor)
 for token in (b'0',b'2',b'-1',b'1.0',b'1e0',b'4294967296',b'true',b'"1"'):
  add(descriptor_inputs,'version_token_'+token.decode(),db.replace(b'"version":1',b'"version":'+token,1),parse_descriptor)
 for key in HASH_FIELDS:
  d=base_descriptor.copy();d[key]=d[key].upper();add(descriptor_inputs,key+'_uppercase',canon(d),parse_descriptor)
 add(job_inputs,'canonical',jb,parse_job)
 for key in JOB_FIELDS:
  for kind in ('missing','null','duplicate','case_alias','escaped_duplicate'):
   if kind in ('duplicate','escaped_duplicate'):
    encoded=canon(key)
    if kind=='escaped_duplicate':encoded=b'"\\u%04x'%ord(key[0])+key[1:].encode()+b'"'
    raw=b'{'+encoded+b':'+canon(base_job[key])+b','+jb[1:]
   else:
    j=copy.deepcopy(base_job)
    if kind=='missing':del j[key]
    elif kind=='null':j[key]=None
    else:j[key.upper()]=j.pop(key)
    raw=canon(j)
   add(job_inputs,key+'_'+kind,raw,parse_job)
 reordered=dict(reversed(list(base_job.items())));reordered['context']=dict(reversed(list(base_job['context'].items())))
 escaped_hex=jb.replace(('"authority_id":"'+base_job['authority_id'][0]).encode(),('"authority_id":"\\u%04x'%ord(base_job['authority_id'][0])).encode(),1)
 positives=[('pretty_reordered',pretty(reordered)),('leading_trailing_whitespace',b' \n\t'+jb+b' \r\n'),('escaped_name',jb.replace(b'"version"',b'"ver\\u0073ion"',1)),('escaped_hex_value',escaped_hex),('whole_size_8192',jb+b' '*(8192-len(jb)))]
 context=canon(base_job['context'])
 at_context=b'{'+b' '*(4096-len(context))+context[1:]
 positives.append(('context_size_4096',jb.replace(context,at_context,1)))
 for name,raw in positives:
  add(job_inputs,name,raw,parse_job);assert job_inputs[-1]['accepted'],name
 for name,raw in [('empty',b''),('root_null',b'null'),('root_array',b'[]'),('trailing_object',jb+b'{}'),('trailing_garbage',jb+b'x'),('unknown',jb[:-1]+b',"extra":1}'),('whole_size_8193',jb+b' '*(8193-len(jb))),('context_size_4097',jb.replace(context,b'{'+b' '*(4097-len(context))+context[1:],1)),('depth3',jb.replace(context,b'{"version":{"nested":1}}',1)),('invalid_utf8',jb+b'\xff')]:add(job_inputs,name,raw,parse_job)
 for token in (b'0',b'1',b'3',b'-0',b'-1',b'2.0',b'2e0',b'4294967296',b'true',b'"2"'):
  add(job_inputs,'version_token_'+token.decode(),jb.replace(b'"version":2',b'"version":'+token,1),parse_job)
 for token in (b'0',b'-1',b'9223372036854775808',b'1794787200000.0',b'1794787200000e0',b'true',b'"1794787200000"'):
  marker=('"issued_at_ms":'+str(base_job['issued_at_ms'])).encode();add(job_inputs,'issued_token_'+token.decode(),jb.replace(marker,b'"issued_at_ms":'+token,1),parse_job)
 for key in CTX_FIELDS:
  for kind in ('missing','null','duplicate'):
   if kind=='duplicate':changed=b'{'+canon(key)+b':'+canon(base_job['context'][key])+b','+context[1:]
   else:
    d=base_job['context'].copy()
    if kind=='missing':del d[key]
    else:d[key]=None
    changed=canon(d)
   add(job_inputs,'context_'+key+'_'+kind,jb.replace(context,changed,1),parse_job)
 for key in ('id','authority_id','descriptor_id','public_key','gym_id','challenge'):
  j=copy.deepcopy(base_job);j[key]=j[key].upper();add(job_inputs,key+'_uppercase',canon(j),parse_job)
 # V1 input must never be auto-upgraded by the new typed reader.
 add(job_inputs,'old_job_v1',canon(OLD_JOBS[0]['job']),parse_job)
 return descriptor_inputs,job_inputs


def structural_cases():
 cases=[]
 def add(name,changes,accepted):
  value=CANDIDATE.copy();value.update(changes)
  try:
   raw=descriptor_bytes(value);identifier=descriptor_id(value)
   assert parse_descriptor(raw)==value
   assert normalize_descriptor(normalize_descriptor(value))==value
  except (ValueError,TypeError):raw=None;identifier=None
  assert (raw is not None)==accepted,name
  cases.append({'name':name,'input':value,'accepted':accepted,'expected':value if accepted else None,'canonical':raw.decode() if raw is not None else None,'id':identifier,'supported_candidate':accepted and identifier==descriptor_id(CANDIDATE),'production_available':False})
 add('candidate',{},True)
 add('zero_digest_shapes',{key:'00'*32 for key in HASH_FIELDS},True)
 for key in VERSIONS:
  add(key+'_zero',{key:0},False)
  add(key+'_uint32_max',{key:(1<<32)-1},key!='version')
  add(key+'_uint32_overflow',{key:1<<32},False)
  if key!='version':add(key+'_unknown_positive',{key:2},True)
 for key in ('generator_id','renderer_id'):
  for suffix,value,accepted in [('one','A',True),('length96','A'*96,True),('length97','A'*97,False),('all_ascii_classes','Az09_-',True),('empty','',False),('space','a b',False),('slash','a/b',False),('dot','a.b',False),('unicode','slug\u00e9',False),('leading_space',' '+CANDIDATE[key],False)]:add(key+'_'+suffix,{key:value},accepted)
 for key in HASH_FIELDS:
  for suffix,value in [('uppercase',CANDIDATE[key].upper()),('short',CANDIDATE[key][:-1]),('long',CANDIDATE[key]+'0'),('nonhex','g'*64),('space',' '+CANDIDATE[key]),('null',None)]:add(key+'_'+suffix,{key:value},False)
 return cases


def constructor_cases(base,unsupported_id):
 original={key:copy.deepcopy(base[key]) for key in ('context','authority_id','descriptor_id','public_key','challenge','issued_at_ms')}
 cases=[]
 def add(name,changes,accepted):
  value=copy.deepcopy(original);value.update(changes)
  try:expected=new_job(value['context'],value['authority_id'],value['descriptor_id'],value['public_key'],value['challenge'],value['issued_at_ms'])
  except (ValueError,TypeError,KeyError):expected=None
  assert (expected is not None)==accepted,name
  cases.append({'name':name,'input':value,'accepted':accepted,'expected':expected})
 add('candidate',{},True)
 for name,value,accepted in [('minimum',1,True),('maximum',MAX_I64-LIFETIME,True),('zero',0,False),('negative',-1,False),('expiry_overflow_one',MAX_I64-LIFETIME+1,False),('signed_max',MAX_I64,False)]:add('issued_'+name,{'issued_at_ms':value},accepted)
 add('zero_authority_challenge',{'authority_id':'00'*32,'challenge':'00'*32},True)
 add('unsupported_descriptor',{'descriptor_id':unsupported_id},False)
 add('zero_descriptor',{'descriptor_id':'00'*32},False)
 for key in ('authority_id','descriptor_id','challenge'):
  for suffix,value in [('short',original[key][:-1]),('long',original[key]+'0'),('nonhex','g'*64),('whitespace',' '+original[key])]:add(key+'_'+suffix,{key:value},False)
 for suffix,value in [('short',original['public_key'][:-2]),('uncompressed','04'+original['public_key'][2:]+'00'*32),('coordinate_overflow','02'+'ff'*32),('off_curve','02'+'00'*32),('bad_prefix','05'+original['public_key'][2:]),('whitespace',' '+original['public_key'])]:add('public_key_'+suffix,{'public_key':value},False)
 for key,value in [('version',0),('version',2),('network',0),('dna_version',0),('dna_version',2),('parent_height',2147483647),('expected_bits',0),('min_timestamp',original['context']['max_timestamp']+1)]:
  context=copy.deepcopy(original['context']);context[key]=value;add('context_'+key+'_'+str(value),{'context':context},False)
 for key in ('genesis','prospect_target','parent_hash'):
  context=copy.deepcopy(original['context']);context[key]='00'*32;add('context_'+key+'_zero',{'context':context},False)
 return cases


def corpus():
 parity=old_parity();candidate_id=descriptor_id(CANDIDATE)
 unsupported=CANDIDATE.copy();unsupported['renderer_id']='slug-fighter-portrait-unsupported-test-v1';unsupported['renderer_contract_hash']=sha(b'SLUG unsupported renderer counterfactual fixture\x00v1\x00').hex()
 unsupported_id=descriptor_id(unsupported);assert candidate_id!=unsupported_id
 descriptors=[{'name':'compiled-candidate','descriptor':CANDIDATE.copy(),'canonical':descriptor_bytes(CANDIDATE).decode(),'id':candidate_id,'supported_candidate':True,'production_available':False},{'name':'unsupported-counterfactual','descriptor':unsupported,'canonical':descriptor_bytes(unsupported).decode(),'id':unsupported_id,'supported_candidate':False,'production_available':False}]
 jobs=[]
 def add_job(name,job,supported=True):
  jobs.append({'name':name,'job':job,'canonical':canon(job).decode(),'job_id_preimage':job_preimage(job).hex(),'commitment_script':commitment(job).hex(),'supported_candidate':supported})
 for i,item in enumerate(OLD_JOBS):
  old=item['job'];authority=bytes(range(64+i*32,96+i*32)).hex()
  add_job('public-key-'+str(i+1),new_job(old['context'],authority,candidate_id,old['public_key'],old['challenge'],old['issued_at_ms']))
 base=jobs[0]['job']
 add_job('minimum-time-zero-namespace-challenge',new_job(base['context'],'00'*32,candidate_id,base['public_key'],'00'*32,1))
 add_job('maximum-issuance',new_job(base['context'],base['authority_id'],candidate_id,base['public_key'],base['challenge'],MAX_I64-LIFETIME))
 alternate_authority='ff'*32
 changed_authority=new_job(base['context'],alternate_authority,candidate_id,base['public_key'],base['challenge'],base['issued_at_ms'])
 add_job('authority-only-change',changed_authority)
 hypothetical=new_job(base['context'],base['authority_id'],unsupported_id,base['public_key'],base['challenge'],base['issued_at_ms'],require_support=False)
 add_job('unsupported-counterfactual',hypothetical,False)
 try:validate_job(hypothetical)
 except ValueError:pass
 else:raise AssertionError('counterfactual accepted operationally')
 candidates=[];authorizations=[]
 for item in OLD_TEMPLATES:
  old=item['job'];job=new_job(old['context'],base['authority_id'],candidate_id,old['public_key'],old['challenge'],old['issued_at_ms'])
  expected,metadata=candidate(job,item['template'],item['payout_address'],item['extra_nonce'])
  candidates.append({'name':item['name'],'job':job,'template':copy.deepcopy(item['template']),'payout_address':item['payout_address'],'extra_nonce':item['extra_nonce'],'expected':expected,**metadata})
  policy=PROOF.policy_id(job['context']);digest=OLD.admission_digest(job,expected['proof'],policy);r,s=AUTH.sign(1,digest)
  assert AUTH.verify(AUTH.GENERATOR,digest,r,s)
  authorizations.append({'name':item['name'],'public_test_scalar':1,'job':job,'proof':expected['proof'],'authorization':{'version':2,'public_key':job['public_key'],'signature':AUTH.der_signature(r,s).hex()},'expected_digest':digest.hex(),'work_verified':False,'issued':False})
 add_job('payable-template-context',candidates[0]['job'])
 descriptor_inputs,job_inputs=negative_or_positive_inputs(CANDIDATE,base)
 job_inputs.append({'name':'unsupported_counterfactual','input_hex':canon(hypothetical).hex(),'accepted':False,'expected':None})
 normalization=[]
 upper=CANDIDATE.copy()
 for key in HASH_FIELDS:upper[key]=upper[key].upper()
 assert normalize_descriptor(upper)==CANDIDATE
 normalization.append({'name':'three_hashes_uppercase','kind':'descriptor','input':upper,'accepted':True,'expected':CANDIDATE.copy(),'supported_candidate':True})
 changed=CANDIDATE.copy();changed['generator_id']=changed['generator_id'].upper()
 normalization.append({'name':'identifier_case_preserved_unknown','kind':'descriptor','input':changed,'accepted':True,'expected':normalize_descriptor(changed),'supported_candidate':False})
 upper_context=base['context'].copy()
 for key in ('genesis','prospect_target','parent_hash'):upper_context[key]=upper_context[key].upper()
 constructor={'context':upper_context,'authority_id':base['authority_id'].upper(),'descriptor_id':base['descriptor_id'].upper(),'public_key':base['public_key'].upper(),'challenge':base['challenge'].upper(),'issued_at_ms':base['issued_at_ms']}
 assert new_job(constructor['context'],constructor['authority_id'],constructor['descriptor_id'],constructor['public_key'],constructor['challenge'],constructor['issued_at_ms'])==base
 normalization.append({'name':'job_constructor_uppercase_hex','kind':'job_constructor','input':constructor,'accepted':True,'expected':base})
 mutations=[]
 for key in JOB_FIELDS:
  if key=='context':continue
  j=copy.deepcopy(base)
  if type(j[key]) is int:j[key]+=1
  else:j[key]=('0' if j[key][0]!='0' else '1')+j[key][1:]
  try:validate_job(j)
  except (ValueError,TypeError):pass
  else:raise AssertionError('job mutation accepted '+key)
  mutations.append({'name':'job_'+key,'job':j,'accepted':False})
 for key in CTX_FIELDS:
  j=copy.deepcopy(base)
  if type(j['context'][key]) is int:j['context'][key]+=1
  else:j['context'][key]=('0' if j['context'][key][0]!='0' else '1')+j['context'][key][1:]
  try:validate_job(j)
  except (ValueError,TypeError):pass
  else:raise AssertionError('context mutation accepted '+key)
  mutations.append({'name':'context_'+key,'job':j,'accepted':False})
 resolver_inputs=[]
 for name,id_ in [('candidate',candidate_id),('unsupported',unsupported_id),('zero','00'*32),('uppercase',candidate_id.upper()),('short',candidate_id[:-1])]:
  for production in (False,True):
   try:expected=resolve(id_,production);error=None
   except ValueError as e:expected=None;error=str(e)
   resolver_inputs.append({'name':name+('_production' if production else '_candidate'),'id':id_,'production':production,'accepted':expected is not None,'expected':expected,'error_category':error})
 assert commitment(base)!=commitment(hypothetical) and commitment(base)!=commitment(changed_authority)
 assert base['id']!=hypothetical['id'] and base['id']!=changed_authority['id']
 for record in jobs:
  if record['supported_candidate']:assert parse_job(record['canonical'].encode())==record['job']
 for entry in descriptor_inputs:
  if entry['name']!='canonical':assert not entry['accepted'],entry['name']
 positives={'canonical','pretty_reordered','leading_trailing_whitespace','escaped_name','escaped_hex_value','whole_size_8192','context_size_4096'}
 for entry in job_inputs:assert entry['accepted']==(entry['name'] in positives),entry['name']
 structural=structural_cases();constructors=constructor_cases(base,unsupported_id)
 return {'preview_only':True,'oracle_version':1,'approved_contract_sha256':CONTRACT_HASH,'descriptor_domain_hex':DESCRIPTOR_DOMAIN.hex(),'job_domain_hex':JOB_DOMAIN.hex(),'job_preimage_bytes':328,'provenance':PROVENANCE,'descriptors':descriptors,'jobs':jobs,'candidates':candidates,'authorizations':authorizations,'descriptor_inputs':descriptor_inputs,'job_inputs':job_inputs,'structural_descriptor_cases':structural,'constructor_cases':constructors,'normalization_cases':normalization,'mutation_cases':mutations,'resolver_inputs':resolver_inputs,'counterfactual':{'same_dna_version':True,'same_context_public_key_gym_challenge_times':True,'candidate_descriptor_id':candidate_id,'unsupported_descriptor_id':unsupported_id,'candidate_job_id':base['id'],'unsupported_hypothetical_job_id':hypothetical['id'],'different_commitments':True,'unsupported_operationally_accepted':False},'old_parity':parity,'validation':{'descriptor_cases':len(descriptors),'job_cases':len(jobs),'candidate_cases':len(candidates),'authorization_cases':len(authorizations),'descriptor_transport_cases':len(descriptor_inputs),'job_transport_cases':len(job_inputs),'structural_descriptor_cases':len(structural),'constructor_cases':len(constructors),'normalization_cases':len(normalization),'mutation_cases':len(mutations),'resolver_cases':len(resolver_inputs),'header_pow_computed':False,'production_activation':False}}


def main():
 parser=argparse.ArgumentParser(description=__doc__);group=parser.add_mutually_exclusive_group(required=True)
 group.add_argument('--generate',action='store_true');group.add_argument('--check',action='store_true');args=parser.parse_args()
 result=corpus();raw=pretty(result);target=HERE/'manifest_job_v1.json'
 if args.generate:target.write_bytes(raw)
 elif target.read_bytes()!=raw:raise SystemExit('FAIL: independent regeneration differs')
 print(json.dumps({'result':'pass','mode':'generate' if args.generate else 'check','corpus_sha256':sha(raw).hex(),'candidate_descriptor_id':result['descriptors'][0]['id'],'validation':result['validation']},sort_keys=True))


if __name__=='__main__':main()
