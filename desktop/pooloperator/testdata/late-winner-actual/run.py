#!/usr/bin/env python3
"""Admitted, source-bound full-intake unseen late winner and reserved payout rehearsal. No import actions."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import threading
import time
import runner_support as support

W = Path('/Users/taytrrip/slug')
R = W / 'slug'
WR = W / 'slugwallet'
HERE = Path(__file__).resolve().parent
PROFILE_HELPER = Path('/private/tmp/slug_set48_build_profile.py')
SETUP_REFERENCE = HERE / 'reference/setup-late-winner-v1.python313.json'
SETUP_SHA = '97fa4879bf955f2f321202c34a95dfc335e684031152e100979e27f0d98da0b3'
SCOPE = 'set74-late-winner-actual'
SOURCES = ('main.go', 'node_helpers.go', 'late_flow.go', 'late_audit.go', 'go.mod')
PHASES = ('01-runtime-listening.json', '02-two-blocks.json',
          '03-due-immature.json', '04-original-pending.json', '05-full-intake.json',
          '06-canonical-late.json', '07-late-restart.json',
          '08-confirmed-reservation.json', '09-combined-final.json')
CHILDREN = {'slugpoold-late-retry'}
MAX_ACTUAL_SECONDS = 1080  # Go scenario900s, remaining180s only bounded cleanup.


def need(ok, code):
    if not ok:
        raise RuntimeError(code)


def error(operation, exc):
    # Never expose arbitrary exception text, command output or a private input.
    return dict(operation=operation, type=type(exc).__name__, errno=getattr(exc, 'errno', None))


def read_json(path, limit):
    path = Path(path)
    identity = support.identity(path)
    need(0 < identity['bytes'] <= limit, 'JSON input bound')
    with path.open('rb') as stream:
        raw = stream.read(limit + 1)
    need(len(raw) == identity['bytes'] and support.identity(path) == identity, 'JSON input changed')
    return json.loads(raw)


def required_inputs():
    files = {HERE / name for name in (*SOURCES, 'run.py', 'runner_support.py',
             'RUNNER.md', 'runner-provenance.json', 'node-helpers-provenance.json',
             'README.md', 'source-manifest.json', 'runner-manifest.json', 'inherited-source.json')}
    files.update((PROFILE_HELPER, SETUP_REFERENCE, R / 'build-profile.json',
                  R / 'scripts/candidate-go.sh', R / 'desktop/go.work',
                  R / 'desktop/go.mod', WR / 'go.mod', R / 'go.mod'))
    # These independently selected product files close the source-only runner
    # against the actual operator slice installed for this execution.
    for name in ('command.go', 'construction.go', 'runtime.go', 'local_actions.go',
                 'maintenance.go', 'setup.go', 'filesystem_unix.go',
                 'private_file_unix.go', 'private_acl_darwin.go'):
        files.add(R / 'desktop/pooloperator' / name)
    files.add(R / 'desktop/cmd/slugpoold/main.go')
    files.update(p for p in (HERE / 'reference').rglob('*') if p.is_file())
    files.update((R / 'poolshare/operator_recovery.go', R / 'poolshare/operator_admission.go', R / 'poolshare/pool.go', R / 'desktop/pooloperator/capacity_envelope.go', R / 'desktop/pooloperator/capacity_drain.go'))
    files.update(p for p in (R / 'internal/poolcapacity').glob('*.go'))
    return files


def approved(path, sha):
    need(support.identity(path)['sha256'] == sha, 'approval bytes changed')
    v = read_json(path, 1 << 20)
    need(v.get('version') == 1 and v.get('scope') == SCOPE and
         v.get('approved_by') == 'root' and v.get('execution_authorized') is True,
         'exact root execution admission required')
    files = v.get('files')
    need(isinstance(files, dict) and 20 <= len(files) <= 160, 'finite input lock required')
    need({str(p) for p in required_inputs()} <= set(files), 'required input omitted')
    for name, value in files.items():
        need(Path(name).is_absolute() and support.identity(name) == value, 'locked input changed')
    need(support.identity(SETUP_REFERENCE) == dict(bytes=28735, sha256=SETUP_SHA),
         'independent setup bytes changed')
    proof = HERE / 'reference/inputs/two-axis-v1.json'
    need(support.identity(proof) == dict(bytes=11917, sha256='699cf788958f4f11e64582a2d31b37102db0afc76fdcf19403337e226405a03c'),
         'independent two-axis proof changed')
    cases = read_json(proof, 65536)['cases']
    matching = [c for c in cases if c['name'] == 'actual-late-fifth-row-third-block']
    need(len(matching) == 1, 'exact independent late case')
    c = matching[0]
    need(c['submissions'] == 5 and c['blocks'] == 3 and c['error'] == '' and c['expected'] ==
         dict(submissions=5, max_batches=5, state_bytes=4813932, round_bytes=211357, largest_round_bytes=67043),
         'independent late reservation mismatch')
    setup = read_json(SETUP_REFERENCE, 1 << 20)['setup']
    i = c['inputs']
    need(setup['pool']['max_submissions'] == i['source_max'] == 8 and
         setup['round_limits']['max_rounds'] >= i['rounds'] == 4 and
         setup['round_limits']['max_bytes'] >= i['round_bytes'] == 8388608 and
         setup['settlement']['max_rounds'] >= i['credit_rounds'] == 4 and
         setup['settlement']['max_batches'] == i['batches'] == 8 and
         len(setup['settlement']['destinations']) == i['gyms'] == 2,
         'actual limits do not contain stricter independent fit')
    return v


def capture(argv, cwd, env, path, seconds, permitted=(), log_cap=16 << 20):
    start = time.monotonic()
    row = dict(command=list(map(str, argv)), pid=None, pgid=None, exit_code=None,
               seconds=0, timed_out=False, log_limit_exceeded=False,
               log_joined=True, capture_errors=[], log_errors=[], cleanup=None)
    limited = threading.Event()
    process = None
    drain_thread = None
    try:
        process = subprocess.Popen(argv, cwd=cwd, env=env, stdout=subprocess.PIPE,
                                   stderr=subprocess.STDOUT, start_new_session=True)
        row['pid'] = row['pgid'] = process.pid
        support.write(path.with_suffix('.process.json'),
                      dict(pid=process.pid, pgid=process.pid, argv=row['command'],
                           timeout_seconds=seconds))
        def drain():
            count = 0
            try:
                fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
                with os.fdopen(fd, 'wb') as stream:
                    while block := process.stdout.read1(65536):
                        if count + len(block) > log_cap:
                            limited.set()
                        if count < log_cap:
                            stream.write(block[:max(0, log_cap - count)])
                            stream.flush()
                        count += len(block)
            except BaseException as exc:
                row['log_errors'].append(error('log_drain', exc))
            finally:
                try:
                    process.stdout.close()
                except OSError as exc:
                    row['log_errors'].append(error('log_pipe_close', exc))
        drain_thread = threading.Thread(target=drain, daemon=True)
        drain_thread.start()
        deadline = start + seconds
        while process.poll() is None:
            if limited.is_set() or row['log_errors']:
                break
            if time.monotonic() >= deadline:
                row['timed_out'] = True
                break
            try:
                process.wait(timeout=min(.25, max(.001, deadline - time.monotonic())))
            except subprocess.TimeoutExpired:
                pass
        row['exit_code'] = process.returncode
    except BaseException as exc:
        row['capture_errors'].append(error('runtime_wait_or_setup', exc))
    finally:
        # Preserve the actual wait result and elapsed even when cleanup fails.
        if process is not None:
            row['exit_code'] = process.poll()
        row['seconds'] = round(time.monotonic() - start, 3)
        try:
            if process is not None:
                row['cleanup'] = support.cleanup(process)
        except BaseException as exc:
            row['capture_errors'].append(error('cleanup', exc))
        finally:
            if drain_thread is not None:
                drain_thread.join(timeout=5)
                row['log_joined'] = not drain_thread.is_alive()
            elif process is not None and process.stdout is not None:
                process.stdout.close()
        row['log_limit_exceeded'] = limited.is_set()
        if process is not None and (not row['cleanup'] or row['cleanup'].get('errors') or
                                    not row['cleanup'].get('group_empty')):
            row['cleanup_census'] = support.owned_group_census(process.pid, set(map(str, permitted)) | {str(argv[0])})
    support.write(path.with_suffix('.command.json'), row)
    return row


def okay(row):
    c = row['cleanup'] or {}
    return (row['exit_code'] == 0 and not row['timed_out'] and
            not row['log_limit_exceeded'] and row['log_joined'] and
            not row['capture_errors'] and not row['log_errors'] and
            c.get('group_empty') is True and c.get('leftover_after_exit') is False and
            not c.get('signals') and not c.get('errors'))


def dependency_files(path):
    raw = path.read_bytes()
    need(len(raw) <= 32 << 20, 'dependency output bound')
    decoder, text, offset, files = json.JSONDecoder(), raw.decode(), 0, set()
    count = 0
    while offset < len(text):
        while offset < len(text) and text[offset].isspace():
            offset += 1
        if offset == len(text):
            break
        item, offset = decoder.raw_decode(text, offset)
        count += 1
        need(count <= 4096 and not item.get('Error') and not item.get('DepsErrors'),
             'dependency resolution refused')
        directory = Path(item['Dir'])
        for field in ('GoFiles', 'CgoFiles', 'CFiles', 'CXXFiles', 'MFiles',
                      'HFiles', 'FFiles', 'SFiles', 'SysoFiles', 'EmbedFiles'):
            files.update((directory / name).resolve() for name in item.get(field, []))
        module = item.get('Module', {})
        module = module.get('Replace', module)
        if module.get('GoMod'):
            mod = Path(module['GoMod']).resolve()
            files.add(mod)
            if mod.with_name('go.sum').is_file():
                files.add(mod.with_name('go.sum'))
    need(count > 0 and len(files) <= 12000, 'dependency closure bound')
    return files


def copy_probe(artifact):
    probe = artifact / 'probe-source'
    probe.mkdir(mode=0o700)
    mapping = {}
    for name in SOURCES:
        source, target = HERE / name, probe / name
        shutil.copyfile(source, target)
        os.chmod(target, 0o600)
        mapping[str(target)] = dict(source=str(source), **support.identity(source))
    source = R / 'desktop/go.sum'
    shutil.copyfile(source, probe / 'go.sum')
    mapping[str(probe / 'go.sum')] = dict(source=str(source), **support.identity(source))
    support.write(artifact / 'source-copies.json', mapping)
    modules = [probe, R / 'desktop', WR, WR / 'wallet/txauthor', WR / 'wallet/txrules',
               WR / 'wallet/txsizes', WR / 'walletdb', WR / 'wtxmgr', R]
    modules += [R / name for name in ('address', 'btcec', 'btcutil', 'chaincfg', 'chainhash',
                                     'psbt', 'txscript', 'v2transport', 'wire')]
    work = artifact / 'go.work'
    raw = 'go 1.27.4\nuse (\n' + ''.join('\t' + json.dumps(str(p)) + '\n' for p in modules) + ')\n'
    fd = os.open(work, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'w') as stream:
        stream.write(raw)
    return probe, work, mapping


def check_copies(mapping):
    return all(support.identity(target) == support.identity(row['source']) ==
               dict(bytes=row['bytes'], sha256=row['sha256']) for target, row in mapping.items())


def public_result(owned):
    v = read_json(owned / 'actual-report.json', 2 << 20)
    need(v.get('version') == 1 and v.get('pass') is True and v.get('completed') is True and
         v.get('clean_join') is True and v.get('public_seed_mining') is False and
         type(v.get('final_height')) is int and v['final_height'] == 165 and
         v.get('normal_blocks_max') == 180 and v.get('cleanup_errors') == [],
         'incomplete actual scenario')
    need(v.get('late_credit_count') == 1 and v.get('ordinary_intake') == 4 and
         v.get('late_rows') == 5 and v.get('late_block_rows') == 3 and
         v.get('pending_batch_unchanged') is True and v.get('exact_restart_receipt') is True and
         v.get('late_births') == 0 and v.get('real_expiry') is True, 'combined late winner proof incomplete')
    nodes, children = v.get('node_cleanup'), v.get('child_cleanup')
    need(isinstance(nodes, list) and len(nodes) == 2 and all(
        n.get('clean') is True and n.get('joined') is True and n.get('forced_kill') is False and
        n.get('unexpected_exit') is False and n.get('wait_error') == '' and
        n.get('log_close_error') is False and n.get('stop_rpc_error') is False for n in nodes),
        'node cleanup incomplete')
    need(isinstance(children, list) and len(children) == 1 and
         {c.get('name') for c in children} == CHILDREN and all(
        c.get('clean') is True and c.get('joined') is True and c.get('forced') is False and
        c.get('wait_error') == '' and c.get('log_close_error') == '' and
        type(c.get('pid')) is int and c['pid'] > 1 for c in children), 'child cleanup incomplete')
    broadcasts = v.get('broadcasts')
    need(isinstance(broadcasts, list) and 1 <= len(broadcasts) <= 32, 'public broadcast count')
    first = broadcasts[0]
    need(isinstance(first.get('raw'), str) and re.fullmatch(r'[0-9a-f]{2,200000}', first['raw']) and
         len(first['raw']) % 2 == 0 and re.fullmatch(r'[0-9a-f]{64}', first.get('txid', '')),
         'public transaction framing')
    need(all(b.get('raw') == first['raw'] and b.get('txid') == first['txid'] for b in broadcasts),
         'recovery bytes changed')
    attempts = v.get('broadcast_attempts')
    need(v.get('broadcast_attempt_overflow') is False and isinstance(attempts, list) and
         1 <= len(attempts) <= 16 and all(a.get('raw') == first['raw'] for a in attempts),
         'attempted recovery bytes changed or inventory overflow')
    evidence = {str(owned / 'actual-report.json'): support.identity(owned / 'actual-report.json')}
    for name in (*PHASES, 'selected-public-setup.json'):
        p = owned / name
        read_json(p, 65 << 20)
        evidence[str(p)] = support.identity(p)
    bootstrap = read_json(owned / '02-two-blocks.json', 2 << 20)
    need(isinstance(bootstrap, list) and len(bootstrap) == 2 and all(
        row['receipt']['sequence'] == i+1 and row['receipt']['accepted_sequence'] == i+1 and
        row['receipt']['block'] is True and row['receipt']['block_state'] == 'canonical-observed' and
        row['receipt']['block_height'] == i+2 and row['ordinary_submissions'] == 0 and
        row['fighter_submissions'] == 0 and row['searched_nonces'] >= 1 and row['searched_nonces'] <= 64
        for i, row in enumerate(bootstrap)), 'deterministic block-only bootstrap incomplete')
    full = read_json(owned / '05-full-intake.json', 65 << 20)
    need(full.get('issue_denial') == dict(code='outcome_unknown', outcome_unknown=True) and
         full['audit'].get('authority_jobs') == 3 and full['audit'].get('works') == 3 and
         full['audit'].get('submissions') == 4 and full['audit'].get('accepted') == 4,
         'closed Issue uncertainty not reconciled against original issuer/pool')
    canonical = read_json(owned / '06-canonical-late.json', 65 << 20)
    need(canonical.get('ordinary_refusal') == dict(code='capacity', outcome_unknown=False) and
         canonical.get('noncanonical_refusal') == dict(code='outcome_unknown', outcome_unknown=True) and
         type(canonical.get('expiry_observed_at_ms')) is int and
         canonical['expiry_observed_at_ms'] >= canonical['job']['expires_at_ms'] and
         type(canonical.get('winner_observed_at_ms')) is int and
         canonical['winner_observed_at_ms'] >= canonical['job']['expires_at_ms'] and
         canonical['absent_audit'].get('authority_jobs') == 3 and
         canonical['absent_audit'].get('works') == 3 and
         canonical['absent_audit'].get('submissions') == 4 and
         canonical['absent_audit'].get('accepted') == 4,
         'exact original negative submissions not reconciled')
    # All are public, originally closed physical audits. No private fixture file is read.
    final = read_json(owned / '09-combined-final.json', 65 << 20)
    need(final.get('late_credit_count') == 1 and final.get('original_batch_unchanged') is True and
         final.get('final_height') == 165 and final['audit'].get('read_only_unchanged') is True and
         len(final['audit']['entries']) == 5 and len(final['audit']['rounds']) == 3 and
         len(final['audit']['credits']) == 3 and len(final['audit']['batches']) == 1 and
         final['audit'].get('authority_jobs') == 3,
         'closed final public audit incomplete')
    return evidence


def run(args):
    lock_path = Path(args.approved_lock)
    lock = approved(lock_path, args.approved_lock_sha256)
    artifact = W / 'artifacts/set74' / args.attempt
    artifact.mkdir(mode=0o700)  # never erase or reuse any attempt, including failure.
    report = dict(version=1, passed=False, scope=SCOPE, actual_started=False, commands=[],
                  fixture_timeout_seconds=900, outer_timeout_seconds=MAX_ACTUAL_SECONDS, max_blocks=180)
    before, mapping, built, bound, bp = None, None, {}, None, None
    source_files = set()
    try:
        support.write(artifact / 'execution-lock.json', lock)
        spec = importlib.util.spec_from_file_location('actual_profile', PROFILE_HELPER)
        bp = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(bp)
        bound = bp.binding()
        support.write(artifact / 'profile.json', bound)
        probe, work, mapping = copy_probe(artifact)
        env = bp.environment(GOCACHE='/private/tmp/slug-go-build',
                             GOMODCACHE='/private/tmp/slug-desktop-modcache', GOPROXY='off',
                             GOSUMDB='off', GOWORK=str(work), GOMAXPROCS='2', CGO_ENABLED='1')
        for key in list(env):
            if key.startswith('SLUG_'):
                del env[key]
        env.update(PYTHONDONTWRITEBYTECODE='1', GOOS='darwin', GOARCH='arm64')
        node_env = dict(env, CGO_ENABLED='0')
        source_files = {Path(p) for p in lock['files']} | {lock_path, work} | set(map(Path, mapping))
        # Three exact go-list closures, including native cgo leaves for operator
        # and probe. Queries are also owned bounded process groups with logs.
        for name, cwd, query_env in [('node', R, node_env), ('operator', R / 'desktop/cmd/slugpoold', env), ('probe', probe, env)]:
            path = artifact / (name + '-dependencies.json')
            row = capture(['go', 'list', '-deps', '-json', '.'], cwd, query_env, path, 120, log_cap=32 << 20)
            report['commands'].append(row)
            need(okay(row), 'dependency query failed')
            source_files.update(dependency_files(path))
        work_sum = work.with_suffix('.work.sum')
        if work_sum.is_file():
            source_files.add(work_sum)
        before = {str(p): support.identity(p) for p in sorted(source_files)}
        support.write(artifact / 'source-before.json', before)
        report['source_copies_exact_before'] = check_copies(mapping)
        need(report['source_copies_exact_before'], 'source copies changed')
        approved(lock_path, args.approved_lock_sha256)
        binary_dir = artifact / 'bin'
        binary_dir.mkdir(mode=0o700)
        for name, filename, cwd, build_env, cgo in [('node', 'slugd', R, node_env, 0),
             ('operator', 'slugpoold', R / 'desktop/cmd/slugpoold', env, 1),
             ('probe', 'late-winner-actual', probe, env, 1)]:
            target = binary_dir / filename
            row = capture(bp.command('build', '-p=1', '-mod=readonly', '-trimpath', '-tags=', '-o', target, '.'),
                          cwd, build_env, artifact / (name + '-build.log'), 240)
            report['commands'].append(row)
            need(okay(row), 'normal binary build failed')
            built[name] = bp.artifact(target, artifact / (name + '-build-info.txt'), tags='', cgo=cgo)
        report['artifacts'] = built
        need(all(support.identity(p) == value for p, value in before.items()), 'source changed before actual')
        bp.same(bound)
        bp.unchanged(built)
        approved(lock_path, args.approved_lock_sha256)
        argv = [built['probe']['path'], '--node', built['node']['path'], '--operator-binary', built['operator']['path'],
                '--artifacts', str(artifact / 'owned'), '--setup-reference', str(SETUP_REFERENCE)]
        report['actual_started'] = True
        row = capture(argv, probe, env, artifact / 'actual.log', MAX_ACTUAL_SECONDS,
                      permitted=[x['path'] for x in built.values()])
        report['commands'].append(row)
        need(okay(row), 'actual command or cleanup failed')
        report['public_evidence'] = public_result(artifact / 'owned')
        report['passed'] = True
    except BaseException as exc:
        report['error'] = error('runner_or_actual', exc)
    finally:
        report['artifacts'] = built
        if before is not None:
            after = {}
            for p in before:
                try:
                    after[p] = support.identity(p)
                except BaseException as exc:
                    after[p] = error('source_identity', exc)
            support.write(artifact / 'source-after.json', after)
            report['source_count'] = len(before)
            report['source_unchanged'] = before == after
            if before != after:
                report['passed'] = False
        try:
            report['source_copies_exact_after'] = mapping is not None and check_copies(mapping)
            need(report['source_copies_exact_after'], 'source copies changed')
            if bp is None or bound is None:
                raise RuntimeError('profile binding unavailable')
            bp.unchanged(built)
            bp.same(bound)
            approved(lock_path, args.approved_lock_sha256)
            report['profile_lock_artifacts_unchanged'] = True
        except BaseException as exc:
            report['passed'] = False
            report['profile_lock_artifacts_unchanged'] = False
            report['final_binding_error'] = error('final_binding', exc)
        if report.get('public_evidence'):
            try:
                report['public_evidence_unchanged'] = all(support.identity(p) == v for p, v in report['public_evidence'].items())
            except BaseException as exc:
                report['public_evidence_unchanged'] = False
                report['evidence_binding_error'] = error('public_evidence_identity', exc)
            if not report['public_evidence_unchanged']:
                report['passed'] = False
        support.write(artifact / 'report.json', report)
    print(json.dumps(dict(passed=report['passed'], artifact=str(artifact))))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--execute', action='store_true')
    parser.add_argument('--approved-lock', required=True)
    parser.add_argument('--approved-lock-sha256', required=True)
    parser.add_argument('--attempt', required=True)
    a = parser.parse_args()
    if (not a.execute or not re.fullmatch(r'late-winner-actual-attempt[1-9][0-9]*', a.attempt) or
        not re.fullmatch(r'[0-9a-f]{64}', a.approved_lock_sha256) or not Path(a.approved_lock).is_absolute()):
        parser.error('explicit execute, exact root lock and fresh finite attempt required')
    try:
        sys.exit(run(a))
    except BaseException as exc:
        if isinstance(exc, SystemExit):
            raise
        print(json.dumps(dict(passed=False, error=error('admission_or_report', exc))))
        sys.exit(1)
