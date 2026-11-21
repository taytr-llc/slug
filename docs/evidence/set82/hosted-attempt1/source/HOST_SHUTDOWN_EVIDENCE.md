# Source review and hosted shutdown evidence

Source-only note, 2026-11-21. No provider call, Go execution, signal or host inspection was performed for this review.

The node build context `node-context-attempt1.json` has12 exact entries, all lengths/SHA256 verified and no extra context files. Dockerfile SHA256 is `d1940524aa487969d0185b4c51244b1869c10d5af6d89109d8fc6fc7bead9bca`; original normalCGO0 `slugd` is `c1b66721bc6ce368a5fced3265829c68719596023b53400aa4de39ca713cf04e`. The source recipe checks native Linuxamd64, the pinned node binary/profile, readonly helper build/profile, tests under UID10001, and unchanged selected inputs afterward. It does not itself prove provider volume or shutdown behavior. Linux execution/results are owned and recorded separately by root.

## Process owners

- **Operator:** the final scratch image's PID1 is original `slugpoold`, UID10001. SIGTERM is handled by its original command context. `RunCommand` joins Runtime, wallet, authority and selected file owners; an independent failure is not converted to a clean cancellation. `serve` does not emit a final successful `complete` JSON record. Exit0 is therefore needed alongside its public status/errors and closed reopens; absence of an error line alone is insufficient.
- **Node:** PID1 is now `slug-node-fixture`, not `slugd`. After fresh bootstrap it runs as10001, requires mTLS on the public P2P relay, and owns exactly one original `slugd` child with one Wait. SIGTERM must target the helper PID1. The helper closes/joins relay connections, then TERM-waits30s for the original child; KILL plus5s is only failure cleanup. Unexpected child exit, relay failure or timeout yields failure. Killing the child directly is deliberately not an accepted clean shutdown.
- **Mac bridge/miner:** retain their original locally owned process handles and join them. Stop the miner before stopping the operator; keep the node available until operator cleanup is complete. Stop the bridge and hosted node only when no remaining original owner depends on them.

The node image defaults to USER0/CMDbootstrap for its first empty volume. Merely changing CMD to `serve` fails because `ownedRoot` requires10001; automatically restarting default bootstrap also refuses an existing tree. Before reopen, select an explicitly non-root final stage or a verified fixed `su ... serve` start command. Do not assume the command silently drops privileges, and keep automatic restart disabled during this handoff.

The first hosted bootstrap retained a prepared node root but the0711 mount blocked the relay's descriptor-relative ancestor read before child startup. The separately selected `package/node-access.sh` handoff handles only that owner-free state: exact nine input files, no data/log directories, mount-only0755 correction and explicit10001 original `serve`. It is not a rebootstrap or permission repair under a live owner. Its public result binds metadata checks, not a hash/read of protected contents. The pre-correction documents remain in `package/before-mount-read-access/`.

## Public capture and safe control surfaces

Known project: `62fdb666-7f67-4cf3-98f0-6177f099b580`. Known isolated node service: `c7ba8d73-fe01-418e-8f7b-14a141da8043` (`slug-pool-node-test`). Resolve the operator's exact service/deployment IDs from root's existing public deployment record; never target the seed service or infer an ID from a name prefix.

For each generation, record the exact environment, service and deployment ID, image/build identity, restart selection, start/signal/exit timestamps and bounded public log identities. The Railway connector's existing read schemas provide:

```
list_deployments({projectId, serviceId, environmentId, limit: 10})
get_logs({projectId, deploymentId, types: ["deploy"],
          startDate: <captured UTC>, endDate: <captured UTC>, limit: 500})
```

Use explicit deployment IDs rather than service-wide mixed logs. If logs exceed one bounded response, retain every selected interval/page and detect truncation. Read status/logs, not service variable values. These schemas expose deployment status but no process exit-code field or stop operation. They therefore cannot alone establish clean shutdown. Root must use its verified provider stop mechanism and retain its actual signal/exit evidence; `restart_service` is not a join barrier and must not race an audit.

The node image contains a shell, so an authorized SSH command can inspect only `/proc/1/exe`, selected fields of `/proc/1/status`, and `/proc/1/task/1/children` to bind the expected helper and10001 UID/GID before a single PID1 signal. If using that route, require the exact `/usr/local/bin/slug-node-fixture` executable before `kill -TERM 1`. Do not print `/proc/*/environ`, full command arguments, protected key/certificate bundles, passphrase/token files or directory contents. An SSH disconnect after signaling is not an exit/join acknowledgment.

The scratch operator has no shell or kill utility. Use the provider's correctly scoped stop rather than assuming the node SSH command works there. The locally installed CLI's stop syntax must be checked by root before invocation; this source note does not invent an unverified CLI/API mutation. Preserve the deployment and volume history, not a delete/recreate/reset sequence.

## Evidence required after a stop

Require the original process termination result and bounded cleanup result, not merely `REMOVED`/stopped or a subsequent successful deployment. The normal node's `Server shutdown complete` log follows `server.WaitForShutdown`; it corroborates the child's join, but alone does not prove PID1 relay cleanup or subsequent database close. Operator exit0 corroborates its original joined cleanup; then run the already planned full stopped-owner audits/reopen against unchanged selection and complete histories. A lock released by forced kill can reopen too, so successful reopen does not retroactively prove a clean join.

If the provider does not expose enough exit evidence for these source-supported conclusions, retain that explicit host-evidence limitation and have root select the smallest fixture-only observation mechanism before claiming the hosted acceptance. Do not add general production telemetry or relax cleanup/financial checks in this note. All old failed attempts and exact original source/profile bindings remain preserved.
