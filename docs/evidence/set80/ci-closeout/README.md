# Set80 final CI closeout

All seven GitHub jobs passed on chain commit
`1f559221c3735227e8475731f2c73a74282db5b0`, paired with wallet
`dfc5d9dd7e0f604486fbac812d0413fce1f45cd6`. This is the maintenance successor
to the original failed `ce9eac74` run; that failure remains preserved.

The downloaded Linux artifact contains 428 required passing assertions in 29
groups. All 61 commands exited successfully with empty process groups, no
cleanup errors, and unchanged clean source inventories. Every downloaded log
was checked against the report's byte count and SHA256. Temporary material was
removed. Actual-node, provisioning, and deployed-host claims remain false in
this CI artifact; the separately qualified actual runs retain their own scope.

See `final-ci-passed.json`, `final-linux-verification.json`, and the exact
public copies bound by `archive.json`. The source inventories before and after
are byte-identical and archived once. These files are evidence, not instructions
to follow historical filesystem paths.
