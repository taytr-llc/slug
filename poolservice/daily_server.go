// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/wire/v2"
	"io"
	"net/http"
	"time"
)

type dailyServerBinding struct {
	settlement   string
	destinations map[string]string
	maxBatches   uint64
}

// NewDailyServer borrows the original fully audited writable daily owners.
// It never selects daily mode from on-disk data or repairs an old root.
func NewDailyServer(ctx context.Context, c ServerConfig, pool *poolshare.Pool, settlement *poolsettle.Store, sessions prospectservice.SessionAuthorizer) (*Server, error) {
	if wire.TestPoW {
		return nil, poolshare.ErrProfile
	}
	if ctx == nil || ctx.Err() != nil || c.Version != Version2 || pool == nil || settlement == nil || !nonnil(sessions) {
		return nil, errServer
	}
	legacy := c
	legacy.Version = Version1
	validated, err := validateListen(legacy)
	if err != nil {
		return nil, errServer
	}
	c.Listen = validated.Listen
	source, err := pool.DailyBinding(ctx)
	if err != nil {
		return nil, err
	}
	cfg, ledgerSource, err := settlement.DailySchedulerBinding(ctx)
	if err != nil {
		return nil, err
	}
	if ledgerSource != source {
		return nil, errServer
	}
	binding, err := poolshare.DailyConfigID(source)
	if err != nil {
		return nil, errServer
	}
	configID, err := poolsettle.DailyConfigBindingID(cfg)
	if err != nil {
		return nil, errServer
	}
	destinations := make(map[string]string, len(cfg.Destinations))
	for _, d := range cfg.Destinations {
		destinations[d.GymID] = d.Script
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return &Server{config: c, source: source, binding: binding, pool: pool, settlement: settlement, sessions: sessions, active: make(chan struct{}, 1), rates: make(map[string]*clientRates), daily: &dailyServerBinding{configID, destinations, uint64(cfg.MaxBatches)}}, nil
}
func writeDailyFailure(w http.ResponseWriter, code string) {
	raw, _ := json.Marshal(ErrorResponse{Version: Version2, Error: APIError{Code: code, OutcomeUnknown: code == "outcome_unknown"}})
	publicHeaders(w)
	w.WriteHeader(dailyErrorStatus(code))
	_, _ = w.Write(append(raw, '\n'))
}

func (s *Server) serveDailyHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil || wire.TestPoW || !s.begin() {
		writeDailyFailure(w, "unavailable")
		return
	}
	defer s.handlers.Done()
	if r == nil || r.URL == nil || r.Body == nil || !validHost(r) || len(r.Header.Values("Origin")) != 0 || r.ContentLength < 0 || len(r.TransferEncoding) != 0 || len(r.Trailer) != 0 || len(r.Header.Values("Content-Length")) > 1 || r.URL.Fragment != "" || r.RequestURI != "" && r.RequestURI != r.URL.Path {
		writeDailyFailure(w, "forbidden")
		return
	}
	action := requestActionSelected(r, dailyPathPrefix)
	if action == "" {
		writeDailyFailure(w, "invalid_request")
		return
	}
	select {
	case s.active <- struct{}{}:
	default:
		writeDailyFailure(w, "busy")
		return
	}
	data, failure := func() ([]byte, string) {
		defer func() { <-s.active }()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		token := bearer(r)
		key, gym, e := s.authenticate(ctx, token, "")
		if e != nil {
			return nil, "unauthorized"
		}
		recipient, member := s.daily.destinations[gym]
		if !member {
			return nil, "unauthorized"
		}
		limit := s.allowance(key, action)
		if limit == nil {
			return nil, "capacity"
		}
		if !limit.Allow() {
			return nil, "rate_limited"
		}
		if r.ContentLength > MaxRequestSize {
			return nil, "too_large"
		}
		body, e := io.ReadAll(io.LimitReader(r.Body, MaxRequestSize+1))
		if len(body) > MaxRequestSize {
			return nil, "too_large"
		}
		if e != nil || int64(len(body)) != r.ContentLength {
			return nil, "invalid_request"
		}
		if ctx.Err() != nil {
			return nil, "unavailable"
		}
		var query poolsettle.DailyPayoutRequest
		var jobID string
		var nonce uint32
		switch action {
		case "payouts":
			if decodeDailyPageRequest(body, &query) != nil || poolsettle.ValidateDailyPayoutRequest(query, s.daily.maxBatches) != nil {
				return nil, "invalid_request"
			}
		case "issue", "balance":
			var empty struct{}
			if decodeDailyStrict(body, MaxRequestSize, &empty) != nil {
				return nil, "invalid_request"
			}
		case "work":
			var input workRequest
			if decodeDailyStrict(body, MaxRequestSize, &input) != nil || !hex64(input.JobID, false) {
				return nil, "invalid_request"
			}
			jobID = input.JobID
		case "submit", "lookup":
			var input submissionRequest
			if decodeDailyStrict(body, MaxRequestSize, &input) != nil || !hex64(input.JobID, false) {
				return nil, "invalid_request"
			}
			jobID, nonce = input.JobID, input.Nonce
		}
		var result any
		mutating := action == "issue" || action == "submit"
		maxSize := MaxResponseSize
		switch action {
		case "payouts":
			var history poolsettle.DailyPayoutPage
			history, e = s.settlement.MinerDailyPayoutPage(ctx, s.sessions, token, query)
			if e == nil {
				if history.Balance.GymID != gym || history.Version != Version2 || history.BindingID != s.binding || history.SettlementConfigID != s.daily.settlement || history.RecipientScript != recipient {
					return nil, "unavailable"
				}
				result = history
				maxSize = MaxPayoutResponseSize
			}
		case "issue", "work":
			var work poolshare.MiningWork
			if action == "issue" {
				work, e = s.pool.IssueMiningWork(ctx, token)
			} else {
				work, e = s.pool.RecoverMiningWork(ctx, token, jobID)
			}
			if e == nil {
				if work.Job.PublicKey != key || work.Job.GymID != gym {
					return nil, operationFailure(errServer, mutating)
				}
				result = DailyWorkResponse{Version2, s.binding, s.daily.settlement, recipient, work}
				maxSize = MaxWorkResponseSize
			}
		case "submit", "lookup":
			var receipt poolshare.Submission
			if action == "submit" {
				receipt, e = s.pool.Submit(ctx, token, jobID, nonce)
			} else {
				receipt, e = s.pool.Lookup(ctx, token, jobID, nonce)
			}
			if e == nil {
				if receipt.GymID != gym {
					return nil, operationFailure(errServer, mutating)
				}
				result = DailySubmissionResponse{Version2, s.binding, s.daily.settlement, recipient, receipt}
			}
		case "balance":
			var balance poolsettle.Balance
			var observed time.Time
			balance, observed, e = s.settlement.MinerBalance(ctx, s.sessions, token)
			if e == nil {
				if balance.GymID != gym {
					return nil, "unavailable"
				}
				var ms int64
				if !observed.IsZero() {
					ms = observed.UnixMilli()
				}
				dailyBalance := poolsettle.DailyBalanceFrom(balance)
				if poolsettle.ValidateDailyBalance(dailyBalance, gym, ms) != nil {
					return nil, "unavailable"
				}
				result = DailyBalanceResponse{Version2, s.binding, s.daily.settlement, recipient, dailyBalance, ms}
			}
		}
		if e != nil {
			if action == "payouts" {
				if ctx.Err() != nil {
					return nil, "unavailable"
				}
				if e == poolsettle.ErrStaleCapture {
					return nil, "stale_capture"
				}
				return nil, payoutFailure(e)
			}
			// Issuance can fail after the underlying job was synchronized but
			// before the complete pool work was acknowledged. No new-issue retry
			// is silently described as recovery of that original job.
			if action == "issue" && e != poolshare.ErrBusy {
				return nil, "outcome_unknown"
			}
			return nil, operationFailure(e, mutating)
		}
		encoded, e := json.Marshal(result)
		if e != nil || len(encoded)+1 > maxSize || ctx.Err() != nil {
			return nil, operationFailure(errServer, mutating)
		}
		if _, _, e = s.authenticate(ctx, token, key); e != nil {
			if mutating {
				return nil, "outcome_unknown"
			}
			return nil, "unauthorized"
		}
		return append(encoded, '\n'), ""
	}()
	if failure != "" {
		writeDailyFailure(w, failure)
		return
	}
	// Slow readers cannot occupy the hash/account operation slot after the
	// synchronized operation and bounded encoding are complete.
	publicHeaders(w)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
