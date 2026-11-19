// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
)

// This corpus was produced independently after the contract was frozen. Go
// consumes its literal expected frames; it never regenerates expected bytes.
func TestWithdrawalIndependentWireCorpus(t *testing.T) {
	raw, e := os.ReadFile("../../frontend/testdata/withdrawal-wire/wire-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "60fe12e20e0c842a43cfc8f41892a8fcf4a7c991592363b17f87a8180098af9c" {
		t.Fatal("independent corpus changed")
	}
	type row struct {
		Name    string                  `json:"name"`
		Kind    string                  `json:"kind"`
		Scope   string                  `json:"scope"`
		Frame   string                  `json:"frame"`
		SHA     string                  `json:"sha256"`
		Raw     string                  `json:"raw_hex"`
		Valid   bool                    `json:"valid"`
		Binding pilot.WithdrawalBinding `json:"expected_binding"`
	}
	var corpus struct {
		Positives []row `json:"positives"`
		Negatives []row `json:"negatives"`
		Raw       []row `json:"raw_cases"`
	}
	if e := json.Unmarshal(raw, &corpus); e != nil {
		t.Fatal(e)
	}
	if len(corpus.Positives) != 142 || len(corpus.Negatives) != 80 || len(corpus.Raw) != 9 {
		t.Fatal("incomplete independent corpus")
	}
	parse := func(kind string, frame []byte) ([]byte, error) {
		// The production line scanner removes one LF, not other whitespace.
		frame = bytes.TrimSuffix(frame, []byte{'\n'})
		if kind == "request" {
			v, e := pilot.ParseWithdrawalRequest(frame)
			if e != nil {
				return nil, e
			}
			return pilot.MarshalWithdrawalRequest(v)
		}
		v, e := pilot.ParseWithdrawalResponse(frame)
		if e != nil {
			return nil, e
		}
		return pilot.MarshalWithdrawalResponse(v)
	}
	for _, c := range corpus.Positives {
		t.Run("positive/"+c.Name, func(t *testing.T) {
			if fmt.Sprintf("%x", sha256.Sum256([]byte(c.Frame))) != c.SHA {
				t.Fatal("frame checksum mismatch")
			}
			got, e := parse(c.Kind, []byte(c.Frame))
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(append(got, '\n'), []byte(c.Frame)) {
				t.Fatal("canonical bytes differ from independent frame")
			}
		})
	}
	for _, c := range corpus.Negatives {
		t.Run("negative/"+c.Name, func(t *testing.T) {
			_, e := parse(c.Kind, []byte(c.Frame))
			if c.Scope == "shape" {
				if e == nil {
					t.Fatal("invalid shape accepted")
				}
				return
			}
			if c.Scope != "correlation" || e != nil {
				t.Fatal("expected well-shaped correlation case", e)
			}
			out, e := pilot.ParseWithdrawalResponse(bytes.TrimSuffix([]byte(c.Frame), []byte{'\n'}))
			if e != nil {
				t.Fatal(e)
			}
			b := &PoolBridge{&StableBridge{poolMode: true, withdrawalBinding: &c.Binding}}
			r := pilot.WithdrawalRequest{ID: out.ID, Version: 1, ViewID: c.Binding.View.ViewID}
			if o := out.Operation; o != nil {
				r.Command, r.OperationID = "withdrawal_operation", o.OperationID
			} else if v := out.Review; v != nil {
				r.Command, r.OperationID, r.ActionID, r.ReviewID, r.ReviewKind, r.DraftDigest, r.TxID = "withdrawal_review", v.OperationID, v.ActionID, v.ReviewID, v.Kind, v.Draft.DraftDigest, v.TxID
				b.withdrawalExpected = &withdrawalExpectation{request: r, binding: c.Binding}
			} else if a := out.Action; a != nil {
				r.Command, r.OperationID, r.ActionID = "withdrawal_action", a.OperationID, a.ActionID
				intent := r
				intent.Command = "withdrawal_" + a.Kind
				b.withdrawalExpected = &withdrawalExpectation{request: intent, binding: c.Binding}
			} else {
				t.Fatal("missing correlation payload")
			}
			if b.correlateWithdrawal(r, out) {
				t.Fatal("foreign original binding accepted")
			}
		})
	}
	for _, c := range corpus.Raw {
		t.Run("raw/"+c.Name, func(t *testing.T) {
			v, e := hex.DecodeString(c.Raw)
			if e != nil {
				t.Fatal(e)
			}
			_, e = parse(c.Kind, v)
			if (e == nil) != c.Valid {
				t.Fatalf("valid=%v error=%v", c.Valid, e)
			}
		})
	}
}
