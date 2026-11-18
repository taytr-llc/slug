// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"time"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
)

type manifestCached struct {
	summary                     ManifestProspectSummary
	manifest                    fightermanifest.Manifest
	record                      registry.ManifestRecord
	svg                         string
	manifestBytes, receiptBytes int
}

func ownedManifestRecord(r registry.ManifestRecord) registry.ManifestRecord {
	r.Proof.MerkleBranch = append([]string{}, r.Proof.MerkleBranch...)
	return r
}
func (c *ManifestController) viewPin() ManifestViewPin {
	return ManifestViewPin{Version: 1, ViewID: c.view.ViewID, AccountAuthorityID: c.account.AuthorityID, AccountID: c.account.AccountID, Generation: c.view.Generation, AuthorityPin: c.manifest.AuthorityPin, AuthorityOrigin: c.manifest.origin(), TransportTrustID: c.manifest.TransportTrustID}
}
func (c *ManifestController) withView(ctx context.Context, fn func(func() error) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return c.view.ViewLease(ctx, func(a gymaccount.Account, recheck func() error) error {
		check := func() error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if a.ID != c.account.AccountID || a.Generation != c.view.Generation || recheck == nil {
				return ErrManifestProjection
			}
			if e := recheck(); e != nil {
				return e
			}
			c.mu.Lock()
			closed := c.closed
			c.mu.Unlock()
			if closed {
				return ErrManifestProjection
			}
			return nil
		}
		if e := check(); e != nil {
			return e
		}
		if e := fn(check); e != nil {
			return e
		}
		return check()
	})
}
func (c *ManifestController) clearManifestCacheLocked(state, code string) {
	c.cache = make(map[uint64]*manifestCached)
	c.receiptSlot = nil
	revision := c.snapshot.Projection.Revision
	if state == "idle" {
		revision = 0
	}
	c.snapshot.Projection = ManifestProjection{State: state, Error: code, Revision: revision, HasOlder: c.snapshot.AcceptedCount > 0, Items: []ManifestProspectSummary{}}
}
func (c *ManifestController) recomputeProjectionLocked() {
	p := &c.snapshot.Projection
	if c.stableSelected {
		p.State = "idle"
		p.Error = ""
		p.Items = []ManifestProspectSummary{}
		p.ThroughSequence = 0
		p.CompleteRecent = false
		p.HasOlder = c.snapshot.AcceptedCount > 0
		return
	}
	p.Items = []ManifestProspectSummary{}
	p.ThroughSequence = 0
	for _, v := range c.cache {
		p.Items = append(p.Items, v.summary)
	}
	sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Sequence > p.Items[j].Sequence })
	if len(p.Items) > 0 {
		p.ThroughSequence = p.Items[0].Sequence
	}
	p.HasOlder = c.snapshot.AcceptedCount > uint64(len(p.Items))
	p.CompleteRecent = p.State == "ready"
	n := c.snapshot.AcceptedCount
	want := min(n, uint64(8))
	if uint64(len(p.Items)) != want {
		p.CompleteRecent = false
	}
	for i, v := range p.Items {
		if v.Sequence != n-uint64(i) {
			p.CompleteRecent = false
		}
	}
}
func manifestCacheFits(cache map[uint64]*manifestCached, slot *ManifestReceipt) bool {
	if len(cache) > 8 {
		return false
	}
	var ms, rs, ss int
	for _, v := range cache {
		ms += v.manifestBytes
		rs += v.receiptBytes
		ss += len(v.svg)
	}
	extra := 0
	if slot != nil {
		b, e := json.Marshal(slot.Record)
		if e != nil || len(b) > registry.MaxRecordSize {
			return false
		}
		extra = len(b)
	}
	return ms <= 65536 && rs <= 524288 && ss <= 262144 && ms+rs+ss+extra <= MaxManifestCacheBytes
}

// projectManifest only consumes records already verified by our owned store or
// worker callback. It is intentionally private, never an import/trust shortcut.
func (c *ManifestController) projectManifest(r registry.ManifestRecord) (*manifestCached, error) {
	p := c.manifest.AuthorityPin
	if r.Version != 5 || r.Sequence == 0 || r.Sequence > 1000 || r.Job.AuthorityID != p.AuthorityID || r.Job.DescriptorID != p.DescriptorID || r.Job.PublicKey != c.manifest.Identity.GymPublicKey || r.Authorization.PublicKey != c.manifest.Identity.GymPublicKey || r.Context.Network != p.Network || r.Context.Genesis != p.Genesis || r.Context != r.Job.Context || r.Birth.BornAtMS <= 0 || r.Birth.BornAtMS > int64(manifestMaxSafe) || r.Birth.AcceptedAtMS <= 0 || r.Birth.AcceptedAtMS > int64(manifestMaxSafe) || len(r.Proof.MerkleBranch) > 20 {
		return nil, ErrManifestProjectionCorrupt
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > registry.MaxRecordSize {
		return nil, ErrManifestProjectionCorrupt
	}
	d, e := fighterdescriptor.ResolveCandidate(p.DescriptorID)
	if e != nil {
		return nil, ErrManifestProjectionCorrupt
	}
	m, e := fightermanifest.GenerateText(d.ManifestVersion, r.Verified.WorkHash)
	if e != nil || m.DNA != r.Verified.DNA {
		return nil, ErrManifestProjectionCorrupt
	}
	md, e := fightermanifest.Digest(m)
	if e != nil {
		return nil, ErrManifestProjectionCorrupt
	}
	pd, e := fighterportrait.Digest(d.RendererVersion, m)
	if e != nil || r.StaticBinding != (registry.StaticBinding{Version: 1, DescriptorID: p.DescriptorID, ManifestDigest: md, PortraitDigest: pd}) {
		return nil, ErrManifestProjectionCorrupt
	}
	mb, e := fightermanifest.Marshal(m)
	if e != nil {
		return nil, ErrManifestProjectionCorrupt
	}
	v := &manifestCached{manifest: m, record: ownedManifestRecord(r), manifestBytes: len(mb), receiptBytes: len(raw)}
	v.summary = ManifestProspectSummary{Sequence: r.Sequence, ID: r.Verified.ProspectID, WorkHash: r.Verified.WorkHash, BornAtMS: r.Birth.BornAtMS, AcceptedAtMS: r.Birth.AcceptedAtMS, Name: m.Name, Physical: m.Physical, Style: m.Style, Rarity: m.DNA.Rarity, TalentBudget: m.DNA.TalentBudget, Attributes: m.DNA.Attributes, DescriptorID: p.DescriptorID, ManifestDigest: md, PortraitDigest: pd, EligibilityMode: r.Eligibility.Source}
	return v, nil
}
func (c *ManifestController) observeManifest(ctx context.Context, r registry.ManifestRecord) error {
	if ctx.Err() != nil {
		return nil
	}
	v, e := c.projectManifest(r)
	if e != nil {
		c.mu.Lock()
		c.clearManifestCacheLocked("failed", "projection_corrupt")
		c.mu.Unlock()
		return ErrManifestProjectionCorrupt
	}
	corrupt := false
	e = c.withView(ctx, func(check func() error) error {
		if e := check(); e != nil {
			return e
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			return ErrManifestProjection
		}
		for seq, old := range c.cache {
			if seq == r.Sequence || old.summary.ID == v.summary.ID {
				if !reflect.DeepEqual(old.record, r) {
					corrupt = true
					c.clearManifestCacheLocked("failed", "projection_corrupt")
					return ErrManifestProjectionCorrupt
				}
				return nil
			}
		}
		next := make(map[uint64]*manifestCached, len(c.cache)+1)
		for seq, old := range c.cache {
			next[seq] = old
		}
		next[r.Sequence] = v
		if len(next) > 8 {
			var low uint64 = ^uint64(0)
			for seq := range next {
				low = min(low, seq)
			}
			delete(next, low)
			if low == r.Sequence {
				return nil
			}
		}
		if !c.cacheFits(next, c.receiptSlot) || c.snapshot.Projection.Revision == manifestMaxSafe {
			corrupt = true
			c.clearManifestCacheLocked("failed", "projection_corrupt")
			return ErrManifestProjectionCorrupt
		}
		c.cache = next
		c.snapshot.AcceptedCount = max(c.snapshot.AcceptedCount, r.Sequence)
		c.snapshot.Projection.State = "ready"
		c.snapshot.Projection.Error = ""
		c.snapshot.Projection.Revision++
		c.recomputeProjectionLocked()
		return nil
	})
	if corrupt {
		return ErrManifestProjectionCorrupt
	}
	// A retired/cancelled original view is a missed notification, never permission
	// to publish under another credential. The parent owns stopping that run.
	_ = e
	return nil
}
func (c *ManifestController) beginOperation(parent context.Context, name string, duration time.Duration) (context.Context, chan struct{}, error) {
	if parent.Err() != nil {
		return nil, nil, parent.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.cancel != nil || c.manifest.Phase != "ready" {
		return nil, nil, ErrManifestProjection
	}
	ctx, cancel := context.WithTimeout(parent, duration)
	c.cancel, c.operation, c.done = cancel, name, make(chan struct{})
	return ctx, c.done, nil
}
func (c *ManifestController) finishOperation(done chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
	c.cancel, c.operation = nil, ""
	c.snapshot.WalletState = "closed"
	close(done)
}
func (c *ManifestController) LoadManifestProspects(ctx context.Context) error {
	return c.startManifestLoad(ctx, false)
}
func (c *ManifestController) RefreshManifestProspects(ctx context.Context) error {
	return c.startManifestLoad(ctx, true)
}
func (c *ManifestController) startManifestLoad(ctx context.Context, async bool) error {
	if e := c.withView(ctx, func(check func() error) error { return check() }); e != nil {
		return ErrManifestProjection
	}
	op, done, e := c.beginOperation(ctx, "projection", 10*time.Minute)
	if e != nil {
		return e
	}
	c.mu.Lock()
	c.clearManifestCacheLocked("loading", "")
	c.mu.Unlock()
	work := func() error {
		defer c.finishOperation(done)
		e := c.loadManifestPage(op)
		if e != nil {
			c.mu.Lock()
			if op.Err() != nil {
				c.clearManifestCacheLocked("cancelled", "projection_cancelled")
			} else {
				code := "projection_unavailable"
				if errors.Is(e, ErrManifestProjectionCorrupt) {
					code = "projection_corrupt"
				}
				c.clearManifestCacheLocked("failed", code)
			}
			c.mu.Unlock()
		}
		return e
	}
	if async {
		go func() { _ = work() }()
		return nil
	}
	return work()
}
func (c *ManifestController) loadManifestPage(ctx context.Context) (resultErr error) {
	if ctx.Err() != nil {
		return ErrManifestProjectionCancelled
	}
	store, e := registry.OpenManifestAuthority(filepath.Join(c.config.DataDir, "manifest-authority"), c.manifest.AuthorityPin, true)
	if e != nil {
		return ErrManifestProjection
	}
	defer func() {
		if store.Close() != nil {
			c.noteShutdown("manifest projection storage close failed")
			resultErr = ErrManifestProjection
		}
	}()
	info, e := store.Info()
	if e != nil || info.Pin != c.manifest.AuthorityPin || info.Records > 1000 {
		return ErrManifestProjectionCorrupt
	}
	next := make(map[uint64]*manifestCached)
	after := uint64(0)
	if info.Records > 8 {
		after = info.Records - 8
	}
	for after < info.Records {
		if ctx.Err() != nil {
			return ErrManifestProjectionCancelled
		}
		rows, e := store.ListManifestAdmissions(after, 1)
		if ctx.Err() != nil {
			return ErrManifestProjectionCancelled
		}
		if e != nil || len(rows) != 1 || rows[0].Sequence != after+1 {
			return ErrManifestProjectionCorrupt
		}
		v, e := c.projectManifest(rows[0])
		if e != nil {
			return e
		}
		for _, old := range next {
			if old.summary.ID == v.summary.ID {
				return ErrManifestProjectionCorrupt
			}
		}
		next[rows[0].Sequence] = v
		after++
	}
	if !c.cacheFits(next, nil) {
		return ErrManifestProjectionCorrupt
	}
	return c.withView(ctx, func(check func() error) error {
		if e := check(); e != nil {
			return e
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed || c.snapshot.Projection.Revision == manifestMaxSafe {
			return ErrManifestProjection
		}
		c.cache = next
		c.receiptSlot = nil
		c.snapshot.AcceptedCount = max(c.snapshot.AcceptedCount, info.Records)
		c.snapshot.Projection.State = "ready"
		c.snapshot.Projection.Error = ""
		c.snapshot.Projection.Revision++
		c.recomputeProjectionLocked()
		p := c.viewPin()
		c.snapshot.View = &p
		return nil
	})
}
func (c *ManifestController) beginLazy(parent context.Context) (context.Context, func(), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.lazyCancel != nil || parent.Err() != nil {
		return nil, nil, ErrManifestProspect
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	c.lazyCancel, c.lazyDone = cancel, done
	return ctx, func() { c.mu.Lock(); defer c.mu.Unlock(); cancel(); c.lazyCancel, c.lazyDone = nil, nil; close(done) }, nil
}
func (c *ManifestController) ManifestPortrait(ctx context.Context, sequence uint64) (out ManifestPortrait, err error) {
	ctx, finish, e := c.beginLazy(ctx)
	if e != nil {
		return out, e
	}
	defer finish()
	var selected manifestCached
	var pin ManifestViewPin
	e = c.withView(ctx, func(check func() error) error {
		if e := check(); e != nil {
			return e
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		v := c.cache[sequence]
		if v == nil {
			return ErrManifestProspect
		}
		selected = *v
		pin = c.viewPin()
		return nil
	})
	if e != nil {
		return ManifestPortrait{}, ErrManifestPortrait
	}
	svg := selected.svg
	if svg == "" {
		d, e := fighterdescriptor.ResolveCandidate(pin.AuthorityPin.DescriptorID)
		if e != nil {
			return out, ErrManifestPortrait
		}
		raw, e := fighterportrait.Render(d.RendererVersion, selected.manifest)
		if e != nil || len(raw) == 0 || len(raw) > 32768 {
			return out, ErrManifestPortrait
		}
		svg = string(raw)
		digest, e := fighterportrait.Digest(d.RendererVersion, selected.manifest)
		if e != nil || digest != selected.summary.PortraitDigest {
			return out, ErrManifestPortrait
		}
	}
	e = c.withView(ctx, func(check func() error) error {
		if e := check(); e != nil {
			return e
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		v := c.cache[sequence]
		if v == nil || v.summary != selected.summary || c.viewPin() != pin {
			return ErrManifestPortrait
		}
		v.svg = svg
		if !c.cacheFits(c.cache, c.receiptSlot) {
			v.svg = ""
			return ErrManifestPortrait
		}
		out = ManifestPortrait{Version: 1, View: pin, Sequence: sequence, ID: v.summary.ID, ManifestDigest: v.summary.ManifestDigest, PortraitDigest: v.summary.PortraitDigest, SVG: svg}
		return nil
	})
	if e != nil {
		return ManifestPortrait{}, ErrManifestPortrait
	}
	return out, nil
}
func (c *ManifestController) ManifestReceipt(ctx context.Context, sequence uint64) (out ManifestReceipt, err error) {
	e := c.withView(ctx, func(check func() error) error {
		if e := check(); e != nil {
			return e
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		v := c.cache[sequence]
		if v == nil {
			return ErrManifestProspect
		}
		selected := ManifestReceipt{Version: 1, View: c.viewPin(), Record: ownedManifestRecord(v.record)}
		if !c.cacheFits(c.cache, &selected) {
			return ErrManifestReceipt
		}
		c.receiptSlot = &selected
		out = selected
		out.Record = ownedManifestRecord(selected.Record)
		return nil
	})
	if e != nil {
		return ManifestReceipt{}, ErrManifestReceipt
	}
	return out, nil
}
func (c *ManifestController) ReleaseManifestReceipt(ctx context.Context, viewID, id string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.receiptSlot != nil && c.receiptSlot.View.ViewID == viewID && c.receiptSlot.Record.Verified.ProspectID == id {
		c.receiptSlot = nil
	}
	return nil
}

func (c *ManifestController) cacheFits(cache map[uint64]*manifestCached, slot *ManifestReceipt) bool {
	return manifestCacheFits(cache, slot) && (c.stable == nil || stableCacheFits(cache, slot, c.careerBytes+c.previewBytes+c.entryBytes+c.eventBytes+c.seedBytes+c.cardBytes+c.boutBytes+c.resultBytes))
}
