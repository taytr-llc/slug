// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"
)

const smallReply = 64 << 10

type nodeRPC struct {
	endpoint, user, password string
	client                   *http.Client
	nextID                   uint64 // Publisher serializes all operations.
}

func newRPC(c ownedConfig) *nodeRPC {
	t := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		DisableKeepAlives: true, DisableCompression: true, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 8192}
	return &nodeRPC{endpoint: "http://" + c.rpc, user: c.user, password: c.password,
		client: &http.Client{Transport: t, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// uniqueJSON rejects duplicate decoded keys at every depth before typed reads.
// Results may contain additional node fields; the RPC envelope is closed below.
func uniqueJSON(raw []byte) error {
	if !utf8.Valid(raw) {
		return ErrUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 16 {
			return ErrUnavailable
		}
		t, err := d.Token()
		if err != nil {
			return ErrUnavailable
		}
		v, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch v {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				name, ok := k.(string)
				if e != nil || !ok || len(name) > 64 || seen[name] {
					return ErrUnavailable
				}
				seen[name] = true
				if e = value(depth + 1); e != nil {
					return e
				}
			}
			t, err = d.Token()
			if err != nil || t != json.Delim('}') {
				return ErrUnavailable
			}
		case '[':
			for d.More() {
				if e := value(depth + 1); e != nil {
					return e
				}
			}
			t, err = d.Token()
			if err != nil || t != json.Delim(']') {
				return ErrUnavailable
			}
		default:
			return ErrUnavailable
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrUnavailable
	}
	return nil
}

func fields(raw []byte, required ...string) (map[string]json.RawMessage, error) {
	if uniqueJSON(raw) != nil {
		return nil, ErrUnavailable
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return nil, ErrUnavailable
	}
	for _, k := range required {
		if len(m[k]) == 0 || bytes.Equal(bytes.TrimSpace(m[k]), []byte("null")) {
			return nil, ErrUnavailable
		}
	}
	return m, nil
}

func (r *nodeRPC) call(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	limit := smallReply
	switch method {
	case "getcurrentnet", "getblockchaininfo", "getblockhash", "getbestblock", "getblockheader", "gettxout", "getpeerinfo", "sendrawtransaction":
	case "getblock":
		limit = MaxBlockSize*2 + 8192
	default:
		return nil, ErrInvalid
	}
	r.nextID++
	request, _ := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      uint64 `json:"id"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
	}{"1.0", r.nextID, method, params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(request))
	if err != nil {
		return nil, ErrUnavailable
	}
	// No GetBody means even the transport cannot replay a submitted body.
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(r.user, r.password)
	response, err := r.client.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "" || response.ContentLength > int64(limit) {
		return nil, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil || len(raw) > limit {
		return nil, ErrUnavailable
	}
	m, err := fields(raw, "jsonrpc", "id")
	if err != nil || len(m) != 4 || len(m["result"]) == 0 || !bytes.Equal(bytes.TrimSpace(m["error"]), []byte("null")) {
		return nil, ErrUnavailable
	}
	var version string
	if json.Unmarshal(m["jsonrpc"], &version) != nil || version != "1.0" || string(m["id"]) != strconv.FormatUint(r.nextID, 10) {
		return nil, ErrUnavailable
	}
	return m["result"], nil
}
