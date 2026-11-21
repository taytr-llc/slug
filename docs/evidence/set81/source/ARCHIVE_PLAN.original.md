# Set81 public archive proposal

`archive-plan.json` is an explicit source-to-repository map, not an installer.
No source, fixture or evidence file was copied. The active actual run and all
bound checker/helper/profile files remain untouched.

The map names 224 public sources. It proposes 123 unique new files totaling
1,446,828 bytes, with 76 references to exact existing repository files and 25
references to another proposed copy. This avoids 16,001,139 bytes of repeated
inputs and preimages. Every entry records the original absolute source path,
byte length, SHA256, role, logical snapshot location and actual repository
destination. Reference entries are documentary mappings, never symlinks.

Proposed locations follow the existing repository convention:

- `testdata/retained-admission/` holds the independent state oracle and both
  correction overlays. The seven inherited corpora remain at their exact
  existing repository paths.
- `desktop/pooloperator/testdata/retained-admission-actual/` holds the new helper,
  original preimages, independent selection producer and qualification. The new
  qualified selection is stored once; both interpreter outputs map to it by
  their equal independently recorded identities.
- `docs/evidence/set81/execution/` holds checker generations, their review
  rationale/diffs, bounded runners and original/final/superseded execution locks.
- `docs/evidence/set81/source/` holds source coverage, peer-review records,
  implementation selections and test/preflight/format provenance. Current
  repository product/test leaves remain exact references.

The existing 26 component/regression/build/smoke files and their archive index
stay in place. Original failure reports and the never-executed superseded A lock
retain their dispositions. This proposal makes no actual or exact-tip CI claim.

After root releases the frozen sources, recheck every source identity and every
existing reference. Refuse any new-copy destination collision. Copy only the
`copy_after_authorization` entries with exclusive creation, then verify all
destination identities and reference mappings. Never recurse from a selected
source or manifest into an unlisted tree. The original manifests and scripts
retain their qualified bytes and original absolute execution paths; the map
does not pretend that the archived layout is directly portable.

Root will archive actual evidence separately after the runs, using only the
checker's exact returned public-file inventory plus explicitly selected public
root reports and dispositions. This plan does not name or read current actual
outputs and never includes private fixtures, credentials, wallet/database files,
node homes or a runtime directory wholesale.
