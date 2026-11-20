# Independent protocol reference plan

Source-only plan; generation waits for the contract/source freeze. Scope is new wire composition conditional on independently accepted public remote configs. No X.509, BIP32, store authority, live wallet balance, mining, process execution or cleanup success is modeled by a JSON fixture.

Inputs will be copied into a closed `reference/inputs` catalog before execution:

- `CONTRACT.md`, exactly the root-reviewed successor.
- The existing independent `remote-pool/reference-v1.json`, SHA256 `5f67fcaaba0c78b97b0f116a4c598eb0ec62ee1436bbcf280edcc747856e5177`; producer validates its hash, exact named selectors, canonical config bytes and independently recomputed config IDs. No Go-marshaled output is an expected input.
- The remote-config producer's explicit recipe, approved lock, source-ready and final two-version qualification, for lineage only. No secrets, actual fixture stores, node RPC configuration or bearer credential values are consumed.
- A literal new recipe defining request IDs, safe observation values, fresh view ID domains and exact case selectors. View IDs are SHA256 of ASCII `SLUG remote native protocol public view\0v1\0` followed by the fixture name and `\0` plus `original` or `replacement`. These synthetic nonces grant no capability.

The producer uses Python's standard library only. Ordered dict fields follow the contract. It independently constructs every new canonical request/result byte string, then checks it with a separately expressed model of shape and cross-field rules. It copies inherited configs by value; no runtime fixture or Go test supplies expected protocol bytes. Positive counters and balance values are modeled observations, never earnings or proof of spendable funds.

Frozen finite output counts: 3 inherited public config selections, 12 request positives, 12 request refusals, 31 response positives (22 success shapes plus 9 error envelopes), 47 response refusals, 17 raw refusals, 2 exact-size raw acceptances, 4 framing controls and 8 helper/view state traces. Literal mutations/selectors live in `reference/model.py`; trace steps live in `reference/inputs/recipe.json`.

Finite families:

1. Requests for all five commands, including empty-view bootstrap and original-view snapshot, both existing fixtures.
2. Success responses for bootstrap, login, Start, Stop, Logout, original-view snapshot and expiry retirement; both fixtures. Additional single-fixture boundaries cover wallet unavailable/syncing/ready, held observation, stopped worker, failed worker, childless failure, zero counters and maximum safe counters/record capacity. No success requires the worker still to be running after Start.
3. Every closed error code, with `result=null`. Unknown mutation outcome stays distinct from a pre-dispatch refusal.
4. Cross-binding refusals: wrong version/ID/command/config/request view; old versus remote source substitution; wrong authority/account/gym/root/fixture/payee; differing embedded views or generation; childless residual fields; signed-in null children; impossible wallet/record bounds; action/result mismatch; unsolicited replacement view.
5. Raw refusals: missing/duplicate/reordered/unknown fields, illegal null, bool-as-integer, negative/fraction/exponent/overflow number, non-ASCII/control string, second JSON value, and exact request/response size limit. Framing controls cover missing LF, CRLF and blank/unsolicited lines separately from JSON object parsing.
6. Small state traces: first snapshot must be signed-out; login establishes one view; Start/Stop retain it; Logout/expiry retire it; empty-view read cannot adopt a signed-in view; stale/other-helper context refuses; unknown action retires the helper. These are model traces, not a substitute for concrete bridge lifetime tests.

The final corpus records each case's expected config name, original request, prior public view if any, typed value/raw bytes, expected acceptance and SHA256 of canonical bytes. No generic result arms or hidden Go-derived golden generation. The bounded runner writes a fresh exclusive output directory, admits only the frozen catalog/source hashes, caps CPU at 30 seconds, wall time at 60 seconds and each output/input at 4 MiB, and preserves failures. Python 3.14 and 3.9 output must be byte-identical before installation is proposed. The caller sets `PYTHONDONTWRITEBYTECODE=1`; only a new attempt directory is writable by the producer.

Future Go tests must also exercise the concrete original process/controller gates, post-action disclosure failure and joined Close. The pure corpus does not claim those effects, nor native UI or public pool readiness.
