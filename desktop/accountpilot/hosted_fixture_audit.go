// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymprovider"
)

func hostedHash(v string) bool {
	if len(v) != 64 || strings.ToLower(v) != v {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil
}
func hostedID(v string) bool { return hostedHash(v) && v != strings.Repeat("0", 64) }
func hostedFixtureIndex(v string) int {
	for i, f := range hostedFixtures {
		if v == f {
			return i
		}
	}
	return -1
}
func hostedTime(v int64) bool { return v > 0 && v <= hostedTimeMax }
func hostedFlowValid(f gymprovider.LoginFlow, c gymprovider.Config, id string) bool {
	return f.Version == 1 && f.CoordinatorID == id && f.AuthorityID == c.AuthorityID && f.Issuer == gymprovider.FixtureIssuer && hostedHash(f.ID) && hostedHash(f.AttemptID) && hostedHash(f.Nonce) && f.Sequence > 0 && hostedTime(f.IssuedAtMS) && hostedTime(f.ExpiresAtMS) && f.ExpiresAtMS-f.IssuedAtMS == gymprovider.LifetimeMS
}
func hostedRequestValid(r gymprovider.ResetRequest, c gymprovider.Config, id string) bool {
	return r.Version == 1 && r.CoordinatorID == id && r.AuthorityID == c.AuthorityID && hostedHash(r.EventID) && hostedHash(r.Nonce) && hostedTime(r.IssuedAtMS) && hostedTime(r.ExpiresAtMS) && r.ExpiresAtMS-r.IssuedAtMS == gymprovider.LifetimeMS
}
func hostedBarrierID(subject, event string) string {
	pid, _ := gymaccount.PrincipalID(gymprovider.FixtureIssuer, subject)
	p, _ := hex.DecodeString(pid)
	h := sha256.New()
	h.Write([]byte("SLUG account recovery event\x00v1\x00"))
	h.Write(p)
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(event)))
	h.Write(n[:])
	h.Write([]byte(event))
	return hex.EncodeToString(h.Sum(nil))
}
func hostedAckValid(row hostedReset, a gymprovider.ResetResult, complete bool) bool {
	b := a.Barrier
	pid, _ := gymaccount.PrincipalID(gymprovider.FixtureIssuer, row.Subject)
	if a.Version != 1 || a.CoordinatorID != row.Request.CoordinatorID || a.AuthorityID != row.Request.AuthorityID || a.EventID != row.Request.EventID || a.ProviderOperationID != row.OperationID || b.Version != 2 || b.EventID != hostedBarrierID(row.Subject, row.Request.EventID) || b.AccountID != row.AccountID || b.PrincipalID != pid || b.Generation != row.ToEpoch || b.FromEpoch != row.FromEpoch || b.ToEpoch != row.ToEpoch || b.VerifiedAtMS != row.VerifiedAtMS || !hostedTime(b.BeganAtMS) || b.BeganAtMS < row.VerifiedAtMS || b.BeginSequence == 0 {
		return false
	}
	if !complete {
		return a.Phase == "pending" && a.CanCommit && b.Status == "pending" && b.CompletedAtMS == 0 && b.HoldUntilMS == 0 && b.CompleteSequence == 0
	}
	if a.Phase != "completed" || a.CanCommit || b.Status != "completed" || !hostedTime(b.CompletedAtMS) || b.CompletedAtMS < b.BeganAtMS || b.CompletedAtMS > hostedTimeMax-gymaccount.RecoveryHoldMS || b.HoldUntilMS != b.CompletedAtMS+gymaccount.RecoveryHoldMS || b.CompleteSequence <= b.BeginSequence || row.BeginAck == nil {
		return false
	}
	old := row.BeginAck.Barrier
	copy := b
	copy.Status = old.Status
	copy.CompletedAtMS = old.CompletedAtMS
	copy.HoldUntilMS = old.HoldUntilMS
	copy.CompleteSequence = old.CompleteSequence
	return copy == old
}
func hostedCloneState(s hostedDriverState) hostedDriverState {
	s.Epochs = append([]hostedEpoch{}, s.Epochs...)
	s.Auth = append([]hostedAuth{}, s.Auth...)
	s.Resets = append([]hostedReset{}, s.Resets...)
	for i := range s.Resets {
		r := &s.Resets[i]
		if r.BeginAck != nil {
			a := *r.BeginAck
			r.BeginAck = &a
		}
		if r.CompleteAck != nil {
			a := *r.CompleteAck
			r.CompleteAck = &a
		}
	}
	return s
}
func auditHostedDriver(s hostedDriverState, c gymprovider.Config, coordinator string, accounts map[string]string) error {
	bad := gymprovider.ErrCorrupt
	if s.Version != 1 || s.DriverID != hostedDriverID || s.CoordinatorID != coordinator || s.Config != c || s.Revision == 0 || s.ClockHighWaterMS < 0 || s.ClockHighWaterMS > hostedTimeMax || len(s.Epochs) != 2 || s.Auth == nil || len(s.Auth) > 1024 || s.Resets == nil || len(s.Resets) > 256 {
		return bad
	}
	epochs := [2]uint64{1, 1}
	pending := [2]string{}
	maxTime := int64(0)
	minimum := uint64(1 + len(s.Auth) + len(s.Resets))
	events, ops := map[string]bool{}, map[string]bool{}
	for i, r := range s.Resets {
		at := hostedFixtureIndex(r.Fixture)
		raw, _ := json.Marshal(r)
		if at < 0 || r.Subject != r.Fixture || r.Version != 1 || r.Sequence != uint32(i+1) || !hostedHash(r.AccountID) || accounts != nil && accounts[r.Fixture] != r.AccountID || !hostedID(r.OperationID) || ops[r.OperationID] || !hostedRequestValid(r.Request, c, coordinator) || events[r.Request.EventID] || r.FromEpoch == 0 || r.FromEpoch == math.MaxUint64 || r.FromEpoch != epochs[at] || r.ToEpoch != r.FromEpoch+1 || pending[at] != "" || !hostedTime(r.VerifiedAtMS) || r.VerifiedAtMS < 1000 || r.VerifiedAtMS%1000 != 0 || r.VerifiedAtMS/1000 < r.Request.IssuedAtMS/1000 || r.VerifiedAtMS >= r.Request.ExpiresAtMS || len(raw) > 4096 {
			return bad
		}
		events[r.Request.EventID], ops[r.OperationID] = true, true
		maxTime = max(maxTime, r.VerifiedAtMS)
		switch r.Phase {
		case "verified", "retired":
			if r.BeginAck != nil || r.CompleteAck != nil {
				return bad
			}
			if r.Phase == "verified" {
				pending[at] = r.Request.EventID
			} else {
				minimum++
				if s.ClockHighWaterMS < r.Request.ExpiresAtMS {
					return bad
				}
			}
		case "committed", "completed":
			minimum++
			if r.BeginAck == nil || !hostedAckValid(r, *r.BeginAck, false) {
				return bad
			}
			epochs[at] = r.ToEpoch
			maxTime = max(maxTime, r.BeginAck.Barrier.BeganAtMS)
			if r.Phase == "committed" {
				if r.CompleteAck != nil {
					return bad
				}
				pending[at] = r.Request.EventID
			} else {
				minimum++
				if r.CompleteAck == nil || !hostedAckValid(r, *r.CompleteAck, true) {
					return bad
				}
				maxTime = max(maxTime, r.CompleteAck.Barrier.CompletedAtMS)
			}
		default:
			return bad
		}
	}
	if uint64(s.Revision) < minimum {
		return bad
	}
	for i, r := range s.Epochs {
		if r.Fixture != hostedFixtures[i] || r.Subject != r.Fixture || r.CredentialEpoch != epochs[i] || r.PendingEventID != pending[i] {
			return bad
		}
	}
	auth, flows := map[string]bool{}, map[string]bool{}
	for i, r := range s.Auth {
		at := hostedFixtureIndex(r.Fixture)
		raw, _ := json.Marshal(r)
		if at < 0 || r.Subject != r.Fixture || r.Version != 1 || r.Sequence != uint32(i+1) || !hostedID(r.AuthID) || auth[r.AuthID] || !hostedFlowValid(r.Flow, c, coordinator) || flows[r.Flow.ID] || r.CredentialEpoch == 0 || r.CredentialEpoch > epochs[at] || !hostedTime(r.AuthenticatedAtMS) || r.AuthenticatedAtMS < 1000 || r.AuthenticatedAtMS%1000 != 0 || r.AuthenticatedAtMS/1000 < r.Flow.IssuedAtMS/1000 || r.AuthenticatedAtMS >= r.Flow.ExpiresAtMS || len(raw) > 2048 {
			return bad
		}
		auth[r.AuthID], flows[r.Flow.ID] = true, true
		maxTime = max(maxTime, r.AuthenticatedAtMS)
	}
	if s.ClockHighWaterMS < maxTime {
		return bad
	}
	raw, e := json.Marshal(s)
	if e != nil || len(raw)+1 > hostedDriverMax {
		return bad
	}
	return nil
}
