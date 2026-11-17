#!/usr/bin/env python3
"""Read-only finite reference verifier; no Go45/runtime imports or calls."""
import hashlib,json,re
from common import *
from crypto import selftest,parse_block,verify_merkle,ed_verify,D
from schema import validate,parse,correlate,FIELDS,Refusal
from reference_seed import materialize,calendar_checks,rpc_amount
from reference_state import phase

def main():
 c=json.loads((HERE/'seed_v1.json').read_bytes());assert c['input_index_sha256']==sha((HERE/'input_hashes.json').read_bytes());selftest()
 text=(HERE/'inputs/contracts/set45-seed-core-contract-draft.md').read_text();orders=[]
 for m in re.finditer(r'^- `([A-Za-z]+)`: `([^`]+)`',text,re.M):
  kind=m.group(1)
  if kind not in FIELDS:continue
  names=[x.strip().split()[0]for x in m.group(2).split(';')]
  assert names==FIELDS[kind],(kind,names,FIELDS[kind]);orders.append(kind)
 assert len(orders)>=18
 for d in c['dtos']:
  v=parse(d['kind'],d['canonical'].encode());assert v==d['value']and J(v).decode()==d['canonical']and sha(J(v))==d['sha256']
 for case in c['codec_cases']:
  b=materialize(case,c['dtos']);assert len(b)==case['input_bytes']and sha(b)==case['input_sha256']
  try:v=parse(case['kind'],b);accepted=True
  except Refusal:accepted=False
  assert accepted==case['accepted'],case['name']
  if accepted:assert J(v).decode()==c['dtos'][case['expected_dto']]['canonical']and sha(J(v))==case['canonical_sha256']
 for case in c['protocol_cases']:
  try:v=correlate(case['request'],case['response'],case['current_view'],case['expected_job'],case['terminal_digest'],case['expected_created_at_ms']);accepted=True
  except(Refusal,ValueError,KeyError,TypeError):accepted=False
  assert accepted==case['accepted'],case['name']
  if accepted:assert sha(J(v))==case['canonical_sha256']
 states={}
 for r in c['record_states']:
  v=parse('Record',r['canonical'].encode());assert record_digest(v)==r['digest'];assert r['digest']not in states;states[r['digest']]=v
 for scenario in c['scenarios']:
  for step in scenario['steps']:
   expected=step['expected'];rid=expected['record_digest'];s=validate('Status',step['status']);r=None if not rid else states[rid];assert phase(r)==s['phase']
   assert len(set(step['distinct_modeled_block_heights']))<=8
   if r is not None and r['publication']is not None:assert r['publication']['coin']['outpoint']in expected['reservations']
 calendar_checks(c['calendar']['rows'])
 for invalid in c['calendar']['invalid_dates']:
  try:dateparts(invalid)
  except ValueError:continue
  raise AssertionError('accepted bad date '+invalid)
 index=json.loads((HERE/c['block_fixture_index']).read_bytes());work=json.loads((HERE/'fixtures/normal_work.json').read_bytes());anchor_count=0
 for b in index['blocks']:
  data=(HERE/b['file']).read_bytes();assert len(data)==b['bytes']and sha(data)==b['sha256'];header,txs=parse_block(data);assert header.hex()==b['header']and work[header.hex()]['work_hash']==b['hash'];assert int(b['hash'],16)<=int(index['target'],16)
  assert [x['txid']for x in txs]==b['txids']
  # Separate direct tree fold cross-check does not call crypto.merkle.
  tree=[bytes.fromhex(x['txid'])[::-1]for x in txs]
  while len(tree)>1:
   if len(tree)%2:tree.append(tree[-1])
   tree=[hashlib.sha256(hashlib.sha256(tree[i]+tree[i+1]).digest()).digest()for i in range(0,len(tree),2)]
  assert tree[0]==header[36:68]
  for a in [index['anchor'],index['maximum_anchor']]:
   if a['hash']!=b['hash']:continue
   matches=[(ti,oi)for ti,tx in enumerate(txs)for oi,o in enumerate(tx['outputs'])if o['script']=='6a49'+a['payload']]
   assert matches[0]==(a['tx_index'],a['output_index']);tx=txs[a['tx_index']];assert sha(tx['raw'])==a['raw_tx_digest']and len(tx['raw'])==a['raw_tx_size'];assert verify_merkle(tx['txid'],a['tx_index'],len(txs),a['merkle_branch'],b['merkle_root']);anchor_count+=1
 assert anchor_count==2
 for case in c['carrier_cases']:
  s=raw(case['script']);expected=raw(case['expected_payload']);matches=len(s)==75 and s[:2]==b'\x6a\x49'and s[2:]==expected;assert matches==case['matches_known_declaration']
 for case in c['rpc_amount_cases']:
  try:n=rpc_amount(case['token']);ok=True
  except ValueError:n=None;ok=False
  assert ok==case['accepted']and n==case['satoshis']
 print(json.dumps(dict(status='passed',contract_field_orders=len(orders),dtos=len(c['dtos']),codec_cases=len(c['codec_cases']),protocol_cases=len(c['protocol_cases']),record_states=len(states),modeled_steps=sum(len(x['steps'])for x in c['scenarios']),calendar_rows=len(c['calendar']['rows']),normal_work_fixture_blocks=len(index['blocks']),normal_argon_headers=len(work),anchor_full_body_checks=anchor_count,corpus_sha256=sha((HERE/'seed_v1.json').read_bytes())),sort_keys=True))
if __name__=='__main__':main()
