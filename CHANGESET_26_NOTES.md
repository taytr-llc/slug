# Change set 26 — desktop wallet and recovery pilot

Implemented on `slug-desktop-wallet`, stacked on set-25 implementation
`11664083bd87d33b7a127d7950cf170561543c3f`, paired with the separate wallet topic
published at `f7cb5fb872f00a0bb476cf4f65d96e4987428063` (based on
set25 `1c7b4ff2fb67ab6230b9de09a6b097e046c28661`). No PRs.

## Behavior

The new Go gym shell integrates the published recovery-wallet fixture through
actual btcwallet RPC for payout addresses and public balances/history. The
supervisor retains an immutable payout and wallet-derived gym, with signing and
recovery contents kept in process. New `LocalWalletPilot` data and a v2 manifest
preserve the old pilot and reject cached pre-upgrade chains. Runtime startup
requires the block-62 activation metadata as well as network and genesis.

Native Save/Open dialogs select recovery files. The webview cannot supply paths
or receive recovery contents. Export reads retained encrypted material; backup
verification restores asynchronously into a fresh wallet while mining is stopped.
The existing wallet and receipt/outbox data remain intact. The paired wallet
server gains an opt-in copied RPC method allowlist, preserving unrestricted
legacy behavior only when the option is nil.

## Deliberate limits

This is a public, deterministic fixture on owned loopback nodes. D70's 24-word
English phrase, no extra phrase passphrase and separate encryption password
remain unchanged. No production derivation allocation, real key generation,
hosted login, production genesis, fighter transfer, calibrated prospect policy,
public mining or wallet deployment to Railway is included. Recovery does not
restore off-chain receipts. The macOS bundle is ad hoc signed, not notarized.

## Verification

- Frontend behavioral suite: eight tests pass, including argument-free native
  recovery actions, serialized requests and polling during asynchronous backup
  verification with mining disabled.
- Full pilot race suite passed 204.946 s after reducing repeated structural-test
  fixture KDF setup. A prior five-minute attempt was still computing scrypt;
  an intermediate fixture-copy filename error was corrected. No production
  KDF/PoW setting changed. Targeted cleanup/cancellation follow-ups passed;
  `set26-controller-checks.json` records source hashes and exact evidence.
- Public bridge/protocol/HTTP focused races passed 7.580 s/1.461 s. Native package
  tests passed, including refusal to overwrite managed data through parent
  aliases. All three native file operations now share known-data-root guards.
- Wallet RPC complete race passed 1.402 s, six module builds and vet passed.
  The adapter's retained export/fresh restore/HTTP/identity tests passed 63.863 s;
  a final manager-cache close regression passed 51.707 s. Evidence:
  `set26-wallet-rpc-checks.json`. Exact-source wallet CI passed on f7cb5fb8: run 43939704270,
  full wallet race 777.252 s, waddrmgr race 4.458 s and all three normal-PoW gates.
- The normal-PoW desktop Go gate passed 462.24 s (package 462.606 s): mine through
  height 100, retain 174 attempts/receipts, export current receive cursor 26 after
  reopen, verify an asynchronous fresh backup, then restore from that file with
  the original DB inaccessible. Exact identity, funds, UTXOs and transaction
  history matched; the restored wallet confirmed a real RPC spend at 101.
  The launcher was accidentally edited while running and the shell attempted
  a stray command after the successful Go process, yielding wrapper 127. This
  is explicitly recorded, not represented as a clean wrapper pass. Later
  first-Start/immediate-Quit proof passed 3.14 s on final lifecycle sources with
  unchanged identity/payout, no mining and no leftover processes. Source
  differences and process cleanup are in `set26-desktop-wallet-smoke.json`.
- Native GUI checks passed with an isolated public fixture: no launch mining,
  Start/Stop, seven blocks/70 immature SLUG/11 receipts, Quit and reopen with
  exact retained identity and payout, export cancellation, later recovery save,
  asynchronous verification progress/success and final Quit with no owned
  processes. Recovery output was 0600 and contained 24 fixture words. Native OS
  file preview may display the selected fixture file; this is not a real-custody
  reveal guarantee. See `set26-native-gui.json`.
- Final independent review found the inherited two-hour wallet and 24-hour GBT
  currentness barriers prevented an idle local chain from restarting. The
  isolated-node solution passed an actual 26-hour old-tip gate: backup rescan
  retained height 1 unchanged, then explicit Start mined height 2 with two
  receipts in20.89 s. The same binary's ordinary mode rejected old-tip GBT with
  initial-download error-10 in1.79 s. Combined package 22.995 s, wrapper 0, no owned
  processes remained. Immutable node SHA256:
  `bd38e17fd71b4977de19fb5d156de5835b8900b6ae8f51fc6fc2f754bef231c4`.
  Before-fix default behavior was also verified on set25. See
  `set26-desktop-stale-smoke.json` and companion source/exit/cleanup evidence.
- All ten chain modules build after the new node option. Complete root race
  passed 3.657 s and vet passed; focused isolation tests passed 0.370 s/1.417 s
  normal/race. No chain consensus path changed. Node mode is never enabled
  on Railway. Final artifacts and seed rollout are recorded separately.

## Idle private-chain recovery

The inherited wallet waited for a tip newer than two hours, while node templates
required a tip newer than 24 hours. A stopped private chain has no miner to
advance either clock, so ordinary startup and fresh restoration could deadlock.
The wallet adapter now uses a private, validated local-chain readiness wrapper.
The node has an explicit isolated-pilot template option with strict startup and
runtime peer/header guards. The ordinary node and wallet behavior stay intact;
no consensus rule, PoW profile or signed-checkpoint policy changes, and no
hidden bootstrap block bypasses the worker's durable job/outbox flow. The
actual 26-hour old-tip restoration/restart regression passed, including ordinary
mode refusal and no implicit block during backup verification.
