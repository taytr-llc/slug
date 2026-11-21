"""Set80 final public daily all-module builds and static checks; separately qualified regressions."""
from pathlib import Path
import hashlib, importlib.util, json, os, re, sys

BASE = Path('/Users/taytrrip/slug')
ROOT, WALLET = BASE/'slug', BASE/'slugwallet'
LEGACY = ROOT/'desktop/pooloperator/testdata/late-winner-actual'
PROFILE = Path('/private/tmp/slug_set48_build_profile.py')
PINNED = {
    LEGACY/'run.py': 'b81af69c02a18b7f74ed96e08ae4dba8548e94a4958aeacb721c62dcbf8ba05e',
    LEGACY/'runner_support.py': '54bb3e7caf33e744ce76bf366dd23a4eff4d370d9246b85758089b57dd2439d7',
    PROFILE: 'b6033d66edb4d24a57a7fac4e59ddaeee6964f0943aca21fddc6f65c7f5e6ee4',
}
for path, expected in PINNED.items():
    if hashlib.sha256(path.read_bytes()).hexdigest() != expected:
        raise RuntimeError('selected capture/profile helper changed')
sys.dont_write_bytecode = True
sys.path.insert(0, str(LEGACY))
sys.path.insert(0, '/private/tmp')
import slug_set48_build_profile as bp
spec = importlib.util.spec_from_file_location('set80_closeout_capture', LEGACY/'run.py')
capture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(capture)
MODULES = (
    (ROOT, 'chain', ['.', 'address', 'btcec', 'btcutil', 'chaincfg', 'chainhash',
                     'psbt', 'txscript', 'v2transport', 'wire']),
    (WALLET, 'wallet', ['.', 'wallet/txauthor', 'wallet/txrules', 'wallet/txsizes',
                       'walletdb', 'wtxmgr']),
    (ROOT/'desktop', 'desktop', ['.']))



def required_tests(log, package, names):
    events = [json.loads(line) for line in log.read_text().splitlines() if line.startswith('{')]
    passed = {e.get('Test') for e in events if e.get('Action') == 'pass' and e.get('Package') == package}
    package_pass = any(e.get('Action') == 'pass' and e.get('Package') == package and not e.get('Test') for e in events)
    if not package_pass or not set(names).issubset(passed) or any(e.get('Action') == 'fail' for e in events):
        raise RuntimeError('required test missing, skipped or failed: '+package+' '+str(set(names)-passed))


def main():
    if len(sys.argv) != 2 or not re.fullmatch(r'[a-z0-9-]+-attempt[1-9][0-9]*', sys.argv[1]):
        raise SystemExit('fresh named attempt required')
    out = BASE/'artifacts/set80'/sys.argv[1]
    out.mkdir(mode=0o700)
    commands, dependencies = [], set()
    report = dict(passed=False, scope='exact-issuance-final17-module-builds-and-static-checks',
                  commands=commands, sources_unchanged=False, profile_unchanged=False,
                  dependency_capture_complete=False)
    before, profile, env = None, None, None

    def run(label, cwd, argv, seconds=600, log_cap=16 << 20):
        print(json.dumps(dict(starting=label)), flush=True)
        log = out/(label+'.log')
        row = capture.capture(argv, cwd, env, log, seconds, log_cap=log_cap)
        row['label'] = label
        commands.append(row)
        if not capture.okay(row):
            raise RuntimeError('failed '+label)
        print(json.dumps(dict(passed=label, seconds=row['seconds'])), flush=True)
        return log

    def inputs(label):
        # The concrete Go closure supplies native and embedded files, including
        # external modules. This inventory also binds unselected tests/testdata
        # and notices newly added local source files on final comparison.
        files = {Path(__file__).resolve(), *PINNED, *dependencies}
        for root, name in ((ROOT, 'chain'), (WALLET, 'wallet')):
            log = run(label+'-'+name, root,
                      ['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard'], 10)
            for item in log.read_bytes().split(b'\0'):
                if not item:
                    continue
                p = root/os.fsdecode(item)
                if p.is_file() and (p.suffix in ('.go', '.mod', '.sum', '.work', '.c', '.h',
                                                '.m', '.mm', '.cc', '.cpp', '.s', '.S', '.syso') or
                                   'testdata' in p.parts or p.name in ('build-profile.json', 'candidate-go.sh')):
                    files.add(p)
        return {str(p): capture.support.identity(p) for p in sorted(files)}

    try:
        profile = bp.binding()
        capture.support.write(out/'profile.json', profile)
        env = bp.environment(GOCACHE='/Users/taytrrip/slug/build-cache/go-build-native',
            GOMODCACHE='/Users/taytrrip/slug/build-cache/go-mod', GOPROXY='off', GOSUMDB='off',
            GOMAXPROCS='2', CGO_ENABLED='1', GOWORK=str(ROOT/'desktop/go.work'))
        before = inputs('inventory-before')
        capture.support.write(out/'sources-inventory-before.json', before)
        # Exact normal test/build dependencies for all17 modules are selected
        # before any build. Includes Go test variants, native code and embeds.
        for root, name, modules in MODULES:
            for i, module in enumerate(modules):
                log = run('deps-'+name+'-'+str(i), root/module,
                    ['go', 'list', '-deps', '-test', '-json', '-mod=readonly', '-tags=', './...'],
                    120, 32 << 20)
                selected = capture.dependency_files(log)
                dependencies.update(selected)
                for p in sorted(selected):
                    identity = capture.support.identity(p)
                    if str(p) in before and before[str(p)] != identity:
                        raise RuntimeError('source changed during dependency capture')
                    before[str(p)] = identity
        report['dependency_capture_complete'] = True
        capture.support.write(out/'sources.json', before)
        for root, name, modules in MODULES:
            for i, module in enumerate(modules):
                run('build-'+name+'-'+str(i), root/module,
                    bp.command('build', '-p=1', '-mod=readonly', '-tags=', './...'))
        run('accounting-vet', ROOT, ['go', 'vet', '-p=1', '-tags=',
            '-gcflags=all=-d=ssa/generic_cse/off,ssa/lowered_cse/off',
            './internal/mininglease', './internal/poolcapacity', './poolshare', './poolround', './poolsettle', './poolschedule', './poolservice', './poolingress', './poolminer', './prospectworker'])
        run('pooloperator-vet', ROOT/'desktop', ['go', 'vet', '-p=1', '-tags=',
            '-gcflags=all=-d=ssa/generic_cse/off,ssa/lowered_cse/off', './pooloperator', './pilot', './accountpilot', './cmd/slugpilot'])
        for root, name in ((ROOT, 'chain'), (WALLET, 'wallet')):
            log = run('gofmt-'+name, root, ['gofmt', '-l', '.'])
            if log.read_bytes().strip():
                raise RuntimeError('gofmt is not clean')
            run('diff-'+name, root, ['git', 'diff', '--check'])
        report['passed'] = True
    except BaseException as exc:
        report['error'] = type(exc).__name__+': '+str(exc)
    finally:
        # Final evidence is independent of the original command outcome.
        if before is not None:
            try:
                capture.support.write(out/'sources-final-bound.json', before)
                after = inputs('inventory-after')
                capture.support.write(out/'sources-after.json', after)
                report['sources_unchanged'] = after == before
            except BaseException as exc:
                report['source_error'] = type(exc).__name__+': '+str(exc)
        try:
            if profile is None:
                raise RuntimeError('profile was not bound')
            bp.same(profile)
            for path, expected in PINNED.items():
                if hashlib.sha256(path.read_bytes()).hexdigest() != expected:
                    raise RuntimeError('selected capture/profile helper changed')
            report['profile_unchanged'] = True
        except BaseException as exc:
            report['profile_error'] = type(exc).__name__+': '+str(exc)
        report['source_count'] = len(before or {})
        report['clean_join'] = bool(commands) and all(
            c.get('log_joined') is True and not c.get('log_errors') and
            (c.get('cleanup') or {}).get('group_empty') is True and
            (c.get('cleanup') or {}).get('leftover_after_exit') is False and
            not (c.get('cleanup') or {}).get('signals') and
            not (c.get('cleanup') or {}).get('errors') for c in commands)
        report['passed'] = (report['passed'] and report['dependency_capture_complete'] and
                            report['sources_unchanged'] and
                            report['profile_unchanged'] and report['clean_join'] and
                            all(capture.okay(c) for c in commands))
        capture.support.write(out/'report.json', report)
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
