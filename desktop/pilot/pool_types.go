// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import "github.com/taytr-llc/slug/poolservice"

const PoolMode = "local-pool-pilot"

type PoolBalanceSnapshot struct {
	State string                       `json:"state"`
	Error string                       `json:"error"`
	Value *poolservice.BalanceResponse `json:"value"`
}

// PoolSnapshot is separate from the solo projection. AssignmentCount is local
// to the signed-in gym; it is never the shared registry's global cursor.
type PoolSnapshot struct {
	Version         uint32              `json:"version"`
	Mode            string              `json:"mode"`
	State           string              `json:"state"`
	Attempts        uint64              `json:"attempts"`
	HashesPerSecond float64             `json:"hashes_per_second"`
	Height          uint64              `json:"height"`
	Pending         uint64              `json:"pending"`
	LastError       string              `json:"last_error"`
	RecoveryError   string              `json:"recovery_error"`
	WalletState     string              `json:"wallet_state"`
	WalletHeight    uint64              `json:"wallet_height"`
	PayoutAddress   string              `json:"payout_address"`
	GymID           string              `json:"gym_id"`
	GymPublicKey    string              `json:"gym_public_key"`
	SpendableAtoms  int64               `json:"spendable_atoms"`
	ImmatureAtoms   int64               `json:"immature_atoms"`
	BackupState     string              `json:"backup_state"`
	BackupPhase     string              `json:"backup_phase"`
	BackupError     string              `json:"backup_error"`
	Account         AccountSnapshot     `json:"account"`
	View            *ManifestViewPin    `json:"view"`
	AssignmentCount uint64              `json:"assignment_count"`
	SourceBindingID string              `json:"source_binding_id"`
	Balance         PoolBalanceSnapshot `json:"balance"`
}

func BlankPoolSnapshot() PoolSnapshot {
	return PoolSnapshot{Version: 1, Mode: PoolMode, State: "stopped", WalletState: "closed", BackupState: "unavailable", Account: AccountSnapshot{State: "signed_out"}, Balance: PoolBalanceSnapshot{State: "unavailable"}}
}
