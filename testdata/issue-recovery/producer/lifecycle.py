"""Finite requirements traces, not simulations of fsync, leases or scheduling."""
from core import canon, clone, need, sha
from states import make_journal, response_for, validate_journal


def projection(model, family):
    journal = make_journal(family, model['source'], model['issuer'], model['reason'])
    validate_journal(journal, family)
    response = None
    if model['local'] == 'known':
        response = response_for(family, model['source'], model['reason'])
    return dict(source=model['source'], issuer=model['issuer'], local=model['local'],
                active_request_id=family['request_id'] if model['local'] == 'null' else '',
                counts=journal['counts'], exact_job_id=family['work']['job']['id'] if model['issuer'] == 'issued' else '',
                job_hex=canon(family['work']['job']).hex() if model['issuer'] == 'issued' else '',
                source_work_hex=canon(family['work']).hex() if model['source'] == 'work' else '',
                retained_response_hex='' if response is None else canon(response).hex(),
                scan=model['scan'], nonce_cursor=0 if model['scan'] else None,
                poisoned=model['poisoned'], running=model['running'], authenticated=model['authenticated'],
                epoch=model['epoch'], run_generation=model['run_generation'], attempt_generation=model['attempt_generation'],
                network_attempts=model['network_attempts'], entropy_samples=model['entropy_samples'])


def trace(name, events, family, expected, scope='conditional state/fault boundary requirements; no measured durability, concurrency, mutex, clock or live session authority'):
    m = dict(source='absent', issuer='absent', local='absent', reason='parent_changed', scan=False,
             poisoned=False, running=True, authenticated=True, epoch=1, attempt_epoch=1,
             run_generation=1, attempt_generation=0, attempt_run=0, network_attempts=0, entropy_samples=0)
    output = []
    for event in events:
        before = projection(m, family)
        effect = 'none'
        if event == 'local-intent':
            need(m['local'] == 'absent' and m['running'] and m['authenticated'] and not m['poisoned'], 'local-intent-admission')
            m['local'] = 'null'
            effect = 'sync-local-null-before-dispatch'
        elif event == 'resume':
            need(m['local'] == 'null' and m['running'] and m['authenticated'] and not m['poisoned'], 'resume-admission')
            m['attempt_epoch'] = m['epoch']
            m['attempt_generation'] += 1
            m['attempt_run'] = m['run_generation']
            m['network_attempts'] += 1
        elif event == 'source-intent':
            need(m['local'] == 'null' and m['source'] == 'absent' and m['running'] and m['authenticated'] and not m['poisoned'], 'source-admission')
            m['source'] = 'pending'
            effect = 'sync-exact-template-intent-and-reserve'
        elif event == 'issuer-commit':
            need(m['source'] == 'pending' and m['running'] and m['authenticated'] and not m['poisoned'], 'issuer-admission')
            if m['issuer'] == 'sealed':
                effect = 'return-original-seal-no-entropy'
            elif m['issuer'] == 'issued':
                effect = 'return-original-job-no-entropy'
            else:
                m['issuer'] = 'issued'
                m['entropy_samples'] += 1
                effect = 'atomic-request-job-reverse-clock'
        elif event in ('seal-parent', 'seal-drain'):
            need(m['source'] == 'pending' and m['running'] and m['authenticated'] and not m['poisoned'], 'seal-admission')
            if m['issuer'] == 'issued':
                effect = 'original-issued-job-wins'
            elif m['issuer'] == 'sealed':
                effect = 'original-seal-reason-wins'
            else:
                m['issuer'] = 'sealed'
                m['reason'] = 'parent_changed' if event == 'seal-parent' else 'intake_drained'
                effect = 'atomic-positive-evidence-seal'
        elif event in ('source-work', 'source-unused', 'source-negative'):
            need(m['source'] == 'pending' and m['running'] and m['authenticated'] and not m['poisoned'], 'source-finalize-admission')
            if event == 'source-negative':
                need(m['issuer'] == 'sealed', 'negative-needs-seal')
                m['source'] = 'not_issued'
            else:
                need(m['issuer'] == 'issued', 'finalize-needs-job')
                m['source'] = 'work' if event == 'source-work' else 'issued_unused'
            effect = 'atomic-source-final-request-and-owned-evidence'
        elif event == 'local-result':
            need(m['local'] == 'null' and m['source'] in ('work', 'issued_unused', 'not_issued')
                 and m['running'] and m['authenticated'] and not m['poisoned'] and m['attempt_epoch'] == m['epoch']
                 and m['attempt_run'] == m['run_generation'] and m['attempt_generation'] > 0, 'original-attempt-publication')
            m['local'] = 'known'
            m['scan'] = m['source'] == 'work'
            effect = 'atomic-retained-response-active-clear-scan-cursor'
        elif event == 'duplicate-result':
            need(m['local'] == 'known' and m['running'] and m['authenticated'] and not m['poisoned'], 'duplicate-result-state')
            effect = 'exact-existing-response-no-new-row'
        elif event == 'expired-completed-replay':
            need(m['source'] == 'work' and m['issuer'] == 'issued' and m['running'] and m['authenticated']
                 and not m['poisoned'] and m['attempt_epoch'] == m['epoch'] and m['attempt_run'] == m['run_generation'], 'completed-precedes-expiry')
            effect = 'exact-original-work-no-expiry-reset'
        elif event == 'remove-expired-scan':
            need(m['local'] == 'known' and m['scan'], 'scan-removal')
            m['scan'] = False
            effect = 'remove-only-scan-and-cursor-unused-first-slot'
        elif event == 'unknown-write':
            m['poisoned'] = True
            effect = 'retain-original-owner-poison-no-in-place-retry'
        elif event in ('stop', 'cancel', 'revoke', 'deadline'):
            m['running'] = False
            m['run_generation'] += 1
            if event == 'revoke':
                m['authenticated'] = False
            effect = 'join-original-attempt-no-disclosure'
        elif event == 'reopen':
            m['poisoned'] = False
            m['running'] = False
            effect = 'read-audit-only-no-resume'
        elif event == 'reauth-same-gym':
            need(not m['running'], 'no-token-replacement-in-attempt')
            m['epoch'] += 1
            m['authenticated'] = True
            effect = 'new-legitimate-original-gym-session-after-join'
        elif event == 'start':
            need(not m['running'] and not m['poisoned'] and m['authenticated'], 'explicit-start')
            m['running'] = True
        elif event in ('busy', 'rate-limited', 'node-error', 'unknown-response', 'malformed-response',
                       'wrong-money', 'lookup-absent', 'lookup-pending', 'snapshot', 'tick-stopped'):
            if event == 'tick-stopped':
                need(not m['running'], 'stopped')
            effect = 'retain-exact-request-no-terminalization'
        elif event == 'old-epoch-result':
            need(m['attempt_epoch'] != m['epoch'] or m['attempt_run'] != m['run_generation'] or not m['running'] or not m['authenticated'], 'stale-attempt-required')
            effect = 'refuse-stale-attempt-no-disclosure'
        else:
            raise ValueError('unknown-trace-event:' + event)
        after = projection(m, family)
        output.append(dict(event=event, effect=effect, before=before, after=after))
    final = projection(m, family)
    for key, value in expected.items():
        need(final[key] == value, 'trace-final:' + name + ':' + key)
    # This is an independently fixed opaque equality obligation; no transaction
    # or financial amount is synthesized by issuance recovery.
    sentinel = b'original-reserved-batch-A-public-invariance-obligation'
    return dict(name=name, scope=scope, events=output, expected=final,
                reserved_batch_before_sha256=sha(sentinel), reserved_batch_after_sha256=sha(sentinel),
                financial_scope='opaque equality requirement only; real original A bytes, D80 credit and D84 are runtime acceptance gates')


def produce_lifecycle(families):
    f = families[0]
    prefix = ['local-intent', 'resume', 'source-intent']
    cases = []
    cases.append(trace('happy-path-duplicate-local-result', prefix + ['issuer-commit', 'source-work', 'local-result', 'duplicate-result'], f,
                       dict(source='work', issuer='issued', local='known', entropy_samples=1, network_attempts=1)))
    # The two sides of each uncertain sync are explicit modeled durable states.
    fault_prefixes = [
        ('before-source-sync', ['local-intent', 'resume'], ['source-intent', 'issuer-commit', 'source-work']),
        ('after-source-sync', prefix, ['issuer-commit', 'source-work']),
        ('after-issuer-sync', prefix + ['issuer-commit'], ['issuer-commit', 'source-work']),
        ('after-work-sync', prefix + ['issuer-commit', 'source-work'], ['expired-completed-replay']),
    ]
    for name, before, suffix in fault_prefixes:
        cases.append(trace(name + '-poison-joined-reopen', before + ['unknown-write', 'stop', 'reopen', 'snapshot', 'tick-stopped', 'start', 'resume'] + suffix + ['local-result'], f,
                           dict(source='work', local='known', entropy_samples=1, network_attempts=2)))
    cases.append(trace('local-result-sync-committed-unknown', prefix + ['issuer-commit', 'source-work', 'local-result', 'unknown-write', 'stop', 'reopen', 'snapshot', 'start', 'duplicate-result'], f,
                       dict(local='known', scan=True, network_attempts=1)))
    cases.append(trace('local-result-sync-not-committed-unknown', prefix + ['issuer-commit', 'source-work', 'unknown-write', 'stop', 'reopen', 'start', 'resume', 'expired-completed-replay', 'local-result'], f,
                       dict(local='known', scan=True, network_attempts=2, entropy_samples=1)))
    cases.append(trace('expired-never-published-issued-unused', prefix + ['issuer-commit', 'unknown-response', 'stop', 'reopen', 'start', 'resume', 'issuer-commit', 'source-unused', 'local-result'], f,
                       dict(source='issued_unused', scan=False, entropy_samples=1)))
    for evidence in ('parent', 'drain'):
        cases.append(trace('seal-' + evidence + '-gap-delayed-commit', prefix + ['seal-' + evidence, 'unknown-write', 'stop', 'reopen', 'start', 'resume', 'issuer-commit', 'source-negative', 'local-result'], f,
                           dict(source='not_issued', issuer='sealed', entropy_samples=0)))
    repeated = trace('original-seal-reason-survives-different-later-evidence', prefix + ['seal-parent', 'seal-drain', 'source-negative', 'local-result'], f,
                     dict(source='not_issued', issuer='sealed', entropy_samples=0))
    need(repeated['expected']['retained_response_hex'] == canon(response_for(f, 'not_issued', 'parent_changed')).hex(), 'first-seal-terminal-unchanged')
    cases.append(repeated)
    cases.append(trace('issued-job-wins-over-negative-seal', prefix + ['issuer-commit', 'seal-parent', 'source-work', 'local-result'], f,
                       dict(source='work', issuer='issued', entropy_samples=1)))
    cases.append(trace('all-errors-and-lookups-retain-null-request', ['local-intent', 'resume', 'busy', 'rate-limited', 'node-error', 'unknown-response', 'malformed-response', 'wrong-money', 'lookup-absent', 'lookup-pending', 'stop', 'reopen', 'snapshot', 'tick-stopped'], f,
                       dict(local='null', source='absent', network_attempts=1, running=False)))
    for event in ('stop', 'cancel', 'revoke', 'deadline'):
        cases.append(trace(event + '-then-new-original-gym-session', prefix + ['issuer-commit', event, 'old-epoch-result', 'reopen', 'reauth-same-gym', 'old-epoch-result', 'start', 'resume', 'issuer-commit', 'source-work', 'local-result'], f,
                           dict(source='work', local='known', entropy_samples=1, epoch=2)))
    cases.append(trace('same-auth-epoch-old-run-result-refused', prefix + ['issuer-commit', 'source-work', 'stop', 'start', 'old-epoch-result', 'resume', 'expired-completed-replay', 'local-result'], f,
                       dict(source='work', local='known', entropy_samples=1, epoch=1, attempt_generation=2)))
    cases.append(trace('expired-scan-removal-retains-request-history', prefix + ['issuer-commit', 'source-work', 'local-result', 'remove-expired-scan', 'stop', 'reopen', 'snapshot'], f,
                       dict(local='known', source='work', scan=False, running=False)))
    return dict(traces=cases,
                concurrency_scope='A later/delayed modeled event is serialized by the assumed original owner; these cases do not measure race freedom or prove atomic storage.',
                authority_refusals=[dict(name=name, allowed=False, effect='none') for name in (
                    'read-only-open-resume', 'legacy-direct-issuer-call', 'legacy-staged-prepare-commit',
                    'legacy-pool-issue', 'legacy-server-borrows-selected-pool', 'legacy-worker-borrows-selected-outbox',
                    'same-request-other-gym', 'same-request-different-intent', 'stale-owner-after-close',
                    'token-replacement-in-active-attempt', 'unknown-write-in-place-retry', 'seal-to-later-job')])
