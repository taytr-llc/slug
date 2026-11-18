"""Independent finite Result helper/native vectors from frozen contracts.

No Go, helper, network, filesystem authority, process or engine is executed.
Core examples include genuine prior47 source and explicitly modeled48 publication.
All recipes are finite, deterministic and hash/byte-count bound. Expected accepts
are declared by each contract control, then checked by this independent schema.
"""
import json
from contextlib import contextmanager
from pathlib import Path
from common import J, clone, sha, label, Refusal, SAFE, DATE_MAX, pretty
import schema_wire as W
import crypto_result as R
from reference_results import examples


def locate(v,path):
    for part in path:v=v[part]
    return v


def edit(v,op):
    target=locate(v,op['path'][:-1]);key=op['path'][-1]
    if op['op']=='set':target[key]=clone(op['value'])
    elif op['op']=='delete':del target[key]
    elif op['op']=='append':target[key].append(clone(op['value']))
    else:raise AssertionError('unknown edit')


def materialize(base,recipe):
    """Single portable recipe vocabulary; all output bytes are independently bound."""
    v=clone(base)
    for op in recipe.get('edits',[]):edit(v,op)
    if recipe.get('rehash_record_status',False):
        # Only this exact helper ReadResult arm; no inferred arbitrary formula.
        v['record']['status']['record_digest']=R.record_digest(None,v['record']['record'])
    style=recipe.get('style','canonical')
    if style=='reversed':
        def reverse(x):
            if type(x)is dict:return {k:reverse(x[k])for k in reversed(list(x))}
            if type(x)is list:return [reverse(i)for i in x]
            return x
        raw=J(reverse(v))
    elif style=='whitespace':raw=json.dumps(v,ensure_ascii=True,indent=2,allow_nan=False).encode('ascii')
    elif style=='canonical':raw=J(v)
    else:raise AssertionError('unknown style')
    if 'replace_once'in recipe:
        old,new=recipe['replace_once'];old=old.encode('utf8');new=new.encode('utf8')
        assert raw.count(old)>=1,(old[:80],recipe)
        raw=raw.replace(old,new,1)
    if 'raw_hex'in recipe:raw=bytes.fromhex(recipe['raw_hex'])
    if 'pad_bytes'in recipe:
        assert recipe['pad_bytes']>=len(raw)
        raw+=b' '*(recipe['pad_bytes']-len(raw))
    if 'append_hex'in recipe:raw+=bytes.fromhex(recipe['append_hex'])
    return raw


def request(cmd,v,x,**kw):
    q=dict(id=1,command=cmd,view_id='',saturday='',bout_id='',job_id='',path='')
    for field in W.MATRIX[cmd]:
        q[field]={'view_id':v['view_id'],'saturday':x['source']['source']['event']['saturday'],
                  'bout_id':x['source']['source']['bout']['bout_id'],
                  'job_id':label('wire job '+cmd),'path':'/tmp/candidate-result-proof.json'}[field]
    q.update(kw);return q


def snapshot(v,x,state='ready',owner='none',node='offline'):
    return dict(version=1,state=state,error='',view=clone(v),store_id=R.store_id(x['pin']),
                pin=clone(x['pin']),source_created_at_ms=x['info']['source_created_at_ms'],
                created_at_ms=x['info']['created_at_ms'],publisher_account_id=x['pin']['publisher_account_id'],
                node_state=node,node_owner=owner,active_job_id='',queued_job_id='',last_job_id='')


def job(v,x,kind='publish',state='queued',state_name=None):
    src=x['source'];s=src['source'];q=dict(version=1,job_id=label('wire job result_'+kind),
        kind=kind,state=state,view=clone(v),store_id=R.store_id(x['pin']),card_id=s['card_id'],
        saturday=s['event']['saturday'],bout_id=s['bout']['bout_id'],
        result_key=R.result_key(x['pin'],s['card_id'],s['bout']['bout_id']),error='',result=None,export=None)
    if state=='completed':
        if kind=='export':q['export']=clone(x['proof_summary'])
        else:q['result']=clone(x['action_results'][state_name or {'publish':'submitted','reconcile':'prepared','attest':'finalized60'}[kind]])
    elif state=='cancelled':q['error']='result_cancelled'
    elif state=='failed':q['error']='outcome_unknown'
    return q


def response(payload,value,v,x,ident=1):
    r=dict(id=ident,ok=True,view=clone(v),store_id=R.store_id(x['pin']),snapshot=None,status=None,record=None,job=None,error='')
    r[payload]=clone(value)
    if payload=='snapshot':r['view']=clone(value['view']);r['store_id']=value['store_id']
    return r


def refusal(err='busy',ident=1):
    return dict(id=ident,ok=False,view=None,store_id='',snapshot=None,status=None,record=None,job=None,error=err)


class Vectors:
    def __init__(self,x):self.x=x;self.p=x['pin'];self.cases=[];self.dtos={};self.names=set()
    def add(self,c):
        assert c['name']not in self.names,c['name'];self.names.add(c['name']);self.cases.append(c)
    def dto(self,kind,name,v):
        canonical=W.validate(kind,v,self.p);b=J(canonical)
        self.dtos[name]=dict(kind=kind,value=canonical)
        self.add(dict(category='dto',kind=kind,name=name,context='transport',value=canonical,canonical=b.decode('utf8'),sha256=sha(b),bytes=len(b)))
        return name
    def codec(self,name,base,recipe,accept):
        dto=self.dtos[base];b=materialize(dto['value'],recipe);value=None
        try:value=W.parse(dto['kind'],b,self.p);actual=True
        except Refusal:actual=False
        assert actual==accept,('codec',name,'expected',accept,'actual',actual)
        c=dict(category='codec',name=name,kind=dto['kind'],context='transport',base=base,recipe=recipe,input_sha256=sha(b),input_bytes=len(b),accept=accept)
        if actual:
            canonical=J(value)
            if canonical==J(dto['value']):c['canonical_ref']=base
            else:c['canonical']=canonical.decode('utf8')
            c['canonical_sha256']=sha(canonical)
        self.add(c)
    def protocol(self,name,q,r,v,accept,expected_job=None,terminal_digest='',expected_source=None,**context):
        x=self.x
        args=dict(current=clone(v),source_created=x['info']['source_created_at_ms'],created=x['info']['created_at_ms'],expected_job=clone(expected_job),terminal_digest=terminal_digest,expected_source=clone(expected_source),known_publication=None)
        args.update(context)
        try:W.correlate(q,r,self.p,**args);actual=True
        except Refusal:actual=False
        assert actual==accept,('protocol',name,'expected',accept,'actual',actual)
        self.add(dict(category='protocol',name=name,request=clone(q),response=clone(r),current_view=args['current'],authority_pin=clone(self.p),source_created_at_ms=args['source_created'],created_at_ms=args['created'],expected_job=args['expected_job'],terminal_digest=args['terminal_digest'],expected_source=args['expected_source'],known_publication=clone(args['known_publication']),accept=accept))


def build_fragment():
    x=examples();v0,v1=x['views'];g=Vectors(x);p=x['pin']
    for fi,v in enumerate(x['views']):
        prefix='fixture'+str(fi)+'/'
        for cmd in W.MATRIX:g.dto('ResultRequest',prefix+'request/'+cmd,request(cmd,v,x))
        for owner,node in [('none','offline'),('publication','starting'),('publication','online'),('mining','online')]:
            g.dto('ResultSnapshot',prefix+'snapshot/'+owner+'-'+node,snapshot(v,x,owner=owner,node=node))
        ss=snapshot(v,x);ss['last_job_id']=label('last job');g.dto('ResultSnapshot',prefix+'snapshot/ready-last',ss)
        for active in ('active_job_id','queued_job_id','foreign'):
            ss=snapshot(v,x,state='busy')
            if active!='foreign':ss[active]=label(active)
            g.dto('ResultSnapshot',prefix+'snapshot/busy-'+active,ss)
        for err in sorted(W.FATAL_SNAPSHOT):
            ss=snapshot(v,x,state='failed');ss['error']=err;g.dto('ResultSnapshot',prefix+'snapshot/failed-'+err,ss)
        for state,err in [('setup_incomplete','result_setup_incomplete'),('unavailable','result_unavailable')]:
            ss=snapshot(v,x,state=state);ss.update(error=err,pin=None,store_id='',publisher_account_id='',source_created_at_ms=0,created_at_ms=0)
            if state=='unavailable':ss['view']=None
            g.dto('ResultSnapshot',prefix+'snapshot/'+state,ss)
        for kind in ('publish','reconcile','attest','export'):
            for state in ('queued','running','completed','failed','cancelled'):
                g.dto('ResultJob',prefix+'job/'+kind+'/'+state,job(v,x,kind,state))
        # Include every valid publication phase in read and reconciliation replies.
        for name,st in x['statuses'].items():g.dto('ResultResponse',prefix+'response/status/'+name,response('status',st,v,x))
        for name,ar in x['action_results'].items():
            g.dto('ResultResponse',prefix+'response/record/'+name,response('record',ar,v,x))
        g.dto('ResultResponse',prefix+'response/record/fresh-source-conflict',response('record',x['source_conflict_read'],v,x))
        g.dto('ResultResponse',prefix+'response/status/fresh-source-conflict',response('status',x['source_conflict_read']['status'],v,x))
        for name in ['none-offline','publication-starting','publication-online','mining-online','setup_incomplete','unavailable']:
            g.dto('ResultResponse',prefix+'response/snapshot/'+name,response('snapshot',g.dtos[prefix+'snapshot/'+name]['value'],v,x))
        for kind in ('publish','reconcile','attest','export'):
            for state in ('queued','completed','failed','cancelled'):
                g.dto('ResultResponse',prefix+'response/job/'+kind+'/'+state,response('job',g.dtos[prefix+'job/'+kind+'/'+state]['value'],v,x))
    # Closed error vocabulary, including the unique zero-ID invalid-frame response.
    for err in sorted(W.ERRORS):g.dto('ResultResponse','response/error/'+err,refusal(err))
    g.dto('ResultResponse','response/error/zero-invalid-frame',refusal('invalid_frame',0))
    # Canonicalization controls for each public wrapper; JSON order is not semantic.
    bases=['fixture0/request/result_publish','fixture0/snapshot/none-offline','fixture0/job/publish/queued','fixture0/response/record/finalized60']
    for base in bases:
        for style in ('canonical','reversed','whitespace'):g.codec('canonical/'+base+'/'+style,base,{'style':style},True)
        for field in W.FIELDS[g.dtos[base]['kind']]:g.codec('missing/'+base+'/'+field,base,{'edits':[{'op':'delete','path':[field]}]},False)
        g.codec('unknown/'+base,base,{'edits':[{'op':'set','path':['unknown'],'value':None}]},False)
    def mutation(name,base,path,value,accept=False):g.codec(name,base,{'edits':[{'op':'set','path':path,'value':value}]},accept)
    reqbase='fixture0/request/result_publish'
    for n in [0,-1,SAFE+1,True,False,1.0,None,'1']:mutation('request/id/'+repr(n),reqbase,['id'],n)
    for n in [1,SAFE]:mutation('request/id/positive-'+str(n),reqbase,['id'],n,True)
    for s in ['', 'RESULT_PUBLISH','seed_publish','result_prepare','result_history','result_connect']:mutation('request/command/'+s,reqbase,['command'],s)
    for field in ('view_id','bout_id'):
        for label_,s in [('empty',''),('upper','A'*64),('short','a'*63),('nonhex','g'*64)]:mutation('request/'+field+'/'+label_,reqbase,[field],s)
    for s in ['2026-11-27','2026-11-29','2026-2-28','2025-11-29','2100-01-02','2026-02-30','2026-11-28T00:00:00Z']:mutation('request/saturday/'+s,reqbase,['saturday'],s)
    for cmd in W.MATRIX:
        base='fixture0/request/'+cmd
        for unused in ('view_id','saturday','bout_id','job_id','path'):
            if unused not in W.MATRIX[cmd]:mutation('request/unused/'+cmd+'/'+unused,base,[unused],'x')
    pathbase='fixture0/request/result_export'
    paths=[('ordinary','/tmp/proof.json',True),('unicode','/tmp/拳击-é.json',True),('line-separators','/tmp/a\u2028b\u2029c',True),('html-quote-backslash','/tmp/<&>"\\proof',True),('at-limit','/'+'a'*1023,True),('utf8-at-limit','/'+('é'*511)+'a',True),('over-limit','/'+'a'*1024,False),('utf8-over-limit','/'+('é'*512),False),('relative','proof.json',False),('empty','',False),('nul','/tmp/\0x',False),('lf','/tmp/\nx',False),('cr','/tmp/\rx',False),('tab','/tmp/\tx',False)]
    for name,path,valid in paths:mutation('path/'+name,pathbase,['path'],path,valid)
    mutation('path/max-json-escaping',pathbase,['path'],'/'+('\x01'*1023),True)
    # Ordinary non-ASCII canonical bytes remain UTF-8; only U+2028/U+2029 escape.
    assert J('/拳\u2028\u2029<&>')==b'"/\xe6\x8b\xb3\\u2028\\u2029\\u003c\\u0026\\u003e"'
    for old,new,name in [('"id":1','"id":-0','negative-zero'),('"id":1','"id":1e0','exponent'),('"id":1','"id":1.0','float'),('"id":1','"id":NaN','nan'),('"id":1','"id":1,"\\u0069d":1','decoded-duplicate')]:g.codec('lexical/'+name,reqbase,{'replace_once':[old,new]},False)
    g.codec('lexical/trailing-document',reqbase,{'append_hex':b' {}'.hex()},False)
    g.codec('lexical/invalid-utf8',reqbase,{'append_hex':'ff'},False)
    for base in [reqbase,'fixture0/response/record/finalized60']:
        cap=W.CAPS[g.dtos[base]['kind']]
        g.codec('frame/exact-lf/'+base,base,{'pad_bytes':cap-1},True)
        g.codec('frame/one-over-lf/'+base,base,{'pad_bytes':cap},False)
    g.codec('lexical/depth21',reqbase,{'raw_hex':(('['*21)+'0'+(']'*21)).encode().hex()},False)
    g.codec('lexical/lone-surrogate-path',pathbase,{'replace_once':['/tmp/candidate-result-proof.json','/tmp/\\ud800']},False)
    # Snapshot node/authority grammar, including failed state's closed fatal subset.
    snap='fixture0/snapshot/none-offline'
    for field,val in [('version',True),('pin',None),('view',None),('state','online'),('error','busy'),('store_id',label('wrong store')),('publisher_account_id',label('wrong publisher')),('source_created_at_ms',0),('created_at_ms',x['info']['source_created_at_ms']-1),('node_state','unknown'),('node_owner','wallet'),('active_job_id',label('active'))]:mutation('snapshot/'+field,snap,[field],val)
    for node,owner in [('offline','publication'),('starting','none'),('starting','mining'),('online','none')]:
        g.codec('snapshot/node-pair/'+node+'/'+owner,snap,{'edits':[{'op':'set','path':['node_state'],'value':node},{'op':'set','path':['node_owner'],'value':owner}]},False)
    busy='fixture0/snapshot/busy-active_job_id'
    mutation('snapshot/two-busy-ids',busy,['queued_job_id'],label('queued'))
    mutation('snapshot/alias-last-active',busy,['last_job_id'],g.dtos[busy]['value']['active_job_id'])
    for err in sorted(W.ERRORS-W.FATAL_SNAPSHOT):mutation('snapshot/nonfatal/'+err,'fixture0/snapshot/failed-result_clock',['error'],err)
    mutation('snapshot/setup-authority','fixture0/snapshot/setup_incomplete',['pin'],p)
    mutation('snapshot/unavailable-view','fixture0/snapshot/unavailable',['view'],v0)
    # Job purpose/null arms and exact current selector constraints.
    jb='fixture0/job/publish/queued'
    for field,val in [('version',True),('kind','prepare'),('state','complete'),('error','busy'),('result',x['action_results']['submitted']),('export',x['proof_summary']),('card_id',label('wrong card')),('result_key',label('wrong resultkey')),('saturday','2026-12-05')]:mutation('job/queued/'+field,jb,[field],val)
    complete='fixture0/job/publish/completed'
    for state in ('prepared','built','dispatch_unknown'):mutation('job/publish-requires-submitted-or-included/'+state,complete,['result'],x['action_results'][state])
    for state in ('included59','checkpoint59','finalized60','reincluded'):mutation('job/publish-included/'+state,complete,['result'],x['action_results'][state],True)
    mutation('job/attest-no-finality','fixture0/job/attest/completed',['result'],x['action_results']['checkpoint59'])
    for kind in ('publish','attest'):
        mutation('job/strong-action-source-conflict/'+kind,'fixture0/job/'+kind+'/completed',['result'],x['source_conflict_read'])
    mutation('job/reconcile-historical-source-conflict','fixture0/job/reconcile/completed',['result'],x['source_conflict_read'],True)
    mutation('job/reconcile-prepared','fixture0/job/reconcile/completed',['result'],x['action_results']['prepared'],True)
    mutation('job/completed-two-arms',complete,['export'],x['proof_summary'])
    mutation('job/completed-no-arm',complete,['result'],None)
    for field,val in [('error','busy'),('result',x['action_results']['submitted'])]:mutation('job/cancelled/'+field,'fixture0/job/publish/cancelled',[field],val)
    mutation('job/failed-payload','fixture0/job/publish/failed',['result'],x['action_results']['submitted'])
    mutation('job/failed-frame-error','fixture0/job/publish/failed',['error'],'invalid_frame')
    # ProofSummary is the only export payload, with exact closed public bounds.
    ex='fixture0/job/export/completed'
    for field,values in {'bytes':[0,25165825,True],'header_count':[0,513,True],'block_count':[0,4,True],'finalized_at_ms':[-1,True],'sha256':['A'*64],'txid':[''],'result_key':[label('wrong key')]}.items():
        for i,val in enumerate(values):mutation('export/'+field+'/'+str(i),ex,['export',field],val)
    for field,val in [('bytes',25165824),('header_count',512),('block_count',3),('finalized_at_ms',0)]:mutation('export/boundary/'+field,ex,['export',field],val,True)
    mutation('export/no-full-proof',ex,['export','proof'],{'headers':[]})
    mutation('export/no-path',ex,['export','path'],'/tmp/proof.json')
    # Embedded signed sentinels: all required -1 paths are explicit, not suffix based.
    rec='fixture0/response/record/prepared'
    for path in [['record','record','cursor','height'],['record','record','cursor','complete_to_height'],['record','status','scan_height'],['record','status','scan_tip_height']]:
        mutation('signed/negative-two/'+'/'.join(path),rec,path,-2)
        mutation('signed/bool/'+'/'.join(path),rec,path,False)
    mutation('signed/unsigned-prepared',rec,['record','record','prepared_at_ms'],-1)
    # Branch/path ceilings (wrong null and unknown arrays fail before broad decoding).
    finalized='fixture0/response/record/finalized60'
    record_path=['record','record']
    for field,hi in [('scan_restarts',16),('prior_inclusions',16),('conflicts',8)]:
        mutation('array/null/'+field,finalized,record_path+[field],None)
        mutation('array/over/'+field,finalized,record_path+[field],[{}]*(hi+1))
    mutation('array/corners-one',finalized,record_path+['statement','statement','source','source','corners'],[x['source']['source']['corners'][0]])
    mutation('array/merkle21',finalized,record_path+['inclusion','anchor','merkle_branch'],[label('branch')]*21)
    mutation('array/unknown',finalized,['anything'],[])
    # Response one-of and error arms are closed, independently of command correlation.
    resp='fixture0/response/status/absent'
    for field,val in [('id',0),('ok',1),('error','busy'),('view',None),('store_id',label('wrong store')),('job',job(v0,x)),('status',None)]:mutation('response/'+field,resp,[field],val)
    for field,val in [('view',v0),('store_id',R.store_id(p)),('status',x['statuses']['absent']),('error','ordinary_exception')]:mutation('response/error-arm/'+field,'response/error/busy',[field],val)
    mutation('response/zero-busy','response/error/busy',['id'],0)
    # Full response identities remain bound to the actual record, not status labels.
    mutation('response/record-digest',finalized,['record','status','record_digest'],label('different observed record'))
    mutation('response/source-record-digest',finalized,['record','status','source_record_digest'],label('different saved record'))
    mutation('response/phase',finalized,['record','status','phase'],'prepared')
    for name,base,paths in [
        ('statement',rec,[['record','record','statement','signature']]),
        ('checkpoint',finalized,[['record','record','checkpoint','signature'],['record','record','finality','checkpoint','signature']]),
        ('equivocation','fixture0/response/record/equivocation',[['record','record','equivocation','signature']])]:
        g.codec('signature/bad-rehashed-'+name,base,{'edits':[{'op':'set','path':path,'value':'00'*64}for path in paths],'rehash_record_status':True},False)
    # Complete native correlation, including legal standalone DTOs in wrong context.
    source_ids={k:x['source'][v]for k,v in [('source_record_digest','record_digest'),('input_id','input_id'),('result_digest','result_digest')]}
    for fi,v in enumerate(x['views']):
        prefix='protocol/fixture'+str(fi)+'/'
        for cmd in W.MATRIX:
            q=request(cmd,v,x)
            if cmd=='result_snapshot':r=response('snapshot',snapshot(v,x),v,x)
            elif cmd=='result_status':r=response('status',x['statuses']['absent'],v,x)
            elif cmd=='result_record':r=response('record',x['action_results']['finalized60'],v,x)
            else:
                kind=W.BEGIN.get(cmd,'publish');j=job(v,x,kind);q['job_id']=j['job_id']if cmd in ('result_job','result_cancel')else'';r=response('job',j,v,x)
            g.protocol(prefix+'command/'+cmd,q,r,v,True,expected_source=source_ids)
            wrong=clone(r);wrong['id']+=1;g.protocol(prefix+'wrong-request-id/'+cmd,q,wrong,v,False)
            if cmd!='result_snapshot':
                other=clone(v);other['generation']+=1;g.protocol(prefix+'new-generation/'+cmd,q,r,other,False)
        q=request('result_publish',v,x);queued=job(v,x);running=job(v,x,state='running');done=job(v,x,state='completed')
        g.protocol(prefix+'ack-not-running',q,response('job',running,v,x),v,False)
        g.protocol(prefix+'ack-not-terminal',q,response('job',done,v,x),v,False)
        g.protocol(prefix+'ack-wrong-kind',q,response('job',job(v,x,'attest'),v,x),v,False)
        q=request('result_job',v,x,job_id=queued['job_id'])
        g.protocol(prefix+'running-to-queued',q,response('job',queued,v,x),v,False,expected_job=running)
        g.protocol(prefix+'queued-to-running',q,response('job',running,v,x),v,True,expected_job=queued)
        g.protocol(prefix+'running-to-completed',q,response('job',done,v,x),v,True,expected_job=running,expected_source=source_ids)
        g.protocol(prefix+'discover-completed',q,response('job',done,v,x),v,True,expected_source=source_ids)
        digest=sha(J(done));g.protocol(prefix+'terminal-identical',q,response('job',done,v,x),v,True,expected_job=done,terminal_digest=digest)
        failed=job(v,x,state='failed');g.protocol(prefix+'terminal-cannot-change',q,response('job',failed,v,x),v,False,expected_job=done,terminal_digest=digest)
        g.protocol(prefix+'terminal-wrong-digest',q,response('job',done,v,x),v,False,terminal_digest=label('wrong canonical digest'))
        changed=clone(done);changed['result']=clone(x['action_results']['included59']);g.protocol(prefix+'terminal-newer-observation-not-rewrite',q,response('job',changed,v,x),v,False,terminal_digest=digest)
        for err in ('result_waiting_for_funds','result_scan_incomplete','outcome_unknown'):
            j=job(v,x,state='failed');j['error']=err;g.protocol(prefix+'definite-or-unknown-no-payload/'+err,q,response('job',j,v,x),v,True,expected_job=queued)
        q=request('result_record',v,x);rr=response('record',x['action_results']['finalized60'],v,x)
        g.protocol(prefix+'source-created-exact',q,rr,v,False,source_created=x['info']['source_created_at_ms']+1)
        g.protocol(prefix+'result-created-not-after-prepared',q,rr,v,False,created=x['now']+1)
        wrong=clone(source_ids);wrong['input_id']=label('other input');g.protocol(prefix+'saved-input-binding',q,rr,v,False,expected_source=wrong)
        g.protocol(prefix+'wrong-command-arm',request('result_status',v,x),rr,v,False)
        g.protocol(prefix+'same-source-new-recorddigest',q,response('record',x['action_results']['reincluded'],v,x),v,True,expected_source=source_ids)
        remembered=dict(statement_id=R.statement_id(x['statement']['statement']),txid=x['transaction']['txid'],raw_tx=x['transaction']['raw_tx'],checkpoint=clone(x['records']['finalized60']['checkpoint']),finality=clone(x['records']['finalized60']['finality']))
        g.protocol(prefix+'remembered-immutable-publication',q,rr,v,True,known_publication=remembered)
        g.protocol(prefix+'fresh-source-conflict-keeps-result-observation',q,response('record',x['source_conflict_read'],v,x),v,True,known_publication=remembered,expected_source=source_ids)
        g.protocol(prefix+'fresh-source-conflict-status',request('result_status',v,x),response('status',x['source_conflict_read']['status'],v,x),v,True,known_publication=remembered,expected_source=source_ids)
        for field in ('statement_id','txid','raw_tx','checkpoint','finality'):
            bad=clone(remembered)
            if field=='checkpoint':bad[field]['signature']='00'*64
            elif field=='finality':bad[field]['observed_at_ms']+=1
            elif field=='raw_tx':bad[field]='00'
            else:bad[field]=label('other '+field)
            g.protocol(prefix+'remembered-'+field+'-must-match',q,rr,v,False,known_publication=bad)
        for state in ('prepared','included59','checkpoint59'):
            g.protocol(prefix+'known-publication-cannot-disappear/'+state,q,response('record',x['action_results'][state],v,x),v,False,known_publication=remembered)
        g.protocol(prefix+'status-known-finality-cannot-disappear',request('result_status',v,x),response('status',x['statuses']['checkpoint59'],v,x),v,False,known_publication=remembered)
        precheckpoint=clone(remembered);precheckpoint['checkpoint']=None;precheckpoint['finality']=None
        g.protocol(prefix+'reinclusion-preserves-raw-transaction',q,response('record',x['action_results']['reincluded'],v,x),v,True,known_publication=precheckpoint)
        q=request('result_snapshot',v,x);rs=response('snapshot',snapshot(v,x),v,x)
        g.protocol(prefix+'snapshot-created',q,rs,v,False,created=x['info']['created_at_ms']+1)
        g.protocol(prefix+'snapshot-source-created',q,rs,v,False,source_created=x['info']['source_created_at_ms']+1)
        for name in ('setup_incomplete','unavailable'):
            ss=g.dtos['fixture'+str(fi)+'/snapshot/'+name]['value'];g.protocol(prefix+'snapshot-'+name,q,response('snapshot',ss,v,x),v,True)
            g.protocol(prefix+'snapshot-no-current-'+name,q,response('snapshot',ss,v,x),None,name=='unavailable')
        g.protocol(prefix+'snapshot-ready-no-current',q,rs,None,False)
        q=request('result_job',v,x,job_id=label('wire job result_export'));exjob=job(v,x,'export','completed')
        g.protocol(prefix+'export-summary-identity',q,response('job',exjob,v,x),v,True,expected_source=source_ids)
        wrong=clone(source_ids);wrong['source_record_digest']=label('wrong source');g.protocol(prefix+'export-wrong-source',q,response('job',exjob,v,x),v,False,expected_source=wrong)
        # A typed refusal is correlated to the request, never accepted as success.
        g.protocol(prefix+'error-current-request',request('result_status',v,x),refusal('outcome_unknown'),v,True)
        g.protocol(prefix+'uncorrelated-invalid-frame',request('result_status',v,x),refusal('invalid_frame',0),v,False)
    counts={k:sum(c['category']==k for c in g.cases)for k in ('dto','codec','protocol')}
    return dict(version=1,kind='wire',scope='independent_transport_and_correlation_only_no_actual_authority_or_process',counts=counts,
        recipe_vocabulary=dict(edits=['set','delete','append'],styles=['canonical','reversed','whitespace'],byte_operations=['replace_once','raw_hex','pad_bytes','append_hex'],derived_operations=['rehash_record_status']),
        provenance=x['provenance'],cases=g.cases)


@contextmanager
def cheap_transport_only():
    """Executable Python call-boundary check, not a future Go runtime claim."""
    import common
    targets=[(common.MANIFEST.__dict__,'generate'),(common.__dict__,'condition'),
        (common.INHERITED.corner.__globals__,'condition'),
        (common.CONDITION.__dict__,'project'),(common.ENGINE.__dict__,'simulate'),
        (common.CRYPTO.__dict__,'ed_sign'),(common.CRYPTO.__dict__,'sign'),
        (common.CRYPTO.__dict__,'verify_merkle')]
    saved=[];seen=set()
    def forbidden(*args,**kwargs):raise AssertionError('transport invoked expensive semantic/proof/sign/engine dependency')
    try:
        for ns,name in targets:
            key=(id(ns),name)
            if key in seen:continue
            seen.add(key);saved.append((ns,name,ns[name]));ns[name]=forbidden
        yield
    finally:
        for ns,name,value in reversed(saved):ns[name]=value


def _self_check(f,p):
    dtos={c['name']:c for c in f['cases']if c['category']=='dto'}
    for c in f['cases']:
        cat=c['category']
        if cat=='dto':
            b=c['canonical'].encode('utf8');assert sha(b)==c['sha256']and len(b)==c['bytes'];assert J(W.parse(c['kind'],b,p))==b
        elif cat=='codec':
            b=materialize(dtos[c['base']]['value'],c['recipe']);assert sha(b)==c['input_sha256']and len(b)==c['input_bytes']
            try:v=W.parse(c['kind'],b,p);ok=True
            except Refusal:ok=False
            assert ok==c['accept'],c['name']
            if ok:
                expected=dtos[c['canonical_ref']]['canonical']if'canonical_ref'in c else c['canonical'];assert J(v)==expected.encode('utf8')and sha(J(v))==c['canonical_sha256']
        else:
            try:W.correlate(c['request'],c['response'],c['authority_pin'],c['current_view'],c['source_created_at_ms'],c['created_at_ms'],c['expected_job'],c['terminal_digest'],c['expected_source'],c['known_publication']);ok=True
            except Refusal:ok=False
            assert ok==c['accept'],c['name']
    assert len({c['name']for c in f['cases']})==len(f['cases'])
    return True


def self_check(f):
    p=examples()['pin']
    with cheap_transport_only():return _self_check(f,p)


if __name__=='__main__':
    f=build_fragment();self_check(f);out=Path(__file__).resolve().parent/'fragments'/'wire.json';out.parent.mkdir(exist_ok=True);out.write_bytes(pretty(f));print(json.dumps(f['counts'],sort_keys=True))
