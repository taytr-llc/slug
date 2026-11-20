// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

const markerName = "SLUG-ACCOUNTS.json"
const application = "slug-fixture-gym-accounts"

type metadata struct {
	Application      string          `json:"application"`
	Version          uint32          `json:"version"`
	Network          uint32          `json:"network"`
	Genesis          string          `json:"genesis"`
	Profile          string          `json:"profile"`
	HoldMS           int64           `json:"hold_ms"`
	AuthorityID      string          `json:"authority_id"`
	Config           Config          `json:"config"`
	ClockMS          int64           `json:"clock_ms"`
	Accounts         uint32          `json:"accounts"`
	Sessions         uint32          `json:"sessions"`
	Challenges       uint32          `json:"challenges"`
	Events           uint32          `json:"events"`
	Identity         *IdentityConfig `json:"identity,omitempty"`
	LoginAttempts    uint32          `json:"login_attempts,omitempty"`
	IdentitySequence uint64          `json:"identity_sequence,omitempty"`
	ReservedEvents   uint32          `json:"reserved_events,omitempty"`
}
type marker struct {
	Application string          `json:"application"`
	Version     uint32          `json:"version"`
	Network     uint32          `json:"network"`
	Genesis     string          `json:"genesis"`
	Profile     string          `json:"profile"`
	HoldMS      int64           `json:"hold_ms"`
	AuthorityID string          `json:"authority_id"`
	Config      Config          `json:"config"`
	Identity    *IdentityConfig `json:"identity,omitempty"`
}
type sessionRecord struct {
	AccountID         string `json:"account_id"`
	Generation        uint64 `json:"generation"`
	Kind              string `json:"kind"`
	Parent            string `json:"parent"`
	AuthenticatedAtMS int64  `json:"authenticated_at_ms"`
	IssuedAtMS        int64  `json:"issued_at_ms"`
	ExpiresAtMS       int64  `json:"expires_at_ms"`
	Revoked           bool   `json:"revoked"`
}
type challengeRecord struct {
	Challenge     EnrollmentChallenge `json:"challenge"`
	Authorization *Authorization      `json:"authorization,omitempty"`
	Result        *EnrollmentResult   `json:"result,omitempty"`
}

// Store owns an exclusively opened fixture database. Protected callbacks run
// under its context-aware serialization gate; never reenter Store from one.
type Store struct {
	gate        chan struct{}
	authorityID string
	db          *leveldb.DB
	meta        metadata
	closed      bool
	poisoned    bool
	now         func() time.Time
	random      io.Reader
	writeBatch  func(*leveldb.Batch, *opt.WriteOptions) error
}

func normalizedConfig(c Config) (Config, error) {
	if c.Version != Version1 || len(c.Principals) != 2 || c.MaxSessions == 0 || c.MaxSessions > 4096 || c.MaxChallenges == 0 || c.MaxChallenges > 4096 || c.MaxEvents == 0 || c.MaxEvents > 16384 {
		return Config{}, ErrCorrupt
	}
	c.Principals = append([]FixturePrincipal(nil), c.Principals...)
	ids := map[string]bool{}
	fixtures := map[string]bool{}
	for _, p := range c.Principals {
		id, err := PrincipalID(p.Issuer, p.Subject)
		if err != nil || ids[id] || fixtures[p.Fixture] {
			return Config{}, ErrCorrupt
		}
		identity, err := FixtureIdentity(p.Fixture)
		if err != nil || ValidateFixtureIdentity(identity.RootPublicKey, identity.GymPublicKey, identity.GymID) != nil {
			return Config{}, ErrCorrupt
		}
		ids[id] = true
		fixtures[p.Fixture] = true
	}
	sort.Slice(c.Principals, func(i, j int) bool {
		a, _ := PrincipalID(c.Principals[i].Issuer, c.Principals[i].Subject)
		b, _ := PrincipalID(c.Principals[j].Issuer, c.Principals[j].Subject)
		return a < b
	})
	return c, nil
}
func canonical(raw []byte, out any) error {
	if len(raw) == 0 || len(raw) > MaxRecordSize {
		return ErrCorrupt
	}
	if json.Unmarshal(raw, out) != nil {
		return ErrCorrupt
	}
	encoded, err := json.Marshal(out)
	if err != nil || !bytes.Equal(raw, encoded) {
		return ErrCorrupt
	}
	return nil
}
func put(batch *leveldb.Batch, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > MaxRecordSize {
		return ErrCorrupt
	}
	batch.Put([]byte(key), raw)
	return nil
}
func validID(id string) bool {
	if len(id) != 64 || strings.ToLower(id) != id {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
func tokenID(token string) (string, error) {
	if !validID(token) {
		return "", ErrUnauthorized
	}
	raw, _ := hex.DecodeString(token)
	sum := sha256.Sum256(raw)
	clear(raw)
	return hex.EncodeToString(sum[:]), nil
}
func randomID(reader io.Reader) (string, error) {
	var raw [32]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return "", errors.New("account randomness unavailable")
	}
	return hex.EncodeToString(raw[:]), nil
}
func regular(path string, directory bool) error {
	st, err := os.Lstat(path)
	if err != nil || st.Mode()&os.ModeSymlink != 0 || (directory && !st.IsDir()) || (!directory && !st.Mode().IsRegular()) {
		return ErrCorrupt
	}
	return nil
}
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
func readMarker(path string) (marker, error) {
	var m marker
	if regular(path, false) != nil {
		return m, ErrCorrupt
	}
	f, err := os.Open(path)
	if err != nil {
		return m, ErrCorrupt
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, MaxRecordSize+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || canonical(raw, &m) != nil {
		return m, ErrCorrupt
	}
	return m, nil
}

// Create creates only a new private directory. Partial initialization remains
// visible and is never silently replaced. Both published fixtures are required.
func Create(path string, config Config) (*Store, error) {
	return createStore(path, config, nil)
}
func createStore(path string, config Config, identity *IdentityConfig) (*Store, error) {
	return createStoreWithClock(path, config, identity, time.Now)
}
func createStoreWithClock(path string, config Config, identity *IdentityConfig, clock func() time.Time) (*Store, error) {
	return createStorePinnedWithClock(path, config, identity, clock, "")
}
func createStorePinnedWithClock(path string, config Config, identity *IdentityConfig, clock func() time.Time, authority string) (*Store, error) {
	version := Version1
	if identity != nil {
		version = IdentityVersion
	}
	c, err := normalizedConfig(config)
	if err != nil {
		return nil, err
	}
	if authority == "" {
		authority, err = randomID(rand.Reader)
		if err != nil {
			return nil, err
		}
	}
	if err = os.Mkdir(path, 0700); err != nil {
		return nil, errors.New("account directory unavailable")
	}
	m := marker{Application: application, Version: version, Identity: identity, Network: FixtureNetwork, Genesis: FixtureGenesis, Profile: FixtureProfile, HoldMS: RecoveryHoldMS, AuthorityID: authority, Config: c}
	raw, _ := json.Marshal(m)
	f, err := os.OpenFile(filepath.Join(path, markerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, ErrCorrupt
	}
	_, werr := f.Write(raw)
	if err = errors.Join(werr, f.Sync(), f.Close()); err != nil {
		return nil, ErrCorrupt
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfExist: true})
	if err != nil {
		return nil, ErrCorrupt
	}
	s := &Store{gate: make(chan struct{}, 1), authorityID: authority, db: db, meta: metadata{Application: application, Version: version, Identity: identity, Network: FixtureNetwork, Genesis: FixtureGenesis, Profile: FixtureProfile, HoldMS: RecoveryHoldMS, AuthorityID: authority, Config: c}, now: clock, random: rand.Reader, writeBatch: db.Write}
	batch := new(leveldb.Batch)
	if err = put(batch, "meta", s.meta); err == nil {
		err = db.Write(batch, &opt.WriteOptions{Sync: true})
	}
	if err == nil {
		err = syncDirectory(path)
	}
	if err == nil {
		err = syncDirectory(filepath.Dir(path))
	}
	if err != nil {
		db.Close()
		return nil, ErrPoisoned
	}
	return s, nil
}

// Open requires a complete matching fixture database; no migration or implicit
// creation occurs. It audits all bounded records and cross-links before use.
func Open(path string, config Config) (*Store, error) {
	return openStore(path, config, nil)
}
func openStore(path string, config Config, identity *IdentityConfig) (*Store, error) {
	return openStoreWithClock(path, config, identity, time.Now)
}
func openStoreWithClock(path string, config Config, identity *IdentityConfig, clock func() time.Time) (*Store, error) {
	return openStorePinnedWithClock(path, config, identity, clock, "")
}
func openStorePinnedWithClock(path string, config Config, identity *IdentityConfig, clock func() time.Time, authority string) (*Store, error) {
	version := Version1
	if identity != nil {
		version = IdentityVersion
	}
	c, err := normalizedConfig(config)
	if err != nil {
		return nil, err
	}
	if regular(path, true) != nil || regular(filepath.Join(path, "db"), true) != nil {
		return nil, ErrCorrupt
	}
	m, err := readMarker(filepath.Join(path, markerName))
	if err != nil || m.Application != application || m.Version != version || !reflect.DeepEqual(m.Identity, identity) || m.Network != FixtureNetwork || m.Genesis != FixtureGenesis || m.Profile != FixtureProfile || m.HoldMS != RecoveryHoldMS || !validID(m.AuthorityID) || !reflect.DeepEqual(m.Config, c) {
		return nil, ErrCorrupt
	}
	// An externally persisted setup pin is checked before LevelDB can create
	// a lock/log or recover the store. Marker contents never select the pin.
	if authority != "" && m.AuthorityID != authority {
		return nil, ErrCorrupt
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true})
	if err != nil {
		return nil, errors.New("account database unavailable")
	}
	s := &Store{gate: make(chan struct{}, 1), authorityID: m.AuthorityID, db: db, now: clock, random: rand.Reader, writeBatch: db.Write}
	raw, err := db.Get([]byte("meta"), nil)
	if err != nil || canonical(raw, &s.meta) != nil || s.meta.Application != application || s.meta.Version != version || !reflect.DeepEqual(s.meta.Identity, identity) || s.meta.Network != m.Network || s.meta.Genesis != m.Genesis || s.meta.Profile != m.Profile || s.meta.HoldMS != m.HoldMS || s.meta.AuthorityID != m.AuthorityID || !reflect.DeepEqual(s.meta.Config, c) || s.audit() != nil {
		db.Close()
		return nil, ErrCorrupt
	}
	return s, nil
}
func (s *Store) lock(ctx context.Context) error {
	if ctx == nil {
		return ErrUnauthorized
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.gate <- struct{}{}:
	}
	if err := ctx.Err(); err != nil {
		<-s.gate
		return err
	}
	if err := s.state(); err != nil {
		<-s.gate
		return err
	}
	return nil
}
func (s *Store) unlock() { <-s.gate }
func (s *Store) state() error {
	if s.closed {
		return ErrClosed
	}
	if s.poisoned {
		return ErrPoisoned
	}
	return nil
}
func (s *Store) Close() error {
	s.gate <- struct{}{}
	defer s.unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	err := s.db.Close()
	if s.poisoned {
		return errors.Join(ErrPoisoned, err)
	}
	return err
}
func (s *Store) AuthorityID() string { return s.authorityID } // Immutable for handle lifetime.
func (s *Store) load(key string, out any) error {
	raw, err := s.db.Get([]byte(key), nil)
	if errors.Is(err, leveldb.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return ErrCorrupt
	}
	return canonical(raw, out)
}
func (s *Store) commit(batch *leveldb.Batch, next metadata) error {
	if put(batch, "meta", next) != nil {
		return ErrCorrupt
	}
	if s.writeBatch(batch, &opt.WriteOptions{Sync: true}) != nil {
		s.poisoned = true
		return ErrPoisoned
	}
	s.meta = next
	return nil
}
func (s *Store) observe() (int64, error) {
	now := s.now().UnixMilli()
	if now <= 0 || now < s.meta.ClockMS || now > math.MaxInt64-RecoveryHoldMS {
		return 0, ErrClock
	}
	if now > s.meta.ClockMS {
		next := s.meta
		next.ClockMS = now
		if err := s.commit(new(leveldb.Batch), next); err != nil {
			return 0, err
		}
	}
	return now, nil
}
func (s *Store) fixture(principalID string) (Identity, error) {
	for _, p := range s.meta.Config.Principals {
		id, _ := PrincipalID(p.Issuer, p.Subject)
		if id == principalID {
			return FixtureIdentity(p.Fixture)
		}
	}
	return Identity{}, ErrUnauthorized
}
func (s *Store) principal(p Principal, now int64) (string, error) {
	id, err := PrincipalID(p.Issuer, p.Subject)
	if err != nil {
		return "", ErrUnauthorized
	}
	if _, err = s.fixture(id); err != nil {
		return "", err
	}
	if p.AuthenticatedAtMS <= 0 || p.AuthenticatedAtMS > now || now-p.AuthenticatedAtMS > FreshAuthenticationMS {
		return "", ErrUnauthorized
	}
	return id, nil
}
func copyAccount(a Account) Account {
	if a.Enrollment != nil {
		identity := *a.Enrollment
		a.Enrollment = &identity
	}
	return a
}
func (s *Store) account(id string) (Account, error) {
	var a Account
	if !validID(id) {
		return a, ErrNotFound
	}
	if err := s.load("account:"+id, &a); err != nil {
		return a, err
	}
	if a.ID != id || s.validateAccount(a) != nil {
		return Account{}, ErrCorrupt
	}
	return a, nil
}
func (s *Store) validateAccount(a Account) error {
	expected, err := s.fixture(a.PrincipalID)
	if err != nil || a.Version != Version1 || !validID(a.ID) || a.Generation == 0 || a.HoldUntilMS < 0 || a.HoldUntilMS > s.meta.ClockMS+RecoveryHoldMS {
		return ErrCorrupt
	}
	if a.Enrollment != nil && *a.Enrollment != expected {
		return ErrCorrupt
	}
	if a.RecoveryRequired && a.Enrollment == nil {
		return ErrCorrupt
	}
	return nil
}
func (s *Store) event(batch *leveldb.Batch, next *metadata, kind string, a Account, at int64, reference string) error {
	if uint64(next.Events)+uint64(next.ReservedEvents) >= uint64(next.Config.MaxEvents) {
		return ErrCapacity
	}
	next.Events++
	return put(batch, fmt.Sprintf("event:%016x", next.Events), Event{Version: Version1, Sequence: uint64(next.Events), Kind: kind, AccountID: a.ID, Generation: a.Generation, AtMS: at, Reference: reference})
}

func (s *Store) commitMutation(ctx context.Context, batch *leveldb.Batch, next metadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.commit(batch, next)
}
