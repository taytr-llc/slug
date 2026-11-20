#!/usr/bin/env python3
"""Locked independent wallet codec producer. No model imports before approval."""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import sys
sys.dont_write_bytecode=True
HERE=Path(__file__).resolve().parent
SOURCES={'codec_model.py','sizing_model.py','generate_reference.py','case-plan.json','README.md','reproduce.py'}
H=lambda b:hashlib.sha256(b).hexdigest()

def need(x,message):
    if not x:raise RuntimeError(message)
def load(name,path):
    spec=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(spec);sys.modules[name]=m;spec.loader.exec_module(m);return m
def safe(root,name):
    rel=Path(name);need(not rel.is_absolute() and rel.parts and all(x not in ('.','..') for x in rel.parts),'relative path')
    p=root
    for part in rel.parts:p=p/part;need(not p.is_symlink(),'symlink input')
    need(p.is_file() and root in p.resolve().parents,'input containment');return p
def guard(args):
    need(args.execute and args.approved_producer_lock and args.approved_lock_sha256 and args.output,'explicit execution/lock/output')
    raw=args.approved_producer_lock.read_bytes();need(H(raw)==args.approved_lock_sha256,'lock hash');lock=json.loads(raw)
    need(lock.get('approved_by')=='root' and lock.get('execution_authorized') is True,'root approval')
    need(set(lock['sources'])==SOURCES,'exact source set')
    before={n:H(safe(HERE,n).read_bytes()) for n in SOURCES};need(before==lock['sources'],'source lock')
    need(H(args.input_catalog.read_bytes())==lock['input_catalog_sha256'],'catalog lock')
    root=args.input_root.resolve(strict=True);catalog=json.loads(args.input_catalog.read_bytes());paths={};total=0
    need(len(catalog['files'])<=64,'input count')
    for row in catalog['files']:
        name=row['path'];need(name not in paths,'duplicate input');p=safe(root,name);raw=p.read_bytes()
        need(len(raw)==row['bytes'] and H(raw)==row['sha256'],'input bytes '+name);paths[name]=p;total+=len(raw)
    need(total<=16*1024*1024,'input byte budget')
    need({str(p.relative_to(root)) for p in root.rglob('*') if p.is_file()}==set(paths),'exact input set')
    need(H(paths['contracts/FIGHTER_CARRIER_WALLET.md'].read_bytes())=='eec5306bf2b088ef2ce5ed5c42df2907b94f6afd137df03c8c89d541eb20a789','wallet contract')
    need(H(paths['contracts/FIGHTER_TRANSFER_CARRIER.md'].read_bytes())=='70e1ff02bd335a7436014fbe34ee76aca4b9046cb1167c4f2ea0d9fbe80e0dc7','carrier contract')
    need(H(paths['carrier/carrier-v1.json'].read_bytes())=='365e2997171b5f06706d628866f2cbc64cf0d7a3ac4205015def259a5340f4af','independent carrier corpus')
    return paths,before,lock

def generate(paths,plan):
    # These imports/derivations are intentionally AFTER complete guard.
    M=load('set73_wallet_codec_model',HERE/'codec_model.py')
    common=load('common',paths['inherited/common.py'])
    public=load('public_derivation',paths['inherited/public_derivation.py'])
    old=load('set73_old_allocation',paths['inherited/allocation_v1.py'])
    math=public.load_oracle(paths['inherited/recovery/reference.py'])
    C=load('set73_independent_carrier_model',paths['carrier/carrier_model.py'])
    R=load('set73_old_carrier_recovery',paths['carrier/inputs/inherited/recovery/reference.py'])
    E=load('set73_old_carrier_ecdsa',paths['carrier/inputs/inherited/ecdsa/reference_authorizations.py'])
    read=lambda name:json.loads(paths[name].read_bytes())
    prior=read('public/allocation-v1.json');addresses=read('public/addresses-v1.json');recipe=read('recipe.json')
    carrier=read('carrier/carrier-v1.json');cr=read('carrier/inputs/recipe.json');published=read('carrier/inputs/published-recovery-v1.json')
    cm=C.Model(R,E,published,cr);small_recipe=copy.deepcopy(cr);small_recipe['parent_transaction']['atoms']=recipe['wallet_selection_case']['each_input_atoms']
    sm=C.Model(R,E,published,small_recipe)
    for row in carrier['positives']:
        got=cm.validate(row);need(got==row['validated'],'inherited carrier recomputation')
    canonical=M.canonical;digest=M.digest;own=M.own
    positives=[];negatives=[];raw_cases=[];contexts=[];bindings=[];allocation_cases=[];state_cases=[];selection_cases=[]
    next_id=recipe['request_sequence_start']
    def ident():
        nonlocal next_id
        next_id+=1;return '%064x'%next_id
    def add(name,kind,value,normalizer,context=None,domain=None):
        normalized=normalizer(own(value));need(canonical(normalized)==canonical(value),'positive canonical order '+name)
        raw=canonical(normalized)
        row=dict(name=name,kind=kind,scope='independent codec/model data; no concrete authority, live chain, wallet restoration or storage durability claim',value=normalized,canonical_utf8=raw.decode(),sha256=H(raw),bytes=len(raw))
        if context is not None:row['context']=context
        if domain is not None:row['digest']=domain(normalized)
        positives.append(row);return normalized
    def bad(name,kind,value,normalizer,context=None):
        try:normalizer(own(value))
        except (M.Refused,common.Rejected,C.Rejected) as e:
            row=dict(name=name,kind=kind,valid=False,expected='refuse',diagnostic=str(e),value=value)
            if context is not None:row['context']=context
            negatives.append(row)
        else:raise RuntimeError('negative accepted '+name)
    def changed(v,path,value):
        out=own(v);at=out
        for k in path[:-1]:at=at[k]
        at[path[-1]]=value;return out
    def guard14(b,bound):
        return dict(version=2,state='bound' if bound else 'locked',inventory_id=b['inventory_id'],authority_id=b['authority_id'],account_id=b['account_id'],wallet_owner_id=b['wallet_owner_id'],source_binding_id=b['source_binding_id'],ownership_pin_id=b['ownership_pin_id'],carrier_journal_id=b['carrier_journal_id'],withdrawal_journal_id=b['withdrawal_journal_id'],initial_manifest_sha256=H(canonical(b['initial_manifest'])),binding_id=M.allocation('binding',b) if bound else '')
    def areq(kind,op,binding='',crypto=None,withdrawal=None):
        r={k:0 for k in M.REQUEST.split()}
        for k in ('kind','consumer_id','withdrawal_binding_id','recipient','recipient_script','carrier_binding_id','ownership_pin_id','prior_state_id','recipient_gym','funding_digest'):r[k]=''
        r.update(version=2,kind=kind,consumer_id=op,purpose=84,branch=0)
        if kind=='carrier_return':
            r.update(branch=1,min_confirmations=1,input_purpose=84,change_purpose=84,tx_version=1,input_sequence=M.U32,sighash_type=1,carrier_binding_id=binding,ownership_pin_id=crypto['expected_pin'],prior_state_id=crypto['expected_prior'],recipient_gym=crypto['recipient_gym'],funding_digest=crypto['intent'][-64:],escrow_atoms=500000,commit_fee_atoms=10000,spend_fee_atoms=10000,min_change_atoms=1000)
        if withdrawal:
            d=withdrawal;r.update(branch=1,withdrawal_binding_id=d['binding_id'],recipient=d['recipient'],recipient_script=d['recipient_script'],amount_atoms=d['amount_atoms'],fee_rate_atoms_per_kb=1000,fee_ceiling_atoms=100000,min_confirmations=1,input_purpose=84,change_purpose=84,tx_version=1,input_sequence=M.U32,sighash_type=1)
        return r
    def reserve(model,b,req,seq,start=0,skip=0):
        selected=start+skip;addr,script=model.derive(req['branch'],selected)
        return dict(version=2,sequence=seq,allocation_id=req['consumer_id'],binding_id=M.allocation('binding',b),request=req,purpose=84,branch=req['branch'],start_index=start,end_index=selected+1,selected_index=selected,invalid_indices=list(range(start,selected)),address=addr,script=script,reserved_at_ms=recipe['base_time_ms']+seq)
    def link(b,res,outcome,prep=''):
        before=[0,0,0,0,2,0,0,0];before[4+res['branch']]=res['start_index'];after=own(before);after[4+res['branch']]=res['end_index'];req=res['request']
        result={k:res[k] for k in ('version','inventory_id') if k in res}
        result=dict(version=2,inventory_id=b['inventory_id'],binding_id=res['binding_id'],sequence=res['sequence'],allocation_id=res['allocation_id'],reservation_digest=M.allocation('reservation',res),request_digest=M.allocation('request',req),outcome=outcome,purpose=84,branch=res['branch'],start_index=res['start_index'],end_index=res['end_index'],selected_index=res['selected_index'],invalid_indices=own(res['invalid_indices']),address=res['address'],script=res['script'],cursors_before=before,cursors_after=after,consumer_kind=req['kind'],consumer_binding_id=req['withdrawal_binding_id'] if req['kind']=='withdrawal_change' else req['carrier_binding_id'] if req['kind']=='carrier_return' else '',consumer_id=req['consumer_id'],preparation_digest=prep)
        return result
    def auth(mode,offset=0):
        n=recipe['action_sequence_start']+offset
        return dict(mode=mode,action_id='%064x'%n,review_id='%064x'%(n+1),view_id='%064x'%(n+2),generation=3 if mode=='hosted' else 0,authorized_at_ms=recipe['base_time_ms']+100)
    def dispatch(mode,result='pending'):
        return dict(attempt=1,authorization=auth(mode,10),intent_at_ms=recipe['base_time_ms']+110,result=result,result_at_ms=0 if result=='pending' else recipe['base_time_ms']+111,error='')
    def observe(package,leg,state='mempool'):
        raw=package[leg];return dict(observed_at_ms=recipe['base_time_ms']+120,state=state,expected_txid=raw['txid'],observed_txid=raw['txid'],node_tip=recipe['tip_hash'],node_height=recipe['tip_height'],block_hash='',block_height=0,transaction_index=0,confirmations=0,transfer_id=package['transfer_id'],raw_sha256=raw['raw_sha256'],error='')

    for fixture_index,fixture in enumerate(recipe['fixture_order']):
        b0=prior['bindings'][fixture_index]['binding'];initial=b0['initial_manifest'];case=next(x for x in addresses['cases'] if x['name']==fixture);derive=public.PublicDerivation(math,case);derive.verify_overlap(case)
        model=M.Model(initial,public.address_for_script,derive.child,old.request,cm.validate,C.parse_tx)
        b={k:(b0[k] if k in b0 else None) for k in M.BINDING.split()};b.update(recipe['fixture_ids'][fixture]);b.update(version=2,manager_schema=14,ownership_pin_id=recipe['ownership_pin_id'])
        b=add(fixture+'/binding2','binding2',b,model.binding,domain=lambda x:M.allocation('binding',x));bindings.append(b)
        for bound in (False,True):add(fixture+('/guard-bound' if bound else '/guard-locked'),'guard14',guard14(b,bound),lambda x:model.guard(x,b),context=dict(binding=b),domain=lambda x:M.allocation('wallet guard',x))
        bad(fixture+'/guard-adopt-binding','guard14',changed(guard14(b,False),['binding_id'],M.allocation('binding',b)),lambda x:model.guard(x,b),dict(binding=b))
        bad(fixture+'/binding-bool-cursor','binding2',changed(b,['initial_manifest','accounts',2,'next_internal'],False),model.binding)
        bad(fixture+'/binding-old-schema13','binding2',changed(b,['manager_schema'],13),model.binding)
        marker=dict(application='slug-hosted-transfer-allocation-inventory',version=2,inventory_id=b['inventory_id'],binding=b)
        add(fixture+'/allocation-marker','allocation_marker',marker,lambda x:model.allocation_marker(x,b),dict(binding=b))
        # Independent raw-range fault cases are explicitly modeled; no natural CKD claim.
        baseline_crypto=next(r for r in carrier['positives'] if r['name']==fixture+'-1-standard-base')
        for skip in (0,2,127):
            op=ident();req=areq('carrier_return',op,'%064x'%73,baseline_crypto);res=reserve(model,b,req,3,0,skip);faults=[[1,i] for i in range(skip)]
            context=dict(binding=b,reservation=res,injected_invalid_children=faults)
            add(fixture+'/range-'+str(skip),'reservation2',res,lambda x:model.reservation(x,b,faults),context,lambda x:M.allocation('reservation',x))
            row=dict(reservation=res,reservation_digest=M.allocation('reservation',res),phase='reserved',completed_at_ms=0,wallet_record_digest='',preparation_digest='')
            add(fixture+'/range-'+str(skip)+'-reserved-row','allocation_row',row,lambda x:model.allocation_row(x,res),context)
            for outcome in ('consumed','burned'):
                l=link(b,res,outcome,'%064x'%74 if outcome=='consumed' else '')
                add(fixture+'/range-'+str(skip)+'-'+outcome,'link2',l,lambda x:model.link(x,res,b,faults),context,lambda x:M.allocation('wallet record',x))
                terminal=dict(reservation=res,reservation_digest=M.allocation('reservation',res),phase=outcome,completed_at_ms=res['reserved_at_ms']+1,wallet_record_digest=M.allocation('wallet record',l),preparation_digest=l['preparation_digest'])
                add(fixture+'/range-'+str(skip)+'-'+outcome+'-row','allocation_row',terminal,lambda x:model.allocation_row(x,res,l),dict(context,link=l))
            if skip:
                bad(fixture+'/forged-valid-child-'+str(skip),'reservation2',res,lambda x:model.reservation(x,b),dict(binding=b,injected_invalid_children=[]))
        # Old receive and withdrawal request shapes remain closed v2 variants.
        for kind in ('pool_recipient','receive'):
            req=areq(kind,ident());add(fixture+'/'+kind+'-request2','request2',req,model.request,domain=lambda x:M.allocation('request',x))
        d=next(x['draft'] for x in read('public/withdrawals-v1.json')['cases'] if x['name']==fixture+'-original-to-84')
        req=areq('withdrawal_change',d['operation_id'],withdrawal=d);add(fixture+'/withdrawal-request2','request2',req,model.request,domain=lambda x:M.allocation('request',x))
        req=areq('carrier_return',ident(),'%064x'%73,baseline_crypto)
        for label,key,value in [('wrong-capability-kind','kind','withdrawal_change'),('fee','commit_fee_atoms',9999),('unused-field','amount_atoms',1),('unknown-kind','kind','carrier'),('bool-fee','spend_fee_atoms',True)]:bad(fixture+'/'+label,'request2',changed(req,[key],value),model.request)

        casedefs=[]
        for mode in ('hosted','restored'):
            casedefs.append((mode,'transfer',baseline_crypto,'one'))
            small=sm.make(fixture,count=16,suffix='wallet-smallest-prefix');sm.validate(small)
            casedefs.append((mode,'transfer',small,'sixteen-small'))
        casedefs.append(('restored','recover_reveal',baseline_crypto,'recovery'))
        if fixture=='zero256':casedefs.append(('restored','recover_refund',next(r for r in carrier['positives'] if r['name']=='zero256-1-refund-base'),'recovery'))
        for mode,kind,crypto,suffix in casedefs:
            prefix=fixture+'/'+mode+'/'+kind+'/'+suffix;reqid=ident();original=own(initial);original['accounts'][2]['next_external']=2;original['accounts'][2]['next_internal']=0 if kind=='transfer' else 1
            pin=dict(version=1,owner_mode=mode,manager_schema=14 if mode=='hosted' else 15,network=initial['network'],genesis=initial['genesis'],wallet_profile=initial['profile'],root_public_key=initial['root_public_key'],gym_public_key=initial['gym_public_key'],gym_id=initial['gym_id'],wallet_owner_id=b['wallet_owner_id'],ownership_pin_id=b['ownership_pin_id'],account_authority_id=b['authority_id'] if mode=='hosted' else '',account_id=b['account_id'] if mode=='hosted' else '',allocation_binding_id=M.allocation('binding',b) if mode=='hosted' else '',original_manifest_sha256=H(canonical(original)) if mode=='restored' else '')
            journal_id=b['carrier_journal_id'];binding=digest('fighter carrier binding',dict(journal_id=journal_id,pin=pin));context=dict(fixture=fixture,pin=pin,journal_id=journal_id,binding_id=binding,original_manifest=original,carrier=crypto)
            contexts.append(dict(name=prefix,**context));model.original=original
            add(prefix+'/pin','carrier_pin',pin,lambda x:model.pin(x,mode,original),context)
            marker=dict(application='slug-fighter-carrier-journal',version=1,journal_id=journal_id,pin=pin,binding_id=binding)
            add(prefix+'/journal-marker','journal_marker',marker,lambda x:model.journal_marker(x,pin,journal_id),context)
            if mode=='restored':
                g=dict(version=1,state='ready',manager_schema=15,wallet_owner_id=b['wallet_owner_id'],carrier_journal_id=journal_id,ownership_pin_id=b['ownership_pin_id'],original_manifest_sha256=H(canonical(original)),original_manifest=original,activation_tip_hash=recipe['tip_hash'],activation_tip_height=110,activation_cursors=[0,0,0,0,2,0 if kind=='transfer' else 1,0,0])
                add(prefix+'/guard15','guard15',g,lambda x:model.restored(x,original),context,lambda x:digest('restored carrier wallet guard',x))
            request=dict(version=1,request_id=reqid,kind=kind,ownership_pin_id=crypto['expected_pin'],prior_state_id=crypto['expected_prior'],recipient_gym=crypto['recipient_gym'],commit_txid='' if kind=='transfer' else crypto['commit_txid'])
            add(prefix+'/request','carrier_request',request,model.crequest,context,lambda x:digest('fighter carrier request',x))
            retaddr,retscript=derive.child(1,0);return_path=dict(address=retaddr,script=retscript,branch=1,index=0)
            inputs=[]
            for coin in crypto['prevouts']:
                branch,index=map(int,coin['path'].split('/')[-2:]);inputs.append(dict(txid=bytes.fromhex(coin['hash_wire'])[::-1].hex(),output_index=coin['index'],atoms=coin['atoms'],script=coin['script'],branch=branch,index=index,block_hash=recipe['tip_hash'],block_height=109,coinbase=False))
            total=sum(x['atoms'] for x in inputs);funding=crypto['intent'][-64:]
            if kind=='transfer':
                eligible=sorted(inputs,key=lambda x:(-x['atoms'],bytes.fromhex(x['txid']),x['output_index']))
                selected=[];selectedsum=0
                for coin in eligible[:16]:
                    selected.append(coin);selectedsum+=coin['atoms']
                    if selectedsum>=511000:break
                need(selectedsum>=511000 and len(selected)==len(inputs),'smallest sufficient prefix')
                need(suffix!='sixteen-small' or (len(selected)==16 and selectedsum==512000 and selectedsum-510000==2000),'sixteen selection')
                selection_cases.append(dict(name=prefix+'/selection',eligible=eligible,selected_request_order=[dict(txid=x['txid'],output_index=x['output_index']) for x in selected],funding_wire_order=[dict(txid=x['txid'],output_index=x['output_index']) for x in inputs],total_atoms=selectedsum,commit_change_atoms=selectedsum-510000,scope='modeled eligibility, exact smallest-prefix arithmetic; signed bytes independently generated and checked'))

            if kind=='transfer':
                req=areq('carrier_return',reqid,binding,crypto);res=reserve(model,b,req,3);resdigest=M.allocation('reservation',res) if mode=='hosted' else ''
                new=dict(inputs=inputs,total_input_atoms=total,funding_digest=funding,allocation_id=reqid,reservation_digest=resdigest,return_path=return_path,commit_change_atoms=total-510000)
                restored=dict(start_index=0,end_index=1,selected_index=0,invalid_indices=[],cursors_before=[0,0,0,0,2,0,0,0],cursors_after=[0,0,0,0,2,1,0,0]) if mode=='restored' else None;recovered=None
            else:
                new=None;restored=None;first=crypto['prevouts'][0];br,ix=map(int,first['path'].split('/')[-2:])
                recovered=dict(commit_raw=crypto['commit_raw'],commit_txid=crypto['commit_txid'],commit_block_hash=recipe['tip_hash'],commit_height=109,commit_position=1,funding_digest=funding,transfer_id=crypto['transfer_id'],funding_public_key=crypto['redeem'][412:478],funding_branch=br,funding_index=ix,return_path=return_path,gym_authorization=crypto['envelope'][-128:])
            review=dict(version=1,request_id=reqid,kind=kind,binding_id=binding,ownership_pin_id=crypto['expected_pin'],prior_state_id=crypto['expected_prior'],sender_gym=crypto['sender_gym'],recipient_gym=crypto['recipient_gym'],transfer_id=crypto['transfer_id'],funding_digest=funding,input_details_digest=digest('fighter carrier owned inputs',inputs) if kind=='transfer' else '',input_count=len(inputs) if kind=='transfer' else 1,total_input_atoms=total if kind=='transfer' else 500000,return_address=retaddr,return_script=retscript,return_branch=1,return_index=0,commit_change_atoms=total-510000 if kind=='transfer' else 0,escrow_atoms=500000,commit_fee_atoms=10000 if kind=='transfer' else 0,spend_fee_atoms=10000,total_fee_atoms=20000 if kind=='transfer' else 10000,returned_atoms=490000,commit_txid='' if kind=='transfer' else crypto['commit_txid'])
            prep=dict(version=1,binding_id=binding,request=request,prepared_at_ms=recipe['base_time_ms']+10,tip_hash=recipe['tip_hash'],tip_height=110,ownership_head_digest=recipe['ownership_head_digest'],new_funding=new,restored_allocation=restored,recovery_funding=recovered,review=review,review_digest=digest('fighter carrier review',review))
            add(prefix+'/review','review',review,lambda x:model.review(x,request,binding),context,lambda x:digest('fighter carrier review',x))
            prep=add(prefix+'/preparation','preparation',prep,lambda x:model.preparation(x,pin,binding,crypto),context,lambda x:digest('fighter carrier preparation',x))
            if kind=='transfer':
                ar=dict(version=1,binding_id=binding,request_id=reqid,review_digest=prep['review_digest'],ownership_pin_id=review['ownership_pin_id'],prior_state_id=review['prior_state_id'],sender_gym=review['sender_gym'],recipient_gym=review['recipient_gym'],funding_digest=funding)
                add(prefix+'/authorization-request','authorization_request',ar,lambda x:model.authorization_request(x,prep),context)
            raw=lambda leg:dict(raw=crypto[leg+'_raw'],txid=crypto[leg+'_txid'],raw_sha256=H(bytes.fromhex(crypto[leg+'_raw'])))
            package=dict(version=1,review_digest=prep['review_digest'],transfer_id=crypto['transfer_id'],gym_authorization=crypto['envelope'][-128:],commit=raw('commit'),spend=raw('spend'),spend_kind='refund' if kind=='recover_refund' else 'reveal')
            add(prefix+'/package','signed_package',package,lambda x:model.package(x,prep,crypto),context,lambda x:digest('fighter carrier signed package',x))
            records={}
            for state in ('prepared','retained','complete','abandoned'):
                signed=state in ('retained','complete');record=dict(version=1,sequence=1,request_id=reqid,binding_id=binding,state=state,preparation=prep,preparation_digest=digest('fighter carrier preparation',prep),signed_package_digest=digest('fighter carrier signed package',package) if signed else '',commit_txid=crypto['commit_txid'] if signed else '',spend_txid=crypto['spend_txid'] if signed else '')
                records[state]=add(prefix+'/wallet-'+state,'wallet_record',record,lambda x:model.wallet_record(x,pin,binding,crypto),context)
            for state in ('prepared','retained'):
                record=records[state];keys=[]
                if kind=='transfer':keys=[bytes.fromhex(x['txid'])[::-1]+x['output_index'].to_bytes(4,'little') for x in inputs]
                if kind!='transfer' or state=='retained':keys.append(bytes.fromhex(crypto['commit_txid'])[::-1]+bytes(4))
                value=dict(version=1,owner_kind='carrier',request_id=reqid,binding_id=binding,preparation_digest=record['preparation_digest'],state=state)
                rows=[dict(key_hex=k.hex(),value=own(value)) for k in sorted(keys)]
                norm=lambda x:model.exclusions(x,[record],{reqid:package})
                # Composite database fixture, not a new public JSON wire object.
                add(prefix+'/exclusions-'+state,'exclusion_set',rows,norm,dict(context,records=[record],packages={reqid:package}))
                bad(prefix+'/exclusions-dangling-'+state,'exclusion_set',rows[:-1],norm,dict(context,records=[record],packages={reqid:package}))
            if mode=='hosted' and suffix=='one':
                # Existing frozen withdrawal owner plus carrier in one shared
                # exclusion index; no claimed live wallet or DB transaction.
                wd=dict(request_id=d['operation_id'],binding_id=d['binding_id'],preparation_digest=next(x['draft_digest'] for x in read('public/withdrawals-v1.json')['cases'] if x['name']==fixture+'-original-to-84'),state='retained',inputs=d['inputs'])
                mixed=own(rows)
                for coin in wd['inputs']:
                    mixed.append(dict(key_hex=(bytes.fromhex(coin['txid'])[::-1]+coin['output_index'].to_bytes(4,'little')).hex(),value=dict(version=1,owner_kind='withdrawal',request_id=wd['request_id'],binding_id=wd['binding_id'],preparation_digest=wd['preparation_digest'],state=wd['state'])))
                mixed.sort(key=lambda x:x['key_hex']);mx=dict(context,records=[records['retained']],packages={reqid:package},withdrawals=[wd])
                add(prefix+'/mixed-exclusions','exclusion_set',mixed,lambda x:model.exclusions(x,[records['retained']],{reqid:package},[wd]),mx)
                overlap=own(wd);overlap['inputs']=[inputs[0]]
                bad(prefix+'/withdrawal-carrier-overlap','exclusion_set',mixed,lambda x:model.exclusions(x,[records['retained']],{reqid:package},[overlap]),dict(mx,withdrawals=[overlap]))
            stages=['intent','prepared','signed']+(['commit_dispatching','commit_observed'] if kind=='transfer' else [])+['spend_dispatching','complete','abandoned']
            for stage in stages:
                hasprep=stage!='intent';signed=stage not in ('intent','prepared','abandoned')
                op=dict(version=1,sequence=1,request_id=reqid,revision=stages.index(stage)+1,created_at_ms=recipe['base_time_ms'],updated_at_ms=recipe['base_time_ms']+200,stage=stage,request=request,request_digest=digest('fighter carrier request',request),preparation=prep if hasprep else None,preparation_digest=digest('fighter carrier preparation',prep) if hasprep else '',signed_package=package if signed else None,signed_package_digest=digest('fighter carrier signed package',package) if signed else '',sign_authorization=auth(mode) if signed else None,commit_dispatch=dispatch(mode,'acknowledged') if kind=='transfer' and stage in ('commit_dispatching','commit_observed','spend_dispatching','complete') else None,spend_dispatch=dispatch(mode,'acknowledged' if stage=='complete' else 'pending') if stage in ('spend_dispatching','complete') else None,commit_observation=observe(package,'commit') if kind=='transfer' and stage in ('commit_observed','spend_dispatching','complete') else None,spend_observation=observe(package,'spend') if stage=='complete' else None)
                add(prefix+'/stage-'+stage,'operation',op,lambda x:model.operation(x,pin,binding,crypto),context)
                state=dict(application='slug-fighter-carrier-journal',version=1,journal_id=journal_id,binding_id=binding,clock_high_water_ms=op['updated_at_ms'],records=[op])
                add(prefix+'/state-'+stage,'journal_state',state,lambda x:model.state(x,pin,journal_id,{reqid:crypto}),context)
                if stage=='prepared':bad(prefix+'/unsigned-dispatch','operation',changed(op,['commit_dispatch'],dispatch(mode)),lambda x:model.operation(x,pin,binding,crypto),context)
                if stage=='signed':bad(prefix+'/missing-pair','operation',changed(op,['signed_package','spend'],None),lambda x:model.operation(x,pin,binding,crypto),context)
                if stage=='spend_dispatching':
                    for label,when in [('dispatch-before-prepared',prep['prepared_at_ms']-1),('dispatch-before-sign',op['sign_authorization']['authorized_at_ms']-1)]:
                        bad(prefix+'/'+label,'operation',changed(op,['spend_dispatch','authorization','authorized_at_ms'],when),lambda x:model.operation(x,pin,binding,crypto),context)
                    if kind=='transfer':bad(prefix+'/erased-commit-dispatch','operation',changed(op,['commit_dispatch'],None),lambda x:model.operation(x,pin,binding,crypto),context)
            for name,key,value in [('fee','spend_fee_atoms',9999),('total','total_fee_atoms',10000 if kind=='transfer' else 20000),('bool-atoms','escrow_atoms',True),('redirect','return_script',derive.child(1,1)[1])]:bad(prefix+'/'+name,'review',changed(review,[key],value),lambda x:model.review(x,request,binding),context)
            bad(prefix+'/changed-prior','preparation',changed(prep,['request','prior_state_id'],'%064x'%111),lambda x:model.preparation(x,pin,binding,crypto),context)
            if kind!='transfer':
                bad(prefix+'/recovery-allocates','preparation',changed(prep,['restored_allocation'],{}),lambda x:model.preparation(x,pin,binding,crypto),context)
                bad(prefix+'/funding-swapped-branch','preparation',changed(prep,['recovery_funding','funding_branch'],1-br),lambda x:model.preparation(x,pin,binding,crypto),context)
                other=own(prep);other['recovery_funding']['return_path']=dict(address=derive.child(1,1)[0],script=derive.child(1,1)[1],branch=1,index=1)
                for key in ('address','script','branch','index'):other['review']['return_'+key]=other['recovery_funding']['return_path'][key]
                other['review_digest']=digest('fighter carrier review',other['review'])
                bad(prefix+'/redirect-valid-return','preparation',other,lambda x:model.preparation(x,pin,binding,crypto),context)
                # A separately expected original import cannot be expanded by
                # replacing a caller-supplied guard or derived lookahead.
                for label,field in [('uncovered-funding','next_external'),('uncovered-return','next_internal')]:
                    limited=own(original);limited['accounts'][2][field]=0;limitedpin=own(pin);limitedpin['original_manifest_sha256']=H(canonical(limited));model.original=limited
                    bad(prefix+'/'+label,'preparation',prep,lambda x:model.preparation(x,limitedpin,binding,crypto),dict(context,pin=limitedpin,original_manifest=limited))
                model.original=original

            # Raw-only parser controls preserve bytes rather than normalizing before refusal.
            if suffix=='one' and mode=='hosted':
                baseline=canonical(review)
                variants=[('valid',baseline,True),('whitespace',b' '+baseline,False),('duplicate',baseline[:-1]+b',"version":1}',False),('unknown',baseline[:-1]+b',"extra":0}',False),('float',baseline.replace(b'"escrow_atoms":500000',b'"escrow_atoms":5e5'),False),('upper',baseline.replace(review['return_script'].encode(),review['return_script'].upper().encode()),False)]
                for name,raw,valid in variants:
                    try:M.raw_case(raw,4096,lambda x:model.review(x,request,binding));accepted=True
                    except M.Refused:accepted=False
                    need(accepted==valid,'raw parser case '+name);raw_cases.append(dict(name=prefix+'/raw-'+name,kind='review',context=context,raw_hex=raw.hex(),valid=valid))
    S=load('set73_wallet_sizing',HERE/'sizing_model.py')
    sizing=S.measure(M,positives,recipe)
    names={'positives':[x['name'] for x in positives],'negatives':[x['name'] for x in negatives],'raw_cases':[x['name'] for x in raw_cases]}
    need(names==plan['exact_case_names'],'fixed pre-execution case plan')
    return dict(version=1,scope=plan['scope'],contexts=contexts,positives=positives,negatives=negatives,raw_cases=raw_cases,selection_cases=selection_cases,sizing=sizing,model_only=plan['model_only'],counts={k:len(v) for k,v in names.items()})

def main():
    p=argparse.ArgumentParser();p.add_argument('--execute',action='store_true');p.add_argument('--input-root',type=Path,default=HERE/'inputs');p.add_argument('--input-catalog',type=Path,default=HERE/'input-catalog.json');p.add_argument('--approved-producer-lock',type=Path);p.add_argument('--approved-lock-sha256');p.add_argument('--output',type=Path);args=p.parse_args()
    if not args.execute:print('Source only; no model imports or outputs.');return
    paths,before,lock=guard(args);plan=json.loads((HERE/'case-plan.json').read_bytes())
    result=generate(paths,plan)
    need({n:H(safe(HERE,n).read_bytes()) for n in SOURCES}==before,'source changed')
    for row in json.loads(args.input_catalog.read_bytes())['files']:need(H(paths[row['path']].read_bytes())==row['sha256'],'input changed')
    result['input_catalog_sha256']=H(args.input_catalog.read_bytes());result['producer_lock_sha256']=args.approved_lock_sha256
    raw=json.dumps(result,ensure_ascii=False,allow_nan=False,separators=(',',':')).encode()+b'\n'
    need(len(raw)<=64*1024*1024,'bounded corpus output')
    args.output.mkdir(parents=True,exist_ok=False)
    (args.output/'wallet-v1.json').write_bytes(raw)
    report=dict(passed=True,source_unchanged=True,input_unchanged=True,output_sha256=H(raw),output_bytes=len(raw),counts=result['counts'],python=sys.version)
    (args.output/'report.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report))

if __name__=='__main__':main()
