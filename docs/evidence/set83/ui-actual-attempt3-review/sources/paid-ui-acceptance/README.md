# Paid original-wallet WIRE2 machine acceptance

Source-only preparation. Root alone builds and executes this helper. This is a
machine protocol proof, not a native-window/CUA observation or a new payment.
The expected transaction, wallet balance, canonical block and money history come
from the named, immutable Set82 attempt5 public qualification. No implementation
output is generated as a new expected fixture.

`run.py --execution /absolute/fresh-root-selection.json` uses one root-selected
window of at most 300 seconds, including elapsed time before invocation. It
reserves the last 45 seconds for root's host cleanup. Every helper request,
bounded exact-Busy repeat, async poll, EOF, signal fallback and Wait uses the
remaining original cutoff. Signals, nonzero exit, stderr, unjoined descriptors,
unknown replies or failed proof predicates fail the attempt. A failed attempt
keeps its fresh logs/report; it never starts a replacement sequence or process.

Root's protected 0600 selection has exactly these fields:

```json
{
  "version": 1,
  "approved_by": "root",
  "execution_authorized": true,
  "started_at_ms": 0,
  "deadline_at_ms": 0,
  "artifacts": "/Users/taytrrip/slug/artifacts/set83/NEW-MACHINE-EVIDENCE",
  "attachment_root": "/Users/taytrrip/slug/artifacts/set83/NEW-MACHINE-EVIDENCE/new-attachment",
  "files": {
    "helper": {"path": "/ABS/slugpilot", "bytes": 0, "sha256": "ROOT-PIN"},
    "audit_helper": {"path": "/ABS/empty-attachment-audit", "bytes": 0, "sha256": "ROOT-PIN"},
    "preparation": {"path": "/ABS/prepared/selection.json", "bytes": 0, "sha256": "ROOT-PIN"},
    "selection_check": {"path": "/ABS/selection-check.json", "bytes": 0, "sha256": "ROOT-PIN"},
    "remote_config": {"path": "/ABS/client-inputs/remote-config.json", "bytes": 0, "sha256": "ROOT-PIN"},
    "node_rpc": {"path": "/ABS/client-inputs/remote-rpc.json", "bytes": 0, "sha256": "ROOT-PIN"},
    "pool_credential": {"path": "/ABS/client-inputs/pool.credential", "bytes": 0, "sha256": "ROOT-PIN"},
    "admission_credential": {"path": "/ABS/client-inputs/admission.credential", "bytes": 0, "sha256": "ROOT-PIN"},
    "source_manifest": {"path": "/Users/taytrrip/slug/set83-prep/paid-ui-acceptance/source-manifest.json", "bytes": 0, "sha256": "ROOT-PIN"}
  }
}
```

The zero values are schema placeholders, not executable selections. Artifacts
must already be an owned canonical 0700 directory; the new attachment must be
absent directly under it. Root supplies the actual paired helper build and the
already checked generation2 inputs. No old credential is overwritten. Every
selected source/input identity is checked before and after. Protected inputs are
not copied into public reports. The helper argv is the existing original
`--daily-recoverable-issue --daily-wallet-helper --daily-wallet-helper-version=2`
with the exact declared data-dir, remote-attachment-root and protected remote
input flags. No command can select Start or Issue.

Two sequential original helper processes open the same paid wallet and the same
new attachment. Each logs in with the published incrementing256 fixture, observes
the original stopped wallet at height165 with 1980000000 spendable atoms, captures
the exact original one-batch payout history, and independently observes its
output0 confirmed in that wallet. Its own-wallet activity capture is paged to its
complete bounded end and must contain the same raw payment exactly once; detail
must reproduce the original incoming net and both outputs. Completed cancellation
and Stop preserve the exact dated captures. Logout then stdin EOF must join the
original controller and process cleanly. The second process gets a new View and
must reject the first View for both original operation families.

Only after each helper has joined, the selected audit binary performs the full
original selected outbox audit and Close, followed by bounded StrictAll public
projection and Close. Both complete logical states must be identical and meta
only: no request, mining work, delivery, scan or cursor. Physical LevelDB files
may change across the new attachment's legitimate lifetimes; each closed audit
must itself be read-only. See `audit/README.md` for its build/interface.

The OLD5 attachment is never opened by a database owner. `catalog.py` reads only
its explicitly allowed public marker/outbox namespace bytes, with complete
bounded traversal and no-follow identity checks. Before/after files, directories,
root identities, sizes, hashes and timestamps must compare exactly. There is no
old-outbox physical-churn exception. The wallet's root inode is retained; root
separately applies the established immutable226 local-history catalog rules.

Public evidence is `machine-report.json`, the strict old-attachment catalogs and
comparison, two original request/response logs, process metadata, two empty
outbox audit reports and their process/output logs. Failure evidence remains in
the same newly selected directory. Root still owns the independent financial
before/after audit (two credits, one immutable batch), node/host readiness and
actual joins, complete wallet catalog, and native-window observation. None of
those gates is inferred from this runner's `passed` value.
