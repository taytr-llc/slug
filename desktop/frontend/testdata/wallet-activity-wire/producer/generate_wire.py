#!/usr/bin/env python3
"""SOURCE ONLY until independent root review and producer lock approval.

The output contains public, synthetic transport cases. Integer facts for four
actual-transaction projections come only from locked prior independent drafts;
no new wallet projection, Go implementation or core producer output is read.
"""
import argparse
import hashlib
import json
from pathlib import Path
import sys
sys.dont_write_bytecode = True
import wire_model as m

INPUT_LOCK_SHA256 = '177734937ccfeaf12c6618fbd15d757b86a9a85731b06460b9a8d3e1269c2ee0'
CONTRACT_SHA256 = '71bff8b11210aab89e324fc96d296a41e3a0248c5ae6e2293face8e51de51138'
SOURCE_NAMES = ('wire_model.py', 'generate_wire.py', 'wire-case-plan.json')
FIXTURES = ('zero256', 'incrementing256')


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def synthetic(label):
    return sha(('SLUG Set70 independent synthetic wire identifier v1:' + label).encode('ascii'))


def load_inputs(root):
    raw = (root/'input-lock.json').read_bytes()
    m.require(sha(raw) == INPUT_LOCK_SHA256, 'approved input lock exact bytes')
    lock = json.loads(raw)
    m.require(len(lock['files']) == 18, 'exact input inventory')
    values = {}
    for row in lock['files']:
        relative = Path(row['path'])
        m.require(not relative.is_absolute() and '..' not in relative.parts, 'relative locked input')
        content = (root/relative).read_bytes()
        m.require(len(content) == row['bytes'] and sha(content) == row['sha256'], 'locked input bytes: '+row['path'])
        values[row['path']] = content
    m.require(sha(values['contract/WALLET_ACTIVITY.md']) == CONTRACT_SHA256, 'frozen contract')
    return lock, values


def read_json(values, path):
    return json.loads(values[path])


def request(binding, command, job_id='', after=0, limit=0, txid='', request_id=1):
    return dict(id=request_id, version=1, command='wallet_activity_'+command,
                view_id='' if command == 'snapshot' else binding['view']['view_id'],
                job_id=job_id, after=after, limit=limit, txid=txid)


def response(req, arm=None, value=None, error=''):
    result = dict(id=req['id'], version=1, ok=not error, snapshot=None,
                  job=None, page=None, detail=None, error=error)
    if arm is not None:
        result[arm] = m.clone(value)
    return result


def capture(tip, count, height=110, at=1795121240000, older=False):
    return dict(version=1, captured_at_ms=at, wallet_height=height, wallet_tip=tip,
                node_height=height, node_tip=tip, count=count, has_older=older)


def job(binding, job_id, state, captured=None, error=''):
    return dict(version=1, job_id=job_id, state=state, binding=m.clone(binding),
                error=error, capture=m.clone(captured))


def snapshot(binding, state='idle', activity='none', active='', last='', captured=''):
    return dict(version=1, state=state, error='activity_unavailable' if binding is None else '',
                binding=m.clone(binding), activity=activity, active_job_id=active,
                last_job_id=last, capture_job_id=captured)


def page(binding, job_id, captured, items, after=0, limit=8):
    selected = items[after:after+limit]
    end = after+len(selected)
    return dict(version=1, binding=m.clone(binding), job_id=job_id, capture=m.clone(captured),
                after=after, limit=limit, next=end, has_more=end<len(items), items=m.clone(selected))


def detail(binding, job_id, captured, item, outputs):
    return dict(version=1, binding=m.clone(binding), job_id=job_id, capture=m.clone(captured),
                item=m.clone(item), outputs=m.clone(outputs))


def output(index, atoms, owned, change, address=''):
    return dict(index=index, atoms=atoms, owned=owned, change=change, address=address)


def modeled_item(label, c=0, d=0, k=0, fee=None, inputs=1, outputs=1,
                 own_inputs=0, own_outputs=1, coinbase=False, block=None, recorded=0,
                 capture_height=110):
    return dict(txid=synthetic(label), raw_sha256=synthetic(label+' raw'), recorded_at_ms=recorded,
                coinbase=coinbase, block=m.clone(block), confirmations=0 if block is None else capture_height-block['height']+1,
                received_atoms=c, spent_atoms=d, change_atoms=k, net_atoms=c-d, fee_atoms=fee,
                input_count=inputs, output_count=outputs, owned_input_count=own_inputs,
                owned_output_count=own_outputs)


def actual_projection(case, legacy, incoming):
    # Draft atoms and exact signed bytes predate Set70. Never inspect the
    # legacy RPC floating amount/fee/details fields as monetary authority.
    draft = case['draft']; signed = case['expected_signed']
    m.require(signed['raw_hex'] == legacy['hex'] and signed['txid'] == legacy['txid'], 'old independently qualified signed bytes')
    m.require(draft['recipient_output_index'] == 0 and draft['change']['output_index'] == 1, 'locked two-output case')
    m.require(len(draft['inputs']) == 1, 'locked one-input case')
    m.require(draft['total_input_atoms'] == draft['amount_atoms']+draft['change']['atoms']+draft['fee_atoms'], 'old integer fee equation')
    c = draft['amount_atoms'] if incoming else draft['change']['atoms']
    d = 0 if incoming else draft['total_input_atoms']
    item = modeled_item('temporary', c=c, d=d, k=0 if incoming else c,
                        fee=None if incoming else draft['fee_atoms'], outputs=2,
                        own_inputs=0 if incoming else 1,
                        block=dict(height=110, hash=legacy['blockhash'], time_ms=legacy['blocktime']*1000),
                        # Receiver first-seen time is absent from the old source.
                        # Zero is a modeled incoming timestamp, not sender time.
                        recorded=0 if incoming else legacy['timereceived']*1000)
    item['txid'] = signed['txid']; item['raw_sha256'] = sha(bytes.fromhex(signed['raw_hex']))
    outputs = [output(0, draft['amount_atoms'], incoming, False, draft['recipient']),
               output(1, draft['change']['atoms'], not incoming, not incoming, draft['change']['address'])]
    return item, outputs


class Corpus:
    def __init__(self, bindings, addresses):
        self.bindings = bindings
        self.allowed = list(bindings.values())
        self.addresses = addresses
        self.contexts = {}
        self.positives = []
        self.negatives = []
        self.raw_cases = []
        self.names = set()

    def context(self, name, binding, req=None, **extra):
        m.require(name not in self.contexts, 'unique context name')
        value = dict(expected_binding=m.clone(binding))
        if req is not None:
            value['request'] = m.clone(req)
        value.update(m.clone(extra)); self.contexts[name] = value
        return name

    def add(self, name, kind, value, context, scope=None, reason=''):
        m.require(name not in self.names, 'unique case name'); self.names.add(name)
        raw = m.frame(value, canonical=scope is None)
        bound = self.contexts[context]
        shape = m.accepts(raw, kind, self.allowed, self.addresses)
        complete = m.accepts(raw, kind, self.allowed, self.addresses, bound)
        if scope is None:
            m.require(shape and complete, 'positive model verdict: '+name)
        else:
            m.require(scope in ('shape', 'correlation'), 'negative scope')
            m.require(not complete and shape == (scope == 'correlation'), 'negative model verdict: '+name)
        row = dict(name=name, kind=kind, context_name=context, expected_binding=m.clone(bound['expected_binding']),
                   value=m.clone(value), frame=raw.decode('utf-8'), sha256=sha(raw))
        if scope is not None:
            row.update(scope=scope, reason=reason); self.negatives.append(row)
        else:
            self.positives.append(row)
        return row

    def mutate(self, name, baseline, path, value, scope='shape', reason=''):
        changed = m.clone(baseline['value'])
        cursor = changed
        for key in path[:-1]: cursor = cursor[key]
        cursor[path[-1]] = m.clone(value)
        return self.add(name, baseline['kind'], changed, baseline['context_name'], scope, reason)

    def raw(self, name, kind, raw, valid, context):
        m.require(name not in self.names, 'unique raw case name'); self.names.add(name)
        m.require(m.accepts(raw, kind, self.allowed, self.addresses, self.contexts[context]) == valid, 'raw model verdict: '+name)
        self.raw_cases.append(dict(name=name, kind=kind, valid=valid, context_name=context,
                                  raw_hex=raw.hex(), sha256=sha(raw)))


def build(values, lock):
    old_wire = read_json(values, 'independent/payout-wire-v1.json')
    prior = read_json(values, 'independent/wallet-withdrawal-v1.json')
    bindings = {}
    cases = {}; legacy = {}
    for fixture in FIXTURES:
        pub = read_json(values, 'public/'+fixture+'.manifest.json')
        old = old_wire['bindings'][fixture]
        m.require(pub['network'] == old['view']['authority_pin']['network'] and pub['genesis'] == old['view']['authority_pin']['genesis'], 'old manifest/source network agreement')
        bindings[fixture] = dict(view=m.clone(old['view']), source_binding_id=old['source_binding_id'],
                                 root_public_key=pub['root_public_key'], gym_public_key=pub['gym_public_key'], gym_id=pub['gym_id'])
        matches = [x for x in prior['cases'] if x['name'] == fixture+'-original-to-84']
        m.require(len(matches) == 1, 'one locked independent case')
        cases[fixture] = matches[0]
        legacy[fixture] = read_json(values, 'public/legacy-confirmed-'+fixture+'.json')
    addresses = sorted({c['draft'][key] for c in cases.values() for key in ('recipient',)} |
                       {c['draft']['change']['address'] for c in cases.values()})
    jobs = dict(old_wire['job_ids']); jobs['later_C'] = synthetic('capture C')
    a, b, c = jobs['capture_A'], jobs['refresh_B'], jobs['later_C']
    tip = lock['locked_case_selectors']['actual_tip_hash']
    m.require(all(x['blockhash'] == tip and x['confirmations'] == 1 for x in legacy.values()), 'old actual110 block facts')
    corpus = Corpus(bindings, addresses)
    rows = {}; details = {}; baselines = {}
    for fixture in FIXTURES:
        opposite = FIXTURES[1] if fixture == FIXTURES[0] else FIXTURES[0]
        outgoing, outgoing_outputs = actual_projection(cases[fixture], legacy[fixture], False)
        incoming, incoming_outputs = actual_projection(cases[opposite], legacy[opposite], True)
        rows[fixture] = sorted([outgoing, incoming], key=m.item_order)
        details[fixture] = {outgoing['txid']: outgoing_outputs, incoming['txid']: incoming_outputs}
        bind = bindings[fixture]; cap = capture(tip, 2)
        for index, command in enumerate(m.COMMANDS):
            req = request(bind, command, '' if command == 'snapshot' else a,
                          limit=8 if command == 'page' else 0,
                          txid=outgoing['txid'] if command == 'detail' else '', request_id=index+1)
            context = corpus.context(fixture+'-'+command, bind, req, capture=cap,
                                     captured_items=rows[fixture], selected_item=outgoing,
                                     known_job_ids=[a,b,c])
            added = corpus.add(fixture+'-'+command+'-request', 'request', req, context)
            if fixture == 'zero256': baselines[command+'-request'] = added
            if command == 'snapshot':
                payload = snapshot(bind, last=a, captured=a); arm='snapshot'
            elif command in ('begin', 'job', 'cancel'):
                state = dict(begin='running', job='done', cancel='cancelled')[command]
                payload=job(bind,a,state,cap if state == 'done' else None); arm='job'
            elif command == 'page':
                payload=page(bind,a,cap,rows[fixture]); arm='page'
            else:
                payload=detail(bind,a,cap,outgoing,outgoing_outputs); arm='detail'
            added = corpus.add(fixture+'-'+command+'-response', 'response', response(req,arm,payload), context)
            if fixture == 'zero256': baselines[command+'-response'] = added
        req=request(bind,'detail',a,txid=incoming['txid'],request_id=17)
        cx=corpus.context(fixture+'-incoming',bind,req,capture=cap,selected_item=incoming)
        corpus.add(fixture+'-incoming-detail','response',response(req,'detail',detail(bind,a,cap,incoming,incoming_outputs)),cx)

    bind=bindings['zero256']; cap=capture(tip,2); original_rows=rows['zero256']
    # Complete closed state/error matrices. Each row has exact native intent.
    req=request(bind,'snapshot')
    cx=corpus.context('snapshots',bind,req,known_job_ids=[a,b,c])
    snapshots = [snapshot(None,'unavailable'), snapshot(bind), snapshot(bind,last=a,captured=a),
                 snapshot(bind,'busy','capture',b,b,a), snapshot(bind,'busy','detail',a,b,a),
                 snapshot(bind,last=b,captured=a), snapshot(bind,last=b,captured=b)]
    for index,value in enumerate(snapshots):
        corpus.add('snapshot-state-'+str(index),'response',response(req,'snapshot',value),cx)
    req=request(bind,'job',b); cx=corpus.context('job-states',bind,req)
    for state,error in [('running',''),('cancelled',''),('failed','busy'),('failed','capacity'),('failed','activity_unavailable'),('done','')]:
        value=job(bind,b,state,capture(tip,0) if state=='done' else None,error)
        corpus.add('job-'+state+'-'+(error or 'empty'),'response',response(req,'job',value),cx)
    for error in m.ERRORS:
        corpus.add('envelope-'+error,'response',response(req,error=error),cx)

    # Empty/exact-end pages and full256 history with8-row pages; raw transaction
    # facts below are synthetic transport controls, not expected wallet records.
    many=[]
    for index in range(256):
        many.append(modeled_item('page-'+str(index),c=index,recorded=1000-index))
    cap256=capture(tip,256,older=True)
    for name,items,captured,after,limit in [('empty',[],capture(tip,0),0,8),
                                         ('end',original_rows,cap,2,1),
                                         ('interior',many,cap256,8,8),
                                         ('tail',many,cap256,248,8),
                                         ('full-end',many,cap256,256,8)]:
        req=request(bind,'page',a,after=after,limit=limit)
        cx=corpus.context('page-'+name,bind,req,capture=captured,captured_items=items)
        corpus.add('page-'+name+'-request','request',req,cx)
        corpus.add('page-'+name+'-response','response',response(req,'page',page(bind,a,captured,items,after,limit)),cx)
    # Both ordering domains and lexical zero txid/raw SHA remain valid.
    order_rows=[modeled_item('unmined-late',c=0,recorded=20),modeled_item('unmined-early',c=1,recorded=10),
                modeled_item('mined-new',c=1,block=dict(height=110,hash=tip,time_ms=0)),
                modeled_item('mined-old',c=1,block=dict(height=0,hash=tip,time_ms=0))]
    order_rows[0]['txid']='0'*64;order_rows[0]['raw_sha256']='0'*64
    order_cap=capture(tip,4)
    req=request(bind,'page',a,limit=8);cx=corpus.context('mixed-order',bind,req,capture=order_cap,captured_items=order_rows)
    ordered_row=corpus.add('mixed-order-page','response',response(req,'page',page(bind,a,order_cap,order_rows)),cx)
    baselines['mixed-order']=ordered_row
    # Each synthetic detail derives its summaries directly from chosen integer
    # amounts and own-index flags. No signing or script derivation is implied.
    modeled = [
        ('self-payment', modeled_item('self',c=1000,d=1141,k=600,fee=141,outputs=2,own_inputs=1,own_outputs=2),
         [output(0,400,True,False),output(1,600,True,True)]),
        ('partial-input', modeled_item('partial',c=10,d=100,k=0,inputs=2,outputs=2,own_inputs=1),
         [output(0,10,True,False),output(1,200,False,False)]),
        ('zero-fee', modeled_item('zero-fee',c=100,d=100,fee=0,own_inputs=1),[output(0,100,True,False)]),
        ('zero-atoms',modeled_item('zero-atoms'),[output(0,0,True,False)]),
        ('coinbase',modeled_item('coinbase',c=500000000,coinbase=True),[output(0,500000000,True,False)]),
        ('no-owned-output',modeled_item('external',d=100,fee=1,own_inputs=1,own_outputs=0),[output(0,99,False,False)]),
        ('maximum-money',modeled_item('max-money',c=m.MAX_MONEY),[output(0,m.MAX_MONEY,True,False)]),
        ('maximum-outputs',modeled_item('max-outputs',c=0,outputs=256,own_outputs=256),[output(i,0,True,False) for i in range(256)]),
    ]
    synthetic_details={}
    for name,item,outputs in modeled:
        captured=capture(tip,1);req=request(bind,'detail',a,txid=item['txid'])
        cx=corpus.context('modeled-'+name,bind,req,capture=captured,selected_item=item)
        synthetic_details[name]=corpus.add('modeled-'+name,'response',response(req,'detail',detail(bind,a,captured,item,outputs)),cx)
    # Lexical/safe integer edge controls have their own exact expected views.
    edge_bind=m.clone(bind);edge_bind['view']['view_id']='0'*64;edge_bind['view']['generation']=m.MAX_SAFE
    edge_req=request(edge_bind,'job',a,request_id=m.MAX_SAFE)
    edge_cap=capture(tip,1,height=m.MAX_HEIGHT,at=m.MAX_MS)
    edge_item=modeled_item('edge-height',c=1,block=dict(height=0,hash=tip,time_ms=m.MAX_MS),recorded=m.MAX_MS,capture_height=m.MAX_HEIGHT)
    cx=corpus.context('edge-view',edge_bind,edge_req)
    corpus.add('safe-id-zero-view-request','request',edge_req,cx)
    corpus.add('safe-id-zero-view-response','response',response(edge_req,'job',job(edge_bind,a,'done',edge_cap)),cx)
    edge_req=request(edge_bind,'detail',a,txid=edge_item['txid'],request_id=m.MAX_SAFE)
    cx=corpus.context('edge-confirmations',edge_bind,edge_req,capture=edge_cap,selected_item=edge_item)
    corpus.add('height-and-confirmations-boundary','response',response(edge_req,'detail',detail(edge_bind,a,edge_cap,edge_item,[output(0,1,True,False)])),cx)

    add_negatives(corpus,baselines,synthetic_details,bindings,jobs)
    add_raw_cases(corpus,baselines)
    scenarios = lifecycle_scenarios(a,b,c)
    formats=[dict(atoms=x,slug=m.format_atoms(x)) for x in (-m.MAX_MONEY,-100000141,-141,-1,0,1,141,100000000,m.MAX_MONEY)]
    provenance=dict(actual_projection_inputs=[x+'-original-to-84' for x in FIXTURES],
                    integer_authority='Locked prior independent drafts and exact signed bytes; never RPC floating amounts.',
                    incoming_time='Receiver first-seen time is not supplied by the old source. Incoming recorded_at_ms=0 is explicitly modeled; only outgoing time uses the old sender timereceived.',
                    synthetic_scope='All transport IDs, capture clocks, modeled rows and lifecycle facts are wire controls, not authentication, current wallet observations or finality.',
                    identity_scope='Only the two frozen public fixture root/gym tuples and inherited exact pin/trust are modeled; no general new key/address oracle.',
                    crypto_scope='Raw SHA256 is independently computed over locked prior bytes. No new transaction parsing/signing/storage expected output is consumed.')
    return dict(version=1,semantics='independent-wallet-activity-wire-v1',input_lock_sha256=INPUT_LOCK_SHA256,
                contract_sha256=CONTRACT_SHA256,provenance=provenance,bindings=bindings,job_ids=jobs,
                addresses=addresses,contexts=corpus.contexts,positives=corpus.positives,
                negatives=corpus.negatives,raw_cases=corpus.raw_cases,scenarios=scenarios,atom_formats=formats)


def add_negatives(corpus,base,details,bindings,jobs):
    a,b,c=jobs['capture_A'],jobs['refresh_B'],jobs['later_C']
    def mut(name,key,path,value,scope='shape',reason=''):
        return corpus.mutate(name,base[key],path,value,scope,reason)
    for name,path,value in [('id-zero',['id'],0),('id-unsafe',['id'],m.MAX_SAFE+1),('id-bool',['id'],True),
                             ('version-two',['version'],2),('unknown-command',['command'],'wallet_activity_raw'),
                             ('zero-job',['job_id'],'0'*64),('uppercase-job',['job_id'],a.upper()),
                             ('job-null',['job_id'],None),('job-tx-selector',['txid'],'1'*64),('job-after',['after'],1),('job-limit',['limit'],1)]:
        mut(name,'job-request',path,value)
    for name,path,value in [('snapshot-view',['view_id'],'1'*64),('snapshot-job',['job_id'],a),('snapshot-tx',['txid'],'1'*64),
                             ('snapshot-after',['after'],1),('snapshot-limit',['limit'],1)]:
        mut(name,'snapshot-request',path,value)
    for name,path,value in [('page-limit-zero',['limit'],0),('page-limit-nine',['limit'],9),('page-after257',['after'],257),
                             ('page-foreign-selector',['txid'],'1'*64)]:
        mut(name,'page-request',path,value)
    for name,path,value in [('detail-empty-tx',['txid'],''),('detail-upper-tx',['txid'],'A'*64),('detail-limit',['limit'],1)]:
        mut(name,'detail-request',path,value)
    for key in m.REQUEST_FIELDS:
        value=m.clone(base['job-request']['value']);del value[key]
        corpus.add('request-missing-'+key,'request',value,base['job-request']['context_name'],'shape')
    value=m.clone(base['job-request']['value']);value['rpc_method']='gettransaction'
    corpus.add('request-unknown-field','request',value,base['job-request']['context_name'],'shape')
    mut('response-wrong-id','job-response',['id'],99,'correlation')
    mut('response-version','job-response',['version'],2)
    mut('success-with-error','job-response',['error'],'busy')
    mut('failure-with-payload','job-response',['ok'],False)
    mut('success-no-arm','job-response',['job'],None)
    value=m.clone(base['job-response']['value']);value['snapshot']=m.clone(base['snapshot-response']['value']['snapshot'])
    corpus.add('success-two-arms','response',value,base['job-response']['context_name'],'shape')
    value=m.clone(base['job-response']['value']);value['job']=None;value['page']=m.clone(base['page-response']['value']['page'])
    corpus.add('wrong-command-arm','response',value,base['job-response']['context_name'],'correlation')
    for key in m.RESPONSE_FIELDS:
        value=m.clone(base['job-response']['value']);del value[key]
        corpus.add('response-missing-'+key,'response',value,base['job-response']['context_name'],'shape')
    for name,path,value,scope in [('foreign-view',['job','binding','view','view_id'],bindings['incrementing256']['view']['view_id'],'correlation'),
                                 ('foreign-generation',['job','binding','view','generation'],2,'correlation'),
                                 ('foreign-account',['job','binding','view','account_id'],'2'*64,'correlation'),
                                 ('foreign-account-authority',['job','binding','view','account_authority_id'],'2'*64,'correlation'),
                                 ('foreign-source',['job','binding','source_binding_id'],'2'*64,'correlation'),
                                 ('foreign-wallet',['job','binding'],bindings['incrementing256'],'correlation'),
                                 ('foreign-job',['job','job_id'],b,'correlation'),
                                 ('zero-response-job',['job','job_id'],'0'*64,'shape'),
                                 ('mismatched-root-gym',['job','binding','gym_id'],bindings['incrementing256']['gym_id'],'shape'),
                                 ('bad-origin',['job','binding','view','authority_origin'],'https://example.invalid','shape'),
                                 ('bad-trust',['job','binding','view','transport_trust_id'],'2'*64,'shape'),
                                 ('bad-descriptor',['job','binding','view','authority_pin','descriptor_id'],'2'*64,'shape')]:
        mut(name,'job-response',path,value,scope)
    for name,path,value in [('done-null-capture',['job','capture'],None),('done-error',['job','error'],'capacity'),
                             ('unknown-job-state',['job','state'],'succeeded'),('running-with-capture',['job','state'],'running'),
                             ('capture-zero-time',['job','capture','captured_at_ms'],0),('capture-high-time',['job','capture','captured_at_ms'],m.MAX_MS+1),
                             ('capture-tip-mismatch',['job','capture','node_tip'],'2'*64),('capture-height-mismatch',['job','capture','node_height'],111),
                             ('capture-count257',['job','capture','count'],257),('capture-older-under256',['job','capture','has_older'],True),
                             ('capture-zero-tip',['job','capture','wallet_tip'],'0'*64)]:
        mut(name,'job-response',path,value)
    for name,path,value in [('idle-active',['snapshot','active_job_id'],a),('idle-activity',['snapshot','activity'],'capture'),
                             ('available-error',['snapshot','error'],'activity_unavailable'),('idle-null-binding',['snapshot','binding'],None),
                             ('snapshot-zero-last',['snapshot','last_job_id'],'0'*64)]:
        mut(name,'snapshot-response',path,value)
    for name,path,value in [('page-wrong-next',['page','next'],1),('page-wrong-more',['page','has_more'],True),
                             ('page-null-items',['page','items'],None),('page-short-items',['page','items'],[]),
                             ('page-after-over-count',['page','after'],3),('page-zero-limit',['page','limit'],0)]:
        mut(name,'page-response',path,value)
    value=m.clone(base['page-response']['value']);value['page']['items'][1]=m.clone(value['page']['items'][0])
    corpus.add('duplicate-page-txid','response',value,base['page-response']['context_name'],'shape')
    value=m.clone(base['mixed-order']['value']);value['page']['items'].reverse()
    corpus.add('page-order-reversed','response',value,base['mixed-order']['context_name'],'shape')
    mut('page-new-capture-time','page-response',['page','capture','captured_at_ms'],1795121240001,'correlation')
    mut('page-changed-retained-record','page-response',['page','items',0,'recorded_at_ms'],1,'correlation')
    # A different otherwise valid local offset/limit must not answer old request.
    value=m.clone(base['page-response']['value']);value['page'].update(after=1,limit=1,next=2,has_more=False,items=value['page']['items'][1:])
    corpus.add('page-different-local-request','response',value,base['page-response']['context_name'],'correlation')
    for name,path,value in [('detail-null-outputs',['detail','outputs'],None),('detail-short-outputs',['detail','outputs'],[]),
                             ('detail-index-gap',['detail','outputs',1,'index'],2),('detail-negative-output',['detail','outputs',0,'atoms'],-1),
                             ('detail-change-unowned',['detail','outputs',0,'change'],True),('detail-address-long',['detail','outputs',0,'address'],'x'*91),
                             ('detail-address-invalid',['detail','outputs',0,'address'],'slug1invalid'),('detail-owned-sum',['detail','outputs',1,'atoms'],1),
                             ('detail-owned-count',['detail','outputs',0,'owned'],True),('detail-known-fee',['detail','item','fee_atoms'],0),
                             ('detail-wrong-net',['detail','item','net_atoms'],0),('detail-change-exceeds',['detail','item','change_atoms'],m.MAX_MONEY),
                             ('detail-negative-credit',['detail','item','received_atoms'],-1),('detail-too-much-credit',['detail','item','received_atoms'],m.MAX_MONEY+1),
                             ('detail-input-zero',['detail','item','input_count'],0),('detail-input257',['detail','item','input_count'],257),
                             ('detail-output-zero',['detail','item','output_count'],0),('detail-output257',['detail','item','output_count'],257),
                             ('detail-own-input-exceeds',['detail','item','owned_input_count'],2),('detail-own-output-exceeds',['detail','item','owned_output_count'],3),
                             ('detail-mined-confirmations',['detail','item','confirmations'],0),('detail-block-over-tip',['detail','item','block','height'],111),
                             ('detail-zero-block',['detail','item','block','hash'],'0'*64),('detail-unknown-fee-when-all-owned',['detail','item','fee_atoms'],None)]:
        mut(name,'detail-response',path,value)
    mut('detail-different-txid','detail-response',['detail','item','txid'],'2'*64,'correlation')
    mut('detail-different-raw-sha','detail-response',['detail','item','raw_sha256'],'2'*64,'correlation')
    corpus.mutate('partial-input-invented-fee',details['partial-input'],['detail','item','fee_atoms'],1)
    corpus.mutate('unmined-has-confirmations',details['zero-atoms'],['detail','item','confirmations'],1)
    corpus.mutate('coinbase-has-fee',details['coinbase'],['detail','item','fee_atoms'],0)
    corpus.mutate('coinbase-two-inputs',details['coinbase'],['detail','item','input_count'],2)
    corpus.mutate('zero-owned-counts',details['zero-atoms'],['detail','item','owned_output_count'],0)
    corpus.mutate('no-owned-input-has-debit',details['zero-atoms'],['detail','item','spent_atoms'],1)
    # Keep net consistent so this negative isolates the zero-ownership rule.
    value=m.clone(details['zero-atoms']['value']);value['detail']['item'].update(spent_atoms=1,net_atoms=-1)
    corpus.add('zero-own-input-consistent-net','response',value,details['zero-atoms']['context_name'],'shape')
    # Context rejects an older successful A when a lost new Begin was B even
    # when all original history/source fields remain identical.
    value=m.clone(base['job-response']['value'])
    cx=corpus.context('lost-new-begin-exact-B',bindings['zero256'],request(bindings['zero256'],'begin',b,request_id=value['id']))
    corpus.add('lost-B-cannot-adopt-old-A','response',value,cx,'correlation')


def add_raw_cases(corpus,base):
    req=base['job-request'];raw=req['frame'].encode('utf-8');cx=req['context_name']
    corpus.raw('raw-request-whitespace','request',b' \t'+raw[:-1]+b' \t\n',True,cx)
    corpus.raw('raw-request-reordered','request',m.compact(dict(reversed(list(req['value'].items()))))+b'\n',True,cx)
    corpus.raw('raw-request-duplicate-id','request',raw.replace(b'{',b'{"id":1,',1),False,cx)
    corpus.raw('raw-request-id-negative-zero','request',raw.replace(b'"id":3',b'"id":-0',1),False,cx)
    # The command enumeration places job at request id3; assert mutation
    # precondition to prevent a vacuous negative if the fixed plan changes.
    m.require(b'"id":3' in raw, 'raw baseline id')
    for label,token in [('fraction',b'3.0'),('exponent',b'3e0'),('nonfinite',b'NaN'),('unsafe',b'9007199254740992')]:
        corpus.raw('raw-request-id-'+label,'request',raw.replace(b'"id":3',b'"id":'+token,1),False,cx)
    corpus.raw('raw-request-invalid-utf8','request',raw.replace(b'wallet_activity_job',b'wallet_activity_\xff'),False,cx)
    corpus.raw('raw-request-trailing-value','request',raw[:-1]+b'{}\n',False,cx)
    corpus.raw('raw-request-depth','request',raw.replace(b'"after":0',b'"after":[[[0]]]'),False,cx)
    # Whitespace is legal within the cap; byte capacity itself is independent
    # of the canonical size of an otherwise valid exact object.
    corpus.raw('raw-request-exact1024','request',raw[:-1]+b' '*(1024-len(raw))+b'\n',True,cx)
    corpus.raw('raw-request-over1024','request',raw[:-1]+b' '*(1025-len(raw))+b'\n',False,cx)
    reply=base['job-response'];raw=reply['frame'].encode('utf-8');cx=reply['context_name']
    corpus.raw('raw-response-duplicate-nested','response',raw.replace(b'"job_id":',b'"job_id":"'+b'1'*64+b'","job_id":',1),False,cx)
    corpus.raw('raw-response-exact16384','response',raw[:-1]+b' '*(16384-len(raw))+b'\n',True,cx)
    corpus.raw('raw-response-over16384','response',raw[:-1]+b' '*(16385-len(raw))+b'\n',False,cx)
    reply=base['detail-response'];raw=reply['frame'].encode('utf-8');cx=reply['context_name']
    corpus.raw('raw-detail-exact98304','response',raw[:-1]+b' '*(98304-len(raw))+b'\n',True,cx)
    corpus.raw('raw-detail-over98304','response',raw[:-1]+b' '*(98305-len(raw))+b'\n',False,cx)
    corpus.raw('raw-detail-negative-zero-atoms','response',raw.replace(b'"fee_atoms":141',b'"fee_atoms":-0'),False,cx)
    corpus.raw('raw-detail-trailing-value','response',raw[:-1]+b'null\n',False,cx)


def lifecycle_scenarios(a,b,c):
    # This finite table deliberately describes native intent retention and
    # presentation decisions, not concurrency/network/account authorization.
    return [
        dict(name='lost-B-old-A-is-not-new-success', initial=dict(capture=a,pending=b,dispatched=True),
             steps=[dict(event='generic_snapshot',job_id=a,expected='do_not_adopt'),
                    dict(event='exact_job',job_id=b,state='running',expected='retain_A_dated'),
                    dict(event='exact_job',job_id=b,state='done',expected='replace_with_B_invalidate_A')]),
        dict(name='definitive-B-missing-allows-explicit-C',initial=dict(capture=a,pending=b,dispatched=True),
             steps=[dict(event='exact_error',job_id=b,error='activity_not_found',expected='retire_B_keep_A'),
                    dict(event='explicit_begin',job_id=c,expected='admit_C')]),
        dict(name='predispatch-busy-retires-only-never-dispatched',initial=dict(capture=a,pending=b,dispatched=False),
             steps=[dict(event='local_busy',job_id=b,expected='retire_B_keep_A'),
                    dict(event='explicit_begin',job_id=c,expected='admit_C')]),
        dict(name='later-local-busy-cannot-retire-dispatched-B',initial=dict(capture=a,pending=b,dispatched=True),
             steps=[dict(event='local_busy',job_id=b,expected='retain_exact_B_unknown'),
                    dict(event='explicit_begin',job_id=c,expected='refuse_busy')]),
        dict(name='failed-refresh-preserves-old-capture',initial=dict(capture=a,pending=b,dispatched=True),
             steps=[dict(event='exact_job',job_id=b,state='failed',error='activity_unavailable',expected='keep_A_dated_no_refresh_label')]),
        dict(name='reset-clears-original-handles',initial=dict(capture=a,pending=b,dispatched=True),
             steps=[dict(event='original_view_revoked',expected='clear_and_join'),
                    dict(event='late_reply',job_id=b,state='done',expected='discard'),
                    dict(event='replacement_view',expected='no_old_handles')])]


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--execute',action='store_true')
    parser.add_argument('--input-root',type=Path,required=True)
    parser.add_argument('--approved-producer-lock',type=Path,required=True)
    parser.add_argument('--approved-lock-sha256',required=True)
    parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args()
    m.require(args.execute,'execution requires separate root authorization')
    source_root=Path(__file__).resolve().parent
    sources={name:sha((source_root/name).read_bytes()) for name in SOURCE_NAMES}
    lock_raw=args.approved_producer_lock.read_bytes()
    m.require(sha(lock_raw)==args.approved_lock_sha256,'approved producer lock exact bytes')
    execution=json.loads(lock_raw)
    m.require(execution['input_lock_sha256']==INPUT_LOCK_SHA256,'producer/input binding')
    m.require(all(execution['sources'].get(k)==v for k,v in sources.items()),'reviewed exact source hashes')
    lock,values=load_inputs(args.input_root)
    result=build(values,lock)
    result['producer_files']=sources
    result['producer_lock_sha256']=args.approved_lock_sha256
    result['counts']={key:len(result[key]) for key in ('positives','negatives','raw_cases','scenarios','atom_formats')}
    # Input and source checks repeat before the only output write. Producer
    # invocation/runtime timestamps are intentionally absent from canonical data.
    load_inputs(args.input_root)
    m.require(sources=={name:sha((source_root/name).read_bytes()) for name in SOURCE_NAMES},'producer source unchanged')
    encoded=m.compact(result)+b'\n'
    with args.output.open('xb') as stream:
        stream.write(encoded)
    print(json.dumps(dict(output=str(args.output),bytes=len(encoded),sha256=sha(encoded),counts=result['counts']),sort_keys=True))


if __name__=='__main__':
    main()
