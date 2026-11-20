"""Finite deterministic public X.509 fixture, independently encoded and signed.

Not a production certificate generator. Seeds are published ASCII test recipes;
no random input, OS custody or real/private file is accepted. DER objects follow
RFC5280/RFC8410 fixed Ed25519 algorithms; the Go consumer separately parses them.
"""
import base64
import hashlib
import ipaddress


def length(n):
    if n < 128: return bytes([n])
    b = n.to_bytes((n.bit_length()+7)//8, 'big')
    return bytes([128+len(b)])+b


def obj(tag, body): return bytes([tag])+length(len(body))+body

def seq(*items): return obj(0x30, b''.join(items))

def integer(n):
    b = n.to_bytes(max(1, (n.bit_length()+7)//8), 'big')
    if b[0] & 128: b = b'\0'+b
    return obj(2, b)


def oid(text):
    nums = [int(x) for x in text.split('.')]
    out = bytearray([40*nums[0]+nums[1]])
    for n in nums[2:]:
        b = [n & 127]; n >>= 7
        while n:
            b.insert(0, 128+(n & 127)); n >>= 7
        out.extend(b)
    return obj(6, bytes(out))


def bits(value, unused=0): return obj(3, bytes([unused])+value)

def name(cn): return seq(obj(0x31, seq(oid('2.5.4.3'), obj(12, cn.encode('ascii')))))

def ext(number, value, critical=False):
    return seq(oid(number), *([obj(1, b'\xff')] if critical else []), obj(4, value))


def pem(der):
    text = base64.b64encode(der).decode('ascii')
    return ('-----BEGIN CERTIFICATE-----\n'+'\n'.join(text[i:i+64] for i in range(0,len(text),64))+'\n-----END CERTIFICATE-----\n').encode('ascii')


def build(E, recipe):
    ca_seed = hashlib.sha256(recipe['ca_seed_ascii'].encode('ascii')).digest()
    leaf_seed = hashlib.sha256(recipe['leaf_seed_ascii'].encode('ascii')).digest()
    ca_public, leaf_public = E.ed_public(ca_seed), E.ed_public(leaf_seed)
    algorithm = seq(oid('1.3.101.112'))
    ca_name = name('SLUG published synthetic provisioning CA')
    leaf_name = name('SLUG published synthetic provisioning server')
    validity = seq(obj(0x17,b'000101000000Z'), obj(0x18,b'21000101000000Z'))
    def cert(serial, issuer, subject, public, extensions, seed):
        spki = seq(algorithm, bits(public))
        tbs = seq(obj(0xa0, integer(2)), integer(serial), algorithm, issuer, validity, subject, spki, obj(0xa3, seq(*extensions)))
        signature = E.ed_sign(seed,tbs)
        assert E.ed_verify(E.ed_public(seed),tbs,signature)
        return seq(tbs,algorithm,bits(signature)),spki
    ca, _ = cert(74,ca_name,ca_name,ca_public,[ext('2.5.29.19',seq(obj(1,b'\xff')),True),ext('2.5.29.15',bits(b'\x04',2),True)],ca_seed)
    san = seq(obj(0x82,recipe['dns_name'].encode('ascii')),obj(0x87,ipaddress.ip_address(recipe['ip']).packed))
    leaf,spki = cert(75,ca_name,leaf_name,leaf_public,[ext('2.5.29.19',seq(),True),ext('2.5.29.15',bits(b'\x80',7),True),ext('2.5.29.37',seq(oid('1.3.6.1.5.5.7.3.1'))),ext('2.5.29.17',san)],ca_seed)
    chain = pem(leaf)+pem(ca)
    projection = dict(version=1,certificate_sha256=hashlib.sha256(chain).hexdigest(),certificate_spki=hashlib.sha256(spki).hexdigest())
    return dict(certificate=projection,chain_pem=chain.decode('ascii'),ca_pem=pem(ca).decode('ascii'),leaf_der_hex=leaf.hex(),leaf_spki_der_hex=spki.hex(),leaf_public_key=leaf_public.hex(),dns_name=recipe['dns_name'],ip=recipe['ip'],not_before_ms=946684800000,not_after_ms=4102444800000)
