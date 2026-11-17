// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
)

type stableInstall struct {
	AccountID string        `json:"account_id"`
	Pin       gymstable.Pin `json:"pin"`
	Phase     string        `json:"phase"`
}
type stableRootMarker struct {
	Version            uint32          `json:"version"`
	Mode               string          `json:"mode"`
	AccountAuthorityID string          `json:"account_authority_id"`
	DescriptorID       string          `json:"descriptor_id"`
	StableSchema       uint32          `json:"stable_schema"`
	StableInstalls     []stableInstall `json:"stable_installs"`
}

// ValidateStableUpgradeChild validates an existing child for the parent upgrade.
// It never initializes, promotes, prepares, unlocks or hashes. Existing LevelDB
// and wallet handles may maintain their own lock/log metadata during validation.
func ValidateStableUpgradeChild(config Config, authorityID, accountID string) error {
	var m manifestIdentity
	if readManifestJSON(filepath.Join(config.DataDir, manifestMarkerName), 8192, &m) != nil || !m.valid() || m.AccountID != accountID || m.AccountAuthorityID != authorityID {
		return ErrManifestInitialization
	}
	if info, err := os.Lstat(filepath.Join(config.DataDir, "instance-lock")); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrManifestInitialization
	}
	if m.Phase == "new" {
		return (&ManifestController{config: config}).checkNewData()
	}
	a := ManifestAccountConfig{Account: AccountConfig{AccountID: accountID, AuthorityID: authorityID, Fixture: m.Fixture, RequireExisting: true}, ViewID: strings.Repeat("0", 64), Generation: 1,
		ViewLease: func(context.Context, func(gymaccount.Account, func() error) error) error {
			return gymaccount.ErrUnauthorized
		}}
	// Constructor dependency validation requires an authorizer and token source,
	// but this read-only route must never invoke either.
	a.Account.Authorizer = stableRejectAuthorizer{}
	a.Account.MiningToken = func(context.Context) (string, error) { return "", gymaccount.ErrUnauthorized }
	c, err := newManifestAccount(config, a, true)
	if err != nil {
		return err
	}
	return c.Close()
}

type stableRejectAuthorizer struct{}

func (stableRejectAuthorizer) WithSession(context.Context, string, func(string, func() error) error) error {
	return gymaccount.ErrUnauthorized
}

func (s *StableController) StablePin() (gymstable.Pin, error) {
	c := s.ManifestController
	c.mu.Lock()
	m := c.manifest
	c.mu.Unlock()
	if m.Phase != "ready" || !m.valid() {
		return gymstable.Pin{}, ErrManifestInitialization
	}
	id, err := gymaccount.FixtureIdentity(m.Fixture)
	if err != nil {
		return gymstable.Pin{}, err
	}
	return gymstable.ValidatePin(gymstable.Pin{Version: 1, AccountAuthorityID: m.AccountAuthorityID, AccountID: m.AccountID, Identity: id, Source: m.AuthorityPin, ConditionVersion: fightercondition.Version1, ProjectorID: fightercondition.ProjectorID})
}
func (s *StableController) installPhase() (string, error) {
	if s.entryEnabled {
		return s.entryStablePhase()
	}
	pin, err := s.StablePin()
	if err != nil {
		return "", err
	}
	var root stableRootMarker
	path := filepath.Join(filepath.Dir(filepath.Dir(s.config.DataDir)), "SLUG-MANIFEST-ACCOUNTS.json")
	if readManifestJSON(path, 8192, &root) != nil || root.Version != 2 || root.Mode != ManifestMode || root.AccountAuthorityID != pin.AccountAuthorityID || root.DescriptorID != pin.Source.DescriptorID || root.StableSchema != 1 || root.StableInstalls == nil || len(root.StableInstalls) > 2 {
		return "", ErrManifestInitialization
	}
	previous := ""
	phase := ""
	for _, entry := range root.StableInstalls {
		p, e := gymstable.ValidatePin(entry.Pin)
		if e != nil || entry.AccountID <= previous || entry.AccountID != p.AccountID || p.AccountAuthorityID != root.AccountAuthorityID || p.Source.DescriptorID != root.DescriptorID || (entry.Phase != "prepared" && entry.Phase != "ready") {
			return "", ErrManifestInitialization
		}
		previous = entry.AccountID
		if entry.AccountID == pin.AccountID {
			if p != pin {
				return "", ErrManifestInitialization
			}
			phase = entry.Phase
		}
	}
	if phase == "" {
		return "", ErrManifestInitialization
	}
	return phase, nil
}
func (s *StableController) CreateStableComponent(ctx context.Context) (resultErr error) {
	phase, err := s.installPhase()
	if err != nil || phase != "prepared" {
		return ErrManifestInitialization
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	pin, err := s.StablePin()
	if err != nil {
		return err
	}
	source, err := registry.OpenManifestAuthority(filepath.Join(s.config.DataDir, "manifest-authority"), pin.Source, true)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, source.Close()) }()
	path := filepath.Join(s.config.DataDir, "stable")
	var store *gymstable.Store
	if _, e := os.Lstat(path); os.IsNotExist(e) {
		store, err = gymstable.Create(path, pin, source)
	} else if e == nil {
		store, err = gymstable.Open(path, pin, source)
	} else {
		return e
	}
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, store.Close()) }()
	status, err := store.Status(ctx, s.session())
	if err != nil {
		return err
	}
	if status.Checkpoint != 0 || status.OperationCount != 0 || status.RosterRevision != 0 || status.Slots != ([10]string{}) || status.PendingOperationID != "" || status.LastOperationID != "" {
		return ErrManifestInitialization
	}
	return ctx.Err()
}
