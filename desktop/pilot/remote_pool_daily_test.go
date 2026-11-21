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

	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/wire/v2"
)

type remoteDailyPositive struct {
	Name         string                        `json:"name"`
	Config       RemotePoolConfig              `json:"config"`
	ConfigID     string                        `json:"config_id"`
	CanonicalHex string                        `json:"canonical_hex"`
	IntentHex    string                        `json:"intent_hex"`
	ReadyHex     string                        `json:"ready_hex"`
	Client       poolservice.DailyClientConfig `json:"daily_client"`
	Worker       prospectworker.PoolConfig     `json:"worker"`
	TrustID      string                        `json:"trust_id"`
}
type remoteDailyReference struct {
	Transport struct {
		Positives []remoteDailyPositive `json:"remote_positives"`
		Cases     []struct {
			Name  string           `json:"name"`
			Valid bool             `json:"valid"`
			Value RemotePoolConfig `json:"value"`
		} `json:"remote_cases"`
		Credentials []struct {
			Name    string                     `json:"name"`
			Valid   bool                       `json:"valid"`
			Value   remotePoolCredentialRecord `json:"value"`
			FileHex string                     `json:"file_hex"`
		} `json:"local_credential_cases"`
	} `json:"transport"`
	Raw []struct {
		Name   string `json:"name"`
		Family string `json:"family"`
		Valid  bool   `json:"valid"`
		Hex    string `json:"raw_hex"`
	} `json:"raw_cases"`
}

func remoteDailyOracle(t *testing.T) remoteDailyReference {
	t.Helper()
	b, e := os.ReadFile("../../testdata/public-daily/reference-v1.json")
	if e != nil || len(b) != 3176583 {
		t.Fatal("daily reference", e)
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != "a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3" {
		t.Fatal("daily reference changed")
	}
	var r remoteDailyReference
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatal(e)
	}
	if len(r.Transport.Positives) != 2 || len(r.Transport.Cases) != 8 || len(r.Transport.Credentials) != 8 || len(r.Raw) != 143 {
		t.Fatal("daily reference scope")
	}
	return r
}
func TestDailyRemoteIndependentConfigAndMoneyBinding(t *testing.T) {
	r := remoteDailyOracle(t)
	for _, v := range r.Transport.Positives {
		t.Run(v.Name, func(t *testing.T) {
			raw, e := MarshalDailyRemotePoolConfig(v.Config)
			if e != nil || !bytes.Equal(raw, remotePoolTestHex(t, v.CanonicalHex)) {
				t.Fatal("daily bytes", e)
			}
			c, e := ParseDailyRemotePoolConfig(raw)
			if e != nil || !reflect.DeepEqual(c, v.Config) {
				t.Fatal("daily parse", e)
			}
			id, e := DailyRemotePoolConfigID(c)
			if e != nil || id != v.ConfigID {
				t.Fatal("daily ID", e)
			}
			i, ready, e := remotePoolMarkerBytes(c)
			if e != nil || !bytes.Equal(i, remotePoolTestHex(t, v.IntentHex)) || !bytes.Equal(ready, remotePoolTestHex(t, v.ReadyHex)) {
				t.Fatal("daily marker bytes", e)
			}
			if e = remotePoolValidateMarkers(i, ready, c); e != nil {
				t.Fatal(e)
			}
			client, e := remotePoolDailyClientConfig(c, v.Client.Token)
			if e != nil || client != v.Client {
				t.Fatal("complete selected money/client projection", e)
			}
			trust, e := poolservice.DailyTransportTrustID(client)
			if e != nil || trust != v.TrustID {
				t.Fatal("daily transport binding", e)
			}
			w, e := remotePoolWorkerConfig(c)
			if e != nil || w != v.Worker {
				t.Fatal("daily worker binding", e)
			}
			if ValidateRemotePoolConfig(c) == nil {
				t.Fatal("legacy validator accepted daily")
			}
			if _, e = ParseRemotePoolConfig(raw); e == nil {
				t.Fatal("legacy parser accepted daily")
			}
			if c.ExpectedLocal.Version != 1 || c.ExpectedLocal.ManagerSchema != 14 || c.ExpectedLocal.OriginalSourceBindingID == c.Miners.SourceID {
				t.Fatal("original wallet source repinned")
			}
		})
	}
	for _, v := range r.Transport.Cases {
		t.Run(v.Name, func(t *testing.T) {
			e := ValidateDailyRemotePoolConfig(v.Value)
			// The oracle's foreign-wallet case additionally requires the exact
			// independently selected original local binding, not mere ID syntax.
			exactLocal := reflect.DeepEqual(v.Value.ExpectedLocal, r.Transport.Positives[0].Config.ExpectedLocal)
			if (e == nil && exactLocal) != v.Valid {
				t.Fatal("daily config/original local binding", e)
			}
		})
	}
	for _, v := range r.Raw {
		if v.Family == "remote-config" {
			t.Run(v.Name, func(t *testing.T) {
				_, e := ParseDailyRemotePoolConfig(remotePoolTestHex(t, v.Hex))
				if (e == nil) != v.Valid {
					t.Fatal("daily config raw grammar", e)
				}
			})
		}
	}
}
func TestDailyRemoteIndependentCredentialGrammar(t *testing.T) {
	r := remoteDailyOracle(t)
	for _, v := range r.Transport.Credentials {
		t.Run(v.Name, func(t *testing.T) {
			got, e := remotePoolParseSelectedCredential(remotePoolTestHex(t, v.FileHex), v.Value.ConfigID, v.Value.GymID, v.Value.Audience, 2)
			if (e == nil) != v.Valid || e == nil && got != v.Value {
				t.Fatal("daily protected credential", e)
			}
			if v.Valid {
				if _, e = remotePoolParseCredential(remotePoolTestHex(t, v.FileHex), v.Value.ConfigID, v.Value.GymID, v.Value.Audience); e == nil {
					t.Fatal("legacy credential reader adopted daily")
				}
			}
		})
	}
	first := r.Transport.Credentials[0].Value
	for _, v := range r.Raw {
		if v.Family == "local-credential" {
			t.Run(v.Name, func(t *testing.T) {
				_, e := remotePoolParseSelectedCredential(remotePoolTestHex(t, v.Hex), first.ConfigID, first.GymID, first.Audience, 2)
				if (e == nil) != v.Valid {
					t.Fatal("daily credential framing", e)
				}
			})
		}
	}
}
func TestDailyRemoteSelectionBeforeOriginalOwnerEffects(t *testing.T) {
	r := remoteDailyOracle(t)
	if ValidateDailyRemotePoolConfig(remotePoolTestReference(t).Positives[0].Config) == nil {
		t.Fatal("daily validator adopted old mode")
	}
	if remotePoolSelectedPublicCode(2, "issue_unknown") != "outcome_unknown" || remotePoolSelectedPublicCode(1, "issue_unknown") != "unavailable" || remotePoolWorkerState("paused") != "paused" {
		t.Fatal("daily hold projection changed legacy vocabulary")
	}
	for _, open := range []func(context.Context, *OwnedWalletSession, string, RemotePoolInputs) (*RemotePoolAttachment, error){OpenRemotePoolAttachment, OpenDailyRemotePoolAttachment} {
		a, e := open(context.Background(), nil, "invalid", RemotePoolInputs{Config: r.Transport.Positives[0].Config})
		if a != nil || e == nil {
			t.Fatal("missing original owner admitted")
		}
		if wire.TestPoW && !errors.Is(e, ErrRemotePoolProfile) {
			t.Fatal("profile not first", e)
		}
	}
}
