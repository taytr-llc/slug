// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// These are relational test inputs: real normal-PoW admissions issued by one
// concrete store, signed by the already published recovery-fixture gym keys.
// They are not expected hash/receipt vectors. The current-parent eligibility
// guard is modeled; no node, live account or chain finality is claimed.
type sharedFixture struct {
	gyms [2]*coreFixture
	base registry.ManifestRecord
	all  []registry.ManifestRecord
}

func newSharedFixture(t *testing.T, order string) *sharedFixture {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal-profile verified shared source")
	}
	f := readCoreCorpus(t)
	x := &sharedFixture{gyms: [2]*coreFixture{{}, {}}}
	for _, p := range f.Pins {
		for i, name := range []string{"zero256", "incrementing256"} {
			if p.Name == name {
				x.gyms[i].pin = p.Pin
			}
		}
	}
	for _, c := range f.Cases {
		if c.Fixture == "zero256" && c.Record.Sequence == 1 {
			x.base = c.Record
		}
	}
	if len(x.base.Proof.MerkleBranch) != 0 || x.base.Sequence == 0 {
		t.Fatal("coinbase-only public proof skeleton missing")
	}
	root := t.TempDir()
	p := x.gyms[0].pin.Source
	source, e := registry.CreateManifestAuthority(filepath.Join(root, "issuer"), p.Network, p.Genesis, p.DescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		for _, g := range x.gyms {
			if g.s != nil {
				if e := g.s.Close(); e != nil {
					t.Error(e)
				}
			}
		}
		if e := source.Close(); e != nil {
			t.Error(e)
		}
	})
	info, e := source.Info()
	if e != nil {
		t.Fatal(e)
	}
	for i, g := range x.gyms {
		g.pin.Version = SharedPinVersion2
		g.pin.Source = info.Pin
		g.source = source
		g.sourcePath = filepath.Join(root, "issuer")
		g.path = filepath.Join(root, []string{"gym-a", "gym-b"}[i])
		g.session = fixtureSession(g.pin, 1, strings.Repeat([]string{"a", "b"}[i], 64))
	}
	for _, gym := range order {
		x.append(t, gym)
	}
	for _, g := range x.gyms {
		g.now = max(g.now, time.Now().UnixMilli()+1000)
		g.s, e = createShared(g.path, g.pin, source, func() time.Time { return time.UnixMilli(g.now) }, nil)
		if e != nil {
			t.Fatal(e)
		}
	}
	return x
}

func (x *sharedFixture) append(t *testing.T, gym rune) registry.ManifestRecord {
	t.Helper()
	if gym != 'A' && gym != 'B' {
		t.Fatal("bad fixture gym")
	}
	index := int(gym - 'A')
	g := x.gyms[index]
	c := x.base.Context
	c.ProspectTarget = strings.Repeat("f", 64) // one normal hash, no probabilistic search
	job, e := g.source.IssueManifestJob(c, g.pin.Identity.GymPublicKey, func() error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	script, e := prospectjob.ManifestCommitmentScript(job)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := hex.DecodeString(x.base.Proof.Coinbase)
	var tx wire.MsgTx
	if e = tx.Deserialize(bytes.NewReader(raw)); e != nil {
		t.Fatal(e)
	}
	tx.TxOut[len(tx.TxOut)-1].PkScript = script
	var coinbase bytes.Buffer
	if e = tx.SerializeNoWitness(&coinbase); e != nil {
		t.Fatal(e)
	}
	raw, _ = hex.DecodeString(x.base.Proof.Header)
	var header wire.BlockHeader
	if e = header.Deserialize(bytes.NewReader(raw)); e != nil {
		t.Fatal(e)
	}
	header.MerkleRoot = tx.TxHash()
	var encoded bytes.Buffer
	if e = header.Serialize(&encoded); e != nil {
		t.Fatal(e)
	}
	proof := x.base.Proof
	proof.JobID, proof.GymID = job.ID, job.GymID
	proof.Header, proof.Coinbase = hex.EncodeToString(encoded.Bytes()), hex.EncodeToString(coinbase.Bytes())
	// Both scalars are public inputs in gymaccount/testdata/enrollment_v1.json.
	scalars := [2]string{"9d3035e7440dd3737fe58ffb69ca3576257d424b6e6172b3199a66a8897bddf6", "948996ecaa9fa9707693267d6d4abefe0b5fb28b6ebae34f542e106244d254e1"}
	scalar, _ := hex.DecodeString(scalars[index])
	key, _ := btcec.PrivKeyFromBytes(scalar)
	defer key.Zero()
	if hex.EncodeToString(key.PubKey().SerializeCompressed()) != g.pin.Identity.GymPublicKey {
		t.Fatal("published fixture key mismatch")
	}
	auth, e := gymauth.SignAdmission(job.Context, proof, key)
	if e != nil {
		t.Fatal(e)
	}
	r, inserted, e := g.source.RecordManifestJobChecked(proof, auth, func(c prospect.Context, _ prospect.Proof, _ prospect.Verified) (registry.Eligibility, error) {
		return registry.Eligibility{RuleVersion: 2, Source: "current-parent", Hash: c.ParentHash, Height: c.ParentHeight}, nil
	})
	if e != nil || !inserted {
		t.Fatalf("shared issuer admission: inserted=%v err=%v", inserted, e)
	}
	x.all = append(x.all, r)
	g.records = append(g.records, r)
	for _, g := range x.gyms {
		g.now = max(g.now, r.Birth.AcceptedAtMS+1000)
	}
	return r
}

func reopenSharedFixture(t *testing.T, g *coreFixture) {
	t.Helper()
	if g.s != nil {
		if e := g.s.Close(); e != nil {
			t.Fatal(e)
		}
		g.s = nil
	}
	var e error
	g.s, e = openSharedCareerWithClock(g.path, g.pin, g.source, func() time.Time { return time.UnixMilli(g.now) }, nil, false)
	if e != nil {
		t.Fatal(e)
	}
}
