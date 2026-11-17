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
	"testing"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

const manifestReferenceSHA = "e9e83d8430fe2e50e8c71c05ed5b8b0cb59eff027faad23197b3f8023a5883c4"
const manifestReferenceScriptSHA = "b03f3235e663c8a77fb4fcf62794ac98ce3201a3784d3cc276c470ecc130420d"

type manifestConfigReference struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Raw      string `json:"raw"`
	Accepted bool   `json:"accepted"`
	Expected *struct {
		Config                    json.RawMessage `json:"config"`
		Canonical                 string          `json:"canonical"`
		PreimageHex               string          `json:"preimage_hex"`
		TransportTrustID          string          `json:"transport_trust_id"`
		StaticConstructorAllowed  bool            `json:"static_constructor_allowed"`
		SessionConstructorAllowed bool            `json:"session_constructor_allowed"`
	} `json:"expected"`
}

type manifestJobReference struct {
	Name               string                        `json:"name"`
	BodyHex            string                        `json:"body_hex"`
	Pin                registry.ManifestAuthorityPin `json:"pin"`
	RequestedPublicKey string                        `json:"requested_public_key"`
	Policy             prospectnode.Policy           `json:"policy"`
	Accepted           bool                          `json:"accepted"`
	Expected           *prospectjob.ManifestJob      `json:"expected"`
}

type manifestReceiptReference struct {
	Name          string                   `json:"name"`
	Job           prospectjob.ManifestJob  `json:"job"`
	Proof         prospect.Proof           `json:"proof"`
	Authorization gymauth.Authorization    `json:"authorization"`
	Policy        prospectnode.Policy      `json:"policy"`
	Body          string                   `json:"body"`
	Accepted      bool                     `json:"accepted"`
	Expected      *ManifestAdmissionResult `json:"expected"`
}

type manifestErrorReference struct {
	Name            string `json:"name"`
	Status          int    `json:"status"`
	Body            string `json:"body"`
	WireValid       bool   `json:"wire_valid"`
	ExpectedCode    string `json:"expected_code"`
	ExpectedUnknown bool   `json:"expected_unknown"`
}

type manifestReferenceCorpus struct {
	Pin          registry.ManifestAuthorityPin `json:"pin"`
	Provenance   map[string]string             `json:"provenance"`
	Configs      []manifestConfigReference     `json:"config_cases"`
	Jobs         []manifestJobReference        `json:"job_cases"`
	Receipts     []manifestReceiptReference    `json:"receipt_cases"`
	Errors       []manifestErrorReference      `json:"error_cases"`
	SourceValues []struct {
		Job           prospectjob.ManifestJob `json:"job"`
		Proof         prospect.Proof          `json:"proof"`
		Authorization gymauth.Authorization   `json:"authorization"`
	} `json:"source_fixtures"`
}

var manifestReferenceOnce sync.Once
var manifestReferenceData manifestReferenceCorpus
var manifestReferenceErr error

func loadManifestReference(t *testing.T) *manifestReferenceCorpus {
	t.Helper()
	manifestReferenceOnce.Do(func() {
		dir := filepath.Join("testdata", "manifest")
		readPinned := func(name, expected string) []byte {
			if manifestReferenceErr != nil {
				return nil
			}
			raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
			if err != nil {
				manifestReferenceErr = err
				return nil
			}
			hash := sha256.Sum256(raw)
			if hex.EncodeToString(hash[:]) != expected {
				manifestReferenceErr = errors.New("independent manifest reference input changed: " + name)
				return nil
			}
			return raw
		}
		raw := readPinned("manifest_service_v1.json", manifestReferenceSHA)
		if manifestReferenceErr == nil {
			manifestReferenceErr = json.Unmarshal(raw, &manifestReferenceData)
		}
		readPinned("reference_manifest_service.py", manifestReferenceScriptSHA)
		for name, expected := range manifestReferenceData.Provenance {
			readPinned(name, expected)
		}
	})
	if manifestReferenceErr != nil {
		t.Fatal(manifestReferenceErr)
	}
	return &manifestReferenceData
}

func TestManifestIndependentConfigAndTrust(t *testing.T) {
	corpus := loadManifestReference(t)
	if len(corpus.Configs) != 102 {
		t.Fatal("incomplete independent configuration corpus")
	}
	for _, fixture := range corpus.Configs {
		t.Run(fixture.Name, func(t *testing.T) {
			if fixture.Kind == "client" {
				config, err := ParseManifestClientConfig([]byte(fixture.Raw))
				if (err == nil) != fixture.Accepted {
					t.Fatalf("parse acceptance got %v, want %v: %v", err == nil, fixture.Accepted, err)
				}
				if err != nil {
					if !reflect.DeepEqual(config, ManifestClientConfig{}) {
						t.Fatal("rejected config returned partial data")
					}
					return
				}
				raw, err := json.Marshal(config)
				if err != nil || string(raw) != fixture.Expected.Canonical {
					t.Fatal("canonical config differs from independent bytes")
				}
				canonical, err := ValidateManifestClientConfig(config)
				if err != nil || !reflect.DeepEqual(canonical, config) {
					t.Fatal("typed validator changed canonical configuration")
				}
				id, err := ManifestTransportTrustID(config)
				if err != nil || id != fixture.Expected.TransportTrustID {
					t.Fatal("transport trust ID differs from independent SHA256 preimage")
				}
				preimage, err := hex.DecodeString(fixture.Expected.PreimageHex)
				hash := sha256.Sum256(preimage)
				if err != nil || hex.EncodeToString(hash[:]) != id {
					t.Fatal("independent preimage does not match its pinned digest")
				}
				return
			}
			config, err := ParseManifestServerConfig([]byte(fixture.Raw))
			if (err == nil) != fixture.Accepted {
				t.Fatalf("parse acceptance got %v, want %v: %v", err == nil, fixture.Accepted, err)
			}
			if err != nil {
				if !reflect.DeepEqual(config, ManifestServerConfig{}) {
					t.Fatal("rejected config returned partial data")
				}
				return
			}
			raw, err := json.Marshal(config)
			if err != nil || string(raw) != fixture.Expected.Canonical {
				t.Fatal("canonical server config differs from independent bytes")
			}
			canonical, err := ValidateManifestServerConfig(config)
			if err != nil || !reflect.DeepEqual(canonical, config) {
				t.Fatal("typed server validator changed canonical configuration")
			}
			if len(config.Clients) > 0 {
				canonical.Clients[0].PublicKey = "changed"
				if config.Clients[0].PublicKey == "changed" {
					t.Fatal("server config containers are not owned")
				}
			}
		})
	}
}

// This responder is a fixed external authority fixture. It never interprets or
// creates a receipt, and each request/result is joined before changing the case.
type manifestReferenceResponder struct {
	mu       sync.Mutex
	body     []byte
	status   int
	requests uint64
	path     string
	key      string
	proof    *prospect.Proof
	auth     *gymauth.Authorization
	bad      bool
}

func (r *manifestReferenceResponder) set(body []byte, status int, path, key string, proof *prospect.Proof, auth *gymauth.Authorization) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.body, r.status, r.path, r.key, r.proof, r.auth, r.bad = body, status, path, key, proof, auth, false
	return r.requests
}

func (r *manifestReferenceResponder) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(request.Body, 65537))
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests++
	if err != nil || len(raw) > 65536 || request.Method != http.MethodPost || request.URL.Path != r.path ||
		request.Header.Get("Authorization") != "Bearer "+strings.Repeat("ab", 32) || request.Header.Get("Content-Type") != "application/json" {
		r.bad = true
	}
	if r.path == "/v3/jobs" {
		var input map[string]json.RawMessage
		var key string
		if json.Unmarshal(raw, &input) != nil || len(input) != 1 || json.Unmarshal(input["public_key"], &key) != nil || key != r.key {
			r.bad = true
		}
	} else {
		var input map[string]json.RawMessage
		var proof prospect.Proof
		var auth gymauth.Authorization
		if json.Unmarshal(raw, &input) != nil || len(input) != 2 || json.Unmarshal(input["proof"], &proof) != nil || json.Unmarshal(input["authorization"], &auth) != nil ||
			!reflect.DeepEqual(proof, *r.proof) || !reflect.DeepEqual(auth, *r.auth) {
			r.bad = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(r.status)
	_, _ = w.Write(r.body)
}

func (r *manifestReferenceResponder) checked(t *testing.T, previous uint64) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bad || r.requests != previous+1 {
		t.Fatal("client request differed from the exact fixture request or retried")
	}
}

func newManifestReferenceClient(t *testing.T, url string, pin registry.ManifestAuthorityPin, policy prospectnode.Policy) *ManifestClient {
	t.Helper()
	client, err := NewManifestClient(ManifestClientConfig{Version: 3, URL: url, Token: strings.Repeat("ab", 32), AuthorityPin: pin}, policy)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if client.ProtocolVersion() != 3 || client.AuthorityOrigin() != url || client.AuthorityPin() != pin {
		t.Fatal("client identity differs from explicit configuration")
	}
	return client
}

func requireManifestReferenceError(t *testing.T, err error, code string, unknown bool) {
	t.Helper()
	var clientError *ClientError
	if !errors.As(err, &clientError) || clientError.Code != code || clientError.OutcomeUnknown != unknown {
		t.Fatalf("unexpected sanitized client error: %v", err)
	}
	if strings.Contains(err.Error(), "fixture-private-detail") || strings.Contains(err.Error(), strings.Repeat("ab", 32)) {
		t.Fatal("client error disclosed transport content")
	}
}

func TestManifestIndependentIssuedJobResponses(t *testing.T) {
	if wire.TestPoW {
		t.Skip("operational client requires normal PoW; public config corpus runs under both profiles")
	}
	corpus := loadManifestReference(t)
	if len(corpus.Jobs) != 130 {
		t.Fatal("incomplete independent job response corpus")
	}
	responder := &manifestReferenceResponder{}
	server := httptest.NewServer(responder)
	defer server.Close()
	for _, fixture := range corpus.Jobs {
		t.Run(fixture.Name, func(t *testing.T) {
			body, err := hex.DecodeString(fixture.BodyHex)
			if err != nil {
				t.Fatal(err)
			}
			before := responder.set(body, 200, "/v3/jobs", fixture.RequestedPublicKey, nil, nil)
			client := newManifestReferenceClient(t, server.URL, fixture.Pin, fixture.Policy)
			job, err := client.IssueManifestJob(context.Background(), fixture.RequestedPublicKey)
			responder.checked(t, before)
			if fixture.Accepted {
				if err != nil || fixture.Expected == nil || job != *fixture.Expected {
					t.Fatalf("job differs from frozen independent value: %v", err)
				}
				return
			}
			requireManifestReferenceError(t, err, "outcome_unknown", true)
			if job != (prospectjob.ManifestJob{}) {
				t.Fatal("untrusted issuance response returned partial job")
			}
		})
	}
}

func TestManifestIndependentAdmissionResponses(t *testing.T) {
	if wire.TestPoW {
		t.Skip("full receipt verification requires production Argon")
	}
	corpus := loadManifestReference(t)
	if len(corpus.Receipts) != 339 {
		t.Fatal("incomplete independent receipt response corpus")
	}
	responder := &manifestReferenceResponder{}
	server := httptest.NewServer(responder)
	defer server.Close()
	// Sequential calls intentionally bound production Argon arenas to one.
	for _, fixture := range corpus.Receipts {
		t.Run(fixture.Name, func(t *testing.T) {
			before := responder.set([]byte(fixture.Body), 200, "/v3/admissions", "", &fixture.Proof, &fixture.Authorization)
			client := newManifestReferenceClient(t, server.URL, corpus.Pin, fixture.Policy)
			result, err := client.SubmitManifest(context.Background(), fixture.Job, fixture.Proof, fixture.Authorization)
			responder.checked(t, before)
			if fixture.Accepted {
				if err != nil || fixture.Expected == nil || !reflect.DeepEqual(result, *fixture.Expected) {
					t.Fatalf("receipt differs from complete independent value: %v", err)
				}
				return
			}
			requireManifestReferenceError(t, err, "outcome_unknown", true)
			if !reflect.DeepEqual(result, ManifestAdmissionResult{}) {
				t.Fatal("untrusted admission response returned partial receipt")
			}
		})
	}
}

func TestManifestIndependentErrorResponses(t *testing.T) {
	if wire.TestPoW {
		t.Skip("operational client requires production profile")
	}
	corpus := loadManifestReference(t)
	if len(corpus.Errors) != 39 {
		t.Fatal("incomplete independent error corpus")
	}
	base := corpus.SourceValues[0]
	policy := prospectnode.Policy{Version: base.Job.Context.Version, Network: base.Job.Context.Network,
		Genesis: base.Job.Context.Genesis, DNAVersion: base.Job.Context.DNAVersion, ProspectTarget: base.Job.Context.ProspectTarget}
	responder := &manifestReferenceResponder{}
	server := httptest.NewServer(responder)
	defer server.Close()
	client := newManifestReferenceClient(t, server.URL, corpus.Pin, policy)
	for _, fixture := range corpus.Errors {
		for _, operation := range []string{"jobs", "admissions"} {
			t.Run(fixture.Name+"/"+operation, func(t *testing.T) {
				before := responder.set([]byte(fixture.Body), fixture.Status, "/v3/"+operation, base.Job.PublicKey, &base.Proof, &base.Authorization)
				var err error
				if operation == "jobs" {
					result, callErr := client.IssueManifestJob(context.Background(), base.Job.PublicKey)
					err = callErr
					if result != (prospectjob.ManifestJob{}) {
						t.Fatal("error response returned job")
					}
				} else {
					result, callErr := client.SubmitManifest(context.Background(), base.Job, base.Proof, base.Authorization)
					err = callErr
					if !reflect.DeepEqual(result, ManifestAdmissionResult{}) {
						t.Fatal("error response returned receipt")
					}
				}
				responder.checked(t, before)
				requireManifestReferenceError(t, err, fixture.ExpectedCode, fixture.ExpectedUnknown)
			})
		}
	}
}

func TestManifestIndependentCorpusHasNoPrivateCertificateKeys(t *testing.T) {
	loadManifestReference(t)
	for _, name := range []string{"go_localhost.pem", "go_ecdsa_p256.pem"} {
		raw, err := os.ReadFile(filepath.Join("testdata", "manifest", "inputs", name))
		if err != nil || !bytes.HasPrefix(raw, []byte("-----BEGIN CERTIFICATE-----")) || bytes.Contains(raw, []byte("PRIVATE KEY")) {
			t.Fatal("public certificate fixture boundary changed")
		}
	}
}
