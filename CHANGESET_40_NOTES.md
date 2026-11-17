# Change set 40 — opt-in candidate fixture desktop

Branch: `slug-manifest-desktop`, stacked on set 39 implementation
`aba8bb828f1de8c577b5826050f3bc09c67ea451`.

The separate SLUG Candidate Pilot bundle connects the existing fixture account
and wallet flows to the pinned schema-5 candidate authority and typed durable
miner. Its explicit helper mode and LocalManifestPilot namespace preserve the
Account Pilot and every older namespace. The separately linked bundle selects
candidate mode on ordinary Finder launch; the old bundle default is unchanged.

Login prepares the same retained wallet and verifies at most eight historical
receipts while stopped. Explicit Start mines. A synchronous worker observer
publishes new candidates only after complete verification and durable accepted
synchronization. The bounded cache retains immutable birth, manifest and
portrait digests, detects incomplete recent history, and clears on view change.
Polling and lazy portrait retrieval perform no proof-of-work verification.

The native bridge has separate typed envelopes, preallocation count/depth
bounds, command correlation and exact portrait digest checks. The new frontend
serializes requests, prioritizes controls, preserves focus and discards stale
account responses. It uses the approved logo unchanged. Receipt export pins
the selected public record across the native dialog, rechecks the original
view and writes exclusively. Held sessions can view/mine/export public evidence;
recovery export remains forbidden. Invalid custom data paths are refused before
directory creation, and old managed roots cannot be adopted.

## Verification and scope

Independent reviews cover lifecycle/lease ordering, durable observer timing,
cache bounds, protocol parsing, native mode/export behavior and the new DOM
flow. Review fixes include a Start/logout race, stale history coverage status,
an export-directory identity window, and the native parser/path checks. Normal,
race, tagged, fuzz and frontend results retain source hashes in workspace set40
evidence. Legacy source preservation is checked explicitly.

The actual helper composition uses both published wallets and normal-PoW owned
loopback nodes. It verifies live portraits before Stop, later work following a
canonical block win, exact selected receipt export, cold reopen equality,
reset-held mining and cleanup. Independent frozen algorithms check proofs,
signatures, complete DNA/static binding, original birth and exact SVG bytes.
The final committed bundle, integrated regression gates and required per-set
smoke are recorded separately before deployment. A fresh height-61 check
precedes promotion, followed by a full 15-minute seed observation.

`MANIFEST_DESKTOP.md` preserves the approved contract. Its packaging addendum
records the separate bundle default. Its acceptance addendum precisely splits
lost-ACK evidence between set39 real normal-PoW recovery, set40 historical
observer tests and actual packaged cold reopening; the packaged helper itself
does not inject a lost HTTP acknowledgment.

The bundle is ad-hoc signed, not notarized. Native chooser interaction, pixel
rendering, actual screen-reader acceptance and Mike's joint D74 review are
uncompleted gates. There is no official descriptor activation, ownership or
roster authority, real signup/custody, production genesis or launch approval.
Railway continues to start only slugd with the existing volume and loopback RPC.
