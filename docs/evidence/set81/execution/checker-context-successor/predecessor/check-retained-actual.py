#!/usr/bin/env python3
"""Strict PUBLIC Set81 result checker; root alone imports or executes.

The source-bound original owner audits supply proof/signature/consensus facts.
This checker independently correlates the public lifecycle, counts, exact bytes
and frozen economics. It never opens a database, private fixture or credential.
"""
import copy
import hashlib
import os
from pathlib import Path
import re
import stat
import struct
import types

BASE = Path('/Users/taytrrip/slug/set80-prep/check-actual.py')
BASE_SHA = '87cb6364644db35b707a90a4bcf1c97467915aa3d691e65c81e3b9fc4539736e'
REFERENCE = Path('/Users/taytrrip/slug/set81-prep/retained-admission-actual/inputs/reference-v1.json')
REFERENCE_SHA = '17557b28921290461fdf47b9ed6f1b800d3d077ee5d325a4ec59277ab3faca6a'
REFERENCE_BYTES = 174293


def original_reader():
    # Only the immutable pure reader/financial primitives are reused. Its old
    # issue-crash scenario check is never invoked or weakened for this case.
    if not BASE.is_absolute() or BASE.is_symlink() or BASE.resolve(strict=True) != BASE:
        raise ValueError('Set81: original checker path')
    fd = os.open(BASE, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_size != 65246:
            raise ValueError('Set81: original checker file')
        raw = bytearray()
        while len(raw) <= 65246:
            part = os.read(fd, 65247-len(raw))
            if not part:
                break
            raw.extend(part)
        after = os.fstat(fd)
        if ((before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns) !=
                (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns)
                or len(raw) != 65246 or hashlib.sha256(raw).hexdigest() != BASE_SHA):
            raise ValueError('Set81: original checker identity')
        raw = bytes(raw)
    finally:
        os.close(fd)
    module = types.ModuleType('set81_pinned_original_public_reader')
    module.__file__ = str(BASE)
    exec(compile(raw, str(BASE), 'exec'), module.__dict__)
    return module


def reference(b):
    raw = b.read_regular(REFERENCE, 1 << 20)
    b.need(len(raw) == REFERENCE_BYTES and b.sha(raw) == REFERENCE_SHA,
           'qualified pre-execution Set81 selection')
    ref = b.parse(raw)
    b.need(ref['version'] == 1 and ref['ordinary_maximum'] == 2 and
           ref['required_accepted_rewards'] == 2 and
           ref['setup']['base']['maintenance_interval_ms'] == 1000,
           'only selected maintenance cadence; mandatory full two rewards')
    expected = dict(accepted_rewards=2, credits=2, batches=1, input_count=2,
                    positive_payees=1, output_count=2, gross_atoms=2000000000,
                    miner_atoms=1980000000, operator_gross_atoms=20000000,
                    fee_atoms=209, change_atoms=19999791, other_gym_atoms=0,
                    shape=dict(stripped_bytes=154, witness_bytes=220,
                               raw_bytes=374, weight=836, vsize=209))
    b.same(ref['selected_economics'], expected, 'independent two-reward arithmetic')
    b.need([e for e in ref['economics'] if e['accepted_rewards'] == 2] == [expected],
           'same qualified economic row; no one-reward success substitute')
    return ref, expected


def original_processes(b, public, report, case, probe_pid, selection):
    b.integer(probe_pid, 1)
    used = {probe_pid}
    end = report['start_ms'] + 900000
    def child(v):
        pid = b.integer(v['pid'], 1)
        b.need(pid not in used and v['parent_pid'] == v['pgid'] == probe_pid,
               'distinct original child and inherited outer group')
        used.add(pid)
        b.truth(v['joined'])
    names = ['operator-init', 'operator-rotate-pool-0', 'operator-rotate-pool-1',
             'operator-rotate-admission']
    names += ['operator-serve-' + str(i) for i in range(1, 4 if case == 'A' else 3)]
    ops = report['operator_generations']
    b.need(type(ops) is list and len(ops) == len(names), 'complete exact operator generations')
    for i, (name, op) in enumerate(zip(names, ops)):
        b.keys(op, 'generation command pid parent_pid pgid joined intentional_crash unexpected_exit signal signal_count exit_code cleanup_error io_error frames')
        b.need(op['generation'] == name and op['command'] == ('init' if i == 0 else 'rotate' if i < 4 else 'serve'), 'original operator purpose')
        child(op)
        for k in ('intentional_crash', 'unexpected_exit', 'cleanup_error', 'io_error'):
            b.truth(op[k], False)
        b.need(op['exit_code'] == 0 and op['signal_count'] == (0 if i < 4 else 1)
               and op['signal'] == ('' if i < 4 else 'SIGTERM'), 'clean original operator shutdown; no crash')
        frames = public.lines(name + '-stdout.jsonl', 256, 1 << 20)
        b.need(frames and public.raw(name + '-stderr.log', 1 << 20) == b'', 'complete clean operator output')
        b.same(frames, op['frames'], 'captured original operator output')
        for frame in frames:
            b.keys(frame, 'version command observed_at_ms code', 'result')
            b.need(frame['version'] == 2 and frame['command'] == op['command']
                   and frame['code'] in ('complete', 'listening', 'status', 'busy', 'unavailable'), 'closed operator output')
            b.integer(frame['observed_at_ms'], report['start_ms'], end)
            if frame['code'] == 'listening':
                b.need(frame['result']['origin'] == selection['setup']['base']['ingress']['origin'], 'original selected listener')
            if frame['code'] == 'status':
                b.need(frame['result']['setup_id'] == report['setup_id'], 'original runtime selection')
        if i < 4:
            b.need(len(frames) == 1 and frames[0]['code'] == 'complete', 'original one-shot command success')
        else:
            b.need(any(f['code'] == 'listening' for f in frames), 'actual original listener started')
    clis = report['cli_generations']
    b.need(type(clis) is list and len(clis) == (3 if case == 'A' else 2), 'complete exact CLI generations')
    zero = dict(version=0, observed_at_ms=0, config_id='', state='', worker_state='', code='', records=0, pending=0)
    for i, cli in enumerate(clis, 1):
        b.keys(cli, 'pid parent_pid pgid joined exit_code signal signal_count forced_kill unexpected_exit wait_error stdout_joined stderr_joined io_error status_count config_id last_status')
        child(cli)
        for k in ('forced_kill', 'unexpected_exit', 'wait_error', 'io_error'):
            b.truth(cli[k], False)
        for k in ('stdout_joined', 'stderr_joined'):
            b.truth(cli[k])
        b.need(cli['exit_code'] == 0 and cli['signal'] == 'SIGTERM' and cli['signal_count'] == 1
               and cli['config_id'] == report['remote_id'], 'one original CLI Stop and clean join')
        frames = public.lines('cli-' + str(i) + '-stdout.jsonl', 1024, 512)
        b.need(len(frames) == cli['status_count'], 'exact complete CLI stdout')
        b.same(cli['last_status'], frames[-1] if frames else zero, 'last emitted CLI status')
        b.need(public.raw('cli-' + str(i) + '-stderr.log', 1 << 20) == b'', 'empty CLI stderr')
        count = 0
        for frame in frames:
            b.keys(frame, 'version observed_at_ms config_id state worker_state code records pending', ordered=True)
            b.need(frame['version'] == 2 and frame['config_id'] == report['remote_id'] and frame['state'] == 'signed_in', 'original signed-in selection')
            b.need(frame['worker_state'] in ('ready', 'stopped', 'failed', 'paused', 'preparing', 'retrying', 'hashing', 'delivering', 'unavailable')
                   and frame['code'] in ('', 'unavailable', 'outcome_unknown', 'busy', 'capacity', 'rate_limited', 'expired'), 'closed worker status')
            count = b.integer(frame['records'], count, 64)
            b.integer(frame['pending'], 0, count)
            b.integer(frame['observed_at_ms'], report['start_ms'], end)
    nodes = report['node_cleanup']
    b.need(type(nodes) is list and len(nodes) == 2, 'two original normal nodes')
    for node in nodes:
        b.keys(node, 'pid parent_pid pgid group_error joined forced_kill unexpected_exit wait_error stop_rpc_error log_close_error clean')
        child(node)
        for k in ('group_error', 'forced_kill', 'unexpected_exit', 'stop_rpc_error', 'log_close_error'):
            b.truth(node[k], False)
        b.need(node['wait_error'] == '', 'normal original node exit')
        b.truth(node['clean'])
    b.need(report['intentional_crash_count'] == 0, 'no intentional crash in retained-admission case')
    return ops, clis


def box(b, value, source):
    b.outbox(value)
    b.need(value['requests'] <= source['max_works'], 'same finite retained request ceiling')
    if value['active_request_id']:
        b.need(value['records'] + value['requests'] < 64, 'original reserved first-found slot')
    return value


def unchanged_entry_identity(b, before, after):
    for k in ('version', 'sequence', 'work', 'nonce', 'work_hash', 'duplicate_key', 'pool_win'):
        b.same(before[k], after[k], 'same retained original entry ' + k)
    for k in ('job', 'candidate', 'work_hash', 'prospect_win', 'authorization'):
        b.same(before['fighter'][k], after['fighter'][k], 'same original signed fighter ' + k)


def unchanged_prefix(b, before, after):
    b.need(type(before) is list and type(after) is list and len(after) >= len(before), 'original retained row prefix')
    for old, new in zip(before, after):
        for k in ('version', 'sequence', 'work', 'nonce', 'work_hash', 'duplicate_key', 'pool_win'):
            b.same(old[k], new[k], 'no replaced retained work ' + k)
        old_f, new_f = old['fighter'], new['fighter']
        for k in ('job', 'candidate', 'work_hash', 'prospect_win'):
            b.same(old_f[k], new_f[k], 'no replaced retained prospect ' + k)
        if 'authorization' in old_f:
            b.same(old_f['authorization'], new_f['authorization'], 'no replaced original signature')
        if 'receipt' in old_f:
            b.same(old_f['receipt'], new_f['receipt'], 'no replaced original acknowledged birth')


def admission_result(b, result, target, inserted):
    b.keys(result, 'version context_source current_node_checked authentication birth_rule inserted record', ordered=True)
    b.need(result['version'] == 3 and result['authentication'] == 'gym-key-control'
           and result['birth_rule'] == 'job-issuance', 'original admission response grammar')
    b.truth(result['inserted'], inserted)
    b.truth(result['current_node_checked'], inserted)
    b.need(result['context_source'] == ('trusted-node-snapshot' if inserted else 'historical-receipt'), 'original response provenance')
    record = result['record']
    b.keys(record, 'version sequence context proof verified authorization job birth eligibility static_binding', ordered=True)
    b.need(record['version'] == 5, 'original immutable manifest record grammar')
    b.same(record['job'], target['work']['job'], 'same original target job')
    b.same(record['context'], target['work']['job']['context'], 'same original context')
    b.same(record['proof'], target['proof'], 'same original target proof')
    b.same(record['authorization'], target['authorization'], 'same original target signature')
    b.need(record['verified']['work_hash'] == target['receipt']['work_hash']
           and record['verified']['gym_id'] == target['work']['job']['gym_id'], 'same actual normal proof hash and gym')
    birth = record['birth']
    b.keys(birth, 'rule_version born_at_ms accepted_at_ms', ordered=True)
    b.need(birth['rule_version'] == target['work']['job']['rule_version'] and birth['born_at_ms'] == target['work']['job']['issued_at_ms']
           and target['work']['job']['issued_at_ms'] <= birth['accepted_at_ms'] < target['work']['job']['expires_at_ms'], 'original issuance-time birth and live acceptance')
    eligibility = record['eligibility']
    b.keys(eligibility, 'rule_version source hash height', ordered=True)
    b.need(eligibility['rule_version'] == 2 and eligibility['source'] == 'canonical-winning-block'
           and eligibility['hash'] == target['receipt']['work_hash']
           and eligibility['height'] == target['receipt']['block_height'], 'original canonical winner eligibility')
    b.integer(record['sequence'], 1, 64)
    return record


def closed_admission(b, value, source, root, target, exists):
    b.keys(value, 'financial target_prospect_id job_id job proof authorization record record_sha256 record_bytes target_records target_sequences target_outbox target_signed_admissions target_publications ownership_state admission admission_id publication counts issuer_rows public_files_before public_files_after read_only_unchanged', ordered=True)
    b.truth(value['read_only_unchanged'])
    b.same(value['public_files_before'], value['public_files_after'], 'complete original public files unchanged after closed audit')
    financial = b.financial_shape(value['financial'], source, root)
    b.need(financial['submissions'] == financial['accepted'] == len(financial['entries']) == 2, 'original complete two accepted source winners')
    b.same(value['public_files_before'], financial['public_files_before'], 'same closed original public roots')
    b.same(value['issuer_rows'], {k: v for k, v in financial['public_rows'].items() if k.startswith('issuer/')}, 'complete original issuer namespace')
    b.same(value['job'], target['work']['job'], 'closed same original job')
    b.same(value['proof'], target['proof'], 'closed same original proof')
    b.same(value['authorization'], target['authorization'], 'closed same original signature')
    b.need(value['job_id'] == target['work']['job']['id'], 'closed original job ID')
    b.hx(value['target_prospect_id'])
    second = financial['entries'][1]
    b.same(second['job'], target['work']['job'], 'target is second accepted source job')
    b.same(second['proof'], target['proof'], 'target is second accepted source proof')
    b.same(b.receipt_base(second['share']), b.receipt_base(target['receipt']), 'exact second accepted receipt')
    first_records = [r for r in financial['admissions'] if r['verified']['work_hash'] == target['first_receipt']['work_hash']]
    b.same(first_records, [target['first_admission']['record']], 'first winner admission is retained by the same original authority')
    counts = b.keys(value['counts'], 'records sequences ownership_outbox signed_admissions publications', ordered=True)
    for number in counts.values():
        b.integer(number, 0, 16384)
    b.need(counts['records'] == counts['sequences'] == counts['ownership_outbox'] == financial['births']
           and counts['signed_admissions'] <= counts['records'], 'full original record/index/permanent-outbox census')
    for field in ('target_records', 'target_sequences', 'target_outbox', 'target_signed_admissions', 'target_publications'):
        b.integer(value[field], 0, 1)
    if not exists:
        b.need(value['record'] is value['admission'] is value['publication'] is None
               and value['record_sha256'] == value['admission_id'] == '' and value['record_bytes'] == 0
               and value['ownership_state'] == 'absent'
               and all(value[k] == 0 for k in ('target_records', 'target_sequences', 'target_outbox', 'target_signed_admissions', 'target_publications')),
               'exact target absent across record/index/outbox/signed/publication')
        b.need(all(r['verified']['prospect_id'] != value['target_prospect_id'] for r in financial['admissions']), 'no hidden target birth in complete audit')
    else:
        record = value['record']
        admission_result(b, dict(version=3, context_source='trusted-node-snapshot', current_node_checked=True,
                                 authentication='gym-key-control', birth_rule='job-issuance', inserted=True, record=record), target, True)
        b.need(record['verified']['prospect_id'] == value['target_prospect_id'], 'same original target prospect ID')
        raw = b.canonical(record)
        b.need(value['record_bytes'] == len(raw) and value['record_sha256'] == b.sha(raw), 'exact original stored record bytes')
        key = 'issuer/' + (b'record:' + value['target_prospect_id'].encode('ascii')).hex()
        b.need(value['issuer_rows'].get(key) == b.sha(raw), 'target original record row')
        seqkey = 'issuer/' + (b'sequence:' + struct.pack('>Q', record['sequence'])).hex()
        b.need(value['issuer_rows'].get(seqkey) == b.sha(value['target_prospect_id'].encode('ascii')), 'target exact receipt inverse')
        out = dict(version=1, admission_sequence=record['sequence'], prospect_id=value['target_prospect_id'])
        outkey = 'issuer/' + (b'ownership/outbox/' + struct.pack('>I', record['sequence'])).hex()
        b.need(value['issuer_rows'].get(outkey) == b.sha(b.canonical(out)), 'target one permanent ownership journal row')
        b.need(value['target_records'] == value['target_sequences'] == value['target_outbox'] == 1, 'exactly one target birth/index/outbox')
        matches = [r for r in financial['admissions'] if r['verified']['prospect_id'] == value['target_prospect_id']]
        b.same(matches, [record], 'exact one target in complete admission census')
        if value['ownership_state'] == 'pending-publication':
            b.need(value['admission'] is value['publication'] is None and value['admission_id'] == ''
                   and value['target_signed_admissions'] == value['target_publications'] == 0, 'original pending publication remains pending')
        else:
            b.need(value['ownership_state'] == 'published' and value['target_signed_admissions'] == value['target_publications'] == 1, 'exact published target')
            a, p = value['admission'], value['publication']
            b.same(a['record'], record, 'signed publication retains exact original record')
            b.need(a['admission_sequence'] == record['sequence'] and p['kind'] == 'admission'
                   and p['payload'] == dict(admission_sequence=record['sequence'], admission_id=value['admission_id']), 'original public publication linkage')
            pin_id = b.identity('SLUG fighter ownership pin', source['authority'])
            b.need(a['pin_id'] == p['pin_id'] == pin_id
                   and a['record_digest'] == b.identity('SLUG fighter ownership admission record', record)
                   and value['admission_id'] == b.identity('SLUG fighter ownership admission id', a), 'independent public pin/record/admission byte commitments')
            b.hx(a['signature'], 64); b.hx(p['signature'], 64)
            akey = 'issuer/' + (b'ownership/admission/' + struct.pack('>I', record['sequence'])).hex()
            pkey = 'issuer/' + (b'ownership/publication/' + struct.pack('>I', p['sequence'])).hex()
            b.need(value['issuer_rows'].get(akey) == b.sha(b.canonical(a))
                   and value['issuer_rows'].get(pkey) == b.sha(b.canonical(p)), 'same fully audited signed admission/publication bytes')
    return financial


def target_identity(b, target, source, start):
    b.keys(target, 'request_id work proof authorization receipt first_receipt first_admission admission_body_sha256 first_held_at_ms', ordered=True)
    b.hx(target['request_id'])
    work = b.keys(target['work'], 'job template', ordered=True)
    job = work['job']
    b.keys(job, 'version rule_version id authority_id descriptor_id context public_key gym_id challenge issued_at_ms expires_at_ms', ordered=True)
    b.need(job['version'] == 2 and job['authority_id'] == source['pool']['authority_pin']['authority_id']
           and job['descriptor_id'] == source['pool']['authority_pin']['descriptor_id'], 'original selected manifest issuer and descriptor')
    b.same(job['context'], work['template']['context'], 'same exact original job/template context')
    b.hx(job['id'])
    gym = source['miners']['enrollments'][0]
    b.need(job['public_key'] == gym['public_key'] and job['gym_id'] == gym['gym_id'], 'original selected gym/key')
    b.need(job['issued_at_ms'] <= target['first_held_at_ms'] < job['expires_at_ms']
           and job['expires_at_ms']-job['issued_at_ms'] == 300000, 'original live five-minute target lifetime')
    b.integer(target['first_held_at_ms'], start, start+900000)
    proof, auth, receipt = target['proof'], target['authorization'], target['receipt']
    b.need(proof['version'] == auth['version'] == 2 and proof['job_id'] == job['id']
           and proof['gym_id'] == job['gym_id'] and auth['public_key'] == job['public_key'], 'original signed job-bound proof')
    b.hx(proof['header'], 80)
    b.need(receipt['job_id'] == job['id'] and receipt['header'] == proof['header']
           and receipt['gym_id'] == gym['gym_id'] and receipt['pool'] is True and receipt['block'] is True
           and receipt['accepted_sequence'] == 2 and receipt['block_height'] == 4
           and receipt['pool_target'] == source['pool']['pool_target'], 'target is second actual accepted pool winner')
    b.need(receipt['nonce'] == int.from_bytes(bytes.fromhex(proof['header'])[76:80], 'little'), 'target exact header nonce')
    b.hx(receipt['work_hash']); b.hx(receipt['duplicate_key'])
    body = b.canonical(dict(proof=proof, authorization=auth))
    b.need(b.sha(body) == target['admission_body_sha256'], 'original exact admission request bytes')
    first = target['first_receipt']; admission = target['first_admission']
    b.need(first['accepted_sequence'] == 1 and first['block_height'] == 3 and first['pool'] is True and first['block'] is True
           and first['gym_id'] == gym['gym_id'] and first['pool_target'] == source['pool']['pool_target']
           and first['duplicate_key'] != receipt['duplicate_key'] and first['work_hash'] != receipt['work_hash'], 'two distinct accepted original winners')
    b.need(admission['inserted'] is True and admission['current_node_checked'] is True
           and admission['context_source'] == 'trusted-node-snapshot'
           and admission['record']['job']['id'] == first['job_id']
           and admission['record']['verified']['work_hash'] == first['work_hash']
           and admission['record']['proof']['header'] == first['header']
           and admission['record']['birth']['accepted_at_ms'] <= target['first_held_at_ms'], 'first winner actually admitted before target selection')
    return target


def target_entry(b, rows, target, state):
    b.need(type(rows) is list and 1 <= len(rows) <= 63, 'bounded complete retained rows')
    matches = [r for r in rows if r['work']['job']['id'] == target['work']['job']['id']
               and r['nonce'] == target['receipt']['nonce']]
    b.need(len(matches) == 1, 'one exact target retained entry')
    row = matches[0]
    b.same(row['work'], target['work'], 'same target work/template/expiry')
    b.need(row['pool_win'] is True and row['pool_state'] == 'accepted'
           and row['work_hash'] == target['receipt']['work_hash']
           and row['duplicate_key'] == target['receipt']['duplicate_key'], 'original accepted pool half retained')
    b.same(b.receipt_base(row['pool_receipt']), b.receipt_base(target['receipt']), 'same original durable pool receipt')
    fighter = row['fighter']
    b.same(fighter['job'], target['work']['job'], 'same signed target job')
    b.same(fighter['candidate']['proof'], target['proof'], 'same target signed candidate')
    b.same(fighter['authorization'], target['authorization'], 'same retained target signature')
    b.need(fighter['admission_state'] == state, 'precise original target local state')
    if state == 'pending':
        b.need('receipt' not in fighter, 'uncertain original admission stays pending')
    elif state == 'accepted':
        admission_result(b, fighter['receipt'], target, False)
    elif state == 'expired':
        b.need('receipt' not in fighter and fighter.get('last_error') == 'expired', 'only authoritative expiry clears target')
    else:
        b.need(False, 'unsupported target evidence state')
    return row


def readiness(b, status, op, report, target, before_ms=None):
    b.need(type(status) is dict and status['setup_id'] == report['setup_id']
           and status['running'] is True and status['closing'] is False, 'original live runtime readiness')
    b.truth(status['public']['ready'])
    b.need(status['public_code'] == status['round_code'] == '', 'both original readiness checks succeeded')
    for key in ('public_at_ms', 'round_at_ms'):
        b.integer(status[key], report['start_ms'], report['start_ms']+900000)
    capacity = status['capacity']
    b.need(capacity['mode'] == 'draining' and capacity['reason'] == 'pool_capacity'
           and capacity['submissions'] == 2 and capacity['over_bound'] is False, 'pool-only full intake; no hard hold substituted')
    b.need(capacity['daily_reservation']['ordinary_maximum'] == 2
           and status['rounds']['through'] == status['rounds']['rounds'] == 2,
           'original independently reserved cap and complete round catch-up')
    b.integer(capacity['at_ms'], target['first_held_at_ms'], report['start_ms']+900000)
    matches = [f for f in op['frames'] if f['code'] == 'status' and f.get('result') == status]
    b.need(matches, 'readiness is an actually emitted original operator frame')
    if before_ms is not None:
        b.need(any(f['observed_at_ms'] <= before_ms for f in matches), 'fresh readiness observed before target forwarding')
    return status


def relay_evidence(b, value, target, source, pid, case, start):
    b.keys(value, 'version max_events target mode lost lost_response events failure closed handlers_joined', ordered=True)
    b.need(value['version'] == 1 and value['max_events'] == 512, 'fixed relay evidence grammar/bound')
    for k in ('closed', 'handlers_joined'):
        b.truth(value[k])
    b.truth(value['failure'], False)
    b.same(value['target'], target, 'one retained exact target for all relay generations')
    b.need(value['mode'] == ('replay' if case == 'A' else 'expired'), 'final exact target relay purpose')
    b.truth(value['lost'], case == 'A')
    events = value['events']
    b.need(type(events) is list and 1 <= len(events) <= 512, 'complete bounded relay attempts')
    requests = b.canonical(dict(proof=target['proof'], authorization=target['authorization']))
    targets, lost, successes, expired = [], [], [], []
    event_fields = 'ordinal path started_at_ms ended_at_ms request_id job_id header request_bytes request_sha256 target mode held_at_ms hold_deadline_ms released_at_ms forwarded_at_ms status response_bytes response_sha256 lost downstream_bytes context failure admission api_error'
    for i, event in enumerate(events, 1):
        b.keys(event, event_fields, ordered=True)
        b.need(event['ordinal'] == i and event['path'] in ('/v3/admissions', '/pool/v3/submit', '/pool/v3/lookup', '/pool/v3/issue', '/pool/v3/issue-lookup'), 'original tracked route/order')
        begin = b.integer(event['started_at_ms'], start, start+900000)
        end = b.integer(event['ended_at_ms'], begin, start+900000)
        b.integer(event['request_bytes'], 1, 1 << 20); b.hx(event['request_sha256'])
        b.integer(event['response_bytes'], 0, 1 << 20)
        b.integer(event['downstream_bytes'], 0, event['response_bytes'])
        b.need(event['context'] in ('', 'canceled', 'deadline_exceeded')
               and event['failure'] in ('', 'upstream', 'downstream'), 'closed request outcome; no validation/hold-timeout suppression')
        if event['failure']:
            b.need(event['context'] == 'canceled', 'only observed joined cancellation may interrupt an unrelated exchange')
        if event['response_bytes']:
            b.hx(event['response_sha256']); b.integer(event['status'], 100, 599)
        else:
            b.need(event['response_sha256'] == '' and event['status'] == 0, 'no fabricated response')
        if event['forwarded_at_ms']:
            b.integer(event['forwarded_at_ms'], begin, end)
        if not event['target']:
            b.need(event['mode'] == '' and event['held_at_ms'] == event['hold_deadline_ms'] == event['released_at_ms'] == 0 and not event['lost'], 'unrelated traffic never held or lost')
            continue
        targets.append(event)
        b.need(event['path'] == '/v3/admissions' and event['request_id'] == target['request_id']
               and event['job_id'] == target['work']['job']['id'] and event['header'] == target['proof']['header']
               and event['request_bytes'] == len(requests) and event['request_sha256'] == b.sha(requests), 'same exact original signed target on every attempt')
        b.need(event['mode'] in ('retain', 'loss', 'hold-after-loss', 'replay', 'expired'), 'selected finite target mode')
        if event['held_at_ms']:
            held = b.integer(event['held_at_ms'], begin, end)
            deadline = b.integer(event['hold_deadline_ms'], held+1, held+10000)
            b.need(deadline <= target['work']['job']['expires_at_ms'], 'hold ends at original expiry')
            b.need(event['mode'] in ('retain', 'loss', 'hold-after-loss'), 'only selected calls can be held')
            if event['released_at_ms']:
                b.need(event['mode'] == 'loss' and held <= event['released_at_ms'] < deadline
                       and event['forwarded_at_ms'] >= event['released_at_ms'], 'one live release before unchanged deadline')
            else:
                b.need(event['forwarded_at_ms'] == event['status'] == event['response_bytes'] == event['downstream_bytes'] == 0
                       and event['context'] == 'canceled' and end <= deadline, 'held target canceled and joined before dispatch/deadline')
        else:
            b.need(event['hold_deadline_ms'] == event['released_at_ms'] == 0 and event['mode'] in ('replay', 'expired'), 'later exact retry has no synthetic hold')
        if event['admission'] is not None:
            b.need(event['status'] == 200 and event['api_error'] is None and event['forwarded_at_ms'] > 0, 'genuine successful forwarded target')
            record = admission_result(b, event['admission'], target, event['mode'] == 'loss')
            raw = b.canonical(event['admission']) + b'\n'
            b.need(len(raw) == event['response_bytes'] and b.sha(raw) == event['response_sha256'], 'exact original successful response bytes')
            if event['mode'] == 'loss':
                b.need(case == 'A' and event['lost'] is True and event['downstream_bytes'] == 0, 'exact one genuine success withheld before output')
                lost.append(event)
            else:
                b.need(case == 'A' and event['mode'] == 'replay' and not event['lost'], 'only historical target completion')
                successes.append(event)
        elif event['api_error'] is not None:
            error = b.keys(event['api_error'], 'version error', ordered=True)
            b.keys(error['error'], 'code outcome_unknown', ordered=True)
            b.need(error['version'] == 3 and event['forwarded_at_ms'] > 0 and not event['lost'], 'original authenticated target refusal')
            raw = b.canonical(error)+b'\n'
            b.need(len(raw) == event['response_bytes'] and b.sha(raw) == event['response_sha256'], 'exact original refusal bytes')
            code = error['error']['code']
            b.need(code in ('busy', 'rate_limited', 'outcome_unknown', 'capacity', 'expired', 'unavailable'), 'closed original retained retry outcome')
            b.truth(error['error']['outcome_unknown'], code == 'outcome_unknown')
            b.need(event['status'] == (429 if code == 'rate_limited' else 409 if code == 'expired' else 503),
                   'exact original refusal status/code pair')
            if code == 'expired':
                b.need(case == 'B' and event['mode'] == 'expired' and event['status'] == 409
                       and event['forwarded_at_ms'] >= target['work']['job']['expires_at_ms'], 'genuine authoritative post-expiry refusal')
                expired.append(event)
        else:
            b.need(event['status'] == 0 and not event['lost'], 'no unmodeled target response')
    initial = [e for e in targets if e['mode'] == 'retain']
    b.need(len(initial) == 1 and initial[0]['forwarded_at_ms'] == 0,
           'one initial unforwarded target stopped before the ticker wait')
    if case == 'A':
        b.need(len(lost) == 1 and successes and not expired, 'one inserted target success then historical recovery')
        b.same(value['lost_response'], lost[0]['admission'], 'same original captured lost response')
        for event in successes:
            b.same(event['admission']['record'], lost[0]['admission']['record'], 'exact same record on every historical response')
        b.need(any(e['downstream_bytes'] == e['response_bytes'] and not e['failure'] for e in successes), 'original historical response delivered completely')
    else:
        b.need(not lost and not successes and value['lost_response'] is None and expired, 'never-forwarded live target becomes expiry, never a birth')
        b.need(all(e['forwarded_at_ms'] == 0 or e['forwarded_at_ms'] >= target['work']['job']['expires_at_ms'] for e in targets), 'no target forwarding before original expiry')
        b.need(any(e['downstream_bytes'] == e['response_bytes'] and not e['failure'] for e in expired), 'authoritative expired result delivered completely')
    return events, initial[0], lost, successes, expired


def exact_pool_observations(b, events, target, source, pid):
    # The relay validates the actual source result before selecting the target.
    # Its deferred end timestamp may follow downstream parsing, so correlation
    # uses original start/forward and exact wire bytes, not an invented end race.
    for receipt in (target['first_receipt'], target['receipt']):
        request = b.canonical(dict(job_id=receipt['job_id'], nonce=receipt['nonce']))
        response = b.canonical(dict(version=3, binding_id=source['miners']['source_id'],
                                    settlement_config_id=source['miners']['settlement_config_id'],
                                    recipient_script=source['miners']['enrollments'][0]['payee_script'],
                                    issue_profile_id=pid, submission=receipt)) + b'\n'
        matches = [e for e in events if e['path'] in ('/pool/v3/submit', '/pool/v3/lookup')
                   and e['status'] == 200 and 0 < e['forwarded_at_ms'] <= target['first_held_at_ms']
                   and e['started_at_ms'] <= target['first_held_at_ms']
                   and e['request_sha256'] == b.sha(request) and e['request_bytes'] == len(request)
                   and e['response_sha256'] == b.sha(response) and e['response_bytes'] == len(response)]
        b.need(matches, 'exact authenticated original pool receipt observed before target selection')
    first = b.canonical(target['first_admission'])+b'\n'
    b.need(any(e['path'] == '/v3/admissions' and not e['target'] and e['status'] == 200
               and e['admission'] == target['first_admission'] and e['response_bytes'] == len(first)
               and e['response_sha256'] == b.sha(first) and 0 < e['forwarded_at_ms'] <= target['first_held_at_ms']
               for e in events), 'genuine first winner admission precedes target selection')


def final_payment(b, public, report, source, remote, econ, rows, final, box_value):
    reopened = public.json('05-stopped-reopen.json')
    b.snapshot(reopened, remote, report['remote_id'], box_value['records'], box_value['pending'])
    mature = b.keys(public.json('06-mature.json'), 'snapshot due_ms observed_at_ms height economics')
    b.same(mature['snapshot']['view'], reopened['view'], 'same original stopped wallet view')
    mature_wallet = b.snapshot(mature['snapshot'], remote, report['remote_id'], box_value['records'], box_value['pending'])
    b.need(mature_wallet['wallet_state'] == 'ready' and mature_wallet['wallet_height'] >= mature['height']
           and mature['height'] >= max(x['block_height'] for x in rows['accepted'])+100
           and mature['due_ms'] == report['due_ms'] and report['start_ms'] <= mature['observed_at_ms'] < report['due_ms'],
           'actual original funding matures before unchanged real UTC due')
    expected_econ = {k: econ[k] for k in ('accepted_rewards', 'credits', 'batches', 'input_count', 'output_count', 'gross_atoms', 'miner_atoms', 'fee_atoms', 'change_atoms')}
    b.same(mature['economics'], expected_econ, 'mandatory independently derived two-reward branch')
    confirmed = b.keys(public.json('07-confirmed-payment.json'), 'payment confirmation_block confirmed_at_ms')
    payment = b.keys(confirmed['payment'], 'txid raw raw_sha256 input_count miner_atoms fee_atoms change_atoms broadcast_at_ms')
    tx = b.parse_transaction(payment['raw'])
    b.hx(confirmed['confirmation_block'])
    b.need(payment['txid'] == tx['txid'] and payment['raw_sha256'] == tx['raw_sha256']
           and payment['input_count'] == len(tx['inputs']) == 2, 'independent actual signed txid/raw/input count')
    b.need(report['due_ms'] <= payment['broadcast_at_ms'] <= confirmed['confirmed_at_ms'] <= report['start_ms']+900000,
           'actual real scheduled send and canonical confirmation within original scenario')
    for key in ('miner_atoms', 'fee_atoms', 'change_atoms'):
        b.need(payment[key] == econ[key], 'independent exact payment '+key)
    b.same(tx['outputs'], [dict(atoms=econ['miner_atoms'], script=source['settlement']['destinations'][0]['script']),
                           dict(atoms=econ['change_atoms'], script=source['settlement']['change_script'])], 'complete original recipient and change outputs')
    b.need(2000000000-sum(o['atoms'] for o in tx['outputs']) == 209 and tx['weight'] <= 836
           and tx['raw_bytes'] <= 374, 'exact full funding fee and native bound')
    wallet = b.keys(public.json('08-payment-wallet.json'), 'payment balances pages wallet_before wallet_after confirmation_block')
    b.same(wallet['payment'], payment, 'same original validated payment')
    b.same(wallet['wallet_before'], mature['snapshot'], 'same before-payment original wallet')
    b.need(wallet['confirmation_block'] == confirmed['confirmation_block'], 'same actual confirmation block')
    after = b.snapshot(wallet['wallet_after'], remote, report['remote_id'], box_value['records'], box_value['pending'])
    b.same(wallet['wallet_after']['view'], reopened['view'], 'same original wallet/view through payment')
    b.need(after['wallet_state'] == 'ready' and after['wallet_height'] >= mature['height']+1
           and after['spendable_atoms']-mature_wallet['spendable_atoms'] == 1980000000, 'exact original stopped wallet paid delta')
    b.integer(report['final_height'], mature['height']+1, report['initial_height']+180)
    b.financial_payment(final, rows, source, econ, payment, tx, report['due_ms'], report['final_height'])
    b.money_pages(wallet, source, econ, final['body'], confirmed['confirmed_at_ms'])
    b.money_refusals(public.json('money-read-evidence.json'), confirmed['confirmed_at_ms'], report['start_ms']+900000)
    sends, attempts = report['sends'], report['send_attempts']
    b.need(type(sends) is list and len(sends) == 1 and type(attempts) is list and 1 <= len(attempts) <= 16, 'one original successful scheduled broadcast')
    for call in sends+attempts:
        b.keys(call, 'raw txid at_ms', ordered=True)
        b.need(call['raw'] == payment['raw'] and report['due_ms'] <= call['at_ms'] <= report['start_ms']+900000
               and call['txid'] in ('', payment['txid']), 'no replacement raw transaction or premature send')
    b.need(sends[0]['txid'] == payment['txid'] and sends[0]['at_ms'] == payment['broadcast_at_ms'], 'same original actual successful send')
    return payment


def original_request(b, response, target, source, profile_id, financial):
    job = target['work']['job']
    intent = dict(version=1, profile_id=profile_id, request_id=target['request_id'],
                  gym_id=job['gym_id'], public_key=job['public_key'],
                  template_sha256=b.sha(b.canonical(target['work']['template'])))
    iid = b.identity('SLUG pool issue recovery intent', intent)
    b.response(response, source, profile_id, target['request_id'],
               dict(job=job, template=target['work']['template'], source_request=dict(intent_id=iid)), 'work')
    result = response['result']
    saved = dict(version=1, intent=intent, intent_id=iid, state='work', job_id=job['id'],
                 resolved_at_ms=result['resolved_at_ms'], reason='')
    issuer = dict(version=1, profile_id=profile_id, request_id=target['request_id'], intent_id=iid,
                  gym_id=job['gym_id'], public_key=job['public_key'], state='issued', job_id=job['id'],
                  sealed_at_ms=0, reason='')
    rows = financial['public_rows']
    for store, value in (('pool', saved), ('issuer', issuer)):
        b.need(rows.get(b.row_key(store, 'issue/r/', target['request_id'])) == b.sha(b.canonical(value)),
               'same original complete request in closed '+store)
        b.need(rows.get(b.row_key(store, 'issue/j/', job['id'])) == b.sha(b.hx(target['request_id'])),
               'same original request/job inverse in '+store)
    b.need(rows.get(b.row_key('issuer', 'job:', job['id'], False)) == b.sha(b.canonical(job)),
           'original issuer retains exact job, template binding and expiry')
    b.need(rows.get(b.row_key('pool', 'w', job['id'], False)) == b.sha(b.canonical(target['work']))
           and b.row_key('pool', 'issue/t/', target['request_id']) not in rows,
           'original completed work retains full template without pending template duplicate')
    return response


def source_census(b, financial, source):
    rows = financial['public_rows']
    raw = {store: [bytes.fromhex(k.partition('/')[2]) for k in rows if k.startswith(store+'/')]
           for store in ('issuer', 'pool')}
    for store, prefix, width, field in (
            ('pool', b'w', 65, 'works'), ('pool', b's', 65, 'submissions'),
            ('pool', b'q', 9, 'submissions'), ('pool', b'a', 9, 'accepted'),
            ('pool', b'issue/r/', 40, 'source_requests'),
            ('issuer', b'issue/r/', 40, 'issuer_requests'), ('issuer', b'job:', 68, 'authority_jobs')):
        b.need(sum(k.startswith(prefix) and len(k) == width for k in raw[store]) == financial[field],
               'complete original physical '+field+' census')
    b.need(2 <= financial['works'] <= financial['authority_jobs'] <= financial['issuer_requests']
           <= financial['source_requests'] <= source['pool']['max_works'], 'bounded original request/job/work totals')


def no_new_jobs(b, earlier, later):
    for key in ('works', 'authority_jobs', 'accepted', 'submissions'):
        b.need(earlier[key] == later[key], 'no new original '+key+' after full intake')
    for store, prefix in (('issuer', b'job:'), ('pool', b'w')):
        select = lambda f: {k: v for k, v in f['public_rows'].items()
                            if k.startswith(store+'/') and bytes.fromhex(k.partition('/')[2]).startswith(prefix)}
        b.same(select(earlier), select(later), 'all original retained job/work bytes unchanged after drain')
    b.need(later['source_requests'] >= earlier['source_requests']
           and later['issuer_requests'] >= earlier['issuer_requests'], 'finite later terminal requests remain counted')
    b.same([b.receipt_base(e['share']) for e in earlier['entries']],
           [b.receipt_base(e['share']) for e in later['entries']], 'unchanged complete accepted source receipts')


def lookup_trace(b, public, evidence, report, events, source, profile_id):
    trace = b.keys(public.json('receipt-lookup-evidence.json'), 'version outbox rows attempts wait_error completed', ordered=True)
    b.need(trace['version'] == 1 and trace['completed'] is True and trace['wait_error'] == '', 'complete original lookup audit')
    b.same(trace['outbox'], evidence['outbox'], 'same original closed outbox before receipt reads')
    b.same(trace['rows'], evidence['rows'], 'same complete saved rows before reads')
    winners = [r for r in evidence['rows'] if r['pool_win']]
    attempts = trace['attempts']
    b.need(type(attempts) is list and 1 <= len(attempts) <= 21*len(winners), 'bounded original lookup attempt list')
    groups, sequence, used, returned = {}, 0, set(), []
    for attempt in attempts:
        b.keys(attempt, 'sequence started_at_ms ended_at_ms deadline_at_ms error_type client_error code outcome_unknown context_error returned', ordered=True)
        n = b.integer(attempt['sequence'], 1, len(evidence['rows']))
        b.need(n >= sequence and evidence['rows'][n-1]['pool_win'] is True, 'original sequential winning-row reads')
        sequence = n
        b.integer(attempt['started_at_ms'], report['start_ms'], report['start_ms']+900000)
        b.integer(attempt['ended_at_ms'], attempt['started_at_ms'], report['start_ms']+900000)
        b.integer(attempt['deadline_at_ms'], attempt['ended_at_ms'], report['start_ms']+900000)
        b.need(attempt['context_error'] == '', 'no hidden canceled/expired audit result')
        groups.setdefault(n, []).append(attempt)
    b.need(set(groups) == {r['sequence'] for r in winners}, 'every original winning row audited')
    for row in winners:
        group = groups[row['sequence']]
        deadline = group[0]['deadline_at_ms']
        b.need(0 < deadline-group[0]['started_at_ms'] <= 20000 and len(group) <= 21, 'original fixed twenty-second read budget')
        for i, attempt in enumerate(group):
            b.need(attempt['deadline_at_ms'] == deadline, 'no renewed read deadline')
            if i:
                b.need(attempt['started_at_ms'] >= group[i-1]['ended_at_ms']+1000, 'one-second definite retry pacing')
            if i < len(group)-1:
                b.need(attempt['error_type'] == '*poolservice.ClientError' and attempt['client_error'] is True
                       and attempt['code'] in ('busy', 'rate_limited') and attempt['outcome_unknown'] is False
                       and attempt['returned'] is None, 'only definite unchanged read retries')
            else:
                b.need(attempt['error_type'] == attempt['code'] == '' and attempt['client_error'] is False
                       and attempt['outcome_unknown'] is False and attempt['returned'] is not None,
                       'full successful authenticated receipt audit; no missing reward')
                r = attempt['returned']
                b.need(r['job_id'] == row['work']['job']['id'] and r['nonce'] == row['nonce']
                       and r['work_hash'] == row['work_hash'] and r['duplicate_key'] == row['duplicate_key']
                       and r['pool'] is True and r['block'] is True, 'exact original work and nonce receipt')
                b.same(b.receipt_base(r), b.receipt_base(row['pool_receipt']), 'same original durable receipt')
                returned.append(r)
            request = b.canonical(dict(job_id=row['work']['job']['id'], nonce=row['nonce']))
            matches = [e for e in events if e['path'] == '/pool/v3/lookup'
                       and attempt['started_at_ms'] <= e['started_at_ms'] <= attempt['ended_at_ms']
                       and e['request_bytes'] == len(request) and e['request_sha256'] == b.sha(request)]
            b.need(len(matches) == 1 and matches[0]['ordinal'] not in used, 'one correlated actual lookup exchange')
            event = matches[0]; used.add(event['ordinal'])
            b.need(event['forwarded_at_ms'] > 0 and not event['failure'] and not event['context'], 'original lookup delivered without transport failure')
            if attempt['returned'] is None:
                envelope = dict(version=3, error=dict(code=attempt['code'], outcome_unknown=False))
                b.need(event['status'] == (429 if attempt['code'] == 'rate_limited' else 503), 'actual definite server read refusal')
            else:
                envelope = dict(version=3, binding_id=source['miners']['source_id'],
                                settlement_config_id=source['miners']['settlement_config_id'],
                                recipient_script=source['miners']['enrollments'][0]['payee_script'],
                                issue_profile_id=profile_id, submission=attempt['returned'])
                b.need(event['status'] == 200, 'actual selected receipt response')
            raw = b.canonical(envelope)+b'\n'
            b.need(event['response_bytes'] == event['downstream_bytes'] == len(raw)
                   and event['response_sha256'] == b.sha(raw), 'exact complete selected receipt/refusal wire bytes')
    b.same(sorted(returned, key=lambda r:r['accepted_sequence']), evidence['accepted'], 'complete exact two-reward receipt list')


def check(owned: Path, case: str, probe_pid: int):
    """Return exact absolute PUBLIC file -> {bytes,sha256}; raise on refusal.

    Root's runner separately owns source/profile/binary/fixture before/after
    checks and final rechecking of every returned public identity. No private
    tree or DB is opened here and no source mutation claim is inferred from a
    helper boolean.
    """
    b = original_reader()
    b.need(case in ('A', 'B'), 'one explicit retained-admission case')
    ref, econ = reference(b)
    public = b.Public(owned)
    report = public.json('actual-report.json')
    common = 'version case passed completed start_ms scope public_seed_mining due_ms setup_id remote_id issue_profile_id initial_height initial_tip tls_relay send_error sends send_attempts operator_generations cli_generations node_cleanup intentional_crash_count cleanup_errors clean_join seconds'
    complete_fields = 'completed case request_id job_id target_prospect_id target_state target_records accepted_rewards accepted_fighters pending_fighters original_signatures stopped_reopen paid_atoms fee_atoms send_count attempts final_audit_unchanged final_height initial_source_requests final_source_requests initial_issuer_requests final_issuer_requests initial_outbox_requests final_outbox_requests'
    b.keys(report, common+' '+' '.join(k for k in complete_fields.split() if k not in common.split())+' miner_atoms scheduled_payment original_wallet_received')
    b.need(report['version'] == 1 and report['case'] == case
           and report['scope'] == 'finite retained fighter admission at pool-only drain with original operator and headless CLI', 'exact finite scenario scope')
    for k in ('passed', 'completed', 'clean_join', 'stopped_reopen', 'final_audit_unchanged', 'scheduled_payment', 'original_wallet_received'):
        b.truth(report[k])
    for k in ('public_seed_mining', 'send_error'):
        b.truth(report[k], False)
    b.need(report['cleanup_errors'] == [] and type(report['seconds']) in (int, float)
           and 0 < report['seconds'] <= 1080, 'all original cleanup and unchanged outer bound')
    b.hx(report['initial_tip'])
    selection = public.json('01-selection.json')
    source, remote, profile_id = b.selected(selection, report, ref)
    ops, clis = original_processes(b, public, report, case, probe_pid, selection)
    before = b.keys(public.json('02-retained-before-forward.json'), 'target rows outbox target_entry request audit drain_status cli operator')
    target = target_identity(b, before['target'], source, report['start_ms'])
    state = 'accepted' if case == 'A' else 'expired'
    events, initial_event, losses, recovered, expired = relay_evidence(b, report['tls_relay'], target, source, profile_id, case, report['start_ms'])
    b.need(initial_event['started_at_ms'] <= target['first_held_at_ms'] <= initial_event['held_at_ms'],
           'original target capture precedes its bounded first hold')
    exact_pool_observations(b, events, target, source, profile_id)
    b.same(before['cli'], clis[0], 'original first CLI joined before initial closed audit')
    b.same(before['operator'], ops[4], 'original first operator joined before initial closed audit')
    initial = closed_admission(b, before['audit'], source, public.root, target, False)
    source_census(b, initial, source)
    b.need(initial['header']['batches'] == initial['header']['credits'] == 0, 'pre-maturity closed audit has no payment or credit')
    request = original_request(b, before['request'], target, source, profile_id, initial)
    box(b, before['outbox'], source['pool'])
    b.need(before['outbox']['records'] == len(before['rows']) and before['outbox']['active_request_id'] != target['request_id'], 'completed target request remains retained before delivery')
    initial_entry = target_entry(b, before['rows'], target, 'pending')
    b.same(before['target_entry'], initial_entry, 'original signed pending target captured')
    ready = readiness(b, before['drain_status'], ops[4], report, target)
    b.need(ready['public_at_ms'] >= target['first_held_at_ms'] and ready['round_at_ms'] >= target['first_held_at_ms'], 'initial pool-only readiness follows actual target')
    if case == 'A':
        b.need(not os.path.lexists(public.root/'03-expired-before-retry.json'), 'no foreign expiry-case artifact')
        lost = b.keys(public.json('03-lost-success.json'), 'target rows outbox target_entry request response audit drain_status cli operator')
        b.same(lost['target'], target, 'same target after physical first reopen')
        b.same(lost['cli'], clis[1], 'original second CLI joined after lost reply')
        b.same(lost['operator'], ops[5], 'original second operator joined for closed audit')
        b.same(lost['response'], losses[0]['admission'], 'exact genuine withheld response')
        committed = closed_admission(b, lost['audit'], source, public.root, target, True)
        source_census(b, committed, source); no_new_jobs(b, initial, committed)
        b.same(committed['header']['batches'], 0, 'lost-response phase precedes original scheduled payment')
        b.same(committed['admissions'][lost['response']['record']['sequence']-1], lost['response']['record'], 'same original committed birth')
        b.same(original_request(b, lost['request'], target, source, profile_id, committed), request, 'complete target request unchanged after response loss')
        box(b, lost['outbox'], source['pool'])
        b.need(lost['outbox']['records'] == len(lost['rows']) and lost['outbox']['active_request_id'] != target['request_id'], 'retained completed target at lost reply')
        unchanged_prefix(b, before['rows'], lost['rows'])
        b.same(lost['target_entry'], target_entry(b, lost['rows'], target, 'pending'), 'lost success remains unacknowledged locally')
        unchanged_entry_identity(b, initial_entry, lost['target_entry'])
        readiness(b, lost['drain_status'], ops[5], report, target, losses[0]['started_at_ms'])
        prior_rows = lost['rows']
    else:
        b.need(not os.path.lexists(public.root/'03-lost-success.json'), 'no foreign inserted-response artifact')
        expiry = b.keys(public.json('03-expired-before-retry.json'), 'target expiry_ms observed_at_ms mature_at_ms height')
        b.same(expiry['target'], target, 'same never-forwarded target before expiry retry')
        b.need(expiry['expiry_ms'] == target['work']['job']['expires_at_ms'], 'unchanged original expiry')
        b.integer(expiry['observed_at_ms'], expiry['expiry_ms'], min(e['started_at_ms'] for e in expired))
        b.need(expiry['height'] == 104 and target['first_held_at_ms'] <= expiry['mature_at_ms'] <= expiry['observed_at_ms']
               and expiry['mature_at_ms'] < report['due_ms'], 'actual maturity during stopped generations before unchanged UTC due')
        prior_rows = before['rows']
    delivered = b.keys(public.json('04-completed-delivery.json'), 'target rows target_entry request drain_status response api_error cli')
    b.same(delivered['target'], target, 'same original target through final delivery')
    b.same(delivered['cli'], clis[-1], 'final original CLI joined before complete row audit')
    b.same(delivered['request'], request, 'complete original target request not replaced')
    evidence = delivered['rows']
    b.need(b.worker_rows(evidence, target['work'], source) == 2 and evidence['lookup_missing'] == [], 'exact two complete canonical pool receipts required')
    box(b, evidence['outbox'], source['pool'])
    unchanged_prefix(b, prior_rows, evidence['rows'])
    terminal_entry = target_entry(b, evidence['rows'], target, state)
    b.same(delivered['target_entry'], terminal_entry, 'same exact durable target terminal')
    unchanged_entry_identity(b, initial_entry, terminal_entry)
    completion_events = recovered if case == 'A' else expired
    readiness(b, delivered['drain_status'], ops[-1], report, target, min(e['started_at_ms'] for e in completion_events))
    final_mode = 'replay' if case == 'A' else 'expired'
    # These are independent last observations since the explicit arm. A prior
    # genuine Busy must remain visible alongside a later historical success.
    final_events = [e for e in events if e['target'] and e['mode'] == final_mode]
    seen_responses = [e['admission'] for e in final_events if e['admission'] is not None]
    seen_errors = [e['api_error'] for e in final_events if e['api_error'] is not None]
    b.same(delivered['response'], seen_responses[-1] if seen_responses else None, 'last original target success observation')
    b.same(delivered['api_error'], seen_errors[-1] if seen_errors else None, 'last original target refusal observation')
    if case == 'A':
        admission_result(b, delivered['response'], target, False)
        b.same(delivered['response']['record'], losses[0]['admission']['record'], 'same one birth across restart/recovery')
        b.same(delivered['response'], terminal_entry['fighter']['receipt'], 'actual historical response is durably acknowledged')
    else:
        b.need(delivered['response'] is None, 'expired target has no successful admission response')
        b.same(delivered['api_error'], dict(version=3, error=dict(code='expired', outcome_unknown=False)), 'exact authoritative expiry terminal')
    lookup_trace(b, public, evidence, report, events, source, profile_id)
    last = b.keys(public.json('09-final-audit.json'), 'audit outbox_after target_entry request')
    final = closed_admission(b, last['audit'], source, public.root, target, case == 'A')
    source_census(b, final, source); no_new_jobs(b, initial, final)
    b.same(original_request(b, last['request'], target, source, profile_id, final), request, 'original target request immutable through payout')
    b.same(last['outbox_after'], evidence['outbox'], 'complete q/e/h/p/meta/scan/cursor namespace unchanged after stopped reopen')
    b.same(last['target_entry'], terminal_entry, 'same exact stopped target across payment')
    if case == 'A':
        b.need(last['audit']['ownership_state'] == 'published', 'exact one target reaches original public ownership')
        b.same(last['audit']['record'], losses[0]['admission']['record'], 'original immutable target record through final publication')
    payment = final_payment(b, public, report, source, remote, econ, evidence, final, evidence['outbox'])
    complete = b.keys(public.json('10-complete.json'), complete_fields)
    for key, value in complete.items():
        b.same(report[key], value, 'exact final report projection '+key)
    expected = dict(completed=True, case=case, request_id=target['request_id'], job_id=target['work']['job']['id'],
                    target_prospect_id=last['audit']['target_prospect_id'], target_state=state,
                    target_records=1 if case == 'A' else 0, accepted_rewards=2,
                    accepted_fighters=evidence['accepted_fighters'], pending_fighters=evidence['pending_fighters'],
                    original_signatures=evidence['authorizations'], stopped_reopen=True, paid_atoms=1980000000,
                    fee_atoms=209, send_count=1, attempts=report['send_attempts'], final_audit_unchanged=True,
                    final_height=105, initial_source_requests=initial['source_requests'], final_source_requests=final['source_requests'],
                    initial_issuer_requests=initial['issuer_requests'], final_issuer_requests=final['issuer_requests'],
                    initial_outbox_requests=before['outbox']['requests'], final_outbox_requests=last['outbox_after']['requests'])
    # Map ordering in phase JSON is Go's sorted-key map order; values remain
    # exact, while typed nested record/response order is checked at its source.
    b.need(complete == expected and report['miner_atoms'] == payment['miner_atoms'] == 1980000000, 'all final assertions independently derived')
    allowed = {Path(path).name for path in public.identities}
    # Inspect only immediate names, never a private directory or its contents.
    for child in public.root.iterdir():
        if child.name.endswith(('.json', '.jsonl', '.log')):
            b.need(child.name in allowed, 'unexpected or foreign-scope public artifact')
    return public.finish()
