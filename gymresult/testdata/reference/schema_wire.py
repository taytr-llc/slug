"""Independent Set48 closed helper/native transport schema.

Nested core DTOs use schema_core's explicit transport context. This module
adds exact wrapper fields, LF-inclusive limits, original-view/command/tuple
correlation and private native Path grammar. It never invokes Go or an engine.
"""
import json
import re
from common import J, clone, sha, Refusal, SAFE, DATE_MAX, cutoff, INHERITED
from crypto_result import store_id, result_key, card_id_for_date, statement_id
import schema_core

FIELDS = {
    'ResultRequest': 'id command view_id saturday bout_id job_id path'.split(),
    'ResultResponse': 'id ok view store_id snapshot status record job error'.split(),
    'ResultSnapshot': 'version state error view store_id pin source_created_at_ms created_at_ms publisher_account_id node_state node_owner active_job_id queued_job_id last_job_id'.split(),
    'ResultJob': 'version job_id kind state view store_id card_id saturday bout_id result_key error result export'.split(),
}
CAPS = {'ResultRequest':8192,'ResultResponse':262144,
        'ResultSnapshot':49152,'ResultJob':245760}
ERRORS = set('invalid_request unauthorized forbidden stale_view busy outcome_unknown result_unavailable result_corrupt result_capacity result_clock result_calendar result_not_found result_not_prepared result_source_incomplete result_source_changed result_source_conflict result_node_offline result_waiting_for_funds result_scan_incomplete result_not_mature result_conflict result_cancelled result_proof_unavailable result_setup_incomplete invalid_frame'.split())
FATAL_SNAPSHOT = {'result_unavailable','result_corrupt','result_clock','result_source_changed'}
TERMINAL = {'completed','failed','cancelled'}
BEGIN = {'result_publish':'publish','result_reconcile':'reconcile',
         'result_attest':'attest','result_export':'export'}
MATRIX = {
    'result_snapshot':(),
    'result_status':('view_id','saturday','bout_id'),
    'result_record':('view_id','saturday','bout_id'),
    'result_publish':('view_id','saturday','bout_id'),
    'result_reconcile':('view_id','saturday','bout_id'),
    'result_attest':('view_id','saturday','bout_id'),
    'result_export':('view_id','saturday','bout_id','path'),
    'result_job':('view_id','job_id'),
    'result_cancel':('view_id','job_id'),
}


def req(ok, why='invalid_request'):
    if not ok:
        raise Refusal(why)


def integer(v, lo=0, hi=SAFE):
    req(type(v) is int and lo <= v <= hi)


def boolean(v):
    req(type(v) is bool)


def text(v, lo=0, hi=256, ascii_only=True):
    req(type(v) is str)
    try:
        b=v.encode('utf-8')
    except UnicodeEncodeError:
        raise Refusal('invalid_request')
    req(lo <= len(b) <= hi and (not ascii_only or v.isascii()))


def hx(v, empty=False):
    text(v,0,64)
    req((empty and v=='') or re.fullmatch('[0-9a-f]{64}',v) is not None)


def timestamp(v, zero=False):
    integer(v,0 if zero else 1,DATE_MAX)


def exact(v, fields):
    req(type(v) is dict and set(v)==set(fields))
    return {k:v[k] for k in fields}


def fields(kind,v):
    out=exact(v,FIELDS[kind])
    if 'version' in out:
        integer(out['version'],1,1)
    return out


def view(v):
    # Exact pre48 public fixture shapes, with original ViewID/generation variable.
    return INHERITED.view(v)


def date(v):
    text(v,10,10)
    try:
        cutoff(v)
    except (TypeError,ValueError,OverflowError,AssertionError):
        raise Refusal('invalid_request')
    return v


def native_path(v):
    text(v,1,1024,ascii_only=False)
    req(v.startswith('/') and not any(c in v for c in '\0\r\n\t'))
    # Filesystem/symlink/protected-root rules require the actual native helper.
    return v


def core(kind,v,p):
    return schema_core.validate(kind,v,p,context='transport')


def request(v):
    v=fields('ResultRequest',v)
    integer(v['id'],1)
    text(v['command'],1,32);req(v['command'] in MATRIX)
    used=MATRIX[v['command']]
    for k in ('view_id','saturday','bout_id','job_id','path'):
        if k not in used:
            req(type(v[k]) is str and v[k]=='')
        elif k=='saturday':
            date(v[k])
        elif k=='path':
            native_path(v[k])
        else:
            hx(v[k])
    return v


def snapshot(v,p):
    v=fields('ResultSnapshot',v)
    text(v['state']);req(v['state'] in ('ready','busy','setup_incomplete','unavailable','failed'))
    text(v['error']);text(v['node_state']);text(v['node_owner'])
    req(v['node_state'] in ('offline','starting','online'))
    req(v['node_owner'] in ('none','publication','mining'))
    req((v['node_state']=='offline' and v['node_owner']=='none') or
        (v['node_state']=='starting' and v['node_owner']=='publication') or
        (v['node_state']=='online' and v['node_owner'] in ('publication','mining')))
    for k in ('store_id','publisher_account_id','active_job_id','queued_job_id','last_job_id'):
        hx(v[k],True)
    timestamp(v['source_created_at_ms'],True);timestamp(v['created_at_ms'],True)
    if v['state'] in ('unavailable','setup_incomplete'):
        if v['state']=='unavailable':
            req(v['view'] is None and v['error']=='result_unavailable')
        else:
            v['view']=view(v['view']);req(v['error']=='result_setup_incomplete')
        req(v['pin'] is None and v['store_id']==v['publisher_account_id']==v['active_job_id']==v['queued_job_id']==v['last_job_id']=='')
        req(v['created_at_ms']==v['source_created_at_ms']==0 and v['node_state']=='offline')
        req(len(J(v))<=CAPS['ResultSnapshot'])
        return v
    v['view']=view(v['view']);v['pin']=core('Pin',v['pin'],p)
    req(v['pin']==p and v['store_id']==store_id(p))
    req(v['publisher_account_id']==p['publisher_account_id'])
    timestamp(v['source_created_at_ms']);timestamp(v['created_at_ms'])
    req(v['source_created_at_ms'] <= v['created_at_ms'])
    ids=[v[k] for k in ('active_job_id','queued_job_id','last_job_id') if v[k]]
    req(len(ids)==len(set(ids)))
    req(not(v['active_job_id'] and v['queued_job_id']))
    if v['active_job_id'] or v['queued_job_id']:
        req(v['state']=='busy')
    if v['state']=='failed':
        req(v['error'] in FATAL_SNAPSHOT and not v['active_job_id'] and not v['queued_job_id'])
    else:
        req(v['error']=='')
    if v['state']=='ready':
        req(not v['active_job_id'] and not v['queued_job_id'])
    req(len(J(v))<=CAPS['ResultSnapshot'])
    return v


def status_selection(st,date_value,bid,card,p):
    req(st['store_id']==store_id(p) and st['card_id']==card and st['bout_id']==bid)
    req(st['event']['saturday']==date_value)
    req(st['result_key']==result_key(p,card,bid))


def job(v,p):
    v=fields('ResultJob',v)
    for k in ('job_id','store_id','card_id','bout_id','result_key'):
        hx(v[k])
    text(v['kind']);req(v['kind'] in ('publish','reconcile','attest','export'))
    text(v['state']);req(v['state'] in ('queued','running','completed','failed','cancelled'))
    text(v['error']);v['view']=view(v['view']);date(v['saturday'])
    req(v['store_id']==store_id(p))
    req(v['card_id']==card_id_for_date(p,v['saturday']))
    req(v['result_key']==result_key(p,v['card_id'],v['bout_id']))
    if v['state'] in ('queued','running'):
        req(v['error']=='' and v['result'] is None and v['export'] is None)
    elif v['state']=='completed':
        req(v['error']=='')
        if v['kind']=='export':
            req(v['result'] is None and v['export'] is not None)
            v['export']=core('ProofSummary',v['export'],p)
            e=v['export'];req(e['result_key']==v['result_key'] and e['card_id']==v['card_id'] and e['bout_id']==v['bout_id'])
        else:
            req(v['result'] is not None and v['export'] is None)
            v['result']=core('ActionResult',v['result'],p)
            status_selection(v['result']['status'],v['saturday'],v['bout_id'],v['card_id'],p)
            row=v['result']['record']
            if v['kind']=='publish':
                req(row['publication'] is not None)
                req(row['publication']['state']=='submitted' or row['inclusion'] is not None)
            if v['kind']=='attest':req(row['finality'] is not None)
            if v['kind'] in ('publish','attest'):
                req(v['result']['status']['source']['saved']['usable'])
    else:
        req(v['result'] is None and v['export'] is None and v['error'] in ERRORS and v['error']!='invalid_frame')
        if v['state']=='cancelled':req(v['error']=='result_cancelled')
    req(len(J(v))<=CAPS['ResultJob'])
    return v


def response(v,p):
    v=fields('ResultResponse',v)
    integer(v['id']);boolean(v['ok']);text(v['error'])
    present=[k for k in ('snapshot','status','record','job') if v[k] is not None]
    if not v['ok']:
        req(v['view'] is None and v['store_id']=='' and not present and v['error'] in ERRORS)
        req(v['id']>0 or v['error']=='invalid_frame')
        return v
    req(v['id']>0 and v['error']=='' and len(present)==1)
    k=present[0]
    if k=='snapshot':
        v[k]=snapshot(v[k],p)
        req(v['view']==v[k]['view'] and v['store_id']==v[k]['store_id'])
        v['view']=clone(v[k]['view'])
        return v
    v['view']=view(v['view']);hx(v['store_id']);req(v['store_id']==store_id(p))
    if k=='job':
        v[k]=job(v[k],p);req(v[k]['view']==v['view'])
    elif k=='status':
        v[k]=core('Status',v[k],p);req(v[k]['store_id']==v['store_id'])
    else:
        v[k]=core('ReadResult',v[k],p);req(v[k]['record']['store_id']==v['store_id'])
    return v


def _arrays(kind):
    allowed={}
    def pin_at(pre):
        allowed[pre+'/source_pin/source_pin/source_pin/source_pin/participants']=(2,2)
    def row_at(pre):
        allowed[pre+'/statement/statement/source/source/corners']=(2,2)
        allowed[pre+'/equivocation/statement/source/source/corners']=(2,2)
        for field,hi in (('scan_restarts',16),('prior_inclusions',16),('conflicts',8)):
            allowed[pre+'/'+field]=(0,hi)
        for path in ('inclusion/anchor/merkle_branch','prior_inclusions/*/inclusion/anchor/merkle_branch','finality/inclusion/anchor/merkle_branch'):
            allowed[pre+'/'+path]=(0,20)
    if kind=='ResultSnapshot':pin_at('/pin')
    if kind=='ResultJob':row_at('/result/record')
    if kind=='ResultResponse':
        pin_at('/snapshot/pin');row_at('/record/record');row_at('/job/result/record')
    return allowed


def _walk(v,allowed,path='',depth=0):
    if type(v) in (dict,list):
        depth+=1;req(depth<=20)
    if type(v) is list:
        req(path in allowed)
        lo,hi=allowed[path];req(lo<=len(v)<=hi)
        for x in v:_walk(x,allowed,path+'/*',depth)
    elif type(v) is dict:
        for k,x in v.items():_walk(x,allowed,path+'/'+k,depth)


def validate(kind,v,p):
    req(kind in FIELDS)
    _walk(v,_arrays(kind))
    fn={'ResultRequest':lambda x:request(x),'ResultSnapshot':lambda x:snapshot(x,p),
        'ResultJob':lambda x:job(x,p),'ResultResponse':lambda x:response(x,p)}[kind]
    v=fn(clone(v))
    # APIs receive one line without its transport LF; account for that LF here.
    extra=1 if kind in ('ResultRequest','ResultResponse') else 0
    req(len(J(v))+extra<=CAPS[kind])
    return v


def _depth_before_decode(raw):
    depth=0;quoted=False;escaped=False
    for b in raw:
        if quoted:
            if escaped:escaped=False
            elif b==92:escaped=True
            elif b==34:quoted=False
        elif b==34:quoted=True
        elif b in (123,91):
            depth+=1;req(depth<=20)
        elif b in (125,93):
            depth-=1;req(depth>=0)
    req(depth==0 and not quoted)


def parse(kind,raw,p):
    req(kind in FIELDS and type(raw) is bytes)
    req(len(raw)+(1 if kind in ('ResultRequest','ResultResponse') else 0)<=CAPS[kind])
    _depth_before_decode(raw)
    def pairs(items):
        d={}
        for k,v in items:
            req(k not in d);d[k]=v
        return d
    def integer_token(s):
        req(s!='-0' and len(s.lstrip('-'))<=16)
        return int(s)
    def no_float(_):raise Refusal('invalid_request')
    try:
        v=json.loads(raw.decode('utf-8'),object_pairs_hook=pairs,
                     parse_int=integer_token,parse_float=no_float,parse_constant=no_float)
    except (UnicodeDecodeError,json.JSONDecodeError,RecursionError,OverflowError):
        raise Refusal('invalid_request')
    return validate(kind,v,p)


def correlate(q,r,p,current,source_created,created,expected_job=None,
              terminal_digest='',expected_source=None,known_publication=None):
    q=validate('ResultRequest',q,p);r=validate('ResultResponse',r,p)
    req(r['id']==q['id'])
    if current is None:
        # This arm advertises no authority and cannot bootstrap one from a reply.
        req(q['command']=='result_snapshot' and r['ok'] and
            r['snapshot'] is not None and r['snapshot']['state']=='unavailable')
        return r
    current=view(current)
    if q['command']!='result_snapshot':req(q['view_id']==current['view_id'])
    if not r['ok']:return r
    expected={'result_snapshot':'snapshot','result_status':'status','result_record':'record',
              'result_job':'job','result_cancel':'job',**{x:'job' for x in BEGIN}}[q['command']]
    req(r[expected] is not None)
    if expected=='snapshot':
        ss=r['snapshot']
        if ss['state']!='unavailable':req(ss['view']==current)
        if ss['pin'] is not None:
            req(ss['source_created_at_ms']==source_created and ss['created_at_ms']==created)
        return r
    req(r['view']==current and r['store_id']==store_id(p))
    payload=None
    if expected=='job':
        j=r['job'];req(j['view']==current)
        if q['command'] in BEGIN:
            req(j['state']=='queued' and j['kind']==BEGIN[q['command']])
            req(j['saturday']==q['saturday'] and j['bout_id']==q['bout_id'])
        else:req(j['job_id']==q['job_id'])
        if expected_job is not None:
            for field in ('job_id','kind','view','store_id','card_id','saturday','bout_id','result_key'):
                req(j[field]==expected_job[field])
            req(not(expected_job['state']=='running' and j['state']=='queued'))
            if expected_job['state'] in TERMINAL:req(J(j)==J(expected_job))
        if terminal_digest:req(sha(J(j))==terminal_digest)
        if j['result'] is not None:payload=j['result']
        if j['export'] is not None and expected_source:
            for field in ('source_record_digest','input_id','result_digest'):
                req(j['export'][field]==expected_source[field])
        if j['export'] is not None:
            req(j['export']['finalized_at_ms']==0 or j['export']['finalized_at_ms']>=created)
            if known_publication and known_publication['txid']:
                req(j['export']['txid']==known_publication['txid'])
            if known_publication and known_publication['finality'] is not None:
                req(j['export']['finalized_at_ms']==known_publication['finality']['observed_at_ms'])
    elif expected=='status':
        st=r['status'];status_selection(st,q['saturday'],q['bout_id'],card_id_for_date(p,q['saturday']),p)
        payload={'status':st}
    else:
        payload=r['record'];st=payload['status']
        status_selection(st,q['saturday'],q['bout_id'],card_id_for_date(p,q['saturday']),p)
    if payload:
        st=payload['status'];req(st['observed_at_ms']>=created)
        if st['exists']:
            req(st['prepared_at_ms']>=created)
            if expected_source:
                for field in ('source_record_digest','input_id','result_digest'):
                    req(st[field]==expected_source[field])
        if 'record' in payload:
            row=payload['record'];source=row['statement']['statement']['source']
            req(source['store_created_at_ms']==source_created)
            req(source['created_at_ms']>=source_created)
            if known_publication:
                # Immutable commitments captured for THIS request. An already
                # validated older terminal keeps its own captured context; the
                # selection model stops it replacing newer explicit evidence.
                # This is never a Pin learned from the current response.
                req(statement_id(row['statement']['statement'])==known_publication['statement_id'])
                pub=row['publication']
                if known_publication['txid']:
                    req(pub is not None and pub['txid']==known_publication['txid'] and pub['raw_tx']==known_publication['raw_tx'])
                if known_publication['checkpoint'] is not None:
                    req(row['checkpoint']==known_publication['checkpoint'])
                if known_publication['finality'] is not None:
                    req(row['finality']==known_publication['finality'])
        if known_publication and known_publication['txid']:
            req(st['txid']==known_publication['txid'])
        if known_publication and known_publication['checkpoint'] is not None:req(st['checkpoint_present'])
        if known_publication and known_publication['finality'] is not None:
            req(st['finalized_at_ms']==known_publication['finality']['observed_at_ms'])
    return r
