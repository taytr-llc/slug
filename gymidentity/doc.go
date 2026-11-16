// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

// Package gymidentity verifies a bounded RS256 identity assertion profile for
// the private fixture account authority. It requires a provider-specific signed
// original-authentication credential epoch and a separate, signed two-stage
// reset protocol. Ordinary OIDC support alone does not satisfy this contract.
//
// Public trust is explicit and immutable. There is no discovery, HTTP transport,
// key refresh, token storage, credential creation, or production provider here.
// VerifiedLogin and VerifiedReset are trusted native adapter output; they must
// never be populated from an unverified request or round-tripped through JS
// numbers. Their public trust digest is pinned by a schema-2 gymaccount store.
//
// The shipped RSA exponent and two wallet identities are published fixtures,
// not secrets or production authority. The default desktop runtime is unchanged.
package gymidentity
