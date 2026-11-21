// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"errors"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/registry"
)

var (
	ErrDailyPayoutReadInvalid  = errors.New("daily payout read selection invalid")
	ErrDailyPayoutReadNotFound = errors.New("daily payout read not retained")
)

// These are native read projections, not a local wire protocol or capabilities.
// A future wire must retain the embedded page's decimal-string counters.
type RemoteDailyPayoutBinding struct {
	View                                                                        RemotePoolView
	ConfigID, SourceBindingID, SettlementConfigID, RecipientScript              string
	IssueProfileID, OriginalSourceBindingID, WalletOwnerID, AllocationBindingID string
}

type RemoteDailyPayoutCapture struct {
	Sequence       uint64
	Binding        RemoteDailyPayoutBinding
	CapturedAtMS   int64
	Stale          bool
	First, Current poolsettle.DailyPayoutPage
}

type RemoteDailyPayoutJob struct {
	Sequence, CaptureSequence, BeforeBatch uint64
	State, Error                           string
}

type RemoteDailyPayoutRead struct {
	Job RemoteDailyPayoutJob
	// This may be the previous completed capture after a failed refresh. Its
	// explicit sequence never adopts the failed job's identity.
	Capture *RemoteDailyPayoutCapture
}

func (a *RemotePoolAttachment) dailyPayoutBinding() (RemoteDailyPayoutBinding, error) {
	if a == nil || a.config.Version != 2 {
		return RemoteDailyPayoutBinding{}, ErrRemotePoolConfig
	}
	c, e := remotePoolDailyClientConfig(a.config, "")
	if e != nil {
		return RemoteDailyPayoutBinding{}, e
	}
	b := RemoteDailyPayoutBinding{View: a.view, ConfigID: a.configID,
		SourceBindingID: a.config.Miners.SourceID, SettlementConfigID: c.SettlementConfigID,
		RecipientScript: c.RecipientScript, OriginalSourceBindingID: a.config.ExpectedLocal.OriginalSourceBindingID,
		WalletOwnerID: a.config.ExpectedLocal.WalletOwnerID, AllocationBindingID: a.config.ExpectedLocal.AllocationBindingID}
	if a.recovery != nil {
		b.IssueProfileID, e = registry.IssueProfileID(a.recovery.IssueProfile)
	}
	return b, e
}

func cloneDailyPayoutPage(p poolsettle.DailyPayoutPage) poolsettle.DailyPayoutPage {
	p.Items = append([]poolsettle.DailyPayoutItem{}, p.Items...)
	return p
}
func cloneDailyPayoutCapture(c *RemoteDailyPayoutCapture) *RemoteDailyPayoutCapture {
	if c == nil {
		return nil
	}
	x := *c
	x.First, x.Current = cloneDailyPayoutPage(c.First), cloneDailyPayoutPage(c.Current)
	return &x
}

// Pure shape and linkage checking. Only the retained original concrete client
// followed by the original View publication fence may supply a live capture.
func dailyPayoutPageMatches(p poolsettle.DailyPayoutPage, b RemoteDailyPayoutBinding) (poolsettle.DailyPayoutPage, error) {
	p, e := poolsettle.ValidateDailyPayoutPage(p, b.View.GymID)
	if e != nil || p.BindingID != b.SourceBindingID || p.SettlementConfigID != b.SettlementConfigID || p.RecipientScript != b.RecipientScript {
		return poolsettle.DailyPayoutPage{}, ErrRemotePoolChanged
	}
	return p, nil
}

func freshDailyPayoutCapture(seq uint64, b RemoteDailyPayoutBinding, at int64, p poolsettle.DailyPayoutPage) (*RemoteDailyPayoutCapture, error) {
	p, e := dailyPayoutPageMatches(p, b)
	if e != nil {
		return nil, e
	}
	if seq == 0 || at <= 0 || p.BeforeBatch != p.HighBatch+1 {
		return nil, ErrDailyPayoutReadInvalid
	}
	return &RemoteDailyPayoutCapture{Sequence: seq, Binding: b, CapturedAtMS: at, First: p, Current: cloneDailyPayoutPage(p)}, nil
}

// Repeats of the two retained windows need no remote audit. Every other request
// must be the exact next window; an empty matching Items slice is not EOF.
func dailyPayoutContinuation(c *RemoteDailyPayoutCapture, seq, before uint64) (poolsettle.DailyPayoutRequest, *poolsettle.DailyPayoutPage, error) {
	if c == nil || seq == 0 || seq != c.Sequence || before == 0 {
		return poolsettle.DailyPayoutRequest{}, nil, ErrDailyPayoutReadNotFound
	}
	for _, p := range []poolsettle.DailyPayoutPage{c.First, c.Current} {
		if p.BeforeBatch == before {
			x := cloneDailyPayoutPage(p)
			return poolsettle.DailyPayoutRequest{}, &x, nil
		}
	}
	if c.Stale {
		return poolsettle.DailyPayoutRequest{}, nil, poolsettle.ErrStaleCapture
	}
	if c.Current.Done || before != c.Current.NextBefore {
		return poolsettle.DailyPayoutRequest{}, nil, ErrDailyPayoutReadInvalid
	}
	return poolsettle.DailyPayoutRequest{Revision: c.First.Revision, HighBatch: c.First.HighBatch, BeforeBatch: before}, nil, nil
}

func extendDailyPayoutCapture(c *RemoteDailyPayoutCapture, q poolsettle.DailyPayoutRequest, p poolsettle.DailyPayoutPage) (*RemoteDailyPayoutCapture, error) {
	if c == nil || c.Stale {
		return nil, poolsettle.ErrStaleCapture
	}
	want, cached, e := dailyPayoutContinuation(c, c.Sequence, q.BeforeBatch)
	if e != nil || cached != nil || q != want {
		return nil, ErrDailyPayoutReadInvalid
	}
	p, e = dailyPayoutPageMatches(p, c.Binding)
	if e != nil {
		return nil, e
	}
	if p.Revision != q.Revision || p.HighBatch != q.HighBatch || p.BeforeBatch != q.BeforeBatch || p.Balance != c.First.Balance || p.ObservedAtMS != c.First.ObservedAtMS {
		return nil, ErrRemotePoolChanged
	}
	x := cloneDailyPayoutCapture(c)
	x.Current = p
	return x, nil
}
