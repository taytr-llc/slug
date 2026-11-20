// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type clientOracleSource struct {
	Name     string                  `json:"name"`
	Job      prospectjob.ManifestJob `json:"job"`
	Template prospectwork.Template   `json:"template"`
	Payout   string                  `json:"payout_address"`
}
type clientOracleCase struct {
	Source   string `json:"source_candidate"`
	Nonce    uint32 `json:"nonce"`
	Header   string `json:"header"`
	Hash     string `json:"work_hash"`
	Key      string `json:"submission_key"`
	Block    bool   `json:"qualifies_block"`
	Prospect bool   `json:"qualifies_prospect"`
}

func clientFixture(t *testing.T) (ClientConfig, poolshare.MiningWork, clientOracleCase) {
	t.Helper()
	raw, e := os.ReadFile("../prospectwork/testdata/pool_share_reference/pool_share_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("independent source changed")
	}
	var corpus struct {
		Sources []clientOracleSource `json:"sources"`
		Cases   []clientOracleCase   `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("decode public source")
	}
	var source clientOracleSource
	var tc clientOracleCase
	for _, s := range corpus.Sources {
		if s.Name == "coinbase-only" {
			source = s
			break
		}
	}
	for _, c := range corpus.Cases {
		if c.Source == source.Name && c.Nonce == 0 {
			tc = c
			break
		}
	}
	if source.Job.ID == "" || tc.Header == "" {
		t.Fatal("missing independent fixture")
	}
	j := source.Job
	p := prospectnode.Policy{Version: j.Context.Version, Network: j.Context.Network, Genesis: j.Context.Genesis, DNAVersion: j.Context.DNAVersion, ProspectTarget: j.Context.ProspectTarget}
	s := poolshare.Config{Version: poolshare.Version1, AuthorityPin: registry.ManifestAuthorityPin{Version: registry.Version5, Network: p.Network, Genesis: p.Genesis, AuthorityID: j.AuthorityID, DescriptorID: j.DescriptorID}, Policy: p, PayoutAddress: source.Payout, PoolTarget: strings.Repeat("f", 64), MaxWorks: 256, MaxSubmissions: 4096, MaxBytes: 1 << 24}
	return ClientConfig{Version: Version1, URL: "http://127.0.0.1:19001", Token: strings.Repeat("ab", 32), PublicKey: j.PublicKey, Source: s}, poolshare.MiningWork{Job: j, Template: source.Template}, tc
}

type clientTransport func(*http.Request) (*http.Response, error)

func (fn clientTransport) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }
func clientResponse(t *testing.T, status int, value any) *http.Response {
	t.Helper()
	raw, e := json.Marshal(value)
	if e != nil {
		t.Fatal(e)
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(append(raw, '\n')))}
}
func clientError(t *testing.T, e error, code string, unknown bool) {
	t.Helper()
	var got *ClientError
	if !errors.As(e, &got) || got.Code != code || got.OutcomeUnknown != unknown {
		t.Fatalf("want %s/%v, got %v", code, unknown, e)
	}
}
func normalClient(t *testing.T) (*Client, poolshare.MiningWork, clientOracleCase) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal work; profile refusal tested separately")
	}
	c, w, tc := clientFixture(t)
	client, e := NewClient(c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(client.Close)
	return client, w, tc
}

func TestPoolClientCanonicalConfigAndTrust(t *testing.T) {
	c, _, _ := clientFixture(t)
	valid, e := ValidateClientConfig(c)
	if e != nil || valid != c {
		t.Fatal("valid config changed", e)
	}
	id, e := TransportTrustID(c)
	if e != nil || !hex64(id, false) {
		t.Fatal(e)
	}
	c.Token = strings.Repeat("cd", 32)
	same, e := TransportTrustID(c)
	if e != nil || same != id {
		t.Fatal("credential rotation changed trust")
	}
	c.Source.MaxWorks--
	different, e := TransportTrustID(c)
	if e != nil || different == id {
		t.Fatal("source bounds not bound")
	}
	for _, mutate := range []func(*ClientConfig){
		func(c *ClientConfig) { c.URL = "http://example.invalid" }, func(c *ClientConfig) { c.URL = "http://127.0.0.1:19001/path" },
		func(c *ClientConfig) { c.URL = "http://127.0.0.1:19001/?query=1" }, func(c *ClientConfig) { c.URL = "http://user@127.0.0.1:19001" },
		func(c *ClientConfig) { c.CACertificate = "garbage" }, func(c *ClientConfig) { c.PublicKey = "04" + c.PublicKey[2:] },
		func(c *ClientConfig) { c.Source.PoolTarget = strings.Repeat("0", 64) }, func(c *ClientConfig) { c.Source.MaxWorks = 257 },
		func(c *ClientConfig) { c.Source.AuthorityPin.Network++ }, func(c *ClientConfig) { c.Source.PayoutAddress = "" },
	} {
		bad := c
		mutate(&bad)
		if _, e = ValidateClientConfig(bad); e == nil {
			t.Fatal("invalid config accepted")
		}
	}
}

func TestPoolClientStrictJSON(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"job_id":"x","job_id":"x"}`, `{"job_id":"x","other":1}`, `{}`, `{"job_id":null}`, `{"job_id":"x"} {}`, `{"job_id": "x"}`, `{"job_id":"\u0078"}`} {
		var value workRequest
		if decodeStrict([]byte(raw), 1024, &value) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	var value workRequest
	if decodeStrict([]byte(" \n{\"job_id\":\"x\"}\n"), 1024, &value) != nil || value.JobID != "x" {
		t.Fatal("outer whitespace refused")
	}
	deep := []byte(strings.Repeat("{\"x\":", 17) + `{}` + strings.Repeat("}", 17))
	if decodeStrict(deep, 1024, &value) == nil {
		t.Fatal("deep JSON accepted")
	}
}

func TestPoolClientTransportAndWorkBinding(t *testing.T) {
	c, w, _ := normalClient(t)
	tr, ok := c.http.Transport.(*http.Transport)
	if !ok || tr.Proxy != nil || !tr.DisableCompression || !tr.DisableKeepAlives || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 || c.http.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("transport trust weakened")
	}
	c.http.Transport = clientTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != pathPrefix+"issue" || r.Header.Get("Authorization") != "Bearer "+c.config.Token {
			t.Fatal("wrong request binding")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "{}" {
			t.Fatal("issuance accepted claimant identity")
		}
		return clientResponse(t, 200, WorkResponse{Version1, c.binding, w}), nil
	})
	got, e := c.Issue(context.Background())
	if e != nil || got.Job != w.Job {
		t.Fatal("valid work rejected", e)
	}
	if _, e = PrepareWork(c.config.Source, c.config.PublicKey, got); e != nil {
		t.Fatal(e)
	}
	c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) {
		return clientResponse(t, 200, WorkResponse{Version1, strings.Repeat("0", 64), w}), nil
	})
	_, e = c.Issue(context.Background())
	clientError(t, e, "outcome_unknown", true)
	_, e = c.Work(context.Background(), w.Job.ID)
	clientError(t, e, "unavailable", false)
	c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("private endpoint and token must be sanitized")
	})
	_, e = c.Issue(context.Background())
	clientError(t, e, "outcome_unknown", true)
	if strings.Contains(e.Error(), "private") {
		t.Fatal("transport error leaked")
	}
	_, e = c.Work(context.Background(), w.Job.ID)
	clientError(t, e, "unavailable", false)
}

func TestPoolClientWorkOwnershipAndRefusal(t *testing.T) {
	c, w, _ := normalClient(t)
	prepared, e := PrepareWork(c.config.Source, c.config.PublicKey, w)
	if e != nil {
		t.Fatal(e)
	}
	original := prepared.Candidate()
	w.Template.Block.Mutable = append(w.Template.Block.Mutable, "fixture-mutation")
	if prepared.Candidate().BlockHex != original.BlockHex {
		t.Fatal("preparation retained caller mutation")
	}
	for _, mutate := range []func(*poolshare.MiningWork){
		func(w *poolshare.MiningWork) { w.Job.PublicKey = "02" + strings.Repeat("00", 32) },
		func(w *poolshare.MiningWork) { w.Template.Context.ParentHeight++ },
		func(w *poolshare.MiningWork) { w.Template.Block.NonceRange = "00000000fffffffe" },
		func(w *poolshare.MiningWork) { w.Template.Block.Mutable = make([]string, 1025) },
	} {
		bad := w
		mutate(&bad)
		if _, e = PrepareWork(c.config.Source, c.config.PublicKey, bad); e == nil {
			t.Fatal("unbound work accepted")
		}
	}
}

func oracleReceipt(w poolshare.MiningWork, tc clientOracleCase, source poolshare.Config) (prospectwork.ShareCheck, poolshare.Submission) {
	check := prospectwork.ShareCheck{JobID: w.Job.ID, GymID: w.Job.GymID, Nonce: tc.Nonce, Header: tc.Header, WorkHash: tc.Hash, DuplicateKey: tc.Key, Pool: true, Prospect: tc.Prospect, Block: tc.Block}
	r := poolshare.Submission{Version: poolshare.Version1, Sequence: 1, AcceptedSequence: 1, JobID: w.Job.ID, GymID: w.Job.GymID, Nonce: tc.Nonce, PoolTarget: source.PoolTarget, Header: tc.Header, WorkHash: tc.Hash, DuplicateKey: tc.Key, Pool: true, Prospect: tc.Prospect, Block: tc.Block, ReceivedAtMS: w.Job.IssuedAtMS + 10, AcceptedAtMS: w.Job.IssuedAtMS + 20, Acceptance: "current-parent", BlockState: "none"}
	return check, r
}
func TestPoolClientReceiptChecksIndependentFacts(t *testing.T) {
	c, w, tc := clientFixture(t)
	check, r := oracleReceipt(w, tc, c.Source)
	if !validSubmission(c.Source, w, tc.Nonce, check, r) {
		t.Fatal("independently bound receipt rejected")
	}
	for _, mutate := range []func(*poolshare.Submission){
		func(r *poolshare.Submission) { r.Sequence = 0 }, func(r *poolshare.Submission) { r.Sequence = 4097 }, func(r *poolshare.Submission) { r.Nonce++ },
		func(r *poolshare.Submission) { r.GymID = strings.Repeat("0", 64) }, func(r *poolshare.Submission) { r.WorkHash = strings.Repeat("0", 64) },
		func(r *poolshare.Submission) { r.Pool = false }, func(r *poolshare.Submission) { r.AcceptedSequence = 0 },
		func(r *poolshare.Submission) { r.AcceptedAtMS = w.Job.ExpiresAtMS }, func(r *poolshare.Submission) { r.BlockState = "canonical-observed" },
	} {
		bad := r
		mutate(&bad)
		if validSubmission(c.Source, w, tc.Nonce, check, bad) {
			t.Fatal("forged receipt accepted")
		}
	}
	// Modeled pending block: structural status test only, not a PoW oracle.
	blockCheck := check
	blockCheck.Block = true
	pending := r
	pending.Block = true
	pending.AcceptedSequence = 0
	pending.AcceptedAtMS = 0
	pending.Acceptance = ""
	pending.BlockState = "pending"
	pending.BlockHeight = w.Job.Context.ParentHeight + 1
	if !validSubmission(c.Source, w, tc.Nonce, blockCheck, pending) {
		t.Fatal("pending block falsely required credit")
	}
	pending.BlockState = "canonical-observed"
	if validSubmission(c.Source, w, tc.Nonce, blockCheck, pending) {
		t.Fatal("canonical block with no acceptance accepted")
	}
}

func TestPoolClientNormalSubmitUsesIndependentHash(t *testing.T) {
	c, w, tc := normalClient(t)
	_, receipt := oracleReceipt(w, tc, c.config.Source)
	c.http.Transport = clientTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != pathPrefix+"submit" {
			t.Fatal("wrong path")
		}
		body, _ := io.ReadAll(r.Body)
		var request submissionRequest
		if decodeStrict(body, MaxRequestSize, &request) != nil || request.JobID != w.Job.ID || request.Nonce != tc.Nonce {
			t.Fatal("wrong exact work identity")
		}
		return clientResponse(t, 200, SubmissionResponse{Version1, c.binding, receipt}), nil
	})
	got, e := c.Submit(context.Background(), w, tc.Nonce)
	if e != nil || got != receipt {
		t.Fatal("independent receipt rejected", e)
	}
}

func TestPoolClientOwnBalanceAndFailures(t *testing.T) {
	c, _, _ := normalClient(t)
	r := BalanceResponse{Version: Version1, BindingID: c.binding, ObservedAtMS: 1795000000000, Balance: poolsettle.Balance{GymID: c.gym, Confirmed: poolsettle.Fraction{Numerator: "300000001", Denominator: "3"}, Available: poolsettle.Fraction{Numerator: "300000001", Denominator: "3"}, Reason: "fee-budget"}}
	c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) { return clientResponse(t, 200, r), nil })
	got, e := c.Balance(context.Background())
	if e != nil || got != r {
		t.Fatal(e)
	}
	for _, mutate := range []func(*BalanceResponse){
		func(r *BalanceResponse) { r.Balance.GymID = strings.Repeat("0", 64) }, func(r *BalanceResponse) { r.Balance.Confirmed.Numerator = "-1" },
		func(r *BalanceResponse) {
			r.Balance.Available = poolsettle.Fraction{Numerator: "999999999", Denominator: "1"}
		},
		func(r *BalanceResponse) { r.Balance.Available = poolsettle.Fraction{Numerator: "0", Denominator: "2"} }, func(r *BalanceResponse) { r.Balance.ReservedAtoms = 100000001 },
		func(r *BalanceResponse) { r.Balance.Reason = "external secret text" }, func(r *BalanceResponse) { r.ObservedAtMS = 0 },
	} {
		bad := r
		mutate(&bad)
		if validBalance(bad, c.gym, c.binding) {
			t.Fatal("unbound balance accepted")
		}
	}
	for _, code := range []string{"unauthorized", "busy", "not_found", "outcome_unknown"} {
		c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) {
			return clientResponse(t, errorStatus(code), ErrorResponse{Version1, APIError{code, code == "outcome_unknown"}}), nil
		})
		_, e = c.Balance(context.Background())
		clientError(t, e, code, code == "outcome_unknown")
	}
	c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) {
		return clientResponse(t, 200, map[string]any{"secret": "not a balance"}), nil
	})
	_, e = c.Balance(context.Background())
	clientError(t, e, "unavailable", false)
}

func TestPoolClientCheapProfileRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	c, w, _ := clientFixture(t)
	if _, e := NewClient(c); e == nil {
		t.Fatal("cheap client constructed")
	}
	if _, e := PrepareWork(c.Source, c.PublicKey, w); e == nil {
		t.Fatal("cheap work prepared")
	}
}
