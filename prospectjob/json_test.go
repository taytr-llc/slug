// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectjob_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

func TestParseJobStrictSchema(t *testing.T) {
	f := fixtures(t)[0]
	raw, err := json.Marshal(f.Job)
	if err != nil {
		t.Fatal(err)
	}
	job, err := prospectjob.ParseJob(raw)
	if err != nil || job != f.Job {
		t.Fatalf("canonical fixture parse: %+v, %v", job, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for field := range fields {
		t.Run("missing "+field, func(t *testing.T) {
			var copy map[string]json.RawMessage
			_ = json.Unmarshal(raw, &copy)
			delete(copy, field)
			data, _ := json.Marshal(copy)
			assertParseRefused(t, data)
		})
		t.Run("null "+field, func(t *testing.T) {
			var copy map[string]json.RawMessage
			_ = json.Unmarshal(raw, &copy)
			copy[field] = json.RawMessage("null")
			data, _ := json.Marshal(copy)
			assertParseRefused(t, data)
		})
	}
	mutations := []struct {
		name string
		raw  string
	}{
		{"non-object", "[]"},
		{"null object", "null"},
		{"truncated object", string(raw[:len(raw)-1])},
		{"unknown field", `{"unknown":1,` + string(raw[1:])},
		{"case alias", strings.Replace(string(raw), `"version":`, `"Version":`, 1)},
		{"duplicate", `{"version":1,` + string(raw[1:])},
		{"escaped duplicate", `{"ver\u0073ion":1,` + string(raw[1:])},
		{"trailing object", string(raw) + `{}`},
		{"trailing garbage", string(raw) + `x`},
		{"numeric string", strings.Replace(string(raw), `"version":1`, `"version":"1"`, 1)},
		{"negative version", strings.Replace(string(raw), `"version":1`, `"version":-1`, 1)},
		{"overflow timestamp", strings.Replace(string(raw), `"issued_at_ms":1794787200000`, `"issued_at_ms":9223372036854775808`, 1)},
		{"fraction timestamp", strings.Replace(string(raw), `"issued_at_ms":1794787200000`, `"issued_at_ms":1794787200000.5`, 1)},
		{"wrong immutable ID", strings.Replace(string(raw), f.Job.ID, strings.Repeat("0", 64), 1)},
		{"uppercase immutable ID", strings.Replace(string(raw), f.Job.ID, strings.ToUpper(f.Job.ID), 1)},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) { assertParseRefused(t, []byte(mutation.raw)) })
	}
	// Whitespace and object field order are not part of the binary job identity.
	var reordered map[string]json.RawMessage
	_ = json.Unmarshal(raw, &reordered)
	pretty, _ := json.MarshalIndent(reordered, "", "  ")
	if got, err := prospectjob.ParseJob(pretty); err != nil || got != f.Job {
		t.Fatalf("JSON formatting changed the canonical job value: %+v, %v", got, err)
	}
	atLimit := append(append([]byte{}, raw...), bytes.Repeat([]byte(" "), prospectjob.MaxJobSize-len(raw))...)
	if got, err := prospectjob.ParseJob(atLimit); err != nil || got != f.Job {
		t.Fatal("exact document-size boundary rejected")
	}
	assertParseRefused(t, append(atLimit, ' '))
}

func assertParseRefused(t *testing.T, raw []byte) {
	t.Helper()
	if job, err := prospectjob.ParseJob(raw); err == nil || job != (prospectjob.Job{}) {
		t.Fatalf("malformed/noncanonical job returned %+v, %v", job, err)
	}
}

func TestParseJobStrictNestedContext(t *testing.T) {
	f := fixtures(t)[0]
	raw, _ := json.Marshal(f.Job)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	context := fields["context"]
	var contextFields map[string]json.RawMessage
	_ = json.Unmarshal(context, &contextFields)
	for name := range contextFields {
		t.Run("missing nested "+name, func(t *testing.T) {
			var copy map[string]json.RawMessage
			_ = json.Unmarshal(context, &copy)
			delete(copy, name)
			data, _ := json.Marshal(copy)
			fields["context"] = data
			job, _ := json.Marshal(fields)
			assertParseRefused(t, job)
		})
	}
	for _, nested := range []string{
		`{"version":1,` + string(context[1:]),
		`{"ver\u0073ion":1,` + string(context[1:]),
		`{"unknown":1,` + string(context[1:]),
		strings.Replace(string(context), `"version":1`, `"version":null`, 1),
		strings.Replace(string(context), `"version":1`, `"version":2`, 1),
	} {
		fields["context"] = json.RawMessage(nested)
		data, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		assertParseRefused(t, data)
	}
	// Stay under the whole-job limit while exceeding the context's own bound.
	paddedContext := append([]byte{'{'}, bytes.Repeat([]byte(" "), prospect.MaxContextSize+1-len(context))...)
	paddedContext = append(paddedContext, context[1:]...)
	oversized := bytes.Replace(raw, context, paddedContext, 1)
	if len(oversized) > prospectjob.MaxJobSize {
		t.Fatal("test must isolate the nested context size bound")
	}
	assertParseRefused(t, oversized)
}
