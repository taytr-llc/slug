"""Independent two-axis extension of the frozen Set74 capacity arithmetic.

No Go import/execution or production output. Literal JSON shapes, claims and
lineage equations are inherited from the already frozen independent producer.
Only the total-row domain and independently capped block-row axis change.
"""
import hashlib
import importlib.util
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
INHERITED = HERE / 'inherited-bounds-v2.py'
assert hashlib.sha256(INHERITED.read_bytes()).hexdigest() == '4cc4d7d687193fe8add679d53879eda747c1a190a0135ade3fdc6779d624ad74'

# Textual extension of the independent arithmetic domain, not of any product.
# Its case() uses precisely the same literal wire sizes and rational bound.
source = INHERITED.read_text()
old = 'assert 1 <= r <= n <= 256 and 1 <= gyms <= 64'
assert source.count(old) == 1
scope = {'__name__': 'inherited_capacity_reference'}
exec(compile(source.replace(old, 'assert 1 <= r <= min(n, 256) and n <= 4096 and 1 <= gyms <= 64'), str(INHERITED), 'exec'), scope)

DEFAULT = dict(source_max=4096, rounds=256, round_bytes=256 << 20,
               credit_rounds=256, batches=366, gyms=64)
FIELDS = ('submissions', 'max_batches', 'state_bytes', 'round_bytes', 'largest_round_bytes')


def valid(c):
    if set(c) != set(DEFAULT) or any(type(x) is not int for x in c.values()):
        return False
    return (1 <= c['source_max'] <= 4096 and 1 <= c['rounds'] <= 256
            and 4096 <= c['round_bytes'] <= 1 << 30
            and 1 <= c['credit_rounds'] <= 256 and 1 <= c['batches'] <= 366
            and 1 <= c['gyms'] <= 64)


def bound(n, blocks, gyms):
    out = dict.fromkeys(FIELDS, 0)
    out['submissions'] = n
    if not blocks:
        return out
    rows = [scope['case'](n, r, gyms) for r in range(1, blocks + 1)]
    out.update(max_batches=2 * blocks - 1,
               state_bytes=max(r['state_bytes'] for r in rows),
               round_bytes=max(r['round_bytes'] for r in rows),
               largest_round_bytes=max(r['largest_round_bytes'] for r in rows))
    return out


def recover(c, n, blocks):
    if not valid(c) or type(n) is not int or type(blocks) is not int or n < 0 or blocks < 0 or n > c['source_max'] or blocks > n:
        return None, 'invalid'
    if blocks > min(c['rounds'], c['credit_rounds'], 256) or (blocks and 2 * blocks - 1 > c['batches']):
        return None, 'capacity'
    b = bound(n, blocks, c['gyms'])
    if (b['state_bytes'] > scope['MAX_STATE'] - scope['STATUS_RESERVE']
            or b['round_bytes'] > c['round_bytes'] or b['largest_round_bytes'] > 2 << 20):
        return None, 'capacity'
    return b, ''


def entry(name, n, blocks, **updates):
    c = dict(DEFAULT, **updates)
    b, error = recover(c, n, blocks)
    return dict(name=name, inputs=c, submissions=n, blocks=blocks,
                expected=b, error=error)


def vectors():
    small = dict(source_max=4, rounds=4, round_bytes=8 << 20,
                 credit_rounds=4, batches=4, gyms=2)
    actual = dict(source_max=8, rounds=4, round_bytes=8 << 20,
                  credit_rounds=4, batches=8, gyms=2)
    rows = [entry('empty', 0, 0), entry('ordinary-only', 183, 0),
            entry('first-winner', 1, 1),
            entry('actual-ordinary-two', 2, 2, **small),
            entry('late-third-row-second-block', 3, 2, **small),
            entry('third-block-does-not-fit-four-batches', 4, 3, **small),
            entry('actual-ordinary-four', 4, 4, **actual),
            entry('actual-late-fifth-row-third-block', 5, 3, **actual),
            entry('183-ordinary-then-first-block', 184, 1),
            entry('183-rows-then-second-block', 184, 2),
            entry('183-rows-then-184th-block', 184, 184),
            entry('physical-last-row-single-block', 4096, 1),
            entry('physical-overflow', 4097, 1),
            entry('block-count-exceeds-row-count', 1, 2),
            entry('credit-bound', 5, 3, credit_rounds=2),
            entry('round-bound', 5, 3, rounds=2),
            entry('byte-bound', 5, 3, round_bytes=4096),
            entry('zero-row-nonzero-block', 0, 1),
            entry('state-byte-bound', 4096, 183),
            entry('unknown-payee-count-conservative', 256, 2, gyms=1)]
    rb = bound(3, 2, 2)['round_bytes']
    rows += [entry('exact-round-byte-edge', 3, 2, **dict(small, round_bytes=rb)),
             entry('below-round-byte-edge', 3, 2, **dict(small, round_bytes=rb - 1))]
    legacy = []
    for n in (1, 4, 16, 64, 127, 128, 183):
        expected = {k: scope['bound'](n, 64)[k] for k in FIELDS}
        assert bound(n, n, 64) == expected
        legacy.append(dict(submissions=n, blocks=n, gyms=64, expected=expected))
    invalid = [dict(source_max=0), dict(source_max=4097), dict(rounds=0),
               dict(rounds=257), dict(round_bytes=4095), dict(round_bytes=(1 << 30)+1),
               dict(credit_rounds=0), dict(credit_rounds=257), dict(batches=0),
               dict(batches=367), dict(gyms=0), dict(gyms=65)]
    for change in invalid:
        assert recover(dict(DEFAULT, **change), 1, 1)[1] == 'invalid'
    return dict(version=1, scope='independent arithmetic only; no spendability or canonical authority',
                inherited_sha256=hashlib.sha256(INHERITED.read_bytes()).hexdigest(),
                cases=rows, legacy_equivalence=legacy, invalid_inputs=invalid,
                true_ordinary_ceilings=[dict(inputs=small, expected=2),
                                        dict(inputs=actual, expected=4)])


if __name__ == '__main__':
    print(json.dumps(vectors(), indent=2))
