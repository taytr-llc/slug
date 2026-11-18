#!/usr/bin/env python3
"""Bounded serial reproducibility capture; no runtime or network calls."""
import datetime
import hashlib
import json
import os
import pathlib
import subprocess
import time

ROOT=pathlib.Path(__file__).resolve().parent
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()


def sources():
    paths=[p for p in ROOT.rglob('*')if p.is_file()and '__pycache__'not in p.parts and 'verification'not in p.relative_to(ROOT).parts and p.name not in ['file_index.json','freeze.json','root_review.json']]
    return {str(p.relative_to(ROOT)):sha(p)for p in sorted(paths)}


before=sources();out=ROOT/'verification';out.mkdir(exist_ok=False)
report=dict(status='running',started_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
            source_sha256_before=before,commands=[],Go_runtime_or_outputs_used=False)
for i,(python,tz,seed)in enumerate([('/usr/bin/python3','UTC','0'),('/opt/homebrew/bin/python3.14','Pacific/Chatham','947331')]):
    env=os.environ.copy();env.update(TZ=tz,PYTHONHASHSEED=seed,PYTHONDONTWRITEBYTECODE='1')
    argv=[python,'-B','-S',str(ROOT/'verify_reference.py'),'--regenerate']
    start=time.monotonic()
    try:
        done=subprocess.run(argv,cwd=ROOT,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=60)
        raw=done.stdout;exit_code=done.returncode;timed_out=False
    except subprocess.TimeoutExpired as exc:
        raw=exc.stdout or b'';exit_code=None;timed_out=True
    log=out/('python-%d.log'%i);log.write_bytes(raw)
    row=dict(argv=argv,timezone=tz,hash_seed=seed,seconds=round(time.monotonic()-start,6),exit_code=exit_code,timed_out=timed_out,log_sha256=sha(log))
    report['commands'].append(row)
    if exit_code!=0:
        report['status']='failed';report['source_sha256_after']=sources()
        (out/'report.json').write_text(json.dumps(report,indent=2)+'\n')
        raise SystemExit('reproducibility failed; retained '+str(log))
    result=json.loads(raw);assert result['status']=='passed'and result['regeneration_byte_identical']
    row['result']=result
    print('passed',python,'seconds',row['seconds'],flush=True)
after=sources();assert before==after,'source/input/corpus drift'
report.update(status='passed',finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),source_sha256_after=after,source_changes=[],corpus_sha256=sha(ROOT/'bout_v1.json'),input_index_sha256=sha(ROOT/'input_index.json'))
(out/'report.json').write_text(json.dumps(report,indent=2)+'\n')
print('report',sha(out/'report.json'),flush=True)
