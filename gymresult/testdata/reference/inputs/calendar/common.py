"""Pinned inputs, exact domains and Gregorian/POSIX-tail calendar, independent of Go45."""
import copy,hashlib,json,re,struct
from pathlib import Path
from crypto import H,ed_public,ed_sign,ed_verify,pubkey,hash160,u32,u64
HERE=Path(__file__).resolve().parent
J=lambda v:json.dumps(v,ensure_ascii=True,separators=(',',':'),allow_nan=False).encode('ascii')
pretty=lambda v:(json.dumps(v,ensure_ascii=True,indent=2,allow_nan=False)+'\n').encode('ascii')
sha=lambda b:hashlib.sha256(b).hexdigest()
raw=lambda s:bytes.fromhex(s)
clone=copy.deepcopy
lp=lambda b:u32(len(b))+b
domain=lambda s:('SLUG '+s+'\0v1\0').encode('ascii')
label=lambda s:sha(('Set45 independent modeled '+s).encode('ascii'))
SAFE=2**53-1;DATE_MAX=8640000000000000;I32=2**31-1;U32=2**32-1;MAXSAT=21000000*100000000
INPUTS=json.loads((HERE/'input_hashes.json').read_bytes())
for path,meta in INPUTS.items():
 b=(HERE/path).read_bytes();assert len(b)==meta['bytes'] and sha(b)==meta['sha256'],path
CORE='51ddfc57931b1937f918f990a36c050dc9874b2d1a60b069273e314e317523df';HELPER='0abdcfa84e2213bd644ad2bfe43789961c3231ef10841121fa2f039776fb8f7e'
assert INPUTS['inputs/contracts/set45-seed-core-contract-draft.md']['sha256']==CORE
assert INPUTS['inputs/contracts/set45-helper-contract-draft.md']['sha256']==HELPER
SOURCE_PIN=json.loads((HERE/'inputs/source44/pin.json').read_bytes());SOURCE_INDEX=json.loads((HERE/'inputs/source44/index.json').read_bytes());VIEWS=json.loads((HERE/'inputs/source44/views.json').read_bytes())
SOURCE_ID=sha(domain('candidate event store')+lp(J(SOURCE_PIN)));assert SOURCE_ID==SOURCE_INDEX['source_store_id']
RECEIPTS=[json.loads((HERE/('inputs/source44/receipt-%d.json'%i)).read_bytes()) for i in [0,1,10]]
for r,e in zip(RECEIPTS,SOURCE_INDEX['receipts']):
 assert sha(domain('candidate cutoff input')+lp(J(r['input'])))==r['snapshot_digest']==e['snapshot_digest']
 assert sha(domain('candidate cutoff receipt')+lp(J(r)))==e['receipt_digest']
ZERO_VIEW=next(v for v in VIEWS if v['fixture']=='zero256');ZERO=next(p['stable_pin'] for p in SOURCE_PIN['participants'] if p['stable_pin']['identity']['gym_id']==ZERO_VIEW['participant_id'])
NET=ZERO['identity']['network'];GENESIS=ZERO['identity']['genesis']
DECL_SEED=H(domain('candidate seed declaration fixture'));CHECK_SEED=H(domain('candidate seed checkpoint fixture'))
DECL_PUB=ed_public(DECL_SEED);CHECK_PUB=ed_public(CHECK_SEED);FUND_PUB=pubkey(1);FUND_SCRIPT=b'\x00\x14'+hash160(FUND_PUB)
RULES=dict(version=1,target='first_anchor_next_block',floor='genesis_inclusive',carrier='SLUGSEED-v1',finality='exact_target_checkpoint_60',max_step_blocks=8,max_scan_restarts=16,fee_satoshis=1000)
RULE_PRE=domain('candidate seed rules')+lp(J(RULES));RULE_ID=sha(RULE_PRE)
CHAIN_PRE=domain('candidate publication chain')+raw(SOURCE_ID)+raw(ZERO['account_id'])+raw(ZERO['source']['authority_id'])+raw(GENESIS)
PIN=dict(version=1,source_store_id=SOURCE_ID,source_pin=SOURCE_PIN,publisher_gym_id=ZERO['identity']['gym_id'],publisher_account_id=ZERO['account_id'],publisher_source_authority_id=ZERO['source']['authority_id'],publisher_chain_id=sha(CHAIN_PRE),network=NET,genesis=GENESIS,floor_height=0,floor_hash=GENESIS,publisher_public_key=DECL_PUB.hex(),checkpoint_public_key=CHECK_PUB.hex(),funding_public_key=FUND_PUB.hex(),funding_script=FUND_SCRIPT.hex(),rules_id=RULE_ID)
STORE_PRE=domain('candidate seed store')+lp(J(PIN));STORE_ID=sha(STORE_PRE)
# Integer civil-days algorithm with Gregorian leap years, epoch 1970-01-01.
def days(y,m,d):
 y-=m<=2;era=y//400;yo=y-era*400;mp=m+(-3 if m>2 else 9);doy=(153*mp+2)//5+d-1;doe=yo*365+yo//4-yo//100+doy
 return era*146097+doe-719468
def civil(z):
 z+=719468;era=z//146097;doe=z-era*146097;yo=(doe-doe//1460+doe//36524-doe//146096)//365;y=yo+era*400;doy=doe-(365*yo+yo//4-yo//100);mp=(5*doy+2)//153;d=doy-(153*mp+2)//5+1;m=mp+(3 if mp<10 else -9);y+=m<=2;return y,m,d
def dateparts(s):
 if type(s)is not str or not re.fullmatch('[0-9]{4}-[0-9]{2}-[0-9]{2}',s):raise ValueError('seed_calendar')
 y,m,d=map(int,s.split('-'))
 if not(2026<=y<=2099 and 1<=m<=12 and 1<=d<=31) or civil(days(y,m,d))!=(y,m,d) or (days(y,m,d)+4)%7!=6:raise ValueError('seed_calendar')
 return y,m,d
def sunday(y,m,n):return 1+(-(days(y,m,1)+4))%7+7*(n-1)
def noon_ms(y,m,d):
 # At noon, US tail rule is DST from second Sunday March through day before first November Sunday.
 dst=(m>3 or m==3 and d>=sunday(y,3,2)) and (m<11 or m==11 and d<sunday(y,11,1))
 return (days(y,m,d)*86400+12*3600+(7 if dst else 8)*3600)*1000
def cutoff(s):return noon_ms(*dateparts(s))
def deadline(s):return noon_ms(*civil(days(*dateparts(s))+7))
def event(s):
 ep=SOURCE_PIN['participants'][0];C=cutoff(s);N=deadline(s)
 pre=domain('candidate Saturday event')+raw(ep['namespace_id'])+raw(ep['calendar_id'])+raw(ep['rules_id'])+s.encode()+u64(C);entry=sha(pre)
 pre=domain('candidate cutoff event')+raw(SOURCE_ID)+raw(entry)+u64(C)+u64(N)
 return dict(version=1,event_id=sha(pre),source_event_id=entry,store_id=SOURCE_ID,namespace_id=ep['namespace_id'],calendar_id=ep['calendar_id'],rules_id=SOURCE_PIN['rules_id'],saturday=s,cutoff_ms=C,incident_deadline_ms=N)
def event_pre(e):return domain('candidate seed event')+u32(NET)+raw(GENESIS)+raw(SOURCE_ID)+raw(e['event_id'])+DECL_PUB
def event_key(e):return sha(event_pre(e))
def declaration(receipt):
 e=receipt['input']['event']
 return dict(version=1,event_key=event_key(e),store_id=STORE_ID,source_store_id=SOURCE_ID,event=e,snapshot_digest=receipt['snapshot_digest'],receipt_digest=sha(domain('candidate cutoff receipt')+lp(J(receipt))),publisher_chain_id=PIN['publisher_chain_id'],network=NET,genesis=GENESIS,floor_height=0,floor_hash=GENESIS,rules_id=RULE_ID,publisher_public_key=DECL_PUB.hex(),checkpoint_public_key=CHECK_PUB.hex())
def decl_id(d):return sha(domain('candidate seed declaration id')+lp(J(d)))
def signed(d):return dict(version=1,declaration=clone(d),signature=ed_sign(DECL_SEED,domain('candidate seed declaration')+lp(J(d))).hex())
def checkpoint(height,hash_):
 c=dict(version=1,network=NET,genesis=GENESIS,height=height,hash=hash_,signature='')
 c['signature']=ed_sign(CHECK_SEED,checkpoint_pre(c)).hex();return c
def checkpoint_pre(c):return b'SLUG signed checkpoint\0'+u32(c['version'])+u32(c['network'])+raw(c['genesis'])[::-1]+u32(c['height'])+raw(c['hash'])[::-1]
def scan_initial(key,generation=0,funding=False):
 return sha(domain('candidate funding scan' if funding else 'candidate seed scan')+raw(STORE_ID)+(b'' if funding else raw(key))+u32(generation)+u32(0)+raw(GENESIS))
def scan_step(prior,height,hash_,funding=False):return sha(domain('candidate funding scan step' if funding else 'candidate seed scan step')+raw(prior)+u32(height)+raw(hash_))
def cursor(key,generation=0):return dict(generation=generation,height=-1,hash='',scan_digest=scan_initial(key,generation),complete_to_height=-1,complete_to_hash='')
def record(receipt=RECEIPTS[0]):
 d=declaration(receipt);t=receipt['completed_at_ms']+1
 return dict(version=1,store_id=STORE_ID,event_key=d['event_key'],source_receipt=clone(receipt),declaration=signed(d),prepared_at_ms=t,publication=None,cursor=cursor(d['event_key']),scan_restarts=[],anchor=None,target=None,checkpoint=None,finality=None,conflicts=[],conflict_overflow=False,incident_at_ms=0,observation=dict(observed_at_ms=0,chain_observed_at_ms=0,tip=None,node_state='unobserved',target_descendants=0,current_finality=False))
def record_digest(r):return sha(domain('candidate seed record')+lp(J(r)))
def calendar():
 out=[]
 for z in range(days(2026,1,1),days(2100,1,1)):
  if(z+4)%7!=6:continue
  s='%04d-%02d-%02d'%civil(z);e=event(s);out.append(dict(saturday=s,cutoff_ms=e['cutoff_ms'],incident_deadline_ms=e['incident_deadline_ms'],next_gap_hours=(e['incident_deadline_ms']-e['cutoff_ms'])//3600000,event=e,event_key=event_key(e)))
 return out
assert event('2026-11-21')==RECEIPTS[0]['input']['event']
