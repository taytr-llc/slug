// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

const MaxCareerResponseSize = 212992
const MaxCareerRequestSize = 4096

type CareerControllerAPI interface {
	ResultControllerAPI
	CareerView(string) (ManifestViewPin, error)
	CareerInventory(context.Context, string, string, uint64, uint32) (gymstable.CareerPage, error)
	CareerRoster(context.Context, string, string, uint32, uint32) (gymstable.CareerRosterPage, error)
}
type CareerRequest struct {
	ID        uint64 `json:"id"`
	Command   string `json:"command"`
	ViewID    string `json:"view_id"`
	JobID     string `json:"job_id"`
	After     uint64 `json:"after"`
	FirstSlot uint32 `json:"first_slot"`
	Limit     uint32 `json:"limit"`
}
type CareerResponse struct {
	ID      uint64                      `json:"id"`
	JobID   string                      `json:"job_id"`
	OK      bool                        `json:"ok"`
	View    *ManifestViewPin            `json:"view"`
	StoreID string                      `json:"store_id"`
	Page    *gymstable.CareerPage       `json:"page"`
	Roster  *gymstable.CareerRosterPage `json:"roster"`
	Error   string                      `json:"error"`
}

func careerRequestValid(r CareerRequest) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger || !canonicalManifestID(r.ViewID) || !canonicalManifestID(r.JobID) || r.Limit < 1 || r.Limit > 8 {
		return false
	}
	switch r.Command {
	case "career_inventory":
		return r.FirstSlot == 0 && r.After <= gymstable.MaxAssignments
	case "career_roster":
		return r.After == 0 && r.FirstSlot >= 1 && r.FirstSlot <= 10
	}
	return false
}
func MarshalCareerRequest(r CareerRequest) ([]byte, error) {
	if !careerRequestValid(r) {
		return nil, careerstate.ErrInvalid
	}
	return json.Marshal(r)
}
func ParseCareerRequest(raw []byte) (CareerRequest, error) {
	var r CareerRequest
	_, e := stableDecode(raw, MaxCareerRequestSize, 2, &r)
	if e != nil || !careerRequestValid(r) {
		return CareerRequest{}, careerstate.ErrInvalid
	}
	return r, nil
}
func careerError(e error) string {
	if errors.Is(e, gymstable.ErrOutcomeUnknown) {
		return "outcome_unknown"
	}
	for _, x := range []error{careerstate.ErrInvalid, careerstate.ErrInactive, careerstate.ErrUnfenced, careerstate.ErrClosed, careerstate.ErrPoisoned, careerstate.ErrCancelled, careerstate.ErrCorrupt, gymstable.ErrCareerCandidateIneligible} {
		if errors.Is(e, x) {
			return x.Error()
		}
	}
	return stableProtocolError(e)
}
func careerErrorCode(s string) bool {
	return stableErrorCode(s) || stableOne(s, "career_invalid", "career_inactive", "career_unfenced", "career_closed", "career_poisoned", "career_cancelled", "career_corrupt", "career_candidate_ineligible")
}
func careerOverlayValid(a careerstate.Activation, digest string, asOf int64, source []gymstable.Item, items []gymstable.CareerItem) bool {
	if careerstate.ValidateActivation(a) != nil || !canonicalManifestID(digest) || asOf < a.ActivatedAtMS || !stableTime(asOf) || items == nil || len(items) != len(source) || len(items) > 8 {
		return false
	}
	for n, x := range items {
		old := source[n]
		t := x.Totals
		count := uint64(t.Wins) + uint64(t.Losses) + uint64(t.Draws)
		// This candidate policy has only standard bouts and at most 256 retained awards.
		if x.ProspectID != old.Assignment.ProspectID || count > 256 || uint64(t.XP) != min(uint64(15), count) || t.BasePoints < 10*t.Wins+4*t.Draws || t.BasePoints > 14*t.Wins+4*t.Draws || (t.BasePoints-10*t.Wins-4*t.Draws)%4 != 0 {
			return false
		}
		expected, e := fightercondition.Project(1, old.Manifest.DNA, old.Assignment.BornAtMS, asOf, int(t.XP))
		if e != nil || !reflect.DeepEqual(expected, x.Condition) {
			return false
		}
	}
	return true
}
func careerResponseValid(r CareerResponse) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	if !r.OK {
		return r.JobID == "" && r.View == nil && r.StoreID == "" && r.Page == nil && r.Roster == nil && careerErrorCode(r.Error)
	}
	if !canonicalManifestID(r.JobID) || r.View == nil || !stableViewValid(*r.View) || !canonicalManifestID(r.StoreID) || r.Error != "" || (r.Page == nil) == (r.Roster == nil) {
		return false
	}
	// Reuse the unchanged public Stable page validator with private fixed framing;
	// the carrier here is validation scaffolding, never a retained or emitted Job.
	j := StableJob{Version: 1, JobID: strings.Repeat("0", 64), State: "done", View: *r.View, StoreID: r.StoreID}
	if p := r.Page; p != nil {
		j.Kind = "inventory"
		j.Page = &p.Source
		b, _ := json.Marshal(p)
		return len(b) <= gymstable.MaxCareerPageSize && p.Version == 1 && p.Source.AsOfMS == p.AsOfMS && stableJobValid(j) && careerOverlayValid(p.Activation, p.CareerDigest, p.AsOfMS, p.Source.Items, p.Items)
	}
	p := r.Roster
	j.Kind = "roster"
	j.Roster = &p.Source
	items := make([]gymstable.Item, len(p.Source.Items))
	for n, x := range p.Source.Items {
		items[n] = x.Item
	}
	b, _ := json.Marshal(p)
	return len(b) <= gymstable.MaxCareerPageSize && p.Version == 1 && p.Source.AsOfMS == p.AsOfMS && stableJobValid(j) && careerOverlayValid(p.Activation, p.CareerDigest, p.AsOfMS, items, p.Items)
}
func MarshalCareerResponse(r CareerResponse) ([]byte, error) {
	if !careerResponseValid(r) {
		return nil, careerstate.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw)+1 > MaxCareerResponseSize {
		return nil, careerstate.ErrInvalid
	}
	return raw, nil
}
func ParseCareerResponse(raw []byte) (CareerResponse, error) {
	var r CareerResponse
	_, e := stableDecode(raw, MaxCareerResponseSize-1, 11, &r)
	if e != nil || !careerResponseValid(r) {
		return CareerResponse{}, careerstate.ErrInvalid
	}
	return r, nil
}
func handleCareerRequest(ctx context.Context, r CareerRequest, c CareerControllerAPI) CareerResponse {
	out := CareerResponse{ID: r.ID, JobID: r.JobID, OK: true}
	var e error
	if r.Command == "career_inventory" {
		p, x := c.CareerInventory(ctx, r.ViewID, r.JobID, r.After, r.Limit)
		e = x
		out.Page = &p
		out.StoreID = p.Source.Status.StoreID
	} else {
		p, x := c.CareerRoster(ctx, r.ViewID, r.JobID, r.FirstSlot, r.Limit)
		e = x
		out.Roster = &p
		out.StoreID = p.Source.Status.StoreID
	}
	if e != nil {
		return CareerResponse{ID: r.ID, Error: careerError(e)}
	}
	v, e := c.CareerView(r.ViewID)
	if e != nil {
		return CareerResponse{ID: r.ID, Error: "outcome_unknown"}
	}
	out.View = &v
	return out
}
func ServeCareers(ctx context.Context, c CareerControllerAPI, in io.Reader, out io.Writer) error {
	return serveResults(ctx, c, in, out, c)
}
