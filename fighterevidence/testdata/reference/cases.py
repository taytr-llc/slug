"""Finite public/model mutations fixed before running the Set66 reference.

Modeled rows and unsigned-time controls are explicitly supplied claims, never
new actual events, issuance or signatures. No signing operation is performed.
"""
import copy
import hashlib
import json

from evidence_schema import canonical

ACTUAL_NAMES = tuple(version + '/' + name for version in ('v1', 'v2')
                     for name in ('incrementing256-awarded', 'zero256-awarded', 'zero256-unawarded'))
COUNTS = (0, 1, 8, 9, 15, 16, 256)
METHODS = ('decision', 'ko', 'tko_referee', 'tko_three_knockdowns',
           'tko_corner', 'decision_draw', 'double_corner_stoppage')
RAW_CONTROLS = ('trailing_lf', 'leading_space', 'unknown_root', 'duplicate_root',
                'duplicate_nested', 'missing_birth_rule', 'root_wrong_order',
                'escaped_format', 'version_exponent', 'version_negative_zero',
                'sequence_overflow', 'null_rows', 'null_merkle', 'slots9', 'slots11',
                'merkle21', 'coinbase16385', 'capture_overlimit', 'row_overlimit',
                'event_overlimit', 'rows257', 'file_overlimit', 'unexpected_depth')
SHAPE_CONTROLS = ('foreign_expected_gym', 'foreign_expected_source', 'wrong_supported_descriptor',
                  'accepted_at_expiry', 'born_not_issued', 'wrong_source_digest_framing',
                  'high_s_signature', 'zero_r_signature', 'altered_header',
                  'uppercase_header', 'uppercase_coinbase', 'uppercase_merkle', 'uppercase_signature',
                  'omitted_row', 'reordered_page_boundary', 'duplicate_result_page_boundary',
                  'interpage_totals', 'history_id', 'effective_after_capture')


def clone(v):
    return copy.deepcopy(v)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def construct(admission, capture, rows):
    return dict(format='slug-public-fighter-evidence', version=1,
                admission=clone(admission), capture=clone(capture), rows=clone(rows))


def rebind(reference, archive):
    r, c = archive['admission'], archive['capture']
    a = c['assignment']
    a.update(prospect_id=r['verified']['prospect_id'], gym_id=r['verified']['gym_id'],
             work_hash=r['verified']['work_hash'], born_at_ms=r['birth']['born_at_ms'],
             accepted_at_ms=r['birth']['accepted_at_ms'], descriptor_id=r['static_binding']['descriptor_id'],
             manifest_digest=r['static_binding']['manifest_digest'], portrait_digest=r['static_binding']['portrait_digest'])
    raw = canonical(r)
    a['source_record_digest'] = sha(b'SLUG stable source record\0v1\0' + len(raw).to_bytes(4, 'little') + raw)
    c['history_id'] = reference.history.history_id(c)


def modeled(reference, original, count, mode='ordinary'):
    h = reference.history
    c = clone(original['capture'])
    template = original['rows'][0]['event']
    at = c['as_of_ms'] + 1000
    events = []
    for i in range(count):
        # The archived function's modeled-* domains and explicit index are
        # unchanged. This records modeled evidence rather than inventing proof.
        events.append(h.model_event(c['activation'], template, 660000 + i,
                                    at if mode == 'ties' else at + i,
                                    METHODS[i % len(METHODS)], ('a', 'b')[i % 2],
                                    unrelated=(mode == 'mixed' and i % 3 == 0)))
    captured = h.derive_capture(c['activation'], events, c['status'], c['assignment'],
                                at + count, at + count)
    return construct(original['admission'], captured['capture'], captured['rows'])


def positives(reference, actual):
    output = {}
    for name in ACTUAL_NAMES:
        expected, archive = actual[name]
        output['actual/' + name] = (expected, archive, 'observed_actual_input_conditional_verification')
    for version in ('v1', 'v2'):
        expected, base = actual[version + '/zero256-awarded']
        for n in COUNTS:
            output[f'modeled/{version}/count{n}'] = (expected, modeled(reference, base, n), 'modeled_history_real_original_receipt')
        for mode, n in (('mixed', 24), ('ties', 16)):
            output[f'modeled/{version}/{mode}'] = (expected, modeled(reference, base, n, mode), 'modeled_history_real_original_receipt')
        omitted = clone(base)
        omitted['rows'] = []
        omitted['capture']['event_count'] = 0
        omitted['capture']['totals'] = reference.history.zero()
        omitted['capture']['history_id'] = reference.history.history_id(omitted['capture'])
        output[f'limit/{version}/coherent_omission'] = (expected, omitted, 'deliberate_limit_no_completeness_authentication')
        changed_time = clone(base)
        job = changed_time['admission']['job']
        previous = changed_time['admission']['birth']['accepted_at_ms']
        changed_time['admission']['birth']['accepted_at_ms'] = job['issued_at_ms'] + (2 if previous == job['issued_at_ms'] + 1 else 1)
        rebind(reference, changed_time)
        output[f'limit/{version}/unsigned_acceptance_time'] = (expected, changed_time, 'deliberate_limit_no_acceptance_time_authentication')
    return output


def raw_mutation(name, base, all_cases):
    x = clone(base)
    raw = canonical(x)
    if name == 'trailing_lf': return raw + b'\n'
    if name == 'leading_space': return b' ' + raw
    if name == 'unknown_root': return b'{"unknown":0,' + raw[1:]
    if name == 'duplicate_root': return raw.replace(b'"version":1,', b'"version":1,"version":1,', 1)
    if name == 'duplicate_nested': return raw.replace(b'"admission":{"version":5,', b'"admission":{"version":5,"version":5,', 1)
    if name == 'missing_birth_rule': del x['admission']['birth']['rule_version']
    elif name == 'root_wrong_order': x = {'version': x['version'], 'format': x['format'], **{k: x[k] for k in ('admission','capture','rows')}}
    elif name == 'escaped_format': return raw.replace(b'slug-public', b'\\u0073lug-public', 1)
    elif name == 'version_exponent': return raw.replace(b'"version":1,', b'"version":1e0,', 1)
    elif name == 'version_negative_zero': return raw.replace(b'"version":1,', b'"version":-0,', 1)
    elif name == 'sequence_overflow': x['admission']['sequence'] = 1 << 64
    elif name == 'null_rows': x['rows'] = None
    elif name == 'null_merkle': x['admission']['proof']['merkle_branch'] = None
    elif name == 'slots9': x['capture']['status']['slots'] = [''] * 9
    elif name == 'slots11': x['capture']['status']['slots'] = [''] * 11
    elif name == 'merkle21': x['admission']['proof']['merkle_branch'] = ['0' * 64] * 21
    elif name == 'coinbase16385': x['admission']['proof']['coinbase'] = '00' * 16385
    elif name == 'capture_overlimit':
        p = x['capture']['status']['pin']
        p['account_authority_id'] = p['account_id'] = p['source']['authority_id'] = 'a' * 4096
    elif name in ('row_overlimit','event_overlimit'):
        x = clone(all_cases['modeled/v2/count9'][1])
        if name == 'event_overlimit': x['rows'][0]['event']['saturday'] = '2' * 1400
        else:
            row = x['rows'][0]
            row['ordinal'] = row['xp_before'] = row['xp_applied'] = (1 << 32) - 1
            for section in ('earned', 'totals_after'):
                for k in row[section]: row[section][k] = (1 << 32) - 1
    elif name == 'rows257':
        x = clone(all_cases['modeled/v2/count256'][1]);x['rows'].append(clone(x['rows'][-1]))
    elif name == 'file_overlimit': return b'0' * 524289
    elif name == 'unexpected_depth': return raw.replace(b'"version":1,', b'"version":' + b'[' * 11 + b'1' + b']' * 11 + b',', 1)
    else:
        if name not in RAW_CONTROLS: raise AssertionError('unknown mutation')
    return canonical(x)


def shape_mutation(name, reference, actual, all_cases):
    expected, original = actual['v2/zero256-awarded']
    expected, x = clone(expected), clone(original)
    if name == 'foreign_expected_gym': expected = clone(actual['v2/incrementing256-awarded'][0])
    elif name == 'foreign_expected_source': expected = clone(actual['v1/zero256-awarded'][0])
    elif name == 'wrong_supported_descriptor': x['admission']['job']['descriptor_id'] = actual['v1/zero256-awarded'][0]['source']['descriptor_id']
    elif name == 'accepted_at_expiry': x['admission']['birth']['accepted_at_ms'] = x['admission']['job']['expires_at_ms'];rebind(reference,x)
    elif name == 'born_not_issued': x['admission']['birth']['born_at_ms'] += 1;rebind(reference,x)
    elif name == 'wrong_source_digest_framing':
        x['capture']['assignment']['source_record_digest'] = sha(b'SLUG stable source record\0v1\0' + canonical(x['admission']))
        x['capture']['history_id'] = reference.history.history_id(x['capture'])
    elif name in ('high_s_signature','zero_r_signature'):
        auth = x['admission']['authorization']
        r,s = reference.admission.parse_signature(auth['signature'])
        if name == 'high_s_signature':
            encoded = reference.jobs.AUTH.der_signature(r,reference.jobs.AUTH.ORDER-s)
        else:
            # Deliberately invalid DER integer zero. The old positive-only
            # serializer correctly refuses zero, so frame this negative here.
            s_field = reference.jobs.AUTH.der_signature(1,s)[5:]
            body = b'\x02\x01\x00' + s_field
            encoded = b'\x30' + bytes((len(body),)) + body
        auth['signature'] = encoded.hex();rebind(reference,x)
    elif name == 'altered_header':
        header = bytearray.fromhex(x['admission']['proof']['header']);header[-1] ^= 1
        x['admission']['proof']['header'] = header.hex();rebind(reference,x)
    elif name in ('uppercase_header','uppercase_coinbase','uppercase_merkle','uppercase_signature'):
        if name == 'uppercase_signature':
            x['admission']['authorization']['signature'] = x['admission']['authorization']['signature'].upper()
        elif name == 'uppercase_merkle':
            branch = x['admission']['proof']['merkle_branch']
            x['admission']['proof']['merkle_branch'] = [v.upper() for v in branch] if branch else ['AB'*32]
        else:
            key = name.removeprefix('uppercase_')
            x['admission']['proof'][key] = x['admission']['proof'][key].upper()
        rebind(reference,x)
    elif name in ('omitted_row','reordered_page_boundary','duplicate_result_page_boundary','interpage_totals'):
        expected,x,_ = all_cases['modeled/v2/count9'];expected,x=clone(expected),clone(x)
        if name == 'omitted_row': x['rows'].pop()
        elif name == 'reordered_page_boundary': x['rows'][7],x['rows'][8] = x['rows'][8],x['rows'][7]
        elif name == 'duplicate_result_page_boundary':
            e=x['rows'][8]['event'];e['result_key']=x['rows'][0]['event']['result_key']
            e['award_id']=reference.history.award_id(x['capture']['activation'],e['result_key'])
        else: x['rows'][7]['totals_after']['base_points'] += 4
    elif name == 'history_id': x['capture']['history_id'] = '0' * 64
    elif name == 'effective_after_capture': x['rows'][0]['event']['effective_at_ms'] = x['capture']['as_of_ms'] + 1
    else: raise AssertionError('unknown mutation')
    return expected, canonical(x)


def work_mismatch(reference, actual, version):
    expected, x = actual[version + '/zero256-unawarded'];expected,x=clone(expected),clone(x)
    r=x['admission'];r['verified']=reference.admission.verified(r['job'],r['proof'],'0'*64)
    _,r['static_binding']=reference.appearance('0'*64,r['job']['descriptor_id'])
    c=r['context'];r['eligibility']=dict(rule_version=2,source='current-parent',hash=c['parent_hash'],height=c['parent_height'])
    rebind(reference,x)
    return expected,canonical(x)


def plan():
    return dict(version=1, scope='fixed before reference outcomes; modeled histories explicitly not actual awards',
                actual_cases=list(ACTUAL_NAMES), modeled_per_version=dict(counts=list(COUNTS), mixed_prefix=24, ties=16),
                modeled_event_index_start=660000, modeled_time='captured as_of_ms+1000+i; ties use constant start',
                modeled_methods=list(METHODS), modeled_corner_winners=['a','b'],
                limit_positives=['coherent_omission','unsigned_acceptance_time'],
                raw_negative_controls=list(RAW_CONTROLS), shape_negative_controls=list(SHAPE_CONTROLS),
                work_negative_controls=['v1/claimed_zero_work_hash','v2/claimed_zero_work_hash'],
                work_negative_recipe='Original proof/header/auth unchanged; recompute only unsigned Verified/static/assignment/capture claims using zero work hash, select consistent current-parent observation; actual Argon must disagree.',
                max_unique_actual_headers=6, normal_hash_reuse='Only exact header bytes previously computed by the independent direct primitive within this run; no supplied hashes accepted.',
                expected_positive_count=28, expected_negative_count=44)
