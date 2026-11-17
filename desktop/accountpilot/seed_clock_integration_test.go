//go:build desktopintegration && slugtestclock && !slugtestpow

package accountpilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func seedClockWait(t *testing.T, ctx context.Context, f func() bool) {
	t.Helper()
	for !f() {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(30 * time.Millisecond):
		}
	}
}
func seedClockSetup(t *testing.T, root, node string) (*StableController, context.Context, string, int64, *atomic.Int64, pilot.Config) {
	fail := func(args ...any) {
		_ = os.WriteFile(filepath.Join(root, "serve-error.txt"), []byte(fmt.Sprint(args...)+"\n"+string(debug.Stack())), 0600)
		t.Fatal(args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	t.Cleanup(cancel)
	var n atomic.Int64
	n.Store(time.Now().UnixMilli())
	clock := func() time.Time { return time.UnixMilli(n.Load()) }
	cfg := pilot.Config{DataDir: filepath.Join(root, "data"), NodeBinary: node}
	c, e := NewSeedsForTestClock(cfg, clock)
	if e != nil {
		fail(e)
	}
	t.Cleanup(func() {
		if c != nil {
			_ = c.Close()
		}
	})
	login := func(f string) {
		t.Helper()
		if e := c.LoginFixture(f); e != nil {
			fail(e)
		}
		seedClockWait(t, ctx, func() bool {
			s := c.Snapshot()
			if s.Account.State == "failed" {
				fail("login", s.Account.LastError)
			}
			return s.Account.State == "signed_in" && s.View != nil
		})
	}
	logout := func() {
		t.Helper()
		if e := c.Logout(); e != nil {
			fail(e)
		}
		seedClockWait(t, ctx, func() bool { s := c.Snapshot(); return s.Account.State == "signed_out" })
	}
	login("zero256")
	v := c.StableSnapshot().View.ViewID
	up, e := c.EntryUpcoming(ctx, v)
	if e != nil {
		fail(e)
	}
	date, C := up.Event.Saturday, up.Event.CutoffMS
	if e = c.Close(); e != nil {
		fail(e)
	}
	c = nil
	n.Store(C - 60000)
	c, e = NewSeedsForTestClock(cfg, clock)
	if e != nil {
		fail(e)
	}
	stableDone := func(j pilot.StableJob, e error) pilot.StableJob {
		t.Helper()
		if e != nil {
			fail(e)
		}
		seedClockWait(t, ctx, func() bool {
			var e error
			j, e = c.StableJob(ctx, j.View.ViewID, j.JobID)
			if e != nil {
				fail(e)
			}
			return j.State != "queued" && j.State != "running"
		})
		if j.State != "done" {
			fail("stable", j.Error)
		}
		return j
	}
	entryDone := func(j pilot.EntryJob, e error) pilot.EntryJob {
		t.Helper()
		if e != nil {
			fail(e)
		}
		seedClockWait(t, ctx, func() bool {
			var e error
			j, e = c.EntryJob(ctx, j.View.ViewID, j.JobID)
			if e != nil {
				fail(e)
			}
			return j.State != "queued" && j.State != "running"
		})
		if j.State != "complete" {
			fail("entry", j.Error)
		}
		return j
	}
	ids := map[string]string{}
	observed := map[string]pilot.ManifestSnapshot{}
	for _, fixture := range []string{"zero256", "incrementing256"} {
		login(fixture)
		observed[fixture] = c.Snapshot()
		progress, _ := json.Marshal(observed)
		if e = os.WriteFile(filepath.Join(root, "observed-fixtures.json"), progress, 0600); e != nil {
			fail(e)
		}
		if e = c.Start(); e != nil {
			fail(e)
		}
		seedClockWait(t, ctx, func() bool {
			s := c.Snapshot()
			if s.State == "failed" {
				fail("mining", s.LastError)
			}
			return s.AcceptedCount >= 1
		})
		c.Stop()
		v = c.StableSnapshot().View.ViewID
		stableDone(c.BeginStableSync(ctx, v))
		inv := stableDone(c.BeginStableInventory(ctx, v, 0, 1))
		if inv.Page == nil || len(inv.Page.Items) != 1 {
			fail("missing genuine record")
		}
		id := inv.Page.Items[0].Assignment.ProspectID
		ids[fixture] = id
		st := c.StableSnapshot()
		if st.Status == nil {
			fail("status")
		}
		p := stableDone(c.BeginStablePrepare(ctx, v, gymstable.SlotRequest{ExpectedRevision: st.Status.RosterRevision, Slot: 1, ProspectID: id}))
		a := stableDone(c.BeginStableApply(ctx, v, p.Intent.OperationID))
		u, e := c.EntryUpcoming(ctx, v)
		if e != nil || u.Event.Saturday != date {
			fail("cutoff changed", e)
		}
		ep := entryDone(c.BeginEntryPrepare(ctx, v, gymentry.Request{Version: 1, EventID: u.Event.EventID, Saturday: date, Action: "replace", ExpectedRosterRevision: a.Mutation.Receipt.Revision, ProspectIDs: []string{id}}))
		entryDone(c.BeginEntryApply(ctx, v, ep.OperationID))
		logout()
	}
	n.Store(C + 1000)
	login("zero256")
	return c, ctx, date, C, &n, cfg
}

type seedClockRPC struct {
	config prospectnode.RPCConfig
	client *http.Client
}

func seedClockOwnedRPC(t *testing.T, home string) *seedClockRPC {
	return seedClockOwnedNamedRPC(t, home, "primary")
}
func seedClockOwnedNamedRPC(t *testing.T, home, name string) *seedClockRPC {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(home, name+".conf"))
	if e != nil {
		t.Fatal(e)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok {
			values[k] = v
		}
	}
	if !strings.HasPrefix(values["rpclisten"], "127.0.0.1:") || values["rpcuser"] == "" || values["rpcpass"] == "" {
		t.Fatal("not owned loopback config")
	}
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	t.Cleanup(tr.CloseIdleConnections)
	return &seedClockRPC{prospectnode.RPCConfig{Version: 1, URL: "http://" + values["rpclisten"], Username: values["rpcuser"], Password: values["rpcpass"]}, &http.Client{Transport: tr, Timeout: 15 * time.Second}}
}
func (r *seedClockRPC) call(ctx context.Context, method string, params []any, out any) error {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": 1, "method": method, "params": params})
	q, e := http.NewRequestWithContext(ctx, http.MethodPost, r.config.URL, bytes.NewReader(body))
	if e != nil {
		return e
	}
	q.SetBasicAuth(r.config.Username, r.config.Password)
	q.Header.Set("Content-Type", "application/json")
	res, e := r.client.Do(q)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if e != nil {
		return e
	}
	var v struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(b, &v) != nil || res.StatusCode != 200 {
		return fmt.Errorf("owned RPC envelope")
	}
	if string(v.Error) != "null" && len(v.Error) != 0 {
		return fmt.Errorf("owned RPC %s refused", method)
	}
	return json.Unmarshal(v.Result, out)
}

// Explicit test-only funding on the actual retained zero256 chain. The
// application never adds a mining address or generates an operator-fund block.
func seedClockMine(t *testing.T, ctx context.Context, rpc *seedClockRPC, payoutHash []byte) (uint32, string) {
	t.Helper()
	client, e := prospectnode.NewClient(rpc.config)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	addr, e := address.NewAddressWitnessPubKeyHash(payoutHash, &chaincfg.MainNetParams)
	if e != nil {
		t.Fatal(e)
	}
	policy := prospectnode.Policy{Version: 1, Network: uint32(chaincfg.MainNetParams.Net), Genesis: chaincfg.MainNetParams.GenesisHash.String(), DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}
	for attempt := 0; attempt < 64; attempt++ {
		template, e := client.ResolveTemplate(ctx, policy)
		if e != nil {
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
			time.Sleep(30 * time.Millisecond)
			continue
		}
		job, e := prospectjob.New(template.Context, "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798", strings.Repeat("45", 32), time.Now().UnixMilli())
		if e != nil {
			t.Fatal(e)
		}
		candidate, e := prospectwork.Build(job, template, addr.EncodeAddress(), uint64(attempt))
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := hex.DecodeString(candidate.BlockHex)
		var block wire.MsgBlock
		if block.Deserialize(bytes.NewReader(raw)) != nil {
			t.Fatal("candidate decode")
		}
		target := blockchain.CompactToBig(block.Header.Bits)
		won := false
		var hash string
		for nonce := uint32(0); nonce < 64; nonce++ {
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
			block.Header.Nonce = nonce
			work := block.Header.BlockHash()
			if blockchain.HashToBig(&work).Cmp(target) <= 0 {
				won = true
				hash = work.String()
				break
			}
		}
		if !won {
			continue
		}
		var encoded bytes.Buffer
		if block.Serialize(&encoded) != nil {
			t.Fatal("encode")
		}
		if client.SubmitBlock(ctx, template.Context, hex.EncodeToString(encoded.Bytes())) != nil {
			continue
		}
		height := template.Context.ParentHeight + 1
		var canonical string
		if rpc.call(ctx, "getblockhash", []any{height}, &canonical) == nil && canonical == hash {
			t.Logf("explicit normal-PoW fixture block height=%d hash=%s", height, hash)
			return height, hash
		}
	}
	t.Fatal("bounded explicit funding retries exhausted")
	return 0, ""
}

// Advance only by explicit fixture blocks paying an unrelated public sink.
// No further key1 outputs are created and no gym wallet output is spent.
func seedClockAdvance(t *testing.T, ctx context.Context, rpc *seedClockRPC, target uint32) {
	t.Helper()
	for i := 0; i < 256; i++ {
		var h uint32
		if e := rpc.call(ctx, "getblockcount", nil, &h); e != nil {
			t.Fatal(e)
		}
		if h >= target {
			return
		}
		seedClockMine(t, ctx, rpc, bytes.Repeat([]byte{0x45}, 20))
	}
	t.Fatal("bounded fixture advance exhausted")
}

type seedClockWire struct {
	reader *io.PipeReader
	closed bool
	in     *io.PipeWriter
	scan   *bufio.Scanner
	done   chan error
	next   uint64
}

func newSeedClockWire(t *testing.T, ctx context.Context, c *StableController) *seedClockWire {
	t.Helper()
	in, writer := io.Pipe()
	reader, out := io.Pipe()
	w := &seedClockWire{reader: reader, in: writer, scan: bufio.NewScanner(reader), done: make(chan error, 1)}
	w.scan.Buffer(make([]byte, 4096), pilot.MaxSeedResponseSize)
	go func() { e := pilot.ServeSeeds(ctx, c, in, out); _ = out.CloseWithError(e); _ = in.Close(); w.done <- e }()
	t.Cleanup(func() { w.close(t) })
	return w
}
func (w *seedClockWire) close(t *testing.T) {
	t.Helper()
	if w.closed {
		return
	}
	w.closed = true
	_ = w.in.Close()
	_ = w.reader.Close()
	select {
	case e := <-w.done:
		if e != nil && e != context.Canceled && e != io.ErrClosedPipe {
			t.Error(e)
		}
	case <-time.After(10 * time.Second):
		t.Error("ServeSeeds did not join")
	}
}
func (w *seedClockWire) call(t *testing.T, r pilot.SeedRequest) pilot.SeedResponse {
	t.Helper()
	w.next++
	r.ID = w.next
	b, e := pilot.MarshalSeedRequest(r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.in.Write(append(b, '\n')); e != nil {
		t.Fatal(e)
	}
	if !w.scan.Scan() {
		t.Fatal("helper frame", w.scan.Err())
	}
	v, e := pilot.ParseSeedResponse(w.scan.Bytes())
	if e != nil || v.ID != r.ID {
		t.Fatalf("helper frame parse id=%d: %v %s", r.ID, e, w.scan.Text())
	}
	return v
}
func (w *seedClockWire) job(t *testing.T, ctx context.Context, view, date, command string) pilot.SeedJob {
	t.Helper()
	var r pilot.SeedResponse
	seedClockWait(t, ctx, func() bool {
		r = w.call(t, pilot.SeedRequest{Command: command, ViewID: view, Saturday: date})
		return r.Error != "busy"
	})
	if !r.OK || r.Job == nil {
		t.Fatalf("begin %s: %s", command, r.Error)
	}
	j := *r.Job
	started, lastState := time.Now(), j.State
	t.Logf("seed task %s %s state=%s at=%s", command, j.JobID, j.State, started.UTC().Format(time.RFC3339Nano))
	seedClockWait(t, ctx, func() bool {
		r = w.call(t, pilot.SeedRequest{Command: "seed_job", ViewID: view, JobID: j.JobID})
		if r.Error == "busy" {
			return false
		}
		if !r.OK || r.Job == nil {
			t.Fatalf("poll %s: %s", command, r.Error)
		}
		j = *r.Job
		if j.State != lastState {
			t.Logf("seed task %s %s state=%s elapsed=%s error=%s request_context=%v", command, j.JobID, j.State, time.Since(started), j.Error, ctx.Err())
			lastState = j.State
		}
		return j.State != "queued" && j.State != "running"
	})
	return j
}

func TestSeedClockActualHelperPublication(t *testing.T) {
	node := os.Getenv("SLUG_STABLE_NODE")
	if node == "" {
		t.Skip("explicit frozen normal node")
	}
	if !filepath.IsAbs(node) {
		t.Fatal("absolute normal node")
	}
	base, e := os.MkdirTemp(os.Getenv("SLUG_ACCOUNT_TEST_ARTIFACTS"), "seed-testclock-")
	if e != nil {
		t.Fatal(e)
	}
	root, e := filepath.EvalSymlinks(base)
	if e != nil {
		t.Fatal(e)
	}
	t.Log("owned evidence", root)
	c, ctx, date, C, now, cfg := seedClockSetup(t, root, node)
	defer func() {
		if c != nil {
			_ = c.Close()
		}
	}()
	login := func(f string) {
		t.Helper()
		if e := c.LoginFixture(f); e != nil {
			t.Fatal(e)
		}
		seedClockWait(t, ctx, func() bool {
			s := c.Snapshot()
			if s.Account.State == "failed" {
				t.Fatal("login", s.Account.LastError)
			}
			return s.Account.State == "signed_in" && s.View != nil
		})
	}
	logout := func() {
		t.Helper()
		if e := c.Logout(); e != nil {
			t.Fatal(e)
		}
		seedClockWait(t, ctx, func() bool { return c.Snapshot().Account.State == "signed_out" })
	}
	if c.SeedSnapshot().CreatedAtMS >= C {
		t.Fatal("seed journal not initialized before cutoff")
	}
	for _, f := range []string{"zero256", "incrementing256"} {
		if c.Snapshot().Account.State == "signed_in" {
			logout()
		}
		login(f)
		v := c.EventSnapshot().View.ViewID
		j, e := c.BeginEventCapture(ctx, v, date)
		if e != nil {
			t.Fatal(e)
		}
		seedClockWait(t, ctx, func() bool {
			j, e = c.EventJob(ctx, v, j.JobID)
			if e != nil {
				t.Fatal(e)
			}
			return j.State != "queued" && j.State != "running"
		})
		if j.State != "completed" {
			t.Fatal("capture", j.Error)
		}
		logout()
	}
	login("zero256")
	w := newSeedClockWire(t, ctx, c)
	v := c.SeedSnapshot().View.ViewID
	snap := w.call(t, pilot.SeedRequest{Command: "seed_snapshot"})
	if !snap.OK || snap.Snapshot.State != "ready" || snap.Snapshot.NodeState != "offline" {
		t.Fatal("snapshot", snap.Error)
	}
	prepared := w.job(t, ctx, v, date, "seed_prepare")
	if prepared.State != "completed" || prepared.Result.Publication != nil {
		t.Fatal("prepare", prepared.Error)
	}
	immutable, _ := json.Marshal(prepared.Result.Declaration)
	offline := w.job(t, ctx, v, date, "seed_publish")
	if offline.Error != "seed_node_offline" {
		t.Fatal("offline", offline.Error)
	}
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	seedClockWait(t, ctx, func() bool {
		s := c.Snapshot()
		if s.State == "failed" {
			t.Fatal("start", s.LastError)
		}
		return c.SeedSnapshot().NodeState == "ready"
	})
	home := filepath.Join(cfg.DataDir, "gyms", c.Snapshot().View.AccountID)
	rpc := seedClockOwnedRPC(t, home)
	fundKeyHash, _ := hex.DecodeString("751e76e8199196d454941c45d1b3a323f1433bd6")
	fundHeight, fundHash := seedClockMine(t, ctx, rpc, fundKeyHash)
	seedClockAdvance(t, ctx, rpc, fundHeight+100)
	seedClockWait(t, ctx, func() bool {
		var h uint32
		if e := rpc.call(ctx, "getblockcount", nil, &h); e != nil {
			t.Fatal(e)
		}
		s := c.Snapshot()
		if s.State == "failed" {
			t.Fatal("mining", s.LastError)
		}
		return h >= fundHeight+100
	})
	var published pilot.SeedJob
	for i := 0; i < 80; i++ {
		j := w.job(t, ctx, v, date, "seed_reconcile")
		if j.State != "completed" {
			t.Fatal("funding reconcile", j.Error)
		}
		published = w.job(t, ctx, v, date, "seed_publish")
		if published.State == "completed" && published.Result.Publication != nil {
			break
		}
		if published.Error != "seed_scan_incomplete" && published.Error != "seed_waiting_for_funds" {
			t.Fatal("publish", published.Error)
		}
	}
	if published.State != "completed" || published.Result.Publication == nil {
		t.Fatal("bounded funding scan exhausted")
	}
	rawTx, txid := published.Result.Publication.RawTx, published.Result.Publication.TxID
	t.Log("durable fixed publication", txid)
	var dispatchedTip uint32
	if e = rpc.call(ctx, "getblockcount", nil, &dispatchedTip); e != nil {
		t.Fatal(e)
	}
	seedClockAdvance(t, ctx, rpc, dispatchedTip+3)
	seedClockFinish(t, c, w, ctx, root, cfg, now, date, v, *snap.Snapshot, immutable, rawTx, txid, fundHash)
}

// The shared finality/reorg lifecycle also resumes an explicitly preserved
// owned journal. It never calls Publish or rebuilds/reselects a transaction.
func seedClockFinish(t *testing.T, c *StableController, w *seedClockWire, ctx context.Context, root string, cfg pilot.Config, now *atomic.Int64, date, v string, snapshot pilot.SeedSnapshot, immutable []byte, rawTx, txid, fundHash string) {
	t.Helper()
	var e error
	defer func() {
		if c != nil {
			_ = c.Close()
		}
	}()
	login := func(f string) {
		t.Helper()
		if e := c.LoginFixture(f); e != nil {
			t.Fatal(e)
		}
		seedClockWait(t, ctx, func() bool {
			x := c.Snapshot()
			if x.Account.State == "failed" {
				t.Fatal("login", x.Account.LastError)
			}
			return x.Account.State == "signed_in" && x.View != nil
		})
	}
	logout := func() {
		t.Helper()
		if e := c.Logout(); e != nil {
			t.Fatal(e)
		}
		seedClockWait(t, ctx, func() bool { return c.Snapshot().Account.State == "signed_out" })
	}
	home := filepath.Join(cfg.DataDir, "gyms", c.Snapshot().View.AccountID)
	rpc := seedClockOwnedRPC(t, home)
	var final pilot.SeedJob
	cancelledReads := 0
	for i := 0; i < 100; i++ {
		final = w.job(t, ctx, v, date, "seed_reconcile")
		if final.State == "cancelled" && final.Error == "seed_cancelled" && ctx.Err() == nil && cancelledReads < 2 {
			// Reconcile never dispatches a transaction. This is an explicit new
			// read-only observation after retaining the cancelled job, not a
			// publication retry or a claim the prior clock observation vanished.
			cancelledReads++
			t.Log("bounded operator-style read-only reconciliation after cancellation", final.JobID)
			continue
		}
		if final.State != "completed" {
			t.Fatal("anchor reconcile", final.Error)
		}
		if final.Result.Target != nil {
			if final.Result.Observation.TargetDescendants >= 60 {
				break
			}
			seedClockAdvance(t, ctx, rpc, final.Result.Target.Height+60)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if final.Result == nil || final.Result.Target == nil || final.Result.Observation.TargetDescendants < 60 {
		t.Fatal("bounded target/depth wait")
	}
	attested := w.job(t, ctx, v, date, "seed_attest")
	if attested.State != "completed" || attested.Result.Finality == nil || attested.Result.Checkpoint == nil {
		t.Fatal("attest", attested.Error)
	}
	r := attested.Result
	if r.Publication.RawTx != rawTx || r.Publication.TxID != txid || r.Target.Height != r.Anchor.Height+1 || r.Checkpoint.Height != int32(r.Target.Height) || r.Checkpoint.Hash.String() != r.Target.Hash || r.Finality.TargetDescendants < 60 {
		t.Fatal("fixed target/publication")
	}
	decl, _ := json.Marshal(r.Declaration)
	if !bytes.Equal(decl, immutable) {
		t.Fatal("declaration changed")
	}

	checkpointKey, _ := hex.DecodeString(snapshot.Pin.CheckpointPublicKey)
	if e = r.Checkpoint.Verify(&chaincfg.MainNetParams, checkpointKey); e != nil {
		t.Fatal("checkpoint signature", e)
	}
	for name, hash := range map[string]string{"funding": fundHash, "anchor": r.Anchor.Hash, "target": r.Target.Hash} {
		var body string
		if e = rpc.call(ctx, "getblock", []any{hash, 0}, &body); e != nil {
			t.Fatal("public body", name, e)
		}
		if _, e = hex.DecodeString(body); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(root, "public-"+name+"-block.hex"), []byte(body+"\n"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	seedClockExportChain(t, ctx, rpc, root, r.Finality.ObservedTip)
	// These generic RPC methods exist only in this explicitly isolated test
	// harness. The production seednode allowlist exposes neither operation.
	peerRPC := seedClockOwnedNamedRPC(t, home, "peer")
	both := func(method, hash string) {
		t.Helper()
		for _, node := range []*seedClockRPC{rpc, peerRPC} {
			var result any
			if e := node.call(ctx, method, []any{hash}, &result); e != nil {
				t.Fatal("isolated reorg", method, e)
			}
		}
	}
	var aboveHash string
	if e = rpc.call(ctx, "getblockhash", []any{r.Target.Height + 10}, &aboveHash); e != nil {
		t.Fatal(e)
	}
	both("invalidateblock", aboveHash)
	rollback := w.job(t, ctx, v, date, "seed_reconcile")
	if rollback.State != "completed" || rollback.Result.Observation.CurrentFinality || rollback.Result.Observation.TargetDescendants >= 60 || len(rollback.Result.Conflicts) != 0 || rollback.Result.IncidentAtMS != 0 || !reflect.DeepEqual(rollback.Result.Finality, r.Finality) || rollback.Result.Target.Hash != r.Target.Hash {
		t.Fatal("above-target rollback", rollback.Error)
	}
	both("reconsiderblock", aboveHash)
	restored := w.job(t, ctx, v, date, "seed_reconcile")
	if restored.State != "completed" || !restored.Result.Observation.CurrentFinality || !reflect.DeepEqual(restored.Result.Finality, r.Finality) {
		t.Fatal("restored depth", restored.Error)
	}
	both("invalidateblock", r.Target.Hash)
	conflict := w.job(t, ctx, v, date, "seed_reconcile")
	if conflict.State != "completed" || len(conflict.Result.Conflicts) == 0 || conflict.Result.Observation.CurrentFinality || conflict.Result.Target.Hash != r.Target.Hash || conflict.Result.Publication.RawTx != rawTx || !reflect.DeepEqual(conflict.Result.Finality, r.Finality) {
		t.Fatal("target rollback must latch conflict without substitution", conflict.Error)
	}
	both("reconsiderblock", r.Target.Hash)
	latched := w.job(t, ctx, v, date, "seed_reconcile")
	if latched.State != "completed" || len(latched.Result.Conflicts) == 0 || latched.Result.Observation.CurrentFinality || !reflect.DeepEqual(latched.Result.Finality, r.Finality) {
		t.Fatal("conflict erased", latched.Error)
	}
	c.Stop()
	stopped := w.call(t, pilot.SeedRequest{Command: "seed_snapshot"})
	if !stopped.OK || stopped.Snapshot.NodeState != "offline" {
		t.Fatal("stop did not revoke publisher")
	}
	record := w.call(t, pilot.SeedRequest{Command: "seed_record", ViewID: v, Saturday: date})
	if !record.OK || record.Record.Finality == nil || !reflect.DeepEqual(record.Record.Finality, r.Finality) {
		t.Fatal("offline historical finality", record.Error)
	}
	history := w.call(t, pilot.SeedRequest{Command: "seed_history", ViewID: v, Limit: 8})
	if !history.OK || len(history.History.Events) != 1 {
		t.Fatal("history", history.Error)
	}
	b, _ := json.MarshalIndent(struct {
		Date        string
		TaggedClock bool
		NormalPoW   bool
		Snapshot    pilot.SeedSnapshot
		Record      gymseed.Record
	}{date, true, true, *stopped.Snapshot, *record.Record}, "", "  ")
	if e = os.WriteFile(filepath.Join(root, "public-seed-result.json"), append(b, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	logout()
	denied := w.call(t, pilot.SeedRequest{Command: "seed_record", ViewID: v, Saturday: date})
	if denied.OK {
		t.Fatal("old view disclosed")
	}
	login("incrementing256")
	iv := c.SeedSnapshot().View.ViewID
	foreign := w.job(t, ctx, iv, date, "seed_publish")
	if foreign.Error != "forbidden" {
		t.Fatal("foreign publish", foreign.Error)
	}
	foreignRead := w.call(t, pilot.SeedRequest{Command: "seed_record", ViewID: iv, Saturday: date})
	if !foreignRead.OK || foreignRead.Record.Publication.RawTx != rawTx {
		t.Fatal("shared historical read", foreignRead.Error)
	}
	logout()
	login("zero256")
	beforeReset := c.Snapshot()
	if e = c.ResetFixtureAccount(); e != nil {
		t.Fatal(e)
	}
	seedClockWait(t, ctx, func() bool {
		s := c.Snapshot()
		if s.Account.State == "failed" {
			t.Fatal("reset", s.Account.LastError)
		}
		return s.Account.State == "signed_in" && s.Account.Generation == beforeReset.Account.Generation+1
	})
	held := c.Snapshot()
	if !held.Account.Held || held.View == nil || held.View.ViewID == beforeReset.View.ViewID {
		t.Fatal("held reset identity")
	}
	hv := held.View.ViewID
	if w.call(t, pilot.SeedRequest{Command: "seed_record", ViewID: beforeReset.View.ViewID, Saturday: date}).OK {
		t.Fatal("old reset view disclosed")
	}
	for _, command := range []string{"seed_publish", "seed_attest"} {
		j := w.job(t, ctx, hv, date, command)
		if j.Error != "forbidden" {
			t.Fatal("held spend/checkpoint capability", command, j.Error)
		}
	}
	replay := w.job(t, ctx, hv, date, "seed_prepare")
	if replay.State != "completed" || replay.Result.Publication.RawTx != rawTx {
		t.Fatal("held view replay", replay.Error)
	}
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	seedClockWait(t, ctx, func() bool { return c.SeedSnapshot().NodeState == "ready" })
	reconciled := w.job(t, ctx, hv, date, "seed_reconcile")
	if reconciled.State != "completed" || reconciled.Result.Publication.DispatchCount != r.Publication.DispatchCount || !reflect.DeepEqual(reconciled.Result.Finality, r.Finality) {
		t.Fatal("held read-only reconcile", reconciled.Error)
	}
	c.Stop()
	oldJob := reconciled.JobID
	oldSeed := c.SeedSnapshot()
	w.close(t)
	c = nil
	c, e = NewSeedsForTestClock(cfg, func() time.Time { return time.UnixMilli(now.Load()) })
	if e != nil {
		t.Fatal("reopen", e)
	}
	login("zero256")
	w = newSeedClockWire(t, ctx, c)
	nv := c.Snapshot().View.ViewID
	fresh := c.SeedSnapshot()
	if fresh.StoreID != oldSeed.StoreID || fresh.CreatedAtMS != oldSeed.CreatedAtMS || fresh.NodeState != "offline" || fresh.LastJobID != "" || !reflect.DeepEqual(fresh.Pin, oldSeed.Pin) {
		t.Fatal("restart identity/cache")
	}
	if old := w.call(t, pilot.SeedRequest{Command: "seed_job", ViewID: nv, JobID: oldJob}); old.OK || old.Error != "seed_not_found" {
		t.Fatal("terminal survived process lifetime", old.Error)
	}
	reopened := w.call(t, pilot.SeedRequest{Command: "seed_record", ViewID: nv, Saturday: date})
	if !reopened.OK || reopened.Record.Publication.RawTx != rawTx || len(reopened.Record.Conflicts) == 0 || !reflect.DeepEqual(reopened.Record.Finality, r.Finality) {
		t.Fatal("reopened historical record", reopened.Error)
	}

}
