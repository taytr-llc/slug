"""Independent cross-checks of the reference itself. No implementation execution."""
import datetime,io,sys,subprocess
from fractions import Fraction
from zoneinfo import ZoneInfo
from reference_card import *

def verify():
 c=corpus();assert pretty(c)==(HERE/'card_v1.json').read_bytes()
 for p,m in INDEX.items():
  origin=m['origin']
  if origin.startswith('git:'):
   repo,ref=origin[4:].split('@',1);b=subprocess.check_output(['git','-C',repo,'show',ref])
  else:b=Path(origin).read_bytes()
  assert sha(b)==m['origin_sha256'],origin
  if m['json_pointer'] is not None:assert J(pointer(json.loads(b),m['json_pointer']))==(HERE/p).read_bytes()
 # Gregorian standard-library/date + only the exact pinned TZif, never host tzdata.
 tzraw=(HERE/'inputs/source44/America_Los_Angeles.tzif').read_bytes();assert len(tzraw)==1294
 tz=ZoneInfo.from_file(io.BytesIO(tzraw));seen=set();gaps={167:0,168:0,169:0}
 for x in c['calendar']:
  d=datetime.date.fromisoformat(x['saturday']);assert d.weekday()==5
  now=datetime.datetime.combine(d,datetime.time(12),tzinfo=tz);nxt=datetime.datetime.combine(d+datetime.timedelta(days=7),datetime.time(12),tzinfo=tz)
  assert int(now.timestamp())*1000==x['event']['cutoff_ms'];assert int(nxt.timestamp())*1000==x['event']['incident_deadline_ms'];gaps[x['gap_hours']]+=1
  assert x['card_id']==sha(bytes.fromhex(x['card_preimage']));assert x['card_id'] not in seen;seen.add(x['card_id'])
 assert len(seen)==3861 and all(gaps.values())
 # Exhaustive exact integer band boundary cross-check with rational division.
 count=0;digest=hashlib.sha256()
 for a in range(1,50001):
  for b in [2*a-1,2*a,min(2*a+1,100000)]:
   expected=Fraction(1,2)<=Fraction(a,b)<=2
   assert band(a,b)==expected==band(b,a);digest.update(u32(a)+u32(b)+bytes([expected]));count+=1
 # Separate class-pair table cross-check: reason map does not call match().
 match_count=0
 for x in c['matcher_cases']:
  if not x['accept']:continue
  ps=x['plan'];xs=x['inputs'];table={}
  for e in xs:table.setdefault(e['class_'],{})[e['gym_id']]=e
  expected_reason={};pairs=[]
  for cls in CLASSES:
   by=table.get(cls,{})
   for g,a in by.items():
    if a['cutoff_eligibility']!='eligible':why={'retired':'retired_at_cutoff','not_rostered':'not_rostered_at_cutoff'}[a['cutoff_eligibility']]
    else:
     b=by.get(next(gg for gg,_ in GYMS if gg!=g))
     if b is None:why='no_opponent_in_class'
     elif b['cutoff_eligibility']!='eligible':why='opponent_ineligible_at_cutoff'
     elif not Fraction(1,2)<=Fraction(a['rating_hundredths'],b['rating_hundredths'])<=2:why='outside_rating_band'
     else:why=''
    expected_reason[a['prospect_id']]=why
   if len(by)==2 and all(expected_reason[v['prospect_id']]=='' for v in by.values()):pairs.append(tuple(sorted(v['prospect_id'] for v in by.values())))
  assert {e['prospect_id']:e['reason'] for e in ps['entrants']}==expected_reason
  assert [(b['corner_a'],b['corner_b']) for b in ps['bouts']]==pairs
  assert len(xs)==ps['counts']['total_count']==ps['counts']['paired_count']+ps['counts']['unmatched_count']
  assert ps['counts']['paired_count']==2*len(pairs);assert sum(ps['counts']['reasons'].values())==ps['counts']['unmatched_count'];match_count+=1
 required=next(x for x in c['matcher_cases'] if x['name']=='all_reasons_and_one_pair')['plan'];assert required['bout_count']==1 and all(required['counts']['reasons'].values())
 five=next(x for x in c['matcher_cases'] if x['name']=='five_classes_all_paired');assert five['accept'] and five['plan']['bout_count']==5
 inputs=five['inputs'];p=match(inputs,five['card_id'],five['input_id'],five['seed']);assert p==match(list(reversed(inputs)),five['card_id'],five['input_id'],five['seed'])
 # Evidence-only changes affect only SourceDigest, not immutable draw recipes.
 ids=c['identity_cases'];original=ids[0]
 for x in ids[1:4]:
  assert x['source_digest']!=original['source_digest']
  for k in ['card_id','input_id','seed','bout_ids','bout_seeds']:assert x[k]==original[k]
 assert ids[4]['card_id']==original['card_id'] and ids[4]['input_id']!=original['input_id'] and ids[4]['seed']!=original['seed']
 for x in c['dtos']:assert J(x['value']).decode()==x['canonical'] and sha(J(x['value']))==x['sha256']
 for x in c['codec_cases']:
  b=materialize(c['dtos'][x['base']]['value'],x['recipe']);assert len(b)==x['input_bytes'] and sha(b)==x['input_sha256']
  try:v=parse(x['kind'],b,x['context']);accepted=True
  except Refusal:accepted=False
  assert accepted==x['accept']
  if accepted:assert sha(J(v))==x['canonical_sha256']
 for x in c['protocol_cases']:
  try:correlate(x['request'],x['response'],x['current_view'],x['expected_job'],x['terminal_digest'],x['created_at_ms']);accepted=True
  except(ValueError,TypeError,KeyError):accepted=False
  assert accepted==x['accept'],x['name']
 # Historical replay cannot overwrite retained receipt/time, even at capacity.
 for scenario in c['scenarios']:
  observed={}
  for step in scenario['steps']:
   for key,dig in step['after']['record_digests'].items():
    if key in observed:assert observed[key]==dig
    observed[key]=dig
 for x in c['initialization']:
  if x['mode']=='resume_prepared' and x['shape'] in ['absent','empty_directory','marker_only','recognized_init','ready_empty']:assert x['accept']
  if x['mode']=='open' and x['shape'] in ['absent','empty_directory','marker_only','recognized_init']:assert not x['accept']
 h=c['history'];assert len(h['index'])==256 and all(x['full_record_validations']<=8 for x in h['cases'])
 return dict(status='passed',corpus_sha256=sha((HERE/'card_v1.json').read_bytes()),input_count=len(INDEX),calendar_count=len(seen),dst_gaps=gaps,band_boundary_checks=count,band_boundary_digest=digest.hexdigest(),valid_matcher_crosschecks=match_count,dtos=len(c['dtos']),codec_cases=len(c['codec_cases']),protocol_cases=len(c['protocol_cases']),journal_scenarios=len(c['scenarios']),history_index_rows=len(h['index']),source_input_claim='prior normal-work evidence reused unchanged; no new Argon or Go46 execution',python=sys.version.split()[0])
if __name__=='__main__':print(json.dumps(verify(),sort_keys=True))
