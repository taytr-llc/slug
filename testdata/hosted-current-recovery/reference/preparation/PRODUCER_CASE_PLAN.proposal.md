# Set71 independent-reference preparation

Status: proposed only, November 19, 2026. The hosted contract is still under
review. No new producer has been executed and no Set71 expected bytes, hashes,
verdicts or signatures have been generated. Repository sources are unchanged.
`input-catalog.proposal.json` inventories 64 inherited public files (2,193,971
bytes) with exact commit/blob or prior-artifact provenance. The copy utility is
staging, not an oracle. The separate `review-snapshot/` is an unfrozen draft
snapshot, not a normative input or implementation authorization.

## Sources and reuse

- Chain parent `0435f59198c27e73621715a162338f38035d303e` supplies the unchanged
  schema2 account/coordinator and verifier codecs, frozen provider contract,
  independent RFC7515/RS256 and coordinated identity reference programs/corpora,
  and the private recovery-file publication contract. The published RFC example
  exponent is public test material; it must never become a real provider key.
- Wallet parent `bfe5db284d769b741220bbd1cbae2c75b8cc6a07` supplies the unchanged
  profile1 canonical manifest/export, independent BIP39/BIP32/mixed49/address
  reference, independent v1 vault AAD/envelope arithmetic, schema9 account-row
  framing, and Set70 public identity reader. Production Go source is a framing
  reference, never an implementation to invoke or an expected-output generator.
- Both current public manifests are exact copies already committed in Set70,
  originally exported by the separately built Set68-source intake before Set69
  implementation. Native84 external/internal cursors are 1/1; all other cursors
  are 0. Public source hashes are zero256
  `f027cb1dfc522b59a6f863ad9921b499d42ea7102fa3d7a17a8153b650be9ed5`
  and incrementing256
  `41776b5beb5b341714a285e62444fceb0cf2a9d408123f05ec1eeeff739db4e6`.
  Public intake reports persisted height110/hash765630b0…a533e, not a fresh chain
  observation. Its export-file hashes are provenance only: no private export or
  database file is staged or opened.
- Existing v1 ciphertext/AAD and official derivation vectors are regression
  anchors, not a reason to issue new cipher formats or regenerate old goldens.
  Final repository packaging should reference unchanged inherited files by
  exact hash and relative path rather than commit another 2.2 MiB duplicate.
  Archive only genuinely new public inputs, producer source, locks and outputs.

## Draft points to settle before source/output freeze

1. Exact hosted INIT/READY/stage-receipt and driver epoch/auth/reset DTO field
   order, storage namespaces, canonical scalar grammar and byte limits. The
   current checkpoint fields are specified; several enclosing encodings are
   still described in prose. Do not infer hashes from a future Go implementation.
2. Exact compiled fixture subject mapping, initial credential epoch, wrapper ID,
   recovery/gym key references and published fixture KEK selection. Existing
   coordinated reference uses different tenant/client/connection strings from
   this draft, so reuse its arithmetic without relabeling its old signed tokens.
   Pin the new constant inputs and exact claim field order before any new JWS.
3. CaptureID and original expected revision recovery across crash versus an
   ephemeral wallet/source lifetime. Lost-ack replay must identify the original
   operation; a stored latest pointer is not proof of that operation. Which
   pending action bytes survive restart must be a frozen store contract.
4. Typed pending-reset lookup containing the original ResetRequest and provider
   operation; startup must allow exact Resume reset without requiring a login
   that the pending barrier rejects. Old FlowStatus alone omits required fields.
5. Account rows physically contain public and encrypted-private spans in one
   value. Freeze the inherited 65,536-byte cap and strict length framing; skip
   the private span without copying/decrypting/retaining it. “No private retention
   key Get” is testable; “no DB value containing private ciphertext is read” is
   not compatible with the existing row format. Imported account checks should
   define the exact bounded index-presence probe and missing-index refusal.
6. Initializer identity adoption must be pinned before accepting generated store
   IDs, with exact crash-point resumption rules. A canonical marker oracle cannot
   demonstrate that an arbitrary nonempty directory is the original initializer.

These are engineering byte/lifetime clarifications, not new owner policy.
After root freezes them, replace this proposal with an exact named-case recipe
and freeze producer/input hashes before the first execution.

## Proposed independent programs

`manifest_checkpoint_model.py`: Python standard library, no Go imports. Ordered
compact UTF8 JSON with inherited manifest field order; no newline; exact decimal
integer grammar; SHA256(manifest bytes), and the draft binding-domain prefix only
if frozen as normative. Emit full canonical bytes plus digests, not just labels.
Use the two old actual 1/1 manifests, independent all-zero default inventories,
and explicitly modeled cursor advances. Validate non-cursor equality to the
original v1 fixture, every branch monotonicity, original-binding equality,
revision/expected-revision, count and clock highwater. Do not compare “current”
claims against arbitrary wall time or infer chain finality.

`provider_model.py`: reuse the old independent explicit PKCS1-v1_5/SHA256/RSA
arithmetic. First verify its published RFC signature and old signed corpus
without rewriting them. Generate only frozen fixture claims with deterministic
public IDs/times. Independently derive verifier config digest, principal/event
binding and raw-event versus derived-barrier identity using the inherited
length-prefix/domain rules. Emit signed transport and normalized public semantic
facts into testdata only; runtime/public logs and IPC must never contain JWTs.

`state_model.py`: deterministic finite model of driver original-auth rows,
credential CAS, account pending/completed reset barrier, checkpoint actions and
current pointers. Inputs explicitly distinguish before-commit, committed/lost
ack and cleanup failure. Outputs are model expectations, not claimed executions
of disk, locks, signatures, accounts or filesystem authority. Keep count/byte
sizing a separate synthetic calculation; it is not an acceptance verdict.

`public_row_model.py`: build synthetic schema9 public account rows independently
from inherited binary framing. Use inert sentinel bytes for encrypted spans;
check public cursor extraction, length/offset overflow and refusal witnesses.
Do not derive valid ciphertext or claim a complete real wallet from these rows.
The implementation tests must also use actual published fixture managers.

## Finite case families to lock before execution

| Family | Proposed cases / independent fact |
| --- | --- |
| Canonical manifests | Both fixtures: unchanged default inventory, original actual1/1, each scope/branch changed in isolation, 4095/4096 edges; exact bytes/hash/root/gym unchanged. Additional values are labeled modeled. |
| Current binding | Exact two fixture bindings; each owner/account/authority/principal/provision/retention/source field crossed independently; generated ID zero and uppercase grammar; wrong ordering/duplicate binding. No session generation in identity. |
| Checkpoint arithmetic | First expected0→revision1; each branch monotonic; same-cursor fresh capture consumes one action; interleaved accounts share128-action cap; exact old capture replay returns old time after later actions; ID reuse with changed intent/expected revision conflicts. |
| Clocks and capacity | Positive/time upper bound; commit before capture; backwards store highwater; revision/counter exhaustion;128th accepted/129th refusal; 4KiB manifest/8KiB row/12KiB batch/2MiB audit sizing. Synthetic maximum sizes do not constitute real valid records. |
| Closed parsers | Missing/extra/duplicate decoded names, aliases, null, invalid UTF8, wrong integer grammar/overflow, reordered canonical bytes, trailing newline; arrays/counts bounded before allocation. Distinguish parser-only cases from already-typed API inputs. |
| Public account rows | Fixed44/49/84/86 coin0, exact mixed49; scope5/account3 refusal witnesses; extra/missing scope/account; imported cursor/key/index presence; 4096/4097; watch-only/profile2/schema12 refusal; truncated/oversized pub/private/name spans and nested-length mismatch. |
| Signed login | Both subjects/epochs, fresh original auth and fresh transport over retained original facts; wrong issuer/audience/kid/type/config/tenant/client/connection/flow/nonce; expired transport; changed original auth/epoch under same operation; no old session-token replay. |
| Signed reset | Begin→pending barrier→one credential CAS→complete→fresh login; exact replay retains verified time/hold; changed EventID/raw-barrier ID/nonce/provider operation/from/to epoch; complete-before-CAS; credential overflow; old epoch rejected; held View/gym signing permitted but export/withdrawal refused. |
| Immutable vault compatibility | Retention/initial ManifestHash unchanged through checkpoint/reset/rewrap; gym-only signing cannot unwrap recovery; current export uses captured Accounts and original entropy/profile; old immutable export remains old. No new envelope format goldens. |

Exact counts/names and all deterministic IDs/times must be reviewed and frozen
before outcomes. No arbitrary “good enough” thresholds or acceptance counts are
implied by this planning table.

## Facts requiring implementation fault-injection, not arithmetic goldens

- Atomic one-View identity+cursor read while another DB write commits; context
  cancellation joins admitted DB work. Bucket spies forbid private-retention
  keys, unbounded ForEach/address scans, KDF/unlock/derive and copy-out aliases.
- Real original token/generation plus epoch fence: reset commits its pending
  barrier while capture/unwrap/signing blocks; no account lease spans that wait;
  no replacement token, current View or fallback local signer rescues old work.
- Missing/double/late/retained/swallowed-error callbacks cannot publish; exact
  local commit may delay reset until sync returns. Do not claim OS disk sync is
  cancelable or preemptible.
- Concrete wallet/source pointer ownership, cross-child close/reopen, actual
  permit ordering and retained cleanup owners. Equal public roots in a different
  restored wallet do not establish installed-source authority.
- Create/write/sync/link/unlink/directory-sync faults, parent inode/path swaps,
  symlinks, existing destination and managed-root overlap. No plaintext temporary
  exists before the final RecoveryExport fence. Post-link errors remain unknown;
  cleanup is joined and sticky uncertainty never deletes a final destination.
- Real process crash at INIT receipt/checkpoint/CAS/reset boundaries, exact
  startup resume, missing established namespace refusal, no hidden miner/export
  on startup, and observable unwrap-purpose counts under held sessions.
- Actual package route and isolated export→file restore→funds/gym/spend prove
  integration. An in-memory manifest comparison or successful file save alone
  cannot prove the file restores. Scratch restoration never installs a replacement
  payment journal or releases permanent signed-input reservations.

## Reproduction and reporting gate

After contract and producer freeze: generate once in an exclusive new attempt,
preserve any failed authoring/run result, reproduce with a second available
Python, then reproduce from the portable repository layout with no workspace
paths. Verify all old inputs/scripts unchanged before and after. No network
install, wallet open, node, PoW, real/provider secret, new Go invocation or live
provider call belongs in this producer. Public source evidence and model outputs
must remain separate from actual fault-injection/integration execution reports.
