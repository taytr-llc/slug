"""Independent conservative JSON sizing, definitions only until approved run.

Envelope bounds are deliberately not valid transactions/evidence. Every
integer is widened to20 decimal digits (larger than the frozen scalar bounds),
and closed enum strings to32ASCII bytes. These safe overestimates prove caps
without treating impossible simultaneous monetary maxima as valid evidence.
"""
import copy

def measure(M,positives,recipe):
    def widen(v,key='',leg=''):
        if v is None:return None
        if type(v) is bool:return False
        if type(v) is int:return 99999999999999999999
        if type(v) is list:
            if key=='invalid_indices':return [4095]*127
            if key=='inputs':
                seed=max(v,key=lambda x:len(M.canonical(x)))
                return [widen(seed) for _ in range(16)]
            return [widen(x) for x in v]
        if type(v) is dict:return {k:widen(x,k,k if k in ('commit','spend') else leg) for k,x in v.items()}
        if key=='raw':return 'f'*(8192 if leg=='commit' else 1024)
        if key=='commit_raw':return 'f'*8192
        if key=='gym_authorization':return 'f'*128
        if key=='root_public_key':return 'x'*128
        if key in ('address','return_address','recipient'):return 'x'*90
        if key in ('script','return_script','recipient_script'):return 'f'*44
        if key in ('gym_public_key','sender_gym','recipient_gym','funding_public_key'):return 'f'*66
        if key.endswith('_id') or key.endswith('_digest') or key.endswith('_sha256') or key.endswith('_hash') or key in ('binding_id','txid','commit_txid','spend_txid','node_tip','genesis','guard_sha256'):return 'f'*64
        return 'x'*max(len(v),32)

    # Canonical overhead is measured with the independent serializer. Objects
    # come from each disjoint union; null arms never get combined unlawfully.
    caps={'binding2':8192,'guard14':2048,'guard15':8192,'request2':4096,
          'reservation2':8192,'link2':8192,'allocation_row':8192,
          'carrier_request':1024,'review':4096,'preparation':16384,
          'wallet_record':24576,'signed_package':12288,'operation':49152}
    result=[]
    for kind,cap in caps.items():
        values=[p for p in positives if p['kind']==kind]
        M.need(bool(values),'sizing coverage')
        bound=max(len(M.canonical(widen(p['value']))) for p in values)
        M.need(bound<=cap,'conservative envelope exceeds '+kind)
        result.append(dict(kind=kind,conservative_bytes=bound,cap=cap,
                           fits=True,scope='fieldwise conservative grammar bound, not semantically valid evidence'))
    by={x['kind']:x['conservative_bytes'] for x in result}
    composed=[
        dict(name='journal128',bytes=128*49152+127+4096,cap=8388608,fits=True),
        dict(name='wallet128',bytes=128*(32+24576)+4096,cap=4194304,fits=True),
        dict(name='exclusions4224',bytes=4224*(36+512)+4096,cap=4194304,fits=True),
        dict(name='allocation-sync-terminal',bytes=8192+75+1024,cap=24576,fits=True),
        dict(name='allocation1024-rowmax-independent-byte-refusal',bytes=1024*(75+8192)+1024,cap=4194304,fits=False),
        dict(name='action-scratch-accounting',bytes=16777216+8388608+49152+24576+16384+12288+4096+4096,cap=33554432,fits=True),
    ]
    for row in composed:M.need((row['bytes']<=row['cap'])==row['fits'],'composition')
    stripped=4+1+16*(32+4+1+4)+1+(8+1+23)+(8+1+76)+(8+1+22)+4
    witness=2+16*(1+1+73+1+33)
    reveal=4+1+36+3+(1+73+2+240)+4+1+(8+1+22)+(8+1+43)+4
    refund=reveal-(8+1+43)
    M.need((stripped,witness,reveal,refund)==(814,1746,452,400),'transaction layout arithmetic')
    return dict(scope='No allocations, filesystem or memory profiler claims. Working-budget row is an accounting recipe, requiring concrete ownership enforcement in Go.',
                field_bounds=result,compositions=composed,
                transaction_maxima=dict(commit_stripped=stripped,commit_witness=witness,commit_total=stripped+witness,reveal=reveal,refund=refund),
                scalar_and_capacity_controls=[dict(count=128,limit=128,shape_allowed=True),dict(count=129,limit=128,shape_allowed=False),dict(count=4224,limit=4224,shape_allowed=True),dict(count=4225,limit=4224,shape_allowed=False),dict(rows=1024,byte_limit_independent=True),dict(revision=4294967295,next_mutation_allowed=False)])
