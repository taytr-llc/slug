// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"strings"
	"time"
)

func cardProtocolError(err error) string {
	if code := careerActionError(err); code != "" {
		return code
	}
	if errors.Is(err, gymaccount.ErrHeld) || errors.Is(err, gymaccount.ErrForbidden) {
		return "forbidden"
	}
	if errors.Is(err, gymaccount.ErrUnauthorized) || errors.Is(err, gymaccount.ErrExpired) {
		return "unauthorized"
	}
	if err != nil && cardErrorCode(err.Error()) {
		return err.Error()
	}
	for _, e := range []error{gymcard.ErrInvalid, gymcard.ErrUnauthorized, gymcard.ErrForbidden, gymcard.ErrStaleView, gymcard.ErrBusy, gymcard.ErrUnavailable, gymcard.ErrCorrupt, gymcard.ErrCapacity, gymcard.ErrClock, gymcard.ErrCalendar, gymcard.ErrSourceIncomplete, gymcard.ErrSourceConflict, gymcard.ErrSourceChanged, gymcard.ErrNotFound, gymcard.ErrCancelled, gymcard.ErrOutcomeUnknown, errCardSetup} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "card_cancelled"
	}
	switch stableProtocolError(err) {
	case "unauthorized", "forbidden", "stale_view", "busy", "invalid_request":
		return stableProtocolError(err)
	}
	return "card_unavailable"
}

var errCardSetup = errors.New("card_setup_incomplete")

func handleCardRequest(ctx context.Context, r CardRequest, c CardControllerAPI) CardResponse {
	out := CardResponse{ID: r.ID, OK: true}
	var err error
	switch r.Command {
	case "card_snapshot":
		v := c.CardSnapshot()
		out.Snapshot = &v
		out.View = v.View
		out.StoreID = v.StoreID
	case "card_status":
		v, e := c.CardStatus(ctx, r.ViewID, r.Saturday)
		out.Status, err = &v, e
	case "card_record":
		v, e := c.CardRecord(ctx, r.ViewID, r.Saturday)
		out.Record, err = &v, e
	case "card_history":
		v, e := c.CardHistory(ctx, r.ViewID, r.AfterCardID, r.Limit)
		out.History, err = &v, e
	case "card_build":
		v, e := c.BeginCardBuild(ctx, r.ViewID, r.Saturday)
		out.Job, err = &v, e
	case "card_job":
		v, e := c.CardJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	case "card_cancel":
		v, e := c.CancelCardJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	default:
		err = gymcard.ErrInvalid
	}
	if err != nil {
		return CardResponse{ID: r.ID, Error: cardProtocolError(err)}
	}
	if r.Command != "card_snapshot" {
		snap := c.CardSnapshot()
		if snap.View == nil || snap.View.ViewID != r.ViewID || snap.StoreID == "" {
			code := "stale_view"
			if r.Command == "card_build" || r.Command == "card_status" || r.Command == "card_record" || r.Command == "card_history" || r.Command == "card_cancel" {
				code = "outcome_unknown"
			}
			return CardResponse{ID: r.ID, Error: code}
		}
		out.View = snap.View
		out.StoreID = snap.StoreID
	}
	return out
}

// ServeCards adds one closed family without changing previous wire grammars.
func ServeCards(ctx context.Context, c CardControllerAPI, input io.Reader, output io.Writer) (resultErr error) {
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
			if strings.HasPrefix(selector.Command, "card_") {
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
