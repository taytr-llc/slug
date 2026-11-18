# Set46 card metadata read engineering addendum

This additive correction is authorized by the parent review after the actual packaged normal-clock metadata failure. The original contract, source commit, failed package, prepared-view-only candidate, timing failures and expected reference corpus remain retained. C1, public schemas, canonical bytes, RulesID, identities, seeds, engine and source authority do not change. The later explicit Card-only metadata budget amendment is recorded below.

## Explicit metadata context budget amendment

`set46-card-liveness-engineering-amendment.json` (SHA256 `b48a45ce117d226dce351b629c8a20794c19433ed9f6d69410372158690cc2eb`) approves a Card Read/Status/History context/admission budget of 500 ms after the fully charged parent-equivalent account checks still exceeded 250 ms following both redundancy reductions. This is a declared engineering correction, not a gameplay, seed, authority or uncertainty-rule change. Shorter caller deadlines remain effective. Cached polls, other family budgets, native/job limits and payload/cache caps remain unchanged; synchronous writes still join rather than promising hard wall-clock interruption.

The decision-time text with the original 250 ms requirement remains byte-identical at `set46-card-read-liveness-index-addendum-pre-budget.md` (SHA256 `9179e4a5f2414cb40ebfa818c088aa88823f2812962ae6f951b341057b8e4861`). The decision record pins those historical bytes; this current document supplies the explicit subsequent provenance mapping. No 250 ms failure is reclassified as a pass.

## Original-session prepared source view

The additive `gymseed.WithCardPreparedView` implements the approved sequence in `set46-card-read-liveness-proposal.md`: initial original View authorization, owned immutable Info/source binding, retained-row preparation outside leases, then the existing full concrete seed-source validation and final same-session source/clock guard. Only Card Status, Read and History adopt it. Existing WithCardView, WithCardSource and Build authority behavior remains.

Dependency closure before completion is classified as closed rather than caller cancellation; a possibly committed observation still has outcome-unknown priority. Close drains the resource gate and the owned callback. No context lifetime, token refresh or generic authority bypass is introduced.

## Private audited immutable-row index

The existing private index gains one SHA256 of the exact canonical stored Record bytes. Each of at most 256 entries retains only a 64-byte CardID, one cutoff timestamp and one 32-byte digest. No Record, CardSource, Plan, condition, manifest or result payload is cached. Slice allocation is exactly the audited/inserted count. On the present 64-bit runtime, index struct fields/backing allocation plus ID bytes and slice header total at most 30,744 bytes, excluding allocator bookkeeping; the stated logical retained-index budget is 64 KiB. Preparation may transiently hold the old and next bounded index; neither becomes a public DTO or response cache. This is a candidate storage bound, not a league limit.

- Open retains its complete canonical and semantic audit of every physical row, creation chronology and count/key graph. Only then does it retain each exact raw-byte digest.
- Build retains full owned Record/Plan/source validation outside its final account lease. Its final actual creation timestamp is serialized once for the existing synchronous batch. The next private index hashes those exact bytes and becomes visible only after successful Sync and metadata publication. A lost acknowledgement leaves the store poisoned and no new usable index entry; reopening performs the original full audit and reconciles retained bytes.
- A selected row read first looks up its private audited ID, obtains the bounded authoritative database value, and compares its raw SHA256. Missing indexed rows, present unindexed rows, or any selected byte drift are corruption. True absence requires both the selected ID and database row to be absent.
- Only after exact byte equality may the private path decode an owned Record without repeating the semantic audit already established for those bytes. Cheap ID/cutoff correspondence is retained. The ordinary public Validate/Parse/Marshal APIs remain fully semantic and unchanged.
- History still scans only the bounded ID/date index, then checks/decodes at most eight selected rows. Unselected external physical corruption is detected when selected or by the next full Open audit; this does not claim a whole-database scan on every metadata request.
- Current concrete seed rows remain fully validated outside the final source lease on every explicit source observation. Source identity, source creation time, current conflict overlay, original View and monotonic clocks are all still checked. A saved-row digest is not an authorization verdict and cannot suppress source conflict or revocation.

## Required verification

Focused regressions cover valid-but-different selected bytes, missing selected rows, unexpected unindexed rows, exact absence, owned returned copies, failed full semantic reopen, successful-Sync-only index publication, lost-ACK poison/reopen, source-created mismatch before row access, original-session denial/revocation/cancellation, closed dependency joining and post-sync uncertainty. Existing frozen DTO/derivation vectors must remain byte-identical. Real-account advancing-clock first/late/exhausted pages at 256 modeled histories, plus exact parent/helper normal-clock empty-journal reads, must pass the explicit 500 ms context budget without busy/unknown retries; fixed-clock source tests alone are not sufficient latency evidence.
