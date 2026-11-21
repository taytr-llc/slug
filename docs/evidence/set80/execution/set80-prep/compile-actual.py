"""Bounded root-only source compilation; never starts the actual fixture."""
from pathlib import Path
import argparse,hashlib,importlib.util,json,os,re,shutil,sys
BASE=Path('/Users/taytrrip/slug');ROOT=BASE/'slug';HERE=BASE/'set80-prep/actual-recovery';LEGACY=BASE/'set79-prep/public-daily-headless'
p=argparse.ArgumentParser();p.add_argument('--attempt',required=True);p.add_argument('--manifest-sha256',required=True);a=p.parse_args()
assert re.fullmatch('actual-helper-compile-attempt[1-9][0-9]*',a.attempt)
manifest=HERE/'source-manifest.json';assert hashlib.sha256(manifest.read_bytes()).hexdigest()==a.manifest_sha256
m=json.loads(manifest.read_bytes())
for r in m['files']:
 raw=(HERE/r['path']).read_bytes();assert len(raw)==r['bytes'] and hashlib.sha256(raw).hexdigest()==r['sha256']
sys.dont_write_bytecode=True;sys.path[:0]=[str(LEGACY),'/private/tmp']
def load(name,path):
 s=importlib.util.spec_from_file_location(name,path);v=importlib.util.module_from_spec(s);s.loader.exec_module(v);return v
old=load('set80_compile_capture',LEGACY/'run.py');bp=load('set80_compile_profile','/private/tmp/slug_set48_build_profile.py')
out=BASE/'artifacts/set80'/a.attempt;out.mkdir(mode=0o700);probe=out/'source';probe.mkdir(mode=0o700)
files={HERE/r['path'] for r in m['files']}|{manifest,Path(__file__).resolve(),LEGACY/'run.py',LEGACY/'runner_support.py',Path('/private/tmp/slug_set48_build_profile.py'),ROOT/'scripts/candidate-go.sh',ROOT/'build-profile.json'}
for name in m['build_files']:
 target=probe/name;shutil.copyfile(HERE/name,target);files.add(target)
shutil.copyfile(ROOT/'desktop/go.sum',probe/'go.sum');files.add(probe/'go.sum')
modules=[probe,ROOT/'desktop',BASE/'slugwallet']+[BASE/'slugwallet'/n for n in ('wallet/txauthor','wallet/txrules','wallet/txsizes','walletdb','wtxmgr')]+[ROOT]+[ROOT/n for n in ('address','btcec','btcutil','chaincfg','chainhash','psbt','txscript','v2transport','wire')]
work=out/'go.work';work.write_text('go 1.27.4\nuse (\n'+''.join('\t'+json.dumps(str(p))+'\n' for p in modules)+')\n');files.add(work)
env=bp.environment(GOCACHE='/private/tmp/slug-go-build',GOMODCACHE=str(BASE/'build-cache/go-mod'),GOPROXY='off',GOSUMDB='off',GOMAXPROCS='2',CGO_ENABLED='1',GOWORK=str(work))
report=dict(version=1,scope='Set80 actual helper compile only; no runtime acceptance',passed=False,actual_started=False,commands=[])
before={str(p):old.support.identity(p) for p in sorted(files)};profile=bp.binding();old.support.write(out/'sources-initial.json',before);old.support.write(out/'profile.json',profile)
def run(name,args,secs):
 print(json.dumps({'starting':name}),flush=True);row=old.capture(args,probe,env,out/(name+'.log'),secs,log_cap=32<<20);report['commands'].append(row)
 if not old.okay(row):raise RuntimeError('failed '+name)
 return out/(name+'.log')
try:
 log=run('deps',['go','list','-deps','-json','-mod=readonly','.'],120);files.update(old.dependency_files(log))
 if work.with_suffix('.work.sum').exists():files.add(work.with_suffix('.work.sum'))
 for p in sorted(files):
  value=old.support.identity(p)
  if str(p) in before:assert before[str(p)]==value
  before[str(p)]=value
 old.support.write(out/'sources-before.json',before)
 target=out/'issue-recovery-actual';run('build',bp.command('build','-p=1','-mod=readonly','-trimpath','-tags=','-o',target,'.'),240)
 report['binary']=bp.artifact(target,out/'build-info.txt',tags='',cgo=1);report['passed']=True
except BaseException as exc:report['error']=type(exc).__name__+': '+str(exc)
finally:
 after={p:old.support.identity(p) for p in before};old.support.write(out/'sources-after.json',after);report['sources_unchanged']=before==after;report['profile_unchanged']=bp.binding()==profile
 report['clean_join']=all(c.get('log_joined') is True and not c.get('capture_errors') and not c.get('log_errors') and (c.get('cleanup') or {}).get('group_empty') is True and not (c.get('cleanup') or {}).get('leftover_after_exit') and not (c.get('cleanup') or {}).get('signals') for c in report['commands'])
 report['passed']=report['passed'] and report['sources_unchanged'] and report['profile_unchanged'] and report['clean_join'];old.support.write(out/'report.json',report)
print(json.dumps({k:v for k,v in report.items() if k not in ('commands','binary')}),flush=True)
raise SystemExit(0 if report['passed'] else 1)
