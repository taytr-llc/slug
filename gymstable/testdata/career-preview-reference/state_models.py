"""Independent finite models derived before Set50 runtime implementation.

Initial draft: checkpoint_envelope; refinement: checkpoint_design_audit.
The original draft is preserved outside this candidate tree. These symbolic
scenarios do not execute an engine, proof, session, mutex, journal or renderer;
model counters are expectations, not measurements of runtime entry points.
Only standard-library pure helpers are imported. Full DTO/cryptographic
validation belongs to the separately authored reference schema/compositions.
"""
import copy

CONTRACT_SHA256 = '5cf347a9e402ae46e4682728733dc637d997a1f702d11ddd65c9f8ebd04efeeb'
AUTHORS = ['checkpoint_envelope (initial draft)', 'checkpoint_design_audit (refinement)']
PREVIEW = ('legacy_preview', 'career_preview')
POOL = 1048576
RESERVATION = 262144
JOB_CAP = 231424
TOKEN_CAP = 24576 + 512
SNAPSHOT_CHARGE = 2304
QUEUED_CHARGE = 2048
NATIVE_META_CAP = 24576
SVG_CAP = 32768
NATIVE_TOTAL = 90112


def clone(value):
    return copy.deepcopy(value)


def case(name, kind, inputs, expected, **extra):
    if kind in ['captured_pair', 'captured_page']:
        extra['refusal_semantics'] = (
            'Abstract capture categories only, not expected Go error identities or '
            'wire error codes. Preserve inherited core errors; the schema/DTO oracle '
            'alone defines exact wire admission. Refusal, no-payload and precedence '
            'are modeled here without prescribing a new error mapping.')
    return dict(name=name, kind=kind, input=clone(inputs), expected=clone(expected),
                modeled_only=True, **extra)


def captured_pair():
    # Symbolic facts, not an admissible public CareerPreviewPair DTO.
    return dict(owner='stable-owner-a', epoch=1, view='original-view',
                account='account-a', gym='gym-a', authority='authority-a',
                source='source-owner-a', source_identity='source-identity-a',
                activation='activation-a', checkpoint=2, revision=0,
                count=2, assignments=['assignment-x', 'assignment-y'],
                prefix='prefix-a', asof=100, totals=[1, 0])


def capture_check(change, page_handle=False):
    captured = captured_pair()
    current = clone(captured)
    current.update(clone(change))
    # Abstract categories only: these do NOT prescribe core/wire error codes.
    # E.g. foreign token owner, count regression and activation mismatch retain
    # their inherited concrete errors even when grouped together in this model.
    # Opaque foreign errors stay opaque; uncertainty keeps frozen precedence.
    if current.get('sync_unknown'):
        return dict(refusal_category='observation_uncertain', pair=None)
    if current.get('foreign_error'):
        return dict(refusal_category='opaque_foreign_error', pair=None, opaque=True, foreign_error=current['foreign_error'])
    if current.get('stable_owner_closed') or current.get('stable_owner_poisoned'):
        return dict(refusal_category='owner_or_source_unavailable', pair=None)
    if current.get('career_owner_closed'):
        return dict(refusal_category='career_owner_closed', pair=None)
    if current.get('career_owner_poisoned'):
        return dict(refusal_category='career_owner_poisoned', pair=None)
    if current.get('cancelled'):
        return dict(refusal_category='caller_cancelled', pair=None)
    if any(current[k] != captured[k] for k in ['owner', 'epoch', 'view', 'account', 'gym', 'authority']):
        return dict(refusal_category='original_context_mismatch', pair=None)
    if current.get('reader_closed'):
        return dict(refusal_category='career_owner_closed', pair=None)
    if current.get('reader_poisoned'):
        return dict(refusal_category='career_owner_poisoned', pair=None)
    if current.get('reader_unattached'):
        return dict(refusal_category='reader_unattached', pair=None)
    if current['activation'] != captured['activation']:
        return dict(refusal_category='activation_mismatch', pair=None)
    if current.get('source_closed') or current.get('source_unhealthy'):
        return dict(refusal_category='owner_or_source_unavailable', pair=None)
    if any(current[k] != captured[k] for k in ['source', 'source_identity', 'checkpoint', 'revision', 'assignments']):
        return dict(refusal_category='retained_source_facts_mismatch', pair=None)
    if current['count'] < captured['count'] or (page_handle and current['count'] != captured['count']):
        return dict(refusal_category='retained_source_facts_mismatch', pair=None)
    return dict(refusal_category='', pair=captured)


class Lane:
    """One shared serial lane and shared preview result, with explicit ACK state."""
    def __init__(self):
        self.active = None
        self.queued = None
        self.latest = dict(id='old', family='legacy_preview', bytes=100)
        self.selected = 'old'
        self.client_selected = 'old'
        self.preview_bytes = 100 + SNAPSHOT_CHARGE
        self.other_bytes = 0
        self.owner = True
        self.serial = 0

    def begin(self, family, *, final_authorized=True, ack_delivered=True,
              eligible_nonpreview=True):
        assert family in PREVIEW
        if not self.owner:
            return 'stale_view'
        if self.queued or (self.active and self.active['family'] in PREVIEW):
            return 'busy'
        if self.active and not eligible_nonpreview:
            return 'busy'
        charge = max(self.preview_bytes, RESERVATION)
        if self.other_bytes + charge > POOL:
            return 'preview_capacity'
        # Provisional allocation is not published to the runner before this
        # original authorization succeeds. A definite refusal preserves old.
        if not final_authorized:
            return 'stale_view'
        self.serial += 1
        job = dict(id='job' + str(self.serial), family=family,
                   state='queued', token=False, proof_calls=0, engine_calls=0)
        self.selected = job['id']
        self.latest = None
        self.preview_bytes = charge
        if self.active:
            self.queued = job
        else:
            self.active = job
        if ack_delivered:
            self.client_selected = job['id']
            return 'queued'
        return 'outcome_unknown'

    def release_nonpreview(self):
        assert self.active and self.active['family'] not in PREVIEW
        self.active = self.queued
        self.queued = None

    def run(self, *, stop_before=None, final_read='ok', sync_unknown=False):
        assert self.active and self.active['family'] in PREVIEW
        task = self.active
        task['state'] = 'running'
        stages = ['proof_a', 'proof_b', 'capture', 'simulate', 'digest', 'final_read', 'publish']
        outcome = 'done'
        for stage in stages:
            if stop_before == stage or not self.owner:
                outcome = 'cancelled'
                break
            if stage.startswith('proof_'):
                task['proof_calls'] += 1
            elif stage == 'capture':
                if sync_unknown:
                    outcome = 'preview_observation_unknown'
                    break
                task['token'] = True
            elif stage in ['simulate', 'digest']:
                task['engine_calls'] += 1
            elif stage == 'final_read' and final_read != 'ok':
                outcome = final_read
                break
        observed = clone(task)
        observed['token'] = False
        observed['state'] = 'done' if outcome == 'done' else 'cancelled' if outcome == 'cancelled' else 'failed'
        self.active = None
        if outcome == 'done' and task['id'] == self.selected and self.owner:
            # Synthetic fixed byte size, not an actual encoded engine result.
            self.latest = dict(id=task['id'], family=task['family'], bytes=200000)
            self.preview_bytes = 200000 + SNAPSHOT_CHARGE
        else:
            self.preview_bytes = QUEUED_CHARGE + SNAPSHOT_CHARGE
        return dict(outcome=outcome, task=observed, full_result_count=int(self.latest is not None))

    def cancel_queued(self):
        assert self.queued
        cancelled = self.queued['id']
        self.queued = None
        self.preview_bytes = QUEUED_CHARGE + SNAPSHOT_CHARGE
        return dict(id=cancelled, state='cancelled', token=False, proof_calls=0, engine_calls=0)

    def stop(self):
        # This terminal model describes state AFTER the actual work is joined.
        self.active = self.queued = None
        self.preview_bytes = (self.latest['bytes'] + SNAPSHOT_CHARGE) if self.latest else 0
        return 'stopped_after_join'

    def clear_owner(self):
        self.owner = False
        self.active = self.queued = self.latest = None
        self.selected = self.client_selected = None
        self.preview_bytes = 0

    def state(self):
        return clone(dict(active=self.active, queued=self.queued, latest=self.latest,
                          selected=self.selected, client_selected=self.client_selected,
                          preview_bytes=self.preview_bytes, other_bytes=self.other_bytes,
                          owner=self.owner, full_result_count=int(self.latest is not None)))


def correlation():
    # Complete symbolic commitment tuple; not a signature/source verdict.
    return dict(family='career_preview', view='original-view', store='stable-a',
                job='job-a', pair=['fighter-a', 'fighter-b'], activation='activation-a',
                prefix='prefix-a', asof=100, seed='seed-a', input_id='input-a',
                engine_digest='engine-a', whole_result='whole-result-a')


def select_terminal(known, incoming, selected, *, snapshot_owned=False):
    if known is None:
        return 'uncorrelated'
    if incoming != known:
        return 'refuse_preserve_commitment'
    if selected not in [None, incoming['job']]:
        return 'retain_newer'
    if selected is None and not snapshot_owned:
        return 'uncorrelated'
    return 'select'


def portrait_decision(*, cache_hit, selected=True, member=True, fixed_digest=True,
                      fresh_authorized=True, exact_tuple=True, family='career_preview'):
    result = dict(disclose=False, render_calls=0, authorization_calls=0, engine_calls=0)
    if not selected or not member or family != 'career_preview':
        return result
    if not cache_hit:
        result['render_calls'] = 1
    if not fixed_digest:
        return result
    result['authorization_calls'] = 1
    result['disclose'] = bool(fresh_authorized and exact_tuple)
    return result


def cases():
    out = []
    captures = [
        ('unchanged', {}, ''),
        ('unrelated-source-append', {'count': 3}, ''),
        ('same-ms-new-prefix', {'prefix': 'prefix-b', 'totals': [2, 0]}, ''),
        ('later-asof-does-not-rebase', {'asof': 101, 'prefix': 'prefix-c'}, ''),
        ('later-conflict-retains-award', {'reported_result_conflict': True}, ''),
        ('held-view-practice', {'held': True}, ''),
        ('off-roster-practice', {'selected_on_roster': False}, ''),
        ('retired-practice', {'retired': True}, ''),
        ('source-count-regression', {'count': 1}, 'retained_source_facts_mismatch'),
        ('checkpoint-change', {'checkpoint': 3}, 'retained_source_facts_mismatch'),
        ('roster-change', {'revision': 1}, 'retained_source_facts_mismatch'),
        ('assignment-change', {'assignments': ['assignment-x', 'assignment-z']}, 'retained_source_facts_mismatch'),
        ('concrete-source-owner-change', {'source': 'other-source-owner'}, 'retained_source_facts_mismatch'),
        ('source-identity-change', {'source_identity': 'other-source-identity'}, 'retained_source_facts_mismatch'),
        ('foreign-owner', {'owner': 'stable-owner-b'}, 'original_context_mismatch'),
        ('reset-generation', {'epoch': 2}, 'original_context_mismatch'),
        ('new-view-same-account-cannot-repair', {'view': 'replacement-view'}, 'original_context_mismatch'),
        ('foreign-account', {'account': 'account-b'}, 'original_context_mismatch'),
        ('foreign-gym', {'gym': 'gym-b'}, 'original_context_mismatch'),
        ('foreign-authority', {'authority': 'authority-b'}, 'original_context_mismatch'),
        ('activation-change', {'activation': 'activation-b'}, 'activation_mismatch'),
        ('closed-career-owner', {'career_owner_closed': True}, 'career_owner_closed'),
        ('closed-stable-owner', {'stable_owner_closed': True}, 'owner_or_source_unavailable'),
        ('closed-reader', {'reader_closed': True}, 'career_owner_closed'),
        ('unattached-reader', {'reader_unattached': True}, 'reader_unattached'),
        ('poisoned-career-owner', {'career_owner_poisoned': True}, 'career_owner_poisoned'),
        ('poisoned-stable-owner', {'stable_owner_poisoned': True}, 'owner_or_source_unavailable'),
        ('poisoned-reader', {'reader_poisoned': True}, 'career_owner_poisoned'),
        ('closed-source', {'source_closed': True}, 'owner_or_source_unavailable'),
        ('unhealthy-source', {'source_unhealthy': True}, 'owner_or_source_unavailable'),
        ('known-cancellation-after-observation', {'cancelled': True}, 'caller_cancelled'),
        ('career-close-before-known-cancellation', {'career_owner_closed': True, 'cancelled': True}, 'career_owner_closed'),
        ('stable-close-before-known-cancellation', {'stable_owner_closed': True, 'cancelled': True}, 'owner_or_source_unavailable'),
        ('unknown-wins-cancellation', {'sync_unknown': True, 'cancelled': True}, 'observation_uncertain'),
        ('unknown-wins-close', {'sync_unknown': True, 'career_owner_closed': True}, 'observation_uncertain'),
        ('opaque-foreign-error-preserved', {'foreign_error': 'opaque-consumer-error', 'career_owner_closed': True}, 'opaque_foreign_error'),
    ]
    for name, change, error in captures:
        expected = capture_check(change)
        assert expected['refusal_category'] == error
        if not error:
            assert expected['pair'] == captured_pair()
        out.append(case(name, 'captured_pair', change, expected))
    old = capture_check({'count': 3}, page_handle=True)
    assert old['refusal_category'] == 'retained_source_facts_mismatch'
    out.append(case('legacy-career-page-count-still-exact', 'captured_page', {'count': 3}, old))
    before = captured_pair()
    returned = capture_check({'count': 3})['pair']
    returned['totals'][0] = 999
    assert capture_check({'count': 3})['pair'] == before
    out.append(case('returned-pair-mutation-detached', 'ownership', {'returned_totals_changed': [999, 0]},
                    dict(retained_pair=before, second_read=before)))
    out.append(case('append-never-overwrites-fresher-shared-status', 'captured_pair',
                    dict(captured_count=2, fresh_source_count=3, shared_cache_count=3),
                    dict(returned_pair_count=2, shared_cache_count=3, captured_prefix='prefix-a')))
    out.append(case('new-explicit-capture-can-see-later-prefix', 'captured_pair',
                    dict(old_asof=100, later_finality_at=100, old_prefix='prefix-a', fresh_prefix='prefix-b'),
                    dict(old_replay_prefix='prefix-a', new_capture_prefix='prefix-b', old_replay_rebased=False)))

    for active in [None, 'stable', 'legacy_preview', 'career_preview']:
        for family in PREVIEW:
            lane = Lane()
            lane.active = dict(id='active-old', family=active) if active else None
            before = lane.state()
            result = lane.begin(family)
            assert result == ('busy' if active in PREVIEW else 'queued')
            if result == 'busy':
                assert lane.state() == before
            out.append(case('lane/' + str(active) + '/' + family, 'shared_lane',
                            dict(before=before, operation='begin', family=family),
                            dict(result=result, after=lane.state())))
    setups = [
        ('existing-queue', dict(active=dict(id='s', family='stable'), queued=dict(id='e', family='entry')), {}, 'busy'),
        ('ineligible-nonpreview-active', dict(active=dict(id='r', family='result')), dict(eligible_nonpreview=False), 'busy'),
        ('capacity-refusal-preserves-replay', dict(other_bytes=POOL-RESERVATION+1), {}, 'preview_capacity'),
        ('exact-capacity', dict(other_bytes=POOL-RESERVATION), {}, 'queued'),
        ('stale-owner', dict(owner=False), {}, 'stale_view'),
        ('final-lease-refusal-before-queue-publication', {}, dict(final_authorized=False), 'stale_view'),
        ('lost-ack-server-accepted-client-unselected', {}, dict(ack_delivered=False), 'outcome_unknown'),
    ]
    for name, setup, kwargs, want in setups:
        lane = Lane()
        for key, value in setup.items():
            setattr(lane, key, clone(value))
        before = lane.state()
        got = lane.begin('career_preview', **kwargs)
        assert got == want
        if got not in ['queued', 'outcome_unknown']:
            assert lane.state() == before
        if got == 'outcome_unknown':
            assert lane.selected == 'job1' and lane.client_selected == 'old'
        out.append(case(name, 'shared_lane', dict(before=before, operation='begin', options=kwargs),
                        dict(result=got, after=lane.state(), automatic_retry=False)))
    for family in PREVIEW:
        lane = Lane()
        lane.latest['family'] = 'career_preview' if family == 'legacy_preview' else 'legacy_preview'
        before = lane.state()
        assert lane.begin(family) == 'queued' and lane.latest is None
        done = lane.run()
        assert done['outcome'] == 'done' and lane.latest['family'] == family
        out.append(case('replace-latest-with-' + family, 'shared_lane', before,
                        dict(run=done, after=lane.state(), second_full_transcript=False)))
    lane = Lane()
    lane.active = dict(id='stable-job', family='stable')
    assert lane.begin('career_preview') == 'queued'
    queued = lane.state()
    lane.release_nonpreview()
    done = lane.run()
    assert done['task']['proof_calls'] == 2 and done['task']['engine_calls'] == 2
    out.append(case('eligible-nonpreview-releases-to-one-preview', 'shared_lane', queued,
                    dict(run=done, after=lane.state()), model_counter_scope='expectations only'))
    for reason in ['explicit_cancel', 'queue_deadline']:
        lane = Lane()
        lane.active = dict(id='stable-job', family='stable')
        lane.begin('career_preview')
        cancelled = lane.cancel_queued()
        lane.release_nonpreview()
        assert lane.active is None and cancelled['proof_calls'] == cancelled['engine_calls'] == 0
        out.append(case('queue-' + reason, 'shared_lane', dict(reason=reason),
                        dict(cancelled=cancelled, after=lane.state())))
    stages = [('proof_a', 0, 0), ('proof_b', 1, 0), ('capture', 2, 0),
              ('simulate', 2, 0), ('digest', 2, 1), ('final_read', 2, 2), ('publish', 2, 2)]
    for stage, proofs, engines in stages:
        lane = Lane()
        lane.begin('career_preview')
        stopped = lane.run(stop_before=stage)
        assert stopped['outcome'] == 'cancelled' and not stopped['task']['token']
        assert stopped['task']['proof_calls'] == proofs and stopped['task']['engine_calls'] == engines
        out.append(case('cancel-before-' + stage, 'job_stages', dict(stop_before=stage), stopped,
                        model_counter_scope='expectations only'))
    for refusal in ['stale_view', 'career_closed', 'stable_conflict']:
        lane = Lane()
        lane.begin('career_preview')
        stopped = lane.run(final_read=refusal)
        assert stopped['outcome'] == refusal and not stopped['task']['token'] and lane.latest is None
        out.append(case('final-token-read-' + refusal, 'job_stages', dict(final_read=refusal), stopped))
    lane = Lane()
    lane.begin('career_preview')
    unknown = lane.run(sync_unknown=True)
    assert unknown['task']['engine_calls'] == 0 and not unknown['task']['token'] and lane.latest is None
    out.append(case('capture-sync-unknown-no-token-or-engine', 'job_stages', {'sync_unknown': True}, unknown))
    for reason in ['logout', 'reset', 'helper_failure', 'close']:
        lane = Lane()
        lane.begin('career_preview')
        lane.run()
        lane.clear_owner()
        assert lane.latest is None and lane.preview_bytes == 0
        out.append(case('clear-' + reason, 'owner_lifecycle', dict(reason=reason), lane.state()))
    for complete in [False, True]:
        lane = Lane()
        lane.begin('career_preview')
        if complete:
            lane.run()
        lane.stop()
        assert bool(lane.latest) == complete
        out.append(case('stop-' + ('completed' if complete else 'running'), 'owner_lifecycle',
                        dict(completed_before_stop=complete),
                        dict(after_join=lane.state(), payload_retained=complete)))
    out.append(case('polls-copy-and-authorize-no-recapture', 'cached_poll',
                    dict(snapshot_calls=3, job_calls=3, local_replay_steps=180, fresh_prefix='prefix-b'),
                    dict(source_reads=0, proof_calls=0, engine_calls=0, result_prefix='prefix-a',
                         original_view_authorization_calls=6, local_replay_helper_calls=0),
                    model_counter_scope='expectations only, account clock observation is allowed'))

    # Deadline arithmetic is independent of the transition counters above.
    for caller_ms, elapsed_in_queue, kind, ceiling in [
        (40000, 29000, 'job', 30000), (10000, 9000, 'job', 30000),
        (40000, 30000, 'job', 30000), (40000, 0, 'poll', 250),
        (20, 0, 'poll', 250),
    ]:
        total = min(caller_ms, ceiling)
        remaining = max(0, total - elapsed_in_queue)
        out.append(case('deadline/' + '/'.join(map(str, [kind, caller_ms, elapsed_in_queue])),
                        'deadline', dict(kind=kind, caller_ms=caller_ms, queue_ms=elapsed_in_queue),
                        dict(total_ms=total, remaining_ms=remaining, may_start=remaining > 0)))
    out.append(case('stale-worker-cannot-install-over-new-selection', 'shared_lane',
                    dict(finishing_job='old-job', selected_job='new-job', owner_valid=True),
                    dict(install=False, selected_job='new-job', token_dropped=True,
                         newer_commitment_erased=False)))

    peak = JOB_CAP + TOKEN_CAP + QUEUED_CHARGE + SNAPSHOT_CHARGE
    assert peak == 260864 and peak <= RESERVATION
    out.append(case('exact-shared-preview-peak', 'cache_accounting',
                    dict(job=JOB_CAP, token=TOKEN_CAP, queued=QUEUED_CHARGE, snapshot=SNAPSHOT_CHARGE),
                    dict(total=peak, reservation=RESERVATION, margin=1280)))
    for family in ['result', 'bout', 'card', 'seed', 'event', 'entry', 'career_page', 'lazy_receipt', 'portrait']:
        for delta in [0, 1]:
            incoming = 4096
            existing_other = POOL - RESERVATION - incoming + delta
            total = existing_other + RESERVATION + incoming
            out.append(case('reciprocal-cache/' + family + '/' + str(delta), 'cache_accounting',
                            dict(preview_reservation=RESERVATION, other_bytes=existing_other,
                                 incoming_family=family, incoming_bytes=incoming),
                            dict(admit=total <= POOL, total=total, existing_preview_unchanged=True)))
    for action in ['done', 'failed', 'cancelled']:
        retained = JOB_CAP if action == 'done' else QUEUED_CHARGE
        out.append(case('terminal-drops-token/' + action, 'cache_accounting', dict(terminal=action),
                        dict(token_bytes=0, terminal_bytes=retained, snapshot_bytes=SNAPSHOT_CHARGE,
                             total=retained+SNAPSHOT_CHARGE, second_transcript_bytes=0)))

    known = correlation()
    for field, changed in [('family', 'legacy_preview'), ('view', 'new-view'), ('store', 'stable-b'),
                           ('job', 'job-b'), ('pair', ['fighter-a', 'fighter-c']),
                           ('activation', 'activation-b'), ('prefix', 'prefix-b'), ('asof', 101),
                           ('seed', 'seed-b'), ('input_id', 'input-b'),
                           ('engine_digest', 'engine-b'), ('whole_result', 'whole-result-b')]:
        incoming = clone(known)
        incoming[field] = changed
        got = select_terminal(known, incoming, 'job-a')
        assert got == 'refuse_preserve_commitment'
        out.append(case('native-mismatch/' + field, 'native_selection',
                        dict(known=known, selected='job-a', incoming=incoming), got))
    for name, registered, selected, owned, want in [
        ('new-terminal', known, 'job-a', False, 'select'),
        ('old-terminal-after-new-selection', known, 'job-b', False, 'retain_newer'),
        ('unsolicited-lost-ack', None, None, False, 'uncorrelated'),
        ('snapshot-owned-recovery', known, None, True, 'select'),
        ('not-snapshot-owned-recovery', known, None, False, 'uncorrelated'),
    ]:
        got = select_terminal(registered, known, selected, snapshot_owned=owned)
        assert got == want
        out.append(case(name, 'native_selection',
                        dict(known=registered, selected=selected, incoming=known, snapshot_owned=owned), got))
    out.append(case('same-job-older-null-terminal-cannot-erase-commitment', 'native_selection',
                    dict(known=known, incoming_state='running', incoming_result=None),
                    dict(known_commitment=known, presentation_cleared=False, payload_replaced=False)))
    out.append(case('fresh-source-self-asserted-activation-not-authority', 'native_authority',
                    dict(correlated_parent_activation=None, reply_activation='activation-a'),
                    dict(admit=False, presentation_retained=False)))
    for hit in [False, True]:
        for label, kwargs, disclose in [
            ('authorized', {}, True), ('original-view-lost', {'fresh_authorized': False}, False),
            ('changed-whole-tuple', {'exact_tuple': False}, False),
            ('wrong-member', {'member': False}, False), ('not-selected', {'selected': False}, False),
            ('wrong-family', {'family': 'legacy_preview'}, False),
            ('wrong-fixed-svg-digest', {'fixed_digest': False}, False),
        ]:
            inputs = dict(cache_hit=hit, **kwargs)
            got = portrait_decision(**inputs)
            assert got['disclose'] == disclose and got['engine_calls'] == 0
            if label == 'authorized':
                assert got['authorization_calls'] == 1 and got['render_calls'] == (0 if hit else 1)
            out.append(case('portrait/' + ('hit' if hit else 'render') + '/' + label,
                            'fresh_exact_job_authorization', inputs, got,
                            model_counter_scope='expectations only, no renderer invoked'))
    out.append(case('portrait-digest-is-engine-not-private-whole-result', 'portrait_binding', known,
                    dict(portrait_result_digest=known['engine_digest'],
                         private_whole_result=known['whole_result'], whole_result_disclosed=False)))
    for tuples, metadata, svg_a, svg_b, want in [
        (3, NATIVE_META_CAP, SVG_CAP, SVG_CAP, True),
        (4, NATIVE_META_CAP, SVG_CAP, SVG_CAP, False),
        (3, NATIVE_META_CAP+1, SVG_CAP, SVG_CAP, False),
        (3, NATIVE_META_CAP, SVG_CAP+1, SVG_CAP-1, False),
    ]:
        total = metadata + svg_a + svg_b
        accept = tuples <= 3 and metadata <= NATIVE_META_CAP and max(svg_a, svg_b) <= SVG_CAP and total <= NATIVE_TOTAL
        assert accept == want
        out.append(case('native-capacity/' + '/'.join(map(str, [tuples, metadata, svg_a, svg_b])),
                        'cache_accounting', dict(tuples=tuples, metadata_bytes=metadata, svg_bytes=[svg_a, svg_b]),
                        dict(admit=accept, total=total, selected_preserved_on_refusal=True)))
    out.append(case('shared-native-presentation-replaces-family', 'native_selection',
                    dict(old_family='legacy_preview', new_family='career_preview', new_job_correlated=True),
                    dict(presentations=1, full_transcripts_retained_natively=0, old_svg_count=0)))
    assert len({row['name'] for row in out}) == len(out)
    assert all(row['modeled_only'] is True for row in out)
    return out
