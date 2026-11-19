"""Independent Set68 public wire projections from frozen contracts/inputs."""
import argparse,sys
from pathlib import Path
sys.dont_write_bytecode=True
from model import *
ROOT=Path(__file__).resolve().parent

def build():
    lock=json.loads((ROOT/'producer-lock.json').read_bytes())
    for name,sha in lock['sources'].items():need(H((ROOT/name).read_bytes())==sha)
    input_lock=json.loads((ROOT/'input-lock.json').read_bytes())
    need(H((ROOT/'input-lock.json').read_bytes())==lock['input_lock_sha256'])
    for name,record in input_lock['inputs'].items():need(H((ROOT/name).read_bytes())==record['sha256'])
    ready=json.loads((ROOT/'original-ready.json').read_bytes());core=json.loads((ROOT/'journal-reference-v1.json').read_bytes());fixture=json.loads((ROOT/'fixture-recovery-v1.json').read_bytes())
    addresses={a['address']:a['script_pub_key'] for f in fixture['cases'] for a in f['addresses']}
    recipients={r['config']['fixture']:r for r in ready['recipients']}
    positives=[];negatives=[];raw_cases=[]
    def positive(name,kind,v,b):
        (request(v,addresses) if kind=='request' else response(v,b,addresses));frame=J(v)+b'\n'
        positives.append(dict(name=name,kind=kind,expected_binding=C(b),value=C(v),frame=frame.decode(),sha256=H(frame)))
    def negative(name,kind,v,b,scope='shape'):
        need(not accepts(request,v,addresses) if kind=='request' else not accepts(response,v,b,addresses));negatives.append(dict(name=name,kind=kind,scope=scope,expected_binding=C(b),value=C(v),frame=(J(v)+b'\n').decode()))
    def resp(arm=None,value=None,error=''):
        r=dict(id=1,version=1,ok=not bool(error),snapshot=None,operation=None,review=None,action=None,error=error)
        if arm:r[arm]=value
        return r
    for c in core['cases']:
        name=c['name'];cfg=recipients[name]['config'];pin=C(c['identity']['pin']);pin.update(account_authority_id=cfg['account_authority_id'],account_id=cfg['account_id'],pool_source_binding_id=recipients[name]['source_binding_id'])
        identity=dict(version=1,journal_id=c['identity']['journal_id'],pin=pin);bound=digest(b'SLUG candidate withdrawal binding\0v1\0',identity)
        origin='http://127.0.0.1:39091'
        view=dict(version=1,view_id=H(('public withdrawal wire view:'+name).encode()),account_authority_id=pin['account_authority_id'],account_id=pin['account_id'],generation=1,authority_pin=C(cfg['source']['authority_pin']),authority_origin=origin,transport_trust_id=trust(origin))
        b=dict(view=view,source_binding_id=pin['pool_source_binding_id'],journal_id=identity['journal_id'],withdrawal_binding_id=bound)
        public={}
        for phase in c['phases']:
            op=C(phase['operation']);op['intent']['binding_id']=bound;op['intent_digest']=digest(b'SLUG candidate withdrawal intent\0v1\0',op['intent'])
            if op['draft']:
                op['draft']['binding_id']=bound;op['draft_digest']=digest(b'SLUG candidate withdrawal draft\0v1\0',op['draft'])
            i=op['intent'];iv=dict(destination=i['destination'],recipient_script=i['recipient_script'],amount_atoms=i['recipient_atoms'],fee_rate_atoms_per_kb=i['fee_rate_atoms_per_kb'],fee_limit_atoms=i['max_fee_atoms'])
            dv=None
            if op['draft']:
                d=op['draft'];txid=hashlib.sha256(hashlib.sha256(bytes.fromhex(d['unsigned_tx_hex'])).digest()).digest()[::-1].hex()
                dv=dict(version=1,draft_digest=op['draft_digest'],unsigned_txid=txid,destination=d['recipient'],recipient_script=d['recipient_script'],amount_atoms=d['amount_atoms'],fee_atoms=d['fee_atoms'],total_debit_atoms=d['amount_atoms']+d['fee_atoms'],input_count=len(d['inputs']),input_atoms=d['total_input_atoms'],recipient_output_index=d['recipient_output_index'],change_atoms=d['change']['atoms'],change_output_index=d['change']['output_index'],prepared_at_ms=op['prepared_at_ms'],observed_tip=d['tip_hash'],observed_height=d['tip_height'])
            p=dict(version=1,operation_id=op['operation_id'],journal_id=b['journal_id'],withdrawal_binding_id=bound,sequence=op['sequence'],revision=op['revision'],created_at_ms=i['created_at_ms'],updated_at_ms=op['updated_at_ms'],prepared_at_ms=op['prepared_at_ms'],stage=op['stage'],intent=iv,intent_digest=op['intent_digest'],draft=dv,txid=op['signed']['txid'] if op['signed'] else '',signed_sha256=op['signed']['raw_sha256'] if op['signed'] else '',dispatch=C(op['dispatch']),observation=C(op['observation']))
            public[phase['name']]=p;positive(name+'_'+phase['name'],'response',resp('operation',p),b)
        prepared=public['prepared'];signed=public['signed'];oid=prepared['operation_id'];aid=H(('public action:'+name).encode());rid=H(('public review:'+name).encode());dg=prepared['draft']['draft_digest'];txid=signed['txid']
        def req(cmd,**updates):
            v=dict(id=1,version=1,command=cmd,view_id='',operation_id='',action_id='',review_id='',review_kind='',draft_digest='',txid='',intent=None)
            if cmd!='withdrawal_snapshot':v.update(view_id=view['view_id'],operation_id=oid)
            if cmd not in ['withdrawal_snapshot','withdrawal_operation']:v['action_id']=aid
            if cmd=='withdrawal_prepare':v['intent']=C(prepared['intent'])
            if cmd=='withdrawal_review':v.update(review_id=rid,review_kind='send',draft_digest=dg)
            if cmd=='withdrawal_approve':v.update(review_id=rid,draft_digest=dg)
            if cmd=='withdrawal_abandon':v['draft_digest']=dg
            if cmd=='withdrawal_observe':v['txid']=txid
            v.update(updates);return v
        for cmd in COMMANDS:positive(name+'_'+cmd,'request',req(cmd),b)
        for label,v in [('review_retry',req('withdrawal_review',review_kind='retry',txid=txid)),('approve_retry',req('withdrawal_approve',txid=txid)),('abandon_intent',req('withdrawal_abandon',draft_digest='')),('max_correlation',req('withdrawal_snapshot',id=SAFE))]:positive(name+'_'+label,'request',v,b)
        rev=dict(version=1,binding=C(b),operation_id=oid,action_id=aid,review_id=rid,kind='send',draft=C(prepared['draft']),txid='',reviewed_at_ms=1795000001000,expires_at_ms=1795000301000)
        positive(name+'_send_review','response',resp('review',rev),b);retry=C(rev);retry.update(kind='retry',txid=txid);positive(name+'_retry_review','response',resp('review',retry),b)
        snap=dict(version=1,state='idle',error='',binding=C(b),active_action_id='',last_action_id='',unresolved_operation_id='',latest_operation_id='',can_prepare=True,can_review=True)
        positive(name+'_idle_empty','response',resp('snapshot',snap),b)
        recovered=C(snap);recovered.update(unresolved_operation_id=oid,latest_operation_id=oid,can_prepare=False);positive(name+'_recovered_operation','response',resp('snapshot',recovered),b)
        busy=C(recovered);busy.update(state='busy',active_action_id=aid);positive(name+'_busy','response',resp('snapshot',busy),b)
        unavailable=C(snap);unavailable.update(state='unavailable',error='withdrawal_unavailable',binding=None,can_prepare=False,can_review=False);positive(name+'_unavailable','response',resp('snapshot',unavailable),b)
        for kind in ['prepare','send','retry','abandon','observe']:
            for state in ['running','done','failed','stopped','outcome_unknown']+(['review'] if kind in ['send','retry'] else []):
                a=dict(version=1,binding=C(b),operation_id=oid,action_id=aid,kind=kind,state=state,review_id=rid if kind in ['send','retry'] else '',error='outcome_unknown' if state=='outcome_unknown' else 'busy' if state=='failed' else '',operation=C(public['abandoned_prepared'] if kind=='abandon' else signed if kind in ['retry','observe'] or kind=='send' and state!='review' else prepared))
                positive(name+'_'+kind+'_'+state,'response',resp('action',a),b)
        for err in ERRORS:positive(name+'_error_'+err,'response',resp(error=err),b)
        # Shape failures and separate exact-owner correlation failures.
        tests=[('missing_field',lambda v:v.pop('intent')),('extra_field',lambda v:v.update(extra=1)),('zero_id',lambda v:v.update(id=0)),('boolean_id',lambda v:v.update(id=True)),('unsafe_id',lambda v:v.update(id=SAFE+1)),('zero_operation',lambda v:v.update(operation_id='0'*64)),('raw_transaction_route',lambda v:v.update(command='sendrawtransaction')),('foreign_selector',lambda v:v.update(txid=txid))]
        for label,change in tests:v=req('withdrawal_prepare');change(v);negative(name+'_'+label,'request',v,b)
        for field,value in [('amount_atoms',0),('amount_atoms',MAX+1),('fee_rate_atoms_per_kb',1001),('fee_limit_atoms',100001),('recipient_script','00')]:
            v=req('withdrawal_prepare');v['intent'][field]=value;negative(name+'_bad_'+field+str(value),'request',v,b)
        for field,value in [('total_debit_atoms',prepared['draft']['total_debit_atoms']+1),('change_output_index',prepared['draft']['recipient_output_index']),('input_count',17),('observed_height',2147483648),('prepared_at_ms',0),('fee_atoms',0),('fee_atoms',100001)]:
            v=resp('operation',C(prepared));v['operation']['draft'][field]=value;negative(name+'_draft_'+field+'_'+str(value),'response',v,b)
        for field,value in [('revision',4294967296),('sequence',129),('stage','paid'),('txid','0'*64),('signed_sha256','')]:
            v=resp('operation',C(signed));v['operation'][field]=value;negative(name+'_operation_'+field,'response',v,b)
        for label,change in [('expired',lambda v:v.update(expires_at_ms=v['reviewed_at_ms'])),('over_five_minutes',lambda v:v.update(expires_at_ms=v['reviewed_at_ms']+300001)),('send_has_txid',lambda v:v.update(txid=txid)),('zero_review_id',lambda v:v.update(review_id='0'*64))]:
            v=C(rev);change(v);negative(name+'_review_'+label,'response',resp('review',v),b)
        for field,value in [('journal_id',H(b'foreign journal')),('withdrawal_binding_id',H(b'foreign binding')),('source_binding_id',H(b'foreign source'))]:
            v=resp('review',C(rev));v['review']['binding'][field]=value;negative(name+'_foreign_'+field,'response',v,b,'correlation')
        v=resp('review',C(rev));v['review']['binding']['view']['generation']=2;negative(name+'_foreign_generation','response',v,b,'correlation')
        v=resp('operation',C(public['submitted_modeled_confirmation']));v['operation']['observation']['confirmations']=0;negative(name+'_confirmed_zero','response',v,b)
        v=resp('operation',C(public['submitted_modeled_confirmation']));v['operation']['observation']['block_hash']='';negative(name+'_confirmed_without_block','response',v,b)
        v=resp('operation',C(public['submitted_later_missing_keeps_signed']));v['operation']['observation']['confirmations']=1;negative(name+'_missing_has_confirmations','response',v,b)
        for label,change in [('two_arms',lambda v:v.update(snapshot=C(snap))),('success_error',lambda v:v.update(error='busy')),('failure_payload',lambda v:v.update(ok=False,error='busy')),('unknown_error',lambda v:v.update(ok=False,operation=None,error='private path'))]:
            v=resp('operation',C(prepared));change(v);negative(name+'_'+label,'response',v,b)
        if name=='zero256':
            rq=J(req('withdrawal_snapshot'));rs=J(resp('review',rev))
            recipes=[('reordered_request','request',J(dict(reversed(list(req('withdrawal_snapshot').items()))))+b'\n',True),('duplicate_request','request',rq.replace(b'"id":1',b'"id":1,"id":1',1),False),('float_id','request',rq.replace(b'"id":1',b'"id":1.0',1),False),('negative_zero','request',rq.replace(b'"id":1',b'"id":-0',1),False),('invalid_utf8','request',rq+b'\xff',False),('trailing_value','request',rq+b'{}',False),('duplicate_nested','response',rs.replace(b'"generation":1',b'"generation":1,"generation":1',1),False),('over_request','request',rq+b' '*(2049-len(rq)),False),('over_response','response',rs+b' '*(16385-len(rs)),False)]
            for label,kind,raw,valid in recipes:
                def check():
                    need(len(raw)<=(2048 if kind=='request' else 16384));v=parse(raw);request(v,addresses) if kind=='request' else response(v,b,addresses)
                need(accepts(check)==valid);raw_cases.append(dict(name=label,kind=kind,valid=valid,expected_binding=C(b),raw_hex=raw.hex()))
    return dict(version=1,scope='Finite independent withdrawal wire shapes and exact binding correlation; prospective public fixture transactions and modeled journal/node states only',positives=positives,negatives=negatives,raw_cases=raw_cases)
def main():
    p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);a=p.parse_args();need(not a.output.exists());v=build();raw=json.dumps(v,indent=2,ensure_ascii=False).encode()+b'\n';a.output.write_bytes(raw);print(json.dumps(dict(positives=len(v['positives']),negatives=len(v['negatives']),raw_cases=len(v['raw_cases']),bytes=len(raw),sha256=H(raw))))
if __name__=='__main__':main()
