# Set80 CI shutdown-race follow-up

The In-tree modules job at `ce9eac7463a75d2ba7e0cefc8924f55d6c77b0a9`
panicked in `TestSharedStableCloseJoinsForeignVerification`. The original
lifetime was canceled before its asynchronously scheduled notification reached
the operation context, permitting a second proof hook. The CI excerpt and full
log identity are retained here. The first-failure CI snapshot is not a claim
that the other jobs had completed.

The new regression holds only callback scheduling, retaining the original
owner's Done/Err and original source/session. Before the fix it deterministically
observed two proof calls and a nil operation error. The exact same regression
passes after synchronous lifetime checks at the existing proof/source and
lease boundaries. No timeout or acceptance assertion was relaxed. The original
CI test remains unchanged. This does not make cancellation atomic with a write
that has already passed its final guard.

Full normal and race gym-stable runs each pass 76 top-level tests and 499
assertions; the one cheap-profile-only test naturally skips. Vet passes.
Every run preserves its source/profile binding and joins its owned processes.
`evidence.json` binds the reports, raw logs and complete source inventories
retained in the workspace. No fixture wallet, credential or runtime database
is included. Follow-up exact-tip CI remains pending.
