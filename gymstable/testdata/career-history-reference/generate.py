#!/usr/bin/env python3
"""Assemble independent math, wire and finite models; no Go/engine/proof work."""
import argparse,json
from common import ROOT,J,sha,require
from reference_history import build_fragment
from reference_wire import build_wire
from state_models import build_models
PRODUCERS=('common.py','history_math.py','reference_history.py','schema_wire.py','reference_wire.py','state_models.py','generate.py','verify_reference.py','check_reproducibility.py','extract_inputs.py')
def source_hashes():return {n:sha((ROOT/n).read_bytes()) for n in PRODUCERS}
def check_inputs():
    p=ROOT/'input_index.json';index=json.loads(p.read_bytes())
    for name,row in index['inputs'].items():
        raw=(ROOT/name).read_bytes();require(sha(raw)==row['sha256'] and len(raw)==row['bytes'],'input_changed_'+name)
    require(sha((ROOT/'extract_inputs.py').read_bytes())==index['extractor_sha256'],'extractor_changed')
    return dict(input_index_sha256=sha(p.read_bytes()),count=len(index['inputs']),bytes=sum(x['bytes'] for x in index['inputs'].values()))
def build():
    inputs=check_inputs();history=build_fragment();wire=build_wire();state=dict(version=1,scope='finite symbolic contract models, not runtime evidence',cases=build_models())
    expected={'history.json':(json.dumps(history,ensure_ascii=False,indent=2)+'\n').encode(),'wire.json':J(wire)+b'\n','state.json':(json.dumps(state,sort_keys=True,separators=(',',':'))+'\n').encode()}
    for name,raw in expected.items():require((ROOT/'fragments'/name).read_bytes()==raw,'fragment_drift_'+name)
    return dict(version=1,family='career_history_v1',scope='Independent preimplementation reference: genuine inherited public facts and finite modeled histories remain distinct; no sealed runtime authority, chain verification, scheduling, timing, whole-league completeness or visual claim.',inputs=inputs,history=history,wire=wire,state=state)
def canonical_corpus():return J(build())+b'\n'
def main():
    parser=argparse.ArgumentParser();parser.add_argument('--output',default=str(ROOT/'corpus.json'));args=parser.parse_args();data=canonical_corpus()
    from pathlib import Path
    p=Path(args.output)
    if p.exists() and p.read_bytes()!=data:raise SystemExit('refuse overwrite differing output; preserve predecessor first')
    p.write_bytes(data);print(json.dumps({'status':'generated','bytes':len(data),'sha256':sha(data)}))
if __name__=='__main__':main()
