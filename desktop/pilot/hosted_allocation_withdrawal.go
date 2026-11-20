// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slugwallet/wallet"
)

// Only private bookkeeping Lookup uses View. Actionable draft/sign methods on
// the allocated implementation independently require the original Withdrawal.
type poolWithdrawalWallet interface {
	Prepare(context.Context, wallet.CandidateWithdrawalRequest) (wallet.CandidateWithdrawalDraft, error)
	Lookup(context.Context, string) (wallet.CandidateWithdrawalRecord, error)
	Sign(context.Context, string, string) (wallet.CandidateWithdrawalSigned, error)
	Retain(context.Context, string, string, string) error
	Abandon(context.Context, string, string) error
	OperationIDs(context.Context) ([]string, error)
	Empty(context.Context) (bool, error)
	Close() error
}
type allocatedWithdrawalWallet struct {
	a *wallet.HostedAllocator
	s *wallet.AllocatedWithdrawals
}

func openAllocatedWithdrawalWallet(ctx context.Context, a *wallet.HostedAllocator, binding string, create bool) (poolWithdrawalWallet, error) {
	var s *wallet.AllocatedWithdrawals
	var e error
	if create {
		s, e = a.CreateWithdrawals(ctx, binding)
	} else {
		s, e = a.OpenWithdrawals(ctx, binding)
	}
	if e != nil {
		return nil, e
	}
	return &allocatedWithdrawalWallet{a, s}, nil
}
func (s *allocatedWithdrawalWallet) Prepare(ctx context.Context, r wallet.CandidateWithdrawalRequest) (wallet.CandidateWithdrawalDraft, error) {
	if r.Version != 1 {
		return wallet.CandidateWithdrawalDraft{}, wallet.ErrAllocationInvalid
	}
	ack, e := s.a.Resolve(ctx, r.OperationID)
	if errors.Is(e, wallet.ErrAllocationNotFound) {
		p, err := s.s.Plan(ctx, r.OperationID, r)
		if err != nil {
			return wallet.CandidateWithdrawalDraft{}, err
		}
		reservation, err := s.a.Reserve(ctx, p)
		if err != nil {
			return wallet.CandidateWithdrawalDraft{}, err
		}
		receipt, err := s.a.Consume(ctx, reservation)
		if err != nil {
			return wallet.CandidateWithdrawalDraft{}, err
		}
		ack, e = s.a.Acknowledge(ctx, receipt)
	}
	// A lost unsigned authoring plan cannot be reconstructed by coin reselection.
	// The explicit Abandon action burns that exact range before a fresh operation.
	if e != nil {
		return wallet.CandidateWithdrawalDraft{}, e
	}
	final, e := s.a.Finalize(ctx, ack)
	if e != nil {
		return wallet.CandidateWithdrawalDraft{}, e
	}
	d, e := final.Draft()
	if e != nil {
		return d, e
	}
	if d.OperationID != r.OperationID || d.Recipient != r.Recipient || d.AmountAtoms != r.AmountAtoms {
		return wallet.CandidateWithdrawalDraft{}, wallet.ErrAllocationConflict
	}
	return d, nil
}
func (s *allocatedWithdrawalWallet) Lookup(c context.Context, id string) (wallet.CandidateWithdrawalRecord, error) {
	return s.s.InspectRecord(c, id)
}
func (s *allocatedWithdrawalWallet) Sign(c context.Context, id, d string) (wallet.CandidateWithdrawalSigned, error) {
	return s.s.Sign(c, id, d)
}
func (s *allocatedWithdrawalWallet) Retain(c context.Context, id, d, t string) error {
	return s.s.Retain(c, id, d, t)
}
func (s *allocatedWithdrawalWallet) Abandon(c context.Context, id, d string) error {
	return s.s.Abandon(c, id, d)
}
func (s *allocatedWithdrawalWallet) OperationIDs(c context.Context) ([]string, error) {
	return s.s.OperationIDs(c)
}
func (s *allocatedWithdrawalWallet) Empty(c context.Context) (bool, error) { return s.s.Empty(c) }
func (s *allocatedWithdrawalWallet) Close() error                          { return s.s.Close() }
func (s *allocatedWithdrawalWallet) abandonUnprepared(ctx context.Context, id string) error {
	_, e := s.a.Resolve(ctx, id)
	if errors.Is(e, wallet.ErrAllocationNotFound) {
		return nil
	}
	if !errors.Is(e, wallet.ErrAllocationResumeRequired) {
		if e == nil {
			return wallet.ErrAllocationConflict
		}
		return e
	}
	r, e := s.a.Resume(ctx, id)
	if e != nil {
		return e
	}
	receipt, e := s.a.Burn(ctx, r)
	if e != nil {
		return e
	}
	_, e = s.a.Acknowledge(ctx, receipt)
	return e
}
