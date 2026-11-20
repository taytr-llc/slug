"""Independent Set73 public bytes/state model. Never imported before root's lock.

The imported Ed25519 implementation is archived Set45 independent source.
Normal-work and canonical-chain truth are NOT manufactured by this model.
"""
import copy
import hashlib
import json
import re
import struct

ZERO = '00' * 32
MAX_TIME = 253402300799999
LIMITS = dict(admissions=1000, publications=16384, carriers=4096,
              checkpoints=1024, cutoffs=256, headers=65536,
              logical_bytes=268435456)
FIELDS = {
    'pin': 'version source publication_id admission_key publication_key cutoff_key checkpoint_key rule_id scan_origin limits',
    'admission': 'version pin_id admission_sequence previous_admission_id record_digest record signature',
    'publication': 'version pin_id sequence previous_id attested_at_ms kind payload signature',
    'initial': 'version pin_id prospect_id admission_id owner_public_key',
    'final': 'version pin_id prospect_id prior_state_id transfer_id checkpoint_publication_id sixtieth_descendant_height sixtieth_descendant_hash owner_public_key',
    'state': 'version pin_id prospect_id state_id owner_public_key birth_admission_id generation evidence',
    'map': 'version pin_id items',
    'frame': 'version pin_id sequence previous_body_id admission_sequence admission_head publication_sequence publication_head chain_height chain_hash ownership_map_id finalized_prefix_id signature',
    'observation': 'request_start_ms response_end_ms instance_id revision height hash',
    'intent': 'version pin_id event_id cutoff_ms first second accepted_admission_sequence publication_sequence publication_head finality_sequence finality_body_head prior_cutoff_id',
    'boundary': 'version pin_id intent signature',
    'cutoff': 'version pin_id event_id cutoff_ms boundary admission_sequence admission_head head ownership_digest signature',
    'head': 'version pin_id admission_sequence admission_head publication_sequence publication_head chain_height chain_hash replay_digest finalized_prefix_id conflict_id',
    'source': 'version network genesis authority_id descriptor_id',
    'scan_origin': 'height hash',
    'limits': 'admissions publications carriers checkpoints cutoffs headers logical_bytes',
}


class Refused(ValueError):
    pass


def need(value, message):
    if not value:
        raise Refused(message)


def J(value):
    return json.dumps(value, ensure_ascii=True, separators=(',', ':'),
                      allow_nan=False).encode('ascii')


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def digest(domain, value):
    raw = J(value)
    return hashlib.sha256(domain.encode('ascii') + b'\0v1\0' +
                          struct.pack('<I', len(raw)) + raw).digest()


def ordered(kind, value):
    names = FIELDS[kind].split()
    need(type(value) is dict and set(value) == set(names), 'closed ' + kind)
    return {name: copy.deepcopy(value[name]) for name in names}


def integer(value, low, high, name):
    need(type(value) is int and low <= value <= high, name)
    return value


def hex_value(value, size=32, nonzero=False):
    need(type(value) is str and re.fullmatch('[0-9a-f]{%d}' % (size * 2), value), 'lowercase hex')
    need(not nonzero or value != '00' * size, 'zero identity')
    return bytes.fromhex(value)


def pairs(items):
    result = {}
    for k, v in items:
        need(k not in result, 'duplicate member')
        result[k] = v
    return result


def parse(raw, kind, cap=8192):
    need(type(raw) is bytes and 0 < len(raw) <= cap, 'raw cap')
    def bad(_):
        raise Refused('non-integer JSON')
    try:
        value = json.loads(raw, object_pairs_hook=pairs, parse_float=bad,
                           parse_constant=bad)
    except (ValueError, UnicodeError) as e:
        raise Refused('JSON') from e
    value = ordered(kind, value)
    need(J(value) == raw, 'canonical JSON')
    return value


def key(prefix, value, sequence=False):
    tail = struct.pack('>I', integer(value, 0, 2**32-1, 'key sequence')) if sequence else hex_value(value)
    return prefix.encode('ascii') + tail


class Model:
    def __init__(self, crypto, recipe):
        self.crypto = crypto
        self.recipe = recipe
        self.seed = bytes([recipe['public_fixture_seed_byte']]) * recipe['public_fixture_seed_bytes']
        need(self.seed == b'\x73' * 32, 'fixed published seed')
        self.public = crypto.ed_public(self.seed).hex()
        self.rule_id = sha(recipe['rule_ascii'].encode('ascii'))
        self.checkpoint_seed = hashlib.sha256(recipe['checkpoint_seed_ascii'].encode('ascii')).digest()
        self.checkpoint_public = crypto.ed_public(self.checkpoint_seed).hex()
        need(self.public != self.checkpoint_public, 'role separation')

    def pin(self, source, index):
        return dict(version=1, source=copy.deepcopy(source),
                    publication_id=bytes([17 + index]).hex() * 32,
                    admission_key=self.public, publication_key=self.public,
                    cutoff_key=self.public, checkpoint_key=self.checkpoint_public,
                    rule_id=self.rule_id,
                    scan_origin=dict(height=0, hash=source['genesis']),
                    limits=copy.deepcopy(LIMITS))

    def pin_id(self, p):
        p = ordered('pin', p)
        p['source'] = ordered('source', p['source'])
        p['scan_origin'] = ordered('scan_origin', p['scan_origin'])
        p['limits'] = ordered('limits', p['limits'])
        need(p['version'] == 1 and p['rule_id'] == self.rule_id, 'rule')
        need(p['admission_key'] == p['publication_key'] == p['cutoff_key'] == self.public, 'fixture roles')
        need(p['checkpoint_key'] == self.checkpoint_public and p['limits'] == LIMITS, 'fixed pin')
        need(p['source']['version'] == 5 and p['scan_origin'] == dict(height=0, hash=p['source']['genesis']), 'source')
        hex_value(p['publication_id'], nonzero=True)
        return digest('SLUG fighter ownership pin', p).hex()

    def parse_pin(self, raw):
        p = parse(raw, 'pin')
        canonical = copy.deepcopy(p)
        for name in ('source', 'scan_origin', 'limits'):
            canonical[name] = ordered(name, p[name])
        need(J(canonical) == raw, 'nested canonical pin')
        self.pin_id(canonical)
        return canonical

    def signed(self, domain, body):
        out = copy.deepcopy(body)
        need('signature' not in out, 'signature preimage')
        out['signature'] = self.crypto.ed_sign(self.seed, digest(domain, body)).hex()
        return out

    def verify(self, kind, domain, value, expected_pin):
        v = ordered(kind, value)
        need(v['version'] == 1 and v['pin_id'] == expected_pin, 'signed identity')
        sig = hex_value(v.pop('signature'), 64)
        need(self.crypto.ed_verify(bytes.fromhex(self.public), digest(domain, v), sig), 'signature')
        return v

    def admission(self, pin, record, previous):
        pid = self.pin_id(pin)
        n = integer(record['sequence'], 1, LIMITS['admissions'], 'admission sequence')
        hex_value(previous)
        need((n == 1) == (previous == ZERO), 'admission link')
        body = dict(version=1, pin_id=pid, admission_sequence=n,
                    previous_admission_id=previous,
                    record_digest=digest('SLUG fighter ownership admission record', record).hex(),
                    record=copy.deepcopy(record))
        return self.signed('SLUG fighter ownership admission statement', body)

    def validate_admission(self, pin, value):
        v = self.verify('admission', 'SLUG fighter ownership admission statement', value, self.pin_id(pin))
        n = integer(v['admission_sequence'], 1, 1000, 'admission sequence')
        need(v['record']['sequence'] == n, 'record sequence')
        need((n == 1) == (v['previous_admission_id'] == ZERO), 'admission link')
        need(v['record_digest'] == digest('SLUG fighter ownership admission record', v['record']).hex(), 'record digest')
        need(v['record']['static_binding']['descriptor_id'] == pin['source']['descriptor_id'], 'descriptor')
        # The six exact record bodies are inherited independently qualified
        # inputs. This wrapper model does not mint new normal-work verdicts.
        return digest('SLUG fighter ownership admission id', value).hex()

    def publication(self, pid, n, previous, time_ms, kind, payload):
        integer(n, 1, 16384, 'publication sequence')
        integer(time_ms, 0, MAX_TIME, 'publication time')
        need((n == 1) == (previous == ZERO), 'publication link')
        need(kind in ('admission', 'checkpoint', 'chain', 'cutoff'), 'publication kind')
        body = dict(version=1, pin_id=pid, sequence=n, previous_id=previous,
                    attested_at_ms=time_ms, kind=kind, payload=copy.deepcopy(payload))
        return self.signed('SLUG fighter ownership publication statement', body)

    def publication_id(self, pid, value):
        v = self.verify('publication', 'SLUG fighter ownership publication statement', value, pid)
        shapes = {'admission':['admission_sequence','admission_id'],
                  'checkpoint':['checkpoint'],
                  'chain':['height','hash','previous_chain_entry_id'], 'cutoff':['cutoff_id']}
        need(v['kind'] in shapes and list(v['payload']) == shapes[v['kind']], 'payload shape')
        integer(v['sequence'], 1, 16384, 'publication count')
        integer(v['attested_at_ms'], 0, MAX_TIME, 'clock')
        need((v['sequence'] == 1) == (v['previous_id'] == ZERO), 'publication previous')
        return digest('SLUG fighter ownership publication id', value).hex()

    def validate_prefix(self, pid, entries):
        previous, clock = ZERO, 0
        for n, entry in enumerate(entries, 1):
            need(entry['sequence'] == n and entry['previous_id'] == previous, 'prefix gap')
            need(entry['attested_at_ms'] >= clock, 'prefix clock rollback')
            previous = self.publication_id(pid, entry)
            clock = entry['attested_at_ms']
        return previous

    def initial(self, pid, admission, gym):
        aid = digest('SLUG fighter ownership admission id', admission).hex()
        prospect = admission['record']['verified']['prospect_id']
        evidence = dict(version=1, pin_id=pid, prospect_id=prospect,
                        admission_id=aid, owner_public_key=gym)
        sid = digest('SLUG fighter ownership state', evidence).hex()
        return dict(version=1, pin_id=pid, prospect_id=prospect, state_id=sid,
                    owner_public_key=gym, birth_admission_id=aid, generation=0,
                    evidence=evidence)

    def transferred(self, pid, previous, transfer, checkpoint, descendant, recipient):
        evidence = dict(version=1, pin_id=pid, prospect_id=previous['prospect_id'],
                        prior_state_id=previous['state_id'], transfer_id=transfer,
                        checkpoint_publication_id=checkpoint,
                        sixtieth_descendant_height=descendant['height'],
                        sixtieth_descendant_hash=descendant['hash'],
                        owner_public_key=recipient)
        sid = digest('SLUG fighter ownership state', evidence).hex()
        return dict(version=1, pin_id=pid, prospect_id=previous['prospect_id'], state_id=sid,
                    owner_public_key=recipient, birth_admission_id=previous['birth_admission_id'],
                    generation=previous['generation']+1, evidence=evidence)

    def ownership_map(self, pid, sequence, state):
        value = dict(version=1, pin_id=pid, items=[dict(admission_sequence=sequence,
                     prospect_id=state['prospect_id'], state_id=state['state_id'])])
        return value, digest('SLUG fighter ownership map', value).hex(), digest('SLUG fighter ownership finalized prefix', value).hex()

    def validate_boundary(self, pid, boundary):
        b = self.verify('boundary', 'SLUG fighter ownership boundary statement', boundary, pid)
        v = ordered('intent', b['intent'])
        need(v['version'] == 1 and v['pin_id'] == pid, 'boundary source')
        integer(v['cutoff_ms'], 1, MAX_TIME, 'cutoff')
        integer(v['finality_sequence'], 0, 4096, 'frozen finality count')
        hex_value(v['finality_body_head'])
        need((v['finality_sequence'] == 0) == (v['finality_body_head'] == ZERO),
             'frozen finality empty/head relation')
        first, second = ordered('observation', v['first']), ordered('observation', v['second'])
        for o in (first, second):
            integer(o['request_start_ms'], 0, MAX_TIME, 'request time')
            integer(o['response_end_ms'], o['request_start_ms'], MAX_TIME, 'response time')
            need(o['response_end_ms']-o['request_start_ms'] <= 5000, 'call bound')
            hex_value(o['instance_id'], nonzero=True); hex_value(o['hash'])
            integer(o['height'], 0, 2**31-1, 'height')
            r = o['revision']
            need(type(r) is str and re.fullmatch('[1-9][0-9]{0,19}', r) and int(r) <= 2**64-1, 'revision')
        need(all(first[k] == second[k] for k in ('instance_id','revision','height','hash')), 'changed node')
        need(first['response_end_ms'] < v['cutoff_ms'] <= second['request_start_ms'], 'not straddling')
        need(second['response_end_ms']-first['request_start_ms'] <= 10000, 'bracket bound')
        return digest('SLUG fighter ownership boundary id', boundary).hex()

    def cutoff_frame_prefix(self, pid, boundary, frames):
        self.validate_boundary(pid, boundary)
        intent = boundary['intent']
        count = intent['finality_sequence']
        need(len(frames) >= count, 'required frozen frame signature unavailable')
        # Later frames, even at an older/equal chain height and publication
        # prefix, are outside this boundary's irreversible-history prefix.
        result = self.frame_prefix(pid, frames[:count])
        need(result['body_head'] == intent['finality_body_head'],
             'frozen unsigned body head mismatch')
        return result

    def frame(self, pid, body):
        need(body['pin_id'] == pid, 'frame pin')
        return self.signed('SLUG fighter ownership finality statement', body)

    def frame_prefix(self, pid, frames):
        previous, head = ZERO, ZERO
        for n, frame in enumerate(frames, 1):
            body = self.verify('frame', 'SLUG fighter ownership finality statement', frame, pid)
            need(n <= 4096 and body['sequence'] == n and body['previous_body_id'] == previous,
                 'frame body prefix')
            previous = digest('SLUG fighter ownership finality body', body).hex()
            head = digest('SLUG fighter ownership finality id', frame).hex()
        return dict(sequence=len(frames), body_head=previous, signed_head=head)


def prior_classification(prior, reachable, complete, missing_known=False,
                         future_or_nonfinal=False):
    """Conditional closed-world lookup, never a network absence proof."""
    if not complete or missing_known:
        return 'unavailable'
    if future_or_nonfinal or prior not in reachable:
        return 'invalid'
    return 'reachable'


def logical_metadata(rows, metadata):
    """Independent exact key+value accounting, including metadata's own digits."""
    need(len({bytes.fromhex(r['key_hex']) for r in rows}) == len(rows), 'duplicate storage key')
    fixed = sum(len(bytes.fromhex(r['key_hex']))+len(bytes.fromhex(r['value_hex'])) for r in rows)
    m = copy.deepcopy(metadata)
    for _ in range(12):
        value = fixed + len(b'meta') + len(J(m))
        if value == m['logical_bytes']:
            need(value <= LIMITS['logical_bytes'], 'logical capacity')
            return m
        m['logical_bytes'] = value
    raise Refused('metadata accounting did not converge')


def model_order(candidates, tip_height, checkpoint_present, missing=False):
    """Conditional policy model; candidate validity is explicitly modeled input.

    It cannot mint a production verified carrier or consensus result.
    """
    if missing:
        return dict(state='unavailable', winner=None, void=[])
    valid = sorted((x for x in candidates if x['modeled_valid']), key=lambda x:(x['height'],x['position']))
    if not valid:
        return dict(state='unchanged', winner=None, void=[])
    winner = valid[0]
    ready = checkpoint_present and tip_height-winner['height'] >= 60
    return dict(state='finalized' if ready else 'pending', winner=winner['name'],
                void=[x['name'] for x in valid[1:]])
