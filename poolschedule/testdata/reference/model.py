#!/usr/bin/env python3
"""Independent scheduler v1 grammar model. No Go outputs or product imports."""
import copy, datetime as dt, hashlib, json, re
TIME = 253402300799999
UINT = 4294967295
CONFIG = ['version','scheduler_id','runtime_id','settlement_binding_id','daily_utc_minute','retry_ms']
STATE = ['version','config','revision','last_clock_ms','daily_cursor','next_wake_ms','attempt']
ATTEMPT = ['sequence','kind','admitted_day','started_at_ms','finished_at_ms','result','report_state','report_reason','error_code']
REASONS = ['', 'payout-pending','broadcast-unknown','transaction-unavailable','payout-conflict','funding-unavailable','funding-conflict','funding-changed','immature','awaiting-confirmation','below-minimum','destination-unavailable','daily-batch-complete','fee-unavailable','fee-budget','change-dust','signing-unavailable','capacity']
ERRORS = ['busy','unavailable','clock','storage','closed','profile','invalid','cancelled','deadline','other']
def raw(v): return json.dumps(v,ensure_ascii=True,separators=(',',':')).encode('ascii')
def integer(n, lo, hi): return type(n) is int and lo <= n <= hi
def day(ms): return (dt.datetime(1970,1,1,tzinfo=dt.timezone.utc)+dt.timedelta(milliseconds=ms)).strftime('%Y-%m-%d')
def valid_day(s):
    try: return type(s) is str and re.fullmatch('[0-9]{4}-[0-9]{2}-[0-9]{2}',s) is not None and dt.date.fromisoformat(s).isoformat()==s
    except (ValueError,TypeError): return False
def ident(s): return type(s) is str and re.fullmatch('[0-9a-f]{64}',s) is not None and s!='0'*64
def config(v):
    return type(v) is dict and list(v)==CONFIG and v['version']==1 and type(v['version']) is int and all(ident(v[k]) for k in CONFIG[1:4]) and integer(v['daily_utc_minute'],0,1439) and integer(v['retry_ms'],1000,3600000)
def state(v):
    if type(v) is not dict or list(v)!=STATE or type(v['version']) is not int or v['version']!=1 or not config(v['config']) or not integer(v['revision'],1,UINT) or not integer(v['last_clock_ms'],1,TIME): return False
    cursor=v['daily_cursor']; clock=v['last_clock_ms']; wake=v['next_wake_ms']; a=v['attempt']
    if type(cursor) is not str or (cursor and (not valid_day(cursor) or cursor>day(clock))): return False
    if not integer(wake,0,TIME) or (wake and wake<=clock): return False
    if a is None: return v['revision']==1 and cursor=='' and wake==0
    if type(a) is not dict or list(a)!=ATTEMPT or not integer(a['sequence'],1,v['revision']) or a['kind'] not in ['recovery','daily'] or not integer(a['started_at_ms'],1,clock) or a['admitted_day']!=day(a['started_at_ms']): return False
    if wake==0 and not (a['kind']=='recovery' and a['result']=='running'): return False
    if a['result']=='running': return a['finished_at_ms']==0 and type(a['finished_at_ms']) is int and a['report_state']==a['report_reason']==a['error_code']==''
    if not integer(a['finished_at_ms'],a['started_at_ms'],clock): return False
    if a['result']=='returned': return a['report_state'] in ['complete','pending','deferred'] and a['report_reason'] in REASONS and a['error_code']==''
    if a['result']=='failed': return a['report_state']==a['report_reason']=='' and a['error_code'] in ERRORS
    return False

def decode(data,kind):
    cap=2048 if kind=='marker' else 4096
    if not isinstance(data,bytes) or not 0<len(data)<=cap or any(x>127 for x in data): return False
    if kind=='marker':
        if not data.endswith(b'\n'): return False
        data=data[:-1]
    try:
        def pairs(items):
            d={}
            for k,v in items:
                if k in d: raise ValueError('duplicate')
                d[k]=v
            return d
        v=json.loads(data,object_pairs_hook=pairs,parse_constant=lambda _: (_ for _ in ()).throw(ValueError()))
        count=[0]
        def walk(x,depth):
            count[0]+=1
            if count[0]>128 or depth>4: raise ValueError('budget')
            if isinstance(x,list): raise ValueError('array')
            if isinstance(x,dict):
                if len(x)>16: raise ValueError('members')
                for k,y in x.items(): walk(k,depth+1); walk(y,depth+1)
            elif isinstance(x,str) and (len(x)>64 or any(ord(c)>127 for c in x)): raise ValueError('string')
        walk(v,0)
        return raw(v)==data and (config(v) if kind=='marker' else state(v))
    except (ValueError,TypeError,OverflowError): return False

def finish(kind, admitted, finish_ms, report_day, report_state, reason, cursor):
    if kind not in ['recovery','daily'] or not integer(admitted,1,TIME) or not integer(finish_ms,admitted,TIME) or not valid_day(report_day) or report_state not in ['complete','pending','deferred'] or reason not in REASONS: return None
    if kind=='daily' and report_day>day(finish_ms): return None
    if kind=='daily' and report_state=='complete' and reason=='daily-batch-complete': return max(cursor,report_day)
    return cursor

def generate():
    c=dict(zip(CONFIG,[1,'11'*32,'22'*32,'33'*32,720,30000]))
    now=1893499200000 # 2030-01-01 12:00:00Z, literal public operational clock
    assert day(now)=='2030-01-01'
    initial=dict(zip(STATE,[1,c,1,now,'',0,None]))
    attempt=dict(zip(ATTEMPT,[1,'recovery','2030-01-01',now,0,'running','','','']))
    running=copy.deepcopy(initial);running.update(revision=2,attempt=attempt)
    returned=copy.deepcopy(running);returned.update(revision=3,last_clock_ms=now+1,next_wake_ms=now+30001);returned['attempt'].update(finished_at_ms=now+1,result='returned',report_state='complete')
    positives=[]; negatives=[]
    def add(name,v,kind='state'): positives.append({'name':name,'kind':kind,'raw_hex':(raw(v)+(b'\n' if kind=='marker' else b'')).hex()})
    def bad(name,v,kind='state'): negatives.append({'name':name,'kind':kind,'raw_hex':(raw(v)+(b'\n' if kind=='marker' else b'')).hex()})
    add('marker-default',c,'marker');add('initial',initial);add('interrupted-recovery',running);add('recovery-drained-does-not-occupy-day',returned)
    for state_name in ['pending','deferred']:
        r=copy.deepcopy(returned);r['attempt'].update(report_state=state_name,report_reason='payout-pending' if state_name=='pending' else 'immature');add('recovery-'+state_name,r)
    daily=copy.deepcopy(returned);daily['attempt'].update(kind='daily',report_reason='daily-batch-complete');daily['daily_cursor']='2030-01-01';add('daily-completed',daily)
    r=copy.deepcopy(returned);r.update(revision=UINT);r['attempt']['sequence']=UINT;add('counter-last-value',r)
    r=copy.deepcopy(returned);r.update(last_clock_ms=TIME-1,next_wake_ms=TIME);add('clock-upper-bound',r)
    for e in ERRORS:
        r=copy.deepcopy(returned);r['attempt'].update(result='failed',report_state='',error_code=e);add('failed-'+e,r)
    for reason in REASONS:
        r=copy.deepcopy(returned);r['attempt']['report_reason']=reason;add('closed-reason-'+(reason or 'empty'),r)
    for label,key,value in [('id-zero','scheduler_id','0'*64),('id-upper','runtime_id','AA'*32),('minute-high','daily_utc_minute',1440),('retry-low','retry_ms',999),('retry-high','retry_ms',3600001),('version','version',2),('boolean','version',True)]:
        r=copy.deepcopy(c);assert r[key]!=value or type(r[key])!=type(value);r[key]=value;bad(label,r,'marker')
    mutations=[('zero-clock','last_clock_ms',0),('clock-overflow','last_clock_ms',TIME+1),('revision-overflow','revision',UINT+1),('future-cursor','daily_cursor','2030-01-02'),('invalid-date','daily_cursor','2030-02-30'),('timer-not-future','next_wake_ms',now),('unexpected-zero-wake','next_wake_ms',0)]
    for label,key,value in mutations:
        r=copy.deepcopy(returned);assert r[key]!=value;r[key]=value;bad(label,r)
    for label,key,value in [('sequence-over-revision','sequence',4),('zero-sequence','sequence',0),('admitted-day','admitted_day','2030-01-02'),('future-start','started_at_ms',now+2),('finish-before-start','finished_at_ms',now-1),('future-finish','finished_at_ms',now+2),('unknown-result','result','complete'),('unknown-reason','report_reason','paid'),('returned-error','error_code','busy')]:
        r=copy.deepcopy(returned);assert r['attempt'][key]!=value;r['attempt'][key]=value;bad(label,r)
    r=copy.deepcopy(running);r['attempt']['report_state']='pending';bad('running-report',r)
    r=copy.deepcopy(returned);r['attempt'].update(result='failed',report_state='',error_code='random');bad('unknown-failure',r)
    r=copy.deepcopy(initial);r['revision']=2;bad('missing-attempt',r)
    base=raw(returned)
    for name,data in [('duplicate',base.replace(b'"version":1',b'"version":1,"version":1',1)),('whitespace',b' '+base),('trailing',base+b'\n'),('reordered',raw(dict(reversed(list(returned.items()))))),('array',base.replace(b'"attempt":{',b'"attempt":[{').replace(b'"error_code":""}}',b'"error_code":""}]}')),('escaped-key',base.replace(b'"revision"',b'"revis\\u0069on"')),('non-ascii',base.replace(b'recovery',b'\xc3\xa9')),('extra-field',base[:-1]+b',"extra":0}'),('oversize',b' '*4097)]:
        negatives.append({'name':name,'kind':'state','raw_hex':data.hex()})
    finish_cases=[]
    for name,kind,end,rd,rs,reason,cursor,expected in [
        ('recovery-no-day','recovery',now+1,'2030-01-01','complete','','',''),
        ('deferred-no-day','daily',now+1,'2030-01-01','deferred','immature','',''),
        ('completed-day','daily',now+1,'2030-01-01','complete','daily-batch-complete','','2030-01-01'),
        ('future-report-refused','daily',now+1,'2030-01-02','complete','daily-batch-complete','',None),
        ('actual-midnight-preparation-day','daily',now+12*3600000+1,'2030-01-02','complete','daily-batch-complete','','2030-01-02'),
        ('cursor-never-decreases','daily',now+1,'2029-12-31','complete','daily-batch-complete','2030-01-01','2030-01-01')]:
        args=[kind,now,end,rd,rs,reason,cursor];assert finish(*args)==expected;finish_cases.append({'name':name,'args':args,'expected_cursor':expected})
    for x in positives: assert decode(bytes.fromhex(x['raw_hex']),x['kind']),x['name']
    for x in negatives: assert not decode(bytes.fromhex(x['raw_hex']),x['kind']),x['name']
    return {'version':1,'description':'Independent scheduler grammar only; opaque logical binding ID, no financial authority or Go-derived expectations.','positives':positives,'refusals':negatives,'finish_cases':finish_cases}
if __name__=='__main__':
    import pathlib,sys
    if len(sys.argv)!=2: raise SystemExit('explicit output path required')
    pathlib.Path(sys.argv[1]).write_bytes(raw(generate())+b'\n')
