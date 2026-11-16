// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"unicode/utf8"

	"github.com/syndtr/goleveldb/leveldb"
)

func challengeIdentity(c EnrollmentChallenge) Identity {
	return Identity{Network: c.Network, Genesis: c.Genesis, Profile: c.Profile, RootPublicKey: c.RootPublicKey, GymPublicKey: c.GymPublicKey, GymID: c.GymID}
}
func (s *Store) BeginChallenge(ctx context.Context, token, action string) (EnrollmentChallenge, error) {
	if err := s.lock(ctx); err != nil {
		return EnrollmentChallenge{}, err
	}
	defer s.unlock()
	_, session, a, now, err := s.authorize(token, Enrollment)
	if err != nil {
		return EnrollmentChallenge{}, err
	}
	if session.Kind != "login" || now-session.AuthenticatedAtMS > FreshAuthenticationMS {
		return EnrollmentChallenge{}, ErrUnauthorized
	}
	if (action == "enroll" && a.Enrollment != nil) || (action == "recover" && (a.Enrollment == nil || !a.RecoveryRequired)) || (action != "enroll" && action != "recover") {
		return EnrollmentChallenge{}, ErrConflict
	}
	if s.meta.Challenges >= s.meta.Config.MaxChallenges {
		return EnrollmentChallenge{}, ErrCapacity
	}
	identity, err := s.fixture(a.PrincipalID)
	if err != nil {
		return EnrollmentChallenge{}, err
	}
	id, err := randomID(s.random)
	if err != nil {
		return EnrollmentChallenge{}, err
	}
	exists, err := s.db.Has([]byte("challenge:"+id), nil)
	if err != nil {
		return EnrollmentChallenge{}, ErrCorrupt
	}
	if exists {
		return EnrollmentChallenge{}, ErrConflict
	}
	challenge := EnrollmentChallenge{Version: Version1, AuthorityID: s.authorityID, AccountID: a.ID, PrincipalID: a.PrincipalID, ChallengeID: id, Generation: a.Generation, Action: action, Network: identity.Network, Genesis: identity.Genesis, Profile: identity.Profile, RootPublicKey: identity.RootPublicKey, GymPublicKey: identity.GymPublicKey, GymID: identity.GymID, IssuedAtMS: now, ExpiresAtMS: now + MaxChallengeLifetimeMS}
	if _, err = ChallengeDigest(challenge); err != nil {
		return EnrollmentChallenge{}, ErrCorrupt
	}
	batch := new(leveldb.Batch)
	next := s.meta
	next.Challenges++
	if put(batch, "challenge:"+id, challengeRecord{Challenge: challenge}) != nil {
		return EnrollmentChallenge{}, ErrCorrupt
	}
	if err = s.event(batch, &next, "challenge", a, now, id); err != nil {
		return EnrollmentChallenge{}, err
	}
	if err = s.commitMutation(ctx, batch, next); err != nil {
		return EnrollmentChallenge{}, err
	}
	return challenge, nil
}

// CompleteChallenge atomically consumes a challenge and fixes the enrollment.
// An exact successful replay in the same generation returns the original result,
// including after challenge expiry; it never renews or changes authorization.
func (s *Store) CompleteChallenge(ctx context.Context, token, id string, auth Authorization) (EnrollmentResult, bool, error) {
	if !validID(id) {
		return EnrollmentResult{}, false, ErrNotFound
	}
	if err := s.lock(ctx); err != nil {
		return EnrollmentResult{}, false, err
	}
	defer s.unlock()
	_, session, a, now, err := s.authorize(token, Enrollment)
	if err != nil {
		return EnrollmentResult{}, false, err
	}
	if session.Kind != "login" || now-session.AuthenticatedAtMS > FreshAuthenticationMS {
		return EnrollmentResult{}, false, ErrUnauthorized
	}
	var record challengeRecord
	if err = s.load("challenge:"+id, &record); err != nil {
		return EnrollmentResult{}, false, err
	}
	c := record.Challenge
	if s.validateChallenge(record) != nil {
		return EnrollmentResult{}, false, ErrCorrupt
	}
	if c.AccountID != a.ID || c.PrincipalID != a.PrincipalID || c.Generation != a.Generation {
		return EnrollmentResult{}, false, ErrUnauthorized
	}
	if VerifyAuthorization(c, auth) != nil {
		return EnrollmentResult{}, false, ErrUnauthorized
	}
	if record.Result != nil {
		if record.Authorization == nil || *record.Authorization != auth {
			return EnrollmentResult{}, false, ErrConflict
		}
		return *record.Result, false, nil
	}
	now, err = s.observe()
	if err != nil {
		return EnrollmentResult{}, false, err
	}
	if err = ctx.Err(); err != nil {
		return EnrollmentResult{}, false, err
	}
	if now >= c.ExpiresAtMS {
		return EnrollmentResult{}, false, ErrExpired
	}
	identity := challengeIdentity(c)
	if c.Action == "enroll" {
		if a.Enrollment != nil {
			return EnrollmentResult{}, false, ErrConflict
		}
	} else if c.Action != "recover" || a.Enrollment == nil || !a.RecoveryRequired || *a.Enrollment != identity {
		return EnrollmentResult{}, false, ErrConflict
	}
	for _, key := range []string{"gym:" + identity.GymID, "root:" + rootID(identity.RootPublicKey)} {
		var owner string
		err = s.load(key, &owner)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return EnrollmentResult{}, false, err
		}
		if err == nil && owner != a.ID {
			return EnrollmentResult{}, false, ErrConflict
		}
	}
	a.Enrollment = &identity
	a.RecoveryRequired = false
	result := EnrollmentResult{Version: Version1, AccountID: a.ID, Generation: a.Generation, ChallengeID: id, Action: c.Action, Identity: identity}
	record.Result = &result
	record.Authorization = &auth
	batch := new(leveldb.Batch)
	next := s.meta
	for key, value := range map[string]any{"account:" + a.ID: a, "challenge:" + id: record, "gym:" + identity.GymID: a.ID, "root:" + rootID(identity.RootPublicKey): a.ID} {
		if put(batch, key, value) != nil {
			return EnrollmentResult{}, false, ErrCorrupt
		}
	}
	if err = s.event(batch, &next, c.Action, a, now, id); err != nil {
		return EnrollmentResult{}, false, err
	}
	if err = s.commitMutation(ctx, batch, next); err != nil {
		return EnrollmentResult{}, false, err
	}
	return result, true, nil
}
func rootID(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:])
}
func resetID(principal, event string) (string, error) {
	if !validID(principal) || len(event) == 0 || len(event) > 256 || !utf8.ValidString(event) {
		return "", ErrUnauthorized
	}
	h := sha256.New()
	h.Write([]byte("SLUG account recovery event\x00v1\x00"))
	raw, _ := hex.DecodeString(principal)
	h.Write(raw)
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(event)))
	h.Write(n[:])
	h.Write([]byte(event))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ApplyReset requires a fresh verified recovery event from the configured
// provider. It retains the exact wallet/gym, revokes every earlier generation,
// and records the fixed 24-hour hold. Replayed events do not restart the hold.
func (s *Store) ApplyReset(ctx context.Context, p Principal) (ResetResult, bool, error) {
	if !p.RecoveryVerified {
		return ResetResult{}, false, ErrUnauthorized
	}
	if err := s.lock(ctx); err != nil {
		return ResetResult{}, false, err
	}
	defer s.unlock()
	principal, err := PrincipalID(p.Issuer, p.Subject)
	if err != nil {
		return ResetResult{}, false, ErrUnauthorized
	}
	if _, err = s.fixture(principal); err != nil {
		return ResetResult{}, false, err
	}
	eventID, err := resetID(principal, p.RecoveryEventID)
	if err != nil {
		return ResetResult{}, false, err
	}
	now, err := s.observe()
	if err != nil {
		return ResetResult{}, false, err
	}
	if _, err = s.principal(p, now); err != nil {
		return ResetResult{}, false, err
	}
	var id string
	if err = s.load("principal:"+principal, &id); err != nil {
		return ResetResult{}, false, err
	}
	a, err := s.account(id)
	if err != nil {
		return ResetResult{}, false, err
	}
	var old ResetResult
	err = s.load("reset:"+eventID, &old)
	if err == nil {
		if s.validateReset(old) != nil || old.AccountID != a.ID {
			return ResetResult{}, false, ErrCorrupt
		}
		return old, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return ResetResult{}, false, err
	}
	if a.Generation == math.MaxUint64 {
		return ResetResult{}, false, ErrCapacity
	}
	a.Generation++
	until := now + RecoveryHoldMS
	if until < a.HoldUntilMS {
		until = a.HoldUntilMS
	}
	a.HoldUntilMS = until
	a.RecoveryRequired = a.Enrollment != nil
	result := ResetResult{Version: Version1, AccountID: a.ID, Generation: a.Generation, EventID: eventID, AtMS: now, HoldUntilMS: until}
	batch := new(leveldb.Batch)
	next := s.meta
	if put(batch, "account:"+a.ID, a) != nil || put(batch, "reset:"+eventID, result) != nil {
		return ResetResult{}, false, ErrCorrupt
	}
	if err = s.event(batch, &next, "reset", a, now, eventID); err != nil {
		return ResetResult{}, false, err
	}
	if err = s.commitMutation(ctx, batch, next); err != nil {
		return ResetResult{}, false, err
	}
	return result, true, nil
}
