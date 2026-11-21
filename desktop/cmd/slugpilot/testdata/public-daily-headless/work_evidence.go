package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/prospectworker"
	"path/filepath"
	"reflect"
	"sort"
	"time"
)

func snapshot(ctx context.Context, c *accountpilot.RemotePoolController) accountpilot.RemotePoolControllerSnapshot {
	work, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		s, e := c.Snapshot(work)
		if e == nil {
			return s
		}
		if e != pilot.ErrRemotePoolBusy {
			check(e, "original snapshot")
		}
		check(pauseActual(work, 100*time.Millisecond), "snapshot bound")
	}
}
func openController(ctx context.Context, root string, f publicFixture, in pilot.RemotePoolInputs, owner **accountpilot.RemotePoolController) string {
	call, cancel := context.WithTimeout(ctx, 60*time.Second)
	c, e := accountpilot.OpenDailyRemotePoolController(call, root, f.Options, in)
	*owner = c
	cancel()
	check(e, "existing-only daily controller")
	s := snapshot(ctx, c)
	id, e := pilot.DailyRemotePoolConfigID(in.Config)
	check(e, "original remote ID")
	require(s.Version == 2 && s.ConfigID == id && s.State == "signed_out" && s.View == nil && s.Remote == nil, "released Open context")
	call, cancel = context.WithTimeout(ctx, 90*time.Second)
	e = c.LoginFixture(call, f.Fixture)
	cancel()
	check(e, "signed original fixture login")
	s = snapshot(ctx, c)
	require(s.State == "signed_in" && s.View != nil && s.Remote != nil && s.Remote.State == "stopped" && s.Remote.Attempts == 0, "released Login context retains stopped owner")
	require(s.View.GymPublicKey == f.ExpectedLocal.Recipient.Config.Identity.GymPublicKey && s.View.AccountID == f.ExpectedLocal.Recipient.Config.AccountID && s.Remote.SourceBindingID == in.Config.Miners.SourceID && s.Remote.OriginalSourceBindingID == f.ExpectedLocal.OriginalSourceBindingID && s.Remote.PayoutAddress == f.ExpectedLocal.Recipient.Recipient.Address, "original wallet and independently selected remote")
	return s.View.ViewID
}

type outboxState struct {
	Records      uint64 `json:"records"`
	Pending      uint64 `json:"pending"`
	MetaSHA256   string `json:"meta_sha256"`
	MetaBytes    int    `json:"meta_bytes"`
	ScanSHA256   string `json:"scan_sha256"`
	CursorSHA256 string `json:"cursor_sha256"`
	IssuePending bool   `json:"issue_pending"`
}

func readOutbox(root string, c pilot.RemotePoolConfig) ([]prospectworker.PoolEntry, outboxState) {
	b, e := prospectworker.OpenDailyPoolOutboxAudit(filepath.Join(root, "outbox"), workerConfig(c))
	check(e, "closed actual daily outbox audit")
	rows, re := b.List(0, 100)
	ce := b.Close()
	check(errors.Join(re, ce), "outbox audit join")
	require(len(rows) > 0 && len(rows) <= 64, "bounded retained rows")
	// The concrete read-only full audit above verifies all entries, indexes,
	// configuration and metadata before these three exact public projections.
	db, e := leveldb.OpenFile(filepath.Join(root, "outbox", "db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true, Strict: opt.StrictAll})
	check(e, "readonly outbox control projection")
	closed := false
	defer func() {
		if !closed {
			check(db.Close(), "partial outbox projection close")
		}
	}()
	raw, e := db.Get([]byte("meta"), nil)
	check(e, "audited original outbox meta")
	require(len(raw) > 0 && len(raw) <= 65536, "outbox meta bound")
	var m struct {
		IssuePending *bool                     `json:"issue_pending"`
		Records      uint64                    `json:"records"`
		Pending      uint64                    `json:"pending"`
		Config       prospectworker.PoolConfig `json:"config"`
	}
	check(json.Unmarshal(raw, &m), "audited hold projection")
	require(m.IssuePending != nil && m.Config == workerConfig(c) && m.Records == uint64(len(rows)) && m.Pending <= m.Records, "same daily hold/config")
	v := outboxState{Records: m.Records, Pending: m.Pending, MetaSHA256: hx(digest(raw)), MetaBytes: len(raw), IssuePending: *m.IssuePending}
	for _, key := range []string{"scan", "cursor"} {
		raw, e = db.Get([]byte(key), nil)
		if errors.Is(e, leveldb.ErrNotFound) {
			continue
		}
		check(e, "audited scan/cursor projection")
		limit := prospectwork.MaxTemplateSize + (64 << 10)
		if key == "cursor" {
			limit = 8
		}
		require(len(raw) > 0 && len(raw) <= limit, "scan/cursor bound")
		if key == "scan" {
			v.ScanSHA256 = hx(digest(raw))
		} else {
			v.CursorSHA256 = hx(digest(raw))
		}
	}
	e = db.Close()
	closed = true
	check(e, "outbox projection joined")
	require((v.ScanSHA256 == "") == (v.CursorSHA256 == "") && (!v.IssuePending || v.ScanSHA256 == ""), "original hold and scan exclusive")
	return rows, v
}
func pauseActual(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func transient(e error) bool {
	f, ok := e.(*poolservice.ClientError)
	return ok && f != nil && !f.OutcomeUnknown && (f.Code == "busy" || f.Code == "rate_limited")
}
func readLookup(ctx context.Context, c *poolservice.Client, row prospectworker.PoolEntry) (poolshare.Submission, error) {
	call, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		v, e := c.Lookup(call, row.Work, row.Nonce)
		if !transient(e) {
			return v, e
		}
		if e = pauseActual(call, time.Second); e != nil {
			return poolshare.Submission{}, e
		}
	}
}
func sourceReceipt(s poolshare.Submission) poolshare.Submission {
	s.BlockState = ""
	s.BlockObservedAtMS = 0
	return s
}

type rowEvidence struct {
	Outbox           outboxState                `json:"outbox"`
	Rows             []prospectworker.PoolEntry `json:"rows"`
	Accepted         []poolshare.Submission     `json:"accepted"`
	Authorizations   int                        `json:"authorizations"`
	AcceptedFighters int                        `json:"accepted_fighters"`
	PendingFighters  int                        `json:"pending_fighters"`
	LookupMissing    []uint64                   `json:"lookup_missing"`
}

func auditRows(ctx context.Context, root string, c pilot.RemotePoolConfig, client *poolservice.Client, n *node) rowEvidence {
	rows, state := readOutbox(root, c)
	out := rowEvidence{Rows: rows, Outbox: state, Accepted: []poolshare.Submission{}, LookupMissing: []uint64{}}
	for i, r := range out.Rows {
		require(r.Sequence == uint64(i+1), "contiguous original outbox")
		require(r.Fighter != nil, "original all-ones prospect result")
		prepared, e := poolservice.PrepareDailyWork(c.Source, c.ExpectedLocal.Recipient.Config.Identity.GymPublicKey, r.Work)
		check(e, "exact actual retained work")
		checked, e := prepared.CheckNonce(ctx, r.Nonce)
		check(e, "actual normal nonce proof")
		require(checked.WorkHash == r.WorkHash && checked.Pool == r.PoolWin && (!checked.Pool || checked.Block), "pool-only target is actual block target")
		f := r.Fighter
		if f.Authorization != nil {
			check(gymauth.VerifyAuthorization(f.Job.Context, f.Candidate.Proof, *f.Authorization), "original vault signature")
			require(f.Authorization.PublicKey == c.ExpectedLocal.Recipient.Config.Identity.GymPublicKey, "same original signer")
			out.Authorizations++
		}
		switch f.AdmissionState {
		case "accepted":
			require(f.Receipt != nil && f.Authorization != nil && f.Receipt.Record.Authorization == *f.Authorization && f.Receipt.Record.Job == f.Job && reflect.DeepEqual(f.Receipt.Record.Proof, f.Candidate.Proof), "exact issuer receipt")
			out.AcceptedFighters++
		case "pending", "needs-signature":
			require(f.Receipt == nil, "no invented birth")
			switch f.LastError {
			case "", "capacity", "busy", "rate_limited", "outcome_unknown", "unavailable":
			default:
				panic("unexpected fighter delivery refusal")
			}
			out.PendingFighters++
		case "expired":
			require(f.Receipt == nil && f.LastError == "expired", "observed actual expiry only")
		default:
			panic("unexpected fighter terminal state")
		}
		if !r.PoolWin {
			require(r.PoolReceipt == nil, "prospect-only never credit")
			continue
		}
		got, e := readLookup(ctx, client, r)
		if e != nil {
			f, ok := e.(*poolservice.ClientError)
			require(ok && !f.OutcomeUnknown && f.Code == "not_found" && r.PoolState == "pending", "exact interrupted pool result only")
			out.LookupMissing = append(out.LookupMissing, r.Sequence)
			continue
		}
		require(got.WorkHash == r.WorkHash && got.Block && got.Pool, "same real winning work")
		if r.PoolReceipt != nil {
			require(sourceReceipt(*r.PoolReceipt) == sourceReceipt(got), "exact retained receipt")
		}
		if got.AcceptedSequence == 0 {
			require(r.PoolState == "pending", "uncredited intent remains pending")
			continue
		}
		var canonical string
		n.rpc.must(ctx, "getblockhash", []any{got.BlockHeight}, &canonical)
		require(canonical == got.WorkHash, "actual winning canonical block")
		out.Accepted = append(out.Accepted, got)
	}
	sort.Slice(out.Accepted, func(i, j int) bool { return out.Accepted[i].AcceptedSequence < out.Accepted[j].AcceptedSequence })
	require(len(out.Accepted) >= 1 && len(out.Accepted) <= 2 && out.Authorizations > 0, "actual receipt and original signer required")
	for i, r := range out.Accepted {
		require(r.AcceptedSequence == uint64(i+1), "complete exact accepted source prefix")
	}
	return out
}
func requireNoControllerFailure(s accountpilot.RemotePoolControllerSnapshot) {
	require(s.Remote != nil, "original remote remains")
	switch s.Remote.LastError {
	case "", "busy", "rate_limited", "capacity", "outcome_unknown", "unavailable":
	default:
		panic(fmt.Sprintf("unexpected original worker code %q", s.Remote.LastError))
	}
}
