"""Independent authority-profile delta over archived public codec primitives.

Only imported by the locked producer. No Go, node, filesystem, network, keys or
normal-work verification. Full source/replay authority is outside this model.
"""
import copy
import hashlib


DOMAINS = {
    'admission': ('admission', 'SLUG fighter ownership admission statement', 'SLUG fighter ownership admission id'),
    'publication': ('publication', 'SLUG fighter ownership publication statement', 'SLUG fighter ownership publication id'),
    'boundary': ('cutoff', 'SLUG fighter ownership boundary statement', 'SLUG fighter ownership boundary id'),
    'cutoff': ('cutoff', 'SLUG fighter ownership cutoff statement', 'SLUG fighter ownership cutoff id'),
    'frame': ('publication', 'SLUG fighter ownership finality statement', 'SLUG fighter ownership finality id'),
}
ROLES = ('admission', 'publication', 'cutoff', 'checkpoint')


class Model:
    def __init__(self, base, crypto, recipe, sources):
        self.B, self.E, self.recipe = base, crypto, recipe
        self.sources = copy.deepcopy(sources)
        self.rule = hashlib.sha256(recipe['rule_ascii'].encode('ascii')).hexdigest()
        self.fixture_seeds = {}
        for row in recipe['known_fixture_seeds']:
            if row['kind'] == 'repeat_byte':
                base.need(row['byte'] == 115 and row['length'] == 32, 'fixture recipe')
                seed = bytes([row['byte']]) * row['length']
            else:
                base.need(row['kind'] == 'sha256_ascii', 'fixture recipe kind')
                seed = hashlib.sha256(row['ascii'].encode('ascii')).digest()
            self.fixture_seeds[row['name']] = seed
        self.excluded = {name: crypto.ed_public(seed).hex()
                         for name, seed in self.fixture_seeds.items()}
        base.need(len(self.excluded) == 5 and len(set(self.excluded.values())) == 5,
                  'five finite known fixture keys')
        self.seeds = {role: hashlib.sha256(text.encode('ascii')).digest()
                      for role, text in recipe['synthetic_role_seed_ascii'].items()}
        base.need(set(self.seeds) == set(ROLES), 'four explicit synthetic roles')
        self.keys = {role: crypto.ed_public(seed).hex() for role, seed in self.seeds.items()}
        base.need(len(set(self.keys.values())) == 4 and
                  not set(self.keys.values()).intersection(self.excluded.values()),
                  'synthetic keys distinct from known fixtures')

    def pin(self, source, label):
        B = self.B
        B.need(label in self.recipe['profile_labels'], 'profile label')
        if label == 'fixture_v1':
            keys = {r: self.excluded['ownership_log'] for r in ROLES}
            keys['checkpoint'] = self.excluded['seed_checkpoint']
            version = 1
        else:
            keys, version = copy.deepcopy(self.keys), 2
            if label == 'operator_v2_shared':
                keys['publication'] = keys['cutoff'] = keys['admission']
        p = dict(version=version, source=copy.deepcopy(source),
                 publication_id=('%02x' % (self.recipe['publication_id_byte'] +
                                           self.sources.index(source)))*32,
                 admission_key=keys['admission'], publication_key=keys['publication'],
                 cutoff_key=keys['cutoff'], checkpoint_key=keys['checkpoint'],
                 rule_id=self.rule, scan_origin=dict(height=0, hash=source['genesis']),
                 limits=copy.deepcopy(B.LIMITS))
        self.pin_id(p, version)
        return p

    def pin_id(self, value, selected_version):
        B = self.B
        p = B.ordered('pin', value)
        for name in ('source', 'scan_origin', 'limits'):
            p[name] = B.ordered(name, p[name])
        B.integer(selected_version, 1, 2, 'selected profile')
        B.integer(p['version'], selected_version, selected_version, 'pin version')
        s = p['source']
        B.integer(s['version'], 5, 5, 'manifest version')
        B.integer(s['network'], 0, 2**32-1, 'network')
        for field in ('genesis', 'authority_id', 'descriptor_id'):
            B.hex_value(s[field], nonzero=True)
        B.need(s in self.sources, 'exact inherited supported source')
        B.hex_value(p['publication_id'], nonzero=True)
        B.integer(p['scan_origin']['height'], 0, 0, 'scan origin height')
        B.need(p['scan_origin']['hash'] == s['genesis'], 'genesis scan origin')
        B.need(p['rule_id'] == self.rule, 'unchanged ownership rule')
        for field, number in B.LIMITS.items():
            B.integer(p['limits'][field], number, number, 'unchanged limit')
        keys = [p[r+'_key'] for r in ROLES]
        for key in keys:
            B.hex_value(key, nonzero=True)
        if selected_version == 1:
            B.need(keys[:3] == [self.excluded['ownership_log']]*3 and
                   keys[3] == self.excluded['seed_checkpoint'], 'closed fixture roles')
        else:
            B.need(not set(keys).intersection(self.excluded.values()), 'known fixture key')
        return B.digest('SLUG fighter ownership pin', p).hex()

    def parse_pin(self, raw, selected_version):
        B = self.B
        value = B.parse(raw, 'pin')
        p = B.ordered('pin', value)
        for name in ('source', 'scan_origin', 'limits'):
            p[name] = B.ordered(name, p[name])
        B.need(B.J(p) == raw, 'nested canonical pin')
        self.pin_id(p, selected_version)
        return p

    def seed_for(self, pin, role):
        key = pin[role+'_key']
        for seed in list(self.fixture_seeds.values()) + list(self.seeds.values()):
            if self.E.ed_public(seed).hex() == key:
                return seed
        raise self.B.Refused('no synthetic seed for selected public key')

    def sign(self, pin, family, body, role_override=None):
        role, domain, _ = DOMAINS[family]
        self.B.need('signature' not in body, 'unsigned body')
        out = copy.deepcopy(body)
        out['signature'] = self.E.ed_sign(self.seed_for(pin, role_override or role),
                                          self.B.digest(domain, body)).hex()
        return out

    def verify(self, pin, selected_version, family, value):
        B = self.B
        pid = self.pin_id(pin, selected_version)
        v = B.ordered(family, value)
        B.integer(v['version'], 1, 1, 'statement grammar version')
        B.need(v['pin_id'] == pid, 'statement pin')
        role, domain, id_domain = DOMAINS[family]
        sig = B.hex_value(v.pop('signature'), 64)
        B.need(self.E.ed_verify(bytes.fromhex(pin[role+'_key']), B.digest(domain, v), sig),
               'role signature')
        # This delta validator checks profile/role/PinID separation, not every
        # inherited replay or nested record invariant. The producer composes
        # exact authenticated inherited records and internally consistent refs.
        return B.digest(id_domain, value).hex()
