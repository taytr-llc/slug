#!/usr/bin/env python3
"""Root-only actual WIRE2 proof; no host or node lifecycle is inferred here."""
import argparse
from decimal import Decimal
import json
import os
from pathlib import Path
import signal
import subprocess
import time

from support import Helper, canonical, directory, exclusive, identifier, need, parse, read_file, sha, write_json
import expectations as expected
from catalog import snapshot_old, compare_old

W = Path('/Users/taytrrip/slug')
OLD = W / 'artifacts/set82/hosted-actual-attempt5'
OWNED = OLD / 'owned/owned-gym'
OLD_ATTACHMENT = OLD / 'remote-attachment'
PUBLIC = {
    'qualification.json': (3310, '5050e22b3b90106b2f65fa9263453bb1aa7194c80f64390bb7a8812f61ccbc79'),
    'proof-confirmed-money.json': (3168, '6f15c9b75f68fb555846ed8214c6a7269719176f9034c95fc743cd5f479e2d18'),
    'wallet-confirmed.json': (779, 'a4fd782e71f3d4fb22f36101fdb85407a77fe61ab1532b5f7af078dbbc051902'),
    'proof-chain.json': (7777, '133a15bd9432fc627d7b9ddc12ab98aadd0a346285d61f57f8f31fd3c4356e0d'),
    'public/remote.json': (6076, '81b2b901eb0329feedfeed5c2dd6eb52c3cce6cb7d6afc3f2b2e24919036811f'),
    'public/setup.json': (5509, '87db5803e70418affbf492b2716e2104d6902ec9294940141fe97a885a5c3525'),
}
PUBLIC_REPORTS = {'qualification.json', 'proof-confirmed-money.json', 'wallet-confirmed.json', 'proof-chain.json'}


def prior_evidence(name, raw):
    """Only these immutable public reports may contain exact decimal timings."""
    need(name in PUBLIC_REPORTS and (len(raw), sha(raw)) == PUBLIC[name], 'named hash-bound public report')
    def pairs(items):
        result = {}
        for key, value in items:
            need(key not in result, 'duplicate prior evidence field')
            result[key] = value
        return result
    def bad(_):
        raise RuntimeError('nonfinite prior evidence value')
    return json.loads(raw.decode('ascii'), object_pairs_hook=pairs, parse_float=Decimal, parse_constant=bad)


def poll_job(h, command, view, arm):
    until = min(h.deadline - 30, time.monotonic() + 35)
    while True:
        need(time.monotonic() < until, 'one original async operation deadline')
        r = h.call(command, view, timeout=until-time.monotonic(), sequence='1')
        need(r['job']['sequence'] == '1', 'original operation identity')
        if r['job']['state'] == 'done':
            return r
        need(r['job']['state'] == 'working' and r['job']['error'] == '', arm + ' original task failed')
        time.sleep(0.1)


def run_generation(h, config, prior_page, started_ms, previous_view):
    record = {'number': h.report['number'], 'logged_out': False, 'old_view_refused': False}
    initial = h.call('snapshot')
    expected.snapshot(initial, config, signed_out=True)
    login = h.call('login_fixture', timeout=100, fixture=expected.FIXTURE)
    view = expected.snapshot(login, config)
    record['view'] = view
    if previous_view is not None:
        need(view['view_id'] != previous_view['view_id'], 'physical reopen must mint a new original View')
        for command in ('daily_payout_read', 'wallet_activity_read'):
            refusal = h.request(command, previous_view['view_id'], sequence='1')
            need(not refusal['ok'] and refusal['error'] == 'stale_view', 'old original View was not refused')
        record['old_view_refused'] = True
    until = min(h.deadline-30, time.monotonic()+60)
    while True:
        need(time.monotonic() < until, 'original watched wallet readiness bound')
        s = h.call('snapshot', view['view_id'], timeout=until-time.monotonic())
        expected.snapshot(s, config, view)
        if s['remote']['wallet_state'] == 'ready':
            expected.snapshot(s, config, view, ready=True)
            record['wallet_before'] = s
            break
        time.sleep(0.25)

    job = h.call('daily_payout_begin', view['view_id'], sequence='1')
    need(job['sequence'] == '1' and job['capture_sequence'] == job['before_batch'] == '0' and
         job['state'] in ('working', 'done') and job['error'] == '', 'one original payout read admitted')
    read = poll_job(h, 'daily_payout_read', view['view_id'], 'payout')
    capture = expected.payout_read(read, config, view, prior_page, started_ms)
    record['payout_read'] = read
    completed = h.call('daily_payout_cancel', view['view_id'], sequence='1')
    need(completed == read, 'completed payout cancellation preserves original capture')
    observed = h.call('daily_payout_observe', view['view_id'], capture_sequence='1', batch_sequence='1', txid=expected.TXID, output_index=0)
    expected.observation(observed, capture, started_ms)
    record['payout_observation'] = observed

    job = h.call('wallet_activity_begin', view['view_id'], sequence='1')
    need(job['sequence'] == '1' and job['state'] in ('working', 'done') and job['error'] == '', 'one original own-activity read admitted')
    activity = poll_job(h, 'wallet_activity_read', view['view_id'], 'activity')
    activity_capture = expected.activity_read(activity, config, view, started_ms)
    record['activity_read'] = activity
    rows, pages, after = [], [], 0
    while True:
        page = h.call('wallet_activity_page', view['view_id'], capture_sequence='1', after=after, limit=8)
        next_after = expected.activity_page(page, activity_capture, after)
        pages.append(page)
        rows.extend(page['items'])
        if not page['has_more']:
            break
        need(next_after > after and len(pages) < 32, 'finite exact captured continuation')
        after = next_after
    need(len(rows) == activity_capture['metadata']['count'] and len({x['txid'] for x in rows}) == len(rows), 'complete captured activity without duplicates')
    targets = [x for x in rows if x['txid'] == expected.TXID]
    need(len(targets) == 1, 'original paid transaction appears exactly once')
    expected.paid_item(targets[0])
    detail = h.call('wallet_activity_detail', view['view_id'], capture_sequence='1', txid=expected.TXID)
    expected.paid_detail(detail, activity_capture, targets[0], config)
    record.update(activity_pages=pages, activity_detail=detail)
    cancelled = h.call('wallet_activity_cancel', view['view_id'], sequence='1')
    need(cancelled == activity, 'completed own-activity cancellation preserves dated capture')
    stopped = h.call('stop', view['view_id'])
    expected.snapshot(stopped, config, view, ready=True)
    need(h.call('daily_payout_read', view['view_id'], sequence='1') == read and
         h.call('wallet_activity_read', view['view_id'], sequence='1') == activity, 'Stop joins while preserving exact dated captures')
    record['wallet_after'] = stopped
    signed_out = h.call('logout', view['view_id'], timeout=45)
    expected.snapshot(signed_out, config, signed_out=True)
    record['logged_out'] = True
    return record, view


def audit_empty(binary, attachment, remote, root, number, deadline, interrupted):
    report = root / ('empty-outbox-%d.json' % number)
    out, err = exclusive(root / ('empty-outbox-%d.stdout' % number)), exclusive(root / ('empty-outbox-%d.stderr' % number))
    child = None
    result = dict(number=number, joined=False, exit_code=None, forced=False, signals=[])
    try:
        need(time.monotonic() < deadline-35 and not interrupted[0], 'closed audit remaining budget')
        child = subprocess.Popen([str(binary), '--attachment-root', str(attachment), '--remote-config', str(remote), '--report', str(report)],
                                 stdin=subprocess.DEVNULL, stdout=out, stderr=err, start_new_session=True, close_fds=True)
        result.update(pid=child.pid, pgid=child.pid, started_at_ms=time.time_ns()//1000000)
        until = min(deadline-30, time.monotonic()+20)
        while child.poll() is None and time.monotonic() < until and not interrupted[0]:
            need(out.tell() <= 1048576 and err.tell() <= 65536, 'bounded closed audit output')
            time.sleep(0.05)
        need(child.poll() is not None, 'closed audit deadline')
        result['exit_code'] = child.wait()
        result['joined'] = True
        need(result['exit_code'] == 0 and err.tell() == 0, 'original closed outbox audit failed')
        raw, _ = read_file(report, 1048576)
        value = parse(raw)
        need(value['passed'] is True and value['empty'] is True and value['original_audit_closed'] is True and
             value['projection_closed'] is True and value['state']['read_only_unchanged'] is True and
             value['old_attachment_opened'] is False and value['root_unchanged'] is True and value['config_unchanged'] is True and
             value['attachment_root'] == str(attachment) and value['pid'] == child.pid and value['parent_pid'] == os.getpid(),
             'complete selected empty outbox audit')
        result['report_sha256'], result['report_bytes'] = sha(raw), len(raw)
        return result, value
    finally:
        if child is not None and child.poll() is None:
            result['signals'].append('TERM')
            try:
                os.killpg(child.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                child.wait(timeout=5)
            except subprocess.TimeoutExpired:
                result['signals'].append('KILL')
                result['forced'] = True
                try:
                    os.killpg(child.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                child.wait(timeout=5)
            result['joined'], result['exit_code'] = True, child.returncode
        result['ended_at_ms'] = time.time_ns()//1000000
        out.close()
        err.close()
        write_json(root / ('empty-outbox-%d.process.json' % number), result)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--execution', required=True)
    args = parser.parse_args()
    os.umask(0o077)
    execution_raw, execution_identity = read_file(args.execution, 65536, True)
    lock = parse(execution_raw)
    need(set(lock) == {'version','approved_by','execution_authorized','started_at_ms','deadline_at_ms','artifacts','attachment_root','files'}, 'closed root execution selection')
    need(lock['version'] == 1 and type(lock['version']) is int and lock['approved_by'] == 'root' and lock['execution_authorized'] is True, 'root authorization required')
    now = time.time_ns()//1000000
    start, until = lock['started_at_ms'], lock['deadline_at_ms']
    need(type(start) is int and type(until) is int and start <= now < until and 0 < until-start <= 300000, 'one original bounded execution deadline')
    # One original root deadline; leave its final 45 seconds for root's host stop.
    # Child cleanup uses this earlier cutoff and cannot renew the root window.
    deadline = time.monotonic() + (until-now)/1000 - 45
    need(time.monotonic() < deadline - 60, 'original window leaves host cleanup reserve')
    root, attachment = Path(lock['artifacts']), Path(lock['attachment_root'])
    directory(root)
    need(root.is_relative_to(W/'artifacts/set83') and attachment.parent == root and not attachment.exists() and not attachment.is_symlink(), 'explicit fresh attachment in new evidence root')
    expected_files = {'helper','audit_helper','preparation','selection_check','remote_config','node_rpc','pool_credential','admission_credential','source_manifest'}
    need(set(lock['files']) == expected_files, 'exact selected inputs')
    originals, values = {}, {}
    for label, row in lock['files'].items():
        need(set(row) == {'path','bytes','sha256'}, 'selected input identity fields')
        cap = 256*1024*1024 if label in ('helper','audit_helper') else 1048576
        raw, inode = read_file(row['path'], cap, label in ('node_rpc','pool_credential','admission_credential','remote_config'))
        need(len(raw) == row['bytes'] and sha(raw) == row['sha256'], 'selected input changed: '+label)
        originals[label] = (row['path'], cap, raw, inode)
        if label not in ('helper','audit_helper','node_rpc','pool_credential','admission_credential'):
            values[label] = parse(raw)
    source = values['source_manifest']
    for row in source['files'] + source['inputs']:
        raw, inode = read_file(row['path'], 1048576)
        need(len(raw) == row['bytes'] and sha(raw) == row['sha256'], 'frozen acceptance source changed')
        originals['source:'+row['path']] = (row['path'], 1048576, raw, inode)
    prior = {}
    for name, (size, digest) in PUBLIC.items():
        path = OLD/name
        raw, inode = read_file(path, size)
        need(len(raw)==size and sha(raw)==digest, 'original public qualification changed')
        originals['prior:'+name] = (str(path),size,raw,inode)
        prior[name] = prior_evidence(name, raw) if name in PUBLIC_REPORTS else parse(raw)
    need(prior['qualification.json']['passed'] is True and prior['proof-chain.json']['passed'] is True and prior['proof-confirmed-money.json']['passed'] is True, 'prior paid qualification')
    config = values['remote_config']
    s, checked = values['preparation'], values['selection_check']
    cid = identifier('SLUG pool issue recovery attachment','v1',config)
    need(s['selection_id']=='set83-paid-ui-1' and s['selected_at_ms'] <= start and until < s['expires_at_ms'] and
         checked['passed'] is True and checked['only_remote_ca_changed'] is True and checked['selection_sha256']==lock['files']['preparation']['sha256'] and
         checked['remote_config_sha256']==lock['files']['remote_config']['sha256'] and cid==s['remote_config_id']==checked['remote_config_id'], 'exact checked fresh transport selection')
    old_remote = prior['public/remote.json']
    original_ca = config['base']['ca_certificate']
    config['base']['ca_certificate'] = old_remote['base']['ca_certificate']
    need(canonical(config)==canonical(old_remote), 'only explicit new CA may change remote selection')
    config['base']['ca_certificate'] = original_ca
    owned_identity = directory(OWNED)
    old_before = snapshot_old(str(OLD_ATTACHMENT))
    write_json(root/'old-attachment-before.json',old_before)
    report = dict(version=1,scope='Actual serial WIRE2 machine helper only; native-window CUA, whole wallet catalog, closed financial and infrastructure audits are separate gates',
                  started_at_ms=start,deadline_at_ms=until,machine_cutoff_at_ms=until-45000,host_cleanup_reserve_seconds=45,execution_sha256=sha(execution_raw),config_id=cid,
                  generations=[],processes=[],empty_audits=[],errors=[],start_calls=0,issue_calls=0,old_attachment_unchanged=False,
                  inputs_unchanged=False,original_wallet_root_unchanged=False,clean_join=False,passed=False)
    interrupted=[False]
    prior_signals={sig:signal.signal(sig,lambda _s,_f:interrupted.__setitem__(0,True)) for sig in (signal.SIGINT,signal.SIGTERM)}
    active = None
    attachment_identity = None
    previous_view = None
    command=[lock['files']['helper']['path'],'--daily-recoverable-issue','--daily-wallet-helper','--daily-wallet-helper-version=2',
             '--data-dir',str(OWNED),'--remote-attachment-root',str(attachment),
             '--remote-config',lock['files']['remote_config']['path'],'--remote-node-rpc',lock['files']['node_rpc']['path'],
             '--remote-pool-credential',lock['files']['pool_credential']['path'],'--remote-admission-credential',lock['files']['admission_credential']['path'],'--candidate-name-version=2']
    try:
        for generation in (1,2):
            need(not interrupted[0] and time.monotonic()<deadline-60,'remaining original generation budget')
            active=Helper(command,root,generation,deadline,interrupted,cid)
            report['processes'].append(active.report)
            record,view=run_generation(active,config,prior['proof-confirmed-money.json']['pages'][0],start,previous_view)
            report['generations'].append(record)
            active.close()
            active=None
            if attachment_identity is None:
                attachment_identity=directory(attachment)
            need(directory(attachment)==attachment_identity,'same new attachment across closed reopen')
            audit,value=audit_empty(lock['files']['audit_helper']['path'],attachment,lock['files']['remote_config']['path'],root,generation,deadline,interrupted)
            need(value['config_id']==cid and value['config_sha256']==lock['files']['remote_config']['sha256'], 'closed audit selected config correlation')
            report['empty_audits'].append(audit)
            if generation==1:
                first_empty=value
            else:
                need(value['state']==first_empty['state'],'new attachment logical namespace changed across stopped reopen')
            previous_view=view
    except BaseException as e:
        report['errors'].append(type(e).__name__+': '+str(e) if isinstance(e,RuntimeError) else type(e).__name__)
    finally:
        if active is not None:
            try:
                active.close()
            except BaseException as e:
                report['errors'].append('original helper cleanup: '+type(e).__name__)
        try:
            old_after=snapshot_old(str(OLD_ATTACHMENT))
            write_json(root/'old-attachment-after.json',old_after)
            comparison=compare_old(old_before,old_after)
            report['old_attachment_unchanged']=comparison['original_attachment_unchanged'] is True
            write_json(root/'old-attachment-comparison.json',comparison)
            need(report['old_attachment_unchanged'],'unopened original attachment changed')
            report['original_wallet_root_unchanged']=directory(OWNED)==owned_identity
            need(report['original_wallet_root_unchanged'],'original wallet root changed')
            for label,(path,cap,raw,inode) in originals.items():
                got,current=read_file(path,cap,label in ('node_rpc','pool_credential','admission_credential','remote_config'))
                need(got==raw and current==inode,'selected original input changed: '+label)
            need(read_file(args.execution,65536,True)==(execution_raw,execution_identity),'original execution selection changed')
            report['inputs_unchanged']=True
        except BaseException as e:
            report['errors'].append('final identity check: '+type(e).__name__)
        report['ended_at_ms']=time.time_ns()//1000000
        report['clean_join']=len(report['processes'])==2 and all(x['joined'] and x['exit_code']==0 and x['closed'] and not x['signals'] for x in report['processes'])
        report['passed']=not report['errors'] and not interrupted[0] and report['ended_at_ms']<=until and report['clean_join'] and len(report['generations'])==2 and len(report['empty_audits'])==2 and all(x['logged_out'] for x in report['generations']) and report['generations'][1]['old_view_refused'] and report['inputs_unchanged'] and report['old_attachment_unchanged'] and report['original_wallet_root_unchanged']
        write_json(root/'machine-report.json',report)
        for sig,handler in prior_signals.items():
            signal.signal(sig,handler)
    print(json.dumps({'passed':report['passed'],'report':str(root/'machine-report.json')}))
    return 0 if report['passed'] else 1


if __name__=='__main__':
    raise SystemExit(main())
