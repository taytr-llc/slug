// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymprovider

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
)

const markerName = "SLUG-PROVIDER.json"
const application = "slug-fixture-provider-coordinator"
const fixtureModulus = "ofgWCuLjybRlzo0tZWJjNiuSfb4p4fAkd_wWJcyQoTbji9k0l8W26mPddxHmfHQp-Vaw-4qPCJrcS2mJPMEzP1Pt0Bm4d4QlL-yRT-SFd2lZS-pCgNMsD1W_YpRPEwOWvG6b32690r2jZ47soMZo9wGzjb_7OMg0LOL-bSf63kpaSHSXndS5z5rexMdbBYUsLA9e-KXBdQOS-UTo7WTBEMa2R2CapHg665xsmtdVMTBQY4uDZlxvb3qCo5ZwKh9kG4LT6_I5IhlJH7aGhyxXFvUK-DWNmoudF8NAco9_h9iaGNj8q2ethFkMLs91kzk2PAcDTW9gb54h4FRWyuXpoQ"

// FixtureVerifierConfig returns an owned public-only fixture trust profile.
// It does not configure or claim compatibility with any real provider.
func FixtureVerifierConfig() gymidentity.Config {
	return gymidentity.Config{Version: 1, Issuer: FixtureIssuer, Audience: "slug-provider-fixture-login", ResetAudience: "slug-provider-fixture-reset", EpochClaim: "slug_auth_epoch", PublicJWKS: []byte(`{"keys":[{"kid":"rfc7515-a2","kty":"RSA","n":"` + fixtureModulus + `","e":"AQAB","alg":"RS256","use":"sig"}]}`)}
}

type marker struct {
	Application   string                   `json:"application"`
	Version       uint32                   `json:"version"`
	CoordinatorID string                   `json:"coordinator_id"`
	Config        Config                   `json:"config"`
	Authority     gymaccount.AuthorityInfo `json:"authority"`
}
type metadata struct {
	Marker     marker `json:"marker"`
	ClockMS    int64  `json:"clock_ms"`
	LoginFlows uint32 `json:"login_flows"`
	ResetFlows uint32 `json:"reset_flows"`
}
type loginSemantic struct {
	Issuer            string `json:"issuer"`
	Subject           string `json:"subject"`
	OriginalAuthID    string `json:"original_auth_id"`
	OriginalFlowID    string `json:"original_flow_id"`
	Mode              string `json:"mode"`
	Epoch             uint64 `json:"epoch"`
	AuthenticatedAtMS int64  `json:"authenticated_at_ms"`
}
type loginRecord struct {
	Flow       LoginFlow      `json:"flow"`
	Phase      string         `json:"phase"`
	OriginKey  string         `json:"origin_key"`
	Semantic   *loginSemantic `json:"semantic"`
	AccountID  string         `json:"account_id"`
	Generation uint64         `json:"generation"`
}
type originRecord struct {
	Semantic  loginSemantic `json:"semantic"`
	AttemptID string        `json:"attempt_id"`
	Sequence  uint64        `json:"sequence"`
	State     string        `json:"state"`
}
type resetSemantic struct {
	Issuer              string `json:"issuer"`
	Subject             string `json:"subject"`
	ProviderOperationID string `json:"provider_operation_id"`
	FromEpoch           uint64 `json:"from_epoch"`
	ToEpoch             uint64 `json:"to_epoch"`
	VerifiedAtMS        int64  `json:"verified_at_ms"`
}
type resetRecord struct {
	Request      ResetRequest             `json:"request"`
	Phase        string                   `json:"phase"`
	OperationKey string                   `json:"operation_key"`
	Semantic     *resetSemantic           `json:"semantic"`
	Barrier      *gymaccount.ResetBarrier `json:"barrier"`
}

// Coordinator exclusively owns its journal and borrows the authority/verifier.
// Never reenter it from an authority callback. No JWT or session token is stored.
type Coordinator struct {
	gate             chan struct{}
	db               *leveldb.DB
	authority        *gymaccount.Store
	verifier         *gymidentity.Verifier
	meta             metadata
	closing          atomic.Bool
	closed, poisoned bool
	closeErr         error
	now              func() time.Time
	random           io.Reader
	writeBatch       func(*leveldb.Batch, *opt.WriteOptions) error
	checkpoint       func(string)
}

func validID(v string) bool {
	if len(v) != 64 || strings.ToLower(v) != v {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func validName(v string) bool {
	if len(v) < 1 || len(v) > 128 {
		return false
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func validConfig(ctx context.Context, c Config, a *gymaccount.Store, v *gymidentity.Verifier) (gymaccount.AuthorityInfo, error) {
	if ctx == nil {
		return gymaccount.AuthorityInfo{}, ErrConfig
	}
	if err := ctx.Err(); err != nil {
		return gymaccount.AuthorityInfo{}, err
	}
	if a == nil || v == nil || c.Version != 1 || c.Provider != ProviderFixture || c.Issuer != FixtureIssuer || !validName(c.TenantID) || !validName(c.ClientID) || !validName(c.ConnectionID) || !validID(c.AuthorityID) || !validID(c.VerifierConfigHash) || c.MaxLoginFlows < 1 || c.MaxLoginFlows > 4096 || c.MaxResetFlows < 1 || c.MaxResetFlows > 256 {
		return gymaccount.AuthorityInfo{}, ErrConfig
	}
	expected, err := gymidentity.NewVerifier(FixtureVerifierConfig())
	if err != nil || v.ConfigDigest() != expected.ConfigDigest() || c.VerifierConfigHash != expected.ConfigDigest() {
		return gymaccount.AuthorityInfo{}, ErrConfig
	}
	info, err := a.AuthorityInfo(ctx)
	if err != nil {
		return gymaccount.AuthorityInfo{}, safeError(err)
	}
	if info.Version != 2 || info.AuthorityID != c.AuthorityID || info.VerifierConfigHash != c.VerifierConfigHash || info.Network != gymaccount.FixtureNetwork || info.Genesis != gymaccount.FixtureGenesis || info.Profile != gymaccount.FixtureProfile {
		return gymaccount.AuthorityInfo{}, ErrConfig
	}
	return info, nil
}
func regular(path string, dir bool) error {
	st, e := os.Lstat(path)
	if e != nil || st.Mode()&os.ModeSymlink != 0 || dir && !st.IsDir() || !dir && !st.Mode().IsRegular() {
		return ErrCorrupt
	}
	return nil
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	return errors.Join(f.Sync(), f.Close())
}
func canonical(raw []byte, out any) error {
	if len(raw) == 0 || len(raw) > MaxRecordSize || json.Unmarshal(raw, out) != nil {
		return ErrCorrupt
	}
	enc, e := json.Marshal(out)
	if e != nil || !bytes.Equal(raw, enc) {
		return ErrCorrupt
	}
	return nil
}
func put(b *leveldb.Batch, key string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil || len(raw) > MaxRecordSize {
		return ErrCorrupt
	}
	b.Put([]byte(key), raw)
	return nil
}
func readMarker(path string) (marker, error) {
	var m marker
	if regular(path, false) != nil {
		return m, ErrCorrupt
	}
	f, e := os.Open(path)
	if e != nil {
		return m, ErrCorrupt
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() {
		return m, ErrCorrupt
	}
	raw, e := io.ReadAll(io.LimitReader(f, MaxRecordSize+1))
	if e != nil || canonical(raw, &m) != nil {
		return marker{}, ErrCorrupt
	}
	return m, nil
}
func newHandle(db *leveldb.DB, a *gymaccount.Store, v *gymidentity.Verifier, m metadata) *Coordinator {
	return &Coordinator{gate: make(chan struct{}, 1), db: db, authority: a, verifier: v, meta: m, now: time.Now, random: rand.Reader, writeBatch: db.Write}
}
func Create(ctx context.Context, path string, cfg Config, a *gymaccount.Store, v *gymidentity.Verifier) (*Coordinator, error) {
	info, e := validConfig(ctx, cfg, a, v)
	if e != nil {
		return nil, e
	}
	id, e := randomID(rand.Reader)
	if e != nil {
		return nil, e
	}
	m := marker{application, 1, id, cfg, info}
	raw, e := json.Marshal(m)
	if e != nil || len(raw) > MaxRecordSize {
		return nil, ErrConfig
	}
	if e = os.Mkdir(path, 0700); e != nil {
		return nil, ErrUnavailable
	}
	f, e := os.OpenFile(filepath.Join(path, markerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, ErrUnavailable
	}
	_, we := f.Write(raw)
	if e = errors.Join(we, f.Sync(), f.Close()); e != nil {
		return nil, ErrOutcomeUnknown
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfExist: true})
	if e != nil {
		return nil, ErrUnavailable
	}
	s := newHandle(db, a, v, metadata{Marker: m})
	b := new(leveldb.Batch)
	e = put(b, "meta", s.meta)
	if e == nil {
		e = db.Write(b, &opt.WriteOptions{Sync: true})
	}
	if e == nil {
		e = syncDir(path)
	}
	if e == nil {
		e = syncDir(filepath.Dir(path))
	}
	if e != nil {
		db.Close()
		return nil, ErrOutcomeUnknown
	}
	if e = ctx.Err(); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func Open(ctx context.Context, path string, cfg Config, a *gymaccount.Store, v *gymidentity.Verifier) (*Coordinator, error) {
	info, e := validConfig(ctx, cfg, a, v)
	if e != nil {
		return nil, e
	}
	if regular(path, true) != nil || regular(filepath.Join(path, "db"), true) != nil {
		return nil, ErrCorrupt
	}
	m, e := readMarker(filepath.Join(path, markerName))
	if e != nil || m.Application != application || m.Version != 1 || !validID(m.CoordinatorID) || m.Config != cfg || m.Authority != info {
		return nil, ErrCorrupt
	}
	entries, e := os.ReadDir(filepath.Join(path, "db"))
	if e != nil || len(entries) > 4096 {
		return nil, ErrCorrupt
	}
	for _, entry := range entries {
		if entry.IsDir() || regular(filepath.Join(path, "db", entry.Name()), false) != nil {
			return nil, ErrCorrupt
		}
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true})
	if e != nil {
		return nil, ErrUnavailable
	}
	s := newHandle(db, a, v, metadata{})
	if e = s.load("meta", &s.meta); e != nil || s.meta.Marker != m || s.audit() != nil {
		db.Close()
		return nil, ErrCorrupt
	}
	if e = ctx.Err(); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Coordinator) lock(ctx context.Context) error {
	if ctx == nil {
		return ErrUnavailable
	}
	if s.closing.Load() {
		return ErrClosed
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.gate <- struct{}{}:
	}
	if e := ctx.Err(); e != nil {
		<-s.gate
		return e
	}
	if s.closed || s.closing.Load() {
		<-s.gate
		return ErrClosed
	}
	if s.poisoned {
		<-s.gate
		return ErrOutcomeUnknown
	}
	return nil
}
func (s *Coordinator) unlock() { <-s.gate }
func (s *Coordinator) Close() error {
	s.closing.Store(true)
	s.gate <- struct{}{}
	defer s.unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	e := s.db.Close()
	if e != nil || s.poisoned {
		s.closeErr = ErrOutcomeUnknown
	}
	return s.closeErr
}
func (s *Coordinator) load(key string, out any) error {
	raw, e := s.db.Get([]byte(key), nil)
	if errors.Is(e, leveldb.ErrNotFound) {
		return ErrNotFound
	}
	if e != nil {
		return ErrCorrupt
	}
	return canonical(raw, out)
}
func (s *Coordinator) commit(ctx context.Context, b *leveldb.Batch, next metadata) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if put(b, "meta", next) != nil {
		return ErrCorrupt
	}
	if s.writeBatch(b, &opt.WriteOptions{Sync: true}) != nil {
		s.poisoned = true
		return ErrOutcomeUnknown
	}
	s.meta = next
	return ctx.Err()
}
func (s *Coordinator) observe(ctx context.Context) (int64, error) {
	now := s.now().UnixMilli()
	if now <= 0 || now > math.MaxInt64-LifetimeMS || now < s.meta.ClockMS {
		return 0, ErrUnavailable
	}
	if now > s.meta.ClockMS {
		next := s.meta
		next.ClockMS = now
		if e := s.commit(ctx, new(leveldb.Batch), next); e != nil {
			return 0, e
		}
	}
	return now, nil
}
func randomID(r io.Reader) (string, error) {
	var b [32]byte
	if _, e := io.ReadFull(r, b[:]); e != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(b[:]), nil
}
func (s *Coordinator) unique(prefix string) (string, error) {
	id, e := randomID(s.random)
	if e != nil {
		return "", e
	}
	yes, e := s.db.Has([]byte(prefix+id), nil)
	if e != nil {
		return "", ErrCorrupt
	}
	if yes {
		return "", ErrConflict
	}
	return id, nil
}
func (s *Coordinator) point(name string) {
	if s.checkpoint != nil {
		s.checkpoint(name)
	}
}
func (s *Coordinator) binding() gymidentity.CoordinatorBinding {
	c := s.meta.Marker.Config
	return gymidentity.CoordinatorBinding{Version: 1, CoordinatorID: s.meta.Marker.CoordinatorID, AuthorityID: c.AuthorityID, TenantID: c.TenantID, ClientID: c.ClientID, ConnectionID: c.ConnectionID}
}
func tupleIndex(domain string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte("SLUG provider " + domain + "\x00v1\x00"))
	var n [4]byte
	for _, p := range parts {
		binary.LittleEndian.PutUint32(n[:], uint32(len(p)))
		h.Write(n[:])
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Coordinator) index(domain, issuer, subject, id string) string {
	c := s.meta.Marker.Config
	return tupleIndex(domain, c.VerifierConfigHash, issuer, subject, c.TenantID, c.ClientID, c.ConnectionID, id)
}
func safeError(e error) error {
	switch {
	case e == nil:
		return nil
	case errors.Is(e, context.Canceled):
		return context.Canceled
	case errors.Is(e, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(e, gymaccount.ErrExpired):
		return ErrExpired
	case errors.Is(e, gymaccount.ErrCapacity):
		return ErrCapacity
	case errors.Is(e, gymaccount.ErrConflict):
		return ErrConflict
	case errors.Is(e, gymaccount.ErrPoisoned):
		return ErrOutcomeUnknown
	case errors.Is(e, gymaccount.ErrCorrupt):
		return ErrCorrupt
	case errors.Is(e, gymaccount.ErrUnauthorized), errors.Is(e, gymaccount.ErrForbidden):
		return ErrEvidence
	default:
		return ErrUnavailable
	}
}
