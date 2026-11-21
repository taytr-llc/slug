# Disposable hosted operator lifecycle witness

Source-only, no execution qualification. This is a fixed public-fixture control script for the current temporary Alpine admin image. It runs the unchanged original `slugpoold` as UID/GID10001 and retains its actual child wait result. It does **not** qualify final scratch PID1 shutdown, Railway service stop semantics, a general daemon supervisor, or production secret custody. The original scratch image and existing bootstrap remain unchanged.

Root installs the exact separately hash-bound script at `/usr/local/bin/host-operator-lifecycle`, root-owned0555, with an exclusive write before starting any product owner. This installation is an explicitly recorded runtime overlay; it is not part of context4's immutable image manifest. Verify its hash after installation and after the attempt. No protected input is generated, replaced, displayed or rewritten by the script.

Railway SSH currently starts its command as root even though the image and its idle PID1 use10001. Every invocation must therefore use an explicit fixed `su -s /bin/sh -c '<quoted command>' slug`. Quote the entire SSH command with the existing host-command adapter's `shlex.join`; do not rely on SSH argv preservation. The script refuses any other real/effective UID/GID.

## Fixed interface and evidence

```
host-operator-lifecycle run 01 ABSOLUTE_UNIX_DEADLINE SETUP_SHA256 BINARY_SHA256
host-operator-lifecycle stop 01 SETUP_SHA256 BINARY_SHA256
host-operator-lifecycle status 01
```

`SETUP_SHA256` is the independently checked public selected-setup identity. `BINARY_SHA256` is the already qualified original operator binary identity. No token, passphrase, seed or key is passed as an argument. The exact standalone serve flags are literal in the script and point only to the prepared original `/data/hosted-test/operator`, schema16 wallet and protected inputs.

The root-selected absolute scenario deadline is at most900 seconds away when generation01 begins. It is written once and must be unchanged for generations02/03; restarting does not renew the budget. Generation02 requires generation01's successful completion and identical setup/binary; generation03 similarly requires02. Each generation has a fresh0700 evidence directory. An exclusive `lifecycle/active` directory prevents overlapping generations and is retained after failure. Never remove it to turn failure into success or start another owner. All original operator histories remain in place.

`run` starts a dedicated session with BusyBox `setsid`, checks its actual session/group identity, and owns one unchanged original operator child. The monitor shares and anchors that group until the original wait returns. It never signals an unrelated PID obtained from external input. The separate `stop` checks saved PID/start time/parent/group/session, UID, executable and selection before writing an exclusive generation stop marker. It does not signal externally. The monitor records the request then sends TERM to its own group; the shell trap records the interruption and the original child's final `wait` result remains authoritative.

The monitor permits30 seconds after TERM. If the child does not return, it records forced failure before KILLing its anchored group. There is then no clean final result and no permission to audit/reopen as if joined. The separate stop caller itself has a30-second observation bound. A deadline or external interruption is a failed attempt even if the original binary subsequently exits0. The parent fixture runner must also retain its original outer bound and must not terminate the admin idle PID1 while an operator generation is active.

Only a result with actual `child_exit:0`, `monitor_exit:0`, `child_waited:true`, `monitor_joined:true`, `reason:"requested"`, `clean:true`, unchanged setup/binary and the final `owner.complete` marker admits the next stopped-owner audit/reopen. Capture stdout/stderr and their hashes; a normal successful operator should retain empty stderr. The script supplies process evidence, not financial acceptance. Original full stopped audits, exact wallet/payment evidence, no duplicate send and same-root reopen checks remain separately mandatory.

BusyBox `setsid` can detach if SSH made its command a group leader. Therefore neither SSH return0 nor SSH disconnection establishes join. Read the bounded generation result and completion marker through a separately selected SSH invocation. A missing/incomplete result, nonzero original exit, forced kill or retained failed lock is failure. Actual shell/BusyBox compatibility and TERM-interrupted `wait` behavior must be checked by root before accepting host evidence; this source preparation performed only `sh -n`.

All output files are public process evidence in the protected fixture tree: numeric process identities/timestamps, selected public hashes, original operator public stdout/stderr, stop/TERM/forced-failure markers and final JSON. The script does not inspect environment variables, full command lines, credentials, private wallet values or database rows. Evidence retention after this finite attempt is root-owned; no automatic deletion or retry is provided.
