import hashlib,json,os,subprocess,sys,tempfile
from pathlib import Path
root=Path(__file__).resolve().parent
with tempfile.TemporaryDirectory(prefix='slug-withdrawal-wire-') as directory:
    out=Path(directory)/'wire-v1.json'
    subprocess.run([sys.executable,str(root/'producer/generate.py'),'--output',str(out)],check=True,env=dict(os.environ,PYTHONDONTWRITEBYTECODE='1'))
    data=out.read_bytes()
    assert data==(root/'wire-v1.json').read_bytes()
    assert hashlib.sha256(data).hexdigest()=='60fe12e20e0c842a43cfc8f41892a8fcf4a7c991592363b17f87a8180098af9c'
    print(json.dumps(dict(status='passed',sha256=hashlib.sha256(data).hexdigest(),bytes=len(data))))
