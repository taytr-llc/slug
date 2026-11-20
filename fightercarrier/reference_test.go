// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const independentWalletSHA = "8648ac23d5e077df1a4dd1e3b8f07157922e650e92e9eea99c233a6993283495"

type referenceCase struct {
	Name, Kind, CanonicalUTF8, Digest string
	Value                             json.RawMessage
	Context                           struct {
		Pin       Pin    `json:"pin"`
		BindingID string `json:"binding_id"`
	}
	RawHex string `json:"raw_hex"`
	Valid  bool
}
type walletReference struct {
	Positives, Negatives []referenceCase
	RawCases             []referenceCase `json:"raw_cases"`
}

func readWalletReference(t *testing.T) walletReference {
	t.Helper()
	raw, e := os.ReadFile("testdata/reference/wallet-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != independentWalletSHA {
		t.Fatal("independent corpus changed")
	}
	var v walletReference
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	if len(v.Positives) != 363 || len(v.Negatives) != 166 || len(v.RawCases) != 12 {
		t.Fatal("reference counts")
	}
	return v
}
func compactReference(t *testing.T, raw json.RawMessage) []byte {
	t.Helper()
	var b bytes.Buffer
	if e := json.Compact(&b, raw); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func referencePreparation(t *testing.T, v walletReference, name string) Preparation {
	t.Helper()
	at := strings.LastIndexByte(name, '/')
	prefix := name[:at] + "/preparation"
	for _, p := range v.Positives {
		if p.Name == prefix {
			var out Preparation
			if e := json.Unmarshal(p.Value, &out); e != nil {
				t.Fatal(e)
			}
			return out
		}
	}
	t.Fatalf("missing preparation %s", prefix)
	return Preparation{}
}
func validateReference(t *testing.T, v walletReference, c referenceCase, raw []byte) (any, string, error, bool) {
	t.Helper()
	var digest string
	switch c.Kind {
	case "carrier_pin":
		var x Pin
		e := decodeJournal(raw, 4096, &x)
		if e == nil {
			e = ValidatePin(x)
		}
		return x, "", e, true
	case "carrier_request":
		x, e := ParseRequest(raw)
		if e == nil {
			digest, e = RequestDigest(x)
		}
		return x, digest, e, true
	case "review":
		x, e := ParseReview(raw)
		if e == nil {
			digest, e = ReviewDigest(x)
		}
		return x, digest, e, true
	case "preparation":
		x, e := ParsePreparation(raw, c.Context.Pin, c.Context.BindingID)
		if e == nil {
			digest, e = PreparationDigest(x)
		}
		return x, digest, e, true
	case "signed_package":
		p := referencePreparation(t, v, c.Name)
		x, e := ParseSignedPackage(raw, p, c.Context.Pin)
		if e == nil {
			digest, e = SignedPackageDigest(x)
		}
		return x, digest, e, true
	case "operation":
		x, e := ParseOperation(raw, c.Context.Pin, c.Context.BindingID)
		return x, "", e, true
	case "wallet_record":
		var x WalletRecord
		e := decodeJournal(raw, MaxWalletRecordBytes, &x)
		if e == nil {
			e = ValidateWalletRecord(x, c.Context.Pin, c.Context.BindingID)
		}
		return x, "", e, true
	case "authorization_request":
		var x AuthorizationRequest
		e := decodeJournal(raw, 4096, &x)
		if e == nil {
			p := referencePreparation(t, v, c.Name)
			want, err := AuthorizationFor(p)
			if err != nil {
				e = err
			} else if x != want {
				e = ErrInvalid
			}
		}
		return x, "", e, true
	}
	return nil, "", nil, false
}

func TestCarrierIndependentWalletCodecs(t *testing.T) {
	v := readWalletReference(t)
	tested := 0
	for _, c := range v.Positives {
		t.Run(c.Name, func(t *testing.T) {
			raw := compactReference(t, c.Value)
			got, d, e, ok := validateReference(t, v, c, raw)
			if !ok {
				return
			}
			tested++
			if e != nil {
				t.Fatal(e)
			}
			b, e := json.Marshal(got)
			if e != nil || !bytes.Equal(b, raw) {
				t.Fatal("canonical bytes differ")
			}
			if c.Digest != "" && d != c.Digest {
				t.Fatalf("digest mismatch: got %s expected %s", d, c.Digest)
			}
		})
	}
	if tested != 189 {
		t.Fatalf("unexpected coverage %d", tested)
	}
}
func TestCarrierIndependentRefusals(t *testing.T) {
	v := readWalletReference(t)
	tested := 0
	walletContext := 0
	for _, c := range v.Negatives {
		t.Run(c.Name, func(t *testing.T) {
			// These three controls require the retained original import and exact
			// manager public path. A parsed Pin contains only its digest. They are
			// intentionally assigned to concrete wallet tests, never silently passed.
			if strings.HasSuffix(c.Name, "/funding-swapped-branch") || strings.HasSuffix(c.Name, "/uncovered-funding") || strings.HasSuffix(c.Name, "/uncovered-return") {
				walletContext++
				return
			}
			_, _, e, ok := validateReference(t, v, c, compactReference(t, c.Value))
			if !ok {
				return
			}
			tested++
			if e == nil {
				t.Fatal("independent refusal accepted")
			}
		})
	}
	if tested != 113 || walletContext != 9 {
		t.Fatalf("coverage shared%d wallet-context%d", tested, walletContext)
	}
}
func TestCarrierIndependentCanonicalParser(t *testing.T) {
	v := readWalletReference(t)
	for _, c := range v.RawCases {
		t.Run(c.Name, func(t *testing.T) {
			b, e := hex.DecodeString(c.RawHex)
			if e != nil {
				t.Fatal(e)
			}
			_, e = ParseReview(b)
			if (e == nil) != c.Valid {
				t.Fatalf("accepted=%v expected=%v err=%v", e == nil, c.Valid, e)
			}
		})
	}
}

func TestCarrierBoundedFramingAndNilArrays(t *testing.T) {
	// Hostile count is refused by the prewalk, before wire can allocate it.
	for _, commit := range []bool{false, true} {
		b := []byte{1, 0, 0, 0}
		if commit {
			b = append(b, 0, 1)
		}
		b = append(b, 255, 255, 255, 255, 255, 255, 255, 255, 127)
		if txFrame(b, commit) {
			t.Fatal("unbounded input count")
		}
	}
	v := readWalletReference(t)
	for _, c := range v.Positives {
		if c.Kind == "preparation" && strings.Contains(c.Name, "/restored/transfer/") {
			var p Preparation
			if e := json.Unmarshal(c.Value, &p); e != nil {
				t.Fatal(e)
			}
			p.RestoredAllocation.InvalidIndices = nil
			if ValidatePreparation(p, c.Context.Pin, c.Context.BindingID) == nil {
				t.Fatal("nullable raw-range inventory")
			}
			break
		}
	}
}
