// Copyright (c) 2026 The SLUG developers. ISC license.

package prospectservice

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// ManifestClient has one immutable endpoint, transport trust policy and
// explicit candidate authority pin. No discovery, retry or token persistence
// occurs. The pin is configured trust, not an authority signature.
type ManifestClient struct {
	version                uint32
	origin, token, trustID string
	pin                    registry.ManifestAuthorityPin
	policy                 prospectnode.Policy
	http                   *http.Client
}

func NewManifestClient(config ManifestClientConfig, policy prospectnode.Policy) (*ManifestClient, error) {
	if wire.TestPoW {
		return nil, errors.New("manifest service requires production PoW")
	}
	config, err := ValidateManifestClientConfig(config)
	if err != nil {
		return nil, err
	}
	policy, err = manifestPolicy(policy)
	if err != nil {
		return nil, err
	}
	descriptor, err := manifestPin(config.AuthorityPin)
	if err != nil || policy.Network != config.AuthorityPin.Network || policy.Genesis != config.AuthorityPin.Genesis || policy.DNAVersion != descriptor.DNAVersion {
		return nil, errManifestConfig
	}
	trustID, err := ManifestTransportTrustID(config)
	if err != nil {
		return nil, err
	}
	// Reuse the unchanged, bounded TLS/HTTP transport construction, not the
	// old protocol's job, receipt parser or acceptance rules.
	transport, err := NewClient(ClientConfig{Version: Version2, URL: config.URL, Token: config.Token, CACertificate: config.CACertificate}, policy)
	if err != nil {
		return nil, errManifestConfig
	}
	return &ManifestClient{version: Version3, origin: config.URL, token: config.Token, trustID: trustID, pin: config.AuthorityPin, policy: policy, http: transport.http}, nil
}

func (client *ManifestClient) ProtocolVersion() uint32                     { return Version3 }
func (client *ManifestClient) AuthorityOrigin() string                     { return client.origin }
func (client *ManifestClient) AuthorityPin() registry.ManifestAuthorityPin { return client.pin }
func (client *ManifestClient) TransportTrustID() string                    { return client.trustID }

// Close only closes idle connections. Owners cancel and join active operations
// before closing or replacing a client; Close does not drain proof validation.
func (client *ManifestClient) Close() { client.http.CloseIdleConnections() }

func (client *ManifestClient) post(ctx context.Context, path string, value any, submission bool) ([]byte, error) {
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
		if manifestJSONShape(data, MaxResponseSize, 2, nil) != nil {
			return nil, clientFailure(true)
		}
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

func parseManifestAdmission(data []byte) (ManifestAdmissionResult, error) {
	var result ManifestAdmissionResult
	if err := manifestJSONShape(data, MaxResponseSize, 5, map[string]int{"record.proof.merkle_branch": 20}); err != nil {
		return result, err
	}
	fields, err := parseObject(data, MaxResponseSize, "version", "context_source", "current_node_checked", "authentication", "birth_rule", "inserted", "record")
	if err != nil {
		return result, err
	}
	if decodeField(fields, "version", &result.Version) != nil || result.Version != Version3 {
		return result, errors.New("unsupported admission version")
	}
	keys := []string{"version", "sequence", "context", "proof", "verified", "authorization", "job", "birth", "eligibility", "static_binding"}
	record, err := parseObject(fields["record"], registry.MaxRecordSize, keys...)
	if err != nil {
		return result, err
	}
	if result.Version == Version3 {
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
	if _, err := prospectjob.ParseManifestJob(record["job"]); err != nil {
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
	if _, err := parseObject(record["static_binding"], MaxResponseSize, "version", "descriptor_id", "manifest_digest", "portrait_digest"); err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return ManifestAdmissionResult{}, err
	}
	return result, nil
}

func manifestJobMatchesPin(job prospectjob.ManifestJob, pin registry.ManifestAuthorityPin) bool {
	return prospectjob.ValidateManifest(job) == nil && job.AuthorityID == pin.AuthorityID && job.DescriptorID == pin.DescriptorID && job.Context.Network == pin.Network && job.Context.Genesis == pin.Genesis
}

// IssueManifestJob checks the full typed assertion without a local clock gate.
// A lost response can strand a durable job; retrying means new issuance.
func (client *ManifestClient) IssueManifestJob(ctx context.Context, key string) (prospectjob.ManifestJob, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	key, err := normalizePublicKey(key)
	if err != nil {
		return prospectjob.ManifestJob{}, &ClientError{Code: "invalid_request"}
	}
	if ctx.Err() != nil {
		return prospectjob.ManifestJob{}, &ClientError{Code: "unavailable"}
	}
	data, err := client.post(ctx, apiPath(Version3, "jobs"), struct {
		PublicKey string `json:"public_key"`
	}{key}, true)
	if err != nil {
		return prospectjob.ManifestJob{}, err
	}
	job, err := prospectjob.ParseManifestJob(data)
	if err != nil || !manifestJobMatchesPin(job, client.pin) || job.PublicKey != key || !samePolicy(job.Context, client.policy) || ctx.Err() != nil {
		return prospectjob.ManifestJob{}, clientFailure(true)
	}
	return job, nil
}

func manifestOwnedEvidence(proof prospect.Proof, auth gymauth.Authorization) (prospect.Proof, gymauth.Authorization, error) {
	bad := errors.New("invalid manifest evidence")
	if proof.Version != prospect.Version2 || auth.Version != gymauth.Version2 || len(proof.MerkleBranch) > 20 {
		return prospect.Proof{}, gymauth.Authorization{}, bad
	}
	normalize := func(value string, min, max int) (string, error) {
		if len(value) < min*2 || len(value) > max*2 || len(value)%2 != 0 {
			return "", bad
		}
		if _, err := hex.DecodeString(value); err != nil {
			return "", bad
		}
		return strings.ToLower(value), nil
	}
	for _, field := range []struct {
		target   *string
		min, max int
	}{
		{&proof.Header, 80, 80}, {&proof.Coinbase, 1, 16384}, {&proof.JobID, 32, 32}, {&proof.GymID, 32, 32}, {&auth.PublicKey, 33, 33}, {&auth.Signature, 8, 72},
	} {
		canonical, err := normalize(*field.target, field.min, field.max)
		if err != nil {
			return prospect.Proof{}, gymauth.Authorization{}, bad
		}
		*field.target = canonical
	}
	branch := make([]string, len(proof.MerkleBranch))
	for i, value := range proof.MerkleBranch {
		canonical, err := normalize(value, 32, 32)
		if err != nil {
			return prospect.Proof{}, gymauth.Authorization{}, bad
		}
		branch[i] = canonical
	}
	proof.MerkleBranch = branch
	return proof, auth, nil
}

// This private derivation is reached only after this client's full normal-PoW
// verification. It is not a public supplied-Verified-to-admission projector.
func manifestResponseBinding(pin registry.ManifestAuthorityPin, verified prospect.Verified) (registry.StaticBinding, error) {
	descriptor, err := fighterdescriptor.ResolveCandidate(pin.DescriptorID)
	if err != nil || !manifestHex(verified.WorkHash, 32) {
		return registry.StaticBinding{}, errors.New("invalid manifest binding")
	}
	m, err := fightermanifest.GenerateText(descriptor.ManifestVersion, verified.WorkHash)
	if err != nil || m.DNA != verified.DNA || m.Version != descriptor.ManifestVersion || m.DNA.DNAVersion != descriptor.DNAVersion || m.Generator != descriptor.GeneratorID || m.Tables != descriptor.ManifestTableHash || m.Portrait.Version != descriptor.PortraitTraitVersion {
		return registry.StaticBinding{}, errors.New("invalid manifest binding")
	}
	manifestDigest, err := fightermanifest.Digest(m)
	if err != nil {
		return registry.StaticBinding{}, errors.New("invalid manifest binding")
	}
	// The resolved descriptor is a frozen full tuple, and Digest enforces its
	// exact renderer/manifest version pairing.
	portraitDigest, err := fighterportrait.Digest(descriptor.RendererVersion, m)
	if err != nil {
		return registry.StaticBinding{}, errors.New("invalid manifest binding")
	}
	return registry.StaticBinding{Version: registry.StaticBindingVersion1, DescriptorID: pin.DescriptorID, ManifestDigest: manifestDigest, PortraitDigest: portraitDigest}, nil
}

// SubmitManifest recovers identical historical evidence even if the client's
// current target changed. Every returned receipt is independently verified;
// malformed/cancelled success after dispatch is always outcome-unknown.
func (client *ManifestClient) SubmitManifest(ctx context.Context, job prospectjob.ManifestJob, proof prospect.Proof, auth gymauth.Authorization) (ManifestAdmissionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	proof, auth, err := manifestOwnedEvidence(proof, auth)
	if err != nil || !manifestJobMatchesPin(job, client.pin) || proof.JobID != job.ID || proof.GymID != job.GymID || auth.PublicKey != job.PublicKey || gymauth.VerifyAuthorization(job.Context, proof, auth) != nil {
		return ManifestAdmissionResult{}, &ClientError{Code: "invalid_request"}
	}
	if ctx.Err() != nil {
		return ManifestAdmissionResult{}, &ClientError{Code: "unavailable"}
	}
	data, err := client.post(ctx, apiPath(Version3, "admissions"), struct {
		Proof         prospect.Proof        `json:"proof"`
		Authorization gymauth.Authorization `json:"authorization"`
	}{proof, auth}, true)
	if err != nil {
		return ManifestAdmissionResult{}, err
	}
	result, err := parseManifestAdmission(data)
	if err != nil {
		return ManifestAdmissionResult{}, clientFailure(true)
	}
	record := result.Record
	canonicalProof, canonicalAuth, err := manifestOwnedEvidence(record.Proof, record.Authorization)
	if err != nil || !reflect.DeepEqual(canonicalProof, record.Proof) || canonicalAuth != record.Authorization ||
		result.Version != Version3 || result.Authentication != "gym-key-control" || result.BirthRule != "job-issuance" || result.Inserted != result.CurrentNodeChecked ||
		(result.Inserted && (result.ContextSource != "trusted-node-snapshot" || !samePolicy(job.Context, client.policy))) || (!result.Inserted && result.ContextSource != "historical-receipt") ||
		record.Version != registry.Version5 || record.Sequence == 0 || record.Job != job || record.Context != job.Context || !sameProof(record.Proof, proof) || record.Authorization.PublicKey != job.PublicKey || gymauth.VerifyAuthorization(job.Context, record.Proof, record.Authorization) != nil ||
		record.Birth.RuleVersion != job.RuleVersion || record.Birth.BornAtMS != job.IssuedAtMS || record.Birth.AcceptedAtMS < job.IssuedAtMS || record.Birth.AcceptedAtMS >= job.ExpiresAtMS {
		return ManifestAdmissionResult{}, clientFailure(true)
	}
	if ctx.Err() != nil {
		return ManifestAdmissionResult{}, clientFailure(true)
	}
	verified, err := prospect.Verify(job.Context, record.Proof)
	if err != nil || verified != record.Verified || ctx.Err() != nil || registry.ValidateEligibility(job.Context, verified, record.Eligibility) != nil {
		return ManifestAdmissionResult{}, clientFailure(true)
	}
	binding, err := manifestResponseBinding(client.pin, verified)
	if err != nil || binding != record.StaticBinding || ctx.Err() != nil {
		return ManifestAdmissionResult{}, clientFailure(true)
	}
	return result, nil
}
