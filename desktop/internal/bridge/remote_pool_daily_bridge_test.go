//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"
)

const dailyBridgeReferenceSHA = "b62572b6f1966acad3845348feb446ebb24cb5d1d3c043498a6f95e5b6645f1a"

type dailyBridgeReference struct {
	Selection struct {
		Remote pilot.RecoverableRemotePoolConfig `json:"remote"`
	} `json:"selection"`
	Cases []struct {
		Name     string `json:"name"`
		Valid    bool   `json:"valid"`
		FrameHex string `json:"frame_hex"`
	} `json:"cases"`
}

func dailyBridgeReferenceAt(path string) (dailyBridgeReference, error) {
	var r dailyBridgeReference
	raw, e := os.ReadFile(path)
	if e != nil {
		return r, e
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != dailyBridgeReferenceSHA {
		return r, ErrUnavailable
	}
	e = json.Unmarshal(raw, &r)
	return r, e
}
func dailyBridgeCommand(t *testing.T, mode string) (*exec.Cmd, pilot.RecoverableRemotePoolConfig) {
	t.Helper()
	path, e := filepath.Abs("../../pilot/testdata/daily-wallet-protocol/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	r, e := dailyBridgeReferenceAt(path)
	if e != nil {
		t.Fatal(e)
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(exe, "-test.run=^TestDailyWalletBridgeFixtureProcess$")
	cmd.Env = append(os.Environ(), "SLUG_DAILY_WALLET_PIPE_FIXTURE=1", "SLUG_DAILY_WALLET_REFERENCE="+path, "SLUG_DAILY_WALLET_MODE="+mode)
	return cmd, r.Selection.Remote
}

// A synthetic protocol process using independent public frames. This proves
// pipe ownership/correlation only, never real wallet/session/HTTP acceptance.
func TestDailyWalletBridgeFixtureProcess(t *testing.T) {
	if os.Getenv("SLUG_DAILY_WALLET_PIPE_FIXTURE") != "1" {
		return
	}
	signal.Ignore(syscall.SIGPIPE)
	r, e := dailyBridgeReferenceAt(os.Getenv("SLUG_DAILY_WALLET_REFERENCE"))
	if e != nil {
		os.Exit(21)
	}
	mode := os.Getenv("SLUG_DAILY_WALLET_MODE")
	active := false
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 2048), pilot.MaxDailyWalletRequestSize)
	for reader.Scan() {
		q, e := pilot.ParseDailyWalletRequest(reader.Bytes(), r.Selection.Remote)
		if e != nil {
			os.Exit(22)
		}
		name := "control-" + q.Command + "-result"
		if q.Command == "snapshot" && !active {
			name = "initial-signed-out-result"
		}
		switch q.Command {
		case "daily_payout_begin":
			name = "begin-working"
		case "daily_payout_read":
			name = "fresh-empty-nonfinal"
		case "daily_payout_cancel":
			name = "completed-cancel-keeps-capture"
		}
		var out pilot.DailyWalletResponse
		found := false
		for _, v := range r.Cases {
			if v.Name == name && v.Valid {
				raw, e := hex.DecodeString(v.FrameHex)
				if e != nil || json.Unmarshal(bytes.TrimSuffix(raw, []byte{'\n'}), &out) != nil {
					os.Exit(23)
				}
				found = true
				break
			}
		}
		if !found {
			os.Exit(24)
		}
		out.ID, out.Command, out.ConfigID, out.ViewID = q.ID, q.Command, q.ConfigID, q.ViewID
		if out.Job != nil {
			out.Job.Sequence, out.Job.CaptureSequence, out.Job.BeforeBatch = q.Sequence, q.CaptureSequence, q.BeforeBatch
		}
		if q.Command == "login_fixture" {
			active = true
		}
		if q.Command == "logout" {
			active = false
		}
		if q.Command == "daily_payout_begin" {
			switch mode {
			case "lost":
				_, _ = io.Copy(io.Discard, os.Stdin)
				os.Exit(0)
			case "wrong-id":
				out.ID++
			case "busy":
				out.OK = false
				out.Job = nil
				out.Error = "busy"
			}
		}
		raw, e := json.Marshal(out)
		if e != nil {
			os.Exit(25)
		}
		if _, e = os.Stdout.Write(append(raw, '\n')); e != nil {
			os.Exit(0)
		}
	}
	os.Exit(0)
}
func dailyBridgeJoin(t *testing.T, b *DailyWalletBridge) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e := b.Close(ctx); e != nil {
		t.Error("original bridge close", e)
	}
	select {
	case <-b.readDone:
	default:
		t.Error("reader not joined")
	}
	select {
	case <-b.waitDone:
	default:
		t.Error("child not joined")
	}
	if b.cmd.ProcessState == nil {
		t.Error("child not reaped")
	}
}
func TestDailyWalletBridgeOriginalProcessCaptureAndOpaqueView(t *testing.T) {
	cmd, cfg := dailyBridgeCommand(t, "normal")
	ctx, cancel := context.WithCancel(context.Background())
	b, e := startDailyWalletCommand(ctx, cmd, cfg)
	cancel()
	if b != nil {
		t.Cleanup(func() { dailyBridgeJoin(t, b) })
	}
	if e != nil {
		t.Fatal(e)
	}
	if _, e = b.LoginFixture(context.Background(), cfg.Base.ExpectedLocal.Recipient.Config.Fixture); e != nil {
		t.Fatal(e)
	}
	v, e := b.Context()
	if e != nil {
		t.Fatal(e)
	}
	copy := *v
	before := b.nextID
	for _, bad := range []*DailyWalletContext{nil, &copy, {helper: &DailyWalletBridge{}, view: v.View()}} {
		if _, e = b.BeginDailyPayoutRead(context.Background(), bad, 1, 0, 0); e == nil || b.nextID != before {
			t.Fatal("manufactured view dispatched")
		}
	}
	for i := 0; i < 2; i++ {
		out, e := b.BeginDailyPayoutRead(context.Background(), v, 1, 0, 0)
		if e != nil || out.Job.Sequence != 1 {
			t.Fatal("same request sequence", e)
		}
	}
	out, e := b.DailyPayoutRead(context.Background(), v, 1)
	if e != nil || out.Read.Capture.Sequence != 1 || out.Read.Capture.First.Done {
		t.Fatal("empty first window became EOF", e)
	}
	original := v.View()
	out.Read.Capture.Binding.View.ViewID = "caller mutation"
	if v.View() != original || !b.ContextMatches(v) {
		t.Fatal("public capture changed original context")
	}
	if _, e = b.CancelDailyPayoutRead(context.Background(), v, 1); e != nil {
		t.Fatal(e)
	}
	if _, e = b.Stop(context.Background(), v); e != nil {
		t.Fatal(e)
	}
	if _, e = b.Logout(context.Background(), v); e != nil {
		t.Fatal(e)
	}
	if b.ContextMatches(v) {
		t.Fatal("logout retained old view")
	}
	before = b.nextID
	if _, e = b.DailyPayoutRead(context.Background(), v, 1); e == nil || b.nextID != before {
		t.Fatal("old view read dispatched")
	}
}
func TestDailyWalletBridgeLostBeginRetiresWithoutReplacement(t *testing.T) {
	for _, mode := range []string{"lost", "wrong-id", "busy"} {
		t.Run(mode, func(t *testing.T) {
			cmd, cfg := dailyBridgeCommand(t, mode)
			b, e := startDailyWalletCommand(context.Background(), cmd, cfg)
			if b != nil {
				t.Cleanup(func() { dailyBridgeJoin(t, b) })
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = b.LoginFixture(context.Background(), cfg.Base.ExpectedLocal.Recipient.Config.Fixture); e != nil {
				t.Fatal(e)
			}
			v, e := b.Context()
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			_, e = b.BeginDailyPayoutRead(ctx, v, 1, 0, 0)
			cancel()
			if mode == "busy" {
				var f *RemotePoolFailure
				if !errors.As(e, &f) || f.Code != "busy" || !b.ContextMatches(v) {
					t.Fatal("definite refusal lost owner", e)
				}
				return
			}
			if !errors.Is(e, ErrRemotePoolOutcomeUnknown) || b.ContextMatches(v) {
				t.Fatal("uncertain Begin did not retire", e)
			}
			before := b.nextID
			if _, e = b.BeginDailyPayoutRead(context.Background(), v, 2, 0, 0); e == nil || b.nextID != before {
				t.Fatal("new request after uncertain original")
			}
		})
	}
}
func TestDailyWalletBridgeSelectedHelperArguments(t *testing.T) {
	s := RemotePoolLaunchSelection{ExistingRoot: "/public/original", AttachmentRoot: "/public/attachment", ConfigPath: "/public/config", NodeRPCPath: "/public/rpc", PoolCredentialPath: "/public/pool", AdmissionCredentialPath: "/public/admission"}
	cmd, e := dailyWalletLaunchCommand("/public/slugpilot", s)
	if e != nil {
		t.Fatal(e)
	}
	want := []string{"/public/slugpilot", "--daily-recoverable-issue", "--daily-wallet-helper", "--data-dir", s.ExistingRoot, "--remote-attachment-root", s.AttachmentRoot, "--remote-config", s.ConfigPath, "--remote-node-rpc", s.NodeRPCPath, "--remote-pool-credential", s.PoolCredentialPath, "--remote-admission-credential", s.AdmissionCredentialPath}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatal("helper route changed", cmd.Args)
	}
}
