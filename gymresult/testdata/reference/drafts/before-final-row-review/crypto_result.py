"""Set48 fixed fixture key2 publication and purpose-separated identities.
Pure arithmetic derived from frozen prose; old integer crypto copied verbatim.
Audit/submit verification never signs or reconstructs a signature.
"""
from common import *
C=CRYPTO
RULES=dict(version=1,source='saved_c1_bout_v1',carrier='SLUGRSLT-v1',fund='fixture_key2_p2wpkh',inclusion='exact_local_tx',pre_checkpoint_reorg='same_tx_reinclude',finality='carrier_checkpoint_60',max_step_blocks=8,max_scan_restarts=16,max_prior_inclusions=16,fee_satoshis=1000,awards='none')
RULE_ID=sha(domain('candidate result rules')+lp(J(RULES)))
PUB_SEED=hashlib.sha256(domain('candidate result statement fixture')).digest();CP_SEED=hashlib.sha256(domain('candidate result checkpoint fixture')).digest()
PUB=C.ed_public(PUB_SEED).hex();CP=C.ed_public(CP_SEED).hex();FUND=C.pubkey(2).hex();SCRIPT=(b'\x00\x14'+C.hash160(raw(FUND))).hex()
def bout_store_id(p):return sha(domain('candidate bout store')+lp(J(p)))
def candidate_pin(source):
 # Concrete inherited immutable Seed Pin already selects the zero256 participant.
 seed=source['source_pin']['source_pin'];ep=seed['source_pin'];eid=seed['source_store_id']
 zero=next(p['stable_pin']for p in ep['participants']if p['stable_pin']['account_id']==seed['publisher_account_id'])
 chain=sha(domain('candidate publication chain')+b''.join(raw(x)for x in [eid,zero['account_id'],zero['source']['authority_id'],zero['source']['genesis']]))
 assert chain==seed['publisher_chain_id']
 return dict(version=1,source_store_id=bout_store_id(source),source_pin=clone(source),publisher_gym_id=zero['identity']['gym_id'],publisher_account_id=zero['account_id'],publisher_source_authority_id=zero['source']['authority_id'],publisher_chain_id=chain,network=seed['network'],genesis=seed['genesis'],floor_height=0,floor_hash=seed['genesis'],publisher_public_key=PUB,checkpoint_public_key=CP,funding_public_key=FUND,funding_script=SCRIPT,rules_id=RULE_ID)
def store_id(p):return sha(domain('candidate result store')+lp(J(p)))
def result_key(p,card_id,bout_id):return sha(domain('candidate result key')+u32(p['network'])+b''.join(raw(x)for x in [p['genesis'],p['publisher_chain_id'],p['source_store_id'],card_id,bout_id,p['publisher_public_key']]))
def card_id_for_date(p,saturday):
 cp=p['source_pin']['source_pin'];return OLD.card_id(cp,OLD.event_for(cp,saturday))
def source_digest(s):return sha(domain('candidate result source')+lp(J(s)))
def statement_id(s):return sha(domain('candidate result statement id')+lp(J(s)))
def signing_bytes(s):return domain('candidate result statement')+lp(J(s))
def record_digest(p,r):return sha(domain('candidate result record')+lp(J(r)))
def source_from_record(record,snapshot):
 return dict(version=1,store_id=record['store_id'],store_created_at_ms=snapshot['created_at_ms'],record_digest=OLD.record_digest(record),source_digest=record['source_digest'],input_id=record['input_id'],result_digest=record['result_digest'],created_at_ms=record['created_at_ms'],source=clone(record['source']),summary={k:record['result'][k]for k in ['winner','method','end_round','end_tick']})
def statement(p,source):
 s=source['source'];return dict(version=1,result_key=result_key(p,s['card_id'],s['bout']['bout_id']),store_id=store_id(p),source_store_id=p['source_store_id'],source=clone(source),publisher_chain_id=p['publisher_chain_id'],network=p['network'],genesis=p['genesis'],rules_id=p['rules_id'],publisher_public_key=p['publisher_public_key'],checkpoint_public_key=p['checkpoint_public_key'])
def sign_statement(s):return dict(version=1,statement=clone(s),signature=C.ed_sign(PUB_SEED,signing_bytes(s)).hex())
def carrier(s):return b'SLUGRSLT\x01'+raw(s['result_key'])+raw(statement_id(s))
def checkpoint_bytes(cp):return b'SLUG signed checkpoint\0'+u32(cp['version'])+u32(cp['network'])+raw(cp['genesis'])[::-1]+u32(cp['height'])+raw(cp['hash'])[::-1]
def checkpoint(p,height,h):
 cp=dict(version=1,network=p['network'],genesis=p['genesis'],height=height,hash=h);cp['signature']=C.ed_sign(CP_SEED,checkpoint_bytes(cp)).hex();return cp

def tx_recipe(coin,payload):
 assert len(payload)==73 and payload.startswith(b'SLUGRSLT\x01')and payload[9:41]!=bytes(32)and payload[41:]!=bytes(32)
 assert coin['script']==SCRIPT and 1294<=coin['value_satoshis']<=21000000*100000000
 op=C.outpoint(coin['outpoint']['txid'],coin['outpoint']['index']);seq=u32(U32);keyhash=C.hash160(raw(FUND));carrier_script=b'\x6a\x49'+payload
 outputs=C.txout(0,carrier_script)+C.txout(coin['value_satoshis']-1000,raw(SCRIPT));sc=b'\x76\xa9\x14'+keyhash+b'\x88\xac'
 pre=u32(2)+C.D(op)+C.D(seq)+op+C.blob(sc)+u64(coin['value_satoshis'])+seq+C.D(outputs)+u32(0)+u32(1)
 stripped=u32(2)+b'\x01'+op+b'\x00'+seq+b'\x02'+outputs+u32(0)
 return pre,stripped,sc,carrier_script

def publication(coin,payload):
 pre,stripped,sc,cs=tx_recipe(coin,payload);dg=C.D(pre);sig,r,s,k=C.sign(2,dg)
 witness=b'\x02'+C.blob(sig+b'\x01')+C.blob(raw(FUND));b=u32(2)+b'\x00\x01'+stripped[4:-4]+witness+u32(0)
 return dict(payload=payload.hex(),carrier_script=cs.hex(),script_code=sc.hex(),bip143_preimage=pre.hex(),bip143_digest=dg.hex(),rfc6979_nonce=hex(k)[2:].zfill(64),signature_der=sig.hex(),signature_r=hex(r)[2:].zfill(64),signature_s=hex(s)[2:].zfill(64),stripped=stripped.hex(),raw_tx=b.hex(),txid=C.D(stripped)[::-1].hex(),wtxid=C.D(b)[::-1].hex(),raw_tx_digest=sha(b))
def der_signature(b):
 if len(b)<8 or len(b)>72 or b[0]!=0x30 or b[1]!=len(b)-2:raise Refusal('DER')
 p=2;nums=[]
 for _ in range(2):
  if p+2>len(b)or b[p]!=2:raise Refusal('DER integer')
  n=b[p+1];p+=2;v=b[p:p+n];p+=n
  if not v or len(v)!=n or v[0]&128 or (len(v)>1 and v[0]==0 and not(v[1]&128)):raise Refusal('DER canonical')
  nums.append(int.from_bytes(v,'big'))
 if p!=len(b)or not(1<=nums[0]<C.N and 1<=nums[1]<=C.N//2):raise Refusal('DER bounds')
 return nums

def validate_publication(coin,payload,raw_tx):
 b=raw(raw_tx)
 if not(1<=len(b)<=1024):raise Refusal('tx size')
 reader=C.Reader(b);tx=C.parse_tx(reader)
 if reader.p!=len(b):raise Refusal('tx trailing')
 pre,stripped,sc,cs=tx_recipe(coin,payload)
 if tx['stripped']!=stripped or len(tx['witness'])!=1 or len(tx['witness'][0])!=2:raise Refusal('tx shape')
 signature,pub=map(raw,tx['witness'][0])
 if pub!=raw(FUND)or not signature or signature[-1]!=1:raise Refusal('tx witness')
 r,s=der_signature(signature[:-1])
 if not C.verify(C.mul(2),C.D(pre),r,s):raise Refusal('signature')
 return dict(txid=tx['txid'],wtxid=tx['wtxid'],raw_tx_digest=sha(b),raw_tx_size=len(b))

def parse_result_block(data):
 """Purpose-private witness/coinbase strengthening over unchanged old parser."""
 header,txs=C.parse_block(data);first=txs[0]
 if len(first['inputs'])!=1 or first['inputs'][0]['txid']!='00'*32 or first['inputs'][0]['index']!=U32:raise Refusal('first tx not coinbase')
 if any(len(t['inputs'])==1 and t['inputs'][0]['txid']=='00'*32 and t['inputs'][0]['index']==U32 for t in txs[1:]):raise Refusal('extra coinbase')
 commitments=[raw(x['script'])[6:38]for x in first['outputs']if x['script'].startswith('6a24aa21a9ed')and len(x['script'])>=76]
 if commitments:
  w=first['witness']
  if len(w)!=1 or len(w[0])!=1 or len(w[0][0])!=64:raise Refusal('coinbase witness nonce')
  root,_=C.merkle(['00'*32]+[x['wtxid']for x in txs[1:]])
  if commitments[-1]!=C.D(raw(root)[::-1]+raw(w[0][0])):raise Refusal('witness commitment')
 elif any(x['witness']for x in txs):raise Refusal('unexpected witness')
 return header,txs

def scan_initial(p,key='',generation=0,funding=False):
 d=domain('candidate result funding scan'if funding else'candidate result inclusion scan')+raw(store_id(p))+(b''if funding else raw(key))+u32(generation)+u32(0)+raw(p['genesis'])
 return dict(generation=generation,height=-1,hash='',scan_digest=sha(d),complete_to_height=-1,complete_to_hash='')
def scan_step(cursor,height,h):
 c=clone(cursor);assert height==c['height']+1;c.update(height=height,hash=h,scan_digest=sha(domain('candidate result scan step')+raw(c['scan_digest'])+u32(height)+raw(h)));return c
