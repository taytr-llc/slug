"""Independent finite History wire grammar and request-aware correlations.
Structural acceptance is not sealed authority or omitted-prefix completeness.
Core arithmetic/immutable-source predicates are supplied by common.validate.
"""
import common as C
ERRORS={'invalid_request','unauthorized','forbidden','stale_view','busy','outcome_unknown','career_inactive','history_unavailable','history_corrupt','history_clock','history_capacity','history_not_found','history_source_changed','history_cancelled','history_observation_unknown'}
TERMINAL=ERRORS-{'outcome_unknown','busy'}
UNAVAILABLE={'history_unavailable','career_inactive','history_clock','history_corrupt'}
COMMANDS={'career_history_'+x for x in ['snapshot','begin','job','cancel','page']}
FIELDS={
'CareerHistoryRequest':'id version command view_id prospect_id job_id after limit',
'CareerHistorySnapshot':'version state error view store_id activation active_job_id queued_job_id last_job_id',
'CareerHistoryJob':'version job_id state view store_id activation_id prospect_id error capture',
'CareerHistoryPageResult':'view store_id job_id page',
'CareerHistoryResponse':'id version ok snapshot job page error'}
CAPS={'CareerHistoryRequest':1023,'CareerHistoryResponse':36863,'CareerHistorySnapshot':4096,'CareerHistoryJob':12288}

def need(value,code='invalid'):C.require(value,code)
def integer(value,lo=0,hi=9007199254740991):
    need(type(value)is int and lo<=value<=hi);return value
def ID(value):
    need(type(value)is str and len(value)==64 and all(c in '0123456789abcdef'for c in value));return value
def empty_or_id(value):
    if value!='':ID(value)
    need(type(value)is str);return value
def closed(kind,value):
    keys=FIELDS[kind].split();need(type(value)is dict and set(value)==set(keys));return {k:C.clone(value[k])for k in keys}
def equal(a,b):
    if type(a)is not type(b):return False
    if type(a)is dict:return set(a)==set(b)and all(equal(a[k],b[k])for k in a)
    if type(a)is list:return len(a)==len(b)and all(equal(x,y)for x,y in zip(a,b))
    return a==b
def bounded(value,cap):need(len(C.J(value))<=cap);return value
def view(value):return bounded(C.validate('View',value),1024)
def activation(value):return bounded(C.validate('Activation',value),512)
def bind_context(value,context):
    for key in ['view','store_id','activation']:
        if key in context and key in value:need(equal(value[key],context[key]),'correlation')
    return value

def validate(kind,value,**context):
    if kind not in FIELDS:return C.validate(kind,value,**context)
    v=closed(kind,value)
    if 'version'in v:integer(v['version'],1,1)
    if kind=='CareerHistoryRequest':
        integer(v['id'],1);need(type(v['command'])is str and v['command']in COMMANDS)
        for k in ['view_id','prospect_id','job_id']:empty_or_id(v[k])
        integer(v['after'],0,256);integer(v['limit'],0,8)
        cmd=v['command'].removeprefix('career_history_')
        if cmd=='snapshot':need(v['view_id']==v['prospect_id']==v['job_id']=='' and v['after']==v['limit']==0)
        else:
            ID(v['view_id'])
            if cmd=='begin':ID(v['prospect_id']);need(v['job_id']==''and v['after']==v['limit']==0)
            elif cmd in ['job','cancel']:ID(v['job_id']);need(v['prospect_id']==''and v['after']==v['limit']==0)
            else:ID(v['job_id']);need(v['prospect_id']==''and 1<=v['limit']<=8)
    elif kind=='CareerHistorySnapshot':
        need(type(v['state'])is str and v['state']in {'unavailable','idle','busy','ready','failed','cancelled'})
        need(type(v['error'])is str)
        for k in ['store_id','active_job_id','queued_job_id','last_job_id']:empty_or_id(v[k])
        active,queued,last=[bool(v[k])for k in ['active_job_id','queued_job_id','last_job_id']]
        if v['state']=='unavailable':
            need(v['view']is None and v['activation']is None and v['store_id']==''and not(active or queued or last)and v['error']in UNAVAILABLE)
        else:
            v['view']=view(v['view']);v['activation']=activation(v['activation']);ID(v['store_id'])
            if v['state']=='idle':need(not(active or queued or last)and v['error']=='')
            elif v['state']=='busy':need(active!=queued and not last and v['error']=='')
            elif v['state']=='ready':need(last and not(active or queued)and v['error']=='')
            elif v['state']=='cancelled':need(last and not(active or queued)and v['error']=='history_cancelled')
            else:need(last and not(active or queued)and v['error']in TERMINAL-{'history_cancelled'})
            bind_context(v,context)
    elif kind=='CareerHistoryJob':
        for k in ['job_id','store_id','activation_id','prospect_id']:ID(v[k])
        v['view']=view(v['view']);need(type(v['state'])is str and v['state']in {'queued','running','done','cancelled','failed'})
        need(type(v['error'])is str)
        if v['state']=='done':
            need(v['capture']is not None and v['error']=='')
            v['capture']=C.validate('CareerHistoryCapture',v['capture'],view=v['view'])
            bounded(v['capture'],8192)
            cap=v['capture'];need(cap['status']['store_id']==v['store_id']and cap['activation']['activation_id']==v['activation_id']and cap['assignment']['prospect_id']==v['prospect_id'],'correlation')
        else:
            need(v['capture']is None)
            if v['state']in ['queued','running']:need(v['error']=='')
            elif v['state']=='cancelled':need(v['error']=='history_cancelled')
            else:need(v['error']in TERMINAL-{'history_cancelled'})
            bounded(v,2048)
        bind_context(v,context)
        if 'activation'in context:need(v['activation_id']==context['activation']['activation_id'],'correlation')
        if 'known_job'in context:
            old=validate('CareerHistoryJob',context['known_job'])
            for key in ['job_id','view','store_id','activation_id','prospect_id']:need(equal(v[key],old[key]),'correlation')
            if old['state']in ['done','failed','cancelled']:need(equal(v,old),'terminal_changed')
    elif kind=='CareerHistoryPageResult':
        v['view']=view(v['view']);ID(v['store_id']);ID(v['job_id'])
        options={'view':v['view']}
        if 'limit'in context:options['limit']=context['limit']
        v['page']=C.validate('CareerHistoryPage',v['page'],**options);bounded(v['page'],32768)
        need(v['page']['capture']['status']['store_id']==v['store_id'],'correlation');bind_context(v,context)
        if 'selected_job'in context:
            job=validate('CareerHistoryJob',context['selected_job'])
            need(job['state']=='done'and v['job_id']==job['job_id']and equal(v['view'],job['view'])and v['store_id']==job['store_id']and equal(v['page']['capture'],job['capture']),'correlation')
    else:
        integer(v['id'],1);need(type(v['ok'])is bool and type(v['error'])is str)
        arms=[k for k in ['snapshot','job','page']if v[k]is not None]
        if not v['ok']:need(not arms and v['error']in ERRORS)
        else:
            need(len(arms)==1 and v['error']=='');arm=arms[0]
            sub={'snapshot':'CareerHistorySnapshot','job':'CareerHistoryJob','page':'CareerHistoryPageResult'}[arm]
            v[arm]=validate(sub,v[arm],**context)
    if kind in CAPS:bounded(v,CAPS[kind])
    return v

def decode(kind,raw,**context):
    need(type(raw)is bytes)
    cap=CAPS.get(kind,36863);depth=1 if kind=='CareerHistoryRequest'else 12
    v=C.strict_parse(raw,cap,depth)
    return validate(kind,v,**context)

def exchange(request,response,**context):
    """Post-native-admission exchange correlation only.
    Unknown job/cancel lookup discovery is a separate native guard, modeled in
    state_models.native_lookup; valid helper JSON alone never admits that lookup.
    """
    q=validate('CareerHistoryRequest',request);r=validate('CareerHistoryResponse',response,**context)
    need(q['id']==r['id'],'correlation')
    if not r['ok']:return r
    cmd=q['command'].removeprefix('career_history_')
    arm='snapshot'if cmd=='snapshot'else 'page'if cmd=='page'else 'job'
    need(r[arm]is not None and all(r[k]is None for k in ['snapshot','job','page']if k!=arm),'correlation')
    if cmd=='snapshot':return r
    value=r[arm];need(value['view']['view_id']==q['view_id'],'correlation')
    if cmd=='begin':need(value['state']=='queued'and value['prospect_id']==q['prospect_id'],'correlation')
    else:need(value['job_id']==q['job_id'],'correlation')
    if cmd=='page':
        need('selected_job'in context,'missing_captured_context')
        need(value['page']['after']==q['after'],'correlation')
        p=value['page'];remaining=p['capture']['event_count']-q['after'];need(remaining>=0)
        need(len(p['items'])==min(q['limit'],remaining),'correlation')
        # Explicit request-aware rule even when the standalone parser only knows8.
        validate('CareerHistoryPageResult',value,**dict(context,limit=q['limit']))
    return r
