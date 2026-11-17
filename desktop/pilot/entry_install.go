// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"io"
	"os"
	"path/filepath"
)

type entryInstall struct {
	AccountID string       `json:"account_id"`
	Pin       gymentry.Pin `json:"pin"`
	Phase     string       `json:"phase"`
}
type entryRootMarker struct {
	Version            uint32          `json:"version"`
	Mode               string          `json:"mode"`
	AccountAuthorityID string          `json:"account_authority_id"`
	DescriptorID       string          `json:"descriptor_id"`
	StableSchema       uint32          `json:"stable_schema"`
	StableInstalls     []stableInstall `json:"stable_installs"`
	EntrySchema        uint32          `json:"entry_schema"`
	CalendarID         string          `json:"entry_calendar_id"`
	RulesID            string          `json:"entry_rules_id"`
	EntryInstalls      []entryInstall  `json:"entry_installs"`
}

func NewEntryManifestAccount(c Config, a StableAccountConfig) (*StableController, error) {
	s, e := NewStableManifestAccount(c, a)
	if e != nil {
		return nil, e
	}
	s.entryEnabled = true
	return s, nil
}
func (s *StableController) EntryPin() (gymentry.Pin, error) {
	p, e := s.StablePin()
	if e != nil {
		return gymentry.Pin{}, e
	}
	sid, e := gymstable.ID(p)
	if e != nil {
		return gymentry.Pin{}, e
	}
	namespace, e := gymentry.NamespaceID(p.Identity.Network, p.Identity.Genesis, p.AccountAuthorityID)
	if e != nil {
		return gymentry.Pin{}, e
	}
	return gymentry.ValidatePin(gymentry.Pin{Version: 1, NamespaceID: namespace, CalendarID: gymentry.CalendarID(), RulesID: gymentry.RulesID(), StableStoreID: sid, StablePin: p})
}
func (s *StableController) entryRoot() (entryRootMarker, error) {
	var m entryRootMarker
	path := filepath.Join(filepath.Dir(filepath.Dir(s.config.DataDir)), "SLUG-MANIFEST-ACCOUNTS.json")
	if !s.entryEnabled || readManifestJSON(path, 16384, &m) != nil || m.Version != 3 || m.Mode != ManifestMode || m.StableSchema != 1 || m.EntrySchema != 1 || m.CalendarID != gymentry.CalendarID() || m.RulesID != gymentry.RulesID() || m.StableInstalls == nil || len(m.StableInstalls) > 2 || m.EntryInstalls == nil || len(m.EntryInstalls) > 2 {
		return m, ErrManifestInitialization
	}
	last := ""
	stable := map[string]stableInstall{}
	for _, x := range m.StableInstalls {
		p, e := gymstable.ValidatePin(x.Pin)
		if e != nil || x.AccountID <= last || x.AccountID != p.AccountID || p.AccountAuthorityID != m.AccountAuthorityID || p.Source.DescriptorID != m.DescriptorID || (x.Phase != "ready" && x.Phase != "prepared") {
			return m, ErrManifestInitialization
		}
		last = x.AccountID
		stable[x.AccountID] = x
	}
	last = ""
	for _, x := range m.EntryInstalls {
		p, e := gymentry.ValidatePin(x.Pin)
		y, ok := stable[x.AccountID]
		if e != nil || x.AccountID <= last || x.AccountID != p.StablePin.AccountID || !ok || y.Phase != "ready" || y.Pin != p.StablePin || (x.Phase != "ready" && x.Phase != "prepared") {
			return m, ErrManifestInitialization
		}
		last = x.AccountID
	}
	return m, nil
}
func (s *StableController) entryStablePhase() (string, error) {
	m, e := s.entryRoot()
	if e != nil {
		return "", e
	}
	p, e := s.StablePin()
	if e != nil {
		return "", e
	}
	for _, x := range m.StableInstalls {
		if x.AccountID == p.AccountID && x.Pin == p {
			return x.Phase, nil
		}
	}
	return "", ErrManifestInitialization
}
func (s *StableController) entryInstallPhase() (string, error) {
	m, e := s.entryRoot()
	if e != nil {
		return "", e
	}
	p, e := s.EntryPin()
	if e != nil {
		return "", e
	}
	for _, x := range m.EntryInstalls {
		if x.AccountID == p.StablePin.AccountID && x.Pin == p {
			return x.Phase, nil
		}
	}
	return "", ErrManifestInitialization
}
func (s *StableController) CreateEntryComponent(ctx context.Context) (resultErr error) {
	phase, e := s.entryInstallPhase()
	if e != nil || phase != "prepared" {
		return ErrManifestInitialization
	}
	pin, e := s.EntryPin()
	if e != nil {
		return e
	}
	source, e := registry.OpenManifestAuthority(filepath.Join(s.config.DataDir, "manifest-authority"), pin.StablePin.Source, true)
	if e != nil {
		return e
	}
	defer func() { resultErr = errors.Join(resultErr, source.Close()) }()
	stable, e := gymstable.Open(filepath.Join(s.config.DataDir, "stable"), pin.StablePin, source)
	if e != nil {
		return e
	}
	defer func() { resultErr = errors.Join(resultErr, stable.Close()) }()
	path := filepath.Join(s.config.DataDir, "entries")
	var store *gymentry.Store
	info, stat := os.Lstat(path)
	if os.IsNotExist(stat) {
		store, e = gymentry.Create(ctx, path, pin, stable)
	} else if stat != nil {
		return stat
	} else {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrManifestInitialization
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		names, err := f.Readdirnames(1)
		_ = f.Close()
		if len(names) == 0 && (err == nil || errors.Is(err, io.EOF)) {
			store, e = gymentry.Create(ctx, path, pin, stable)
		} else {
			store, e = gymentry.Open(ctx, path, pin, stable)
		}
	}
	if e != nil {
		return e
	}
	defer func() { resultErr = errors.Join(resultErr, store.Close()) }()
	history, e := store.History(ctx, s.session(), "", 1)
	if e != nil {
		return e
	}
	pending, e := store.Pending(ctx, s.session())
	if e != nil {
		return e
	}
	if len(history.Events) != 0 || pending.Operation != nil {
		return ErrManifestInitialization
	}
	return ctx.Err()
}
func (s *StableController) AttachEntries(ctx context.Context) error {
	phase, e := s.entryInstallPhase()
	if e != nil || phase != "ready" {
		return ErrManifestInitialization
	}
	p, e := s.EntryPin()
	if e != nil {
		return e
	}
	id, e := gymentry.ID(p)
	if e != nil {
		return e
	}
	s.entryPin = p
	s.entryStoreID = id
	return s.AttachStable(ctx)
}
