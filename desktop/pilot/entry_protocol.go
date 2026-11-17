// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"strings"
	"time"
)

func entryProtocolError(err error) string {
	for _, e := range []error{gymentry.ErrInvalid, gymentry.ErrUnauthorized, gymentry.ErrForbidden, gymentry.ErrStaleView, gymentry.ErrBusy, gymentry.ErrUnavailable, gymentry.ErrCorrupt, gymentry.ErrCapacity, gymentry.ErrClock, gymentry.ErrCalendar, gymentry.ErrClosed, gymentry.ErrConflict, gymentry.ErrPending, gymentry.ErrNotFound, gymentry.ErrSyncRequired, gymentry.ErrNotRostered, gymentry.ErrRetired, gymentry.ErrClassConflict, gymentry.ErrCancelled, gymentry.ErrOutcomeUnknown} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "entry_cancelled"
	}
	switch stableProtocolError(err) {
	case "unauthorized", "forbidden", "stale_view", "busy", "invalid_request":
		return stableProtocolError(err)
	}
	return "entry_unavailable"
}
func handleEntryRequest(ctx context.Context, r EntryRequest, c EntryControllerAPI) EntryResponse {
	out := EntryResponse{ID: r.ID, OK: true}
	var err error
	switch r.Command {
	case "entry_snapshot":
		v := c.EntrySnapshot()
		out.Snapshot = &v
		if v.View != nil {
			out.View = v.View
			out.StoreID = v.StoreID
		}
	case "entry_upcoming":
		v, e := c.EntryUpcoming(ctx, r.ViewID)
		out.Status, err = &v, e
	case "entry_status":
		v, e := c.EntryStatus(ctx, r.ViewID, r.Saturday)
		out.Status, err = &v, e
	case "entry_history":
		v, e := c.EntryHistory(ctx, r.ViewID, r.AfterEventID, r.Limit)
		out.History, err = &v, e
	case "entry_prepare":
		v, e := c.BeginEntryPrepare(ctx, r.ViewID, *r.Request)
		out.Job, err = &v, e
	case "entry_apply":
		v, e := c.BeginEntryApply(ctx, r.ViewID, r.OperationID)
		out.Job, err = &v, e
	case "entry_check":
		v, e := c.BeginEntryCheck(ctx, r.ViewID, r.Saturday)
		out.Job, err = &v, e
	case "entry_job":
		v, e := c.EntryJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	case "entry_cancel":
		v, e := c.CancelEntryJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	case "entry_operation":
		v, e := c.EntryOperation(ctx, r.ViewID, r.OperationID)
		out.Operation, err = &v, e
	case "entry_pending":
		v, e := c.EntryPending(ctx, r.ViewID)
		out.Operation, err = &v, e
	case "entry_abandon":
		v, e := c.AbandonEntryOperation(ctx, r.ViewID, r.OperationID)
		out.Operation, err = &v, e
	default:
		err = gymentry.ErrInvalid
	}
	if err != nil {
		return EntryResponse{ID: r.ID, Error: entryProtocolError(err)}
	}
	if r.Command != "entry_snapshot" {
		snap := c.EntrySnapshot()
		if snap.View == nil || snap.View.ViewID != r.ViewID {
			return EntryResponse{ID: r.ID, Error: entryResponseLostViewCode(r.Command)}
		}
		out.View = snap.View
		out.StoreID = snap.StoreID
	}
	return out
}
func entryResponseLostViewCode(command string) string {
	switch command {
	case "entry_prepare", "entry_apply", "entry_abandon":
		return "outcome_unknown"
	}
	return "stale_view"
}

// ServeEntries adds intake without changing any previous family grammar.
func ServeEntries(ctx context.Context, c EntryControllerAPI, input io.Reader, output io.Writer) (resultErr error) {
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
			if strings.HasPrefix(selector.Command, "entry_") {
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
