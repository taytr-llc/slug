//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectservice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type srvFixture struct {
	Job           prospectjob.Job       `json:"job"`
	Context       prospect.Context      `json:"context"`
	Proof         prospect.Proof        `json:"proof"`
	Authorization gymauth.Authorization `json:"authorization"`
}

func srvLoadFixture(t *testing.T) srvFixture {
	t.Helper()
	raw, err := os.ReadFile("../prospectjob/testdata/jobs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []srvFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil || len(fixtures) == 0 {
		t.Fatalf("independent fixture: %v", err)
	}
	return fixtures[0]
}

type srvNode struct {
	context  prospect.Context
	failed   atomic.Bool
	resolves atomic.Int32
	checks   atomic.Int32
}

func (node *srvNode) Resolve(ctx context.Context, policy prospectnode.Policy) (prospect.Context, error) {
	node.resolves.Add(1)
	if node.failed.Load() {
		return prospect.Context{}, errors.New("backend secret must never appear")
	}
	return node.context, nil
}
func (node *srvNode) RecheckContext(ctx context.Context, initial prospect.Context) error {
	node.checks.Add(1)
	if node.failed.Load() {
		return errors.New("backend secret must never appear")
	}
	return nil
}
func (node *srvNode) Recheck(ctx context.Context, initial prospect.Context, proof prospect.Proof) error {
	return node.RecheckContext(ctx, initial)
}

func srvSetup(t *testing.T) (*Server, *registry.Store, *srvNode, srvFixture, string) {
	t.Helper()
	f := srvLoadFixture(t)
	store, err := registry.CreateBirth(filepath.Join(t.TempDir(), "birth"), f.Context.Network, f.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	// Explicit public test credential, unrelated to wallet/league keys.
	token := strings.Repeat("23", 32)
	decoded, _ := hex.DecodeString(token)
	digest := sha256.Sum256(decoded)
	config := ServerConfig{Version: 1, Listen: "127.0.0.1:0", MaxJobs: 10, MaxRecords: 10,
		Clients: []Credential{{PublicKey: f.Job.PublicKey, TokenSHA256: hex.EncodeToString(digest[:])}}}
	policy := prospectnode.Policy{Version: 1, Network: f.Context.Network, Genesis: f.Context.Genesis,
		DNAVersion: f.Context.DNAVersion, ProspectTarget: f.Context.ProspectTarget}
	node := &srvNode{context: f.Context}
	server, err := NewServer(config, policy, node, store)
	if err != nil {
		t.Fatal(err)
	}
	return server, store, node, f, token
}

func srvRequest(t *testing.T, server *Server, token, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	request.Host = "127.0.0.1:29334"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

func srvError(t *testing.T, response *httptest.ResponseRecorder, status int, code string, unknown bool) {
	t.Helper()
	var failure ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || response.Code != status || failure.Version != 1 ||
		failure.Error.Code != code || failure.Error.OutcomeUnknown != unknown {
		t.Fatalf("HTTP %d %s, want %d/%s/unknown=%v (%v)", response.Code, response.Body.String(), status, code, unknown, err)
	}
}

func srvProof(t *testing.T, f srvFixture, job prospectjob.Job, nonce uint32) (prospect.Proof, gymauth.Authorization) {
	t.Helper()
	raw, err := hex.DecodeString(f.Proof.Coinbase)
	if err != nil {
		t.Fatal(err)
	}
	var coinbase wire.MsgTx
	if err := coinbase.DeserializeNoWitness(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	script, err := prospectjob.CommitmentScript(job)
	if err != nil {
		t.Fatal(err)
	}
	coinbase.TxOut[len(coinbase.TxOut)-1].PkScript = script
	var tx bytes.Buffer
	if err := coinbase.SerializeNoWitness(&tx); err != nil {
		t.Fatal(err)
	}
	headerRaw, err := hex.DecodeString(f.Proof.Header)
	if err != nil {
		t.Fatal(err)
	}
	var header wire.BlockHeader
	if err := header.Deserialize(bytes.NewReader(headerRaw)); err != nil {
		t.Fatal(err)
	}
	header.MerkleRoot = coinbase.TxHash()
	header.Nonce = nonce
	var encoded bytes.Buffer
	if err := header.Serialize(&encoded); err != nil {
		t.Fatal(err)
	}
	proof := prospect.Proof{Version: 2, GymID: job.GymID, JobID: job.ID, Header: hex.EncodeToString(encoded.Bytes()),
		Coinbase: hex.EncodeToString(tx.Bytes()), MerkleBranch: []string{}}
	private, _ := btcec.PrivKeyFromBytes([]byte{1}) // Published fixture scalar only.
	authorization, err := gymauth.SignAdmission(job.Context, proof, private)
	if err != nil {
		t.Fatal(err)
	}
	return proof, authorization
}

func TestServerJobAdmissionAndHistoricalCapacity(t *testing.T) {
	server, store, node, f, token := srvSetup(t)
	server.config.MaxRecords = 1 // Fixed before the first operation.
	issued := srvRequest(t, server, token, "/v1/jobs", map[string]any{"public_key": f.Job.PublicKey})
	if issued.Code != 200 {
		t.Fatalf("issue failed: %s", issued.Body.String())
	}
	var job prospectjob.Job
	if err := json.Unmarshal(issued.Body.Bytes(), &job); err != nil || prospectjob.Validate(job) != nil || job.PublicKey != f.Job.PublicKey {
		t.Fatalf("invalid issued job: %v", err)
	}
	if job.ExpiresAtMS-job.IssuedAtMS != 300000 || node.resolves.Load() != 1 || node.checks.Load() != 1 {
		t.Fatal("issuance skipped D68 or node guards")
	}
	proof, authorization := srvProof(t, f, job, 0)
	body := map[string]any{"proof": proof, "authorization": authorization}
	response := srvRequest(t, server, token, "/v1/admissions", body)
	if response.Code != 200 {
		t.Fatalf("admission failed: %s", response.Body.String())
	}
	var first AdmissionResult
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if !first.Inserted || !first.CurrentNodeChecked || first.ContextSource != "trusted-node-snapshot" ||
		first.Authentication != "gym-key-control" || first.BirthRule != "job-issuance" ||
		first.Record.Version != 3 || first.Record.Sequence != 1 || first.Record.Job == nil || *first.Record.Job != job ||
		first.Record.Birth == nil || first.Record.Birth.BornAtMS != job.IssuedAtMS {
		t.Fatalf("wrong new receipt: %+v", first)
	}
	second, secondAuth := srvProof(t, f, job, 1)
	checks := node.checks.Load()
	srvError(t, srvRequest(t, server, token, "/v1/admissions", map[string]any{"proof": second, "authorization": secondAuth}), 503, "capacity", false)
	if node.checks.Load() != checks {
		t.Fatal("capacity failure contacted node")
	}
	// A restart with a changed operator target and unavailable node must not
	// block recovery of the original receipt at the durable record limit.
	server.stopAdmission()
	changedPolicy := server.policy
	changedPolicy.ProspectTarget = strings.Repeat("e", 64)
	restarted, err := NewServer(server.config, changedPolicy, node, store)
	if err != nil {
		t.Fatal(err)
	}
	node.failed.Store(true)
	response = srvRequest(t, restarted, token, "/v1/admissions", body)
	var historical AdmissionResult
	if err := json.Unmarshal(response.Body.Bytes(), &historical); err != nil || response.Code != 200 {
		t.Fatalf("historical recovery failed: %s", response.Body.String())
	}
	if historical.Inserted || historical.CurrentNodeChecked || historical.ContextSource != "historical-receipt" ||
		!reflect.DeepEqual(historical.Record, first.Record) || node.checks.Load() != checks {
		t.Fatal("historical recovery changed evidence or consulted current node")
	}
	info, err := store.Info()
	if err != nil || info.Jobs != 1 || info.Records != 1 {
		t.Fatal("capacity/replay changed counts")
	}
}

func TestServerConstructorRejectsWrongStoreAndCopiesConfiguration(t *testing.T) {
	server, store, node, f, _ := srvSetup(t)
	legacy, err := registry.Create(filepath.Join(t.TempDir(), "legacy"), f.Context.Network, f.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, err := NewServer(server.config, server.policy, node, legacy); err == nil {
		t.Fatal("accepted legacy evidence store")
	}
	foreign, err := registry.CreateBirth(filepath.Join(t.TempDir(), "foreign"), f.Context.Network+1, f.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	if _, err := NewServer(server.config, server.policy, node, foreign); err == nil {
		t.Fatal("accepted different chain")
	}
	if _, err := NewServer(server.config, server.policy, nil, store); err == nil {
		t.Fatal("accepted missing node")
	}
	if _, err := NewServer(server.config, server.policy, node, nil); err == nil {
		t.Fatal("accepted missing store")
	}
	readonlyPath := filepath.Join(t.TempDir(), "readonly")
	writable, err := registry.CreateBirth(readonlyPath, f.Context.Network, f.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	if err := writable.Close(); err != nil {
		t.Fatal(err)
	}
	readonly, err := registry.Open(readonlyPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	if _, err := NewServer(server.config, server.policy, node, readonly); err == nil {
		t.Fatal("accepted read-only store")
	}
	config := server.config
	copyServer, err := NewServer(config, server.policy, node, store)
	if err != nil {
		t.Fatal(err)
	}
	config.Clients[0].PublicKey = strings.Repeat("0", 66)
	if copyServer.clients[0].publicKey != f.Job.PublicKey || copyServer.config.Clients[0].PublicKey != f.Job.PublicKey {
		t.Fatal("caller mutated server credentials")
	}
}

func TestServerBearerHexCaseIdentifiesSameCredential(t *testing.T) {
	server, _, _, _, _ := srvSetup(t)
	token := strings.Repeat("ab", 32)
	raw, _ := hex.DecodeString(token)
	server.clients[0].digest = sha256.Sum256(raw)
	for _, encoded := range []string{token, strings.ToUpper(token)} {
		request := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
		request.Header.Set("Authorization", "Bearer "+encoded)
		if server.authenticate(request) != &server.clients[0] {
			t.Fatal("equivalent hex spelling failed to authenticate")
		}
	}
}

func TestServerSanitizedErrorsAndPoisonLatch(t *testing.T) {
	server, _, node, f, token := srvSetup(t)
	node.failed.Store(true)
	response := srvRequest(t, server, token, "/v1/jobs", map[string]any{"public_key": f.Job.PublicKey})
	srvError(t, response, 503, "unavailable", false)
	if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), token) {
		t.Fatal("backend error or credential exposed")
	}
	// The latch is independent of subsequent Store state: an uncertain write
	// cannot be bypassed by another operation without a server restart.
	failure := server.storeFailure(registry.ErrPoisoned)
	if failure.code != "outcome_unknown" || !failure.unknown {
		t.Fatal("uncertain write lost outcome classification")
	}
	before := node.resolves.Load()
	srvError(t, srvRequest(t, server, token, "/v1/jobs", map[string]any{"public_key": f.Job.PublicKey}), 503, "outcome_unknown", true)
	if node.resolves.Load() != before {
		t.Fatal("poisoned authority attempted another operation")
	}
}

func TestServerUnknownJobAndFrozenPolicy(t *testing.T) {
	server, _, node, f, token := srvSetup(t)
	srvError(t, srvRequest(t, server, token, "/v1/admissions", map[string]any{"proof": f.Proof, "authorization": f.Authorization}), 404, "unknown_job", false)
	node.context.ProspectTarget = strings.Repeat("e", 64)
	srvError(t, srvRequest(t, server, token, "/v1/jobs", map[string]any{"public_key": f.Job.PublicKey}), 503, "unavailable", false)
	if node.checks.Load() != 0 {
		t.Fatal("inconsistent resolved policy reached issuance guard")
	}
}
