// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/rsa"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymprovider"
)

const hostedDriverID = "local-published-fixture-evidence-v1"
const hostedWrapperID = "public-fixture-wrapper-v1"
const hostedTimeMax int64 = 253402300799999
const hostedDriverMax = 4194304

var hostedFixtures = [...]string{"zero256", "incrementing256"}
var errHostedBusy = errors.New("hosted fixture operation busy")
var errHostedPending = errors.New("hosted fixture reset pending")

// HostedFixtureEvidence is private helper authority. No assertion or session
// material is accepted from or disclosed to the webview.
type HostedFixtureEvidence interface {
	ID() string
	FreshLogin(context.Context, gymprovider.LoginFlow, string) (string, error)
	BeginResetEvidence(context.Context, gymprovider.ResetRequest, string) (string, error)
	CommitReset(context.Context, gymprovider.ResetRequest, gymprovider.ResetResult) error
	CompleteResetEvidence(context.Context, gymprovider.ResetRequest) (string, error)
	PendingReset(context.Context, string) (*FixturePendingReset, error)
	ObserveResetComplete(context.Context, gymprovider.ResetRequest, gymprovider.ResetResult) error
	RetireUnbegunReset(context.Context, gymprovider.ResetRequest) error
	Close() error
}
type FixturePendingReset struct {
	Version      uint32                   `json:"version"`
	Fixture      string                   `json:"fixture"`
	AccountID    string                   `json:"account_id"`
	OperationID  string                   `json:"operation_id"`
	Request      gymprovider.ResetRequest `json:"request"`
	FromEpoch    uint64                   `json:"from_epoch"`
	ToEpoch      uint64                   `json:"to_epoch"`
	VerifiedAtMS int64                    `json:"verified_at_ms"`
	Phase        string                   `json:"phase"`
}
type hostedEpoch struct {
	Fixture         string `json:"fixture"`
	Subject         string `json:"subject"`
	CredentialEpoch uint64 `json:"credential_epoch"`
	PendingEventID  string `json:"pending_event_id"`
}
type hostedAuth struct {
	Version           uint32                `json:"version"`
	Sequence          uint32                `json:"sequence"`
	Fixture           string                `json:"fixture"`
	Subject           string                `json:"subject"`
	AuthID            string                `json:"auth_id"`
	Flow              gymprovider.LoginFlow `json:"flow"`
	CredentialEpoch   uint64                `json:"credential_epoch"`
	AuthenticatedAtMS int64                 `json:"authenticated_at_ms"`
}
type hostedReset struct {
	Version      uint32                   `json:"version"`
	Sequence     uint32                   `json:"sequence"`
	Fixture      string                   `json:"fixture"`
	Subject      string                   `json:"subject"`
	AccountID    string                   `json:"account_id"`
	OperationID  string                   `json:"operation_id"`
	Request      gymprovider.ResetRequest `json:"request"`
	FromEpoch    uint64                   `json:"from_epoch"`
	ToEpoch      uint64                   `json:"to_epoch"`
	VerifiedAtMS int64                    `json:"verified_at_ms"`
	Phase        string                   `json:"phase"`
	BeginAck     *gymprovider.ResetResult `json:"begin_ack"`
	CompleteAck  *gymprovider.ResetResult `json:"complete_ack"`
}
type hostedDriverState struct {
	Version          uint32             `json:"version"`
	DriverID         string             `json:"driver_id"`
	CoordinatorID    string             `json:"coordinator_id"`
	Config           gymprovider.Config `json:"config"`
	Revision         uint32             `json:"revision"`
	ClockHighWaterMS int64              `json:"clock_high_water_ms"`
	Epochs           []hostedEpoch      `json:"epochs"`
	Auth             []hostedAuth       `json:"auth"`
	Resets           []hostedReset      `json:"resets"`
}

// hostedFixtureDriver owns one bounded atomic state file. Its permit is always
// acquired outside account leases. Close waits for an admitted operation.
type hostedFixtureDriver struct {
	mu               sync.Mutex
	root             *os.Root
	directory        *os.File
	lock             *leveldb.DB
	path             string
	state            hostedDriverState
	accounts         *gymaccount.Store
	coordinator      *gymprovider.Coordinator
	key              *rsa.PrivateKey
	random           io.Reader
	now              func() time.Time
	closed, poisoned bool
	closeErr         error
	// Test seam operates on the exact already bounded canonical state bytes.
	publish func([]byte) error
}
