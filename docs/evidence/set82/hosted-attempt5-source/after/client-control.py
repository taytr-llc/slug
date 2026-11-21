#!/usr/bin/env python3
"""Launch or request one stop of the fixed selected local fixture child owner."""
import argparse,json,os,signal,subprocess,sys,time
from pathlib import Path
B=Path(__file__).resolve().parent;A=Path('/Users/taytrrip/slug/artifacts/set82/hosted-actual-attempt5')
p=argparse.ArgumentParser();p.add_argument('action',choices=['start','stop']);p.add_argument('mode',choices=['miner','observe']);a=p.parse_args()
os.umask(0o077)
selected=json.loads((A/'attempt-selection.json').read_text());checked=json.loads((A/'selection-check.json').read_text());assert checked['passed']
name='miner-1' if a.mode=='miner' else 'observer-1'
marker=A/(name+'-launcher.json')
if a.action=='stop':
    info=json.loads(marker.read_text());pid=info['pid'];assert pid>1 and info['deadline_ms']==selected['deadline_ms']
    args=subprocess.run(['ps','-p',str(pid),'-o','args='],capture_output=True,text=True,check=True).stdout
    assert str(B/'client-flow/run-client.py') in args and str(A/'client-evidence'/name) in args
    with (A/(name+'-stop-request.json')).open('x') as f:json.dump({'pid':pid,'requested_at_ms':time.time_ns()//1000000},f)
    os.kill(pid,signal.SIGTERM)
    print(json.dumps({'requested_stop':True,'mode':a.mode,'pid':pid}))
else:
    binary='/Users/taytrrip/slug/artifacts/set82/client-binaries-attempt2/slugpilot' if a.mode=='miner' else '/Users/taytrrip/slug/artifacts/set82/client-native-attempt2/slug-host-client'
    private=A/'client-inputs'
    cmd=[sys.executable,str(B/'client-flow/run-client.py'),'--mode',a.mode,'--binary',binary,
        '--config-id',checked['remote_id'],'--deadline-ms',str(selected['deadline_ms']),
        '--owned-root',str(A/'owned/owned-gym'),'--attachment-root',str(A/'remote-attachment'),
        '--remote-config',str(private/'remote-config.json'),'--node-rpc',str(private/'remote-rpc.json'),
        '--pool-credential',str(private/'pool.credential'),'--admission-credential',str(private/'admission.credential'),
        '--evidence-dir',str(A/'client-evidence'/name)]
    if a.mode=='observe':cmd+=['--reference','/Users/taytrrip/slug/set81-prep/retained-admission-actual/inputs/reference-v1.json']
    with marker.open('x') as f:json.dump({'pid':os.getpid(),'mode':a.mode,'deadline_ms':selected['deadline_ms'],'started_at_ms':time.time_ns()//1000000},f);f.flush();os.fsync(f.fileno())
    os.execv(sys.executable,cmd)
