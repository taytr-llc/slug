// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

func issueKey(prefix, id string) []byte {
	b, _ := hex.DecodeString(id)
	return append([]byte(prefix), b...)
}
func (s *ManifestStore) issueKeyAllowed(k []byte) bool {
	return s.issue != nil && len(k) == 40 && (bytes.HasPrefix(k, []byte("issue/r/")) || bytes.HasPrefix(k, []byte("issue/j/")))
}
func (s *ManifestStore) decodeIssueRow(raw []byte, id string) (IssuerRequest, error) {
	var r IssuerRequest
	if decodeManifestJSON(raw, MaxIssueRowSize, 1, &r) != nil {
		return r, ErrCorrupt
	}
	pid, _ := IssueProfileID(s.issue.profile)
	if r.Version != IssueVersion1 || r.ProfileID != pid || r.RequestID != id || !publicHex(id, 32, true) || !publicHex(r.IntentID, 32, true) || ValidateIssueIntent(IssueIntent{IssueVersion1, pid, id, r.GymID, r.PublicKey, r.IntentID}) != nil {
		return IssuerRequest{}, ErrCorrupt
	}
	gym, ge := gymauth.GymID(prospect.Context{Version: prospect.Version1, Network: s.meta.Network, Genesis: s.meta.Genesis}, r.PublicKey)
	if ge != nil || gym != r.GymID {
		return IssuerRequest{}, ErrCorrupt
	}
	switch r.State {
	case "issued":
		if !publicHex(r.JobID, 32, true) || r.SealedAtMS != 0 || r.Reason != "" {
			return IssuerRequest{}, ErrCorrupt
		}
		j, err := s.loadJob(r.JobID)
		if err != nil {
			return IssuerRequest{}, err
		}
		gym, err := gymauth.GymID(j.Context, j.PublicKey)
		if err != nil || gym != r.GymID || j.PublicKey != r.PublicKey {
			return IssuerRequest{}, ErrCorrupt
		}
		inv, err := s.db.Get(issueKey("issue/j/", j.ID), nil)
		want, _ := hex.DecodeString(id)
		if err != nil || !bytes.Equal(inv, want) {
			return IssuerRequest{}, ErrCorrupt
		}
	case "sealed":
		if r.JobID != "" || r.SealedAtMS <= 0 || r.SealedAtMS > s.meta.LastClockMS || r.SealedAtMS > MaxPublicTimestampMS || (r.Reason != "parent_changed" && r.Reason != "intake_drained") {
			return IssuerRequest{}, ErrCorrupt
		}
	default:
		return IssuerRequest{}, ErrCorrupt
	}
	return r, nil
}
func (s *ManifestStore) loadIssueRow(id string) (IssuerRequest, error) {
	if !publicHex(id, 32, true) {
		return IssuerRequest{}, ErrManifestAuthority
	}
	raw, err := s.db.Get(issueKey("issue/r/", id), nil)
	if errors.Is(err, leveldb.ErrNotFound) {
		return IssuerRequest{}, ErrIssueRequestNotFound
	}
	if err != nil {
		return IssuerRequest{}, err
	}
	return s.decodeIssueRow(raw, id)
}
func (s *ManifestStore) auditIssueRows(ctx context.Context) error {
	if s.issue == nil || s.public == nil || !s.public.codec.operator || s.meta.LastClockMS > MaxPublicTimestampMS {
		return ErrCorrupt
	}
	var requests, issued, inverses uint64
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		k := it.Key()
		if bytes.HasPrefix(k, []byte("issue/")) && !s.issueKeyAllowed(k) {
			return ErrCorrupt
		}
		switch {
		case len(k) == 40 && bytes.HasPrefix(k, []byte("issue/r/")):
			r, err := s.decodeIssueRow(it.Value(), hex.EncodeToString(k[8:]))
			if err != nil {
				return err
			}
			requests++
			if r.State == "issued" {
				issued++
			}
			if requests > s.issue.profile.MaxRequests {
				return ErrCorrupt
			}
		case len(k) == 40 && bytes.HasPrefix(k, []byte("issue/j/")):
			if len(it.Value()) != 32 {
				return ErrCorrupt
			}
			r, err := s.loadIssueRow(hex.EncodeToString(it.Value()))
			if err != nil || r.State != "issued" || r.JobID != hex.EncodeToString(k[8:]) {
				return ErrCorrupt
			}
			inverses++
		}
	}
	if err := it.Error(); err != nil {
		return err
	}
	if requests != s.issue.requests || issued != s.meta.JobCount || inverses != issued {
		return ErrCorrupt
	}
	return ctx.Err()
}
func (s *ManifestStore) TryIssueRequest(ctx context.Context, id string) (IssuerRequest, error) {
	ctx, cancel, err := issueContext(ctx)
	if err != nil {
		return IssuerRequest{}, err
	}
	defer cancel()
	if s == nil {
		return IssuerRequest{}, ErrClosed
	}
	if !s.mu.TryLock() {
		return IssuerRequest{}, ErrBusy
	}
	defer s.mu.Unlock()
	if err = s.state(false); err != nil {
		return IssuerRequest{}, err
	}
	if s.issue == nil {
		return IssuerRequest{}, ErrManifestAuthority
	}
	if err = ctx.Err(); err != nil {
		return IssuerRequest{}, err
	}
	r, err := s.loadIssueRow(id)
	if err != nil {
		return IssuerRequest{}, err
	}
	if err = ctx.Err(); err != nil {
		return IssuerRequest{}, err
	}
	return r, nil
}

// PreparedIssueRequest cannot be forged, transferred between owners or used to
// choose a challenge or issuance time. Replays resolve the same durable row.
type PreparedIssueRequest struct{ state *preparedIssueRequest }
type preparedIssueRequest struct {
	store     *ManifestStore
	profile   IssueProfile
	intent    IssueIntent
	intentID  string
	context   prospect.Context
	challenge string
	ctx       context.Context
	deadline  time.Time
}

func (s *ManifestStore) matchIssueIntent(i IssueIntent, r IssuerRequest) error {
	id, err := IssueIntentID(i)
	if err != nil {
		return err
	}
	pid, _ := IssueProfileID(s.issue.profile)
	if i.ProfileID != pid {
		return ErrManifestAuthority
	}
	if r.RequestID != i.RequestID || r.ProfileID != pid || r.GymID != i.GymID || r.PublicKey != i.PublicKey || r.IntentID != id {
		return ErrConflict
	}
	return nil
}
func (s *ManifestStore) PrepareIssueRequest(ctx context.Context, i IssueIntent, c prospect.Context) (*PreparedIssueRequest, error) {
	if ctx == nil {
		return nil, ErrManifestAuthority
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	originalContext := ctx
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if s == nil {
		return nil, ErrClosed
	}
	if !s.mu.TryLock() {
		return nil, ErrBusy
	}
	defer s.mu.Unlock()
	if err := s.state(true); err != nil {
		return nil, err
	}
	if s.issue == nil {
		return nil, ErrManifestAuthority
	}
	pid, _ := IssueProfileID(s.issue.profile)
	id, err := IssueIntentID(i)
	if err != nil || i.ProfileID != pid {
		return nil, ErrManifestAuthority
	}
	if err = s.auditIssueLive(ctx); err != nil {
		return nil, err
	}
	genesis, err := chainIdentity(c.Network, c.Genesis)
	gym, ge := gymauth.GymID(c, i.PublicKey)
	if err != nil || ge != nil || gym != i.GymID || c.Network != s.meta.Network || genesis != s.meta.Genesis || c.DNAVersion != s.meta.Descriptor.DNAVersion {
		return nil, ErrManifestAuthority
	}
	validated, err := prospectjob.NewManifest(c, s.meta.AuthorityID, s.meta.DescriptorID, i.PublicKey, stringsZeroChallenge, 1)
	if err != nil {
		return nil, err
	}
	p := &preparedIssueRequest{store: s, profile: s.issue.profile, intent: i, intentID: id, context: validated.Context, ctx: originalContext}
	p.deadline, _ = ctx.Deadline()
	if r, err := s.loadIssueRow(i.RequestID); err == nil {
		if err = s.matchIssueIntent(i, r); err != nil {
			return nil, err
		}
		return &PreparedIssueRequest{p}, nil
	} else if err != ErrIssueRequestNotFound {
		return nil, err
	}
	if s.issue.requests >= s.issue.profile.MaxRequests {
		return nil, ErrPublicCapacity
	}
	random := s.random
	if random == nil {
		random = rand.Reader
	}
	var challenge [32]byte
	if _, err = io.ReadFull(random, challenge[:]); err != nil {
		return nil, errors.New("manifest challenge generation failed")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	p.challenge = hex.EncodeToString(challenge[:])
	return &PreparedIssueRequest{p}, nil
}
func (s *ManifestStore) resultIssueRow(r IssuerRequest) (IssuerRequest, prospectjob.ManifestJob, error) {
	if r.State == "sealed" {
		return r, prospectjob.ManifestJob{}, nil
	}
	j, err := s.loadJob(r.JobID)
	return r, j, err
}
func (s *ManifestStore) CommitIssueRequest(prepared *PreparedIssueRequest, check func() error) (IssuerRequest, prospectjob.ManifestJob, error) {
	var z IssuerRequest
	var j prospectjob.ManifestJob
	if s == nil {
		return z, j, ErrClosed
	}
	if !s.mu.TryLock() {
		return z, j, ErrBusy
	}
	defer s.mu.Unlock()
	if err := s.state(true); err != nil {
		return z, j, err
	}
	if s.issue == nil || prepared == nil || prepared.state == nil || prepared.state.store != s || prepared.state.profile != s.issue.profile || check == nil {
		return z, j, ErrManifestAuthority
	}
	p := prepared.state
	if err := p.ctx.Err(); err != nil {
		return z, j, err
	}
	if !time.Now().Before(p.deadline) {
		return z, j, context.DeadlineExceeded
	}
	if err := check(); err != nil {
		return z, j, err
	}
	if r, err := s.loadIssueRow(p.intent.RequestID); err == nil {
		if err = s.matchIssueIntent(p.intent, r); err != nil {
			return z, j, err
		}
		return s.resultIssueRow(r)
	} else if err != ErrIssueRequestNotFound {
		return z, j, err
	}
	if p.challenge == "" {
		return z, j, ErrCorrupt
	}
	if s.issue.requests >= s.issue.profile.MaxRequests {
		return z, j, ErrPublicCapacity
	}
	issued, err := s.clockMillis()
	if err != nil {
		return z, j, err
	}
	if issued > MaxPublicTimestampMS-prospectjob.LifetimeMS {
		return z, j, ErrClock
	}
	j, err = prospectjob.NewManifest(p.context, s.meta.AuthorityID, s.meta.DescriptorID, p.intent.PublicKey, p.challenge, issued)
	if err != nil {
		return z, prospectjob.ManifestJob{}, err
	}
	if exists, err := s.db.Has(jobKey(j.ID), nil); err != nil {
		return z, prospectjob.ManifestJob{}, err
	} else if exists {
		return z, prospectjob.ManifestJob{}, ErrConflict
	}
	r := IssuerRequest{IssueVersion1, p.intent.ProfileID, p.intent.RequestID, p.intentID, p.intent.GymID, p.intent.PublicKey, "issued", j.ID, 0, ""}
	raw, _ := json.Marshal(r)
	jr, err := prospectjob.MarshalManifestJob(j)
	if err != nil || len(raw) > MaxIssueRowSize {
		return z, prospectjob.ManifestJob{}, ErrCorrupt
	}
	next := s.meta
	next.JobCount++
	next.LastClockMS = issued
	b := new(leveldb.Batch)
	b.Put(issueKey("issue/r/", r.RequestID), raw)
	b.Put(jobKey(j.ID), jr)
	id, _ := hex.DecodeString(r.RequestID)
	b.Put(issueKey("issue/j/", j.ID), id)
	s.mark("before-issue")
	if err = p.ctx.Err(); err != nil {
		return z, prospectjob.ManifestJob{}, err
	}
	if !time.Now().Before(p.deadline) {
		return z, prospectjob.ManifestJob{}, context.DeadlineExceeded
	}
	if err = check(); err != nil {
		return z, prospectjob.ManifestJob{}, err
	}
	if err = s.commitIssue(b, next, s.issue.requests+1); err != nil {
		return z, prospectjob.ManifestJob{}, err
	}
	s.mark("issued")
	return r, j, nil
}

// SealIssueRequest atomically prevents a future job for the exact absent
// request. The original source supplies the already verified terminal cause;
// an existing job or seal always wins without changing its time or reason.
func (s *ManifestStore) SealIssueRequest(ctx context.Context, i IssueIntent, reason string, check func() error) (IssuerRequest, prospectjob.ManifestJob, error) {
	var z IssuerRequest
	var j prospectjob.ManifestJob
	if ctx == nil {
		return z, j, ErrManifestAuthority
	}
	if err := ctx.Err(); err != nil {
		return z, j, err
	}
	if s == nil {
		return z, j, ErrClosed
	}
	if !s.mu.TryLock() {
		return z, j, ErrBusy
	}
	defer s.mu.Unlock()
	if err := s.state(true); err != nil {
		return z, j, err
	}
	if s.issue == nil || check == nil || (reason != "parent_changed" && reason != "intake_drained") {
		return z, j, ErrManifestAuthority
	}
	id, err := IssueIntentID(i)
	pid, _ := IssueProfileID(s.issue.profile)
	if err != nil || i.ProfileID != pid {
		return z, j, ErrManifestAuthority
	}
	gym, ge := gymauth.GymID(prospect.Context{Version: prospect.Version1, Network: s.meta.Network, Genesis: s.meta.Genesis}, i.PublicKey)
	if ge != nil || gym != i.GymID {
		return z, j, ErrManifestAuthority
	}
	if err = check(); err != nil {
		return z, j, err
	}
	if r, err := s.loadIssueRow(i.RequestID); err == nil {
		if err = s.matchIssueIntent(i, r); err != nil {
			return z, j, err
		}
		return s.resultIssueRow(r)
	} else if err != ErrIssueRequestNotFound {
		return z, j, err
	}
	if s.issue.requests >= s.issue.profile.MaxRequests {
		return z, j, ErrPublicCapacity
	}
	now, err := s.clockMillis()
	if err != nil {
		return z, j, err
	}
	if now > MaxPublicTimestampMS {
		return z, j, ErrClock
	}
	r := IssuerRequest{IssueVersion1, i.ProfileID, i.RequestID, id, i.GymID, i.PublicKey, "sealed", "", now, reason}
	raw, _ := json.Marshal(r)
	if len(raw) > MaxIssueRowSize {
		return z, j, ErrCorrupt
	}
	b := new(leveldb.Batch)
	b.Put(issueKey("issue/r/", r.RequestID), raw)
	next := s.meta
	next.LastClockMS = now
	if err = ctx.Err(); err != nil {
		return z, j, err
	}
	if err = check(); err != nil {
		return z, j, err
	}
	if err = s.commitIssue(b, next, s.issue.requests+1); err != nil {
		return z, j, err
	}
	s.mark("issue-sealed")
	return r, j, nil
}
