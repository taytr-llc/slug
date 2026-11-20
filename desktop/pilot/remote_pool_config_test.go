// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/wire/v2"
)

type remotePoolReference struct {
	Positives []struct {
		Name         string                    `json:"name"`
		Config       RemotePoolConfig          `json:"config"`
		CanonicalHex string                    `json:"canonical_hex"`
		ConfigID     string                    `json:"config_id"`
		IntentHex    string                    `json:"intent_hex"`
		ReadyHex     string                    `json:"ready_hex"`
		Worker       prospectworker.PoolConfig `json:"worker"`
	} `json:"positives"`
	Refusals []struct {
		Name   string           `json:"name"`
		Config RemotePoolConfig `json:"config"`
	} `json:"refusals"`
	RawRefusals []struct {
		Name   string `json:"name"`
		RawHex string `json:"raw_hex"`
	} `json:"raw_refusals"`
	Credentials []struct {
		Name         string `json:"name"`
		ConfigID     string `json:"config_id"`
		Audience     string `json:"audience"`
		CanonicalHex string `json:"canonical_hex"`
	} `json:"credentials"`
}

func remotePoolTestReference(t *testing.T) remotePoolReference {
	t.Helper()
	b, e := os.ReadFile("testdata/remote-pool/reference-v1.json")
	if e != nil || len(b) != 563867 {
		t.Fatal("reference input", e)
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != "5f67fcaaba0c78b97b0f116a4c598eb0ec62ee1436bbcf280edcc747856e5177" {
		t.Fatal("reference input changed")
	}
	var r remotePoolReference
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatal(e)
	}
	if len(r.Positives) != 4 || len(r.Refusals) != 32 || len(r.RawRefusals) != 10 || len(r.Credentials) != 4 {
		t.Fatal("reference scope")
	}
	return r
}
func remotePoolTestHex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestRemotePoolReferenceConfigAndMarkers(t *testing.T) {
	for _, v := range remotePoolTestReference(t).Positives {
		t.Run(v.Name, func(t *testing.T) {
			b, e := MarshalRemotePoolConfig(v.Config)
			if e != nil || !bytes.Equal(b, remotePoolTestHex(t, v.CanonicalHex)) {
				t.Fatal("independent canonical bytes", e)
			}
			p, e := ParseRemotePoolConfig(b)
			if e != nil || !reflect.DeepEqual(p, v.Config) {
				t.Fatal("canonical parse", e)
			}
			id, e := RemotePoolConfigID(p)
			if e != nil || id != v.ConfigID {
				t.Fatal("independent config ID", e)
			}
			i, r, e := remotePoolMarkerBytes(p)
			if e != nil || !bytes.Equal(i, remotePoolTestHex(t, v.IntentHex)) || !bytes.Equal(r, remotePoolTestHex(t, v.ReadyHex)) {
				t.Fatal("independent markers", e)
			}
			if remotePoolValidateMarkers(i, r, p) != nil {
				t.Fatal("exact marker validation")
			}
			w, e := remotePoolWorkerConfig(p)
			if e != nil || w != v.Worker {
				t.Fatal("independent transport/worker identity", e)
			}
			if p.ExpectedLocal.OriginalSourceBindingID == p.Miners.SourceID {
				t.Fatal("fixture must preserve distinct original source")
			}
			p.ExpectedLocal.WalletOwnerID = p.ConnectionID
			if remotePoolValidateMarkers(i, r, p) == nil {
				t.Fatal("marker selected a replacement local wallet")
			}
			if remotePoolValidateMarkers(append(i, '\n'), r, v.Config) == nil {
				t.Fatal("extra marker newline")
			}
		})
	}
}
func TestRemotePoolReferenceRefusals(t *testing.T) {
	r := remotePoolTestReference(t)
	for _, v := range r.Refusals {
		t.Run(v.Name, func(t *testing.T) {
			if ValidateRemotePoolConfig(v.Config) == nil {
				t.Fatal("invalid public cross-binding accepted")
			}
		})
	}
	for _, v := range r.RawRefusals {
		t.Run("raw-"+v.Name, func(t *testing.T) {
			if _, e := ParseRemotePoolConfig(remotePoolTestHex(t, v.RawHex)); e == nil {
				t.Fatal("noncanonical shape accepted")
			}
		})
	}
}
func TestRemotePoolCredentialReferenceGrammar(t *testing.T) {
	r := remotePoolTestReference(t)
	for _, v := range r.Credentials {
		t.Run(v.Name, func(t *testing.T) {
			var c RemotePoolConfig
			for _, p := range r.Positives {
				if p.ConfigID == v.ConfigID {
					c = p.Config
				}
			}
			b := remotePoolTestHex(t, v.CanonicalHex)
			gym := c.ExpectedLocal.Recipient.Config.Identity.GymID
			got, e := remotePoolParseCredential(b, v.ConfigID, gym, v.Audience)
			if e != nil || got.Generation == 0 {
				t.Fatal("independent private record grammar", e)
			}
			other := "pool"
			if v.Audience == "pool" {
				other = "admission"
			}
			if _, e = remotePoolParseCredential(b, v.ConfigID, gym, other); e == nil {
				t.Fatal("cross audience")
			}
			if _, e = remotePoolParseCredential(b, c.ConnectionID, gym, v.Audience); e == nil {
				t.Fatal("replacement config")
			}
			if _, e = remotePoolParseCredential(b[:len(b)-1], v.ConfigID, gym, v.Audience); e == nil {
				t.Fatal("missing file LF")
			}
			if _, e = remotePoolParseCredential(append(b, '\n'), v.ConfigID, gym, v.Audience); e == nil {
				t.Fatal("extra file LF")
			}
		})
	}
}
func TestRemotePoolCredentialProfileRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile-only refusal")
	}
	if o, e := openRemotePoolCredentials(context.Background(), RemotePoolConfig{}, remotePoolCredentialSelection{}); o != nil || !errors.Is(e, errRemotePoolCredentialProfile) {
		t.Fatal("cheap runtime custody admitted", e)
	}
}
