#!/usr/bin/env python3
"""Root-owned Set80 full affected suites. Drafted source-only; each run is exclusive.

No -run filter, no actual tags. The one named expiry test is independently
qualified in both modes; all other selected tests must finish PASS or an exact
reported natural skip. A skip never supplies a PASS.
"""
from pathlib import Path
import argparse
import hashlib
import importlib.util
import json
import os
import re
import sys

BASE = Path('/Users/taytrrip/slug')
ROOT, WALLET = BASE/'slug', BASE/'slugwallet'
HERE = Path(__file__).resolve().parent
LEGACY = ROOT/'desktop/pooloperator/testdata/late-winner-actual'
PINNED = {
    LEGACY/'run.py': 'b81af69c02a18b7f74ed96e08ae4dba8548e94a4958aeacb721c62dcbf8ba05e',
    LEGACY/'runner_support.py': '54bb3e7caf33e744ce76bf366dd23a4eff4d370d9246b85758089b57dd2439d7',
    Path('/private/tmp/slug_set48_build_profile.py'): 'b6033d66edb4d24a57a7fac4e59ddaeee6964f0943aca21fddc6f65c7f5e6ee4',
}
SLICES = {
    'core': ['registry', 'poolshare'],
    'transport': ['poolservice', 'poolingress'],
    'worker': ['prospectworker'],
    'operator': ['desktop/pooloperator'],
    'desktop': ['desktop/pilot', 'desktop/accountpilot', 'desktop/cmd/slugpilot'],
    'financial': ['poolsettle', 'poolschedule'],
}
EXCLUDED = 'TestRecoverableDailyFullIntakeExpiredWinnerPreservesPendingPayout'
EXCLUDED_PACKAGE = 'github.com/taytr-llc/slug/poolsettle'
PRIOR = {
    'financial-normal-attempt2': '1f7d6a9ec1c28e8e0fd18ac49c2a5218e90449f324dda72cec0dfcfcaf6a5160',
    'financial-race-attempt1': '0ba9a9384ad5644454cacc496e68ea449bf72bb7f2705cc90c5c15ab0f98475b',
}
TEST_DECLARATION = re.compile(r'\bfunc\s+(Test[A-Za-z0-9_]+)\s*\(\s*\w+\s+\*testing\.T\s*\)')
GO_STRING = re.compile(r'"(?:[^"\\]|\\.)*"')


def require(value, message):
    if not value:
        raise RuntimeError(message)


def json_stream(path):
    raw = path.read_bytes()
    require(len(raw) <= 32 << 20, 'dependency output bound')
    text, decoder, offset, items = raw.decode(), json.JSONDecoder(), 0, []
    while offset < len(text):
        while offset < len(text) and text[offset].isspace():
            offset += 1
        if offset == len(text):
            break
        item, offset = decoder.raw_decode(text, offset)
        require(not item.get('Error') and not item.get('DepsErrors'), 'dependency error')
        items.append(item)
        require(len(items) <= 4096, 'dependency count bound')
    require(items, 'empty dependency inventory')
    return items


def fixture_roots(package_roots):
    """Bind literal public fixture roots used by these suites, not all repositories.

    The selected package trees include their own testdata. Cross-package literals
    bind the containing testdata tree, including dynamically appended filenames.
    Dependency files/native files/embeds are separately supplied by go list.
    """
    found = set()
    for directory in package_roots:
        for source in directory.glob('*_test.go'):
            for match in GO_STRING.finditer(source.read_text()):
                try:
                    value = json.loads(match.group())
                except ValueError:
                    continue
                parts = value.split('/')
                if 'testdata' not in parts:
                    continue
                relative = Path(*parts[:parts.index('testdata')+1])
                candidate = source.parent/relative
                require(not relative.is_absolute(), 'absolute fixture literal refused')
                candidate = candidate.resolve()
                require(candidate == ROOT/'testdata' or ROOT in candidate.parents or WALLET in candidate.parents,
                        'fixture outside original public checkouts')
                require(candidate.is_dir(), 'missing public fixture directory: '+str(candidate))
                found.add(candidate)
    return found


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--attempt', required=True)
    parser.add_argument('--slice', choices=tuple(SLICES), required=True)
    parser.add_argument('--mode', choices=('normal', 'race'), required=True)
    args = parser.parse_args()
    require(re.fullmatch(r'[a-z][a-z0-9-]*-attempt[1-9][0-9]*', args.attempt), 'fresh attempt name')
    require(sys.platform in ('darwin', 'linux'), 'selected local platform only')
    for path, digest in PINNED.items():
        require(hashlib.sha256(path.read_bytes()).hexdigest() == digest, 'capture/profile source changed')
    sys.dont_write_bytecode = True
    sys.path[:0] = [str(LEGACY), '/private/tmp']
    import slug_set48_build_profile as bp
    spec = importlib.util.spec_from_file_location('set80_regression_capture', LEGACY/'run.py')
    cap = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(cap)

    selected = SLICES[args.slice]
    package_roots = [ROOT/p for p in selected]
    cwd = ROOT/'desktop' if args.slice in ('operator', 'desktop') else ROOT
    packages = ['./'+str(p.relative_to(cwd)) for p in package_roots]
    expected_packages = {'github.com/taytr-llc/slug/'+p for p in selected}
    # The product per-call deadlines are unchanged. Go's 600s budget is per
    # package; -p=1 serializes packages, so capture permits their sum + 120s build.
    test_seconds = 600*len(packages)+120
    out = BASE/'artifacts/set80'/args.attempt
    out.mkdir(mode=0o700, exist_ok=False)
    report = dict(version=1, scope='full affected legacy and Set80 regression',
                  slice=args.slice, mode=args.mode, packages=sorted(expected_packages),
                  package_timeout_seconds=600, aggregate_capture_seconds=test_seconds,
                  dependency_capture_seconds=120, passed=False, commands=[],
                  sources_unchanged=False, profile_unchanged=False,
                  dependency_capture_complete=False, test_exclusions=[],
                  actual_acceptance_claim=False)
    # Do not accidentally opt into external nodes/owned fixtures from the shell.
    # Tests may still set their own local child-control environment themselves.
    clean_base = {k: v for k, v in os.environ.items() if not k.startswith('SLUG_')}
    env = bp.environment(base=clean_base,
        GOCACHE=str(BASE/'build-cache/go-build-native') if args.slice=='desktop' else '/private/tmp/slug-go-build',
        GOMODCACHE=str(BASE/'build-cache/go-mod'), GOPROXY='off', GOSUMDB='off',
        GOMAXPROCS='2', CGO_ENABLED='1', GOWORK=str(ROOT/'desktop/go.work'))
    report['environment'] = dict(tags='', cgo='1', gomaxprocs='2', go_parallelism=1,
        go_work=env['GOWORK'], actual_optins='all inherited SLUG_* removed; no actual tags selected')
    fixed = {Path(__file__).resolve(), HERE/'regression-skips.json', *PINNED,
             ROOT/'scripts/candidate-go.sh', ROOT/'build-profile.json'}
    for path in (ROOT/'go.mod', ROOT/'go.sum', ROOT/'desktop/go.mod', ROOT/'desktop/go.sum',
                 ROOT/'desktop/go.work', ROOT/'desktop/go.work.sum'):
        if path.is_file():
            fixed.add(path)
    dependencies = set()
    before = profile = preliminary = None
    suite_ok = False

    def inventory():
        files = fixed | dependencies
        # Recompute both trees and fixture roots at finalization to detect adds
        # and removals, not merely changes to files found at the first snapshot.
        for directory in set(package_roots) | fixture_roots(package_roots):
            files.update(p for p in directory.rglob('*') if p.is_file())
        return {str(p): cap.support.identity(p) for p in sorted(files)}

    def run(label, argv, seconds, log_cap):
        print(json.dumps(dict(starting=label, slice=args.slice, mode=args.mode)), flush=True)
        path = out/(label+'.log')
        row = cap.capture(argv, cwd, env, path, seconds, log_cap=log_cap)
        row['label'] = label
        report['commands'].append(row)
        return path, cap.okay(row)

    try:
        skips = json.loads((HERE/'regression-skips.json').read_bytes())
        allowed = {(r['package'], r['test']): r for r in skips['skips']}
        require(len(allowed) == len(skips['skips']), 'duplicate skip admission')
        if args.slice == 'financial':
            evidence = []
            for attempt, digest in PRIOR.items():
                path = BASE/'artifacts/set80'/attempt/'report.json'
                require(hashlib.sha256(path.read_bytes()).hexdigest() == digest, 'expiry evidence changed')
                old = json.loads(path.read_bytes())
                require(all(old.get(k) is True for k in ('passed','sources_unchanged','profile_unchanged','clean_join')),
                        'expiry qualification incomplete')
                require(EXCLUDED_PACKAGE+'/'+EXCLUDED in old['passing_tests'], 'expiry test not passed')
                fixed.add(path)
                evidence.append(dict(path=str(path), **cap.support.identity(path)))
            report['test_exclusions'] = [dict(package=EXCLUDED_PACKAGE, test=EXCLUDED,
                reason='original real 300s expiry separately passed normal and race; no other test excluded',
                evidence=evidence)]
        profile = bp.binding()
        cap.support.write(out/'profile.json', profile)
        preliminary = inventory()
        cap.support.write(out/'sources-inventory-before.json', preliminary)
        deps_command = ['go','list','-deps','-test','-json','-p=1','-mod=readonly','-tags=']
        if args.mode == 'race':
            deps_command.append('-race')
        log, ok = run('deps', deps_command+packages, 120, 32 << 20)
        require(ok, 'dependency capture failed')
        dependencies.update(cap.dependency_files(log))
        expected_tests = set()
        seen_packages = set()
        for row in json_stream(log):
            name = row.get('ImportPath')
            if name not in expected_packages:
                continue
            seen_packages.add(name)
            for filename in row.get('TestGoFiles', [])+row.get('XTestGoFiles', []):
                source = Path(row['Dir'])/filename
                expected_tests.update((name, n) for n in TEST_DECLARATION.findall(source.read_text()))
        require(seen_packages == expected_packages and all(any(p==n for p,t in expected_tests) for n in expected_packages),
                'selected test declaration inventory incomplete')
        if args.slice == 'financial':
            require((EXCLUDED_PACKAGE, EXCLUDED) in expected_tests, 'expected expiry declaration missing')
            expected_tests.remove((EXCLUDED_PACKAGE, EXCLUDED))
        before = inventory()
        require(all(before.get(p)==v for p,v in preliminary.items()), 'source changed during dependency discovery')
        report['dependency_capture_complete'] = True
        cap.support.write(out/'sources-before.json', before)
        cap.support.write(out/'selected-tests.json', [dict(package=p,test=t) for p,t in sorted(expected_tests)])
        command = ['sh',str(ROOT/'scripts/candidate-go.sh'),'test','-count=1','-p=1','-timeout=600s','-json','-tags=']
        if args.mode == 'race':
            command.append('-race')
        if args.slice == 'financial':
            command += ['-skip', '^'+EXCLUDED+'$']
        log, ok = run(args.mode, command+packages, test_seconds, 64 << 20)
        terminal, outputs, package_pass, failures = {}, {}, set(), []
        for line in log.read_text().splitlines():
            if not line.startswith('{'):
                continue
            event = json.loads(line)
            package, test, action = event.get('Package',''), event.get('Test'), event.get('Action')
            if action in ('fail','build-fail'):
                failures.append(dict(package=package,test=test,action=action))
            if test:
                key = (package,test)
                if action == 'output':
                    # Only admission-relevant reason text is retained in memory;
                    # the complete bounded original log remains the evidence.
                    outputs[key] = (outputs.get(key,'')+event.get('Output',''))[-8192:]
                if action in ('pass','fail','skip'):
                    require(key not in terminal, 'duplicate terminal test event')
                    terminal[key] = action
            elif action == 'pass':
                require(package not in package_pass, 'duplicate package PASS')
                package_pass.add(package)
        reported_skips, unknown_skips = [], []
        for key, action in sorted(terminal.items()):
            if action != 'skip':
                continue
            rule = allowed.get(key)
            admitted = bool(rule) and rule.get('platform', sys.platform)==sys.platform
            if admitted:
                admitted = bool(re.search(r':\d+:\s+'+re.escape(rule['reason'])+r'\s*$', outputs.get(key,''), re.M))
            item = dict(package=key[0],test=key[1],action='skip',admitted=admitted)
            if admitted:
                item.update(category=rule['category'],reason=rule['reason'])
            else:
                unknown_skips.append(item)
            reported_skips.append(item)
        top = [dict(package=p,test=t,action=a) for (p,t),a in sorted(terminal.items()) if '/' not in t]
        missing = [dict(package=p,test=t) for p,t in sorted(expected_tests) if terminal.get((p,t)) not in ('pass','skip')]
        report.update(passing_packages=sorted(package_pass), top_level_tests=top,
                      passing_tests=[p+'/'+t for (p,t),a in sorted(terminal.items()) if a=='pass'],
                      skipped_tests=reported_skips, unexpected_skips=unknown_skips,
                      failed_tests=failures, missing_tests=missing,
                      top_level_pass_count=sum(x['action']=='pass' for x in top),
                      top_level_skip_count=sum(x['action']=='skip' for x in top))
        cap.support.write(out/'test-inventory.json', dict(top_level=top,skips=reported_skips,
            failures=failures,missing=missing,package_pass=sorted(package_pass)))
        require(ok and package_pass==expected_packages and not failures and not missing and not unknown_skips,
                'test capture or complete PASS/skip inventory failed')
        require(all(any(p==name and '/' not in t and a=='pass' for (p,t),a in terminal.items()) for name in expected_packages),
                'package without a top-level PASS')
        suite_ok = True
    except BaseException as exc:
        report['error'] = type(exc).__name__+': '+str(exc)
    finally:
        # Final checks also run after dependency/test/skip failures. Unique files
        # preserve inherited O_EXCL evidence semantics.
        try:
            after = inventory()
            cap.support.write(out/'sources-after.json', after)
            report['sources_unchanged'] = (before if before is not None else preliminary) == after and preliminary is not None
        except BaseException as exc:
            report['source_error'] = type(exc).__name__+': '+str(exc)
        try:
            final_profile = bp.binding()
            cap.support.write(out/'profile-after.json', final_profile)
            report['profile_unchanged'] = profile is not None and final_profile==profile
        except BaseException as exc:
            report['profile_error'] = type(exc).__name__+': '+str(exc)
        report['clean_join'] = bool(report['commands']) and all(
            r.get('log_joined') is True and not r.get('capture_errors') and not r.get('log_errors')
            and (r.get('cleanup') or {}).get('group_empty') is True
            and (r.get('cleanup') or {}).get('leftover_after_exit') is False
            and not (r.get('cleanup') or {}).get('signals') and not (r.get('cleanup') or {}).get('errors')
            for r in report['commands'])
        report['passed'] = (suite_ok and report['dependency_capture_complete'] and
            report['sources_unchanged'] and report['profile_unchanged'] and report['clean_join'])
        cap.support.write(out/'report.json', report)
    print(json.dumps({k:v for k,v in report.items() if k not in ('commands','passing_tests','top_level_tests','skipped_tests')}), flush=True)
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
