#!/usr/bin/env python3
"""Independent Set66 private-wire model from the frozen desktop contract.

No production DTO, encoder, controller or UI is imported. Correlation IDs and
native selection are explicitly synthetic transport context, never real file
identity, session authority or newly signed evidence. No work/signing/network.
"""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import re
import sys

sys.dont_write_bytecode=True
HERE=Path(__file__).resolve().parent
CONTRACT='be2053422fb57adfd7839624bb545c237aea265547891917ff1f028335ed9feb'
CORE='eb51028daf2ebbda44dfbfdb79f58353137ed03f1836fad5b8db8cf428300772'
SAFE=9007199254740991
DATE_MAX=8640000000000000
ERRORS=('invalid_request','stale_view','unauthorized','forbidden','busy','history_not_found',
        'history_unavailable','evidence_not_found','evidence_capacity','evidence_operation_conflict',
        'evidence_unavailable','evidence_profile','outcome_unknown')
TERMINAL=('evidence_invalid','evidence_source_changed','evidence_unavailable','evidence_destination_conflict',
          'evidence_storage_failed','evidence_export_uncertain')
REQUEST_FIELDS='id version command view_id history_job_id history_id job_id selection'
RESPONSE_FIELDS='id version ok snapshot job error'
SNAPSHOT_FIELDS='version state error view store_id active_job_id last_job_id'
JOB_FIELDS='version job_id state view store_id history_job_id history_id prospect_id error summary'
SUMMARY_FIELDS='version file_sha256 file_bytes prospect_id history_id source_record_digest event_count captured_as_of_ms'
CASES=('actual/v1/zero256-awarded','actual/v2/incrementing256-awarded','actual/v2/zero256-unawarded')


class Refusal(ValueError):pass
def require(ok):
    if not ok:raise Refusal('invalid_wire_or_context')
def clone(v):return copy.deepcopy(v)
def sha(b):return hashlib.sha256(b).hexdigest()
def canon(v):return json.dumps(v,ensure_ascii=True,separators=(',',':'),allow_nan=False).encode('ascii')
def pretty(v):return (json.dumps(v,ensure_ascii=True,indent=2,allow_nan=False)+'\n').encode('ascii')
def fields(v,keys):require(type(v) is dict and set(v)==set(keys.split()));return v
def integer(v,lo=0,hi=SAFE):return type(v) is int and lo<=v<=hi
def ident(v):return type(v) is str and re.fullmatch('[0-9a-f]{64}',v) is not None
def optional(v):return v=='' or ident(v)
def bound(v,n):require(len(canon(v))<=n)
def label(case,kind):return sha(b'SLUG Set66 wire fixture\0'+kind.encode()+b'\0'+case.encode())


def strict_wire(raw,limit):
    require(type(raw) is bytes and 0<len(raw)<=limit)
    quoted=escaped=False;depth=0
    for ch in raw:
        if quoted:
            if escaped:escaped=False
            elif ch==92:escaped=True
            elif ch==34:quoted=False
        elif ch==34:quoted=True
        elif ch in (91,123):depth+=1;require(depth<=8)
        elif ch in (93,125):depth-=1;require(depth>=0)
    require(not quoted and depth==0)
    def pairs(values):
        out={}
        for k,v in values:require(k not in out);out[k]=v
        return out
    def number(text):
        require(re.fullmatch('0|[1-9][0-9]*',text) is not None and len(text)<=16)
        value=int(text);require(value<=SAFE);return value
    def forbidden(_):raise Refusal('noninteger')
    return json.loads(raw.decode('utf-8'),object_pairs_hook=pairs,parse_int=number,parse_float=forbidden,parse_constant=forbidden)


def context(case):
    archive=json.loads(case['canonical']);c=archive['capture'];pin=case['expected_pin'];name=case['name']
    origin='http://127.0.0.1:19666';raw=origin.encode()
    trust=sha(b'SLUG manifest transport trust\0v1\0'+len(raw).to_bytes(4,'little')+raw+b'\1'+bytes(4))
    view=dict(version=1,view_id=label(name,'view'),account_authority_id=pin['account_authority_id'],
              account_id=pin['account_id'],generation=7,authority_pin=pin['source'],authority_origin=origin,transport_trust_id=trust)
    summary=dict(version=1,file_sha256=case['report']['file_sha256'],file_bytes=case['report']['file_bytes'],
                 prospect_id=c['assignment']['prospect_id'],history_id=c['history_id'],
                 source_record_digest=c['assignment']['source_record_digest'],event_count=c['event_count'],captured_as_of_ms=c['as_of_ms'])
    return dict(name=name,scope='Synthetic private-wire authority/selection over independently checked actual public capture; never an actual login or native file selection',
                view=view,store_id=c['status']['store_id'],history_job_id=label(name,'history'),history_id=c['history_id'],
                job_id=label(name,'evidence'),prospect_id=c['assignment']['prospect_id'],summary=summary,
                selection=dict(version=1,mode='create',path='/tmp/slug-public-evidence-fixture.json',parent=dict(device='0000000000000001',inode='0000000000000002'),file=None))


def view(v,c):
    fields(v,'version view_id account_authority_id account_id generation authority_pin authority_origin transport_trust_id')
    fields(v['authority_pin'],'version network genesis authority_id descriptor_id')
    require(type(v['version']) is int and v['version']==1 and integer(v['generation'],1))
    require(type(v['authority_pin']['version']) is int and v['authority_pin']['version']==5 and integer(v['authority_pin']['network'],1,(1<<32)-1))
    require(v==c['view'])


def selection(v):
    fields(v,'version mode path parent file');fields(v['parent'],'device inode')
    require(type(v['version']) is int and v['version']==1 and v['mode']=='create' and v['file'] is None)
    require(type(v['path']) is str and v['path'].startswith('/') and len(v['path'].encode())<=2048)
    require(all(type(v['parent'][k]) is str and re.fullmatch('[0-9a-f]{16}',v['parent'][k]) for k in ('device','inode')))
    require(v['parent']['inode']!='0000000000000000')


def request(v,c):
    fields(v,REQUEST_FIELDS);bound(v,4096)
    require(integer(v['id'],1) and type(v['version']) is int and v['version']==1)
    require(all(type(v[k]) is str for k in ('command','view_id','history_job_id','history_id','job_id')))
    command=v['command']
    if command=='fighter_evidence_snapshot':
        require(v['view_id'] in ('',c['view']['view_id']) and not v['history_job_id'] and not v['history_id'] and not v['job_id'] and v['selection'] is None)
    elif command=='fighter_evidence_begin':
        require(all(v[k]==c[k] for k in ('history_job_id','history_id','job_id')) and v['view_id']==c['view']['view_id'])
        selection(v['selection'])
    elif command in ('fighter_evidence_job','fighter_evidence_cancel'):
        require(v['view_id']==c['view']['view_id'] and v['job_id']==c['job_id'] and not v['history_job_id'] and not v['history_id'] and v['selection'] is None)
    else:raise Refusal('unknown_command')
    return v


def summary(v,c):
    fields(v,SUMMARY_FIELDS);bound(v,2048)
    require(type(v['version']) is int and v['version']==1 and ident(v['file_sha256']))
    require(integer(v['file_bytes'],1,524288) and integer(v['event_count'],0,256) and integer(v['captured_as_of_ms'],1,DATE_MAX))
    require(all(ident(v[k]) for k in ('prospect_id','history_id','source_record_digest')))
    # File bytes/hash are newly produced metadata, not known before export.
    # Check original capture links here; terminal immutability below fixes an
    # already accepted checksum. The exact positive bytes remain independently
    # derived from the core canonical archive, never from production output.
    require(all(v[k]==c['summary'][k] for k in ('version','prospect_id','history_id','source_record_digest','event_count','captured_as_of_ms')))
    return v


def snapshot(v,c):
    fields(v,SNAPSHOT_FIELDS);bound(v,4096)
    require(type(v['version']) is int and v['version']==1)
    require(all(optional(v[k]) for k in ('active_job_id','last_job_id')))
    if v['state']=='unavailable':
        require(v['error']=='evidence_unavailable' and v['view'] is None and not v['store_id'] and not v['active_job_id'] and not v['last_job_id'])
    else:
        require(v['state'] in ('idle','busy') and v['error']=='' and v['store_id']==c['store_id']);view(v['view'],c)
        require((v['state']=='busy')==bool(v['active_job_id']))
        # Model native's admitted original operation; do not discover a raw
        # helper "latest" operation while a different begin is unresolved.
        require(all(v[k] in ('',c['job_id']) for k in ('active_job_id','last_job_id')))
    return v


def job(v,c,previous=None):
    fields(v,JOB_FIELDS);bound(v,4096);view(v['view'],c)
    require(type(v['version']) is int and v['version']==1 and v['store_id']==c['store_id'])
    require(all(ident(v[k]) and v[k]==c[k] for k in ('job_id','history_job_id','history_id','prospect_id')))
    state=v['state']
    if state=='done':require(v['error']=='');summary(v['summary'],c)
    elif state=='running':require(v['error']=='' and v['summary'] is None)
    elif state=='failed':require(v['error'] in TERMINAL and v['summary'] is None)
    elif state=='cancelled':require(v['error']=='evidence_cancelled' and v['summary'] is None)
    else:raise Refusal('state')
    if previous and previous['state']!='running':require(v==previous)
    return v


def response(v,c,command,request_id=1,previous=None):
    fields(v,RESPONSE_FIELDS);bound(v,8192)
    require(integer(v['id'],1) and v['id']==request_id and type(v['version']) is int and v['version']==1 and type(v['ok']) is bool)
    if not v['ok']:
        require(v['snapshot'] is None and v['job'] is None and v['error'] in ERRORS)
        return v
    require(v['error']=='')
    if command=='fighter_evidence_snapshot':require(v['job'] is None);snapshot(v['snapshot'],c)
    else:require(v['snapshot'] is None);job(v['job'],c,previous)
    return v


def make_request(c,command,discovery=False):
    v=dict(id=1,version=1,command=command,view_id='' if discovery else c['view']['view_id'],history_job_id='',history_id='',job_id='',selection=None)
    if command=='fighter_evidence_begin':v.update(history_job_id=c['history_job_id'],history_id=c['history_id'],job_id=c['job_id'],selection=clone(c['selection']))
    elif command!='fighter_evidence_snapshot':v['job_id']=c['job_id']
    return v


def make_job(c,state='running',error=''):
    return dict(version=1,job_id=c['job_id'],state=state,view=clone(c['view']),store_id=c['store_id'],history_job_id=c['history_job_id'],history_id=c['history_id'],prospect_id=c['prospect_id'],error=error,summary=clone(c['summary']) if state=='done' else None)


def make_snapshot(c,state='idle',last=False):
    if state=='unavailable':return dict(version=1,state=state,error='evidence_unavailable',view=None,store_id='',active_job_id='',last_job_id='')
    return dict(version=1,state=state,error='',view=clone(c['view']),store_id=c['store_id'],active_job_id=c['job_id'] if state=='busy' else '',last_job_id=c['job_id'] if last else '')


def make_response(kind,value,ok=True,error=''):
    return dict(id=1,version=1,ok=ok,snapshot=value if kind=='snapshot' else None,job=value if kind=='job' else None,error=error)


def build(core):
    selected=[context(next(v for v in core['positives'] if v['name']==name)) for name in CASES]
    positives=[];negatives=[]
    def positive(name,kind,value,c,command='',previous=None):
        {'request':lambda:request(value,c),'snapshot':lambda:snapshot(value,c),'job':lambda:job(value,c,previous),
         'summary':lambda:summary(value,c),'response':lambda:response(value,c,command,previous=previous)}[kind]()
        positives.append(dict(name=name,kind=kind,context_name=c['name'],command=command,previous=previous,value=value,canonical=canon(value).decode(),sha256=sha(canon(value))))
    def negative(name,kind,value,c,command='',previous=None):
        try:
            {'request':lambda:request(value,c),'snapshot':lambda:snapshot(value,c),'job':lambda:job(value,c,previous),
             'summary':lambda:summary(value,c),'response':lambda:response(value,c,command,previous=previous)}[kind]()
        except (Refusal,TypeError,KeyError,ValueError):
            negatives.append(dict(name=name,kind=kind,context_name=c['name'],command=command,previous=previous,value=value,canonical=canon(value).decode(),expected='reject'));return
        raise AssertionError('negative unexpectedly accepted '+name)
    for c in selected:
        prefix=c['name']+'/'
        for command in ('fighter_evidence_snapshot','fighter_evidence_begin','fighter_evidence_job','fighter_evidence_cancel'):
            positive(prefix+command,'request',make_request(c,command),c)
        positive(prefix+'snapshot_discovery','request',make_request(c,'fighter_evidence_snapshot',True),c)
        q=make_request(c,'fighter_evidence_snapshot');q['id']=SAFE;positive(prefix+'maximum_correlation','request',q,c)
        for state,last in (('unavailable',False),('idle',False),('busy',False),('idle',True)):
            snap=make_snapshot(c,state,last);positive(prefix+'snapshot_'+state+str(last),'snapshot',snap,c)
            positive(prefix+'response_snapshot_'+state+str(last),'response',make_response('snapshot',snap),c,'fighter_evidence_snapshot')
        positive(prefix+'summary','summary',clone(c['summary']),c)
        for state,error in [('running',''),('done',''),('cancelled','evidence_cancelled')]+[('failed',e) for e in TERMINAL]:
            j=make_job(c,state,error);positive(prefix+'job_'+state+error,'job',j,c)
            positive(prefix+'response_job_'+state+error,'response',make_response('job',j),c,'fighter_evidence_job')
    c=selected[0];done=make_job(c,'done');running=make_job(c)
    for code in ERRORS:positive('error/'+code,'response',make_response('',None,False,code),c,'fighter_evidence_begin')
    controls=[]
    for value in (0,SAFE+1,True):
        q=make_request(c,'fighter_evidence_begin');q['id']=value;controls.append(('request_id_'+str(value),'request',q,'',None))
    for key in ('history_id','history_job_id','job_id','view_id'):
        q=make_request(c,'fighter_evidence_begin');q[key]='f'*64;controls.append(('request_foreign_'+key,'request',q,'',None))
    q=make_request(c,'fighter_evidence_job');q['selection']=clone(c['selection']);controls.append(('job_request_selection','request',q,'',None))
    q=make_request(c,'fighter_evidence_begin');q['selection']=None;controls.append(('begin_without_selection','request',q,'',None))
    q=make_request(c,'fighter_evidence_begin');q['selection']['mode']='read';controls.append(('begin_read_selection','request',q,'',None))
    q=make_request(c,'fighter_evidence_begin');q['path']='/tmp/extra';controls.append(('extra_plain_path','request',q,'',None))
    for key,value in [('file_bytes',0),('file_bytes',524289),('event_count',257),('captured_as_of_ms',c['summary']['captured_as_of_ms']+1),('history_id','f'*64),('source_record_digest','f'*64)]:
        q=clone(c['summary']);q[key]=value;controls.append(('summary_'+key+str(value),'summary',q,'',None))
    q=clone(c['summary']);q['checks']={};controls.append(('summary_offline_report','summary',q,'',None))
    q=clone(c['summary']);q['path']='/tmp/private';controls.append(('summary_path','summary',q,'',None))
    for key in ('history_id','history_job_id','prospect_id','job_id','store_id'):
        q=clone(done);q[key]='f'*64;controls.append(('job_foreign_'+key,'job',q,'',None))
    q=clone(done);q['view']=clone(selected[1]['view']);controls.append(('job_foreign_full_view','job',q,'',None))
    q=clone(done);q['summary']=None;controls.append(('done_no_summary','job',q,'',None))
    q=clone(running);q['summary']=clone(c['summary']);controls.append(('running_summary','job',q,'',None))
    q=make_job(c,'failed','busy');controls.append(('late_capacity_or_busy','job',q,'',None))
    q=make_job(c,'cancelled','');controls.append(('cancelled_no_code','job',q,'',None))
    q=clone(done);q['state']='running';q['summary']=None;controls.append(('terminal_regression','job',q,'',done))
    q=clone(done);q['summary']['file_sha256']='f'*64;controls.append(('terminal_checksum_mutation','job',q,'',done))
    q=make_snapshot(c,'unavailable');q['error']='unauthorized';controls.append(('unavailable_auth_payload','snapshot',q,'',None))
    q=make_snapshot(c,'unavailable');q['store_id']=c['store_id'];controls.append(('unavailable_source_leak','snapshot',q,'',None))
    q=make_snapshot(c,'busy');q['active_job_id']='f'*64;controls.append(('unknown_latest_not_adopted','snapshot',q,'',None))
    q=make_snapshot(c);q['active_job_id']=c['job_id'];controls.append(('idle_active','snapshot',q,'',None))
    q=make_response('job',done);q['snapshot']=make_snapshot(c);controls.append(('two_response_arms','response',q,'fighter_evidence_job',None))
    q=make_response('job',done,False,'outcome_unknown');controls.append(('error_payload_leak','response',q,'fighter_evidence_begin',None))
    q=make_response('job',done);q['id']=2;controls.append(('wrong_response_correlation','response',q,'fighter_evidence_job',None))
    for name,kind,value,command,previous in controls:negative(name,kind,value,c,command,previous)
    raw_negatives=[]
    request_raw=canon(make_request(c,'fighter_evidence_begin'));response_raw=canon(make_response('job',done))
    for name,kind,raw,limit in (
        ('duplicate_request_id','request',request_raw.replace(b'"id":1,',b'"id":1,"id":1,',1),4096),
        ('duplicate_response_version','response',response_raw.replace(b'"version":1,',b'"version":1,"version":1,',1),8192),
        ('request_trailing_value','request',request_raw+b'{}',4096),
        ('response_overlimit','response',b' '*8193,8192)):
        try:strict_wire(raw,limit)
        except (Refusal,ValueError,UnicodeError):raw_negatives.append(dict(name=name,kind=kind,raw=raw.decode('ascii'),expected='reject'));continue
        raise AssertionError('raw control unexpectedly accepted '+name)
    require(len(positives)==112 and len(negatives)==38 and len(raw_negatives)==4)
    return dict(version=1,contract_sha256=CONTRACT,core_corpus_sha256=CORE,
                scope='Independent synthetic private-wire/correlation fixtures over frozen actual public captures; no native session/file authority or work execution',
                contexts=selected,positives=positives,negatives=negatives,raw_negatives=raw_negatives)


def plan():
    return dict(version=1,contract_sha256=CONTRACT,core_corpus_sha256=CORE,source_cases=list(CASES),
                synthetic_domain_hex=b'SLUG Set66 wire fixture\0'.hex(),synthetic_origin='http://127.0.0.1:19666',synthetic_generation=7,
                synthetic_selection_path='/tmp/slug-public-evidence-fixture.json',synthetic_parent={'device':'0000000000000001','inode':'0000000000000002'},
                request_commands=['fighter_evidence_snapshot','fighter_evidence_begin','fighter_evidence_job','fighter_evidence_cancel'],
                envelope_errors=list(ERRORS),terminal_errors=list(TERMINAL),
                controls='All named finite controls are fixed in wire_reference.py before execution; source hash binds every mutation.',
                planned_positive_count=112,planned_negative_count=38,planned_raw_negative_count=4,
                normal_work_calls=0,production_implementation_used=False)


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--repo-root',type=Path);p.add_argument('--lock',type=Path,required=True);p.add_argument('--attempt',type=Path,required=True);p.add_argument('--check',type=Path);a=p.parse_args()
    lockraw=a.lock.read_bytes();lock=json.loads(lockraw);require(lock['execution_authorized_by_root'] is True)
    for name,row in lock['files'].items():require(sha((HERE/name).read_bytes())==row['sha256'])
    require(sha((HERE/'contract.md').read_bytes())==CONTRACT and json.loads((HERE/'wire-plan.json').read_bytes())==plan())
    root=a.repo_root or next((p for p in HERE.parents if (p/'go.mod').is_file() and (p/'fighterevidence/testdata/reference/reference-v1.json').is_file()),None)
    require(root is not None)
    corepath=root/'fighterevidence/testdata/reference/reference-v1.json';raw=corepath.read_bytes();require(sha(raw)==CORE)
    a.attempt.mkdir(parents=True,exist_ok=False)
    (a.attempt/'producer.py').write_bytes((HERE/'wire_reference.py').read_bytes());(a.attempt/'execution-lock.json').write_bytes(lockraw)
    try:
        out=build(json.loads(raw));out['execution_lock_sha256']=sha(lockraw);encoded=pretty(out);(a.attempt/'wire-v1.json').write_bytes(encoded)
        if a.check:require(a.check.read_bytes()==encoded)
        for name,row in lock['files'].items():require(sha((HERE/name).read_bytes())==row['sha256'])
        report=dict(passed=True,bytes=len(encoded),sha256=sha(encoded),positives=len(out['positives']),negatives=len(out['negatives']),raw_negatives=len(out['raw_negatives']),normal_work_calls=0,python=sys.version)
        (a.attempt/'report.json').write_bytes(pretty(report));print(json.dumps(report))
    except BaseException as e:
        import traceback
        (a.attempt/'failure.txt').write_text(traceback.format_exc());(a.attempt/'report.json').write_bytes(pretty(dict(passed=False,error_type=type(e).__name__,error=str(e))))
        raise


if __name__=='__main__':main()
