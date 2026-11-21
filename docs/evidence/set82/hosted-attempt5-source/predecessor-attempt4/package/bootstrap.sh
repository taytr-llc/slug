#!/bin/sh
# Disposable public-fixture administration only. Never used by the serve image.
set -eu
umask 077
PATH=/usr/local/bin:/usr/bin:/bin
export PATH

fail() { printf '%s\n' "host bootstrap: $1" >&2; exit 1; }
uid10001() {
    [ "$(id -u)" = 10001 ] && [ "$(id -ru)" = 10001 ] &&
        [ "$(id -g)" = 10001 ] && [ "$(id -rg)" = 10001 ] || fail 'UID/GID10001 required'
}
fresh() { [ ! -e "$1" ] && [ ! -L "$1" ] || fail 'selected fresh path already exists'; }

[ "$#" = 1 ] || fail 'one explicit action required'
case "$1" in
prepare-dirs)
    [ "$(id -u)" = 0 ] && [ "$(id -ru)" = 0 ] || fail 'one-time root preparation required'
    [ -d /data ] && [ ! -L /data ] || fail 'real /data volume required'
    awk '$5 == "/data" { found = 1 } END { exit !found }' /proc/self/mountinfo || fail 'attached /data mount required'
    [ "$(stat -c '%u:%g:%a' /data)" = 0:0:755 ] || fail 'existing prepared mount mismatch'
    [ "$(awk '/^Uid:/ { print $2 ":" $3 ":" $4 ":" $5 }' /proc/1/status)" = 10001:10001:10001:10001 ] || fail 'nonroot idle deployment required'
    prior=/data/hosted-test
    prior_selection=/data/hosted-selection
    for entry in "$prior" "$prior_selection" "$prior/operator"; do
        [ -d "$entry" ] && [ ! -L "$entry" ] &&
            [ "$(stat -c '%u:%g:%a' "$entry")" = 10001:10001:700 ] || fail 'retained attempt1 directory mismatch'
    done
    intent=$prior/operator/SLUG-POOL-OPERATOR-ISSUE1.json
    [ -f "$intent" ] && [ ! -L "$intent" ] &&
        [ "$(stat -c '%u:%g:%a:%h' "$intent")" = 10001:10001:600:1 ] || fail 'retained incomplete intent mismatch'
    [ "$(stat -c %s "$intent")" -gt 0 ] || fail 'empty retained intent'
    [ ! -e "$prior/operator/SLUG-POOL-OPERATOR-READY-ISSUE1.json" ] &&
        [ ! -L "$prior/operator/SLUG-POOL-OPERATOR-READY-ISSUE1.json" ] || fail 'attempt1 is already ready'
    [ ! -e "$prior/lifecycle/active" ] && [ ! -L "$prior/lifecycle/active" ] || fail 'prior lifecycle owner or failed lock remains'
    prior2=/data/hosted-test-2
    prior2_selection=/data/hosted-selection-2
    for entry in "$prior2" "$prior2/private" "$prior2/operator" "$prior2_selection"; do
        [ -d "$entry" ] && [ ! -L "$entry" ] &&
            [ "$(stat -c '%u:%g:%a' "$entry")" = 10001:10001:700 ] || fail 'retained attempt2 directory mismatch'
    done
    ready2=$prior2/operator/SLUG-POOL-OPERATOR-READY-ISSUE1.json
    [ -f "$ready2" ] && [ ! -L "$ready2" ] &&
        [ "$(stat -c '%u:%g:%a:%h' "$ready2")" = 10001:10001:600:1 ] &&
        [ "$(stat -c %s "$ready2")" -gt 0 ] || fail 'retained attempt2 READY mismatch'
    [ ! -e "$prior2/lifecycle/active" ] && [ ! -L "$prior2/lifecycle/active" ] || fail 'attempt2 lifecycle owner or failed lock remains'
    prior3=/data/hosted-test-3
    prior3_selection=/data/hosted-selection-3
    for entry in "$prior3" "$prior3/private" "$prior3/operator" "$prior3_selection" "$prior3/lifecycle" "$prior3/lifecycle/01" "$prior3/lifecycle/active"; do
        [ -d "$entry" ] && [ ! -L "$entry" ] &&
            [ "$(stat -c '%u:%g:%a' "$entry")" = 10001:10001:700 ] || fail 'retained attempt3 directory mismatch'
    done
    ready3=$prior3/operator/SLUG-POOL-OPERATOR-READY-ISSUE1.json
    failed3=$prior3/lifecycle/01/result.json
    active3=$prior3/lifecycle/active/generation
    for entry in "$ready3" "$failed3" "$active3"; do
        [ -f "$entry" ] && [ ! -L "$entry" ] &&
            [ "$(stat -c '%u:%g:%a:%h' "$entry")" = 10001:10001:600:1 ] &&
            [ "$(stat -c %s "$entry")" -gt 0 ] || fail 'retained attempt3 marker mismatch'
    done
    [ "$(stat -c %s "$active3")" = 3 ] && [ "$(cat "$active3")" = 01 ] || fail 'retained failed generation mismatch'
    [ ! -e "$prior3/lifecycle/01/owner.complete" ] && [ ! -L "$prior3/lifecycle/01/owner.complete" ] || fail 'unexpected attempt3 completion'
    # Exact already-captured PUBLIC result: generation01, premature_exit,
    # child_exit1, monitor_exit0, both waited/joined, cleanfalse. Keep its
    # failed active marker in place; never relabel it a successful owner.
    [ "$(stat -c %s "$failed3")" = 414 ] &&
        [ "$(sha256sum "$failed3" | cut -d ' ' -f 1)" = "7c6ffd9ed9c3f4c52b6abd837d5cd6eda53dc44b747fc169c0b4b63fc0b2830b" ] || fail 'retained public failure result changed'
    # Read process names only, never credentials, argv, environment or DBs.
    # Root serializes this one-shot with all product commands; this census is
    # an additional fail-closed check, not a general process authority service.
    no_product_owners() {
        for proc in /proc/[0-9]*; do
            [ -d "$proc" ] || continue
            if name=$(cat "$proc/comm" 2>/dev/null); then
                case "$name" in
                    slugpoold|slugpool-provis*|slug-host-fixt*|slug-host-audit)
                        fail 'live product/preparer/auditor process remains' ;;
                esac
            else
                [ ! -d "$proc" ] || fail 'live process metadata unreadable'
            fi
        done
    }
    no_product_owners
    stamp() { stat -c '%d:%i:%u:%g:%a:%s:%Y:%Z' "$1"; }
    prior_stamp=$(stamp "$prior")
    selection_stamp=$(stamp "$prior_selection")
    operator_stamp=$(stamp "$prior/operator")
    intent_stamp=$(stamp "$intent")
    prior2_stamp=$(stamp "$prior2")
    prior2_private_stamp=$(stamp "$prior2/private")
    prior2_operator_stamp=$(stamp "$prior2/operator")
    prior2_selection_stamp=$(stamp "$prior2_selection")
    prior2_ready_stamp=$(stamp "$ready2")
    prior3_stamp=$(stamp "$prior3")
    prior3_private_stamp=$(stamp "$prior3/private")
    prior3_operator_stamp=$(stamp "$prior3/operator")
    prior3_selection_stamp=$(stamp "$prior3_selection")
    prior3_lifecycle_stamp=$(stamp "$prior3/lifecycle")
    prior3_generation_stamp=$(stamp "$prior3/lifecycle/01")
    prior3_active_stamp=$(stamp "$prior3/lifecycle/active")
    prior3_ready_stamp=$(stamp "$ready3")
    prior3_failed_stamp=$(stamp "$failed3")
    prior3_active_generation_stamp=$(stamp "$active3")
    mount_stamp=$(stat -c '%d:%i:%u:%g:%a' /data)
    a=/data/hosted-test-4
    b=/data/hosted-selection-4
    fresh "$a"
    fresh "$b"
    # Only these two absent adjacent paths are created. No mount chmod,
    # recursive chown, old-root reset, intent rewrite or automatic retry.
    mkdir -m 0700 "$a" "$b"
    chown 10001:10001 "$a" "$b"
    [ "$(stat -c '%u:%g:%a' "$a")" = 10001:10001:700 ] || fail 'new fixture directory ownership'
    [ "$(stat -c '%u:%g:%a' "$b")" = 10001:10001:700 ] || fail 'new selection directory ownership'
    [ "$(stamp "$prior")" = "$prior_stamp" ] &&
        [ "$(stamp "$prior_selection")" = "$selection_stamp" ] &&
        [ "$(stamp "$prior/operator")" = "$operator_stamp" ] &&
        [ "$(stamp "$intent")" = "$intent_stamp" ] &&
        [ "$(stamp "$prior2")" = "$prior2_stamp" ] &&
        [ "$(stamp "$prior2/private")" = "$prior2_private_stamp" ] &&
        [ "$(stamp "$prior2/operator")" = "$prior2_operator_stamp" ] &&
        [ "$(stamp "$prior2_selection")" = "$prior2_selection_stamp" ] &&
        [ "$(stamp "$ready2")" = "$prior2_ready_stamp" ] &&
        [ "$(stamp "$prior3")" = "$prior3_stamp" ] &&
        [ "$(stamp "$prior3/private")" = "$prior3_private_stamp" ] &&
        [ "$(stamp "$prior3/operator")" = "$prior3_operator_stamp" ] &&
        [ "$(stamp "$prior3_selection")" = "$prior3_selection_stamp" ] &&
        [ "$(stamp "$prior3/lifecycle")" = "$prior3_lifecycle_stamp" ] &&
        [ "$(stamp "$prior3/lifecycle/01")" = "$prior3_generation_stamp" ] &&
        [ "$(stamp "$prior3/lifecycle/active")" = "$prior3_active_stamp" ] &&
        [ "$(stamp "$ready3")" = "$prior3_ready_stamp" ] &&
        [ "$(stamp "$failed3")" = "$prior3_failed_stamp" ] &&
        [ "$(stamp "$active3")" = "$prior3_active_generation_stamp" ] &&
        [ "$(stat -c '%d:%i:%u:%g:%a' /data)" = "$mount_stamp" ] || fail 'retained metadata changed'
    no_product_owners
    printf '%s\n' '{"version":1,"action":"prepare-dirs","attempt":4,"new_roots_owned_10001":true,"prior_attempts_directory_and_marker_metadata_unchanged":true,"product_process_census_empty":true}'
    ;;
idle)
    uid10001
    # PID1 must handle TERM explicitly, even during the owner-free window.
    sleep 1800 &
    idle_child=$!
    stop_idle() {
        trap - TERM INT
        kill -TERM "$idle_child" 2>/dev/null || :
        wait "$idle_child" 2>/dev/null || :
        exit 0
    }
    trap stop_idle TERM INT
    wait "$idle_child"
    ;;
prepare)
    uid10001
    [ -f /data/hosted-selection-4/selection.json ] &&
        [ ! -L /data/hosted-selection-4/selection.json ] || fail 'selected public input required'
    [ -f /data/hosted-selection-4/node-rpc.json ] &&
        [ ! -L /data/hosted-selection-4/node-rpc.json ] || fail 'selected protected RPC input required'
    exec /usr/local/bin/slug-host-fixture prepare \
        --root /data/hosted-test-4 \
        --selection-file /data/hosted-selection-4/selection.json \
        --reference-dir /usr/share/slug/fixture \
        --node-rpc-file /data/hosted-selection-4/node-rpc.json
    ;;
*) fail 'unsupported action' ;;
esac
