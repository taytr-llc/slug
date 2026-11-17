//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
)

func TestManifestCheapProfileRefusesBeforeState(t *testing.T) {
	raw, err := os.ReadFile("../registry/testdata/manifest/reference/manifest_admission_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Pin   registry.ManifestAuthorityPin `json:"pin"`
		Cases []struct {
			Job       prospectjob.ManifestJob `json:"job"`
			Candidate prospectwork.Candidate  `json:"candidate"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	tc := f.Cases[2]
	c := ManifestConfig{Version: 2, Policy: prospectnode.Policy{Version: 1, Network: tc.Job.Context.Network, Genesis: tc.Job.Context.Genesis, DNAVersion: tc.Job.Context.DNAVersion, ProspectTarget: tc.Job.Context.ProspectTarget}, PublicKey: tc.Job.PublicKey, PayoutAddress: tc.Candidate.PayoutAddress, AuthorityOrigin: "http://127.0.0.1:37619", AuthorityPin: f.Pin, MaxRecords: 100, MaxBytes: 64 << 20}
	c.TransportTrustID, err = prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: c.AuthorityOrigin, Token: strings.Repeat("00", 32), AuthorityPin: f.Pin})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := ValidateManifestConfig(c)
	if err != nil {
		t.Fatal("pure production config inspection refused", err)
	}
	raw, _ = json.Marshal(canonical)
	if got, err := ParseManifestConfig(raw); err != nil || got != canonical {
		t.Fatal("pure config roundtrip", err)
	}
	require := func(err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "slugtestpow") {
			t.Fatal("missing profile refusal", err)
		}
	}
	path := filepath.Join(t.TempDir(), "absent")
	box, err := CreateManifestOutbox(path, c)
	require(err)
	if box != nil {
		t.Fatal("box leaked")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Create touched path")
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := []byte("must remain untouched")
	marker := filepath.Join(path, markerName)
	if err = os.WriteFile(marker, sentinel, 0600); err != nil {
		t.Fatal(err)
	}
	box, err = OpenManifestOutbox(path, c)
	require(err)
	if box != nil {
		t.Fatal("opened")
	}
	after, _ := os.ReadFile(marker)
	if !bytes.Equal(after, sentinel) {
		t.Fatal("Open touched path")
	}
	w, err := NewManifest(c, nil, nil, nil, nil)
	require(err)
	if w != nil {
		t.Fatal("worker leaked")
	}
	require((&ManifestWorker{}).Run(context.Background()))
	b := &ManifestOutbox{}
	rows, err := b.List(0, 1)
	require(err)
	if rows != nil {
		t.Fatal("cheap rows leaked")
	}
}
