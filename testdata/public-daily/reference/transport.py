"""Explicit daily trust, embedded legacy evidence and original-local selection."""
from urllib.parse import urlsplit
from core import *
from composition import miner_config, public_key

CLIENT_FIELDS='version,url,token,ca_certificate,public_key,source,settlement_config_id,recipient_script'
WORKER_FIELDS='version,source,public_key,authority_origin,transport_trust_id,admission_origin,admission_trust_id,max_records,max_bytes'
REMOTE_FIELDS='version,connection_id,operator_pin,source,settlement,miners,origin,ca_certificate,expected_local,max_records,max_bytes'
HTTP_STATUS={'invalid_request':400,'unauthorized':401,'forbidden':403,'not_found':404,
             'expired':409,'rejected':422,'too_large':413,'rate_limited':429,
             'busy':503,'capacity':503,'unavailable':503,'outcome_unknown':503,
             'stale_capture':409}

def client(c):
    fields(c,CLIENT_FIELDS);integer(c['version'],2,2);require(c['source']['version']==2)
    public_key(c['public_key']);hexstr(c['settlement_config_id'],64,True);script(c['recipient_script']);hexstr(c['token'],64)
    u=urlsplit(c['url']);require(u.scheme in ('http','https') and u.netloc and not u.path and not u.query and not u.fragment and not u.username)
    require(u.netloc==u.netloc.lower() and (u.scheme!='http' or u.hostname in ('127.0.0.1','::1','localhost')))
    if u.scheme=='http':require(c['ca_certificate']=='')
    manifest_trust(c['url'],c['ca_certificate'])

def worker(c):
    fields(c,WORKER_FIELDS);integer(c['version'],2,2);require(c['source']['version']==2);public_key(c['public_key'])
    integer(c['max_records'],1,4096);integer(c['max_bytes'],64<<20,1<<30)
    hexstr(c['transport_trust_id'],64,True);hexstr(c['admission_trust_id'],64,True)

def worker_from(c,admission_origin=None):
    admission_origin=admission_origin or c['url'];_,at=manifest_trust(admission_origin,c['ca_certificate']);_,pt=daily_trust(c)
    return dict(version=2,source=c['source'],public_key=c['public_key'],authority_origin=c['url'],transport_trust_id=pt,admission_origin=admission_origin,admission_trust_id=at,max_records=64,max_bytes=64<<20)

def inherited_wire_template(original, order):
    # Input dictionaries are semantic fixture recipes, not a serialized Go
    # transport. Select the frozen pre-existing field order independently.
    def arrange(value, names, exact=True):
        require(type(value) is dict and set(value) <= set(names), 'wire fields')
        if exact: require(set(value) == set(names), 'required wire fields')
        return {name: clone(value[name]) for name in names if name in value}
    result=arrange(original,order['template'])
    result['context']=arrange(result['context'],order['context'])
    block=arrange(result['block'],order['block'],False)
    required=set(order['block'])-set(order['block_omit_empty'])
    require(required <= set(block),'required block fields')
    # All inherited present optional fields must already satisfy omitempty.
    # Refuse contrary input instead of silently changing its semantic content.
    for name in order['block_omit_empty']:
        if name in block:
            if name in order['block_optional_pointer']:require(block[name] is not None,'present pointer')
            else:require(block[name] not in (None,0,'',[]),'present optional value')
    block['transactions']=[arrange(tx,order['transaction']) for tx in block['transactions']]
    if 'coinbaseaux' in block:block['coinbaseaux']=arrange(block['coinbaseaux'],order['aux'])
    if 'coinbasetxn' in block:block['coinbasetxn']=arrange(block['coinbasetxn'],order['transaction'])
    result['block']=block
    require(result==original,'wire ordering changed semantic values')
    return result

def source_for(s):
    j=s['job'];p=j['context']
    return dict(version=2,authority_pin=dict(version=5,network=p['network'],genesis=p['genesis'],authority_id=j['authority_id'],descriptor_id=j['descriptor_id']),policy={k:p[k] for k in ('version','network','genesis','dna_version','prospect_target')},payout_address=s['payout_address'],pool_target='f'*64,max_works=8,max_submissions=32,max_bytes=64<<20)

def money_expected(c):
    return dict(binding_id=source_id(c['source']),settlement_config_id=c['settlement_config_id'],recipient_script=c['recipient_script'],gym_id=gym_id(c['source']['policy']['network'],c['source']['policy']['genesis'],c['public_key']))

def envelope(c,key,value):
    e=money_expected(c)
    return dict(version=2,binding_id=e['binding_id'],settlement_config_id=e['settlement_config_id'],recipient_script=e['recipient_script'],**{key:value})

def bound(v,c,key,original):
    fields(v,'version,binding_id,settlement_config_id,recipient_script,'+key)
    integer(v['version'],2,2);e=money_expected(c)
    for f in ('binding_id','settlement_config_id','recipient_script'):require(v[f]==e[f],'binding')
    require(v[key]==original,'embedded original evidence')

def error_response(v,status=None):
    fields(v,'version,error');integer(v['version'],2,2)
    e=v['error'];fields(e,'code,outcome_unknown')
    require(e['code'] in ('invalid_request','unauthorized','forbidden','not_found',
                         'expired','rejected','too_large','rate_limited','busy',
                         'capacity','unavailable','outcome_unknown','stale_capture'))
    require(type(e['outcome_unknown']) is bool and e['outcome_unknown']==(e['code']=='outcome_unknown'))
    if status is not None:require(HTTP_STATUS[e['code']]==status,'HTTP error status')
    require(len(canon(v))+1<=65536,'error size')

def receipt(s,case):
    j=s['job'];block=case['qualifies_block'];at=j['issued_at_ms']+100
    return dict(version=1,sequence=1,accepted_sequence=1,job_id=j['id'],gym_id=j['gym_id'],nonce=case['nonce'],pool_target='f'*64,header=case['header'],work_hash=case['work_hash'],duplicate_key=case['submission_key'],pool=True,prospect=case['qualifies_prospect'],block=block,block_height=j['context']['parent_height']+1 if block else 0,received_at_ms=at,accepted_at_ms=at,acceptance='current-parent',block_state='canonical-observed' if block else 'none',block_observed_at_ms=at if block else 0)

def remote_valid(c,local):
    fields(c,REMOTE_FIELDS);integer(c['version'],2,2);hexstr(c['connection_id'],64,True)
    require(c['expected_local']==local,'original local binding')
    require(local['version']==1 and local['manager_schema']==14)
    require(local['original_source_binding_id']==local['recipient']['source_binding_id'])
    require(c['source']['version']==2 and c['settlement']['version']==2)
    require(local['recipient']['config']['source']['policy']==c['source']['policy'])
    require(c['operator_pin']['source']==c['source']['authority_pin'])
    m=c['miners'];miner_config(m)
    require(m['operator_profile_id']==profile_id(c['operator_pin']) and m['source_id']==source_id(c['source']) and m['settlement_config_id']==settlement_id(c['settlement']))
    require([dict(gym_id=e['gym_id'],script=e['payee_script']) for e in m['enrollments']]==c['settlement']['destinations'],'full enrollment/destination binding')
    identity=local['recipient']['config']['identity'];script_value=local['recipient']['recipient']['script']
    selected=[e for e in m['enrollments'] if e['gym_id']==identity['gym_id']]
    dest=[d for d in c['settlement']['destinations'] if d['gym_id']==identity['gym_id']]
    require(len(selected)==len(dest)==1 and selected[0]['public_key']==identity['gym_public_key'] and selected[0]['payee_script']==dest[0]['script']==script_value)
    require(all(d['script']!=c['settlement']['change_script'] for d in c['settlement']['destinations']))
    require(c['origin'].startswith('https://') and c['ca_certificate'])
    integer(c['max_records'],1,4096);integer(c['max_bytes'],64<<20,1<<30);require(len(canon(c))<=262144)

def remote_client(c,token):
    return dict(version=2,url=c['origin'],token=token,ca_certificate=c['ca_certificate'],public_key=c['expected_local']['recipient']['config']['identity']['gym_public_key'],source=c['source'],settlement_config_id=settlement_id(c['settlement']),recipient_script=c['expected_local']['recipient']['recipient']['script'])

def credential_record(r,cid,gym,audience):
    fields(r,'version,config_id,gym_id,audience,generation,expires_at_ms,token');integer(r['version'],2,2)
    require(r['config_id']==cid and r['gym_id']==gym and r['audience']==audience and audience in ('pool','admission'))
    integer(r['generation'],1,0xffffffff);integer(r['expires_at_ms'],1,MAX_TIME);hexstr(r['token'],64,True)
    require(len(canon(r))+1<=2048)

def produce_transport(inputs,composition):
    legacy=inputs['identities'];oldc=legacy['client'];pre,mid=manifest_trust(oldc['url'],oldc['ca_certificate'])
    require(pre.hex()==legacy['manifest_http_preimage_hex'] and mid==legacy['manifest_http_trust_id'],'legacy HTTP anchor')
    oldsid=sha(b'SLUG pool service source\0v1\0'+canon(oldc['source']));require(oldsid==legacy['source_id'])
    require(sha(b'SLUG pool transport trust\0v1\0'+mid.encode()+oldsid.encode()+oldc['public_key'].encode())==legacy['pool_transport_trust_id'])
    errors=Cases()
    for code in ('stale_capture','outcome_unknown','busy'):
        e=dict(version=2,error=dict(code=code,outcome_unknown=code=='outcome_unknown'))
        errors.add(code,e,error_response,http_status=HTTP_STATUS[code])
        errors.bad(code+'-wrong-flag',e,error_response,lambda v:v['error'].update(outcome_unknown=not v['error']['outcome_unknown']),http_status=HTTP_STATUS[code])
    e=dict(version=2,error=dict(code='stale_capture',outcome_unknown=False))
    errors.bad('unknown-code',e,error_response,lambda v:v['error'].update(code='manual_clear'))
    errors.bad('v1-error',e,error_response,lambda v:v.update(version=1))
    cfg=composition['positives'][1]['setup']['settlement'];wire=[];cases=Cases();work_cases=[]
    for s in inputs['pool_share']['sources']:
        case=next(x for x in inputs['pool_share']['cases'] if x['source_candidate']==s['name'] and x['nonce']==0)
        require(s['job']['version']==2,'embedded original job version')
        c=dict(version=2,url=oldc['url'],token=oldc['token'],ca_certificate='',public_key=s['job']['public_key'],source=source_for(s),settlement_config_id=settlement_id(cfg),recipient_script=inputs['daily_actual']['accounts'][0]['payee_script'])
        client(c)
        order=inputs['work_wire_order']
        require(list(s['job'])==order['job'] and list(s['job']['context'])==order['context'],'original job/context order')
        work=dict(job=clone(s['job']),template=inherited_wire_template(s['template'],order));sub=receipt(s,case)
        require(work==dict(job=s['job'],template=s['template']),'all original work values unchanged')
        # This verifies old independent digest identity; it never rehashes PoW.
        require(sha(b'SLUG pool submission\0v1\0'+bytes.fromhex(case['header']))==case['submission_key'])
        for key,value in [('work',work),('submission',sub)]:
            v=envelope(c,key,value);pred=lambda x,c=c,key=key,value=value:bound(x,c,key,value)
            cases.add(s['name']+'-'+key,v,pred,scope='original immutable job/proof; conditional receipt status and selected wrapper',expected=money_expected(c))
            if s['name']=='coinbase-only':
                for f in ('binding_id','settlement_config_id','recipient_script'):
                    cases.bad('wrong-'+key+'-'+f,v,pred,lambda x,f=f:x.update({f:'0'*len(x[f])}),expected=money_expected(c),issue_submit_outcome='unknown',read_outcome='unavailable')
                cases.bad('missing-'+key+'-money',v,pred,lambda x:x.pop('settlement_config_id'))
        p,t=daily_trust(c);wire.append(dict(name=s['name'],client=c,client_hex=canon(c).hex(),trust_preimage_hex=p.hex(),trust_id=t,worker=worker_from(c)))
        work_cases.append(dict(source=s,case=case,client=c,work=work,submission=sub))
    remotes=[];remote_cases=Cases();credential_cases=Cases()
    for row in inputs['remote']['positives'][:2]:
        prior=row['config'];c=clone(prior);c['version']=2;c['source']['version']=2;c['settlement']['version']=2;c['miners']['version']=2
        c['miners']['source_id']=source_id(c['source']);c['miners']['settlement_config_id']=settlement_id(c['settlement'])
        local=clone(prior['expected_local']);remote_valid(c,local)
        cid=ident('SLUG remote pool attachment','v2',c)
        intent=canon(dict(version=2,application='slug-remote-pool-worker',config_id=cid,config=c))+b'\n'
        ready=canon(dict(version=2,application='slug-remote-pool-worker',config_id=cid,intent_sha256=sha(intent)))+b'\n'
        dc=remote_client(c,'97'*32);client(dc);p,t=daily_trust(dc);wc=worker_from(dc);wc.update(max_records=c['max_records'],max_bytes=c['max_bytes'])
        require(c['expected_local']==prior['expected_local'] and c['expected_local']['original_source_binding_id']!=source_id(c['source']))
        remotes.append(dict(name=row['name'],config=c,config_id=cid,canonical_hex=canon(c).hex(),intent_hex=intent.hex(),ready_hex=ready.hex(),daily_client=dc,worker=wc,trust_preimage_hex=p.hex(),trust_id=t,original_local_unchanged=True))
        remote_cases.add(row['name'],c,lambda x,local=local:remote_valid(x,local))
        gym=c['expected_local']['recipient']['config']['identity']['gym_id']
        for audience,token in [('pool','97'*32),('admission','98'*32)]:
            r=dict(version=2,config_id=cid,gym_id=gym,audience=audience,generation=1,expires_at_ms=2000000000000,token=token)
            pred=lambda x,cid=cid,gym=gym,audience=audience:credential_record(x,cid,gym,audience)
            credential_cases.add(row['name']+'-'+audience,r,pred,file_hex=(canon(r)+b'\n').hex())
            credential_cases.bad(row['name']+'-'+audience+'-v1',r,pred,lambda v:v.update(version=1))
    c=remotes[0]['config'];local=c['expected_local'];pred=lambda x:remote_valid(x,local)
    for name,mut in [('v1',lambda v:v.update(version=1)),('repinned-local',lambda v:v['expected_local'].update(original_source_binding_id=source_id(v['source']))),('foreign-wallet',lambda v:v['expected_local'].update(wallet_owner_id='fe'*32)),('wrong-payee',lambda v:v['miners']['enrollments'][0].update(payee_script='0014'+'fe'*20)),('wrong-source-pin',lambda v:v['miners'].update(source_id='fe'*32)),('limit-over',lambda v:v.update(max_records=4097))]:remote_cases.bad(name,c,pred,mut)
    # Trust is public identity, independent of credentials; both money selections matter.
    dc=clone(remotes[0]['daily_client']);before=daily_trust(dc)[1];dc['token']='99'*32;require(daily_trust(dc)[1]==before)
    changes=[]
    for name,value in [('settlement_config_id','ed'*32),('recipient_script','0014'+'ed'*20),('url','https://other.example.test:3443')]:
        v=clone(dc);v[name]=value;client(v);new=daily_trust(v)[1];require(new!=before);changes.append(dict(field=name,client=v,trust_id=new))
    require(accepted(error_response,e,409) and not accepted(error_response,e,503),'selected stale_capture status')
    return dict(wire_identities=wire,envelope_cases=cases.rows,error_cases=errors.rows,error_status_map=HTTP_STATUS,stale_status_refusal=dict(value=e,http_status=503,valid=False),remote_positives=remotes,remote_cases=remote_cases.rows,local_credential_cases=credential_cases.rows,trust_changes=changes,token_rotation_trust=before),work_cases
