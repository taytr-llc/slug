"""Bounded root-owned Set81 qualification; each attempt is retained exclusively."""
from pathlib import Path
import argparse,hashlib,importlib.util,json,os,re,sys
BASE=Path('/Users/taytrrip/slug');ROOT=BASE/'slug'
LEGACY=ROOT/'desktop/pooloperator/testdata/late-winner-actual'
PINNED={LEGACY/'run.py':'b81af69c02a18b7f74ed96e08ae4dba8548e94a4958aeacb721c62dcbf8ba05e',LEGACY/'runner_support.py':'54bb3e7caf33e744ce76bf366dd23a4eff4d370d9246b85758089b57dd2439d7',Path('/private/tmp/slug_set48_build_profile.py'):'b6033d66edb4d24a57a7fac4e59ddaeee6964f0943aca21fddc6f65c7f5e6ee4'}
for p,h in PINNED.items():assert hashlib.sha256(p.read_bytes()).hexdigest()==h
sys.dont_write_bytecode=True;sys.path[:0]=[str(LEGACY),'/private/tmp']
import slug_set48_build_profile as bp
spec=importlib.util.spec_from_file_location('set81_capture',LEGACY/'run.py');cap=importlib.util.module_from_spec(spec);spec.loader.exec_module(cap)
parser=argparse.ArgumentParser();parser.add_argument('--attempt',required=True);parser.add_argument('--slice',choices=['core','service','transport','worker','operator','desktop','financial'],required=True);parser.add_argument('--mode',choices=['compile','normal','race','cheap','vet'],required=True);parser.add_argument('--pattern',default='^Test(DailyRetained|ManifestAdmissionPreflight|DailyFencedAdmission|RetainedPoolAdmission)');args=parser.parse_args()
assert re.fullmatch(r'[a-z][a-z0-9-]*-attempt[1-9][0-9]*',args.attempt)
packages={'core':['./registry'], 'service':['./prospectservice'],'transport':['./poolservice','./poolingress'],'worker':['./prospectworker'],'financial':['./poolsettle','./poolschedule'],'operator':['./pooloperator'],'desktop':['./pilot','./accountpilot','./cmd/slugpilot']}[args.slice]
cwd=ROOT/'desktop' if args.slice in ('operator','desktop') else ROOT
out=BASE/'artifacts/set81'/args.attempt;out.mkdir(mode=0o700,exist_ok=False)
env=bp.environment(GOCACHE=str(BASE/'build-cache/go-build-native') if args.slice=='desktop' else '/private/tmp/slug-go-build',GOMODCACHE=str(BASE/'build-cache/go-mod'),GOPROXY='off',GOSUMDB='off',GOMAXPROCS='2',CGO_ENABLED='1',GOWORK=str(ROOT/'desktop/go.work'))
report=dict(version=1,scope='Set81 '+args.slice+' '+args.mode,pattern=args.pattern,passed=False,commands=[])
roots=[ROOT/x for x in ['poolsettle','poolschedule','registry','poolshare','poolservice','poolingress','prospectservice','prospectworker','desktop/pooloperator','desktop/pilot','desktop/accountpilot','desktop/cmd/slugpilot','testdata/issue-recovery']]+[BASE/'set81-prep/oracle',BASE/'set81-prep/oracle-clock-correction',BASE/'set81-prep/oracle-lease-provenance-correction']
fixed=set(PINNED)|{Path(__file__).resolve(),ROOT/'scripts/candidate-go.sh',ROOT/'build-profile.json',BASE/'set81-prep/implementation-selection.json',BASE/'set81-prep/implementation-selection-lease-addendum.json',BASE/'set81-prep/SET81_PLAN.md',ROOT/'scripts/qualify-operator-linux.py',ROOT/'.github/workflows/operator-linux.yml'}
deps_files=set()
def inventory():
 files=set(fixed)|deps_files
 for root in roots:files.update(p for p in root.rglob('*') if p.is_file())
 return {str(p):cap.support.identity(p) for p in sorted(files)}
def run(label,argv,seconds,logcap=32<<20):
 print(json.dumps({'starting':label,'slice':args.slice}),flush=True)
 row=cap.capture(argv,cwd,env,out/(label+'.log'),seconds,log_cap=logcap);row['label']=label;report['commands'].append(row)
 if not cap.okay(row):raise RuntimeError('failed '+label)
 print(json.dumps({'passed':label,'seconds':row['seconds']}),flush=True)
 return out/(label+'.log')
before=None
try:
 profile=bp.binding();cap.support.write(out/'profile.json',profile)
 tags='slugtestpow' if args.mode=='cheap' else ''
 dep_log=run('deps',['go','list','-deps','-test','-json','-mod=readonly','-tags='+tags]+packages,120)
 deps_files.update(cap.dependency_files(dep_log));before=inventory();cap.support.write(out/'sources-before.json',before)
 if args.mode=='vet':
  run('vet',['go','vet']+packages,180)
 else:
  command=['sh',str(ROOT/'scripts/candidate-go.sh'),'test','-count=1','-p=1','-timeout='+'450s','-json']
  if args.mode=='race':command+=['-race']
  if tags:command+=['-tags='+tags]
  pattern='^$' if args.mode=='compile' else args.pattern
  command+=['-run',pattern]+packages
  logfile=run(args.mode,command,480)
  passed=[];failed=[];skipped=[];package_pass=[]
  for line in logfile.read_text().splitlines():
   try:row=json.loads(line)
   except ValueError:continue
   if row.get('Test'):
    item=row.get('Package','')+'/'+row['Test']
    if row.get('Action')=='pass':passed.append(item)
    if row.get('Action')=='fail':failed.append(item)
    if row.get('Action')=='skip':skipped.append(item)
   elif row.get('Action')=='pass':package_pass.append(row.get('Package'))
  report.update(passing_tests=passed,failed_tests=failed,skipped_tests=skipped,passing_packages=package_pass)
  required_by_slice={
   'core':['TestManifestAdmissionPreflightOriginalOwnerAndHistory','TestManifestAdmissionPreflightExpiryAndRestart','TestManifestAdmissionPreflightClockCapacityAndUncertainty','TestManifestAdmissionPreflightPublicCounts'],
   'service':['TestDailyFencedAdmissionExpiryBeforeIntake','TestDailyFencedAdmissionSessionAndDuplicateFences','TestDailyFencedAdmissionHistoricalAndLegacy','TestDailyFencedAdmissionErrorProvenance','TestDailyFencedAdmissionLiveCapacityReopen'],
   'worker':['TestRetainedPoolAdmissionDefiniteExpiryAndUnknownReopen'],
   'operator':['TestDailyRetainedRoundOriginalIssuerReconciles','TestDailyRetainedRoundCancellationJoinsWhileIssuerHeld','TestDailyRetainedRoundRetryClassifiesExactBusy','TestDailyRetainedRoundRetryKeepsOriginalDeadline','TestDailyRetainedAdmissionSelectionReadinessAndDirectDrain','TestDailyRetainedAdmissionFullSourceReopenPreservesPayment','TestDailyRetainedAdmissionHardEscalationAndConcurrentReadiness','TestDailyRetainedAdmissionHardCauseSurvivesSourceBusy','TestDailyRetainedAdmissionEnableErrorsKeepOriginalOwner','TestDailyRetainedAdmissionCanceledBeforeTransition']}
  if args.mode=='cheap':required_by_slice={'core':['TestManifestAdmissionPreflightCheapRefuses'],'service':['TestDailyFencedAdmissionCheapRefuses']}
  required=[n for n in required_by_slice.get(args.slice,[]) if re.search(pattern,n)] if args.mode!='compile' else []
  observed={name.rsplit('/',1)[-1] for name in passed}
  report['required_tests']=required
  if not set(required)<=observed:raise RuntimeError('missing required tests: '+str(sorted(set(required)-observed)))
  unexpected_skips=[name for name in skipped if 'Cheap' not in name.rsplit('/',1)[-1]]
  report['expected_profile_skips']=[name for name in skipped if 'Cheap' in name.rsplit('/',1)[-1]] if args.mode in ('normal','race') else []
  if failed or len(package_pass)!=len(packages) or (args.mode!='compile' and not passed) or (args.mode in ('normal','race') and unexpected_skips):raise RuntimeError('test result inventory')
 report['profile_unchanged']=bp.binding()==profile
 after=inventory();report['sources_unchanged']=before==after
 report['passed']=report['sources_unchanged'] and report['profile_unchanged']
except BaseException as exc:report['error']=type(exc).__name__+': '+str(exc)
finally:
 if before is not None:
  try:
   after=inventory();cap.support.write(out/'sources-after.json',after);report['sources_unchanged']=before==after
  except BaseException as exc:report['source_error']=str(exc);report['sources_unchanged']=False
 report['clean_join']=bool(report['commands']) and all(c.get('log_joined') is True and not c.get('log_errors') and (c.get('cleanup') or {}).get('group_empty') is True and not (c.get('cleanup') or {}).get('leftover_after_exit') and not (c.get('cleanup') or {}).get('signals') and not (c.get('cleanup') or {}).get('errors') for c in report['commands'])
 report['passed']=report['passed'] and report.get('sources_unchanged',False) and report['clean_join']
 cap.support.write(out/'report.json',report)
print(json.dumps({k:v for k,v in report.items() if k not in ('commands','passing_tests','skipped_tests')}),flush=True)
raise SystemExit(0 if report['passed'] else 1)
