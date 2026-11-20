//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

// SOURCE PREPARATION ONLY: not installed, compiled, approved or executed.
// Baseline helpers are reused from their separately qualified source. Every
// durable action below retains its original request and view before dispatch.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type ownedFundedFixture struct {
	c               *OwnedGymController
	rpc             *poolActualRPC
	root, artifacts string
	scopes          map[string]pilot.OwnedGymScope
	items           map[string]gymstable.OwnedItem
	entries         map[string]gymentry.OwnedReceipt
	funding         ownedFundedEvidence
	counters        pilot.OwnedFixtureCounters
	spendCanonical  ownedFundedCanonical
}

func ownedFundedPhase(t *testing.T, path, phase string, public any) {
	t.Helper()
	raw, e := json.Marshal(public)
	if e != nil || len(raw) > 1<<20 {
		t.Fatal("bounded public phase evidence", e)
	}
	hostedActualWrite(t, filepath.Join(path, phase+".json"), public)
	t.Log("owned funded phase", phase)
}

func ownedFundedNew(t *testing.T, ctx context.Context, artifacts, binary string) *ownedFundedFixture {
	t.Helper()
	f := &ownedFundedFixture{root: filepath.Join(artifacts, "owned-gym"), artifacts: artifacts,
		scopes: map[string]pilot.OwnedGymScope{}, items: map[string]gymstable.OwnedItem{}, entries: map[string]gymentry.OwnedReceipt{}}
	if _, e := os.Lstat(f.root); !os.IsNotExist(e) {
		t.Fatal("fresh exclusive funded root required")
	}
	var e error
	f.c, e = NewOwnedGymPool(ctx, pilot.Config{DataDir: f.root, NodeBinary: binary}, pilot.PoolOptions{})
	if e != nil {
		t.Fatal("funded constructor", e)
	}
	t.Cleanup(func() {
		if f.c != nil {
			if e := f.c.Close(); e != nil {
				t.Error("funded owner cleanup", e)
			}
		}
	})
	f.rpc = poolActualNode(t, f.root)
	t.Cleanup(f.rpc.client.CloseIdleConnections)
	ownedFundedPhase(t, artifacts, "01-constructed", f.c.Snapshot())
	for _, fixture := range []string{"zero256", "incrementing256"} {
		s := ownedActualLogin(t, ctx, f.c, fixture)
		f.scopes[fixture], e = f.c.Scope(ctx, fixture)
		if e != nil {
			t.Fatal(e)
		}
		before := ownedActualRetry(t, ctx, func() (pilot.OwnedFixtureCounters, error) {
			return f.c.runtime.OwnedFixtureCountersForTest(ctx, f.c.init.Source)
		})
		ownedActualRetry(t, ctx, func() (struct{}, error) { return struct{}{}, f.c.Start(ctx, s.View.ViewID) })
		poolActualWait(t, ctx, "funded original fixture admission", func(ctx context.Context) bool {
			v, e := f.c.runtime.OwnedFixtureCountersForTest(ctx, f.c.init.Source)
			if ownedActualReadCategory(e) == "definite" {
				t.Fatal("admission evidence", e)
			}
			return e == nil && v.Records > before.Records
		})
		ownedActualRetry(t, ctx, func() (struct{}, error) { return struct{}{}, f.c.Stop(ctx, s.View.ViewID) })
		capture := ownedActualCapture(t, ctx, f.c, s, "funded/birth/"+fixture, 1)
		f.items[fixture] = capture.Page.Items[0]
		f.entries[fixture] = ownedActualEntry(t, ctx, f.c, s, capture)
		ownedActualLogout(t, ctx, f.c, s.View.ViewID)
		ownedFundedPhase(t, artifacts, "02-birth-"+fixture, f.items[fixture])
	}
	// No Start/job/admission follows this point, including after clock offset.
	ownedFundedIdleSource(t, ctx, f.c)
	poolActualWait(t, ctx, "all accepted admissions publicly signed", func(ctx context.Context) bool {
		var e error
		f.counters, e = f.c.runtime.OwnedFixtureCountersForTest(ctx, f.c.init.Source)
		if ownedActualReadCategory(e) == "definite" {
			t.Fatal("publication counters", e)
		}
		return e == nil && f.counters.Accepted == f.counters.Signed && f.counters.Records > 0
	})
	f.funding = ownedFundedMature(t, ctx, f.rpc, "zero256")
	ownedFundedPhase(t, artifacts, "03-funded", f.funding)
	s := ownedActualLogin(t, ctx, f.c, "zero256")
	poolActualWait(t, ctx, "actual sender spendable native84 funds", func(context.Context) bool {
		v := f.c.Snapshot()
		if v.View == nil || v.View.ViewID != s.View.ViewID || v.Child == nil {
			t.Fatal("funding lost original sender")
		}
		return v.Child.WalletState == "ready" && v.Child.WalletHeight >= uint64(f.funding.MatureHeight) && v.Child.SpendableAtoms >= hostedActualFundingAtoms
	})
	ownedFundedCaptureHeight(t, ctx, f.c, s, "funded/mature-sender", uint32(f.funding.MatureHeight), 1)
	f.checkCounters(t, ctx)
	return f
}

func (f *ownedFundedFixture) checkCounters(t *testing.T, ctx context.Context) {
	t.Helper()
	v := ownedActualRetry(t, ctx, func() (pilot.OwnedFixtureCounters, error) {
		return f.c.runtime.OwnedFixtureCountersForTest(ctx, f.c.init.Source)
	})
	if v != f.counters {
		t.Fatal("forbidden post-funding job/admission activity", v, f.counters)
	}
}

// This is independent test evidence from the actual primary chain. It is not
// a carrier journal observation or authority to change the saved delivery row.
type ownedFundedCanonical struct {
	Height           int32  `json:"height"`
	Hash             string `json:"hash"`
	TransactionIndex uint32 `json:"transaction_index"`
	TxID             string `json:"txid"`
	RawSHA256        string `json:"raw_sha256"`
}

func (f *ownedFundedFixture) canonicalLeg(t *testing.T, ctx context.Context, expected fightercarrier.RawTransaction) ownedFundedCanonical {
	t.Helper()
	// poolActualConfirm has already mined exactly one candidate containing the
	// selected transaction and proved its canonical inclusion. Re-read the same
	// current block, verify its raw header and exact transaction bytes, then
	// bracket the retained height before using it as checkpoint input.
	var height int64
	if e := f.rpc.call(ctx, "getblockcount", nil, &height); e != nil || height < 1 || height > 2147483647 {
		t.Fatal("canonical leg height", e)
	}
	var hash, rawHex string
	if e := f.rpc.call(ctx, "getblockhash", []any{height}, &hash); e != nil {
		t.Fatal(e)
	}
	if e := f.rpc.call(ctx, "getblock", []any{hash, 0}, &rawHex); e != nil || len(rawHex) > 2*wire.MaxBlockPayload {
		t.Fatal("bounded canonical leg block", e)
	}
	raw, e := hex.DecodeString(rawHex)
	if e != nil {
		t.Fatal("canonical leg block hex", e)
	}
	reader := bytes.NewReader(raw)
	var block wire.MsgBlock
	if e = block.Deserialize(reader); e != nil || reader.Len() != 0 {
		t.Fatal("canonical leg block encoding", e)
	}
	var encoded bytes.Buffer
	if block.Serialize(&encoded) != nil || !bytes.Equal(encoded.Bytes(), raw) {
		t.Fatal("canonical leg block roundtrip")
	}
	poolActualWait(t, ctx, "shared-gate canonical leg header verification", func(ctx context.Context) bool {
		actual, e := prospect.TryHashHeader(ctx, block.Header)
		if e == prospect.ErrHeaderHashBusy {
			return false
		}
		if e != nil || actual.String() != hash {
			t.Fatal("canonical leg header identity", e)
		}
		return true
	})
	matches, index := 0, uint32(0)
	for n, tx := range block.Transactions {
		if tx.TxHash().String() != expected.TxID {
			continue
		}
		var transaction bytes.Buffer
		if n == 0 || tx.Serialize(&transaction) != nil || hex.EncodeToString(transaction.Bytes()) != expected.Raw {
			t.Fatal("canonical leg differs from exact signed transaction")
		}
		sum := sha256.Sum256(transaction.Bytes())
		if hex.EncodeToString(sum[:]) != expected.RawSHA256 {
			t.Fatal("canonical leg raw digest")
		}
		matches++
		index = uint32(n)
	}
	var after string
	if matches != 1 || f.rpc.call(ctx, "getblockhash", []any{height}, &after) != nil || after != hash {
		t.Fatal("canonical leg inclusion bracket")
	}
	return ownedFundedCanonical{int32(height), hash, index, expected.TxID, expected.RawSHA256}
}

func (f *ownedFundedFixture) transfer(t *testing.T, ctx context.Context) fightercarrier.Operation {
	t.Helper()
	s := f.c.Snapshot()
	if s.View == nil || s.View.Fixture != "zero256" {
		t.Fatal("original sender required")
	}
	id := ownedActualID("funded/exact-transfer")
	item := f.items["zero256"]
	recipient := f.scopes["incrementing256"].Carrier.Pin.GymPublicKey
	review := ownedActualRetry(t, ctx, func() (pilot.OwnedTransferReview, error) {
		return f.c.TransferPrepare(ctx, s.View.ViewID, id, item.Birth.ProspectID, recipient)
	})
	if review.RequestID != id || review.ProspectID != item.Birth.ProspectID || review.Details.PriorStateID != item.Ownership.StateID || review.Details.RecipientGym != recipient || review.Details.SenderGym != s.View.GymPublicKey {
		t.Fatal("original transfer review mismatch")
	}
	op := f.oneUse(t, ctx, s.View.ViewID, id, "confirm", func() (fightercarrier.Operation, error) {
		return f.c.TransferConfirm(ctx, s.View.ViewID, review.ReviewID)
	})
	if op.Stage != "signed" || op.SignedPackage == nil || op.RequestID != id {
		t.Fatal("exact signed transfer missing")
	}
	signed := *op.SignedPackage
	ownedFundedPhase(t, f.artifacts, "04-signed-public-carrier", signed)
	for _, leg := range []string{"commit", "spend"} {
		raw := signed.Commit
		if leg == "spend" {
			raw = signed.Spend
		}
		disclosure := ownedActualRetry(t, ctx, func() (pilot.OwnedTransferDeliveryReview, error) {
			return f.c.TransferReviewDelivery(ctx, s.View.ViewID, id, leg)
		})
		if disclosure.RequestID != id || disclosure.Leg != leg || disclosure.TxID != raw.TxID || disclosure.RawSHA256 != raw.RawSHA256 || disclosure.TransferID != signed.TransferID {
			t.Fatal("separate delivery review mismatch")
		}
		op = f.oneUse(t, ctx, s.View.ViewID, id, "deliver-"+leg, func() (fightercarrier.Operation, error) {
			return f.c.TransferDeliver(ctx, s.View.ViewID, disclosure.ReviewID)
		})
		var terminal []byte
		if leg == "spend" {
			if op.Stage != "complete" || op.SpendObservation == nil || (op.SpendObservation.State != "mempool" && op.SpendObservation.State != "canonical") {
				t.Fatal("exact spend delivery not complete")
			}
			var e error
			terminal, e = json.Marshal(op)
			if e != nil {
				t.Fatal(e)
			}
		}
		poolActualWait(t, ctx, "actual mining node accepted "+leg, func(ctx context.Context) bool {
			var mempool []string
			if e := f.rpc.call(ctx, "getrawmempool", nil, &mempool); e != nil {
				t.Fatal(e)
			}
			for _, txid := range mempool {
				if txid == raw.TxID {
					return true
				}
			}
			return false
		})
		poolActualConfirm(t, ctx, f.rpc, raw.TxID)
		canonical := f.canonicalLeg(t, ctx, raw)
		ownedFundedPhase(t, f.artifacts, "05-canonical-"+leg, canonical)
		if leg == "spend" {
			f.spendCanonical = canonical
		}
		op = ownedActualRetry(t, ctx, func() (fightercarrier.Operation, error) { return f.c.TransferObserve(ctx, s.View.ViewID, id, leg) })
		observation := op.CommitObservation
		if leg == "spend" {
			observation = op.SpendObservation
		}
		if observation == nil || observation.ObservedTxID != raw.TxID || observation.TransferID != signed.TransferID || observation.RawSHA256 != raw.RawSHA256 {
			t.Fatal("exact delivery observation missing", leg)
		}
		if leg == "commit" && observation.State != "canonical" {
			t.Fatal("canonical commit observation missing")
		}
		if leg == "spend" {
			after, e := json.Marshal(op)
			if e != nil || !bytes.Equal(terminal, after) {
				t.Fatal("completed Observe rewrote exact saved delivery")
			}
		}
		ownedFundedPhase(t, f.artifacts, "05-observed-"+leg, *observation)
	}
	if op.Stage != "complete" || op.SignedPackage == nil || *op.SignedPackage != signed {
		t.Fatal("delivery changed signed intent")
	}
	return op
}

func (f *ownedFundedFixture) checkpoint(t *testing.T, ctx context.Context, height int32, hash string) registry.PublicPublicationEntry {
	t.Helper()
	h, e := chainhash.NewHashFromStr(hash)
	if e != nil {
		t.Fatal(e)
	}
	cp := chaincfg.SignedCheckpoint{Version: 1, Network: chaincfg.MainNetParams.Net, GenesisHash: *chaincfg.MainNetParams.GenesisHash, Height: height, Hash: *h}
	seed := sha256.Sum256([]byte("SLUG candidate seed checkpoint fixture\x00v1\x00"))
	key := ed25519.NewKeyFromSeed(seed[:])
	defer clear(key)
	if hex.EncodeToString(key.Public().(ed25519.PublicKey)) != f.c.init.Source.Ownership.CheckpointKey {
		t.Fatal("published fixture checkpoint key mismatch")
	}
	message, e := cp.SigningBytes()
	if e != nil {
		t.Fatal(e)
	}
	cp.Signature = ed25519.Sign(key, message)
	return ownedActualRetry(t, ctx, func() (registry.PublicPublicationEntry, error) {
		return f.c.runtime.PublishOwnedCheckpointForTest(ctx, f.c.init.Source, cp)
	})
}

func (f *ownedFundedFixture) finality(t *testing.T, ctx context.Context, view, id, want string, height uint32) pilot.OwnedTransferFinality {
	t.Helper()
	var v pilot.OwnedTransferFinality
	var last error
	poolActualWait(t, ctx, "complete public transfer "+want, func(ctx context.Context) bool {
		v, last = f.c.TransferFinality(ctx, view, id)
		if ownedActualReadCategory(last) == "definite" {
			t.Fatal("finality definite refusal", last)
		}
		if last == nil && v.Head.ChainHeight >= height && v.State != want {
			t.Fatal("completed public replay disagrees", v.State, want)
		}
		return last == nil && v.Head.ChainHeight >= height && v.State == want
	}, func() any {
		return struct {
			State  string
			Height uint32
			Error  error
		}{v.State, v.Head.ChainHeight, last}
	})
	return v
}

func TestOwnedGymActualNativeFundedHandoff(t *testing.T) {
	artifacts, binary := os.Getenv("SLUG_OWNED_FUNDED_ROOT"), os.Getenv("SLUG_OWNED_FUNDED_NODE")
	if artifacts == "" || binary == "" {
		t.Skip("explicit isolated funded fixture required")
	}
	if !filepath.IsAbs(artifacts) || !filepath.IsAbs(binary) {
		t.Fatal("absolute fixture paths required")
	}
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Minute)
	defer stop()
	f := ownedFundedNew(t, ctx, artifacts, binary)
	s := f.c.Snapshot()
	ownedActualLogout(t, ctx, f.c, s.View.ViewID)
	if e := f.c.Close(); e != nil {
		t.Fatal("native handoff cleanup", e)
	}
	f.c = nil
	// No TransferPrepare has occurred. The same explicit private fixture tree
	// remains here; this report contains only independently checked public data.
	stat, e := os.Lstat(f.root)
	if e != nil || !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
		t.Fatal("exact native root identity", e)
	}
	identity, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("native root filesystem identity unavailable")
	}
	ownedFundedPhase(t, artifacts, "native-handoff", struct {
		Version                                                                                     uint32
		Mode, Root, RootDevice, RootInode                                                           string
		SenderFixture, RecipientFixture, ProspectID, RecipientGym, NodeBinary, RPCConfigurationPath string
		Scopes                                                                                      map[string]pilot.OwnedGymScope
		Items                                                                                       map[string]gymstable.OwnedItem
		Funding                                                                                     ownedFundedEvidence
		Counters                                                                                    pilot.OwnedFixtureCounters
		Prepared                                                                                    bool
	}{1, pilot.OwnedGymMode, f.root, strconv.FormatUint(uint64(identity.Dev), 16), strconv.FormatUint(uint64(identity.Ino), 16),
		"zero256", "incrementing256", f.items["zero256"].Birth.ProspectID, f.scopes["incrementing256"].Carrier.Pin.GymPublicKey, binary, filepath.Join(f.root, "shared", "primary.conf"),
		f.scopes, f.items, f.funding, f.counters, false})
}

// An approval handle may already be consumed when a lower layer reports Busy.
// Do not retry it by sentinel. Inspect only its original durable request, then
// fail this attempt with public correlation; no replacement approval is minted.
func (f *ownedFundedFixture) oneUse(t *testing.T, ctx context.Context, view, request, phase string, fn func() (fightercarrier.Operation, error)) fightercarrier.Operation {
	t.Helper()
	value, e := fn()
	if e == nil {
		return value
	}
	retained, inspectErr := f.c.TransferOperation(ctx, view, request)
	t.Fatalf("one-use %s failed; request=%s retained-stage=%s inspect=%v original=%v", phase, request, retained.Stage, inspectErr, e)
	return fightercarrier.Operation{}
}

// PrepareOwnedSource is constructor-only. A stopped, signed-out live runtime
// instead advances its original concrete source through the public bounded API.
func ownedFundedIdleSource(t *testing.T, ctx context.Context, c *OwnedGymController) {
	t.Helper()
	if s := c.Snapshot(); s.State != "signed_out" || s.View != nil || s.Child != nil {
		t.Fatal("idle source requires joined signed-out controller")
	}
	poolActualWait(t, ctx, "idle exact public source catch-up", func(ctx context.Context) bool {
		p, e := c.runtime.RefreshOwnership(ctx, 8)
		if ownedActualReadCategory(e) == "definite" {
			t.Fatal("idle source definite refusal", e)
		}
		return e == nil && p.Ready && p.Head != nil
	})
}
func ownedFundedCaptureHeight(t *testing.T, ctx context.Context, c *OwnedGymController, s pilot.OwnedGymSnapshot, label string, height uint32, count int) pilot.OwnedStableCapture {
	t.Helper()
	var capture pilot.OwnedStableCapture
	n := 0
	poolActualWait(t, ctx, "fresh owned capture through funding height", func(ctx context.Context) bool {
		n++
		var e error
		capture, e = c.CaptureStable(ctx, s.View.ViewID, ownedActualID(label+"/"+strconv.Itoa(n)))
		if ownedActualReadCategory(e) == "definite" {
			t.Fatal("funded capture definite refusal", e)
		}
		return e == nil && capture.Page.Head.ChainHeight >= height && len(capture.Page.Items) >= count
	})
	if pilot.ValidateOwnedStableCapture(capture, c.init.Source, *s.View) != nil {
		t.Fatal("funded original capture invalid")
	}
	return capture
}

func ownedFundedContains(t *testing.T, capture pilot.OwnedStableCapture, id string) *gymstable.OwnedItem {
	t.Helper()
	if capture.Page.HasMore {
		t.Fatal("bounded actual fixture exceeded single complete page; cannot assert absence")
	}
	for n := range capture.Page.Items {
		if capture.Page.Items[n].Birth.ProspectID == id {
			v := capture.Page.Items[n]
			return &v
		}
	}
	return nil
}

func (f *ownedFundedFixture) addReceivedEntry(t *testing.T, ctx context.Context, s pilot.OwnedGymSnapshot) {
	t.Helper()
	capture := ownedActualCapture(t, ctx, f.c, s, "recipient/final-before-entry", 2)
	item := ownedFundedContains(t, capture, f.items["zero256"].Birth.ProspectID)
	if item == nil || item.Birth != f.items["zero256"].Birth {
		t.Fatal("recipient lost original birth")
	}
	intent := ownedActualRetry(t, ctx, func() (gymstable.OwnedIntent, error) {
		return f.c.PrepareSlot(ctx, s.View.ViewID, gymstable.OwnedSlotRequest{ExpectedRevision: capture.Page.Status.RosterRevision, Slot: 2, ProspectID: item.Birth.ProspectID, ExpectedStateID: item.Ownership.StateID, Head: capture.Page.Head})
	})
	mutation := ownedActualRetry(t, ctx, func() (gymstable.OwnedMutationResult, error) {
		return f.c.ApplySlot(ctx, s.View.ViewID, intent.OperationID)
	})
	upcoming := ownedActualRetry(t, ctx, func() (gymentry.OwnedEventStatus, error) { return f.c.EntryUpcoming(ctx, s.View.ViewID) })
	// The unchanged entry rule permits one fighter per class. Keep both owned
	// roster slots, but enter only the received fighter for this cutoff proof.
	ids := []string{item.Birth.ProspectID}
	req := gymentry.Request{Version: 1, EventID: upcoming.Event.EventID, Saturday: upcoming.Event.Saturday, Action: "replace", ExpectedRevision: upcoming.Revision, ExpectedRosterRevision: mutation.Receipt.Revision, ProspectIDs: ids}
	entry := ownedActualRetry(t, ctx, func() (gymentry.OwnedIntent, error) { return f.c.PrepareEntry(ctx, s.View.ViewID, req) })
	result := ownedActualRetry(t, ctx, func() (gymentry.OwnedMutationResult, error) {
		return f.c.ApplyEntry(ctx, s.View.ViewID, entry.OperationID)
	})
	f.entries["incrementing256"] = result.Receipt
}

func (f *ownedFundedFixture) waitParticipant(t *testing.T, ctx context.Context, s pilot.OwnedGymSnapshot, date string) {
	t.Helper()
	scope := f.scopes[s.View.Fixture]
	id, e := gymevent.OwnedParticipantID(scope.EventPin, scope.EntryPin)
	if e != nil {
		t.Fatal(e)
	}
	var status gymevent.OwnedEventStatus
	var last error
	poolActualWait(t, ctx, "original noon participant "+s.View.Fixture, func(ctx context.Context) bool {
		status, last = f.c.EventStatus(ctx, s.View.ViewID, date)
		if last != nil && last != gymevent.ErrBusy && last != gymevent.ErrUnavailable && ownedActualReadCategory(last) == "definite" {
			t.Fatal("event status definite refusal", last)
		}
		if last != nil {
			return false
		}
		for _, v := range status.CapturedParticipants {
			if v == id {
				return true
			}
		}
		return false
	}, func() any {
		return struct {
			Status gymevent.OwnedEventStatus
			Error  error
		}{status, last}
	})
}

func TestOwnedGymActualFundedNoonPending(t *testing.T)     { ownedFundedNoon(t, false) }
func TestOwnedGymActualFundedNoonFinalBefore(t *testing.T) { ownedFundedNoon(t, true) }

func ownedFundedNoon(t *testing.T, finalBefore bool) {
	artifacts, binary := os.Getenv("SLUG_OWNED_FUNDED_ROOT"), os.Getenv("SLUG_OWNED_FUNDED_NODE")
	if artifacts == "" || binary == "" {
		t.Skip("explicit isolated funded/noon fixture required")
	}
	if !filepath.IsAbs(artifacts) || !filepath.IsAbs(binary) {
		t.Fatal("absolute fixture paths required")
	}
	grid := os.Getenv("SLUG_OWNED_CONDITION_GRID")
	if !filepath.IsAbs(grid) || filepath.Clean(grid) != grid {
		t.Fatal("absolute locked condition grid required")
	}
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Minute)
	defer stop()
	f := ownedFundedNew(t, ctx, artifacts, binary)
	op := f.transfer(t, ctx)
	if op.SpendObservation == nil || op.SignedPackage == nil {
		t.Fatal("actual reveal missing")
	}
	checkpoint := f.checkpoint(t, ctx, f.spendCanonical.Height, f.spendCanonical.Hash)
	ownedFundedPhase(t, artifacts, "06-checkpoint", checkpoint)
	poolActualMine(t, ctx, f.rpc, 59)
	s := f.c.Snapshot()
	boundaryHeight := uint32(f.spendCanonical.Height) + 59
	f.finality(t, ctx, s.View.ViewID, op.RequestID, "pending", boundaryHeight)
	fighter := f.items["zero256"].Birth.ProspectID
	if item := ownedFundedContains(t, ownedActualCapture(t, ctx, f.c, s, "pending/sender", 1), fighter); item == nil || item.Ownership.OwnerPublicKey != s.View.GymPublicKey {
		t.Fatal("prior owner lost before sixty")
	}
	ownedActualLogout(t, ctx, f.c, s.View.ViewID)
	b := ownedActualLogin(t, ctx, f.c, "incrementing256")
	if ownedFundedContains(t, ownedActualCapture(t, ctx, f.c, b, "pending/recipient", 1), fighter) != nil {
		t.Fatal("recipient gained ownership at59")
	}
	ownedActualLogout(t, ctx, f.c, b.View.ViewID)
	s = ownedActualLogin(t, ctx, f.c, "zero256")
	if finalBefore {
		poolActualMine(t, ctx, f.rpc, 1)
		f.finality(t, ctx, s.View.ViewID, op.RequestID, "finalized", boundaryHeight+1)
		ownedActualLogout(t, ctx, f.c, s.View.ViewID)
		b = ownedActualLogin(t, ctx, f.c, "incrementing256")
		f.addReceivedEntry(t, ctx, b)
		ownedActualLogout(t, ctx, f.c, b.View.ViewID)
		s = ownedActualLogin(t, ctx, f.c, "zero256")
	}
	f.checkCounters(t, ctx)
	upcoming := ownedActualRetry(t, ctx, func() (gymentry.OwnedEventStatus, error) { return f.c.EntryUpcoming(ctx, s.View.ViewID) })
	date := upcoming.Event.Saturday
	league, e := f.c.runtime.OwnedLeague()
	if e != nil {
		t.Fatal(e)
	}
	config := league.Config()
	ownedActualLogout(t, ctx, f.c, s.View.ViewID)
	// This real catch-up happens before the idle clock transition. There are
	// no retained child borrows, workers or subsequent admission operations.
	ownedFundedIdleSource(t, ctx, f.c)
	offset := time.UnixMilli(upcoming.Event.CutoffMS).Add(-40 * time.Second).Sub(time.Now())
	if offset <= 0 || offset > 8*24*time.Hour {
		t.Fatal("fresh run cannot reach a supported future boundary")
	}
	if e = f.c.runtime.UseOwnedCalendarOffsetForTest(ctx, config, offset); e != nil {
		t.Fatal("idle original clock installation", e)
	}
	ownedFundedPhase(t, artifacts, "07-clock-transition", struct {
		CutoffMS int64
		OffsetNS int64
		Frozen   pilot.OwnedFixtureCounters
	}{upcoming.Event.CutoffMS, int64(offset), f.counters})
	s = ownedActualLogin(t, ctx, f.c, "zero256")
	f.waitParticipant(t, ctx, s, date)
	// Completing historical capture does not itself make current replay ready.
	ownedActualCapture(t, ctx, f.c, s, "noon/current-after-A", 0)
	f.checkCounters(t, ctx)
	ownedActualLogout(t, ctx, f.c, s.View.ViewID)
	b = ownedActualLogin(t, ctx, f.c, "incrementing256")
	f.waitParticipant(t, ctx, b, date)
	record := ownedActualRetry(t, ctx, func() (gymevent.OwnedSnapshotReceipt, error) { return f.c.EventRecord(ctx, b.View.ViewID, date) })
	if len(record.Input.Participants) != 2 || record.Input.Event.Saturday != date || record.Input.Cutoff.CutoffMS != upcoming.Event.CutoffMS {
		t.Fatal("original two-participant noon snapshot")
	}
	wantOwner := f.scopes["zero256"].Carrier.Pin.GymPublicKey
	if finalBefore {
		wantOwner = f.scopes["incrementing256"].Carrier.Pin.GymPublicKey
	}
	matches := 0
	excludedSender := 0
	seenParticipants := map[string]bool{}
	for _, participant := range record.Input.Participants {
		fixture := ""
		for _, candidate := range []string{"zero256", "incrementing256"} {
			scope := f.scopes[candidate]
			want, e := gymevent.OwnedParticipantID(scope.EventPin, scope.EntryPin)
			if e != nil {
				t.Fatal(e)
			}
			if participant.ParticipantID == want {
				fixture = candidate
			}
		}
		if fixture == "" || seenParticipants[fixture] || participant.Entry.LatestReceipt == nil || !reflect.DeepEqual(*participant.Entry.LatestReceipt, f.entries[fixture]) {
			t.Fatal("noon participant/original entry correlation")
		}
		seenParticipants[fixture] = true
		if fixture == "zero256" {
			for _, excluded := range participant.Excluded {
				if excluded.ProspectID == fighter {
					if !finalBefore || excluded.Reason != "not_owned_at_cutoff" || excluded.OwnerPublicKey != wantOwner {
						t.Fatal("unexpected noon exclusion")
					}
					excludedSender++
				}
			}
		}
		for _, member := range participant.Members {
			if member.Birth.ProspectID == fighter {
				matches++
				ownedFundedAssertPrimeCondition(t, os.Getenv("SLUG_OWNED_CONDITION_GRID"), f.items["zero256"], member, upcoming.Event.CutoffMS)
				if member.Birth != f.items["zero256"].Birth || member.Ownership.OwnerPublicKey != wantOwner {
					t.Fatal("noon owner/birth mismatch")
				}
			}
		}
	}
	if len(seenParticipants) != 2 || finalBefore && excludedSender != 1 || !finalBefore && excludedSender != 0 {
		t.Fatal("noon exact participant/exclusion mismatch")
	}
	if matches != 1 {
		t.Fatal("noon ownership not represented exactly once", matches)
	}
	beforeEvent, e := league.Events().Info(ctx)
	if e != nil {
		t.Fatal("retained live event identity", e)
	}
	ownedFundedPhase(t, artifacts, "08-noon-event", record)
	f.checkCounters(t, ctx)
	ownedActualLogout(t, ctx, f.c, b.View.ViewID)
	s = ownedActualLogin(t, ctx, f.c, "zero256")
	if !finalBefore {
		poolActualMine(t, ctx, f.rpc, 1)
	}
	final := f.finality(t, ctx, s.View.ViewID, op.RequestID, "finalized", boundaryHeight+1)
	if final.OwnerGym != f.scopes["incrementing256"].Carrier.Pin.GymPublicKey {
		t.Fatal("final recipient mismatch")
	}
	if ownedFundedContains(t, ownedActualCapture(t, ctx, f.c, s, "final/sender", 0), fighter) != nil {
		t.Fatal("sender still owns finalized fighter")
	}
	after := ownedActualRetry(t, ctx, func() (gymevent.OwnedSnapshotReceipt, error) { return f.c.EventRecord(ctx, s.View.ViewID, date) })
	if !reflect.DeepEqual(record, after) {
		t.Fatal("current finality rewrote original noon snapshot")
	}
	ownedActualLogout(t, ctx, f.c, s.View.ViewID)
	b = ownedActualLogin(t, ctx, f.c, "incrementing256")
	item := ownedFundedContains(t, ownedActualCapture(t, ctx, f.c, b, "final/recipient", 2), fighter)
	if item == nil || item.Birth != f.items["zero256"].Birth || !reflect.DeepEqual(item.Admission, f.items["zero256"].Admission) {
		t.Fatal("recipient ownership lost historical admission")
	}
	f.checkCounters(t, ctx)
	ownedFundedPhase(t, artifacts, "09-finality-after-record", final)
	ownedActualLogout(t, ctx, f.c, b.View.ViewID)
	ownedFundedIdleSource(t, ctx, f.c)
	originalInit := f.c.init
	if e = f.c.Close(); e != nil {
		t.Fatal("close original future-clock owners", e)
	}
	f.c = nil
	// Reacquire the actual existing root's lifetime lock. No new account/session,
	// wallet or full controller is opened with a future clock.
	lock, e := openOwnedRoot(f.root)
	if e != nil {
		t.Fatal("original component audit lifetime", e)
	}
	defer func() {
		if e := lock.Close(); e != nil {
			t.Error("component audit lifetime close", e)
		}
	}()
	var auditedInit ownedGymInit
	present, e := readHostedControl(filepath.Join(f.root, ownedInitName), ownedControlLimit, &auditedInit)
	if e != nil || !present || !reflect.DeepEqual(auditedInit, originalInit) {
		t.Fatal("component audit original INIT mismatch", e)
	}
	audit, e := pilot.AuditOwnedNoonComponentsForTest(ctx, filepath.Join(f.root, "shared"), binary, config, offset, beforeEvent, record, f.counters)
	if e != nil {
		t.Fatal("real component replay and full read-only event audit", e)
	}
	if audit.Head.ChainHeight < boundaryHeight+1 || audit.Event.EventCount != 1 || !reflect.DeepEqual(audit.Cutoff, record.Input.Cutoff) {
		t.Fatal("reopened component history mismatch")
	}
	ownedFundedPhase(t, artifacts, "10-component-audit", audit)
}
