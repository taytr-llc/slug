// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

func remotePoolPath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsAny(path, "\x00\r\n\t")
}
func remotePoolContains(parent, child string) bool {
	rel, e := filepath.Rel(parent, child)
	return e == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// No directory is created here. The separate attachment root cannot contain or
// be contained by the original account root, including through a symlink.
func remotePoolRoots(root, attachment string) error {
	if !remotePoolPath(root) || !remotePoolPath(attachment) || remotePoolContains(root, attachment) || remotePoolContains(attachment, root) {
		return pilot.ErrRemotePoolConfig
	}
	for _, selected := range []string{root, filepath.Dir(attachment)} {
		actual, e := filepath.EvalSymlinks(selected)
		if e != nil || actual != selected {
			return errors.Join(pilot.ErrRemotePoolConfig, e)
		}
	}
	if i, e := os.Lstat(attachment); e == nil {
		if !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
			return pilot.ErrRemotePoolConfig
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	return nil
}

// OpenHosted can finish a marker for an empty initializer. This established-
// root route refuses that recovery branch before the library opens its DB.
// The library still validates the marker's complete exact public bytes.
func requireRemoteHostedVaultMarker(root string) error {
	path := filepath.Join(root, "hosted-vault", "SLUG-RECOVERY-VAULT.json")
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 2 || info.Size() > 32768 {
		return errors.Join(pilot.ErrManifestInitialization, e)
	}
	return nil
}

func (c *RemotePoolController) auditRootIdentity() error {
	for _, item := range []struct {
		path     string
		original os.FileInfo
	}{{c.root, c.rootInfo}, {filepath.Join(c.root, "instance-lock"), c.lockInfo}} {
		now, e := os.Lstat(item.path)
		if e != nil || item.original == nil || !now.IsDir() || now.Mode()&os.ModeSymlink != 0 || !os.SameFile(item.original, now) {
			return errors.Join(pilot.ErrManifestInitialization, e)
		}
	}
	return nil
}

func (c *RemotePoolController) auditControls() error {
	if e := c.auditRootIdentity(); e != nil {
		return e
	}
	if e := requireRemoteHostedVaultMarker(c.root); e != nil {
		return e
	}
	var initial ownedGymInit
	var ready ownedGymReady
	found, e := readHostedControl(filepath.Join(c.root, ownedInitName), ownedControlLimit, &initial)
	if e != nil || !found || !validOwnedInit(initial, c.options) {
		return errors.Join(pilot.ErrManifestInitialization, e)
	}
	found, e = readHostedControl(filepath.Join(c.root, ownedReadyName), ownedControlLimit, &ready)
	if e != nil || !found || ready.Version != 1 || ready.InitSHA256 != hostedCanonicalHash(initial) {
		return errors.Join(pilot.ErrManifestInitialization, e)
	}
	prefix, e := readOwnedPrefix(c.root, initial)
	if e != nil || len(prefix) != len(ownedStages) {
		return errors.Join(pilot.ErrManifestInitialization, e)
	}
	local := c.inputs.Config.ExpectedLocal
	source := local.Recipient.Config.Source
	sourceID, e := poolservice.BindingID(source)
	if e != nil || initial.Hosted.AuthorityID != local.Recipient.Config.AccountAuthorityID || initial.Source.Ownership.Source != source.AuthorityPin || sourceID != local.OriginalSourceBindingID || ready.SourceBindingID != sourceID {
		return errors.Join(pilot.ErrManifestInitialization, e)
	}
	for i, p := range initial.Wallets {
		binding := ready.Fixtures[i]
		identity, e := gymaccount.FixtureIdentity(p.Fixture)
		_, be := recoveryvault.CurrentBindingID(binding)
		if e != nil || be != nil || binding.HostedVaultID != initial.Hosted.HostedVaultID || binding.WalletOwnerID != p.WalletOwnerID || binding.SourceBindingID != sourceID || binding.VaultBinding.AuthorityID != initial.Hosted.AuthorityID || binding.VaultBinding.RootPublicKey != identity.RootPublicKey || binding.VaultBinding.GymPublicKey != identity.GymPublicKey || binding.VaultBinding.GymID != identity.GymID {
			return errors.Join(pilot.ErrManifestInitialization, e, be)
		}
		if p.Fixture == local.Recipient.Config.Fixture && (p.WalletOwnerID != local.WalletOwnerID || binding.VaultBinding.AccountID != local.Recipient.Config.AccountID) {
			return pilot.ErrManifestInitialization
		}
		// The old whole-root namespace is retained even though this controller opens
		// only one wallet session and no stable/event/result histories.
		if found, e := hostedComponentExists(filepath.Join(c.root, "shared", "gyms", p.Fixture)); e != nil || !found {
			return errors.Join(pilot.ErrManifestInitialization, e)
		}
	}
	league, e := pilot.OwnedLeagueConfigFor(initial.Source, source, initial.Hosted.AuthorityID, ready.Fixtures[0].VaultBinding.AccountID, ready.Fixtures[1].VaultBinding.AccountID)
	if e != nil || ready.LeagueSHA256 != hostedCanonicalHash(league) {
		return errors.Join(pilot.ErrManifestInitialization, e)
	}
	hashes := []string{
		hostedInitialStageHash(initial.Hosted, "authority"), hostedInitialStageHash(initial.Hosted, "coordinator"),
		hostedInitialStageHash(initial.Hosted, "driver"), hostedInitialStageHash(initial.Hosted, "vault"),
		hostedCanonicalHash(struct {
			Source  pilot.OwnedSourceConfig `json:"source"`
			Binding string                  `json:"source_binding_id"`
		}{initial.Source, sourceID}),
		hostedCanonicalHash(ready.Fixtures[0]), hostedCanonicalHash(ready.Fixtures[1]), ready.LeagueSHA256,
	}
	for i, row := range prefix {
		if row.IdentitySHA256 != hashes[i] {
			return pilot.ErrManifestInitialization
		}
	}
	if e = pilot.AuditExistingOwnedWalletSource(filepath.Join(c.root, "shared"), initial.Hosted.AuthorityID, initial.Source, source, local.Recipient); e != nil {
		if errors.Is(e, pilot.ErrRemotePoolCleanup) {
			// The typed cause retains the exact failed audit descriptor. Keep the
			// original controller/root alongside it; Close must not release dependencies.
			c.mu.Lock()
			c.shutdownErr = errors.Join(c.shutdownErr, e)
			c.mu.Unlock()
		}
		return e
	}
	if c.init.Version != 0 && (!reflect.DeepEqual(c.init, initial) || c.ready != ready || !reflect.DeepEqual(c.prefix, prefix)) {
		return pilot.ErrManifestInitialization
	}
	c.init, c.ready, c.prefix = initial, ready, prefix
	return nil
}

func (c *RemotePoolController) selectedCurrent() (recoveryvault.CurrentBinding, error) {
	for i, p := range c.init.Wallets {
		if p.Fixture == c.fixture {
			return c.ready.Fixtures[i], nil
		}
	}
	return recoveryvault.CurrentBinding{}, pilot.ErrManifestInitialization
}
