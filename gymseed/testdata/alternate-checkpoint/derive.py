#!/usr/bin/env python3
"""Test-only alternate Ed25519 checkpoint signature, public fixture key only.
Uses the frozen Python point arithmetic, never Go output. Not an oracle amendment.
"""
import hashlib, importlib.util, json, pathlib, sys
sys.dont_write_bytecode=True
ROOT=pathlib.Path('/Users/taytrrip/slug')
REF=ROOT/'set45-seed-reference'
OUT=pathlib.Path(__file__).resolve().parent
H=lambda b:hashlib.sha256(b).hexdigest()
expected={'crypto.py':'ea87b9021e14712b1ac7130daa73cd12fb8acf2fd8995ca5ce9e32d0d770fd1c', 'seed_v1.json':'2b8d78e8ef665e372543eb87bb5d25a975026a0ae946b15a871ac97bbf2153f0'}
# Pin the frozen independent arithmetic and original corpus before importing.
crypto_bytes=(REF/'crypto.py').read_bytes()
assert H(crypto_bytes)==expected['crypto.py']
assert H((REF/'seed_v1.json').read_bytes())==expected['seed_v1.json']
spec=importlib.util.spec_from_file_location('frozen_crypto',REF/'crypto.py')
c=importlib.util.module_from_spec(spec);spec.loader.exec_module(c)
f=json.loads((REF/'seed_v1.json').read_bytes())
seed=bytes.fromhex(f['fixture_keys']['checkpoint_seed']);pub=bytes.fromhex(f['fixture_keys']['checkpoint_public'])
cp=f['crypto']['checkpoint'];message=bytes.fromhex(f['crypto']['checkpoint_preimage'])
pre=b'SLUG signed checkpoint\0'+c.u32(cp['version'])+c.u32(cp['network'])+bytes.fromhex(cp['genesis'])[::-1]+c.u32(cp['height'])+bytes.fromhex(cp['hash'])[::-1]
assert message==pre and pub==c.ed_public(seed)
original=bytes.fromhex(cp['signature']);assert original==c.ed_sign(seed,message) and c.ed_verify(pub,message,original)
a,prefix=c.ed_parts(seed)
nonce_domain=b'SLUG independent alternate checkpoint regression\0v1\0'
r=int.from_bytes(hashlib.sha512(nonce_domain+prefix+message).digest(),'little')%c.L
assert r!=0
R=c.ed_enc(c.ed_mul(c.ED_BASE,r));k=int.from_bytes(hashlib.sha512(R+pub+message).digest(),'little')%c.L
alternate=R+((r+k*a)%c.L).to_bytes(32,'little')
assert alternate!=original and c.ed_verify(pub,message,alternate)
assert not c.ed_verify(pub,message+b'x',alternate)
other=c.ed_public(hashlib.sha256(b'SLUG unrelated public regression fixture').digest())
assert not c.ed_verify(other,message,alternate)
alt=dict(cp,signature=alternate.hex())
value={'version':1,'scope':'Test-only public fixture checkpoint signature; independent of Go; original frozen oracle remains unchanged. This is a valid alternate signature, not RFC8032 deterministic signing output.','input_sha256':{str(REF/'crypto.py'):H(crypto_bytes),str(REF/'seed_v1.json'):H((REF/'seed_v1.json').read_bytes()),str(REF/'freeze.json'):H((REF/'freeze.json').read_bytes())},'script_sha256':H(pathlib.Path(__file__).read_bytes()),'public_key':pub.hex(),'published_fixture_seed':seed.hex(),'message_preimage':message.hex(),'message_sha256':H(message),'nonce_derivation':'LE_integer(SHA512(nonce_domain || RFC8032_private_prefix || message)) mod L','nonce_domain_hex':nonce_domain.hex(),'nonce_scalar_le_hex':r.to_bytes(32,'little').hex(),'original_checkpoint':cp,'alternate_checkpoint':alt,'checks':{'original_valid':True,'alternate_valid':True,'signatures_distinct':True,'changed_message_refused':True,'wrong_key_refused':True}}
data=(json.dumps(value,indent=2)+'\n').encode()
p=OUT/'alternate-checkpoint.json'
if '--check' in sys.argv:assert p.read_bytes()==data
else:p.write_bytes(data)
print(json.dumps({'status':'passed','output':str(p),'sha256':H(data),'script_sha256':H(pathlib.Path(__file__).read_bytes())}))
