"""Appendix A ordered byte constructors. No marker can grant a capability."""
from common import FIXTURES, MODE, DRIVER, WRAPPER, canonical, need, packed, sha, own, exact


def validate_pinned(value,expected,cap):
    """Exact expected setup identity, supplied independently of encoded marker."""
    ordered=exact(value,tuple(expected))
    need(canonical(ordered)==canonical(expected),'conflict')
    packed(ordered,cap,True)
    return own(ordered)


def init_record(ids,descriptor,configs,verifier):
    identity,provider,vault=configs
    value=dict(version=1,mode=MODE,descriptor_id=descriptor,authority_id=ids['authority'],
               coordinator_id=ids['coordinator'],hosted_vault_id=ids['vault'],provider_driver_id=DRIVER,
               wrapper_id=WRAPPER,verifier_config_hash=verifier,identity_config_sha256=sha(canonical(identity)),
               provider_config_sha256=sha(canonical(provider)),vault_config_sha256=sha(canonical(vault)),
               wallets=[dict(fixture=f,wallet_owner_id=ids['wallets'][f]) for f in FIXTURES])
    packed(value,32768,True)
    return value


def ready_record(init,bindings):
    value=dict(version=1,init_sha256=sha(canonical(init)),authority_id=init['authority_id'],
               coordinator_id=init['coordinator_id'],hosted_vault_id=init['hosted_vault_id'],
               source_binding_id=bindings[0]['source_binding_id'],
               fixtures=[dict(fixture=f,current_binding=own(b)) for f,b in zip(FIXTURES,bindings)])
    need(bindings[0]['source_binding_id']==bindings[1]['source_binding_id'])
    packed(value,32768,True)
    return value


def stage_receipts(init,bindings):
    marker=dict(application='slug-hosted-current-recovery',version=1,bindings=own(bindings))
    preimages=[
      ('authority',dict(authority_id=init['authority_id'],identity_config_sha256=init['identity_config_sha256'])),
      ('coordinator',dict(coordinator_id=init['coordinator_id'],provider_config_sha256=init['provider_config_sha256'])),
      ('driver',dict(provider_driver_id=init['provider_driver_id'],coordinator_id=init['coordinator_id'],provider_config_sha256=init['provider_config_sha256'])),
      ('vault',dict(hosted_vault_id=init['hosted_vault_id'],authority_id=init['authority_id'],vault_config_sha256=init['vault_config_sha256'])),
      ('runtime',dict(source_binding_id=bindings[0]['source_binding_id'],descriptor_id=init['descriptor_id'],authority_id=init['authority_id'])),
      ('fixture-zero256',own(bindings[0])),('fixture-incrementing256',own(bindings[1])),
      ('current',dict(version=marker['version'],bindings=own(bindings)))]
    rows=[]
    for stage,preimage in preimages:
        value=dict(version=1,stage=stage,init_sha256=sha(canonical(init)),identity_sha256=sha(canonical(preimage)))
        packed(value,8192,True)
        rows.append(dict(stage=stage,preimage=preimage,preimage_utf8=canonical(preimage).decode(),receipt=value,receipt_file_utf8=packed(value,8192,True).decode()))
    return rows
