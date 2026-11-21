# Public hosted selection checker

`check-selection.py` is source-authored and AST-checked only. It has not been imported or executed. Root controls any execution and must use a fresh report path. It reads only six explicitly named public preparation outputs plus the hash-pinned174293byte Set81 reference. It never reads the private directory, TLS key, credentials, wallet/database or host state, and has no subprocess/network/product-code import.

The public schema is tied to preparer manifest `da1428dbb558fe5a99a968e9eb3878d3ef54c3737832879bffc2aca4926d5afe`: `endpoints.json`, canonical `setup.json`, canonical `remote.json`, `prepared.json`, `ca.pem`, and `tls-cert.pem`. Endpoint and preparation JSON reject duplicate keys, floats and unrecognized fields/order. Canonical Setup/remote bytes are reconstructed independently from the frozen reference, changing only explicitly selected ingress fields, remote Origin/CA, UTC minute and the dependent enclosing IDs. Source, authority, wallet guard, payees, limits, fee and all other fields remain byte-equivalent to that reference.

The checker independently restates the published frame domains and little-endian lengths for base Setup2, base remote2, IssueProfile1 and their wrappers. It compares the prepared IDs, exact CA bytes and leaf SPKI hash extracted from bounded public DER. DER framing/SPKI identity does not prove certificate signatures, CA eligibility, hostname, lifetime or a real host handshake; original preparer/client checks and an observed remote TLS1.3 exchange still supply those facts. Wallet-created/closed and node-RPC-copied booleans remain typed preparation metadata, not wallet/runtime/HTTPS evidence. The checker explicitly returns runtime/TLS-handshake/wallet qualification false even on a selection PASS.

It recomputes the independent two-input/two-native-output maximum signing shape and exact two-reward economics: miner1,980,000,000atoms, fee209, change19,999,791. The maximum signing shape374raw bytes/836weight/209vbytes is not a claim that actual DER signatures consume the maximum; A2's observed raw transaction was371bytes. The actual transaction must still be decoded, verified, broadcast and confirmed during the hosted test.

Root supplies the already selected external Origin, UTC due and original start. The checker requires due=floorUTC(start+7minutes), no UTC date crossing during the15minute scenario, exact prepared due and credential expiry=due+1hour. It does not query or move the clock. The normal-node height2→62 refresh is an actual prerequisite, not something this public-selection checker claims to observe. Hosted rewards63/64, maturity164 and payout confirmation165 remain separately required actual evidence.

Example recipe only, not an executed command:

```text
python3 /Users/taytrrip/slug/set82-prep/hosted-actual/check-selection.py \
  --public /absolute/prepared/public \
  --reference /absolute/qualified/reference-v1.json \
  --origin https://selected-host:port \
  --due-utc YYYY-MM-DDTHH:MM:00Z \
  --initial-start-ms ORIGINAL_INTEGER_MILLISECONDS \
  --out /absolute/new/selection-verification.json
```

All exact allowlisted input hashes are bracketed before/after. Paths must be canonical and no-follow regular single-link files. Existing output is refused, and the report must be outside the prepared public directory. The checker does not repair or overwrite preparation outputs. This is not an independent expected-output producer: the expected fixture economics were already qualified, while dynamic host-specific identities are calculated from the explicit selected inputs.

Root later applied formatting only. `preparer-formatted-manifest.json` binds current files; the reviewed originals and original manifest remain in `preparer-before-root-format/*.txt`. The earlier acceptance/checker manifest is preserved in `pre-preparer-format-rebind/`; source identity rebinding does not change the checker or its expectations.
