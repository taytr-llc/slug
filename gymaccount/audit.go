// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import (
	"fmt"
	"math"
	"strings"
)

func (s *Store) validateChallenge(r challengeRecord) error {
	c := r.Challenge
	if _, err := ChallengeDigest(c); err != nil {
		return ErrCorrupt
	}
	a, err := s.account(c.AccountID)
	if err != nil {
		return ErrCorrupt
	}
	expected, err := s.fixture(a.PrincipalID)
	if err != nil || c.AuthorityID != s.authorityID || c.PrincipalID != a.PrincipalID || c.Generation > a.Generation || c.IssuedAtMS > s.meta.ClockMS || c.ExpiresAtMS-c.IssuedAtMS != MaxChallengeLifetimeMS || challengeIdentity(c) != expected {
		return ErrCorrupt
	}
	if (r.Result == nil) != (r.Authorization == nil) {
		return ErrCorrupt
	}
	if r.Result != nil {
		want := EnrollmentResult{Version: Version1, AccountID: c.AccountID, Generation: c.Generation, ChallengeID: c.ChallengeID, Action: c.Action, Identity: expected}
		if *r.Result != want || VerifyAuthorization(c, *r.Authorization) != nil || a.Enrollment == nil || *a.Enrollment != expected {
			return ErrCorrupt
		}
	}
	return nil
}
func (s *Store) validateReset(r ResetResult) error {
	a, err := s.account(r.AccountID)
	if err != nil {
		return ErrCorrupt
	}
	if r.Version != Version1 || !validID(r.EventID) || r.Generation < 2 || r.Generation > a.Generation || r.AtMS <= 0 || r.AtMS > s.meta.ClockMS || r.HoldUntilMS != r.AtMS+RecoveryHoldMS || r.HoldUntilMS > a.HoldUntilMS {
		return ErrCorrupt
	}
	return nil
}
func (s *Store) audit() error {
	if s.meta.ClockMS < 0 || s.meta.ClockMS > math.MaxInt64-RecoveryHoldMS || s.meta.Accounts > 2 || s.meta.Sessions > s.meta.Config.MaxSessions || s.meta.Challenges > s.meta.Config.MaxChallenges || s.meta.Events > s.meta.Config.MaxEvents {
		return ErrCorrupt
	}
	var accounts, sessions, challenges, events, resets uint32
	indexes := map[string]string{}
	seenIndexes := map[string]string{}
	challengeIssues := map[string]uint32{}
	challengeCompletions := map[string]uint32{}
	challengeConsumed := map[string]bool{}
	resetEvents := map[string]uint32{}
	firstLogins := map[string]bool{}
	accountRows := map[string]Account{}
	resetsByAccount := map[string]map[uint64]ResetResult{}
	completionsByAccount := map[string]map[uint64]string{}
	iterator := s.db.NewIterator(nil, nil)
	defer iterator.Release()
	maxKeys := 1 + 2*4 + int(s.meta.Config.MaxSessions) + int(s.meta.Config.MaxChallenges) + int(s.meta.Config.MaxEvents)*2
	count := 0
	for iterator.Next() {
		count++
		if count > maxKeys || len(iterator.Key()) > 100 || len(iterator.Value()) > MaxRecordSize {
			return ErrCorrupt
		}
		key := string(iterator.Key())
		raw := iterator.Value()
		switch {
		case key == "meta":
		case strings.HasPrefix(key, "account:"):
			var a Account
			if canonical(raw, &a) != nil || key != "account:"+a.ID || s.validateAccount(a) != nil {
				return ErrCorrupt
			}
			accounts++
			accountRows[a.ID] = a
			if _, exists := indexes["principal:"+a.PrincipalID]; exists {
				return ErrCorrupt
			}
			indexes["principal:"+a.PrincipalID] = a.ID
			if a.Enrollment != nil {
				for _, index := range []string{"gym:" + a.Enrollment.GymID, "root:" + rootID(a.Enrollment.RootPublicKey)} {
					if _, exists := indexes[index]; exists {
						return ErrCorrupt
					}
					indexes[index] = a.ID
				}
			}
		case strings.HasPrefix(key, "principal:") || strings.HasPrefix(key, "gym:") || strings.HasPrefix(key, "root:"):
			var id string
			if canonical(raw, &id) != nil || !validID(id) {
				return ErrCorrupt
			}
			seenIndexes[key] = id
		case strings.HasPrefix(key, "session:"):
			if !validID(strings.TrimPrefix(key, "session:")) {
				return ErrCorrupt
			}
			var record sessionRecord
			if canonical(raw, &record) != nil {
				return ErrCorrupt
			}
			a, err := s.account(record.AccountID)
			if err != nil || s.validateSession(record, a) != nil {
				return ErrCorrupt
			}
			sessions++
			if record.Kind == "mining" {
				var parent sessionRecord
				if s.load("session:"+record.Parent, &parent) != nil || parent.Kind != "login" || s.validateSession(parent, a) != nil || parent.Generation != record.Generation || parent.AccountID != record.AccountID || record.ExpiresAtMS > parent.ExpiresAtMS {
					return ErrCorrupt
				}
			}
		case strings.HasPrefix(key, "challenge:"):
			var record challengeRecord
			if canonical(raw, &record) != nil || key != "challenge:"+record.Challenge.ChallengeID || s.validateChallenge(record) != nil {
				return ErrCorrupt
			}
			challenges++
			challengeConsumed[record.Challenge.ChallengeID] = record.Result != nil
		case strings.HasPrefix(key, "reset:"):
			var record ResetResult
			if canonical(raw, &record) != nil || key != "reset:"+record.EventID || s.validateReset(record) != nil {
				return ErrCorrupt
			}
			resets++
			if resetsByAccount[record.AccountID] == nil {
				resetsByAccount[record.AccountID] = map[uint64]ResetResult{}
			}
			if _, exists := resetsByAccount[record.AccountID][record.Generation]; exists {
				return ErrCorrupt
			}
			resetsByAccount[record.AccountID][record.Generation] = record
		case strings.HasPrefix(key, "event:"):
			var event Event
			if canonical(raw, &event) != nil || event.Version != Version1 || event.Sequence == 0 || event.Sequence > uint64(s.meta.Events) || key != fmt.Sprintf("event:%016x", event.Sequence) || event.AtMS <= 0 || event.AtMS > s.meta.ClockMS {
				return ErrCorrupt
			}
			events++
			a, err := s.account(event.AccountID)
			if err != nil || event.Generation == 0 || event.Generation > a.Generation {
				return ErrCorrupt
			}
			switch event.Kind {
			case "login":
				if event.Generation == 1 {
					firstLogins[a.ID] = true
				}
				if event.Reference != "" {
					return ErrCorrupt
				}
			case "logout", "rotation", "mining-session":
				if event.Reference != "" {
					return ErrCorrupt
				}
			case "challenge", "enroll", "recover":
				var c challengeRecord
				if s.load("challenge:"+event.Reference, &c) != nil || c.Challenge.AccountID != a.ID || c.Challenge.Generation != event.Generation {
					return ErrCorrupt
				}
				if event.Kind == "challenge" {
					if event.AtMS != c.Challenge.IssuedAtMS {
						return ErrCorrupt
					}
					challengeIssues[event.Reference]++
				} else {
					if c.Result == nil || c.Challenge.Action != event.Kind || event.AtMS < c.Challenge.IssuedAtMS || event.AtMS >= c.Challenge.ExpiresAtMS {
						return ErrCorrupt
					}
					challengeCompletions[event.Reference]++
					if completionsByAccount[a.ID] == nil {
						completionsByAccount[a.ID] = map[uint64]string{}
					}
					if _, exists := completionsByAccount[a.ID][event.Generation]; exists {
						return ErrCorrupt
					}
					completionsByAccount[a.ID][event.Generation] = event.Kind
				}
			case "reset":
				var r ResetResult
				if s.load("reset:"+event.Reference, &r) != nil || r.AccountID != a.ID || r.Generation != event.Generation || r.AtMS != event.AtMS {
					return ErrCorrupt
				}
				resetEvents[event.Reference]++
			default:
				return ErrCorrupt
			}
		default:
			return ErrCorrupt
		}
	}
	if iterator.Error() != nil || accounts != s.meta.Accounts || sessions != s.meta.Sessions || challenges != s.meta.Challenges || events != s.meta.Events || len(indexes) != len(seenIndexes) || resets != uint32(len(resetEvents)) {
		return ErrCorrupt
	}
	for key, id := range indexes {
		if seenIndexes[key] != id {
			return ErrCorrupt
		}
		if strings.HasPrefix(key, "principal:") && !firstLogins[id] {
			return ErrCorrupt
		}
	}
	for _, count := range resetEvents {
		if count != 1 {
			return ErrCorrupt
		}
	}
	for id, account := range accountRows {
		resets := resetsByAccount[id]
		if uint64(len(resets))+1 != account.Generation {
			return ErrCorrupt
		}
		var lastAt int64
		for generation := uint64(2); generation <= account.Generation; generation++ {
			reset, found := resets[generation]
			if !found || reset.AtMS < lastAt {
				return ErrCorrupt
			}
			lastAt = reset.AtMS
		}
		if account.Generation == 1 && account.HoldUntilMS != 0 {
			return ErrCorrupt
		}
		if account.Generation > 1 && account.HoldUntilMS != resets[account.Generation].HoldUntilMS {
			return ErrCorrupt
		}
		completed := completionsByAccount[id]
		var enrollGeneration uint64
		for generation, action := range completed {
			if action == "enroll" {
				if enrollGeneration != 0 {
					return ErrCorrupt
				}
				enrollGeneration = generation
			}
		}
		if (account.Enrollment != nil) != (enrollGeneration != 0) {
			return ErrCorrupt
		}
		for generation, action := range completed {
			if action == "recover" && generation <= enrollGeneration {
				return ErrCorrupt
			}
		}
		_, currentCompletion := completed[account.Generation]
		if account.RecoveryRequired != (account.Enrollment != nil && !currentCompletion) {
			return ErrCorrupt
		}
	}
	// Every issued/consumed challenge has exactly its corresponding audit entries.
	for id, consumed := range challengeConsumed {
		if challengeIssues[id] != 1 || (consumed && challengeCompletions[id] != 1) || (!consumed && challengeCompletions[id] != 0) {
			return ErrCorrupt
		}
	}
	return nil
}
