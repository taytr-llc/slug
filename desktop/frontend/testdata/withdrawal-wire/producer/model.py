"""Finite frozen-contract withdrawal wire model; no application imports."""
import copy,hashlib,json,re
J=lambda v:json.dumps(v,separators=(',',':'),ensure_ascii=False).encode()
H=lambda b:hashlib.sha256(b).hexdigest()
C=copy.deepcopy
MAX=2100000000000000
SAFE=9007199254740991
TIME=253402300799999
ERRORS=['invalid_request','unauthorized','stale_view','held','reauthentication_required','busy','capacity','withdrawal_unavailable','withdrawal_not_found','operation_conflict','draft_stale','insufficient_funds','fee_limit','outcome_unknown']
COMMANDS=['withdrawal_snapshot','withdrawal_operation','withdrawal_action','withdrawal_prepare','withdrawal_review','withdrawal_approve','withdrawal_stop','withdrawal_abandon','withdrawal_observe']
def need(ok):
    if not ok:raise ValueError('frozen wire rule')
def fields(v,names):need(type(v) is dict and list(v)==names or type(v) is dict and set(v)==set(names))
def integer(x,lo=0,hi=SAFE):need(type(x) is int and lo<=x<=hi)
def hexid(s,empty=False):need(type(s) is str and (empty and s=='' or re.fullmatch('[0-9a-f]{64}',s)))
def ident(s,empty=False):
    hexid(s,empty);need(s=='' and empty or s!='0'*64)
def digest(domain,v):
    raw=J(v);return H(domain+len(raw).to_bytes(4,'little')+raw)
def trust(origin):
    raw=origin.encode();return H(b'SLUG manifest transport trust\0v1\0'+len(raw).to_bytes(4,'little')+raw+b'\1'+bytes(4))
def binding(v,expected):
    fields(v,['view','source_binding_id','journal_id','withdrawal_binding_id']);need(v==expected)
    for k in ['source_binding_id','journal_id','withdrawal_binding_id']:ident(v[k])
    view=v['view'];fields(view,['version','view_id','account_authority_id','account_id','generation','authority_pin','authority_origin','transport_trust_id'])
    integer(view['version'],1,1);integer(view['generation'],1);need(view['transport_trust_id']==trust(view['authority_origin']))
    for k in ['view_id','account_authority_id','account_id']:hexid(view[k])
    need(len(J(v))<=4096)
def intent(v,addresses):
    fields(v,['destination','recipient_script','amount_atoms','fee_rate_atoms_per_kb','fee_limit_atoms'])
    need(addresses.get(v['destination'])==v['recipient_script']);integer(v['amount_atoms'],1,MAX);integer(v['fee_rate_atoms_per_kb'],1000,1000);integer(v['fee_limit_atoms'],100000,100000);need(len(J(v))<=512)
def draft(v,addresses):
    fields(v,['version','draft_digest','unsigned_txid','destination','recipient_script','amount_atoms','fee_atoms','total_debit_atoms','input_count','input_atoms','recipient_output_index','change_atoms','change_output_index','prepared_at_ms','observed_tip','observed_height'])
    integer(v['version'],1,1)
    for k in ['draft_digest','unsigned_txid','observed_tip']:hexid(v[k])
    intent(dict(destination=v['destination'],recipient_script=v['recipient_script'],amount_atoms=v['amount_atoms'],fee_rate_atoms_per_kb=1000,fee_limit_atoms=100000),addresses)
    integer(v['fee_atoms'],1,100000);integer(v['input_count'],1,16);integer(v['input_atoms'],1,MAX);integer(v['total_debit_atoms'],1,MAX);integer(v['change_atoms'],0,MAX);integer(v['recipient_output_index'],0,1);integer(v['prepared_at_ms'],1,TIME);integer(v['observed_height'],0,2147483647)
    need(v['total_debit_atoms']==v['amount_atoms']+v['fee_atoms'] and v['input_atoms']==v['total_debit_atoms']+v['change_atoms'])
    integer(v['change_output_index'],-1,1)
    need(v['change_atoms']==0 and v['change_output_index']==-1 and v['recipient_output_index']==0 or v['change_atoms']>0 and v['change_output_index']>=0 and v['change_output_index']!=v['recipient_output_index'])
    need(len(J(v))<=2048)
def dispatch(v,created,updated):
    fields(v,['attempt','authorization','intent_at_ms','result','result_at_ms','error']);integer(v['attempt'],1,4294967295)
    a=v['authorization'];fields(a,['action_id','review_id','view_id','generation','authorized_at_ms'])
    for k in ['action_id','review_id','view_id']:ident(a[k])
    integer(a['generation'],1);integer(a['authorized_at_ms'],created,updated);integer(v['intent_at_ms'],a['authorized_at_ms'],updated)
    need(v['result'] in ['pending','acknowledged','unknown','rejected'])
    if v['result']=='pending':need(v['result_at_ms']==0 and v['error']=='')
    else:
        integer(v['result_at_ms'],v['intent_at_ms'],updated);need(v['error']=={'acknowledged':'','unknown':'node_unavailable','rejected':'node_rejected'}[v['result']])
def observation(v,txid,created,updated):
    fields(v,['version','txid','observed_at_ms','state','node_tip','node_height','confirmations','block_hash','error']);integer(v['version'],1,1);need(v['txid']==txid);integer(v['observed_at_ms'],created,updated);integer(v['node_height'],0,2147483647);integer(v['confirmations'],0,2147483648)
    if v['state']=='unknown':need(v['node_tip']==v['block_hash']=='' and v['node_height']==v['confirmations']==0 and v['error']=='node_unavailable')
    else:
        need(v['state'] in ['not_observed','mempool','confirmed'] and v['error']=='');ident(v['node_tip'])
        if v['state']=='confirmed':ident(v['block_hash']);integer(v['confirmations'],1,v['node_height']+1)
        else:need(v['block_hash']=='' and v['confirmations']==0)
    need(len(J(v))<=2048)
def operation(v,b,addresses):
    fields(v,['version','operation_id','journal_id','withdrawal_binding_id','sequence','revision','created_at_ms','updated_at_ms','prepared_at_ms','stage','intent','intent_digest','draft','txid','signed_sha256','dispatch','observation']);integer(v['version'],1,1);ident(v['operation_id']);hexid(v['intent_digest']);need(v['journal_id']==b['journal_id'] and v['withdrawal_binding_id']==b['withdrawal_binding_id']);integer(v['sequence'],1,128);integer(v['revision'],1,4294967295);integer(v['created_at_ms'],1,TIME);integer(v['updated_at_ms'],v['created_at_ms'],TIME);integer(v['prepared_at_ms'],0,v['updated_at_ms']);intent(v['intent'],addresses)
    stage=v['stage'];need(stage in ['intent','prepared','signed','dispatching','submitted','abandoned'])
    if v['draft'] is None:need(stage in ['intent','abandoned'] and v['prepared_at_ms']==0)
    else:
        draft(v['draft'],addresses);need(stage!='intent' and v['draft']['prepared_at_ms']==v['prepared_at_ms'] and v['prepared_at_ms']>=v['created_at_ms'])
        for k in ['destination','recipient_script','amount_atoms']:need(v['draft'][k]==v['intent'][k])
    if stage in ['signed','dispatching','submitted']:
        need(v['draft'] is not None);hexid(v['txid']);hexid(v['signed_sha256']);need(v['txid']==v['draft']['unsigned_txid'])
    else:need(v['txid']==v['signed_sha256']=='' and v['dispatch'] is None and v['observation'] is None)
    if v['dispatch'] is not None:need(stage in ['dispatching','submitted']);dispatch(v['dispatch'],v['created_at_ms'],v['updated_at_ms'])
    if stage=='dispatching':need(v['dispatch'] is not None)
    if v['observation'] is not None:need(stage in ['signed','dispatching','submitted']);observation(v['observation'],v['txid'],v['created_at_ms'],v['updated_at_ms'])
    if stage=='submitted':need(v['observation'] is not None)
    need(len(J(v))<=8192)
def review(v,b,addresses):
    fields(v,['version','binding','operation_id','action_id','review_id','kind','draft','txid','reviewed_at_ms','expires_at_ms']);integer(v['version'],1,1);binding(v['binding'],b)
    for k in ['operation_id','action_id','review_id']:ident(v[k])
    draft(v['draft'],addresses);integer(v['reviewed_at_ms'],1,TIME);integer(v['expires_at_ms'],v['reviewed_at_ms']+1,min(TIME,v['reviewed_at_ms']+300000));need(v['kind']=='send' and v['txid']=='' or v['kind']=='retry' and v['txid']==v['draft']['unsigned_txid']);need(len(J(v))<=8192)
def action(v,b,addresses):
    fields(v,['version','binding','operation_id','action_id','kind','state','review_id','error','operation']);integer(v['version'],1,1);binding(v['binding'],b);ident(v['operation_id']);ident(v['action_id']);ident(v['review_id'],True);need(v['kind'] in ['prepare','send','retry','abandon','observe']);need(v['state'] in ['running','review','done','failed','stopped','outcome_unknown'])
    if v['kind'] in ['send','retry']:ident(v['review_id'])
    else:need(v['review_id']=='')
    if v['state']=='review':
        need(v['kind'] in ['send','retry'] and v['operation'] is not None)
        need(v['kind']=='send' and v['operation']['stage']=='prepared' and v['operation']['txid']=='' or v['kind']=='retry' and v['operation']['stage'] in ['signed','dispatching','submitted'] and v['operation']['txid']!='')
    if v['state']=='failed':need(v['error'] in ERRORS and v['error']!='outcome_unknown')
    elif v['state']=='outcome_unknown':need(v['error']=='outcome_unknown')
    else:need(v['error']=='')
    if v['operation'] is not None:operation(v['operation'],b,addresses);need(v['operation']['operation_id']==v['operation_id'])
    need(len(J(v))<=14336)
def snapshot(v,b):
    fields(v,['version','state','error','binding','active_action_id','last_action_id','unresolved_operation_id','latest_operation_id','can_prepare','can_review']);integer(v['version'],1,1)
    for k in ['active_action_id','last_action_id','unresolved_operation_id','latest_operation_id']:ident(v[k],True)
    need(type(v['can_prepare']) is bool and type(v['can_review']) is bool)
    if v['state']=='unavailable':need(v['binding'] is None and v['error']=='withdrawal_unavailable' and not any(v[k] for k in ['active_action_id','last_action_id','unresolved_operation_id','latest_operation_id','can_prepare','can_review']))
    else:
        binding(v['binding'],b);need(v['state'] in ['idle','busy'] and v['error']=='');need(bool(v['active_action_id'])==(v['state']=='busy'))
        if v['can_prepare']:need(v['state']=='idle' and v['unresolved_operation_id']=='')
    need(len(J(v))<=5120)
def request(v,addresses):
    fields(v,['id','version','command','view_id','operation_id','action_id','review_id','review_kind','draft_digest','txid','intent']);integer(v['id'],1);integer(v['version'],1,1);cmd=v['command'];need(cmd in COMMANDS)
    for k in ['view_id','draft_digest','txid']:hexid(v[k],True)
    for k in ['operation_id','action_id','review_id']:ident(v[k],True)
    selected={k for k in ['view_id','operation_id','action_id','review_id','review_kind','draft_digest','txid','intent'] if v[k] is not None and v[k]!=''}
    required=set() if cmd=='withdrawal_snapshot' else {'view_id','operation_id'}
    if cmd in ['withdrawal_action','withdrawal_prepare','withdrawal_review','withdrawal_approve','withdrawal_stop','withdrawal_abandon','withdrawal_observe']:required.add('action_id')
    if cmd=='withdrawal_prepare':required.add('intent');intent(v['intent'],addresses)
    if cmd=='withdrawal_review':
        required.update(['review_id','review_kind','draft_digest']);need(v['review_kind'] in ['send','retry'])
        if v['review_kind']=='retry':required.add('txid')
    if cmd=='withdrawal_approve':
        required.update(['review_id','draft_digest'])
        if v['txid']:required.add('txid')
    if cmd=='withdrawal_abandon' and v['draft_digest']:required.add('draft_digest')
    if cmd=='withdrawal_observe':required.add('txid')
    need(selected==required and len(J(v))+1<=2048)
def response(v,b,addresses):
    fields(v,['id','version','ok','snapshot','operation','review','action','error']);integer(v['id'],1);integer(v['version'],1,1);need(type(v['ok']) is bool);arms=[k for k in ['snapshot','operation','review','action'] if v[k] is not None]
    if not v['ok']:need(not arms and v['error'] in ERRORS)
    else:
        need(len(arms)==1 and v['error']=='');k=arms[0]
        if k=='snapshot':snapshot(v[k],b)
        else:globals()[k](v[k],b,addresses)
    need(len(J(v))+1<=16384)
def parse(raw):
    def pairs(items):
        d={}
        for k,v in items:need(k not in d);d[k]=v
        return d
    def number(s):need(s!='-0');return int(s)
    def bad(s):raise ValueError('not integer')
    return json.loads(raw.decode('utf-8'),object_pairs_hook=pairs,parse_int=number,parse_float=bad,parse_constant=bad)
def accepts(fn,*args):
    try:fn(*args);return True
    except (ValueError,TypeError,KeyError,OverflowError):return False
