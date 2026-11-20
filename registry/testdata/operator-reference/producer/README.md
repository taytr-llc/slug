# Set74 independent authority-profile corpus — unexecuted source

This fixed producer prepares the approved public pin v2 boundary without using
Go-derived expected outputs. The copied policy defines unchanged ordered pin
fields/digest and v1 statement grammar, new explicit v2 selection, known-fixture
exclusions and fresh V2 guards. No new byte choices are inferred from current Go
implementation drafts. Runtime signer method names are outside this corpus.

`inputs/inherited/crypto.py` is byte-identical to the reviewed Set45 independent
integer Ed25519/RFC8032 primitive source (SHA256
`ea87b9021e14712b1ac7130daa73cd12fb8acf2fd8995ca5ce9e32d0d770fd1c`).
Its known-answer selftest runs before generation. The unchanged Set73 model
supplies canonical encoding/domain arithmetic and conditional state recipes.
The copied approved source catalog/lock record provenance. The old portable
public-core corpus is itself independently generated; only its legacy expected
pin/admission bytes, public literals and modeled recipient point are reused.
No new Go code, executable or product output is imported or invoked.

The original sequence-one records are `v1/zero256-unawarded` and
`v2/zero256-awarded`, selected by the explicit recipe mapping. Both retain every
record byte, source ID, descriptor and sequence. Three variants exercise fixture
v1, four distinct synthetic v2 role keys and explicitly shared online v2 keys.
The source labels `actual/v1` and `actual/v2` denote inherited descriptor
families, not authority pin versions. New signatures are synthetic wrappers,
not claims that those old authorities actually published these statements.

The exact five excluded key recipes are in `inputs/recipe.json`: seed73 repeated
32 times; and SHA256 of the ASCII domains `SLUG candidate seed declaration
fixture`, `SLUG candidate seed checkpoint fixture`, `SLUG candidate result
statement fixture`, `SLUG candidate result checkpoint fixture`, each followed
by NUL, `v1`, NUL. Recipes are compared to copied source provenance. Synthetic
role seeds use SHA256 of `SLUG set74 public synthetic operator ` plus the role
(`admission`, `publication`, `cutoff`, `checkpoint`) plus NUL, `v1`, NUL. These
are visibly published fixtures, not secrets or operational custody. Public keys
will be derived independently only after execution approval; none is derived
at source-preparation time.

The official checkpoint private seed remains on the Mac, unread by root and
never exported to the pool host. The online signer has admission/publication/
cutoff keys only. Pure checkpoint vectors qualify byte/key separation; they do
not authorize online official signing. See the explicit copied offline policy.

`case-plan.json` fixes 54 positive objects, 102 refusals, 12 canonical/raw cases,
12 guard projections and four checkpoint cross-profile refusals. Guard rows are
codec projections, not a writable-open/downgrade test. Signed frame/cutoff
composition is conditional modeled evidence; actual source lifetime, normal
PoW, canonical/reorg/finality, storage atomicity, key loading and cleanup still
require their separate product qualification. The profile delta validator is
not an exhaustive general schema or normal-work validator.

The frozen executable source set is `authority_model.py`,
`generate_reference.py`, `case-plan.json`, `README.md`. `input-catalog.json`
binds exact immutable copied input membership/size/hash, including policy and
provenance-only Go source. A root-approved lock must bind this catalog and all
four source hashes before imports or key derivation. The current proposed lock
has `execution_authorized:false`; it is deliberately not runnable approval.

After root review only, the intended finite invocation is:

```sh
python3 generate_reference.py --execute \
  --approved-lock producer-lock.approved.json \
  --approved-lock-sha256 EXACT_APPROVED_LOCK_SHA256 \
  --output /Users/taytrrip/slug/artifacts/set74/authority-reference-attempt1
```

Use the same approved lock under Python3.14 and3.9 into separate fresh output
directories and require byte-identical `authority-v2.json`. Do not overwrite a
failed attempt or adjust a case during execution. The producer rechecks source
and inputs after generation, caps the corpus at2MiB and checks a120s deadline
between finite cases. No subprocess, network, Go, node or Argon work occurs.
An external runner should enforce a hard120s process timeout as well; the inner
deadline alone cannot interrupt a stuck primitive. A failure after creating the
exclusive output directory retains a failed report. Pre-admission hash failures
produce no output and must be captured by the invoking reviewed runner.

No producer, primitive selftest, import, public-key derivation, build or Go test
was executed during the original source preparation. Root's first approved
Python3.14 attempt then failed at the sequence-one input assertion, before new
corpus output: the mistakenly selected `v2/zero256-unawarded` record has integer
sequence2, not a string. The complete predecessor sources, inputs, approved lock
and failed report are retained in `pre-sequence-selector/`. This successor only
selects the already qualified sequence-one v2 awarded record and uses its exact
legacy case name for byte comparisons and conditional recipient lookup. No
record rewrite, type coercion, case-count, formula, schema or crypto change is
made. The successor is unexecuted and requires its own root-approved lock.
Files remain outside the checkout.
