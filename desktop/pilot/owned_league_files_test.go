// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOwnedGymLeagueCompleteIntentFraming(t *testing.T) {
	r := ownedSourceRuntimeTest(t)
	config, e := OwnedLeagueConfigFor(r.owned.config, r.source, r.marker.AccountAuthorityID, strings.Repeat("1", 64), strings.Repeat("2", 64))
	if e != nil {
		t.Fatal("complete valid config", e)
	}
	intent := ownedLeagueIntent{"slug-owned-league", 1, config}
	raw, e := json.Marshal(intent)
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("complete valid owned intent: %d canonical JSON bytes, %d file bytes", len(raw), len(raw)+1)
	if len(raw)+1 <= poolRuntimeMarkerLimit || len(raw)+1 > ownedLeagueMarkerLimit {
		t.Fatal("fixture does not exercise the distinct frozen marker bounds", len(raw)+1)
	}
	path := filepath.Join(r.config.DataDir, ownedLeagueIntentName)
	if e = ownedLeagueWrite(path, intent); e != nil {
		t.Fatal("full owned intent refused", e)
	}
	var got ownedLeagueIntent
	if e = ownedLeagueRead(path, &got); e != nil || !reflect.DeepEqual(got, intent) {
		t.Fatal("full owned intent roundtrip", e)
	}
	if e = ownedLeagueWrite(path, intent); e != nil {
		t.Fatal("matching-existing lost-ack retry used legacy cap", e)
	}
	changed, e := OwnedLeagueConfigFor(r.owned.config, r.source, r.marker.AccountAuthorityID, strings.Repeat("1", 64), strings.Repeat("3", 64))
	if e != nil {
		t.Fatal(e)
	}
	if e = ownedLeagueWrite(path, ownedLeagueIntent{"slug-owned-league", 1, changed}); e == nil {
		t.Fatal("valid but different original pin replaced established intent")
	}
	stored, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(stored, append(raw, '\n')) {
		t.Fatal("refused replacement changed established bytes", e)
	}
	legacy := filepath.Join(r.config.DataDir, "legacy-intent.json")
	if e = writePoolRuntimeRecord(legacy, intent); e == nil {
		t.Fatal("legacy writer widened past its32KiB grammar")
	}
	if _, e = os.Lstat(legacy); !os.IsNotExist(e) {
		t.Fatal("legacy refusal created file", e)
	}
	oversize := filepath.Join(r.config.DataDir, "oversize.json")
	if e = ownedLeagueWrite(oversize, strings.Repeat("x", ownedLeagueMarkerLimit)); e == nil {
		t.Fatal("owned writer exceeded64KiB including framing")
	}
	if _, e = os.Lstat(oversize); !os.IsNotExist(e) {
		t.Fatal("oversize refusal created file", e)
	}
	if e = writePoolRuntimeRecordLimit(oversize, "small", 128<<10); e == nil {
		t.Fatal("private writer accepted an unapproved arbitrary cap")
	}
}
