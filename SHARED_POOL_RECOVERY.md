# Shared fixture wallet recovery

Set62 connects the Pool Pilot's existing recovery controls to its actual
recipient-bound wallet. This remains the two published test fixtures on the
owned Phase-A chain. It is not production custody or permanent-chain recovery.

## Export and authority

Stop mining before exporting or checking a backup. Native code captures the
original account view before opening the file chooser. It retains descriptors
for the selected parent directory and, for verification, the selected file.
Only public file identities and that original view cross the helper protocol;
recovery words and wallet bytes never pass through the webview or IPC response.

The helper acquires matching descriptors before acknowledging verification.
Directory walks reject symlink substitution and protect application data,
managed namespaces and bundled binaries. Export never overwrites an existing
file. It writes an owner-only temporary file and publishes exclusively through
the pinned parent directory, then synchronizes the directory. A bounded parent
identity check remains inside the original account capability. Slow wallet
preparation stays outside that capability.

D72's recovery-export permission is checked again at publication. A replaced
login or reset cannot lend a new credential to an earlier operation. During the
24-hour reset hold, export is refused while existing-file verification remains
available under viewing authority. If publication may have happened but its
completion cannot be established, the interface says to inspect the selected
destination before retrying. It never claims a definite failed save.

## Verification and lifecycle

Verification restores into a fresh scratch wallet from the selected file only.
It checks the original wallet identity and native84 recipient metadata, scans
the shared chain, waits for actual synchronization even at genesis, proves
the restored gym can sign, and compares accounts,
balances, UTXOs and history with the original wallet at an unchanged chain tip.
Local transaction-discovery timestamps are excluded; chain membership, amounts
and block timestamps remain part of the comparison. The original wallet is
sampled again before success. A moving chain or wallet state requires retry.

A fresh chain rescan does not replay every previously received mempool payment.
Any unconfirmed balance, UTXO or transaction in any observation therefore makes
verification temporarily unavailable. The user can retry after confirmation;
the verifier does not drop pending entries merely to produce a match.

Export success returns to idle. Only completed fresh restoration is labelled
verified. Verification never spends, allocates another recipient or starts
mining. Mining cannot start during a backup operation. Cancellation, logout,
reset and shutdown join the operation and its cleanup; a scratch wallet closes
before its directory is removed. Cleanup failure is visible and retains the
directory if close fails. A cancelled operation leaves the original wallet.

The pinned filesystem implementation is runtime-qualified on macOS. Linux
compilation and the explicit unsupported Windows stub are separate evidence;
they do not establish runtime support on those platforms. Legacy solo recovery
and its stores retain their existing behavior. The final joint D74 review,
real provider/custody setup and permanent genesis remain separate gates.
