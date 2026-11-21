"""New wrapper bytes and matching unmined jobs from frozen public inputs."""
from core import (Cases, canon, clone, derive_job, fields, frame, gym_id, ident,
                  make_profile, manifest_preimage, need, profile_id, sha,
                  source_id, validate_profile)


def exact_wrapper(value, base, profile, tail=()):
    fields(value, ','.join(['version', 'base', 'issue_profile'] + list(tail)))
    need(value['version'] == 1 and type(value['version']) is int, 'wrapper-version')
    need(canon(value['base']) == canon(base), 'original-base')
    need(canon(value['issue_profile']) == canon(profile), 'original-profile')


def record(name, value, framing='value', label=None, scope='conditional inherited-valid base bytes; no live owner authority'):
    raw = canon(value) + (b'\n' if framing == 'lf' else b'')
    result = dict(name=name, scope=scope, value=value, framing=framing, bytes=len(raw), canonical_hex=raw.hex())
    if label:
        pre = frame(label, canon(value))
        result.update(id=sha(pre), preimage_hex=pre.hex())
    return result


def new_trust(old_trust, profile):
    body = old_trust.encode('ascii') + profile_id(profile).encode('ascii')
    pre = frame('SLUG pool issue recovery transport', body)
    return pre, sha(pre)


def worker_wrapper(worker, client, profile, old_trust):
    _, trust = new_trust(old_trust, profile)
    return dict(version=1, base=clone(worker), issue_profile=clone(profile),
                settlement_config_id=client['settlement_config_id'], recipient_script=client['recipient_script'],
                transport_trust_id=trust)


def produce_fixtures(inherited, manifest, recipe, name_v2_reference):
    positives = {x['name']: x for x in inherited['composition']['positives']}
    selected = [positives[name] for name in recipe['composition_names']]
    wrappers, boundaries, families, profiles = [], [], [], []
    profile_cases = Cases('new profile predicate; inherited source validity is a conditional input premise')
    wrapper_cases = Cases('exact selected complete base/profile binding; no runtime or wallet authority')
    descriptors = {x['id']: x['descriptor'] for x in manifest['descriptors']}
    # Setup2 selects the later independently qualified names-v2 candidate.
    # The old ManifestJob2 corpus remains the exact job-grammar/parity input;
    # it does not contain this newer descriptor tuple. Never relabel an old one.
    selected_descriptor = name_v2_reference['descriptor']
    descriptor_raw = canon(selected_descriptor['descriptor'])
    descriptor_preimage = b'SLUG fighter issuance descriptor\0v1\0' + descriptor_raw
    need(descriptor_raw.decode('utf-8') == selected_descriptor['canonical']
         and descriptor_preimage.hex() == selected_descriptor['preimage_hex']
         and sha(descriptor_preimage) == selected_descriptor['id'], 'inherited-name-v2-descriptor-bytes')
    need(selected_descriptor['supported_candidate'] is True
         and selected_descriptor['id'] not in descriptors, 'distinct-inherited-name-v2-selection')
    descriptors[selected_descriptor['id']] = selected_descriptor['descriptor']
    old_work = next(x['value']['work'] for x in inherited['transport']['envelope_cases']
                    if x['name'] == 'coinbase-only-work' and x['valid'])

    def wrapped_pair(name, old_value, new_value, old_fields, new_fields, framing='value'):
        need(list(old_value) == old_fields.split(','), 'inherited-field-order:' + name)
        need(list(new_value) == new_fields.split(','), 'new-field-order:' + name)
        wrappers.append(record(name, new_value, framing))
        # The selected decoders require these closed shapes, so cross-mode
        # refusal is modeled explicitly without invoking product decoders.
        need(list(old_value) != list(new_value), 'mode-distinct-shape:' + name)
        boundaries.append(dict(name=name, inherited_hex=(canon(old_value) + (b'\n' if framing == 'lf' else b'')).hex(),
                               selected_hex=wrappers[-1]['canonical_hex'], old_accepts_new=False,
                               new_accepts_old=False, basis='closed ordered fields and explicit selector'))

    for index, item in enumerate(selected):
        setup = item['setup']
        need(ident('SLUG pool operator setup', setup, 'v2') == item['setup_id'], 'inherited-setup-id')
        need(canon(setup).hex() == item['setup_hex'], 'inherited-setup-bytes')
        source = setup['pool']
        need(source_id(source) == item['source_id'], 'inherited-source-id')
        profile = make_profile(source, item['setup_id'])
        profiles.append(dict(name=item['name'], profile=profile, profile_id=profile_id(profile),
                             canonical_hex=canon(profile).hex(), preimage_hex=frame('SLUG pool issue recovery profile', canon(profile)).hex()))
        profile_cases.add(item['name'], profile, validate_profile, source, item['setup_id'])
        setup_wrapper = dict(version=1, base=clone(setup), issue_profile=clone(profile))
        setup_id = ident('SLUG pool issue recovery setup', setup_wrapper)
        wrappers.append(record(item['name'] + '-setup', setup_wrapper, label='SLUG pool issue recovery setup'))
        wrapper_cases.add(item['name'], setup_wrapper, exact_wrapper, setup, profile)
        intent = clone(setup_wrapper)
        wrappers.append(record(item['name'] + '-operator-intent', intent, 'lf'))
        ready = dict(version=1, runtime_id=setup['runtime_id'], setup_id=setup_id, issue_profile_id=profile_id(profile))
        wrappers.append(record(item['name'] + '-operator-ready', ready, 'lf'))
        for field, suffix in [('provision_intent_hex', 'provision-intent'), ('provision_ready_hex', 'provision-ready'),
                              ('published_fixture_backup_hex', 'published-fixture-backup')]:
            import json
            base = json.loads(bytes.fromhex(item[field]))
            new_value = (dict(version=1, base=base, issue_recovery_version=1) if suffix == 'provision-intent'
                         else dict(version=1, base=base, setup_id=setup_id, issue_profile_id=profile_id(profile)))
            wrappers.append(record(item['name'] + '-' + suffix, new_value, 'lf',
                                   scope='unchanged published synthetic fixture material only; never a real custody/backup assertion'))
        pin = source['authority_pin']
        descriptor = descriptors[pin['descriptor_id']]
        base_marker = dict(application='slug-prospect-registry', version=5, network=pin['network'], genesis=pin['genesis'],
                           authority_id=pin['authority_id'], descriptor_id=pin['descriptor_id'], descriptor=clone(descriptor))
        base_meta = {k: clone(v) for k, v in base_marker.items() if k != 'application'}
        base_meta.update(last_sequence=0, job_count=0, last_clock_ms=0)
        wrapped_pair(item['name'] + '-issuer-marker', base_marker,
                     dict(application='slug-manifest-issue-recovery', version=1, base=base_marker, issue_profile=profile),
                     'application,version,network,genesis,authority_id,descriptor_id,descriptor',
                     'application,version,base,issue_profile', 'lf')
        wrapped_pair(item['name'] + '-issuer-meta', base_meta,
                     dict(application='slug-manifest-issue-recovery', version=1, base=base_meta, issue_profile=profile, requests=0),
                     'version,network,genesis,authority_id,descriptor_id,descriptor,last_sequence,job_count,last_clock_ms',
                     'application,version,base,issue_profile,requests')
        pool_marker = dict(application='slug-pool-share-journal', version=2, config=clone(source))
        pool_meta = clone(pool_marker)
        pool_meta.update(works=0, submissions=0, accepted=0, bytes=0, last_clock_ms=0)
        wrapped_pair(item['name'] + '-source-marker', pool_marker,
                     dict(application='slug-pool-issue-recovery', version=1, base=pool_marker, issue_profile=profile),
                     'application,version,config', 'application,version,base,issue_profile')
        wrapped_pair(item['name'] + '-source-meta', pool_meta,
                     dict(application='slug-pool-issue-recovery', version=1, base=pool_meta, issue_profile=profile,
                          requests=0, request_bytes=0, request_reserved=0),
                     'application,version,config,works,submissions,accepted,bytes,last_clock_ms',
                     'application,version,base,issue_profile,requests,request_bytes,request_reserved')

        # This NEW family is a matching full composition, not an old work with
        # relabeled authority. Only public fixed challenge/time inputs are used.
        template = clone(old_work['template'])
        need(all(template['context'][k] == source['policy'][k] for k in source['policy']), 'matching-template-policy')
        key = setup['miners']['enrollments'][0]['public_key']
        job = derive_job(template['context'], pin, key, recipe['job_challenges'][index], recipe['issued_at_ms'])
        need(job['gym_id'] == setup['miners']['enrollments'][0]['gym_id'], 'matching-selected-gym')
        work = dict(job=job, template=template)
        families.append(dict(name=item['name'], scope='new independently derived unmined matching-owner family; no new PoW expected value',
                             setup=setup, setup_id=item['setup_id'], profile=profile, profile_id=profile_id(profile),
                             work=work, job_preimage_hex=manifest_preimage(job).hex(), job_hex=canon(job).hex(),
                             template_hex=canon(template).hex(), request_id=recipe['request_ids'][index]))

    first = selected[0]
    base = profiles[0]['profile']
    for count in (1, 8, 8192):
        source = clone(first['setup']['pool'])
        source['max_works'] = count
        value = make_profile(source, recipe['conditional_setup_id'])
        profile_cases.add('conditional-source-count-' + str(count), value, validate_profile, source)
    for name, field, value in [('zero-count', 'max_requests', 0), ('over-count', 'max_requests', 8193),
                               ('count-mismatch', 'max_requests', 1), ('wrong-source', 'source_config_id', 'ee' * 32),
                               ('wrong-setup', 'base_setup_id', 'dd' * 32), ('old-profile-version', 'version', 2)]:
        profile_cases.bad(name, base, lambda v, f=field, x=value: v.update({f: x}), validate_profile,
                          first['setup']['pool'], first['setup_id'])
    wrapper = dict(version=1, base=clone(first['setup']), issue_profile=clone(base))
    wrapper_cases.bad('changed-base-limit', wrapper, lambda v: v['base']['pool'].update(max_works=7), exact_wrapper, first['setup'], base)
    wrapper_cases.bad('changed-profile', wrapper, lambda v: v['issue_profile'].update(max_requests=7), exact_wrapper, first['setup'], base)
    wrapper_cases.bad('missing-wrapper', wrapper, lambda v: v.pop('issue_profile'), exact_wrapper, first['setup'], base)
    wrapper_cases.bad('unknown-wrapper-field', wrapper, lambda v: v.update(adopt=True), exact_wrapper, first['setup'], base)
    wrapper_cases.bad('reordered-nested-profile', wrapper,
                      lambda v: v.update(issue_profile=dict(reversed(list(v['issue_profile'].items())))),
                      exact_wrapper, first['setup'], base)

    conditional = []
    for item in inherited['transport']['wire_identities']:
        client, worker = item['client'], item['worker']
        profile = make_profile(client['source'], recipe['conditional_setup_id'])
        pre, trust = new_trust(item['trust_id'], profile)
        cw = dict(version=1, base=clone(client), issue_profile=profile)
        ww = worker_wrapper(worker, client, profile, item['trust_id'])
        wrapped_pair(item['name'] + '-client', client, cw, 'version,url,token,ca_certificate,public_key,source,settlement_config_id,recipient_script', 'version,base,issue_profile')
        wrappers.append(record(item['name'] + '-worker-config', ww))
        marker = dict(application='slug-pool-worker-issue-recovery', version=1, config=ww)
        metadata = clone(marker)
        metadata.update(last_sequence=0, records=0, bytes=0, pending=0, reserved=0, requests=0, active_request_id='')
        wrappers.append(record(item['name'] + '-worker-marker', marker))
        wrappers.append(record(item['name'] + '-worker-meta', metadata))
        conditional.append(dict(name=item['name'], scope='conditional selected opaque SetupID; NOT matched full operator composition',
                                profile=profile, client=cw, worker=ww, trust_id=trust, trust_preimage_hex=pre.hex()))

    remote = []
    for item in inherited['transport']['remote_positives']:
        need(item['name'] in recipe['remote_names'], 'selected-remote')
        config = item['config']
        profile = make_profile(config['source'], recipe['conditional_setup_id'])
        wrapper = dict(version=1, base=clone(config), issue_profile=profile)
        cid = ident('SLUG pool issue recovery attachment', wrapper)
        intention = dict(version=1, application='slug-remote-pool-issue-recovery', config_id=cid, config=wrapper)
        intention_raw = canon(intention) + b'\n'
        ready = dict(version=1, application='slug-remote-pool-issue-recovery', config_id=cid, intent_sha256=sha(intention_raw))
        wrappers.append(record(item['name'] + '-remote', wrapper, label='SLUG pool issue recovery attachment'))
        wrappers.append(record(item['name'] + '-remote-intent', intention, 'lf'))
        wrappers.append(record(item['name'] + '-remote-ready', ready, 'lf'))
        credentials = []
        for audience in ('pool', 'admission'):
            credential = dict(version=2, config_id=item['config_id'],
                              gym_id=config['expected_local']['recipient']['config']['identity']['gym_id'],
                              audience=audience, generation=1, expires_at_ms=recipe['credential_expires_at_ms'],
                              token=recipe['synthetic_tokens'][audience])
            wrapped = dict(version=1, base=credential, issue_profile_id=profile_id(profile))
            wrappers.append(record(item['name'] + '-credential-' + audience, wrapped, 'lf'))
            credentials.append(wrapped)
        remote.append(dict(name=item['name'], scope='conditional inherited local recipient/credential grammar; no live wallet assertion',
                           config=wrapper, config_id=cid, credentials=credentials,
                           original_local_unchanged=canon(wrapper['base']['expected_local']) == canon(config['expected_local'])))

    anchors = []
    for item in inherited['transport']['envelope_cases']:
        if item['valid'] and item['name'].endswith(('-work', '-submission')):
            need(canon(item['value']).hex() == item['canonical_hex'], 'inherited-wire-bytes')
            anchors.append(dict(name=item['name'], value=clone(item['value']), canonical_hex=item['canonical_hex'],
                                scope='unchanged inherited conditional PoW/work/receipt anchor; no new hash calculation'))
    for old in manifest['jobs']:
        need(sha(manifest_preimage(old['job'])) == old['job']['id'], 'inherited-manifest-ID')
    return dict(profiles=profiles, profile_cases=profile_cases.rows, wrapper_cases=wrapper_cases.rows,
                wrappers=wrappers, mode_boundaries=boundaries, matching_families=families,
                conditional_transport=conditional, remote=remote, inherited_anchors=anchors)
