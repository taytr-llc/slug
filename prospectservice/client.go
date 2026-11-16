// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectservice

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// ClientError exposes a stable public code without endpoint, body or credential
// text. OutcomeUnknown means a submission may already have committed: replay
// the same evidence to recover its original receipt, never reset its birth.
type ClientError struct {
	Code           string
	OutcomeUnknown bool
}

func (err *ClientError) Error() string {
	if err.OutcomeUnknown {
		return "prospect service: " + err.Code + "; outcome unknown; retry the same proof and authorization"
	}
	return "prospect service: " + err.Code
}

// Client uses one explicit authority and policy. It never retries, redirects,
// uses an environment proxy, reads a signing key, or trusts receipt summaries.
type Client struct {
	version uint32
	origin  string
	token   string
	policy  prospectnode.Policy
	http    *http.Client
}

func NewClient(config ClientConfig, policy prospectnode.Policy) (*Client, error) {
	if wire.TestPoW {
		return nil, errors.New("prospect service requires production PoW")
	}
	config, endpoint, err := validateClientConfig(config)
	if err != nil {
		return nil, errors.New("invalid service client configuration")
	}
	raw, _ := json.Marshal(policy)
	policy, err = prospectnode.ParsePolicy(raw)
	if err != nil {
		return nil, errors.New("invalid service policy")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if config.CACertificate != "" {
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM([]byte(config.CACertificate)) {
			return nil, errors.New("invalid service CA certificate")
		}
		tlsConfig.RootCAs = roots
	}
	transport := &http.Transport{
		Proxy: nil, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSClientConfig: tlsConfig, TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second, MaxResponseHeaderBytes: 8192,
		DisableCompression: true, DisableKeepAlives: true,
	}
	return &Client{version: config.Version, origin: strings.TrimRight(endpoint.String(), "/"), token: config.Token, policy: policy,
		http: &http.Client{Transport: transport, Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (client *Client) Close() { client.http.CloseIdleConnections() }

// ProtocolVersion identifies the explicitly selected admission rules.
func (client *Client) ProtocolVersion() uint32 { return client.version }

// AuthorityOrigin identifies the configured authority without credentials.
func (client *Client) AuthorityOrigin() string { return client.origin }

func clientFailure(submission bool) error {
	if submission {
		return &ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	return &ClientError{Code: "unavailable"}
}

func errorStatus(code string) int {
	switch code {
	case "unauthorized":
		return 401
	case "forbidden":
		return 403
	case "invalid_request":
		return 400
	case "too_large":
		return 413
	case "rate_limited":
		return 429
	case "expired":
		return 409
	case "unknown_job":
		return 404
	case "rejected":
		return 422
	case "busy", "capacity", "unavailable", "outcome_unknown":
		return 503
	}
	return 0
}

func (client *Client) post(ctx context.Context, path string, value any, submission bool) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > MaxRequestSize {
		return nil, &ClientError{Code: "invalid_request"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.origin+path, bytes.NewReader(raw))
	if err != nil {
		return nil, &ClientError{Code: "invalid_request"}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+client.token)
	response, err := client.http.Do(request)
	if err != nil {
		return nil, clientFailure(submission)
	}
	defer response.Body.Close()
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || response.Header.Get("Content-Encoding") != "" {
		return nil, clientFailure(submission)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxResponseSize+1))
	if err != nil || len(data) > MaxResponseSize {
		return nil, clientFailure(submission)
	}
	if response.StatusCode != http.StatusOK {
		fields, err := parseObject(data, MaxResponseSize, "version", "error")
		var version uint32
		if err != nil || decodeField(fields, "version", &version) != nil || version != client.version {
			return nil, clientFailure(submission)
		}
		errorFields, err := parseObject(fields["error"], MaxResponseSize, "code", "outcome_unknown")
		var code string
		var unknown bool
		if err != nil || decodeField(errorFields, "code", &code) != nil || decodeField(errorFields, "outcome_unknown", &unknown) != nil ||
			errorStatus(code) != response.StatusCode || unknown != (code == "outcome_unknown") {
			return nil, clientFailure(submission)
		}
		return nil, &ClientError{Code: code, OutcomeUnknown: unknown}
	}
	return data, nil
}

func samePolicy(context prospect.Context, policy prospectnode.Policy) bool {
	return context.Version == policy.Version && context.Network == policy.Network &&
		strings.EqualFold(context.Genesis, policy.Genesis) && context.DNAVersion == policy.DNAVersion &&
		strings.EqualFold(context.ProspectTarget, policy.ProspectTarget)
}

func (client *Client) IssueJob(ctx context.Context, publicKey string) (prospectjob.Job, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	key, err := normalizePublicKey(publicKey)
	if err != nil {
		return prospectjob.Job{}, &ClientError{Code: "invalid_request"}
	}
	data, err := client.post(ctx, apiPath(client.version, "jobs"), struct {
		PublicKey string `json:"public_key"`
	}{key}, false)
	if err != nil {
		return prospectjob.Job{}, err
	}
	job, err := prospectjob.ParseJob(data)
	if err != nil || !samePolicy(job.Context, client.policy) || job.PublicKey != key || ctx.Err() != nil {
		return prospectjob.Job{}, clientFailure(false)
	}
	return job, nil
}

func sameProof(first, second prospect.Proof) bool {
	if first.Version != second.Version || !strings.EqualFold(first.JobID, second.JobID) ||
		!strings.EqualFold(first.GymID, second.GymID) || !strings.EqualFold(first.Header, second.Header) ||
		!strings.EqualFold(first.Coinbase, second.Coinbase) || len(first.MerkleBranch) != len(second.MerkleBranch) {
		return false
	}
	for i := range first.MerkleBranch {
		if !strings.EqualFold(first.MerkleBranch[i], second.MerkleBranch[i]) {
			return false
		}
	}
	return true
}

// parseAdmission validates every nested schema before decoding typed values.
// The wire receipt is not assumed canonical merely because it came over TLS.
func parseAdmission(data []byte) (AdmissionResult, error) {
	var result AdmissionResult
	fields, err := parseObject(data, MaxResponseSize, "version", "context_source", "current_node_checked", "authentication", "birth_rule", "inserted", "record")
	if err != nil {
		return result, err
	}
	if decodeField(fields, "version", &result.Version) != nil || (result.Version != Version1 && result.Version != Version2) {
		return result, errors.New("unsupported admission version")
	}
	keys := []string{"version", "sequence", "context", "proof", "verified", "authorization", "job", "birth"}
	if result.Version == Version2 {
		keys = append(keys, "eligibility")
	}
	record, err := parseObject(fields["record"], registry.MaxRecordSize, keys...)
	if err != nil {
		return result, err
	}
	if result.Version == Version2 {
		if _, err := parseObject(record["eligibility"], MaxResponseSize, "rule_version", "source", "hash", "height"); err != nil {
			return result, err
		}
	}
	if _, err := prospect.ParseContext(record["context"]); err != nil {
		return result, err
	}
	if _, err := prospect.ParseProof(record["proof"]); err != nil {
		return result, err
	}
	if _, err := gymauth.ParseAuthorization(record["authorization"]); err != nil {
		return result, err
	}
	if _, err := prospectjob.ParseJob(record["job"]); err != nil {
		return result, err
	}
	if _, err := parseObject(record["birth"], MaxResponseSize, "rule_version", "born_at_ms", "accepted_at_ms"); err != nil {
		return result, err
	}
	verified, err := parseObject(record["verified"], MaxResponseSize, "version", "verification", "network", "genesis", "prospect_id", "policy_id", "gym_id", "work_hash", "parent_hash", "parent_height", "dna")
	if err != nil {
		return result, err
	}
	dna, err := parseObject(verified["dna"], MaxResponseSize, "dna_version", "seed", "rarity", "talent_budget", "attributes", "prime_weeks", "decline_weeks")
	if err != nil {
		return result, err
	}
	if _, err := parseObject(dna["attributes"], MaxResponseSize, "power", "speed", "chin", "stamina", "skill"); err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	return result, err
}

// Submit validates the entire returned receipt against the exact expected
// issued job and submitted work. A historical receipt may preserve a different
// valid signature by the same key; this never upgrades an old policy to a new one.
func (client *Client) Submit(ctx context.Context, job prospectjob.Job, proof prospect.Proof, authorization gymauth.Authorization) (AdmissionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if len(proof.MerkleBranch) > 20 {
		return AdmissionResult{}, &ClientError{Code: "invalid_request"}
	}
	proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
	if prospectjob.Validate(job) != nil || job.Context.Network != client.policy.Network ||
		job.Context.Genesis != client.policy.Genesis || proof.Version != prospect.Version2 ||
		!strings.EqualFold(proof.JobID, job.ID) || !strings.EqualFold(authorization.PublicKey, job.PublicKey) ||
		gymauth.VerifyAuthorization(job.Context, proof, authorization) != nil {
		return AdmissionResult{}, &ClientError{Code: "invalid_request"}
	}
	data, err := client.post(ctx, apiPath(client.version, "admissions"), struct {
		Proof         prospect.Proof        `json:"proof"`
		Authorization gymauth.Authorization `json:"authorization"`
	}{proof, authorization}, true)
	if err != nil {
		return AdmissionResult{}, err
	}
	result, err := parseAdmission(data)
	if err != nil {
		return AdmissionResult{}, clientFailure(true)
	}
	record := result.Record
	expectedSchema := registry.Version3
	if client.version == Version2 {
		expectedSchema = registry.Version4
	}
	if result.Version != client.version || result.Authentication != "gym-key-control" || result.BirthRule != "job-issuance" ||
		result.Inserted != result.CurrentNodeChecked ||
		(result.Inserted && (result.ContextSource != "trusted-node-snapshot" || !samePolicy(job.Context, client.policy))) ||
		(!result.Inserted && result.ContextSource != "historical-receipt") ||
		record.Version != expectedSchema || record.Sequence == 0 || record.Job == nil || *record.Job != job ||
		record.Context != job.Context || !sameProof(record.Proof, proof) || record.Authorization == nil ||
		!strings.EqualFold(record.Authorization.PublicKey, job.PublicKey) ||
		gymauth.VerifyAuthorization(job.Context, record.Proof, *record.Authorization) != nil || record.Birth == nil ||
		record.Birth.RuleVersion != job.RuleVersion || record.Birth.BornAtMS != job.IssuedAtMS ||
		record.Birth.AcceptedAtMS < job.IssuedAtMS || record.Birth.AcceptedAtMS >= job.ExpiresAtMS {
		return AdmissionResult{}, clientFailure(true)
	}
	verified, err := prospect.Verify(job.Context, record.Proof)
	if err != nil || verified != record.Verified || ctx.Err() != nil {
		return AdmissionResult{}, clientFailure(true)
	}
	if client.version == Version2 && (record.Eligibility == nil || registry.ValidateEligibility(job.Context, verified, *record.Eligibility) != nil) {
		return AdmissionResult{}, clientFailure(true)
	}
	return result, nil
}

// SignAndSubmit automatically asks the caller's in-memory wallet adapter to
// authorize this proof. It returns the generated public authorization even on
// transport failure, so callers can retain and replay the identical submission.
// The signer must honor ctx; this package never receives or opens a private key.
func (client *Client) SignAndSubmit(ctx context.Context, job prospectjob.Job, proof prospect.Proof,
	signer func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error)) (AdmissionResult, gymauth.Authorization, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if signer == nil || prospectjob.Validate(job) != nil || proof.Version != prospect.Version2 || len(proof.MerkleBranch) > 20 ||
		!strings.EqualFold(job.ID, proof.JobID) || job.Context.Network != client.policy.Network || job.Context.Genesis != client.policy.Genesis {
		return AdmissionResult{}, gymauth.Authorization{}, &ClientError{Code: "invalid_request"}
	}
	if _, err := gymauth.AdmissionDigest(job.Context, proof, job.PublicKey); err != nil {
		return AdmissionResult{}, gymauth.Authorization{}, &ClientError{Code: "invalid_request"}
	}
	proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
	toSign := proof
	toSign.MerkleBranch = append([]string{}, proof.MerkleBranch...)
	authorization, err := signer(ctx, job.Context, toSign)
	if err != nil || ctx.Err() != nil {
		return AdmissionResult{}, authorization, &ClientError{Code: "invalid_request"}
	}
	result, err := client.Submit(ctx, job, proof, authorization)
	return result, authorization, err
}
