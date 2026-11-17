import os,json,time,subprocess,hashlib,sys
from pathlib import Path
R=Path('/Users/taytrrip/slug/set45-seed-reference');V=R/'verification';V.mkdir(exist_ok=False)
def h(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def snapshot():return {str(p.relative_to(R)):h(p)for p in sorted(R.rglob('*'))if p.is_file()and 'verification'not in p.relative_to(R).parts and p.name not in ['verification.json','freeze.json']}
before=snapshot();commands=[];status='passed'
def run(argv,tz='UTC',timeout=240):
 global status
 env=dict(os.environ,TZ=tz,PYTHONDONTWRITEBYTECODE='1',GOWORK='off',GOPROXY='off',GOCACHE='/private/tmp/slug-go-build',GOMODCACHE='/private/tmp/slug-desktop-modcache',GOMAXPROCS='4');start=time.monotonic();name='%02d.log'%(len(commands)+1)
 try:
  p=subprocess.run(argv,cwd=R,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=timeout);code=p.returncode;out=p.stdout;timed=False
 except subprocess.TimeoutExpired as e:code=-1;out=e.stdout or b'';timed=True
 (V/name).write_bytes(out);row=dict(argv=argv,timezone=tz,exit_code=code,timed_out=timed,seconds=round(time.monotonic()-start,3),log='verification/'+name,log_sha256=h(V/name));commands.append(row);print(json.dumps(row),flush=True)
 if code:status='failed'
 return code
py14='/opt/homebrew/opt/python@3.14/bin/python3.14';py39='/usr/bin/python3'
run([py14,'--version']);run([py39,'--version']);run(['go','version'])
run([py14,'-B','crypto.py'])
run([py14,'-B','reference_blocks.py'])
run([py14,'-B','reference_seed.py','--check'],'UTC')
run([py14,'-B','verify_reference.py'],'UTC')
run([py39,'-B','reference_seed.py','--check'],'America/Los_Angeles')
run([py39,'-B','verify_reference.py'],'America/Los_Angeles')
run([py14,'-B','reference_seed.py','--check'],'America/Los_Angeles')
run([py39,'-B','reference_seed.py','--check'],'UTC')
run([py14,'-B','reference_blocks.py','--verify-work'],timeout=600)
after=snapshot();drift=[p for p in sorted(set(before)|set(after))if before.get(p)!=after.get(p)]
if drift:status='failed'
report=dict(status=status,scope='independent reference regeneration/crypto/wire checks only; no45 runtime source/output or comparison',source_sha256_before=before,source_sha256_after=after,source_changes=drift,commands=commands,argon_binary_sha256=h(Path('/private/tmp/slug-set45-argon-reference')),runner_sha256=h(Path(__file__)),normal_work_claim='standalone literal production Argon parameters; fixture bodies are not node-accepted consensus evidence')
(R/'verification.json').write_text(json.dumps(report,indent=2)+'\n');print('FINAL',status,h(R/'verification.json'),flush=True)
raise SystemExit(0 if status=='passed'else 1)
