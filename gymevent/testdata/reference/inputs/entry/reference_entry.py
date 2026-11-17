#!/usr/bin/env python3
"""Independent, bounded Set43 integer/calendar/DTO/state reference. No Go or Argon.
State sources are semantic fixtures, NOT a public trusted-selection/import API.
"""
import argparse, bisect, copy, datetime as dt, hashlib, importlib.util, io, json, re, struct, sys
from pathlib import Path
from zoneinfo import ZoneInfo
sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
CONTRACT = '958d3fda0cb50cda3bad38f7e461a44537a6a3c20ef5cc2b0c2abb7d2fdafcae'
TZSHA = '200d05754f6d83a371cf408d7085125797657b3b0bebeba1e508cffe86a3e5c8'
SAFE = 9007199254740991
DATE_MAX = 8640000000000000
CLASSES = ('light','welter','middle','light_heavy','heavy')
PROJECTOR = 'slug-fighter-condition-candidate-v1'
WEEK = 604800000
UTC = dt.timezone.utc
EPOCH = dt.datetime(1970,1,1,tzinfo=UTC)
ERRORS = ('invalid_request','unauthorized','forbidden','stale_view','busy','entry_unavailable','entry_corrupt','entry_capacity','entry_clock','entry_calendar','entry_closed','entry_conflict','entry_pending','entry_not_found','entry_sync_required','entry_not_rostered','entry_retired','entry_class_conflict','entry_cancelled','outcome_unknown')
def sha(b): return hashlib.sha256(b).hexdigest()
def canon(x): return json.dumps(x,ensure_ascii=True,separators=(',',':'),allow_nan=False).encode('ascii')
def pretty(x): return (json.dumps(x,ensure_ascii=True,indent=2,allow_nan=False)+'\n').encode('ascii')
def clone(x): return copy.deepcopy(x)
def u32(x): return struct.pack('<I',x)
def u64(x): return struct.pack('<Q',x)
def lp(x): b=x.encode('utf8'); return u32(len(b))+b
def raw(x): assert re.fullmatch('[0-9a-f]{64}',x); return bytes.fromhex(x)
def domain(x): return ('SLUG '+x+'\0v1\0').encode('ascii')
def ms(x): delta=x.astimezone(UTC)-EPOCH; return delta.days*86400000+delta.seconds*1000+delta.microseconds//1000
class Refusal(ValueError): pass
def fail(s='invalid_request'): raise Refusal(s)
def req(test,code='invalid_request'):
    if not test: fail(code)
def load(name,path):
    spec=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(spec);sys.modules[name]=m;spec.loader.exec_module(m);return m
INPUTS=json.loads((HERE/'input_hashes.json').read_bytes())
for p,v in INPUTS.items():
    b=(HERE/p).read_bytes();assert sha(b)==v['sha256'] and len(b)==v['bytes'],p
assert INPUTS['inputs/contracts/set43-entry-contract-draft.md']['sha256']==CONTRACT
TZBYTES=(HERE/'inputs/America_Los_Angeles.tzif').read_bytes()
assert len(TZBYTES)==1294 and sha(TZBYTES)==TZSHA and TZBYTES[:5]==b'TZif2'
TZ=ZoneInfo.from_file(io.BytesIO(TZBYTES),key='pinned-entry-v1')
# An independent date-rule cross-check is obtained from the exact pinned POSIX tail.
TAIL=TZBYTES.split(b'\n')[-2].decode('ascii')
assert TAIL=='PST8PDT,M3.2.0,M11.1.0'
COND=load('entry_old_condition',HERE/'inputs/condition/reference_condition.py')
MAN=load('entry_old_manifest',HERE/'inputs/manifest/reference_manifest.py')
OLD=json.loads((HERE/'inputs/stable/stable_v1.json').read_bytes())
PUBLIC=json.loads((HERE/'inputs/stable/inputs/public-candidate-records.json').read_bytes())
MC=json.loads((HERE/'inputs/manifest/manifests_v1.json').read_bytes())
PORTRAITS=json.loads((HERE/'inputs/portraits_v1.json').read_bytes())
CAL_PRE=domain('candidate entry calendar')+u32(1)+lp('America/Los_Angeles')+raw(TZSHA)+b''.join(u32(x) for x in (2026,2099,6,12,0))
RULE_PRE=domain('candidate entry rules')+u32(1)+u32(5)+u32(1)+u64(0)+u32(1)+u32(1)+u32(1)+lp(PROJECTOR)+u32(0)+b''.join(lp(c) for c in CLASSES)+lp('intake-only')
CALENDAR_ID=sha(CAL_PRE);RULES_ID=sha(RULE_PRE)
def date_value(s):
    req(isinstance(s,str) and bool(re.fullmatch(r'[0-9]{4}-[0-9]{2}-[0-9]{2}',s)),'entry_calendar')
    try: d=dt.date.fromisoformat(s)
    except ValueError: fail('entry_calendar')
    req(2026<=d.year<=2099 and d.weekday()==5,'entry_calendar');return d

def cutoff(s):
    d=date_value(s);local=dt.datetime(d.year,d.month,d.day,12,tzinfo=TZ);value=ms(local)
    req(EPOCH+dt.timedelta(milliseconds=value)==local.astimezone(UTC),'entry_calendar')
    req((EPOCH+dt.timedelta(milliseconds=value)).astimezone(TZ).replace(tzinfo=None)==local.replace(tzinfo=None),'entry_calendar')
    return value

def tail_cutoff(d):
    # M3.2.0 / M11.1.0, Sunday=0. At Saturday noon transitions are unambiguous.
    def sunday(month,n):
        first=dt.date(d.year,month,1);return first+dt.timedelta(days=(6-first.weekday())%7+7*(n-1))
    daylight=sunday(3,2)<=d<sunday(11,1)
    return ms(dt.datetime(d.year,d.month,d.day,19 if daylight else 20,tzinfo=UTC))
DATES=[];d=dt.date(2026,1,1)
while d.weekday()!=5:d+=dt.timedelta(days=1)
while d.year<=2099:DATES.append(d.isoformat());d+=dt.timedelta(days=7)
CUTOFFS=[cutoff(d) for d in DATES]
def upcoming(now):
    req(type(now) is int and 0<now<=DATE_MAX,'entry_clock');i=bisect.bisect_right(CUTOFFS,now)
    req(i<len(DATES),'entry_calendar');return DATES[i]
def stable_id(p): b=canon(p);return sha(domain('candidate stable')+u32(len(b))+b)
def namespace_pre(p):return domain('local candidate entry namespace')+u32(p['identity']['network'])+raw(p['identity']['genesis'])+raw(p['account_authority_id'])
def entry_pin(p):return dict(version=1,namespace_id=sha(namespace_pre(p)),calendar_id=CALENDAR_ID,rules_id=RULES_ID,stable_store_id=stable_id(p),stable_pin=clone(p))
def store_pre(p): return domain('candidate entry store')+canon(p)
def store_id(p): return sha(store_pre(p))
def event_pre(p,s):return domain('candidate Saturday event')+raw(p['namespace_id'])+raw(p['calendar_id'])+raw(p['rules_id'])+s.encode('ascii')+u64(cutoff(s))
def event(p,s):return dict(version=1,event_id=sha(event_pre(p,s)),namespace_id=p['namespace_id'],calendar_id=p['calendar_id'],rules_id=p['rules_id'],saturday=s,cutoff_ms=cutoff(s))
def entry_pre(p,e):return domain('candidate Saturday entry')+raw(store_id(p))+raw(e['event_id'])+raw(p['stable_pin']['identity']['gym_id'])
def entry_id(p,e):return sha(entry_pre(p,e))
def receipt_pre(r): return domain('candidate entry receipt')+canon(r)
def receipt_digest(r):return sha(receipt_pre(r))
def request(p,s,ids,revision=0,roster_revision=3,action='replace'):
    return dict(version=1,event_id=event(p,s)['event_id'],saturday=s,action=action,expected_revision=revision,expected_roster_revision=roster_revision if action=='replace' else 0,prospect_ids=sorted(ids) if action=='replace' else [])
def condition(a,m,now):return COND.project(1,m['dna'],a['born_at_ms'],now,0)
def item(a,m,now):return dict(assignment=clone(a),manifest=clone(m),condition=condition(a,m,now))
def member(a,m,slot,now):return dict(slot=slot,assignment=clone(a),**{'class':m['physical']['weight_class']},condition_at_save=condition(a,m,now))
def fixture_source(name):
    values=[x for x in OLD['cases'] if x['fixture']==name];values.sort(key=lambda x:x['assignment']['prospect_id'])
    for x in values:
        a=x['assignment'];m=MAN.generate(a['work_hash']);assert m==x['manifest']
        b=canon(x['record']);assert b.decode()==x['record_canonical']
        assert sha(domain('stable source record')+u32(len(b))+b)==a['source_record_digest']
        for c in x['conditions']:assert condition(a,m,c['as_of_ms'])==c['expected']
    return {x['assignment']['prospect_id']:{'assignment':clone(x['assignment']),'manifest':clone(x['manifest']),'slot':i+1} for i,x in enumerate(values)}
def fake_five(p):
    out={};portraits={x['seed']:x for x in PORTRAITS['cases']}
    for cl in CLASSES:
        x=next(x for x in MC['cases'] if x['manifest']['physical']['weight_class']==cl);m=MAN.generate(x['seed']);assert m==x['manifest'] and canon(m).decode()==x['canonical']
        # Synthetic public assignment facts: no source receipt/proof exists for these IDs.
        a=dict(version=1,sequence=len(out)+1,prospect_id=sha(b'Set43 structural-only prospect\0'+raw(x['seed'])),gym_id=p['stable_pin']['identity']['gym_id'],work_hash=x['seed'],born_at_ms=1794893600000,accepted_at_ms=1794893600001,descriptor_id=p['stable_pin']['source']['descriptor_id'],manifest_digest=x['digest'],portrait_digest=portraits[x['seed']]['svg_digest'],source_record_digest=sha(b'Set43 structural-only source digest\0'+raw(x['seed'])))
        out[a['prospect_id']]={'assignment':a,'manifest':m,'slot':len(out)+1}
    return dict(sorted(out.items()))

# Closed lexical/schema checks. This reference validates its corpus DTO families;
# it is not a replacement for production provenance or a filesystem journal parser.
FIELDS={
 'Pin':'version namespace_id calendar_id rules_id stable_store_id stable_pin',
 'Event':'version event_id namespace_id calendar_id rules_id saturday cutoff_ms',
 'Request':'version event_id saturday action expected_revision expected_roster_revision prospect_ids',
 'Intent':'version sequence operation_id generation view_id request prepared_at_ms',
 'Member':'slot assignment class condition_at_save',
 'Receipt':'version store_id entry_id event operation_id operation_sequence generation view_id action previous_revision revision roster_revision stable_checkpoint observed_source_count committed_at_ms members',
 'Operation':'version intent state receipt abandoned_at_ms',
 'MutationResult':'version receipt replayed',
 'OperationResult':'version store_id operation',
 'EventStatus':'version store_id event exists phase entry_id revision latest_operation_id latest_receipt_digest saved_ids saved_at_ms saved_roster_revision current_roster_revision roster_changed eligibility pending_operation_id observed_at_ms closed_observed_at_ms',
 'CheckMember':'slot item eligibility',
 'CheckResult':'version status as_of_ms roster_revision members',
 'HistoryPage':'version store_id after_event_id next_event_id has_more observed_at_ms events',
 'EntryRequest':'id command view_id saturday after_event_id limit request operation_id job_id',
 'EntryResponse':'id ok view store_id snapshot status history job operation error',
 'EntrySnapshot':'version state error view store_id pending_operation_id active_job_id queued_job_id last_job_id',
 'EntryJob':'version job_id kind state view store_id event_id saturday operation_id error intent mutation check',
 'Marker':'application version store_id pin',
 'Metadata':'version store_id pin event_count operation_count pending_operation_id last_operation_id observed_checkpoint observed_source_count observed_roster_revision clock_ms',
 'EventRow':'version event entry_id created_at_ms revision latest_operation_id phase closed_observed_at_ms'}
FIELDS={k:v.split() for k,v in FIELDS.items()}
SCHEMA_EXAMPLES={}
def exact(v,keys):req(type(v) is dict and set(v)==set(keys))
def integer(x,lo=0,hi=SAFE):req(type(x) is int and lo<=x<=hi)
def hexid(x,empty=False):req(type(x) is str and ((empty and x=='') or re.fullmatch('[0-9a-f]{64}',x)))
def ordered_like(v,s):
    if isinstance(s,dict):exact(v,s);return {k:ordered_like(v[k],s[k]) for k in s}
    if isinstance(s,list):req(type(v) is list);return [ordered_like(x,s[0]) for x in v] if s else v
    req(type(v) is type(s));return v

def parse(raw_bytes,kind,pin):
    cap=8192 if kind in ('Pin','Request','EntryRequest','Marker','Metadata') else (32768 if kind=='Operation' else 131072)
    req(type(raw_bytes) is bytes and 0<len(raw_bytes)<=cap)
    try:s=raw_bytes.decode('utf8','strict')
    except UnicodeError:fail()
    def pairs(ps):
        d={}
        for k,v in ps:req(k not in d);d[k]=v
        return d
    def num(t):req(bool(re.fullmatch('0|[1-9][0-9]*|-[1-6]',t)));return int(t)
    try:v=json.loads(s,object_pairs_hook=pairs,parse_int=num,parse_float=lambda _:fail(),parse_constant=lambda _:fail())
    except (ValueError,RecursionError):fail()
    def bounded(v,path=(),depth=0):
        if isinstance(v,(dict,list)):req(depth+1<=10)
        if isinstance(v,dict):
            req(len(v)<=64)
            for k,x in v.items():bounded(x,path+(k,),depth+1)
        elif isinstance(v,list):
            lim=8 if path and path[-1]=='events' else 5
            req(len(v)<=lim)
            for x in v:bounded(x,path+('*',),depth+1)
        elif type(v) is int:
            if path[-1:] == ('shoulder_jitter',):integer(v,-6,6)
            else:integer(v)
        elif isinstance(v,str):req(not any(0xD800<=ord(c)<=0xDFFF for c in v))
    bounded(v);return validate(kind,v,pin)

def validate(kind,v,p):
    exact(v,FIELDS[kind]);v={k:v[k] for k in FIELDS[kind]}
    if 'version' in v:integer(v['version'],1,1)
    sid=store_id(p)
    for k,x in v.items():
        if k in ('generation',):integer(x,1)
        elif k in ('sequence','operation_sequence'):integer(x,1,4096)
        elif k.endswith('_ms'):integer(x,0,DATE_MAX)
        elif k in ('revision','previous_revision','expected_revision','roster_revision','expected_roster_revision','saved_roster_revision','current_roster_revision','stable_checkpoint','observed_source_count','observed_checkpoint','observed_roster_revision','event_count','operation_count'):integer(x)
        if k in ('store_id',):req(x==sid)
        if k in ('operation_id','view_id','job_id','pending_operation_id','latest_operation_id','last_operation_id','active_job_id','queued_job_id','last_job_id','after_event_id','next_event_id','latest_receipt_digest'):hexid(x,True)
    if kind=='Marker':req(v['application']=='slug-candidate-entries');v['pin']=validate('Pin',v['pin'],p)
    elif kind=='Metadata':
        v['pin']=validate('Pin',v['pin'],p);req(0<=v['observed_checkpoint']<=v['observed_source_count']<=1000);integer(v['event_count'],0,256);integer(v['operation_count'],0,4096)
    elif kind=='EventRow':
        v['event']=validate('Event',v['event'],p);req(v['entry_id']==entry_id(p,v['event']));req(0<v['created_at_ms']<v['event']['cutoff_ms']);req(v['phase'] in ('accepting','closed_pending_lock'));req(v['closed_observed_at_ms']==0 if v['phase']=='accepting' else v['closed_observed_at_ms']>=v['event']['cutoff_ms'])
    if kind=='Pin':v=ordered_like(v,p);req(canon(v)==canon(p));return v
    if kind=='Event':req(v==event(p,v['saturday']));return v
    if kind=='Request':
        req(v['event_id']==event(p,v['saturday'])['event_id']);req(v['action'] in ('replace','withdraw'));ids=v['prospect_ids'];req(type(ids) is list)
        for x in ids:hexid(x)
        req(ids==sorted(set(ids)))
        if v['action']=='replace':req(1<=len(ids)<=5)
        else:req(ids==[] and v['expected_roster_revision']==0)
    elif kind=='Intent':
        hexid(v['operation_id']);hexid(v['view_id']);v['request']=validate('Request',v['request'],p);req(0<v['prepared_at_ms']<cutoff(v['request']['saturday']))
    elif kind=='Member':
        integer(v['slot'],1,10);a=v['assignment'];m=MAN.generate(a['work_hash']);req(v['class']==m['physical']['weight_class']);req(v['condition_at_save']==condition(a,m,v['condition_at_save']['as_of_ms']));req(a['gym_id']==p['stable_pin']['identity']['gym_id']);req(a['descriptor_id']==p['stable_pin']['source']['descriptor_id']);req(0<a['born_at_ms']<=a['accepted_at_ms']<=v['condition_at_save']['as_of_ms']<=DATE_MAX)
        v['assignment']=ordered_like(a,OLD['cases'][0]['assignment']);v['condition_at_save']=ordered_like(v['condition_at_save'],OLD['cases'][0]['conditions'][0]['expected'])
    elif kind=='Receipt':
        v['event']=validate('Event',v['event'],p);req(v['entry_id']==entry_id(p,v['event']));hexid(v['operation_id']);hexid(v['view_id']);req(0<v['committed_at_ms']<v['event']['cutoff_ms']);req(v['revision']==v['previous_revision']+1 and v['revision']<=4096);req(0<=v['stable_checkpoint']<=v['observed_source_count']<=1000)
        req(type(v['members']) is list);v['members']=[validate('Member',m,p) for m in v['members']];ids=[m['assignment']['prospect_id'] for m in v['members']]
        req(ids==sorted(set(ids)));req(len({m['class'] for m in v['members']})==len(ids));req(len({m['slot'] for m in v['members']})==len(ids));req(all(m['condition_at_save']['as_of_ms']==v['committed_at_ms'] and m['condition_at_save']['phase']!='retired' for m in v['members']))
        req((v['action']=='replace' and 1<=len(ids)<=5) or (v['action']=='withdraw' and not ids and v['roster_revision']==0))
    elif kind=='Operation':
        v['intent']=validate('Intent',v['intent'],p);req(v['state'] in ('prepared','committed','abandoned'))
        if v['state']=='committed':
            v['receipt']=validate('Receipt',v['receipt'],p);r=v['receipt'];i=v['intent'];q=i['request'];req(v['abandoned_at_ms']==0 and i['prepared_at_ms']<=r['committed_at_ms']);req(all(r[k]==i[k] for k in ('operation_id','generation','view_id')) and r['operation_sequence']==i['sequence']);req(r['event']['event_id']==q['event_id'] and r['action']==q['action'] and r['previous_revision']==q['expected_revision']);req([m['assignment']['prospect_id'] for m in r['members']]==q['prospect_ids']);req(r['roster_revision']==q['expected_roster_revision'])
        else:req(v['receipt'] is None and (v['abandoned_at_ms']==0 if v['state']=='prepared' else v['abandoned_at_ms']>=v['intent']['prepared_at_ms']))
    elif kind=='MutationResult':v['receipt']=validate('Receipt',v['receipt'],p);req(type(v['replayed']) is bool)
    elif kind=='OperationResult':
        if v['operation'] is not None:v['operation']=validate('Operation',v['operation'],p)
    elif kind=='EventStatus':
        v['event']=validate('Event',v['event'],p);req(v['entry_id']==entry_id(p,v['event']));req(v['eligibility']=='not_checked');req(v['phase']==('closed_pending_lock' if v['observed_at_ms']>=v['event']['cutoff_ms'] else 'accepting'));req(type(v['exists']) is bool and type(v['roster_changed']) is bool);req(v['roster_changed']==bool(v['saved_ids'] and v['saved_roster_revision']!=v['current_roster_revision']));req(v['saved_ids']==sorted(set(v['saved_ids'])) and len(v['saved_ids'])<=5)
        if not v['exists']:req(v['revision']==v['saved_at_ms']==v['saved_roster_revision']==v['closed_observed_at_ms']==0 and not v['latest_operation_id'] and not v['latest_receipt_digest'] and not v['saved_ids'])
        elif v['phase']=='closed_pending_lock':req(v['event']['cutoff_ms']<=v['closed_observed_at_ms']<=v['observed_at_ms'])
    elif kind=='CheckMember':
        integer(v['slot'],0,10);it=v['item'];exact(it,('assignment','manifest','condition'));a=ordered_like(it['assignment'],OLD['cases'][0]['assignment']);m=MAN.generate(a['work_hash']);req(it['manifest']==m);req(it['condition']==condition(a,m,it['condition']['as_of_ms']));v['item']=item(a,m,it['condition']['as_of_ms']);req(v['eligibility']==('retired' if it['condition']['phase']=='retired' else 'not_rostered' if v['slot']==0 else 'eligible'))
    elif kind=='CheckResult':
        v['status']=validate('EventStatus',v['status'],p);req(v['as_of_ms']==v['status']['observed_at_ms'] and v['roster_revision']==v['status']['current_roster_revision']);v['members']=[validate('CheckMember',m,p) for m in v['members']];req([m['item']['assignment']['prospect_id'] for m in v['members']]==v['status']['saved_ids']);req(all(m['item']['condition']['as_of_ms']==v['as_of_ms'] for m in v['members']))
    elif kind=='HistoryPage':
        req(type(v['events']) is list and len(v['events'])<=8 and type(v['has_more']) is bool);v['events']=[validate('EventStatus',e,p) for e in v['events']];req(all(e['exists'] and e['observed_at_ms']==v['observed_at_ms'] for e in v['events']));req(v['next_event_id']==(v['events'][-1]['event']['event_id'] if v['has_more'] and v['events'] else ''))
    return v

class State:
    """Finite semantic authority model. Source fixture facts are explicit assumptions."""
    def __init__(self,p,source,view,now):
        self.pin=clone(p);self.source=clone(source);self.view=clone(view);self.clock=now;self.events={};self.ops={};self.pending='';self.last='';self.checkpoint=len(source);self.count=len(source);self.rr=len(source);self.proofs=0;self.poison=False
    def observe(self,now):req(type(now) is int and self.clock<=now<=DATE_MAX,'entry_clock');self.clock=now
    def close_event(self,s):
        if s in self.events and self.clock>=cutoff(s) and self.events[s]['phase']=='accepting':self.events[s]['phase']='closed_pending_lock';self.events[s]['closed_observed_at_ms']=self.clock
    def eventrow(self,s):
        e=event(self.pin,s);return dict(version=1,event=e,entry_id=entry_id(self.pin,e),created_at_ms=self.clock,revision=0,latest_operation_id='',phase='accepting',closed_observed_at_ms=0)
    def status(self,s):
        self.close_event(s);e=event(self.pin,s);row=self.events.get(s);r=self.ops[row['latest_operation_id']]['receipt'] if row and row['latest_operation_id'] else None
        ids=[x['assignment']['prospect_id'] for x in r['members']] if r else []
        return dict(version=1,store_id=store_id(self.pin),event=e,exists=row is not None,phase='closed_pending_lock' if self.clock>=e['cutoff_ms'] else 'accepting',entry_id=entry_id(self.pin,e),revision=row['revision'] if row else 0,latest_operation_id=row['latest_operation_id'] if row else '',latest_receipt_digest=receipt_digest(r) if r else '',saved_ids=ids,saved_at_ms=r['committed_at_ms'] if r else 0,saved_roster_revision=r['roster_revision'] if r else 0,current_roster_revision=self.rr,roster_changed=bool(ids and r['roster_revision']!=self.rr),eligibility='not_checked',pending_operation_id=self.pending if self.pending and self.ops[self.pending]['intent']['request']['saturday']==s else '',observed_at_ms=self.clock,closed_observed_at_ms=row['closed_observed_at_ms'] if row else 0)
    def summary(self):return dict(clock_ms=self.clock,event_count=len(self.events),operation_count=len(self.ops),pending_operation_id=self.pending,last_operation_id=self.last,observed_checkpoint=self.checkpoint,observed_source_count=self.count,observed_roster_revision=self.rr,events=clone(list(self.events.values())),operations=[{'operation_id':k,'state':o['state'],'receipt_digest':receipt_digest(o['receipt']) if o['receipt'] else ''} for k,o in self.ops.items()],poisoned=self.poison)
    def call(self,c):
        try:return self._call(c)
        except Refusal:
            # A fresh observation after slow authenticated source failure is retained.
            if c.get('authorized',True) and self.proofs>0 and 'final_now_ms' in c:
                self.observe(c['final_now_ms'])
                if c.get('operation_id') in self.ops:self.close_event(self.ops[c['operation_id']]['intent']['request']['saturday'])
            raise
    def _call(self,c):
        self.proofs=0;method=c['method'];now=c['now_ms'];v=c.get('session',self.view)
        req(not self.poison,'entry_unavailable');req(c.get('authorized',True),'unauthorized')
        if method in ('prepare','apply','abandon'):req(c.get('session_type','login')=='login','forbidden')
        self.observe(now)
        if 'source_observation' in c:
            z=c['source_observation'];req(z['checkpoint']>=self.checkpoint and z['count']>=self.count and z['roster_revision']>=self.rr,'entry_corrupt');self.checkpoint=z['checkpoint'];self.count=z['count'];self.rr=z['roster_revision']
        if method=='prepare':
            q=validate('Request',clone(c['request']),self.pin);s=q['saturday'];self.close_event(s)
            if self.pending:
                old=self.ops[self.pending]['intent'];req(old['request']==q and old['view_id']==v['view_id'] and old['generation']==v['generation'],'entry_pending');return clone(old)
            req(now<cutoff(s),'entry_closed');req(q['expected_revision']==self.events.get(s,{}).get('revision',0),'entry_conflict');req(len(self.ops)<4096 and (s in self.events or len(self.events)<256),'entry_capacity')
            self.observe(c.get('final_now_ms',now));self.close_event(s);req(self.clock<cutoff(s),'entry_closed');req(not c.get('cancel_before_commit',False),'entry_cancelled')
            op=c['random_id'];hexid(op);req(op not in self.ops,'entry_capacity');i=dict(version=1,sequence=len(self.ops)+1,operation_id=op,generation=v['generation'],view_id=v['view_id'],request=q,prepared_at_ms=self.clock)
            if s not in self.events:self.events[s]=self.eventrow(s)
            self.ops[op]=dict(version=1,intent=i,state='prepared',receipt=None,abandoned_at_ms=0);self.pending=op;self.last=op;result=clone(i)
        elif method=='apply':
            op=c['operation_id'];req(op in self.ops,'entry_not_found');o=self.ops[op];i=o['intent'];q=i['request'];s=q['saturday'];self.close_event(s)
            req(i['view_id']==v['view_id'] and i['generation']==v['generation'],'stale_view')
            if o['state']=='committed':return dict(version=1,receipt=clone(o['receipt']),replayed=True)
            req(o['state']=='prepared','entry_conflict');req(self.pending==op and self.events[s]['revision']==q['expected_revision'],'entry_conflict');members=[]
            if q['action']=='replace':
                req(self.rr==q['expected_roster_revision'],'entry_conflict')
                for pid in q['prospect_ids']:
                    req(pid in self.source,'entry_sync_required');self.proofs+=1;src=self.source[pid];req(not c.get('source_digest_drift',False),'entry_corrupt');req(src['assignment']['gym_id']==self.pin['stable_pin']['identity']['gym_id'] and src['assignment']['descriptor_id']==self.pin['stable_pin']['source']['descriptor_id'],'entry_corrupt');m=member(src['assignment'],src['manifest'],src['slot'],self.clock);members.append(m)
                req(all(m['slot']>0 for m in members),'entry_not_rostered');req(all(m['condition_at_save']['phase']!='retired' for m in members),'entry_retired')
                req(len({m['class'] for m in members})==len(members),'entry_class_conflict');req(not c.get('roster_changes_after_proof',False),'entry_conflict')
            self.observe(c.get('final_now_ms',now));self.close_event(s);req(not c.get('cancel_before_commit',False),'entry_cancelled');req(self.clock<cutoff(s),'entry_closed')
            for m in members:
                src=self.source[m['assignment']['prospect_id']];m['condition_at_save']=condition(src['assignment'],src['manifest'],self.clock);req(m['condition_at_save']['phase']!='retired','entry_retired')
            e=event(self.pin,s);r=dict(version=1,store_id=store_id(self.pin),entry_id=entry_id(self.pin,e),event=e,operation_id=op,operation_sequence=i['sequence'],generation=i['generation'],view_id=i['view_id'],action=q['action'],previous_revision=q['expected_revision'],revision=q['expected_revision']+1,roster_revision=q['expected_roster_revision'],stable_checkpoint=self.checkpoint,observed_source_count=self.count,committed_at_ms=self.clock,members=members)
            validate('Receipt',r,self.pin);o.update(state='committed',receipt=r);self.events[s].update(revision=r['revision'],latest_operation_id=op);self.pending='';result=dict(version=1,receipt=clone(r),replayed=False)
        elif method in ('operation','receipt','abandon'):
            op=c['operation_id'];req(op in self.ops,'entry_not_found');o=self.ops[op];self.close_event(o['intent']['request']['saturday'])
            if method=='receipt':req(o['receipt'] is not None,'entry_not_found');return clone(o['receipt'])
            if method=='abandon' and o['state']=='prepared':o.update(state='abandoned',abandoned_at_ms=now);self.pending=''
            result=dict(version=1,store_id=store_id(self.pin),operation=clone(o))
        elif method=='pending':return dict(version=1,store_id=store_id(self.pin),operation=clone(self.ops[self.pending]) if self.pending else None)
        elif method=='status':return self.status(c['saturday'])
        elif method=='upcoming':return self.status(upcoming(now))
        elif method=='history':
            lim=c['limit'];integer(lim,1,8);rows=sorted(self.events,key=lambda s:(cutoff(s),event(self.pin,s)['event_id']));after=c.get('after_event_id','');start=0
            if after:
                ids=[event(self.pin,s)['event_id'] for s in rows];req(after in ids,'entry_not_found');start=ids.index(after)+1
            selected=rows[start:start+lim];more=start+lim<len(rows);return dict(version=1,store_id=store_id(self.pin),after_event_id=after,next_event_id=event(self.pin,selected[-1])['event_id'] if more else '',has_more=more,observed_at_ms=now,events=[self.status(s) for s in selected])
        elif method=='check':
            s=c['saturday'];st=self.status(s);members=[];selected=[]
            for pid in st['saved_ids']:
                req(pid in self.source,'entry_sync_required');self.proofs+=1;src=self.source[pid];req(not c.get('source_digest_drift',False),'entry_corrupt');selected.append(src)
            req(not c.get('roster_changes_after_proof',False),'entry_conflict');self.observe(c.get('final_now_ms',now));st=self.status(s)
            for src in selected:
                it=item(src['assignment'],src['manifest'],self.clock);members.append(dict(slot=src['slot'],item=it,eligibility='retired' if it['condition']['phase']=='retired' else 'not_rostered' if src['slot']==0 else 'eligible'))
            result=dict(version=1,status=st,as_of_ms=self.clock,roster_revision=self.rr,members=members)
        else:fail()
        if c.get('write_ack_unknown',False):self.poison=True;fail('outcome_unknown')
        if c.get('postcommit_lease_error',False):fail('outcome_unknown')
        return result

def run_steps(name,model,steps,source_kind='retained_independently_verified_fixture'):
    initial_source=clone(list(model.source.values())); initial_state=model.summary(); out=[]
    for c in steps:
        model.proofs=0
        if c['method']=='reopen':model.poison=False;result=None;err=''
        elif c['method']=='set_slots':
            for pid,slot in c['slots'].items():model.source[pid]['slot']=slot
            model.rr=c['roster_revision'];result=None;err=''
        else:
            try:result=model.call(c);err=''
            except Refusal as e:result=None;err=str(e)
        bounds=dict(min=model.proofs,max=model.proofs,reason='exact reference path')
        if c['method']=='apply' and err=='entry_closed' and c.get('operation_id') in model.ops:
            intent=model.ops[c['operation_id']]['intent']
            if c['now_ms']>=cutoff(intent['request']['saturday']):
                bounds=dict(min=0,max=len(intent['request']['prospect_ids']),reason='already closed at initial observation may refuse before source verification')
        out.append({'call':clone(c),'expected_error':err,'expected_result':clone(result),'expected_canonical':canon(result).decode() if result is not None else '', 'reference_model_proof_count':model.proofs,'runtime_proof_bounds':bounds,'final_guard_required':c['method']=='apply' and 'final_now_ms' in c,'expected_state':model.summary()})
    return dict(name=name,source_kind=source_kind,pin=clone(model.pin),initial_source=initial_source,initial_state=initial_state,steps=out,final_operations=clone(list(model.ops.values())))

def codec_cases(kind,value,p):
    # Valid decoded transport variants, then lexical/closed-schema failures.
    b=canon(value);out=[]
    def add(name,raw_bytes,accepted):
        try:v=parse(raw_bytes,kind,p);result=canon(v).decode();ok=True
        except (Refusal,AssertionError,KeyError,TypeError,ValueError):ok=False;result=''
        assert ok==accepted,(kind,name,ok)
        out.append(dict(name=kind+'/'+name,kind=kind,pin=p,raw_hex=raw_bytes.hex(),accepted=accepted,canonical=result))
    add('canonical',b,True)
    if kind not in ('Marker','Metadata','EventRow'):
        def reverse_keys(x):
            if isinstance(x,dict):return {k:reverse_keys(x[k]) for k in reversed(x)}
            if isinstance(x,list):return [reverse_keys(v) for v in x]
            return x
        rev=reverse_keys(value);add('ordered_whitespace',json.dumps(rev,indent=1).encode(),True)
        first=next(iter(value));escaped='\\u%04x'%ord(first[0])+first[1:];add('escaped_ascii_key',b.replace(('"'+first+'"').encode(),('"'+escaped+'"').encode(),1),True)
    add('unknown',b[:-1]+b',"unexpected":0}',False)
    first=next(iter(value));add('duplicate',b[:-1]+b','+canon(first)+b':'+canon(value[first])+b'}',False)
    escaped_first=('\\u%04x'%ord(first[0]))+first[1:];add('duplicate_decoded',b[:-1]+b',\"'+escaped_first.encode()+b'\":'+canon(value[first])+b'}',False)
    x=clone(value);x.pop(first);add('missing',canon(x),False)
    add('bom',b'\xef\xbb\xbf'+b,False);add('trailing',b+b'{}',False);add('invalid_utf8',b[:-1]+b'\xff}',False)
    if 'version' in value:
        for name,repl in [('version_bool',b'true'),('version_unknown',b'2'),('version_float',b'1.0'),('version_exponent',b'1e0'),('version_negative',b'-1'),('version_negzero',b'-0')]:add(name,b.replace(b'"version":1',b'"version":'+repl,1),False)
    return out

# Protocol correlation is separate from structural parsing: it needs the captured
# request, current view, derived pin, and original acknowledged job tuple.
def wire_request(command,view='',**kw):
    v=dict(id=1,command=command,view_id=view,saturday='',after_event_id='',limit=0,request=None,operation_id='',job_id='');v.update(kw);return v

def validate_wire_request(q,p):
    exact(q,FIELDS['EntryRequest']);integer(q['id'],1);cmd=q['command']
    allowed={'entry_snapshot':set(),'entry_upcoming':{'view_id'},'entry_status':{'view_id','saturday'},'entry_history':{'view_id','after_event_id','limit'},'entry_prepare':{'view_id','request'},'entry_apply':{'view_id','operation_id'},'entry_check':{'view_id','saturday'},'entry_job':{'view_id','job_id'},'entry_cancel':{'view_id','job_id'},'entry_operation':{'view_id','operation_id'},'entry_pending':{'view_id'},'entry_abandon':{'view_id','operation_id'}}
    req(cmd in allowed);blank=wire_request(cmd)
    for k in blank:
        if k not in ('id','command') and k not in allowed[cmd]:req(q[k]==blank[k] and type(q[k]) is type(blank[k]))
    if cmd!='entry_snapshot':hexid(q['view_id'])
    if 'saturday' in allowed[cmd]:date_value(q['saturday'])
    if 'operation_id' in allowed[cmd]:hexid(q['operation_id'])
    if 'job_id' in allowed[cmd]:hexid(q['job_id'])
    if cmd=='entry_history':integer(q['limit'],1,8);hexid(q['after_event_id'],True)
    if cmd=='entry_prepare':q=clone(q);q['request']=validate('Request',q['request'],p)
    return {k:q[k] for k in FIELDS['EntryRequest']}

def wire_response(q,view,p,field,value):
    r=dict(id=q['id'],ok=True,view=clone(view),store_id=store_id(p),snapshot=None,status=None,history=None,job=None,operation=None,error='');r[field]=clone(value);return r

def validate_response(r,p):
    exact(r,FIELDS['EntryResponse']);integer(r['id'],1);req(type(r['ok']) is bool);payloads=('snapshot','status','history','job','operation')
    if not r['ok']:
        req(r['view'] is None and r['store_id']=='' and all(r[k] is None for k in payloads) and r['error'] in ERRORS);return {k:r[k] for k in FIELDS['EntryResponse']}
    req(r['error']=='' and sum(r[k] is not None for k in payloads)==1)
    if r['snapshot'] is not None and r['snapshot']['state']=='unavailable':
        req(r['view'] is None and r['store_id']=='');req(r['snapshot']==dict(version=1,state='unavailable',error=r['snapshot']['error'],view=None,store_id='',pending_operation_id='',active_job_id='',queued_job_id='',last_job_id=''));req(r['snapshot']['error'] in ('',)+ERRORS)
    else:
        req(r['view'] is not None and r['store_id']==store_id(p));v=r['view'];expected=next(x['view'] for x in PUBLIC['records'] if x['fixture']==next(x['name'] for x in OLD['pins'] if x['pin']==p['stable_pin']))
        v=ordered_like(v,expected);hexid(v['view_id']);integer(v['generation'],1)
        req(v['account_authority_id']==p['stable_pin']['account_authority_id'] and v['account_id']==p['stable_pin']['account_id'] and v['authority_pin']==p['stable_pin']['source']);r=clone(r);r['view']=v
    for field,kind in [('status','EventStatus'),('history','HistoryPage'),('operation','OperationResult')]:
        if r[field] is not None:r[field]=validate(kind,r[field],p)
    if r['snapshot'] is not None and r['snapshot']['state']!='unavailable':
        s=r['snapshot'];exact(s,FIELDS['EntrySnapshot']);req(s['version']==1 and s['view']==r['view'] and s['store_id']==r['store_id']);req(s['state'] in ('idle','busy','failed'));req(s['error'] in ('',)+ERRORS)
        for k in ('pending_operation_id','active_job_id','queued_job_id','last_job_id'):hexid(s[k],True)
        req(not s['active_job_id'] or s['active_job_id']!=s['queued_job_id'])
    if r['job'] is not None:
        j=r['job'];exact(j,FIELDS['EntryJob']);req(j['version']==1 and j['view']==r['view'] and j['store_id']==r['store_id']);j={k:j[k] for k in FIELDS['EntryJob']};j['view']=clone(r['view']);r['job']=j;hexid(j['job_id']);hexid(j['operation_id'],True);req(j['kind'] in ('prepare','apply','check') and j['state'] in ('queued','running','complete','failed','cancelled'));req(j['event_id']==event(p,j['saturday'])['event_id']);req(j['error'] in ('',)+ERRORS)
        field,kind={'prepare':('intent','Intent'),'apply':('mutation','MutationResult'),'check':('check','CheckResult')}[j['kind']]
        for k in ('intent','mutation','check'):req((j[k] is not None)==(j['state']=='complete' and k==field))
        if j['state']=='complete':
            req(j['error']=='');j[field]=validate(kind,j[field],p)
            if j['kind']=='prepare':req(j['intent']['operation_id']==j['operation_id'] and j['intent']['request']['event_id']==j['event_id'] and j['intent']['view_id']==j['view']['view_id'] and j['intent']['generation']==j['view']['generation'])
            if j['kind']=='apply':req(j['mutation']['receipt']['operation_id']==j['operation_id'] and j['mutation']['receipt']['event']['event_id']==j['event_id'] and j['mutation']['receipt']['view_id']==j['view']['view_id'] and j['mutation']['receipt']['generation']==j['view']['generation'])
            if j['kind']=='check':req(j['check']['status']['event']['event_id']==j['event_id'])
        elif j['state'] in ('queued','running'):req(j['error']=='')
    return {k:r[k] for k in FIELDS['EntryResponse']}

BASE_VALIDATE=validate
def validate(kind,v,p):
    if kind=='EntryRequest':return validate_wire_request(v,p)
    if kind=='EntryResponse':return validate_response(v,p)
    return BASE_VALIDATE(kind,v,p)

def correlate(q,r,p,view,expected_job=None,terminal_digest=None):
    q=validate_wire_request(q,p);r=validate_response(r,p);req(r['id']==q['id'])
    if not r['ok']:return r
    if q['command']=='entry_snapshot' and r['snapshot'] is not None and r['snapshot']['state']=='unavailable':return r
    req(r['view']==view and r['store_id']==store_id(p));req(q['view_id'] in ('',view['view_id']))
    f={'entry_snapshot':'snapshot','entry_upcoming':'status','entry_status':'status','entry_history':'history','entry_prepare':'job','entry_apply':'job','entry_check':'job','entry_job':'job','entry_cancel':'job','entry_operation':'operation','entry_pending':'operation','entry_abandon':'operation'}[q['command']];req(r[f] is not None)
    if f=='operation':
        o=r[f]['operation'];req(o is not None or q['command']=='entry_pending')
        if o is not None and q['command']!='entry_pending':req(o['intent']['operation_id']==q['operation_id'])
    if f=='status':
        s=r[f]
        if q['command']=='entry_status':req(s['event']['saturday']==q['saturday'])
        else:req(s['event']['saturday']==upcoming(s['observed_at_ms']))
    if f=='history':req(r[f]['after_event_id']==q['after_event_id'] and len(r[f]['events'])<=q['limit'])
    if f=='job':
        j=r[f]
        if q['command'] in ('entry_job','entry_cancel'):req(j['job_id']==q['job_id'])
        else:
            req(j['state']=='queued' and j['kind']==q['command'].split('_')[1])
            if j['kind']=='prepare':req(j['event_id']==q['request']['event_id'] and j['operation_id']=='')
            if j['kind']=='check':req(j['saturday']==q['saturday'])
            if j['kind']=='apply':req(j['operation_id']==q['operation_id'])
        if expected_job:
            for k in ('job_id','kind','store_id','event_id','saturday','view'):req(j[k]==expected_job[k])
            if j['kind']=='apply':req(j['operation_id']==expected_job['operation_id'])
            if j['kind']=='prepare' and j['state']=='complete':req(j['intent']['request']==expected_job['request'])
        if terminal_digest: req(sha(canon(j))==terminal_digest)
    return r

def corpus():
    calendars=[]
    for i,s in enumerate(DATES):
        d=date_value(s);t=CUTOFFS[i];assert t==tail_cutoff(d)
        nextdate=DATES[i+1] if i+1<len(DATES) else None
        assert upcoming(t-1)==s
        for n in (t,t+1):
            try:got=upcoming(n)
            except Refusal:got=None
            assert got==nextdate
        calendars.append(dict(saturday=s,cutoff_ms=t,utc=(EPOCH+dt.timedelta(milliseconds=t)).strftime('%Y-%m-%dT%H:%M:%SZ'),utc_offset_seconds=int(dt.datetime(d.year,d.month,d.day,12,tzinfo=TZ).utcoffset().total_seconds()),since_previous_hours=(t-CUTOFFS[i-1])//3600000 if i else 0,upcoming_cutoff_minus_1=s,upcoming_cutoff_exact=nextdate,upcoming_cutoff_plus_1=nextdate))
    assert set(x['since_previous_hours'] for x in calendars)=={0,167,168,169}
    invalid_dates=['','2025-12-27','2100-01-02','2026-1-03','2026-01-3','2026-02-29','2028-02-29','2026-01-02','2026-01-04','2026-13-01','2026-00-01','2026-11-31','2026-11-21Z',' 2026-11-21','2026-11-21 ','２０２６-11-21','2026-11-21\x00','0000-01-01']
    for s in invalid_dates:
        try:cutoff(s);raise AssertionError(s)
        except Refusal:pass
    pins=[];events=[];scenarios=[];dtos=[];codecs=[];protocol=[];shapes=[]
    for oldpin in OLD['pins']:
        name=oldpin['name'];p=entry_pin(oldpin['pin']);sid=store_id(p);src=fixture_source(name);view=clone(next(x['view'] for x in PUBLIC['records'] if x['fixture']==name));ids=sorted(src);s='2026-11-21';t=cutoff(s);n=t-10000
        pins.append(dict(fixture=name,pin=p,canonical=canon(p).decode(),namespace_preimage=namespace_pre(oldpin['pin']).hex(),store_preimage=store_pre(p).hex(),store_id=sid))
        for ds in ('2026-01-03','2026-03-07','2026-03-14','2026-10-31','2026-11-07','2026-11-21','2028-02-26','2028-03-04','2099-12-26'):
            e=event(p,ds);events.append(dict(fixture=name,event=e,canonical=canon(e).decode(),event_preimage=event_pre(p,ds).hex(),entry_preimage=entry_pre(p,e).hex(),entry_id=entry_id(p,e)))
        op1=sha(('Set43 public operation '+name+' 1').encode());op2=sha(('Set43 public operation '+name+' 2').encode());op3=sha(('Set43 public operation '+name+' 3').encode());op4=sha(('Set43 public operation '+name+' 4').encode());q=request(p,s,ids);newview=clone(view);newview['view_id']=sha(b'Set43 relogin view'+raw(view['view_id']))
        model=State(p,src,view,n-1)
        steps=[dict(method='upcoming',now_ms=n),dict(method='status',saturday=s,now_ms=n),dict(method='prepare',request=q,random_id=op1,now_ms=n+1),dict(method='prepare',request=q,random_id=op2,now_ms=n+2),dict(method='receipt',operation_id=op1,now_ms=n+3),dict(method='apply',operation_id=op1,now_ms=n+4,final_now_ms=n+5),dict(method='apply',operation_id=op1,now_ms=n+6),dict(method='check',saturday=s,now_ms=n+7),dict(method='prepare',request=q,random_id=op2,now_ms=n+8),dict(method='prepare',request=request(p,s,ids,1),random_id=op2,now_ms=n+9),dict(method='apply',operation_id=op2,now_ms=n+10),dict(method='prepare',request=request(p,s,[],2,action='withdraw'),random_id=op3,now_ms=n+11),dict(method='apply',operation_id=op3,now_ms=n+12),dict(method='prepare',request=request(p,s,[],3,action='withdraw'),random_id=op4,now_ms=n+13),dict(method='apply',operation_id=op4,now_ms=n+14),dict(method='status',saturday=s,now_ms=t),dict(method='apply',operation_id=op1,now_ms=t+1),dict(method='operation',operation_id=op1,session=newview,now_ms=t+2),dict(method='apply',operation_id=op1,session=newview,now_ms=t+3),dict(method='history',limit=8,now_ms=t+4),dict(method='prepare',request=request(p,s,ids,4),random_id=op2,now_ms=t+5)]
        scenarios.append(run_steps(name+'/save-repeat-withdraw-close-historical',model,steps))
        committed=model.ops[op1];receipt=committed['receipt'];initial=clone(committed);initial.update(state='prepared',receipt=None)
        statmodel=State(p,src,view,n-1);statmodel.call(dict(method='prepare',request=q,random_id=op1,now_ms=n));statmodel.call(dict(method='apply',operation_id=op1,now_ms=n+1));st=statmodel.status(s);check=statmodel.call(dict(method='check',saturday=s,now_ms=n+2));history=statmodel.call(dict(method='history',limit=8,now_ms=n+2))
        examples={'Pin':p,'Event':event(p,s),'Request':q,'Intent':committed['intent'],'Member':receipt['members'][0],'Receipt':receipt,'Operation':committed,'MutationResult':dict(version=1,receipt=receipt,replayed=False),'OperationResult':dict(version=1,store_id=sid,operation=committed),'EventStatus':st,'CheckMember':check['members'][0],'CheckResult':check,'HistoryPage':history}
        for k,v in examples.items():
            validated=validate(k,clone(v),p);assert canon(validated)==canon(v),(k,'order');dtos.append(dict(fixture=name,kind=k,value=v,canonical=canon(v).decode(),canonical_sha256=sha(canon(v))));codecs+=codec_cases(k,v,p)
        dtos.append(dict(fixture=name,kind='ReceiptDigest',canonical_receipt=canon(receipt).decode(),preimage=receipt_pre(receipt).hex(),digest=receipt_digest(receipt)))
        marker=dict(application='slug-candidate-entries',version=1,store_id=sid,pin=p)
        metadata=dict(version=1,store_id=sid,pin=p,event_count=len(model.events),operation_count=len(model.ops),pending_operation_id=model.pending,last_operation_id=model.last,observed_checkpoint=model.checkpoint,observed_source_count=model.count,observed_roster_revision=model.rr,clock_ms=model.clock)
        for kind,val in [('Marker',marker),('Metadata',metadata),('EventRow',model.events[s])]:
            dtos.append(dict(fixture=name,kind=kind,value=val,canonical=canon(val).decode(),canonical_sha256=sha(canon(val)),disk_hex=(canon(val)+(b'\n' if kind=='Marker' else b'')).hex()));codecs+=codec_cases(kind,val,p)
        # Exact disk key/value preimages independent of a LevelDB implementation.
        disk_rows=[{'key_hex':b'meta'.hex(),'value_hex':canon(metadata).hex()}]
        disk_rows += [{'key_hex':('e:'+row['event']['event_id']).encode().hex(),'value_hex':canon(row).hex()} for row in model.events.values()]
        for oid,oprow in model.ops.items():
            disk_rows += [{'key_hex':('o:'+oid).encode().hex(),'value_hex':canon(oprow).hex()},{'key_hex':(b's:'+struct.pack('>Q',oprow['intent']['sequence'])).hex(),'value_hex':raw(oid).hex()}]
        dtos.append(dict(fixture=name,kind='DiskRows',rows=disk_rows))
        # Finite fault/temporal scenarios, never runtime execution evidence.
        def scenario(label,ss,source=None,start=n-1):scenarios.append(run_steps(name+'/'+label,State(p,source or src,view,start),ss))
        prepare=dict(method='prepare',request=q,random_id=op1,now_ms=n)
        scenario('prepare-crosses-cutoff',[dict(prepare,final_now_ms=t),dict(method='status',saturday=s,now_ms=t)])
        scenario('prepare-replay-after-cutoff',[prepare,dict(prepare,now_ms=t),dict(method='apply',operation_id=op1,now_ms=t+1),dict(method='abandon',operation_id=op1,session=newview,now_ms=t+2),dict(method='abandon',operation_id=op1,session=newview,now_ms=t+3)])
        scenario('apply-crosses-cutoff',[prepare,dict(method='apply',operation_id=op1,now_ms=t-1,final_now_ms=t),dict(method='receipt',operation_id=op1,now_ms=t+1)])
        scenario('clock-rollback-after-rejection',[prepare,dict(method='apply',operation_id=op1,now_ms=n+10,final_now_ms=n+20,source_digest_drift=True),dict(method='status',saturday=s,now_ms=n+19)])
        scenario('unknown-prepare-reopen-query',[dict(prepare,write_ack_unknown=True),dict(method='pending',now_ms=n+1),dict(method='reopen'),dict(method='pending',session=newview,now_ms=n+2),dict(method='apply',operation_id=op1,session=newview,now_ms=n+3),dict(method='abandon',operation_id=op1,session=newview,now_ms=n+4)])
        scenario('unknown-apply-reopen-replay',[prepare,dict(method='apply',operation_id=op1,now_ms=n+1,write_ack_unknown=True),dict(method='reopen'),dict(method='operation',operation_id=op1,session=newview,now_ms=n+2),dict(method='apply',operation_id=op1,now_ms=n+3)])
        scenario('postcommit-lease-unknown',[prepare,dict(method='apply',operation_id=op1,now_ms=n+1,postcommit_lease_error=True),dict(method='operation',operation_id=op1,now_ms=n+2)])
        scenario('precommit-cancel',[prepare,dict(method='apply',operation_id=op1,now_ms=n+1,cancel_before_commit=True),dict(method='pending',now_ms=n+2)])
        scenario('same-generation-new-view',[prepare,dict(method='apply',operation_id=op1,session=newview,now_ms=n+1),dict(method='pending',session=newview,now_ms=n+2),dict(method='abandon',operation_id=op1,session=newview,now_ms=n+3)])
        scenario('held-login-permitted-mining-denied',[dict(prepare,held=True,session_type='login'),dict(method='apply',operation_id=op1,now_ms=n+1,session_type='mining'),dict(method='apply',operation_id=op1,now_ms=n+2,held=True,session_type='login')])
        scenario('roster-change-final-fence',[prepare,dict(method='apply',operation_id=op1,now_ms=n+1,roster_changes_after_proof=True)])
        unrostered=clone(src);unrostered[ids[0]]['slot']=0
        scenario('not-rostered', [prepare,dict(method='apply',operation_id=op1,now_ms=n+1)],unrostered)
        missing=request(p,s,[sha(b'unconsumed public ID')]);scenario('unconsumed',[dict(prepare,request=missing),dict(method='apply',operation_id=op1,now_ms=n+1)])
        for field,value in [('gym_id','0'*64),('descriptor_id','0'*64)]:
            altered=clone(src);altered[ids[0]]['assignment'][field]=value
            scenario('source-'+field+'-mismatch',[prepare,dict(method='apply',operation_id=op1,now_ms=n+1)],altered)
        for pid,source_member in src.items():
            rt=source_member['assignment']['born_at_ms']+(source_member['manifest']['dna']['prime_weeks']+source_member['manifest']['dna']['decline_weeks'])*WEEK
            rs=upcoming(rt);rq=request(p,rs,[pid]);rprepare=dict(method='prepare',request=rq,random_id=op1,now_ms=rt-2)
            scenario('retirement-before-final-'+pid[:8],[rprepare,dict(method='apply',operation_id=op1,now_ms=rt-1,final_now_ms=rt)],start=rt-3)
            scenario('retirement-last-ms-'+pid[:8],[rprepare,dict(method='apply',operation_id=op1,now_ms=rt-1)],start=rt-3)
        scenario('source-highwater-regression',[dict(method='status',saturday=s,now_ms=n,source_observation=dict(checkpoint=3,count=4,roster_revision=3)),dict(method='status',saturday=s,now_ms=n+1,source_observation=dict(checkpoint=3,count=3,roster_revision=3))])
        # Check's warnings do not rewrite saved receipt, and retirement dominates off-roster.
        retirement=max(x['assignment']['born_at_ms']+(x['manifest']['dna']['prime_weeks']+x['manifest']['dna']['decline_weeks'])*WEEK for x in src.values())
        scenario('check-offroster-retirement-preserves-history',[prepare,dict(method='apply',operation_id=op1,now_ms=n+1),dict(method='set_slots',slots={pid:0 for pid in ids},roster_revision=4),dict(method='check',saturday=s,now_ms=n+2),dict(method='check',saturday=s,now_ms=retirement-1,final_now_ms=retirement),dict(method='receipt',operation_id=op1,now_ms=retirement+1)])
        # Date-order history with arbitrary operation order and exact retained cursors.
        dates=[DATES[100+i] for i in (8,0,7,1,6,2,5,3,4)]
        histnow=cutoff(DATES[99])-100;hm=State(p,src,view,histnow);hsteps=[]
        for i,ds in enumerate(dates):
            oi=sha(('history '+name+str(i)).encode());hsteps.extend([dict(method='prepare',request=request(p,ds,[],0,action='withdraw'),random_id=oi,now_ms=histnow+2*i),dict(method='abandon',operation_id=oi,now_ms=histnow+2*i+1)])
        hsteps.extend([dict(method='history',limit=8,now_ms=histnow+30),dict(method='history',limit=8,after_event_id=event(p,DATES[107])['event_id'],now_ms=histnow+31),dict(method='history',limit=8,after_event_id=event(p,DATES[108])['event_id'],now_ms=histnow+32),dict(method='history',limit=8,after_event_id=sha(b'unknown cursor'),now_ms=histnow+33)])
        scenarios.append(run_steps(name+'/history-nine-events',hm,hsteps))
        # Five-class semantic stubs are explicitly separate from the genuine source cases.
        synth=fake_five(p);sm=State(p,synth,view,n-1);sq=request(p,s,list(synth),roster_revision=5);sop=sha(b'synthetic five class'+raw(sid));sc=run_steps(name+'/synthetic-five-class',[sm][0],[dict(method='prepare',request=sq,random_id=sop,now_ms=n),dict(method='apply',operation_id=sop,now_ms=n+1),dict(method='check',saturday=s,now_ms=n+2)],'synthetic_structural_state_only_no_source_proofs');scenarios.append(sc)
        # A duplicate class from a distinct independent manifest is an eligibility error.
        first_class=next(iter(synth.values()))['manifest']['physical']['weight_class']
        existing_seeds={z['assignment']['work_hash'] for z in synth.values()}
        extra=next(x for x in MC['cases'] if x['manifest']['physical']['weight_class']==first_class and x['seed'] not in existing_seeds)
        ss=clone(synth);sample=clone(next(z for z in ss.values() if z['manifest']['physical']['weight_class']==first_class));sample['manifest']=clone(extra['manifest']);sample['assignment'].update(prospect_id=sha(b'Set43 duplicate class'+raw(extra['seed'])),work_hash=extra['seed'],manifest_digest=extra['digest'],portrait_digest=next(x['svg_digest'] for x in PORTRAITS['cases'] if x['seed']==extra['seed']),source_record_digest=sha(b'Set43 duplicate class source'+raw(extra['seed'])),sequence=6);sample['slot']=6;ss[sample['assignment']['prospect_id']]=sample
        two=sorted([sample['assignment']['prospect_id'],next(k for k,z in synth.items() if z['manifest']['physical']['weight_class']==first_class)])
        dc=State(p,ss,view,n-1);dq=request(p,s,two,roster_revision=6)
        scenarios.append(run_steps(name+'/synthetic-duplicate-class',dc,[dict(method='prepare',request=dq,random_id=sop,now_ms=n),dict(method='apply',operation_id=sop,now_ms=n+1)],'synthetic_structural_state_only_no_source_proofs'))
        sr=sm.ops[sop]['receipt'];scheck=sc['steps'][-1]['expected_result'];soprow=sm.ops[sop]
        for kind,val in [('Receipt',sr),('Operation',soprow),('CheckResult',scheck)]:dtos.append(dict(fixture=name,kind=kind,source_kind='synthetic_structural_state_only_no_source_proofs',value=val,canonical=canon(val).decode(),canonical_sha256=sha(canon(val))))
        # Full nested negative shoulder_jitter is a required positive transport control.
        assert any(m['item']['manifest']['portrait']['shoulder_jitter']<0 for m in scheck['members'])
        j=dict(version=1,job_id=sha(b'entry job'+raw(sid)),kind='check',state='complete',view=clone(view),store_id=sid,event_id=event(p,s)['event_id'],saturday=s,operation_id='',error='',intent=None,mutation=None,check=scheck)
        query=wire_request('entry_job',view['view_id'],job_id=j['job_id']);response=wire_response(query,view,p,'job',j)
        shapes.append(dict(fixture=name,source_kind='synthetic_structural_state_only_no_source_proofs',member_max=max(len(canon(m)) for m in sr['members']),receipt=len(canon(sr)),operation=len(canon(soprow)),check_member_max=max(len(canon(m)) for m in scheck['members']),check=len(canon(scheck)),full_response=len(canon(response)),depth=container_depth(response)))
        for k,val in [('EntryRequest',query),('EntryResponse',response)]:dtos.append(dict(fixture=name,kind=k,value=val,canonical=canon(val).decode(),canonical_sha256=sha(canon(val))));codecs+=codec_cases(k,val,p)
        def pc(label,qq,rr,accepted,expected=None,digest=None):
            try:correlate(qq,rr,p,view,expected,digest);ok=True
            except (Refusal,KeyError,TypeError,ValueError):ok=False
            assert ok==accepted,(label,ok)
            protocol.append(dict(name=name+'/'+label,pin=p,current_view=view,request=qq,response=rr,expected_job=expected,terminal_digest=digest,accepted=accepted))
        pc('nested-negative-jitter',query,response,True,j,sha(canon(j)))
        for kind in ('prepare','apply'):
            done=dict(version=1,job_id=sha(('completed-'+kind+name).encode()),kind=kind,state='complete',view=clone(view),store_id=sid,event_id=event(p,s)['event_id'],saturday=s,operation_id=op1,error='',intent=clone(committed['intent']) if kind=='prepare' else None,mutation=dict(version=1,receipt=clone(receipt),replayed=True) if kind=='apply' else None,check=None)
            dq=wire_request('entry_job',view['view_id'],job_id=done['job_id']);dr=wire_response(dq,view,p,'job',done);expected=clone(done)
            if kind=='prepare':expected['request']=clone(q)
            pc('completed-'+kind,dq,dr,True,expected,sha(canon(done)))
            bad=clone(dr)
            target=bad['job']['intent'] if kind=='prepare' else bad['job']['mutation']['receipt']
            target['view_id']=newview['view_id'];target['generation']=view['generation']+1
            pc('completed-'+kind+'-different-author',dq,bad,False,expected)

        for label,change in [('wrong-transport-id',lambda r:r.update(id=2)),('wrong-store',lambda r:r.update(store_id='0'*64)),('wrong-current-view',lambda r:r['view'].update(view_id='0'*64)),('wrong-job-id',lambda r:r['job'].update(job_id='0'*64)),('wrong-kind',lambda r:r['job'].update(kind='prepare'))]:
            r=clone(response);change(r);pc(label,query,r,False,j)
        # A coherent as-of change passes structural projection but cannot replace the first terminal result.
        drift=clone(response);ck=drift['job']['check'];ck['as_of_ms']+=1;ck['status']['observed_at_ms']+=1
        for m in ck['members']:it=m['item'];it['condition']=condition(it['assignment'],it['manifest'],ck['as_of_ms'])
        pc('terminal-coherent-time-rewrite',query,drift,False,j,sha(canon(j)))
        pending=wire_request('entry_pending',view['view_id']);pr=wire_response(pending,view,p,'operation',dict(version=1,store_id=sid,operation=None));pc('pending-null',pending,pr,True)
        oq=wire_request('entry_operation',view['view_id'],operation_id=op1);pc('operation-null',oq,dict(pr,id=oq['id']),False)
        histop=clone(committed);histop['intent']['view_id']=newview['view_id'];histop['receipt']['view_id']=newview['view_id'];hr=wire_response(oq,view,p,'operation',dict(version=1,store_id=sid,operation=histop));pc('historical-author-current-envelope',oq,hr,True)
        for cmd in ('entry_prepare','entry_apply','entry_check'):
            kind=cmd.split('_')[1];qwire=wire_request(cmd,view['view_id'],**({'request':q} if kind=='prepare' else {'operation_id':op1} if kind=='apply' else {'saturday':s}));ack=dict(version=1,job_id=sha(('ack'+kind+name).encode()),kind=kind,state='queued',view=view,store_id=sid,event_id=event(p,s)['event_id'],saturday=s,operation_id=op1 if kind=='apply' else '',error='',intent=None,mutation=None,check=None);pc(kind+'-queued-ack',qwire,wire_response(qwire,view,p,'job',ack),True)
        # Every command's allowed/unused fields; correlation is independently checked above.
        for cmd in ('entry_snapshot','entry_upcoming','entry_status','entry_history','entry_prepare','entry_apply','entry_check','entry_job','entry_cancel','entry_operation','entry_pending','entry_abandon'):
            kw={}
            if cmd in ('entry_status','entry_check'):kw['saturday']=s
            if cmd=='entry_history':kw['limit']=8
            if cmd=='entry_prepare':kw['request']=q
            if cmd in ('entry_apply','entry_operation','entry_abandon'):kw['operation_id']=op1
            if cmd in ('entry_job','entry_cancel'):kw['job_id']=j['job_id']
            wq=wire_request(cmd,'' if cmd=='entry_snapshot' else view['view_id'],**kw)
            codecs+=codec_cases('EntryRequest',wq,p)
            for field in ('saturday','operation_id','job_id','request','limit','after_event_id'):
                if wq[field] not in ('',0,None):continue
                bad=clone(wq);bad[field]={'saturday':s,'operation_id':op1,'job_id':j['job_id'],'request':q,'limit':1,'after_event_id':event(p,s)['event_id']}[field]
                # after_event_id is optional in history, so it is a valid extra bound cursor there.
                accepted=cmd=='entry_history' and field=='after_event_id'
                try:parsed=parse(canon(bad),'EntryRequest',p);ok=True
                except (Refusal,ValueError,TypeError):ok=False
                assert ok==accepted,(cmd,field)
                codecs.append(dict(name=name+'/'+cmd+'/field-'+field,kind='EntryRequest',pin=p,raw_hex=canon(bad).hex(),accepted=accepted,canonical=canon(parsed).decode() if accepted else ''))
        for err in ERRORS:
            er=dict(id=oq['id'],ok=False,view=None,store_id='',snapshot=None,status=None,history=None,job=None,operation=None,error=err);pc('error-'+err,oq,er,True)
    # Explicit typed request edge variants. No malformed input is used as state authority.
    p=pins[0]['pin'];base=next(x['value'] for x in dtos if x['fixture']=='zero256' and x['kind']=='Request')
    for label,mut in [('zero-selection',lambda x:x.update(prospect_ids=[])),('six-selection',lambda x:x.update(prospect_ids=sorted(sha(str(i).encode()) for i in range(6)))),('duplicate',lambda x:x.update(prospect_ids=[x['prospect_ids'][0]]*2)),('unsorted',lambda x:x.update(prospect_ids=list(reversed(x['prospect_ids'])))),('uppercase',lambda x:x.update(prospect_ids=[x['prospect_ids'][0].upper()])),('withdraw-has-selection',lambda x:x.update(action='withdraw')),('wrong-event',lambda x:x.update(event_id='0'*64)),('roster-negative',lambda x:x.update(expected_roster_revision=-1)),('unsafe-integer',lambda x:x.update(expected_revision=SAFE+1)),('null-array',lambda x:x.update(prospect_ids=None)),('unknown-action',lambda x:x.update(action='lock'))]:
        v=clone(base);mut(v)
        try:parse(canon(v),'Request',p);ok=True
        except (Refusal,AssertionError,KeyError,TypeError,ValueError):ok=False
        assert not ok,label;codecs.append(dict(name='Request/'+label,kind='Request',pin=p,raw_hex=canon(v).hex(),accepted=False,canonical=''))
    # Transport byte limits, lexical integer strictness and nested exception controls.
    for kind,base_value,cap in [('Request',base,8192),('EntryResponse',next(x['value'] for x in dtos if x['kind']=='EntryResponse' and x['fixture']=='zero256'),131072)]:
        b=canon(base_value)
        for extra,accepted in [(0,True),(1,False)]:
            padded=b+b' '*(cap-len(b)+extra)
            try:parsed=parse(padded,kind,p);ok=True
            except (Refusal,AssertionError,KeyError,TypeError,ValueError):ok=False
            assert ok==accepted
            codecs.append(dict(name=kind+'/exact-cap-plus-'+str(extra),kind=kind,pin=p,raw_hex=padded.hex(),accepted=accepted,canonical=canon(parsed).decode() if accepted else ''))
    full=next(x['value'] for x in dtos if x['kind']=='EntryResponse' and x['fixture']=='zero256')
    for label,fn in [('too-many-members',lambda x:x['job']['check']['members'].append(clone(x['job']['check']['members'][0]))),('negative-slot',lambda x:x['job']['check']['members'][0].update(slot=-1)),('jitter-outside-range',lambda x:x['job']['check']['members'][0]['item']['manifest']['portrait'].update(shoulder_jitter=-7)),('date-overflow',lambda x:x['job']['check'].update(as_of_ms=DATE_MAX+1)),('untrusted-XP',lambda x:x['job']['check']['members'][0]['item']['condition'].update(pro_xp=1))]:
        b=clone(full);fn(b)
        try:parse(canon(b),'EntryResponse',p);ok=True
        except (Refusal,AssertionError,KeyError,TypeError,ValueError):ok=False
        assert not ok,label
        codecs.append(dict(name='EntryResponse/'+label,kind='EntryResponse',pin=p,raw_hex=canon(b).hex(),accepted=False,canonical=''))
    for name,k,v in [('withdraw','Request',request(p,'2026-11-21',[],action='withdraw')),('no-pending','OperationResult',dict(version=1,store_id=store_id(p),operation=None))]:dtos.append(dict(fixture='zero256',name=name,kind=k,value=v,canonical=canon(v).decode(),canonical_sha256=sha(canon(v))))
    # Exact retirement boundary from immutable retained DNA, projected independently.
    retirement=[]
    for x in OLD['cases']:
        a=x['assignment'];m=x['manifest'];t=a['born_at_ms']+(m['dna']['prime_weeks']+m['dna']['decline_weeks'])*WEEK
        for offset in (-1,0,1):retirement.append(dict(fixture=x['fixture'],prospect_id=a['prospect_id'],as_of_ms=t+offset,expected_condition=condition(a,m,t+offset),eligible_if_rostered=offset<0))
    capacities=[]
    for events_count,ops_count,new_event in [(0,0,True),(255,4095,True),(256,4095,False),(256,4095,True),(255,4096,False),(256,4096,False)]:capacities.append(dict(event_count=events_count,operation_count=ops_count,new_event=new_event,new_prepare_allowed=ops_count<4096 and (not new_event or events_count<256),existing_terminal_replay_requires_capacity=False,prepared_completion_requires_capacity=False,closure_requires_capacity=False,abandon_requires_capacity=False))
    assert pins[0]['pin']['namespace_id']==pins[1]['pin']['namespace_id'] and pins[0]['store_id']!=pins[1]['store_id']
    return dict(version=1,candidate_fixture_only=True,contract_sha256=CONTRACT,independent_of_go=True,source_proof_verification='Six retained assignments inherit prior independently checked normal-work evidence; no new Argon performed. Synthetic vectors have no admitted source receipt.',calendar=dict(tzif_sha256=TZSHA,tzif_bytes=1294,posix_tail=TAIL,calendar_preimage=CAL_PRE.hex(),calendar_id=CALENDAR_ID,rules_preimage=RULE_PRE.hex(),rules_id=RULES_ID,all_supported_saturdays=calendars,invalid_dates=[dict(input=x,error='entry_calendar') for x in invalid_dates],upcoming_bounds=[dict(now_ms=1,date=DATES[0]),dict(now_ms=CUTOFFS[-1]-1,date=DATES[-1]),dict(now_ms=CUTOFFS[-1],error='entry_calendar'),dict(now_ms=DATE_MAX,error='entry_calendar')]),pins=pins,events=events,dtos=dtos,codec_cases=codecs,protocol_cases=protocol,scenarios=scenarios,retirement=retirement,capacity_cases=capacities,shapes=shapes,conservative_bounds=dict(member=3712,receipt=22528,operation=32256,event_row=1504,history=33792,check_member=8320,check=55808,full_response=64000,parent_v3_marker_cap=16384,logical_journal_upper_bound=136*1024*1024),coverage_limits=['No actual filesystem, LevelDB crash, process cancellation, account lease, provider, mining, GUI or runtime execution is claimed.','State source facts are semantic fixtures, not public trusted-selection capabilities.','The six retained genuine source records cover middle/light_heavy/heavy only. Five-class cases are explicitly synthetic structural/state expectations.','Ordinary codec and correlation refusals do not establish provenance; production parsing must additionally enforce its complete compiled validators.','Calendar law is pinned candidate interpretation, not a prediction of future civil-time legislation.'])

def container_depth(x):
    if isinstance(x,dict):return 1+max([container_depth(v) for v in x.values()]+[0])
    if isinstance(x,list):return 1+max([container_depth(v) for v in x]+[0])
    return 0

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--check',action='store_true');args=ap.parse_args();f=corpus();b=pretty(f);path=HERE/'entry_v1.json'
    if args.check:req(path.read_bytes()==b)
    else:path.write_bytes(b)
    print(json.dumps(dict(status='passed',check=args.check,corpus_sha256=sha(b),bytes=len(b),saturdays=len(f['calendar']['all_supported_saturdays']),pins=len(f['pins']),events=len(f['events']),dto_cases=len(f['dtos']),codec_cases=len(f['codec_cases']),protocol_cases=len(f['protocol_cases']),scenarios=len(f['scenarios']),state_steps=sum(len(s['steps']) for s in f['scenarios']),retirement_cases=len(f['retirement']),shapes=f['shapes']),sort_keys=True))
if __name__=='__main__':main()
