#!/usr/bin/env python3
"""Formula-only JSON envelope sizes, not valid inputs, reference outputs or simulations."""
import hashlib,json,pathlib,sys
ROOT=pathlib.Path('/Users/taytrrip/slug'); REPO=ROOT/'slug'
H='f'*64; N=9007199254740991; T=8640000000000000
# All variable numeric fields deliberately use 16 digits even where enums/caps
# permit far fewer. Public string lengths below exceed the two exact fixtures.
def obj(keys,values): return dict(zip(keys.split(),values))
def size(v):return len(json.dumps(v,ensure_ascii=False,separators=(',',':')).encode())
def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def attrs():return dict.fromkeys(('power','speed','chin','stamina','skill'),N)
def condition():
 return obj('version projector_id dna_version seed born_at_ms as_of_ms age_weeks phase elapsed_decline_weeks prime_remaining_weeks prime_remaining_ms career_remaining_ms pro_xp aged_attributes_tenths league_attributes_tenths rating_hundredths',[N,'x'*64,N,H,T,T,N,'retired',N,N,T,T,N,attrs(),attrs(),N])
def assignment():return obj('version sequence prospect_id gym_id work_hash born_at_ms accepted_at_ms descriptor_id manifest_digest portrait_digest source_record_digest',[N,N,H,H,H,T,T,H,H,H,H])
def stablepin():
 identity=obj('network genesis profile root_public_key gym_public_key gym_id',[N,H,'x'*64,'x'*256,'x'*66,H])
 source=obj('version network genesis authority_id descriptor_id',[N,N,H,H,H])
 return obj('version account_authority_id account_id identity source condition_version projector_id',[N,H,H,identity,source,N,'x'*64])
def entrypin():return obj('version namespace_id calendar_id rules_id stable_store_id stable_pin',[N,H,H,H,H,stablepin()])
def source_event():return obj('version event_id namespace_id calendar_id rules_id saturday cutoff_ms',[N,H,H,H,H,'2099-12-26',T])
def entrymember():return obj('slot assignment class condition_at_save',[N,assignment(),'light_heavy',condition()])
def entryreceipt():return obj('version store_id entry_id event operation_id operation_sequence generation view_id action previous_revision revision roster_revision stable_checkpoint observed_source_count committed_at_ms members',[N,H,H,source_event(),H,N,N,H,'withdraw',N,N,N,N,N,T,[entrymember() for _ in range(5)]])
def cutoffentry():return obj('version store_id event entry_id committed_count last_sequence history_digest latest_receipt',[N,H,source_event(),H,N,N,H,entryreceipt()])
def roster():return obj('version store_id cutoff_ms committed_count last_sequence history_digest revision slots assigned_at_ms',[N,H,T,N,N,H,N,[H]*10,[T]*10])
def fence():return obj('version store_id cutoff_ms observed_at_ms operation_count',[N,H,T,T,N])
def member():return obj('slot slot_assigned_at_ms assignment class condition_at_cutoff eligibility',[N,T,assignment(),'light_heavy',condition(),'not_rostered'])
def owner():return obj('ownership_basis account_authority_id account_id gym_id',['candidate_assignment',H,H,H])
def participant():return obj('version participant_id entry_store_id owner roster entry members',[N,H,H,owner(),roster(),cutoffentry(),[member() for _ in range(5)]])
def evidence():return obj('version participant_id stable_fence entry_fence captured_at_ms',[N,H,fence(),fence(),T])
def capture():return obj('version snapshot evidence',[N,participant(),evidence()])
def pin():return obj('version calendar_id rules_id participants',[N,H,H,[entrypin(),entrypin()]])
def event():return obj('version event_id source_event_id store_id namespace_id calendar_id rules_id saturday cutoff_ms incident_deadline_ms',[N,H,H,H,H,H,H,'2099-12-26',T,T])
def inp():return obj('version event participants',[N,event(),[participant(),participant()]])
def receipt():return obj('version input snapshot_digest captures completed_at_ms',[N,inp(),H,[evidence(),evidence()],T])
def status():return obj('version store_id event exists phase captured_participants snapshot_digest coordinator_created_at_ms created_at_ms completed_at_ms observed_at_ms seed_authority',[N,H,event(),False,'awaiting_capture',[H,H],H,T,T,T,T,'unavailable'])
paths=[ROOT/'set44-core-contract-draft.md',pathlib.Path(__file__),REPO/'gymstable/types.go',REPO/'gymaccount/types.go',REPO/'gymaccount/fixtures.go',REPO/'gymentry/types.go',REPO/'fightercondition/condition.go',REPO/'registry/manifest_types.go']
before={str(p):digest(p) for p in paths}
shapes={
 'Pin':(pin(),16384),
 'Marker':(obj('application version store_id pin',['slug-candidate-events',N,H,pin()]),16383),
 'InitIntent':(obj('version store_id pin',[N,H,pin()]),20480),
 'Metadata':(obj('version store_id pin created_at_ms clock_ms event_count',[N,H,pin(),T,T,N]),20480),
 'Info':(obj('version store_id pin created_at_ms event_count',[N,H,pin(),T,N]),20480),
 'Event':(event(),2048),'EventStatus':(status(),2048),
 'CutoffRoster':(roster(),4096),'CutoffEntry':(cutoffentry(),36864),
 'CutoffMember':({k:v for k,v in member().items() if k!='eligibility'},4096),
 'CutoffSelection':(obj('version roster members',[N,roster(),[{k:v for k,v in member().items() if k!='eligibility'}for _ in range(5)]]),61440),
 'CutoffSnapshot':(obj('version entry roster members',[N,cutoffentry(),roster(),[{k:v for k,v in member().items() if k!='eligibility'}for _ in range(5)]]),61440),
 'Member':(member(),4096),'ParticipantSnapshot':(participant(),57344),
 'CaptureEvidence':(evidence(),2048),'ParticipantCapture':(capture(),61440),
 'SnapshotInput':(inp(),122880),'SnapshotReceipt':(receipt(),126976),
 'CaptureResult':(obj('version status capture replayed',[N,status(),capture(),False]),131072),
 'HistoryPage':(obj('version store_id after_event_id next_event_id has_more observed_at_ms events',[N,H,H,H,False,T,[status() for _ in range(8)]]),131072),
 'EventRow':(obj('version event phase created_at_ms completed_at_ms snapshot_digest',[N,event(),'awaiting_capture',T,T,H]),2048),
}
results={name:{'upper_envelope_bytes':size(v),'cap_bytes':cap,'passes':size(v)<=cap}for name,(v,cap)in shapes.items()}
# Combine the largest measured result with a deliberately overwide 8KiB envelope.
results['LargestCoreResultPlus8KiBEnvelope']={'upper_envelope_bytes':max(size(shapes[x][0])for x in ['SnapshotReceipt','CaptureResult','HistoryPage'])+8192,'cap_bytes':131072}
results['LargestCoreResultPlus8KiBEnvelope']['passes']=results['LargestCoreResultPlus8KiBEnvelope']['upper_envelope_bytes']<=131072
# Logical reserved row caps, not LevelDB WAL/compaction/filesystem space.
logical=256*(2048+2*61440+126976)+20480+16384
logical_keys=256*(33+2*65+33)+4
assert logical+logical_keys<64*1024*1024
assert all(v['passes']for v in results.values())
after={str(p):digest(p)for p in paths};assert before==after
report={'status':'passed','kind':'formula_only_max_shape','claims_no_valid_vectors_or_engine_execution':True,'python':sys.version,'reviewed_sha256':before,'source_changes':[],'conservative_assumptions':['All numeric fields widened to16 decimal digits, even versions/counters/attributes','Root public key256bytes; profile/projector64bytes, exceeding exact fixture/selector lengths','All nullable receipt positions populated and all arrays at declared cardinality','Unknown keys/strings remain forbidden; these structurally oversized envelopes are not logical positive fixtures'], 'shapes':results,'logical_reserved_value_bytes':logical,'logical_key_bytes':logical_keys,'logical_total_upper_bytes':logical+logical_keys,'logical_value_limit':64*1024*1024,'physical_leveldb_overhead_included':False}
(ROOT/'set44-core-shape-bounds.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({'status':report['status'],'largest_receipt_envelope':results['SnapshotReceipt'],'status_envelope':results['EventStatus'],'reserved_values':logical,'report_sha256':digest(ROOT/'set44-core-shape-bounds.json')}))
