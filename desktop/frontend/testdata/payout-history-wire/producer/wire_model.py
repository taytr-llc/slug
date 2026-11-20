"""Independent private Set67 wire shape/correlation model; no Go or frontend."""
from common import *

COMMANDS=['pool_payout_snapshot','pool_payout_begin','pool_payout_job','pool_payout_cancel','pool_payout_page','pool_payout_wallet']
ERRORS=['invalid_request','unauthorized','stale_view','busy','capacity','payout_unavailable','payout_not_found','operation_conflict','outcome_unknown']
REQUEST_FIELDS=['id','version','command','view_id','job_id','after','limit','txid','output_index']
RESPONSE_FIELDS=['id','version','ok','snapshot','job','page','observation','error']
def maybeid(v,nonzero=True):
    require(type(v) is str)
    if v:hex64(v,nonzero)
def request(v):
    fields(v,REQUEST_FIELDS);integer(v['id'],1,MAX_SAFE);integer(v['version'],1,1);require(v['command'] in COMMANDS)
    for f in ['view_id','job_id','txid']:maybeid(v[f],f=='job_id')
    integer(v['after'],0,366);integer(v['limit'],0,8);integer(v['output_index'],0,255)
    cmd=v['command'];require(cmd=='pool_payout_snapshot' or v['view_id'] and v['job_id'])
    if cmd=='pool_payout_snapshot':require(v['view_id']==v['job_id']==v['txid']=='' and v['after']==v['limit']==v['output_index']==0)
    elif cmd=='pool_payout_page':require(1<=v['limit']<=8 and v['txid']=='' and v['output_index']==0)
    elif cmd=='pool_payout_wallet':require(v['txid']!='' and v['after']==v['limit']==0)
    else:require(v['after']==v['limit']==v['output_index']==0 and v['txid']=='')
    require(len(canon(v))+1<=1024)
def loopback_trust(origin):
    require(type(origin) is str and re.fullmatch(r'http://127\.0\.0\.1:[1-9][0-9]{0,4}',origin))
    integer(int(origin.rsplit(':',1)[1]),1,65535);raw=origin.encode('ascii')
    return sha(b'SLUG manifest transport trust\0v1\0'+len(raw).to_bytes(4,'little')+raw+b'\1'+(0).to_bytes(4,'little'))
def binding(v,expected):
    fields(v,['view','source_binding_id','recipient_script']);hex64(v['source_binding_id'],True);script(v['recipient_script'])
    fields(v['view'],['version','view_id','account_authority_id','account_id','generation','authority_pin','authority_origin','transport_trust_id'])
    require(v['view']['transport_trust_id']==loopback_trust(v['view']['authority_origin']))
    require(v==expected and len(canon(v))<=4096)
def capture(v):
    fields(v,['version','count','paid_atoms','reserved_atoms','settlement_observed_at_ms','captured_at_ms'])
    integer(v['version'],1,1);integer(v['count'],0,366);integer(v['paid_atoms'],0,MAX_MONEY);integer(v['reserved_atoms'],0,MAX_MONEY)
    require(v['paid_atoms']+v['reserved_atoms']<=MAX_MONEY)
    integer(v['settlement_observed_at_ms'],0,MAX_TIME);integer(v['captured_at_ms'],1,MAX_TIME)
    if not v['settlement_observed_at_ms']:require(v['count']==v['paid_atoms']==v['reserved_atoms']==0)
    require(len(canon(v))<=512)
def snapshot(v,expected):
    fields(v,['version','state','error','binding','activity','active_job_id','last_job_id','capture_job_id'])
    integer(v['version'],1,1);require(v['state'] in ['unavailable','idle','busy'])
    for f in ['active_job_id','last_job_id','capture_job_id']:maybeid(v[f])
    if v['state']=='unavailable':
        require(v['error']=='payout_unavailable' and v['binding'] is None and v['activity']=='none' and v['active_job_id']==v['last_job_id']==v['capture_job_id']=='')
    else:
        binding(v['binding'],expected);require(v['error']=='')
        if v['state']=='idle':require(v['activity']=='none' and v['active_job_id']=='')
        else:require(v['activity'] in ['capture','wallet'] and v['active_job_id']!='')
        if v['activity']=='wallet':require(v['active_job_id']==v['capture_job_id'])
    require(len(canon(v))<=5120)
def job(v,expected):
    fields(v,['version','job_id','state','binding','error','capture'])
    integer(v['version'],1,1);hex64(v['job_id'],True);binding(v['binding'],expected)
    require(v['state'] in ['running','done','failed','cancelled'])
    if v['state']=='done':require(v['error']=='');capture(v['capture'])
    elif v['state']=='failed':require(v['capture'] is None and v['error'] in ['busy','capacity','payout_unavailable'])
    else:require(v['capture'] is None and v['error']=='')
    require(len(canon(v))<=6144)
def page(v,expected):
    fields(v,['version','binding','job_id','capture','after','limit','next','has_more','items'])
    integer(v['version'],1,1);binding(v['binding'],expected);hex64(v['job_id'],True);capture(v['capture'])
    integer(v['after'],0,v['capture']['count']);integer(v['limit'],1,8)
    rows=v['items'];require(type(rows) is list and len(rows)==min(v['limit'],v['capture']['count']-v['after']))
    integer(v['next'],0,366);require(type(v['has_more']) is bool and v['next']==v['after']+len(rows) and v['has_more']==(v['next']<v['capture']['count']))
    prev=None
    for row in rows:
        item(row,v['capture']['settlement_observed_at_ms'])
        if prev:require(row['day']<prev['day'] and row['created_at_ms']<prev['created_at_ms'])
        prev=row
    require(len({r['txid'] for r in rows})==len(rows));require(len(canon(v))<=10240)
def observation(v,expected):
    fields(v,['version','binding','job_id','capture','payment','observed_at_ms','state','error','wallet_synced','wallet_height','wallet_tip','confirmations','block_hash'])
    integer(v['version'],1,1);binding(v['binding'],expected);hex64(v['job_id'],True);capture(v['capture']);item(v['payment'],v['capture']['settlement_observed_at_ms'])
    integer(v['observed_at_ms'],1,MAX_TIME);require(v['state'] in ['not_observed','unconfirmed','confirmed','unavailable'])
    require(type(v['wallet_synced']) is bool);integer(v['wallet_height'],0,2147483647);integer(v['confirmations'],0,2147483648)
    if v['state']=='unavailable':require(v['error']=='wallet_unavailable' and not v['wallet_synced'] and v['wallet_height']==v['confirmations']==0 and v['wallet_tip']==v['block_hash']=='')
    else:
        require(v['error']=='' and v['wallet_synced']);hex64(v['wallet_tip'],True)
        if v['state']=='confirmed':integer(v['confirmations'],1,v['wallet_height']+1);hex64(v['block_hash'],True)
        else:require(v['confirmations']==0 and v['block_hash']=='')
    require(len(canon(v))<=7168)
def response(v,expected):
    fields(v,RESPONSE_FIELDS);integer(v['id'],1,MAX_SAFE);integer(v['version'],1,1);require(type(v['ok']) is bool)
    arms={k:v[k] for k in ['snapshot','job','page','observation'] if v[k] is not None}
    if not v['ok']:require(not arms and v['error'] in ERRORS)
    else:
        require(len(arms)==1 and v['error']=='');key=next(iter(arms));globals()[key](arms[key],expected)
    require(len(canon(v))+1<=12288)
def decode(raw,kind,expected):
    require(len(raw)<=(1024 if kind=='request' else 12288));v=parse(raw,1024 if kind=='request' else 12288)
    (request(v) if kind=='request' else response(v,expected));return v
