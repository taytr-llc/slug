"""Public-fixture RS256/config/binding arithmetic. No live authentication authority."""
import base64
import hashlib
from common import (FIXTURES, ISSUER, DRIVER, WRAPPER, U64_MAX, TIME_MAX, Rejected, parse,
                    canonical, event_id, exact, hex64, integer, need, own,
                    packed, principal_id, sha)

LOGIN_AUD='slug-provider-fixture-login'
RESET_AUD='slug-provider-fixture-reset'
FLOW=('version','coordinator_id','authority_id','id','attempt_id','issuer','nonce','sequence','issued_at_ms','expires_at_ms')
REQUEST=('version','coordinator_id','authority_id','event_id','nonce','issued_at_ms','expires_at_ms')
BARRIER=('version','event_id','account_id','principal_id','generation','from_epoch','to_epoch','status','verified_at_ms','began_at_ms','completed_at_ms','hold_until_ms','begin_sequence','complete_sequence')
RESULT=('version','coordinator_id','authority_id','event_id','provider_operation_id','phase','can_commit','barrier')


def public_config(key):
    need(key['fixture_only'] is True and key['kty']=='RSA' and key['kid']=='rfc7515-a2' and key['e']=='AQAB')
    return dict(version=1,algorithm='RS256',issuer=ISSUER,audience=LOGIN_AUD,
                reset_audience=RESET_AUD,epoch_claim='slug_auth_epoch',
                keys=[dict(kid=key['kid'],n=key['n'],e=key['e'])])


def verifier_digest(key):
    return sha(b'SLUG identity verifier configuration\x00v1\x00'+canonical(public_config(key)))


def configurations(key, authority, policy):
    principals=[dict(issuer=ISSUER,subject=f,fixture=f) for f in FIXTURES]
    # Root's explicit clarification: hash inherited normalized persisted order.
    principals.sort(key=lambda p:principal_id(p['issuer'],p['subject']))
    accounts=dict(version=1,principals=principals,max_sessions=4096,max_challenges=4096,max_events=16384)
    identity=dict(version=2,accounts=accounts,verifier_config_hash=verifier_digest(key),max_login_attempts=1024)
    coordinator=dict(version=1,provider='published-fixture-v1',issuer=ISSUER,
                     tenant_id='fixture-tenant',client_id='fixture-client',connection_id='fixture-database',
                     authority_id=authority,verifier_config_hash=verifier_digest(key),max_login_flows=1024,max_reset_flows=256)
    vault=dict(version=1,wrapper_id=WRAPPER,recovery_key_ref='fixture-recovery-v1',gym_key_ref='fixture-gym-v1',
               allowed_recovery_key_refs=['fixture-recovery-v1'],allowed_gym_key_refs=['fixture-gym-v1'],max_operations=128,policy=own(policy))
    return identity,coordinator,vault


def sample_seconds_ms(now_ms):
    integer(now_ms,1000,TIME_MAX)
    return now_ms//1000*1000


def common_claims(config, coordinator, subject, now_ms):
    need(subject in FIXTURES)
    now=sample_seconds_ms(now_ms)//1000
    need((now+120)*1000<=TIME_MAX,'capacity')
    return dict(iss=ISSUER,sub=subject,iat=now,exp=now+120,provider_version=1,
                coordinator_id=coordinator,authority_id=config['authority_id'],tenant_id=config['tenant_id'],
                client_id=config['client_id'],connection_id=config['connection_id'])


def login_claims(config, auth, now_ms):
    # This is the canonical claim order frozen by the producer recipe; object
    # field order is not an extra verifier authority assumption.
    c=common_claims(config,auth['flow']['coordinator_id'],auth['subject'],now_ms)
    c.update(aud=LOGIN_AUD,auth_time=auth['authenticated_at_ms']//1000,
             nonce=auth['flow']['nonce'],slug_auth_epoch=auth['credential_epoch'],
             login_flow_id=auth['flow']['id'],original_auth_id=auth['auth_id'],
             original_flow_id=auth['flow']['id'],auth_mode='fresh')
    need(auth['authenticated_at_ms']%1000==0)
    return c


def reset_claims(config,row,stage,now_ms):
    need(stage in ('begin','complete'))
    c=common_claims(config,row['request']['coordinator_id'],row['subject'],now_ms)
    c.update(aud=RESET_AUD,event_id=row['request']['event_id'],stage=stage,
             from_epoch=row['from_epoch'],to_epoch=row['to_epoch'],verified_at=row['verified_at_ms']//1000,
             reset_nonce=row['request']['nonce'],provider_operation_id=row['operation_id'])
    need(row['verified_at_ms']%1000==0)
    return c


def b64(raw):return base64.urlsafe_b64encode(raw).rstrip(b'=').decode('ascii')

def unb64(value):return base64.urlsafe_b64decode(value+'='*(-len(value)%4))


def sign_fixture(key,kind,claims):
    """Explicit RFC7515 educational arithmetic over the published test exponent."""
    need(key['fixture_only'] is True)
    header=dict(alg='RS256',kid='rfc7515-a2',typ='JWT' if kind=='login' else 'slug-reset+jwt')
    head,body=canonical(header),canonical(claims)
    message=(b64(head)+'.'+b64(body)).encode('ascii')
    n,e,d=(int.from_bytes(unb64(key[k]),'big') for k in ('n','e','d'))
    size=(n.bit_length()+7)//8
    tail=bytes.fromhex('3031300d060960864801650304020105000420')+hashlib.sha256(message).digest()
    encoded=b'\x00\x01'+b'\xff'*(size-len(tail)-3)+b'\x00'+tail
    signature=pow(int.from_bytes(encoded,'big'),d,n).to_bytes(size,'big')
    need(pow(int.from_bytes(signature,'big'),e,n).to_bytes(size,'big')==encoded)
    return dict(header_utf8=head.decode(),claims_utf8=body.decode(),signing_input=message.decode(),
                signature_hex=signature.hex(),compact_jws=message.decode()+'.'+b64(signature))


def validate_flow(value,config,coordinator):
    out=exact(value,FLOW)
    need(type(out['version']) is int and out['version']==1)
    need(out['coordinator_id']==coordinator and out['authority_id']==config['authority_id'] and out['issuer']==ISSUER,'conflict')
    for k in ('id','attempt_id','nonce'):hex64(out[k])
    integer(out['sequence'],1,U64_MAX)
    integer(out['issued_at_ms'],1,TIME_MAX);integer(out['expires_at_ms'],out['issued_at_ms']+1,TIME_MAX)
    need(out['expires_at_ms']-out['issued_at_ms']==300000)
    return out


def validate_request(value,config,coordinator):
    out=exact(value,REQUEST)
    need(type(out['version']) is int and out['version']==1)
    need(out['coordinator_id']==coordinator and out['authority_id']==config['authority_id'],'conflict')
    for k in ('event_id','nonce'):hex64(out[k])
    integer(out['issued_at_ms'],1,TIME_MAX);integer(out['expires_at_ms'],out['issued_at_ms']+1,TIME_MAX)
    need(out['expires_at_ms']-out['issued_at_ms']==300000)
    return out


def reset_result(row,generation,began,beginseq,completed=0,completeseq=0):
    """Modeled inherited authority receipt; not a verified or live capability."""
    pid=principal_id(ISSUER,row['subject'])
    barrier=dict(version=2,event_id=event_id(pid,row['request']['event_id']),account_id=row['account_id'],
                 principal_id=pid,generation=generation,from_epoch=row['from_epoch'],to_epoch=row['to_epoch'],
                 status='completed' if completed else 'pending',verified_at_ms=row['verified_at_ms'],began_at_ms=began,
                 completed_at_ms=completed,hold_until_ms=completed+86400000 if completed else 0,
                 begin_sequence=beginseq,complete_sequence=completeseq)
    return dict(version=1,coordinator_id=row['request']['coordinator_id'],authority_id=row['request']['authority_id'],
                event_id=row['request']['event_id'],provider_operation_id=row['operation_id'],
                phase='completed' if completed else 'pending',can_commit=not bool(completed),barrier=barrier)


class DriverModel:
    """Finite durable public state model; caller capabilities remain unmodeled."""
    def __init__(self,config,coordinator,accounts):
        self.config=own(config);self.coordinator=coordinator;self.accounts=own(accounts)
        self.state=dict(version=1,driver_id=DRIVER,coordinator_id=coordinator,config=own(config),revision=1,clock_high_water_ms=0,
                        epochs=[dict(fixture=f,subject=f,credential_epoch=1,pending_event_id='') for f in FIXTURES],auth=[],resets=[])

    def transition(self,next_state,now):
        integer(now,1,TIME_MAX);need(now>=self.state['clock_high_water_ms'],'conflict')
        next_state['revision']=integer(self.state['revision']+1,1)
        next_state['clock_high_water_ms']=now
        for row in next_state['auth']:packed(row,2048)
        for row in next_state['resets']:packed(row,4096)
        packed(next_state,4194304,True)
        self.state=next_state

    def epoch(self,fixture,state=None):
        need(fixture in FIXTURES)
        return (self.state if state is None else state)['epochs'][FIXTURES.index(fixture)]

    def fresh(self,flow,fixture,auth_id,now):
        integer(now,1000,TIME_MAX);need(now>=self.state['clock_high_water_ms'],'conflict')
        flow=validate_flow(flow,self.config,self.coordinator);hex64(auth_id,True)
        for old in self.state['auth']:
            if old['flow']['id']==flow['id']:
                need(old['flow']==flow and old['fixture']==fixture,'conflict');return own(old),False
        need(len(self.state['auth'])<1024,'capacity')
        need(not self.epoch(fixture)['pending_event_id'],'conflict')
        need(all(r['auth_id']!=auth_id for r in self.state['auth']),'conflict')
        at=sample_seconds_ms(now);need(at//1000>=flow['issued_at_ms']//1000 and now<flow['expires_at_ms'],'expired')
        row=dict(version=1,sequence=len(self.state['auth'])+1,fixture=fixture,subject=fixture,auth_id=auth_id,
                 flow=flow,credential_epoch=self.epoch(fixture)['credential_epoch'],authenticated_at_ms=at)
        staged=own(self.state);staged['auth'].append(row);self.transition(staged,now)
        return own(row),True

    def begin(self,request,fixture,operation_id,now):
        integer(now,1000,TIME_MAX);need(now>=self.state['clock_high_water_ms'],'conflict')
        request=validate_request(request,self.config,self.coordinator);hex64(operation_id,True)
        for old in self.state['resets']:
            if old['request']['event_id']==request['event_id']:
                need(old['request']==request and old['fixture']==fixture and old['operation_id']==operation_id,'conflict');return own(old),False
        need(len(self.state['resets'])<256,'capacity')
        need(all(r['operation_id']!=operation_id for r in self.state['resets']),'conflict')
        epoch=self.epoch(fixture);need(not epoch['pending_event_id'],'conflict')
        integer(epoch['credential_epoch'],1,U64_MAX-1)
        at=sample_seconds_ms(now);need(at//1000>=request['issued_at_ms']//1000 and now<request['expires_at_ms'],'expired')
        row=dict(version=1,sequence=len(self.state['resets'])+1,fixture=fixture,subject=fixture,account_id=self.accounts[fixture],operation_id=operation_id,
                 request=request,from_epoch=epoch['credential_epoch'],to_epoch=epoch['credential_epoch']+1,
                 verified_at_ms=at,phase='verified',begin_ack=None,complete_ack=None)
        staged=own(self.state);staged['resets'].append(row);self.epoch(fixture,staged)['pending_event_id']=request['event_id'];self.transition(staged,now)
        return own(row),True

    def ack(self,request,value,complete,now):
        integer(now,1,TIME_MAX);need(now>=self.state['clock_high_water_ms'],'conflict')
        request=validate_request(request,self.config,self.coordinator);value=exact(value,RESULT)
        matches=[r for r in self.state['resets'] if r['request']['event_id']==request['event_id']]
        need(len(matches)==1,'not_found');old=matches[0];need(old['request']==request,'conflict')
        barrier=exact(value['barrier'],BARRIER)
        for field in ('generation','begin_sequence'):integer(barrier[field],1,U64_MAX)
        integer(barrier['complete_sequence'],0,U64_MAX)
        for field in ('verified_at_ms','began_at_ms'):integer(barrier[field],1,TIME_MAX)
        for field in ('completed_at_ms','hold_until_ms'):integer(barrier[field],0,TIME_MAX)
        expected=reset_result(old,barrier['generation'],barrier['began_at_ms'],barrier['begin_sequence'],barrier['completed_at_ms'] if complete else 0,barrier['complete_sequence'] if complete else 0)
        need(canonical(value)==canonical(expected) and type(value['can_commit']) is bool,'conflict')
        need(barrier['generation']==old['to_epoch'] and barrier['began_at_ms']>=old['verified_at_ms'] and barrier['begin_sequence']>0,'conflict')
        if complete:need(barrier['completed_at_ms']>=barrier['began_at_ms'] and barrier['complete_sequence']>barrier['begin_sequence'],'conflict')
        slot='complete_ack' if complete else 'begin_ack'
        if old[slot] is not None:
            need(old[slot]==value,'conflict');return own(old),False
        need(old['phase']==('committed' if complete else 'verified'),'conflict')
        staged=own(self.state);row=staged['resets'][old['sequence']-1];epoch=self.epoch(old['fixture'],staged)
        need(epoch['pending_event_id']==request['event_id'],'conflict')
        if complete:
            prior=row['begin_ack']['barrier']
            need(all(barrier[k]==prior[k] for k in BARRIER if k not in ('status','completed_at_ms','hold_until_ms','complete_sequence')),'conflict')
            need(epoch['credential_epoch']==row['to_epoch'],'conflict');epoch['pending_event_id']='';row['phase']='completed'
        else:
            need(epoch['credential_epoch']==row['from_epoch'],'conflict');epoch['credential_epoch']=row['to_epoch'];row['phase']='committed'
        row[slot]=own(value);self.transition(staged,now);return own(row),True

    def pending(self,fixture):
        event=self.epoch(fixture)['pending_event_id']
        need(event!='','not_found')
        rows=[r for r in self.state['resets'] if r['fixture']==fixture and r['request']['event_id']==event]
        need(len(rows)==1 and rows[0]['phase'] in ('verified','committed'),'conflict')
        row=rows[0]
        fields=('version','fixture','account_id','operation_id','request','from_epoch','to_epoch','verified_at_ms','phase')
        return {k:own(row[k]) for k in fields}

    def retire(self,request,coordinator_phase,barrier_present,now):
        # Observations are explicit arithmetic-model inputs, never capabilities.
        integer(now,1,TIME_MAX);need(now>=self.state['clock_high_water_ms'],'conflict')
        request=validate_request(request,self.config,self.coordinator)
        rows=[r for r in self.state['resets'] if r['request']['event_id']==request['event_id']]
        need(len(rows)==1,'not_found');old=rows[0]
        need(old['request']==request and old['phase']=='verified','conflict')
        need(coordinator_phase=='issued' and barrier_present is False and now>=request['expires_at_ms'],'conflict')
        staged=own(self.state);row=staged['resets'][old['sequence']-1]
        epoch=self.epoch(old['fixture'],staged)
        need(epoch['pending_event_id']==request['event_id'],'conflict')
        row['phase']='retired';epoch['pending_event_id']=''
        self.transition(staged,now);return own(row)


AUTH=('version','sequence','fixture','subject','auth_id','flow','credential_epoch','authenticated_at_ms')
RESET=('version','sequence','fixture','subject','account_id','operation_id','request','from_epoch','to_epoch','verified_at_ms','phase','begin_ack','complete_ack')
EPOCH=('fixture','subject','credential_epoch','pending_event_id')
STATE=('version','driver_id','coordinator_id','config','revision','clock_high_water_ms','epochs','auth','resets')


def audit_driver(value,config,coordinator,accounts):
    """Bounded persisted public-state arithmetic; no filesystem/account claim."""
    state=own(exact(value,STATE))
    need(state['version']==1 and type(state['version']) is int)
    need(state['driver_id']==DRIVER and state['coordinator_id']==coordinator and canonical(state['config'])==canonical(config),'conflict')
    integer(state['revision'],1);integer(state['clock_high_water_ms'],0,TIME_MAX)
    need(type(state['auth']) is list and len(state['auth'])<=1024,'capacity')
    need(type(state['resets']) is list and len(state['resets'])<=256,'capacity')
    need(type(state['epochs']) is list and len(state['epochs'])==2)
    epochs={};pending={};seen_events=set();seen_ops=set();max_time=0
    for index,(fixture,row) in enumerate(zip(FIXTURES,state['epochs'])):
        row=exact(row,EPOCH)
        state['epochs'][index]=row
        need(row['fixture']==fixture and row['subject']==fixture,'conflict')
        integer(row['credential_epoch'],1,U64_MAX)
        if row['pending_event_id']:hex64(row['pending_event_id'])
        epochs[fixture]=1;pending[fixture]=''
    for index,supplied in enumerate(state['resets'],1):
        row=exact(supplied,RESET);packed(row,4096)
        state['resets'][index-1]=row
        need(type(row['version']) is int and row['version']==1 and type(row['sequence']) is int and row['sequence']==index)
        fixture=row['fixture'];need(fixture in FIXTURES and row['subject']==fixture and row['account_id']==accounts[fixture],'conflict')
        hex64(row['operation_id'],True);need(row['operation_id'] not in seen_ops,'conflict');seen_ops.add(row['operation_id'])
        request=validate_request(row['request'],config,coordinator)
        row['request']=request
        need(request['event_id'] not in seen_events,'conflict');seen_events.add(request['event_id'])
        integer(row['from_epoch'],1,U64_MAX-1);integer(row['to_epoch'],1,U64_MAX)
        need(row['from_epoch']==epochs[fixture] and row['to_epoch']==row['from_epoch']+1 and not pending[fixture],'conflict')
        integer(row['verified_at_ms'],1000,TIME_MAX);need(row['verified_at_ms']%1000==0)
        need(row['verified_at_ms']//1000>=request['issued_at_ms']//1000 and row['verified_at_ms']<request['expires_at_ms'],'conflict')
        max_time=max(max_time,row['verified_at_ms'])
        phase=row['phase'];need(phase in ('verified','committed','completed','retired'))
        if phase in ('verified','retired'):
            need(row['begin_ack'] is None and row['complete_ack'] is None,'conflict')
            if phase=='verified':pending[fixture]=request['event_id']
            else:need(state['clock_high_water_ms']>=request['expires_at_ms'],'conflict')
        else:
            begin=exact(row['begin_ack'],RESULT);b=exact(begin['barrier'],BARRIER)
            begin['barrier']=b;row['begin_ack']=begin
            for key in ('generation','begin_sequence'):integer(b[key],1,U64_MAX)
            integer(b['began_at_ms'],row['verified_at_ms'],TIME_MAX)
            # Inherited schema2 identity.go validates Generation==ToEpoch while
            # pending; this is not a general provider/session equivalence claim.
            need(b['generation']==row['to_epoch'],'conflict')
            need(canonical(begin)==canonical(reset_result(row,b['generation'],b['began_at_ms'],b['begin_sequence'])) and begin['can_commit'] is True,'conflict')
            epochs[fixture]=row['to_epoch'];max_time=max(max_time,b['began_at_ms'])
            if phase=='committed':
                need(row['complete_ack'] is None,'conflict');pending[fixture]=request['event_id']
            else:
                complete=exact(row['complete_ack'],RESULT);c=exact(complete['barrier'],BARRIER)
                complete['barrier']=c;row['complete_ack']=complete
                integer(c['completed_at_ms'],b['began_at_ms'],TIME_MAX-86400000)
                integer(c['complete_sequence'],b['begin_sequence']+1,U64_MAX)
                need(canonical(complete)==canonical(reset_result(row,b['generation'],b['began_at_ms'],b['begin_sequence'],c['completed_at_ms'],c['complete_sequence'])) and complete['can_commit'] is False,'conflict')
                max_time=max(max_time,c['completed_at_ms'])
    for row in state['epochs']:
        fixture=row['fixture'];need(row['credential_epoch']==epochs[fixture] and row['pending_event_id']==pending[fixture],'conflict')
    expected_revision=1+len(state['auth'])+len(state['resets'])
    expected_revision+=sum({'verified':0,'committed':1,'completed':2,'retired':1}[row['phase']] for row in state['resets'])
    # Additional acknowledged clock-only transitions are not forbidden by the
    # contract. Retained operations impose this minimum, not exact equality.
    need(state['revision']>=expected_revision,'conflict')
    seen_auth=set();seen_flow=set()
    for index,supplied in enumerate(state['auth'],1):
        row=exact(supplied,AUTH);packed(row,2048)
        state['auth'][index-1]=row
        need(type(row['version']) is int and row['version']==1 and type(row['sequence']) is int and row['sequence']==index)
        fixture=row['fixture'];need(fixture in FIXTURES and row['subject']==fixture,'conflict')
        hex64(row['auth_id'],True);need(row['auth_id'] not in seen_auth,'conflict');seen_auth.add(row['auth_id'])
        flow=validate_flow(row['flow'],config,coordinator);need(flow['id'] not in seen_flow,'conflict');seen_flow.add(flow['id'])
        row['flow']=flow
        integer(row['credential_epoch'],1,epochs[fixture]);integer(row['authenticated_at_ms'],1000,TIME_MAX)
        need(row['authenticated_at_ms']%1000==0 and row['authenticated_at_ms']//1000>=flow['issued_at_ms']//1000 and row['authenticated_at_ms']<flow['expires_at_ms'],'conflict')
        max_time=max(max_time,row['authenticated_at_ms'])
    need(state['clock_high_water_ms']>=max_time,'conflict')
    packed(state,4194304,True)
    return own(state)


def verify_fixture_token(key,kind,token,config,coordinator,expected,now_ms):
    """Independent coordinated-token checks for this finite fixture corpus."""
    need(kind in ('login','reset') and type(token) is str and len(token.encode())<=16384)
    parts=token.split('.');need(len(parts)==3)
    try:
        raw=[unb64(p) for p in parts]
    except Exception as error:
        raise Rejected('invalid') from error
    need(all(b64(r)==p for r,p in zip(raw,parts)))
    header,_=parse(raw[0],2048);claims,_=parse(raw[1],8192)
    need(header==dict(alg='RS256',kid='rfc7515-a2',typ='JWT' if kind=='login' else 'slug-reset+jwt'))
    n,e=(int.from_bytes(unb64(key[k]),'big') for k in ('n','e'))
    size=(n.bit_length()+7)//8;need(len(raw[2])==size and int.from_bytes(raw[2],'big')<n)
    message=(parts[0]+'.'+parts[1]).encode('ascii')
    tail=bytes.fromhex('3031300d060960864801650304020105000420')+hashlib.sha256(message).digest()
    encoding=b'\x00\x01'+b'\xff'*(size-len(tail)-3)+b'\x00'+tail
    need(pow(int.from_bytes(raw[2],'big'),e,n).to_bytes(size,'big')==encoding)
    base=('iss','sub','iat','exp','provider_version','coordinator_id','authority_id','tenant_id','client_id','connection_id','aud')
    extra=('auth_time','nonce','slug_auth_epoch','login_flow_id','original_auth_id','original_flow_id','auth_mode') if kind=='login' else ('event_id','stage','from_epoch','to_epoch','verified_at','reset_nonce','provider_operation_id')
    claims=exact(claims,base+extra)
    need(claims['iss']==ISSUER and claims['sub'] in FIXTURES and claims['aud']==(LOGIN_AUD if kind=='login' else RESET_AUD),'conflict')
    need(type(claims['provider_version']) is int and claims['provider_version']==1)
    need(claims['coordinator_id']==coordinator and all(claims[k]==config[k] for k in ('authority_id','tenant_id','client_id','connection_id')),'conflict')
    integer(now_ms,1,TIME_MAX)
    integer(claims['iat'],1,TIME_MAX//1000);integer(claims['exp'],1,TIME_MAX//1000)
    issued=claims['iat']*1000;expires=claims['exp']*1000
    need(issued<=now_ms<expires and 0<expires-issued<=300000 and now_ms-issued<=300000,'expired')
    if kind=='login':
        need(claims['login_flow_id']==expected['id'] and claims['nonce']==expected['nonce'],'conflict')
        for field in ('login_flow_id','nonce','original_auth_id','original_flow_id'):hex64(claims[field])
        need(claims['auth_mode']=='fresh' and claims['original_flow_id']==claims['login_flow_id'],'conflict')
        integer(claims['auth_time'],1,TIME_MAX//1000);integer(claims['slug_auth_epoch'],1,U64_MAX)
        need(claims['auth_time']*1000<=issued and now_ms-claims['auth_time']*1000<=300000,'expired')
    else:
        need(claims['event_id']==expected['event_id'] and claims['reset_nonce']==expected['nonce'],'conflict')
        for field in ('event_id','reset_nonce','provider_operation_id'):hex64(claims[field])
        need(claims['stage'] in ('begin','complete'))
        integer(claims['from_epoch'],1,U64_MAX-1);integer(claims['to_epoch'],1,U64_MAX)
        need(claims['to_epoch']==claims['from_epoch']+1)
        integer(claims['verified_at'],1,TIME_MAX//1000);need(claims['verified_at']*1000<=issued)
    return claims
