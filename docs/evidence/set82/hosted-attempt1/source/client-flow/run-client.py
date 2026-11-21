#!/usr/bin/env python3
"""Own exactly one selected miner OR stopped observer; no restart or network API."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import selectors
import signal
import stat
import subprocess
import time


def need(ok, message):
    if not ok:
        raise ValueError(message)


def path(value, exists=True):
    p = Path(value)
    need(p.is_absolute() and str(p) == os.path.normpath(str(p)), 'canonical absolute path required')
    if exists:
        need(p == p.resolve(strict=True), 'original path required')
    else:
        need(p.parent == p.parent.resolve(strict=True) and not p.exists() and not p.is_symlink(), 'fresh output required')
    return p


def identity(p):
    st = p.stat()
    need(stat.S_ISREG(st.st_mode) and st.st_nlink == 1 and st.st_size <= 128 << 20, 'bounded original file')
    digest = hashlib.sha256()
    with p.open('rb') as f:
        while True:
            b = f.read(65536)
            if not b:
                break
            digest.update(b)
    after = p.stat()
    need((st.st_dev, st.st_ino, st.st_size, st.st_mtime_ns, st.st_ctime_ns) ==
         (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns), 'input changed')
    return dict(bytes=st.st_size, sha256=digest.hexdigest(), device=st.st_dev, inode=st.st_ino)


def file(p):
    return open(p, 'xb', buffering=0)


def write(f, data):
    need(f.write(data) == len(data), 'complete evidence write required')


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--mode', choices=('miner', 'observe'), required=True)
    for key in ('binary', 'config-id', 'owned-root', 'attachment-root', 'remote-config', 'node-rpc',
                'pool-credential', 'admission-credential', 'evidence-dir'):
        p.add_argument('--' + key, required=True)
    p.add_argument('--reference')
    p.add_argument('--deadline-ms', type=int, required=True, help='original root-selected scenario deadline, never reset')
    a = p.parse_args()
    os.umask(0o077)
    binary = path(a.binary)
    owned, attachment = path(a.owned_root), path(a.attachment_root, False) if not Path(a.attachment_root).exists() else path(a.attachment_root)
    out = path(a.evidence_dir, False)
    parent = out.parent.stat()
    need(parent.st_uid == os.geteuid() and stat.S_IMODE(parent.st_mode) == 0o700, 'owned0700 evidence parent')
    inputs = [binary] + [path(getattr(a, name)) for name in ('remote_config', 'node_rpc', 'pool_credential', 'admission_credential')]
    need(len(a.config_id) == 64 and all(c in '0123456789abcdef' for c in a.config_id), 'selected config ID')
    now = time.time_ns() // 1000000
    need(now < a.deadline_ms <= now + 900000, 'same original finite scenario deadline')
    deadline = time.monotonic() + (a.deadline_ms - now) / 1000
    if a.mode == 'observe':
        need(a.reference is not None, 'observer reference required')
        inputs.append(path(a.reference))
    else:
        need(a.reference is None, 'miner takes no separate reference')
    before = {str(v): identity(v) for v in inputs}
    # Root binds the concrete binary/source/profile; this also detects substitution
    # during the selected invocation. These private input hashes are NOT emitted.
    os.mkdir(out, 0o700)
    command = [str(binary)]
    if a.mode == 'miner':
        command += ['--daily-recoverable-issue', '--candidate-name-version=1', '--data-dir', str(owned),
                    '--remote-attachment-root', str(attachment), '--remote-config', a.remote_config,
                    '--remote-node-rpc', a.node_rpc, '--remote-pool-credential', a.pool_credential,
                    '--remote-admission-credential', a.admission_credential]
    else:
        command += ['observe', '--owned-root', str(owned), '--attachment-root', str(attachment),
                    '--reference', a.reference, '--remote-config', a.remote_config, '--node-rpc', a.node_rpc,
                    '--pool-credential', a.pool_credential, '--admission-credential', a.admission_credential,
                    '--snapshots', str(out / 'wallet-snapshots.jsonl'), '--report', str(out / 'wallet-report.json')]
    report = dict(version=1, mode=a.mode, parent_pid=os.getpid(), child_pid=0, pgid=0, deadline_at_ms=a.deadline_ms,
                  started_at_ms=now, ended_at_ms=0, config_id=a.config_id, binary=before[str(binary)],
                  requested_stop=False, signal_count=0, forced_kill=False, unexpected_exit=False,
                  joined=False, exit_code=None, stdout_bytes=0, stderr_bytes=0, status_count=0,
                  last_status=None, inputs_unchanged=False, cleanup_complete=False, passed=False, errors=[])
    requested = [False]
    def stop(_signum, _frame):
        requested[0] = True
    old = {sig: signal.signal(sig, stop) for sig in (signal.SIGTERM, signal.SIGINT)}
    child = None
    logs = {}
    sel = selectors.DefaultSelector()
    stop_at = kill_at = exited_at = None
    buffer = b''
    try:
        logs = {name: file(out / (name + '.log')) for name in ('stdout', 'stderr')}
        child = subprocess.Popen(command, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                 start_new_session=False, close_fds=True)
        report['child_pid'], report['pgid'] = child.pid, os.getpgid(child.pid)
        need(report['pgid'] == os.getpgrp() and child.pid != os.getpid(), 'original inherited process group')
        for name, pipe in (('stdout', child.stdout), ('stderr', child.stderr)):
            os.set_blocking(pipe.fileno(), False)
            sel.register(pipe, selectors.EVENT_READ, name)
        while True:
            current = time.monotonic()
            code = child.poll()
            if code is not None and exited_at is None:
                exited_at = current
            if code is not None and stop_at is None:
                report['unexpected_exit'] = True
                report['errors'].append('premature_exit')
            if current >= deadline and stop_at is None:
                report['errors'].append('scenario_deadline')
            if stop_at is None and code is None and (requested[0] or report['errors']):
                report['requested_stop'] = requested[0]
                child.send_signal(signal.SIGTERM)
                report['signal_count'] += 1
                stop_at = current
            if stop_at is not None and code is None and current - stop_at >= 30 and kill_at is None:
                child.kill()
                report['forced_kill'] = True
                report['errors'].append('stop_deadline')
                kill_at = current
            if kill_at is not None and code is None and current - kill_at >= 5:
                report['errors'].append('child_not_joined')
                break
            for key, _ in sel.select(0.1):
                data = os.read(key.fd, 65536)
                if not data:
                    sel.unregister(key.fileobj)
                    key.fileobj.close()
                    continue
                name = key.data
                cap = (1 << 20) if name == 'stdout' else 65536
                prior = report[name + '_bytes']
                report[name + '_bytes'] += len(data)
                if prior < cap:
                    write(logs[name], data[:cap - prior])
                if report[name + '_bytes'] > cap and 'output_limit' not in report['errors']:
                    report['errors'].append('output_limit')
                if name == 'stdout' and a.mode == 'miner' and not report['errors']:
                    buffer += data
                    while b'\n' in buffer:
                        line, buffer = buffer.split(b'\n', 1)
                        status = json.loads(line)
                        need(len(line) < 512 and list(status) == ['version', 'observed_at_ms', 'config_id', 'state', 'worker_state', 'code', 'records', 'pending'], 'closed original status frame')
                        need(json.dumps(status, separators=(',', ':')).encode('ascii') == line and status['version'] == 2 and status['config_id'] == a.config_id, 'original selected canonical status')
                        need(type(status['records']) is int and type(status['pending']) is int and 0 <= status['pending'] <= status['records'] <= 64, 'bounded retained counts')
                        report['status_count'] += 1
                        need(report['status_count'] <= 1024, 'status count bound')
                        report['last_status'] = status
                    need(len(buffer) <= 512, 'bounded unfinished status')
            if code is not None and not sel.get_map():
                break
            if exited_at is not None and current - exited_at > 5:
                report['errors'].append('output_not_joined')
                break
    except Exception:
        report['errors'].append('launcher_failure')
    finally:
        if child is not None:
            if child.poll() is None and stop_at is None:
                try:
                    child.send_signal(signal.SIGTERM)
                    report['signal_count'] += 1
                    stop_at = time.monotonic()
                except Exception:
                    report['errors'].append('signal_failure')
            try:
                remaining = max(0, 30 - (time.monotonic() - stop_at)) if stop_at is not None else 0
                report['exit_code'] = child.wait(timeout=remaining)
                report['joined'] = True
            except subprocess.TimeoutExpired:
                report['forced_kill'] = True
                try:
                    if kill_at is None:
                        child.kill()
                        kill_at = time.monotonic()
                    report['exit_code'] = child.wait(timeout=max(0, kill_at + 5 - time.monotonic()))
                    report['joined'] = True
                except Exception:
                    report['errors'].append('cleanup_join_failure')
            except Exception:
                report['errors'].append('wait_failure')
            for stream in (child.stdout, child.stderr):
                if stream is not None and not stream.closed:
                    report['errors'].append('output_not_drained')
                    stream.close()
        sel.close()
        for log in logs.values():
            try:
                os.fsync(log.fileno())
                log.close()
            except Exception:
                report['errors'].append('output_close_failure')
        try:
            report['inputs_unchanged'] = all(identity(v) == before[str(v)] for v in inputs)
        except Exception:
            report['errors'].append('input_recheck_failure')
        report['ended_at_ms'] = time.time_ns() // 1000000
        report['cleanup_complete'] = report['joined'] and not report['forced_kill'] and not report['errors']
        report['passed'] = bool(report['cleanup_complete'] and report['requested_stop'] and report['signal_count'] == 1 and report['exit_code'] == 0 and report['stderr_bytes'] == 0 and report['inputs_unchanged'] and not buffer and (a.mode != 'miner' or report['status_count'] > 0))
        # For observe mode this is process evidence only. Independently require
        # the observer report, ready snapshots and exact wallet delta afterward.
        with file(out / 'process-report.json') as output:
            write(output, (json.dumps(report, indent=2) + '\n').encode('ascii'))
            os.fsync(output.fileno())
        for sig, handler in old.items():
            signal.signal(sig, handler)
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
