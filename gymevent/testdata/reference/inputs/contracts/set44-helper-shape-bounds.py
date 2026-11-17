#!/usr/bin/env python3
"""Formula-only conservative wrappers; no outcomes, manifests or valid golden DTOs."""
import ast, hashlib, json, pathlib, sys
root=pathlib.Path('/Users/taytrrip/slug')
core=root/'set44-core-shape-bounds.py'
inputs=[core,root/'set44-core-contract-draft.md',root/'set44-helper-contract-proposal.md',pathlib.Path(__file__),root/'slug/desktop/pilot/entry_types.go',root/'slug/desktop/pilot/manifest_types.go',root/'slug/desktop/accountpilot/entry_files.go']
hash_file=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
before={str(p):hash_file(p) for p in inputs}
# Reuse the reviewed deliberately oversized shape constructors, but do not run
# that script's writer/capture. This is shared-shape arithmetic, not a second oracle.
tree=ast.parse(core.read_text())
ns={'json':json,'hashlib':hashlib,'H':'f'*64,'N':9007199254740991,'T':8640000000000000}
exec(compile(ast.Module(body=[n for n in tree.body if isinstance(n,ast.FunctionDef)],type_ignores=[]),str(core),'exec'),ns)
obj=ns['obj']; size=ns['size']; H=ns['H']; N=ns['N']; T=ns['T']
view=obj('version view_id account_authority_id account_id generation authority_pin authority_origin transport_trust_id',[N,H,H,H,N,ns['stablepin']()['source'],'https://'+'x'*248,H])
snapshot=obj('version state error view store_id pin coordinator_created_at_ms participant_id active_job_id queued_job_id last_job_id',[N,'setup_incomplete','event_created_too_late',view,H,ns['pin'](),T,H,H,H,H])
result=obj('version status capture replayed',[N,ns['status'](),ns['capture'](),False])
job=obj('version job_id kind state view store_id event_id saturday participant_id error result',[N,H,'capture','completed',view,H,H,'2099-12-26',H,'event_created_too_late',result])
history=obj('version store_id after_event_id next_event_id has_more observed_at_ms events',[N,H,H,H,False,T,[ns['status']() for _ in range(8)]])
reply=obj('id ok view store_id snapshot status record history job error',[N,True,view,H,None,None,None,None,None,''])
responses={}
for key,val in [('snapshot',snapshot),('status',ns['status']()),('record',ns['receipt']()),('history',history),('job',job)]:
 v=dict(reply);v[key]=val;responses[key]=size(v)+1
stableinst=[obj('account_id pin phase',[H,ns['stablepin'](),'prepared']) for _ in range(2)]
entryinst=[obj('account_id pin phase',[H,ns['entrypin'](),'prepared']) for _ in range(2)]
marker=obj('version mode account_authority_id descriptor_id stable_schema stable_installs entry_schema entry_calendar_id entry_rules_id entry_installs event_schema event_phase event_store_id',[N,'local-manifest-pilot',H,H,N,stableinst,N,H,H,entryinst,N,'waiting_participants',H])
marker_size=size(marker)+1
retained=size(snapshot)+size(job)+size(ns['status']())
assert marker_size<=16384
assert max(responses.values())<=131072
assert retained<=131072
assert before=={str(p):hash_file(p) for p in inputs}
report={'status':'passed','kind':'formula_only_shared_oversized_shapes','valid_reference_or_outcome_vectors':False,'python':sys.version,'source_sha256':before,'source_unchanged':True,'response_bytes_including_lf':responses,'response_cap':131072,'v4_parent_marker_bytes_including_lf':marker_size,'parent_marker_cap':16384,'retained_snapshot_job_status_bytes':retained,'retained_cap':131072,'limitations':['Shared core oversized shape constructors; this is not an independent semantic oracle.','Impossible enum/numeric combinations deliberately overcount bounded widths.','Record/History are explicit transient reads, not duplicated retained helper cache.','Physical LevelDB and transient serializer memory are not retained-cache accounting.']}
out=root/'set44-helper-shape-bounds.json';out.write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({'report_sha256':hash_file(out),'responses':responses,'marker':marker_size,'retained':retained}))
