"""Set80 current-source HANDOVER smoke. Build first; --run starts owned node.

No live data, deployment, private key or public-chain mining is permitted.
"""
import hashlib
import importlib.util
import json
import os
import pathlib
import shutil
import socket
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True
LEGACY = pathlib.Path('/Users/taytrrip/slug/slug/desktop/pooloperator/testdata/late-winner-actual')
sys.path.insert(0, str(LEGACY))
sys.path.insert(0, '/private/tmp')
import slug_set48_build_profile as BP
_spec = importlib.util.spec_from_file_location('set80_smoke_capture', LEGACY/'run.py')
CAP = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(CAP)

ROOT = pathlib.Path('/Users/taytrrip/slug/slug')
OUT = pathlib.Path('/Users/taytrrip/slug/artifacts/set80/smoke-attempt1')
CHECKPOINT = ROOT / 'checkpoints/phase-a/height-1.json'
PUBLIC = '811349556ea792060edbb0518dd3a513c5906abe495a81eaf629abd5900c44a3'
SEED = 'junction.proxy.rlwy.net:16023'
ENV = BP.environment(GOCACHE='/private/tmp/slug-go-build',
                     GOMODCACHE='/Users/taytrrip/slug/build-cache/go-mod',
                     GOPROXY='off', GOWORK=str(ROOT/'go.work'),
                     GOMAXPROCS='2', CGO_ENABLED='0')

def need(value, message):
    if not value:
        raise RuntimeError(message)

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def write(name, value):
    (OUT/name).write_text(json.dumps(value, indent=2)+'\n')

def emit(event, **values):
    print(json.dumps({'event': event, **values}), flush=True)

def check_sources(evidence):
    for path, digest in evidence['files'].items():
        need(sha(pathlib.Path(path)) == digest, 'build dependency changed: '+path)

def captured(commands, path, argv, seconds):
    row = CAP.capture(argv, ROOT, ENV, path, seconds)
    commands.append(row)
    write('build-commands.json', commands)
    need(CAP.okay(row), 'owned command failed: '+path.name)
    return path.read_bytes()

def source_inputs(commands):
    raw = captured(commands, OUT/'source-packages.jsonl',
                   ['go', 'list', '-deps', '-json', '.', './cmd/slugctl'], 90)
    decoder, remaining, packages, files = json.JSONDecoder(), raw.decode(), [], set()
    while remaining.strip():
        package, index = decoder.raw_decode(remaining.lstrip())
        remaining = remaining.lstrip()[index:]
        packages.append(package['ImportPath'])
        directory = pathlib.Path(package['Dir'])
        for field in ('GoFiles', 'CgoFiles', 'CFiles', 'CXXFiles', 'MFiles', 'HFiles', 'FFiles', 'SFiles', 'SwigFiles', 'SwigCXXFiles', 'SysoFiles', 'EmbedFiles'):
            for relative in package.get(field, []):
                files.add(directory/relative)
        module = package.get('Module', {})
        for candidate in (module, module.get('Replace', {})):
            if candidate.get('GoMod'):
                mod = pathlib.Path(candidate['GoMod'])
                files.add(mod)
                if mod.with_name('go.sum').is_file():
                    files.add(mod.with_name('go.sum'))
    for name in ('go.work', 'go.work.sum', 'build-profile.json', 'scripts/candidate-go.sh'):
        file = ROOT/name
        if file.is_file():
            files.add(file)
    files.add(CHECKPOINT)
    files.update((pathlib.Path(__file__), LEGACY/'run.py', LEGACY/'runner_support.py',
                  pathlib.Path('/private/tmp/slug_set48_build_profile.py')))
    values = {str(file): sha(file) for file in sorted(files)}
    encoded = json.dumps(values, sort_keys=True, separators=(',', ':')).encode()
    return {'version': 1, 'git_head': subprocess.check_output(['git','rev-parse','HEAD'], cwd=ROOT,text=True,timeout=10).strip(),
            'git_branch': subprocess.check_output(['git','branch','--show-current'], cwd=ROOT,text=True,timeout=10).strip(),
            'git_status': subprocess.check_output(['git','status','--short'], cwd=ROOT,text=True,timeout=10),
            'targets': ['.', './cmd/slugctl'], 'packages': packages, 'files': values,
            'files_sha256': hashlib.sha256(encoded).hexdigest()}

def final_checks(result, inputs, bound, artifacts, evidence, binaries):
    checks = [('sources', lambda: check_sources(inputs)),
              ('profile', lambda: BP.same(bound))]
    checks += [('artifact-'+name, lambda row=row: BP.unchanged({'artifact': row}))
               for name, row in artifacts.items()]
    checks += [('binary-'+name, lambda path=path, value=value:
                need(CAP.support.identity(path) == value, 'binary changed'))
               for name, (path, value) in binaries.items()]
    checks += [('evidence-'+path.name, lambda path=path, value=value:
                need(CAP.support.identity(path) == value, 'evidence changed'))
               for path, value in evidence.items()]
    result['final_checks'] = {}
    for label, check in checks:
        try:
            check()
            result['final_checks'][label] = True
        except BaseException as error:
            result['final_checks'][label] = False
            result.setdefault('final_errors', []).append(label+': '+type(error).__name__)
    if not all(result['final_checks'].values()):
        result['status'] = 'failed'

def build():
    OUT.mkdir(parents=True, exist_ok=False)
    (OUT/'bin').mkdir()
    shutil.copy2(__file__, OUT/'runner.py')
    bound, inputs = None, None
    artifacts, commands, evidence, binaries = {}, [], {}, {}
    result = {'version': 1, 'status': 'failed', 'commands': commands}
    try:
        evidence[OUT/'runner.py'] = CAP.support.identity(OUT/'runner.py')
        bound = BP.binding()
        write('build-profile-binding.json', bound)
        evidence[OUT/'build-profile-binding.json'] = CAP.support.identity(OUT/'build-profile-binding.json')
        inputs = source_inputs(commands)
        write('source-inputs.json', inputs)
        evidence[OUT/'source-inputs.json'] = CAP.support.identity(OUT/'source-inputs.json')
        for name, target in [('slugd','.'), ('slugctl','./cmd/slugctl')]:
            binary = OUT/'bin'/name
            try:
                captured(commands, OUT/(name+'-build.log'),
                         BP.command('build', '-p=1', '-o', binary, target), 180)
            finally:
                # Retain/recheck an output even when its command or group failed;
                # recording bytes does not qualify that failed build.
                if binary.exists():
                    binaries[name] = (binary, CAP.support.identity(binary))
            info = OUT/(name+'-build-info.txt')
            raw = captured(commands, info, ['go', 'version', '-m', str(binary)], 20)
            need(len(raw) <= 262144, 'oversized artifact build information')
            settings = BP.parse_build_info(raw, tags='', cgo=0)
            artifacts[name] = dict(path=str(binary), sha256=binaries[name][1]['sha256'],
                                   build_info=str(info), build_info_sha256=sha(info), settings=settings,
                                   go_version=BP.EXPECTED['go_version'], compiler_setting_evidence='embedded_build_info')
        write('build-artifacts.json', artifacts)
        evidence[OUT/'build-artifacts.json'] = CAP.support.identity(OUT/'build-artifacts.json')
        result['status'] = 'passed'
    except BaseException as error:
        result['error'] = type(error).__name__+': '+str(error)
    finally:
        final_checks(result, inputs, bound, artifacts, evidence, binaries)
        result['artifacts'] = artifacts
        result['binary_identities'] = {name: {'path': str(path), **value}
                                       for name, (path, value) in binaries.items()}
        result['clean_join'] = all(CAP.okay(row) for row in commands)
        if not result['clean_join']:
            result['status'] = 'failed'
        write('build-result.json', result)
    need(result['status'] == 'passed', 'build failed; evidence preserved')
    emit('built_current_source', output=str(OUT), dependency_files=len(inputs['files']), dependency_sha256=inputs['files_sha256'])

def run():
    need(not (OUT/'result.json').exists(), 'previous smoke result exists; preserve the attempt')
    result = {'version': 1, 'status': 'failed', 'checkpoint': str(CHECKPOINT), 'seed_endpoint': SEED,
              'public_chain_mined': False, 'isolated_blocks_before_public_connection': 2,
              'sync_limit_seconds': 300}
    inputs, bound, runpath, log, node, ctl = None, None, None, None, None, None
    artifacts, evidence, binaries, nodes, joined = {}, {}, {}, [], {}
    connected = False

    def rpc(*params, timeout=30):
        need(not (connected and params[0] == 'generate'), 'no mining after public connection')
        raw = subprocess.check_output([*ctl, *map(str,params)], text=True, stderr=subprocess.PIPE, timeout=timeout).strip()
        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            return raw

    def start():
        nonlocal node
        if node is not None:
            need(node.pid in joined and joined[node.pid]['clean'], 'previous node group has not joined')
        node = subprocess.Popen(args, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        nodes.append(node)
        for _ in range(150):
            need(node.poll() is None, 'owned node exited during startup')
            try:
                rpc('getblockcount', timeout=3)
                return
            except subprocess.CalledProcessError:
                time.sleep(.2)
        raise RuntimeError('RPC startup timed out')

    def join_node(original, request_stop=False):
        if original.pid in joined:
            return joined[original.pid]
        errors = []
        if original.poll() is None and request_stop:
            try:
                subprocess.run([*ctl, 'stop'], stdout=subprocess.DEVNULL,
                               stderr=subprocess.DEVNULL, timeout=5, check=True)
                original.wait(timeout=30)
            except BaseException as error:
                errors.append(type(error).__name__)
        # Cleanup owns precisely the original launched PGID, including any
        # descendants after its leader exits. Never signal by executable/name.
        try:
            row = CAP.support.cleanup(original)
        except BaseException as error:
            row = {'pid': original.pid, 'pgid': original.pid, 'group_empty': False,
                   'signals': [], 'errors': [type(error).__name__]}
        row['stop_errors'] = errors
        row['clean'] = (original.poll() == 0 and row.get('group_empty') is True and
                        row.get('leftover_after_exit') is False and not row.get('signals') and
                        not row.get('errors') and not errors)
        joined[original.pid] = row
        return row

    def report(status):
        data = rpc('getcheckpointinfo')
        need(data['mode']=='report-only' and data['authorityconfigured'] is True, 'checkpoint mode changed')
        need(data['publickey']==PUBLIC and data['requireddescendants']==60, 'checkpoint authority/depth changed')
        need(len(data['checkpoints'])==1, 'unexpected checkpoint count')
        record = data['checkpoints'][0]
        for key,value in signed.items():
            need(record[key]==value, 'signed checkpoint field changed: '+key)
        need(record['status']==status and data['conflict']==(status=='conflict'), 'unexpected checkpoint status')
        if status == 'finalized':
            need(record['localhash']==signed['hash'], 'finalized wrong checkpoint')
            need(record['descendants']==data['bestheight']-1>=60, 'wrong finalized descendants')
        return data

    try:
        need(sha(pathlib.Path(__file__)) == sha(OUT/'runner.py'), 'runner changed after build')
        need(json.loads((OUT/'build-result.json').read_text())['status'] == 'passed', 'build was not qualified')
        inputs = json.loads((OUT/'source-inputs.json').read_text())
        bound = json.loads((OUT/'build-profile-binding.json').read_text())
        artifacts = json.loads((OUT/'build-artifacts.json').read_text())
        need(set(artifacts) == {'slugd', 'slugctl'}, 'both qualified artifacts required')
        for name in ('runner.py', 'source-inputs.json', 'build-profile-binding.json', 'build-artifacts.json', 'build-result.json'):
            evidence[OUT/name] = CAP.support.identity(OUT/name)
        BP.same(bound)
        BP.unchanged(artifacts)
        check_sources(inputs)
        for name, row in artifacts.items():
            path = pathlib.Path(row['path'])
            binaries[name] = (path, CAP.support.identity(path))
        signed = json.loads(CHECKPOINT.read_text())
        runpath = pathlib.Path(tempfile.mkdtemp(prefix='slug-set78-smoke-', dir='/private/tmp'))
        (OUT/'runtime-path.txt').write_text(str(runpath)+'\n')
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        slugd, slugctl = str(OUT/'bin/slugd'), str(OUT/'bin/slugctl')
        result.update(run=str(runpath), source=inputs['git_head'],
                      source_inputs_sha256=sha(OUT/'source-inputs.json'),
                      build_profile_binding_sha256=sha(OUT/'build-profile-binding.json'),
                      build_artifacts_sha256=sha(OUT/'build-artifacts.json'))
        result['node_version'] = subprocess.check_output([slugd, '--version'], text=True, timeout=20).strip()
        need(result['node_version'] == 'slugd version 0.27.0', 'unexpected node version')
        ctl = [slugctl, '--notls', '--rpcuser=u', '--rpcpass=p', '--rpcserver=127.0.0.1:'+str(port),
               '--configfile='+str(runpath/'slugctl.conf')]
        args = [slugd, '--notls', '--rpcuser=u', '--rpcpass=p',
                '--datadir='+str(runpath/'data'), '--logdir='+str(runpath/'logs'),
                '--configfile='+str(runpath/'slug.conf'), '--nolisten', '--nodnsseed',
                '--rpclisten=127.0.0.1:'+str(port), '--signedcheckpoint='+str(CHECKPOINT),
                '--miningaddr=amkchND9pP6STAzbb4M6U4SzU2anReC4R4']
        result['node_arguments'] = args
        log = (runpath/'node.log').open('w')
        emit('starting_owned_node', run=str(runpath))
        start()
        need(rpc('getblockcount')==0 and rpc('getblockhash',0)==signed['genesis'], 'not a fresh expected chain')
        result['initial'] = report('pending')
        need('report-only' in rpc('help','getcheckpointinfo'), 'missing report-only RPC help')
        need(rpc('getconnectioncount')==0, 'isolated mining requires zero peers')
        result['mined'] = rpc('generate',2,timeout=180)
        need(len(result['mined'])==2 and rpc('getblockcount')==2 and rpc('getconnectioncount')==0, 'isolated mining failed')
        block = rpc('getblock',result['mined'][0],2)
        result['coinbase_value'] = sum(o['value'] for o in block['rawtx'][0]['vout'])
        need(result['coinbase_value']==10, 'block subsidy changed')
        need(result['mined'][0]!=signed['hash'], 'isolated branch did not conflict')
        result['conflicting_branch'] = report('conflict')
        need(result['conflicting_branch']['checkpoints'][0]['localhash']==result['mined'][0], 'wrong conflict branch')
        emit('isolated_mining_passed', coinbase=result['coinbase_value'], report_only_conflict=True)
        connected = True
        rpc('addnode',SEED,'add')
        started, notice = time.monotonic(), -30
        while time.monotonic()-started<300:
            remaining = max(.1, 300-(time.monotonic()-started))
            peers = rpc('getpeerinfo',timeout=min(10,remaining))
            remaining = max(.1, 300-(time.monotonic()-started))
            chain = rpc('getblockchaininfo',timeout=min(10,remaining))
            if chain['blocks']>=61 and chain['headers']==chain['blocks'] and any(p['subver']=='/btcwire:0.5.0/slug:0.27.0/' for p in peers):
                result['peers'],result['chain']=peers,chain
                break
            if time.monotonic()-started-notice>=30:
                emit('syncing_read_only',blocks=chain['blocks'],headers=chain['headers'])
                notice=time.monotonic()-started
            time.sleep(.5)
        else:
            raise RuntimeError('public seed sync timed out after 300 seconds')
        result['sync_seconds']=time.monotonic()-started
        need(rpc('getblockhash',3)=='42d92f37dd5d5f1e52aa374d00635ecd4d5983a092ea502fe4d0e9b2f7105188','height3 history changed')
        need(rpc('getblockhash',61)=='1f08a4ac11d4c60ebecbb8dc85401fa2756576c3d1920c45cad5f7113b587ee0','height61 history changed')
        need(chain['blocks']==61,'public seed height changed')
        result['finalized']=report('finalized')
        for name in ('csv','segwit','taproot'):
            deployment=chain['bip9_softforks'][name]
            need(deployment['status']=='active' and deployment['always_active_height']==62,'activation changed: '+name)
        result['next_block_activation']={'candidate_height':62,'csv':True,'segwit':True,'taproot':True}
        result['verifychain']=rpc('verifychain',1,chain['blocks'],timeout=120)
        need(result['verifychain'] is True,'chain verification failed')
        try:
            rpc('getblocktemplate','{"mode":"template"}')
            raise RuntimeError('aged chain unexpectedly permits normal mining template')
        except subprocess.CalledProcessError as error:
            need('downloading blocks' in error.stderr,'unexpected stale template error')
        result['normal_stale_template_refused']=True
        need(rpc('stop')=='slugd stopping.' and node.wait(timeout=30)==0,'first stop failed')
        need(join_node(node)['clean'], 'first node process group did not join cleanly')
        start()
        result['after_restart']=report('finalized')
        need(result['after_restart']['besthash']==result['finalized']['besthash'],'restart changed chain')
        need(rpc('stop')=='slugd stopping.','final stop failed')
        result['exit']=node.wait(timeout=30)
        need(result['exit']==0,'nonzero final exit')
        log.flush()
        logs=(runpath/'node.log').read_text()
        need('Signed league checkpoints are report-only: authority='+PUBLIC in logs,'checkpoint startup log missing')
        need('status=finalized descendants=60' in logs,'finalized log missing')
        result['startup_checkpoint_log_verified']=True
        result['status']='passed'
    except BaseException as error:
        result['status'],result['error']='failed',type(error).__name__+': '+str(error)
        emit('failed',error=result['error'])
    finally:
        groups = [join_node(original, request_stop=True) for original in reversed(nodes)]
        cleanup = {'owned_pid': node.pid if node is not None else None,
                   'forced': any(row.get('signals') for row in groups),
                   'joined': all(original.poll() is not None for original in nodes),
                   'exit_code': node.poll() if node is not None else None,
                   'groups': groups,
                   'remaining_process_groups': [row['pgid'] for row in groups if row.get('group_empty') is not True]}
        if not all(row['clean'] for row in groups):
            result.update(status='failed', error='owned node required forced or incomplete group cleanup')
        result['cleanup']=cleanup
        for operation in ([('log-close', log.close)] if log is not None else []) + (
                [('log-copy', lambda: shutil.copy2(runpath/'node.log', OUT/'node.log'))]
                if runpath is not None and (runpath/'node.log').is_file() else []):
            try:
                operation[1]()
            except BaseException as error:
                result.setdefault('final_errors', []).append(operation[0]+': '+type(error).__name__)
                result['status'] = 'failed'
        final_checks(result, inputs, bound, artifacts, evidence, binaries)
        write('result.json',result)
        if runpath is not None:
            (runpath/'result.json').write_text(json.dumps(result,indent=2)+'\n')
    need(result.get('status')=='passed','smoke failed; evidence preserved')
    emit('passed',run=str(runpath),descendants=result['after_restart']['checkpoints'][0]['descendants'])

if __name__=='__main__':
    if sys.argv[1:]==['--build']:
        build()
    elif sys.argv[1:]==['--run']:
        run()
    else:
        raise SystemExit('choose --build or --run')
