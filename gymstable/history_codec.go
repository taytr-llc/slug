// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"

	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// These validators establish closed public structure and local arithmetic, not
// authority or completeness. Only PrepareCareerHistory mints a trusted handle.
func historyStatusValid(s Status) bool {
	id, e := ID(s.Pin)
	return e == nil && s.Version == Version1 && s.StoreID == id && s.Checkpoint <= MaxAssignments &&
		s.ObservedSourceCount >= s.Checkpoint && s.ObservedSourceCount <= MaxAssignments && s.CaughtUp == (s.Checkpoint == s.ObservedSourceCount) &&
		s.OperationCount <= MaxOperations && s.RosterRevision <= uint64(s.OperationCount) && validSlots(s.Slots) &&
		s.LastAsOfMS >= 0 && s.LastAsOfMS <= MaxTimestampMS &&
		(s.PendingOperationID == "" || hashID(s.PendingOperationID)) && (s.LastOperationID == "" || hashID(s.LastOperationID)) &&
		(s.OperationCount == 0) == (s.LastOperationID == "") && (s.PendingOperationID == "" || s.PendingOperationID == s.LastOperationID) && careerPreviewBound(s, 4096)
}
func historyTotalsValid(t fightercareer.Totals, n uint32) bool {
	if n > 256 || t.Wins > 256 || t.Losses > 256 || t.Draws > 256 || t.Wins+t.Losses+t.Draws != n || t.XP != min(uint32(15), n) {
		return false
	}
	lo, hi := 10*t.Wins+4*t.Draws, 14*t.Wins+4*t.Draws
	return t.BasePoints >= lo && t.BasePoints <= hi && (t.BasePoints-lo)%4 == 0 && careerPreviewBound(t, 128)
}

// CareerHistoryID is a display/correlation commitment. It grants no access and
// cannot verify the omitted global events that contributed to CareerDigest.
func CareerHistoryID(c CareerHistoryCapture) string {
	h := sha256.New()
	h.Write([]byte("SLUG candidate fighter history\x00v1\x00"))
	for _, s := range []string{c.Status.StoreID, c.Activation.ActivationID, c.CareerDigest} {
		b, e := hex.DecodeString(s)
		if e != nil || len(b) != 32 {
			return ""
		}
		h.Write(b)
	}
	writeLP := func(v any) {
		b, _ := json.Marshal(v)
		var n [4]byte
		binary.LittleEndian.PutUint32(n[:], uint32(len(b)))
		h.Write(n[:])
		h.Write(b)
	}
	writeLP(c.Assignment)
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], uint64(c.AsOfMS))
	h.Write(n[:])
	for _, v := range []uint32{c.PrefixCount, c.EventCount} {
		binary.LittleEndian.PutUint32(n[:4], v)
		h.Write(n[:4])
	}
	writeLP(c.Totals)
	return hex.EncodeToString(h.Sum(nil))
}
func ValidateCareerHistoryCapture(c CareerHistoryCapture) (CareerHistoryCapture, error) {
	if c.Version != Version1 || !historyStatusValid(c.Status) || careerstate.ValidateActivation(c.Activation) != nil ||
		!careerPreviewBound(c.Activation, 512) || !validAssignment(c.Assignment, c.Status.Pin) || c.Assignment.Sequence > c.Status.Checkpoint || !careerPreviewBound(c.Assignment, 2048) ||
		!hashID(c.CareerDigest) || c.AsOfMS <= 0 || c.AsOfMS > MaxTimestampMS || c.AsOfMS != c.Status.LastAsOfMS || c.AsOfMS < c.Activation.ActivatedAtMS || c.AsOfMS < c.Assignment.AcceptedAtMS ||
		c.PrefixCount > 256 || c.EventCount > c.PrefixCount || !historyTotalsValid(c.Totals, c.EventCount) || c.HistoryID != CareerHistoryID(c) || !careerPreviewBound(c, MaxCareerHistoryCaptureSize) {
		return CareerHistoryCapture{}, ErrInvalid
	}
	return c, nil
}
func historyRowValid(r CareerHistoryRow, c CareerHistoryCapture) bool {
	if r.Ordinal < 1 || r.Ordinal > c.EventCount || !careerPreviewBound(r.Event, MaxCareerHistoryEventSize) {
		return false
	}
	if _, e := careerstate.ValidateIndex(c.Activation, []careerstate.Event{r.Event}); e != nil {
		return false
	}
	e := r.Event
	// The new public context follows the frozen ISO calendar grammar. Keep
	// the old stored Event validator untouched (time.Parse also accepts year 0).
	if len(e.Saturday) != 10 || e.Saturday[:4] == "0000" {
		return false
	}
	pid := c.Assignment.ProspectID
	if e.EffectiveAtMS > c.AsOfMS {
		return false
	}
	corner := e.CornerA
	which := 0
	if corner.ProspectID != pid {
		corner = e.CornerB
		which = 1
	}
	if corner.ProspectID != pid || corner.AccountID != c.Status.Pin.AccountID || corner.GymID != c.Assignment.GymID {
		return false
	}
	winner := "draw"
	if e.Winner == e.CornerA.ProspectID {
		winner = "a"
	} else if e.Winner == e.CornerB.ProspectID {
		winner = "b"
	}
	d, err := fightercareer.Award(winner, e.Method, "standard")
	if err != nil || r.Earned != d[which] || r.XPBefore != min(uint32(15), r.Ordinal-1) || r.XPApplied != min(uint32(15), r.Ordinal)-r.XPBefore || !historyTotalsValid(r.TotalsAfter, r.Ordinal) {
		return false
	}
	t := r.TotalsAfter
	if t.Wins < r.Earned.Wins || t.Losses < r.Earned.Losses || t.Draws < r.Earned.Draws || t.BasePoints < r.Earned.BasePoints {
		return false
	}
	prior := fightercareer.Totals{Wins: t.Wins - r.Earned.Wins, Losses: t.Losses - r.Earned.Losses, Draws: t.Draws - r.Earned.Draws, XP: r.XPBefore, BasePoints: t.BasePoints - r.Earned.BasePoints}
	return historyTotalsValid(prior, r.Ordinal-1) && (r.Ordinal != c.EventCount || t == c.Totals) && careerPreviewBound(r, MaxCareerHistoryRowSize)
}
func ValidateCareerHistoryRow(r CareerHistoryRow, c CareerHistoryCapture) (CareerHistoryRow, error) {
	if _, e := ValidateCareerHistoryCapture(c); e != nil || !historyRowValid(r, c) {
		return CareerHistoryRow{}, ErrInvalid
	}
	return r, nil
}
func ValidateCareerHistoryPage(p CareerHistoryPage) (CareerHistoryPage, error) {
	c, e := ValidateCareerHistoryCapture(p.Capture)
	if e != nil || p.Version != Version1 || p.After > c.EventCount || p.Items == nil || len(p.Items) > 8 {
		return CareerHistoryPage{}, ErrInvalid
	}
	n := uint32(len(p.Items))
	remaining := c.EventCount - p.After
	if (n == 0) != (remaining == 0) || n > remaining || p.Next != p.After+n || p.HasMore != (p.Next < c.EventCount) {
		return CareerHistoryPage{}, ErrInvalid
	}
	awards, results := map[string]bool{}, map[string]bool{}
	for i, r := range p.Items {
		if r.Ordinal != c.EventCount-p.After-uint32(i) || !historyRowValid(r, c) || awards[r.Event.AwardID] || results[r.Event.ResultKey] {
			return CareerHistoryPage{}, ErrInvalid
		}
		awards[r.Event.AwardID] = true
		results[r.Event.ResultKey] = true
		if i > 0 {
			newer := p.Items[i-1]
			if r.Event.EffectiveAtMS > newer.Event.EffectiveAtMS || r.Event.EffectiveAtMS == newer.Event.EffectiveAtMS && r.Event.AwardID >= newer.Event.AwardID {
				return CareerHistoryPage{}, ErrInvalid
			}
			t, err := fightercareer.Apply(r.TotalsAfter, newer.Earned)
			if err != nil || t != newer.TotalsAfter {
				return CareerHistoryPage{}, ErrInvalid
			}
		}
	}
	if !careerPreviewBound(p, MaxCareerHistoryPageSize) {
		return CareerHistoryPage{}, ErrInvalid
	}
	// Return a detached slice even to typed callers.
	p.Items = append([]CareerHistoryRow{}, p.Items...)
	return p, nil
}
func ValidateCareerHistoryPageLimit(p CareerHistoryPage, limit uint32) (CareerHistoryPage, error) {
	p, e := ValidateCareerHistoryPage(p)
	if e != nil || limit < 1 || limit > 8 || uint32(len(p.Items)) != min(limit, p.Capture.EventCount-p.After) {
		return CareerHistoryPage{}, ErrInvalid
	}
	return p, nil
}
func MarshalCareerHistoryCapture(c CareerHistoryCapture) ([]byte, error) {
	c, e := ValidateCareerHistoryCapture(c)
	if e != nil {
		return nil, e
	}
	return json.Marshal(c)
}
func ParseCareerHistoryCapture(raw []byte) (CareerHistoryCapture, error) {
	var c CareerHistoryCapture
	if historyDecode(raw, MaxCareerHistoryCaptureSize, &c) != nil {
		return CareerHistoryCapture{}, ErrInvalid
	}
	return ValidateCareerHistoryCapture(c)
}
func MarshalCareerHistoryRow(r CareerHistoryRow, c CareerHistoryCapture) ([]byte, error) {
	r, e := ValidateCareerHistoryRow(r, c)
	if e != nil {
		return nil, e
	}
	return json.Marshal(r)
}
func ParseCareerHistoryRow(raw []byte, c CareerHistoryCapture) (CareerHistoryRow, error) {
	var r CareerHistoryRow
	if historyDecode(raw, MaxCareerHistoryRowSize, &r) != nil {
		return CareerHistoryRow{}, ErrInvalid
	}
	return ValidateCareerHistoryRow(r, c)
}
func MarshalCareerHistoryPage(p CareerHistoryPage) ([]byte, error) {
	p, e := ValidateCareerHistoryPage(p)
	if e != nil {
		return nil, e
	}
	return json.Marshal(p)
}
func ParseCareerHistoryPage(raw []byte) (CareerHistoryPage, error) {
	var p CareerHistoryPage
	if historyDecode(raw, MaxCareerHistoryPageSize, &p) != nil {
		return CareerHistoryPage{}, ErrInvalid
	}
	return ValidateCareerHistoryPage(p)
}
