//go:build desktopintegration && slugtestclock && !slugtestpow

package accountpilot

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/seednode"
)

// These tests operate only on an externally created cold copy of the public
// Set46 fixture. They never repair its old conflict or replace its admissions.
type boutClockPublic struct {
	Version     uint32              `json:"version"`
	TaggedClock bool                `json:"tagged_clock"`
	NormalPoW   bool                `json:"normal_pow"`
	ClockMS     int64               `json:"clock_ms"`
	Saturday    string              `json:"saturday"`
	Snapshot    pilot.CardSnapshot  `json:"snapshot"`
	Build       gymcard.BuildResult `json:"build"`
	Read        gymcard.ReadResult  `json:"read"`
	Seed        json.RawMessage     `json:"seed"`
}

func boutClockRoot(t *testing.T) (string, string) {
	t.Helper()
	root, node := os.Getenv("SLUG_BOUT_CLOCK_ROOT"), os.Getenv("SLUG_STABLE_NODE")
	if root == "" || node == "" {
		t.Skip("explicit owned cold copy and normal-PoW node required")
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil || !filepath.IsAbs(root) || root != real || !filepath.IsAbs(node) {
		t.Fatal("canonical owned paths required", err)
	}
	var marker struct {
		Version    uint32 `json:"version"`
		Scope      string `json:"scope"`
		SourceRoot string `json:"source_root"`
	}
	b, err := os.ReadFile(filepath.Join(root, "SET47-OWNED-COPY.json"))
	if err != nil || json.Unmarshal(b, &marker) != nil || marker.Version != 1 || marker.Scope != "Set47 owned cold copy" || marker.SourceRoot == root {
		t.Fatal("explicit cold-copy marker required", err)
	}
	return root, node
}

func boutClockLoad(t *testing.T, root, name string) boutClockPublic {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name))
	var p boutClockPublic
	if err != nil || len(b) > 2<<20 || json.Unmarshal(b, &p) != nil || p.Version != 1 || !p.TaggedClock || !p.NormalPoW || p.ClockMS <= 0 || p.Build.Record.Plan.BoutCount != 1 {
		t.Fatal("bounded genuine public card declaration", name, err)
	}
	return p
}

func boutClockSave(t *testing.T, root, name string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(root, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := f.Write(append(b, '\n'))
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
}

func newBoutClockWire(t *testing.T, ctx context.Context, c *StableController) *seedClockWire {
	t.Helper()
	in, writer := io.Pipe()
	reader, out := io.Pipe()
	w := &seedClockWire{reader: reader, in: writer, scan: bufio.NewScanner(reader), done: make(chan error, 1)}
	w.scan.Buffer(make([]byte, 4096), pilot.MaxBoutResponseSize)
	go func() {
		err := pilot.ServeBouts(ctx, c, in, out)
		_ = out.CloseWithError(err)
		_ = in.Close()
		w.done <- err
	}()
	t.Cleanup(func() { w.close(t) })
	return w
}

func boutClockOpen(t *testing.T, root, node string, clock int64) *cardClockHarness {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	t.Cleanup(cancel)
	h := &cardClockHarness{ctx: ctx, root: root, cfg: pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: node}}
	h.now.Store(clock)
	var err error
	h.c, err = NewBoutsForTestClock(ctx, h.cfg, func() time.Time { return time.UnixMilli(h.now.Load()) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if h.c != nil {
			if err := h.c.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	return h
}

// boutCoreSeed owns only this cold copy's established zero256 process pair and
// concrete source journals. It runs no ManifestWorker/admission service. This is
// source-authorized core preparation, not helper seed scheduling/epoch coverage.
type boutCoreSeed struct {
	ctx         context.Context
	binding     seednode.OwnerBinding
	source      *registry.ManifestStore
	seeds       *gymseed.Store
	events      *gymevent.Store
	entries     *gymentry.Store
	session     gymstable.Session
	rpc, peer   *seedClockRPC
	closers     []func() error
	closed      bool
	accepted    uint64
	accountID   string
	generation  uint64
	holdUntilMS int64
}

func (o *boutCoreSeed) close(t *testing.T) {
	t.Helper()
	if o.closed {
		return
	}
	o.closed = true
	for i := len(o.closers) - 1; i >= 0; i-- {
		if e := o.closers[i](); e != nil {
			t.Error("core fixture cleanup", e)
		}
	}
}
func (o *boutCoreSeed) assertAdmissions(t *testing.T) {
	t.Helper()
	i, e := o.source.Info()
	if e != nil || i.Records != o.accepted {
		t.Fatal("core-only phase changed actual admissions", e)
	}
}
func boutClockResume(t *testing.T, h *cardClockHarness) {
	t.Helper()
	var e error
	h.c, e = NewBoutsForTestClock(h.ctx, h.cfg, func() time.Time { return time.UnixMilli(h.now.Load()) })
	if e != nil {
		t.Fatal(e)
	}
	h.login(t, "zero256")
}
func boutCoreOpen(t *testing.T, h *cardClockHarness, p gymseed.Pin, stage string, accepted uint64) *boutCoreSeed {
	t.Helper()
	if h.c != nil {
		t.Fatal("normal parent must be fully closed before core-only preparation")
	}
	o := &boutCoreSeed{ctx: h.ctx, accepted: accepted}
	t.Cleanup(func() { o.close(t) })
	m, e := readBoutMarker(filepath.Join(h.cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json"))
	if e != nil {
		t.Fatal(e)
	}
	ep, e := seedEventAccountPin(cardBase(boutBase(m)))
	if e != nil || !reflect.DeepEqual(ep, p.SourcePin) {
		t.Fatal("exact established event pin", e)
	}
	var entryPin gymentry.Pin
	for _, v := range m.EntryInstalls {
		if v.AccountID == p.PublisherAccountID && v.Phase == "ready" {
			entryPin = v.Pin
		}
	}
	zero, e := gymaccount.FixtureIdentity("zero256")
	if e != nil || entryPin.StablePin.Identity != zero {
		t.Fatal("fixed established zero participant", e)
	}
	clock := func() time.Time { return time.UnixMilli(h.now.Load()) }
	accounts, e := gymaccount.OpenFixtureForTestClock(filepath.Join(h.cfg.DataDir, "authority"), storeConfig(), clock)
	if e != nil {
		t.Fatal(e)
	}
	o.closers = append(o.closers, accounts.Close)
	principal, e := eventFactoriesForTestClock(clock).verify(h.ctx, "zero256", false)
	if e != nil {
		t.Fatal(e)
	}
	login, e := accounts.Login(h.ctx, principal)
	if e != nil || login.Account.ID != p.PublisherAccountID || login.Account.Enrollment == nil || *login.Account.Enrollment != zero {
		t.Fatal("actual original zero account", e)
	}
	o.accountID, o.generation, o.holdUntilMS = login.Account.ID, login.Account.Generation, login.Account.HoldUntilMS
	o.closers = append(o.closers, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return accounts.Logout(ctx, login.Token)
	})
	var view [32]byte
	if _, e = rand.Read(view[:]); e != nil {
		t.Fatal(e)
	}
	o.session = gymstable.Session{ViewID: hex.EncodeToString(view[:]), Generation: login.Account.Generation, Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		return accounts.WithSession(ctx, login.Token, cap, func(a gymaccount.Account, check func() error) error {
			if a.ID != login.Account.ID || a.Generation != login.Account.Generation {
				return gymaccount.ErrUnauthorized
			}
			if e := check(); e != nil {
				return e
			}
			if e := fn(a, check); e != nil {
				return e
			}
			return check()
		})
	}}
	home := filepath.Join(h.cfg.DataDir, "gyms", p.PublisherAccountID)
	o.source, e = registry.OpenManifestAuthority(filepath.Join(home, "manifest-authority"), entryPin.StablePin.Source, true)
	if e != nil {
		t.Fatal(e)
	}
	o.closers = append(o.closers, o.source.Close)
	o.assertAdmissions(t)
	stable, e := gymstable.OpenForTestClock(filepath.Join(home, "stable"), entryPin.StablePin, o.source, clock)
	if e != nil {
		t.Fatal(e)
	}
	o.closers = append(o.closers, stable.Close)
	o.entries, e = gymentry.OpenForTestClock(h.ctx, filepath.Join(home, "entries"), entryPin, stable, clock)
	if e != nil {
		t.Fatal(e)
	}
	o.closers = append(o.closers, o.entries.Close)
	o.events, e = gymevent.OpenForTestClock(h.ctx, filepath.Join(h.cfg.DataDir, "events"), p.SourcePin, clock)
	if e != nil {
		t.Fatal(e)
	}
	o.closers = append(o.closers, o.events.Close)
	o.seeds, e = gymseed.OpenForTestClock(h.ctx, filepath.Join(h.cfg.DataDir, "seeds"), p, o.events, clock)
	if e != nil {
		t.Fatal(e)
	}
	o.closers = append(o.closers, o.seeds.Close)
	// Same exact config topology as pilot.openNodes; never read wallet/key files.
	// The cold copy's old absolute config paths must not point back to Set46.
	ports := make([]int, 4)
	seen := map[int]bool{}
	for i := range ports {
		for {
			n := manifestTestPort(t)
			if !seen[n] {
				ports[i] = n
				seen[n] = true
				break
			}
		}
	}
	start := func(name string, rpcPort, connectPort, listenPort int) seednode.OwnedProcess {
		path := filepath.Join(home, name+".conf")
		old, e := os.ReadFile(path)
		if e != nil || len(old) > 8192 {
			t.Fatal("bounded copied node config", e)
		}
		f, e := os.OpenFile(filepath.Join(h.root, "private-"+stage+"-"+name+"-config.before"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			t.Fatal(e)
		}
		_, we := f.Write(old)
		ce := f.Close()
		if we != nil || ce != nil {
			t.Fatal(we, ce)
		}
		for _, d := range []string{name + "-chain", name + "-logs"} {
			resolved, e := filepath.EvalSymlinks(filepath.Join(home, d))
			if e != nil || resolved != filepath.Join(home, d) {
				t.Fatal("established real node directory", e)
			}
		}
		var secret [32]byte
		if _, e = rand.Read(secret[:]); e != nil {
			t.Fatal(e)
		}
		config := fmt.Sprintf("rpcuser=local-pilot\nrpcpass=%s\nnotls=1\nrpclisten=127.0.0.1:%d\ndatadir=%s\nlogdir=%s\nnodnsseed=1\nlocalwalletpilot=1\nconnect=127.0.0.1:%d\n", hex.EncodeToString(secret[:]), rpcPort, filepath.Join(home, name+"-chain"), filepath.Join(home, name+"-logs"), connectPort)
		if listenPort != 0 {
			config += fmt.Sprintf("listen=127.0.0.1:%d\n", listenPort)
		} else {
			config += "nolisten=1\n"
		}
		if e = os.WriteFile(path, []byte(config), 0600); e != nil {
			t.Fatal(e)
		}
		log, e := os.OpenFile(filepath.Join(h.root, stage+"-"+name+"-node.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			t.Fatal(e)
		}
		rpc := seedClockOwnedNamedRPC(t, home, name)
		cmd := exec.Command(h.cfg.NodeBinary, "--configfile="+path)
		cmd.Stdin = nil
		cmd.Stdout = log
		cmd.Stderr = log
		if e = cmd.Start(); e != nil {
			_ = log.Close()
			t.Fatal(e)
		}
		done := make(chan struct{})
		var waitErr error
		go func() { waitErr = errors.Join(cmd.Wait(), log.Close()); close(done) }()
		o.closers = append(o.closers, func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			var out any
			_ = rpc.call(ctx, "stop", nil, &out)
			cancel()
			select {
			case <-done:
				return waitErr
			case <-time.After(12 * time.Second):
				_ = cmd.Process.Kill()
			}
			select {
			case <-done:
				return errors.New("owned node required forced shutdown")
			case <-time.After(15 * time.Second):
				return errors.New("owned node did not join after kill")
			}
		})
		if name == "primary" {
			o.rpc = rpc
		} else {
			o.peer = rpc
		}
		return seednode.OwnedProcess{Cmd: cmd, Done: done}
	}
	peer := start("peer", ports[0], ports[3], ports[1])
	primary := start("primary", ports[2], ports[1], 0)
	o.binding = seednode.OwnerBinding{Source: p.SourcePin, Home: home, Primary: primary, Peer: peer}
	ready, cancel := context.WithTimeout(h.ctx, 30*time.Second)
	defer cancel()
	seedClockWait(t, ready, func() bool {
		publisher, e := seednode.AttachOwned(ready, o.binding)
		if e != nil {
			return false
		}
		if e = publisher.Close(); e != nil {
			t.Fatal(e)
		}
		return true
	})
	return o
}
func (o *boutCoreSeed) action(t *testing.T, kind, date string) gymseed.Record {
	t.Helper()
	started := time.Now()
	defer func() { t.Logf("core-only action %s elapsed=%s", kind, time.Since(started)) }()
	o.assertAdmissions(t)
	ctx, cancel := context.WithTimeout(o.ctx, 30*time.Second)
	defer cancel()
	p, e := seednode.AttachOwned(ctx, o.binding)
	if e != nil {
		t.Fatal("owned publisher attachment", e)
	}
	var r gymseed.Record
	switch kind {
	case "reconcile":
		r, e = o.seeds.Reconcile(ctx, o.events, o.entries, o.session, p, date)
	case "publish":
		r, e = o.seeds.Publish(ctx, o.events, o.entries, o.session, p, date)
	case "attest":
		r, e = o.seeds.Attest(ctx, o.events, o.entries, o.session, p, date)
	default:
		e = errors.New("unexpected core-only action")
	}
	ce := p.Close()
	if e != nil || ce != nil {
		t.Fatal("core-only "+kind+"; no automatic retry", e, ce)
	}
	o.assertAdmissions(t)
	return r
}

func boutClockAdvance(t *testing.T, h *cardClockHarness, o *boutCoreSeed, target uint32) {
	t.Helper()
	if target >= 256 {
		t.Fatal("exported normal chain must remain within256 headers")
	}
	for n := 0; n < 256; n++ {
		o.assertAdmissions(t)
		var height uint32
		if e := o.rpc.call(h.ctx, "getblockcount", nil, &height); e != nil {
			t.Fatal(e)
		}
		if height >= target {
			return
		}
		seedClockMine(t, h.ctx, o.rpc, bytes.Repeat([]byte{0x45}, 20))
	}
	t.Fatal("bounded later-event chain advance exhausted")
}

func TestBoutClockPrepareLaterCard(t *testing.T) {
	root, node := boutClockRoot(t)
	old := boutClockLoad(t, root, "public-card-result.json")
	if old.Read.Status.Usable || !reflect.DeepEqual(old.Read.Record, old.Build.Record) {
		t.Fatal("original card must remain historical and conflicted")
	}
	h := boutClockOpen(t, root, node, old.ClockMS)
	h.login(t, "zero256")
	w := newBoutClockWire(t, h.ctx, h.c)
	v := h.c.Snapshot().View.ViewID
	prior := w.cardRead(t, h.ctx, pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: old.Saturday})
	if !prior.OK || prior.Record.Status.Usable || !reflect.DeepEqual(prior.Record.Record, old.Build.Record) {
		t.Fatal("copied original evidence", prior.Error)
	}
	h.logout(t)
	// The old event's calendar deadline is exactly the next Saturday noon.
	h.cutoff = old.Build.Record.Source.SourceReceipt.Input.Event.IncidentDeadlineMS
	h.now.Store(h.cutoff - 60000)
	for _, fixture := range []string{"zero256", "incrementing256"} {
		h.login(t, fixture)
		v = h.c.Snapshot().View.ViewID
		u, err := h.c.EntryUpcoming(h.ctx, v)
		if err != nil || u.Event.CutoffMS != h.cutoff || u.Event.Saturday == old.Saturday {
			t.Fatal("later Saturday", err)
		}
		h.date = u.Event.Saturday
		ids := []string{}
		for _, participant := range old.Build.Record.Source.SourceReceipt.Input.Participants {
			if participant.Owner.AccountID == h.c.Snapshot().View.AccountID {
				for _, m := range participant.Members {
					ids = append(ids, m.Assignment.ProspectID)
				}
			}
		}
		if len(ids) == 0 {
			t.Fatal("no retained genuine entrants")
		}
		sort.Strings(ids)
		st := h.c.StableSnapshot()
		j := h.entryJob(t, func() (pilot.EntryJob, error) {
			return h.c.BeginEntryPrepare(h.ctx, v, gymentry.Request{Version: 1, EventID: u.Event.EventID, Saturday: h.date, Action: "replace", ExpectedRosterRevision: st.Status.RosterRevision, ProspectIDs: ids})
		})
		h.entryJob(t, func() (pilot.EntryJob, error) { return h.c.BeginEntryApply(h.ctx, v, j.OperationID) })
		h.logout(t)
	}
	h.now.Store(h.cutoff + 1000)
	for _, fixture := range []string{"zero256", "incrementing256"} {
		h.login(t, fixture)
		v = h.c.Snapshot().View.ViewID
		j, err := h.c.BeginEventCapture(h.ctx, v, h.date)
		if err != nil {
			t.Fatal(err)
		}
		seedClockWait(t, h.ctx, func() bool {
			j, err = h.c.EventJob(h.ctx, v, j.JobID)
			if err != nil {
				t.Fatal(err)
			}
			return j.State != "queued" && j.State != "running"
		})
		if j.State != "completed" {
			t.Fatal("later snapshot", j.Error)
		}
		h.logout(t)
	}
	h.login(t, "zero256")
	v = h.c.Snapshot().View.ViewID
	prepared := w.job(t, h.ctx, v, h.date, "seed_prepare")
	if prepared.State != "completed" || prepared.Result.Publication != nil {
		t.Fatal("fresh later publication intent", prepared.Error)
	}
	acceptedBefore := h.c.Snapshot().AcceptedCount
	seedPin := *h.c.SeedSnapshot().Pin
	w.close(t)
	if err := h.c.Close(); err != nil {
		t.Fatal(err)
	}
	h.c = nil
	o := boutCoreOpen(t, h, seedPin, "prepare", acceptedBefore)
	// Funding scans run against a stationary owned chain. Do not call Publish
	// until its full retained funding prefix equals the actual current tip.
	complete := false
	for n := 0; n < 120; n++ {
		_ = o.action(t, "reconcile", h.date)
		info, e := o.seeds.Info(h.ctx)
		if e != nil {
			t.Fatal(e)
		}
		var tip uint32
		var hash string
		if e = o.rpc.call(h.ctx, "getblockcount", nil, &tip); e != nil {
			t.Fatal(e)
		}
		if e = o.rpc.call(h.ctx, "getblockhash", []any{tip}, &hash); e != nil {
			t.Fatal(e)
		}
		t.Logf("core-only funding height=%d complete=%t tip=%d", info.FundingHeight, info.FundingComplete, tip)
		if info.FundingComplete && info.FundingHeight == int32(tip) && info.FundingHash == hash {
			complete = true
			break
		}
	}
	if !complete {
		t.Fatal("bounded stationary funding scan exhausted")
	}
	publication := o.action(t, "publish", h.date)
	if publication.Publication == nil {
		t.Fatal("missing definite publication")
	}
	var tip uint32
	if err := o.rpc.call(h.ctx, "getblockcount", nil, &tip); err != nil {
		t.Fatal(err)
	}
	boutClockAdvance(t, h, o, tip+3)
	var observed gymseed.Record
	for n := 0; n < 120; n++ {
		observed = o.action(t, "reconcile", h.date)
		if observed.Target != nil {
			if observed.Observation.TargetDescendants >= 60 {
				break
			}
			boutClockAdvance(t, h, o, observed.Target.Height+60)
		}
	}
	if observed.Target == nil || observed.Observation.TargetDescendants < 60 {
		t.Fatal("bounded fixed target/depth wait")
	}
	attest := o.action(t, "attest", h.date)
	if attest.Finality == nil || attest.Publication == nil || attest.Publication.RawTx != publication.Publication.RawTx {
		t.Fatal("later finality")
	}
	seedClockExportChain(t, h.ctx, o.rpc, root, attest.Finality.ObservedTip)
	for name, hash := range map[string]string{"later-anchor": attest.Anchor.Hash, "later-target": attest.Target.Hash} {
		var body string
		if err := o.rpc.call(h.ctx, "getblock", []any{hash, 0}, &body); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "public-"+name+"-block.hex"), []byte(body+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	o.assertAdmissions(t)
	o.close(t)
	if t.Failed() {
		t.Fatal("core-only owner did not cleanly close before parent reopen")
	}
	boutClockResume(t, h)
	v = h.c.Snapshot().View.ViewID
	if h.c.Snapshot().AcceptedCount != acceptedBefore {
		t.Fatal("core preparation admitted a candidate")
	}
	w = newBoutClockWire(t, h.ctx, h.c)
	built := w.cardJob(t, h.ctx, v, h.date)
	if built.State != "completed" || built.Result == nil || built.Result.Replayed || !built.Result.Status.Usable {
		t.Fatal("later usable card", built.Error)
	}
	plan := built.Result.Record.Plan
	if plan.BoutCount != 1 || plan.Counts.TotalCount != old.Build.Record.Plan.Counts.TotalCount || plan.Counts.PairedCount != 2 || plan.Counts.TotalCount != plan.Counts.PairedCount+plan.Counts.UnmatchedCount {
		t.Fatal("later C1 accounting", plan.Counts)
	}
	// Retained assignments and birth stay exact; only the new cutoff projection changes.
	for _, p := range built.Result.Record.Source.SourceReceipt.Input.Participants {
		for _, m := range p.Members {
			found := false
			for _, op := range old.Build.Record.Source.SourceReceipt.Input.Participants {
				for _, om := range op.Members {
					if om.Assignment.ProspectID == m.Assignment.ProspectID {
						found = reflect.DeepEqual(om.Assignment, m.Assignment)
					}
				}
			}
			if !found {
				t.Fatal("original assignment/birth changed")
			}
		}
	}
	prior = w.cardRead(t, h.ctx, pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: old.Saturday})
	if !prior.OK || prior.Record.Status.Usable || !reflect.DeepEqual(prior.Record.Record, old.Build.Record) {
		t.Fatal("new event changed original conflict/evidence", prior.Error)
	}
	read := w.cardRead(t, h.ctx, pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: h.date})
	if !read.OK || !read.Record.Status.Usable {
		t.Fatal("later saved card", read.Error)
	}
	seed, err := json.Marshal(attest)
	if err != nil {
		t.Fatal(err)
	}
	boutClockSave(t, root, "public-bout-prepared-card.json", boutClockPublic{1, true, true, h.now.Load(), h.date, h.c.CardSnapshot(), *built.Result, *read.Record, seed})
	if h.c.Snapshot().AcceptedCount != acceptedBefore || h.c.Snapshot().AcceptedCount > acceptedBefore+28 {
		t.Fatal("additional admission cap exceeded")
	}
	boutClockSave(t, root, "public-bout-preparation-counts.json", map[string]any{"preparation_mode": "authenticated_core_seed_with_owned_normal_nodes_no_admission_worker", "automatic_admissions": 0, "core_account_id": o.accountID, "core_account_generation": o.generation, "core_account_hold_until_ms": o.holdUntilMS, "accepted_before": acceptedBefore, "accepted_after": h.c.Snapshot().AcceptedCount, "maximum_additional_per_gym": 32, "maximum_preparation_additional": 28, "card_counts": plan.Counts, "entrants": plan.Entrants})
	// The later conflict-stage harness has its own login observations. Preserve
	// this stage's current View evidence before that reusable log is replaced.
	boutClockSave(t, root, "public-bout-prepared-views.json", h.views)
	t.Logf("later genuine card entered=%d paired=%d unmatched=%d bouts=%d", plan.Counts.TotalCount, plan.Counts.PairedCount, plan.Counts.UnmatchedCount, plan.BoutCount)
	w.close(t)
	h.c = nil
}

func TestBoutClockObserveNewConflict(t *testing.T) {
	root, node := boutClockRoot(t)
	prepared := boutClockLoad(t, root, "public-bout-prepared-card.json")
	old := boutClockLoad(t, root, "public-card-result.json")
	var first struct {
		Record gymbout.Record `json:"record"`
	}
	b, err := os.ReadFile(filepath.Join(root, "public-bout-native-first.json"))
	if err != nil || len(b) > 2<<20 || json.Unmarshal(b, &first) != nil || first.Record.BoutID != prepared.Build.Record.Plan.Bouts[0].BoutID {
		t.Fatal("actual native first Run evidence required", err)
	}
	h := boutClockOpen(t, root, node, prepared.ClockMS+1)
	h.login(t, "zero256")
	v := h.c.Snapshot().View.ViewID
	read, err := h.c.BoutRecord(h.ctx, v, prepared.Saturday, first.Record.BoutID)
	if err != nil || !reflect.DeepEqual(read.Record, first.Record) || !read.Status.Saved.Usable {
		t.Fatal("saved first Run before conflict", err)
	}
	before := h.c.Snapshot().AcceptedCount
	seedPin := *h.c.SeedSnapshot().Pin
	if err := h.c.Close(); err != nil {
		t.Fatal(err)
	}
	h.c = nil
	o := boutCoreOpen(t, h, seedPin, "conflict", before)
	for _, rpc := range []*seedClockRPC{o.rpc, o.peer} {
		var out any
		if err := rpc.call(h.ctx, "invalidateblock", []any{first.Record.Source.TargetHash}, &out); err != nil {
			t.Fatal("owned target invalidation", err)
		}
	}
	conflict := o.action(t, "reconcile", prepared.Saturday)
	if len(conflict.Conflicts) == 0 {
		t.Fatal("new retained conflict")
	}
	o.assertAdmissions(t)
	o.close(t)
	if t.Failed() {
		t.Fatal("core-only owner did not cleanly close before parent reopen")
	}
	boutClockResume(t, h)
	v = h.c.Snapshot().View.ViewID
	w := newBoutClockWire(t, h.ctx, h.c)
	read, err = h.c.BoutRecord(h.ctx, v, prepared.Saturday, first.Record.BoutID)
	if err != nil || read.Status.Saved.Usable || read.Status.Source.ConflictCount == 0 || !reflect.DeepEqual(read.Record, first.Record) {
		t.Fatal("conflict rewrote or hid saved transcript", err)
	}
	prior := w.cardRead(t, h.ctx, pilot.CardRequest{Command: "card_record", ViewID: v, Saturday: old.Saturday})
	if !prior.OK || prior.Record.Status.Usable || !reflect.DeepEqual(prior.Record.Record, old.Build.Record) {
		t.Fatal("original conflict changed", prior.Error)
	}
	after := h.c.Snapshot().AcceptedCount
	if after != before {
		t.Fatal("conflict-only phase unexpectedly mined additional admissions")
	}
	boutClockSave(t, root, "public-bout-conflict.json", map[string]any{"status": "passed", "clock_ms": h.now.Load(), "normal_pow": true, "tagged_clock": true, "seed": conflict, "read": read, "original_card": prior.Record, "preparation_mode": "authenticated_core_seed_with_owned_normal_nodes_no_admission_worker", "automatic_admissions": 0, "core_account_id": o.accountID, "core_account_generation": o.generation, "core_account_hold_until_ms": o.holdUntilMS, "accepted_before": before, "accepted_after": after})
	w.close(t)
	h.c = nil
}
