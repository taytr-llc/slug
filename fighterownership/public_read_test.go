// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func publicReadEmptyStatus(t *testing.T, pin Pin) PublicReadStatus {
	t.Helper()
	id, e := ID(pin)
	if e != nil {
		t.Fatal(e)
	}
	return PublicReadStatus{Version: 1, PinID: id, Publication: registry.PublicationStatus{Version: 1, PinID: id, AdmissionHead: zeroID, PublicationHead: zeroID}, FinalityBodyHead: zeroID, FinalityHead: zeroID}
}
func TestPublicReadClosedProtocol(t *testing.T) {
	pin := chainReferencePin(t)
	v := publicReadEmptyStatus(t, pin)
	if e := validatePublicReadStatus(pin, v); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*PublicReadStatus){
		"body count":        func(v *PublicReadStatus) { v.FinalitySequence = 1 },
		"body head":         func(v *PublicReadStatus) { v.FinalityBodyHead = strings.Repeat("1", 64) },
		"frame count":       func(v *PublicReadStatus) { v.PublishedFinalitySequence = 1 },
		"frame head":        func(v *PublicReadStatus) { v.FinalityHead = strings.Repeat("1", 64) },
		"signed admissions": func(v *PublicReadStatus) { v.Publication.SignedAdmissionSequence = 1 },
		"pending hidden":    func(v *PublicReadStatus) { v.Publication.AcceptedSequence = 1 },
		"publication head":  func(v *PublicReadStatus) { v.Publication.PublicationHead = strings.Repeat("1", 64) },
		"pin replacement":   func(v *PublicReadStatus) { v.Publication.PinID = strings.Repeat("1", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			bad := v
			change(&bad)
			if validatePublicReadStatus(pin, bad) == nil {
				t.Fatal("invalid frontier accepted")
			}
		})
	}
	v.Publication.AcceptedSequence = 1
	v.Publication.PublicEvidencePending = true
	if e := validatePublicReadStatus(pin, v); e != nil {
		t.Fatal("declared admission gap", e)
	}
	v.FinalitySequence = 1
	v.FinalityBodyHead = strings.Repeat("1", 64)
	if e := validatePublicReadStatus(pin, v); e != nil {
		t.Fatal("declared signing gap", e)
	}
	request := publicReadRequest{Version: 1, PinID: v.PinID, ID: 1, Method: "status"}
	raw, _ := json.Marshal(request)
	want := `{"version":1,"pin_id":"` + v.PinID + `","id":1,"method":"status","sequence":0,"event_id":""}`
	if string(raw) != want {
		t.Fatal("request bytes differ from declared ordered protocol")
	}
	for _, bad := range []string{want + "\n", strings.Replace(want, `"id":1`, `"id":1,"id":1`, 1), strings.Replace(want, `"sequence":0`, `"sequence":0.0`, 1), strings.Replace(want, `"sequence":0`, `"sequence":0,"private":true`, 1)} {
		var r publicReadRequest
		if decode([]byte(bad), 1024, &r) == nil {
			t.Fatal("noncanonical request accepted")
		}
	}
	for _, r := range []publicReadRequest{
		{Version: 1, PinID: v.PinID, ID: 1, Method: "admission", Sequence: 1001},
		{Version: 1, PinID: v.PinID, ID: 1, Method: "publication", Sequence: 16385},
		{Version: 1, PinID: v.PinID, ID: 1, Method: "finality_frame", Sequence: 4097},
		{Version: 1, PinID: v.PinID, ID: 1, Method: "status", Sequence: 1},
		{Version: 1, PinID: v.PinID, ID: 1, Method: "cutoff", EventID: zeroID},
	} {
		if r.validate(v.PinID) == nil {
			t.Fatal("invalid selector")
		}
	}
}

func TestPublicReaderStrictResponses(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal-profile reader only")
	}
	pin := chainReferencePin(t)
	status := publicReadEmptyStatus(t, pin)
	for _, kind := range []string{"duplicate", "unknown", "wrong_id", "wrong_method", "wrong_pin", "inner_unknown", "nested_duplicate", "null", "error_result", "oversized", "compressed", "redirect", "body_head"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req publicReadRequest
				json.NewDecoder(r.Body).Decode(&req)
				b, _ := json.Marshal(status)
				v := publicReadResponse{Version: 1, PinID: status.PinID, ID: req.ID, Method: req.Method, Result: b}
				second := calls.Add(1) > 1
				if second {
					switch kind {
					case "wrong_id":
						v.ID++
					case "wrong_method":
						v.Method = "admission"
					case "wrong_pin":
						v.PinID = strings.Repeat("1", 64)
					case "null":
						v.Result = []byte("null")
					case "error_result":
						v.Error = "busy"
					case "body_head":
						x := status
						x.FinalitySequence = 1
						v.Result, _ = json.Marshal(x)
					}
				}
				raw, _ := json.Marshal(v)
				w.Header().Set("Content-Type", "application/json")
				if second {
					switch kind {
					case "duplicate":
						raw = bytes.Replace(raw, []byte(`"id":`), []byte(`"id":1,"id":`), 1)
					case "unknown":
						raw = append(raw[:len(raw)-1], []byte(`,"private":true}`)...)
					case "inner_unknown":
						raw = bytes.Replace(raw, []byte(`"publication":{`), []byte(`"publication":{"private":true,`), 1)
					case "nested_duplicate":
						raw = bytes.Replace(raw, []byte(`"accepted_sequence":0`), []byte(`"accepted_sequence":0,"accepted_sequence":0`), 1)
					case "oversized":
						raw = bytes.Repeat([]byte(" "), 4097)
					case "compressed":
						w.Header().Set("Content-Encoding", "gzip")
					case "redirect":
						w.Header().Set("Location", "/other")
						w.WriteHeader(302)
						return
					}
				}
				w.Write(raw)
			}))
			defer server.Close()
			r, e := NewPublicReader(context.Background(), pin, PublicReadConfig{URL: server.URL + publicReadPath}, context.Background())
			if e != nil {
				t.Fatal(e)
			}
			defer r.Close()
			if _, e = r.Status(context.Background()); e == nil {
				t.Fatal("malformed response accepted")
			}
			if calls.Load() != 2 {
				t.Fatal("automatic retry or redirect")
			}
		})
	}
}

func TestPublicReaderLifetimeAndCapacity(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal-profile reader only")
	}
	pin := chainReferencePin(t)
	status := publicReadEmptyStatus(t, pin)
	entered := make(chan struct{}, 8)
	cancelled := make(chan struct{}, 8)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req publicReadRequest
		json.NewDecoder(r.Body).Decode(&req)
		if calls.Add(1) > 1 {
			entered <- struct{}{}
			<-r.Context().Done()
			cancelled <- struct{}{}
			return
		}
		b, _ := json.Marshal(status)
		out, _ := json.Marshal(publicReadResponse{Version: 1, PinID: status.PinID, ID: req.ID, Method: req.Method, Result: b})
		w.Header().Set("Content-Type", "application/json")
		w.Write(out)
	}))
	defer server.Close()
	life, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, e := NewPublicReader(context.Background(), pin, PublicReadConfig{URL: server.URL + publicReadPath}, life)
	if e != nil {
		t.Fatal(e)
	}
	results := make(chan error, 8)
	for n := 0; n < 8; n++ {
		go func() { _, e := r.Status(context.Background()); results <- e }()
	}
	for n := 0; n < 8; n++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("missing admitted request")
		}
	}
	if _, e = r.Status(context.Background()); !errors.Is(e, ErrBusy) {
		t.Fatal("unbounded concurrency", e)
	}
	cancel()
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 8; n++ {
		select {
		case e := <-results:
			if e == nil {
				t.Fatal("closed original returned success")
			}
		case <-time.After(time.Second):
			t.Fatal("unjoined operation")
		}
		select {
		case <-cancelled:
		case <-time.After(time.Second):
			t.Fatal("HTTP cancellation not observed")
		}
	}
	if _, e = r.Status(context.Background()); !errors.Is(e, ErrClosed) {
		t.Fatal("replacement lifetime", e)
	}
}

func TestPublicReaderConfigAndProfile(t *testing.T) {
	pin := chainReferencePin(t)
	ctx := context.Background()
	for _, c := range []PublicReadConfig{
		{URL: "http://example.com" + publicReadPath},
		{URL: "http://localhost" + publicReadPath},
		{URL: "https://example.com" + publicReadPath},
		{URL: "http://127.0.0.1" + publicReadPath + "?private=true"},
		{URL: "http://user:secret@127.0.0.1" + publicReadPath},
		{URL: "http://127.0.0.1:0" + publicReadPath},
	} {
		r, e := NewPublicReader(ctx, pin, c, ctx)
		if e == nil {
			r.Close()
			t.Fatal("invalid config opened")
		}
		if wire.TestPoW && !errors.Is(e, ErrProfile) {
			t.Fatal("profile guard late", e)
		}
	}
}

func TestPublicReaderIndependentSignedRows(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal-profile reader only")
	}
	corpus, pins := coreReference(t)
	var frame FinalityFrame
	var admission AdmissionStatement
	var frameRaw, admissionRaw json.RawMessage
	for _, v := range corpus.Positives {
		if v.Family == "frame" && v.Name == "v1/zero256-unawarded/one" {
			frameRaw = append([]byte(nil), v.Value...)
			json.Unmarshal(v.Value, &frame)
		}
		if v.Family == "admission" && v.Name == "v1/zero256-unawarded" {
			admissionRaw = append([]byte(nil), v.Value...)
			json.Unmarshal(v.Value, &admission)
		}
	}
	pin := pins[frame.PinID]
	status := publicReadEmptyStatus(t, pin)
	var tamper atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req publicReadRequest
		json.NewDecoder(r.Body).Decode(&req)
		var b json.RawMessage
		switch req.Method {
		case "status":
			b, _ = json.Marshal(status)
		case "admission":
			b = append([]byte(nil), admissionRaw...)
		case "finality_frame":
			b = append([]byte(nil), frameRaw...)
		default:
			http.Error(w, "unexpected", 400)
			return
		}
		// Re-encoding only removes corpus formatting. Expected signatures and
		// object fields remain the independent Python producer's exact values.
		var compact bytes.Buffer
		json.Compact(&compact, b)
		b = compact.Bytes()
		if tamper.Load() {
			at := bytes.LastIndex(b, []byte(`"signature":"`)) + len(`"signature":"`)
			if b[at] == '0' {
				b[at] = '1'
			} else {
				b[at] = '0'
			}
		}
		out, _ := json.Marshal(publicReadResponse{Version: 1, PinID: status.PinID, ID: req.ID, Method: req.Method, Result: b})
		w.Header().Set("Content-Type", "application/json")
		w.Write(out)
	}))
	defer server.Close()
	r, e := NewPublicReader(context.Background(), pin, PublicReadConfig{URL: server.URL + publicReadPath}, context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	f, e := r.FinalityFrame(context.Background(), frame.Sequence)
	if e != nil || f != frame {
		t.Fatal("independent frame", e)
	}
	a, e := r.Admission(context.Background(), admission.AdmissionSequence)
	if e != nil || a.RecordDigest != admission.RecordDigest || a.Signature != admission.Signature {
		t.Fatal("independent admission", e)
	}
	tamper.Store(true)
	if _, e = r.FinalityFrame(context.Background(), frame.Sequence); e == nil {
		t.Fatal("forged frame accepted")
	}
	if _, e = r.Admission(context.Background(), admission.AdmissionSequence); e == nil {
		t.Fatal("forged admission accepted")
	}
}

// This helper sends exact HTTP bytes so server refusal tests cannot silently
// normalize malformed requests through the production client's encoder.
func publicReadHTTP(t *testing.T, url, method, raw string) (int, []byte) {
	t.Helper()
	req, e := http.NewRequest(method, url, strings.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 2 * time.Second}
	r, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	b, e := io.ReadAll(io.LimitReader(r.Body, 10000))
	if e != nil {
		t.Fatal(e)
	}
	return r.StatusCode, b
}
