#!/bin/sh
# One disposable fixture operator, not a production service supervisor.
set -eu
umask 077
PATH=/usr/local/bin:/usr/bin:/bin
export PATH
BASE=/data/hosted-test-5
EVIDENCE=$BASE/set83-ui5-lifecycle
SELF=/usr/local/bin/host-paid-ui5-lifecycle-83
BIN=/usr/local/bin/slugpoold
SETUP=$BASE/private/selected-setup.json
CERT=$BASE/set83-ui-private/tls-cert.pem
PUBLIC_SELECTION=$BASE/set83-ui-public/selection.json

fail() { printf '%s\n' "operator lifecycle: $1" >&2; exit 1; }
uid() {
    [ "$(id -u):$(id -ru):$(id -g):$(id -rg)" = 10001:10001:10001:10001 ] || fail 'UID/GID10001 required'
}
directory() {
    [ -d "$1" ] && [ ! -L "$1" ] &&
        [ "$(stat -c '%u:%g:%a' "$1")" = 10001:10001:700 ] || fail 'protected directory mismatch'
}
fresh() { [ ! -e "$1" ] && [ ! -L "$1" ] || fail 'evidence path already exists'; }
hash() { sha256sum "$1" | cut -d ' ' -f 1; }
hex() { [ "${#1}" = 64 ] && case "$1" in *[!0-9a-f]*) false;; *) true;; esac; }
number() { case "$1" in ''|*[!0-9]*) return 1;; *) [ "${#1}" -le 10 ];; esac; }
proc() {
    # Public process identity only; never read environ or full cmdline.
    p=$1
    number "$p" && [ "$p" -gt 1 ] || return 1
    [ "$(awk '/^Uid:/ { print $2 ":" $3 ":" $4 ":" $5 }' "/proc/$p/status")" = 10001:10001:10001:10001 ] || return 1
    awk '{ sub(/^.*\) /, ""); split($0,a," "); print a[2] ":" a[3] ":" a[4] ":" a[20] }' "/proc/$p/stat"
}
same() {
    p=$1 want=$2 executable=$3
    [ "$(proc "$p" 2>/dev/null)" = "$want" ] &&
        [ "$(readlink "/proc/$p/exe" 2>/dev/null)" = "$executable" ]
}
public_selection() {
    directory "$BASE/set83-ui-public"
    directory "$BASE/set83-ui-private"
    for file in "$PUBLIC_SELECTION" "$CERT"; do
        [ -f "$file" ] && [ ! -L "$file" ] &&
            [ "$(stat -c '%u:%g:%a:%h' "$file")" = 10001:10001:600:1 ] || fail 'renewed public selection file mismatch'
        [ "$(stat -c '%s' "$file")" -gt 0 ] && [ "$(stat -c '%s' "$file")" -le 16384 ] || fail 'renewed public input bound'
    done
    [ "$(hash "$PUBLIC_SELECTION")" = "$selection_sha" ] || fail 'public execution selection changed'
    # No jq, source, eval or general JSON parsing. The public record has ten
    # fixed ordered fields; the grammar admits only decimal times/hex pins.
    [ "$(wc -l < "$PUBLIC_SELECTION" | tr -d ' \t')" = 1 ] || fail 'one public selection line required'
    line=$(cat "$PUBLIC_SELECTION")
    printf '%s\n' "$line" | grep -Eq '^\{"version":1,"selection_id":"set83-paid-ui-1","selected_at_ms":[1-9][0-9]{0,14},"expires_at_ms":[1-9][0-9]{0,14},"setup_sha256":"[0-9a-f]{64}","operator_sha256":"[0-9a-f]{64}","original_leaf_spki":"[0-9a-f]{64}","renewed_certificate_sha256":"[0-9a-f]{64}","new_ca_sha256":"[0-9a-f]{64}","remote_config_id":"[0-9a-f]{64}"\}$' || fail 'public execution selection grammar'
    [ "$(printf '%s\n' "$line" | sha256sum | cut -d ' ' -f 1)" = "$selection_sha" ] || fail 'one canonical selection line required'
    selected_ms=$(printf '%s' "$line" | sed -n 's/.*"selected_at_ms":\([0-9]*\),.*/\1/p')
    expires_ms=$(printf '%s' "$line" | sed -n 's/.*"expires_at_ms":\([0-9]*\),.*/\1/p')
    selected_setup=$(printf '%s' "$line" | sed -n 's/.*"setup_sha256":"\([0-9a-f]*\)".*/\1/p')
    selected_binary=$(printf '%s' "$line" | sed -n 's/.*"operator_sha256":"\([0-9a-f]*\)".*/\1/p')
    selected_spki=$(printf '%s' "$line" | sed -n 's/.*"original_leaf_spki":"\([0-9a-f]*\)".*/\1/p')
    certificate_sha=$(printf '%s' "$line" | sed -n 's/.*"renewed_certificate_sha256":"\([0-9a-f]*\)".*/\1/p')
    [ "$selected_ms" -le 253402293599999 ] && [ "$((selected_ms % 1000))" = 0 ] &&
        [ "$expires_ms" = "$((selected_ms + 7200000))" ] || fail 'fixed selected two-hour credential interval required'
    [ "$selected_setup" = "$setup_sha" ] && [ "$selected_binary" = "$binary_sha" ] &&
        [ "$selected_spki" = 2cca4de52f06009cff05c22ea607fd212aff0aaf9c7eb6c238a25956fa62c48f ] &&
        [ "$(hash "$CERT")" = "$certificate_sha" ] || fail 'original key selection or renewed certificate mismatch'
}
selection() {
    [ -f "$SETUP" ] && [ ! -L "$SETUP" ] &&
        [ "$(hash "$SETUP")" = "$setup_sha" ] &&
        [ "$(hash "$BIN")" = "$binary_sha" ] || fail 'selected setup/binary mismatch'
    public_selection
}

# This is a NEW bounded read acceptance over the already paid original root.
# Completed Set82/native83 evidence and old deadlines are never reused.
# A fresh public certificate keeps the original leaf key/SPKI and original setup.
prior_paid_owner() {
    selection
    [ "$(( $(date +%s) * 1000 ))" -ge "$selected_ms" ] &&
        [ "$((deadline * 1000))" -lt "$expires_ms" ] || fail 'outside selected credential interval'
    [ "$setup_sha" = 87db5803e70418affbf492b2716e2104d6902ec9294940141fe97a885a5c3525 ] &&
        [ "$binary_sha" = 86a3bfa7cb07951e83eaca6df346a4be8bebe72df30d779705c83f7f8ac3a85f ] || fail 'original paid selection required'
    directory "$BASE/operator"
    directory "$BASE/lifecycle"
    [ ! -e "$BASE/lifecycle/active" ] && [ ! -L "$BASE/lifecycle/active" ] || fail 'old operator owner or failed lock retained'
    for prior in 01 02 03; do
        directory "$BASE/lifecycle/$prior"
        for name in owner.complete result.json; do
            [ -f "$BASE/lifecycle/$prior/$name" ] && [ ! -L "$BASE/lifecycle/$prior/$name" ] || fail 'old joined generation absent'
        done
        grep -q '"clean":true' "$BASE/lifecycle/$prior/result.json" || fail 'old generation did not join'
    done
    [ "$(hash "$BASE/lifecycle/03/result.json")" = 8daa8957a71aa4bd7d85fa5772c93945efbd5b7267661074309b0e433aa6c96c ] || fail 'original final joined result changed'
    directory "$BASE/set83-native-lifecycle"
    directory "$BASE/set83-native-lifecycle/01"
    [ ! -e "$BASE/set83-native-lifecycle/active" ] && [ ! -L "$BASE/set83-native-lifecycle/active" ] || fail 'native83 owner or failed lock remains'
    [ -f "$BASE/set83-native-lifecycle/01/owner.complete" ] && [ ! -L "$BASE/set83-native-lifecycle/01/owner.complete" ] &&
        [ -f "$BASE/set83-native-lifecycle/01/result.json" ] && [ ! -L "$BASE/set83-native-lifecycle/01/result.json" ] &&
        [ "$(hash "$BASE/set83-native-lifecycle/01/result.json")" = cec64486a7388c5ee773a7c1364bf2c7c8f8143cb7271e27501875378737c771 ] || fail 'original native83 clean join changed'
    directory "$BASE/set83-ui-lifecycle"
    directory "$BASE/set83-ui-lifecycle/01"
    [ ! -e "$BASE/set83-ui-lifecycle/active" ] && [ ! -L "$BASE/set83-ui-lifecycle/active" ] || fail 'prior UI owner or failed lock remains'
    for file in owner.complete result.json selection.sha256 certificate.sha256; do
        [ -f "$BASE/set83-ui-lifecycle/01/$file" ] && [ ! -L "$BASE/set83-ui-lifecycle/01/$file" ] &&
            [ "$(stat -c '%u:%g:%a:%h' "$BASE/set83-ui-lifecycle/01/$file")" = 10001:10001:600:1 ] || fail 'prior UI clean owner evidence missing'
    done
    [ "$(hash "$BASE/set83-ui-lifecycle/01/result.json")" = 4c3dd73f0f55e5ed7f8475cd8cabcbd80ff4e8fd42b7b67f443a3401adf67de0 ] &&
        [ "$(stat -c '%s' "$BASE/set83-ui-lifecycle/01/result.json")" = 408 ] || fail 'exact original UI clean result changed'
    [ "$(cat "$BASE/set83-ui-lifecycle/01/selection.sha256")" = "$selection_sha" ] &&
        [ "$(cat "$BASE/set83-ui-lifecycle/01/certificate.sha256")" = "$certificate_sha" ] || fail 'original prepared UI selection changed'
    # Prior UI3 reached its deadline. The original child and monitor joined,
    # but clean=false and its active marker remain evidence of that failure.
    directory "$BASE/set83-ui3-lifecycle"
    directory "$BASE/set83-ui3-lifecycle/01"
    directory "$BASE/set83-ui3-lifecycle/active"
    [ ! -e "$BASE/set83-ui3-lifecycle/01/owner.complete" ] && [ ! -L "$BASE/set83-ui3-lifecycle/01/owner.complete" ] || fail 'prior deadline result must not be relabeled clean'
    for file in result.json selection.sha256 certificate.sha256 child.waited term.reason owner.pid owner.identity owner.exe child.pid child.identity; do
        [ -f "$BASE/set83-ui3-lifecycle/01/$file" ] && [ ! -L "$BASE/set83-ui3-lifecycle/01/$file" ] &&
            [ "$(stat -c '%u:%g:%a:%h' "$BASE/set83-ui3-lifecycle/01/$file")" = 10001:10001:600:1 ] || fail 'prior UI3 joined deadline evidence missing'
    done
    [ -f "$BASE/set83-ui3-lifecycle/active/generation" ] && [ ! -L "$BASE/set83-ui3-lifecycle/active/generation" ] &&
        [ "$(stat -c '%u:%g:%a:%h' "$BASE/set83-ui3-lifecycle/active/generation")" = 10001:10001:600:1 ] &&
        [ "$(cat "$BASE/set83-ui3-lifecycle/active/generation")" = 01 ] || fail 'prior UI3 failed active marker changed'
    [ "$(hash "$BASE/set83-ui3-lifecycle/01/result.json")" = d2acb5869e704ee5d6cf898cd052be790d15be4007d5896f06269748bdf43ad7 ] &&
        [ "$(stat -c '%s' "$BASE/set83-ui3-lifecycle/01/result.json")" = 408 ] &&
        [ "$(cat "$BASE/set83-ui3-lifecycle/01/child.waited")" = 0 ] &&
        [ "$(cat "$BASE/set83-ui3-lifecycle/01/term.reason")" = deadline ] || fail 'exact prior UI3 joined deadline result changed'
    [ "$(cat "$BASE/set83-ui3-lifecycle/01/selection.sha256")" = "$selection_sha" ] &&
        [ "$(cat "$BASE/set83-ui3-lifecycle/01/certificate.sha256")" = "$certificate_sha" ] || fail 'prepared selection or certificate renewed after UI3'
    [ "$(cat "$BASE/set83-ui3-lifecycle/01/owner.pid")" = 102 ] &&
        [ "$(cat "$BASE/set83-ui3-lifecycle/01/child.pid")" = 248 ] || fail 'original UI3 process identity changed'
    if same 102 "$(cat "$BASE/set83-ui3-lifecycle/01/owner.identity")" "$(cat "$BASE/set83-ui3-lifecycle/01/owner.exe")"; then fail 'original UI3 owner still live'; fi
    if same 248 "$(cat "$BASE/set83-ui3-lifecycle/01/child.identity")" "$BIN"; then fail 'original UI3 child still live'; fi
    directory "$BASE/set83-ui4-lifecycle"
    directory "$BASE/set83-ui4-lifecycle/01"
    [ ! -e "$BASE/set83-ui4-lifecycle/active" ] && [ ! -L "$BASE/set83-ui4-lifecycle/active" ] || fail 'prior UI4 owner or failed lock remains'
    for file in owner.complete result.json selection.sha256 certificate.sha256; do
        [ -f "$BASE/set83-ui4-lifecycle/01/$file" ] && [ ! -L "$BASE/set83-ui4-lifecycle/01/$file" ] &&
            [ "$(stat -c '%u:%g:%a:%h' "$BASE/set83-ui4-lifecycle/01/$file")" = 10001:10001:600:1 ] || fail 'prior UI4 clean owner evidence missing'
    done
    [ "$(hash "$BASE/set83-ui4-lifecycle/01/result.json")" = 26559da2bc6ea5335165be990231b89906a5237613ea70017bdf3e7370dc1692 ] &&
        [ "$(stat -c '%s' "$BASE/set83-ui4-lifecycle/01/result.json")" = 410 ] || fail 'exact original UI4 clean result changed'
    [ "$(cat "$BASE/set83-ui4-lifecycle/01/selection.sha256")" = "$selection_sha" ] &&
        [ "$(cat "$BASE/set83-ui4-lifecycle/01/certificate.sha256")" = "$certificate_sha" ] || fail 'prepared selection or certificate renewed after UI4'

}

uid
[ "$#" -ge 2 ] || fail 'run|stop|status plus generation required'
action=$1 gen=$2
case "$gen" in 01|02|03) ;; *) fail 'generation must be 01,02,03';; esac
directory "$BASE"
directory "$BASE/private"
[ -d /data ] && [ ! -L /data ] && [ "$(stat -c '%u:%a' /data)" = 0:755 ] || fail 'selected mount mismatch'
g=$EVIDENCE/$gen

case "$action" in
run)
    [ "$#" = 6 ] || fail 'run GEN ABSOLUTE_UNIX_DEADLINE SETUP_SHA256 BINARY_SHA256 SELECTION_SHA256'
    number "$3" && hex "$4" && hex "$5" && hex "$6" || fail 'invalid run selection'
    now=$(date +%s)
    [ "$3" -gt "$now" ] && [ "$3" -le "$((now + 300))" ] || fail 'absolute deadline outside 300s bound'
    deadline=$3 setup_sha=$4 binary_sha=$5 selection_sha=$6
    prior_paid_owner
    # Preserve the launch boundary before setsid. All paths are public
    # evidence inside the fixed protected fixture tree, never private inputs.
    launch_root=$BASE/set83-ui5-lifecycle-launch
    if [ ! -e "$launch_root" ] && [ ! -L "$launch_root" ]; then mkdir -m 0700 "$launch_root"; fi
    directory "$launch_root"
    launch=$launch_root/$gen
    fresh "$launch"
    mkdir -m 0700 "$launch"
    printf '{"version":1,"generation":"%s","parent_pid":%s,"started_at":%s,"deadline":%s,"setup_sha256":"%s","binary_sha256":"%s"}\n' \
        "$gen" "$$" "$now" "$3" "$4" "$5" > "$launch/start.json"
    # With job control disabled, this background child inherits our PGID but
    # has its own PID. BusyBox setsid therefore execs instead of forking away
    # from SSH. The foreground shell retains its exact original wait handle.
    set +m
    setsid "$SELF" own "$gen" "$3" "$4" "$5" "$6" \
        </dev/null >"$launch/stdout.log" 2>"$launch/stderr.log" &
    launcher=$!
    printf '%s\n' "$launcher" > "$launch/launcher.pid"
    set +e
    wait "$launcher"
    launch_rc=$?
    set -e
    owner_joined=false
    if [ "$launch_rc" = 0 ] && [ -f "$g/owner.pid" ] &&
        [ "$(cat "$g/owner.pid")" = "$launcher" ] &&
        [ -f "$g/owner.complete" ] && [ -f "$g/result.json" ] &&
        grep -q '"clean":true' "$g/result.json"; then owner_joined=true; fi
    printf '{"version":1,"generation":"%s","launcher_pid":%s,"finished_at":%s,"wait_exit":%s,"owner_joined":%s}\n' \
        "$gen" "$launcher" "$(date +%s)" "$launch_rc" "$owner_joined" > "$launch/wait.json"
    sync
    [ "$owner_joined" = true ] || fail 'launch did not yield matching joined original owner; inspect launch evidence'
    exit 0
    ;;
own)
    [ "$#" = 6 ] || fail 'invalid owner arguments'
    deadline=$3 setup_sha=$4 binary_sha=$5 selection_sha=$6
    number "$deadline" && hex "$setup_sha" && hex "$binary_sha" && hex "$selection_sha" || fail 'invalid owner selection'
    now=$(date +%s)
    [ "$deadline" -gt "$now" ] && [ "$deadline" -le "$((now + 300))" ] || fail 'owner deadline expired'
    prior_paid_owner
    owner=$$
    identity=$(proc "$owner") || fail 'owner process unavailable'
    owner_exe=$(readlink "/proc/$owner/exe")
    [ "$owner_exe" = /bin/busybox ] || fail 'expected BusyBox shell owner'
    [ "$(printf '%s' "$identity" | cut -d : -f 2-3)" = "$owner:$owner" ] || fail 'dedicated session/group required'
    selection
    if [ ! -e "$EVIDENCE" ] && [ ! -L "$EVIDENCE" ]; then mkdir -m 0700 "$EVIDENCE"; fi
    directory "$EVIDENCE"
    if [ "$gen" = 01 ]; then
        fresh "$EVIDENCE/deadline"
        (set -C; printf '%s\n' "$deadline" > "$EVIDENCE/deadline")
    else
        [ -f "$EVIDENCE/deadline" ] && [ ! -L "$EVIDENCE/deadline" ] &&
            [ "$(cat "$EVIDENCE/deadline")" = "$deadline" ] || fail 'original shared deadline changed'
        previous=01
        [ "$gen" != 03 ] || previous=02
        [ -f "$EVIDENCE/$previous/owner.complete" ] &&
            grep -q '"clean":true' "$EVIDENCE/$previous/result.json" &&
            [ "$(cat "$EVIDENCE/$previous/setup.sha256")" = "$setup_sha" ] &&
            [ "$(cat "$EVIDENCE/$previous/binary.sha256")" = "$binary_sha" ] &&
            [ "$(cat "$EVIDENCE/$previous/selection.sha256")" = "$selection_sha" ] &&
            [ "$(cat "$EVIDENCE/$previous/certificate.sha256")" = "$certificate_sha" ] || fail 'previous same-selection generation not complete'
    fi
    # Exclusive generation and active ownership. Never remove a failed lock.
    mkdir -m 0700 "$EVIDENCE/active" || fail 'another generation or failed owner remains'
    fresh "$g"
    mkdir -m 0700 "$g"
    printf '%s\n' "$gen" > "$EVIDENCE/active/generation"
    printf '%s\n' "$owner" > "$g/owner.pid"
    printf '%s\n' "$identity" > "$g/owner.identity"
    printf '%s\n' "$owner_exe" > "$g/owner.exe"
    printf '%s\n' "$setup_sha" > "$g/setup.sha256"
    printf '%s\n' "$binary_sha" > "$g/binary.sha256"
    printf '%s\n' "$selection_sha" > "$g/selection.sha256"
    printf '%s\n' "$certificate_sha" > "$g/certificate.sha256"
    printf '%s\n' "$deadline" > "$g/deadline"
    printf '%s\n' "$now" > "$g/started_at"
    interrupted=0
    on_signal() {
        interrupted=1
        # The monitor records its intent before signaling our anchored group.
        if [ ! -f "$g/term.reason" ]; then : > "$g/external-signal"; fi
    }
    trap on_signal TERM INT HUP
    "$BIN" serve --daily-recoverable-issue \
        --setup-file "$SETUP" --root "$BASE/operator" \
        --wallet-root "$BASE/custody/wallet16" \
        --wallet-public-passphrase-file "$BASE/private/public.pass" \
        --wallet-unlock-passphrase-file "$BASE/private/unlock.pass" \
        --admission-seed-file "$BASE/private/admission.seed" \
        --publication-seed-file "$BASE/private/publication.seed" \
        --cutoff-seed-file "$BASE/private/cutoff.seed" \
        --rpc-config-file "$BASE/private/node-rpc.json" \
        --tls-certificate-file "$CERT" \
        --tls-key-file "$BASE/private/tls-key.pem" \
        </dev/null >"$g/stdout.jsonl" 2>"$g/stderr.log" &
    child=$!
    printf '%s\n' "$child" > "$g/child.pid"
    # This monitor anchors the session until the original child's actual wait.
    # A forced group kill cannot accidentally target a recycled foreign PID.
    (
        trap '' TERM INT HUP
        reason=
        while [ ! -f "$g/child.waited" ]; do
            if [ -f "$g/external-signal" ]; then reason=external_signal
            elif [ -f "$g/stop.request" ]; then reason=requested
            elif [ "$(date +%s)" -ge "$deadline" ]; then reason=deadline
            else sleep 0.1; continue
            fi
            printf '%s\n' "$reason" > "$g/term.reason"
            date +%s > "$g/term.at"
            kill -TERM "-$owner"
            end=$(( $(date +%s) + 30 ))
            while [ ! -f "$g/child.waited" ]; do
                if [ "$(date +%s)" -ge "$end" ]; then
                    printf '%s\n' 'forced_kill_no_clean_join' > "$g/forced-kill"
                    sync
                    kill -KILL "-$owner"
                    exit 1
                fi
                sleep 0.1
            done
            break
        done
    ) </dev/null >"$g/monitor.log" 2>&1 &
    monitor=$!
    # Capture the real child identity after exec, without assuming immediate
    # scheduling. A launch failure is still collected by the original wait.
    until [ "$(readlink "/proc/$child/exe" 2>/dev/null || :)" = "$BIN" ]; do
        [ -r "/proc/$child/stat" ] || break
        state=$(awk '{ sub(/^.*\) /, ""); print $1 }' "/proc/$child/stat")
        [ "$state" != Z ] || break
        [ "$(date +%s)" -lt "$deadline" ] || break
        sleep 0.1
    done
    if [ "$(readlink "/proc/$child/exe" 2>/dev/null || :)" = "$BIN" ]; then proc "$child" > "$g/child.identity"; fi
    while :; do
        interrupted=0
        set +e
        wait "$child"
        rc=$?
        set -e
        # POSIX wait interrupted by our TERM trap has not supplied the final
        # child result. Retry only that interruption, never reinterpret rc.
        if [ "$interrupted" = 1 ] && [ "$rc" -gt 128 ]; then continue; fi
        break
    done
    printf '%s\n' "$rc" > "$g/child.waited"
    set +e
    wait "$monitor"
    monitor_rc=$?
    set -e
    trap - TERM INT HUP
    reason=premature_exit
    [ ! -f "$g/term.reason" ] || reason=$(cat "$g/term.reason")
    finished=$(date +%s)
    clean=false
    if [ "$rc" = 0 ] && [ "$monitor_rc" = 0 ] && [ "$reason" = requested ] &&
        [ ! -e "$g/external-signal" ] && [ ! -e "$g/forced-kill" ]; then clean=true; fi
    # Recheck the unchanged selected public identity before admitting reopen.
    selection
    printf '{"version":1,"generation":"%s","owner_pid":%s,"child_pid":%s,"started_at":%s,"finished_at":%s,"deadline":%s,"reason":"%s","child_exit":%s,"monitor_exit":%s,"child_waited":true,"monitor_joined":true,"clean":%s,"setup_sha256":"%s","binary_sha256":"%s"}\n' \
        "$gen" "$owner" "$child" "$now" "$finished" "$deadline" "$reason" "$rc" "$monitor_rc" "$clean" "$setup_sha" "$binary_sha" > "$g/result.pending"
    sync
    mv "$g/result.pending" "$g/result.json"
    sync
    if [ "$clean" = true ]; then
        rm "$EVIDENCE/active/generation"
        rmdir "$EVIDENCE/active"
        printf '%s\n' 'child_and_monitor_joined_active_released' > "$g/owner.complete"
        sync
        exit 0
    fi
    exit 1
    ;;
stop)
    [ "$#" = 5 ] || fail 'stop GEN SETUP_SHA256 BINARY_SHA256 SELECTION_SHA256'
    setup_sha=$3 binary_sha=$4 selection_sha=$5
    hex "$setup_sha" && hex "$binary_sha" && hex "$selection_sha" || fail 'invalid stop selection'
    directory "$EVIDENCE"; directory "$g"; directory "$EVIDENCE/active"
    [ "$(cat "$EVIDENCE/active/generation")" = "$gen" ] || fail 'active generation mismatch'
    [ "$(cat "$g/setup.sha256")" = "$setup_sha" ] && [ "$(cat "$g/binary.sha256")" = "$binary_sha" ] &&
        [ "$(cat "$g/selection.sha256")" = "$selection_sha" ] || fail 'saved selection mismatch'
    selection
    owner=$(cat "$g/owner.pid") child=$(cat "$g/child.pid")
    same "$owner" "$(cat "$g/owner.identity")" "$(cat "$g/owner.exe")" || fail 'original owner changed'
    same "$child" "$(cat "$g/child.identity")" "$BIN" || fail 'original child changed'
    [ "$(cut -d : -f 1 "$g/child.identity")" = "$owner" ] || fail 'original parent mismatch'
    fresh "$g/stop.request"
    (set -C; date +%s > "$g/stop.request") || fail 'stop already requested'
    end=$(( $(date +%s) + 30 ))
    while [ ! -f "$g/result.json" ] || [ ! -f "$g/owner.complete" ]; do
        [ ! -f "$g/result.json" ] || grep -q '"clean":true' "$g/result.json" || fail 'original owner did not close cleanly'
        [ "$(date +%s)" -lt "$end" ] || fail '30s stop observation expired; no clean join claim'
        sleep 0.1
    done
    cat "$g/result.json"
    grep -q '"clean":true' "$g/result.json" || fail 'original owner did not close cleanly'
    ;;
status)
    [ "$#" = 2 ] || fail 'status GEN'
    directory "$EVIDENCE"; directory "$g"
    [ -f "$g/result.json" ] && [ ! -L "$g/result.json" ] && [ -f "$g/owner.complete" ] || fail 'no final clean join result'
    cat "$g/result.json"
    ;;
*) fail 'unsupported action';;
esac
