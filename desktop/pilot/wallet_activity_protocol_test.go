// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"github.com/taytr-llc/slug/gymstable"
	"strings"
	"testing"
)

type activityWireController struct {
	PoolControllerAPI
	calls int
}

func (*activityWireController) Stop()        {}
func (*activityWireController) Close() error { return nil }
func (c *activityWireController) WalletActivitySnapshot(context.Context) (WalletActivitySnapshot, error) {
	c.calls++
	return BlankWalletActivitySnapshot(), gymstable.ErrUnauthorized
}
func (c *activityWireController) BeginWalletActivity(context.Context, string, string) (WalletActivityJob, error) {
	c.calls++
	return WalletActivityJob{Version: 1, JobID: strings.Repeat("f", 64)}, gymstable.ErrUnauthorized
}
func (c *activityWireController) WalletActivityJob(context.Context, string, string) (WalletActivityJob, error) {
	c.calls++
	return WalletActivityJob{}, gymstable.ErrUnauthorized
}
func (c *activityWireController) CancelWalletActivity(context.Context, string, string) (WalletActivityJob, error) {
	c.calls++
	return WalletActivityJob{}, gymstable.ErrUnauthorized
}
func (c *activityWireController) WalletActivityPage(context.Context, string, string, uint32, uint32) (WalletActivityPage, error) {
	c.calls++
	return WalletActivityPage{}, gymstable.ErrUnauthorized
}
func (c *activityWireController) WalletActivityDetail(context.Context, string, string, string) (WalletActivityDetail, error) {
	c.calls++
	return WalletActivityDetail{}, gymstable.ErrUnauthorized
}
func TestWalletActivityHelperClosedRoutingAndNoErrorPayload(t *testing.T) {
	for _, cmd := range []string{"snapshot", "begin", "job", "page", "detail", "cancel"} {
		t.Run(cmd, func(t *testing.T) {
			r := WalletActivityRequest{ID: 1, Version: 1, Command: "wallet_activity_" + cmd}
			if cmd != "snapshot" {
				r.ViewID = strings.Repeat("a", 64)
				r.JobID = strings.Repeat("b", 64)
			}
			if cmd == "page" {
				r.Limit = 8
			}
			if cmd == "detail" {
				r.TxID = strings.Repeat("c", 64)
			}
			body, e := MarshalWalletActivityRequest(r)
			if e != nil {
				t.Fatal(e)
			}
			for _, invalid := range []bool{false, true} {
				raw := append([]byte(nil), body...)
				if invalid {
					raw = append([]byte(`{"rpc_method":"gettransaction",`), raw[1:]...)
				}
				c := &activityWireController{}
				var out bytes.Buffer
				if e = ServePool(t.Context(), bytes.NewReader(append(raw, '\n')), &out, c); e != nil {
					t.Fatal(e)
				}
				response, e := ParseWalletActivityResponse(out.Bytes())
				if e != nil {
					t.Fatal(e)
				}
				expected := "unauthorized"
				calls := 1
				if invalid {
					expected = "invalid_request"
					calls = 0
				}
				if response.OK || response.Error != expected || response.Snapshot != nil || response.Job != nil || response.Page != nil || response.Detail != nil || c.calls != calls {
					t.Fatal("unclosed routing or disclosed failure payload", response, c.calls)
				}
			}
		})
	}
}
