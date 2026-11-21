# Explicit daily round journal (selected engineering contract)

The new constructors CreateDaily/OpenDaily/OpenDailyAudit accept a nonnil live
context, Limits.Version=2 and the original concrete poolshare Config.Version=2
owner. They require independently selected positive MaxRounds<=16384 and
4096<=MaxBytes<=8GiB. Existing constructors remain strict Limits1/source1 and
retain all their old bounds. Roots are fresh; no migration or fallback occurs.

The journal uses existing canonical ordered marker and meta grammar, with the
outer version equal to Limits.Version. Config/source equality is exact. Global
r+blockhash and i+big-endian uint64 round-sequence keys do not restart. Immutable
Round.Version1, receipt.Version1, allocation arithmetic, record encoding and
prefix hashing stay byte-identical for identical inputs. The accepted prefix
initial digest includes the complete Config2 JSON under the original prefix
hash domain; this is an explicit selected config, never an old source identity.

Lifetime accepted-prefix derivation may cover16384 rows for source2. Each round
or open share window retains its4096-share bound. Derivation still authenticates
the entire fixed prefix and all duplicate/ordering guards. If a new window
exceeds4096, Sync completes source and existing-key verification before returning
ErrCapacity with no commit. It does not close a round or reuse shares to escape
the bound. A saved consumed prefix violating that bound is corrupt. Existing
V1 derivation/limits remain unchanged. This finite slice does not promise
unlimited dry spells or perpetual source availability.

Caller contexts pass through constructor source inspection and complete reopen
audit; no new Background or deadline increase. Failed/canceled opens close and
join the opened DB. A close failure returns the partial concrete owner plus the
error for cleanup. New APIs never make node observations during open. Profile
refusal happens before filesystem/source effects. Existing methods keep their
original owner, gate, observer, cancellation and atomic sync semantics.

Independent references use the public source-config corpus already copied by
the sourceV2 stage. A standalone Python producer derives exact marker/meta bytes,
initial prefix and index-key cases (255,256,257,720,721,4096,4097,16384). Expected
conditional economics tests may use symbolic authenticated-input assumptions
only; they must not be described as real mined source evidence. Concrete source
integration uses the sourceV2 original issuer/reopen tests and separate later
normal-PoW composed acceptance. Existing V1 accounting vectors remain unchanged.
