"""Frozen Set66 closed byte grammar. No imports of project Go or work code.

This producer is staged, not executed against inputs before root's input freeze.
Schema declarations are transcribed from Appendix A of the independently
reviewed contract, not inferred from a new encoder's output.
"""
import json

CONTRACT_SHA256 = '16a9166ebe1c7a0f5b0a8d554a043e6a690c29bec7864df9de123fc5cc01f272'
MAX_FILE = 524288
MAX_SAFE = 9007199254740991
U32 = ('int', (1 << 32) - 1)
U64 = ('int', (1 << 64) - 1)
I64 = ('int', (1 << 63) - 1)
SAFE = ('int', MAX_SAFE)
BOOL = ('bool',)
S = ('str', 4096)


class Refusal(ValueError):
    def __init__(self, code='invalid_evidence'):
        self.code = code
        super().__init__(code)


def need(condition, code='invalid_evidence'):
    if not condition:
        raise Refusal(code)


def obj(**fields):
    return ('object', tuple(fields.items()))


def array(element, minimum, maximum):
    return ('array', element, minimum, maximum)


CONTEXT = obj(version=U32, network=U32, genesis=S, dna_version=U32,
              prospect_target=S, parent_hash=S, parent_height=U32,
              expected_bits=U32, min_timestamp=U32, max_timestamp=U32)
DNA = obj(dna_version=U32, seed=S, rarity=S, talent_budget=U32,
          attributes=obj(power=U32, speed=U32, chin=U32, stamina=U32, skill=U32),
          prime_weeks=U32, decline_weeks=U32)
PROOF = obj(version=U32, gym_id=S, header=('str', 160),
            coinbase=('str', 32768), merkle_branch=array(('str', 64), 0, 20), job_id=S)
VERIFIED = obj(version=U32, verification=S, network=U32, genesis=S,
               prospect_id=S, policy_id=S, gym_id=S, work_hash=S,
               parent_hash=S, parent_height=U32, dna=DNA)
AUTH = obj(version=U32, public_key=('str', 66), signature=('str', 144))
JOB = obj(version=U32, rule_version=U32, id=S, authority_id=S, descriptor_id=S,
          context=CONTEXT, public_key=('str', 66), gym_id=S, challenge=S,
          issued_at_ms=I64, expires_at_ms=I64)
ADMISSION = obj(version=U32, sequence=U64, context=CONTEXT, proof=PROOF,
                verified=VERIFIED, authorization=AUTH, job=JOB,
                birth=obj(rule_version=U32, born_at_ms=I64, accepted_at_ms=I64),
                eligibility=obj(rule_version=U32, source=S, hash=S, height=U32),
                static_binding=obj(version=U32, descriptor_id=S, manifest_digest=S, portrait_digest=S))
IDENTITY = obj(network=U32, genesis=S, profile=S, root_public_key=S, gym_public_key=S, gym_id=S)
SOURCE = obj(version=U32, network=U32, genesis=S, authority_id=S, descriptor_id=S)
PIN = obj(version=U32, account_authority_id=S, account_id=S, identity=IDENTITY,
          source=SOURCE, condition_version=U32, projector_id=S)
STATUS = obj(version=U32, store_id=S, pin=PIN, checkpoint=SAFE,
             observed_source_count=SAFE, caught_up=BOOL, roster_revision=SAFE,
             slots=array(S, 10, 10), operation_count=U32,
             pending_operation_id=S, last_operation_id=S, last_as_of_ms=SAFE)
ACTIVATION = obj(version=U32, policy_id=S, activation_id=S, result_store_id=S,
                 result_created_at_ms=SAFE, source_created_at_ms=SAFE, activated_at_ms=SAFE)
ASSIGNMENT = obj(version=U32, sequence=SAFE, prospect_id=S, gym_id=S, work_hash=S,
                 born_at_ms=SAFE, accepted_at_ms=SAFE, descriptor_id=S,
                 manifest_digest=S, portrait_digest=S, source_record_digest=S)
TOTALS = obj(wins=U32, losses=U32, draws=U32, xp=U32, base_points=U32)
CAPTURE = obj(version=U32, status=STATUS, activation=ACTIVATION, career_digest=S,
              as_of_ms=SAFE, assignment=ASSIGNMENT, prefix_count=U32,
              event_count=U32, totals=TOTALS, history_id=S)
CORNER = obj(prospect_id=S, account_id=S, gym_id=S)
EVENT = obj(version=U32, award_id=S, result_key=S, card_id=S, bout_id=S,
            source_record_digest=S, input_id=S, result_digest=S, finality_digest=S,
            effective_at_ms=SAFE, saturday=S, corner_a=CORNER, corner_b=CORNER,
            winner=S, method=S)
ROW = obj(ordinal=U32, event=EVENT, earned=TOTALS, xp_before=U32,
          xp_applied=U32, totals_after=TOTALS)
ARCHIVE = obj(format=S, version=U32, admission=ADMISSION, capture=CAPTURE,
              rows=array(ROW, 0, 256))


def canonical(value):
    # Existing Go public canonical objects use HTML escaping. All actual values
    # in this archive have stricter ASCII semantics, but spelling is explicit.
    text = json.dumps(value, ensure_ascii=True, separators=(',', ':'), allow_nan=False)
    for before, after in (('<', '\\u003c'), ('>', '\\u003e'), ('&', '\\u0026')):
        text = text.replace(before, after)
    return text.encode('ascii')


class Scanner:
    """A bounded expected-schema reader; never first decode an arbitrary tree."""
    def __init__(self, raw, maximum=MAX_FILE):
        need(type(raw) is bytes and 0 < len(raw) <= maximum and raw.isascii())
        self.raw, self.pos, self.spans, self.ends = raw, 0, {}, [maximum]

    def advance(self, n):
        need(self.pos + n <= min(len(self.raw), self.ends[-1]))
        self.pos += n

    def take(self, token):
        need(self.raw[self.pos:self.pos + len(token)] == token)
        self.advance(len(token))

    def string(self, maximum):
        start = self.pos
        self.take(b'"')
        escaped = False
        while self.pos < len(self.raw):
            ch = self.raw[self.pos]
            need(ch >= 32)
            self.advance(1)
            # A bounded escaped scalar never becomes an unbounded JSON string.
            need(self.pos - start <= maximum * 6 + 2)
            if escaped:
                escaped = False
            elif ch == 92:
                escaped = True
            elif ch == 34:
                token = self.raw[start:self.pos]
                try:
                    result = json.loads(token)
                except (ValueError, UnicodeError) as error:
                    raise Refusal() from error
                need(type(result) is str and result.isascii() and len(result) <= maximum)
                need(canonical(result) == token)
                return result
        raise Refusal()

    def read(self, schema, path='', depth=1):
        kind = schema[0]
        start = self.pos
        component_limit = {'admission': 65536, 'capture': 8192,
                           'rows[*]': 1536, 'rows[*].event': 1344}.get(path)
        if component_limit is not None:
            self.ends.append(min(self.ends[-1], start + component_limit))
        if kind in ('object', 'array'):
            need(depth <= 10)
            if path == 'admission' or path.startswith('admission.'):
                need(depth - 1 <= 4)
            if path == 'capture' or path.startswith('capture.'):
                need(depth - 1 <= 8)
            if path.startswith('rows[*]'):
                need(depth - 2 <= 8)
        if kind == 'object':
            self.take(b'{')
            value = {}
            for n, (key, child) in enumerate(schema[1]):
                if n:
                    self.take(b',')
                self.take(canonical(key) + b':')
                value[key] = self.read(child, path + '.' + key if path else key, depth + 1)
            self.take(b'}')
        elif kind == 'array':
            self.take(b'[')
            value = []
            while self.pos < len(self.raw) and self.raw[self.pos] != 93:
                need(len(value) < schema[3])
                if value:
                    self.take(b',')
                value.append(self.read(schema[1], path + '[*]', depth + 1))
            self.take(b']')
            need(schema[2] <= len(value) <= schema[3])
        elif kind == 'str':
            value = self.string(schema[1])
        elif kind == 'bool':
            if self.raw[self.pos:self.pos + 4] == b'true':
                self.take(b'true')
                value = True
            else:
                self.take(b'false')
                value = False
        elif kind == 'int':
            while self.pos < len(self.raw) and 48 <= self.raw[self.pos] <= 57:
                self.advance(1)
                need(self.pos - start <= 20)
            token = self.raw[start:self.pos]
            need(bool(token) and (token == b'0' or token[0] != 48))
            value = int(token)
            need(value <= schema[1])
        else:
            raise AssertionError('unknown author-controlled schema kind')
        if component_limit is not None:
            need(self.pos - start <= component_limit)
            self.spans.setdefault(path, []).append((start, self.pos))
            self.ends.pop()
        return value

    def finish(self, schema=ARCHIVE):
        value = self.read(schema)
        need(self.pos == len(self.raw) and canonical(value) == self.raw)
        return value


def parse_archive(raw):
    value = Scanner(raw).finish()
    need(value['format'] == 'slug-public-fighter-evidence' and value['version'] == 1)
    return value


def parse_expected_pin(raw):
    # Reference input pin files are frozen canonical values. Production CLI
    # retains its existing ParsePin transport grammar instead of widening it.
    return Scanner(raw, 4096).finish(PIN)
