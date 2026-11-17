// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// These are transport-shape examples, never purported proof/ownership goldens.
func manifestBridgeSnapshot(t *testing.T) pilot.ManifestSnapshot {
	t.Helper()
	s := pilot.BlankManifestSnapshot()
	id, e := gymaccount.FixtureIdentity("zero256")
	if e != nil {
		t.Fatal(e)
	}
	s.Account = pilot.AccountSnapshot{State: "signed_in", Fixture: "zero256", ID: strings.Repeat("b", 64), Generation: 1, CanExport: true}
	s.GymID = id.GymID
	s.GymPublicKey = id.GymPublicKey
	s.PayoutAddress = "amkchND9pP6STAzbb4M6U4SzU2anReC4R4"
	s.View = &pilot.ManifestViewPin{Version: 1, ViewID: strings.Repeat("d", 64), AccountID: s.Account.ID, AccountAuthorityID: strings.Repeat("c", 64), Generation: 1, AuthorityOrigin: "http://127.0.0.1:39111", AuthorityPin: registry.ManifestAuthorityPin{Version: 5, Network: 1397511495, Genesis: "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd", AuthorityID: strings.Repeat("a", 64), DescriptorID: "c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303"}, TransportTrustID: "d97ebf299708f68beeee1ea2c9b5c795ef10784477e1c3560e3d954eb8972471"}
	// Trust ID was independently derived with Python hashlib: domain + LE32
	// origin length + origin bytes + mode1 + LE32 zero certificate count.
	s.Projection.State = "ready"
	s.Projection.Revision = 1
	s.Projection.CompleteRecent = true
	return s
}
func TestManifestBridgeCanonicalScopeAndRevocation(t *testing.T) {
	blank := pilot.BlankManifestSnapshot()
	valid := manifestBridgeSnapshot(t)
	if wire.TestPoW {
		raw, _ := json.Marshal(pilot.ManifestResponse{ID: 1, OK: true, Snapshot: &valid})
		if _, e := decodeManifest(raw); e == nil {
			t.Fatal("production identity accepted by incompatible test build")
		}
		return
	}
	for _, s := range []pilot.ManifestSnapshot{blank, valid} {
		raw, _ := json.Marshal(pilot.ManifestResponse{ID: 1, OK: true, Snapshot: &s})
		if _, e := decodeManifest(raw); e != nil {
			t.Fatal("valid shape refused", e)
		}
		for _, bad := range [][]byte{
			bytes.Replace(raw, []byte(`"id":1`), []byte(`"id":1,"id":1`), 1),
			bytes.Replace(raw, []byte(`"view":`), []byte(`"token":"secret","view":`), 1),
			bytes.Replace(raw, []byte(`"items":[]`), []byte(`"items":null`), 1),
			bytes.Replace(raw, []byte(`"version":1`), []byte(`"Version":1`), 1),
		} {
			if _, e := decodeManifest(bad); e == nil {
				t.Fatal("malformed shape accepted")
			}
		}
	}
	for _, change := range []func(*pilot.ManifestSnapshot){
		func(s *pilot.ManifestSnapshot) { s.Account.State = "expired" },
		func(s *pilot.ManifestSnapshot) { s.View.Generation++ },
		func(s *pilot.ManifestSnapshot) { s.View.AccountID = strings.Repeat("e", 64) },
		func(s *pilot.ManifestSnapshot) { s.Account.Fixture = "incrementing256" },
		func(s *pilot.ManifestSnapshot) { s.View.AuthorityOrigin = "http://localhost:39111" },
		func(s *pilot.ManifestSnapshot) { s.View.TransportTrustID = strings.Repeat("0", 64) },
		func(s *pilot.ManifestSnapshot) { s.View.AuthorityPin.Genesis = strings.Repeat("0", 64) },
		func(s *pilot.ManifestSnapshot) { s.AcceptedCount = 2; s.Projection.HasOlder = true },
		func(s *pilot.ManifestSnapshot) { s.Attempts = 1 << 53 },
		func(s *pilot.ManifestSnapshot) { s.LastError = "private backend details" },
	} {
		s := manifestBridgeSnapshot(t)
		change(&s)
		raw, _ := json.Marshal(pilot.ManifestResponse{ID: 1, OK: true, Snapshot: &s})
		if _, e := decodeManifest(raw); e == nil {
			t.Fatal("contradictory view accepted")
		}
	}
	// A disclosed historical gap is valid. Claiming completeness is not.
	s := manifestBridgeSnapshot(t)
	s.AcceptedCount = 2
	s.Projection.HasOlder = true
	s.Projection.CompleteRecent = false
	raw, _ := json.Marshal(pilot.ManifestResponse{ID: 1, OK: true, Snapshot: &s})
	if _, e := decodeManifest(raw); e != nil {
		t.Fatal(e)
	}
	legacy, _ := json.Marshal(pilot.Response{ID: 1, OK: true, Snapshot: &pilot.Snapshot{Mode: pilot.AccountMode}})
	if _, e := decodeManifest(legacy); e == nil {
		t.Fatal("legacy union accepted")
	}
	if _, e := fighterdescriptor.ResolveCandidate(s.View.AuthorityPin.DescriptorID); e != nil {
		t.Fatal(e)
	}
}

func TestManifestBridgePreallocationBounds(t *testing.T) {
	array := func(count int) string { return "[" + strings.TrimSuffix(strings.Repeat("{},", count), ",") + "]" }
	for _, raw := range []string{
		`{"snapshot":{"projection":{"items":` + array(8) + `}}}`,
		`{"manifest_receipt":{"record":{"proof":{"merkle_branch":` + array(20) + `}}}}`,
		strings.Repeat(`{"a":`, 7) + `0` + strings.Repeat("}", 7),
	} {
		if e := preflightManifest([]byte(raw)); e != nil {
			t.Fatal("bounded shape rejected", e)
		}
	}
	for _, raw := range []string{
		`{"snapshot":{"projection":{"items":` + array(9) + `}}}`,
		`{"manifest_receipt":{"record":{"proof":{"merkle_branch":` + array(21) + `}}}}`,
		`{"other":[]}`,
		`{"snapshot":{"projection":{"items":[{"items":[]}]}}}`,
		`{"` + strings.Repeat("a", 65) + `":0}`,
		strings.Repeat(`{"a":`, 8) + `0` + strings.Repeat("}", 8),
		`{} {}`,
		`{"a":`,
	} {
		if e := preflightManifest([]byte(raw)); e == nil {
			t.Fatal("unbounded/malformed shape reached typed decode")
		}
	}
}

func TestManifestBridgePortraitBoundary(t *testing.T) {
	snapshot := manifestBridgeSnapshot(t)
	// Minimal transport-shape control; this is not a renderer golden. Digest
	// derived separately with Python hashlib over the domain and literal bytes.
	valid := pilot.ManifestPortrait{Version: 1, View: *snapshot.View, Sequence: 1,
		ID: strings.Repeat("a", 64), ManifestDigest: strings.Repeat("b", 64), SVG: "<svg/>",
		PortraitDigest: "844e9864f44f11b53de11b3f9dc26a4b2f4a0e357a01774fcd22358cb1d0e4cf"}
	raw, _ := json.Marshal(pilot.ManifestResponse{ID: 1, OK: true, Portrait: &valid})
	if wire.TestPoW {
		if _, e := decodeManifest(raw); e == nil {
			t.Fatal("production portrait identity accepted by incompatible test build")
		}
		return
	}
	if _, e := decodeManifest(raw); e != nil {
		t.Fatal("valid portrait transport refused", e)
	}
	for _, change := range []func(*pilot.ManifestPortrait){
		func(p *pilot.ManifestPortrait) { p.SVG = "<svG/>" },
		func(p *pilot.ManifestPortrait) { p.PortraitDigest = strings.Repeat("0", 64) },
		func(p *pilot.ManifestPortrait) { p.SVG += "é" },
		func(p *pilot.ManifestPortrait) { p.SVG += "\x01" },
		func(p *pilot.ManifestPortrait) { p.SVG = strings.Repeat("a", 32769) },
		func(p *pilot.ManifestPortrait) { p.Version++ },
		func(p *pilot.ManifestPortrait) { p.View.AuthorityPin.Genesis = strings.Repeat("0", 64) },
		func(p *pilot.ManifestPortrait) { p.View.ViewID = "" },
	} {
		bad := valid
		change(&bad)
		raw, _ := json.Marshal(pilot.ManifestResponse{ID: 1, OK: true, Portrait: &bad})
		if _, e := decodeManifest(raw); e == nil {
			t.Fatal("invalid portrait transport accepted")
		}
	}
}

func TestManifestBridgeFixture(t *testing.T) {
	scenario := os.Getenv("SLUG_MANIFEST_BRIDGE_FIXTURE")
	if scenario == "" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req pilot.ManifestRequest
		_ = json.Unmarshal(scanner.Bytes(), &req)
		switch scenario {
		case "duplicate":
			fmt.Println(`{"id":1,"id":1,"ok":false,"error":"invalid_request"}`)
		case "oversized":
			fmt.Println(strings.Repeat("x", pilot.MaxResponseSize+1))
		case "wrong_id":
			fmt.Println(`{"id":999,"ok":false,"error":"invalid_request"}`)
		case "legacy":
			raw, _ := json.Marshal(pilot.Response{ID: req.ID, OK: true, Snapshot: &pilot.Snapshot{Mode: pilot.AccountMode}})
			fmt.Println(string(raw))
		case "stall":
			_, _ = io.Copy(io.Discard, os.Stdin)
			os.Exit(0)
		default:
			s := pilot.BlankManifestSnapshot()
			raw, _ := json.Marshal(pilot.ManifestResponse{ID: req.ID, OK: true, Snapshot: &s})
			fmt.Println(string(raw))
		}
	}
	if path := os.Getenv("SLUG_MANIFEST_BRIDGE_DRAIN"); path != "" {
		_ = os.WriteFile(path, []byte("joined"), 0600)
	}
	os.Exit(0)
}
func manifestBridgeFixture(t *testing.T, scenario string) *ManifestBridge {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(exe, "-test.run=^TestManifestBridgeFixture$")
	cmd.Env = append(os.Environ(), "SLUG_MANIFEST_BRIDGE_FIXTURE="+scenario)
	b, e := startManifest(cmd)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}
func TestManifestBridgeMalformedAndCommandCorrelation(t *testing.T) {
	for _, scenario := range []string{"duplicate", "oversized", "wrong_id", "legacy"} {
		t.Run(scenario, func(t *testing.T) {
			b := manifestBridgeFixture(t, scenario)
			if _, e := b.Request(context.Background(), "snapshot", 0); e == nil {
				t.Fatal("invalid frame accepted")
			}
		})
	}
	b := manifestBridgeFixture(t, "valid")
	if _, e := b.Request(context.Background(), "manifest_portrait", 1); e == nil {
		t.Fatal("snapshot accepted as portrait")
	}
}
func TestManifestBridgeCancellationAndCleanDrain(t *testing.T) {
	marker := t.TempDir() + "/joined"
	t.Setenv("SLUG_MANIFEST_BRIDGE_DRAIN", marker)
	b := manifestBridgeFixture(t, "valid")
	if _, e := b.Request(context.Background(), "snapshot", 0); e != nil {
		t.Fatal(e)
	}
	if e := b.Close(); e != nil {
		t.Fatal(e)
	}
	if raw, e := os.ReadFile(marker); e != nil || string(raw) != "joined" {
		t.Fatal("returned before process drain", e)
	}
	if _, e := b.Request(context.Background(), "start", 0); e == nil {
		t.Fatal("closed request accepted")
	}
	stalled := manifestBridgeFixture(t, "stall")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, e := stalled.Request(ctx, "snapshot", 0); e == nil {
		t.Fatal("cancelled request accepted")
	}
	if e := stalled.Close(); e != nil {
		t.Fatal(e)
	}
}
