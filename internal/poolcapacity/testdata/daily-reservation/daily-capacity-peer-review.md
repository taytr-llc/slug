# Daily reservation premise review

2026-11-20. Reviewed proposed `daily-capacity-plan.md`
`45c0b384a480f981aced9d5147f258a18d014cd7b39d4db7a81055e396d1bfdd`
against current source only. No producer, Go, runtime, private data or network
execution. No concrete counterexample found under the proposal's stated
immutable-source/planner premises. This closes the derivation premises, not the
future independent encoded-byte vectors or implementation.

- **Fractions:** `poolsettle/model.go` derives each original net claim as
  reward × shares ×99/(window×100), window<=4096. `planProfile` subsequently
  subtracts integer payments/fees or earlier exact claim fractions; it never
  proportionally divides a residual. All denominators therefore divide
  D=100×LCM(1..4096). For each prime, the additional power in L(2m)/L(m) supplies
  a carry term in the central binomial coefficient, proving the stated
  divisibility. Applying the bound at powers of two gives L(4096)<=2^8190.
  The stated denominator/numerator digit bounds are conservative even for
  16384×MaxMoney gross income. They do not justify weakening the4096-digit parser.
- **Lineage:** accepted windows partition original accepted rows, so original
  non-fee claims<=N and fee claims<=R. Once-consumed original roots/changes and
  immutable destinations support B<=2R−1: a change-only batch must combine
  distinct prior change lineages; one prior change by itself cannot make a
  configured gym newly payable after the same whole planner paid it below
  minimum. Pending A already counts. Projected change is only reserved, never
  spend-authorized. Counting alias block rows separately overestimates capacity;
  it does not make duplicate funding credit valid.
- **Window:** n<=4095 with the (n+1,n+1) reservation covers the first closing
  winner. It relies on the source invariant that every retained non-block row
  was accepted atomically. After drain, permit no newly retained ordinary row;
  each later accepted block closes its window. Saved previously unaccepted
  block rows must count in N/R before acceptance. Reopen must authenticate that
  invariant rather than trust the fact a controller was attached previously.
- **Bytes and counters:** source `maxWorkSize` is exactly MaxTemplateSize+16384;
  each submission consumes a logical4096 reserve. A worst-work plus remaining
  ordinary/closing receipt holdback before issuer commit is needed; a check only
  in `saveWork` can leave an extra durable issuer job before failure. All
  arithmetic must be checked before subtracting available bytes. Daily paid
  accumulation is checked int64, unlike V1's lifetime MaxMoney presentation.

The independent producer must still enumerate the exact codec frames. Required
coverage is all `c/`, `cr/`, `b/`, `bs/`, `bt/`, `bd/`, `e/`, `o/` and `spent/`
keys/values, the128KiB head/8MiB report reserves and256-byte delivery reserves.
Compare total input references<=R+B and payments<=G×B, plus per-credit128KiB and
per-body2MiB limits. Raw transaction bounds use the full witness byte count,
including framing, never virtual size. Signed body rows store references and
claims digests, not expanded historical claim copies.

Expanded signing requests are a separate bound: N+R<=32768 input claims, at most
the same number in change, and the actual640MiB streaming grammar. The report
must bound two full fractions per balance plus exact fixed fields for<=256 gyms;
do not substitute a per-page or64-enrollment count for the authenticated union.
The proposed R×MaxMoney<=MaxInt64 proof is conservative for every per-gym paid/
reserved tally and must use big/checked integers, not the old uint64 product.

The companion transport proposal consequently uses exact V2 int64 cumulative
totals and a separately bounded daily history page. No storage-capacity result
implies a fitting transaction, eventual payment, bounded cold audit time or
unlimited old-job receipt retention. D84, aliases, exact pending recovery and
finite signed-authority limits remain independent hard conditions.
