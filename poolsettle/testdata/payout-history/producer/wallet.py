"""Independent narrow wallet-result model; all RPC cases are modeled inputs."""
import math
from common import fields,require,integer,hex64,parse,MAX_TIME,MAX_MONEY
from transaction import check_selection

RESULT_REQUIRED={'amount','confirmations','blockhash','blockindex','blocktime','txid','walletconflicts','time','timereceived','details','hex'}
DETAIL_REQUIRED={'account','amount','category','vout'}
def text(v,cap=256):require(type(v) is str and len(v.encode('utf-8'))<=cap)
def finite(v):require(type(v) in (int,float) and math.isfinite(v))
def i64(v):integer(v,-9223372036854775808,9223372036854775807)
def sample(v):
    fields(v,['synced','height','hash']);require(v['synced'] is True)
    integer(v['height'],0,2147483647);hex64(v['hash'],True)
def wallet_read(raw,selected,before,after,observed,request_id=1):
    sample(before);sample(after);require(before==after);integer(observed,1,MAX_TIME)
    env=parse(raw,524288,floats=True);fields(env,['jsonrpc','result','error','id'])
    require(env['jsonrpc']=='1.0' and type(env['id']) is int and env['id']==request_id)
    base={'observed_at_ms':observed,'wallet_height':after['height'],'wallet_tip':after['hash'],'confirmations':0,'block_hash':''}
    if env['error'] is not None:
        fields(env['error'],['code','message'])
        require(env['result'] is None and type(env['error']['code']) is int and env['error']['code']==-5 and env['error']['message']=='No information for transaction')
        return {'state':'not_observed',**base}
    result=env['result'];require(type(result) is dict and RESULT_REQUIRED<=set(result)<=RESULT_REQUIRED|{'fee'})
    finite(result['amount'])
    if 'fee' in result:finite(result['fee'])
    for key in ['confirmations','blockindex','blocktime','time','timereceived']:i64(result[key])
    hex64(result['txid']);require(type(result['blockhash']) is str)
    conflicts=result['walletconflicts'];require(type(conflicts) is list and len(conflicts)<=256)
    for conflict in conflicts:hex64(conflict)
    details=result['details'];require(type(details) is list and len(details)<=258)
    for detail in details:
        require(type(detail) is dict and DETAIL_REQUIRED<=set(detail)<=DETAIL_REQUIRED|{'address','involveswatchonly','fee'})
        for key in ['account','category']:text(detail[key])
        if 'address' in detail:text(detail['address'])
        if 'involveswatchonly' in detail:require(type(detail['involveswatchonly']) is bool)
        finite(detail['amount']);integer(detail['vout'],0,4294967295)
        if 'fee' in detail:finite(detail['fee'])
    check_selection(result['hex'],selected,result['txid'])
    count=result['confirmations'];integer(count,0,after['height']+1)
    if count:
        hex64(result['blockhash'],True);base.update(confirmations=count,block_hash=result['blockhash'])
        return {'state':'confirmed',**base}
    require(result['blockhash']=='')
    return {'state':'unconfirmed',**base}
