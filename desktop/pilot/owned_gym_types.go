// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
)

const (
	OwnedGymMode            = "local-owned-gym"
	MaxOwnedGymRequestSize  = 8 * 1024
	MaxOwnedGymResponseSize = 1024 * 1024
)

// OwnedGymView is public correlation, never an account capability. The native
// caller also retains the exact original helper and independently expected source.
type OwnedGymView struct {
	Version            uint32 `json:"version"`
	ViewID             string `json:"view_id"`
	AccountAuthorityID string `json:"account_authority_id"`
	AccountID          string `json:"account_id"`
	Generation         uint64 `json:"generation"`
	Fixture            string `json:"fixture"`
	RootPublicKey      string `json:"root_public_key"`
	GymPublicKey       string `json:"gym_public_key"`
	GymID              string `json:"gym_id"`
}

type OwnedGymSnapshot struct {
	Version        uint32                 `json:"version"`
	Mode           string                 `json:"mode"`
	State          string                 `json:"state"`
	Account        AccountSnapshot        `json:"account"`
	View           *OwnedGymView          `json:"view"`
	Source         OwnedSourceConfig      `json:"source"`
	Child          *OwnedGymChildSnapshot `json:"child"`
	PendingResetID string                 `json:"pending_reset_id"`
	Error          string                 `json:"error"`
}

// No legacy stable/event snapshot is embedded in this distinct mode. Rate is
// integer thousandths of a hash/second, avoiding float tokens in the closed codec.
type OwnedGymChildSnapshot struct {
	State                string                    `json:"state"`
	Attempts             uint64                    `json:"attempts"`
	HashesPerSecondMilli uint64                    `json:"hashes_per_second_milli"`
	Height               uint64                    `json:"height"`
	Pending              uint64                    `json:"pending"`
	LastError            string                    `json:"last_error"`
	WalletState          string                    `json:"wallet_state"`
	WalletHeight         uint64                    `json:"wallet_height"`
	PayoutAddress        string                    `json:"payout_address"`
	SpendableAtoms       int64                     `json:"spendable_atoms"`
	ImmatureAtoms        int64                     `json:"immature_atoms"`
	StableStoreID        string                    `json:"stable_store_id"`
	EntryStoreID         string                    `json:"entry_store_id"`
	EventStoreID         string                    `json:"event_store_id"`
	OwnershipHead        *fighterownership.HeadRef `json:"ownership_head"`
	Backup               HostedBackupSnapshot      `json:"backup"`
}

// CaptureID maps to one retained *gymstable.OwnedRead in this original child.
// Page is the first page, not authority to reconstruct that handle.
type OwnedStableCapture struct {
	Version   uint32              `json:"version"`
	CaptureID string              `json:"capture_id"`
	ViewID    string              `json:"view_id"`
	Page      gymstable.OwnedPage `json:"page"`
}

func ValidateOwnedGymSource(c OwnedSourceConfig) error {
	if c.Version != 1 || registry.ValidatePublicManifestPin(c.Ownership) != nil || !payoutNonzeroID(c.Store.StoreID) || c.PublicReadPort == 0 {
		return gymstable.ErrInvalid
	}
	return nil
}
func ValidateOwnedGymView(v OwnedGymView) error {
	f, e := gymaccount.FixtureIdentity(v.Fixture)
	if e != nil || v.Version != 1 || !payoutNonzeroID(v.ViewID) || !payoutNonzeroID(v.AccountAuthorityID) || !payoutNonzeroID(v.AccountID) || v.Generation == 0 || v.Generation > gymstable.MaxSafeInteger || v.RootPublicKey != f.RootPublicKey || v.GymPublicKey != f.GymPublicKey || v.GymID != f.GymID {
		return gymstable.ErrInvalid
	}
	return nil
}
func ownedGymStablePin(source OwnedSourceConfig, v OwnedGymView) (gymstable.OwnedPin, error) {
	if ValidateOwnedGymSource(source) != nil || ValidateOwnedGymView(v) != nil {
		return gymstable.OwnedPin{}, gymstable.ErrInvalid
	}
	f, _ := gymaccount.FixtureIdentity(v.Fixture)
	return gymstable.ValidateOwnedPin(gymstable.OwnedPin{Version: 1, Stable: gymstable.Pin{Version: gymstable.SharedPinVersion2, AccountAuthorityID: v.AccountAuthorityID, AccountID: v.AccountID, Identity: f, Source: source.Ownership.Source, ConditionVersion: 1, ProjectorID: fightercondition.ProjectorID}, Ownership: source.Ownership})
}

// This is bounded structural/context validation, not live replay or proof of a
// particular helper's authority. expected must come from the original sealed
// source configuration, never from s.Source itself.
func ValidateOwnedGymSnapshot(s OwnedGymSnapshot, expected OwnedSourceConfig, expectedEvent gymevent.OwnedPin) error {
	bad := gymstable.ErrInvalid
	if s.Version != 1 || s.Mode != OwnedGymMode || ValidateOwnedGymSource(expected) != nil || s.Source != expected || s.State != s.Account.State || !stableOne(s.Error, "", "owned_unavailable", "hosted_reset_pending") {
		return bad
	}
	a := s.Account
	pending := a.LastError == "hosted_reset_pending" && stableOne(a.State, "signed_out", "failed") && stableOne(a.Fixture, "zero256", "incrementing256")
	if !stableOne(a.State, "signed_out", "signing_in", "signed_in", "resetting", "signing_out", "failed", "expired") || !stableOne(a.Fixture, "", "zero256", "incrementing256") || !(stableOne(a.LastError, "", "account_unavailable") || pending) || a.Generation > gymstable.MaxSafeInteger || a.HoldUntilMS < 0 || a.HoldUntilMS > fighterownership.MaxTimestampMS || a.Held && (a.CanExport || a.HoldUntilMS == 0) {
		return bad
	}
	if pending {
		if !payoutNonzeroID(s.PendingResetID) {
			return bad
		}
	} else if s.PendingResetID != "" {
		return bad
	}
	if a.State != "signed_in" {
		if s.View != nil || s.Child != nil || a.ID != "" || a.Generation != 0 || a.HoldUntilMS != 0 || a.Held || a.CanExport || (s.Error == "hosted_reset_pending") != pending {
			return bad
		}
		return nil
	}
	if s.View == nil || s.Child == nil || s.Error != "" || a.LastError != "" || ValidateOwnedGymView(*s.View) != nil || s.View.AccountID != a.ID || s.View.Generation != a.Generation || s.View.Fixture != a.Fixture {
		return bad
	}
	c := s.Child
	if !stableOne(c.State, "stopped", "starting", "running", "stopping", "failed") || !stableOne(c.WalletState, "closed", "connecting", "syncing", "ready", "failed") || !PoolStatusCode(c.LastError) || c.Attempts > gymstable.MaxSafeInteger || c.HashesPerSecondMilli > gymstable.MaxSafeInteger || c.Height > gymstable.MaxSafeInteger || c.WalletHeight > gymstable.MaxSafeInteger || c.Pending > 1000 || c.SpendableAtoms < 0 || c.SpendableAtoms > poolsettle.MaxMoney || c.ImmatureAtoms < 0 || c.ImmatureAtoms > poolsettle.MaxMoney || !poolPayoutPattern.MatchString(c.PayoutAddress) || !payoutNonzeroID(c.EventStoreID) {
		return bad
	}
	sp, e := ownedGymStablePin(expected, *s.View)
	if e != nil {
		return bad
	}
	sid, e := gymstable.OwnedID(sp)
	if e != nil || c.StableStoreID != sid {
		return bad
	}
	ns, e := gymentry.NamespaceID(sp.Stable.Identity.Network, sp.Stable.Identity.Genesis, sp.Stable.AccountAuthorityID)
	if e != nil {
		return bad
	}
	eid, e := gymentry.OwnedID(gymentry.OwnedPin{Version: 1, NamespaceID: ns, CalendarID: gymentry.CalendarID(), RulesID: gymentry.RulesID(), StableStoreID: sid, StablePin: sp})
	if e != nil || c.EntryStoreID != eid {
		return bad
	}
	eventPin, eventErr := gymevent.ValidateOwnedPin(expectedEvent)
	if eventErr != nil || eventPin.Ownership != expected.Ownership {
		return bad
	}
	eventID, eventErr := gymevent.OwnedID(eventPin)
	if eventErr != nil || c.EventStoreID != eventID {
		return bad
	}
	matched := 0
	for _, ep := range eventPin.Participants {
		if ep.StablePin == sp && ep.StableStoreID == sid {
			matched++
		}
	}
	if matched != 1 {
		return bad
	}
	if c.OwnershipHead != nil && fighterownership.ValidateHead(expected.Ownership, *c.OwnershipHead) != nil {
		return bad
	}
	b := PoolSnapshot{State: c.State, BackupState: c.Backup.State, BackupPhase: c.Backup.Phase, BackupError: c.Backup.Error}
	if !poolBackupValid(b) {
		return bad
	}
	if !ownedcodec.Fits(s, MaxOwnedGymResponseSize) {
		return bad
	}
	return nil
}
func ValidateOwnedStableCapture(c OwnedStableCapture, expected OwnedSourceConfig, view OwnedGymView) error {
	if c.Version != 1 || !payoutNonzeroID(c.CaptureID) || c.ViewID != view.ViewID || c.Page.After != 0 {
		return gymstable.ErrInvalid
	}
	p, e := ownedGymStablePin(expected, view)
	if e != nil || c.Page.Status.Pin != p {
		return gymstable.ErrInvalid
	}
	if _, e = gymstable.ValidateOwnedPage(c.Page); e != nil {
		return e
	}
	if !ownedcodec.Fits(c, MaxOwnedGymResponseSize) {
		return gymstable.ErrInvalid
	}
	return nil
}

// OwnedGymScope is the immutable setup disclosure for one explicit fixture.
// Native seals it to the original launched helper/root/fixture before using
// action responses. Validation recomputes identities but grants no capability.
type OwnedGymScope struct {
	Version          uint32                  `json:"version"`
	Fixture          string                  `json:"fixture"`
	Source           OwnedSourceConfig       `json:"source"`
	PoolSource       poolshare.Config        `json:"pool_source"`
	SourceBindingID  string                  `json:"source_binding_id"`
	StablePin        gymstable.OwnedPin      `json:"stable_pin"`
	StableStoreID    string                  `json:"stable_store_id"`
	EntryPin         gymentry.OwnedPin       `json:"entry_pin"`
	EntryStoreID     string                  `json:"entry_store_id"`
	EventPin         gymevent.OwnedPin       `json:"event_pin"`
	EventStoreID     string                  `json:"event_store_id"`
	Carrier          fightercarrier.Identity `json:"carrier"`
	CarrierBindingID string                  `json:"carrier_binding_id"`
}

func ValidateOwnedGymScope(s OwnedGymScope) error {
	bad := gymstable.ErrInvalid
	f, e := gymaccount.FixtureIdentity(s.Fixture)
	if e != nil || s.Version != 1 || ValidateOwnedGymSource(s.Source) != nil || s.PoolSource.AuthorityPin != s.Source.Ownership.Source {
		return bad
	}
	sourceID, e := poolservice.BindingID(s.PoolSource)
	if e != nil || sourceID != s.SourceBindingID {
		return bad
	}
	sp, e := gymstable.ValidateOwnedPin(s.StablePin)
	if e != nil || sp.Ownership != s.Source.Ownership || sp.Stable.Identity != f {
		return bad
	}
	sid, e := gymstable.OwnedID(sp)
	if e != nil || sid != s.StableStoreID {
		return bad
	}
	ep, e := gymentry.ValidateOwnedPin(s.EntryPin)
	if e != nil || ep.StablePin != sp || ep.StableStoreID != sid {
		return bad
	}
	eid, e := gymentry.OwnedID(ep)
	if e != nil || eid != s.EntryStoreID {
		return bad
	}
	event, e := gymevent.ValidateOwnedPin(s.EventPin)
	if e != nil || event.Ownership != s.Source.Ownership {
		return bad
	}
	eventID, e := gymevent.OwnedID(event)
	if e != nil || eventID != s.EventStoreID {
		return bad
	}
	matched := 0
	for _, member := range event.Participants {
		if member == ep {
			matched++
		}
	}
	if matched != 1 {
		return bad
	}
	cid, e := fightercarrier.BindingID(s.Carrier)
	if e != nil || cid != s.CarrierBindingID {
		return bad
	}
	ownershipID, e := fighterownership.ID(s.Source.Ownership)
	if e != nil {
		return bad
	}
	cp := s.Carrier.Pin
	if cp.OwnerMode != "hosted" || cp.ManagerSchema != 14 || cp.OwnershipPinID != ownershipID || cp.AccountAuthorityID != sp.Stable.AccountAuthorityID || cp.AccountID != sp.Stable.AccountID || cp.Network != f.Network || cp.Genesis != f.Genesis || cp.WalletProfile != f.Profile || cp.RootPublicKey != f.RootPublicKey || cp.GymPublicKey != f.GymPublicKey || cp.GymID != f.GymID {
		return bad
	}
	if !ownedcodec.Fits(s, MaxOwnedGymResponseSize) {
		return bad
	}
	return nil
}
func (s OwnedGymScope) ViewMatches(v OwnedGymView) bool {
	return ValidateOwnedGymScope(s) == nil && ValidateOwnedGymView(v) == nil && v.Fixture == s.Fixture && v.AccountAuthorityID == s.StablePin.Stable.AccountAuthorityID && v.AccountID == s.StablePin.Stable.AccountID && v.RootPublicKey == s.Carrier.Pin.RootPublicKey && v.GymPublicKey == s.Carrier.Pin.GymPublicKey && v.GymID == s.Carrier.Pin.GymID
}
func ValidateOwnedGymSnapshotWithScope(s OwnedGymSnapshot, expected OwnedGymScope) error {
	if ValidateOwnedGymScope(expected) != nil || ValidateOwnedGymSnapshot(s, expected.Source, expected.EventPin) != nil {
		return gymstable.ErrInvalid
	}
	if s.View != nil && (!expected.ViewMatches(*s.View) || s.Child.StableStoreID != expected.StableStoreID || s.Child.EntryStoreID != expected.EntryStoreID || s.Child.EventStoreID != expected.EventStoreID) {
		return gymstable.ErrInvalid
	}
	return nil
}
func ValidateOwnedStableCaptureWithScope(c OwnedStableCapture, expected OwnedGymScope, view OwnedGymView) error {
	if !expected.ViewMatches(view) {
		return gymstable.ErrInvalid
	}
	return ValidateOwnedStableCapture(c, expected.Source, view)
}
