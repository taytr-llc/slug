package pilot

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/taytr-llc/slugwallet/waddrmgr"
)

func TestRestoredGymIndependentSetupCodec(t *testing.T) {
	raw, err := os.ReadFile("testdata/restored-gym/reference/setup-v1.json")
	if err != nil || restoredSHA(raw) != "3d31e7f195fa20456978739258f5517fbe2a4d97d93b379c9b8ade941daa3147" {
		t.Fatal("independent reference hash", err)
	}
	type row struct {
		Name, Context, Kind string
		Value               json.RawMessage
		FileHex             string `json:"file_hex"`
		FileSHA256          string `json:"file_sha256"`
		CanonicalHex        string `json:"canonical_hex"`
		Digest              *string
		RawHex              string `json:"raw_hex"`
		ExpectedOverride    *struct {
			NodeBinding restoredNodeBinding `json:"node_binding"`
		} `json:"expected_override"`
	}
	var corpus struct {
		Contexts []struct {
			Name     string
			Expected json.RawMessage
			Init     restoredSetupInit
			Activate restoredSetupActivate
		}
		Positives, Negatives []row
		RawCases             []row `json:"raw_cases"`
	}
	if err = json.Unmarshal(raw, &corpus); err != nil || len(corpus.Positives) != 24 || len(corpus.Negatives) != 54 || len(corpus.RawCases) != 48 {
		t.Fatal("reference shape", err)
	}
	type context struct {
		initial  restoredSetupInit
		activate restoredSetupActivate
		guard    waddrmgr.RestoredCarrierGuard
	}
	contexts := map[string]context{}
	for _, c := range corpus.Contexts {
		var expected struct{ Guard waddrmgr.RestoredCarrierGuard }
		if err = json.Unmarshal(c.Expected, &expected); err != nil {
			t.Fatal(err)
		}
		contexts[c.Name] = context{c.Init, c.Activate, expected.Guard}
	}
	check := func(r row, data []byte) (any, error) {
		c, ok := contexts[r.Context]
		if !ok {
			t.Fatal("missing reference context")
		}
		if r.ExpectedOverride != nil {
			c.initial.NodeBinding = r.ExpectedOverride.NodeBinding
		}
		switch r.Kind {
		case "init":
			var v restoredSetupInit
			if e := restoredParseMarker(data, restoredInitLimit, &v); e != nil {
				return v, e
			}
			return v, restoredValidateInit(v, &c.initial)
		case "activate":
			var v restoredSetupActivate
			if e := restoredParseMarker(data, restoredActivateLimit, &v); e != nil {
				return v, e
			}
			return v, restoredValidateActivate(v, c.initial, &c.guard)
		case "ready":
			var v restoredSetupReady
			if e := restoredParseMarker(data, restoredReadyLimit, &v); e != nil {
				return v, e
			}
			return v, restoredValidateReady(v, c.initial, c.activate)
		default:
			t.Fatal("unknown reference kind")
			return nil, ErrRestoredGym
		}
	}
	for _, r := range corpus.Positives {
		t.Run(r.Name, func(t *testing.T) {
			data, e := hex.DecodeString(r.FileHex)
			if e != nil || restoredSHA(data) != r.FileSHA256 {
				t.Fatal("reference file", e)
			}
			v, e := check(r, data)
			if e != nil {
				t.Fatal(e)
			}
			encoded, e := restoredCanonical(v)
			if e != nil || hex.EncodeToString(encoded) != r.CanonicalHex {
				t.Fatal("canonical encoding differs", e)
			}
			if r.Digest != nil {
				kind := "setup"
				if r.Kind == "activate" {
					kind = "activation"
				}
				if restoredDigest(kind, v) != *r.Digest {
					t.Fatal("digest differs")
				}
			}
		})
	}
	for _, r := range corpus.Negatives {
		t.Run(r.Name, func(t *testing.T) {
			var data bytes.Buffer
			if err := json.Compact(&data, r.Value); err != nil {
				t.Fatal(err)
			}
			if _, err := check(r, append(data.Bytes(), '\n')); err == nil {
				t.Fatal("semantic mutant accepted")
			}
		})
	}
	for _, r := range corpus.RawCases {
		t.Run(r.Name, func(t *testing.T) {
			data, err := hex.DecodeString(r.RawHex)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := check(r, data); err == nil {
				t.Fatal("malformed stored bytes accepted")
			}
		})
	}
}
