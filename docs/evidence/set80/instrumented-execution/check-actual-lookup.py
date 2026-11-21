#!/usr/bin/env python3
"""Supplement the immutable Set80 public checker with A-only lookup evidence.

Root alone imports/executes this module. It never reads a private fixture,
wallet, credential, node home or database. The original checker must pass first.
"""
from pathlib import Path
import hashlib
import os
import stat
import types

ORIGINAL = Path('/Users/taytrrip/slug/set80-prep/check-actual.py')
ORIGINAL_SHA256 = '87cb6364644db35b707a90a4bcf1c97467915aa3d691e65c81e3b9fc4539736e'
LOOKUP_FILE = 'receipt-lookup-evidence.json'
WIRE_FILE = 'receipt-lookup-wire.json'


def original_bytes():
    if ORIGINAL.resolve(strict=True) != ORIGINAL:
        raise ValueError('original checker path changed')
    fd = os.open(ORIGINAL, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_size != 65246:
            raise ValueError('original checker file changed')
        raw = bytearray()
        while len(raw) <= 65246:
            chunk = os.read(fd, 65247-len(raw))
            if not chunk:
                break
            raw.extend(chunk)
        after = os.fstat(fd)
        if ((before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns) !=
                (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns)
                or hashlib.sha256(raw).hexdigest() != ORIGINAL_SHA256):
            raise ValueError('original public checker source changed')
        return bytes(raw)
    finally:
        os.close(fd)


def original_checker():
    # Root binds this path and this wrapper in the new execution lock. Loading
    # the reviewed original is the sole code dependency; no product is imported.
    raw = original_bytes()
    module = types.ModuleType('set80_original_public_checker')
    module.__file__ = str(ORIGINAL)
    # Compile only these exact checked bytes, without import-cache artifacts or
    # a second source read that could select a different program.
    exec(compile(raw, str(ORIGINAL), 'exec'), module.__dict__)
    original_bytes()
    return module


def check_lookup_trace(base, trace, closed, report):
    need, keys, integer, same = base.need, base.keys, base.integer, base.same
    keys(trace, 'version outbox rows attempts wait_error completed', ordered=True)
    integer(trace['version'], 1, 1)
    need(trace['completed'] is True and trace['wait_error'] == '',
         'complete original row audit without pause failure')
    same(trace['outbox'], closed['rows']['outbox'], 'same original pre-Lookup outbox controls')
    same(trace['rows'], closed['rows']['rows'], 'same complete pre-Lookup saved rows')
    rows, accepted = closed['rows']['rows'], closed['rows']['accepted']
    winning = [r for r in rows if r['pool_win'] is True]
    attempts = trace['attempts']
    need(type(attempts) is list and 1 <= len(attempts) <= 21*len(winning),
         'finite original20s per-winning-row attempt inventory')
    groups = {}
    last_sequence = 0
    for event in attempts:
        keys(event, 'sequence started_at_ms ended_at_ms deadline_at_ms error_type client_error code outcome_unknown context_error returned', ordered=True)
        sequence = integer(event['sequence'], 1, len(rows))
        need(sequence >= last_sequence and rows[sequence-1]['pool_win'] is True,
             'ordered exact original winning-row attempts only')
        last_sequence = sequence
        integer(event['started_at_ms'], report['start_ms'], report['start_ms']+900000)
        integer(event['ended_at_ms'], event['started_at_ms'], report['start_ms']+900000)
        integer(event['deadline_at_ms'], event['started_at_ms'], report['start_ms']+900000)
        need(event['ended_at_ms'] <= event['deadline_at_ms'], 'completed lookup within original deadline')
        need(type(event['client_error']) is bool and type(event['outcome_unknown']) is bool,
             'exact client-error flags')
        need(event['context_error'] == '', 'successful complete audit retains no canceled/expired attempt')
        base.text(event['error_type'], 128)
        base.text(event['code'], 64)
        groups.setdefault(sequence, []).append(event)
    need(set(groups) == {r['sequence'] for r in winning}, 'no omitted winning-row lookup')
    successes, missing = [], []
    for row in winning:
        group = groups[row['sequence']]
        first, last = group[0], group[-1]
        deadline = first['deadline_at_ms']
        need(1 <= len(group) <= 21 and 0 < deadline-first['started_at_ms'] <= 20000,
             'one original20s child deadline, not a renewed budget')
        for index, event in enumerate(group):
            need(event['deadline_at_ms'] == deadline, 'same original per-row deadline across retries')
            if index:
                need(event['started_at_ms'] >= group[index-1]['ended_at_ms']+1000,
                     'original one-second retry pause')
            if index < len(group)-1:
                need(event['error_type'] == '*poolservice.ClientError' and event['client_error'] is True
                     and event['code'] in ('busy','rate_limited') and event['outcome_unknown'] is False
                     and event['returned'] is None, 'only exact definite busy/rate retries')
        if last['returned'] is None:
            need(last['error_type'] == '*poolservice.ClientError' and last['client_error'] is True
                 and last['code'] == 'not_found' and last['outcome_unknown'] is False
                 and row['pool_state'] == 'pending', 'unchanged pending/not-found terminal predicate')
            missing.append(row['sequence'])
        else:
            need(last['error_type'] == last['code'] == '' and last['client_error'] is False
                 and last['outcome_unknown'] is False, 'successful exact Lookup result')
            receipt = last['returned']
            need(type(receipt) is dict and receipt['job_id'] == row['work']['job']['id']
                 and receipt['nonce'] == row['nonce'] and receipt['work_hash'] == row['work_hash']
                 and receipt['duplicate_key'] == row['duplicate_key']
                 and receipt['pool'] is True and receipt['block'] is True,
                 'successful returned receipt matches the original exact work/nonce')
            integer(receipt['accepted_sequence'], 0, 2)
            if receipt['accepted_sequence']:
                successes.append(receipt)
            else:
                need(row['pool_state'] == 'pending', 'unaccepted original intent stays pending')
    same(missing, closed['rows']['lookup_missing'], 'same complete not-found pending list')
    successes.sort(key=lambda r:r['accepted_sequence'])
    same(successes, accepted, 'all successful accepted receipts are the exact source audit results')
    return attempts


EVENT_KEYS = ('ordinal started_at_ms ended_at_ms method_post origin_ok request_bytes '
              'request_sha256 request forwarded status json_content_type no_content_encoding '
              'declared_length_fits response_bytes response_sha256 body_complete response_kind '
              'receipt api_error selected_binding request_binding error_status_matches '
              'error_unknown_matches failure_phase failure_kind request_context read_error '
              'close_error downstream_attempted downstream_bytes downstream_error')
BOOL_KEYS = ('method_post origin_ok forwarded json_content_type no_content_encoding '
             'declared_length_fits body_complete selected_binding request_binding '
             'error_status_matches error_unknown_matches read_error close_error downstream_attempted')
ERROR_STATUS = {'invalid_request': 400, 'unauthorized': 401, 'forbidden': 403,
                'not_found': 404, 'expired': 409, 'stale_capture': 409,
                'rejected': 422, 'too_large': 413, 'rate_limited': 429,
                'busy': 503, 'capacity': 503, 'unavailable': 503, 'outcome_unknown': 503}


def check_wire(base, wire, attempts, closed, report):
    need, keys, integer, same = base.need, base.keys, base.integer, base.same
    keys(wire, 'version path max_events dropped events', ordered=True)
    integer(wire['version'], 1, 1)
    integer(wire['max_events'], 128, 128)
    integer(wire['dropped'], 0, 0)
    need(wire['path'] == '/pool/v3/lookup' and type(wire['events']) is list
         and 1 <= len(wire['events']) <= 128, 'complete selected receipt route inventory')
    events = wire['events']
    last_start = report['start_ms']
    for ordinal, event in enumerate(events, 1):
        keys(event, EVENT_KEYS, ordered=True)
        integer(event['ordinal'], ordinal, ordinal)
        integer(event['started_at_ms'], last_start, report['start_ms']+900000)
        last_start = event['started_at_ms']
        integer(event['ended_at_ms'], event['started_at_ms'], report['start_ms']+900000)
        for name in BOOL_KEYS.split():
            need(type(event[name]) is bool, 'exact relay diagnostic boolean')
        for name in ('request_bytes', 'response_bytes', 'downstream_bytes'):
            integer(event[name], 0, (16 << 20)+65536+1)
        integer(event['status'], 0, 599)
        for name in ('request_sha256', 'response_sha256'):
            if event[name] != '':
                base.hx(event[name])
        need(event['response_kind'] in ('none', 'invalid', 'read_error', 'oversize', 'receipt', 'error'),
             'closed relay response category')
        need(event['failure_phase'] in ('', 'origin', 'request_read', 'request_create', 'upstream', 'response_read_close')
             and event['failure_kind'] in ('', 'invalid', 'canceled', 'deadline_exceeded', 'other'),
             'closed relay failure category')
        for name in ('request_context', 'downstream_error'):
            need(event[name] in ('', 'canceled', 'deadline_exceeded', 'other'), 'closed context/write category')
        if event['request'] is not None:
            q = keys(event['request'], 'job_id nonce', ordered=True)
            base.hx(q['job_id']); integer(q['nonce'], 0, (1 << 32)-1)
            raw = base.canonical(q)
            need(event['request_bytes'] == len(raw) and event['request_sha256'] == base.sha(raw),
                 'original client canonical request bytes')
        if event['receipt'] is not None:
            keys(event['receipt'], 'version binding_id settlement_config_id recipient_script issue_profile_id submission', ordered=True)
            need(event['response_kind'] == 'receipt' and event['api_error'] is None,
                 'unambiguous receipt projection')
        if event['api_error'] is not None:
            error = keys(event['api_error'], 'version error', ordered=True)
            integer(error['version'], 3, 3)
            error = keys(error['error'], 'code outcome_unknown', ordered=True)
            need(error['code'] in ERROR_STATUS and type(error['outcome_unknown']) is bool
                 and event['response_kind'] == 'error' and event['receipt'] is None,
                 'closed public API error projection')
    # Extra events belong to the original worker's earlier read/recovery calls;
    # only the closed-outbox audit attempts below must show successful framing.
    used = set()
    for attempt in attempts:
        row = closed['rows']['rows'][attempt['sequence']-1]
        q = {'job_id': row['work']['job']['id'], 'nonce': row['nonce']}
        matches = [e for e in events if e['request'] == q
                   and attempt['started_at_ms'] <= e['started_at_ms'] <= attempt['ended_at_ms']]
        need(len(matches) == 1 and matches[0]['ordinal'] not in used,
             'one exact forwarded request for each original audit attempt')
        event = matches[0]
        used.add(event['ordinal'])
        for name in ('method_post', 'origin_ok', 'forwarded', 'json_content_type',
                     'no_content_encoding', 'declared_length_fits', 'body_complete', 'downstream_attempted'):
            need(event[name] is True, 'complete original lookup framing/delivery')
        need(event['failure_phase'] == event['failure_kind'] == event['request_context'] == event['downstream_error'] == ''
             and event['read_error'] is False and event['close_error'] is False,
             'no hidden relay transport/read/close failure in a successful audit')
        if attempt['returned'] is not None:
            envelope = event['receipt']
            need(event['status'] == 200 and event['response_kind'] == 'receipt' and envelope is not None
                 and event['api_error'] is None and event['selected_binding'] is True
                 and event['request_binding'] is True, 'successful selected receipt envelope')
            integer(envelope['version'], 3, 3)
            for name in ('binding_id', 'settlement_config_id', 'recipient_script', 'issue_profile_id'):
                need(envelope[name] == closed['request'][name], 'original selected source/money/profile')
            same(envelope['submission'], attempt['returned'], 'wire receipt equals original Client result')
        else:
            envelope = event['api_error']
            need(event['response_kind'] == 'error' and envelope is not None and event['receipt'] is None
                 and event['error_status_matches'] is True and event['error_unknown_matches'] is True,
                 'exact known server error, not local validation failure')
            need(envelope['error']['code'] == attempt['code']
                 and envelope['error']['outcome_unknown'] is attempt['outcome_unknown']
                 and event['status'] == ERROR_STATUS[attempt['code']], 'same bounded server/client refusal')
        raw = base.canonical(envelope)+b'\n'
        need(len(raw) <= 65536 and event['response_bytes'] == event['downstream_bytes'] == len(raw)
             and event['response_sha256'] == base.sha(raw), 'complete unchanged original server response bytes')


def check(owned: Path, case: str, probe_pid: int):
    """Original complete proof first; then A-only immutable lookup crosslinks."""
    base = original_checker()
    identities = base.check(owned, case, probe_pid)
    if case == 'A':
        public = base.Public(owned)
        report = public.json('actual-report.json')
        closed = public.json('07-worker-closed.json')
        trace = public.json(LOOKUP_FILE)
        wire = public.json(WIRE_FILE)
        attempts = check_lookup_trace(base, trace, closed, report)
        check_wire(base, wire, attempts, closed, report)
        for path, identity in public.finish().items():
            base.need(path not in identities or identities[path] == identity,
                      'original and supplementary evidence reads agree')
            identities[path] = identity
    # Case B retains the exact old proof and requires no new lookup trace.
    for path, identity in identities.items():
        raw = base.read_regular(Path(path), base.MAX_JSON)
        base.need({'bytes': len(raw), 'sha256': base.sha(raw)} == identity,
                  'all original and supplementary public evidence stays unchanged')
    original_bytes()
    return identities
