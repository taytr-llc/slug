#!/usr/bin/env python3
"""Independent canonical/hash derivation from frozen public reference inputs.
No Go imports, wallet access, key derivation, signatures, network or wall clock.
"""
import argparse, copy, hashlib, json, pathlib, struct

def canonical(value):
    return json.dumps(value,separators=(',',':'),ensure_ascii=True).encode('ascii')

def h(raw):
    return hashlib.sha256(raw).hexdigest()

def framed(domain,value):
    raw=canonical(value)
    assert len(raw)<=65536
    return h(domain+struct.pack('<I',len(raw))+raw)

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--inputs',required=True);ap.add_argument('--out',required=True);args=ap.parse_args()
    directory=pathlib.Path(args.inputs);recipe=json.loads((directory/'recipe.json').read_bytes())
    raw=(directory/'operator-wallet-v1.json').read_bytes();assert h(raw)==recipe['inherited_sha256']
    old=json.loads(raw);assert old['version']==1
    source_domain=bytes.fromhex(recipe['source_domain_hex']);config_domain=bytes.fromhex(recipe['config_domain_hex']);guard_domain=bytes.fromhex(recipe['guard_domain_hex'])
    def source_id(source):return h(source_domain+canonical(source))
    def config_id(config):return framed(config_domain,config)
    def valid(binding):
        source,config,guard=binding['source'],binding['settlement'],binding['guard']
        return (binding['version']==2 and source['version']==2 and config['version']==2 and
                1<=source['max_works']<=8192 and 1<=source['max_submissions']<=16384 and 4096<=source['max_bytes']<=8589934592 and
                1<=config['max_rounds']<=16384 and 1<=config['max_batches']<=32767 and
                guard['version']==1 and guard['manager_schema']==16 and
                binding['wallet_owner_id']==guard['wallet_owner_id'] and binding['operator_profile_id']==guard['operator_profile_id'] and
                guard['source_binding_id']==source_id(source) and guard['settlement_config_id']==config_id(config) and
                config['change_script']==guard['funding_script'])
    positives=[];originals={row['fixture']:row for row in old['guards']}
    for fixture in recipe['fixtures']:
        prior=originals[fixture];source=copy.deepcopy(prior['source']);config=copy.deepcopy(prior['settlement']);guard=copy.deepcopy(prior['guard'])
        source['version']=2;source.update(recipe['source_limits']);config['version']=2;config.update(recipe['settlement_limits'])
        guard['source_binding_id']=source_id(source);guard['settlement_config_id']=config_id(config)
        binding={'version':2,'wallet_owner_id':guard['wallet_owner_id'],'operator_profile_id':guard['operator_profile_id'],'source':source,'settlement':config,'guard':guard}
        assert valid(binding)
        assert guard['source_binding_id']!=prior['guard']['source_binding_id'] and guard['settlement_config_id']!=prior['guard']['settlement_config_id']
        positives.append({'fixture':fixture,'binding':binding,'source_id':source_id(source),'config_id':config_id(config),'guard_hex':canonical(guard).hex(),'guard_digest':framed(guard_domain,guard),'binding_hex':canonical(binding).hex(),'binding_sha256':h(canonical(binding))})
    refusals=[];base=positives[0]['binding'];prior=originals[recipe['fixtures'][0]]
    for name in recipe['negative_cases']:
        b=copy.deepcopy(base)
        if name=='binding-v1':b['version']=1
        elif name=='source-v1':b['source']['version']=1
        elif name=='settlement-v1':b['settlement']['version']=1
        elif name=='source-over-limit':b['source']['max_submissions']=16385
        elif name=='rounds-over-limit':b['settlement']['max_rounds']=16385
        elif name=='batches-over-limit':b['settlement']['max_batches']=32768
        elif name=='zero-rounds':b['settlement']['max_rounds']=0
        elif name=='old-source-id':b['guard']['source_binding_id']=prior['guard']['source_binding_id']
        elif name=='old-config-id':b['guard']['settlement_config_id']=prior['guard']['settlement_config_id']
        elif name=='wrong-owner':b['wallet_owner_id']='73'*32
        elif name=='wrong-operator':b['operator_profile_id']='73'*32
        elif name=='wrong-funding-script':b['guard']['funding_script']='0014'+'00'*20
        else:raise AssertionError('unknown frozen case')
        assert not valid(b)
        refusals.append({'name':name,'binding':b,'accepted':False})
    result={'version':1,'scope':recipe['scope'],'positives':positives,'refusals':refusals,'shape_reference_sha256':h((directory/'native84-v1.json').read_bytes()),'prior_reference_sha256':h(raw)}
    output=json.dumps(result,indent=2,ensure_ascii=True).encode('ascii')+b'\n';assert len(output)<1<<20
    with open(args.out,'xb') as f:f.write(output)
if __name__=='__main__':main()
