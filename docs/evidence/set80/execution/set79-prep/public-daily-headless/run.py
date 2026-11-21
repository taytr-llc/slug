#!/usr/bin/env python3
"""Admitted, source-bound existing-wallet headless DAILY CLI rehearsal. No import actions."""
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
SETUP_REFERENCE = W / 'artifacts/set79/attachment-reference-attempt1/python314.json'
BASE_REFERENCE = R / 'testdata/public-daily/reference-v1.json'
SETUP_SHA = 'a48a854d20298fced726b816d11690aa45e9f4184e3b161cbc7c258b74897386'
SCOPE = 'set79-public-daily-headless-actual'
REFERENCE_DIR = HERE.parent / 'public-daily-attachment/reference'
SOURCES = ('main.go','selection.go','runtime_helpers.go','relay.go','work_evidence.go','payment.go','operator_audit.go','node_helpers.go','cli_process.go','go.mod')
PHASES = ('01-runtime-and-selection.json','02-cli-running.json',
          '03-cli-joined.json','04-closed-rows.json','05-stopped-reopen.json',
          '06-mature-before-due.json','07-payment-and-wallet.json',
          '08-final-public-audit.json','09-complete.json')
MAX_ACTUAL_SECONDS = 1080  # Original900-second scenario plus180 mandatory cleanup.



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
             'README.md', 'PLAN.md', 'provenance.json', 'fixture-public.json',
             'fixture-catalog.json', 'go-source-manifest.json', 'source-manifest.json')}
    files.update((PROFILE_HELPER, SETUP_REFERENCE, BASE_REFERENCE, R / 'build-profile.json',
                  R / 'scripts/candidate-go.sh', R / 'desktop/go.work',
                  R / 'desktop/go.mod', WR / 'go.mod', R / 'go.mod'))
    # These independently selected product files close the source-only runner
    # against the actual operator slice installed for this execution.
    for name in ('command.go', 'construction.go', 'runtime.go', 'local_actions.go',
                 'maintenance.go', 'setup.go', 'filesystem_unix.go',
                 'private_file_unix.go', 'private_acl_darwin.go'):
        files.add(R / 'desktop/pooloperator' / name)
    for name in ('remote_pool_controller.go', 'remote_pool_files.go', 'remote_pool_session.go'):
        files.add(R / 'desktop/accountpilot' / name)
    for p in (R / 'desktop/pilot').glob('remote_pool*.go'):
        files.add(p)
    files.update(p for p in REFERENCE_DIR.rglob('*') if p.is_file())
    files.update((R/'desktop/cmd/slugpilot').glob('*.go'))
    files.add(R/'desktop/go.sum')
    if (R/'desktop/go.work.sum').is_file(): files.add(R/'desktop/go.work.sum')
    files.add(HERE/'runner-manifest.json')
    files.add(W/'artifacts/set79/attachment-reference-attempt1/report.json')
    files.add(W/'artifacts/set79/attachment-reference-attempt1/python39.json')
    for name in ('daily_setup.go','capacity_drain.go','daily_provision.go'):
        p=R/'desktop/pooloperator'/name
        if p.is_file(): files.add(p)
    return files


def approved(path, sha):
    need(support.identity(path)['sha256'] == sha, 'approval bytes changed')
    v = read_json(path, 1 << 20)
    need(v.get('version') == 1 and v.get('scope') == SCOPE and
         v.get('approved_by') == 'root' and v.get('execution_authorized') is True,
         'exact root execution admission required')
    files = v.get('files')
    need(isinstance(files, dict) and 20 <= len(files) <= 200, 'finite input lock required')
    need({str(p) for p in required_inputs()} <= set(files), 'required input omitted')
    for name, value in files.items():
        need(Path(name).is_absolute() and support.identity(name) == value, 'locked input changed')
    need(support.identity(SETUP_REFERENCE) == dict(bytes=46498, sha256=SETUP_SHA),
         'independent setup bytes changed')
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


def fixture_tree(root):
    files, directories = {}, {}
    need(root.is_dir() and not root.is_symlink(), 'original fixture root missing')
    for p in sorted(root.rglob('*')):
        need(not p.is_symlink(), 'fixture symlink refused')
        rel = str(p.relative_to(root))
        if p.is_dir():
            directories[rel] = p.stat().st_mode & 0o777
        else:
            files[rel] = dict(**support.identity(p), mode=p.stat().st_mode & 0o777)
        need(len(files) + len(directories) <= 2000, 'fixture inventory cap')
    need(sum(v['bytes'] for v in files.values()) <= 64 << 20, 'fixture byte cap')
    return files, directories


def fixture_original(catalog):
    root = Path(catalog['root'])
    need(str(root) == str(W / 'artifacts/set73/owned-gym-baseline-attempt6/owned/owned-gym'), 'original artifact selection')
    st = root.lstat()
    need(dict(device=st.st_dev,inode=st.st_ino,uid=st.st_uid,mode=st.st_mode&0o777)==catalog['root_identity'], 'original root identity changed')
    files, dirs = fixture_tree(root)
    need(files == catalog['files'] and dirs == catalog['directories'], 'closed original fixture changed')
    report = catalog['report']
    need(support.identity(report['path']) == dict(bytes=report['bytes'], sha256=report['sha256']), 'baseline report changed')
    v = read_json(report['path'], 1 << 20)
    need(v['passed'] is True and v['source_unchanged'] is True and v['clock_offset_used'] is False and
         v['normal_controller_cleanup_verified'] is True and all(c['exit_code']==0 and not c['timed_out'] and c['log_joined'] and not c['log_limit_exceeded'] and c['cleanup']['group_empty'] and not c['cleanup']['leftover_after_exit'] and not c['cleanup']['signals'] for c in v['commands']), 'original clean baseline proof')
    return root


def copy_closed_tree(source, target, catalog_files, prefix=''):
    target.mkdir(mode=0o700)
    # Fresh byte copies only. Never links, xattr/custody inheritance, or symlink traversal.
    for rel, row in sorted(catalog_files.items()):
        if prefix and not rel.startswith(prefix + '/'):
            continue
        local = rel[len(prefix)+1:] if prefix else rel
        p, q = source / rel, target / local
        q.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        need(not q.exists(), 'copy destination already exists')
        shutil.copyfile(p, q, follow_symlinks=False)
        need((p.stat().st_dev,p.stat().st_ino)!=(q.stat().st_dev,q.stat().st_ino), 'hardlinked fixture refused')
        os.chmod(q, row['mode'])
        need(support.identity(q) == dict(bytes=row['bytes'],sha256=row['sha256']), 'fixture byte copy changed')


def prepare_fixture(artifact, catalog):
    source = fixture_original(catalog)
    owned = artifact / 'owned'
    owned.mkdir(mode=0o700)
    copy_closed_tree(source, owned / 'owned-gym', catalog['files'])
    # Preserve even empty original directories for exact established-root inventory.
    for rel, mode in sorted(catalog['directories'].items()):
        q = owned / 'owned-gym' / rel
        q.mkdir(mode=mode, parents=True, exist_ok=True)
        os.chmod(q, mode)
    for target, original in catalog['node_copies'].items():
        q = owned / target
        q.parent.mkdir(mode=0o700)
        copy_closed_tree(source, q, catalog['files'], original)
    files, dirs = fixture_tree(owned / 'owned-gym')
    need(files == catalog['files'] and dirs == catalog['directories'], 'full copied original differs')
    support.write(artifact / 'fixture-copied.json', dict(source=str(source), files=files,
        directories=dirs, node_copies=catalog['node_copies'], hardlinks=False))
    return owned


def fixture_after(artifact, catalog):
    fixture_original(catalog) # compare all original bytes, on success or failure
    root = artifact / 'owned/owned-gym'
    if not root.exists():
        return dict(original_unchanged=True, copied=False)
    files, directories = fixture_tree(root)
    immutable = catalog['immutable_copy_paths']
    need(all(files.get(p) == catalog['files'][p] for p in immutable), 'old copied history/control changed')
    def allowed(p):
        return p in catalog['permitted_mutable_exact'] or any(p.startswith(x) for x in catalog['permitted_mutable_prefixes'])
    changes = sorted(p for p in set(files) | set(catalog['files']) if files.get(p) != catalog['files'].get(p))
    need(all(allowed(p) for p in changes), 'unlisted original namespace change')
    need(directories == catalog['directories'], 'original directory inventory changed')
    result = dict(original_unchanged=True,copied=True,immutable_paths=len(immutable),
                  old_histories_unchanged=True, allowed_changed_paths=changes)
    support.write(artifact / 'fixture-after.json', result)
    return result


def cli_evidence(owned, report, cli, first_status, probe_pid):
    need(report.get('cli_cleanup') == cli and type(probe_pid) is int and probe_pid > 0,
         'original CLI cleanup projection')
    need(type(cli.get('pid')) is int and cli['pid'] > 0 and cli['pid'] != probe_pid and
         cli.get('parent_pid') == probe_pid and cli.get('pgid') == probe_pid and
         cli.get('joined') is True and cli.get('exit_code') == 0 and
         cli.get('signal') == 'SIGTERM' and cli.get('signal_count') == 1 and
         cli.get('forced_kill') is False and cli.get('unexpected_exit') is False and
         cli.get('wait_error') is False and cli.get('stdout_joined') is True and
         cli.get('stderr_joined') is True and cli.get('io_error') is False and
         type(cli.get('status_count')) is int and 1 <= cli['status_count'] <= 1024,
         'original CLI process did not close cleanly')
    node_pids = [n.get('pid') for n in report['node_cleanup']]
    need(all(type(n) is int and n > 0 for n in node_pids) and
         len(set([probe_pid, cli['pid'], *node_pids])) == 4,
         'one CLI and two original node identities')
    config_id = cli.get('config_id')
    need(isinstance(config_id, str) and re.fullmatch(r'[0-9a-f]{64}', config_id),
         'CLI selected config identity')
    reopened = read_json(owned/'05-stopped-reopen.json', 1 << 20)
    need(reopened.get('config_id') == config_id, 'CLI/stopped owner config changed')
    evidence = {}
    path = owned/'cli-stdout.jsonl'
    identity = support.identity(path)
    need(0 < identity['bytes'] <= 1 << 20, 'CLI stdout byte cap')
    with path.open('rb') as stream:
        raw = stream.read((1 << 20) + 1)
    need(len(raw) == identity['bytes'] and support.identity(path) == identity,
         'CLI stdout changed during read')
    lines = raw.splitlines(keepends=True)
    need(len(lines) == cli['status_count'], 'CLI status inventory')
    fields = ('version','observed_at_ms','config_id','state','worker_state','code','records','pending')
    states = ('ready','stopped','failed','paused','preparing','retrying','hashing','delivering','unavailable')
    codes = ('','unavailable','outcome_unknown','busy','capacity','rate_limited')
    values = []
    for line in lines:
        need(0 < len(line) < 4096 and line.endswith(b'\n') and b'\r' not in line,
             'CLI canonical status frame')
        value = json.loads(line)
        need(isinstance(value, dict) and tuple(value) == fields and
             json.dumps(value, ensure_ascii=False, separators=(',', ':')).encode() + b'\n' == line,
             'CLI exact status grammar')
        need(type(value['version']) is int and value['version'] == 2 and
             type(value['observed_at_ms']) is int and 0 < value['observed_at_ms'] <= 253402300799999 and
             value['config_id'] == config_id and value['state'] == 'signed_in' and
             value['worker_state'] in states and value['code'] in codes and
             type(value['records']) is int and 0 <= value['records'] <= 64 and
             type(value['pending']) is int and 0 <= value['pending'] <= value['records'],
             'CLI original bound status')
        if values:
            need(values[-1]['records'] <= value['records'], 'CLI retained record regression')
        values.append(value)
    need(first_status in values and values[-1] == cli.get('last_status'),
         'CLI first observed and final status correlation')
    evidence[str(path)] = identity
    stderr = owned/'cli-stderr.log'
    identity = support.identity(stderr)
    need(identity['bytes'] == 0, 'unexpected CLI diagnostics')
    evidence[str(stderr)] = identity
    return evidence


def public_result(owned, probe_pid):
    v=read_json(owned/'actual-report.json',2<<20)
    n=v.get('accepted_rewards')
    need(v.get('version')==1 and v.get('passed') is True and v.get('completed') is True and
         v.get('clean_join') is True and v.get('actual_cli') is True and v.get('public_seed_mining') is False and
         v.get('initial_height')==2 and type(v.get('final_height')) is int and
         103 <= v['final_height'] <=105 and type(n) is int and n in (1,2) and
         v.get('cleanup_errors')==[] and all(v.get(k) is True for k in
         ('stopped_reopen','original_wallet_received','scheduled_payment')),
         'incomplete original wallet DAILY payment')
    nodes=v.get('node_cleanup')
    need(isinstance(nodes,list) and len(nodes)==2 and all(
         x.get('clean') is True and x.get('joined') is True and x.get('forced_kill') is False and
         x.get('unexpected_exit') is False and x.get('wait_error')=='' and
         x.get('log_close_error') is False and x.get('stop_rpc_error') is False for x in nodes),
         'original node cleanup')
    evidence={}
    for name in (*PHASES,'outbox-after.json','selected-public-setup.json','actual-report.json','07-confirmed-payment.json','money-read-evidence.json'):
        p=owned/name;read_json(p,4<<20);evidence[str(p)]=support.identity(p)
    ref=read_json(SETUP_REFERENCE,262144)
    expected=next(x for x in ref['economics'] if x['accepted_rewards']==n)
    rows=read_json(owned/'04-closed-rows.json',4<<20)
    cli=read_json(owned/'03-cli-joined.json',1<<20)
    first_status=read_json(owned/'02-cli-running.json',65536)
    reopened=read_json(owned/'05-stopped-reopen.json',1<<20)
    done=read_json(owned/'09-complete.json',4<<20)
    outbox=read_json(owned/'outbox-after.json',65536)
    need(rows['outbox']==outbox and type(outbox.get('issue_pending')) is bool, 'original hold/metadata/scan/cursor changed')
    paid=read_json(owned/'07-payment-and-wallet.json',4<<20)
    audit=read_json(owned/'08-final-public-audit.json',4<<20)
    need(len(rows['accepted'])==n and rows['authorizations']>0 and
         1<=len(rows['rows'])<=64 and reopened['remote']['attempts']==0 and
         reopened['remote']['records']==len(rows['rows']) and
         reopened['remote']['pending']==rows['outbox']['pending'], 'original outbox/reopen')
    need(done.get('completed') is True and done.get('stopped_reopen') is True and
         done.get('final_audit_unchanged') is True and done.get('send_count')==1 and
         done['paid_atoms']==expected['miner_atoms'] and done['fee_atoms']==expected['fee_atoms'] and
         v['miner_atoms']==expected['miner_atoms'] and v['fee_atoms']==expected['fee_atoms'], 'exact independent economic branch')
    payment=paid['payment']
    confirmed=read_json(owned/'07-confirmed-payment.json',4<<20)
    reads=read_json(owned/'money-read-evidence.json',65536)
    need(confirmed['payment']==payment and confirmed['confirmation_block']==paid['confirmation_block'] and
         type(confirmed['confirmed_at_ms']) is int and confirmed['confirmed_at_ms']>0,
         'original validated payment evidence')
    count=reads.get('refusal_count')
    need(reads.get('complete') is True and type(count) is int and 0<=count<=2400 and
         isinstance(reads.get('refusals'),list) and len(reads['refusals'])==min(count,32) and
         (reads.get('last_refusal') is None)==(count==0), 'complete bounded money read evidence')
    for refusal in reads['refusals']+([reads['last_refusal']] if count else []):
        need(refusal['read'] in ('balance','payouts') and type(refusal['gym_index']) is int and
             0<=refusal['gym_index']<2 and refusal['code'] in ('busy','rate_limited','unavailable') and
             refusal['outcome_unknown'] is False, 'only definite pure read retries')
    need(payment['miner_atoms']==expected['miner_atoms'] and payment['change_atoms']==expected['change_atoms'] and
         payment['fee_atoms']==expected['fee_atoms'] and payment['input_count']==n and
         paid['wallet_after']['remote']['spendable_atoms']-paid['wallet_before']['remote']['spendable_atoms']==expected['miner_atoms'], 'actual original wallet exact payment')
    need(audit['header']['credits']==n and audit['header']['batches']==1 and audit['header']['pending_batch']==0 and
         audit['delivery']=='confirmed' and audit['body']['raw']==payment['raw'] and audit['body']['txid']==payment['txid'] and
         audit['accepted']==n and audit['read_only_unchanged'] is True, 'closed original daily ledger')
    need(len(done['attempts'])>=1 and len(done['attempts'])<=16 and
         all(x['raw']==payment['raw'] for x in done['attempts']), 'no replacement broadcast bytes')
    evidence.update(cli_evidence(owned, v, cli, first_status, probe_pid))
    return evidence


def run(args):
    lock_path = Path(args.approved_lock)
    lock = approved(lock_path, args.approved_lock_sha256)
    artifact = W / 'artifacts/set79' / args.attempt
    artifact.mkdir(mode=0o700)  # never erase or reuse any attempt, including failure.
    report = dict(version=1, passed=False, scope=SCOPE, actual_started=False, commands=[],
                  fixture_timeout_seconds=900, outer_timeout_seconds=MAX_ACTUAL_SECONDS, max_chain_height=180)
    before, mapping, built, bound, bp = None, None, {}, None, None
    catalog = None
    source_files = set()
    try:
        catalog = read_json(HERE / 'fixture-catalog.json', 1 << 20)
        fixture_original(catalog)
        support.write(artifact / 'execution-lock.json', lock)
        spec = importlib.util.spec_from_file_location('actual_profile', PROFILE_HELPER)
        bp = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(bp)
        bound = bp.binding()
        support.write(artifact / 'profile.json', bound)
        probe, work, mapping = copy_probe(artifact)
        env = bp.environment(GOCACHE='/private/tmp/slug-go-build',
                             GOMODCACHE='/Users/taytrrip/slug/build-cache/go-mod', GOPROXY='off',
                             GOSUMDB='off', GOWORK=str(work), GOMAXPROCS='2', CGO_ENABLED='1')
        for key in list(env):
            if key.startswith('SLUG_'):
                del env[key]
        env.update(PYTHONDONTWRITEBYTECODE='1', GOOS='darwin', GOARCH='arm64')
        node_env = dict(env, CGO_ENABLED='0')
        source_files = {Path(p) for p in lock['files']} | {lock_path, work} | set(map(Path, mapping))
        # Three exact closures bind node, real CLI and probe, including native cgo leaves.
        for name, cwd, query_env, target in [('node', R, node_env, '.'),
             ('helper', R/'desktop', env, './cmd/slugpilot'), ('probe', probe, env, '.')]:
            path = artifact / (name + '-dependencies.json')
            row = capture(['go', 'list', '-deps', '-json', target], cwd, query_env, path, 120, log_cap=32 << 20)
            report['commands'].append(row)
            need(okay(row), 'dependency query failed')
            source_files.update(dependency_files(path))
            if name in ('probe', 'helper'):
                need('github.com/taytr-llc/slugwallet/walletdb/bdb' in path.read_text(), 'real wallet driver absent')
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
        for name, filename, cwd, build_env, cgo, package in [('node', 'slugd', R, node_env, 0, '.'),
             ('helper', 'slugpilot', R/'desktop', env, 1, './cmd/slugpilot'),
             ('probe', 'daily-headless-actual', probe, env, 1, '.')]:
            target = binary_dir / filename
            row = capture(bp.command('build', '-p=1', '-mod=readonly', '-trimpath', '-tags=', '-o', target, package),
                          cwd, build_env, artifact / (name + '-build.log'), 240)
            report['commands'].append(row)
            need(okay(row), 'normal binary build failed')
            built[name] = bp.artifact(target, artifact / (name + '-build-info.txt'), tags='', cgo=cgo)
        report['artifacts'] = built
        need(all(support.identity(p) == value for p, value in before.items()), 'source changed before actual')
        bp.same(bound)
        bp.unchanged(built)
        approved(lock_path, args.approved_lock_sha256)
        owned = prepare_fixture(artifact, catalog)
        argv = [built['probe']['path'], '--node', built['node']['path'],
                '--helper', built['helper']['path'],
                '--artifacts', str(owned), '--reference', str(SETUP_REFERENCE),
                '--base-reference', str(BASE_REFERENCE)]
        report['actual_started'] = True
        row = capture(argv, probe, env, artifact / 'actual.log', MAX_ACTUAL_SECONDS,
                      permitted=[x['path'] for x in built.values()])
        report['commands'].append(row)
        need(okay(row), 'actual command or cleanup failed')
        report['public_evidence'] = public_result(artifact / 'owned', row['pid'])
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
        try:
            need(catalog is not None, 'fixture catalog unavailable')
            report['fixture_preservation'] = fixture_after(artifact, catalog)
            if report.get('actual_started') or (artifact/'fixture-copied.json').exists():
                need(report['fixture_preservation'].get('copied') is True and report['fixture_preservation'].get('old_histories_unchanged') is True, 'prepared original copy missing')
        except BaseException as exc:
            report['passed'] = False
            report['fixture_preservation_error'] = error('fixture_finalization', exc)
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
    if (not a.execute or not re.fullmatch(r'public-daily-headless-actual-attempt[1-9][0-9]*', a.attempt) or
        not re.fullmatch(r'[0-9a-f]{64}', a.approved_lock_sha256) or not Path(a.approved_lock).is_absolute()):
        parser.error('explicit execute, exact root lock and fresh finite attempt required')
    try:
        sys.exit(run(a))
    except BaseException as exc:
        if isinstance(exc, SystemExit):
            raise
        print(json.dumps(dict(passed=False, error=error('admission_or_report', exc))))
        sys.exit(1)
