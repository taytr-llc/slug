// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymidentity

import (
	"context"
	"math"
	"strings"

	"github.com/taytr-llc/slug/gymaccount"
)

// CoordinatorBinding comes from the coordinator's pinned configuration and
// durable marker, never from unverified request claims. All fields are values.
type CoordinatorBinding struct {
	Version                          uint32
	CoordinatorID, AuthorityID       string
	TenantID, ClientID, ConnectionID string
}

// CoordinatedLogin is trusted verifier output, not a request DTO. Its signed
// origin still needs the coordinator journal's immutability/reuse checks, then
// the authority's atomic nonce, epoch and pending-reset checks.
type CoordinatedLogin struct {
	Login                                        gymaccount.VerifiedLogin
	FlowID, OriginalAuthID, OriginalFlowID, Mode string
}

// CoordinatedReset binds a signed provider operation to the issued reset
// request. It does not itself grant permission to mutate provider credentials;
// only the coordinator's durable, currently pending begin acknowledgement can.
type CoordinatedReset struct {
	Reset                           gymaccount.VerifiedReset
	ProviderOperationID, ResetNonce string
}

func coordinatorName(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') &&
			(c < '0' || c > '9') && c != '.' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func validCoordinatorBinding(b CoordinatorBinding) bool {
	return b.Version == Version1 && canonicalID(b.CoordinatorID) &&
		canonicalID(b.AuthorityID) && coordinatorName(b.TenantID) &&
		coordinatorName(b.ClientID) && coordinatorName(b.ConnectionID)
}

func exactCoordinatedFields(m map[string]any, names ...string) bool {
	return len(m) == len(names) && onlyFields(m, names...)
}

// coordinatedAssertion adds the closed v1 header requirement while reusing the
// existing bounded parser, pinned RSA verification and canonical claim handling.
// Re-reading the small header avoids changing legacy VerifyLogin's optional typ.
func (v *Verifier) coordinatedAssertion(ctx context.Context, token string, reset bool) (map[string]any, error) {
	if ctx == nil || v == nil || v.now == nil || len(v.keys) == 0 ||
		v.digest == "" || v.epochClaim != "slug_auth_epoch" {
		return nil, ErrConfig
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(token) == 0 || len(token) > MaxAssertionSize || strings.Count(token, ".") != 2 {
		return nil, ErrAssertion
	}
	headerRaw, err := rawURL(strings.SplitN(token, ".", 2)[0], MaxHeaderSize)
	if err != nil {
		return nil, ErrAssertion
	}
	header, err := parseObject(headerRaw, MaxHeaderSize)
	typ := "JWT"
	if reset {
		typ = "slug-reset+jwt"
	}
	if err != nil || !exactCoordinatedFields(header, "alg", "kid", "typ") || header["typ"] != typ {
		return nil, ErrAssertion
	}
	return v.assertion(ctx, token, reset)
}

func coordinatedClaimsMatch(claims map[string]any, binding CoordinatorBinding) bool {
	version, err := uintField(claims, "provider_version")
	return err == nil && version == uint64(Version1) &&
		claims["coordinator_id"] == binding.CoordinatorID &&
		claims["authority_id"] == binding.AuthorityID &&
		claims["tenant_id"] == binding.TenantID &&
		claims["client_id"] == binding.ClientID &&
		claims["connection_id"] == binding.ConnectionID
}

func coordinatedID(claims map[string]any, name string) (string, error) {
	id, ok := claims[name].(string)
	if !ok || !canonicalID(id) {
		return "", ErrAssertion
	}
	return id, nil
}

// VerifyCoordinatedLogin verifies the exact signed coordinator protocol. The
// expected flow and nonce must come from a persisted issued flow. A fresh claim
// must originate in that flow; a reuse claim's accepted origin and immutable
// authentication time/epoch are checked by the coordinator, never inferred here.
// No session or account mutation occurs in this method.
func (v *Verifier) VerifyCoordinatedLogin(ctx context.Context, token string,
	binding CoordinatorBinding, flowID, nonce string) (CoordinatedLogin, error) {

	var none CoordinatedLogin
	if !validCoordinatorBinding(binding) {
		return none, ErrConfig
	}
	if !canonicalID(flowID) || !canonicalID(nonce) {
		return none, ErrAssertion
	}
	claims, err := v.coordinatedAssertion(ctx, token, false)
	if err != nil {
		return none, err
	}
	if !exactCoordinatedFields(claims, "iss", "aud", "sub", "iat", "exp", "auth_time", "nonce", "slug_auth_epoch",
		"provider_version", "coordinator_id", "authority_id", "tenant_id", "client_id", "connection_id",
		"login_flow_id", "original_auth_id", "original_flow_id", "auth_mode") ||
		!coordinatedClaimsMatch(claims, binding) || claims["aud"] != v.audience ||
		claims["login_flow_id"] != flowID || claims["nonce"] != nonce {
		return none, ErrAssertion
	}
	subject, issued, expires, now, err := v.common(claims)
	if err != nil {
		return none, err
	}
	authenticated, err := secondsField(claims, "auth_time")
	if err != nil || authenticated > issued || now-authenticated > FreshnessSeconds*1000 {
		return none, ErrAssertion
	}
	epoch, err := uintField(claims, "slug_auth_epoch")
	if err != nil || epoch == 0 {
		return none, ErrAssertion
	}
	origin, err := coordinatedID(claims, "original_auth_id")
	if err != nil {
		return none, err
	}
	originFlow, err := coordinatedID(claims, "original_flow_id")
	if err != nil {
		return none, err
	}
	mode, ok := claims["auth_mode"].(string)
	if !ok || (mode != "fresh" && mode != "reuse") || (mode == "fresh" && originFlow != flowID) {
		return none, ErrAssertion
	}
	if err := ctx.Err(); err != nil {
		return none, err
	}
	return CoordinatedLogin{
		Login: gymaccount.VerifiedLogin{VerifierConfigHash: v.digest, Issuer: v.issuer,
			Subject: subject, Nonce: nonce, AuthenticatedAtMS: authenticated,
			IssuedAtMS: issued, ExpiresAtMS: expires, CredentialEpoch: epoch},
		FlowID: flowID, OriginalAuthID: origin, OriginalFlowID: originFlow, Mode: mode,
	}, nil
}

// VerifyCoordinatedReset authenticates the separate reset audience and type,
// full provider-operation binding, and exact consecutive epochs. Fresh transport
// can attest an old immutable verified_at; the authority applies freshness only
// to a new begin and matches the stored event on reconciliation.
func (v *Verifier) VerifyCoordinatedReset(ctx context.Context, token string,
	binding CoordinatorBinding, eventID, nonce string) (CoordinatedReset, error) {

	var none CoordinatedReset
	if !validCoordinatorBinding(binding) {
		return none, ErrConfig
	}
	if !canonicalID(eventID) || !canonicalID(nonce) {
		return none, ErrAssertion
	}
	claims, err := v.coordinatedAssertion(ctx, token, true)
	if err != nil {
		return none, err
	}
	if !exactCoordinatedFields(claims, "iss", "aud", "sub", "iat", "exp", "event_id", "stage", "from_epoch", "to_epoch", "verified_at",
		"provider_version", "coordinator_id", "authority_id", "tenant_id", "client_id", "connection_id",
		"reset_nonce", "provider_operation_id") ||
		!coordinatedClaimsMatch(claims, binding) || claims["aud"] != v.resetAudience ||
		claims["event_id"] != eventID || claims["reset_nonce"] != nonce {
		return none, ErrAssertion
	}
	subject, issued, expires, _, err := v.common(claims)
	if err != nil {
		return none, err
	}
	stage, ok := claims["stage"].(string)
	if !ok || (stage != "begin" && stage != "complete") {
		return none, ErrAssertion
	}
	from, err := uintField(claims, "from_epoch")
	if err != nil || from == 0 || from == math.MaxUint64 {
		return none, ErrAssertion
	}
	to, err := uintField(claims, "to_epoch")
	if err != nil || to != from+1 {
		return none, ErrAssertion
	}
	verified, err := secondsField(claims, "verified_at")
	if err != nil || verified > issued {
		return none, ErrAssertion
	}
	operation, err := coordinatedID(claims, "provider_operation_id")
	if err != nil {
		return none, err
	}
	if err := ctx.Err(); err != nil {
		return none, err
	}
	return CoordinatedReset{
		Reset: gymaccount.VerifiedReset{VerifierConfigHash: v.digest, Issuer: v.issuer,
			Subject: subject, EventID: eventID, Stage: stage, FromEpoch: from, ToEpoch: to,
			VerifiedAtMS: verified, IssuedAtMS: issued, ExpiresAtMS: expires},
		ProviderOperationID: operation, ResetNonce: nonce,
	}, nil
}
