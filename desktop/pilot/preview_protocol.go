// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"strings"
	"time"
)

func previewProtocolError(e error) string {
	switch {
	case errors.Is(e, gymstable.ErrNotFound):
		return "preview_not_found"
	case errors.Is(e, gymstable.ErrClassMismatch):
		return "preview_class_mismatch"
	case errors.Is(e, gymstable.ErrCapacity):
		return "preview_capacity"
	case errors.Is(e, gymstable.ErrCancelled), errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded):
		return "preview_cancelled"
	case errors.Is(e, gymstable.ErrOutcomeUnknown):
		return "preview_observation_unknown"
	}
	code := stableProtocolError(e)
	if previewErrorCode(code) {
		return code
	}
	return "preview_unavailable"
}
func handlePreviewRequest(ctx context.Context, r PreviewRequest, c PreviewControllerAPI) PreviewResponse {
	out := PreviewResponse{ID: r.ID, Version: 1, OK: true}
	var err error
	switch r.Command {
	case "preview_snapshot":
		v := c.PreviewSnapshot()
		out.Snapshot = &v
	case "preview_begin":
		v, e := c.BeginFightPreview(ctx, r.ViewID, r.FighterAID, r.FighterBID)
		out.Job, err = &v, e
	case "preview_job":
		v, e := c.FightPreviewJob(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	case "preview_cancel":
		v, e := c.CancelFightPreview(ctx, r.ViewID, r.JobID)
		out.Job, err = &v, e
	default:
		err = gymstable.ErrInvalid
	}
	if err != nil {
		return PreviewResponse{ID: r.ID, Version: 1, Error: previewProtocolError(err)}
	}
	return out
}

// ServePreview adds a closed command family on the existing owned helper pipe.
func ServePreview(ctx context.Context, input io.Reader, output io.Writer, c PreviewControllerAPI) (resultErr error) {
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
			if strings.HasPrefix(selector.Command, "preview_") {
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
