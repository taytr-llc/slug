# Change set 76: explicit daily pool accounting and custody

Set76 adds a fresh, explicitly selected daily profile across pool source,
rounds, settlement, node evidence, operator wallet and scheduler. Existing V1
constructors, identities and limits remain strict. This is finite capacity
engineering; public hosting and continuous operation are not yet qualified.

## Behavior

`poolshare.CreateDaily/OpenDaily` select Config2 and a distinct source digest.
The selected ceilings are 8,192 works, 16,384 submissions and 8 GiB. The source
retains original jobs, global sequences and duplicate checks. A private warm
memo avoids repeating a previously verified retained nonce's pure proof hash;
it does not cache canonicality, expiry, session authority or credit. Cold reopen
still audits the original retained work. Daily rounds retain the existing exact
allocation and confirmation rules under explicit Version2 limits.

The daily settlement journal stores immutable signed batch bodies separately
from delivery state, with indexed credits, reservations, consumed inputs and
daily slots. Full source and namespace audits precede financial disclosure or
mutation. A single original Store retains the backend and signing capability.
Recovery resolves an existing signed batch before importing new credit or
preparing another payment. The selected lifetime ceilings are 16,384 credits
and 32,767 batches, with an 8 GiB logical journal limit. These are not rolling
page limits or an unlimited-history claim. Creation is fresh-only: no V1 root
is adopted, rewritten or repinned.

The shared `pooltxshape` calculator bounds native P2WPKH transactions by the
existing 400,000-weight standard limit. Daily signing permits at most 1,464
inputs and 256 payees, further constrained by the complete transaction shape.
The original wallet receives only the original Store's active, exact-request
capability. It uses a cheap lifetime check per input and repeats the complete
immutable request check before disclosing signed bytes.

`poolsettlenode.NewDaily` owns the entire funding request before network work,
uses at most four joined RPC workers, and requires an unchanged exact tip before
returning a complete result. It can inspect the full bounded funding frontier,
including carried change with more than 256 origins. Aggregate claims and
unique origins are each bounded at 65,536; signing has its separate tighter
shape and request limits. No evidence cache or longer operation timeout is
introduced. Legacy node/signing paths retain 256-input and 100,000-byte limits.

D84 remains whole-payout deferral. The daily ledger verifies the complete ready
set and visibly reports `transaction-capacity` if it cannot fit. It selects no
partial subset, signs nothing, adds no fee and leaves the daily slot unused.
The explicit daily scheduler retains one original Store, recovers pending work
first, and advances its UTC cursor only on a completed daily result. The 99%/1%
allocation, operator-funded fees, 1 SLUG minimum, 60-descendant earnings lock,
100-block maturity and no-reserve-advance rules are unchanged.

## Evidence available

The candidate compiler/profile qualification passed normal and race tests,
cheap-profile refusals and vet. The complete ledger and scheduler suites passed
71 top-level tests per mode (32.9 seconds normal, 218.1 seconds race). Component
qualification also passed the complete source/round suites, original source
fences, signing capabilities, legacy and daily wallet controls, and daily node
evidence controls. Recorded sources remained unchanged.

Closeout also passed 17 module/build targets and the full `pooloperator` suite
(37.030 seconds normal, 242.281 seconds race), vet, formatting and diff checks;
all 7,388 captured source inputs stayed unchanged and process groups joined.
A normal 256 MiB PoW smoke run synced the current seed at height 61, verified
the report-only height-1 checkpoint with 60 descendants and restarted cleanly.
Only its two isolated pre-connection blocks were mined; no public-chain mining
or pool deployment is implied.

The source test with 721 original jobs proves job retention and reopen behavior;
it is not 721 accepted blocks. Private-leaf wallet tests exercise 721 signatures
and script checks with controlled inputs; they do not substitute for the
original-ledger-to-wallet actual proof. A concrete two-winner integration test
covers pending A before broadcast, complete original-owner reopen, recovery
without new import or signing, exactly one later credit, the same-day occupied
slot, next-day carry and unchanged A bytes. Node observations and its backend
are explicitly controlled fixtures.

Independent Python inputs were frozen before expected outputs. Public source,
round, transaction-shape, wallet-binding, signing, indexed-ledger, scheduler and
64-gym actual references are retained with their recipes. Their exact SHA256
identities and the focused qualification reports are listed in
[the evidence index](docs/evidence/set76/reference-identities.json).

**Actual attempt3 passed in 1,116.386 seconds within its in-process scope.**
It produced 721 original canonical rewards and paid 64 fixture gyms through
one 721-input second payout: 7,128 SLUG, 109,086 serialized bytes and a
51,236-atom fee. The two original A/B broadcasts, complete economic bytes,
accepted lost-response recovery, all balances and one change with 721 origins
passed; both nodes and all original owners joined cleanly.

The owner reopen stayed in the same test process. Its 0.2645-second source
reopen benefited from `wire.BlockHash`'s 4,096-header process cache; it is not
fresh-process Argon2 replay evidence. The passed result and separate scope
clarification are preserved.

**Fresh-process attempt4 passed in 1,255.733 seconds.** After all original
financial owners closed, one new process opened the same roots and fully audited
the original source in 142.534 seconds. It recovered and confirmed B without a
third broadcast, preserved both reserved economic bodies, and verified every
balance and all 721 change origins. B paid 7,128 SLUG to 64 fixture gyms using
721 inputs, 109,102 serialized bytes and a 51,236-atom fee. The child and both
nodes joined cleanly. All 2,531 captured inputs and profile artifacts remained
unchanged. The public reports bind the child identity, original deadline,
handoff and final financial evidence. Exact-tip GitHub qualification is tracked
separately from these completed local checks.
Only settlement bookkeeping uses a test-only day selector; neither actual
qualifies a real-day scheduler or public service. Winning jobs and 64 payees do
not model continuous mining by 64 miners or ordinary share/refresh traffic.

Attempt1 stopped at an unused test import; attempt2 omitted standalone `bdb`
registration and stopped before wallet DB creation or payout. Both failures
remain preserved; their test-only import corrections changed no product guard.
The synthetic fixture trees remain retained; only public evidence is publishable.

## Remaining gates

The daily profile still needs explicit public transport/client, operator setup,
ingress and native attachment integration; V1 interfaces must not silently
adopt its new source identity or sequence range. Signed admission/ownership
publication limits are a separate authority boundary. A V2 full-intake late
winner, original-job restart and unchanged pending-payout capacity proof remains
required; Set74's V1 proof is preserved, not relabeled. Larger history also
needs operational retention, disk-full and bounded-reopen qualification.

Exact final CI, public host operation and real scheduled payout acceptance
remain separate gates. No real operator or official checkpoint private
key was accessed, no old custody root migrated, and no deployment is claimed.
Sign-in/onboarding remains after public pool work. D74's full joint product
organization/design review remains last.
