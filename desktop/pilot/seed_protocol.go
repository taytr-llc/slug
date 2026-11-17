// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"strings"
	"time"
)

func seedProtocolError(err error) string {
	if errors.Is(err, gymaccount.ErrHeld) || errors.Is(err, gymaccount.ErrForbidden) {
		return "forbidden"
	}
	if errors.Is(err, gymaccount.ErrUnauthorized) || errors.Is(err, gymaccount.ErrExpired) {
		return "unauthorized"
	}
	if err != nil && seedErrorCode(err.Error()) {
		return err.Error()
	}
	for _, e := range []error{gymseed.ErrInvalid, gymseed.ErrUnauthorized, gymseed.ErrForbidden, gymseed.ErrStaleView, gymseed.ErrBusy, gymseed.ErrUnavailable, gymseed.ErrCorrupt, gymseed.ErrCapacity, gymseed.ErrClock, gymseed.ErrCalendar, gymseed.ErrCreatedTooLate, gymseed.ErrSourceIncomplete, gymseed.ErrNotFound, gymseed.ErrNotPrepared, gymseed.ErrNodeOffline, gymseed.ErrWaitingForFunds, gymseed.ErrScanIncomplete, gymseed.ErrConflict, gymseed.ErrTargetUnavailable, gymseed.ErrNotMature, gymseed.ErrCancelled, gymseed.ErrOutcomeUnknown, errSeedSetup} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "seed_cancelled"
	}
	switch stableProtocolError(err) {
	case "unauthorized", "forbidden", "stale_view", "busy", "invalid_request":
		return stableProtocolError(err)
	}
	return "seed_unavailable"
}

var errSeedSetup = errors.New("seed_setup_incomplete")

func handleSeedRequest(ctx context.Context, r SeedRequest, c SeedControllerAPI) SeedResponse {
	out := SeedResponse{ID: r.ID, OK: true}
	var err error
	switch r.Command {
	case "seed_snapshot":
		v := c.SeedSnapshot()
		out.Snapshot = &v
		out.View = v.View
		out.StoreID = v.StoreID
	case "seed_status":
		v, e := c.SeedStatus(ctx, r.ViewID, r.Saturday)
		out.Status, err = &v, e
	case "seed_record":
		v, e := c.SeedRecord(ctx, r.ViewID, r.Saturday)
		out.Record, err = &v, e
	case "seed_history":
		v, e := c.SeedHistory(ctx, r.ViewID, r.AfterEventKey, r.Limit)
		out.History, err = &v, e
	case "seed_prepare":
		v, e := c.BeginSeedPrepare(ctx, r.ViewID, r.Saturday)
		out.Job, err = &v, e
	case "seed_publish":
		v, e := c.BeginSeedPublish(ctx, r.ViewID, r.Saturday)
		out.Job, err = &v, e
	case "seed_reconcile":
		v, e := c.BeginSeedReconcile(ctx, r.ViewID, r.Saturday)
		out.Job, err = &v, e
	case "seed_attest":
		v, e := c.BeginSeedAttest(ctx, r.ViewID, r.Saturday)
		out.Job, err = &v, e
	case "seed_job":
		v, e := c.SeedJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	case "seed_cancel":
		v, e := c.CancelSeedJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	default:
		err = gymseed.ErrInvalid
	}
	if err != nil {
		return SeedResponse{ID: r.ID, Error: seedProtocolError(err)}
	}
	if r.Command != "seed_snapshot" {
		snap := c.SeedSnapshot()
		if snap.View == nil || snap.View.ViewID != r.ViewID || snap.StoreID == "" {
			code := "stale_view"
			if r.Command == "seed_prepare" || r.Command == "seed_publish" || r.Command == "seed_attest" || r.Command == "seed_reconcile" {
				code = "outcome_unknown"
			}
			return SeedResponse{ID: r.ID, Error: code}
		}
		out.View = snap.View
		out.StoreID = snap.StoreID
	}
	return out
}

// ServeSeeds adds one closed family without changing previous wire grammars.
func ServeSeeds(ctx context.Context, c SeedControllerAPI, input io.Reader, output io.Writer) (resultErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type line struct {
		data []byte
		err  error
	}
	lines := make(chan line, 8)
	readerDone, shutdownDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-ctx.Done():
		case <-readerDone:
		}
		c.Stop()
		if x, ok := input.(io.Closer); ok {
			_ = x.Close()
		}
		if x, ok := output.(io.Closer); ok {
			_ = x.Close()
		}
		_ = c.Close()
	}()
	defer func() { cancel(); <-shutdownDone; resultErr = errors.Join(resultErr, c.Close()) }()
	go func() {
		defer close(readerDone)
		defer close(lines)
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 1024), MaxRequestSize+1)
		for scanner.Scan() {
			value := line{data: append([]byte(nil), scanner.Bytes()...)}
			select {
			case lines <- value:
			case <-ctx.Done():
				return
			default:
				return
			}
		}
		select {
		case lines <- line{err: scanner.Err()}:
		case <-ctx.Done():
		default:
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case value, open := <-lines:
			if !open || value.data == nil {
				return value.err
			}
			var selector struct {
				ID      uint64 `json:"id"`
				Command string `json:"command"`
			}
			if json.Unmarshal(value.data, &selector) != nil {
				return gymstable.ErrInvalid
			}
			if strings.HasPrefix(selector.Command, "seed_") {
				req, err := ParseSeedRequest(value.data)
				response := SeedResponse{ID: selector.ID, Error: "invalid_request"}
				if err == nil {
					response = handleSeedRequest(ctx, req, c)
				}
				raw, err := MarshalSeedResponse(response)
				if err != nil {
					return err
				}
				if d, ok := output.(interface{ SetWriteDeadline(time.Time) error }); ok {
					_ = d.SetWriteDeadline(time.Now().Add(5 * time.Second))
				}
				raw = append(raw, '\n')
				n, err := output.Write(raw)
				if err != nil {
					return err
				}
				if n != len(raw) {
					return io.ErrShortWrite
				}
			} else if strings.HasPrefix(selector.Command, "event_") {
				req, err := ParseEventRequest(value.data)
				response := EventResponse{ID: selector.ID, Error: "invalid_request"}
				if err == nil {
					response = handleEventRequest(ctx, req, c)
				}
				raw, err := MarshalEventResponse(response)
				if err != nil {
					return err
				}
				if d, ok := output.(interface{ SetWriteDeadline(time.Time) error }); ok {
					_ = d.SetWriteDeadline(time.Now().Add(5 * time.Second))
				}
				raw = append(raw, '\n')
				n, err := output.Write(raw)
				if err != nil {
					return err
				}
				if n != len(raw) {
					return io.ErrShortWrite
				}
			} else if strings.HasPrefix(selector.Command, "entry_") {
				req, err := ParseEntryRequest(value.data)
				response := EntryResponse{ID: selector.ID, Error: "invalid_request"}
				if err == nil {
					response = handleEntryRequest(ctx, req, c)
				}
				raw, err := MarshalEntryResponse(response)
				if err != nil {
					return err
				}
				if d, ok := output.(interface{ SetWriteDeadline(time.Time) error }); ok {
					_ = d.SetWriteDeadline(time.Now().Add(5 * time.Second))
				}
				raw = append(raw, '\n')
				n, err := output.Write(raw)
				if err != nil {
					return err
				}
				if n != len(raw) {
					return io.ErrShortWrite
				}
			} else if strings.HasPrefix(selector.Command, "preview_") {
				req, err := ParsePreviewRequest(value.data)
				response := PreviewResponse{ID: selector.ID, Version: 1, Error: "invalid_request"}
				if err == nil {
					response = handlePreviewRequest(ctx, req, c)
				}
				raw, err := MarshalPreviewResponse(response)
				if err != nil {
					return err
				}
				if d, ok := output.(interface{ SetWriteDeadline(time.Time) error }); ok {
					_ = d.SetWriteDeadline(time.Now().Add(5 * time.Second))
				}
				raw = append(raw, '\n')
				n, err := output.Write(raw)
				if err != nil {
					return err
				}
				if n != len(raw) {
					return io.ErrShortWrite
				}
			} else if strings.HasPrefix(selector.Command, "stable_") {
				req, err := ParseStableRequest(value.data)
				var response StableResponse
				if err != nil {
					if selector.ID == 0 || selector.ID > gymstable.MaxSafeInteger {
						return gymstable.ErrInvalid
					}
					response = StableResponse{ID: selector.ID, Error: "invalid_request"}
				} else {
					response = handleStableRequest(ctx, req, c)
				}
				if err = respondStable(output, response); err != nil {
					return err
				}
			} else {
				req, err := parseManifestRequest(value.data)
				var response ManifestResponse
				if err != nil {
					response = ManifestResponse{ID: req.ID, Error: "invalid_request"}
				} else {
					response = handleManifestRequest(ctx, req, c)
				}
				if err = respondManifest(output, response); err != nil {
					return err
				}
			}
		}
	}
}
