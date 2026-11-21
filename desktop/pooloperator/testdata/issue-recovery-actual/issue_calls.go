package main

import (
	"context"
	"github.com/taytr-llc/slug/poolservice"
	"time"
)

// The new exact-ID protocol permits bounded resumption of the SAME durable
// request. This never chooses a latest job or creates/replaces a request ID.
func (v *scenario) exactIssue(lookup bool, label string) poolservice.IssueResponse {
	ctx, cancel := context.WithTimeout(v.ctx, 30*time.Second)
	defer cancel()
	refusals := []map[string]any{}
	defer func() {
		if len(refusals) > 0 {
			v.out[label+"_refusals"] = refusals
		}
	}()
	for {
		v.current.running()
		var result poolservice.IssueResponse
		var e error
		if lookup {
			result, e = v.owners.clients[0].LookupIssue(ctx, v.id)
		} else {
			result, e = v.owners.clients[0].ResumeIssue(ctx, v.id)
		}
		if e == nil {
			return result
		}
		failure, ok := e.(*poolservice.ClientError)
		if !ok || failure == nil || (failure.Code != "busy" && failure.Code != "rate_limited" && failure.Code != "unavailable" && failure.Code != "outcome_unknown") {
			check(e, label)
		}
		require(len(refusals) < 128, "bounded exact-ID refusals")
		refusals = append(refusals, map[string]any{"at_ms": time.Now().UnixMilli(), "request_id": v.id, "lookup": lookup, "code": failure.Code, "outcome_unknown": failure.OutcomeUnknown})
		check(pauseActual(ctx, 250*time.Millisecond), label+" original total deadline")
	}
}
