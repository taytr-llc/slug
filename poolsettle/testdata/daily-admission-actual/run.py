#!/usr/bin/env python3
"""Source-bound opt-in normal-node daily late-winner proof; only main executes."""
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
REFERENCE_SHA = '1d2b1462296bd022c53df9dc583db3c38cd13a8e13e84f9983f2505b3aefb08f'
REFERENCE_BYTES = 29980
TEST = '^TestDailyAdmissionActualLateWinnerFreshProcess$'
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
        need(len(raw) == identity['bytes'] and hashlib.sha256(raw).hexdigest() == identity['sha256']
             and support.identity(path) == identity, 'public evidence changed during read')
        parsed[name] = json.loads(raw)
        evidence[str(path)] = identity
        return parsed[name]
    expected = json.loads(reference.read_bytes())
    economics = expected['actual_economics']
    accounts = expected['accounts']
    report = read('actual-report.json')
    need(report.get('pass') is True and report.get('completed') is True and
         report.get('cleanup_errors') == [], 'actual proof or cleanup incomplete')
    nodes = report.get('node_cleanup')
    need(isinstance(nodes, list) and len(nodes) == 2 and all(
        n.get('clean') is True and n.get('joined') is True and
        n.get('forced_kill') is False and n.get('unexpected_exit') is False and
        n.get('wait_error') == '' and n.get('log_close_error') is False and
        n.get('stop_rpc_error') is False for n in nodes), 'node cleanup')
    first, pending, final = read('01-A.json'), read('02-B-pending.json'), read('03-complete.json')
    need(report.get('complete') == final, 'parent and child completion evidence agree')
    batch = first['batch']
    day = first['day']
    need(report['day'] == day and batch['day'] == day and
         batch['state'] == 'prepared', 'same real day and original prepared payout')
    need(batch['payments'] == [dict(gym_id=accounts[0]['gym_id'],script=accounts[0]['payee_script'],atoms=economics['a_payment_atoms'])]
         and len(batch['inputs']) == 1 and batch['fee_atoms'] == economics['a_fee_atoms']
         and batch['change']['atoms'] == economics['a_change_atoms']
         and batch['change']['script'] == expected['binding']['guard']['funding_script'], 'independent A payment and fee')
    sends, attempts = report['broadcasts'], report['broadcast_attempts']
    need(not report.get('broadcast_overflow') and len(sends) == len(attempts) == 1 and
         sends[0]['raw'] == attempts[0]['raw'] == batch['raw'] and
         sends[0]['txid'] == batch['txid'] and attempts[0]['txid'] == '' and
         attempts[0]['at_ms'] <= sends[0]['at_ms'], 'one original A send; no recovery resend')
    immutable = lambda value: {k:v for k,v in value.items() if k != 'state'}
    def snapshot(value, credits, pending_number, state):
        need(value['Credits'] == credits and value['Pending'] == pending_number and len(value['Batches']) == 1,
             'original indexed ledger cardinality')
        need(immutable(value['Batches'][0]) == immutable(batch) and value['Batches'][0]['state'] == state,
             'original reserved economic body changed')
    snapshot(first['snapshot'],1,1,'prepared')
    snapshot(pending['snapshot'],1,1,'prepared')
    snapshot(final['snapshot'],2,0,'confirmed')
    def projected(actual, name):
        want = economics['reports'][name]
        need(actual['day'] == day and actual['state'] == want['state'] and actual.get('reason', '') == want['reason']
             and len(actual['balances']) == 2, 'same-day independent report phase')
        got = {b['gym_id']:b for b in actual['balances']}
        need(len(got) == 2, 'distinct report gyms')
        for b in want['balances']:
            gym = accounts[0 if b['gym']=='gym1' else 1]['gym_id']
            need(gym in got and {k:v for k,v in got[gym].items() if k!='gym_id'} ==
                 {k:v for k,v in b.items() if k!='gym'}, 'exact independent outstanding, available, reserved and paid balance')
    projected(first['snapshot']['Report'],'a_lost_reply')
    projected(final['report'],'b60_same_day')
    need(final['height'] == 163 and final['source_id'] == expected['source_id'] and
         final['config_id'] == expected['config_id'] and final['guard_digest'] == expected['guard_digest'], 'exact original selected identities')
    def status(value, rows, blocks):
        need(value['maximum'] == expected['capacity']['maximum'] == 2 and value['works'] == 3
             and value['submissions'] == value['accepted'] == rows and value['recovery_blocks'] == blocks
             and value['mode'] == 'draining' and value['canonical_recovery'] is True and value['over_bound'] is False,
             'derived full ordinary intake and complete recovery census')
    status(pending['before_status'],2,1)
    status(pending['status'],3,2)
    status(final['status'],3,2)
    receipt = pending['receipt']; job = pending['work']['job']
    need(pending['absent_before'] is True and receipt == final['receipt'] and receipt['sequence'] == 3
         and receipt['accepted_sequence'] == 3 and receipt['block'] is True and receipt['block_height'] == 103
         and receipt['acceptance'] == 'canonical-late-winner' and receipt['block_state'] == 'canonical-observed'
         and receipt['job_id'] == job['id'] and receipt['gym_id'] == accounts[1]['gym_id'], 'one original previously unseen late-winner receipt')
    need(job['expires_at_ms']-job['issued_at_ms'] == 300000 and
         pending['expiry_observed_at_ms'] >= job['expires_at_ms'] and
         pending['canonical_observed_at_ms'] >= pending['expiry_observed_at_ms'] and
         receipt['received_at_ms'] >= pending['canonical_observed_at_ms'] and
         receipt['accepted_at_ms'] >= receipt['received_at_ms'], 'real original expiry then canonical acceptance')
    tail = pending['tail']
    # AccountingPage deliberately excludes mutable block observations.
    accounting_share = lambda value: dict(value, block_state='', block_observed_at_ms=0)
    need(tail['config'] == expected['binding']['source'] and tail['through'] == 3 and len(tail['entries']) == 3
         and tail['entries'][2]['share'] == accounting_share(receipt) and
         tail['entries'][1]['share'] == accounting_share(pending['ordinary']), 'original complete accepted source tail')
    body = first['evidence']
    need(re.fullmatch('[0-9a-f]{64}',body['body_sha256']) and 0 < body['body_bytes'] <= 2<<20,
         'bounded original immutable body identity')
    for value in (pending['evidence'],final['evidence']):
        need(value['body_sha256'] == body['body_sha256'] and value['body_bytes'] == body['body_bytes'], 'original b/1 bytes changed')
    credits = final['evidence']['credits']
    need(len(credits)==2 and credits[0] == body['credits'][0] == pending['evidence']['credits'][0], 'original A credit unchanged and one B credit')
    coin = credits[1]['coin']; block = receipt['work_hash']
    need(credits[1]['round_hash'] == block and coin['block_hash'] == block and coin['block_height'] == 103
         and coin['coinbase'] is True and coin['atoms'] == economics['b_reward_atoms']
         and coin['script'] == expected['binding']['guard']['funding_script'], 'B original canonical coinbase provenance')
    expected_claims = {'':economics['operator_b_earned'],accounts[0]['gym_id']:economics['round_b_net_by_gym'][0],accounts[1]['gym_id']:economics['round_b_net_by_gym'][1]}
    need(len(coin['claims']) == 3 and len({c['gym_id'] for c in coin['claims']}) == 3 and
         all(c['round_hash']==block and c['round_height']==103 and c['amount']==expected_claims.get(c['gym_id']) for c in coin['claims']), 'exact independently derived B claims')
    frontier = final['snapshot']['Frontier']
    need(len(frontier) == 2 and batch['change'] in frontier and coin in frontier and
         sum(c['atoms'] for c in frontier) == economics['final_frontier_atoms'], 'original carry and coinbase conservation')
    funding = {(f['txid'],f['index']):f['state'] for f in final['funding']}
    need(len(final['funding']) == len(funding) == 2 and funding[(coin['txid'],coin['index'])]=='immature'
         and funding[(batch['change']['txid'],batch['change']['index'])]=='ready', 'no advance against immature B')
    snapshot(final['pending_reopened'],1,1,'prepared')
    snapshot(final['recovered'],1,0,'confirmed')
    snapshot(final['snapshot59'],1,0,'confirmed')
    projected(final['recovered_report'],'a_recovered_before_b_credit')
    projected(final['report59'],'b59_same_day')
    r59,r60=final['round59'],final['round60']
    need(r59['confirmation'] is None and r59['observation']['canonical'] is True
         and r59['observation']['descendants']==59 and r59['observation']['tip_height']==162,
         'no earnings lock at59 descendants')
    need(r60['round']==r59['round'] and r60['confirmation']['canonical'] is True
         and r60['confirmation']['descendants']==60 and r60['confirmation']['tip_height']==163,
         'same immutable round locks at60 descendants')
    round_=r60['round']
    need(round_['sequence']==2 and round_['block_hash']==block and round_['block_height']==103
         and round_['first_accepted']==2 and round_['last_accepted']==3
         and round_['reward_atoms']==economics['b_reward_atoms'] and round_['winner']==tail['entries'][2]
         and round_['share_keys']==[pending['ordinary']['duplicate_key'],receipt['duplicate_key']]
         and round_['pool_target']==expected['binding']['source']['pool_target']
         and round_['payout_address']==expected['binding']['source']['payout_address'], 'one complete fixed B share window')
    allocations={a['gym_id']:a for a in round_['allocations']}
    need(len(allocations)==len(round_['allocations'])==2, 'two unique round allocations')
    for i,a in enumerate(accounts):
        allocation=allocations[a['gym_id']]
        need(allocation['shares']==1 and {k:allocation[k] for k in ('numerator','denominator')}==economics['round_b_gross_by_gym'][i], 'independent equal-difficulty gross share')
    # Fresh process identity and original evidence/deadline chain are checked below.
    process = report.get('recovery_process',{})
    parent,group = parent_process['pid'],parent_process['pgid']; child_pid=process.get('launched_child_pid')
    need(type(child_pid) is int and child_pid>0 and child_pid!=parent and process.get('parent_pid')==parent
         and process.get('parent_pgid')==group and process.get('child_reported_pid')==child_pid
         and process.get('child_reported_parent_pid')==parent and process.get('child_reported_pgid')==group, 'fresh recovery address space')
    need(process.get('exit_code')==0 and process.get('joined') is True and process.get('forced') is False
         and process.get('clean') is True and process.get('wait_error')=='' and process.get('log_close_error') is False
         and process.get('child_completed') is True and process.get('child_owners_closed') is True
         and process.get('child_cleanup_errors')==[], 'original recovery process joined')
    handoff,child=read('recovery-handoff.json'),read('recovery-child-report.json')
    identity=lambda name:evidence[str(out/name)]['sha256']
    need(handoff['version']==child['version']==1 and handoff['parent_pid']==child['parent_pid']==parent
         and handoff['parent_pgid']==child['pgid']==group and child['pid']==child_pid
         and handoff['scenario_deadline_ns']==child['scenario_deadline_ns']==process['scenario_deadline_ns']
         and handoff['day']==day, 'original child identity and deadline')
    need(handoff['root']==str(out.parent/'fixture') and handoff['output']==str(out) and handoff['reference']==str(reference)
         and handoff['phase_a_sha256']==identity('01-A.json') and handoff['phase_b_sha256']==identity('02-B-pending.json')
         and child['handoff_sha256']==process['handoff_sha256']==identity('recovery-handoff.json')
         and process['child_report_sha256']==identity('recovery-child-report.json')
         and child['complete_sha256']==identity('03-complete.json'), 'original public evidence hash chain')
    need(child['pass'] is True and child['completed'] is True and child['owners_closed'] is True
         and child['cleanup_errors']==[] and child['reopens']==1 and not child.get('failure'), 'one completed cold recovery')
    child_log=out/'recovery-child.log';evidence[str(child_log)]=support.identity(child_log)
    need(0 < evidence[str(child_log)]['bytes'] <= 16<<20, 'bounded joined child log')
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
    need(lock.get('scope') == 'set78-daily-admission-actual' and lock.get('approved_by') == 'root',
         'selected root execution lock required')
    reference = Path(lock['reference'])
    required = {Path(__file__).resolve(), reference, *PINNED,
        ROOT/'build-profile.json', ROOT/'scripts/candidate-go.sh',
        ROOT/'go.mod', ROOT/'go.sum', BASE/'slugwallet/go.work',
        BASE/'slugwallet/go.mod', BASE/'slugwallet/go.sum'}
    required.update(ROOT/'poolsettle'/name for name in (
        'daily_actual_test.go', 'daily_actual_export_test.go',
        'daily_actual_node_test.go', 'daily_actual_relay_test.go',
        'daily_actual_process_test.go', 'daily_admission_actual_test.go',
        'daily_admission_actual_process_test.go', 'daily_admission_actual_export_test.go'))
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
    need(support.identity(reference) == dict(bytes=REFERENCE_BYTES, sha256=REFERENCE_SHA),
         'independent reference changed')
    out = BASE/'artifacts/set78'/args.attempt
    out.mkdir(mode=0o700)
    report = dict(version=1, scope='set78-daily-admission-actual', passed=False,
                  actual_started=False, commands=[], max_blocks=163,
                  scenario_seconds=900, test_seconds=1020, outer_seconds=1080)
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
                '-test.timeout=17m', '-test.v']
        report['actual_started'] = True
        row = inherited.capture(argv, ROOT/'poolsettle', actual_env, out/'actual.log', 1080,
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
