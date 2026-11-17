# Set 40 packaging selection

Engineering addition to the frozen desktop contract `dab9b2e016b6ff5314b0323f37e560022bec025ee0428bb9306d8bb75040126c`.

The explicit `--candidate-manifests` flag remains available in the shell and helper. A separately named **SLUG Candidate Pilot.app** should open the candidate mode when launched normally from Finder. Its dedicated build script sets a private string at link time (`main.candidateBuild=1`), which supplies the shell flag's default. The existing build script does not set this string, so its default remains the Account Pilot. The helper always receives the explicit mode flag from the shell. No environment-variable activation is added.

The candidate bundle has its own display name and bundle identifier, and selects the existing new LocalManifestPilot namespace. Its node/helper builds retain normal PoW with explicit empty tags. It is an ad-hoc signed engineering artifact, not notarized or approved for friends/tester/public release. Selecting the new bundle is the opt-in action; the runtime behavior, typed transport, account/session boundaries and all remaining release gates are unchanged. The original contract remains byte-for-byte intact.

Validation: compare default flag help on the two build variants, verify bundle metadata and exact source/binary hashes, run the candidate packaged helper acceptance, and retain old-mode regression checks. No GUI rendering attempt is required or implied by packaging.
