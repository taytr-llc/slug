#!/usr/bin/env python3
"""Bounded local execution of the root-approved independent public producer."""
import hashlib,json,os,resource,signal,subprocess,time
from pathlib import Path
W=Path('/Users/taytrrip/slug')
P=W/'set74-prep/operator-provision/reference'
A=W/'artifacts/set74'
h=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
proposed=P/'producer-lock.proposed.json'
assert h(proposed)=='0e276671cdab0d926bf356f8918e94fd6d8835a0f9a27a423a3d7c1e11cf0bb6'
lock=json.loads(proposed.read_bytes());assert lock['approved_by'] is None
lock['approved_by']='root'
approved=P/'producer-lock.approved.json'
with approved.open('x') as f:f.write(json.dumps(lock,indent=2)+'\n')
lock_sha=h(approved)
def current():
 return {r['path']:{'bytes':(P/r['path']).stat().st_size,'sha256':h(P/r['path'])} for r in lock['inputs']}
def limits():
 resource.setrlimit(resource.RLIMIT_CPU,(90,90))
 resource.setrlimit(resource.RLIMIT_FSIZE,(3<<20,3<<20))
 resource.setrlimit(resource.RLIMIT_CORE,(0,0))
runs=[]
for label,python in [('py314','/opt/homebrew/opt/python@3.14/bin/python3.14'),('py39','/usr/bin/python3')]:
 path=A/('provision-reference-'+label+'-attempt1');path.mkdir()
 before=current();expected={r['path']:{'bytes':r['bytes'],'sha256':r['sha256']} for r in lock['inputs']};assert before==expected
 (path/'inputs-before.json').write_text(json.dumps(before,indent=2)+'\n')
 version=subprocess.check_output([python,'--version'],timeout=5,text=True).strip()
 assert ('3.14.' in version if label=='py314' else '3.9.' in version)
 output=path/'provision-v1.json';start=time.monotonic();error=None;forced=False
 cmd=[python,str(P/'generate.py'),'--execute','--lock',str(approved),'--lock-sha',lock_sha,'--output',str(output)]
 with (path/'producer.log').open('xb') as log:
  process=subprocess.Popen(cmd,cwd=path,env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'},stdout=log,stderr=subprocess.STDOUT,preexec_fn=limits,start_new_session=True)
  try:code=process.wait(timeout=120)
  except subprocess.TimeoutExpired:
   forced=True;os.killpg(process.pid,signal.SIGKILL);code=process.wait(timeout=5);error='deadline'
 elapsed=time.monotonic()-start
 try:os.killpg(process.pid,0);empty=False
 except ProcessLookupError:empty=True
 except PermissionError:empty=False
 after=current();(path/'inputs-after.json').write_text(json.dumps(after,indent=2)+'\n')
 counts=None;sha=None;size=None
 if output.exists():
  size=output.stat().st_size
  if size<=lock['output_max_bytes']:
   value=json.loads(output.read_bytes());counts=value['counts'];sha=h(output)
 passed=code==0 and error is None and not forced and empty and before==after and h(approved)==lock_sha and counts==lock['expected_counts'] and size is not None and size<=lock['output_max_bytes'] and (path/'producer.log').stat().st_size<=65536
 report=dict(passed=passed,interpreter=python,version=version,interpreter_sha256=h(Path(python)),command=cmd,returncode=code,elapsed_seconds=round(elapsed,3),error=error,forced=forced,group_empty=empty,inputs_unchanged=before==after,lock_sha256=lock_sha,lock_unchanged=h(approved)==lock_sha,output=str(output),bytes=size,sha256=sha,counts=counts,scope='independent published synthetic bytes; no Go/network/private creation')
 (path/'report.json').write_text(json.dumps(report,indent=2)+'\n');runs.append(report)
 print(json.dumps(report,separators=(',',':')),flush=True)
 if not passed:break
same=len(runs)==2 and all(r['passed'] for r in runs) and Path(runs[0]['output']).read_bytes()==Path(runs[1]['output']).read_bytes()
record=dict(passed=same,byte_identical=same,runs=runs,producer_lock_sha256=lock_sha,runner_sha256=h(Path(__file__)))
(A/'provision-reference-reproduction.json').write_text(json.dumps(record,indent=2)+'\n')
if not same:raise SystemExit(1)
