// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
)

const MaxRecoverableSetupBytes = MaxSetupBytes + 1024

// RecoverableDailySetup explicitly selects fresh request-recoverable owners.
// Base retains every original DAILY financial, wallet and authority identity.
type RecoverableDailySetup struct {
	Version      uint32                `json:"version"`
	Base         Setup                 `json:"base"`
	IssueProfile registry.IssueProfile `json:"issue_profile"`
}

func ComposeRecoverableDailySetup(base Setup) (RecoverableDailySetup, error) {
	id, e := DailySetupID(base)
	if e != nil {
		return RecoverableDailySetup{}, e
	}
	source, e := poolshare.DailyConfigID(base.Pool)
	if e != nil {
		return RecoverableDailySetup{}, e
	}
	s := RecoverableDailySetup{Version: 1, Base: base, IssueProfile: registry.IssueProfile{
		Version: 1, BaseSetupID: id, SourceConfigID: source, MaxRequests: base.Pool.MaxWorks,
	}}
	return s, ValidateRecoverableDailySetup(s)
}

func ValidateRecoverableDailySetup(s RecoverableDailySetup) error {
	if s.Version != 1 || ValidateDailySetup(s.Base) != nil {
		return ErrInvalid
	}
	baseID, e := DailySetupID(s.Base)
	if e != nil {
		return ErrInvalid
	}
	sourceID, e := poolshare.DailyConfigID(s.Base.Pool)
	if e != nil || s.IssueProfile != (registry.IssueProfile{
		Version: 1, BaseSetupID: baseID, SourceConfigID: sourceID, MaxRequests: s.Base.Pool.MaxWorks,
	}) {
		return ErrInvalid
	}
	profile, e := registry.NewOperatorProfile(s.Base.Authority)
	if e != nil || registry.ValidateRecoverableOperatorProfile(profile, s.IssueProfile) != nil {
		return ErrInvalid
	}
	if _, e = DailyCapacityEnvelope(s.Base); e != nil {
		return e
	}
	return nil
}

func MarshalRecoverableDailySetup(s RecoverableDailySetup) ([]byte, error) {
	if e := ValidateRecoverableDailySetup(s); e != nil {
		return nil, e
	}
	// The outer allowance never enlarges the inherited whole-Setup bound.
	if _, e := MarshalDailySetup(s.Base); e != nil {
		return nil, e
	}
	raw, e := json.Marshal(s)
	if e != nil || len(raw) > MaxRecoverableSetupBytes {
		return nil, ErrInvalid
	}
	return raw, nil
}

func ParseRecoverableDailySetup(raw []byte) (RecoverableDailySetup, error) {
	var s RecoverableDailySetup
	if len(raw) == 0 || len(raw) > MaxRecoverableSetupBytes {
		return s, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&s) != nil {
		return RecoverableDailySetup{}, ErrInvalid
	}
	canonical, e := MarshalRecoverableDailySetup(s)
	if e != nil || !bytes.Equal(raw, canonical) {
		return RecoverableDailySetup{}, ErrInvalid
	}
	return s, nil
}

func RecoverableDailySetupID(s RecoverableDailySetup) (string, error) {
	raw, e := MarshalRecoverableDailySetup(s)
	if e != nil {
		return "", e
	}
	return framedHash("SLUG pool issue recovery setup", raw), nil
}

type recoverableReadyRecord struct {
	Version        uint32 `json:"version"`
	RuntimeID      string `json:"runtime_id"`
	SetupID        string `json:"setup_id"`
	IssueProfileID string `json:"issue_profile_id"`
}

func recoverableReadyBytes(s RecoverableDailySetup) ([]byte, error) {
	id, e := RecoverableDailySetupID(s)
	if e != nil {
		return nil, e
	}
	pid, e := registry.IssueProfileID(s.IssueProfile)
	if e != nil {
		return nil, e
	}
	raw, e := json.Marshal(recoverableReadyRecord{1, s.Base.RuntimeID, id, pid})
	return append(raw, '\n'), e
}

func CreateRecoverableDaily(ctx context.Context, root string, expected RecoverableDailySetup, borrowed BorrowedOwners, private PrivateInputs) (*Runtime, error) {
	return constructRecoverableDaily(ctx, root, expected, borrowed, private, true)
}

func OpenRecoverableDaily(ctx context.Context, root string, expected RecoverableDailySetup, borrowed BorrowedOwners, private PrivateInputs) (*Runtime, error) {
	return constructRecoverableDaily(ctx, root, expected, borrowed, private, false)
}

func constructRecoverableDaily(ctx context.Context, root string, expected RecoverableDailySetup, borrowed BorrowedOwners, private PrivateInputs, create bool) (*Runtime, error) {
	raw, e := MarshalRecoverableDailySetup(expected)
	if e != nil {
		return nil, e
	}
	owned, e := ParseRecoverableDailySetup(raw)
	if e != nil {
		return nil, e
	}
	return constructSelection(ctx, root, owned.Base, borrowed, private, create, true, &owned)
}

// These wrappers retain the complete old objects, including their old IDs.
type recoverableProvisionIntent struct {
	Version              uint32          `json:"version"`
	Base                 provisionIntent `json:"base"`
	IssueRecoveryVersion uint32          `json:"issue_recovery_version"`
}
type recoverableProvisionReady struct {
	Version        uint32         `json:"version"`
	Base           provisionReady `json:"base"`
	SetupID        string         `json:"setup_id"`
	IssueProfileID string         `json:"issue_profile_id"`
}
type recoverableProvisionBackup struct {
	Version        uint32          `json:"version"`
	Base           provisionBackup `json:"base"`
	SetupID        string          `json:"setup_id"`
	IssueProfileID string          `json:"issue_profile_id"`
}

func (recoverableProvisionBackup) String() string {
	return "protected recoverable operator wallet backup"
}
func (recoverableProvisionBackup) GoString() string {
	return "protected recoverable operator wallet backup"
}

func issueProvisionIntentBytes(v provisionIntent) ([]byte, error) {
	if v.Version != 2 {
		return nil, ErrProvisionInvalid
	}
	if _, e := provisionJSON(v, 131072); e != nil {
		return nil, e
	}
	return provisionJSON(recoverableProvisionIntent{1, v, 1}, 131072+1024)
}

func parseIssueProvisionIntent(raw []byte, p ProvisionPolicy, c ProvisionCertificate) (provisionIntent, error) {
	var v recoverableProvisionIntent
	if provisionJSONFile(raw, 131072+1024, &v) != nil || v.Version != 1 || v.IssueRecoveryVersion != 1 || v.Base.Version != 2 {
		return provisionIntent{}, ErrProvisionInvalid
	}
	base, e := provisionJSON(v.Base, 131072)
	if e != nil {
		return provisionIntent{}, e
	}
	return parseProvisionIntent(base, p, c)
}

func issueProvisionReadyBytes(v provisionReady, s RecoverableDailySetup) ([]byte, error) {
	baseID, e := DailySetupID(s.Base)
	if e != nil || v.Version != 2 || v.SetupID != baseID {
		return nil, ErrProvisionInvalid
	}
	id, e := RecoverableDailySetupID(s)
	if e != nil {
		return nil, e
	}
	pid, e := registry.IssueProfileID(s.IssueProfile)
	if e != nil {
		return nil, e
	}
	return provisionJSON(recoverableProvisionReady{1, v, id, pid}, 1024)
}

func issueProvisionBackupBytes(raw []byte, s RecoverableDailySetup) ([]byte, error) {
	var v provisionBackup
	if provisionJSONFile(raw, 8192, &v) != nil {
		return nil, ErrProvisionInvalid
	}
	id, e := RecoverableDailySetupID(s)
	if e != nil {
		return nil, e
	}
	pid, e := registry.IssueProfileID(s.IssueProfile)
	if e != nil {
		return nil, e
	}
	return provisionJSON(recoverableProvisionBackup{1, v, id, pid}, 8192+1024)
}
