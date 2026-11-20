// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolsettle"
	"strings"
	"time"
)

const payoutMaxMS int64 = 253402300799999

func payoutTime(t int64, zero bool) bool { return t >= 0 && t <= payoutMaxMS && (zero || t > 0) }
func payoutNonzeroID(s string) bool      { return canonicalManifestID(s) && s != strings.Repeat("0", 64) }
func payoutScript(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 22 && s == hex.EncodeToString(b) && b[0] == 0 && b[1] == 20
}
func PoolPayoutErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "unauthorized", "stale_view", "busy", "capacity", "payout_unavailable", "payout_not_found", "operation_conflict", "outcome_unknown")
}
func payoutBindingValid(b PoolPayoutBinding) bool {
	return stableViewValid(b.View) && payoutNonzeroID(b.SourceBindingID) && payoutScript(b.RecipientScript) && historySize(b, 4096)
}
func payoutCaptureValid(c PoolPayoutCapture) bool {
	if c.Count > 0 {
		confirmed := int64(c.Count)
		if c.ReservedAtoms > 0 {
			confirmed--
			if c.ReservedAtoms < poolsettle.MinimumAtoms {
				return false
			}
		}
		if c.PaidAtoms < confirmed*poolsettle.MinimumAtoms || (confirmed == 0 && c.PaidAtoms != 0) {
			return false
		}
	}
	return c.Version == 1 && c.Count <= 366 && c.PaidAtoms >= 0 && c.ReservedAtoms >= 0 && c.PaidAtoms <= poolsettle.MaxMoney && c.ReservedAtoms <= poolsettle.MaxMoney-c.PaidAtoms && payoutTime(c.SettlementObservedAtMS, true) && payoutTime(c.CapturedAtMS, false) && (c.Count != 0 || c.PaidAtoms == 0 && c.ReservedAtoms == 0) && (c.SettlementObservedAtMS != 0 || c.Count == 0 && c.PaidAtoms == 0 && c.ReservedAtoms == 0) && historySize(c, 512)
}
func payoutItemValid(p poolsettle.PayoutItem, observed int64) bool {
	d, e := time.Parse("2006-01-02", p.Day)
	return e == nil && len(p.Day) == 10 && d.Format("2006-01-02") == p.Day && payoutTime(p.CreatedAtMS, false) && p.CreatedAtMS <= observed && time.UnixMilli(p.CreatedAtMS).UTC().Format("2006-01-02") == p.Day && canonicalManifestID(p.TxID) && p.OutputIndex <= 255 && p.Atoms >= 100000000 && p.Atoms <= poolsettle.MaxMoney && stableOne(p.RecordedState, "prepared", "broadcast", "confirmed") && historySize(p, 256)
}
func payoutRequestValid(r PoolPayoutRequest) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger || r.Version != 1 {
		return false
	}
	if r.Command == "pool_payout_snapshot" {
		return r.ViewID == "" && r.JobID == "" && r.After == 0 && r.Limit == 0 && r.TxID == "" && r.OutputIndex == 0
	}
	if !canonicalManifestID(r.ViewID) || !payoutNonzeroID(r.JobID) {
		return false
	}
	switch r.Command {
	case "pool_payout_begin", "pool_payout_job", "pool_payout_cancel":
		return r.After == 0 && r.Limit == 0 && r.TxID == "" && r.OutputIndex == 0
	case "pool_payout_page":
		return r.After <= 366 && r.Limit >= 1 && r.Limit <= 8 && r.TxID == "" && r.OutputIndex == 0
	case "pool_payout_wallet":
		return r.After == 0 && r.Limit == 0 && canonicalManifestID(r.TxID) && r.OutputIndex <= 255
	}
	return false
}
func ValidatePoolPayoutSnapshot(s PoolPayoutSnapshot) error {
	bad := gymstable.ErrInvalid
	if s.Version != 1 || !stableOptionalID(s.ActiveJobID) || !stableOptionalID(s.LastJobID) || !stableOptionalID(s.CaptureJobID) || !historySize(s, 5120) {
		return bad
	}
	for _, id := range []string{s.ActiveJobID, s.LastJobID, s.CaptureJobID} {
		if id != "" && !payoutNonzeroID(id) {
			return bad
		}
	}
	if s.State == "unavailable" {
		if s.Error == "payout_unavailable" && s.Binding == nil && s.Activity == "none" && s.ActiveJobID == "" && s.LastJobID == "" && s.CaptureJobID == "" {
			return nil
		}
		return bad
	}
	if s.Binding == nil || !payoutBindingValid(*s.Binding) || s.Error != "" {
		return bad
	}
	if s.State == "idle" && s.Activity == "none" && s.ActiveJobID == "" {
		return nil
	}
	if s.State == "busy" && s.ActiveJobID != "" {
		if s.Activity == "wallet" && s.ActiveJobID == s.CaptureJobID {
			return nil
		}
		if s.Activity == "capture" {
			return nil
		}
	}
	return bad
}
func ValidatePoolPayoutJob(j PoolPayoutJob) error {
	if j.Version != 1 || !payoutNonzeroID(j.JobID) || !payoutBindingValid(j.Binding) || !historySize(j, 6144) {
		return gymstable.ErrInvalid
	}
	switch j.State {
	case "running", "cancelled":
		if j.Error == "" && j.Capture == nil {
			return nil
		}
	case "failed":
		if j.Capture == nil && stableOne(j.Error, "busy", "capacity", "payout_unavailable") {
			return nil
		}
	case "done":
		if j.Error == "" && j.Capture != nil && payoutCaptureValid(*j.Capture) {
			return nil
		}
	}
	return gymstable.ErrInvalid
}
func ValidatePoolPayoutPage(p PoolPayoutPage) error {
	if p.Version != 1 || !payoutBindingValid(p.Binding) || !payoutNonzeroID(p.JobID) || !payoutCaptureValid(p.Capture) || p.After > p.Capture.Count || p.Limit < 1 || p.Limit > 8 || p.Items == nil || !historySize(p, 10240) {
		return gymstable.ErrInvalid
	}
	n := min(p.Limit, p.Capture.Count-p.After)
	if uint32(len(p.Items)) != n || p.Next != p.After+n || p.HasMore != (p.Next < p.Capture.Count) {
		return gymstable.ErrInvalid
	}
	var paid, reserved int64
	seen := map[string]bool{}
	for i, row := range p.Items {
		if !payoutItemValid(row, p.Capture.SettlementObservedAtMS) || seen[row.TxID] || row.RecordedState != "confirmed" && p.After+uint32(i) != 0 {
			return gymstable.ErrInvalid
		}
		seen[row.TxID] = true
		if i > 0 && (row.Day >= p.Items[i-1].Day || row.CreatedAtMS >= p.Items[i-1].CreatedAtMS) {
			return gymstable.ErrInvalid
		}
		if row.RecordedState == "confirmed" {
			if row.Atoms > p.Capture.PaidAtoms-paid {
				return gymstable.ErrInvalid
			}
			paid += row.Atoms
		} else {
			if row.Atoms > p.Capture.ReservedAtoms-reserved {
				return gymstable.ErrInvalid
			}
			reserved += row.Atoms
		}
	}
	if p.After == 0 && reserved != p.Capture.ReservedAtoms {
		return gymstable.ErrInvalid
	}
	if p.After == 0 && p.Next == p.Capture.Count && (paid != p.Capture.PaidAtoms || reserved != p.Capture.ReservedAtoms) {
		return gymstable.ErrInvalid
	}
	return nil
}
func ValidatePoolPayoutObservation(o PoolPayoutObservation) error {
	if o.Version != 1 || !payoutBindingValid(o.Binding) || !payoutNonzeroID(o.JobID) || !payoutCaptureValid(o.Capture) || o.Capture.Count == 0 || !payoutItemValid(o.Payment, o.Capture.SettlementObservedAtMS) || !payoutTime(o.ObservedAtMS, false) || o.WalletHeight > 2147483647 || !historySize(o, 7168) {
		return gymstable.ErrInvalid
	}
	if o.Payment.RecordedState == "confirmed" {
		if o.Payment.Atoms > o.Capture.PaidAtoms {
			return gymstable.ErrInvalid
		}
	} else if o.Payment.Atoms != o.Capture.ReservedAtoms {
		return gymstable.ErrInvalid
	}
	if o.State == "unavailable" {
		if o.Error == "wallet_unavailable" && !o.WalletSynced && o.WalletHeight == 0 && o.WalletTip == "" && o.Confirmations == 0 && o.BlockHash == "" {
			return nil
		}
		return gymstable.ErrInvalid
	}
	if o.Error != "" || !o.WalletSynced || !payoutNonzeroID(o.WalletTip) {
		return gymstable.ErrInvalid
	}
	switch o.State {
	case "not_observed", "unconfirmed":
		if o.Confirmations == 0 && o.BlockHash == "" {
			return nil
		}
	case "confirmed":
		if o.Confirmations > 0 && o.Confirmations <= o.WalletHeight+1 && payoutNonzeroID(o.BlockHash) {
			return nil
		}
	}
	return gymstable.ErrInvalid
}
func payoutResponseValid(r PoolPayoutResponse) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger || r.Version != 1 {
		return false
	}
	n := 0
	valid := true
	if r.Snapshot != nil {
		n++
		valid = valid && ValidatePoolPayoutSnapshot(*r.Snapshot) == nil
	}
	if r.Job != nil {
		n++
		valid = valid && ValidatePoolPayoutJob(*r.Job) == nil
	}
	if r.Page != nil {
		n++
		valid = valid && ValidatePoolPayoutPage(*r.Page) == nil
	}
	if r.Observation != nil {
		n++
		valid = valid && ValidatePoolPayoutObservation(*r.Observation) == nil
	}
	if !r.OK {
		return n == 0 && PoolPayoutErrorCode(r.Error)
	}
	return n == 1 && valid && r.Error == ""
}
func payoutJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'}), nil
}
func ParsePoolPayoutRequest(raw []byte) (PoolPayoutRequest, error) {
	var r PoolPayoutRequest
	if _, e := poolPayoutDecode(raw, MaxPoolPayoutRequestSize-1, 3, &r); e != nil || !payoutRequestValid(r) {
		return PoolPayoutRequest{}, gymstable.ErrInvalid
	}
	return r, nil
}
func MarshalPoolPayoutRequest(r PoolPayoutRequest) ([]byte, error) {
	if !payoutRequestValid(r) {
		return nil, gymstable.ErrInvalid
	}
	b, e := payoutJSON(r)
	if e != nil || len(b) >= MaxPoolPayoutRequestSize {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}
func ParsePoolPayoutResponse(raw []byte) (PoolPayoutResponse, error) {
	var r PoolPayoutResponse
	if _, e := poolPayoutDecode(raw, MaxPoolPayoutResponseSize-1, 8, &r); e != nil || !payoutResponseValid(r) {
		return PoolPayoutResponse{}, gymstable.ErrInvalid
	}
	return r, nil
}
func MarshalPoolPayoutResponse(r PoolPayoutResponse) ([]byte, error) {
	if !payoutResponseValid(r) {
		return nil, gymstable.ErrInvalid
	}
	b, e := payoutJSON(r)
	if e != nil || len(b) >= MaxPoolPayoutResponseSize {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}

// PoolPayoutBindingForSnapshot uses the independently pinned public native84
// fixture recipient, not a destination supplied by the payout-history reply.
func PoolPayoutBindingForSnapshot(s PoolSnapshot) (PoolPayoutBinding, error) {
	if ValidatePoolSnapshot(s) != nil || s.View == nil || s.Account.State != "signed_in" {
		return PoolPayoutBinding{}, gymstable.ErrInvalid
	}
	f, e := gymaccount.FixtureIdentity(s.Account.Fixture)
	if e != nil {
		return PoolPayoutBinding{}, e
	}
	p, e := expectedPoolRecipient(walletIdentity{RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID})
	if e != nil || p.Payout.Address != s.PayoutAddress {
		return PoolPayoutBinding{}, gymstable.ErrInvalid
	}
	b := PoolPayoutBinding{View: *s.View, SourceBindingID: s.SourceBindingID, RecipientScript: p.Script}
	if !payoutBindingValid(b) {
		return PoolPayoutBinding{}, gymstable.ErrInvalid
	}
	return b, nil
}
