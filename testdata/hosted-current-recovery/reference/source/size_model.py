"""Independent conservative byte composition from frozen fields and maxima.

Constructed maxima are SIZE TEMPLATES, not valid event histories/capabilities.
This proves encoded/copy reservations only, not allocator heap usage. Runtime
owned-object charging, actual overlapping lifetimes and refusal-before-work
remain implementation assertions.
"""
from common import canonical, principal_id, own, TIME_MAX, U64_MAX, U32_MAX, ISSUER


def size_report(recipe, read):
    h='f'*64; t=TIME_MAX
    key=read(recipe['paths']['rsa'])
    verifier=dict(version=1,algorithm='RS256',issuer=ISSUER,audience='slug-provider-fixture-login',
                  reset_audience='slug-provider-fixture-reset',epoch_claim='slug_auth_epoch',
                  keys=[dict(kid='rfc7515-a2',n=key['n'],e=key['e'])])
    principals=[dict(issuer=ISSUER,subject=f,fixture=f) for f in recipe['fixture_order']]
    principals.sort(key=lambda p:principal_id(p['issuer'],p['subject']))
    identity=dict(version=2,accounts=dict(version=1,principals=principals,max_sessions=4096,max_challenges=4096,max_events=16384),
                  verifier_config_hash=h,max_login_attempts=1024)
    config=dict(version=1,provider='published-fixture-v1',issuer=ISSUER,tenant_id='fixture-tenant',client_id='fixture-client',
                connection_id='fixture-database',authority_id=h,verifier_config_hash=h,max_login_flows=1024,max_reset_flows=256)
    vault=dict(version=1,wrapper_id='public-fixture-wrapper-v1',recovery_key_ref='fixture-recovery-v1',gym_key_ref='fixture-gym-v1',
               allowed_recovery_key_refs=['fixture-recovery-v1'],allowed_gym_key_refs=['fixture-gym-v1'],max_operations=128,policy=own(recipe['policy']))
    bindings=[];manifests=[]
    for fixture in recipe['fixture_order']:
        m=read(recipe['paths']['actual_manifests'][fixture])
        for a in m['accounts']:a['next_external']=a['next_internal']=4096
        manifests.append(m)
        b=dict(version=1,authority_id=h,account_id=h,principal_id=h,provision_id=h,network=m['network'],genesis=m['genesis'],
               profile=m['profile'],root_public_key=m['root_public_key'],gym_public_key=m['gym_public_key'],gym_id=m['gym_id'])
        bindings.append(dict(version=1,hosted_vault_id=h,vault_binding=b,retention_id=h,wallet_owner_id=h,source_binding_id=h))
    manifest=max(manifests,key=lambda v:len(canonical(v)))
    binding=max(bindings,key=lambda v:len(canonical(v)))
    checkpoint=dict(version=1,binding=binding,revision=128,capture_id=h,expected_revision=127,captured_at_ms=t,
                    committed_at_ms=t,manifest=manifest,manifest_sha256=h)
    current_marker=dict(application='slug-hosted-current-recovery',version=1,bindings=bindings)
    meta=dict(version=1,marker_sha256=h,actions=128,clock_high_water_ms=t,
              current=[dict(account_id=h,revision=128,capture_id=h) for unused in range(2)])
    init=dict(version=1,mode='local-hosted-fixture-pool',descriptor_id=h,authority_id=h,coordinator_id=h,hosted_vault_id=h,
              provider_driver_id='local-published-fixture-evidence-v1',wrapper_id='public-fixture-wrapper-v1',
              verifier_config_hash=h,identity_config_sha256=h,provider_config_sha256=h,vault_config_sha256=h,
              wallets=[dict(fixture=f,wallet_owner_id=h) for f in recipe['fixture_order']])
    ready=dict(version=1,init_sha256=h,authority_id=h,coordinator_id=h,hosted_vault_id=h,source_binding_id=h,
               fixtures=[dict(fixture=f,current_binding=b) for f,b in zip(recipe['fixture_order'],bindings)])
    receipt=dict(version=1,stage='fixture-incrementing256',init_sha256=h,identity_sha256=h)
    authority=dict(version=2,authority_id=h,verifier_config_hash=h,network=manifest['network'],genesis=manifest['genesis'],profile=manifest['profile'])
    vault_marker=dict(application='slug-hosted-fixture-recovery-vault',version=1,hosted_vault_id=h,authority=authority,config=vault)
    fixture='incrementing256'  # longer of the two frozen fixture/subject strings
    flow=dict(version=1,coordinator_id=h,authority_id=h,id=h,attempt_id=h,issuer=ISSUER,nonce=h,sequence=U64_MAX,
              issued_at_ms=t,expires_at_ms=t)
    auth=dict(version=1,sequence=1024,fixture=fixture,subject=fixture,auth_id=h,flow=flow,credential_epoch=U64_MAX,authenticated_at_ms=t)
    request=dict(version=1,coordinator_id=h,authority_id=h,event_id=h,nonce=h,issued_at_ms=t,expires_at_ms=t)
    barrier=dict(version=2,event_id=h,account_id=h,principal_id=h,generation=U64_MAX,from_epoch=U64_MAX-1,to_epoch=U64_MAX,
                 status='completed',verified_at_ms=t,began_at_ms=t,completed_at_ms=t,hold_until_ms=t,
                 begin_sequence=U64_MAX,complete_sequence=U64_MAX)
    complete=dict(version=1,coordinator_id=h,authority_id=h,event_id=h,provider_operation_id=h,phase='completed',can_commit=False,barrier=barrier)
    # Conservatively use the larger completed result for BOTH ack fields.
    # This is only a byte upper bound; begin_ack must actually remain pending.
    reset=dict(version=1,sequence=256,fixture=fixture,subject=fixture,account_id=h,operation_id=h,request=request,
               from_epoch=U64_MAX-1,to_epoch=U64_MAX,verified_at_ms=t,phase='completed',begin_ack=complete,complete_ack=own(complete))
    driver=dict(version=1,driver_id='local-published-fixture-evidence-v1',coordinator_id=h,config=config,revision=U32_MAX,
                clock_high_water_ms=t,epochs=[dict(fixture=f,subject=f,credential_epoch=U64_MAX,pending_event_id=h) for f in recipe['fixture_order']],
                auth=[auth]*1024,resets=[reset]*256)
    sizes={
        'public_manifest':(len(canonical(manifest)),4096,'raw'),
        'current_checkpoint':(len(canonical(checkpoint)),8192,'raw'),
        'current_meta':(len(canonical(meta)),4096,'raw'),
        'current_marker_file':(len(canonical(current_marker))+1,32768,'one LF'),
        'hosted_init_file':(len(canonical(init))+1,32768,'one LF'),
        'hosted_ready_file':(len(canonical(ready))+1,32768,'one LF'),
        'setup_receipt_file':(len(canonical(receipt))+1,8192,'one LF'),
        'hosted_vault_marker_file':(len(canonical(vault_marker))+1,32768,'one LF'),
        'fixture_auth_row':(len(canonical(auth)),2048,'raw'),
        'fixture_reset_row':(len(canonical(reset)),4096,'raw'),
        'driver_state_file':(len(canonical(driver))+1,4194304,'one LF')}
    row=len(canonical(checkpoint));meta_bytes=len(canonical(meta));key=7+64
    sizes['current_prepared_batch']=(key+row+4+meta_bytes,12288,'raw key/value composition')
    sizes['current_all128_key_values']=(128*(key+row)+4+meta_bytes,2097152,'raw key/value composition')
    result=[]
    for name,(actual,cap,framing) in sizes.items():
        if actual>cap:raise AssertionError(name+' exceeds frozen cap')
        result.append(dict(name=name,conservative_bytes=actual,cap_bytes=cap,framing=framing))
    # Separate allowances. Include both retained/staged row-cap owners and two
    # exact encoded states, rather than claiming an encoded-byte count is heap.
    driver_objects=2*(1024*2048+256*4096+8192)
    driver_encodings=2*sizes['driver_state_file'][0]
    driver_work=driver_objects+driver_encodings
    # Public mixed rows remain borrowed from the read transaction; this still
    # pessimistically charges all eight capped returned rows. No span is owned.
    recovery_components=dict(current_two_row_cap_owners=2*(128*(8192+71)+4096),
        current_two_encodings=2*sizes['current_all128_key_values'][0],
        legacy_envelopes_and_encoded_copies=4*196608,wrapped_keys_and_copies=4*8192,
        private_export_and_output_copies=4*65536,public_manifests_and_copies=4*4096,
        borrowed_mixed_row_upper_bound=8*65536,prepared_local_batch=12288)
    recovery_work=sum(recovery_components.values())
    if driver_work>16*1024*1024 or recovery_work>16*1024*1024:raise AssertionError('component allowance exceeded')
    return dict(scope='Conservative closed-schema byte/copy arithmetic; templates need not be semantically valid histories',
        record_sizes=result,
        fixed_components=dict(max_scopes_visited=5,max_accounts_per_scope_visited=3,
                              max_import_presence_probes=4,max_mixed_account_row_bytes=65536,
                              max_new_current_actions=128,max_driver_auth=1024,max_driver_resets=256,
                              wrapper_blob_nonce_bytes=12,wrapper_blob_plaintext_bytes=32,wrapper_blob_tag_bytes=16,
                              fixed_wrapper_blob_bytes=60),
        driver_reservation=dict(two_row_cap_owners=driver_objects,two_encodings=driver_encodings,
                                charged_component_bytes=driver_work,allowance_bytes=16*1024*1024),
        recovery_reservation=dict(components=recovery_components,charged_component_bytes=recovery_work,allowance_bytes=16*1024*1024),
        limitations=['No exact allocator heap claim','Runtime owner/copy lifetimes and fixed reservation charge require tests',
                     'Max-width templates intentionally overapproximate reachable states; never acceptance positives',
                     'No encrypted content, entropy, DEK, private key or live filesystem metadata is generated'])
