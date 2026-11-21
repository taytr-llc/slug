# Set83: remote wallet balances and history

Branch: `slug-wallet-daily-history`, stacked on Set82 `25d220e6`.
Status: implementation and verification in progress; no wallet UI qualification
or completed Set83 claim yet.

The first native slice connects authenticated DAILY balances and payout pages
to the original remote attachment and account controller. Reads use an exact
operation sequence and a dated, revision-bound capture. They borrow the running
worker's original credential lease or own a temporary stopped lease; they never
obtain mining authority. Stop, logout and Close join reads, and disclosure
rechecks the original View, recipient and protected credential files.

At most the first and current pages are retained. A stale continuation keeps the
old capture visibly dated rather than mixing revisions. Failed refreshes retain
the previous capture under its own sequence. Money and cumulative counters stay
exact; pool-recorded payment and wallet-observed confirmation remain separate.

The installed source passed two finite source reviews and the native
pilot/accountpilot/slugpilot build. The first focused test execution was blocked
by the sandbox's loopback listener restriction. The authorized loopback rerun
timed out at five minutes in mock-server cleanup because its handler left the
POST body unread before waiting for disconnect. A test-only correction consumes
the bounded request and requires actual handler cancellation before cleanup.
All ten new top-level tests then pass with the original timeout unchanged.
Both failed attempts remain retained. All 64 affected normal tests and 64 race tests now pass, along with five
profile/refusal controls. Vet passes after naming two external Fraction fields
in a test literal; its original diagnostic is retained. The native methods and
helper build successfully. See [component evidence](docs/evidence/set83/native-components/report.json).

Actual-read preparation found that occupied credential gates exposed a private
busy sentinel. The selected read boundary now maps only that exact value to
public ErrRemotePoolBusy. Its concrete held-TLS regression proves the original
request/client stays alive, no extra request occurs, wrapped/mixed errors stay
unchanged, and cancellation joins.

The actual healthy paid-wallet read/reopen now passes on source `005897c3`.
Two physical lifetimes of the original wallet read 19.8 test SLUG and the same
confirmed payment at height 165. The old View is refused after reopen; cached
history survives Stop. Full receipt replay and closed hosted audits preserve
the complete outbox, both credits and the original signed/reserved payment.
All 226 protected history files remain intact. The bounded orchestration took
14.816 seconds, with clean original process joins and separately verified node
cleanup. No mining, new issuance or payment was requested. See
[native acceptance](docs/evidence/set83/native-actual/QUALIFICATION.md).
Preparatory command/deployment failures remain separately recorded.

The explicit DAILY wallet helper, concrete bridge and existing wallet display
now pass component qualification: 13 new normal tests, 53 combined race tests,
40 original-path normal regressions, two cheap-profile refusals, six DOM tests,
vet and both normal helper/app builds. The independent wire corpus has 170
cases, reproduced byte-for-byte by Python 3.9.6 and 3.14.3. Its cumulative money
and operation/cursor counters remain exact decimal strings. See the
[integration report](docs/evidence/set83/wallet-integration/report.json).
The first run exposed incorrect frontend public-key shape checks, a contextual
View test applied to a stateless parser, and an unavailable cached dependency
index. Narrow corrections and a fresh build cache pass; failed evidence stays
retained. EF-19 records the separate cache follow-up. All six triggered GitHub
jobs pass for the earlier native component at `005897c3`; this does not qualify
the later integration commit.

The next native slice connects the original DAILY wallet allocator to bounded
own-wallet activity captures, fixed pages and transaction detail. Selected pool
payments can be observed only from the retained authenticated capture; callers
cannot supply an amount or recipient script. Identical first/current-page
copies are deduplicated, while conflicting copies fail closed. Original View,
operation, cancellation and wallet-owner fences remain in force.
Eighteen focused normal tests and twenty combined race tests pass, with five
cheap-profile controls and two expected normal-wallet skips. Vet passes. See
[own-activity component evidence](docs/evidence/set83/own-activity-native/report.json).
The first incorrectly spelled cheap-build tag is retained as a normal-profile
diagnostic; only the corrected `slugtestpow` run qualifies the cheap controls.
These tests do not replace a healthy watched-wallet or native-window run.

Remaining: actual selected helper/native-window acceptance and the additive
activity/output helper and display path;
then package and integrated acceptance. Existing V1 paths, headless mining,
wallet pins and payout rules stay unchanged. Sign-in follows this work, D74
remains the final joint design review, and marketplace remains on hold.
