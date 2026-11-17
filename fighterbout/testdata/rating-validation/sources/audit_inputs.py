#!/usr/bin/env python3
"""Input/byte/provenance audit only. Never simulates or fits."""
import collections,gzip,json,pathlib,struct,time
import common as C
import prepare_inputs as P

def main():
 start=time.monotonic();before=C.check_pins();cf=C.read_json(C.DATA/'coefficient-input-freeze.json');pf=C.read_json(C.DATA/'population-input-freeze.json')
 for f in (cf,pf):
  for name,h in f['files'].items():assert C.filehash(C.DATA/name)==h
 oldids=set();oldinputs=set()
 for r in C.rows(C.OLD/'calibration/inputs.jsonl.gz'):
  oldinputs.add(r['input_id']);oldids.update(r['input'][k]['prospect_id'] for k in ('a','b'))
 for r in C.read_json(C.OLD/'bout_v1.json')['cases']:
  oldinputs.add(r['input_id']);oldids.update(r['input'][k]['prospect_id'] for k in ('a','b'))
 for r in C.read_json(C.OLD/'preview_transport_v1.json')['cases']:
  v=json.loads(r['canonical_input']);oldinputs.add(r['input_id']);oldids.update(v[k]['prospect_id'] for k in ('a','b'))
 train=[r for r in C.rows(C.OLD/'calibration/outcomes.jsonl.gz') if r['partition']=='train'];raw=(C.DATA/'training.dat').read_bytes();assert raw[:16]==b'D75TRN01'+C.u32(4000)+C.u32(5)
 expected=b''.join(struct.pack('<5iI',*r['league_differences'],r['outcome']+1) for r in train);assert raw[16:]==expected
 ix=(C.DATA/'indices.dat').read_bytes();assert ix[:16]==b'D75IDX01'+C.u32(1000)+C.u32(4000);key=bytes.fromhex(C.sha(b'SLUG D75 coefficient bootstrap\0v1\0'));budget=[0,10000000]
 for rep in range(1000):
  for cl in range(5):
   rng=C.Stream(key,C.u32(rep)+C.u32(cl),budget);expected=struct.pack('<800H',*[cl*800+rng.uniform(800) for _ in range(800)]);startix=16+rep*8000+cl*1600;assert ix[startix:startix+1600]==expected
 assert budget[0]==cf['raw_words']
 schedules={r['unit']:r for r in C.rows(C.DATA/'population-schedules.jsonl.gz')};unit_rows=collections.defaultdict(list);seen=set();ids=set();count=0
 for r in C.rows(C.DATA/'population-inputs.jsonl.gz'):
  inp=r['input'];assert C.B.validate_input(inp)==inp;assert C.B.input_id(inp)==r['input_id'] and r['input_id'] not in seen|oldinputs;seen.add(r['input_id']);assert inp['as_of_ms']==C.ASOF
  for k in ('a','b'):assert inp[k]['prospect_id'] not in ids|oldids;ids.add(inp[k]['prospect_id'])
  unit=r['unit'];i=r['row_index'];s=schedules[unit];assert tuple(s['cells'][i])==(r['age_pair_code'],r['xp_gap_code']) and s['xp_pairs'][i]==r['original_xp'];order=r['canonical_original_corner_order'];assert sorted(order)==[0,1]
  for ci,k in enumerate(('a','b')):
   oc=order[ci];f=inp[k];seed=C.sha(b'SLUG D75 coverage manifest\0v1\0'+C.frame(unit)+C.u32(i)+C.u32(r['pair_attempt'])+C.u32(oc)+C.u32(r['class_attempts'][oc]));assert seed==f['manifest']['dna']['seed'];assert f['manifest']['physical']['weight_class']==r['class'];pid=C.sha(b'SLUG D75 coverage identity\0v1\0'+bytes.fromhex(seed)+C.frame(unit)+C.u32(i)+C.u32(oc));assert pid==f['prospect_id'];age=P.ageweeks(f['manifest'],r['original_age_positions'][oc]);assert age==r['original_age_weeks'][oc] and f['born_at_ms']==C.ASOF-age*C.WEEK and f['pro_xp']==r['original_xp'][oc] and f['training_skill']==0
   c=C.B.CONDITION.project(1,f['manifest']['dna'],f['born_at_ms'],C.ASOF,f['pro_xp']);assert c==r['canonical_conditions'][ci] and c['rating_hundredths']==r['incumbent_ratings'][ci] and c['phase']!='retired'
  assert inp['seed']==C.sha(b'SLUG D75 coverage bout\0v1\0'+C.frame(unit)+C.u32(i));lo,hi=min(r['incumbent_ratings']),max(r['incumbent_ratings']);assert (r['ratio_numerator'],r['ratio_denominator'])==(hi,lo)
  assert r['panel']=='natural' or (r['panel']=='close' and 20*hi<=21*lo) or (r['side']=='inside' and 9*lo<=5*hi and hi<=2*lo) or (r['side']=='outside' and hi>2*lo and 5*hi<=11*lo)
  unit_rows[unit].append(r);count+=1
 assert count==20000 and len(ids)==40000 and len(unit_rows)==80
 schedule_budget=[0,1000000]
 for unit,rs in unit_rows.items():
  n=100 if rs[0]['side'] else 400;assert [r['row_index'] for r in rs]==list(range(n));pop=rs[0]['population'];design=P.cells(pop,n);rng=C.Stream(bytes.fromhex(C.sha(b'SLUG D75 coverage schedule\0v1\0'+C.frame(unit)+C.frame('design'))),budget=schedule_budget);rng.shuffle(design);xr=C.Stream(bytes.fromhex(C.sha(b'SLUG D75 coverage schedule\0v1\0'+C.frame(unit)+C.frame('xp-pair'))),budget=schedule_budget);xp=[list(P.XP_CHOICES[g][xr.uniform(len(P.XP_CHOICES[g]))]) if pop.endswith('xpvar') else [0,0] for a,g in design]
  assert [list(v) for v in design]==schedules[unit]['cells'] and xp==schedules[unit]['xp_pairs'];assert rng.counter==schedules[unit]['design_raw_words'] and xr.counter==schedules[unit]['xp_raw_words']
 assert schedule_budget[0]==pf['schedule_raw_words'];assert before==C.check_pins()
 prefix=C.ROOT/'preparation-attempts/attempt-001/data/population-inputs.jsonl.gz';prefix_count=0
 if prefix.exists():
  with gzip.open(prefix,'rb') as old,gzip.open(C.DATA/'population-inputs.jsonl.gz','rb') as new:
   for line in old:assert line==new.readline();prefix_count+=1
  assert prefix_count==14151
 C.write_json(C.DATA/'input-audit.json',{'status':'passed','at':C.now(),'seconds':time.monotonic()-start,'families':count,'unique_fighter_ids':len(ids),'original_input_ids_compared':len(oldinputs),'original_fighter_ids_compared':len(oldids),'disjoint_original_inputs':True,'disjoint_original_fighter_ids':True,'exact_failed_attempt_prefix_rows':prefix_count,'exact_regenerated_bootstrap_words':budget[0],'exact_regenerated_schedule_words':schedule_budget[0],'no_fits_or_fights':True,'source_changes':[],'files':{p:C.filehash(C.DATA/p) for p in ('training.dat','indices.dat','population-inputs.jsonl.gz','population-schedules.jsonl.gz')}})
 print('Input audit passed; no fights or fits executed.',flush=True)
if __name__=='__main__':main()
