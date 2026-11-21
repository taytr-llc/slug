# Set81 CI wire-log parser correction

This is a source-only, unexecuted evidence-parser successor to manifest `77ad8ea5f178f2ea0729ae9e5e08362a648b17d59b5db93c1f7589ae00dab9fb`. The original directory and failed root verification remain untouched. No product, test, report, log, expectation or CI source changed.

Root's first verification failed while decoding the normal wire-selection merged evidence log as JSON. The original gate parses stdout alone, but its retained log merges stdout and stderr. That exact log begins with two ordinary Go download lines, then one complete JSON object:

```text
go: downloading golang.org/x/crypto v0.57.0
go: downloading golang.org/x/sys v0.48.0
```

Both versions match the selected committed `wire/go.mod` and `wire/go.sum`. The successor removes only zero, one or two of those exact LF-terminated lines at the beginning, each at most once, with a 128-byte per-line bound. It then requires one complete strict JSON object using the existing duplicate-key/nonfinite/trailing-data-rejecting decoder. Arbitrary warnings, errors, other modules/versions, duplicate lines, extra objects and suffix text are refused. The original entire log is still checked against its exact byte length and SHA256 before parsing, and rehashed at the end.

The unchanged `expected.json` remains SHA256 `f059d2f4d2988b1c1febebbc1b503d4ce95b6b893335f4671e2fb1fb57cbc2e1`: 470 required PASS assertions, 32 groups, 64 commands, exact source/package/binary/profile/cleanup checks. The only verifier code changes are the bounded helper and the wire-selection JSON call. No pass result is claimed by this source correction.

Root may use this new `verify.py` with the same arguments as the predecessor, choosing a fresh output path for attempt 2. No original report should be overwritten. Source preparation performed AST parsing and hash/byte inspection only; the verifier and parser were not executed here.
