# Public ownership evidence transport

Set73 finite candidate read protocol. The independently retained full ownership
pin and endpoint select a source. Its declared frontier is not a claim of
worldwide completeness. Signatures, normal header work, transaction inclusion,
history and finality are still verified by the concrete replay owner.

Only `POST /slug/ownership/v1` exists. Request canonical ordered JSON fields are
`version,pin_id,id,method,sequence,event_id`: version1, exact expected PinID,
positive uint64 request ID, one of `status,admission,publication,finality_frame,
cutoff_intent,cutoff`. Sequence is uint32, positive and bounded by1000 admissions,16384
publications or4096 frames for the three numbered methods; zero otherwise. EventID is lowercase nonzero64hex for the two cutoff methods
and empty otherwise. Maximum request body1024 bytes. Unknown or duplicate
members, noncanonical encodings, alternate paths, query strings, redirects,
compression and other HTTP methods are refused.

Response ordered JSON is `version,pin_id,id,method,result,error`. Identity and
method must exactly echo the original request. Success has error empty and one
non-null canonical existing public object as result. Failure has result null
and one fixed code: `invalid,busy,closed,not_found,unavailable`. No database paths,
secrets or detailed internal errors are returned. HTTP status200 is used only
for this exact response; malformed HTTP requests receive a generic400/405/404.
Content-Type is application/json and Content-Encoding must be absent.

Status ordered fields are `version,pin_id,publication,finality_sequence,
published_finality_sequence,finality_body_head,finality_head`. Publication is
the existing registry PublicationStatus. The body head names the latest durable
unsigned finality body; finality_head names the last published signed frame.
Zero sequence iff zero64hex head. Signed counts cannot exceed durable counts.
The server holds the original core mutex while reading the concrete registry's
status, yielding a coherent snapshot; contention returns busy. A signing gap
is explicitly visible and prevents a complete remote replay. A mutable status
never modifies a previously captured source snapshot.

Per-result limits in bytes: status2048, admission69632, publication8192,
finality_frame8192, cutoff_intent4096, cutoff8192. Envelope overhead is bounded
by2048. Reads use Content-Length rejection and a cap+1 bounded stream before
decode. Existing canonical object validators and signatures are unchanged.

The concrete reader retains one fixed URL and original parent lifetime. Plain
HTTP is allowed only for literal loopback IPs. HTTPS uses ordinary certificate
and hostname validation plus the independently configured lowercase SHA256
of the leaf certificate's SPKI. No proxy, redirect, automatic retry, cookie jar,
compression or reusable connection is enabled. Each operation has a five-second
deadline. At most eight operations are active; excess returns busy. Close
cancels and joins admitted operations. Callers own the HTTP listener/server and
must shut it down before closing the borrowed core and publication stores.

The server borrows exact live core/publication owners selected by its creator.
It exposes only committed admission/publication rows, signed frames and retained
cutoff objects. It exports no account state, allocator/payment journals, private
authority database or locally verified ownership-map flags. Remote imports are
untrusted evidence until replay independently verifies all historical branches.
No remote method signs, captures a cutoff or mutates ownership. A copied private
source database cannot satisfy independent recovered-gym acceptance.
