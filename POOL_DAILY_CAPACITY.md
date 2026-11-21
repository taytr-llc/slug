# Explicit daily pool capacity

Set76 adds a separately selected daily accounting profile with completed local
component, build, smoke and actual fresh-process payout qualification. Public
pool integration and deployment remain ahead; existing journals and V1 clients
retain their original identities and limits.

At the two-minute target interval, a pool finding every block would average
720 rewards per day. Spending 720 newly mature rewards and one carried change
requires 721 inputs. The existing V1 input and provenance limits cannot express
that ordinary daily case. Native SegWit can fit it within the existing standard
transaction weight: 721 inputs, 64 payments and change conservatively require
204,943 weight units and 51,236 virtual bytes. No consensus or subsidy change is
involved.

## Explicit owners and storage

Source, round, settlement and scheduler constructors select daily mode
explicitly. Fresh source Config2 and round Limits2 can retain up to 16,384
accepted submissions and completed rounds; selected lower limits remain
immutable. The source work limit is 8,192. Source and round logical storage
limits are at most 8 GiB. Existing constructors, markers and source identities
continue to enforce V1. Neither profile silently adopts another root.

Round economics and Round.Version1 bytes stay unchanged. Source validation
retains original jobs, accepted sequence ordering, global duplicate indices,
fixed round windows and the 4,096-share per-round limit. A private warm source
memo can avoid repeating proof hashing for an already retained exact work and
nonce. It retains no acceptance, expiry, canonicality, receipt or payment
authority. Reopening a source performs the cold normal-proof validation again.

Settlement Config2 separates immutable credits and signed payout bodies from
delivery status and inverse indices. Batch bodies retain exact signed bytes
and input references; audited preceding events reconstruct their complete
funding claims. The original owner's full namespace, indices, interleaved
credit/batch replay and original source are checked before financial disclosure
or mutation. Historical batch arrays are not rewritten on every payment.

The daily settlement limits are 16,384 original rounds and 32,767 batches,
8 GiB logical storage, and 256 distinct configured or credited miner identities.
Original claim provenance remains bounded and retained, including the pool's
fee claims after the miner has been paid. These finite limits do not constitute
indefinite operation or permission to discard history.

## Whole-payout selection and recovery

The node checks the complete live funding frontier in one observation, with
consistent chain evidence before and after the read. Its read-only frontier
limit is 16,384 coins. Signing, fee quotation and broadcast retain the separate
native transaction limits: at most 1,464 inputs, 256 payments and 400,000
standard weight units. A particular output count may impose a lower input
limit. Read capacity is not spending authority.

D84 applies to the whole ready payout. If it cannot fit, settlement retains all
balances, reports `transaction-capacity`, sends nothing and leaves the day
unused. No oldest-first subset, extra daily transaction, consolidation, miner
surcharge or reserve advance is permitted. Waiting alone may not remove a
capacity hold.

A previously prepared payment recovers before new credit import or capacity
planning. Exact inputs, claims, outputs, fee, signed bytes, transaction ID and
occupied day remain fixed. A lost broadcast response does not authorize a
replacement signature. Credits, reservations, signed body, indices, day and
report are committed synchronously together before transmission.

The dedicated wallet selects Binding2 explicitly while preserving schema16,
its existing key derivation and guard grammar. An active request from the
original attached settlement owner is required for signing. A configuration
digest or copied request is not a signing capability. Generic spending remains
unavailable to the dedicated wallet.

The daily scheduler binds that same writable settlement owner, recovers first,
and advances its UTC cursor only for a completed daily batch. Capacity and
delivery deferrals retain the cursor. Scheduler and node operation deadlines
remain unchanged.

## Qualification and remaining work

Independent reference producers are frozen and executed before expected codec,
identity, signature and transaction-shape bytes are consumed by Go tests.
Conditional fixtures are labeled separately from real proof or payment tests.
The actual financial acceptance case uses normal local nodes, one original
dedicated wallet, 64 public fixture miner identities, an initial payout followed
by a 721-input payout, a lost response, and one fresh-process owner reopen. The
passed run recovered exact reserved bytes without an extra broadcast, retained
all balances and verified one change with 721 origins. Cold source validation
took 142.534 seconds; the complete case took 1,255.733 seconds. Its
settlement dates use a test-only clock; it is not a claim that a real day elapsed
or that a public scheduler was deployed.

Public operator provisioning and transport selection, worker history limits,
signed fighter/publication authority limits, preserved late-winner reservations
at expanded capacity, and concrete host qualification remain release work.
Existing V1 histories remain intact. Sign-in and onboarding follow the public
pool milestone; the full joint D74 design review remains the final product
review before release.
