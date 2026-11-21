"""Root-owned, finite Set79 public DAILY original-owner acceptance runner."""
from pathlib import Path
import argparse, hashlib, importlib.util, json, os, re, shutil, sys

BASE=Path('/Users/taytrrip/slug'); ROOT=BASE/'slug'; WALLET=BASE/'slugwallet'
HERE=BASE/'set79-prep/public-daily-actual'
LEGACY=ROOT/'desktop/pooloperator/testdata/late-winner-actual'
PROFILE=Path('/private/tmp/slug_set48_build_profile.py')
REFERENCE=ROOT/'testdata/public-daily/reference-v1.json'
ECONOMICS=ROOT/'poolsettle/testdata/daily-admission-actual/reference-v1.json'
CHECKER=BASE/'set79-prep/check-public-actual.py'
SOURCES=('main.go','node_helpers.go','late_flow.go','late_audit.go','balance_wait.go','go.mod')
PINNED={LEGACY/'run.py':'b81af69c02a18b7f74ed96e08ae4dba8548e94a4958aeacb721c62dcbf8ba05e',
        LEGACY/'runner_support.py':'54bb3e7caf33e744ce76bf366dd23a4eff4d370d9246b85758089b57dd2439d7',
        PROFILE:'b6033d66edb4d24a57a7fac4e59ddaeee6964f0943aca21fddc6f65c7f5e6ee4',
        REFERENCE:'a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3',
        ECONOMICS:'1d2b1462296bd022c53df9dc583db3c38cd13a8e13e84f9983f2505b3aefb08f'}

def need(value,reason):
    if not value:raise RuntimeError(reason)

def module(name,path):
    spec=importlib.util.spec_from_file_location(name,path)
    value=importlib.util.module_from_spec(spec);spec.loader.exec_module(value);return value

def run(args):
    for p,h in PINNED.items():need(hashlib.sha256(p.read_bytes()).hexdigest()==h,'inherited source/reference changed')
    sys.path[:0]=[str(LEGACY),str(PROFILE.parent)];sys.dont_write_bytecode=True
    old=module('set79_original_capture',LEGACY/'run.py');support=old.support
    bp=module('set79_actual_profile',PROFILE)
    lockpath=Path(args.approved_lock)
    need(support.identity(lockpath)['sha256']==args.approved_lock_sha256,'execution lock changed')
    lock=json.loads(lockpath.read_bytes())
    required=set(PINNED)|{HERE/s for s in SOURCES}|{HERE/'PLAN.md',HERE/'source-manifest.json',HERE/'balance-wait-manifest.json',CHECKER,Path(__file__).resolve(),ROOT/'desktop/go.sum',ROOT/'build-profile.json',ROOT/'scripts/candidate-go.sh'}
    need(lock.get('approved_by')=='root' and lock.get('execution_authorized') is True and lock.get('scope')=='set79-public-daily-actual','missing exact execution selection')
    need(required <= set(map(Path,lock['files'])) and len(lock['files'])<=160,'incomplete/oversized input selection')
    def locked():return all(support.identity(p)==v for p,v in lock['files'].items()) and support.identity(lockpath)['sha256']==args.approved_lock_sha256
    need(locked(),'selected input changed')
    checker=module('set79_public_checker',CHECKER)
    artifact=BASE/'artifacts/set79'/args.attempt;artifact.mkdir(mode=0o700,exist_ok=False)
    report=dict(scope=lock['scope'],passed=False,actual_started=False,commands=[],fixture_timeout_seconds=900,outer_timeout_seconds=1080,max_blocks=180)
    before=None;built={};mapping={};bound=None
    try:
        support.write(artifact/'execution-lock.json',lock)
        bound=bp.binding();support.write(artifact/'profile.json',bound)
        probe=artifact/'probe-source';probe.mkdir(mode=0o700)
        for name in SOURCES:
            source=HERE/name;target=probe/name;shutil.copyfile(source,target);os.chmod(target,0o600)
            mapping[str(target)]=dict(source=str(source),**support.identity(source))
        source=ROOT/'desktop/go.sum';target=probe/'go.sum';shutil.copyfile(source,target)
        mapping[str(target)]=dict(source=str(source),**support.identity(source))
        support.write(artifact/'source-copies.json',mapping)
        modules=[probe,ROOT/'desktop',WALLET]+[WALLET/p for p in ('wallet/txauthor','wallet/txrules','wallet/txsizes','walletdb','wtxmgr')]+[ROOT]+[ROOT/p for p in ('address','btcec','btcutil','chaincfg','chainhash','psbt','txscript','v2transport','wire')]
        work=artifact/'go.work'
        with work.open('x') as f:f.write('go 1.27.4\nuse (\n'+''.join('\t'+json.dumps(str(p))+'\n' for p in modules)+')\n')
        env=bp.environment(GOCACHE='/private/tmp/slug-go-build',GOMODCACHE=str(BASE/'build-cache/go-mod'),GOPROXY='off',GOSUMDB='off',GOWORK=str(work),GOMAXPROCS='2',CGO_ENABLED='1',GOOS='darwin',GOARCH='arm64')
        for k in list(env):
            if k.startswith('SLUG_'):del env[k]
        env['PYTHONDONTWRITEBYTECODE']='1';nodeenv=dict(env,CGO_ENABLED='0')
        files=set(map(Path,lock['files']))|{lockpath,work}|set(map(Path,mapping))
        def execute(label,argv,cwd,selected,seconds,cap=16<<20,permitted=()):
            print(json.dumps(dict(starting=label)),flush=True)
            path=artifact/(label+'.log');row=old.capture(argv,cwd,selected,path,seconds,log_cap=cap,permitted=permitted)
            row['label']=label;report['commands'].append(row);need(old.okay(row),'failed '+label)
            print(json.dumps(dict(passed=label,seconds=row['seconds'])),flush=True);return path
        for label,cwd,selected in [('node',ROOT,nodeenv),('operator',ROOT/'desktop/cmd/slugpoold',env),('probe',probe,env)]:
            path=execute(label+'-deps',['go','list','-deps','-json','.'],cwd,selected,120,32<<20)
            files.update(old.dependency_files(path))
        if work.with_suffix('.work.sum').is_file():files.add(work.with_suffix('.work.sum'))
        before={str(p):support.identity(p) for p in sorted(files)};support.write(artifact/'source-before.json',before)
        need(old.check_copies(mapping) and locked(),'copy/input changed before build')
        binaries=artifact/'bin';binaries.mkdir(mode=0o700)
        for label,name,cwd,selected,cgo in [('node','slugd',ROOT,nodeenv,0),('operator','slugpoold',ROOT/'desktop/cmd/slugpoold',env,1),('probe','public-daily-actual',probe,env,1)]:
            target=binaries/name;execute(label+'-build',bp.command('build','-p=1','-mod=readonly','-trimpath','-tags=','-o',target,'.'),cwd,selected,240)
            built[label]=bp.artifact(target,artifact/(label+'-build-info.txt'),tags='',cgo=cgo)
        need(all(support.identity(p)==v for p,v in before.items()) and locked(),'source changed before actual')
        bp.same(bound);bp.unchanged(built)
        report['actual_started']=True
        execute('actual',[built['probe']['path'],'--node',built['node']['path'],'--operator-binary',built['operator']['path'],'--artifacts',str(artifact/'owned'),'--setup-reference',str(REFERENCE),'--economics-reference',str(ECONOMICS)],probe,env,1080,permitted=[x['path'] for x in built.values()])
        report['public_evidence']=checker.check(artifact/'owned',REFERENCE,ECONOMICS,support)
        report['passed']=True
    except BaseException as e:
        report['error']=dict(type=type(e).__name__,message=str(e)[:1000])
    finally:
        # Preserve bounded PUBLIC observations even when the actual subprocess
        # failed before the full success checker. This grants no acceptance.
        try:
            if (artifact/'owned/balance-wait-evidence.json').exists():
                report['balance_wait_diagnostics']=checker.balance_wait_evidence(artifact/'owned',support)
                if report.get('passed'):
                    need(all(report['public_evidence'].get(p)==v for p,v in report['balance_wait_diagnostics']['evidence'].items()),'public balance diagnostics changed after success checks')
        except BaseException as e:
            report['passed']=False
            report['balance_wait_diagnostic_error']=dict(type=type(e).__name__,message=str(e)[:1000])
        report['artifacts']=built
        try:
            after={str(p):support.identity(p) for p in before} if before is not None else None
            if after is not None:support.write(artifact/'source-after.json',after)
            report['sources_unchanged']=before is not None and before==after
            report['copies_unchanged']=bool(mapping) and old.check_copies(mapping)
            need(report['sources_unchanged'] and report['copies_unchanged'] and locked(),'source/input changed after actual')
            need(bound is not None,'missing profile');bp.same(bound);bp.unchanged(built)
            report['profile_artifacts_unchanged']=True
        except BaseException as e:
            report['passed']=False;report['final_binding_error']=dict(type=type(e).__name__,message=str(e)[:1000])
        report['clean_join']=bool(report['commands']) and all(old.okay(c) for c in report['commands'])
        support.write(artifact/'report.json',report)
    print(json.dumps({k:v for k,v in report.items() if k not in ('commands','artifacts','public_evidence','balance_wait_diagnostics')}),flush=True)
    return 0 if report['passed'] else 1

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--approved-lock',required=True);p.add_argument('--approved-lock-sha256',required=True);p.add_argument('--attempt',required=True);a=p.parse_args()
    need(re.fullmatch('public-actual-attempt[1-9][0-9]*',a.attempt) and re.fullmatch('[0-9a-f]{64}',a.approved_lock_sha256) and Path(a.approved_lock).is_absolute(),'invalid explicit invocation')
    raise SystemExit(run(a))
