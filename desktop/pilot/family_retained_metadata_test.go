//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/registry"
)

func TestFamilyRetainedMetadataBounds(t *testing.T) {
	// This deliberately relaxed shape is arithmetic, not an authorized Snapshot:
	// all three IDs and the longest state/error are filled simultaneously. The
	// actual closed origin grammar is ASCII loopback with a canonical five-digit
	// port at most; it cannot contain escaped arbitrary URL text.
	h := strings.Repeat("f", 64)
	v := ManifestViewPin{Version: 1, ViewID: h, AccountAuthorityID: h, AccountID: h, Generation: 9007199254740991,
		AuthorityPin:    registry.ManifestAuthorityPin{Version: 5, Network: 4294967295, Genesis: h, AuthorityID: h, DescriptorID: h},
		AuthorityOrigin: "http://127.0.0.1:65535", TransportTrustID: h}
	card := CardSnapshot{Version: 1, State: "setup_incomplete", Error: strings.Repeat("x", 128), View: &v, StoreID: h,
		SourceCreatedAtMS: 8640000000000000, CreatedAtMS: 8640000000000000, ActiveJobID: h, QueuedJobID: h, LastJobID: h}
	bout := BoutSnapshot{Version: card.Version, State: card.State, Error: card.Error, View: &v, StoreID: h,
		SourceCreatedAtMS: card.SourceCreatedAtMS, CreatedAtMS: card.CreatedAtMS, ActiveJobID: h, QueuedJobID: h, LastJobID: h}
	result := ResultSnapshot{Version: card.Version, State: card.State, Error: card.Error, View: &v, StoreID: h,
		SourceCreatedAtMS: card.SourceCreatedAtMS, CreatedAtMS: card.CreatedAtMS, ActiveJobID: h, QueuedJobID: h, LastJobID: h,
		PublisherAccountID: h, NodeState: "starting", NodeOwner: "publication"}
	for _, tc := range []struct {
		name                         string
		value                        any
		pinCap, base, publicMetadata int
	}{{"card", card, gymcard.MaxPinSize, cardRetainedMetadataBytes, MaxCardMetadataSize},
		{"bout", bout, gymbout.MaxPinSize, boutRetainedMetadataBytes, MaxBoutMetadataSize},
		{"result", result, gymresult.MaxPinSize, resultRetainedMetadataBytes, MaxResultMetadataSize}} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			// Remove only the null Pin value; its full independently enforced core
			// cap is substituted below. Keys/commas/quotes/full View remain counted.
			shell := len(raw) - len("null")
			if shell > 2048 || tc.base != tc.pinCap+2048+4096 || tc.base >= tc.publicMetadata {
				t.Fatalf("shell=%d base=%d pin=%d public=%d", shell, tc.base, tc.pinCap, tc.publicMetadata)
			}
		})
	}
	if cardRetainedMetadataBytes != 26*1024 || boutRetainedMetadataBytes != 30*1024 || resultRetainedMetadataBytes != 38*1024 {
		t.Fatal("retained partition changed")
	}
	if MaxCardCacheSize != 288*1024 || MaxBoutCacheSize != 384*1024 || MaxResultCacheSize != 288*1024 ||
		MaxCardMetadataSize != 48*1024 || MaxBoutMetadataSize != 72*1024 || MaxResultMetadataSize != 48*1024 ||
		MaxCareerCacheSize != 200*1024 || careerHistoryAllowance != 512*1024 || MaxManifestCacheBytes != 1024*1024 {
		t.Fatal("a public/full-admission/aggregate cap changed")
	}
}

func retainedReferenceJob(t *testing.T, path, digest, kind string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != digest {
		t.Fatal("frozen input changed")
	}
	type dto struct{ Category, Kind, Canonical string }
	var doc struct{ DTOs, Cases []dto }
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var largest []byte
	for _, d := range append(doc.DTOs, doc.Cases...) {
		if d.Kind != kind || d.Category != "" && d.Category != "dto" {
			continue
		}
		var j struct {
			State  string
			Result json.RawMessage
		}
		if json.Unmarshal([]byte(d.Canonical), &j) != nil {
			t.Fatal("canonical job")
		}
		if j.State == "completed" && len(j.Result) > 4 && len(d.Canonical) > len(largest) {
			largest = []byte(d.Canonical)
		}
	}
	if len(largest) == 0 {
		t.Fatal("missing full saved-result positive")
	}
	return largest
}

func TestFamilyRetainedMetadataChargesFullTerminal(t *testing.T) {
	for _, tc := range []struct {
		kind, path, hash string
		base             int
	}{
		{"CardJob", "../../gymcard/testdata/reference/card_v1.json", "c52f513f8dfda34c884312c6a2caaf93b429bbf788aaf7c2cba3fbd901329790", cardRetainedMetadataBytes},
		{"BoutJob", "../../gymbout/testdata/reference/bout_v1.json", "09f0430789328b9226fc4a24aae3cdb07beeb8b41d61b42cd33e4ba3121fa2ec", boutRetainedMetadataBytes},
		{"ResultJob", "../../gymresult/testdata/reference/fragments/wire.json", "aa60fdb0267a77365039d95545b61e8a4adb256bad8366325d89fdf081611408", resultRetainedMetadataBytes},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			// Direct retain is a bounded ownership/accounting unit check, not a
			// claim that these independent payloads were admitted concurrently.
			s := &StableController{ManifestController: &ManifestController{cache: map[uint64]*manifestCached{}}}
			protected := &manifestCached{}
			s.cache[1] = protected
			s.careerBytes, s.historyBytes = MaxCareerCacheSize, careerHistoryAllowance
			raw := retainedReferenceJob(t, tc.path, tc.hash, tc.kind)
			var retained []byte
			var charge int
			switch tc.kind {
			case "CardJob":
				var j CardJob
				if json.Unmarshal(raw, &j) != nil || !cardJobValid(&j) {
					t.Fatal("card input")
				}
				s.retainCardLocked(j)
				j.Result.Record.CardID = strings.Repeat("0", 64)
				retained, _ = json.Marshal(s.cardLast)
				charge = s.cardBytes
			case "BoutJob":
				var j BoutJob
				if json.Unmarshal(raw, &j) != nil || !boutJobValid(&j) {
					t.Fatal("bout input")
				}
				s.retainBoutLocked(j)
				j.Result.Record.BoutID = strings.Repeat("0", 64)
				retained, _ = json.Marshal(s.boutLast)
				charge = s.boutBytes
			case "ResultJob":
				var j ResultJob
				if json.Unmarshal(raw, &j) != nil || !resultJobValid(&j) {
					t.Fatal("result input")
				}
				s.retainResultLocked(j)
				j.Result.Record.ResultKey = strings.Repeat("0", 64)
				retained, _ = json.Marshal(s.resultLast)
				charge = s.resultBytes
			}
			if !bytes.Equal(raw, retained) || charge != tc.base+len(retained) {
				t.Fatalf("terminal ownership/charge: %d != %d+%d", charge, tc.base, len(retained))
			}
			if s.cache[1] != protected || s.careerBytes != MaxCareerCacheSize || s.historyBytes != careerHistoryAllowance {
				t.Fatal("unrelated evidence evicted or charge changed")
			}
		})
	}
}
