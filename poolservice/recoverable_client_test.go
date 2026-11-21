// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/wire/v2"
	"io"
	"net/http"
	"strings"
	"testing"
)

func recoveryNamed(t *testing.T, ref recoveryReference, name string) recoveryCase {
	t.Helper()
	for _, r := range ref.States.Responses {
		if r.Name == name {
			return r
		}
	}
	t.Fatal("missing reference", name)
	return recoveryCase{}
}
func TestRecoverableClientOriginalResponseProvenance(t *testing.T) {
	ref := recoveryReferenceRead(t)
	c := recoveryResponseClient(t, ref)
	other := recoveryResponseClient(t, ref)
	request := ref.Fixtures.Matching[0].RequestID
	calls := 0
	raw := recoveryRaw(t, recoveryNamed(t, ref, "resume-work"))
	c.http.Transport = clientTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body, e := io.ReadAll(r.Body)
		expected, _ := json.Marshal(issueRequest{request})
		if e != nil || !bytes.Equal(body, expected) || r.URL.Path != recoverablePathPrefix+"issue" || r.Header.Get("Authorization") != "Bearer "+c.config.Token {
			t.Error("exact original request changed")
		}
		return dailyHTTPRaw(raw, 200), nil
	})
	response, e := c.ResumeIssue(context.Background(), request)
	if e != nil || calls != 1 || c.CheckIssueResponse(request, response) != nil {
		t.Fatal("original response", e, calls)
	}
	if other.CheckIssueResponse(request, response) == nil || c.CheckIssueResponse(strings.Repeat("2", 64), response) == nil {
		t.Fatal("foreign client/request provenance")
	}
	serialized, e := json.Marshal(response)
	if e != nil || !bytes.Equal(serialized, raw) {
		t.Fatal("provenance leaked into bytes")
	}
	var decoded IssueResponse
	if json.Unmarshal(serialized, &decoded) != nil || c.CheckIssueResponse(request, decoded) == nil {
		t.Fatal("decoded value manufactured receipt")
	}
	changed := response
	changed.Result.Reason = "parent_changed"
	if c.CheckIssueResponse(request, changed) == nil {
		t.Fatal("mutated terminal fields")
	}
	// The returned Work is caller-owned; changing any nested field invalidates
	// its immutable receipt, even when the outer value still holds that receipt.
	response.Result.Work.Template.Context.ParentHeight++
	if c.CheckIssueResponse(request, response) == nil {
		t.Fatal("nested mutation survived digest")
	}
	if _, e = c.Issue(context.Background()); e == nil || calls != 1 {
		t.Fatal("selected owner invoked legacy issuance")
	}
}
func TestRecoverableClientExactRequestUnknownAndRead(t *testing.T) {
	ref := recoveryReferenceRead(t)
	c := recoveryResponseClient(t, ref)
	id := ref.Fixtures.Matching[0].RequestID
	requests := []string{}
	var mode string
	c.http.Transport = clientTransport(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.Path)
		var q issueRequest
		if json.NewDecoder(r.Body).Decode(&q) != nil || q.RequestID != id {
			t.Error("request changed")
		}
		switch mode {
		case "lost":
			return nil, io.ErrUnexpectedEOF
		case "wrong-money":
			var v IssueResponse
			_ = json.Unmarshal(recoveryNamed(t, ref, "resume-work").Value, &v)
			v.RecipientScript = "0014" + strings.Repeat("e", 40)
			return clientResponse(t, 200, v), nil
		case "absent":
			return dailyHTTPRaw(recoveryRaw(t, recoveryNamed(t, ref, "lookup-absent")), 200), nil
		default:
			return dailyHTTPRaw(recoveryRaw(t, recoveryNamed(t, ref, "resume-issued_unused")), 200), nil
		}
	})
	mode = "lost"
	_, e := c.ResumeIssue(context.Background(), id)
	clientError(t, e, "outcome_unknown", true)
	if len(requests) != 1 {
		t.Fatal("unknown mutation retried")
	}
	mode = "wrong-money"
	_, e = c.ResumeIssue(context.Background(), id)
	clientError(t, e, "outcome_unknown", true)
	if len(requests) != 2 {
		t.Fatal("invalid success retried")
	}
	mode = "absent"
	v, e := c.LookupIssue(context.Background(), id)
	if e != nil || v.Result.State != "absent" || c.CheckIssueResponse(id, v) != nil || requests[2] != recoverablePathPrefix+"issue-lookup" {
		t.Fatal("exact read", e)
	}
	_, e = c.ResumeIssue(context.Background(), id)
	clientError(t, e, "outcome_unknown", true)
	mode = "terminal"
	v, e = c.ResumeIssue(context.Background(), id)
	if e != nil || v.Result.State != "issued_unused" || c.CheckIssueResponse(id, v) != nil {
		t.Fatal("authenticated terminal", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := len(requests)
	_, e = c.ResumeIssue(ctx, id)
	if e == nil || len(requests) != before {
		t.Fatal("pre-canceled request escaped")
	}
	// Observe caller cancellation after a successful transport response; no valid
	// receipt may be published from that canceled operation.
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) {
		cancel()
		return dailyHTTPRaw(recoveryRaw(t, recoveryNamed(t, ref, "resume-work")), 200), nil
	})
	_, e = c.ResumeIssue(ctx, id)
	clientError(t, e, "outcome_unknown", true)
}
func TestRecoverableTransportCheapRefuses(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile refusal")
	}
	ref := recoveryReferenceRead(t)
	if c, e := NewRecoverableDailyClient(ref.Fixtures.Conditional[0].Client); e == nil || c != nil {
		t.Fatal("cheap client constructed")
	}
	if s, e := NewRecoverableDailyServer(context.Background(), ServerConfig{Version: 3, Listen: "127.0.0.1:0"}, nil, nil, nil, ref.Fixtures.Conditional[0].Client.IssueProfile); s != nil || e == nil {
		t.Fatal("cheap server constructed")
	}
	var c *Client
	if _, e := c.ResumeIssue(context.Background(), strings.Repeat("1", 64)); e == nil {
		t.Fatal("nil client")
	}
}
