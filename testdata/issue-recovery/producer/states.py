"""Conditional cross-owner state model; it does not emulate storage or authority."""
from core import (Cases, MAX_ROW, MAX_TIME, MAX_U64, canon, clone, fields, gym_id, hexid, ident,
                  integer, manifest_preimage, need, profile_id, public_key, request_key, sha)

INTENT_FIELDS = 'version,profile_id,request_id,gym_id,public_key,template_sha256'
SOURCE_FIELDS = 'version,intent,intent_id,state,job_id,resolved_at_ms,reason'
ISSUER_FIELDS = 'version,profile_id,request_id,intent_id,gym_id,public_key,state,job_id,sealed_at_ms,reason'
LEGAL = {('absent', 'absent'), ('pending', 'absent'), ('pending', 'issued'), ('pending', 'sealed'),
         ('work', 'issued'), ('issued_unused', 'issued'), ('not_issued', 'sealed')}


def intent(family, request_id=None):
    job = family['work']['job']
    return dict(version=1, profile_id=family['profile_id'], request_id=request_id or family['request_id'],
                gym_id=job['gym_id'], public_key=job['public_key'], template_sha256=sha(canon(family['work']['template'])))


def intent_id(value):
    return ident('SLUG pool issue recovery intent', value)


def source_row(family, state, reason='parent_changed'):
    i = intent(family)
    job = family['work']['job']
    job_id = job['id'] if state in ('work', 'issued_unused') else ''
    when = 0 if state == 'pending' else job['expires_at_ms'] if state == 'issued_unused' else job['issued_at_ms'] + 1
    why = 'expired_before_work' if state == 'issued_unused' else reason if state == 'not_issued' else ''
    return dict(version=1, intent=i, intent_id=intent_id(i), state=state, job_id=job_id, resolved_at_ms=when, reason=why)


def issuer_row(family, state, reason='parent_changed'):
    i = intent(family)
    return dict(version=1, profile_id=i['profile_id'], request_id=i['request_id'], intent_id=intent_id(i),
                gym_id=i['gym_id'], public_key=i['public_key'], state=state,
                job_id=family['work']['job']['id'] if state == 'issued' else '',
                sealed_at_ms=0 if state == 'issued' else family['work']['job']['issued_at_ms'],
                reason='' if state == 'issued' else reason)


def make_journal(family, source_state, issuer_state, reason='parent_changed'):
    job = family['work']['job']
    rid = family['request_id']
    source = [] if source_state == 'absent' else [source_row(family, source_state, reason)]
    issuer = [] if issuer_state == 'absent' else [issuer_row(family, issuer_state, reason)]
    return dict(profile=clone(family['profile']), source_requests=source,
                source_templates={rid: clone(family['work']['template'])} if source_state not in ('absent', 'work') else {},
                source_job_index={job['id']: rid} if source_state in ('work', 'issued_unused') else {},
                works={job['id']: clone(family['work'])} if source_state == 'work' else {},
                issuer_requests=issuer, issuer_job_index={job['id']: rid} if issuer_state == 'issued' else {},
                jobs={job['id']: clone(job)} if issuer_state == 'issued' else {},
                counts=dict(source_requests=len(source), registry_requests=len(issuer), jobs=int(issuer_state == 'issued'), works=int(source_state == 'work')),
                source_clock_ms=job['expires_at_ms'], issuer_clock_ms=job['expires_at_ms'])


def validate_intent(value, family):
    fields(value, INTENT_FIELDS)
    integer(value['version'], 1, 1)
    for key in ('profile_id', 'request_id', 'gym_id', 'template_sha256'):
        hexid(value[key])
    public_key(value['public_key'])
    need(value['profile_id'] == family['profile_id'], 'profile')
    need(value['gym_id'] == gym_id(family['setup']['pool']['policy'], value['public_key']), 'gym-key')
    need(any(e['gym_id'] == value['gym_id'] and e['public_key'] == value['public_key']
             for e in family['setup']['miners']['enrollments']), 'retained-membership')


def validate_journal(value, family):
    fields(value, 'profile,source_requests,source_templates,source_job_index,works,issuer_requests,issuer_job_index,jobs,counts,source_clock_ms,issuer_clock_ms')
    need(canon(value['profile']) == canon(family['profile']), 'profile-selection')
    for k in ('source_clock_ms', 'issuer_clock_ms'):
        integer(value[k], 0, MAX_TIME)
    sr, ir = {}, {}
    for row in value['source_requests']:
        fields(row, SOURCE_FIELDS)
        integer(row['version'], 1, 1)
        need(len(canon(row)) <= MAX_ROW, 'source-row-size')
        validate_intent(row['intent'], family)
        rid = row['intent']['request_id']
        need(rid not in sr and row['intent_id'] == intent_id(row['intent']), 'source-intent-identity')
        integer(row['resolved_at_ms'], 0, value['source_clock_ms'])
        need(row['state'] in ('pending', 'work', 'issued_unused', 'not_issued'), 'source-state')
        sr[rid] = row
    expected_issuer_reverse = {}
    for row in value['issuer_requests']:
        fields(row, ISSUER_FIELDS)
        integer(row['version'], 1, 1)
        need(len(canon(row)) <= MAX_ROW, 'issuer-row-size')
        for key in ('profile_id', 'request_id', 'intent_id', 'gym_id'):
            hexid(row[key])
        public_key(row['public_key'])
        need(row['profile_id'] == family['profile_id'], 'issuer-profile')
        need(row['gym_id'] == gym_id(family['setup']['pool']['policy'], row['public_key']), 'issuer-gym')
        rid = row['request_id']
        need(rid not in ir, 'duplicate-issuer-request')
        integer(row['sealed_at_ms'], 0, value['issuer_clock_ms'])
        if row['state'] == 'issued':
            hexid(row['job_id'])
            need(row['sealed_at_ms'] == 0 and row['reason'] == '', 'issued-fields')
            job = value['jobs'][row['job_id']]
            need(job['id'] == row['job_id'] and sha(manifest_preimage(job)) == job['id'], 'job-id')
            need(job['public_key'] == row['public_key'] and job['gym_id'] == row['gym_id'], 'job-owner')
            pin = family['setup']['pool']['authority_pin']
            need(job['authority_id'] == pin['authority_id'] and job['descriptor_id'] == pin['descriptor_id'], 'job-pin')
            need(job['expires_at_ms'] - job['issued_at_ms'] == 300000 and 0 < job['issued_at_ms'] <= value['issuer_clock_ms'], 'job-clock')
            need(row['job_id'] not in expected_issuer_reverse, 'duplicate-issued-job')
            expected_issuer_reverse[row['job_id']] = rid
        else:
            need(row['state'] == 'sealed' and row['job_id'] == '' and row['sealed_at_ms'] > 0
                 and row['reason'] in ('parent_changed', 'intake_drained'), 'sealed-fields')
        ir[rid] = row
    need(value['issuer_job_index'] == expected_issuer_reverse, 'issuer-reverse')
    need(set(value['jobs']) == set(expected_issuer_reverse), 'issuer-job-coverage')
    counts = value['counts']
    fields(counts, 'source_requests,registry_requests,jobs,works')
    for count in counts.values():
        integer(count, 0, MAX_U64)
    expected = dict(source_requests=len(sr), registry_requests=len(ir), jobs=len(value['jobs']), works=len(value['works']))
    need(counts == expected, 'stored-counters')
    need(0 <= counts['works'] <= counts['jobs'] <= counts['registry_requests'] <= counts['source_requests'] <= family['profile']['max_requests'], 'count-order')
    templates, reverse, works = {}, {}, {}
    matched, matched_issued = 0, 0
    for rid, source in sr.items():
        issuer = ir.get(rid)
        state = source['state']
        need((state, issuer['state'] if issuer else 'absent') in LEGAL, 'state-pair')
        if issuer:
            matched += 1
            matched_issued += issuer['state'] == 'issued'
            need(issuer['intent_id'] == source['intent_id'] and issuer['gym_id'] == source['intent']['gym_id']
                 and issuer['public_key'] == source['intent']['public_key'], 'cross-owner-intent')
        if state == 'pending':
            need(source['job_id'] == source['reason'] == '' and source['resolved_at_ms'] == 0, 'pending-fields')
        elif state == 'not_issued':
            need(source['job_id'] == '' and source['reason'] == issuer['reason']
                 and source['resolved_at_ms'] >= issuer['sealed_at_ms'], 'terminal-seal')
        else:
            job = value['jobs'][issuer['job_id']]
            need(source['job_id'] == job['id'], 'source-job')
            reverse[job['id']] = rid
            if state == 'work':
                need(source['reason'] == '' and job['issued_at_ms'] <= source['resolved_at_ms'] < job['expires_at_ms'], 'work-clock')
                work = value['works'][job['id']]
                need(work['job'] == job, 'work-job')
                works[job['id']] = work
            else:
                need(source['reason'] == 'expired_before_work' and source['resolved_at_ms'] >= job['expires_at_ms'], 'unused-clock')
        if state == 'work':
            template = works[source['job_id']]['template']
        else:
            template = value['source_templates'][rid]
            templates[rid] = template
        need(sha(canon(template)) == source['intent']['template_sha256'], 'original-template')
        if issuer and issuer['state'] == 'issued':
            need(template['context'] == value['jobs'][issuer['job_id']]['context'], 'template-job-context')
    need(matched == counts['registry_requests'] and matched_issued == counts['jobs'], 'matched-issuer-census')
    need(value['source_templates'] == templates and value['source_job_index'] == reverse and value['works'] == works, 'source-reverse-coverage')


def materialized_rows(value):
    source, issuer = {}, {}
    for r in value['source_requests']:
        source[request_key('r', r['intent']['request_id']).hex()] = canon(r).hex()
    for rid, template in value['source_templates'].items():
        source[request_key('t', rid).hex()] = canon(template).hex()
    for jid, rid in value['source_job_index'].items():
        source[request_key('j', jid).hex()] = rid
    for jid, work in value['works'].items():
        source[(b'w' + jid.encode()).hex()] = canon(work).hex()
    for r in value['issuer_requests']:
        issuer[request_key('r', r['request_id']).hex()] = canon(r).hex()
    for jid, rid in value['issuer_job_index'].items():
        issuer[request_key('j', jid).hex()] = rid
    for jid, job in value['jobs'].items():
        issuer[(b'job:' + jid.encode()).hex()] = canon(job).hex()
    return dict(source=dict(sorted(source.items())), issuer=dict(sorted(issuer.items())))


def result_for(family, state, reason='parent_changed'):
    row = None if state == 'absent' else source_row(family, state, reason)
    return dict(version=1, state=state, intent_id='' if row is None else row['intent_id'],
                job_id='' if row is None else row['job_id'], resolved_at_ms=0 if row is None else row['resolved_at_ms'],
                reason='' if row is None else row['reason'], work=clone(family['work']) if state == 'work' else None)


def response_for(family, state, reason='parent_changed'):
    job = family['work']['job']
    recipient = next(x['script'] for x in family['setup']['settlement']['destinations'] if x['gym_id'] == job['gym_id'])
    return dict(version=3, binding_id=family['profile']['source_config_id'],
                settlement_config_id=ident('SLUG operator pool settlement config', family['setup']['settlement'], 'v2'),
                recipient_script=recipient, issue_profile_id=family['profile_id'], request_id=family['request_id'],
                gym_id=job['gym_id'], public_key=job['public_key'], result=result_for(family, state, reason))


def validate_response(value, family, lookup):
    fields(value, 'version,binding_id,settlement_config_id,recipient_script,issue_profile_id,request_id,gym_id,public_key,result')
    state = value['result']['state']
    need(state in ('absent', 'pending', 'work', 'issued_unused', 'not_issued'), 'result-state')
    need(lookup or state not in ('absent', 'pending'), 'resume-not-terminal')
    reason = value['result']['reason'] if state == 'not_issued' else 'parent_changed'
    need(reason in ('parent_changed', 'intake_drained'), 'terminal-reason')
    expected = response_for(family, state, reason)
    need(canon(value) == canon(expected), 'response-binding-or-result')


def produce_states(families):
    family = families[0]
    pairs = Cases('complete modeled NEW namespaces; inherited descriptor/job/template validity conditional except independently checked IDs; no live audit/write claim')
    for source in ('absent', 'pending', 'work', 'issued_unused', 'not_issued'):
        for issuer in ('absent', 'issued', 'sealed'):
            value = make_journal(family, source, issuer)
            pairs.add(source + '/' + issuer, value, validate_journal, family, valid=(source, issuer) in LEGAL)
    for row in pairs.rows:
        if row['valid']:
            row['namespace_rows'] = materialized_rows(row['value'])
    mutations = Cases(pairs.scope)
    work = make_journal(family, 'work', 'issued')
    jobid = family['work']['job']['id']
    rid = family['request_id']
    changes = [
        ('missing-source-reverse', lambda v: v['source_job_index'].clear()),
        ('missing-issuer-reverse', lambda v: v['issuer_job_index'].clear()),
        ('wrong-source-reverse', lambda v: v['source_job_index'].update({jobid: 'ab' * 32})),
        ('wrong-issuer-reverse', lambda v: v['issuer_job_index'].update({jobid: 'ab' * 32})),
        ('missing-work', lambda v: v['works'].clear()),
        ('extra-work', lambda v: v['works'].update({'ef' * 32: clone(family['work'])})),
        ('extra-job', lambda v: v['jobs'].update({'ef' * 32: clone(family['work']['job'])})),
        ('retained-template-on-work', lambda v: v['source_templates'].update({rid: clone(family['work']['template'])})),
        ('unknown-intent-field', lambda v: v['source_requests'][0]['intent'].update(adopt=True)),
        ('wrong-intent-id', lambda v: v['issuer_requests'][0].update(intent_id='ab' * 32)),
        ('wrong-gym', lambda v: v['issuer_requests'][0].update(gym_id='ab' * 32)),
        ('expired-work-commit', lambda v: v['source_requests'][0].update(resolved_at_ms=family['work']['job']['expires_at_ms'])),
        ('duplicate-source-request', lambda v: v['source_requests'].append(clone(v['source_requests'][0]))),
        ('duplicate-issuer-request', lambda v: v['issuer_requests'].append(clone(v['issuer_requests'][0]))),
        ('completed-work-terminal-contradiction', lambda v: v['source_requests'][0].update(state='issued_unused', reason='expired_before_work')),
    ]
    for name, change in changes:
        mutations.bad(name, work, change, validate_journal, family)
    for counter in ('source_requests', 'registry_requests', 'jobs', 'works'):
        mutations.bad('wrong-count-' + counter, work, lambda v, c=counter: v['counts'].update({c: v['counts'][c] + 1}), validate_journal, family)
        for wrong, suffix in ((True, 'bool'), (1.0, 'float')):
            mutations.bad('noninteger-' + counter + '-' + suffix, work,
                          lambda v, c=counter, x=wrong: v['counts'].update({c: x}), validate_journal, family)
    mutations.bad('reordered-selected-profile', work,
                  lambda v: v.update(profile=dict(reversed(list(v['profile'].items())))), validate_journal, family)
    pending = make_journal(family, 'pending', 'sealed')
    mutations.bad('orphan-seal-balanced-counts', pending,
                  lambda v: v['issuer_requests'][0].update(request_id='ba' * 32), validate_journal, family)
    mutations.bad('negative-seal-missing', make_journal(family, 'not_issued', 'sealed'),
                  lambda v: v['issuer_requests'].clear(), validate_journal, family)
    mutations.bad('unused-before-expiry', make_journal(family, 'issued_unused', 'issued'),
                  lambda v: v['source_requests'][0].update(resolved_at_ms=family['work']['job']['expires_at_ms'] - 1), validate_journal, family)
    responses = Cases('same selected request/profile/source/money/gym/key; terminal acceptance is authenticated protocol closure, not proof of miner eligibility')
    for state in ('absent', 'pending', 'work', 'issued_unused', 'not_issued'):
        responses.add('lookup-' + state, response_for(family, state), validate_response, family, True)
        responses.add('resume-' + state, response_for(family, state), validate_response, family, False, valid=state not in ('absent', 'pending'))
    responses.add('terminal-intake-drained', response_for(family, 'not_issued', 'intake_drained'), validate_response, family, False)
    for field in ('binding_id', 'settlement_config_id', 'issue_profile_id', 'request_id', 'gym_id', 'public_key'):
        responses.bad('wrong-' + field, response_for(family, 'issued_unused'),
                      lambda v, k=field: v.update({k: ('02' + 'ab' * 32) if k == 'public_key' else 'ab' * 32}), validate_response, family, True)
    responses.bad('wrong-recipient', response_for(family, 'work'), lambda v: v.update(recipient_script='0014' + 'ab' * 20), validate_response, family, True)
    responses.bad('terminal-work-injection', response_for(family, 'issued_unused'), lambda v: v['result'].update(work=clone(family['work'])), validate_response, family, True)
    responses.bad('missing-required-null', response_for(family, 'issued_unused'), lambda v: v['result'].pop('work'), validate_response, family, True)
    return dict(state_pairs=pairs.rows, state_refusals=mutations.rows, response_cases=responses.rows,
                all_legal_pairs=sorted([list(pair) for pair in LEGAL]),
                unchanged_financial_scope='No payout amounts, due decisions, signed batches or D84 policy are generated or changed.')
