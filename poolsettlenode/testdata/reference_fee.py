#!/usr/bin/env python3
"""Independent arithmetic: Bitcoin native-witness framing and D58 fee units.
No Go import, product output, networking or private key use.
"""
import json
from fractions import Fraction
from math import ceil

def compact(n): return 1 if n<253 else 3 if n<=65535 else 5
cases=[]
for n,m in [(1,1),(2,1),(8,8),(1,251),(1,252),(252,1),(253,1),(256,256)]:
    # Serialize a maximum supported native-key witness symbolically: two items,
    # length byte + 73-byte signature, length byte + 33-byte compressed key.
    base=4+compact(n)+n*(32+4+1+4)+compact(m+1)+(m+1)*(8+1+22)+4
    witness=2+n*(1+1+73+1+33)
    virtual=ceil(Fraction(base*4+witness,4))
    for rate in [1,1000,1234,1000000000]:
        cases.append(dict(inputs=n,payments=m,rate=rate,vsize=virtual,fee=ceil(Fraction(rate*virtual,1000))))
rates=[dict(decimal=s,rate=ceil(Fraction(s)*100000000)) for s in ['0.00001','1e-8','0.000000001','0.000010001','1.23456789','10']]
print(json.dumps(dict(version=1,scope='independent size and exact fee arithmetic; no funding or estimator observation',sizes=cases,rates=rates),indent=2))
