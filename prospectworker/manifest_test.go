//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"bytes"
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

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
)

type mwCase struct {
	Job       prospectjob.ManifestJob `json:"job"`
	Template  prospectwork.Template   `json:"template"`
	Candidate prospectwork.Candidate  `json:"candidate"`
	Proof     prospect.Proof          `json:"proof"`
	Auth      gymauth.Authorization   `json:"authorization"`
	Verified  prospect.Verified       `json:"expected_verified"`
	Record    registry.ManifestRecord `json:"expected_record"`
}

func mwFixture(t *testing.T) (mwCase, ManifestConfig) {
	t.Helper()
	raw, err := os.ReadFile("../registry/testdata/manifest/reference/manifest_admission_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "d32a8653c76c643addb3df091ca8d7dfdba66c2486f2ca305e718113f71d5794" {
		t.Fatal("independent corpus changed")
	}
	var f struct {
		Pin   registry.ManifestAuthorityPin `json:"pin"`
		Cases []mwCase                      `json:"cases"`
	}
	if json.Unmarshal(raw, &f) != nil || len(f.Cases) != 5 {
		t.Fatal("fixture decode")
	}
	tc := f.Cases[2] // Independent normal-PoW witness winner, nonce zero.
	c := ManifestConfig{Version: 2, Policy: prospectnode.Policy{Version: 1, Network: tc.Job.Context.Network, Genesis: tc.Job.Context.Genesis, DNAVersion: tc.Job.Context.DNAVersion, ProspectTarget: tc.Job.Context.ProspectTarget}, PublicKey: tc.Job.PublicKey, PayoutAddress: tc.Candidate.PayoutAddress, AuthorityOrigin: "http://127.0.0.1:37619", AuthorityPin: f.Pin, MaxRecords: 100, MaxBytes: 64 << 20}
	c.TransportTrustID, err = prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: c.AuthorityOrigin, Token: strings.Repeat("00", 32), AuthorityPin: f.Pin})
	if err != nil {
		t.Fatal(err)
	}
	c, err = ValidateManifestConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	return tc, c
}
func mwBox(t *testing.T, c ManifestConfig) (*ManifestOutbox, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "outbox")
	b, err := CreateManifestOutbox(p, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b, p
}
func mwScan(f mwCase) ManifestScan {
	return ManifestScan{Version: 2, Job: f.Job, Template: f.Template, Candidate: f.Candidate, ExtraNonce: ^uint64(0)}
}
func mwFound(t *testing.T, b *ManifestOutbox, f mwCase) ManifestEntry {
	t.Helper()
	s := mwScan(f)
	if err := b.saveScan(&s); err != nil {
		t.Fatal(err)
	}
	if err := b.reserve(s); err != nil {
		t.Fatal(err)
	}
	s.NextNonce++
	e, err := b.found(ManifestEntry{Version: 2, WorkHash: f.Verified.WorkHash, Job: f.Job, Candidate: f.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature"}, s)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func mwResult(f mwCase) prospectservice.ManifestAdmissionResult {
	return prospectservice.ManifestAdmissionResult{Version: 3, ContextSource: "trusted-node-snapshot", CurrentNodeChecked: true, Authentication: "gym-key-control", BirthRule: "job-issuance", Inserted: true, Record: f.Record}
}

type mwAuthority struct {
	c      ManifestConfig
	f      mwCase
	issue  func()
	submit func() (prospectservice.ManifestAdmissionResult, error)
}

func (a *mwAuthority) ProtocolVersion() uint32                     { return 3 }
func (a *mwAuthority) AuthorityOrigin() string                     { return a.c.AuthorityOrigin }
func (a *mwAuthority) AuthorityPin() registry.ManifestAuthorityPin { return a.c.AuthorityPin }
func (a *mwAuthority) TransportTrustID() string                    { return a.c.TransportTrustID }
func (a *mwAuthority) IssueManifestJob(context.Context, string) (prospectjob.ManifestJob, error) {
	if a.issue != nil {
		a.issue()
	}
	return a.f.Job, nil
}
func (a *mwAuthority) SubmitManifest(context.Context, prospectjob.ManifestJob, prospect.Proof, gymauth.Authorization) (prospectservice.ManifestAdmissionResult, error) {
	return a.submit()
}

type mwNode struct {
	f    mwCase
	call func(string)
}

func (n *mwNode) ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error) {
	return n.f.Template, nil
}
func (n *mwNode) Proposal(context.Context, prospect.Context, string) error {
	if n.call != nil {
		n.call("proposal")
	}
	return nil
}
func (n *mwNode) RecheckContext(context.Context, prospect.Context) error { return nil }
func (n *mwNode) SubmitBlock(context.Context, prospect.Context, string) error {
	if n.call != nil {
		n.call("broadcast")
	}
	return nil
}
func (n *mwNode) CanonicalBlock(context.Context, prospect.Context, prospect.Proof, string) error {
	if n.call != nil {
		n.call("canonical")
	}
	return nil
}

func TestManifestWorkerProductionOrderingAndReceipt(t *testing.T) {
	f, c := mwFixture(t)
	b, _ := mwBox(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events := []string{}
	n := &mwNode{f: f, call: func(s string) {
		events = append(events, s)
		if s == "broadcast" {
			rows, e := b.List(0, 1)
			if e != nil || len(rows) != 1 || rows[0].Authorization != nil {
				t.Fatal("broadcast preceded found Sync", e)
			}
		}
	}}
	a := &mwAuthority{c: c, f: f, submit: func() (prospectservice.ManifestAdmissionResult, error) {
		events = append(events, "admit")
		rows, e := b.List(0, 1)
		if e != nil || len(rows) != 1 || rows[0].Authorization == nil {
			t.Fatal("submit preceded auth Sync", e)
		}
		cancel()
		return mwResult(f), nil
	}}
	w, err := NewManifest(c, a, n, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		events = append(events, "sign")
		return f.Auth, nil
	}, b)
	if err != nil {
		t.Fatal(err)
	}
	w.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS + 100) }
	scan := mwScan(f)
	if err := b.saveScan(&scan); err != nil {
		t.Fatal(err)
	}
	if err = w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"broadcast", "canonical", "sign", "admit"}) {
		t.Fatal(events)
	}
	rows, err := b.List(0, 8)
	if err != nil || len(rows) != 1 || rows[0].AdmissionState != "accepted" || rows[0].BlockState != "canonical" || !reflect.DeepEqual(rows[0].Receipt.Record, f.Record) {
		t.Fatal("receipt not retained exactly", err)
	}
	if rows[0].WorkHash != f.Verified.WorkHash || w.Status().Attempts != 1 {
		t.Fatal("normal production hash differs")
	}
	if err = w.Run(ctx); err == nil {
		t.Fatal("worker ran twice")
	}
}

func TestManifestOutboxIdentityAndCanonicalIsolation(t *testing.T) {
	f, c := mwFixture(t)
	b, path := mwBox(t, c)
	mwFound(t, b, f)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ManifestConfig){func(x *ManifestConfig) { x.TransportTrustID = strings.Repeat("f", 64) }, func(x *ManifestConfig) { x.AuthorityPin.AuthorityID = strings.Repeat("f", 64) }, func(x *ManifestConfig) { x.AuthorityOrigin = "http://127.0.0.1:37620" }, func(x *ManifestConfig) { x.MaxBytes-- }} {
		bad := c
		mutate(&bad)
		if opened, err := OpenManifestOutbox(path, bad); err == nil || opened != nil {
			t.Fatal("rebound outbox")
		}
	}
	if old, err := OpenOutbox(path, Config{Version: 1, Policy: c.Policy, PublicKey: c.PublicKey, PayoutAddress: c.PayoutAddress, AuthorityOrigin: c.AuthorityOrigin, MaxRecords: c.MaxRecords, MaxBytes: c.MaxBytes}); err == nil || old != nil {
		t.Fatal("old reader accepted new journal")
	}
	b, err := OpenManifestOutbox(path, c)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	raw, _ := b.db.Get([]byte("meta"), nil)
	if manifestDecodeCanonical(append(raw, '\n'), maxMetadataSize, new(manifestOutboxMetadata)) == nil {
		t.Fatal("stored noncanonical JSON accepted")
	}
	rows, err := b.List(0, 1)
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	rows[0].Candidate.Proof.MerkleBranch[0] = strings.Repeat("f", 64)
	again, _ := b.List(0, 1)
	if reflect.DeepEqual(rows, again) {
		t.Fatal("returned entry aliases storage")
	}
}

func TestManifestOutboxUncertainFoundAndReceiptReopen(t *testing.T) {
	f, c := mwFixture(t)
	b, path := mwBox(t, c)
	s := mwScan(f)
	if err := b.saveScan(&s); err != nil {
		t.Fatal(err)
	}
	write := b.writeBatch
	b.writeBatch = func(batch *leveldb.Batch, o *opt.WriteOptions) error {
		if o == nil || !o.Sync {
			t.Fatal("unsynced mutation")
		}
		if err := write(batch, o); err != nil {
			return err
		}
		return errors.New("lost ack")
	}
	s.NextNonce = 1
	e, err := b.found(ManifestEntry{Version: 2, WorkHash: f.Verified.WorkHash, Job: f.Job, Candidate: f.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature"}, s)
	if !errors.Is(err, ErrPoisoned) || e.Sequence != 0 {
		t.Fatal("uncertain write leaked success", err)
	}
	if rows, err := b.List(0, 1); !errors.Is(err, ErrPoisoned) || rows != nil {
		t.Fatal("poisoned read")
	}
	b.Close()
	b, err = OpenManifestOutbox(path, c)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	rows, err := b.List(0, 1)
	if err != nil || len(rows) != 1 {
		t.Fatal("lost found", err)
	}
	e = rows[0]
	e.BlockState = "submitted"
	if err = b.update(e); err != nil {
		t.Fatal(err)
	}
	e.BlockState = "canonical"
	if err = b.update(e); err != nil {
		t.Fatal(err)
	}
	e.Authorization = &f.Auth
	e.AdmissionState = "pending"
	if err = b.update(e); err != nil {
		t.Fatal(err)
	}
	result := mwResult(f)
	e.Receipt = &result
	e.AdmissionState = "accepted"
	if err = b.update(e); err != nil {
		t.Fatal(err)
	}
	mutated := e
	other := *e.Receipt
	other.Record.StaticBinding.ManifestDigest = strings.Repeat("f", 64)
	mutated.Receipt = &other
	if b.update(mutated) == nil {
		t.Fatal("changed first receipt")
	}
	e.BlockState = "pending"
	if b.update(e) == nil {
		t.Fatal("terminal block state regressed")
	}
	if b.meta.Reserved != 0 || b.meta.Pending != 0 {
		t.Fatal("terminal reservations retained")
	}
}

func TestManifestConfigStrictAndRoundTrip(t *testing.T) {
	_, c := mwFixture(t)
	raw, _ := json.Marshal(c)
	got, e := ParseManifestConfig(raw)
	if e != nil || got != c {
		t.Fatal(e)
	}
	for _, bad := range [][]byte{append(raw, 'x'), bytes.Replace(raw, []byte(`"version":2`), []byte(`"version":2,"ver\u0073ion":2`), 1), bytes.Replace(raw, []byte(`"version":2`), []byte(`"Version":2`), 1), bytes.Replace(raw, []byte(`"max_records":100`), []byte(`"max_records":null`), 1), bytes.Replace(raw, []byte(`"version":2`), []byte(`"version":1`), 1)} {
		if _, e := ParseManifestConfig(bad); e == nil {
			t.Fatal("bad config accepted")
		}
	}
}

func TestManifestWorkerAdapterTrustAndResultRefusal(t *testing.T) {
	f, c := mwFixture(t)
	b, _ := mwBox(t, c)
	a := &mwAuthority{c: c, f: f}
	n := &mwNode{f: f}
	w, err := NewManifest(c, a, n, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		return f.Auth, nil
	}, b)
	if err != nil {
		t.Fatal(err)
	}
	a.c.TransportTrustID = strings.Repeat("f", 64)
	if _, err = w.prepare(context.Background()); err == nil {
		t.Fatal("changed transport accepted")
	}
	a.c = c
	entry := mwFound(t, b, f)
	entry.BlockState = "submitted"
	if err = b.update(entry); err != nil {
		t.Fatal(err)
	}
	entry.BlockState = "canonical"
	if err = b.update(entry); err != nil {
		t.Fatal(err)
	}
	entry.Authorization = &f.Auth
	entry.AdmissionState = "pending"
	if err = b.update(entry); err != nil {
		t.Fatal(err)
	}
	a.submit = func() (prospectservice.ManifestAdmissionResult, error) {
		r := mwResult(f)
		r.Record.StaticBinding.PortraitDigest = strings.Repeat("f", 64)
		return r, nil
	}
	if _, err = w.deliver(context.Background(), &entry); !errors.Is(err, ErrCorrupt) {
		t.Fatal("forged adapter result accepted", err)
	}
	rows, _ := b.List(0, 1)
	if rows[0].Receipt != nil || rows[0].AdmissionState != "pending" {
		t.Fatal("forged receipt reached storage")
	}
}
