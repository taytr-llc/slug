"""Independent Set70 closed-wire model. SOURCE ONLY until root producer lock.

Only Python stdlib and explicitly supplied locked public input values are used.
This models wire shape/correlation; it cannot authorize an account or prove that
synthetic item facts came from a wallet, raw transaction, node or capture.
"""
import copy
import hashlib
import json
import re

MAX_SAFE = 9007199254740991
MAX_MONEY = 2100000000000000
MAX_MS = 253402300799999
MAX_HEIGHT = 2147483647
COMMANDS = ('snapshot', 'begin', 'job', 'page', 'detail', 'cancel')
ERRORS = ('invalid_request', 'unauthorized', 'stale_view', 'busy', 'capacity',
          'activity_unavailable', 'activity_not_found', 'operation_conflict', 'outcome_unknown')
REQUEST_FIELDS = 'id version command view_id job_id after limit txid'.split()
RESPONSE_FIELDS = 'id version ok snapshot job page detail error'.split()
VIEW_FIELDS = 'version view_id account_authority_id account_id generation authority_pin authority_origin transport_trust_id'.split()
PIN_FIELDS = 'version network genesis authority_id descriptor_id'.split()
BINDING_FIELDS = 'view source_binding_id root_public_key gym_public_key gym_id'.split()
CAPTURE_FIELDS = 'version captured_at_ms wallet_height wallet_tip node_height node_tip count has_older'.split()
BLOCK_FIELDS = 'height hash time_ms'.split()
ITEM_FIELDS = 'txid raw_sha256 recorded_at_ms coinbase block confirmations received_atoms spent_atoms change_atoms net_atoms fee_atoms input_count output_count owned_input_count owned_output_count'.split()
OUTPUT_FIELDS = 'index atoms owned change address'.split()
SNAPSHOT_FIELDS = 'version state error binding activity active_job_id last_job_id capture_job_id'.split()
JOB_FIELDS = 'version job_id state binding error capture'.split()
PAGE_FIELDS = 'version binding job_id capture after limit next has_more items'.split()
DETAIL_FIELDS = 'version binding job_id capture item outputs'.split()
SCHEMAS = (REQUEST_FIELDS, RESPONSE_FIELDS, VIEW_FIELDS, PIN_FIELDS, BINDING_FIELDS,
           CAPTURE_FIELDS, BLOCK_FIELDS, ITEM_FIELDS, OUTPUT_FIELDS, SNAPSHOT_FIELDS,
           JOB_FIELDS, PAGE_FIELDS, DETAIL_FIELDS)


class Invalid(ValueError):
    pass


def require(ok, reason):
    if not ok:
        raise Invalid(reason)


def obj(value, fields):
    require(type(value) is dict and set(value) == set(fields), 'exact object fields')


def integer(value, low=0, high=MAX_SAFE):
    require(type(value) is int and low <= value <= high, 'bounded integer')


def boolean(value):
    require(type(value) is bool, 'boolean')


def string(value, maximum):
    require(type(value) is str, 'string')
    try:
        raw = value.encode('utf-8', 'strict')
    except UnicodeError as exc:
        raise Invalid('invalid string Unicode') from exc
    require(len(raw) <= maximum, 'bounded string')


def hex64(value, nonzero=False):
    require(type(value) is str and re.fullmatch('[0-9a-f]{64}', value) is not None, 'lowercase hash')
    require(not nonzero or value != '0' * 64, 'nonzero identifier')


def optional_id(value):
    if value != '':
        hex64(value, True)


def version(value):
    require(type(value) is int and value == 1, 'version')


def clone(value):
    return copy.deepcopy(value)


def ordered(value):
    if type(value) is dict:
        fields = next((s for s in SCHEMAS if set(s) == set(value)), None)
        require(fields is not None, 'canonical object schema')
        return {key: ordered(value[key]) for key in fields}
    if type(value) is list:
        return [ordered(item) for item in value]
    return value


def compact(value):
    # All valid strings in this model are ASCII. Match Go's standard HTML
    # escaping too, so malformed string mutations have explicit stable bytes.
    result = json.dumps(value, ensure_ascii=False, separators=(',', ':'), allow_nan=False)
    for literal, escaped in [('&', '\\u0026'), ('<', '\\u003c'), ('>', '\\u003e'),
                             ('\u2028', '\\u2028'), ('\u2029', '\\u2029')]:
        result = result.replace(literal, escaped)
    return result.encode('utf-8')


def frame(value, canonical=True):
    return compact(ordered(value) if canonical else value) + b'\n'


def validate_binding(value, allowed):
    obj(value, BINDING_FIELDS)
    view = value['view']; obj(view, VIEW_FIELDS); version(view['version'])
    for key in ('view_id', 'account_authority_id', 'account_id', 'transport_trust_id'):
        hex64(view[key])
    integer(view['generation'], 1)
    pin = view['authority_pin']; obj(pin, PIN_FIELDS)
    require(type(pin['version']) is int and pin['version'] == 5, 'manifest authority v5')
    integer(pin['network'], 1, 4294967295)
    for key in ('genesis', 'authority_id', 'descriptor_id'):
        hex64(pin[key])
    string(view['authority_origin'], 256)
    # Inherited trust/descriptor grammar and the two permitted public identities
    # are pinned by old independent input, not regenerated from new Go code.
    require(any(view['authority_origin'] == b['view']['authority_origin'] and
                view['transport_trust_id'] == b['view']['transport_trust_id'] and
                pin == b['view']['authority_pin'] for b in allowed), 'inherited origin/trust/pin')
    hex64(value['source_binding_id'])
    string(value['root_public_key'], 128); string(value['gym_public_key'], 66); hex64(value['gym_id'])
    require(any(tuple(value[k] for k in ('root_public_key', 'gym_public_key', 'gym_id')) ==
                tuple(b[k] for k in ('root_public_key', 'gym_public_key', 'gym_id'))
                for b in allowed), 'published canonical root/gym tuple')


def validate_capture(value):
    obj(value, CAPTURE_FIELDS); version(value['version'])
    integer(value['captured_at_ms'], 1, MAX_MS)
    for k in ('wallet_height', 'node_height'):
        integer(value[k], 0, MAX_HEIGHT)
    for k in ('wallet_tip', 'node_tip'):
        hex64(value[k], True)
    require(value['wallet_height'] == value['node_height'] and value['wallet_tip'] == value['node_tip'], 'same captured tips')
    integer(value['count'], 0, 256); boolean(value['has_older'])
    require(not value['has_older'] or value['count'] == 256, 'older witness count')


def validate_item(value, capture):
    obj(value, ITEM_FIELDS)
    hex64(value['txid']); hex64(value['raw_sha256'])
    integer(value['recorded_at_ms'], 0, MAX_MS); boolean(value['coinbase'])
    integer(value['confirmations'], 0, MAX_HEIGHT + 1)
    for k in ('received_atoms', 'spent_atoms', 'change_atoms'):
        integer(value[k], 0, MAX_MONEY)
    integer(value['net_atoms'], -MAX_MONEY, MAX_MONEY)
    for k in ('input_count', 'output_count'):
        integer(value[k], 1, 256)
    integer(value['owned_input_count'], 0, value['input_count'])
    integer(value['owned_output_count'], 0, value['output_count'])
    require(value['owned_input_count'] + value['owned_output_count'] > 0, 'at least one recorded own index')
    c, d, k = (value[x] for x in ('received_atoms', 'spent_atoms', 'change_atoms'))
    require(k <= c and value['net_atoms'] == c-d, 'exact own atom formula')
    require(value['owned_input_count'] != 0 or d == 0, 'zero own inputs means zero debit')
    require(value['owned_output_count'] != 0 or c == k == 0, 'zero own outputs means zero credit/change')
    if value['block'] is None:
        require(value['confirmations'] == 0, 'unmined confirmations')
    else:
        block = value['block']; obj(block, BLOCK_FIELDS)
        integer(block['height'], 0, capture['wallet_height']); hex64(block['hash'], True)
        integer(block['time_ms'], 0, MAX_MS)
        require(value['confirmations'] == capture['wallet_height']-block['height']+1, 'captured confirmations')
    unknown_fee = value['coinbase'] or value['owned_input_count'] < value['input_count']
    require((value['fee_atoms'] is None) == unknown_fee, 'fee knowledge, not zero default')
    if not unknown_fee:
        integer(value['fee_atoms'], 0, d)
    if value['coinbase']:
        require(value['input_count'] == 1 and value['owned_input_count'] == 0 and d == 0, 'coinbase has no debit')


def item_order(value):
    block = value['block']
    return (0, -value['recorded_at_ms'], value['txid']) if block is None else (1, -block['height'], value['txid'])


def validate_snapshot(value, allowed):
    obj(value, SNAPSHOT_FIELDS); version(value['version'])
    for key in ('active_job_id', 'last_job_id', 'capture_job_id'):
        optional_id(value[key])
    if value['state'] == 'unavailable':
        require(value['error'] == 'activity_unavailable' and value['binding'] is None and
                value['activity'] == 'none' and not any(value[k] for k in ('active_job_id', 'last_job_id', 'capture_job_id')), 'blank unavailable snapshot')
        return
    validate_binding(value['binding'], allowed); require(value['error'] == '', 'available snapshot error')
    require(value['state'] in ('idle', 'busy'), 'snapshot state')
    if value['state'] == 'idle':
        require(value['activity'] == 'none' and value['active_job_id'] == '', 'idle activity')
    else:
        require(value['activity'] in ('capture', 'detail'), 'busy activity')
        hex64(value['active_job_id'], True)
        if value['activity'] == 'detail':
            require(value['active_job_id'] == value['capture_job_id'], 'detail uses retained capture')


def validate_job(value, allowed):
    obj(value, JOB_FIELDS); version(value['version']); hex64(value['job_id'], True)
    validate_binding(value['binding'], allowed)
    if value['state'] == 'done':
        require(value['error'] == '', 'done error'); validate_capture(value['capture'])
    elif value['state'] == 'failed':
        require(value['capture'] is None and value['error'] in ('busy', 'capacity', 'activity_unavailable'), 'failed job fields')
    else:
        require(value['state'] in ('running', 'cancelled') and value['error'] == '' and value['capture'] is None, 'nonterminal/cancelled job fields')


def validate_page(value, allowed):
    obj(value, PAGE_FIELDS); version(value['version']); hex64(value['job_id'], True)
    validate_binding(value['binding'], allowed); validate_capture(value['capture'])
    count = value['capture']['count']
    integer(value['after'], 0, count); integer(value['limit'], 1, 8)
    integer(value['next'], 0, count); boolean(value['has_more'])
    require(type(value['items']) is list and len(value['items']) == min(value['limit'], count-value['after']), 'exact local page length')
    require(value['next'] == value['after']+len(value['items']) and value['has_more'] == (value['next'] < count), 'local page cursor')
    for item in value['items']:
        validate_item(item, value['capture'])
    require(len({x['txid'] for x in value['items']}) == len(value['items']), 'unique page txids')
    require(value['items'] == sorted(value['items'], key=item_order), 'captured item order')


def validate_detail(value, allowed, addresses):
    obj(value, DETAIL_FIELDS); version(value['version']); hex64(value['job_id'], True)
    validate_binding(value['binding'], allowed); validate_capture(value['capture'])
    item = value['item']; validate_item(item, value['capture'])
    require(type(value['outputs']) is list and len(value['outputs']) == item['output_count'], 'exact output description count')
    c = k = total = owned = 0
    for index, output in enumerate(value['outputs']):
        obj(output, OUTPUT_FIELDS); integer(output['index'], index, index)
        integer(output['atoms'], 0, MAX_MONEY); boolean(output['owned']); boolean(output['change'])
        require(not output['change'] or output['owned'], 'change is owned')
        string(output['address'], 90)
        # Addresses are copied from old independent standard-script vectors;
        # this wire producer deliberately does not create a new script oracle.
        require(output['address'] == '' or output['address'] in addresses, 'locked canonical address')
        total += output['atoms']
        if output['owned']:
            c += output['atoms']; owned += 1
        if output['change']:
            k += output['atoms']
    require(total <= MAX_MONEY and c == item['received_atoms'] and k == item['change_atoms'] and owned == item['owned_output_count'], 'detail ownership/amount sums')
    if item['fee_atoms'] is not None:
        require(total+item['fee_atoms'] == item['spent_atoms'], 'known exact fee')


def validate_request(value):
    obj(value, REQUEST_FIELDS); integer(value['id'], 1); version(value['version'])
    require(value['command'] in tuple('wallet_activity_'+x for x in COMMANDS), 'closed command')
    integer(value['after'], 0, 256); integer(value['limit'], 0, 8)
    if value['command'] == 'wallet_activity_snapshot':
        require(value['view_id'] == value['job_id'] == value['txid'] == '' and value['after'] == value['limit'] == 0, 'snapshot zeros')
        return
    hex64(value['view_id']); hex64(value['job_id'], True)
    if value['command'] == 'wallet_activity_page':
        require(value['txid'] == '' and value['limit'] >= 1, 'page selectors')
    elif value['command'] == 'wallet_activity_detail':
        hex64(value['txid']); require(value['after'] == value['limit'] == 0, 'detail zeros')
    else:
        require(value['txid'] == '' and value['after'] == value['limit'] == 0, 'job command zeros')


def validate_response(value, allowed, addresses):
    obj(value, RESPONSE_FIELDS); integer(value['id'], 1); version(value['version']); boolean(value['ok'])
    arms = [key for key in ('snapshot', 'job', 'page', 'detail') if value[key] is not None]
    if not value['ok']:
        require(not arms and value['error'] in ERRORS, 'error has no payload')
        return
    require(len(arms) == 1 and value['error'] == '', 'exactly one success arm')
    arm = arms[0]
    if arm == 'snapshot': validate_snapshot(value[arm], allowed)
    elif arm == 'job': validate_job(value[arm], allowed)
    elif arm == 'page': validate_page(value[arm], allowed)
    else: validate_detail(value[arm], allowed, addresses)


def correlate(kind, value, context):
    expected = context['expected_binding']
    if kind == 'request':
        if value['command'] != 'wallet_activity_snapshot':
            require(value['view_id'] == expected['view']['view_id'], 'original request view')
        return
    request = context['request']
    require(value['id'] == request['id'], 'request id correlation')
    if not value['ok']:
        return
    arm = {'snapshot': 'snapshot', 'begin': 'job', 'job': 'job', 'cancel': 'job', 'page': 'page', 'detail': 'detail'}[request['command'].removeprefix('wallet_activity_')]
    require(value[arm] is not None, 'command response arm')
    payload = value[arm]
    if arm == 'snapshot':
        if payload['binding'] is not None:
            require(payload['binding'] == expected, 'original snapshot binding')
        for key in ('active_job_id', 'last_job_id', 'capture_job_id'):
            require(not payload[key] or payload[key] in context.get('known_job_ids', []), 'native-known snapshot IDs only')
        if 'expected_active_job_id' in context:
            require(payload['active_job_id'] == context['expected_active_job_id'], 'exact native action recovery')
        return
    require(payload['binding'] == expected and payload['job_id'] == request['job_id'], 'original binding/job')
    capture = context.get('capture')
    if capture is not None and arm in ('page', 'detail'):
        require(payload['capture'] == capture, 'same immutable capture')
    if arm == 'page':
        require(payload['after'] == request['after'] and payload['limit'] == request['limit'], 'original page request')
        if 'captured_items' in context:
            require(payload['items'] == context['captured_items'][request['after']:payload['next']], 'exact captured page slice')
    if arm == 'detail':
        require(payload['item']['txid'] == request['txid'], 'exact selected txid')
        require(payload['item'] == context['selected_item'], 'exact original selected row')


def _pairs(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, 'duplicate JSON key')
        result[key] = value
    return result


def _integer(text):
    require(text != '-0' and re.fullmatch('-?(0|[1-9][0-9]*)', text) is not None, 'canonical integer token')
    value = int(text); integer(value, -MAX_SAFE, MAX_SAFE); return value


def _not_number(text):
    raise Invalid('fraction/exponent/nonfinite number')


def _depth(value, current=1):
    if type(value) is dict:
        return max([current]+[_depth(x, current+1) for x in value.values() if type(x) in (dict, list)])
    if type(value) is list:
        return max([current]+[_depth(x, current+1) for x in value if type(x) in (dict, list)])
    return current


def parse(raw, kind, allowed, addresses, context=None):
    require(type(raw) is bytes and len(raw) <= (1024 if kind == 'request' else 98304), 'frame byte bound')
    try:
        value = json.loads(raw.decode('utf-8', 'strict'), object_pairs_hook=_pairs,
                           parse_int=_integer, parse_float=_not_number, parse_constant=_not_number)
    except (UnicodeError, ValueError, RecursionError) as exc:
        raise Invalid('invalid raw JSON') from exc
    require(_depth(value) <= (3 if kind == 'request' else 8), 'token depth')
    if kind == 'request':
        validate_request(value)
    else:
        require(kind == 'response', 'wire kind')
        validate_response(value, allowed, addresses)
        require(value['detail'] is not None or len(raw) <= 16384, 'small-arm frame bound')
    if context is not None:
        correlate(kind, value, context)
    return value


def accepts(raw, kind, allowed, addresses, context=None):
    try:
        parse(raw, kind, allowed, addresses, context)
        return True
    except Invalid:
        return False


def format_atoms(atoms):
    integer(atoms, -MAX_MONEY, MAX_MONEY)
    sign = '-' if atoms < 0 else ''
    whole, fraction = divmod(abs(atoms), 100000000)
    return '%s%d.%08d' % (sign, whole, fraction)
