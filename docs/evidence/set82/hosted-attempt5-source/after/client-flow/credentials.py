#!/usr/bin/env python3
"""Offline published-fixture wrapper encoding. No network or product execution."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import stat
import struct
import time

ORIGIN = 'https://turntable.proxy.rlwy.net:26354'
REFERENCE = '17557b28921290461fdf47b9ed6f1b800d3d077ee5d325a4ec59277ab3faca6a'


def need(ok, message):
    if not ok:
        raise ValueError(message)


def pairs(rows):
    result = {}
    for key, value in rows:
        need(key not in result, 'duplicate JSON field')
        result[key] = value
    return result


def number(_):
    raise ValueError('noninteger JSON number')


def parse(raw):
    return json.loads(raw.decode('ascii'), object_pairs_hook=pairs,
                      parse_float=number, parse_constant=number)


def canon(value):
    return json.dumps(value, ensure_ascii=True, separators=(',', ':'), allow_nan=False).replace('<', '\\u003c').replace('>', '\\u003e').replace('&', '\\u0026').encode('ascii')


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def ident(domain, value, version='v1'):
    body = canon(value)
    return sha(domain.encode('ascii') + b'\0' + version.encode('ascii') + b'\0' + struct.pack('<I', len(body)) + body)


def integer(value, lower, upper):
    return type(value) is int and lower <= value <= upper


def read(path, cap, protected=False):
    path = Path(path)
    need(path.is_absolute() and path == path.resolve(strict=True), 'canonical original input required')
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        st = os.fstat(fd)
        need(stat.S_ISREG(st.st_mode) and st.st_nlink == 1 and 0 < st.st_size <= cap, 'bounded single-link regular input')
        if protected:
            need(st.st_uid == os.geteuid() and stat.S_IMODE(st.st_mode) & 0o077 == 0, 'protected input ownership')
        raw = b''
        while len(raw) <= cap:
            part = os.read(fd, min(65536, cap + 1 - len(raw)))
            if not part:
                break
            raw += part
        end = os.fstat(fd)
        need((st.st_dev, st.st_ino, st.st_size, st.st_mtime_ns, st.st_ctime_ns) ==
             (end.st_dev, end.st_ino, end.st_size, end.st_mtime_ns, end.st_ctime_ns) and len(raw) == st.st_size,
             'input changed during read')
        return raw
    finally:
        os.close(fd)


def exclusive(path, raw):
    path = Path(path)
    need(path.is_absolute() and path.parent == path.parent.resolve(strict=True), 'canonical output parent')
    st = path.parent.stat()
    need(st.st_uid == os.geteuid() and stat.S_IMODE(st.st_mode) == 0o700, 'owned0700 output parent')
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    try:
        need(os.write(fd, raw) == len(raw), 'complete exclusive write')
        os.fsync(fd)
    finally:
        os.close(fd)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for flag in ('selection-report', 'setup', 'remote', 'prepared', 'rotate-result', 'token-file', 'output'):
        p.add_argument('--' + flag, required=True)
    p.add_argument('--audience', choices=('pool', 'admission'), required=True)
    a = p.parse_args()
    original = {}
    def selected(name, cap, protected=False):
        path = getattr(a, name.replace('-', '_'))
        raw = read(path, cap, protected)
        original[path] = (cap, protected, raw)
        return parse(raw)
    report = selected('selection-report', 65536)
    need(report.get('passed') is True and report.get('source_reference_sha256') == REFERENCE,
         'root-admitted independent public selection report required')
    setup, remote, prepared = selected('setup', 1 << 20), selected('remote', 1 << 20), selected('prepared', 65536)
    # The checker report is a root-selected artifact, not a replacement authority.
    for name in ('setup', 'remote', 'prepared'):
        raw = original[getattr(a, name)][2]
        matches = [r for r in report['inputs'] if Path(r['path']).name == name + '.json']
        need(len(matches) == 1 and matches[0]['bytes'] == len(raw) and matches[0]['sha256'] == sha(raw), 'exact independently checked public bytes')
    need(setup['version'] == remote['version'] == 1 and remote['base']['version'] == 2 and remote['base']['origin'] == ORIGIN, 'selected hosted mode/origin')
    need(setup['issue_profile'] == remote['issue_profile'], 'one original issue profile')
    base_id = ident('SLUG remote pool attachment', remote['base'], 'v2')
    profile_id = ident('SLUG pool issue recovery profile', remote['issue_profile'])
    remote_id = ident('SLUG pool issue recovery attachment', remote)
    setup_id = ident('SLUG pool issue recovery setup', setup)
    for key, value in (('base_remote_id', base_id), ('profile_id', profile_id), ('remote_id', remote_id), ('setup_id', setup_id)):
        need(report[key] == prepared[key] == value, 'selected public identity mismatch')
    gym = remote['base']['expected_local']['recipient']['config']['identity']['gym_id']
    enrollments = setup['base']['miners']['enrollments']
    need(len(enrollments) == 2 and enrollments[0]['gym_id'] == gym and setup['base']['miners'] == remote['base']['miners'], 'original two-enrollment selection')
    frame = selected('rotate-result', 16384)
    need(list(frame) == ['version', 'command', 'observed_at_ms', 'code', 'result'] and frame['version'] == 2 and frame['command'] == 'rotate' and frame['code'] == 'complete', 'exact successful original rotate frame')
    c = frame['result']
    need(list(c) == ['gym_id', 'audience', 'generation', 'token_hash', 'reuse_hash', 'issued_at_ms', 'expires_at_ms', 'revoked_at_ms'], 'exact credential result fields')
    expiry = prepared['credential_expires_at_ms']
    need(integer(expiry, 1, 253402300799999) and expiry == prepared['due_at_ms'] + 3600000 and expiry > time.time_ns() // 1000000, 'original selected credential expiry still live')
    need(c['gym_id'] == gym and c['audience'] == a.audience and type(c['generation']) is int and c['generation'] == 1 and type(c['revoked_at_ms']) is int and c['revoked_at_ms'] == 0 and type(c['expires_at_ms']) is int and c['expires_at_ms'] == expiry, 'exact original generation1 nonrevoked credential')
    need(integer(c['issued_at_ms'], 1, expiry - 1) and integer(frame['observed_at_ms'], c['issued_at_ms'], expiry - 1), 'original issuance/observation timestamps')
    token_raw = read(a.token_file, 64, True)
    original[a.token_file] = (64, True, token_raw)
    token = token_raw.decode('ascii')
    need(token == sha(('SLUG Set79 Path A ' + a.audience + ' ' + gym).encode('ascii')), 'exact published fixture token, no new secret')
    enrollment_id = ident('SLUG pool miner enrollment', setup['base']['miners'], 'v2')
    need(c['token_hash'] == ident('SLUG pool miner credential', dict(config_id=enrollment_id, audience=a.audience, token=token), 'v2') and
         c['reuse_hash'] == ident('SLUG pool miner token reuse', dict(config_id=enrollment_id, token=token), 'v2'), 'real rotate token/enrollment hashes')
    wrapper = dict(version=1, base=dict(version=2, config_id=base_id, gym_id=gym, audience=a.audience,
                                       generation=1, expires_at_ms=expiry, token=token), issue_profile_id=profile_id)
    for path, (cap, protected, raw) in original.items():
        need(read(path, cap, protected) == raw, 'original selected input changed')
    body = canon(wrapper) + b'\n'
    need(len(body) <= 2048, 'original credential size bound')
    exclusive(a.output, body)
    for path, (cap, protected, raw) in original.items():
        need(read(path, cap, protected) == raw, 'input changed after output; retain output as failed')
    print(json.dumps(dict(version=1, encoded=True, audience=a.audience, gym_id=gym,
                          generation=1, expires_at_ms=expiry, base_remote_id=base_id,
                          issue_profile_id=profile_id, remote_id=remote_id), separators=(',', ':')))


if __name__ == '__main__':
    try:
        main()
    except Exception:
        # Never echo a token, private input, arbitrary OS path or input bytes.
        raise SystemExit('published credential preparation refused; retain all inputs/partial output')
