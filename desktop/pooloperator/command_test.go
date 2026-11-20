//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func commandTestArgs(command string) []string {
	return []string{command, "--setup-file", "/selected/setup", "--root", "/selected/root", "--wallet-root", "/selected/wallet", "--wallet-public-passphrase-file", "/selected/public-pass", "--wallet-unlock-passphrase-file", "/selected/unlock-pass", "--admission-seed-file", "/selected/admission", "--publication-seed-file", "/selected/publication", "--cutoff-seed-file", "/selected/cutoff", "--rpc-config-file", "/selected/rpc", "--tls-certificate-file", "/selected/certificate", "--tls-key-file", "/selected/key"}
}
func commandTestRotation() []string {
	return append(commandTestArgs("rotate"), "--gym-id", strings.Repeat("a1", 32), "--audience", "pool", "--expected-generation", "0", "--token-file", "/selected/token", "--expires-at-ms", "2000000000000")
}
func TestOperatorCommandFlags(t *testing.T) {
	for _, command := range []string{"init", "serve", "status"} {
		if o, e := parseCommand(commandTestArgs(command)); e != nil || o.command != command {
			t.Fatal("base command", command, e)
		}
	}
	if _, e := parseCommand(commandTestRotation()); e != nil {
		t.Fatal(e)
	}
	revoke := append(commandTestArgs("revoke"), "--gym-id", strings.Repeat("a1", 32), "--audience", "admission", "--expected-generation", "1")
	if _, e := parseCommand(revoke); e != nil {
		t.Fatal(e)
	}
	if _, e := parseCommand(append(commandTestArgs("checkpoint"), "--checkpoint-file", "/selected/offline-signed-public")); e != nil {
		t.Fatal(e)
	}
	controls := [][]string{
		append(commandTestArgs("status"), "--token-file", "/selected/token"),
		append(commandTestArgs("status"), "--gym-id", "pasted secret"),
		append(commandTestArgs("status"), "--setup-file=/substitute/setup"),
		append(commandTestArgs("status"), "--checkpoint-seed-file", "/selected/forbidden"),
		append(commandTestArgs("status"), "--password", "pasted secret"),
		append(commandTestArgs("status"), "unrequested-positional"),
		append(commandTestArgs("checkpoint"), "--checkpoint-file", "relative"),
		append(commandTestArgs("status"), "--help"),
	}
	for _, replace := range []struct{ at, value string }{
		{"--gym-id", "pasted secret"}, {"--gym-id", strings.Repeat("00", 32)},
		{"--audience", "transfer"}, {"--expected-generation", "+0"}, {"--expected-generation", "00"},
		{"--expected-generation", "4294967296"}, {"--expected-generation", "4097"},
		{"--expires-at-ms", "0"}, {"--expires-at-ms", "253402300800000"},
		{"--token-file", "/selected/../token"}, {"--setup-file", "/selected//setup"},
	} {
		a := commandTestRotation()
		for i := range a {
			if a[i] == replace.at {
				a[i+1] = replace.value
			}
		}
		controls = append(controls, a)
	}
	for i, a := range controls {
		if _, e := parseCommand(a); !errors.Is(e, ErrCommandArguments) {
			t.Fatalf("refusal %d: %v", i, e)
		}
	}
}
func TestOperatorCommandPublicParsing(t *testing.T) {
	ref := opSetupTestReference(t).Positives[0]
	raw := opSetupTestHex(t, ref.Canonical)
	for _, v := range [][]byte{raw, append(append([]byte(nil), raw...), '\n')} {
		s, e := parseCommandSetup(v)
		id, _ := SetupID(s)
		if e != nil || id != ref.ID {
			t.Fatal("canonical setup", e)
		}
	}
	for _, v := range [][]byte{append([]byte(" "), raw...), append(append([]byte(nil), raw...), '\r', '\n'), append(append([]byte(nil), raw...), '\n', '\n'), []byte("null")} {
		if _, e := parseCommandSetup(v); e == nil {
			t.Fatal("noncanonical setup")
		}
	}
	// Public syntax only: this is deliberately not a valid signature and cannot
	// authorize ImportCheckpoint. Use the existing strict checkpoint decoder.
	checkpoint := `{"version":1,"network":1,"genesis":"` + strings.Repeat("01", 32) + `","height":1,"hash":"` + strings.Repeat("02", 32) + `","signature":"` + strings.Repeat("03", 64) + `"}`
	if c, e := parseCommandCheckpoint([]byte(checkpoint)); e != nil || len(c.Signature) != 64 {
		t.Fatal("checkpoint syntax", e)
	}
	for _, v := range []string{checkpoint + "{}", strings.Replace(checkpoint, `"version":1`, `"version":1,"version":1`, 1), strings.Replace(checkpoint, `"height":1`, `"height":null`, 1), strings.Replace(checkpoint, `"height":1`, `"unknown":1`, 1), strings.Repeat(" ", 4097)} {
		if _, e := parseCommandCheckpoint([]byte(v)); e == nil {
			t.Fatal("checkpoint strict refusal")
		}
	}
}
func TestOperatorCommandPrivateInputAndCleanup(t *testing.T) {
	ctx := context.Background()
	dir := opSetupTestDir(t)
	path := filepath.Join(dir, "raw-passphrase")
	raw := []byte("\x00 synthetic raw passphrase \r\n")
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	o := &commandOwner{}
	defer func() {
		if e := o.close(); e != nil {
			t.Error(e)
		}
	}()
	got, e := o.readInput(ctx, path, 1024)
	if e != nil || !bytes.Equal(got, raw) {
		t.Fatal("passphrase bytes trimmed", e)
	}
	clear(got)
	replacement := filepath.Join(dir, "replacement")
	if e = os.WriteFile(replacement, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(replacement, path); e != nil {
		t.Fatal(e)
	}
	if e = o.check(ctx); !errors.Is(e, ErrIntegrity) {
		t.Fatal("selected path was rebound", e)
	}
	if e = o.close(); e != nil {
		t.Fatal(e)
	}
	// A real failed file close keeps the exact command/custody owner reachable.
	selected, e := selectPrivateInput(ctx, path, 1024)
	if e != nil {
		t.Fatal(e)
	}
	custody, cancel := context.WithCancel(ctx)
	defer cancel()
	failed := &commandOwner{inputs: []*privateInput{selected}, custodyCancel: cancel}
	if e = selected.file.Close(); e != nil {
		t.Fatal(e)
	}
	first := failed.close()
	var retained *CommandCleanupError
	if !errors.As(first, &retained) || retained.owner != failed || retained.Close() != first || failed.close() != first || custody.Err() != nil {
		t.Fatal("cleanup ownership/custody lost")
	}
}
func commandTestFile(t *testing.T, name string) *os.File {
	t.Helper()
	f, e := os.CreateTemp(t.TempDir(), name)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
func commandTestRead(t *testing.T, f *os.File) []byte {
	t.Helper()
	if _, e := f.Seek(0, io.SeekStart); e != nil {
		t.Fatal(e)
	}
	raw, e := io.ReadAll(io.LimitReader(f, 65537))
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestOperatorCommandNoSecretOutput(t *testing.T) {
	out := commandTestFile(t, "out")
	errout := commandTestFile(t, "err")
	secret := "PRIVATE-PASSPHRASE-MUST-NOT-PRINT"
	args := append(commandTestArgs("status"), "--password", secret)
	e := RunCommand(context.Background(), args, out, errout)
	if !errors.Is(e, ErrCommandArguments) || strings.Contains(e.Error(), secret) {
		t.Fatal("invalid command error")
	}
	stdout := commandTestRead(t, out)
	stderr := commandTestRead(t, errout)
	if len(stdout) != 0 || bytes.Contains(stderr, []byte(secret)) || bytes.Contains(stderr, []byte("/selected")) || !bytes.Contains(stderr, []byte(`"code":"invalid"`)) {
		t.Fatal("argument/private output leak")
	}
}
func commandTestFullPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	r, w, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	if e = w.SetWriteDeadline(time.Now().Add(50 * time.Millisecond)); e != nil {
		t.Fatal(e)
	}
	block := bytes.Repeat([]byte("x"), 65536)
	full := false
	for n := 0; n < 64; n++ {
		if _, e = w.Write(block); e != nil {
			if !errors.Is(e, os.ErrDeadlineExceeded) {
				t.Fatal(e)
			}
			full = true
			break
		}
	}
	if !full {
		t.Fatal("could not establish bounded full-pipe fixture")
	}
	if e = w.SetWriteDeadline(time.Time{}); e != nil {
		t.Fatal(e)
	}
	return r, w
}
func TestOperatorCommandBlockedOutputJoins(t *testing.T) {
	_, w := commandTestFullPipe(t)
	out := &commandOutput{file: w}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	e := out.write(ctx, []byte("public status\n"))
	if !errors.Is(e, ErrCommandOutput) || !errors.Is(e, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatal("blocked output did not join", e)
	}
	out.mu.Lock()
	interrupted := out.interrupted
	ce := out.closeErr
	out.mu.Unlock()
	if !interrupted || ce != nil {
		t.Fatal("original output interruption", ce)
	}
	if _, e = w.Write([]byte("late")); !errors.Is(e, os.ErrClosed) {
		t.Fatal("output remained live", e)
	}
}
func TestOperatorCommandFinalDiagnosticBound(t *testing.T) {
	_, w := commandTestFullPipe(t)
	out := commandTestFile(t, "out")
	started := time.Now()
	e := RunCommand(context.Background(), []string{"unknown-private-input"}, out, w)
	if !errors.Is(e, ErrCommandArguments) || !errors.Is(e, ErrCommandOutput) || time.Since(started) > 7*time.Second {
		t.Fatal("final diagnostic escaped bounded output ownership", e)
	}
}
func TestOperatorCommandUnknownPrecedence(t *testing.T) {
	e := errors.Join(ErrActionOutcomeUnknown, ErrCleanup, context.DeadlineExceeded, context.Canceled, errors.New("PRIVATE-NOT-FOR-OUTPUT"))
	if commandCode(e) != "outcome_unknown" || commandCancelled(e) {
		t.Fatal("ambiguous mutation disguised")
	}
	public := &CommandError{code: commandCode(e), cause: e}
	if strings.Contains(public.Error(), "PRIVATE") || !errors.Is(public, ErrActionOutcomeUnknown) {
		t.Fatal("static classification lost cause")
	}
	if commandCode(errors.Join(ErrCleanup, context.Canceled)) != "cleanup_incomplete" || commandCancelled(errors.Join(ErrCleanup, context.Canceled)) {
		t.Fatal("cleanup hidden by cancellation")
	}
	if !commandCancelled(errors.Join(context.Canceled, context.Canceled)) || commandCancelled(context.DeadlineExceeded) {
		t.Fatal("shutdown cancellation classification")
	}
}
