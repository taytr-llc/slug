# Set50 intake: career-aware, unrecorded same-gym preview

Source-only proposal; not a frozen contract or implementation authorization. No repository changes, reference generation, engine execution, tests or branch creation. Read against the exact files listed below while Set49 acceptance continues.

## Smallest useful outcome

Let a signed-in candidate gym explicitly preview two of its own consumed assignments using their trusted, already-accounted Career XP. Keep the current preview panel, deterministic engine-v1, condition-v1/D65, original birth/DNA, same immutable weight class, sorted full IDs and zero training. Preserve the inherited allowance for off-roster and retired candidates and for held View sessions: this remains practice, not entry eligibility. Display “Unrecorded preview”, both captured XP values and the common observation time. It awards nothing and creates no durable fight/result/history; replay never reruns the engine.

This removes Set49's explicit zero-XP practice limitation in `desktop/frontend/stable.js` (`acceptCareer` selection clearing, item button, `selectPreview`, replay's hardcoded XP0). It does not widen C1 booking, backfill noon, allocate title/quality bonuses, resolve ranking-window ties, or create a rankings/title screen. No new gameplay decision is necessary for this bounded slice. D74 remains separate.

Preserve every Preview-v1 recipe, frame, validator and frozen expected byte. Add separately typed Career-preview commands/DTOs on the existing Career route; a distinct version/domain makes the authority change explicit. Legacy callers continue to obtain the documented zero-XP preview. Do not make v1 validators accept new fields or reinterpret a v1 transcript as career-aware. No parent-v10 or journal migration is needed: v9 already binds the required reader and stores.

## Concrete trusted read and publication boundary

Current code: `gymstable.ReadPreviewPair` in `gymstable/preview.go` owns the Stable operation gate, resolves both immutable assignments before proof work, runs at most two serial `readSource` checks, and calls `publishRead` (`gymstable/read.go`) for one monotonic as-of and original View lease. `PreviewPair` currently projects XP0. The new operation must use that acquisition path, not two separately dated Career inventory pages or a browser-supplied overlay.

Recommended additive seam, with names to freeze later: `PrepareCareerPreviewPair(ctx, originalSession, [2]ID)` returns a separately typed pair plus a process-local opaque recheck handle; `ReadCareerPreviewPair(ctx, originalSession, handle)` returns a detached captured pair after a short recheck. Reuse the narrow opaque-read pattern of `gymstable/career_read.go`; there is no generic import, arbitrary callback, source selector or public XP setter. Either an explicit third private handle kind or a dedicated pair handle is sufficient; no new framework is needed. Wrong store/kind/session or reopened-owner handles must refuse.

The preparation order should be precise:

1. Validate/sort two distinct IDs, enter the existing Stable gate and original `gymaccount.View` authority, consult concrete source Info, and resolve both consumed assignments. No catch-up, substitution or cross-gym lookup.
2. Verify both source receipts/proofs serially outside account/store/cache locks, preserving original manifests, birth, source-record and portrait digests. Compare immutable class; join cancellation after the current proof before starting the next.
3. Capture one real monotonic Stable as-of at least both births/accepted times and Career activation. Obtain **one** `careerstate.Current` snapshot for those exact sorted two IDs from the already attached guard/reader. That reader fixes Result activation and authorized Stable StoreID scope, copies a bounded audited event index under only the short Result mutex, then derives both totals. Missing/inactive/poisoned/closed authority is a refusal, never XP0 fallback.
4. Project both conditions at that same as-of from original DNA/birth and their respective `Totals.XP`. Reacquire the original View and check assignment/checkpoint/roster/source-count consistency plus reader activation/liveness before exposing the composite. Only the inherited clock/count/LastAsOf observations may be synced. Unknown observation writes disclose no pair and retain existing poison/unknown semantics.
5. Seal detached public bytes and exact Stable owner, original Generation/ViewID, pair IDs, source revisions, activation and captured Career digest in the private handle. Do not retain private receipt aliases. Release core gates/leases before simulation.
6. Immediately before publishing the terminal preview, use that handle to recheck the original account epoch and concrete source metadata/assignments and reader liveness, then the controller's original full View/active task fence. This final read performs no Argon or engine work and accepts no caller-authored DTO. The new completed result remains bound to the original captured pair; it cannot be re-labelled by a later page or view.

A Current snapshot is a captured prefix, not a permanently closed time interval. A later first Finality can extend the prefix even at the same millisecond. Final recheck should verify health/activation without silently replacing the captured digest/XP; a newly requested preview captures the new prefix. Later result conflict does not undo the retained first-finality award. **Do not call `Result.FenceCareer` for this preview**: it is needed for sealed future cutoff consumers, whereas practice uses Current and creates no cutoff receipt. Never enter Result's action gate/source traversal while holding Stable/account locks; the existing short borrowed-reader lock order is the appropriate boundary.

## Recipe, engine and cache

Use a new canonical recipe/domain (for example `SLUG career fight preview\0v1\0`, actual NUL bytes and explicit length framing), retaining network/genesis/Stable source identity/descriptor/as-of/engine/rounds/zero training. Bind exact ActivationID, Career digest, and each sorted fighter's full ID, source-record digest, manifest digest, career condition digest and accounted XP. The captured DTO should include enough activation/totals to validate the recipe and display XP, without duplicating the full immutable source or transcript. Session/job IDs authorize/correlate; they remain outside deterministic seed selection. A new recipe version intentionally has distinct bytes even at XP0.

Construct unchanged `fighterbout.FighterInput.ProXP` from the sealed totals (0..15), with `TrainingSkill=0`; do not add the XP-adjusted `LeagueAttributesTenths` again. The engine derives aged attributes and applies XP once, as `FIGHTER_BOUT.md` specifies. Retain the existing explicit `Simulate` plus `Digest`'s full deterministic re-simulation inside the bounded job. This is two known engine evaluations, not an invented once-only guarantee. Cached native/helper parsers and local replay must not call re-simulating codecs.

Keep the existing shared lane, one active plus one explicit queued task, original-session cancellation/Close join, 30-second work deadline (including queue) and latest-terminal-only preview cache. Prefer one latest preview slot across old/new modes, with a closed type discriminator; do not retain a second full transcript or enlarge the1MiB aggregate casually. Cached controls keep their existing short authorization budgets; full proof+projection remains in the30-second job. Charge any pair handle alongside terminal result during overlap, then release the handle after guarded publication. Independently prove the full new wrapper/recipe/portrait/cache envelope before freezing DTO fields: existing224KiB result/256KiB allowance should not be assumed to absorb extra fields merely by adding their nominal caps.

Native correlation must pin the exact new family/version, original full View, job ID, sorted pair, activation/digest/as-of, seed/InputID, engine digest and a digest of the complete captured result. Late/older terminals cannot replace a newer selection. Reuse existing typed native portrait authorization and immutable manifest/portrait digests; no web SVG import or renderer bypass.

## Independent inputs and focused acceptance

Before runtime authoring, pin the final Set50 prose and the exact existing Python sources/inputs: frozen engine `reference_bout.py`, preview generator/transport freeze, manifest/condition/portrait reference dependencies, and Career arithmetic/prefix producers and input indexes. Copy minimally, never use new Go outputs as expected values. Existing preview examples are explicitly synthetic transport data; they prove no admitted assignment or finality.

The new reference should compose modeled finalized Career events into Current totals/prefix digest, both conditions, new recipe/seed, unchanged engine InputID and transcript/digest. Include zero/nonzero XP, unequal XP, cap15, draw-derived XP, retained first Finality followed by conflict, before/equal/after as-of, same-ms later-prefix distinction, canonical reversal, original-birth preservation and deliberate double-application negatives. Also cover malformed/mixed activation/IDs/time/digest/XP, v1/v2 cross-decoding, source/job/view substitution, size/depth ceilings and finite terminal/lost-ACK models. Transport-only checks must trap engine/work calls. No new vector may redefine the old v1 corpus.

Runtime acceptance should target concrete two-proof acquisition, original View/held/logout/reset/Close, reader closure, changed assignment/source count during engine gap, detached output, immutable replay under later finality, cancellation before second proof, cache reciprocity and two cold proofs with256 Career events under unchanged work budget. Keep normal latency separate from race correctness. A genuine integrated positive needs an awarded fighter and a **same-gym, same-class** consumed partner; do not silently borrow the opposing gym's booked corner. Verify availability from completed retained artifacts before execution, or state the bounded legitimate fixture setup explicitly. Model-only reference pairs are not a substitute for this authority test.

Likely implementation files are additive pair logic/tests under `gymstable`, separate typed helpers beside `desktop/pilot/preview_*`, native correlation beside `desktop/internal/bridge/preview_bridge.go`, parent forwarding in `desktop/accountpilot/preview.go`, and the existing `desktop/frontend/stable.js` preview controls/tests. No changes to engine equations, manifests, condition/D65, career award policy, entry/card booking or persistent schemas are needed.

## Inspected source/input identities

These hashes bind intake research, not a future freeze or execution result.

- `FIGHT_PREVIEW.md` — `e5dddc1129a7561a125fa1eebe8847523ff6e49c00647817374c6320fab12d60`
- `FIGHTER_BOUT.md` — `7f4a3683097877e3463523c045dd1c78668c128f3675efab14d33d5b926bd28d`
- `FIGHTER_CAREER.md` — `07de417554cf2a61085e8545f78fbf43bb5b02fc63a3d3d4d6889acadbe193d1`
- `CAREER_ACCOUNTING.md` — `e771d004a9815d4c7a21ea071dbc96295bfcbdf7d671d76df89e5875481c33dc`
- `CAREER_CONSUMERS.md` — `622e7b96d6a784555dd96c6eb86e5c4e1c66db5cf441aeb8e655e3a1dc7945a6`
- `gymstable/preview.go` — `56c1f126f84b5d5c9d46d838bea9209ead789cc777d45584e9ff61b580a167bd`
- `gymstable/read.go` — `3107ab73cdcfc5f79dba7097ab23a119db4302acd1b07ba6e8d6fde0b19666ae`
- `gymstable/store.go` — `f27101ad01805ac7d48ac39430ee7527be4a3ad8c95199c32c642ed8ae77e338`
- `gymstable/career.go` — `aef30959bb54fde9d7a8a8734953d5267b6d6c0793be631fb84ee4d9317d7c4a`
- `gymstable/career_read.go` — `e9c8c6d6ee54f111f2fa38509c0c25353ef5f9dce51650150a5d5ddace632f89`
- `internal/careerstate/state.go` — `eaedd8f52777cbb11b1e085474a1b51d2a144bd5179542156f5c2d8713463528`
- `internal/careerconsumer/guard.go` — `f592047da0031c2d4931d1bf7ae606d10bd08e3f0a959269ee678cadfccf6984`
- `gymresult/career.go` — `3ea3e29088bab7be2d3779dc9056211e38a58cded11eb5cf09b87d875ef07739`
- `fighterbout/input.go` — `75c29f8c30f481b155beec5d91b4f304a9e5064929efb8cf1f347d74b223431d`
- `fighterbout/codec.go` — `95a76026eb68322958b1cec915ada910f71957ca8f1068929536b5b8133f9673`
- `desktop/pilot/preview_controller.go` — `5b381f3aeb630d1c78c36d2564a7656b0bcee5d1ac2bac1cf9176e68ecad0879`
- `desktop/pilot/preview_binding.go` — `8aebb8b014eabb5d66e8b17b7238087f6dfbc79c9fa215e0e48e8e9207e3da3c`
- `desktop/pilot/preview_types.go` — `6fccac0c575ceb89784cd425628c7b186ed9324c58a6bcbae05c4c3006ca1f9f`
- `desktop/pilot/preview_validation.go` — `1094eb74d19d2fa7de1ad661b6b0111d52fd1d230a64f56140f1488dd7087ce9`
- `desktop/internal/bridge/preview_bridge.go` — `bd06c34bdea6476e85d400584758a4c9ee8f1a13273067a41428ae7b595bf02a`
- `desktop/accountpilot/preview.go` — `4ded35514d987e8795c79517ca47d827f1b5197af895849dac1814dd942468a5`
- `desktop/frontend/stable.js` — `ccce87415808f8ca224a87d58772faac437d05417d1ce1d24b19fc0fca8617db`
- `fighterbout/testdata/reference/reference_bout.py` — `c3247a0d856270cc5043b5767ace9f628b2efb9cd120d786773f9221d928bd7f`
- `fighterbout/testdata/reference/generate_preview.py` — `5f3ae24d63e339c5c4f174a7943eca233aae27fc0ca7031762d675cf824e25ed`
- `fighterbout/testdata/reference/preview-freeze.json` — `7a9da17f02a0724531ae84835257fad68cf148c8c7fe543417bee82c168c3911`
- `internal/careerstate/testdata/reference/generate.py` — `9cd4b150c274ef4bc54b0a21e35013d86b540405bbf003c8b7b0dd8899a53bdc`
- `internal/careerstate/testdata/reference/file_index.json` — `b3271752c4f4c8e6d4c44751df19ec38310be58db9a8cc8011d0511c57c43e9f`
- `fightercareer/testdata/reference/generate.py` — `506cb30affe74039b16bd0254f236fbfea4cf4b69f8a35760b0373666565857c`
- `fightercareer/testdata/reference/file_index.json` — `04bf565d748f4ad1cf8b70e7ffefd2d426bb6ce9949b3eff20a60251330d9efa`
