#!/usr/bin/env python3
"""Fixed generation controls for the selected isolated operator; no retries."""
import argparse,base64,hashlib,json,shlex,subprocess,time
from pathlib import Path
B=Path(__file__).resolve().parent; A=Path('/Users/taytrrip/slug/artifacts/set82/hosted-actual-attempt1')
p=argparse.ArgumentParser();p.add_argument('action',choices=['start','stop','logs','audit-before','audit-after']);p.add_argument('generation',choices=['01','02','03']);a=p.parse_args()
selection=json.loads((A/'attempt-selection.json').read_text())
checked=json.loads((A/'selection-check.json').read_text());assert checked['passed']
setup_sha=hashlib.sha256((A/'public/setup.json').read_bytes()).hexdigest()
binary_sha='7a8f7e783d7798a5200775d27e41bb077a196928e4e5a4c3f9baa510d65a443d'
def ssh(name,script,limit):
    cmd=['python3',str(B/'host-command.py'),'--service','pool','--output',str(A/name),'--timeout',str(limit),
         '--','/bin/su','-s','/bin/sh','-c',script,'slug']
    subprocess.run(cmd,check=True,timeout=limit+5)
    assert json.loads((A/(name+'.status.json')).read_text())['exit_code']==0
    assert (A/(name+'.stderr')).stat().st_size==0
if a.action in ('start','stop'):
    mode='run' if a.action=='start' else 'stop'
    args=['/usr/local/bin/host-operator-lifecycle',mode,a.generation]
    if a.action=='start':
        assert time.time_ns()//1000000<selection['deadline_ms']
        args.append(str(selection['deadline_ms']//1000))
    args += [setup_sha,binary_sha]
    ssh('operator-'+a.generation+'-'+a.action+'.txt','exec '+shlex.join(args),1080 if a.action=='start' else 45)
    if a.action=='stop':
        report=json.loads((A/('operator-'+a.generation+'-stop.txt')).read_text())
        assert report['clean'] and report['child_waited'] and report['monitor_joined'] and report['child_exit']==report['monitor_exit']==0
elif a.action=='logs':
    name='operator-'+a.generation+'-logs.base64'
    ssh(name,'set -eu; tar -C /data/hosted-test/lifecycle/'+a.generation+' -cf - stdout.jsonl stderr.log result.json owner.complete | base64',30)
    import io,tarfile,os
    os.umask(0o077);out=A/('operator-'+a.generation+'-closed');out.mkdir(mode=0o700)
    raw=base64.b64decode(b''.join((A/name).read_bytes().split()),validate=True);assert len(raw)<8*1024*1024
    with tarfile.open(fileobj=io.BytesIO(raw)) as t:
        members=t.getmembers();assert [m.name for m in members]==['stdout.jsonl','stderr.log','result.json','owner.complete']
        for m in members:
            assert m.isfile() and m.size<=4*1024*1024
            with (out/m.name).open('xb') as f:f.write(t.extractfile(m).read())
    assert (out/'stderr.log').stat().st_size==0
else:
    assert (a.action,a.generation) in [('audit-before','02'),('audit-after','03')]
    name=a.action+'.json'
    cmd=['/usr/local/bin/slug-host-audit','audit','--root','/data/hosted-test',
         '--expected-setup-id',checked['setup_id'],'--expect-delivery','confirmed']
    if a.action=='audit-after':cmd+=['--compare','/data/hosted-test/public/audit-before.json']
    script='set -euC; umask 077; '+shlex.join(cmd)+' > /data/hosted-test/public/'+name
    ssh(a.action+'-command.txt',script,60)
    encoded=a.action+'.base64'
    ssh(encoded,'exec base64 /data/hosted-test/public/'+name,30)
    raw=base64.b64decode(b''.join((A/encoded).read_bytes().split()),validate=True);assert len(raw)<16*1024*1024
    with (A/name).open('xb') as f:f.write(raw)
    report=json.loads(raw);assert report['passed']
    if a.action=='audit-after':assert report['compared'] and report['immutable_batch_and_credits_unchanged']
print(json.dumps({'action':a.action,'generation':a.generation,'completed':True}))
