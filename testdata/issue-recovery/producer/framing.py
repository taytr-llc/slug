"""Independent bounded framing recipes. Large whitespace is represented compactly."""
from core import canon, classify, clone, decode, need, sha
from fixtures import exact_wrapper
from outbox import qrow, request, scan, validate_q
from states import (intent, source_row, issuer_row, make_journal, response_for,
                    validate_intent, validate_journal, validate_response)


def exact(value, expected):
    need(canon(value) == canon(expected), 'closed-selected-value')


def produce_framing(fixtures, states, wire):
    family = fixtures['matching_families'][0]
    targets = []

    def target(name, value, cap, framing='value', validator=None):
        targets.append(dict(name=name, value=value, cap=cap, framing=framing,
                            validator=validator or (lambda v, e=clone(value): exact(v, e))))

    target('request', dict(request_id=family['request_id']), 1024, 'http', request)
    target('intent', intent(family), 4096, validator=lambda v: validate_intent(v, family))
    for state in ('pending', 'work', 'issued_unused', 'not_issued'):
        target('source-' + state, source_row(family, state), 4096)
    for state in ('issued', 'sealed'):
        target('issuer-' + state, issuer_row(family, state), 4096)
    for state in ('absent', 'pending', 'work', 'issued_unused', 'not_issued'):
        target('lookup-' + state, response_for(family, state), (16 << 20) + (64 << 10), 'http',
               lambda v: validate_response(v, family, True))
    qcap = len(canon(qrow(family))) - 4 + (16 << 20) + (64 << 10)
    target('retained-q-null', qrow(family), qcap, validator=lambda v: validate_q(v, family))
    target('retained-q-work', qrow(family, 'work'), qcap, validator=lambda v: validate_q(v, family))
    target('scan-reference', scan(family), (16 << 20) + (64 << 10))
    # Every new file/metadata wrapper family is represented; matching larger
    # 64-enrollment objects are already emitted by fixtures, avoiding repetition.
    chosen = [v for v in fixtures['wrappers'] if v['name'].startswith(('one-enrollment-', 'coinbase-only-', 'zero256-default-'))]
    for row in chosen:
        name = row['name']
        if 'issuer-' in name:
            cap = 8192
        elif 'source-' in name:
            cap = 16384
        elif 'worker-' in name:
            cap = 2 * 131072
        elif 'credential-' in name:
            cap = 2048 + 1024
        elif name.endswith('-ready'):
            cap = 1024
        elif name.endswith('-provision-intent'):
            cap = 131072 + 1024
        elif name.endswith('-published-fixture-backup'):
            cap = 8192 + 1024
        elif name.endswith('-client'):
            cap = 65536 + 1024
        elif name.endswith('-remote-intent'):
            cap = 262144 + 512 + 1024
        else:
            cap = 262144 + 1024 + (1 if row['framing'] == 'lf' else 0)
        target(name, row['value'], cap, row['framing'])
    for group, cap in [('balance', 65536), ('page', 131072), ('work-receipt', (16 << 20) + (64 << 10))]:
        row = next(v for v in wire['response_cases'] if v['valid'] and v['name'].startswith(group + '-'))
        target('existing-route-' + group, row['value'], cap, 'http')
    error = next(v for v in wire['error_cases'] if v['valid'])
    target('error-envelope', error['value'], 65536, 'http')
    cases = []

    def add(t, name, raw, valid, compact=None):
        def check():
            t['validator'](decode(raw, t['cap'], t['framing']))
        okay, reason = classify(check)
        need(okay is valid, 'raw-classification:' + t['name'] + ':' + name)
        row = dict(name=t['name'] + '/' + name, target=t['name'], framing=t['framing'], cap=t['cap'],
                   raw_bytes=len(raw), raw_sha256=sha(raw), valid=valid, refusal=reason)
        if compact is None:
            row['raw_hex'] = raw.hex()
        else:
            row['raw_recipe'] = compact
        cases.append(row)

    for t in targets:
        body = canon(t['value'])
        ending = b'\n' if t['framing'] == 'lf' else b''
        raw = body + ending
        add(t, 'canonical', raw, True)
        add(t, 'unknown-field', body[:-1] + b',"unknown":0}' + ending, False)
        key = next(iter(t['value']))
        duplicate = b'{' + canon(key) + b':' + canon(t['value'][key]) + b',' + body[1:]
        add(t, 'duplicate-field', duplicate + ending, False)
        ordered = dict(reversed(list(t['value'].items())))
        if len(ordered) > 1:
            add(t, 'reordered-fields', canon(ordered) + ending, False)
        missing = clone(t['value'])
        del missing[key]
        add(t, 'missing-field', canon(missing) + ending, False)
        add(t, 'invalid-utf8', b'\xff' + raw, False)
        add(t, 'vertical-tab-prefix', b'\x0b' + raw, False)
        add(t, 'trailing-value', raw + b'{}', False)
        if 'version' in t['value']:
            alternate = body.replace(b'"version":' + str(t['value']['version']).encode(), b'"version":1e0', 1)
            add(t, 'alternate-number', alternate + ending, False)
        if t['framing'] == 'http':
            add(t, 'standard-outer-whitespace', b' \t\r\n' + body + b'\n\r\t ', True)
            # Only one large target needs cap-boundary construction; all share
            # the same independent cap-before-trim predicate. Never emit its hex.
            if t['name'] in ('request', 'lookup-work', 'existing-route-balance', 'existing-route-page', 'error-envelope'):
                for delta in (0, 1):
                    padding = t['cap'] - len(body) + delta
                    need(padding >= 0, 'selected-frame-exceeds-cap')
                    huge = b' ' * padding + body
                    add(t, 'pretrim-cap' + ('-plus-one' if delta else '-exact'), huge, not delta,
                        dict(prefix_byte_hex='20', repeat=padding, suffix_hex=body.hex()))
            add(t, 'encoder-final-lf', body + b'\n', True)
        elif t['framing'] == 'lf':
            add(t, 'missing-final-lf', body, False)
            add(t, 'double-final-lf', raw + b'\n', False)
        else:
            add(t, 'unexpected-final-lf', body + b'\n', False)
    return dict(cases=cases, targets=[{k: v for k, v in t.items() if k != 'validator'} for t in targets],
                scope='Exact ordered new byte grammar; large cap boundaries are compact deterministic raw recipes, not actual authenticated maximum rows.')
