// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectnode

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/prospect"
)

const (
	maxRPCResponseSize = 16 * 1024 * 1024
	operationTimeout   = 30 * time.Second
	rpcTimeout         = 10 * time.Second
)

// Client owns a bounded HTTP transport to one explicitly configured node.
// Requests have no redirect, proxy, retry, or insecure-TLS fallback.
type Client struct {
	endpoint string
	username string
	password string
	http     *http.Client
	nextID   atomic.Uint64
}

func NewClient(config RPCConfig) (*Client, error) {
	if err := productionOnly(); err != nil {
		return nil, err
	}
	endpoint, err := validateRPCConfig(config)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if config.CACertificate != "" {
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM([]byte(config.CACertificate)) {
			return nil, errors.New("invalid RPC CA certificate")
		}
		tlsConfig.RootCAs = roots
	}
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            (&net.Dialer{Timeout: rpcTimeout}).DialContext,
		TLSClientConfig:        tlsConfig,
		TLSHandshakeTimeout:    rpcTimeout,
		ResponseHeaderTimeout:  rpcTimeout,
		MaxResponseHeaderBytes: 65536,
		DisableCompression:     true,
		// Fresh connections also prevent transport replay on a stale pooled
		// connection. There is no retry loop at the RPC or snapshot layer.
		DisableKeepAlives: true,
	}
	return &Client{
		endpoint: endpoint.String(), username: config.Username, password: config.Password,
		http: &http.Client{Transport: transport, Timeout: rpcTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

// Close releases idle transport resources. It does not cancel active calls.
func (client *Client) Close() { client.http.CloseIdleConnections() }

func callError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("node RPC request failed")
}

func (client *Client) call(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	return client.callResult(ctx, method, params, false)
}

// callResult allows a null result only for proposal/submission RPCs, whose
// documented successful result is null. Ordinary node reads require a value.
func (client *Client) callResult(ctx context.Context, method string, params []any, allowNull bool) (json.RawMessage, error) {
	id := client.nextID.Add(1)
	payload, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      uint64 `json:"id"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
	}{"1.0", id, method, params})
	if err != nil {
		return nil, errors.New("unable to encode RPC request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, errors.New("invalid RPC request")
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(client.username, client.password)
	response, err := client.http.Do(request)
	if err != nil {
		return nil, callError(ctx)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "" {
		return nil, errors.New("node RPC returned an unsupported HTTP response")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxRPCResponseSize+1))
	if err != nil {
		return nil, callError(ctx)
	}
	if len(data) > maxRPCResponseSize {
		return nil, errors.New("node RPC response exceeds size limit")
	}
	fields, err := object(data, maxRPCResponseSize)
	if err != nil {
		return nil, errors.New("invalid node RPC response")
	}
	for key := range fields {
		if key != "id" && key != "result" && key != "error" && key != "jsonrpc" {
			return nil, errors.New("invalid node RPC envelope")
		}
	}
	if version, ok := fields["jsonrpc"]; ok {
		var value string
		if json.Unmarshal(version, &value) != nil || (value != "1.0" && value != "2.0") {
			return nil, errors.New("invalid node RPC version")
		}
	}
	var responseID uint64
	if value, ok := fields["id"]; !ok || isNull(value) || json.Unmarshal(value, &responseID) != nil || responseID != id {
		return nil, errors.New("node RPC response ID mismatch")
	}
	if value, ok := fields["error"]; !ok || !isNull(value) {
		return nil, errors.New("node RPC reported an error")
	}
	result, ok := fields["result"]
	if !ok || (!allowNull && isNull(result)) {
		return nil, errors.New("node RPC result is missing")
	}
	return result, nil
}

func (client *Client) identity(ctx context.Context) error {
	data, err := client.call(ctx, "getcurrentnet", []any{})
	if err != nil {
		return err
	}
	var network uint32
	if json.Unmarshal(data, &network) != nil || network != mainnetNetwork {
		return errors.New("node is not production SLUG mainnet")
	}
	data, err = client.call(ctx, "getblockhash", []any{0})
	if err != nil {
		return err
	}
	var genesis string
	if json.Unmarshal(data, &genesis) != nil {
		return errors.New("invalid node genesis response")
	}
	genesis, err = normalizeHash(genesis, true)
	if err != nil || genesis != mainnetGenesis {
		return errors.New("node genesis does not match production SLUG mainnet")
	}
	return nil
}

type tip struct {
	hash   string
	height int64
	median uint32
}

func (client *Client) chainTip(ctx context.Context) (tip, error) {
	data, err := client.call(ctx, "getblockchaininfo", []any{})
	if err != nil {
		return tip{}, err
	}
	var result tip
	var chain string
	if decodeFields(data, maxRPCResponseSize, map[string]any{
		"chain": &chain, "blocks": &result.height,
		"bestblockhash": &result.hash, "mediantime": &result.median,
	}, true) != nil || chain != "mainnet" || result.height < 0 || result.height > math.MaxInt32-1 || result.median == math.MaxUint32 {
		return tip{}, errors.New("invalid node chain information")
	}
	result.hash, err = normalizeHash(result.hash, true)
	if err != nil {
		return tip{}, errors.New("invalid node parent hash")
	}
	return result, nil
}

// compactTarget accepts only canonical positive compact values bounded by the
// SLUG mainnet proof-of-work limit. This is separate from the prospect target.
func compactTarget(value string) (uint32, string, error) {
	if len(value) == 0 || len(value) > 8 {
		return 0, "", errors.New("invalid template bits")
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return 0, "", errors.New("invalid template bits")
		}
	}
	number, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return 0, "", errors.New("invalid template bits")
	}
	bits := uint32(number)
	size, mantissa := bits>>24, bits&0x007fffff
	if bits&0x00800000 != 0 {
		return 0, "", errors.New("negative template target")
	}
	target := new(big.Int).SetUint64(uint64(mantissa))
	if size <= 3 {
		target.Rsh(target, uint(8*(3-size)))
	} else {
		target.Lsh(target, uint(8*(size-3)))
	}
	if target.Sign() <= 0 || target.Cmp(chaincfg.MainNetParams.PowLimit) > 0 {
		return 0, "", errors.New("template target is outside mainnet limits")
	}
	canonicalSize := uint32((target.BitLen() + 7) / 8)
	var canonical uint32
	if canonicalSize <= 3 {
		canonical = uint32(target.Uint64()) << (8 * (3 - canonicalSize))
	} else {
		canonical = uint32(new(big.Int).Rsh(new(big.Int).Set(target), uint(8*(canonicalSize-3))).Uint64())
	}
	if canonical&0x00800000 != 0 {
		canonical >>= 8
		canonicalSize++
	}
	canonical |= canonicalSize << 24
	if canonical != bits {
		return 0, "", errors.New("noncanonical template bits")
	}
	return bits, hex.EncodeToString(target.FillBytes(make([]byte, 32))), nil
}

// Resolve observes matching chain identity at both ends and a stable parent
// around getblocktemplate. A changing tip fails; callers decide when to retry.
// The template's peer/current checks are preserved, including its genesis
// exception. These checks are node assertions, not independent synchronization.
func (client *Client) Resolve(ctx context.Context, policy Policy) (prospect.Context, error) {
	resolved, _, err := client.resolveTemplateSnapshot(ctx, policy)
	return resolved, err
}

func (client *Client) resolveTemplateSnapshot(ctx context.Context, policy Policy) (prospect.Context, json.RawMessage, error) {
	if err := productionOnly(); err != nil {
		return prospect.Context{}, nil, err
	}
	policy, err := validatePolicy(policy)
	if err != nil {
		return prospect.Context{}, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := client.identity(ctx); err != nil {
		return prospect.Context{}, nil, err
	}
	before, err := client.chainTip(ctx)
	if err != nil {
		return prospect.Context{}, nil, err
	}
	data, err := client.call(ctx, "getblocktemplate", []any{struct {
		Mode         string   `json:"mode"`
		Capabilities []string `json:"capabilities"`
	}{"template", []string{"coinbasevalue"}}})
	if err != nil {
		return prospect.Context{}, nil, err
	}
	var parent, bitsText, targetText string
	var height int64
	var minTime, curTime, maxTime uint32
	if decodeFields(data, maxRPCResponseSize, map[string]any{
		"previousblockhash": &parent, "height": &height, "bits": &bitsText, "target": &targetText,
		"mintime": &minTime, "curtime": &curTime, "maxtime": &maxTime,
	}, true) != nil {
		return prospect.Context{}, nil, errors.New("invalid node block template")
	}
	parent, err = normalizeHash(parent, true)
	if err != nil || parent != before.hash || height != before.height+1 {
		return prospect.Context{}, nil, errors.New("node template parent is inconsistent")
	}
	if minTime == 0 || minTime != before.median+1 || curTime < minTime || curTime > maxTime {
		return prospect.Context{}, nil, errors.New("node template time interval is inconsistent")
	}
	bits, target, err := compactTarget(bitsText)
	if err != nil {
		return prospect.Context{}, nil, err
	}
	targetText, err = normalizeHash(targetText, true)
	if err != nil || targetText != target {
		return prospect.Context{}, nil, errors.New("node template target does not match bits")
	}
	templateData := data
	data, err = client.call(ctx, "getbestblock", []any{})
	if err != nil {
		return prospect.Context{}, nil, err
	}
	var after tip
	if decodeFields(data, maxRPCResponseSize, map[string]any{"hash": &after.hash, "height": &after.height}, true) != nil {
		return prospect.Context{}, nil, errors.New("invalid node best block response")
	}
	after.hash, err = normalizeHash(after.hash, true)
	if err != nil || after.hash != before.hash || after.height != before.height {
		return prospect.Context{}, nil, errors.New("node tip changed during context resolution")
	}
	if err := client.identity(ctx); err != nil {
		return prospect.Context{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return prospect.Context{}, nil, err
	}
	return prospect.Context{
		Version: prospect.Version1, Network: policy.Network, Genesis: policy.Genesis,
		DNAVersion: policy.DNAVersion, ProspectTarget: policy.ProspectTarget,
		ParentHash: before.hash, ParentHeight: uint32(before.height), ExpectedBits: bits,
		MinTimestamp: minTime, MaxTimestamp: maxTime,
	}, templateData, nil
}

// Recheck is a cheap final admission guard for a proof already verified under
// initial. It does not verify work or the coinbase. It resolves a fresh context,
// then requires the same parent/difficulty and current timestamp eligibility.
// A reorganization remains possible after this observation.
func (client *Client) Recheck(ctx context.Context, initial prospect.Context, proof prospect.Proof) error {
	if err := productionOnly(); err != nil {
		return err
	}
	if err := validateInitial(initial); err != nil {
		return err
	}
	switch proof.Version {
	case prospect.Version1:
		if proof.JobID != "" {
			return errors.New("version-1 proof must not carry a job ID")
		}
	case prospect.Version2:
		if _, err := normalizeHash(proof.JobID, false); err != nil {
			return errors.New("invalid proof job ID")
		}
	default:
		return errors.New("unsupported proof version")
	}
	if len(proof.Header) != 160 {
		return errors.New("invalid proof header size")
	}
	parent, err := normalizeHash(initial.ParentHash, true)
	if err != nil {
		return errors.New("invalid initial parent")
	}
	header, err := hex.DecodeString(proof.Header)
	if err != nil {
		return errors.New("invalid proof header")
	}
	var displayParent [32]byte
	for i := range displayParent {
		displayParent[i] = header[35-i]
	}
	timestamp := binary.LittleEndian.Uint32(header[68:72])
	if hex.EncodeToString(displayParent[:]) != parent || binary.LittleEndian.Uint32(header[72:76]) != initial.ExpectedBits ||
		timestamp < initial.MinTimestamp || timestamp > initial.MaxTimestamp {
		return errors.New("proof header does not match initial context")
	}
	fresh, err := client.resolveSameParent(ctx, initial)
	if err != nil {
		return err
	}
	if timestamp < fresh.MinTimestamp || timestamp > fresh.MaxTimestamp {
		return errors.New("proof context is no longer current")
	}
	return nil
}

func validateInitial(initial prospect.Context) error {
	if initial.Version != prospect.Version1 || initial.ParentHeight > math.MaxInt32-1 ||
		initial.MinTimestamp == 0 || initial.MaxTimestamp < initial.MinTimestamp {
		return errors.New("invalid initial node context")
	}
	if _, err := normalizeHash(initial.ParentHash, true); err != nil {
		return errors.New("invalid initial parent")
	}
	if _, err := validatePolicy(Policy{Version: Version1, Network: initial.Network, Genesis: initial.Genesis,
		DNAVersion: initial.DNAVersion, ProspectTarget: initial.ProspectTarget}); err != nil {
		return err
	}
	if _, _, err := compactTarget(strconv.FormatUint(uint64(initial.ExpectedBits), 16)); err != nil {
		return err
	}
	return nil
}

func (client *Client) resolveSameParent(ctx context.Context, initial prospect.Context) (prospect.Context, error) {
	if err := validateInitial(initial); err != nil {
		return prospect.Context{}, err
	}
	fresh, err := client.Resolve(ctx, Policy{Version: Version1, Network: initial.Network, Genesis: initial.Genesis,
		DNAVersion: initial.DNAVersion, ProspectTarget: initial.ProspectTarget})
	if err != nil {
		return prospect.Context{}, err
	}
	parent, _ := normalizeHash(initial.ParentHash, true)
	if fresh.ParentHash != parent || fresh.ParentHeight != initial.ParentHeight || fresh.ExpectedBits != initial.ExpectedBits ||
		fresh.MinTimestamp != initial.MinTimestamp {
		return prospect.Context{}, errors.New("node context parent or difficulty changed")
	}
	return fresh, nil
}

// RecheckContext guards issuance against a changed parent or backward-moving
// node time bounds without constructing a proof. It does not issue a job or
// attest a wall clock. A chain change remains possible after this observation.
func (client *Client) RecheckContext(ctx context.Context, initial prospect.Context) error {
	if err := productionOnly(); err != nil {
		return err
	}
	fresh, err := client.resolveSameParent(ctx, initial)
	if err != nil {
		return err
	}
	if fresh.MaxTimestamp < initial.MaxTimestamp {
		return errors.New("node context upper time bound moved backward")
	}
	return nil
}

// Verify obtains node context, verifies production work, and rechecks the node.
// The returned supplied-context result is not an atomic canonical-chain claim.
func (client *Client) Verify(ctx context.Context, policy Policy, proof prospect.Proof) (prospect.Context, prospect.Verified, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	resolved, err := client.Resolve(ctx, policy)
	if err != nil {
		return prospect.Context{}, prospect.Verified{}, err
	}
	verified, err := prospect.Verify(resolved, proof)
	if err != nil {
		return prospect.Context{}, prospect.Verified{}, err
	}
	if err := client.Recheck(ctx, resolved, proof); err != nil {
		return prospect.Context{}, prospect.Verified{}, err
	}
	return resolved, verified, nil
}
