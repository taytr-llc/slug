#!/usr/bin/env python3
"""Independent Set44 reference: frozen Python inputs and contracts only.
Semantic journals and fault events are modeled, not portable authority proofs.
No Go imports/outputs, filesystem-journal execution, mining, SVG or Argon work.
"""
import argparse, copy, datetime as dt, hashlib, importlib.util, json, re, struct, sys
from pathlib import Path
sys.dont_write_bytecode=True
HERE=Path(__file__).resolve().parent
CORE='7e0c2842cb6fac2c67896c8e646ffe5778f1a0d03d8ec0e386a074e289537379'
HELPER='fedecce1bb5f7a4c975390c915dd8671850790d284899194e23b1e7329cc82f4'
CLARIFICATION='1a43251fbee36cfb6fb4cb22a7ef677555f46876e4a00f3a231f6cba94149ead'
def sha(b):return hashlib.sha256(b).hexdigest()
def canon(v):return json.dumps(v,ensure_ascii=True,separators=(',',':'),allow_nan=False).encode('ascii')
def pretty(v):return (json.dumps(v,ensure_ascii=True,indent=2,allow_nan=False)+'\n').encode('ascii')
def clone(v):return copy.deepcopy(v)
def load(name,p):
    s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);sys.modules[name]=m;s.loader.exec_module(m);return m
INPUTS=json.loads((HERE/'input_hashes.json').read_bytes())
for p,h in INPUTS.items():
    b=(HERE/p).read_bytes();assert len(b)==h['bytes'] and sha(b)==h['sha256'],p
assert INPUTS['inputs/contracts/set44-core-contract-draft.md']['sha256']==CORE
assert INPUTS['inputs/contracts/set44-helper-contract-proposal.md']['sha256']==HELPER
assert INPUTS['inputs/contracts/set44-contract-clarification-1.md']['sha256']==CLARIFICATION
OLD=load('set44_frozen_entry_reference',HERE/'inputs/entry/reference_entry.py')
SAFE=OLD.SAFE;DATE_MAX=OLD.DATE_MAX;CLASSES=OLD.CLASSES
class Refusal(ValueError):pass
def fail(code='invalid_request'):raise Refusal(code)
def req(v,code='invalid_request'):
    if not v:fail(code)
def integer(x,lo=0,hi=SAFE):req(type(x)is int and lo<=x<=hi)
def hexid(x,empty=False):req(type(x)is str and (empty and x=='' or re.fullmatch('[0-9a-f]{64}',x)))
def exact(v,keys):req(type(v)is dict and set(v)==set(keys))
def ordered(v,shape):
    if type(shape)is dict:exact(v,shape);return {k:ordered(v[k],shape[k]) for k in shape}
    if type(shape)is list:req(type(v)is list);return [ordered(x,shape[0]) for x in v] if shape else clone(v)
    req(type(v)is type(shape));return v
raw=lambda s:bytes.fromhex(s)
u32=lambda n:struct.pack('<I',n)
u64=lambda n:struct.pack('<Q',n)
def lp(b):return u32(len(b))+b
def domain(s):return ('SLUG '+s+'\0v1\0').encode('ascii')
def label(s):return sha(('Set44 modeled public '+s).encode('ascii'))
RULES=dict(version=1,boundary='committed_before_cutoff',condition_time='cutoff',ownership_basis='candidate_assignment',condition_version=1,projector_id='slug-fighter-condition-candidate-v1',xp=0,training=0,participants=2,members_per_gym=5,exclusion_order='retired_then_not_rostered',later_changes='immutable',seed_authority='unavailable')
RULE_PRE=domain('candidate snapshot rules')+lp(canon(RULES));RULES_ID=sha(RULE_PRE)
def pin():return dict(version=1,calendar_id=OLD.CALENDAR_ID,rules_id=RULES_ID,participants=sorted([OLD.entry_pin(x['pin']) for x in OLD.OLD['pins']],key=lambda p:p['stable_pin']['identity']['gym_id']))
PIN=pin()
def sid(p):return sha(store_pre(p))
def store_pre(p):return domain('candidate event store')+lp(canon(p))
def ep_for(p,gym):
    req(type(gym)is str)
    for x in p['participants']:
        if x['stable_pin']['identity']['gym_id']==gym:return x
    fail('unauthorized')
def fixture_name(ep):return next(x['name'] for x in OLD.OLD['pins'] if x['pin']['identity']==ep['stable_pin']['identity'])
def view_for(ep):return clone(next(x['view'] for x in OLD.PUBLIC['records'] if x['fixture']==fixture_name(ep)))
def deadline(s):
    d=OLD.date_value(s)+dt.timedelta(days=7)
    t=OLD.ms(dt.datetime(d.year,d.month,d.day,12,tzinfo=OLD.TZ))
    assert t==OLD.tail_cutoff(d)
    return t

def event_pre(p,s):
    e=OLD.event(p['participants'][0],s)
    return domain('candidate cutoff event')+raw(sid(p))+raw(e['event_id'])+u64(e['cutoff_ms'])+u64(deadline(s))
def event(p,s):
    OLD.date_value(s)
    e=OLD.event(p['participants'][0],s)
    return dict(version=1,event_id=sha(event_pre(p,s)),source_event_id=e['event_id'],store_id=sid(p),namespace_id=e['namespace_id'],calendar_id=p['calendar_id'],rules_id=p['rules_id'],saturday=s,cutoff_ms=e['cutoff_ms'],incident_deadline_ms=deadline(s))
def snap_pre(v):return domain('candidate cutoff input')+lp(canon(v))
def receipt_pre(v):return domain('candidate cutoff receipt')+lp(canon(v))
def stable_start(ep,C):return domain('stable cutoff history')+raw(ep['stable_store_id'])+u64(C)
def entry_start(ep,s):return domain('entry cutoff history')+raw(OLD.store_id(ep))+raw(OLD.event(ep,s)['event_id'])+u64(OLD.cutoff(s))
def fold_row(kind,h,seq,row):return domain(kind+' cutoff row')+raw(h)+u64(seq)+lp(canon(row))
FIELDS={
'Pin':'version calendar_id rules_id participants',
'Info':'version store_id pin created_at_ms event_count',
'Event':'version event_id source_event_id store_id namespace_id calendar_id rules_id saturday cutoff_ms incident_deadline_ms',
'Owner':'ownership_basis account_authority_id account_id gym_id',
'Fence':'version store_id cutoff_ms observed_at_ms operation_count',
'Roster':'version store_id cutoff_ms committed_count last_sequence history_digest revision slots assigned_at_ms',
'Entry':'version store_id event entry_id committed_count last_sequence history_digest latest_receipt',
'Member':'slot slot_assigned_at_ms assignment class condition_at_cutoff eligibility',
'ParticipantSnapshot':'version participant_id entry_store_id owner roster entry members',
'CaptureEvidence':'version participant_id stable_fence entry_fence captured_at_ms',
'ParticipantCapture':'version snapshot evidence',
'SnapshotInput':'version event participants',
'SnapshotReceipt':'version input snapshot_digest captures completed_at_ms',
'EventStatus':'version store_id event exists phase captured_participants snapshot_digest coordinator_created_at_ms created_at_ms completed_at_ms observed_at_ms seed_authority',
'CaptureResult':'version status capture replayed',
'HistoryPage':'version store_id after_event_id next_event_id has_more observed_at_ms events',
'Marker':'application version store_id pin',
'Metadata':'version store_id pin created_at_ms clock_ms event_count',
'Init':'version store_id pin',
'EventRow':'version event phase created_at_ms completed_at_ms snapshot_digest',
'EventRequest':'id command view_id saturday after_event_id limit job_id',
'EventResponse':'id ok view store_id snapshot status record history job error',
'EventSnapshot':'version state error view store_id pin coordinator_created_at_ms participant_id active_job_id queued_job_id last_job_id',
'EventJob':'version job_id kind state view store_id event_id saturday participant_id error result',
'StableIntent':'version sequence operation_id generation view_id request prepared_at_ms',
'StableReceipt':'version operation_id generation view_id request previous_prospect_id changed previous_revision revision slots committed_at_ms',
'StableOperation':'version intent state receipt abandoned_at_ms'}
FIELDS={k:v.split() for k,v in FIELDS.items()}
CAPS={'Pin':16384,'Info':20480,'Event':2048,'Owner':1024,'Fence':2048,'Roster':4096,'Entry':36864,'Member':4096,'ParticipantSnapshot':57344,'CaptureEvidence':2048,'ParticipantCapture':61440,'SnapshotInput':122880,'SnapshotReceipt':126976,'EventStatus':2048,'CaptureResult':131072,'HistoryPage':131072,'Marker':16384,'Metadata':20480,'Init':20480,'EventRow':2048,'EventRequest':8192,'EventResponse':131072,'EventSnapshot':131072,'EventJob':131072}
ERRORS='invalid_request unauthorized forbidden stale_view busy event_unavailable event_corrupt event_capacity event_clock event_calendar event_not_due event_created_too_late event_incomplete event_conflict event_not_found event_cancelled outcome_unknown event_setup_incomplete'.split()
def norm(kind,v):exact(v,FIELDS[kind]);return {k:clone(v[k]) for k in FIELDS[kind]}
def val_pin(p):
    p=norm('Pin',p);req(p['version']==1 and type(p['version'])is int and p['calendar_id']==OLD.CALENDAR_ID and p['rules_id']==RULES_ID)
    req(type(p['participants'])is list and len(p['participants'])==2)
    out=[];names=[]
    for ep in p['participants']:
        ep=ordered(ep,PIN['participants'][0]);sp=ep['stable_pin'];ident=sp['identity'];name=next((x['name'] for x in OLD.OLD['pins'] if x['pin']['identity']==ident),None);req(name is not None);names.append(name)
        base=next(x['pin'] for x in OLD.OLD['pins'] if x['name']==name)
        req(sp['version']==1 and sp['condition_version']==1 and sp['projector_id']==base['projector_id'])
        for k in ('account_authority_id','account_id'):hexid(sp[k])
        req(set(sp['source'])==set(base['source']))
        for k in sp['source']:
            if k=='authority_id':hexid(sp['source'][k])
            else:req(sp['source'][k]==base['source'][k])
        req(ep==OLD.entry_pin(sp));out.append(ep)
    req(sorted(names)==['incrementing256','zero256']);req(out==sorted(out,key=lambda x:x['stable_pin']['identity']['gym_id']))
    a,b=[x['stable_pin'] for x in out];req(a['account_authority_id']==b['account_authority_id'] and a['account_id']!=b['account_id']);req(out[0]['namespace_id']==out[1]['namespace_id']);p['participants']=out;return p

def assignment(a,ep,C):
    a=ordered(a,OLD.OLD['cases'][0]['assignment']);integer(a['version'],1,1);integer(a['sequence'],1,1000)
    for k in ('prospect_id','gym_id','work_hash','descriptor_id','manifest_digest','portrait_digest','source_record_digest'):hexid(a[k])
    req(a['gym_id']==ep['stable_pin']['identity']['gym_id'] and a['descriptor_id']==ep['stable_pin']['source']['descriptor_id']);integer(a['born_at_ms'],1,DATE_MAX);integer(a['accepted_at_ms'],a['born_at_ms'],DATE_MAX);req(a['accepted_at_ms']<C)
    m=OLD.MAN.generate(a['work_hash']);req(a['manifest_digest']==OLD.MAN.digest_manifest(OLD.MAN.canonical(m)));return a,m

def validate(kind,value,p=PIN):
    if kind=='Pin':v=val_pin(value)
    elif kind in ('EventRequest','EventResponse','EventSnapshot','EventJob'):v=validate_wire(kind,value,p)
    else:
        v=norm(kind,value)
        if 'version'in v:integer(v['version'],1,1)
        if 'store_id'in v and kind not in ('Roster','Entry','Fence'):req(v['store_id']==sid(p))
        if kind in ('Info','Marker','Metadata','Init'):
            v['pin']=val_pin(v['pin']);req(v['pin']==p)
            if kind=='Marker':req(v['application']=='slug-candidate-events')
            if kind in ('Info','Metadata'):integer(v['created_at_ms'],1,DATE_MAX);integer(v['event_count'],0,256)
            if kind=='Metadata':integer(v['clock_ms'],v['created_at_ms'],DATE_MAX)
        elif kind=='Event':req(v==event(p,v['saturday']))
        elif kind=='ParticipantSnapshot':v=val_part(v,p)
        elif kind=='CaptureEvidence':v=val_evidence(v,p)
        elif kind=='ParticipantCapture':
            v['snapshot']=validate('ParticipantSnapshot',v['snapshot'],p);v['evidence']=validate('CaptureEvidence',v['evidence'],p);req(v['snapshot']['participant_id']==v['evidence']['participant_id']);C=v['snapshot']['roster']['cutoff_ms'];req(all(v['evidence'][k]['cutoff_ms']==C for k in ('stable_fence','entry_fence')))
            for k,h in [('stable_fence','roster'),('entry_fence','entry')]:req(v['evidence'][k]['operation_count']>=v['snapshot'][h]['last_sequence'])
        elif kind=='SnapshotInput':
            v['event']=validate('Event',v['event'],p);req(type(v['participants'])is list and len(v['participants'])==2);v['participants']=[validate('ParticipantSnapshot',x,p) for x in v['participants']];req([x['participant_id'] for x in v['participants']]==gyms(p));req(all(x['entry']['event']['event_id']==v['event']['source_event_id'] and x['roster']['cutoff_ms']==v['event']['cutoff_ms'] for x in v['participants']))
        elif kind=='SnapshotReceipt':
            v['input']=validate('SnapshotInput',v['input'],p);req(v['snapshot_digest']==sha(snap_pre(v['input'])));req(type(v['captures'])is list and len(v['captures'])==2);v['captures']=[validate('CaptureEvidence',x,p) for x in v['captures']];req([x['participant_id'] for x in v['captures']]==gyms(p));integer(v['completed_at_ms'],max(x['captured_at_ms'] for x in v['captures']),DATE_MAX)
            for s,e in zip(v['input']['participants'],v['captures']):validate('ParticipantCapture',dict(version=1,snapshot=s,evidence=e),p)
        elif kind=='EventStatus':
            v['event']=validate('Event',v['event'],p);C=v['event']['cutoff_ms'];req(type(v['exists'])is bool and v['seed_authority']=='unavailable');integer(v['coordinator_created_at_ms'],1,DATE_MAX);integer(v['observed_at_ms'],v['coordinator_created_at_ms'],DATE_MAX);integer(v['created_at_ms'],0,DATE_MAX);integer(v['completed_at_ms'],0,DATE_MAX);hexid(v['snapshot_digest'],True)
            req(type(v['captured_participants'])is list and v['captured_participants']==[g for g in gyms(p) if g in v['captured_participants']]);req(len(set(v['captured_participants']))==len(v['captured_participants']))
            if not v['exists']:req(v['created_at_ms']==v['completed_at_ms']==0 and not v['captured_participants'] and not v['snapshot_digest']);req(v['phase']==('before_cutoff' if v['observed_at_ms']<C else 'awaiting_capture'))
            else:
                req(v['coordinator_created_at_ms']<C<=v['created_at_ms']<=v['observed_at_ms']);n=len(v['captured_participants']);req(v['phase']==('awaiting_capture','capturing','snapshotted')[n]);req(bool(v['snapshot_digest'])==(n==2));req(0<v['completed_at_ms']<=v['observed_at_ms'] and v['completed_at_ms']>=v['created_at_ms'] if n==2 else v['completed_at_ms']==0)
        elif kind=='CaptureResult':
            v['status']=validate('EventStatus',v['status'],p);v['capture']=validate('ParticipantCapture',v['capture'],p);req(type(v['replayed'])is bool);req(v['capture']['snapshot']['participant_id']in v['status']['captured_participants']);req(v['capture']['snapshot']['entry']['event']['event_id']==v['status']['event']['source_event_id']);req(v['status']['created_at_ms']<=v['capture']['evidence']['captured_at_ms']<=v['status']['observed_at_ms'])
        elif kind=='HistoryPage':
            hexid(v['after_event_id'],True);hexid(v['next_event_id'],True);integer(v['observed_at_ms'],1,DATE_MAX);req(type(v['has_more'])is bool and type(v['events'])is list and len(v['events'])<=8);v['events']=[validate('EventStatus',x,p) for x in v['events']];req(all(x['exists'] and x['observed_at_ms']==v['observed_at_ms'] for x in v['events']));keys=[(x['event']['cutoff_ms'],x['event']['event_id'])for x in v['events']];req(keys==sorted(set(keys)));req(v['next_event_id']==(keys[-1][1] if v['has_more'] and keys else ''));req(not v['has_more']or bool(keys));req(v['after_event_id']not in [x[1]for x in keys])
        elif kind=='EventRow':
            v['event']=validate('Event',v['event'],p);integer(v['created_at_ms'],v['event']['cutoff_ms'],DATE_MAX);req(v['phase']in('awaiting_capture','capturing','snapshotted'));hexid(v['snapshot_digest'],True);req(bool(v['snapshot_digest'])==(v['phase']=='snapshotted'));integer(v['completed_at_ms'],0,DATE_MAX);req(v['completed_at_ms']>=v['created_at_ms'] if v['phase']=='snapshotted'else v['completed_at_ms']==0)
        else:fail()
    req(len(canon(v))<=CAPS[kind]);return v

def gyms(p):return [x['stable_pin']['identity']['gym_id']for x in p['participants']]
def val_fence(v,store,C):
    v=norm('Fence',v);integer(v['version'],1,1);req(v['store_id']==store and v['cutoff_ms']==C);integer(v['observed_at_ms'],C,DATE_MAX);integer(v['operation_count'],0,4096);return v

def val_evidence(v,p):
    ep=ep_for(p,v['participant_id']);C=v['stable_fence']['cutoff_ms'];integer(C,1,DATE_MAX);req(C in OLD.CUTOFFS);v['stable_fence']=val_fence(v['stable_fence'],ep['stable_store_id'],C);v['entry_fence']=val_fence(v['entry_fence'],OLD.store_id(ep),C);req(v['entry_fence']['observed_at_ms']>=v['stable_fence']['observed_at_ms']);integer(v['captured_at_ms'],max(v['stable_fence']['observed_at_ms'],v['entry_fence']['observed_at_ms']),DATE_MAX);return v

def val_part(v,p):
    ep=ep_for(p,v['participant_id']);sp=ep['stable_pin'];req(v['entry_store_id']==OLD.store_id(ep));v['owner']=ordered(v['owner'],dict(ownership_basis='candidate_assignment',account_authority_id=sp['account_authority_id'],account_id=sp['account_id'],gym_id=sp['identity']['gym_id']));req(v['owner']==dict(ownership_basis='candidate_assignment',account_authority_id=sp['account_authority_id'],account_id=sp['account_id'],gym_id=sp['identity']['gym_id']))
    h=norm('Entry',v['entry']);integer(h['version'],1,1);req(h['store_id']==OLD.store_id(ep));h['event']=OLD.validate('Event',h['event'],ep);C=h['event']['cutoff_ms'];req(h['entry_id']==OLD.entry_id(ep,h['event']));integer(h['committed_count'],0,4096);integer(h['last_sequence'],h['committed_count'],4096);hexid(h['history_digest'])
    if h['committed_count']==0:req(h['last_sequence']==0 and h['latest_receipt']is None and h['history_digest']==sha(entry_start(ep,h['event']['saturday'])))
    else:
        req(h['latest_receipt']is not None);h['latest_receipt']=OLD.validate('Receipt',h['latest_receipt'],ep);r=h['latest_receipt'];req(r['event']==h['event']and r['operation_sequence']==h['last_sequence']and r['revision']==h['committed_count'])
    v['entry']=h;r=norm('Roster',v['roster']);integer(r['version'],1,1);req(r['store_id']==ep['stable_store_id']and r['cutoff_ms']==C);integer(r['committed_count'],0,4096);integer(r['last_sequence'],r['committed_count'],4096);integer(r['revision'],0,r['committed_count']);hexid(r['history_digest']);req(type(r['slots'])is list and type(r['assigned_at_ms'])is list and len(r['slots'])==len(r['assigned_at_ms'])==10)
    for i,t in zip(r['slots'],r['assigned_at_ms']):hexid(i,True);integer(t,0,C-1);req(bool(i)==bool(t))
    ids=[x for x in r['slots']if x];req(len(set(ids))==len(ids));req(bool(r['committed_count'])==bool(r['last_sequence']))
    if not r['committed_count']:req(r['revision']==0 and not ids and r['history_digest']==sha(stable_start(ep,C)))
    v['roster']=r;req(type(v['members'])is list and len(v['members'])<=5);members=[]
    oldmembers=[] if h['latest_receipt']is None else h['latest_receipt']['members']
    req(len(v['members'])==len(oldmembers))
    for x,om in zip(v['members'],oldmembers):
        x=norm('Member',x);integer(x['slot_assigned_at_ms'],0,C-1);a,m=assignment(x['assignment'],ep,C);req(a==om['assignment']and a['accepted_at_ms']<=h['latest_receipt']['committed_at_ms']);x['assignment']=a;slot=r['slots'].index(a['prospect_id'])+1 if a['prospect_id']in r['slots']else 0;req(x['slot']==slot and type(x['slot'])is int and x['slot_assigned_at_ms']==(r['assigned_at_ms'][slot-1]if slot else 0));req(not slot or a['accepted_at_ms']<=x['slot_assigned_at_ms']);req(x['class']==m['physical']['weight_class']);expected=OLD.condition(a,m,C);x['condition_at_cutoff']=ordered(x['condition_at_cutoff'],expected);req(x['condition_at_cutoff']==expected);req(x['eligibility']==('retired'if expected['phase']=='retired'else 'not_rostered'if not slot else 'eligible'));members.append(x)
    req([x['assignment']['prospect_id']for x in members]==sorted(set(x['assignment']['prospect_id']for x in members)));v['members']=members;return v

# Contract-owned old operation factories. These create modeled journals only.
def stable_op(seq,view,slot,prospect,slots,revision,prepared,committed=None,state='committed'):
    q=dict(expected_revision=revision,slot=slot,prospect_id=prospect);oid=label('stable op '+view['view_id']+' '+str(seq));i=dict(version=1,sequence=seq,operation_id=oid,generation=view['generation'],view_id=view['view_id'],request=q,prepared_at_ms=prepared);r=None
    if state=='committed':
        before=slots[slot-1];changed=before!=prospect;new=clone(slots);new[slot-1]=prospect
        r=dict(version=1,operation_id=oid,generation=view['generation'],view_id=view['view_id'],request=clone(q),previous_prospect_id=before,changed=changed,previous_revision=revision,revision=revision+int(changed),slots=new,committed_at_ms=committed)
    return dict(version=1,intent=i,state=state,receipt=r,abandoned_at_ms=prepared+1 if state=='abandoned'else 0)

def stable_replay(ep,source,rows,C):
    req(len(rows)<=4096,'event_corrupt');slots=['']*10;times=[0]*10;hist_slots=clone(slots);hist_times=clone(times);rev=0;hrev=0;count=0;last=0;h=sha(stable_start(ep,C));folds=[];lasttime=0;opids=set()
    for seq,o in enumerate(rows,1):
        o=norm('StableOperation',o);integer(o['version'],1,1);i=norm('StableIntent',o['intent']);integer(i['version'],1,1);integer(i['sequence'],1,4096);req(i['sequence']==seq and i['operation_id']not in opids,'event_corrupt');opids.add(i['operation_id']);hexid(i['operation_id']);hexid(i['view_id']);integer(i['generation'],1);integer(i['prepared_at_ms'],lasttime,DATE_MAX);q=ordered(i['request'],dict(expected_revision=0,slot=1,prospect_id=''));integer(q['expected_revision'],0,4096);integer(q['slot'],1,10);hexid(q['prospect_id'],True);req(q['expected_revision']==rev,'event_corrupt');req(o['state']in('prepared','abandoned','committed'),'event_corrupt')
        if o['state']!='committed':
            req(o['receipt']is None,'event_corrupt');integer(o['abandoned_at_ms'],0,DATE_MAX);req(o['abandoned_at_ms']>=i['prepared_at_ms']if o['state']=='abandoned'else o['abandoned_at_ms']==0,'event_corrupt');lasttime=max(i['prepared_at_ms'],o['abandoned_at_ms']);req(o['state']!='prepared'or seq==len(rows),'event_corrupt');continue
        r=norm('StableReceipt',o['receipt']);integer(r['version'],1,1);req(o['abandoned_at_ms']==0,'event_corrupt');req(r['version']==1 and all(r[k]==i[k]for k in ('operation_id','generation','view_id','request')),'event_corrupt');integer(r['committed_at_ms'],i['prepared_at_ms'],DATE_MAX);req(q['expected_revision']==rev and r['previous_revision']==rev,'event_corrupt');before=slots[q['slot']-1];changed=before!=q['prospect_id'];req(type(r['changed'])is bool and r['changed']==changed and r['previous_prospect_id']==before and r['revision']==rev+int(changed),'event_corrupt');new=clone(slots);new[q['slot']-1]=q['prospect_id'];req(r['slots']==new and len(set(x for x in new if x))==len([x for x in new if x]),'event_corrupt');req(not q['prospect_id']or q['prospect_id']in source,'event_corrupt')
        if q['prospect_id'] and r['committed_at_ms']<C:req(0<source[q['prospect_id']]['assignment']['born_at_ms']<=source[q['prospect_id']]['assignment']['accepted_at_ms']<=r['committed_at_ms'],'event_corrupt')
        slots=new;rev=r['revision'];lasttime=r['committed_at_ms']
        if changed:times[q['slot']-1]=r['committed_at_ms']if q['prospect_id']else 0
        if r['committed_at_ms']<C:
            hrev=rev;hist_slots=clone(slots);hist_times=clone(times);count+=1;last=seq;b=fold_row('stable',h,seq,o);h=sha(b);folds.append(dict(sequence=seq,canonical_operation=canon(o).decode(),preimage=b.hex(),digest=h))
    return dict(version=1,store_id=ep['stable_store_id'],cutoff_ms=C,committed_count=count,last_sequence=last,history_digest=h,revision=hrev,slots=hist_slots,assigned_at_ms=hist_times),folds

def entry_op(ep,source,view,s,seq,previous,rosterrev,slots,prepared,committed,ids=None,state='committed'):
    ids=sorted(source)if ids is None else sorted(ids);action='replace'if ids else'withdraw';q=OLD.request(ep,s,ids,previous,rosterrev,action);oid=label('entry op '+view['view_id']+' '+str(seq));i=dict(version=1,sequence=seq,operation_id=oid,generation=view['generation'],view_id=view['view_id'],request=q,prepared_at_ms=prepared);r=None
    if state=='committed':r=dict(version=1,store_id=OLD.store_id(ep),entry_id=OLD.entry_id(ep,OLD.event(ep,s)),event=OLD.event(ep,s),operation_id=oid,operation_sequence=seq,generation=view['generation'],view_id=view['view_id'],action=action,previous_revision=previous,revision=previous+1,roster_revision=rosterrev if ids else 0,stable_checkpoint=len(source),observed_source_count=len(source),committed_at_ms=committed,members=[OLD.member(source[k]['assignment'],source[k]['manifest'],slots.index(k)+1,committed)for k in ids])
    o=dict(version=1,intent=i,state=state,receipt=r,abandoned_at_ms=prepared+1 if state=='abandoned'else 0);return OLD.validate('Operation',o,ep)

def entry_replay(ep,rows,s):
    C=OLD.cutoff(s);req(len(rows)<=4096,'event_corrupt');h=sha(entry_start(ep,s));count=last=0;latest=None;folds=[];revs={};lasttime=0;ops=set();lastroster=0;lastcheckpoint=0;lastcount=0
    for seq,row in enumerate(rows,1):
        o=OLD.validate('Operation',clone(row),ep);i=o['intent'];req(i['sequence']==seq and i['operation_id']not in ops,'event_corrupt');ops.add(i['operation_id']);req(i['prepared_at_ms']>=lasttime,'event_corrupt');req(i['request']['expected_revision']==revs.get(i['request']['event_id'],0),'event_corrupt');lasttime=max(i['prepared_at_ms'],o['abandoned_at_ms']);req(o['state']!='prepared'or seq==len(rows),'event_corrupt')
        if o['state']!='committed':continue
        r=o['receipt'];req(r['stable_checkpoint']>=lastcheckpoint and r['observed_source_count']>=lastcount,'event_corrupt');lastcheckpoint=r['stable_checkpoint'];lastcount=r['observed_source_count'];eid=r['event']['event_id'];req(r['previous_revision']==revs.get(eid,0),'event_corrupt');revs[eid]=r['revision'];lasttime=r['committed_at_ms']
        if r['action']=='replace':req(r['roster_revision']>=lastroster,'event_corrupt');lastroster=r['roster_revision']
        if r['event']['saturday']==s and r['committed_at_ms']<C:
            count+=1;last=seq;latest=clone(r);b=fold_row('entry',h,seq,o);h=sha(b);folds.append(dict(sequence=seq,canonical_operation=canon(o).decode(),preimage=b.hex(),digest=h))
    return dict(version=1,store_id=OLD.store_id(ep),event=OLD.event(ep,s),entry_id=OLD.entry_id(ep,OLD.event(ep,s)),committed_count=count,last_sequence=last,history_digest=h,latest_receipt=latest),folds

def history_fixture(ep,s='2026-11-21',mode='ordinary',synthetic=False):
    source=OLD.fake_five(ep)if synthetic else OLD.fixture_source(fixture_name(ep));C=OLD.cutoff(s);view=view_for(ep);slots=['']*10;rows=[];rev=0
    if mode.startswith('retire_'):
        key=sorted(source)[0];x=source[key];d=x['manifest']['dna'];weeks=d['prime_weeks']+d['decline_weeks'];shift=0 if mode in('retire_exact','retire_unrostered')else 1
        x['assignment']['born_at_ms']=C-weeks*OLD.WEEK+shift;x['assignment']['accepted_at_ms']=x['assignment']['born_at_ms']+1
    for idx,k in enumerate(source):
        o=stable_op(len(rows)+1,view,idx+1,k,slots,rev,C-20000+idx*2,C-19999+idx*2);rows.append(o);slots=clone(o['receipt']['slots']);rev=o['receipt']['revision']
    if mode=='empty':rows=[];slots=['']*10;rev=0
    savedslots=clone(slots);savedrev=rev;entryrows=[]if mode=='empty'else [entry_op(ep,source,view,s,1,0,savedrev,savedslots,C-15000,C-14999)]
    if mode in('noop','equal_millisecond','moved','unrostered','retire_unrostered','post_cutoff','at_cutoff','delayed_write','abandoned','prepared','other_event','withdraw','none'):
        k=sorted(source)[0]
        if mode=='noop':rows.append(stable_op(len(rows)+1,view,1,slots[0],slots,rev,C-10000,C-9999))
        elif mode=='equal_millisecond':
            for target in ('',k):
                o=stable_op(len(rows)+1,view,1,target,slots,rev,C-9999,C-9999);rows.append(o);slots=clone(o['receipt']['slots']);rev=o['receipt']['revision']
        elif mode=='moved':
            for slot,target in ((1,''),(10,k)):
                o=stable_op(len(rows)+1,view,slot,target,slots,rev,C-10000+len(rows),C-9999+len(rows));rows.append(o);slots=clone(o['receipt']['slots']);rev=o['receipt']['revision']
        elif mode in('unrostered','retire_unrostered','post_cutoff','at_cutoff','delayed_write'):
            t={'post_cutoff':C+1,'at_cutoff':C,'delayed_write':C-1}.get(mode,C-9999);rows.append(stable_op(len(rows)+1,view,1,'',slots,rev,t-1,t))
        elif mode in('abandoned','prepared'):rows.append(stable_op(len(rows)+1,view,1,'',slots,rev,C-9999,state=mode))
        elif mode=='other_event':entryrows.append(entry_op(ep,source,view,'2026-11-28',2,0,rev,slots,C-10000,C-9999))
        elif mode=='withdraw':entryrows.append(entry_op(ep,source,view,s,2,1,rev,slots,C-10000,C-9999,ids=[]))
        elif mode=='none':entryrows=[]
    roster,sf=stable_replay(ep,source,rows,C);entry,ef=entry_replay(ep,entryrows,s)
    members=[]
    for saved in entry['latest_receipt']['members']if entry['latest_receipt']else []:
        a=saved['assignment'];m=source[a['prospect_id']]['manifest'];slot=roster['slots'].index(a['prospect_id'])+1 if a['prospect_id']in roster['slots']else 0;co=OLD.condition(a,m,C)
        members.append(dict(slot=slot,slot_assigned_at_ms=roster['assigned_at_ms'][slot-1]if slot else 0,assignment=clone(a),**{'class':m['physical']['weight_class']},condition_at_cutoff=co,eligibility='retired'if co['phase']=='retired'else'not_rostered'if not slot else'eligible'))
    sp=ep['stable_pin'];part=dict(version=1,participant_id=sp['identity']['gym_id'],entry_store_id=OLD.store_id(ep),owner=dict(ownership_basis='candidate_assignment',account_authority_id=sp['account_authority_id'],account_id=sp['account_id'],gym_id=sp['identity']['gym_id']),roster=roster,entry=entry,members=members)
    part=validate('ParticipantSnapshot',part)
    return dict(name=fixture_name(ep)+'/'+mode+('/synthetic-five'if synthetic else''),fixture=fixture_name(ep),source_kind='synthetic_structural_assignments_no_admission_proof'if synthetic else'genuine_immutable_receipt_inputs_modeled_journal',saturday=s,source=source,stable_operations=rows,entry_operations=entryrows,stable_history_start_preimage=stable_start(ep,C).hex(),entry_history_start_preimage=entry_start(ep,s).hex(),stable_fold=sf,entry_fold=ef,expected_snapshot=part,canonical_snapshot=canon(part).decode(),expected_proof_count=len(members))

def request(command,view='',**kw):
    q=dict(id=1,command=command,view_id=view,saturday='',after_event_id='',limit=0,job_id='');q.update(kw);return q

def val_view(v,p):
    req(type(v)is dict);matches=[ep for ep in p['participants']if ep['stable_pin']['account_id']==v.get('account_id')];req(len(matches)==1);ep=matches[0];base=view_for(ep);v=ordered(v,base)
    for k in base:
        if k not in ('view_id','generation'):req(v[k]==base[k])
    hexid(v['view_id']);integer(v['generation'],1);return v,ep

def snapshot(view=None,state='ready',created=1,p=PIN,**kw):
    if state=='unavailable':v=dict(version=1,state=state,error='event_unavailable',view=None,store_id='',pin=None,coordinator_created_at_ms=0,participant_id='',active_job_id='',queued_job_id='',last_job_id='')
    else:
        vv,ep=val_view(view,p);v=dict(version=1,state=state,error='event_setup_incomplete'if state=='setup_incomplete'else'',view=vv,store_id=''if state=='setup_incomplete'else sid(p),pin=None if state=='setup_incomplete'else clone(p),coordinator_created_at_ms=0 if state=='setup_incomplete'else created,participant_id=ep['stable_pin']['identity']['gym_id'],active_job_id='',queued_job_id='',last_job_id='')
    v.update(kw);return v

def response(q,view,field,value,p=PIN):
    r=dict(id=q['id'],ok=True,view=clone(view),store_id=sid(p),snapshot=None,status=None,record=None,history=None,job=None,error='');r[field]=clone(value)
    if field=='snapshot':r['view']=clone(value['view']);r['store_id']=value['store_id']
    return r

def validate_wire(kind,value,p):
    v=norm(kind,value)
    if kind=='EventRequest':
        integer(v['id'],1);cmd=v['command'];allowed={'event_snapshot':set(),'event_status':{'view_id','saturday'},'event_record':{'view_id','saturday'},'event_capture':{'view_id','saturday'},'event_history':{'view_id','after_event_id','limit'},'event_job':{'view_id','job_id'},'event_cancel':{'view_id','job_id'}};req(cmd in allowed);blank=request(cmd)
        for k in blank:
            if k not in ('id','command')and k not in allowed[cmd]:req(v[k]==blank[k]and type(v[k])is type(blank[k]))
        if cmd!='event_snapshot':hexid(v['view_id'])
        if 'saturday'in allowed[cmd]:OLD.date_value(v['saturday'])
        if 'job_id'in allowed[cmd]:hexid(v['job_id'])
        if cmd=='event_history':hexid(v['after_event_id'],True);integer(v['limit'],1,8)
    elif kind=='EventSnapshot':
        integer(v['version'],1,1);req(v['state']in('unavailable','setup_incomplete','ready','busy','failed'))
        if v['state']=='unavailable':req(v==snapshot(state='unavailable'));return v
        v['view'],ep=val_view(v['view'],p);req(v['participant_id']==ep['stable_pin']['identity']['gym_id'])
        if v['state']=='setup_incomplete':req(v==snapshot(v['view'],'setup_incomplete',p=p));return v
        v['pin']=val_pin(v['pin']);req(v['pin']==p and v['store_id']==sid(p));integer(v['coordinator_created_at_ms'],1,DATE_MAX)
        for k in ('active_job_id','queued_job_id','last_job_id'):hexid(v[k],True)
        req(not v['active_job_id']or v['active_job_id']!=v['queued_job_id']);req(v['error']==''if v['state']in('ready','busy')else v['error']in ERRORS and v['error']!='outcome_unknown')
    elif kind=='EventJob':
        integer(v['version'],1,1);hexid(v['job_id']);v['view'],ep=val_view(v['view'],p);req(v['store_id']==sid(p)and v['participant_id']==ep['stable_pin']['identity']['gym_id']);req(v['kind']=='capture'and v['state']in('queued','running','completed','failed','cancelled'));req(v['event_id']==event(p,v['saturday'])['event_id'])
        if v['state']=='completed':
            req(v['error']=='');v['result']=validate('CaptureResult',v['result'],p);req(v['result']['status']['event']['event_id']==v['event_id']and v['result']['capture']['snapshot']['participant_id']==v['participant_id'])
        else:req(v['result']is None);req(v['error']==''if v['state']in('queued','running')else v['error']=='event_cancelled'if v['state']=='cancelled'else v['error']in ERRORS)
    elif kind=='EventResponse':
        integer(v['id'],1);req(type(v['ok'])is bool);names=('snapshot','status','record','history','job')
        if not v['ok']:req(v['view']is None and v['store_id']==''and all(v[k]is None for k in names)and v['error']in ERRORS);return v
        req(v['error']==''and sum(v[k]is not None for k in names)==1)
        for k,t in [('snapshot','EventSnapshot'),('status','EventStatus'),('record','SnapshotReceipt'),('history','HistoryPage'),('job','EventJob')]:
            if v[k]is not None:v[k]=validate(t,v[k],p)
        if v['snapshot']is not None:
            req(v['view']==v['snapshot']['view']and v['store_id']==v['snapshot']['store_id']);v['view']=clone(v['snapshot']['view'])
        else:
            v['view'],ep=val_view(v['view'],p);req(v['store_id']==sid(p))
            if v['job']is not None:req(v['job']['view']==v['view'])
    return v

def correlate(q,r,p,view,expected=None,terminal=None):
    q=validate('EventRequest',q,p);r=validate('EventResponse',r,p);req(q['id']==r['id'])
    if not r['ok']:return r
    field={'event_snapshot':'snapshot','event_status':'status','event_record':'record','event_history':'history','event_capture':'job','event_job':'job','event_cancel':'job'}[q['command']];req(r[field]is not None)
    if field=='snapshot':
        req(r['view']==view or r['snapshot']['state']=='unavailable');return r
    req(r['view']==view and q['view_id']==view['view_id']and r['store_id']==sid(p))
    if field=='status':req(r[field]['event']['saturday']==q['saturday'])
    elif field=='record':req(r[field]['input']['event']['saturday']==q['saturday'])
    elif field=='history':req(r[field]['after_event_id']==q['after_event_id']and len(r[field]['events'])<=q['limit'])
    elif field=='job':
        j=r[field]
        if q['command']=='event_capture':req(j['state']=='queued'and j['saturday']==q['saturday'])
        else:req(j['job_id']==q['job_id'])
        if expected:
            for k in ('view','store_id','job_id','event_id','saturday','participant_id'):req(j[k]==expected[k])
        if terminal:req(sha(canon(j))==terminal)
    return r

def depth(v):return 1+max([depth(x)for x in (v.values()if type(v)is dict else v)]+[0])if type(v)in(dict,list)else 0

def parse(b,kind,p=PIN):
    req(type(b)is bytes and 0<len(b)<=CAPS[kind]);req(not b.startswith(b'\xef\xbb\xbf'))
    try:s=b.decode('utf8','strict')
    except UnicodeError:fail()
    def pairs(ps):
        d={}
        for k,v in ps:req(k not in d and len(k.encode('utf8'))<=64);d[k]=v
        return d
    def num(t):req(bool(re.fullmatch('0|[1-9][0-9]*',t)));n=int(t);integer(n);return n
    try:v=json.loads(s,object_pairs_hook=pairs,parse_int=num,parse_float=lambda _:fail(),parse_constant=lambda _:fail())
    except (ValueError,RecursionError):fail()
    def bound(x,path=()):
        if type(x)is dict:
            req(len(x)<=64)
            for k,y in x.items():bound(y,path+(k,))
        elif type(x)is list:
            limits={'participants':2,'captures':2,'slots':10,'assigned_at_ms':10,'members':5,'captured_participants':2,'events':8}
            req(path and path[-1]in limits and len(x)<=limits[path[-1]])
            for y in x:bound(y,path+('*',))
        elif type(x)is bool:req(path and path[-1]in('exists','has_more','replayed','ok'))
        elif type(x)is str:req(not any(0xD800<=ord(c)<=0xDFFF for c in x))
    req(depth(v)<=12);bound(v)
    try:return validate(kind,v,p)
    except OLD.Refusal:fail()

class Coordinator:
    """Finite semantic state model; not a LevelDB/crash/authority implementation."""
    def __init__(self,p,created,histories):
        self.pin=clone(p);self.created=created;self.clock=created;self.rows={};self.histories=clone(histories);self.poison=False;self.proofs=0;self.fences={g:dict(stable=created,entry=created)for g in gyms(p)};self.source_poison=set()
    def status(self,s):
        e=event(self.pin,s);row=self.rows.get(s);caps=row['captures']if row else{};receipt=row['receipt']if row else None
        return validate('EventStatus',dict(version=1,store_id=sid(self.pin),event=e,exists=bool(row),phase=('before_cutoff'if self.clock<e['cutoff_ms']else'awaiting_capture')if not row else('awaiting_capture','capturing','snapshotted')[len(caps)],captured_participants=[g for g in gyms(self.pin)if g in caps],snapshot_digest=receipt['snapshot_digest']if receipt else'',coordinator_created_at_ms=self.created,created_at_ms=row['created']if row else 0,completed_at_ms=receipt['completed_at_ms']if receipt else 0,observed_at_ms=self.clock,seed_authority='unavailable'),self.pin)
    def summary(self):return dict(clock_ms=self.clock,event_count=len(self.rows),poisoned=self.poison,source_poisoned=sorted(self.source_poison),source_fences=clone(self.fences),source_fences_scope='successful final fence observations only; begin and metadata observations are not modeled as complete source clock metadata',events=[dict(saturday=s,status=self.status(s),participant_capture_sha256={g:sha(canon(c))for g,c in row['captures'].items()},receipt_digest=sha(receipt_pre(row['receipt']))if row['receipt']else'')for s,row in sorted(self.rows.items())])
    def call(self,q):
        self.proofs=0;m=q['method']
        if m=='reopen':self.poison=False;self.source_poison=set();return self.summary()
        req(q.get('normal_profile',True),'event_unavailable');req(not self.poison,'outcome_unknown');req(q.get('authorized',True),'unauthorized');req(q.get('session_type','login')=='login','forbidden');req(q.get('current_view',True),'stale_view');g=q.get('participant_id',gyms(self.pin)[0]);ep=ep_for(self.pin,g);req(q.get('session_participant_id',g)==g,'unauthorized');req(q.get('matching_source_pin',True),'event_conflict');now=q['now_ms'];req(type(now)is int and self.clock<=now<=DATE_MAX,'event_clock');s=q.get('saturday','2026-11-21');e=event(self.pin,s);C=e['cutoff_ms'];self.clock=now
        if m=='observe':return self.status(s)
        if m=='read':req(s in self.rows,'event_not_found');req(self.rows[s]['receipt']is not None,'event_incomplete');return clone(self.rows[s]['receipt'])
        if m=='history':
            integer(q['limit'],1,8);ordered_rows=sorted(self.rows,key=lambda d:(OLD.cutoff(d),event(self.pin,d)['event_id']));after=q.get('after_event_id','');offset=0
            if after:
                keys=[event(self.pin,d)['event_id']for d in ordered_rows];req(after in keys,'event_not_found');offset=keys.index(after)+1
            dates=ordered_rows[offset:offset+q['limit']];more=offset+len(dates)<len(ordered_rows);return validate('HistoryPage',dict(version=1,store_id=sid(self.pin),after_event_id=after,next_event_id=event(self.pin,dates[-1])['event_id']if more else'',has_more=more,observed_at_ms=self.clock,events=[self.status(d)for d in dates]),self.pin)
        req(m=='capture');req(g not in self.source_poison,'event_unavailable')
        if s in self.rows and g in self.rows[s]['captures']:return dict(version=1,status=self.status(s),capture=clone(self.rows[s]['captures'][g]),replayed=True)
        req(self.created<C,'event_created_too_late');req(now>=C,'event_not_due')
        if s not in self.rows:
            req(len(self.rows)<256,'event_capacity');self.rows[s]=dict(created=now,captures={},receipt=None)
            if q.get('fault')=='reservation_unknown':self.poison=True;fail('outcome_unknown')
        hist=self.histories[g];req(hist['saturday']==s,'event_corrupt');req(not q.get('source_missing',False),'event_unavailable');req(not q.get('history_corrupt',False),'event_corrupt')
        for idx,_ in enumerate(hist['expected_snapshot']['members']):
            req(q.get('cancel_before_proof')!=idx,'event_cancelled');req(q.get('revoke_before_proof')!=idx,'stale_view');self.proofs+=1;req(q.get('source_corrupt_at')!=idx,'event_corrupt')
        req(not q.get('cancel_before_lease',False),'event_cancelled');st=q.get('stable_now_ms',now);et=q.get('entry_now_ms',st);ct=q.get('final_now_ms',et);f=self.fences[g]
        req(st>=max(C,f['stable']),'event_clock');f['stable']=st
        if q.get('fault')=='stable_fence_unknown':self.source_poison.add(g);fail('outcome_unknown')
        req(et>=max(st,f['entry']),'event_clock');f['entry']=et
        if q.get('fault')=='entry_fence_unknown':self.source_poison.add(g);fail('outcome_unknown')
        req(ct>=max(et,self.clock),'event_clock');self.clock=ct;req(not q.get('cancel_before_commit',False),'event_cancelled')
        ev=dict(version=1,participant_id=g,stable_fence=dict(version=1,store_id=ep['stable_store_id'],cutoff_ms=C,observed_at_ms=st,operation_count=len(hist['stable_operations'])),entry_fence=dict(version=1,store_id=OLD.store_id(ep),cutoff_ms=C,observed_at_ms=et,operation_count=len(hist['entry_operations'])),captured_at_ms=ct)
        cap=validate('ParticipantCapture',dict(version=1,snapshot=clone(hist['expected_snapshot']),evidence=ev),self.pin);row=self.rows[s];row['captures'][g]=cap
        if len(row['captures'])==2:
            inp=dict(version=1,event=e,participants=[clone(row['captures'][x]['snapshot'])for x in gyms(self.pin)]);row['receipt']=validate('SnapshotReceipt',dict(version=1,input=inp,snapshot_digest=sha(snap_pre(inp)),captures=[clone(row['captures'][x]['evidence'])for x in gyms(self.pin)],completed_at_ms=ct),self.pin)
        if q.get('fault')=='event_unknown':self.poison=True;fail('outcome_unknown')
        if q.get('cancel_after_commit')or q.get('outer_lease_error'):fail('outcome_unknown')
        return validate('CaptureResult',dict(version=1,status=self.status(s),capture=cap,replayed=False),self.pin)

def run_steps(name,model,steps):
    results=[]
    for q in steps:
        try:value=model.call(q);error=''
        except (Refusal,OLD.Refusal)as e:value=None;error=str(e)
        ceiling=len(model.histories[q.get('participant_id',gyms(model.pin)[0])]['expected_snapshot']['members'])if q['method']=='capture'else 0
        bounds=[model.proofs,model.proofs]if not error or q['method']!='capture'else[0,ceiling]
        results.append(dict(request=q,result=value,error=error,expected_proof_count=model.proofs,proof_count_scope='reference_model_accounting',runtime_proof_bounds=bounds,state=model.summary()))
    return dict(name=name,evidence_kind='semantic_model_not_filesystem_execution',created_at_ms=model.created,steps=results)

def reverse_objects(x):
    if type(x)is dict:return {k:reverse_objects(x[k])for k in reversed(x)}
    if type(x)is list:return [reverse_objects(y)for y in x]
    return x

def codec_cases(kind,v,name):
    canonical=canon(validate(kind,clone(v))).decode();base=canonical.encode();out=[]
    def add(label,b):
        try:r=parse(b,kind);accepted=True;c=canon(r).decode()
        except (Refusal,OLD.Refusal,ValueError,TypeError,KeyError):accepted=False;c=None
        out.append(dict(name=name+'/'+label,kind=kind,input_hex=b.hex(),accepted=accepted,canonical=c))
    add('canonical',base);add('ordered_whitespace',b' \n'+json.dumps(reverse_objects(v),indent=1).encode()+b'\t ')
    assert out[-1]['accepted']and out[-1]['canonical']==canonical,(kind,name,'canonical order')
    key=next(iter(v));escaped='\\u%04x'%ord(key[0])+key[1:];add('escaped_key',base.replace(('"'+key+'"').encode(),('"'+escaped+'"').encode(),1))
    add('duplicate_decoded',b'{"'+escaped.encode()+b'":'+canon(v[key])+b','+base[1:]);add('unknown_key',base[:-1]+b',"unknown":0}');add('missing_key',canon({k:x for k,x in v.items()if k!=key}));add('trailing',base+b'{}');add('bom',b'\xef\xbb\xbf'+base);add('invalid_utf8',base[:-1]+b'\xff}');add('float',base.replace(b':1',b':1.0',1));add('exponent',base.replace(b':1',b':1e0',1));add('negative_zero',base.replace(b':1',b':-0',1));add('wrong_container',b'[]');add('unknown_array',base[:-1]+b',"array":[]}');add('oversized',b' '*(CAPS[kind]+1));add('exact_transport_cap',base+b' '*(CAPS[kind]-len(base)));return out

def mutate_path(x,path,value):
    x=clone(x);cur=x
    for k in path[:-1]:cur=cur[k]
    cur[path[-1]]=value;return x

def init_cases():
    cases=[];C=OLD.cutoff('2026-11-21')
    for stage in ('absent','empty','marker_no_db','marker_empty_db','db_empty','init_only','initialized_empty','initialized_nonempty','mixed_init_meta','wrong_marker','missing_marker','symlink','unknown_row','partial_meta'):
        for action in ('create','open','resume_prepared'):
            ok=(action=='create'and stage in('absent','empty'))or(action=='open'and stage in('initialized_empty','initialized_nonempty'))or(action=='resume_prepared'and stage in('marker_no_db','marker_empty_db','db_empty','init_only','initialized_empty'))
            created=C-1 if ok and stage.startswith('initialized')else C+1 if ok else None
            cases.append(dict(name=action+'/'+stage,method=action,state=stage,pin_durable_at_ms=C-1,post_pin_sample_ms=C+1,old_created_at_ms=C-1,accepted=ok,expected_created_at_ms=created,historical_capture_permitted=bool(ok and created<C),evidence_kind='modeled_initialization_not_filesystem_or_crash_execution'))
    return cases

def corpus():
    validate('Pin',PIN);s='2026-11-21';C=OLD.cutoff(s);cal=[]
    for ds in OLD.DATES:
        t=OLD.cutoff(ds);d=deadline(ds);assert t==OLD.tail_cutoff(OLD.date_value(ds));cal.append(dict(saturday=ds,cutoff_ms=t,incident_deadline_ms=d,deadline_saturday=(OLD.date_value(ds)+dt.timedelta(days=7)).isoformat(),hours_to_deadline=(d-t)//3600000,upcoming_before=OLD.upcoming(t-1),upcoming_at=OLD.upcoming(t)if ds!=OLD.DATES[-1]else None,event=event(PIN,ds)))
    assert {x['hours_to_deadline']for x in cal}=={167,168,169}
    invalid_dates=['','2025-12-27','2100-01-02','2026-11-20','2026-11-22','2026-2-07','2026-02-30','2026-11-21Z',' 2026-11-21','2026-11-21 ','2026-11-21\0','２０２６-11-21']
    calendar_invalid=[]
    for ds in invalid_dates:
        try:event(PIN,ds);raise AssertionError(ds)
        except (Refusal,OLD.Refusal):calendar_invalid.append(dict(saturday=ds,accepted=False))
    fixtures=[];histories=[]
    for ep in PIN['participants']:
        name=fixture_name(ep);fixtures.append(dict(fixture=name,entry_pin=ep,current_view=view_for(ep),participant_id=ep['stable_pin']['identity']['gym_id'],source_kind='unchanged_genuine_public_receipts_previously_normal_Argon_verified',source_records=[clone(x)for x in OLD.OLD['cases']if x['fixture']==name]))
        for mode in ('empty','ordinary','noop','equal_millisecond','moved','unrostered','post_cutoff','at_cutoff','delayed_write','abandoned','prepared','other_event','withdraw','none'):histories.append(history_fixture(ep,mode=mode))
        for mode in ('ordinary','retire_exact','retire_after','retire_unrostered'):histories.append(history_fixture(ep,mode=mode,synthetic=True))
    base={h['expected_snapshot']['participant_id']:h for h in histories if h['name'].endswith('/ordinary')};model=Coordinator(PIN,C-100000,base)
    g0,g1=gyms(PIN);r0=model.call(dict(method='capture',participant_id=g0,now_ms=C,stable_now_ms=C,entry_now_ms=C,final_now_ms=C));r1=model.call(dict(method='capture',participant_id=g1,now_ms=C+1,stable_now_ms=C+1,entry_now_ms=C+2,final_now_ms=C+3));record=model.call(dict(method='read',now_ms=C+4));history=model.call(dict(method='history',limit=8,now_ms=C+5))
    records=[dict(name='both_genuine_receipt_inputs_modeled_histories',value=record,canonical=canon(record).decode(),snapshot_preimage=snap_pre(record['input']).hex(),snapshot_digest=record['snapshot_digest'],receipt_preimage=receipt_pre(record).hex(),receipt_digest=sha(receipt_pre(record)))]
    for suffix in ('empty','noop','moved','unrostered','post_cutoff','at_cutoff','delayed_write','withdraw','none','ordinary/synthetic-five','retire_exact/synthetic-five','retire_after/synthetic-five','retire_unrostered/synthetic-five'):
        hs={h['expected_snapshot']['participant_id']:h for h in histories if h['name'].split('/',1)[1]==suffix};m=Coordinator(PIN,C-100000,hs)
        for g in reversed(gyms(PIN)):m.call(dict(method='capture',participant_id=g,now_ms=C+100,stable_now_ms=C+100,entry_now_ms=C+100,final_now_ms=C+100))
        rec=m.call(dict(method='read',now_ms=C+100));records.append(dict(name=suffix,value=rec,canonical=canon(rec).decode(),snapshot_preimage=snap_pre(rec['input']).hex(),snapshot_digest=rec['snapshot_digest'],receipt_preimage=receipt_pre(rec).hex(),receipt_digest=sha(receipt_pre(rec))))
    # Post-C or exact-C tails cannot enter SnapshotInput; late capture time only changes evidence digest.
    for n in ('post_cutoff','at_cutoff'):assert next(x for x in records if x['name']==n)['snapshot_digest']==record['snapshot_digest']
    assert next(x for x in records if x['name']=='noop')['snapshot_digest']!=record['snapshot_digest'] # no-op is deliberately retained history
    dtos=[];codecs=[];protocol=[];full_codec_kinds=set()
    def add_dto(name,kind,v,fixture=''):
        v=validate(kind,v);dtos.append(dict(name=name,kind=kind,fixture=fixture,value=v,canonical=canon(v).decode()));cc=codec_cases(kind,v,name);codecs.extend(cc if kind not in full_codec_kinds else cc[:3]);full_codec_kinds.add(kind);return v
    add_dto('coordinator','Pin',PIN);add_dto('coordinator','Info',dict(version=1,store_id=sid(PIN),pin=PIN,created_at_ms=C-100000,event_count=1));add_dto('coordinator','Event',event(PIN,s));add_dto('coordinator','SnapshotInput',record['input']);add_dto('coordinator','SnapshotReceipt',record);add_dto('coordinator','HistoryPage',history)
    add_dto('coordinator','Marker',dict(application='slug-candidate-events',version=1,store_id=sid(PIN),pin=PIN));add_dto('coordinator','Init',dict(version=1,store_id=sid(PIN),pin=PIN));add_dto('coordinator','Metadata',dict(version=1,store_id=sid(PIN),pin=PIN,created_at_ms=C-100000,clock_ms=C+5,event_count=1))
    for f in fixtures:
        ep=f['entry_pin'];name=f['fixture'];view=f['current_view'];g=f['participant_id'];result=r0 if g==g0 else r1;cap=result['capture'];add_dto(name,'ParticipantSnapshot',cap['snapshot'],name);add_dto(name,'CaptureEvidence',cap['evidence'],name);add_dto(name,'ParticipantCapture',cap,name);add_dto(name,'CaptureResult',result,name)
        for state in ('unavailable','setup_incomplete','ready','busy','failed'):
            ss=snapshot(view,state,C-100000,**({'error':'event_corrupt'}if state=='failed'else{}));add_dto(name+'/'+state,'EventSnapshot',ss,name);q=request('event_snapshot');rr=response(q,view,'snapshot',ss);add_dto(name+'/'+state,'EventResponse',rr,name);protocol.append(dict(name=name+'/snapshot/'+state,request=q,response=rr,fixture=name,current_view=view,expected_job=None,terminal_digest=None,accepted=True,canonical=canon(correlate(q,rr,PIN,view)).decode()))
        job=dict(version=1,job_id=label('capture job '+name),kind='capture',state='queued',view=view,store_id=sid(PIN),event_id=event(PIN,s)['event_id'],saturday=s,participant_id=g,error='',result=None);expected=clone(job)
        for st in ('queued','running','completed','failed','cancelled'):
            j=clone(job);j['state']=st;j['result']=clone(result)if st=='completed'else None;j['error']='outcome_unknown'if st=='failed'else'event_cancelled'if st=='cancelled'else'';add_dto(name+'/'+st,'EventJob',j,name)
            q=request('event_capture',view['view_id'],saturday=s)if st=='queued'else request('event_job',view['view_id'],job_id=j['job_id']);rr=response(q,view,'job',j);add_dto(name+'/'+st,'EventResponse',rr,name);protocol.append(dict(name=name+'/job/'+st,request=q,response=rr,fixture=name,current_view=view,expected_job=expected,terminal_digest=sha(canon(j))if st in('completed','failed','cancelled')else None,accepted=True,canonical=canon(correlate(q,rr,PIN,view,expected,sha(canon(j))if st in('completed','failed','cancelled')else None)).decode()))
        for cmd,field,v in [('event_status','status',model.status(s)),('event_record','record',record),('event_history','history',history)]:
            q=request(cmd,view['view_id'],**({'limit':8}if cmd=='event_history'else{'saturday':s}));rr=response(q,view,field,v);add_dto(name+'/'+cmd,'EventRequest',q,name);add_dto(name+'/'+cmd,'EventResponse',rr,name);protocol.append(dict(name=name+'/'+cmd,request=q,response=rr,fixture=name,current_view=view,expected_job=None,terminal_digest=None,accepted=True,canonical=canon(correlate(q,rr,PIN,view)).decode()))
        for state,now in [('before_cutoff',C-1),('awaiting_capture',C),('deadline_reached',deadline(s))]:
            empty=Coordinator(PIN,C-100000,base);stat=empty.call(dict(method='observe',now_ms=now));add_dto(name+'/'+state,'EventStatus',stat,name)
    # Eight-row History metadata is legal; it never embeds eight full records.
    hist_model=Coordinator(PIN,C-100000,base);dates=OLD.DATES[OLD.DATES.index(s):OLD.DATES.index(s)+9];hist_model.clock=OLD.cutoff(dates[-1])
    for ds in dates:hist_model.rows[ds]=dict(created=OLD.cutoff(ds),captures={},receipt=None)
    maxpage=hist_model.call(dict(method='history',now_ms=hist_model.clock,limit=8));add_dto('maximum_eight_metadata','HistoryPage',maxpage)
    q=request('event_history',fixtures[0]['current_view']['view_id'],limit=8);rr=response(q,fixtures[0]['current_view'],'history',maxpage);add_dto('maximum_eight_metadata','EventResponse',rr)
    nextpage=hist_model.call(dict(method='history',now_ms=hist_model.clock,limit=8,after_event_id=maxpage['next_event_id']));add_dto('history_exact_cursor','HistoryPage',nextpage)
    # Representative malformed typed values plus exact transport grammar mutations.
    mutations=[('Pin',PIN,['participants'],list(reversed(PIN['participants']))),('Pin',PIN,['participants'],[PIN['participants'][0]]),('Pin',PIN,['rules_id'],'0'*64),('Event',event(PIN,s),['incident_deadline_ms'],C+7*86400000+1),('SnapshotReceipt',record,['snapshot_digest'],'0'*64),('SnapshotReceipt',record,['completed_at_ms'],C-1),('SnapshotReceipt',record,['captures'],list(reversed(record['captures']))),('SnapshotInput',record['input'],['participants'],list(reversed(record['input']['participants'])))]
    part=record['input']['participants'][0]
    for path,value in [(['owner','ownership_basis'],'transferable_ownership'),(['owner','gym_id'],g1),(['roster','slots'],part['roster']['slots']+['']),(['members'],part['members']*2),(['members',0,'eligibility'],'retired'),(['members',0,'condition_at_cutoff','as_of_ms'],C+1),(['members',0,'slot_assigned_at_ms'],C),(['members',0,'assignment','source_record_digest'],'x'*64),(['entry','latest_receipt','committed_at_ms'],C)]:mutations.append(('ParticipantSnapshot',part,path,value))
    unrostered=next(x['value']['input']['participants'][0]for x in records if x['name']=='unrostered');add_dto('unrostered_numeric_zero_control','ParticipantSnapshot',unrostered);zero_index=next(i for i,x in enumerate(unrostered['members'])if x['slot']==0);mutations.append(('ParticipantSnapshot',unrostered,['members',zero_index,'slot_assigned_at_ms'],False))
    for idx,(kind,v,path,value)in enumerate(mutations):
        m=mutate_path(v,path,value);b=canon(m)
        try:parse(b,kind);raise AssertionError(('mutation accepted',kind,path))
        except(Refusal,OLD.Refusal,ValueError):pass
        codecs.append(dict(name='semantic/'+str(idx),kind=kind,input_hex=b.hex(),accepted=False,canonical=None))
    # Correlation failures can be structurally valid; expected tuple/digest is independent of reply.
    positives=clone(protocol)
    for basecase in positives:
        for what in ('id','outer_view','wrong_payload'):
            x=clone(basecase);x['name']+='/refuse_'+what
            if what=='id':x['response']['id']+=1
            elif what=='outer_view':
                if x['response']['view']is None:continue
                x['response']['view']['view_id']=label('stale response')
            else:
                if x['response']['status']is not None:x['response']['record']=clone(record)
                else:x['response']['status']=model.status(s)
            try:correlate(x['request'],x['response'],PIN,x['current_view'],x['expected_job'],x['terminal_digest']);raise AssertionError(x['name'])
            except(Refusal,OLD.Refusal,ValueError):pass
            x.update(accepted=False,canonical=None);protocol.append(x)
    for bc in positives:
        if '/job/completed'not in bc['name']:continue
        for path,val in [(['job','result','status','observed_at_ms'],C+1000),(['job','participant_id'],g1 if bc['response']['job']['participant_id']==g0 else g0),(['job','job_id'],label('otherjob')),(['job','saturday'],'2026-11-28')]:
            x=clone(bc);x['response']=mutate_path(x['response'],path,val);x['name']+='/refuse_'+'.'.join(str(z)for z in path)
            try:correlate(x['request'],x['response'],PIN,x['current_view'],x['expected_job'],x['terminal_digest']);raise AssertionError(x['name'])
            except(Refusal,OLD.Refusal,ValueError):pass
            x.update(accepted=False,canonical=None);protocol.append(x)
    # Repeated terminal replies bind the previous terminal job, not merely its queued tuple.
    for bc in clone(positives):
        if '/job/completed'not in bc['name']:continue
        terminal=clone(bc['response']['job']);basecase=clone(bc);basecase['name']+='/repeat_terminal';basecase['expected_job']=terminal;protocol.append(basecase)
        for state in ('queued','running'):
            x=clone(basecase);x['name']+='/refuse_regression_'+state;x['response']['job']['state']=state;x['response']['job']['result']=None
            try:correlate(x['request'],x['response'],PIN,x['current_view'],x['expected_job'],x['terminal_digest']);raise AssertionError(x['name'])
            except(Refusal,OLD.Refusal,ValueError):pass
            x.update(accepted=False,canonical=None);protocol.append(x)
        # A fresh login may explicitly replay the old participant capture with fresh status.
        ep=ep_for(PIN,terminal['participant_id']);view=clone(basecase['current_view']);view['generation']+=1;view['view_id']=label('fresh capture replay '+ep['stable_pin']['account_id']);m=Coordinator(PIN,C-100000,base)
        for g in gyms(PIN):m.call(dict(method='capture',participant_id=g,now_ms=C))
        replay=m.call(dict(method='capture',participant_id=terminal['participant_id'],now_ms=C+100));j=clone(terminal);j.update(job_id=label('new replay job '+ep['stable_pin']['account_id']),view=view,result=replay);q=request('event_job',view['view_id'],job_id=j['job_id']);rr=response(q,view,'job',j);expected=clone(j);digest=sha(canon(j));protocol.append(dict(name=fixture_name(ep)+'/fresh_view_historical_capture',request=q,response=rr,fixture=fixture_name(ep),current_view=view,expected_job=expected,terminal_digest=digest,accepted=True,canonical=canon(correlate(q,rr,PIN,view,expected,digest)).decode()))
    for err in ERRORS:
        q=request('event_status',fixtures[0]['current_view']['view_id'],saturday=s);rr=dict(id=1,ok=False,view=None,store_id='',snapshot=None,status=None,record=None,history=None,job=None,error=err);add_dto('error/'+err,'EventResponse',rr);protocol.append(dict(name='error/'+err,request=q,response=rr,fixture=fixtures[0]['fixture'],current_view=fixtures[0]['current_view'],expected_job=None,terminal_digest=None,accepted=True,canonical=canon(rr).decode()))
    scenarios=[]
    for order in ((g0,g1),(g1,g0)):
        steps=[dict(method='observe',now_ms=C-1),dict(method='capture',participant_id=order[0],now_ms=C-1),dict(method='capture',participant_id=order[0],now_ms=C),dict(method='read',now_ms=C),dict(method='capture',participant_id=order[1],now_ms=C+1),dict(method='reopen'),dict(method='capture',participant_id=order[0],now_ms=C+2),dict(method='read',now_ms=C+2),dict(method='history',limit=1,now_ms=C+2),dict(method='observe',now_ms=deadline(s)),dict(method='observe',now_ms=deadline(s)-1)]
        scenarios.append(run_steps('capture_order/'+order[0],Coordinator(PIN,C-100000,base),steps))
    for fault in ('reservation_unknown','stable_fence_unknown','entry_fence_unknown','event_unknown'):
        steps=[dict(method='capture',participant_id=g0,now_ms=C,fault=fault),dict(method='reopen'),dict(method='capture',participant_id=g0,now_ms=C+1),dict(method='capture',participant_id=g1,now_ms=C+2),dict(method='read',now_ms=C+3)]
        scenarios.append(run_steps('lost_ack/'+fault,Coordinator(PIN,C-100000,base),steps))
    scenarios.append(run_steps('lost_ack/second_capture_completion',Coordinator(PIN,C-100000,base),[dict(method='capture',participant_id=g0,now_ms=C),dict(method='capture',participant_id=g1,now_ms=C+1,fault='event_unknown'),dict(method='read',now_ms=C+2),dict(method='reopen'),dict(method='read',now_ms=C+2),dict(method='capture',participant_id=g1,now_ms=C+3)]))
    for field in ('cancel_after_commit','outer_lease_error','cancel_before_lease','cancel_before_commit','history_corrupt','source_missing'):
        scenarios.append(run_steps('guard/'+field,Coordinator(PIN,C-100000,base),[dict(method='capture',participant_id=g0,now_ms=C,**{field:True}),dict(method='capture',participant_id=g0,now_ms=C+1)]))
    for field in ('cancel_before_proof','revoke_before_proof','source_corrupt_at'):
        for n in range(3):scenarios.append(run_steps(field+'/'+str(n),Coordinator(PIN,C-100000,base),[dict(method='capture',participant_id=g0,now_ms=C,**{field:n}),dict(method='capture',participant_id=g0,now_ms=C+1)]))
    for created in (C-1,C,C+1):scenarios.append(run_steps('created_boundary/'+str(created-C),Coordinator(PIN,created,base),[dict(method='capture',participant_id=g0,now_ms=C+2)]))
    for opts in ({'stable_now_ms':C-1},{'entry_now_ms':C-1},{'stable_now_ms':C+2,'entry_now_ms':C+1},{'stable_now_ms':C+2,'entry_now_ms':C+2,'final_now_ms':C+1},{'authorized':False},{'session_type':'mining'},{'current_view':False},{'session_participant_id':g1},{'normal_profile':False},{'matching_source_pin':False}):scenarios.append(run_steps('refusal/'+','.join(opts),Coordinator(PIN,C-100000,base),[dict(method='capture',participant_id=g0,now_ms=C,**opts)]))
    for cc in codecs:cc['validation_scope']='pinned_semantic_codec'if cc['kind']not in('EventRequest','EventResponse','EventSnapshot','EventJob')else'closed_transport_plus_pinned_semantics'
    bounds=dict(max_observed_depth=max(depth(x['value'])for x in dtos),max_canonical_bytes_by_kind={k:max(len(x['canonical'])for x in dtos if x['kind']==k)for k in sorted({x['kind']for x in dtos})},caps=CAPS,formula_core_sha256=INPUTS['inputs/contracts/set44-core-shape-bounds.json']['sha256'],formula_helper_sha256=INPUTS['inputs/contracts/set44-helper-shape-bounds.json']['sha256'])
    return dict(version=1,core_contract_sha256=CORE,helper_contract_sha256=HELPER,clarification_sha256=CLARIFICATION,reference_input_index_sha256=sha((HERE/'input_hashes.json').read_bytes()),rules=dict(value=RULES,canonical=canon(RULES).decode(),preimage=RULE_PRE.hex(),id=RULES_ID),pin=dict(value=PIN,canonical=canon(PIN).decode(),store_preimage=store_pre(PIN).hex(),store_id=sid(PIN)),fixtures=fixtures,calendar=cal,calendar_invalid=calendar_invalid,histories=histories,history_cases=history_cases(histories),maximum_history=maximum_history(),records=records,dtos=dtos,codec_cases=codecs,protocol_cases=protocol,initialization_cases=init_cases(),capacity_case=capacity_case(base),scenarios=scenarios,bounds=bounds,coverage_limits=['Parallel Go authoring was allowed after contract freeze; no new Go source/output was read to derive expectations.','Six unchanged public receipts were independently normal-Argon verified by retained Set41 reference; no Argon/mining rerun here.','Stable/entry operations, capture evidence, fault timing and initialization are independently modeled fixtures, not recorded real authority/crash execution.','Synthetic five-class/retirement assignments have no admitted proof and are never represented as trusted receipts.','No portable source authentication, filesystem durability, actual lease/gate concurrency, GUI, seed/finality or production ownership claim.'])

def history_cases(histories):
    out=[]
    for family in ('stable','entry'):
        h=next(x for x in histories if x['name'].endswith('/ordinary'));ep=next(x for x in PIN['participants']if fixture_name(x)==h['fixture']);rows=h[family+'_operations']
        mutations=[('missing_first',rows[1:]),('duplicate_sequence',rows+[clone(rows[-1])]),('changed_intent_id',mutate_path(rows,[0,'intent','operation_id'],label('different'))),('future_prepare',mutate_path(rows,[0,'intent','prepared_at_ms'],OLD.cutoff(h['saturday'])+1)),('receipt_author_drift',mutate_path(rows,[0,'receipt','view_id'],label('different author')))]
        if family=='stable':mutations += [('revision_regression',mutate_path(rows,[-1,'receipt','revision'],0)),('wrong_result_slots',mutate_path(rows,[-1,'receipt','slots',0],''))]
        else:mutations += [('previous_revision_gap',mutate_path(rows,[0,'receipt','previous_revision'],2)),('cutoff_commit',mutate_path(rows,[0,'receipt','committed_at_ms'],OLD.cutoff(h['saturday'])))]
        for name,changed in mutations:
            try:
                req(len(changed)==len(rows),'event_corrupt')
                stable_replay(ep,h['source'],changed,OLD.cutoff(h['saturday']))if family=='stable'else entry_replay(ep,changed,h['saturday']);accepted=True
            except(Refusal,OLD.Refusal,ValueError):accepted=False
            assert not accepted,(family,name)
            out.append(dict(name=family+'/'+name,base_history=h['name'],family=family,expected_operation_count=len(rows),operations=changed,accepted=False))
    for family in ('stable','entry'):
        h=next(x for x in histories if x['name'].endswith('/ordinary'));ep=next(x for x in PIN['participants']if fixture_name(x)==h['fixture']);C=OLD.cutoff(h['saturday']);view=view_for(ep)
        for state in ('prepared','abandoned'):
            if family=='stable':
                rows=clone(h['stable_operations']);old=rows[-1]['receipt'];o=stable_op(len(rows)+1,view,1,'',old['slots'],old['revision'],C-9999,state=state)
            else:
                rows=clone(h['entry_operations']);o=entry_op(ep,h['source'],view,h['saturday'],len(rows)+1,1,3,h['expected_snapshot']['roster']['slots'],C-9999,None,state=state)
            o['intent']['request']['expected_revision']+=1;rows.append(o)
            try:
                stable_replay(ep,h['source'],rows,C)if family=='stable'else entry_replay(ep,rows,h['saturday']);raise AssertionError('noncommitted stale revision')
            except(Refusal,OLD.Refusal):pass
            out.append(dict(name=family+'/'+state+'_wrong_expected_revision',base_history=h['name'],family=family,expected_operation_count=len(rows),operations=rows,accepted=False))
        if family=='entry':
            for lower_count in (False,True):
                rows=clone(h['entry_operations']);o=entry_op(ep,h['source'],view,h['saturday'],2,1,3,h['expected_snapshot']['roster']['slots'],C-9999,C-9998,ids=[]);o['receipt']['stable_checkpoint']=2
                if lower_count:o['receipt']['observed_source_count']=2
                rows.append(o)
                try:entry_replay(ep,rows,h['saturday']);raise AssertionError('source highwater regression')
                except(Refusal,OLD.Refusal):pass
                out.append(dict(name='entry/source_'+('count'if lower_count else'checkpoint')+'_regression',base_history=h['name'],family=family,expected_operation_count=len(rows),operations=rows,accepted=False))
    for h in histories:
        if not h['name'].endswith('/prepared'):continue
        ep=next(x for x in PIN['participants']if fixture_name(x)==h['fixture']);rows=clone(h['stable_operations']);old=rows[-1];i=old['intent'];before=rows[-2]['receipt'];resolved=stable_op(i['sequence'],view_for(ep),i['request']['slot'],i['request']['prospect_id'],before['slots'],before['revision'],i['prepared_at_ms'],OLD.cutoff(h['saturday'])+1);rows[-1]=resolved;got,_=stable_replay(ep,h['source'],rows,OLD.cutoff(h['saturday']));assert got==h['expected_snapshot']['roster']
        out.append(dict(name=h['fixture']+'/prepared_resolves_post_cutoff',base_history=h['name'],family='stable',operations=rows,accepted=True,expected_roster=got,expected_historical_digest_unchanged=True))
    # A corrupted retained assignment cannot be used for even an unselected past placement.
    h=next(x for x in histories if x['name'].endswith('/none'));ep=next(x for x in PIN['participants']if fixture_name(x)==h['fixture']);source=clone(h['source']);key=next(iter(source));source[key]['assignment']['accepted_at_ms']=OLD.cutoff(h['saturday'])
    try:stable_replay(ep,source,h['stable_operations'],OLD.cutoff(h['saturday']));raise AssertionError('late assignment accepted')
    except Refusal:pass
    out.append(dict(name='unselected_past_placement_late_acceptance',base_history=h['name'],family='stable',assignment_patch=dict(prospect_id=key,accepted_at_ms=OLD.cutoff(h['saturday'])),accepted=False))
    return out

def maximum_history():
    ep=PIN['participants'][0];s='2026-11-21';C=OLD.cutoff(s);view=view_for(ep);src=OLD.fixture_source(fixture_name(ep));slot=['']*10;rev=0;rows=[]
    for n in range(4096):
        target=next(iter(src));o=stable_op(n+1,view,1,target,slot,rev,C-1,C-1);rows.append(o);slot=o['receipt']['slots'];rev=o['receipt']['revision']
    roster,_=stable_replay(ep,src,rows,C)
    entryrows=[entry_op(ep,src,view,s,n+1,n,0,['']*10,C-1,C-1,ids=[])for n in range(4096)];ent,_=entry_replay(ep,entryrows,s)
    return dict(recipe='4096 stable commits at C-1: place first sorted source in slot1 then4095 noops;4096 entry withdrawals at C-1; every prepare C-1; operation IDs use unchanged modeled factories',fixture=fixture_name(ep),saturday=s,stable_count=len(rows),entry_count=len(entryrows),stable_max_row_bytes=max(len(canon(x))for x in rows),entry_max_row_bytes=max(len(canon(x))for x in entryrows),expected_roster=roster,expected_entry=ent,evidence_kind='bounded_reference_arithmetic_not_runtime_resource_or_gate_test')

def capacity_case(histories):
    s='2026-11-21';C=OLD.cutoff(s);m=Coordinator(PIN,C-100000,histories);dates=OLD.DATES[OLD.DATES.index(s):OLD.DATES.index(s)+257];now=OLD.cutoff(dates[-1]);m.clock=now
    for ds in dates[:-1]:m.rows[ds]=dict(created=OLD.cutoff(ds),captures={},receipt=None)
    results=[]
    for q in [dict(method='capture',saturday=dates[-1],now_ms=now),dict(method='capture',saturday=s,participant_id=gyms(PIN)[0],now_ms=now),dict(method='capture',saturday=s,participant_id=gyms(PIN)[1],now_ms=now)]:
        try:r=m.call(q);error=''
        except Refusal as e:r=None;error=str(e)
        results.append(dict(request=q,result=r,error=error,event_count=len(m.rows),expected_proof_count=m.proofs))
    assert results[0]['error']=='event_capacity'and all(not x['error']for x in results[1:])
    return dict(recipe='256 existing awaiting_capture reservations on consecutive supported Saturdays starting2026-11-21, coordinator created C-100000; same actual observation at257th cutoff; new reservation refuses, original admitted event completes',evidence_kind='semantic_capacity_not_disk_resource_measurement',initial_event_count=256,steps=results)

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--check',action='store_true');args=ap.parse_args();obj=corpus();b=pretty(obj);p=HERE/'event_v1.json'
    if args.check:
        if not p.exists()or p.read_bytes()!=b:raise SystemExit('event_v1.json differs')
    else:p.write_bytes(b)
    print(json.dumps(dict(status='passed',check=args.check,sha256=sha(b),bytes=len(b),calendar=len(obj['calendar']),histories=len(obj['histories']),records=len(obj['records']),dtos=len(obj['dtos']),codec_cases=len(obj['codec_cases']),protocol_cases=len(obj['protocol_cases']),scenarios=len(obj['scenarios']),initialization_cases=len(obj['initialization_cases']))))
if __name__=='__main__':main()
