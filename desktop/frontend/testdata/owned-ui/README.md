# Owned gym frontend behavior checks

Run with Node from any directory, providing a fresh absolute artifact directory:

```sh
node desktop/frontend/testdata/owned-ui/run.mjs /absolute/path/to/owned-ui-check
```

The runner hashes the exact checkout and the independently frozen owned-consumer
corpus before execution. Its twelve cases exercise the actual frontend module
inside a small DOM harness, with native calls mocked. They cover old-view replies,
cleared identity and balance displays after disconnection, reset holds, exact
saved requests, separate native consent, delivery versus finality, and bounded
roster/entry actions. Sources are checked again after execution.

This is behavioral qualification, not a visible-browser rendering, Wails dialog,
real helper/node, cryptographic, or wallet-spending test. It generates no expected
cryptographic values. The public corpus's own provenance remains authoritative.
