#!/usr/bin/env python3
"""Source-bound opt-in normal-node daily payout proof; only main executes."""
from pathlib import Path
import argparse, hashlib, importlib.util, json, os, re, sys

BASE = Path('/Users/taytrrip/slug')
ROOT = BASE/'slug'
LEGACY = ROOT/'desktop/pooloperator/testdata/late-winner-actual'
PROFILE = Path('/private/tmp/slug_set48_build_profile.py')
PINNED = {
    LEGACY/'run.py': 'b81af69c02a18b7f74ed96e08ae4dba8548e94a4958aeacb721c62dcbf8ba05e',
    LEGACY/'runner_support.py': '54bb3e7caf33e744ce76bf366dd23a4eff4d370d9246b85758089b57dd2439d7',
    PROFILE: 'b6033d66edb4d24a57a7fac4e59ddaeee6964f0943aca21fddc6f65c7f5e6ee4',
}
REFERENCE_SHA = '9bcb4b64505e38de193411dcf51fb19f4d1332cbb2af53d21904663848cb4955'
TEST = '^TestDailyActual721Inputs64Gyms$'
TAG = 'slugdailyactual'

def need(ok, reason):
    if not ok:
        raise RuntimeError(reason)

def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value

def public_result(out, reference, support, parent_process):
    parsed, evidence = {}, {}
    def read(name):
        if name in parsed:
            return parsed[name]
        path = out/name
        identity = support.identity(path)
        need(0 < identity['bytes'] <= 16 << 20, 'public evidence bound')
        with path.open('rb') as stream:
            raw = stream.read((16 << 20)+1)
        need(len(raw) == identity['bytes'] and
             hashlib.sha256(raw).hexdigest() == identity['sha256'] and
             support.identity(path) == identity, 'public evidence changed during read')
        parsed[name] = json.loads(raw)
        evidence[str(path)] = identity
        return parsed[name]
    report = read('actual-report.json')
    need(report.get('pass') is True and report.get('completed') is True and
         report.get('cleanup_errors') == [], 'actual proof or cleanup incomplete')
    for key, value in dict(height=924, inputs=721, payees=64,
                           change_origins=721, batches=2, reopens=1).items():
        need(report.get(key) == value, 'actual cardinality '+key)
    nodes = report.get('node_cleanup')
    need(isinstance(nodes, list) and len(nodes) == 2 and all(
        n.get('clean') is True and n.get('joined') is True and
        n.get('forced_kill') is False and n.get('unexpected_exit') is False and
        n.get('wait_error') == '' and n.get('log_close_error') is False and
        n.get('stop_rpc_error') is False for n in nodes), 'node cleanup')
    sends, attempts = report.get('broadcasts'), report.get('broadcast_attempts')
    need(not report.get('broadcast_overflow') and isinstance(sends, list) and
         isinstance(attempts, list) and len(sends) == len(attempts) == 2,
         'exactly two original send attempts')
    need(sends[0]['txid'] != sends[1]['txid'] and all(
        a['raw'] == b['raw'] for a, b in zip(sends, attempts)), 'send identity')
    expected = json.loads(reference.read_bytes())
    first = read('01-A.json')['batch']
    pending = read('02-B-pending.json')
    second = pending['batch']
    final = read('03-complete.json')
    snap = final['snapshot']
    need(snap['Credits'] == 721 and snap['Pending'] == 0 and
         len(snap['Batches']) == 2 and len(snap['Frontier']) == 1,
         'complete original ledger counts')
    for index, (batch, count, payees, fee, change, field) in enumerate([
        (first, 1, 1, 141, 9999859, 'first_atoms'),
        (second, 721, 64, 51236, 7209948623, 'second_atoms')]):
        payments = [dict(gym_id=p['gym_id'], script=p['script'], atoms=p[field])
                    for p in expected['payments'] if p[field] > 0]
        need(len(batch['inputs']) == count and len(batch['payments']) == payees and
             batch['payments'] == payments and batch['fee_atoms'] == fee and
             batch['change']['atoms'] == change, 'independent payout economics')
        need(sends[index]['raw'] == batch['raw'] and sends[index]['txid'] == batch['txid'],
             'retained bytes differ from transmission')
        immutable = lambda b: {k: v for k, v in b.items() if k != 'state'}
        need(immutable(snap['Batches'][index]) == immutable(batch) and
             snap['Batches'][index]['state'] == 'confirmed', 'original signed body changed')
    need(100000 < len(second['raw'])//2 <= 400000 and
         second['inputs'][0] == first['change'], 'daily byte boundary and original carry')
    change = snap['Frontier'][0]
    need(change == second['change'] and len(change['claims']) == 721 and
         len({c['round_hash'] for c in change['claims']}) == 721 and
         all(c['gym_id'] == '' for c in change['claims']), 'complete carried origins')
    balances = final['report']['balances']
    need(len(balances) == 64 and all(b['gym_id'] == p['gym_id'] and
         b['paid_atoms'] == p['total_atoms'] and b['reserved_atoms'] == 0 and
         b['confirmed'] == dict(numerator='0', denominator='1')
         for b, p in zip(balances, expected['payments'])), 'independent final balances')
    # One fresh address space, correlated with the exact outer-owned parent.
    process = report.get('recovery_process', {})
    parent, group = parent_process['pid'], parent_process['pgid']
    child_pid = process.get('launched_child_pid')
    need(type(child_pid) is int and child_pid > 0 and child_pid != parent and
         process.get('parent_pid') == parent and process.get('parent_pgid') == group and
         process.get('child_reported_pid') == child_pid and
         process.get('child_reported_parent_pid') == parent and
         process.get('child_reported_pgid') == group, 'fresh recovery process identity')
    need(process.get('exit_code') == 0 and process.get('joined') is True and
         process.get('forced') is False and process.get('clean') is True and
         process.get('wait_error') == '' and process.get('log_close_error') is False and
         process.get('child_completed') is True and process.get('child_owners_closed') is True and
         process.get('child_cleanup_errors') == [], 'recovery process cleanup')
    handoff = read('recovery-handoff.json')
    child = read('recovery-child-report.json')
    identity = lambda name: evidence[str(out/name)]['sha256']
    need(handoff.get('version') == child.get('version') == 1 and
         handoff.get('parent_pid') == child.get('parent_pid') == parent and
         handoff.get('parent_pgid') == child.get('pgid') == group and
         child.get('pid') == child_pid and
         handoff.get('scenario_deadline_ns') == child.get('scenario_deadline_ns') ==
             process.get('scenario_deadline_ns') and
         type(process.get('scenario_deadline_ns')) is int and
         process['scenario_deadline_ns'] > 0, 'original deadline and handoff identity')
    need(handoff.get('root') == str(out.parent/'fixture') and
         handoff.get('output') == str(out) and handoff.get('reference') == str(reference) and
         handoff.get('phase_a_sha256') == identity('01-A.json') and
         handoff.get('phase_b_sha256') == identity('02-B-pending.json') and
         child.get('handoff_sha256') == process.get('handoff_sha256') ==
             identity('recovery-handoff.json') and
         process.get('child_report_sha256') == identity('recovery-child-report.json') and
         child.get('complete_sha256') == identity('03-complete.json'), 'original phase chain')
    need(child.get('pass') is True and child.get('completed') is True and
         child.get('owners_closed') is True and child.get('cleanup_errors') == [] and
         child.get('reopens') == 1 and not child.get('failure') and
         child.get('cold_source_seconds') == report.get('cold_source_seconds') and
         child.get('final_change_inspect_seconds') == report.get('final_change_inspect_seconds'),
         'one completed fresh-process reopen')
    child_log = out/'recovery-child.log'
    evidence[str(child_log)] = support.identity(child_log)
    need(0 < evidence[str(child_log)]['bytes'] <= 16 << 20, 'bounded closed child log')
    for name in ('actual-report.json', '01-A.json', '02-B-pending.json', '03-complete.json'):
        read(name)
    return evidence

def run(args):
    sys.dont_write_bytecode = True
    for path, expected in PINNED.items():
        need(hashlib.sha256(path.read_bytes()).hexdigest() == expected,
             'inherited capture/profile source changed')
    sys.path.insert(0, str(LEGACY))
    inherited = module('daily_actual_capture', LEGACY/'run.py')
    support = inherited.support
    bp = module('daily_actual_profile', PROFILE)
    lockpath = Path(args.lock)
    need(hashlib.sha256(lockpath.read_bytes()).hexdigest() == args.lock_sha256,
         'execution lock changed')
    lock = json.loads(lockpath.read_bytes())
    need(lock.get('scope') == 'set76-daily-actual' and lock.get('approved_by') == 'root',
         'selected root execution lock required')
    reference = Path(lock['reference'])
    required = {Path(__file__).resolve(), reference, *PINNED,
        ROOT/'build-profile.json', ROOT/'scripts/candidate-go.sh',
        ROOT/'go.mod', ROOT/'go.sum', BASE/'slugwallet/go.work',
        BASE/'slugwallet/go.mod', BASE/'slugwallet/go.sum'}
    required.update(ROOT/'poolsettle'/name for name in (
        'daily_actual_test.go', 'daily_actual_export_test.go',
        'daily_actual_node_test.go', 'daily_actual_relay_test.go',
        'daily_actual_process_test.go'))
    work_sum = BASE/'slugwallet/go.work.sum'
    def check_lock():
        need(hashlib.sha256(lockpath.read_bytes()).hexdigest() == args.lock_sha256,
             'execution lock changed')
        selected = required | ({work_sum} if work_sum.exists() else set())
        need({str(p) for p in selected} <= set(lock['files']),
             'mandatory execution input omitted')
        for name, value in lock['files'].items():
            need(support.identity(name) == value, 'locked source changed')
    check_lock()
    need(support.identity(reference) == dict(bytes=85070, sha256=REFERENCE_SHA),
         'independent reference changed')
    out = BASE/'artifacts/set76'/args.attempt
    out.mkdir(mode=0o700)
    report = dict(version=1, scope='set76-daily-actual', passed=False,
                  actual_started=False, commands=[], max_blocks=924,
                  scenario_seconds=1800, test_seconds=2100, outer_seconds=2400)
    before, built, bound = None, {}, None
    try:
        support.write(out/'execution-lock.json', lock)
        bound = bp.binding()
        support.write(out/'profile.json', bound)
        env = bp.environment(GOCACHE='/private/tmp/slug-go-build',
            GOMODCACHE='/private/tmp/slug-desktop-modcache', GOPROXY='off',
            GOSUMDB='off', GOWORK=str(BASE/'slugwallet/go.work'),
            GOMAXPROCS='2', CGO_ENABLED='1', GOOS='darwin', GOARCH='arm64')
        for key in list(env):
            if key.startswith('SLUG_'):
                del env[key]
        env['PYTHONDONTWRITEBYTECODE'] = '1'
        node_env = dict(env, CGO_ENABLED='0')
        files = {Path(p) for p in lock['files']} | {lockpath}
        for name, argv, selected in [
            ('node', ['go', 'list', '-deps', '-json', '.'], node_env),
            ('test', ['go', 'list', '-deps', '-test', '-json', '-tags='+TAG, './poolsettle'], env)]:
            path = out/(name+'-dependencies.json')
            row = inherited.capture(argv, ROOT, selected, path, 120, log_cap=32 << 20)
            report['commands'].append(row)
            need(inherited.okay(row), 'dependency query failed')
            files.update(inherited.dependency_files(path))
        before = {str(p): support.identity(p) for p in sorted(files)}
        support.write(out/'source-before.json', before)
        check_lock()
        (out/'bin').mkdir(mode=0o700)
        commands = [
            ('node', 'slugd', node_env, 0, '', 'build', ['.']),
            ('test', 'daily-actual.test', env, 1, TAG, 'test', ['-c', './poolsettle'])]
        for name, filename, selected, cgo, tags, verb, suffix in commands:
            target = out/'bin'/filename
            argv = bp.command(verb, '-p=1', '-mod=readonly', '-trimpath',
                              '-tags='+tags, '-o', target, *suffix)
            row = inherited.capture(argv, ROOT, selected, out/(name+'-build.log'), 300)
            report['commands'].append(row)
            need(inherited.okay(row), 'candidate build failed')
            built[name] = bp.artifact(target, out/(name+'-build-info.txt'), tags=tags, cgo=cgo)
        need(all(support.identity(p) == v for p, v in before.items()), 'source changed before proof')
        check_lock()
        bp.same(bound)
        bp.unchanged(built)
        (out/'public').mkdir(mode=0o700)
        (out/'scratch').mkdir(mode=0o700)
        actual_env = dict(env, SLUG_DAILY_ACTUAL='1', SLUG_DAILY_NODE=built['node']['path'],
            SLUG_DAILY_REFERENCE=str(reference), SLUG_DAILY_OUTPUT=str(out/'public'),
            SLUG_DAILY_ROOT=str(out/'fixture'),
            TMPDIR=str(out/'scratch'))
        argv = [built['test']['path'], '-test.run='+TEST, '-test.count=1',
                '-test.timeout=35m', '-test.v']
        report['actual_started'] = True
        row = inherited.capture(argv, ROOT/'poolsettle', actual_env, out/'actual.log', 2400,
                                permitted=[x['path'] for x in built.values()])
        report['commands'].append(row)
        need(inherited.okay(row), 'actual command or cleanup failed')
        report['public_evidence'] = public_result(out/'public', reference, support, row)
        report['fresh_process_recovery_verified'] = True
        report['passed'] = True
    except BaseException as exc:
        report['error'] = inherited.error('daily_actual_runner', exc)
    finally:
        report['artifacts'] = built
        if before is not None:
            after = {}
            for path in before:
                try:
                    after[path] = support.identity(path)
                except BaseException as exc:
                    after[path] = inherited.error('source_identity', exc)
            support.write(out/'source-after.json', after)
            report['source_count'] = len(before)
            report['source_unchanged'] = before == after
            if before != after:
                report['passed'] = False
        try:
            check_lock()
            need(bound is not None, 'profile not bound')
            bp.same(bound)
            bp.unchanged(built)
            report['profile_lock_artifacts_unchanged'] = True
        except BaseException as exc:
            report['passed'] = False
            report['profile_lock_artifacts_unchanged'] = False
            report['binding_error'] = inherited.error('final_binding', exc)
        if report.get('public_evidence'):
            try:
                report['public_evidence_unchanged'] = all(
                    support.identity(p) == v for p, v in report['public_evidence'].items())
            except BaseException as exc:
                report['public_evidence_unchanged'] = False
                report['evidence_error'] = inherited.error('final_public_identity', exc)
            if not report['public_evidence_unchanged']:
                report['passed'] = False
        support.write(out/'report.json', report)
    print(json.dumps(dict(passed=report['passed'], artifacts=str(out))))
    return 0 if report['passed'] else 1

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--lock', required=True)
    parser.add_argument('--lock-sha256', required=True)
    parser.add_argument('--attempt', required=True)
    args = parser.parse_args()
    need(re.fullmatch(r'daily-actual-attempt[1-9][0-9]*', args.attempt), 'fresh named attempt required')
    need(Path(args.lock).is_absolute() and re.fullmatch(r'[0-9a-f]{64}', args.lock_sha256), 'exact lock required')
    raise SystemExit(run(args))
