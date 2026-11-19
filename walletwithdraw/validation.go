// Copyright (c) 2026 The SLUG developers. ISC license.
package walletwithdraw

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"math"
	"strconv"

	"github.com/taytr-llc/slug/gymaccount"
)

func ValidatePin(p Pin) error {
	f, e := gymaccount.FixtureIdentity(p.Fixture)
	if e != nil || p.Version != 1 || p.Network != f.Network || p.Genesis != f.Genesis || p.WalletProfile != f.Profile || p.RootPublicKey != f.RootPublicKey || p.GymPublicKey != f.GymPublicKey || p.GymID != f.GymID || !hexValue(p.AccountAuthorityID, 32, true) || !hexValue(p.AccountID, 32, true) || !hexValue(p.PoolSourceBindingID, 32, true) {
		return ErrInvalid
	}
	return nil
}
func BindingID(i Identity) (string, error) {
	if ValidatePin(i.Pin) != nil || !hexValue(i.JournalID, 32, true) {
		return "", ErrInvalid
	}
	payload := struct {
		Version   uint32 `json:"version"`
		JournalID string `json:"journal_id"`
		Pin       Pin    `json:"pin"`
	}{1, i.JournalID, i.Pin}
	return digest("SLUG candidate withdrawal binding\x00v1\x00", payload), nil
}
func markerFor(i Identity, b string) marker { return marker{application, 1, i.JournalID, i.Pin, b} }
func validTime(t int64) bool                { return t > 0 && t <= maxTime }
func done(o Operation) bool                 { return o.Stage == StageSubmitted || o.Stage == StageAbandoned }
func same(a, b any) bool {
	x, e := json.Marshal(a)
	y, f := json.Marshal(b)
	return e == nil && f == nil && bytes.Equal(x, y)
}
func intentDigest(v Intent) string { return digest("SLUG candidate withdrawal intent\x00v1\x00", v) }
func validAction(a AuthorizationInput) bool {
	return hexValue(a.ActionID, 32, true) && hexValue(a.ReviewID, 32, true) && hexValue(a.ViewID, 32, true) && a.Generation > 0 && a.Generation <= maxGeneration
}
func validAuthorization(a Authorization, from, to int64) bool {
	return validAction(AuthorizationInput{a.ActionID, a.ReviewID, a.ViewID, a.Generation}) && a.AuthorizedAtMS >= from && a.AuthorizedAtMS <= to && validTime(a.AuthorizedAtMS)
}
func observationValid(v Observation, txid string, from, to int64) bool {
	if v.Version != 1 || v.TxID != txid || !validTime(v.ObservedAtMS) || v.ObservedAtMS < from || v.ObservedAtMS > to || v.NodeHeight > math.MaxInt32 {
		return false
	}
	if v.State == ObservationUnknown {
		return v.NodeTip == "" && v.NodeHeight == 0 && v.Confirmations == 0 && v.BlockHash == "" && v.Error == "node_unavailable"
	}
	if !hexValue(v.NodeTip, 32, true) || v.Error != "" {
		return false
	}
	switch v.State {
	case ObservationNotObserved, ObservationMempool:
		return v.Confirmations == 0 && v.BlockHash == ""
	case ObservationConfirmed:
		return hexValue(v.BlockHash, 32, true) && v.Confirmations > 0 && uint64(v.Confirmations) <= uint64(v.NodeHeight)+1
	default:
		return false
	}
}
func validateOperation(o Operation, binding string, verifySignatures bool) error {
	bad := func() error { return ErrCorrupt }
	in := o.Intent
	if o.Version != 1 || o.Sequence < 1 || o.Sequence > MaxOperations || o.Revision < 1 || !hexValue(o.OperationID, 32, true) || !validTime(o.UpdatedAtMS) || in.Version != 1 || in.OperationID != o.OperationID || in.BindingID != binding || !validTime(in.CreatedAtMS) || in.CreatedAtMS > o.UpdatedAtMS || in.FeeRateAtomsPerKB != 1000 || in.MaxFeeAtoms != 100000 || in.MinConfirmations != 1 || in.InputPurpose != 84 || in.ChangePurpose != 84 || in.Account != 0 || o.IntentDigest != intentDigest(in) {
		return bad()
	}
	p, e := requestScript(Request{1, o.OperationID, in.Destination, in.RecipientAtoms})
	if e != nil || in.RecipientScript != hex.EncodeToString(p) {
		return bad()
	}
	if o.Draft == nil {
		if o.DraftDigest != "" || o.PreparedAtMS != 0 {
			return bad()
		}
	} else {
		d := o.Draft
		dg, e := DraftDigest(*d)
		if e != nil || dg != o.DraftDigest || d.BindingID != binding || d.OperationID != o.OperationID || d.Recipient != in.Destination || d.RecipientScript != in.RecipientScript || d.AmountAtoms != in.RecipientAtoms || !validTime(o.PreparedAtMS) || o.PreparedAtMS < in.CreatedAtMS || o.PreparedAtMS > o.UpdatedAtMS {
			return bad()
		}
	}
	if o.Signed == nil {
		if o.SignAuthorization != nil || o.Dispatch != nil || o.Observation != nil {
			return bad()
		}
	} else {
		if o.Draft == nil || o.SignAuthorization == nil || !validAuthorization(*o.SignAuthorization, o.PreparedAtMS, o.UpdatedAtMS) || !hexValue(o.Signed.TxID, 32, false) || !hexValue(o.Signed.RawSHA256, 32, false) || len(o.Signed.Raw) > 8192 || len(o.Signed.Raw)%2 != 0 || !hexValue(o.Signed.Raw, len(o.Signed.Raw)/2, false) {
			return bad()
		}
		if verifySignatures {
			raw, _ := hex.DecodeString(o.Signed.Raw)
			s, e := ValidateSigned(*o.Draft, raw)
			if e != nil || s != *o.Signed {
				return bad()
			}
		}
		if o.Dispatch != nil {
			d := o.Dispatch
			if d.Attempt == 0 || !validAuthorization(d.Authorization, o.SignAuthorization.AuthorizedAtMS, o.UpdatedAtMS) || d.IntentAtMS != d.Authorization.AuthorizedAtMS {
				return bad()
			}
			switch d.Result {
			case DeliveryPending:
				if d.ResultAtMS != 0 || d.Error != "" {
					return bad()
				}
			case DeliveryAcknowledged, DeliveryUnknown, DeliveryRejected:
				if !validTime(d.ResultAtMS) || d.ResultAtMS < d.IntentAtMS || d.ResultAtMS > o.UpdatedAtMS {
					return bad()
				}
				code := ""
				if d.Result == DeliveryUnknown {
					code = "node_unavailable"
				}
				if d.Result == DeliveryRejected {
					code = "node_rejected"
				}
				if d.Error != code {
					return bad()
				}
			default:
				return bad()
			}
		}
		if o.Observation != nil && !observationValid(*o.Observation, o.Signed.TxID, o.SignAuthorization.AuthorizedAtMS, o.UpdatedAtMS) {
			return bad()
		}
	}
	switch o.Stage {
	case StageIntent:
		if o.Draft != nil || o.Signed != nil || o.Revision != 1 || o.UpdatedAtMS != in.CreatedAtMS {
			return bad()
		}
	case StagePrepared:
		if o.Revision < 2 || o.Draft == nil || o.Signed != nil {
			return bad()
		}
	case StageSigned:
		if o.Revision < 3 || o.Signed == nil || o.Dispatch != nil || o.Observation != nil && (o.Observation.State == ObservationMempool || o.Observation.State == ObservationConfirmed) {
			return bad()
		}
	case StageDispatching:
		if o.Revision < 4 || o.Signed == nil || o.Dispatch == nil || o.Observation != nil && (o.Observation.State == ObservationMempool || o.Observation.State == ObservationConfirmed) {
			return bad()
		}
	case StageSubmitted:
		if o.Revision < 4 || o.Signed == nil || o.Observation == nil {
			return bad()
		}
	case StageAbandoned:
		if o.Revision < 2 || o.Signed != nil {
			return bad()
		}
	default:
		return bad()
	}
	raw, e := json.Marshal(o)
	if e != nil || len(raw) > MaxOperationBytes {
		return bad()
	}
	return nil
}
func validateState(ctx context.Context, s state, i Identity, binding string) error {
	if s.Application != application || s.Version != 1 || s.JournalID != i.JournalID || s.BindingID != binding || s.Records == nil || len(s.Records) > MaxOperations || s.ClockHighWaterMS < 0 || s.ClockHighWaterMS > maxTime {
		return ErrCorrupt
	}
	ids := make(map[string]bool)
	signed := make(map[string]bool)
	unresolved := 0
	var high, created int64
	for n, o := range s.Records {
		if e := ctx.Err(); e != nil {
			return e
		}
		if o.Sequence != uint32(n+1) || ids[o.OperationID] || o.Intent.CreatedAtMS < created || validateOperation(o, binding, true) != nil {
			return ErrCorrupt
		}
		ids[o.OperationID] = true
		created = o.Intent.CreatedAtMS
		if o.UpdatedAtMS > high {
			high = o.UpdatedAtMS
		}
		if !done(o) {
			unresolved++
			if n != len(s.Records)-1 {
				return ErrCorrupt
			}
		}
		if o.Draft != nil {
			for _, in := range o.Draft.Inputs {
				k := in.TxID + ":" + strconv.FormatUint(uint64(in.OutputIndex), 10)
				if signed[k] {
					return ErrCorrupt
				}
				if o.Signed != nil {
					signed[k] = true
				}
			}
		}
	}
	if unresolved > 1 || s.ClockHighWaterMS != high {
		return ErrCorrupt
	}
	raw, e := json.Marshal(s)
	if e != nil || len(raw) > MaxStateBytes {
		return ErrCorrupt
	}
	return nil
}
