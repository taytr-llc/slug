// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

const (
	HostedRecoveryPoolMode        = "local-hosted-recovery-pool"
	MaxHostedRecoveryRequestSize  = 4096
	MaxHostedRecoveryResponseSize = 262144
	MaxHostedReceiveResponseSize  = 16384
)

type HostedOwnerKind string

const (
	HostedHealthyOwner HostedOwnerKind = "healthy"
	HostedMissingOwner HostedOwnerKind = "recovery_only"
)

// HostedRecoverySurfaceAPI adds the third mode without changing the legacy
// controller interface or turning a missing wallet into a PoolChild.
type HostedRecoverySurfaceAPI interface {
	HostedRecoverySnapshot() HostedRecoverySnapshot
	LoginFixture(string) error
	Logout() error
	ResetFixtureAccount() error
	StartHostedRecovery(context.Context, string) error
	StopHostedRecovery(context.Context, string) error
	BeginHostedReceive(context.Context, string, string, string) (HostedReceiveJob, error)
	LookupHostedReceive(context.Context, string, string, string) (HostedReceiveJob, error)
	ResumeHostedReceive(context.Context, string, string, string) (HostedReceiveJob, error)
	RedisplayHostedReceive(context.Context, string, string, string) (HostedReceiveJob, error)
	CancelHostedReceive(context.Context, string, string, string) (HostedReceiveJob, error)
	ExportHostedRecovery(context.Context, HostedOwnerKind, string, recoveryfile.Pin) error
	VerifyHostedBackup(context.Context, HostedOwnerKind, string, recoveryfile.Pin) error
	CancelHostedBackup(context.Context, HostedOwnerKind, string) error
}

type HostedRecoverySnapshot struct {
	Version  uint32                       `json:"version"`
	Mode     string                       `json:"mode"`
	Kind     string                       `json:"kind"`
	Account  AccountSnapshot              `json:"account"`
	Healthy  *HostedHealthySnapshot       `json:"healthy"`
	Recovery *HostedMissingWalletSnapshot `json:"recovery"`
	Error    string                       `json:"error"`
}
type HostedHealthySnapshot struct {
	Pool             PoolSnapshot          `json:"pool"`
	Allocation       HostedAllocationScope `json:"allocation"`
	PendingReceiveID string                `json:"pending_receive_id"`
	IssuedReceiveID  string                `json:"issued_receive_id"`
}
type HostedMissingWalletSnapshot struct {
	View              HostedRecoveryView    `json:"view"`
	Allocation        HostedAllocationScope `json:"allocation"`
	LocalWallet       string                `json:"local_wallet"`
	InventoryRevision uint32                `json:"inventory_revision"`
	ObservedAtMS      int64                 `json:"observed_at_ms"`
	Backup            HostedBackupSnapshot  `json:"backup"`
}
type HostedRecoveryView struct {
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
type HostedAllocationScope struct {
	Version            uint32                       `json:"version"`
	InventoryID        string                       `json:"inventory_id"`
	InventoryBindingID string                       `json:"inventory_binding_id"`
	CurrentBinding     recoveryvault.CurrentBinding `json:"current_binding"`
}
type HostedBackupSnapshot struct {
	State string `json:"state"`
	Phase string `json:"phase"`
	Error string `json:"error"`
}
type HostedReceiveJob struct {
	Version     uint32                `json:"version"`
	ActionID    string                `json:"action_id"`
	OperationID string                `json:"operation_id"`
	ViewID      string                `json:"view_id"`
	Allocation  HostedAllocationScope `json:"allocation"`
	State       string                `json:"state"`
	Error       string                `json:"error"`
	Address     *HostedReceiveAddress `json:"address"`
}
type HostedReceiveAddress struct {
	Address           string `json:"address"`
	Script            string `json:"script"`
	Purpose           uint32 `json:"purpose"`
	Coin              uint32 `json:"coin"`
	Account           uint32 `json:"account"`
	Branch            uint32 `json:"branch"`
	Index             uint32 `json:"index"`
	InventoryRevision uint32 `json:"inventory_revision"`
}
type HostedRecoveryRequest struct {
	ID          uint64            `json:"id"`
	Version     uint32            `json:"version"`
	Command     string            `json:"command"`
	Fixture     string            `json:"fixture,omitempty"`
	OwnerKind   HostedOwnerKind   `json:"owner_kind,omitempty"`
	ViewID      string            `json:"view_id,omitempty"`
	ActionID    string            `json:"action_id,omitempty"`
	OperationID string            `json:"operation_id,omitempty"`
	Selection   *recoveryfile.Pin `json:"selection,omitempty"`
}
type HostedRecoveryResponse struct {
	ID       uint64                  `json:"id"`
	Version  uint32                  `json:"version"`
	OK       bool                    `json:"ok"`
	Snapshot *HostedRecoverySnapshot `json:"snapshot"`
	Receive  *HostedReceiveJob       `json:"receive"`
	Error    string                  `json:"error"`
}

func BlankHostedRecoverySnapshot() HostedRecoverySnapshot {
	return HostedRecoverySnapshot{Version: 1, Mode: HostedRecoveryPoolMode, Kind: "inactive", Account: AccountSnapshot{State: "signed_out"}}
}
