#!/usr/bin/env python3
"""Serial dual-Python checks; preserves reports and never invokes Go/node."""
import argparse,hashlib,json,os,pathlib,subprocess,sys
sys.dont_write_bytecode=True
HERE=pathlib.Path(__file__).resolve().parent
H=lambda b:hashlib.sha256(b).hexdigest()
def main():
 a=argparse.ArgumentParser();a.add_argument('--output',required=True);args=a.parse_args();out=pathlib.Path(args.output).resolve();out.mkdir(exist_ok=False,parents=True)
 corpus=H((HERE/'corpus.json').read_bytes());sources={p.name:H(p.read_bytes())for p in sorted(HERE.glob('*.py'))};runs=[]
 for label,exe,seed,tz in [('system','/usr/bin/python3','17','UTC'),('homebrew','/opt/homebrew/bin/python3','483','America/Los_Angeles')]:
  report=out/(label+'.json');log=out/(label+'.log');env=dict(os.environ,PYTHONDONTWRITEBYTECODE='1',PYTHONHASHSEED=seed,TZ=tz);cmd=[exe,'-B',str(HERE/'verify_reference.py'),'--check','--report',str(report)]
  with log.open('wb')as f:p=subprocess.run(cmd,env=env,cwd=HERE,stdout=f,stderr=subprocess.STDOUT,timeout=120)
  entry=dict(label=label,command=cmd,hash_seed=seed,timezone=tz,exit_code=p.returncode,log_sha256=H(log.read_bytes()))
  if report.exists():entry.update(report_sha256=H(report.read_bytes()));r=json.loads(report.read_bytes());assert r['status']=='passed'and r['corpus_sha256']==corpus
  runs.append(entry)
  if p.returncode:break
 status='passed'if len(runs)==2 and all(x['exit_code']==0 for x in runs)else'failed'
 after={p.name:H(p.read_bytes())for p in sorted(HERE.glob('*.py'))};assert after==sources and H((HERE/'corpus.json').read_bytes())==corpus
 result=dict(status=status,findings=[]if status=='passed'else['Preserved failed dual-Python attempt; inspect per-run log.'],corpus_sha256=corpus,producer_sha256=sources,runs=runs,source_unchanged=True,scope='Independent standard-library Python only. No new Go/node/normal Argon execution.')
 (out/'report.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(dict(status=status,report=str(out/'report.json'),sha256=H((out/'report.json').read_bytes()))));return 0 if status=='passed'else 1
if __name__=='__main__':sys.exit(main())
