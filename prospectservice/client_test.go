// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectservice

import (
	"context"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
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

const clientTestToken = "4242424242424242424242424242424242424242424242424242424242424242"

type clientFixture struct {
	Job           prospectjob.Job       `json:"job"`
	Proof         prospect.Proof        `json:"proof"`
	Authorization gymauth.Authorization `json:"authorization"`
	Verified      prospect.Verified     `json:"expected_verified"`
}

func clientReference(t *testing.T) clientFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "prospectjob", "testdata", "jobs_v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []clientFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil || len(fixtures) < 2 {
		t.Fatalf("independent fixture: %v", err)
	}
	return fixtures[0]
}

func clientPolicy(f clientFixture) prospectnode.Policy {
	c := f.Job.Context
	return prospectnode.Policy{Version: 1, Network: c.Network, Genesis: c.Genesis, DNAVersion: c.DNAVersion, ProspectTarget: c.ProspectTarget}
}

func fixtureAdmission(f clientFixture) AdmissionResult {
	auth, job := f.Authorization, f.Job
	return AdmissionResult{Version: 1, ContextSource: "historical-receipt", Authentication: "gym-key-control", BirthRule: "job-issuance",
		Record: registry.Record{Version: 3, Sequence: 1, Context: job.Context, Proof: f.Proof, Verified: f.Verified, Authorization: &auth, Job: &job,
			Birth: &registry.Birth{RuleVersion: 1, BornAtMS: job.IssuedAtMS, AcceptedAtMS: job.IssuedAtMS + 1}}}
}

func testClient(t *testing.T, origin string, f clientFixture) *Client {
	t.Helper()
	client, err := NewClient(ClientConfig{Version: 1, URL: origin, Token: clientTestToken}, clientPolicy(f))
	if wire.TestPoW {
		if err == nil || client != nil {
			t.Fatal("cheap profile created production service client")
		}
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func serveJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func TestClientIssueAndValidateHistoricalReceipt(t *testing.T) {
	f := clientReference(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != "POST" || request.Header.Get("Authorization") != "Bearer "+clientTestToken {
			t.Error("wrong authenticated request")
		}
		if request.URL.Path == "/v1/jobs" {
			serveJSON(writer, f.Job)
		} else {
			serveJSON(writer, fixtureAdmission(f))
		}
	}))
	defer server.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	client := testClient(t, server.URL, f)
	if client == nil {
		return
	}
	job, err := client.IssueJob(context.Background(), strings.ToUpper(f.Job.PublicKey))
	if err != nil || job != f.Job {
		t.Fatalf("wrong job: %v", err)
	}
	proof := f.Proof
	proof.Header, proof.Coinbase = strings.ToUpper(proof.Header), strings.ToUpper(proof.Coinbase)
	auth := alternateFixtureSignature(t, f)
	if auth.Signature == f.Authorization.Signature {
		t.Fatal("alternate signature did not differ")
	}
	// A changed current target cannot renew old eligibility, but does not
	// prevent recovery of the immutable receipt under its original job.
	client.policy.ProspectTarget = strings.Repeat("0", 63) + "1"
	result, err := client.Submit(context.Background(), job, proof, auth)
	if err != nil || !reflect.DeepEqual(result, fixtureAdmission(f)) {
		t.Fatalf("historical receipt differs: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatal("unexpected retries")
	}
}

// Public fixture key 1 and nonce 2 produce a second valid signature for replay
// behavior. This is not a new golden vector or production signing code.
func alternateFixtureSignature(t *testing.T, f clientFixture) gymauth.Authorization {
	t.Helper()
	digest, err := gymauth.AdmissionDigest(f.Job.Context, f.Proof, f.Job.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	var scalar [32]byte
	scalar[31] = 2
	_, public := btcec.PrivKeyFromBytes(scalar[:])
	n := btcec.S256().Params().N
	r := new(big.Int).Mod(new(big.Int).SetBytes(public.SerializeCompressed()[1:]), n)
	s := new(big.Int).Add(new(big.Int).SetBytes(digest[:]), r)
	s.Mul(s, new(big.Int).ModInverse(big.NewInt(2), n)).Mod(s, n)
	if s.Cmp(new(big.Int).Rsh(new(big.Int).Set(n), 1)) > 0 {
		s.Sub(n, s)
	}
	encoded, err := asn1.Marshal(struct{ R, S *big.Int }{r, s})
	if err != nil {
		t.Fatal(err)
	}
	auth := f.Authorization
	auth.Signature = hex.EncodeToString(encoded)
	if err := gymauth.VerifyAuthorization(f.Job.Context, f.Proof, auth); err != nil {
		t.Fatal(err)
	}
	return auth
}

func TestClientRejectsMalformedReceipts(t *testing.T) {
	f := clientReference(t)
	for _, name := range []string{"unknown nested field", "duplicate birth", "null attribute", "zero sequence", "false birth", "wrong work", "new old policy", "wrong source", "oversized"} {
		t.Run(name, func(t *testing.T) {
			result := fixtureAdmission(f)
			switch name {
			case "zero sequence":
				result.Record.Sequence = 0
			case "false birth":
				result.Record.Birth.BornAtMS++
			case "wrong work":
				result.Record.Verified.WorkHash = strings.Repeat("0", 64)
			case "new old policy":
				result.Inserted = true
				result.CurrentNodeChecked = true
				result.ContextSource = "trusted-node-snapshot"
			case "wrong source":
				result.CurrentNodeChecked = true
			}
			raw, _ := json.Marshal(result)
			switch name {
			case "unknown nested field":
				raw = []byte(strings.Replace(string(raw), `"power":`, `"extra":1,"power":`, 1))
			case "duplicate birth":
				raw = []byte(strings.Replace(string(raw), `"born_at_ms":`, `"born_at_ms":1,"born_at_ms":`, 1))
			case "null attribute":
				var value map[string]any
				_ = json.Unmarshal(raw, &value)
				value["record"].(map[string]any)["verified"].(map[string]any)["dna"].(map[string]any)["attributes"].(map[string]any)["power"] = nil
				raw, _ = json.Marshal(value)
			case "oversized":
				raw = []byte(strings.Repeat(" ", MaxResponseSize+1))
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(raw)
			}))
			defer server.Close()
			client := testClient(t, server.URL, f)
			if client == nil {
				return
			}
			if name == "new old policy" {
				client.policy.ProspectTarget = strings.Repeat("0", 63) + "1"
			}
			_, err := client.Submit(context.Background(), f.Job, f.Proof, f.Authorization)
			var failure *ClientError
			if !errors.As(err, &failure) || !failure.OutcomeUnknown {
				t.Fatalf("malformed success was acknowledged: %v", err)
			}
		})
	}
}

func TestClientErrorsAreStrictAndRedacted(t *testing.T) {
	f := clientReference(t)
	for _, test := range []struct {
		name                 string
		status               int
		code                 string
		unknown, wantUnknown bool
	}{
		{"busy", 503, "busy", false, false}, {"wrong status", 503, "expired", false, true},
		{"wrong flag", 503, "busy", true, true}, {"hidden uncertainty", 503, "outcome_unknown", false, true},
		{"server text", 503, clientTestToken, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				_ = json.NewEncoder(w).Encode(ErrorResponse{Version: 1, Error: APIError{Code: test.code, OutcomeUnknown: test.unknown}})
			}))
			defer server.Close()
			client := testClient(t, server.URL, f)
			if client == nil {
				return
			}
			_, err := client.Submit(context.Background(), f.Job, f.Proof, f.Authorization)
			var failure *ClientError
			if !errors.As(err, &failure) || failure.OutcomeUnknown != test.wantUnknown || strings.Contains(err.Error(), clientTestToken) || strings.Contains(err.Error(), server.URL) {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}

func TestSignAndSubmitRetainsAuthorizationAfterLostResponse(t *testing.T) {
	f := clientReference(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = connection.Close()
		}
	}))
	defer server.Close()
	client := testClient(t, server.URL, f)
	if client == nil {
		return
	}
	var signed int
	signer := func(ctx context.Context, c prospect.Context, p prospect.Proof) (gymauth.Authorization, error) {
		signed++
		var scalar [32]byte
		scalar[31] = 1
		key, _ := btcec.PrivKeyFromBytes(scalar[:])
		return gymauth.SignAdmission(c, p, key)
	}
	_, auth, err := client.SignAndSubmit(context.Background(), f.Job, f.Proof, signer)
	var failure *ClientError
	if !errors.As(err, &failure) || !failure.OutcomeUnknown || signed != 1 || requests.Load() != 1 || gymauth.VerifyAuthorization(f.Job.Context, f.Proof, auth) != nil {
		t.Fatalf("lost-response recovery data missing: %v", err)
	}
	foreign := f.Job.Context
	foreign.Network++
	foreignJob, err := prospectjob.New(foreign, f.Job.PublicKey, f.Job.Challenge, f.Job.IssuedAtMS)
	if err != nil {
		t.Fatal(err)
	}
	proof := f.Proof
	proof.JobID = foreignJob.ID
	proof.GymID = foreignJob.GymID
	_, _, err = client.SignAndSubmit(context.Background(), foreignJob, proof, signer)
	if err == nil || signed != 1 || requests.Load() != 1 {
		t.Fatal("foreign chain reached signer or transport")
	}
}

func TestClientTLSAndRedirectBoundary(t *testing.T) {
	f := clientReference(t)
	var received atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1); serveJSON(w, f.Job) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client := testClient(t, redirect.URL, f)
	if client == nil {
		return
	}
	if _, err := client.IssueJob(context.Background(), f.Job.PublicKey); err == nil || received.Load() != 0 {
		t.Fatal("followed authority redirect")
	}
	tlsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serveJSON(w, f.Job) }))
	tlsServer.Config.ErrorLog = log.New(io.Discard, "", 0)
	tlsServer.StartTLS()
	defer tlsServer.Close()
	untrusted := testClient(t, tlsServer.URL, f)
	if _, err := untrusted.IssueJob(context.Background(), f.Job.PublicKey); err == nil {
		t.Fatal("accepted untrusted TLS")
	}
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsServer.Certificate().Raw})
	trusted, err := NewClient(ClientConfig{Version: 1, URL: tlsServer.URL, Token: clientTestToken, CACertificate: string(ca)}, clientPolicy(f))
	if err != nil {
		t.Fatal(err)
	}
	defer trusted.Close()
	if job, err := trusted.IssueJob(context.Background(), f.Job.PublicKey); err != nil || job != f.Job {
		t.Fatalf("verified TLS failed: %v", err)
	}
}
