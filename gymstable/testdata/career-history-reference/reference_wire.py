"""Small independent wire/correlation corpus; no Go/engine/proof authority.
Core examples come from separately authored frozen-prose history arithmetic.
"""
import base64,copy,hashlib,json,pathlib
import common as C
import schema_wire as W

def change(value,path,replacement):
    out=C.clone(value);p=out
    for key in path[:-1]:p=p[key]
    p[path[-1]]=replacement;return out

def build_wire(examples=None):
    if examples is None:
        import history_math as H
        examples=H.examples()
    dtos=[];codecs=[];protocols=[];counter=0
    views=examples['views'];v=views['zero256'];a=examples['activation']
    captures=examples['captures']
    cap=max((x for x in captures.values()if x['status']['pin']['account_id']==v['account_id']),key=lambda x:x['event_count'])
    store=cap['status']['store_id'];pid=cap['assignment']['prospect_id'];jobid=C.ident(9001)
    authority=dict(view=v,store_id=store,activation=a)
    def job(state='done',capture=cap,error=''):
        return dict(version=1,job_id=jobid,state=state,view=C.clone(v),store_id=store,activation_id=a['activation_id'],prospect_id=pid,error=error,capture=C.clone(capture))
    def snapshot(state='ready',error='',active='',queued='',last=jobid):
        return dict(version=1,state=state,error=error,view=C.clone(v),store_id=store,activation=C.clone(a),active_job_id=active,queued_job_id=queued,last_job_id=last)
    def request(command,**kwargs):
        d=dict(id=1,version=1,command='career_history_'+command,view_id='',prospect_id='',job_id='',after=0,limit=0);d.update(kwargs);return d
    def response(arm=None,value=None,error=''):
        d=dict(id=1,version=1,ok=not bool(error),snapshot=None,job=None,page=None,error=error)
        if arm:d[arm]=C.clone(value)
        return d
    def dto(name,kind,value):
        val=W.validate(kind,value);raw=C.J(val);W.decode(kind,raw)
        dtos.append(dict(id=name,kind=kind,value=val,canonical_json=raw.decode(),bytes=len(raw)))
        return val
    def codec(name,kind,value=None,accept=False,raw=None,context=None):
        if raw is None:raw=C.J(value)
        context=context or {}
        try:W.decode(kind,raw,**context);got=True
        except (C.Refusal,ValueError,UnicodeError):got=False
        assert got is accept,(name,got,accept)
        codecs.append(dict(id=name,kind=kind,raw_base64=base64.b64encode(raw).decode(),input_sha256=hashlib.sha256(raw).hexdigest(),input_bytes=len(raw),accept=accept,context=C.clone(context)))
    def protocol(name,q,r,accept=True,context=None):
        ctx=context or authority
        try:W.exchange(q,r,**ctx);got=True
        except (C.Refusal,ValueError,UnicodeError):got=False
        assert got is accept,(name,got,accept)
        protocols.append(dict(id=name,request=C.clone(q),response=C.clone(r),context=C.clone(ctx),accept=accept))
    jq=job('queued',None);jr=job('running',None);jd=job()
    dto('job/queued','CareerHistoryJob',jq);dto('job/running','CareerHistoryJob',jr);dto('job/done','CareerHistoryJob',jd)
    for error in sorted(W.TERMINAL):
        state='cancelled'if error=='history_cancelled'else 'failed'
        dto('job/'+error,'CareerHistoryJob',job(state,None,error))
    snapshots=[('idle',snapshot('idle',last='')),('active',snapshot('busy',active=jobid,last='')),('queued',snapshot('busy',queued=jobid,last='')),('ready',snapshot()),('cancelled',snapshot('cancelled','history_cancelled'))]
    for error in sorted(W.TERMINAL-{'history_cancelled'}):snapshots.append((error,snapshot('failed',error)))
    for error in sorted(W.UNAVAILABLE):snapshots.append(('unavailable/'+error,dict(version=1,state='unavailable',error=error,view=None,store_id='',activation=None,active_job_id='',queued_job_id='',last_job_id='')))
    for name,s in snapshots:dto('snapshot/'+name,'CareerHistorySnapshot',s)
    q_snapshot=request('snapshot');q_begin=request('begin',view_id=v['view_id'],prospect_id=pid);q_job=request('job',view_id=v['view_id'],job_id=jobid);q_cancel=dict(q_job,command='career_history_cancel')
    for name,q in [('snapshot',q_snapshot),('begin',q_begin),('job',q_job),('cancel',q_cancel)]:dto('request/'+name,'CareerHistoryRequest',q)
    for error in sorted(W.ERRORS):
        refusal=response(error=error);dto('response/'+error,'CareerHistoryResponse',refusal);protocol('refusal/'+error,q_begin,refusal)
    for name,q,r in [('snapshot',q_snapshot,response('snapshot',snapshot())),('begin',q_begin,response('job',jq)),('running-job',q_job,response('job',jr)),('done-job',q_job,response('job',jd)),('cancel-completed',q_cancel,response('job',jd)),('cancel-joining',q_cancel,response('job',jr))]:
        dto('response/'+name,'CareerHistoryResponse',r);protocol('success/'+name,q,r)
    pages=list(examples['pages'].values())
    page=next(p for p in pages if p['capture']['history_id']==cap['history_id']and p['after']==0)
    wrapped=dict(view=v,store_id=store,job_id=jobid,page=page)
    limit=max(1,len(page['items']));qp=request('page',view_id=v['view_id'],job_id=jobid,after=page['after'],limit=limit)
    rp=response('page',wrapped);ctx=dict(authority,selected_job=jd)
    dto('page-result/first','CareerHistoryPageResult',wrapped);dto('request/page','CareerHistoryRequest',qp);dto('response/page','CareerHistoryResponse',rp)
    protocol('page/exact-request',qp,rp,context=ctx)
    # Each core empty/end/middle/max page is wrapped under its own original View.
    for name,p in examples['pages'].items():
        pc=p['capture'];pv=next(z for z in views.values()if z['account_id']==pc['status']['pin']['account_id'])
        j=dict(version=1,job_id=C.ident(9100+len(protocols)),state='done',view=pv,store_id=pc['status']['store_id'],activation_id=pc['activation']['activation_id'],prospect_id=pc['assignment']['prospect_id'],error='',capture=pc)
        r=response('page',dict(view=pv,store_id=j['store_id'],job_id=j['job_id'],page=p));q=request('page',view_id=pv['view_id'],job_id=j['job_id'],after=p['after'],limit=max(1,len(p['items'])))
        protocol('page/core/'+name,q,r,context=dict(view=pv,store_id=j['store_id'],activation=pc['activation'],selected_job=j))
    # Lexical/closed-field controls isolate typed grammar, without source simulation.
    codec('lexical/reordered-request','CareerHistoryRequest',raw=json.dumps(dict(reversed(list(q_begin.items()))),separators=(',',':')).encode(),accept=True)
    codec('lexical/duplicate','CareerHistoryRequest',raw=C.J(q_begin)[:-1]+b',"id":1}')
    codec('lexical/exponent','CareerHistoryRequest',raw=C.J(q_begin).replace(b'"id":1',b'"id":1e0',1))
    codec('lexical/float','CareerHistoryRequest',raw=C.J(q_begin).replace(b'"id":1',b'"id":1.0',1))
    codec('lexical/nonfinite','CareerHistoryRequest',raw=C.J(q_begin).replace(b'"id":1',b'"id":NaN',1))
    codec('lexical/invalid-utf8','CareerHistoryRequest',raw=C.J(q_begin)[:-1]+b'\xff}')
    codec('lexical/trailing','CareerHistoryRequest',raw=C.J(q_begin)+b'{}')
    codec('lexical/depth','CareerHistoryRequest',value=dict(q_begin,after=[]))
    codec('lexical/request-cap-minus-lf','CareerHistoryRequest',raw=C.J(q_begin)+b' '*(1023-len(C.J(q_begin))),accept=True)
    codec('lexical/response-cap-minus-lf','CareerHistoryResponse',raw=C.J(rp)+b' '*(36863-len(C.J(rp))),accept=True)
    codec('lexical/request-cap-lf','CareerHistoryRequest',raw=C.J(q_begin)+b' '*(1024-len(C.J(q_begin))))
    codec('lexical/response-cap-lf','CareerHistoryResponse',raw=C.J(rp)+b' '*(36864-len(C.J(rp))))
    for key in q_begin:
        missing=C.clone(q_begin);del missing[key];codec('request/missing/'+key,'CareerHistoryRequest',missing)
    codec('request/unknown','CareerHistoryRequest',dict(q_begin,AsOfMS=1))
    for name,path,value in [('bool-id',['id'],True),('zero-id',['id'],0),('unsafe-id',['id'],9007199254740992),('version-bool',['version'],True),('wrong-family',['command'],'career_page'),('upper-view',['view_id'],'F'*64),('unused-job',['job_id'],jobid),('unused-after',['after'],1),('unused-limit',['limit'],1)]:codec('request/'+name,'CareerHistoryRequest',change(q_begin,path,value))
    for name,path,value in [('no-limit',['limit'],0),('too-large-limit',['limit'],9),('after257',['after'],257),('unused-prospect',['prospect_id'],pid)]:codec('page-request/'+name,'CareerHistoryRequest',change(qp,path,value))
    for name,s in [('busy-two-ids',snapshot('busy',active=jobid,queued=C.ident(7),last='')),('busy-last',snapshot('busy',active=jobid,last=jobid)),('idle-error',snapshot('idle','history_clock',last='')),('ready-no-job',snapshot('ready',last='')),('cancel-wrong-code',snapshot('cancelled','history_unavailable')),('failed-cancel',snapshot('failed','history_cancelled'))]:codec('snapshot/'+name,'CareerHistorySnapshot',s)
    for name,path,value in [('done-null',['capture'],None),('done-error',['error'],'history_clock'),('wrong-store',['store_id'],C.ident(1)),('wrong-activation',['activation_id'],C.ident(2)),('wrong-prospect',['prospect_id'],C.ident(3)),('wrong-view',['view'],views['incrementing256'])]:codec('job/'+name,'CareerHistoryJob',change(jd,path,value))
    for code in ['busy','outcome_unknown','invalid_frame','history_cancelled']:codec('failed-job/'+code,'CareerHistoryJob',job('failed',None,code))
    codec('queued/with-capture','CareerHistoryJob',job('queued',cap));codec('cancelled/wrong-code','CareerHistoryJob',job('cancelled',None,'history_unavailable'))
    for name,r in [('two-arms',dict(rp,job=jd)),('false-payload',dict(rp,ok=False,error='busy')),('success-error',dict(rp,error='busy')),('no-arm',response()),('false-no-code',dict(response(),ok=False)),('bool-version',dict(rp,version=True)),('unknown-error',response(error='invalid_frame'))]:codec('response/'+name,'CareerHistoryResponse',r)
    protocol('correlation/wrong-id',q_job,dict(response('job',jd),id=2),False)
    protocol('correlation/begin-done',q_begin,response('job',jd),False)
    protocol('correlation/begin-wrong-prospect',dict(q_begin,prospect_id=C.ident(33)),response('job',jq),False)
    protocol('correlation/lookup-wrong-job',dict(q_job,job_id=C.ident(34)),response('job',jd),False)
    protocol('correlation/stale-view',dict(q_job,view_id=C.ident(35)),response('job',jd),False)
    protocol('correlation/wrong-full-view',q_job,response('job',jd),False,dict(authority,view=dict(v,generation=v['generation']+1)))
    protocol('correlation/terminal-regression',q_job,response('job',jr),False,dict(authority,known_job=jd))
    protocol('correlation/terminal-repeat',q_job,response('job',jd),True,dict(authority,known_job=jd))
    protocol('correlation/page-without-sealed-job',qp,rp,False,authority)
    protocol('correlation/page-other-selected-job',qp,rp,False,dict(authority,selected_job=dict(jd,job_id=C.ident(36))))
    protocol('correlation/page-wrong-after',dict(qp,after=page['capture']['event_count']),rp,False,ctx)
    if len(page['items'])>1:protocol('correlation/page-too-many-for-request',dict(qp,limit=1),rp,False,ctx)
    protocol('correlation/wrong-family-arm',q_job,response('snapshot',snapshot()),False)
    # HistoryID deliberately omits captured mutable Status fields. A structurally
    # coherent different Status must still fail exact captured-object correlation.
    changed_page=C.clone(wrapped)
    cs=changed_page['page']['capture']['status'];cs['observed_source_count']+=1;cs['caught_up']=False
    codec('page/changed-status-locally-valid','CareerHistoryPageResult',changed_page,accept=True)
    protocol('correlation/same-HistoryID-changed-Status',qp,response('page',changed_page),False,ctx)
    for label,path,value in [('bool-xp',['items',0,'xp_before'],False),('bool-points',['items',0,'earned','base_points'],True),('wrong-applied',['items',0,'xp_applied'],1),('wrong-ordinal',['items',0,'ordinal'],1),('after-capture-time',['items',0,'event','effective_at_ms'],cap['as_of_ms']+1),('wrong-original-owner',['items',0,'event','corner_a','account_id'],C.ident(70))]:
        changed=change(wrapped,['page']+path,value)
        codec('row/'+label,'CareerHistoryPageResult',changed)
    # An artificial short middle page is structurally possible for a smaller
    # limit, but the actual requested limit must still reject it.
    short=C.clone(wrapped);short['page']['items']=short['page']['items'][:1];short['page']['next']=short['page']['after']+1;short['page']['has_more']=short['page']['next']<cap['event_count']
    codec('page/short-for-smaller-limit','CareerHistoryPageResult',short,accept=True)
    protocol('correlation/artificial-short-page',qp,response('page',short),False,ctx)
    result=dict(version=1,scope='Independent structural grammar and explicit supplied-authority correlation; no sealed handle or completeness imported from wire. Job/cancel exchange positives assume native admission already succeeded for a known original-authority tuple or latest same-authority Snapshot-named ID; native lookup discovery is separately modeled.',dtos=dtos,codecs=codecs,protocols=protocols)
    result['transport_trap']=trap_verify(result)
    return result


def trap_verify(fragment):
    """Validate already derived DTOs with input/derivation/process/RPC access trapped."""
    import builtins,subprocess,socket,os
    import history_math as H
    # Module initialization captures only documentary scalar constants before traps.
    saved=[];calls={k:0 for k in ['input_reads','history_derivation','process','network']}
    def trap(category):
        def blocked(*args,**kwargs):
            calls[category]+=1
            raise AssertionError('forbidden transport '+category)
        return blocked
    def replace(obj,name,category):
        saved.append((obj,name,getattr(obj,name)));setattr(obj,name,trap(category))
    for obj,name,category in [(C,'load','input_reads'),(pathlib.Path,'read_bytes','input_reads'),(pathlib.Path,'read_text','input_reads'),(pathlib.Path,'open','input_reads'),(builtins,'open','input_reads'),(H,'derive_capture','history_derivation'),(H,'derive_event','history_derivation'),(H,'examples','history_derivation'),(H,'_examples','history_derivation'),(subprocess,'Popen','process'),(subprocess,'run','process'),(subprocess,'check_output','process'),(os,'system','process'),(socket,'socket','network')]:replace(obj,name,category)
    try:
        for d in fragment['dtos']:W.decode(d['kind'],d['canonical_json'].encode())
        for d in fragment['codecs']:
            try:W.decode(d['kind'],base64.b64decode(d['raw_base64'],validate=True),**d['context']);accepted=True
            except (C.Refusal,ValueError,UnicodeError):accepted=False
            assert accepted is d['accept'],d['id']
        for d in fragment['protocols']:
            try:W.exchange(d['request'],d['response'],**d['context']);accepted=True
            except (C.Refusal,ValueError,UnicodeError):accepted=False
            assert accepted is d['accept'],d['id']
        assert not any(calls.values())
    finally:
        for obj,name,value in reversed(saved):setattr(obj,name,value)
    return dict(status='passed',dtos=len(fragment['dtos']),codecs=len(fragment['codecs']),protocols=len(fragment['protocols']),forbidden_calls=calls,scope='Python parser isolation only; no actual runtime counters, source proof or sealed authority.')

if __name__=='__main__':
    out=build_wire();p=pathlib.Path(__file__).parent/'fragments/wire.json'
    p.write_bytes(C.J(out)+b'\n');print(len(out['dtos']),len(out['codecs']),len(out['protocols']),hashlib.sha256(p.read_bytes()).hexdigest())
