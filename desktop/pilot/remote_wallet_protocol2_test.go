// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"strconv"
	"testing"
)

// Filled only from root's dual-interpreter qualification, never product output.
const dailyWalletReference2SHA = "65bc7cb4119e2dabae9260112745ef3fd4cd0541398b209c6b0816acf29c1e8a"

type dailyWalletCase2 struct {
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	Valid      bool            `json:"valid"`
	FrameHex   string          `json:"frame_hex"`
	FrameBytes int             `json:"frame_bytes"`
	SHA256     string          `json:"sha256"`
	Request    json.RawMessage `json:"request"`
}
type dailyWalletReference2 struct {
	Version      uint32                        `json:"version"`
	Selection    dailyWalletReferenceSelection `json:"selection"`
	Cases        []dailyWalletCase2            `json:"cases"`
	ContextCases []struct {
		Name      string             `json:"name"`
		Capture   DailyWalletCapture `json:"capture"`
		Selectors []json.RawMessage  `json:"selectors"`
		Valid     bool               `json:"valid"`
	} `json:"context_cases"`
}

func dailyWalletTestReference2(t *testing.T) dailyWalletReference2 {
	t.Helper()
	if wire.TestPoW {
		t.Skip("positive independently qualified normal-profile codec")
	}
	raw, e := os.ReadFile("testdata/daily-wallet-activity/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != dailyWalletReference2SHA {
		t.Fatal("independent WIRE2 reference pin")
	}
	var r dailyWalletReference2
	if e := json.Unmarshal(raw, &r); e != nil {
		t.Fatal("independent reference decode", e)
	}
	if r.Version != 1 || len(r.Cases) == 0 {
		t.Fatal("reference shape", r.Version, len(r.Cases))
	}
	return r
}
func dailyWalletFrame2(t *testing.T, c dailyWalletCase2) []byte {
	t.Helper()
	raw, e := hex.DecodeString(c.FrameHex)
	if e != nil || len(raw) != c.FrameBytes {
		t.Fatal(c.Name, "frame size", e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != c.SHA256 {
		t.Fatal(c.Name, "frame hash")
	}
	return raw
}
func TestDailyWallet2IndependentCorpus(t *testing.T) {
	r := dailyWalletTestReference2(t)
	for _, c := range r.Cases {
		t.Run(c.Name, func(t *testing.T) {
			raw := dailyWalletFrame2(t, c)
			framed := len(raw) > 1 && raw[len(raw)-1] == '\n' && bytes.IndexByte(raw[:len(raw)-1], '\n') < 0 && bytes.IndexByte(raw, '\r') < 0
			accepted := false
			if framed {
				body := raw[:len(raw)-1]
				switch c.Kind {
				case "request":
					q, e := ParseDailyWalletRequest2(body, r.Selection.Remote)
					accepted = e == nil
					if accepted && c.Valid {
						encoded, e := MarshalDailyWalletRequest2(q, r.Selection.Remote)
						if e != nil || !bytes.Equal(encoded, body) {
							t.Fatal("canonical request changed", e)
						}
					}
				case "response":
					var request DailyWalletRequest2
					if e := json.Unmarshal(c.Request, &request); e != nil {
						t.Fatal("response companion request", c.Name, e)
					}
					var prior *RemotePoolView
					if request.ViewID != "" {
						v := r.Selection.View
						prior = &v
					}
					out, e := ParseDailyWalletResponse2(body, request, r.Selection.Remote, prior)
					accepted = e == nil
					if accepted && c.Valid {
						encoded, e := json.Marshal(out)
						if e != nil || !bytes.Equal(encoded, body) {
							t.Fatal("canonical response changed", e)
						}
					}
				default:
					t.Fatal("case kind", c.Kind)
				}
			}
			if accepted != c.Valid {
				t.Fatalf("accepted=%v expected=%v", accepted, c.Valid)
			}
		})
	}
}
func TestDailyWallet2IndependentRetainedPaymentMembership(t *testing.T) {
	r := dailyWalletTestReference2(t)
	if len(r.ContextCases) != 8 {
		t.Fatal("complete contextual case inventory")
	}
	for _, v := range r.ContextCases {
		t.Run(v.Name, func(t *testing.T) {
			if len(v.Selectors) != 4 {
				t.Fatal("selectors")
			}
			var ss, bs, tx string
			var index uint32
			if json.Unmarshal(v.Selectors[0], &ss) != nil || json.Unmarshal(v.Selectors[1], &bs) != nil || json.Unmarshal(v.Selectors[2], &tx) != nil || json.Unmarshal(v.Selectors[3], &index) != nil {
				t.Fatal("selector types")
			}
			seq, e := strconv.ParseUint(ss, 10, 64)
			if e != nil {
				t.Fatal(e)
			}
			batch, e := strconv.ParseUint(bs, 10, 64)
			if e != nil {
				t.Fatal(e)
			}
			b := v.Capture.Binding
			c := RemoteDailyPayoutCapture{Sequence: v.Capture.Sequence, Binding: RemoteDailyPayoutBinding{View: b.View, ConfigID: b.ConfigID, SourceBindingID: b.SourceBindingID, SettlementConfigID: b.SettlementConfigID, RecipientScript: b.RecipientScript, IssueProfileID: b.IssueProfileID, OriginalSourceBindingID: b.OriginalSourceBindingID, WalletOwnerID: b.WalletOwnerID, AllocationBindingID: b.AllocationBindingID}, CapturedAtMS: v.Capture.CapturedAtMS, Stale: v.Capture.Stale, First: v.Capture.First, Current: v.Capture.Current}
			p, e := selectedDailyPayment(&c, seq, batch, tx, index)
			if (e == nil) != v.Valid {
				t.Fatalf("selected=%v expected=%v: %v", e == nil, v.Valid, e)
			}
			if e == nil && (p.TxID != tx || p.OutputIndex != index) {
				t.Fatal("different original payment selected")
			}
		})
	}
}
func TestDailyWallet2PreservesWideLegacyMoney(t *testing.T) {
	r := dailyWalletTestReference(t)
	n := 0
	for _, c := range r.Cases {
		if !c.Valid || c.Kind != "response" || c.SelectionOverride != nil {
			continue
		}
		raw := dailyWalletTestFrame(t, c)
		var old DailyWalletResponse
		if json.Unmarshal(bytes.TrimSuffix(raw, []byte{'\n'}), &old) != nil {
			t.Fatal(c.Name)
		}
		if old.Read == nil || old.Read.Capture == nil {
			continue
		}
		b := old.Read.Capture.First.Balance
		if b.PaidAtoms <= int64(remotePoolWireMaxSafe) && b.ReservedAtoms <= int64(remotePoolWireMaxSafe) {
			continue
		}
		n++
		q := DailyWalletRequest2{Version: 2, Application: DailyWalletApplication2, ID: c.Request.ID, Command: c.Request.Command, ConfigID: c.Request.ConfigID, ViewID: c.Request.ViewID, Fixture: c.Request.Fixture, Sequence: c.Request.Sequence, CaptureSequence: c.Request.CaptureSequence, BeforeBatch: c.Request.BeforeBatch}
		out := DailyWalletResponse2{Version: 2, Application: DailyWalletApplication2, ID: old.ID, Command: old.Command, ConfigID: old.ConfigID, ViewID: old.ViewID, OK: old.OK, Snapshot: old.Snapshot, Job: old.Job, Read: old.Read, Error: old.Error}
		if e := ValidateDailyWalletResponse2(out, q, r.Selection.Remote, &r.Selection.View); e != nil {
			t.Fatal(c.Name, "wide original money lost", e)
		}
	}
	if n == 0 {
		t.Fatal("no independent wide money control")
	}
}
func TestDailyWallet2CheapProfileRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	if _, e := MarshalDailyWalletRequest2(DailyWalletRequest2{}, RecoverableRemotePoolConfig{}); e == nil {
		t.Fatal("cheap selected owner accepted")
	}
	if ValidateDailyWalletResponse2(DailyWalletResponse2{}, DailyWalletRequest2{}, RecoverableRemotePoolConfig{}, nil) == nil {
		t.Fatal("cheap response accepted")
	}
}
