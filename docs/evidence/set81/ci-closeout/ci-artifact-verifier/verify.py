#!/usr/bin/env python3
"""Read only downloaded Set81 Linux evidence; write one fresh verification file.

No network, subprocess, Go, module import from the checkout, or report repair.
The pinned expectations were statically read from the selected gate, not inferred
from a downloaded report. Root alone chooses whether/when to execute this file.
"""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path, PurePosixPath
import re
import stat

EXPECTED_SHA256 = 'f059d2f4d2988b1c1febebbc1b503d4ce95b6b893335f4671e2fb1fb57cbc2e1'


def need(condition, label):
    if not condition:
        raise ValueError(label)


def unique_object(pairs):
    value = {}
    for key, item in pairs:
        need(key not in value, 'duplicate JSON key: ' + key)
        value[key] = item
    return value


def decode(raw):
    return json.loads(raw, object_pairs_hook=unique_object,
                      parse_constant=lambda value: (_ for _ in ()).throw(ValueError('nonfinite JSON: ' + value)))


def integer(value, minimum=0):
    return type(value) is int and value >= minimum


def finite(value, positive=False):
    return type(value) in (int, float) and math.isfinite(value) and (value > 0 if positive else value >= 0)


def canonical_absolute(path):
    path = Path(path)
    need(path.is_absolute() and str(path) == str(path.resolve(strict=True)), 'noncanonical or symlink path: ' + str(path))
    return path


def read_public(path, limit, keep=True):
    """Open each component without following links; bound and bracket the file."""
    path = Path(path)
    need(path.is_absolute() and '..' not in path.parts, 'absolute input required')
    fd = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    try:
        for part in path.parts[1:-1]:
            child = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = child
        source = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW, dir_fd=fd)
        try:
            before = os.fstat(source)
            need(stat.S_ISREG(before.st_mode) and before.st_nlink == 1 and before.st_size <= limit,
                 'regular bounded single-link public input: ' + str(path))
            digest = hashlib.sha256()
            parts = []
            size = 0
            while True:
                block = os.read(source, 1 << 20)
                if not block:
                    break
                size += len(block)
                need(size <= limit, 'input grew past bound: ' + str(path))
                digest.update(block)
                if keep:
                    parts.append(block)
            after = os.fstat(source)
            need((before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns, before.st_ctime_ns) ==
                 (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns) and
                 size == before.st_size, 'input changed during read: ' + str(path))
            return b''.join(parts) if keep else None, {'bytes': size, 'sha256': digest.hexdigest()}
        finally:
            os.close(source)
    finally:
        os.close(fd)


def relative(value):
    need(type(value) is str and value and not value.startswith('/'), 'relative public artifact path')
    p = PurePosixPath(value)
    need(str(p) == value and all(x not in ('', '.', '..') for x in p.parts), 'canonical public artifact path')
    return value


def verify(artifacts, ci_report):
    expected_path = canonical_absolute(Path(__file__).parent / 'expected.json')
    raw, pin = read_public(expected_path, 2 << 20)
    need(pin['sha256'] == EXPECTED_SHA256, 'exact statically derived expectations')
    expected = decode(raw)
    inputs = {str(expected_path): pin}
    limits = {str(expected_path): 2 << 20}

    def read(path, bound, keep=True):
        path = Path(path)
        raw, ident = read_public(path, bound, keep)
        previous = inputs.get(str(path))
        need(previous is None or previous == ident, 'input changed between reads: ' + str(path))
        inputs[str(path)] = ident
        limits[str(path)] = bound
        return raw, ident

    raw_report, report_ident = read(artifacts / 'report.json', 16 << 20)
    report = decode(raw_report)
    need(report['version'] == 1 and report['scope'] == 'focused_linux_operator', 'report version/scope')
    for name in ('passed', 'clean_join', 'source_unchanged', 'temporary_material_removed'):
        need(report.get(name) is True, 'required report success: ' + name)
    for name in ('provisioning_qualified', 'deployed_host_qualified', 'daily_actual_qualified'):
        need(report.get(name) is False, 'scope does not claim ' + name)
    need(not report.get('temporary_material_retained', False) and report['errors'] == [], 'no retained work/errors')
    need(report['profile'] == expected['profile'] and integer(report['uid'], 1), 'profile and non-root Linux runner')
    need(report['uname'][0] == 'Linux' and report['uname'][4] == 'x86_64', 'native Linux x86_64')
    need(finite(report['elapsed_seconds'], True), 'finite report duration')
    before_raw, before_ident = read(artifacts / 'sources-before.json', 16 << 20)
    after_raw, after_ident = read(artifacts / 'sources-after.json', 16 << 20)
    need(before_raw == after_raw, 'byte-identical source inventories')
    sources = decode(before_raw)
    need(set(sources) == {'chain', 'wallet'}, 'exact paired source inventories')
    inventories = {}
    for owner, head in (('chain', expected['head']), ('wallet', expected['wallet'])):
        item = sources[owner]
        need(item['head'] == head and item['status'] == '', 'exact clean ' + owner + ' head')
        rows = item['files']
        need(0 < len(rows) <= 20000, 'source inventory bound')
        names = [relative(row['path']) for row in rows]
        need(names == sorted(set(names)), 'unique ordered inventory paths')
        for row in rows:
            need(row['kind'] in ('file', 'symlink') and integer(row['size']) and
                 re.fullmatch('[0-9a-f]{64}', row['sha256']), 'source inventory scalar types')
        inventories[owner] = {row['path']: row for row in rows}
    for path, pin in expected['source_pins'].items():
        row = inventories['chain'][path]
        need(row['kind'] == 'file' and {'bytes': row['size'], 'sha256': row['sha256']} == pin, 'selected gate input: ' + path)
    for path, pin in expected['required_declaration_files'].items():
        prefix, selected = path.split('/', 1)
        row = inventories['chain' if prefix == 'slug' else 'wallet'][selected]
        need(row['kind'] == 'file' and {'bytes': row['size'], 'sha256': row['sha256']} == pin, 'required test source: ' + path)
    commands = report['commands']
    need([c['name'] for c in commands] == [c['name'] for c in expected['commands']], 'all 64 commands in exact order')
    chain = PurePosixPath(commands[0]['cwd'])
    need(chain.is_absolute() and '..' not in chain.parts and str(chain) == commands[0]['cwd'], 'remote checkout path')
    build = next(c for c in commands if c['name'] == 'build-slugd')
    binary_path = PurePosixPath(build['argv'][build['argv'].index('-o') + 1])
    remote_artifacts = binary_path.parent.parent
    need(binary_path.is_absolute() and str(binary_path) == str(remote_artifacts / 'bin/slugd') and
         '..' not in binary_path.parts, 'original artifact root')
    goenv = report['go_environment']
    for key, value in {'GOVERSION': 'go1.27.4', 'GOOS': 'linux', 'GOARCH': 'amd64', 'CGO_ENABLED': '1',
                       'GOFLAGS': '', 'GOEXPERIMENT': ''}.items():
        need(goenv.get(key) == value, 'Go environment: ' + key)
    need(type(goenv['CC']) is str and goenv['CC'], 'original C compiler selection')
    replace = {'$CHAIN': str(chain), '$WALLET': str(chain.parent / 'slugwallet'),
               '$DESKTOP': str(chain / 'desktop'), '$ARTIFACT': str(remote_artifacts), '$CC': goenv['CC']}

    def expand(value):
        for key, replacement in replace.items():
            value = value.replace(key, replacement)
        return value

    need(goenv['GOWORK'] == expand('$DESKTOP/go.work'), 'original desktop workspace')
    groups = report['test_results']
    need([g['command'] for g in groups] == [g['command'] for g in expected['groups']], 'all 32 groups in exact order')
    actual_groups = {g['command']: g for g in groups}
    selected_groups = {g['command']: g for g in expected['groups']}
    required_passes = 0
    for command, selected in zip(commands, expected['commands']):
        name = command['name']
        need(command['argv'] == [expand(x) for x in selected['argv']] and command['cwd'] == expand(selected['cwd']), 'exact command arguments/cwd: ' + name)
        need(command.get('passed') is True and type(command.get('exit_code')) is int and command['exit_code'] == 0 and
             command.get('group_empty') is True and command.get('signals') == [] and command.get('cleanup_errors') == [] and
             not command.get('error'), 'successful joined command: ' + name)
        need(integer(command['pid'], 1) and command['pgid'] == command['pid'], 'original command process group: ' + name)
        need(finite(command['bound_seconds'], True) and command['bound_seconds'] <= selected['max_bound_seconds'] and
             finite(command['elapsed_seconds']), 'finite original command bound: ' + name)
        env = {'GOENV': 'off', 'GOTOOLCHAIN': 'local', 'GOFLAGS': '', 'GOEXPERIMENT': '', 'GOOS': 'linux',
               'GOARCH': 'amd64', 'CGO_ENABLED': selected['cgo'], 'GOMAXPROCS': '2', 'GOWORK': expand(selected['workspace'])}
        need(command['environment'] == env, 'exact environment: ' + name)
        need(command['log'] == 'logs/' + name + '.log', 'named log path: ' + name)
        log, identity = read(artifacts / command['log'], expected['log_limit_bytes'])
        need(identity == {'bytes': command['log_bytes'], 'sha256': command['log_sha256']}, 'log bytes/hash: ' + name)
        if name in selected_groups:
            wanted, observed = selected_groups[name], actual_groups[name]
            need(observed['required'] == wanted['required'] and observed['tags'] == wanted['tags'], 'exact required names/tags: ' + name)
            events = []
            for line in log.splitlines():
                try:
                    event = decode(line)
                except (ValueError, UnicodeError):
                    continue
                if isinstance(event, dict) and event.get('Action') in ('pass', 'fail', 'skip'):
                    events.append({k: event[k] for k in ('Action', 'Package', 'Test', 'Elapsed') if k in event})
            need(events == observed['events'], 'reported events exactly match bounded log: ' + name)
            need(not any(e['Action'] == 'fail' for e in events), 'no failed test/package: ' + name)
            packages = [e['Package'] for e in events if e['Action'] == 'pass' and 'Test' not in e]
            need(sorted(packages) == sorted(wanted['package_names']), 'every selected package passed: ' + name)
            for test in wanted['required']:
                matches = [e for e in events if e.get('Test') == test and e.get('Package') == wanted['required_packages'][test]]
                need(len(matches) == 1 and matches[0]['Action'] == 'pass', 'required assertion PASS, not skip: ' + name + '/' + test)
                required_passes += 1
        if name.startswith('sources-'):
            _, phase, owner, part = name.split('-')
            need(phase in ('before', 'after'), 'inventory phase')
            if part == 'head':
                need(log == (sources[owner]['head'] + '\n').encode(), 'head command evidence')
            elif part == 'status':
                need(log == b'', 'clean source status evidence')
            else:
                need(log.endswith(b'\0') and sorted(x.decode() for x in log[:-1].split(b'\0')) == sorted(inventories[owner]), 'full tracked source list evidence')
    need(required_passes == expected['totals']['required_assertions'] == 470, '470 required PASS assertions')
    for cheap in (False, True):
        name = 'cheap-wire-selection' if cheap else 'normal-wire-selection'
        raw, _ = read(artifacts / ('logs/' + name + '.log'), expected['log_limit_bytes'])
        wire = decode(raw)
        need(wire['Dir'] == expand('$CHAIN/wire'), 'original wire module')
        yes, no = ('pow_testpow.go', 'pow_production.go') if cheap else ('pow_production.go', 'pow_testpow.go')
        need(yes in wire['GoFiles'] and no not in wire['GoFiles'], 'normal/cheap wire selection')
    need([b['path'] for b in report['binaries']] == ['bin/slugd', 'bin/slugpoold', 'bin/slugpilot'], 'all three normal binaries')
    for binary in report['binaries']:
        path = relative(binary['path'])
        need(all(binary.get(x) is True for x in ('profile_verified', 'normal_tags_and_symbols', 'unchanged_after_checks')), 'normal binary assertions')
        _, identity = read(artifacts / path, 256 << 20, False)
        need(identity == {'bytes': binary['bytes'], 'sha256': binary['sha256']}, 'binary bytes/hash: ' + path)
        name = PurePosixPath(path).name
        metadata, _ = read(artifacts / ('logs/verify-' + name + '.log'), expected['log_limit_bytes'])
        symbols, _ = read(artifacts / ('logs/symbols-' + name + '.log'), expected['log_limit_bytes'])
        need(b'slugtestpow' not in metadata and b'slugtestclock' not in metadata and
             all(s.encode() not in symbols for s in expected['forbidden_symbols']), 'no diagnostic profile in normal binary')
        need(all(x in metadata for x in (b'GOOS=linux', b'GOARCH=amd64', b'CGO_ENABLED=' + (b'1' if name == 'slugpilot' else b'0'))), 'normal binary target')
    all_ci = False
    if ci_report is not None:
        raw, _ = read(ci_report, 2 << 20)
        ci = decode(raw)
        need(ci['head_sha'] == expected['head'] and ci['branch'] == expected['branch'] and ci['status'] == 'passed', 'exact-tip CI observation')
        runs = ci['runs']
        need(len(runs) == len(expected['overall_jobs']) and {r['name'] for r in runs} == set(expected['overall_jobs']), 'exact two workflow observations')
        for run in runs:
            need(run['status'] == 'completed' and run['conclusion'] == 'success' and integer(run['id'], 1), 'completed workflow')
            jobs = run['jobs']
            need(len(jobs) == len(expected['overall_jobs'][run['name']]) and
                 {j['name'] for j in jobs} == set(expected['overall_jobs'][run['name']]) and
                 all(j['status'] == 'completed' and j['conclusion'] == 'success' for j in jobs), 'all seven named jobs passed')
        all_ci = True
    # Rehash only the exact allowlisted inputs, never walk the download tree.
    for path, identity in inputs.items():
        _, after = read_public(Path(path), limits[path], False)
        need(after == identity, 'final unchanged public input: ' + path)
    return {'version': 1, 'passed': True, 'scope': 'Downloaded exact-tip Set81 Linux operator gate verification',
            'head': expected['head'], 'wallet': expected['wallet'], 'required_passes': required_passes,
            'test_groups': len(groups), 'commands_verified': len(commands), 'sources_equal': True, 'clean_join': True,
            'actual_or_deployment_claim': False, 'overall_seven_job_ci_verified': all_ci,
            'report': report_ident, 'sources_before': before_ident, 'sources_after': after_ident,
            'inputs': [{'path': path, **identity} for path, identity in sorted(inputs.items())]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('artifacts', help='canonical absolute downloaded operator-linux directory')
    parser.add_argument('--ci-report', help='optional exact-tip root watch report for all seven CI jobs')
    parser.add_argument('--out', required=True, help='fresh absolute verification JSON outside the artifact tree')
    args = parser.parse_args()
    artifacts = canonical_absolute(args.artifacts)
    need(artifacts.is_dir(), 'artifact directory')
    output = Path(args.out)
    need(output.is_absolute() and str(output.parent) == str(output.parent.resolve(strict=True)) and
         not output.exists() and not output.is_symlink() and output != artifacts and artifacts not in output.parents,
         'fresh output outside original artifact tree')
    try:
        result = verify(artifacts, canonical_absolute(args.ci_report) if args.ci_report else None)
    except (OSError, ValueError, KeyError, TypeError, IndexError, AssertionError) as error:
        result = {'version': 1, 'passed': False, 'scope': 'Downloaded Set81 Linux evidence verification',
                  'error': type(error).__name__ + ': ' + str(error), 'actual_or_deployment_claim': False}
    raw = (json.dumps(result, indent=2, sort_keys=True) + '\n').encode()
    fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'wb') as handle:
        handle.write(raw)
    print(json.dumps({'passed': result['passed'], 'verification': str(output)}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
