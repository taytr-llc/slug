package pilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/gymentry"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"strings"
	"testing"
)

func TestEntryTransportAvoidsExpensiveCoreValidation(t *testing.T) {
	// A static dependency guard complements value conformance: core validation
	// regenerates SVG and must remain outside this polling transport call path.
	for _, name := range []string{"entry_validation.go", "entry_member_validation.go"} {
		f, e := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if e != nil {
			t.Fatal(e)
		}
		for _, i := range f.Imports {
			if strings.Contains(i.Path.Value, "fighterportrait") || strings.Contains(i.Path.Value, "fighterbout") || strings.Contains(i.Path.Value, "/prospect\"") {
				t.Fatal("expensive transport dependency", i.Path.Value)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			s, ok := c.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := s.X.(*ast.Ident)
			if !ok {
				return true
			}
			if id.Name == "gymentry" {
				switch s.Sel.Name {
				case "ValidateMutationResult", "ValidateOperationResult", "ValidateOperation", "ValidateReceipt", "ValidateCheckResult", "ReceiptDigest", "MarshalReceipt", "ParseReceipt":
					t.Error("transport entered core portrait validation", s.Sel.Name)
				}
			}
			return true
		})
	}
}
func TestEntryClosedRequestsAndUnavailable(t *testing.T) {
	id := strings.Repeat("1", 64)
	for _, r := range []EntryRequest{{ID: 1, Command: "entry_snapshot"}, {ID: 2, Command: "entry_upcoming", ViewID: id}, {ID: 3, Command: "entry_history", ViewID: id, Limit: 8}} {
		raw, e := MarshalEntryRequest(r)
		if e != nil {
			t.Fatal(e)
		}
		got, e := ParseEntryRequest(raw)
		if e != nil || got.Command != r.Command {
			t.Fatal(e)
		}
		if _, e = ParseStableRequest(raw); e == nil {
			t.Fatal("old stable grammar widened")
		}
		if _, e = ParsePreviewRequest(raw); e == nil {
			t.Fatal("old preview grammar widened")
		}
		bad := bytes.Replace(raw, []byte(`"id":`), []byte(`"id":1,"\u0069d":`), 1)
		if _, e = ParseEntryRequest(bad); e == nil {
			t.Fatal("decoded duplicate accepted")
		}
	}
	r := EntryResponse{ID: 1, OK: true, Snapshot: &EntrySnapshot{Version: 1, State: "unavailable"}}
	raw, e := MarshalEntryResponse(r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ParseEntryResponse(raw); e != nil {
		t.Fatal(e)
	}
	r.Snapshot.StoreID = id
	if _, e = MarshalEntryResponse(r); e == nil {
		t.Fatal("unavailable pins disclosed")
	}
}

type entryProtocolFixture struct{ previewProtocolController }

func (c *entryProtocolFixture) EntrySnapshot() EntrySnapshot {
	return EntrySnapshot{Version: 1, State: "unavailable"}
}
func (c *entryProtocolFixture) EntryUpcoming(context.Context, string) (gymentry.EventStatus, error) {
	return gymentry.EventStatus{}, gymentry.ErrUnavailable
}
func (c *entryProtocolFixture) EntryStatus(context.Context, string, string) (gymentry.EventStatus, error) {
	return gymentry.EventStatus{}, gymentry.ErrUnavailable
}
func (c *entryProtocolFixture) EntryHistory(context.Context, string, string, uint32) (gymentry.HistoryPage, error) {
	return gymentry.HistoryPage{}, gymentry.ErrUnavailable
}
func (c *entryProtocolFixture) BeginEntryPrepare(context.Context, string, gymentry.Request) (EntryJob, error) {
	return EntryJob{}, gymentry.ErrUnavailable
}
func (c *entryProtocolFixture) BeginEntryApply(context.Context, string, string) (EntryJob, error) {
	return EntryJob{}, gymentry.ErrUnavailable
}
func (c *entryProtocolFixture) BeginEntryCheck(context.Context, string, string) (EntryJob, error) {
	return EntryJob{}, gymentry.ErrUnavailable
}
func (c *entryProtocolFixture) EntryJob(context.Context, string, string) (EntryJob, error) {
	return EntryJob{}, gymentry.ErrNotFound
}
func (c *entryProtocolFixture) CancelEntryJob(context.Context, string, string) (EntryJob, error) {
	return EntryJob{}, gymentry.ErrNotFound
}
func (c *entryProtocolFixture) EntryOperation(context.Context, string, string) (gymentry.OperationResult, error) {
	return gymentry.OperationResult{}, gymentry.ErrNotFound
}
func (c *entryProtocolFixture) EntryPending(context.Context, string) (gymentry.OperationResult, error) {
	return gymentry.OperationResult{}, gymentry.ErrUnavailable
}
func (c *entryProtocolFixture) AbandonEntryOperation(context.Context, string, string) (gymentry.OperationResult, error) {
	return gymentry.OperationResult{}, gymentry.ErrNotFound
}
func TestEntryServePreservesAllFamiliesAndDrain(t *testing.T) {
	input, client := io.Pipe()
	responses, output := io.Pipe()
	c := &entryProtocolFixture{}
	done := make(chan error, 1)
	go func() { done <- ServeEntries(context.Background(), c, input, output) }()
	reader := bufio.NewReader(responses)
	send := func(s string) []byte {
		t.Helper()
		if _, e := io.WriteString(client, s+"\n"); e != nil {
			t.Fatal(e)
		}
		b, e := reader.ReadBytes('\n')
		if e != nil {
			t.Fatal(e)
		}
		return bytes.TrimSpace(b)
	}
	raw := send(`{"id":1,"command":"snapshot"}`)
	var old ManifestResponse
	if json.Unmarshal(raw, &old) != nil || !old.OK {
		t.Fatal("manifest")
	}
	raw = send(`{"id":2,"command":"stable_snapshot"}`)
	if _, e := ParseStableResponse(raw); e != nil {
		t.Fatal("stable", e)
	}
	raw = send(`{"id":3,"version":1,"command":"preview_snapshot"}`)
	if _, e := ParsePreviewResponse(raw); e != nil {
		t.Fatal("preview", e)
	}
	request, _ := MarshalEntryRequest(EntryRequest{ID: 4, Command: "entry_snapshot"})
	raw = send(string(request))
	if r, e := ParseEntryResponse(raw); e != nil || !r.OK || r.Snapshot.State != "unavailable" {
		t.Fatal("entry", e)
	}
	_ = client.Close()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	_ = responses.Close()
}
