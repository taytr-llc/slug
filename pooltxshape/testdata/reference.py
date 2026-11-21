#!/usr/bin/env python3
"""Independent finite symbolic native84 serialization, not product outputs.
Root must approve the frozen source/input lock before running this producer.
No keys, signatures, networking, Go imports or custody operations.
"""
import hashlib
import json
import struct
from pathlib import Path

HERE=Path(__file__).resolve().parent
INPUT_SHA='eb270e0b4cc22ecc5b3b6c768e58611da9588b301cdf0075e8bc1b80cb404715'
CONTRACT_SHA='671997100f7dfe07e726249e0121bafad3fbd095e81c289ed196b200617df16b'

def compact(n):
    assert type(n) is int and 0 <= n <= 0xffffffffffffffff
    if n < 253:
        return bytes([n])
    if n <= 65535:
        return b'\xfd' + struct.pack('<H',n)
    if n <= 0xffffffff:
        return b'\xfe' + struct.pack('<I',n)
    return b'\xff' + struct.pack('<Q',n)

def symbolic_shape(n,p,change):
    assert 1<=n<=1464 and 1<=p<=256 and type(change) is bool
    o=p+int(change)
    vin=bytes(32)+bytes(4)+compact(0)+b'\xff'*4
    vout=struct.pack('<Q',100000000)+compact(22)+b'\x00\x14'+bytes(20)
    version=struct.pack('<I',2)
    locktime=bytes(4)
    stripped=version+compact(n)+vin*n+compact(o)+vout*o+locktime
    witness=compact(2)+compact(73)+b's'*73+compact(33)+b'k'*33
    full=version+b'\x00\x01'+compact(n)+vin*n+compact(o)+vout*o+witness*n+locktime
    assert len(full) <= 300000
    weight=3*len(stripped)+len(full)
    return dict(stripped_bytes=len(stripped),witness_bytes=len(full)-len(stripped),
                raw_bytes=len(full),weight=weight,vsize=(weight+3)//4)

def main():
    raw=(HERE/'inputs/cases.json').read_bytes()
    assert len(raw)<=16384 and hashlib.sha256(raw).hexdigest()==INPUT_SHA
    assert hashlib.sha256((HERE/'CONTRACT.md').read_bytes()).hexdigest()==CONTRACT_SHA
    data=json.loads(raw)
    assert data['version']==1 and len(data['cases'])==26
    cases=[]
    seen=set()
    for recipe in data['cases']:
        assert set(recipe)=={'name','inputs','payees','change'}
        name,n,p,c=recipe['name'],recipe['inputs'],recipe['payees'],recipe['change']
        assert isinstance(name,str) and name not in seen and len(name)<=64
        assert type(n) is int and type(p) is int and type(c) is bool
        seen.add(name)
        dimensions=None
        if not(1<=n<=1464 and 1<=p<=256):
            result='invalid'
        else:
            dimensions=symbolic_shape(n,p,c)
            result='ok' if dimensions['weight']<=400000 else 'capacity'
        cases.append(dict(**recipe,result=result,dimensions=dimensions))
    # Fixed independent hand calculations, not values learned from Go.
    expected={
        'daily-721-64':(31588,78591,110179,204943,51236),
        'daily-721-256':(37542,78591,116133,228759,57190),
        'full-payees-1348':(63249,146934,210183,399930,99983),
        'full-payees-1349':(63290,147043,210333,400203,100051),
        'exact-weight':(62776,148896,211672,400000,100000),
        'over-exact-weight':(62807,148896,211703,400124,100031),
        'maximum-inputs':(60098,159578,219676,399970,99993),
    }
    for row in cases:
        if row['name'] in expected:
            d=row['dimensions']
            assert tuple(d[k] for k in ['stripped_bytes','witness_bytes','raw_bytes','weight','vsize'])==expected[row['name']]
    out=dict(version=1,scope='symbolic native84 shape only; no signature, funding, ledger or daily payout execution',
             input_sha256=INPUT_SHA,contract_sha256=CONTRACT_SHA,cases=cases)
    encoded=json.dumps(out,indent=2)+'\n'
    assert len(encoded.encode())<=65536
    print(encoded,end='')

if __name__=='__main__':
    main()
