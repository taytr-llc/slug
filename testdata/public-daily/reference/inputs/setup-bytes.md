# Operator setup v1 bytes

Freeze before the new Go codec. Canonical compact UTF-8 JSON, exact ordered
members, integer decimal tokens, no unknown/missing/duplicate fields, no alternate
escaping, insignificant whitespace or trailing data. Public setup is at most
262144 bytes. The existing nested codecs retain their own stricter bounds.

`Setup` order: `version` (1), `runtime_id`, `authority` (full operator V2 pin),
`ownership_store_id`, `pool` (poolshare.Config), `round_limits` (poolround.Limits),
`settlement` (poolsettle.Config), `schedule` (poolschedule.Config), `miners`
(poolminer.Config), `wallet_guard` (waddrmgr.PoolSettlementGuard), `node`
(poolsettlenode.Config), `ingress` (poolingress.Config),
`maintenance_interval_ms`, `unlock_lifetime_ms`, `unlock_renewal_ms`.

All selected identities are nonzero lowercase32-byte hex. Authority source must
equal the pool AuthorityPin. Existing pool binding/config codecs validate the
pool and settlement. The miner binding is derived from the same full profile,
source and settlement; its RuntimeID must equal Setup.RuntimeID. Wallet guard
profile/source/settlement digests must match those originals. Pool payout script,
settlement change, wallet funding and node funding key/script must agree exactly;
node and wallet chain must equal the profile source chain. No endpoint or marker
can select those values for the caller.

Round limits retain version1 and 4096..1GiB byte bounds, with MaxRounds equal
to settlement.MaxRounds and at least pool.MaxWorks. This finite runtime does not
admit more job windows than it can represent as rounds. Schedule retains version1,
nonzero SchedulerID, exact RuntimeID and the existing full settlement/source
binding digest; UTC minute0..1439 and RetryMS1000..3600000. Maintenance interval
is1000..60000ms. Unlock lifetime is60000..86400000ms; explicit renewal interval
is1000..3600000ms and at most half the lifetime. Renewal is a separate local
custody operation on the originally selected protected passphrase file; Sign
never reads it or unlocks. Failure locks/defers payments visibly.

Ingress retains its pure config contract. The three private loopback origins must
be distinct because this runtime owns three separate listeners. Public Listen
port must be nonzero; port0 remains useful only in ingress unit tests, not durable
operator setup. No fixture/private input is implicitly selected.

H(label,raw)=SHA256(ASCII label||NUL||ASCII v1||NUL||LE32(len(raw))||raw).
SetupID=H(`SLUG pool operator setup`,canonical Setup). The intent file is
`SLUG-POOL-OPERATOR-V1.json`: canonical Setup plus exactly one LF. READY file
`SLUG-POOL-OPERATOR-READY-V1.json` contains ordered
`version`=1,`runtime_id`,`setup_id`, plus exactly one LF. READY is written
exclusively after exact child construction and backend binding; it is not a
current chain/funding/ownership readiness claim. Existing child journal formats
are unchanged. A future transport/config change requires an explicit qualified
reconfiguration path; hand-editing a marker is not one.

Independent setup examples combine the already-frozen public operator authority,
wallet16 source/guard and public gym identity fixtures. Source/profile/settlement
bindings are recomputed from the specified byte recipes after intentional test
composition; no production Go output supplies expected digests. These examples
do not grant custody or prove a funded runtime. Raw refusal cases and independent
cross-field mismatches precede Go implementation.
