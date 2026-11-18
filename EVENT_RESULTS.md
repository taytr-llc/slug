# Candidate result publication

Set48 adds a separate publication journal for an already saved candidate bout.
The first bout transcript, noon inputs, seed and outcome remain immutable.
Publication does not award professional or exhibition records, XP, training,
rankings, titles, prizes or ownership. C1's two-fixture candidate exception and
D65's rating formula remain unchanged.

This implementation is under integration validation on `slug-result-publication`.
The independently derived reference was committed first at `17e6045d`.
Actual-chain, packaged-app and deployment acceptance are separate gates; the
reference's modeled funding/result blocks do not establish normal proof of work.

## Publication and finality

The candidate screen offers four explicit actions:

- **Publish test result** prepares its signed statement and submits its one
  retained transaction when the separate test fund has a mature available coin.
- **Check status** advances bounded funding/inclusion observations. It does not
  prepare, sign, submit or automatically retry a transaction.
- **Finalize test result** requires the result's exact inclusion block, sixty
  descendant blocks and its own signed result checkpoint.
- **Save public proof** exports the complete bounded public evidence through a
  native-selected destination. The webview receives only a small summary.

The event seed's finality does not finalize a result. Connection state, last
chain observation, first historical finality and current conflicts are distinct.
Before the first result checkpoint, the same retained transaction can be
re-included after an orphaned inclusion; its original inclusion and scan history
remain recorded. Once checkpointed, the original inclusion, certificate and
historical finality stay fixed. A later crossing reorganization is reported as
a conflict under D62, without changing normal node chain selection.

Uncertain submission is query-first. A missing acknowledgment cannot trigger a
replacement transaction, new input or automatic retry. A stale cached job reply
cannot resolve a later uncertain request. The interface requires a complete
explicit read or a successful response correlated to the known uncertain job.

## Candidate authority and node ownership

The statement and checkpoint use separate, published Ed25519 fixture domains.
Publication spends only the published scalar-2 native SegWit test fund with a
fixed 1,000-satoshi fee. The existing seed scalar-1 fund and reservations remain
separate. These reproducible fixture signatures are conditional test evidence;
they are not production signing authority.

Only the original fresh, unheld Demo A session can prepare, publish or finalize.
A held Demo A session can view, check and export public result evidence. Demo B
can view the shared retained result but cannot connect the publisher. Original
session, account generation, authority, source incarnation and saved digests are
checked at the protected boundaries.

An explicit network action can start the existing owned node pair in the
`result_nodes` operation. This starts no wallet signer, mining worker or
admission service. Passive reads and polling do not connect nodes. Stop, logout,
reset, expiry and Close drain the relevant owner. An authorized mining Start
drains node-only ownership before starting the existing mining path. The
purpose-specific adapter borrows the exact process pair and never owns it.

## Storage and evidence

`gymresult` keeps compact statements and publication history in a separate
journal; it does not duplicate full fight transcripts. Its protected source
projection consumes the already audited Bout/Card/Seed/Event/Entry journals.
New result operations do not run the fight engine. The explicit parent schema8
installation retains recoverable prepared/created/ready boundaries and refuses
missing established components; older application routes cannot adopt it.

The fixed limits are 256 results, 128 funding coins, 256 permanent reservations,
eight distinct full-block hash attempts per borrowed operation, sixteen scan
restarts/prior inclusions and eight retained conflicts plus first equivocation.
The private four-field result index stays within96KiB. The new display family
reserves288KiB within the unchanged aggregate1MiB cache.

A public proof contains the saved Bout, Card, compact result record and status,
at most512 unique headers and three full blocks, up to24MiB including its final
newline. Headers cover complete genesis paths for the current tip, first result
finality, first seed finality and required bodies. The original funding
occurrence must be a mature ancestor of the retained result inclusion.
Historical blocks are fetched by their retained hashes.

The producer verifies each selected full block's normal Argon header, Merkle
root, witness commitment and coinbase structure. Header-only collection is
structural; the independent offline verifier checks every exported header's
work, links and difficulty. The proof makes no claim of full UTXO replay, global
best-chain selection, seed firstness, private wall-clock accuracy or production
ownership. It includes no fresh session, wallet recovery material or private
file paths. Existing immutable public receipt provenance remains intact.

Export writes one private temporary file outside account leases, synchronizes
it, then publishes it exclusively under the final original-session/source fence.
It never overwrites an existing destination or presents a partial proof as a
successful export.

## Validation and remaining release gates

The frozen contracts, derivation inputs, corpus, reproduction evidence and
reviews are in `gymresult/testdata/reference`. `freeze.json` has SHA256
`7d597804a76d6e5c4490c0455f1e7bb571454f77e5cb619580135b51698b0d93`;
`corpus.json` has SHA256
`07c00eef8d87ae9ae5dcd2d120a5dfd1f7b1ba33a1447b773cfd4d5194fd1a94`.
Keep failed candidates and the distinction between finite models and actual
owned-chain evidence.

The actual composition uses a cold copy of Set47, preserves both older
conflicted cards and original fighter births, and prepares the later December5
source from the same entrants. It exercises native publication, restart,
re-inclusion, depth59/60/61, report-only conflict and complete proof exports
without additional admissions. Passing that scoped-clock exercise does not
establish real-calendar package closure, native chooser behavior or visual
rendering.

Mike's thorough joint design review remains mandatory before friends/testers or
public release (D74). Portrait rendering, accessibility, production authority,
permanent genesis and the timing of professional record/XP awards remain
separate decisions or acceptance gates.
