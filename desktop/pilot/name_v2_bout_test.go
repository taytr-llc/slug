// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/prospectservice"
)

func nameV2ActualBout(t *testing.T) (gymbout.Record, gymbout.Pin) {
	t.Helper()
	read := func(name, want string, out any) {
		b, err := os.ReadFile("testdata/name-v2-bout/" + name)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != want || json.Unmarshal(b, out) != nil {
			t.Fatal("preserved actual input changed", name)
		}
	}
	var r gymbout.Record
	var p gymbout.Pin
	read("record.json", "849c5566b32135a5d89c5defcd36fc429ac5e624d6afb7640de6417d24692996", &r)
	read("pin.json", "e25b7ed79e1d0e0fd9b10976b65afa25ecd9f09219f3cb320527f8062261e878", &p)
	return r, p
}

// Build transport-only observation metadata around the unmodified saved record.
// This is not a new source/finality assertion or an expected engine result.
func nameV2ActualBoutJob(t *testing.T, r gymbout.Record, p gymbout.Pin) BoutJob {
	t.Helper()
	src := r.Source
	state := gymcard.BoutState{Version: 1, CardID: r.CardID, Event: src.Event, CardExists: true,
		CardRecordDigest: src.CardRecordDigest, CardSourceDigest: src.CardSourceDigest,
		CardCreatedAtMS: src.CardCreatedAtMS, FinalizedAtMS: src.FinalizedAtMS,
		NodeState: "unobserved", ObservedAtMS: r.CreatedAtMS, Usable: true}
	saved := gymbout.SavedBout{Bout: src.Bout, SourceDigest: r.SourceDigest, Exists: true,
		RecordDigest: boutRecordHash(r), CreatedAtMS: r.CreatedAtMS, InputID: r.InputID,
		ResultDigest: r.ResultDigest, Winner: r.Result.Winner, Method: r.Result.Method,
		EndRound: r.Result.EndRound, EndTick: r.Result.EndTick, Usable: true}
	st := gymbout.Status{Version: 1, StoreID: r.StoreID, CardID: r.CardID, ObservedAtMS: r.CreatedAtMS, Source: state, Saved: saved}
	sp := p.SourcePin.SourcePin.SourcePin.Participants[0].StablePin
	v := ManifestViewPin{Version: 1, ViewID: strings.Repeat("a", 64), AccountAuthorityID: sp.AccountAuthorityID,
		AccountID: sp.AccountID, Generation: 1, AuthorityPin: sp.Source, AuthorityOrigin: "http://127.0.0.1:19065"}
	var err error
	v.TransportTrustID, err = prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: v.AuthorityOrigin, Token: strings.Repeat("0", 64), AuthorityPin: v.AuthorityPin})
	if err != nil {
		t.Fatal(err)
	}
	return BoutJob{Version: 1, JobID: strings.Repeat("b", 64), Kind: "run", State: "completed", View: v,
		StoreID: r.StoreID, CardID: r.CardID, Saturday: src.Event.Saturday, BoutID: r.BoutID,
		Result: &gymbout.RunResult{Version: 1, Record: r, Status: st}}
}

func TestNameV2SavedBoutCompletedJob(t *testing.T) {
	r, p := nameV2ActualBout(t)
	// Validate the entire unchanged record under its retained exact source pin;
	// this pure check does not open stores, hash headers, or mint authority.
	if _, err := gymbout.ValidateRecord(context.Background(), p, r); err != nil {
		t.Fatal("actual saved record", err)
	}
	if !boutRecordShape(r) {
		t.Fatal("actual v2 record rejected by transport")
	}
	j := nameV2ActualBoutJob(t, r, p)
	if !boutJobValid(&j) {
		t.Fatal("completed v2 job rejected after durable commit")
	}
	response := BoutResponse{ID: 1, OK: true, View: &j.View, StoreID: j.StoreID, Job: &j}
	raw, err := MarshalBoutResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseBoutResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip, err := MarshalBoutResponse(parsed)
	if err != nil || !bytes.Equal(raw, roundtrip) {
		t.Fatal("complete job roundtrip", err)
	}
	for name, mutate := range map[string]func(*gymbout.Record){
		"v1_digest_under_v2_source": func(x *gymbout.Record) {
			b, _ := json.Marshal(x.Input.A.Manifest)
			x.Source.Corners[0].Member.Assignment.ManifestDigest = previewHash("SLUG fighter manifest digest\x00v1\x00", b, false)
		},
		"v1_descriptor_with_v2_manifest": func(x *gymbout.Record) { x.Source.Corners[0].Member.Assignment.DescriptorID = manifestDescriptorID },
		"unknown_descriptor":             func(x *gymbout.Record) { x.Source.Corners[0].Member.Assignment.DescriptorID = strings.Repeat("f", 64) },
		"mixed_manifest_version":         func(x *gymbout.Record) { x.Input.A.Manifest.Version = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			x, pin := nameV2ActualBout(t)
			mutate(&x)
			// Repair only the enclosing transport correlations so refusal tests
			// the mismatched content rather than a stale outer checksum.
			x.SourceDigest = boutSourceHash(x.Source)
			in, _ := json.Marshal(x.Input)
			x.InputID = previewHash("SLUG fighter bout input\x00v1\x00", in, false)
			x.Result.InputID = x.InputID
			result, _ := json.Marshal(x.Result)
			x.ResultDigest = previewHash("SLUG fighter bout result\x00v1\x00", result, false)
			bad := nameV2ActualBoutJob(t, x, pin)
			if boutRecordShape(x) || boutJobValid(&bad) {
				t.Fatal("mixed descriptor/digest accepted")
			}
		})
	}
}
