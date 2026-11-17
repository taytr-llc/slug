#!/usr/bin/env python3
"""Bounded conformance coverage search, never calibration/model tuning."""
import copy,json,time,pathlib
import reference_bout as B
P=pathlib.Path(__file__).resolve().parent
source=json.loads((P/'bout_v1.json').read_bytes());base=copy.deepcopy(source['cases'][0]['input']);base['b']['manifest']=copy.deepcopy(base['a']['manifest']);base['a']['pro_xp']=base['b']['pro_xp']=0;base['a']['training_skill']=base['b']['training_skill']=0;found={};start=time.monotonic()
for n in range(20000):
 inp=copy.deepcopy(base);inp['seed']=B.sha(b'SLUG bout conformance balanced\0v1\0'+n.to_bytes(4,'little'))
 out=B.simulate(inp);diff=abs(out['scores'][0]['total_tenths']-out['scores'][1]['total_tenths'])
 name='exact_decision_draw' if out['method']=='decision_draw' else 'adjacent_decision_scores' if out['method']=='decision' and diff==1 else 'public_corner_finish' if out['method']=='tko_corner' else None
 if name and name not in found:
  found[name]={'index':n,'input':inp};print(json.dumps({'found':name,'index':n,'elapsed':time.monotonic()-start}),flush=True)
 if 'exact_decision_draw' in found and 'adjacent_decision_scores' in found:break
 if n%2000==1999:print(json.dumps({'checked':n+1,'found':list(found),'elapsed':time.monotonic()-start}),flush=True)
report={'version':1,'scope':'coverage-selected conformance inputs, excluded from frozen calibration','bound':20000,'tested':n+1,'selection_seed_domain':'SLUG bout conformance balanced\\0v1\\0','base_input_id':B.input_id(base),'cases':found}
(P/'search_balanced.json').write_bytes(B.canonical(report)+b'\n')
print(json.dumps({'status':'completed','found':list(found),'tested':n+1,'seconds':time.monotonic()-start}),flush=True)
