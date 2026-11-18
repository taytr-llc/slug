#!/usr/bin/env python3
"""Regenerate/check independent Set48 reference, with no Go/runtime/network."""
import argparse,contextlib,datetime,hashlib,json,pathlib,sys,time
sys.dont_write_bytecode=True
from common import *
import crypto_result as R
import schema_core as S
import reference_results as CORE
import reference_store as STORE
import reference_proof as PROOF
import reference_wire as WIRE
import reference_state as STATE

@contextlib.contextmanager
def no_new_work(transport=False):
 def forbidden(*a,**kw):raise AssertionError('forbidden expensive/authority operation in codec')
 items=[(CRYPTO.__dict__,'ed_sign'),(CRYPTO.__dict__,'sign'),(ENGINE.__dict__,'simulate')]
 if transport:items +=[(MANIFEST.__dict__,'generate'),(INHERITED.source.__globals__,'condition'),(CRYPTO.__dict__,'verify_merkle')]
 old=[(d,k,d[k])for d,k in items]
 try:
  for d,k,_ in old:d[k]=forbidden
  yield
 finally:
  for d,k,v in old:d[k]=v

def sources():
 return {p.name:sha(p.read_bytes())for p in sorted(HERE.glob('*.py'))}
def inputs():return {p:sha((HERE/p).read_bytes())for p in INDEX}
def source_provenance():
 r=BOUT_RECORD;s=OLD.source_projection(CARD_PIN,CARD_RECORD,CARD_RECORD['plan']['bouts'][0]);assert s==r['source'];inp=OLD.derive_input(s);assert inp==r['input'];out=ENGINE.simulate(inp);assert out==r['result'];assert ENGINE.input_id(inp)==r['input_id']and ENGINE.digest(out)==r['result_digest'];report=load('inputs/actual47/bout-report.json');chain=load('inputs/actual47/chain-report.json');assert report['status']==chain['status']=='passed';assert OLD.record_digest(r)==report['record_digest'];assert chain['chain_sha256']==INDEX['inputs/actual47/public-chain-evidence.json']['sha256']
 return dict(genuine_saved_saturday=s['event']['saturday'],card_id=r['card_id'],bout_id=r['bout_id'],input_id=r['input_id'],result_digest=r['result_digest'],record_digest=OLD.record_digest(r),independent_prior_engine_simulations=1,source_header_count=chain['normal_argon_headers'],source_normal_argon_rechecked=False,source_argon_basis='Byte-identical original public chain and prior independently passed report; no new Argon expected values',source_scope='Genuine prior fixture evidence; inherited local-clock/admission/checkpoint limits remain. All new48 publication evidence is modeled.')

def materialize(c,bases):
 if 'raw'in c:return c['raw'].encode('utf8')
 if 'raw_hex'in c:return raw(c['raw_hex'])
 rec=c['recipe'];return bases[rec['base']]['canonical'].encode()+b' '*rec['append_space']

def verify_core(f):
 p=f['pin'];bases={x['name']:x for x in f['positive']}
 for context in ['core','transport']:
  with no_new_work(context=='transport'):
   for x in f['positive']:
    b=S.canonical(x['kind'],x['value'],p,context);assert b.decode()==x['canonical']and sha(b)==x['sha256']
 with no_new_work():
  for c in f['cases']:
   b=materialize(c,bases)
   try:v=S.parse(c['kind'],b,p);accepted=True
   except(Refusal,ValueError,TypeError,AssertionError,KeyError):accepted=False;v=None
   assert accepted==c['accept'],c['name']
   if accepted:assert J(v).decode()==c['canonical']
 # Direct admission controls, fixed purposes and exact preserved alternative tx.
 fct=f['facts'];assert R.valid_carrier(raw(fct['carrier']))
 for offset in [9,41]:
  b=bytearray(raw(fct['carrier']));b[offset:offset+32]=bytes(32);assert not R.valid_carrier(bytes(b))
 C=CRYPTO;C.selftest();x=CORE.examples();assert C.pubkey(2).hex()=='02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5';assert R.FUND!=C.pubkey(1).hex()
 with no_new_work():
  assert R.validate_publication(x['coin'],raw(fct['carrier']),fct['alternate_raw_tx'])['txid']==fct['transaction']['txid']
 return dict(positive=len(f['positive']),codec_cases=len(f['cases']),core_no_engine_or_signing=True,transport_no_generation_engine_signing_or_merkle_hashing=True)

def verify_store(f,p):
 for c in f['cases']:
  try:o=STORE.audit(p,c['rows']);accepted=True
  except(Exception):o=None;accepted=False
  assert accepted==c['accept'],c['name']
  if accepted:assert o==c['expected']
 return dict(row_cases=len(f['cases']),initialization_models=len(f['initialization']),index_rows=f['index_capacity']['entries'],index_canonical_bytes=f['index_capacity']['canonical_bytes'])

def verify_proof(f):
 assert PROOF.structural_verify(f['proof'])==f['structural_result']
 for c in f['cases']:
  v=clone(c.get('proof')or f['proof'])
  if'mutation'in c:
   m=c['mutation'];a=v
   for k in m['path'][:-1]:a=a[k]
   a[m['path'][-1]]=clone(m['value'])
  try:PROOF.structural_verify(v);accepted=True
  except(Exception):accepted=False
  assert accepted==c['structural_accept'],c['name'];assert c['normal_pow_claimed']is False
 for c in f['block_controls']:
  try:R.parse_result_block(raw(c['raw_block']));accepted=True
  except(Exception):accepted=False
  assert accepted==c['structural_accept'],c['name']
 return dict(proof_dag_cases=len(f['cases']),block_controls=len(f['block_controls']),normal_pow_checked=False,normal_consensus_model_claim=False)

def build():
 core=CORE.core_fragment();store=STORE.store_fragment();proof=PROOF.proof_fragment();proof['block_controls']=PROOF.block_controls();wire=WIRE.build_fragment();state=STATE.build_fragment()
 checks=dict(core=verify_core(core),store=verify_store(store,core['pin']),proof=verify_proof(proof));WIRE.self_check(wire);STATE.self_check(state);checks['wire']=wire['counts'];checks['state']=dict(cases=len(state['cases']),steps=sum(len(c['steps'])for c in state['cases']))
 corpus=dict(version=1,scope='independent_candidate_result_publication_v1',contract_sha256=INDEX['inputs/contracts/set48-core-contract-draft.md']['sha256'],helper_contract_sha256=INDEX['inputs/contracts/set48-helper-contract-draft.md']['sha256'],clarifications=[INDEX['inputs/contracts/set48-contract-clarification-'+str(i)+'.md']['sha256']for i in [1,2]],input_index_sha256=sha((HERE/'input_index.json').read_bytes()),source_index_sha256=sha((HERE/'source_index.json').read_bytes()),extractions=EXTRACTIONS,provenance=source_provenance(),core=core,store=store,proof=proof,wire=wire,state=state)
 return corpus,checks

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--generate',action='store_true');ap.add_argument('--check',action='store_true');ap.add_argument('--report');args=ap.parse_args();before=sources();ib=inputs();started=time.monotonic();corpus,checks=build();encoded=J(corpus)+b'\n'
 if args.generate:
  (HERE/'corpus.json').write_bytes(encoded)
  for name in ['core','store','proof','wire','state']:
   path=HERE/'fragments'/f'{name}.json';expected=STATE.canonical(corpus[name])if name=='state'else pretty(corpus[name])
   if name in ['wire','state']:
    assert path.read_bytes()==expected,'helper fragment drift; owner must approve'
   else:path.write_bytes(expected)
 else:assert (HERE/'corpus.json').read_bytes()==encoded,'corpus byte mismatch'
 assert sources()==before and inputs()==ib,'source/input changed during verification'
 report=dict(status='passed',findings=[],python=sys.version,executable=sys.executable,corpus_sha256=sha(encoded),corpus_bytes=len(encoded),source_sha256=before,input_index_sha256=sha((HERE/'input_index.json').read_bytes()),input_sha256=ib,checks=checks,provenance=corpus['provenance'],wall_seconds=round(time.monotonic()-started,6),limitations=['No Go/runtime output used as new expectations.','Added publication/proof headers are explicit unmined models; no new normal-PoW/UTXO/network/clock authority claim.','Codec verification invokes no engine/signing; one independent frozen42 engine recomposition verifies the genuine saved source.'])
 if args.report:pathlib.Path(args.report).write_bytes(pretty(report))
 print(json.dumps({k:report[k]for k in ['status','corpus_sha256','corpus_bytes','checks','wall_seconds']},sort_keys=True))
if __name__=='__main__':main()
