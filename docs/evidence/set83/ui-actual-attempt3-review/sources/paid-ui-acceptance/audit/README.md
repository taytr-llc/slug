# Closed attachment evidence

This small standalone helper audits only the fresh selected attachment after
the original WIRE2 helper has logged out, closed, exited and joined. Root alone
builds and executes it under the remaining original 300-second acceptance
deadline; it is not a wallet or network client.

```
paid-ui-audit --attachment-root NEW_ABSOLUTE_ROOT --remote-config PUBLIC_REMOTE_JSON --report FRESH_PUBLIC_JSON
```

`outbox_audit.go` is byte-identical to the qualified hosted attempt5 proofclient
leaf. `bindings.go` keeps exactly its two original pure worker-config function
bodies. The concrete `OpenRecoverableDailyPoolOutboxAudit` first performs the
complete selected namespace/configuration audit and closes. Only then does the
inherited helper reopen LevelDB with `ReadOnly`, `ErrorIfMissing` and `StrictAll`
to project every key/value, close, and verify its own file hashes unchanged.
The fresh attachment must have one metadata row and no q/e/h/p/scan/cursor:
all record/request/pending/logical-byte/reservation counts and active ID are
zero. The complete selected metadata and marker hashes remain in the report.
The original attachment root and public configuration are checked again after
the audit. No Start, Issue, wallet, credential, RPC or signing API is used.

There is no cancellable storage audit API. The acceptance runner must launch
this one original process under its existing remaining deadline, retain its
stdout/stderr/exit and join, and classify a timeout as failure. A successful
helper report alone does not prove the preceding original WIRE2 owner joined;
the parent process evidence supplies that precondition. Failed/partial audits
must not be called successful joins. The report never claims hosted financial
or whole-acceptance qualification by itself.

The sibling `catalog.py` exports `snapshot_old(root)` and
`compare_old(before, after)` for exactly the old attempt5 attachment path. It
does not open a database: it hashes only its known public marker and outbox
files after owners have joined, with finite counts/bytes, no symlinks and
original file/root identity checks. Unlike the earlier native catalog policy,
this comparison allows **no** changed old DB files. It ignores access times so
the read itself cannot change the result. Keep old account/wallet catalogs and
financial audits in the parent runner's separately selected scope.

Source-only freeze: no Go build, formatter, runtime or live/private file read
was performed in this lane. Root composes this module with the exact paired
workspace and records any formatting successor before execution.
