"""Fixed paid-fixture observations, independently bound to prior public proof."""
import time
from support import canonical, identifier, need

TXID = '2a475d6318ff194115024169265271221aea644a95c7d89e2782ec7492a1afa9'
RAW_SHA = '83889f90b7ffcee9d5ae8f283f1a9b74b214da27037549ac1f861abe8466eb8c'
BLOCK = '47a427c54e584cd90cfedc0f2918ab4251eee194bd93f2bdf6b8774159acf503'
ATOMS = 1980000000
HEIGHT = 165
GYM = '28452fc8a478d28f284dfe671cfc57e7cabeca08ea953aef371f5674d3f4c510'
FIXTURE = 'incrementing256'


def integer(v, lo, hi):
    return type(v) is int and lo <= v <= hi


def hexid(v):
    return type(v) is str and len(v) == 64 and set(v) <= set('0123456789abcdef') and v != '0' * 64


def decimal(v, lo=0, hi=(1 << 64) - 1):
    return type(v) is str and v.isascii() and v.isdigit() and str(int(v)) == v and lo <= int(v) <= hi


def snapshot(s, config, view=None, ready=False, signed_out=False):
    cid = identifier('SLUG pool issue recovery attachment', 'v1', config)
    local = config['base']['expected_local']
    recipient = local['recipient']
    need(tuple(s) == ('version', 'config_id', 'state', 'account', 'view', 'remote', 'error'), 'complete native snapshot')
    need(s['version'] == 2 and type(s['version']) is int and s['config_id'] == cid and s['error'] == '', 'same selected snapshot')
    if signed_out:
        need(s['state'] == s['account']['state'] == 'signed_out' and s['view'] is None and s['remote'] is None, 'joined signed-out state')
        return None
    v = s['view']
    need(s['state'] == s['account']['state'] == 'signed_in' and type(v) is dict, 'original signed account')
    need(tuple(v) == ('version', 'view_id', 'account_authority_id', 'account_id', 'generation', 'fixture', 'root_public_key', 'gym_public_key', 'gym_id'), 'original View shape')
    need(v['version'] == 1 and hexid(v['view_id']) and v['fixture'] == FIXTURE and v['gym_id'] == GYM and
         v['account_authority_id'] == recipient['config']['account_authority_id'] and
         v['account_id'] == recipient['config']['account_id'] and
         v['root_public_key'] == recipient['config']['identity']['root_public_key'] and
         v['gym_public_key'] == recipient['config']['identity']['gym_public_key'] and
         integer(v['generation'], 1, 9007199254740991), 'same original account/recipient View')
    if view is not None:
        need(v == view, 'original View changed during lifetime')
    need(s['account']['id'] == v['account_id'] and s['account']['generation'] == v['generation'] and s['account']['fixture'] == FIXTURE, 'account and View correlation')
    r = s['remote']
    need(r['version'] == 2 and r['config_id'] == cid and r['view'] == v and r['source_binding_id'] == config['base']['miners']['source_id'] and
         r['original_source_binding_id'] == local['original_source_binding_id'] and r['payout_address'] == recipient['recipient']['address'], 'same original remote wallet identity')
    need(r['state'] == r['worker_state'] == 'stopped' and r['last_error'] == '' and
         all(type(r[k]) is int and r[k] == 0 for k in ('attempts', 'hashes_per_second_milli', 'records', 'pending')), 'read-only empty attachment; worker never started')
    if ready:
        need(r['wallet_state'] == 'ready' and r['wallet_height'] == HEIGHT and r['spendable_atoms'] == ATOMS and
             r['immature_atoms'] == r['unconfirmed_atoms'] == 0, 'original ready paid wallet unchanged')
    return v


def binding(config, view):
    base = config['base']
    local = base['expected_local']
    return dict(view=view, config_id=identifier('SLUG pool issue recovery attachment', 'v1', config),
                source_binding_id=base['miners']['source_id'], settlement_config_id=identifier('SLUG operator pool settlement config', 'v2', base['settlement']),
                recipient_script=local['recipient']['recipient']['script'], issue_profile_id=identifier('SLUG pool issue recovery profile', 'v1', config['issue_profile']),
                original_source_binding_id=local['original_source_binding_id'], wallet_owner_id=local['wallet_owner_id'], allocation_binding_id=local['allocation_binding_id'])


def payout_read(r, config, view, prior, started_ms):
    need(tuple(r) == ('job', 'capture') and tuple(r['job']) == ('sequence', 'capture_sequence', 'before_batch', 'state', 'error'), 'exact payout read shape')
    need(r['job'] == dict(sequence='1', capture_sequence='0', before_batch='0', state='done', error=''), 'same completed payout operation')
    c = r['capture']
    need(tuple(c) == ('sequence', 'binding', 'captured_at_ms', 'stale', 'first', 'current') and c['sequence'] == '1' and
         canonical(c['binding']) == canonical(binding(config, view)) and c['stale'] is False and
         integer(c['captured_at_ms'], started_ms, time.time_ns() // 1000000), 'original completed capture binding')
    need(c['first'] == c['current'], 'one exact retained payout window')
    p = c['first']
    need(tuple(p) == tuple(prior) and p['version'] == 2 and decimal(p['revision'], 1) and p['high_batch'] == '1' and
         p['before_batch'] == '2' and p['next_before'] == '0' and p['done'] is True and
         integer(p['observed_at_ms'], prior['observed_at_ms'], time.time_ns() // 1000000), 'complete one-batch history')
    for key in ('binding_id', 'settlement_config_id', 'recipient_script', 'balance', 'items'):
        need(p[key] == prior[key], 'original payout history field changed: ' + key)
    need(p['balance']['paid_atoms'] == str(ATOMS) and p['balance']['reserved_atoms'] == '0' and
         len(p['items']) == 1 and p['items'][0]['batch_sequence'] == '1' and p['items'][0]['payment']['txid'] == TXID, 'original single paid output')
    return c


def observation(o, capture, started_ms):
    p = capture['first']
    need(o['capture_sequence'] == capture['sequence'] and o['binding'] == capture['binding'] and
         o['revision'] == p['revision'] and o['high_batch'] == p['high_batch'] and
         o['captured_at_ms'] == capture['captured_at_ms'] and o['settlement_observed_at_ms'] == p['observed_at_ms'] and
         o['stale'] is False and o['batch_sequence'] == '1' and o['payment'] == p['items'][0]['payment'], 'selected retained output correlation')
    need(o['state'] == 'confirmed' and o['error'] == '' and o['wallet_synced'] is True and
         o['wallet_height'] == HEIGHT and o['wallet_tip'] == BLOCK and o['confirmations'] == 1 and o['block_hash'] == BLOCK and
         integer(o['observed_at_ms'], started_ms, time.time_ns() // 1000000), 'original wallet independently observed confirmed output')


def activity_read(r, config, view, started_ms):
    need(tuple(r) == ('job', 'capture') and r['job'] == dict(sequence='1', state='done', error=''), 'same completed own-activity operation')
    c = r['capture']
    need(tuple(c) == ('sequence', 'binding', 'metadata') and c['sequence'] == '1' and
         canonical(c['binding']) == canonical(binding(config, view)), 'activity belongs to original complete View binding')
    m = c['metadata']
    need(tuple(m) == ('version', 'captured_at_ms', 'wallet_height', 'wallet_tip', 'node_height', 'node_tip', 'count', 'has_older'), 'activity capture metadata shape')
    need(m['version'] == 1 and m['wallet_height'] == m['node_height'] == HEIGHT and m['wallet_tip'] == m['node_tip'] == BLOCK and
         integer(m['captured_at_ms'], started_ms, time.time_ns() // 1000000) and integer(m['count'], 1, 256) and type(m['has_older']) is bool and
         (not m['has_older'] or m['count'] == 256), 'dated original synced activity capture')
    return c


def activity_page(p, capture, after):
    need(tuple(p) == ('capture', 'after', 'limit', 'next', 'has_more', 'items') and p['capture'] == capture, 'same retained activity page capture')
    count = capture['metadata']['count']
    end = min(after + 8, count)
    need(p['after'] == after and p['limit'] == 8 and p['next'] == end and p['has_more'] is (end < count) and
         type(p['items']) is list and len(p['items']) == end - after, 'exact captured activity positions')
    return end


def paid_item(item):
    need(item['txid'] == TXID and item['raw_sha256'] == RAW_SHA and item['coinbase'] is False and
         item['input_count'] == item['output_count'] == 2 and item['owned_input_count'] == 0 and item['owned_output_count'] == 1 and
         item['received_atoms'] == item['net_atoms'] == ATOMS and item['spent_atoms'] == item['change_atoms'] == 0 and item['fee_atoms'] is None and
         item['block']['height'] == HEIGHT and item['block']['hash'] == BLOCK and item['confirmations'] == 1, 'exact original incoming raw payment activity')


def paid_detail(d, capture, item, config):
    need(tuple(d) == ('capture', 'item', 'outputs') and d['capture'] == capture and d['item'] == item, 'detail reparses same retained raw item')
    outputs = d['outputs']
    need(type(outputs) is list and len(outputs) == 2, 'two original payment outputs')
    need(outputs[0] == dict(index=0, atoms=ATOMS, owned=True, change=False, address=config['base']['expected_local']['recipient']['recipient']['address']), 'original payee output')
    need(outputs[1]['index'] == 1 and outputs[1]['atoms'] == 19999791 and outputs[1]['owned'] is False and outputs[1]['change'] is False, 'original external change output')
    need(sum(x['atoms'] for x in outputs) == 2000000000 - 209, 'original raw total and fee unchanged')
