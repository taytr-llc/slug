// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package pilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
)

// backupPath accepts a native-dialog selection, never a webview-supplied path.
// Resolve its parent before comparing managed roots or creating a file.
func (c *Controller) backupPath(path string, existing bool) (string, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("managed application storage unavailable")
	}
	return c.backupPathUnderConfig(path, existing, configRoot)
}

// The explicit config root lets tests exercise sibling-pilot protection without
// creating, reading or changing any real application data.
func (c *Controller) backupPathUnderConfig(path string, existing bool, configRoot string) (string, error) {
	if !ValidRecoveryPath(path) || strings.ContainsRune(path, '\t') {
		return "", errors.New("invalid backup path")
	}
	path = filepath.Clean(path)
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", errors.New("backup directory unavailable")
	}
	path = filepath.Join(parent, filepath.Base(path))
	roots := []string{c.config.DataDir, filepath.Dir(c.config.NodeBinary), filepath.Join(filepath.Dir(c.config.DataDir), "LocalPilot"), filepath.Join(configRoot, "SLUG")}
	for _, root := range roots {
		if canonical, err := filepath.EvalSymlinks(root); err == nil {
			root = canonical
		}
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", errors.New("backup path overlaps managed data")
		}
	}
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return "", errors.New("backup must be a regular file")
		}
		if existing && info.Size() > recovery.MaxExportSize {
			return "", errors.New("backup exceeds size bound")
		}
	} else if existing || !os.IsNotExist(err) {
		return "", errors.New("backup file unavailable")
	}
	return path, nil
}

func (c *Controller) beginOperation(name string, duration time.Duration) (context.Context, chan struct{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.cancel != nil || c.manifest.Phase != "ready" {
		return nil, nil, errors.New("stop mining and initialize the wallet before this operation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	c.cancel, c.operation, c.done = cancel, name, make(chan struct{})
	return ctx, c.done, nil
}

func (c *Controller) finishOperation(done chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
	c.cancel, c.operation = nil, ""
	c.snapshot.WalletState = "closed"
	close(done)
}

// ExportRecovery writes the actual reopened retained wallet export. Neither its
// bytes nor its native-selected path are returned through the public bridge.
func (c *Controller) ExportRecovery(path string) (resultErr error) {
	path, err := c.backupPath(path, false)
	if err != nil {
		return errors.New("recovery export unavailable")
	}
	ctx, done, err := c.beginOperation("export", 30*time.Second)
	if err != nil {
		return err
	}
	defer c.finishOperation(done)
	wallet, err := openFixtureWallet(filepath.Join(c.config.DataDir, "wallet"), false)
	if err != nil {
		return errors.New("recovery export unavailable")
	}
	defer func() {
		if wallet.Close() != nil {
			resultErr = errors.New("recovery export close failed")
			c.noteShutdown("pilot wallet export close failed")
		}
	}()
	if wallet.Identity() != c.manifest.Identity {
		return errors.New("recovery export identity mismatch")
	}
	bundle, err := wallet.ExportRecovery(ctx)
	if err != nil {
		return errors.New("recovery export unavailable")
	}
	raw, err := recovery.MarshalExport(bundle)
	bundle.Mnemonic = ""
	defer clear(raw)
	if err != nil || ctx.Err() != nil {
		return errors.New("recovery export unavailable")
	}
	output := append(raw, '\n')
	defer clear(output)
	if atomicBytes(path, output) != nil {
		return errors.New("recovery export could not be saved")
	}
	return nil
}

// VerifyBackup starts a bounded, cancellable file-only restoration without
// replacing the live wallet, registry, outbox or their identities.
func (c *Controller) VerifyBackup(path string) error {
	path, err := c.backupPath(path, true)
	if err != nil {
		return errors.New("backup verification unavailable")
	}
	ctx, done, err := c.beginOperation("backup", 10*time.Minute)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.snapshot.BackupState, c.snapshot.BackupPhase, c.snapshot.BackupError = "running", "opening", ""
	c.mu.Unlock()
	go func() {
		defer c.finishOperation(done)
		code := c.verifyBackup(ctx, path)
		c.backupResult(ctx, code)
	}()
	return nil
}

func (c *Controller) backupResult(ctx context.Context, code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case code == "backup_storage_failed":
		// Cancellation cannot hide a failed close or scratch-directory removal.
		c.snapshot.BackupState, c.snapshot.BackupError = "failed", code
	case errors.Is(ctx.Err(), context.Canceled):
		c.snapshot.BackupState, c.snapshot.BackupPhase, c.snapshot.BackupError = "cancelled", "", ""
	case code != "":
		c.snapshot.BackupState, c.snapshot.BackupError = "failed", code
	default:
		c.snapshot.BackupState, c.snapshot.BackupPhase, c.snapshot.BackupError = "succeeded", "complete", ""
	}
}

// The remover argument keeps filesystem-failure tests independent of platform
// permission behavior; production always supplies os.RemoveAll.
func (c *Controller) cleanupBackupScratch(path string, remove func(string) error) error {
	if err := remove(path); err != nil {
		c.noteShutdown("backup scratch cleanup failed")
		return errors.New("backup scratch cleanup failed")
	}
	return nil
}

func (c *Controller) backupPhase(phase string) {
	c.mu.Lock()
	c.snapshot.BackupPhase = phase
	c.mu.Unlock()
}

func readBackup(path string) (recovery.Export, error) {
	selected, err := os.Lstat(path)
	if err != nil || !selected.Mode().IsRegular() || selected.Size() > recovery.MaxExportSize {
		return recovery.Export{}, errors.New("invalid backup file")
	}
	f, err := os.Open(path)
	if err != nil {
		return recovery.Export{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(selected, info) || info.Size() > recovery.MaxExportSize {
		return recovery.Export{}, errors.New("invalid backup file")
	}
	raw, err := io.ReadAll(io.LimitReader(f, recovery.MaxExportSize+1))
	defer clear(raw)
	if err != nil || len(raw) > recovery.MaxExportSize {
		return recovery.Export{}, errors.New("invalid backup size")
	}
	return recovery.ParseExport(raw)
}

func (c *Controller) verifyBackup(ctx context.Context, path string) (code string) {
	bundle, err := readBackup(path)
	if err != nil {
		return "backup_invalid"
	}
	defer func() { bundle.Mnemonic = "" }()
	if bundle.Manifest.RootPublicKey != c.manifest.Identity.RootPublicKey || bundle.Manifest.GymPublicKey != c.manifest.Identity.GymPublicKey || bundle.Manifest.GymID != c.manifest.Identity.GymID {
		return "backup_mismatch"
	}
	if ctx.Err() != nil {
		return "backup_unavailable"
	}
	if checkInitializedChains(c.config.DataDir, c.manifest) != nil {
		return "backup_storage_failed"
	}
	c.backupPhase("checking")
	nodes, err := c.openNodes(ctx)
	if err != nil {
		if errors.Is(err, errPilotNodeClose) {
			return "backup_storage_failed"
		}
		return "backup_unavailable"
	}
	defer func() {
		if nodes.Close() != nil {
			code = "backup_storage_failed"
			c.noteShutdown("backup node close failed")
		}
	}()
	original, err := openFixtureWallet(filepath.Join(c.config.DataDir, "wallet"), false)
	if err != nil {
		return "backup_unavailable"
	}
	originalClosed := false
	defer func() {
		if !originalClosed && original.Close() != nil {
			code = "backup_storage_failed"
			c.noteShutdown("backup original wallet close failed")
		}
	}()
	if original.Identity() != c.manifest.Identity || original.Attach(ctx, nodes.rpcConfig()) != nil {
		return "backup_mismatch"
	}
	if _, err := original.EnsurePayout(ctx, &c.manifest.Payout); err != nil {
		return "backup_mismatch"
	}
	if waitWallet(ctx, original, nodes.primary) != nil {
		return "backup_unavailable"
	}
	expected, err := original.VerificationState(ctx)
	if err != nil {
		return "backup_unavailable"
	}
	tip, err := observedTip(ctx, nodes.primary)
	if err != nil {
		return "backup_unavailable"
	}
	if err := original.Close(); err != nil {
		originalClosed = true
		c.noteShutdown("backup original wallet close failed")
		return "backup_storage_failed"
	}
	originalClosed = true
	// The fresh restorer receives only the selected file's parsed bundle.
	// The original wallet has been closed; expectations above are public data.
	c.backupPhase("restoring")
	scratch, err := os.MkdirTemp(c.config.DataDir, "backup-verify-")
	if err != nil {
		return "backup_storage_failed"
	}
	defer func() {
		if c.cleanupBackupScratch(scratch, os.RemoveAll) != nil {
			code = "backup_storage_failed"
		}
	}()
	restored, err := restoreFixtureWallet(filepath.Join(scratch, "wallet"), bundle)
	bundle.Mnemonic = ""
	if err != nil {
		return "backup_invalid"
	}
	defer func() {
		if restored.Close() != nil {
			code = "backup_storage_failed"
			c.noteShutdown("backup restored wallet close failed")
		}
	}()
	if restored.Identity() != c.manifest.Identity || restored.Attach(ctx, nodes.rpcConfig()) != nil {
		return "backup_mismatch"
	}
	if _, err := restored.EnsurePayout(ctx, &c.manifest.Payout); err != nil {
		return "backup_mismatch"
	}
	c.backupPhase("syncing")
	if waitWallet(ctx, restored, nodes.primary) != nil {
		return "backup_unavailable"
	}
	actual, err := restored.VerificationState(ctx)
	if err != nil {
		return "backup_unavailable"
	}
	if !reflect.DeepEqual(expected, actual) {
		return "backup_mismatch"
	}
	c.backupPhase("verifying")
	// A deliberately unmined, unpublished job tests only restored key control.
	// There is no work/eligibility claim, submission, admission or new receipt.
	initial, proof, err := backupSigningProof(tip, c.manifest.Identity)
	if err != nil {
		return "backup_unavailable"
	}
	signer, err := restored.Signer(ctx, policy())
	if err != nil {
		return "backup_unavailable"
	}
	authorization, err := signer(ctx, initial, proof)
	if err != nil || gymauth.VerifyAuthorization(initial, proof, authorization) != nil {
		return "backup_mismatch"
	}
	after, err := observedTip(ctx, nodes.primary)
	if err != nil || after != tip || ctx.Err() != nil {
		return "backup_unavailable"
	}
	c.mu.Lock()
	c.snapshot.Height, c.snapshot.WalletHeight = uint64(actual.View.Height), uint64(actual.View.Height)
	c.snapshot.SpendableAtoms, c.snapshot.ImmatureAtoms = actual.View.Spendable, actual.View.Immature
	c.mu.Unlock()
	return ""
}

// backupSigningProof is a syntactic public challenge, never a mined share or an
// authority-issued job. Its fixed time/bits deliberately avoid a dependency on
// GBT currentness when verifying an old, stopped local chain's wallet backup.
func backupSigningProof(tip localTip, identity walletIdentity) (prospect.Context, prospect.Proof, error) {
	p := policy()
	initial := prospect.Context{Version: 1, Network: p.Network, Genesis: p.Genesis,
		DNAVersion: p.DNAVersion, ProspectTarget: p.ProspectTarget, ParentHash: tip.Hash,
		ParentHeight: uint32(tip.Height), ExpectedBits: chaincfg.MainNetParams.PowLimitBits,
		MinTimestamp: 1, MaxTimestamp: 1}
	job, err := prospectjob.New(initial, identity.GymPublicKey, strings.Repeat("42", 32), 1)
	if err != nil {
		return prospect.Context{}, prospect.Proof{}, err
	}
	script, err := prospectjob.CommitmentScript(job)
	if err != nil {
		return prospect.Context{}, prospect.Proof{}, err
	}
	height, err := txscript.NewScriptBuilder().AddInt64(int64(tip.Height) + 1).AddData([]byte("PUBLIC-BACKUP-CHECK")).Script()
	if err != nil {
		return prospect.Context{}, prospect.Proof{}, err
	}
	coinbase := wire.NewMsgTx(2)
	coinbase.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: wire.MaxPrevOutIndex}, height, nil))
	coinbase.AddTxOut(wire.NewTxOut(0, script))
	parent, err := chainhash.NewHashFromStr(tip.Hash)
	if err != nil {
		return prospect.Context{}, prospect.Proof{}, err
	}
	header := wire.BlockHeader{Version: 4, PrevBlock: *parent, MerkleRoot: coinbase.TxHash(), Timestamp: time.Unix(1, 0), Bits: initial.ExpectedBits}
	var headerBytes, transaction bytes.Buffer
	if err := header.Serialize(&headerBytes); err != nil {
		return prospect.Context{}, prospect.Proof{}, err
	}
	if err := coinbase.Serialize(&transaction); err != nil {
		return prospect.Context{}, prospect.Proof{}, err
	}
	return initial, prospect.Proof{Version: 2, GymID: identity.GymID, JobID: job.ID,
		Header: hex.EncodeToString(headerBytes.Bytes()), Coinbase: hex.EncodeToString(transaction.Bytes()), MerkleBranch: []string{}}, nil
}

type localTip struct {
	Hash   string `json:"hash"`
	Height int32  `json:"height"`
}

func observedTip(ctx context.Context, node *childNode) (localTip, error) {
	var tip localTip
	if err := node.rpc.call(ctx, "getbestblock", &tip); err != nil || len(tip.Hash) != 64 || tip.Height < 0 {
		return localTip{}, errors.New("node tip unavailable")
	}
	return tip, nil
}

func waitWallet(ctx context.Context, wallet *walletRuntime, node *childNode) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		view, err := wallet.Snapshot(ctx)
		tip, tipErr := observedTip(ctx, node)
		// Genesis bootstrap can be deemed non-current by the inherited wallet
		// until a contemporary block exists. Registration/identity still passed.
		if err == nil && tipErr == nil && view.Height == tip.Height && (view.Synced || tip.Height == 0) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
