#!/usr/bin/env python3
"""Guarded independent public ownership reference; no model import before approval."""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import struct
import sys

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
SOURCE_NAMES = {'core_model.py', 'generate_reference.py', 'case-plan.json', 'README.md'}


def need(value, message):
    if not value:
        raise RuntimeError(message)


def file_sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def check_inputs(root, catalog):
    expected = {}
    for row in catalog['files']:
        rel = Path(row['path'])
        need(not rel.is_absolute() and '..' not in rel.parts, 'input path')
        path = root / rel
        need(path.is_file() and not path.is_symlink(), 'input type')
        need(path.stat().st_size == row['bytes'] and file_sha(path) == row['sha256'], 'input bytes')
        need(row['path'] not in expected, 'duplicate input')
        expected[row['path']] = row['sha256']
    need(not any(p.is_symlink() for p in root.rglob('*')), 'input symlink')
    actual = {str(p.relative_to(root)) for p in root.rglob('*') if p.is_file()}
    need(actual == set(expected), 'exact input set')
    return expected


def produce(M, E, root, recipe):
    E.selftest()  # Archived Set45 RFC8032/secp primitive expectations, not Go73.
    model = M.Model(E, recipe)
    positives, negatives, raw_cases, state_cases, storage_cases = [], [], [], [], []

    def positive(family, name, value, ids=None, origin='modeled public fixture'):
        row = dict(family=family, name=name, origin=origin, value=copy.deepcopy(value),
                   canonical_hex=M.J(value).hex(), canonical_sha256=M.sha(M.J(value)))
        if ids is not None:
            row['ids'] = ids
        positives.append(row)
        return row

    def reject(family, name, value, check, expected='invalid'):
        try:
            check(value)
        except M.Refused as error:
            negatives.append(dict(family=family, name=name, value=copy.deepcopy(value),
                                  expected=expected, reason=str(error)))
        else:
            raise RuntimeError('negative accepted: ' + name)

    def raw_case(name, kind, raw, check, valid):
        try:
            check(raw)
        except M.Refused:
            need(not valid, 'valid raw refused: ' + name)
        else:
            need(valid, 'invalid raw accepted: ' + name)
        raw_cases.append(dict(name=name, kind=kind, raw_hex=raw.hex(), valid=valid))

    seq1 = []
    # Source IDs, all nested record bytes/values and original sequences stay
    # inherited. Higher-sequence wrappers make NO complete-prefix claim.
    for version in ('v1', 'v2'):
        for fixture in ('incrementing256-awarded', 'zero256-awarded', 'zero256-unawarded'):
            base = root / 'actual' / version / fixture
            record = json.loads((base / 'admission.json').read_bytes())
            inherited = json.loads((base / 'expected-pin.json').read_bytes())
            pin = model.pin(inherited['source'], 0 if version == 'v1' else 1)
            pid = model.pin_id(pin)
            previous = M.ZERO if record['sequence'] == 1 else '44' * 32
            admission = model.admission(pin, record, previous)
            aid = model.validate_admission(pin, admission)
            name = version + '/' + fixture
            positive('pin', name, pin, dict(pin_id=pid))
            positive('admission', name, admission, dict(admission_id=aid),
                     'synthetic signature wrapping exact qualified old record; no new live admission')
            need(M.J(admission['record']) == (base / 'admission.json').read_bytes().rstrip(b'\n'),
                 'original nested record bytes')
            if record['sequence'] == 1:
                seq1.append((name, pin, inherited, admission, aid))
    need(len(seq1) == 2, 'two exact inherited sequence-one inputs')

    fixtures = []
    for name, pin, inherited, admission, aid in seq1:
        pid = model.pin_id(pin)
        source = pin['source']
        birth = model.initial(pid, admission, inherited['identity']['gym_public_key'])
        initial_map, initial_map_id, initial_prefix = model.ownership_map(pid, 1, birth)
        positive('state', name + '/initial', birth, dict(state_id=birth['state_id']))
        positive('map', name + '/initial', initial_map,
                 dict(map_id=initial_map_id, finalized_prefix_id=initial_prefix))
        # These heights/hashes are explicitly conditional public-model facts,
        # not normal-work headers, chain truth or actual Set64/65 chronology.
        inclusion = dict(height=100, hash='51' * 32)
        descendant = dict(height=160, hash='52' * 32)
        checkpoint = dict(version=1, network=source['network'], genesis=source['genesis'],
                          height=inclusion['height'], hash=inclusion['hash'], signature='')
        preimage = (b'SLUG signed checkpoint\0' + struct.pack('<II', 1, source['network']) +
                    bytes.fromhex(source['genesis'])[::-1] + struct.pack('<I', inclusion['height']) +
                    bytes.fromhex(inclusion['hash'])[::-1])
        checkpoint['signature'] = E.ed_sign(model.checkpoint_seed, preimage).hex()
        need(E.ed_verify(bytes.fromhex(model.checkpoint_public), preimage,
                         bytes.fromhex(checkpoint['signature'])), 'checkpoint signature')
        positive('checkpoint', name, checkpoint, dict(preimage_hex=preimage.hex()),
                 'inherited exact checkpoint signature grammar; modeled block hash')
        publications = []
        previous = M.ZERO
        payloads = [('admission', dict(admission_sequence=1, admission_id=aid)),
                    ('checkpoint', dict(checkpoint=checkpoint)),
                    ('chain', dict(height=160, hash=descendant['hash'], previous_chain_entry_id=M.ZERO))]
        for n, (kind, payload) in enumerate(payloads, 1):
            entry = model.publication(pid, n, previous, recipe['base_ms'] + n, kind, payload)
            previous = model.publication_id(pid, entry)
            publications.append(entry)
            positive('publication', name + '/' + kind, entry, dict(publication_id=previous))
        need(model.validate_prefix(pid, publications) == previous, 'complete prefix')
        checkpoint_id = model.publication_id(pid, publications[1])
        other_base = root / 'actual' / name.split('/')[0] / 'incrementing256-awarded' / 'expected-pin.json'
        recipient = json.loads(other_base.read_bytes())['identity']['gym_public_key']
        final = model.transferred(pid, birth, '61' * 32, checkpoint_id, descendant, recipient)
        final_map, map_id, prefix = model.ownership_map(pid, 1, final)
        positive('state', name + '/final', final, dict(state_id=final['state_id']),
                 'conditional verified-carrier and canonical-branch model; not a chain proof')
        positive('map', name + '/final', final_map, dict(map_id=map_id, finalized_prefix_id=prefix))
        reacquired = model.transferred(pid, final, '62' * 32, checkpoint_id,
                                      dict(height=221, hash='53' * 32), birth['owner_public_key'])
        need(reacquired['state_id'] != birth['state_id'], 'reacquisition identity')
        positive('state', name + '/reacquired', reacquired, dict(state_id=reacquired['state_id']),
                 'conditional lineage model; prior outgoing authorization stays stale')
        replay = dict(pin_id=pid, admission_sequence=1, admission_head=aid,
                      publication_sequence=3, publication_head=previous,
                      chain_height=160, chain_hash=descendant['hash'],
                      ownership_map_id=map_id, finalized_prefix_id=prefix, conflict_id='')
        head = dict(version=1, pin_id=pid, admission_sequence=1, admission_head=aid,
                    publication_sequence=3, publication_head=previous,
                    chain_height=160, chain_hash=descendant['hash'],
                    replay_digest=M.digest('SLUG fighter ownership replay', replay).hex(),
                    finalized_prefix_id=prefix, conflict_id='')
        positive('head', name, head, dict(head_id=M.digest('SLUG fighter ownership head', head).hex()))
        frame_body = dict(version=1, pin_id=pid, sequence=1, previous_body_id=M.ZERO,
                          admission_sequence=1, admission_head=aid,
                          publication_sequence=3, publication_head=previous,
                          chain_height=160, chain_hash=descendant['hash'],
                          ownership_map_id=map_id, finalized_prefix_id=prefix)
        body_id = M.digest('SLUG fighter ownership finality body', frame_body).hex()
        # Compute successor body before either frame is signed.
        second_body = copy.deepcopy(frame_body)
        second_body.update(sequence=2, previous_body_id=body_id)
        second_body['chain_height'], second_body['chain_hash'] = 161, '54' * 32
        frame, second_frame = model.frame(pid, frame_body), model.frame(pid, second_body)
        frame_id = M.digest('SLUG fighter ownership finality id', frame).hex()
        prefix_result = model.frame_prefix(pid, [frame, second_frame])
        positive('frame_body', name + '/one', frame_body, dict(body_id=body_id))
        positive('frame_body', name + '/two-before-signatures', second_body,
                 dict(body_id=M.digest('SLUG fighter ownership finality body', second_body).hex()))
        positive('frame', name + '/one', frame, dict(frame_id=frame_id))
        positive('frame', name + '/two', second_frame, prefix_result)
        frontier = dict(kind='finality_frame', id=frame_id)
        witness = dict(version=1, pin_id=pid, state_id=final['state_id'], frontier=frontier,
                       checkpoint_publication_id=checkpoint_id,
                       sixtieth_descendant_height=160, sixtieth_descendant_hash=descendant['hash'])
        positive('finality_witness', name, witness,
                 dict(frontier_evidence_id=M.digest('SLUG fighter ownership frontier evidence', frontier).hex()))
        cutoff_ms = recipe['base_ms'] + 1000
        first = dict(request_start_ms=cutoff_ms-2, response_end_ms=cutoff_ms-1,
                     instance_id='71'*32, revision='19', height=160, hash=descendant['hash'])
        second = dict(request_start_ms=cutoff_ms, response_end_ms=cutoff_ms+1,
                      instance_id='71'*32, revision='19', height=160, hash=descendant['hash'])
        intent = dict(version=1, pin_id=pid, event_id='72'*32, cutoff_ms=cutoff_ms,
                      first=first, second=second, accepted_admission_sequence=1,
                      publication_sequence=3, publication_head=previous,
                      finality_sequence=1, finality_body_head=body_id, prior_cutoff_id=M.ZERO)
        boundary = model.signed('SLUG fighter ownership boundary statement',
                                dict(version=1, pin_id=pid, intent=intent))
        boundary_id = model.validate_boundary(pid, boundary)
        positive('boundary', name, boundary, dict(boundary_id=boundary_id))
        cutoff_body = dict(version=1, pin_id=pid, event_id=intent['event_id'], cutoff_ms=cutoff_ms,
                           boundary=boundary, admission_sequence=1, admission_head=aid,
                           head=head, ownership_digest=map_id)
        cutoff = model.signed('SLUG fighter ownership cutoff statement', cutoff_body)
        model.verify('cutoff', 'SLUG fighter ownership cutoff statement', cutoff, pid)
        positive('cutoff', name, cutoff,
                 dict(cutoff_id=M.digest('SLUG fighter ownership cutoff id', cutoff).hex()))
        fixtures.append(dict(name=name, pin=pin, pid=pid, admission=admission, aid=aid,
                             publications=publications, birth=birth, final=final, initial_map=initial_map,
                             initial_map_id=initial_map_id, final_map=final_map, map_id=map_id,
                             head=head, frame_body=frame_body, frame=frame, second_frame=second_frame,
                             frontier=frontier, witness=witness, intent=intent, boundary=boundary, cutoff=cutoff))

    f = fixtures[0]
    pin, pid = f['pin'], f['pid']
    for label, field, value in [('foreign-rule', 'rule_id', 'ab'*32),
                                 ('zero-source-id', 'publication_id', M.ZERO),
                                 ('wrong-key-role', 'checkpoint_key', model.public)]:
        mutated = copy.deepcopy(pin); mutated[field] = value
        reject('pin', label, mutated, model.pin_id)
    bad = copy.deepcopy(f['admission']); bad['signature'] = '00'*64
    reject('admission', 'bad-signature', bad, lambda v:model.validate_admission(pin, v))
    bad = copy.deepcopy(f['admission']); bad['pin_id'] = 'ab'*32
    reject('admission', 'foreign-pin', bad, lambda v:model.validate_admission(pin, v))
    for label, mutate in [('record-sequence', lambda b:b['record'].__setitem__('sequence', 2)),
                          ('record-digest', lambda b:b.__setitem__('record_digest', 'ab'*32))]:
        body = copy.deepcopy(f['admission']); del body['signature']; mutate(body)
        value = model.signed('SLUG fighter ownership admission statement', body)
        reject('admission', label, value, lambda v:model.validate_admission(pin, v))
    for label, entries in [('middle-gap', [f['publications'][0], f['publications'][2]]),
                            ('reverse-prefix', list(reversed(f['publications'])))]:
        reject('publication_prefix', label, entries, lambda v:model.validate_prefix(pid, v), 'unavailable')
    body = copy.deepcopy(f['publications'][1]); del body['signature']; body['attested_at_ms'] = 0
    entries = [f['publications'][0], model.signed('SLUG fighter ownership publication statement', body)]
    reject('publication_prefix', 'clock-rollback', entries, lambda v:model.validate_prefix(pid, v))
    signed_id = M.digest('SLUG fighter ownership finality id', f['frame']).hex()
    body = copy.deepcopy(f['second_frame']); del body['signature']; body['previous_body_id'] = signed_id
    reject('frame_prefix', 'signed-id-in-body-link', [f['frame'], model.frame(pid, body)],
           lambda v:model.frame_prefix(pid, v))
    reject('frame_prefix', 'middle-frame-gap', [f['second_frame']],
           lambda v:model.frame_prefix(pid, v), 'unavailable')
    for label, side, field, value in [
        ('ABA-revision', 'second', 'revision', '21'),
        ('restart-instance', 'second', 'instance_id', '73'*32),
        ('wrong-hash', 'second', 'hash', '74'*32),
        ('first-at-cutoff', 'first', 'response_end_ms', f['intent']['cutoff_ms']),
        ('second-before-cutoff', 'second', 'request_start_ms', f['intent']['cutoff_ms']-1),
        ('revision-leading-zero', 'first', 'revision', '019'),
        ('revision-overflow', 'first', 'revision', str(2**64)),
        ('single-call-over-bound', 'first', 'request_start_ms', f['intent']['cutoff_ms']-5002)]:
        intent = copy.deepcopy(f['intent']); intent[side][field] = value
        b = model.signed('SLUG fighter ownership boundary statement', dict(version=1,pin_id=pid,intent=intent))
        reject('boundary', label, b, lambda v:model.validate_boundary(pid, v))
    intent = copy.deepcopy(f['intent'])
    intent['first'].update(request_start_ms=intent['cutoff_ms']-6000,response_end_ms=intent['cutoff_ms']-1000)
    intent['second'].update(request_start_ms=intent['cutoff_ms']+1,response_end_ms=intent['cutoff_ms']+5001)
    b = model.signed('SLUG fighter ownership boundary statement', dict(version=1,pin_id=pid,intent=intent))
    reject('boundary', 'combined-over-bound', b, lambda v:model.validate_boundary(pid, v))

    raw = M.J(pin)
    raw_case('pin-canonical', 'pin', raw, model.parse_pin, True)
    raw_case('pin-whitespace', 'pin', raw+b'\n', model.parse_pin, False)
    raw_case('pin-duplicate', 'pin', raw[:-1]+b',"version":1}', model.parse_pin, False)
    value = copy.deepcopy(pin); value['unknown'] = 1
    raw_case('pin-unknown', 'pin', M.J(value), model.parse_pin, False)
    value = dict(reversed(list(pin.items())))
    raw_case('pin-outer-order', 'pin', M.J(value), model.parse_pin, False)
    value = copy.deepcopy(pin); value['source'] = dict(reversed(list(value['source'].items())))
    raw_case('pin-nested-order', 'pin', M.J(value), model.parse_pin, False)
    value = copy.deepcopy(pin); value['publication_id'] = 'AB'*32
    raw_case('pin-uppercase', 'pin', M.J(value), model.parse_pin, False)

    # The following state controls take explicit modeled canonical/validation
    # facts. They do not validate raw transactions or actual chain work.
    candidates = [dict(name='later', height=100, position=3, modeled_valid=True),
                  dict(name='invalid-first', height=99, position=1, modeled_valid=False),
                  dict(name='first-valid', height=100, position=2, modeled_valid=True)]
    for name, height, checkpoint, missing in [('59-descendants',159,True,False),
        ('60-descendants',160,True,False),('no-checkpoint',160,False,False),
        ('known-required-evidence-missing',160,True,True)]:
        state_cases.append(dict(name=name, input=dict(candidates=candidates,tip_height=height,
                           checkpoint_present=checkpoint,missing=missing),
                           expected=M.model_order(candidates,height,checkpoint,missing)))
    reachable = [f['birth']['state_id'], f['final']['state_id']]
    for name, prior, complete, missing, future in [
        ('known-final-predecessor', reachable[1], True, False, False),
        ('junk-unknown-prior-closed-frontier', 'ff'*32, True, False, False),
        ('known-parent-evidence-missing', reachable[1], True, True, False),
        ('unknown-prior-incomplete-frontier', 'ff'*32, False, False, False),
        ('proved-future-nonfinal-prior', 'ff'*32, True, False, True)]:
        inputs = dict(prior=prior,reachable=reachable,complete=complete,
                      missing_known=missing,future_or_nonfinal=future)
        state_cases.append(dict(name=name,input=inputs,expected=M.prior_classification(**inputs)))
    for name, descendant, commit_height, expected in [('strictly-earlier',160,161,True),
        ('same-height',160,160,False),('later-descendant',161,160,False)]:
        need((descendant < commit_height) == expected, 'chronology model')
        state_cases.append(dict(name='predecessor/'+name,
            input=dict(sixtieth_height=descendant,successor_commit_height=commit_height),expected=expected))
    state_cases.extend([
        dict(name='unsigned-finality-outbox',input=dict(effective=True,frames=1,published=0),
             expected=dict(owner=f['final']['owner_public_key'],public_evidence_pending=True,portable='unavailable')),
        dict(name='signed-finality-prefix',input=dict(effective=True,frames=1,published=1),
             expected=dict(owner=f['final']['owner_public_key'],public_evidence_pending=False,portable='requires-complete-branch')),
        dict(name='postfinal-reorg-with-public-witness',input=dict(finalized=f['final']['state_id'],contradictory_branch=True),
             expected=dict(retained_state_id=f['final']['state_id'],conflict='finalized_branch_conflict')),
        dict(name='postfinal-reorg-missing-witness',input=dict(local_claim=f['final']['state_id'],signed_witness=False),
             expected='unavailable-official-history'),
        dict(name='partition-independent-map',input=dict(partitions=[[1,1,1],[3]]),
             expected=dict(map_id=f['map_id'],state_id=f['final']['state_id'])),
        dict(name='admission-accepted-before-signature',input=dict(accepted=1,signed=0),
             expected=dict(initial_owner=f['birth']['owner_public_key'],portable='unavailable')),
    ])

    # Exact key/value accounting fixtures, deliberately not full Open fixtures:
    # modeled final states lack real block bytes and carrier proofs.
    # D76/D83 amendment: the exact unsigned finality prefix is frozen under
    # the original core+registry permits. These are prefix model controls, not
    # claims that an unsigned or later frame independently proves ownership.
    selected = model.cutoff_frame_prefix(pid, f['boundary'], [f['frame'], f['second_frame']])
    state_cases.append(dict(name='cutoff-excludes-later-frame-at-same-old-frontiers',
        input=dict(boundary=f['boundary'], frames=[f['frame'], f['second_frame']]),
        expected=selected))
    zero_intent = copy.deepcopy(f['intent'])
    zero_intent.update(finality_sequence=0, finality_body_head=M.ZERO)
    zero_boundary = model.signed('SLUG fighter ownership boundary statement',
        dict(version=1, pin_id=pid, intent=zero_intent))
    state_cases.append(dict(name='cutoff-zero-retained-prefix-is-not-latest-frame',
        input=dict(boundary=zero_boundary, frames=[f['frame'], f['second_frame']]),
        expected=model.cutoff_frame_prefix(pid, zero_boundary, [f['frame'], f['second_frame']])))
    reject('cutoff_frame_prefix', 'frozen-signature-gap',
        dict(boundary=f['boundary'], frames=[]),
        lambda v:model.cutoff_frame_prefix(pid, v['boundary'], v['frames']), 'unavailable')
    wrong_intent = copy.deepcopy(f['intent'])
    wrong_intent['finality_body_head'] = M.digest('SLUG fighter ownership finality id', f['frame']).hex()
    wrong_boundary = model.signed('SLUG fighter ownership boundary statement',
        dict(version=1, pin_id=pid, intent=wrong_intent))
    reject('cutoff_frame_prefix', 'signed-frame-id-is-not-frozen-body-id',
        dict(boundary=wrong_boundary, frames=[f['frame'], f['second_frame']]),
        lambda v:model.cutoff_frame_prefix(pid, v['boundary'], v['frames']))
    for label, count, head in [('zero-prefix-nonzero-head', 0, f['intent']['finality_body_head']),
                             ('nonzero-prefix-zero-head', 1, M.ZERO),
                             ('finality-prefix-cap', 4097, f['intent']['finality_body_head'])]:
        bad_intent = copy.deepcopy(f['intent'])
        bad_intent.update(finality_sequence=count, finality_body_head=head)
        bad_boundary = model.signed('SLUG fighter ownership boundary statement',
            dict(version=1, pin_id=pid, intent=bad_intent))
        reject('boundary', label, bad_boundary, lambda v:model.validate_boundary(pid, v))

    rows = []
    def row(k, value):
        rows.append(dict(key_hex=k.hex(),value_hex=M.J(value).hex()))
    row(M.key('a/',1,True),f['admission'])
    for entry in f['publications']:
        row(M.key('p/',entry['sequence'],True),entry)
    for state in (f['birth'],f['final']):row(M.key('s/',state['state_id']),state)
    row(M.key('m/',f['initial_map_id']),f['initial_map'])
    row(M.key('m/',f['map_id']),f['final_map'])
    row(M.key('fo/',1,True),f['frame_body']);row(M.key('f/',1,True),f['frame'])
    frontier_id=M.digest('SLUG fighter ownership frontier evidence',f['frontier']).hex()
    row(b'v/'+bytes.fromhex(f['final']['state_id'])+bytes.fromhex(frontier_id),f['witness'])
    row(M.key('b/',f['intent']['event_id']),f['boundary'])
    row(M.key('k/',f['intent']['event_id']),f['cutoff'])
    counts=dict(admissions=1,publications=3,headers=0,carriers=0,witnesses=0,
                states=2,maps=2,cutoffs=1,boundaries=1,conflicts=0)
    metadata=dict(version=1,pin_id=pid,store_id='21'*32,revision=1,clock_high_water_ms=recipe['base_ms']+1000,
                  counts=counts,logical_bytes=0,current_head=f['head'],current_map_id=f['map_id'],work_id='',
                  finality_sequence=1,published_finality_sequence=1,finality_head=signed_id)
    metadata=M.logical_metadata(rows,metadata)
    storage_cases.append(dict(name='exact-logical-key-value-accounting',origin='codec/accounting only; not complete verified storage',
                              rows=rows,metadata=metadata,metadata_hex=M.J(metadata).hex(),logical_bytes=metadata['logical_bytes']))
    reject('storage','duplicate-key',rows+[rows[0]],lambda v:M.logical_metadata(v,metadata))
    state_cases.append(dict(name='partial-work-keeps-committed-pair',
       input=dict(committed_map=f['initial_map_id'],working_map=f['map_id'],phase='chain'),
       expected=dict(visible_map=f['initial_map_id'],replacement_requires='one-complete-sync-batch')))
    # Conservative grammar arithmetic, not measurements of Go heap or a claim
    # that every independent count cap fits simultaneously in 256 MiB.
    maximal_item=dict(admission_sequence=1000,prospect_id='ff'*32,state_id='ff'*32)
    maximal_map=dict(version=1,pin_id='ff'*32,items=[maximal_item]*1000)
    maximal_frame=copy.deepcopy(f['frame'])
    maximal_frame.update(sequence=4096,admission_sequence=1000,publication_sequence=16384,
                         chain_height=2147483647)
    admission_shell=copy.deepcopy(f['admission']);admission_shell['record']={}
    admission_shell['admission_sequence']=1000
    admission_upper=len(M.J(admission_shell))-2+65536
    page_shell=dict(version=1,pin_id='ff'*32,through_sequence=1000,through_head='ff'*32,
                    after=1000,next=1000,complete=False,items=[])
    page_upper=len(M.J(page_shell))-2+2+8*69632+7
    owned_page_upper=page_upper+8*(4096+len('{"admission":,"state":}'))
    witness_upper=(4096+12000)*2+16*(64+10+16+44+80)+4096
    budgets=[dict(name='max-map',bytes=len(M.J(maximal_map)),cap=262144),
             dict(name='max-frame-fields',bytes=len(M.J(maximal_frame)),cap=8192),
             dict(name='statement-with-max-old-record',bytes=admission_upper,cap=69632),
             dict(name='eight-admission-page',bytes=page_upper,cap=655360),
             dict(name='eight-owned-items-page',bytes=owned_page_upper,cap=655360),
             dict(name='carrier-witness-conservative',bytes=witness_upper,cap=65536)]
    for budget in budgets:
        need(budget['bytes']<=budget['cap'],'frozen grammar cap: '+budget['name'])
    return dict(version=1,scope='independent public codecs and conditional replay model; no new normal-work verdict',
                literals=dict(rule_id=model.rule_id,public_log_key=model.public,checkpoint_key=model.checkpoint_public),
                positives=positives,negatives=negatives,raw_cases=raw_cases,state_cases=state_cases,
                storage_cases=storage_cases,budgets=budgets)


def main():
    p=argparse.ArgumentParser()
    p.add_argument('--execute',action='store_true')
    p.add_argument('--input-root',type=Path,default=HERE/'inputs')
    p.add_argument('--input-catalog',type=Path,default=HERE/'input-catalog.json')
    p.add_argument('--approved-producer-lock',type=Path)
    p.add_argument('--approved-lock-sha256')
    p.add_argument('--output',type=Path)
    args=p.parse_args()
    if not args.execute:
        print('Source prepared only; no model imports or expected-output generation.')
        return
    need(args.approved_producer_lock is not None and args.output is not None,'explicit lock/output required')
    need(file_sha(args.approved_producer_lock)==args.approved_lock_sha256,'lock hash')
    lock=json.loads(args.approved_producer_lock.read_bytes())
    need(lock.get('approved_by')=='root' and lock.get('execution_authorized') is True,'approval')
    need(lock['input_catalog_sha256']==file_sha(args.input_catalog),'input catalog')
    need(set(lock['sources'])==SOURCE_NAMES,'exact producer source set')
    before={n:file_sha(HERE/n) for n in SOURCE_NAMES}
    need(before==lock['sources'],'producer source bytes')
    catalog=json.loads(args.input_catalog.read_bytes())
    need(catalog['public_only'] is True,'public inputs')
    input_before=check_inputs(args.input_root,catalog)
    args.output.mkdir(parents=True,exist_ok=False)
    M=load('set73_public_core_model',HERE/'core_model.py')
    E=load('set45_public_reference_crypto',args.input_root/'inherited/crypto.py')
    recipe=json.loads((args.input_root/'recipe.json').read_bytes())
    result=produce(M,E,args.input_root,recipe)
    result['provenance']=dict(producer_lock_sha256=args.approved_lock_sha256,
                              input_catalog_sha256=file_sha(args.input_catalog))
    raw=M.J(result)+b'\n'
    need(len(raw)<=16*1024*1024,'bounded corpus output')
    need(before=={n:file_sha(HERE/n) for n in SOURCE_NAMES},'source unchanged')
    need(check_inputs(args.input_root,catalog)==input_before,'input unchanged')
    with (args.output/'public-core-v1.json').open('xb') as out:out.write(raw)
    report=dict(status='passed',sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw),
                counts={k:len(result[k]) for k in ('positives','negatives','raw_cases','state_cases','storage_cases','budgets')},
                source_unchanged=True,input_unchanged=True,normal_work_calls=0)
    with (args.output/'report.json').open('xb') as out:out.write(M.J(report)+b'\n')
    print(json.dumps(report,separators=(',',':')))


if __name__=='__main__':
    main()
