# Set82 hosted package evidence

[Package CI](package-ci/) preserves the successful native Linux image job at
`2d79de95984c72c64ee37575a5333b0afc24d470` and its downloaded-artifact checks.
The product source is pinned to Set81 `138553b1`, with wallet `dfc5d9dd`.

The archive contains only public source/build metadata, component-test output
and the two help-only container inspections. It contains no runtime wallet,
private custody or deployed service data. Ten mandatory component tests and
five ACL subcases pass under UID10001; both shipped commands run in the final
image. The first artifact check's hidden-file inventory error remains recorded
beside its passing source-hash verification successor.

This is packaging evidence, not hosted mining, volume recovery, full service
shutdown or scheduled-payout qualification. Broader exact-tip CI is recorded
separately when complete.
