from pathlib import Path
import argparse, hashlib, importlib.util, json, os, sys

BASE = Path('/Users/taytrrip/slug')
SOURCE = BASE/'set80-prep/actual-recovery'
LEGACY = BASE/'slug/desktop/pooloperator/testdata/late-winner-actual'
PINNED = {
 LEGACY/'run.py':'b81af69c02a18b7f74ed96e08ae4dba8548e94a4958aeacb721c62dcbf8ba05e',
 LEGACY/'runner_support.py':'54bb3e7caf33e744ce76bf366dd23a4eff4d370d9246b85758089b57dd2439d7',
 Path('/private/tmp/slug_set48_build_profile.py'):'b6033d66edb4d24a57a7fac4e59ddaeee6964f0943aca21fddc6f65c7f5e6ee4',
}
def identity(path):
 raw=Path(path).read_bytes();return {'bytes':len(raw),'sha256':hashlib.sha256(raw).hexdigest()}

def main():
 parser=argparse.ArgumentParser();parser.add_argument('--lock',required=True);parser.add_argument('--lock-sha256',required=True);parser.add_argument('--attempt',required=True);args=parser.parse_args()
 import re
 assert re.fullmatch(r'actual-reference-attempt[1-9][0-9]*',args.attempt)
 lock=Path(args.lock);assert lock.is_absolute() and identity(lock)['sha256']==args.lock_sha256
 selection=json.loads(lock.read_bytes());assert selection['execution_authorized'] is True and selection['approved_by']=='root'
 for path,digest in PINNED.items():assert identity(path)['sha256']==digest
 sys.dont_write_bytecode=True;sys.path[:0]=[str(LEGACY),'/private/tmp']
 spec=importlib.util.spec_from_file_location('set80_public_capture',LEGACY/'run.py');cap=importlib.util.module_from_spec(spec);spec.loader.exec_module(cap)
 out=BASE/'artifacts/set80'/args.attempt;out.mkdir(parents=True,mode=0o700,exist_ok=False)
 files={SOURCE/row['path'] for row in selection['files']}|set(PINNED)|{Path(__file__).resolve(),lock,SOURCE/'selection-lock.proposed.json'}
 interpreters={'python314':Path('/opt/homebrew/bin/python3'),'python39':Path('/Library/Developer/CommandLineTools/usr/bin/python3')}
 files.update(p.resolve() for p in interpreters.values())
 before={str(p):identity(p) for p in sorted(files)}
 cap.support.write(out/'sources-before.json',before);cap.support.write(out/'root-selection.json',selection)
 env={'PATH':'/usr/bin:/bin:/usr/sbin:/sbin','LANG':'en_US.UTF-8','PYTHONDONTWRITEBYTECODE':'1','PYTHONNOUSERSITE':'1'}
 report={'version':1,'passed':False,'scope':'Set80 independent actual wrapper selection','base_commit':selection['base_commit'],'commands':[],'outputs':{},'interpreters':{}}
 def run(label,argv,seconds):
  row=cap.capture(argv,SOURCE,env,out/(label+'.log'),seconds,log_cap=1<<20);row['label']=label;report['commands'].append(row)
  if not cap.okay(row):raise RuntimeError('failed '+label)
  print(json.dumps({'passed':label,'seconds':row['seconds']}),flush=True)
  return out/(label+'.log')
 try:
  for label,python in interpreters.items():
   ver=run(label+'-version',[str(python),'-B','-S','--version'],10).read_text().strip()
   assert ver.startswith('Python 3.14.') if label=='python314' else ver.startswith('Python 3.9.')
   report['interpreters'][label]={'version':ver,'path':str(python),'real_path':str(python.resolve()),**identity(python.resolve())}
   output=out/(label+'.json')
   log=run(label,[str(python),'-B','-S',str(SOURCE/'selection_reference.py'),'--lock',str(lock),'--output',str(output)],120)
   result=json.loads(log.read_text());actual=identity(output)
   assert result['passed'] is True and result['bytes']==actual['bytes'] and result['sha256']==actual['sha256'] and actual['bytes']<=1<<20
   report['outputs'][label]={'path':str(output),**actual,'frames':result['frames']}
  assert (out/'python314.json').read_bytes()==(out/'python39.json').read_bytes()
  report['byte_identical']=True;report['passed']=True
 except BaseException as exc:report['error']=type(exc).__name__+': '+str(exc)
 finally:
  try:
   after={str(p):identity(p) for p in sorted(files)};cap.support.write(out/'sources-after.json',after);report['sources_unchanged']=before==after
   report['lock_unchanged']=identity(lock)['sha256']==args.lock_sha256
  except BaseException as exc:report['binding_error']=type(exc).__name__+': '+str(exc)
  report['clean_join']=bool(report['commands']) and all(c.get('log_joined') is True and not c.get('log_errors') and (c.get('cleanup') or {}).get('group_empty') is True and not (c.get('cleanup') or {}).get('leftover_after_exit') and not (c.get('cleanup') or {}).get('signals') and not (c.get('cleanup') or {}).get('errors') for c in report['commands'])
  report['passed']=report['passed'] and report.get('sources_unchanged',False) and report.get('lock_unchanged',False) and report['clean_join']
  cap.support.write(out/'report.json',report)
 print(json.dumps({k:v for k,v in report.items() if k not in ('commands','outputs','interpreters')}),flush=True)
 return 0 if report['passed'] else 1
if __name__=='__main__':raise SystemExit(main())
