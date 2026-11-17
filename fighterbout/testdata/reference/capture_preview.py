import hashlib,json,os,pathlib,shutil,subprocess,time
r=pathlib.Path(__file__).resolve().parent
names=['generate_preview.py','capture_preview.py','preview_transport_v1.json','preview_inputs/stable_v1.json','preview_inputs/portraits_v1.json']
def hashes():return {p:hashlib.sha256((r/p).read_bytes()).hexdigest() for p in names}
before=hashes();commands=[]
with (r/'preview-verification.log').open('w') as log:
 for exe in ('/usr/bin/python3','/opt/homebrew/bin/python3'):
  cmd=[exe,'-B','-S',str(r/'generate_preview.py'),'--check'];start=time.monotonic();p=subprocess.run(cmd,capture_output=True,text=True,timeout=30,env=dict(os.environ,PYTHONHASHSEED='19',TZ='UTC'));log.write(json.dumps(cmd)+'\n'+p.stdout+p.stderr);commands.append({'command':cmd,'seconds':time.monotonic()-start,'exit_code':p.returncode});assert p.returncode==0
assert before==hashes()
report={'status':'frozen','scope':'synthetic transport bindings only, not admitted proof evidence','source_sha256':before,'commands':commands,'source_changes':[],'pre_freeze_correction':'Initial generator asserted the stream domain rather than the existing manifest DIGEST_DOMAIN; assertion failed before creating any output. Corrected to the immutable independent digest_manifest helper. No expected output or model change.'}
(r/'preview-freeze.json').write_text(json.dumps(report,indent=2)+'\n')
d=pathlib.Path('/Users/taytrrip/slug/set42-fight-stage/fighterbout/testdata/reference')
for name in names+['preview-verification.log','preview-freeze.json']:
 t=d/name;t.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(r/name,t);assert hashlib.sha256(t.read_bytes()).hexdigest()==hashlib.sha256((r/name).read_bytes()).hexdigest()
print(json.dumps(report,indent=2))
