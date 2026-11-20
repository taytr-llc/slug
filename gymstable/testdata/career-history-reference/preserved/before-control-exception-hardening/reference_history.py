"""Finite independent history arithmetic/core-shape cases, not authority evidence."""
import importlib.util
from common import *
from history_math import *

def prior_module():
    path=ROOT/'inputs/prior-career/generate.py'
    spec=importlib.util.spec_from_file_location('pinned_prior_career',path)
    m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m

def setpath(v,path,value):
    x=v
    for k in path[:-1]:x=x[k]
    x[path[-1]]=clone(value)

def build_fragment():
    ex=examples();a=ex['activation'];real=ex['event'];at=a['activated_at_ms'];st=load('winner-status.json');ass=load('winner-assignment.json');prior=prior_module();histories=[];cases=[];dtos=[];controls=[]
    def add(name,events,asof=None,clock=None,status_value=None,assignment_value=None,provenance='modeled'):
        if asof is None:asof=max([at]+[e['effective_at_ms'] for e in events])
        if clock is None:clock=max([asof]+[e['effective_at_ms'] for e in events])
        s=status_value or st;assignment_value=assignment_value or ass
        data=derive_capture(a,events,s,assignment_value,asof,clock);p=prior.snapshot(a,events,[assignment_value['prospect_id']],asof,'current',clock)
        require(p['error']=='' and p['digest']==data['capture']['career_digest'] and p['totals'][0]['totals']==data['capture']['totals'] and p['prefix_award_ids']==data['prefix_award_ids'],'prior_equation_mismatch')
        requests=[(0,8),(len(data['rows']),8)]
        if len(data['rows'])>8:requests.extend([(8,8),(max(0,len(data['rows'])-3),8),(0,1)])
        pages=[dict(after=n,limit=l,value=derive_page(data,n,l)) for n,l in requests]
        item=dict(name=name,provenance=provenance,model_only=provenance!='genuine_49_public_facts',input=dict(activation=a,events=events,status=s,assignment=assignment_value,as_of_ms=asof,clock_ms=clock),expected=dict(capture=data['capture'],rows=data['rows'],prefix_preimage_hex=data['prefix_preimage_hex'],prefix_award_ids=data['prefix_award_ids'],pages=pages),prior_current_equations_equal=True)
        histories.append(item);return data
    empty=add('empty',[],status_value=load('provisional-status.json'),assignment_value=load('provisional-assignment.json'),provenance='genuine_49_public_facts')
    require(not load('provisional-awarded.json') and empty['capture']['totals']==load('provisional-expected_totals.json'))
    win=add('genuine_win',[real],provenance='genuine_49_public_facts')
    add('genuine_loss',[real],status_value=load('loser-status.json'),assignment_value=load('loser-assignment.json'),provenance='genuine_49_public_facts')
    require(load('winner-awarded.json') and win['capture']['totals']==load('winner-expected_totals.json'))
    require(histories[-1]['expected']['capture']['totals']==load('loser-expected_totals.json'))
    conflict=derive_event(a,load('result-conflict.json'));require(conflict==real and load('conflict-expected_totals.json')==win['capture']['totals'])
    cases.append(dict(name='retained_first_finality_survives_conflict',model_only=False,event=real,conflict_event=conflict,equal=True,limits='Re-derives the old authenticated public event; no new signature/chain verification.'))
    require(load('result-provisional.json')['finality'] is None)
    cases.append(dict(name='provisional_has_no_event',model_only=False,result_key=load('result-provisional.json')['result_key'],has_finality=False,event_count=0))
    for m in METHODS:
        for side in ('a','b') if m not in DRAWS else ('a',):
            add('method_'+m+'_'+side,[model_event(a,real,2000+METHODS.index(m)*2+(side=='b'),at+1,m,side)])
    for count in (14,15,16,255,256):add('count_'+str(count),[model_event(a,real,3000+n,at+n+1,METHODS[n%7],('a','b')[n%2]) for n in range(count)])
    add('mixed_unrelated_256',[model_event(a,real,4000+n,at+n+1,METHODS[n%7],('a','b')[n%2],n%3!=0) for n in range(256)])
    unrelated=[model_event(a,real,5000+n,at+n+1,unrelated=True) for n in range(256)]
    un=add('all_unrelated_256',unrelated);require(un['capture']['event_count']==0 and un['capture']['prefix_count']==256)
    add('future_retained_events_omitted_from_prefix',[model_event(a,real,6000+n,at+n+1) for n in range(3)],asof=at+1,clock=at+3)
    add('current_beyond_owner_clock',[real],asof=at+10,clock=at)
    # Equal-time insertion before an existing fifteenth row changes explanation,
    # never the event itself or an already retained capture.
    equal=sorted([model_event(a,real,7000+n,at+1) for n in range(16)],key=lambda e:e['award_id'])
    old=add('same_time_before_insertion',equal[1:]);new=add('same_time_after_insertion',list(reversed(equal)))
    old_by={r['event']['award_id']:r for r in old['rows']};new_by={r['event']['award_id']:r for r in new['rows']};last=equal[-1]['award_id']
    require(old_by[last]['xp_applied']==1 and new_by[last]['xp_applied']==0 and old['capture']['totals']['xp']==new['capture']['totals']['xp']==15)
    cases.append(dict(name='same_ms_capture_relative_fifteenth_xp',model_only=True,award_id=last,old_row=old_by[last],new_row=new_by[last],old_capture_immutable=True,event_unchanged=old_by[last]['event']==new_by[last]['event']))
    # HistoryID intentionally excludes mutable captured Status fields; full
    # Capture/terminal correlation, not ID alone, detects this difference.
    growth=clone(st);growth['observed_source_count']+=1;growth['caught_up']=False
    g=add('unrelated_source_growth_same_history',[real],status_value=growth)
    require(g['capture']['history_id']==win['capture']['history_id'] and g['capture']!=win['capture'])
    cases.append(dict(name='full_capture_not_history_id_only',model_only=True,history_id=g['capture']['history_id'],first_capture_sha256=sha(J(win['capture'])),second_capture_sha256=sha(J(g['capture'])),same_id=True,different_full_capture=True))
    # Authority/account IDs are generic: only the inherited fixture Identity and
    # compiled network/genesis/descriptor are intentionally restricted.
    generic=clone(st);generic['pin']['account_authority_id']=ident(88001);generic['pin']['account_id']=ident(88002);generic['pin']['source']['authority_id']=ident(88003);generic['store_id']=stable_id(generic['pin'])
    add('generic_authority_empty',[],status_value=generic)
    for name,data in ex['histories'].items():
        for kind,value in [('CareerHistoryCapture',data['capture']),('CareerHistoryPage',derive_page(data,0,8))]:
            valid=validate_core(kind,value);dtos.append(dict(name=name+'_'+kind,kind=kind,value=valid,canonical=J(valid).decode(),sha256=sha(J(valid))))
    def control(name,kind,base,edits=None,context=None,accept=False):
        value=clone(base)
        for path,val in edits or []:setpath(value,path,val)
        ctx=context or {};actual=True
        try:validate_core(kind,value,**ctx)
        except (Refusal,KeyError,TypeError,ValueError):actual=False
        require(actual==accept,'control_'+name)
        controls.append(dict(name=name,kind=kind,value=value,context=ctx,accept=accept,input_sha256=sha(J(value))))
    cap=ex['captures']['capacity256'];first=ex['pages']['capacity256'];late=ex['pages']['capacity256_late'];row=first['items'][0]
    for field,val in [('version',True),('prefix_count',257),('event_count',257),('as_of_ms',False),('history_id',ident(123))]:control('capture_'+field,'CareerHistoryCapture',cap,[([field],val)])
    for field,val in [('ordinal',0),('xp_before',14),('xp_applied',1),('earned',dict(wins=0,losses=0,draws=0,xp=0,base_points=0))]:control('row_'+field,'CareerHistoryRow',row,[([field],val)],{'capture':cap})
    for field,val in [('award_id',ident(123)),('winner',ident(123)),('effective_at_ms',at-1),('saturday','2026-12-06'),('method','tko'),('version',True)]:control('event_'+field,'Event',real,[([field],val)],{'activation':a})
    control('row_other_account','CareerHistoryRow',row,[(['event','corner_a','account_id'],ident(700))],{'capture':cap})
    control('late_page_totals_are_not_global','CareerHistoryPage',late,context={'limit':8},accept=True)
    for name,value in [('null_items',None),('empty_nonterminal',[]),('short_page',first['items'][:1]),('reversed',list(reversed(first['items']))),('duplicate',[first['items'][0]]*8)]:control(name,'CareerHistoryPage',first,[(['items'],value)],{'limit':8})
    for field,val in [('after',257),('next',7),('has_more',False),('version',True)]:control('page_'+field,'CareerHistoryPage',first,[([field],val)],{'limit':8})
    # Coherently short page still refuses when correlated with requested limit8.
    short=clone(first);short['items']=short['items'][:1];short['next']=1
    control('coherent_short_requested8','CareerHistoryPage',short,context={'limit':8})
    control('coherent_short_requested1','CareerHistoryPage',short,context={'limit':1},accept=True)
    duplicate=clone(first);duplicate['items'][1]['event']['result_key']=duplicate['items'][0]['event']['result_key'];duplicate['items'][1]['event']['award_id']=duplicate['items'][0]['event']['award_id']
    control('coherent_duplicate_result_key_and_award','CareerHistoryPage',duplicate,context={'limit':8})
    control('ordinal_not_display_offset','CareerHistoryPage',first,[(['items',0,'ordinal'],1)],{'limit':8})
    control('newest_totals_mismatch','CareerHistoryPage',first,[(['items',0,'totals_after','base_points'],row['totals_after']['base_points']-4)],{'limit':8})
    for lim in (0,9,2**32-1):control('request_limit_'+str(lim),'CareerHistoryPage',first,context={'limit':lim})
    controls.append(dict(name='strict_negative_zero',kind='lexical',input='{"n":-0}',accept=False))
    try:strict_parse(b'{"n":-0}',1023,1);raise AssertionError('negative zero accepted')
    except Refusal:pass
    bad_indexes=[]
    base=[model_event(a,real,9000+n,at+n+1) for n in range(257)]
    for name,events,clock in [('capacity257',base,at+257),('duplicate_event',base[:1]*2,at+257),('event_beyond_clock',base[:1],at),('malformed_unrelated_after_asof',base[:1]+[dict(base[1],winner=ident(123))],at+257)]:
        ok=True
        try:derive_capture(a,events,st,ass,at,clock)
        except Refusal:ok=False
        require(not ok,'bad_index_'+name)
        bad_indexes.append(dict(name=name,events=events,as_of_ms=at,clock_ms=clock,accept=False))
    # Retention arithmetic is byte accounting, not a live heap/process benchmark.
    largest=max(histories,key=lambda h:len(J(h['expected']['rows'])))['expected'];rows=largest['rows'];canonical_rows=J(rows);capture_bytes=J(largest['capture']);retained=len(canonical_rows)+len(capture_bytes)+257*4+4096
    require(retained<=416*1024)
    return dict(version=1,kind='history',scope='Independent equations/finite strict shape expectations; historical genuine facts and additional models are separately labelled.',histories=histories,cases=cases,dtos=dtos,controls=controls,index_controls=bad_indexes,bounds=dict(max_actual_row_bytes=max(len(J(r)) for h in histories for r in h['expected']['rows']),modeled256_rows_bytes=len(canonical_rows),capture_bytes=len(capture_bytes),retained_accounted_bytes=retained,retained_cap=416*1024,not_heap_measurement=True),prior_equation_source_sha256=sha((ROOT/'inputs/prior-career/generate.py').read_bytes()))

if __name__=='__main__':
    import json
    value=build_fragment();out=ROOT/'fragments/history.json';out.parent.mkdir(exist_ok=True);out.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n');print(json.dumps({'histories':len(value['histories']),'controls':len(value['controls']),'sha256':sha(out.read_bytes()),'bytes':out.stat().st_size}))
