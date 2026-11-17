# Candidate desktop fight preview

The existing candidate gym gains an explicit, unrecorded fight preview between
two saved candidates from the same weight class. Both conditions are obtained
at one trusted observation time, and the deterministic result is retained only
for the current authenticated view. Replay does not run the engine again.

This documents frozen Set42 source, not a shipped product. D75 retains
D65 and its existing condition projection; the new validation studies passed
independent audit and successor acceptance. Final packaged preview acceptance
remains pending after a failed startup attempt and the cleanup repair described
in `CHANGESET_42_NOTES.md`.
There is no recorded fight, training/XP award, entry lock, transferable ownership,
future-block fairness or official league activation in this feature.

## Player flow

1. Sign in to an existing published fixture gym. Login does not start mining.
2. Select two saved candidates of the same class, including across inventory
   pages, and choose **Preview fight**. Selection is explicit; polling or
   navigating never creates a fight.
3. The helper acknowledges a queued task. Account, cancellation and mining
   controls remain available while its one shared work lane completes the task.
4. A completed result opens paused, showing its common as-of observation.
   Play/Pause, Previous/Next step, Replay from start and Show result operate
   locally. Scorecards show whole-fight components; exact decision ties and
   simultaneous corner stoppages have distinct explanations.
5. Changing the pair clears old replay data and cancels obsolete work. Logout,
   reset/view change, authorization failure or helper failure removes the old
   replay. Restart does not reconstruct a fight result cache.

The frontend uses text nodes for names/events and native-generated image-only
SVGs for portraits. Reduced-motion preference disables automatic timed playback;
hiding or closing the page pauses/disposes it. Conditions remain labelled as-of;
browser time does not age a retained result.

## Trusted pair boundary

`gymstable.ReadPreviewPair(ctx, Session, [2]string)` accepts only two canonical,
distinct full IDs. It sorts complete fighters by ID, looks up existing assigned
members in the concrete pinned source and verifies at most two normal proofs
serially. It compares the complete retained record digests and derived static
bindings before publishing anything. A public receipt, cached UI Item or caller
supplied class/condition does not grant assignment.

Both members must have the same canonical manifest weight class. They need not
occupy a roster slot; this is not an entry operation. The helper supplies zero
professional XP and zero training, because it has no trusted professional-result
or training-award authority. Pure-engine bonus support does not create one.

A final original-session View lease checks enrollment, account, generation and
view. The stable store's persisted monotonic observation clock supplies one
as-of time for both
condition-v1 projections. It is at least their retained original birth and
accepted-at observations. The projection synchronizes only existing clock/count
observation metadata. It does not assign new candidates, change a roster,
consume a prepared mutation or rewrite birth. If source growth is observed,
backlog remains honest; preview does not silently initiate catch-up.

Cancellation between the two source reads prevents the next proof. The current
proof is joined rather than detached. Unknown synchronized observation writes
retain the store's poison/reconciliation semantics and disclose no partial pair.
A terminal job reports that distinction as `preview_observation_unknown`.

## Recipe and deterministic model

The pair binds the complete stable pin/StoreID, source authority and candidate
descriptor. Its ordered member bindings contain full prospect ID and retained
source-record, manifest and condition digests. Condition digest is:

```
SHA256(ASCII("SLUG fight preview condition\0v1\0") ||
       LE32(canonical condition-v1 JSON length) || canonical condition-v1 JSON)
```

The pure helper regenerates the condition from its canonical DNA/birth/as-of/XP
and requires exact equality before using that digest. The recipe fixes engine
version1/ID, standard three rounds, training `[0,0]`, network/genesis, store/source/
descriptor, common as-of and those ordered bindings. Its seed is:

```
SHA256(ASCII("SLUG fight preview\0v1\0") ||
       LE32(canonical Recipe JSON length) || canonical Recipe JSON)
```

All hashes use lowercase hex. Reversing UI selection does not alter canonical
A/B. View/session identity and roster revision authorize and label the response;
they are not seed selectors. The seed is predictable and unrecorded. This is
not a future-block source, anti-grinding promise or fair official draw.

The helper constructs the full [fighterbout](FIGHTER_BOUT.md) input from the
trusted pair and seed, runs simulation and full deterministic result validation,
then publishes one owned result. Its input and output IDs bind the complete
model inputs/transcript. The native process does not run this engine.

## Protocol and compatibility

Preview is a separate closed JSON-lines family, Version1. It preserves the
existing Manifest/Stable v1 command and response bytes rather than appending
fields to them. `ServePreview` routes the families on the same owned pipe. The
existing explicit `--candidate-manifests --candidate-stable` helper route selects
this extended dispatcher. Other legacy, account and manifest routes retain their
previous constructors/dispatchers; no public HTTP endpoint or listener is added.

| Command | Required fields beyond `id`, `version`, `command` | Success value |
| --- | --- | --- |
| `preview_snapshot` | none | `snapshot` |
| `preview_begin` | `view_id`, `fighter_a_id`, `fighter_b_id` | queued `job` |
| `preview_job` | `view_id`, `job_id` | retained/current `job` |
| `preview_cancel` | `view_id`, `job_id` | current/cancelled `job` |

Native typed methods are `PreviewSnapshot`, `BeginFightPreview`,
`FightPreviewJob`, `CancelFightPreview` and `FightPreviewPortrait`. There is no
browser-facing generic command passthrough, caller seed, result import or
manifest/receipt input method. Typed native Begin sorts validated IDs before
sending; the wire grammar requires ascending order.

Response fields are `id`, `version`, `ok`, and exactly one matching success
value or sanitized `error`. A Snapshot has state/error, current view/store/engine
pin and active/queued/latest IDs. A Job additionally fixes its sorted two IDs,
state/error and nullable result. States are `queued`, `running`, `done`, `failed`
or `cancelled`; only `done` carries a result. The result contains Pair, Recipe,
Seed, EngineResult and EngineResultDigest. It contains no private receipt or key.

Error vocabulary is closed: `invalid_request`, `unauthorized`, `forbidden`,
`stale_view`, `busy`, `stable_corrupt`, `stable_clock`, `stable_conflict`,
`stable_sync_required`, `outcome_unknown`, `preview_not_found`,
`preview_class_mismatch`, `preview_capacity`, `preview_cancelled`,
`preview_unavailable`, `preview_observation_unknown`. A possibly dispatched Begin
whose acknowledgement is lost is `outcome_unknown`; it is not silently repeated
under a new job. Terminal observation uncertainty uses the distinct code above.
An unknown/evicted job is not regenerated by reading it.

The parser rejects duplicate decoded keys, aliases/unknown fields, missing
fields, unsafe or noncanonical numeric spellings, invalid UTF-8/nulls, trailing
tokens and excess arrays/depth/bytes. Transport can contain bounded whitespace
or reordered fields; validated typed canonical encoding supplies commitments.
No raw embedded engine substring must already be canonical. Fixed arrays are
checked before decoding; a decoder that silently drops extra elements is not a
valid substitute. Legacy errors/frames are not widened.

## Scheduling, cancellation and account isolation

Preview uses the existing stable/miner work lane. Across these operations there
is at most one active task and one queued task; it does not add a parallel Argon
or engine worker. A Begin reserves bounded result capacity before acknowledgement
and retains the original view/context with a 30-second task deadline. A failed
Begin preserves the earlier replay. An accepted new Begin replaces it.

Queued cancellation prevents execution. Running cancellation joins the current
proof and is checked before another proof, engine call, full result validation
and final publication. Stop/Close joins actual work without holding account or
controller locks. Final result publication and each later disclosure authorize
the captured current view; reset/logout/revocation cannot be repaired by swapping
in a fresh token. Current authorized held sessions may preview under the same
D72 interpretation as reversible roster use. Enumerated asset/recovery/market
restrictions stay in force.

Snapshot and result polls copy cached bounded DTOs. They do not acquire the
source for a proof, compute a new as-of or simulate. The latest result belongs
to one current view and is not persisted. Private source evidence is stripped
before the job becomes retained/public.

## Native correlation, portraits and UI cache

The native bridge first establishes the current Manifest view and full stable
pin/StoreID. Every preview must match them and the compiled engine. Requested
pair/job/state transitions are correlated to a bounded expected tuple, or a
same-view active/queued/latest job advertised by Snapshot. At most three tuples
are retained. Repeated completed results must preserve the complete captured
result, including Pair.Status; a private SHA256 digest detects a coherent status
rewrite even when recipe/InputID/transcript are unchanged.

Native checks canonical commitments, closed transcript shape and internal
score/state consistency without Argon or simulation. It retains only one bounded
presentation tuple containing the two validated manifests and their static
bindings, plus at most two SVGs. It does not retain a second full transcript,
receipt or condition in that tuple.

`FightPreviewPortrait` renders only a member of that checked completed tuple.
After rendering, and also on a cache hit, a fresh current-view job read must
still authorize the exact result/member before SVG disclosure. Images are bound
to job/result/descriptor/manifest/portrait digests. There is no arbitrary SVG
fallback or public manifest renderer on this path.

The frontend adds its own selection epoch and current-view checks. It discards
late pair/image replies after navigation of identity or selection, clears caches
on typed authorization/helper failure, and preserves a replay for a recoverable
`busy`. A busy Snapshot observed before a subsequent completed Job cannot evict
that newer result. Only the latest selected result is shown; local playback is
bounded to its transcript and makes no helper call.

## Resource limits

| Boundary | Maximum |
| --- | ---: |
| Request / entire response | 4,096 / 262,144 bytes |
| Transport nesting | 12 containers, root counted as 1 |
| Pair / Recipe / full PreviewResult | 24,576 / 4,096 / 229,376 bytes |
| EngineResult / total ticks | 196,608 bytes / 180 |
| Pair members / recipe members / training entries | exactly 2 |
| Helper preview reservation | 262,144 bytes within existing aggregate 1MiB cache |
| Native presentation metadata | 24,576 bytes |
| Native SVGs | two, each at most 32,768 bytes |
| Native total presentation / portrait response | 90,112 / 98,304 bytes |
| Frontend retained cache | existing aggregate 1MiB, two selections, one result, two portraits |

The existing authority's receipt/member capacity and stable-page bounds are
unchanged. Cached Page/Receipt evidence is not duplicated inside the retained
preview. No observer/poll is allowed to create implicit simulation or proof work.

## Validation evidence and pending release work

Independent complete transcripts, primitive/codec cases and wrapped public pair/
recipe/result/portrait fixtures verify exact bytes. Core/helper tests cover same-
time pairs, class mismatch, cancellation before the second proof/engine, final
lease loss, queue interactions, unchanged assignments/roster, private-receipt
stripping, cache capacity, observation uncertainty and no hidden catch-up.
Native tests cover request/result correlation, captured-status drift, portrait
cache-hit authorization and late responses. DOM tests cover explicit selection,
paging, busy-Snapshot/done-Job ordering, local-only playback, motion/visibility
and immediate typed-error invalidation.

The original source snapshot passed the full normal desktop suite, full tagged suite after a test-only fixture-profile
correction, desktop integration compile-only, vet and all 111 frontend tests
at its captured hashes. The earlier failed tagged attempt is preserved;
it is not represented as a clean first run. Normal production fixture expectations
must not be accepted under the cheap PoW profile.

The optional actual-helper test's final repaired-commit acceptance is pending;
the first failed startup attempt and cleanup regression evidence are retained.
With
`SLUG_PREVIEW_ACCEPTANCE=1`, it extends the existing `desktopintegration &&
!slugtestpow` stable acceptance using `SLUG_STABLE_HELPER`, `SLUG_STABLE_NODE`
and `SLUG_ACCOUNT_TEST_ARTIFACTS`. It mines at least six genuine receipts in each
published fixture gym to obtain a same-class pair, then checks initial/reopened/
held preview, three identical cached result rereads, cross-page native portraits,
unchanged persistent game/wallet facts, old-view refusal, restart cache absence
and exact process/listener cleanup. It publishes complete public pair/result/
portrait evidence for an independent checker. It does not exercise frontend
Play/Pause; that is DOM-tested and still needs actual packaged UI acceptance.

[D75](RATING_VALIDATION_POLICY.md) retains D65 and preserves condition v1, stable Pin/StoreID/markers/status,
assignment/roster/operation data and this preview-v1 recipe/seed. No current-v2
surface or coefficient change is authorized by these docs. The completed
[studies](RATING_VALIDATION_REPORT.md) and explicit D75 successor acceptance
leave the engineering package/deploy/observation gates ahead. Pending actual visual/native chooser/screenreader and
joint D74 review block friends/tester/public product release, not the already
authorized engineering deployment process after its required gates.
