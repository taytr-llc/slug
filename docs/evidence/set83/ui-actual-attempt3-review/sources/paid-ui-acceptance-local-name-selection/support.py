"""Bounded files and serial original helper ownership. Root executes only."""
import hashlib
import json
import os
from pathlib import Path
import selectors
import signal
import stat
import struct
import subprocess
import time

APP = 'SLUG-REMOTE-DAILY-WALLET-2'
ARMS = ('snapshot', 'job', 'read', 'activity_job', 'activity_read', 'activity_page', 'activity_detail', 'payout_observation')
RESPONSE = ('version', 'application', 'id', 'command', 'config_id', 'view_id', 'ok') + ARMS + ('error',)
COMMAND_ARMS = dict(snapshot='snapshot', login_fixture='snapshot', stop='snapshot', logout='snapshot',
                    daily_payout_begin='job', daily_payout_read='read', daily_payout_cancel='read',
                    wallet_activity_begin='activity_job', wallet_activity_read='activity_read',
                    wallet_activity_cancel='activity_read', wallet_activity_page='activity_page',
                    wallet_activity_detail='activity_detail', daily_payout_observe='payout_observation')


def need(value, message):
    if not value:
        raise RuntimeError(message)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical(value):
    return json.dumps(value, ensure_ascii=True, separators=(',', ':'), allow_nan=False).replace('&', '\\u0026').replace('<', '\\u003c').replace('>', '\\u003e').encode('ascii')


def parse(raw):
    def pairs(items):
        result = {}
        for k, v in items:
            need(k not in result, 'duplicate JSON field')
            result[k] = v
        return result
    def bad(_):
        raise RuntimeError('noninteger JSON value')
    return json.loads(raw.decode('ascii'), object_pairs_hook=pairs, parse_float=bad, parse_constant=bad)


def identity(st):
    return (st.st_dev, st.st_ino, st.st_mode, st.st_nlink, st.st_uid, st.st_gid, st.st_size, st.st_mtime_ns, st.st_ctime_ns)


def read_file(path, cap, protected=False):
    path = Path(path)
    need(path.is_absolute() and path == path.resolve(strict=True), 'canonical nonlinked selected input')
    before = path.lstat()
    need(stat.S_ISREG(before.st_mode) and before.st_nlink == 1 and 0 < before.st_size <= cap, 'selected regular input bound')
    if protected:
        need(before.st_uid == os.geteuid() and stat.S_IMODE(before.st_mode) == 0o600, 'selected protected input owner')
    with os.fdopen(os.open(path, os.O_RDONLY | os.O_NOFOLLOW), 'rb') as f:
        need(identity(os.fstat(f.fileno())) == identity(before), 'selected descriptor identity')
        raw = f.read(cap + 1)
        need(identity(os.fstat(f.fileno())) == identity(before), 'selected descriptor changed')
    need(identity(path.lstat()) == identity(before) and len(raw) == before.st_size, 'selected input changed')
    return raw, identity(before)


def directory(path):
    p = Path(path)
    need(p.is_absolute() and p == p.resolve(strict=True), 'canonical selected directory')
    s = p.lstat()
    need(stat.S_ISDIR(s.st_mode) and stat.S_IMODE(s.st_mode) == 0o700 and s.st_uid == os.geteuid(), 'owned0700 directory')
    return (s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_gid)


def exclusive(path):
    path = Path(path)
    directory(path.parent)
    return os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600), 'wb', buffering=0)


def write_json(path, value):
    raw = json.dumps(value, indent=2, allow_nan=False).encode('ascii') + b'\n'
    need(len(raw) <= 4 * 1024 * 1024, 'public JSON report bound')
    with exclusive(path) as f:
        need(f.write(raw) == len(raw), 'complete evidence write')
        os.fsync(f.fileno())


def identifier(domain, version, value):
    raw = canonical(value)
    return sha(domain.encode('ascii') + b'\0' + version.encode('ascii') + b'\0' + struct.pack('<I', len(raw)) + raw)


class Helper:
    """Exactly one child, one serialized request at a time, one original Wait."""
    def __init__(self, command, root, number, deadline, interrupted, config_id):
        self.deadline, self.interrupted, self.config_id = deadline, interrupted, config_id
        self.id, self.buffer, self.stdout_total, self.stderr_total = 0, b'', 0, 0
        self.frames, self.signals, self.closed = [], [], False
        self.stdin_closed = False
        self.logs = []
        try:
            for suffix in ('requests.jsonl', 'stdout.jsonl', 'stderr.log'):
                self.logs.append(exclusive(root / ('helper-%d.%s' % (number, suffix))))
        except BaseException:
            for f in self.logs:
                f.close()
            raise
        env = os.environ.copy()
        for name in ('RAILWAY_TOKEN', 'RAILWAY_API_TOKEN'):
            env.pop(name, None)
        try:
            self.child = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                          env=env, start_new_session=True, close_fds=True, bufsize=0)
        except BaseException:
            for f in self.logs:
                f.close()
            raise
        self.report = dict(number=number, pid=self.child.pid, pgid=self.child.pid, parent_pid=os.getpid(),
                           started_at_ms=time.time_ns() // 1000000, joined=False, exit_code=None,
                           eof_requested=False, forced=False, signals=self.signals, commands=[], closed=False)
        self.select = selectors.DefaultSelector()
        for label, stream in (('stdout', self.child.stdout), ('stderr', self.child.stderr)):
            os.set_blocking(stream.fileno(), False)
            self.select.register(stream, selectors.EVENT_READ, label)
        os.set_blocking(self.child.stdin.fileno(), False)

    def pump(self, timeout):
        for event, _ in self.select.select(max(0, min(timeout, 0.1))):
            data = os.read(event.fileobj.fileno(), 65536)
            if not data:
                self.select.unregister(event.fileobj)
                continue
            index = 1 if event.data == 'stdout' else 2
            need(self.logs[index].write(data) == len(data), 'complete child evidence write')
            if index == 1:
                self.stdout_total += len(data)
                self.buffer += data
                need(self.stdout_total <= 16 * 1024 * 1024 and len(self.buffer) <= 98304, 'bounded original helper stdout')
            else:
                self.stderr_total += len(data)
                need(self.stderr_total <= 65536, 'bounded original helper stderr')

    def request(self, command, view='', timeout=30, **fields):
        need(command in COMMAND_ARMS and command != 'start', 'closed read-only command selection')
        need(not self.closed and not self.stdin_closed and not self.interrupted[0], 'original helper remains available')
        need(self.id < 2048, 'bounded command count')
        self.id += 1
        q = dict(version=2, application=APP, id=str(self.id), command=command, config_id=self.config_id,
                 view_id=view, fixture='', sequence='0', capture_sequence='0', before_batch='0',
                 after=0, limit=0, txid='', output_index=0, batch_sequence='0')
        need(set(fields) <= set(q) - {'version', 'application', 'id', 'command', 'config_id', 'view_id'}, 'closed selectors')
        q.update(fields)
        raw = canonical(q) + b'\n'
        need(len(raw) <= 2048, 'bounded request frame')
        until = min(self.deadline - 30, time.monotonic() + timeout)
        need(time.monotonic() < until and not self.buffer, 'original remaining command budget')
        need(self.logs[0].write(raw) == len(raw), 'complete original request evidence')
        position = 0
        while position < len(raw):
            need(time.monotonic() < until and not self.interrupted[0] and self.child.poll() is None, 'request delivery failed')
            try:
                position += os.write(self.child.stdin.fileno(), raw[position:])
            except BlockingIOError:
                self.pump(0.05)
        self.report['commands'].append(dict(id=q['id'], command=command, view_id=view, request_sha256=sha(raw), sent_at_ms=time.time_ns() // 1000000))
        while b'\n' not in self.buffer:
            need(time.monotonic() < until and not self.interrupted[0], 'original response deadline')
            self.pump(0.1)
            need(self.child.poll() is None or b'\n' in self.buffer, 'original helper exited before response')
        raw, self.buffer = self.buffer.split(b'\n', 1)
        raw += b'\n'
        cap = 98304 if command in ('wallet_activity_page', 'wallet_activity_detail') else 16384 if command.startswith('wallet_activity_') or command == 'daily_payout_observe' else 65536
        need(2 <= len(raw) <= cap and all(32 <= b <= 126 for b in raw[:-1]) and not self.buffer, 'single canonical response frame')
        r = parse(raw[:-1])
        need(canonical(r) + b'\n' == raw and tuple(r) == RESPONSE, 'strict complete response envelope')
        need(all(type(r[k]) is type(q[k]) and r[k] == q[k] for k in RESPONSE[:6]), 'exact original request correlation')
        need(type(r['ok']) is bool and type(r['error']) is str, 'response status shape')
        if r['ok']:
            arm = COMMAND_ARMS[command]
            need(r['error'] == '' and r[arm] is not None and all(r[x] is None for x in ARMS if x != arm), 'exact successful response arm')
        else:
            need(r['error'] in ('busy', 'stale_view') and all(r[x] is None for x in ARMS), 'unexpected definite or unknown refusal')
        self.report['commands'][-1].update(response_sha256=sha(raw), response_bytes=len(raw), received_at_ms=time.time_ns() // 1000000, ok=r['ok'], error=r['error'])
        self.frames.append((q, r))
        return r

    def call(self, command, view='', timeout=30, **fields):
        """Only exact exported Busy can repeat a read or same-sequence admission."""
        until = min(self.deadline - 30, time.monotonic() + timeout)
        while True:
            remaining = until - time.monotonic()
            need(remaining > 0, 'same-invocation Busy budget exhausted')
            r = self.request(command, view, timeout=remaining, **fields)
            if r['ok']:
                return r[COMMAND_ARMS[command]]
            need(r['error'] == 'busy', 'unexpected refusal in successful read path')
            time.sleep(min(0.1, max(0, until - time.monotonic())))

    def close(self):
        if self.closed:
            return self.report
        failures = []
        def drain_once():
            try:
                self.pump(0.1)
            except BaseException:
                failures.append('output drain failure')
                return False
            return True
        try:
            if not self.stdin_closed:
                try:
                    self.child.stdin.close()
                except BaseException:
                    failures.append('stdin close failure')
                self.stdin_closed = True
                self.report['eof_requested'] = True
            until = min(self.deadline - 12, time.monotonic() + 30)
            while self.child.poll() is None and time.monotonic() < until:
                if not drain_once():
                    break
            if self.child.poll() is None:
                self.signals.append('TERM')
                try:
                    os.killpg(self.child.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
                until = min(self.deadline - 7, time.monotonic() + 5)
                while self.child.poll() is None and time.monotonic() < until:
                    if not drain_once():
                        break
            if self.child.poll() is None:
                self.signals.append('KILL')
                self.report['forced'] = True
                try:
                    os.killpg(self.child.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            self.report['exit_code'] = self.child.wait(timeout=max(0, min(5, self.deadline-2-time.monotonic())))
            self.report['joined'] = True
            drain = min(self.deadline, time.monotonic() + 2)
            while self.select.get_map() and time.monotonic() < drain:
                if not drain_once():
                    break
            need(not self.select.get_map(), 'original output descriptors joined')
            need(not self.buffer, 'unsolicited or truncated final response')
            need(self.report['exit_code'] == 0 and not self.signals and self.stderr_total == 0 and not failures, 'original helper cleanup failed')
            self.report['closed'] = True
        finally:
            self.closed = True
            self.report.update(ended_at_ms=time.time_ns() // 1000000, stdout_bytes=self.stdout_total, stderr_bytes=self.stderr_total)
            self.select.close()
            for stream in (self.child.stdin, self.child.stdout, self.child.stderr):
                if not stream.closed:
                    stream.close()
            for f in self.logs:
                os.fsync(f.fileno())
                f.close()
        return self.report
