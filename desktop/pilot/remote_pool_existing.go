// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
)

// AuditExistingOwnedWalletSource corroborates independently supplied original
// source and recipient against closed local controls. It opens no chain, wallet,
// issuer or history store and makes no current-head/readiness authority claim.
// Caller holds the original owned-app root lock throughout this audit/open.
func AuditExistingOwnedWalletSource(sharedRoot, accountAuthorityID string, original OwnedSourceConfig, expected poolshare.Config, recipient PoolRecipientBinding) error {
	if !filepath.IsAbs(sharedRoot) || filepath.Clean(sharedRoot) != sharedRoot || !validAccountID(accountAuthorityID) || ValidateOwnedGymSource(original) != nil || ValidatePoolRecipientBinding(recipient) != nil || recipient.Config.AccountAuthorityID != accountAuthorityID || recipient.Config.Source != expected {
		return ErrRemotePoolChanged
	}
	if e := ownedWalletAncestors(sharedRoot); e != nil {
		return e
	}
	// Pre-bound directory enumeration before the inherited inventory checker.
	f, e := os.Open(sharedRoot)
	if e != nil {
		return ErrRemotePoolUnavailable
	}
	names, re := f.ReadDir(33)
	ce := remotePoolCloseAuditFile(f)
	if ce != nil {
		return ce
	}
	if re != nil && !errors.Is(re, io.EOF) || len(names) > 32 {
		return ErrRemotePoolChanged
	}
	if e = poolRuntimeRootFilesMode(sharedRoot, true); e != nil {
		return e
	}
	var marker poolRuntimeMarker
	if e = readManifestJSON(filepath.Join(sharedRoot, poolRuntimeMarkerName), poolRuntimeMarkerLimit, &marker); e != nil {
		return e
	}
	if !marker.validFor(accountAuthorityID, original.Ownership.Source.DescriptorID) || marker.source(original.Ownership.Source) != expected {
		return ErrRemotePoolChanged
	}
	id, e := poolservice.BindingID(expected)
	if e != nil || id != recipient.SourceBindingID {
		return ErrRemotePoolChanged
	}
	for _, record := range []struct {
		name  string
		value any
	}{{ownedSourceConfigName, original}, {poolRuntimeSourceName, expected}, {poolRuntimeCoreName, poolRuntimeCore{1, id}}} {
		if e = readPoolRuntimeRecord(filepath.Join(sharedRoot, record.name), record.value); e != nil {
			return e
		}
	}
	var ready poolRuntimeReady
	if e = readManifestJSON(filepath.Join(sharedRoot, poolRuntimeReadyName), poolRuntimeMarkerLimit, &ready); e != nil {
		return e
	}
	if ready.Version != 1 || ready.Binding != id || len(ready.Recipients) != 2 {
		return ErrRemotePoolChanged
	}
	matched := 0
	ids := map[string]string{}
	for i, b := range ready.Recipients {
		if ValidatePoolRecipientBinding(b) != nil || b.Config.Source != expected || b.Config.AccountAuthorityID != accountAuthorityID || i > 0 && ready.Recipients[i-1].Config.Identity.GymID >= b.Config.Identity.GymID || ids[b.Config.Fixture] != "" {
			return ErrRemotePoolChanged
		}
		ids[b.Config.Fixture] = b.Config.AccountID
		if b == recipient {
			matched++
		}
	}
	if matched != 1 || ids["zero256"] == "" || ids["incrementing256"] == "" || ids["zero256"] == ids["incrementing256"] {
		return ErrRemotePoolChanged
	}
	for _, name := range []string{"instance-lock", "issuer", "shares", "rounds", "settlement", "gyms", ownedSourceDirectory, "primary-chain", "peer-chain", ownedLeagueDirectory} {
		if e = plainChild(filepath.Join(sharedRoot, name), true); e != nil {
			return e
		}
	}
	for _, name := range []string{"zero256", "incrementing256"} {
		if e = plainChild(filepath.Join(sharedRoot, "gyms", name), true); e != nil {
			return e
		}
	}
	cfg, e := OwnedLeagueConfigFor(original, expected, accountAuthorityID, ids["zero256"], ids["incrementing256"])
	if e != nil {
		return e
	}
	b, e := loadOwnedLeague(filepath.Join(sharedRoot, ownedLeagueDirectory), cfg)
	if e != nil {
		return e
	}
	if b.ready == nil {
		return ErrRemotePoolChanged
	}
	return nil
}

// The constructor must retain this exact cleanup error and its original root
// owner. An ambiguous Close is sticky and must never be retried on a raw fd.
type remotePoolAuditCloseError struct{ file *os.File }

func (*remotePoolAuditCloseError) Error() string { return "remote pool audit cleanup incomplete" }
func (*remotePoolAuditCloseError) Unwrap() error { return ErrRemotePoolCleanup }
func remotePoolCloseAuditFile(file *os.File) error {
	if file.Close() != nil {
		return &remotePoolAuditCloseError{file: file}
	}
	return nil
}
