package pilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/taytr-llc/slug/poolsettle"
)

type walletPayoutTransaction struct {
	txid          string
	confirmations int64
	blockHash     string
	raw           []byte
}

// payoutTransaction is deliberately method-specific. The generic legacy call
// erases RPC error codes, so it cannot distinguish absence from an RPC failure.
func (r *walletHTTPClient) payoutTransaction(ctx context.Context, txid string) (walletPayoutTransaction, bool, error) {
	bad := func() (walletPayoutTransaction, bool, error) {
		if ctx != nil && ctx.Err() != nil {
			return walletPayoutTransaction{}, false, ctx.Err()
		}
		return walletPayoutTransaction{}, false, errWalletPayoutUnavailable
	}
	if r == nil || ctx == nil || r.client == nil || !payoutLowerHex(txid, 64, false) {
		return bad()
	}
	u, err := url.Parse(r.url)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.RawPath != "" || u.Path != "" && u.Path != "/" {
		return bad()
	}
	ip := net.ParseIP(u.Hostname())
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || ip == nil || !ip.IsLoopback() {
		return bad()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return bad()
	}
	r.mu.Lock()
	if r.nextID == math.MaxUint64 {
		r.mu.Unlock()
		return bad()
	}
	r.nextID++
	id := r.nextID
	if r.calls == nil {
		r.calls = make(map[string]uint64)
	}
	r.calls["gettransaction"]++
	r.mu.Unlock()
	body, err := json.Marshal(struct {
		JSONRPC string   `json:"jsonrpc"`
		ID      uint64   `json:"id"`
		Method  string   `json:"method"`
		Params  []string `json:"params"`
	}{"1.0", id, "gettransaction", []string{txid}})
	if err != nil {
		return bad()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return bad()
	}
	req.SetBasicAuth(r.user, r.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	client := *r.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return bad()
	}
	defer resp.Body.Close()
	media, params, mediaErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || resp.Uncompressed || resp.Header.Get("Content-Encoding") != "" ||
		mediaErr != nil || media != "application/json" || len(params) != 0 {
		return bad()
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxWalletPayoutResponse+1))
	if err != nil || len(raw) > maxWalletPayoutResponse || ctx.Err() != nil {
		return bad()
	}
	tx, missing, ok := decodeWalletPayoutRPC(raw, id)
	if !ok || !missing && tx.txid != txid {
		return bad()
	}
	return tx, missing, nil
}

// These scanners retain slices into one bounded response. They do not decode
// attacker-controlled maps/slices before checking the allowed shape and count.
func payoutSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\r' || b == '\n' }
func payoutSkipSpace(raw []byte, p int) int {
	for p < len(raw) && payoutSpace(raw[p]) {
		p++
	}
	return p
}
func payoutValueEnd(raw []byte, p, depth int) (int, bool) {
	if p >= len(raw) || depth > 5 {
		return p, false
	}
	switch raw[p] {
	case '"':
		for p++; p < len(raw); p++ {
			if raw[p] == '\\' {
				p++
			} else if raw[p] == '"' {
				return p + 1, true
			}
		}
		return p, false
	case '{', '[':
		close := byte('}')
		if raw[p] == '[' {
			close = ']'
		}
		for p++; p < len(raw); {
			if raw[p] == close {
				return p + 1, true
			}
			if payoutSpace(raw[p]) || raw[p] == ',' || raw[p] == ':' {
				p++
				continue
			}
			var ok bool
			p, ok = payoutValueEnd(raw, p, depth+1)
			if !ok {
				return p, false
			}
		}
		return p, false
	default:
		start := p
		for p < len(raw) && !payoutSpace(raw[p]) && raw[p] != ',' && raw[p] != '}' && raw[p] != ']' {
			p++
		}
		return p, p > start
	}
}

func payoutString(raw []byte, limit int) (string, bool) {
	if len(raw) < 2 || len(raw) > 6*limit+2 || raw[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil || len(s) > limit {
		return "", false
	}
	return s, true
}

func payoutObject(raw []byte, required, optional string) (map[string][]byte, bool) {
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		return nil, false
	}
	fields := make(map[string][]byte)
	allowed := strings.Fields(required + " " + optional)
	for p := payoutSkipSpace(raw, 1); p < len(raw)-1; {
		end, ok := payoutValueEnd(raw, p, 0)
		if !ok {
			return nil, false
		}
		key, ok := payoutString(raw[p:end], 32)
		known := false
		for _, name := range allowed {
			known = known || name == key
		}
		if !ok || !known || fields[key] != nil {
			return nil, false
		}
		p = payoutSkipSpace(raw, end)
		if p >= len(raw)-1 || raw[p] != ':' {
			return nil, false
		}
		p = payoutSkipSpace(raw, p+1)
		end, ok = payoutValueEnd(raw, p, 0)
		if !ok {
			return nil, false
		}
		fields[key] = raw[p:end]
		p = payoutSkipSpace(raw, end)
		if p < len(raw)-1 {
			if raw[p] != ',' {
				return nil, false
			}
			p = payoutSkipSpace(raw, p+1)
		}
	}
	for _, name := range strings.Fields(required) {
		if fields[name] == nil {
			return nil, false
		}
	}
	return fields, true
}

func payoutArray(raw []byte, limit int, validate func([]byte) bool) bool {
	if len(raw) < 2 || raw[0] != '[' || raw[len(raw)-1] != ']' {
		return false
	}
	count := 0
	for p := payoutSkipSpace(raw, 1); p < len(raw)-1; {
		count++
		if count > limit {
			return false
		}
		end, ok := payoutValueEnd(raw, p, 0)
		if !ok || !validate(raw[p:end]) {
			return false
		}
		p = payoutSkipSpace(raw, end)
		if p < len(raw)-1 {
			if raw[p] != ',' {
				return false
			}
			p = payoutSkipSpace(raw, p+1)
		}
	}
	return true
}

func payoutInteger(raw []byte) (int64, bool) {
	if len(raw) == 0 || len(raw) > 20 {
		return 0, false
	}
	i, err := strconv.ParseInt(string(raw), 10, 64)
	return i, err == nil && strconv.FormatInt(i, 10) == string(raw)
}
func payoutFloat(raw []byte) bool {
	if len(raw) == 0 || raw[0] == '"' {
		return false
	}
	v, err := strconv.ParseFloat(string(raw), 64)
	return err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
}

func payoutDetail(raw []byte) bool {
	f, ok := payoutObject(raw, "account amount category vout", "address involveswatchonly fee")
	if !ok || !payoutFloat(f["amount"]) || f["fee"] != nil && !payoutFloat(f["fee"]) {
		return false
	}
	for _, field := range []string{"account", "category", "address"} {
		if field == "address" && f[field] == nil {
			continue
		}
		if _, ok := payoutString(f[field], 256); !ok {
			return false
		}
	}
	if b := f["involveswatchonly"]; b != nil && string(b) != "true" && string(b) != "false" {
		return false
	}
	v, ok := payoutInteger(f["vout"])
	return ok && v >= 0 && v <= math.MaxUint32
}

func decodeWalletPayoutRPC(raw []byte, id uint64) (walletPayoutTransaction, bool, bool) {
	zero := walletPayoutTransaction{}
	if len(raw) == 0 || len(raw) > maxWalletPayoutResponse || !utf8.Valid(raw) || !json.Valid(raw) {
		return zero, false, false
	}
	raw = bytes.Trim(raw, " \r\n\t")
	if _, ok := payoutValueEnd(raw, 0, 0); !ok {
		return zero, false, false
	}
	f, ok := payoutObject(raw, "jsonrpc result error id", "")
	if !ok || string(f["jsonrpc"]) != `"1.0"` || string(f["id"]) != strconv.FormatUint(id, 10) {
		return zero, false, false
	}
	if string(f["error"]) != "null" {
		e, valid := payoutObject(f["error"], "code message", "")
		if !valid || string(f["result"]) != "null" || string(e["code"]) != "-5" {
			return zero, false, false
		}
		message, valid := payoutString(e["message"], 64)
		missing := valid && message == "No information for transaction"
		return zero, missing, missing
	}
	f, ok = payoutObject(f["result"], "amount confirmations blockhash blockindex blocktime txid walletconflicts time timereceived details hex", "fee")
	if !ok || !payoutFloat(f["amount"]) || f["fee"] != nil && !payoutFloat(f["fee"]) {
		return zero, false, false
	}
	confirmations, ok := payoutInteger(f["confirmations"])
	if !ok || confirmations < 0 || confirmations > math.MaxInt32+1 {
		return zero, false, false
	}
	for _, field := range []string{"blockindex", "blocktime", "time", "timereceived"} {
		if _, ok := payoutInteger(f[field]); !ok {
			return zero, false, false
		}
	}
	txid, ok := payoutString(f["txid"], 64)
	if !ok || !payoutLowerHex(txid, 64, false) {
		return zero, false, false
	}
	block, ok := payoutString(f["blockhash"], 64)
	if !ok || confirmations == 0 && block != "" || confirmations > 0 && !payoutLowerHex(block, 64, true) {
		return zero, false, false
	}
	if !payoutArray(f["walletconflicts"], 256, func(v []byte) bool {
		h, ok := payoutString(v, 64)
		return ok && payoutLowerHex(h, 64, false)
	}) || !payoutArray(f["details"], 258, payoutDetail) {
		return zero, false, false
	}
	h, ok := payoutString(f["hex"], poolsettle.MaxTransactionBytes*2)
	if !ok || len(h) < 2 || len(h)%2 != 0 {
		return zero, false, false
	}
	for _, c := range h {
		if c < '0' || c > '9' && c < 'a' || c > 'f' {
			return zero, false, false
		}
	}
	txRaw := make([]byte, len(h)/2)
	if _, err := hex.Decode(txRaw, []byte(h)); err != nil {
		return zero, false, false
	}
	return walletPayoutTransaction{txid, confirmations, block, txRaw}, false, true
}
