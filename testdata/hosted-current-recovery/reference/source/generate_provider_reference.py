#!/usr/bin/env python3
"""Set71 independent provider/setup corpus. SOURCE ONLY until reviewed lock.

Consumes only the locked inherited public sources and literal fixture recipe.
No Go, current-checkpoint producer output, live account, wallet, key or network.
"""
import argparse
import importlib.util
import json
from pathlib import Path
import sys
sys.dont_write_bytecode = True
import provider_model as m
import marker_model as markers
from common import FIXTURES, Rejected, canonical, own, need, packed, parse, principal_id, sha

INPUT_LOCK_SHA256='f7809a938be24d9a320beeff9f8ed09fb2a43f9245864ca7ab0d4f5bb7a16dd9'
CONTRACT_SHA256='92b247a5c3f47e637ed0dbf09d08c8f580a881cc80ff114a4c22d2e119572a86'
SOURCE_NAMES=('common.py','provider_model.py','marker_model.py','generate_provider_reference.py','provider-case-plan.json')


def load_inputs(root):
    lock_raw=(root.parent/'input-lock-v1.json').read_bytes()
    need(sha(lock_raw)==INPUT_LOCK_SHA256,'input lock')
    lock=json.loads(lock_raw)
    catalog_raw=(root.parent/lock['catalog']).read_bytes()
    need(sha(catalog_raw)==lock['catalog_sha256'],'catalog hash')
    catalog=json.loads(catalog_raw);need(len(catalog['files'])==lock['file_count']==75,'inventory count')
    values={}
    for row in catalog['files']:
        relative=Path(row['path']);need(not relative.is_absolute() and '..' not in relative.parts)
        need(row['path'] not in values,'duplicate input')
        raw=(root/relative).read_bytes()
        need(sha(raw)==row['sha256'] and len(raw)==row['bytes'],'input changed: '+row['path'])
        values[row['path']]=raw
    need(sum(len(v) for v in values.values())==lock['input_bytes'],'input bytes')
    recipe=json.loads(values['fixture-recipe.json'])
    need(sha(values['fixture-recipe.json'])==lock['recipe_sha256'],'recipe hash')
    need(sha(values[recipe['paths']['contract']])==CONTRACT_SHA256==lock['contract_sha256'],'contract hash')
    return lock,recipe,values


def baseline(root,values,recipe):
    # This executes only after the caller's exact reviewed producer lock passes.
    # Reproduce all old identity values before generating any new JWS.
    path='inherited/slug/gymidentity/testdata/reference_identity.py'
    spec=importlib.util.spec_from_file_location('set71_inherited_rfc_reference',root/path)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    previous=module.corpus()
    raw=(json.dumps(previous,indent=2,ensure_ascii=True)+'\n').encode()
    need(raw==values[recipe['paths']['old_identity']],'old identity corpus changed')
    key=json.loads(values[recipe['paths']['rsa']])
    need(key==module.fixture_key(),'old RFC key changed')
    token=module.rfc_self_check()
    return dict(source_sha256=sha(values[path]),old_corpus_sha256=sha(raw),
                old_cases=len(previous['cases']),rfc_signature_reproduced=True,
                rfc_token_sha256=sha(token.encode()))


def sid(recipe,kind,index):
    number=recipe['sequence_starts'][kind]+index
    need(0<number<2**256)
    return format(number,'064x')


def flow(recipe,index,now):
    ids=recipe['ids']
    return dict(version=1,coordinator_id=ids['coordinator'],authority_id=ids['authority'],
                id=sid(recipe,'flow',index),attempt_id=sid(recipe,'attempt',index),issuer=m.ISSUER,
                nonce=sid(recipe,'nonce',index),sequence=index+1,issued_at_ms=now,expires_at_ms=now+300000)


def request(recipe,index,now):
    ids=recipe['ids']
    return dict(version=1,coordinator_id=ids['coordinator'],authority_id=ids['authority'],
                event_id=sid(recipe,'event',index),nonce=sid(recipe,'nonce',2048+index),
                issued_at_ms=now,expires_at_ms=now+300000)


def independent_bindings(recipe,values):
    # Deliberately does not import current_model or any generated expectation.
    result=[];ids=recipe['ids']
    for fixture in FIXTURES:
        manifest=json.loads(values[recipe['paths']['actual_manifests'][fixture]])
        binding=dict(version=1,authority_id=ids['authority'],account_id=ids['accounts'][fixture],
                     principal_id=principal_id(m.ISSUER,fixture),provision_id=ids['provisions'][fixture],
                     network=manifest['network'],genesis=manifest['genesis'],profile=manifest['profile'],
                     root_public_key=manifest['root_public_key'],gym_public_key=manifest['gym_public_key'],gym_id=manifest['gym_id'])
        result.append(dict(version=1,hosted_vault_id=ids['vault'],vault_binding=binding,
                           retention_id=ids['retentions'][fixture],wallet_owner_id=ids['wallets'][fixture],
                           source_binding_id=ids['source_binding']))
    return result


class Corpus:
    def __init__(self,config,coordinator,accounts):
        self.config=config;self.coordinator=coordinator;self.accounts=accounts
        self.records=[];self.tokens=[];self.transitions=[];self.negatives=[];self.raw_cases=[];self.capacities=[]
        self.names=set()

    def name(self,name):
        need(name not in self.names,'duplicate case name');self.names.add(name)

    def record(self,name,kind,value,cap,file=True):
        self.name(name);raw=packed(value,cap,file)
        self.records.append(dict(name=name,kind=kind,value=own(value),canonical_utf8=raw.decode(),bytes=len(raw),sha256=sha(raw)))

    def state(self,name,model):
        state=m.audit_driver(model.state,self.config,self.coordinator,self.accounts)
        self.record(name,'driver_state',state,4194304)

    def transition(self,name,model,action,expected_code=None):
        self.name(name);before=canonical(model.state)
        try:
            result=action()
        except Rejected as error:
            need(expected_code==error.code,'unexpected modeled rejection: '+name+': '+error.code)
            need(canonical(model.state)==before,'rejection mutated model')
            self.transitions.append(dict(name=name,accepted=False,error=error.code,before_sha256=sha(before),after_sha256=sha(before)))
            return None
        need(expected_code is None,'expected rejection did not occur: '+name)
        after=canonical(model.state)
        self.transitions.append(dict(name=name,accepted=True,error='',before_sha256=sha(before),after_sha256=sha(after),result=own(result)))
        return result

    def reject_state(self,name,value,code=None):
        self.name(name)
        try:m.audit_driver(value,self.config,self.coordinator,self.accounts)
        except Rejected as error:
            if code is not None:need(error.code==code)
            self.negatives.append(dict(name=name,kind='driver_state',value=own(value),error=error.code));return
        raise ValueError('negative state accepted: '+name)

    def token(self,name,key,kind,claims,expected,now,accepted=True):
        self.name(name);signed=m.sign_fixture(key,kind,claims)
        try:
            semantic=m.verify_fixture_token(key,kind,signed['compact_jws'],self.config,self.coordinator,expected,now)
            need(accepted,'negative signed token accepted: '+name);error=''
        except Rejected as failure:
            need(not accepted,'positive signed token rejected: '+name);semantic=None;error=failure.code
        self.tokens.append(dict(name=name,kind=kind,expected_request=own(expected),now_ms=now,
                                cryptographically_signed=True,fixture_verifier_accepted=accepted,
                                account_authorized=False,error=error,semantic=semantic,**signed))


def build(root,recipe,values):
    need(tuple(recipe['fixture_order'])==FIXTURES)
    baseline_result=baseline(root,values,recipe)
    key=json.loads(values[recipe['paths']['rsa']]);ids=recipe['ids'];now=recipe['non_second_aligned_ms']
    configs=m.configurations(key,ids['authority'],recipe['policy']);identity,config,vault=configs
    corpus=Corpus(config,ids['coordinator'],ids['accounts'])
    for name,value in [('identity',identity),('coordinator',config),('vault',vault)]:
        corpus.record('config-'+name,'config',value,32768,False)
    corpus.record('normalized-verifier','verifier_config',m.public_config(key),32768,False)
    bindings=independent_bindings(recipe,values)
    descriptor=json.loads(values[recipe['paths']['descriptor']])['id']
    initial=markers.init_record(ids,descriptor,configs,m.verifier_digest(key))
    corpus.record('hosted-init','init',initial,32768)
    for receipt in markers.stage_receipts(initial,bindings):
        corpus.record('stage-'+receipt['stage'],'stage_receipt',receipt['receipt'],8192)
    corpus.record('hosted-ready','ready',markers.ready_record(initial,bindings),32768)
    corpus.record('current-store-marker','marker',dict(application='slug-hosted-current-recovery',version=1,bindings=own(bindings)),32768)
    for field,replacement in [('authority_id','a0'*32),('coordinator_id','a1'*32),('hosted_vault_id','a2'*32),
                              ('mode','local-pool-pilot'),('descriptor_id','a3'*32),('identity_config_sha256','a4'*32)]:
        changed=own(initial);changed[field]=replacement
        name='reject-init-pinned-'+field;corpus.name(name)
        try:markers.validate_pinned(changed,initial,32768)
        except Rejected as error:
            corpus.negatives.append(dict(name=name,kind='pinned_init',expected=own(initial),value=changed,error=error.code))
        else:raise ValueError('foreign init accepted: '+field)
    driver=m.DriverModel(config,ids['coordinator'],ids['accounts'])
    corpus.state('driver-empty',driver)
    auth=[]
    for index,fixture in enumerate(FIXTURES):
        f=flow(recipe,index,now+index*1000)
        row,_=corpus.transition('fresh-'+fixture,driver,lambda f=f,fixture=fixture,index=index:driver.fresh(f,fixture,sid(recipe,'auth',index),now+index*1000))
        auth.append(row)
        corpus.token('login-'+fixture,key,'login',m.login_claims(config,row,now+index*1000),f,now+index*1000)
    corpus.state('driver-two-original-auth',driver)
    corpus.transition('exact-auth-flow-replay',driver,lambda:driver.fresh(auth[0]['flow'],FIXTURES[0],sid(recipe,'auth',999),now+2000))
    corpus.token('login-fresh-transport-same-origin',key,'login',m.login_claims(config,auth[0],now+2000),auth[0]['flow'],now+2000)
    changed=own(auth[0]['flow']);changed['nonce']=sid(recipe,'nonce',999)
    corpus.transition('same-flow-changed-nonce',driver,lambda:driver.fresh(changed,FIXTURES[0],sid(recipe,'auth',3),now+2000),'conflict')
    r=request(recipe,0,now+3000)
    reset,_=corpus.transition('verify-reset',driver,lambda:driver.begin(r,FIXTURES[0],sid(recipe,'operation',0),now+3000))
    corpus.state('driver-reset-verified',driver)
    corpus.record('pending-exact-reset','pending_reset',driver.pending(FIXTURES[0]),4096,False)
    corpus.token('reset-begin',key,'reset',m.reset_claims(config,reset,'begin',now+3000),r,now+3000)
    corpus.transition('pending-refuses-new-login',driver,lambda:driver.fresh(flow(recipe,3,now+4000),FIXTURES[0],sid(recipe,'auth',3),now+4000),'conflict')
    begin=m.reset_result(reset,2,now+4000,7)
    premature=m.reset_result(reset,2,now+4000,7,now+5000,8)
    corpus.transition('complete-before-credential-cas',driver,lambda:driver.ack(r,premature,True,now+5000),'conflict')
    corpus.transition('credential-cas-once',driver,lambda:driver.ack(r,begin,False,now+5000))
    corpus.state('driver-credential-committed',driver)
    corpus.transition('credential-cas-replay',driver,lambda:driver.ack(r,begin,False,now+6000))
    complete=m.reset_result(reset,2,now+4000,7,now+7000,8)
    corpus.token('reset-complete',key,'reset',m.reset_claims(config,reset,'complete',now+7000),r,now+7000)
    corpus.transition('observe-exact-completion',driver,lambda:driver.ack(r,complete,True,now+7000))
    corpus.state('driver-reset-completed',driver)
    corpus.transition('completed-replay-keeps-hold-time',driver,lambda:driver.ack(r,complete,True,now+8000))
    changed=own(complete);changed['barrier']['hold_until_ms']+=1000
    corpus.transition('completed-replay-cannot-extend-hold',driver,lambda:driver.ack(r,changed,True,now+8000),'conflict')
    f=flow(recipe,4,now+9000)
    fresh,_=corpus.transition('fresh-login-new-epoch',driver,lambda:driver.fresh(f,FIXTURES[0],sid(recipe,'auth',4),now+9000))
    corpus.token('login-new-epoch',key,'login',m.login_claims(config,fresh,now+9000),f,now+9000)
    corpus.state('driver-login-after-reset',driver)
    # Signature validity does not make old original auth acceptable to account.
    corpus.token('signed-old-epoch-after-reset-no-account-claim',key,'login',m.login_claims(config,auth[0],now+10000),auth[0]['flow'],now+10000)
    corpus.token('reset-old-verified-time-fresh-transport',key,'reset',m.reset_claims(config,reset,'complete',now+86401000),r,now+86401000)
    for field,value in [('authority_id','f0'*32),('coordinator_id','f1'*32),('tenant_id','other-tenant'),('aud',m.RESET_AUD),('nonce','f2'*32),('auth_time',now//1000+100)]:
        claims=m.login_claims(config,auth[0],now+10000);claims[field]=value
        corpus.token('reject-login-'+field,key,'login',claims,auth[0]['flow'],now+10000,False)
    for field,value in [('event_id','f3'*32),('reset_nonce','f4'*32),('to_epoch',3),('stage','other')]:
        claims=m.reset_claims(config,reset,'complete',now+10000);claims[field]=value
        corpus.token('reject-reset-'+field,key,'reset',claims,r,now+10000,False)
    # New reset under the second fixture is independently retained, then retired
    # only from the explicitly modeled expired/no-barrier observation.
    r2=request(recipe,1,now+11000)
    corpus.transition('second-fixture-verify-reset',driver,lambda:driver.begin(r2,FIXTURES[1],sid(recipe,'operation',1),now+11000))
    corpus.transition('retire-before-expiry-refused',driver,lambda:driver.retire(r2,'issued',False,now+12000),'conflict')
    corpus.transition('retire-existing-barrier-refused',driver,lambda:driver.retire(r2,'pending',True,now+312000),'conflict')
    corpus.transition('retire-expired-never-begun',driver,lambda:driver.retire(r2,'issued',False,now+312000))
    corpus.state('driver-retired-row-retained',driver)
    corpus.transition('clock-rollback-refused',driver,lambda:driver.fresh(flow(recipe,7,now+13000),FIXTURES[1],sid(recipe,'auth',7),now+13000),'conflict')
    invalid_states(corpus,driver.state)
    raw_cases(corpus,initial,driver.state)
    capacity_cases(corpus,recipe,config,ids,now)
    return dict(version=1,fixture_only=True,scope='Independent public bytes and finite arithmetic; no session, filesystem, live provider or opaque source authority',
                contract_sha256=CONTRACT_SHA256,input_lock_sha256=INPUT_LOCK_SHA256,baseline=baseline_result,
                verifier_config_hash=m.verifier_digest(key),bindings=bindings,records=corpus.records,tokens=corpus.tokens,
                transitions=corpus.transitions,negatives=corpus.negatives,raw_cases=corpus.raw_cases,capacities=corpus.capacities)


def invalid_states(corpus,valid):
    edits=[('wrong-driver',lambda v:v.update(driver_id='foreign')),
           ('zero-revision',lambda v:v.update(revision=0)),
           ('revision-missing-transitions',lambda v:v.update(revision=1)),
           ('wrong-epoch',lambda v:v['epochs'][0].update(credential_epoch=1)),
           ('dangling-pending',lambda v:v['epochs'][1].update(pending_event_id='f9'*32)),
           ('missing-auth-field',lambda v:v['auth'][0].pop('auth_id')),
           ('zero-auth-id',lambda v:v['auth'][0].update(auth_id='0'*64)),
           ('auth-sequence-gap',lambda v:v['auth'][1].update(sequence=3)),
           ('auth-millisecond-relabel',lambda v:v['auth'][0].update(authenticated_at_ms=v['auth'][0]['authenticated_at_ms']+1)),
           ('reset-wrong-account',lambda v:v['resets'][0].update(account_id=v['epochs'][1]['pending_event_id'])),
           ('reset-hold-extended',lambda v:v['resets'][0]['complete_ack']['barrier'].update(hold_until_ms=v['resets'][0]['complete_ack']['barrier']['hold_until_ms']+1)),
           ('retired-has-ack',lambda v:v['resets'][1].update(begin_ack=own(v['resets'][0]['begin_ack']))),
           ('clock-before-history',lambda v:v.update(clock_high_water_ms=1))]
    for name,edit in edits:
        value=own(valid);edit(value);corpus.reject_state('reject-state-'+name,value)


def raw_cases(corpus,init,state):
    init_raw=packed(init,32768,True);state_raw=packed(state,4194304,True)
    reordered=own(state);request_row=reordered['resets'][0]['request']
    reordered['resets'][0]['request']={key:request_row[key] for key in reversed(tuple(request_row))}
    cases=[('canonical-init','init',init_raw,True),('canonical-driver','driver',state_raw,True),
           ('nested-reordered-request','driver',canonical(reordered)+b'\n',False),
           ('duplicate-version','init',init_raw.replace(b'"version":1',b'"version":1,"version":1',1),False),
           ('duplicate-decoded-version','init',init_raw.replace(b'"version":1',b'"version":1,"\\u0076ersion":1',1),False),
           ('unknown-key','init',init_raw.replace(b'{',b'{"unknown":0,',1),False),
           ('fraction-version','init',init_raw.replace(b'"version":1',b'"version":1.0',1),False),
           ('negative-zero','init',init_raw.replace(b'"version":1',b'"version":-0',1),False),
           ('missing-lf','init',init_raw[:-1],False),('second-lf','init',init_raw+b'\n',False),
           ('trailing-value','init',init_raw[:-1]+b'{}\n',False),
           ('invalid-utf8','init',init_raw.replace(b'local-hosted',b'local-\xffhosted',1),False)]
    for name,kind,raw,valid in cases:
        corpus.name('raw-'+name)
        try:
            value,_=parse(raw,32768 if kind=='init' else 4194304,True)
            if kind=='init':need(value==init and packed(value,32768,True)==raw)
            else:need(packed(m.audit_driver(value,corpus.config,corpus.coordinator,corpus.accounts),4194304,True)==raw)
            accepted=True
        except Rejected:accepted=False
        need(accepted==valid,'raw case mismatch: '+name)
        corpus.raw_cases.append(dict(name='raw-'+name,kind=kind,valid=valid,raw_hex=raw.hex()))


def capacity_cases(corpus,recipe,config,ids,now):
    # Full maximum states are inspected but emitted only as count/byte/hash
    # summaries plus the last row, avoiding megabytes of repeated fixture rows.
    driver=m.DriverModel(config,ids['coordinator'],ids['accounts'])
    for index in range(1024):
        fixture=FIXTURES[index%2]
        driver.state['auth'].append(dict(version=1,sequence=index+1,fixture=fixture,subject=fixture,
            auth_id=sid(recipe,'auth',index),flow=flow(recipe,index,now),credential_epoch=1,authenticated_at_ms=m.sample_seconds_ms(now)))
    driver.state['clock_high_water_ms']=now;driver.state['revision']=1025
    checked=m.audit_driver(driver.state,config,ids['coordinator'],ids['accounts']);raw=packed(checked,4194304,True)
    corpus.capacities.append(dict(name='auth1024-bounded-state',count=1024,bytes=len(raw),sha256=sha(raw),last_row=own(checked['auth'][-1])))
    corpus.transition('auth1025-refused',driver,lambda:driver.fresh(flow(recipe,1024,now),FIXTURES[0],sid(recipe,'auth',1024),now),'capacity')
    driver=m.DriverModel(config,ids['coordinator'],ids['accounts'])
    for index in range(256):
        fixture=FIXTURES[index%2]
        issued=now+index*300001
        driver.state['resets'].append(dict(version=1,sequence=index+1,fixture=fixture,subject=fixture,account_id=ids['accounts'][fixture],
            operation_id=sid(recipe,'operation',index),request=request(recipe,index,issued),from_epoch=1,to_epoch=2,
            verified_at_ms=m.sample_seconds_ms(issued),phase='retired',begin_ack=None,complete_ack=None))
    after_retirements=now+256*300001
    driver.state['clock_high_water_ms']=after_retirements;driver.state['revision']=513
    checked=m.audit_driver(driver.state,config,ids['coordinator'],ids['accounts']);raw=packed(checked,4194304,True)
    corpus.capacities.append(dict(name='reset256-retained-state',count=256,bytes=len(raw),sha256=sha(raw),last_row=own(checked['resets'][-1])))
    corpus.transition('reset257-refused',driver,lambda:driver.begin(request(recipe,256,after_retirements),FIXTURES[0],sid(recipe,'operation',256),after_retirements),'capacity')
    revision=m.DriverModel(config,ids['coordinator'],ids['accounts']);revision.state['revision']=2**32-1
    corpus.transition('driver-revision-overflow-synthetic-boundary',revision,lambda:revision.fresh(flow(recipe,0,now),FIXTURES[0],sid(recipe,'auth',0),now),'invalid')
    epoch=m.DriverModel(config,ids['coordinator'],ids['accounts']);epoch.state['epochs'][0]['credential_epoch']=2**64-1
    # The direct boundary seam is explicitly synthetic, not a fully audited
    # authority history that could fit into the256 retained-reset policy.
    corpus.transition('credential-epoch-overflow-synthetic-boundary',epoch,lambda:epoch.begin(request(recipe,0,now),FIXTURES[0],sid(recipe,'operation',0),now),'invalid')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--execute',action='store_true')
    parser.add_argument('--input-root',type=Path,required=True)
    parser.add_argument('--approved-producer-lock',type=Path,required=True)
    parser.add_argument('--approved-lock-sha256',required=True)
    parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args();need(args.execute,'separate execution authorization required')
    source_root=Path(__file__).resolve().parent
    sources={name:sha((source_root/name).read_bytes()) for name in SOURCE_NAMES}
    lock_raw=args.approved_producer_lock.read_bytes();need(sha(lock_raw)==args.approved_lock_sha256,'approved producer lock')
    approval=json.loads(lock_raw)
    need(approval['input_lock_sha256']==INPUT_LOCK_SHA256,'input binding')
    need(approval.get('approved_by')=='root' and approval.get('execution_authorized') is True,'lock does not authorize execution')
    need(type(approval['sources']) is dict and set(approval['sources'])==set(SOURCE_NAMES),'exact reviewed source set')
    need(approval['sources']==sources,'reviewed source bytes')
    lock,recipe,values=load_inputs(args.input_root)
    result=build(args.input_root,recipe,values)
    result['producer_files']=sources;result['producer_lock_sha256']=args.approved_lock_sha256
    result['counts']={name:len(result[name]) for name in ('records','tokens','transitions','negatives','raw_cases','capacities')}
    load_inputs(args.input_root)
    need(sources=={name:sha((source_root/name).read_bytes()) for name in SOURCE_NAMES},'source unchanged')
    encoded=canonical(result)+b'\n'
    with args.output.open('xb') as stream:stream.write(encoded)
    print(json.dumps(dict(output=str(args.output),bytes=len(encoded),sha256=sha(encoded),counts=result['counts']),sort_keys=True))


if __name__=='__main__':main()
