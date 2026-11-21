package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"reflect"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/desktop/pooloperator"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
)

type lateAudit struct {
	Submissions       uint64                      `json:"submissions"`
	Accepted          uint64                      `json:"accepted"`
	Works             uint64                      `json:"works"`
	AuthorityJobs     uint64                      `json:"authority_jobs"`
	Births            uint64                      `json:"births"`
	Entries           []poolshare.AccountingEntry `json:"entries"`
	Rounds            []poolround.Record          `json:"rounds"`
	Credits           []poolsettle.Credit         `json:"credits"`
	Batches           []poolsettle.Batch          `json:"batches"`
	Report            poolsettle.Report           `json:"report"`
	ReadOnlyUnchanged bool                        `json:"read_only_unchanged"`
}

// Only the four named public evidence/accounting stores. No wallet, custody,
// credentials, arbitrary LevelDB iteration or private fixture tree is read.
func latePublicFiles(root string) map[string]string {
	out := map[string]string{}
	for _, name := range []string{"issuer", "pool", "rounds", "settlement"} {
		dir := filepath.Join(root, "operator", name)
		check(filepath.WalkDir(dir, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			require(info.Mode().IsRegular() && info.Size() <= 128<<20 && len(out) < 4096, "bounded regular public audit inventory")
			out[path] = hx(digest(readBound(path, 128<<20)))
			return nil
		}), "public audit files")
	}
	return out
}

// Caller must have joined all runtime/CLI owners of this exact root first.
func lateClosedAudit(ctx context.Context, root string, s pooloperator.Setup) (out lateAudit) {
	before := latePublicFiles(root)
	var closers []func() error
	defer func() {
		for i := len(closers) - 1; i >= 0; i-- {
			check(closers[i](), "partial public audit close")
		}
	}()
	profile, e := registry.NewOperatorProfile(s.Authority)
	check(e, "original audit profile")
	issuer, e := registry.OpenOperatorManifestAuthorityAudit(filepath.Join(root, "operator", "issuer"), profile)
	check(e, "original public issuer audit")
	closers = append(closers, issuer.Close)
	info, e := issuer.Info()
	check(e, "public birth count")
	out.Births = info.Records
	out.AuthorityJobs = info.Jobs
	pool, e := poolshare.OpenAudit(filepath.Join(root, "operator", "pool"), s.Pool, issuer)
	check(e, "original pool physical audit")
	closers = append(closers, pool.Close)
	entries, e := pool.AccountingPage(ctx, 0, 0, 8)
	check(e, "bounded complete acceptance audit")
	require(entries.Config == s.Pool && entries.Through == uint64(len(entries.Entries)) && entries.Through <= 5, "complete retained acceptance prefix")
	out.Entries = entries.Entries
	rounds, e := poolround.OpenAudit(filepath.Join(root, "operator", "rounds"), s.RoundLimits, pool)
	check(e, "original round physical audit")
	closers = append(closers, rounds.Close)
	out.Rounds, e = rounds.List(ctx, 0, 8)
	check(e, "bounded original rounds")
	settle, e := poolsettle.OpenAudit(filepath.Join(root, "operator", "settlement"), s.Settlement, rounds)
	check(e, "original settlement full physical audit")
	closers = append(closers, settle.Close)
	out.Report, e = settle.View(ctx)
	check(e, "original saved report")
	for len(closers) > 0 {
		i := len(closers) - 1
		close := closers[i]
		closers = closers[:i]
		check(close(), "public audit close")
	}
	// Pool.OpenAudit audits every physical work/submission/nonce/sequence row
	// against these counters, including unaccepted block intents. Thus accepted
	// prefix equality cannot hide a newly saved unseen B row after an unknown reply.
	poolDB, e := leveldb.OpenFile(filepath.Join(root, "operator", "pool", "db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true, Strict: opt.StrictAll})
	check(e, "readonly pool counter projection")
	metaRaw, e := poolDB.Get([]byte("meta"), nil)
	poolClose := poolDB.Close()
	check(e, "exact public pool meta")
	check(poolClose, "readonly pool close")
	require(len(metaRaw) > 0 && len(metaRaw) <= 16<<10, "pool metadata bound")
	var meta struct {
		Config      poolshare.Config `json:"config"`
		Submissions uint64           `json:"submissions"`
		Accepted    uint64           `json:"accepted"`
		Works       uint64           `json:"works"`
	}
	check(json.Unmarshal(metaRaw, &meta), "audited pool counters")
	require(meta.Config == s.Pool && meta.Accepted == entries.Through && meta.Submissions <= 8 && meta.Works <= 4, "original physical counters")
	out.Submissions, out.Accepted, out.Works = meta.Submissions, meta.Accepted, meta.Works
	// OpenAudit above validates the complete public state and all original source
	// links. The ordinary API intentionally does not export Batch/Claims: inspect
	// exactly its single existing public financial key while every owner is closed.
	db, e := leveldb.OpenFile(filepath.Join(root, "operator", "settlement", "db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true, Strict: opt.StrictAll})
	check(e, "readonly public financial projection")
	raw, e := db.Get([]byte("slug-pool-settlement-v1"), nil)
	ce := db.Close()
	check(e, "exact public financial key")
	check(ce, "readonly financial close")
	require(len(raw) > 0 && len(raw) <= 64<<20, "existing financial state bound")
	var saved struct {
		Config      poolsettle.Config   `json:"config"`
		Source      poolshare.Config    `json:"source"`
		Credits     []poolsettle.Credit `json:"credits"`
		Batches     []poolsettle.Batch  `json:"batches"`
		LastClockMS int64               `json:"last_clock_ms"`
		Report      poolsettle.Report   `json:"report"`
	}
	check(json.Unmarshal(raw, &saved), "public financial projection decode")
	require(reflect.DeepEqual(saved.Config, s.Settlement) && saved.Source == s.Pool && reflect.DeepEqual(saved.Report, out.Report), "audited exact financial projection")
	out.Credits = saved.Credits
	out.Batches = saved.Batches
	require(reflect.DeepEqual(before, latePublicFiles(root)), "physical readonly audit changed public evidence")
	out.ReadOnlyUnchanged = true
	return
}

func lateAssertWinner(a lateAudit, w poolshare.MiningWork, r poolshare.Submission, births uint64, confirmed bool) {
	require(a.Submissions == 5 && a.Accepted == 5 && a.Works == 3 && a.AuthorityJobs == 3 && len(a.Entries) == 5 && len(a.Rounds) == 3 && a.Births == births, "one accepted row/round, no expired birth")
	count := 0
	for _, entry := range a.Entries {
		if entry.Share.DuplicateKey == r.DuplicateKey {
			count++
			require(entry.Share.AcceptedSequence == 5 && entry.Job.ID == w.Job.ID && entry.Share.WorkHash == r.WorkHash, "original late winner entry")
		}
	}
	require(count == 1, "unique late accepted identity")
	round := a.Rounds[2]
	require(round.Round.FirstAccepted == 3 && round.Round.LastAccepted == 5 && len(round.Round.ShareKeys) == 3 && round.Round.ShareKeys[2] == r.DuplicateKey && round.Round.BlockHash == r.WorkHash && round.Round.BlockHeight == 104 && round.Round.RewardAtoms == 1000000000, "D81 appended window3..5, never rebased old rounds")
	require(len(round.Round.Allocations) == 1 && round.Round.Allocations[0] == (poolround.Allocation{GymID: w.Job.GymID, Shares: 3, Numerator: "1000000000", Denominator: "1"}), "same-gym exact gross allocation")
	require((round.Confirmation != nil) == confirmed, "exact confirmation boundary")
}
