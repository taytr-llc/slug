#!/usr/bin/env python3
"""Two-interpreter author reproduction, exclusive outputs and exact source guards."""
import argparse,hashlib,json,os,pathlib,subprocess,time
ROOT=pathlib.Path(__file__).resolve().parent
SOURCES=['common.py','schema.py','generate.py','state_models.py','verify_reference.py','check_reproducibility.py','README.md','input_index.json']
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def capture():
 index=json.loads((ROOT/'input_index.json').read_bytes());names=SOURCES+list(index['inputs'])
 return {name:sha(ROOT/name)for name in sorted(names)}
def run(cmd,env,out,label):
 t=time.monotonic();r=subprocess.run(cmd,cwd=ROOT,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=120)
 log=out/(label+'.log')
 with log.open('xb')as f:f.write(r.stdout)
 result=dict(command=cmd,environment={k:env[k]for k in ['PYTHONDONTWRITEBYTECODE','PYTHONHASHSEED','TZ']},exit_code=r.returncode,seconds=round(time.monotonic()-t,6),log=str(log),log_sha256=sha(log))
 assert r.returncode==0,(label,result)
 return result

def main():
 a=argparse.ArgumentParser();a.add_argument('--out',type=pathlib.Path,required=True);a.add_argument('--legacy',type=pathlib.Path);v=a.parse_args();v.out.mkdir(exist_ok=False,parents=True);before=capture();commands=[];outfiles=[]
 for i,(exe,seed,tz)in enumerate([('/usr/bin/python3','7','UTC'),('/opt/homebrew/bin/python3','113','Pacific/Auckland')],1):
  env=dict(os.environ,PYTHONDONTWRITEBYTECODE='1',PYTHONHASHSEED=seed,TZ=tz);path=v.out/('corpus-python'+str(i)+'.json');outfiles.append(path)
  version=subprocess.run([exe,'--version'],check=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT).stdout.decode().strip()
  command=run([exe,'-B','-S','generate.py','--write',str(path)],env,v.out,'generate-python'+str(i));command['python_version']=version;commands.append(command)
  commands.append(run([exe,'-B','-S','verify_reference.py',str(path)],env,v.out,'verify-python'+str(i)))
 assert outfiles[0].read_bytes()==outfiles[1].read_bytes(),'complete bytes differ'
 after=capture();assert before==after,'source/input drift';legacy=None
 if v.legacy:
  pins=json.loads(v.legacy.read_bytes());source=pathlib.Path('/Users/taytrrip/slug/slug')
  for name,pin in pins['files'].items():
   p=source/name;assert p.is_file()and not p.is_symlink()and p.stat().st_size==pin['bytes']and sha(p)==pin['sha256'],name
  legacy=dict(index_path=str(v.legacy),index_sha256=sha(v.legacy),files=len(pins['files']),all_unchanged=True)
 corpus=json.loads(outfiles[0].read_bytes());report=dict(version=1,status='passed',scope='Author two-Python reference reproduction; no Go/Argon/journal/network/runtime execution or independent-review claim',commands=commands,source_and_input_sha256=before,source_and_input_unchanged=True,outputs={str(p):dict(sha256=sha(p),bytes=p.stat().st_size)for p in outfiles},identical_complete_output=True,counts={k:len(corpus[k])for k in ['compositions','dtos','codec_cases','protocol_cases','portrait_protocol_cases','owner_cases','state_cases']},legacy_preservation=legacy)
 reportpath=v.out/'report.json'
 with reportpath.open('x')as f:json.dump(report,f,indent=2);f.write('\n')
 print(json.dumps(dict(status='passed',report=str(reportpath),report_sha256=sha(reportpath),corpus_sha256=sha(outfiles[0]),counts=report['counts'])))
if __name__=='__main__':main()
