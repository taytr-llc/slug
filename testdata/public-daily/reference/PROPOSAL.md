# Set79 independent reference producer proposal

This is a source-only proposal. It contains no generated expected values and
does not authorize producer execution. The service lane owns the final byte/API
contract. This proposal binds root selection `root-contract-selection-v4.json`
SHA256 `e6533d4af294a0cd4f1baf4471c2f7cc1cc5db92f18aaf67a251a519dcc78a08`.
Root authorized external producer authoring; the resulting source, finite named case
inventory/counts and output bound must then be frozen in a separate root-approved
producer lock before either Python interpreter runs.

The reviewed contract is `public-daily-contract.md` SHA256
`a6a30ab580f6463d30bd29c00a4c60d7d7b32e6d6d70560b761a7661cbd0da05`,
with test plan `3896b45cb46aab155e90f54a67c1d5a5083cbbe9b4b8a3cd3217deb90aeb1e61`.
Every work/receipt success binds settlement/payee before disclosure. The DAILY
outbox durably holds unknown Issue across restart; no new idempotency or recovery
identity is inferred. These final corrections are inputs to this proposal.

## Inputs and independence

Use immutable copies of already qualified **public independent** corpora:

- Original provision Policy1/Setup1/public material/certificate examples and
  their byte recipes. Select two published gym identities and a funding key
  whose native change script differs from every selected payee. No fresh random
  identity, user file, wallet/database, key loading or checkpoint secret.
- Existing Source2/daily wallet and scheduler references, Set78 finite capacity
  reference, and the two-gym Set78 actual reference. These anchor unchanged
  component IDs/guards/capacity; new public configuration is recomposed from the
  selected contract rather than copied from a product serialization.
- Original V1 service identity input/oracle, credential input/oracle, independent
  pool job/template/nonce/proof corpus, and original admission-record corpus.
  Carry selected old embedded values byte-for-byte; no new PoW, admission,
  signature, canonical block or payment authority is inferred.
- Existing payout-history reference and its independent Python arithmetic/byte
  recipes. Retain original signed payout examples unchanged. Larger paged
  histories are expressly conditional serialization/arithmetic projections,
  not manufactured signed transactions or observed financial history.
- Original remote-attachment corpus, its byte recipe and literal published
  recipient/CA inputs. Preserve each `ExpectedLocal` Version1/schema14 object,
  old allocation/recipient/source marker identity and policy byte-for-byte while
  composing a separate remote Source2/Settlement2/Miners2. Use the already
  corrected separate operator change script; never rewrite local history to
  avoid a payee/change collision.

The input catalog records exact source file identities without importing or
executing them. After review, inputs are copied once into the new reference
directory, preserving original relative helper dependencies where any helper is
used. Prefer a self-contained standard-library producer: old helper sources are
then documentary inputs, not modules imported or executed by the producer.
Product Go files may explain the contract but never supply expected IDs,
bytes, classifications, balances or page results. No old corpus is rewritten.

## Derivation families

For new framed digests, `F(label,mode,body)` means SHA256 of the ASCII label,
NUL, ASCII mode, NUL, LE32 byte length, then the exact body. Core JSON has no
final LF; file LF is included only in a file digest expressly defined that way.
The final recipe must pin this inventory, without replacing an inherited domain:

| Result | Independent preimage recipe |
| --- | --- |
| Policy2 / Setup2 | `F` with existing provision-policy / operator-setup labels, mode `v2`, ordered canonical body |
| Seven namespaces | Existing provision-identity label, mode `v2`, decoded request-ID32 followed by one literal role name |
| Source2 ID | Existing `SLUG pool share daily source` + NUL `v2` NUL + canonical Source2; **no length prefix** |
| Settlement2 ID | Existing operator-pool-settlement-config label, mode `v2`, framed canonical Config2 |
| Scheduler binding | Existing pool-settlement-binding label, mode `v2`, framed exact ordered `{"config":Config2,"source":Source2}` |
| Profile / schema16 guard | Unchanged inherited recipes and versions, with recomposed selected values; verify against old public anchors |
| Credential2 IDs/hashes | Existing enrollment / credential / token-reuse labels, mode `v2`, their unchanged ordered bodies |
| Daily pool trust | Unframed daily transport domain followed by ASCII manifest trust, SourceID, gym key, SettlementConfigID and recipient script |
| Admission trust | Unchanged manifest recipe over exact origin/version/ordered unique public CA DER digests; token excluded |
| Remote attachment2 ID | Existing remote-pool-attachment label, mode `v2`, framed canonical complete remote config |
| Remote READY intent digest | Plain SHA256 of the entire exact INTENT file, including its one LF |

Existing AuthorityPin and embedded jobs/receipts/signatures are reused literally
under their inherited conditional validity. Composed examples must be internally
compatible; an unrelated old proof must not be relabeled as issued by the new
operator. When a wire-serialization projection carries an inherited original
proof with a separately modeled wrapper, label that condition explicitly and
do not use it as a concrete admission fixture.

1. **Fresh composition.** From explicit Policy2 and published PublicMaterial/
   Certificate projections, derive the seven independently named namespace IDs,
   unchanged OperatorProfile2/public-pin identity, exact Source2 and Config2 IDs,
   daily scheduler binding, daily schema16 guard, credential2 ID, Setup2 ID,
   intent/READY and supported provision metadata. Frame every digest preimage
   exactly as the frozen contract specifies. Emit both preimage hex and digest,
   canonical core/file bytes and byte counts. No ID is treated as a capability.

2. **Credential mode.** Derive ordered config/initial state and selected
   rotate/revoke states for both fixed audiences from public literal tokens.
   Emit credential/reuse hash preimages and hashes with the new selected domain.
   Preserve exact original revision, clock, generation, lifetime, ordering and
   retained-history constraints. This models bytes and state predicates; actual
   session gates and cancellation remain concrete integration tests.

3. **Transport identity and envelopes.** Independently derive the unchanged
   admission transport trust from literal loopback and public synthetic CA
   inputs, then the new daily pool trust including full Source2 identity, gym
   public key, **SettlementConfigID and RecipientScript**. Daily client config is
   a separate type; V1 remains unchanged. Use immutable original embedded work/
   job/Submission data for modeled V2 envelope/cross-mode checks. Token rotation
   must preserve public trust; selected endpoint/CA/source/gym/settlement/payee
   changes must change it or refuse. No network or certificate private input.

4. **Daily worker/outbox selection.** Independently encode fresh complete
   config/marker/meta and representative scan/entry bytes, preserving nested
   manifest, proof, nonce and receipt encodings. Daily reopen compares the entire
   selected config, including record/byte limits. V1's existing limit-handling
   behavior is not changed. Model reserved/retained counters only from the frozen
   recipe; do not claim durability, hashing, child lifecycle or recovery from
   these bytes alone.

   Root selected a daily-only appended metadata `issue_pending` bool. Model the
   synced pre-Issue intent, same-attempt exact definite `busy`/`rate_limited`
   clearing, and atomic validated scan+intent-clear transition separately. Unknown/cancel/crash or
   reopened intent stays held against new issuance; previously saved pending
   submissions remain recoverable. Required field order is existing
   `application,version,config,last_sequence,records,bytes,pending,reserved`
   followed by `issue_pending`; true forbids a saved scan/cursor. The active
   attempt is private and never serialized. Its daily public projection is the
   existing paused/outcome_unknown pair. Lock release and original-attempt
   rechecks across actual network calls require concrete lifecycle tests.

5. **Daily balances and captured payout pages.** Use exact Fraction arithmetic
   and checked integer counters. Construct conditional global batch slots and
   select at most eight descending slots, filtering the selected gym afterward.
   Derive captured revision/high-water/next/done and the independent full dated
   balance. Empty matching items need not mean EOF. Paid/reserved wire values
   are canonical nonnegative decimal strings within int64; each payment retains
   its separate original monetary bound. No page sum substitutes for lifetime
   balances. Include exact bytes for zero report, empty ledger, sparse pages,
   pending payment, more than366 global batches and int64 boundaries.

6. **Original local signer selection.** Derive RemotePoolConfig2/attachment ID,
   exact INTENT/READY files and separate two-audience credential-file bytes from
   the unchanged original local binding and selected remote daily composition.
   Derive the daily client/worker configs from that complete configuration. Emit
   the preserved old-source ID alongside the distinct remote daily-source ID.
   These are conditional public cross-binding vectors: an equal-looking foreign
   session, original Mining lease, vault epoch, saved candidate, actual protected
   descriptors and joined Close must still be tested with concrete owners.

## Required positive and refusal matrix

Freeze named cases, not random fuzzing or values obtained by asking Go:

- At least one complete two-gym fresh daily composition; same public material
  with a different request ID; same identity with independently changed selected
  limits/timing; explicit loopback and public-CA TLS trust. Preserve a literal
  V1 compatibility anchor for each reused domain/grammar.
- Cross-mode Policy/Setup/credential/client/server/outbox refusal before any
  authority claim; exact marker/READY/runtime/source/config/guard mismatch;
  payee/change collision; enrollment order/duplicates/wrong gym or key; malformed
  network/genesis/profile/checkpoint public key; invalid selected finite limits.
- Daily trust changes for settlement ID and recipient script; mismatched
  balance/page source/config/gym/payee; unchanged trust on token rotation;
  distinct pool/admission token audiences and retained token reuse refusal.
- Work and receipt responses bind the independently selected settlement/payee
  before any Balance call. Their order is `version,binding_id,settlement_config_id,
  recipient_script,work` or the same prefix followed by `submission`.
  Unknown Issue must not trigger another issuance in DAILY mode. Freeze that
  transition against the final contract without inventing a recoverable job ID
  or treating a serialization vector as evidence of process durability.
- Daily issue-intent cases: empty false; durable intent true with no scan;
  unchanged intent after unknown, cancellation, crash and reopen; same-attempt
  definite busy/rate_limited clear; validated stored scan plus clear; retained pending
  receipt delivery while intent remains true. Refuse missing/null/numeric intent,
  intent-plus-active-scan, and clear from a foreign attempt or stale success.
  Only schema and conditional state-machine expectations belong to this corpus;
  sync ordering, crash injection and original attempt identity are runtime tests.
- Preserve old-local/new-remote attachment positives for both original fixture
  recipients. Refuse substituted OriginalSourceBindingID, policy, schema, local
  recipient, remote payee, marker version or credential audience/config identity.
  Do not model a fixture signing callback as a production attachment.
- Full-config outbox limit increase **and** decrease refusal; marker/meta mode
  mismatch; original job/gym/nonce/header/receipt mismatch; copied V1 outer entry
  refused while unchanged nested evidence remains allowed by the daily wrapper.
- History slots0/1/8/9/16/367; zero matching payments in a nonfinal page; one
  pending original payment; arbitrary valid read cursor without a consumed-page
  claim; exact stale revision/high-water refusal; mixed zero capture fields,
  duplicate/order/cursor/data mismatch, malformed decimal and int64 overflow.
- Whole-history confirmed paid/reserved reconciliation across one unchanged
  capture; per-page sums bounded by the corresponding totals; outstanding
  Confirmed versus lifetime paid distinguished. No D84 subset or new signing
  transition is modeled.
- Raw canonical duplicate/unknown/missing/reordered fields, extra LF/value,
  whitespace policy, escaped aliases, numeric exponent/negative-zero/leading-
  zero variants, null versus required arrays, exact size bounds and one-byte
  overflow. Keep each independent parser's framing rules separate.
  DAILY HTTP alone permits outer SP/TAB/CR/LF inside the original pre-trim size
  bound; config/metadata remain exact and protected credential/marker files keep
  their stated LF rule. Actual observation times retain MAX_TIME, while the
  frame cross-check conservatively overestimates that field at 19 digits.

Before execution the exact counts and output size caps are fixed. Boundaries
which only require a long string/body use explicit literal/repetition recipes;
they do not allocate arbitrary financial state or forge accepted evidence.

## Output and execution proposal

Each refusal records its family, predicate and classification scope. Pure
configuration/raw-byte refusals must not pretend to predict a particular wrapped
Go error. Only explicitly frozen wire codes (including `stale_capture` and the
known/unknown distinction) have an exact public error expectation. Conditional
issue-intent transitions describe original-attempt requirements without treating
strings or recipe identities as a substitute for the concrete original owner.

One versioned public corpus contains source-provenance digests, legacy anchors,
complete composition cases, credential states/hashes, transport preimages and
envelopes, outbox projections, page/balance cases, classified refusals, exact
canonical/raw bytes, original-local/remote attachment projections and byte-count
summaries. Each projection labels which predicate is conditional and which
unchanged independent input it carries.

The producer is standard-library Python only (json/hashlib/struct/fractions/
base64/datetime plus explicitly copied independent helpers if needed). It may
not invoke Go, shell commands, network, randomness, a private file, key service,
wallet or product database. It checks every approved source/input hash before
and after evaluation; writes one fresh exclusive output; and retains failed
attempts instead of replacing them. Root will run the two existing Python3.14
and3.9 interpreters separately under fixed elapsed/log/output bounds and require
byte-identical outputs. No expected output is generated by this proposal.

## Serialization-only successor after first consumer check

Preserve qualified output `0ee0cfc5946987e7f2f1d739fccfda3735591863c8ad8e769edea52f19d8de5a`
and the full original reference tree. Existing source declarations define GBT
maxtime before mintime, default_witness_commitment before long-poll/target fields,
and each transaction data/hash/txid/depends/fee/sigops/weight. The old independent
fixture input dictionaries have the same values but different member ordering.
A literal copied field-order recipe now constructs the existing canonical nested
wire shape before wrappers/scans/entries. It asserts complete decoded equality,
unchanged original job/context order, known field sets and present optional fields
consistent with existing omitempty. It derives no crypto/hash/proof/value from Go
output. All case inventories, limits, domains, money and expiry semantics remain.
