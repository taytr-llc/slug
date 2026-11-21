// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type dailyReference struct {
	Version   int `json:"version"`
	Positives []struct {
		Name      string `json:"name"`
		Config    Config `json:"config"`
		Canonical string `json:"config_canonical"`
		ID        string `json:"daily_config_id"`
		Marker    string `json:"marker_canonical"`
		Metadata  string `json:"metadata_canonical"`
	} `json:"positives"`
	ConfigRefusals []struct {
		Name string `json:"name"`
		Raw  string `json:"config_canonical"`
	} `json:"daily_config_refusals"`
	StorageRefusals []struct {
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		Raw    string `json:"raw"`
		Config Config `json:"expected_config"`
	} `json:"storage_refusals"`
	Keys []struct {
		Name     string `json:"name"`
		Sequence uint64 `json:"sequence"`
		Nonce    uint32 `json:"nonce"`
		Hex      string `json:"hex"`
	} `json:"key_cases"`
	JobID         string `json:"original_job_id"`
	SubmissionKey string `json:"original_submission_key"`
}

func readDailyReference(t *testing.T) dailyReference {
	t.Helper()
	raw, err := os.ReadFile("testdata/daily-source-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "3ae0cac7a3ecdd38f8f4dd1613f919da205ea36fb1958dc121211057a8fa23f9" {
		t.Fatal("independent daily source reference changed")
	}
	var f dailyReference
	if json.Unmarshal(raw, &f) != nil || f.Version != 1 || len(f.Positives) != 4 || len(f.ConfigRefusals) != 13 || len(f.StorageRefusals) != 20 || len(f.Keys) != 26 {
		t.Fatal("reference scope")
	}
	return f
}

func TestDailySourceIndependentGrammarAndKeys(t *testing.T) {
	f := readDailyReference(t)
	for _, tc := range f.Positives {
		raw, _ := json.Marshal(tc.Config)
		if string(raw) != tc.Canonical {
			t.Fatal("config bytes", tc.Name)
		}
		m, _ := json.Marshal(marker{application, tc.Config.Version, tc.Config})
		meta, _ := json.Marshal(metadata{Application: application, Version: tc.Config.Version, Config: tc.Config})
		if string(m) != tc.Marker || string(meta) != tc.Metadata {
			t.Fatal("persistent bytes", tc.Name)
		}
		id, err := DailyConfigID(tc.Config)
		if wire.TestPoW {
			if !errors.Is(err, ErrProfile) || id != "" {
				t.Fatal("cheap identity profile", err)
			}
		} else if tc.Config.Version == Version1 {
			if !errors.Is(err, ErrConfig) || id != "" {
				t.Fatal("legacy adopted by daily identity", err)
			}
		} else if err != nil || id != tc.ID {
			t.Fatal("independent configuration digest", tc.Name, id, err)
		}
	}
	for _, tc := range f.ConfigRefusals {
		var c Config
		if json.Unmarshal([]byte(tc.Raw), &c) != nil {
			continue
		} // uint64 overflow is a decoder refusal
		if id, err := DailyConfigID(c); err == nil || id != "" {
			t.Fatal("invalid daily config", tc.Name)
		}
	}
	for _, tc := range f.Keys {
		var key []byte
		switch {
		case tc.Name == "work-original":
			key = workKey(f.JobID)
		case tc.Name == "submission-original":
			key = recordKey(f.SubmissionKey)
		case strings.HasPrefix(tc.Name, "nonce-"):
			key = nonceKey(f.JobID, tc.Nonce)
		default:
			key = seqKey(tc.Name[0], tc.Sequence)
		}
		if hex.EncodeToString(key) != tc.Hex {
			t.Fatal("changed global key encoding", tc.Name)
		}
	}
}

// Same independently encoded original authority as seedPoolAuthority, with its
// exact owned path retained so this test can close/reopen the authority itself.
func dailyFixtureAuthority(t *testing.T) (*registry.ManifestStore, string, registry.ManifestAuthorityPin) {
	t.Helper()
	var f struct {
		Pin    registry.ManifestAuthorityPin `json:"pin"`
		Marker string                        `json:"marker_canonical"`
		Rows   struct {
			Meta          string `json:"meta"`
			Jobs, Records []struct{ Key, Value string }
			Sequences     []struct {
				KeyHex string `json:"key_hex"`
				Value  string
			}
		} `json:"database_rows"`
	}
	raw, err := os.ReadFile("../registry/testdata/manifest/reference/manifest_admission_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "d32a8653c76c643addb3df091ca8d7dfdba66c2486f2ca305e718113f71d5794" || json.Unmarshal(raw, &f) != nil {
		t.Fatal("original authority fixture changed")
	}
	path := filepath.Join(t.TempDir(), "issuer")
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(path, "SLUG-REGISTRY.json"), []byte(f.Marker), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfExist: true, Strict: opt.StrictAll})
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
			_ = db.Close()
			t.Fatal(e)
		}
		batch.Put(key, []byte(row.Value))
	}
	err = db.Write(batch, &opt.WriteOptions{Sync: true})
	closeErr := db.Close()
	if errors.Join(err, closeErr) != nil {
		t.Fatal(errors.Join(err, closeErr))
	}
	a, err := registry.OpenManifestAuthority(path, f.Pin, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a, path, f.Pin
}

func dailyFixture(t *testing.T, works uint64) (*poolHarness, string) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal-PoW original fixture; cheap refusal tested separately")
	}
	f := poolFixture(t)
	source := f.source(t, "coinbase-only")
	a, path, pin := dailyFixtureAuthority(t)
	c := source.Job.Context
	h := &poolHarness{path: filepath.Join(t.TempDir(), "pool"), authority: a, source: source,
		ordinary: f.caseByName(t, "coinbase-only-nonce-0"), winner: f.caseByName(t, "coinbase-only-nonce-4294967295"), now: source.Job.IssuedAtMS + 1000,
		sessions: &testPoolSessions{key: source.Job.PublicKey}, node: &testPoolNode{template: source.Template, canonicalErr: errors.New("not canonical")},
		config: Config{Version: Version2, AuthorityPin: pin, Policy: prospectnode.Policy{Version: 1, Network: c.Network, Genesis: c.Genesis, DNAVersion: c.DNAVersion, ProspectTarget: c.ProspectTarget}, PayoutAddress: source.PayoutAddress, PoolTarget: strings.Repeat("f", 64), MaxWorks: works, MaxSubmissions: 8192, MaxBytes: 1 << 30}}
	h.node.want = &h.winner
	var err error
	construction, cancel := context.WithCancel(context.Background())
	h.pool, err = CreateDaily(construction, h.path, h.config, h.authority, h.node, h.sessions)
	cancel() // construction admission must not own the surviving pool lifetime
	if err != nil {
		t.Fatal(err)
	}
	h.pool.now = func() time.Time { return time.UnixMilli(h.now) }
	w := savedWork{Job: source.Job, Template: source.Template}
	if _, err = h.pool.prepareContext(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if err = h.pool.saveWork(w, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.pool.Close(); _ = h.authority.Close() })
	return h, path
}

func dailyRows(t *testing.T, p *Pool) [32]byte {
	t.Helper()
	h := sha256.New()
	it := p.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		_, _ = h.Write(it.Key())
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(it.Value())
		_, _ = h.Write([]byte{0})
	}
	if it.Error() != nil {
		t.Fatal(it.Error())
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func TestDailySource721OriginalJobsAndReopen(t *testing.T) {
	h, issuerPath := dailyFixture(t, 1024)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	first, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	assertFrozenSubmission(t, first, h.ordinary, h.source, h.config.PoolTarget)
	before, err := h.authority.Info()
	if err != nil {
		t.Fatal(err)
	}
	h.pool.now = time.Now
	ids := map[string]bool{h.source.Job.ID: true}
	for i := 1; i <= 721; i++ {
		w, e := h.pool.IssueMiningWork(ctx, poolTestToken)
		if e != nil || ids[w.Job.ID] {
			t.Fatal("real immutable issuance", i, e)
		}
		ids[w.Job.ID] = true
		original, e := h.authority.TryGetManifestJob(ctx, w.Job.ID)
		if e != nil || original != w.Job {
			t.Fatal("issuer original job", i, e)
		}
		if i == 256 || i == 720 {
			old, e := h.pool.RecoverMiningWork(ctx, poolTestToken, h.source.Job.ID)
			if e != nil || old.Job != h.source.Job || !reflect.DeepEqual(old.Template, h.source.Template) {
				t.Fatal("original A lost across work boundary", i, e)
			}
		}
	}
	after, err := h.authority.Info()
	if err != nil || after.Jobs != before.Jobs+721 || after.Records != before.Records || h.pool.meta.Works != 722 || h.pool.meta.Submissions != 1 {
		t.Fatal("issuance counters/births", after, err)
	}
	pin := h.config.AuthorityPin
	if err = h.pool.Close(); err != nil {
		t.Fatal(err)
	}
	if err = h.authority.Close(); err != nil {
		t.Fatal(err)
	}
	h.authority, err = registry.OpenManifestAuthority(issuerPath, pin, false)
	if err != nil {
		t.Fatal(err)
	}
	h.pool, err = OpenDaily(ctx, h.path, h.config, h.authority, h.node, h.sessions)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := h.pool.Lookup(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	if err != nil || saved != first {
		t.Fatal("A receipt changed across physical reopen", err)
	}
	h.node.canonicalErr = nil
	late, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || late.Sequence != 2 || late.AcceptedSequence != 2 || late.Acceptance != "canonical-late-winner" {
		t.Fatal("original expired A winner", late, err)
	}
	assertFrozenSubmission(t, late, h.winner, h.source, h.config.PoolTarget)
	if err = h.pool.Close(); err != nil {
		t.Fatal(err)
	}
	h.pool, err = OpenDaily(ctx, h.path, h.config, h.authority, h.node, h.sessions)
	if err != nil {
		t.Fatal(err)
	}
	broadcasts := h.node.broadcasts
	dup, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || dup != late || h.node.broadcasts != broadcasts || h.pool.meta.Works != 722 || h.pool.meta.Submissions != 2 || h.pool.meta.Accepted != 2 {
		t.Fatal("exact original retry duplicated evidence", err)
	}
	if _, err = h.pool.AttachOperatorAdmission(ctx, h.config, 2); err == nil {
		t.Fatal("V1 financial admission adopted V2")
	}
}

func TestDailySourceCanonicalStorageRefusals(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal source fixture")
	}
	f := readDailyReference(t)
	a, _, _ := dailyFixtureAuthority(t)
	node := &testPoolNode{}
	sessions := &testPoolSessions{}
	for _, tc := range f.StorageRefusals {
		t.Run(tc.Name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pool")
			p, err := CreateDaily(context.Background(), path, tc.Config, a, node, sessions)
			if err != nil {
				t.Fatal(err)
			}
			if err = p.Close(); err != nil {
				t.Fatal(err)
			}
			if tc.Kind == "marker" {
				if err = os.WriteFile(filepath.Join(path, markerName), []byte(tc.Raw), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true, Strict: opt.StrictAll})
				if e != nil {
					t.Fatal(e)
				}
				e = db.Put([]byte("meta"), []byte(tc.Raw), &opt.WriteOptions{Sync: true})
				ce := db.Close()
				if errors.Join(e, ce) != nil {
					t.Fatal(errors.Join(e, ce))
				}
			}
			opened, err := OpenDaily(context.Background(), path, tc.Config, a, node, sessions)
			if opened != nil || !errors.Is(err, ErrCorrupt) {
				if opened != nil {
					_ = opened.Close()
				}
				t.Fatal("invalid stored identity admitted", err)
			}
		})
	}
}

func TestDailySourceModesCancellationAndLimits(t *testing.T) {
	h, _ := dailyFixture(t, 3) // original issuer already has two independently frozen jobs
	legacy := h.config
	legacy.Version = Version1
	if p, err := CreateDaily(context.Background(), filepath.Join(t.TempDir(), "daily"), legacy, h.authority, h.node, h.sessions); p != nil || !errors.Is(err, ErrConfig) {
		t.Fatal("daily constructor adopted V1", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p, err := OpenDaily(ctx, h.path, h.config, h.authority, h.node, h.sessions); p != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled open", err)
	}
	if p, err := Create(filepath.Join(t.TempDir(), "legacy"), h.config, h.authority, h.node, h.sessions); p != nil || err == nil {
		t.Fatal("legacy Create adoptedV2")
	}
	if p, err := Open(h.path, h.config, h.authority, h.node, h.sessions); p != nil || err == nil {
		t.Fatal("legacy Open adoptedV2")
	}
	if p, err := OpenAudit(h.path, h.config, h.authority); p != nil || err == nil {
		t.Fatal("legacy audit adoptedV2")
	}
	h.pool.now = time.Now
	if _, err := h.pool.Issue(context.Background(), poolTestToken); err != nil {
		t.Fatal(err)
	}
	before := dailyRows(t, h.pool)
	if _, err := h.pool.Issue(context.Background(), poolTestToken); !errors.Is(err, ErrCapacity) {
		t.Fatal("selected issuer cap", err)
	}
	if dailyRows(t, h.pool) != before {
		t.Fatal("capacity refusal changed pool")
	}
	if err := h.pool.Close(); err != nil {
		t.Fatal(err)
	}
	wrong := h.config
	wrong.MaxWorks++
	if p, err := OpenDaily(context.Background(), h.path, wrong, h.authority, h.node, h.sessions); p != nil || !errors.Is(err, ErrCorrupt) {
		t.Fatal("reopen widened selected cap", err)
	}
	read, err := OpenDailyAudit(context.Background(), h.path, h.config, h.authority)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	if _, err = read.Issue(context.Background(), poolTestToken); !errors.Is(err, ErrInvalid) {
		t.Fatal("audit wrote", err)
	}
	if _, err = read.AccountingPage(context.Background(), 0, 0, 8); err != nil {
		t.Fatal(err)
	}
}

func TestDailySourceCheapRefusesBeforeEffects(t *testing.T) {
	if !wire.TestPoW {
		return
	}
	c := readDailyReference(t).Positives[1].Config
	path := filepath.Join(t.TempDir(), "never")
	if p, err := CreateDaily(context.Background(), path, c, nil, nil, nil); p != nil || !errors.Is(err, ErrProfile) {
		t.Fatal("cheap create", err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("cheap create touched storage")
	}
	if p, err := OpenDaily(context.Background(), path, c, nil, nil, nil); p != nil || !errors.Is(err, ErrProfile) {
		t.Fatal("cheap open", err)
	}
}

func TestDailySourceMemoOwnedWarmColdAndTamper(t *testing.T) {
	h, _ := dailyFixture(t, 16)
	ctx := context.Background()
	if len(h.pool.nonceMemo) != 0 {
		t.Fatal("unretained work primed memo")
	}
	first, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	if err != nil || len(h.pool.nonceMemo) != 1 {
		t.Fatal("retained check not memoized", err)
	}
	work, prepared, err := h.pool.loadWorkContext(ctx, h.source.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	warm, err := h.pool.validateSubmission(ctx, first, work, prepared)
	if err != nil || warm.Header != h.ordinary.Header || warm.WorkHash != h.ordinary.WorkHash || warm.DuplicateKey != h.ordinary.DuplicateKey {
		t.Fatal("warm independent check", err)
	}
	if warm.Candidate == nil {
		t.Fatal("missing reconstructed candidate")
	}
	if !reflect.DeepEqual(warm.Candidate, h.ordinary.Candidate) {
		t.Fatal("warm candidate differs from independent complete bytes/empty branch")
	}
	warm.Candidate.BlockHex = "caller mutation"
	warm.Candidate.Proof.Header = "caller mutation"
	again, err := h.pool.validateSubmission(ctx, first, work, prepared)
	if err != nil || again.Candidate == nil || again.Candidate.BlockHex == "caller mutation" || len(h.pool.nonceMemo) != 1 {
		t.Fatal("candidate ownership", err)
	}
	for _, field := range []string{"header", "hash", "index"} {
		before := dailyRows(t, h.pool)
		bad := first
		switch field {
		case "header":
			bad.Header = strings.Repeat("0", 160)
		case "hash":
			bad.WorkHash = strings.Repeat("0", 64)
		case "index":
			bad.AcceptedSequence++
		}
		if _, err = h.pool.validateSubmission(ctx, bad, work, prepared); !errors.Is(err, ErrCorrupt) || dailyRows(t, h.pool) != before {
			t.Fatal("warm memo masked tamper", field, err)
		}
	}
	// A complete check alone cannot prime an unretained nonce, even when it is
	// a qualifying winner. Only full retained validation/commit may insert.
	if _, err = h.pool.checkRetainedNonce(ctx, work, prepared, h.winner.Nonce); err != nil || len(h.pool.nonceMemo) != 1 {
		t.Fatal("unretained check primed memo", err)
	}
	indexKey := seqKey('a', first.AcceptedSequence)
	indexRaw, err := h.pool.db.Get(indexKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.pool.db.Delete(indexKey, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	_, indexErr := h.pool.Lookup(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	if err = h.pool.db.Put(indexKey, indexRaw, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(indexErr, ErrCorrupt) {
		t.Fatal("warm cache masked missing acceptance index", indexErr)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = h.pool.validateSubmission(canceled, first, work, prepared); !errors.Is(err, context.Canceled) || len(h.pool.nonceMemo) != 1 {
		t.Fatal("canceled memo disclosure", err)
	}
	key, err := h.pool.nonceMemoKey(ctx, work, first.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	changed := work
	changed.Template.Block.WorkID = "different complete template"
	other, err := h.pool.nonceMemoKey(ctx, changed, first.Nonce)
	if err != nil || key == other {
		t.Fatal("memo omitted template", err)
	}
	before := dailyRows(t, h.pool)
	oldPool := h.pool
	if err = h.pool.Close(); err != nil {
		t.Fatal(err)
	}
	h.pool, err = OpenDaily(ctx, h.path, h.config, h.authority, h.node, h.sessions)
	if err != nil || h.pool == oldPool || len(h.pool.nonceMemo) != 1 || dailyRows(t, h.pool) != before {
		t.Fatal("cold full audit/reopen", err)
	}
	// Only private memo state is perturbed to distinguish reuse from source
	// acceptance. An invalid cached header must fail; disk bytes stay untouched.
	for k, v := range oldPool.nonceMemo {
		v.Header = "old owner"
		oldPool.nonceMemo[k] = v
	}
	if got, e := h.pool.Lookup(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce); e != nil || got != first {
		t.Fatal("memo shared across owners", e)
	}
	for k, v := range h.pool.nonceMemo {
		v.Header = "poisoned"
		h.pool.nonceMemo[k] = v
	}
	if _, err = h.pool.Lookup(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce); !errors.Is(err, ErrCorrupt) || dailyRows(t, h.pool) != before {
		t.Fatal("invalid memo header not refused", err)
	}
}

func TestDailySourceFailedFileCloseRetainsOwner(t *testing.T) {
	p := newPool(Config{Version: Version2}, nil, nil, nil)
	f, err := os.Create(filepath.Join(t.TempDir(), "file"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	closeErr := f.Close()
	if closeErr == nil {
		t.Fatal("closed-descriptor fixture")
	}
	got, err := failedDailyFile(p, f, closeErr)
	if got != p || got.failedFile != f || !errors.Is(err, ErrPoisoned) || !errors.Is(got.Close(), ErrPoisoned) {
		t.Fatal("failed original closer discarded", err)
	}
}
