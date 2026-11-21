#!/usr/bin/env python3
"""One new bounded native-read execution over the original paid Set82 roots."""
import hashlib
import json
import os
from pathlib import Path
import shlex
import signal
import subprocess
import time

W = Path('/Users/taytrrip/slug')
A = W / 'artifacts/set83/native-actual-attempt1'
OLD = W / 'artifacts/set82/hosted-actual-attempt5'
BIN = W / 'artifacts/set83/native-acceptance-build/native-payout-acceptance'
BIN_SHA = '18a3ed34493d6598a6871462ee51a1dff18557c9e94349b6f97fbd3fe3114ba6'
PROOF = W / 'artifacts/set82/proofclient-native-attempt5/slug-host-proof'
PROOF_SHA = '6ae3e5a8e417ba68a253e29b03ab1b4f815f86c9ed8230af17b66b6f22a80f66'
PRIOR_REPLAY_SHA = '65f48981afe3f86bd5a3e3ca00c7bb7a81b117ba1b9388d6140908ad21ac8095'
SELF = '/usr/local/bin/host-native-payout-lifecycle-83'
SETUP_SHA = '87db5803e70418affbf492b2716e2104d6902ec9294940141fe97a885a5c3525'
OPERATOR_SHA = '86a3bfa7cb07951e83eaca6df346a4be8bebe72df30d779705c83f7f8ac3a85f'
HOST = '/data/hosted-test-5/set83-native-lifecycle/01'


def need(value, message):
    if not value:
        raise RuntimeError(message)


def write(name, value):
    with (A / name).open('x') as f:
        json.dump(value, f, indent=2)
        f.write('\n')
        f.flush()
        os.fsync(f.fileno())


def main():
    os.umask(0o077)
    need(A == A.resolve(strict=True) and A.is_dir(), 'selected existing evidence root')
    need((A.stat().st_mode & 0o777) == 0o700 and A.stat().st_uid == os.geteuid(), 'evidence root must be owned0700')
    names = ['native-execution-selection.json', 'native-orchestration-report.json', 'native-report.json']
    names += [name + suffix for name in ('operator-run', 'operator-listening', 'native-helper', 'outbox-replay', 'operator-stop')
              for suffix in ('.stdout', '.stderr')]
    need(all(not (A / name).exists() and not (A / name).is_symlink() for name in names), 'fresh named evidence required')
    need(BIN == BIN.resolve(strict=True) and hashlib.sha256(BIN.read_bytes()).hexdigest() == BIN_SHA, 'selected native binary')
    need(PROOF == PROOF.resolve(strict=True) and hashlib.sha256(PROOF.read_bytes()).hexdigest() == PROOF_SHA, 'original qualified proof binary')
    old_execution_raw = (OLD / 'execution-selection.json').read_bytes()
    old_execution = json.loads(old_execution_raw)
    selected_binary = [v for v in old_execution['files'] if v['path'] == str(PROOF)]
    need(len(selected_binary) == 1 and selected_binary[0]['sha256'] == PROOF_SHA and
         selected_binary[0]['bytes'] == PROOF.stat().st_size, 'old original binary selection')
    prior_raw = (OLD / 'proof-replay-after.json').read_bytes()
    need(hashlib.sha256(prior_raw).hexdigest() == PRIOR_REPLAY_SHA, 'qualified original outbox projection')
    prior_replay = json.loads(prior_raw)
    now = time.time_ns() // 1000000
    # Round down once for the shell seconds interface; never renew this deadline.
    deadline_ms = ((now + 300000) // 1000) * 1000
    need(deadline_ms < 1795287660000, 'original credentials must outlast full execution')
    need(time.strftime('%Y-%m-%d', time.gmtime(now / 1000)) == '2026-11-21', 'original completed day')
    deadline = time.monotonic() + (deadline_ms - now) / 1000
    report = dict(version=1, scope='Native read/owner joins only; root financial and infrastructure audits remain required',
                  started_at_ms=now, deadline_at_ms=deadline_ms, ended_at_ms=0,
                  native_binary_sha256=BIN_SHA, processes={}, stop_result=None,
                  native_report_passed=False, outbox_replay_unchanged=False, inputs_unchanged=False, errors=[], passed=False)
    write('native-execution-selection.json', dict(version=1, started_at_ms=now, deadline_at_ms=deadline_ms,
          setup_sha256=SETUP_SHA, operator_sha256=OPERATOR_SHA, native_binary_sha256=BIN_SHA))
    env = os.environ.copy()
    env.pop('RAILWAY_TOKEN', None)
    env.pop('RAILWAY_API_TOKEN', None)
    interrupted = [False]
    previous = {s: signal.signal(s, lambda _s, _f: interrupted.__setitem__(0, True))
                for s in (signal.SIGTERM, signal.SIGINT)}
    owners = {}

    def ssh(script):
        remote = shlex.join(['/bin/su', '-s', '/bin/sh', '-c', script, 'slug'])
        return ['railway', 'ssh', '--project', '62fdb666-7f67-4cf3-98f0-6177f099b580',
                '--environment', '3fd7da79-0f2f-4077-bf29-e7035d62ac57',
                '--service', 'dedabe18-acd1-4dfe-a775-8bd02f799717', '--', remote]

    def start(name, command):
        out = (A / (name + '.stdout')).open('xb', buffering=0)
        err = (A / (name + '.stderr')).open('xb', buffering=0)
        try:
            child = subprocess.Popen(command, stdin=subprocess.DEVNULL, stdout=out, stderr=err,
                                     env=env, start_new_session=True, close_fds=True)
        except BaseException:
            out.close()
            err.close()
            raise
        owners[name] = (child, out, err)
        report['processes'][name] = dict(pid=child.pid, pgid=child.pid, started_at_ms=time.time_ns() // 1000000,
                                      joined=False, exit_code=None, forced=False, signals=[])
        return child

    def finish(name, until, honor_signal=True, early_owner=None):
        child, out, err = owners[name]
        item = report['processes'][name]
        term_at = kill_at = None
        try:
            while child.poll() is None:
                current = time.monotonic()
                failed = (current >= until or (honor_signal and interrupted[0]) or
                          out.tell() > 1048576 or err.tell() > 1048576 or
                          (early_owner is not None and early_owner.poll() is not None))
                if failed and term_at is None:
                    report['errors'].append(name + ': interrupted, bounded wait, output cap, or early operator exit')
                    try:
                        os.killpg(child.pid, signal.SIGTERM)
                    except ProcessLookupError:
                        pass
                    item['signals'].append('TERM')
                    term_at = current
                if term_at is not None and current >= term_at + 30 and kill_at is None:
                    try:
                        os.killpg(child.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                    item['signals'].append('KILL')
                    item['forced'] = True
                    kill_at = current
                if kill_at is not None and current >= kill_at + 5:
                    raise RuntimeError(name + ': original process did not join')
                time.sleep(0.05)
            item['exit_code'] = child.wait()
            item['joined'] = True
            item['finished_at_ms'] = time.time_ns() // 1000000
            item['stdout_bytes'], item['stderr_bytes'] = out.tell(), err.tell()
            need(item['exit_code'] == 0 and not item['signals'] and item['stderr_bytes'] == 0,
                 name + ': original command did not return cleanly')
        finally:
            out.close()
            err.close()

    launcher = None
    try:
        launcher = start('operator-run', ssh('exec ' + shlex.join(
            [SELF, 'run', '01', str(deadline_ms // 1000), SETUP_SHA, OPERATOR_SHA])))
        # One bounded read-only startup observation, not an operation retry.
        # Original serve emits this line only after attaching its actual listener.
        ready_end = min(int(time.time()) + 45, deadline_ms // 1000 - 60)
        ready_script = ('set -eu; end=' + str(ready_end) + '; '
                        'while ! test -s ' + HOST + '/stdout.jsonl; do '
                        'test ! -f ' + HOST + '/result.json; test "$(date +%s)" -lt "$end"; sleep 0.1; done; '
                        'head -n 1 ' + HOST + '/stdout.jsonl')
        start('operator-listening', ssh(ready_script))
        finish('operator-listening', min(deadline - 60, time.monotonic() + 50), early_owner=launcher)
        line = json.loads((A / 'operator-listening.stdout').read_bytes())
        need(line['version'] == 2 and line['command'] == 'serve' and line['code'] == 'listening' and
             line['result'] == {'origin': 'https://turntable.proxy.rlwy.net:26354'} and
             now <= line['observed_at_ms'] <= time.time_ns() // 1000000, 'original fresh listener required')
        need(launcher.poll() is None and not interrupted[0] and time.monotonic() < deadline - 60,
             'original operator must remain live')
        private = OLD / 'client-inputs'
        command = [str(BIN), '--owned-root', str(OLD / 'owned/owned-gym'),
                   '--attachment-root', str(OLD / 'remote-attachment'),
                   '--reference', str(W / 'set81-prep/retained-admission-actual/inputs/reference-v1.json'),
                   '--remote-config', str(private / 'remote-config.json'), '--node-rpc', str(private / 'remote-rpc.json'),
                   '--pool-credential', str(private / 'pool.credential'),
                   '--admission-credential', str(private / 'admission.credential'),
                   '--prior-qualification', str(OLD / 'qualification.json'),
                   '--prior-money', str(OLD / 'proof-confirmed-money.json'),
                   '--prior-wallet', str(OLD / 'wallet-confirmed.json'),
                   '--deadline-ms', str(deadline_ms), '--report', str(A / 'native-report.json')]
        start('native-helper', command)
        finish('native-helper', deadline - 125, early_owner=launcher)
        raw = (A / 'native-report.json').read_bytes()
        need(len(raw) <= 256 * 1024, 'bounded native report')
        native = json.loads(raw)
        report['native_report_passed'] = native['passed'] is True and native['clean_join'] is True
        need(report['native_report_passed'] and native['deadline_at_ms'] == deadline_ms,
             'original native acceptance failed')
        # The native controller has now closed the original outbox. One exact
        # historical Lookup/duplicate Submit replay adds no Issue or new work.
        # Do not run the old wrapper, whose prior scenario deadline is expired.
        need(time.monotonic() + 65 <= deadline - 60, 'original replay and stop reserve required')
        replay_command = [str(PROOF), 'replay', '--reference', str(W / 'set81-prep/retained-admission-actual/inputs/reference-v1.json'),
                          '--setup', str(OLD / 'public/setup.json'), '--remote', str(private / 'remote-config.json'),
                          '--expected-setup-id', prior_replay['setup_id'], '--expected-remote-id', prior_replay['remote_id'],
                          '--pool-credential', str(private / 'pool.credential'),
                          '--attachment-root', str(OLD / 'remote-attachment'), '--sequence', '1', '--phase', 'after']
        start('outbox-replay', replay_command)
        finish('outbox-replay', min(deadline - 60, time.monotonic() + 65), early_owner=launcher)
        replay_raw = (A / 'outbox-replay.stdout').read_bytes()
        need(len(replay_raw) <= 1048576, 'bounded replay projection')
        replay = json.loads(replay_raw)
        need(replay['passed'] is True and replay['clean_join'] is True and replay['selected_inputs_unchanged'] is True and
             replay['new_issue_calls'] == 0 and replay['same_original_receipt'] is True and
             replay['outbox_before'] == replay['outbox_after'], 'original replay must preserve complete outbox')
        for field in ('rows', 'outbox_after', 'work', 'nonce', 'receipt', 'sequence', 'setup_id', 'remote_id', 'reference_sha256'):
            need(replay[field] == prior_replay[field], 'original retained replay field changed: ' + field)
        report['outbox_replay_unchanged'] = True
    except BaseException as e:
        report['errors'].append(type(e).__name__ + ': ' + str(e))
    finally:
        # Exactly one stop request, including any startup/native failure. The
        # unchanged host wrapper independently proves child Wait and monitor join.
        if launcher is not None:
            try:
                start('operator-stop', ssh('exec ' + shlex.join([SELF, 'stop', '01', SETUP_SHA, OPERATOR_SHA])))
                finish('operator-stop', min(deadline, time.monotonic() + 40), honor_signal=False)
                stopped = json.loads((A / 'operator-stop.stdout').read_bytes())
                report['stop_result'] = stopped
                need(stopped['clean'] is True and stopped['reason'] == 'requested' and
                     stopped['child_waited'] is True and stopped['monitor_joined'] is True and
                     stopped['child_exit'] == stopped['monitor_exit'] == 0 and stopped['generation'] == '01' and
                     stopped['deadline'] == deadline_ms // 1000 and stopped['setup_sha256'] == SETUP_SHA and
                     stopped['binary_sha256'] == OPERATOR_SHA, 'original host owner did not join cleanly')
            except BaseException as e:
                report['errors'].append('stop: ' + type(e).__name__ + ': ' + str(e))
            try:
                finish('operator-run', min(deadline, time.monotonic() + 10), honor_signal=False)
            except BaseException as e:
                report['errors'].append('launcher: ' + type(e).__name__ + ': ' + str(e))
        report['ended_at_ms'] = time.time_ns() // 1000000
        try:
            report['inputs_unchanged'] = (hashlib.sha256(BIN.read_bytes()).hexdigest() == BIN_SHA and
                hashlib.sha256(PROOF.read_bytes()).hexdigest() == PROOF_SHA and
                (OLD / 'execution-selection.json').read_bytes() == old_execution_raw and
                (OLD / 'proof-replay-after.json').read_bytes() == prior_raw)
        except OSError:
            report['errors'].append('native binary final recheck failed')
        if report['ended_at_ms'] > deadline_ms:
            report['errors'].append('shared execution deadline exceeded')
        report['passed'] = (not report['errors'] and report['native_report_passed'] and report['outbox_replay_unchanged'] and
                            report['inputs_unchanged'] and report['stop_result'] is not None and
                            all(v['joined'] and v['exit_code'] == 0 and not v['forced'] for v in report['processes'].values()))
        write('native-orchestration-report.json', report)
        for sig, handler in previous.items():
            signal.signal(sig, handler)
    print(json.dumps({'passed': report['passed'], 'report': str(A / 'native-orchestration-report.json')}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
