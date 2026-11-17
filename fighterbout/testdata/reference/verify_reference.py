#!/usr/bin/env python3
"""Regenerate frozen normative bytes and validate preexisting Python oracles."""
import json,pathlib,sys
import reference_bout as B
import generate_corpus as G
P=pathlib.Path(__file__).resolve().parent
manifest=json.loads((P/'inputs/manifest/manifests_v1.json').read_bytes())
for c in manifest['cases']:
 actual=B.MANIFEST.generate(c['seed']);assert actual==c['manifest'];assert B.canonical(actual).decode()==c['canonical']
condition=json.loads((P/'inputs/condition/condition_v1.json').read_bytes())
for c in condition['cases']:
 dna=condition['dna_cases'][c['dna_index']]['dna']
 assert B.CONDITION.project(c['version'],dna,c['born_at_ms'],c['as_of_ms'],c['pro_xp'])==c['expected']
for report in ('search_unequal.json','search_balanced.json'):
 value=json.loads((P/report).read_bytes());domain=value['selection_seed_domain'].replace('\\0','\0').encode('ascii')
 for row in value['cases'].values():assert row['input']['seed']==B.sha(domain+row['index'].to_bytes(4,'little'))
out=G.generate();raw=B.canonical(out)+b'\n';assert (P/'bout_v1.json').read_bytes()==raw,'normative bytes differ'
print(json.dumps({'status':'passed','python':sys.version,'manifest_cases':len(manifest['cases']),'condition_cases':len(condition['cases']),'complete_bouts':len(out['cases']),'corpus_sha256':B.sha(raw)}))
