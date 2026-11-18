#!/bin/sh
# Copyright (c) 2026 The SLUG developers. ISC license.
# Temporary candidate engineering profile. Qualification is separate evidence.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
profile="$root/build-profile.json"
expected='{
  "version": 1,
  "id": "slug-go1274-no-cse-v1",
  "go_version": "go1.27.4",
  "gcflags": "all=-d=ssa/generic_cse/off,ssa/lowered_cse/off",
  "scope": "candidate_engineering"
}'
fail() { printf '%s\n' "$*" >&2; exit 1; }
[ -f "$profile" ] && [ ! -L "$profile" ] || fail 'Missing regular build-profile.json.'
printf '%s\n' "$expected" | cmp -s - "$profile" || fail 'Unexpected candidate build profile.'

# Ignore persisted go-env settings and inherited compiler flags. Explicit cache,
# module, target and CGO environment remain the caller's recorded build inputs.
export GOENV=off GOTOOLCHAIN=local GOFLAGS= GOEXPERIMENT=
[ "$(go env GOVERSION)" = go1.27.4 ] || fail 'Candidate profile requires local Go 1.27.4.'
gcflags='all=-d=ssa/generic_cse/off,ssa/lowered_cse/off'
mode=${1:-}
[ "$#" -gt 0 ] && shift
case "$mode" in
  check) [ "$#" -eq 0 ] || fail 'check takes no arguments.' ;;
  profile) [ "$#" -eq 0 ] || fail 'profile takes no arguments.'; cat "$profile" ;;
  build|test|install)
    for arg do
      # Go accepts either one or two leading dashes for named flags.
      case "$arg" in -*) ;; *) continue ;; esac
      option=${arg#-}
      option=${option#-}
      case "$option" in
        gcflags|gcflags=*|asmflags|asmflags=*|gccgoflags|gccgoflags=*|toolexec|toolexec=*|compiler|compiler=*|overlay|overlay=*|pgo|pgo=*)
          fail 'Compiler overrides are not accepted by the candidate profile.' ;;
      esac
    done
    exec go "$mode" "-gcflags=$gcflags" "$@"
    ;;
  verify-binary)
    [ "$#" -gt 0 ] || fail 'verify-binary requires artifact paths.'
    for binary do
      [ -f "$binary" ] && [ ! -L "$binary" ] || fail 'Expected a regular binary artifact.'
      metadata=$(go version -m "$binary")
      printf '%s\n' "$metadata" | awk -F '\t' -v wanted="$gcflags" '
        NR == 1 { n = split($0, words, " "); version = words[n] == "go1.27.4" }
        $2 == "build" {
          if ($3 ~ /^-gcflags=/) { flags++; if ($3 != "-gcflags=" wanted) bad = 1 }
          if ($3 ~ /^-compiler=/) { compiler++; if ($3 != "-compiler=gc") bad = 1 }
          if ($3 ~ /^(-asmflags=|-gccgoflags=|-toolexec=|-pgo=|GOEXPERIMENT=)/) bad = 1
        }
        END { exit !(version && flags == 1 && compiler == 1 && !bad) }
      ' || fail 'Binary compiler settings do not match the candidate profile.'
      printf '%s\n' "$metadata"
    done
    ;;
  *) fail 'Usage: candidate-go.sh check|profile|verify-binary FILE...|build|test|install [Go arguments]' ;;
esac
