// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/registry"
)

func TestOwnedSourceRuntimeCutoffResumeBeforeLeague(t *testing.T) {
	r, _, _ := ownedStartupSource(t, false)
	ctx := context.Background()
	o := r.owned
	source, e := fighterownership.NewPublicSource(ctx, o.config.Ownership, o.publisher)
	if e != nil {
		t.Fatal(e)
	}
	boundary, e := o.store.CaptureBoundary(ctx, source, o.node, strings.Repeat("71", 32), time.Now().Add(150*time.Millisecond).UnixMilli())
	if e != nil {
		t.Fatal(e)
	}
	replay, e := o.store.BeginCutoffReplay(ctx, source, o.node, boundary)
	if e != nil {
		t.Fatal(e)
	}
	defer replay.Close()
	var cutoff *fighterownership.CutoffRecord
	for n := 0; n < 20; n++ {
		progress, e := replay.Step(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if progress.Ready {
			cutoff = progress.Cutoff
			break
		}
	}
	if cutoff == nil {
		t.Fatal("finite normal genesis cutoff replay incomplete")
	}
	prepared, e := o.publisher.PrepareCutoffPublication(ctx, *cutoff)
	if e != nil {
		t.Fatal(e)
	}
	want, _ := json.Marshal(prepared.Entry())
	if _, e = o.publisher.PublishNext(ctx); e != registry.ErrBusy {
		t.Fatal("missing predecessor queue obstruction", e)
	}
	// Reopen both stores before league construction. Parsed rows alone must
	// neither grant readiness nor require a missing league to drain its queue.
	if e = o.store.Close(); e != nil {
		t.Fatal(e)
	}
	if e = r.issuer.Close(); e != nil {
		t.Fatal(e)
	}
	r.issuer, e = registry.OpenPublicManifestAuthority(filepath.Join(r.config.DataDir, "issuer"), o.config.Ownership)
	if e != nil {
		t.Fatal(e)
	}
	o.publisher, e = r.issuer.PublicPublication(o.config.Ownership)
	if e != nil {
		t.Fatal(e)
	}
	o.store, e = fighterownership.Open(filepath.Join(r.config.DataDir, ownedSourceDirectory), o.config.Ownership, o.config.Store)
	if e != nil {
		t.Fatal(e)
	}
	if r.ownedGym.league != nil || r.Ready() {
		t.Fatal("fixture already has a ready league")
	}
	progress, e := r.RefreshOwnership(ctx, 1)
	if e != nil || progress.Ready || progress.WorkID == "" {
		t.Fatal("reference recovery did not start ordinary verification", e)
	}
	entry, e := o.publisher.Publication(ctx, 1)
	if e != nil || entry.Kind != "cutoff" {
		t.Fatal("pending cutoff not first", e)
	}
	entry.Signature = ""
	got, _ := json.Marshal(entry)
	if !bytes.Equal(got, want) {
		t.Fatal("runtime retimestamped or replaced pending reference")
	}
	if _, e = o.store.Head(ctx); e != fighterownership.ErrUnavailable {
		t.Fatal("reference recovery granted Head", e)
	}
	if _, e = o.store.Cutoff(ctx, cutoff.EventID); e != fighterownership.ErrUnavailable {
		t.Fatal("reference recovery granted event authority", e)
	}
	if e = r.PrepareOwnedSource(ctx, o.config); e != nil {
		t.Fatal("pre-league startup could not complete normal verification", e)
	}
	if _, e = o.store.Head(ctx); e != nil {
		t.Fatal("normal verification not ready", e)
	}
	if r.ownedGym.league != nil || r.Ready() {
		t.Fatal("source prerequisite created/published a league")
	}
	stored, e := o.publisher.Cutoff(ctx, cutoff.EventID)
	if e != nil || stored != *cutoff {
		t.Fatal("original signed cutoff changed", e)
	}
}
