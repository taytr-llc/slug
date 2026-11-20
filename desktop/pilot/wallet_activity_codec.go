// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/json"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolsettle"
)

func WalletActivityErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "unauthorized", "stale_view", "busy", "capacity", "activity_unavailable", "activity_not_found", "operation_conflict", "outcome_unknown")
}
func ValidateWalletActivityBinding(b WalletActivityBinding) error {
	if !stableViewValid(b.View) || !payoutNonzeroID(b.SourceBindingID) || gymaccount.ValidateFixtureIdentity(b.RootPublicKey, b.GymPublicKey, b.GymID) != nil {
		return gymstable.ErrInvalid
	}
	return nil
}
func WalletActivityBindingForSnapshot(s PoolSnapshot) (WalletActivityBinding, error) {
	if ValidatePoolSnapshot(s) != nil || s.View == nil || s.Account.State != "signed_in" {
		return WalletActivityBinding{}, gymstable.ErrInvalid
	}
	f, e := gymaccount.FixtureIdentity(s.Account.Fixture)
	if e != nil || f.GymID != s.GymID || f.GymPublicKey != s.GymPublicKey {
		return WalletActivityBinding{}, gymstable.ErrInvalid
	}
	b := WalletActivityBinding{View: *s.View, SourceBindingID: s.SourceBindingID, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID}
	return b, ValidateWalletActivityBinding(b)
}
func ValidateWalletActivityCapture(c WalletActivityCapture) error {
	if c.Version != 1 || !payoutTime(c.CapturedAtMS, false) || c.WalletHeight > 2147483647 || c.NodeHeight != c.WalletHeight || !payoutNonzeroID(c.WalletTip) || c.NodeTip != c.WalletTip || c.Count > 256 || c.HasOlder && c.Count != 256 {
		return gymstable.ErrInvalid
	}
	return nil
}
func activityAtoms(n int64) bool { return n >= 0 && n <= poolsettle.MaxMoney }
func ValidateWalletActivityItem(v WalletActivityItem, c WalletActivityCapture) error {
	if ValidateWalletActivityCapture(c) != nil || !canonicalManifestID(v.TxID) || !canonicalManifestID(v.RawSHA256) || !payoutTime(v.RecordedAtMS, true) || v.InputCount < 1 || v.InputCount > 256 || v.OutputCount < 1 || v.OutputCount > 256 || v.OwnedInputCount > v.InputCount || v.OwnedOutputCount > v.OutputCount || v.OwnedInputCount+v.OwnedOutputCount == 0 || !activityAtoms(v.ReceivedAtoms) || !activityAtoms(v.SpentAtoms) || !activityAtoms(v.ChangeAtoms) || v.ChangeAtoms > v.ReceivedAtoms || v.NetAtoms != v.ReceivedAtoms-v.SpentAtoms || v.OwnedInputCount == 0 && v.SpentAtoms != 0 || v.OwnedOutputCount == 0 && (v.ReceivedAtoms != 0 || v.ChangeAtoms != 0) {
		return gymstable.ErrInvalid
	}
	if v.Block == nil {
		if v.Confirmations != 0 {
			return gymstable.ErrInvalid
		}
	} else if v.Block.Height > c.WalletHeight || !payoutNonzeroID(v.Block.Hash) || !payoutTime(v.Block.TimeMS, true) || v.Confirmations != c.WalletHeight-v.Block.Height+1 {
		return gymstable.ErrInvalid
	}
	unknown := v.Coinbase || v.OwnedInputCount < v.InputCount
	if unknown != (v.FeeAtoms == nil) || v.FeeAtoms != nil && (*v.FeeAtoms < 0 || *v.FeeAtoms > v.SpentAtoms) {
		return gymstable.ErrInvalid
	}
	if v.Coinbase && (v.InputCount != 1 || v.OwnedInputCount != 0 || v.SpentAtoms != 0) {
		return gymstable.ErrInvalid
	}
	return nil
}

// WalletActivityItemLess is the immutable captured order, never a live cursor.
func WalletActivityItemLess(a, b WalletActivityItem) bool {
	if (a.Block == nil) != (b.Block == nil) {
		return a.Block == nil
	}
	if a.Block == nil {
		if a.RecordedAtMS != b.RecordedAtMS {
			return a.RecordedAtMS > b.RecordedAtMS
		}
	} else if a.Block.Height != b.Block.Height {
		return a.Block.Height > b.Block.Height
	}
	return a.TxID < b.TxID
}
func CloneWalletActivityItem(v WalletActivityItem) WalletActivityItem {
	if v.Block != nil {
		x := *v.Block
		v.Block = &x
	}
	if v.FeeAtoms != nil {
		x := *v.FeeAtoms
		v.FeeAtoms = &x
	}
	return v
}
func WalletActivityItemEqual(a, b WalletActivityItem) bool {
	if (a.Block == nil) != (b.Block == nil) || (a.FeeAtoms == nil) != (b.FeeAtoms == nil) {
		return false
	}
	if a.Block != nil && *a.Block != *b.Block {
		return false
	}
	if a.FeeAtoms != nil && *a.FeeAtoms != *b.FeeAtoms {
		return false
	}
	a.Block, b.Block, a.FeeAtoms, b.FeeAtoms = nil, nil, nil, nil
	return a == b
}
func ValidateWalletActivitySnapshot(s WalletActivitySnapshot) error {
	if s.Version != 1 {
		return gymstable.ErrInvalid
	}
	for _, id := range []string{s.ActiveJobID, s.LastJobID, s.CaptureJobID} {
		if id != "" && !payoutNonzeroID(id) {
			return gymstable.ErrInvalid
		}
	}
	if s.State == "unavailable" {
		if s.Error == "activity_unavailable" && s.Binding == nil && s.Activity == "none" && s.ActiveJobID == "" && s.LastJobID == "" && s.CaptureJobID == "" {
			return nil
		}
		return gymstable.ErrInvalid
	}
	if s.Binding == nil || ValidateWalletActivityBinding(*s.Binding) != nil || s.Error != "" {
		return gymstable.ErrInvalid
	}
	if s.State == "idle" && s.Activity == "none" && s.ActiveJobID == "" {
		return nil
	}
	if s.State == "busy" && s.ActiveJobID != "" && (s.Activity == "capture" || s.Activity == "detail" && s.ActiveJobID == s.CaptureJobID) {
		return nil
	}
	return gymstable.ErrInvalid
}
func ValidateWalletActivityJob(j WalletActivityJob) error {
	if j.Version != 1 || !payoutNonzeroID(j.JobID) || ValidateWalletActivityBinding(j.Binding) != nil {
		return gymstable.ErrInvalid
	}
	switch j.State {
	case "running", "cancelled":
		if j.Error == "" && j.Capture == nil {
			return nil
		}
	case "failed":
		if j.Capture == nil && stableOne(j.Error, "busy", "capacity", "activity_unavailable") {
			return nil
		}
	case "done":
		if j.Error == "" && j.Capture != nil {
			return ValidateWalletActivityCapture(*j.Capture)
		}
	}
	return gymstable.ErrInvalid
}
func ValidateWalletActivityPage(p WalletActivityPage) error {
	if p.Version != 1 || ValidateWalletActivityBinding(p.Binding) != nil || !payoutNonzeroID(p.JobID) || ValidateWalletActivityCapture(p.Capture) != nil || p.After > p.Capture.Count || p.Limit < 1 || p.Limit > 8 || p.Items == nil {
		return gymstable.ErrInvalid
	}
	n := min(p.Limit, p.Capture.Count-p.After)
	if uint32(len(p.Items)) != n || p.Next != p.After+n || p.HasMore != (p.Next < p.Capture.Count) {
		return gymstable.ErrInvalid
	}
	seen := make(map[string]bool, len(p.Items))
	for i, v := range p.Items {
		if ValidateWalletActivityItem(v, p.Capture) != nil || seen[v.TxID] || i > 0 && !WalletActivityItemLess(p.Items[i-1], v) {
			return gymstable.ErrInvalid
		}
		seen[v.TxID] = true
	}
	return nil
}
func activityAddress(s string) bool {
	if s == "" {
		return true
	}
	if len(s) > 90 {
		return false
	}
	for _, v := range []byte(s) {
		if v < 33 || v > 126 {
			return false
		}
	}
	a, e := address.DecodeAddress(s, &chaincfg.MainNetParams)
	if e != nil || !a.IsForNet(&chaincfg.MainNetParams) || a.EncodeAddress() != s {
		return false
	}
	switch a.(type) {
	case *address.AddressPubKeyHash, *address.AddressScriptHash, *address.AddressWitnessPubKeyHash, *address.AddressWitnessScriptHash, *address.AddressTaproot:
		return true
	}
	return false
}
func ValidateWalletActivityDetail(d WalletActivityDetail) error {
	if d.Version != 1 || ValidateWalletActivityBinding(d.Binding) != nil || !payoutNonzeroID(d.JobID) || ValidateWalletActivityCapture(d.Capture) != nil || d.Capture.Count == 0 || ValidateWalletActivityItem(d.Item, d.Capture) != nil || d.Outputs == nil || uint32(len(d.Outputs)) != d.Item.OutputCount {
		return gymstable.ErrInvalid
	}
	var total, credit, change int64
	var owned uint32
	for i, v := range d.Outputs {
		if v.Index != uint32(i) || !activityAtoms(v.Atoms) || v.Atoms > poolsettle.MaxMoney-total || v.Change && !v.Owned || !activityAddress(v.Address) {
			return gymstable.ErrInvalid
		}
		total += v.Atoms
		if v.Owned {
			owned++
			credit += v.Atoms
		}
		if v.Change {
			change += v.Atoms
		}
	}
	if owned != d.Item.OwnedOutputCount || credit != d.Item.ReceivedAtoms || change != d.Item.ChangeAtoms || d.Item.FeeAtoms != nil && total+*d.Item.FeeAtoms != d.Item.SpentAtoms {
		return gymstable.ErrInvalid
	}
	return nil
}
func activityRequestValid(r WalletActivityRequest) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger || r.Version != 1 {
		return false
	}
	if r.Command == "wallet_activity_snapshot" {
		return r.ViewID == "" && r.JobID == "" && r.After == 0 && r.Limit == 0 && r.TxID == ""
	}
	if !canonicalManifestID(r.ViewID) || !payoutNonzeroID(r.JobID) {
		return false
	}
	switch r.Command {
	case "wallet_activity_begin", "wallet_activity_job", "wallet_activity_cancel":
		return r.After == 0 && r.Limit == 0 && r.TxID == ""
	case "wallet_activity_page":
		return r.After <= 256 && r.Limit >= 1 && r.Limit <= 8 && r.TxID == ""
	case "wallet_activity_detail":
		return r.After == 0 && r.Limit == 0 && canonicalManifestID(r.TxID)
	}
	return false
}
func activityResponseValid(r WalletActivityResponse) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger || r.Version != 1 {
		return false
	}
	n := 0
	valid := true
	if r.Snapshot != nil {
		n++
		valid = valid && ValidateWalletActivitySnapshot(*r.Snapshot) == nil
	}
	if r.Job != nil {
		n++
		valid = valid && ValidateWalletActivityJob(*r.Job) == nil
	}
	if r.Page != nil {
		n++
		valid = valid && ValidateWalletActivityPage(*r.Page) == nil
	}
	if r.Detail != nil {
		n++
		valid = valid && ValidateWalletActivityDetail(*r.Detail) == nil
	}
	if !r.OK {
		return n == 0 && WalletActivityErrorCode(r.Error)
	}
	return n == 1 && valid && r.Error == ""
}
func activityFrameLimit(raw []byte, limit int) bool {
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		return len(raw) <= limit
	}
	return len(raw) < limit
}
func ParseWalletActivityRequest(raw []byte) (WalletActivityRequest, error) {
	var r WalletActivityRequest
	if !activityFrameLimit(raw, MaxWalletActivityRequestSize) || walletActivityDecode(raw, MaxWalletActivityRequestSize, 3, &r) != nil || !activityRequestValid(r) {
		return WalletActivityRequest{}, gymstable.ErrInvalid
	}
	return r, nil
}
func ParseWalletActivityResponse(raw []byte) (WalletActivityResponse, error) {
	var r WalletActivityResponse
	if !activityFrameLimit(raw, MaxWalletActivityResponseSize) || walletActivityDecode(raw, MaxWalletActivityResponseSize, 8, &r) != nil || !activityResponseValid(r) || r.Detail == nil && !activityFrameLimit(raw, MaxWalletActivitySmallResponseSize) {
		return WalletActivityResponse{}, gymstable.ErrInvalid
	}
	return r, nil
}

// Marshal returns the body without LF, as expected by the existing pipe
// exchange. Its size reservation includes the transport's final LF.
func MarshalWalletActivityRequest(r WalletActivityRequest) ([]byte, error) {
	if !activityRequestValid(r) {
		return nil, gymstable.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) >= MaxWalletActivityRequestSize {
		return nil, gymstable.ErrInvalid
	}
	return raw, nil
}
func MarshalWalletActivityResponse(r WalletActivityResponse) ([]byte, error) {
	if !activityResponseValid(r) {
		return nil, gymstable.ErrInvalid
	}
	limit := MaxWalletActivitySmallResponseSize
	if r.Detail != nil {
		limit = MaxWalletActivityResponseSize
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) >= limit {
		return nil, gymstable.ErrInvalid
	}
	return raw, nil
}

// Own the nested immutable item pointers when retaining a page across calls.
func CloneWalletActivityPage(p WalletActivityPage) WalletActivityPage {
	p.Items = append(p.Items[:0:0], p.Items...)
	if p.Items == nil {
		p.Items = []WalletActivityItem{}
	}
	for i := range p.Items {
		p.Items[i] = CloneWalletActivityItem(p.Items[i])
	}
	return p
}
