package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Hand-written legacy RPC shape, independent of the new decoder. Transaction
// identity/outputs are covered by the separately derived transaction corpus.
const payoutRPCShape = `{"jsonrpc":"1.0","result":{"amount":1.25,"confirmations":1,"blockhash":"2222222222222222222222222222222222222222222222222222222222222222","blockindex":0,"blocktime":1795070792,"txid":"1111111111111111111111111111111111111111111111111111111111111111","walletconflicts":[],"time":1795070717,"timereceived":1795070717,"details":[],"hex":"00"},"error":null,"id":7}`
const payoutRPCMissing = `{"jsonrpc":"1.0","result":null,"error":{"code":-5,"message":"No information for transaction"},"id":7}`

func TestWalletPayoutRPCClosedShape(t *testing.T) {
	change := func(from, to string) string { return strings.Replace(payoutRPCShape, from, to, 1) }
	cases := []struct {
		name, raw  string
		ok, absent bool
	}{
		{"current-success", payoutRPCShape, true, false},
		{"exact-missing", payoutRPCMissing, true, true},
		{"other-error-code", strings.Replace(payoutRPCMissing, "-5", "-4", 1), false, false},
		{"other-error-text", strings.Replace(payoutRPCMissing, "No information", "Missing information", 1), false, false},
		{"missing-with-result", strings.Replace(payoutRPCMissing, `"result":null`, `"result":{}`, 1), false, false},
		{"wrong-id", change(`"id":7`, `"id":8`), false, false},
		{"string-id", change(`"id":7`, `"id":"7"`), false, false},
		{"exponent-id", change(`"id":7`, `"id":7e0`), false, false},
		{"other-version", change(`"1.0"`, `"2.0"`), false, false},
		{"missing-version", change(`"jsonrpc":"1.0",`, ``), false, false},
		{"duplicate-envelope", change(`"id":7`, `"id":7,"id":7`), false, false},
		{"duplicate-nested", change(`"amount":1.25`, `"amount":1.25,"amount":1.25`), false, false},
		{"duplicate-escaped", change(`"amount":1.25`, `"amount":1.25,"\u0061mount":1.25`), false, false},
		{"unknown-empty-key", change(`"amount":1.25`, `"amount":1.25,"":0`), false, false},
		{"unknown-compound-key", change(`"amount":1.25`, `"amount":1.25,"amount confirmations":0`), false, false},
		{"unknown-case", change(`"amount":1.25`, `"amount":1.25,"Amount":1`), false, false},
		{"missing-required", change(`"amount":1.25,`, ``), false, false},
		{"negative-confirmations", change(`"confirmations":1`, `"confirmations":-1`), false, false},
		{"fraction-confirmations", change(`"confirmations":1`, `"confirmations":1.0`), false, false},
		{"exponent-confirmations", change(`"confirmations":1`, `"confirmations":1e0`), false, false},
		{"negative-zero", change(`"blockindex":0`, `"blockindex":-0`), false, false},
		{"confirmation-overflow", change(`"confirmations":1`, `"confirmations":2147483649`), false, false},
		{"zero-with-block", change(`"confirmations":1`, `"confirmations":0`), false, false},
		{"confirmed-without-block", change(strings.Repeat("2", 64), ""), false, false},
		{"non-finite-amount", change(`"amount":1.25`, `"amount":1e400`), false, false},
		{"ignored-finite-exponent", change(`"amount":1.25`, `"amount":1.25e-4`), true, false},
		{"ignored-long-finite", change(`"amount":1.25`, `"amount":0.`+strings.Repeat("0", 200)+`1`), true, false},
		{"null-optional-fee", change(`"amount":1.25`, `"amount":1.25,"fee":null`), false, false},
		{"finite-optional-fee", change(`"amount":1.25`, `"amount":1.25,"fee":-0.0001`), true, false},
		{"null-details", change(`"details":[]`, `"details":null`), false, false},
		{"null-conflicts", change(`"walletconflicts":[]`, `"walletconflicts":null`), false, false},
		{"uppercase-hex", change(`"hex":"00"`, `"hex":"AB"`), false, false},
		{"odd-hex", change(`"hex":"00"`, `"hex":"0"`), false, false},
		{"empty-hex", change(`"hex":"00"`, `"hex":""`), false, false},
		{"escaped-hex", change(`"hex":"00"`, `"hex":"\u0030\u0030"`), true, false},
		{"oversized-transaction", change(`"hex":"00"`, `"hex":"`+strings.Repeat("00", 100001)+`"`), false, false},
		{"trailing-value", payoutRPCShape + `{}`, false, false},
		{"invalid-utf8", change(`"details":[]`, "\"details\":\"\xff\""), false, false},
		{"oversized-body", strings.Repeat(" ", maxWalletPayoutResponse) + payoutRPCMissing, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx, absent, ok := decodeWalletPayoutRPC([]byte(tc.raw), 7)
			if ok != tc.ok || absent != tc.absent {
				t.Fatalf("valid=%v absent=%v; want %v %v", ok, absent, tc.ok, tc.absent)
			}
			if (!ok || absent) && (tx.txid != "" || tx.raw != nil || tx.confirmations != 0 || tx.blockHash != "") {
				t.Fatal("rejected/absent reply leaked partial transaction")
			}
		})
	}
}

func TestWalletPayoutRPCArrayLimits(t *testing.T) {
	detail := `{"account":"","amount":1,"category":"receive","vout":1}`
	array := func(item string, n int) string {
		return "[" + strings.TrimSuffix(strings.Repeat(item+",", n), ",") + "]"
	}
	for _, tc := range []struct {
		field, item string
		count       int
		ok          bool
	}{
		{"details", detail, 258, true}, {"details", detail, 259, false},
		{"walletconflicts", `"` + strings.Repeat("a", 64) + `"`, 256, true},
		{"walletconflicts", `"` + strings.Repeat("a", 64) + `"`, 257, false},
		{"details", strings.Replace(detail, `"account":""`, `"account":"`+strings.Repeat("a", 256)+`"`, 1), 1, true},
		{"details", strings.Replace(detail, `"account":""`, `"account":"`+strings.Repeat("a", 257)+`"`, 1), 1, false},
		{"details", strings.Replace(detail, `"vout":1`, `"vout":1,"fee":null`, 1), 1, false},
		{"details", strings.Replace(detail, `"vout":1`, `"vout":1,"vout":1`, 1), 1, false},
		{"details", strings.Replace(detail, `"vout":1`, `"vout":1,"involveswatchonly":false,"address":""`, 1), 1, true},
	} {
		raw := strings.Replace(payoutRPCShape, `"`+tc.field+`":[]`, `"`+tc.field+`":`+array(tc.item, tc.count), 1)
		if _, _, ok := decodeWalletPayoutRPC([]byte(raw), 7); ok != tc.ok {
			t.Fatalf("%s count%d valid=%v want%v", tc.field, tc.count, ok, tc.ok)
		}
	}
}

type payoutRoundTrip func(*http.Request) (*http.Response, error)

func (f payoutRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWalletPayoutHTTPMethodAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, encoding string
		status               int
		ok, missing          bool
	}{
		{"found", payoutRPCShape, "", 200, true, false},
		{"missing", payoutRPCMissing, "", 200, true, true},
		{"other-error", strings.Replace(payoutRPCMissing, "-5", "-4", 1), "", 200, false, false},
		{"redirect", payoutRPCMissing, "", 302, false, false},
		{"compressed", payoutRPCMissing, "gzip", 200, false, false},
		{"server-error", payoutRPCMissing, "", 500, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := newWalletHTTPClient("http://127.0.0.1:29332/", "fixture-user", "fixture-pass")
			client.nextID = 6
			client.client.Transport = payoutRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				u, p, ok := req.BasicAuth()
				if !ok || u != "fixture-user" || p != "fixture-pass" || req.Method != "POST" || req.Header.Get("Accept-Encoding") != "identity" {
					t.Fatal("wrong private request")
				}
				var request struct {
					Method string   `json:"method"`
					Params []string `json:"params"`
					ID     uint64   `json:"id"`
				}
				if json.NewDecoder(req.Body).Decode(&request) != nil || request.Method != "gettransaction" || request.ID != 7 || len(request.Params) != 1 || request.Params[0] != strings.Repeat("1", 64) {
					t.Fatal("arbitrary RPC request")
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": []string{"application/json"}, "Content-Encoding": []string{tc.encoding}, "Location": []string{"http://127.0.0.1:9/"}}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: req}, nil
			})
			_, missing, err := client.payoutTransaction(context.Background(), strings.Repeat("1", 64))
			if (err == nil) != tc.ok || missing != tc.missing || calls != 1 || client.callCounts()["gettransaction"] != 1 {
				t.Fatalf("err%v missing%v calls%d", err, missing, calls)
			}
		})
	}
}

func TestWalletPayoutMissingSyncAndCancellation(t *testing.T) {
	selection := walletPayoutSelection{strings.Repeat("1", 64), 0, 100000000, "0014" + strings.Repeat("3", 40)}
	tip := walletPayoutTip{true, 109, strings.Repeat("2", 64)}
	clock := func() time.Time { return time.UnixMilli(1795070792113) }
	for _, kind := range []string{"stable", "height-change", "same-height-reorg", "unsynced-before", "unsynced-after", "rpc-failure", "cancel-during-rpc", "clock-invalid"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			samples, calls := 0, 0
			sample := func() walletPayoutTip {
				samples++
				v := tip
				if kind == "unsynced-before" || samples == 2 && kind == "unsynced-after" {
					v.synced = false
				}
				if samples == 2 && kind == "height-change" {
					v.height++
				}
				if samples == 2 && kind == "same-height-reorg" {
					v.hash = strings.Repeat("4", 64)
				}
				return v
			}
			query := func(context.Context, string) (walletPayoutTransaction, bool, error) {
				calls++
				if kind == "cancel-during-rpc" {
					cancel()
				}
				if kind == "rpc-failure" {
					return walletPayoutTransaction{}, false, errors.New("fixture failure")
				}
				return walletPayoutTransaction{}, true, nil
			}
			now := clock
			if kind == "clock-invalid" {
				now = func() time.Time { return time.UnixMilli(0) }
			}
			got, err := observeWalletPayout(ctx, selection, sample, query, now)
			if kind == "stable" {
				want := walletPayoutRead{State: "not_observed", ObservedAtMS: 1795070792113, WalletHeight: 109, WalletTip: tip.hash}
				if err != nil || got != want {
					t.Fatalf("got%+v err%v", got, err)
				}
			} else if err == nil || got != (walletPayoutRead{}) {
				t.Fatalf("bad observation %+v %v", got, err)
			}
			if kind == "unsynced-before" {
				if calls != 0 {
					t.Fatal("queried unsynchronized wallet")
				}
			} else if calls != 1 {
				t.Fatal("RPC resampled")
			}
		})
	}
	owner := &walletRuntime{}
	owner.mu.Lock()
	got, err := owner.ObservePoolPayout(context.Background(), selection)
	owner.mu.Unlock()
	if !errors.Is(err, errWalletPayoutBusy) || got != (walletPayoutRead{}) {
		t.Fatal("wallet owner did not refuse busy")
	}
}
