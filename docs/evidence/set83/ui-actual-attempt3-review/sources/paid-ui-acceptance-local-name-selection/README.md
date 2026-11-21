# Original local name selection correction

The previous attempt's helper exited before its first snapshot/login. Its
runner selected name version2 for the original owned wallet. Public prepared
remote configuration retains ExpectedLocal descriptor c1a8650a (name1), while
the separate remote operator source is cd02ae57 (name2). The original controller
correctly rejects a selected local descriptor different from its saved init.
The successful Set82 hosted5 client runner explicitly selected name1.

This successor changes ONLY the helper argv --candidate-name-version=2 to
--candidate-name-version=1. All deadlines, 300-second root window, cleanup,
read requests, exact money/activity/output assertions, remote configuration,
operator name profile and source semantics remain unchanged. Old ec7 source
and failed attempt2 remain preserved. Root selects a new independent attempt
and deadline; no old window is extended or resumed.

Native-window argv uses the SAME original local selector. Invoke the exact
selected package executable with these flags (substitute only root-selected
absolute protected paths, never command-line credentials):

```
"/absolute/SLUG Remote Pool Pilot.app/Contents/MacOS/SLUGPilot" \
  --daily-recoverable-issue --daily-wallet-helper \
  --daily-wallet-helper-version=2 --candidate-name-version=1 \
  --data-dir /Users/taytrrip/slug/artifacts/set82/hosted-actual-attempt5/owned/owned-gym \
  --remote-attachment-root /absolute/new-attempt/remote-attachment \
  --remote-config /absolute/protected/remote-config.json \
  --remote-node-rpc /absolute/protected/remote-rpc.json \
  --remote-pool-credential /absolute/protected/pool.credential \
  --remote-admission-credential /absolute/protected/admission.credential
```

The name selector binds the existing local wallet source, not the remote
operator's new work descriptor. Neither original history is rewritten. Start
remains explicit and is forbidden in this read acceptance. Root must join the
machine helper before native-window ownership of the same wallet. No runtime,
private-input access, Go/build or host action occurred in this source stage.
