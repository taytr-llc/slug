// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type remoteWireReferenceCase struct {
	Name         string          `json:"name"`
	Config       string          `json:"config"`
	Request      json.RawMessage `json:"request"`
	Prior        *RemotePoolView `json:"prior_view"`
	Response     json.RawMessage `json:"response"`
	CanonicalHex string          `json:"canonical_hex"`
	RawHex       string          `json:"raw_hex"`
	Family       string          `json:"family"`
}
type remoteWireReference struct {
	Configs []struct {
		Name   string           `json:"name"`
		Config RemotePoolConfig `json:"config"`
	} `json:"configs"`
	Requests        []remoteWireReferenceCase `json:"requests"`
	RequestRefusals []remoteWireReferenceCase `json:"request_refusals"`
	Positives       []remoteWireReferenceCase `json:"positives"`
	Refusals        []remoteWireReferenceCase `json:"refusals"`
	RawRefusals     []remoteWireReferenceCase `json:"raw_refusals"`
	RawAcceptances  []remoteWireReferenceCase `json:"raw_acceptances"`
	Framing         []struct {
		Name   string `json:"name"`
		RawHex string `json:"raw_hex"`
		Accept bool   `json:"accept"`
	} `json:"framing"`
}

func remoteWireTestReference(t *testing.T) (remoteWireReference, map[string]RemotePoolConfig) {
	t.Helper()
	b, e := os.ReadFile("testdata/remote-pool-protocol/reference-v1.json")
	if e != nil || len(b) != 490221 {
		t.Fatal("public protocol reference", e)
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != "001430345f09c1fc8e1c924db18564250df6b5f32e21a2cddcfbc8d6be83a865" {
		t.Fatal("public reference changed")
	}
	var r remoteWireReference
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatal(e)
	}
	if len(r.Configs) != 3 || len(r.Requests) != 12 || len(r.RequestRefusals) != 12 || len(r.Positives) != 31 || len(r.Refusals) != 47 || len(r.RawRefusals) != 17 || len(r.RawAcceptances) != 2 || len(r.Framing) != 4 {
		t.Fatal("finite reference scope")
	}
	configs := make(map[string]RemotePoolConfig, len(r.Configs))
	for _, row := range r.Configs {
		if ValidateRemotePoolConfig(row.Config) != nil {
			t.Fatal("inherited positive config")
		}
		configs[row.Name] = row.Config
	}
	return r, configs
}
func remoteWireTestHex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func remoteWireTestRequest(t *testing.T, raw json.RawMessage) RemotePoolRequest {
	t.Helper()
	var q RemotePoolRequest
	if e := json.Unmarshal(raw, &q); e != nil {
		t.Fatal(e)
	}
	return q
}
func remoteWireTestCompact(t *testing.T, raw []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if e := json.Compact(&out, raw); e != nil {
		t.Fatal(e)
	}
	return out.Bytes()
}

func TestRemotePoolProtocolIndependentCorpus(t *testing.T) {
	r, configs := remoteWireTestReference(t)
	for _, x := range r.Requests {
		t.Run("request/"+x.Name, func(t *testing.T) {
			want := remoteWireTestHex(t, x.CanonicalHex)
			q, e := ParseRemotePoolRequest(want, configs[x.Config])
			if e != nil {
				t.Fatal(e)
			}
			got, e := MarshalRemotePoolRequest(q, configs[x.Config])
			if e != nil || !bytes.Equal(got, want) {
				t.Fatal("independent request bytes", e)
			}
			if !reflect.DeepEqual(q, remoteWireTestRequest(t, x.Request)) {
				t.Fatal("request fields")
			}
		})
	}
	for _, x := range r.Positives {
		t.Run("response/"+x.Name, func(t *testing.T) {
			want := remoteWireTestHex(t, x.CanonicalHex)
			got, e := ParseRemotePoolResponse(want, remoteWireTestRequest(t, x.Request), configs[x.Config], x.Prior)
			if e != nil {
				t.Fatal(e)
			}
			encoded, e := json.Marshal(got)
			if e != nil || !bytes.Equal(encoded, want) {
				t.Fatal("independent response bytes", e)
			}
		})
	}
	for _, x := range r.RequestRefusals {
		t.Run("request-refusal/"+x.Name, func(t *testing.T) {
			if _, e := ParseRemotePoolRequest(remoteWireTestCompact(t, x.Request), configs[x.Config]); e == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
	for _, x := range r.Refusals {
		t.Run("response-refusal/"+x.Name, func(t *testing.T) {
			if _, e := ParseRemotePoolResponse(remoteWireTestCompact(t, x.Response), remoteWireTestRequest(t, x.Request), configs[x.Config], x.Prior); e == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
	for _, x := range r.RawRefusals {
		t.Run("raw-refusal/"+x.Name, func(t *testing.T) {
			b := remoteWireTestHex(t, x.RawHex)
			var e error
			if x.Family == "request" {
				_, e = ParseRemotePoolRequest(b, configs[x.Config])
			} else {
				_, e = ParseRemotePoolResponse(b, remoteWireTestRequest(t, x.Request), configs[x.Config], x.Prior)
			}
			if e == nil {
				t.Fatal("invalid raw accepted")
			}
		})
	}
	for _, x := range r.RawAcceptances {
		t.Run("raw-accept/"+x.Name, func(t *testing.T) {
			b := remoteWireTestHex(t, x.RawHex)
			var e error
			if x.Family == "request" {
				_, e = ParseRemotePoolRequest(b, configs[x.Config])
			} else {
				_, e = ParseRemotePoolResponse(b, remoteWireTestRequest(t, x.Request), configs[x.Config], x.Prior)
			}
			if e != nil {
				t.Fatal("exact-size valid object refused", e)
			}
		})
	}
}
