"""Capture joined independent-only checks; no new expected values or Go execution."""
from pathlib import Path
import hashlib,json,os,subprocess,time,sys
HERE=Path(__file__).resolve().parent
sha=lambda b:hashlib.sha256(b).hexdigest()
def capture():return {str(p.relative_to(HERE)):sha(p.read_bytes()) for p in sorted(HERE.rglob('*')) if p.is_file() and p.name not in ['verification.json','verification.log','file_index.json','freeze.json'] and '__pycache__' not in p.parts}
before=capture();runs=[];logs=[]
for python,tz in [('/opt/homebrew/bin/python3.13','UTC'),('/opt/homebrew/bin/python3.13','Pacific/Honolulu'),('/opt/homebrew/bin/python3.14','UTC'),('/opt/homebrew/bin/python3.14','Pacific/Honolulu')]:
 env=dict(os.environ,TZ=tz,PYTHONPATH='',PYTHONDONTWRITEBYTECODE='1');start=time.monotonic();r=subprocess.run([python,'-B','verify_reference.py'],cwd=HERE,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=60);logs.append('PYTHON='+python+' TZ='+tz+'\n'+r.stdout);runs.append(dict(argv=[python,'-B','verify_reference.py'],timezone=tz,exit_code=r.returncode,seconds=round(time.monotonic()-start,3),joined=True,result=json.loads(r.stdout) if r.returncode==0 else None));assert r.returncode==0,r.stdout
log='\n'.join(logs).encode();(HERE/'verification.log').write_bytes(log);after=capture();changed=[p for p in set(before)|set(after) if before.get(p)!=after.get(p)];assert not changed,changed
result=dict(version=1,status='passed',scope='independent Python only; no Set46 runtime read or comparison',source_sha256_before=before,source_sha256_after=after,source_changes=changed,runs=runs,log_sha256=sha(log),corpus_sha256=sha((HERE/'card_v1.json').read_bytes()))
(HERE/'verification.json').write_text(json.dumps(result,indent=2)+'\n');print('passed',result['corpus_sha256'],'verification',sha((HERE/'verification.json').read_bytes()))
