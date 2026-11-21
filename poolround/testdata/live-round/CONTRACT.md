# Set77: selected live round List/Refresh — proposed

**Keep original full Sync atomic/captured-tail behavior unchanged.** Optimize only repeated List/Refresh on the original fully audited live round owner. No source write/prefix API, partial catch-up, cap/schema/profile/pin change, persisted verification cache, timeout increase or economic change. Full WalkSettlement, ledger import, signing and pending-payment recovery stay unchanged. The prior page-step proposal is preserved as rescinded.

## Exact additive API

```go
// poolround: opaque original-owner handle; no files/goroutines/Close of its own.
type Live struct { /* original Store, private epoch */ }
var ErrLiveStale = errors.New("live round verification is stale")
func (s *Store) BeginLive(ctx context.Context) (*Live, error)
func (l *Live) List(ctx context.Context, after uint64) ([]Record, error)
func (l *Live) Refresh(ctx context.Context, blockHash string) (Record, error)

// pooloperator: explicit local opt-in, before first Run; no wire/GUI command.
func (r *Runtime) EnableLiveRoundMaintenance(ctx context.Context) error

// Private poolround seams; caller holds the original Store operation gate.
func (s *Store) buildLiveLocked(ctx context.Context) (*liveDirectory, error)
func (s *Store) readLiveRecordLocked(ctx context.Context, hash string) (Record, error)
func (s *Store) commitWithLiveLocked(batch *leveldb.Batch, next metadata, delta *liveDelta) error
func (s *Store) invalidateLiveLocked()
```

List is fixed to MaxPage=8, uses existing after-sequence semantics, and returns owned records. No caller-supplied source, node, digest, signature or proof callback is accepted. Zero/foreign/stale handles and read-only stores refuse. Contexts bound operations, not successful handle lifetime. Original Close revokes the handle and joins calls; no new cleanup owner is required. Both existing round versions may opt in while source/version/limit checks stay strict. This does not enable DailyV2 public-runtime construction.

## Directory and commit contract

First Begin performs the **complete existing consumed-source and namespace audit** before publishing a handle. Build a private bounded directory of canonical raw metadata, sequence-to-hash/index commitments and hash-to-exact-record digest. Digest is SHA256 of canonical raw bytes; the key is stored separately. Each entry originates in that full audit or a fully validated original sync commit. No disk `verified` field is trusted. The directory is bounded by selected MaxRounds. Repeated Begin returns the same valid handle; explicit reestablishment after stale state performs a full audit and creates a new epoch.

Every live operation uses the same original nonwaiting Store gate and checks epoch/lifecycle, selected source identity, exact stored metadata and touched index/record bytes against the directory. The original concrete source remains retained. No live path replaces original node, source or account authority.

Prepare a private delta only after original validation completes. It binds prior epoch/head and exact touched keys, owns next canonical bytes/digests, and makes no visible change before sync. `commitWithLiveLocked` preserves existing metadata monotonicity and synchronous-write behavior. After successful sync, publish metadata and delta under the same gate; applying a prepared delta introduces no new validation failure. Failed/uncertain sync poisons the owner and revokes the handle, retaining existing cleanup semantics. A successful mutator without a verified delta invalidates the directory. The old `commit` may remain a wrapper passing nil.

**Sync stays full and atomic:** it validates the entire captured accepted tail, old round bodies/indexes and complete namespace, including corruption beyond tentative additions, before its original single commit. Capacity/cancellation/error discards all tentative additions exactly as today. When live state exists, Sync may collect new record/index digests during this existing validation and publish its delta after the same commit. It must compare remembered bytes rather than silently bless an out-of-band changed record. If maintaining a correct delta is unavailable, invalidate; later explicit Begin fully reaudits. No new partial Through is committed or disclosed.

Legacy Refresh retains its full audit and may maintain the same exact one-record delta on success. Full read APIs never use the directory to skip namespace/source checks. A full audit finding corruption revokes live state. A read-only full audit does not stale a healthy directory merely by reading.

## Targeted List/Refresh

Live List validates metadata and at most8 exact sequence indexes/records; absence of a directory-known key is corruption, not an empty page. Reuse `source.AccountingPage(ctx, Through, Through, 1)` for original source liveness: require exact Config, and for nonzero Through require exact returned Through and empty Entries. With Through=0 the existing convention may return a newer tail and at most1 validated entry; discard both without advancing any consumed metadata/directory. This preserves original closed/poisoned/context gates without a new source API. List makes no node call.

Live Refresh validates metadata and the selected original index/record, then calls existing targeted `checkWinner` **before** node I/O. It obtains the same fresh node observation, validates it, and preserves the final post-observation `checkWinner` before committing. This retains both the original pre-observation source authentication and final source liveness fence without a full prefix scan.

Refresh preserves first60-descendant Confirmation and later funding-conflict observation. Any precommit failure leaves record/directory unchanged. Old rounds remain eligible regardless of ingestion order. Do not clone or scan a lifetime-sized map; use one-record deltas. No current canonicality, confirmation or spendability decision is cached.

Warm bounds: List<=8 index/record pairs plus1 source-gate call (no source entries for nonzero consumed prefix, at most1 for zero); Refresh1 pair, **2** targeted source-entry checks and1 existing node-observation operation, plus fixed metadata/lifecycle checks. Existing record/template/node limits and caller deadlines remain. No full iterator, full-source walk or hidden reanchor occurs inside these live methods. Full Sync, first Begin, cold Open and financial audits remain history-dependent.

Live scope does not promise immediate detection of unrelated untouched database mutation. **Existing Sync/Get/List/Refresh/WalkSettlement/audit/Open retain full checks and reject those mutations.** Financial import/signing still pass through those checks. A live method cannot rescue a store rejected by a full audit or silently fall back after corruption.

## Minimal concrete consumer and ownership

Enable uses Runtime's original local-operation gate and exact `r.rounds`. It is allowed only before first Run; a private selection-in-progress flag makes concurrent Run refuse. Close cancels and joins selection. Failure leaves prior maintenance selection intact. Success changes only the private maintenance handle, not pool, round Store, ledger, scheduler, backend or wallet.

Selected `roundMaintenance.step` **still calls original full `store.Sync` first**. Its existing rotating refresh path then uses live List/Refresh for at most8 records; capacity retains original round-refresh behavior. If Sync invalidated the handle, explicitly reestablish with full Begin before targeted reads under the same existing30s context; do not extend the deadline. Defaults keep old List/Refresh. Initial intake readiness still requires successful full Sync and that cycle's successful refresh, with no incomplete state. The scheduler uses the same fully committed consumed prefix as today: no new partial-tail exposure or scheduler barrier.

This removes up to9 repeated full audits after each maintenance Sync and addresses the measured721-Refresh repetition. It does not remove full Sync, prove cold performance, extend source/authority caps or alter D80/financial reservations.

Minimal implementation ownership: source/round lane owns new `poolround/live.go`, narrow `poolround/{types,store,operations,stream}.go` hooks and live tests; operator lane owns new `desktop/pooloperator/live_round.go`, narrow `maintenance.go`/Run selection guard and tests. No poolshare/registry/wallet/settlement codec or policy edits are needed. Root assigns lanes after approval. No implementation or producer execution has occurred.
