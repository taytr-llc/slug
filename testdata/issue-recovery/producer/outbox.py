"""Independent selected outbox grammar and conditional prevalidated-entry links.

The e/h/p bodies and PoW remain inherited anchors. Abstract entry byte/reserve
lengths below are explicitly conditional, never replacement authenticated rows.
"""
import struct

from core import Cases, MAX_U64, canon, clone, fields, hexid, integer, need
from states import response_for, validate_response

OUTCOME = 40 << 20
RESPONSE = (16 << 20) + (64 << 10)


def qrow(family, state=None):
    return dict(version=1, request=dict(request_id=family['request_id']),
                result=None if state is None else response_for(family, state))


def request(value):
    fields(value, 'request_id')
    hexid(value['request_id'])


def validate_q(value, family):
    fields(value, 'version,request,result')
    integer(value['version'], 1, 1)
    request(value['request'])
    if value['result'] is not None:
        # One original owner may retain many requests. A response must bind its
        # own q key/request; the selected source/money/gym remain unchanged.
        selected = dict(family, request_id=value['request']['request_id'])
        validate_response(value['result'], selected, False)


def scan(family):
    return dict(version=1, request_id=family['request_id'])


def active_reserve(row, family):
    # The response bound replaces exactly the four bytes of required null.
    empty = len(canon(qrow(family)))
    maximum_q = empty - 4 + RESPONSE
    return maximum_q - len(canon(row)) + len(canon(scan(family))) + 8 + OUTCOME


def state(family, result=None, scanning=False, entry=False):
    rows = [] if result == 'absent' else [qrow(family, result)]
    entries = []
    if entry:
        entries = [dict(sequence=1, work=clone(family['work']), payload_bytes=2048,
                        pending=True, pending_reserve=4096)]
    value = dict(requests=rows, active_request_id=family['request_id'] if result is None else '',
                 scan=scan(family) if scanning else None, cursor=0 if scanning else None,
                 entries=entries, max_records=8, max_requests=family['profile']['max_requests'],
                 max_bytes=1 << 30, first_outcome_reserve=8192,
                 metadata=dict(last_sequence=len(entries), records=len(entries), bytes=0, pending=len(entries), reserved=0, requests=len(rows)))
    value['metadata']['bytes'], value['metadata']['reserved'] = occupancy(value, family)
    return value


def occupancy(value, family):
    payload = sum(len(canon(q)) for q in value['requests'])
    payload += sum(e['payload_bytes'] for e in value['entries'])
    reserved = sum(e['pending_reserve'] for e in value['entries'] if e['pending'])
    if value['scan'] is not None:
        payload += len(canon(value['scan'])) + 8
        if not any(e['work']['job']['id'] == family['work']['job']['id'] for e in value['entries']):
            reserved += value['first_outcome_reserve']
    if value['active_request_id']:
        q = next(q for q in value['requests'] if q['request']['request_id'] == value['active_request_id'])
        reserved += active_reserve(q, family)
    need(0 <= payload <= MAX_U64 and 0 <= reserved <= MAX_U64 and payload + reserved <= MAX_U64, 'occupancy-overflow')
    return payload, reserved


def validate_state(value, family):
    fields(value, 'requests,active_request_id,scan,cursor,entries,max_records,max_requests,max_bytes,first_outcome_reserve,metadata')
    integer(value['max_records'], 1, 4096)
    integer(value['max_requests'], 1, 8192)
    integer(value['max_bytes'], 64 << 20, 1 << 30)
    integer(value['first_outcome_reserve'], 1, OUTCOME)
    need(value['max_requests'] == family['profile']['max_requests'], 'profile-count')
    ids, jobs, pending = {}, {}, []
    for row in value['requests']:
        validate_q(row, family)
        rid = row['request']['request_id']
        need(rid not in ids, 'duplicate-request')
        ids[rid] = row
        if row['result'] is None:
            pending.append(rid)
        elif row['result']['result']['state'] == 'work':
            work = row['result']['result']['work']
            jid = work['job']['id']
            need(jid not in jobs, 'duplicate-request-job')
            jobs[jid] = work
    need(pending == ([value['active_request_id']] if value['active_request_id'] else []), 'active-null-bijection')
    if value['active_request_id']:
        need(value['scan'] is None and value['cursor'] is None, 'active-scan-conflict')
    scan_slot = 0
    if value['scan'] is None:
        need(value['cursor'] is None, 'dangling-cursor')
    else:
        fields(value['scan'], 'version,request_id')
        integer(value['scan']['version'], 1, 1)
        q = ids[value['scan']['request_id']]
        need(q['result'] is not None and q['result']['result']['state'] == 'work', 'scan-work')
        integer(value['cursor'], 0, 1 << 32)
        scan_slot = not any(e['work']['job']['id'] == q['result']['result']['job_id'] for e in value['entries'])
    seq = 0
    for entry in value['entries']:
        fields(entry, 'sequence,work,payload_bytes,pending,pending_reserve')
        seq += 1
        need(type(entry['sequence']) is int and entry['sequence'] == seq, 'entry-order')
        need(canon(jobs[entry['work']['job']['id']]) == canon(entry['work']), 'entry-work-link')
        integer(entry['payload_bytes'], 1, OUTCOME)
        need(type(entry['pending']) is bool, 'pending-boolean')
        integer(entry['pending_reserve'], 0, OUTCOME)
        need(entry['pending'] or entry['pending_reserve'] == 0, 'completed-entry-reserve')
    meta = value['metadata']
    fields(meta, 'last_sequence,records,bytes,pending,reserved,requests')
    for number in meta.values():
        integer(number, 0, MAX_U64)
    payload, reserved = occupancy(value, family)
    expected = dict(last_sequence=seq, records=seq, bytes=payload, pending=sum(e['pending'] for e in value['entries']),
                    reserved=reserved, requests=len(ids))
    need(meta == expected, 'audited-metadata')
    need(len(ids) <= value['max_requests'] and seq + len(ids) + int(bool(value['active_request_id'])) + int(scan_slot) <= value['max_records'], 'shared-record-capacity')
    need(payload + reserved <= value['max_bytes'], 'byte-capacity')


def rows(value):
    result = {('71' + q['request']['request_id']): canon(q).hex() for q in value['requests']}
    if value['scan'] is not None:
        result[b'scan'.hex()] = canon(value['scan']).hex()
        result[b'cursor'.hex()] = struct.pack('>Q', value['cursor']).hex()
    return dict(sorted(result.items()))


def produce_outbox(families, inherited):
    family = families[0]
    cases = Cases('conditional new q/scan/cursor and full counters; entry payload/reserve lengths explicitly modeled after independent entry authentication, not synthetic PoW')
    for name, result, scanning, entry in [
            ('empty', 'absent', False, False), ('durable-null-before-network', None, False, False),
            ('known-work-first-slot', 'work', True, False), ('known-work-pending-entry', 'work', True, True),
            ('expired-scan-cleared-result-retained', 'work', False, False),
            ('expired-scan-pending-entry-retained', 'work', False, True),
            ('unused-terminal', 'issued_unused', False, False), ('sealed-terminal', 'not_issued', False, False)]:
        v = state(family, result, scanning, entry)
        row = cases.add(name, v, validate_state, family)
        row['new_namespace_rows'] = rows(v)
    good = state(family, 'work', True, True)
    for name, mutate in [
            ('missing-retained-request', lambda v: v['requests'].clear()),
            ('retained-response-wrong-money', lambda v: v['requests'][0]['result'].update(settlement_config_id='ab' * 32)),
            ('wrong-scan-request', lambda v: v['scan'].update(request_id='ab' * 32)),
            ('duplicate-request', lambda v: v['requests'].append(clone(v['requests'][0]))),
            ('entry-work-mismatch', lambda v: v['entries'][0]['work']['job'].update(challenge='ab' * 32)),
            ('entry-sequence-gap', lambda v: v['entries'][0].update(sequence=2)),
            ('cursor-overflow', lambda v: v.update(cursor=(1 << 32) + 1)),
            ('cursor-negative', lambda v: v.update(cursor=-1)),
            ('cursor-bool', lambda v: v.update(cursor=True)),
            ('active-known-result', lambda v: v.update(active_request_id=family['request_id'])),
            ('missing-required-q-null', lambda v: v['requests'][0].pop('result'))]:
        cases.bad(name, good, mutate, validate_state, family)
    for name in ('last_sequence', 'records', 'bytes', 'pending', 'reserved', 'requests'):
        cases.bad('wrong-meta-' + name, good, lambda v, k=name: v['metadata'].update({k: v['metadata'][k] + 1}), validate_state, family)
        for wrong, suffix in ((True, 'bool'), (1.0, 'float')):
            cases.bad('noninteger-meta-' + name + '-' + suffix, good,
                      lambda v, k=name, x=wrong: v['metadata'].update({k: x}), validate_state, family)
    null = state(family)
    cases.bad('null-without-active', null, lambda v: v.update(active_request_id=''), validate_state, family)
    cases.bad('null-with-scan', null, lambda v: v.update(scan=scan(family), cursor=0), validate_state, family)
    cleared = state(family, 'work')
    cases.bad('cleared-scan-dangling-cursor', cleared, lambda v: v.update(cursor=0), validate_state, family)
    for end in (0, (1 << 32) - 1, 1 << 32):
        v = state(family, 'work', True)
        v['cursor'] = end
        cases.add('cursor-' + str(end), v, validate_state, family)['new_namespace_rows'] = rows(v)
    # Same original owner: old response/entry survive a distinct current issue.
    later = dict(family, request_id='ab' * 32)
    history = state(family, 'work', False, True)
    history['requests'].append(qrow(later))
    history['active_request_id'] = later['request_id']
    history['metadata']['requests'] = 2
    history['metadata']['bytes'], history['metadata']['reserved'] = occupancy(history, family)
    cases.add('older-completed-and-pending-entry-with-new-active-request', history, validate_state, family)['new_namespace_rows'] = rows(history)
    cases.bad('older-scan-cannot-coexist-with-new-active-request', history,
              lambda v: v.update(scan=scan(family), cursor=0), validate_state, family)
    cases.bad('older-entry-cannot-rebind-new-request-work', history,
              lambda v: v['entries'][0]['work']['job'].update(id='ab' * 32), validate_state, family)
    duplicate_job = state(family, 'work', False)
    duplicate_job['requests'].append(qrow(later, 'work'))
    duplicate_job['metadata']['requests'] = 2
    duplicate_job['metadata']['bytes'], duplicate_job['metadata']['reserved'] = occupancy(duplicate_job, family)
    cases.add('two-requests-cannot-claim-same-job', duplicate_job, validate_state, family, valid=False)
    qcases = Cases('ordered retained-response codec; no parsed DTO confers ownership')
    for name in (None, 'work', 'issued_unused', 'not_issued'):
        qcases.add('q-' + str(name), qrow(family, name), validate_q, family)
    for name in ('absent', 'pending'):
        qcases.add('q-cannot-retain-' + name, qrow(family, name), validate_q, family, valid=False)
    anchors = [clone(v) for v in inherited['outbox']['entry_cases']]
    for row in anchors:
        need(canon(row['value']).hex() == row['canonical_hex'], 'unchanged-entry-anchor')
    return dict(cases=cases.rows, request_rows=qcases.rows, inherited_entry_anchors=anchors,
                counter_scope='bytes are payload-only for q/e/scan/cursor; source/registry key-inclusive counts are different; exact entry/authentication behavior remains inherited')
