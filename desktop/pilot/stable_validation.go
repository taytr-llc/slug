// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospectservice"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func stableViewValid(v ManifestViewPin) bool {
	if v.Version != 1 || !canonicalManifestID(v.ViewID) || !canonicalManifestID(v.AccountID) || !canonicalManifestID(v.AccountAuthorityID) || v.Generation == 0 || v.Generation > gymstable.MaxSafeInteger || v.AuthorityPin.Version != 5 || v.AuthorityPin.Network != uint32(chaincfg.MainNetParams.Net) || v.AuthorityPin.Genesis != chaincfg.MainNetParams.GenesisHash.String() || v.AuthorityPin.AuthorityID == v.AccountAuthorityID || !canonicalManifestID(v.AuthorityPin.AuthorityID) || !canonicalManifestID(v.TransportTrustID) {
		return false
	}
	if _, err := fighterdescriptor.ResolveCandidate(v.AuthorityPin.DescriptorID); err != nil {
		return false
	}
	u, err := url.Parse(v.AuthorityOrigin)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || len(v.AuthorityOrigin) > 256 {
		return false
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || host != "127.0.0.1" {
		return false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port || v.AuthorityOrigin != "http://127.0.0.1:"+port {
		return false
	}
	trust, err := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: v.AuthorityOrigin, Token: strings.Repeat("0", 64), AuthorityPin: v.AuthorityPin})
	return err == nil && trust == v.TransportTrustID
}

func stableOne(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func stableErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "unauthorized", "forbidden", "stale_view", "busy", "stable_unavailable", "stable_corrupt", "stable_capacity", "stable_clock", "stable_sync_required", "stable_conflict", "stable_pending", "stable_not_found", "stable_cancelled", "outcome_unknown")
}
func stableOptionalID(s string) bool { return s == "" || canonicalManifestID(s) }
func stableTime(n int64) bool        { return n > 0 && n <= gymstable.MaxTimestampMS }
func stableSlots(s [10]string) bool {
	seen := map[string]bool{}
	for _, id := range s {
		if id == "" {
			continue
		}
		if !canonicalManifestID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func stableStatusValid(s gymstable.Status, v ManifestViewPin, storeID string) bool {
	id, err := gymstable.ID(s.Pin)
	if err != nil || s.Version != 1 || id != s.StoreID || s.StoreID != storeID || s.Pin.AccountAuthorityID != v.AccountAuthorityID || s.Pin.AccountID != v.AccountID || s.Pin.Source != v.AuthorityPin {
		return false
	}
	if s.Checkpoint > s.ObservedSourceCount || s.ObservedSourceCount > 1000 || s.CaughtUp != (s.Checkpoint == s.ObservedSourceCount) || s.RosterRevision > uint64(s.OperationCount) || s.OperationCount > 4096 || !stableSlots(s.Slots) || !stableOptionalID(s.PendingOperationID) || !stableOptionalID(s.LastOperationID) || s.LastAsOfMS < 0 || s.LastAsOfMS > gymstable.MaxTimestampMS {
		return false
	}
	return (s.OperationCount == 0) == (s.LastOperationID == "") && (s.PendingOperationID == "" || s.PendingOperationID == s.LastOperationID)
}
func stableSnapshotValid(s StableSnapshot) bool {
	if s.Version != 1 || !stableOne(s.State, "unavailable", "syncing", "ready", "paused", "failed") || s.Error != "" && !stableErrorCode(s.Error) || !stableOptionalID(s.ActiveJobID) || !stableOptionalID(s.QueuedJobID) || !stableOptionalID(s.LastJobID) || !stableOptionalID(s.PendingOperationID) {
		return false
	}
	if s.View == nil || s.Status == nil {
		return s.State == "unavailable" && s.View == nil && s.Status == nil && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == "" && s.PendingOperationID == ""
	}
	return s.State != "unavailable" && (s.ActiveJobID == "" || s.ActiveJobID != s.QueuedJobID) && stableViewValid(*s.View) && stableStatusValid(*s.Status, *s.View, s.Status.StoreID) && s.PendingOperationID == s.Status.PendingOperationID && (s.State != "ready" || s.Status.CaughtUp)
}
func stableIntentValid(i gymstable.Intent) bool {
	_, err := gymstable.ValidateSlotRequest(i.Request)
	return err == nil && i.Version == 1 && i.Sequence > 0 && i.Sequence <= 4096 && canonicalManifestID(i.OperationID) && i.Generation > 0 && i.Generation <= gymstable.MaxSafeInteger && canonicalManifestID(i.ViewID) && stableTime(i.PreparedAtMS)
}
func stableReceiptValid(r gymstable.MutationReceipt) bool {
	_, err := gymstable.ValidateSlotRequest(r.Request)
	if err != nil || r.Version != 1 || !canonicalManifestID(r.OperationID) || r.Generation == 0 || r.Generation > gymstable.MaxSafeInteger || !canonicalManifestID(r.ViewID) || !stableOptionalID(r.PreviousProspectID) || r.PreviousRevision > 4096 || r.Revision > 4096 || !stableSlots(r.Slots) || !stableTime(r.CommittedAtMS) {
		return false
	}
	delta := uint64(0)
	if r.Changed {
		delta = 1
	}
	return r.Request.ExpectedRevision == r.PreviousRevision && r.Revision == r.PreviousRevision+delta && r.Changed == (r.PreviousProspectID != r.Request.ProspectID) && r.Slots[r.Request.Slot-1] == r.Request.ProspectID
}
func stableCoreOperationValid(o gymstable.Operation) bool {
	if o.Version != 1 || !stableIntentValid(o.Intent) {
		return false
	}
	switch o.State {
	case "prepared":
		return o.Receipt == nil && o.AbandonedAtMS == 0
	case "abandoned":
		return o.Receipt == nil && stableTime(o.AbandonedAtMS) && o.AbandonedAtMS >= o.Intent.PreparedAtMS
	case "committed":
		if o.Receipt == nil || !stableReceiptValid(*o.Receipt) || o.AbandonedAtMS != 0 {
			return false
		}
		r := o.Receipt
		i := o.Intent
		return r.OperationID == i.OperationID && r.Generation == i.Generation && r.ViewID == i.ViewID && r.Request == i.Request && r.CommittedAtMS >= i.PreparedAtMS
	}
	return false
}
func stableOperationValid(o StableOperation) bool {
	return o.Version == 1 && stableViewValid(o.View) && canonicalManifestID(o.StoreID) && (o.Operation == nil || stableCoreOperationValid(*o.Operation))
}

func stableManifestShape(m fightermanifest.Manifest, seed string) bool {
	if m.Version != 1 || m.Generator != fightermanifest.GeneratorID || m.Tables != fightermanifest.TableHash || m.DNA.DNAVersion != 1 || m.DNA.Seed != seed || m.DNA.PrimeWeeks < 26 || m.DNA.PrimeWeeks > 52 || m.DNA.DeclineWeeks < 13 || m.DNA.DeclineWeeks > 39 {
		return false
	}
	ranges := map[string][2]int{"club": {250, 299}, "professional": {300, 349}, "contender": {350, 399}, "elite": {400, 449}, "generational": {450, 489}}
	bound, ok := ranges[m.DNA.Rarity]
	if !ok || m.DNA.TalentBudget < bound[0] || m.DNA.TalentBudget > bound[1] {
		return false
	}
	a := m.DNA.Attributes
	total := 0
	for _, n := range []int{a.Power, a.Speed, a.Chin, a.Stamina, a.Skill} {
		if n < 1 || n > 100 {
			return false
		}
		total += n
	}
	if total != m.DNA.TalentBudget {
		return false
	}
	ascii := func(s string, min, max int) bool {
		if len(s) < min || len(s) > max {
			return false
		}
		for _, c := range s {
			if c < 32 || c > 126 {
				return false
			}
		}
		return true
	}
	if !ascii(m.Name.First, 1, 64) || !ascii(m.Name.Last, 1, 64) || m.Name.Display != m.Name.First+" "+m.Name.Last {
		return false
	}
	p := m.Physical
	if p.Height < 62 || p.Height > 80 || p.Reach < p.Height-2 || p.Reach > p.Height+4 || p.Weight < 128 || p.Weight > 268 || !stableOne(p.Stance, "orthodox", "southpaw") || !stableOne(m.Style, "slugger", "outfighter", "boxer_puncher", "pressure", "boxer") {
		return false
	}
	class := "heavy"
	switch {
	case p.Weight <= 145:
		class = "light"
	case p.Weight <= 170:
		class = "welter"
	case p.Weight <= 195:
		class = "middle"
	case p.Weight <= 225:
		class = "light_heavy"
	}
	if p.Class != class {
		return false
	}
	r := m.Portrait
	return r.Version == 1 && r.Skin >= 0 && r.Skin <= 5 && r.HairStyle >= 0 && r.HairStyle <= 7 && r.HairColor >= 0 && r.HairColor <= 5 && r.FacialHair >= 0 && r.FacialHair <= 4 && r.HeadShape >= 0 && r.HeadShape <= 3 && r.Mouth >= 0 && r.Mouth <= 3 && r.Brow >= 0 && r.Brow <= 2 && r.Background >= 0 && r.Background <= 7 && stableOne(r.Scar, "none", "left_brow", "right_cheek") && r.ShoulderJitter >= -6 && r.ShoulderJitter <= 6
}
func stableItemValid(i gymstable.Item, s gymstable.Status, asOf int64) bool {
	a := i.Assignment
	m := i.Manifest
	c := i.Condition
	if a.Version != 1 || a.Sequence == 0 || a.Sequence > s.Checkpoint || a.GymID != s.Pin.Identity.GymID || !canonicalManifestID(a.ProspectID) || !canonicalManifestID(a.WorkHash) || !canonicalManifestID(a.ManifestDigest) || !canonicalManifestID(a.PortraitDigest) || !canonicalManifestID(a.SourceRecordDigest) || a.DescriptorID != s.Pin.Source.DescriptorID || !stableTime(a.BornAtMS) || !stableTime(a.AcceptedAtMS) || a.AcceptedAtMS < a.BornAtMS || a.AcceptedAtMS >= a.BornAtMS+300000 || asOf < a.AcceptedAtMS || !stableManifestShape(m, a.WorkHash) {
		return false
	}
	if c.Version != 1 || c.ProjectorID != s.Pin.ProjectorID || c.DNAVersion != 1 || c.Seed != a.WorkHash || c.BornAtMS != a.BornAtMS || c.AsOfMS != asOf || c.ProXP != 0 {
		return false
	}
	const week int64 = 604800000
	elapsed := asOf - a.BornAtMS
	age := elapsed / week
	prime, decline := int64(m.DNA.PrimeWeeks), int64(m.DNA.DeclineWeeks)
	step := max(int64(0), min(decline, age-prime))
	phase := "prime"
	if age >= prime+decline {
		phase = "retired"
	} else if age >= prime {
		phase = "decline"
	}
	if c.AgeWeeks != age || c.Phase != phase || c.ElapsedDeclineWeeks != int(step) || c.PrimeRemainingWeeks != int(max(int64(0), prime-age)) || c.PrimeRemainingMS != max(int64(0), prime*week-elapsed) || c.CareerRemainingMS != max(int64(0), (prime+decline)*week-elapsed) || c.AgedAttributesTenths != c.LeagueAttributesTenths {
		return false
	}
	original := m.DNA.Attributes
	aged := c.AgedAttributesTenths
	values := []int{original.Power, original.Speed, original.Chin, original.Stamina, original.Skill}
	actual := []int{aged.Power, aged.Speed, aged.Chin, aged.Stamina, aged.Skill}
	loss := []int64{20, 40, 30, 30, 10}
	for x, n := range values {
		num := int64(n) * (100*decline - loss[x]*step)
		den := 10 * decline
		q, rem := num/den, num%den
		if 2*rem > den || 2*rem == den && q%2 == 1 {
			q++
		}
		if actual[x] != int(q) {
			return false
		}
	}
	return c.RatingHundredths == 10*aged.Power+25*aged.Speed+2*aged.Chin+4*aged.Stamina+8*aged.Skill
}
func stablePageItems(items []gymstable.Item, s gymstable.Status, asOf int64, first uint64) bool {
	if items == nil || len(items) > 8 || !stableTime(asOf) || s.LastAsOfMS != asOf {
		return false
	}
	seen := map[string]bool{}
	previous := uint64(0)
	for n, i := range items {
		sequence := i.Assignment.Sequence
		ordered := sequence == first+uint64(n)
		if s.Pin.Version == gymstable.SharedPinVersion2 {
			ordered = sequence >= first && sequence > previous
		}
		if !ordered || !stableItemValid(i, s, asOf) || seen[i.Assignment.ProspectID] {
			return false
		}
		previous = sequence
		seen[i.Assignment.ProspectID] = true
	}
	return true
}
func stableJobValid(j StableJob) bool {
	if j.Version != 1 || !canonicalManifestID(j.JobID) || !canonicalManifestID(j.StoreID) || !stableViewValid(j.View) || !stableOne(j.Kind, "sync", "inventory", "roster", "prepare", "apply") || !stableOne(j.State, "queued", "running", "done", "cancelled", "failed", "outcome_unknown") {
		return false
	}
	if j.Kind == "apply" {
		if !canonicalManifestID(j.OperationID) {
			return false
		}
	} else if j.OperationID != "" {
		return false
	}
	count := 0
	for _, present := range []bool{j.Sync != nil, j.Page != nil, j.Roster != nil, j.Intent != nil, j.Mutation != nil} {
		if present {
			count++
		}
	}
	if j.State != "done" {
		if count != 0 {
			return false
		}
		switch j.State {
		case "queued", "running":
			return j.Error == ""
		case "outcome_unknown":
			return j.Error == "outcome_unknown"
		case "cancelled":
			return j.Error == "stable_cancelled"
		default:
			return stableErrorCode(j.Error)
		}
	}
	if count != 1 || j.Error != "" {
		return false
	}
	switch j.Kind {
	case "sync":
		p := j.Sync
		if p == nil || p.Version != 1 || !stableStatusValid(p.Status, j.View, j.StoreID) || uint64(len(p.Items)) > p.Status.Checkpoint {
			return false
		}
		first := p.Status.Checkpoint - uint64(len(p.Items)) + 1
		if p.Status.Pin.Version == gymstable.SharedPinVersion2 {
			// A shared sync consumes at most one bounded global-source page,
			// including foreign records that yield no local assignments.
			first = p.Status.Checkpoint - min(p.Status.Checkpoint, uint64(gymstable.MaxPageSize)) + 1
		}
		return stablePageItems(p.Items, p.Status, p.AsOfMS, first)
	case "inventory":
		p := j.Page
		if p == nil || p.Version != 1 || !stableStatusValid(p.Status, j.View, j.StoreID) || p.After > p.Status.Checkpoint || p.NextAfter > p.Status.Checkpoint {
			return false
		}
		if p.Status.Pin.Version == gymstable.SharedPinVersion2 {
			if p.HasMore {
				if len(p.Items) == 0 || p.NextAfter != p.Items[len(p.Items)-1].Assignment.Sequence || p.NextAfter >= p.Status.Checkpoint {
					return false
				}
			} else if p.NextAfter != p.Status.Checkpoint {
				return false
			}
		} else if p.NextAfter != p.After+uint64(len(p.Items)) || p.HasMore != (p.NextAfter < p.Status.Checkpoint) {
			return false
		}
		return stablePageItems(p.Items, p.Status, p.AsOfMS, p.After+1)
	case "roster":
		p := j.Roster
		if p == nil || p.Version != 1 || !stableStatusValid(p.Status, j.View, j.StoreID) || !stableTime(p.AsOfMS) || p.Status.LastAsOfMS != p.AsOfMS || p.FirstSlot < 1 || p.FirstSlot > 10 || p.NextSlot != 0 && (p.NextSlot <= p.FirstSlot || p.NextSlot > 10) || p.Items == nil || len(p.Items) > 8 {
			return false
		}
		end := p.NextSlot
		if end == 0 {
			end = 11
		}
		if end-p.FirstSlot > 8 {
			return false
		}
		idx := 0
		for slot := p.FirstSlot; slot < end; slot++ {
			id := p.Status.Slots[slot-1]
			if id == "" {
				continue
			}
			if idx >= len(p.Items) || p.Items[idx].Slot != slot || p.Items[idx].Item.Assignment.ProspectID != id || !stableItemValid(p.Items[idx].Item, p.Status, p.AsOfMS) {
				return false
			}
			idx++
		}
		return idx == len(p.Items)
	case "prepare":
		return j.Intent != nil && stableIntentValid(*j.Intent) && j.Intent.ViewID == j.View.ViewID && j.Intent.Generation == j.View.Generation
	case "apply":
		return j.Mutation != nil && j.Mutation.Version == 1 && stableReceiptValid(j.Mutation.Receipt) && j.Mutation.Receipt.OperationID == j.OperationID && j.Mutation.Receipt.ViewID == j.View.ViewID && j.Mutation.Receipt.Generation == j.View.Generation
	}
	return false
}
