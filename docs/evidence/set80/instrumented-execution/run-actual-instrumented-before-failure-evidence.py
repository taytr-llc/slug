"""Root-owned bounded Set80 A/B qualification. No import-time execution."""
from pathlib import Path
import argparse,hashlib,importlib.util,json,os,re,shutil,sys,stat
BASE=Path('/Users/taytrrip/slug');REPO=BASE/'slug';WALLET=BASE/'slugwallet'
HERE=BASE/'set80-prep/actual-recovery-instrumented'
LEGACY=BASE/'set79-prep/public-daily-headless'
PROFILE=Path('/private/tmp/slug_set48_build_profile.py')
REFERENCE=BASE/'artifacts/set80/actual-reference-attempt1/python314.json'
OLD_REFERENCE=HERE/'inputs/path-c-reference.json'
BASE_REFERENCE=HERE/'inputs/base-reference.json'
CHECKER=BASE/'set80-prep/check-actual-lookup.py'
SCOPE='set80-exact-issuance-recovery-actual'

def identity(p):
 path=Path(p)
 for part in (path,*path.parents):
  if stat.S_ISLNK(part.lstat().st_mode):raise RuntimeError('symlink source refused')
 before=path.stat();need(stat.S_ISREG(before.st_mode),'ordinary source required')
 h=hashlib.sha256();size=0
 fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW)
 with os.fdopen(fd,'rb') as stream:
  opened=os.fstat(stream.fileno());need(os.path.samestat(before,opened),'source replaced on open')
  while block:=stream.read(1<<20):h.update(block);size+=len(block)
  after=os.fstat(stream.fileno())
 need(os.path.samestat(before,path.stat()) and before.st_size==size==after.st_size and before.st_mtime_ns==after.st_mtime_ns,'source changed while hashing')
 return dict(bytes=size,sha256=h.hexdigest())
def need(v,why):
 if not v:raise RuntimeError(why)
def load_module(name,path):
 spec=importlib.util.spec_from_file_location(name,path);mod=importlib.util.module_from_spec(spec);spec.loader.exec_module(mod);return mod

def execute(args):
 lock=Path(args.lock);need(identity(lock)['sha256']==args.lock_sha256,'root lock bytes')
 selection=json.loads(lock.read_bytes());need(selection.get('approved_by')=='root' and selection.get('execution_authorized') is True and selection.get('scope')==SCOPE and selection.get('case')==args.case,'root exact case admission')
 need(selection.get('scenario_seconds')==900 and selection.get('outer_seconds')==1080,'finite original deadlines')
 files=selection['files'];need(isinstance(files,dict) and 20<=len(files)<=256,'finite root selection')
 for path,want in files.items():need(Path(path).is_absolute() and identity(path)==want,'locked source changed')
 need(identity(REFERENCE)==dict(bytes=173644,sha256='0f0890e529b64c10ed000673aab7861b9bd526057cc3c2bbbe05b868028f3d10'),'qualified actual reference')
 required={Path(__file__).resolve(),LEGACY/'run.py',LEGACY/'runner_support.py',PROFILE,REFERENCE,OLD_REFERENCE,BASE_REFERENCE,HERE/'fixture-catalog.json',HERE/'source-manifest.json',CHECKER,REPO/'build-profile.json',REPO/'scripts/candidate-go.sh'}
 need(all(str(p) in files for p in required),'required source omitted')
 sys.dont_write_bytecode=True;sys.path[:0]=[str(LEGACY),str(PROFILE.parent)]
 old=load_module('set80_original_capture',LEGACY/'run.py');bp=load_module('set80_actual_profile',PROFILE)
 out=BASE/'artifacts/set80'/args.attempt;out.mkdir(mode=0o700,exist_ok=False)
 report=dict(version=1,scope=SCOPE,case=args.case,passed=False,actual_started=False,commands=[],scenario_seconds=900,outer_seconds=1080,maximum_chain_height=180)
 before=None;catalog=None;mapping={};built={};bound=None;owned=None
 source_files=set(map(Path,files))|{lock}
 def run(label,argv,cwd,env,seconds,logcap=32<<20):
  print(json.dumps(dict(starting=label,case=args.case)),flush=True)
  row=old.capture(argv,cwd,env,out/(label+'.log'),seconds,permitted=[v['path'] for v in built.values()],log_cap=logcap);row['label']=label;report['commands'].append(row)
  need(old.okay(row),'command failed: '+label)
  print(json.dumps(dict(passed=label,seconds=row['seconds'])),flush=True);return out/(label+'.log'),row
 try:
  old.support.write(out/'execution-lock.json',selection)
  catalog=old.read_json(HERE/'fixture-catalog.json',1<<20);old.fixture_original(catalog)
  bound=bp.binding();old.support.write(out/'profile.json',bound)
  source=old.read_json(HERE/'source-manifest.json',1<<20)
  leaves=source['build_files'];need(isinstance(leaves,list) and 8<=len(leaves)<=40 and 'go.mod' in leaves,'finite helper build inventory')
  probe=out/'probe-source';probe.mkdir(mode=0o700)
  for name in leaves:
   need(Path(name).name==name and (name.endswith('.go') or name=='go.mod'),'flat build source')
   p=HERE/name;need(str(p) in files,'build input not admitted');q=probe/name;shutil.copyfile(p,q);os.chmod(q,0o600);mapping[str(q)]={'source':str(p),**identity(p)}
  p=REPO/'desktop/go.sum';q=probe/'go.sum';shutil.copyfile(p,q);mapping[str(q)]={'source':str(p),**identity(p)}
  old.support.write(out/'source-copies.json',mapping)
  modules=[probe,REPO/'desktop',WALLET,WALLET/'wallet/txauthor',WALLET/'wallet/txrules',WALLET/'wallet/txsizes',WALLET/'walletdb',WALLET/'wtxmgr',REPO]
  modules += [REPO/n for n in ('address','btcec','btcutil','chaincfg','chainhash','psbt','txscript','v2transport','wire')]
  work=out/'go.work';work.write_text('go 1.27.4\nuse (\n'+''.join('\t'+json.dumps(str(p))+'\n' for p in modules)+')\n')
  env=bp.environment(GOCACHE='/private/tmp/slug-go-build',GOMODCACHE=str(BASE/'build-cache/go-mod'),GOPROXY='off',GOSUMDB='off',GOWORK=str(work),GOMAXPROCS='2',CGO_ENABLED='1')
  env={k:v for k,v in env.items() if not k.startswith('SLUG_')};env.update(PYTHONDONTWRITEBYTECODE='1',GOOS='darwin',GOARCH='arm64');node_env=dict(env,CGO_ENABLED='0')
  source_files.update(map(Path,mapping));source_files.add(work)
  targets=[('node','slugd',REPO,node_env,0,'.'),('operator','slugpoold',REPO/'desktop',env,1,'./cmd/slugpoold'),('helper','slugpilot',REPO/'desktop',env,1,'./cmd/slugpilot'),('probe','issue-recovery-actual',probe,env,1,'.')]
  for name,filename,cwd,e,cgo,target in targets:
   log,_=run(name+'-deps',['go','list','-deps','-json','-mod=readonly',target],cwd,e,120)
   source_files.update(old.dependency_files(log))
   if name in ('operator','helper','probe'):need('github.com/taytr-llc/slugwallet/walletdb/bdb' in log.read_text(),'original real-wallet driver absent')
  if work.with_suffix('.work.sum').exists():source_files.add(work.with_suffix('.work.sum'))
  before={str(p):identity(p) for p in sorted(source_files)};old.support.write(out/'sources-before.json',before)
  need(old.check_copies(mapping),'source copies changed')
  need(identity(lock)['sha256']==args.lock_sha256 and all(identity(p)==value for p,value in files.items()),'admission changed before build')
  bindir=out/'bin';bindir.mkdir(mode=0o700)
  for name,filename,cwd,e,cgo,target in targets:
   binary=bindir/filename
   run(name+'-build',bp.command('build','-p=1','-mod=readonly','-trimpath','-tags=','-o',binary,target),cwd,e,240)
   built[name]=bp.artifact(binary,out/(name+'-build-info.txt'),tags='',cgo=cgo)
  need(all(identity(p)==value for p,value in before.items()),'sources changed before runtime');bp.same(bound);bp.unchanged(built)
  need(identity(lock)['sha256']==args.lock_sha256 and all(identity(p)==value for p,value in files.items()),'admission changed before runtime')
  owned=old.prepare_fixture(out,catalog)
  argv=[built['probe']['path'],'--case',args.case,'--node',built['node']['path'],'--operator',built['operator']['path'],'--helper',built['helper']['path'],'--artifacts',str(owned),'--reference',str(REFERENCE),'--old-reference',str(OLD_REFERENCE),'--base-reference',str(BASE_REFERENCE)]
  report['actual_started']=True
  _,command=run('actual',argv,probe,env,1080)
  # The independent root checker is a separately locked source, installed only
  # once the helper's final public evidence schema has been reviewed.
  checker=load_module('set80_actual_checker',CHECKER)
  report['public_evidence']=checker.check(owned,args.case,command['pid'])
  report['passed']=True
 except BaseException as exc:report['error']=old.error('root_runner_or_actual',exc)
 finally:
  report['artifacts']=built
  try:
   evidence=report.get('public_evidence')
   if evidence is not None:
    need(isinstance(evidence,dict) and 1<=len(evidence)<=256,'bounded public evidence inventory')
    for path,want in evidence.items():
     p=Path(path)
     need(owned is not None and p.is_absolute() and p.is_relative_to(owned),'public evidence outside original fixture')
     need(identity(p)==want,'checked public evidence changed')
    report['public_evidence_unchanged']=True
   elif report['actual_started']:
    report['public_evidence_unchanged']=False
    p=owned/'actual-report.json'
    if p.is_file():
     value=identity(p);need(value['bytes']<=2<<20,'oversized actual failure report')
     report['actual_failure_report']={'path':str(p),**value}
  except BaseException as exc:
   report['public_evidence_unchanged']=False;report['public_evidence_error']=old.error('public_evidence_finalization',exc);report['passed']=False
  if before is not None:
   try:
    after={p:identity(p) for p in before};old.support.write(out/'sources-after.json',after);report['sources_unchanged']=before==after
   except BaseException as exc:report['binding_error']=old.error('source_finalization',exc);report['sources_unchanged']=False
  try:
   need(identity(lock)['sha256']==args.lock_sha256,'root lock changed');need(all(identity(p)==v for p,v in files.items()),'admitted inputs changed');need(old.check_copies(mapping),'helper copies changed');bp.same(bound);bp.unchanged(built);report['profile_lock_binaries_unchanged']=True
  except BaseException as exc:report['profile_lock_binaries_unchanged']=False;report['final_binding_error']=old.error('final_binding',exc)
  try:
   need(catalog is not None,'missing fixture catalog');report['fixture_preservation']=old.fixture_after(out,catalog)
   if report['actual_started']:need(report['fixture_preservation'].get('old_histories_unchanged') is True,'original wallet histories changed')
  except BaseException as exc:report['fixture_error']=old.error('fixture_finalization',exc);report['passed']=False
  report['clean_join']=bool(report['commands']) and all(c.get('log_joined') is True and not c.get('log_errors') and not c.get('capture_errors') and (c.get('cleanup') or {}).get('group_empty') is True and (c.get('cleanup') or {}).get('leftover_after_exit') is False and not (c.get('cleanup') or {}).get('signals') and not (c.get('cleanup') or {}).get('errors') for c in report['commands'])
  report['passed']=report['passed'] and report.get('sources_unchanged',False) and report.get('profile_lock_binaries_unchanged',False) and report['clean_join'] and report.get('public_evidence_unchanged',False) and all(old.okay(c) for c in report['commands'])
  old.support.write(out/'report.json',report)
 print(json.dumps({k:v for k,v in report.items() if k not in ('commands','artifacts','public_evidence')}),flush=True)
 return 0 if report['passed'] else 1
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--case',choices=['A','B'],required=True);p.add_argument('--attempt',required=True);p.add_argument('--lock',required=True);p.add_argument('--lock-sha256',required=True);a=p.parse_args()
 need(re.fullmatch('issue-recovery-[ab]-actual-attempt[1-9][0-9]*',a.attempt),'fresh attempt name')
 raise SystemExit(execute(a))
