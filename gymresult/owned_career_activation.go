// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/internal/careerstate"
)

const ownedCareerTempName = ".SLUG-OWNED-RESULT.career-marker.tmp"

type ownedCareerIntent struct {
	Version           uint32 `json:"version"`
	PolicyID          string `json:"policy_id"`
	ResultStoreID     string `json:"result_store_id"`
	ResultCreatedAtMS int64  `json:"result_created_at_ms"`
	SourceCreatedAtMS int64  `json:"source_created_at_ms"`
	OldMarkerSHA256   string `json:"old_marker_sha256"`
}
type ownedCareerMarker struct {
	Format             string            `json:"format"`
	Version            uint32            `json:"version"`
	StoreID            string            `json:"store_id"`
	Pin                OwnedPin          `json:"pin"`
	CareerPhase        string            `json:"career_phase"`
	CareerIntentDigest string            `json:"career_intent_digest"`
	Career             *CareerActivation `json:"career"`
}

func (m ownedCareerMarker) base() ownedMarker {
	return ownedMarker{m.Format, m.Version, m.StoreID, m.Pin}
}
func ownedIntentFor(m ownedMetadata) ownedCareerIntent {
	return ownedCareerIntent{1, CareerPolicyID(), m.StoreID, m.CreatedAtMS, m.SourceCreatedAtMS, plainSHA(markerBytes(ownedMarker{"slug-owned-result", 1, m.StoreID, m.Pin}))}
}
func (s *OwnedStore) careerIntent() (ownedCareerIntent, bool, error) {
	var in ownedCareerIntent
	b, ok, e := s.raw([]byte("career_init"), 2048)
	if e != nil || !ok {
		return in, ok, e
	}
	if ownedDecode(b, 2048, &in, true) != nil || in != ownedIntentFor(s.meta) {
		return in, true, ErrCorrupt
	}
	return in, true, nil
}
func ownedHasFinality(s *OwnedStore) (bool, error) {
	for _, i := range s.index {
		r, ok, e := s.record(i.ResultKey)
		if e != nil || !ok {
			return false, ErrCorrupt
		}
		if r.Finality != nil {
			return true, nil
		}
	}
	return false, nil
}

// CheckOwnedCareerActivation audits while holding the real DB lock but never writes
// career state. Activation rechecks after the parent publishes its own intent.
func CheckOwnedCareerActivation(ctx context.Context, path string, p OwnedPin, bouts *gymbout.OwnedStore) error {
	return ownedCheckCareerActivation(ctx, path, p, bouts, time.Now)
}
func ownedCheckCareerActivation(ctx context.Context, path string, p OwnedPin, bouts *gymbout.OwnedStore, clock func() time.Time) (result error) {
	s, e := ownedOpen(ctx, path, p, bouts, clock, false)
	if e != nil {
		return e
	}
	defer func() {
		if e := s.Close(); e != nil {
			result = &OwnedCleanupError{Owner: s, Cause: errors.Join(result, e)}
		}
	}()
	has, e := ownedHasFinality(s)
	if e != nil {
		return e
	}
	if has {
		return ErrCareerLegacyFinality
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	return nil
}
func ActivateOwnedCareer(ctx context.Context, path string, p OwnedPin, bouts *gymbout.OwnedStore) (*OwnedStore, error) {
	return ownedOpenCareer(ctx, path, p, bouts, time.Now, "activate", nil)
}
func ResumeOwnedCareerActivation(ctx context.Context, path string, p OwnedPin, bouts *gymbout.OwnedStore) (*OwnedStore, error) {
	return ownedOpenCareer(ctx, path, p, bouts, time.Now, "resume", nil)
}
func OpenOwnedCareer(ctx context.Context, path string, p OwnedPin, bouts *gymbout.OwnedStore) (*OwnedStore, error) {
	return ownedOpenCareer(ctx, path, p, bouts, time.Now, "open", nil)
}
func ownedCareerFiles(path string, p OwnedPin) (ownedMarker, ownedCareerMarker, []byte, bool, error) {
	var m ownedMarker
	var cm ownedCareerMarker
	if !protectedPath(path) || !realDirectory(path) {
		return m, cm, nil, false, ErrCorrupt
	}
	f, e := os.Open(path)
	if e != nil {
		return m, cm, nil, false, ErrCorrupt
	}
	items, e := f.ReadDir(4)
	f.Close()
	if e != nil && e != io.EOF || len(items) > 3 {
		return m, cm, nil, false, ErrCorrupt
	}
	mark, db, tmp := false, false, false
	for _, v := range items {
		switch v.Name() {
		case ownedMarkerName:
			mark = true
		case "db":
			db = realDirectory(filepath.Join(path, "db"))
		case ownedCareerTempName:
			tmp = true
		default:
			return m, cm, nil, false, ErrCorrupt
		}
	}
	if !mark || !db {
		return m, cm, nil, false, ErrCorrupt
	}
	b, e := careerFile(filepath.Join(path, ownedMarkerName), OwnedMaxPinSize)
	if e != nil || len(b) < 2 || b[len(b)-1] != '\n' {
		return m, cm, nil, false, ErrCorrupt
	}
	var header struct {
		Version uint32 `json:"version"`
	}
	if json.Unmarshal(b, &header) != nil {
		return m, cm, nil, false, ErrCorrupt
	}
	id, _ := OwnedID(p)
	if header.Version == 1 {
		if ownedDecode(b[:len(b)-1], OwnedMaxPinSize, &m, true) != nil || !equal(m, ownedMarker{"slug-owned-result", 1, id, p}) {
			return m, cm, nil, false, ErrCorrupt
		}
	} else if header.Version == 2 {
		if ownedDecode(b[:len(b)-1], OwnedMaxPinSize, &cm, true) != nil || !equal(cm.base(), ownedMarker{"slug-owned-result", 2, id, p}) {
			return m, cm, nil, false, ErrCorrupt
		}
		m = cm.base()
	} else {
		return m, cm, nil, false, ErrCorrupt
	}
	return m, cm, b, tmp, nil
}
func (s *OwnedStore) replaceCareerMarker(ctx context.Context, path string, old, next []byte) error {
	if ctx.Err() != nil {
		return ErrOutcomeUnknown
	}
	f, e := os.OpenFile(filepath.Join(path, ownedCareerTempName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return ErrOutcomeUnknown
	}
	s.mark("career-marker-temp-created")
	_, e = f.Write(next)
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return ErrOutcomeUnknown
	}
	s.mark("career-marker-temp-synced")
	if ctx.Err() != nil {
		return ErrOutcomeUnknown
	}
	actual, e := careerFile(filepath.Join(path, ownedMarkerName), OwnedMaxPinSize)
	if e != nil || !bytes.Equal(actual, old) {
		return ErrOutcomeUnknown
	}
	if e = os.Rename(filepath.Join(path, ownedCareerTempName), filepath.Join(path, ownedMarkerName)); e != nil {
		return ErrOutcomeUnknown
	}
	s.mark("career-marker-renamed")
	if e = syncDir(path); e != nil {
		return ErrOutcomeUnknown
	}
	s.mark("career-marker-dirsynced")
	return nil
}
func ownedOpenCareer(ctx context.Context, path string, expected OwnedPin, bouts *gymbout.OwnedStore, clock func() time.Time, mode string, checkpoint func(string)) (*OwnedStore, error) {
	return ownedOpenCareerAudit(ctx, path, expected, bouts, clock, mode, checkpoint, false)
}

// OpenOwnedCareerAudit requires the original completed career activation and never
// resumes activation, repairs markers or changes journal bytes.
func OpenOwnedCareerAudit(ctx context.Context, path string, p OwnedPin, bouts *gymbout.OwnedStore) (*OwnedStore, error) {
	return ownedOpenCareerAudit(ctx, path, p, bouts, time.Now, "open", nil, true)
}
func ownedOpenCareerAudit(ctx context.Context, path string, expected OwnedPin, bouts *gymbout.OwnedStore, clock func() time.Time, mode string, checkpoint func(string), readOnly bool) (*OwnedStore, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if ctx == nil || clock == nil {
		return nil, ErrInvalid
	}
	p, e := ValidateOwnedPin(expected)
	if e != nil {
		return nil, e
	}
	source, e := ownedCheckSource(ctx, bouts, p)
	if e != nil {
		return nil, e
	}
	m, cm, current, tmp, e := ownedCareerFiles(path, p)
	if e != nil {
		return nil, e
	}
	if mode == "open" && (m.Version != 2 || cm.CareerPhase != "ready" || tmp) {
		return nil, ErrCareerInactive
	}
	if mode == "activate" && (m.Version != 1 || tmp) {
		return nil, ErrCorrupt
	}
	if ctx.Err() != nil {
		return nil, ErrCancelled
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{Strict: opt.StrictAll, ErrorIfMissing: true, ReadOnly: readOnly})
	if e != nil {
		return nil, ErrUnavailable
	}
	s := ownedNewStore(db, clock)
	s.sourceCreatedAtMS = source.CreatedAtMS
	s.readOnly = readOnly
	s.careerOpening = true
	s.checkpoint = checkpoint
	fail := func(e error) (*OwnedStore, error) { return ownedFailed(s, e) }
	if e = s.audit(ctx, m); e != nil {
		return fail(e)
	}
	in, present, e := s.careerIntent()
	if e != nil {
		return fail(e)
	}
	if mode == "activate" && present {
		return fail(ErrCorrupt)
	}
	if m.Version == 1 && s.meta.Version != 1 || m.Version == 2 && !present {
		return fail(ErrCorrupt)
	}
	if m.Version == 2 {
		if cm.CareerIntentDigest != plainSHA(encoded(in)) {
			return fail(ErrCorrupt)
		}
		switch cm.CareerPhase {
		case "prepared":
			if cm.Career != nil {
				return fail(ErrCorrupt)
			}
		case "ready":
			if s.meta.Career == nil || cm.Career == nil || *cm.Career != *s.meta.Career {
				return fail(ErrCorrupt)
			}
		default:
			return fail(ErrCorrupt)
		}
	}
	if s.meta.Career == nil {
		has, e := ownedHasFinality(s)
		if e != nil {
			return fail(e)
		}
		if has {
			return fail(ErrCareerLegacyFinality)
		}
	}
	changed := present || m.Version == 2
	// Only exact generated ownedMarker prefixes are recoverable; unrelated files remain.
	if tmp {
		if !present || mode != "resume" {
			return fail(ErrCorrupt)
		}
		next := ownedCareerMarker{"slug-owned-result", 2, m.StoreID, p, "prepared", plainSHA(encoded(in)), nil}
		if s.meta.Career != nil {
			next.CareerPhase = "ready"
			next.Career = s.meta.Career
		}
		b, e := careerFile(filepath.Join(path, ownedCareerTempName), OwnedMaxPinSize)
		if e != nil || !bytes.HasPrefix(markerBytes(next), b) && !bytes.Equal(current, b) {
			return fail(ErrCorrupt)
		}
		if os.Remove(filepath.Join(path, ownedCareerTempName)) != nil || syncDir(path) != nil {
			return fail(ErrOutcomeUnknown)
		}
	}
	if m.Version == 2 && cm.CareerPhase == "ready" {
		if ctx.Err() != nil {
			if tmp {
				return fail(ErrOutcomeUnknown)
			}
			return fail(ErrCancelled)
		}
		s.careerOpening = false
		return s, nil
	}
	if mode == "open" {
		return fail(ErrCareerInactive)
	}
	if ctx.Err() != nil {
		if changed {
			return fail(ErrOutcomeUnknown)
		}
		return fail(ErrCancelled)
	}
	if !present {
		in = ownedIntentFor(s.meta)
		batch := new(leveldb.Batch)
		if e = put(batch, []byte("career_init"), in, 2048); e != nil {
			return fail(e)
		}
		if e = s.writeBatch(batch, &opt.WriteOptions{Sync: true}); e != nil {
			return fail(ErrOutcomeUnknown)
		}
		changed = true
		s.mark("career-intent-synced")
	}
	if m.Version == 1 {
		next := ownedCareerMarker{"slug-owned-result", 2, m.StoreID, p, "prepared", plainSHA(encoded(in)), nil}
		nextBytes := markerBytes(next)
		if e = s.replaceCareerMarker(ctx, path, current, nextBytes); e != nil {
			return fail(e)
		}
		current = nextBytes
		cm = next
		m = next.base()
		s.mark("career-prepared")
	}
	if s.meta.Career == nil {
		if ctx.Err() != nil {
			return fail(ErrOutcomeUnknown)
		}
		n, e := s.sampleLocked(s.meta.ClockMS)
		if e != nil {
			return fail(ErrOutcomeUnknown)
		}
		a := CareerActivation{Version: 1, PolicyID: CareerPolicyID(), ResultStoreID: s.meta.StoreID, ResultCreatedAtMS: s.meta.CreatedAtMS, SourceCreatedAtMS: s.meta.SourceCreatedAtMS, ActivatedAtMS: n}
		a.ActivationID = careerstate.ActivationID(a)
		if careerstate.ValidateActivation(a) != nil {
			return fail(ErrOutcomeUnknown)
		}
		next := s.meta
		next.Version = 2
		next.ClockMS = n
		next.Career = &a
		if e = s.commitLocked(new(leveldb.Batch), next); e != nil {
			return fail(ErrOutcomeUnknown)
		}
		s.careerIndex = []careerstate.Event{}
		s.mark("career-metadata-synced")
	}
	cm.CareerPhase = "ready"
	cm.Career = s.meta.Career
	if e = s.replaceCareerMarker(ctx, path, current, markerBytes(cm)); e != nil {
		return fail(e)
	}
	s.mark("career-ready")
	if ctx.Err() != nil {
		return fail(ErrOutcomeUnknown)
	}
	s.careerOpening = false
	return s, nil
}

// Closed v1 ownedMetadata has no career field. Do not relax the generic public codec
// to implement a private versioned storage schema.
type ownedLegacyCareerMetadata struct {
	Version           uint32        `json:"version"`
	StoreID           string        `json:"store_id"`
	Pin               OwnedPin      `json:"pin"`
	SourceCreatedAtMS int64         `json:"source_created_at_ms"`
	CreatedAtMS       int64         `json:"created_at_ms"`
	ClockMS           int64         `json:"clock_ms"`
	ResultCount       uint32        `json:"result_count"`
	FundingCursor     Cursor        `json:"funding_cursor"`
	FundingRestarts   []ScanRestart `json:"funding_restarts"`
	FundingComplete   bool          `json:"funding_complete"`
	LastFundingTip    *Tip          `json:"last_funding_tip"`
}

func ownedDecodeCareerMetadata(raw []byte, limit int, out *ownedMetadata, canonical bool) error {
	if len(raw) == 0 || len(raw) > limit {
		return ErrInvalid
	}
	var header struct {
		Version uint32 `json:"version"`
	}
	if json.Unmarshal(raw, &header) != nil {
		return ErrInvalid
	}
	if header.Version == 1 {
		var old ownedLegacyCareerMetadata
		if e := ownedDecode(raw, limit, &old, canonical); e != nil {
			return e
		}
		if json.Unmarshal(raw, out) != nil {
			return ErrInvalid
		}
		out.Career = nil
		return nil
	}
	if header.Version != 2 {
		return ErrInvalid
	}
	type v2 ownedMetadata
	var m v2
	if e := ownedDecode(raw, limit, &m, canonical); e != nil {
		return e
	}
	if m.Career == nil {
		return ErrInvalid
	}
	*out = ownedMetadata(m)
	return nil
}
