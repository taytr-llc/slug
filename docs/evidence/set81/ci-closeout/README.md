# Set81 exact-tip CI closeout

All seven jobs pass at `138553b1c6c3b5e9b1fd13d17861912f7ce1cd4c`:
[Build and Test](https://github.com/taytr-llc/slug/actions/runs/44918359851) and
[Operator Linux qualification](https://github.com/taytr-llc/slug/actions/runs/44918359817).

The downloaded Linux gate verifies 470 required PASS assertions across 32 groups
and 64 commands, the exact paired source/wallet revisions, original compiler
profile, normal/cheap separation, binary hashes, bounded logs, clean joins and
unchanged sources. `linux-sources.json` is byte-identical to both original source
inventories; the archive map records the public copies.

`verification-failed.json` preserves the initial verifier's JSON-prelude error.
The frozen successor accepts only the two exact observed, version-pinned Go
download lines before one strict wire-selection JSON object. It changes no
assertion, source, scope or cleanup requirement. `verification-passed.json`
records the passing result and hashes of all checked logs and binaries.

No actual-B or hosted acceptance is implied. B1 remains failed and tracked in
EF-12; A2's separate actual evidence remains in its original archive.
