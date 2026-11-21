#!/usr/bin/env python3
"""Check only public hosted-selection bytes against the qualified Set81 fixture.

No product imports, network, subprocesses, keys, wallets, databases or Go.
This checks selection/codec bindings; it does not qualify TLS or wallet behavior.
"""
import argparse
import base64
import copy
import datetime as dt
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import stat
import struct
from urllib.parse import urlsplit

REFERENCE = '17557b28921290461fdf47b9ed6f1b800d3d077ee5d325a4ec59277ab3faca6a'
BASE = 'a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3'
ATTACHMENT = 'a48a854d20298fced726b816d11690aa45e9f4184e3b161cbc7c258b74897386'
PUBLIC_NAMES = ('endpoints.json', 'setup.json', 'remote.json', 'prepared.json', 'ca.pem', 'tls-cert.pem')


def need(ok, label):
    if not ok:
        raise ValueError(label)


def pairs(items):
    value = {}
    for key, item in items:
        need(key not in value, 'duplicate JSON field')
        value[key] = item
    return value


def invalid_number(_):
    raise ValueError('integer JSON values only')


def parsed(raw):
    return json.loads(raw.decode('ascii'), object_pairs_hook=pairs,
                      parse_float=invalid_number, parse_constant=invalid_number)


def canonical(value):
    return json.dumps(value, ensure_ascii=True, allow_nan=False, separators=(',', ':')).encode('ascii')


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def identity(domain, value, version='v1'):
    body = canonical(value)
    return digest(domain.encode('ascii') + b'\0' + version.encode('ascii') + b'\0' + struct.pack('<I', len(body)) + body)


def read_public(path, cap):
    path = Path(path)
    need(path.is_absolute() and str(path) == str(path.resolve(strict=True)), 'canonical public path without symlinks')
    directory = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    try:
        for part in path.parts[1:-1]:
            child = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=directory)
            os.close(directory)
            directory = child
        fd = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW, dir_fd=directory)
        try:
            before = os.fstat(fd)
            need(stat.S_ISREG(before.st_mode) and before.st_nlink == 1 and 0 < before.st_size <= cap, 'bounded regular public file')
            chunks, size = [], 0
            while True:
                block = os.read(fd, 65536)
                if not block:
                    break
                size += len(block)
                need(size <= cap, 'public file grew past bound')
                chunks.append(block)
            after = os.fstat(fd)
            need((before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns, before.st_ctime_ns) ==
                 (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns) and size == before.st_size,
                 'public input changed during read')
            raw = b''.join(chunks)
            return raw, {'path': str(path), 'bytes': size, 'sha256': digest(raw)}
        finally:
            os.close(fd)
    finally:
        os.close(directory)


def endpoint(value, scheme, loopback=False):
    need(type(value) is str and 0 < len(value) <= 512 and value.isascii(), 'bounded ASCII endpoint')
    uri = urlsplit(value)
    need(uri.scheme == scheme and uri.hostname and uri.username is None and uri.password is None and
         uri.path == '' and uri.query == '' and uri.fragment == '' and uri.geturl() == value and
         not any(c.isspace() for c in value), 'exact endpoint origin')
    port = uri.port
    need(port is None or 1 <= port <= 65535, 'endpoint port')
    if loopback:
        need(ipaddress.ip_address(uri.hostname).is_loopback and port is not None, 'literal loopback upstream with port')
    return uri


def listen(value):
    need(type(value) is str and len(value) <= 128, 'bounded listen address')
    uri = urlsplit('http://' + value)
    need(uri.hostname and uri.port is not None and 1 <= uri.port <= 65535 and uri.username is None and
         uri.password is None and uri.path == '' and uri.query == '' and uri.fragment == '' and
         uri.netloc == value, 'literal listen socket')
    ipaddress.ip_address(uri.hostname)


def utc_minute(value):
    need(type(value) is str and re.fullmatch(r'[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:00Z', value), 'exact UTC minute')
    moment = dt.datetime.strptime(value, '%Y-%m-%dT%H:%M:%SZ').replace(tzinfo=dt.timezone.utc)
    milliseconds = int((moment - dt.datetime(1970, 1, 1, tzinfo=dt.timezone.utc)).total_seconds()) * 1000
    need(0 < milliseconds <= 253402300799999, 'original report-clock range')
    return moment, milliseconds


def certificates(raw):
    need(len(raw) <= 131072 and raw.isascii(), 'bounded public certificate PEM')
    pattern = re.compile(rb'-----BEGIN CERTIFICATE-----\r?\n([A-Za-z0-9+/=\r\n]+)-----END CERTIFICATE-----')
    values, offset = [], 0
    for match in pattern.finditer(raw):
        need(raw[offset:match.start()].strip(b' \t\r\n') == b'', 'only certificate PEM blocks')
        body = re.sub(rb'[\r\n]', b'', match.group(1))
        der = base64.b64decode(body, validate=True)
        need(0 < len(der) <= 65536 and base64.b64encode(der) == body, 'bounded canonical certificate base64')
        values.append(der)
        need(len(values) <= 8, 'certificate count bound')
        offset = match.end()
    need(values and raw[offset:].strip(b' \t\r\n') == b'', 'no key or opaque PEM material')
    return values


def der_parts(raw, expected_tag):
    """Small strict DER framing reader solely to extract the public leaf SPKI."""
    def one(offset):
        need(offset + 2 <= len(raw), 'DER header')
        start, tag = offset, raw[offset]
        need(tag & 31 != 31, 'bounded one-byte DER tag')
        first = raw[offset + 1]
        offset += 2
        if first < 128:
            size = first
        else:
            count = first & 127
            need(1 <= count <= 4 and offset + count <= len(raw) and raw[offset] != 0, 'DER definite canonical length')
            size = int.from_bytes(raw[offset:offset + count], 'big')
            need(size >= 128, 'DER short length canonicality')
            offset += count
        end = offset + size
        need(end <= len(raw), 'DER content bound')
        return tag, raw[start:end], raw[offset:end], end
    tag, _, body, end = one(0)
    need(tag == expected_tag and end == len(raw), 'one complete DER object')
    original = raw
    raw = body
    values, offset = [], 0
    while offset < len(raw):
        tag, framed, body, offset = one(offset)
        values.append((tag, framed, body))
        need(len(values) <= 64, 'DER child count bound')
    need(original, 'nonempty DER object')
    return values


def leaf_spki(der):
    certificate = der_parts(der, 0x30)
    need(len(certificate) == 3 and [x[0] for x in certificate] == [0x30, 0x30, 0x03], 'X509 certificate public framing')
    tbs = der_parts(certificate[0][1], 0x30)
    start = 1 if tbs and tbs[0][0] == 0xa0 else 0
    need(len(tbs) >= start + 6 and [x[0] for x in tbs[start:start + 6]] == [0x02, 0x30, 0x30, 0x30, 0x30, 0x30], 'X509 TBS public fields')
    spki = tbs[start + 5][1]
    parts = der_parts(spki, 0x30)
    need(len(parts) == 2 and [x[0] for x in parts] == [0x30, 0x03], 'public SPKI framing')
    return digest(spki)


def verify(directory, reference, origin, due_utc, initial_start_ms):
    raw, ref_identity = read_public(reference, 1 << 20)
    need(ref_identity['bytes'] == 174293 and ref_identity['sha256'] == REFERENCE, 'qualified independent reference')
    ref = parsed(raw)
    inputs = {str(reference): (1 << 20, ref_identity)}
    public = {}
    for name in PUBLIC_NAMES:
        body, pin = read_public(directory / name, 512 << 10)
        inputs[str(directory / name)] = (512 << 10, pin)
        public[name] = body
    host = parsed(public['endpoints.json'])
    need(list(host) == ['version', 'origin', 'listen', 'pool_url', 'admission_url', 'ownership_url', 'due_utc'], 'public host selection contains endpoints/due only')
    need(type(host['version']) is int and host['version'] == 1 and host['origin'] == origin and host['due_utc'] == due_utc, 'original explicit root selection')
    endpoint(origin, 'https')
    listen(host['listen'])
    for name in ('pool_url', 'admission_url', 'ownership_url'):
        endpoint(host[name], 'http', True)
    need(len({host[k] for k in ('pool_url', 'admission_url', 'ownership_url')}) == 3, 'distinct original upstream listeners')
    due, due_ms = utc_minute(due_utc)
    need(type(initial_start_ms) is int and 0 < initial_start_ms <= 253402300799999, 'original initial start')
    start = dt.datetime.fromtimestamp(initial_start_ms / 1000, tz=dt.timezone.utc)
    need(due_ms == ((initial_start_ms + 7 * 60 * 1000) // 60000) * 60000 and
         start.date() == (start + dt.timedelta(minutes=15)).date(), 'unchanged initial seven-minute due and same-day bound')
    ca, chain = certificates(public['ca.pem']), certificates(public['tls-cert.pem'])
    # Public DER framing and leaf SPKI only. Chain signatures, CA constraints,
    # hostname and lifetime remain original preparer/client/handshake checks.
    for certificate in ca + chain:
        leaf_spki(certificate)
    spki = leaf_spki(chain[0])
    base_setup = copy.deepcopy(ref['setup']['base'])
    base_setup['schedule']['daily_utc_minute'] = due.hour * 60 + due.minute
    base_setup['ingress'] = dict(version=2, listen=host['listen'], origin=origin, certificate_spki=spki,
                                 pool_url=host['pool_url'], admission_url=host['admission_url'], ownership_url=host['ownership_url'])
    base_setup_id = identity('SLUG pool operator setup', base_setup, 'v2')
    source_id = digest(b'SLUG pool share daily source\0v2\0' + canonical(base_setup['pool']))
    need(source_id == ref['profile']['source_config_id'] == base_setup['wallet_guard']['source_binding_id'], 'unchanged original source identity')
    profile = dict(version=1, base_setup_id=base_setup_id, source_config_id=source_id, max_requests=base_setup['pool']['max_works'])
    profile_id = identity('SLUG pool issue recovery profile', profile)
    setup = dict(version=1, base=base_setup, issue_profile=profile)
    base_remote = copy.deepcopy(ref['remote']['base'])
    base_remote['origin'], base_remote['ca_certificate'] = origin, public['ca.pem'].decode('ascii')
    remote = dict(version=1, base=base_remote, issue_profile=profile)
    need(public['setup.json'] == canonical(setup), 'exact ordered Setup; only endpoint/TLS/schedule normalization')
    need(public['remote.json'] == canonical(remote), 'exact ordered remote; only Origin/CA and dependent profile normalization')
    setup_id = identity('SLUG pool issue recovery setup', setup)
    remote_id = identity('SLUG pool issue recovery attachment', remote)
    base_remote_id = identity('SLUG remote pool attachment', base_remote, 'v2')
    # Independent inherited two-input/two-native-output shape; never derive a
    # desired amount from a product report or from the supplied prepared fields.
    stripped = 4 + 1 + 2 * (36 + 1 + 4) + 1 + 2 * (8 + 1 + 22) + 4
    witness = 2 + 2 * (1 + 1 + 73 + 1 + 33)
    weight, gross = stripped * 4 + witness, 2 * 1000000000
    vsize = (weight + 3) // 4
    payment, fee = gross * 99 // 100, vsize
    change = gross - payment - fee
    need((stripped, witness, weight, vsize, payment, fee, change) == (154, 220, 836, 209, 1980000000, 209, 19999791), 'independent shape/economic anchors')
    need(ref['selected_economics']['miner_atoms'] == payment and ref['selected_economics']['fee_atoms'] == fee and
         ref['selected_economics']['change_atoms'] == change, 'qualified economic premise')
    prepared = parsed(public['prepared.json'])
    need(type(prepared.get('node_rpc_copied')) is bool, 'typed preparation-only node RPC copy flag')
    wanted = dict(version=1, source_reference_sha256=REFERENCE, attachment_reference_sha256=ATTACHMENT,
                  base_reference_sha256=BASE, setup_id=setup_id, base_setup_id=base_setup_id,
                  profile_id=profile_id, remote_id=remote_id, base_remote_id=base_remote_id,
                  due_at_ms=due_ms, credential_expires_at_ms=due_ms + 3600000, tls_spki=spki, wallet_created=True, wallet_closed=True,
                  expected_rewards=2, miner_atoms=payment, fee_atoms=fee, change_atoms=change,
                  node_rpc_copied=prepared['node_rpc_copied'])
    need(canonical(prepared) == canonical(wanted), 'complete exact prepared selection metadata')
    for path, (cap, before) in inputs.items():
        _, after = read_public(Path(path), cap)
        need(after == before, 'public selection unchanged after checks')
    return dict(version=1, passed=True, scope='Public hosted selection and independent economic bindings only',
                runtime_qualified=False, tls_handshake_qualified=False, wallet_qualified=False,
                source_reference_sha256=REFERENCE, setup_id=setup_id, base_setup_id=base_setup_id,
                profile_id=profile_id, remote_id=remote_id, base_remote_id=base_remote_id, source_id=source_id,
                original_initial_start_ms=initial_start_ms, due_at_ms=due_ms, tls_spki=spki,
                economics=dict(rewards=2, gross_atoms=gross, miner_atoms=payment, fee_atoms=fee, change_atoms=change,
                               shape_upper_bound=dict(raw_bytes=stripped+witness, weight=weight, vsize=vsize)),
                inputs=[item[1] for item in inputs.values()])


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--public', required=True)
    p.add_argument('--reference', required=True)
    p.add_argument('--origin', required=True)
    p.add_argument('--due-utc', required=True)
    p.add_argument('--initial-start-ms', required=True, type=int)
    p.add_argument('--out', required=True)
    a = p.parse_args()
    out = Path(a.out)
    directory, reference = Path(a.public), Path(a.reference)
    need(directory.is_absolute() and directory.is_dir() and str(directory) == str(directory.resolve(strict=True)), 'canonical public directory')
    need(out.is_absolute() and not out.exists() and not out.is_symlink() and directory not in out.parents and
         str(out.parent) == str(out.parent.resolve(strict=True)), 'fresh external output')
    try:
        report = verify(directory, reference, a.origin, a.due_utc, a.initial_start_ms)
    except (ValueError, OSError, KeyError, TypeError, IndexError, OverflowError) as error:
        report = dict(version=1, passed=False, scope='Public hosted selection verification',
                      runtime_qualified=False, error=type(error).__name__ + ': ' + str(error))
    fd = os.open(out, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'wb') as handle:
        handle.write((json.dumps(report, indent=2, sort_keys=True) + '\n').encode('ascii'))
    print(json.dumps({'passed': report['passed'], 'report': str(out)}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
