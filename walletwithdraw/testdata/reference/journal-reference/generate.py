"""Independent frozen-contract journal envelopes; public fixture model only."""
import argparse,copy,hashlib,json
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
J=lambda v:json.dumps(v,separators=(',',':'),ensure_ascii=False).encode()
H=lambda b:hashlib.sha256(b).hexdigest()
def digest(domain,value):
    raw=J(value);return H(domain+len(raw).to_bytes(4,'little')+raw)
def check():
    lock=json.loads((ROOT/'journal-reference/producer-lock.json').read_bytes())
    for path,sha in lock['inputs'].items():assert H((ROOT/path).read_bytes())==sha,path
    assert H(Path(__file__).read_bytes())==lock['producer_sha256']
def build():
    check();fixtures={x['name']:x for x in json.loads((ROOT/'reference-inputs/public/fixture-recovery-v1.json').read_bytes())['cases']}
    wallet=json.loads((ROOT/'journal-reference/wallet-reference-v1.json').read_bytes());cases=[]
    for original in wallet['cases']:
        if original['name'] not in ['zero256-original-to-84','incrementing256-original-to-84']:continue
        fixture=original['fixture'];m=fixtures[fixture]['export']['manifest'];journal=H(b'public journal namespace:'+fixture.encode())
        pin=dict(version=1,network=m['network'],genesis=m['genesis'],account_authority_id=H(b'public fixture account authority'),account_id=H(b'public fixture account:'+fixture.encode()),fixture=fixture,wallet_profile=m['profile'],root_public_key=m['root_public_key'],gym_public_key=m['gym_public_key'],gym_id=m['gym_id'],pool_source_binding_id=H(b'public fixture pool source'))
        payload=dict(version=1,journal_id=journal,pin=pin);binding=digest(b'SLUG candidate withdrawal binding\x00v1\x00',payload)
        marker=dict(application='slug-candidate-withdrawal-journal',version=1,journal_id=journal,pin=pin,binding_id=binding)
        draft=copy.deepcopy(original['draft']);draft['binding_id']=binding;dg=digest(b'SLUG candidate withdrawal draft\x00v1\x00',draft)
        clock=1795000000000;opid=draft['operation_id']
        intent=dict(version=1,operation_id=opid,binding_id=binding,created_at_ms=clock,destination=draft['recipient'],recipient_script=draft['recipient_script'],recipient_atoms=draft['amount_atoms'],fee_rate_atoms_per_kb=1000,max_fee_atoms=100000,min_confirmations=1,input_purpose=84,change_purpose=84,account=0)
        ig=digest(b'SLUG candidate withdrawal intent\x00v1\x00',intent)
        op=dict(version=1,sequence=1,operation_id=opid,revision=1,updated_at_ms=clock,prepared_at_ms=0,stage='intent',intent=intent,intent_digest=ig,draft=None,draft_digest='',signed=None,sign_authorization=None,dispatch=None,observation=None)
        phases=[]
        def save(name,operation):
            state=dict(application=marker['application'],version=1,journal_id=journal,binding_id=binding,clock_high_water_ms=operation['updated_at_ms'],records=[copy.deepcopy(operation)])
            phases.append(dict(name=name,operation=copy.deepcopy(operation),canonical_operation=J(operation).decode(),state=state,canonical_state=J(state).decode(),state_sha256=H(J(state))))
        save('intent',op)
        abandoned=copy.deepcopy(op);abandoned.update(revision=2,updated_at_ms=clock+1,stage='abandoned');save('abandoned_intent',abandoned)
        op.update(revision=2,updated_at_ms=clock+1,prepared_at_ms=clock+1,stage='prepared',draft=draft,draft_digest=dg);save('prepared',op)
        abandoned=copy.deepcopy(op);abandoned.update(revision=3,updated_at_ms=clock+2,stage='abandoned');save('abandoned_prepared',abandoned)
        action=dict(action_id=H(b'public send action:'+fixture.encode()),review_id=H(b'public send review:'+fixture.encode()),view_id=H(b'public view:'+fixture.encode()),generation=1,authorized_at_ms=clock+2)
        signed=dict(raw=original['expected_signed']['raw_hex'],txid=original['expected_signed']['txid'],raw_sha256=original['signed_sha256'])
        op.update(revision=3,updated_at_ms=clock+2,stage='signed',signed=signed,sign_authorization=action);save('signed',op)
        direct=copy.deepcopy(op);direct.update(revision=4,updated_at_ms=clock+3,stage='submitted',observation=dict(version=1,txid=signed['txid'],observed_at_ms=clock+3,state='mempool',node_tip=draft['tip_hash'],node_height=draft['tip_height'],confirmations=0,block_hash='',error=''));save('signed_observed_without_app_dispatch',direct)
        dispatchauth=copy.deepcopy(action);dispatchauth['authorized_at_ms']=clock+3
        op.update(revision=4,updated_at_ms=clock+3,stage='dispatching',dispatch=dict(attempt=1,authorization=dispatchauth,intent_at_ms=clock+3,result='pending',result_at_ms=0,error=''));save('dispatch_pending',op)
        op['revision']=5;op['updated_at_ms']=clock+4;op['dispatch'].update(result='unknown',result_at_ms=clock+4,error='node_unavailable');save('dispatch_unknown',op)
        observation=dict(version=1,txid=signed['txid'],observed_at_ms=clock+5,state='mempool',node_tip=draft['tip_hash'],node_height=draft['tip_height'],confirmations=0,block_hash='',error='')
        op.update(revision=6,updated_at_ms=clock+5,stage='submitted',observation=observation);save('submitted_exact_observation_after_lost_ack',op)
        missing=copy.deepcopy(op);missing['revision']=7;missing['updated_at_ms']=clock+6;missing['observation'].update(observed_at_ms=clock+6,state='not_observed');save('submitted_later_missing_keeps_signed',missing)
        confirmed=copy.deepcopy(op);confirmed['revision']=7;confirmed['updated_at_ms']=clock+6;confirmed['observation'].update(observed_at_ms=clock+6,state='confirmed',node_tip=H(b'public modeled confirmation block'),node_height=110,confirmations=1,block_hash=H(b'public modeled confirmation block'));save('submitted_modeled_confirmation',confirmed)
        cases.append(dict(name=fixture,provenance='Modeled application state and node outcomes; original input payout and independently derived prospective transaction only. No claim of actual new withdrawal.',identity=payload,marker=marker,canonical_marker=J(marker).decode(),binding_id=binding,canonical_pin=J(pin).decode(),intent_digest=ig,draft_digest=dg,phases=phases))
    return dict(version=1,scope='Independent frozen journal value grammar, digest domains and modeled lifecycle states; no Go/runtime outputs',cases=cases)
def main():
    p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);a=p.parse_args();assert not a.output.exists()
    value=build();raw=json.dumps(value,indent=2,ensure_ascii=False).encode()+b'\n';a.output.write_bytes(raw);print(json.dumps(dict(cases=len(value['cases']),phase_states=sum(len(c['phases']) for c in value['cases']),bytes=len(raw),sha256=H(raw))))
if __name__=='__main__':main()
