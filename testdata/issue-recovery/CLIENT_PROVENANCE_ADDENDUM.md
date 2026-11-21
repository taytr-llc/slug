# Set80 original client and forwarding custody

Root's implementation selection supplements approved contract
`f3e019a840d6b69d853350763364a3fc41f10ea899f00ed6f164db5d58a95610`.
It changes no serialized field, marker, hash domain, response limit or economic
rule. Product authoring remains gated on the qualified independent reference.

The new worker retains both the exact `*poolservice.Client` and the existing
private fixed-operation forwarding wrapper. Calls continue through the original
`remotePoolAuthority` Mining/View fence, then `remotePoolPoolLease` protected-file,
credential-expiry and cancellation fence, then that same concrete client.
Passing the raw client directly as the call path would skip those fences and is
not the selected implementation. No exported credential getter, generic callback
hook or alternative account/signer owner is introduced.

The service lane owns these additive declarations:

```go
type RecoverableDailySelection struct {
    Version              uint32 // wrapper 1
    BaseVersion          uint32 // DAILY 2
    URL                  string
    PublicKey            string
    Source               poolshare.Config
    IssueProfile         registry.IssueProfile
    SettlementConfigID   string
    RecipientScript      string
    BaseTransportTrustID string
    TransportTrustID     string
}

func (c *Client) CheckRecoverableDailySelection(expected RecoverableDailySelection) error
func (c *Client) CheckIssueResponse(requestID string, response IssueResponse) error
```

The worker derives this public tuple from its complete independently selected
`RecoverablePoolConfig`. The constructor requires exact original client mode,
base version, source, profile, key, money and both transport identities. The base
trust ID already commits the validated original endpoint/CA policy and full
DAILY source/key/money binding; the outer trust adds the exact issue profile.
There is no dummy bearer token, credential export or need to pass a secret into
the worker merely to compare public identity. Client construction still validates
the complete actual client config and owns its original credential privately.
This identity check does not grant a session or replace any live owner fence.

`IssueResponse` carries an unexported, immutable provenance receipt in memory.
Only the concrete client mints it, after bounded strict decoding and complete
request/profile/source/money/gym/key/result validation. It binds the exact client
pointer, requested ID and canonical response bytes. Retain an owned canonical
byte copy, or a purpose-domain digest plus exact length over those same bytes;
either implementation must compare the complete response, not selected fields.
The receipt contributes no JSON bytes and is never serialized or minted by a
public parser. A zero value, JSON round trip, other-client receipt, changed request
or altered response must fail `CheckIssueResponse`.

The private wrappers forward the exact response value, including its opaque
receipt. Before durable completion the worker checks the response against its
retained original client and takes an owned bounded copy. Under the original
outbox mutex it then rechecks the same Run/attempt pointer, parent context,
selected wrapper, active request ID, running/closing/poison state and original
session/custody outcome. No mutex/account lease spans the network or complete
response validation. Only then may one sync retain the response and clear the
active ID, with the scan/cursor for work. Any observed wrapper loss still prevents
publication, even when the lower client's response was valid.

After persistence, the receipt is deliberately absent. Full selected outbox
audit validates exact q/result/scan/e linkage and canonical retained bytes; it
does not manufacture a live receipt. A later Resume obtains a new receipt from
the same explicitly constructed current client for the exact request. Historical
local reads do not need network authority, and Open/Snapshot still cannot resume.
Legacy DAILY constructors refuse the already-open new outbox/client selection.

The worker lane adds `RecoverableDailyAuthority`, containing the existing fixed
Authority operations plus `ResumeIssue` and `LookupIssue`, both with the selected
client signatures. `NewRecoverableDailyPool` takes the selected worker wrapper,
the concrete client, that forwarding authority, and the unchanged admission,
node, signer and original outbox arguments. The private pilot construction has
both objects already; neither escapes through a getter. The concrete pointer and
receipt prevent an arbitrary interface implementation from manufacturing an
accepted Issue result. Existing strict work/receipt validation and private
operation fences remain on all other forwarded calls.

Required focused controls: original-client positive; identical-config second
client refusal; zero/decoded/changed/nested-mutated response refusal; wrong
request/profile/source/money/trust refusal; canceled or replaced Run and changed
credential/View after a valid lower response cannot commit; valid exact result
persists without receipt bytes and reopens under full audit. Real TLS/headless
acceptance must still prove the complete original-owner path. No new provenance
receipt alone proves durable issuance, current session authority or payment.
