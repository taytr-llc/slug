# Selected registry file-close retention

This is a narrow correction proposal for the frozen first-compile checkpoint `87bf9f2509c1dd8df25abf79d0924601185d8af04982719ce4d701a59bd3a6fc`. No implementation or execution is included.

The new registry constructors currently reuse `writePublicFile` and `readSelectedPublicGuard`. Those legacy helpers return file Close errors but discard the original file pointer. The new DB failure path already returns the original ManifestStore when its Close fails; the remaining gap is a marker or ownership-guard descriptor before the DB has opened. This is an ownership/cleanup gap, not altered issuance or financial semantics.

Use selected-only private file helpers in `registry/issue_store.go`, and one private retained-file field in `registry/manifest_types.go`. Allocate the selected ManifestStore before opening either file. Its limited reader checks Lstat/Stat identity and regular-file type, bounds the read, and closes the exact original descriptor. Its exclusive writer checks short writes, synchronizes, and closes the exact original descriptor. Both keep the original descriptor and sticky joined error only if Close itself fails. Read, write or sync failure with successful Close returns no retained owner.

The selected guard reader retains the exact existing inventory, marker filename, canonical guard2 bytes and 8192-byte cap. The selected marker reader retains wrapper1, original base5, profile, LF, cap and canonical comparisons. No legacy helper or constructor changes. No file is reopened to pretend cleanup succeeded.

`ManifestStore.Close` must recognize the retained pre-DB failure owner and return the same stored error without retrying Close or waiting on a nonexistent DB. Its ordinary DB/publication shutdown remains unchanged. Constructor failures return that nonnil closed owner only on an actual descriptor Close failure; the borrowed operatorauthority owner is never closed by the registry.

Tests should cover the private failure-retention helper with the exact original descriptor and sticky repeated Close result, successful-close failures returning nil ownership, and normal selected create/open byte equality from the independent corpus. These are conditional helper controls, not a claim to have induced a kernel Close failure. Existing DB uncertain-write/reopen tests stay unchanged. No new injectable signing, resolver, verification, or I/O authority is required.

Files: registry/issue_store.go, registry/manifest_types.go, the initial Close guard in registry/manifest_store.go, and one focused test leaf. No serialized change, cap increase, root replacement, retry, migration or source repinning.
