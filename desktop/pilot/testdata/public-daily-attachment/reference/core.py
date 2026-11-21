"""Independent Set79 byte/arithmetic helpers. No product or old model imports."""
import base64
import copy
import hashlib
import json
import math
import re
import struct
from fractions import Fraction

MAX_TIME = 253402300799999
MAX_MONEY = 2100000000000000
MAX_I64 = (1 << 63) - 1
MAX_U64 = (1 << 64) - 1
REASONS = ('', 'payout-pending', 'broadcast-unknown', 'transaction-unavailable',
           'payout-conflict', 'funding-unavailable', 'funding-conflict',
           'funding-changed', 'immature', 'awaiting-confirmation', 'below-minimum',
           'destination-unavailable', 'daily-batch-complete', 'fee-unavailable',
           'fee-budget', 'change-dust', 'signing-unavailable', 'capacity',
           'transaction-capacity')

def require(value, code='invalid'):
    if not value:
        raise ValueError(code)

def clone(value):
    return copy.deepcopy(value)

def canon(value):
    return json.dumps(value, ensure_ascii=True, allow_nan=False,
                      separators=(',', ':')).encode('ascii')

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def frame(label, mode, body):
    require(len(body) <= 0xffffffff, 'size')
    return label.encode('ascii') + b'\0' + mode.encode('ascii') + b'\0' + struct.pack('<I', len(body)) + body

def ident(label, mode, value):
    return sha(frame(label, mode, canon(value)))

def fields(value, names):
    require(type(value) is dict and list(value) == names.split(','), 'fields')

def integer(value, lo, hi):
    require(type(value) is int and lo <= value <= hi, 'integer')

def hexstr(value, n, nonzero=False):
    require(type(value) is str and re.fullmatch('[0-9a-f]{%d}' % n, value) is not None
            and (not nonzero or value != '0' * n), 'hex')

def decimal(value, hi=MAX_U64):
    require(type(value) is str and re.fullmatch('0|[1-9][0-9]*', value) is not None
            and len(value) <= len(str(hi)), 'decimal')
    n = int(value)
    require(n <= hi, 'overflow')
    return n

def script(value):
    require(type(value) is str and re.fullmatch('0014[0-9a-f]{40}', value) is not None, 'script')

def fraction(value):
    fields(value, 'numerator,denominator')
    ns, ds = value['numerator'], value['denominator']
    require(type(ns) is str and type(ds) is str and 1 <= len(ns) <= 4096 and
            1 <= len(ds) <= 4096 and re.fullmatch('0|[1-9][0-9]*', ns) and
            re.fullmatch('[1-9][0-9]*', ds), 'fraction')
    n, d = int(ns), int(ds)
    require(math.gcd(n, d) == 1, 'fraction')
    return Fraction(n, d)

def frac(value):
    x = Fraction(value)
    return dict(numerator=str(x.numerator), denominator=str(x.denominator))

def parse(raw, cap, final_lf=False, optional_lf=False):
    require(type(raw) is bytes and 0 < len(raw) <= cap, 'size')
    if final_lf:
        require(raw.endswith(b'\n'), 'framing')
        raw = raw[:-1]
    elif optional_lf and raw.endswith(b'\n'):
        raw = raw[:-1]
    def pairs(rows):
        out = {}
        for k, v in rows:
            require(k not in out, 'duplicate')
            out[k] = v
        return out
    def bad(_):
        raise ValueError('number')
    value = json.loads(raw.decode('ascii'), object_pairs_hook=pairs,
                       parse_float=bad, parse_constant=bad)
    require(canon(value) == raw, 'canonical')
    return value

def accepted(fn, *args):
    try:
        fn(*args)
        return True
    except (ValueError, TypeError, KeyError, IndexError, OverflowError, UnicodeError):
        return False

def source_id(source):
    return sha(b'SLUG pool share daily source\0v2\0' + canon(source))

def settlement_id(config):
    return ident('SLUG operator pool settlement config', 'v2', config)

def profile_id(profile):
    # Version2 profile deliberately retains the original public-pin byte domain.
    return ident('SLUG fighter ownership pin', 'v1', profile)

def scheduler_id(config, source):
    return ident('SLUG pool settlement binding', 'v2', dict(config=config, source=source))

def gym_id(network, genesis, public_key):
    return sha(b'SLUG gym identity\0v1\0' + struct.pack('<I', network)
               + bytes.fromhex(genesis) + bytes.fromhex(public_key))

def manifest_trust(origin, pem):
    # Certificate validity is inherited from the frozen public CA fixture.
    ders = []
    if pem:
        parts = re.findall(r'-----BEGIN CERTIFICATE-----\n([A-Za-z0-9+/=\n]+)-----END CERTIFICATE-----\n', pem)
        require(parts, 'certificate')
        ders = sorted(set(hashlib.sha256(base64.b64decode(p)).digest() for p in parts))
    raw = origin.encode('ascii')
    mode = 1 if origin.startswith('http://') else (3 if pem else 2)
    pre = b'SLUG manifest transport trust\0v1\0' + struct.pack('<I', len(raw)) + raw + bytes([mode]) + struct.pack('<I', len(ders)) + b''.join(ders)
    return pre, sha(pre)

def daily_trust(client):
    _, admission = manifest_trust(client['url'], client['ca_certificate'])
    pre = (b'SLUG pool transport trust\0v2\0' + admission.encode('ascii')
           + source_id(client['source']).encode('ascii') + client['public_key'].encode('ascii')
           + client['settlement_config_id'].encode('ascii') + client['recipient_script'].encode('ascii'))
    return pre, sha(pre)

class Cases:
    """Explicit named facts, each verified by this independent predicate."""
    def __init__(self):
        self.rows = []
    def add(self, name, value, predicate, valid=True, scope='pure_bytes', **extra):
        require(name not in {x['name'] for x in self.rows}, 'duplicate case')
        require(accepted(predicate, value) == valid, 'case did not change predicate: ' + name)
        self.rows.append(dict(name=name, valid=valid, scope=scope, value=clone(value),
                             canonical_hex=canon(value).hex(), **extra))
    def bad(self, name, value, predicate, mutate, **extra):
        v = clone(value)
        mutate(v)
        self.add(name, v, predicate, False, **extra)
