// Package pilot owns a disposable, loopback-only mining environment and a
// published recovery-wallet fixture. It never handles production wallet keys.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/fighter"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
)

const Mode = "local-wallet-pilot"
const AccountMode = "local-account-pilot"
const MaxRequestSize = 4096
const MaxResponseSize = 256 << 10

type Config struct {
	DataDir    string
	NodeBinary string
}

// AccountConfig is private helper wiring for the explicit two-fixture mode.
// Credentials and callbacks are never serialized or exposed to the webview.
type AccountConfig struct {
	Fixture         string
	AccountID       string
	AuthorityID     string
	RequireExisting bool
	Authorizer      prospectservice.SessionAuthorizer
	MiningToken     func(context.Context) (string, error)
	// AdmissionSigner is optional private helper wiring. It receives only a
	// scoped mining credential and typed admission; it must honor cancellation.
	// A configured signer failure never falls back to the local wallet.
	AdmissionSigner func(context.Context, string, prospect.Context, prospect.Proof) (gymauth.Authorization, error)
}
type Prospect struct {
	Sequence     uint64             `json:"sequence"`
	ID           string             `json:"id"`
	WorkHash     string             `json:"work_hash"`
	BornAtMS     int64              `json:"born_at_ms"`
	Rarity       string             `json:"rarity"`
	TalentBudget int                `json:"talent_budget"`
	Attributes   fighter.Attributes `json:"attributes"`
}
type Snapshot struct {
	Account         *AccountSnapshot `json:"account,omitempty"`
	State           string           `json:"state"`
	Mode            string           `json:"mode"`
	Attempts        uint64           `json:"attempts"`
	HashesPerSecond float64          `json:"hashes_per_second"`
	Height          uint64           `json:"height"`
	Pending         uint64           `json:"pending"`
	LastError       string           `json:"last_error"`
	RecoveryError   string           `json:"recovery_error"`
	ProspectCount   uint64           `json:"prospect_count"`
	Prospects       []Prospect       `json:"prospects"`
	WalletState     string           `json:"wallet_state"`
	WalletHeight    uint64           `json:"wallet_height"`
	PayoutAddress   string           `json:"payout_address"`
	GymID           string           `json:"gym_id"`
	GymPublicKey    string           `json:"gym_public_key"`
	SpendableAtoms  int64            `json:"spendable_atoms"`
	ImmatureAtoms   int64            `json:"immature_atoms"`
	BackupState     string           `json:"backup_state"`
	BackupPhase     string           `json:"backup_phase"`
	BackupError     string           `json:"backup_error"`
}

// AccountSnapshot contains public state only. It is not a session credential.
type AccountSnapshot struct {
	State       string `json:"state"`
	Fixture     string `json:"fixture"`
	ID          string `json:"id"`
	Generation  uint64 `json:"generation"`
	HoldUntilMS int64  `json:"hold_until_ms"`
	Held        bool   `json:"held"`
	CanExport   bool   `json:"can_export"`
	LastError   string `json:"last_error"`
}
