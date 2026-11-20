"""Frozen independent envelope edges; no product imports or Go execution."""
import functools
import hashlib
import importlib.util
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
source = HERE / 'bounds-v2.py'
assert hashlib.sha256(source.read_bytes()).hexdigest() == '4cc4d7d687193fe8add679d53879eda747c1a190a0135ade3fdc6779d624ad74'
spec = importlib.util.spec_from_file_location('independent_bound', source)
model = importlib.util.module_from_spec(spec)
spec.loader.exec_module(model)
model.bound = functools.lru_cache(maxsize=None)(model.bound)

def edge(name, source_max=4096, rounds=256, round_bytes=256 << 20,
         credit_rounds=256, batches=366, gyms=64):
    inputs = dict(source_max=source_max, rounds=rounds, round_bytes=round_bytes,
                  credit_rounds=credit_rounds, batches=batches, gyms=gyms)
    maximum = model.envelope(**inputs)
    expected = dict(submissions=maximum, max_batches=0, state_bytes=0,
                    round_bytes=0, largest_round_bytes=0)
    if maximum:
        bound = model.bound(maximum, gyms)
        for key in expected:
            expected[key] = bound[key]
    return dict(name=name, inputs=inputs, expected=expected)

if __name__ == '__main__':
    four = model.bound(4, 2)['round_bytes']
    result = dict(version=2, scope='independent logical reservation bounds, not financial or runtime authority',
                  cases=[edge('maximum-existing'), edge('larger-round-journal', round_bytes=1 << 30),
                         edge('batch-364', batches=364), edge('batch-365', batches=365),
                         edge('source-four', source_max=4), edge('round-four', rounds=4),
                         edge('credit-four', credit_rounds=4),
                         edge('small-selected', source_max=4, rounds=4, round_bytes=8 << 20, credit_rounds=4, batches=8, gyms=2),
                         edge('exact-round-byte-edge', source_max=4, gyms=2, round_bytes=four),
                         edge('below-round-byte-edge', source_max=4, gyms=2, round_bytes=four-1),
                         edge('zero-reservation', round_bytes=4096),
                         edge('one-batch', batches=1), edge('one-payee', gyms=1)],
                  refused_inputs=[dict(source_max=0),dict(source_max=4097),dict(rounds=0),dict(rounds=257),
                                  dict(round_bytes=4095),dict(round_bytes=(1 << 30)+1),dict(credit_rounds=257),
                                  dict(batches=0),dict(batches=367),dict(gyms=0),dict(gyms=65)])
    print(json.dumps(result, indent=2))
