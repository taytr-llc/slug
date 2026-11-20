#!/usr/bin/env python3
"""Serial independent Python/TZ/hashseed reproductions with mandatory fresh reports."""
import argparse,hashlib,json,os,subprocess,sys,time
from pathlib import Path
ROOT=Path(__file__).resolve().parent
PRODUCERS=('common.py','history_math.py','reference_history.py','schema_wire.py','reference_wire.py','state_models.py','generate.py','verify_reference.py','check_reproducibility.py','extract_inputs.py')
def sha(b):return hashlib.sha256(b).hexdigest()
def source_hashes():return {n:sha((ROOT/n).read_bytes()) for n in PRODUCERS}
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--attempt',required=True);ap.add_argument('--python-a',required=True);ap.add_argument('--python-b',required=True);a=ap.parse_args();out=ROOT/'verification'/a.attempt
    if out.exists():raise SystemExit('preserve existing attempt; use new name')
    out.mkdir(parents=True);before=source_hashes();corpus=sha((ROOT/'corpus.json').read_bytes());rows=[];started=time.monotonic();failed=False
    for i,(exe,tz,seed) in enumerate([(a.python_a,'UTC','7'),(a.python_b,'Pacific/Auckland','981')],1):
        report=out/f'python{i}.json';log=out/f'python{i}.log';cmd=[exe,'-B',str(ROOT/'verify_reference.py'),'--report',str(report)];env=dict(os.environ,PYTHONDONTWRITEBYTECODE='1',TZ=tz,PYTHONHASHSEED=seed);start=time.monotonic()
        row=dict(command=cmd,environment={k:env[k] for k in ('TZ','PYTHONHASHSEED','PYTHONDONTWRITEBYTECODE')},report=str(report.relative_to(ROOT)),log=str(log.relative_to(ROOT)))
        try:
            with log.open('wb') as f:p=subprocess.run(cmd,cwd=ROOT,env=env,stdout=f,stderr=subprocess.STDOUT,timeout=60)
            row['exit_code']=p.returncode;row['elapsed_seconds']=time.monotonic()-start;row['log_sha256']=sha(log.read_bytes())
            if not report.is_file():raise ValueError('missing fresh child report')
            r=json.loads(report.read_bytes());row['report_sha256']=sha(report.read_bytes());row['python']=r.get('python')
            if p.returncode!=0 or r.get('status')!='passed' or r.get('corpus_sha256')!=corpus or r.get('source_sha256')!=before:raise ValueError('child report/source/corpus/exit mismatch')
            row['status']='passed'
        except BaseException as e:row.update(status='failed',error=str(e));failed=True
        rows.append(row)
        if failed:break
    after=source_hashes();stable=after==before;versions=[r.get('python') for r in rows]
    ok=not failed and stable and len(rows)==2 and all(r['status']=='passed' for r in rows) and versions[0]!=versions[1]
    report=dict(version=1,status='passed' if ok else 'failed',findings=[] if ok else ['See preserved child evidence; no prior failure relabelled.'],source_sha256=before,source_sha256_after=after,sources_unchanged=stable,corpus_sha256=corpus,runs=rows,serial=True,distinct_python_versions=len(rows)==2 and versions[0]!=versions[1],elapsed_seconds=time.monotonic()-started,scope='Pure independent reference reproduction only; no runtime/engine/Argon/signing/proof work.')
    target=out/'report.json';target.write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({'status':report['status'],'path':str(target),'sha256':sha(target.read_bytes())}));raise SystemExit(0 if ok else 1)
if __name__=='__main__':main()
