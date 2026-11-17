#!/usr/bin/env python3
"""Bounded conformance coverage search, never calibration/model tuning."""
import copy,json,time,pathlib
import reference_bout as B
P=pathlib.Path(__file__).resolve().parent
source=json.loads((P/'bout_v1.json').read_bytes());base=source['cases'][0]['input'];found={};start=time.monotonic()
for n in range(20000):
 inp=copy.deepcopy(base);inp['seed']=B.sha(b'SLUG bout conformance coverage\0v1\0'+n.to_bytes(4,'little'))
 out=B.simulate(inp);diff=abs(out['scores'][0]['total_tenths']-out['scores'][1]['total_tenths'])
 name='exact_decision_draw' if out['method']=='decision_draw' else 'adjacent_decision_scores' if out['method']=='decision' and diff==1 else 'public_corner_finish' if out['method']=='tko_corner' else None
 if name and name not in found:
  found[name]={'index':n,'input':inp};print(json.dumps({'found':name,'index':n,'elapsed':time.monotonic()-start}),flush=True)
 if len(found)==3:break
 if n%2000==1999:print(json.dumps({'checked':n+1,'found':list(found),'elapsed':time.monotonic()-start}),flush=True)
report={'version':1,'scope':'coverage-selected conformance inputs, excluded from frozen calibration','bound':20000,'tested':n+1,'selection_seed_domain':'SLUG bout conformance coverage\\0v1\\0','base_input_id':B.input_id(base),'cases':found}
(P/'selected_public_cases.json').write_bytes(B.canonical(report)+b'\n')
print(json.dumps({'status':'completed','found':list(found),'tested':n+1,'seconds':time.monotonic()-start}),flush=True)
