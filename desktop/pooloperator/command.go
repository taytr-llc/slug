// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/poolminer"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/wallet"
)

const commandUsage = `slugpoold COMMAND [local file selections]
Commands: init, serve, status, rotate, revoke, checkpoint
Required selections: --setup-file --root --wallet-root
 --wallet-public-passphrase-file --wallet-unlock-passphrase-file
 --admission-seed-file --publication-seed-file --cutoff-seed-file
 --rpc-config-file --tls-certificate-file --tls-key-file
rotate: --gym-id --audience pool|admission --expected-generation
 --token-file --expires-at-ms
revoke: --gym-id --audience pool|admission --expected-generation
checkpoint: --checkpoint-file (already signed public envelope)
All paths must be absolute and canonical. Selected files are bounded, protected
regular files in private directories. Passphrases are raw file bytes; tokens are
exactly 64 lowercase hexadecimal bytes without a newline. init creates/resumes
runtime children only: the schema16 wallet and online seed files must preexist.
No command generates keys, starts a miner, or exposes an offline checkpoint key.
Only serve starts listeners and the configured settlement scheduler. Local
commands require exclusive ownership of the stopped runtime root. Output is
bounded public JSON lines with dated observations and closed error codes.
`

var (
	ErrCommandArguments      = errors.New("invalid pool operator command arguments")
	ErrCommandOutput         = errors.New("pool operator command output unavailable")
	ErrCommandOutcomeUnknown = errors.New("pool operator command outcome unknown; recover exact original request")
)

// CommandError exposes only a closed code; Unwrap preserves original cleanup
// owners and unknown-outcome classification for programmatic local recovery.
type CommandError struct {
	code  string
	cause error
}

func (e *CommandError) Error() string    { return "pool operator command " + e.code }
func (e *CommandError) GoString() string { return e.Error() }
func (e *CommandError) Unwrap() error    { return e.cause }

type commandOptions struct {
	command, setup, root, walletRoot, publicPass, unlockPass          string
	admissionSeed, publicationSeed, cutoffSeed, rpc, certificate, key string
	gym, audience, token, checkpoint                                  string
	generation                                                        uint32
	expires                                                           int64
}

func commandPath(s string) bool {
	return len(s) > 1 && len(s) <= 4096 && filepath.IsAbs(s) && filepath.Clean(s) == s && !strings.ContainsRune(s, 0)
}
func parseCommand(args []string) (commandOptions, error) {
	var o commandOptions
	if len(args) == 0 || len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		return o, flag.ErrHelp
	}
	if len(args) > 64 {
		return o, ErrCommandArguments
	}
	total := 0
	for _, a := range args {
		total += len(a)
		if len(a) > 8192 || total > 65536 {
			return o, ErrCommandArguments
		}
	}
	o.command = args[0]
	switch o.command {
	case "init", "serve", "status", "rotate", "revoke", "checkpoint":
	default:
		return o, ErrCommandArguments
	}
	fs := flag.NewFlagSet("slugpoold", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	for name, p := range map[string]*string{
		"setup-file": &o.setup, "root": &o.root, "wallet-root": &o.walletRoot,
		"wallet-public-passphrase-file": &o.publicPass, "wallet-unlock-passphrase-file": &o.unlockPass,
		"admission-seed-file": &o.admissionSeed, "publication-seed-file": &o.publicationSeed, "cutoff-seed-file": &o.cutoffSeed,
		"rpc-config-file": &o.rpc, "tls-certificate-file": &o.certificate, "tls-key-file": &o.key,
		"gym-id": &o.gym, "audience": &o.audience, "token-file": &o.token, "checkpoint-file": &o.checkpoint,
	} {
		fs.StringVar(p, name, "", "")
	}
	var generation, expires string
	fs.StringVar(&generation, "expected-generation", "", "")
	fs.StringVar(&expires, "expires-at-ms", "", "")
	// Reject duplicates, including alternate --flag=value spelling. flag itself
	// otherwise silently replaces an earlier original selection.
	seen := map[string]bool{}
	for i := 1; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return o, ErrCommandArguments
		}
		name := strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-")
		if j := strings.IndexByte(name, '='); j >= 0 {
			name = name[:j]
		} else {
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
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return o, ErrCommandArguments
	}
	for _, p := range []string{o.setup, o.root, o.walletRoot, o.publicPass, o.unlockPass, o.admissionSeed, o.publicationSeed, o.cutoffSeed, o.rpc, o.certificate, o.key} {
		if !commandPath(p) {
			return o, ErrCommandArguments
		}
	}
	local := o.command == "rotate" || o.command == "revoke"
	for _, name := range []string{"gym-id", "audience", "expected-generation"} {
		if seen[name] != local {
			return o, ErrCommandArguments
		}
	}
	if local {
		n, e := strconv.ParseUint(generation, 10, 32)
		if e != nil || strconv.FormatUint(n, 10) != generation || n > 4096 || !identifier(o.gym) || (o.audience != poolminer.PoolAudience && o.audience != poolminer.AdmissionAudience) {
			return o, ErrCommandArguments
		}
		o.generation = uint32(n)
		if o.command == "revoke" && n == 0 {
			return o, ErrCommandArguments
		}
	}
	for _, name := range []string{"token-file", "expires-at-ms"} {
		if seen[name] != (o.command == "rotate") {
			return o, ErrCommandArguments
		}
	}
	if o.command == "rotate" {
		n, e := strconv.ParseInt(expires, 10, 64)
		if e != nil || strconv.FormatInt(n, 10) != expires || n < 1 || n > poolminer.MaxTimeMS || !commandPath(o.token) {
			return o, ErrCommandArguments
		}
		o.expires = n
	}
	if seen["checkpoint-file"] != (o.command == "checkpoint") || o.command == "checkpoint" && !commandPath(o.checkpoint) {
		return o, ErrCommandArguments
	}
	return o, nil
}

// commandOutput writes synchronously to an actual descriptor. A bounded joined
// cancellation callback interrupts that descriptor; no detached writer can keep
// runtime/custody cleanup waiting on a full output pipe. The caller gives this
// command permission to close that descriptor on cancellation/output timeout.
// Ordinary successful output does not close or change the caller's descriptor.
type commandOutput struct {
	file        *os.File
	mu          sync.Mutex
	interrupted bool
	closeErr    error
}

func (w *commandOutput) interrupt() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.interrupted {
		return
	}
	w.interrupted = true
	if e := w.file.Close(); e != nil {
		w.closeErr = errors.Join(ErrCleanup, e)
	}
}
func (w *commandOutput) write(ctx context.Context, raw []byte) error {
	if ctx == nil || w == nil || w.file == nil || len(raw) > 65536 {
		return ErrCommandOutput
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	w.mu.Lock()
	closed := w.interrupted
	w.mu.Unlock()
	if closed {
		return ErrCommandOutput
	}
	work, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	joined := make(chan struct{})
	stop := context.AfterFunc(work, func() { defer close(joined); w.interrupt() })
	n, e := w.file.Write(raw)
	if !stop() {
		<-joined
	}
	w.mu.Lock()
	ce := w.closeErr
	interrupted := w.interrupted
	w.mu.Unlock()
	if e != nil || n != len(raw) || interrupted || work.Err() != nil {
		return errors.Join(ErrCommandOutput, work.Err(), ce)
	}
	return nil
}
func (w *commandOutput) json(ctx context.Context, v any) error {
	raw, e := json.Marshal(v)
	if e != nil || len(raw) >= 65536 {
		return ErrCommandOutput
	}
	return w.write(ctx, append(raw, '\n'))
}

type commandOwner struct {
	runtime        *Runtime
	wallet         *wallet.PoolWalletOwner
	authority      *operatorauthority.Owner
	custodyCancel  context.CancelFunc
	inputs         []*privateInput
	stdout, stderr *commandOutput
	closed         bool
	closeErr       error
}

func (*commandOwner) String() string   { return "pool operator command owner" }
func (*commandOwner) GoString() string { return "pool operator command owner" }

// CommandCleanupError retains every original owner that could not be closed.
// Close reports the same sticky result: it never retries an uncertain OS close
// or cancels borrowed custody while dependent cleanup remains incomplete.
type CommandCleanupError struct {
	owner *commandOwner
	cause error
}

func (*CommandCleanupError) Error() string    { return "pool operator command cleanup incomplete" }
func (*CommandCleanupError) GoString() string { return "pool operator command cleanup incomplete" }
func (e *CommandCleanupError) Unwrap() error  { return e.cause }
func (e *CommandCleanupError) Close() error   { return e.owner.close() }
func (o *commandOwner) close() error {
	if o.closeErr != nil {
		return o.closeErr
	}
	if o.closed {
		return nil
	}
	fail := func(e error) error {
		o.closeErr = &CommandCleanupError{owner: o, cause: errors.Join(ErrCleanup, e)}
		return o.closeErr
	}
	if o.runtime != nil {
		if e := o.runtime.Close(); e != nil {
			return fail(e)
		}
		o.runtime = nil
	}
	if o.wallet != nil {
		if e := o.wallet.Close(); e != nil {
			return fail(e)
		}
		o.wallet = nil
	}
	if o.authority != nil {
		if e := o.authority.Close(); e != nil {
			return fail(e)
		}
		o.authority = nil
	}
	for i := len(o.inputs) - 1; i >= 0; i-- {
		if e := o.inputs[i].close(); e != nil {
			return fail(e)
		}
		o.inputs = o.inputs[:i]
	}
	for _, w := range []*commandOutput{o.stdout, o.stderr} {
		if w != nil {
			w.mu.Lock()
			e := w.closeErr
			w.mu.Unlock()
			if e != nil {
				return fail(e)
			}
		}
	}
	if o.custodyCancel != nil {
		o.custodyCancel()
		o.custodyCancel = nil
	}
	o.closed = true
	return nil
}
func (o *commandOwner) selectInput(ctx context.Context, path string, limit int) (*privateInput, error) {
	input, e := selectPrivateInput(ctx, path, limit)
	if input != nil {
		o.inputs = append(o.inputs, input)
	}
	return input, e
}
func (o *commandOwner) readInput(ctx context.Context, path string, limit int) ([]byte, error) {
	input, e := o.selectInput(ctx, path, limit)
	if e != nil {
		return nil, e
	}
	return input.read(ctx)
}
func (o *commandOwner) check(ctx context.Context) error {
	for _, input := range o.inputs {
		if e := input.check(ctx); e != nil {
			return e
		}
	}
	return ctx.Err()
}
func parseCommandSetup(raw []byte) (Setup, error) {
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}
	return ParseSetup(raw)
}
func parseCommandCheckpoint(raw []byte) (chaincfg.SignedCheckpoint, error) {
	var out chaincfg.SignedCheckpoint
	if len(raw) == 0 || len(raw) > 4096 || out.UnmarshalJSON(raw) != nil {
		return chaincfg.SignedCheckpoint{}, ErrInvalid
	}
	return out, nil
}

type commandResponse struct {
	Version      uint32 `json:"version"`
	Command      string `json:"command"`
	ObservedAtMS int64  `json:"observed_at_ms"`
	Code         string `json:"code"`
	Result       any    `json:"result,omitempty"`
}

func commandCode(e error) string {
	switch {
	case errors.Is(e, ErrCommandOutcomeUnknown), errors.Is(e, ErrActionOutcomeUnknown), errors.Is(e, poolminer.ErrOutcomeUnknown):
		return "outcome_unknown"
	case errors.Is(e, ErrCleanup):
		return "cleanup_incomplete"
	case errors.Is(e, ErrCommandArguments), errors.Is(e, ErrInvalid):
		return "invalid"
	case errors.Is(e, ErrCommandOutput):
		return "output_unavailable"
	case errors.Is(e, context.DeadlineExceeded):
		return "timed_out"
	case errors.Is(e, context.Canceled):
		return "cancelled"
	case errors.Is(e, ErrActionBusy):
		return "busy"
	default:
		return "unavailable"
	}
}

// Only a tree containing cancellation and nothing else is an ordinary stop.
// In particular joined cleanup, storage and unknown-outcome errors must survive.
func commandCancelled(e error) bool {
	if e == nil {
		return true
	}
	if e == context.Canceled {
		return true
	}
	if many, ok := e.(interface{ Unwrap() []error }); ok {
		for _, v := range many.Unwrap() {
			if !commandCancelled(v) {
				return false
			}
		}
		return true
	}
	if one, ok := e.(interface{ Unwrap() error }); ok {
		return commandCancelled(one.Unwrap())
	}
	return false
}
func (o *commandOwner) serve(ctx context.Context) (result error) {
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- o.runtime.Run(work, func(origin string) { ready <- origin }) }()
	joined := false
	defer func() {
		cancel()
		if !joined {
			result = errors.Join(result, <-done)
		}
	}()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case origin := <-ready:
			if e := o.check(work); e != nil {
				return e
			}
			if e := o.stdout.json(work, commandResponse{1, "serve", time.Now().UTC().UnixMilli(), "listening", struct {
				Origin string `json:"origin"`
			}{origin}}); e != nil {
				return e
			}
		case <-ticker.C:
			if e := o.check(work); e != nil {
				return e
			}
			s, e := o.runtime.Status(work)
			response := commandResponse{Version: 1, Command: "serve", ObservedAtMS: time.Now().UTC().UnixMilli(), Code: "status"}
			if e != nil {
				response.Code = commandCode(e)
			} else {
				response.Result = s
			}
			if e = o.stdout.json(work, response); e != nil {
				return e
			}
		case e := <-done:
			joined = true
			if ctx.Err() == context.Canceled && commandCancelled(e) {
				return nil
			}
			return e
		case <-ctx.Done():
			cancel()
			e := <-done
			joined = true
			if ctx.Err() == context.Canceled && commandCancelled(e) {
				return nil
			}
			return errors.Join(ctx.Err(), e)
		}
	}
}

// RunCommand owns selected files and concrete runtime/custody lifetimes. All
// flags and public inputs are validated before custody or runtime mutation. No
// literal secret flag, stdin secret, key generation or automatic mining exists.
// stdout/stderr must be actual owned output descriptors; see commandOutput.
// An error retains failed cleanup owners and uses a closed-code Error method.
// Printed diagnostics never include the underlying error tree.
func RunCommand(ctx context.Context, args []string, stdout, stderr *os.File) (result error) {
	if ctx == nil || stdout == nil || stderr == nil {
		return ErrCommandArguments
	}
	out := &commandOutput{file: stdout}
	errout := &commandOutput{file: stderr}
	if stdout == stderr {
		errout = out
	}
	o := &commandOwner{stdout: out, stderr: errout}
	mutated := false
	var response *commandResponse
	command := "command"
	defer func() {
		// No successful local mutation is reported until all original owners close.
		ce := o.close()
		if ce != nil {
			result = errors.Join(result, ce)
			if mutated {
				result = errors.Join(ErrCommandOutcomeUnknown, result)
			}
		}
		if result == nil && response != nil {
			if e := out.json(ctx, response); e != nil {
				result = e
				if mutated {
					result = errors.Join(ErrCommandOutcomeUnknown, result)
				}
			}
		}
		if result != nil && mutated {
			result = errors.Join(ErrCommandOutcomeUnknown, result)
		}
		if result != nil {
			diagnostic := commandResponse{Version: 1, Command: command, ObservedAtMS: time.Now().UTC().UnixMilli(), Code: commandCode(result)}
			// Even cancellation diagnostics get their own finite output bound; never
			// log the underlying RPC, filesystem or argument error text.
			logctx, end := context.WithTimeout(context.Background(), 5*time.Second)
			if e := errout.json(logctx, diagnostic); e != nil {
				result = errors.Join(result, e)
				if mutated {
					result = errors.Join(ErrCommandOutcomeUnknown, result)
				}
			}
			end()
		}
		// Output interruption may itself have failed after custody cleanup. Preserve
		// its exact descriptor owner too, without retrying Close.
		for _, w := range []*commandOutput{out, errout} {
			w.mu.Lock()
			e := w.closeErr
			w.mu.Unlock()
			if e != nil {
				if o.closeErr == nil {
					o.closeErr = &CommandCleanupError{owner: o, cause: errors.Join(ErrCleanup, e)}
				}
				result = errors.Join(result, o.closeErr)
			}
		}
		if result != nil {
			result = &CommandError{code: commandCode(result), cause: result}
		}
	}()
	options, e := parseCommand(args)
	if e == flag.ErrHelp {
		return out.write(ctx, []byte(commandUsage))
	}
	if e != nil {
		return ErrCommandArguments
	}
	command = options.command
	if e = ctx.Err(); e != nil {
		return e
	}
	raw, e := o.readInput(ctx, options.setup, MaxSetupBytes+1)
	if e != nil {
		return e
	}
	setup, e := parseCommandSetup(raw)
	clear(raw)
	if e != nil {
		return ErrInvalid
	}
	raw, e = o.readInput(ctx, options.rpc, prospectnode.MaxRPCConfigSize)
	if e != nil {
		return e
	}
	rpc, e := prospectnode.ParseRPCConfig(raw)
	clear(raw)
	if e != nil {
		return ErrInvalid
	}
	for _, spec := range []struct {
		path  string
		limit int
	}{
		{options.admissionSeed, 32}, {options.publicationSeed, 32}, {options.cutoffSeed, 32},
		{options.unlockPass, 1024}, {options.certificate, 65536}, {options.key, 16384},
	} {
		if _, e = o.selectInput(ctx, spec.path, spec.limit); e != nil {
			return e
		}
	}
	var token []byte
	defer func() { clear(token) }()
	var checkpoint chaincfg.SignedCheckpoint
	if command == "rotate" {
		token, e = o.readInput(ctx, options.token, 64)
		if e != nil {
			return e
		}
		if !identifier(string(token)) {
			return ErrInvalid
		}
	}
	if command == "checkpoint" {
		raw, e = o.readInput(ctx, options.checkpoint, 4096)
		if e != nil {
			return e
		}
		checkpoint, e = parseCommandCheckpoint(raw)
		clear(raw)
		if e != nil {
			return e
		}
	}
	publicPass, e := o.readInput(ctx, options.publicPass, 1024)
	if e != nil {
		return e
	}
	defer clear(publicPass)
	profile, e := registry.NewOperatorProfile(setup.Authority)
	if e != nil {
		return ErrInvalid
	}
	binding, e := authorityBinding(profile)
	if e != nil {
		return e
	}
	if e = o.check(ctx); e != nil {
		return e
	}
	// Signal cancellation controls work, never borrowed custody teardown. These
	// original owners remain usable for Runtime.Close's lock/join until all close.
	custody, cancel := context.WithCancel(context.Background())
	o.custodyCancel = cancel
	o.authority, e = operatorauthority.OpenOnline(custody, binding, operatorauthority.OnlineSeedFiles{Admission: options.admissionSeed, Publication: options.publicationSeed, Cutoff: options.cutoffSeed})
	if e != nil {
		return e
	}
	if e = o.check(ctx); e != nil {
		return e
	}
	o.wallet, e = wallet.OpenPoolSettlementWallet(custody, options.walletRoot, setup.walletBinding(), publicPass)
	clear(publicPass)
	if e != nil {
		return e
	}
	if e = o.check(ctx); e != nil {
		return e
	}
	borrowed := BorrowedOwners{Authority: o.authority, Wallet: o.wallet}
	private := PrivateInputs{NodeRPC: rpc, TLSCertificateFile: options.certificate, TLSPrivateKeyFile: options.key, UnlockPassphraseFile: options.unlockPass}
	if command == "init" {
		mutated = true
		o.runtime, e = Create(ctx, options.root, setup, borrowed, private)
	} else {
		o.runtime, e = Open(ctx, options.root, setup, borrowed, private)
	}
	if e != nil {
		if mutated {
			return errors.Join(ErrCommandOutcomeUnknown, e)
		}
		return e
	}
	if e = o.check(ctx); e != nil {
		if mutated {
			return errors.Join(ErrCommandOutcomeUnknown, e)
		}
		return e
	}
	response = &commandResponse{Version: 1, Command: command, ObservedAtMS: time.Now().UTC().UnixMilli(), Code: "complete"}
	switch command {
	case "serve":
		response = nil
		return o.serve(ctx)
	case "init", "status":
		response.Result, e = o.runtime.Status(ctx)
	case "rotate":
		response.Result, e = o.runtime.RotateCredential(ctx, options.gym, options.audience, options.generation, string(token), options.expires)
		mutated = e == nil || errors.Is(e, ErrActionOutcomeUnknown)
	case "revoke":
		response.Result, e = o.runtime.RevokeCredential(ctx, options.gym, options.audience, options.generation)
		mutated = e == nil || errors.Is(e, ErrActionOutcomeUnknown)
	case "checkpoint":
		response.Result, e = o.runtime.ImportCheckpoint(ctx, checkpoint)
		mutated = e == nil || errors.Is(e, ErrActionOutcomeUnknown)
	}
	clear(token)
	if e != nil {
		return e
	}
	if e = o.check(ctx); e != nil {
		if mutated {
			return errors.Join(ErrCommandOutcomeUnknown, e)
		}
		return e
	}
	response.ObservedAtMS = time.Now().UTC().UnixMilli()
	return nil
}
