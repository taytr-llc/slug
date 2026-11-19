// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/walletwithdraw"
	"github.com/taytr-llc/slugwallet/wallet"
)

const poolWithdrawalGuardName = "SLUG-POOL-WITHDRAWAL-V1.json"
const poolWithdrawalDirectory = "withdrawals"

// Only the parent establishes this scope. Children borrow a value, never a
// browser-selected directory, journal identity, or wallet namespace.
type poolWithdrawalScope struct {
	Identity    walletwithdraw.Identity
	BindingID   string
	journalPath string
	init        poolWithdrawalInit
	ready       poolWithdrawalReady
	guard       poolWithdrawalGuard
}
type poolWithdrawalGuard struct {
	Application        string           `json:"application"`
	Version            uint32           `json:"version"`
	SourceBindingID    string           `json:"source_binding_id"`
	Source             poolshare.Config `json:"source"`
	AccountAuthorityID string           `json:"account_authority_id"`
}
type poolWithdrawalInit struct {
	Application         string             `json:"application"`
	Version             uint32             `json:"version"`
	UpgradeGuardSHA256  string             `json:"upgrade_guard_sha256"`
	JournalID           string             `json:"journal_id"`
	Pin                 walletwithdraw.Pin `json:"pin"`
	WithdrawalBindingID string             `json:"withdrawal_binding_id"`
}
type poolWithdrawalReady struct {
	Application         string `json:"application"`
	Version             uint32 `json:"version"`
	IntentSHA256        string `json:"intent_sha256"`
	JournalID           string `json:"journal_id"`
	WithdrawalBindingID string `json:"withdrawal_binding_id"`
}

func withdrawalRecordHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func withdrawalPrivatePath(path string, directory, create bool) error {
	i, e := os.Lstat(path)
	if os.IsNotExist(e) && create && directory {
		if os.Mkdir(path, 0700) != nil || syncDirectory(filepath.Dir(path)) != nil {
			return ErrPoolRuntime
		}
		i, e = os.Lstat(path)
	}
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	if e != nil || i.Mode()&os.ModeSymlink != 0 || i.IsDir() != directory || (!directory && !i.Mode().IsRegular()) || i.Mode().Perm() != mode {
		return ErrPoolRuntime
	}
	return nil
}
func withdrawalReadRecord(path string, expected any) error {
	if withdrawalPrivatePath(path, false, false) != nil {
		return ErrPoolRuntime
	}
	return readPoolRuntimeRecord(path, expected)
}
func (r *PoolRuntime) withdrawalGuard() (poolWithdrawalGuard, error) {
	b, e := poolservice.BindingID(r.source)
	return poolWithdrawalGuard{"slug-pool-withdrawal-upgrade", 1, b, r.source, r.marker.AccountAuthorityID}, e
}

// Root inventory runs before opening wallets. An upgrade marker without its
// directory is a resumable first step; a directory without the marker is loss.
func (r *PoolRuntime) validateWithdrawalRoot(incoming ...PoolRecipientBinding) error {
	allowed := make(map[string]bool, 2)
	for _, id := range r.gymAccounts {
		allowed[id] = true
	}
	for _, b := range incoming {
		if ValidatePoolRecipientBinding(b) != nil || b.Config.Source != r.source || b.Config.AccountAuthorityID != r.marker.AccountAuthorityID {
			return ErrPoolRuntime
		}
		if old, exists := r.gymAccounts[b.Config.Fixture]; exists && old != b.Config.AccountID {
			return ErrPoolRuntime
		}
		allowed[b.Config.AccountID] = true
	}
	if len(allowed) > 2 {
		return ErrPoolRuntime
	}
	guard, e := r.withdrawalGuard()
	if e != nil {
		return e
	}
	gp := filepath.Join(r.config.DataDir, poolWithdrawalGuardName)
	hasGuard, e := poolRuntimeExists(gp)
	if e != nil {
		return e
	}
	dir := filepath.Join(r.config.DataDir, poolWithdrawalDirectory)
	hasDir, e := poolRuntimeExists(dir)
	if e != nil || hasDir && !hasGuard {
		return ErrPoolRuntime
	}
	if hasGuard && withdrawalReadRecord(gp, guard) != nil {
		return ErrPoolRuntime
	}
	if !hasDir {
		return nil
	}
	if withdrawalPrivatePath(dir, true, false) != nil {
		return ErrPoolRuntime
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) > 2 {
		return ErrPoolRuntime
	}
	for _, entry := range entries {
		if !validAccountID(entry.Name()) || !allowed[entry.Name()] || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return ErrPoolRuntime
		}
		if withdrawalScopeInventory(filepath.Join(dir, entry.Name())) != nil {
			return ErrPoolRuntime
		}
	}
	return nil
}
func withdrawalScopeInventory(root string) error {
	if withdrawalPrivatePath(root, true, false) != nil {
		return ErrPoolRuntime
	}
	entries, e := os.ReadDir(root)
	if e != nil || len(entries) > 3 {
		return ErrPoolRuntime
	}
	for _, entry := range entries {
		if entry.Name() != "INIT.json" && entry.Name() != "READY.json" && entry.Name() != "journal" {
			return ErrPoolRuntime
		}
		if withdrawalPrivatePath(filepath.Join(root, entry.Name()), entry.Name() == "journal", false) != nil {
			return ErrPoolRuntime
		}
	}
	return nil
}
func (r *PoolRuntime) ensureWithdrawalScope(ctx context.Context, b PoolRecipientBinding, w *walletRuntime) (scope poolWithdrawalScope, resultErr error) {
	if ctx == nil || ctx.Err() != nil || ValidatePoolRecipientBinding(b) != nil || b.Config.Source != r.source || b.Config.AccountAuthorityID != r.marker.AccountAuthorityID || w == nil {
		return scope, ErrPoolRuntime
	}
	if e := r.validateWithdrawalRoot(b); e != nil {
		return scope, e
	}
	guard, e := r.withdrawalGuard()
	if e != nil {
		return scope, e
	}
	gp := filepath.Join(r.config.DataDir, poolWithdrawalGuardName)
	if e = writePoolRuntimeRecord(gp, guard); e != nil {
		return scope, e
	}
	if withdrawalReadRecord(gp, guard) != nil {
		return scope, ErrPoolRuntime
	}
	dir := filepath.Join(r.config.DataDir, poolWithdrawalDirectory)
	if withdrawalPrivatePath(dir, true, true) != nil {
		return scope, ErrPoolRuntime
	}
	root := filepath.Join(dir, b.Config.AccountID)
	if withdrawalPrivatePath(root, true, true) != nil || withdrawalScopeInventory(root) != nil {
		return scope, ErrPoolRuntime
	}
	identity := b.Config.Identity
	pin := walletwithdraw.Pin{Version: 1, Network: identity.Network, Genesis: identity.Genesis, AccountAuthorityID: b.Config.AccountAuthorityID, AccountID: b.Config.AccountID, Fixture: b.Config.Fixture, WalletProfile: identity.Profile, RootPublicKey: identity.RootPublicKey, GymPublicKey: identity.GymPublicKey, GymID: identity.GymID, PoolSourceBindingID: guard.SourceBindingID}
	if walletwithdraw.ValidatePin(pin) != nil || !w.mu.TryLock() {
		return scope, ErrPoolRuntime
	}
	defer w.mu.Unlock()
	if w.closed || w.wallet == nil || w.identity != (walletIdentity{pin.RootPublicKey, pin.GymPublicKey, pin.GymID}) {
		return scope, ErrPoolRuntime
	}
	initPath, readyPath, journalPath := filepath.Join(root, "INIT.json"), filepath.Join(root, "READY.json"), filepath.Join(root, "journal")
	hasInit, e := poolRuntimeExists(initPath)
	if e != nil {
		return scope, e
	}
	hasReady, e := poolRuntimeExists(readyPath)
	if e != nil {
		return scope, e
	}
	hasJournal, e := poolRuntimeExists(journalPath)
	if e != nil || !hasInit && (hasReady || hasJournal) {
		return scope, ErrPoolRuntime
	}
	var init poolWithdrawalInit
	if hasInit {
		if readManifestJSON(initPath, poolRuntimeMarkerLimit, &init) != nil {
			return scope, ErrPoolRuntime
		}
	} else {
		var id [32]byte
		if _, e = rand.Read(id[:]); e != nil {
			return scope, e
		}
		init.JournalID = hex.EncodeToString(id[:])
	}
	id := walletwithdraw.Identity{JournalID: init.JournalID, Pin: pin}
	binding, e := walletwithdraw.BindingID(id)
	if e != nil {
		return scope, e
	}
	expected := poolWithdrawalInit{"slug-pool-withdrawal-init", 1, withdrawalRecordHash(guard), id.JournalID, pin, binding}
	ready := poolWithdrawalReady{"slug-pool-withdrawal-ready", 1, withdrawalRecordHash(expected), id.JournalID, binding}
	if hasInit && withdrawalReadRecord(initPath, expected) != nil || hasReady && withdrawalReadRecord(readyPath, ready) != nil {
		return scope, ErrPoolRuntime
	}
	// Presence is checked before creating any journal. An established wallet
	// namespace cannot authorize reconstruction of a missing journal or INIT.
	seam, walletErr := w.wallet.OpenCandidateWithdrawals(ctx, binding)
	if walletErr != nil && !errors.Is(walletErr, wallet.ErrWithdrawalNotFound) {
		return scope, walletErr
	}
	if seam != nil {
		defer func() { resultErr = errors.Join(resultErr, seam.Close()) }()
	}
	if !hasInit && seam != nil || hasReady && (seam == nil || !hasJournal) {
		return scope, ErrPoolRuntime
	}
	if !hasInit {
		if e = writePoolRuntimeRecord(initPath, expected); e != nil {
			return scope, e
		}
	}
	var journal *walletwithdraw.Journal
	if hasReady || seam != nil {
		journal, e = walletwithdraw.Open(ctx, journalPath, id)
	} else {
		journal, e = walletwithdraw.Create(ctx, journalPath, id)
	}
	if e != nil {
		return scope, e
	}
	defer func() { resultErr = errors.Join(resultErr, journal.Close()) }()
	if !hasReady {
		owner, err := journal.TryAcquire(ctx)
		if err != nil {
			return scope, err
		}
		snapshot, err := owner.Snapshot()
		err = errors.Join(err, owner.Close())
		if err != nil || snapshot.Count != 0 {
			return scope, ErrPoolRuntime
		}
		if seam == nil {
			seam, e = w.wallet.CreateCandidateWithdrawals(ctx, binding)
			if e != nil {
				return scope, e
			}
			defer func() { resultErr = errors.Join(resultErr, seam.Close()) }()
		}
		empty, err := seam.Empty(ctx)
		if err != nil || !empty {
			return scope, ErrPoolRuntime
		}
		if ctx.Err() != nil {
			return scope, ctx.Err()
		}
		if e = writePoolRuntimeRecord(readyPath, ready); e != nil {
			return scope, e
		}
	}
	scope = poolWithdrawalScope{id, binding, journalPath, expected, ready, guard}
	return scope, nil
}
func (s poolWithdrawalScope) check() error {
	b, e := walletwithdraw.BindingID(s.Identity)
	if e != nil || b != s.BindingID || !filepath.IsAbs(s.journalPath) || filepath.Clean(s.journalPath) != s.journalPath {
		return ErrPoolRuntime
	}
	root := filepath.Dir(s.journalPath)
	shared := filepath.Dir(filepath.Dir(root))
	if filepath.Base(root) != s.Identity.Pin.AccountID || filepath.Base(filepath.Dir(root)) != poolWithdrawalDirectory || filepath.Base(s.journalPath) != "journal" || withdrawalScopeInventory(root) != nil || withdrawalReadRecord(filepath.Join(shared, poolWithdrawalGuardName), s.guard) != nil || withdrawalReadRecord(filepath.Join(root, "INIT.json"), s.init) != nil || withdrawalReadRecord(filepath.Join(root, "READY.json"), s.ready) != nil {
		return ErrPoolRuntime
	}
	return nil
}
