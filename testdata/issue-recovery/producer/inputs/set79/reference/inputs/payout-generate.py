#!/usr/bin/env python3
"""Frozen-before-execution Set67 core/transaction/wallet oracle.

Runs only stdlib models and separately extracted original public inputs.
No application, implementation, database, key, node or network is opened.
"""
import sys
sys.dont_write_bytecode=True
import argparse,json
from pathlib import Path
from common import *
from transaction import *
from wallet import wallet_read

ROOT=Path(__file__).resolve().parents[1]
def load(name):return json.loads((ROOT/'inputs'/name).read_bytes())
def model_history(expected,n=0,pending=None):
    start=1735689600000
    rows=[]
    for i in range(n):
        ms=start+i*86400000
        rows.append({'day':date(ms),'created_at_ms':ms,'txid':tagged('row:'+str(i)),'output_index':i%256,'atoms':MIN_MONEY,'recorded_state':'confirmed'})
    rows.reverse()
    if pending and rows:rows[0]['recorded_state']=pending
    reserved=MIN_MONEY if pending and rows else 0
    b={'gym_id':expected['gym_id'],'confirmed':frac(reserved),'available':frac(0),'reserved_atoms':reserved,'paid_atoms':n*MIN_MONEY-reserved,'reason':'payout-pending' if reserved else ''}
    return {'version':1,'recipient_script':expected['recipient_script'],'balance':b,'settlement_observed_at_ms':start+max(0,n-1)*86400000+1000,'items':rows}
def project(state,recipient):
    gym=recipient['config']['identity']['gym_id'];out=[]
    for batch in reversed(state['batches']):
        for index,p in enumerate(batch['payments']):
            if p['gym_id']==gym:
                require(p['script']==recipient['recipient']['script'])
                out.append({'day':batch['day'],'created_at_ms':batch['created_at_ms'],'txid':batch['txid'],'output_index':index,'atoms':p['atoms'],'recorded_state':batch['state']})
    bs=[b for b in state['report']['balances'] if b['gym_id']==gym];require(len(bs)==1)
    return {'version':1,'recipient_script':recipient['recipient']['script'],'balance':clone(bs[0]),'settlement_observed_at_ms':state['report']['observed_at_ms'],'items':out}
def envelope(h,e):return {'version':1,'binding_id':e['binding_id'],'history':h}
def selected(row,script):return {'txid':row['txid'],'output_index':row['output_index'],'atoms':row['atoms'],'recipient_script':script}
def main():
    arg=argparse.ArgumentParser();arg.add_argument('--out',required=True);args=arg.parse_args()
    state,summary,ready=load('original-state-public.json'),load('original-summary.json'),load('original-ready.json')
    binding=sha(b'SLUG pool service source\0v1\0'+canon(state['source']))
    require(binding==summary['source_binding_id']==ready['source_binding_id'])
    expected={};actual={}
    for recipient in ready['recipients']:
        fixture=recipient['config']['fixture'];require(recipient['config']['source']==state['source'])
        expected[fixture]={'binding_id':binding,'gym_id':recipient['config']['identity']['gym_id'],'recipient_script':recipient['recipient']['script']}
        actual[fixture]=project(state,recipient)
        require(actual[fixture]['balance']['paid_atoms']==summary['paid_atoms'][fixture])
    positives=[];negatives=[];raws=[]
    def positive(name,h,e,origin='modeled'):
        history(h,e['gym_id']);r=envelope(h,e);raw=canon(r)+b'\n';response(raw,e)
        positives.append({'name':name,'origin':origin,'expected':clone(e),'history':clone(h),'response_json':canon(r).decode(),'http_body':raw.decode(),'http_sha256':sha(raw)})
    def negative(name,h,e):
        require(not accepts(history,h,e['gym_id']))
        negatives.append({'name':name,'expected':clone(e),'history':clone(h)})
    def rawcase(name,raw,e,valid=False):
        require(accepts(response,raw,e)==valid)
        raws.append({'name':name,'valid':valid,'expected':clone(e),'raw_hex':raw.hex()})
    for fixture,h in actual.items():positive('actual_'+fixture,h,expected[fixture],'original_retained_journal')
    e=expected['zero256'];a=actual['zero256']
    for n in [0,1,8,9,16,256,366]:positive('modeled_count_'+str(n),model_history(e,n),e)
    for pending in ['prepared','broadcast']:positive('modeled_latest_'+pending,model_history(e,9,pending),e)
    h=model_history(e);h['settlement_observed_at_ms']=0;positive('initial_configured',h,e)
    h=model_history(e);h['recipient_script']='';positive('unconfigured_empty',h,e)
    h=model_history(e);h['recipient_script']='';h['balance'].update(confirmed=frac(7),reason='destination-unavailable');positive('unconfigured_entitlement',h,e)
    huge=10**4095;h=model_history(e);h['balance'].update(confirmed=frac(Fraction(huge+1,huge)),available=frac(Fraction(huge+1,huge)),reason='below-minimum');positive('four_4096_digit_fractions',h,e)
    h=model_history(e,1);h['items'][0].update(day='9999-12-31',created_at_ms=MAX_TIME,output_index=255,atoms=MAX_MONEY);h['balance']['paid_atoms']=MAX_MONEY;h['settlement_observed_at_ms']=MAX_TIME;positive('maximum_scalars',h,e)
    for reason in REASONS:
        h=model_history(e);h['balance'].update(confirmed=frac(1),reason=reason);positive('reason_'+(reason or 'empty'),h,e)
    def hn(name,fn,base=None):
        h=clone(a if base is None else base);fn(h);negative(name,h,e)
    hn('unknown_field',lambda h:h.update(extra=1));hn('missing_items',lambda h:h.pop('items'))
    hn('null_items',lambda h:h.update(items=None));hn('null_balance',lambda h:h.update(balance=None))
    hn('wrong_version',lambda h:h.update(version=2));hn('boolean_version',lambda h:h.update(version=True))
    hn('foreign_gym',lambda h:h['balance'].update(gym_id=expected['incrementing256']['gym_id']))
    hn('uppercase_gym',lambda h:h['balance'].update(gym_id=h['balance']['gym_id'].upper()))
    hn('empty_recipient_with_payment',lambda h:h.update(recipient_script=''))
    hn('wrong_script_prefix',lambda h:h.update(recipient_script='0020'+'a'*40))
    hn('uppercase_script',lambda h:h.update(recipient_script=h['recipient_script'].upper()))
    hn('paid_sum_mismatch',lambda h:h['balance'].update(paid_atoms=h['balance']['paid_atoms']+1))
    hn('reserved_sum_mismatch',lambda h:h['balance'].update(reserved_atoms=1))
    hn('unknown_reason',lambda h:h['balance'].update(reason='secret'))
    hn('zero_confirmed_nonempty_reason',lambda h:h['balance'].update(reason='immature'))
    hn('fraction_nonreduced',lambda h:h['balance'].update(confirmed={'numerator':'0','denominator':'2'}))
    hn('fraction_leading_zero',lambda h:h['balance'].update(confirmed={'numerator':'00','denominator':'1'}))
    hn('fraction_zero_denominator',lambda h:h['balance'].update(confirmed={'numerator':'0','denominator':'0'}))
    hn('fraction_negative',lambda h:h['balance'].update(confirmed={'numerator':'-1','denominator':'1'}))
    hn('fraction_4097_digits',lambda h:h['balance'].update(confirmed={'numerator':'1','denominator':'1'+'0'*4096}))
    hn('fraction_over_money',lambda h:h['balance'].update(confirmed=frac(MAX_MONEY+1)))
    hn('available_above_confirmed',lambda h:h['balance'].update(available=frac(1)))
    hn('date_disagrees',lambda h:h['items'][0].update(day='2026-11-18'))
    hn('date_invalid',lambda h:h['items'][0].update(day='2026-02-30'))
    hn('created_zero',lambda h:h['items'][0].update(created_at_ms=0))
    hn('created_over_range',lambda h:h['items'][0].update(created_at_ms=MAX_TIME+1))
    hn('report_before_creation',lambda h:h.update(settlement_observed_at_ms=h['items'][0]['created_at_ms']-1))
    hn('report_zero_with_history',lambda h:h.update(settlement_observed_at_ms=0))
    hn('report_over_range',lambda h:h.update(settlement_observed_at_ms=MAX_TIME+1))
    for field,value,name in [('output_index',256,'index_256'),('output_index',-1,'negative_index'),('output_index',True,'boolean_index'),('atoms',MIN_MONEY-1,'below_minimum'),('atoms',MAX_MONEY+1,'over_money'),('atoms',1.0,'float_atoms'),('recorded_state','pending','unknown_state')]:hn(name,lambda h,f=field,v=value:h['items'][0].update({f:v}))
    hn('uppercase_txid',lambda h:h['items'][0].update(txid=h['items'][0]['txid'].upper()))
    two=model_history(e,2)
    hn('duplicate_txid',lambda h:h['items'][1].update(txid=h['items'][0]['txid']),two)
    hn('duplicate_date',lambda h:h['items'][1].update(day=h['items'][0]['day'],created_at_ms=h['items'][0]['created_at_ms']),two)
    hn('ascending_rows',lambda h:h['items'].reverse(),two)
    hn('older_nonconfirmed',lambda h:(h['items'][1].update(recorded_state='prepared'),h['balance'].update(confirmed=frac(MIN_MONEY),reserved_atoms=MIN_MONEY,paid_atoms=MIN_MONEY,reason='payout-pending')),two)
    hn('excess_rows',lambda h:h.update(items=model_history(e,367)['items']),two)
    hn('unconfigured_wrong_reason',lambda h:h.update(recipient_script=''),{**model_history(e),'balance':{**model_history(e)['balance'],'confirmed':frac(1),'reason':'immature'}})
    raw=canon(envelope(a,e))
    rawcase('ascii_padding',b' \t\r\n'+raw+b'\n',e,True)
    rawcase('exact_131072_byte_body',raw+b' '*(131072-len(raw)),e,True)
    rawcase('over_131072_byte_body',raw+b' '*(131073-len(raw)),e)
    rawcase('unicode_padding',b'\xc2\xa0'+raw,e)
    rawcase('duplicate_root_key',raw.replace(b'{"version":1,',b'{"version":1,"version":1,',1),e)
    rawcase('internal_whitespace',raw.replace(b'"version":1',b'"version": 1',1),e)
    rawcase('trailing_value',raw+b'{}',e)
    rawcase('escaped_key',raw.replace(b'"version"',b'"versi\\u006fn"',1),e)
    rawcase('negative_zero',raw.replace(b'"reserved_atoms":0',b'"reserved_atoms":-0',1),e)
    rawcase('integer_exponent',raw.replace(b'"reserved_atoms":0',b'"reserved_atoms":0e0',1),e)
    rawcase('wrong_source',canon({**envelope(a,e),'binding_id':'f'*64}),e)
    rawcase('wrong_root_order',canon({'binding_id':e['binding_id'],'version':1,'history':a}),e)

    transaction_cases=[];wallet_cases=[]
    batch=state['batches'][0];actual_raw=bytes.fromhex(batch['raw'])
    def txcase(name,raw,sel,valid,origin='modeled',parse_only=False):
        fn=(lambda:parse_transaction(raw)) if parse_only else (lambda:check_selection(raw.hex(),sel))
        require(accepts(fn)==valid);details=fn() if valid else None
        transaction_cases.append({'name':name,'origin':origin,'valid':valid,'parse_only':parse_only,'raw_hex':raw.hex(),'selection':sel,'expected':details})
    for fixture,h in actual.items():txcase('actual_'+fixture,actual_raw,selected(h['items'][0],h['recipient_script']),True,'original_signed_batch')
    require(parse_transaction(actual_raw)['txid']==batch['txid'])
    sel=selected(a['items'][0],a['recipient_script'])
    legacy=modeled_transaction(e['recipient_script']);witness=modeled_transaction(e['recipient_script'],witness=True)
    for name,b in [('legacy',legacy),('witness_zero_length_item',witness),('256_inputs',modeled_transaction(e['recipient_script'],inputs=256)),('257_outputs',modeled_transaction(e['recipient_script'],outputs=257))]:
        info=parse_transaction(b);s={'txid':info['txid'],'output_index':0,'atoms':MIN_MONEY,'recipient_script':e['recipient_script']};txcase('modeled_'+name,b,s,True)
    for field,value in [('txid','f'*64),('output_index',0),('atoms',sel['atoms']+1),('recipient_script',expected['incrementing256']['recipient_script'])]:txcase('wrong_selection_'+field,actual_raw,{**sel,field:value},False)
    txcase('actual_trailing_byte',actual_raw+b'\0',sel,False)
    for size in [0,1,3,4,5,41,len(actual_raw)-1]:txcase('truncated_'+str(size),actual_raw[:size],sel,False,parse_only=True)
    for name,b in [
        ('noncanonical_input_count',legacy[:4]+b'\xfd\1\0'+legacy[5:]),
        ('zero_outputs',legacy[:46]+b'\0'+legacy[47:]),
        ('noncanonical_output_count',legacy[:46]+b'\xfd\1\0'+legacy[47:]),
        ('noncanonical_script_length',legacy[:55]+b'\xfd\x16\0'+legacy[56:]),
        ('negative_output',legacy[:47]+(-1).to_bytes(8,'little',signed=True)+legacy[55:]),
        ('output_over_money',legacy[:47]+(MAX_MONEY+1).to_bytes(8,'little',signed=True)+legacy[55:]),
        ('invalid_witness_flag',witness[:5]+b'\2'+witness[6:]),
        ('noncanonical_witness_count',witness[:-6]+b'\xfd\1\0'+witness[-5:]),
        ('noncanonical_witness_item_length',witness[:-5]+b'\xfd\0\0'+witness[-4:]),
        ('huge_witness_count',witness[:-6]+b'\xff'+b'\xff'*8+witness[-5:]),
        ('huge_witness_item_length',witness[:-5]+b'\xff'+b'\xff'*8+witness[-4:]),
        ('superfluous_witness',modeled_transaction(e['recipient_script'],witness=True,empty_stack=True)),
        ('257_inputs',modeled_transaction(e['recipient_script'],inputs=257)),
        ('258_outputs',modeled_transaction(e['recipient_script'],outputs=258)),
        ('sum_over_money',modeled_transaction(e['recipient_script'],atoms=MAX_MONEY,outputs=2)),
        ('huge_input_count',legacy[:4]+b'\xff'+b'\xff'*8+legacy[5:]),
        ('huge_script_length',legacy[:55]+b'\xff'+b'\xff'*8+legacy[56:]),
        ('oversize_raw',legacy+b'\0'*100000),
    ]:txcase(name,b,sel,False,parse_only=True)
    before={'synced':True,'height':110,'hash':tagged('wallet-tip')};observed=state['report']['observed_at_ms']+1000
    result={'amount':999999.125,'confirmations':1,'blockhash':tagged('wallet-block'),'blockindex':1,'blocktime':1795070000,'txid':batch['txid'],'walletconflicts':[],'time':1795070000,'timereceived':1795070000,'details':[{'account':'default','amount':-9999.5,'category':'receive','vout':1}],'hex':batch['raw']}
    rpc={'jsonrpc':'1.0','result':result,'error':None,'id':1}
    def walletcase(name,env,valid,pre=None,post=None,selection=None,raw_override=None):
        pre=clone(before if pre is None else pre);post=clone(before if post is None else post);s=clone(sel if selection is None else selection)
        raw=canon(env) if raw_override is None else raw_override
        require(accepts(wallet_read,raw,s,pre,post,observed)==valid)
        expected_read=wallet_read(raw,s,pre,post,observed) if valid else None
        wallet_cases.append({'name':name,'origin':'modeled_RPC_using_original_signed_transaction','valid':valid,'raw_hex':raw.hex(),'selection':s,'before':pre,'after':post,'observed_at_ms':observed,'request_id':1,'expected':expected_read})
    walletcase('confirmed_actual_transaction_modeled_RPC',rpc,True)
    unconfirmed=clone(rpc);unconfirmed['result'].update(confirmations=0,blockhash='');walletcase('unconfirmed',unconfirmed,True)
    missing={'jsonrpc':'1.0','result':None,'error':{'code':-5,'message':'No information for transaction'},'id':1};walletcase('exact_missing',missing,True)
    optional=clone(rpc);optional['result']['fee']=-0.00001;optional['result']['details'][0].update(address='x'*256,involveswatchonly=False,fee=-0.00001);walletcase('permitted_optional_fields',optional,True)
    details=clone(rpc);details['result']['details']*=258;walletcase('258_details',details,True)
    conflicts=clone(rpc);conflicts['result']['walletconflicts']=[tagged('conflict')]*256;walletcase('256_ignored_conflict_entries',conflicts,True)
    walletcase('same_height_new_hash',rpc,False,post={**before,'hash':tagged('other-tip')})
    walletcase('new_height',rpc,False,post={**before,'height':111})
    walletcase('before_unsynced',rpc,False,pre={**before,'synced':False})
    walletcase('after_unsynced',rpc,False,post={**before,'synced':False})
    def wn(name,fn,base=None):
        r=clone(rpc if base is None else base);fn(r);walletcase(name,r,False)
    wn('rpc_other_code',lambda r:r['error'].update(code=-1),missing)
    wn('rpc_other_message',lambda r:r['error'].update(message='not found'),missing)
    wn('rpc_missing_with_result',lambda r:r.update(result={}),missing)
    wn('rpc_wrong_id',lambda r:r.update(id=2));wn('rpc_wrong_version',lambda r:r.update(jsonrpc='2.0'))
    wn('rpc_extra_field',lambda r:r.update(extra=1));wn('rpc_missing_field',lambda r:r.pop('jsonrpc'))
    for field,value,name in [('confirmations',-1,'negative_confirmations'),('confirmations',112,'excess_confirmations'),('confirmations',1.0,'float_confirmations'),('blockhash','','confirmed_no_block'),('txid','f'*64,'wrong_rpc_txid'),('hex',batch['raw'].upper(),'uppercase_raw'),('walletconflicts',None,'null_conflicts'),('details',None,'null_details'),('walletconflicts',[tagged('c')]*257,'257_conflicts'),('details',result['details']*259,'259_details')]:wn(name,lambda r,f=field,v=value:r['result'].update({f:v}))
    wn('unconfirmed_has_block',lambda r:r['result'].update(confirmations=0))
    wn('null_optional_fee',lambda r:r['result'].update(fee=None))
    wn('unknown_detail',lambda r:r['result']['details'][0].update(extra=1))
    wn('long_detail_string',lambda r:r['result']['details'][0].update(account='x'*257))
    wn('negative_detail_vout',lambda r:r['result']['details'][0].update(vout=-1))
    wn('unknown_result',lambda r:r['result'].update(extra=1))
    wn('missing_result_hex',lambda r:r['result'].pop('hex'))
    wr=canon(rpc)
    walletcase('duplicate_result_name',rpc,False,raw_override=wr.replace(b'"txid":',b'"txid":"'+batch['txid'].encode()+b'","txid":',1))
    walletcase('nonfinite_ignored_amount',rpc,False,raw_override=wr.replace(b'999999.125',b'1e999'))
    walletcase('wallet_body_oversize',rpc,False,raw_override=wr+b' '*(524289-len(wr)))
    out={'version':1,'semantics':'Original journal projection is historical; modeled RPC states are neither actual wallet observations nor chain proofs. No signatures/work rerun.','producer_files':{p.name:sha(p.read_bytes()) for p in sorted((ROOT/'producer').glob('*.py'))},'input_files':{p.name:sha(p.read_bytes()) for p in sorted((ROOT/'inputs').glob('*.json'))},'core':{'positives':positives,'negatives':negatives,'raw_cases':raws},'transactions':transaction_cases,'wallet_rpc':wallet_cases}
    dest=Path(args.out);dest.mkdir(parents=True,exist_ok=False)
    raw=canon(out);(dest/'reference-v1.json').write_bytes(raw)
    report={'version':1,'reference_sha256':sha(raw),'reference_bytes':len(raw),'core_positive':len(positives),'core_negative':len(negatives),'core_raw':len(raws),'transaction_cases':len(transaction_cases),'wallet_cases':len(wallet_cases),'production_calls':0,'normal_work_calls':0,'actual_originals':2,'modeled_wallet_observations_only':True}
    (dest/'report.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report))
if __name__=='__main__':main()
