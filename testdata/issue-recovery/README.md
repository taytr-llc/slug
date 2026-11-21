# Set80 independent issue recovery reference

The root runner reproduced `reference-v1.json` under Python 3.14.3 and 3.9.6 before Go implementation began. Its 6,134,980 bytes have SHA256 `4bbe4eb9556ed5a894cee6ff565b0dcca805f27fea2d77cfe3a56242dc3dac91`. All 128 bound producer files stayed unchanged; both interpreter processes joined.

`producer/` contains exact independently authored Python and public inputs. The source-only ready file retains its historical pre-execution status; `qualification.json` and `qualification/attempt5/report.json` record the later successful execution. No production Go supplied expected values. `predecessors/index.json` explains exact reconstruction of the failed producer versions using their complete locks and changed leaves. Every failed attempt is retained under `qualification/`. Attempt 4 completed the Python 3.14 producer, then stopped on macOS launcher diagnostics before Python 3.9 generation. The successor runner selects the installed interpreter directly, without changing producer bytes or checks.

These are conditional byte/state/capacity cases, not evidence of live authority, synchronization, PoW, payouts or host deployment. Original inherited PoW and financial anchors are unchanged. Existing absolute capture paths document original execution; this archive does not promise a portable runtime harness.
