# Argon2id Parameter Check: 64 MB vs 256 MB

Historical benchmark note: set 7's accepted report-only checkpoint policy
supersedes the fast-sync claims below. Signed league declarations do not
skip PoW or other validation; the measured sync cost remains a review item.

November 15, 2026. Benchmarked with golang.org/x/crypto/argon2
(bench source in argon2id-bench/) on Ren's VM, a shared vCPU that is
slower than a modern laptop. Absolute times will improve roughly
1.5-2x on current hardware; the ratios are the decision-grade part.
Single lane (p=1) per attempt, as miners parallelize across attempts.

| Setting        | ms per hash | hashes/s per thread | PoW verify time per year of chain (262,800 blocks) |
|----------------|-------------|---------------------|-----------------------------------------------------|
| 64 MB,  t=1    | 66          | 15.2                | 4.8 h                                               |
| 256 MB, t=1    | 340         | 2.9                 | 24.8 h                                              |
| 64 MB,  t=3    | 219         | 4.6                 | 16.0 h                                              |
| 256 MB, t=3    | 907         | 1.1                 | 66.2 h                                              |

## What the numbers mean

- **The prospect cadence does not change.** Difficulty retargets to
  the network's total attempts, so a one-device gym finds a prospect
  every 2 to 3 days (D30) at either size. The parameter buys hardware
  fairness margins and costs, not user-visible rates.
- **Steady-state nodes barely notice either setting.** Verifying one
  block costs one hash: 66 ms or 340 ms against a 2-minute block
  time. The cost only concentrates in initial sync, and the design's
  signed checkpoints (D60 area) already double as fast-sync anchors,
  which is where most of the 25 h per year at 256 MB goes away.
- **Laptops are fine at both sizes.** Mining memory is threads times
  arena; cores run out before RAM does (16 threads at 256 MB is
  4 GB). 64 MB is gentler in a browser tab or on a phone, but the
  client plan settled Nov 15 is desktop apps first, browser second.
- **The farm difference is real but modest.** A 24 GB GPU fits about
  90 concurrent attempts at 256 MB against about 370 at 64 MB, but
  per-attempt speed is bandwidth-bound at both sizes, so the card's
  edge over a laptop stays in the same band. 256 MB is the stronger
  version of the fairness story; it is not a different world.
- **Time cost is the expensive lever.** Going t=1 to t=3 roughly
  triples verify and sync cost at either memory size. 256 MB at t=1
  (340 ms) buys more memory-hardness per verify millisecond than
  64 MB at t=3 (219 ms).

## Recommendation

Keep the settled **256 MB, t=1, p=1**. The sync tax is the only
material cost, and checkpoints already answer it. Revisit 64 MB only
if the browser gym ever becomes the primary client.

**Outcome: Mike closed the parameter the same day. 256 MB, t=1, p=1
stands as settled, and the spec's open item is closed.**
