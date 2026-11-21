package main

import (
	"context"
	"encoding/binary"
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

type dailyAuditHeader struct {
	Application  string `json:"application"`
	Version      uint32 `json:"version"`
	ConfigID     string `json:"config_id"`
	SourceID     string `json:"source_id"`
	Revision     uint64 `json:"revision"`
	Credits      uint64 `json:"credits"`
	Batches      uint64 `json:"batches"`
	Events       uint64 `json:"events"`
	LogicalBytes uint64 `json:"logical_bytes"`
	PendingBatch uint64 `json:"pending_batch"`
	LastDay      string `json:"last_day"`
	LastClockMS  int64  `json:"last_clock_ms"`
}
type dailyAuditBody struct {
	Version     uint32 `json:"version"`
	Sequence    uint64 `json:"sequence"`
	Day         string `json:"day"`
	CreatedAtMS int64  `json:"created_at_ms"`
	Inputs      []struct {
		TxID  string `json:"txid"`
		Index uint32 `json:"index"`
	} `json:"inputs"`
	Payments []poolsettle.Payment `json:"payments"`
	FeeAtoms int64                `json:"fee_atoms"`
	Change   *struct {
		Index        uint32 `json:"index"`
		Atoms        int64  `json:"atoms"`
		Script       string `json:"script"`
		ClaimCount   uint64 `json:"claim_count"`
		ClaimsDigest string `json:"claims_digest"`
	} `json:"change"`
	TxID string `json:"txid"`
	Raw  string `json:"raw"`
}
type lateAudit struct {
	Submissions       uint64                      `json:"submissions"`
	Accepted          uint64                      `json:"accepted"`
	Works             uint64                      `json:"works"`
	AuthorityJobs     uint64                      `json:"authority_jobs"`
	Births            uint64                      `json:"births"`
	Entries           []poolshare.AccountingEntry `json:"entries"`
	Rounds            []poolround.Record          `json:"rounds"`
	Credits           []poolsettle.Credit         `json:"credits"`
	Header            dailyAuditHeader            `json:"header"`
	Body              dailyAuditBody              `json:"body"`
	BodySHA256        string                      `json:"body_sha256"`
	BodyBytes         int                         `json:"body_bytes"`
	Delivery          string                      `json:"delivery"`
	Report            poolsettle.Report           `json:"report"`
	ReadOnlyUnchanged bool                        `json:"read_only_unchanged"`
}

// Only named public accounting stores; never wallets, credentials or seed files.
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
			require(info.Mode().IsRegular() && info.Size() <= 128<<20 && len(out) < 4096, "bounded public audit inventory")
			out[path] = hx(digest(readBound(path, 128<<20)))
			return nil
		}), "public audit inventory")
	}
	return out
}
func auditKey(prefix string, n uint64) []byte {
	k := make([]byte, len(prefix)+8)
	copy(k, prefix)
	binary.BigEndian.PutUint64(k[len(prefix):], n)
	return k
}

// Called only after every original runtime/CLI has joined and released this root.
// Full production audits authorize the subsequent bounded public projection.
func lateClosedAudit(ctx context.Context, root string, s pooloperator.Setup) (out lateAudit) {
	before := latePublicFiles(root)
	var closers []func() error
	failedClose := false
	defer func() {
		if failedClose {
			return
		}
		for i := len(closers) - 1; i >= 0; i-- {
			e := closers[i]()
			if e != nil {
				failedClose = true
				check(e, "partial original audit close")
				return
			}
		}
	}()
	profile, e := registry.NewOperatorProfile(s.Authority)
	check(e, "audit original profile")
	issuer, e := registry.OpenOperatorManifestAuthorityAudit(filepath.Join(root, "operator", "issuer"), profile)
	check(e, "full issuer audit")
	closers = append(closers, issuer.Close)
	info, e := issuer.Info()
	check(e, "issuer counters")
	out.AuthorityJobs = info.Jobs
	out.Births = info.Records
	pool, e := poolshare.OpenDailyAudit(ctx, filepath.Join(root, "operator", "pool"), s.Pool, issuer)
	check(e, "full daily source audit")
	closers = append(closers, pool.Close)
	entries, e := pool.AccountingPage(ctx, 0, 0, 8)
	check(e, "complete bounded accepted prefix")
	require(entries.Config == s.Pool && entries.Through == uint64(len(entries.Entries)) && entries.Through <= 3, "original complete prefix")
	out.Entries = entries.Entries
	rounds, e := poolround.OpenDailyAudit(ctx, filepath.Join(root, "operator", "rounds"), s.RoundLimits, pool)
	check(e, "full daily round audit")
	closers = append(closers, rounds.Close)
	out.Rounds, e = rounds.List(ctx, 0, 8)
	check(e, "bounded rounds")
	settle, e := poolsettle.OpenDailyAudit(ctx, filepath.Join(root, "operator", "settlement"), s.Settlement, rounds)
	check(e, "full indexed settlement/source audit")
	closers = append(closers, settle.Close)
	out.Report, e = settle.View(ctx)
	check(e, "original saved report")
	for len(closers) > 0 {
		i := len(closers) - 1
		c := closers[i]
		closeErr := c()
		if closeErr != nil {
			failedClose = true
			check(closeErr, "original public audit close")
		}
		closers = closers[:i]
	}
	poolDB, e := leveldb.OpenFile(filepath.Join(root, "operator", "pool", "db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true, Strict: opt.StrictAll})
	check(e, "readonly source counters")
	metaRaw, e := poolDB.Get([]byte("meta"), nil)
	ce := poolDB.Close()
	check(e, "exact source metadata")
	check(ce, "source projection close")
	require(len(metaRaw) > 0 && len(metaRaw) <= 16<<10, "source metadata bound")
	var meta struct {
		Config      poolshare.Config `json:"config"`
		Submissions uint64           `json:"submissions"`
		Accepted    uint64           `json:"accepted"`
		Works       uint64           `json:"works"`
	}
	check(json.Unmarshal(metaRaw, &meta), "audited source counters")
	require(meta.Config == s.Pool && meta.Accepted == entries.Through && meta.Submissions <= 3 && meta.Works <= 3, "all physical rows independently audited")
	out.Submissions, out.Accepted, out.Works = meta.Submissions, meta.Accepted, meta.Works
	db, e := leveldb.OpenFile(filepath.Join(root, "operator", "settlement", "db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true, Strict: opt.StrictAll})
	check(e, "readonly indexed projection")
	closed := false
	defer func() {
		if !closed {
			check(db.Close(), "partial indexed projection close")
		}
	}()
	get := func(key []byte, limit int, target any) []byte {
		raw, e := db.Get(key, nil)
		check(e, "exact audited index key")
		require(len(raw) > 0 && len(raw) <= limit, "bounded exact index row")
		check(json.Unmarshal(raw, target), "public index projection")
		return raw
	}
	get([]byte("head"), 128<<10, &out.Header)
	require(out.Header.Version == 2 && out.Header.Application == "slug-pool-settlement-daily" && out.Header.ConfigID == s.WalletGuard.SettlementConfigID && out.Header.SourceID == s.WalletGuard.SourceBindingID && out.Header.Batches == 1 && out.Header.Credits >= 1 && out.Header.Credits <= 2 && out.Header.Events == out.Header.Credits+1, "fixed tiny original daily ledger")
	var report poolsettle.Report
	get([]byte("report"), 8<<20, &report)
	require(reflect.DeepEqual(report, out.Report), "audited original saved report")
	body := get(auditKey("b/", 1), 2<<20, &out.Body)
	out.BodySHA256 = hx(digest(body))
	out.BodyBytes = len(body)
	var state struct {
		State string `json:"state"`
	}
	get(auditKey("bs/", 1), 256, &state)
	out.Delivery = state.State
	out.Credits = []poolsettle.Credit{}
	for seq := uint64(1); seq <= out.Header.Credits; seq++ {
		var credit poolsettle.Credit
		get(auditKey("c/", seq), 128<<10, &credit)
		out.Credits = append(out.Credits, credit)
	}
	// The full audit above has already checked all e/ references and inverse keys.
	// Reading the exact three possible e/ bodies binds their tiny event projection.
	for seq := uint64(1); seq <= out.Header.Events; seq++ {
		var event struct {
			Kind     string `json:"kind"`
			Sequence uint64 `json:"sequence"`
		}
		get(auditKey("e/", seq), 256, &event)
		require((event.Kind == "credit" && event.Sequence <= out.Header.Credits) || (event.Kind == "batch" && event.Sequence == 1), "bounded original event references")
	}
	closeErr := db.Close()
	closed = true
	check(closeErr, "readonly indexed close")
	require(reflect.DeepEqual(before, latePublicFiles(root)), "closed public audit changed files")
	out.ReadOnlyUnchanged = true
	return
}
func lateSameBatch(original lateAudit, got lateAudit) {
	require(original.BodySHA256 == got.BodySHA256 && original.BodyBytes == got.BodyBytes && reflect.DeepEqual(original.Body, got.Body), "entire original signed A body unchanged")
	require(got.Header.Batches == 1 && len(got.Credits) >= 1 && reflect.DeepEqual(original.Credits[0], got.Credits[0]), "original A credit/claims unchanged; only one signed batch")
}
