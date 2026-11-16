# Public signed league checkpoints

These records are report-only declarations signed offline by the operator.
They do not change proof-of-work validation or chain selection. No private
signing material belongs in this directory.

## Phase A, height 1

File: `phase-a/height-1.json`

- Network: disposable SLUG Phase A (`mainnet` in the client).
- Genesis: `7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd`
- Signed height: **1**
- Signed hash: `729d92f2de29fa6415f4920df49164a8a9824971ff972d42037ee7c9d41f470f`
- Authority: `811349556ea792060edbb0518dd3a513c5906abe495a81eaf629abd5900c44a3`
- Public JSON SHA-256: `8a21dc887e9107aef878c121c94cac9e02e5b5c99f67958c0e913526a5a9cebd`

Mike signed this record on November 15, 2026. Signature verification and
normal node import passed. At tip height 61, the node reports `finalized`
with 60 descendants and no conflict. The observed tip was
`1f08a4ac11d4c60ebecbb8dc85401fa2756576c3d1920c45cad5f7113b587ee0`.
This is an observation of that chain state, not a rule preventing reorgs.

Verify from the repository root:

```sh
go build -tags= -o slugcheckpoint ./cmd/slugcheckpoint
./slugcheckpoint verify --network mainnet \
  --pubkey 811349556ea792060edbb0518dd3a513c5906abe495a81eaf629abd5900c44a3 \
  --file checkpoints/phase-a/height-1.json
```

Load explicitly when starting the node, adding this flag to its normal
arguments:

```text
--signedcheckpoint=/absolute/path/to/checkpoints/phase-a/height-1.json
```

Then query `slugctl getcheckpointinfo`. A node that has not synchronized
yet can report `pending` or `provisional`; a conflicting best chain reports
`conflict` and continues normal chain selection. Never convert this record
to `--addcheckpoint`.

The deployment image includes the same public record at
`/usr/share/slug/checkpoints/phase-a/height-1.json`. It still requires the
explicit `--signedcheckpoint` flag. No Railway settings were changed by
publishing this file.
