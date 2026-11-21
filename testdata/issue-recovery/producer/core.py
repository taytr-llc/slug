"""Independent Set80 scalar/byte grammar. No product or inherited-model imports."""
import copy
import hashlib
import json
import re
import struct

MAX_TIME = 253402300799999
MAX_U64 = (1 << 64) - 1
MAX_TEMPLATE = 16 << 20
MAX_JOB = 8192
MAX_ROW = 4096
MAX_WORK_RESPONSE = MAX_TEMPLATE + (64 << 10)


def need(value, code='invalid'):
    if not value:
        raise ValueError(code)


def clone(value):
    return copy.deepcopy(value)


def canon(value):
    # Go's encoding/json emits UTF-8, escapes HTML punctuation and U+2028/2029.
    text = json.dumps(value, ensure_ascii=False, allow_nan=False, separators=(',', ':'))
    for char, escaped in (('&', '\\u0026'), ('<', '\\u003c'), ('>', '\\u003e'),
                          ('\u2028', '\\u2028'), ('\u2029', '\\u2029')):
        text = text.replace(char, escaped)
    return text.encode('utf-8')


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def frame(label, raw, version='v1'):
    need(len(raw) <= 0xffffffff, 'length-overflow')
    return label.encode('ascii') + b'\0' + version.encode('ascii') + b'\0' + struct.pack('<I', len(raw)) + raw


def ident(label, value, version='v1'):
    return sha(frame(label, canon(value), version))


def fields(value, names):
    need(type(value) is dict and list(value) == names.split(','), 'fields')


def integer(value, low, high):
    need(type(value) is int and low <= value <= high, 'integer')


def hexid(value, size=64, nonzero=True):
    need(type(value) is str and re.fullmatch('[0-9a-f]{%d}' % size, value) is not None
         and (not nonzero or value != '0' * size), 'hex-id')


def source_id(source):
    return sha(b'SLUG pool share daily source\0v2\0' + canon(source))


def gym_id(context, public_key):
    return sha(b'SLUG gym identity\0v1\0' + struct.pack('<I', context['network'])
               + bytes.fromhex(context['genesis']) + bytes.fromhex(public_key))


def public_key(value):
    hexid(value, 66)
    need(value[:2] in ('02', '03'), 'public-key')
    x = int(value[2:], 16)
    p = (1 << 256) - (1 << 32) - 977
    y2 = (x * x * x + 7) % p
    need(x < p and pow(pow(y2, (p + 1) // 4, p), 2, p) == y2, 'public-key')


def decode(raw, cap, framing='value'):
    need(type(raw) is bytes and 0 < len(raw) <= cap, 'size-before-framing')
    if framing == 'http':
        raw = raw.strip(b' \t\r\n')
    elif framing == 'lf':
        need(raw.endswith(b'\n'), 'final-lf')
        raw = raw[:-1]
    else:
        need(framing == 'value', 'framing-mode')

    def pairs(items):
        result = {}
        for key, value in items:
            need(key not in result, 'duplicate-field')
            result[key] = value
        return result

    def bad_number(_):
        raise ValueError('noninteger-number')

    value = json.loads(raw.decode('utf-8'), object_pairs_hook=pairs,
                       parse_float=bad_number, parse_constant=bad_number)
    need(canon(value) == raw, 'canonical-bytes')
    return value


def classify(validator, *args):
    try:
        validator(*args)
        return True, ''
    except (ValueError, TypeError, KeyError, IndexError, OverflowError, UnicodeError) as exc:
        return False, str(exc) if isinstance(exc, ValueError) else type(exc).__name__


class Cases:
    def __init__(self, scope):
        self.rows = []
        self.scope = scope

    def add(self, name, value, validator, *args, valid=True):
        okay, reason = classify(validator, value, *args)
        need(okay is valid, 'independent-case-classification:' + name)
        row = dict(name=name, scope=self.scope, valid=valid, value=clone(value),
                   canonical_hex=canon(value).hex(), refusal=reason)
        self.rows.append(row)
        return row

    def bad(self, name, original, mutate, validator, *args):
        value = clone(original)
        mutate(value)
        need(value != original or canon(value) != canon(original), 'no-op-mutation:' + name)
        return self.add(name, value, validator, *args, valid=False)


PROFILE_FIELDS = 'version,base_setup_id,source_config_id,max_requests'


def validate_profile(profile, source, expected_setup_id=None):
    fields(profile, PROFILE_FIELDS)
    integer(profile['version'], 1, 1)
    hexid(profile['base_setup_id'])
    hexid(profile['source_config_id'])
    integer(profile['max_requests'], 1, 8192)
    need(source['version'] == 2 and profile['max_requests'] == source['max_works'], 'source-count')
    need(profile['source_config_id'] == source_id(source), 'source-config')
    if expected_setup_id is not None:
        need(profile['base_setup_id'] == expected_setup_id, 'setup-selection')
    need(len(canon(profile)) <= 202, 'profile-cap')


def make_profile(source, setup_id):
    profile = dict(version=1, base_setup_id=setup_id, source_config_id=source_id(source), max_requests=source['max_works'])
    validate_profile(profile, source, setup_id)
    return profile


def profile_id(profile):
    return ident('SLUG pool issue recovery profile', profile)


def request_key(kind, value):
    need(kind in ('r', 't', 'j'), 'key-kind')
    hexid(value)
    raw = ('issue/' + kind + '/').encode('ascii') + bytes.fromhex(value)
    need(len(raw) == 40, 'key-width')
    return raw


def manifest_preimage(job):
    c = job['context']
    out = b'SLUG mining job\0v2\0'
    out += struct.pack('<II', job['version'], job['rule_version'])
    out += bytes.fromhex(job['authority_id']) + bytes.fromhex(job['descriptor_id'])
    out += struct.pack('<II', c['version'], c['network']) + bytes.fromhex(c['genesis'])
    out += struct.pack('<I', c['dna_version']) + bytes.fromhex(c['prospect_target']) + bytes.fromhex(c['parent_hash'])
    out += struct.pack('<IIII', c['parent_height'], c['expected_bits'], c['min_timestamp'], c['max_timestamp'])
    out += bytes.fromhex(job['public_key']) + bytes.fromhex(job['gym_id']) + bytes.fromhex(job['challenge'])
    out += struct.pack('<QQ', job['issued_at_ms'], job['expires_at_ms'])
    return out


def derive_job(context, pin, key, challenge, issued):
    public_key(key)
    hexid(challenge)
    integer(issued, 1, MAX_TIME - 300000)
    need(all(context[k] == pin[k] for k in ('network', 'genesis')), 'job-chain')
    job = dict(version=2, rule_version=1, id='', authority_id=pin['authority_id'], descriptor_id=pin['descriptor_id'],
               context=clone(context), public_key=key, gym_id=gym_id(context, key), challenge=challenge,
               issued_at_ms=issued, expires_at_ms=issued + 300000)
    job['id'] = sha(manifest_preimage(job))
    need(len(canon(job)) <= MAX_JOB, 'job-cap')
    return job
