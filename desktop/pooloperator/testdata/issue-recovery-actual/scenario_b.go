package main

import (
	"github.com/taytr-llc/slug/poolservice"
	"reflect"
	"time"
)

func (v *scenario) runB(a, b *node) {
	phase = "real original unpublished job expiry"
	for time.Now().UnixMilli() < v.gap.Job.ExpiresAtMS {
		left := time.Until(time.UnixMilli(v.gap.Job.ExpiresAtMS))
		if left > time.Second {
			left = time.Second
		}
		check(pauseActual(v.ctx, left), "real expiry wait")
	}
	expiredAt := time.Now().UnixMilli()
	require(expiredAt >= v.gap.Job.ExpiresAtMS, "actual original expiry")
	v.serve("operator-serve-2")
	phase = "same authenticated request becomes issued-unused"
	// Readiness uses only bounded status; there is no replacement request ID.
	wait(v.ctx, 90*time.Second, func() bool { s, ok := v.current.status(); return ok && s.Public.Ready && s.Capacity.Mode == "enabled" })
	terminal := v.exactIssue(false, "original expired request resume")
	check(v.owners.clients[0].CheckIssueResponse(v.id, terminal), "original client terminal provenance")
	require(terminal.Result.State == "issued_unused" && terminal.Result.Work == nil && terminal.Result.JobID == v.gap.Job.ID && terminal.Result.IntentID == v.gap.Request.IntentID && terminal.Result.Reason == "expired_before_work" && terminal.Result.ResolvedAtMS >= v.gap.Job.ExpiresAtMS, "fixed original unused terminal")
	var replays []poolservice.IssueResponse
	for _, lookup := range []bool{true, false} {
		r := v.exactIssue(lookup, "terminal exact replay")
		check(v.owners.clients[0].CheckIssueResponse(v.id, r), "terminal original client provenance")
		require(sameIssue(r, terminal), "first terminal wins unchanged")
		replays = append(replays, r)
	}
	v.phase("04-expired-terminal", map[string]any{"expired_observed_at_ms": expiredAt, "original_expires_at_ms": v.gap.Job.ExpiresAtMS, "terminal": terminal, "replays": replays})
	check(v.current.stop(false, 0), "unused terminal operator joined")
	final := auditIssue(v.ctx, v.root, v.s, v.id)
	require(final.SourceRequests == 1 && final.IssuerRequests == 1 && final.Financial.AuthorityJobs == 1 && final.Financial.Works == 0 && final.Financial.Submissions == 0 && final.Financial.Accepted == 0 && final.Financial.Births == 0 && len(final.Financial.Rounds) == 0 && final.Financial.Header.Credits == 0 && final.Financial.Header.Batches == 0, "no work/share/birth/round/credit/payment")
	require(final.Request.State == "issued_unused" && final.Request.ResolvedAtMS == terminal.Result.ResolvedAtMS && final.Request.Reason == terminal.Result.Reason && final.Job == v.gap.Job && final.Issuer == v.gap.Issuer && reflect.DeepEqual(final.Template, v.gap.Template), "same original retained intent/job/template terminal")
	result, local := readRequest(v.in.AttachmentRoot, v.c, v.id)
	require(result == nil && local.ActiveRequestID == v.id && local.Requests == 1 && local.Records == 0, "stopped external worker never started a replacement request")
	v.phase("12-final-audit", map[string]any{"audit": final, "outbox": local, "local_terminal_consumption": "separate focused original worker control"})
	calls, attempts, e := v.owners.rpc.sends()
	check(e, "no B payment RPC")
	require(len(calls) == 0 && len(attempts) == 0, "zero B payment attempts")
	var height uint32
	a.rpc.must(v.ctx, "getblockcount", nil, &height)
	require(height == 2 && sameTip(v.ctx, a, b) != "", "no B mining")
	done := map[string]any{"completed": true, "case": "B", "request_id": v.id, "job_id": v.gap.Job.ID, "expiry_ms": v.gap.Job.ExpiresAtMS, "resumed_at_ms": terminal.Result.ResolvedAtMS, "state": terminal.Result.State, "works": 0, "submissions": 0, "accepted": 0, "births": 0, "credits": 0, "batches": 0, "send_count": 0, "final_height": height, "read_only_unchanged": final.ReadOnlyUnchanged}
	v.phase("13-complete", done)
	for k, x := range done {
		v.out[k] = x
	}
}
