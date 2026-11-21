#!/usr/bin/env python3
"""Root-only closed-owner hash catalog; no database decoding or mutation."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import stat

W = Path('/Users/taytrrip/slug')
A = W / 'artifacts/set83/native-actual-attempt1'
OLD = W / 'artifacts/set82/hosted-actual-attempt5'
CAT = W / 'set81-prep/retained-admission-actual/fixture-catalog.json'
CAT_SHA = '2637978c2e08637dae2a21bf7fcb8e9ab37edb3476e2b9512c73ff16809b7e55'
ROOTS = {'owned': OLD / 'owned/owned-gym', 'attachment': OLD / 'remote-attachment'}
EXACT = ['fixture-provider/state.json']
PREFIX = ['authority/db/', 'coordinator/db/', 'fixture-provider/instance-lock/',
          'hosted-vault/db/', 'instance-lock/',
          'shared/gyms/incrementing256/owned-wallet/inventory/db/',
          'shared/gyms/incrementing256/owned-wallet/wallet/']
MARKERS = {'SLUG-REMOTE-POOL-INTENT.json', 'SLUG-REMOTE-POOL-READY.json',
           'outbox/SLUG-POOL-WORKER.json'}


def need(ok, why):
    if not ok:
        raise ValueError(why)


def read(path, cap=64 << 20):
    need(path.is_absolute() and path.resolve(strict=True) == path, 'canonical original file')
    before = path.lstat()
    need(stat.S_ISREG(before.st_mode) and before.st_nlink == 1 and 0 <= before.st_size <= cap, 'bounded regular file')
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, 'rb') as stream:
        opened = os.fstat(stream.fileno())
        need((before.st_dev, before.st_ino) == (opened.st_dev, opened.st_ino), 'file replaced before read')
        raw = stream.read(cap + 1)
        after = os.fstat(stream.fileno())
    now = path.lstat()
    fields = lambda x: (x.st_dev, x.st_ino, x.st_size, x.st_mode, x.st_uid, x.st_gid, x.st_mtime_ns)
    need(fields(before) == fields(after) == fields(now) and len(raw) == before.st_size, 'file changed during closed capture')
    return raw, {'bytes': len(raw), 'sha256': hashlib.sha256(raw).hexdigest(), 'mode': stat.S_IMODE(before.st_mode)}


def tree(root):
    need(root.resolve(strict=True) == root, 'canonical original root')
    st = root.lstat()
    need(stat.S_ISDIR(st.st_mode) and stat.S_IMODE(st.st_mode) == 0o700 and st.st_uid == os.geteuid(), 'owned0700 original root')
    files, dirs, total = {}, {}, 0
    for directory, names, leaves in os.walk(root, followlinks=False):
        names.sort(); leaves.sort()
        for name in names + leaves:
            path = Path(directory) / name
            info = path.lstat()
            need(not stat.S_ISLNK(info.st_mode), 'symlink refused')
            rel = str(path.relative_to(root))
            if stat.S_ISDIR(info.st_mode):
                dirs[rel] = stat.S_IMODE(info.st_mode)
            else:
                _, row = read(path)
                files[rel] = row
                total += row['bytes']
            need(len(files) + len(dirs) <= 2000 and total <= 64 << 20, 'finite original catalog cap')
    end = root.lstat()
    identity = lambda x: {'device': x.st_dev, 'inode': x.st_ino, 'uid': x.st_uid, 'gid': x.st_gid, 'mode': stat.S_IMODE(x.st_mode)}
    need(identity(st) == identity(end), 'root replaced during capture')
    return {'path': str(root), 'identity': identity(st), 'files': files, 'directories': dirs}


def policy(catalog, trees):
    need(catalog['permitted_mutable_exact'] == EXACT and catalog['permitted_mutable_prefixes'] == PREFIX, 'original mutable policy changed')
    owned = trees['owned']
    need(owned['directories'] == catalog['directories'], 'original owned directory inventory changed')
    need(all(owned['files'].get(p) == catalog['files'][p] for p in catalog['immutable_copy_paths']), 'immutable baseline history changed')
    changes = [p for p in set(owned['files']) | set(catalog['files']) if owned['files'].get(p) != catalog['files'].get(p)]
    need(all(p in EXACT or any(p.startswith(prefix) for prefix in PREFIX) for p in changes), 'unlisted account or wallet file change')
    attachment = trees['attachment']
    need(set(attachment['directories']) == {'outbox', 'outbox/db'} and all(v == 0o700 for v in attachment['directories'].values()), 'attachment directory inventory changed')
    need(set(p for p in attachment['files'] if not p.startswith('outbox/db/')) == MARKERS, 'unlisted attachment control')


def write(name, value):
    need(A.resolve(strict=True) == A and stat.S_IMODE(A.stat().st_mode) == 0o700 and A.stat().st_uid == os.geteuid(), 'fresh evidence parent must be owned0700')
    fd = os.open(A / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'w') as f:
        json.dump(value, f, indent=2, sort_keys=True)
        f.write('\n'); f.flush(); os.fsync(f.fileno())


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['before', 'after', 'compare'])
    args = parser.parse_args()
    raw, cat_id = read(CAT, 1 << 20)
    need(cat_id['sha256'] == CAT_SHA, 'original public catalog selection')
    catalog = json.loads(raw)
    if args.action != 'compare':
        trees = {name: tree(root) for name, root in ROOTS.items()}
        policy(catalog, trees)
        write('paid-roots-' + args.action + '.json', {'version': 1, 'scope': 'closed owner hash inventory only; root separately observes owner joins', 'catalog_sha256': CAT_SHA, 'trees': trees, 'immutable_baseline_paths': len(catalog['immutable_copy_paths']), 'logical_outbox_checked': False})
        return
    before = json.loads(read(A / 'paid-roots-before.json', 2 << 20)[0])
    after = json.loads(read(A / 'paid-roots-after.json', 2 << 20)[0])
    need(before['catalog_sha256'] == after['catalog_sha256'] == CAT_SHA, 'snapshot catalog selection')
    policy(catalog, before['trees']); policy(catalog, after['trees'])
    changed = {}
    for name in ROOTS:
        old, new = before['trees'][name], after['trees'][name]
        need(old['path'] == new['path'] == str(ROOTS[name]) and old['identity'] == new['identity'] and old['directories'] == new['directories'], 'original root or directory identity changed')
        changes = sorted(p for p in set(old['files']) | set(new['files']) if old['files'].get(p) != new['files'].get(p))
        if name == 'owned':
            need(all(p in EXACT or any(p.startswith(prefix) for prefix in PREFIX) for p in changes), 'unlisted owned change')
        else:
            need(all(p.startswith('outbox/db/') for p in changes), 'attachment control changed')
        changed[name] = changes
    write('paid-roots-comparison.json', {'version': 1, 'physical_catalog_passed': True, 'catalog_sha256': CAT_SHA, 'immutable_baseline_paths': len(catalog['immutable_copy_paths']), 'allowed_changed_paths': changed, 'logical_outbox_checked': False, 'acceptance_passed': False, 'remaining_required': 'Exact full new qualified proofclient replay outbox_before/outbox_after and complete rows versus original attempt5 proof-replay-after; process/financial/infra evidence separate.'})


if __name__ == '__main__':
    main()
