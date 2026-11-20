// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

type operatorReferencePositive struct {
	Family, Name string
	Value        json.RawMessage
	CanonicalHex string `json:"canonical_hex"`
	CanonicalSHA string `json:"canonical_sha256"`
	IDs          map[string]string
}
type operatorReferenceCorpus struct {
	Version   uint32
	Positives []operatorReferencePositive
	Refusals  []struct {
		Name, Family    string
		ExpectedPin     PublicManifestPin `json:"expected_pin"`
		SelectedProfile uint32            `json:"selected_profile"`
		Value           json.RawMessage
	}
	RawCases []struct {
		Name            string
		SelectedProfile uint32 `json:"selected_profile"`
		RawHex          string `json:"raw_hex"`
		Valid           bool
	} `json:"raw_cases"`
	Guards []struct {
		Name, Filename  string
		ExpectedProfile uint32 `json:"expected_profile"`
		Value           publicGuard
		Accepted        bool
	}
	CheckpointMatrix []struct {
		Name        string
		ExpectedKey string `json:"expected_key"`
		Checkpoint  chaincfg.SignedCheckpoint
		Valid       bool
	} `json:"checkpoint_matrix"`
}

func readOperatorReference(t *testing.T) operatorReferenceCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/operator-reference/authority-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "7328ea2102c9ac993281bd732590c3e734eda26a3d180c2fa98bbd37da02c6b4" {
		t.Fatal("independent corpus changed")
	}
	var c operatorReferenceCorpus
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if c.Version != 1 || len(c.Positives) != 54 || len(c.Refusals) != 102 || len(c.RawCases) != 12 || len(c.Guards) != 12 || len(c.CheckpointMatrix) != 4 {
		t.Fatal("corpus inventory")
	}
	return c
}

// These tests consume the registry families only. Six finality-frame positives
// and ten frame refusals are inventoried but belong to fighterownership. None
// of these signature/codec checks claims full PoW, canonical replay or ownership.
func TestOperatorReferenceRegistryPositiveBytes(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal-chain positive codec vectors; cheap checkpoint refusals tested separately")
	}
	corpus := readOperatorReference(t)
	pins := map[string]PublicManifestPin{}
	for _, row := range corpus.Positives {
		if row.Family == "pin" {
			var p PublicManifestPin
			if e := json.Unmarshal(row.Value, &p); e != nil {
				t.Fatal(e)
			}
			pins[row.Name] = p
		}
	}
	registryCount, coreCount := 0, 0
	for _, row := range corpus.Positives {
		if row.Family == "frame" {
			coreCount++
			continue
		}
		registryCount++
		t.Run(row.Name+"/"+row.Family, func(t *testing.T) {
			raw, e := hex.DecodeString(row.CanonicalHex)
			if e != nil {
				t.Fatal(e)
			}
			h := sha256.Sum256(raw)
			if hex.EncodeToString(h[:]) != row.CanonicalSHA {
				t.Fatal("oracle canonical hash")
			}
			parts := strings.Split(row.Name, "/")
			p, ok := pins[strings.Join(parts[:2], "/")]
			if !ok {
				t.Fatal("pin absent")
			}
			out, ids, e := operatorReferenceRoundtrip(p.Version, p, row.Family, raw)
			if e != nil || !bytes.Equal(out, raw) || !reflect.DeepEqual(ids, row.IDs) {
				t.Fatal("canonical bytes/IDs", e, ids)
			}
		})
	}
	if registryCount != 48 || coreCount != 6 {
		t.Fatal("registry/core family inventory", registryCount, coreCount)
	}
}

func operatorReferenceRoundtrip(selected uint32, p PublicManifestPin, family string, raw []byte) ([]byte, map[string]string, error) {
	ids := map[string]string{}
	var codec OperatorCodec
	if selected != 1 && selected != 2 {
		return nil, nil, ErrPublicOwnership
	}
	if family == "pin" {
		if selected == 1 {
			v, e := ParsePublicManifestPin(raw)
			if e != nil {
				return nil, nil, e
			}
			id, e := PublicManifestPinID(v)
			if e != nil {
				return nil, nil, e
			}
			ids["pin_id"] = id
			b, e := MarshalPublicManifestPin(v)
			return b, ids, e
		}
		v, e := ParseOperatorProfile(raw)
		if e != nil {
			return nil, nil, e
		}
		ids["pin_id"] = v.ID()
		b, e := v.Marshal()
		return b, ids, e
	}
	if selected == 2 {
		profile, e := NewOperatorProfile(p)
		if e != nil {
			return nil, nil, e
		}
		codec, e = profile.Codec()
		if e != nil {
			return nil, nil, e
		}
	} else if e := ValidatePublicManifestPin(p); e != nil {
		return nil, nil, e
	}
	switch family {
	case "admission":
		var v PublicAdmissionStatement
		var e error
		if selected == 1 {
			v, e = ParsePublicAdmission(p, raw)
		} else {
			v, e = codec.ParseAdmission(raw)
		}
		if e != nil {
			return nil, nil, e
		}
		var id string
		var b []byte
		if selected == 1 {
			id, e = PublicAdmissionID(p, v)
			if e == nil {
				b, e = MarshalPublicAdmission(p, v)
			}
		} else {
			id, e = codec.AdmissionID(v)
			if e == nil {
				b, e = codec.MarshalAdmission(v)
			}
		}
		ids["admission_id"] = id
		return b, ids, e
	case "publication":
		var v PublicPublicationEntry
		var e error
		if selected == 1 {
			v, e = ParsePublicPublication(p, raw)
		} else {
			v, e = codec.ParsePublication(raw)
		}
		if e != nil {
			return nil, nil, e
		}
		var id string
		var b []byte
		if selected == 1 {
			id, e = PublicPublicationID(p, v)
			if e == nil {
				b, e = MarshalPublicPublication(p, v)
			}
		} else {
			id, e = codec.PublicationID(v)
			if e == nil {
				b, e = codec.MarshalPublication(v)
			}
		}
		ids["publication_id"] = id
		return b, ids, e
	case "boundary":
		var v PublicSignedBoundary
		var e error
		if selected == 1 {
			v, e = ParsePublicBoundary(p, raw)
		} else {
			v, e = codec.ParseBoundary(raw)
		}
		if e != nil {
			return nil, nil, e
		}
		var id string
		var b []byte
		if selected == 1 {
			id, e = PublicBoundaryID(p, v)
			if e == nil {
				b, e = MarshalPublicBoundary(p, v)
			}
		} else {
			id, e = codec.BoundaryID(v)
			if e == nil {
				b, e = codec.MarshalBoundary(v)
			}
		}
		ids["boundary_id"] = id
		return b, ids, e
	case "cutoff":
		var v PublicCutoffRecord
		var e error
		if selected == 1 {
			v, e = ParsePublicCutoff(p, raw)
		} else {
			v, e = codec.ParseCutoff(raw)
		}
		if e != nil {
			return nil, nil, e
		}
		var id string
		var b []byte
		if selected == 1 {
			id, e = PublicCutoffID(p, v)
			if e == nil {
				b, e = MarshalPublicCutoff(p, v)
			}
		} else {
			id, e = codec.CutoffID(v)
			if e == nil {
				b, e = codec.MarshalCutoff(v)
			}
		}
		ids["cutoff_id"] = id
		return b, ids, e
	case "checkpoint":
		var v chaincfg.SignedCheckpoint
		if e := json.Unmarshal(raw, &v); e != nil {
			return nil, nil, e
		}
		if e := validatePublicCheckpoint(p, v); e != nil {
			return nil, nil, e
		}
		b, e := v.SigningBytes()
		if e != nil {
			return nil, nil, e
		}
		ids["signing_bytes_hex"] = hex.EncodeToString(b)
		encoded, e := json.Marshal(v)
		return encoded, ids, e
	}
	return nil, nil, errors.New("unsupported registry reference family")
}

func TestOperatorReferenceRegistryRefusals(t *testing.T) {
	corpus := readOperatorReference(t)
	registryCount, coreCount := 0, 0
	for _, row := range corpus.Refusals {
		if row.Family == "frame" {
			coreCount++
			continue
		}
		registryCount++
		t.Run(row.Name, func(t *testing.T) {
			var compact bytes.Buffer
			if e := json.Compact(&compact, row.Value); e != nil {
				t.Fatal(e)
			}
			if _, _, e := operatorReferenceRoundtrip(row.SelectedProfile, row.ExpectedPin, row.Family, compact.Bytes()); e == nil {
				t.Fatal("cross-profile/shape refusal accepted")
			}
		})
	}
	if registryCount != 92 || coreCount != 10 {
		t.Fatal("refusal family inventory", registryCount, coreCount)
	}
	for _, row := range corpus.RawCases {
		t.Run(row.Name, func(t *testing.T) {
			raw, e := hex.DecodeString(row.RawHex)
			if e != nil {
				t.Fatal(e)
			}
			_, _, e = operatorReferenceRoundtrip(row.SelectedProfile, PublicManifestPin{}, "pin", raw)
			if (e == nil) != row.Valid {
				t.Fatal("raw canonical result", e)
			}
		})
	}
}

func TestOperatorReferenceGuardsCheckpointRoles(t *testing.T) {
	corpus := readOperatorReference(t)
	for _, row := range corpus.Guards {
		t.Run(row.Name, func(t *testing.T) {
			codec := publicEvidenceCodec{operator: row.ExpectedProfile == 2}
			valid := codec.validatePin(row.Value.Pin) == nil && codec.guard(row.Value.Pin) == row.Value && codec.markerName() == row.Filename
			if valid != row.Accepted {
				t.Fatal("guard projection")
			}
		})
	}
	for _, row := range corpus.CheckpointMatrix {
		t.Run(row.Name, func(t *testing.T) {
			key, e := hex.DecodeString(row.ExpectedKey)
			if e != nil {
				t.Fatal(e)
			}
			params := chaincfg.MainNetParams
			e = row.Checkpoint.Verify(&params, ed25519.PublicKey(key))
			if (e == nil) != row.Valid {
				t.Fatal("separate checkpoint role", e)
			}
		})
	}
}

// The normal-chain checkpoint vectors must not be accepted under the distinct
// cheap genesis. Keep the frozen inputs unchanged and assert the actual refusal.
func TestOperatorReferenceCheapCheckpointRefusals(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile refusal control")
	}
	corpus := readOperatorReference(t)
	pins := map[string]PublicManifestPin{}
	for _, row := range corpus.Positives {
		if row.Family == "pin" {
			var p PublicManifestPin
			if e := json.Unmarshal(row.Value, &p); e != nil {
				t.Fatal(e)
			}
			pins[row.Name] = p
		}
	}
	n := 0
	for _, row := range corpus.Positives {
		if row.Family != "checkpoint" && !(row.Family == "publication" && strings.HasSuffix(row.Name, "/checkpoint")) {
			continue
		}
		n++
		t.Run(row.Name+"/"+row.Family, func(t *testing.T) {
			parts := strings.Split(row.Name, "/")
			p, ok := pins[strings.Join(parts[:2], "/")]
			if !ok {
				t.Fatal("pin absent")
			}
			raw, e := hex.DecodeString(row.CanonicalHex)
			if e != nil {
				t.Fatal(e)
			}
			if _, _, e = operatorReferenceRoundtrip(p.Version, p, row.Family, raw); e == nil {
				t.Fatal("normal checkpoint accepted by cheap profile")
			}
		})
	}
	if n != 12 {
		t.Fatal("cheap checkpoint inventory", n)
	}
}
