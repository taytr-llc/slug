# Set51 independent fighter-history reference

This is a preimplementation Python reference for the frozen CareerHistory v1 contract. It derives history equations and closed transport expectations without new Go output, engine simulation, Argon, signing, mining, chain RPC, private journals or keys. It does not create a trusted runtime handle.

The frozen contract is `inputs/contracts/set51-fighter-history-contract.md` (SHA256 `24d9ef135ca949947c29ed1a887b517209d9ae932ca8286ad9a2ad9046b53e53`). Its status paragraph is historical draft prose; the separately captured root freeze authorizes this reference. Existing career policy, event and Current-prefix domains and canonical bytes are unchanged.

## Provenance and ownership

`input_index.json` binds every captured input by exact SHA256, length, original path/hash, role, and JSON pointer where extracted. `extract_inputs.py` performs only the public extraction. It checks the original Set49 actual and independent reports and the four original public artifact hashes before extracting; it refuses to replace different existing bytes. The main generator verifies the copied inputs, not mutable future paths outside this reference tree. No production DB is opened.

The genuine source is the already completed Set49 attempt3 public evidence. The independent Event is rederived from the saved Result's fixed source and retained first Finality, then compared byte-for-byte to the earlier independent public proof report's Event. The conflict record gives the same Event. The provisional record has no Finality and contributes no event. Winner, loser and empty totals also match the extracted public phase facts. These are normal-PoW **scoped-clock** facts at 1796500801001 (2026-12-05 20:00:01.001Z), not a normal-calendar award or a new proof execution. The original proof report's limitations remain inputs; neither this producer nor its reviewers rerun those chain/signature proofs.

All additional methods, 14/15/16/255/256-event histories, ties, inserted events and generic authority IDs are explicitly modeled. Modeled IDs are deterministic labels, not genuine booked bouts, signatures, admissions or finality. The mixed/unrelated cases exercise prefix inclusion independently of selected-fighter rows. Current-prefix equations/totals for all 28 history cases are independently compared against the minimally copied, previously frozen career producer. That old producer imports only Python standard modules and performs no engine or proof work. Its untouched documentary inputs are retained exactly, including any historical diff whitespace.

`checkpoint_design_audit` authored the extraction, common/core math, history cases, assembly and reproduction runner. `checkpoint_tool` authored `schema_wire.py`, `reference_wire.py`, `state_models.py` and their two fragments. Their role is author, not independent reviewer of those files. The complete first held134-file candidate is preserved outside this directory at `../set51-reference-before-native-discovery`, indexed by SHA256 `836bf113a7f414297442ee01d29dfd7b01ca79482c042211d7f4b23f2dcb9b75`. Its successor adds only an explicit wire-exchange pre-admission scope note and12 finite native discovery cases; the old65 DTOs,67 codec controls,45 exchanges and57 models retain their values. Independent review/freeze is recorded separately by root and a nonauthor reviewer. Authoring predecessors are retained under `preserved/`; they are not accepted runtime evidence. The first assembled candidate was preserved before adding the locally checkable within-page duplicate ResultKey/AwardID refusal. No failed result is relabelled passed.

## Equations and parser scope

`history_math.py` defines exact ordered canonical JSON, activation/award/finality hashes, unchanged Current-prefix framing, standard W/L/D/base-point arithmetic, the XP15 cap, HistoryID and newest-first pages. A row folds chronologically from zero. Its earned XP remains1; its applied XP becomes0 after saturation. An equal-time insertion can change a later capture's fifteenth-XP explanation without changing old Event bytes or an already captured history. Page offsets count newest-first rows and do not reset cumulative totals. Empty and exhausted pages contain an actual empty array.

Every retained event is checked before filtering by as-of; duplicate or malformed events outside the requested prefix cannot hide behind an earlier query. The selected singleton Current prefix includes unrelated events in its digest/count. Each included page row checks its original selected corner/account/gym, event identity, standard contribution, ordinal and local arithmetic. Adjacent rows and within-page ResultKey/AwardID uniqueness are checked. Exact requested page length is enforced only with the request's limit, since an isolated page has no limit field. Only the globally newest ordinal must equal Capture.Totals.

A filtered page cannot prove omitted-prefix completeness or recompute CareerDigest from omitted events. The decoder checks locally available structural/arithmetic/correlation facts and never imports authority from JSON. Whole Capture bytes must remain identical across pages even when HistoryID is unchanged by a mutable captured Status field. The finite native model retains per-ordinal commitments, not full history payloads.

The inherited installed route intentionally restricts `Identity` to the two published fixture identities and network/genesis/descriptor to the compiled candidate values (pinned `gymstable/codec.go` and `desktop/pilot/stable_validation.go`). Full Pin account/source authority IDs remain generic valid identities; a modeled generic-authority case prevents treating historical account IDs as a universal allowlist. These documentary constants load once at module initialization. Transport validation makes no input-file reads or source derivations.

`schema_wire.py` validates all-required exact Snapshot/Job/Page/Response arms and request-aware original full-View/store/activation/job/capture correlation. Wire `exchange` assumes the native lookup request has already been admitted; it does not itself grant discovery authority to an arbitrary job ID. The separate finite NativeDiscovery cases require either an already admitted exact authority/job tuple or a name in the latest same-authority Snapshot, and refuse unnamed, superseded-Snapshot or foreign-authority requests. These are symbolic pre-admission obligations, not actual runtime authorization proof.

`reference_wire.py` traps file reads, history derivation, processes and network after examples are derived and then repeats every transport check. Those traps prove Python parser separation only, not actual Go runtime call counts. The finite state models use explicitly abstract refusal categories where inherited core errors differ; concrete wire error precedence is separately labelled. Synthetic counters, byte accounting and lane transitions are obligations, not race, timing or heap measurements.

## Reproduction

Run from any directory with an existing Python3.9+ installation, with bytecode disabled:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -B generate.py --output corpus.json
PYTHONDONTWRITEBYTECODE=1 python3 -B verify_reference.py --report verification/local.json
PYTHONDONTWRITEBYTECODE=1 python3 -B check_reproducibility.py --attempt new-name --python-a /usr/bin/python3 --python-b /opt/homebrew/bin/python3
```

The generator refuses to overwrite a different corpus. Assembly independently regenerates and compares all three fragment bytes before encoding the final corpus. The serial runner changes Python version, timezone and hash seed, requires two fresh passed reports with the exact corpus and all producer hashes, and preserves logs/failures. A zero-exit child without its exact report cannot pass. Verification neither consults new Go nor changes inputs. The final inventory records the held source, corpus, inputs, authoring predecessors and exact successful reproduction report; the inventory itself has no recursive self-hash.
