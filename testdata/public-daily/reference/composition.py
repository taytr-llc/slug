"""Conditional public composition from independently qualified public material."""
from core import *

ROLES = ('authority', 'publication', 'ownership', 'wallet', 'runtime', 'scheduler', 'credentials')
POLICY_FIELDS = 'version,scope,request_id,network,genesis,descriptor_id,checkpoint_public_key,mining,limits,enrollments,schedule,fee,transport,maintenance_interval_ms,unlock_lifetime_ms,unlock_renewal_ms'
SETUP_FIELDS = 'version,runtime_id,authority,ownership_store_id,pool,round_limits,settlement,schedule,miners,wallet_guard,node,ingress,maintenance_interval_ms,unlock_lifetime_ms,unlock_renewal_ms'
MINER_FIELDS = 'version,credential_id,runtime_id,operator_profile_id,source_id,settlement_config_id,enrollments'

def public_key(key):
    require(type(key) is str and re.fullmatch('0[23][0-9a-f]{64}', key), 'key')
    x = int(key[2:], 16)
    p = (1 << 256) - (1 << 32) - 977
    y2 = (x*x*x+7) % p
    require(x < p and pow(pow(y2, (p+1)//4, p), 2, p) == y2, 'key')

def valid_policy(p, inherited):
    fields(p, POLICY_FIELDS)
    require(p['version'] == 2 and type(p['version']) is int, 'mode')
    require(p['scope'] == 'phase-a-finite-operator', 'scope')
    hexstr(p['request_id'], 64, True)
    for k in ('network', 'genesis', 'descriptor_id', 'checkpoint_public_key'):
        require(p[k] == inherited[k], 'chain/descriptor/checkpoint')
    fields(p['limits'], 'max_works,max_submissions,pool_max_bytes,max_rounds,max_batches,round_max_bytes')
    l = p['limits']
    for k, high in [('max_works',8192),('max_submissions',16384),('max_rounds',16384),('max_batches',32767)]:
        integer(l[k], 1, high)
    for k in ('pool_max_bytes','round_max_bytes'):
        integer(l[k], 4096, 8 << 30)
    require(l['max_rounds'] >= l['max_works'], 'round coverage')
    require(type(p['enrollments']) is list and 1 <= len(p['enrollments']) <= 64, 'enrollments')
    last = ''; seen_keys = set(); seen_scripts = set()
    for e in p['enrollments']:
        fields(e, 'public_key,payee_script'); public_key(e['public_key']); script(e['payee_script'])
        gym = gym_id(p['network'],p['genesis'],e['public_key'])
        require(gym > last and e['public_key'] not in seen_keys and e['payee_script'] not in seen_scripts, 'enrollment order/duplicates')
        last = gym; seen_keys.add(e['public_key']); seen_scripts.add(e['payee_script'])
    fields(p['schedule'], 'daily_utc_minute,retry_ms')
    integer(p['schedule']['daily_utc_minute'],0,1439); integer(p['schedule']['retry_ms'],1000,3600000)
    # The inherited non-limit mining, fee, transport and custody predicates are
    # held at independently validated literal values in this finite family.
    for k in ('mining','fee','transport','maintenance_interval_ms','unlock_lifetime_ms','unlock_renewal_ms'):
        require(p[k] == inherited[k], 'inherited public predicate')
    require(len(canon(p)) <= 65536, 'size')

def compose(old, p):
    valid_policy(p, old['policy'])
    s = clone(old['setup']); s['version'] = 2
    ids = {r:sha(frame('SLUG pool provision identity','v2',bytes.fromhex(p['request_id'])+r.encode('ascii'))) for r in ROLES}
    s['runtime_id'] = ids['runtime']; s['ownership_store_id'] = ids['ownership']
    s['authority']['source']['authority_id'] = ids['authority']
    s['authority']['publication_id'] = ids['publication']
    pool = s['pool']; pool['version'] = 2; pool['authority_pin'] = clone(s['authority']['source'])
    l = p['limits']
    pool.update(max_works=l['max_works'],max_submissions=l['max_submissions'],max_bytes=l['pool_max_bytes'])
    s['round_limits'].update(version=2,max_rounds=l['max_rounds'],max_bytes=l['round_max_bytes'])
    dest = [dict(gym_id=gym_id(p['network'],p['genesis'],e['public_key']),script=e['payee_script']) for e in p['enrollments']]
    require(all(x['script'] != s['settlement']['change_script'] for x in dest), 'change/payee collision')
    s['settlement'].update(version=2,destinations=dest,max_rounds=l['max_rounds'],max_batches=l['max_batches'])
    sid, cid, pid = source_id(pool), settlement_id(s['settlement']), profile_id(s['authority'])
    s['wallet_guard'].update(wallet_owner_id=ids['wallet'],operator_profile_id=pid,source_binding_id=sid,settlement_config_id=cid)
    s['schedule'].update(version=2,scheduler_id=ids['scheduler'],runtime_id=ids['runtime'],settlement_binding_id=scheduler_id(s['settlement'],pool),**p['schedule'])
    enroll = [dict(public_key=e['public_key'],gym_id=d['gym_id'],payee_script=e['payee_script']) for e,d in zip(p['enrollments'],dest)]
    s['miners'].update(version=2,credential_id=ids['credentials'],runtime_id=ids['runtime'],operator_profile_id=pid,source_id=sid,settlement_config_id=cid,enrollments=enroll)
    s['ingress']['version'] = 2
    fields(s, SETUP_FIELDS)
    policy_id = ident('SLUG pool provision policy','v2',p)
    setup_id = ident('SLUG pool operator setup','v2',s)
    guard_id = ident('SLUG operator pool wallet guard','v1',s['wallet_guard'])
    created = old['public_material']['created_at_ms']
    intent = dict(version=2,policy=p,policy_id=policy_id,certificate_sha256=old['certificate']['certificate_sha256'],created_at_ms=created)
    ready = dict(version=2,request_id=p['request_id'],policy_id=policy_id,setup_id=setup_id,wallet_guard_id=guard_id,created_at_ms=created)
    backup = clone(old['backup']); backup.update(version=2,request_id=p['request_id'],policy_id=policy_id,setup_id=setup_id,wallet_guard=s['wallet_guard'],wallet_guard_id=guard_id)
    return dict(policy=p,setup=s,namespace_ids=ids,policy_id=policy_id,setup_id=setup_id,
                source_id=sid,settlement_config_id=cid,operator_profile_id=pid,
                settlement_binding_id=s['schedule']['settlement_binding_id'],wallet_guard_id=guard_id,
                miner_config_id=ident('SLUG pool miner enrollment','v2',s['miners']),
                policy_hex=canon(p).hex(),setup_hex=canon(s).hex(),
                policy_preimage_hex=frame('SLUG pool provision policy','v2',canon(p)).hex(),
                setup_preimage_hex=frame('SLUG pool operator setup','v2',canon(s)).hex(),
                operator_intent_hex=(canon(s)+b'\n').hex(),
                operator_ready_hex=(canon(dict(version=2,runtime_id=ids['runtime'],setup_id=setup_id))+b'\n').hex(),
                provision_intent_hex=(canon(intent)+b'\n').hex(),provision_ready_hex=(canon(ready)+b'\n').hex(),
                published_fixture_backup_hex=(canon(backup)+b'\n').hex())

def miner_config(c):
    fields(c,MINER_FIELDS); integer(c['version'],2,2)
    for k in list(c)[1:6]: hexstr(c[k],64,True)
    es = c['enrollments']; require(type(es) is list and 1 <= len(es) <= 64)
    last=''; keys=set(); scripts=set()
    for e in es:
        fields(e,'public_key,gym_id,payee_script'); public_key(e['public_key']); hexstr(e['gym_id'],64,True); script(e['payee_script'])
        require(e['gym_id'] > last and e['public_key'] not in keys and e['payee_script'] not in scripts)
        last=e['gym_id']; keys.add(e['public_key']); scripts.add(e['payee_script'])
    require(len(canon(c))+1 <= 65536,'size')

def miner_state(s):
    fields(s,'version,config,revision,clock_ms,credentials,history'); integer(s['version'],2,2); miner_config(s['config'])
    integer(s['revision'],1,0xffffffff); integer(s['clock_ms'],1,MAX_TIME)
    cs,hs=s['credentials'],s['history']
    require(type(cs) is list and len(cs)<=128 and type(hs) is list and len(hs)<=4096 and len(hs)<=s['revision']-1)
    tokens=set(); reuse=set(); pairs=set()
    for h in hs:
        fields(h,'token_hash,reuse_hash'); hexstr(h['token_hash'],64,True); hexstr(h['reuse_hash'],64,True)
        require(h['token_hash'] not in tokens and h['reuse_hash'] not in reuse)
        tokens.add(h['token_hash']); reuse.add(h['reuse_hash']); pairs.add((h['token_hash'],h['reuse_hash']))
    gyms={e['gym_id'] for e in s['config']['enrollments']}; last=''; used=set(); total=0
    for c in cs:
        fields(c,'gym_id,audience,generation,token_hash,reuse_hash,issued_at_ms,expires_at_ms,revoked_at_ms')
        order=c['gym_id']+':'+c['audience']; require(c['gym_id'] in gyms and c['audience'] in ('pool','admission') and order>last)
        integer(c['generation'],1,4096); integer(c['issued_at_ms'],1,s['clock_ms'])
        integer(c['expires_at_ms'],c['issued_at_ms']+1,min(MAX_TIME,c['issued_at_ms']+2592000000))
        require(type(c['revoked_at_ms']) is int and (c['revoked_at_ms']==0 or c['issued_at_ms']<=c['revoked_at_ms']<=s['clock_ms']))
        pair=(c['token_hash'],c['reuse_hash']); require(pair in pairs and pair not in used)
        used.add(pair); total+=c['generation']; last=order
    require(total==len(hs) and len(canon(s))<=1048576)

def credentials(config):
    cid=ident('SLUG pool miner enrollment','v2',config); gym=config['enrollments'][0]['gym_id']
    cases=Cases(); state=dict(version=2,config=config,revision=1,clock_ms=2000,credentials=[],history=[])
    cases.add('empty',state,miner_state)
    hashes=[]
    for audience,token in [('pool','97'*32),('admission','98'*32)]:
        tb=dict(config_id=cid,audience=audience,token=token); rb=dict(config_id=cid,token=token)
        h=dict(token_hash=ident('SLUG pool miner credential','v2',tb),reuse_hash=ident('SLUG pool miner token reuse','v2',rb))
        hashes.append(dict(audience=audience,token=token,**h,token_preimage_hex=frame('SLUG pool miner credential','v2',canon(tb)).hex(),reuse_preimage_hex=frame('SLUG pool miner token reuse','v2',canon(rb)).hex()))
        state['history'].append(h); state['revision']+=1
        state['credentials'].append(dict(gym_id=gym,audience=audience,generation=1,**h,issued_at_ms=1000,expires_at_ms=1000+2592000000,revoked_at_ms=0))
        state['credentials'].sort(key=lambda c:(c['gym_id'],c['audience']))
        cases.add('active-'+audience,state,miner_state)
    state['credentials'][0]['revoked_at_ms']=2000; state['revision']+=1
    cases.add('revoked-admission-retains-history',state,miner_state)
    tb=dict(config_id=cid,audience='pool',token='99'*32);rb=dict(config_id=cid,token='99'*32)
    rotated=dict(token_hash=ident('SLUG pool miner credential','v2',tb),reuse_hash=ident('SLUG pool miner token reuse','v2',rb))
    hashes.append(dict(audience='pool',token='99'*32,**rotated,token_preimage_hex=frame('SLUG pool miner credential','v2',canon(tb)).hex(),reuse_preimage_hex=frame('SLUG pool miner token reuse','v2',canon(rb)).hex()))
    state['history'].append(rotated);state['revision']+=1
    pool=next(c for c in state['credentials'] if c['audience']=='pool')
    pool.update(generation=2,**rotated,issued_at_ms=2000,expires_at_ms=2000+2592000000,revoked_at_ms=0)
    cases.add('rotated-pool-generation-two-keeps-old-hashes',state,miner_state)
    mutations=[('v1',lambda v:v.update(version=1)),('missing-history',lambda v:v['history'].pop()),('duplicate-history',lambda v:v['history'].append(clone(v['history'][0]))),('unknown-audience',lambda v:v['credentials'][0].update(audience='account')),('future-issuance',lambda v:v['credentials'][0].update(issued_at_ms=2001)),('over-lifetime',lambda v:v['credentials'][0].update(expires_at_ms=1001+2592000000)),('generation-not-covered',lambda v:v['credentials'][0].update(generation=2)),('reordered-credentials',lambda v:v['credentials'].reverse()),('zero-revision',lambda v:v.update(revision=0))]
    for name,mut in mutations: cases.bad(name,state,miner_state,mut)
    return dict(config=config,config_id=cid,hashes=hashes,cases=cases.rows)

def produce_composition(inputs,recipe):
    original=inputs['provision']['positives']; old=next(x for x in original if x['name']=='official-leading-zero-account')
    for row in original:
        require(ident('SLUG pool provision policy','v1',row['policy'])==row['policy_id'],'legacy policy anchor')
        require(ident('SLUG pool operator setup','v1',row['setup'])==row['setup_id'],'legacy setup anchor')
        require(profile_id(row['setup']['authority'])==row['operator_profile_id'],'legacy profile anchor')
    p=clone(old['policy']);p['version']=2;p['limits']=clone(recipe['limits'])
    out=[]
    for name,n in [('one-enrollment',1),('two-enrollments',2),('64-enrollments',64)]:
        q=clone(p)
        if n==64:
            q['enrollments']=[dict(public_key=a['public_key'],payee_script=a['payee_script']) for a in sorted(inputs['daily_actual']['accounts'],key=lambda a:a['gym_id'])]
        else:q['enrollments']=q['enrollments'][:n]
        out.append(dict(name=name,scope='conditional inherited public key/certificate material; fresh identities only',**compose(old,q)))
    q=clone(p);q['request_id']=recipe['alternate_request_id'];out.append(dict(name='new-request',scope='conditional composition',**compose(old,q)))
    q=clone(p);q['schedule']['daily_utc_minute']=6;out.append(dict(name='changed-selected-schedule',scope='conditional composition',**compose(old,q)))
    check=lambda q:valid_policy(q,old['policy']); cases=Cases(); cases.add('baseline',p,check)
    mutations=[('policy-v1',lambda v:v.update(version=1)),('scope',lambda v:v.update(scope='continuous')),('zero-request',lambda v:v.update(request_id='0'*64)),('network',lambda v:v.update(network=0)),('genesis',lambda v:v.update(genesis='0'*64)),('descriptor',lambda v:v.update(descriptor_id='0'*64)),('checkpoint',lambda v:v.update(checkpoint_public_key='0'*64)),('no-enrollment',lambda v:v.update(enrollments=[])),('duplicate-enrollment',lambda v:v['enrollments'].append(clone(v['enrollments'][0]))),('order',lambda v:v['enrollments'].reverse()),('works-over',lambda v:v['limits'].update(max_works=8193)),('submissions-over',lambda v:v['limits'].update(max_submissions=16385)),('rounds-over',lambda v:v['limits'].update(max_rounds=16385)),('batches-over',lambda v:v['limits'].update(max_batches=32768)),('bytes-over',lambda v:v['limits'].update(pool_max_bytes=(8<<30)+1)),('rounds-below-work',lambda v:v['limits'].update(max_rounds=7)),('utc-over',lambda v:v['schedule'].update(daily_utc_minute=1440))]
    for name,mut in mutations:cases.bad(name,p,check,mut)
    too_many=clone(out[2]['policy']);too_many['enrollments'].append(clone(too_many['enrollments'][0]));cases.add('65-enrollments',too_many,check,False)
    collision=clone(p);collision['enrollments'][0]['payee_script']=old['setup']['settlement']['change_script']
    require(not accepted(lambda v:compose(old,v),collision),'collision refusal')
    return dict(positives=out,policy_cases=cases.rows,composition_refusals=[dict(name='change-payee-collision',policy=collision,valid=False)],legacy_anchor_count=len(original)),old
