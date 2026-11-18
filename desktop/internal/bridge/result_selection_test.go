// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymresult"
)

func TestResultOldTerminalPreservesNewerSelectedCommitments(t *testing.T) {
	var old, fresh resultProtocolCase
	for _, c := range resultProtocolCases(t) {
		switch c.Name {
		case "protocol/fixture0/terminal-identical":
			old = c
		case "protocol/fixture0/command/result_record":
			fresh = c
		}
	}
	parse := func(raw json.RawMessage) pilot.ResultResponse {
		t.Helper()
		var buf bytes.Buffer
		if err := json.Compact(&buf, raw); err != nil {
			t.Fatal(err)
		}
		r, err := pilot.ParseResultResponse(buf.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	j, rr := parse(old.Response).Job, parse(fresh.Response).Record
	if old.View == nil || j == nil || j.Result == nil || rr == nil || rr.Record.Checkpoint == nil || rr.Record.Finality == nil || j.Result.Record.Checkpoint != nil || j.Result.Record.Finality != nil {
		t.Fatal("frozen old-terminal/new-read controls missing")
	}
	v := *old.View
	b := &StableBridge{result: &resultCorrelation{view: v, jobs: map[string]resultJobExpectation{}}}
	date, bid := j.Saturday, j.BoutID
	// A first or recovered terminal can establish the selected record.
	b.rememberResultSelection(v, j.Result.Record, true)
	if got := b.resultKnown(date, bid); got != resultCommitment(j.Result.Record) {
		t.Fatal("first terminal failed to establish selection")
	}
	// An explicit current read learns the first retained certificate/finality.
	b.rememberResultSelection(v, rr.Record, false)
	want := b.resultKnown(date, bid)
	if want.checkpoint == "" || want.finality == "" || !want.matches(rr.Record) {
		t.Fatal("current read did not retain commitments")
	}
	id, err := gymresult.ID(old.Pin)
	if err != nil {
		t.Fatal(err)
	}
	a := resultAuthority{pin: old.Pin, storeID: id, createdMS: old.Created, sourceCreatedMS: old.SourceCreated}
	e := resultJobExpectation{cardID: j.CardID, saturday: date, boutID: bid, kind: j.Kind, state: j.State, terminalDigest: resultJobDigest(*j)}
	if !b.correlateResultJob(old.Request, *j, e, a, v) {
		t.Fatal("unchanged retained terminal must still correlate")
	}
	b.rememberResultSelection(v, j.Result.Record, true)
	if b.resultKnown(date, bid) != want {
		t.Fatal("old terminal erased newer selected commitments")
	}
	// The same predicates used by the next native Record/Status response must
	// still reject disappearance, despite accepting the historical job above.
	if b.resultKnown(date, bid).matches(j.Result.Record) || b.resultKnown(date, bid).matchesStatus(j.Result.Status) {
		t.Fatal("later response could erase learned certificate/finality")
	}
	missing := rr.Record
	missing.Checkpoint = nil
	if b.resultKnown(date, bid).matches(missing) {
		t.Fatal("missing certificate accepted")
	}
	missing = rr.Record
	missing.Finality = nil
	if b.resultKnown(date, bid).matches(missing) {
		t.Fatal("missing finality accepted")
	}
	// A job with the complete extension remains useful to a selected old row.
	b.rememberResultSelection(v, j.Result.Record, false)
	b.rememberResultSelection(v, rr.Record, true)
	if b.resultKnown(date, bid) != want {
		t.Fatal("job could not extend same selected record")
	}
}
