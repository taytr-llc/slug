package main

import (
	"context"
	"fmt"
	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectworker"
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
func openController(ctx context.Context, root string, f publicFixture, in pilot.RecoverableRemotePoolInputs, owner **accountpilot.RemotePoolController) string {
	require(in.Config.Base.ExpectedLocal == f.ExpectedLocal, "exact original local fixture binding")
	call, cancel := context.WithTimeout(ctx, 60*time.Second)
	c, e := accountpilot.OpenRecoverableDailyRemotePoolController(call, root, f.Options, in)
	*owner = c
	cancel()
	check(e, "existing-only daily controller")
	s := snapshot(ctx, c)
	id, e := pilot.RecoverableRemotePoolConfigID(in.Config)
	check(e, "original remote ID")
	require(s.Version == 2 && s.ConfigID == id && s.State == "signed_out" && s.View == nil && s.Remote == nil, "released Open context")
	call, cancel = context.WithTimeout(ctx, 90*time.Second)
	e = c.LoginFixture(call, f.Fixture)
	cancel()
	check(e, "signed original fixture login")
	s = snapshot(ctx, c)
	require(s.State == "signed_in" && s.View != nil && s.Remote != nil && s.Remote.State == "stopped" && s.Remote.Attempts == 0, "released Login context retains stopped owner")
	require(s.View.GymPublicKey == f.ExpectedLocal.Recipient.Config.Identity.GymPublicKey && s.View.AccountID == f.ExpectedLocal.Recipient.Config.AccountID && s.Remote.SourceBindingID == in.Config.Base.Miners.SourceID && s.Remote.OriginalSourceBindingID == f.ExpectedLocal.OriginalSourceBindingID && s.Remote.PayoutAddress == f.ExpectedLocal.Recipient.Recipient.Address, "original wallet and independently selected remote")
	return s.View.ViewID
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

func auditRows(ctx context.Context, root string, c pilot.RecoverableRemotePoolConfig, client *poolservice.Client, n *node) rowEvidence {
	wc := recoveryWorkerConfig(c)
	check(client.CheckRecoverableDailySelection(poolservice.RecoverableDailySelection{Version: 1, BaseVersion: 2, URL: wc.Base.AuthorityOrigin, PublicKey: wc.Base.PublicKey, Source: wc.Base.Source, IssueProfile: wc.IssueProfile, SettlementConfigID: wc.SettlementConfigID, RecipientScript: wc.RecipientScript, BaseTransportTrustID: wc.Base.TransportTrustID, TransportTrustID: wc.TransportTrustID}), "original selected client for authenticated receipt audit")
	rows, state := readOutbox(root, c)
	out := rowEvidence{Rows: rows, Outbox: state, Accepted: []poolshare.Submission{}, LookupMissing: []uint64{}}
	for i, r := range out.Rows {
		require(r.Sequence == uint64(i+1), "contiguous original outbox")
		require(r.Fighter != nil, "original all-ones prospect result")
		prepared, e := poolservice.PrepareDailyWork(c.Base.Source, c.Base.ExpectedLocal.Recipient.Config.Identity.GymPublicKey, r.Work)
		check(e, "exact actual retained work")
		checked, e := prepared.CheckNonce(ctx, r.Nonce)
		check(e, "actual normal nonce proof")
		require(checked.WorkHash == r.WorkHash && checked.Pool == r.PoolWin && (!checked.Pool || checked.Block), "pool-only target is actual block target")
		f := r.Fighter
		if f.Authorization != nil {
			check(gymauth.VerifyAuthorization(f.Job.Context, f.Candidate.Proof, *f.Authorization), "original vault signature")
			require(f.Authorization.PublicKey == c.Base.ExpectedLocal.Recipient.Config.Identity.GymPublicKey, "same original signer")
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
