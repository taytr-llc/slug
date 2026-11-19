# Set66: portable public fighter evidence

Qualified candidate implementation on `slug-public-fighter-evidence`, stacked
from completed Set65 `7bf924f6a11f496060633c58623242e18b1e3004`.
The normal Mac package is **0.5.5 / build 66**.

## What changed and why

The existing completed fighter-history panel can export the original public
admission receipt and every row of that exact captured history. It reads the
original concrete source, including fighters outside the eight-item Recent
cache, and preserves both v1 and v2 appearance/source bindings. It does not
recapture a newer history or turn caller-supplied JSON into a trusted store.

The additive `fighterevidence` package and bundled `slugevidence` command check
supplied work, gym authorization, appearance, receipt/assignment links and
history arithmetic against a separately supplied full expected stable Pin.
A success means **consistent supplied evidence**. It does not authenticate
issuer/birth/acceptance timing, actual awards/result finality, canonical chain,
current ownership or complete/fresh authoritative history. The report always
retains those explicit unchecked fields. Coherent omissions or changed unsigned
acceptance times can remain internally consistent; the reference demonstrates
that limit instead of presenting a checksum as authority.

The canonical file is bounded at 512 KiB, with at most 256 rows and separate
nested byte/count/depth limits. Cheap shape/source checks precede one normal
proof verification; a fail-fast verifier gate prevents a queue of work. The live
export uses its one existing concrete source verification, followed by cheap
serialization, rather than hashing the same proof again. A pure structure helper
factors the old prospect pre-hash checks without changing normal verification.

Native selects a fresh destination. The public writer stages and synchronizes
bulk bytes outside account leases, then exclusively links the file under the
original view/history fence. It never overwrites a destination. Cancellation
and Close join work and cleanup before a terminal result. Possible publication
remains explicitly uncertain after later errors. The old recovery writer's
64-KiB bound is unchanged.

The new private protocol carries metadata only. Native knows the export ID
before dispatch and retains one uncertain destination handle until exact
resolution or proven cleanup. It tracks the exact current action independently
of the pending handle; an older success for the same history cannot stand in
for a new export. Stop joins direct export work, and asynchronous account
transition acknowledgments are not treated as proof of cleanup. One parent
permit accounts for a 4-MiB scratch reservation across child generations; this
is not a measured hard heap cap. Mining must be stopped before export, without
implicit pause/resume. Newly authenticated held sessions may export public
View data; revoked sessions cannot finish. Wallet recovery remains held.

The byte contracts are `PUBLIC_FIGHTER_EVIDENCE.md` and
`PUBLIC_FIGHTER_EVIDENCE_DESKTOP.md`. Their opening freeze/status paragraphs are
preserved historical records; qualification status is recorded here.

## Verification and retained corrections

Evidence is under workspace `../artifacts/set66`.

- Old-API extraction from copies of completed Set64 v1 and Set65 v2 roots
  passed. Six public cases cover both awarded gyms and one genuinely unawarded
  fighter per source. Original trees stayed unchanged.
- The independent core corpus has 28 positives and 44 refusals, six directly
  recomputed normal-work headers, and 24 parser-boundary diagnostics. Final
  SHA256 is `eb51028daf2ebbda44dfbfdb79f58353137ed03f1836fad5b8db8cf428300772`.
  It reproduced under Python 3.14.3 and 3.9.6 and from a repository checkout.
  Preserve the successful predecessor and narrow signed64 DNA-bound correction:
  all original case bytes/verdicts stayed identical; only producer-lock metadata
  changed. No expected result came from the new Go implementation.
- Independent private-wire references have 112 positives, 38 contextual
  negatives and four raw grammar controls, SHA256
  `c2489601d6c58440e001839c4ed14ce17dbaec43203a36ca197fa9ff89fc6dcb`.
  The pre-execution decimal parent-identity proposal was corrected to the
  inherited exact-hex grammar before generation; both proposals are preserved.
  Synthetic transport identities do not claim actual login/file authority.
- Core normal/race/profile-refusal, original prospect/history regressions and
  accessor checks passed. The full recoveryfile normal/race suite and vet passed.
  Finite independent reviews found no additional writer/core defects.
- Review corrected three desktop lifecycle/recovery issues: Stop needed an
  explicit direct-lane join; account transition acknowledgments can precede
  cleanup; and generic latest-job recovery could substitute an older same-history
  export. Focused normal/race tests and final native regressions passed. Prior
  reports remain bound to their original source rather than relabeled.
- CLI review replaced pre-stat/blocking-open with a nonblocking descriptor-first
  open followed by a regular-file check. Normal/race/profile tests and Linux/
  Windows compile checks passed. File input executes on Darwin/Linux; other
  platforms explicitly refuse rather than use an unqualified blocking fallback.
- All **331 frontend tests** passed on final source. Preserve the initial
  test-only fixture-name typo and both successful successor runs.
- Actual live export passed in 42.85 seconds: all six files matched independent
  canonical bytes exactly, with original captures and detached expected pins.
  The v1 sequence-1 fighter was genuinely outside Follow Recent8. Source records,
  original trees and chain tips stayed unchanged; owned nodes stopped normally.
- All six complete desktop packages passed on identical frozen sources. The
  first run retained four passing packages but sandbox loopback denials in
  accountpilot/pilot. Only those affected packages were rerun with authorized
  loopback access; they passed in 20.45/141.58 seconds. The explicit composite
  report preserves the failed first run and both source/log bindings.
- Final vet, gofmt, diff checks and builds of all ten modules passed. The normal
  Mac bundle built in 16.11 seconds with strict ad-hoc signature verification,
  normal profile/tag/symbol checks and the packaged offline verifier.
- The exact bundled verifier matched all six independent full reports with
  source runtimes stopped, and refused wrong-pin, noncanonical and oversized
  files. The packaged helper passed in 108.34 seconds for both default v1 and
  explicit v2: source discovery, uncaptured-history/unknown-job refusal without
  file mutation, old-view refusal, recovery, restart and selector mismatch.
  No implicit mining, native-dialog rendering or GUI acceptance is claimed.
- The required HANDOVER smoke passed: isolated two-block mining, 10-SLUG subsidy,
  report-only checkpoint conflict, read-only public seed sync, height-61 history,
  activation-at-62 reporting, chain verification, stale-template refusal and
  clean restart. No public block was mined. Its 1,770 node/ctl inputs match Set65
  exactly, aggregate SHA256
  `8649ebcae2d50c7440ebad5254ce40c9f95f048f3cc0108804974ffb89a43e38`.
  No seed rollout is required or performed for these unchanged inputs.

## Offline usage and remaining work

The Mac package includes `Contents/Resources/bin/slugevidence`. Invoke it with
`verify --source-pin PIN_FILE --file EVIDENCE_FILE`. The pin must come from a
separately established source configuration; copying an embedded claimant pin
into a second file establishes no trust. Success writes one bounded JSON report
to stdout and explains the limits on stderr. Rejection emits no success report.
The command reads public files only, with no wallet, source discovery or network
operation. The live export's checksum is not an offline verification report.

No importer, transferable ownership authority, new signing power, historical
rewrite, descriptor activation, real custody, production genesis, notarization
or Windows runtime acceptance is added. The thorough joint D74 review, visual/
accessibility acceptance, app organization and navigation remain final gates.
Transfer effective timing and competing-transfer ordering remain explicit future
policy questions. Next implement authenticated own-payout history and a separate
dated wallet transaction observation under the already settled payout rules.
