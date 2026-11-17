#!/usr/bin/env python3
import datetime,hashlib,json,os,pathlib,subprocess,time
P=pathlib.Path(__file__).resolve().parent
names=['reference_bout.py','generate_corpus.py','verify_reference.py','select_public_cases.py','select_symmetric_cases.py','input_hashes.json','selected_public_cases.json','search_unequal.json','search_balanced.json','bout_v1.json','README.md','capture_corpus.py']
def hashes():return {str(p.relative_to(P)):hashlib.sha256(p.read_bytes()).hexdigest() for p in [*(P/n for n in names),*sorted((P/'inputs').rglob('*'))] if p.is_file()}
before=hashes();report={'status':'running','scope':'independent normative corpus freeze only','started_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'commands':[],'files':before,'go_source_or_output_read':False,'calibration_status':'not_run','requires_root_release_before_go_execution':True}
log=P/'verification.log';log.write_text('')
for executable,seed,zone in [('/usr/bin/python3','0','UTC'),('/opt/homebrew/bin/python3','87123','America/Los_Angeles')]:
 env=os.environ.copy();env.pop('PYTHONPATH',None);env.update(PYTHONDONTWRITEBYTECODE='1',PYTHONHASHSEED=seed,TZ=zone)
 command=[executable,'-B','-S',str(P/'verify_reference.py')];start=time.monotonic();r=subprocess.run(command,cwd=P,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=120)
 with log.open('a') as f:f.write(json.dumps(command)+'\n'+r.stdout+'\n')
 item={'command':command,'hash_seed':seed,'timezone':zone,'elapsed_seconds':time.monotonic()-start,'exit_code':r.returncode};report['commands'].append(item);print(json.dumps(item),flush=True)
 if r.returncode:report['status']='failed';break
else:report['status']='frozen'
after=hashes();report['source_changes']=[k for k in set(before)|set(after) if before.get(k)!=after.get(k)];assert not report['source_changes']
report['completed_at']=datetime.datetime.now(datetime.timezone.utc).isoformat();report['log_sha256']=hashlib.sha256(log.read_bytes()).hexdigest()
(P/'corpus-freeze.json').write_text(json.dumps(report,indent=2)+'\n')
if report['status']!='frozen':raise SystemExit(1)
print(json.dumps({'status':'frozen','corpus':before['bout_v1.json'],'source':before['reference_bout.py'],'files':len(before)}))
