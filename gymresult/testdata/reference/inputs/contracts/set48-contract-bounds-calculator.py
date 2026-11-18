#!/usr/bin/env python3
"""Only impossible conservative JSON shapes; no vectors/crypto/engine/runtime."""
import ast,datetime,hashlib,json,pathlib,re
W=pathlib.Path('/Users/taytrrip/slug')
H=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
paths=[W/'set48-core-contract-draft.md',W/'set48-helper-contract-draft.md']
before={str(p):H(p)for p in paths}
old=W/'set47-contract-bounds-calculator.py'
# Execute only inherited shape constructors, stopping before validation/output.
ns={};nodes=[]
for n in ast.parse(old.read_text()).body:
 if isinstance(n,ast.Assign)and any(isinstance(t,ast.Name)and t.id=='text'for t in n.targets):break
 nodes.append(n)
exec(compile(ast.Module(body=nodes,type_ignores=[]),str(old),'exec'),ns)
obj,size,depth,N,T,X=[ns[x]for x in ('obj','size','depth','N','T','X')]
e=ns['ns'];b=ns
pin=obj('version source_store_id source_pin publisher_gym_id publisher_account_id publisher_source_authority_id publisher_chain_id network genesis floor_height floor_hash publisher_public_key checkpoint_public_key funding_public_key funding_script rules_id',[N,X,b['pin'],X,X,X,X,N,X,N,X,X,X,'f'*66,'f'*44,X])
summary=obj('winner method end_round end_tick',[X,'double_corner_stoppage',N,N])
source=obj('version store_id store_created_at_ms record_digest source_digest input_id result_digest created_at_ms source summary',[N,X,T,X,X,X,X,T,b['source'],summary])
statement=obj('version result_key store_id source_store_id source publisher_chain_id network genesis rules_id publisher_public_key checkpoint_public_key',[N,X,X,X,source,X,N,X,X,X,X]);signed=obj('version statement signature',[N,statement,'f'*128])
publication=dict(e['publication']);publication['state']='dispatch_unknown'
tip,cursor,restart,checkpoint=[e[x]for x in ('tip','cursor','restart','checkpoint')]
anchor=dict(e['anchor'])
# Anchor's five unsigned numeric fields are uint32, not MaxSafeInteger.
for k in ('height','raw_tx_size','tx_count','tx_index','output_index'):anchor[k]=4294967295
inclusion=obj('version generation anchor observed_at_ms observed_tip',[N,N,anchor,T,tip])
prior=obj('version inclusion orphaned_at_ms observed_tip replacement_hash',[N,inclusion,T,tip,X])
finality=obj('checkpoint inclusion observed_at_ms observed_tip descendants',[checkpoint,inclusion,T,tip,N])
conflict=obj('kind observed_at_ms expected_height expected_hash observed_hash observed_tip checkpoint',['checkpoint_chain_conflict',T,N,X,X,tip,checkpoint])
observation=obj('observed_at_ms chain_observed_at_ms tip node_state descendants current_finality',[T,T,tip,'offline',N,False])
record=obj('version store_id result_key statement prepared_at_ms publication cursor scan_restarts inclusion prior_inclusions checkpoint finality equivocation conflicts conflict_overflow observation',[N,X,X,signed,T,publication,cursor,[restart]*16,inclusion,[prior]*16,checkpoint,finality,signed,[conflict]*8,False,observation])
status=obj('version store_id result_key card_id bout_id event exists phase record_digest source_record_digest input_id result_digest prepared_at_ms publication_state txid scan_height scan_tip_height scan_generation inclusion_height inclusion_hash inclusion_generation prior_inclusion_count descendants checkpoint_present finalized_at_ms conflict_count conflict_overflow observed_at_ms chain_observed_at_ms node_state funding_state current_finality source',[N,X,X,X,X,ns['e']['event'](),False,'provisional',X,X,X,X,T,'dispatch_unknown',X,N,N,N,N,X,N,N,N,False,T,N,False,T,T,'offline','waiting_for_funds',False,b['status']])
action=obj('version record status',[N,record,status]);info=obj('version store_id pin source_created_at_ms created_at_ms result_count funding_height funding_hash funding_complete funding_generation funding_restart_count',[N,X,pin,T,T,N,N,X,False,N,N]);marker=obj('application version store_id pin',['slug-candidate-results',N,X,pin]);metadata=obj('version store_id pin source_created_at_ms created_at_ms clock_ms result_count funding_cursor funding_restarts funding_complete last_funding_tip',[N,X,pin,T,T,T,N,cursor,[restart]*16,False,tip]);init=obj('version store_id pin source_created_at_ms',[N,X,pin,T])
view=dict(e['view']);view['authority_origin']='\x01'*256 # six-byte escaping upper bound, intentionally not a valid origin
snapshot=obj('version state error view store_id pin source_created_at_ms created_at_ms publisher_account_id node_state node_owner active_job_id queued_job_id last_job_id',[N,'setup_incomplete','result_source_incomplete',view,X,pin,T,T,X,'starting','publication',X,X,X]);proofsummary=obj('version result_key card_id bout_id record_digest source_record_digest input_id result_digest txid sha256 bytes header_count block_count finalized_at_ms observed_tip',[N,X,X,X,X,X,X,X,X,X,N,N,N,T,tip]);job=obj('version job_id kind state view store_id card_id saturday bout_id result_key error result export',[N,X,'reconcile','completed',view,X,X,'2099-12-26',X,X,'result_source_incomplete',action,None]);request=obj('id command view_id saturday bout_id job_id path',[N,'result_reconcile',X,'2099-12-26',X,X,'\x01'*1024]);reply=obj('id ok view store_id snapshot status record job error',[N,False,view,X,None,None,None,None,'result_source_incomplete'])
# Parent inherited v5 marker, additive closed suffixes only.
parent=dict(e['parent'])
for k in ('card','bout','result'):parent.update({k+'_schema':N,k+'_phase':'waiting_source',k+'_store_id':X})
tuple_=obj('job_id kind state card_id saturday bout_id result_key terminal_digest',[X,'reconcile','completed',X,'2099-12-26',X,X,X]);retained=obj('snapshot selected_status correlation render_identity',[snapshot,status,[tuple_]*3,'x'*4096])
indexrow=obj('result_key saturday bout_id record_digest',[X,'2099-12-26',X,X])
shapes=dict(Pin=pin,Info=info,Marker=marker,Metadata=metadata,Init=init,ResultSource=source,ResultSummary=summary,Statement=statement,SignedStatement=signed,Publication=publication,Anchor=anchor,Inclusion=inclusion,PriorInclusion=prior,ScanRestart=restart,Conflict=conflict,Record=record,Status=status,ActionResult=action,ReadResult=action,ResultSnapshot=snapshot,ResultJob=job,ResultProofSummary=proofsummary,ResultRequest=request,ResultResponse=reply,ParentV8Marker=parent,RetainedMetadata=retained,PrivateIndexRow=indexrow)
ct,ht=[p.read_text()for p in paths]
def snake(v):
 if v=='TxID':return 'txid' # inherited exact tag; confirmed in core clarification
 return re.sub(r'([a-z0-9])([A-Z])',r'\1_\2',re.sub(r'(.)([A-Z][a-z]+)',r'\1_\2',v)).lower()
def struct_fields(txt,name):
 m=re.search(r'type '+name+r' struct \{(.*?)\n\}',txt,re.S);assert m,name
 return [snake(x)for x in re.findall(r'^\s+([A-Z][A-Za-z0-9]*)\s+',m.group(1),re.M)]
for name,value in [('ResultSummary',summary),('ResultSource',source)]:assert list(value)==struct_fields(ct,name),(name,list(value),struct_fields(ct,name))
for name in ['Info','Statement','SignedStatement','Publication','Inclusion','PriorInclusion','Finality','Conflict','Observation','Record','Status','ActionResult','ReadResult']:
 m=re.search(r'^'+name+r'\{(.*?)\}',ct,re.M|re.S);assert m,name
 expected=[snake(x.strip().split()[0])for x in m.group(1).split(',')]
 v=shapes.get(name,{'Finality':finality,'Observation':observation}.get(name));assert list(v)==expected,(name,list(v),expected)
for name in ['ResultRequest','ResultResponse','ResultSnapshot','ResultJob']:assert list(shapes[name])==struct_fields(ht,name),(name,list(shapes[name]),struct_fields(ht,name))
assert re.search(r'Export\s+\*gymresult.ProofSummary',ht)
caps=dict(Pin=32768,Info=36864,Marker=36864,Metadata=65536,ResultSource=20480,Statement=24576,SignedStatement=25600,Publication=4096,Anchor=2048,Inclusion=3072,PriorInclusion=4096,ScanRestart=1024,Conflict=2048,Record=196608,Status=16384,ActionResult=229376,ReadResult=229376,ResultJob=245760,ResultRequest=8192,ParentV8Marker=16384,RetainedMetadata=49152)
rows={k:dict(bytes=size(v)+(1 if k in ('Marker','ParentV8Marker','ResultRequest')else 0),container_depth=depth(v),cap=caps.get(k))for k,v in shapes.items()}
# Independent nested caps are allowed to overestimate impossible simultaneous maxima.
nested={
 'pin_at_bout_pin_cap':size(pin)+24576-size(b['pin']),
 'source_at_bout_source_cap':size(source)+16384-size(b['source']),
 'statement_at_source_cap':size(statement)+20480-size(source),
 'signed_at_statement_cap':size(signed)+24576-size(statement),
 'record_at_all_nested_caps':size(record)+2*(25600-size(signed))+(4096-size(publication))+16*(1024-size(restart))+(3072-size(inclusion))+16*(4096-size(prior))+(3072-size(inclusion))+8*(2048-size(conflict)),
 'status_at_bout_status_cap':size(status)+8192-size(b['status']),
 'action_at_record_status_caps':size(action)+196608-size(record)+16384-size(status),
 'job_at_action_cap':size(job)+229376-size(action),
 'metadata_at_pin_and_status_caps':size(retained)+32768-size(pin)+16384-size(status),
 'metadata_at_exact_pin_and_status_caps':size(retained)+16384-size(status),
 'metadata_at_pin_cap_exact_status':size(retained)+32768-size(pin),
 'journal_metadata_at_pin_restart_caps':size(metadata)+32768-size(pin)+16*(1024-size(restart)),
}
responses={}
for k,v in [('snapshot',snapshot),('status',status),('record',action),('job',job)]:
 q=dict(reply);q[k]=v;responses[k]=dict(bytes_including_LF=size(q)+1,container_depth=depth(q))
responses['job_at_action_cap']=dict(bytes_including_LF=responses['job']['bytes_including_LF']+229376-size(action),container_depth=responses['job']['container_depth'])
# Full public export: sum independent nested caps plus exact wrapper/headers.
# Raw block hex contribution computed arithmetically; never allocate24MB body.
n46={};parts=[];old46=W/'set46-contract-bounds-calculator.py'
for n in ast.parse(old46.read_text()).body:
 parts.append(n)
 if isinstance(n,ast.Assign)and any(isinstance(t,ast.Name)and t.id=='shapes'for t in n.targets):break
exec(compile(ast.Module(body=parts,type_ignores=[]),str(old46),'exec'),n46)
header=obj('height hash header',[N,X,'f'*160]);block=obj('height hash raw_block',[N,X,''])
proof=obj('version scope pin result status bout card observed_tip result_finality_tip source_finality_tip headers blocks',[N,'candidate_result_proof_v1',pin,record,status,b['record'],n46['record'],tip,tip,tip,[header]*512,[block]*3])
for k,v in [('ProofHeader',header),('ProofBlock',block),('PublicProof',proof),('ProofSummary',proofsummary)]:assert list(v)==struct_fields(ct,k),(k,list(v),struct_fields(ct,k))
proof_bytes=size(proof)+1+3*8000000+32768-size(pin)+196608-size(record)+16384-size(status)+237568-size(b['record'])+196608-size(n46['record'])
proof_info=dict(bytes_at_nested_caps_including_LF=proof_bytes,cap=25165824,headroom=25165824-proof_bytes,container_depth=depth(proof),maximum_unique_headers=512,maximum_full_bodies=3,maximum_binary_bytes_per_body=4000000,full_body_hex_bytes=24000000,proof_summary_bytes=size(proofsummary),proof_summary_cap=2048)
assert proof_bytes<=25165824 and depth(proof)<=20 and size(proofsummary)<=2048
# Rooted arrays from overfull nullable arms.

arrays={}
def walk(v,p):
 if isinstance(v,dict):
  for k,x in v.items():walk(x,p+'.'+k if p else k)
 elif isinstance(v,list):
  arrays[p]=len(v)
  if v:walk(v[0],p+'[*]')
for p,v in [('snapshot',snapshot),('record',action),('job',job)]:walk(v,p)
assert size([indexrow]*256)<=98304
assert nested['job_at_action_cap']<=245760 and nested['metadata_at_exact_pin_and_status_caps']<=49152
assert responses['job_at_action_cap']['bytes_including_LF']<=262144
assert nested['record_at_all_nested_caps']<=196608 and nested['action_at_record_status_caps']<=229376
assert max(depth(v)for v in shapes.values())<=20
findings=[dict(shape=k,bytes=x['bytes'],cap=x['cap'])for k,x in rows.items()if x['cap']and x['bytes']>x['cap']]
report=dict(status='passed_formula_only',at_utc=datetime.datetime.now(datetime.timezone.utc).isoformat(),contract_sha256=before,calculator_sha256=H(pathlib.Path(__file__)),inherited_calculator_sha256={str(old):H(old),str(old46):H(old46),str(W/'set45-contract-bounds-calculator.py'):H(W/'set45-contract-bounds-calculator.py')},shapes=rows,nested_cap_upper_bytes=nested,responses=responses,public_proof=proof_info,array_paths=arrays,index256_canonical_bytes=size([indexrow]*256),logical_journal_upper_bytes=256*(196608+33)+65536+36864+128*(size(e['coin'])+37)+256*(160+37),findings=findings,input_changes=[p for p,h in before.items()if H(pathlib.Path(p))!=h],assumptions=['All numbers use16digits even where uint32 or tiny enum bounds apply. False uses5bytes. Every optional arm populated, even incompatible ones.','Fixed hex/ASCII fields avoid JSON escapes; source inherited conditions/owners use previously reviewed conservative bounded schemas. No full transcript in wire shapes.','Render identity4096 is charged as combined encoded contribution, never raw user text.','Formula shapes deliberately invalid: no simulations, cryptographic signatures, Go outputs or reference vectors.','Public proof uses independent full nested caps, not smaller finite source examples. Private index exact4fields at256 rows is canonical/accounted96KiB bound, not native heap bytes.','Path1024 decoded UTF8 bytes conservatively charged6escapedbytes each; inherited View origin256 similarly overcharged. These invalid strings are only bounds, never valid expected inputs.'])
assert not report['input_changes'],report['input_changes']
out=W/'set48-contract-bounds-formula.json';out.write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({k:report[k]for k in ['status','shapes','nested_cap_upper_bytes','responses','index256_canonical_bytes','public_proof','findings']},indent=2))
