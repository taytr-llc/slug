// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/internal/careerstate"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type downstreamCase struct {
	Family       string `json:"family"`
	Name         string `json:"name"`
	Scope        string `json:"scope"`
	CanonicalHex string `json:"canonical_hex"`
	SHA256       string `json:"sha256"`
	MessageHex   string `json:"message_hex"`
	PublicKey    string `json:"public_key"`
	Digest       string `json:"digest"`
	PreimageHex  string `json:"preimage_hex"`
	FileHex      string `json:"file_hex"`
	FileSHA256   string `json:"file_sha256"`
	PayloadHex   string `json:"payload_hex"`
	Value        json.RawMessage
	RawHex       string `json:"raw_hex"`
	Valid        bool
}
type downstreamReference struct {
	Contexts struct {
		Pins     map[string]json.RawMessage
		EventPin gymevent.OwnedPin `json:"event_pin"`
	}
	Positives               []downstreamCase `json:"codec_positives"`
	Compositions, Negatives []downstreamCase
	Raw                     []downstreamCase `json:"raw_cases"`
}

func downstreamRef(t *testing.T) downstreamReference {
	t.Helper()
	raw, e := os.ReadFile("../gymseed/testdata/owned-reference/owned-downstream-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "018049df53a0e381bbd6233d58e6438efc9f159bc56fffe9b4dd75fb4ed75a2c" {
		t.Fatal("independent downstream reference changed")
	}
	var r downstreamReference
	if e = json.Unmarshal(raw, &r); e != nil {
		t.Fatal(e)
	}
	return r
}
func downstreamBytes(t *testing.T, c downstreamCase) []byte {
	t.Helper()
	b, e := hex.DecodeString(c.CanonicalHex)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != c.SHA256 {
		t.Fatal("reference case digest")
	}
	return b
}
func downstreamComposition(t *testing.T, r downstreamReference, family, name string) downstreamCase {
	t.Helper()
	for _, c := range r.Compositions {
		if c.Family == family && c.Name == name {
			return c
		}
	}
	t.Fatalf("missing %s/%s", family, name)
	return downstreamCase{}
}

func TestOwnedResultIndependentComposition(t *testing.T) {
	r := downstreamRef(t)
	positive, negative := 0, 0
	for _, c := range append(append([]downstreamCase{}, r.Positives...), r.Negatives...) {
		if c.Family != "result_pin" {
			continue
		}
		b := downstreamBytes(t, c)
		v, e := ParseOwnedPin(b)
		if c.Name == "candidate" {
			positive++
			if e != nil {
				t.Fatal(e)
			}
			out, e := MarshalOwnedPin(v)
			if e != nil || !bytes.Equal(out, b) {
				t.Fatal("pin bytes")
			}
		} else {
			negative++
			if e == nil {
				t.Fatalf("accepted %s", c.Name)
			}
		}
	}
	if positive != 1 || negative != 7 {
		t.Fatalf("coverage %d/%d", positive, negative)
	}
	// This supplied source is deliberately only a byte/signature composition.
	// It is not passed to ValidateOwnedResultSource or a full statement validator.
	c := downstreamComposition(t, r, "signature", "result statement")
	var signed OwnedSignedStatement
	if e := json.Unmarshal(c.Value, &signed); e != nil {
		t.Fatal(e)
	}
	message, _ := hex.DecodeString(c.MessageHex)
	pub, _ := hex.DecodeString(c.PublicKey)
	sig, _ := hex.DecodeString(signed.Signature)
	if !bytes.Equal(ownedStatementMessage(signed.Statement), message) || !ed25519.Verify(pub, message, sig) {
		t.Fatal("owned decoded signature message")
	}
	body := downstreamComposition(t, r, "domain", "result statement")
	id, e := OwnedStatementID(signed.Statement)
	if e != nil || id != body.Digest {
		t.Fatalf("statement composition %v", e)
	}
	payload, e := OwnedCarrier(signed.Statement)
	want := downstreamComposition(t, r, "carrier", "result")
	if e != nil || hex.EncodeToString(payload[:]) != want.PayloadHex {
		t.Fatalf("payload composition %v", e)
	}
	n := 0
	for _, c := range r.Negatives {
		if c.Family != "signature" || !strings.HasPrefix(c.Name, "result statement/") {
			continue
		}
		n++
		var bad OwnedSignedStatement
		if e := json.Unmarshal(c.Value, &bad); e != nil {
			t.Fatal(e)
		}
		sig, _ := hex.DecodeString(bad.Signature)
		if ed25519.Verify(pub, ownedStatementMessage(bad.Statement), sig) {
			t.Fatalf("accepted wrong signature %s", c.Name)
		}
	}
	if n != 3 {
		t.Fatal("signature mutation coverage")
	}
}
func TestOwnedResultCareerIndependentPrefixBytes(t *testing.T) {
	r := downstreamRef(t)
	c := downstreamComposition(t, r, "career_transition", "exact-prefixes")
	var v struct {
		V1              ownedMarker
		Intent          ownedCareerIntent
		Prepared, Ready ownedCareerMarker
		PolicyID        string `json:"policy_id"`
	}
	if e := json.Unmarshal(c.Value, &v); e != nil {
		t.Fatal(e)
	}
	a := v.Ready.Career
	if a == nil || careerstate.ValidateActivation(*a) != nil || a.PolicyID != CareerPolicyID() || a.ActivationID != careerstate.ActivationID(*a) {
		t.Fatal("activation recipe")
	}
	meta := ownedMetadata{StoreID: v.V1.StoreID, Pin: v.V1.Pin, CreatedAtMS: a.ResultCreatedAtMS, SourceCreatedAtMS: a.SourceCreatedAtMS}
	if ownedIntentFor(meta) != v.Intent {
		t.Fatal("intent includes exact old marker LF")
	}
	if plainSHA(encoded(v.Intent)) != v.Prepared.CareerIntentDigest || v.Prepared.CareerIntentDigest != v.Ready.CareerIntentDigest {
		t.Fatal("intent digest")
	}
	for _, m := range []any{v.V1, v.Prepared, v.Ready} {
		b, e := json.Marshal(m)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(markerBytes(m), append(b, '\n')) {
			t.Fatal("marker LF recipe")
		}
	}
	// Actual marker reader preserves the owned family and v1/v2 framing. Full
	// activation/open needs a concrete live bout source and is tested separately.
	for _, tc := range []struct {
		name   string
		marker any
	}{{"v1", v.V1}, {"prepared", v.Prepared}, {"ready", v.Ready}} {
		t.Run(tc.name, func(t *testing.T) {
			// The real marker reader rejects symlink ancestors; canonicalize the
			// platform temp root (macOS exposes /var as /private/var).
			d, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			if e := os.Mkdir(filepath.Join(d, "db"), 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(filepath.Join(d, ownedMarkerName), markerBytes(tc.marker), 0600); e != nil {
				t.Fatal(e)
			}
			_, _, got, _, e := ownedCareerFiles(d, v.V1.Pin)
			if e != nil || !bytes.Equal(got, markerBytes(tc.marker)) {
				t.Fatalf("owned marker reader %v", e)
			}
			legacy := bytes.Replace(got, []byte("slug-owned-result"), []byte("slug-result"), 1)
			if e = os.WriteFile(filepath.Join(d, ownedMarkerName), legacy, 0600); e != nil {
				t.Fatal(e)
			}
			if _, _, _, _, e = ownedCareerFiles(d, v.V1.Pin); e == nil {
				t.Fatal("legacy marker adopted")
			}
		})
	}
}
