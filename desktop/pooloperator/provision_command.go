// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/wallet"
)

const provisionUsage = `slugpool-provision create|resume|verify [local file selections]
--daily explicitly selects fresh DAILY provisioning; defaults remain V1.
--daily-recoverable-issue selects fresh exact-request issuance recovery.
The two selections are mutually exclusive.
Required: --policy-file --root --tls-certificate-file
 --wallet-public-passphrase-file --wallet-unlock-passphrase-file
Paths must be absolute and canonical; selected files must be protected regular
files in private directories. Passphrases are exact raw file bytes. create uses
OS randomness and a fresh root. resume keeps the original material and identities.
verify requires READY and every original output. No command overwrites or resets
files, contacts a node, starts services, mines, or accepts a checkpoint seed.
The protected operator wallet backup is not a service-journal restoration file.
Only bounded public JSON and closed error codes are printed.
`

type provisionOptions struct {
	command, policy, root, certificate, publicPass, unlockPass string
	daily, recoverableIssue                                    bool
}

func parseProvisionCommand(args []string) (provisionOptions, error) {
	var o provisionOptions
	if len(args) == 0 || len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		return o, flag.ErrHelp
	}
	if len(args) > 12 {
		return o, ErrCommandArguments
	}
	total := 0
	for _, a := range args {
		total += len(a)
		if len(a) > 8192 || total > 32768 {
			return o, ErrCommandArguments
		}
	}
	o.command = args[0]
	switch o.command {
	case "create", "resume", "verify":
	default:
		return o, ErrCommandArguments
	}
	fs := flag.NewFlagSet("slugpool-provision", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.daily, "daily", false, "")
	fs.BoolVar(&o.recoverableIssue, "daily-recoverable-issue", false, "")
	for name, dst := range map[string]*string{"policy-file": &o.policy, "root": &o.root, "tls-certificate-file": &o.certificate, "wallet-public-passphrase-file": &o.publicPass, "wallet-unlock-passphrase-file": &o.unlockPass} {
		fs.StringVar(dst, name, "", "")
	}
	seen := map[string]bool{}
	for i := 1; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return o, ErrCommandArguments
		}
		name := strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-")
		if j := strings.IndexByte(name, '='); j >= 0 {
			if (name[:j] == "daily" || name[:j] == "daily-recoverable-issue") && name[j+1:] != "true" {
				return o, ErrCommandArguments
			}
			name = name[:j]
		} else if name != "daily" && name != "daily-recoverable-issue" {
			i++
			if i >= len(args) {
				return o, ErrCommandArguments
			}
		}
		if name == "" || seen[name] {
			return o, ErrCommandArguments
		}
		seen[name] = true
	}
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || len(seen) != 5+boolCount(seen["daily"])+boolCount(seen["daily-recoverable-issue"]) || seen["daily"] && seen["daily-recoverable-issue"] {
		return o, ErrCommandArguments
	}
	for _, p := range []string{o.policy, o.root, o.certificate, o.publicPass, o.unlockPass} {
		if !commandPath(p) {
			return o, ErrCommandArguments
		}
	}
	// Selected inputs live outside the output inventory and cannot become outputs.
	for _, p := range []string{o.policy, o.certificate, o.publicPass, o.unlockPass} {
		if p == o.root || strings.HasPrefix(p, o.root+string(filepath.Separator)) {
			return o, ErrCommandArguments
		}
	}
	return o, nil
}

type provisionCommandOwner struct {
	common   *commandOwner
	files    *provisionFiles
	closeErr error
	closed   bool
}

func (*provisionCommandOwner) String() string   { return "operator provisioning owner" }
func (*provisionCommandOwner) GoString() string { return "operator provisioning owner" }

// ProvisionCleanupError retains the exact wallet, selected files and locked root
// after incomplete cleanup. Close repeats the sticky result without an OS retry.
type ProvisionCleanupError struct {
	owner *provisionCommandOwner
	cause error
}

func (*ProvisionCleanupError) Error() string    { return "operator provisioning cleanup incomplete" }
func (*ProvisionCleanupError) GoString() string { return "operator provisioning cleanup incomplete" }
func (e *ProvisionCleanupError) Unwrap() error  { return e.cause }
func (e *ProvisionCleanupError) Close() error   { return e.owner.close() }
func (o *provisionCommandOwner) retain(e error) error {
	if o.closeErr == nil {
		o.closeErr = &ProvisionCleanupError{o, errors.Join(ErrCleanup, e)}
	}
	return o.closeErr
}
func (o *provisionCommandOwner) close() error {
	if o.closeErr != nil {
		return o.closeErr
	}
	if o.closed {
		return nil
	}
	// Custody closes first. A failed wallet/selected-input close keeps the root lock.
	if e := o.common.close(); e != nil {
		return o.retain(e)
	}
	if o.files != nil {
		if e := o.files.close(); e != nil {
			return o.retain(e)
		}
	}
	o.closed = true
	return nil
}
func (o *provisionCommandOwner) check(ctx context.Context) error {
	if e := o.common.check(ctx); e != nil {
		return e
	}
	return o.files.check(ctx)
}
func provisionCode(e error) string {
	switch {
	case errors.Is(e, ErrCommandOutcomeUnknown):
		return "outcome_unknown"
	case errors.Is(e, ErrCleanup):
		return "cleanup_incomplete"
	case errors.Is(e, ErrProvisionIncompleteWallet):
		return "incomplete_wallet"
	case errors.Is(e, ErrProvisionIncomplete):
		return "incomplete"
	case errors.Is(e, ErrProvisionInvalid), errors.Is(e, ErrCommandArguments):
		return "invalid"
	default:
		return commandCode(e)
	}
}

type provisionResult struct {
	RequestID      string `json:"request_id"`
	PolicyID       string `json:"policy_id"`
	SetupID        string `json:"setup_id"`
	WalletGuardID  string `json:"wallet_guard_id"`
	AdmissionKey   string `json:"admission_key"`
	PublicationKey string `json:"publication_key"`
	CutoffKey      string `json:"cutoff_key"`
	FundingAddress string `json:"funding_address"`
}

// RunProvisionCommand is the operator-run, OS-random-only entry point. No public
// API, argument or environment selection injects entropy or a borrowed root key.
// Output uses the same bounded descriptor interruption contract as RunCommand.
func RunProvisionCommand(ctx context.Context, args []string, stdout, stderr *os.File) error {
	return runProvisionCommand(ctx, args, stdout, stderr, rand.Reader, time.Now)
}

// entropy and clock are private test seams; only published synthetic capsules are
// used by package tests. Neither is exposed by the production command.
func runProvisionCommand(ctx context.Context, args []string, stdout, stderr *os.File, entropy io.Reader, now func() time.Time) (result error) {
	if ctx == nil || stdout == nil || stderr == nil || entropy == nil || now == nil {
		return ErrCommandArguments
	}
	out := &commandOutput{file: stdout}
	errout := &commandOutput{file: stderr}
	if stdout == stderr {
		errout = out
	}
	o := &provisionCommandOwner{common: &commandOwner{stdout: out, stderr: errout}}
	command := "command"
	version := uint32(1)
	var response *commandResponse
	changed := func() bool { return o.files != nil && o.files.changed }
	defer func() {
		if e := o.close(); e != nil {
			result = errors.Join(result, e)
		}
		if result == nil && response != nil {
			result = out.json(ctx, response)
		}
		if result != nil && changed() {
			result = errors.Join(ErrCommandOutcomeUnknown, result)
		}
		if result != nil {
			diagnostic := commandResponse{Version: version, Command: command, ObservedAtMS: now().UTC().UnixMilli(), Code: provisionCode(result)}
			logctx, end := context.WithTimeout(context.Background(), 5*time.Second)
			e := errout.json(logctx, diagnostic)
			end()
			result = errors.Join(result, e)
		}
		// A failed interrupt after cleanup still owns its exact descriptor; never lose
		// that owner merely because custody had already joined.
		for _, w := range []*commandOutput{out, errout} {
			w.mu.Lock()
			e := w.closeErr
			w.mu.Unlock()
			if e != nil {
				result = errors.Join(result, o.retain(e))
			}
		}
		if result != nil {
			if changed() {
				result = errors.Join(ErrCommandOutcomeUnknown, result)
			}
			result = &CommandError{code: provisionCode(result), cause: result}
		}
	}()
	options, e := parseProvisionCommand(args)
	if e == flag.ErrHelp {
		return out.write(ctx, []byte(provisionUsage))
	}
	if e != nil {
		return e
	}
	command = options.command
	if options.daily || options.recoverableIssue {
		version = 2
	}
	fileVersion := version
	if options.recoverableIssue {
		fileVersion = 3
	}
	if wire.TestPoW {
		return ErrProvisionInvalid
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	raw, e := o.common.readInput(ctx, options.policy, MaxProvisionPolicyBytes+1)
	if e != nil {
		return e
	}
	policy, e := parseProvisionPolicyVersion(raw, version)
	clear(raw)
	if e != nil {
		return e
	}
	if options.daily || options.recoverableIssue {
		if e = dailyProvisionCapacity(policy); e != nil {
			return e
		}
	}
	if options.recoverableIssue {
		// Pure limits preflight precedes root creation and entropy. Exact complete
		// Setup/profile validation repeats after original material is derived.
		if e = registry.ValidateRecoverableIssueCapacity(registry.PublicOwnershipDefaultLimits(), policy.Limits.MaxWorks); e != nil {
			return ErrProvisionInvalid
		}
	}
	raw, e = o.common.readInput(ctx, options.certificate, 65536)
	if e != nil {
		return e
	}
	certificate, e := provisionCertificate(raw, policy, now().UTC())
	clear(raw)
	if e != nil {
		return e
	}
	publicPass, e := o.common.readInput(ctx, options.publicPass, 1024)
	if e != nil {
		return e
	}
	defer clear(publicPass)
	unlockPass, e := o.common.readInput(ctx, options.unlockPass, 1024)
	if e != nil {
		return e
	}
	defer clear(unlockPass)
	if len(publicPass) == 0 || len(unlockPass) == 0 {
		return ErrProvisionInvalid
	}
	if e = o.common.check(ctx); e != nil {
		return e
	}
	o.files, e = openProvisionFilesVersion(options.root, command == "create", fileVersion)
	if e != nil {
		return e
	}
	f := o.files
	intentLimit := 131072
	if options.recoverableIssue {
		intentLimit += 1024
	}
	raw, exists, e := f.read(ctx, "", provisionIntentFile(fileVersion), intentLimit)
	if e != nil {
		return e
	}
	var intent provisionIntent
	if exists {
		if options.recoverableIssue {
			intent, e = parseIssueProvisionIntent(raw, policy, certificate)
		} else {
			intent, e = parseProvisionIntent(raw, policy, certificate)
		}
		clear(raw)
		if e != nil {
			return ErrProvisionIncomplete
		}
	} else {
		if command != "create" {
			return ErrProvisionIncomplete
		}
		created := now().UTC().UnixMilli()
		if !provisionTime(created) {
			return ErrProvisionInvalid
		}
		pid, e := provisionPolicyIDVersion(policy, version)
		if e != nil {
			return e
		}
		intent = provisionIntent{version, policy, pid, certificate.CertificateSHA256, created}
		if options.recoverableIssue {
			raw, e = issueProvisionIntentBytes(intent)
		} else {
			raw, e = provisionJSON(intent, 131072)
		}
		if e != nil {
			return e
		}
		e = f.ensure(ctx, "", provisionIntentFile(fileVersion), raw, intentLimit, true)
		clear(raw)
		if e != nil {
			return e
		}
	}
	readyRaw, established, e := f.read(ctx, "", provisionReadyFile(fileVersion), 1024)
	if e != nil {
		return e
	}
	defer clear(readyRaw)
	if command == "verify" && !established {
		return ErrProvisionIncomplete
	}
	allowCreate := !established && command != "verify"
	if _, e = f.child("offline", allowCreate); e != nil {
		return e
	}
	capsule, hasMaterial, e := f.read(ctx, "offline", "material-v1.bin", 128)
	if e != nil {
		return errors.Join(ErrProvisionIncomplete, e)
	}
	defer func() { clear(capsule) }()
	if !hasMaterial {
		if !allowCreate || !f.canGenerate() {
			return ErrProvisionIncomplete
		}
		capsule = make([]byte, 128)
		if _, e = io.ReadFull(entropy, capsule); e != nil {
			return ErrUnavailable
		}
	}
	if len(capsule) != 128 {
		return ErrProvisionIncomplete
	}
	material, root, e := deriveProvisionMaterial(capsule, intent.CreatedAtMS)
	if e != nil {
		return e
	}
	defer root.Zero()
	setup, e := composeProvisionSetupVersion(policy, material, certificate, version)
	if e != nil {
		return e
	}
	var issueSetup *RecoverableDailySetup
	if options.recoverableIssue {
		v, x := ComposeRecoverableDailySetup(setup)
		if x != nil {
			return x
		}
		issueSetup = &v
	}
	expectedReady, e := makeProvisionReady(policy, setup, intent.CreatedAtMS)
	if e != nil {
		return e
	}
	expectedReadyRaw, e := provisionJSON(expectedReady, 1024)
	if issueSetup != nil {
		expectedReadyRaw, e = issueProvisionReadyBytes(expectedReady, *issueSetup)
	}
	if e != nil {
		return e
	}
	if established {
		if issueSetup != nil {
			if !bytes.Equal(readyRaw, expectedReadyRaw) {
				return ErrProvisionIncomplete
			}
		} else {
			var got provisionReady
			if provisionJSONFile(readyRaw, 1024, &got) != nil || got != expectedReady {
				return ErrProvisionIncomplete
			}
		}
	}
	if !hasMaterial {
		if e = f.ensure(ctx, "offline", "material-v1.bin", capsule, 128, true); e != nil {
			return e
		}
	}
	var walletEntropy [32]byte
	copy(walletEntropy[:], capsule[:32])
	backup, e := provisionBackupBytes(walletEntropy, policy, setup, intent.CreatedAtMS)
	clear(walletEntropy[:])
	if e != nil {
		return e
	}
	defer clear(backup)
	if e = verifyProvisionBackup(backup, policy, setup, intent.CreatedAtMS); e != nil {
		return e
	}
	backupLimit := 8192
	if issueSetup != nil {
		wrapped, x := issueProvisionBackupBytes(backup, *issueSetup)
		if x != nil {
			return x
		}
		clear(backup)
		backup = wrapped
		defer clear(wrapped)
		backupLimit += 1024
	}
	if e = f.ensure(ctx, "offline", provisionBackupFile(fileVersion), backup, backupLimit, allowCreate); e != nil {
		return e
	}
	for _, dir := range []string{"online", "public"} {
		if _, e = f.child(dir, allowCreate); e != nil {
			return e
		}
	}
	for i, name := range []string{"admission.seed", "publication.seed", "cutoff.seed"} {
		if e = f.ensure(ctx, "online", name, capsule[32+i*32:64+i*32], 32, allowCreate); e != nil {
			return e
		}
	}
	setupRaw, e := marshalSetupVersion(setup, version)
	setupLimit := MaxSetupBytes
	if issueSetup != nil {
		setupRaw, e = MarshalRecoverableDailySetup(*issueSetup)
		setupLimit = MaxRecoverableSetupBytes
	}
	if e != nil {
		return e
	}
	setupRaw = append(setupRaw, '\n')
	if e = f.ensure(ctx, "public", "setup.json", setupRaw, setupLimit+1, allowCreate); e != nil {
		return e
	}
	if e = o.check(ctx); e != nil {
		return e
	}
	walletExists, e := f.child("wallet", false)
	if e != nil {
		return errors.Join(ErrProvisionIncompleteWallet, e)
	}
	if !walletExists && !allowCreate {
		return ErrProvisionIncompleteWallet
	}
	custody, cancelCustody := context.WithCancel(context.Background())
	o.common.custodyCancel = cancelCustody
	openWallet, createWallet := wallet.OpenPoolSettlementWallet, wallet.CreatePoolSettlementWallet
	if options.daily || options.recoverableIssue {
		openWallet, createWallet = wallet.OpenDailyPoolSettlementWallet, wallet.CreateDailyPoolSettlementWallet
	}
	if walletExists {
		o.common.wallet, e = openWallet(custody, filepath.Join(options.root, "wallet"), setup.walletBinding(), publicPass)
	} else {
		// The factory may publish its directory before reporting any error. Never
		// retry Create on an existing directory, including a torn first creation.
		f.changed = true
		o.common.wallet, e = createWallet(custody, filepath.Join(options.root, "wallet"), setup.walletBinding(), publicPass, unlockPass, root, time.UnixMilli(intent.CreatedAtMS).UTC())
	}
	if e != nil {
		return errors.Join(ErrProvisionIncompleteWallet, e)
	}
	if _, e = f.child("wallet", false); e != nil {
		return e
	}
	if e = f.inventory(); e != nil {
		return e
	}
	if e = o.check(ctx); e != nil {
		return e
	}
	if e = o.common.wallet.Unlock(ctx, unlockPass, time.Minute); e != nil {
		return e
	}
	if e = o.common.wallet.Lock(ctx); e != nil {
		return e
	}
	if e = o.check(ctx); e != nil {
		return e
	}
	if e = o.common.wallet.Close(); e != nil {
		return o.retain(e)
	}
	o.common.wallet = nil
	// READY follows the original wallet's successful close. Other selected files
	// and the root lock stay owned through exact publication and final checks.
	if e = f.ensure(ctx, "", provisionReadyFile(fileVersion), expectedReadyRaw, 1024, allowCreate); e != nil {
		return e
	}
	if e = f.inventory(); e != nil {
		return e
	}
	if e = o.check(ctx); e != nil {
		return e
	}
	status := "verified"
	if !established {
		status = "provisioned"
	}
	selectedSetupID := expectedReady.SetupID
	if issueSetup != nil {
		selectedSetupID, e = RecoverableDailySetupID(*issueSetup)
		if e != nil {
			return e
		}
	}
	response = &commandResponse{Version: version, Command: command, ObservedAtMS: now().UTC().UnixMilli(), Code: status, Result: provisionResult{policy.RequestID, expectedReady.PolicyID, selectedSetupID, expectedReady.WalletGuardID, material.AdmissionKey, material.PublicationKey, material.CutoffKey, setup.Pool.PayoutAddress}}
	return nil
}

func boolCount(v bool) int {
	if v {
		return 1
	}
	return 0
}
