# Daily synchronous signing check

Only a fresh explicitly constructed Store with private daily state and Config2
can mint the private synchronous daily request. The legacy private key and
CheckSigningRequest never accept this mode. The daily request uses a distinct
context key, owns complete config/source/input/payment/change slices and raw
bytes, and invalidates immediately when the original backend Sign returns.

Canonical request body is the existing ordered signingRequestBody grammar:
version=2, config, source, inputs, payments, fee_atoms, change, unsigned. Coin,
Claim, Fraction, Payment and Config retain their exact existing ordered JSON
fields. Request digest is SHA256("SLUG operator pool daily signing request" +
NUL + "v2" + NUL + canonical body JSON), without a length prefix or LF.
Stream one scalar/Claim at a time; never serialize the complete body into RAM.

Input count1..1464, payees1..256, input claim count1..32768 total and optional
change claim count1..32768. Each Fraction scalar stays <=4096 decimal digits;
all hash/script/gym string lengths retain their closed existing limits. Before
copying/encoding, reject unsupported shape/count/scalar lengths. An independently
calculated conservative encoded bound uses65536 claims, each<9KiB,1465 coin
headers at512bytes,65536 config bytes,8192 source bytes,256 payment rows at512,
800000 unsigned-hex bytes and1024 fixed syntax bytes, strictly below640MiB.
This is a serialization bound, not permission to omit claims or accept arbitrary
DTOs. It follows the source lifetime proofN+R<=32768; active inputs+change may
repeat the same leaves. Stream through a640MiB counting sink; overflow is error.

CaptureDailySigningCheck performs complete config/source/coin/raw equality and
streamed owned-body digest comparison under the original private active request.
It returns an opaque concrete handle; zero/caller-created values refuse. Check
per input consults only original active flag, parent cancellation, immutable
mode/identity and Store lifetime. No new context can revive an old request.
Recheck performs full equality/digest again before final disclosure. No callback,
foreign owner, copied database, DTO, or retained post-Sign handle grants signing.
No Store gate/DB/RPC is acquired by Check/Capture/Recheck.

CheckDailyBackendBinding is constructor-only and audits original writable Store,
concrete backend pointer, selected Config2/source and full original ledger. It
never runs inside Sign. Legacy binding expressly refuses a daily owner.

The independent corpus below uses only published fixture inputs and independently
computed conditional99/1 carry for a one-input transaction; it is a byte/digest
reference, not a mined-credit or actual daily signing proof. Three public fixture
wallet variants and a null-change serialization control cover the stream codec.
