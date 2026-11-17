// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/registry"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestManifestProtocolStrictCommandShapes(t *testing.T) {
	id := strings.Repeat("00", 32)
	valid := []string{
		`{"id":1,"command":"snapshot"}`, `{"id":2,"command":"start"}`, `{"id":3,"command":"stop"}`,
		`{"id":1,"command":"login_fixture","fixture":"zero256"}`, `{"id":1,"command":"logout"}`, `{"id":1,"command":"reset_fixture"}`,
		`{"id":1,"command":"refresh_manifest_prospects"}`, `{"id":1,"command":"manifest_portrait","sequence":1}`,
		`{"id":1,"command":"manifest_receipt","sequence":1000}`, `{"id":1,"command":"export_recovery","path":"/tmp/public-fixture.json"}`,
		`{"id":1,"command":"verify_backup","path":"/tmp/public-fixture.json"}`,
		`{"id":1,"command":"export_manifest_receipt","sequence":1,"path":"/tmp/receipt.json","view_id":"` + id + `","prospect_id":"` + id + `"}`,
		`{"id":9007199254740991,"command":"release_manifest_receipt","view_id":"` + id + `","prospect_id":"` + id + `"}`,
	}
	for _, raw := range valid {
		if _, err := parseManifestRequest([]byte(raw)); err != nil {
			t.Fatalf("valid command refused: %s: %v", raw, err)
		}
	}
	invalid := []string{
		`{"id":1,"command":"receipt","sequence":1}`, `{"id":0,"command":"snapshot"}`, `{"id":9007199254740992,"command":"snapshot"}`,
		`{"id":1.0,"command":"snapshot"}`, `{"id":1e0,"command":"snapshot"}`, `{"id":1,"command":"snapshot","sequence":0}`,
		`{"id":1,"command":"snapshot","path":""}`, `{"id":1,"command":"snapshot","fixture":null}`, `{"id":1,"command":"snapshot","ID":2}`,
		`{"id":1,"command":"snapshot","id":2}`, `{"id":1,"command":"snapshot","\u0069d":2}`,
		`{"id":1,"command":"manifest_portrait"}`, `{"id":1,"command":"manifest_portrait","sequence":1001}`,
		`{"id":1,"command":"manifest_receipt","sequence":-1}`, `{"id":1,"command":"manifest_receipt","sequence":1,"view_id":"` + id + `"}`,
		`{"id":1,"command":"export_manifest_receipt","sequence":1,"path":"/tmp/x","view_id":"` + id + `"}`,
		`{"id":1,"command":"release_manifest_receipt","sequence":0,"view_id":"` + id + `","prospect_id":"` + id + `"}`,
		`{"id":1,"command":"login_fixture","fixture":"other"}`, `{"id":1,"command":"export_recovery","path":"relative"}`,
		`{"id":1,"command":"export_recovery","path":"/tmp/a\n"}`, `{"id":1,"command":"snapshot"} {}`,
		`{"id":1,"command":"manifest_receipt","sequence":1,"record":{}}`, strings.Repeat(" ", MaxRequestSize+1),
	}
	for _, raw := range invalid {
		if _, err := parseManifestRequest([]byte(raw)); err == nil {
			t.Fatalf("invalid command accepted: %s", raw)
		}
	}
	for _, field := range []string{"view_id", "prospect_id"} {
		raw := valid[len(valid)-1]
		raw = strings.Replace(raw, `"`+field+`":"`+id+`"`, `"`+field+`":"`+strings.Repeat("AA", 32)+`"`, 1)
		if _, err := parseManifestRequest([]byte(raw)); err == nil {
			t.Fatal("noncanonical pin accepted")
		}
	}
	// New commands cannot be silently routed through the old request codec.
	if _, err := parseRequest([]byte(valid[6])); err == nil {
		t.Fatal("legacy protocol accepted manifest command")
	}
}

type manifestProtocolController struct {
	stopped  atomic.Int32
	closed   atomic.Int32
	once     sync.Once
	closedCh chan struct{}
	failure  error
}

func (c *manifestProtocolController) Start() error { return nil }
func (c *manifestProtocolController) Stop()        { c.stopped.Add(1) }
func (c *manifestProtocolController) Close() error {
	c.once.Do(func() {
		c.closed.Add(1)
		if c.closedCh != nil {
			close(c.closedCh)
		}
	})
	return c.failure
}
func (c *manifestProtocolController) Snapshot() ManifestSnapshot { return BlankManifestSnapshot() }
func (c *manifestProtocolController) LoginFixture(string) error  { return nil }
func (c *manifestProtocolController) Logout() error              { return nil }
func (c *manifestProtocolController) ResetFixtureAccount() error { return nil }
func (c *manifestProtocolController) RefreshManifestProspects(context.Context) error {
	return c.failure
}
func (c *manifestProtocolController) ManifestPortrait(context.Context, uint64) (ManifestPortrait, error) {
	return ManifestPortrait{SVG: "must not leak on error"}, c.failure
}
func (c *manifestProtocolController) ManifestReceipt(context.Context, uint64) (ManifestReceipt, error) {
	return ManifestReceipt{}, c.failure
}
func (c *manifestProtocolController) ExportManifestReceipt(context.Context, uint64, string, string, string) error {
	return c.failure
}
func (c *manifestProtocolController) ReleaseManifestReceipt(context.Context, string, string) error {
	return c.failure
}
func (c *manifestProtocolController) ExportRecovery(context.Context, string) error { return c.failure }
func (c *manifestProtocolController) VerifyBackup(context.Context, string) error   { return c.failure }

func TestManifestProtocolTypedErrorsAndBounds(t *testing.T) {
	c := &manifestProtocolController{failure: errors.New("private /wallet/path token=secret")}
	for _, command := range []string{"manifest_portrait", "manifest_receipt", "export_manifest_receipt", "refresh_manifest_prospects"} {
		response := handleManifestRequest(context.Background(), ManifestRequest{ID: 1, Command: command, Sequence: 1}, c)
		raw, _ := json.Marshal(response)
		if response.OK || response.Portrait != nil || response.ManifestReceipt != nil || response.Snapshot != nil || strings.Contains(string(raw), "private") || strings.Contains(string(raw), "must not leak") {
			t.Fatal("partial/unsafe error", string(raw))
		}
	}
	c.failure = errors.Join(errors.New("hidden"), ErrManifestProjectionRequired)
	if got := handleManifestRequest(context.Background(), ManifestRequest{ID: 1, Command: "manifest_receipt"}, c).Error; got != "projection_required" {
		t.Fatal(got)
	}
	var output bytes.Buffer
	portrait := ManifestPortrait{SVG: strings.Repeat("x", MaxResponseSize)}
	if err := respondManifest(&output, ManifestResponse{ID: 1, OK: true, Portrait: &portrait}); err == nil || output.Len() != 0 {
		t.Fatal("oversized response partially written")
	}
	blank := BlankManifestSnapshot()
	if err := respondManifest(&output, ManifestResponse{ID: 1, OK: true, Snapshot: &blank}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"view":null`)) || !bytes.Contains(output.Bytes(), []byte(`"items":[]`)) {
		t.Fatal("blank shape")
	}
}

type manifestBlockedOutput struct {
	entered, release chan struct{}
	once             sync.Once
}

func (w *manifestBlockedOutput) Write([]byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return 0, io.ErrClosedPipe
}
func (w *manifestBlockedOutput) Close() error {
	select {
	case <-w.release:
	default:
		close(w.release)
	}
	return nil
}
func TestManifestProtocolEOFDrainsBlockedOutput(t *testing.T) {
	input, writer := io.Pipe()
	output := &manifestBlockedOutput{entered: make(chan struct{}), release: make(chan struct{})}
	c := &manifestProtocolController{closedCh: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- ServeManifest(context.Background(), input, output, c) }()
	if _, err := io.WriteString(writer, "{\"id\":1,\"command\":\"snapshot\"}\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-output.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("no response")
	}
	_ = writer.Close()
	select {
	case <-c.closedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("EOF did not independently drain controller")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked stdout outlived quit")
	}
	if c.closed.Load() != 1 || c.stopped.Load() == 0 {
		t.Fatal("bad shutdown")
	}
}
func TestManifestProtocolCancelDrainsAndRetainsCloseFailure(t *testing.T) {
	input, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	c := &manifestProtocolController{failure: errors.New("drain failed")}
	done := make(chan error, 1)
	go func() { done <- ServeManifest(ctx, input, io.Discard, c) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, c.failure) {
			t.Fatal("close failure lost", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not drain")
	}
}

// The largest independently derived set37 receipt must remain transportable;
// the response preflight may not reject the actual maximum proof shape.
func TestManifestProtocolIndependentMaximumReceipt(t *testing.T) {
	raw, err := os.ReadFile("../../registry/testdata/manifest/reference/manifest_admission_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			Name      string                  `json:"name"`
			Record    registry.ManifestRecord `json:"expected_record"`
			Canonical string                  `json:"record_canonical"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tc := range corpus.Cases {
		if tc.Name != "maximum-proof-current-parent" {
			continue
		}
		found = true
		if len(tc.Record.Proof.Coinbase) != 32768 || len(tc.Record.Proof.MerkleBranch) != 20 {
			t.Fatal("maximum fixture shape")
		}
		var out bytes.Buffer
		value := ManifestReceipt{Version: 1, Record: tc.Record}
		if err := respondManifest(&out, ManifestResponse{ID: 1, OK: true, ManifestReceipt: &value}); err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Receipt struct {
				Record json.RawMessage `json:"record"`
			} `json:"manifest_receipt"`
		}
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if string(envelope.Receipt.Record) != tc.Canonical {
			t.Fatal("public evidence bytes changed")
		}
	}
	if !found {
		t.Fatal("missing independent maximum fixture")
	}
}

func FuzzManifestProtocolRequest(f *testing.F) {
	for _, raw := range []string{`{"id":1,"command":"snapshot"}`, `{"id":2,"command":"manifest_receipt","sequence":1000}`, `{"id":1,"command":"snapshot","id":2}`, `{"id":1,"command":"export_recovery","path":"/tmp/public-fixture.json"}`} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		request, err := parseManifestRequest(raw)
		if err != nil {
			return
		}
		canonical, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		again, err := parseManifestRequest(canonical)
		if err != nil || again != request {
			t.Fatal("accepted command failed canonical round trip")
		}
	})
}
