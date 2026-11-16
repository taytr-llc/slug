//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectservice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
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
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/registry"
)

func v2Reference(t *testing.T) clientFixture {
	t.Helper()
	raw, err := os.ReadFile("../registry/testdata/mining_winner_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture clientFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func v2Receipt(f clientFixture) AdmissionResult {
	result := fixtureAdmission(f)
	result.Version = Version2
	result.Record.Version = registry.Version4
	result.Record.Eligibility = &registry.Eligibility{RuleVersion: registry.EligibilityRuleVersion2,
		Source: "canonical-winning-block", Hash: f.Verified.WorkHash, Height: f.Job.Context.ParentHeight + 1}
	return result
}

func v2Config(f clientFixture) ServerConfig {
	raw, _ := hex.DecodeString(clientTestToken)
	digest := sha256.Sum256(raw)
	return ServerConfig{Version: Version2, Listen: "127.0.0.1:0", MaxJobs: 10, MaxRecords: 1,
		Clients: []Credential{{PublicKey: f.Job.PublicKey, TokenSHA256: hex.EncodeToString(digest[:])}}}
}

type v2Node struct {
	*srvNode
	admissions atomic.Int32
	legacy     atomic.Int32
	expected   prospect.Verified
	source     string
}

func (node *v2Node) Recheck(context.Context, prospect.Context, prospect.Proof) error {
	node.legacy.Add(1)
	return errors.New("legacy current-parent path is unavailable")
}

func (node *v2Node) RecheckAdmission(_ context.Context, initial prospect.Context, proof prospect.Proof, verified prospect.Verified) (string, error) {
	node.admissions.Add(1)
	if node.failed.Load() {
		return "", errors.New("private node failure must not be exposed")
	}
	if initial != node.context || verified != node.expected || verified.Version != proof.Version {
		return "", errors.New("guard was not supplied internally verified work")
	}
	return node.source, nil
}

func TestV2ConstructorsRequireExplicitMatchingSchemaAndNode(t *testing.T) {
	f := v2Reference(t)
	config, policy := v2Config(f), clientPolicy(f)
	legacy, err := registry.CreateBirth(filepath.Join(t.TempDir(), "legacy"), f.Job.Context.Network, f.Job.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	mining, err := registry.CreateMining(filepath.Join(t.TempDir(), "mining"), f.Job.Context.Network, f.Job.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer mining.Close()
	node := &v2Node{srvNode: &srvNode{context: f.Job.Context}, source: "canonical-winning-block"}
	if _, err := NewServer(config, policy, node, legacy); err == nil {
		t.Fatal("v2 silently upgraded schema3")
	}
	if _, err := NewServer(config, policy, node.srvNode, mining); err == nil {
		t.Fatal("v2 accepted node without D69 guard")
	}
	config.Version = Version1
	if _, err := NewServer(config, policy, node, mining); err == nil {
		t.Fatal("v1 silently enabled schema4")
	}
	if _, err := NewServer(config, policy, node.srvNode, legacy); err != nil {
		t.Fatalf("v1 no longer accepts its original node/schema: %v", err)
	}
	if info, err := legacy.Info(); err != nil || info.Version != registry.Version3 || info.Jobs != 0 || info.Records != 0 {
		t.Fatal("constructor mutated legacy schema or contents")
	}
	if node.admissions.Load() != 0 || node.resolves.Load() != 0 {
		t.Fatal("constructor contacted a node")
	}
}

func TestV2CanonicalWinnerAdmissionUsesVerifiedGuardAndPersistsEligibility(t *testing.T) {
	f := v2Reference(t)
	store, err := registry.CreateMining(filepath.Join(t.TempDir(), "mining"), f.Job.Context.Network, f.Job.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	node := &v2Node{srvNode: &srvNode{context: f.Job.Context}, source: "canonical-winning-block"}
	server, err := NewServer(v2Config(f), clientPolicy(f), node, store)
	if err != nil {
		t.Fatal(err)
	}
	wrong := srvRequest(t, server, clientTestToken, "/v1/jobs", map[string]any{"public_key": f.Job.PublicKey})
	var failure ErrorResponse
	if json.Unmarshal(wrong.Body.Bytes(), &failure) != nil || wrong.Code != 400 || failure.Version != Version2 || node.resolves.Load() != 0 {
		t.Fatalf("v1 path reached v2 issuance: %s", wrong.Body.String())
	}
	issued := srvRequest(t, server, clientTestToken, "/v2/jobs", map[string]any{"public_key": f.Job.PublicKey})
	var job prospectjob.Job
	if issued.Code != 200 || json.Unmarshal(issued.Body.Bytes(), &job) != nil {
		t.Fatalf("v2 issuance: %s", issued.Body.String())
	}
	// This is a bounded behavior test for the fresh random job, not a golden
	// derivation: only public fixture scalar1 is used. At most16 normal hashes.
	var proof prospect.Proof
	var auth gymauth.Authorization
	found := false
	for nonce := uint32(0); nonce < 16; nonce++ {
		proof, auth = srvProof(t, srvFixture{Job: f.Job, Context: f.Job.Context, Proof: f.Proof, Authorization: f.Authorization}, job, nonce)
		node.expected, err = prospect.Verify(job.Context, proof)
		if err != nil {
			t.Fatal(err)
		}
		eligibility := registry.Eligibility{RuleVersion: 2, Source: "canonical-winning-block", Hash: node.expected.WorkHash, Height: job.Context.ParentHeight + 1}
		if registry.ValidateEligibility(job.Context, node.expected, eligibility) == nil {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("bounded public fixture attempts found no block-target winner")
	}
	response := srvRequest(t, server, clientTestToken, "/v2/admissions", map[string]any{"proof": proof, "authorization": auth})
	var first AdmissionResult
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &first) != nil || !first.Inserted || first.Version != Version2 ||
		first.Record.Version != registry.Version4 || first.Record.Eligibility == nil || first.Record.Eligibility.Source != "canonical-winning-block" ||
		first.Record.Eligibility.Hash != node.expected.WorkHash || first.Record.Eligibility.Height != job.Context.ParentHeight+1 ||
		node.admissions.Load() != 1 || node.legacy.Load() != 0 {
		t.Fatalf("v2 winner admission: %s", response.Body.String())
	}
	node.failed.Store(true)
	server.policy.ProspectTarget = strings.Repeat("e", 64)
	response = srvRequest(t, server, clientTestToken, "/v2/admissions", map[string]any{"proof": proof, "authorization": auth})
	var replay AdmissionResult
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &replay) != nil || replay.Inserted || replay.CurrentNodeChecked ||
		!reflect.DeepEqual(replay.Record, first.Record) || node.admissions.Load() != 1 {
		t.Fatalf("capacity/policy/outage replay changed eligibility: %s", response.Body.String())
	}
}

// Persist independently derived historical evidence to model reopening an
// expired schema4 receipt, without adding a public time override to the API.
func v2HistoricalStore(t *testing.T, f clientFixture) *registry.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "historical")
	store, err := registry.CreateMining(path, f.Job.Context.Network, f.Job.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true, Strict: opt.StrictAll})
	if err != nil {
		t.Fatal(err)
	}
	record := v2Receipt(f).Record
	meta := struct {
		Version      uint32 `json:"version"`
		Network      uint32 `json:"network"`
		Genesis      string `json:"genesis"`
		LastSequence uint64 `json:"last_sequence"`
		JobCount     uint64 `json:"job_count,omitempty"`
		LastClockMS  int64  `json:"last_clock_ms,omitempty"`
	}{4, f.Job.Context.Network, f.Job.Context.Genesis, 1, 1, record.Birth.AcceptedAtMS}
	encode := func(value any) []byte { data, _ := json.Marshal(value); return data }
	sequence := make([]byte, len("sequence:")+8)
	copy(sequence, "sequence:")
	binary.BigEndian.PutUint64(sequence[len("sequence:"):], 1)
	batch := new(leveldb.Batch)
	batch.Put([]byte("meta"), encode(meta))
	batch.Put([]byte("job:"+f.Job.ID), encode(f.Job))
	batch.Put([]byte("record:"+f.Verified.ProspectID), encode(record))
	batch.Put(sequence, []byte(f.Verified.ProspectID))
	if err := db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = registry.Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestV2ClientRecoversExpiredCanonicalReceiptWithoutNode(t *testing.T) {
	f := v2Reference(t)
	if time.Now().UnixMilli() < f.Job.ExpiresAtMS {
		t.Fatal("public historical fixture must have expired for this regression")
	}
	store := v2HistoricalStore(t, f)
	node := &v2Node{srvNode: &srvNode{context: f.Job.Context}, source: "canonical-winning-block"}
	node.failed.Store(true)
	server, err := NewServer(v2Config(f), clientPolicy(f), node, store)
	if err != nil {
		t.Fatal(err)
	}
	server.policy.ProspectTarget = strings.Repeat("e", 64)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	client, err := NewClient(ClientConfig{Version: 2, URL: httpServer.URL, Token: clientTestToken}, clientPolicy(f))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.ProtocolVersion() != Version2 {
		t.Fatal("client lost explicit v2 protocol")
	}
	got, err := client.Submit(context.Background(), f.Job, f.Proof, f.Authorization)
	if err != nil || !reflect.DeepEqual(got, v2Receipt(f)) || node.admissions.Load() != 0 || node.resolves.Load() != 0 || node.legacy.Load() != 0 {
		t.Fatalf("expired historical D69 receipt recovery: %+v %v", got, err)
	}
}

func TestV2ClientRejectsForgedOrNonStrictEligibility(t *testing.T) {
	f := v2Reference(t)
	for _, test := range []string{"missing", "null", "unknown", "duplicate", "case-alias", "rule", "source", "hash", "height", "schema", "response-version", "parent-binding"} {
		t.Run(test, func(t *testing.T) {
			result := v2Receipt(f)
			switch test {
			case "missing", "null":
				result.Record.Eligibility = nil
			case "rule":
				result.Record.Eligibility.RuleVersion = 1
			case "source":
				result.Record.Eligibility.Source = "submitted-block"
			case "hash":
				result.Record.Eligibility.Hash = f.Job.Context.ParentHash
			case "height":
				result.Record.Eligibility.Height--
			case "schema":
				result.Record.Version = 3
			case "response-version":
				result.Version = 1
			case "parent-binding":
				result.Record.Eligibility.Source = "current-parent"
			}
			raw, _ := json.Marshal(result)
			switch test {
			case "null":
				raw = bytes.Replace(raw, []byte(`"birth":`), []byte(`"eligibility":null,"birth":`), 1)
			case "unknown":
				raw = bytes.Replace(raw, []byte(`"eligibility":{`), []byte(`"eligibility":{"extra":1,`), 1)
			case "duplicate":
				raw = bytes.Replace(raw, []byte(`"eligibility":{`), []byte(`"eligibility":{"rule_version":2,`), 1)
			case "case-alias":
				raw = bytes.Replace(raw, []byte(`"eligibility":{`), []byte(`"eligibility":{"RULE_VERSION":2,`), 1)
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/v2/admissions" {
					t.Error("v2 client used legacy route")
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write(raw)
			}))
			defer server.Close()
			client, err := NewClient(ClientConfig{Version: 2, URL: server.URL, Token: clientTestToken}, clientPolicy(f))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			got, err := client.Submit(context.Background(), f.Job, f.Proof, f.Authorization)
			var failure *ClientError
			if !errors.As(err, &failure) || !failure.OutcomeUnknown || !reflect.DeepEqual(got, AdmissionResult{}) {
				t.Fatalf("forged D69 response acknowledged: %+v %v", got, err)
			}
		})
	}
}

func TestV2ClientRejectsV1ErrorEnvelope(t *testing.T) {
	f := v2Reference(t)
	for _, version := range []uint32{Version1, Version2} {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(writer).Encode(ErrorResponse{Version: version, Error: APIError{Code: "expired"}})
		}))
		client, err := NewClient(ClientConfig{Version: 2, URL: server.URL, Token: clientTestToken}, clientPolicy(f))
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Submit(context.Background(), f.Job, f.Proof, f.Authorization)
		var failure *ClientError
		if !errors.As(err, &failure) || failure.OutcomeUnknown != (version == Version1) || (version == Version2 && failure.Code != "expired") {
			t.Fatalf("wrong response version uncertainty: %v", err)
		}
		client.Close()
		server.Close()
	}
}
