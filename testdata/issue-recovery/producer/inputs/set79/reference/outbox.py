"""Independent conditional DAILY outbox bytes and Issue-hold state model.

This leaf never verifies PoW, a node, a source journal, account authority, or an
actual sync. Embedded work/digests come unchanged from the locked old corpus.
"""
from core import *
from transport import worker_from, worker

APPLICATION = 'slug-pool-worker'
META_FIELDS = 'application,version,config,last_sequence,records,bytes,pending,reserved,issue_pending'
ENTRY_REQUIRED = 'version,sequence,work,nonce,work_hash,duplicate_key,pool_win,pool_state'
MUTABLE_ALLOWANCE = 96 << 10
MAX_ENTRY = 40 << 20
MAX_SCAN = (16 << 20) + (64 << 10)
MAX_META = 128 << 10


def outbox_config(c, expected):
    worker(c)
    require(canon(c) == canon(expected), 'exact complete daily configuration')


def marker(c):
    return dict(application=APPLICATION, version=2, config=clone(c))


def marker_valid(v, c):
    fields(v, 'application,version,config')
    require(v['application'] == APPLICATION)
    integer(v['version'], 2, 2)
    outbox_config(v['config'], c)
    require(len(canon(v)) <= MAX_META, 'marker size')


def scan_value(row, nonce=0):
    return dict(version=2, work=clone(row['work']), next_nonce=nonce)


def scan_valid(v, c, row):
    fields(v, 'version,work,next_nonce')
    integer(v['version'], 2, 2)
    integer(v['next_nonce'], 0, 1 << 32)
    require(canon(v['work']) == canon(row['work']), 'exact inherited work')
    require(v['work']['job']['version'] == 2, 'embedded job remains v2')
    require(v['work']['job']['public_key'] == c['public_key'])
    require(canon(c['source']) == canon(row['client']['source']))
    require(len(canon(v)) <= MAX_SCAN, 'scan size')


def pool_only_entry(row, state='pending', error=''):
    v = dict(version=2, sequence=1, work=clone(row['work']),
             nonce=row['case']['nonce'], work_hash=row['case']['work_hash'],
             duplicate_key=row['case']['submission_key'], pool_win=True,
             pool_state=state)
    if state == 'accepted':
        v['pool_receipt'] = clone(row['submission'])
    if error:
        v['last_error'] = error
    v.update(next_attempt_ms=0, retry_count=0)
    return v


def entry_valid(v, c, row):
    names = ENTRY_REQUIRED.split(',')
    if 'pool_receipt' in v:
        names.append('pool_receipt')
    if 'last_error' in v:
        names.append('last_error')
    names += ['next_attempt_ms', 'retry_count']
    fields(v, ','.join(names))
    integer(v['version'], 2, 2)
    integer(v['sequence'], 1, c['max_records'])
    require(v['sequence'] == 1 and canon(v['work']) == canon(row['work']))
    require(row['case']['qualifies_prospect'] is False and
            row['case']['qualifies_block'] is False, 'pool-only fixture')
    require(c['source']['pool_target'] == 'f' * 64)
    integer(v['nonce'], 0, (1 << 32)-1)
    require(v['pool_win'] is True and v['nonce'] == row['case']['nonce'])
    require(v['work_hash'] == row['case']['work_hash'] and
            v['duplicate_key'] == row['case']['submission_key'])
    require(v['pool_state'] in ('pending', 'accepted', 'expired', 'rejected'))
    if 'pool_receipt' in v:
        require(v['pool_state'] == 'accepted' and
                canon(v['pool_receipt']) == canon(row['submission']), 'exact conditional receipt')
        require(v['pool_receipt']['version'] == 1, 'receipt remains v1')
    else:
        require(v['pool_state'] != 'accepted', 'missing accepted receipt')
    if 'last_error' in v:
        require(v['last_error'] in ('unavailable', 'outcome_unknown', 'busy',
                'capacity', 'rate_limited', 'expired', 'rejected', 'not_found',
                'unauthorized', 'forbidden', 'invalid_request', 'too_large',
                'signer_unavailable', 'clock', 'stale'))
    integer(v['next_attempt_ms'], 0, MAX_I64)
    integer(v['retry_count'], 0, 32)
    require(len(canon(v)) <= MAX_ENTRY)


def immutable_entry(v):
    out = clone(v)
    out['pool_state'] = ''
    out.pop('pool_receipt', None)
    out.pop('last_error', None)
    out['next_attempt_ms'] = 0
    out['retry_count'] = 0
    return out


def entry_reserve(v):
    immutable = len(canon(immutable_entry(v)))
    require(immutable <= MAX_ENTRY - MUTABLE_ALLOWANCE, 'immutable size')
    budget = immutable + MUTABLE_ALLOWANCE
    require(len(canon(v)) <= budget, 'mutable budget')
    return budget - len(canon(v)) if v['pool_state'] == 'pending' else 0


def seq_key(seq):
    return b'e' + struct.pack('>Q', seq)


def pending_key(v):
    return b'p' + struct.pack('>QQ', v['next_attempt_ms'], v['sequence'])


def journal(c, row, entry=None, scan=None, issue_pending=False):
    """Model only the selected finite namespace, including exact index values."""
    rows = {}
    size = pending = reserved = 0
    if scan is not None:
        stored = clone(scan)
        stored['next_nonce'] = 0
        rows[b'scan'] = canon(stored)
        rows[b'cursor'] = struct.pack('>Q', scan['next_nonce'])
        size += len(rows[b'scan']) + 8
    if entry is not None:
        key = seq_key(entry['sequence'])
        raw = canon(entry)
        rows[key] = raw
        rows[b'h' + entry['duplicate_key'].encode('ascii')] = key
        size += len(raw)
        reserved = entry_reserve(entry)
        if entry['pool_state'] == 'pending':
            rows[pending_key(entry)] = key
            pending = 1
    count = int(entry is not None)
    meta = dict(application=APPLICATION, version=2, config=clone(c),
                last_sequence=count, records=count, bytes=size, pending=pending,
                reserved=reserved, issue_pending=issue_pending)
    rows[b'meta'] = canon(meta)
    return dict(marker=marker(c), rows=[dict(key_hex=k.hex(), value_hex=v.hex())
                                     for k, v in sorted(rows.items())])


def journal_valid(v, c, row):
    fields(v, 'marker,rows')
    marker_valid(v['marker'], c)
    require(type(v['rows']) is list and len(v['rows']) <= 6)
    rows = {}
    previous = None
    for item in v['rows']:
        fields(item, 'key_hex,value_hex')
        key, raw = bytes.fromhex(item['key_hex']), bytes.fromhex(item['value_hex'])
        require(key.hex() == item['key_hex'] and raw.hex() == item['value_hex'])
        require(key and (previous is None or key > previous), 'unique ordered keys')
        rows[key] = raw
        previous = key
    m = parse(rows[b'meta'], MAX_META)
    fields(m, META_FIELDS)
    require(m['application'] == APPLICATION)
    integer(m['version'], 2, 2)
    outbox_config(m['config'], c)
    require(type(m['issue_pending']) is bool, 'required bool')
    for name in ('last_sequence', 'records', 'bytes', 'pending', 'reserved'):
        integer(m[name], 0, MAX_U64)
    require(m['last_sequence'] == m['records'] <= c['max_records'])
    require(m['pending'] <= m['records'] and m['bytes'] <= c['max_bytes'] and
            m['reserved'] <= c['max_bytes'] - m['bytes'])
    entry = None
    if seq_key(1) in rows:
        entry = parse(rows[seq_key(1)], MAX_ENTRY)
        entry_valid(entry, c, row)
    scan = None
    require((b'scan' in rows) == (b'cursor' in rows), 'scan/cursor pair')
    if b'scan' in rows:
        require(not m['issue_pending'], 'hold excludes scan')
        scan = parse(rows[b'scan'], MAX_SCAN)
        require(scan['next_nonce'] == 0, 'stored cursor separation')
        require(len(rows[b'cursor']) == 8)
        scan['next_nonce'] = struct.unpack('>Q', rows[b'cursor'])[0]
        scan_valid(scan, c, row)
    # Exact reconstruction validates the complete selected namespace and counts;
    # malformed/extra indexes cannot hide behind a matching metadata total.
    expected = journal(c, row, entry, scan, m['issue_pending'])
    require(v == expected, 'full conditional namespace/counter census')


def change_meta(v, mutate):
    item = next(x for x in v['rows'] if x['key_hex'] == b'meta'.hex())
    meta = parse(bytes.fromhex(item['value_hex']), MAX_META)
    mutate(meta)
    item['value_hex'] = canon(meta).hex()


def issue_model(c, row, initial_pending, events):
    """Finite abstract events, not simulated HTTP, threads, storage or authority."""
    held = active = dispatched = poisoned = False
    scan = None
    entry = pool_only_entry(row) if initial_pending else None
    calls = blocked = recoveries = 0
    for event in events:
        if event == 'arm':
            require(not held and not active and not poisoned and scan is None)
            held, active, dispatched = True, True, False
        elif event == 'dispatch':
            if active and held and not dispatched and not poisoned:
                calls += 1
                dispatched = True
            else:
                blocked += 1
        elif event in ('busy', 'rate_limited'):
            require(active and dispatched and held and not poisoned)
            held = active = dispatched = False
        elif event in ('success', 'expired_success'):
            require(active and dispatched and held and not poisoned)
            scan = scan_value(row)
            held = active = dispatched = False
        elif event == 'response_received_before_commit':
            require(active and dispatched and held and not poisoned)
            # A validated response in lost process memory changes no durable
            # state until the atomic known-scan/clear batch commits.
        elif event == 'retire_known_scan':
            require(scan is not None and not held and not poisoned)
            scan = None
        elif event in ('unknown', 'timeout', 'cancel', 'malformed_success',
                       'wrong_money', 'other_error', 'unknown_busy', 'wrapped_busy'):
            require(held)
            active = dispatched = False
        elif event in ('crash', 'stop', 'reopen'):
            active = dispatched = False
            if event == 'reopen':
                poisoned = False
        elif event in ('foreign_attempt_clear', 'stale_attempt_clear'):
            require(held)
            # An invalid attempt cannot change the durable bool, even if its
            # error code and equal-looking configuration match the old one.
        elif event in ('premark_fail_before_sync', 'premark_fail_after_sync'):
            require(not held and not active and scan is None)
            held = event.endswith('after_sync')
            poisoned = True
        elif event in ('scan_fail_before_sync', 'scan_fail_after_sync'):
            require(active and dispatched and held and not poisoned)
            if event.endswith('after_sync'):
                scan, held = scan_value(row), False
            active = dispatched = False
            poisoned = True
        elif event in ('busy_clear_fail_before_sync', 'busy_clear_fail_after_sync'):
            require(active and dispatched and held and not poisoned)
            if event.endswith('after_sync'):
                held = False
            active = dispatched = False
            poisoned = True
        elif event == 'recover_pending':
            require(entry is not None and entry['pool_state'] == 'pending' and not poisoned)
            before = immutable_entry(entry)
            entry = pool_only_entry(row, 'accepted')
            require(before == immutable_entry(entry))
            recoveries += 1
        else:
            raise ValueError('unknown model event')
    state = journal(c, row, entry, scan, held)
    journal_valid(state, c, row)
    return dict(issue_pending=held, scan_present=scan is not None,
                pool_pending=entry is not None and entry['pool_state'] == 'pending',
                outbound_issue_calls=calls, blocked_issue_calls=blocked,
                recovery_calls=recoveries, poisoned=poisoned, active_attempt=active), state


def produce_outbox(work_cases):
    require(type(work_cases) is list and len(work_cases) == 3, 'three original works')
    only = [x for x in work_cases if x['source']['name'] == 'modeled-all-three-miss']
    require(len(only) == 1)
    row = only[0]
    require(row['case']['qualifies_prospect'] is False and
            row['case']['qualifies_block'] is False)
    c = worker_from(row['client'])
    c['max_bytes'] = 128 << 20  # Both +/-1 reopen tests remain within pure bounds.
    worker(c)
    configs, markers, scans, entries, journals = [Cases() for _ in range(5)]
    configs.add('daily-exact', c, lambda v: outbox_config(v, c))
    for name, key, value in [('v1', 'version', 1),
            ('records-decreased', 'max_records', 63), ('records-increased', 'max_records', 65),
            ('bytes-decreased', 'max_bytes', (128 << 20)-1),
            ('bytes-increased', 'max_bytes', (128 << 20)+1),
            ('foreign-trust', 'transport_trust_id', 'ed'*32)]:
        configs.bad(name, c, lambda v: outbox_config(v, c),
                    lambda v, k=key, x=value: v.update({k:x}))
    mk = marker(c)
    markers.add('daily-marker', mk, lambda v: marker_valid(v, c), filename='SLUG-POOL-WORKER.json')
    markers.bad('v1-marker', mk, lambda v: marker_valid(v, c), lambda v:v.update(version=1))
    markers.bad('changed-marker-limit', mk, lambda v: marker_valid(v, c),
                lambda v:v['config'].update(max_records=65))
    for original in work_cases:
        wc = worker_from(original['client'])
        for nonce in (0, 1, 1 << 32):
            sc = scan_value(original, nonce)
            pred = lambda v, wc=wc, r=original: scan_valid(v, wc, r)
            scans.add(original['source']['name']+'-cursor-'+str(nonce), sc, pred,
                      scope='conditional scan; unchanged inherited job2/template',
                      stored_scan_hex=canon(scan_value(original)).hex(),
                      cursor_hex=struct.pack('>Q', nonce).hex())
        sc = scan_value(original)
        scans.bad(original['source']['name']+'-v1-scan', sc, pred, lambda v:v.update(version=1))
        scans.bad(original['source']['name']+'-cursor-over', sc, pred, lambda v:v.update(next_nonce=(1<<32)+1))
        scans.bad(original['source']['name']+'-job-version3', sc, pred, lambda v:v['work']['job'].update(version=3))
    for state in ('pending', 'accepted', 'expired', 'rejected'):
        e = pool_only_entry(row, state)
        entries.add('pool-only-'+state, e, lambda v:entry_valid(v, c, row),
                    scope='conditional retained digest/status; no PoW or canonicality claim',
                    immutable_hex=canon(immutable_entry(e)).hex(), reserve_bytes=entry_reserve(e))
    e = pool_only_entry(row)
    entries.bad('wrong-entry-mode', e, lambda v:entry_valid(v,c,row),lambda v:v.update(version=1))
    entries.bad('unexpected-fighter',e,lambda v:entry_valid(v,c,row),lambda v:v.update(fighter=None))
    entries.bad('accepted-without-receipt',e,lambda v:entry_valid(v,c,row),lambda v:v.update(pool_state='accepted'))
    acc=pool_only_entry(row,'accepted')
    entries.bad('receipt-must-remain-v1',acc,lambda v:entry_valid(v,c,row),lambda v:v['pool_receipt'].update(version=2))
    positives = [('empty',None,None,False),('known-scan',None,scan_value(row,1),False),
                 ('pending-entry',e,None,False),('pending-plus-scan',e,scan_value(row,1),False),
                 ('held-empty',None,None,True),('held-pending',e,None,True),
                 ('held-accepted',acc,None,True)]
    pred=lambda v:journal_valid(v,c,row)
    for name,en,sc,held in positives:
        journals.add(name,journal(c,row,en,sc,held),pred,
                     scope='conditional exact logical bytes/indexes; no actual DB/sync claim')
    held=journal(c,row,e,None,True)
    for name,mut in [('missing-bool',lambda m:m.pop('issue_pending')),
                     ('null-bool',lambda m:m.update(issue_pending=None)),
                     ('numeric-bool',lambda m:m.update(issue_pending=1)),
                     ('string-bool',lambda m:m.update(issue_pending='true')),
                     ('v1-meta',lambda m:m.update(version=1)),
                     ('wrong-record-count',lambda m:m.update(records=0)),
                     ('wrong-byte-count',lambda m:m.update(bytes=m['bytes']+1)),
                     ('wrong-reserve-count',lambda m:m.update(reserved=m['reserved']+1))]:
        journals.bad(name,held,pred,lambda v,mut=mut:change_meta(v,mut))
    journals.add('hold-with-scan',journal(c,row,e,scan_value(row),True),pred,False)
    known=journal(c,row,None,scan_value(row),False)
    journals.bad('orphan-scan',known,pred,lambda v:v['rows'].__setitem__(slice(None),[x for x in v['rows'] if x['key_hex']!=b'cursor'.hex()]))
    journals.bad('missing-hash-index',held,pred,lambda v:v['rows'].__setitem__(slice(None),[x for x in v['rows'] if not x['key_hex'].startswith('68')]))
    journals.bad('missing-pending-index',held,pred,lambda v:v['rows'].__setitem__(slice(None),[x for x in v['rows'] if not x['key_hex'].startswith('70')]))
    journals.bad('unknown-namespace',held,pred,lambda v:v['rows'].append(dict(key_hex=b'z'.hex(),value_hex='00')))
    transitions=[]
    def transition(name, events, expected, pending=False):
        summary, stored=issue_model(c,row,pending,events)
        require(summary==expected,'transition expectation: '+name)
        transitions.append(dict(name=name,scope='conditional original-owner state model; no runtime or authority',
                                initial_pending=pending,events=events,expected=summary,stored=stored))
    def result(held=True, scan=False, pending=False, calls=1, blocked=0,
               recoveries=0, poisoned=False, active=False):
        return dict(issue_pending=held,scan_present=scan,pool_pending=pending,
                    outbound_issue_calls=calls,blocked_issue_calls=blocked,
                    recovery_calls=recoveries,poisoned=poisoned,active_attempt=active)
    for event in ('unknown','timeout','cancel','malformed_success','wrong_money',
                  'other_error','unknown_busy','wrapped_busy'):
        transition(event+'-reopen-holds',['arm','dispatch',event,'stop','reopen','dispatch'],result(blocked=1))
    transition('crash-before-dispatch',['arm','crash','reopen','dispatch'],result(calls=0,blocked=1))
    transition('crash-after-dispatch',['arm','dispatch','crash','reopen','dispatch'],result(blocked=1))
    transition('crash-after-response-before-commit',['arm','dispatch','response_received_before_commit','crash','reopen','dispatch'],result(blocked=1))
    for event in ('busy','rate_limited'):
        transition('same-attempt-'+event,['arm','dispatch',event],result(held=False))
        transition(event+'-permits-new-definite-attempt',['arm','dispatch',event,'arm','dispatch','success'],result(held=False,scan=True,calls=2))
    transition('validated-scan-atomic',['arm','dispatch','success'],result(held=False,scan=True))
    transition('expired-known-work-is-saved',['arm','dispatch','expired_success'],result(held=False,scan=True))
    transition('foreign-attempt-cannot-clear',['arm','dispatch','unknown','foreign_attempt_clear','dispatch'],result(blocked=1))
    transition('reopen-attempt-cannot-clear',['arm','dispatch','crash','reopen','stale_attempt_clear','dispatch'],result(blocked=1))
    transition('held-pending-recovers',['arm','dispatch','unknown','reopen','recover_pending','dispatch'],result(blocked=1,recoveries=1),pending=True)
    for suffix in ('before_sync','after_sync'):
        after=suffix=='after_sync'
        transition('premark-'+suffix,['premark_fail_'+suffix,'dispatch'],result(held=after,calls=0,blocked=1,poisoned=True))
        transition('scan-commit-'+suffix,['arm','dispatch','scan_fail_'+suffix,'reopen'],result(held=not after,scan=after))
        transition('busy-clear-'+suffix,['arm','dispatch','busy_clear_fail_'+suffix,'dispatch'],result(held=not after,blocked=1,poisoned=True))
    return dict(scope='independent conditional byte/state model; no node, PoW, source acceptance, signing or real sync authority',
                constants=dict(application=APPLICATION,mutable_allowance=MUTABLE_ALLOWANCE,
                               max_entry=MAX_ENTRY,max_scan=MAX_SCAN,max_metadata=MAX_META,
                               byte_accounting='entry payloads + stored scan payload + eight-byte cursor; excludes keys/index/meta'),
                fixture_name=row['source']['name'],config_cases=configs.rows,marker_cases=markers.rows,
                scan_cases=scans.rows,entry_cases=entries.rows,journal_cases=journals.rows,
                issue_transitions=transitions)
