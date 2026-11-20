// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/walletwithdraw"
)

func WithdrawalBindingForSnapshot(s PoolSnapshot, journal string) (WithdrawalBinding, error) {
	if ValidatePoolSnapshot(s) != nil || s.View == nil || s.Account.State != "signed_in" || !payoutNonzeroID(journal) {
		return WithdrawalBinding{}, gymstable.ErrInvalid
	}
	f, e := gymaccount.FixtureIdentity(s.Account.Fixture)
	if e != nil {
		return WithdrawalBinding{}, e
	}
	p := walletwithdraw.Pin{Version: 1, Network: f.Network, Genesis: f.Genesis, AccountAuthorityID: s.View.AccountAuthorityID, AccountID: s.View.AccountID, Fixture: s.Account.Fixture, WalletProfile: f.Profile, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID, PoolSourceBindingID: s.SourceBindingID}
	id, e := walletwithdraw.BindingID(walletwithdraw.Identity{JournalID: journal, Pin: p})
	if e != nil {
		return WithdrawalBinding{}, e
	}
	b := WithdrawalBinding{View: *s.View, SourceBindingID: s.SourceBindingID, JournalID: journal, WithdrawalBindingID: id}
	if !withdrawalBindingValid(b) {
		return WithdrawalBinding{}, gymstable.ErrInvalid
	}
	return b, nil
}
func withdrawalProjection(b WithdrawalBinding, o walletwithdraw.Operation) (WithdrawalOperation, error) {
	i := o.Intent
	out := WithdrawalOperation{Version: 1, OperationID: o.OperationID, JournalID: b.JournalID, WithdrawalBindingID: b.WithdrawalBindingID, Sequence: o.Sequence, Revision: o.Revision, CreatedAtMS: i.CreatedAtMS, UpdatedAtMS: o.UpdatedAtMS, PreparedAtMS: o.PreparedAtMS, Stage: string(o.Stage), Intent: WithdrawalIntent{i.Destination, i.RecipientScript, i.RecipientAtoms, i.FeeRateAtomsPerKB, i.MaxFeeAtoms}, IntentDigest: o.IntentDigest}
	if o.Draft != nil {
		d := o.Draft
		raw, e := hex.DecodeString(d.UnsignedTxHex)
		if e != nil {
			return WithdrawalOperation{}, e
		}
		txid := chainhash.DoubleHashH(raw).String()
		clear(raw)
		out.Draft = &WithdrawalDraftView{Version: 1, DraftDigest: o.DraftDigest, UnsignedTxID: txid, Destination: d.Recipient, RecipientScript: d.RecipientScript, AmountAtoms: d.AmountAtoms, FeeAtoms: d.FeeAtoms, TotalDebitAtoms: d.AmountAtoms + d.FeeAtoms, InputCount: uint32(len(d.Inputs)), InputAtoms: d.TotalInputAtoms, RecipientOutputIndex: d.RecipientOutputIndex, ChangeAtoms: d.Change.Atoms, ChangeOutputIndex: d.Change.OutputIndex, PreparedAtMS: o.PreparedAtMS, ObservedTip: d.TipHash, ObservedHeight: d.TipHeight}
	}
	if o.Signed != nil {
		out.TxID, out.SignedSHA256 = o.Signed.TxID, o.Signed.RawSHA256
	}
	if o.Dispatch != nil {
		d := o.Dispatch
		a := d.Authorization
		out.Dispatch = &WithdrawalDispatch{Attempt: d.Attempt, Authorization: WithdrawalAuthorization{a.ActionID, a.ReviewID, a.ViewID, a.Generation, a.AuthorizedAtMS}, IntentAtMS: d.IntentAtMS, Result: string(d.Result), ResultAtMS: d.ResultAtMS, Error: d.Error}
	}
	if o.Observation != nil {
		o := o.Observation
		out.Observation = &WithdrawalObservation{Version: o.Version, TxID: o.TxID, ObservedAtMS: o.ObservedAtMS, State: string(o.State), NodeTip: o.NodeTip, NodeHeight: o.NodeHeight, Confirmations: o.Confirmations, BlockHash: o.BlockHash, Error: o.Error}
	}
	if ValidateWithdrawalOperation(out) != nil {
		return WithdrawalOperation{}, gymstable.ErrInvalid
	}
	return out, nil
}
func cloneWithdrawalAction(a WithdrawalAction) WithdrawalAction {
	if a.Operation != nil {
		o := *a.Operation
		if o.Draft != nil {
			d := *o.Draft
			o.Draft = &d
		}
		if o.Dispatch != nil {
			d := *o.Dispatch
			o.Dispatch = &d
		}
		if o.Observation != nil {
			d := *o.Observation
			o.Observation = &d
		}
		a.Operation = &o
	}
	return a
}
func withdrawalWalletDraft(v any) (walletwithdraw.Draft, error) {
	raw, e := json.Marshal(v)
	if e != nil {
		return walletwithdraw.Draft{}, e
	}
	return walletwithdraw.ParseDraft(raw)
}
