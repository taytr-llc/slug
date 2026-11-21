# Set82: bounded hosted pool packaging

Branch: `slug-hosted-pool-test`, stacked on Set81 `138553b1`.
Status: public-source export and native image/component qualification pass.
Hosted service and scheduled-payout acceptance remain open.

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

The [native package job](https://github.com/taytr-llc/slug/actions/runs/44919766385)
passes at `2d79de95`. Downloaded evidence matches the two locally exported public
source archives and the exact package source. Ten required component tests plus
five named Linux ACL subcases pass under UID/GID10001; 34 test/subtest PASS events
are observed in total. Both final scratch-image command checks exit cleanly as
UID10001 with read-only filesystems and networking disabled. Source files remain
unchanged. Image ID: `sha256:4e0ee315e0c2f5c1410b6d5a3bd08af8dd8839d1d5516699d8ff6fdfed1041b0`.

The first artifact check incorrectly expected a raw `.dockerignore`, omitted by
the upload action's hidden-file policy. That failed check is retained; the
successor verifies its context-manifest hash against the pinned Git source.
The image job itself passed on its first run. See [public evidence](docs/evidence/set82/package-ci/).
The broader exact-tip CI jobs are still being observed separately.

The intended host path uses its own TLS listener over raw TCP forwarding,
literal-loopback private services and a separate verified HTTPS node RPC. The
existing Railway seed remains unchanged. Persistent-volume ownership, exact
endpoint/certificate and original private custody must be selected and verified
before hosted startup. See [package instructions](deploy/pool/README.md).

After hosted pool/payouts: wallet balances/history, sign-in, then the full joint
D74 review. Marketplace stays on hold. EF-12's extended-outage rehearsal remains
failed and deferred unless it meets the immediate-fix rule.
