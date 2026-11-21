#!/usr/bin/env python3
"""Unexecuted independent Set80 A/B selection wrapper producer; stdlib only.

Root alone admits execution using the exact source/input lock. This program never
imports product/old producer code or reads runtime/private fixture material.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import struct

HERE = Path(__file__).resolve().parent
A48 = 'a48a854d20298fced726b816d11690aa45e9f4184e3b161cbc7c258b74897386'
BASE = 'a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3'
ISSUE = '4bbe4eb9556ed5a894cee6ff565b0dcca805f27fea2d77cfe3a56242dc3dac91'

def need(ok, message):
    if not ok:
        raise ValueError(message)

def canonical(value):
    return json.dumps(value, ensure_ascii=True, allow_nan=False, separators=(',', ':')).encode('ascii')

def digest(raw):
    return hashlib.sha256(raw).hexdigest()

def frame(label, raw, version='v1'):
    need(len(raw) <= 0xffffffff, 'framed size')
    return label.encode('ascii') + b'\0' + version.encode('ascii') + b'\0' + struct.pack('<I', len(raw)) + raw

def identity(label, value, version='v1'):
    return digest(frame(label, canonical(value), version))

def owned(path, expected, cap):
    raw = path.read_bytes()
    need(0 < len(raw) <= cap and digest(raw) == expected, 'input identity:' + path.name)
    return json.loads(raw)

def build():
    old = owned(HERE / 'inputs/path-c-reference.json', A48, 262144)
    base = owned(HERE / 'inputs/base-reference.json', BASE, 4 << 20)
    selected = owned(HERE / 'inputs/reference-v1.json', ISSUE, 8 << 20)
    need(selected['version'] == 1 and len(selected['fixtures']['profiles']) == 3, 'selected reference grammar')
    s, c, worker = old['setup'], old['remote'], old['worker']
    need(canonical(s).hex() == old['setup_hex'] and identity('SLUG pool operator setup', s, 'v2') == old['setup_id'], 'exact inherited setup')
    need(canonical(c).hex() == old['remote_hex'] and identity('SLUG remote pool attachment', c, 'v2') == old['remote_id'], 'exact inherited remote')
    need(s['version'] == c['version'] == worker['version'] == 2 and old['ordinary_maximum'] == 2, 'explicit original daily selection')
    need(canonical(s['pool']) == canonical(c['source']) == canonical(worker['source']), 'same original source')
    need(canonical(c['settlement']) == canonical(s['settlement']), 'same original financial policy')
    source_id = digest(b'SLUG pool share daily source\0v2\0' + canonical(s['pool']))
    need(source_id == s['wallet_guard']['source_binding_id'], 'original source digest')
    profile = dict(version=1, base_setup_id=old['setup_id'], source_config_id=source_id, max_requests=s['pool']['max_works'])
    need(1 <= profile['max_requests'] <= 8192, 'finite retained requests')
    pid = identity('SLUG pool issue recovery profile', profile)
    setup = dict(version=1, base=s, issue_profile=profile)
    remote = dict(version=1, base=c, issue_profile=profile)
    setup_id = identity('SLUG pool issue recovery setup', setup)
    remote_id = identity('SLUG pool issue recovery attachment', remote)
    settlement = s['miners']['settlement_config_id']
    recipient = c['expected_local']['recipient']['recipient']['script']
    trust_preimage = frame('SLUG pool issue recovery transport', (worker['transport_trust_id'] + pid).encode('ascii'))
    wrapped_worker = dict(version=1, base=worker, issue_profile=profile, settlement_config_id=settlement,
                          recipient_script=recipient, transport_trust_id=digest(trust_preimage))
    need(canonical(setup['base']) == canonical(s) and canonical(remote['base']) == canonical(c) and canonical(wrapped_worker['base']) == canonical(worker), 'unchanged base objects')
    original = next(v for v in base['composition']['positives'] if v['name'] == 'two-enrollments')
    need(original['setup_id'] == old['base_composition_setup_id'], 'original published custody identity')
    frames = []
    for name, value, label in [('profile', profile, 'SLUG pool issue recovery profile'), ('setup', setup, 'SLUG pool issue recovery setup'), ('remote', remote, 'SLUG pool issue recovery attachment')]:
        raw = canonical(value)
        frames.append(dict(name=name, value=value, bytes=len(raw), canonical_hex=raw.hex(), id=identity(label, value), preimage_hex=frame(label, raw).hex()))
    intent = dict(version=1, application='slug-remote-pool-issue-recovery', config_id=remote_id, config=remote)
    ready = dict(version=1, application=intent['application'], config_id=remote_id, intent_sha256=digest(canonical(intent)+b'\n'))
    operator_ready = dict(version=1, runtime_id=s['runtime_id'], setup_id=setup_id, issue_profile_id=pid)
    for name, value in [('operator-intent', setup), ('operator-ready', operator_ready), ('remote-intent', intent), ('remote-ready', ready)]:
        raw=canonical(value)+b'\n'
        frames.append(dict(name=name, value=value, bytes=len(raw), canonical_hex=raw.hex(), framing='exact-final-lf'))
    # Credential frames are public-shape examples with explicit synthetic tokens,
    # not a live credential/session. Base ConfigID is the OLD remote config ID.
    for audience, token in [('pool', '4'*64), ('admission', '5'*64)]:
        inner=dict(version=2, config_id=old['remote_id'], gym_id=c['expected_local']['recipient']['config']['identity']['gym_id'], audience=audience,
                   generation=1, expires_at_ms=1794790800000, token=token)
        value=dict(version=1, base=inner, issue_profile_id=pid)
        raw=canonical(value)+b'\n'
        frames.append(dict(name='synthetic-credential-'+audience, value=value, bytes=len(raw), canonical_hex=raw.hex(), framing='exact-final-lf', scope='published shape only; scenario enrolls original session separately'))
    need(len(frames)==9, 'complete wrapper frame inventory')
    need(len(canonical(setup)) <= 262144+1024 and len(canonical(remote)) <= 262144+1024, 'conservative inherited wrapper envelope')
    need([v['accepted_rewards'] for v in old['economics']] == [1,2], 'original finite economic branches')
    return dict(version=1, scope='independent wrapper selections over qualified Set79 Path C; no new PoW, authorization or runtime evidence',
                inputs=dict(path_c=A48, base=BASE, issue=ISSUE), fixture=old['fixture'], base_actual=old,
                profile=profile, profile_id=pid, setup=setup, setup_id=setup_id, remote=remote, remote_id=remote_id,
                worker=wrapped_worker, transport_trust_id=wrapped_worker['transport_trust_id'], trust_preimage_hex=trust_preimage.hex(),
                frames=frames, economics=old['economics'], ordinary_maximum=2,
                expected_gap=dict(source_requests=1, source_state='pending', source_works=0, issuer_requests=1, issuer_jobs=1, issuer_state='issued', submissions=0, accepted=0, births=0),
                expected_b=dict(source_state='issued_unused', issuer_state='issued', reason='expired_before_work', works=0, submissions=0, accepted=0, births=0, credits=0, batches=0),
                exclusions=['No generated mining result', 'No product output read', 'No new custody or payout rule', 'Runtime ephemeral transport/due fields require separate exact normalization and binding'])

def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--lock', required=True)
    parser.add_argument('--output', required=True)
    args=parser.parse_args()
    lock_path=Path(args.lock).resolve(); output=Path(args.output)
    need(output.is_absolute() and not output.exists() and not output.is_relative_to(HERE), 'exclusive external output')
    lock_raw=lock_path.read_bytes(); lock=json.loads(lock_raw)
    need(lock.get('execution_authorized') is True and lock.get('version')==1, 'root execution admission')
    before={}
    for row in lock['files']:
        relative=Path(row['path'])
        need(not relative.is_absolute() and '..' not in relative.parts and row['path'] not in before, 'locked relative path')
        path=HERE/relative
        need(path.is_file() and not any((HERE/Path(*relative.parts[:i])).is_symlink() for i in range(1,len(relative.parts)+1)), 'locked regular public input')
        raw=path.read_bytes();need(len(raw)==row['bytes'] and digest(raw)==row['sha256'], 'locked identity')
        before[row['path']]=raw
    need('selection_reference.py' in before and all('inputs/'+name in before for name in ['path-c-reference.json','base-reference.json','reference-v1.json']), 'complete selected source admission')
    try:
        raw=canonical(build())+b'\n';need(len(raw)<=1048576,'output bound')
        # Source identity is rechecked before publishing even a correct model.
        for name,data in before.items():need((HERE/name).read_bytes()==data,'source changed')
        need(lock_path.read_bytes()==lock_raw,'lock changed')
        fd=os.open(output,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
        with os.fdopen(fd,'wb') as f:
            need(f.write(raw)==len(raw),'complete output');f.flush();os.fsync(f.fileno())
        print(json.dumps(dict(passed=True,bytes=len(raw),sha256=digest(raw),frames=9)))
    finally:
        for name,data in before.items():need((HERE/name).read_bytes()==data,'final source changed')
        need(lock_path.read_bytes()==lock_raw,'final lock changed')
if __name__=='__main__':main()
