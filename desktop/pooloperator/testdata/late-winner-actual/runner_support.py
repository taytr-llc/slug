#!/usr/bin/env python3
"""Copied Set73 finite owned-file/group leaves. No action on import."""
import hashlib
import json
import os
from pathlib import Path
import signal
import stat
import subprocess
import time

def identity(path):
    path = Path(path)
    for part in (path, *path.parents):
        if stat.S_ISLNK(part.lstat().st_mode):
            raise RuntimeError('symlink source refused')
    if not path.is_file():
        raise RuntimeError('ordinary input required')
    h = hashlib.sha256()
    with path.open('rb') as f:
        while block := f.read(1 << 20):
            h.update(block)
    return dict(bytes=path.stat().st_size, sha256=h.hexdigest())


def write(path, obj):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'w') as f:
        json.dump(obj, f, indent=2, sort_keys=True)
        f.write('\n')
        f.flush()
        os.fsync(f.fileno())


def group_alive(pid):
    try:
        os.killpg(pid, 0)
        return True
    except ProcessLookupError:
        return False


def cleanup(p):
    # Preserve first cleanup uncertainty. A later absent group is useful census
    # evidence, but does not convert a failed first probe/signal into clean join.
    row = dict(pid=p.pid, pgid=p.pid, leftover_after_exit=None, signals=[],
               group_empty=False, errors=[])
    def failure(operation, error):
        row['errors'].append(dict(operation=operation, error=type(error).__name__,
                                  errno=getattr(error, 'errno', None)))
    def probe(operation):
        try:
            return group_alive(p.pid)
        except OSError as e:
            failure(operation, e)
            return None
    def send(sig, operation):
        try:
            os.killpg(p.pid, sig)
            row['signals'].append(operation)
        except ProcessLookupError:
            pass # Re-probe below; ESRCH alone is not a new ownership fact.
        except OSError as e:
            failure(operation, e)
    alive = probe('initial_group_probe')
    row['leftover_after_exit'] = alive
    if alive is True:
        send(signal.SIGTERM, 'SIGTERM')
        end = time.monotonic() + 15
        while alive is True and time.monotonic() < end and not row['errors']:
            p.poll(); time.sleep(.1); alive = probe('term_group_probe')
        if alive is True and not row['errors']:
            send(signal.SIGKILL, 'SIGKILL')
    try:
        p.wait(timeout=5)
    except (OSError, subprocess.TimeoutExpired) as e:
        failure('process_wait', e)
    end = time.monotonic() + 5
    alive = probe('final_group_probe')
    while alive is True and time.monotonic() < end and not row['errors']:
        time.sleep(.1); alive = probe('final_group_probe')
    row['group_empty'] = alive is False
    row['return_code'] = p.returncode
    return row


def owned_group_census(pgid, permitted):
    # This is a read-only exact PGID census, never a name-based signal target.
    result = dict(pgid=pgid, members=[], available=False)
    try:
        raw = subprocess.check_output(['/bin/ps', '-axo', 'pid=,pgid=,comm='], timeout=5)
        if len(raw) > 2 << 20:
            raise RuntimeError('process census bound')
        for line in raw.decode().splitlines():
            fields = line.strip().split(None, 2)
            if len(fields) != 3 or int(fields[1]) != pgid:
                continue
            result['members'].append(dict(pid=int(fields[0]), executable=fields[2],
                                          expected=fields[2] in permitted))
            if len(result['members']) > 8:
                raise RuntimeError('owned group member bound')
        result['available'] = True
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as e:
        result['error'] = dict(type=type(e).__name__, errno=getattr(e, 'errno', None))
    return result


