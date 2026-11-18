#!/usr/bin/env python3
"""Verify only independent Python and retained inputs. Never import Go/runtime."""
import datetime
import io
import json
import pathlib
import sys
import time
import zoneinfo
from common import *
import schema as S
from reference_bouts import materialize, calendar_cases, generate
from reference_state import scenarios, initialization


def verify(path):
    raw_corpus=path.read_bytes();v=json.loads(raw_corpus);start=time.monotonic()
    assert v['core_contract_sha256']==CORE and v['helper_contract_sha256']==HELPER
    assert v['input_index_sha256']==sha((HERE/'input_index.json').read_bytes())
    assert v['dependency_freeze_sha256']==INDEX['inputs/contracts/set47-contract-dependency-freeze.json']['sha256']
    assert v['final_dependency_source_sha256']==INDEX['inputs/contracts/set47-final-dependency-source.json']['sha256']
    assert v['candidate_only'] and not v['expected_values_from_Go'] and not v['new47_runtime_read']
    assert len(v['compositions'])==11 and len(v['calendar_cases'])==3861
    for i,f in enumerate(v['compositions']):
        assert J(f['source']).decode()==f['canonical_source']
        assert J(derive_input(f['source'])).decode()==f['canonical_input']
        assert ENGINE.input_id(f['input'])==f['input_id']
        assert ENGINE.simulate(f['input'])==f['result']
        assert J(f['result']).decode()==f['canonical_result']
        assert ENGINE.digest(f['result'])==f['result_digest']
        assert J(f['record']).decode()==f['canonical_record']
        assert record_digest(f['record'])==f['record_digest']
        assert f['input']['as_of_ms']==f['source']['event']['cutoff_ms']
        assert f['input']['seed']==f['source']['bout']['seed']
        assert all(f['input'][side]['pro_xp']==f['input'][side]['training_skill']==0 for side in ['a','b'])
        if i>=6:
            assert all(condition(1,f['input'][side]['manifest']['dna'],f['input'][side]['born_at_ms'],f['run_at_ms'],0)['phase']=='retired' for side in ['a','b'])
    for i,d in enumerate(v['dtos']):
        try:
            got=S.parse(d['kind'],d['canonical'].encode(),v['compositions'][d['fixture_index']]['pin'],d['context'])
            assert J(got).decode()==d['canonical']
            assert sha(J(got))==d['sha256']
        except Exception as exc:raise AssertionError(('DTO',i,d['kind'],d['name'],repr(exc)))from exc
    for i,c in enumerate(v['codec_cases']):
        b=materialize(v['dtos'][c['base']]['value'],c['recipe'])
        assert sha(b)==c['input_sha256'] and len(b)==c['input_bytes']
        try:
            got=S.parse(c['kind'],b,v['compositions'][c['fixture_index']]['pin'],c['context']);ok=True
        except S.Refusal:ok=False
        assert ok==c['accept'],('codec',i,c['name'],ok,c['accept'])
        if ok:
            if c['canonical_ref']is not None:assert J(got).decode()==v['dtos'][c['canonical_ref']]['canonical'],('canonical',i,c['name'])
            elif c.get('canonical')is not None:assert J(got).decode()==c['canonical'],('canonical',i,c['name'])
    for i,c in enumerate(v['protocol_cases']):
        try:
            S.parse('BoutRequest',J(c['request']),c['authority_pin'],'transport')
            reply=S.parse('BoutResponse',J(c['response']),c['authority_pin'],'transport')
            S.correlate(c['request'],reply,c['authority_pin'],c['current_view'],c['source_created_at_ms'],c['created_at_ms'],c['expected_job'],c['terminal_digest']);ok=True
        except S.Refusal:ok=False
        assert ok==c['accept'],('protocol',i,c['name'],ok,c['accept'])
    for i,c in enumerate(v['portrait_cases']):
        try:
            selected=v['compositions'][c['selected_record_fixture']]['record']if c['selected_record_fixture']is not None else None
            reply=S.parse('BoutPortraitResponse',J(c['response']),v['compositions'][c['fixture_index']]['pin'],'transport')
            S.correlate_portrait(reply,v['compositions'][c['fixture_index']]['pin'],c['current_view'],selected,c['source_created_at_ms'],c['created_at_ms']);ok=True
        except S.Refusal:ok=False
        assert ok==c['accept'],('portrait',i,c['name'],ok,c['accept'])
    # Prove the transport path cannot silently call the full engine or generators.
    def forbidden(*args,**kwargs):raise AssertionError('forbidden expensive transport dependency')
    sim,gen,cond=ENGINE.simulate,MANIFEST.generate,S.condition
    ENGINE.simulate=MANIFEST.generate=S.condition=forbidden
    try:
        for d in v['dtos']:
            S.parse(d['kind'],d['canonical'].encode(),v['compositions'][d['fixture_index']]['pin'],'transport')
        for c in v['protocol_cases']:
            try:S.correlate(c['request'],c['response'],c['authority_pin'],c['current_view'],c['source_created_at_ms'],c['created_at_ms'],c['expected_job'],c['terminal_digest'])
            except S.Refusal:pass
        for c in v['portrait_cases']:
            selected=v['compositions'][c['selected_record_fixture']]['record']if c['selected_record_fixture']is not None else None
            try:S.correlate_portrait(c['response'],v['compositions'][c['fixture_index']]['pin'],c['current_view'],selected,c['source_created_at_ms'],c['created_at_ms'])
            except S.Refusal:pass
    finally:ENGINE.simulate,MANIFEST.generate,S.condition=sim,gen,cond
    assert v['state_cases']==scenarios(v['compositions'][1])
    assert v['initialization_cases']==initialization()
    assert v['calendar_cases']==calendar_cases()
    # Independent library implementation reads the exact pinned TZif, never host TZ data.
    tz=zoneinfo.ZoneInfo.from_file(io.BytesIO((HERE/'inputs/calendar/America_Los_Angeles.tzif').read_bytes()),key='pinned/LA')
    gaps=set()
    for row in v['calendar_cases']:
        y,m,d=map(int,row['saturday'].split('-'));dt=datetime.datetime(y,m,d,12,tzinfo=tz)
        nxt=datetime.datetime.combine(dt.date()+datetime.timedelta(days=7),datetime.time(12),tzinfo=tz)
        assert dt.weekday()==5 and int(dt.timestamp()*1000)==row['cutoff_ms']
        assert int(nxt.timestamp()*1000)==row['incident_deadline_ms']
        gaps.add(row['hours_to_next'])
    assert gaps=={167,168,169}
    for date in v['calendar_refusals']:
        try:cutoff(date)
        except(ValueError,AssertionError):pass
        else:raise AssertionError(('calendar must refuse',date))
    for f,c in zip(v['compositions'],v['identity_cases']):
        assert source_digest(c['changed_evidence_source'])==c['changed_source_digest']!=c['source_digest']
        assert ENGINE.input_id(derive_input(c['changed_evidence_source']))==c['changed_input_id']==f['input_id']
        assert c['later_creation_record_digest']!=c['record_digest']
    # Frozen engine controls are reused, not searched or regenerated as new expectations.
    legacy=[]
    for n in [0,2,5,6,52,53,54]:
        old=load('inputs/engine42/case-%02d.json'%n)
        inp=json.loads(old['canonical_input']);res=ENGINE.simulate(inp)
        assert ENGINE.input_id(inp)==old['input_id'] and J(res).decode()==old['canonical_result']
        assert ENGINE.digest(res)==old['result_digest'];legacy.append(res['method'])
    for name,metadata in INDEX.items():
        assert sha((HERE/name).read_bytes())==metadata['sha256'],name
    actual=v['actual_source_context']
    assert actual['original_status']==load('inputs/actual46/initial_status.json')
    assert actual['retained_conflict_status']==load('inputs/actual46/conflict_status.json')
    assert not actual['new_run_authorized_by_reference'] and actual['bout_journal_times_are_modeled']
    return dict(status='passed',corpus_sha256=sha(raw_corpus),input_index_sha256=sha((HERE/'input_index.json').read_bytes()),
                counts={k:len(v[k])for k in ['compositions','dtos','codec_cases','protocol_cases','portrait_cases','calendar_cases','calendar_refusals','identity_cases','state_cases','initialization_cases']},
                genuine_source_compositions=1,modeled_source_compositions=10,legacy_engine_control_methods=sorted(set(legacy)),
                transport_expensive_dependencies_trapped=True,pinned_TZif_independent_library_crosscheck=True,
                all_inputs_unchanged=True,Go_runtime_or_outputs_used=False,seconds=round(time.monotonic()-start,6))


if __name__=='__main__':
    path=HERE/'bout_v1.json'
    if '--regenerate'in sys.argv:
        regenerated=pretty(generate());assert regenerated==path.read_bytes(),'independent regeneration differs'
    result=verify(path)
    result['regeneration_byte_identical']='--regenerate'in sys.argv
    result['python_version']=sys.version
    print(json.dumps(result,indent=2))
