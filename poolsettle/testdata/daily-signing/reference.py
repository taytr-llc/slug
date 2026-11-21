import copy,hashlib,json,pathlib,sys
root=pathlib.Path(__file__).resolve().parent
spec={'bindings.json':'9b8e52f41690db7ca1fee37c34f99dfcd0a7266b81a7793859871bd9b006ae30','operator-wallet-v1.json':'c1f2d725fcdbc3af7ba1b0a30f4dd4dfc6983352ffb0b4f72d83f7c09ac7b087'}
data={}
for name,sha in spec.items():
 raw=(root/'inputs'/name).read_bytes();assert hashlib.sha256(raw).hexdigest()==sha;data[name]=json.loads(raw)
def canon(x):return json.dumps(x,separators=(',',':')).encode()
def claim(gym,atoms):return dict(round_hash='11'*32,round_height=62,gym_id=gym,amount=dict(numerator=str(atoms),denominator='1'))
cases=[]
for fixture in data['bindings.json']['positives']:
 binding=fixture['binding'];tx=next(t for t in data['operator-wallet-v1.json']['transactions'] if t['fixture']==fixture['fixture'] and t['shape']['inputs']==1)
 c=tx['coins'][0];gym=binding['settlement']['destinations'][0]['gym_id'];net=c['atoms']*99//100;fee=c['atoms']//100;pay=100000000
 coin=dict(txid=bytes.fromhex(c['hash_wire'])[::-1].hex(),index=c['index'],atoms=c['atoms'],script=c['script'],coinbase=True,block_hash='11'*32,block_height=62,claims=[claim(gym,net),claim('',fee)])
 change=dict(txid='',index=1,atoms=c['atoms']-pay-tx['fee_atoms'],script=c['script'],coinbase=False,block_hash='',block_height=0,claims=[claim(gym,net-pay),claim('',fee-tx['fee_atoms'])])
 body=dict(version=2,config=binding['settlement'],source=binding['source'],inputs=[coin],payments=[dict(gym_id=gym,script=binding['settlement']['destinations'][0]['script'],atoms=pay)],fee_atoms=tx['fee_atoms'],change=change,unsigned=tx['unsigned_hex'])
 for suffix in ['change','null-control']:
  value=copy.deepcopy(body)
  if suffix=='null-control':value['change']=None
  raw=canon(value);cases.append(dict(name=fixture['fixture']+'-'+suffix,body=value,canonical=raw.decode(),digest=hashlib.sha256(b'SLUG operator pool daily signing request\x00v2\x00'+raw).hexdigest()))
worst=claim('f'*64,1);worst['round_hash']='f'*64;worst['round_height']=4294967295;worst['amount']={'numerator':'9'*4096,'denominator':'9'*4096}
claim_bytes=len(canon(worst));assert claim_bytes<9*1024
upper=65536*(9*1024)+1465*512+65536+8192+256*512+800000+1024
assert upper<640*1024*1024
print(json.dumps(dict(scope='conditional byte codec and bounds only',cases=cases,bound=dict(max_claim_bytes=claim_bytes,conservative_body_bytes=upper,limit=640*1024*1024)),indent=2))
