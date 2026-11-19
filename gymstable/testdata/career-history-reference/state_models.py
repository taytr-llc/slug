"""Finite synthetic contract models; not scheduler, authorization or timing evidence.
Internal hash strings compare symbolic dictionaries, not production wire digests.
Refusal strings below are abstract categories except named wire mappings explicitly
marked concrete. No parser acceptance creates a trusted handle. No runtime imports.
"""
import copy,hashlib,json
K=1024
BASE=(136+200)*K
RESERVE=512*K
CAP=1024*K

def copied(v):return copy.deepcopy(v)
def fits(other):return other>=0 and BASE+RESERVE+other<=CAP

def capture_check(c,live):
    # Compare ownership, not newly recomputed totals/prefix/LastAsOf.
    for key in ['owner','source','generation','view','activation','assignment','checkpoint','roster_revision']:
        if c[key]!=live[key]:return 'identity_or_epoch_mismatch'
    if not live['healthy']:return 'dependency_unusable'
    if live['source_count']<c['source_count']:return 'source_regression'
    return 'accepted_captured_bytes'

class Lane:
    def __init__(self):
        self.active='';self.queue=None;self.selected='old';self.terminal='old'
        self.reserved=RESERVE;self.other=0;self.live=True;self.stopping=False
        self.published=[];self.proof_calls=0;self.capture_calls=0
    def begin(self,job,authorized=True):
        if not authorized or not self.live:return 'authorization_refused'
        if self.stopping or self.queue is not None or self.active=='history':return 'busy'
        if not fits(self.other):return 'capacity'
        self.queue={'id':job,'published':False,'prior':self.reserved}
        self.reserved=RESERVE
        return 'unpublished'
    def publish(self,job,authorized=True):
        if self.queue is None or self.queue['id']!=job:return 'superseded_callback'
        if not authorized or not self.live:
            prior=self.queue['prior'];self.queue=None
            self.reserved=prior if self.live else 0
            return 'authorization_refused'
        self.queue['published']=True;self.selected='';self.terminal=''
        self.published.append(job);return 'queued'
    def rollback(self,job):
        if self.queue is None or self.queue['id']!=job:return 'superseded_callback'
        self.reserved=self.queue['prior'] if self.live else 0;self.queue=None
        return 'rolled_back'
    def start(self,job):
        if self.active or self.queue is None or self.queue['id']!=job or not self.queue['published']:return 'not_runnable'
        self.active='history';self.task=job;self.queue=None;return 'running'
    def finish(self,job,outcome='done'):
        if self.active!='history' or getattr(self,'task',None)!=job:return 'superseded_callback'
        self.active='';self.terminal=job
        self.selected=job if outcome=='done' and self.live and not self.stopping else ''
        if not self.selected:self.reserved=0
        return outcome
    def stop(self):
        self.stopping=True;self.queue=None;self.active='';self.selected='';self.terminal='';self.reserved=0
        return 'joined_and_invalidated'
    def state(self):
        return dict(active=self.active,queue=copied(self.queue),selected=self.selected,terminal=self.terminal,reserved=self.reserved,other=self.other,live=self.live,stopping=self.stopping,published=list(self.published))

class Native:
    def __init__(self):self.intent='old';self.selected='old';self.terminals={};self.rows=[None]*256
    def select(self,job):self.intent=job
    def terminal(self,job,value):
        if job in self.terminals and self.terminals[job]!=value:return 'changed_terminal'
        if len(self.terminals)==3 and job not in self.terminals:return 'bounded_discovery_refusal'
        self.terminals[job]=copied(value)
        if self.intent==job:
            if self.selected!=job:self.rows=[None]*256
            self.selected=job
            return 'selected'
        return 'valid_historical_not_selected'
    def page(self,job,capture,expected_capture,rows):
        if job!=self.selected or job!=self.intent:return 'not_current_selection'
        if capture!=expected_capture:return 'whole_capture_mismatch'
        nextrows=list(self.rows)
        for ordinal,value in rows:
            if type(ordinal)is not int or not 1<=ordinal<=256:return 'ordinal_range'
            h=hashlib.sha256(json.dumps(value,sort_keys=True,separators=(',',':')).encode()).hexdigest()
            if nextrows[ordinal-1]not in (None,h):return 'changed_seen_row'
            nextrows[ordinal-1]=h
        self.rows=nextrows;return 'accepted'


def native_lookup(current,known,latest,requested_job,request_authority):
    """Native admission only: response grammar/correlation is still checked later.
    current/known/latest are trusted modeled native memory, not caller DTO imports.
    latest replaces earlier Snapshot discovery; known admitted tuples are separate.
    """
    same=lambda a,b:json.dumps(a,sort_keys=True,separators=(',',':'))==json.dumps(b,sort_keys=True,separators=(',',':'))
    if current is None or not same(current,request_authority):return 'original_authority_refused'
    if requested_job in known:
        return 'known_admitted_tuple'if same(known[requested_job],current)else 'original_authority_refused'
    if latest is None or not same(latest['authority'],current):return 'discovery_authority_refused'
    if requested_job not in latest['named_job_ids']:return 'undiscovered_job'
    return 'latest_named_requires_typed_response_validation'

def build_models():
    out=[]
    def add(name,inputs,expected,kind='abstract_refusal_categories'):
        out.append(dict(id=name,modeled_only=True,semantics=kind,input=copied(inputs),expected=copied(expected)))
    capture=dict(owner='stable-A',source='registry-A',generation=1,view='view-A',activation='activation-A',assignment='fighter-A',checkpoint=8,roster_revision=3,source_count=8,healthy=True,prefix='old-prefix',totals='old-totals',last_as_of=100)
    for key,value in [('owner','reopened-A'),('source','registry-B'),('generation',2),('view','new-login'),('activation','activation-B'),('assignment','changed'),('checkpoint',9),('roster_revision',4),('healthy',False),('source_count',7),('source_count',9),('source_count',8),('prefix','same-ms-later'),('totals','new-award'),('last_as_of',101)]:
        live=dict(capture);live[key]=value
        answer=capture_check(capture,live)
        add('capture/'+key+'/'+str(value),dict(capture=capture,live=live),dict(category=answer,returned_prefix=capture['prefix']if answer=='accepted_captured_bytes'else None))
    for descriptor in ['held','retired','off_roster']:
        live=dict(capture);live[descriptor]=True
        assert capture_check(capture,live)=='accepted_captured_bytes'
        add('capture/allowed-'+descriptor,dict(capture=capture,live=live),dict(category='accepted_captured_bytes'))
    for label,active,queued,stop,auth in [('idle','',False,False,True),('eligible-other','other',False,False,True),('active-history','history',False,False,True),('any-queued','other',True,False,True),('draining','',False,True,True),('unauthorized','',False,False,False)]:
        m=Lane();m.active=active;m.queue={'id':'existing'}if queued else None;m.stopping=stop
        result=m.begin('new',auth);add('admission/'+label,dict(active=active,queued=queued,stopping=stop,authorized=auth),dict(category=result,state=m.state()))
    m=Lane();events=[m.begin('first'),m.start('first'),m.rollback('first'),m.begin('successor'),m.rollback('first'),m.publish('first'),m.publish('successor')]
    assert m.queue['id']=='successor'and m.queue['published']and m.selected==''
    add('queue/unpublished-cannot-run-late-callback-cannot-clobber',dict(operations=['begin first','start first','rollback first','begin successor','late rollback first','late publish first','publish successor']),dict(results=events,state=m.state()))
    m=Lane();events=[m.begin('denied'),m.publish('denied',False)]
    assert m.selected=='old'and m.terminal=='old'
    add('queue/final-auth-denied-preserves-selection',dict(operations=['begin denied','deny final authorization']),dict(results=events,state=m.state()))
    m=Lane();m.begin('closed');m.live=False;result=m.rollback('closed');assert m.reserved==0
    add('queue/closed-owner-not-resurrected',dict(operations=['begin','close owner','late rollback']),dict(category=result,reserved=m.reserved))
    m=Lane();results=[m.begin('lost-ack'),m.publish('lost-ack'),m.start('lost-ack'),m.finish('lost-ack')]
    assert m.published==['lost-ack']and m.selected=='lost-ack'
    add('job/lost-ack-reconcile-no-rebegin',dict(ack_received=False,explicit_recovery=['snapshot','job']),dict(results=results,begin_count=1,state=m.state()))
    m=Lane();m.begin('stopped');m.publish('stopped');m.start('stopped');results=[m.stop(),m.finish('stopped')]
    assert m.selected==''and m.reserved==0
    add('job/stop-joins-before-close-invalidates-handle',dict(operations=['begin','publish','start','Stop','late finish']),dict(results=results,state=m.state()))
    for other in [0,176*K,176*K+1,256*K]:
        m=Lane();m.other=other;result=m.begin('capacity')
        if not fits(other):assert m.selected=='old'and m.queue is None
        add('cache/admission/'+str(other),dict(stable_and_career=BASE,history=RESERVE,other=other),dict(category=result,sum=BASE+RESERVE+other,limit=CAP,selection_preserved=result=='capacity'))
    add('cache/lazy-other-family-growth',dict(retained=CAP-1,growth=2),dict(category='capacity',original_payloads_unchanged=True))
    add('cache/no-duplicate-retained-history',dict(header_cap=8192,row_array_cap=393473,offsets=1028,owner_allowance=4096,global_index_retained=False,decoded_rows_retained=False),dict(retained=406789,cap=416*K))
    for stage in ['before_proof','after_proof','before_capture','before_final_lease','before_task_publication']:
        add('cancel/'+stage,dict(cancellation_stage=stage),dict(category='cancelled',returned_payload=False,join_proof_if_started=True))
    for metadata in ['snapshot','admission','page']:
        add('deadline/'+metadata,dict(operation=metadata,shorter_caller_deadline=True),dict(error='busy',payload=False),'concrete_wire_mapping_only')
    add('deadline/admitted-task',dict(operation='capture_task',expired=True),dict(error='history_cancelled',payload=False),'concrete_wire_mapping_only')
    for lower in ['cancellation','owner_close','deadline','dependency_unavailable']:
        add('precedence/known-unknown-over-'+lower,dict(known_observation_unknown=True,also=lower),dict(error='history_observation_unknown',payload=False),'concrete_wire_mapping_only')
    add('precedence/post-sync-known-cancellation',dict(sync_succeeded=True,with_lease_cancelled=True),dict(error='history_cancelled',payload=False),'concrete_wire_mapping_only')
    add('precedence/poison-not-cancelled',dict(learned_fatal='poison',also_cancelled=True),dict(category='fatal_dependency',handle_retained=False))
    add('precedence/opaque-error-not-cancellation',dict(foreign_error_text='cancelled'),dict(category='default_unavailable',infer_cancellation_from_text=False))
    n=Native();n.select('new');results=[n.terminal('old',dict(state='done',capture='old')),n.terminal('new',dict(state='done',capture='new')),n.terminal('new',dict(state='running',capture=None))]
    assert n.selected=='new'and results[-1]=='changed_terminal'
    add('native/late-old-terminal-and-terminal-regression',dict(operations=['select new','old terminal','new terminal','new running']),dict(results=results,selected=n.selected))
    n=Native();n.select('one');n.terminal('one',dict(state='done',capture='cap'))
    results=[n.page('one','cap','cap',[(3,{'award':'C'}),(2,{'award':'B'})]),n.page('one','cap','cap',[(2,{'award':'B'}),(1,{'award':'A'})]),n.page('one','cap','cap',[(2,{'award':'changed'})]),n.page('one','same-id-new-status','cap',[(1,{'award':'A'})])]
    assert results==['accepted','accepted','changed_seen_row','whole_capture_mismatch']
    add('native/overlapping-page-row-commitments',dict(operations=['page after0 limit2','page after1 limit2','changed ordinal2','changed Capture same HistoryID']),dict(results=results,seen=sum(x is not None for x in n.rows),fixed_slots=len(n.rows)))
    n.select('two');assert n.page('one','cap','cap',[])=='not_current_selection';n.terminal('two',dict(state='done',capture='cap2'));assert all(x is None for x in n.rows)
    add('native/new-selection-resets-after-correlation',dict(new_job='two',prior_job='one'),dict(old_page_category='not_current_selection',seen=0,fixed_slots=256))
    add('work/one-capture-many-pages',dict(explicit_capture_jobs=1,cold_source_cache_miss=True,page_requests=33,metadata_requests=12),dict(modeled_proof_calls=1,modeled_atomic_state_captures=1,modeled_engine_calls=0,modeled_rpc_calls=0,page_new_proof_calls=0,page_current_recomputations=0),'synthetic_obligation_counts_not_runtime_measurements')
    add('authority/caller-json-never-mints-token',dict(supplied='valid canonical Capture and all rows'),dict(category='untrusted_handle',no_authority_from_codec=True))
    add('result-link/current-overlay-is-separate',dict(retained_event=True,later_source_conflict=True),dict(award_retained=True,automatic_check=False,automatic_finalize=False,automatic_export=False,current_status_without_fetch='not_checked'))
    # Exact latest-Snapshot discovery is distinct from helper request structure
    # and post-admission response correlation. A historical named ID is not proof
    # that the current native process may look it up under another authority.
    authority=dict(view=dict(view_id='original',generation=1,origin='loopback-A',transport_trust='trust-A'),store_id='store-A',activation_id='activation-A')
    known={'admitted':copied(authority)}
    latest=dict(epoch=2,authority=copied(authority),named_job_ids=['newly-named'])
    discovery=[
        ('known-without-snapshot',known,None,'admitted',authority,authority,'known_admitted_tuple'),
        ('known-despite-latest-other-id',known,latest,'admitted',authority,authority,'known_admitted_tuple'),
        ('recover-latest-exact-id',{},latest,'newly-named',authority,authority,'latest_named_requires_typed_response_validation'),
        ('unknown-no-snapshot',{},None,'arbitrary',authority,authority,'discovery_authority_refused'),
        ('unknown-not-named',{},latest,'arbitrary',authority,authority,'undiscovered_job'),
        ('only-old-snapshot-named',{},latest,'old-snapshot-only',authority,authority,'undiscovered_job'),
        ('known-foreign-original-view',known,latest,'admitted',dict(authority,view=dict(authority['view'],generation=2)),authority,'original_authority_refused'),
        ('known-same-id-changed-trust',known,latest,'admitted',dict(authority,view=dict(authority['view'],transport_trust='other')),authority,'original_authority_refused'),
        ('authority-cleared',known,latest,'admitted',authority,None,'original_authority_refused'),
    ]
    for field,value in [('view',dict(authority['view'],generation=2)),('store_id','store-B'),('activation_id','activation-B')]:
        foreign=dict(latest,authority=dict(authority,**{field:value}))
        discovery.append(('foreign-snapshot-'+field,{},foreign,'newly-named',authority,authority,'discovery_authority_refused'))
    for name,k,last,job,request_auth,current,expected in discovery:
        got=native_lookup(current,k,last,job,request_auth);assert got==expected,name
        add('native-discovery/'+name,dict(current_authority=current,known_admitted_tuples=k,latest_snapshot=last,requested_job_id=job,request_authority=request_auth,previous_snapshot_only_names=['old-snapshot-only']),dict(category=got,wire_validity_alone_is_insufficient=True,accepted_lookup_still_requires_typed_response_validation=True))
    assert len({x['id']for x in out})==len(out)
    return out

if __name__=='__main__':
    import pathlib
    p=pathlib.Path(__file__).parent/'fragments/state.json'
    p.write_text(json.dumps(dict(version=1,scope='finite symbolic contract models, not runtime evidence',cases=build_models()),sort_keys=True,separators=(',',':'))+'\n')
    print(len(build_models()),hashlib.sha256(p.read_bytes()).hexdigest())
