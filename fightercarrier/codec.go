// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fightertransfer"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func hexValue(s string, n int, id bool) bool { return journalHex(s, n, id) }
func hashBytes(b []byte) string              { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func bounded(v any, cap int) error {
	b, e := json.Marshal(v)
	if e != nil {
		return ErrInvalid
	}
	if len(b) > cap {
		return ErrCapacity
	}
	return nil
}
func valueDigest(kind string, v any, cap int) (string, error) {
	if e := bounded(v, cap); e != nil {
		return "", e
	}
	b, e := json.Marshal(v)
	if e != nil {
		return "", ErrInvalid
	}
	h := sha256.New()
	h.Write([]byte("SLUG " + kind + "\x00v1\x00"))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), nil
}
func point(s string) bool {
	if !hexValue(s, 33, false) {
		return false
	}
	b, _ := hex.DecodeString(s)
	p, e := btcec.ParsePubKey(b)
	return e == nil && bytes.Equal(p.SerializeCompressed(), b)
}
func native(s string) bool {
	return len(s) == 44 && strings.HasPrefix(s, "0014") && hexValue(s, 22, false)
}
func validPath(p ReturnPath) bool {
	if p.Branch > 1 || p.Index >= 4096 || !native(p.Script) || len(p.Address) > 90 {
		return false
	}
	a, e := address.DecodeAddress(p.Address, &chaincfg.MainNetParams)
	if e != nil || a.EncodeAddress() != p.Address || !a.IsForNet(&chaincfg.MainNetParams) {
		return false
	}
	s, e := txscript.PayToAddrScript(a)
	return e == nil && hex.EncodeToString(s) == p.Script
}
func sameValue(a, b any) bool {
	x, e := json.Marshal(a)
	y, f := json.Marshal(b)
	return e == nil && f == nil && bytes.Equal(x, y)
}

func ValidatePin(p Pin) error {
	if p.Version != 1 || !hexValue(p.WalletOwnerID, 32, true) || !hexValue(p.OwnershipPinID, 32, true) {
		return ErrInvalid
	}
	found := false
	for _, name := range []string{"zero256", "incrementing256"} {
		f, e := gymaccount.FixtureIdentity(name)
		if e == nil && p.Network == f.Network && p.Genesis == f.Genesis && p.WalletProfile == f.Profile && p.RootPublicKey == f.RootPublicKey && p.GymPublicKey == f.GymPublicKey && p.GymID == f.GymID {
			found = true
		}
	}
	if !found {
		return ErrInvalid
	}
	switch p.OwnerMode {
	case "hosted":
		if p.ManagerSchema != 14 || !hexValue(p.AccountAuthorityID, 32, true) || !hexValue(p.AccountID, 32, true) || !hexValue(p.AllocationBindingID, 32, true) || p.OriginalManifestSHA256 != "" {
			return ErrInvalid
		}
	case "restored":
		if p.ManagerSchema != 15 || p.AccountAuthorityID != "" || p.AccountID != "" || p.AllocationBindingID != "" || !hexValue(p.OriginalManifestSHA256, 32, false) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
func BindingID(i Identity) (string, error) {
	if ValidatePin(i.Pin) != nil || !hexValue(i.JournalID, 32, true) {
		return "", ErrInvalid
	}
	return valueDigest("fighter carrier binding", struct {
		JournalID string `json:"journal_id"`
		Pin       Pin    `json:"pin"`
	}{i.JournalID, i.Pin}, 4096)
}
func ValidateRequest(r Request) error {
	if r.Version != 1 || !hexValue(r.RequestID, 32, true) || !hexValue(r.OwnershipPinID, 32, true) || !hexValue(r.PriorStateID, 32, true) || !point(r.RecipientGym) {
		return ErrInvalid
	}
	switch r.Kind {
	case "transfer":
		if r.CommitTxID != "" {
			return ErrInvalid
		}
	case "recover_reveal", "recover_refund":
		if !hexValue(r.CommitTxID, 32, false) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return bounded(r, MaxRequestBytes)
}
func RequestDigest(r Request) (string, error) {
	if e := ValidateRequest(r); e != nil {
		return "", e
	}
	return valueDigest("fighter carrier request", r, MaxRequestBytes)
}

// ValidateReview checks the closed economic projection. It authenticates no
// wallet path, original import, predecessor, account, or native approval.
func ValidateReview(r Review) error {
	if r.Version != 1 || ValidateRequest(Request{1, r.RequestID, r.Kind, r.OwnershipPinID, r.PriorStateID, r.RecipientGym, r.CommitTxID}) != nil || !hexValue(r.BindingID, 32, false) || !point(r.SenderGym) || r.SenderGym == r.RecipientGym || !hexValue(r.TransferID, 32, false) || !hexValue(r.FundingDigest, 32, false) || !validPath(ReturnPath{r.ReturnAddress, r.ReturnScript, r.ReturnBranch, r.ReturnIndex}) || r.EscrowAtoms != 500000 || r.SpendFeeAtoms != 10000 || r.ReturnedAtoms != 490000 {
		return ErrInvalid
	}
	if r.Kind == "transfer" {
		if !hexValue(r.InputDetailsDigest, 32, false) || r.InputCount < 1 || r.InputCount > 16 || r.TotalInputAtoms < 511000 || r.TotalInputAtoms > MaxMoney || r.ReturnBranch != 1 || r.CommitChangeAtoms != r.TotalInputAtoms-510000 || r.CommitFeeAtoms != 10000 || r.TotalFeeAtoms != 20000 {
			return ErrInvalid
		}
	} else if r.InputDetailsDigest != "" || r.InputCount != 1 || r.TotalInputAtoms != 500000 || r.CommitChangeAtoms != 0 || r.CommitFeeAtoms != 0 || r.TotalFeeAtoms != 10000 {
		return ErrInvalid
	}
	intent, e := reviewIntent(r)
	if e != nil {
		return e
	}
	id, e := fightertransfer.TransferID(intent)
	if e != nil || hex.EncodeToString(id[:]) != r.TransferID {
		return ErrInvalid
	}
	return bounded(r, MaxReviewBytes)
}
func ReviewDigest(r Review) (string, error) {
	if e := ValidateReview(r); e != nil {
		return "", e
	}
	return valueDigest("fighter carrier review", r, MaxReviewBytes)
}
func reviewIntent(r Review) (fightertransfer.Intent, error) {
	var i fightertransfer.Intent
	for _, v := range []struct {
		s string
		b []byte
	}{{r.OwnershipPinID, i.PinID[:]}, {r.PriorStateID, i.PriorStateID[:]}, {r.RecipientGym, i.RecipientGym[:]}, {r.FundingDigest, i.FundingDigest[:]}} {
		b, e := hex.DecodeString(v.s)
		if e != nil || len(b) != len(v.b) {
			return i, ErrInvalid
		}
		copy(v.b, b)
	}
	return i, nil
}

func inputDetails(inputs []OwnedInput, tip int32) (string, int64, []wire.OutPoint, error) {
	if len(inputs) < 1 || len(inputs) > 16 {
		return "", 0, nil, ErrInvalid
	}
	ops := make([]wire.OutPoint, len(inputs))
	var total int64
	for n, v := range inputs {
		if !hexValue(v.TxID, 32, false) || !hexValue(v.BlockHash, 32, false) || !native(v.Script) || v.Atoms < 1 || v.Atoms > MaxMoney-total || v.Branch > 1 || v.Index >= 4096 || v.BlockHeight < 0 || v.BlockHeight > tip || v.Coinbase && int64(tip)-int64(v.BlockHeight)+1 < 100 {
			return "", 0, nil, ErrInvalid
		}
		b, _ := hex.DecodeString(v.TxID)
		for i := range b {
			ops[n].Hash[i] = b[len(b)-1-i]
		}
		ops[n].Index = v.OutputIndex
		total += v.Atoms
	}
	if _, e := fightertransfer.FundingDigest(ops); e != nil {
		return "", 0, nil, ErrInvalid
	}
	d, e := valueDigest("fighter carrier owned inputs", inputs, 8192)
	return d, total, ops, e
}
func restoredRange(a *RestoredAllocation, p ReturnPath) bool {
	if a == nil || a.InvalidIndices == nil || len(a.InvalidIndices) > 127 || len(a.CursorsBefore) != 8 || len(a.CursorsAfter) != 8 || a.StartIndex >= a.EndIndex || a.EndIndex > 4096 || a.EndIndex-a.StartIndex > 128 || a.SelectedIndex != a.EndIndex-1 || a.SelectedIndex != p.Index || uint32(len(a.InvalidIndices)) != a.SelectedIndex-a.StartIndex {
		return false
	}
	for i, v := range a.InvalidIndices {
		if v != a.StartIndex+uint32(i) {
			return false
		}
	}
	for i, v := range a.CursorsBefore {
		if v > 4096 || a.CursorsAfter[i] > 4096 {
			return false
		}
		want := v
		if i == 5 {
			if v != a.StartIndex {
				return false
			}
			want = a.EndIndex
		}
		if a.CursorsAfter[i] != want {
			return false
		}
	}
	return true
}

// ValidatePreparation checks stored shape/economics and supplied public
// relationships. Concrete wallet validation additionally proves each path and
// invalid child, original-import coverage, UTXOs and live ownership authority.
func ValidatePreparation(p Preparation, pin Pin, binding string) error {
	if ValidatePin(pin) != nil || p.Version != 1 || p.BindingID != binding || !hexValue(binding, 32, false) || ValidateRequest(p.Request) != nil || p.Request.OwnershipPinID != pin.OwnershipPinID || p.Request.RecipientGym == pin.GymPublicKey || !journalTime(p.PreparedAtMS) || !hexValue(p.TipHash, 32, true) || p.TipHeight < 0 || !hexValue(p.OwnershipHeadDigest, 32, false) {
		return ErrInvalid
	}
	r := p.Review
	if r.BindingID != binding || r.RequestID != p.Request.RequestID || r.Kind != p.Request.Kind || r.OwnershipPinID != p.Request.OwnershipPinID || r.PriorStateID != p.Request.PriorStateID || r.RecipientGym != p.Request.RecipientGym || r.SenderGym != pin.GymPublicKey || r.CommitTxID != p.Request.CommitTxID {
		return ErrInvalid
	}
	d, e := ReviewDigest(r)
	if e != nil || d != p.ReviewDigest {
		return ErrInvalid
	}
	var path ReturnPath
	if p.Request.Kind == "transfer" {
		n := p.NewFunding
		if n == nil || p.RecoveryFunding != nil || n.AllocationID != p.Request.RequestID || !validPath(n.ReturnPath) || n.ReturnPath.Branch != 1 {
			return ErrInvalid
		}
		path = n.ReturnPath
		d, total, ops, e := inputDetails(n.Inputs, p.TipHeight)
		if e != nil || d != r.InputDetailsDigest || uint32(len(n.Inputs)) != r.InputCount || total != n.TotalInputAtoms || total != r.TotalInputAtoms || total < 511000 || n.CommitChangeAtoms != total-510000 || n.CommitChangeAtoms != r.CommitChangeAtoms {
			return ErrInvalid
		}
		f, e := fightertransfer.FundingDigest(ops)
		if e != nil || hex.EncodeToString(f[:]) != n.FundingDigest || n.FundingDigest != r.FundingDigest {
			return ErrInvalid
		}
		if pin.OwnerMode == "hosted" {
			if !hexValue(n.ReservationDigest, 32, false) || p.RestoredAllocation != nil {
				return ErrInvalid
			}
		} else if n.ReservationDigest != "" || !restoredRange(p.RestoredAllocation, path) {
			return ErrInvalid
		}
	} else {
		n := p.RecoveryFunding
		if pin.OwnerMode != "restored" || n == nil || p.NewFunding != nil || p.RestoredAllocation != nil || n.CommitTxID != p.Request.CommitTxID || !hexValue(n.CommitBlockHash, 32, false) || n.CommitHeight < 0 || n.CommitHeight > p.TipHeight || !point(n.FundingPublicKey) || n.FundingBranch > 1 || n.FundingIndex >= 4096 || !validPath(n.ReturnPath) || !hexValue(n.GymAuthorization, 64, false) || n.FundingDigest != r.FundingDigest || n.TransferID != r.TransferID {
			return ErrInvalid
		}
		path = n.ReturnPath
		tx, e := parseWriterRaw(n.CommitRaw, 4096, true)
		if e != nil || tx.TxHash().String() != n.CommitTxID || hex.EncodeToString(tx.TxOut[2].PkScript) != path.Script {
			return ErrInvalid
		}
		if len(tx.TxIn[0].Witness) != 2 || hex.EncodeToString(tx.TxIn[0].Witness[1]) != n.FundingPublicKey {
			return ErrInvalid
		}
		if e = checkCommit(p, tx, n.GymAuthorization); e != nil {
			return e
		}
	}
	if path != (ReturnPath{r.ReturnAddress, r.ReturnScript, r.ReturnBranch, r.ReturnIndex}) {
		return ErrInvalid
	}
	return bounded(p, MaxPreparationBytes)
}

// Digest helpers deliberately validate context-free shape; only a concrete
// original owner can supply the independently expected pin and wallet state.
func PreparationDigest(p Preparation) (string, error) {
	if p.Version != 1 || ValidateRequest(p.Request) != nil || ValidateReview(p.Review) != nil || !journalTime(p.PreparedAtMS) || !hexValue(p.BindingID, 32, false) || !hexValue(p.ReviewDigest, 32, false) {
		return "", ErrInvalid
	}
	d, e := ReviewDigest(p.Review)
	if e != nil || d != p.ReviewDigest {
		return "", ErrInvalid
	}
	return valueDigest("fighter carrier preparation", p, MaxPreparationBytes)
}
func SignedPackageDigest(p SignedPackage) (string, error) {
	if p.Version != 1 || !hexValue(p.ReviewDigest, 32, false) || !hexValue(p.TransferID, 32, false) || !hexValue(p.GymAuthorization, 64, false) || p.SpendKind != "reveal" && p.SpendKind != "refund" {
		return "", ErrInvalid
	}
	return valueDigest("fighter carrier signed package", p, MaxSignedPackageBytes)
}
