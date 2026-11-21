# Set82: bounded hosted pool packaging

Branch: `slug-hosted-pool-test`, stacked on Set81 `138553b1`.
Status: bounded actual hosted mining, scheduled payout and same-root recovery
PASS, including original-owner cleanup inside the original 900-second clock.
Public-source export and all seven corrected-package CI jobs also pass.

After Mike's explicit restart approval, attempt5 completed on the isolated
Railway service with product `359a2853`, package `4a15e8c9` and wallet `dfc5d9dd`.
The original desktop miner filled ordinary intake (two accepted rewards at
heights 63/64); after maturity the scheduler paid 19.8 test SLUG to its original
wallet in one transaction. The 209-atom network fee came from the pool's 1%.
Payment `2a475d6318ff194115024169265271221aea644a95c7d89e2782ec7492a1afa9`
confirmed at height 165. The wallet's spendable delta was exactly 1,980,000,000 atoms.

Three original operator generations joined cleanly. Restart retained both
exact work/nonce/receipts, the complete outbox (including the third active null
request), two credits and one immutable signed payout. A 90-second recovery
observation found the same canonical payment and an empty mempool throughout.
This proves one canonical payment, not a count of literal broadcast RPC calls.
All 80 selected inputs and 226 prior-history paths remained unchanged. The
wallet, nodes and relay joined within 856.392 seconds, leaving 43.608 seconds on
the original clock. See the [qualification report](docs/evidence/set82/hosted-attempt5-completion/qualification.json),
[financial proof](docs/evidence/set82/hosted-attempt5-financial-proof/), and
[selected public helper sources](docs/evidence/set82/hosted-attempt5-source/).

This qualifies the bounded synthetic hosted test. The final scratch PID1/provider
lifecycle remains EF-15; public catch-up was busy after rapid maturity mining
(EF-12), so no complete public fighter-view or general load claim follows.
EF-18 records the conservative full-intake worker banner. The overly strict
auxiliary status watcher failed; the original source status and closed audits
independently prove full intake. Earlier failed attempts remain failed.
The live seed and permanent-genesis gates are unchanged. Wallet balances and
history are next; sign-in and the joint D74 review follow.

The first timed hosted attempt failed during initialization, before rotations,
serving, mining or payouts. Only the original runtime intent was written; the
ownership reader rejected the selected private DNS HTTPS endpoint under its
desktop-only address rule. The [failed attempt](docs/evidence/set82/hosted-attempt1/)
retains its original clock and state. Normal nodes separately reached matching
height62, and hosted image/volume/public-selection checks passed.

The operator-specific correction permits non-loopback HTTPS only with an
explicit CA. Desktop restrictions and TLS/hostname, RPC authentication,
network/genesis, redirect, lifetime and cleanup checks remain intact. A real
TLS regression fails on the old code and passes after the correction. The
complete ownership normal/race suites, nine transport/identity refusals,
cheap-profile refusal and vet pass; see the
[regression report](docs/evidence/set82/operator-https-fix/report.json).
The corrected actual hosted proof is recorded above. Final scratch PID1/provider
shutdown and a reconciled one-off upload failure are logged as EF-15/16.

All seven CI jobs pass at `4a15e8c9`, including the full race job; see
[exact-commit CI completion](docs/evidence/set82/https-fix-ci.json).
The corrected package also passes its native Linux image job
[44929223689](https://github.com/taytr-llc/slug/actions/runs/44929223689).
Its exported source archives match the selected `359a2853` product and
`dfc5d9dd` wallet; 26 required assertions and 45 observed test/subtest PASS
records cover UID10001, ACL/lifetime, ingress and real HTTPS controls. Both
scratch-image help commands exit0. See
[corrected package evidence](docs/evidence/set82/package-https-ci/).

[Hosted attempt2](docs/evidence/set82/hosted-attempt2-failed/) successfully initialized and rotated all three fixture
credentials, proving the private HTTPS constructor correction on the actual
host. Its SSH/BusyBox launcher detached before the original owner began;
no miner or payout ran. The corrected fixture launcher retains a foreground
wait and five public launch-boundary artifacts; a harmless host probe passed.

[Attempt3](docs/evidence/set82/hosted-attempt3/) reached the original operator, which exited before public readiness.
Its child and monitor were joined; the run failed. The host's generic TCP
listener normalized the selected IPv4 wildcard to `[::]:8443`, which the
runtime's exact address guard rejected. A same-host Go probe reproduced the
mismatch and confirmed that explicitly selecting `[::]:8443` matches. Attempt4
then listened successfully but failed the old negative-control helper's
incorrect assumption that missing authentication must return401JSON. The
actual ingress correctly returned400plain text. Its stop joined both owners
but retained child0/monitor1 and clean=false because BusyBox rejected the
helper's `kill --` syntax. See [attempt4](docs/evidence/set82/hosted-attempt4-failed/).
Attempt5 corrected only the source-derived HTTP expectations, that fixture
signal syntax and fresh attempt paths. Product binaries stayed unchanged.
All previous roots and clocks remain intact. EF-17 retains the diagnostic
cancellation-label follow-up.

Automatic approval review initially rejected restarting the idle disposable
pool service because of its attached volume. Mike explicitly approved that
restart; it succeeded before attempts 4/5. The completed test stopped its
product owners and left the admin container to its original idle timeout.

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
All six broader exact-tip Build and Test jobs also pass, including the complete
race job, so all seven jobs are green at `2d79de95`. See the retained
[CI completion report](docs/evidence/set82/ci.json). This closes package/source
CI, not hosted execution.

The verified fixture host path uses its own TLS listener over raw TCP forwarding,
literal-loopback private services and a separate verified HTTPS node RPC. The
existing Railway seed remains unchanged. The proof selected and verified its
persistent-volume ownership, endpoint/certificate and synthetic fixture custody.
Real custody and general public launch remain separate. See [package instructions](deploy/pool/README.md).

After hosted pool/payouts: wallet balances/history, sign-in, then the full joint
D74 review. Marketplace stays on hold. EF-12's extended-outage rehearsal remains
failed and deferred unless it meets the immediate-fix rule.
