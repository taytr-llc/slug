// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"strings"
	"time"
)

func boutProtocolError(err error) string {
	if errors.Is(err, gymaccount.ErrHeld) || errors.Is(err, gymaccount.ErrForbidden) {
		return "forbidden"
	}
	if errors.Is(err, gymaccount.ErrUnauthorized) || errors.Is(err, gymaccount.ErrExpired) {
		return "unauthorized"
	}
	if err != nil && boutErrorCode(err.Error()) {
		return err.Error()
	}
	for _, e := range []error{gymbout.ErrInvalid, gymbout.ErrUnauthorized, gymbout.ErrForbidden, gymbout.ErrStaleView, gymbout.ErrBusy, gymbout.ErrUnavailable, gymbout.ErrCorrupt, gymbout.ErrCapacity, gymbout.ErrClock, gymbout.ErrCalendar, gymbout.ErrSourceIncomplete, gymbout.ErrSourceConflict, gymbout.ErrSourceChanged, gymbout.ErrNotFound, gymbout.ErrCancelled, gymbout.ErrOutcomeUnknown, errBoutSetup} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "bout_cancelled"
	}
	switch stableProtocolError(err) {
	case "unauthorized", "forbidden", "stale_view", "busy", "invalid_request":
		return stableProtocolError(err)
	}
	return "bout_unavailable"
}

var errBoutSetup = errors.New("bout_setup_incomplete")

func handleBoutRequest(ctx context.Context, r BoutRequest, c BoutSurfaceAPI) BoutResponse {
	out := BoutResponse{ID: r.ID, OK: true}
	var err error
	switch r.Command {
	case "bout_snapshot":
		v := c.BoutSnapshot()
		out.Snapshot = &v
		out.View = v.View
		out.StoreID = v.StoreID
	case "bout_card_status":
		v, e := c.BoutCardStatus(ctx, r.ViewID, r.Saturday)
		out.CardStatus, err = &v, e
	case "bout_status":
		v, e := c.BoutStatus(ctx, r.ViewID, r.Saturday, r.BoutID)
		out.Status, err = &v, e
	case "bout_record":
		v, e := c.BoutRecord(ctx, r.ViewID, r.Saturday, r.BoutID)
		out.Record, err = &v, e
	case "bout_run":
		v, e := c.BeginBoutRun(ctx, r.ViewID, r.Saturday, r.BoutID)
		out.Job, err = &v, e
	case "bout_job":
		v, e := c.BoutJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	case "bout_cancel":
		v, e := c.CancelBoutJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	default:
		err = gymbout.ErrInvalid
	}
	if err != nil {
		return BoutResponse{ID: r.ID, Error: boutProtocolError(err)}
	}
	if r.Command != "bout_snapshot" {
		snap := c.BoutSnapshot()
		if snap.View == nil || snap.View.ViewID != r.ViewID || snap.StoreID == "" {
			code := "stale_view"
			if r.Command == "bout_run" || r.Command == "bout_status" || r.Command == "bout_record" || r.Command == "bout_card_status" || r.Command == "bout_cancel" {
				code = "outcome_unknown"
			}
			return BoutResponse{ID: r.ID, Error: code}
		}
		out.View = snap.View
		out.StoreID = snap.StoreID
	}
	return out
}

// ServeBouts adds one closed family without changing previous wire grammars.
func ServeBouts(ctx context.Context, c BoutControllerAPI, input io.Reader, output io.Writer) (resultErr error) {
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
			if strings.HasPrefix(selector.Command, "bout_") {
				req, err := ParseBoutRequest(value.data)
				response := BoutResponse{ID: selector.ID, Error: "invalid_request"}
				if err == nil {
					response = handleBoutRequest(ctx, req, c)
				}
				raw, err := MarshalBoutResponse(response)
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

			} else if strings.HasPrefix(selector.Command, "card_") {
				req, err := ParseCardRequest(value.data)
				response := CardResponse{ID: selector.ID, Error: "invalid_request"}
				if err == nil {
					response = handleCardRequest(ctx, req, c)
				}
				raw, err := MarshalCardResponse(response)
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
			} else if strings.HasPrefix(selector.Command, "seed_") {
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
