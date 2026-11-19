// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"github.com/taytr-llc/slug/gymentry"
	"reflect"
	"time"
)

func entryErrorCode(s string) bool {
	if careerActionCode(s) {
		return true
	}
	switch s {
	case "invalid_request", "unauthorized", "forbidden", "stale_view", "busy", "entry_unavailable", "entry_corrupt", "entry_capacity", "entry_clock", "entry_calendar", "entry_closed", "entry_conflict", "entry_pending", "entry_not_found", "entry_sync_required", "entry_not_rostered", "entry_retired", "entry_class_conflict", "entry_cancelled", "outcome_unknown":
		return true
	}
	return false
}
func entryRequestValid(r EntryRequest) bool {
	if r.ID == 0 || r.ID > gymentry.MaxSafeInteger {
		return false
	}
	allowed := map[string]bool{"id": true, "command": true}
	add := func(names ...string) {
		for _, n := range names {
			allowed[n] = true
		}
	}
	switch r.Command {
	case "entry_snapshot":
	case "entry_upcoming", "entry_pending":
		add("view_id")
	case "entry_status", "entry_check":
		add("view_id", "saturday")
	case "entry_history":
		add("view_id", "after_event_id", "limit")
	case "entry_prepare":
		add("view_id", "request")
	case "entry_apply", "entry_operation", "entry_abandon":
		add("view_id", "operation_id")
	case "entry_job", "entry_cancel":
		add("view_id", "job_id")
	default:
		return false
	}
	v := reflect.ValueOf(r)
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if !allowed[t.Field(i).Tag.Get("json")] && !v.Field(i).IsZero() {
			return false
		}
	}
	if r.Command != "entry_snapshot" && !canonicalManifestID(r.ViewID) {
		return false
	}
	if allowed["operation_id"] && !canonicalManifestID(r.OperationID) || allowed["job_id"] && !canonicalManifestID(r.JobID) {
		return false
	}
	if allowed["saturday"] && !entryDateValid(r.Saturday) {
		return false
	}
	if r.Command == "entry_history" && (r.Limit < 1 || r.Limit > 8 || r.AfterEventID != "" && !canonicalManifestID(r.AfterEventID)) {
		return false
	}
	if r.Command == "entry_prepare" {
		if r.Request == nil {
			return false
		}
		_, e := gymentry.ValidateRequest(*r.Request)
		return e == nil
	}
	return true
}
func entryDateValid(s string) bool {
	if len(s) != 10 {
		return false
	}
	d, e := time.Parse("2006-01-02", s)
	return e == nil && d.Format("2006-01-02") == s && d.Year() >= 2026 && d.Year() <= 2099 && d.Weekday() == time.Saturday
}
func entryEventMatches(v ManifestViewPin, e gymentry.Event) bool {
	n, err := gymentry.NamespaceID(v.AuthorityPin.Network, v.AuthorityPin.Genesis, v.AccountAuthorityID)
	return err == nil && e.NamespaceID == n && e.CalendarID == gymentry.CalendarID() && e.RulesID == gymentry.RulesID()
}
func entrySnapshotValid(s EntrySnapshot) bool {
	if s.Version != 1 {
		return false
	}
	if s.State == "unavailable" {
		return s.View == nil && s.StoreID == "" && s.PendingOperationID == "" && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == "" && (s.Error == "" || entryErrorCode(s.Error))
	}
	if s.View == nil || !stableViewValid(*s.View) || !canonicalManifestID(s.StoreID) || (s.State != "idle" && s.State != "busy" && s.State != "failed") || s.Error != "" && !entryErrorCode(s.Error) {
		return false
	}
	for _, id := range []string{s.PendingOperationID, s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" && !canonicalManifestID(id) {
			return false
		}
	}
	return s.ActiveJobID == "" || s.ActiveJobID != s.QueuedJobID
}
func entryJobValid(j *EntryJob) bool {
	if j == nil || !careerIneligibleValid(j.Error, j.IneligibleProspectIDs) {
		return false
	}

	if j.Version != 1 || !canonicalManifestID(j.JobID) || !stableViewValid(j.View) || !canonicalManifestID(j.StoreID) || !canonicalManifestID(j.EventID) || !entryDateValid(j.Saturday) {
		return false
	}
	if j.OperationID != "" && !canonicalManifestID(j.OperationID) || j.Error != "" && !entryErrorCode(j.Error) {
		return false
	}
	if j.Kind != "prepare" && j.Kind != "apply" && j.Kind != "check" {
		return false
	}
	if j.Kind == "apply" && j.OperationID == "" || j.Kind == "check" && j.OperationID != "" {
		return false
	}
	switch j.State {
	case "queued", "running":
		if j.Error != "" {
			return false
		}
	case "failed", "cancelled":
		if j.Error == "" {
			return false
		}
	case "complete":
		if j.Error != "" {
			return false
		}
	default:
		return false
	}
	if j.State != "complete" {
		return j.Intent == nil && j.Mutation == nil && j.Check == nil
	}
	switch j.Kind {
	case "prepare":
		if j.Intent == nil || j.Mutation != nil || j.Check != nil {
			return false
		}
		v, e := gymentry.ValidateIntent(*j.Intent)
		if e != nil {
			return false
		}
		j.Intent = &v
		return v.OperationID == j.OperationID && v.ViewID == j.View.ViewID && v.Generation == j.View.Generation && v.Request.EventID == j.EventID && v.Request.Saturday == j.Saturday
	case "apply":
		if j.Mutation == nil || j.Intent != nil || j.Check != nil {
			return false
		}
		v := *j.Mutation
		if v.Version != 1 || !entryReceiptShape(v.Receipt, j.View.AuthorityPin.DescriptorID) {
			return false
		}
		j.Mutation = &v
		r := v.Receipt
		return r.StoreID == j.StoreID && r.OperationID == j.OperationID && r.ViewID == j.View.ViewID && r.Generation == j.View.Generation && r.Event.EventID == j.EventID && r.Event.Saturday == j.Saturday && entryEventMatches(j.View, r.Event)
	case "check":
		if j.Check == nil || j.Intent != nil || j.Mutation != nil {
			return false
		}
		v := *j.Check
		if !entryCheckShape(v, j.View.AuthorityPin.DescriptorID) {
			return false
		}
		j.Check = &v
		return v.Status.StoreID == j.StoreID && v.Status.Event.EventID == j.EventID && v.Status.Event.Saturday == j.Saturday && entryEventMatches(j.View, v.Status.Event)
	}
	return false
}
func entryResponseValid(r *EntryResponse) bool {
	if r.ID == 0 || r.ID > gymentry.MaxSafeInteger {
		return false
	}
	count := 0
	for _, b := range []bool{r.Snapshot != nil, r.Status != nil, r.History != nil, r.Job != nil, r.Operation != nil} {
		if b {
			count++
		}
	}
	if !r.OK {
		return count == 0 && r.View == nil && r.StoreID == "" && entryErrorCode(r.Error)
	}
	if count != 1 || r.Error != "" {
		return false
	}
	if r.Snapshot != nil && r.Snapshot.State == "unavailable" {
		return r.View == nil && r.StoreID == "" && entrySnapshotValid(*r.Snapshot)
	}
	if r.View == nil || !stableViewValid(*r.View) || !canonicalManifestID(r.StoreID) {
		return false
	}
	switch {
	case r.Snapshot != nil:
		return entrySnapshotValid(*r.Snapshot) && r.Snapshot.View != nil && *r.Snapshot.View == *r.View && r.Snapshot.StoreID == r.StoreID
	case r.Job != nil:
		return entryJobValid(r.Job) && r.Job.View == *r.View && r.Job.StoreID == r.StoreID
	case r.Status != nil:
		v, e := gymentry.ValidateEventStatus(*r.Status)
		if e != nil {
			return false
		}
		r.Status = &v
		return v.StoreID == r.StoreID && entryEventMatches(*r.View, v.Event)
	case r.History != nil:
		v, e := gymentry.ValidateHistoryPage(*r.History)
		if e != nil || v.StoreID != r.StoreID {
			return false
		}
		r.History = &v
		for _, s := range v.Events {
			if !entryEventMatches(*r.View, s.Event) {
				return false
			}
		}
		return true
	case r.Operation != nil:
		v := *r.Operation
		if !entryOperationShape(v, r.View.AuthorityPin.DescriptorID) || v.StoreID != r.StoreID {
			return false
		}
		r.Operation = &v
		return v.Operation == nil || v.Operation.Receipt == nil || entryEventMatches(*r.View, v.Operation.Receipt.Event)
	}
	return false
}
