#!/usr/bin/env python3
"""Portable byte-check capture; never executes Go, Argon or live services."""
import hashlib,json,os,pathlib,subprocess,sys,time
ROOT=pathlib.Path(__file__).resolve().parent
EXCLUDED={'verification.json','verification.log','freeze.json'}
def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def capture():return {str(p.relative_to(ROOT)):digest(p) for p in sorted(ROOT.rglob('*')) if p.is_file() and p.name not in EXCLUDED and '__pycache__' not in p.parts}
def main():
 before=capture();rows=[];logs=[]
 for exe in ('/usr/bin/python3','/opt/homebrew/bin/python3'):
  identity=subprocess.run([exe,'-c','import sys; print(sys.executable); print(sys.version)'],capture_output=True,text=True,check=True).stdout.strip()
  for tz in ('UTC','Pacific/Kiritimati'):
   argv=[exe,'-B',str(ROOT/'reference_entry.py'),'--check'];env=dict(os.environ,TZ=tz,PYTHONDONTWRITEBYTECODE='1');start=time.monotonic();r=subprocess.run(argv,cwd=ROOT,env=env,capture_output=True,text=True,timeout=60);seconds=time.monotonic()-start
   logs.append(json.dumps({'argv':argv,'TZ':tz,'identity':identity})+'\n'+r.stdout+r.stderr)
   rows.append(dict(argv=argv,TZ=tz,interpreter=identity,exit_code=r.returncode,seconds=round(seconds,3),stdout=r.stdout,stderr=r.stderr));assert r.returncode==0,r.stderr
 after=capture();assert before==after
 report=dict(status='passed',go_source_or_output_used_for_expected_values=False,verification_executes_go=False,new_normal_work_verifications=0,source_sha256_before=before,source_sha256_after=after,source_changes=[],commands=rows)
 log='\n'.join(logs).encode();(ROOT/'verification.log').write_bytes(log);report['log_sha256']=hashlib.sha256(log).hexdigest();(ROOT/'verification.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({'status':'passed','checks':len(rows),'source_files':len(before),'corpus_sha256':digest(ROOT/'entry_v1.json'),'verification_sha256':digest(ROOT/'verification.json')}))
if __name__=='__main__':main()
