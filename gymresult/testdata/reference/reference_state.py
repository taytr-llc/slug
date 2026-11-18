"""Finite Set48 helper lifecycle/phase expectations from the frozen contracts.

These are specification models, NOT executed account leases, processes, disk
writes, networking, proof verification or latency measurements. They invoke no
Go code, engine, crypto signer or private input. Actual source/chain models and
crypto expectations belong to reference_results/crypto_result.
"""
import copy
import json
from pathlib import Path

VERSION = 1
SCOPE = 'finite_helper_model_only_no_actual_auth_process_sync_network_or_engine'
TOTAL_CACHE = 1048576
RESULT_CACHE = 294912
RESULT_JOB = 245760
RESULT_METADATA = 49152
PROOF_BYTES = 25165824


def clone(v):
    return copy.deepcopy(v)


def require(ok, why='invalid modeled input'):
    if not ok:
        raise ValueError(why)


class PublicationFlow:
    """One admitted action, with explicit modeled fault boundaries.

    Acknowledged Prepare is distinguished from the possibly sent boundary.
    Error inputs here describe known phase boundaries; they are not imported
    trust verdicts in the proposed application API.
    """
    def __init__(self, initial=None):
        initial = initial or {}
        self.prepared = initial.get('prepared', False)
        self.publication = initial.get('publication', 'none')
        self.included = initial.get('included', False)
        self.finalized = initial.get('finalized', False)
        self.dispatch_count = initial.get('dispatch_count', 1 if self.publication in ('dispatch_unknown','submitted') else 0)
        require(type(self.dispatch_count) is int and 0 <= self.dispatch_count <= 4294967295)
        require(type(self.prepared) is bool and type(self.included) is bool and type(self.finalized) is bool)
        require(self.publication in ('none', 'built', 'dispatch_unknown', 'submitted'))
        require((self.dispatch_count==0)==(self.publication in ('none','built')))
        require(self.publication=='none' or self.prepared)
        require(not self.included or self.publication != 'none')
        require(not self.finalized or self.included)

    def call(self, action, **kw):
        require(action in ('publish', 'check', 'finalize', 'export', 'status', 'record', 'poll'))
        out = dict(action=action, error='', payload='none', prepare_calls=0,
                   publish_calls=0, reconcile_calls=0, attest_calls=0,
                   export_calls=0, statement_signatures=0,
                   checkpoint_signatures=0, submit_calls=0, engine_calls=0,
                   node_readiness_requested=False, automatic_funding=False,
                   automatic_mining=False, possible_dispatch=False,
                   possible_file_publication=False, publication_known=True, preparation='acknowledged' if self.prepared else 'none')

        def done(error='', payload='none'):
            out.update(error=error, payload=payload, prepared=self.prepared,
                       publication=self.publication, included=self.included,
                       finalized=self.finalized, dispatch_count=self.dispatch_count)
            return out

        if not kw.get('authorized', True):
            return done('unauthorized')
        if not kw.get('original_view_same', True):
            return done('stale_view')
        if not kw.get('source_same', True):
            return done('result_source_changed')
        if kw.get('cancel_before', False):
            return done('result_cancelled')
        if action in ('status', 'record', 'poll'):
            if action == 'record' and not self.prepared:
                return done('result_not_found')
            if kw.get('observation_unknown', False) and action != 'poll':
                return done('outcome_unknown')
            return done(payload='cached_job' if action == 'poll' else action)
        if action in ('publish', 'finalize'):
            if not kw.get('publisher_account', True) or kw.get('held', False):
                return done('forbidden')
            if not kw.get('fresh', True) or not kw.get('parent_published', True):
                return done('unauthorized')
            if not kw.get('saved', True):
                return done('result_source_incomplete')
            if not kw.get('source_healthy', True):
                return done('result_source_conflict')
        if action == 'publish':
            out['prepare_calls'] = 1
            if kw.get('prepare_unknown', False):
                out['preparation'] = 'unknown'
                return done('outcome_unknown')
            if kw.get('prepare_error', ''):
                return done(kw['prepare_error'])
            if not self.prepared:
                self.prepared = True
                out['statement_signatures'] = 1
            out['preparation'] = 'acknowledged'
            if kw.get('cancel_after_prepare', False):
                # The preparation was acknowledged; no publish invocation ran.
                return done('result_cancelled')
        elif not self.prepared:
            return done('result_proof_unavailable' if action == 'export' else 'result_not_prepared')
        if action == 'export' and (self.publication == 'none' or not self.included):
            return done('result_proof_unavailable')
        if not kw.get('publisher_account', True):
            # Other gym View can read retained local facts, not borrow zero's pair.
            return done('result_node_offline')
        out['node_readiness_requested'] = True
        if kw.get('node_offline', False):
            return done('result_node_offline')

        if action == 'check':
            out['reconcile_calls'] = 1
            if kw.get('observation_unknown', False):
                return done('outcome_unknown')
            if kw.get('reconcile_error', ''):
                return done(kw['reconcile_error'])
            return done(payload='action_result')
        if action == 'finalize':
            out['attest_calls'] = 1
            if not self.included or kw.get('descendants', 60) < 60:
                return done('result_not_mature')
            if not kw.get('captured_tip_still_member', True):
                return done('result_unavailable')
            if not self.finalized:
                out['checkpoint_signatures'] = 1
                if kw.get('checkpoint_sync_unknown', False):
                    return done('outcome_unknown')
                self.finalized = True
            if kw.get('after_success_failure', False):
                return done('outcome_unknown')
            return done(payload='action_result')
        if action == 'export':
            out['export_calls'] = 1
            if not kw.get('evidence_complete', True) or not kw.get('funding_ancestor', True):
                return done('result_proof_unavailable')
            if kw.get('header_count', 512) > 512 or kw.get('proof_bytes', PROOF_BYTES) > PROOF_BYTES or kw.get('block_count', 3) > 3:
                return done('result_capacity')
            if not kw.get('final_view_same', True):
                return done('outcome_unknown' if kw.get('source_observation_may_sync', True) else 'unauthorized')
            out['possible_file_publication'] = True
            if kw.get('after_link_failure', False):
                return done('outcome_unknown')
            return done(payload='proof_summary')

        out['publish_calls'] = 1
        refusal = kw.get('publish_refusal', '')
        require(refusal in ('', 'result_waiting_for_funds', 'result_scan_incomplete',
                            'result_node_offline', 'result_source_conflict',
                            'unauthorized', 'result_cancelled', 'result_capacity'))
        if refusal:
            return done(refusal)
        if kw.get('raw_tx_sync_unknown', False):
            out['publication_known'] = False
            return done('outcome_unknown')
        if self.publication == 'none':
            self.publication = 'built'
        if kw.get('already_anchored', False):
            require(self.included)
            return done(payload='action_result')
        if self.dispatch_count == 4294967295:
            return done('result_capacity')
        out['possible_dispatch'] = True
        self.publication = 'dispatch_unknown'
        self.dispatch_count += 1
        if kw.get('dispatch_arm_unknown', False):
            out['publication_known'] = False
            return done('outcome_unknown')
        out['submit_calls'] = 1
        if kw.get('send_unknown', False) or kw.get('ack_sync_unknown', False):
            out['publication_known'] = not kw.get('ack_sync_unknown', False)
            return done('outcome_unknown')
        self.publication = 'submitted'
        if kw.get('after_success_failure', False):
            return done('outcome_unknown')
        return done(payload='action_result')


class NodeOwner:
    """Captured epoch/phase model; never starts real processes."""
    def __init__(self, initial=None):
        initial = initial or {}
        self.view = initial.get('view', 'view-zero-1')
        self.epoch = initial.get('epoch', 0)
        self.mode = initial.get('mode', 'none')
        self.start_count = 0
        self.close_count = 0
        self.borrowers = []
        require(self.mode in ('none', 'starting', 'publication', 'mining'))

    def call(self, action, **kw):
        out = dict(action=action, error='', ignored_stale_cleanup=False,
                   mining_worker_started=False, automatic_admissions=0,
                   publisher_borrow_closed=False, borrowers_cancelled=0)
        epoch = kw.get('epoch', self.epoch)
        def done(error=''):
            out.update(error=error, mode=self.mode, epoch=self.epoch,
                       process_pair_starts=self.start_count,
                       process_pair_closes=self.close_count,
                       borrowers=list(self.borrowers), view=self.view)
            return out
        if action in ('snapshot', 'status', 'record', 'poll'):
            return done()
        if action == 'connect_for_action':
            if kw.get('view', self.view) != self.view or not kw.get('authorized', True) or not kw.get('parent_published', True):
                return done('unauthorized')
            if not kw.get('publisher_account', True):
                return done('result_node_offline')
            if not kw.get('source_prerequisite', True):
                return done('result_source_incomplete')
            if kw.get('competing_operation', False):
                return done('busy')
            if self.mode == 'starting':
                return done('busy')
            if self.mode in ('publication', 'mining'):
                self.borrowers.append(kw.get('job', 'job'))
                return done()
            self.epoch += 1
            self.mode = 'starting'
            self.start_count += 1
            return done()
        if action == 'ready':
            if epoch != self.epoch or self.mode != 'starting':
                out['ignored_stale_cleanup'] = True
                return done('result_node_offline')
            if kw.get('view', self.view) != self.view or not kw.get('authorized', True):
                self.mode = 'none'; self.close_count += 1
                return done('unauthorized')
            self.mode = 'publication'
            return done()
        if action in ('startup_cancel', 'startup_failure', 'old_cleanup'):
            if epoch != self.epoch or (action in ('startup_cancel','startup_failure') and self.mode != 'starting'):
                out['ignored_stale_cleanup'] = True
                return done()
            if action == 'old_cleanup' and kw.get('captured_mode', 'starting') != self.mode:
                out['ignored_stale_cleanup'] = True
                return done()
            if self.mode != 'none':
                self.close_count += 1; self.mode = 'none'
            return done()
        if action == 'borrow_end':
            job = kw.get('job', 'job')
            if job in self.borrowers:
                self.borrowers.remove(job)
            out['publisher_borrow_closed'] = True
            return done()
        if action == 'owner_exit':
            if epoch != self.epoch:
                out['ignored_stale_cleanup'] = True
                return done()
            out['borrowers_cancelled']=len(self.borrowers);self.borrowers=[]
            if self.mode!='none':self.close_count+=1
            self.mode='none'
            return done('result_node_offline')
        if action in ('stop', 'logout', 'reset', 'expiry', 'close', 'start_mining'):
            # Unlike explicit Stop, a refused Start cannot disconnect an owner.
            if action=='start_mining' and not kw.get('mining_authorized',True):
                return done('unauthorized')
            out['borrowers_cancelled'] = len(self.borrowers)
            self.borrowers = []
            if self.mode != 'none':
                self.close_count += 1
            self.mode = 'none'
            if action in ('logout', 'reset', 'expiry', 'close'):
                self.view = kw.get('new_view', '')
            if action == 'start_mining':
                self.epoch += 1; self.mode = 'mining'; self.start_count += 1
                out['mining_worker_started'] = True
            return done()
        if action in ('backup', 'recovery', 'projection', 'account_signing'):
            return done('busy' if self.mode != 'none' else '')
        raise ValueError('unknown modeled owner action')


class CacheSelection:
    def __init__(self, initial=None):
        initial = initial or {}
        self.other = initial.get('other_bytes', 0)
        self.result = initial.get('result_bytes', 0)
        self.selected = initial.get('selected', '')
        self.selection_epoch = initial.get('selection_epoch', 0)
        self.status_time = initial.get('status_time', 0)
        self.status_epoch = initial.get('status_epoch', 0)
        self.conflict = initial.get('conflict', False)
        self.active = False
        self.prior = None
        require(0 <= self.other <= TOTAL_CACHE and 0 <= self.result <= RESULT_CACHE)

    def call(self, action, **kw):
        error = ''; selected_changed = False
        if action == 'begin':
            if self.active:
                error = 'busy'
            elif self.other + RESULT_CACHE > TOTAL_CACHE:
                error = 'result_capacity'
            else:
                old = (self.result, self.selected, self.selection_epoch, self.status_time, self.status_epoch, self.conflict)
                self.result = RESULT_CACHE
                if not kw.get('ack_published', True):
                    self.result, self.selected, self.selection_epoch, self.status_time, self.status_epoch, self.conflict = old
                    error = 'stale_view'
                else:
                    self.active = True; self.prior = old
        elif action == 'backend_finished':
            self.active = False
        elif action == 'read':
            size = kw.get('bytes', 1)
            if self.active:
                error = 'busy'
            elif size > RESULT_JOB or self.other + RESULT_METADATA + size > TOTAL_CACHE:
                error = 'result_capacity'
            else:
                self.selected = kw['selected']; self.selection_epoch += 1
                self.result = RESULT_METADATA + size
                self.status_time = kw.get('observed_at', self.status_time)
                self.status_epoch += 1
                self.conflict = kw.get('conflict', False); selected_changed = True
        elif action == 'explicit_status':
            if kw.get('selected', self.selected) == self.selected:
                self.status_time = kw['observed_at']; self.conflict = kw.get('conflict', False)
                self.status_epoch += 1
        elif action == 'terminal':
            self.active = False
            same = self.selected == kw['selected']
            can_select = not self.selected or same or kw.get('started_epoch', -1) == self.selection_epoch
            size = kw.get('bytes', 1)
            if size > RESULT_JOB or (can_select and self.other + RESULT_METADATA + size > TOTAL_CACHE):
                error = 'result_capacity'
            elif can_select:
                self.selected = kw['selected']; selected_changed = True
                if kw.get('started_status_epoch',0) >= self.status_epoch and kw.get('observed_at', 0) >= self.status_time:
                    self.status_time = kw.get('observed_at', 0)
                    self.conflict = kw.get('conflict', False)
                self.result = RESULT_METADATA + size
            # Rejected old selection retains only its compact tuple/digest.
            # It neither replaces nor undercharges the newer selected payload.
        elif action == 'old_family_admit':
            additional = kw['bytes']
            if self.other + self.result + additional > TOTAL_CACHE:
                error = 'capacity'
            else:
                self.other += additional
        elif action == 'view_loss':
            self.result = 0; self.selected = ''; self.selection_epoch += 1
            self.status_time = 0; self.status_epoch += 1; self.conflict = False; self.active = False
        else:
            raise ValueError('unknown modeled cache action')
        return dict(action=action,error=error,result_bytes=self.result,
                    other_bytes=self.other,aggregate_bytes=self.result+self.other,
                    selected=self.selected,selection_epoch=self.selection_epoch,
                    status_time=self.status_time,status_epoch=self.status_epoch,conflict=self.conflict,
                    active=self.active,selected_changed=selected_changed)


class ExchangeDisposition:
    """Finite gate/prefetch/dispatch model; no real pipe or timer is exercised."""
    def __init__(self, initial=None):
        self.next_id=(initial or {}).get('next_id',1)

    def call(self, action, **kw):
        require(action in ('publish','check','finalize','export','status','record','job','cancel'))
        out=dict(action=action,error='',request_ids=[],final_command_sent=False,
                 auto_retry=False,prepare_from_poll=False,dispatch_inferred=False)
        def finish(code=''):
            out['error']=code;out['next_id']=self.next_id;return out
        if kw.get('cancel_before',False):return finish('result_cancelled')
        if kw.get('unavailable_before',False):return finish('result_unavailable')
        for _ in range(kw.get('prefetch_count',0)):
            out['request_ids'].append(self.next_id);self.next_id+=1
        if kw.get('cancel_after_prefetch',False):return finish('result_cancelled')
        out['request_ids'].append(self.next_id);self.next_id+=1;out['final_command_sent']=True
        if kw.get('lost_or_malformed_reply',False):return finish('outcome_unknown')
        if kw.get('view_changed_reply',False):return finish('outcome_unknown')
        if kw.get('refusal',''):return finish(kw['refusal'])
        # Even a valid queued ACK does not assert a transaction was sent.
        return finish()


def scenario(category, name, cls, steps, initial=None):
    model = cls(initial)
    out = dict(category=category,name=name,scope=SCOPE,initial=initial or {},steps=[])
    for action, args in steps:
        out['steps'].append(dict(action=action,args=clone(args),expected=model.call(action,**args)))
    return out


def build_fragment():
    cases = []
    def flow(name, steps, initial=None):
        cases.append(scenario('composite_phase',name,PublicationFlow,steps,initial))
    def owner(name, steps, initial=None):
        cases.append(scenario('node_owner',name,NodeOwner,steps,initial))
    def cache(name, steps, initial=None):
        cases.append(scenario('cache_selection',name,CacheSelection,steps,initial))
    def exchange(name, steps, initial=None):
        cases.append(scenario('exchange_disposition',name,ExchangeDisposition,steps,initial))
    flow('one_publish_prepares_then_sends',[('publish',{}),('poll',{}),('status',{})])
    for code in ('result_waiting_for_funds','result_scan_incomplete','result_node_offline'):
        flow('acknowledged_prepare_'+code,[('publish',{'publish_refusal':code}),('status',{}),('check',{})])
    flow('check_absent_never_prepares',[('check',{}),('status',{})])
    flow('prepare_unknown_no_send',[('publish',{'prepare_unknown':True})])
    flow('cancel_after_known_prepare',[('publish',{'cancel_after_prepare':True}),('status',{})])
    for fault in ('raw_tx_sync_unknown','dispatch_arm_unknown','send_unknown','ack_sync_unknown','after_success_failure'):
        flow(fault,[('publish',{fault:True}),('check',{})])
    for args, name in (({'authorized':False},'revoked'),({'original_view_same':False},'changed_view'),({'source_same':False},'source_incarnation'),({'held':True},'held'),({'fresh':False},'stale_login'),({'parent_published':False},'publication_window'),({'publisher_account':False},'other_gym'),({'saved':False},'unsaved'),({'source_healthy':False},'source_conflict')):
        flow('publish_'+name,[('publish',args)])
    flow('held_check_no_sign',[('check',{'held':True}),('record',{'held':True})],{'prepared':True,'publication':'submitted'})
    flow('other_gym_local_read_network_refused',[('record',{'publisher_account':False}),('check',{'publisher_account':False})],{'prepared':True,'publication':'submitted'})
    flow('readonly_observation_unknown',[('check',{'observation_unknown':True}),('status',{'observation_unknown':True})],{'prepared':True})
    for depth in (59,60,61):
        flow('attest_depth_'+str(depth),[('finalize',{'descendants':depth})],{'prepared':True,'publication':'submitted','included':True})
    flow('attest_captured_tip_replaced',[('finalize',{'captured_tip_still_member':False})],{'prepared':True,'publication':'submitted','included':True})
    flow('attest_replay_keeps_certificate',[('finalize',{})],{'prepared':True,'publication':'submitted','included':True,'finalized':True})
    flow('publish_anchored_no_resend',[('publish',{'already_anchored':True})],{'prepared':True,'publication':'submitted','included':True})
    flow('dispatch_uint32_exhausted',[('publish',{})],{'prepared':True,'publication':'submitted','dispatch_count':4294967295})
    flow('source_conflict_readable_not_republished',[('record',{'source_healthy':False}),('check',{'source_healthy':False}),('publish',{'source_healthy':False}),('finalize',{'source_healthy':False})],{'prepared':True,'publication':'submitted','included':True})
    flow('source_conflict_preserves_historical_finality_not_permission',[('record',{'source_healthy':False}),('check',{'source_healthy':False}),('publish',{'source_healthy':False}),('finalize',{'source_healthy':False})],{'prepared':True,'publication':'submitted','included':True,'finalized':True})
    flow('export_absent',[('export',{})])
    flow('export_unincluded',[('export',{})],{'prepared':True,'publication':'built'})
    export_base={'prepared':True,'publication':'submitted','included':True}
    for name, args in [('provisional',{}),('held',{'held':True}),('missing_body',{'evidence_complete':False}),('funding_other_branch',{'funding_ancestor':False}),('headers_513',{'header_count':513}),('bodies_4',{'block_count':4}),('bytes_over',{'proof_bytes':PROOF_BYTES+1}),('view_lost',{'final_view_same':False}),('link_unknown',{'after_link_failure':True})]:
        flow('export_'+name,[('export',args)],export_base)
    owner('polls_never_connect',[(x,{}) for x in ('snapshot','status','record','poll')])
    owner('startup_cancel_before_transfer',[('connect_for_action',{}),('startup_cancel',{'epoch':1}),('ready',{'epoch':1})])
    owner('cancel_after_transfer_keeps_owner',[('connect_for_action',{}),('ready',{'epoch':1}),('startup_cancel',{'epoch':1}),('connect_for_action',{'job':'r'}),('borrow_end',{'job':'r'}),('stop',{})])
    owner('startup_failure_after_transfer_ignored',[('connect_for_action',{}),('ready',{}),('startup_failure',{'epoch':1})])
    owner('old_epoch_cannot_clear_successor',[('connect_for_action',{}),('startup_cancel',{'epoch':1}),('connect_for_action',{}),('ready',{'epoch':2}),('old_cleanup',{'epoch':1})])
    owner('mining_borrow_timeout_keeps_miner',[('connect_for_action',{'job':'r'}),('borrow_end',{'job':'r'})],{'mode':'mining','epoch':3})
    owner('explicit_mining_start_drains_publication',[('connect_for_action',{}),('ready',{}),('connect_for_action',{'job':'r'}),('start_mining',{})])
    owner('refused_mining_start_preserves_publication',[('connect_for_action',{}),('ready',{}),('connect_for_action',{'job':'r'}),('start_mining',{'mining_authorized':False})])
    for act in ('stop','logout','reset','expiry','close'):
        owner('owner_'+act,[('connect_for_action',{}),('ready',{}),('connect_for_action',{'job':'seed'}),('connect_for_action',{'job':'result'}),(act,{})])
    owner('backup_conflict_then_stop',[('connect_for_action',{}),('ready',{}),('backup',{}),('stop',{}),('backup',{})])
    owner('owner_exit_has_no_automatic_reconnect',[('connect_for_action',{}),('ready',{}),('connect_for_action',{'job':'r'}),('owner_exit',{}),('poll',{}),('connect_for_action',{}),('ready',{})])
    owner('reset_rejects_previous_view',[('connect_for_action',{}),('ready',{}),('reset',{'new_view':'view-zero-2'}),('connect_for_action',{'view':'view-zero-1'})])
    for name,args in [('wrong_view',{'view':'old'}),('wrong_gym',{'publisher_account':False}),('missing_source',{'source_prerequisite':False}),('parent_pending',{'parent_published':False}),('backup_active',{'competing_operation':True})]:
        owner('connection_'+name,[('connect_for_action',args)])
    cache('reservation_exact_limit',[('begin',{}),('old_family_admit',{'bytes':1})],{'other_bytes':TOTAL_CACHE-RESULT_CACHE})
    cache('reservation_one_over_preserves_old',[('begin',{})],{'other_bytes':TOTAL_CACHE-RESULT_CACHE+1,'result_bytes':4096,'selected':'old'})
    cache('unpublished_ack_rolls_back',[('begin',{'ack_published':False})],{'other_bytes':100,'result_bytes':4096,'selected':'old','selection_epoch':4,'status_time':10,'conflict':True})
    cache('rejected_read_transactional',[('read',{'selected':'new','bytes':RESULT_JOB+1})],{'result_bytes':5000,'selected':'old','selection_epoch':2})
    cache('old_terminal_cannot_replace_new_selection',[('begin',{}),('backend_finished',{}),('read',{'selected':'new','bytes':8000,'observed_at':20}),('terminal',{'selected':'old','started_epoch':0,'observed_at':10,'bytes':7000})],{'selected':'old'})
    cache('fresh_conflict_survives_old_terminal',[('explicit_status',{'observed_at':20,'conflict':True}),('terminal',{'selected':'same','started_epoch':1,'observed_at':10,'conflict':False})],{'selected':'same','selection_epoch':1,'status_time':5})
    cache('same_timestamp_explicit_conflict_survives_terminal',[('explicit_status',{'observed_at':20,'conflict':True}),('terminal',{'selected':'same','started_epoch':1,'observed_at':20,'conflict':False})],{'selected':'same','selection_epoch':1,'status_time':5})
    cache('discovered_terminal_selects_when_empty',[('terminal',{'selected':'first','started_epoch':0,'observed_at':5})])
    cache('view_loss_releases_family',[('begin',{}),('view_loss',{})],{'other_bytes':1000,'selected':'old'})
    for action in ('publish','check','finalize','export','status','record','job','cancel'):
        exchange('lost_reply_'+action,[(action,{'lost_or_malformed_reply':True})])
    exchange('publish_cancel_after_prefetch',[('publish',{'prefetch_count':2,'cancel_after_prefetch':True})])
    exchange('cancel_cancel_after_prefetch',[('cancel',{'prefetch_count':1,'cancel_after_prefetch':True})])
    exchange('strict_transport_ids',[('publish',{'prefetch_count':3}),('job',{}),('status',{})],{'next_id':17})
    exchange('queued_ack_never_implies_send',[('publish',{})])
    exchange('definite_busy_not_unknown',[('publish',{'refusal':'busy'})])
    exchange('check_absent_not_prepare',[('check',{'refusal':'result_not_prepared'})])
    exchange('prefinal_unavailable_no_request',[('publish',{'unavailable_before':True})])
    return dict(version=1,kind='state',scope=SCOPE,cases=cases)


def self_check(fragment):
    cases={x['name']:x for x in fragment['cases']}
    last=lambda n:cases[n]['steps'][-1]['expected']
    first=lambda n:cases[n]['steps'][0]['expected']
    require(first('check_absent_never_prepares')['prepare_calls']==0)
    require(first('check_absent_never_prepares')['error']=='result_not_prepared')
    require(first('acknowledged_prepare_result_waiting_for_funds')['prepared'])
    require(first('acknowledged_prepare_result_waiting_for_funds')['submit_calls']==0)
    require(first('acknowledged_prepare_result_waiting_for_funds')['error']=='result_waiting_for_funds')
    require(first('send_unknown')['error']=='outcome_unknown' and first('send_unknown')['possible_dispatch'])
    require(last('mining_borrow_timeout_keeps_miner')['mode']=='mining')
    require(last('refused_mining_start_preserves_publication')['mode']=='publication')
    require(last('refused_mining_start_preserves_publication')['borrowers']==['r'])
    require(last('refused_mining_start_preserves_publication')['process_pair_closes']==0)
    require(last('old_epoch_cannot_clear_successor')['mode']=='publication')
    require(last('old_terminal_cannot_replace_new_selection')['selected']=='new')
    require(last('old_terminal_cannot_replace_new_selection')['result_bytes']==RESULT_METADATA+8000)
    require(last('fresh_conflict_survives_old_terminal')['conflict'])
    require(last('same_timestamp_explicit_conflict_survives_terminal')['conflict'])
    require(last('unpublished_ack_rolls_back')['result_bytes']==4096)
    for c in fragment['cases']:
        for s in c['steps']:
            e=s['expected']
            if c['category']=='composite_phase':
                require(e['engine_calls']==0 and not e['automatic_mining'] and not e['automatic_funding'])
                if s['action']=='check':require(e['prepare_calls']==e['statement_signatures']==e['submit_calls']==e['checkpoint_signatures']==0)
            if c['category']=='node_owner' and s['action'] in ('snapshot','status','record','poll'):
                # Poll retains the prior count; it never increments it.
                i=c['steps'].index(s);before=c['steps'][i-1]['expected']['process_pair_starts'] if i else 0
                require(e['process_pair_starts']==before)
            if c['category']=='exchange_disposition':
                require(not e['auto_retry']and not e['prepare_from_poll']and not e['dispatch_inferred'])
    return True


def canonical(v):
    return json.dumps(v,ensure_ascii=False,separators=(',',':')).encode('utf-8')+b'\n'


if __name__=='__main__':
    result=build_fragment();self_check(result)
    target=Path(__file__).resolve().parent/'fragments'/'state.json'
    target.parent.mkdir(exist_ok=True)
    target.write_bytes(canonical(result))
    print(json.dumps(dict(kind='state',cases=len(result['cases']),steps=sum(len(c['steps']) for c in result['cases']))))
