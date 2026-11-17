# Change set 37 — pinned candidate manifest authority

Branch: `slug-manifest-authority`, stacked on set36 implementation
`e1e5ebff19e4aeb2c92fae368dd007419c59d05e`.

## What changed

A separate schema-5 `registry.ManifestStore` binds candidate appearance to
durable issuance and accepted work. Creation generates a public authority
namespace and pins the exact compiled descriptor. Opening requires an explicit
trusted pin that agrees with both marker and metadata. Unsigned inspection
helps recover a lost creation acknowledgment, but cannot authenticate the
authority or silently become its own trusted pin. Incomplete directories stay
intact and fail closed.

Manifest-job admission verifies the actual issued job, gym authorization and
production proof before generating static manifest and portrait digests. One
synchronous receipt batch includes sequence, original issuance-based birth,
D69 eligibility and the static binding. The final D68 clock check follows
generation/rendering and the trusted node guard. Write uncertainty poisons the
handle until explicit close/reopen; an error does not prove rollback.

Historical duplicate recovery returns the first canonical receipt without
renewing birth, eligibility or appearance. Verified Get/List rehash production
work and regenerate the original static binding. Pages contain at most eight
receipts and eight serial Argon operations. Opening performs structural and
cheap binding checks; it does not claim full work verification. No public API
turns arbitrary caller receipt JSON into a trusted admission.

`MANIFEST_ADMISSION.md` preserves the approved exact contract verbatim (SHA256
`d51178db1decf13ad09986802bb3eb7861827d372a7f35e99c13adc739420f27`).
Its proposal status records provenance; implementation and validation status
are maintained here and in the workspace evidence.

## Validation

The frozen independent Python corpus contains five exact receipts, two issued
jobs, witness and block-winning work, alternate valid signatures, 170 encoding
cases and 26 receipt mutations. It verifies the largest supported proof
(16,384-byte coinbase and 20 siblings) and original source inputs. Regeneration
passed on Python 3.9 and 3.14 and again from the integrated tree. Expected
bytes were not derived from the Go implementation.

All ten workspace modules build. Normal, race and cheap-profile refusal tests
pass for the new registry and existing service/worker compatibility; staged
checks also cover descriptor/job/candidate dependencies. Nine actual process
kills recover cleanly, uncertain writes poison the handle, page limits and
corrupt continuation fail closed, and storage-parser fuzzing completed 34,513
executions. Vet, full-tree formatting and whitespace checks pass. All 4,016
captured stage hashes and all 55 integrated source/input hashes stayed fixed;
no previously archived runtime file changed.

A 344.821-second, two-node isolated normal-PoW acceptance run created three
receipts from one real five-minute job. It covered multiple prospects,
broadcast-before-admission of an exact canonical block winner, stale/side-chain
refusal, independent full binding checks, administrative node reorganization,
real wall-clock expiry, and offline reopen/duplicate/Get/List recovery.
Original receipt, birth and static binding stayed identical. The expired new
canonical winner was rejected. Both owned nodes exited zero and all three
listeners closed. The administrative reorg does not claim a natural
heaviest-chain fork experiment. No public-chain work was mined.

Workspace evidence: `set37-stage-checks.json`, `set37-checks.json`,
`set37-integrated-reference-check.json`, `set37-authority-acceptance.json`,
and the independent review. Publication additionally requires the unchanged
per-set normal-PoW smoke, frozen release binaries and a fresh public height-61
preflight. Deployment/observation results are recorded separately.

## Preserved behavior and remaining integration

Existing schema-1–4 stores, jobs, services, workers and receipts retain their
contracts. There is no silent migration, official generator activation,
account/session service, new CLI, desktop route or public receipt import.
The authority namespace is not a signature. Coherent administrator rewrites,
whole-store rollback and issuer clock honesty remain outside local integrity
guarantees. Future authenticated callers must enforce access on historical
disclosure as well as new admission.

Admission history identifies the original gym; current ownership, transfers,
ten-slot roster selection, XP/fights and public portable provenance still need
their explicit authorities. The Mac pilot remains the existing fixture build.
Mike's portrait rendering check remains pending, actual screen-reader acceptance
is open, and D74 still requires a thorough joint review before product release.
