# Set69 implementation clarification: completed Phase-A loader cleanup

Reviewed clarification, frozen before implementation. This clarifies the frozen
PHASE_A_BACKUP_RECOVERY.md rule that every schema12 cleanup failure retains its
owner and prevents scratch deletion. It does not alter its formats, derivation,
scanner limits, public recovery APIs, or schema8/9/10/11 behavior.

## Identifying the owned schema12 transition

The Loader that successfully begins or opens a Phase-A recovery session retains
an internal Phase-A identity flag for its fixed database path. Successful session
Close releases the active reservation, not that known-profile fact. The desktop
verifier reuses this same Loader for the completed ordinary OpenExistingWallet
transition; it does not construct a new unclassified loader for that step.

For an ordinary loader without that retained fact, a bounded read of the exact
address-manager version (four bytes, before generic wallet open or key decoding)
identifies schema12. The read uses the existing read-only manager-version API;
it does not inspect private material or adopt expected recovery identity from
storage. The cleanup owner is selected by that exact version or the retained
Phase-A fact, never by caller JSON. A known Phase-A loader refuses a different or
malformed manager version instead of silently opening another profile. Public
fixture/root/gym and pending/completed checks still run as required by the main
contract; identifying cleanup policy is not wallet authorization.

## Failed ordinary open

If OpenExistingWallet has acquired a database and ordinary wallet open fails,
the schema12 path attempts all owned cleanup once. If cleanup succeeds, return
the original ordinary error and release its active reservation. If cleanup fails,
return *PhaseARecoveryCleanupError with the initiating open error and first
cleanup error available through Unwrap. Retain the opaque database/manager owner
and the Loader reservation. Do not log-and-discard the close error, return a
normal unowned failure, or let another create/open/unload reuse the Loader.

The typed error's Close only joins and returns the original sticky cleanup
result. It never calls the backend again or converts an uncertain first close
into success because bbolt later returns nil. An acquisition failure that returns
no database owner remains an ordinary error, as in the main contract.

## Ordinary wallet unload

After a completed schema12 wallet has opened, UnloadWallet stops and joins all
wallet goroutines before its one owned database-close attempt. Successful cleanup
releases the active wallet/database reservation as usual. On failure it returns
the same typed cleanup error, retains the stopped wallet/database owner, and
keeps the Loader reserved. Later UnloadWallet or typed Close calls return that
same sticky cleanup result without retrying the database. LoadedWallet must not
republish the stopped failed-cleanup wallet as an available wallet.

The internal failed owner remains reachable through the Loader and typed error.
No new secret/DB pointer, repair setter, retry option, background cleanup task,
or arbitrary callback is exposed. Serialization/join occurs outside account
leases. No hard wall-clock bound is claimed for an operating-system close or
fsync; there is exactly one joined cleanup attempt, not an unbounded retry loop.

## Desktop and qualification obligations

The desktop retains the runtime/Loader owner before identity validation and
retains any failed-construction cleanup error. It must not use a helper whose
identity-failure branch drops the cleanup owner. Session close must succeed
before ordinary open; all session, source and runtime closes must succeed before
RemoveAll. Any uncertain cleanup reports backup_storage_failed and preserves
both the directory and its retained cleanup owner. The original verification
operation cannot become successful afterward.

Focused tests inject a first database-close error followed by a hypothetical
second-call nil at (a) failed ordinary schema12 open and (b) ordinary schema12
unload. They require one actual close call, the identical sticky error, retained
Loader exclusion, no available LoadedWallet, and no permitted scratch removal.
Keep schema11 and other existing-schema loader tests unchanged and passing.
