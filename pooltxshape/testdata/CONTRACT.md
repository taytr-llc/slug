# Native daily transaction shape — additive public API

Frozen source-only proposal for public package `github.com/taytr-llc/slug/pooltxshape`. Both the chain and separate wallet module may import it. It validates a finite transaction shape only; it supplies no signing, key, funding, source, account, transaction-construction or broadcast authority. No existing V1 call site,256-input/100000-byte rule or codec is changed.

```go
const MaxNativeInputs = 1464
const MaxPayees = 256
const MaxStandardWeight = 400000

type Shape struct {
    StrippedBytes int64
    WitnessBytes  int64 // includes the two marker/flag bytes
    RawBytes      int64
    Weight        int64
    VSize         int64
}
func NativeP2WPKH(inputs, payees int, change bool) (Shape, error)
```

Inputs and payees must each be positive, within1464 and256 respectively. Each input is an empty-scriptSig native P2WPKH spend with a conservative73-byte signature including sighash and a33-byte compressed public key. Every output, including optional change, has a22-byte native P2WPKH script. This function cannot size another script family or an already serialized arbitrary transaction.

With `n=inputs`, `o=payees+(change?1:0)` and canonical CompactSize encoded length `C`:

```text
stripped = 4 + C(n) + 41*n + C(o) + 31*o + 4
witness  = 2 + n*(1 + 1 + 73 + 1 + 33) = 2 + 109*n
raw      = stripped + witness
weight   = 4*stripped + witness
vsize    = ceil(weight/4)
```

400000 is inclusive. Positive in-domain counts whose conservative weight exceeds400000 return `ErrCapacity`; malformed/out-of-domain counts or checked-arithmetic failure return `ErrInvalid`. **Every error returns zero Shape**, not partial dimensions. Invalid counts are rejected before adding change or multiplying, so machine-int extremes cannot wrap. Dimension arithmetic uses checked nonnegative int64 addition/multiplication. No float, fee, weight-to-raw substitution or vsize-as-raw cap is used.

This is additive and selected only by later explicitly configured V2 profiles/APIs. A positive result does not authorize widening any old signer/parser bound. Later consumers must still validate actual transaction bytes, scripts, signatures, source obligations, sums, fee and original custody. A conservative size refusal can defer a shape that shorter actual signatures might fit; no input/payment subset is selected here.

Independent hand checks fixed before producer execution:

| Inputs | Payees | Change | Stripped | Witness+marker | Raw | Weight | Vsize | Result |
|---:|---:|:---:|---:|---:|---:|---:|---:|:---|
|721|64|yes|31588|78591|110179|204943|51236|allow|
|721|256|yes|37542|78591|116133|228759|57190|allow|
|1348|256|yes|63249|146934|210183|399930|99983|allow|
|1349|256|yes|63290|147043|210333|400203|100051|capacity|
|1366|218|no|62776|148896|211672|400000|100000|allow|
|1366|219|no|62807|148896|211703|400124|100031|capacity|
|1464|1|yes|60098|159578|219676|399970|99993|allow|

The721-input shapes demonstrate raw bytes above100000 while weight is standard. They do not establish a reachable V1 ledger, daily production funding, or approval of automatic online signing. The1366-input case proves exact inclusive400000 semantics. The1464 maximum follows the smallest supported positive-payee shape:1465 inputs already exceed standard weight, even without change.

The independently authored Python producer builds bounded symbolic byte strings from transaction field sizes and CompactSize encoding. It imports no product code and derives no expected result from Go. Symbolic witness bytes are not valid signatures. The literal input recipes include721/64,721/256,1348/256 versus1349/256, input/output CompactSize252/253, minimum/maximum counts, exact weight boundary and invalid counts. Go tests additionally pass machine-int extrema, without allocating slices proportional to them.

Producer and input lock must be reviewed before root executes Python. No Python or Go execution has occurred in this draft lane. Tests consume the eventual frozen corpus; its SHA must be installed into the deliberately empty test constant after approved producer execution, before qualification. No fallback skips that guard.
