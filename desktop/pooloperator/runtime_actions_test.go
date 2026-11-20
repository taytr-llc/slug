//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/poolminer"
	"github.com/taytr-llc/slug/registry"
)

// This fixture constructs real partial owners only: outer intent/files, online
// synthetic authority, miner journal, issuer and ownership journal. It creates
// no READY, wallet, node, listener, scheduler or fake verified/funded state.
// These tests qualify local action/lifetime composition, not Create/Open/Run.
func opActionTestRuntime(t *testing.T) (*Runtime, string) {
	t.Helper()
	ctx := context.Background()
	setup := opSetupTestReference(t).Positives[0].Setup
	profile, e := registry.NewOperatorProfile(setup.Authority)
	if e != nil {
		t.Fatal(e)
	}
	binding, e := authorityBinding(profile)
	if e != nil {
		t.Fatal(e)
	}
	secrets := opSetupTestDir(t)
	root := filepath.Join(opSetupTestDir(t), "operator")
	paths := map[string]string{}
	for _, role := range []string{"admission", "publication", "cutoff"} {
		// Published independent synthetic recipes. No checkpoint seed is selected.
		seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
		path := filepath.Join(secrets, role)
		paths[role] = path
		if e = os.WriteFile(path, seed[:], 0600); e != nil {
			t.Fatal(e)
		}
	}
	owner, e := operatorauthority.OpenOnline(ctx, binding, operatorauthority.OnlineSeedFiles{Admission: paths["admission"], Publication: paths["publication"], Cutoff: paths["cutoff"]})
	if owner != nil {
		t.Cleanup(func() {
			if e := owner.Close(); e != nil {
				t.Error(e)
			}
		})
	}
	if e != nil {
		t.Fatal(e)
	}
	life, cancel := context.WithCancel(ctx)
	id, e := SetupID(setup)
	if e != nil {
		t.Fatal(e)
	}
	r := &Runtime{setup: setup, setupID: id, profile: profile, binding: binding, borrowed: BorrowedOwners{Authority: owner}, life: life, cancel: cancel, runDone: make(chan struct{}), localGate: make(chan struct{}, 1)}
	t.Cleanup(func() {
		if e := r.Close(); e != nil {
			t.Error(e)
		}
	})
	r.files, e = openOperatorFiles(root, setup, true)
	if e != nil {
		t.Fatal(e)
	}
	for _, item := range []struct {
		name string
		dst  **privateInput
	}{{"certificate", &r.tlsCertificate}, {"tls-key", &r.tlsKey}, {"unlock", &r.unlockInput}} {
		path := filepath.Join(secrets, item.name)
		if e = os.WriteFile(path, []byte("public synthetic local-action input"), 0600); e != nil {
			t.Fatal(e)
		}
		*item.dst, e = selectPrivateInput(ctx, path, 1024)
		if e != nil {
			t.Fatal(e)
		}
	}
	path, fresh, e := r.files.child("miners")
	if e != nil || !fresh {
		t.Fatal(e)
	}
	r.miners, e = poolminer.Create(ctx, path, setup.Miners, setup.minerBinding(profile))
	if e != nil {
		t.Fatal(e)
	}
	if e = r.files.adopt("miners"); e != nil {
		t.Fatal(e)
	}
	path, fresh, e = r.files.child("issuer")
	if e != nil || !fresh {
		t.Fatal(e)
	}
	descriptor, e := fighterdescriptor.ResolveCandidate(setup.Authority.Source.DescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	r.issuer, e = registry.CreateOperatorManifestAuthority(path, descriptor, profile, owner)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.files.adopt("issuer"); e != nil {
		t.Fatal(e)
	}
	r.publisher, e = r.issuer.OperatorPublication(profile)
	if e != nil {
		t.Fatal(e)
	}
	path, fresh, e = r.files.child("ownership")
	if e != nil || !fresh {
		t.Fatal(e)
	}
	r.ownership, e = fighterownership.CreateOperator(path, profile, fighterownership.StoreIdentity{StoreID: setup.OwnershipStoreID}, owner)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.files.adopt("ownership"); e != nil {
		t.Fatal(e)
	}
	if e = r.checkInputs(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Lstat(filepath.Join(root, readyName)); !os.IsNotExist(e) {
		t.Fatal("partial fixture must not publish READY", e)
	}
	return r, root
}
func opActionTestCheckpoint(t *testing.T) chaincfg.SignedCheckpoint {
	t.Helper()
	f, e := os.Open("../../registry/testdata/operator-reference/authority-v2.json")
	if e != nil {
		t.Fatal(e)
	}
	raw, e := io.ReadAll(io.LimitReader(f, 1<<20))
	ce := f.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
	sum := sha256.Sum256(raw)
	if len(raw) != 622582 || hex.EncodeToString(sum[:]) != "7328ea2102c9ac993281bd732590c3e734eda26a3d180c2fa98bbd37da02c6b4" {
		t.Fatal("independent authority input changed")
	}
	var corpus struct {
		Positives []struct {
			Family, Name string
			Value        json.RawMessage
		}
	}
	if e = json.Unmarshal(raw, &corpus); e != nil {
		t.Fatal(e)
	}
	for _, row := range corpus.Positives {
		if row.Family == "checkpoint" && row.Name == "v1/operator_v2_distinct" {
			var cp chaincfg.SignedCheckpoint
			if e = json.Unmarshal(row.Value, &cp); e != nil {
				t.Fatal(e)
			}
			return cp
		}
	}
	t.Fatal("independent signed checkpoint missing")
	return chaincfg.SignedCheckpoint{}
}
func TestOperatorRuntimeCredentialActions(t *testing.T) {
	r, _ := opActionTestRuntime(t)
	ctx := context.Background()
	gym := r.setup.Miners.Enrollments[0].GymID
	token, otherToken := strings.Repeat("71", 32), strings.Repeat("72", 32)
	expires := time.Now().Add(time.Hour).UnixMilli()
	credential, e := r.RotateCredential(ctx, gym, poolminer.PoolAudience, 0, token, expires)
	if e != nil || credential.Generation != 1 || credential.GymID != gym || credential.Audience != poolminer.PoolAudience || credential.ExpiresAtMS != expires {
		t.Fatal("original rotation", credential, e)
	}
	retry, e := r.RotateCredential(ctx, gym, poolminer.PoolAudience, 0, token, expires)
	if e != nil || retry != credential {
		t.Fatal("exact lost-ack rotation", e)
	}
	if _, e = r.RotateCredential(ctx, gym, poolminer.PoolAudience, 0, otherToken, expires); !errors.Is(e, poolminer.ErrConflict) {
		t.Fatal("changed request reused old generation", e)
	}
	entered := false
	e = r.miners.PoolSessions().WithSession(ctx, token, func(pub string, check func() error) error {
		entered = true
		if pub != r.setup.Miners.Enrollments[0].PublicKey {
			t.Error("wrong original enrollment")
		}
		return check()
	})
	if e != nil || !entered {
		t.Fatal("original credential rejected", e)
	}
	e = r.miners.AdmissionSessions().WithSession(ctx, token, func(string, func() error) error { t.Error("wrong audience dispatched"); return nil })
	if !errors.Is(e, poolminer.ErrUnauthorized) {
		t.Fatal("cross-audience credential", e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = r.RevokeCredential(canceled, gym, poolminer.PoolAudience, 1); !errors.Is(e, context.Canceled) {
		t.Fatal("canceled revoke", e)
	}
	if e = r.miners.PoolSessions().WithSession(ctx, token, func(string, func() error) error { return nil }); e != nil {
		t.Fatal("canceled revoke mutated credential", e)
	}
	revoked, e := r.RevokeCredential(ctx, gym, poolminer.PoolAudience, 1)
	if e != nil || revoked.Generation != 1 || revoked.RevokedAtMS == 0 {
		t.Fatal("revoke", e)
	}
	again, e := r.RevokeCredential(ctx, gym, poolminer.PoolAudience, 1)
	if e != nil || again != revoked {
		t.Fatal("exact revoke recovery", e)
	}
	if e = r.miners.PoolSessions().WithSession(ctx, token, func(string, func() error) error { t.Error("revoked token dispatched"); return nil }); !errors.Is(e, poolminer.ErrUnauthorized) {
		t.Fatal("revoked credential admitted", e)
	}
}
func TestOperatorRuntimeCheckpointImportNoHead(t *testing.T) {
	r, root := opActionTestRuntime(t)
	ctx := context.Background()
	cp := opActionTestCheckpoint(t)
	if r.borrowed.Authority.Capabilities().Checkpoint {
		t.Fatal("online checkpoint custody")
	}
	bad := cp
	bad.Signature = append([]byte(nil), cp.Signature...)
	bad.Signature[0] ^= 1
	if _, e := r.ImportCheckpoint(ctx, bad); !errors.Is(e, ErrInvalid) {
		t.Fatal("wrong signature imported", e)
	}
	before, e := r.publisher.Status(ctx)
	if e != nil || before.PublicationSequence != 0 {
		t.Fatal("invalid input mutated publication", e)
	}
	entry, e := r.ImportCheckpoint(ctx, cp)
	if e != nil || entry.Kind != "checkpoint" || entry.Sequence != 1 {
		t.Fatal("public checkpoint import", e)
	}
	encoded, e := json.Marshal(entry)
	if e != nil {
		t.Fatal(e)
	}
	// The independently signed envelope is preserved, not a current chain claim.
	var payload struct {
		Checkpoint chaincfg.SignedCheckpoint `json:"checkpoint"`
	}
	if e = json.Unmarshal(entry.Payload, &payload); e != nil || !reflect.DeepEqual(payload.Checkpoint, cp) {
		t.Fatal("checkpoint bytes changed", e)
	}
	if _, e = r.ownership.Head(ctx); !errors.Is(e, fighterownership.ErrUnavailable) {
		t.Fatal("public import granted Head", e)
	}
	entry.Payload[0] ^= 1
	retry, e := r.ImportCheckpoint(ctx, cp)
	if e != nil {
		t.Fatal(e)
	}
	retryBytes, e := json.Marshal(retry)
	if e != nil || !bytes.Equal(encoded, retryBytes) {
		t.Fatal("duplicate changed retained signed entry", e)
	}
	after, e := r.publisher.Status(ctx)
	if e != nil || after.PublicationSequence != 1 {
		t.Fatal("duplicate appended publication", e)
	}
	owner, profile := r.borrowed.Authority, r.profile
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	if e = owner.Check(ctx, r.binding); e != nil {
		t.Fatal("runtime closed borrowed authority", e)
	}
	issuer, e := registry.OpenOperatorManifestAuthority(filepath.Join(root, "issuer"), profile, owner)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := issuer.Close(); e != nil {
			t.Error(e)
		}
	}()
	pub, e := issuer.OperatorPublication(profile)
	if e != nil {
		t.Fatal(e)
	}
	reopened, e := pub.PublishCheckpoint(ctx, cp)
	if e != nil {
		t.Fatal(e)
	}
	got, e := json.Marshal(reopened)
	if e != nil || !bytes.Equal(encoded, got) {
		t.Fatal("checkpoint exact retry after reopen", e)
	}
}
func TestOperatorRuntimeCloseJoinsAdmittedLocalAction(t *testing.T) {
	r, root := opActionTestRuntime(t)
	ctx := context.Background()
	action, end, e := r.beginAction(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var once sync.Once
	release := func() { once.Do(end) }
	closed := make(chan error, 1)
	defer release()
	if _, e = r.RotateCredential(ctx, r.setup.Miners.Enrollments[0].GymID, poolminer.PoolAudience, 0, strings.Repeat("73", 32), time.Now().Add(time.Hour).UnixMilli()); !errors.Is(e, ErrActionBusy) {
		t.Fatal("local action reentered original gate", e)
	}
	go func() { closed <- r.Close() }()
	t.Cleanup(func() {
		release()
		select {
		case e := <-closed:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(5 * time.Second):
			t.Error("Close did not join")
		}
	})
	select {
	case <-action.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not cancel admitted action")
	}
	select {
	case e := <-closed:
		closed <- e
		t.Fatal("Close returned before admitted action ended")
	default:
	}
	if _, _, e = r.beginAction(ctx); !errors.Is(e, ErrUnavailable) {
		t.Fatal("closing runtime admitted action", e)
	}
	// The original root stays locked while an admitted action is joining.
	f, e := openOperatorFiles(root, r.setup, true)
	if f != nil {
		if ce := f.close(); ce != nil {
			t.Fatal(ce)
		}
	}
	if e == nil {
		t.Fatal("root released before action joined")
	}
	release()
	// Cleanup receives the result and joins the exact close goroutine.
}
func TestOperatorRuntimeOriginalPrivateInputAndOwner(t *testing.T) {
	t.Run("selected file metadata changed", func(t *testing.T) {
		r, _ := opActionTestRuntime(t)
		ctx := context.Background()
		input := r.unlockInput
		original := input.file
		before, e := r.miners.Snapshot(ctx)
		if e != nil {
			t.Fatal(e)
		}
		// Mode mutation is observable on the original selected descriptor; no path
		// replacement/reselection can make it an authorized new input.
		if e = original.Chmod(0644); e != nil {
			t.Fatal(e)
		}
		defer func() {
			if e := original.Chmod(0600); e != nil {
				t.Error(e)
			}
		}()
		if _, e = r.RotateCredential(ctx, r.setup.Miners.Enrollments[0].GymID, poolminer.PoolAudience, 0, strings.Repeat("74", 32), time.Now().Add(time.Hour).UnixMilli()); e == nil {
			t.Fatal("changed private input admitted mutation")
		}
		after, e := r.miners.Snapshot(ctx)
		if e != nil || len(after.Credentials) != len(before.Credentials) || len(after.History) != len(before.History) {
			t.Fatal("changed input mutated credentials", e)
		}
	})
	t.Run("original authority canceled", func(t *testing.T) {
		r, _ := opActionTestRuntime(t)
		ctx := context.Background()
		if e := r.borrowed.Authority.Close(); e != nil {
			t.Fatal(e)
		}
		if _, e := r.RotateCredential(ctx, r.setup.Miners.Enrollments[0].GymID, poolminer.PoolAudience, 0, strings.Repeat("75", 32), time.Now().Add(time.Hour).UnixMilli()); e == nil {
			t.Fatal("closed original authority admitted mutation")
		}
		state, e := r.miners.Snapshot(ctx)
		if e != nil || len(state.Credentials) != 0 {
			t.Fatal("closed owner mutated original journal", e)
		}
	})
}
