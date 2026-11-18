"""Proof DAG/witness arithmetic fixtures, explicitly not new consensus evidence.
The initial 176 headers/bodies are independently verified retained normal47
public evidence. Added result/funding blocks are unmined structural models;
no producer result or normal-PoW validity is inferred for those additions.
"""
from common import *
import crypto_result as R
import schema_core as S
from reference_results import examples
C=CRYPTO

def tx(inputs,outputs,witness=None):
 ins=b''.join(C.outpoint(h,i)+C.blob(sc)+u32(U32)for h,i,sc in inputs);outs=b''.join(C.txout(v,sc)for v,sc in outputs);body=C.varint(len(inputs))+ins+C.varint(len(outputs))+outs
 if witness is None:return u32(2)+body+u32(0)
 w=b''.join(C.varint(len(stack))+b''.join(C.blob(v)for v in stack)for stack in witness);return u32(2)+b'\x00\x01'+body+w+u32(0)
def parsed(b):
 r=C.Reader(b);t=C.parse_tx(r);assert r.p==len(b);return t

def coinbase(commit=None,witness=None):
 outs=[(0,b'\x51')]
 if commit is not None:outs.append((0,b'\x6a\x24\xaa\x21\xa9\xed'+commit))
 return tx([('00'*32,U32,b'\x01\x01')],outs,witness)

def body(previous,height,transactions):
 root,_=C.merkle([parsed(t)['txid']for t in transactions]);header=u32(4)+raw(previous)[::-1]+raw(root)[::-1]+u32(1795896001+height)+u32(0x207fffff)+u32(height)
 b=header+C.varint(len(transactions))+b''.join(transactions)
 # This modeled digest is deliberately NOT the normal Argon block hash.
 return dict(height=height,hash=sha(b'Set48 unmined header model\0'+header),header=header.hex(),raw_block=b.hex())

def proof_example():
 x=examples();p=x['pin'];original=load('inputs/actual47/public-chain-evidence.json');headers=[dict(height=z['height'],hash=z['hash'],header=z['block_hex'][:160])for z in original];blocks=[]
 funding_tx=tx([(label('unmodeled funding input'),0,b'')],[(100000,raw(R.SCRIPT))]);fund=body(headers[-1]['hash'],176,[coinbase(),funding_tx]);headers.append({k:fund[k]for k in ['height','hash','header']});blocks.append({k:fund[k]for k in ['height','hash','raw_block']})
 coin=dict(outpoint=dict(txid=parsed(funding_tx)['txid'],index=0),value_satoshis=100000,script=R.SCRIPT,block_height=176,block_hash=fund['hash'],coinbase=False);payload=R.carrier(x['statement']['statement']);pub=R.publication(coin,payload);wr,_=C.merkle(['00'*32,pub['wtxid']]);nonce=bytes(32);commit=C.D(raw(wr)[::-1]+nonce);cb=coinbase(commit,[[nonce]])
 incl=body(fund['hash'],177,[cb,raw(pub['raw_tx'])]);headers.append({k:incl[k]for k in ['height','hash','header']});blocks.append({k:incl[k]for k in ['height','hash','raw_block']})
 for h in range(178,238):
  b=body(headers[-1]['hash'],h,[coinbase()]);headers.append({k:b[k]for k in ['height','hash','header']})
 seed=CARD_RECORD['source']['anchor'];b=original[seed['height']];assert b['hash']==seed['hash'];blocks.append(dict(height=b['height'],hash=b['hash'],raw_block=b['block_hex']));blocks.sort(key=lambda b:(b['height'],b['hash']))
 _,txs=R.parse_result_block(raw(incl['raw_block']));_,branch=C.merkle([t['txid']for t in txs],1);a=dict(height=177,hash=incl['hash'],header=incl['header'],txid=pub['txid'],raw_tx_digest=pub['raw_tx_digest'],raw_tx_size=len(raw(pub['raw_tx'])),tx_count=2,tx_index=1,output_index=0,payload=payload.hex(),merkle_branch=branch)
 r=clone(x['records']['submitted']);r['publication'].update(raw_tx=pub['raw_tx'],txid=pub['txid'],coin=coin);now=x['now'];tip=dict(height=237,hash=headers[-1]['hash']);r['inclusion']=dict(version=1,generation=0,anchor=a,observed_at_ms=now+3,observed_tip=tip);r['checkpoint']=R.checkpoint(p,177,a['hash']);r['finality']=dict(checkpoint=clone(r['checkpoint']),inclusion=clone(r['inclusion']),observed_at_ms=now+4,observed_tip=tip,descendants=60);r['observation']=dict(observed_at_ms=now+4,chain_observed_at_ms=now+4,tip=tip,node_state='online',descendants=60,current_finality=True);r['cursor']=R.scan_initial(p,r['result_key'])
 for z in headers[:178]:r['cursor']=R.scan_step(r['cursor'],z['height'],z['hash'])
 status=S.status_from_record(p,r,x['source_status'],'available');proof=dict(version=1,scope='candidate_result_proof_v1',pin=p,result=r,status=status,bout=clone(BOUT_RECORD),card=clone(CARD_RECORD),observed_tip=tip,result_finality_tip=clone(tip),source_finality_tip=clone(CARD_RECORD['source']['finality']['observed_tip']),headers=headers,blocks=blocks)
 return proof

def structural_verify(proof):
 """Full shape/DAG/signature/body/attribution checks EXCLUDING normal Argon.
Do not expose this as a production proof-acceptance or admission verdict.
"""
 v=S.fields('PublicProof',clone(proof));S.req(v['scope']=='candidate_result_proof_v1');p=S.pin(v['pin']);r=S.validate('Record',v['result'],p);S.validate('ReadResult',dict(version=1,record=r,status=v['status']),p);S.req(r['inclusion']is not None and r['publication']is not None);src=r['statement']['statement']['source'];S.req(v['bout']==BOUT_RECORD and v['card']==CARD_RECORD);S.req(src==R.source_from_record(v['bout'],BOUT_SNAPSHOT));S.req(OLD.source_projection(CARD_PIN,v['card'],v['card']['plan']['bouts'][0])==v['bout']['source']);S.req(v['source_finality_tip']==v['card']['source']['finality']['observed_tip']);S.req(v['result_finality_tip']==(r['finality']['observed_tip']if r['finality']else None));S.tip(v['observed_tip']);S.array(v['headers'],1,512);S.array(v['blocks'],1,3);S.req(len(J(v))+1<=24*1024*1024)
 hs={};pairs=[]
 for h in v['headers']:
  h=S.fields('ProofHeader',h);S.integer(h['height'],0,I32);S.hx(h['hash']);S.hexstr(h['header'],80);S.req(h['hash']not in hs);hs[h['hash']]=h;pairs.append((h['height'],h['hash']))
 S.req(pairs==sorted(set(pairs)));gen=[h for h in hs.values()if h['height']==0];S.req(len(gen)==1 and gen[0]['hash']==p['genesis']);orig=load('inputs/actual47/public-chain-evidence.json')[0];S.req(gen[0]['header']==orig['block_hex'][:160])
 for h in hs.values():
  if h['height']:
   parent=raw(h['header'])[4:36][::-1].hex();S.req(parent in hs and hs[parent]['height']==h['height']-1)
 def ancestry(start):
  seen=set();h=hs[start['hash']];S.req(h['height']==start['height'])
  while True:
   seen.add(h['hash'])
   if h['height']==0:return seen
   h=hs[raw(h['header'])[4:36][::-1].hex()]
 required={r['inclusion']['anchor']['hash']:r['inclusion']['anchor']['height'],r['publication']['coin']['block_hash']:r['publication']['coin']['block_height'],v['card']['source']['anchor']['hash']:v['card']['source']['anchor']['height']}
 S.req([(b['height'],b['hash'])for b in v['blocks']]==sorted((h,k)for k,h in required.items()));decoded={}
 for b in v['blocks']:
  b=S.fields('ProofBlock',b);S.hexstr(b['raw_block'],lo=162,hi=8000000);header,txs=R.parse_result_block(raw(b['raw_block']));S.req(b['hash']in hs and hs[b['hash']]['height']==b['height']and hs[b['hash']]['header']==header.hex());decoded[b['hash']]=(header,txs)
 used=set()
 for t in [v['observed_tip'],v['source_finality_tip'],v['result_finality_tip']]+[dict(height=h,hash=k)for k,h in required.items()]:
  if t:used|=ancestry(t)
 S.req(used==set(hs));inc=r['inclusion'];a=inc['anchor'];header,txs=decoded[a['hash']];S.req(len(txs)==a['tx_count']and txs[a['tx_index']]['raw'].hex()==r['publication']['raw_tx']);t=txs[a['tx_index']];S.req(t['outputs'][0]==dict(value=0,script=(b'\x6a\x49'+R.carrier(r['statement']['statement'])).hex()));S.req(a['raw_tx_digest']==sha(t['raw'])and a['raw_tx_size']==len(t['raw']));S.req(a['header']==header.hex())
 coin=r['publication']['coin'];ftxs=decoded[coin['block_hash']][1];matched=[(i,t)for i,t in enumerate(ftxs)if t['txid']==coin['outpoint']['txid']];S.req(len(matched)==1);i,ft=matched[0];S.req(ft['outputs'][coin['outpoint']['index']]==dict(value=coin['value_satoshis'],script=coin['script']));S.req(coin['coinbase']==(i==0));S.req(coin['block_hash']in ancestry(dict(height=a['height'],hash=a['hash'])));S.req(a['height']>=62 and a['height']>coin['block_height']);S.req(not coin['coinbase']or a['height']-coin['block_height']>=100)
 sa=v['card']['source']['anchor'];sh,stxs=decoded[sa['hash']];S.req(sh.hex()==sa['header']and len(stxs)==sa['tx_count']);st=stxs[sa['tx_index']];S.req(st['txid']==sa['txid']and sha(st['raw'])==sa['raw_tx_digest']and len(st['raw'])==sa['raw_tx_size']);S.req(st['outputs'][sa['output_index']]['script']=='6a49'+sa['payload'])
 sf=v['card']['source']['finality'];cp=sf['checkpoint'];sp=CARD_PIN['source_pin'];S.req(C.ed_verify(raw(sp['checkpoint_public_key']),R.checkpoint_bytes(cp),raw(cp['signature'])));S.req(cp['hash']in ancestry(v['source_finality_tip'])and v['source_finality_tip']['height']-cp['height']>=60)
 if r['finality']:S.req(a['hash']in ancestry(v['result_finality_tip'])and v['result_finality_tip']['height']-a['height']>=60)
 return dict(header_count=len(hs),block_count=len(decoded),normal_pow_checked=False,full_utxo_checked=False,source_admission_verified_here=False,limitation='Structural/cryptographic reference only. Unmined added headers require normal Argon verification and are not accepted normal-chain evidence.')

def proof_fragment():
 proof=proof_example();expected=structural_verify(proof);cases=[]
 def test(name,v,accept):
  try:structural_verify(v);actual=True
  except(Exception):actual=False
  assert actual==accept,(name,actual);cases.append(dict(name=name,structural_accept=accept,normal_pow_claimed=False,proof=v if accept else None))
 test('modeled-complete-DAG',proof,True)
 mutations=[('missing-genesis',['headers'],proof['headers'][1:]),('duplicate-header',['headers'],proof['headers']+[proof['headers'][-1]]),('reverse-order',['headers'],list(reversed(proof['headers']))),('missing-funding-block',['blocks'],[b for b in proof['blocks']if b['height']!=176]),('wrong-finality-tip',['result_finality_tip'],proof['source_finality_tip'])]
 for name,path,value in mutations:
  v=clone(proof);v[path[0]]=value;test(name,v,False);cases[-1].pop('proof');cases[-1]['mutation']=dict(path=path,value=value)
 # Keep the original checkpointed result block on its historical path while a
 # separately observed current branch crosses that height.
 fork=clone(proof);prior={h['height']:h for h in proof['headers']};branch=[];parent=prior[176]['hash']
 for height in range(177,238):
  b=body(parent,height,[coinbase()]);branch.append({k:b[k]for k in ['height','hash','header']});parent=b['hash']
 fork['headers']=sorted(proof['headers']+branch,key=lambda h:(h['height'],h['hash']));fork['observed_tip']=dict(height=237,hash=branch[-1]['hash']);r=fork['result'];now=r['observation']['observed_at_ms']+1
 r['conflicts']=[dict(kind='checkpoint_chain_conflict',observed_at_ms=now,expected_height=177,expected_hash=r['inclusion']['anchor']['hash'],observed_hash=branch[0]['hash'],observed_tip=clone(fork['observed_tip']),checkpoint=None)]
 r['observation'].update(observed_at_ms=now,chain_observed_at_ms=now,tip=clone(fork['observed_tip']),descendants=0,current_finality=False);fork['status']=S.status_from_record(fork['pin'],r,fork['status']['source'],'available')
 test('historical-orphan-complete-separate-current-path',fork,True)
 # Funding transaction occurs on a separate orphan branch at the same height.
 unavailable=clone(proof);fb=next(b for b in proof['blocks']if b['height']==176);rawfb=bytearray(raw(fb['raw_block']));rawfb[76:80]=u32(999);fh=sha(b'Set48 unmined header model\0'+bytes(rawfb[:80]));unavailable['headers'].append(dict(height=176,hash=fh,header=bytes(rawfb[:80]).hex()));unavailable['headers'].sort(key=lambda h:(h['height'],h['hash']))
 unavailable['blocks']=[dict(height=176,hash=fh,raw_block=bytes(rawfb).hex())if b['height']==176 else b for b in unavailable['blocks']];unavailable['blocks'].sort(key=lambda b:(b['height'],b['hash']));unavailable['result']['publication']['coin']['block_hash']=fh;unavailable['status']=S.status_from_record(unavailable['pin'],unavailable['result'],unavailable['status']['source'],'available')
 test('original-funding-not-on-inclusion-branch',unavailable,False);cases[-1]['proof']=unavailable;cases[-1]['expected_error']='result_proof_unavailable'
 return dict(version=1,kind='proof',scope='Independent bounded structural model and retained pre48 normal-header inputs; no new normal-PoW positive claimed',proof=proof,structural_result=expected,cases=cases)

def block_controls():
 x=examples();p=proof_example();result=next(b for b in p['blocks']if b['height']==177);good=raw(result['raw_block']);h,ts=R.parse_result_block(good);cases=[]
 def check(name,b,accept):
  try:R.parse_result_block(b);got=True
  except(Exception):got=False
  assert got==accept,(name,got);cases.append(dict(name=name,raw_block=b.hex(),structural_accept=accept,normal_pow_checked=False))
 check('valid-modeled-result-witness',good,True)
 check('zero-transactions',h+b'\x00',False)
 # Commitment presence requires the nonce even if no transaction carries witness.
 cb=coinbase(bytes(32));check('commitment-without-any-witness',raw(body(label('parent'),1,[cb])['raw_block']),False)
 cb=coinbase(bytes(32),[[b'\x00'*31]]);check('commitment-short-nonce',raw(body(label('parent'),1,[cb])['raw_block']),False)
 check('first-not-coinbase',raw(body(label('parent'),1,[tx([(label('noncoinbase'),0,b'')],[(1,b'\x51')])])['raw_block']),False)
 check('extra-later-coinbase',raw(body(label('parent'),1,[coinbase(),coinbase()])['raw_block']),False)
 # Exact txid cannot authenticate altered witness bytes.
 altered=bytearray(ts[1]['raw']);altered[-5]^=1
 check('same-txid-altered-witness',raw(body(h[4:36][::-1].hex(),177,[ts[0]['raw'],bytes(altered)])['raw_block']),False)
 no_commit=coinbase(None,[[bytes(32)]]);check('witness-without-commitment',raw(body(label('parent'),1,[no_commit])['raw_block']),False)
 # Highest-index valid commitment wins; an earlier invalid one is irrelevant.
 nonce=bytes(32);root,_=C.merkle(['00'*32,ts[1]['wtxid']]);commit=C.D(raw(root)[::-1]+nonce)
 cb=tx([('00'*32,U32,b'\x01\x01')],[(0,b'\x6a\x24\xaa\x21\xa9\xed'+bytes(32)),(0,b'\x6a\x24\xaa\x21\xa9\xed'+commit)],[[nonce]])
 check('highest-commitment-valid',raw(body(label('parent'),1,[cb,ts[1]['raw']])['raw_block']),True)
 cb=tx([('00'*32,U32,b'\x01\x01')],[(0,b'\x6a\x24\xaa\x21\xa9\xed'+commit),(0,b'\x6a\x24\xaa\x21\xa9\xed'+bytes(32))],[[nonce]])
 check('highest-commitment-invalid',raw(body(label('parent'),1,[cb,ts[1]['raw']])['raw_block']),False)
 return cases
