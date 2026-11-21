# Set82: bounded hosted pool packaging

Branch: `slug-hosted-pool-test`, stacked on Set81 `138553b1`.
Status: public-source export checked; native image qualification pending.

The active work is a finite hosted operator and scheduled-payout test. General
capacity continuation remains in [the follow-up log](docs/EDGE_CASE_FOLLOW_UPS.md).
This slice packages the existing qualified source with the pinned paired wallet
and candidate compiler. No chain, accounting, custody or expiry rule changes.

`deploy/pool/export-context.py` exports only the two exact committed public
trees and verifies their archive commit identities. It never copies a local
wallet or working tree. The operator image builds static Linux binaries, keeps
private runtime inputs outside the image, and runs as UID/GID10001. The CI gate
executes existing file/ACL/lifetime controls under that UID and verifies both
shipped command help paths in the final image. These checks do not establish
hosted mining, mounted-volume behavior or a scheduled payment.

The first local context export failed because this Mac's Python lacks
`hashlib.file_digest`. The bounded streaming SHA256 successor exports both
original commits successfully; source selection is unchanged. The failed
partial context remains outside the repository. Native Docker is unavailable
locally, so the branch-scoped Linux workflow supplies image execution evidence.

The intended host path uses its own TLS listener over raw TCP forwarding,
literal-loopback private services and a separate verified HTTPS node RPC. The
existing Railway seed remains unchanged. Persistent-volume ownership, exact
endpoint/certificate and original private custody must be selected and verified
before hosted startup. See [package instructions](deploy/pool/README.md).

After hosted pool/payouts: wallet balances/history, sign-in, then the full joint
D74 review. Marketplace stays on hold. EF-12's extended-outage rehearsal remains
failed and deferred unless it meets the immediate-fix rule.
