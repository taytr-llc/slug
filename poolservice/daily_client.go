// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"time"
)

func (c *Client) dailyWork(ctx context.Context, id string, issue bool) (poolshare.MiningWork, error) {
	if c.recoverable != nil && issue {
		return poolshare.MiningWork{}, &ClientError{Code: "invalid_request"}
	}
	if err := c.ready(ctx); err != nil {
		return poolshare.MiningWork{}, err
	}
	path := "issue"
	var request any = struct{}{}
	if !issue {
		if !hex64(id, false) {
			return poolshare.MiningWork{}, &ClientError{Code: "invalid_request"}
		}
		path = "work"
		request = workRequest{id}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := c.post(ctx, path, request, issue, MaxWorkResponseSize)
	if err != nil {
		return poolshare.MiningWork{}, err
	}
	var r DailyWorkResponse
	if c.decodeSelectedWorkResponse(raw, &r) != nil || !c.dailyBinding(r.Version, r.BindingID, r.SettlementConfigID, r.RecipientScript) || !issue && r.Work.Job.ID != id {
		return poolshare.MiningWork{}, failure(issue)
	}
	if _, err = PrepareDailyWork(c.config.Source, c.config.PublicKey, r.Work); err != nil || ctx.Err() != nil {
		return poolshare.MiningWork{}, failure(issue)
	}
	return r.Work, nil
}
func (c *Client) dailySubmission(ctx context.Context, w poolshare.MiningWork, nonce uint32, submit bool) (poolshare.Submission, error) {
	if err := c.ready(ctx); err != nil {
		return poolshare.Submission{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	prepared, err := PrepareDailyWork(c.config.Source, c.config.PublicKey, w)
	if err != nil {
		return poolshare.Submission{}, &ClientError{Code: "invalid_request"}
	}
	check, err := prepared.CheckNonce(ctx, nonce)
	if err != nil {
		return poolshare.Submission{}, failure(false)
	}
	if !check.Pool && !check.Block {
		return poolshare.Submission{}, &ClientError{Code: "rejected"}
	}
	path := "lookup"
	if submit {
		path = "submit"
	}
	raw, err := c.post(ctx, path, submissionRequest{w.Job.ID, nonce}, submit, MaxResponseSize)
	if err != nil {
		return poolshare.Submission{}, err
	}
	var r DailySubmissionResponse
	if c.decodeSelectedSubmissionResponse(raw, &r) != nil || !c.dailyBinding(r.Version, r.BindingID, r.SettlementConfigID, r.RecipientScript) || !SubmissionMatchesDaily(c.config.Source, w, nonce, check, r.Submission) || ctx.Err() != nil {
		return poolshare.Submission{}, failure(submit)
	}
	return r.Submission, nil
}

// DailyBalance preserves the lossless wire counters; Authority.Balance is only
// an in-process projection after these complete immutable binding checks.
func (c *Client) DailyBalance(ctx context.Context) (DailyBalanceResponse, error) {
	if c == nil || c.daily == nil {
		return DailyBalanceResponse{}, &ClientError{Code: "invalid_request"}
	}
	if err := c.ready(ctx); err != nil {
		return DailyBalanceResponse{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := c.post(ctx, "balance", struct{}{}, false, MaxResponseSize)
	if err != nil {
		return DailyBalanceResponse{}, err
	}
	var r DailyBalanceResponse
	if c.decodeSelectedBalance(raw, &r) != nil || !c.dailyBinding(r.Version, r.BindingID, r.SettlementConfigID, r.RecipientScript) || poolsettle.ValidateDailyBalance(r.Balance, c.gym, r.ObservedAtMS) != nil || ctx.Err() != nil {
		return DailyBalanceResponse{}, failure(false)
	}
	return r, nil
}

// PayoutsPage reads exactly one captured eight-slot projection. Empty Items is
// not EOF; callers use Done/NextBefore and retain the captured revision.
func (c *Client) PayoutsPage(ctx context.Context, q poolsettle.DailyPayoutRequest) (poolsettle.DailyPayoutPage, error) {
	if c == nil || c.daily == nil || poolsettle.ValidateDailyPayoutRequest(q, poolsettle.MaxDailyPayoutBatches) != nil {
		return poolsettle.DailyPayoutPage{}, &ClientError{Code: "invalid_request"}
	}
	if err := c.ready(ctx); err != nil {
		return poolsettle.DailyPayoutPage{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := c.post(ctx, "payouts", q, false, MaxPayoutResponseSize)
	if err != nil {
		var remote *ClientError
		if !errors.As(err, &remote) || remote.OutcomeUnknown || (!payoutErrorCode(remote.Code) && remote.Code != "stale_capture") {
			return poolsettle.DailyPayoutPage{}, failure(false)
		}
		return poolsettle.DailyPayoutPage{}, err
	}
	var r poolsettle.DailyPayoutPage
	if c.decodeSelectedPage(raw, &r) != nil || !c.dailyBinding(r.Version, r.BindingID, r.SettlementConfigID, r.RecipientScript) {
		return poolsettle.DailyPayoutPage{}, failure(false)
	}
	if q.Revision == 0 {
		if r.BeforeBatch != r.HighBatch+1 {
			return poolsettle.DailyPayoutPage{}, failure(false)
		}
	} else if r.Revision != q.Revision || r.HighBatch != q.HighBatch || r.BeforeBatch != q.BeforeBatch {
		return poolsettle.DailyPayoutPage{}, failure(false)
	}
	r, err = poolsettle.ValidateDailyPayoutPage(r, c.gym)
	if err != nil || ctx.Err() != nil {
		return poolsettle.DailyPayoutPage{}, failure(false)
	}
	return r, nil
}
