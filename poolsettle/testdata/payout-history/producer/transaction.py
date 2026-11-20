"""Bounded Bitcoin serialization parser written independently for this oracle.

No script validation or chain claims. The original stored txid/output values
are separate anchors. Synthetic serialization cases are explicitly modeled.
"""
import hashlib
from common import require, integer, script, MAX_MONEY, MIN_MONEY

def compact(n):
    if n<253:return bytes([n])
    if n<=65535:return b'\xfd'+n.to_bytes(2,'little')
    if n<=4294967295:return b'\xfe'+n.to_bytes(4,'little')
    return b'\xff'+n.to_bytes(8,'little')
def parse_transaction(raw):
    require(type(raw) is bytes and 1<=len(raw)<=100000)
    at=0
    def take(n):
        nonlocal at
        require(0<=n<=len(raw)-at); b=raw[at:at+n];at+=n;return b
    def count():
        first=take(1)[0]
        if first<253:return first
        width={253:2,254:4,255:8}[first];v=int.from_bytes(take(width),'little')
        require(v>={253:253,254:65536,255:4294967296}[first]);return v
    version=take(4); witness=False
    if raw[at:at+1]==b'\0':
        require(take(2)==b'\0\1');witness=True
    vin=count();require(1<=vin<=256 and vin<=(len(raw)-at)//41)
    inputs=[]
    for _ in range(vin):
        begin=at;take(36);n=count();take(n);take(4);inputs.append(raw[begin:at])
    vout=count();require(1<=vout<=257 and vout<=(len(raw)-at)//9)
    outputs=[];output_bytes=[];total=0
    for _ in range(vout):
        begin=at;value=int.from_bytes(take(8),'little',signed=True);integer(value,0,MAX_MONEY)
        total+=value;require(total<=MAX_MONEY);n=count();pk=take(n)
        outputs.append({'atoms':value,'script':pk.hex()});output_bytes.append(raw[begin:at])
    if witness:
        any_stack=False
        for _ in range(vin):
            n=count();require(n<=len(raw)-at);any_stack|=n>0
            for _ in range(n):take(count())
        require(any_stack)
    lock=take(4);require(at==len(raw))
    stripped=version+compact(vin)+b''.join(inputs)+compact(vout)+b''.join(output_bytes)+lock
    txid=hashlib.sha256(hashlib.sha256(stripped).digest()).digest()[::-1].hex()
    return {'txid':txid,'input_count':vin,'output_count':vout,'has_witness':witness,'stripped_hex':stripped.hex(),'outputs':outputs}
def check_selection(raw_hex,selected,rpc_txid=None):
    require(type(raw_hex) is str and len(raw_hex)%2==0 and 2<=len(raw_hex)<=200000 and raw_hex.lower()==raw_hex)
    require(all(c in '0123456789abcdef' for c in raw_hex))
    tx=parse_transaction(bytes.fromhex(raw_hex));require(tx['txid']==selected['txid'])
    if rpc_txid is not None:require(tx['txid']==rpc_txid)
    integer(selected['output_index'],0,255);integer(selected['atoms'],MIN_MONEY,MAX_MONEY);script(selected['recipient_script'])
    require(selected['output_index']<tx['output_count'])
    out=tx['outputs'][selected['output_index']]
    require(out['atoms']==selected['atoms'] and out['script']==selected['recipient_script'])
    return tx
def modeled_transaction(recipient,atoms=100000000,witness=False,empty_stack=False,inputs=1,outputs=1):
    # Deliberately unsigned public serialization, not a valid broadcast fixture.
    version=(2).to_bytes(4,'little');vin=bytes.fromhex('11'*32)+(0).to_bytes(4,'little')+b'\0'+bytes.fromhex('ffffffff')
    out=atoms.to_bytes(8,'little',signed=True)+compact(len(bytes.fromhex(recipient)))+bytes.fromhex(recipient)
    body=compact(inputs)+vin*inputs+compact(outputs)+out*outputs
    witness_bytes=(b'\0' if empty_stack else b'\1\0')*inputs if witness else b''
    return version+(b'\0\1' if witness else b'')+body+witness_bytes+b'\0'*4
