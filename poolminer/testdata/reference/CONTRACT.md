# Explicit operator miner credentials v1

This local operator journal authorizes only pre-enrolled gym mining against one
original configured source. It is not provider login, MFA, account recovery,
wallet access, a fighter transfer signer, or proof of public deployment.

Public `Config` fields, in canonical JSON order: `version` (1), `credential_id`,
`runtime_id`, `operator_profile_id`, `source_id`, `settlement_config_id`,
`enrollments`. Every ID is nonzero lowercase 32-byte hex. An enrollment is
`public_key` (canonical compressed secp256k1), `gym_id`, `payee_script` (native
0014 +20 bytes). Enrollments number 1..64, strictly sorted by gym_id, with unique
keys and scripts. They must equal the complete settlement destination list.

`Binding` carries the independently retained `registry.OperatorProfile`, complete
`poolshare.Config` Source and `poolsettle.Config` Settlement. Pure
`ValidateConfig(Config,Binding)` verifies all three IDs using the existing codecs,
operator Source equals pool AuthorityPin, each derived GymID matches its original
key/network/genesis, and payees exactly match. Pure `ConfigID(Config,Binding)`
adds no owner authority. Create/Open repeat validation; they never learn expected
configuration from the marker. ConfigID is H("SLUG pool miner enrollment",config).
H(domain,value) = SHA256(ASCII domain || NUL || "v1" || NUL || LE32(JSON size) ||
canonical compact ASCII JSON). Config and marker are at most 65536 bytes; marker
is exact canonical Config plus one LF, named SLUG-POOL-MINERS.json.

`Create/Open(ctx,path,expected,binding)` retain one original exclusive, private
0700 root and its db/0600 marker descriptors. Established missing files fail
closed. Standard LevelDB and the existing trusted operator-directory model apply;
this does not claim isolation from a hostile same-user local writer. Exact
replacement is detected and poisons the live owner. No secret plaintext is stored.

`PoolSessions()` and `AdmissionSessions()` are concrete SessionAuthorizer adapters
for exactly audiences `pool` and `admission`. `Rotate(ctx,gymID,audience,
expectedGeneration,tokenHex,expiresAtMS)` explicitly issues/replaces one token. Newly issued lifetime is
at most 30 days (2592000000 ms); exact replay never extends issuance or expiry.
`Revoke(ctx,gymID,audience,expectedGeneration)` disables that generation. No
network request can enroll, rotate, revoke, change key, or redirect payees.
Tokens are canonical 32-byte nonzero hex, supplied privately by the operator.
Each audience has its own credential. A plaintext token cannot be reused in any
audience/gym in this journal, including after revocation/restart. Nothing returns
a token. Snapshot contains only public enrollment and hash/status metadata.

TokenHash = H("SLUG pool miner credential", {config_id,audience,token}); ReuseHash
= H("SLUG pool miner token reuse", {config_id,token}). These transient JSON values
contain the token, are never persisted/logged, and only their digest is retained.
The config_id scopes both to the exact enrollment. Use constant-time hash matches.

One LevelDB key `state` stores ordered fields `version,config,revision,clock_ms,
credentials,history`. Revision is 1..4294967295. Clock is 1..253402300799999 Unix
milliseconds. Credentials (<=128) sorted gym_id then audience contain ordered
`gym_id,audience,generation,token_hash,reuse_hash,issued_at_ms,expires_at_ms,
revoked_at_ms`; generation 1..4096; revoked_at is zero or issued..clock; expiry
is issued+1..min(issued+2592000000,maximum). Issued<=clock, expiry may already have passed. History
(<=4096) records ordered `token_hash,reuse_hash`, in issuance order, unique in
both hashes. Every current credential's pair occurs exactly once in history.
The sum of current generations equals history length. No history is evicted. Revision/generation/history exhaustion refuses before work.
State <=1048576 bytes; canonical parser limits depth8/tokens65536/object fields16/
array4096/string128 and rejects duplicate/unknown/reordered/missing fields,
non-ASCII/alternate numeric spellings/trailing bytes. Null arrays are refused.

Rotate uses exact generation CAS (zero for first issuance). An exact lost-response
retry with current generation=expected+1 and exact hash/expiry returns the saved
credential without another rotation. A revoked generation cannot replay into
active state. A repeated exact revoke returns its saved revoked credential.
Old generations, other payloads and any previously used token refuse. Revocation
retains its generation; subsequent fresh rotation increments it.

One nonwaiting original operation gate serializes session callbacks with local
rotation/revocation; contention returns Busy, not a different token/owner. A
session validates the original token and audience, holds the gate through fn,
and exposes only its original public key and an active-only recheck. Recheck
serializes its own concurrent calls, checks original parent context, Close,
poison, current expiry and clock, and syncs any increased clock high-water before
granting authority. Invalid token attempts perform no writes. Clock rollback
refuses. Recheck cannot survive callback return (including panic); callback work
and any concurrent in-flight recheck are joined before releasing the gate. Final
disclosure is rechecked; a failed post-callback check remains an error for the
existing mininglease outcome-unknown classification. No arbitrary errors retry.

Close marks the owner closing immediately, invalidates rechecks, and joins the
original callback before closing DB and descriptors. Its caller context bounds
the join; timeout retains ownership/lock for a later Close. Actual resource-close
errors remain sticky and retain failed owners. Constructor cleanup failure is a
typed retained cleanup owner. No borrowed public source/settlement is closed.
Mutations sync before disclosure; any uncertain write/sync poisons the owner and
returns a static outcome-unknown error. Reopen audits the complete sole state and
its exact marker; fresh identities never replace a missing established journal.

Independent model covers grammar/digests/state relations only. Tests separately
exercise original lease/revoke/rotation/reopen, canceled and escaped checks,
Close joining, exact lost acknowledgement, poison/capacity/clock, binding and
file replacement. Normal PoW is required to open/use the journal; pure codecs
remain profile-neutral. These are credential-engineering checks; the outer
runtime still owns TLS ingress, actual PoW, admission and payout acceptance.
