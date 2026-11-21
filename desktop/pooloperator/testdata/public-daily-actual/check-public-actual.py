"""Independent checks of named PUBLIC Set79 Path B evidence only."""
import hashlib, json, re

def need(ok,why):
    if not ok:raise RuntimeError(why)

def balance_wait_evidence(root,support,complete=False):
    path=root/'balance-wait-evidence.json';identity=support.identity(path)
    need(0<identity['bytes']<=4<<20,'public balance wait evidence bound')
    raw=path.read_bytes();need(len(raw)==identity['bytes'] and support.identity(path)==identity,'public balance wait evidence changed')
    value=json.loads(raw)
    need(set(value)=={'version','waits'} and value['version']==1 and isinstance(value['waits'],list) and 1<=len(value['waits'])<=4,'balance wait evidence shape')
    selected=root/'selected-public-setup.json';selected_id=support.identity(selected)
    need(0<selected_id['bytes']<=1<<20,'public selected setup bound')
    setup=json.loads(selected.read_bytes());need(support.identity(selected)==selected_id,'selected setup changed during diagnostic read')
    expected=[('a_lost_reply',45000,False),('a_recovered_before_b_credit',90000,True),('b59_same_day',45000,True),('b60_same_day',150000,True)]
    codes={'invalid_request','unauthorized','forbidden','not_found','expired','rejected','too_large','rate_limited','busy','capacity','unavailable','outcome_unknown','non_client_error','unrecognized'}
    fields={'name','budget_ms','started_at_ms','ended_at_ms','elapsed_ms','context_deadline_ms','after_ms','check_reasons','total_samples','last','terminal'}
    if complete:need(len(value['waits'])==4,'all original balance waits required')
    for index,w in enumerate(value['waits']):
        name,budget,reasons=expected[index]
        need(set(w)==fields and w['name']==name and w['budget_ms']==budget and w['check_reasons'] is reasons,'original balance wait selection')
        need(all(type(w[k]) is int and w[k]>=0 for k in ('started_at_ms','ended_at_ms','elapsed_ms','context_deadline_ms','after_ms','total_samples')) and w['started_at_ms']>0 and w['ended_at_ms']>0,'public balance wait times/count')
        need(w['terminal'] in ('success','failure') and (not complete or w['terminal']=='success') and (index==len(value['waits'])-1 or w['terminal']=='success'),'balance wait terminal state')
        tail=w['last'];total=w['total_samples']
        need(isinstance(tail,list) and len(tail)==min(32,total),'bounded last32 balance outcomes')
        for n,row in enumerate(tail,start=total-len(tail)+1):
            common={'sample','gym_index','observed_at_ms'}
            need(set(row) in (common|{'failure'},common|{'balance'}) and row['sample']==n and type(row['gym_index']) is int and row['gym_index'] in (0,1) and type(row['observed_at_ms']) is int and row['observed_at_ms']>0,'public sample framing/correlation')
            if 'failure' in row:
                f=row['failure'];need(set(f)=={'code','outcome_unknown'} and f['code'] in codes and type(f['outcome_unknown']) is bool,'closed public refusal only')
            else:
                b=row['balance'];i=row['gym_index'];enrollment=setup['miners']['enrollments'][i]
                need(set(b)=={'version','binding_id','settlement_config_id','recipient_script','balance','observed_at_ms'} and b['version']==2 and b['binding_id']==setup['wallet_guard']['source_binding_id'] and b['settlement_config_id']==setup['wallet_guard']['settlement_config_id'] and b['recipient_script']==enrollment['payee_script'] and b['balance']['gym_id']==enrollment['gym_id'],'original authenticated diagnostic balance binding')
        if w['terminal']=='success':need(len(tail)>=2 and [x['gym_index'] for x in tail[-2:]]==[0,1] and all('balance' in x for x in tail[-2:]),'successful wait needs both final authenticated reads')
    return dict(evidence={str(path):identity,str(selected):selected_id},waits=value['waits'])

def check(root,reference,economics,support):
    evidence={}
    def read(name,limit=16<<20):
        p=root/name;identity=support.identity(p)
        need(0<identity['bytes']<=limit,'public evidence bound '+name)
        b=p.read_bytes();need(support.identity(p)==identity and len(b)==identity['bytes'],'public evidence changed '+name)
        evidence[str(p)]=identity;return json.loads(b)
    e=json.loads(economics.read_bytes())['actual_economics']
    original=next(x['setup'] for x in json.loads(reference.read_bytes())['composition']['positives'] if x['name']=='two-enrollments')
    s=read('selected-public-setup.json');selected=json.loads(json.dumps(s))
    selected['ingress']=original['ingress'];selected['schedule']['daily_utc_minute']=original['schedule']['daily_utc_minute']
    need(selected==original,'actual monetary/source configuration changed')
    enroll=s['miners']['enrollments'];gyms=[v['gym_id'] for v in enroll]
    need(len(gyms)==2 and gyms==sorted(gyms),'selected two original gyms')
    result=read('actual-report.json',2<<20)
    facts=dict(version=2,pass_=True,completed=True,clean_join=True,public_seed_mining=False,
               final_height=163,normal_blocks_max=180,ordinary_intake=2,late_rows=3,
               late_block_rows=2,late_credit_count=1,pending_batch_unchanged=True,
               exact_restart_receipt=True,late_births=0,real_expiry=True,signed_batches=1,
               fresh_production_cli_lifetimes=2,daily_tls_money_binding=True,
               original_a_recovery_without_resend=True)
    facts['pass']=facts.pop('pass_')
    need(all(result.get(k)==v for k,v in facts.items()) and result.get('cleanup_errors')==[],'actual scenario or cleanup incomplete')
    nodes=result['node_cleanup'];children=result['child_cleanup']
    need(len(nodes)==2 and all(n['clean'] and n['joined'] and not n['forced_kill'] and not n['unexpected_exit'] and n['wait_error']=='' and not n['log_close_error'] and not n['stop_rpc_error'] for n in nodes),'original nodes not joined')
    need(len(children)==2 and {c['name'] for c in children}=={'slugpoold-daily-b59','slugpoold-daily-b60'} and all(c['clean'] and c['joined'] and not c['forced'] and c['wait_error']=='' and c['log_close_error']=='' and c['pid']>1 for c in children),'two production CLI owners not joined')
    listening=read('01-runtime-listening.json')
    a=read('02-A-block.json');first=read('03-A-pending.json');full=read('04-full-intake.json')
    absent=read('05-expired-absent.json');b=read('06-B-pending.json');p59=read('07-B59-restart.json');p60=read('08-B60-final.json')
    waits=balance_wait_evidence(root,support,complete=True);evidence.update(waits['evidence'])
    final_reads=(first['balances'],p59['recovered_balances'],p59['balances'],p60['balances'])
    for w,balances in zip(waits['waits'],final_reads):
        need([sample['balance'] for sample in w['last'][-2:]]==balances,'diagnostic final reads differ from original accepted phase evidence')
    auditA=first['audit'];body=auditA['body'];day=body['day']
    need(body['payments']==[dict(gym_id=gyms[0],script=enroll[0]['payee_script'],atoms=e['a_payment_atoms'])] and len(body['inputs'])==e['a_inputs'] and body['fee_atoms']==e['a_fee_atoms'] and body['change']['atoms']==e['a_change_atoms'] and body['change']['index']==1 and body['change']['script']==s['wallet_guard']['funding_script'],'independent A transaction amounts/provenance')
    need(body['version']==2 and body['sequence']==1 and auditA['delivery']=='prepared' and auditA['header']['pending_batch']==1,'original A reserved')
    canonical_body=json.dumps(body,separators=(',',':')).encode('ascii')
    need(len(canonical_body)==auditA['body_bytes'] and hashlib.sha256(canonical_body).hexdigest()==auditA['body_sha256'],'exact persisted A body digest')
    need(re.fullmatch('[0-9a-f]{64}',body['txid']) and re.fullmatch('[0-9a-f]+',body['raw']) and len(body['raw'])%2==0,'public transaction framing')
    raw=bytes.fromhex(body['raw']);need(len(raw)<=100000,'actual signed transaction size')
    # Body is a decoded fixed-order typed struct. Preserve every field and the
    # independently captured exact persisted body length/hash at every reopen.
    for x in (auditA,full['audit'],absent['audit'],b['audit'],p59['audit'],p60['audit']):
        need(x['read_only_unchanged'] is True and x['body']==body and x['body_sha256']==auditA['body_sha256'] and x['body_bytes']==auditA['body_bytes'] and x['credits'][0]==auditA['credits'][0],'A body/claim changed or audit wrote')
        h=x['header'];need(h['version']==2 and h['config_id']==s['wallet_guard']['settlement_config_id'] and h['source_id']==s['wallet_guard']['source_binding_id'] and h['batches']==1 and h['events']==h['credits']+1 and h['last_day']==day,'original indexed ledger identity')
    need(a['height']==2 and a['receipt']['sequence']==a['receipt']['accepted_sequence']==1 and a['receipt']['block'] is True,'original A canonical receipt')
    need(first['due_observed_ms']>=first['due_ms'] and body['created_at_ms']>=first['due_ms'],'payout before real daily due')
    def census(x,rows,credits,pending):
        need(x['submissions']==x['accepted']==rows and x['works']==x['authority_jobs']==3 and x['births']==0 and len(x['entries'])==rows and len(x['credits'])==x['header']['credits']==credits and x['header']['pending_batch']==pending,'original physical/credit census')
    census(full['audit'],2,1,1);census(absent['audit'],2,1,1);census(b['audit'],3,1,1);census(p59['audit'],3,1,0);census(p60['audit'],3,2,0)
    need(p59['audit']['delivery']==p60['audit']['delivery']=='confirmed','original A delivery after restart')
    need(full['audit']['entries']==absent['audit']['entries'] and full['issue_denial']==dict(code='outcome_unknown',outcome_unknown=True),'full ordinary intake or terminal Issue proof')
    job=full['work_b']['job'];receipt=b['receipt']
    need(job==b['work']['job']==absent['job'] and job['expires_at_ms']-job['issued_at_ms']==300000,'original five-minute B job')
    need(all(x['job']['id']!=job['id'] for x in full['audit']['entries']),'B was already retained before expiry')
    need(absent['expiry_observed_at_ms']>=job['expires_at_ms'] and absent['winner_observed_at_ms']>=job['expires_at_ms'] and receipt['accepted_at_ms']>=job['expires_at_ms'],'actual wall expiry not proved')
    need(absent['ordinary_refusal']==dict(code='capacity',outcome_unknown=False) and absent['noncanonical_refusal']==dict(code='outcome_unknown',outcome_unknown=True),'ordinary/noncanonical late refusal')
    need(receipt['sequence']==receipt['accepted_sequence']==3 and receipt['acceptance']=='canonical-late-winner' and receipt['block_state']=='canonical-observed' and receipt['block_height']==103,'canonical late winning credit')
    need(receipt==p59['receipt']==p59['duplicate']==p59['lookup']==p60['receipt'],'restart changed exact receipt')
    projection=dict(receipt);projection['block_state']='';projection['block_observed_at_ms']=0
    for x in (b['audit'],p59['audit'],p60['audit']):
        need(x['entries'][2]['share']==projection and x['entries'][2]['job']['id']==job['id'],'original public accounting projection')
        round_=x['rounds'][1]['round']
        need(round_['first_accepted']==2 and round_['last_accepted']==3 and len(round_['share_keys'])==2 and round_['share_keys'][1]==receipt['duplicate_key'] and round_['block_hash']==receipt['work_hash'] and round_['block_height']==103 and round_['reward_atoms']==e['b_reward_atoms'],'B fixed original round window')
        allocations=round_['allocations'];need(len(allocations)==2,'B two-gym allocation')
        for i,v in enumerate(allocations):need(v['gym_id']==gyms[i] and v['shares']==1 and {k:v[k] for k in ('numerator','denominator')}==e['round_b_gross_by_gym'][i],'independent B equal difficulty allocation')
    r59=p59['audit']['rounds'][1];r60=p60['audit']['rounds'][1]
    need(not r59.get('confirmation') and r59['observation']['canonical'] and r59['observation']['descendants']==59 and p59['height']==162,'credit/lock before60')
    need(r60['confirmation']['descendants']==60 and p60['height']==163 and p60['late_credit_count']==1 and p60['original_batch_unchanged'] is True,'exact60 confirmation and one credit')
    late=[c for c in p60['audit']['credits'] if c['round_hash']==receipt['work_hash']]
    need(len(late)==1,'B credited more than once');coin=late[0]['coin']
    need(coin['coinbase'] and coin['block_height']==103 and coin['block_hash']==receipt['work_hash'] and coin['atoms']==e['b_reward_atoms'] and coin['script']==s['wallet_guard']['funding_script'],'original B coinbase')
    claims={c['gym_id']:c['amount'] for c in coin['claims']}
    need(len(claims)==len(coin['claims'])==3 and claims=={'':e['operator_b_earned'],gyms[0]:e['round_b_net_by_gym'][0],gyms[1]:e['round_b_net_by_gym'][1]} and all(c['round_hash']==receipt['work_hash'] and c['round_height']==103 for c in coin['claims']),'independent B99/1 exact claims')
    need(len(p60['funding'])==1 and p60['funding'][0]['state']=='immature' and p60['funding'][0]['txid']==coin['txid'] and p60['funding'][0]['index']==coin['index'],'no advance from immature B')
    def balance(value,want,i,wire):
        need(value['gym_id']==gyms[i] and value['confirmed']==want['confirmed'] and value['available']==want['available'] and value['reason']==want['reason'],'authenticated balance fractions/reason')
        for k in ('reserved_atoms','paid_atoms'):need(value[k]==(str(want[k]) if wire else want[k]),'exact cumulative amount type/value')
    for phase,label in ((p59,'b59_same_day'),(p60,'b60_same_day')):
        need(len(phase['balances'])==len(phase['pages'])==2,'both authenticated gyms must be observed')
        wants=e['reports'][label]['balances']
        for i,response in enumerate(phase['balances']):
            need(response['binding_id']==s['wallet_guard']['source_binding_id'] and response['settlement_config_id']==s['wallet_guard']['settlement_config_id'] and response['recipient_script']==enroll[i]['payee_script'] and response['observed_at_ms']>0,'authenticated money/source binding')
            balance(response['balance'],wants[i],i,True)
        for i,page in enumerate(phase['pages']):
            need(page['version']==2 and page['binding_id']==s['wallet_guard']['source_binding_id'] and page['settlement_config_id']==s['wallet_guard']['settlement_config_id'] and page['recipient_script']==enroll[i]['payee_script'] and page['high_batch']=='1' and page['before_batch']=='2' and page['next_before']=='0' and page['done'] is True and int(page['revision'])>0,'original daily captured history')
            balance(page['balance'],wants[i],i,True)
            need(len(page['items'])==(1 if i==0 else 0),'gym-local own history')
            if i==0:
                item=page['items'][0];need(item['batch_sequence']=='1' and item['payment']==dict(day=day,created_at_ms=body['created_at_ms'],txid=body['txid'],output_index=0,atoms=e['a_payment_atoms'],recorded_state='confirmed'),'exact original A payment history')
    sends=result['broadcasts'];attempts=result['broadcast_attempts']
    need(not result['broadcast_attempt_overflow'] and len(sends)==len(attempts)==1 and sends[0]['raw']==attempts[0]['raw']==body['raw'] and sends[0]['txid']==body['txid'],'second send or changed payout bytes')
    fault=result['observation_fault'];need(fault['method']=='getrawtransaction' and fault['verbose']==1 and fault['txid']==body['txid'] and fault['refusals']>0 and fault['released'] is True and b['observation_fault_released_after_parent_join'] is True,'explicit observation-only fault boundary')
    return evidence
