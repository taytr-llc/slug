#!/usr/bin/env python3
"""Fixed generation controls for the selected isolated operator; no retries."""
import argparse,base64,hashlib,json,os,shlex,stat,subprocess,time
from pathlib import Path
B=Path(__file__).resolve().parent; A=Path('/Users/taytrrip/slug/artifacts/set82/hosted-actual-attempt5')
p=argparse.ArgumentParser();p.add_argument('action',choices=['start','stop','logs','audit-before','audit-after']);p.add_argument('generation',choices=['01','02','03']);a=p.parse_args()
selection=json.loads((A/'attempt-selection.json').read_text())
checked=json.loads((A/'selection-check.json').read_text());assert checked['passed']
setup_sha=hashlib.sha256((A/'public/setup.json').read_bytes()).hexdigest()
binary_selection=A/'binary-selection.json'
def binary_input():
    assert binary_selection==binary_selection.resolve(strict=True)
    fd=os.open(binary_selection,os.O_RDONLY|os.O_NOFOLLOW)
    try:
        before=os.fstat(fd)
        assert stat.S_ISREG(before.st_mode) and before.st_nlink==1 and before.st_uid==os.geteuid()
        assert stat.S_IMODE(before.st_mode) in (0o400,0o600) and 0<before.st_size<=256
        raw=os.read(fd,257);after=os.fstat(fd)
        identity=lambda st:(st.st_dev,st.st_ino,st.st_size,st.st_mtime_ns,st.st_ctime_ns,st.st_mode,st.st_uid,st.st_nlink)
        assert identity(before)==identity(after) and len(raw)==before.st_size
        return raw,identity(before)
    finally:os.close(fd)
binary_raw,binary_identity=binary_input()
def binary_pairs(rows):
    result={}
    for key,value in rows:
        assert key not in result
        result[key]=value
    return result
binary_record=json.loads(binary_raw,object_pairs_hook=binary_pairs)
assert list(binary_record)==['version','operator_sha256'] and type(binary_record['version']) is int and binary_record['version']==1
binary_sha=binary_record['operator_sha256']
assert type(binary_sha) is str and len(binary_sha)==64 and binary_sha!='0'*64 and all(c in '0123456789abcdef' for c in binary_sha)
def binary_unchanged():
    assert binary_input()==(binary_raw,binary_identity)
def ssh(name,script,limit):
    binary_unchanged()
    cmd=['python3',str(B/'host-command.py'),'--service','pool','--output',str(A/name),'--timeout',str(limit),
         '--','/bin/su','-s','/bin/sh','-c',script,'slug']
    subprocess.run(cmd,check=True,timeout=limit+5)
    assert json.loads((A/(name+'.status.json')).read_text())['exit_code']==0
    assert (A/(name+'.stderr')).stat().st_size==0
    binary_unchanged()
if a.action in ('start','stop'):
    mode='run' if a.action=='start' else 'stop'
    args=['/usr/local/bin/host-operator-lifecycle-5',mode,a.generation]
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
    ssh(name,'set -eu; tar -C /data/hosted-test-5/lifecycle/'+a.generation+' -cf - stdout.jsonl stderr.log result.json owner.complete | base64',30)
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
    cmd=['/usr/local/bin/slug-host-audit','audit','--root','/data/hosted-test-5',
         '--expected-setup-id',checked['setup_id'],'--expect-delivery','confirmed']
    if a.action=='audit-after':cmd+=['--compare','/data/hosted-test-5/public/audit-before.json']
    script='set -euC; umask 077; '+shlex.join(cmd)+' > /data/hosted-test-5/public/'+name
    ssh(a.action+'-command.txt',script,60)
    encoded=a.action+'.base64'
    ssh(encoded,'exec base64 /data/hosted-test-5/public/'+name,30)
    raw=base64.b64decode(b''.join((A/encoded).read_bytes().split()),validate=True);assert len(raw)<16*1024*1024
    with (A/name).open('xb') as f:f.write(raw)
    report=json.loads(raw);assert report['passed']
    if a.action=='audit-after':assert report['compared'] and report['immutable_batch_and_credits_unchanged']
binary_unchanged()
print(json.dumps({'action':a.action,'generation':a.generation,'completed':True}))
