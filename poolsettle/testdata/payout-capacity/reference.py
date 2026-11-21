#!/usr/bin/env python3
"""Independent symbolic native84 serialized-size boundary arithmetic.
No product import, signatures, keys, network or custody operations.
"""
import json
import struct

def compact(n):
    if not 0 <= n <= 0xffffffffffffffff:
        raise ValueError("CompactSize domain")
    if n < 253:
        return bytes([n])
    if n <= 65535:
        return b'\xfd' + struct.pack('<H', n)
    if n <= 0xffffffff:
        return b'\xfe' + struct.pack('<I', n)
    return b'\xff' + struct.pack('<Q', n)

def dimensions(n, payments, change):
    outputs = payments + int(change)
    if not (1 <= n <= 1000 and 1 <= payments <= 1000):
        return None
    # Symbolic bytes, not valid cryptographic signatures or spend authority.
    vin = bytes(32) + bytes(4) + compact(0) + b'\xff' * 4
    vout = struct.pack('<Q', 100000000) + compact(22) + b'\x00\x14' + bytes(20)
    stripped = struct.pack('<I', 2) + compact(n) + vin*n + compact(outputs) + vout*outputs + bytes(4)
    witness = compact(2) + compact(73) + b's'*73 + compact(33) + b'k'*33
    signed = struct.pack('<I', 2) + b'\x00\x01' + compact(n) + vin*n + compact(outputs) + vout*outputs + witness*n + bytes(4)
    return dict(outputs=outputs, stripped_bytes=len(stripped), witness_and_marker_bytes=2+n*len(witness), raw_bytes=len(signed), vsize=(4*len(stripped)+2+n*len(witness)+3)//4)

shapes = [(1,1,False),(1,1,True),(252,1,False),(253,1,False),
          (1,251,True),(1,252,False),(1,252,True),(1,253,False),
          (256,256,False),(256,256,True),(257,1,False),
          (1,257,False),(0,1,False),(-1,1,True),(1,0,False),(1,-1,True)]
# Literal finite search is over symbolic size only, never nonce/keys/work.
for target in (99999,100000,100001):
    options = []
    for n in range(253,668):
        for p in range(1,257):
            o=p
            raw=4+len(compact(n))+41*n+len(compact(o))+31*o+4+2+109*n
            if raw == target:
                options.append((n,p,False))
    assert options, target
    shapes.append(min(options))
cases=[]
for n,p,c in shapes:
    d=dimensions(n,p,c)
    cases.append(dict(inputs=n,payments=p,change=c,dimensions=d,
                      allowed=1<=n<=256 and 1<=p<=256 and d['raw_bytes']<=100000,
                      scope='size-only-outside-valid-counts' if not (1<=n<=256 and 1<=p<=256) else 'valid-counts-symbolic-size'))
assert dimensions(256,256,True)['raw_bytes'] == 46383
assert dimensions(256,256,True)['vsize'] == 25454
print(json.dumps(dict(version=1,scope='independent symbolic native84 byte/count arithmetic only; no funding, signature, capacity-continuation or RunDaily execution claim',cases=cases),indent=2))
