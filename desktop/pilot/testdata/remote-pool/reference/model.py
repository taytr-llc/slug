"""Independent bounded remote-attachment composition; no Go or secret inputs.

Inherited public profile/recipient validity is conditional on the locked input
corpora. This producer independently computes new cross-bindings/ordered bytes
and IDs. It does not claim a second implementation of X.509 or all old codecs.
"""
import base64
import copy
import hashlib
import json
import re
import struct
from urllib.parse import urlsplit

MAX_CONFIG = 262144
APP = 'slug-remote-pool-worker'
CONFIG_FIELDS = ['version','connection_id','operator_pin','source','settlement','miners','origin','ca_certificate','expected_local','max_records','max_bytes']
LOCAL_FIELDS = ['version','manager_schema','wallet_owner_id','allocation_binding_id','original_source_binding_id','recipient']
CREDENTIAL_FIELDS = ['version','config_id','gym_id','audience','generation','expires_at_ms','token']

def raw(v):
    return json.dumps(v,separators=(',',':'),ensure_ascii=True).encode('ascii')

def sha(v):
    return hashlib.sha256(v).hexdigest()

def H(label,v):
    b=raw(v)
    return sha(label.encode('ascii')+b'\0v1\0'+struct.pack('<I',len(b))+b)

def ID(s):
    return isinstance(s,str) and re.fullmatch('[0-9a-f]{64}',s) is not None and s!='00'*32

def integer(x,lo,hi):
    return type(x) is int and lo<=x<=hi

def source_id(s):
    return sha(b'SLUG pool service source\0v1\0'+raw(s))

def canonical_ca(pem):
    parts=re.findall(r'-----BEGIN CERTIFICATE-----\n([A-Za-z0-9+/=\n]+)-----END CERTIFICATE-----\n',pem)
    if not parts: raise ValueError('public certificate input')
    ders={sha(base64.b64decode(x,validate=False)):base64.b64decode(x,validate=False) for x in parts}
    result=''
    for digest,der in sorted(ders.items()):
        b=base64.b64encode(der).decode('ascii')
        result+='-----BEGIN CERTIFICATE-----\n'+'\n'.join(b[i:i+64] for i in range(0,len(b),64))+'\n-----END CERTIFICATE-----\n'
    return result,list(sorted(ders))

def trust(c):
    _,certs=canonical_ca(c['ca_certificate'])
    origin=c['origin'].encode('ascii')
    admission=sha(b'SLUG manifest transport trust\0v1\0'+struct.pack('<I',len(origin))+origin+b'\3'+struct.pack('<I',len(certs))+b''.join(bytes.fromhex(s) for s in certs))
    pool=sha(b'SLUG pool transport trust\0v1\0'+admission.encode()+source_id(c['source']).encode()+c['expected_local']['recipient']['config']['identity']['gym_public_key'].encode())
    return pool,admission

def worker(c):
    pool,admission=trust(c)
    return dict(version=1,source=c['source'],public_key=c['expected_local']['recipient']['config']['identity']['gym_public_key'],authority_origin=c['origin'],transport_trust_id=pool,admission_origin=c['origin'],admission_trust_id=admission,max_records=c['max_records'],max_bytes=c['max_bytes'])

def config_id(c): return H('SLUG remote pool attachment',c)

def marker_bytes(c):
    cid=config_id(c)
    intent=raw(dict(version=1,application=APP,config_id=cid,config=c))+b'\n'
    ready=raw(dict(version=1,application=APP,config_id=cid,intent_sha256=sha(intent)))+b'\n'
    return intent,ready

def validate(c,profile,originals,ca):
    try:
        assert list(c)==CONFIG_FIELDS and len(raw(c))<=MAX_CONFIG
        assert c['version']==1 and ID(c['connection_id']) and c['operator_pin']==profile
        local=c['expected_local']; assert list(local)==LOCAL_FIELDS
        assert local['version']==1 and local['manager_schema']==14
        assert ID(local['wallet_owner_id']) and ID(local['allocation_binding_id'])
        r=local['recipient']; fixture=r['config']['fixture']; original=originals[fixture]
        # Recipient grammar/derivation is inherited independently. Original
        # source can vary only in the explicit mismatch fixture below.
        assert r['version']==1 and r['config']['version']==1
        assert r['config']['identity']==original['config']['identity'] and r['recipient']==original['recipient']
        assert ID(r['config']['account_id']) and ID(r['config']['account_authority_id'])
        assert local['original_source_binding_id']==r['source_binding_id']==source_id(r['config']['source'])
        assert r['config']['source']['policy']==c['source']['policy']
        assert c['source']['authority_pin']==profile['source']
        m=c['miners']; s=c['settlement']; identity=r['config']['identity']
        assert m['version']==1 and all(ID(m[k]) for k in ['credential_id','runtime_id','operator_profile_id','source_id','settlement_config_id'])
        assert m['operator_profile_id']==H('SLUG fighter ownership pin',profile)
        assert m['source_id']==source_id(c['source']) and m['settlement_config_id']==H('SLUG operator pool settlement config',s)
        en=m['enrollments']; dest=s['destinations']; assert 1<=len(en)<=64 and len(en)==len(dest)
        assert s['version']==1 and integer(s['max_rounds'],1,256) and integer(s['max_batches'],1,366)
        assert [x['gym_id'] for x in en]==sorted(set(x['gym_id'] for x in en))
        assert len(set(x['public_key'] for x in en))==len(en) and len(set(x['payee_script'] for x in en))==len(en)
        assert re.fullmatch('0014[0-9a-f]{40}',s['change_script'])
        assert all(d['script']!=s['change_script'] for d in dest)
        selected=[]
        for e,d in zip(en,dest):
            assert re.fullmatch('0014[0-9a-f]{40}',e['payee_script'])
            gym=sha(b'SLUG gym identity\0v1\0'+struct.pack('<I',c['source']['policy']['network'])+bytes.fromhex(c['source']['policy']['genesis'])+bytes.fromhex(e['public_key']))
            assert gym==e['gym_id']==d['gym_id'] and e['payee_script']==d['script']
            if gym==identity['gym_id']: selected.append(e)
        assert len(selected)==1 and selected[0]['public_key']==identity['gym_public_key'] and selected[0]['payee_script']==r['recipient']['script']
        u=urlsplit(c['origin']); assert u.scheme=='https' and u.hostname and u.hostname==u.hostname.lower() and u.hostname.encode().isascii()
        assert not (u.path or u.query or u.fragment or u.username or u.password) and c['origin']==('https://'+u.netloc) and u.netloc==u.netloc.lower()
        if u.port is not None: assert 1<=u.port<=65535
        assert c['ca_certificate']==ca and canonical_ca(ca)[0]==ca
        assert integer(c['max_records'],1,4096) and integer(c['max_bytes'],67108864,1073741824)
        return True
    except (AssertionError,KeyError,TypeError,ValueError): return False

def credential(c,a,recipe):
    return dict(version=1,config_id=config_id(c),gym_id=c['expected_local']['recipient']['config']['identity']['gym_id'],audience=a,generation=recipe['credential_generations'][a],expires_at_ms=recipe['credential_expires_at_ms'],token=recipe['synthetic_tokens'][a])

def build(inputs):
    setups=json.loads((inputs/'operator-setup-v1.json').read_bytes())
    recipe=json.loads((inputs/'recipe.json').read_bytes())
    index=recipe['setup_case_index']; assert integer(index,0,len(setups['positives'])-1)
    selected=setups['positives'][index]; assert selected['name']==recipe['setup_case_name']
    setup=selected['setup']
    ready=json.loads((inputs/'original-recipients.json').read_bytes()); originals={x['config']['fixture']:x for x in ready['recipients']}
    ca=canonical_ca((inputs/'published-ca.pem').read_text())[0]
    profile=setup['authority']; positives=[]; changed=copy.deepcopy(setup)
    by_gym={r['config']['identity']['gym_id']:r for r in originals.values()}
    for e,d in zip(changed['miners']['enrollments'],changed['settlement']['destinations']):
        e['payee_script']=d['script']=by_gym[e['gym_id']]['recipient']['script']
    changed['miners']['settlement_config_id']=H('SLUG operator pool settlement config',changed['settlement'])
    for n,fixture in enumerate(['zero256','incrementing256']):
        r=copy.deepcopy(originals[fixture])
        local=dict(version=1,manager_schema=14,wallet_owner_id=recipe['wallet_owner_ids'][n],allocation_binding_id=recipe['allocation_binding_ids'][n],original_source_binding_id=r['source_binding_id'],recipient=r)
        c=dict(version=1,connection_id=recipe['connection_ids'][n],operator_pin=copy.deepcopy(profile),source=copy.deepcopy(changed['pool']),settlement=copy.deepcopy(changed['settlement']),miners=copy.deepcopy(changed['miners']),origin=setup['ingress']['origin'],ca_certificate=ca,expected_local=local,max_records=recipe['max_records'],max_bytes=recipe['max_bytes'])
        assert raw(c['expected_local']['recipient'])==raw(originals[fixture])
        for suffix,records,size in [('default',64,67108864),('maximum',4096,1073741824)]:
            x=copy.deepcopy(c);x['max_records']=records;x['max_bytes']=size
            assert validate(x,profile,originals,ca)
            intent,rd=marker_bytes(x)
            positives.append(dict(name=fixture+'-'+suffix,config=x,canonical_hex=raw(x).hex(),config_id=config_id(x),intent_hex=intent.hex(),ready_hex=rd.hex(),worker=worker(x),miner_config_id=H('SLUG pool miner enrollment',x['miners'])))
    base=positives[0]['config']; negatives=[]
    def bad(name,path,value):
        x=copy.deepcopy(base);node=x
        for part in path[:-1]:node=node[part]
        node[path[-1]]=value
        assert not validate(x,profile,originals,ca),name
        negatives.append(dict(name=name,config=x))
    for name,path,value in [
      ('version',['version'],2),('zero-connection',['connection_id'],'00'*32),('legacy-profile',['operator_pin','version'],1),
      ('source-authority',['source','authority_pin','authority_id'],'99'*32),('profile-binding',['miners','operator_profile_id'],'99'*32),
      ('source-binding',['miners','source_id'],'99'*32),('settlement-binding',['miners','settlement_config_id'],'99'*32),
      ('payee',['miners','enrollments',1,'payee_script'],'0014'+'99'*20),('gym',['miners','enrollments',1,'gym_id'],'99'*32),
      ('local-version',['expected_local','version'],2),('schema15',['expected_local','manager_schema'],15),
      ('zero-owner',['expected_local','wallet_owner_id'],'00'*32),('zero-allocation',['expected_local','allocation_binding_id'],'00'*32),
      ('replace-original-source',['expected_local','original_source_binding_id'],base['miners']['source_id']),
      ('local-recipient-script',['expected_local','recipient','recipient','script'],'0014'+'99'*20),
      ('local-gym',['expected_local','recipient','config','identity','gym_public_key'],originals['incrementing256']['config']['identity']['gym_public_key']),
      ('http-upstream',['origin'],setup['ingress']['pool_url']),('origin-path',['origin'],base['origin']+'/slug/pool/v1'),
      ('origin-query',['origin'],base['origin']+'?x=1'),('origin-userinfo',['origin'],'https://user@pool.example.test:3443'),
      ('origin-upper',['origin'],'https://POOL.example.test:3443'),('origin-zero-port',['origin'],'https://pool.example.test:0'),
      ('empty-ca',['ca_certificate'],''),('noncanonical-ca',['ca_certificate'],ca+'\n'),
      ('records-zero',['max_records'],0),('records-high',['max_records'],4097),('bytes-low',['max_bytes'],67108863),('bytes-high',['max_bytes'],1073741825),
    ]: bad(name,path,value)
    for name,mut in [('untouched-operator-payees',None),('same-remote-payee-but-not-local',None),('different-local-policy',None)]:
        x=copy.deepcopy(base)
        if name=='untouched-operator-payees': x['settlement']=copy.deepcopy(setup['settlement']); x['miners']=copy.deepcopy(setup['miners'])
        elif name=='same-remote-payee-but-not-local':
            x['settlement']['destinations'][1]['script']=x['miners']['enrollments'][1]['payee_script']='0014'+'99'*20
            x['miners']['settlement_config_id']=H('SLUG operator pool settlement config',x['settlement'])
        else:
            r=x['expected_local']['recipient'];r['config']['source']['policy']['prospect_target']='7f'*32
            r['source_binding_id']=x['expected_local']['original_source_binding_id']=source_id(r['config']['source'])
        assert not validate(x,profile,originals,ca),name
        negatives.append(dict(name=name,config=x))
    collision=copy.deepcopy(base)
    original_setup=setups['positives'][0]['setup']
    collision['source']['payout_address']=original_setup['pool']['payout_address']
    collision['settlement']['change_script']=original_setup['settlement']['change_script']
    collision['miners']['source_id']=source_id(collision['source'])
    collision['miners']['settlement_config_id']=H('SLUG operator pool settlement config',collision['settlement'])
    assert collision['settlement']['change_script']==collision['expected_local']['recipient']['recipient']['script']
    assert not validate(collision,profile,originals,ca)
    negatives.append(dict(name='operator-change-equals-miner-payee',config=collision))
    b=raw(base)
    raw_cases=[('leading-space',b' '+b),('trailing-lf',b+b'\n'),('duplicate',b'{"version":1,'+b[1:]),('missing',b'{'+b[len(b'{"version":1,'):]),('unknown',b[:-1]+b',"unknown":0}'),('float',b.replace(b'"version":1',b'"version":1.0',1)),('bool',b.replace(b'"version":1',b'"version":true',1)),('null',b.replace(b'"version":1',b'"version":null',1)),('reverse-order',raw(dict(reversed(list(base.items()))))),('extra-object',b+b'{}')]
    creds=[]
    for p in (positives[0],positives[2]):
        for a in ('pool','admission'):
            c=credential(p['config'],a,recipe)
            creds.append(dict(name=p['name']+'-'+a,config_id=p['config_id'],audience=a,credential=c,canonical_hex=(raw(c)+b'\n').hex()))
    return dict(version=1,scope=recipe['scope'],positives=positives,refusals=negatives,raw_refusals=[dict(name=n,raw_hex=x.hex()) for n,x in raw_cases],credentials=creds,provenance=dict(operator_setup_sha256=sha((inputs/'operator-setup-v1.json').read_bytes()),recipients_sha256=sha((inputs/'original-recipients.json').read_bytes()),original_recipient_bytes_unchanged=True,old_operator_pairing_refused=True))
