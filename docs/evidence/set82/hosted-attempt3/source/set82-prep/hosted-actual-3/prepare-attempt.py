#!/usr/bin/env python3
"""Select one immutable hosted fixture clock and run original offline setup."""
import base64, datetime, hashlib, importlib.util, json, os, shlex, shutil, subprocess, time
from pathlib import Path
B=Path(__file__).resolve().parent
A=Path('/Users/taytrrip/slug/artifacts/set82/hosted-actual-attempt3')
REFERENCE=Path('/Users/taytrrip/slug/set81-prep/retained-admission-actual/inputs/reference-v1.json')
os.umask(0o077)
start=time.time_ns()//1000000
due=((start+420000)//60000)*60000
selected={'version':1,'start_ms':start,'due_ms':due,'deadline_ms':start+900000,'outer_deadline_ms':start+1080000,
          'due_utc':datetime.datetime.fromtimestamp(due/1000,datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:00Z')}
assert datetime.datetime.fromtimestamp(start/1000,datetime.timezone.utc).date()==datetime.datetime.fromtimestamp((start+900000)/1000,datetime.timezone.utc).date()
with (A/'attempt-selection.json').open('x') as f:json.dump(selected,f,indent=2);f.write('\n');f.flush();os.fsync(f.fileno())
def run(args):
    remaining=(selected['deadline_ms']-time.time_ns()//1000000)/1000
    assert remaining>0
    result=subprocess.run([str(x) for x in args],check=True,timeout=min(90,remaining))
    assert time.time_ns()//1000000<selected['deadline_ms']
    return result
def ssh(name,argv):
    run(['python3',B/'host-command.py','--service','pool','--output',A/name,'--timeout','60','--',*argv])
    assert json.loads((A/(name+'.status.json')).read_text())['exit_code']==0
    assert (A/(name+'.stderr')).stat().st_size==0
def nonroot(name,args):
    ssh(name,['/bin/su','-s','/bin/sh','-c','exec '+shlex.join([str(x) for x in args]),'slug'])
def operator(command,extra=()):
    p='/data/hosted-test-3/private/'
    return ['/usr/local/bin/slugpoold',command,'--daily-recoverable-issue',
        '--setup-file',p+'selected-setup.json','--root','/data/hosted-test-3/operator',
        '--wallet-root','/data/hosted-test-3/custody/wallet16',
        '--wallet-public-passphrase-file',p+'public.pass','--wallet-unlock-passphrase-file',p+'unlock.pass',
        '--admission-seed-file',p+'admission.seed','--publication-seed-file',p+'publication.seed',
        '--cutoff-seed-file',p+'cutoff.seed','--rpc-config-file',p+'node-rpc.json',
        '--tls-certificate-file',p+'tls-cert.pem','--tls-key-file',p+'tls-key.pem',*extra]
host={'version':1,'origin':'https://turntable.proxy.rlwy.net:26354','listen':'0.0.0.0:8443',
      'pool_url':'http://127.0.0.1:18441','admission_url':'http://127.0.0.1:18442','ownership_url':'http://127.0.0.1:18443',
      'due_utc':selected['due_utc'],'tls_ca_path':'','tls_certificate_path':'','tls_key_path':''}
raw=json.dumps(host,separators=(',',':')).encode()
with (A/'host-selection.json').open('xb') as f:f.write(raw)
script='umask 077; set -euC; test "$(id -u):$(id -g)" = 10001:10001; printf %s '+shlex.quote(base64.b64encode(raw).decode())+' | base64 -d > /data/hosted-selection-3/selection.json'
ssh('selection-host-handoff.txt',['/bin/su','-s','/bin/sh','-c',script,'slug'])
nonroot('prepare-host.stdout.json',['/usr/local/bin/host-bootstrap-3','prepare'])
run(['python3',B/'fetch-selection.py','--output',A/'public'])
run(['python3',B/'check-selection.py','--public',A/'public','--reference',REFERENCE,
     '--origin',host['origin'],'--due-utc',selected['due_utc'],'--initial-start-ms',start,'--out',A/'selection-check.json'])
setup=json.loads((A/'public/setup.json').read_text())
prepared=json.loads((A/'public/prepared.json').read_text())
nonroot('operator-init.stdout.json',operator('init'))
frame=json.loads((A/'operator-init.stdout.json').read_text())
assert frame['version']==2 and frame['command']=='init' and frame['code']=='complete'
private=A/'client-inputs'; private.mkdir(mode=0o700)
(A/'client-evidence').mkdir(mode=0o700)
for src,dst in [(A/'public/remote.json',private/'remote-config.json')]:
    with dst.open('xb') as f:f.write(src.read_bytes())
with (private/'remote-rpc.json').open('x') as f:json.dump({'version':1,'url':'http://127.0.0.1:29582','username':'fixture','password':'public-pool-hosted','ca_certificate':''},f,separators=(',',':'))
spec=importlib.util.spec_from_file_location('credential_encoder',B/'client-flow/credentials.py')
enc=importlib.util.module_from_spec(spec);spec.loader.exec_module(enc)
gyms=setup['base']['miners']['enrollments'];assert len(gyms)==2
enrollment_id=enc.ident('SLUG pool miner enrollment',setup['base']['miners'],'v2')
expiry=prepared['credential_expires_at_ms'];assert expiry==due+3600000
for audience,index,name in [('pool',0,'pool-0'),('pool',1,'pool-1'),('admission',0,'admission')]:
    gym=gyms[index]['gym_id']; token=hashlib.sha256(('SLUG Set79 Path A '+audience+' '+gym).encode()).hexdigest()
    token_name='pool-token-'+str(index) if audience=='pool' else 'admission-token'
    with (private/token_name).open('x') as f:f.write(token)
    result='rotate-'+name+'.stdout.json'
    nonroot(result,operator('rotate',['--gym-id',gym,'--audience',audience,'--expected-generation','0',
        '--token-file','/data/hosted-test-3/private/'+token_name,'--expires-at-ms',str(expiry)]))
    frame=json.loads((A/result).read_text()); c=frame['result']
    assert frame['version']==2 and frame['command']=='rotate' and frame['code']=='complete'
    assert c['gym_id']==gym and c['audience']==audience and c['generation']==1 and c['revoked_at_ms']==0 and c['expires_at_ms']==expiry
    assert c['token_hash']==enc.ident('SLUG pool miner credential',dict(config_id=enrollment_id,audience=audience,token=token),'v2')
    assert c['reuse_hash']==enc.ident('SLUG pool miner token reuse',dict(config_id=enrollment_id,token=token),'v2')
    if index==0:
        run(['python3',B/'client-flow/credentials.py','--selection-report',A/'selection-check.json',
            '--setup',A/'public/setup.json','--remote',A/'public/remote.json','--prepared',A/'public/prepared.json',
            '--audience',audience,'--rotate-result',A/result,'--token-file',private/token_name,
            '--output',private/(audience+'.credential')])
assert time.time_ns()//1000000<selected['deadline_ms']
with (A/'setup-complete.json').open('x') as f:json.dump({'version':1,'passed':True,'ended_ms':time.time_ns()//1000000,'selection':selected},f,indent=2);f.write('\n')
print(json.dumps({'prepared':True,'start_ms':start,'due_ms':due,'deadline_ms':selected['deadline_ms']}))
