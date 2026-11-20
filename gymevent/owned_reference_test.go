package gymevent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymentry"
	"os"
	"testing"
)

func TestOwnedEventIndependentComposition(t *testing.T) {
	raw, e := os.ReadFile("../gymstable/testdata/owned-reference/owned-consumers-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "44f3d12b96a94f6d2d071270b6c90296cae1db50249ff4725ec5e28adc9bfa15" {
		t.Fatal("independent corpus changed")
	}
	type row struct {
		Family, Name, Context string
		Value                 json.RawMessage
		CanonicalHex          string `json:"canonical_hex"`
		SHA256                string
	}
	var corpus struct {
		Contexts             map[string]json.RawMessage
		Positives, Negatives []row
	}
	if e = json.Unmarshal(raw, &corpus); e != nil {
		t.Fatal(e)
	}
	var context struct{ Pin OwnedPin }
	if e = json.Unmarshal(corpus.Contexts["event"], &context); e != nil {
		t.Fatal(e)
	}
	positive, negative := 0, 0
	for _, v := range corpus.Positives {
		if v.Family != "snapshot" && v.Family != "entry_pin" {
			continue
		}
		positive++
		t.Run("positive/"+v.Family+"/"+v.Name, func(t *testing.T) {
			b, e := hex.DecodeString(v.CanonicalHex)
			if e != nil {
				t.Fatal(e)
			}
			var out []byte
			if v.Family == "entry_pin" {
				p, e := gymentry.ParseOwnedPin(b)
				if e != nil {
					t.Fatal(e)
				}
				out, e = gymentry.MarshalOwnedPin(p)
				if e != nil {
					t.Fatal(e)
				}
			} else {
				s, e := ParseOwnedSnapshotInput(context.Pin, b)
				if e != nil {
					t.Fatal(e)
				}
				out, e = MarshalOwnedSnapshotInput(context.Pin, s)
				if e != nil {
					t.Fatal(e)
				}
			}
			if !bytes.Equal(out, b) {
				t.Fatal("independent bytes differ")
			}
		})
	}
	for _, v := range corpus.Negatives {
		if v.Family != "snapshot" && v.Family != "entry_pin" {
			continue
		}
		negative++
		t.Run("negative/"+v.Name, func(t *testing.T) {
			var compact bytes.Buffer
			if e := json.Compact(&compact, v.Value); e != nil {
				t.Fatal(e)
			}
			var err error
			if v.Family == "entry_pin" {
				_, err = gymentry.ParseOwnedPin(compact.Bytes())
			} else {
				_, err = ParseOwnedSnapshotInput(context.Pin, compact.Bytes())
			}
			if err == nil {
				t.Fatal("invalid composition accepted")
			}
		})
	}
	if positive != 3 || negative != 8 {
		t.Fatalf("scope %d/%d", positive, negative)
	}
}
