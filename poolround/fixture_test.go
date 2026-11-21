// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

const roundToken = "public-round-test-session"

type frozenSource struct {
	Name     string                  `json:"name"`
	Job      prospectjob.ManifestJob `json:"job"`
	Template prospectwork.Template   `json:"template"`
	Payout   string                  `json:"payout_address"`
}
type frozenCase struct {
	Name      string                  `json:"name"`
	Nonce     uint32                  `json:"nonce"`
	Header    string                  `json:"header"`
	Hash      string                  `json:"work_hash"`
	Key       string                  `json:"submission_key"`
	Block     bool                    `json:"qualifies_block"`
	Prospect  bool                    `json:"qualifies_prospect"`
	Candidate *prospectwork.Candidate `json:"qualifying_candidate"`
}

func checkedFixture(t *testing.T, path, want string, value any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != want {
		t.Fatal("independent fixture changed", path)
	}
	if json.Unmarshal(raw, value) != nil {
		t.Fatal("invalid fixture", path)
	}
}

func roundAuthority(t *testing.T) (*registry.ManifestStore, registry.ManifestAuthorityPin) {
	t.Helper()
	var f struct {
		Pin    registry.ManifestAuthorityPin `json:"pin"`
		Marker string                        `json:"marker_canonical"`
		Rows   struct {
			Meta          string `json:"meta"`
			Jobs, Records []struct{ Key, Value string }
			Sequences     []struct {
				KeyHex string `json:"key_hex"`
				Value  string `json:"value"`
			}
		} `json:"database_rows"`
	}
	checkedFixture(t, "../registry/testdata/manifest/reference/manifest_admission_v1.json", "d32a8653c76c643addb3df091ca8d7dfdba66c2486f2ca305e718113f71d5794", &f)
	path := filepath.Join(t.TempDir(), "issuer")
	if os.Mkdir(path, 0700) != nil {
		t.Fatal("create issuer fixture")
	}
	if os.WriteFile(filepath.Join(path, "SLUG-REGISTRY.json"), []byte(f.Marker), 0600) != nil {
		t.Fatal("write issuer fixture")
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfExist: true})
	if err != nil {
		t.Fatal(err)
	}
	batch := new(leveldb.Batch)
	batch.Put([]byte("meta"), []byte(f.Rows.Meta))
	for _, rows := range [][]struct{ Key, Value string }{f.Rows.Jobs, f.Rows.Records} {
		for _, row := range rows {
			batch.Put([]byte(row.Key), []byte(row.Value))
		}
	}
	for _, row := range f.Rows.Sequences {
		key, e := hex.DecodeString(row.KeyHex)
		if e != nil {
			t.Fatal(e)
		}
		batch.Put(key, []byte(row.Value))
	}
	if db.Write(batch, &opt.WriteOptions{Sync: true}) != nil {
		t.Fatal("sync issuer fixture")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	authority, err := registry.OpenManifestAuthority(path, f.Pin, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = authority.Close() })
	return authority, f.Pin
}

type fixtureSessions struct{ key string }

func (s *fixtureSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	if token != roundToken {
		return errors.New("bad fixture session")
	}
	return fn(s.key, ctx.Err)
}

type fixturePoolNode struct {
	source    frozenSource
	winner    frozenCase
	canonical bool
}

func (n *fixturePoolNode) ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error) {
	return n.source.Template, nil
}
func (n *fixturePoolNode) Proposal(context.Context, prospect.Context, string) error { return nil }
func (n *fixturePoolNode) RecheckContext(context.Context, prospect.Context) error   { return nil }
func (n *fixturePoolNode) SubmitBlock(_ context.Context, _ prospect.Context, block string) error {
	if n.winner.Candidate == nil || block != n.winner.Candidate.BlockHex {
		return errors.New("wrong frozen block")
	}
	return nil
}
func (n *fixturePoolNode) CanonicalBlock(_ context.Context, _ prospect.Context, proof prospect.Proof, hash string) error {
	if !n.canonical || n.winner.Candidate == nil || !reflect.DeepEqual(proof, n.winner.Candidate.Proof) || hash != n.winner.Hash {
		return errors.New("not exact canonical fixture")
	}
	return nil
}

type roundHarness struct {
	store        *Store
	path         string
	limits       Limits
	source       *poolshare.Pool
	sourceConfig poolshare.Config
	poolNode     *fixturePoolNode
	observer     *fixtureObserver
	input        frozenSource
	winner       frozenCase
}

type fixtureObserver struct {
	want      frozenCase
	canonical bool
	depth     uint32
	err       error
	change    func(*prospectnode.PoolBlockObservation)
	calls     int
}

func (o *fixtureObserver) ObservePoolBlock(ctx context.Context, c prospect.Context, p prospect.Proof, hash string) (prospectnode.PoolBlockObservation, error) {
	o.calls++
	if err := ctx.Err(); err != nil {
		return prospectnode.PoolBlockObservation{}, err
	}
	if o.err != nil {
		return prospectnode.PoolBlockObservation{}, o.err
	}
	if hash != o.want.Hash || o.want.Candidate == nil || !reflect.DeepEqual(p, o.want.Candidate.Proof) {
		return prospectnode.PoolBlockObservation{}, errors.New("unbound round work")
	}
	height := c.ParentHeight + 1
	tip := strings.Repeat("a", 64)
	if o.canonical && o.depth == 0 {
		tip = hash
	}
	r := prospectnode.PoolBlockObservation{BlockHash: hash, BlockHeight: height, TipHash: tip, TipHeight: height + o.depth, Canonical: o.canonical}
	if o.canonical {
		r.Descendants = o.depth
	}
	if o.change != nil {
		o.change(&r)
	}
	return r, nil
}

func sourceSequence(prefix byte, number uint64) []byte {
	k := make([]byte, 9)
	k[0] = prefix
	binary.BigEndian.PutUint64(k[1:], number)
	return k
}

// This fixture writes known public issued inputs and accepted work into the
// published Set53 storage format. The concrete Pool.Open still independently
// verifies every actual Argon proof, binding, index and counter before use.
func newRoundHarness(t *testing.T, pending bool) *roundHarness {
	return newRoundProfileHarness(t, pending, Version1)
}

func newRoundProfileHarness(t *testing.T, pending bool, version uint32) *roundHarness {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal-PoW source; profile refusal checked separately")
	}
	var f struct {
		Sources []frozenSource `json:"sources"`
		Cases   []frozenCase   `json:"cases"`
	}
	checkedFixture(t, "../prospectwork/testdata/pool_share_reference/pool_share_v1.json", "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87", &f)
	var input frozenSource
	for _, s := range f.Sources {
		if s.Name == "coinbase-only" {
			input = s
		}
	}
	var first, second, winner frozenCase
	for _, c := range f.Cases {
		switch c.Name {
		case "coinbase-only-nonce-0":
			first = c
		case "coinbase-only-nonce-1":
			second = c
		case "coinbase-only-nonce-4294967295":
			winner = c
		}
	}
	if input.Job.ID == "" || winner.Candidate == nil {
		t.Fatal("missing independent pool input")
	}
	authority, pin := roundAuthority(t)
	c := input.Job.Context
	config := poolshare.Config{Version: version, AuthorityPin: pin, Policy: prospectnode.Policy{Version: 1, Network: c.Network, Genesis: c.Genesis, DNAVersion: c.DNAVersion, ProspectTarget: c.ProspectTarget}, PayoutAddress: input.Payout, PoolTarget: strings.Repeat("f", 64), MaxWorks: 16, MaxSubmissions: 16, MaxBytes: 32 << 20}
	poolNode := &fixturePoolNode{source: input, winner: winner}
	sessions := &fixtureSessions{key: input.Job.PublicKey}
	sourcePath := filepath.Join(t.TempDir(), "source")
	createSource := poolshare.Create
	openSource := poolshare.Open
	if version == Version2 {
		createSource = func(path string, c poolshare.Config, a *registry.ManifestStore, n poolshare.Node, s prospectservice.SessionAuthorizer) (*poolshare.Pool, error) {
			return poolshare.CreateDaily(context.Background(), path, c, a, n, s)
		}
		openSource = func(path string, c poolshare.Config, a *registry.ManifestStore, n poolshare.Node, s prospectservice.SessionAuthorizer) (*poolshare.Pool, error) {
			return poolshare.OpenDaily(context.Background(), path, c, a, n, s)
		}
	}
	p, err := createSource(sourcePath, config, authority, poolNode, sessions)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := leveldb.OpenFile(filepath.Join(sourcePath, "db"), &opt.Options{ErrorIfMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	workRaw, err := json.Marshal(struct {
		Job      prospectjob.ManifestJob `json:"job"`
		Template prospectwork.Template   `json:"template"`
	}{input.Job, input.Template})
	if err != nil {
		t.Fatal(err)
	}
	batch := new(leveldb.Batch)
	batch.Put([]byte("w"+input.Job.ID), workRaw)
	cases := []frozenCase{first, second, winner}
	if pending {
		cases = []frozenCase{winner, first, second}
	}
	accepted := uint64(3)
	if pending {
		accepted = 2
	}
	for i, tc := range cases {
		sequence := uint64(i + 1)
		accept := sequence
		if pending {
			if i == 0 {
				accept = 0
			} else {
				accept = uint64(i)
			}
		}
		when := input.Job.IssuedAtMS + 1000 + int64(sequence)
		r := poolshare.Submission{Version: 1, Sequence: sequence, AcceptedSequence: accept, JobID: input.Job.ID, GymID: input.Job.GymID, Nonce: tc.Nonce, PoolTarget: config.PoolTarget, Header: tc.Header, WorkHash: tc.Hash, DuplicateKey: tc.Key, Pool: true, Prospect: tc.Prospect, Block: tc.Block, ReceivedAtMS: when, BlockState: "none"}
		if accept != 0 {
			r.AcceptedAtMS = when
			r.Acceptance = "current-parent"
		}
		if tc.Block {
			r.BlockHeight = c.ParentHeight + 1
			r.BlockState = "processed"
			r.BlockObservedAtMS = when
			if pending {
				r.BlockState = "pending"
				r.BlockObservedAtMS = 0
			}
		}
		raw, e := json.Marshal(r)
		if e != nil {
			t.Fatal(e)
		}
		batch.Put([]byte("s"+tc.Key), raw)
		var nonce [4]byte
		binary.BigEndian.PutUint32(nonce[:], tc.Nonce)
		batch.Put([]byte("n"+input.Job.ID+hex.EncodeToString(nonce[:])), []byte(tc.Key))
		batch.Put(sourceSequence('q', sequence), []byte(tc.Key))
		if accept != 0 {
			batch.Put(sourceSequence('a', accept), []byte(tc.Key))
		}
	}
	meta := struct {
		Application string           `json:"application"`
		Version     uint32           `json:"version"`
		Config      poolshare.Config `json:"config"`
		Works       uint64           `json:"works"`
		Submissions uint64           `json:"submissions"`
		Accepted    uint64           `json:"accepted"`
		Bytes       uint64           `json:"bytes"`
		LastClockMS int64            `json:"last_clock_ms"`
	}{"slug-pool-share-journal", version, config, 1, 3, accepted, uint64(len(workRaw)) + 3*4096, input.Job.IssuedAtMS + 1003}
	metaRaw, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	batch.Put([]byte("meta"), metaRaw)
	if db.Write(batch, &opt.WriteOptions{Sync: true}) != nil {
		t.Fatal("seed concrete pool source")
	}
	if db.Close() != nil {
		t.Fatal("close source seed")
	}
	p, err = openSource(sourcePath, config, authority, poolNode, sessions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	h := &roundHarness{path: filepath.Join(t.TempDir(), "rounds"), limits: Limits{Version: version, MaxRounds: 16, MaxBytes: 32 << 20}, source: p, sourceConfig: config, poolNode: poolNode, input: input, winner: winner, observer: &fixtureObserver{want: winner, canonical: true, depth: 59}}
	if version == Version2 {
		h.limits.MaxRounds = 721
		h.store, err = CreateDaily(context.Background(), h.path, h.limits, h.source, h.observer)
	} else {
		h.store, err = Create(h.path, h.limits, h.source, h.observer)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.store.Close() })
	return h
}

func (h *roundHarness) reopen(t *testing.T) {
	t.Helper()
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	h.store, err = Open(h.path, h.limits, h.source, h.observer)
	if err != nil {
		t.Fatal(err)
	}
}
