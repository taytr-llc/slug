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
stage)
    [ "$(id -u)" = 0 ] && [ "$(id -ru)" = 0 ] || fail 'one-time root preparation required'
    [ -d /data ] && [ ! -L /data ] || fail 'real /data volume required'
    awk '$5 == "/data" { found = 1 } END { exit !found }' /proc/self/mountinfo || fail 'attached /data mount required'
    # A previously unused volume only. The sole allowed provider entry is an
    # empty, real, root-owned lost+found directory; it is never modified.
    layout=empty
    for entry in /data/.[!.]* /data/..?* /data/*; do
        [ -e "$entry" ] || [ -L "$entry" ] || continue
        [ "$entry" = /data/lost+found ] || fail 'unexpected mount entry'
        [ -d "$entry" ] && [ ! -L "$entry" ] || fail 'lost+found is not a real directory'
        [ "$(stat -c %u "$entry")" = 0 ] || fail 'lost+found owner mismatch'
        entries=$(find "$entry" -mindepth 1 -maxdepth 1 -print -quit) || fail 'lost+found inventory failed'
        [ -z "$entries" ] || fail 'lost+found is not empty'
        layout=empty_lost_found
    done
    [ "$(stat -c %u /data)" = 0 ] || fail 'expected root-owned fresh mount'
    # Original protected inputs open every ancestor O_RDONLY|O_DIRECTORY.
    # Search-only0711 cannot satisfy that read; protected children stay0700.
    chmod 0755 /data
    [ "$(stat -c '%u:%a' /data)" = 0:755 ] || fail 'mount read/search correction failed'
    printf '{"version":1,"fresh_mount":"%s","mount_mode":"0755"}\n' "$layout"
    a=/data/hosted-test
    b=/data/hosted-selection
    fresh "$a"
    fresh "$b"
    mkdir -m 0700 "$a" "$b"
    chown 10001:10001 "$a" "$b"
    [ "$(stat -c '%u:%g:%a' "$a")" = 10001:10001:700 ] || fail 'first directory ownership'
    [ "$(stat -c '%u:%g:%a' "$b")" = 10001:10001:700 ] || fail 'second directory ownership'
    # This fixed window contains no product owner or listener. Once it ends,
    # there is no automatic restart/resume/reset action.
    exec su -s /bin/sh -c 'exec /usr/local/bin/host-bootstrap idle' slug
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
    [ -f /data/hosted-selection/selection.json ] &&
        [ ! -L /data/hosted-selection/selection.json ] || fail 'selected public input required'
    [ -f /data/hosted-selection/node-rpc.json ] &&
        [ ! -L /data/hosted-selection/node-rpc.json ] || fail 'selected protected RPC input required'
    exec /usr/local/bin/slug-host-fixture prepare \
        --root /data/hosted-test \
        --selection-file /data/hosted-selection/selection.json \
        --reference-dir /usr/share/slug/fixture \
        --node-rpc-file /data/hosted-selection/node-rpc.json
    ;;
*) fail 'unsupported action' ;;
esac
