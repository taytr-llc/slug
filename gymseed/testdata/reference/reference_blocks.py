"""Independent complete wire blocks and literal normal-Argon work fixtures.
Bodies are serialization/Merkle/normal-work fixtures, NOT a node-accepted chain:
the funding/foreign input references are synthetic and do not prove UTXO validity.
Only the pinned pre45 standalone Argon program supplies the memory-hard primitive.
"""
import argparse,json,os,subprocess
from common import *
from crypto import D,varint,blob,txout,publication,merkle
ARGON='/private/tmp/slug-set45-argon-reference'
BITS=0x207fffff;TARGET=(BITS&0x7fffff)<<(8*((BITS>>24)-3))

def ordinary(script,tag,padding=0):
 op=raw(label('synthetic input '+tag))[::-1]+u32(0)
 outs=txout(0,script)
 if padding:outs+=txout(0,b'\x6a'+b'\0'*padding)
 r=u32(2)+b'\x01'+op+b'\0'+u32(0xffffffff)+varint(2 if padding else 1)+outs+u32(0)
 return dict(raw=r,stripped=r,txid=D(r)[::-1].hex(),wtxid=D(r)[::-1].hex())
def coinbase(height,witnesses):
 script=bytes([3])+height.to_bytes(3,'little')+b'SLUG45';outs=txout(5000000000,b'\x51');reserved=b'\0'*32
 if witnesses:
  root,_=merkle(['00'*32]+witnesses);commit=D(raw(root)[::-1]+reserved);outs+=txout(0,b'\x6a\x24\xaa\x21\xa9\xed'+commit)
 base=u32(2)+b'\x01'+b'\0'*32+u32(0xffffffff)+blob(script)+u32(0xffffffff)+varint(2 if witnesses else 1)+outs+u32(0)
 full=u32(2)+b'\0\x01'+base[4:-4]+b'\x01'+blob(reserved)+u32(0) if witnesses else base
 return dict(raw=full,stripped=base,txid=D(base)[::-1].hex(),wtxid=D(full)[::-1].hex())
def fixture_transactions():
 d=declaration(RECEIPTS[0]);coin=dict(outpoint=dict(txid=label('synthetic confirmed funding output'),index=1),value_satoshis=100000,script=FUND_SCRIPT.hex(),block_height=1,block_hash=label('synthetic funding block'),coinbase=False)
 p=publication(coin,event_key(d['event']),decl_id(d));foreign=ordinary(raw(p['carrier_script']),'earlier foreign carrier',1200)
 local=dict(raw=raw(p['raw_tx']),stripped=raw(p['stripped']),txid=p['txid'],wtxid=p['wtxid'])
 return coin,p,foreign,local

def generate(work_cache,execute=False):
 coin,pub,foreign,local=fixture_transactions();rows=[];previous=GENESIS;used=[]
 def work(header):
  h=header.hex();used.append(h)
  if h not in work_cache:
   assert execute,'missing independently computed normal Argon header '+h
   reply=json.loads(subprocess.check_output([ARGON,h],text=True,timeout=30));assert len(reply)==1 and reply[0]['header']==h
   d=reply[0];assert raw(d['digest'])[::-1].hex()==d['work_hash'];work_cache[h]=d
  return work_cache[h]
 def block(name,height,prev,extras,pad_exact=False):
  witnesses=[t['wtxid'] for t in extras] if any(t['raw']!=t['stripped'] for t in extras) else []
  cb=coinbase(height,witnesses);txs=[cb]+extras;root,_=merkle([t['txid'] for t in txs]);body=varint(len(txs))+b''.join(t['raw'] for t in txs)
  timestamp=1795291200+height*60
  attempts=[]
  for nonce in range(256):
   header=u32(0x20000000)+raw(prev)[::-1]+raw(root)[::-1]+u32(timestamp)+u32(BITS)+u32(nonce);w=work(header);attempts.append(w)
   if int(w['work_hash'],16)<=TARGET:break
  else:raise AssertionError('finite 256-nonce reference search exhausted')
  b=header+body;assert len(b)<=4000000
  if pad_exact:assert len(b)==4000000
  path='fixtures/'+name+'.bin'
  if execute:(HERE/path).write_bytes(b)
  else:assert (HERE/path).read_bytes()==b
  row=dict(name=name,height=height,hash=w['work_hash'],header=header.hex(),previous=prev,merkle_root=root,txids=[t['txid'] for t in txs],file=path,bytes=len(b),sha256=sha(b),nonce=nonce,attempted_headers=[a['header'] for a in attempts])
  rows.append(row);return row,txs
 for height in range(1,66):
  extras=[foreign,local] if height==2 else []
  row,txs=block('chain-%03d'%height,height,previous,extras);previous=row['hash']
  if height==2:
   _,branch=merkle(row['txids'],1);anchor=dict(height=2,hash=row['hash'],header=row['header'],txid=foreign['txid'],raw_tx_digest=sha(foreign['raw']),raw_tx_size=len(foreign['raw']),tx_count=len(txs),tx_index=1,output_index=0,payload=pub['payload'],merkle_branch=branch)
 # Competing canonical branches are independent normal-work fixtures, not consensus reorg executions.
 prebranch,_=block('fork-pre-anchor',1,GENESIS,[ordinary(b'\x6a\x01z','pre-anchor fork')])
 branch,_=block('fork-target',3,rows[1]['hash'],[ordinary(b'\x6a\x01x','target fork')])
 above,_=block('fork-above-target',4,rows[2]['hash'],[ordinary(b'\x6a\x01y','above target fork')])
 # Exactly4MB full block: larger-than-local-cap earlier foreign carrier is not skippable.
 basecb=coinbase(2,[]);probe=ordinary(raw(pub['carrier_script']),'maximum foreign carrier',4000000)
 padding=4000000-(80+1+len(basecb['raw'])+len(probe['raw']))+4000000
 large=ordinary(raw(pub['carrier_script']),'maximum foreign carrier',padding)
 maximum,txs=block('maximum-earlier-carrier',2,rows[0]['hash'],[large],True)
 _,br=merkle(maximum['txids'],1)
 maxanchor=dict(height=2,hash=maximum['hash'],header=maximum['header'],txid=large['txid'],raw_tx_digest=sha(large['raw']),raw_tx_size=len(large['raw']),tx_count=2,tx_index=1,output_index=0,payload=pub['payload'],merkle_branch=br)
 return dict(version=1,claim='Complete wire serialization, txid/witness Merkle construction and independently computed normal Argon threshold only; synthetic input UTXOs, not full consensus/chain admission.',argon_source_sha256=INPUTS['inputs/argon/argon_oracle.go']['sha256'],normal_parameters=dict(time=1,memory_kib=262144,parallelism=1,length=32),bits=BITS,target=hex(TARGET)[2:].zfill(64),genesis_parent=GENESIS,coin=coin,publication=pub,anchor=anchor,maximum_anchor=maxanchor,blocks=rows,work_headers=used)

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--generate',action='store_true');ap.add_argument('--verify-work',action='store_true');a=ap.parse_args();cachepath=HERE/'fixtures/normal_work.json'
 cache=json.loads(cachepath.read_bytes()) if cachepath.exists() else {}
 if a.verify_work:
  for h,w in cache.items():assert json.loads(subprocess.check_output([ARGON,h],text=True,timeout=30))==[w]
  print('normal Argon independently recomputed:',len(cache));return
 index=generate(cache,a.generate)
 outputs={'fixtures/normal_work.json':pretty(dict(sorted(cache.items()))),'fixtures/block_index.json':pretty(index)}
 for p,b in outputs.items():
  if a.generate:(HERE/p).write_bytes(b)
  else:assert(HERE/p).read_bytes()==b,p
 print('normal-work fixture serialization:',len(index['blocks']),'blocks;',len(cache),'Argon headers;',sum(x['bytes'] for x in index['blocks']),'body bytes')
if __name__=='__main__':main()
