// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"strings"
	"time"
)

var errResultSetup = errors.New("result_setup_incomplete")

func resultProtocolError(err error) string {
	// A possibly completed mutation cannot become a definite refusal because a
	// joined cleanup/authentication error happens to precede it in the chain.
	if errors.Is(err, gymresult.ErrOutcomeUnknown) || errors.Is(err, gymstable.ErrOutcomeUnknown) {
		return "outcome_unknown"
	}
	if errors.Is(err, gymaccount.ErrHeld) || errors.Is(err, gymaccount.ErrForbidden) {
		return "forbidden"
	}
	if errors.Is(err, gymaccount.ErrUnauthorized) || errors.Is(err, gymaccount.ErrExpired) {
		return "unauthorized"
	}
	if errors.Is(err, gymevent.ErrCalendar) {
		return "result_calendar"
	}
	for _, e := range []error{gymresult.ErrInvalid, gymresult.ErrUnauthorized, gymresult.ErrForbidden, gymresult.ErrStaleView, gymresult.ErrBusy, gymresult.ErrUnavailable, gymresult.ErrCorrupt, gymresult.ErrCapacity, gymresult.ErrClock, gymresult.ErrCalendar, gymresult.ErrSourceIncomplete, gymresult.ErrSourceChanged, gymresult.ErrSourceConflict, gymresult.ErrNotFound, gymresult.ErrNotPrepared, gymresult.ErrCancelled, gymresult.ErrOutcomeUnknown, gymresult.ErrNodeOffline, gymresult.ErrWaitingForFunds, gymresult.ErrScanIncomplete, gymresult.ErrNotMature, gymresult.ErrConflict, gymresult.ErrProofUnavailable, errResultSetup} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "result_cancelled"
	}
	switch stableProtocolError(err) {
	case "unauthorized", "forbidden", "stale_view", "busy", "invalid_request":
		return stableProtocolError(err)
	}
	return "result_unavailable"
}
func handleResultRequest(ctx context.Context, r ResultRequest, c ResultSurfaceAPI) ResultResponse {
	out := ResultResponse{ID: r.ID, OK: true}
	var err error
	switch r.Command {
	case "result_snapshot":
		v := c.ResultSnapshot()
		out.Snapshot = &v
		out.View = v.View
		out.StoreID = v.StoreID
	case "result_status":
		v, e := c.ResultStatus(ctx, r.ViewID, r.Saturday, r.BoutID)
		out.Status, err = &v, e
	case "result_record":
		v, e := c.ResultRecord(ctx, r.ViewID, r.Saturday, r.BoutID)
		out.Record, err = &v, e
	case "result_publish":
		v, e := c.BeginResultPublish(ctx, r.ViewID, r.Saturday, r.BoutID)
		out.Job, err = &v, e
	case "result_reconcile":
		v, e := c.BeginResultReconcile(ctx, r.ViewID, r.Saturday, r.BoutID)
		out.Job, err = &v, e
	case "result_attest":
		v, e := c.BeginResultAttest(ctx, r.ViewID, r.Saturday, r.BoutID)
		out.Job, err = &v, e
	case "result_export":
		v, e := c.BeginResultExport(ctx, r.ViewID, r.Saturday, r.BoutID, r.Path)
		out.Job, err = &v, e
	case "result_job":
		v, e := c.ResultJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	case "result_cancel":
		v, e := c.CancelResultJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	default:
		err = gymresult.ErrInvalid
	}
	if err != nil {
		return ResultResponse{ID: r.ID, Error: resultProtocolError(err)}
	}
	if r.Command != "result_snapshot" {
		snap := c.ResultSnapshot()
		if snap.View == nil || snap.View.ViewID != r.ViewID || snap.StoreID == "" {
			code := "stale_view"
			if r.Command == "result_publish" || r.Command == "result_status" || r.Command == "result_record" || r.Command == "result_reconcile" || r.Command == "result_attest" || r.Command == "result_export" || r.Command == "result_cancel" {
				code = "outcome_unknown"
			}
			return ResultResponse{ID: r.ID, Error: code}
		}
		out.View = snap.View
		out.StoreID = snap.StoreID
	}
	return out
}

// ServeResults adds one closed family without changing previous wire grammars.
func ServeResults(ctx context.Context, c ResultControllerAPI, input io.Reader, output io.Writer) error {
	return serveResults(ctx, c, input, output, nil)
}
func serveResults(ctx context.Context, c ResultControllerAPI, input io.Reader, output io.Writer, career CareerControllerAPI) (resultErr error) {
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
			if strings.HasPrefix(selector.Command, "fighter_evidence_") {
				req, e := ParseFighterEvidenceRequest(value.data)
				id := selector.ID
				if id == 0 || id > gymstable.MaxSafeInteger {
					id = 1
				}
				response := FighterEvidenceResponse{ID: id, Version: 1, Error: "invalid_request"}
				if surface, ok := career.(FighterEvidenceSurfaceAPI); e == nil && ok {
					response = handleFighterEvidenceRequest(ctx, req, surface)
				}
				raw, e := MarshalFighterEvidenceResponse(response)
				if e != nil {
					return e
				}
				if d, ok := output.(interface{ SetWriteDeadline(time.Time) error }); ok {
					_ = d.SetWriteDeadline(time.Now().Add(5 * time.Second))
				}
				raw = append(raw, '\n')
				n, e := output.Write(raw)
				if e != nil {
					return e
				}
				if n != len(raw) {
					return io.ErrShortWrite
				}
			} else if strings.HasPrefix(selector.Command, "career_history_") {
				req, e := ParseCareerHistoryRequest(value.data)
				id := selector.ID
				if id == 0 || id > gymstable.MaxSafeInteger {
					id = 1
				}
				response := CareerHistoryResponse{ID: id, Version: 1, Error: "invalid_request"}
				if history, ok := career.(CareerHistoryControllerAPI); e == nil && ok {
					response = handleCareerHistoryRequest(ctx, req, history)
				}
				raw, e := MarshalCareerHistoryResponse(response)
				if e != nil {
					return e
				}
				if d, ok := output.(interface{ SetWriteDeadline(time.Time) error }); ok {
					_ = d.SetWriteDeadline(time.Now().Add(5 * time.Second))
				}
				raw = append(raw, '\n')
				n, e := output.Write(raw)
				if e != nil {
					return e
				}
				if n != len(raw) {
					return io.ErrShortWrite
				}
			} else if strings.HasPrefix(selector.Command, "career_preview_") {
				req, e := ParseCareerPreviewRequest(value.data)
				response := CareerPreviewResponse{ID: selector.ID, Version: 1, Error: "invalid_request"}
				if preview, ok := career.(CareerPreviewControllerAPI); e == nil && ok {
					response = handleCareerPreviewRequest(ctx, req, preview)
				}
				raw, e := MarshalCareerPreviewResponse(response)
				if e != nil {
					return e
				}
				if d, ok := output.(interface{ SetWriteDeadline(time.Time) error }); ok {
					_ = d.SetWriteDeadline(time.Now().Add(5 * time.Second))
				}
				raw = append(raw, '\n')
				n, e := output.Write(raw)
				if e != nil {
					return e
				}
				if n != len(raw) {
					return io.ErrShortWrite
				}
			} else if career != nil && strings.HasPrefix(selector.Command, "career_") {
				req, e := ParseCareerRequest(value.data)
				response := CareerResponse{ID: selector.ID, Error: "invalid_request"}
				if e == nil {
					response = handleCareerRequest(ctx, req, career)
				}
				raw, e := MarshalCareerResponse(response)
				if e != nil {
					return e
				}
				if d, ok := output.(interface{ SetWriteDeadline(time.Time) error }); ok {
					_ = d.SetWriteDeadline(time.Now().Add(5 * time.Second))
				}
				raw = append(raw, '\n')
				n, e := output.Write(raw)
				if e != nil {
					return e
				}
				if n != len(raw) {
					return io.ErrShortWrite
				}
			} else if strings.HasPrefix(selector.Command, "result_") {
				req, err := ParseResultRequest(value.data)
				response := ResultResponse{ID: selector.ID, Error: "invalid_request"}
				if err == nil {
					response = handleResultRequest(ctx, req, c)
				}
				raw, err := MarshalResultResponse(response)
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

			} else if strings.HasPrefix(selector.Command, "bout_") {
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
