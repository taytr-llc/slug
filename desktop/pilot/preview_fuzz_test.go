// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"testing"
)

func FuzzPreviewClosedCodecs(f *testing.F) {
	f.Add([]byte(`{"id":1,"version":1,"command":"preview_snapshot"}`))
	f.Add([]byte(`{"id":1,"version":1,"ok":true,"snapshot":{"version":1,"state":"unavailable","error":"","view":null,"store_id":"","engine_version":0,"engine_id":"","active_job_id":"","queued_job_id":"","last_job_id":""}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > MaxResponseSize+1 {
			return
		}
		if r, e := ParsePreviewRequest(raw); e == nil {
			encoded, e := MarshalPreviewRequest(r)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = ParsePreviewRequest(encoded); e != nil {
				t.Fatal(e)
			}
		}
		if r, e := ParsePreviewResponse(raw); e == nil {
			encoded, e := MarshalPreviewResponse(r)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = ParsePreviewResponse(encoded); e != nil {
				t.Fatal(e)
			}
		}
	})
}
