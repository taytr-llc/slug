#!/usr/bin/env python3
"""Explicitly admitted independent setup corpus; no product or runtime IO."""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import sys

HERE = Path(__file__).resolve().parent
SOURCES = {'setup_model.py', 'generate_setup_reference.py', 'case-plan.json'}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def need(value, message):
    if not value:
        raise ValueError(message)


def safe(root, name):
    p = root / name
    need(not Path(name).is_absolute() and '..' not in Path(name).parts and p.is_file(), 'input path')
    need(not p.is_symlink() and p.resolve().is_relative_to(root.resolve()), 'input alias')
    return p


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def run(args):
    need(args.execute, 'explicit execute required')
    lock_path = Path(args.approved_producer_lock)
    raw = lock_path.read_bytes()
    need(sha(raw) == args.approved_lock_sha256, 'approval hash')
    lock = json.loads(raw)
    need(lock.get('version') == 1 and lock.get('approved_by') == 'root' and lock.get('execution_authorized') is True, 'root approval')
    need(set(lock['sources']) == SOURCES, 'closed source set')
    before = {name: sha(safe(HERE, name).read_bytes()) for name in SOURCES}
    need(before == lock['sources'], 'source hashes')
    input_root = Path(args.input_root).resolve()
    catalog_path = input_root.parent / 'input-catalog.proposal.json'
    need(sha(catalog_path.read_bytes()) == lock['input_catalog_sha256'], 'catalog hash')
    catalog = json.loads(catalog_path.read_bytes())
    need(catalog['version'] == 1 and 1 <= len(catalog['files']) <= 16, 'input catalog')
    def check_inputs():
        for name, expected in catalog['files'].items():
            data = safe(input_root, name).read_bytes()
            need(len(data) == expected['bytes'] and sha(data) == expected['sha256'], 'input differs')
    check_inputs()
    def read(name):
        need(name in catalog['files'], 'unlisted input')
        return json.loads(safe(input_root, name).read_bytes())
    # Import definitions only after every reviewed source/input byte matched.
    w = load('inherited_wallet_setup', safe(input_root, 'inherited/wallet_model.py'))
    c = load('inherited_core_setup', safe(input_root, 'inherited/core_model.py'))
    model = load('independent_restored_setup', safe(HERE, 'setup_model.py'))
    recipe = read('recipe.json')
    wallet = read('public/wallet-v1.json')
    core = read('public/core-v1.json')
    recovery = read('public/recovery-v1.json')
    point = next(x['value'] for x in core['positives'] if x['family'] == recipe['pin_case']['family'] and x['name'] == recipe['pin_case']['name'])
    contexts, positives, negatives, raw_cases = [], [], [], []
    base_bundles = []
    negative_baselines = {}

    def rejected(fn):
        try:
            fn()
        except (w.Refused, c.Refused, ValueError, KeyError, TypeError):
            return True
        return False

    def semantic(ctx_name, name, kind, value, m, initial, activation):
        need(w.canonical(value) != w.canonical(negative_baselines[ctx_name][kind]), 'negative mutation must differ: ' + name)
        call = lambda: m.init(value) if kind == 'init' else m.activate(value, initial) if kind == 'activate' else m.ready(value, initial, activation)
        need(rejected(call), 'supposed semantic negative accepted: ' + name)
        negatives.append(dict(name=ctx_name+'/'+name, context=ctx_name, kind=kind, value=value, accepted=False))

    for fi, fixture in enumerate(recipe['fixtures']):
        old = next(x for x in wallet['positives'] if x['name'] == recipe['original_case'].replace('<fixture>', fixture))
        phrase = next(x['export']['mnemonic'] for x in recovery['cases'] if x['name'] == fixture)
        for variant in ('base', 'maximum-cursors', 'file-whitespace', 'public-https'):
            original = copy.deepcopy(old['value']['original_manifest'])
            if variant == 'maximum-cursors':
                for account in original['accounts']:
                    account['next_external'] = account['next_internal'] = 4096
            selected = dict(version=1, mnemonic=phrase, manifest=original)
            file_bytes = (json.dumps(selected, indent=2, ensure_ascii=False)+'\n').encode() if variant == 'file-whitespace' else w.canonical(selected)+b'\n'
            need(len(file_bytes) <= 65536, 'modeled selected file cap')
            ids = {name: format(recipe['id_rule']['first_setup_integer']+fi*recipe['id_rule']['per_fixture_stride']+offset, '064x') for name, offset in recipe['id_rule']['offsets'].items()}
            public_read = w.fields(recipe['public_read'], 'url tls_spki_sha256')
            node = dict(url=recipe['node_binding_input']['url'], ca_certificate_sha256=sha(recipe['node_binding_input']['ca_certificate_ascii'].encode('ascii')))
            if variant == 'public-https':
                public_read = dict(url='https://recovery.example.invalid/slug/ownership/v1', tls_spki_sha256='ab'*32)
            pin = copy.deepcopy(old['context']['pin'])
            pin['wallet_owner_id'] = ids['wallet_owner_id']
            pin['ownership_pin_id'] = c.digest('SLUG fighter ownership pin', point).hex()
            pin['original_manifest_sha256'] = sha(w.canonical(original))
            identity = dict(journal_id=ids['carrier_journal_id'], pin=pin)
            initial = dict(version=1, mode='file-restored-gym', **ids, ownership=copy.deepcopy(point), ownership_pin_id=pin['ownership_pin_id'], public_read=public_read, node_binding=node, original_file_bytes=len(file_bytes), original_file_sha256=sha(file_bytes), original_manifest=original, original_manifest_sha256=pin['original_manifest_sha256'], carrier_identity=identity, carrier_binding_id=w.digest('fighter carrier binding', identity), restore_schema=12, active_schema=15)
            # Literal ID recipe order is not encoding authority; explicit field
            # selection below imposes the new contract's ordered top-level set.
            initial = w.fields(initial, model.INIT)
            guard = copy.deepcopy(old['value'])
            for key in ('wallet_owner_id', 'carrier_journal_id', 'ownership_pin_id', 'original_manifest_sha256', 'original_manifest'):
                guard[key] = copy.deepcopy(initial[key])
            guard['activation_tip_hash'] = original['genesis']
            guard['activation_tip_height'] = 0
            guard['activation_cursors'] = [n for account in original['accounts'] for n in (account['next_external'], account['next_internal'])]
            if variant == 'base':
                guard['activation_cursors'][0] = min(4096, guard['activation_cursors'][0]+1)
            expected = dict(initial, guard=guard)
            m = model.Model(w, c, expected)
            activation = dict(version=1, setup_id=ids['setup_id'], init_digest=m.digest('setup', initial), guard=guard, guard_digest=w.digest('restored carrier wallet guard', guard))
            ready = dict(version=1, setup_id=ids['setup_id'], init_digest=activation['init_digest'], activate_digest=m.digest('activation', activation), guard_digest=activation['guard_digest'], carrier_binding_id=initial['carrier_binding_id'], ownership_pin_id=initial['ownership_pin_id'], ownership_store_id=ids['ownership_store_id'])
            values = m.bundle(initial, activation, ready)
            ctx_name = fixture+'/'+variant
            contexts.append(dict(name=ctx_name, expected=expected, init=initial, activate=activation))
            for kind, value in zip(('init', 'activate', 'ready'), values):
                encoded = w.canonical(value)
                positives.append(dict(name=ctx_name+'/'+kind, context=ctx_name, kind=kind, value=value, canonical_hex=encoded.hex(), file_hex=(encoded+b'\n').hex(), file_bytes=len(encoded)+1, file_sha256=sha(encoded+b'\n'), digest=m.digest('setup' if kind=='init' else 'activation', value) if kind!='ready' else None))
                need(m.parse(encoded+b'\n', kind, initial, activation) == value, 'positive file roundtrip')
            if variant == 'base':
                base_bundles.append((ctx_name,m,initial,activation,ready))
                negative_baselines[ctx_name] = dict(init=initial, activate=activation, ready=ready)
            # Clear only mutable references to the public-fixture phrase input;
            # no claim of erasing Python immutable strings is made.
            selected.clear()
            del file_bytes

    for ctx_name,m,initial,activation,ready in base_bundles:
        mutations = [
            ('wrong-mode','mode','hosted'), ('schema13','active_schema',13),
            ('zero-setup','setup_id','00'*32), ('wrong-file-hash','original_file_sha256','11'*32),
            ('wrong-file-size','original_file_bytes',initial['original_file_bytes']+1),
            ('wrong-public-id','ownership_pin_id','11'*32), ('wrong-manifest-hash','original_manifest_sha256','11'*32),
        ]
        for name,key,value in mutations:
            v=copy.deepcopy(initial);v[key]=value;semantic(ctx_name,name,'init',v,m,initial,activation)
        v=copy.deepcopy(initial);del v['mode']
        semantic(ctx_name,'missing-field','init',v,m,initial,activation)
        v=copy.deepcopy(initial);v['extra']=0
        semantic(ctx_name,'extra-field','init',v,m,initial,activation)
        v=copy.deepcopy(initial);v['carrier_identity']=None
        semantic(ctx_name,'null-identity','init',v,m,initial,activation)
        v=copy.deepcopy(initial);v['public_read']['tls_spki_sha256']='11'*32
        semantic(ctx_name,'HTTP-trust-sentinel','init',v,m,initial,activation)
        v=copy.deepcopy(initial);v['original_manifest']['accounts'][0]['next_internal']=False
        semantic(ctx_name,'nested-boolean-cursor','init',v,m,initial,activation)
        v=copy.deepcopy(initial);v['node_binding']['url']='http://127.0.0.1:47303/'
        semantic(ctx_name,'changed-node-self-consistent','init',v,m,initial,activation)
        # Transport refusals retain the changed expected endpoint too, proving
        # profile validation rather than merely an original-pin mismatch.
        for label,url in (('node-HTTPS','https://127.0.0.1:47302/'),('node-remote','http://node.example.invalid/')):
            v=copy.deepcopy(initial);v['node_binding']['url']=url
            expected=copy.deepcopy(m.expected);expected['node_binding']=copy.deepcopy(v['node_binding'])
            refused_model=model.Model(w,c,expected)
            semantic(ctx_name,label,'init',v,refused_model,initial,activation)
            negatives[-1]['expected_override']=dict(node_binding=copy.deepcopy(v['node_binding']))
        v=copy.deepcopy(initial);v['carrier_identity']['pin']['account_id']='11'*32
        v['carrier_binding_id']=w.digest('fighter carrier binding',v['carrier_identity'])
        semantic(ctx_name,'restored-account-sentinel','init',v,m,initial,activation)
        v=copy.deepcopy(initial);v['ownership']['publication_id']='fe'*32
        need(v['ownership']['publication_id']!=initial['ownership']['publication_id'],'source mutant must differ')
        v['ownership_pin_id']=c.digest('SLUG fighter ownership pin',v['ownership']).hex()
        semantic(ctx_name,'changed-independent-source','init',v,m,initial,activation)
        for kind,base in (('activate',activation),('ready',ready)):
            for key in ('setup_id','init_digest','guard_digest'):
                v=copy.deepcopy(base);v[key]='11'*32
                semantic(ctx_name,kind+'-wrong-'+key,kind,v,m,initial,activation)
        for key in ('activate_digest','ownership_store_id'):
            v=copy.deepcopy(ready);v[key]='11'*32
            semantic(ctx_name,'ready-wrong-'+key,'ready',v,m,initial,activation)
        v=copy.deepcopy(activation);v['guard']['activation_cursors'][0]+=1
        v['guard_digest']=w.digest('restored carrier wallet guard',v['guard'])
        semantic(ctx_name,'different-valid-activation-snapshot','activate',v,m,initial,activation)
        v=copy.deepcopy(activation);v['guard']['activation_cursors'][4]=0
        v['guard_digest']=w.digest('restored carrier wallet guard',v['guard'])
        semantic(ctx_name,'activation-below-original','activate',v,m,initial,activation)
        for kind,base in (('init',initial),('activate',activation),('ready',ready)):
            encoded=w.canonical(base)+b'\n'
            cases={'missing-LF':encoded[:-1], 'extra-LF':encoded+b'\n', 'trailing-bytes':encoded+b'{}',
                   'duplicate':encoded.replace(b'{',b'{"version":1,',1),
                   'escaped-duplicate':encoded.replace(b'{',b'{"\\u0076ersion":1,',1),
                   'float':encoded.replace(b'"version":1',b'"version":1.0',1),
                   'capacity':b' '*model.CAPS[kind]+b'\n'}
            reordered=dict(reversed(list(base.items())))
            cases['reordered']=w.canonical(reordered)+b'\n'
            for label,raw in cases.items():
                need(raw != encoded, 'raw mutation must differ: ' + label)
                need(rejected(lambda:m.parse(raw,kind,initial,activation)), 'raw negative accepted')
                raw_cases.append(dict(name=ctx_name+'/'+kind+'/'+label,context=ctx_name,kind=kind,raw_hex=raw.hex(),accepted=False,scope='file-parser-only'))

    # Fixed nested public values + worst ASCII escaping for both 2048-byte
    # URLs, instead of measuring a convenient short local endpoint alone.
    fixed=[]
    for row in positives:
        if row['kind']=='init':
            exemplar=row['value']
            fixed.append(row['file_bytes']-len(w.canonical(exemplar['public_read']['url']))-len(w.canonical(exemplar['node_binding']['url']))-len(str(exemplar['original_file_bytes'])))
    sizing_init=max(fixed)+2*(2+2048*6)+len(str(65536))
    need(sizing_init<=32768, 'independent worst INIT bound')
    activate_measured=max(p['file_bytes'] for p in positives if p['kind']=='activate')
    activate_upper=activate_measured+len(str(2147483647))-len(str(0))
    ready_upper=max(p['file_bytes'] for p in positives if p['kind']=='ready')
    need(activate_upper<=16384 and ready_upper<=4096 and sizing_init+activate_upper+ready_upper<=65536,'independent complete marker bounds')
    sizing=dict(init_fixed_worst_url_upper=sizing_init,init_cap=32768,activate_max_measured=activate_measured,activate_worst_height_upper=activate_upper,activate_cap=16384,ready_fixed_upper=ready_upper,ready_cap=4096,individual_caps_sum=32768+16384+4096,aggregate_cap=65536,not_a_whole_process_memory_claim=True)
    check_inputs()
    need({name:sha(safe(HERE,name).read_bytes()) for name in SOURCES}==before,'source changed')
    result=dict(version=1,scope='independent public setup bytes; no actual filesystem/replay/capability proof',contexts=contexts,positives=positives,negatives=negatives,raw_cases=raw_cases,sizing=sizing,counts=dict(positives=len(positives),negatives=len(negatives),raw_cases=len(raw_cases)),input_catalog_sha256=lock['input_catalog_sha256'],producer_lock_sha256=args.approved_lock_sha256)
    need(result['counts']==json.loads(safe(HERE,'case-plan.json').read_bytes())['expected_counts'],'planned case count')
    output=json.dumps(result,indent=2,ensure_ascii=True,allow_nan=False).encode()+b'\n'
    with Path(args.output).open('xb') as f:
        f.write(output)
    print(json.dumps(dict(passed=True,sha256=sha(output),bytes=len(output),counts=result['counts'],python=sys.version)))


if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--execute',action='store_true');p.add_argument('--input-root',required=True);p.add_argument('--approved-producer-lock',required=True);p.add_argument('--approved-lock-sha256',required=True);p.add_argument('--output',required=True)
    run(p.parse_args())
