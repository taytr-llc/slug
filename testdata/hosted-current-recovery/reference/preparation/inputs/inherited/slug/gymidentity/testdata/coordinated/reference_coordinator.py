#!/usr/bin/env python3
"""Public-fixture coordinated JWS oracle; Python stdlib only, no Go/network.

RSA signing delegates to the copied independent RFC7515 reference, whose entire
previous corpus is reproduced first. Protocol expectations are from the frozen
set31 contract, not an implementation or a claimed executed state machine.
"""
import argparse
import hashlib
import json
from pathlib import Path
import struct
import sys

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT / 'baseline'))
import reference_identity as rsa

NOW = 1700000000
LOGIN_AUD = 'slug-provider-fixture-login'
RESET_AUD = 'slug-provider-fixture-reset'
BINDING = {'version': 1, 'coordinator_id': 'a1'*32, 'authority_id': 'b2'*32,
           'tenant_id': 'slug-fixture-tenant', 'client_id': 'slug-fixture-client',
           'connection_id': 'public-fixture-password'}
FLOW, NONCE, ORIGIN = 'c3'*32, 'd4'*32, 'e5'*32
EVENT, RESET_NONCE, OPERATION = 'f6'*32, '07'*32, '18'*32
NEXT_FLOW, NEXT_NONCE, NEXT_EVENT, NEXT_RESET_NONCE = '29'*32, '3a'*32, '4b'*32, '5c'*32
MAX_U64 = 2**64-1


def baseline_check():
    hashes = {'reference_identity.py': '7aac1e2243572cf0c5f60ce236ff7b3f40bc98015b1c182cf2000e2f392b3819',
              'identity_v1.json': 'd1a3275c79dda31b59b00f8051a7d503a495c460bff4779b66c69b5acfcafa80',
              'rfc7515_rsa.json': '6dadd3d7c1ba76fb24557fbdd26ac956b6430692a4cac701cbefa035b2692a9a'}
    for name, expected in hashes.items():
        assert hashlib.sha256((ROOT/'baseline'/name).read_bytes()).hexdigest() == expected
    previous = rsa.corpus()
    assert (ROOT/'baseline/identity_v1.json').read_text() == json.dumps(previous, indent=2, ensure_ascii=True)+'\n'
    expected_key = json.dumps(rsa.fixture_key(), indent=2)+'\n'
    assert (ROOT/'rfc7515_rsa.json').read_text() == expected_key
    return {'rfc7515_exact_signature': True, 'previous_cases': len(previous['cases']),
            'previous_accepted': sum(c['accepted'] for c in previous['cases']),
            'source_hashes': hashes}


def common():
    return {'provider_version': 1, **{k:v for k,v in BINDING.items() if k!='version'}}


def login():
    return {'iss': rsa.ISSUER, 'aud': LOGIN_AUD, 'sub': 'public-fixture-a',
            'iat': NOW-5, 'exp': NOW+295, 'auth_time': NOW-20,
            'nonce': NONCE, 'slug_auth_epoch': 1, **common(),
            'login_flow_id': FLOW, 'original_auth_id': ORIGIN,
            'original_flow_id': FLOW, 'auth_mode': 'fresh'}


def reset():
    return {'iss': rsa.ISSUER, 'aud': RESET_AUD, 'sub': 'public-fixture-a',
            'iat': NOW-5, 'exp': NOW+295, 'event_id': EVENT,
            'stage': 'begin', 'from_epoch': 1, 'to_epoch': 2,
            'verified_at': NOW-20, **common(), 'reset_nonce': RESET_NONCE,
            'provider_operation_id': OPERATION}


def expected(kind, claims):
    nested = rsa.expected(kind, claims)
    if kind=='login':
        return {'login': nested, 'flow_id': claims['login_flow_id'],
                'original_auth_id': claims['original_auth_id'],
                'original_flow_id': claims['original_flow_id'], 'mode': claims['auth_mode']}
    return {'reset': nested, 'provider_operation_id': claims['provider_operation_id'],
            'reset_nonce': claims['reset_nonce']}


def tuple_index(domain, components):
    raw = domain.encode('ascii') + b''.join(struct.pack('<I', len(v.encode('utf8')))+v.encode('utf8') for v in components)
    return {'components': components, 'encoded': raw.hex(), 'sha256': hashlib.sha256(raw).hexdigest()}


def corpus():
    baseline = baseline_check()
    cases = []
    defaults = {'login': login(), 'reset': reset()}
    headers = {'login': {'alg':'RS256','kid':rsa.KID,'typ':'JWT'},
               'reset': {'alg':'RS256','kid':rsa.KID,'typ':'slug-reset+jwt'}}

    def add(name, kind='login', changes=None, *, claims=None, accept=False, reason='',
            binding=None, flow=FLOW, nonce=None, event=EVENT, now=NOW,
            header=None, header_raw=None, payload=None, token=None, signed=True):
        claims = dict(defaults[kind] if claims is None else claims)
        if changes: claims.update(changes)
        header = headers[kind] if header is None else header
        body = rsa.raw_json(claims) if payload is None else payload
        head = rsa.raw_json(header) if header_raw is None else header_raw
        token = rsa.compact(head,body) if token is None else token
        c={'name':name,'kind':kind,'now_unix_seconds':now,
           'binding':dict(BINDING if binding is None else binding),
           'expected_flow_id':flow if kind=='login' else '',
           'expected_event_id':event if kind=='reset' else '',
           'expected_nonce':nonce if nonce is not None else (NONCE if kind=='login' else RESET_NONCE),
           'token':token,'mathematically_signed':signed,'verifier_accepted':accept}
        if accept: c['expected']=expected(kind,claims)
        else:
            assert reason
            c['rejection_reason']=reason
        cases.append(c)
        return token

    fresh=add('login-fresh',accept=True)
    add('login-second-principal',changes={'sub':'public-fixture-b'},accept=True)
    add('login-reuse-accepted-origin',changes={'login_flow_id':NEXT_FLOW,'nonce':NEXT_NONCE,'auth_mode':'reuse'},flow=NEXT_FLOW,nonce=NEXT_NONCE,accept=True)
    add('login-reuse-unknown-origin',changes={'login_flow_id':NEXT_FLOW,'nonce':NEXT_NONCE,'auth_mode':'reuse','original_auth_id':'6d'*32},flow=NEXT_FLOW,nonce=NEXT_NONCE,accept=True)
    add('login-fresh-relabels-known-origin',changes={'login_flow_id':NEXT_FLOW,'original_flow_id':NEXT_FLOW,'nonce':NEXT_NONCE},flow=NEXT_FLOW,nonce=NEXT_NONCE,accept=True)
    add('login-reuse-relabels-epoch',changes={'login_flow_id':NEXT_FLOW,'nonce':NEXT_NONCE,'auth_mode':'reuse','slug_auth_epoch':2},flow=NEXT_FLOW,nonce=NEXT_NONCE,accept=True)
    add('login-reuse-relabels-authentication-time',changes={'login_flow_id':NEXT_FLOW,'nonce':NEXT_NONCE,'auth_mode':'reuse','auth_time':NOW-19},flow=NEXT_FLOW,nonce=NEXT_NONCE,accept=True)
    for epoch in [2**53+1,MAX_U64]:
        add('login-epoch-'+str(epoch),changes={'slug_auth_epoch':epoch},accept=True)
    add('login-exact-auth-freshness-boundary',changes={'auth_time':NOW-300},accept=True)
    add('login-fresh-header-at-limit',header_raw=rsa.raw_json(headers['login']).ljust(2048,b' '),accept=True)
    add('login-fresh-payload-at-limit',payload=rsa.raw_json(login()).ljust(8192,b' '),accept=True)
    for field in ['tenant_id','client_id','connection_id']:
        add('login-'+field+'-at-limit',changes={field:'a'*128},binding={**BINDING,field:'a'*128},accept=True)
        for label,value in [('over-limit','a'*129),('non-ascii','\u00e9'),('invalid-punctuation','tenant/client')]:
            add('reject-login-'+field+'-'+label,changes={field:value},binding={**BINDING,field:value},reason='matching configured identifiers still require1..128 ASCII letters/digits/._-')
    zero_binding={**BINDING,'coordinator_id':'0'*64,'authority_id':'0'*64}
    add('login-zero-id-shapes',changes={'coordinator_id':'0'*64,'authority_id':'0'*64,'login_flow_id':'0'*64,'original_flow_id':'0'*64,'original_auth_id':'0'*64,'nonce':'0'*64},binding=zero_binding,flow='0'*64,nonce='0'*64,accept=True)

    begin=add('reset-begin','reset',accept=True)
    add('reset-complete','reset',{'stage':'complete'},accept=True)
    add('reset-begin-old-verification','reset',{'verified_at':NOW-86400},accept=True)
    add('reset-complete-old-verification','reset',{'stage':'complete','verified_at':NOW-86400},accept=True)
    add('reset-from-above-js-safe-integer','reset',{'from_epoch':2**53+1,'to_epoch':2**53+2},accept=True)
    add('reset-to-max-uint64','reset',{'from_epoch':MAX_U64-1,'to_epoch':MAX_U64},accept=True)
    add('reset-same-operation-new-handle','reset',{'event_id':NEXT_EVENT,'reset_nonce':NEXT_RESET_NONCE},event=NEXT_EVENT,nonce=NEXT_RESET_NONCE,accept=True)
    add('reset-same-handle-new-operation','reset',{'provider_operation_id':'7e'*32},accept=True)
    add('reset-complete-other-principal','reset',{'stage':'complete','sub':'public-fixture-b'},accept=True)
    add('reset-complete-changed-verified-time','reset',{'stage':'complete','verified_at':NOW-19},accept=True)
    add('reset-complete-changed-epochs','reset',{'stage':'complete','from_epoch':2,'to_epoch':3},accept=True)
    add('reset-zero-id-shapes','reset',{'event_id':'0'*64,'reset_nonce':'0'*64,'provider_operation_id':'0'*64},event='0'*64,nonce='0'*64,accept=True)

    # Each binding substitution is correctly RSA-signed. Expected server-side
    # routing values remain pinned, so none can select a different trust scope.
    for kind in ['login','reset']:
        for field,value in [('coordinator_id','8f'*32),('authority_id','90'*32),
                            ('tenant_id','other-tenant'),('client_id','other-client'),
                            ('connection_id','other-connection')]:
            add('reject-'+kind+'-'+field,kind,{field:value},reason='signed binding differs from immutable expected configuration')
        for field in defaults[kind]:
            c=dict(defaults[kind]);del c[field]
            add('reject-'+kind+'-missing-'+field,kind,claims=c,reason='exact coordinated schema requires every field')
            add('reject-'+kind+'-null-'+field,kind,{field:None},reason='required coordinated claim cannot be null')
        for field,value in [('email','same@example.invalid'),('amr',['pwd']),('acr','fixture'),('nbf',NOW-5),('azp',LOGIN_AUD),('verifier_config_hash','00'*32)]:
            add('reject-'+kind+'-unknown-'+field,kind,{field:value},reason='coordinated profile forbids all unknown claims, including general verifier optional claims')
        for field in ['provider_version','iat','exp']+(['auth_time','slug_auth_epoch'] if kind=='login' else ['verified_at','from_epoch','to_epoch']):
            for label,value in [('fractional',1.0),('string','1'),('bool',True),('negative',-1),('zero',0),('overflow',2**64)]:
                add('reject-'+kind+'-'+field+'-'+label,kind,{field:value},reason='security number must have its bounded positive canonical unsigned decimal representation')
        add('reject-'+kind+'-provider-version',kind,{'provider_version':2},reason='provider_version is exactly1')
        add('reject-'+kind+'-audience-array',kind,{'aud':[defaults[kind]['aud']]},reason='coordinated audience must be one string')
        add('reject-'+kind+'-wrong-issuer',kind,{'iss':'https://other.identity.invalid'},reason='issuer pinned exactly')
        add('reject-'+kind+'-wrong-audience',kind,{'aud':'other-audience'},reason='audience pinned exactly')
        add('reject-'+kind+'-previous-set28-audience',kind,{'aud':rsa.AUDIENCE if kind=='login' else rsa.RESET_AUDIENCE},reason='new coordinator profile has distinct audiences')
        add('reject-'+kind+'-expired',kind,{'exp':NOW},reason='expiry exclusive')
        add('reject-'+kind+'-future-issued',kind,{'iat':NOW+1,'exp':NOW+300},reason='issuance cannot be future')
        add('reject-'+kind+'-ttl301',kind,{'exp':NOW+296},reason='transport lifetime bounded300seconds')
        add('reject-'+kind+'-milliseconds-overflow',kind,{'exp':(2**63-1)//1000+1},reason='NumericDate conversion must not overflow int64 milliseconds')
        add('reject-'+kind+'-claim-case-alias',kind,{'COORDINATOR_ID':BINDING['coordinator_id']},reason='exact case-sensitive closed claim schema')
        raw=rsa.raw_json(defaults[kind])
        add('reject-'+kind+'-exponent-numericdate',kind,payload=raw.replace(b'"iat":1699999995',b'"iat":1.699999995e9'),reason='NumericDate requires canonical integer decimal, not an exponent')
        for suffix,label in [(b',"provider_version":1}','duplicate'),(b',"\\u0070rovider_version":1}','escaped-duplicate')]:
            add('reject-'+kind+'-'+label,kind,payload=raw[:-1]+suffix,reason='duplicate decoded keys forbidden')
        add('reject-'+kind+'-utf8',kind,payload=raw.replace(b'public-fixture-a',b'public-fixture-\xff'),reason='invalid UTF8 cannot be repaired')
        add('reject-'+kind+'-payload-limit',kind,payload=raw.ljust(8193,b' '),reason='decoded payload bounded8192bytes')
        add('reject-'+kind+'-trailing-json',kind,payload=raw+b'{}',reason='one payload object only')
        for field,value in [('alg','none'),('alg','HS256'),('alg','PS256'),('kid','unknown'),('typ','wrong'),('jku','https://other.invalid/key'),('crit',[])]:
            add('reject-'+kind+'-header-'+field+'-'+str(value),kind,header={**headers[kind],field:value},reason='protected header must be exact RS256/kid/type with no unknown fields')
        for field in headers[kind]:
            h=dict(headers[kind]);del h[field]
            add('reject-'+kind+'-header-missing-'+field,kind,header=h,reason='all three protected header fields required')
        head=rsa.raw_json(headers[kind])
        add('reject-'+kind+'-header-duplicate',kind,header_raw=head[:-1]+b',"typ":"JWT"}',reason='duplicate protected field forbidden')
        add('reject-'+kind+'-header-limit',kind,header_raw=head.ljust(2049,b' '),reason='decoded header bounded2048bytes')
        idfields=['coordinator_id','authority_id']+(['nonce','login_flow_id','original_auth_id','original_flow_id'] if kind=='login' else ['event_id','reset_nonce','provider_operation_id'])
        for field in idfields:
            for label,value in [('short','a'*63),('uppercase','A'*64),('nonhex','z'*64)]:
                add('reject-'+kind+'-'+field+'-'+label,kind,{field:value},reason='IDs/nonces canonical lowercase32bytehex')
    add('reject-login-flow-mismatch',changes={'login_flow_id':NEXT_FLOW},reason='current login flow must match durable expected flow')
    add('reject-login-fresh-original-flow-mismatch',changes={'original_flow_id':NEXT_FLOW},reason='fresh signed original_flow_id must equal signed current login_flow_id before trusted verifier output')
    add('reject-login-nonce-mismatch',changes={'nonce':NEXT_NONCE},reason='nonce must match authority-issued attempt')
    add('reject-login-old-auth-refresh',changes={'iat':NOW,'exp':NOW+300,'auth_time':NOW-301},reason='new transport cannot freshen original authentication')
    add('reject-login-auth-after-issue',changes={'auth_time':NOW-4},reason='auth_time cannot follow iat')
    add('reject-login-mode',changes={'auth_mode':'refresh'},reason='auth_mode only fresh or reuse')
    add('reject-login-exponent-epoch',payload=rsa.raw_json(login()).replace(b'"slug_auth_epoch":1',b'"slug_auth_epoch":1e0'),reason='epoch exponent forbidden')
    add('reject-reset-event-mismatch','reset',{'event_id':NEXT_EVENT},reason='event must match durable expected handle')
    add('reject-reset-nonce-mismatch','reset',{'reset_nonce':NEXT_RESET_NONCE},reason='reset nonce must match issued handle')
    add('reject-reset-stage','reset',{'stage':'abort'},reason='no abort stage exists')
    add('reject-reset-skipped-epoch','reset',{'to_epoch':3},reason='to_epoch must equal from_epoch+1')
    add('reject-reset-epoch-wrap','reset',{'from_epoch':MAX_U64,'to_epoch':1},reason='epoch overflow cannot wrap')
    add('reject-reset-verification-after-issue','reset',{'verified_at':NOW-4},reason='verified_at cannot follow iat')
    add('reject-login-as-reset','reset',token=fresh,reason='login action/audience/type cannot authorize reset')
    add('reject-reset-as-login',token=begin,reason='reset action/audience/type cannot authorize login')
    # The signing bytes still have a valid RSA signature for alg confusion
    # cases above; these framing/signature negatives deliberately do not.
    add('reject-total-limit',token='A'*16385,signed=False,reason='compact token bounded16384bytes')
    parts=fresh.split('.')
    for i,label in enumerate(['header','payload','signature']):
        p=list(parts);p[i]+='='
        add('reject-base64-padding-'+label,token='.'.join(p),signed=False,reason='raw canonical unpadded base64url required')
    sig=bytearray(rsa.unb64(parts[2]));sig[-1]^=1
    add('reject-bad-signature',token='.'.join(parts[:2]+[rsa.b64(sig)]),signed=False,reason='RS256 signature must verify')

    # Index encodings are tested with an explicit synthetic trust digest. Its
    # value is input data, not an invented expected Verifier.ConfigDigest().
    index_tuple=['ab'*32,rsa.ISSUER,'public-fixture-a',BINDING['tenant_id'],BINDING['client_id'],BINDING['connection_id']]
    indices={'original_authentication':tuple_index('SLUG provider original authentication\0v1\0',index_tuple+[ORIGIN]),
             'reset_operation':tuple_index('SLUG provider reset operation\0v1\0',index_tuple+[OPERATION])}
    assert indices['original_authentication']['sha256']!=indices['reset_operation']['sha256']
    same=index_tuple+[ORIGIN]
    indices['same_components_other_domain']=tuple_index('SLUG provider reset operation\0v1\0',same)
    assert indices['same_components_other_domain']['sha256']!=indices['original_authentication']['sha256']
    left=list(same);left[2:4]=['ab','c']
    right=list(same);right[2:4]=['a','bc']
    assert ''.join(left)==''.join(right)
    indices['length_prefix_left']=tuple_index('SLUG provider original authentication\0v1\0',left)
    indices['length_prefix_right']=tuple_index('SLUG provider original authentication\0v1\0',right)
    assert indices['length_prefix_left']['encoded']!=indices['length_prefix_right']['encoded']
    assert indices['length_prefix_left']['sha256']!=indices['length_prefix_right']['sha256']
    assert len({c['name'] for c in cases})==len(cases)
    return {'fixture_only':True,'version':1,'fixed_now_seconds':NOW,
            'contract':'set31-coordinator-contract.md','previous_reference_check':baseline,
            'config':{'version':1,'issuer':rsa.ISSUER,'audience':LOGIN_AUD,'reset_audience':RESET_AUD,'epoch_claim':'slug_auth_epoch','public_jwks':{'keys':[{'kty':'RSA','kid':rsa.KID,'n':rsa.RSA_N,'e':rsa.RSA_E,'alg':'RS256','use':'sig'}]}},
            'binding':BINDING,'cases':cases,'tuple_indices':indices,
            'coordinator_scenarios':scenarios()}


def scenarios():
    # These are explicit acceptance plans, not outcomes of a mock state machine.
    return {'status':'contract expectations; not runtime execution evidence','cases':[
        {'name':'fresh-then-reuse','steps':['Issue/Finish login-fresh; persist accepted origin','Issue/Finish login-reuse-accepted-origin'], 'expected':'reuse may create new session only while original auth freshness/epoch and authority nonce remain valid'},
        {'name':'unknown-origin-reuse','token':'login-reuse-unknown-origin','expected':'reject without session; a valid signature does not create an accepted origin'},
        {'name':'relabel-origin-as-fresh','after':'login-fresh accepted','token':'login-fresh-relabels-known-origin','expected':'conflict; existing origin cannot be rebound to new original flow'},
        {'name':'relabel-origin-epoch','after':'login-fresh accepted','token':'login-reuse-relabels-epoch','expected':'conflict; current account epoch never rewrites an old origin'},
        {'name':'relabel-origin-auth-time','after':'login-fresh accepted','token':'login-reuse-relabels-authentication-time','expected':'conflict; fresh transport preserves original auth_time'},
        {'name':'reset-invalidates-origin','steps':['accept login-fresh','begin and complete reset','attempt login-reuse-accepted-origin with new nonce and unchanged epoch1'], 'expected':'authority refuses old epoch even though signed transport is fresh'},
        {'name':'uncertain-origin','steps':['persist login submitting/prepared origin','authority consumes attempt; lose response before coordinator acceptance','reopen and attempt reuse'], 'expected':'no token reconstruction or origin promotion; explicit new authentication required'},
        {'name':'one-operation-two-handles','steps':['acknowledge reset-begin','present reset-same-operation-new-handle'], 'expected':'conflict; original barrier and hold unchanged; no second mutation permit'},
        {'name':'one-handle-two-operations','steps':['acknowledge reset-begin','present reset-same-handle-new-operation'], 'expected':'conflict; handle remains bound to original operation'},
        {'name':'complete-before-durable-begin','token':'reset-complete','expected':'reject; signature alone cannot clear pending or authorize completion'},
        {'name':'cross-stage-method','steps':['present reset-complete to BeginReset','present reset-begin to CompleteReset'], 'expected':'reject action mismatch; cryptographic validity cannot select another state transition'},
        {'name':'historical-begin-replay','steps':['acknowledge reset-begin','acknowledge reset-complete','replay reset-begin with fresh iat/exp'], 'expected':'return original completed receipt, CanCommit=false, unchanged generation/hold'},
        {'name':'historical-completion-after-later-reset','steps':['complete first event','complete a distinct subsequent event','replay first completion'], 'expected':'return original first receipt; never extend current hold or assign another epoch'},
        {'name':'fresh-transport-old-uncommitted-verification','token':'reset-begin-old-verification','expected':'verifier accepts; new authority barrier refused when original verified_at is stale'},
        {'name':'fresh-transport-old-committed-verification','steps':['persist matching pending begin while verification fresh','later retry same immutable event with fresh transport'], 'expected':'return original pending barrier; do not rewrite verified_at'},
        {'name':'completion-immutable-substitution','after':'acknowledge reset-begin','tokens':['reset-complete-other-principal','reset-complete-changed-verified-time','reset-complete-changed-epochs'],'expected':'reject each against persisted begin; no completion or hold change'},
        {'name':'lost-login-session-ack','after':'completed flow/session but caller loses response','expected':'never replay session token; status is sanitized metadata, not a credential'},
        {'name':'missing-provider-proof','after':'durable pending begin','steps':['wait beyond request expiry','receive ordinary valid login instead of completion'], 'expected':'pending remains; no timeout release or synthetic completion'},
    ]}


if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    mode=p.add_mutually_exclusive_group(required=True)
    mode.add_argument('--generate',action='store_true')
    mode.add_argument('--check',action='store_true')
    args=p.parse_args()
    data=corpus();encoded=json.dumps(data,indent=2,ensure_ascii=True)+'\n'
    target=ROOT/'coordinated_identity_v1.json'
    if args.generate:target.write_text(encoded)
    else:assert target.read_text()==encoded,'coordinated fixture differs'
    print(f"RFC/baseline reproduced; {len(data['cases'])} coordinated cases ({sum(c['verifier_accepted'] for c in data['cases'])} verifier-accepted), {len(data['coordinator_scenarios']['cases'])} unexecuted state scenarios.")
