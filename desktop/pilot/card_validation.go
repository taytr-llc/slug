// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

func cardErrorCode(s string) bool {
	switch s {
	case "invalid_request", "unauthorized", "forbidden", "stale_view", "busy", "outcome_unknown", "card_unavailable", "card_setup_incomplete", "card_corrupt", "card_capacity", "card_clock", "card_calendar", "card_not_found", "card_source_incomplete", "card_source_conflict", "card_source_changed", "card_cancelled":
		return true
	}
	return false
}
func cardRequestValid(r CardRequest) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	b := r
	b.ID = 0
	b.Command = ""
	switch r.Command {
	case "card_snapshot":
	case "card_status", "card_record", "card_build":
		if !canonicalManifestID(r.ViewID) || !entryDateValid(r.Saturday) {
			return false
		}
		b.ViewID = ""
		b.Saturday = ""
	case "card_history":
		if !canonicalManifestID(r.ViewID) || r.AfterCardID != "" && !canonicalManifestID(r.AfterCardID) || r.Limit < 1 || r.Limit > 8 {
			return false
		}
		b.ViewID = ""
		b.AfterCardID = ""
		b.Limit = 0
	case "card_job", "card_cancel":
		if !canonicalManifestID(r.ViewID) || !canonicalManifestID(r.JobID) {
			return false
		}
		b.ViewID = ""
		b.JobID = ""
	default:
		return false
	}
	return b == (CardRequest{})
}
func cardSnapshotValid(s CardSnapshot) bool {
	if s.Version != 1 {
		return false
	}
	blank := s.Pin == nil && s.StoreID == "" && s.SourceCreatedAtMS == 0 && s.CreatedAtMS == 0 && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == ""
	if s.State == "unavailable" {
		return blank && s.View == nil && s.Error == "card_unavailable"
	}
	if s.View == nil || !stableViewValid(*s.View) {
		return false
	}
	if s.State == "setup_incomplete" {
		return blank && s.Error == "card_setup_incomplete"
	}
	if s.Pin == nil || !stableTime(s.SourceCreatedAtMS) || !stableTime(s.CreatedAtMS) || s.CreatedAtMS < s.SourceCreatedAtMS {
		return false
	}
	p, e := gymcard.ValidatePin(*s.Pin)
	if e != nil {
		return false
	}
	id, e := gymcard.ID(p)
	if e != nil || id != s.StoreID {
		return false
	}
	if _, ok := eventViewParticipant(*s.View, p.SourcePin.SourcePin); !ok {
		return false
	}
	switch s.State {
	case "ready", "busy":
		if s.Error != "" {
			return false
		}
	case "failed":
		if s.Error != "card_unavailable" && s.Error != "card_corrupt" && s.Error != "card_clock" && s.Error != "card_source_changed" {
			return false
		}
	default:
		return false
	}
	seen := map[string]bool{}
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" {
			if !canonicalManifestID(id) || seen[id] {
				return false
			}
			seen[id] = true
		}
	}
	if s.ActiveJobID != "" && s.QueuedJobID != "" {
		return false
	}
	return (s.ActiveJobID == "" && s.QueuedJobID == "") || s.State == "busy"
}
func cardHashParts(domain string, ids ...string) string {
	b := []byte(domain)
	for _, id := range ids {
		x, e := hex.DecodeString(id)
		if e != nil {
			return ""
		}
		b = append(b, x...)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func cardIDShape(store, event string) string {
	return cardHashParts("SLUG candidate card event\x00v1\x00", store, event)
}
func cardRecordHash(r gymcard.Record) string {
	return seedHash("SLUG candidate card record\x00v1\x00", r)
}
func cardSourceHash(r gymseed.CardSource) string {
	return seedHash("SLUG candidate card source\x00v1\x00", r)
}

// Card transports preserve already checked authority evidence. These private
// checks never derive a condition/name/Plan, verify a signature, hash PoW,
// render an image or call a source store. Core validates those facts once.
func cardSourceShape(s gymseed.CardSource) bool {
	d := s.Declaration.Declaration
	a := s.Anchor
	t := s.Target
	f := s.Finality
	if s.Version != 1 || !canonicalManifestID(s.StoreID) || !canonicalManifestID(s.EventKey) || !eventRecordShape(s.SourceReceipt) || s.Declaration.Version != 1 || d.Version != 1 || !seedHex(s.Declaration.Signature, 64) || d.StoreID != s.StoreID || d.EventKey != s.EventKey || d.Event != s.SourceReceipt.Input.Event || d.SourceStoreID != d.Event.StoreID || d.SnapshotDigest != s.SourceReceipt.SnapshotDigest || d.ReceiptDigest != seedHash("SLUG candidate cutoff receipt\x00v1\x00", s.SourceReceipt) || d.RulesID != gymseed.RulesID() || d.FloorHeight != 0 || d.FloorHash != d.Genesis || seedEventKey(d) != s.EventKey {
		return false
	}
	for _, x := range []string{d.PublisherChainID, d.Genesis, d.PublisherPublicKey, d.CheckpointPublicKey} {
		if !canonicalManifestID(x) {
			return false
		}
	}
	if a.Height > math.MaxInt32 || !canonicalManifestID(a.Hash) || !seedHex(a.Header, 80) || !canonicalManifestID(a.TxID) || !canonicalManifestID(a.RawTxDigest) || a.RawTxSize < 1 || a.RawTxSize > 4000000 || a.TxCount < 1 || a.TxCount > 1000000 || a.TxIndex >= a.TxCount || a.OutputIndex >= 4000000/9 || a.MerkleBranch == nil || len(a.MerkleBranch) > 20 || !seedHex(a.Payload, 73) {
		return false
	}
	for _, x := range a.MerkleBranch {
		if !canonicalManifestID(x) {
			return false
		}
	}
	payload := "534c55475345454401" + s.EventKey + seedHash("SLUG candidate seed declaration id\x00v1\x00", d)
	if a.Payload != payload || t.Height != a.Height+1 || t.Height > math.MaxInt32 || !canonicalManifestID(t.Hash) || !seedHex(t.Header, 80) || !stableTime(t.ObservedAtMS) || t.ObservedAtMS < s.SourceReceipt.CompletedAtMS || !seedTipShape(t.ObservedTip) || t.ObservedTip.Height < t.Height || !stableTime(f.ObservedAtMS) || f.ObservedAtMS < t.ObservedAtMS || !seedTipShape(f.ObservedTip) || f.ObservedTip.Height < t.Height || f.TargetDescendants != f.ObservedTip.Height-t.Height || f.TargetDescendants < 60 {
		return false
	}
	raw, _ := hex.DecodeString(t.Header)
	parent, e := chainhash.NewHashFromStr(a.Hash)
	if e != nil || !bytes.Equal(raw[4:36], parent[:]) {
		return false
	}
	c := f.Checkpoint
	return c.Version == 1 && uint32(c.Network) == d.Network && c.GenesisHash.String() == d.Genesis && c.Height == int32(t.Height) && c.Hash.String() == t.Hash && len(c.Signature) == 64
}
func cardStateShape(s gymseed.CardState) bool {
	return s.Version == 1 && seedStatusShape(s.Status) && ((s.Status.FinalizedAtMS == 0 && s.SourceDigest == "") || (s.Status.FinalizedAtMS > 0 && canonicalManifestID(s.SourceDigest)))
}
func cardCountsShape(c gymcard.Counts) bool {
	r := c.Reasons
	return c.TotalCount <= 10 && c.EligibleCount <= c.TotalCount && c.PairedCount <= c.EligibleCount && c.UnmatchedCount <= c.TotalCount && c.TotalCount == c.PairedCount+c.UnmatchedCount && r.RetiredAtCutoff <= 10 && r.NotRosteredAtCutoff <= 10 && r.NoOpponentInClass <= 10 && r.OpponentIneligibleAtCutoff <= 10 && r.OutsideRatingBand <= 10 && r.RetiredAtCutoff+r.NotRosteredAtCutoff+r.NoOpponentInClass+r.OpponentIneligibleAtCutoff+r.OutsideRatingBand == c.UnmatchedCount
}
func cardClass(s string) int {
	for i, x := range []string{"light", "welter", "middle", "light_heavy", "heavy"} {
		if s == x {
			return i
		}
	}
	return -1
}
func cardCountEntrant(c *gymcard.Counts, e gymcard.Entrant) {
	c.TotalCount++
	if e.CutoffEligibility == "eligible" {
		c.EligibleCount++
	}
	if e.State == "paired" {
		c.PairedCount++
		return
	}
	c.UnmatchedCount++
	switch e.Reason {
	case "retired_at_cutoff":
		c.Reasons.RetiredAtCutoff++
	case "not_rostered_at_cutoff":
		c.Reasons.NotRosteredAtCutoff++
	case "no_opponent_in_class":
		c.Reasons.NoOpponentInClass++
	case "opponent_ineligible_at_cutoff":
		c.Reasons.OpponentIneligibleAtCutoff++
	case "outside_rating_band":
		c.Reasons.OutsideRatingBand++
	}
}
func cardPlanShape(p gymcard.Plan) bool {
	if p.Version != 1 || !canonicalManifestID(p.CardID) || !canonicalManifestID(p.InputID) || !canonicalManifestID(p.Seed) || p.RecordBasis != "candidate_zero_record_no_results_authority" || !cardCountsShape(p.Counts) || p.Gyms == nil || len(p.Gyms) != 2 || p.Entrants == nil || len(p.Entrants) > 10 || p.Bouts == nil || len(p.Bouts) > 5 || p.BoutCount != uint32(len(p.Bouts)) || p.Counts.PairedCount != 2*p.BoutCount {
		return false
	}
	gyms := map[string]*gymcard.Counts{}
	for i, g := range p.Gyms {
		if !canonicalManifestID(g.GymID) || i > 0 && p.Gyms[i-1].GymID >= g.GymID || !cardCountsShape(g.Counts) || g.Counts.TotalCount > 5 {
			return false
		}
		gyms[g.GymID] = &gymcard.Counts{}
	}
	var count gymcard.Counts
	seen := map[string]gymcard.Entrant{}
	classes := map[string]bool{}
	for i, e := range p.Entrants {
		ci := cardClass(e.Class)
		if ci < 0 || !canonicalManifestID(e.ProspectID) || !canonicalManifestID(e.AccountID) || gyms[e.GymID] == nil || len(e.DisplayName) < 1 || len(e.DisplayName) > 128 || e.RatingHundredths > 100000 || e.ProfessionalRecord != (gymcard.ProfessionalRecord{}) {
			return false
		}
		for _, b := range []byte(e.DisplayName) {
			if b < 32 || b > 126 {
				return false
			}
		}
		if _, ok := seen[e.ProspectID]; ok {
			return false
		}
		key := e.GymID + e.Class
		if classes[key] {
			return false
		}
		classes[key] = true
		if i > 0 {
			prev := p.Entrants[i-1]
			pc := cardClass(prev.Class)
			if pc > ci || pc == ci && (prev.GymID > e.GymID || prev.GymID == e.GymID && prev.ProspectID >= e.ProspectID) {
				return false
			}
		}
		if e.CutoffEligibility != "eligible" && e.CutoffEligibility != "retired" && e.CutoffEligibility != "not_rostered" {
			return false
		}
		if e.State == "paired" {
			if e.CutoffEligibility != "eligible" || e.RatingHundredths == 0 || e.Reason != "" || !canonicalManifestID(e.BoutID) {
				return false
			}
		} else if e.State == "unmatched" {
			if e.BoutID != "" {
				return false
			}
			switch e.Reason {
			case "retired_at_cutoff":
				if e.CutoffEligibility != "retired" {
					return false
				}
			case "not_rostered_at_cutoff":
				if e.CutoffEligibility != "not_rostered" {
					return false
				}
			case "no_opponent_in_class", "opponent_ineligible_at_cutoff", "outside_rating_band":
				if e.CutoffEligibility != "eligible" {
					return false
				}
			default:
				return false
			}
		} else {
			return false
		}
		cardCountEntrant(&count, e)
		cardCountEntrant(gyms[e.GymID], e)
		seen[e.ProspectID] = e
	}
	if count != p.Counts {
		return false
	}
	for _, g := range p.Gyms {
		if *gyms[g.GymID] != g.Counts {
			return false
		}
	}
	used := map[string]bool{}
	last := -1
	for _, b := range p.Bouts {
		ci := cardClass(b.Class)
		a, aok := seen[b.CornerA]
		z, zok := seen[b.CornerB]
		if ci <= last || !aok || !zok || b.CornerA >= b.CornerB || a.GymID == z.GymID || a.Class != b.Class || z.Class != b.Class || a.State != "paired" || z.State != "paired" || a.BoutID != b.BoutID || z.BoutID != b.BoutID || used[b.CornerA] || used[b.CornerB] {
			return false
		}
		last = ci
		used[b.CornerA] = true
		used[b.CornerB] = true
		raw := []byte("SLUG candidate card bout\x00v1\x00")
		id, _ := hex.DecodeString(p.InputID)
		raw = append(raw, id...)
		raw = binary.LittleEndian.AppendUint32(raw, uint32(ci))
		for _, x := range []string{b.CornerA, b.CornerB} {
			v, _ := hex.DecodeString(x)
			raw = append(raw, v...)
		}
		h := sha256.Sum256(raw)
		if hex.EncodeToString(h[:]) != b.BoutID || cardHashParts("SLUG candidate card bout seed\x00v1\x00", p.Seed, b.BoutID) != b.Seed {
			return false
		}
	}
	return len(used) == int(p.Counts.PairedCount)
}
func cardRecordShape(r gymcard.Record) bool {
	if r.Version != 1 || !canonicalManifestID(r.StoreID) || !cardSourceShape(r.Source) || !cardPlanShape(r.Plan) || r.CardID != cardIDShape(r.StoreID, r.Source.SourceReceipt.Input.Event.EventID) || r.Plan.CardID != r.CardID || r.SourceDigest != cardSourceHash(r.Source) || !stableTime(r.CreatedAtMS) || r.CreatedAtMS < r.Source.Finality.ObservedAtMS {
		return false
	}
	raw := []byte("SLUG candidate card input\x00v1\x00")
	for _, x := range []string{r.CardID, r.Source.SourceReceipt.SnapshotDigest, r.Source.EventKey} {
		v, _ := hex.DecodeString(x)
		raw = append(raw, v...)
	}
	raw = binary.LittleEndian.AppendUint32(raw, r.Source.Target.Height)
	for _, x := range []string{r.Source.Target.Hash, gymcard.RulesID()} {
		v, _ := hex.DecodeString(x)
		raw = append(raw, v...)
	}
	h := sha256.Sum256(raw)
	if r.Plan.InputID != hex.EncodeToString(h[:]) || r.Plan.Seed != cardHashParts("SLUG candidate card seed\x00v1\x00", r.Plan.InputID, r.Source.Target.Hash) {
		return false
	}
	n := 0
	for _, p := range r.Source.SourceReceipt.Input.Participants {
		for _, m := range p.Members {
			n++
			found := false
			for _, e := range r.Plan.Entrants {
				if e.ProspectID == m.Assignment.ProspectID {
					if e.GymID != p.Owner.GymID || e.AccountID != p.Owner.AccountID || e.Class != m.Class || e.RatingHundredths != uint32(m.ConditionAtCutoff.RatingHundredths) || e.CutoffEligibility != m.Eligibility {
						return false
					}
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return n == len(r.Plan.Entrants)
}
func cardStatusShape(s gymcard.Status) bool {
	if s.Version != 1 || !canonicalManifestID(s.StoreID) || !eventShape(s.Event) || s.CardID != cardIDShape(s.StoreID, s.Event.EventID) || !stableTime(s.ObservedAtMS) || !cardStateShape(s.Source) || s.Source.Status.Event != s.Event || s.Source.Status.ObservedAtMS > s.ObservedAtMS || !cardCountsShape(s.Counts) || s.BoutCount > 5 || s.Counts.PairedCount != 2*s.BoutCount {
		return false
	}
	if !s.Exists {
		return s.RecordDigest == "" && s.CreatedAtMS == 0 && s.Counts == (gymcard.Counts{}) && s.BoutCount == 0 && !s.Usable
	}
	// Standalone Status does not carry the saved Record's SourceDigest. A
	// healthy current source may still differ from that immutable saved source.
	return canonicalManifestID(s.RecordDigest) && stableTime(s.CreatedAtMS) && s.CreatedAtMS <= s.ObservedAtMS && s.Source.Status.FinalizedAtMS <= s.CreatedAtMS && s.Source.SourceDigest != "" && (!s.Usable || cardSourceHealthy(s))
}
func cardSourceHealthy(s gymcard.Status) bool {
	return s.Source.Status.FinalizedAtMS > 0 && s.Source.Status.ConflictCount == 0 && !s.Source.Status.ConflictOverflow
}
func cardResultShape(r gymcard.Record, s gymcard.Status) bool {
	return cardRecordShape(r) && cardStatusShape(s) && s.Exists && s.StoreID == r.StoreID && s.CardID == r.CardID && s.RecordDigest == cardRecordHash(r) && s.CreatedAtMS == r.CreatedAtMS && s.Source.SourceDigest == r.SourceDigest && s.Usable == cardSourceHealthy(s) && s.Counts == r.Plan.Counts && s.BoutCount == r.Plan.BoutCount && s.Event == r.Source.SourceReceipt.Input.Event && s.Source.Status.StoreID == r.Source.StoreID && s.Source.Status.EventKey == r.Source.EventKey
}
func cardHistoryShape(h gymcard.HistoryPage) bool {
	if h.Version != 1 || !canonicalManifestID(h.StoreID) || !stableTime(h.ObservedAtMS) || h.AfterCardID != "" && !canonicalManifestID(h.AfterCardID) || h.Cards == nil || len(h.Cards) > 8 {
		return false
	}
	for i, s := range h.Cards {
		if !cardStatusShape(s) || !s.Exists || s.StoreID != h.StoreID || s.ObservedAtMS != h.ObservedAtMS || s.CardID == h.AfterCardID || i > 0 && (h.Cards[i-1].Event.CutoffMS > s.Event.CutoffMS || h.Cards[i-1].Event.CutoffMS == s.Event.CutoffMS && h.Cards[i-1].CardID >= s.CardID) {
			return false
		}
	}
	if h.HasMore {
		return len(h.Cards) > 0 && h.NextCardID == h.Cards[len(h.Cards)-1].CardID
	}
	return h.NextCardID == ""
}
func cardJobValid(j *CardJob) bool {
	if j.Version != 1 || !canonicalManifestID(j.JobID) || j.Kind != "build" || !stableViewValid(j.View) || !canonicalManifestID(j.StoreID) || !canonicalManifestID(j.CardID) || !entryDateValid(j.Saturday) {
		return false
	}
	switch j.State {
	case "queued", "running":
		return j.Error == "" && j.Result == nil
	case "failed":
		return j.Result == nil && cardErrorCode(j.Error) && j.Error != "card_setup_incomplete"
	case "cancelled":
		return j.Result == nil && j.Error == "card_cancelled"
	case "completed":
		return j.Error == "" && j.Result != nil && j.Result.Version == 1 && cardResultShape(j.Result.Record, j.Result.Status) && j.Result.Record.CardID == j.CardID && j.Result.Record.StoreID == j.StoreID && j.Result.Status.Event.Saturday == j.Saturday
	}
	return false
}
func cardResponseValid(r *CardResponse) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	n := 0
	for _, x := range []bool{r.Snapshot != nil, r.Status != nil, r.Record != nil, r.History != nil, r.Job != nil} {
		if x {
			n++
		}
	}
	if !r.OK {
		return n == 0 && r.View == nil && r.StoreID == "" && cardErrorCode(r.Error)
	}
	if n != 1 || r.Error != "" {
		return false
	}
	if r.Snapshot != nil {
		return cardSnapshotValid(*r.Snapshot) && reflect.DeepEqual(r.View, r.Snapshot.View) && r.StoreID == r.Snapshot.StoreID
	}
	if r.View == nil || !stableViewValid(*r.View) || !canonicalManifestID(r.StoreID) {
		return false
	}
	switch {
	case r.Status != nil:
		return cardStatusShape(*r.Status) && r.Status.StoreID == r.StoreID
	case r.Record != nil:
		return r.Record.Version == 1 && cardResultShape(r.Record.Record, r.Record.Status) && r.Record.Record.StoreID == r.StoreID
	case r.History != nil:
		return cardHistoryShape(*r.History) && r.History.StoreID == r.StoreID
	case r.Job != nil:
		return cardJobValid(r.Job) && r.Job.StoreID == r.StoreID && r.Job.View == *r.View
	}
	return false
}
func cloneCardJob(v CardJob) CardJob {
	raw, _ := json.Marshal(v)
	var out CardJob
	_ = json.Unmarshal(raw, &out)
	return out
}
func cloneCardPin(v gymcard.Pin) gymcard.Pin {
	raw, _ := json.Marshal(v)
	var out gymcard.Pin
	_ = json.Unmarshal(raw, &out)
	return out
}
