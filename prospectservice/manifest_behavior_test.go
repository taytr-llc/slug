//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.

package prospectservice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

const mbToken = "abababababababababababababababababababababababababababababababab"

type mbCase struct {
	Job           prospectjob.ManifestJob `json:"job"`
	Proof         prospect.Proof          `json:"proof"`
	Authorization gymauth.Authorization   `json:"authorization"`
	Record        registry.ManifestRecord `json:"expected_record"`
}
type mbCorpus struct {
	Pin    registry.ManifestAuthorityPin `json:"pin"`
	Marker string                        `json:"marker_canonical"`
	Cases  []mbCase                      `json:"cases"`
	Rows   struct {
		Meta          string `json:"meta"`
		Jobs, Records []struct{ Key, Value string }
		Sequences     []struct {
			KeyHex string `json:"key_hex"`
			Value  string `json:"value"`
		}
	} `json:"database_rows"`
}

func mbLoad(t *testing.T) mbCorpus {
	t.Helper()
	raw, err := os.ReadFile("../registry/testdata/manifest/reference/manifest_admission_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f mbCorpus
	if err := json.Unmarshal(raw, &f); err != nil || len(f.Cases) != 5 {
		t.Fatal("independent fixture", err)
	}
	return f
}
func mbPolicy(c prospect.Context) prospectnode.Policy {
	return prospectnode.Policy{Version: 1, Network: c.Network, Genesis: c.Genesis, DNAVersion: c.DNAVersion, ProspectTarget: c.ProspectTarget}
}

type mbNode struct {
	initial          prospect.Context
	resolves, checks atomic.Int32
	resolve          func(context.Context) error
	guard            func(context.Context) error
	source           string
}

func (n *mbNode) Resolve(ctx context.Context, _ prospectnode.Policy) (prospect.Context, error) {
	n.resolves.Add(1)
	if n.resolve != nil {
		if err := n.resolve(ctx); err != nil {
			return prospect.Context{}, err
		}
	}
	return n.initial, nil
}
func (n *mbNode) RecheckContext(ctx context.Context, _ prospect.Context) error {
	n.checks.Add(1)
	if n.guard != nil {
		return n.guard(ctx)
	}
	return nil
}
func (n *mbNode) RecheckAdmission(ctx context.Context, _ prospect.Context, _ prospect.Proof, _ prospect.Verified) (string, error) {
	n.checks.Add(1)
	if n.guard != nil {
		if err := n.guard(ctx); err != nil {
			return "", err
		}
	}
	return n.source, nil
}
func mbConfig(pin registry.ManifestAuthorityPin, key string) ManifestServerConfig {
	raw, _ := hex.DecodeString(mbToken)
	sum := sha256.Sum256(raw)
	return ManifestServerConfig{Version: 3, Listen: "127.0.0.1:0", AuthorityPin: pin, Clients: []Credential{{PublicKey: key, TokenSHA256: hex.EncodeToString(sum[:])}}, MaxJobs: 10, MaxRecords: 10}
}
func mbSetup(t *testing.T) (*ManifestServer, *registry.ManifestStore, *mbNode, mbCorpus, string) {
	t.Helper()
	f := mbLoad(t)
	path := filepath.Join(t.TempDir(), "manifest")
	store, err := registry.CreateManifestAuthority(path, f.Pin.Network, f.Pin.Genesis, f.Pin.DescriptorID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	info, err := store.Info()
	if err != nil {
		t.Fatal(err)
	}
	node := &mbNode{initial: f.Cases[0].Job.Context, source: "current-parent"}
	s, err := NewManifestServer(mbConfig(info.Pin, f.Cases[0].Job.PublicKey), mbPolicy(node.initial), node, store)
	if err != nil {
		t.Fatal(err)
	}
	return s, store, node, f, path
}
func mbSeed(t *testing.T, f mbCorpus) (*registry.ManifestStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "retained")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SLUG-REGISTRY.json"), []byte(f.Marker), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfExist: true})
	if err != nil {
		t.Fatal(err)
	}
	b := new(leveldb.Batch)
	b.Put([]byte("meta"), []byte(f.Rows.Meta))
	for _, rows := range [][]struct{ Key, Value string }{f.Rows.Jobs, f.Rows.Records} {
		for _, r := range rows {
			b.Put([]byte(r.Key), []byte(r.Value))
		}
	}
	for _, r := range f.Rows.Sequences {
		k, err := hex.DecodeString(r.KeyHex)
		if err != nil {
			t.Fatal(err)
		}
		b.Put(k, []byte(r.Value))
	}
	if err := db.Write(b, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := registry.OpenManifestAuthority(path, f.Pin, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, path
}
func mbRequest(t *testing.T, h http.Handler, token, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	r.Host = "127.0.0.1:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func mbError(t *testing.T, w *httptest.ResponseRecorder, status int, code string, unknown bool) {
	t.Helper()
	var e ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || w.Code != status || e.Version != 3 || e.Error.Code != code || e.Error.OutcomeUnknown != unknown {
		t.Fatalf("got %d %s, want %d/%s/%v", w.Code, w.Body.String(), status, code, unknown)
	}
}
func mbIssue(t *testing.T, s *ManifestServer, key string) prospectjob.ManifestJob {
	t.Helper()
	w := mbRequest(t, s, mbToken, "/v3/jobs", map[string]string{"public_key": key})
	job, err := prospectjob.ParseManifestJob(w.Body.Bytes())
	if w.Code != 200 || err != nil {
		t.Fatal("issue", w.Body.String(), err)
	}
	return job
}
func mbProof(t *testing.T, f mbCase, job prospectjob.ManifestJob, nonce uint32) (prospect.Proof, gymauth.Authorization) {
	t.Helper()
	raw, _ := hex.DecodeString(f.Proof.Coinbase)
	var tx wire.MsgTx
	if err := tx.DeserializeNoWitness(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	script, err := prospectjob.ManifestCommitmentScript(job)
	if err != nil {
		t.Fatal(err)
	}
	tx.TxOut[len(tx.TxOut)-1].PkScript = script
	var cb bytes.Buffer
	if err := tx.SerializeNoWitness(&cb); err != nil {
		t.Fatal(err)
	}
	raw, _ = hex.DecodeString(f.Proof.Header)
	var h wire.BlockHeader
	if err := h.Deserialize(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	h.MerkleRoot = tx.TxHash()
	h.Nonce = nonce
	var hb bytes.Buffer
	if err := h.Serialize(&hb); err != nil {
		t.Fatal(err)
	}
	p := prospect.Proof{Version: 2, Header: hex.EncodeToString(hb.Bytes()), Coinbase: hex.EncodeToString(cb.Bytes()), MerkleBranch: []string{}, GymID: job.GymID, JobID: job.ID}
	key, _ := btcec.PrivKeyFromBytes([]byte{1})
	defer key.Zero()
	a, err := gymauth.SignAdmission(job.Context, p, key)
	if err != nil {
		t.Fatal(err)
	}
	return p, a
}
func mbSubmit(p prospect.Proof, a gymauth.Authorization) any {
	return struct {
		Proof         prospect.Proof        `json:"proof"`
		Authorization gymauth.Authorization `json:"authorization"`
	}{p, a}
}

func TestManifestBehaviorConstructorModesAndOwnership(t *testing.T) {
	s, store, node, _, path := mbSetup(t)
	cfg := s.config
	if _, err := NewManifestSessionServer(cfg, s.policy, node, store, &mbSessions{key: cfg.Clients[0].PublicKey}); err == nil {
		t.Fatal("session mode accepted static fallback")
	}
	cfg.Clients = nil
	if _, err := NewManifestServer(cfg, s.policy, node, store); err == nil {
		t.Fatal("static mode accepted empty credentials")
	}
	if _, err := NewManifestSessionServer(cfg, s.policy, node, store, nil); err == nil {
		t.Fatal("nil authorizer accepted")
	}
	if _, err := NewManifestSessionServer(cfg, s.policy, node, store, &mbSessions{key: s.config.Clients[0].PublicKey}); err != nil {
		t.Fatal(err)
	}
	wrong := s.config
	wrong.AuthorityPin.AuthorityID = strings.Repeat("00", 32)
	if _, err := NewManifestServer(wrong, s.policy, node, store); err == nil {
		t.Fatal("wrong exact pin accepted")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := registry.OpenManifestAuthority(path, s.config.AuthorityPin, true)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if _, err := NewManifestServer(s.config, s.policy, node, ro); err == nil {
		t.Fatal("read-only authority accepted")
	}
}

func TestManifestBehaviorAdmissionCapacityAndHistoricalPolicy(t *testing.T) {
	s, store, node, f, path := mbSetup(t)
	s.config.MaxJobs = 1
	s.config.MaxRecords = 1
	job := mbIssue(t, s, f.Cases[0].Job.PublicKey)
	mbError(t, mbRequest(t, s, mbToken, "/v3/jobs", map[string]string{"public_key": job.PublicKey}), 503, "capacity", false)
	p, a := mbProof(t, f.Cases[0], job, 0)
	w := mbRequest(t, s, mbToken, "/v3/admissions", mbSubmit(p, a))
	var first ManifestAdmissionResult
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil || w.Code != 200 || !first.Inserted || first.Record.Job != job || first.Record.StaticBinding.DescriptorID != s.config.AuthorityPin.DescriptorID {
		t.Fatal("admission", w.Body.String(), err)
	}
	p2, a2 := mbProof(t, f.Cases[0], job, 1)
	checks := node.checks.Load()
	mbError(t, mbRequest(t, s, mbToken, "/v3/admissions", mbSubmit(p2, a2)), 503, "capacity", false)
	if node.checks.Load() != checks {
		t.Fatal("capacity reached node")
	}
	changed := s.policy
	changed.ProspectTarget = strings.Repeat("e", 64)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := registry.OpenManifestAuthority(path, s.config.AuthorityPin, false)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	offline := &mbNode{initial: node.initial, resolve: func(context.Context) error { return errors.New("offline secret") }, guard: func(context.Context) error { return errors.New("offline secret") }}
	restarted, err := NewManifestServer(s.config, changed, offline, store)
	if err != nil {
		t.Fatal(err)
	}
	w = mbRequest(t, restarted, mbToken, "/v3/admissions", mbSubmit(p, a))
	var historical ManifestAdmissionResult
	if err := json.Unmarshal(w.Body.Bytes(), &historical); err != nil || w.Code != 200 || historical.Inserted || historical.CurrentNodeChecked || historical.ContextSource != "historical-receipt" || !reflect.DeepEqual(first.Record, historical.Record) {
		t.Fatal("historical receipt changed", w.Body.String(), err)
	}
	if offline.resolves.Load() != 0 || offline.checks.Load() != 0 {
		t.Fatal("historical recovery contacted node")
	}
}

func TestManifestBehaviorClientOwnsProofBeforeTransportWait(t *testing.T) {
	f := mbLoad(t).Cases[1] // An independent proof with a nonempty branch.
	p := f.Proof
	p.MerkleBranch = append([]string(nil), p.MerkleBranch...)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	resume := func() { once.Do(func() { close(release) }) }
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ManifestAdmissionResult{Version: 3,
			ContextSource: "historical-receipt", Authentication: "gym-key-control",
			BirthRule: "job-issuance", Record: f.Record})
	}))
	defer h.Close()
	defer resume()
	pin := registry.ManifestAuthorityPin{Version: 5, Network: f.Job.Context.Network,
		Genesis: f.Job.Context.Genesis, AuthorityID: f.Job.AuthorityID, DescriptorID: f.Job.DescriptorID}
	c, err := NewManifestClient(ManifestClientConfig{Version: 3, URL: h.URL, Token: mbToken, AuthorityPin: pin}, mbPolicy(f.Job.Context))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	done := make(chan error, 1)
	go func() { _, err := c.SubmitManifest(context.Background(), f.Job, p, f.Authorization); done <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("transport was not reached")
	}
	p.MerkleBranch[0] = strings.Repeat("00", 32)
	resume()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("caller branch changed owned evidence", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("submission did not drain")
	}
}

// This lease double models serialization and expiring authorization, not an
// account authority. Durable reset behavior belongs to gymaccount integration.
type mbSessions struct {
	mu             sync.Mutex
	key            string
	calls          atomic.Int32
	revoked        atomic.Bool
	denyFinal      bool
	afterOperation error
}

func (a *mbSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	call := a.calls.Add(1)
	checks := 0
	check := func() error {
		checks++
		if token != mbToken || a.revoked.Load() || ctx.Err() != nil || (a.denyFinal && call == 2 && checks >= 3) {
			return errors.New("private lease rejected")
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	if err := fn(a.key, check); err != nil {
		return err
	}
	if call == 2 && a.afterOperation != nil {
		return a.afterOperation
	}
	return nil
}
func mbSessionServer(t *testing.T, s *ManifestServer, store *registry.ManifestStore, node ManifestNode, a SessionAuthorizer) *ManifestServer {
	t.Helper()
	cfg := s.config
	cfg.Clients = nil
	r, err := NewManifestSessionServer(cfg, s.policy, node, store, a)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestManifestBehaviorFinalLeaseAndUnknownCommit(t *testing.T) {
	for _, mode := range []string{"expiry-at-guard", "cancel-at-guard", "lease-error-after-commit"} {
		t.Run(mode, func(t *testing.T) {
			s, store, node, f, _ := mbSetup(t)
			access := &mbSessions{key: f.Cases[0].Job.PublicKey}
			s = mbSessionServer(t, s, store, node, access)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "expiry-at-guard" {
				node.guard = func(context.Context) error { access.revoked.Store(true); return nil }
			}
			if mode == "cancel-at-guard" {
				node.guard = func(context.Context) error { cancel(); return nil }
			}
			if mode == "lease-error-after-commit" {
				access.afterOperation = errors.New("sensitive outer lease error")
			}
			r := httptest.NewRequest(http.MethodPost, "/v3/jobs", strings.NewReader(`{"public_key":"`+access.key+`"}`)).WithContext(ctx)
			r.Host = "127.0.0.1:12345"
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+mbToken)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			info, err := store.Info()
			if err != nil {
				t.Fatal(err)
			}
			if node.resolves.Load() != 1 || node.checks.Load() != 1 {
				t.Fatal("issuance did not reach its final guard")
			}
			switch mode {
			case "expiry-at-guard":
				mbError(t, w, 401, "unauthorized", false)
				if info.Jobs != 0 {
					t.Fatal("expired lease wrote")
				}
			case "cancel-at-guard":
				mbError(t, w, 503, "unavailable", false)
				if info.Jobs != 0 {
					t.Fatal("canceled guard wrote")
				}
			default:
				mbError(t, w, 503, "outcome_unknown", true)
				if info.Jobs != 1 {
					t.Fatal("test did not lose actual durable acknowledgment")
				}
			}
		})
	}
}

func TestManifestBehaviorHistoricalFinalLeaseRefusal(t *testing.T) {
	f := mbLoad(t)
	store, _ := mbSeed(t, f)
	node := &mbNode{initial: f.Cases[0].Job.Context, source: "current-parent"}
	cfg := mbConfig(f.Pin, f.Cases[0].Job.PublicKey)
	cfg.Clients = nil
	access := &mbSessions{key: f.Cases[0].Job.PublicKey, denyFinal: true}
	s, err := NewManifestSessionServer(cfg, mbPolicy(node.initial), node, store, access)
	if err != nil {
		t.Fatal(err)
	}
	w := mbRequest(t, s, mbToken, "/v3/admissions", mbSubmit(f.Cases[0].Proof, f.Cases[0].Authorization))
	mbError(t, w, 401, "unauthorized", false)
	if strings.Contains(w.Body.String(), f.Cases[0].Record.Verified.ProspectID) || node.checks.Load() != 0 || node.resolves.Load() != 0 {
		t.Fatal("revoked historical read disclosed receipt or checked node")
	}
}

func TestManifestBehaviorRevocationBeforeOperationAndCrossGym(t *testing.T) {
	s, store, node, f, _ := mbSetup(t)
	access := &mbSessions{key: f.Cases[0].Job.PublicKey}
	s = mbSessionServer(t, s, store, node, access)
	body := &revokeOnRead{Reader: strings.NewReader(`{"public_key":"` + access.key + `"}`), f: func() { access.revoked.Store(true) }}
	r := httptest.NewRequest(http.MethodPost, "/v3/jobs", body)
	r.Host = "127.0.0.1:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+mbToken)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	mbError(t, w, 401, "unauthorized", false)
	access.revoked.Store(false)
	mbError(t, mbRequest(t, s, mbToken, "/v3/admissions", mbSubmit(f.Cases[3].Proof, f.Cases[3].Authorization)), 403, "forbidden", false)
	info, err := store.Info()
	if err != nil || info.Jobs != 0 || info.Records != 0 || node.resolves.Load() != 0 || node.checks.Load() != 0 {
		t.Fatal("unauthorized work reached state", err)
	}
}

func TestManifestBehaviorAdmissionGuardRefusals(t *testing.T) {
	for _, mode := range []string{"stale-or-side-winner", "session-expired", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			s, store, node, f, _ := mbSetup(t)
			job := mbIssue(t, s, f.Cases[0].Job.PublicKey)
			p, a := mbProof(t, f.Cases[0], job, 0)
			checksBefore := node.checks.Load()
			access := &mbSessions{key: job.PublicKey}
			s = mbSessionServer(t, s, store, node, access)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			node.guard = func(context.Context) error {
				switch mode {
				case "session-expired":
					access.revoked.Store(true)
				case "cancelled":
					cancel()
				default:
					return errors.New("exact canonical membership refused")
				}
				return nil
			}
			raw, _ := json.Marshal(mbSubmit(p, a))
			r := httptest.NewRequest(http.MethodPost, "/v3/admissions", bytes.NewReader(raw)).WithContext(ctx)
			r.Host = "127.0.0.1:12345"
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+mbToken)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if node.checks.Load() != checksBefore+1 {
				t.Fatal("admission did not reach its final guard")
			}
			if mode == "session-expired" {
				mbError(t, w, 401, "unauthorized", false)
			} else {
				mbError(t, w, 503, "unavailable", false)
			}
			info, err := store.Info()
			if err != nil || info.Records != 0 {
				t.Fatal("rejected guard committed receipt", err)
			}
		})
	}
}

func TestManifestBehaviorClientCloseDoesNotCancelActiveIssue(t *testing.T) {
	f := mbLoad(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		once.Do(func() { close(entered) })
		<-release
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.Cases[0].Job)
	}))
	defer h.Close()
	c, err := NewManifestClient(ManifestClientConfig{Version: 3, URL: h.URL, Token: mbToken, AuthorityPin: f.Pin}, mbPolicy(f.Cases[0].Job.Context))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := c.IssueManifestJob(context.Background(), f.Cases[0].Job.PublicKey); done <- err }()
	<-entered
	c.Close()
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("Close cancelled active operation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("active operation did not finish")
	}
}
