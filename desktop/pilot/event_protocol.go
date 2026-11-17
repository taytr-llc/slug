// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"strings"
	"time"
)

func eventProtocolError(err error) string {
	for _, e := range []error{gymevent.ErrInvalid, gymevent.ErrUnauthorized, gymevent.ErrForbidden, gymevent.ErrStaleView, gymevent.ErrBusy, gymevent.ErrUnavailable, gymevent.ErrCorrupt, gymevent.ErrCapacity, gymevent.ErrClock, gymevent.ErrCalendar, gymevent.ErrNotDue, gymevent.ErrCreatedTooLate, gymevent.ErrIncomplete, gymevent.ErrConflict, gymevent.ErrNotFound, gymevent.ErrCancelled, gymevent.ErrOutcomeUnknown, errEventSetup} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "event_cancelled"
	}
	switch stableProtocolError(err) {
	case "unauthorized", "forbidden", "stale_view", "busy", "invalid_request":
		return stableProtocolError(err)
	}
	return "event_unavailable"
}

var errEventSetup = errors.New("event_setup_incomplete")

func handleEventRequest(ctx context.Context, r EventRequest, c EventControllerAPI) EventResponse {
	out := EventResponse{ID: r.ID, OK: true}
	var err error
	switch r.Command {
	case "event_snapshot":
		v := c.EventSnapshot()
		out.Snapshot = &v
		out.View = v.View
		out.StoreID = v.StoreID
	case "event_status":
		v, e := c.EventStatus(ctx, r.ViewID, r.Saturday)
		out.Status, err = &v, e
	case "event_record":
		v, e := c.EventRecord(ctx, r.ViewID, r.Saturday)
		out.Record, err = &v, e
	case "event_history":
		v, e := c.EventHistory(ctx, r.ViewID, r.AfterEventID, r.Limit)
		out.History, err = &v, e
	case "event_capture":
		v, e := c.BeginEventCapture(ctx, r.ViewID, r.Saturday)
		out.Job, err = &v, e
	case "event_job":
		v, e := c.EventJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	case "event_cancel":
		v, e := c.CancelEventJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	default:
		err = gymevent.ErrInvalid
	}
	if err != nil {
		return EventResponse{ID: r.ID, Error: eventProtocolError(err)}
	}
	if r.Command != "event_snapshot" {
		snap := c.EventSnapshot()
		if snap.View == nil || snap.View.ViewID != r.ViewID || snap.StoreID == "" {
			code := "stale_view"
			if r.Command == "event_capture" {
				code = "outcome_unknown"
			}
			return EventResponse{ID: r.ID, Error: code}
		}
		out.View = snap.View
		out.StoreID = snap.StoreID
	}
	return out
}

// ServeEvents adds intake without changing any previous family grammar.
func ServeEvents(ctx context.Context, c EventControllerAPI, input io.Reader, output io.Writer) (resultErr error) {
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
			if strings.HasPrefix(selector.Command, "event_") {
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
