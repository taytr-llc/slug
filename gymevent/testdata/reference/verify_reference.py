from pathlib import Path
import concurrent.futures,hashlib,json,os,subprocess,time,sys
r=Path(__file__).resolve().parent
def hashes():return {str(p.relative_to(r)):hashlib.sha256(p.read_bytes()).hexdigest()for p in sorted(r.rglob('*'))if p.is_file()and str(p.relative_to(r))not in ('verification.json','freeze.json')}
before=hashes()
def run(spec):
    exe,tz=spec;env=dict(os.environ,TZ=tz,PYTHONDONTWRITEBYTECODE='1');argv=[exe,'-B',str(r/'reference_event.py'),'--check'];start=time.monotonic();p=subprocess.run(argv,cwd=r,env=env,capture_output=True,text=True,timeout=90);v=subprocess.run([exe,'-c','import sys;print(sys.executable);print(sys.version)'],capture_output=True,text=True,check=True)
    return dict(argv=argv,tz=tz,seconds=round(time.monotonic()-start,3),exit_code=p.returncode,stdout=p.stdout,stderr=p.stderr,interpreter=v.stdout.strip())
with concurrent.futures.ThreadPoolExecutor(max_workers=4)as pool:commands=list(pool.map(run,[(x,tz)for x in('/usr/bin/python3','/opt/homebrew/bin/python3')for tz in('UTC','Pacific/Honolulu')]))
after=hashes();changes=[k for k in set(before)|set(after)if before.get(k)!=after.get(k)];status='passed'if not changes and all(x['exit_code']==0 for x in commands)else'failed';out=dict(status=status,kind='independent_reference_regeneration',source_sha256_before=before,source_sha256_after=after,source_changes=changes,commands=commands,checker_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),limitations=['No Go source/output read or comparison','No new Argon/mining/GUI/filesystem crash execution','Runtime authoring in parallel is authorized; output derivation remains independent'])
(r/'verification.json').write_text(json.dumps(out,indent=2)+'\n');print(json.dumps({'status':status,'commands':[{k:x[k]for k in('interpreter','tz','seconds','exit_code')}for x in commands],'files':len(before),'source_changes':changes}));sys.exit(status!='passed')
