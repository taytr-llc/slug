"""Finite encoded witness measurements, not a maximum or total-heap proof.
The inherited/adopted conservative bounds were reviewed separately. This check
only ensures this finite independent corpus fits its exact declared family cap.
"""
CAPS={'seed_pin':64*1024,'card_pin':64*1024,'bout_pin':64*1024,'result_pin':64*1024,'event_receipt':232*1024,'seed_declaration':384*1024,'seed_signed_declaration':384*1024,'seed_record':384*1024}
def compose(cases):
    out=[]
    for case in cases:
        size=len(bytes.fromhex(case['canonical_hex']));cap=CAPS[case['family']]
        assert size<=cap
        out.append(dict(name=case['family']+'/'+case['name'],encoded_bytes=size,declared_family_cap=cap,scope='finite witness only; not maximum encoded input or total Go heap proof'))
    return out
