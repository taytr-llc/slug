#!/usr/bin/env python3
"""Rebuild exact corpus and authenticate captured inputs/source; no Go or network."""
import argparse,json,os,platform,sys,time,traceback
from pathlib import Path
from common import ROOT,J,sha,require
from generate import canonical_corpus,source_hashes,check_inputs

def verify():
    before=source_hashes();inputs=check_inputs();data=canonical_corpus();expected=(ROOT/'corpus.json').read_bytes();require(data==expected,'corpus_mismatch')
    corpus=json.loads(data);h=corpus['history'];w=corpus['wire'];s=corpus['state'];after=source_hashes();require(before==after,'producer_drift')
    return dict(status='passed',findings=[],python=sys.version,executable=sys.executable,platform=platform.platform(),environment={k:os.environ.get(k) for k in ('TZ','PYTHONHASHSEED','PYTHONDONTWRITEBYTECODE')},inputs=inputs,source_sha256=after,corpus_sha256=sha(data),corpus_bytes=len(data),counts=dict(histories=len(h['histories']),core_controls=len(h['controls']),index_controls=len(h['index_controls']),wire_dtos=len(w['dtos']),wire_codecs=len(w['codecs']),protocols=len(w['protocols']),state_models=len(s['cases'])),transport_trap=w['transport_trap'],genuine_event_rederived_from_retained_first_finality=True,pinned_prior_current_equations_equal=all(x['prior_current_equations_equal'] for x in h['histories']),limitations=['No new Go/runtime, admission proof, engine, Argon, signing, chain RPC, mining or database access.','Genuine inputs inherit Set49 public proof report; those proofs are not rerun.','Synthetic histories and finite lane/cache models do not establish runtime authority, races, duration or physical heap limits.'])
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--report',required=True);a=ap.parse_args();out=Path(a.report);require(not out.exists(),'report_exists');start=time.monotonic()
    try:report=verify()
    except BaseException as e:
        report=dict(status='failed',error=str(e),traceback=traceback.format_exc(),python=sys.version);out.parent.mkdir(parents=True,exist_ok=True);out.write_text(json.dumps(report,indent=2)+'\n');raise
    report['elapsed_seconds']=time.monotonic()-start;out.parent.mkdir(parents=True,exist_ok=True);out.write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({'status':report['status'],'report':str(out),'sha256':sha(out.read_bytes()),'counts':report['counts']}))
if __name__=='__main__':main()
