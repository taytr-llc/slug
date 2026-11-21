// Copyright (c) 2026 The SLUG developers. ISC license.
package poolingress

import (
	"encoding/hex"
	"mime"
	"net/http"
	"strings"
	"time"
)

type route struct {
	upstream, requestLimit, responseLimit int
	timeout                               time.Duration
}

func routeFor(path string) (route, bool) { return selectedRouteFor(path, 1) }
func selectedRouteFor(path string, version uint32) (route, bool) {
	if version != 1 && version != 2 && version != 3 {
		return route{}, false
	}
	prefix := "/pool/v1/"
	if version == 2 {
		prefix = "/pool/v2/"
	}
	if version == 3 {
		prefix = "/pool/v3/"
	}
	r := route{requestLimit: 1024, responseLimit: 65536, timeout: 35 * time.Second}
	switch path {
	case prefix + "issue", prefix + "work":
		r.responseLimit = 16842752
	case prefix + "issue-lookup":
		if version != 3 {
			return route{}, false
		}
		r.responseLimit = 16842752
	case prefix + "submit", prefix + "lookup", prefix + "balance":
	case prefix + "payouts":
		r.responseLimit = 131072
	case "/v3/admissions":
		r.upstream = 1
		r.requestLimit = 65536
		r.responseLimit = 73728
	case "/slug/ownership/v2":
		r.upstream = 2
		r.responseLimit = 73728
		r.timeout = 5 * time.Second
	default:
		return route{}, false
	}
	return r, true
}
func jsonMedia(values []string) bool {
	if len(values) != 1 {
		return false
	}
	kind, params, e := mime.ParseMediaType(values[0])
	if e != nil || kind != "application/json" {
		return false
	}
	for k, v := range params {
		if k != "charset" || !strings.EqualFold(v, "utf-8") {
			return false
		}
	}
	return true
}
func bearer(values []string) bool {
	if len(values) != 1 || len(values[0]) != 71 || !strings.HasPrefix(values[0], "Bearer ") {
		return false
	}
	raw, e := hex.DecodeString(values[0][7:])
	return e == nil && len(raw) == 32
}
func requestRoute(r *http.Request, host string) (route, bool) {
	return requestSelectedRoute(r, host, 1)
}
func requestSelectedRoute(r *http.Request, host string, version uint32) (route, bool) {
	if r == nil || r.URL == nil || r.Host != host || r.Method != http.MethodPost || r.URL.IsAbs() || r.URL.Host != "" || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" || r.URL.Opaque != "" || r.ProtoMajor != 1 || r.ProtoMinor != 1 || len(r.TransferEncoding) != 0 || len(r.Trailer) != 0 {
		return route{}, false
	}
	selected, ok := selectedRouteFor(r.URL.Path, version)
	if !ok || r.RequestURI != "" && r.RequestURI != r.URL.Path {
		return route{}, false
	}
	if len(r.Header.Values("Content-Length")) > 1 || !jsonMedia(r.Header.Values("Content-Type")) || r.ContentLength < 0 || r.ContentLength > int64(selected.requestLimit) {
		return route{}, false
	}
	for k := range r.Header {
		k = strings.ToLower(k)
		if k == "origin" || k == "forwarded" || strings.HasPrefix(k, "x-forwarded-") || k == "x-real-ip" || k == "proxy-authorization" || k == "proxy-connection" || k == "upgrade" || k == "expect" || k == "trailer" || k == "te" || k == "transfer-encoding" || k == "content-encoding" {
			return route{}, false
		}
	}
	if values := r.Header.Values("Connection"); len(values) > 0 && (len(values) != 1 || !strings.EqualFold(values[0], "close")) {
		return route{}, false
	}
	if selected.upstream == 2 {
		if len(r.Header.Values("Authorization")) != 0 || r.Header.Get("Content-Type") != "application/json" {
			return route{}, false
		}
	} else if !bearer(r.Header.Values("Authorization")) {
		return route{}, false
	}
	return selected, true
}
