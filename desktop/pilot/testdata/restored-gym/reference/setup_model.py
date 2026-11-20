"""Independent restored setup byte model; definitions only, no IO on import.

Nested codecs are the earlier reviewed independent Python models. This model
does not import product Go, read a wallet or grant any concrete capability.
"""
import copy
import ipaddress
import json
import urllib.parse

INIT = ('version mode setup_id wallet_owner_id carrier_journal_id ownership_store_id '
        'ownership ownership_pin_id public_read node_binding original_file_bytes '
        'original_file_sha256 original_manifest original_manifest_sha256 '
        'carrier_identity carrier_binding_id restore_schema active_schema')
ACTIVATE = 'version setup_id init_digest guard guard_digest'
READY = 'version setup_id init_digest activate_digest guard_digest carrier_binding_id ownership_pin_id ownership_store_id'
CAPS = {'init': 32768, 'activate': 16384, 'ready': 4096}


class Model:
    def __init__(self, wallet, core, expected):
        self.w = wallet
        self.c = core
        self.expected = copy.deepcopy(expected)
        initial = copy.deepcopy(expected['original_manifest'])
        for account in initial['accounts']:
            account['next_external'] = account['next_internal'] = 0
        self.nested = wallet.Model(initial, None, None, None, None, None)

    def same(self, a, b, label):
        self.w.need(self.w.canonical(a) == self.w.canonical(b), label)

    def cap(self, value, kind):
        self.w.need(len(self.w.canonical(value)) + 1 <= CAPS[kind], 'capacity')
        return value

    def digest(self, kind, value):
        return self.w.digest('restored gym ' + kind, value)

    def pin(self, value):
        c, w = self.c, self.w
        p = c.ordered('pin', value)
        for kind in ('source', 'scan_origin', 'limits'):
            p[kind] = c.ordered(kind, p[kind])
        self.same(p, self.expected['ownership'], 'independent source pin')
        # Existing public digest includes its own LE32 byte-length framing.
        return p, c.digest('SLUG fighter ownership pin', p).hex()

    def endpoint(self, value, public):
        w = self.w
        v = w.fields(value, 'url tls_spki_sha256' if public else 'url ca_certificate_sha256')
        w.text(v['url'], 2048)
        u = urllib.parse.urlsplit(v['url'])
        w.need(u.scheme in ('http', 'https') and u.hostname and not u.username and not u.password,
               'endpoint authority')
        w.need(not u.query and not u.fragment and '?' not in v['url'] and '#' not in v['url'],
               'endpoint suffix')
        w.need(u.path == '/slug/ownership/v1' if public else u.path in ('', '/'), 'endpoint path')
        if public:
            if u.scheme == 'http':
                w.need(v['tls_spki_sha256'] == '', 'HTTP trust sentinel')
            else:
                w.hexv(v['tls_spki_sha256'], nonzero=True)
        else:
            w.need(u.scheme == 'http', 'node HTTP profile')
            w.hexv(v['ca_certificate_sha256'])
        if u.scheme == 'http':
            try:
                local = ipaddress.ip_address(u.hostname).is_loopback
            except ValueError:
                local = False
            w.need(local, 'literal loopback')
            if not public:
                w.need(v['ca_certificate_sha256'] == w.sha(b''), 'HTTP empty CA')
        self.same(v, self.expected['public_read' if public else 'node_binding'], 'independent transport')
        return v

    def init(self, value):
        w = self.w
        o = w.fields(value, INIT)
        w.integer(o['version'], 1, 1)
        w.need(o['mode'] == 'file-restored-gym', 'mode')
        for name in ('setup_id', 'wallet_owner_id', 'carrier_journal_id', 'ownership_store_id'):
            w.hexv(o[name], nonzero=True)
            w.need(o[name] == self.expected[name], 'original local ID')
        o['ownership'], pin_id = self.pin(o['ownership'])
        w.need(o['ownership_pin_id'] == pin_id, 'public pin ID')
        o['public_read'] = self.endpoint(o['public_read'], True)
        o['node_binding'] = self.endpoint(o['node_binding'], False)
        w.integer(o['original_file_bytes'], 1, 65536)
        w.hexv(o['original_file_sha256'])
        for name in ('original_file_bytes', 'original_file_sha256'):
            w.need(o[name] == self.expected[name], 'selected original file')
        o['original_manifest'] = w.manifest(o['original_manifest'], self.expected['original_manifest'])
        original_hash = w.sha(w.canonical(o['original_manifest']))
        w.need(o['original_manifest_sha256'] == original_hash, 'original manifest hash')
        identity = w.fields(o['carrier_identity'], 'journal_id pin')
        w.need(identity['journal_id'] == o['carrier_journal_id'], 'journal ID')
        identity['pin'] = self.nested.pin(identity['pin'], 'restored', o['original_manifest'])
        for field in ('wallet_owner_id', 'ownership_pin_id', 'original_manifest_sha256'):
            w.need(identity['pin'][field] == o[field], 'carrier pin binding')
        o['carrier_identity'] = identity
        w.need(o['carrier_binding_id'] == w.digest('fighter carrier binding', identity), 'carrier binding')
        w.integer(o['restore_schema'], 12, 12)
        w.integer(o['active_schema'], 15, 15)
        return self.cap(o, 'init')

    def activate(self, value, initial):
        w = self.w
        initial = self.init(initial)
        o = w.fields(value, ACTIVATE)
        w.integer(o['version'], 1, 1)
        w.need(o['setup_id'] == initial['setup_id'], 'setup ID')
        w.need(o['init_digest'] == self.digest('setup', initial), 'INIT digest')
        o['guard'] = self.nested.restored(o['guard'], initial['original_manifest'])
        for field in ('wallet_owner_id', 'carrier_journal_id', 'ownership_pin_id', 'original_manifest_sha256'):
            w.need(o['guard'][field] == initial[field], 'activation binding')
        self.same(o['guard'], self.expected['guard'], 'precommitted exact activation snapshot')
        w.need(o['guard_digest'] == w.digest('restored carrier wallet guard', o['guard']), 'guard digest')
        return self.cap(o, 'activate')

    def ready(self, value, initial, activation):
        w = self.w
        initial = self.init(initial)
        activation = self.activate(activation, initial)
        o = w.fields(value, READY)
        w.integer(o['version'], 1, 1)
        for name in ('setup_id', 'carrier_binding_id', 'ownership_pin_id', 'ownership_store_id'):
            w.need(o[name] == initial[name], 'READY binding')
        w.need(o['init_digest'] == self.digest('setup', initial), 'INIT digest')
        w.need(o['activate_digest'] == self.digest('activation', activation), 'ACTIVATE digest')
        w.need(o['guard_digest'] == activation['guard_digest'], 'guard digest')
        return self.cap(o, 'ready')

    def parse(self, raw, kind, initial=None, activation=None):
        w = self.w
        w.need(type(raw) is bytes and 0 < len(raw) <= CAPS[kind], 'capacity')
        w.need(raw.endswith(b'\n') and not raw.endswith(b'\n\n'), 'one LF')
        def pairs(rows):
            out = {}
            for k, v in rows:
                w.need(k not in out, 'duplicate member')
                out[k] = v
            return out
        def bad(_):
            raise w.Refused('noninteger JSON')
        try:
            value = json.loads(raw, object_pairs_hook=pairs, parse_float=bad, parse_constant=bad)
        except (UnicodeError, ValueError) as e:
            raise w.Refused('JSON') from e
        parsed = self.init(value) if kind == 'init' else self.activate(value, initial) if kind == 'activate' else self.ready(value, initial, activation)
        w.need(w.canonical(parsed) + b'\n' == raw, 'canonical file')
        return parsed

    def bundle(self, initial, activation, ready):
        values = (self.init(initial), self.activate(activation, initial), self.ready(ready, initial, activation))
        self.w.need(sum(len(self.w.canonical(v)) + 1 for v in values) <= 65536, 'aggregate capacity')
        return values
