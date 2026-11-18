"""Independent Set48 examples and core vectors from genuine saved47 source.
Result signatures/transactions/inclusions are explicit hypothetical models.
"""
from common import *
import crypto_result as R
import schema_core as S

def modeled_anchor(pub,payload,height,branch):
 # These structurally consistent headers are NOT normal Argon/consensus evidence.
 root,proof=CRYPTO.merkle([pub['txid']]);header=u32(4)+raw(label(branch+' parent'))[::-1]+raw(root)[::-1]+u32(1795896001)+u32(0x207fffff)+u32(height)
 return dict(height=height,hash=label(branch+' modeled block '+str(height)),header=header.hex(),txid=pub['txid'],raw_tx_digest=sha(raw(pub['raw_tx'])),raw_tx_size=len(raw(pub['raw_tx'])),tx_count=1,tx_index=0,output_index=0,payload=payload.hex(),merkle_branch=proof)

def examples():
 p=R.candidate_pin(BOUT_PIN);src=R.source_from_record(BOUT_RECORD,BOUT_SNAPSHOT);now=max(src['created_at_ms'],src['store_created_at_ms'])+1000;ss=load('inputs/actual47/bout_status.json')
 ss['observed_at_ms']=ss['source']['observed_at_ms']=now+10000
 info=dict(version=1,store_id=R.store_id(p),pin=p,source_created_at_ms=BOUT_SNAPSHOT['created_at_ms'],created_at_ms=now-1,result_count=0,funding_height=-1,funding_hash='',funding_complete=False,funding_generation=0,funding_restart_count=0)
 signed=R.sign_statement(R.statement(p,src));key=signed['statement']['result_key'];r=dict(version=1,store_id=R.store_id(p),result_key=key,statement=signed,prepared_at_ms=now,publication=None,cursor=R.scan_initial(p,key),scan_restarts=[],inclusion=None,prior_inclusions=[],checkpoint=None,finality=None,equivocation=None,conflicts=[],conflict_overflow=False,observation=dict(observed_at_ms=0,chain_observed_at_ms=0,tip=None,node_state='unknown',descendants=0,current_finality=False))
 records={'prepared':clone(r)};coin=dict(outpoint=dict(txid=label('unmined key2 funding transaction'),index=0),value_satoshis=100000,script=R.SCRIPT,block_height=1,block_hash=label('modeled funding block'),coinbase=True);tx=R.publication(coin,R.carrier(signed['statement']))
 r['publication']=dict(version=1,raw_tx=tx['raw_tx'],txid=tx['txid'],coin=coin,built_at_ms=now+1,dispatch_count=0,first_dispatch_at_ms=0,last_dispatch_at_ms=0,state='built');r['observation']['observed_at_ms']=now+1;records['built']=clone(r)
 r['publication'].update(dispatch_count=1,first_dispatch_at_ms=now+2,last_dispatch_at_ms=now+2,state='dispatch_unknown');r['observation']['observed_at_ms']=now+2;records['dispatch_unknown']=clone(r)
 r['publication']['state']='submitted';records['submitted']=clone(r)
 a=modeled_anchor(r['publication'],R.carrier(signed['statement']),200,'primary');tip=dict(height=259,hash=label('primary modeled tip259'));r['inclusion']=dict(version=1,generation=0,anchor=a,observed_at_ms=now+3,observed_tip=tip)
 # Finite modeled scan commitments; explicitly not retained public chain proof.
 for h in range(201):r['cursor']=R.scan_step(r['cursor'],h,a['hash']if h==200 else p['genesis']if h==0 else label('model primary '+str(h)))
 r['observation']=dict(observed_at_ms=now+3,chain_observed_at_ms=now+3,tip=tip,node_state='online',descendants=59,current_finality=False);records['included59']=clone(r)
 r['checkpoint']=R.checkpoint(p,a['height'],a['hash']);records['checkpoint59']=clone(r)
 tip=dict(height=260,hash=label('primary modeled tip260'));r['finality']=dict(checkpoint=clone(r['checkpoint']),inclusion=clone(r['inclusion']),observed_at_ms=now+4,observed_tip=tip,descendants=60);r['observation']=dict(observed_at_ms=now+4,chain_observed_at_ms=now+4,tip=tip,node_state='online',descendants=60,current_finality=True);records['finalized60']=clone(r)
 r['observation'].update(observed_at_ms=now+5,chain_observed_at_ms=now+5,tip=dict(height=261,hash=label('primary modeled tip261')),descendants=61);records['finalized61']=clone(r)
 r['observation'].update(observed_at_ms=now+6,node_state='offline',current_finality=False);records['offline_historical']=clone(r)
 r['observation']=dict(observed_at_ms=now+7,chain_observed_at_ms=now+7,tip=dict(height=259,hash=label('above inclusion shallow tip')),node_state='online',descendants=59,current_finality=False);records['shallow_historical']=clone(r)
 cf=dict(kind='checkpoint_chain_conflict',observed_at_ms=now+8,expected_height=200,expected_hash=a['hash'],observed_hash=label('replacement result block'),observed_tip=dict(height=260,hash=label('replacement tip')),checkpoint=None)
 r['conflicts']=[cf];r['observation'].update(observed_at_ms=now+8,chain_observed_at_ms=now+8,tip=cf['observed_tip'],current_finality=False);records['checkpoint_conflict']=clone(r)
 # Same exact raw transaction, pre-checkpoint reinclusion; no invented replacement.
 r=clone(records['included59']);r['prior_inclusions']=[dict(version=1,inclusion=clone(r['inclusion']),orphaned_at_ms=now+4,observed_tip=dict(height=201,hash=label('reorg tip')),replacement_hash=label('reorg at200'))]
 previous=clone(r['cursor']);r['scan_restarts']=[dict(generation=1,previous=previous,observed_at_ms=now+4,observed_tip=dict(height=201,hash=label('reorg tip')),replacement_hash=label('reorg at200'))];r['cursor']=R.scan_initial(p,key,1)
 for h in range(202):r['cursor']=R.scan_step(r['cursor'],h,label('reinclude model '+str(h))if h else p['genesis'])
 a2=modeled_anchor(r['publication'],R.carrier(signed['statement']),201,'reincluded');r['cursor']['hash']=a2['hash'];r['inclusion']=dict(version=1,generation=1,anchor=a2,observed_at_ms=now+5,observed_tip=dict(height=202,hash=label('reinclude tip202')));r['observation']=dict(observed_at_ms=now+5,chain_observed_at_ms=now+5,tip=r['inclusion']['observed_tip'],node_state='online',descendants=1,current_finality=False);records['reincluded']=clone(r)
 # Disputed claims are valid signed structural evidence, never local replacement.
 r=clone(records['prepared']);eq=clone(signed['statement']);eq['source']['record_digest']=label('disputed saved record');eq['source']['result_digest']=label('disputed result');eq['source']['summary']['winner']=src['source']['bout']['corner_b']if src['summary']['winner']!=src['source']['bout']['corner_b']else src['source']['bout']['corner_a'];eq['source']['summary']['method']='ko';r['equivocation']=R.sign_statement(eq);r['conflicts']=[dict(kind='statement_equivocation',observed_at_ms=now+1,expected_height=0,expected_hash=R.statement_id(signed['statement']),observed_hash=R.statement_id(eq),observed_tip=None,checkpoint=None)];r['observation']['observed_at_ms']=now+1;records['equivocation']=clone(r)
 statuses={k:S.status_from_record(p,v,ss,'available'if v['publication']else'unknown')for k,v in records.items()};statuses['absent']=S.status_from_record(p,None,ss)
 actions={k:dict(version=1,record=v,status=statuses[k])for k,v in records.items()}
 ps=dict(version=1,result_key=key,card_id=src['source']['card_id'],bout_id=src['source']['bout']['bout_id'],record_digest=R.record_digest(p,records['finalized60']),source_record_digest=src['record_digest'],input_id=src['input_id'],result_digest=src['result_digest'],txid=tx['txid'],sha256=label('modeled proof placeholder; no exported proof authority'),bytes=100000,header_count=261,block_count=3,finalized_at_ms=now+4,observed_tip=records['finalized60']['finality']['observed_tip'])
 overlay=clone(actions['finalized60']);bs=overlay['status']['source'];bs['source'].update(conflict_count=1,usable=False,current_finality=False);bs['saved']['usable']=False;overlay['status']=S.status_from_record(p,overlay['record'],bs,'available')
 return dict(source_conflict_read=overlay,pin=p,info=info,source=src,source_status=ss,views=clone(VIEWS),records=records,statuses=statuses,action_results=actions,proof_summary=ps,transaction=tx,coin=coin,statement=signed,rules=R.RULES,now=now,provenance='Source is the independently verified actual Set47 November28 saved C1 bout. All Set48 funding/publication/headers/inclusions/checkpoints are hypothetical arithmetic models, not mined or externally admitted evidence.')

if __name__=='__main__':
 x=examples()
 for k,r in x['records'].items():S.validate('Record',r,x['pin']);S.validate('ActionResult',x['action_results'][k],x['pin'])
 print('examples',len(x['records']),'all core semantic checks passed')

def alternate_checkpoint(cp):
 # Same fixed message/key, independently chosen public-fixture nonce.
 out=clone(cp);a,_=CRYPTO.ed_parts(R.CP_SEED);msg=R.checkpoint_bytes(cp);nonce=int.from_bytes(hashlib.sha512(b'Set48 independent alternate checkpoint nonce\0'+msg).digest(),'little')%CRYPTO.L
 point=CRYPTO.ed_enc(CRYPTO.ed_mul(CRYPTO.ED_BASE,nonce));k=int.from_bytes(hashlib.sha512(point+raw(R.CP)+msg).digest(),'little')%CRYPTO.L;out['signature']=(point+((nonce+k*a)%CRYPTO.L).to_bytes(32,'little')).hex();assert out['signature']!=cp['signature']and CRYPTO.ed_verify(raw(R.CP),msg,raw(out['signature']));return out

def alternate_transaction(coin,payload):
 pre,stripped,_,_=R.tx_recipe(coin,payload);dg=CRYPTO.D(pre);k=(CRYPTO.rfc6979(2,dg)+1)%CRYPTO.N;r=CRYPTO.mul(k)[0]%CRYPTO.N;s=(pow(k,CRYPTO.N-2,CRYPTO.N)*(int.from_bytes(dg,'big')+2*r))%CRYPTO.N;s=min(s,CRYPTO.N-s);der=CRYPTO.der_num(r)+CRYPTO.der_num(s);sig=b'\x30'+bytes([len(der)])+der;w=b'\x02'+CRYPTO.blob(sig+b'\x01')+CRYPTO.blob(raw(R.FUND));b=u32(2)+b'\x00\x01'+stripped[4:-4]+w+u32(0);assert R.validate_publication(coin,payload,b.hex());return b.hex()

def core_fragment():
 x=examples();p=x['pin'];positive=[];cases=[]
 def pos(name,kind,v):
  v=S.validate(kind,v,p);encoded=J(v);positive.append(dict(name=name,kind=kind,value=v,canonical=encoded.decode(),sha256=sha(encoded)));return v
 def case(name,kind,v,accept=False,raw_bytes=None):
  b=J(v)if raw_bytes is None else raw_bytes
  try:o=S.parse(kind,b,p);actual=True
  except(Refusal,AssertionError,ValueError,TypeError,KeyError):actual=False;o=None
  assert actual==accept,(name,actual,accept)
  c=dict(name=name,kind=kind,accept=accept)
  try:c['raw']=b.decode('utf8')
  except UnicodeError:c['raw_hex']=b.hex()
  if accept:c['canonical']=J(o).decode()
  cases.append(c)
 pos('pin','Pin',p);pos('rules-info','Info',x['info']);pos('saved-source','ResultSource',x['source']);pos('summary','ResultSummary',x['source']['summary']);pos('statement','Statement',x['statement']['statement']);pos('signed','SignedStatement',x['statement']);pos('funding-coin','FundingCoin',x['coin']);pos('outpoint','Outpoint',x['coin']['outpoint']);pos('proof-summary','ProofSummary',x['proof_summary'])
 for name,r in x['records'].items():pos('record-'+name,'Record',r);pos('status-'+name,'Status',x['statuses'][name]);pos('action-'+name,'ActionResult',x['action_results'][name])
 pos('absent','Status',x['statuses']['absent']);pos('read-finalized','ReadResult',x['action_results']['finalized60'])
 r=x['records']['finalized60']
 for kind,value in [('Publication',r['publication']),('Cursor',r['cursor']),('Anchor',r['inclusion']['anchor']),('Inclusion',r['inclusion']),('Finality',r['finality']),('SignedCheckpoint',r['checkpoint']),('Tip',r['finality']['observed_tip']),('Observation',r['observation']),('PriorInclusion',x['records']['reincluded']['prior_inclusions'][0]),('ScanRestart',x['records']['reincluded']['scan_restarts'][0]),('Conflict',x['records']['checkpoint_conflict']['conflicts'][0])]:pos(kind,kind,value)
 pos('marker','Marker',dict(application='slug-candidate-results',version=1,store_id=R.store_id(p),pin=p));pos('init','Init',dict(version=1,store_id=R.store_id(p),pin=p,source_created_at_ms=x['info']['source_created_at_ms']))
 pos('metadata-empty','Metadata',dict(version=1,store_id=R.store_id(p),pin=p,source_created_at_ms=x['info']['source_created_at_ms'],created_at_ms=x['info']['created_at_ms'],clock_ms=x['info']['created_at_ms'],result_count=0,funding_cursor=R.scan_initial(p,funding=True),funding_restarts=[],funding_complete=False,last_funding_tip=None))
 pos('reservation','Reservation',dict(version=1,result_key=r['result_key'],txid=r['publication']['txid']));pos('index','IndexEntry',dict(result_key=r['result_key'],saturday=x['source']['source']['event']['saturday'],bout_id=x['source']['source']['bout']['bout_id'],record_digest=R.record_digest(p,r)))
 alt=alternate_checkpoint(r['checkpoint']);pos('alternate-valid-checkpoint','SignedCheckpoint',alt)
 altpub=clone(r['publication']);altpub['raw_tx']=alternate_transaction(x['coin'],R.carrier(x['statement']['statement']));assert altpub['raw_tx']!=r['publication']['raw_tx'];pos('alternate-valid-transaction','Publication',altpub)
 # Closed parser controls apply to every exact canonical type, one base per kind.
 bykind={}
 for z in positive:bykind.setdefault(z['kind'],z)
 for kind,z in bykind.items():
  b=z['canonical'].encode();v=z['value'];case(kind+' canonical',kind,v,True);case(kind+' reordered',kind,v,True,raw_bytes=json.dumps(dict(reversed(list(v.items()))),indent=1).encode());case(kind+' trailing',kind,v,raw_bytes=b+b'{}');case(kind+' duplicate',kind,v,raw_bytes=b'{"'+next(iter(v)).encode()+b'":null,'+b[1:]);bad=clone(v);bad['unexpected']=0;case(kind+' unknown',kind,bad);bad=clone(v);bad.pop(next(iter(v)));case(kind+' missing',kind,bad)
  if 'version'in v:
   for name,n in [('bool',True),('wrongversion',2),('float',1.0)]:bad=clone(v);bad['version']=n;case(kind+' '+name,kind,bad)
   case(kind+' exponent',kind,v,raw_bytes=b.replace(b'"version":1',b'"version":1e0',1));case(kind+' negativezero',kind,v,raw_bytes=b.replace(b'"version":1',b'"version":-0',1))
  case(kind+' invalidUTF8',kind,v,raw_bytes=b'\xff'+b)
  cases.append(dict(name=kind+' overcap',kind=kind,accept=False,recipe=dict(base=z['name'],append_space=S.CAPS[kind]-len(b)+1)))
 # Cross-field semantic controls preserve all unrelated exact identities.
 def mutate(name,kind,base,path,value):
  v=clone(base);at=v
  for k in path[:-1]:at=at[k]
  at[path[-1]]=value;case(name,kind,v)
 mutate('source before saved store','ResultSource',x['source'],['created_at_ms'],x['source']['store_created_at_ms']-1)
 mutate('source false condition','ResultSource',x['source'],['source','corners',0,'member','condition_at_cutoff','rating_hundredths'],1)
 mutate('source wrong winner','ResultSource',x['source'],['summary','winner'],label('not corner'))
 mutate('wrong statement key purpose','SignedStatement',x['statement'],['signature'],CRYPTO.ed_sign(hashlib.sha256(domain('candidate seed declaration fixture')).digest(),R.signing_bytes(x['statement']['statement'])).hex())
 mutate('wrong checkpoint key purpose','SignedCheckpoint',r['checkpoint'],['signature'],CRYPTO.ed_sign(R.PUB_SEED,R.checkpoint_bytes(r['checkpoint'])).hex())
 mutate('dispatch overflow','Publication',r['publication'],['dispatch_count'],U32+1)
 mutate('finality59','Finality',r['finality'],['descendants'],59)
 mutate('wrong finality depth','Finality',r['finality'],['observed_tip','height'],261)
 mutate('checkpoint source height','Record',r,['checkpoint','height'],x['source']['source']['target_height'])
 mutate('finality certificate replaced','Record',r,['checkpoint'],alt)
 mutate('same txid changed retained witness','Record',r,['publication','raw_tx'],altpub['raw_tx'])
 mutate('signature corruption','SignedStatement',x['statement'],['signature'],'00'*64)
 mutate('key1 funding','FundingCoin',x['coin'],['script'],(b'\x00\x14'+CRYPTO.hash160(CRYPTO.pubkey(1))).hex())
 mutate('zero funding','FundingCoin',x['coin'],['value_satoshis'],0)
 mutate('bool funding','FundingCoin',x['coin'],['value_satoshis'],True)
 mutate('premature current finality','Record',x['records']['checkpoint59'],['observation','current_finality'],True)
 mutate('status current chain clock false','Status',x['statuses']['finalized60'],['chain_observed_at_ms'],0)
 mutate('absent publication state','Status',x['statuses']['absent'],['publication_state'],'')
 mutate('action detached summary','ActionResult',x['action_results']['finalized60'],['status','result_digest'],label('changed result'))
 # Alternate valid signature is admissible, but replay must retain whichever was first.
 a=clone(r);a['checkpoint']=clone(alt);a['finality']['checkpoint']=clone(alt);pos('record-alternate-first-certificate','Record',a)
 # Fixed conflict identity survives disputed claims; wrong identity never does.
 mutate('equivocation wrong fixed bout','Record',x['records']['equivocation'],['equivocation','statement','source','source','bout','bout_id'],label('other bout'))
 # Review controls: a shorter observed tip has no hash at the old height.
 short=clone(x['records']['reincluded']['scan_restarts'][0]);short['observed_tip']=dict(height=199,hash=label('shorter observed tip'));short['replacement_hash']='';pos('shorter-tip-restart','ScanRestart',short)
 bad=clone(short);bad['observed_tip']['height']=200;case('empty-replacement-at-existing-height','ScanRestart',bad)
 short=clone(x['records']['reincluded']['prior_inclusions'][0]);short['observed_tip']=dict(height=199,hash=label('shorter observed tip'));short['replacement_hash']='';pos('shorter-tip-prior-inclusion','PriorInclusion',short)
 bad=clone(short);bad['replacement_hash']=label('cannot exist beyond tip');case('nonempty-replacement-above-tip','PriorInclusion',bad)
 mutate('prior witness digest mismatch','Record',x['records']['reincluded'],['prior_inclusions',0,'inclusion','anchor','raw_tx_digest'],label('different witness same txid'))
 mutate('prior witness size mismatch','Record',x['records']['reincluded'],['prior_inclusions',0,'inclusion','anchor','raw_tx_size'],300)
 for field,value in [('source_digest',label('wrong compact source')),('winner',x['source']['source']['bout']['corner_a']if x['source']['summary']['winner']!=x['source']['source']['bout']['corner_a']else x['source']['source']['bout']['corner_b']),('created_at_ms',x['source']['created_at_ms']+1)]:
  mutate('read immutable saved '+field,'ReadResult',x['action_results']['finalized60'],['status','source','saved',field],value)
 mutate('read immutable card digest','ReadResult',x['action_results']['finalized60'],['status','source','source','card_record_digest'],label('different card bytes'))
 mutate('restart before preparation','Record',x['records']['reincluded'],['scan_restarts',0,'observed_at_ms'],x['now']-1)
 mutate('conflict before preparation','Record',x['records']['checkpoint_conflict'],['conflicts',0,'observed_at_ms'],x['now']-1)
 mutate('scan without publication','Record',x['records']['prepared'],['cursor'],x['records']['included59']['cursor'])
 cf=clone(x['records']['checkpoint_conflict']['conflicts'][0]);cf['observed_tip']=dict(height=199,hash=label('short conflict tip'));cf['observed_hash']='';pos('shorter-tip-chain-conflict','Conflict',cf)
 bad=clone(cf);bad['observed_tip']['height']=200;case('empty-conflict-hash-existing-height','Conflict',bad)
 dup=clone(x['records']['checkpoint_conflict']);c=clone(dup['conflicts'][0]);c['observed_at_ms']+=1;dup['conflicts'].append(c);dup['observation']['observed_at_ms']+=1;case('duplicate semantic conflict different timestamp','Record',dup)
 for half in [0,1]:
  bad=clone(x['records']['included59']['inclusion']['anchor']);payload=bytearray(raw(bad['payload']));payload[9+32*half:41+32*half]=bytes(32);bad['payload']=payload.hex();case('zero carrier half '+str(half),'Anchor',bad)
 for method in S.METHODS:
  z=dict(winner=''if method in ['decision_draw','double_corner_stoppage']else x['source']['source']['bout']['corner_a'],method=method,end_round=3 if method.startswith('decision')else 1,end_tick=60);pos('structural-summary-'+method,'ResultSummary',z)
 mutate('prior inclusion before preparation','Record',x['records']['reincluded'],['prior_inclusions',0,'inclusion','observed_at_ms'],x['now']-1)
 # Fresh source conflict does not fabricate a new result-chain observation.
 overlay=clone(x['action_results']['finalized60']);bs=overlay['status']['source'];bs['source'].update(conflict_count=1,usable=False,current_finality=False);bs['saved']['usable']=False;overlay['status']=S.status_from_record(p,overlay['record'],bs,'available');assert overlay['status']['phase']=='conflict'and overlay['status']['current_finality'];pos('fresh-source-conflict-retained-result-chain-observation','ReadResult',overlay)
 # A first mismatching signed checkpoint is evidence, not an accepted lock.
 mismatch=clone(x['records']['included59']);other=R.checkpoint(p,200,label('signed competing result inclusion'));mismatch['conflicts']=[dict(kind='checkpoint_equivocation',observed_at_ms=x['now']+4,expected_height=200,expected_hash=mismatch['inclusion']['anchor']['hash'],observed_hash=other['hash'],observed_tip=None,checkpoint=other)];mismatch['observation']['observed_at_ms']=x['now']+4;pos('checkpoint-equivocation-before-lock','Record',mismatch)
 bad=clone(x['statuses']['finalized60']);bad['phase']='conflict';bad['conflict_count']=1;case('standalone own conflict cannot retain current finality','Status',bad)
 mutate('current observation tip below claimed60','Record',x['records']['finalized60'],['observation','tip','height'],259)
 mutate('standalone current tip below claimed60','Status',x['statuses']['finalized60'],['scan_tip_height'],259)
 facts=dict(rules_id=R.RULE_ID,store_id=R.store_id(p),result_key=r['result_key'],result_source_digest=R.source_digest(x['source']),statement_id=R.statement_id(x['statement']['statement']),statement_signing_bytes=R.signing_bytes(x['statement']['statement']).hex(),publisher_public_key=R.PUB,checkpoint_public_key=R.CP,funding_public_key=R.FUND,funding_script=R.SCRIPT,carrier=R.carrier(x['statement']['statement']).hex(),record_digests={k:R.record_digest(p,v)for k,v in x['records'].items()},transaction=x['transaction'],checkpoint_signing_bytes=R.checkpoint_bytes(r['checkpoint']).hex(),alternate_checkpoint=alt,alternate_raw_tx=altpub['raw_tx'])
 return dict(version=1,kind='core',provenance=x['provenance'],pin=p,positive=positive,cases=cases,facts=facts,case_count=len(cases))
