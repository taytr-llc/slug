"""Independent finite Set73 downstream compositions, from frozen prose/old inputs.
No Go imports or runtime output. This module is imported only after the caller
verifies an approved source/input lock. Result-source witnesses are conditional
byte compositions, not complete owned bookings, simulation or finality proofs.
"""
import copy
import hashlib
import json
import struct


def canonical(value):
    return json.dumps(value, ensure_ascii=False, separators=(',', ':'), allow_nan=False).replace('&', '\\u0026').replace('<', '\\u003c').replace('>', '\\u003e').replace('\u2028', '\\u2028').replace('\u2029', '\\u2029').encode('utf-8')


def sha(raw): return hashlib.sha256(raw).hexdigest()
def raw(value): return bytes.fromhex(value)
def u32(value): return struct.pack('<I', value)
def u64(value): return struct.pack('<Q', value)
def lp(value): return u32(len(value)) + value
def old_domain(label): return ('SLUG ' + label + '\0v1\0').encode('ascii')
def preimage(label, value): return old_domain('owned ' + label) + lp(canonical(value))
def digest(label, value): return sha(preimage(label, value))
def old_digest(label, value): return sha(old_domain(label) + lp(canonical(value)))
def clone(value): return copy.deepcopy(value)


class Refused(ValueError): pass


def require(condition, reason):
    if not condition: raise Refused(reason)


def exact(value, keys):
    require(type(value) is dict and list(value) == keys.split(','), 'closed ordered fields')
    return value


def integer(value, low, high):
    require(type(value) is int and low <= value <= high, 'integer')


def hex64(value):
    require(type(value) is str and len(value) == 64 and all(c in '0123456789abcdef' for c in value), 'hash')


def rules():
    seed = dict(version=1, target='first_anchor_next_block', floor='genesis_inclusive', carrier='SLUGSEED-v1', finality='exact_target_checkpoint_60', max_step_blocks=8, max_scan_restarts=16, fee_satoshis=1000)
    card = dict(version=1, scope='two_fixture_candidate', record_basis='candidate_zero_record_no_results_authority', classes=['light','welter','middle','light_heavy','heavy'], band_min_numerator=1, band_min_denominator=2, band_max_numerator=2, band_max_denominator=1, widen_to_five=False, condition_version=1, rating='D65', cutoff_eligibility=True, max_entrants=10, max_bouts=5)
    bout = dict(version=1, scope='two_fixture_booked_candidate_bout', record_basis=card['record_basis'], card_rules_id=old_digest('candidate card rules',card), engine_version=1, engine_id='slug-fighter-bout-candidate-v1', mode='standard', seed_kind='explicit', rounds=3, as_of='event_cutoff', seed='booked_bout_seed', pro_xp=0, training_skill=0, new_run_requires_usable_card=True, saved_replay_immutable=True)
    result = dict(version=1, source='saved_c1_bout_v1', carrier='SLUGRSLT-v1', fund='fixture_key2_p2wpkh', inclusion='exact_local_tx', pre_checkpoint_reorg='same_tx_reinclude', finality='carrier_checkpoint_60', max_step_blocks=8, max_scan_restarts=16, max_prior_inclusions=16, fee_satoshis=1000, awards='none')
    return dict(seed=seed,card=card,bout=bout,result=result)


def assemble_pins(event_pin, recipe, crypto):
    physical = rules()
    entries = event_pin['participants']
    zero = [x['stable_pin']['stable'] for x in entries if x['stable_pin']['stable']['identity']['gym_id'] == recipe['fixture_publisher_gym']]
    require(len(zero) == 1, 'original zero fixture publisher')
    zero = zero[0]; source = zero['source']; event_id = digest('event store', event_pin)
    chain = sha(old_domain('candidate publication chain') + b''.join(raw(x) for x in [event_id,zero['account_id'],source['authority_id'],source['genesis']]))
    seeds = {x: hashlib.sha256(old_domain(x)).digest() for x in recipe['fixture_seeds']}
    def publication_pin(family, source_pin, source_id, scalar, label, checkpoint):
        pub = crypto.pubkey(scalar)
        return dict(version=1,source_store_id=source_id,source_pin=clone(source_pin),publisher_gym_id=zero['identity']['gym_id'],publisher_account_id=zero['account_id'],publisher_source_authority_id=source['authority_id'],publisher_chain_id=chain,network=source['network'],genesis=source['genesis'],floor_height=0,floor_hash=source['genesis'],publisher_public_key=crypto.ed_public(seeds[label]).hex(),checkpoint_public_key=crypto.ed_public(seeds[checkpoint]).hex(),funding_public_key=pub.hex(),funding_script=(b'\x00\x14'+crypto.hash160(pub)).hex(),rules_id=old_digest('candidate '+family+' rules',physical[family]))
    seed = publication_pin('seed',event_pin,event_id,1,'candidate seed declaration fixture','candidate seed checkpoint fixture')
    card = dict(version=1,source_store_id=digest('seed store',seed),source_pin=clone(seed),rules_id=old_digest('candidate card rules',physical['card']))
    bout = dict(version=1,source_store_id=digest('card store',card),source_pin=clone(card),rules_id=old_digest('candidate bout rules',physical['bout']))
    result = publication_pin('result',bout,digest('bout store',bout),2,'candidate result statement fixture','candidate result checkpoint fixture')
    return dict(seed=seed,card=card,bout=bout,result=result), physical, seeds


def make_receipt(snapshot, offset):
    cutoff = snapshot['event']['cutoff_ms']
    cid = old_digest('fighter ownership cutoff id',snapshot['cutoff'])
    captures=[]
    for p in snapshot['participants']:
        def fence(store,count):return dict(version=1,store_id=store,cutoff_ms=cutoff,observed_at_ms=cutoff+offset,operation_count=count)
        captures.append(dict(version=1,participant_id=p['participant_id'],stable_fence=fence(p['roster']['store_id'],p['roster']['last_sequence']),entry_fence=fence(p['entry_store_id'],p['entry']['last_sequence']),cutoff_id=cid,captured_at_ms=cutoff+offset))
    return dict(version=1,input=clone(snapshot),snapshot_digest=digest('event snapshot',snapshot),captures=captures,completed_at_ms=cutoff+offset)


def declaration(pin,receipt):
    event=receipt['input']['event']; store=digest('seed store',pin)
    event_key=digest('seed event',dict(version=1,store_id=store,event_id=event['event_id']))
    return dict(version=1,event_key=event_key,store_id=store,source_store_id=pin['source_store_id'],event=clone(event),snapshot_digest=receipt['snapshot_digest'],receipt_digest=digest('event receipt',receipt),publisher_chain_id=pin['publisher_chain_id'],network=pin['network'],genesis=pin['genesis'],floor_height=0,floor_hash=pin['genesis'],rules_id=pin['rules_id'],publisher_public_key=pin['publisher_public_key'],checkpoint_public_key=pin['checkpoint_public_key'])


def sign_body(label, body, seed, crypto, field):
    message=raw(digest(label+' signature',body))
    signature=crypto.ed_sign(seed,message)
    require(crypto.ed_verify(crypto.ed_public(seed),message,signature),'independent sign verification')
    return dict(version=1,**{field:clone(body)},signature=signature.hex())


def initial_cursor(family,store,key,genesis):
    label='candidate seed scan' if family=='seed' else 'candidate result inclusion scan'
    initial=sha(old_domain(label)+raw(store)+raw(key)+u32(0)+u32(0)+raw(genesis))
    return dict(generation=0,height=-1,hash='',scan_digest=initial,complete_to_height=-1,complete_to_hash='')


def prepared_seed(pin,receipt,signed):
    body=signed['declaration']
    return dict(version=1,store_id=body['store_id'],event_key=body['event_key'],source_receipt=clone(receipt),declaration=clone(signed),prepared_at_ms=receipt['completed_at_ms']+1,publication=None,cursor=initial_cursor('seed',body['store_id'],body['event_key'],pin['genesis']),scan_restarts=[],anchor=None,target=None,checkpoint=None,finality=None,conflicts=[],conflict_overflow=False,incident_at_ms=0,observation=dict(observed_at_ms=0,chain_observed_at_ms=0,tip=None,node_state='unobserved',target_descendants=0,current_finality=False))


def card_ids(pin,event,snapshot_digest,event_key,height,target):
    card=digest('card',dict(version=1,store_id=digest('card store',pin),event_id=event['event_id']))
    body=dict(version=1,card_id=card,snapshot_digest=snapshot_digest,seed_event_key=event_key,target_height=height,target_hash=target,rules_id=pin['rules_id'])
    return card,body,digest('card input',body)


def conditional_result_source(pin,old,receipt,recipe,ids,upstream):
    """Complete ordered source shape, ONLY for signature/domain composition.

    The old physical source/result is preserved as an input. Rebinding identities
    here is an explicit conditional model, not an assertion that its old bout
    remains a usable owned booking or that a simulation was rerun for new IDs.
    The generated corpus does NOT list this as a positive semantic parser case.
    """
    card,card_input=ids; src=clone(old['source']); cutoff=receipt['input']['event']['cutoff_ms']
    src['store_id']=pin['source_store_id'];src['card_id']=card
    src['card_record_digest']=sha(b'Set73 conditional card record bytes')
    src['card_source_digest']=sha(b'Set73 conditional card source bytes')
    src['card_created_at_ms']=cutoff+15;src['event']=clone(receipt['input']['event'])
    src['snapshot_digest']=receipt['snapshot_digest'];src['seed_event_key']=declaration(pin['source_pin']['source_pin'],receipt)['event_key']
    src['target_height']=recipe['target_height'];src['target_hash']=recipe['target_hash'];src['finalized_at_ms']=cutoff+14
    cls=src['bout']['class']; a,b=sorted(x['member']['assignment']['prospect_id']for x in src['corners'])
    plan_seed=sha(old_domain('candidate card seed')+raw(card_input)+raw(recipe['target_hash']))
    bout_id=sha(old_domain('candidate card bout')+raw(card_input)+u32(['light','welter','middle','light_heavy','heavy'].index(cls))+raw(a)+raw(b))
    src['bout']=dict(bout_id=bout_id,**{'class':cls},corner_a=a,corner_b=b,seed=sha(old_domain('candidate card bout seed')+raw(plan_seed)+raw(bout_id)))
    # New public type shape, old immutable corner facts. The synthetic state is
    # deliberately not presented as an authenticated core output for these IDs.
    state_template=clone(upstream['contexts']['v1/zero256-unawarded/initial']['state'])
    corners=[]
    for old_corner in src['corners']:
        m=old_corner['member'];birth=clone(m['assignment']);state=clone(state_template)
        state['prospect_id']=birth['prospect_id'];state['owner_public_key']=next(ep['stable_pin']['stable']['identity']['gym_public_key']for ep in pin['source_pin']['source_pin']['source_pin']['participants'] if ep['stable_pin']['stable']['identity']['gym_id']==old_corner['owner']['gym_id'])
        owner=dict(ownership_basis='finalized_public_chain',account_authority_id=old_corner['owner']['account_authority_id'],account_id=old_corner['owner']['account_id'],gym_id=old_corner['owner']['gym_id'])
        member=dict(slot=m['slot'],slot_assigned_at_ms=m['slot_assigned_at_ms'],birth=birth,ownership=state,**{'class':m['class']},condition_at_cutoff=clone(m['condition_at_cutoff']),eligibility=m['eligibility'])
        corners.append(dict(owner=owner,member=member))
    src['corners']=sorted(corners,key=lambda c: 0 if c['member']['birth']['prospect_id']==a else 1)
    return dict(version=1,store_id=digest('bout store',pin),store_created_at_ms=cutoff+recipe['source_created_offset_ms'],record_digest=sha(b'Set73 conditional bout record bytes'),source_digest=digest('card bout source',src),input_id=old['input_id'],result_digest=old['result_digest'],created_at_ms=cutoff+recipe['source_created_offset_ms']+1,source=src,summary={k:clone(old['result'][k])for k in ['winner','method','end_round','end_tick']})


def result_statement(pin,source):
    store=digest('result store',pin)
    key=digest('result key',dict(version=1,store_id=store,bout_id=source['source']['bout']['bout_id']))
    return dict(version=1,result_key=key,store_id=store,source_store_id=pin['source_store_id'],source=clone(source),publisher_chain_id=pin['publisher_chain_id'],network=pin['network'],genesis=pin['genesis'],rules_id=pin['rules_id'],publisher_public_key=pin['publisher_public_key'],checkpoint_public_key=pin['checkpoint_public_key'])


def career(pin,cutoff,recipe):
    policy=dict(version=1,scope='two_fixture_standard_career',source='result_v1_first_finality_after_activation',xp='D42_standard_1_cap15',base_points='D41_no_quality',effective_time='first_finality_observed_at_ms',cutoff='strict_before',later_conflict='retain',c1='zero_professional_record_only')
    pid=old_digest('candidate career policy',policy);store=digest('result store',pin)
    marker=dict(format='slug-owned-result',version=1,store_id=store,pin=clone(pin))
    source_time=cutoff+recipe['source_created_offset_ms'];result_time=cutoff+recipe['result_created_offset_ms'];activation_time=cutoff+recipe['activation_offset_ms']
    intent=dict(version=1,policy_id=pid,result_store_id=store,result_created_at_ms=result_time,source_created_at_ms=source_time,old_marker_sha256=sha(canonical(marker)+b'\n'))
    aid=sha(old_domain('candidate career activation')+raw(pid)+raw(store)+u64(result_time)+u64(source_time)+u64(activation_time))
    activation=dict(version=1,policy_id=pid,activation_id=aid,result_store_id=store,result_created_at_ms=result_time,source_created_at_ms=source_time,activated_at_ms=activation_time)
    prepared=dict(format='slug-owned-result',version=2,store_id=store,pin=clone(pin),career_phase='prepared',career_intent_digest=sha(canonical(intent)),career=None)
    ready=clone(prepared);ready['career_phase']='ready';ready['career']=activation
    return dict(policy=policy,policy_id=pid,v1=marker,intent=intent,prepared=prepared,ready=ready)


def verify_career(pin,value,recipe,cutoff):
    expected=career(pin,cutoff,recipe)
    require(canonical(value)==canonical(expected),'exact career prefix/activation binding')
    return clone(value)


def validate_pin(family,value,expected):
    exact(value,','.join(expected[family]))
    require(canonical(value)==canonical(expected[family]),'exact upstream/candidate pin')
    return clone(value)


def verify_signed(label,body_field,value,pub,crypto):
    exact(value,'version,'+body_field+',signature');integer(value['version'],1,1)
    require(type(value['signature'])is str and len(value['signature'])==128,'signature shape')
    try: signature=bytes.fromhex(value['signature'])
    except ValueError: raise Refused('signature hex')
    require(crypto.ed_verify(raw(pub),raw(digest(label+' signature',value[body_field])),signature),'signature/domain')
    return clone(value)
