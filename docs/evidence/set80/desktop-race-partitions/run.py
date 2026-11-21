#!/usr/bin/env python3
"""Root-owned complete desktop race successor; four fixed pilot partitions.

Source-only authoring. Every selected top-level Test and Fuzz seed target is
covered exactly once. No timeout increase, exclusions, retries or new skips.
Original passing accountpilot/slugpilot results are separately source-bound;
the failed original aggregate remains failed.
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
PLAN = HERE/'partition-plan.json'
PLAN_SHA = '508589c0f545e558a1637a70c558f462344a9d00b7673d17ecafe516f1a7b91d'
PINNED = {
    LEGACY/'run.py': 'b81af69c02a18b7f74ed96e08ae4dba8548e94a4958aeacb721c62dcbf8ba05e',
    LEGACY/'runner_support.py': '54bb3e7caf33e744ce76bf366dd23a4eff4d370d9246b85758089b57dd2439d7',
    Path('/private/tmp/slug_set48_build_profile.py'): 'b6033d66edb4d24a57a7fac4e59ddaeee6964f0943aca21fddc6f65c7f5e6ee4',
}
TEST_DECLARATION = re.compile(r'\bfunc\s+((?:Test|Fuzz)[A-Za-z0-9_]+)\s*\(\s*\w+\s+\*testing\.(?:T|F)\s*\)')
GO_STRING = re.compile(r'"(?:[^"\\]|\\.)*"')
PILOT = 'github.com/taytr-llc/slug/desktop/pilot'

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



def test_log(path, packages, expected, allowed):
    require(path.stat().st_size <= 64 << 20, 'bounded original test log')
    terminal, outputs, package_pass, failures = {}, {}, set(), []
    for line in path.read_text().splitlines():
        if not line.startswith('{'):
            continue
        event = json.loads(line)
        package, test, action = event.get('Package', ''), event.get('Test'), event.get('Action')
        if package not in packages:
            continue
        if action in ('fail', 'build-fail'):
            failures.append(dict(package=package, test=test, action=action))
        if test:
            key = (package, test)
            require((package, test.split('/')[0]) in expected, 'unselected test/fuzz event')
            if action == 'output':
                outputs[key] = (outputs.get(key, '')+event.get('Output', ''))[-8192:]
            if action in ('pass', 'fail', 'skip'):
                require(key not in terminal, 'duplicate terminal event')
                terminal[key] = action
        elif action == 'pass':
            require(package not in package_pass, 'duplicate per-command package PASS')
            package_pass.add(package)
    reported_skips, unknown_skips = [], []
    for key, action in sorted(terminal.items()):
        if action != 'skip':
            continue
        rule = allowed.get(key)
        admitted = bool(rule) and rule.get('platform', sys.platform) == sys.platform
        if admitted:
            admitted = bool(re.search(r':\d+:\s+'+re.escape(rule['reason'])+r'\s*$', outputs.get(key, ''), re.M))
        item = dict(package=key[0], test=key[1], action='skip', admitted=admitted)
        if admitted:
            item.update(category=rule['category'], reason=rule['reason'])
        else:
            unknown_skips.append(item)
        reported_skips.append(item)
    missing = [dict(package=p, test=t) for p, t in sorted(expected) if terminal.get((p, t)) not in ('pass', 'skip')]
    complete = package_pass == packages and not failures and not missing and not unknown_skips
    return dict(terminal=terminal, skips=reported_skips, unknown_skips=unknown_skips,
                missing=missing, failures=failures, package_pass=package_pass, complete=complete)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--attempt', required=True)
    args = parser.parse_args()
    require(re.fullmatch(r'[a-z][a-z0-9-]*-attempt[1-9][0-9]*', args.attempt), 'fresh attempt name')
    require(sys.platform == 'darwin', 'exact captured Darwin inventory')
    require(hashlib.sha256(PLAN.read_bytes()).hexdigest() == PLAN_SHA, 'frozen partition selection')
    plan = json.loads(PLAN.read_bytes())
    require(plan['version'] == 1 and len(plan['commands']) == 4, 'four exact pilot partitions')
    for path, digest in PINNED.items():
        require(hashlib.sha256(path.read_bytes()).hexdigest() == digest, 'capture/profile source changed')
    sys.dont_write_bytecode = True
    sys.path[:0] = [str(LEGACY), '/private/tmp']
    import slug_set48_build_profile as bp
    spec = importlib.util.spec_from_file_location('set80_partition_capture', LEGACY/'run.py')
    cap = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(cap)
    expected = {(x['package'], x['test']) for x in plan['expected_tests']}
    require(len(expected) == len(plan['expected_tests']) == 595, 'complete frozen race inventory')
    prior_packages = set(plan['prior_packages'])
    expected_packages = {PILOT} | prior_packages
    package_roots = [ROOT/'desktop/pilot', ROOT/'desktop/accountpilot', ROOT/'desktop/cmd/slugpilot']
    cwd = ROOT/'desktop'
    packages = ['./pilot', './accountpilot', './cmd/slugpilot']
    partition_union = set()
    for i, selection in enumerate(plan['commands'], 1):
        tests = selection['tests']
        require(selection['label'] == 'race-pilot-'+str(i) and selection['package'] == PILOT and selection['argument'] == './pilot', 'exact original pilot partition')
        require(tests == sorted(set(tests)) and all(re.fullmatch(r'(Test|Fuzz)[A-Za-z0-9_]+', t) for t in tests), 'whole top-level names only')
        require(selection['run_pattern'] == '^('+'|'.join(tests)+')$', 'exact anchored selection, all subtests retained')
        require(selection['go_timeout_seconds'] == 600 and selection['capture_seconds'] == 720, 'unchanged per-process budget')
        current = {(PILOT, t) for t in tests}
        require(not current & partition_union, 'no partition overlap')
        partition_union |= current
    require(partition_union == {x for x in expected if x[0] == PILOT} and len(partition_union) == 442, 'complete440 Test plus2 Fuzz union')
    out = BASE/'artifacts/set80'/args.attempt
    out.mkdir(mode=0o700, exist_ok=False)
    report = dict(version=1, scope='complete desktop race composite: four pilot partitions plus exact prior passing packages',
                  mode='race', packages=sorted(expected_packages), passed=False, commands=[],
                  package_timeout_seconds=600, aggregate_capture_seconds=2880, dependency_capture_seconds=120,
                  test_exclusions=[], partition_count=4, source_only_estimates_not_guarantees=True,
                  sources_unchanged=False, profile_unchanged=False, dependency_capture_complete=False,
                  prior_sources_unchanged=False, prior_aggregate_still_failed=True, actual_acceptance_claim=False)
    clean_base = {k: v for k, v in os.environ.items() if not k.startswith('SLUG_')}
    env = bp.environment(base=clean_base, GOCACHE=str(BASE/'build-cache/go-build-native'),
        GOMODCACHE=str(BASE/'build-cache/go-mod'), GOPROXY='off', GOSUMDB='off',
        GOMAXPROCS='2', CGO_ENABLED='1', GOWORK=str(ROOT/'desktop/go.work'))
    report['environment'] = dict(tags='', cgo='1', gomaxprocs='2', go_parallelism=1,
        go_work=env['GOWORK'], actual_optins='all inherited SLUG_* removed; no actual tags selected')
    fixed = {Path(__file__).resolve(), PLAN, HERE/'regression-skips.json', *PINNED,
             ROOT/'scripts/candidate-go.sh', ROOT/'build-profile.json'}
    fixed.update(Path(p) for p in plan['evidence_inputs'])
    for path in (ROOT/'go.mod', ROOT/'go.sum', ROOT/'desktop/go.mod', ROOT/'desktop/go.sum',
                 ROOT/'desktop/go.work', ROOT/'desktop/go.work.sum'):
        if path.is_file(): fixed.add(path)
    dependencies = set()
    before = profile = preliminary = prior_before = prior_report = None
    suite_ok = False
    terminal, skipped, failures, unknown_skips, completed_partitions = {}, [], [], [], []

    def inventory():
        files = fixed | dependencies
        for directory in set(package_roots) | fixture_roots(package_roots):
            files.update(p for p in directory.rglob('*') if p.is_file())
        return {str(p): cap.support.identity(p) for p in sorted(files)}

    def run(label, argv, seconds, log_cap):
        print(json.dumps(dict(starting=label, mode='race')), flush=True)
        path = out/(label+'.log')
        row = cap.capture(argv, cwd, env, path, seconds, log_cap=log_cap)
        row['label'] = label
        report['commands'].append(row)
        return path, cap.okay(row)

    def merge(result):
        require(not set(terminal) & set(result['terminal']), 'each top-level family executed exactly once')
        terminal.update(result['terminal'])
        skipped.extend(result['skips']); failures.extend(result['failures']); unknown_skips.extend(result['unknown_skips'])

    try:
        for path, identity in plan['evidence_inputs'].items():
            require(cap.support.identity(Path(path)) == identity, 'original timing/failure evidence changed')
        for path, identity in plan['compiled_test_sources'].items():
            require(cap.support.identity(Path(path)) == identity, 'selected test source changed since inventory')
        rules = json.loads((HERE/'regression-skips.json').read_bytes())['skips']
        allowed = {(r['package'], r['test']): r for r in rules}
        require(len(allowed) == len(rules), 'duplicate exact skip rule')
        prior = Path(plan['prior_attempt'])
        prior_report = json.loads((prior/'report.json').read_bytes())
        require(prior_report.get('passed') is False and all(prior_report.get(k) is True for k in ('sources_unchanged', 'profile_unchanged', 'clean_join', 'dependency_capture_complete')), 'original failed aggregate and clean original source evidence')
        require(set(prior_report['passing_packages']) == prior_packages, 'only original complete passing packages reused')
        require(prior_report['failed_tests'] == [dict(action='fail', package=PILOT, test=None)], 'original failure limited to pilot package')
        require(prior_report['environment'] == report['environment'] and not prior_report['test_exclusions'], 'same complete profile/environment selection')
        prior_before = json.loads((prior/'sources-before.json').read_bytes())
        require(prior_before == json.loads((prior/'sources-after.json').read_bytes()), 'original source inventory unchanged')
        require(all(cap.support.identity(Path(p)) == v for p, v in prior_before.items()), 'current exact source still matches prior passing packages')
        fixed.update(Path(p) for p in prior_before)
        profile = bp.binding()
        require(profile == json.loads((prior/'profile.json').read_bytes()) == json.loads((prior/'profile-after.json').read_bytes()), 'same prior compiled profile')
        prior_expected = {x for x in expected if x[0] in prior_packages}
        old = test_log(prior/'race.log', prior_packages, prior_expected, allowed)
        require(old['complete'], 'prior complete package/test/skip evidence')
        merge(old)
        report['prior_package_evidence'] = dict(attempt=str(prior), packages=sorted(prior_packages),
            top_level_pass=sum(a=='pass' and '/' not in t for (p,t),a in old['terminal'].items()),
            top_level_skip=sum(a=='skip' and '/' not in t for (p,t),a in old['terminal'].items()),
            source_files=len(prior_before), report_identity=cap.support.identity(prior/'report.json'),
            log_identity=cap.support.identity(prior/'race.log'), aggregate_passed=False)
        cap.support.write(out/'profile.json', profile)
        preliminary = inventory()
        cap.support.write(out/'sources-inventory-before.json', preliminary)
        deps = ['go', 'list', '-deps', '-test', '-json', '-p=1', '-mod=readonly', '-tags=', '-race']
        log, ok = run('deps', deps+packages, 120, 32 << 20)
        require(ok, 'dependency capture failed')
        dependencies.update(cap.dependency_files(log))
        current_tests, seen_packages = set(), set()
        for row in json_stream(log):
            name = row.get('ImportPath')
            if name not in expected_packages: continue
            seen_packages.add(name)
            for filename in row.get('TestGoFiles', [])+row.get('XTestGoFiles', []):
                source = Path(row['Dir'])/filename
                content = source.read_text()
                require(not re.search(r'\bfunc\s+Example[A-Za-z0-9_]*\s*\(', content), 'unaccounted example requires a new explicit inventory')
                current_tests.update((name, n) for n in TEST_DECLARATION.findall(content))
        require(seen_packages == expected_packages and current_tests == expected, 'exact full currently compiled Test/Fuzz inventory')
        before = inventory()
        require(all(before.get(p)==v for p,v in preliminary.items()), 'source changed during dependency discovery')
        report['dependency_capture_complete'] = True
        cap.support.write(out/'sources-before.json', before)
        cap.support.write(out/'selected-tests.json', plan['expected_tests'])
        cap.support.write(out/'selected-partitions.json', plan['commands'])
        for selection in plan['commands']:
            command = ['sh', str(ROOT/'scripts/candidate-go.sh'), 'test', '-count=1', '-p=1', '-timeout=600s', '-json', '-tags=', '-race', '-run', selection['run_pattern'], './pilot']
            log, ok = run(selection['label'], command, 720, 64 << 20)
            want = {(PILOT, t) for t in selection['tests']}
            result = test_log(log, {PILOT}, want, allowed)
            merge(result)
            report.setdefault('partition_results', []).append(dict(label=selection['label'], passed=ok and result['complete'], expected_top_level=len(want),
                missing=result['missing'], failures=result['failures'], unknown_skips=result['unknown_skips']))
            if not ok or not result['complete']:
                break  # no retry, and no failed process can be hidden by a later one
            completed_partitions.append(selection['label'])
        actual_skips = {(r['package'],r['test']) for r in skipped}
        expected_skips = {(r['package'],r['test']) for r in plan['expected_natural_skips']}
        require(actual_skips == expected_skips, 'exact unchanged natural skip inventory')
        require(len(completed_partitions)==4 and not failures and not unknown_skips
                and all(terminal.get(key) in ('pass','skip') for key in expected), 'complete disjoint desktop race coverage')
        suite_ok = True
    except BaseException as exc:
        report['error'] = type(exc).__name__+': '+str(exc)
    finally:
        top = [dict(package=p,test=t,action=a) for (p,t),a in sorted(terminal.items()) if '/' not in t]
        missing = [dict(package=p,test=t) for p,t in sorted(expected) if terminal.get((p,t)) not in ('pass','skip')]
        report.update(top_level_tests=top, passing_tests=[p+'/'+t for (p,t),a in sorted(terminal.items()) if a=='pass'],
            skipped_tests=skipped, unexpected_skips=unknown_skips, failed_tests=failures, missing_tests=missing,
            top_level_pass_count=sum(x['action']=='pass' for x in top), top_level_skip_count=sum(x['action']=='skip' for x in top),
            completed_partitions=completed_partitions, passing_packages=sorted(expected_packages) if suite_ok else [])
        cap.support.write(out/'test-inventory.json', dict(top_level=top, skips=skipped, failures=failures, missing=missing, completed_partitions=completed_partitions))
        try:
            after = inventory()
            cap.support.write(out/'sources-after.json', after)
            report['sources_unchanged'] = preliminary is not None and (before if before is not None else preliminary) == after
            report['prior_sources_unchanged'] = prior_before is not None and all(cap.support.identity(Path(p)) == v for p,v in prior_before.items())
        except BaseException as exc:
            report['source_error'] = type(exc).__name__+': '+str(exc)
        try:
            final_profile = bp.binding()
            cap.support.write(out/'profile-after.json', final_profile)
            report['profile_unchanged'] = profile is not None and final_profile == profile
        except BaseException as exc:
            report['profile_error'] = type(exc).__name__+': '+str(exc)
        report['clean_join'] = bool(report['commands']) and prior_report is not None and prior_report.get('clean_join') is True and all(
            r.get('log_joined') is True and not r.get('capture_errors') and not r.get('log_errors')
            and (r.get('cleanup') or {}).get('group_empty') is True
            and (r.get('cleanup') or {}).get('leftover_after_exit') is False
            and not (r.get('cleanup') or {}).get('signals') and not (r.get('cleanup') or {}).get('errors')
            for r in report['commands'])
        report['passed'] = (suite_ok and report['dependency_capture_complete'] and report['sources_unchanged']
            and report['prior_sources_unchanged'] and report['profile_unchanged'] and report['clean_join'])
        cap.support.write(out/'report.json', report)
    print(json.dumps({k:v for k,v in report.items() if k not in ('commands','passing_tests','top_level_tests','skipped_tests')}), flush=True)
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
