"""Independent carrier arithmetic/encoding model; never imports Go or a node.

R and E are the separately locked inherited Python recovery and ECDSA modules.
Only published fixture seeds enter Model. No private scalar is returned/emitted.
Validation proves the modeled transaction/signature relationships, not canonical
chain inclusion, real input availability, admission or checkpoint authority.
"""
import hashlib
import struct

H = lambda b: hashlib.sha256(b).digest()
D = lambda b: H(H(b))
U32 = lambda n: struct.pack('<I', n)
U64 = lambda n: struct.pack('<Q', n)
MAX_MONEY = 21000000 * 100000000
FUND = b'SLUG fighter transfer funding\x00v1\x00'
AUTH = b'SLUG fighter transfer authorization\x00v1\x00'
IDENT = b'SLUG fighter transfer id\x00v1\x00'


class Rejected(ValueError):
    pass


class Unavailable(ValueError):
    pass


def need(v, reason):
    if not v:
        raise Rejected(reason)


def compact(n):
    need(type(n) is int and 0 <= n < 2**64, 'count')
    if n < 253:
        return bytes([n])
    if n <= 65535:
        return b'\xfd' + struct.pack('<H', n)
    if n <= 0xffffffff:
        return b'\xfe' + U32(n)
    return b'\xff' + U64(n)


def blob(b):
    return compact(len(b)) + b


def h160(b):
    return hashlib.new('ripemd160', H(b)).digest()


def outpoint(c):
    return bytes.fromhex(c['hash_wire']) + U32(c['index'])


def output(o):
    return U64(o['atoms']) + blob(o['script'])


def serialize(t, witness=True):
    w = witness and any(i.get('witness') for i in t['inputs'])
    b = U32(t['version']) + (b'\x00\x01' if w else b'') + compact(len(t['inputs']))
    for i in t['inputs']:
        b += outpoint(i) + blob(i.get('script', b'')) + U32(i['sequence'])
    b += compact(len(t['outputs'])) + b''.join(output(o) for o in t['outputs'])
    if w:
        for i in t['inputs']:
            stack = i.get('witness', [])
            b += compact(len(stack)) + b''.join(blob(x) for x in stack)
    return b + U32(t['locktime'])


class Reader:
    def __init__(self, b):
        need(type(b) is bytes and len(b) <= 4000000, 'raw_size')
        self.b, self.p = b, 0

    def take(self, n):
        need(0 <= n <= len(self.b) - self.p, 'truncated')
        value = self.b[self.p:self.p+n]
        self.p += n
        return value

    def number(self, n):
        return int.from_bytes(self.take(n), 'little')

    def count(self, cap):
        n = self.number(1)
        if n >= 253:
            size = {253: 2, 254: 4, 255: 8}[n]
            n = self.number(size)
            need(n >= {2: 253, 4: 65536, 8: 2**32}[size], 'nonminimal_compactsize')
        need(n <= cap, 'count_capacity')
        return n

    def blob(self, cap):
        return self.take(self.count(cap))


def parse_tx(raw, previous=False):
    r = Reader(raw)
    version = r.number(4)
    witness = r.b[r.p:r.p+1] == b'\x00'
    if witness:
        need(r.take(2) == b'\x00\x01', 'witness_flags')
    n = r.count(len(raw)//41 if previous else 17)
    need(n > 0, 'empty_inputs')
    ins = []
    for _ in range(n):
        ins.append(dict(hash_wire=r.take(32).hex(), index=r.number(4),
                        script=r.blob(len(raw) if previous else 10000), sequence=r.number(4), witness=[]))
    outs = []
    for _ in range(r.count(len(raw)//9 if previous else 4)):
        atoms = r.number(8)
        need(atoms <= MAX_MONEY, 'output_value')
        outs.append(dict(atoms=atoms, script=r.blob(len(raw) if previous else 10000)))
    need(sum(o['atoms'] for o in outs) <= MAX_MONEY, 'output_total')
    if witness:
        for i in ins:
            i['witness'] = [r.blob(len(raw) if previous else 10000) for _ in range(r.count(len(raw) if previous else 1000))]
        need(any(i['witness'] for i in ins), 'superfluous_witness')
    t = dict(version=version, inputs=ins, outputs=outs, locktime=r.number(4))
    need(r.p == len(raw) and serialize(t) == raw, 'trailing_or_framing')
    return t


def push(b, nonminimal=False):
    need(len(b) <= 520, 'element_size')
    if nonminimal:
        return b'\x4d' + struct.pack('<H', len(b)) + b
    if len(b) <= 75:
        return bytes([len(b)]) + b
    if len(b) <= 255:
        return b'\x4c' + bytes([len(b)]) + b
    return b'\x4d' + struct.pack('<H', len(b)) + b


def pushes(script):
    need(len(script) <= 10000, 'scriptsig_size')
    r, values = Reader(script), []
    while r.p < len(script):
        op = r.number(1)
        if op <= 75:
            b = r.take(op)
        elif op in (76, 77, 78):
            n = r.number({76: 1, 77: 2, 78: 4}[op])
            need(n <= 520, 'element_size')
            b = r.take(n)
        elif op == 79:
            b = b'\x81'
        elif 81 <= op <= 96:
            b = bytes([op - 80])
        else:
            raise Rejected('non_push_or_reserved_opcode')
        values.append(b)
        need(len(values) <= 1000, 'stack_capacity')
    return values


def witness_hash(t, pos, coin):
    script = bytes.fromhex(coin['script'])
    code = b'\x76\xa9\x14' + script[2:] + b'\x88\xac'
    preimage = (U32(t['version']) + D(b''.join(outpoint(i) for i in t['inputs'])) +
                D(b''.join(U32(i['sequence']) for i in t['inputs'])) + outpoint(t['inputs'][pos]) +
                blob(code) + U64(coin['atoms']) + U32(t['inputs'][pos]['sequence']) +
                D(b''.join(output(o) for o in t['outputs'])) + U32(t['locktime']) + U32(1))
    return D(preimage)


def legacy_hash(t, redeem):
    need(len(t['inputs']) == 1, 'spend_inputs')
    i = dict(t['inputs'][0], script=redeem, witness=[])
    return D(serialize(dict(t, inputs=[i]), witness=False) + U32(1))


class Model:
    def __init__(self, recovery, ecdsa, published, recipe):
        self.R, self.E, self.recipe = recovery, ecdsa, recipe
        need(published.get('fixture_only') is True, 'unpublished_fixtures')
        self.fixtures = {r['name']: r for r in published['cases'] if r['name'] in recipe['fixtures']}
        need(set(self.fixtures) == set(recipe['fixtures']), 'fixtures')
        self.keys = {}
        for name, row in self.fixtures.items():
            for path in [recipe['gym_path'], *recipe['funding_paths'], recipe['return_path']]:
                key = recovery.derive_path(bytes.fromhex(row['seed']), path)
                self.keys[(name, path)] = key.scalar
            pub = self.public(name, recipe['gym_path'])
            need(pub.hex() == row['export']['manifest']['gym_public_key'], 'old_gym_identity')

    def public(self, fixture, path):
        return self.E.public_key(self.keys[(fixture, path)])

    def point(self, pub):
        need(len(pub) == 33 and pub[0] in (2, 3), 'public_key_encoding')
        x = int.from_bytes(pub[1:], 'big')
        need(x < self.E.FIELD, 'public_key_x')
        y = pow((x*x*x + 7) % self.E.FIELD, (self.E.FIELD + 1)//4, self.E.FIELD)
        need((y*y - x*x*x - 7) % self.E.FIELD == 0, 'public_key_point')
        if y & 1 != pub[0] & 1:
            y = self.E.FIELD - y
        return x, y

    def der(self, sig):
        need(len(sig) >= 9 and sig[-1] == 1, 'sighash_all')
        b = sig[:-1]
        need(b[0] == 0x30 and b[1] == len(b)-2 and b[2] == 2, 'der')
        nr = b[3]; need(nr > 0 and 4+nr+2 <= len(b), 'der_r')
        rb = b[4:4+nr]; pos = 4+nr
        need(b[pos] == 2, 'der_s'); ns = b[pos+1]; sb = b[pos+2:]
        need(ns == len(sb) and ns > 0, 'der_s')
        for value in (rb, sb):
            need(value[0] < 128 and not (len(value)>1 and value[0]==0 and value[1]<128), 'der_integer')
        rr, ss = int.from_bytes(rb, 'big'), int.from_bytes(sb, 'big')
        need(1 <= rr < self.E.ORDER and 1 <= ss < self.E.ORDER, 'der_scalar')
        return rr, ss

    def verify_der(self, pub, digest, sig):
        rr, ss = self.der(sig)
        need(self.E.verify(self.point(pub), digest, rr, ss), 'funding_signature')

    def funding_tuple(self, coins):
        need(1 <= len(coins) <= 16, 'input_count')
        ops = [outpoint(c) for c in coins]
        need(all(len(o) == 36 for o in ops) and len(set(ops)) == len(ops), 'duplicate_outpoint')
        need(ops == sorted(ops), 'outpoint_order')
        return bytes([len(ops)]) + b''.join(ops)

    def intent(self, pin, prior, recipient, coins):
        need(len(pin) == len(prior) == 32 and any(pin) and any(prior), 'zero_or_bad_id')
        self.point(recipient)
        return pin + prior + recipient + H(FUND + self.funding_tuple(coins))

    def sign_gym(self, sender, intent, alternative=False):
        secret = self.keys[(sender, self.recipe['gym_path'])]
        digest = H(AUTH + intent)
        if alternative:
            # Explicit modeled non-RFC writer: fixed public-reference nonce2.
            k = 2; rr = self.E.multiply(k)[0] % self.E.ORDER
            ss = pow(k, -1, self.E.ORDER) * (int.from_bytes(digest, 'big') + rr*secret) % self.E.ORDER
            ss = min(ss, self.E.ORDER-ss)
        else:
            rr, ss = self.E.sign(secret, digest)
        return rr.to_bytes(32, 'big') + ss.to_bytes(32, 'big')

    def make(self, sender, count=1, variant='standard', payer=None, suffix='base'):
        recipe = self.recipe; payer = payer or sender
        recipient_name = next(n for n in recipe['fixtures'] if n != sender)
        recipient = self.public(recipient_name, recipe['gym_path'])
        coins, parents = [], []
        for n in range(count):
            path = recipe['funding_paths'][n % len(recipe['funding_paths'])]
            pub = self.public(payer, path)
            pt = recipe['parent_transaction']
            label = pt['dummy_input_hash_domain'] + '/' + suffix + '/' + sender + '/' + str(n)
            parent = dict(version=pt['version'], inputs=[dict(hash_wire=H(label.encode()).hex(),
                          index=pt['dummy_input_index'], script=bytes.fromhex(pt['dummy_input_script_hex']),
                          sequence=pt['sequence'], witness=[])],
                          outputs=[dict(atoms=pt['atoms'],script=b'\x00\x14'+h160(pub))],locktime=pt['locktime'])
            if variant == 'large_unrelated_parent_output':
                extra=pt['unrelated_large_output']
                parent['outputs'].append(dict(atoms=extra['atoms'],script=bytes.fromhex(extra['script_prefix_hex'])+bytes(extra['zero_padding_bytes'])))
            parents.append(serialize(parent).hex())
            coins.append(dict(hash_wire=D(serialize(parent,False)).hex(), index=pt['output_index'],
                              atoms=pt['atoms'], script=(b'\x00\x14'+h160(pub)).hex(),
                              fixture=payer, path=path))
        coins.sort(key=outpoint)
        pin, prior = bytes.fromhex(recipe['pin_id']), bytes.fromhex(recipe['prior_state_id'])
        intent = self.intent(pin, prior, recipient, coins)
        sig = self.sign_gym(sender, intent, variant == 'non_rfc_gym')
        envelope = b'SLUGFTR1\x01' + intent + sig
        fundpub = self.public(coins[0]['fixture'], coins[0]['path'])
        redeem = b'\x4c\xca' + envelope + b'\x75\x21' + fundpub + b'\xac'
        change = b'\x00\x14' + h160(self.public(payer, recipe['return_path']))
        locator = b'SLUGFCM1\x01' + prior + recipient
        commit = dict(version=1, inputs=[dict(c, sequence=0xffffffff, script=b'', witness=[]) for c in coins],
                      outputs=[dict(atoms=500000, script=b'\xa9\x14'+h160(redeem)+b'\x87'),
                               dict(atoms=0, script=b'\x6a'+push(locator)),
                               dict(atoms=sum(c['atoms'] for c in coins)-510000, script=change)], locktime=0)
        for pos, coin in enumerate(coins):
            rr, ss = self.E.sign(self.keys[(coin['fixture'],coin['path'])], witness_hash(commit,pos,coin))
            commit['inputs'][pos]['witness'] = [self.E.der_signature(rr,ss)+b'\x01',self.public(coin['fixture'],coin['path'])]
        transfer = H(IDENT+intent)
        spend = dict(version=1,inputs=[dict(hash_wire=D(serialize(commit,False)).hex(),index=0,
                                          sequence=0xffffffff,script=b'',witness=[])],
                     outputs=[dict(atoms=490000,script=change)],locktime=0)
        if variant != 'refund':
            spend['outputs'].append(dict(atoms=0,script=b'\x6a'+push(b'SLUGFRV1\x01'+transfer)))
        first = coins[0]
        rr, ss = self.E.sign(self.keys[(first['fixture'],first['path'])], legacy_hash(spend,redeem))
        if variant == 'high_s_outer': ss = self.E.ORDER-ss
        outer = self.E.der_signature(rr,ss)+b'\x01'
        prefix = push(b'harmless') if variant == 'extra_stack' else b''
        spend['inputs'][0]['script'] = prefix+push(outer,variant=='nonminimal_outer')+push(redeem,variant=='nonminimal_outer')
        return dict(name=sender+'-'+str(count)+'-'+variant+'-'+suffix, sender_gym=self.public(sender,recipe['gym_path']).hex(),
                    recipient_gym=recipient.hex(), expected_pin=pin.hex(), expected_prior=prior.hex(),
                    prevouts=coins,funding_transactions=parents,commit_raw=serialize(commit).hex(),spend_raw=serialize(spend).hex(),
                    intent=intent.hex(),authorization_digest=H(AUTH+intent).hex(),envelope=envelope.hex(),
                    redeem=redeem.hex(),transfer_id=transfer.hex(),commit_txid=D(serialize(commit,False))[::-1].hex(),
                    commit_wtxid=D(serialize(commit))[::-1].hex(),spend_txid=D(serialize(spend,False))[::-1].hex(),
                    commit_fee_atoms=10000,spend_fee_atoms=10000,temporary_lock_atoms=500000,
                    final_return_atoms=490000,is_transfer=variant!='refund',
                    deterministic_reconstruction=variant!='non_rfc_gym',
                    provenance='full modeled parent transactions with hash-bound selected outputs; published fixture-derived keys; no chain existence/parent signature/inclusion/admission/finality claim')

    def validate(self, row):
        commit, spend = parse_tx(bytes.fromhex(row['commit_raw'])), parse_tx(bytes.fromhex(row['spend_raw']))
        coins = row['prevouts']; ins = commit['inputs']; outs = commit['outputs']
        need(commit['version']==1 and commit['locktime']==0 and 1<=len(ins)<=16, 'commit_header')
        need(len(coins)==len(ins) and all(outpoint(c)==outpoint(i) for c,i in zip(coins,ins)), 'prevout_binding')
        parent_outputs={}
        need(len(row['funding_transactions'])<=16,'parent_count')
        for raw in row['funding_transactions']:
            parent=parse_tx(bytes.fromhex(raw),previous=True)
            parent_id=D(serialize(parent,False)).hex()
            need(parent_id not in parent_outputs,'duplicate_parent')
            parent_outputs[parent_id]=parent['outputs']
        for c in coins:
            if c['hash_wire'] not in parent_outputs:
                raise Unavailable('missing_parent')
            outputs=parent_outputs[c['hash_wire']]
            need(c['index']<len(outputs),'prevout_index')
            need(outputs[c['index']]==dict(atoms=c['atoms'],script=bytes.fromhex(c['script'])),'parent_prevout_mismatch')
        tuple_bytes=self.funding_tuple(coins)
        need(all(type(c['atoms']) is int and 0<c['atoms']<=MAX_MONEY for c in coins), 'prevout_value')
        need(sum(c['atoms'] for c in coins)<=MAX_MONEY, 'input_total')
        for pos,(coin,i) in enumerate(zip(coins,ins)):
            need(i['sequence']==0xffffffff and not i['script'] and len(i['witness'])==2, 'commit_input')
            pub=i['witness'][1]; self.point(pub)
            need(bytes.fromhex(coin['script'])==b'\x00\x14'+h160(pub), 'prevout_program')
            self.verify_der(pub,witness_hash(commit,pos,coin),i['witness'][0])
        need(len(outs)==3 and outs[0]['atoms']==500000 and outs[1]['atoms']==0 and outs[2]['atoms']>=1000, 'commit_outputs')
        need(len(outs[2]['script'])==22 and outs[2]['script'][:2]==b'\x00\x14', 'return_program')
        need(sum(c['atoms'] for c in coins)-sum(o['atoms'] for o in outs)==10000, 'commit_fee')
        locator=outs[1]['script']; need(len(locator)==76 and locator[:11]==b'\x6a\x4aSLUGFCM1\x01', 'commit_locator')
        need(spend['version']==1 and spend['locktime']==0 and len(spend['inputs'])==1, 'spend_header')
        i=spend['inputs'][0]
        need(i['hash_wire']==D(serialize(commit,False)).hex() and i['index']==0 and i['sequence']==0xffffffff and not i['witness'], 'spend_input')
        stack=pushes(i['script']); need(len(stack)>=2, 'spend_stack')
        redeem=stack[-1]
        need(len(redeem)==240 and redeem[:2]==b'\x4c\xca' and redeem[204:206]==b'\x75\x21' and redeem[-1:]==b'\xac', 'redeem_template')
        need(outs[0]['script']==b'\xa9\x14'+h160(redeem)+b'\x87', 'commitment')
        envelope, fundpub=redeem[2:204],redeem[206:239]
        need(fundpub==ins[0]['witness'][1], 'first_funding_key')
        self.verify_der(fundpub,legacy_hash(spend,redeem),stack[-2])
        need(envelope[:9]==b'SLUGFTR1\x01', 'envelope_tag')
        intent,sig=envelope[9:138],envelope[138:]
        pin,prior,recipient,funding=intent[:32],intent[32:64],intent[64:97],intent[97:]
        need(any(pin) and any(prior), 'zero_id')
        need(pin.hex()==row['expected_pin'] and prior.hex()==row['expected_prior'], 'source_or_prior')
        self.point(recipient); sender=bytes.fromhex(row['sender_gym']); point=self.point(sender)
        need(recipient!=sender, 'self_transfer')
        need(locator[11:43]==prior and locator[43:]==recipient, 'locator_intent')
        need(funding==H(FUND+tuple_bytes), 'funding_digest')
        rr,ss=int.from_bytes(sig[:32],'big'),int.from_bytes(sig[32:],'big')
        need(1<=rr<self.E.ORDER and 1<=ss<=self.E.ORDER//2, 'gym_scalar')
        need(self.E.verify(point,H(AUTH+intent),rr,ss), 'gym_signature')
        os=spend['outputs']; need(len(os) in (1,2) and os[0]==dict(atoms=490000,script=outs[2]['script']), 'spend_return')
        transfer=H(IDENT+intent)
        if len(os)==2:
            need(os[1]==dict(atoms=0,script=b'\x6a\x29SLUGFRV1\x01'+transfer), 'reveal_locator')
        need(500000-sum(o['atoms'] for o in os)==10000, 'spend_fee')
        return dict(transfer_id=transfer.hex(),is_transfer=len(os)==2,
                    commit_txid=D(serialize(commit,False))[::-1].hex(),spend_txid=D(serialize(spend,False))[::-1].hex(),
                    envelope_bytes=len(envelope),redeem_bytes=len(redeem),intent_bytes=len(intent))

    def resign(self, row, commit, spend, redeem=None):
        """Re-sign a modeled mutation so negative checks reach its semantic edge."""
        coins = row['prevouts']
        for pos, coin in enumerate(coins):
            rr,ss=self.E.sign(self.keys[(coin['fixture'],coin['path'])],witness_hash(commit,pos,coin))
            commit['inputs'][pos]['witness']=[self.E.der_signature(rr,ss)+b'\x01',self.public(coin['fixture'],coin['path'])]
        if redeem is None: redeem=pushes(spend['inputs'][0]['script'])[-1]
        spend['inputs'][0]['hash_wire']=D(serialize(commit,False)).hex()
        first=coins[0]
        rr,ss=self.E.sign(self.keys[(first['fixture'],first['path'])],legacy_hash(spend,redeem))
        spend['inputs'][0]['script']=push(self.E.der_signature(rr,ss)+b'\x01')+push(redeem)
        row['commit_raw'],row['spend_raw']=serialize(commit).hex(),serialize(spend).hex()
        return row
