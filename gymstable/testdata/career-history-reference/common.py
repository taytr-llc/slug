"""Small independent canonical/scalar/closed-shape primitives; no runtime imports."""
from pathlib import Path
import copy, hashlib, json, re, struct
ROOT=Path(__file__).resolve().parent
MAX_SAFE=9007199254740991
MAX_TIME=8640000000000000
class Refusal(ValueError):
    def __init__(self,code='invalid'):
        self.code=code;super().__init__(code)
def require(ok,code='invalid'):
    if not ok:raise Refusal(code)
def clone(v):return copy.deepcopy(v)
def sha(b):return hashlib.sha256(b).hexdigest()
def ident(n):return '%064x'%n
def ID(v):return type(v) is str and re.fullmatch('[0-9a-f]{64}',v) is not None
def integer(v,lo=0,hi=MAX_SAFE):return type(v) is int and lo<=v<=hi
def stamp(v):return integer(v,1,MAX_TIME)
def u32(n):return struct.pack('<I',n)
def u64(n):return struct.pack('<Q',n)
def raw(s):require(ID(s));return bytes.fromhex(s)
def lp(b):return u32(len(b))+b
def J(v):
    s=json.dumps(v,ensure_ascii=False,separators=(',',':'),allow_nan=False)
    for a,b in [('<','\\u003c'),('>','\\u003e'),('&','\\u0026'),('\u2028','\\u2028'),('\u2029','\\u2029')]:s=s.replace(a,b)
    return s.encode('utf-8',errors='strict')
def fields(v,names):
    if type(names) is str:names=names.split()
    require(type(v) is dict and set(v)==set(names));return {k:v[k] for k in names}
def bounded(v,n):require(len(J(v))<=n);return v
def _pairs(pairs):
    d={}
    for k,v in pairs:require(k not in d);d[k]=v
    return d
def _no_float(_):raise Refusal()
def _int(s):
    require(re.fullmatch(r'0|[1-9][0-9]*',s) is not None);v=int(s);require(v<=MAX_SAFE);return v
def strict_parse(data,max_bytes,max_depth):
    try:
        if type(data) is str:data=data.encode('utf-8',errors='strict')
        require(type(data) is bytes and 0<len(data)<=max_bytes)
        value=json.loads(data.decode('utf-8',errors='strict'),object_pairs_hook=_pairs,parse_int=_int,parse_float=_no_float,parse_constant=_no_float)
        def walk(v,depth):
            if isinstance(v,(dict,list)):
                require(depth<=max_depth)
                if type(v) is dict:
                    require(len(v)<=64)
                    for k,x in v.items():k.encode('utf-8',errors='strict');walk(x,depth+1)
                else:
                    require(len(v)<=256)
                    for x in v:walk(x,depth+1)
            elif type(v) is str:v.encode('utf-8',errors='strict')
            else:require(v is None or type(v) is bool or integer(v))
        walk(value,1);return value
    except (UnicodeError,ValueError,TypeError,RecursionError) as e:
        if isinstance(e,Refusal):raise
        raise Refusal() from e
def load(name):return json.loads((ROOT/'inputs/extracted'/name).read_bytes())
def validate(kind,value,**context):
    from history_math import validate_core
    return validate_core(kind,value,**context)
