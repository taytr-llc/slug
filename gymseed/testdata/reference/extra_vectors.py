"""Additional independently derived bounded transitions and arithmetic controls."""
from common import *
from schema import validate,Refusal
from reference_state import C,CREATED,DEADLINE,BLOCKS
from crypto import publication,verify_merkle,merkle

def initialization():
 # These are abstract crash states, never actual filesystem/kill evidence.
 cases=[]
 def c(name,disk,action,now,expected,error='',changed=False):cases.append(dict(name=name,kind='modeled_initialization_not_actual_filesystem',disk=disk,action=action,now_ms=now,expected=expected,error=error,writes=changed))
 empty=dict(marker=False,init=False,metadata=False,created_at_ms=0,event_count=0,reservations=0,inventory=0)
 c('create_refuse_invalid_pin_before_effects',empty,'Create invalid Pin',C-100,empty,'invalid_request')
 c('create_refuse_cheap_profile_before_effects',empty,'Create cheap profile',C-100,empty,'seed_unavailable')
 marker={**empty,'marker':True};init={**marker,'init':True};ready={**marker,'metadata':True,'created_at_ms':C-100}
 c('after_marker_sync',marker,'Open',C-100,marker,'seed_corrupt')
 c('resume_marker_only_samples_after_pin',marker,'ResumePrepared',C-90,{**ready,'created_at_ms':C-90},changed=True)
 c('after_init_sync',init,'Open',C-80,init,'seed_corrupt')
 c('resume_init_samples_after_pin',init,'ResumePrepared',C-70,{**ready,'created_at_ms':C-70},changed=True)
 c('lost_ready_ack_preserves_creation',ready,'ResumePrepared',C+10,ready)
 c('empty_initialized_reopen_preserves_creation',ready,'Open',C+20,ready)
 c('late_resume_cannot_backdate',init,'ResumePrepared',C+1,{**ready,'created_at_ms':C+1},changed=True)
 c('late_created_prepare_refuses',{**ready,'created_at_ms':C},'Prepare',C+100,{**ready,'created_at_ms':C},'seed_created_too_late')
 for field in ['event_count','reservations','inventory']:
  disk={**ready,field:1};c('resume_nonempty_'+field,disk,'ResumePrepared',C+30,disk,'seed_corrupt')
 c('ready_parent_missing_component',empty,'Parent ready Open',C+40,empty,'seed_corrupt')
 c('unknown_init_key_retained',{**init,'unknown_key':True},'ResumePrepared',C+50,{**init,'unknown_key':True},'seed_corrupt')
 return cases

def selection():
 # Different high bytes intentionally distinguish display-order from internal-wire order.
 coins=[dict(outpoint=dict(txid=x,index=i),value_satoshis=v,script=FUND_SCRIPT.hex(),block_height=h,block_hash=label('selection block '+str(h)),coinbase=cb) for x,i,v,h,cb in [('01'+'00'*31,2,100000,1,False),('00'*31+'02',1,1294,1,False),('00'*31+'02',0,1293,1,False),('00'*31+'03',0,200000,2,True)]]
 cases=[]
 for T in [99,100,101]:
  for reserved in [[],[coins[1]['outpoint']],[c['outpoint'] for c in coins]]:
   eligible=[c for c in coins if c['outpoint']not in reserved and c['value_satoshis']>=1294 and(c['block_height']<=T if not c['coinbase'] else T+1-c['block_height']>=100)]
   eligible.sort(key=lambda c:(c['outpoint']['txid'],c['outpoint']['index']))
   cases.append(dict(tip_height=T,coins=coins,reserved=reserved,expected_outpoint=eligible[0]['outpoint']if eligible else None,error=''if eligible else'seed_waiting_for_funds'))
 return cases

def carrier_cases():
 p=bytes.fromhex(BLOCKS['publication']['payload']);exact=b'\x6a\x49'+p
 vals=[('exact',exact,True),('pushdata1_nonminimal',b'\x6a\x4c\x49'+p,False),('trailing_opcode',exact+b'\0',False),('wrong_magic',exact[:2]+b'x'+exact[3:],False),('wrong_version',exact[:10]+b'\x02'+exact[11:],False),('short',exact[:-1],False),('long',exact+b'x',False),('unknown_event',exact[:11]+b'\0'*32+exact[43:],False)]
 return [dict(name=n,script=s.hex(),expected_payload=p.hex(),matches_known_declaration=ok)for n,s,ok in vals]

def fold_cases():
 out=[];key=event_key(event('2026-11-21'))
 for funding in [False,True]:
  for g in [0,1,16]:
   h=scan_initial(key,g,funding);steps=[]
   for height,hash_ in [(0,GENESIS),(1,BLOCKS['blocks'][0]['hash']),(2,BLOCKS['blocks'][1]['hash'])]:
    before=h;pre=domain('candidate funding scan step'if funding else'candidate seed scan step')
    pre+=raw(before)+u32(height)+raw(hash_);h=scan_step(h,height,hash_,funding);assert h==sha(pre);steps.append(dict(height=height,hash=hash_,preimage=pre.hex(),digest=h))
   pre=domain('candidate funding scan'if funding else'candidate seed scan')+raw(STORE_ID)+(b''if funding else raw(key))+u32(g)+u32(0)+raw(GENESIS)
   out.append(dict(kind='funding'if funding else'event',generation=g,event_key=''if funding else key,initial_preimage=pre.hex(),initial_digest=scan_initial(key,g,funding),steps=steps))
 return out
