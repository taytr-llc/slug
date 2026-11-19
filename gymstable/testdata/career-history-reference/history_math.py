"""Set51 independent history equations and closed core grammar.
Historical public facts are input, not new output authority. No Go, engine,
Argon, signature or chain implementation is imported or executed.
"""
import datetime,re
from functools import lru_cache
from common import *
POLICY='{"version":1,"scope":"two_fixture_standard_career","source":"result_v1_first_finality_after_activation","xp":"D42_standard_1_cap15","base_points":"D41_no_quality","effective_time":"first_finality_observed_at_ms","cutoff":"strict_before","later_conflict":"retain","c1":"zero_professional_record_only"}'
POLICY_ID=sha(b'SLUG candidate career policy\0v1\0'+lp(POLICY.encode()))
METHODS=('decision','ko','tko_referee','tko_three_knockdowns','tko_corner','decision_draw','double_corner_stoppage')
DRAWS=('decision_draw','double_corner_stoppage')
# Installed candidate restrictions are inherited from pinned ValidatePin and
# stableViewValid. Public input files are read once, never during transport.
ID_FIELDS='network genesis profile root_public_key gym_public_key gym_id'
FIXTURE_IDENTITIES=tuple(J(fields(load(n+'-status.json')['pin']['identity'],ID_FIELDS)) for n in ('winner','loser'))
_FIXED_AUTH=load('winner-view.json')['authority_pin']
CANDIDATE_AUTH=(_FIXED_AUTH['network'],_FIXED_AUTH['genesis'],_FIXED_AUTH['descriptor_id'])
del _FIXED_AUTH
EVENT_FIELDS='version award_id result_key card_id bout_id source_record_digest input_id result_digest finality_digest effective_at_ms saturday corner_a corner_b winner method'
TOTAL_FIELDS='wins losses draws xp base_points'
CAPTURE_FIELDS='version status activation career_digest as_of_ms assignment prefix_count event_count totals history_id'
ROW_FIELDS='ordinal event earned xp_before xp_applied totals_after'

def activation_id(a):return sha(b'SLUG candidate career activation\0v1\0'+raw(a['policy_id'])+raw(a['result_store_id'])+u64(a['result_created_at_ms'])+u64(a['source_created_at_ms'])+u64(a['activated_at_ms']))
def award_id(a,key):return sha(b'SLUG candidate career award\0v1\0'+raw(a['activation_id'])+raw(key))
def finality_digest(f):return sha(b'SLUG candidate career finality\0v1\0'+lp(J(f)))
def stable_id(pin):return sha(b'SLUG candidate stable\0v1\0'+lp(J(pin)))
def trust_id(origin):return sha(b'SLUG manifest transport trust\0v1\0'+lp(origin.encode())+b'\1'+u32(0))
def zero():return {k:0 for k in TOTAL_FIELDS.split()}
def delta(e,pid):
    require(pid in (e['corner_a']['prospect_id'],e['corner_b']['prospect_id']))
    win=e['winner']==pid;draw=e['winner']==''
    return dict(wins=int(win),losses=int(not win and not draw),draws=int(draw),xp=1,base_points=4 if draw else (10 if e['method']=='decision' else 14) if win else 0)
def apply(t,d):
    x={k:t[k]+d[k] for k in TOTAL_FIELDS.split()};x['xp']=min(15,x['xp']);return x

def activation(v):
    v=fields(v,'version policy_id activation_id result_store_id result_created_at_ms source_created_at_ms activated_at_ms')
    require(type(v['version']) is int and v['version']==1 and v['policy_id']==POLICY_ID and ID(v['result_store_id']))
    require(all(stamp(v[k]) for k in ('source_created_at_ms','result_created_at_ms','activated_at_ms')))
    require(v['source_created_at_ms']<=v['result_created_at_ms']<=v['activated_at_ms'] and v['activation_id']==activation_id(v));return bounded(v,512)
def authority_pin(v):
    v=fields(v,'version network genesis authority_id descriptor_id');network,genesis,descriptor=CANDIDATE_AUTH
    require(type(v['version']) is int and v['version']==5 and type(v['network']) is int and v['network']==network and v['genesis']==genesis and v['descriptor_id']==descriptor and ID(v['authority_id']));return v

def view(v):
    v=fields(v,'version view_id account_authority_id account_id generation authority_pin authority_origin transport_trust_id')
    require(type(v['version']) is int and v['version']==1 and integer(v['generation'],1))
    require(all(ID(v[k]) for k in ('view_id','account_authority_id','account_id','transport_trust_id')))
    v['authority_pin']=authority_pin(v['authority_pin']);require(v['authority_pin']['authority_id']!=v['account_authority_id'])
    origin=v['authority_origin'];require(type(origin) is str and re.fullmatch(r'http://127\.0\.0\.1:[1-9][0-9]{0,4}',origin) is not None)
    require(1<=int(origin.rsplit(':',1)[1])<=65535 and v['transport_trust_id']==trust_id(origin));return bounded(v,1024)
def pin(v):
    v=fields(v,'version account_authority_id account_id identity source condition_version projector_id')
    require(type(v['version']) is int and v['version']==1 and all(ID(v[k]) for k in ('account_authority_id','account_id')))
    v['identity']=fields(v['identity'],ID_FIELDS)
    require(J(v['identity']) in FIXTURE_IDENTITIES and type(v['condition_version']) is int and v['condition_version']==1 and v['projector_id']=='slug-fighter-condition-candidate-v1')
    v['identity']=fields(v['identity'],'network genesis profile root_public_key gym_public_key gym_id')
    v['source']=authority_pin(v['source']);require(v['source']['authority_id']!=v['account_authority_id'] and v['source']['network']==v['identity']['network'] and v['source']['genesis']==v['identity']['genesis']);return bounded(v,4096)
def status(v,view_context=None):
    v=fields(v,'version store_id pin checkpoint observed_source_count caught_up roster_revision slots operation_count pending_operation_id last_operation_id last_as_of_ms');v['pin']=pin(v['pin'])
    require(type(v['version']) is int and v['version']==1 and v['store_id']==stable_id(v['pin']))
    require(integer(v['checkpoint'],0,1000) and integer(v['observed_source_count'],v['checkpoint'],1000) and type(v['caught_up']) is bool and v['caught_up']==(v['checkpoint']==v['observed_source_count']))
    require(integer(v['operation_count'],0,4096) and integer(v['roster_revision'],0,v['operation_count']) and integer(v['last_as_of_ms'],0,MAX_TIME))
    slots=v['slots'];require(type(slots) is list and len(slots)==10 and all(x=='' or ID(x) for x in slots));require(len([x for x in slots if x])==len(set(x for x in slots if x)))
    for k in ('pending_operation_id','last_operation_id'):require(v[k]=='' or ID(v[k]))
    require((v['operation_count']==0)==(v['last_operation_id']=='') and (not v['pending_operation_id'] or v['pending_operation_id']==v['last_operation_id']))
    if view_context is not None:
        vv=view(view_context);p=v['pin'];require(p['account_id']==vv['account_id'] and p['account_authority_id']==vv['account_authority_id'] and p['source']==vv['authority_pin'])
    return bounded(v,4096)
def assignment(v,status_context=None):
    v=fields(v,'version sequence prospect_id gym_id work_hash born_at_ms accepted_at_ms descriptor_id manifest_digest portrait_digest source_record_digest')
    require(type(v['version']) is int and v['version']==1 and integer(v['sequence'],1,1000))
    require(all(ID(v[k]) for k in ('prospect_id','gym_id','work_hash','descriptor_id','manifest_digest','portrait_digest','source_record_digest')))
    require(stamp(v['born_at_ms']) and stamp(v['accepted_at_ms']) and 0<=v['accepted_at_ms']-v['born_at_ms']<300000)
    if status_context is not None:
        s=status_context;require(v['sequence']<=s['checkpoint'] and v['gym_id']==s['pin']['identity']['gym_id'] and v['descriptor_id']==s['pin']['source']['descriptor_id'])
    return bounded(v,2048)
def totals(v,count=None):
    v=fields(v,TOTAL_FIELDS);require(all(integer(v[k],0,256) for k in ('wins','losses','draws')))
    n=v['wins']+v['losses']+v['draws'];require(n<=256 and integer(v['xp'],0,15) and v['xp']==min(15,n))
    require(integer(v['base_points'],10*v['wins']+4*v['draws'],14*v['wins']+4*v['draws']) and (v['base_points']-10*v['wins']-4*v['draws'])%4==0)
    if count is not None:require(n==count)
    return bounded(v,128)
def event(v,a=None):
    v=fields(v,EVENT_FIELDS);require(type(v['version']) is int and v['version']==1)
    require(all(ID(v[k]) for k in EVENT_FIELDS.split()[1:9]) and stamp(v['effective_at_ms']))
    require(type(v['saturday']) is str and re.fullmatch('[0-9]{4}-[0-9]{2}-[0-9]{2}',v['saturday']) is not None)
    try:date=datetime.date.fromisoformat(v['saturday']);require(date.weekday()==5 and date.isoformat()==v['saturday'])
    except ValueError as e:raise Refusal() from e
    for k in ('corner_a','corner_b'):
        v[k]=fields(v[k],'prospect_id account_id gym_id');require(all(ID(x) for x in v[k].values()))
    require(all(v['corner_a'][k]!=v['corner_b'][k] for k in ('prospect_id','account_id','gym_id')))
    require(v['method'] in METHODS and (v['winner']=='' if v['method'] in DRAWS else v['winner'] in (v['corner_a']['prospect_id'],v['corner_b']['prospect_id'])))
    if a is not None:require(v['effective_at_ms']>=a['activated_at_ms'] and v['award_id']==award_id(a,v['result_key']))
    return bounded(v,1344)
def history_id(c):
    return sha(b'SLUG candidate fighter history\0v1\0'+raw(c['status']['store_id'])+raw(c['activation']['activation_id'])+raw(c['career_digest'])+lp(J(c['assignment']))+u64(c['as_of_ms'])+u32(c['prefix_count'])+u32(c['event_count'])+lp(J(c['totals'])))
def capture(v,view_context=None):
    v=fields(v,CAPTURE_FIELDS);require(type(v['version']) is int and v['version']==1)
    v['status']=status(v['status'],view_context);v['activation']=activation(v['activation']);v['assignment']=assignment(v['assignment'],v['status'])
    require(ID(v['career_digest']) and stamp(v['as_of_ms']) and v['as_of_ms']==v['status']['last_as_of_ms'] and v['as_of_ms']>=max(v['activation']['activated_at_ms'],v['assignment']['accepted_at_ms']))
    require(integer(v['prefix_count'],0,256) and integer(v['event_count'],0,v['prefix_count']));v['totals']=totals(v['totals'],v['event_count'])
    require(v['history_id']==history_id(v));return bounded(v,8192)
def history_row(v,c):
    v=fields(v,ROW_FIELDS);require(integer(v['ordinal'],1,c['event_count']));n=v['ordinal'];v['event']=event(v['event'],c['activation']);e=v['event'];pid=c['assignment']['prospect_id']
    require(e['effective_at_ms']<=c['as_of_ms']);corners=[e[k] for k in ('corner_a','corner_b') if e[k]['prospect_id']==pid];require(len(corners)==1)
    require(corners[0]['account_id']==c['status']['pin']['account_id'] and corners[0]['gym_id']==c['assignment']['gym_id'])
    v['earned']=fields(v['earned'],TOTAL_FIELDS);expected=delta(e,pid);require(all(type(x) is int for x in v['earned'].values()) and v['earned']==expected)
    require(integer(v['xp_before'],0,15) and v['xp_before']==min(15,n-1) and integer(v['xp_applied'],0,1) and v['xp_applied']==min(15,n)-v['xp_before'])
    v['totals_after']=totals(v['totals_after'],n)
    prior={k:v['totals_after'][k]-v['earned'][k] for k in ('wins','losses','draws','base_points')};prior['xp']=v['xp_before'];totals(prior,n-1)
    if n==c['event_count']:require(v['totals_after']==c['totals'])
    return bounded(v,1536)
def page(v,view_context=None,limit=None):
    v=fields(v,'version capture after next has_more items');require(type(v['version']) is int and v['version']==1);v['capture']=capture(v['capture'],view_context);c=v['capture']
    require(integer(v['after'],0,c['event_count']) and type(v['items']) is list and len(v['items'])<=8)
    remain=c['event_count']-v['after'];require((len(v['items'])==0)==(remain==0) and len(v['items'])<=remain)
    if limit is not None:require(integer(limit,1,8) and len(v['items'])==min(limit,remain))
    require(integer(v['next'],v['after'],c['event_count']) and v['next']==v['after']+len(v['items']) and type(v['has_more']) is bool and v['has_more']==(v['next']<c['event_count']))
    v['items']=[history_row(x,c) for x in v['items']]
    for key in ('award_id','result_key'):require(len({x['event'][key] for x in v['items']})==len(v['items']))
    for i,x in enumerate(v['items']):
        require(x['ordinal']==c['event_count']-v['after']-i)
        if i:
            newer=v['items'][i-1];require((x['event']['effective_at_ms'],x['event']['award_id'])<(newer['event']['effective_at_ms'],newer['event']['award_id']))
            require(apply(x['totals_after'],newer['earned'])==newer['totals_after'])
    return bounded(v,32768)
def validate_core(kind,v,**ctx):
    v=clone(v)
    if kind=='Activation':return activation(v)
    if kind=='View':return view(v)
    if kind=='Status':return status(v,ctx.get('view'))
    if kind=='Assignment':return assignment(v,ctx.get('status'))
    if kind=='Event':return event(v,ctx.get('activation'))
    if kind=='CareerHistoryCapture':return capture(v,ctx.get('view'))
    if kind=='CareerHistoryRow':return history_row(v,ctx['capture'])
    if kind=='CareerHistoryPage':return page(v,ctx.get('view'),ctx.get('limit'))
    raise Refusal('unknown_kind')

def derive_event(a,record):
    require(record['finality'] is not None);f=record['finality'];v=record['statement']['statement']['source'];s=v['source'];corners=s['corners'];require(len(corners)==2)
    cs=[dict(prospect_id=x['member']['assignment']['prospect_id'],account_id=x['owner']['account_id'],gym_id=x['owner']['gym_id']) for x in corners]
    e=dict(version=1,award_id=award_id(a,record['result_key']),result_key=record['result_key'],card_id=s['card_id'],bout_id=s['bout']['bout_id'],source_record_digest=v['record_digest'],input_id=v['input_id'],result_digest=v['result_digest'],finality_digest=finality_digest(f),effective_at_ms=f['observed_at_ms'],saturday=s['event']['saturday'],corner_a=cs[0],corner_b=cs[1],winner=v['summary']['winner'],method=v['summary']['method'])
    return event(e,a)
def derive_capture(a,events,st,assign,asof,clock):
    a=activation(clone(a));require(stamp(clock) and clock>=a['activated_at_ms'] and stamp(asof) and asof>=a['activated_at_ms'])
    require(type(events) is list and len(events)<=256,'career_corrupt')
    es=[event(clone(e),a) for e in events];require(all(e['effective_at_ms']<=clock for e in es),'career_corrupt')
    require(len({e['award_id'] for e in es})==len(es) and len({e['result_key'] for e in es})==len(es),'career_corrupt')
    es=sorted(es,key=lambda e:(e['effective_at_ms'],e['award_id']));require(len(J(es))<=544*1024,'career_corrupt');es=[e for e in es if e['effective_at_ms']<=asof]
    pid=assign['prospect_id'];pre=b'SLUG candidate career prefix\0v1\0'+raw(a['activation_id'])+u64(asof)+lp(b'current')+u32(1)+raw(pid)+u32(len(es))+b''.join(lp(J(e)) for e in es)
    total=zero();rows=[]
    for e in es:
        if pid not in (e['corner_a']['prospect_id'],e['corner_b']['prospect_id']):continue
        d=delta(e,pid);before=total['xp'];total=apply(total,d)
        rows.append(dict(ordinal=len(rows)+1,event=e,earned=d,xp_before=before,xp_applied=total['xp']-before,totals_after=clone(total)))
    st=clone(st);st['last_as_of_ms']=asof
    c=dict(version=1,status=st,activation=a,career_digest=sha(pre),as_of_ms=asof,assignment=clone(assign),prefix_count=len(es),event_count=len(rows),totals=total,history_id='')
    c['history_id']=history_id(c);capture(c)
    rows.reverse()
    for rr in rows:history_row(rr,c)
    return {'capture':c,'rows':rows,'prefix_preimage_hex':pre.hex(),'prefix_award_ids':[e['award_id'] for e in es]}
def derive_page(capture_data,after,limit):
    c=clone(capture_data['capture']);require(integer(after,0,c['event_count']) and integer(limit,1,8));items=clone(capture_data['rows'][after:after+limit]);n=after+len(items)
    return page(dict(version=1,capture=c,after=after,next=n,has_more=n<c['event_count'],items=items),limit=limit)
def model_event(a,template,n,at,method='decision',winner='a',unrelated=False):
    e=clone(template);e['result_key']=sha(('modeled-result-'+str(n)).encode());e['award_id']=award_id(a,e['result_key'])
    for k in ('card_id','bout_id','source_record_digest','input_id','result_digest','finality_digest'):e[k]=sha(('modeled-'+k+'-'+str(n)).encode())
    e['effective_at_ms']=at;e['method']=method
    if unrelated:
        e['corner_a']['prospect_id']=ident(100000+n);e['corner_b']['prospect_id']=ident(200000+n)
    e['winner']='' if method in DRAWS else e['corner_'+winner]['prospect_id'];return event(e,a)
@lru_cache(maxsize=1)
def _examples():
    a=load('career-activation.json');real=derive_event(a,load('result-finalized.json'));require(derive_event(a,load('result-conflict.json'))==real)
    prior=load('independent-career.json');require(real==prior['event'] and J(real).decode()==prior['event_canonical_json'])
    views={'zero256':view(load('winner-view.json')),'incrementing256':view(load('loser-view.json'))}
    st=load('winner-status.json');assign=load('winner-assignment.json');at=a['activated_at_ms'];c={}
    c['empty']=derive_capture(a,[],st,assign,at,at)
    c['genuine_win']=derive_capture(a,[real],st,assign,at,at)
    c['genuine_loss']=derive_capture(a,[real],load('loser-status.json'),load('loser-assignment.json'),at,at)
    c['capacity256']=derive_capture(a,[model_event(a,real,n,at+n+1,METHODS[n%7],('a','b')[n%2]) for n in range(256)],st,assign,at+256,at+256)
    c['mixed']=derive_capture(a,[model_event(a,real,1000+n,at+n+1,METHODS[n%7],('a','b')[n%2],n%3==0) for n in range(24)],st,assign,at+24,at+24)
    pages={name:derive_page(x,0,8) for name,x in c.items()}
    pages['capacity256_second']=derive_page(c['capacity256'],8,8);pages['capacity256_late']=derive_page(c['capacity256'],248,8);pages['capacity256_exhausted']=derive_page(c['capacity256'],256,8)
    return dict(activation=activation(a),views=views,event=real,captures={n:x['capture'] for n,x in c.items()},histories=c,pages=pages)
def examples():return clone(_examples())
