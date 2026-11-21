#!/usr/bin/env python3
"""Fetch only six public fixture-selection files without PTY byte conversion."""
import argparse,base64,hashlib,io,json,os,subprocess,tarfile
from pathlib import Path
p=argparse.ArgumentParser(); p.add_argument('--output',type=Path,required=True); a=p.parse_args()
assert a.output.is_absolute() and not a.output.exists()
os.umask(0o077); a.output.mkdir(mode=0o700)
names=('endpoints.json','setup.json','remote.json','prepared.json','ca.pem','tls-cert.pem')
encoded=a.output.parent/(a.output.name+'-transport.base64')
cmd='set -eu; tar -C /data/hosted-test-5/public -cf - '+' '.join(names)+' | base64'
subprocess.run(['python3',str(Path(__file__).with_name('host-command.py')),
    '--service','pool','--output',str(encoded),'--','/bin/sh','-c',cmd],check=True)
raw=encoded.read_bytes(); assert len(raw)<4*1024*1024
archive=base64.b64decode(b''.join(raw.split()),validate=True)
report={}
with tarfile.open(fileobj=io.BytesIO(archive)) as t:
    members=t.getmembers(); assert [x.name for x in members]==list(names)
    for m in members:
        assert m.isfile() and 0<m.size<=512*1024
        body=t.extractfile(m).read(); assert len(body)==m.size
        with (a.output/m.name).open('xb') as f:f.write(body)
        report[m.name]={'bytes':len(body),'sha256':hashlib.sha256(body).hexdigest()}
with (a.output.parent/(a.output.name+'-transfer.json')).open('x') as f:json.dump(report,f,indent=2);f.write('\n')
print(json.dumps({'public_files':len(report),'output':str(a.output)}))
