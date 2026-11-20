"""Independent ordered wallet codec model. Definitions only on import.

Uses reviewed public reference arithmetic, never a Go encoder or live wallet.
Capabilities, storage atomicity and lifecycle behavior cannot be minted here.
"""
import copy
import hashlib
import json
import re
import struct

MONEY=2100000000000000
TIME=253402300799999
U32=4294967295
GEN=9007199254740991
N=0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141

class Refused(ValueError):
    pass

def need(v,why='invalid'):
    if not v:raise Refused(why)
def own(v):return copy.deepcopy(v)
def integer(v,lo=0,hi=U32):
    need(type(v) is int and lo<=v<=hi,'integer')
    return v
def text(v,cap):
    need(type(v) is str and v.isascii() and len(v)<=cap,'text')
    return v
def hexv(v,size=32,nonzero=False):
    text(v,size*2);need(len(v)==size*2 and re.fullmatch('[0-9a-f]+',v) is not None,'hex')
    need(not nonzero or any(bytes.fromhex(v)),'zero id')
    return v
def empty(v):need(v=='','sentinel')
def rawhex(v,cap):
    text(v,2*cap);need(len(v)%2==0 and bool(v) and re.fullmatch('[0-9a-f]+',v) is not None,'raw hex')
    return bytes.fromhex(v)
def canonical(v):
    raw=json.dumps(v,ensure_ascii=False,allow_nan=False,separators=(',',':'))
    return raw.replace('<','\\u003c').replace('>','\\u003e').replace('&','\\u0026').replace('\u2028','\\u2028').replace('\u2029','\\u2029').encode('utf8')
def sha(raw):return hashlib.sha256(raw).hexdigest()
def digest(kind,v,version=1):
    return sha(('SLUG '+kind+'\0v'+str(version)+'\0').encode('ascii')+canonical(v))
def allocation(kind,v):return digest('hosted allocation '+kind,v,2)
def limited(v,cap):need(len(canonical(v))<=cap,'capacity');return v
def fields(v,names):
    names=names.split() if isinstance(names,str) else names
    need(type(v) is dict and set(v)==set(names),'fields')
    return {k:v[k] for k in names}
def cur(v,zero_other=False):
    need(type(v) is list and len(v)==8,'cursors')
    for x in v:integer(x,0,4096)
    if zero_other:need(all(v[i]==0 for i in (0,1,2,3,6,7)),'unsupported cursors')
    return list(v)
def path(v,address):
    o=fields(v,'address script branch index');integer(o['branch'],0,1);integer(o['index'],0,4095)
    script=rawhex(o['script'],22);need(len(script)==22 and script[:2]==b'\x00\x14','native84')
    text(o['address'],90);need(o['address']==address(script),'address')
    return o
def point(v):
    hexv(v,33);b=bytes.fromhex(v);need(b[0] in (2,3),'point')
    p=2**256-2**32-977;x=int.from_bytes(b[1:],'big');need(x<p,'point')
    y2=(x*x*x+7)%p;need(pow(pow(y2,(p+1)//4,p),2,p)==y2,'point')
    return v

BINDING='version inventory_id authority_id account_id principal_id provision_id hosted_vault_id retention_id wallet_owner_id source_binding_id ownership_pin_id carrier_journal_id withdrawal_journal_id manager_schema max_allocations max_bytes max_plan_attempts max_branch_index initial_manifest'
GUARD='version state inventory_id authority_id account_id wallet_owner_id source_binding_id ownership_pin_id carrier_journal_id withdrawal_journal_id initial_manifest_sha256 binding_id'
RESTORED='version state manager_schema wallet_owner_id carrier_journal_id ownership_pin_id original_manifest_sha256 original_manifest activation_tip_hash activation_tip_height activation_cursors'
REQUEST='version kind consumer_id purpose branch withdrawal_binding_id recipient recipient_script amount_atoms fee_rate_atoms_per_kb fee_ceiling_atoms min_confirmations input_purpose change_purpose account tx_version input_sequence lock_time sighash_type carrier_binding_id ownership_pin_id prior_state_id recipient_gym funding_digest escrow_atoms commit_fee_atoms spend_fee_atoms min_change_atoms'
RESERVATION='version sequence allocation_id binding_id request purpose branch start_index end_index selected_index invalid_indices address script reserved_at_ms'
LINK='version inventory_id binding_id sequence allocation_id reservation_digest request_digest outcome purpose branch start_index end_index selected_index invalid_indices address script cursors_before cursors_after consumer_kind consumer_binding_id consumer_id preparation_digest'
PIN='version owner_mode manager_schema network genesis wallet_profile root_public_key gym_public_key gym_id wallet_owner_id ownership_pin_id account_authority_id account_id allocation_binding_id original_manifest_sha256'
CREQUEST='version request_id kind ownership_pin_id prior_state_id recipient_gym commit_txid'
INPUT='txid output_index atoms script branch index block_hash block_height coinbase'
REVIEW='version request_id kind binding_id ownership_pin_id prior_state_id sender_gym recipient_gym transfer_id funding_digest input_details_digest input_count total_input_atoms return_address return_script return_branch return_index commit_change_atoms escrow_atoms commit_fee_atoms spend_fee_atoms total_fee_atoms returned_atoms commit_txid'
PREPARATION='version binding_id request prepared_at_ms tip_hash tip_height ownership_head_digest new_funding restored_allocation recovery_funding review review_digest'
PACKAGE='version review_digest transfer_id gym_authorization commit spend spend_kind'
OPERATION='version sequence request_id revision created_at_ms updated_at_ms stage request request_digest preparation preparation_digest signed_package signed_package_digest sign_authorization commit_dispatch spend_dispatch commit_observation spend_observation'

def manifest(v,expected=None,initial=False):
    o=fields(v,'version profile network genesis derivation scan_from_height root_public_key gym_path gym_public_key gym_id accounts')
    integer(o['version'],1,1);integer(o['network'],0x534c5547,0x534c5547);integer(o['scan_from_height'],0,0)
    for k in ('profile','derivation','root_public_key','gym_path'):text(o[k],128)
    hexv(o['genesis']);point(o['gym_public_key']);hexv(o['gym_id'])
    need(type(o['accounts']) is list and len(o['accounts'])==4,'accounts')
    rows=[]
    for purpose,row in zip((44,49,84,86),o['accounts']):
        r=fields(row,'purpose coin account external_type internal_type next_external next_internal')
        integer(r['purpose'],purpose,purpose);integer(r['coin'],0,0);integer(r['account'],0,0)
        for k in ('next_external','next_internal'):integer(r[k],0,0 if initial else 4096)
        text(r['external_type'],32);text(r['internal_type'],32);rows.append(r)
    o['accounts']=rows
    if expected is not None:need(canonical(o)==canonical(expected),'manifest identity')
    return limited(o,4096)

class Model:
    def __init__(self,initial,address,derive,old_request,crypto_validate,parse_tx):
        self.initial=manifest(initial,initial,True)
        self.address=address;self.derive=derive;self.old_request=old_request;self.crypto_validate=crypto_validate;self.parse_tx=parse_tx
        self.original=None

    def binding(self,v):
        o=fields(v,BINDING);integer(o['version'],2,2)
        for k in BINDING.split()[1:13]:hexv(o[k],nonzero=True)
        for k,x in zip(BINDING.split()[13:18],(14,1024,4194304,128,4096)):integer(o[k],x,x)
        o['initial_manifest']=manifest(o['initial_manifest'],self.initial,True)
        return limited(o,8192)

    def guard(self,v,b):
        o=fields(v,GUARD);integer(o['version'],2,2);need(o['state'] in ('locked','bound'))
        for k in GUARD.split()[2:10]:hexv(o[k],nonzero=True);need(o[k]==b[k],'guard identity')
        need(o['initial_manifest_sha256']==sha(canonical(b['initial_manifest'])),'manifest digest')
        need(o['binding_id']==(allocation('binding',b) if o['state']=='bound' else ''),'guard binding')
        return limited(o,2048)

    def restored(self,v,original):
        o=fields(v,RESTORED);integer(o['version'],1,1);need(o['state']=='ready');integer(o['manager_schema'],15,15)
        for k in ('wallet_owner_id','carrier_journal_id','ownership_pin_id'):hexv(o[k],nonzero=True)
        o['original_manifest']=manifest(o['original_manifest'],original)
        need(o['original_manifest_sha256']==sha(canonical(original)))
        hexv(o['activation_tip_hash'],nonzero=True);integer(o['activation_tip_height'],0,2147483647)
        o['activation_cursors']=cur(o['activation_cursors'])
        floor=[x for a in original['accounts'] for x in (a['next_external'],a['next_internal'])]
        need(all(x<=y for x,y in zip(floor,o['activation_cursors'])),'activation floor')
        return limited(o,8192)

    def request(self,v):
        o=fields(v,REQUEST);integer(o['version'],2,2);hexv(o['consumer_id'],nonzero=True);integer(o['purpose'],84,84)
        extra=REQUEST.split()[19:]
        if o['kind']!='carrier_return':
            old={k:o[k] for k in REQUEST.split()[:19]};old['version']=1
            self.old_request(old,o['consumer_id'])
            for k in extra[:5]:empty(o[k])
            for k in extra[5:]:integer(o[k],0,0)
        else:
            integer(o['branch'],1,1)
            for k in ('withdrawal_binding_id','recipient','recipient_script'):empty(o[k])
            for k in ('amount_atoms','fee_rate_atoms_per_kb','fee_ceiling_atoms'):integer(o[k],0,0)
            for k,x in {'min_confirmations':1,'input_purpose':84,'change_purpose':84,'account':0,'tx_version':1,'input_sequence':U32,'lock_time':0,'sighash_type':1,'escrow_atoms':500000,'commit_fee_atoms':10000,'spend_fee_atoms':10000,'min_change_atoms':1000}.items():integer(o[k],x,x)
            for k in ('carrier_binding_id','ownership_pin_id','prior_state_id','funding_digest'):hexv(o[k],nonzero=True)
            point(o['recipient_gym']);need(o['recipient_gym']!=self.initial['gym_public_key'],'self')
        return limited(o,4096)

    def reservation(self,v,b,injected=()):
        o=fields(v,RESERVATION);integer(o['version'],2,2);integer(o['sequence'],1,1024)
        hexv(o['allocation_id'],nonzero=True);need(o['binding_id']==allocation('binding',b))
        o['request']=self.request(o['request']);need(o['allocation_id']==o['request']['consumer_id'])
        for k in ('purpose','branch'):need(o[k]==o['request'][k])
        self.range(o,injected);integer(o['reserved_at_ms'],1,TIME)
        if o['request']['kind']=='pool_recipient':need(o['sequence']==1 and o['start_index']==o['selected_index']==0)
        else:need(o['sequence']>1)
        if o['request']['kind']=='receive':need(o['start_index']>=1)
        return o

    def range(self,o,injected=()):
        integer(o['purpose'],84,84);integer(o['branch'],0,1)
        start=integer(o['start_index'],0,4095);end=integer(o['end_index'],1,4096)
        need(start<end<=start+128);integer(o['selected_index'],end-1,end-1)
        need(type(o['invalid_indices']) is list and len(o['invalid_indices'])<=127)
        for x in o['invalid_indices']:integer(x,0,4095)
        need(o['invalid_indices']==list(range(start,end-1)))
        faults=set(tuple(x) for x in injected)
        for x in o['invalid_indices']:need((o['branch'],x) in faults or self.derive(o['branch'],x) is None,'forged invalid child')
        need((o['branch'],end-1) not in faults)
        need(self.derive(o['branch'],end-1)==(o['address'],o['script']),'selected path')
        path(dict(address=o['address'],script=o['script'],branch=o['branch'],index=end-1),self.address)

    def link(self,v,res,b,injected=()):
        o=fields(v,LINK);integer(o['version'],2,2);need(o['inventory_id']==b['inventory_id'])
        for k in ('binding_id','sequence','allocation_id','purpose','branch','start_index','end_index','selected_index','invalid_indices','address','script'):need(type(o[k]) is type(res[k]) and o[k]==res[k],'link')
        need(o['reservation_digest']==allocation('reservation',res));need(o['request_digest']==allocation('request',res['request']))
        need(o['outcome'] in ('consumed','burned'));before=cur(o['cursors_before'],True);after=cur(o['cursors_after'],True)
        slot=4+o['branch'];want=list(before);need(want[slot]==o['start_index']);want[slot]=o['end_index'];need(after==want)
        req=res['request'];need(o['consumer_kind']==req['kind'] and o['consumer_id']==req['consumer_id'])
        binding=req['withdrawal_binding_id'] if req['kind']=='withdrawal_change' else req['carrier_binding_id'] if req['kind']=='carrier_return' else ''
        need(o['consumer_binding_id']==binding)
        if o['outcome']=='burned' or req['kind'] in ('receive','pool_recipient'):empty(o['preparation_digest'])
        else:hexv(o['preparation_digest'])
        self.range(o,injected)
        return limited(o,8192)

    def allocation_row(self,v,res,link=None):
        o=fields(v,'reservation reservation_digest phase completed_at_ms wallet_record_digest preparation_digest')
        need(canonical(o['reservation'])==canonical(res));need(o['reservation_digest']==allocation('reservation',res))
        need(o['phase'] in ('reserved','consumed','burned'))
        if o['phase']=='reserved':
            integer(o['completed_at_ms'],0,0);empty(o['wallet_record_digest']);empty(o['preparation_digest'])
        else:
            integer(o['completed_at_ms'],res['reserved_at_ms'],TIME);need(link is not None and o['phase']==link['outcome'])
            need(o['wallet_record_digest']==allocation('wallet record',link) and o['preparation_digest']==link['preparation_digest'])
        return limited(o,8192)

    def allocation_marker(self,v,b):
        o=fields(v,'application version inventory_id binding');need(o['application']=='slug-hosted-transfer-allocation-inventory');integer(o['version'],2,2)
        need(o['inventory_id']==b['inventory_id'] and canonical(o['binding'])==canonical(b));need(len(canonical(o))+1<=8192)
        return o

    def authorization_request(self,v,prep):
        o=fields(v,'version binding_id request_id review_digest ownership_pin_id prior_state_id sender_gym recipient_gym funding_digest');integer(o['version'],1,1)
        need(prep['request']['kind']=='transfer')
        for k in ('binding_id','request_id','ownership_pin_id','prior_state_id','sender_gym','recipient_gym','funding_digest'):need(o[k]==prep['review'][k])
        need(o['review_digest']==prep['review_digest']);return o

    def journal_marker(self,v,pin,journal_id):
        o=fields(v,'application version journal_id pin binding_id');need(o['application']=='slug-fighter-carrier-journal');integer(o['version'],1,1)
        need(o['journal_id']==journal_id and canonical(o['pin'])==canonical(pin));need(o['binding_id']==digest('fighter carrier binding',dict(journal_id=journal_id,pin=pin)))
        need(len(canonical(o))+1<=4096);return o

    def pin(self,v,mode,original=None):
        o=fields(v,PIN);integer(o['version'],1,1);need(o['owner_mode']==mode and mode in ('hosted','restored'))
        integer(o['manager_schema'],14 if mode=='hosted' else 15,14 if mode=='hosted' else 15)
        mapping={'network':'network','genesis':'genesis','wallet_profile':'profile','root_public_key':'root_public_key','gym_public_key':'gym_public_key','gym_id':'gym_id'}
        for k,m in mapping.items():need(type(o[k]) is type(self.initial[m]) and o[k]==self.initial[m])
        for k in ('wallet_owner_id','ownership_pin_id'):hexv(o[k],nonzero=True)
        if mode=='hosted':
            for k in ('account_authority_id','account_id','allocation_binding_id'):hexv(o[k],nonzero=True)
            empty(o['original_manifest_sha256'])
        else:
            for k in ('account_authority_id','account_id','allocation_binding_id'):empty(o[k])
            need(o['original_manifest_sha256']==sha(canonical(original)))
        return o

    def crequest(self,v):
        o=fields(v,CREQUEST);integer(o['version'],1,1)
        for k in ('request_id','ownership_pin_id','prior_state_id'):hexv(o[k],nonzero=True)
        point(o['recipient_gym']);need(o['recipient_gym']!=self.initial['gym_public_key'])
        need(o['kind'] in ('transfer','recover_reveal','recover_refund'))
        if o['kind']=='transfer':empty(o['commit_txid'])
        else:hexv(o['commit_txid'])
        return limited(o,1024)

    def inputs(self,v):
        need(type(v) is list and 1<=len(v)<=16);out=[];last=None;total=0
        for value in v:
            o=fields(value,INPUT);hexv(o['txid']);integer(o['output_index']);integer(o['atoms'],1,MONEY)
            key=bytes.fromhex(o['txid'])[::-1]+struct.pack('<I',o['output_index']);need(last is None or last<key,'tuple order');last=key
            integer(o['branch'],0,1);integer(o['index'],0,4095);hexv(o['block_hash']);integer(o['block_height'],0,2147483647);need(type(o['coinbase']) is bool)
            need(self.derive(o['branch'],o['index'])[1]==o['script'],'input path');total+=o['atoms'];need(total<=MONEY)
            out.append(o)
        return out

    def review(self,v,request,binding):
        o=fields(v,REVIEW);integer(o['version'],1,1)
        for k in ('request_id','kind','ownership_pin_id','prior_state_id','recipient_gym'):need(o[k]==request[k])
        need(o['binding_id']==binding and o['sender_gym']==self.initial['gym_public_key'])
        for k in ('transfer_id','funding_digest'):hexv(o[k],nonzero=True)
        p=path(dict(address=o['return_address'],script=o['return_script'],branch=o['return_branch'],index=o['return_index']),self.address)
        need(self.derive(p['branch'],p['index'])==(p['address'],p['script']))
        for k,x in {'escrow_atoms':500000,'spend_fee_atoms':10000,'returned_atoms':490000}.items():integer(o[k],x,x)
        if request['kind']=='transfer':
            empty(o['commit_txid']);hexv(o['input_details_digest']);integer(o['input_count'],1,16);integer(o['total_input_atoms'],511000,MONEY)
            integer(o['return_branch'],1,1);need(type(o['commit_change_atoms']) is int and o['commit_change_atoms']==o['total_input_atoms']-510000)
            integer(o['commit_fee_atoms'],10000,10000);integer(o['total_fee_atoms'],20000,20000)
        else:
            need(o['commit_txid']==request['commit_txid']);empty(o['input_details_digest'])
            for k,x in {'input_count':1,'total_input_atoms':500000,'commit_change_atoms':0,'commit_fee_atoms':0,'total_fee_atoms':10000}.items():integer(o[k],x,x)
        return limited(o,4096)

    def preparation(self,v,pin,binding,crypto):
        o=fields(v,PREPARATION);integer(o['version'],1,1);need(o['binding_id']==binding)
        req=self.crequest(o['request']);o['request']=req;integer(o['prepared_at_ms'],1,TIME);hexv(o['tip_hash'],nonzero=True);integer(o['tip_height'],0,2147483647);hexv(o['ownership_head_digest'])
        review=self.review(o['review'],req,binding);o['review']=review;need(o['review_digest']==digest('fighter carrier review',review))
        need(req['ownership_pin_id']==pin['ownership_pin_id'])
        if req['kind']=='transfer':
            need(o['recovery_funding'] is None)
            n=fields(o['new_funding'],'inputs total_input_atoms funding_digest allocation_id reservation_digest return_path commit_change_atoms')
            n['inputs']=self.inputs(n['inputs']);need(n['total_input_atoms']==sum(x['atoms'] for x in n['inputs']))
            for item in n['inputs']:
                # Context is an explicit modeled live inventory; recovery uses
                # the separately immutable original inventory below.
                native=self.original['accounts'][2] if self.original is not None else None
                need(native is not None and item['index']<native['next_external' if item['branch']==0 else 'next_internal'],'live inventory')
            raw=bytes([len(n['inputs'])])+b''.join(bytes.fromhex(x['txid'])[::-1]+struct.pack('<I',x['output_index']) for x in n['inputs'])
            need(n['funding_digest']==sha(b'SLUG fighter transfer funding\0v1\0'+raw))
            need(n['allocation_id']==req['request_id']);n['return_path']=path(n['return_path'],self.address)
            need(n['return_path']['branch']==1 and n['commit_change_atoms']==n['total_input_atoms']-510000>=1000)
            need(review['input_details_digest']==digest('fighter carrier owned inputs',n['inputs']) and review['input_count']==len(n['inputs']))
            for k in ('total_input_atoms','funding_digest','commit_change_atoms'):need(review[k]==n[k])
            if pin['owner_mode']=='hosted':hexv(n['reservation_digest']);need(o['restored_allocation'] is None)
            else:
                empty(n['reservation_digest']);a=fields(o['restored_allocation'],'start_index end_index selected_index invalid_indices cursors_before cursors_after')
                r=dict(a,purpose=84,branch=1,address=n['return_path']['address'],script=n['return_path']['script'])
                self.range(r);before=cur(a['cursors_before']);after=cur(a['cursors_after']);want=list(before);need(before[5]==a['start_index']);want[5]=a['end_index'];need(after==want);need(a['selected_index']==n['return_path']['index']);o['restored_allocation']=a
            o['new_funding']=n;rp=n['return_path']
            actual=self.parse_tx(bytes.fromhex(crypto['commit_raw']))
            need(actual['outputs'][2]['script'].hex()==rp['script'],'recorded return')
        else:
            need(pin['owner_mode']=='restored' and o['new_funding'] is None and o['restored_allocation'] is None)
            r=fields(o['recovery_funding'],'commit_raw commit_txid commit_block_hash commit_height commit_position funding_digest transfer_id funding_public_key funding_branch funding_index return_path gym_authorization')
            rawhex(r['commit_raw'],4096);hexv(r['commit_txid']);hexv(r['commit_block_hash']);integer(r['commit_height'],0,2147483647);integer(r['commit_position']);point(r['funding_public_key']);integer(r['funding_branch'],0,1);integer(r['funding_index'],0,4095)
            hexv(r['gym_authorization'],64);r['return_path']=path(r['return_path'],self.address)
            need(r['commit_raw']==crypto['commit_raw'] and r['commit_txid']==crypto['commit_txid'])
            need(r['funding_public_key']==crypto['redeem'][412:478] and r['gym_authorization']==crypto['envelope'][-128:])
            need(r['funding_digest']==crypto['intent'][-64:] and r['transfer_id']==crypto['transfer_id'])
            need(req['commit_txid']==r['commit_txid']);o['recovery_funding']=r;rp=r['return_path']
            need(self.original is not None and pin['original_manifest_sha256']==sha(canonical(self.original)))
            native=self.original['accounts'][2]
            for br,ix in ((r['funding_branch'],r['funding_index']),(rp['branch'],rp['index'])):
                need(ix<native['next_external' if br==0 else 'next_internal'],'original inventory')
            first=crypto['prevouts'][0]
            need(self.derive(r['funding_branch'],r['funding_index'])[1]==first['script'],'funding path')
            need('0014'+hashlib.new('ripemd160',hashlib.sha256(bytes.fromhex(r['funding_public_key'])).digest()).hexdigest()==first['script'],'funding key')
            actual=self.parse_tx(bytes.fromhex(r['commit_raw']))
            need(actual['outputs'][2]['script'].hex()==rp['script'],'recorded return')
        for k in ('address','script','branch','index'):need(review['return_'+k]==rp[k])
        need(review['funding_digest']==crypto['intent'][-64:] and review['transfer_id']==crypto['transfer_id'])
        intent=bytes.fromhex(req['ownership_pin_id']+req['prior_state_id']+req['recipient_gym']+review['funding_digest'])
        need(review['transfer_id']==sha(b'SLUG fighter transfer id\0v1\0'+intent),'intent identity')
        need(crypto['intent']==intent.hex(),'independent carrier intent')
        return limited(o,16384)

    def package(self,v,prep,crypto):
        o=fields(v,PACKAGE);integer(o['version'],1,1);need(o['review_digest']==prep['review_digest'])
        need(o['transfer_id']==prep['review']['transfer_id']);hexv(o['gym_authorization'],64)
        b=bytes.fromhex(o['gym_authorization']);need(1<=int.from_bytes(b[:32],'big')<N and 1<=int.from_bytes(b[32:],'big')<=N//2)
        test=own(crypto)
        for leg,cap in (('commit',4096),('spend',512)):
            r=fields(o[leg],'raw txid raw_sha256');raw=rawhex(r['raw'],cap);hexv(r['txid']);need(r['raw_sha256']==sha(raw));test[leg+'_raw']=r['raw'];o[leg]=r
        result=self.crypto_validate(test)
        for leg in ('commit','spend'):need(o[leg]['txid']==result[leg+'_txid'])
        need(o['transfer_id']==result['transfer_id']);need(o['gym_authorization']==crypto['envelope'][-128:])
        want='refund' if prep['request']['kind']=='recover_refund' else 'reveal';need(o['spend_kind']==want and result['is_transfer']==(want=='reveal'))
        return limited(o,12288)

    def exclusion(self,v):
        o=fields(v,'version owner_kind request_id binding_id preparation_digest state');integer(o['version'],1,1)
        need(o['owner_kind'] in ('withdrawal','carrier') and o['state'] in ('prepared','retained'))
        hexv(o['request_id'],nonzero=True);hexv(o['binding_id']);hexv(o['preparation_digest'])
        return limited(o,512)

    def authorization(self,v,mode):
        o=fields(v,'mode action_id review_id view_id generation authorized_at_ms');need(o['mode']==mode)
        for k in ('action_id','review_id','view_id'):hexv(o[k],nonzero=True)
        integer(o['generation'],1 if mode=='hosted' else 0,GEN if mode=='hosted' else 0);integer(o['authorized_at_ms'],1,TIME)
        return o

    def dispatch(self,v,mode):
        o=fields(v,'attempt authorization intent_at_ms result result_at_ms error');integer(o['attempt'],1,U32);o['authorization']=self.authorization(o['authorization'],mode);integer(o['intent_at_ms'],o['authorization']['authorized_at_ms'],TIME)
        need(o['result'] in ('pending','acknowledged','unknown','rejected'))
        if o['result']=='pending':integer(o['result_at_ms'],0,0);empty(o['error'])
        else:integer(o['result_at_ms'],o['intent_at_ms'],TIME);need(o['error'] in ('','unavailable','not_observed','rejected','cancelled','outcome_unknown'))
        return o

    def observation(self,v,expected):
        o=fields(v,'observed_at_ms state expected_txid observed_txid node_tip node_height block_hash block_height transaction_index confirmations transfer_id raw_sha256 error');integer(o['observed_at_ms'],1,TIME);need(o['expected_txid']==expected)
        state=o['state'];need(state in ('unknown','not_observed','mempool','canonical','conflict'));need(o['error'] in ('','unavailable','not_observed','conflict'))
        if state=='unknown':
            for k in ('observed_txid','node_tip','block_hash','transfer_id','raw_sha256'):empty(o[k])
            for k in ('node_height','block_height','transaction_index','confirmations'):integer(o[k],0,0)
        else:
            hexv(o['node_tip'],nonzero=True);integer(o['node_height'],0,2147483647)
            if state=='not_observed':
                for k in ('observed_txid','transfer_id','raw_sha256'):empty(o[k])
            else:
                for k in ('observed_txid','transfer_id','raw_sha256'):hexv(o[k])
            if state in ('not_observed','mempool'):
                empty(o['block_hash'])
                for k in ('block_height','transaction_index','confirmations'):integer(o[k],0,0)
            else:
                hexv(o['block_hash'],nonzero=True);integer(o['block_height'],0,2147483647);integer(o['transaction_index']);integer(o['confirmations'])
                need(o['confirmations']==(o['node_height']-o['block_height']+1 if state=='canonical' else 0))
                if state=='canonical':need(o['node_height']>=o['block_height'])
        return o

    def operation(self,v,pin,binding,crypto):
        o=fields(v,OPERATION);integer(o['version'],1,1);integer(o['sequence'],1,128);integer(o['revision'],1,U32);integer(o['created_at_ms'],1,TIME);integer(o['updated_at_ms'],o['created_at_ms'],TIME)
        req=self.crequest(o['request']);need(o['request_id']==req['request_id']);o['request']=req;need(o['request_digest']==digest('fighter carrier request',req))
        stage=o['stage'];need(stage in ('intent','prepared','signed','commit_dispatching','commit_observed','spend_dispatching','complete','abandoned'))
        if stage=='intent':need(o['preparation'] is None)
        if o['preparation'] is None:empty(o['preparation_digest']);need(stage in ('intent','abandoned'))
        else:
            o['preparation']=self.preparation(o['preparation'],pin,binding,crypto);need(o['preparation']['request']==req);need(o['preparation_digest']==digest('fighter carrier preparation',o['preparation']))
            integer(o['preparation']['prepared_at_ms'],o['created_at_ms'],o['updated_at_ms'])
        unsigned=stage in ('intent','prepared','abandoned')
        if unsigned:
            need(o['signed_package'] is None and o['sign_authorization'] is None and o['commit_dispatch'] is None and o['spend_dispatch'] is None);empty(o['signed_package_digest'])
        else:
            need(o['preparation'] is not None);o['signed_package']=self.package(o['signed_package'],o['preparation'],crypto);need(o['signed_package_digest']==digest('fighter carrier signed package',o['signed_package']));o['sign_authorization']=self.authorization(o['sign_authorization'],pin['owner_mode'])
            integer(o['sign_authorization']['authorized_at_ms'],o['preparation']['prepared_at_ms'],o['updated_at_ms'])
        if stage=='signed':need(o['commit_dispatch'] is None and o['spend_dispatch'] is None)
        for leg in ('commit','spend'):
            if o[leg+'_dispatch'] is not None:
                o[leg+'_dispatch']=self.dispatch(o[leg+'_dispatch'],pin['owner_mode']);integer(o[leg+'_dispatch']['intent_at_ms'],o['created_at_ms'],o['updated_at_ms'])
                need(o[leg+'_dispatch']['result_at_ms']<=o['updated_at_ms'])
                integer(o[leg+'_dispatch']['authorization']['authorized_at_ms'],o['sign_authorization']['authorized_at_ms'],o['updated_at_ms'])
            if o[leg+'_observation'] is not None:
                need(o['signed_package'] is not None);o[leg+'_observation']=self.observation(o[leg+'_observation'],o['signed_package'][leg]['txid'])
                integer(o[leg+'_observation']['observed_at_ms'],o['created_at_ms'],o['updated_at_ms'])
        if req['kind']!='transfer':need(o['commit_dispatch'] is None)
        if stage in ('commit_dispatching','commit_observed'):need(req['kind']=='transfer' and o['commit_dispatch'] is not None and o['spend_dispatch'] is None)
        if stage in ('spend_dispatching','complete'):need(o['spend_dispatch'] is not None)
        if stage in ('spend_dispatching','complete') and req['kind']=='transfer':need(o['commit_dispatch'] is not None)
        if stage in ('commit_observed','spend_dispatching','complete') and req['kind']=='transfer':need(o['commit_observation'] is not None and o['commit_observation']['state'] in ('mempool','canonical'))
        if stage=='complete':need(o['spend_observation'] is not None and o['spend_observation']['state'] in ('mempool','canonical'))
        return limited(o,49152)

    def state(self,v,pin,journal_id,crypto_by_request):
        o=fields(v,'application version journal_id binding_id clock_high_water_ms records');need(o['application']=='slug-fighter-carrier-journal');integer(o['version'],1,1);need(o['journal_id']==journal_id)
        binding=digest('fighter carrier binding',dict(journal_id=journal_id,pin=pin));need(o['binding_id']==binding)
        integer(o['clock_high_water_ms'],0,TIME);need(type(o['records']) is list and len(o['records'])<=128)
        seen=set();active=0;rows=[]
        for seq,row in enumerate(o['records'],1):
            r=self.operation(row,pin,binding,crypto_by_request[row['request_id']]);need(r['sequence']==seq and r['request_id'] not in seen);seen.add(r['request_id']);need(r['updated_at_ms']<=o['clock_high_water_ms'])
            active+=r['stage'] not in ('complete','abandoned');rows.append(r)
        need(active<=1,'active requests');o['records']=rows
        if not rows:integer(o['clock_high_water_ms'],0,0)
        return limited(o,8388608)

    def wallet_record(self,v,pin,binding,crypto):
        o=fields(v,'version sequence request_id binding_id state preparation preparation_digest signed_package_digest commit_txid spend_txid')
        integer(o['version'],1,1);integer(o['sequence'],1,128);need(o['binding_id']==binding)
        o['preparation']=self.preparation(o['preparation'],pin,binding,crypto)
        need(o['request_id']==o['preparation']['request']['request_id'])
        need(o['preparation_digest']==digest('fighter carrier preparation',o['preparation']))
        need(o['state'] in ('prepared','retained','complete','abandoned'))
        for k in ('signed_package_digest','commit_txid','spend_txid'):
            if o['state'] in ('prepared','abandoned'):empty(o[k])
            else:hexv(o[k])
        return limited(o,24576)

    def exclusions(self,rows,records,packages,withdrawals=()):
        need(type(rows) is list and len(rows)<=4224,'capacity');expected={}
        for withdrawal in withdrawals:
            for item in withdrawal['inputs']:
                key=bytes.fromhex(hexv(item['txid']))[::-1]+struct.pack('<I',integer(item['output_index']))
                need(key not in expected,'overlap')
                expected[key]=self.exclusion(dict(version=1,owner_kind='withdrawal',request_id=withdrawal['request_id'],binding_id=withdrawal['binding_id'],preparation_digest=withdrawal['preparation_digest'],state=withdrawal['state']))
        for record in records:
            p=record['preparation'];kind=p['request']['kind'];state=record['state']
            if state=='abandoned':continue
            keys=[]
            if kind=='transfer':
                keys=[bytes.fromhex(x['txid'])[::-1]+struct.pack('<I',x['output_index']) for x in p['new_funding']['inputs']]
                if state in ('retained','complete'):keys.append(bytes.fromhex(record['commit_txid'])[::-1]+bytes(4))
            else:keys=[bytes.fromhex(p['request']['commit_txid'])[::-1]+bytes(4)]
            for key in keys:
                need(key not in expected,'overlap')
                expected[key]=dict(version=1,owner_kind='carrier',request_id=record['request_id'],binding_id=record['binding_id'],preparation_digest=record['preparation_digest'],state='prepared' if state=='prepared' else 'retained')
            if state in ('retained','complete'):
                package=packages[record['request_id']]
                need(record['signed_package_digest']==digest('fighter carrier signed package',package))
                need(record['commit_txid']==package['commit']['txid'] and record['spend_txid']==package['spend']['txid'])
        got={};logical=0
        for row in rows:
            r=fields(row,'key_hex value');key=bytes.fromhex(hexv(r['key_hex'],36));need(key not in got,'duplicate exclusion');value=self.exclusion(r['value']);got[key]=value;logical+=len(key)+len(canonical(value))
        need(logical<=4194304,'capacity');need(got==expected,'dangling exclusion')
        return rows

def prewalk(raw):
    """Bound lexical containers/string spans before Python's DTO allocations."""
    pos=0;objects=0;tokens=0;size=len(raw)
    array_caps={'records':128,'inputs':16,'invalid_indices':127,'cursors':8,
                'activation_cursors':8,'cursors_before':8,'cursors_after':8,'accounts':4}
    def token():
        nonlocal tokens
        tokens+=1;need(tokens<=262144,'token capacity')
    def ws():
        nonlocal pos
        while pos<size and raw[pos] in b' \t\r\n':pos+=1
    def string(key=False):
        nonlocal pos
        start=pos;need(pos<size and raw[pos]==34);pos+=1
        while pos<size:
            c=raw[pos];pos+=1
            if c==34:
                need(pos-start<=((128 if key else 8192)+2),'string capacity')
                try:return json.loads(raw[start:pos])
                except ValueError as e:raise Refused('string') from e
            need(c>=32,'string')
            if c==92:
                need(pos<size);pos+=1
            need(pos-start<=((128 if key else 8192)+2),'string capacity')
        raise Refused('truncated')
    def value(depth,member=''):
        nonlocal pos,objects
        need(depth<=16,'depth');ws();need(pos<size);token();c=raw[pos]
        if c==123:
            objects+=1;need(objects<=8192,'object capacity');pos+=1;ws()
            if pos<size and raw[pos]==125:pos+=1;return
            while True:
                ws();name=string(True);token();ws();need(pos<size and raw[pos]==58);pos+=1;value(depth+1,name);ws();need(pos<size)
                c=raw[pos];pos+=1
                if c==125:return
                need(c==44)
        if c==91:
            need(member in array_caps,'array path');pos+=1;ws();n=0
            if pos<size and raw[pos]==93:pos+=1;return
            while True:
                n+=1;need(n<=array_caps[member],'array capacity');value(depth+1);ws();need(pos<size);c=raw[pos];pos+=1
                if c==93:return
                need(c==44)
        if c==34:string();return
        start=pos
        while pos<size and raw[pos] not in b',]} \t\r\n':pos+=1
        need(pos>start and pos-start<=32,'scalar span')
    value(1);ws();need(pos==size,'trailing')

def raw_case(raw,cap,normalize):
    need(type(raw) is bytes and 0<len(raw)<=cap,'capacity')
    prewalk(raw)
    def pairs(items):
        o={}
        for k,v in items:need(k not in o,'duplicate');o[k]=v
        return o
    def number(s):need(re.fullmatch('0|[1-9][0-9]*',s) is not None,'number');return int(s)
    def refuse(s):raise Refused('number')
    try:v=json.loads(raw.decode('utf8'),object_pairs_hook=pairs,parse_int=number,parse_float=refuse,parse_constant=refuse)
    except (UnicodeError,ValueError) as e:raise Refused('json') from e
    out=normalize(v);need(canonical(out)==raw,'canonical');return out
