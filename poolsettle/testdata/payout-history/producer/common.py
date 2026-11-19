"""Independent Set67 reference helpers; stdlib only, no implementation imports."""
import copy, hashlib, json, math, re
from datetime import datetime, timezone
from fractions import Fraction

MAX_TIME=253402300799999
MAX_MONEY=2100000000000000
MIN_MONEY=100000000
MAX_SAFE=9007199254740991
REASONS=('', 'payout-pending','broadcast-unknown','transaction-unavailable','payout-conflict','funding-unavailable','funding-conflict','funding-changed','immature','awaiting-confirmation','below-minimum','destination-unavailable','daily-batch-complete','fee-unavailable','fee-budget','change-dust','signing-unavailable','capacity')

def require(condition):
    if not condition: raise ValueError('rejected')
def canon(value): return json.dumps(value,ensure_ascii=False,separators=(',',':'),allow_nan=False).encode('utf-8')
def sha(raw): return hashlib.sha256(raw).hexdigest()
def tagged(label): return sha(('SLUG Set67 independent modeled input\0v1\0'+label).encode('ascii'))
def clone(value): return copy.deepcopy(value)
def integer(v,low,high): require(type(v) is int and low<=v<=high)
def hex64(v,nonzero=False): require(type(v) is str and re.fullmatch('[0-9a-f]{64}',v) and (not nonzero or v!='0'*64))
def script(v,empty=False): require(type(v) is str and ((empty and v=='') or re.fullmatch('0014[0-9a-f]{40}',v)))
def fields(v,names,ordered=False):
    require(type(v) is dict and set(v)==set(names))
    if ordered: require(list(v)==list(names))
def date(ms):
    integer(ms,1,MAX_TIME)
    # Integer seconds retain exact year9999 boundary without float rounding.
    return datetime.fromtimestamp(ms//1000,timezone.utc).strftime('%Y-%m-%d')
def decimal(v,positive=False):
    require(type(v) is str and 1<=len(v)<=4096 and re.fullmatch('0|[1-9][0-9]*',v))
    n=int(v); require(n>0 if positive else n>=0); return n
def rational(v):
    fields(v,['numerator','denominator'])
    n,d=decimal(v['numerator']),decimal(v['denominator'],True)
    require(math.gcd(n,d)==1); return Fraction(n,d)
def frac(value):
    v=Fraction(value); return {'numerator':str(v.numerator),'denominator':str(v.denominator)}
def parse(raw,cap,canonical=False,floats=False):
    require(type(raw) is bytes and len(raw)<=cap)
    def pairs(xs):
        result={}
        for k,v in xs: require(k not in result); result[k]=v
        return result
    def pint(s): require(s!='-0'); return int(s)
    def pfloat(s):
        require(floats); v=float(s); require(math.isfinite(v)); return v
    def bad(s): raise ValueError('constant')
    v=json.loads(raw.decode('utf-8'),object_pairs_hook=pairs,parse_int=pint,parse_float=pfloat,parse_constant=bad)
    if canonical: require(raw.strip(b' \t\r\n')==canon(v))
    return v

ITEM_FIELDS=['day','created_at_ms','txid','output_index','atoms','recorded_state']
BALANCE_FIELDS=['gym_id','confirmed','available','reserved_atoms','paid_atoms','reason']
HISTORY_FIELDS=['version','recipient_script','balance','settlement_observed_at_ms','items']
def item(v,observed=MAX_TIME):
    fields(v,ITEM_FIELDS)
    require(type(v['day']) is str and len(v['day'])==10 and v['day']==date(v['created_at_ms']))
    require(v['created_at_ms']<=observed); hex64(v['txid']); integer(v['output_index'],0,255)
    integer(v['atoms'],MIN_MONEY,MAX_MONEY); require(v['recorded_state'] in ('prepared','broadcast','confirmed'))
    require(len(canon(v))<=256)
def history(v,gym):
    fields(v,HISTORY_FIELDS); integer(v['version'],1,1); script(v['recipient_script'],True)
    b=v['balance']; fields(b,BALANCE_FIELDS); hex64(b['gym_id']); require(b['gym_id']==gym)
    conf,available=rational(b['confirmed']),rational(b['available'])
    integer(b['paid_atoms'],0,MAX_MONEY); integer(b['reserved_atoms'],0,MAX_MONEY)
    require(b['paid_atoms']+b['reserved_atoms']<=MAX_MONEY)
    require(0<=available<=conf-b['reserved_atoms'] and conf<=MAX_MONEY and b['reason'] in REASONS)
    require(conf!=0 or b['reason']==''); require(len(canon(b))<=17408)
    observed=v['settlement_observed_at_ms']; integer(observed,0,MAX_TIME)
    rows=v['items']; require(type(rows) is list and len(rows)<=366)
    paid=reserved=0; txids=set(); prev=None
    for i,row in enumerate(rows):
        item(row,observed); require(row['txid'] not in txids); txids.add(row['txid'])
        if prev: require(row['day']<prev['day'] and row['created_at_ms']<prev['created_at_ms'])
        if row['recorded_state']=='confirmed': paid+=row['atoms']
        else: require(i==0); reserved+=row['atoms']
        prev=row
    require(paid==b['paid_atoms'] and reserved==b['reserved_atoms'] and paid+reserved<=MAX_MONEY)
    if not v['recipient_script']:
        require(not rows and paid==reserved==0)
        require(conf==0 or b['reason']=='destination-unavailable')
    if not observed: require(not rows and not conf and not available and paid==reserved==0 and b['reason']=='')
    require(len(canon(v))<=131072)
    return v
def response(raw,expected):
    r=parse(raw,131072,True); fields(r,['version','binding_id','history'],True)
    integer(r['version'],1,1); hex64(r['binding_id']); require(r['binding_id']==expected['binding_id'])
    h=history(r['history'],expected['gym_id'])
    # Core HTTP is canonical in nested order too; pure value checks are not.
    fields(h,HISTORY_FIELDS,True); fields(h['balance'],BALANCE_FIELDS,True)
    for f in [h['balance']['confirmed'],h['balance']['available']]:fields(f,['numerator','denominator'],True)
    for row in h['items']:fields(row,ITEM_FIELDS,True)
    return r

def accepts(fn,*args):
    try: fn(*args); return True
    except (ValueError,TypeError,KeyError,OverflowError,IndexError,UnicodeError): return False
