"""Conditional daily balance/page projection; no fabricated signed history."""
from datetime import datetime, timezone
from core import *

BALANCE_FIELDS='gym_id,confirmed,available,reserved_atoms,paid_atoms,reason'
PAYMENT_FIELDS='day,created_at_ms,txid,output_index,atoms,recorded_state'
PAGE_FIELDS='version,binding_id,settlement_config_id,revision,high_batch,before_batch,next_before,done,recipient_script,balance,observed_at_ms,items'

def balance(b, gym):
    fields(b,BALANCE_FIELDS);require(b['gym_id']==gym);hexstr(gym,64)
    confirmed,available=fraction(b['confirmed']),fraction(b['available'])
    reserved=decimal(b['reserved_atoms'],MAX_I64);decimal(b['paid_atoms'],MAX_I64)
    require(available<=confirmed-reserved and b['reason'] in REASONS)
    require(confirmed!=0 or b['reason']=='')
    require(len(canon(b))<=17408,'size')

def empty_balance(gym):
    return dict(gym_id=gym,confirmed=frac(0),available=frac(0),reserved_atoms='0',paid_atoms='0',reason='')

def money_prefix(v, expected):
    integer(v['version'],2,2)
    for name in ('binding_id','settlement_config_id','recipient_script'):
        require(v[name]==expected[name],'binding')

def balance_response(v, expected):
    fields(v,'version,binding_id,settlement_config_id,recipient_script,balance,observed_at_ms')
    money_prefix(v,expected);balance(v['balance'],expected['gym_id']);integer(v['observed_at_ms'],0,MAX_TIME)
    if v['observed_at_ms']==0:require(v['balance']==empty_balance(expected['gym_id']))
    require(len(canon(v))+1<=65536,'size')

def request(q):
    fields(q,'revision,high_batch,before_batch')
    r,h,b=(decimal(q[x]) for x in ('revision','high_batch','before_batch'))
    require((r,h,b)==(0,0,0) or (r>0 and 1<=h<=32767 and 1<=b<=h+1),'cursor')
    require(len(canon(q))<=1024,'size')
    return r,h,b

def payment(p, observed):
    fields(p,PAYMENT_FIELDS);integer(p['created_at_ms'],1,min(observed,MAX_TIME))
    day=datetime.fromtimestamp(p['created_at_ms']//1000,timezone.utc).strftime('%Y-%m-%d')
    require(p['day']==day);hexstr(p['txid'],64);integer(p['output_index'],0,255)
    integer(p['atoms'],100000000,MAX_MONEY)
    require(p['recorded_state'] in ('prepared','broadcast','confirmed'))
    require(len(canon(p))<=256,'size')

def page(v, expected, q):
    fields(v,PAGE_FIELDS);money_prefix(v,expected);balance(v['balance'],expected['gym_id'])
    integer(v['observed_at_ms'],0,MAX_TIME)
    rv,hv,bv,nv=(decimal(v[x]) for x in ('revision','high_batch','before_batch','next_before'))
    r,h,b=request(q);require(rv>0 and hv<=32767 and 1<=bv<=hv+1,'capture')
    if r:require((rv,hv,bv)==(r,h,b),'capture')
    else:require(bv==hv+1,'fresh cursor')
    require(type(v['done']) is bool and type(v['items']) is list and len(v['items'])<=8)
    low=max(1,bv-8); visited=max(0,bv-1)
    done=(visited==0 or low==1)
    require(v['done']==done and nv==(0 if done else low),'next cursor')
    last=bv;txids=set();paid=reserved=0;previous=None
    for row in v['items']:
        fields(row,'batch_sequence,payment');seq=decimal(row['batch_sequence'])
        require(low<=seq<last and seq<bv,'slot order');last=seq
        p=row['payment'];payment(p,v['observed_at_ms']);require(p['txid'] not in txids);txids.add(p['txid'])
        if previous:require(p['day']<previous['day'] and p['created_at_ms']<previous['created_at_ms'],'day/time order')
        previous=p
        if p['recorded_state']=='confirmed':paid+=p['atoms']
        else:require(seq==hv,'pending is global high');reserved+=p['atoms']
    require(paid<=int(v['balance']['paid_atoms']) and reserved<=int(v['balance']['reserved_atoms']),'page sums')
    if v['observed_at_ms']==0:require(hv==0 and not v['items'] and v['balance']==empty_balance(expected['gym_id']))
    require(len(canon(v))+1<=131072,'size')

def conditional_slots(n,gym,other,sparse=False,pending=''):
    slots=[];start=1735689600000
    for i in range(1,n+1):
        ms=start+(i-1)*86400000
        p=dict(day=datetime.fromtimestamp(ms//1000,timezone.utc).strftime('%Y-%m-%d'),created_at_ms=ms,
               txid=sha(('SLUG Set79 conditional payout slot '+str(i)).encode('ascii')),
               output_index=(i-1)%256,atoms=100000000,recorded_state=pending if pending and i==n else 'confirmed')
        slots.append(dict(sequence=i,gym_id=(gym if not sparse or i==1 else other),payment=p))
    return slots

def project(slots,expected,revision=7,before=0):
    n=len(slots);before=before or n+1;selected=[s for s in slots if s['gym_id']==expected['gym_id']]
    paid=sum(s['payment']['atoms'] for s in selected if s['payment']['recorded_state']=='confirmed')
    reserved=sum(s['payment']['atoms'] for s in selected if s['payment']['recorded_state']!='confirmed')
    b=empty_balance(expected['gym_id']);b.update(confirmed=frac(reserved),reserved_atoms=str(reserved),paid_atoms=str(paid),reason='payout-pending' if reserved else '')
    low=max(1,before-8);done=before==1 or low==1
    rows=[dict(batch_sequence=str(s['sequence']),payment=clone(s['payment'])) for s in reversed(slots) if low<=s['sequence']<before and s['gym_id']==expected['gym_id']]
    observed=max([s['payment']['created_at_ms'] for s in slots]+[1735689600000])+1000
    return dict(version=2,binding_id=expected['binding_id'],settlement_config_id=expected['settlement_config_id'],revision=str(revision),high_batch=str(n),before_batch=str(before),next_before=str(0 if done else low),done=done,recipient_script=expected['recipient_script'],balance=b,observed_at_ms=observed,items=rows)

def produce_payments(expected):
    b=empty_balance(expected['gym_id']);bc=Cases()
    envelope=lambda x,at=1735689600000:dict(version=2,binding_id=expected['binding_id'],settlement_config_id=expected['settlement_config_id'],recipient_script=expected['recipient_script'],balance=x,observed_at_ms=at)
    check=lambda v:balance_response(v,expected)
    bc.add('exact-initial-empty',envelope(b,0),check)
    for n in (0,(1<<53)-1,1<<53,MAX_I64):
        x=clone(b);x['paid_atoms']=str(n);bc.add('paid-'+str(n),envelope(x),check,scope='conditional scalar codec')
    for n in ((1<<53)-1,1<<53,MAX_I64):
        x=clone(b);x.update(confirmed=frac(n),reserved_atoms=str(n),reason='payout-pending')
        bc.add('reserved-'+str(n),envelope(x),check,scope='conditional scalar codec')
    x=clone(b);x.update(confirmed=frac(MAX_MONEY+1),available=frac(MAX_MONEY+1),reason='below-minimum')
    bc.add('outstanding-above-v1-money-cap',envelope(x),check,scope='conditional scalar codec')
    huge=10**4095;x=clone(b);x.update(confirmed=frac(Fraction(huge+1,huge)),available=frac(Fraction(huge+1,huge)),reason='below-minimum')
    bc.add('four-4096-digit-components',envelope(x),check,scope='conditional scalar codec')
    for reason in REASONS:
        x=clone(b);x.update(confirmed=frac(1),reason=reason);bc.add('reason-'+(reason or 'empty'),envelope(x),check)
    for name,bad in [('over-int64',str(MAX_I64+1)),('negative','-1'),('leading-zero','01'),('exponent','1e3'),('numeric',1),('decimal','1.0')]:
        bc.bad(name,envelope(b),check,lambda v,bad=bad:v['balance'].update(paid_atoms=bad))
    for key in ('binding_id','settlement_config_id','recipient_script'):
        bc.bad('wrong-'+key,envelope(b),check,lambda v,key=key:v.update({key:'0'*len(v[key])}))
    bc.bad('zero-with-reason',envelope(b),check,lambda v:v['balance'].update(reason='immature'))
    bc.bad('observation-over-inherited-clock',envelope(b),check,lambda v:v.update(observed_at_ms=MAX_TIME+1))
    bc.bad('reserved-over-int64',envelope(b),check,lambda v:v['balance'].update(confirmed=frac(MAX_I64+1),reserved_atoms=str(MAX_I64+1),reason='payout-pending'))
    pc=Cases();fresh=dict(revision='0',high_batch='0',before_batch='0');other='ef'*32
    traversals=[]
    initial=project([],expected,revision=1);initial['observed_at_ms']=0
    pc.add('initial-empty-revision-one',initial,lambda v:page(v,expected,fresh),request=fresh)
    for n in (0,1,8,9,16,367):
        slots=conditional_slots(n,expected['gym_id'],other)
        first=project(slots,expected);pc.add('slots-'+str(n),first,lambda v,q=fresh:page(v,expected,q),scope='conditional unsigned slot projection',request=fresh)
        cursor=n+1;pages=[];all_items=[]
        while True:
            v=project(slots,expected,before=cursor);q=fresh if not pages else dict(revision='7',high_batch=str(n),before_batch=str(cursor))
            page(v,expected,q);pages.append(v);all_items.extend(v['items'])
            if v['done']:break
            cursor=int(v['next_before'])
        require([int(x['batch_sequence']) for x in all_items]==list(range(n,0,-1)))
        require(sum(x['payment']['atoms'] for x in all_items)==int(first['balance']['paid_atoms']))
        traversals.append(dict(name='complete-'+str(n),slot_count=n,pages=pages,paid_sum=str(sum(x['payment']['atoms'] for x in all_items))))
    for kind in ('prepared','broadcast'):
        v=project(conditional_slots(9,expected['gym_id'],other,pending=kind),expected)
        pc.add('global-high-'+kind,v,lambda x:page(x,expected,fresh),request=fresh)
    sparse=project(conditional_slots(17,expected['gym_id'],other,sparse=True),expected)
    require(not sparse['items'] and not sparse['done']);pc.add('empty-nonfinal-page',sparse,lambda v:page(v,expected,fresh),request=fresh)
    base=project(conditional_slots(16,expected['gym_id'],other),expected)
    q=dict(revision='7',high_batch='16',before_batch='8');arbitrary=project(conditional_slots(16,expected['gym_id'],other),expected,before=8)
    pc.add('arbitrary-valid-cursor',arbitrary,lambda v:page(v,expected,q),request=q)
    for name,mut in [('wrong-next',lambda v:v.update(next_before='8')),('premature-done',lambda v:v.update(done=True)),('duplicate-item',lambda v:v['items'].__setitem__(1,clone(v['items'][0]))),('wrong-money-binding',lambda v:v.update(settlement_config_id='0'*64)),('stale-response-revision',lambda v:v.update(revision='8')),('pending-not-global-high',lambda v:v['items'][-1]['payment'].update(recorded_state='prepared')),('duplicate-day-time',lambda v:v['items'][1]['payment'].update(day=v['items'][0]['payment']['day'],created_at_ms=v['items'][0]['payment']['created_at_ms']))]:
        cap=dict(revision='7',high_batch='16',before_batch='17')
        pc.bad(name,base,lambda v:page(v,expected,cap),mut,request=cap)
    pc.bad('observation-over-inherited-clock',base,lambda v:page(v,expected,fresh),lambda v:v.update(observed_at_ms=MAX_TIME+1),request=fresh)
    rc=Cases();rc.add('fresh',fresh,request)
    for name,q in [('empty-continuation',dict(revision='7',high_batch='0',before_batch='1')),('zero-revision-nonzero-tail',dict(revision='0',high_batch='1',before_batch='2')),('before-past-high',dict(revision='7',high_batch='8',before_batch='10')),('high-over-limit',dict(revision='7',high_batch='32768',before_batch='32769')),('revision-over-u64',dict(revision=str(MAX_U64+1),high_batch='1',before_batch='2'))]:rc.add(name,q,request,False)
    return dict(balances=bc.rows,pages=pc.rows,requests=rc.rows,traversals=traversals,
                stale_capture=dict(request=dict(revision='7',high_batch='16',before_batch='9'),current_revision='8',current_high_batch='16',error='stale_capture',outcome_unknown=False,items_disclosed=False))

def frame_bounds():
    # Maximal textual widths are conditional codec envelopes. They need not be
    # simultaneously reduced, spendable, or attainable in a retained ledger.
    f=dict(numerator='9'*4096,denominator='8'*4096)
    b=dict(gym_id='f'*64,confirmed=f,available=f,reserved_atoms=str(MAX_I64),paid_atoms=str(MAX_I64),reason=max(REASONS,key=len))
    item=dict(day='9999-12-31',created_at_ms=MAX_TIME,txid='f'*64,output_index=255,atoms=MAX_MONEY,recorded_state='confirmed')
    wrapped=dict(batch_sequence='32767',payment=item);items=[wrapped]*8
    response=dict(version=2,binding_id='f'*64,settlement_config_id='f'*64,recipient_script='0014'+'f'*40,balance=b,observed_at_ms=MAX_I64)
    p=dict(version=2,binding_id='f'*64,settlement_config_id='f'*64,revision=str(MAX_U64),high_batch='32767',before_batch='32768',next_before='32767',done=False,recipient_script='0014'+'f'*40,balance=b,observed_at_ms=MAX_I64,items=items)
    q=dict(revision=str(MAX_U64),high_batch='32767',before_batch='32768')
    # Raw embedded objects count once; no JSON string-escape expansion.
    work_overhead=len(canon(dict(version=2,binding_id='f'*64,settlement_config_id='f'*64,recipient_script='0014'+'f'*40,work={}))) - 2
    receipt_overhead=len(canon(dict(version=2,binding_id='f'*64,settlement_config_id='f'*64,recipient_script='0014'+'f'*40,submission={}))) - 2
    old_work=len(canon(dict(version=1,binding_id='f'*64,work={}))) - 2
    rows=dict(fraction=len(canon(f)),balance=len(canon(b)),payment=len(canon(item)),wrapped_payment=len(canon(wrapped)),eight_items=len(canon(items)),balance_response_lf=len(canon(response))+1,page_response_lf=len(canon(p))+1,request=len(canon(q)),money_binding_increment=work_overhead-old_work,work_response_lf=(1<<24)+8192+20+work_overhead+1,receipt_response_lf=4096+receipt_overhead+1)
    require(rows['balance']<=17408 and rows['payment']<=256 and rows['wrapped_payment']<=256 and rows['page_response_lf']<=131072 and rows['request']<=1024)
    require(rows['work_response_lf']<=(1<<24)+(64<<10) and rows['receipt_response_lf']<=64<<10)
    return dict(scope='conditional conservative codec widths only; not attainable financial state or observed I/O',widths=rows,literal_frames=dict(fraction=f,balance=b,payment=item,page=p,request=q))
