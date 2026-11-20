"""Independent owned-consumer byte model; no product imports or live authority.

Only an approved generator imports this source. Appearance/condition and core
signature primitives are independently frozen older Python inputs.
"""
import copy
import hashlib
import json
import re
import struct

VERSION = 1
MAX_TIME = 253402300799999
MAX_SAFE = 9007199254740991
FIELDS = {
    'pin': 'version stable ownership',
    'status': 'version store_id pin head roster_revision slots operation_count pending_operation_id last_operation_id last_as_of_ms',
    'career': 'activation career_digest totals current_gym_wins',
    'item': 'birth admission ownership manifest condition career',
    'page': 'version status head as_of_ms after next_after has_more items',
    'request': 'expected_revision slot prospect_id expected_state_id head',
    'intent': 'version sequence operation_id generation view_id request prepared_at_ms',
    'receipt': 'version operation_id generation view_id request previous_prospect_id changed previous_revision revision slots committed_at_ms',
    'operation': 'version intent state receipt abandoned_at_ms',
    'mutation': 'version receipt replayed',
    'history': 'version status activation career_digest as_of_ms birth ownership prefix_count event_count totals current_gym_wins history_id',
    'history_page': 'version capture after next has_more items',
    'entry_pin': 'version namespace_id calendar_id rules_id stable_store_id stable_pin',
    'entry_member': 'slot birth ownership class condition_at_save',
    'entry_receipt': 'version store_id entry_id event operation_id operation_sequence generation view_id action previous_revision revision roster_revision ownership_head committed_at_ms members',
    'cutoff_entry': 'version store_id event entry_id committed_count last_sequence history_digest latest_receipt',
    'event_pin': 'version calendar_id rules_id ownership participants',
    'member': 'slot slot_assigned_at_ms birth ownership class condition_at_cutoff eligibility',
    'excluded': 'prospect_id state_id owner_public_key reason',
    'owner': 'ownership_basis account_authority_id account_id gym_id',
    'participant': 'version participant_id entry_store_id owner roster entry members excluded',
    'snapshot': 'version event cutoff participants',
    'capture_evidence': 'version participant_id stable_fence entry_fence cutoff_id captured_at_ms',
    'snapshot_receipt': 'version input snapshot_digest captures completed_at_ms',
}
CAPS = dict(pin=16384,status=24576,item=90112,page=753664,request=4096,
            intent=8192,receipt=12288,operation=16384,mutation=16384,
            history=32768,history_page=65536,entry_pin=32768,
            entry_member=6144,entry_receipt=49152,cutoff_entry=53248,
            event_pin=65536,member=6144,excluded=1024,owner=1024,
            participant=106496,snapshot=229376,capture_evidence=4096,
            snapshot_receipt=237568)

class Refused(ValueError):
    pass

def need(v, msg):
    if not v:
        raise Refused(msg)

def J(v):
    return json.dumps(v, ensure_ascii=True, separators=(',', ':'), allow_nan=False).encode('ascii')

def own(v):
    return copy.deepcopy(v)

def entry_namespace(stable):
    identity=stable['identity']
    integer(identity['network'],0,0xffffffff)
    ident(identity['genesis']); ident(stable['account_authority_id'])
    return hashlib.sha256(b'SLUG local candidate entry namespace\0v1\0'+struct.pack('<I',identity['network'])+bytes.fromhex(identity['genesis'])+bytes.fromhex(stable['account_authority_id'])).hexdigest()

def H(label, v):
    b = J(v)
    return hashlib.sha256(b'SLUG owned ' + label.encode('ascii') + b'\0v1\0' + struct.pack('<I', len(b)) + b).hexdigest()

def integer(v, lo=0, hi=MAX_SAFE):
    need(type(v) is int and lo <= v <= hi, 'integer range/type')
    return v

def ident(v, optional=False, size=64):
    need(type(v) is str and ((optional and v == '') or re.fullmatch('[0-9a-f]{%d}' % size, v)), 'identifier')
    return v

def exact(kind, v):
    names = FIELDS[kind].split()
    need(type(v) is dict and set(v) == set(names), 'closed ' + kind)
    return {k: own(v[k]) for k in names}

def same(a,b):
    return J(a) == J(b)

def no_duplicates(rows):
    out = {}
    for key,val in rows:
        need(key not in out,'duplicate key')
        out[key] = val
    return out

def parse(raw, kind, validate):
    need(type(raw) is bytes and 0 < len(raw) <= CAPS[kind], 'raw size')
    def bad(_):
        raise Refused('noninteger number')
    try:
        value = json.loads(raw, object_pairs_hook=no_duplicates, parse_float=bad, parse_constant=bad)
    except (ValueError,UnicodeError) as e:
        raise Refused('JSON') from e
    out = validate(value)
    need(J(out) == raw,'canonical encoding/order')
    return out

class Model:
    def __init__(self, core_module, core, appearance, condition, recipe):
        self.CM, self.core, self.A, self.C, self.recipe = core_module,core,appearance,condition,recipe

    def pin(self,v):
        v=exact('pin',v); integer(v['version'],1,1)
        p=v['stable']; need(type(p) is dict and p['version']==2,'shared stable pin')
        self.core.pin_id(v['ownership'])
        need(same(p['source'],v['ownership']['source']),'source pin')
        need(p['condition_version']==1 and p['projector_id']=='slug-fighter-condition-candidate-v1','projector')
        ident(p['account_authority_id']); ident(p['account_id']); ident(p['identity']['gym_id'])
        ident(p['identity']['gym_public_key'],size=66)
        need(p['identity']['network']==p['source']['network'] and p['identity']['genesis']==p['source']['genesis'],'chain')
        return v

    def state(self,p,admission,state):
        s=self.CM.ordered('state',state); aid=self.core.validate_admission(p['ownership'],admission)
        need(s['version']==1 and s['pin_id']==self.core.pin_id(p['ownership']),'state pin')
        need(s['birth_admission_id']==aid and s['prospect_id']==admission['record']['verified']['prospect_id'],'birth link')
        integer(s['generation'],0,4096); ident(s['owner_public_key'],size=66)
        kind='initial' if s['generation']==0 else 'final'
        e=self.CM.ordered(kind,s['evidence'])
        need(e['pin_id']==s['pin_id'] and e['prospect_id']==s['prospect_id'] and e['owner_public_key']==s['owner_public_key'],'state evidence')
        if s['generation']==0:
            need(e['admission_id']==aid and s['owner_public_key']==admission['record']['authorization']['public_key'],'initial owner')
        else:
            for field in ('prior_state_id','transfer_id','checkpoint_publication_id','sixtieth_descendant_hash'):
                ident(e[field])
            integer(e['sixtieth_descendant_height'],60,2**31-1)
        need(s['state_id']==self.CM.digest('SLUG fighter ownership state',e).hex(),'state digest')
        s['evidence']=e
        return s

    def head(self,p,h):
        h=self.CM.ordered('head',h); integer(h['version'],1,1)
        need(h['pin_id']==self.core.pin_id(p['ownership']),'head pin')
        integer(h['admission_sequence'],0,1000); integer(h['publication_sequence'],0,16384); integer(h['chain_height'],0,2**31-1)
        for k in ('admission_head','publication_head','chain_hash','replay_digest','finalized_prefix_id'):ident(h[k])
        ident(h['conflict_id'],True); return h

    def status(self,v):
        v=exact('status',v); integer(v['version'],1,1); v['pin']=self.pin(v['pin'])
        need(v['store_id']==H('stable store',v['pin']),'store id'); v['head']=self.head(v['pin'],v['head'])
        integer(v['roster_revision']); integer(v['operation_count'],0,4096); integer(v['last_as_of_ms'],0,MAX_TIME)
        need(type(v['slots']) is list and len(v['slots'])==10,'ten slots')
        for x in v['slots']:ident(x,True)
        used=[x for x in v['slots'] if x]; need(len(used)==len(set(used)),'duplicate slot')
        ident(v['pending_operation_id'],True); ident(v['last_operation_id'],True)
        need(v['roster_revision']<=v['operation_count'],'revision count')
        return v

    def request(self,p,v):
        v=exact('request',v); integer(v['expected_revision']); integer(v['slot'],1,10)
        ident(v['prospect_id'],True); ident(v['expected_state_id'],True)
        need(bool(v['prospect_id'])==bool(v['expected_state_id']),'clear state')
        v['head']=self.head(p,v['head']); return v

    def operation(self,p,v):
        p=self.pin(p); v=exact('operation',v); integer(v['version'],1,1)
        i=exact('intent',v['intent']); integer(i['version'],1,1); integer(i['sequence'],1,4096)
        ident(i['operation_id']); integer(i['generation'],1,MAX_SAFE); ident(i['view_id'])
        i['request']=self.request(p,i['request']); integer(i['prepared_at_ms'],1,MAX_TIME); v['intent']=i
        need(v['state'] in ('prepared','committed','abandoned'),'operation state')
        if v['state']=='committed':
            r=exact('receipt',v['receipt']); integer(r['version'],1,1)
            for k in ('operation_id','generation','view_id','request'):need(same(i[k],r[k]),'receipt intent')
            r['request']=self.request(p,r['request']); ident(r['previous_prospect_id'],True)
            need(type(r['changed']) is bool,'changed bool'); need(r['changed']==(r['previous_prospect_id']!=i['request']['prospect_id']),'changed meaning')
            need(type(r['slots']) is list and len(r['slots'])==10,'receipt slots')
            for x in r['slots']:ident(x,True)
            need(len([x for x in r['slots'] if x])==len(set(x for x in r['slots'] if x)),'receipt duplicate')
            need(r['slots'][i['request']['slot']-1]==i['request']['prospect_id'],'applied slot')
            integer(r['previous_revision']); integer(r['revision']); integer(r['committed_at_ms'],i['prepared_at_ms'],MAX_TIME)
            need(r['previous_revision']==i['request']['expected_revision'] and r['revision']==r['previous_revision']+int(r['changed']),'receipt revision')
            need(v['abandoned_at_ms']==0,'committed abandoned'); v['receipt']=r
        else:
            need(v['receipt'] is None,'noncommit receipt')
            if v['state']=='prepared':need(type(v['abandoned_at_ms']) is int and v['abandoned_at_ms']==0,'prepared abandoned')
            else:integer(v['abandoned_at_ms'],i['prepared_at_ms'],MAX_TIME)
        return v

    def birth(self,admission):
        r=admission['record']; b=r['birth']; s=r['static_binding']; v=r['verified']
        raw=J(r); digest=hashlib.sha256(b'SLUG stable source record\0v1\0'+struct.pack('<I',len(raw))+raw).hexdigest()
        return dict(version=1,sequence=r['sequence'],prospect_id=v['prospect_id'],gym_id=v['gym_id'],work_hash=v['work_hash'],born_at_ms=b['born_at_ms'],accepted_at_ms=b['accepted_at_ms'],descriptor_id=s['descriptor_id'],manifest_digest=s['manifest_digest'],portrait_digest=s['portrait_digest'],source_record_digest=digest)

    def item(self,p,v,asof):
        v=exact('item',v); p=self.pin(p); integer(asof,1,MAX_TIME)
        v['ownership']=self.state(p,v['admission'],v['ownership'])
        need(v['ownership']['owner_public_key']==p['stable']['identity']['gym_public_key'],'viewer current owner')
        need(same(v['birth'],self.birth(v['admission'])),'immutable birth')
        r=v['admission']['record']; mv=1 if p['stable']['source']['descriptor_id']!=hashlib.sha256(self.A.DESCRIPTOR_DOMAIN+J(self.A.APPROVAL['descriptor'])).hexdigest() else 2
        manifest=self.A.V1.generate(r['verified']['work_hash']) if mv==1 else self.A.generate(r['verified']['work_hash'])
        need(same(v['manifest'],manifest),'manifest')
        c=exact('career',v['career']); integer(c['totals']['xp'],0,15); integer(c['current_gym_wins'],0,c['totals']['wins']); ident(c['career_digest'])
        need(same(v['condition'],self.C.project(1,manifest['dna'],v['birth']['born_at_ms'],asof,c['totals']['xp'])),'condition/career')
        need(v['birth']['accepted_at_ms']<=asof,'asof'); v['career']=c
        need(len(J(v))<=CAPS['item'],'item bytes'); return v

    def page(self,v):
        v=exact('page',v); integer(v['version'],1,1); v['status']=self.status(v['status'])
        p=v['status']['pin']; v['head']=self.head(p,v['head']); need(same(v['head'],v['status']['head']),'page head')
        integer(v['as_of_ms'],1,MAX_TIME); integer(v['after'],0,1000); integer(v['next_after'],0,1000)
        need(type(v['has_more']) is bool and type(v['items']) is list and len(v['items'])<=8,'page bounds')
        v['items']=[self.item(p,x,v['as_of_ms']) for x in v['items']]
        seq=[x['birth']['sequence'] for x in v['items']]
        need(seq==sorted(set(seq)) and all(n>v['after'] for n in seq),'page original sequence')
        need(v['next_after']==(seq[-1] if seq else v['after']),'page cursor')
        need(not v['has_more'] or len(seq)>0,'empty continuation'); return v

    def history(self,v):
        v=exact('history',v); integer(v['version'],1,1); v['status']=self.status(v['status'])
        integer(v['as_of_ms'],1,MAX_TIME); integer(v['prefix_count'],0,256); integer(v['event_count'],0,v['prefix_count']); ident(v['career_digest'])
        need(v['ownership']['prospect_id']==v['birth']['prospect_id'] and v['ownership']['owner_public_key']==v['status']['pin']['stable']['identity']['gym_public_key'],'history current selection')
        ident(v['ownership']['state_id']); integer(v['totals']['xp'],0,15); integer(v['current_gym_wins'],0,v['totals']['wins'])
        body={k:own(x) for k,x in v.items() if k!='history_id'}
        need(v['history_id']==H('fighter history',body),'history id'); return v

    def history_page(self,v):
        v=exact('history_page',v); integer(v['version'],1,1); v['capture']=self.history(v['capture'])
        integer(v['after'],0,256); integer(v['next'],v['after'],256); need(type(v['has_more']) is bool,'history continuation')
        need(type(v['items']) is list and len(v['items'])<=8,'history size')
        pid=v['capture']['birth']['prospect_id']; last=v['after']
        for row in v['items']:
            need(row['ordinal']==last+1,'history ordinal'); last=row['ordinal']; e=row['event']
            corners=[e['corner_a'],e['corner_b']]; need(sum(c['prospect_id']==pid for c in corners)==1,'history selected fighter')
            # Deliberately no equality of historic corner gym/account to viewer/birth.
            mine=next(c for c in corners if c['prospect_id']==pid)
            outcome='draws' if e['winner']=='' else ('wins' if e['winner']==pid else 'losses')
            earned=dict(wins=0,losses=0,draws=0,xp=1,base_points=0); earned[outcome]=1
            if outcome=='wins':earned['base_points']=14 if e['method'] in ('ko','tko_referee','tko_three_knockdowns','tko_corner') else 10
            if outcome=='draws':earned['base_points']=4
            need(same(row['earned'],earned),'historical award')
            integer(row['xp_before'],0,15); need(row['xp_applied']==min(1,15-row['xp_before']),'xp cap')
        need(v['next']==last and v['has_more']==(last<v['capture']['event_count']),'history cursor'); return v

    def entry_pin(self,v):
        v=exact('entry_pin',v); integer(v['version'],1,1); ident(v['namespace_id']); ident(v['calendar_id']); ident(v['rules_id'])
        v['stable_pin']=self.pin(v['stable_pin']); need(v['namespace_id']==entry_namespace(v['stable_pin']['stable']),'entry namespace'); need(v['stable_store_id']==H('stable store',v['stable_pin']),'entry stable id'); return v

    def snapshot(self,pin,v):
        pin=exact('event_pin',pin); self.core.pin_id(pin['ownership'])
        v=exact('snapshot',v); integer(v['version'],1,1); ev=v['event']; cutoff=v['cutoff']
        need(cutoff['pin_id']==self.core.pin_id(pin['ownership']),'cutoff expected source')
        need(cutoff['event_id']==ev['event_id'] and cutoff['cutoff_ms']==ev['cutoff_ms'],'exact cutoff')
        self.core.verify('cutoff','SLUG fighter ownership cutoff statement',cutoff,cutoff['pin_id'])
        self.core.validate_boundary(cutoff['pin_id'],cutoff['boundary'])
        need(type(v['participants']) is list and len(v['participants'])==2,'participants')
        gymids=[]; physical=[]
        for n,p in enumerate(v['participants']):
            p=exact('participant',p); v['participants'][n]=p; integer(p['version'],1,1)
            p['owner']=exact('owner',p['owner']); need(p['owner']['ownership_basis']=='finalized_public_chain','ownership basis'); gymids.append(p['owner']['gym_id'])
            need(p['roster']['cutoff_ms']==ev['cutoff_ms'] and p['entry']['event']['cutoff_ms']==ev['cutoff_ms'],'local cutoff')
            need(type(p['members']) is list and type(p['excluded']) is list and len(p['members'])+len(p['excluded'])<=5,'members')
            chosen=[]
            for j,m in enumerate(p['members']):
                m=exact('member',m); p['members'][j]=m; integer(m['slot'],1,10); integer(m['slot_assigned_at_ms'],0,ev['cutoff_ms']-1)
                need(m['birth']['prospect_id']==m['ownership']['prospect_id'],'member identity'); chosen.append(m['birth']['prospect_id']); physical.append(m['birth']['prospect_id'])
                need(m['ownership']['pin_id']==cutoff['pin_id'],'member source')
                source=pin['ownership']['source']
                gym=hashlib.sha256(b'SLUG gym identity\0v1\0'+struct.pack('<I',source['network'])+bytes.fromhex(source['genesis'])+bytes.fromhex(m['ownership']['owner_public_key'])).hexdigest()
                need(gym==p['owner']['gym_id'],'cutoff effective owner')
                need(m['eligibility'] in ('eligible','retired','not_rostered'),'eligibility')
            for j,x in enumerate(p['excluded']):
                x=exact('excluded',x); p['excluded'][j]=x; ident(x['prospect_id']);ident(x['state_id']);ident(x['owner_public_key'],size=66)
                need(x['reason']=='not_owned_at_cutoff','exclusion reason')
                source=pin['ownership']['source']; other=hashlib.sha256(b'SLUG gym identity\0v1\0'+struct.pack('<I',source['network'])+bytes.fromhex(source['genesis'])+bytes.fromhex(x['owner_public_key'])).hexdigest()
                need(other!=p['owner']['gym_id'],'false exclusion');chosen.append(x['prospect_id'])
            need(len(chosen)==len(set(chosen)),'participant duplicates')
            saved=p['entry']['latest_receipt']
            need(saved is None or saved['action'] in ('replace','withdraw'),'closed entry action')
            expected=[] if saved is None or saved['action']=='withdraw' else [x['birth']['prospect_id'] for x in saved['members']]
            need(sorted(chosen)==sorted(expected),'complete partition')
        need(gymids==sorted(set(gymids)),'ordered gyms');need(len(physical)==len(set(physical)),'duplicate physical fighter')
        return v


def select_career_source(legacy_events,current_events):
    integer(legacy_events,0,256);integer(current_events,0,256)
    need(not (legacy_events and current_events),'owned_career_conflict')
    return 'legacy' if legacy_events else 'current'
