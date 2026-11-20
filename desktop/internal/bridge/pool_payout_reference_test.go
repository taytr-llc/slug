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
func TestPoolPayoutIndependentWireCorpus(t *testing.T) {
	raw, e := os.ReadFile("../../frontend/testdata/payout-history-wire/wire-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "4b3eafc010d515557097357a968bd007571c7126f7c0f15a2cd4184bcc88f802" {
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
		Binding pilot.PoolPayoutBinding `json:"expected_binding"`
	}
	var corpus struct {
		Positives []row `json:"positives"`
		Negatives []row `json:"negatives"`
		Raw       []row `json:"raw_cases"`
	}
	if e := json.Unmarshal(raw, &corpus); e != nil {
		t.Fatal(e)
	}
	if len(corpus.Positives) != 72 || len(corpus.Negatives) != 61 || len(corpus.Raw) != 8 {
		t.Fatal("incomplete independent corpus")
	}
	parse := func(kind string, frame []byte) ([]byte, error) {
		// The production line scanner removes one LF, not other whitespace.
		frame = bytes.TrimSuffix(frame, []byte{'\n'})
		if kind == "request" {
			v, e := pilot.ParsePoolPayoutRequest(frame)
			if e != nil {
				return nil, e
			}
			return pilot.MarshalPoolPayoutRequest(v)
		}
		v, e := pilot.ParsePoolPayoutResponse(frame)
		if e != nil {
			return nil, e
		}
		return pilot.MarshalPoolPayoutResponse(v)
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
			out, e := pilot.ParsePoolPayoutResponse(bytes.TrimSuffix([]byte(c.Frame), []byte{'\n'}))
			if e != nil || out.Job == nil {
				t.Fatal(e)
			}
			b := &PoolBridge{&StableBridge{poolMode: true, payoutBinding: &c.Binding}}
			if e = b.RememberPoolPayout(c.Binding.View.ViewID, out.Job.JobID); e != nil {
				t.Fatal(e)
			}
			r := pilot.PoolPayoutRequest{ID: out.ID, Version: 1, Command: "pool_payout_job", ViewID: c.Binding.View.ViewID, JobID: out.Job.JobID}
			if b.correlatePoolPayout(r, out) {
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
