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

const dailyBridgeReference2SHA = "65bc7cb4119e2dabae9260112745ef3fd4cd0541398b209c6b0816acf29c1e8a"

type dailyBridgeReference2 struct {
	Selection struct {
		Remote pilot.RecoverableRemotePoolConfig `json:"remote"`
	} `json:"selection"`
	Cases []struct {
		Name     string `json:"name"`
		Valid    bool   `json:"valid"`
		FrameHex string `json:"frame_hex"`
	} `json:"cases"`
}

func dailyBridgeReference2At(path string) (dailyBridgeReference2, error) {
	var r dailyBridgeReference2
	raw, e := os.ReadFile(path)
	if e != nil {
		return r, e
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != dailyBridgeReference2SHA {
		return r, ErrUnavailable
	}
	e = json.Unmarshal(raw, &r)
	return r, e
}
func dailyBridgeCommand2(t *testing.T, mode string) (*exec.Cmd, pilot.RecoverableRemotePoolConfig) {
	t.Helper()
	path, e := filepath.Abs("../../pilot/testdata/daily-wallet-activity/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	r, e := dailyBridgeReference2At(path)
	if e != nil {
		t.Fatal(e)
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(exe, "-test.run=^TestDailyWallet2BridgeFixtureProcess$")
	cmd.Env = append(os.Environ(), "SLUG_DAILY_WALLET2_PIPE_FIXTURE=1", "SLUG_DAILY_WALLET2_REFERENCE="+path, "SLUG_DAILY_WALLET2_MODE="+mode)
	return cmd, r.Selection.Remote
}

// A synthetic protocol process using independent public frames. This proves
// pipe ownership/correlation only, never real wallet/session/HTTP acceptance.
func TestDailyWallet2BridgeFixtureProcess(t *testing.T) {
	if os.Getenv("SLUG_DAILY_WALLET2_PIPE_FIXTURE") != "1" {
		return
	}
	signal.Ignore(syscall.SIGPIPE)
	r, e := dailyBridgeReference2At(os.Getenv("SLUG_DAILY_WALLET2_REFERENCE"))
	if e != nil {
		os.Exit(21)
	}
	mode := os.Getenv("SLUG_DAILY_WALLET2_MODE")
	active := false
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 2048), pilot.MaxDailyWalletRequestSize)
	for reader.Scan() {
		q, e := pilot.ParseDailyWalletRequest2(reader.Bytes(), r.Selection.Remote)
		if e != nil {
			os.Exit(22)
		}
		name := "wire2-control-" + q.Command + "-result"
		if q.Command == "snapshot" && !active {
			name = "wire2-initial-signed-out-result"
		}
		switch q.Command {
		case "wallet_activity_begin":
			name = "activity-begin-working-empty"
		case "wallet_activity_read":
			name = "activity-read-done-retained-empty"
		case "wallet_activity_detail":
			name = "activity-detail-zero256-detail-response"
		case "wallet_activity_cancel":
			name = "activity-cancel-retains-original"
		case "daily_payout_begin":
			name = "wire2-begin-working"
		case "daily_payout_read":
			name = "wire2-fresh-empty-nonfinal"
		case "daily_payout_cancel":
			name = "wire2-completed-cancel-keeps-capture"
		}
		var out pilot.DailyWalletResponse2
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
		if out.ActivityJob != nil {
			out.ActivityJob.Sequence = q.Sequence
		}
		if out.ActivityRead != nil {
			out.ActivityRead.Job.Sequence = q.Sequence
			if out.ActivityRead.Job.State == "cancelled" {
				out.ActivityRead.Capture = nil
			}
		}
		if q.Command == "login_fixture" {
			active = true
		}
		if q.Command == "logout" {
			active = false
		}
		if q.Command == "wallet_activity_begin" {
			switch mode {
			case "lost":
				_, _ = io.Copy(io.Discard, os.Stdin)
				os.Exit(0)
			case "wrong-id":
				out.ID++
			case "busy":
				out.OK = false
				out.ActivityJob = nil
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
func dailyBridgeJoin2(t *testing.T, b *DailyWalletBridge2) {
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
func TestDailyWallet2BridgeOriginalProcessCaptureAndOpaqueView(t *testing.T) {
	cmd, cfg := dailyBridgeCommand2(t, "normal")
	ctx, cancel := context.WithCancel(context.Background())
	b, e := startDailyWalletCommand2(ctx, cmd, cfg)
	cancel()
	if b != nil {
		t.Cleanup(func() { dailyBridgeJoin2(t, b) })
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
	for _, bad := range []*DailyWalletContext2{nil, &copy, {helper: &DailyWalletBridge2{}, view: v.View()}} {
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
	if _, e = b.BeginWalletActivityRead(context.Background(), v, 1); e != nil {
		t.Fatal(e)
	}
	own, e := b.WalletActivityRead(context.Background(), v, 1)
	if e != nil || own.ActivityRead == nil || own.ActivityRead.Capture == nil || own.ActivityRead.Capture.Sequence != 1 {
		t.Fatal("new activity original capture", e)
	}
	detailID := ""
	// Read the independent detail's selector, not a generated product value.
	path, e := filepath.Abs("../../pilot/testdata/daily-wallet-activity/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	ref, e := dailyBridgeReference2At(path)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range ref.Cases {
		if c.Name == "activity-detail-zero256-detail-response" && c.Valid {
			raw, e := hex.DecodeString(c.FrameHex)
			if e != nil {
				t.Fatal(e)
			}
			var r pilot.DailyWalletResponse2
			if json.Unmarshal(bytes.TrimSuffix(raw, []byte{'\n'}), &r) != nil || r.ActivityDetail == nil {
				t.Fatal("detail control")
			}
			detailID = r.ActivityDetail.Item.TxID
		}
	}
	if detailID == "" {
		t.Fatal("detail selector absent")
	}
	detail, e := b.WalletActivityDetail(context.Background(), v, 1, detailID)
	if e != nil || detail.ActivityDetail == nil || detail.ActivityDetail.Item.TxID != detailID {
		t.Fatal("original selected detail", e)
	}
	if _, e = b.CancelWalletActivityRead(context.Background(), v, 2); e != nil {
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
func TestDailyWallet2BridgeLostBeginRetiresWithoutReplacement(t *testing.T) {
	for _, mode := range []string{"lost", "wrong-id", "busy"} {
		t.Run(mode, func(t *testing.T) {
			cmd, cfg := dailyBridgeCommand2(t, mode)
			b, e := startDailyWalletCommand2(context.Background(), cmd, cfg)
			if b != nil {
				t.Cleanup(func() { dailyBridgeJoin2(t, b) })
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
			_, e = b.BeginWalletActivityRead(ctx, v, 1)
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
			if _, e = b.BeginWalletActivityRead(context.Background(), v, 2); e == nil || b.nextID != before {
				t.Fatal("new request after uncertain original")
			}
		})
	}
}
func TestDailyWallet2BridgeSelectedHelperArguments(t *testing.T) {
	s := RemotePoolLaunchSelection{ExistingRoot: "/public/original", AttachmentRoot: "/public/attachment", ConfigPath: "/public/config", NodeRPCPath: "/public/rpc", PoolCredentialPath: "/public/pool", AdmissionCredentialPath: "/public/admission"}
	cmd, e := dailyWalletLaunchCommand2("/public/slugpilot", s)
	if e != nil {
		t.Fatal(e)
	}
	want := []string{"/public/slugpilot", "--daily-recoverable-issue", "--daily-wallet-helper", "--data-dir", s.ExistingRoot, "--remote-attachment-root", s.AttachmentRoot, "--remote-config", s.ConfigPath, "--remote-node-rpc", s.NodeRPCPath, "--remote-pool-credential", s.PoolCredentialPath, "--remote-admission-credential", s.AdmissionCredentialPath}
	want = append(want, "--daily-wallet-helper-version=2")
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatal("helper route changed", cmd.Args)
	}
}
