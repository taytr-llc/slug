// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/walletwithdraw"
)

const withdrawalRPCBytes = 65536

var errWithdrawalNode = errors.New("candidate withdrawal node unavailable")

type withdrawalNode struct {
	config prospectnode.RPCConfig
	http   *http.Client
	next   atomic.Uint64
}
type withdrawalTip struct {
	Hash   string `json:"hash"`
	Height uint32 `json:"height"`
}
type withdrawalRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *withdrawalRPCError) Error() string { return "candidate withdrawal node refusal" }
func newWithdrawalNode(c prospectnode.RPCConfig) (*withdrawalNode, error) {
	raw, e := json.Marshal(c)
	if e != nil {
		return nil, errWithdrawalNode
	}
	if _, e = prospectnode.ParseRPCConfig(raw); e != nil {
		return nil, errWithdrawalNode
	}
	u, e := url.Parse(c.URL)
	if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || c.CACertificate != "" {
		return nil, errWithdrawalNode
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 8192, DisableCompression: true, DisableKeepAlives: true}
	return &withdrawalNode{config: c, http: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (n *withdrawalNode) close() {
	if n != nil && n.http != nil {
		n.http.CloseIdleConnections()
	}
}

// Scan every value before typed allocation, including extension fields we do
// not consume. Duplicate decoded keys are rejected at every nesting level.
func withdrawalRPCJSON(raw []byte) error {
	if len(raw) == 0 || len(raw) > withdrawalRPCBytes || !utf8.Valid(raw) {
		return errWithdrawalNode
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		t, e := d.Token()
		if e != nil || depth > 8 {
			return errWithdrawalNode
		}
		switch v := t.(type) {
		case json.Delim:
			switch v {
			case '{':
				seen := make(map[string]bool)
				for d.More() {
					k, e := d.Token()
					s, ok := k.(string)
					if e != nil || !ok || len(s) > 64 || len(seen) >= 64 || seen[s] {
						return errWithdrawalNode
					}
					seen[s] = true
					if e = walk(depth + 1); e != nil {
						return e
					}
				}
				end, e := d.Token()
				if e != nil || end != json.Delim('}') {
					return errWithdrawalNode
				}
			case '[':
				count := 0
				for d.More() {
					if count >= 32 {
						return errWithdrawalNode
					}
					count++
					if e = walk(depth + 1); e != nil {
						return e
					}
				}
				end, e := d.Token()
				if e != nil || end != json.Delim(']') {
					return errWithdrawalNode
				}
			default:
				return errWithdrawalNode
			}
		case string:
			if len(v) > 8192 {
				return errWithdrawalNode
			}
		case json.Number:
			if len(v) > 64 {
				return errWithdrawalNode
			}
		case bool, nil:
		default:
			return errWithdrawalNode
		}
		return nil
	}
	if e := walk(0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return errWithdrawalNode
	}
	return nil
}
func withdrawalFields(raw []byte, required ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, errWithdrawalNode
	}
	for _, name := range required {
		v, ok := fields[name]
		if !ok || bytes.Equal(v, []byte("null")) {
			return nil, errWithdrawalNode
		}
	}
	return fields, nil
}
func withdrawalField(fields map[string]json.RawMessage, name string, out any) error {
	raw, ok := fields[name]
	if !ok || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, out) != nil {
		return errWithdrawalNode
	}
	return nil
}
func (n *withdrawalNode) call(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errWithdrawalNode
	}
	work, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	id := n.next.Add(1)
	if id == 0 {
		return nil, errWithdrawalNode
	}
	if params == nil {
		params = []any{}
	}
	raw, e := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      uint64 `json:"id"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
	}{"1.0", id, method, params})
	if e != nil || len(raw) > 16384 {
		return nil, errWithdrawalNode
	}
	request, e := http.NewRequestWithContext(work, http.MethodPost, n.config.URL, bytes.NewReader(raw))
	if e != nil {
		return nil, errWithdrawalNode
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(n.config.Username, n.config.Password)
	response, e := n.http.Do(request)
	if e != nil {
		return nil, errWithdrawalNode
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "" {
		return nil, errWithdrawalNode
	}
	body, e := io.ReadAll(io.LimitReader(response.Body, withdrawalRPCBytes+1))
	if e != nil || withdrawalRPCJSON(body) != nil {
		return nil, errWithdrawalNode
	}
	fields, e := withdrawalFields(body, "id")
	if e != nil {
		return nil, e
	}
	for k := range fields {
		if k != "id" && k != "result" && k != "error" && k != "jsonrpc" {
			return nil, errWithdrawalNode
		}
	}
	var actual uint64
	if withdrawalField(fields, "id", &actual) != nil || actual != id {
		return nil, errWithdrawalNode
	}
	if _, ok := fields["jsonrpc"]; ok {
		var version string
		if withdrawalField(fields, "jsonrpc", &version) != nil || version != "1.0" && version != "2.0" {
			return nil, errWithdrawalNode
		}
	}
	result, hasResult := fields["result"]
	failure, hasError := fields["error"]
	if !hasResult || !hasError {
		return nil, errWithdrawalNode
	}
	if !bytes.Equal(failure, []byte("null")) {
		errFields, e := withdrawalFields(failure, "code", "message")
		if e != nil || len(errFields) != 2 || !bytes.Equal(result, []byte("null")) {
			return nil, errWithdrawalNode
		}
		var refusal withdrawalRPCError
		if json.Unmarshal(failure, &refusal) != nil || len(refusal.Message) > 1024 {
			return nil, errWithdrawalNode
		}
		return nil, &refusal
	}
	return result, nil
}
func (n *withdrawalNode) read(ctx context.Context, method string, params []any, out any, required ...string) error {
	raw, e := n.call(ctx, method, params)
	if e != nil {
		return e
	}
	if len(required) > 0 {
		if _, e = withdrawalFields(raw, required...); e != nil {
			return e
		}
	}
	if bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, out) != nil {
		return errWithdrawalNode
	}
	return nil
}
func withdrawalHash(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s && !bytes.Equal(b, make([]byte, 32))
}
func (n *withdrawalNode) snapshot(ctx context.Context) (withdrawalTip, error) {
	var magic uint32
	var origin, canonical string
	var t withdrawalTip
	if n.read(ctx, "getcurrentnet", nil, &magic) != nil || magic != uint32(chaincfg.MainNetParams.Net) || n.read(ctx, "getblockhash", []any{0}, &origin) != nil || origin != chaincfg.MainNetParams.GenesisHash.String() || n.read(ctx, "getbestblock", nil, &t, "hash", "height") != nil || !withdrawalHash(t.Hash) || t.Height > math.MaxInt32 || n.read(ctx, "getblockhash", []any{t.Height}, &canonical) != nil || canonical != t.Hash {
		return withdrawalTip{}, errWithdrawalNode
	}
	var info struct {
		Chain       string `json:"chain"`
		Blocks      uint32 `json:"blocks"`
		Best        string `json:"bestblockhash"`
		Deployments map[string]struct {
			Status string `json:"status"`
			Height uint32 `json:"always_active_height"`
		} `json:"bip9_softforks"`
	}
	if n.read(ctx, "getblockchaininfo", nil, &info, "chain", "blocks", "bestblockhash", "bip9_softforks") != nil || info.Chain != "mainnet" || info.Blocks != t.Height || info.Best != t.Hash {
		return withdrawalTip{}, errWithdrawalNode
	}
	for _, name := range []string{"csv", "segwit", "taproot"} {
		d, ok := info.Deployments[name]
		if !ok || d.Height != chaincfg.PhaseAActivationHeight || (d.Status == "active") != (t.Height+1 >= chaincfg.PhaseAActivationHeight) {
			return withdrawalTip{}, errWithdrawalNode
		}
		switch d.Status {
		case "defined", "started", "locked_in", "active", "failed":
		default:
			return withdrawalTip{}, errWithdrawalNode
		}
	}
	return t, nil
}
func (n *withdrawalNode) unchanged(ctx context.Context, t withdrawalTip) error {
	after, e := n.snapshot(ctx)
	if e != nil || after != t {
		return errWithdrawalNode
	}
	return nil
}
func withdrawalAtoms(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || len(raw) > 64 {
		return 0, errWithdrawalNode
	}
	if i := strings.IndexAny(string(raw), "eE"); i >= 0 {
		e, err := strconv.ParseInt(string(raw[i+1:]), 10, 32)
		if err != nil || e < -32 || e > 32 {
			return 0, errWithdrawalNode
		}
	}
	r, ok := new(big.Rat).SetString(string(raw))
	if !ok || r.Sign() < 0 {
		return 0, errWithdrawalNode
	}
	r.Mul(r, big.NewRat(100000000, 1))
	if !r.IsInt() || !r.Num().IsInt64() || r.Num().Int64() > withdrawalMaxMoney {
		return 0, errWithdrawalNode
	}
	return r.Num().Int64(), nil
}
func (n *withdrawalNode) inspect(ctx context.Context, d walletwithdraw.Draft, walletTip withdrawalTip) error {
	t, e := n.snapshot(ctx)
	if e != nil {
		return e
	}
	if t != walletTip || t.Height+1 < chaincfg.PhaseAActivationHeight {
		return walletwithdraw.ErrConflict
	}
	type outpoint struct {
		TxID  string `json:"txid"`
		Index uint32 `json:"vout"`
	}
	points := make([]outpoint, len(d.Inputs))
	origins := make(map[uint32]string)
	for i, in := range d.Inputs {
		points[i] = outpoint{in.TxID, in.OutputIndex}
		if in.BlockHeight > t.Height || in.BlockHeight == 0 {
			return walletwithdraw.ErrConflict
		}
		conf := t.Height - in.BlockHeight + 1
		if conf < d.MinConfirmations || in.Coinbase && conf < uint32(chaincfg.MainNetParams.CoinbaseMaturity) {
			return walletwithdraw.ErrConflict
		}
		canonical, ok := origins[in.BlockHeight]
		if !ok {
			if n.read(ctx, "getblockhash", []any{in.BlockHeight}, &canonical) != nil {
				return errWithdrawalNode
			}
			origins[in.BlockHeight] = canonical
		}
		if canonical != in.BlockHash {
			return walletwithdraw.ErrConflict
		}
		raw, err := n.call(ctx, "gettxout", []any{in.TxID, in.OutputIndex, true})
		if err != nil {
			return err
		}
		if bytes.Equal(raw, []byte("null")) {
			return walletwithdraw.ErrConflict
		}
		fields, err := withdrawalFields(raw, "bestblock", "confirmations", "value", "coinbase", "scriptPubKey")
		if err != nil {
			return err
		}
		var utxo struct {
			BestBlock     string          `json:"bestblock"`
			Confirmations uint32          `json:"confirmations"`
			Value         json.RawMessage `json:"value"`
			Coinbase      bool            `json:"coinbase"`
			Script        struct {
				Hex string `json:"hex"`
			} `json:"scriptPubKey"`
		}
		if _, err = withdrawalFields(fields["scriptPubKey"], "hex"); err != nil || json.Unmarshal(raw, &utxo) != nil {
			return errWithdrawalNode
		}
		atoms, err := withdrawalAtoms(utxo.Value)
		if err != nil || utxo.BestBlock != t.Hash || utxo.Confirmations != conf || atoms != in.Atoms || utxo.Coinbase != in.Coinbase || utxo.Script.Hex != in.Script {
			return walletwithdraw.ErrConflict
		}
	}
	raw, e := n.call(ctx, "gettxspendingprevout", []any{points})
	if e != nil {
		return e
	}
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil || len(rows) != len(points) {
		return errWithdrawalNode
	}
	for i, row := range rows {
		fields, err := withdrawalFields(row, "txid", "vout")
		if err != nil || len(fields) > 3 {
			return errWithdrawalNode
		}
		for key := range fields {
			if key != "txid" && key != "vout" && key != "spendingtxid" {
				return errWithdrawalNode
			}
		}
		var spent struct {
			TxID     string `json:"txid"`
			Index    uint32 `json:"vout"`
			Spending string `json:"spendingtxid"`
		}
		if json.Unmarshal(row, &spent) != nil || spent.TxID != points[i].TxID || spent.Index != points[i].Index {
			return errWithdrawalNode
		}
		if _, exists := fields["spendingtxid"]; exists && withdrawalField(fields, "spendingtxid", &spent.Spending) != nil {
			return errWithdrawalNode
		}
		if spent.Spending != "" {
			if !withdrawalHash(spent.Spending) {
				return errWithdrawalNode
			}
			return walletwithdraw.ErrConflict
		}
	}
	return n.unchanged(ctx, t)
}
func (n *withdrawalNode) deliver(ctx context.Context, raw []byte, txid string) (walletwithdraw.DeliveryResult, string) {
	if len(raw) == 0 || len(raw) > 4096 || !withdrawalHash(txid) {
		return walletwithdraw.DeliveryUnknown, "node_unavailable"
	}
	t, e := n.snapshot(ctx)
	if e != nil || t.Height+1 < chaincfg.PhaseAActivationHeight {
		return walletwithdraw.DeliveryUnknown, "node_unavailable"
	}
	var ack string
	e = n.read(ctx, "sendrawtransaction", []any{hex.EncodeToString(raw), false}, &ack)
	if e == nil && ack == txid {
		return walletwithdraw.DeliveryAcknowledged, ""
	}
	var refusal *withdrawalRPCError
	if errors.As(e, &refusal) && (refusal.Code == -22 || refusal.Code == -25 || refusal.Code == -26) {
		return walletwithdraw.DeliveryRejected, "node_rejected"
	}
	return walletwithdraw.DeliveryUnknown, "node_unavailable"
}
func (n *withdrawalNode) observe(ctx context.Context, s walletwithdraw.Signed) walletwithdraw.ObservationInput {
	unknown := walletwithdraw.ObservationInput{State: walletwithdraw.ObservationUnknown, Error: "node_unavailable"}
	t, e := n.snapshot(ctx)
	if e != nil {
		return unknown
	}
	var raw string
	e = n.read(ctx, "getrawtransaction", []any{s.TxID, 0}, &raw)
	if e != nil {
		var refusal *withdrawalRPCError
		if !errors.As(e, &refusal) || refusal.Code != -5 || refusal.Message != "No information available about transaction "+s.TxID || n.unchanged(ctx, t) != nil {
			return unknown
		}
		return walletwithdraw.ObservationInput{State: walletwithdraw.ObservationNotObserved, NodeTip: t.Hash, NodeHeight: t.Height}
	}
	if raw != s.Raw {
		return unknown
	}
	var verbose struct {
		Hex           string `json:"hex"`
		TxID          string `json:"txid"`
		BlockHash     string `json:"blockhash"`
		Confirmations uint32 `json:"confirmations"`
	}
	data, e := n.call(ctx, "getrawtransaction", []any{s.TxID, 1})
	if e != nil {
		return unknown
	}
	fields, e := withdrawalFields(data, "hex", "txid")
	if e != nil || json.Unmarshal(data, &verbose) != nil || verbose.Hex != s.Raw || verbose.TxID != s.TxID {
		return unknown
	}
	if _, exists := fields["blockhash"]; exists && withdrawalField(fields, "blockhash", &verbose.BlockHash) != nil {
		return unknown
	}
	if _, exists := fields["confirmations"]; exists && withdrawalField(fields, "confirmations", &verbose.Confirmations) != nil {
		return unknown
	}
	observation := walletwithdraw.ObservationInput{State: walletwithdraw.ObservationMempool, NodeTip: t.Hash, NodeHeight: t.Height}
	if verbose.BlockHash == "" {
		if verbose.Confirmations != 0 {
			return unknown
		}
	} else {
		if !withdrawalHash(verbose.BlockHash) {
			return unknown
		}
		var header withdrawalTip
		var canonical string
		if n.read(ctx, "getblockheader", []any{verbose.BlockHash, true}, &header, "hash", "height") != nil || header.Hash != verbose.BlockHash || header.Height > t.Height || n.read(ctx, "getblockhash", []any{header.Height}, &canonical) != nil || canonical != header.Hash || verbose.Confirmations != t.Height-header.Height+1 {
			return unknown
		}
		observation.State = walletwithdraw.ObservationConfirmed
		observation.BlockHash = header.Hash
		observation.Confirmations = verbose.Confirmations
	}
	if n.unchanged(ctx, t) != nil {
		return unknown
	}
	return observation
}
