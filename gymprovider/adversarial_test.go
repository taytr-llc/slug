// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymprovider

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
)

// The Python expected indices existed before the journal implementation. A
// synthetic trust digest is an explicit input, not an expected ConfigDigest.
func TestIndependentProviderTupleIndices(t *testing.T) {
	raw, err := os.ReadFile("../gymidentity/testdata/coordinated/coordinated_identity_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Indices map[string]struct {
			Components []string `json:"components"`
			Encoded    string   `json:"encoded"`
			SHA256     string   `json:"sha256"`
		} `json:"tuple_indices"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Indices) != 5 {
		t.Fatal("missing independent tuple boundary vectors")
	}
	for name, fixture := range corpus.Indices {
		t.Run(name, func(t *testing.T) {
			domain := "original authentication"
			if name == "reset_operation" || name == "same_components_other_domain" {
				domain = "reset operation"
			}
			encoded, err := hex.DecodeString(fixture.Encoded)
			if err != nil || len(encoded) == 0 || len(fixture.Components) != 7 {
				t.Fatal("invalid independent tuple fixture")
			}
			if got := tupleIndex(domain, fixture.Components...); got != fixture.SHA256 {
				t.Fatal("provider index differs from independent domain/length/order encoding")
			}
		})
	}
}

func reopenProviderFixture(t *testing.T, f *providerFixture) {
	t.Helper()
	if err := f.Coordinator.Close(); err != nil && !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatal(err)
	}
	if err := f.Authority.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.Authority, err = gymaccount.OpenIdentity(filepath.Join(f.Root, "authority"), f.IdentityConfig)
	if err != nil {
		t.Fatal(err)
	}
	f.Coordinator, err = Open(context.Background(), filepath.Join(f.Root, "coordinator"), f.Config, f.Authority, f.Verifier)
	if err != nil {
		t.Fatal(err)
	}
}

func adversarialLogin(t *testing.T, f *providerFixture) (LoginFlow, string, int64, gymaccount.Session) {
	t.Helper()
	flow, err := f.Coordinator.IssueLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	origin := strings.Repeat("31", 32)
	auth := flow.IssuedAtMS / 1000 * 1000
	session, err := f.Coordinator.FinishLogin(context.Background(), flow.ID, signLogin(t, flow, "alice", origin, flow.ID, "fresh", 1, auth))
	if err != nil || session.Token == "" {
		t.Fatalf("setup login: %v", err)
	}
	return flow, origin, auth, session
}

func TestAdversarialOriginalAuthenticationCannotBeRelabeled(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	original, origin, auth, _ := adversarialLogin(t, f)
	for _, test := range []struct {
		name, subject, origin, mode string
		epoch                       uint64
		auth                        int64
	}{
		{"unknown-origin", "alice", strings.Repeat("42", 32), "reuse", 1, auth},
		{"changed-principal", "bob", origin, "reuse", 1, auth},
		{"changed-epoch", "alice", origin, "reuse", 2, auth},
		{"changed-auth-time", "alice", origin, "reuse", 1, auth - 1000},
		{"fresh-relabel", "alice", origin, "fresh", 1, auth},
	} {
		t.Run(test.name, func(t *testing.T) {
			flow, err := f.Coordinator.IssueLogin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			originalID := original.ID
			when := test.auth
			if test.mode == "fresh" {
				originalID = flow.ID
				when = flow.IssuedAtMS / 1000 * 1000
			}
			token := signLogin(t, flow, test.subject, test.origin, originalID, test.mode, test.epoch, when)
			// These are valid signed verifier outputs; the journal must reject
			// the substituted immutable origin, not rely on crypto failure.
			if _, err = f.Verifier.VerifyCoordinatedLogin(ctx, token, f.Coordinator.binding(), flow.ID, flow.Nonce); err != nil {
				t.Fatalf("mutation not verifier-valid: %v", err)
			}
			session, err := f.Coordinator.FinishLogin(ctx, flow.ID, token)
			if err == nil || session.Token != "" {
				t.Fatal("relabelled original authentication yielded a session")
			}
		})
	}
	reopenProviderFixture(t, f)
	flow, err := f.Coordinator.IssueLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := f.Coordinator.FinishLogin(ctx, flow.ID, signLogin(t, flow, "alice", origin, original.ID, "reuse", 1, auth))
	if err != nil || session.Token == "" {
		t.Fatalf("valid original origin was damaged by rejected mutations: %v", err)
	}
}

func TestAdversarialResetOperationAndCompletionBinding(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	adversarialLogin(t, f)
	request, err := f.Coordinator.IssueReset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.Coordinator.IssueReset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	op := strings.Repeat("53", 32)
	verified := time.Now().Unix() * 1000
	begin := signReset(t, request, "alice", op, "begin", 1, 2, verified)
	pending, err := f.Coordinator.BeginReset(ctx, request.EventID, begin)
	if err != nil || !pending.CanCommit || pending.Phase != "pending" {
		t.Fatalf("begin: %v", err)
	}
	if pending.Barrier.EventID == request.EventID {
		t.Fatal("raw event confused with authority's domain-separated event")
	}
	for _, test := range []struct {
		name                      string
		request                   ResetRequest
		subject, operation, stage string
		from, to                  uint64
		verified                  int64
	}{
		{"same-operation-other-handle", other, "alice", op, "begin", 1, 2, verified},
		{"same-handle-other-operation", request, "alice", strings.Repeat("64", 32), "begin", 1, 2, verified},
		{"completion-other-subject", request, "bob", op, "complete", 1, 2, verified},
		{"completion-other-time", request, "alice", op, "complete", 1, 2, verified - 1000},
		{"completion-other-epochs", request, "alice", op, "complete", 2, 3, verified},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := signReset(t, test.request, test.subject, test.operation, test.stage, test.from, test.to, test.verified)
			if _, err := f.Verifier.VerifyCoordinatedReset(ctx, token, f.Coordinator.binding(), test.request.EventID, test.request.Nonce); err != nil {
				t.Fatalf("mutation not verifier-valid: %v", err)
			}
			var got ResetResult
			var err error
			if test.stage == "begin" {
				got, err = f.Coordinator.BeginReset(ctx, test.request.EventID, token)
			} else {
				got, err = f.Coordinator.CompleteReset(ctx, test.request.EventID, token)
			}
			if err == nil || got != (ResetResult{}) {
				t.Fatal("mismatched immutable reset operation acknowledged")
			}
		})
	}
	complete := signReset(t, request, "alice", op, "complete", 1, 2, verified)
	done, err := f.Coordinator.CompleteReset(ctx, request.EventID, complete)
	if err != nil || done.CanCommit || done.Phase != "completed" {
		t.Fatalf("complete: %v", err)
	}
	reopenProviderFixture(t, f)
	replayed, err := f.Coordinator.BeginReset(ctx, request.EventID, begin)
	if err != nil || replayed.CanCommit || replayed.Barrier != done.Barrier {
		t.Fatalf("historical begin became permit or altered receipt: %v", err)
	}
	second, err := f.Coordinator.IssueReset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	secondTime := time.Now().Unix() * 1000
	secondOp := strings.Repeat("75", 32)
	if _, err = f.Coordinator.BeginReset(ctx, second.EventID, signReset(t, second, "alice", secondOp, "begin", 2, 3, secondTime)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Coordinator.CompleteReset(ctx, second.EventID, signReset(t, second, "alice", secondOp, "complete", 2, 3, secondTime)); err != nil {
		t.Fatal(err)
	}
	old, err := f.Coordinator.CompleteReset(ctx, request.EventID, complete)
	if err != nil || old.CanCommit || old.Barrier != done.Barrier {
		t.Fatalf("old completion changed after another reset: %v", err)
	}
}

func TestAdversarialExpiredTransportPersistsClockAcrossReopen(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	flow, err := f.Coordinator.IssueLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now().UnixMilli() + 1000
	f.Coordinator.now = func() time.Time { return time.UnixMilli(clock) }
	claims := providerLoginClaims(flow, "alice", strings.Repeat("86", 32), flow.ID, "fresh", 1, flow.IssuedAtMS)
	claims["iat"] = time.Now().Unix() - 10
	claims["exp"] = time.Now().Unix() - 1
	if session, err := f.Coordinator.FinishLogin(ctx, flow.ID, signProviderClaims(t, false, claims)); err == nil || session.Token != "" {
		t.Fatal("expired transport accepted")
	}
	reopenProviderFixture(t, f)
	f.Coordinator.now = func() time.Time { return time.UnixMilli(clock - 1) }
	token := signLogin(t, flow, "alice", strings.Repeat("86", 32), flow.ID, "fresh", 1, flow.IssuedAtMS)
	if session, err := f.Coordinator.FinishLogin(ctx, flow.ID, token); !errors.Is(err, ErrUnavailable) || session.Token != "" {
		t.Fatalf("reopen forgot rejected-transport clock observation: %v", err)
	}
}

func TestAdversarialCommittedWriteErrorNeverReturnsSession(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	flow, err := f.Coordinator.IssueLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	origin := strings.Repeat("97", 32)
	auth := flow.IssuedAtMS
	originalWrite := f.Coordinator.writeBatch
	armed := false
	f.Coordinator.checkpoint = func(stage string) {
		if stage == "authority-login" {
			armed = true
		}
	}
	f.Coordinator.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
		if err := originalWrite(batch, options); err != nil {
			return err
		}
		if armed {
			return errors.New("injected acknowledged storage uncertainty")
		}
		return nil
	}
	session, err := f.Coordinator.FinishLogin(ctx, flow.ID, signLogin(t, flow, "alice", origin, flow.ID, "fresh", 1, auth))
	if !errors.Is(err, ErrOutcomeUnknown) || session.Token != "" {
		t.Fatalf("uncertain final journal commit exposed a session: %v", err)
	}
	if _, err := f.Coordinator.Status(ctx, "login", flow.ID); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("uncertain handle not poisoned: %v", err)
	}
	reopenProviderFixture(t, f)
	if session, err := f.Coordinator.FinishLogin(ctx, flow.ID, signLogin(t, flow, "alice", origin, flow.ID, "fresh", 1, auth)); !errors.Is(err, ErrConsumed) || session.Token != "" {
		t.Fatalf("reconstructed a consumed session after uncertain ACK: %v", err)
	}
	status, err := f.Coordinator.Status(ctx, "login", flow.ID)
	if err != nil || status.Phase != "completed" {
		t.Fatalf("committed journal result not retained: %v", err)
	}
}

func corruptProviderJournal(t *testing.T, f *providerFixture, change func(*leveldb.DB)) {
	t.Helper()
	if err := f.Coordinator.Close(); err != nil {
		t.Fatal(err)
	}
	f.Coordinator = nil
	db, err := leveldb.OpenFile(filepath.Join(f.Root, "coordinator", "db"), &opt.Options{ErrorIfMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	change(db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(context.Background(), filepath.Join(f.Root, "coordinator"), f.Config, f.Authority, f.Verifier)
	if opened != nil {
		_ = opened.Close()
	}
	if !errors.Is(err, ErrCorrupt) || opened != nil {
		t.Fatalf("inconsistent durable journal reopened: %v", err)
	}
}

func auditReadRow(t *testing.T, db *leveldb.DB, key string, out any) {
	t.Helper()
	raw, err := db.Get([]byte(key), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatal(err)
	}
}
func auditWriteRow(t *testing.T, db *leveldb.DB, key string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Put([]byte(key), raw, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
}

func TestAdversarialReopenRejectsImpossibleSemanticTimes(t *testing.T) {
	t.Run("auth-after-clock", func(t *testing.T) {
		f := newProviderFixture(t)
		flow, _, _, _ := adversarialLogin(t, f)
		corruptProviderJournal(t, f, func(db *leveldb.DB) {
			var meta metadata
			var login loginRecord
			auditReadRow(t, db, "meta", &meta)
			auditReadRow(t, db, "login:"+flow.ID, &login)
			login.Semantic.AuthenticatedAtMS = (meta.ClockMS/1000 + 1) * 1000
			var origin originRecord
			auditReadRow(t, db, "origin:"+login.OriginKey, &origin)
			origin.Semantic = *login.Semantic // retain every cross-record binding
			auditWriteRow(t, db, "login:"+flow.ID, login)
			auditWriteRow(t, db, "origin:"+login.OriginKey, origin)
		})
	})
	for _, name := range []string{"verification-after-clock", "verification-after-request-expiry"} {
		t.Run(name, func(t *testing.T) {
			f := newProviderFixture(t)
			adversarialLogin(t, f)
			request, err := f.Coordinator.IssueReset(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.Coordinator.checkpoint = func(stage string) {
				if stage == "begin-prepared" {
					cancel()
				}
			}
			result, err := f.Coordinator.BeginReset(ctx, request.EventID, signReset(t, request, "alice", strings.Repeat("ca", 32), "begin", 1, 2, time.Now().Unix()*1000))
			if !errors.Is(err, context.Canceled) || result != (ResetResult{}) {
				t.Fatalf("failed to leave prepared-only test state: %v", err)
			}
			corruptProviderJournal(t, f, func(db *leveldb.DB) {
				var meta metadata
				var reset resetRecord
				auditReadRow(t, db, "meta", &meta)
				auditReadRow(t, db, "reset:"+request.EventID, &reset)
				if reset.Phase != "begin_prepared" || reset.Barrier != nil {
					t.Fatal("unexpected prepared fixture")
				}
				if name == "verification-after-clock" {
					reset.Semantic.VerifiedAtMS = (meta.ClockMS/1000 + 1) * 1000
				} else {
					reset.Semantic.VerifiedAtMS = (request.ExpiresAtMS/1000 + 1) * 1000
					meta.ClockMS = reset.Semantic.VerifiedAtMS // isolate expiry, not highwater
					auditWriteRow(t, db, "meta", meta)
				}
				auditWriteRow(t, db, "reset:"+request.EventID, reset)
			})
		})
	}
}
