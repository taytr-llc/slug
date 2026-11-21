#!/usr/bin/env python3
"""One root-selected hosted proof phase; no automatic retries or new clock."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import signal
import stat
import subprocess
import time

A = Path('/Users/taytrrip/slug/artifacts/set82/hosted-actual-attempt4')
BINARY = Path('/Users/taytrrip/slug/artifacts/set82/proofclient-native-attempt2/slug-host-proof')
REFERENCE = Path('/Users/taytrrip/slug/set81-prep/retained-admission-actual/inputs/reference-v1.json')
REFERENCE_SHA = '17557b28921290461fdf47b9ed6f1b800d3d077ee5d325a4ec59277ab3faca6a'
LIMITS = {'stdout': (16 << 20) + 1, 'stderr': 65536}


def need(ok, message):
    if not ok:
        raise ValueError(message)


def pairs(items):
    out = {}
    for key, value in items:
        need(key not in out, 'duplicate JSON field')
        out[key] = value
    return out


def no_number(_):
    raise ValueError('noninteger JSON value')


def parse(raw):
    value = json.loads(raw.decode('utf-8'), object_pairs_hook=pairs,
                       parse_float=no_number, parse_constant=no_number)
    need(type(value) is dict, 'one complete JSON object required')
    return value


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def is_id(value):
    return isinstance(value, str) and len(value) == 64 and all(c in '0123456789abcdef' for c in value)


def read(path, cap, protected=False):
    need(path.is_absolute() and path == path.resolve(strict=True), 'canonical original file')
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        before = os.fstat(fd)
        need(stat.S_ISREG(before.st_mode) and before.st_nlink == 1 and 0 < before.st_size <= cap,
             'bounded original regular file')
        if protected:
            parent = path.parent.stat()
            need(before.st_uid == parent.st_uid == os.geteuid() and
                 stat.S_IMODE(before.st_mode) in (0o400, 0o600) and stat.S_IMODE(parent.st_mode) == 0o700,
                 'protected original input')
        chunks = []
        remaining = before.st_size
        while remaining:
            chunk = os.read(fd, min(65536, remaining))
            need(bool(chunk), 'complete original input')
            chunks.append(chunk)
            remaining -= len(chunk)
        after = os.fstat(fd)
        fields = lambda s: (s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_nlink, s.st_size, s.st_mtime_ns, s.st_ctime_ns)
        need(fields(before) == fields(after) == fields(path.lstat()), 'original input changed')
        raw = b''.join(chunks)
        return raw, (fields(before), digest(raw))
    finally:
        os.close(fd)


def fresh(path):
    need(path.parent == path.parent.resolve(strict=True), 'canonical evidence parent')
    return os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=('initial-money', 'replay-before', 'replay-after', 'confirmed-money', 'chain'))
    p.add_argument('--txid')
    p.add_argument('--after-ms', type=int)
    p.add_argument('--audit-file', type=Path)
    args = p.parse_args()
    need((args.txid is not None) == (args.action in ('confirmed-money', 'chain')), 'txid only for confirmed money or chain')
    need((args.after_ms is not None) == (args.action == 'confirmed-money'), 'after-ms only for confirmed money')
    need((args.audit_file is not None) == (args.action == 'chain'), 'audit-file only for chain')
    if args.txid is not None:
        need(is_id(args.txid), 'original transaction ID')
    os.umask(0o077)
    need(A == A.resolve(strict=True), 'original attempt root')
    root = A.stat()
    need(stat.S_ISDIR(root.st_mode) and root.st_uid == os.geteuid() and stat.S_IMODE(root.st_mode) == 0o700,
         'owned0700 original attempt root')
    prefix = A / ('proof-' + args.action)
    # Claim all evidence exclusively before any child effect. Partial claims on
    # failure remain present; a second invocation cannot erase the first attempt.
    status_fd = fresh(Path(str(prefix) + '.status.json'))
    outputs = {}
    child = None
    selected_inputs = {}
    report = dict(version=1, action=args.action, passed=False, parent_pid=os.getpid(), child_pid=0,
                  joined=False, exit_code=None, signal_count=0, forced_kill=False, interrupted=False,
                  timed_out=False, inputs_unchanged=False, error='')
    requested = [False]
    def request_stop(_sig, _frame):
        requested[0] = True
    old_handlers = {sig: signal.signal(sig, request_stop) for sig in (signal.SIGTERM, signal.SIGINT)}
    end = None
    try:
        for name in ('stdout', 'stderr'):
            outputs[name] = fresh(Path(str(prefix) + ('.json' if name == 'stdout' else '.stderr')))
        def selected(path, cap, protected=False):
            raw, identity = read(path, cap, protected)
            selected_inputs[path] = (cap, protected, identity)
            return raw
        selection = parse(selected(A / 'attempt-selection.json', 65536))
        started = time.time_ns() // 1000000
        deadline = selection['deadline_ms']
        need(type(selection['start_ms']) is int and type(deadline) is int and
             deadline == selection['start_ms'] + 900000 and selection['start_ms'] <= started < deadline,
             'original fixed scenario deadline')
        phase_end_ms = min(deadline, started + 65000)
        end = time.monotonic() + (phase_end_ms - started) / 1000
        need(end - time.monotonic() > 5, 'insufficient original time for joined phase')
        report.update(started_at_ms=started, scenario_deadline_ms=deadline, phase_deadline_ms=phase_end_ms)
        checked = parse(selected(A / 'selection-check.json', 65536))
        need(checked.get('passed') is True and checked.get('source_reference_sha256') == REFERENCE_SHA and
             checked.get('original_initial_start_ms') == selection['start_ms'] and checked.get('due_at_ms') == selection['due_ms'],
             'same independently checked public selection')
        need(is_id(checked.get('setup_id')) and is_id(checked.get('remote_id')), 'checked public IDs')
        ref = selected(REFERENCE, 174293)
        need(len(ref) == 174293 and digest(ref) == REFERENCE_SHA, 'original qualified reference')
        setup = selected(A / 'public/setup.json', 1 << 20)
        remote_public = selected(A / 'public/remote.json', 1 << 20)
        for name, raw in [('setup.json', setup), ('remote.json', remote_public)]:
            rows = [x for x in checked['inputs'] if x['path'] == str(A / 'public' / name)]
            need(len(rows) == 1 and rows[0]['bytes'] == len(raw) and rows[0]['sha256'] == digest(raw),
                 'exact checked public bytes')
        private = A / 'client-inputs'
        need(selected(private / 'remote-config.json', 1 << 20, True) == remote_public, 'exact protected remote copy')
        execution = parse(selected(A / 'execution-selection.json', 1 << 20))
        binary = selected(BINARY, 128 << 20)
        rows = [x for x in execution['files'] if x['path'] == str(BINARY)]
        need(len(rows) == 1 and rows[0]['bytes'] == len(binary) and rows[0]['sha256'] == digest(binary),
             'root-frozen original native binary')
        report.update(binary=dict(path=str(BINARY), bytes=len(binary), sha256=digest(binary)),
                      setup_id=checked['setup_id'], remote_id=checked['remote_id'])
        command = 'replay' if args.action.startswith('replay-') else ('chain' if args.action == 'chain' else 'money')
        cmd = [str(BINARY), command, '--reference', str(REFERENCE), '--setup', str(A / 'public/setup.json'),
               '--remote', str(private / 'remote-config.json'), '--expected-setup-id', checked['setup_id'],
               '--expected-remote-id', checked['remote_id']]
        if command in ('money', 'replay'):
            selected(private / 'pool.credential', 2048, True)
            cmd += ['--pool-credential', str(private / 'pool.credential')]
        if command == 'money':
            selected(private / 'pool-token-1', 64, True)
            cmd += ['--other-pool-token', str(private / 'pool-token-1'), '--phase',
                    'initial' if args.action == 'initial-money' else 'confirmed']
            if args.action == 'confirmed-money':
                need(0 < args.after_ms <= started, 'original confirmation observation time')
                cmd += ['--txid', args.txid, '--after-ms', str(args.after_ms)]
        elif command == 'replay':
            cmd += ['--attachment-root', str(A / 'remote-attachment'), '--sequence', '1']
        else:
            need(args.audit_file in (A / 'audit-before.json', A / 'audit-after.json'), 'named original closed host audit')
            audit = selected(args.audit_file, 16 << 20)
            need(parse(audit).get('passed') is True, 'successful root-captured closed audit')
            selected(private / 'remote-rpc.json', 65536, True)
            cmd += ['--node-rpc', str(private / 'remote-rpc.json'), '--audit-report', str(args.audit_file),
                    '--audit-sha256', digest(audit), '--txid', args.txid]
            report['audit'] = dict(path=str(args.audit_file), bytes=len(audit), sha256=digest(audit))
        # Reserve the final five seconds of this SAME bound for TERM/KILL/reap.
        # No child restart, time-budget renewal or additional network retry.
        work_end = end - 5
        need(not requested[0] and time.monotonic() < work_end and time.time_ns() // 1000000 < deadline,
             'original phase still admitted')
        child = subprocess.Popen(cmd, stdin=subprocess.DEVNULL, stdout=outputs['stdout'], stderr=outputs['stderr'],
                                 close_fds=True, start_new_session=False)
        report.update(child_pid=child.pid, pgid=os.getpgid(child.pid))
        term_sent = False
        while child.poll() is None:
            now = time.monotonic()
            over = any(os.fstat(fd).st_size > LIMITS[name] for name, fd in outputs.items())
            if not term_sent and (requested[0] or over or now >= work_end or time.time_ns() // 1000000 >= deadline):
                report['interrupted'] = requested[0]
                report['timed_out'] = now >= work_end or time.time_ns() // 1000000 >= deadline
                report['output_overflow'] = over
                child.send_signal(signal.SIGTERM)
                report['signal_count'] += 1
                term_sent = True
            if now >= end - 1 and child.poll() is None:
                child.kill()
                report['forced_kill'] = True
                break
            time.sleep(min(0.05, max(0.001, end - now)))
        code = child.wait(timeout=max(0.001, end - time.monotonic()))
        report.update(joined=True, exit_code=code)
        need(not term_sent and not requested[0] and time.monotonic() < end and time.time_ns() // 1000000 < deadline,
             'phase interrupted or deadline reached')
        need(code == 0, 'original proof child failed')
        for name, fd in outputs.items():
            need(os.fstat(fd).st_size <= LIMITS[name], 'public output bound')
            os.fsync(fd)
        raw, _ = read(Path(str(prefix) + '.json'), LIMITS['stdout'])
        proof = parse(raw)
        need(os.fstat(outputs['stderr']).st_size == 0, 'proof stderr is not empty')
        need(proof.get('passed') is True and proof.get('clean_join') is True and proof.get('selected_inputs_unchanged') is True and
             proof.get('command') == command and proof.get('setup_id') == checked['setup_id'] and
             proof.get('remote_id') == checked['remote_id'] and proof.get('reference_sha256') == REFERENCE_SHA and
             proof.get('pid') == child.pid and proof.get('parent_pid') == os.getpid(), 'complete original proof result')
        for path, (cap, protected, identity) in selected_inputs.items():
            need(read(path, cap, protected)[1] == identity, 'selected input changed')
        need(not requested[0] and time.monotonic() < end and time.time_ns() // 1000000 < deadline,
             'original phase ended before success')
        report['inputs_unchanged'] = True
        report['passed'] = True
    except Exception as exc:
        # Local fixed messages only; never serialize arbitrary OS/input/secret text.
        report['error'] = str(exc) if type(exc) is ValueError else type(exc).__name__
    finally:
        if child is not None and child.poll() is None:
            try:
                child.kill()
                report['forced_kill'] = True
                code = child.wait(timeout=max(0.001, (end or time.monotonic()) - time.monotonic()))
                report.update(joined=True, exit_code=code)
            except Exception:
                report['cleanup_failed'] = True
            report['passed'] = False
        elif child is not None and not report['joined']:
            report.update(joined=True, exit_code=child.wait(timeout=0))
        for name, fd in outputs.items():
            try:
                os.fsync(fd)
                report[name + '_bytes'] = os.fstat(fd).st_size
                os.close(fd)
                path = Path(str(prefix) + ('.json' if name == 'stdout' else '.stderr'))
                # Empty failure outputs are retained and hashed without parsing.
                report[name + '_sha256'] = digest(path.read_bytes()) if path.stat().st_size <= LIMITS[name] else None
            except Exception:
                report['output_cleanup_failed'] = True
                report['passed'] = False
        for sig, handler in old_handlers.items():
            signal.signal(sig, handler)
        report['ended_at_ms'] = time.time_ns() // 1000000
        raw = (json.dumps(report, sort_keys=True, separators=(',', ':'), allow_nan=False) + '\n').encode()
        try:
            need(os.write(status_fd, raw) == len(raw), 'complete status write')
            os.fsync(status_fd)
        finally:
            os.close(status_fd)
    print(json.dumps(dict(action=args.action, passed=report['passed'], status=str(prefix) + '.status.json')))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
