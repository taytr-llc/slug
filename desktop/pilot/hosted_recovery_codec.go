// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
)

func HostedRecoveryErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "unauthorized", "forbidden", "stale_view", "busy", "cancelled", "outcome_unknown", "hosted_recovery_unavailable", "hosted_reset_pending", "allocation_capacity", "allocation_conflict", "allocation_not_found", "allocation_resume_required", "allocation_unavailable", "wallet_missing", "backup_invalid", "backup_mismatch", "backup_unavailable", "backup_storage_failed", "backup_export_uncertain")
}

// HostedAllocationBinding reconstructs the immutable public preimage. It uses
// the pinned fixture identity and compiled zero inventory, never reply cursors.
func HostedAllocationBinding(s HostedAllocationScope) (wallet.AllocationBinding, error) {
	if s.Version != 1 || !payoutNonzeroID(s.InventoryID) || !payoutNonzeroID(s.InventoryBindingID) {
		return wallet.AllocationBinding{}, gymstable.ErrInvalid
	}
	c := s.CurrentBinding
	v := c.VaultBinding
	if _, err := recoveryvault.CurrentBindingID(c); err != nil {
		return wallet.AllocationBinding{}, gymstable.ErrInvalid
	}
	if gymaccount.ValidateFixtureIdentity(v.RootPublicKey, v.GymPublicKey, v.GymID) != nil {
		return wallet.AllocationBinding{}, gymstable.ErrInvalid
	}
	m := hostedInitialManifest(v.RootPublicKey, v.GymPublicKey, v.GymID)
	b := wallet.AllocationBinding{Version: 1, InventoryID: s.InventoryID, AuthorityID: v.AuthorityID, AccountID: v.AccountID, PrincipalID: v.PrincipalID, ProvisionID: v.ProvisionID, HostedVaultID: c.HostedVaultID, RetentionID: c.RetentionID, WalletOwnerID: c.WalletOwnerID, SourceBindingID: c.SourceBindingID, ManagerSchema: 13, MaxAllocations: 1024, MaxBytes: 4194304, MaxPlanAttempts: 128, MaxBranchIndex: 4096, InitialManifest: m}
	id, err := wallet.AllocationBindingID(b)
	if err != nil || id != s.InventoryBindingID {
		return wallet.AllocationBinding{}, gymstable.ErrInvalid
	}
	return b, nil
}
func ValidateHostedAllocationScope(s HostedAllocationScope) error {
	_, e := HostedAllocationBinding(s)
	return e
}
func HostedAllocationScopeEqual(a, b HostedAllocationScope) bool { return a == b }
func ValidateHostedRecoveryView(v HostedRecoveryView) error {
	i, e := gymaccount.FixtureIdentity(v.Fixture)
	if e != nil || v.Version != 1 || !payoutNonzeroID(v.ViewID) || !payoutNonzeroID(v.AccountAuthorityID) || !payoutNonzeroID(v.AccountID) || v.Generation == 0 || v.Generation > gymstable.MaxSafeInteger || v.RootPublicKey != i.RootPublicKey || v.GymPublicKey != i.GymPublicKey || v.GymID != i.GymID {
		return gymstable.ErrInvalid
	}
	return nil
}
func hostedScopeIdentity(s HostedAllocationScope, a AccountSnapshot, authority, source string) bool {
	v := s.CurrentBinding.VaultBinding
	i, e := gymaccount.FixtureIdentity(a.Fixture)
	return e == nil && ValidateHostedAllocationScope(s) == nil && v.AuthorityID == authority && v.AccountID == a.ID && v.RootPublicKey == i.RootPublicKey && v.GymPublicKey == i.GymPublicKey && v.GymID == i.GymID && (source == "" || s.CurrentBinding.SourceBindingID == source)
}
func ValidateHostedRecoverySnapshot(s HostedRecoverySnapshot) error {
	bad := gymstable.ErrInvalid
	if s.Version != 1 || s.Mode != HostedRecoveryPoolMode {
		return bad
	}
	a := s.Account
	if s.Kind == "inactive" {
		if s.Healthy != nil || s.Recovery != nil || a.State == "signed_in" || !stableOne(s.Error, "", "hosted_recovery_unavailable", "hosted_reset_pending") {
			return bad
		}
		blank := BlankPoolSnapshot()
		blank.Mode = HostedRecoveryPoolMode
		blank.Account = a
		if ValidatePoolSnapshot(blank) != nil {
			return bad
		}
		if s.Error == "hosted_recovery_unavailable" && (a.State != "failed" || a.LastError != "account_unavailable" || a.Fixture == "") {
			return bad
		}
		if (s.Error == "hosted_reset_pending") != (a.LastError == "hosted_reset_pending") {
			return bad
		}
		return nil
	}
	if a.State != "signed_in" || s.Error != "" {
		return bad
	}
	if s.Kind == "healthy" {
		h := s.Healthy
		if h == nil || s.Recovery != nil || h.Pool.Mode != s.Mode || h.Pool.Account != a || ValidatePoolSnapshot(h.Pool) != nil {
			return bad
		}
		if !hostedScopeIdentity(h.Allocation, a, h.Pool.View.AccountAuthorityID, h.Pool.SourceBindingID) {
			return bad
		}
		for _, id := range []string{h.PendingReceiveID, h.IssuedReceiveID} {
			if id != "" && !payoutNonzeroID(id) {
				return bad
			}
		}
		if h.PendingReceiveID != "" && h.PendingReceiveID == h.IssuedReceiveID {
			return bad
		}
		return nil
	}
	if s.Kind != "recovery_only" || s.Recovery == nil || s.Healthy != nil {
		return bad
	}
	r := s.Recovery
	v := r.View
	if ValidateHostedRecoveryView(v) != nil || v.AccountID != a.ID || v.Generation != a.Generation || v.Fixture != a.Fixture || !hostedScopeIdentity(r.Allocation, a, v.AccountAuthorityID, "") || r.LocalWallet != "missing" || r.InventoryRevision == 0 || !payoutTime(r.ObservedAtMS, false) {
		return bad
	}
	// Reuse the inherited account and backup grammar without inventing any
	// wallet/amount fields in the actual recovery-only payload.
	if a.LastError != "" || a.HoldUntilMS < 0 || a.HoldUntilMS > 8640000000000000 || a.Held && (a.CanExport || a.HoldUntilMS == 0) {
		return bad
	}
	if r.Backup.State == "unavailable" || !poolBackupValid(PoolSnapshot{State: "stopped", BackupState: r.Backup.State, BackupPhase: r.Backup.Phase, BackupError: r.Backup.Error}) {
		return bad
	}
	return nil
}
func ValidateHostedReceiveJob(j HostedReceiveJob) error {
	bad := gymstable.ErrInvalid
	if j.Version != 1 || !payoutNonzeroID(j.ActionID) || !payoutNonzeroID(j.OperationID) || !canonicalManifestID(j.ViewID) || ValidateHostedAllocationScope(j.Allocation) != nil {
		return bad
	}
	if j.State != "ready" {
		if j.Address != nil {
			return bad
		}
		switch j.State {
		case "running":
			if j.Error == "" {
				return nil
			}
		case "needs_resume":
			if j.Error == "allocation_resume_required" {
				return nil
			}
		case "cancelled":
			if j.Error == "cancelled" {
				return nil
			}
		case "unknown":
			if j.Error == "outcome_unknown" {
				return nil
			}
		case "failed":
			if HostedRecoveryErrorCode(j.Error) {
				return nil
			}
		}
		return bad
	}
	a := j.Address
	if a == nil || j.Error != "" || a.Purpose != 84 || a.Coin != 0 || a.Account != 0 || a.Branch != 0 || a.Index < 1 || a.Index > 4095 || a.InventoryRevision == 0 || len(a.Script) != 44 || !activityAddress(a.Address) || a.Address == "" {
		return bad
	}
	d, e := address.DecodeAddress(a.Address, &chaincfg.MainNetParams)
	if e != nil {
		return bad
	}
	if _, ok := d.(*address.AddressWitnessPubKeyHash); !ok {
		return bad
	}
	script, e := txscript.PayToAddrScript(d)
	if e != nil || hex.EncodeToString(script) != a.Script {
		return bad
	}
	return nil
}

func hostedRequestValid(r HostedRecoveryRequest) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger || r.Version != 1 {
		return false
	}
	base := HostedRecoveryRequest{ID: r.ID, Version: r.Version, Command: r.Command}
	switch strings.TrimPrefix(r.Command, "hosted_recovery_") {
	case "snapshot", "logout", "reset_fixture":
	case "login_fixture":
		if !stableOne(r.Fixture, "zero256", "incrementing256") {
			return false
		}
		base.Fixture = r.Fixture
	case "start", "stop":
		if !canonicalManifestID(r.ViewID) {
			return false
		}
		base.ViewID = r.ViewID
	case "receive_begin", "receive_lookup", "receive_resume", "receive_redisplay", "receive_cancel":
		if !canonicalManifestID(r.ViewID) || !payoutNonzeroID(r.ActionID) || !payoutNonzeroID(r.OperationID) {
			return false
		}
		base.ViewID = r.ViewID
		base.ActionID = r.ActionID
		base.OperationID = r.OperationID
	case "export_recovery", "verify_backup", "cancel_pool_backup":
		if r.OwnerKind != HostedHealthyOwner && r.OwnerKind != HostedMissingOwner {
			return false
		}
		if !canonicalManifestID(r.ViewID) || r.OwnerKind == HostedMissingOwner && !payoutNonzeroID(r.ViewID) {
			return false
		}
		base.OwnerKind = r.OwnerKind
		base.ViewID = r.ViewID
		if r.Command != "hosted_recovery_cancel_pool_backup" {
			mode := recoveryfile.ReadMode
			if r.Command == "hosted_recovery_export_recovery" {
				mode = recoveryfile.CreateMode
			}
			if r.Selection == nil || recoveryfile.ValidatePin(*r.Selection, mode) != nil {
				return false
			}
			base.Selection = r.Selection
		}
	default:
		return false
	}
	return strings.HasPrefix(r.Command, "hosted_recovery_") && reflect.DeepEqual(base, r)
}
func hostedResponseValid(r HostedRecoveryResponse) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger || r.Version != 1 {
		return false
	}
	if !r.OK {
		return r.Snapshot == nil && r.Receive == nil && HostedRecoveryErrorCode(r.Error)
	}
	if r.Error != "" {
		return false
	}
	if r.Snapshot != nil {
		return r.Receive == nil && ValidateHostedRecoverySnapshot(*r.Snapshot) == nil
	}
	return r.Receive != nil && ValidateHostedReceiveJob(*r.Receive) == nil
}
func hostedDecode(raw []byte, limit int, out any) error {
	if !activityFrameLimit(raw, limit) {
		return gymstable.ErrInvalid
	}
	raw = bytes.TrimSuffix(raw, []byte{'\n'})
	if _, e := poolDecode(raw, limit-1, 16, out); e != nil {
		return e
	}
	b, e := json.Marshal(out)
	if e != nil || !bytes.Equal(raw, b) {
		return gymstable.ErrInvalid
	}
	return nil
}
func ParseHostedRecoveryRequest(raw []byte) (HostedRecoveryRequest, error) {
	var r HostedRecoveryRequest
	if hostedDecode(raw, MaxHostedRecoveryRequestSize, &r) != nil || !hostedRequestValid(r) {
		return HostedRecoveryRequest{}, gymstable.ErrInvalid
	}
	return r, nil
}
func MarshalHostedRecoveryRequest(r HostedRecoveryRequest) ([]byte, error) {
	if !hostedRequestValid(r) {
		return nil, gymstable.ErrInvalid
	}
	b, e := json.Marshal(r)
	if e != nil || len(b) >= MaxHostedRecoveryRequestSize {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}
func ParseHostedRecoveryResponse(raw []byte) (HostedRecoveryResponse, error) {
	var r HostedRecoveryResponse
	if hostedDecode(raw, MaxHostedRecoveryResponseSize, &r) != nil || !hostedResponseValid(r) || r.Receive != nil && !activityFrameLimit(raw, MaxHostedReceiveResponseSize) {
		return HostedRecoveryResponse{}, gymstable.ErrInvalid
	}
	return r, nil
}
func MarshalHostedRecoveryResponse(r HostedRecoveryResponse) ([]byte, error) {
	if !hostedResponseValid(r) {
		return nil, gymstable.ErrInvalid
	}
	limit := MaxHostedRecoveryResponseSize
	if r.Receive != nil {
		limit = MaxHostedReceiveResponseSize
	}
	b, e := json.Marshal(r)
	if e != nil || len(b) >= limit {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}

func hostedInitialManifest(root, key, id string) recovery.Manifest {
	return recovery.Manifest{Version: 1, Profile: recovery.ProfileID, Network: recovery.Network, Genesis: recovery.Genesis, Derivation: recovery.DerivationStandard, RootPublicKey: root, GymPath: recovery.GymPath, GymPublicKey: key, GymID: id, Accounts: recovery.DefaultAccounts()}
}
