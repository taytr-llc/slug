"""Exact candidate build-profile binding for Set48 acceptance tools.

This checks build provenance; it does not certify a compiler fix or a release.
"""
import hashlib
import json
import os
import pathlib
import shutil
import subprocess

W = pathlib.Path('/Users/taytrrip/slug')
R = W / 'slug'
PROFILE = R / 'build-profile.json'
WRAPPER = R / 'scripts/candidate-go.sh'
EXPECTED = dict(version=1, id='slug-go1274-no-cse-v1', go_version='go1.27.4',
                gcflags='all=-d=ssa/generic_cse/off,ssa/lowered_cse/off',
                scope='candidate_engineering')

def need(value, message):
    if not value:
        raise RuntimeError(message)

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def environment(base=None, **extra):
    env = dict(os.environ if base is None else base)
    env.update(GOENV='off', GOTOOLCHAIN='local', GOEXPERIMENT='',
               GOFLAGS='-mod=readonly', GOCACHEPROG='')
    env.update(extra)
    return env

def binding():
    need(PROFILE.is_file() and not PROFILE.is_symlink(), 'owned build profile missing')
    need(json.loads(PROFILE.read_bytes()) == EXPECTED, 'unexpected candidate build profile')
    need(WRAPPER.is_file() and not WRAPPER.is_symlink(), 'owned build wrapper missing')
    env = environment()
    values = json.loads(subprocess.check_output(
        ['go', 'env', '-json', 'GOVERSION', 'GOROOT', 'GOTOOLDIR', 'GOTOOLCHAIN', 'GOEXPERIMENT'],
        env=env, timeout=20))
    need(values['GOVERSION'] == EXPECTED['go_version'] and values['GOTOOLCHAIN'] == 'local'
         and values['GOEXPERIMENT'] == '', 'local compiler profile mismatch')
    go = pathlib.Path(shutil.which('go')).resolve()
    compiler = pathlib.Path(values['GOTOOLDIR']) / 'compile'
    linker = pathlib.Path(values['GOTOOLDIR']) / 'link'
    return dict(profile=EXPECTED, profile_sha256=sha(PROFILE), wrapper_sha256=sha(WRAPPER),
                checker_sha256=sha(pathlib.Path(__file__)), go_binary=str(go), go_sha256=sha(go),
                compiler=str(compiler), compiler_sha256=sha(compiler),
                linker=str(linker), linker_sha256=sha(linker))

def same(bound):
    need(binding() == bound, 'candidate compiler/profile bytes changed')

def command(verb, *args):
    need(verb in {'build', 'test', 'install'}, 'unqualified build command')
    return ['sh', str(WRAPPER), verb, *map(str, args)]

def parse_build_info(raw, *, tags, cgo, goos='darwin', goarch='arm64'):
    lines = raw.decode('utf-8').splitlines()
    need(lines and lines[0].rsplit(': ', 1)[-1] == EXPECTED['go_version'], 'artifact compiler version mismatch')
    settings = {}
    for line in lines[1:]:
        if not line.startswith('\tbuild\t'):
            continue
        key, separator, value = line[len('\tbuild\t'):].partition('=')
        need(separator and key not in settings, 'ambiguous artifact build settings')
        settings[key] = value
    need(settings.get('-gcflags') == EXPECTED['gcflags'], 'artifact compiler option mismatch')
    need(settings.get('-compiler') == 'gc' and settings.get('-buildmode') == 'exe', 'artifact compiler/build mode mismatch')
    need(settings.get('GOOS') == goos and settings.get('GOARCH') == goarch, 'artifact target mismatch')
    need(settings.get('CGO_ENABLED') == str(cgo), 'artifact CGO mismatch')
    need(settings.get('-tags', '') == tags, 'artifact tag mismatch')
    need(settings.get('GOEXPERIMENT', '') == '' and not any(
        key in settings for key in ['-toolexec','-asmflags','-gccgoflags','-pgo','-race','-msan','-asan']),
         'unexpected artifact compiler override')
    return settings

def artifact(path, info_path, *, tags, cgo, goos='darwin', goarch='arm64'):
    path, info_path = pathlib.Path(path), pathlib.Path(info_path)
    need(path.is_file() and not path.is_symlink(), 'owned executable missing')
    before = sha(path)
    raw = subprocess.check_output(['go', 'version', '-m', str(path)], env=environment(), timeout=20)
    need(len(raw) <= 262144, 'oversized artifact build information')
    settings = parse_build_info(raw, tags=tags, cgo=cgo, goos=goos, goarch=goarch)
    with info_path.open('xb') as stream:
        stream.write(raw)
    need(sha(path) == before, 'artifact changed during inspection')
    return dict(path=str(path), sha256=before, build_info=str(info_path),
                build_info_sha256=sha(info_path), settings=settings,
                go_version=EXPECTED['go_version'], compiler_setting_evidence='embedded_build_info')

def unchanged(evidence):
    for row in evidence.values():
        need(sha(pathlib.Path(row['path'])) == row['sha256'], 'executed artifact bytes changed')
        need(sha(pathlib.Path(row['build_info'])) == row['build_info_sha256'], 'artifact build evidence changed')
