#!/usr/bin/env python3
"""Author self-checks of the finite corpus; not an independent reviewer."""
import argparse,ast,json,pathlib,hashlib
from common import ROOT,J,sha,INDEX
import generate as G
import schema as S

def verify(path):
 raw=path.read_bytes();v=json.loads(raw);assert raw==J(v)+b'\n'
 assert v['version']==1 and v['contract_sha256']==INDEX['contract_sha256']
 assert v['input_index_sha256']==sha((ROOT/'input_index.json').read_bytes())
 for group in ['compositions','dtos','codec_cases','protocol_cases','portrait_protocol_cases','owner_cases','state_cases']:
  keys=[(r.get('kind'),r['name'])for r in v[group]];assert len(keys)==len(set(keys)),group
 for r in v['dtos']:
  parsed=S.parse(r['kind'],r['canonical'].encode());assert J(parsed)==r['canonical'].encode() and sha(J(parsed))==r['sha256']
 for row in v['codec_cases']:
  value=G.materialize(row,v['dtos']);assert len(value)==row['input_bytes']and sha(value)==row['input_sha256']
  try:parsed=S.parse(v['dtos'][row['base']]['kind'],value);ok=True
  except(S.Refusal,ValueError,TypeError,KeyError,OverflowError,IndexError,RecursionError):ok=False
  assert ok==row['accept'],row['name']
  if ok:assert sha(J(parsed))==row['canonical_sha256']
 # Exact excerpt provenance: definitions only, no import or runtime edits to predecessor.
 old=(ROOT/'inputs/prior-schema47.py').read_text();nodes=ast.parse(old).body
 extracted='\n\n'.join(ast.get_source_segment(old,n)for n in nodes if isinstance(n,ast.FunctionDef)and n.name in ['bounded_engine_state','engine_result'])+'\n'
 assert extracted==(ROOT/'inputs/engine_transport_excerpt.py.txt').read_text()
 assert all(x['modeled_only'] is True for x in v['state_cases'])
 return dict(status='passed',scope='Author corpus/codec/source-excerpt self-check; no runtime or independent-review claim',corpus_sha256=sha(raw),input_index_sha256=sha((ROOT/'input_index.json').read_bytes()),counts={k:len(v[k])for k in ['compositions','dtos','codec_cases','protocol_cases','portrait_protocol_cases','owner_cases','state_cases']})

if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('corpus',type=pathlib.Path);a=p.parse_args();print(json.dumps(verify(a.corpus),sort_keys=True))
