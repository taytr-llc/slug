# Restored gym owner: exact setup and file boundary candidate

Source-only proposal, 20 November 2026. This supplements the reviewed `PLAN.md`; it is not an adopted codec or a completed command. Existing carrier, public ownership, recovery v1, and schema12/15 bytes remain closed. The approved restored actual3 is running independently. No expected bytes or product code were generated for this proposal.

## Owner, inputs, and trust

`RestoredGymSession` is a new explicit owner for one selected state directory, one original recovery export, one independently supplied complete public ownership pin/read configuration, and one private node configuration. It does not contain an account/provider/vault authority. It holds a single operation gate, Loader, public reader/Store/source, concrete carrier Journal, and later the schema15 wallet/carrier owner. Existing backup Verify remains disposable12. The tool does not start mining or copy an old private tree.

Creation requires a fresh empty canonical directory and an explicitly selected ordinary recovery file. The initializer chooses nonzero random lower-hex64 `setup_id`, `wallet_owner_id`, `carrier_journal_id`, and `ownership_store_id` before subordinate effects. The selected local directory and its exclusive setup marker become the trusted local continuity record; reopening does not infer an authority or a source pin from a subordinate DB. Every invocation supplies the independently expected complete public pin/read configuration and node endpoint/trust configuration and compares them to INIT. Until activation completes, it also supplies the original selected file and compares exact bytes/digest/manifest. The local IDs are generated only on explicit create, never regenerated on resume. Copying or tampering with the whole trusted local directory is outside the ordinary application API, as for existing private journals; no new attestation key is introduced.

The fixed mode is `file-restored-gym`, version1, recovery version1/profile `slug-phase-a-fixture-bip39-v1`, restore schema12 and active schema15. Keep the existing published fixture/profile allowlist; this command is not a production password or derivation expansion.

## Exact ordered marker grammar

All records use strict UTF-8 JSON with every listed member exactly once, in the listed order; unknown/duplicate/missing members, nulls where not specified, floats/exponents for integer fields, and trailing non-whitespace refuse. Stored bytes are canonical compact JSON followed by exactly one LF. Hashes below cover canonical compact JSON **without** LF unless explicitly called file SHA. Re-encoding is compared before accepting a record. IDs are nonzero lower-hex64; SHA fields are lower-hex64; integer maxima are explicit. The file/guard/source validators remain necessary in addition to this grammar.

Nested `Ownership` is the exact existing `registry.PublicManifestPin` codec, including its source authority, four publication keys, rule, scan origin, and all fixed limits. `OriginalManifest` is the existing strict fixed-four-account `recovery.Manifest`, not an arbitrary 64-KiB object. `CarrierIdentity` is the existing `fightercarrier.Identity`; its pin is derived from the original manifest/new local owner/exact public ownership PinID, mode restored/schema15, and the frozen zero sentinel account/allocation fields. It must equal the independently derived expected value. No source/config field is accepted merely because its stored digest agrees with itself.

`SLUG-RESTORED-GYM-INIT.json` ordered fields:

1. `version` =1
2. `mode` =`file-restored-gym`
3. `setup_id`
4. `wallet_owner_id`
5. `carrier_journal_id`
6. `ownership_store_id`
7. `ownership` (complete existing pin)
8. `ownership_pin_id` (existing public pin digest)
9. `public_read` object ordered `url`, `tls_spki_sha256` (existing reader rules)
10. `node_binding` object ordered `url`, `ca_certificate_sha256` (see below)
11. `original_file_bytes` integer1..65536
12. `original_file_sha256` (SHA256 of the exact selected file bytes, including its original whitespace)
13. `original_manifest` (existing canonical manifest field order)
14. `original_manifest_sha256` (SHA256 of existing `MarshalManifest` bytes)
15. `carrier_identity` (existing complete exact Identity)
16. `carrier_binding_id` (existing `fightercarrier.BindingID`)
17. `restore_schema` =12
18. `active_schema` =15

INIT is at most32768 bytes including LF. `InitDigest = SHA256("SLUG restored gym setup\x00v1\x00" || canonical INIT)`; this domain identifies local setup, not a signing or ownership authority. No timestamp is needed for setup identity. Public URL is ≤2048 bytes under the existing reader validator; node URL ≤2048 bytes and is the exact canonical accepted endpoint, not a redirected/discovered replacement. The node binding omits username/password. `ca_certificate_sha256` is SHA256 of empty bytes for this milestone; it does not bind rotating RPC credentials. The node endpoint must use literal-loopback HTTP and empty CA, matching the existing private owned-node transport. Node HTTPS and remote node hosts refuse; the independently pinned public reader retains its existing verified-HTTPS support. The same input endpoint is converted to its exact `/ws` transport for wallet recovery/backend; no proxy, redirect, TLS bypass, or alternate-node fallback. Each connection still checks network/genesis/activation rules, and action observations retain exact instance/revision identity.

`SLUG-RESTORED-GYM-ACTIVATE.json` ordered fields:

1. `version` =1
2. `setup_id`
3. `init_digest`
4. `guard` (exact existing ordered `waddrmgr.RestoredCarrierGuard`)
5. `guard_digest` (existing `RestoredCarrierGuardDigest`)

ACTIVATE is at most16384 bytes including LF. Its guard contains the original manifest/hash, preselected local owner/journal/public pin, audited ready12 activation hash/height, and all eight activation cursors. It is durably published **before** changing schema12 to15. `ActivateDigest = SHA256("SLUG restored gym activation\x00v1\x00" || canonical ACTIVATE)`.

`SLUG-RESTORED-GYM-READY.json` ordered fields:

1. `version` =1
2. `setup_id`
3. `init_digest`
4. `activate_digest`
5. `guard_digest`
6. `carrier_binding_id`
7. `ownership_pin_id`
8. `ownership_store_id`

READY is at most4096 bytes including LF. READY means exact paired wallet/journal/ownership-store establishment and a successfully audited schema15 open, not current chain synchronization, completed public replay, recoverable escrow, or spend authority. A stale/offline source after READY remains unavailable without mutating markers. All three markers are immutable, exclusive publications; no stage overwrites another. Aggregate control-file allocation is at most64KiB, and each read precharges its own bound.

## Topology, publication, and restart

Allowed root children are exactly the three marker names above, `wallet/`, `carrier/`, and `ownership/`, plus one fixed owner-lock file if a platform advisory lock needs it. Names and directories are never taken from JSON. The root is absolute/canonical, ≤4096 UTF-8 bytes, owner-only, with no symlink ancestor; each child is an ordinary single-link file or directory as appropriate. Directory enumeration is bounded before retaining names. Reject hardlink aliases, symlink/reparse substitution, unexpected siblings, descendant/ancestor overlap with selected input files, and a selected recovery/config file residing in the mutable state tree. Use existing directory/descriptor identity helpers and exclusive creation patterns; do not weaken them to allow a test temp alias.

Hold a concrete exclusive root owner across marker decisions, subordinate opens, and session lifetime. A process death releases the OS lock, not durable IDs. Write exact bounded bytes to a same-directory exclusive0600 temporary, fsync, publish without replacing an existing destination, fsync the directory, then read/compare retained identity. A failed publication is not permission to overwrite: audit the exact target once; an uncertain or nonmatching result poisons the owner, joins work and preserves files. No unbounded retries or repair-by-deletion.

Creation order: INIT → exact empty carrier Journal → exact empty public ownership Store → BeginPhaseARecovery. Before wallet creation, existing subordinate files are opened only with the exact INIT identity and full native audit; partial creation that the existing component cannot audit is a typed storage refusal, not adopted/rebuilt. Once a wallet DB exists, either missing subordinate store refuses even before READY. Fresh creation never imports a populated wallet or journal. No carrier operation is admitted before READY.

Pending12 resume opens `OpenPhaseARecovery` with the exact original manifest. One Step remains128 raw child attempts or one canonical block. Close/cancel preserves pending DB and joins its owner. Ready12 is closed cleanly before inspecting activation. Persist ACTIVATE, perform exact-expected activation, audit/open15, then publish READY. A crash after ACTIVATE but before DB mutation retries only that exact guard; a crash after schema15 commit but before READY opens only the guard saved in ACTIVATE, never adopts a guard read from the DB. Schema/guard mismatch and unknown write outcome refuse until a fresh exact audit can resolve it. READY with any missing marker/store/wallet is terminal setup refusal; no fresh empty replacement journal.

### Necessary narrow activation API addition

Current `ActivateRestoredCarrierWallet` generates/returns its guard inside the write transaction. That is insufficient alone for a durable caller crash between activation commit and marker publication. Proposed additive methods, keeping the old API unchanged:

```go
func (l *Loader) InspectRestoredCarrierActivation(
    ctx context.Context, pubPass []byte, original recovery.Manifest,
    journal *fightercarrier.Journal,
) (waddrmgr.RestoredCarrierGuard, error)

func (l *Loader) ActivateRestoredCarrierWalletExpected(
    ctx context.Context, pubPass []byte, expected waddrmgr.RestoredCarrierGuard,
    journal *fightercarrier.Journal,
) (*Wallet, error)
```

Inspect requires unloaded ready12 and exact empty concrete journal, performs the same scanner/ledger/manager audit in one DB snapshot, returns only an owned public guard, and fully joins its close before return. It shares the existing sticky loader cleanup owner on any close failure. Expected activation rederives the snapshot in the same DB Update as namespace creation/schema mutation and compares exact guard bytes before any write. No caller DTO can skip these concrete audits. A resume can try exact OpenRestoredCarrierWallet first and then exact-expected12 activation only on an explicitly observed schema12 profile result with fully successful close; generic errors/unknown cleanup never select a fallback. A narrow internal version dispatch in this explicit owner can avoid opening twice without broadening generic loaders.

## Selected files and secret lifetime

Recovery and private node files each have65536-byte input bounds; expected public configuration has16384. Open through a retained regular-file descriptor, reject symlinks/hardlinks/aliases, precheck size and bounded read, and compare descriptor identity/size before and after. Reject input mutation; no reopen onto a replacement pathname during an action. A relocated exact recovery file is allowed on later explicit resume: trust is exact byte hash+size+strict original manifest, not an inode that cannot survive a normal copy. The file path/inode and phrase are never stored in public setup records. After READY, ordinary reopen needs no recovery file; it still requires independently supplied source/node config and exact retained INIT/ACTIVATE/READY.

Private node credentials remain only in bounded owned memory and the concrete transports; errors return closed categories. Never stringify RPCConfig or echo parser input. Recovery byte/entropy buffers are cleared promptly; inherited immutable mnemonic strings cannot honestly be claimed erased. No raw export, private DB/journal contents, signed transaction bytes, phrase, credential, or sensitive path is emitted to stdout/stderr. Public status may print setup ID, gym key/ID, progress, dated public ownership, request IDs, and explicit review fields. Signing bytes remain private Journal/Wallet data. Permissions remain0700 directories/0600 files. CLI argument parsing accepts file paths, never mnemonic/password literals.

## Automatic recovery inventory and action lifetime

Adopt the separate source-only proposal `set73-prep/restored-commit-discovery-api.md` SHA bc32c69f1bde0b381d7e19ea322cf42a335d94d2606bd8d4d7f467279d811937: opaque Store-owned fixed-head capture of at most4096 existing canonical commits, one capture slot, ≤1MiB fixed metadata, pages≤8, sorted height/position/txid. Filter using authenticated historical predecessor owner equal to the restored gym, not current owner or a caller's claimed locator. A stale source/node/head makes capture unavailable; it never silently changes head mid-page.

The session automatically enumerates every candidate, one expensive recovery probe at a time. Existing `PrepareRecovery` checks full canonical commit/funding parents/current unspent escrow; a new read-only wallet match scan checks128 original-export native84 paths per Step until funding+return match or bounded exhaustion, without creating a carrier request, acquiring a journal operation, or changing wallet state. Only an explicitly selected matched candidate and reveal/refund kind may create the later durable intent; its concrete recovery capability and match are rechecked before preparation. DTO candidates grant no signing authority. A typed exact-spent result, distinct from malformed/missing node or source evidence, permits reporting spent candidates separately from original-file-unmatched candidates; missing required source/chain evidence stops an honest complete claim. The UI/CLI need not know a commit ID in advance. A fixed-head discovery cursor is process-local; restarting explicitly recaptures/rechecks, it does not rewrite carrier state or sign again. Before review or signing, fresh opaque public recovery capability and original exported coverage remain mandatory.

Restore may run one bounded action up to10minutes before returning resumable progress; each RPC retains existing deadlines/cancellation and each Step remains independently bounded. Carrier/discovery actions use one30-second operation and native review expires after60seconds; no held inventory/public permit spans a ten-minute restore. Close cancels/joins the action, releases exact capabilities, then carrier owner, wallet Loader/backend, public Store/reader/source and root owner. Sticky cleanup failures retain their owner and storage; repeated nil from an underlying close cannot launder a prior error. No background goroutine continues after claimed closed status.

Existing caps are unchanged: recovery branch4096/total32768; block transactions/outputs16384;32MiB transaction work/8MiB watch payload; carrier128 operations/8MiB journal; native84 new funding≤16 inputs; fixed500000 escrow and10000 per-leg fee. Current sync watches all already-issued44/49/84/86 addresses with recovery window0; it does not expand live recovery lookahead. Exactly one unresolved carrier operation and permanent signed input/escrow exclusions remain enforced by concrete wallet and journal owners.

## Finite evidence required before command acceptance

Independently model these three new marker bytes/digests using old public manifests/pins plus literal modeled local IDs; product output is never the oracle. Add parser/cap/cross-record negatives and lifecycle fault tests for initial/partial/resumed setup, ready12→ACTIVATE→committed15 crash windows, exact file substitution, missing established pieces, original source mismatch, sticky cleanup and exclusive publication. Reuse already frozen carrier crypto/economic references.

Actual3 new-transfer acceptance is a dependency, not evidence for this unimplemented command. The later separately packaged command must additionally exercise automatic discovery and BOTH commit-only reveal/refund from original export-covered inventory, with original hosted trees closed/unchanged and no private authority/journal copy. Reveal changes fighter ownership only after the existing60-descendant finality; refund has no invented timelock and leaves ownership unchanged. Explicit-ID recovery alone is incomplete for the selected automatic-recovery scope.
