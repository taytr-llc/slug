// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"math/big"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

func NewClient(config ClientConfig) (*Client, error) {
	if wire.TestPoW {
		return nil, errConfig
	}
	c, e := ValidateClientConfig(config)
	if e != nil {
		return nil, e
	}
	binding, e := BindingID(c.Source)
	if e != nil {
		return nil, e
	}
	trust, e := TransportTrustID(c)
	if e != nil {
		return nil, e
	}
	gym, e := gymauth.GymID(identity(c.Source), c.PublicKey)
	if e != nil {
		return nil, errConfig
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.CACertificate != "" {
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM([]byte(c.CACertificate)) {
			return nil, errConfig
		}
		tlsConfig.RootCAs = roots
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext, TLSClientConfig: tlsConfig, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, MaxResponseHeaderBytes: 8192, DisableCompression: true, DisableKeepAlives: true}
	return &Client{config: c, binding: binding, trust: trust, gym: gym, http: &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) ProtocolVersion() uint32        { return Version1 }
func (c *Client) AuthorityOrigin() string        { return c.config.URL }
func (c *Client) SourceConfig() poolshare.Config { return c.config.Source }
func (c *Client) TransportTrustID() string       { return c.trust }
func (c *Client) PublicKey() string              { return c.config.PublicKey }

// Close releases idle connections. The owner cancels and joins active requests
// and normal-PoW validation before replacing this immutable client.
func (c *Client) Close() {
	if c != nil && c.http != nil {
		c.http.CloseIdleConnections()
	}
}

func failure(mutation bool) error {
	if mutation {
		return &ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	return &ClientError{Code: "unavailable"}
}
func (c *Client) ready(ctx context.Context) error {
	if c == nil || c.http == nil || ctx == nil || wire.TestPoW {
		return &ClientError{Code: "invalid_request"}
	}
	if ctx.Err() != nil {
		return failure(false)
	}
	return nil
}
func (c *Client) post(ctx context.Context, path string, value any, mutation bool, limit int) ([]byte, error) {
	raw, e := json.Marshal(value)
	if e != nil || len(raw) > MaxRequestSize {
		return nil, &ClientError{Code: "invalid_request"}
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.config.URL+pathPrefix+path, bytes.NewReader(raw))
	if e != nil {
		return nil, &ClientError{Code: "invalid_request"}
	}
	req.Header.Set("Authorization", "Bearer "+c.config.Token)
	req.Header.Set("Content-Type", "application/json")
	res, e := c.http.Do(req)
	if e != nil {
		return nil, failure(mutation)
	}
	defer res.Body.Close()
	values := res.Header.Values("Content-Type")
	if len(values) != 1 || len(res.Header.Values("Content-Encoding")) != 0 {
		return nil, failure(mutation)
	}
	media, params, e := mime.ParseMediaType(values[0])
	if e != nil || media != "application/json" {
		return nil, failure(mutation)
	}
	for k, v := range params {
		if k != "charset" || !strings.EqualFold(v, "utf-8") {
			return nil, failure(mutation)
		}
	}
	if res.StatusCode != http.StatusOK {
		limit = MaxResponseSize
	}
	if res.ContentLength > int64(limit) {
		return nil, failure(mutation)
	}
	raw, e = io.ReadAll(io.LimitReader(res.Body, int64(limit)+1))
	if e != nil || len(raw) > limit || ctx.Err() != nil {
		return nil, failure(mutation)
	}
	if res.StatusCode != http.StatusOK {
		var result ErrorResponse
		if decodeStrict(raw, MaxResponseSize, &result) != nil || result.Version != Version1 || errorStatus(result.Error.Code) != res.StatusCode || result.Error.OutcomeUnknown != (result.Error.Code == "outcome_unknown") {
			return nil, failure(mutation)
		}
		return nil, &ClientError{Code: result.Error.Code, OutcomeUnknown: result.Error.OutcomeUnknown}
	}
	return raw, nil
}
func (c *Client) work(ctx context.Context, jobID string, issue bool) (poolshare.MiningWork, error) {
	if e := c.ready(ctx); e != nil {
		return poolshare.MiningWork{}, e
	}
	path := "issue"
	var request any = struct{}{}
	if !issue {
		if !hex64(jobID, false) {
			return poolshare.MiningWork{}, &ClientError{Code: "invalid_request"}
		}
		path = "work"
		request = workRequest{jobID}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, e := c.post(ctx, path, request, issue, MaxWorkResponseSize)
	if e != nil {
		return poolshare.MiningWork{}, e
	}
	var result WorkResponse
	if decodeStrict(raw, MaxWorkResponseSize, &result) != nil || result.Version != Version1 || result.BindingID != c.binding || !issue && result.Work.Job.ID != jobID {
		return poolshare.MiningWork{}, failure(issue)
	}
	if _, e = PrepareWork(c.config.Source, c.config.PublicKey, result.Work); e != nil || ctx.Err() != nil {
		return poolshare.MiningWork{}, failure(issue)
	}
	return result.Work, nil
}
func (c *Client) Issue(ctx context.Context) (poolshare.MiningWork, error) {
	return c.work(ctx, "", true)
}
func (c *Client) Work(ctx context.Context, jobID string) (poolshare.MiningWork, error) {
	return c.work(ctx, jobID, false)
}

func validSubmission(source poolshare.Config, w poolshare.MiningWork, nonce uint32, check prospectwork.ShareCheck, r poolshare.Submission) bool {
	if r.Version != poolshare.Version1 || r.Sequence == 0 || r.Sequence > source.MaxSubmissions || r.AcceptedSequence > source.MaxSubmissions || r.JobID != w.Job.ID || r.GymID != w.Job.GymID || r.Nonce != nonce || r.PoolTarget != source.PoolTarget || r.Header != check.Header || r.WorkHash != check.WorkHash || r.DuplicateKey != check.DuplicateKey || r.Pool != check.Pool || r.Prospect != check.Prospect || r.Block != check.Block || !r.Pool && !r.Block || r.ReceivedAtMS < w.Job.IssuedAtMS {
		return false
	}
	if r.AcceptedSequence == 0 {
		if r.AcceptedAtMS != 0 || r.Acceptance != "" || !r.Block || r.BlockState == "canonical-observed" {
			return false
		}
	} else {
		if r.AcceptedAtMS < r.ReceivedAtMS {
			return false
		}
		switch r.Acceptance {
		case "current-parent":
			if !r.Pool || r.AcceptedAtMS >= w.Job.ExpiresAtMS {
				return false
			}
		case "canonical-winning-block":
			if !r.Block || r.BlockState != "canonical-observed" || r.AcceptedAtMS >= w.Job.ExpiresAtMS {
				return false
			}
		case "canonical-late-winner":
			if !r.Block || r.BlockState != "canonical-observed" || r.AcceptedAtMS < w.Job.ExpiresAtMS {
				return false
			}
		default:
			return false
		}
	}
	if !r.Block {
		return r.BlockHeight == 0 && r.BlockState == "none" && r.BlockObservedAtMS == 0
	}
	if r.BlockHeight != w.Job.Context.ParentHeight+1 {
		return false
	}
	switch r.BlockState {
	case "pending":
		return r.BlockObservedAtMS == 0
	case "processed", "unknown", "rejected", "canonical-observed":
		return r.BlockObservedAtMS >= r.ReceivedAtMS && r.BlockObservedAtMS >= r.AcceptedAtMS
	}
	return false
}
func (c *Client) submission(ctx context.Context, w poolshare.MiningWork, nonce uint32, submit bool) (poolshare.Submission, error) {
	if e := c.ready(ctx); e != nil {
		return poolshare.Submission{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	prepared, e := PrepareWork(c.config.Source, c.config.PublicKey, w)
	if e != nil {
		return poolshare.Submission{}, &ClientError{Code: "invalid_request"}
	}
	check, e := prepared.CheckNonce(ctx, nonce)
	if e != nil {
		return poolshare.Submission{}, failure(false)
	}
	if !check.Pool && !check.Block {
		return poolshare.Submission{}, &ClientError{Code: "rejected"}
	}
	path := "lookup"
	if submit {
		path = "submit"
	}
	raw, e := c.post(ctx, path, submissionRequest{w.Job.ID, nonce}, submit, MaxResponseSize)
	if e != nil {
		return poolshare.Submission{}, e
	}
	var result SubmissionResponse
	if decodeStrict(raw, MaxResponseSize, &result) != nil || result.Version != Version1 || result.BindingID != c.binding || !validSubmission(c.config.Source, w, nonce, check, result.Submission) || ctx.Err() != nil {
		return poolshare.Submission{}, failure(submit)
	}
	return result.Submission, nil
}
func (c *Client) Submit(ctx context.Context, w poolshare.MiningWork, nonce uint32) (poolshare.Submission, error) {
	return c.submission(ctx, w, nonce, true)
}
func (c *Client) Lookup(ctx context.Context, w poolshare.MiningWork, nonce uint32) (poolshare.Submission, error) {
	return c.submission(ctx, w, nonce, false)
}

func fraction(f poolsettle.Fraction) (*big.Rat, bool) {
	if len(f.Numerator) == 0 || len(f.Denominator) == 0 || len(f.Numerator) > 4096 || len(f.Denominator) > 4096 {
		return nil, false
	}
	n, nok := new(big.Int).SetString(f.Numerator, 10)
	d, dok := new(big.Int).SetString(f.Denominator, 10)
	if !nok || !dok || n.Sign() < 0 || d.Sign() <= 0 || n.String() != f.Numerator || d.String() != f.Denominator {
		return nil, false
	}
	r := new(big.Rat).SetFrac(n, d)
	if r.Num().String() != f.Numerator || r.Denom().String() != f.Denominator || r.Cmp(new(big.Rat).SetInt64(poolsettle.MaxMoney)) > 0 {
		return nil, false
	}
	return r, true
}
func balanceReason(s string) bool {
	switch s {
	case "", "payout-pending", "broadcast-unknown", "transaction-unavailable", "payout-conflict", "funding-unavailable", "funding-conflict", "funding-changed", "immature", "awaiting-confirmation", "below-minimum", "destination-unavailable", "daily-batch-complete", "fee-unavailable", "fee-budget", "change-dust", "signing-unavailable", "capacity":
		return true
	}
	return false
}
func validBalance(r BalanceResponse, gym, binding string) bool {
	b := r.Balance
	if r.Version != Version1 || r.BindingID != binding || b.GymID != gym || r.ObservedAtMS < 0 || b.PaidAtoms < 0 || b.PaidAtoms > poolsettle.MaxMoney || b.ReservedAtoms < 0 || b.ReservedAtoms > poolsettle.MaxMoney || !balanceReason(b.Reason) {
		return false
	}
	confirmed, ok := fraction(b.Confirmed)
	if !ok {
		return false
	}
	available, ok := fraction(b.Available)
	if !ok {
		return false
	}
	remaining := new(big.Rat).Sub(confirmed, new(big.Rat).SetInt64(b.ReservedAtoms))
	if remaining.Sign() < 0 || available.Cmp(remaining) > 0 || confirmed.Sign() == 0 && b.Reason != "" {
		return false
	}
	return r.ObservedAtMS != 0 || (confirmed.Sign() == 0 && available.Sign() == 0 && b.PaidAtoms == 0 && b.ReservedAtoms == 0 && b.Reason == "")
}
func (c *Client) Balance(ctx context.Context) (BalanceResponse, error) {
	if e := c.ready(ctx); e != nil {
		return BalanceResponse{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, e := c.post(ctx, "balance", struct{}{}, false, MaxResponseSize)
	if e != nil {
		return BalanceResponse{}, e
	}
	var result BalanceResponse
	if decodeStrict(raw, MaxResponseSize, &result) != nil || !validBalance(result, c.gym, c.binding) || ctx.Err() != nil {
		return BalanceResponse{}, failure(false)
	}
	return result, nil
}

var _ Authority = (*Client)(nil)
