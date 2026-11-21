# Receipt lookup relay diagnostics (source only)

This successor is staged separately from the frozen actual helper and checker. It composes with `a2-lookup-diagnostic-successor` (manifest `6f30cd172e62e2094ef0bc87a25f161e96e9c7f844d73e947cfbf2a6d7d7e59b`). It adds one new helper source, `receipt_lookup_relay.go`, and narrow hooks in the original relay and final cleanup. Root alone installs/builds/executes the new immutable instrumented helper tree.

Only `/pool/v3/lookup` is observed: the original selected recoverable Client.Lookup uses this route through pathPrefix. `/pool/v2/lookup` belongs to the prior daily mode. No issue/issue-lookup behavior, original report/tls_relay key set, retry predicate, response forwarding, client validation, or deadline is changed.

After the original relay server and all its handlers join, cleanup writes `receipt-lookup-wire.json`, independently of `actual-report.json`. It is available on both success and failure whenever the relay was constructed. The old checker continues to see the original report shape. The new wrapper additionally binds this file and the separate `receipt-lookup-evidence.json` client trace.

## Public schema

Top-level required fields: `version` (1), `path` (`/pool/v3/lookup`), `max_events` (128), `dropped` (saturating uint64), and `events` (array, at most128). Events reserve their ordinal at request start and are completed before joined final projection; concurrency cannot overwrite another slot. Overflow records only dropped count and does not change the original request's forwarding. A complete proof must reject dropped>0.

Every event has the fields declared in receiptWireEvent. `request` is null or canonical ordered job_id/nonce with a64-character lowercase hexadecimal job ID; request_bytes/request_sha256 describe the original bytes. Method and origin are booleans, not copied URL/header data. Start/end times bracket the whole original handler. `forwarded` means the original upstream Do was invoked, not that it succeeded.

`status` is the original upstream HTTP status (zero if none). `json_content_type`, `no_content_encoding`, and `declared_length_fits` reproduce the original client's framing checks as booleans only; no raw headers are exported. Response byte length/SHA cover the bytes actually read under the unchanged original relay cap; `body_complete` distinguishes a complete read from a truncated/capped body. `response_kind` is none, invalid, read_error, oversize, receipt, or error. Typed canonical bounded receipt/error projections contain only closed public fields. Malformed or oversized bodies are never exported. Receipt parsing is a shape check, not authentication, work validation, or authority; `selected_binding`/`request_binding` are diagnostic comparisons only. The original Client still makes all actual decisions. Error projection records status/unknown consistency separately, without laundering a mismatch into a valid response.

`failure_phase` is empty or origin/request_read/request_create/upstream/response_read_close; `failure_kind` is empty, invalid, canceled, deadline_exceeded, or other. Read/Close errors remain separate booleans. Request-context and downstream-write errors are classified without printing arbitrary error strings. Downstream byte count records the observed original Write return. No headers, bearer credentials, private keys, runtime DB contents, or opaque body bytes are included.

The new public file has the existing16MiB artifact cap; 128 receipt frames each bounded64KiB leave ample room. The original generic relay read cap, forwarding, and outcome semantics are unchanged. Successful wrapper qualification still requires exact client receipts and original financial/owner proof; diagnostics do not authorize retries or acceptance of unavailable/not-found.

## Validation scope

Source-only review and exact preimage/diff/hash checks. No Go, gofmt, runtime, HTTP request, reference execution, or active-source mutation by this lane. The paused original helper/checker and the previously qualified A2 readonly diagnostic are unchanged. Root will perform formatting, compilation, and runtime qualification after independent source review.
