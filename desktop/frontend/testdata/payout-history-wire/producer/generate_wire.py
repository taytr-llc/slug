#!/usr/bin/env python3
"""Independent wire recipes, consuming only the frozen independent core model."""
import argparse,json,sys
from pathlib import Path
sys.dont_write_bytecode=True
ap=argparse.ArgumentParser();ap.add_argument('--core-bundle',required=True);ap.add_argument('--out',required=True);args=ap.parse_args()
CORE=Path(args.core_bundle).resolve();sys.path.insert(0,str(CORE/'producer'))
from common import *
from generate import model_history,project
from wire_model import *
ROOT=Path(__file__).resolve().parents[1]
ready=json.loads((CORE/'inputs/original-ready.json').read_bytes());state=json.loads((CORE/'inputs/original-state-public.json').read_bytes())
recipients={r['config']['fixture']:r for r in ready['recipients']}
def make_binding(fixture):
    r=recipients[fixture];cfg=r['config']
    view={'version':1,'view_id':tagged('synthetic-view:'+fixture),'account_authority_id':cfg['account_authority_id'],'account_id':cfg['account_id'],'generation':1,'authority_pin':clone(cfg['source']['authority_pin']),'authority_origin':'http://127.0.0.1:39091','transport_trust_id':loopback_trust('http://127.0.0.1:39091')}
    return {'view':view,'source_binding_id':r['source_binding_id'],'recipient_script':r['recipient']['script']}
B=make_binding('zero256');OTHER=make_binding('incrementing256');JA=tagged('capture-A');JB=tagged('capture-B')
expected={'binding_id':B['source_binding_id'],'gym_id':recipients['zero256']['config']['identity']['gym_id'],'recipient_script':B['recipient_script']}
actual=project(state,recipients['zero256'])
def meta(h):return {'version':1,'count':len(h['items']),'paid_atoms':h['balance']['paid_atoms'],'reserved_atoms':h['balance']['reserved_atoms'],'settlement_observed_at_ms':h['settlement_observed_at_ms'],'captured_at_ms':1795070800000}
C=meta(actual);PAYMENT=actual['items'][0]
def req(cmd,**change):
    v={'id':1,'version':1,'command':cmd,'view_id':'','job_id':'','after':0,'limit':0,'txid':'','output_index':0}
    if cmd!='pool_payout_snapshot':v.update(view_id=B['view']['view_id'],job_id=JA)
    if cmd=='pool_payout_page':v['limit']=8
    if cmd=='pool_payout_wallet':v.update(txid=PAYMENT['txid'],output_index=PAYMENT['output_index'])
    v.update(change);return v
def resp(arm,value,**change):
    r={'id':1,'version':1,'ok':True,'snapshot':None,'job':None,'page':None,'observation':None,'error':''};r[arm]=value;r.update(change);return r
def snap(state='idle',**change):
    v={'version':1,'state':state,'error':'','binding':clone(B),'activity':'none','active_job_id':'','last_job_id':'','capture_job_id':''};v.update(change);return v
def j(state='done',jobid=JA,**change):
    v={'version':1,'job_id':jobid,'state':state,'binding':clone(B),'error':'','capture':clone(C) if state=='done' else None};v.update(change);return v
def pg(h,after,limit):
    rows=clone(h['items'][after:after+limit]);nxt=after+len(rows)
    return {'version':1,'binding':clone(B),'job_id':JA,'capture':meta(h),'after':after,'limit':limit,'next':nxt,'has_more':nxt<len(h['items']),'items':rows}
def obs(state='confirmed',**change):
    v={'version':1,'binding':clone(B),'job_id':JA,'capture':clone(C),'payment':clone(PAYMENT),'observed_at_ms':1795070801000,'state':state,'error':'','wallet_synced':True,'wallet_height':110,'wallet_tip':tagged('wallet-tip'),'confirmations':1 if state=='confirmed' else 0,'block_hash':tagged('wallet-block') if state=='confirmed' else ''}
    if state=='unavailable':v.update(error='wallet_unavailable',wallet_synced=False,wallet_height=0,wallet_tip='',confirmations=0,block_hash='')
    v.update(change);return v
positives=[];negatives=[];raw_cases=[]
def positive(name,kind,v,b=B):
    (request(v) if kind=='request' else response(v,b));raw=canon(v)+b'\n'
    positives.append({'name':name,'kind':kind,'expected_binding':clone(b),'value':clone(v),'frame':raw.decode(),'sha256':sha(raw)})
def negative(name,kind,v,b=B,scope='shape'):
    require(not accepts(request,v) if kind=='request' else not accepts(response,v,b))
    negatives.append({'name':name,'kind':kind,'scope':scope,'expected_binding':clone(b),'value':clone(v),'frame':(canon(v)+b'\n').decode()})
def rn(name,fn,base=None):
    v=clone(req('pool_payout_begin') if base is None else base);fn(v);negative(name,'request',v)
def sn(name,fn,base=None,scope='shape'):
    v=clone(resp('job',j()) if base is None else base);fn(v);negative(name,'response',v,scope=scope)
for cmd in COMMANDS:positive(cmd,'request',req(cmd))
for name,v in [('request_max_id',req('pool_payout_snapshot',id=MAX_SAFE)),('page_end_366',req('pool_payout_page',after=366)),('page_limit_one',req('pool_payout_page',limit=1)),('wallet_index_255',req('pool_payout_wallet',output_index=255))]:positive(name,'request',v)
for name,v in [
 ('snapshot_unavailable',snap('unavailable',error='payout_unavailable',binding=None)),
 ('snapshot_idle_empty',snap()),
 ('snapshot_idle_capture_A',snap(last_job_id=JA,capture_job_id=JA)),
 ('snapshot_failed_B_retains_A',snap(last_job_id=JB,capture_job_id=JA)),
 ('snapshot_refresh_B_retains_A',snap('busy',activity='capture',active_job_id=JB,last_job_id=JA,capture_job_id=JA)),
 ('snapshot_wallet_A',snap('busy',activity='wallet',active_job_id=JA,last_job_id=JA,capture_job_id=JA)),
]:positive(name,'response',resp('snapshot',v))
for name,v in [('job_running_B',j('running',JB)),('job_done_A',j()),('job_cancelled_B',j('cancelled',JB))]:positive(name,'response',resp('job',v))
for err in ['busy','capacity','payout_unavailable']:positive('job_failed_'+err,'response',resp('job',j('failed',JB,error=err)))
for err in ERRORS:positive('error_'+err,'response',resp('job',None,ok=False,error=err))
for n in [0,1,8,9,16,256,366]:
    h=model_history(expected,n)
    for after in sorted({0,max(0,n-8),n}):
        for limit in [1,8]:positive('page_count%d_after%d_limit%d'%(n,after,limit),'response',resp('page',pg(h,after,limit)))
for kind in ['not_observed','unconfirmed','confirmed','unavailable']:positive('wallet_'+kind,'response',resp('observation',obs(kind)))
positive('wallet_max_confirmations','response',resp('observation',obs(wallet_height=2147483647,confirmations=2147483648)))
positive('wallet_max_timestamp','response',resp('observation',obs(observed_at_ms=MAX_TIME)))
other_obs=obs();other_actual=project(state,recipients['incrementing256']);other_obs.update(binding=OTHER,capture=meta(other_actual),payment=other_actual['items'][0]);positive('wallet_other_gym_bound','response',resp('observation',other_obs),OTHER)

rn('unknown_command',lambda v:v.update(command='sendcoins'));rn('unknown_request_field',lambda v:v.update(extra=1));rn('missing_request_field',lambda v:v.pop('txid'))
rn('zero_id',lambda v:v.update(id=0));rn('unsafe_id',lambda v:v.update(id=MAX_SAFE+1));rn('bool_id',lambda v:v.update(id=True));rn('wrong_request_version',lambda v:v.update(version=2))
rn('begin_has_wallet_selector',lambda v:v.update(txid=PAYMENT['txid']));rn('begin_without_job',lambda v:v.update(job_id=''));rn('zero_job',lambda v:v.update(job_id='0'*64));rn('uppercase_view',lambda v:v.update(view_id=v['view_id'].upper()))
for field,value,name in [('limit',0,'page_zero_limit'),('limit',9,'page_nine_limit'),('after',367,'page_after367')]:rn(name,lambda v,f=field,x=value:v.update({f:x}),req('pool_payout_page'))
for field,value,name in [('txid','','wallet_missing_txid'),('output_index',256,'wallet_index256'),('output_index',-1,'wallet_negative_index'),('txid','abc','wallet_bad_txid')]:rn(name,lambda v,f=field,x=value:v.update({f:x}),req('pool_payout_wallet'))
sn('unknown_response_field',lambda v:v.update(extra=1));sn('missing_response_arm',lambda v:v.pop('snapshot'));sn('response_zero_id',lambda v:v.update(id=0));sn('wrong_response_version',lambda v:v.update(version=2));sn('nonboolean_ok',lambda v:v.update(ok=1))
sn('two_success_arms',lambda v:v.update(snapshot=snap()));sn('success_no_arm',lambda v:v.update(job=None));sn('success_has_error',lambda v:v.update(error='busy'));sn('failure_has_payload',lambda v:v.update(ok=False,error='busy'));sn('unknown_error',lambda v:v.update(ok=False,job=None,error='raw private error'))
sn('foreign_binding',lambda v:v['job'].update(binding=OTHER),scope='correlation')
sn('replaced_view_generation',lambda v:v['job']['binding']['view'].update(generation=2),scope='correlation')
sn('different_source',lambda v:v['job']['binding'].update(source_binding_id=tagged('foreign-source')),scope='correlation')
sn('different_recipient',lambda v:v['job']['binding'].update(recipient_script=OTHER['recipient_script']),scope='correlation')
sn('done_without_capture',lambda v:v['job'].update(capture=None));sn('running_has_capture',lambda v:v['job'].update(state='running'));sn('cancelled_has_error',lambda v:v['job'].update(state='cancelled',capture=None,error='busy'))
sn('failed_unknown_error',lambda v:v['job'].update(state='failed',capture=None,error='outcome_unknown'));sn('unknown_job_state',lambda v:v['job'].update(state='complete'));sn('zero_job_id',lambda v:v['job'].update(job_id='0'*64))
sn('capture_excess_count',lambda v:v['job']['capture'].update(count=367));sn('capture_unsafe_date',lambda v:v['job']['capture'].update(captured_at_ms=MAX_TIME+1));sn('capture_report_zero_nonempty',lambda v:v['job']['capture'].update(settlement_observed_at_ms=0));sn('capture_sum_over_money',lambda v:v['job']['capture'].update(paid_atoms=MAX_MONEY,reserved_atoms=1))
sr=resp('snapshot',snap('unavailable',error='payout_unavailable',binding=None))
sn('unavailable_leaks_binding',lambda v:v['snapshot'].update(binding=B),sr);sn('unavailable_leaks_job',lambda v:v['snapshot'].update(last_job_id=JA),sr);sn('unavailable_wrong_error',lambda v:v['snapshot'].update(error='unauthorized'),sr)
sn('idle_has_activity',lambda v:v['snapshot'].update(activity='wallet'),resp('snapshot',snap()))
sn('busy_missing_active',lambda v:v['snapshot'].update(state='busy',activity='capture'),resp('snapshot',snap()))
sn('wallet_wrong_active_capture',lambda v:v['snapshot'].update(state='busy',activity='wallet',active_job_id=JB,capture_job_id=JA),resp('snapshot',snap()))
pr=resp('page',pg(model_history(expected,9),0,8))
sn('page_wrong_next',lambda v:v['page'].update(next=7),pr);sn('page_wrong_more',lambda v:v['page'].update(has_more=False),pr);sn('page_missing_row',lambda v:v['page']['items'].pop(),pr);sn('page_extra_row',lambda v:v['page']['items'].append(clone(v['page']['items'][0])),pr);sn('page_ascending',lambda v:v['page']['items'].reverse(),pr);sn('page_null_items',lambda v:v['page'].update(items=None),pr)
ob=resp('observation',obs())
sn('wallet_confirmed_no_block',lambda v:v['observation'].update(block_hash=''),ob);sn('wallet_confirmed_zero',lambda v:v['observation'].update(confirmations=0),ob);sn('wallet_excess_confirmations',lambda v:v['observation'].update(confirmations=112),ob);sn('wallet_false_sync',lambda v:v['observation'].update(wallet_synced=False),ob);sn('wallet_unknown_state',lambda v:v['observation'].update(state='conflict-free'),ob)
sn('wallet_unavailable_leaks_tip',lambda v:v['observation'].update(wallet_tip=tagged('tip')),resp('observation',obs('unavailable')));sn('wallet_not_observed_has_block',lambda v:v['observation'].update(block_hash=tagged('block')),resp('observation',obs('not_observed')))

def rawcase(name,kind,raw,valid):
    require(accepts(decode,raw,kind,B)==valid)
    raw_cases.append({'name':name,'kind':kind,'valid':valid,'expected_binding':clone(B),'raw_hex':raw.hex()})
base=canon(req('pool_payout_snapshot'))
rawcase('noncanonical_order_allowed','request',canon(dict(reversed(list(req('pool_payout_snapshot').items()))))+b'\n',True)
rawcase('internal_whitespace_allowed','request',base.replace(b'"id":1',b'"id": 1')+b'\n',True)
rawcase('duplicate_request','request',base.replace(b'{"id":1,',b'{"id":1,"id":1,',1),False)
rawcase('negative_zero_selector','request',base.replace(b'"after":0',b'"after":-0'),False)
rawcase('float_id','request',base.replace(b'"id":1',b'"id":1.0'),False)
rawcase('over_request_frame','request',base+b' '*(1025-len(base)),False)
rb=canon(resp('job',j()));rawcase('over_response_frame','response',rb+b' '*(12289-len(rb)),False)
rawcase('invalid_utf8','request',base[:-1]+b'\xff}',False)

scenarios=[
 {'name':'failed_refresh_preserves_old_capture','known_capture':JA,'new_action':JB,'responses':['snapshot_refresh_B_retains_A','job_failed_payout_unavailable','snapshot_failed_B_retains_A','job_done_A'],'expected':'A remains dated history; B failure is explicit and cannot be treated as successful replacement'},
 {'name':'lost_begin_reply_never_adopts_old_capture','known_capture':JA,'new_action':JB,'responses':['error_outcome_unknown','snapshot_idle_capture_A','error_payout_not_found'],'expected':'exact B lookup unresolved/not-started; A is old and never completes B'},
 {'name':'new_capture_replaces_old_selection','old_capture':JA,'new_capture':JB,'expected':'after successful B publication, A page/wallet selectors are rejected; generation/reset rejects both'},
]
atom_formats=[]
for value in [0,1,99999999,100000000,100000001,2970000000,4950000000,MAX_MONEY]:
    whole,fraction=divmod(value,100000000);atom_formats.append({'atoms':value,'text':str(whole)+'.'+str(fraction).zfill(8)})
out={'version':1,'semantics':'Synthetic view/job identities and a public loopback-origin trust digest for wire correlation only; original public source and recipient values retained; no session authenticity or fresh wallet observation claimed.','core_lock_sha256':sha((CORE/'producer-lock.json').read_bytes()),'bindings':{'zero256':B,'incrementing256':OTHER},'job_ids':{'capture_A':JA,'refresh_B':JB},'producer_files':{p.name:sha(p.read_bytes()) for p in sorted((ROOT/'producer').glob('*.py'))},'positives':positives,'negatives':negatives,'raw_cases':raw_cases,'scenarios':scenarios,'atom_formats':atom_formats}
dest=Path(args.out);dest.mkdir(parents=True,exist_ok=False);raw=canon(out);(dest/'wire-v1.json').write_bytes(raw)
report={'version':1,'wire_sha256':sha(raw),'wire_bytes':len(raw),'positives':len(positives),'negatives':len(negatives),'raw_cases':len(raw_cases),'scenarios':len(scenarios),'normal_work_calls':0,'production_calls':0}
(dest/'report.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report))
