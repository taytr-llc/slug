// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospect

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func contextJSON(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(Context{
		Version: 1, Network: 1, Genesis: strings.Repeat("11", 32),
		DNAVersion: 1, ProspectTarget: strings.Repeat("ff", 32),
		ParentHash: strings.Repeat("22", 32), ParentHeight: 0,
		ExpectedBits: 0x207fffff, MinTimestamp: 1, MaxTimestamp: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func proofJSON(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(Proof{
		Version: 1, GymID: strings.Repeat("33", 32),
		Header: strings.Repeat("00", 80), Coinbase: "00",
		MerkleBranch: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func replaceJSONField(t *testing.T, data []byte, field string, value json.RawMessage) []byte {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	if value == nil {
		delete(object, field)
	} else {
		object[field] = value
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestParseJSONSchemaAndSizeLimits(t *testing.T) {
	context, err := ParseContext(contextJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	if context.Version != 1 || context.Network != 1 || context.ParentHeight != 0 ||
		context.Genesis != strings.Repeat("11", 32) || context.ExpectedBits != 0x207fffff {
		t.Fatalf("context fields decoded incorrectly: %+v", context)
	}
	proof, err := ParseProof(proofJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	if proof.Version != 1 || proof.GymID != strings.Repeat("33", 32) ||
		proof.MerkleBranch == nil || len(proof.MerkleBranch) != 0 {
		t.Fatalf("proof fields decoded incorrectly: %+v", proof)
	}
	for _, test := range []struct {
		name  string
		data  []byte
		max   int
		parse func([]byte) error
	}{
		{"context", contextJSON(t), MaxContextSize, func(data []byte) error { _, err := ParseContext(data); return err }},
		{"proof", proofJSON(t), MaxProofSize, func(data []byte) error { _, err := ParseProof(data); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Whitespace is part of the transport limit, even though a JSON
			// decoder would otherwise ignore it.
			atLimit := append([]byte(" \n"), test.data...)
			atLimit = append(atLimit, strings.Repeat(" ", test.max-len(atLimit))...)
			if err := test.parse(atLimit); err != nil {
				t.Fatalf("rejected exactly %d bytes: %v", test.max, err)
			}
			if err := test.parse(append(atLimit, '\n')); err == nil {
				t.Fatal("accepted oversized input with trailing whitespace")
			}
		})
	}
}

func TestParseJSONRejectsMalformedObjects(t *testing.T) {
	for _, test := range []struct {
		name  string
		data  []byte
		parse func([]byte) error
	}{
		{"context", contextJSON(t), func(data []byte) error {
			got, err := ParseContext(data)
			if err != nil && got != (Context{}) {
				t.Error("failed parse returned a partial context")
			}
			return err
		}},
		{"proof", proofJSON(t), func(data []byte) error {
			got, err := ParseProof(data)
			if err != nil && !reflect.DeepEqual(got, Proof{}) {
				t.Error("failed parse returned a partial proof")
			}
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			valid := string(test.data)
			cases := map[string][]byte{
				"empty": {}, "null": []byte("null"), "array": []byte("[]"),
				"string": []byte(`"object"`), "number": []byte("1"),
				"missing all":       []byte("{}"),
				"trailing object":   []byte(valid + `{}`),
				"trailing garbage":  []byte(valid + `garbage`),
				"duplicate":         []byte(strings.TrimSuffix(valid, "}") + `,"version":1}`),
				"escaped duplicate": []byte(strings.TrimSuffix(valid, "}") + `,"versi\u006fn":1}`),
				"unknown":           []byte(strings.TrimSuffix(valid, "}") + `,"trusted":true}`),
				"case alias":        []byte(strings.Replace(valid, `"version"`, `"Version"`, 1)),
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(test.data, &fields); err != nil {
				t.Fatal(err)
			}
			for name := range fields {
				cases["missing "+name] = replaceJSONField(t, test.data, name, nil)
				cases["null "+name] = replaceJSONField(t, test.data, name, json.RawMessage("null"))
				cases["wrong type "+name] = replaceJSONField(t, test.data, name, json.RawMessage("false"))
			}
			for name, value := range map[string]string{
				"negative": "-1", "overflow": "4294967296", "fraction": "1.5",
				"float": "1.0", "exponent": "1e0", "quoted": `"1"`,
			} {
				cases["version "+name] = replaceJSONField(t, test.data, "version", json.RawMessage(value))
			}
			for name, data := range cases {
				t.Run(name, func(t *testing.T) {
					if err := test.parse(data); err == nil {
						t.Fatalf("accepted malformed JSON: %s", data)
					}
				})
			}
		})
	}
}

func TestParseProofRejectsMalformedBranches(t *testing.T) {
	for name, value := range map[string]string{
		"null element":      `[null]`,
		"null after string": `["` + strings.Repeat("11", 32) + `",null]`,
		"number element":    `[1]`,
		"object element":    `[{}]`,
		"nested array":      `[[]]`,
		"too many":          `[` + strings.Repeat(`"",`, 20) + `""]`,
	} {
		t.Run(name, func(t *testing.T) {
			data := replaceJSONField(t, proofJSON(t), "merkle_branch", json.RawMessage(value))
			if _, err := ParseProof(data); err == nil {
				t.Fatalf("accepted malformed branch %s", value)
			}
		})
	}
	branch := `[` + strings.Repeat(`"`+strings.Repeat("11", 32)+`",`, 19) + `"` + strings.Repeat("22", 32) + `"]`
	data := replaceJSONField(t, proofJSON(t), "merkle_branch", json.RawMessage(branch))
	if proof, err := ParseProof(data); err != nil || len(proof.MerkleBranch) != 20 {
		t.Fatalf("rejected 20 siblings: %+v (%v)", proof, err)
	}
}
