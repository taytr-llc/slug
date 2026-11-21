"""Version3 outer correlation; embedded DAILY economic/PoW cases stay exact."""
from core import Cases, canon, clone, fields, need, profile_id, source_id
from fixtures import make_profile


def extend(value, profile, page=False):
    result = {}
    for key, item in value.items():
        result[key] = 3 if key == 'version' else clone(item)
        if key == ('settlement_config_id' if page else 'recipient_script'):
            result['issue_profile_id'] = profile_id(profile)
    return result


def validate_extension(value, original, profile, page, inherited_valid):
    need(inherited_valid, 'inherited-invalid-body')
    expected = extend(original, profile, page)
    need(canon(value) == canon(expected), 'selected-outer-correlation')
    need(value['version'] == 3 and value['binding_id'] == profile['source_config_id'], 'selected-source')


def validate_error(value, expected, status, inherited_valid):
    fields(value, 'version,error')
    fields(value['error'], 'code,outcome_unknown')
    need(type(value['version']) is int and value['version'] == 3, 'selected-error-version')
    need(inherited_valid and canon(value) == canon(expected), 'unchanged-error-code-flag')
    need(status in (400, 401, 403, 404, 409, 413, 429, 500, 503), 'inherited-status')


def produce_wire(fixtures, inherited, recipe):
    profiles = {source_id(i['client']['source']): make_profile(i['client']['source'], recipe['conditional_setup_id'])
                for i in inherited['transport']['wire_identities']}
    first = fixtures['conditional_transport'][0]['profile']
    cases = Cases('exact new outer envelope over unchanged inherited classified DAILY body; not a second payout/PoW oracle')
    for group, source in [('work-receipt', inherited['transport']['envelope_cases']),
                          ('balance', inherited['payments']['balances']), ('page', inherited['payments']['pages'])]:
        for old in source:
            profile = profiles.get(old['value'].get('binding_id'), first)
            value = extend(old['value'], profile, group == 'page')
            row = cases.add(group + '-' + old['name'], value, validate_extension, old['value'], profile, group == 'page', old['valid'], valid=old['valid'])
            row['inherited_canonical_hex'] = old['canonical_hex']
            row['profile'] = profile
            if 'request' in old:
                row['request'] = clone(old['request'])
            if old['valid']:
                cases.bad(group + '-wrong-profile-' + old['name'], value,
                          lambda v: v.update(issue_profile_id='ab' * 32), validate_extension,
                          old['value'], profile, group == 'page', True)
    errors = Cases('Version3 only; exact inherited DAILY code/flag/status; no server error clears retained local intent')
    for old in inherited['transport']['error_cases']:
        value = clone(old['value'])
        expected = clone(value)
        expected['version'] = 3
        # Preserve a deliberately wrong inherited outer version as wrong; do
        # not turn that negative into a valid frame then inherit its old label.
        if value['version'] == 2:
            value['version'] = 3
        row = errors.add(old['name'], value, validate_error, expected, old['http_status'], old['valid'], valid=old['valid'])
        row.update(http_status=old['http_status'], clears_active_request=False)
    return dict(response_cases=cases.rows, error_cases=errors.rows,
                page_requests_unchanged=clone(inherited['payments']['requests']),
                retained_legacy_error_status_map=clone(inherited['transport']['error_status_map']),
                scope='Old invalid-body cases remain invalid by inherited premise, not by a newly derived monetary rule; additional profile mutations are independently refused.')
