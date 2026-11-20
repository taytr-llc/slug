#!/usr/bin/env python3
"""Run only the frozen public producer into a fresh exclusive directory."""
import hashlib
import importlib.util
import json
from pathlib import Path
import resource
import signal
import sys
import time

ROOT = Path(__file__).resolve().parent
LIMIT = 4 * 1024 * 1024


def identity(path):
    assert path.is_file() and not path.is_symlink() and path.stat().st_size <= LIMIT
    b = path.read_bytes()
    return dict(bytes=len(b), sha256=hashlib.sha256(b).hexdigest())


def main():
    assert not sys.flags.optimize and len(sys.argv) == 2
    assert sys.version_info[:2] in [(3, 14), (3, 9)]
    resource.setrlimit(resource.RLIMIT_CPU, (30, 30))
    resource.setrlimit(resource.RLIMIT_FSIZE, (LIMIT, LIMIT))
    def expire(_signum, _frame):
        raise TimeoutError('bounded public producer exceeded 60 seconds')
    signal.signal(signal.SIGALRM, expire)
    signal.alarm(60)
    lock = json.loads((ROOT / 'execution-lock.json').read_bytes())
    assert lock['version'] == 1 and lock['approved_by'] == 'root' and len(lock['files']) <= 16
    def verify():
        for row in lock['files']:
            path = ROOT / row['path']
            assert path.resolve().is_relative_to(ROOT)
            assert identity(path) == dict(bytes=row['bytes'], sha256=row['sha256']), row['path']
    verify()
    out = Path(sys.argv[1])
    assert out.is_absolute() and out.parent.is_dir() and not out.exists()
    out.mkdir(mode=0o700)
    began = time.monotonic()
    report = dict(version=1, status='failed', python=sys.version.split()[0], source_unchanged=False)
    try:
        spec = importlib.util.spec_from_file_location('remote_protocol_model', ROOT / 'model.py')
        model = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(model)
        result = model.build(ROOT / 'inputs')
        b = json.dumps(result, ensure_ascii=True, indent=2).encode('ascii') + b'\n'
        assert len(b) <= LIMIT
        with (out / 'reference-v1.json').open('xb') as f:
            f.write(b)
        report.update(status='pass', corpus=dict(bytes=len(b), sha256=hashlib.sha256(b).hexdigest()),
                      counts={k: len(v) for k, v in result.items() if isinstance(v, list)})
    except Exception as e:
        report['failure'] = dict(type=type(e).__name__, reason=str(e)[:500])
    finally:
        try:
            verify()
            report['source_unchanged'] = True
        except Exception:
            report['status'] = 'failed'
            report['failure'] = dict(type='SourceChanged', reason='locked source/input mismatch')
        report['elapsed_seconds'] = round(time.monotonic() - began, 6)
        report['execution_lock'] = identity(ROOT / 'execution-lock.json')
        (out / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
        signal.alarm(0)
    print(json.dumps(report, separators=(',', ':')))
    return 0 if report['status'] == 'pass' else 1


if __name__ == '__main__':
    sys.exit(main())
