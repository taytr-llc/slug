#!/usr/bin/env python3
"""Source/input-locked independent owned consumer composition generator."""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import struct
import sys
sys.dont_write_bytecode=True
HERE=Path(__file__).resolve().parent
SOURCE_NAMES={'consumer_model.py','generate_reference.py','sizing_model.py','case-plan.json','README.md'}

def need(v,msg):
    if not v:raise RuntimeError(msg)

def sha(b):return hashlib.sha256(b).hexdigest()

def load(name,path):
    spec=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(spec);sys.modules[name]=m;spec.loader.exec_module(m);return m

def produce(M,C,E,A,F,S,root,recipe):
    core=C.Model(E,json.loads((root/'core/inputs/recipe.json').read_bytes()))
    model=M.Model(C,core,A,F,recipe)
    public=json.loads((root/'core/public-core-v1.json').read_bytes())
    need(sha((root/'core/public-core-v1.json').read_bytes())==recipe['core_sha256'],'approved core bytes')
    rows={(r['family'],r['name']):r['value'] for r in public['positives']}
    pos=[];neg=[];raw=[];contexts={}; lifetimes=[]
    def positive(family,name,value,context,check):
        value=check(value); encoded=M.J(value)
        need(len(encoded)<=M.CAPS.get(family,65536),'positive cap '+family)
        pos.append(dict(family=family,name=name,context=context,value=value,canonical_hex=encoded.hex(),sha256=sha(encoded)))
        return value
    def negative(family,name,value,context,check):
        try:check(value)
        except (M.Refused,C.Refused):pass
        else:raise RuntimeError('negative accepted '+name)
        neg.append(dict(family=family,name=name,context=context,value=copy.deepcopy(value),expected='invalid'))
    def rawcase(family,name,data,context,check,valid):
        try:M.parse(data,family,check)
        except (M.Refused,C.Refused):need(not valid,'valid raw rejected '+name)
        else:need(valid,'bad raw accepted '+name)
        raw.append(dict(family=family,name=name,context=context,raw_hex=data.hex(),valid=valid))
    def read(base,file):return json.loads((root/'actual'/base/file).read_bytes())
    for base in recipe['fixtures']:
        version=base.split('/')[0]; cp=rows['pin',base]; admission=rows['admission',base]
        original=read(base,'capture.json'); previous_rows=read(base,'rows.json')
        if ('state',base+'/initial') not in rows:
            # Conditional derived state wrappers over the already-locked public
            # winning fighter; no complete admission-prefix or chain claim.
            pid=core.pin_id(cp);initial=core.initial(pid,admission,original['status']['pin']['identity']['gym_public_key'])
            other=read(version+'/zero256-awarded','expected-pin.json')['identity']['gym_public_key']
            final=core.transferred(pid,initial,'64'*32,'65'*32,dict(height=160,hash='52'*32),other)
            rows['state',base+'/initial']=initial;rows['state',base+'/final']=final
            rows['state',base+'/reacquired']=core.transferred(pid,final,'66'*32,'65'*32,dict(height=221,hash='53'*32),initial['owner_public_key'])
            template=copy.deepcopy(rows['head',version+'/zero256-awarded']);template.update(admission_sequence=admission['admission_sequence'],admission_head=core.validate_admission(cp,admission));rows['head',base]=template
        need(M.same(admission['record'],read(base,'admission.json')),'immutable source record')
        birth=model.birth(admission); need(M.same(birth,original['assignment']),'old birth source digest')
        for phase in recipe['states']:
            name=base+'/'+phase; state=rows['state',name]
            other_fixture='zero256-awarded' if base.endswith('incrementing256-awarded') else 'incrementing256-awarded'
            owner_base=version+('/'+other_fixture if phase=='final' else '/'+base.split('/')[1])
            stable=read(owner_base,'expected-pin.json')
            current_wins=sum(1 for row in previous_rows if row['event']['winner']==birth['prospect_id'] and any(corner['prospect_id']==birth['prospect_id'] and corner['gym_id']==stable['identity']['gym_id'] for corner in (row['event']['corner_a'],row['event']['corner_b'])))
            pin=dict(version=1,stable=stable,ownership=cp)
            pin=model.pin(pin); aid=core.validate_admission(cp,admission)
            omap,mapid,prefix=core.ownership_map(state['pin_id'],admission['admission_sequence'],state)
            sourcehead=copy.deepcopy(rows['head',base])
            if phase=='initial':sourcehead.update(chain_height=0,chain_hash=cp['source']['genesis'])
            elif phase=='reacquired':sourcehead.update(chain_height=221,chain_hash='53'*32)
            replay=dict(pin_id=state['pin_id'],admission_sequence=admission['admission_sequence'],admission_head=aid,publication_sequence=sourcehead['publication_sequence'],publication_head=sourcehead['publication_head'],chain_height=sourcehead['chain_height'],chain_hash=sourcehead['chain_hash'],ownership_map_id=mapid,finalized_prefix_id=prefix,conflict_id='')
            sourcehead.update(finalized_prefix_id=prefix,replay_digest=C.digest('SLUG fighter ownership replay',replay).hex())
            slots=['']*10
            status=dict(version=1,store_id=M.H('stable store',pin),pin=pin,head=sourcehead,roster_revision=0,slots=slots,operation_count=0,pending_operation_id='',last_operation_id='',last_as_of_ms=recipe['as_of_ms'])
            contexts[name]=dict(pin=pin,admission=admission,state=state,head=sourcehead,original_capture=original,original_rows=previous_rows)
            positive('pin',name,pin,name,model.pin)
            positive('status',name,status,name,model.status)
            mv=1 if version=='v1' else 2; manifest=A.V1.generate(birth['work_hash']) if mv==1 else A.generate(birth['work_hash'])
            item=dict(birth=birth,admission=admission,ownership=state,manifest=manifest,condition=F.project(1,manifest['dna'],birth['born_at_ms'],recipe['as_of_ms'],original['totals']['xp']),career=dict(activation=original['activation'],career_digest=original['career_digest'],totals=original['totals'],current_gym_wins=current_wins))
            positive('item',name,item,name,lambda v:model.item(pin,v,recipe['as_of_ms']))
            page=dict(version=1,status=status,head=sourcehead,as_of_ms=recipe['as_of_ms'],after=0,next_after=birth['sequence'],has_more=False,items=[item])
            positive('page',name,page,name,model.page)
            request=dict(expected_revision=0,slot=1,prospect_id=birth['prospect_id'],expected_state_id=state['state_id'],head=sourcehead)
            intent=dict(version=1,sequence=1,operation_id=recipe['operation_id'],generation=recipe['generation'],view_id=recipe['view_id'],request=request,prepared_at_ms=recipe['prepared_at_ms'])
            prepared=dict(version=1,intent=intent,state='prepared',receipt=None,abandoned_at_ms=0)
            positive('operation',name+'/prepared',prepared,name,lambda v:model.operation(pin,v))
            receipt=dict(version=1,operation_id=intent['operation_id'],generation=intent['generation'],view_id=intent['view_id'],request=request,previous_prospect_id='',changed=True,previous_revision=0,revision=1,slots=[birth['prospect_id']]+['']*9,committed_at_ms=recipe['committed_at_ms'])
            committed=dict(version=1,intent=intent,state='committed',receipt=receipt,abandoned_at_ms=0)
            positive('operation',name+'/committed',committed,name,lambda v:model.operation(pin,v))
            abandoned=copy.deepcopy(prepared);abandoned.update(state='abandoned',abandoned_at_ms=recipe['committed_at_ms'])
            positive('operation',name+'/abandoned',abandoned,name,lambda v:model.operation(pin,v))
            history=dict(version=1,status=status,activation=original['activation'],career_digest=original['career_digest'],as_of_ms=recipe['as_of_ms'],birth=birth,ownership=state,prefix_count=original['prefix_count'],event_count=original['event_count'],totals=original['totals'],current_gym_wins=current_wins)
            history['history_id']=M.H('fighter history',history)
            positive('history',name,history,name,model.history)
            hp=dict(version=1,capture=history,after=0,next=len(previous_rows),has_more=False,items=previous_rows)
            positive('history_page',name,hp,name,model.history_page)
            # Contextual mutations never rewrite expected signatures/birth to
            # make invalid input appear authoritative.
            for label,path,val in [('birth_gym',['birth','gym_id'],'aa'*32),('state',['ownership','state_id'],'aa'*32),('owner',['ownership','owner_public_key'],rows['state',base+('/final' if phase!='final' else '/initial')]['owner_public_key']),('source',['admission','pin_id'],'aa'*32),('dna',['manifest','dna','talent_budget'],0),('xp',['condition','pro_xp'],15)]:
                bad=copy.deepcopy(item);p=bad
                for k in path[:-1]:p=p[k]
                p[path[-1]]=val
                negative('item',name+'/'+label,bad,name,lambda v:model.item(pin,v,recipe['as_of_ms']))
            for label,field,val in [('bool_version','version',True),('duplicate_slot','slots',[birth['prospect_id']]*10),('foreign_store','store_id','ab'*32),('operation_cap','operation_count',4097)]:
                bad=copy.deepcopy(status);bad[field]=val;negative('status',name+'/'+label,bad,name,model.status)
            for label in ('mismatched_request','wrong_revision','commit_before_prepare','terminal_receipt_missing'):
                bad=copy.deepcopy(committed)
                if label=='mismatched_request':
                    bad['receipt']['request']=copy.deepcopy(bad['receipt']['request']);bad['receipt']['request']['slot']=2
                elif label=='wrong_revision':bad['receipt']['revision']=2
                elif label=='commit_before_prepare':bad['receipt']['committed_at_ms']=intent['prepared_at_ms']-1
                else:bad['receipt']=None
                negative('operation',name+'/'+label,bad,name,lambda v:model.operation(pin,v))
            if phase=='final':
                need(birth['gym_id']!=stable['identity']['gym_id'],'transferred birth distinct')
                if previous_rows:
                    historical=next(corner for corner in (previous_rows[0]['event']['corner_a'],previous_rows[0]['event']['corner_b']) if corner['prospect_id']==birth['prospect_id'])
                    need(historical['gym_id']!=stable['identity']['gym_id'],'older corner survives')
                need(current_wins==0,'prior gym win is not transferred')
                lifetimes.append(dict(name=name,rule='old-view capture never retargeted; current recipient may see immutable birth and old event corner',birth_gym=birth['gym_id'],viewer_gym=stable['identity']['gym_id'],history_rows=len(previous_rows)))
            if phase=='reacquired':lifetimes.append(dict(name=name,rule='old initial state authorization stays stale after reacquisition',old_state=rows['state',base+'/initial']['state_id'],new_state=state['state_id']))
    # Exact Saturday noon composition. Only the unawarded fighter enters C1;
    # awarded v2 remains a display/history case, never forced into a later bout.
    base='v1/zero256-unawarded'; c=contexts[base+'/final']; pin=c['pin']; adm=c['admission']; state=c['state']; birth=model.birth(adm)
    cutoffms=recipe['cutoff_ms']; saturday=recipe['saturday']
    u32=lambda x:struct.pack('<I',x);u64=lambda x:struct.pack('<Q',x);lp=lambda s:u32(len(s.encode()))+s.encode()
    stable=pin['stable']; namespace=sha(b'SLUG local candidate entry namespace\0v1\0'+u32(stable['identity']['network'])+bytes.fromhex(stable['identity']['genesis'])+bytes.fromhex(stable['account_authority_id']))
    need(namespace==M.entry_namespace(stable),'independent inherited namespace recipe')
    calendar=sha(b'SLUG candidate entry calendar\0v1\0'+u32(1)+lp('America/Los_Angeles')+hashlib.sha256((root/'inherited/gymentry/calendar/America_Los_Angeles.tzif').read_bytes()).digest()+b''.join(u32(x) for x in [2026,2099,6,12,0]))
    rules=sha(b'SLUG candidate entry rules\0v1\0'+b''.join(u32(x) for x in [1,5,1])+u64(0)+b''.join(u32(x) for x in [1,1,1])+lp('slug-fighter-condition-candidate-v1')+u32(0)+b''.join(lp(x) for x in ['light','welter','middle','light_heavy','heavy','intake-only']))
    eventid=sha(b'SLUG candidate Saturday event\0v1\0'+bytes.fromhex(namespace+calendar+rules)+saturday.encode()+u64(cutoffms))
    entryevent=dict(version=1,event_id=eventid,namespace_id=namespace,calendar_id=calendar,rules_id=rules,saturday=saturday,cutoff_ms=cutoffms)
    entrypins=[]
    for phase in ('initial','final'):
        sp=contexts[base+'/'+phase]['pin'];ep=dict(version=1,namespace_id=namespace,calendar_id=calendar,rules_id=rules,stable_store_id=M.H('stable store',sp),stable_pin=sp);entrypins.append(ep)
        positive('entry_pin',phase,ep,base+'/'+phase,model.entry_pin)
        bad=copy.deepcopy(ep);bad['namespace_id']=recipe['invalid_namespace_id'];negative('entry_pin',phase+'/foreign_namespace',bad,base+'/'+phase,model.entry_pin)
    entrypins.sort(key=lambda x:x['stable_pin']['stable']['identity']['gym_id'])
    eventrules=M.H('event rules',dict(version=1,boundary='committed_before_cutoff',condition_time='cutoff',ownership_basis='finalized_public_chain',condition_version=1,projector_id='slug-fighter-condition-candidate-v1',xp=0,training=0,participants=2,members_per_gym=5,exclusion_order='retired_then_not_rostered',later_changes='immutable',seed_authority='unavailable'))
    eventpin=dict(version=1,calendar_id=calendar,rules_id=eventrules,ownership=pin['ownership'],participants=entrypins)
    # This fixture's two Saturday noons do not cross DST. Runtime uses the
    # inherited pinned calendar, never this literal seven-day arithmetic.
    estore=M.H('event store',eventpin);ev=dict(version=1,event_id=M.H('event',dict(version=1,store_id=estore,source_event_id=eventid)),source_event_id=eventid,store_id=estore,namespace_id=namespace,calendar_id=calendar,rules_id=eventrules,saturday=saturday,cutoff_ms=cutoffms,incident_deadline_ms=cutoffms+604800000)
    head=c['head']; pid=state['pin_id'];first=dict(request_start_ms=cutoffms-2,response_end_ms=cutoffms-1,instance_id='71'*32,revision='19',height=head['chain_height'],hash=head['chain_hash']);second=copy.deepcopy(first);second.update(request_start_ms=cutoffms,response_end_ms=cutoffms+1)
    intent=dict(version=1,pin_id=pid,event_id=ev['event_id'],cutoff_ms=cutoffms,first=first,second=second,accepted_admission_sequence=1,publication_sequence=head['publication_sequence'],publication_head=head['publication_head'],finality_sequence=rows['boundary',base]['intent']['finality_sequence'],finality_body_head=rows['boundary',base]['intent']['finality_body_head'],prior_cutoff_id='00'*32)
    boundary=core.signed('SLUG fighter ownership boundary statement',dict(version=1,pin_id=pid,intent=intent))
    _,mapid,_=core.ownership_map(pid,1,state)
    cutoff=core.signed('SLUG fighter ownership cutoff statement',dict(version=1,pin_id=pid,event_id=ev['event_id'],cutoff_ms=cutoffms,boundary=boundary,admission_sequence=1,admission_head=head['admission_head'],head=head,ownership_digest=mapid))
    participants=[]
    manifest=A.V1.generate(birth['work_hash']);condition=F.project(1,manifest['dna'],birth['born_at_ms'],cutoffms,0)
    for ep in entrypins:
        sp=ep['stable_pin'];identity=sp['stable']['identity'];mine=identity['gym_public_key']==state['owner_public_key'];savedstate=state if mine else rows['state',base+'/initial']
        sid=M.H('stable store',sp);eid=M.H('entry store',ep);partid=M.H('event participant',dict(version=1,event_store_id=estore,entry_store_id=eid,gym_id=identity['gym_id']))
        entryid=M.H('entry',dict(version=1,store_id=eid,event_id=eventid,gym_id=identity['gym_id']))
        savedmember=dict(slot=1,birth=birth,ownership=savedstate,**{'class':manifest['physical']['weight_class']},condition_at_save=F.project(1,manifest['dna'],birth['born_at_ms'],recipe['committed_at_ms'],0))
        receipt=dict(version=1,store_id=eid,entry_id=entryid,event=entryevent,operation_id=recipe['operation_id'],operation_sequence=1,generation=recipe['generation'],view_id=recipe['view_id'],action='replace',previous_revision=0,revision=1,roster_revision=1,ownership_head=contexts[base+('/final' if mine else '/initial')]['head'],committed_at_ms=recipe['committed_at_ms'],members=[savedmember])
        phase='final' if mine else 'initial'
        stableop=next(r['value'] for r in pos if r['family']=='operation' and r['name']==base+'/'+phase+'/committed')
        rh=M.H('stable cutoff history',dict(version=1,store_id=sid,event_id='',cutoff_ms=cutoffms));rh=M.H('stable cutoff history',dict(previous_digest=rh,operation=stableop))
        roster=dict(version=1,store_id=sid,cutoff_ms=cutoffms,committed_count=1,last_sequence=1,history_digest=rh,revision=1,slots=[birth['prospect_id']]+['']*9,assigned_at_ms=[recipe['committed_at_ms']]+[0]*9)
        er=dict(version=1,event_id=eventid,saturday=saturday,action='replace',expected_revision=0,expected_roster_revision=1,prospect_ids=[birth['prospect_id']])
        ei=dict(version=1,sequence=1,operation_id=recipe['operation_id'],generation=recipe['generation'],view_id=recipe['view_id'],request=er,prepared_at_ms=recipe['prepared_at_ms'])
        eo=dict(version=1,intent=ei,state='committed',receipt=receipt,abandoned_at_ms=0)
        eh=M.H('entry cutoff history',dict(version=1,store_id=eid,event_id=eventid,cutoff_ms=cutoffms));eh=M.H('entry cutoff history',dict(previous_digest=eh,operation=eo))
        entry=dict(version=1,store_id=eid,event=entryevent,entry_id=entryid,committed_count=1,last_sequence=1,history_digest=eh,latest_receipt=receipt)
        member=dict(slot=1,slot_assigned_at_ms=recipe['committed_at_ms'],birth=birth,ownership=state,**{'class':manifest['physical']['weight_class']},condition_at_cutoff=condition,eligibility='eligible')
        exclusion=dict(prospect_id=state['prospect_id'],state_id=state['state_id'],owner_public_key=state['owner_public_key'],reason='not_owned_at_cutoff')
        participants.append(dict(version=1,participant_id=partid,entry_store_id=eid,owner=dict(ownership_basis='finalized_public_chain',account_authority_id=sp['stable']['account_authority_id'],account_id=sp['stable']['account_id'],gym_id=identity['gym_id']),roster=roster,entry=entry,members=[member] if mine else [],excluded=[] if mine else [exclusion]))
    snapshot=dict(version=1,event=ev,cutoff=cutoff,participants=participants);contexts['event']=dict(pin=eventpin,source='modeled exact-noon attestation; not normal-work or live chronology')
    positive('snapshot','transferred-before-noon',snapshot,'event',lambda v:model.snapshot(eventpin,v))
    for label in ('cutoff_event','cutoff_time','ownership_basis','missing_partition','duplicate_fighter','birth_as_owner'):
        bad=copy.deepcopy(snapshot);receiving=next(p for p in bad['participants'] if p['members']);outgoing=next(p for p in bad['participants'] if p['excluded'])
        if label=='cutoff_event':bad['cutoff']['event_id']='ab'*32
        elif label=='cutoff_time':bad['event']['cutoff_ms']+=1
        elif label=='ownership_basis':receiving['owner']['ownership_basis']='candidate_assignment'
        elif label=='missing_partition':outgoing['excluded']=[]
        elif label=='duplicate_fighter':outgoing['members']=copy.deepcopy(receiving['members']);outgoing['excluded']=[]
        else:receiving['members'][0]['ownership']['owner_public_key']=rows['state',base+'/initial']['owner_public_key']
        negative('snapshot',label,bad,'event',lambda v:model.snapshot(eventpin,v))
    # Canonical raw controls include nested order, not merely top-level order.
    example=next(x for x in pos if x['family']=='page');v=example['value'];ctx=example['context'];b=M.J(v)
    rawcase('page','canonical',b,ctx,model.page,True)
    rawcase('page','trailing-lf',b+b'\n',ctx,model.page,False)
    rawcase('page','duplicate-version',b.replace(b'{"version":1,',b'{"version":1,"version":1,',1),ctx,model.page,False)
    rawcase('page','numeric-float',b.replace(b'"version":1',b'"version":1.0',1),ctx,model.page,False)
    bad=copy.deepcopy(v);bad['status']={k:bad['status'][k] for k in reversed(list(bad['status']))};rawcase('page','nested-order',M.J(bad),ctx,model.page,False)
    rawcase('page','top-order',M.J({k:v[k] for k in reversed(list(v))}),ctx,model.page,False)
    rawcase('page','null-items',b.replace(b'"items":[',b'"items":null,"old_items":[',1),ctx,model.page,False)
    compatibility=[]
    for legacy,current in [(0,0),(1,0),(0,1),(1,1)]:
        try:choice=M.select_career_source(legacy,current)
        except M.Refused:choice='owned_career_conflict'
        compatibility.append(dict(legacy_events=legacy,current_events=current,expected=choice,claim='pure compatibility rule; no reader capability'))
    sizes=S.compositions(M.CAPS)
    return dict(version=1,scope='independent conditional owned-consumer bytes, immutable old public birth/career inputs; no imported authority',contexts=contexts,positives=pos,negatives=neg,raw_cases=raw,lifecycle_cases=lifetimes,career_compatibility=compatibility,budgets=sizes)

def main():
    p=argparse.ArgumentParser();p.add_argument('--execute',action='store_true');p.add_argument('--input-root',type=Path,required=True);p.add_argument('--approved-producer-lock',type=Path,required=True);p.add_argument('--approved-lock-sha256',required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args()
    need(a.execute,'explicit execution required');lockraw=a.approved_producer_lock.read_bytes();need(sha(lockraw)==a.approved_lock_sha256,'approved lock bytes');lock=json.loads(lockraw)
    need(lock.get('approved_by')=='root' and lock.get('execution_authorized') is True,'approval');need(set(lock['sources'])==SOURCE_NAMES,'exact source names')
    for rel,digest in lock['sources'].items():need(sha((HERE/rel).read_bytes())==digest,'source '+rel)
    catraw=(HERE/'input-catalog.json').read_bytes();need(sha(catraw)==lock['input_catalog_sha256'],'catalog');catalog=json.loads(catraw);seen=set()
    for row in catalog['files']:
        rel=Path(row['path']);need(not rel.is_absolute() and '..' not in rel.parts and str(rel) not in seen,'input path');seen.add(str(rel));path=a.input_root/rel;need(path.is_file() and not path.is_symlink() and path.stat().st_size==row['bytes'] and sha(path.read_bytes())==row['sha256'],'input '+str(rel))
    need(not any(x.is_symlink() for x in a.input_root.rglob('*')),'input symlink');need({str(x.relative_to(a.input_root)) for x in a.input_root.rglob('*') if x.is_file()}==seen,'exact input set');need(not a.output.exists(),'exclusive output')
    M=load('owned_consumer_reference_model',HERE/'consumer_model.py');C=load('archived_owned_core',a.input_root/'core/core_model.py');E=load('archived_owned_crypto',a.input_root/'core/inputs/inherited/crypto.py');A=load('archived_owned_appearance',a.input_root/'appearance/independent/reference_v2.py');F=load('archived_owned_condition',a.input_root/'condition/reference_condition.py');S=load('owned_consumer_size_model',HERE/'sizing_model.py')
    result=produce(M,C,E,A,F,S,a.input_root,json.loads((a.input_root/'recipe.json').read_bytes()));raw=(json.dumps(result,indent=2,ensure_ascii=True,allow_nan=False)+'\n').encode()
    a.output.parent.mkdir(parents=True,exist_ok=True)
    with a.output.open('xb') as f:f.write(raw)
    print(json.dumps(dict(output=str(a.output),sha256=sha(raw),positives=len(result['positives']),negatives=len(result['negatives']),raw_cases=len(result['raw_cases']),bytes=len(raw))))
if __name__=='__main__':main()
