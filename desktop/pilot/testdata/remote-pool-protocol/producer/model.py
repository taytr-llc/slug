"""Independent conditional protocol model; public inputs only, no Go outputs.

The copied remote config corpus supplies previously independently qualified
identities and configs. This model implements only the new wire grammar and
cross-correlations. It asserts no live owner, process, balance or mining fact.
"""
import copy
import hashlib
import json
import re
import struct

MAX_SAFE = 9007199254740991
MAX_ATOMS = 2100000000000000
REQUEST_LIMIT = 2048
RESPONSE_LIMIT = 16384
REQ = 'version id command config_id view_id fixture'.split()
RES = 'version id command config_id view_id ok result error'.split()
SNAP = 'version config_id state account view remote error'.split()
ACCOUNT = 'state fixture id generation hold_until_ms held can_export last_error'.split()
VIEW = 'version view_id account_authority_id account_id generation fixture root_public_key gym_public_key gym_id'.split()
REMOTE = ('version config_id view source_binding_id original_source_binding_id state worker_state last_error attempts '
          'hashes_per_second_milli records pending wallet_state wallet_height spendable_atoms immature_atoms '
          'unconfirmed_atoms payout_address').split()
COMMANDS = ['snapshot', 'login_fixture', 'start', 'stop', 'logout']
ERRORS = ['invalid_request', 'stale_view', 'busy', 'unauthorized', 'forbidden', 'cancelled',
          'remote_unavailable', 'outcome_unknown', 'cleanup_incomplete']
WORKERS = ['ready', 'stopped', 'failed', 'paused', 'preparing', 'retrying', 'hashing', 'delivering', 'unavailable']
WORKER_ERRORS = ['', 'unavailable', 'outcome_unknown', 'busy', 'capacity', 'rate_limited', 'expired',
                 'rejected', 'not_found', 'unauthorized', 'forbidden', 'invalid_request', 'too_large',
                 'stale', 'clock', 'cycle_unavailable', 'signer_unavailable', 'worker_unavailable', 'cleanup_incomplete']


def raw(x):
    return json.dumps(x, ensure_ascii=True, separators=(',', ':')).encode('ascii')


def sha(x):
    return hashlib.sha256(x).hexdigest()


def config_id(c):
    b = raw(c)
    return sha(b'SLUG remote pool attachment\0v1\0' + struct.pack('<I', len(b)) + b)


def integer(x, lo=0, hi=MAX_SAFE):
    return type(x) is int and lo <= x <= hi


def ident(s):
    return type(s) is str and re.fullmatch('[0-9a-f]{64}', s) is not None and s != '0' * 64


def shape(x, keys):
    assert type(x) is dict and list(x) == keys


def scalars(x, depth=0):
    assert depth <= 8
    if type(x) is dict:
        assert len(x) <= 24
        for k, v in x.items():
            scalars(k, depth + 1)
            scalars(v, depth + 1)
    elif type(x) is str:
        assert all(32 <= ord(c) <= 126 for c in x)
    elif type(x) is int:
        assert integer(x)
    else:
        assert x is None or type(x) is bool


def decode(b, limit):
    assert 0 < len(b) <= limit and b'\r' not in b and b'\n' not in b
    assert all(c < 128 for c in b)
    def pairs(rows):
        d = {}
        for k, v in rows:
            assert k not in d
            d[k] = v
        return d
    def number(s):
        assert re.fullmatch('0|[1-9][0-9]*', s)
        n = int(s)
        assert integer(n)
        return n
    def forbidden(_):
        raise AssertionError('noninteger number')
    x = json.loads(b.decode('ascii'), object_pairs_hook=pairs, parse_int=number,
                   parse_float=forbidden, parse_constant=forbidden)
    scalars(x)
    return x


def request_ok(q, c):
    try:
        shape(q, REQ)
        scalars(q)
        assert len(raw(q)) <= REQUEST_LIMIT and q['version'] == 1 and type(q['version']) is int
        assert integer(q['id'], 1) and q['command'] in COMMANDS and q['config_id'] == config_id(c)
        assert type(q['view_id']) is str and (q['view_id'] == '' or ident(q['view_id']))
        assert q['fixture'] == (c['expected_local']['recipient']['config']['fixture'] if q['command'] == 'login_fixture' else '')
        if q['command'] == 'login_fixture':
            assert q['view_id'] == ''
        elif q['command'] in ['start', 'stop', 'logout']:
            assert ident(q['view_id'])
        return True
    except (AssertionError, KeyError, TypeError, ValueError):
        return False


def snapshot_ok(s, c):
    try:
        shape(s, SNAP)
        scalars(s)
        assert type(s['version']) is int and s['version'] == 1 and s['config_id'] == config_id(c)
        a = s['account']
        shape(a, ACCOUNT)
        rc = c['expected_local']['recipient']['config']
        identity = rc['identity']
        assert s['state'] in ['signed_out', 'signed_in', 'failed', 'expired'] and a['state'] == s['state']
        assert a['fixture'] == rc['fixture'] and a['can_export'] is False and type(a['held']) is bool
        assert s['error'] == a['last_error']
        if s['state'] != 'signed_in':
            assert s['view'] is None and s['remote'] is None and a['id'] == ''
            assert type(a['generation']) is int and a['generation'] == 0
            assert type(a['hold_until_ms']) is int and a['hold_until_ms'] == 0 and a['held'] is False
            allowed = {'signed_out': [''], 'failed': ['remote_unavailable'], 'expired': ['', 'remote_unavailable']}
            assert s['error'] in allowed[s['state']]
            return True
        assert s['error'] == '' and a['id'] == rc['account_id'] and integer(a['generation'], 1)
        assert integer(a['hold_until_ms']) and (not a['held'] or a['hold_until_ms'] > 0)
        v = s['view']
        shape(v, VIEW)
        assert type(v['version']) is int and v['version'] == 1 and ident(v['view_id'])
        assert v['generation'] == a['generation'] and type(v['generation']) is int
        for k in ['account_authority_id', 'account_id', 'fixture']:
            assert v[k] == rc[k]
        for k in ['root_public_key', 'gym_public_key', 'gym_id']:
            assert v[k] == identity[k]
        r = s['remote']
        shape(r, REMOTE)
        assert type(r['version']) is int and r['version'] == 1 and r['config_id'] == s['config_id'] and raw(r['view']) == raw(v)
        assert r['source_binding_id'] == c['miners']['source_id']
        assert r['original_source_binding_id'] == c['expected_local']['original_source_binding_id']
        assert r['payout_address'] == c['expected_local']['recipient']['recipient']['address']
        assert r['state'] in ['stopped', 'starting', 'running', 'failed'] and r['worker_state'] in WORKERS
        assert r['last_error'] in WORKER_ERRORS
        assert integer(r['attempts']) and integer(r['hashes_per_second_milli'])
        assert r['state'] == 'running' or r['hashes_per_second_milli'] == 0
        assert integer(r['records'], 0, c['max_records']) and integer(r['pending'], 0, r['records'])
        assert r['wallet_state'] in ['unavailable', 'syncing', 'ready'] and integer(r['wallet_height'], 0, 2147483647)
        amounts = [r[k] for k in ['spendable_atoms', 'immature_atoms', 'unconfirmed_atoms']]
        assert all(integer(x, 0, MAX_ATOMS) for x in amounts)
        if r['wallet_state'] == 'unavailable':
            assert r['wallet_height'] == 0 and amounts == [0, 0, 0]
        return True
    except (AssertionError, KeyError, TypeError, ValueError):
        return False


def response_ok(r, q, c, prior):
    try:
        assert request_ok(q, c)
        shape(r, RES)
        scalars(r)
        assert len(raw(r)) <= RESPONSE_LIMIT and type(r['ok']) is bool
        for k in RES[:5]:
            assert r[k] == q[k] and type(r[k]) is type(q[k])
        if q['command'] in ['start', 'stop', 'logout'] or q['command'] == 'snapshot' and q['view_id']:
            assert prior is not None and q['view_id'] == prior['view_id']
        else:
            assert prior is None
        if not r['ok']:
            assert r['result'] is None and r['error'] in ERRORS
            return True
        s = r['result']
        assert r['error'] == '' and snapshot_ok(s, c)
        cmd = q['command']
        if cmd == 'login_fixture':
            assert s['state'] == 'signed_in' and s['remote']['state'] == 'stopped'
        elif cmd == 'logout':
            assert s['state'] == 'signed_out'
        elif cmd in ['start', 'stop']:
            assert s['state'] == 'signed_in' and s['view'] == prior
            if cmd == 'stop':
                assert s['remote']['state'] in ['stopped', 'failed'] and s['remote']['hashes_per_second_milli'] == 0
        elif prior is None:
            assert s['state'] != 'signed_in'
            if q['id'] == 1:
                assert s['state'] == 'signed_out'
        elif s['state'] == 'signed_in':
            assert s['view'] == prior
        return True
    except (AssertionError, KeyError, TypeError, ValueError):
        return False


def view(c, suffix='original'):
    rc = c['expected_local']['recipient']['config']
    v = dict(version=1, view_id=sha(b'SLUG remote native protocol public view\0v1\0' +
                                  rc['fixture'].encode('ascii') + b'\0' + suffix.encode('ascii')),
             account_authority_id=rc['account_authority_id'], account_id=rc['account_id'], generation=1,
             fixture=rc['fixture'], root_public_key=rc['identity']['root_public_key'],
             gym_public_key=rc['identity']['gym_public_key'], gym_id=rc['identity']['gym_id'])
    return v


def snapshot(c, state='signed_in', running=False):
    rc = c['expected_local']['recipient']['config']
    a = dict(state=state, fixture=rc['fixture'], id='', generation=0, hold_until_ms=0,
             held=False, can_export=False, last_error='')
    s = dict(version=1, config_id=config_id(c), state=state, account=a, view=None, remote=None, error='')
    if state == 'failed':
        s['error'] = a['last_error'] = 'remote_unavailable'
    if state != 'signed_in':
        return s
    v = view(c)
    a.update(id=rc['account_id'], generation=1)
    s['view'] = v
    s['remote'] = dict(version=1, config_id=config_id(c), view=copy.deepcopy(v),
                       source_binding_id=c['miners']['source_id'],
                       original_source_binding_id=c['expected_local']['original_source_binding_id'],
                       state='running' if running else 'stopped', worker_state='hashing' if running else 'stopped',
                       last_error='', attempts=7, hashes_per_second_milli=1250 if running else 0,
                       records=1, pending=0, wallet_state='ready', wallet_height=104,
                       spendable_atoms=990000000, immature_atoms=0, unconfirmed_atoms=0,
                       payout_address=c['expected_local']['recipient']['recipient']['address'])
    return s


def request(c, cmd, n=2, with_view=None):
    if with_view is None:
        with_view = cmd in ['start', 'stop', 'logout']
    return dict(version=1, id=n, command=cmd, config_id=config_id(c),
                view_id=view(c)['view_id'] if with_view else '',
                fixture=c['expected_local']['recipient']['config']['fixture'] if cmd == 'login_fixture' else '')


def response(q, s=None, error=''):
    return dict(version=q['version'], id=q['id'], command=q['command'], config_id=q['config_id'],
                view_id=q['view_id'], ok=not error, result=s if not error else None, error=error)


def mutate(x, path, value):
    y = copy.deepcopy(x)
    target = y
    keys = path.split('.')
    for k in keys[:-1]:
        target = target[k]
    target[keys[-1]] = value
    return y


def trace_results(trace):
    """Small specification machine, not process/controller simulation."""
    owner, alive, last, retained = 'original', True, 0, ''
    observed = []
    for step in trace['steps']:
        accept = alive and step['helper'] == owner
        n, cmd, given = step['id'], step['command'], step['view']
        if accept and (n <= last or last == 0 and (n != 1 or cmd != 'snapshot' or given != '')):
            accept, alive = False, False
        if accept:
            if cmd == 'login_fixture':
                accept = not retained and not given
            elif cmd in ['start', 'stop', 'logout']:
                accept = bool(retained) and given == retained
            elif cmd == 'snapshot':
                accept = given == retained
            else:
                accept, alive = False, False
        if accept:
            last = n
            result = step['outcome']
            if result == 'unknown':
                alive, retained = False, ''
            elif result == 'pre_busy':
                pass
            elif result == 'expire':
                assert cmd == 'snapshot' and retained
                retained = ''
            elif result == 'success':
                if cmd == 'login_fixture':
                    retained = step['new_view']
                    assert retained in ['first', 'second']
                elif cmd == 'logout':
                    retained = ''
            else:
                raise AssertionError('unknown trace outcome')
        assert accept == step['accept'], trace['name']
        observed.append(dict(accepted=accept, helper_alive=alive, last_id=last, retained_view=retained))
    return observed


def build(inputs):
    corpus_bytes = (inputs / 'remote-config-v1.json').read_bytes()
    assert sha(corpus_bytes) == '5f67fcaaba0c78b97b0f116a4c598eb0ec62ee1436bbcf280edcc747856e5177'
    inherited = json.loads(corpus_bytes)
    recipe = json.loads((inputs / 'recipe.json').read_bytes())
    selected = {x['name']: x for x in inherited['positives'] if x['name'] in recipe['config_names']}
    assert list(selected) == recipe['config_names']
    for row in selected.values():
        assert raw(row['config']).hex() == row['canonical_hex'] and config_id(row['config']) == row['config_id']
    cfgs = {name: x['config'] for name, x in selected.items()}
    requests, request_refusals, positives, refusals, raw_cases = [], [], [], [], []

    def add(name, cfg, q, s=None, error=''):
        c = cfgs[cfg]
        prior = view(c) if q['view_id'] else None
        r = response(q, s, error)
        assert response_ok(r, q, c, prior), name
        b = raw(r)
        assert response_ok(decode(b, RESPONSE_LIMIT), q, c, prior), name
        positives.append(dict(name=name, config=cfg, request=q, prior_view=prior, response=r,
                              canonical_hex=b.hex(), sha256=sha(b)))

    defaults = recipe['default_names']
    for cfg in defaults:
        c = cfgs[cfg]
        for cmd, n, with_view in [('snapshot', 1, False), ('login_fixture', 2, False), ('start', 3, True),
                                  ('stop', 4, True), ('logout', 5, True), ('snapshot', 6, True)]:
            q = request(c, cmd, n, with_view)
            assert request_ok(q, c) and request_ok(decode(raw(q), REQUEST_LIMIT), c)
            requests.append(dict(name=cfg + '-' + cmd + ('-original' if cmd == 'snapshot' and with_view else ''),
                                 config=cfg, request=q, canonical_hex=raw(q).hex(), sha256=sha(raw(q))))
            state = 'signed_out' if cmd == 'logout' or n == 1 else 'signed_in'
            add(cfg + '-' + cmd + ('-original' if cmd == 'snapshot' and with_view else ''), cfg, q,
                snapshot(c, state, running=cmd in ['start', 'snapshot'] and n != 1))
        add(cfg + '-expired', cfg, request(c, 'snapshot', 7, True), snapshot(c, 'expired'))

    cfg = defaults[0]
    c = cfgs[cfg]
    for name, cmd, path, value in [
        ('unknown-command', 'snapshot', 'command', 'scope'),
        ('inactive-fixture', 'snapshot', 'fixture', 'zero256'),
        ('wrong-login-fixture', 'login_fixture', 'fixture', 'incrementing256'),
        ('start-empty-view', 'start', 'view_id', ''),
        ('logout-zero-view', 'logout', 'view_id', '0' * 64),
        ('login-active-view', 'login_fixture', 'view_id', view(c)['view_id']),
        ('wrong-selected-config', 'snapshot', 'config_id', 'a' * 64),
        ('zero-id', 'snapshot', 'id', 0), ('future-version', 'snapshot', 'version', 2),
        ('unsafe-id', 'snapshot', 'id', MAX_SAFE + 1), ('fraction-id', 'snapshot', 'id', 1.5),
        ('boolean-version', 'snapshot', 'version', True),
    ]:
        bad = mutate(request(c, cmd), path, value)
        assert not request_ok(bad, c), name
        request_refusals.append(dict(name=name, config=cfg, request=bad))
    q = request(c, 'snapshot', 8, True)
    s = snapshot(c)
    no_wallet = copy.deepcopy(s)
    no_wallet['remote'].update(wallet_state='unavailable', wallet_height=0, spendable_atoms=0)
    add('wallet-unavailable', cfg, q, no_wallet)
    add('wallet-syncing', cfg, q, mutate(s, 'remote.wallet_state', 'syncing'))
    failed = copy.deepcopy(s)
    failed['remote'].update(state='failed', worker_state='failed', last_error='worker_unavailable')
    add('worker-failed', cfg, q, failed)
    held = copy.deepcopy(s)
    held['account'].update(hold_until_ms=recipe['hold_until_ms'], held=True)
    add('held-observation', cfg, q, held)
    add('parent-failed', cfg, request(c, 'snapshot', 8), snapshot(c, 'failed'))
    expired = snapshot(c, 'expired')
    expired['error'] = expired['account']['last_error'] = 'remote_unavailable'
    add('expired-unavailable', cfg, q, expired)
    maximum = cfgs[recipe['maximum_name']]
    ms = snapshot(maximum, running=True)
    ms['remote'].update(attempts=MAX_SAFE, hashes_per_second_milli=MAX_SAFE, records=maximum['max_records'],
                        pending=maximum['max_records'], wallet_height=2147483647, spendable_atoms=MAX_ATOMS,
                        immature_atoms=MAX_ATOMS, unconfirmed_atoms=MAX_ATOMS)
    add('maximum-scalars', recipe['maximum_name'], request(maximum, 'snapshot', MAX_SAFE, True), ms)
    add('stop-failed-worker', cfg, request(c, 'stop', 9), failed)
    for code in ERRORS:
        eq = request(c, 'start', 10) if code == 'outcome_unknown' else request(c, 'snapshot', 10)
        add('error-' + code, cfg, eq, error=code)

    baseline = next(x for x in positives if x['name'] == cfg + '-start')
    replacements = [
        ('version', 2), ('id', 4), ('command', 'stop'), ('config_id', 'a' * 64), ('view_id', 'b' * 64),
        ('error', 'busy'), ('ok', 1), ('result.version', True), ('result.config_id', 'a' * 64),
        ('result.account.fixture', 'other'), ('result.account.id', 'a' * 64), ('result.account.generation', 2),
        ('result.account.can_export', True), ('result.account.held', True), ('result.account.hold_until_ms', -1),
        ('result.view.account_authority_id', 'a' * 64), ('result.view.account_id', 'a' * 64),
        ('result.view.gym_id', 'a' * 64), ('result.view.gym_public_key', '02' + 'a' * 64),
        ('result.view.root_public_key', 'not-the-original-root'), ('result.view.view_id', 'a' * 64),
        ('result.view.generation', 0), ('result.view.fixture', 'other'), ('result.remote', None),
        ('result.remote.source_binding_id', c['expected_local']['original_source_binding_id']),
        ('result.remote.original_source_binding_id', c['miners']['source_id']),
        ('result.remote.config_id', 'a' * 64), ('result.remote.payout_address', 'not-the-original-address'),
        ('result.remote.view.view_id', 'a' * 64), ('result.remote.state', 'complete'),
        ('result.remote.worker_state', 'complete'), ('result.remote.last_error', 'raw-private-error'),
        ('result.remote.attempts', MAX_SAFE + 1), ('result.remote.hashes_per_second_milli', -1),
        ('result.remote.records', c['max_records'] + 1), ('result.remote.pending', 2),
        ('result.remote.wallet_height', 2147483648), ('result.remote.spendable_atoms', MAX_ATOMS + 1),
        ('result.remote.wallet_state', 'unavailable'), ('result.account.state', 'signed_out'),
    ]
    for path, value in replacements:
        r = mutate(baseline['response'], path, value)
        assert not response_ok(r, baseline['request'], c, baseline['prior_view']), path
        refusals.append(dict(name='wrong-' + path, config=cfg, request=baseline['request'],
                             prior_view=baseline['prior_view'], response=r))

    def reject_case(name, q, s, prior):
        r = response(q, s)
        assert not response_ok(r, q, c, prior), name
        refusals.append(dict(name=name, config=cfg, request=q, prior_view=prior, response=r))
    reject_case('bootstrap-adopts-signed-in', request(c, 'snapshot', 1), snapshot(c), None)
    reject_case('login-running', request(c, 'login_fixture'), snapshot(c, running=True), None)
    reject_case('stop-running', request(c, 'stop'), snapshot(c, running=True), view(c))
    reject_case('logout-keeps-child', request(c, 'logout'), snapshot(c), view(c))
    replacement = snapshot(c)
    replacement['view'] = view(c, 'replacement')
    replacement['remote']['view'] = copy.deepcopy(replacement['view'])
    reject_case('snapshot-retargets-view', request(c, 'snapshot', 9, True), replacement, view(c))
    residual = snapshot(c, 'signed_out')
    residual['account']['id'] = c['expected_local']['recipient']['config']['account_id']
    reject_case('childless-retains-account', request(c, 'snapshot', 9), residual, None)
    null_error = response(request(c, 'snapshot', 9), error='busy')
    null_error['result'] = snapshot(c, 'signed_out')
    assert not response_ok(null_error, request(c, 'snapshot', 9), c, None)
    refusals.append(dict(name='error-carries-result', config=cfg, request=request(c, 'snapshot', 9), prior_view=None, response=null_error))

    # Exact raw grammar cases use one closed request, no future runtime output.
    rq = request(c, 'snapshot', 1)
    rb = raw(rq)
    variants = [
        ('missing-version', rb.replace(b'"version":1,', b'', 1)),
        ('duplicate-version', rb.replace(b'"version":1', b'"version":1,"version":1', 1)),
        ('reordered-fields', rb.replace(b'"version":1,"id":1', b'"id":1,"version":1', 1)),
        ('unknown-field', rb[:-1] + b',"token":"synthetic-forbidden"}'),
        ('null-field', rb.replace(b'"fixture":""', b'"fixture":null')),
        ('bool-number', rb.replace(b'"id":1', b'"id":true')),
        ('negative-number', rb.replace(b'"id":1', b'"id":-1')),
        ('fraction-number', rb.replace(b'"id":1', b'"id":1.0')),
        ('exponent-number', rb.replace(b'"id":1', b'"id":1e0')),
        ('unsafe-number', rb.replace(b'"id":1', b'"id":9007199254740992')),
        ('escaped-nonascii', rb.replace(b'"fixture":""', b'"fixture":"\\u00e9"')),
        ('escaped-control', rb.replace(b'"fixture":""', b'"fixture":"\\n"')),
        ('second-value', rb + b'{}'), ('request-oversize', rb + b' ' * (REQUEST_LIMIT + 1 - len(rb))),
    ]
    for name, b in variants:
        try:
            accepted = request_ok(decode(b, REQUEST_LIMIT), c)
        except (AssertionError, ValueError, TypeError):
            accepted = False
        assert not accepted, name
        raw_cases.append(dict(name=name, family='request', config=cfg, raw_hex=b.hex()))
    # Response wire must include even the null children and empty error.
    empty = next(x for x in positives if x['name'] == cfg + '-snapshot')
    eb = bytes.fromhex(empty['canonical_hex'])
    for name, b in [('omitted-null-view', eb.replace(b'"view":null,', b'', 1)),
                    ('omitted-null-remote', eb.replace(b'"remote":null,', b'', 1)),
                    ('response-oversize', eb + b' ' * (RESPONSE_LIMIT + 1 - len(eb)))]:
        try:
            accepted = response_ok(decode(b, RESPONSE_LIMIT), empty['request'], c, None)
        except (AssertionError, ValueError, TypeError):
            accepted = False
        assert not accepted, name
        raw_cases.append(dict(name=name, family='response', config=cfg, request=empty['request'], prior_view=None, raw_hex=b.hex()))

    # Framing and original-owner traces are literal model expectations. They
    # require concrete lifetime tests later; no opaque capability is fabricated.
    frames = [dict(name='single-lf', raw_hex=(rb + b'\n').hex(), accept=True),
              dict(name='missing-lf', raw_hex=rb.hex(), accept=False),
              dict(name='crlf', raw_hex=(rb + b'\r\n').hex(), accept=False),
              dict(name='blank', raw_hex=b'\n'.hex(), accept=False)]
    for f in frames:
        b = bytes.fromhex(f['raw_hex'])
        framed = b.endswith(b'\n') and b.count(b'\n') == 1 and b'\r' not in b
        try:
            accepted = framed and request_ok(decode(b[:-1], REQUEST_LIMIT), c)
        except (AssertionError, ValueError, TypeError):
            accepted = False
        assert accepted == f['accept']
    raw_acceptances = []
    for family, data, limit in [('request', rb, REQUEST_LIMIT), ('response', eb, RESPONSE_LIMIT)]:
        b = data + b' ' * (limit - len(data))
        x = decode(b, limit)
        assert request_ok(x, c) if family == 'request' else response_ok(x, empty['request'], c, None)
        raw_acceptances.append(dict(name=family + '-exact-bound', family=family, config=cfg,
                                    request=empty['request'], prior_view=None, raw_hex=b.hex()))
    traces = [dict(name=t['name'], steps=t['steps'], expected_states=trace_results(t)) for t in recipe['traces']]
    assert len(requests) == 12 and len(request_refusals) == 12 and len(positives) == 31
    assert len(refusals) == 47 and len(raw_cases) == 17 and len(frames) == 4 and len(traces) == 8
    return dict(version=1, scope='conditional independent wire composition; no live authority or effect claim',
                configs=[dict(name=n, config=c, config_id=config_id(c)) for n, c in cfgs.items()],
                requests=requests, request_refusals=request_refusals, positives=positives, refusals=refusals, raw_refusals=raw_cases,
                raw_acceptances=raw_acceptances, framing=frames, traces=traces)
