#!/usr/bin/env python3
"""One bounded normal-PoW RPC mutation on the isolated fixture node."""
import argparse, base64, json, os, time, urllib.request
from pathlib import Path
p=argparse.ArgumentParser()
p.add_argument('--before',type=int,required=True)
p.add_argument('--count',type=int,required=True)
p.add_argument('--output',type=Path,required=True)
a=p.parse_args()
assert a.count > 0 and a.before+a.count <= 180
os.umask(0o077)
def rpc(method,params,timeout):
    body=json.dumps({'jsonrpc':'1.0','id':'bounded-host-test','method':method,'params':params}).encode()
    req=urllib.request.Request('http://127.0.0.1:29582',data=body,headers={
        'Authorization':'Basic '+base64.b64encode(b'fixture:public-pool-hosted').decode(),
        'Content-Type':'application/json'})
    with urllib.request.urlopen(req,timeout=timeout) as response:
        raw=response.read(4*1024*1024+1)
    assert len(raw)<=4*1024*1024
    frame=json.loads(raw)
    assert frame['error'] is None,frame['error']
    return frame['result']
report={'version':1,'before':a.before,'count':a.count,'started':time.time(),'normal_pow':True}
with a.output.open('x') as stream:
    try:
        before=rpc('getblockchaininfo',[],20)
        report['initial']=before
        assert before['blocks']==a.before
        hashes=rpc('generate',[a.count],300)
        report['generated']=hashes
        assert len(hashes)==a.count
        after=rpc('getblockchaininfo',[],20)
        report['final']=after
        assert after['blocks']==a.before+a.count and after['bestblockhash']==hashes[-1]
        report['passed']=True
    except Exception as e:
        report['passed']=False
        report['error']=str(e)
        raise
    finally:
        report['ended']=time.time()
        json.dump(report,stream,indent=2)
        stream.write('\n'); stream.flush(); os.fsync(stream.fileno())
print(json.dumps({'passed':True,'height':after['blocks'],'hash':after['bestblockhash'],'seconds':report['ended']-report['started']}))
