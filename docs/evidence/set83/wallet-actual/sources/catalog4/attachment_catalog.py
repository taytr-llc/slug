"""Closed OLD5 attachment bytes only; no database open or private sibling access."""
import hashlib
import os
from pathlib import Path
import stat

OLD_ATTACHMENT = Path('/Users/taytrrip/slug/artifacts/set82/hosted-actual-attempt5/remote-attachment')
MARKERS = {'SLUG-REMOTE-POOL-INTENT.json', 'SLUG-REMOTE-POOL-READY.json', 'outbox/SLUG-POOL-WORKER.json'}
DIRECTORIES = {'outbox', 'outbox/db'}


def need(ok, why):
    if not ok:
        raise ValueError(why)


def identity(st):
    return dict(device=st.st_dev, inode=st.st_ino, uid=st.st_uid, gid=st.st_gid, mode=stat.S_IMODE(st.st_mode))


def file_fields(st):
    return (st.st_dev, st.st_ino, st.st_mode, st.st_nlink, st.st_uid, st.st_gid,
            st.st_size, st.st_mtime_ns, st.st_ctime_ns)


def snapshot_old(root):
    """Caller proves all old owners joined before either hash-only snapshot."""
    root = Path(root)
    need(root == OLD_ATTACHMENT and root == root.resolve(strict=True), 'only original nonlinked OLD5 attachment')
    original = root.lstat()
    need(stat.S_ISDIR(original.st_mode) and stat.S_IMODE(original.st_mode) == 0o700 and
         original.st_uid == os.geteuid(), 'original owned0700 attachment')
    files, directories, total = {}, {}, 0
    def walk_error(error):
        raise error
    for parent, names, leaves in os.walk(root, followlinks=False, onerror=walk_error):
        names.sort(); leaves.sort()
        for name in names + leaves:
            path = Path(parent) / name
            before = path.lstat()
            relative = str(path.relative_to(root))
            need(path == path.resolve(strict=True) and not stat.S_ISLNK(before.st_mode), 'unlinked original public path')
            if stat.S_ISDIR(before.st_mode):
                need(relative in DIRECTORIES and stat.S_IMODE(before.st_mode) == 0o700 and
                     before.st_uid == os.geteuid(), 'closed original attachment directory selection')
                directories[relative] = identity(before)
            else:
                need(relative in MARKERS or relative.startswith('outbox/db/'), 'public attachment namespace only')
                need(stat.S_ISREG(before.st_mode) and before.st_nlink == 1 and before.st_uid == os.geteuid() and
                     0 <= before.st_size <= 64 << 20, 'bounded regular original file')
                descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
                with os.fdopen(descriptor, 'rb') as stream:
                    need(file_fields(os.fstat(stream.fileno())) == file_fields(before), 'original public descriptor')
                    digest = hashlib.sha256()
                    size = 0
                    while True:
                        block = stream.read(65536)
                        if not block:
                            break
                        size += len(block)
                        need(size <= before.st_size, 'unchanged bounded public bytes')
                        digest.update(block)
                    need(file_fields(os.fstat(stream.fileno())) == file_fields(before), 'original descriptor unchanged')
                need(file_fields(path.lstat()) == file_fields(before) and size == before.st_size, 'original named file unchanged')
                files[relative] = dict(bytes=size, sha256=digest.hexdigest(), identity=identity(before),
                                       links=before.st_nlink, modified_ns=before.st_mtime_ns, changed_ns=before.st_ctime_ns)
                total += size
            need(len(files) + len(directories) <= 2000 and total <= 64 << 20, 'finite closed public inventory')
    need(set(directories) == DIRECTORIES and {p for p in files if not p.startswith('outbox/db/')} == MARKERS,
         'complete original marker/directory inventory')
    need(identity(root.lstat()) == identity(original), 'original attachment identity unchanged')
    for relative, expected in directories.items():
        need(identity((root / relative).lstat()) == expected, 'original public directory identity unchanged')
    return dict(version=1, scope='Closed original OLD5 attachment hash inventory only; no DB open',
                path=str(root), identity=identity(original), directories=directories, files=files,
                total_bytes=total, database_opened=False)


def compare_old(before, after):
    need(type(before) is dict and type(after) is dict and before.get('path') == after.get('path') == str(OLD_ATTACHMENT)
         and before.get('database_opened') is False and after.get('database_opened') is False,
         'same unopened original attachment selection')
    need(before == after, 'unopened OLD5 attachment changed')
    return dict(version=1, original_attachment_unchanged=True, database_opened=False,
                files=len(before['files']), total_bytes=before['total_bytes'])
