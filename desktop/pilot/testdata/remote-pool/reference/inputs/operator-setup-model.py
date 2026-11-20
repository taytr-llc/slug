"""Independent setup-byte composition from frozen public fixtures, no Go."""
from pathlib import Path
import copy, hashlib, json, struct, sys

def raw(v):
    return json.dumps(v, separators=(',', ':'), ensure_ascii=True).encode()

def H(label, value):
    body = raw(value)
    return hashlib.sha256(label.encode()+b'\0v1\0'+struct.pack('<I', len(body))+body).hexdigest()

def gym(network, genesis, public):
    return hashlib.sha256(b'SLUG gym identity\0v1\0'+struct.pack('<I',network)+bytes.fromhex(genesis)+bytes.fromhex(public)).hexdigest()

def build(inputs):
    authority = json.loads((inputs/'authority.json').read_bytes())
    wallets = json.loads((inputs/'wallet.json').read_bytes())
    ready = json.loads((inputs/'recipients.json').read_bytes())
    keys = {x['config']['identity']['gym_id']:x['config']['identity']['gym_public_key'] for x in ready['recipients']}
    positives=[]
    for n, row in enumerate(wallets['guards']):
        # This is new test composition, not a rewrite of the inherited corpus.
        p=copy.deepcopy(authority['conditional_compositions']['v2/operator_v2_distinct']['pin'])
        source=copy.deepcopy(row['source']);source['authority_pin']=copy.deepcopy(p['source'])
        settlement=copy.deepcopy(row['settlement'])
        pid=H('SLUG fighter ownership pin',p)
        sid=hashlib.sha256(b'SLUG pool service source\0v1\0'+raw(source)).hexdigest()
        cid=H('SLUG operator pool settlement config',settlement)
        guard=copy.deepcopy(row['guard']);guard.update(operator_profile_id=pid,source_binding_id=sid,settlement_config_id=cid)
        runtime=('81','82','83')[n]*32
        enrollments=[]
        for d in settlement['destinations']:
            k=keys[d['gym_id']]
            assert gym(source['policy']['network'],source['policy']['genesis'],k)==d['gym_id']
            enrollments.append(dict(public_key=k,gym_id=d['gym_id'],payee_script=d['script']))
        miners=dict(version=1,credential_id='84'*32,runtime_id=runtime,operator_profile_id=pid,source_id=sid,settlement_config_id=cid,enrollments=enrollments)
        schedule=dict(version=1,scheduler_id='85'*32,runtime_id=runtime,settlement_binding_id=H('SLUG pool settlement binding',dict(config=settlement,source=source)),daily_utc_minute=(0,5,1439)[n],retry_ms=30000)
        node=dict(version=1,network=guard['network'],genesis=guard['genesis'],pool_public_key=guard['funding_public_key'],pool_script=guard['funding_script'],fee=dict(mode='fixed',confirmation_target=0,atoms_per_kvb=1000,max_atoms_per_kvb=100000,max_fee_atoms=1000000))
        ingress=dict(version=1,listen='127.0.0.1:3443',origin='https://pool.example.test:3443',certificate_spki='86'*32,pool_url='http://127.0.0.1:3444',admission_url='http://127.0.0.1:3445',ownership_url='http://127.0.0.1:3446')
        setup=dict(version=1,runtime_id=runtime,authority=p,ownership_store_id='87'*32,pool=source,round_limits=dict(version=1,max_rounds=256,max_bytes=268435456),settlement=settlement,schedule=schedule,miners=miners,wallet_guard=guard,node=node,ingress=ingress,maintenance_interval_ms=30000,unlock_lifetime_ms=3600000,unlock_renewal_ms=300000)
        setup_id=H('SLUG pool operator setup',setup)
        ready=dict(version=1,runtime_id=runtime,setup_id=setup_id)
        positives.append(dict(name=row['fixture'],setup=setup,canonical_hex=raw(setup).hex(),setup_id=setup_id,intent_hex=(raw(setup)+b'\n').hex(),ready_hex=(raw(ready)+b'\n').hex(),miner_config_id=H('SLUG pool miner enrollment',miners)))
    base=positives[0]['setup']; negatives=[]
    def bad(name,path,value):
        x=copy.deepcopy(base);node=x
        for part in path[:-1]:node=node[part]
        node[path[-1]]=value
        negatives.append(dict(name=name,setup=x))
    bad('version',['version'],2)
    bad('runtime-zero',['runtime_id'],'00'*32)
    bad('ownership-zero',['ownership_store_id'],'00'*32)
    bad('legacy-authority',['authority','version'],1)
    bad('different-source',['pool','authority_pin','authority_id'],'88'*32)
    bad('different-wallet-profile',['wallet_guard','operator_profile_id'],'88'*32)
    bad('different-wallet-source',['wallet_guard','source_binding_id'],'88'*32)
    bad('different-wallet-settlement',['wallet_guard','settlement_config_id'],'88'*32)
    bad('different-node-script',['node','pool_script'],'0014'+'88'*20)
    bad('different-node-genesis',['node','genesis'],'88'*32)
    bad('different-schedule-runtime',['schedule','runtime_id'],'88'*32)
    bad('different-schedule-binding',['schedule','settlement_binding_id'],'88'*32)
    bad('schedule-minute',['schedule','daily_utc_minute'],1440)
    bad('different-miner-runtime',['miners','runtime_id'],'88'*32)
    bad('different-payee',['miners','enrollments',0,'payee_script'],'0014'+'88'*20)
    bad('rounds-below-jobs',['round_limits','max_rounds'],255)
    bad('maintenance-fast',['maintenance_interval_ms'],999)
    bad('unlock-too-long',['unlock_lifetime_ms'],86400001)
    bad('renewal-too-late',['unlock_renewal_ms'],1800001)
    bad('same-upstream',['ingress','admission_url'],base['ingress']['pool_url'])
    bad('ephemeral-public-port',['ingress','listen'],'127.0.0.1:0')
    encoded=raw(base)
    raw_bad=[dict(name='leading-space',raw_hex=(b' '+encoded).hex()),dict(name='trailing-lf',raw_hex=(encoded+b'\n').hex()),dict(name='duplicate-version',raw_hex=(b'{"version":1,'+encoded[1:]).hex()),dict(name='missing-version',raw_hex=(b'{'+encoded[len(b'{"version":1,'):]).hex()),dict(name='unknown-member',raw_hex=(encoded[:-1]+b',"extra":0}').hex())]
    return dict(version=1,scope='synthetic public setup byte composition only',positives=positives,refusals=negatives,raw_refusals=raw_bad)

if __name__=='__main__':
    print(json.dumps(build(Path(sys.argv[1])),indent=2))
