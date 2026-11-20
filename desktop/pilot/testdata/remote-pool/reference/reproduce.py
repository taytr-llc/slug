#!/usr/bin/env python3
"""Execute only the locked pure Python composition into one new output file."""
from pathlib import Path
import hashlib, json, sys
import model

root=Path(__file__).resolve().parent
lock=json.loads((root/'execution-lock.json').read_bytes())
for row in lock['files']:
    b=(root/row['path']).read_bytes()
    if len(b)!=row['bytes'] or hashlib.sha256(b).hexdigest()!=row['sha256']:
        raise SystemExit('locked public producer input changed')
output=Path(sys.argv[1]);output.parent.mkdir(parents=True,exist_ok=True)
result=model.build(root/'inputs')
data=json.dumps(result,indent=2,ensure_ascii=True).encode('ascii')+b'\n'
with output.open('xb') as f:f.write(data)
print(json.dumps(dict(output=str(output),bytes=len(data),sha256=hashlib.sha256(data).hexdigest(),positives=len(result['positives']),refusals=len(result['refusals']),raw_refusals=len(result['raw_refusals']),credentials=len(result['credentials']))))
