package pilot

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// Conditional protocol controller only; it cannot create a wallet capability.
type dailyWireController2 struct {
	*dailyWireTestController
	ownLast               RemoteWalletActivityJob
	ownBegins, ownCancels int
}

func (c *dailyWireController2) BeginWalletActivityRead(ctx context.Context, v string, s uint64) (RemoteWalletActivityJob, error) {
	if ctx.Err() != nil {
		return RemoteWalletActivityJob{}, ctx.Err()
	}
	if s == c.ownLast.Sequence && s != 0 {
		return c.ownLast, nil
	}
	if v != c.logged.View.ViewID || s != c.ownLast.Sequence+1 {
		return RemoteWalletActivityJob{}, ErrDailyPayoutReadInvalid
	}
	c.ownBegins++
	c.ownLast = RemoteWalletActivityJob{Sequence: s, State: "working"}
	return c.ownLast, nil
}
func (c *dailyWireController2) WalletActivityRead(ctx context.Context, v string, s uint64) (RemoteWalletActivityRead, error) {
	if ctx.Err() != nil {
		return RemoteWalletActivityRead{}, ctx.Err()
	}
	if v != c.logged.View.ViewID || s == 0 || s != c.ownLast.Sequence {
		return RemoteWalletActivityRead{}, ErrActivityNotFound
	}
	return RemoteWalletActivityRead{Job: c.ownLast}, nil
}
func (c *dailyWireController2) CancelWalletActivityRead(ctx context.Context, v string, s uint64) error {
	if _, e := c.WalletActivityRead(ctx, v, s); e != nil {
		return e
	}
	if c.ownLast.State == "working" {
		c.ownLast.State = "cancelled"
		c.ownCancels++
	}
	return nil
}
func (c *dailyWireController2) WalletActivityPage(context.Context, string, uint64, uint32, uint32) (RemoteWalletActivityPage, error) {
	return RemoteWalletActivityPage{}, ErrActivityNotFound
}
func (c *dailyWireController2) WalletActivityDetail(context.Context, string, uint64, string) (RemoteWalletActivityDetail, error) {
	return RemoteWalletActivityDetail{}, ErrActivityNotFound
}
func (c *dailyWireController2) ObserveDailyPayout(context.Context, string, uint64, uint64, string, uint32) (RemoteDailyPayoutObservation, error) {
	return RemoteDailyPayoutObservation{}, ErrDailyPayoutReadNotFound
}
func dailyWireFixture2(t *testing.T) (*dailyWireController2, dailyWalletReference) {
	t.Helper()
	r := dailyWalletTestReference(t)
	initial, logged := dailyWalletTestSnapshots(t, r)
	return &dailyWireController2{dailyWireTestController: &dailyWireTestController{remoteWireTestController: &remoteWireTestController{initial: initial, logged: logged, current: initial, stopped: logged}}}, r
}
func TestDailyWallet2PipeSequenceCancelAndNoImplicitStart(t *testing.T) {
	c, r := dailyWireFixture2(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	input, send := io.Pipe()
	receive, output := io.Pipe()
	done := make(chan error, 1)
	joined := false
	go func() { done <- ServeDailyWallet2(ctx, input, output, c, r.Selection.Remote) }()
	defer func() {
		cancel()
		_ = send.Close()
		_ = receive.Close()
		if !joined {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("helper join")
			}
		}
	}()
	reader := bufio.NewReader(receive)
	var counter uint64
	var prior *RemotePoolView
	call := func(command string, seq uint64) DailyWalletResponse2 {
		t.Helper()
		counter++
		q := DailyWalletRequest2{Version: 2, Application: DailyWalletApplication2, ID: counter, Command: command, ConfigID: r.Selection.ConfigID, Sequence: seq}
		if prior != nil {
			q.ViewID = prior.ViewID
		}
		if command == "login_fixture" {
			q.Fixture = r.Selection.Remote.Base.ExpectedLocal.Recipient.Config.Fixture
		}
		raw, e := MarshalDailyWalletRequest2(q, r.Selection.Remote)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = send.Write(append(raw, '\n')); e != nil {
			t.Fatal(e)
		}
		line, e := reader.ReadBytes('\n')
		if e != nil {
			t.Fatal(e)
		}
		out, e := ParseDailyWalletResponse2(line[:len(line)-1], q, r.Selection.Remote, prior)
		if e != nil {
			t.Fatal(e)
		}
		if out.OK && out.Snapshot != nil {
			prior = out.Snapshot.View
		}
		return out
	}
	call("snapshot", 0)
	call("login_fixture", 0)
	for i := 0; i < 2; i++ {
		out := call("wallet_activity_begin", 1)
		if !out.OK || out.ActivityJob.State != "working" {
			t.Fatal(out)
		}
	}
	if out := call("wallet_activity_read", 1); !out.OK || out.ActivityRead.Job.Sequence != 1 {
		t.Fatal(out)
	}
	if out := call("wallet_activity_cancel", 1); !out.OK || out.ActivityRead.Job.State != "cancelled" {
		t.Fatal(out)
	}
	if out := call("wallet_activity_read", 2); out.OK || out.Error != "not_found" {
		t.Fatal("adopted latest", out)
	}
	call("logout", 0)
	if e := send.Close(); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		joined = true
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("original owner did not join")
	}
	if c.ownBegins != 1 || c.ownCancels != 1 || c.starts != 0 || c.closes != 1 {
		t.Fatal("duplicate effect, implicit Start or wrong owner join")
	}
}
func TestDailyWallet2WrongViewAndLegacyFormatRefuse(t *testing.T) {
	c, r := dailyWireFixture2(t)
	c.current = c.logged
	q := DailyWalletRequest2{Version: 2, Application: DailyWalletApplication2, ID: 2, Command: "wallet_activity_begin", ConfigID: r.Selection.ConfigID, ViewID: r.Selection.View.ViewID, Sequence: 1}
	q.ViewID = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	out, e := dailyWalletStep2(context.Background(), q, c, r.Selection.Remote, &r.Selection.View)
	if e != nil || out.OK || out.Error != "stale_view" || c.ownBegins != 0 {
		t.Fatal("foreign View before dispatch", e, out)
	}
	old := dailyWalletRequest1(q)
	old.Command = "daily_payout_begin"
	raw, e := json.Marshal(old)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ParseDailyWalletRequest2(raw, r.Selection.Remote); e == nil {
		t.Fatal("legacy wire accepted")
	}
	raw, e = json.Marshal(q)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ParseDailyWalletRequest(raw, r.Selection.Remote); e == nil {
		t.Fatal("new wire adopted by old parser")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
}
