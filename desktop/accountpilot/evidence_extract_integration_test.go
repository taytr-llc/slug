//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
)

// This is an input extractor, not a fighterevidence test or expected-output
// generator. It uses only APIs that existed at the end of Set65. Original
// fixture trees must be copied and hashed by the separately retained runner.
type evidenceExtractCase struct {
	Name, Root, Output, Node, DescriptorID string
	ClockMS                                int64
	Pins                                   map[string]gymstable.Pin
	Awarded                                map[string]string
}
type evidenceExtractSpec struct {
	Scope string
	Cases []evidenceExtractCase
}

func TestExistingAPIPublicEvidenceExtraction(t *testing.T) {
	path := os.Getenv("SLUG_EVIDENCE_EXTRACT_SPEC")
	if path == "" {
		t.Skip("explicit retained-copy extraction spec required")
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 32768 {
		t.Fatal("bounded extraction spec", err)
	}
	var spec evidenceExtractSpec
	if json.Unmarshal(raw, &spec) != nil || spec.Scope != "set66-existing-api-fixture-copy-extraction" || len(spec.Cases) != 2 {
		t.Fatal("extraction scope")
	}
	for _, item := range spec.Cases {
		t.Run(item.Name, func(t *testing.T) { evidenceExtractExisting(t, item) })
	}
}

func evidenceExtractWrite(t *testing.T, dir, name string, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil || len(b) > 524288 {
		t.Fatal("bounded public input", name, err)
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := f.Write(b)
	syncErr, closeErr := f.Sync(), f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		t.Fatal("public input write", writeErr, syncErr, closeErr)
	}
}

func evidenceExtractExisting(t *testing.T, item evidenceExtractCase) {
	if item.Name != "v1" && item.Name != "v2" || !filepath.IsAbs(item.Root) || !filepath.IsAbs(item.Output) || !filepath.IsAbs(item.Node) || filepath.Base(filepath.Dir(item.Root)) != "work-copies" || filepath.Base(item.Root) != item.Name || item.ClockMS <= 0 || len(item.Pins) != 2 || len(item.Awarded) != 2 {
		t.Fatal("explicit owned copy required")
	}
	real, err := filepath.EvalSymlinks(item.Root)
	if err != nil || real != item.Root {
		t.Fatal("canonical copied root", err)
	}
	for _, name := range []string{"SLUG-POOL-ACCOUNTS.json", "shared/SLUG-POOL-RUNTIME.json", "shared/SLUG-POOL-SOURCE.json", "shared/SLUG-POOL-READY.json", "shared/event-cycle-v1/READY.json", "shared/event-cycle-v1/ACTIVATION.json", "shared/event-cycle-v1/seeds-READY.json", "shared/event-cycle-v1/cards-READY.json", "shared/event-cycle-v1/bouts-READY.json", "shared/event-cycle-v1/results-READY.json"} {
		// The runner inventories every established marker; this check must
		// never create a new authority to repair a missing copied component.
		if _, err := os.Stat(filepath.Join(item.Root, name)); err != nil {
			t.Fatal("established copy missing", name, err)
		}
	}
	fixtures := []string{"zero256", "incrementing256"}
	for _, fixture := range fixtures {
		pin := item.Pins[fixture]
		if _, err := gymstable.ValidatePin(pin); err != nil || pin.Version != gymstable.SharedPinVersion2 || pin.Source.DescriptorID != item.DescriptorID || len(item.Awarded[fixture]) != 64 {
			t.Fatal("independently retained expected pin", fixture, err)
		}
	}
	if item.Pins[fixtures[0]].Source != item.Pins[fixtures[1]].Source {
		t.Fatal("shared source changed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	// Read through the concrete copy while no runtime owns its issuer. The
	// source pin was frozen separately from the later capture/records.
	issuer, err := registry.OpenManifestAuthority(filepath.Join(item.Root, "shared", "issuer"), item.Pins[fixtures[0]].Source, true)
	if err != nil {
		t.Fatal("copy issuer", err)
	}
	beforeInfo, err := issuer.Info()
	if err != nil {
		_ = issuer.Close()
		t.Fatal(err)
	}
	receipts := map[string]registry.ManifestRecord{}
	for _, fixture := range fixtures {
		r, err := issuer.GetManifestAdmission(item.Awarded[fixture])
		if err != nil || r.Verified.GymID != item.Pins[fixture].Identity.GymID {
			_ = issuer.Close()
			t.Fatal("original awarded receipt", fixture, err)
		}
		receipts[fixture+"-awarded"] = r
	}
	// The first eight global rows were mined by the first fixture. Select an
	// original nonbooked receipt; never fabricate a zero-history fighter.
	page, err := issuer.ListManifestAdmissions(0, 8)
	if err != nil {
		_ = issuer.Close()
		t.Fatal(err)
	}
	for _, r := range page {
		if r.Verified.GymID == item.Pins[fixtures[0]].Identity.GymID && r.Verified.ProspectID != item.Awarded[fixtures[0]] {
			receipts[fixtures[0]+"-unawarded"] = r
			break
		}
	}
	if len(receipts) != 3 {
		_ = issuer.Close()
		t.Fatal("original unawarded receipt absent")
	}
	if err := issuer.Close(); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return time.UnixMilli(item.ClockMS) }
	c, err := NewPoolForTestClockWithOptions(ctx, pilot.Config{DataDir: item.Root, NodeBinary: item.Node}, clock, pilot.PoolOptions{CandidateDescriptorID: item.DescriptorID})
	if err != nil {
		t.Fatal("reopen existing copied runtime", err)
	}
	t.Cleanup(func() {
		if c != nil {
			if err := c.Close(); err != nil {
				t.Error("copy close", err)
			}
		}
	})
	rpc := poolActualNode(t, item.Root)
	defer rpc.client.CloseIdleConnections()
	var beforeHeight, afterHeight uint32
	var beforeHash, afterHash string
	if err := rpc.call(ctx, "getblockcount", nil, &beforeHeight); err != nil {
		t.Fatal(err)
	}
	if err := rpc.call(ctx, "getblockhash", []any{beforeHeight}, &beforeHash); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		s := poolActualLogin(t, ctx, c, fixture)
		if s.View == nil || s.View.AuthorityPin != item.Pins[fixture].Source || s.View.AccountID != item.Pins[fixture].AccountID || s.View.AccountAuthorityID != item.Pins[fixture].AccountAuthorityID || s.Attempts != 0 || s.State != "stopped" {
			t.Fatal("copied original view binding")
		}
		for _, suffix := range []string{"awarded", "unawarded"} {
			label := fixture + "-" + suffix
			receipt, exists := receipts[label]
			if !exists {
				continue
			}
			j := poolCycleDispatch(t, ctx, "history input capture", func(work context.Context) (pilot.CareerHistoryJob, error) {
				return c.BeginCareerHistory(work, s.View.ViewID, receipt.Verified.ProspectID)
			})
			poolActualWait(t, ctx, "history input completion", func(work context.Context) bool {
				next, e := c.CareerHistoryJob(work, s.View.ViewID, j.JobID)
				if poolCycleBusy(e) {
					return false
				}
				if e != nil {
					t.Fatal(e)
				}
				j = next
				return j.State != "queued" && j.State != "running"
			})
			if j.State != "done" || j.Capture == nil || j.Capture.Status.Pin != item.Pins[fixture] || j.Capture.AsOfMS != item.ClockMS || j.Capture.Assignment.ProspectID != receipt.Verified.ProspectID || j.Capture.Assignment.Sequence != receipt.Sequence {
				t.Fatal("exact captured input", label, j.State, j.Error)
			}
			if (suffix == "awarded" && j.Capture.EventCount != 1) || (suffix == "unawarded" && j.Capture.EventCount != 0) {
				t.Fatal("actual history coverage", label, j.Capture.EventCount)
			}
			dir := filepath.Join(item.Output, label)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			rows := []gymstable.CareerHistoryRow{}
			for after := uint32(0); ; {
				var result pilot.CareerHistoryPageResult
				poolCycleRead(t, ctx, "original history page", func() error {
					var e error
					result, e = c.CareerHistoryPage(ctx, s.View.ViewID, j.JobID, after, 8)
					return e
				})
				if !reflect.DeepEqual(result.Page.Capture, *j.Capture) || result.Page.After != after {
					t.Fatal("capture changed during extraction")
				}
				rows = append(rows, result.Page.Items...)
				if !result.Page.HasMore {
					break
				}
				if result.Page.Next <= after || result.Page.Next > 256 {
					t.Fatal("bounded page cursor")
				}
				after = result.Page.Next
			}
			if len(rows) != int(j.Capture.EventCount) {
				t.Fatal("input rows omitted")
			}
			evidenceExtractWrite(t, dir, "admission.json", receipt)
			evidenceExtractWrite(t, dir, "capture.json", *j.Capture)
			evidenceExtractWrite(t, dir, "rows.json", rows)
			evidenceExtractWrite(t, dir, "expected-pin.json", item.Pins[fixture])
			t.Log("extracted original public inputs", item.Name, label, "sequence", receipt.Sequence, "rows", len(rows))
		}
		poolActualLogout(t, ctx, c)
	}
	if err := rpc.call(ctx, "getblockcount", nil, &afterHeight); err != nil {
		t.Fatal(err)
	}
	if err := rpc.call(ctx, "getblockhash", []any{afterHeight}, &afterHash); err != nil {
		t.Fatal(err)
	}
	if beforeHeight != afterHeight || beforeHash != afterHash {
		t.Fatal("extraction changed chain tip")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c = nil
	issuer, err = registry.OpenManifestAuthority(filepath.Join(item.Root, "shared", "issuer"), item.Pins[fixtures[0]].Source, true)
	if err != nil {
		t.Fatal("copy issuer postcheck", err)
	}
	afterInfo, infoErr := issuer.Info()
	closeErr := issuer.Close()
	if infoErr != nil || closeErr != nil || !reflect.DeepEqual(beforeInfo, afterInfo) {
		t.Fatal("extraction changed source authority/jobs/records", infoErr, closeErr)
	}
	evidenceExtractWrite(t, item.Output, "report.json", map[string]any{"scope": "existing APIs, retained copied fixture source only; no mining/authority creation", "descriptor_id": item.DescriptorID, "clock_ms": item.ClockMS, "height": beforeHeight, "block_hash": beforeHash, "cases": 3, "start_called": false, "source": strings.TrimSpace(item.Name)})
}
