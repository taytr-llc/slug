"""Independent public provisioning composition; never imports SLUG Go outputs.

Only inherited published synthetic inputs are consumed. The capsule byte recipe
is checked using the archived BIP39/BIP32 model; no real/private file is read.
"""
import copy
import hashlib
import ipaddress
import json
import re
import struct
from urllib.parse import urlsplit

POLICY_FIELDS = ('version', 'scope', 'request_id', 'network', 'genesis',
                 'descriptor_id', 'checkpoint_public_key', 'mining', 'limits',
                 'enrollments', 'schedule', 'fee', 'transport',
                 'maintenance_interval_ms', 'unlock_lifetime_ms', 'unlock_renewal_ms')
ROLES = ('runtime', 'authority', 'publication', 'ownership', 'wallet', 'scheduler', 'credentials')
ONLINE = ('admission', 'publication', 'cutoff')
MAX_POLICY = 65536
MAX_MONEY = 21000000 * 100000000


def need(ok, why):
    if not ok:
        raise ValueError(why)


def raw(value):
    return json.dumps(value, separators=(',', ':'), ensure_ascii=True).encode('ascii')


def H(label, body):
    return hashlib.sha256(label.encode('ascii') + b'\0v1\0' + struct.pack('<I', len(body)) + body).hexdigest()


def tagged(label, value):
    return H(label, raw(value))


def fields(value, names):
    need(type(value) is dict and tuple(value) == tuple(names), 'ordered exact fields')


def integer(value, lo, hi):
    need(type(value) is int and lo <= value <= hi, 'integer bound')


def hex_value(value, n=32, nonzero=True):
    need(type(value) is str and re.fullmatch('[0-9a-f]{%d}' % (2*n), value) is not None, 'canonical hex')
    need(not nonzero or int(value, 16) != 0, 'nonzero hex')
    return bytes.fromhex(value)


def identities(request_id):
    b = hex_value(request_id)
    return {role: H('SLUG pool provision identity', b + role.encode('ascii')) for role in ROLES}


def gym(network, genesis, public):
    return hashlib.sha256(b'SLUG gym identity\0v1\0' + struct.pack('<I', network) + bytes.fromhex(genesis) + bytes.fromhex(public)).hexdigest()


def host_ok(host):
    need(type(host) is str and 0 < len(host) <= 253 and host == host.lower(), 'host')
    try:
        parsed = ipaddress.ip_address(host)
    except ValueError:
        for label in host.split('.'):
            need(re.fullmatch('[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?', label) is not None, 'DNS label')
    else:
        need(str(parsed) == host, 'IP canonical')


def endpoint(value, scheme, loopback=False):
    need(type(value) is str and 0 < len(value) <= 512, 'URL bound')
    u = urlsplit(value)
    need(u.scheme == scheme and not u.path and not u.query and not u.fragment and u.username is None and u.password is None, 'URL components')
    host = u.hostname
    host_ok(host)
    authority = '[' + host + ']' if ':' in host else host
    if u.port is not None:
        integer(u.port, 1, 65535)
        authority += ':' + str(u.port)
    need(value == scheme + '://' + authority, 'URL canonical')
    if loopback:
        need(u.port is not None and ipaddress.ip_address(host).is_loopback, 'literal loopback URL')
    return host


def listen(value):
    need(type(value) is str and len(value) <= 128, 'listen bound')
    u = urlsplit('tcp://' + value)
    ip = ipaddress.ip_address(u.hostname)
    host = str(ip)
    authority = '[' + host + ']' if ip.version == 6 else host
    integer(u.port, 1, 65535)
    need(value == authority + ':' + str(u.port), 'literal nonzero listen')


class Model:
    def __init__(self, recipe, inherited, recovery, crypto):
        self.recipe, self.I, self.R, self.E = recipe, inherited, recovery, crypto
        self.setup = inherited['setup']['positives'][0]['setup']
        self.wallets = {r['fixture']: r for r in inherited['wallet']['guards']}
        self.recovery = {r['name']: r for r in inherited['recovery']['cases']}
        self.descriptors = set(recipe['supported_descriptor_ids'])
        self.online_keys = inherited['authority']['conditional_compositions']['v2/operator_v2_distinct']['pin']
        self.network = recipe['network']
        self.genesis = recipe['genesis']

    def policy(self, p):
        fields(p, POLICY_FIELDS)
        integer(p['version'], 1, 1)
        need(p['scope'] == 'phase-a-finite-operator', 'scope')
        hex_value(p['request_id'])
        integer(p['network'], self.network, self.network)
        need(p['genesis'] == self.genesis, 'current Phase-A genesis')
        need(p['descriptor_id'] in self.descriptors, 'supported candidate descriptor')
        need(p['checkpoint_public_key'] == self.recipe['checkpoint_public_key'], 'explicit compiled checkpoint public key')
        m = p['mining']; fields(m, ('dna_version', 'prospect_target', 'pool_target'))
        integer(m['dna_version'], 1, 1)
        hex_value(m['prospect_target']); hex_value(m['pool_target'])
        l = p['limits']; fields(l, ('max_works', 'max_submissions', 'pool_max_bytes', 'max_rounds', 'max_batches', 'round_max_bytes'))
        integer(l['max_works'], 1, 256); integer(l['max_submissions'], 1, 4096)
        integer(l['pool_max_bytes'], 4096, 1 << 30); integer(l['max_rounds'], l['max_works'], 256)
        integer(l['max_batches'], 1, 366); integer(l['round_max_bytes'], 4096, 1 << 30)
        need(type(p['enrollments']) is list and 1 <= len(p['enrollments']) <= 64, 'enrollment count')
        last = ''; keys = set(); scripts = set()
        for en in p['enrollments']:
            fields(en, ('public_key', 'payee_script'))
            public = hex_value(en['public_key'], 33)
            self.R.validate_public_key(public)
            script = hex_value(en['payee_script'], 22, False)
            need(script[:2] == b'\0\x14', 'native84 payee script')
            ident = gym(p['network'], p['genesis'], en['public_key'])
            need(ident > last and en['public_key'] not in keys and en['payee_script'] not in scripts, 'ordered unique enrollment')
            last = ident; keys.add(en['public_key']); scripts.add(en['payee_script'])
        s = p['schedule']; fields(s, ('daily_utc_minute', 'retry_ms'))
        integer(s['daily_utc_minute'], 0, 1439); integer(s['retry_ms'], 1000, 3600000)
        f = p['fee']; fields(f, ('mode', 'confirmation_target', 'atoms_per_kvb', 'max_atoms_per_kvb', 'max_fee_atoms'))
        integer(f['max_atoms_per_kvb'], 1, 1000000000); integer(f['max_fee_atoms'], 1, MAX_MONEY)
        if f['mode'] == 'fixed':
            integer(f['confirmation_target'], 0, 0); integer(f['atoms_per_kvb'], 1, f['max_atoms_per_kvb'])
        else:
            need(f['mode'] == 'node', 'explicit fee mode')
            integer(f['confirmation_target'], 1, 25); integer(f['atoms_per_kvb'], 0, 0)
        t = p['transport']; fields(t, ('listen', 'origin', 'pool_url', 'admission_url', 'ownership_url'))
        listen(t['listen']); endpoint(t['origin'], 'https')
        urls = [t[n] for n in ('pool_url', 'admission_url', 'ownership_url')]
        need(len(set(urls)) == 3, 'distinct private services')
        for url in urls: endpoint(url, 'http', True)
        integer(p['maintenance_interval_ms'], 1000, 60000)
        integer(p['unlock_lifetime_ms'], 60000, 86400000)
        integer(p['unlock_renewal_ms'], 1000, min(3600000, p['unlock_lifetime_ms']//2))
        need(len(raw(p)) <= MAX_POLICY, 'policy cap')
        return tagged('SLUG pool provision policy', p)

    def parse_policy(self, data, file_boundary=False):
        need(type(data) is bytes and 0 < len(data) <= MAX_POLICY + (1 if file_boundary else 0), 'policy raw cap')
        if file_boundary and data.endswith(b'\n'): data = data[:-1]
        def pairs(items):
            d = {}
            for k, v in items:
                need(k not in d, 'duplicate member'); d[k] = v
            return d
        p = json.loads(data, object_pairs_hook=pairs)
        self.policy(p)
        need(raw(p) == data, 'canonical policy')
        return p

    def material(self, fixture):
        row = self.wallets[fixture]; g = row['guard']; published = self.recovery[fixture]
        entropy = bytes.fromhex(published['entropy'])
        need(len(entropy) == 32, 'capsule entropy length')
        words = self.R.mnemonic_from_entropy(entropy)
        need(len(words.split(' ')) == 24, 'existing 24 word recipe')
        seed = self.R.mnemonic_to_seed(words, '')
        need(seed.hex() == published['seed'], 'inherited no-extra-pass BIP39 seed')
        root = self.R.ExtendedPrivate.master(seed)
        key = self.R.derive_path(seed, "m/84'/0'/0'/0/0")
        pub = self.R.public_key(key.scalar)
        need(root.serialize(True, True) == g['root_public_key'], 'independent master corroboration')
        need(pub.hex() == g['funding_public_key'] and '0014'+self.R.hash160(pub).hex() == g['funding_script'], 'independent fixed funding corroboration')
        address = self.R.witness_v0(self.R.hash160(pub))
        need(address == row['source']['payout_address'], 'inherited funding address')
        m = dict(version=1, created_at_ms=self.recipe['created_at_ms'], root_public_key=g['root_public_key'], funding_public_key=g['funding_public_key'])
        for role in ONLINE:
            seed_ascii = self.I['seed_recipe']['synthetic_role_seed_ascii'][role]
            derived = self.E.ed_public(hashlib.sha256(seed_ascii.encode('ascii')).digest()).hex()
            need(derived == self.online_keys[role+'_key'], 'inherited synthetic online key')
            m[role+'_key'] = derived
        return m

    def public_material(self, m):
        fields(m, ('version', 'created_at_ms', 'root_public_key', 'funding_public_key', 'admission_key', 'publication_key', 'cutoff_key'))
        integer(m['version'], 1, 1); integer(m['created_at_ms'], 1, 253402300799999)
        need(type(m['root_public_key']) is str and len(m['root_public_key']) <= 120, 'root bound')
        root = self.R.validate_extended(m['root_public_key'])
        need(int.from_bytes(root[:4], 'big') == self.R.SLUG_PUBLIC and root[4:13] == bytes(9), 'master public root')
        self.R.validate_public_key(hex_value(m['funding_public_key'], 33))
        excluded = set(self.I['authority']['literals']['known_fixture_keys'].values())
        for role in ONLINE:
            hex_value(m[role+'_key'])
            need(m[role+'_key'] not in excluded, 'known fixture signer exclusion')
        need(len(raw(m)) <= 2048, 'public material cap')

    def certificate(self, c):
        fields(c, ('version', 'certificate_sha256', 'certificate_spki'))
        integer(c['version'], 1, 1)
        hex_value(c['certificate_sha256']); hex_value(c['certificate_spki'])
        need(len(raw(c)) <= 1024, 'certificate projection cap')

    def compose(self, p, m, certificate):
        policy_id = self.policy(p)
        self.public_material(m)
        self.certificate(certificate)
        funding = bytes.fromhex(m['funding_public_key'])
        funding_script = '0014'+self.R.hash160(funding).hex()
        funding_address = self.R.witness_v0(self.R.hash160(funding))
        names = identities(p['request_id'])
        pin = copy.deepcopy(self.setup['authority'])
        pin['source'] = dict(version=5, network=p['network'], genesis=p['genesis'], authority_id=names['authority'], descriptor_id=p['descriptor_id'])
        pin['publication_id'] = names['publication']
        for role in ONLINE: pin[role+'_key'] = m[role+'_key']
        pin['checkpoint_key'] = p['checkpoint_public_key']
        pin['scan_origin'] = dict(height=0, hash=p['genesis'])
        pid = tagged('SLUG fighter ownership pin', pin)
        source = dict(version=1, authority_pin=copy.deepcopy(pin['source']), policy=dict(version=1, network=p['network'], genesis=p['genesis'], dna_version=p['mining']['dna_version'], prospect_target=p['mining']['prospect_target']), payout_address=funding_address, pool_target=p['mining']['pool_target'], max_works=p['limits']['max_works'], max_submissions=p['limits']['max_submissions'], max_bytes=p['limits']['pool_max_bytes'])
        source_id = hashlib.sha256(b'SLUG pool service source\0v1\0'+raw(source)).hexdigest()
        enrollments = [dict(public_key=en['public_key'], gym_id=gym(p['network'], p['genesis'], en['public_key']), payee_script=en['payee_script']) for en in p['enrollments']]
        need(funding_script not in {e['payee_script'] for e in enrollments}, 'payee/change collision')
        settlement = dict(version=1, change_script=funding_script, destinations=[dict(gym_id=en['gym_id'], script=en['payee_script']) for en in enrollments], max_rounds=p['limits']['max_rounds'], max_batches=p['limits']['max_batches'])
        cid = tagged('SLUG operator pool settlement config', settlement)
        guard = dict(version=1, manager_schema=16, wallet_owner_id=names['wallet'], operator_profile_id=pid, source_binding_id=source_id, settlement_config_id=cid, derivation_id='slug-phase-a-operator-pool-native84-v1', network=p['network'], genesis=p['genesis'], root_public_key=m['root_public_key'], funding_public_key=m['funding_public_key'], funding_script=funding_script)
        miners = dict(version=1, credential_id=names['credentials'], runtime_id=names['runtime'], operator_profile_id=pid, source_id=source_id, settlement_config_id=cid, enrollments=enrollments)
        schedule = dict(version=1, scheduler_id=names['scheduler'], runtime_id=names['runtime'], settlement_binding_id=tagged('SLUG pool settlement binding', dict(config=settlement, source=source)), daily_utc_minute=p['schedule']['daily_utc_minute'], retry_ms=p['schedule']['retry_ms'])
        ingress = dict(version=1, listen=p['transport']['listen'], origin=p['transport']['origin'], certificate_spki=certificate['certificate_spki'], pool_url=p['transport']['pool_url'], admission_url=p['transport']['admission_url'], ownership_url=p['transport']['ownership_url'])
        setup = dict(version=1, runtime_id=names['runtime'], authority=pin, ownership_store_id=names['ownership'], pool=source, round_limits=dict(version=1, max_rounds=p['limits']['max_rounds'], max_bytes=p['limits']['round_max_bytes']), settlement=settlement, schedule=schedule, miners=miners, wallet_guard=guard, node=dict(version=1, network=p['network'], genesis=p['genesis'], pool_public_key=m['funding_public_key'], pool_script=funding_script, fee=copy.deepcopy(p['fee'])), ingress=ingress, maintenance_interval_ms=p['maintenance_interval_ms'], unlock_lifetime_ms=p['unlock_lifetime_ms'], unlock_renewal_ms=p['unlock_renewal_ms'])
        need(len(raw(setup)) <= 262144, 'setup cap')
        created = m['created_at_ms']
        setup_id = tagged('SLUG pool operator setup', setup)
        guard_id = tagged('SLUG operator pool wallet guard', guard)
        intent = dict(version=1, policy=copy.deepcopy(p), policy_id=policy_id, certificate_sha256=certificate['certificate_sha256'], created_at_ms=created)
        ready = dict(version=1, request_id=p['request_id'], policy_id=policy_id, setup_id=setup_id, wallet_guard_id=guard_id, created_at_ms=created)
        return dict(policy_id=policy_id, namespace_ids=names, setup=setup, setup_id=setup_id, wallet_guard_id=guard_id, source_id=source_id, settlement_config_id=cid, operator_profile_id=pid, miner_config_id=tagged('SLUG pool miner enrollment', miners), settlement_binding_id=schedule['settlement_binding_id'], canonical_setup_hex=raw(setup).hex(), public_setup_file_hex=(raw(setup)+b'\n').hex(), intent_hex=(raw(intent)+b'\n').hex(), ready_hex=(raw(ready)+b'\n').hex())

    def backup(self, p, m, result, fixture):
        mnemonic = self.R.mnemonic_from_entropy(bytes.fromhex(self.recovery[fixture]['entropy']))
        return dict(version=1, kind='slug-operator-pool-wallet-backup', request_id=p['request_id'], policy_id=result['policy_id'], setup_id=result['setup_id'], created_at_ms=m['created_at_ms'], mnemonic_profile='bip39-english-256-empty-passphrase', mnemonic=mnemonic, derivation_id='slug-phase-a-operator-pool-native84-v1', funding_path="m/84'/0'/0'/0/0", network=p['network'], genesis=p['genesis'], wallet_guard=copy.deepcopy(result['setup']['wallet_guard']), wallet_guard_id=result['wallet_guard_id'])

    def validate_backup(self, b, p, m, certificate):
        fields(b, ('version', 'kind', 'request_id', 'policy_id', 'setup_id', 'created_at_ms', 'mnemonic_profile', 'mnemonic', 'derivation_id', 'funding_path', 'network', 'genesis', 'wallet_guard', 'wallet_guard_id'))
        integer(b['version'], 1, 1)
        need(b['kind'] == 'slug-operator-pool-wallet-backup' and b['mnemonic_profile'] == 'bip39-english-256-empty-passphrase', 'distinct operator backup')
        need(b['derivation_id'] == 'slug-phase-a-operator-pool-native84-v1' and b['funding_path'] == "m/84'/0'/0'/0/0", 'fixed backup path')
        phrase = b['mnemonic']
        need(type(phrase) is str and len(phrase) <= 256 and len(phrase.split(' ')) == 24, 'canonical 24 word phrase')
        entropy = self.R.entropy_from_mnemonic(phrase)
        need(len(entropy) == 32 and self.R.mnemonic_from_entropy(entropy) == phrase, 'canonical entropy')
        seed = self.R.mnemonic_to_seed(phrase, '')
        root = self.R.ExtendedPrivate.master(seed)
        public = self.R.public_key(self.R.derive_path(seed, "m/84'/0'/0'/0/0").scalar)
        need(root.serialize(True, True) == m['root_public_key'] and public.hex() == m['funding_public_key'], 'original backup key correspondence')
        expected = self.compose(p, m, certificate)
        for k in ('policy_id', 'setup_id', 'wallet_guard_id'):
            need(b[k] == expected[k], 'original backup identity')
        need(b['wallet_guard'] == expected['setup']['wallet_guard'] and tuple(b['wallet_guard']) == tuple(expected['setup']['wallet_guard']), 'exact original guard')
        need(b['request_id'] == p['request_id'] and b['network'] == p['network'] and b['genesis'] == p['genesis'] and b['created_at_ms'] == m['created_at_ms'], 'original backup metadata')
        need(len(raw(b))+1 <= 8192, 'backup cap')


def build_vectors(M, cert):
    base_setup = M.setup
    original = dict(version=1, scope='phase-a-finite-operator', request_id='a1'*32, network=M.network, genesis=M.genesis, descriptor_id=base_setup['authority']['source']['descriptor_id'], checkpoint_public_key=M.recipe['checkpoint_public_key'], mining=dict(dna_version=1, prospect_target=base_setup['pool']['policy']['prospect_target'], pool_target=base_setup['pool']['pool_target']), limits=dict(max_works=256, max_submissions=4096, pool_max_bytes=268435456, max_rounds=256, max_batches=366, round_max_bytes=268435456), enrollments=[dict(public_key=en['public_key'], payee_script=en['payee_script']) for en in base_setup['miners']['enrollments']], schedule=dict(daily_utc_minute=5, retry_ms=30000), fee=copy.deepcopy(base_setup['node']['fee']), transport=dict(listen='127.0.0.1:3443', origin='https://pool.example.test:3443', pool_url='http://127.0.0.1:3444', admission_url='http://127.0.0.1:3445', ownership_url='http://127.0.0.1:3446'), maintenance_interval_ms=30000, unlock_lifetime_ms=3600000, unlock_renewal_ms=300000)
    positives = []
    def positive(name, fixture, p, m):
        p = copy.deepcopy(p); m = copy.deepcopy(m)
        result = M.compose(p, m, cert['certificate'])
        b = M.backup(p, m, result, fixture)
        M.validate_backup(b, p, m, cert['certificate'])
        assert M.parse_policy(raw(p)) == p
        assert M.parse_policy(raw(p)+b'\n', True) == p
        positives.append(dict(name=name, fixture=fixture, policy=p, canonical_policy_hex=raw(p).hex(), policy_file_hex=(raw(p)+b'\n').hex(), public_material=m, certificate=copy.deepcopy(cert['certificate']), **result, backup=b, backup_file_hex=(raw(b)+b'\n').hex()))
    for fixture, req in zip(M.recipe['fixtures'], M.recipe['request_id_bytes']):
        p = copy.deepcopy(original); p['request_id'] = ('%02x' % req)*32
        positive(fixture, fixture, p, M.material(fixture))
    base = positives[0]; fixture = base['fixture']; material = base['public_material']
    p = copy.deepcopy(original); p['request_id'] = 'a4'*32
    positive('same-material-new-request', fixture, p, material)
    p = copy.deepcopy(original); p['fee'] = dict(mode='node', confirmation_target=6, atoms_per_kvb=0, max_atoms_per_kvb=100000, max_fee_atoms=1000000)
    positive('same-request-explicit-node-fee', fixture, p, material)
    m = copy.deepcopy(material); m['created_at_ms'] += 1
    positive('same-policy-later-material-time', fixture, original, m)
    assert positives[3]['namespace_ids'] != base['namespace_ids'] and positives[3]['setup_id'] != base['setup_id']
    assert positives[4]['namespace_ids'] == base['namespace_ids'] and positives[4]['policy_id'] != base['policy_id'] and positives[4]['setup_id'] != base['setup_id']
    assert positives[5]['setup_id'] == base['setup_id'] and positives[5]['intent_hex'] != base['intent_hex'] and positives[5]['backup_file_hex'] != base['backup_file_hex']
    refusals=[]
    def refused(name, family, value, fn):
        try: fn(value)
        except (ValueError, TypeError, KeyError, IndexError, OverflowError, UnicodeError): pass
        else: raise AssertionError('refusal admitted: '+name)
        refusals.append(dict(name=name, family=family, value=value))
    def change(name, path, value):
        p = copy.deepcopy(original); at=p
        for key in path[:-1]: at=at[key]
        at[path[-1]]=value
        refused(name,'policy',p,M.policy)
    for name,path,value in [
        ('version',['version'],2), ('scope',['scope'],'production'), ('zero-request',['request_id'],'00'*32), ('uppercase-request',['request_id'],'AB'*32),
        ('wrong-network',['network'],1), ('wrong-genesis',['genesis'],'12'*32), ('unknown-descriptor',['descriptor_id'],'12'*32), ('fixture-checkpoint',['checkpoint_public_key'],M.I['authority']['literals']['known_fixture_keys']['seed_checkpoint']),
        ('zero-prospect-target',['mining','prospect_target'],'00'*32), ('zero-pool-target',['mining','pool_target'],'00'*32), ('wrong-dna',['mining','dna_version'],2),
        ('zero-works',['limits','max_works'],0), ('excess-works',['limits','max_works'],257), ('rounds-below-works',['limits','max_rounds'],255), ('excess-submissions',['limits','max_submissions'],4097), ('excess-batches',['limits','max_batches'],367), ('pool-bytes-under',['limits','pool_max_bytes'],4095),
        ('empty-enrollments',['enrollments'],[]), ('unsorted-enrollments',['enrollments'],list(reversed(original['enrollments']))), ('duplicate-enrollment',['enrollments'],[original['enrollments'][0],original['enrollments'][0]]), ('invalid-point',['enrollments',0,'public_key'],'02'+'ff'*32), ('non-native-payee',['enrollments',0,'payee_script'],'5120'+'12'*32), ('duplicate-payee',['enrollments',1,'payee_script'],original['enrollments'][0]['payee_script']),
        ('daily-minute',['schedule','daily_utc_minute'],1440), ('retry-under',['schedule','retry_ms'],999), ('unknown-fee-mode',['fee','mode'],'automatic'), ('fixed-has-target',['fee','confirmation_target'],1), ('rate-over-cap',['fee','atoms_per_kvb'],100001), ('zero-max-fee',['fee','max_fee_atoms'],0),
        ('insecure-origin',['transport','origin'],'http://pool.example.test:3443'), ('origin-path',['transport','origin'],'https://pool.example.test:3443/'), ('origin-userinfo',['transport','origin'],'https://user@pool.example.test:3443'), ('public-upstream',['transport','pool_url'],'http://8.8.8.8:3444'), ('hostname-upstream',['transport','pool_url'],'http://localhost:3444'), ('repeated-upstream',['transport','pool_url'],original['transport']['admission_url']), ('zero-listen-port',['transport','listen'],'127.0.0.1:0'),
        ('maintenance-under',['maintenance_interval_ms'],999), ('unlock-over',['unlock_lifetime_ms'],86400001), ('renewal-after-half',['unlock_renewal_ms'],1800001)]:
        change(name,path,value)
    for name,field,value in [('material-version','version',2),('material-time-zero','created_at_ms',0),('material-time-over','created_at_ms',253402300800000),('material-root','root_public_key','not-a-master'),('material-funding-point','funding_public_key','02'+'ff'*32),('material-zero-online','admission_key','00'*32),('material-fixture-online','publication_key',M.I['authority']['literals']['known_fixture_keys']['ownership_log'])]:
        m=copy.deepcopy(material);m[field]=value;refused(name,'public_material',m,M.public_material)
    for name,field,value in [('certificate-version','version',2),('certificate-zero-file','certificate_sha256','00'*32),('certificate-upper-spki','certificate_spki','AB'*32)]:
        c=copy.deepcopy(cert['certificate']);c[field]=value;refused(name,'certificate',c,M.certificate)
    p=copy.deepcopy(original);p['enrollments'][0]['payee_script']=base['setup']['wallet_guard']['funding_script']
    refused('payee-funding-collision','composition',p,lambda v:M.compose(v,material,cert['certificate']))
    for name,path,value in [('backup-kind',['kind'],'slug-player-recovery'),('backup-request',['request_id'],'b1'*32),('backup-policy',['policy_id'],'b1'*32),('backup-setup',['setup_id'],'b1'*32),('backup-time',['created_at_ms'],material['created_at_ms']+1),('backup-profile',['mnemonic_profile'],'bip39-with-passphrase'),('backup-path',['funding_path'],"m/84'/0'/0'/0/1"),('backup-network',['network'],1),('backup-funding',['wallet_guard','funding_public_key'],positives[1]['public_material']['funding_public_key']),('backup-other-valid-phrase',['mnemonic'],positives[1]['backup']['mnemonic']),('backup-phrase-checksum',['mnemonic'],' '.join(['abandon']*24))]:
        b=copy.deepcopy(base['backup']);at=b
        for key in path[:-1]:at=at[key]
        at[path[-1]]=value
        refused(name,'backup',b,lambda v:M.validate_backup(v,original,material,cert['certificate']))
    encoded=raw(original)
    raw_cases=[]
    def raw_case(name,b,accept):
        try: M.parse_policy(b,True); actual=True
        except (ValueError,TypeError,KeyError,IndexError,OverflowError,UnicodeError):actual=False
        assert actual == accept, name
        raw_cases.append(dict(name=name,raw_hex=b.hex(),accepted=accept))
    raw_case('canonical',encoded,True);raw_case('one-terminal-lf',encoded+b'\n',True)
    for name,b in [('leading-space',b' '+encoded),('trailing-space',encoded+b' '),('two-lf',encoded+b'\n\n'),('crlf',encoded+b'\r\n'),('duplicate',b'{"version":1,'+encoded[1:]),('missing',b'{'+encoded[len(b'{"version":1,'):]),('unknown',encoded[:-1]+b',"unexpected":0}'),('float',encoded.replace(b'"version":1,',b'"version":1.0,',1)),('boolean',encoded.replace(b'"version":1,',b'"version":true,',1)),('null',encoded.replace(b'"version":1,',b'"version":null,',1)),('negative-zero',encoded.replace(b'"version":1,',b'"version":-0,',1)),('escaped-alias',encoded.replace(b'"version"',b'"\\u0076ersion"',1)),('trailing-object',encoded+b'{}'),('nul',encoded+b'\0'),('truncated',encoded[:-1]),('over-cap',b' '*65538)]:
        raw_case(name,b,False)
    counts=dict(positives=len(positives),refusals=len(refusals),raw_cases=len(raw_cases))
    assert counts == dict(positives=6,refusals=61,raw_cases=18), counts
    return dict(version=1,scope='Published synthetic policy/namespace/Setup/operator-backup byte composition only; no private creation, runtime, financial or deployment authority',certificate_fixture=cert,positives=positives,refusals=refusals,raw_cases=raw_cases,counts=counts)
