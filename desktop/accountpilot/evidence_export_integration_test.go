//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterevidence"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
)

type evidenceExportExpected struct {
	Name      string                 `json:"name"`
	Pin       gymstable.Pin          `json:"expected_pin"`
	Canonical string                 `json:"canonical"`
	Report    fighterevidence.Report `json:"report"`
}

func TestPublicFighterEvidenceActualCopiedSources(t *testing.T) {
	path := os.Getenv("SLUG_EVIDENCE_EXPORT_SPEC")
	if path == "" {
		t.Skip("explicit retained-copy live export spec required")
	}
	raw, e := os.ReadFile(path)
	if e != nil || len(raw) > 32768 {
		t.Fatal("bounded export spec", e)
	}
	var spec evidenceExtractSpec
	if json.Unmarshal(raw, &spec) != nil || spec.Scope != "set66-existing-copy-live-evidence-export" || len(spec.Cases) != 2 {
		t.Fatal("explicit export scope")
	}
	raw, e = os.ReadFile("../../fighterevidence/testdata/reference/reference-v1.json")
	if e != nil || len(raw) > 4*1024*1024 {
		t.Fatal("frozen independent corpus", e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "eb51028daf2ebbda44dfbfdb79f58353137ed03f1836fad5b8db8cf428300772" {
		t.Fatal("independent corpus changed")
	}
	var corpus struct {
		Positives []evidenceExportExpected `json:"positives"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("independent actual cases")
	}
	expected := map[string]evidenceExportExpected{}
	for _, p := range corpus.Positives {
		if strings.HasPrefix(p.Name, "actual/") {
			expected[p.Name] = p
		}
	}
	if len(expected) != 6 {
		t.Fatal("six actual public inputs required")
	}
	for _, item := range spec.Cases {
		t.Run(item.Name, func(t *testing.T) { evidenceExportActual(t, item, expected) })
	}
}

func evidenceExportActual(t *testing.T, item evidenceExtractCase, expected map[string]evidenceExportExpected) {
	if item.Name != "v1" && item.Name != "v2" || !filepath.IsAbs(item.Root) || !filepath.IsAbs(item.Output) || !filepath.IsAbs(item.Node) || filepath.Base(filepath.Dir(item.Root)) != "work-copies" || filepath.Base(item.Root) != item.Name || item.ClockMS <= 0 || len(item.Pins) != 2 {
		t.Fatal("owned fixture copies required")
	}
	real, e := filepath.EvalSymlinks(item.Root)
	if e != nil || real != item.Root {
		t.Fatal("canonical fixture copy", e)
	}
	for _, name := range []string{"SLUG-POOL-ACCOUNTS.json", "shared/SLUG-POOL-RUNTIME.json", "shared/SLUG-POOL-SOURCE.json", "shared/SLUG-POOL-READY.json", "shared/event-cycle-v1/READY.json", "shared/event-cycle-v1/ACTIVATION.json", "shared/event-cycle-v1/seeds-READY.json", "shared/event-cycle-v1/cards-READY.json", "shared/event-cycle-v1/bouts-READY.json", "shared/event-cycle-v1/results-READY.json"} {
		if _, e = os.Stat(filepath.Join(item.Root, name)); e != nil {
			t.Fatal("established component missing", name, e)
		}
	}
	fixtures := []string{"zero256", "incrementing256"}
	for _, fixture := range fixtures {
		pin := item.Pins[fixture]
		if _, e = gymstable.ValidatePin(pin); e != nil || pin.Version != 2 || pin.Source.DescriptorID != item.DescriptorID || pin != expected["actual/"+item.Name+"/"+fixture+"-awarded"].Pin {
			t.Fatal("separate exact expected pin", fixture, e)
		}
	}
	if item.Pins[fixtures[0]].Source != item.Pins[fixtures[1]].Source {
		t.Fatal("shared source changed")
	}
	issuer, e := registry.OpenManifestAuthority(filepath.Join(item.Root, "shared/issuer"), item.Pins[fixtures[0]].Source, true)
	if e != nil {
		t.Fatal(e)
	}
	beforeInfo, infoErr := issuer.Info()
	closeErr := issuer.Close()
	if infoErr != nil || closeErr != nil {
		t.Fatal(infoErr, closeErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	clock := func() time.Time { return time.UnixMilli(item.ClockMS) }
	c, e := NewPoolForTestClockWithOptions(ctx, pilot.Config{DataDir: item.Root, NodeBinary: item.Node}, clock, pilot.PoolOptions{CandidateDescriptorID: item.DescriptorID})
	if e != nil {
		t.Fatal("existing copied runtime", e)
	}
	t.Cleanup(func() {
		if c != nil {
			if e := c.Close(); e != nil {
				t.Error(e)
			}
		}
	})
	rpc := poolActualNode(t, item.Root)
	defer rpc.client.CloseIdleConnections()
	var beforeHeight, afterHeight uint32
	var beforeHash, afterHash string
	if e = rpc.call(ctx, "getblockcount", nil, &beforeHeight); e != nil {
		t.Fatal(e)
	}
	if e = rpc.call(ctx, "getblockhash", []any{beforeHeight}, &beforeHash); e != nil {
		t.Fatal(e)
	}
	var priorView, priorJob string
	var summaries []pilot.FighterEvidenceSummary
	for _, fixture := range fixtures {
		s := poolActualLogin(t, ctx, c, fixture)
		c.Stop()
		if s.View == nil || s.View.AuthorityPin != item.Pins[fixture].Source || s.View.AccountID != item.Pins[fixture].AccountID || s.View.AccountAuthorityID != item.Pins[fixture].AccountAuthorityID || s.Attempts != 0 || s.State != "stopped" {
			t.Fatal("original account/source binding")
		}
		if priorView != "" {
			j, e := c.FighterEvidenceJob(ctx, priorView, priorJob)
			if e == nil || j.Version != 0 || j.Summary != nil {
				t.Fatal("new account disclosed old evidence job", e)
			}
		}
		if item.Name == "v1" && fixture == "zero256" {
			recent := poolCycleDispatch(t, ctx, "recent original fighters", func(work context.Context) (pilot.StableJob, error) { return c.BeginStableRecent(work, s.View.ViewID) })
			poolActualWait(t, ctx, "recent original fighters complete", func(work context.Context) bool {
				next, e := c.StableJob(work, s.View.ViewID, recent.JobID)
				if poolCycleBusy(e) {
					return false
				}
				if e != nil {
					t.Fatal(e)
				}
				recent = next
				return recent.State != "queued" && recent.State != "running"
			})
			if recent.State != "done" || recent.Page == nil || len(recent.Page.Items) != 8 {
				t.Fatal("eight recent original assignments", recent.State, recent.Error)
			}
			for _, row := range recent.Page.Items {
				if row.Assignment.Sequence == 1 {
					t.Fatal("older-than-recent fixture was visible")
				}
			}
		}
		for _, suffix := range []string{"awarded", "unawarded"} {
			label := fixture + "-" + suffix
			p, ok := expected["actual/"+item.Name+"/"+label]
			if !ok {
				continue
			}
			a, e := fighterevidence.Parse([]byte(p.Canonical))
			if e != nil {
				t.Fatal(e)
			}
			j := poolCycleDispatch(t, ctx, "exact selected history", func(work context.Context) (pilot.CareerHistoryJob, error) {
				return c.BeginCareerHistory(work, s.View.ViewID, a.Admission.Verified.ProspectID)
			})
			poolActualWait(t, ctx, "history capture complete", func(work context.Context) bool {
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
			if j.State != "done" || j.Capture == nil || *j.Capture != a.Capture {
				t.Fatal("capture differs from original independently frozen input", label, j.State, j.Error)
			}
			destination := filepath.Join(item.Output, label+".json")
			selection, e := recoveryfile.Capture(destination, nil, recoveryfile.CreateMode)
			if e != nil {
				t.Fatal("native selection", e)
			}
			opHash := sha256.Sum256([]byte("SLUG Set66 actual public export/" + item.Name + "/" + label))
			opID := hex.EncodeToString(opHash[:])
			var export pilot.FighterEvidenceJob
			poolActualWait(t, ctx, "public export dispatch", func(context.Context) bool {
				var err error
				export, err = c.BeginFighterEvidence(ctx, s.View.ViewID, j.JobID, j.Capture.HistoryID, opID, selection.Pin())
				if poolCycleBusy(err) {
					return false
				}
				if errors.Is(err, pilot.ErrEvidenceReplyUnknown) {
					return true
				}
				if err != nil {
					_ = selection.Close()
					t.Fatal("dispatch", err)
				}
				return true
			})
			// Synchronous Begin owns its descriptor before native releases it.
			if e := selection.Close(); e != nil {
				t.Fatal(e)
			}
			poolActualWait(t, ctx, "public export complete", func(work context.Context) bool {
				next, e := c.FighterEvidenceJob(work, s.View.ViewID, opID)
				if poolCycleBusy(e) {
					return false
				}
				if e != nil {
					t.Fatal(e)
				}
				export = next
				return export.State != "running"
			})
			if export.State != "done" || export.Summary == nil || export.HistoryID != j.Capture.HistoryID || export.HistoryJobID != j.JobID || export.View != *s.View {
				t.Fatal("live export failed", label, export.State, export.Error)
			}
			raw, e := os.ReadFile(destination)
			if e != nil || string(raw) != p.Canonical {
				t.Fatal("live file differs from independent exact public bytes", label, e)
			}
			st, e := os.Stat(destination)
			if e != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 {
				t.Fatal("exclusive private file mode", e)
			}
			fileHash := sha256.Sum256(raw)
			summary := *export.Summary
			if summary.Version != 1 || summary.FileSHA256 != hex.EncodeToString(fileHash[:]) || summary.FileSHA256 != p.Report.FileSHA256 || summary.FileBytes != uint32(len(raw)) || summary.FileBytes != uint32(p.Report.FileBytes) || summary.ProspectID != a.Admission.Verified.ProspectID || summary.HistoryID != a.Capture.HistoryID || summary.SourceRecordDigest != a.Capture.Assignment.SourceRecordDigest || summary.EventCount != a.Capture.EventCount || summary.CapturedAsOfMS != a.Capture.AsOfMS {
				t.Fatal("exact export summary", label)
			}
			evidenceExtractWrite(t, item.Output, label+"-expected-pin.json", p.Pin)
			evidenceExtractWrite(t, item.Output, label+"-summary.json", summary)
			summaries = append(summaries, summary)
			priorView, priorJob = s.View.ViewID, opID
			t.Log("exported original public evidence", item.Name, label, "source sequence", a.Admission.Sequence, "rows", a.Capture.EventCount, "file checksum", summary.FileSHA256)
		}
		poolActualLogout(t, ctx, c)
		blank, e := c.FighterEvidenceSnapshot(ctx)
		if e != nil || blank.View != nil || blank.ActiveJobID != "" || blank.LastJobID != "" {
			t.Fatal("signed-out evidence disclosure", e)
		}
		if j, e := c.FighterEvidenceJob(ctx, priorView, priorJob); e == nil || j.Version != 0 || j.Summary != nil {
			t.Fatal("signed-out evidence lookup", e)
		}
	}
	if len(summaries) != 3 {
		t.Fatal("actual awarded and empty evidence coverage")
	}
	if e = rpc.call(ctx, "getblockcount", nil, &afterHeight); e != nil {
		t.Fatal(e)
	}
	if e = rpc.call(ctx, "getblockhash", []any{afterHeight}, &afterHash); e != nil {
		t.Fatal(e)
	}
	if beforeHeight != afterHeight || beforeHash != afterHash {
		t.Fatal("export mined or changed tip")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c = nil
	issuer, e = registry.OpenManifestAuthority(filepath.Join(item.Root, "shared/issuer"), item.Pins[fixtures[0]].Source, true)
	if e != nil {
		t.Fatal(e)
	}
	afterInfo, infoErr := issuer.Info()
	closeErr = issuer.Close()
	if infoErr != nil || closeErr != nil || !reflect.DeepEqual(beforeInfo, afterInfo) {
		t.Fatal("source authority/jobs/records changed", infoErr, closeErr)
	}
	evidenceExtractWrite(t, item.Output, "report.json", map[string]any{"scope": "actual live export on copied retained sources; independent byte equality; no mining or new awards", "descriptor_id": item.DescriptorID, "clock_ms": item.ClockMS, "height": beforeHeight, "block_hash": beforeHash, "exports": summaries, "start_called": false, "original_source_unchanged": true, "old_view_refused": true, "signed_out_blank": true, "outside_recent_eight_checked": item.Name == "v1"})
}
