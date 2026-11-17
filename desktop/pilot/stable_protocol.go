// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

func stableProtocolError(err error) string {
	for _, e := range []error{gymstable.ErrInvalid, gymstable.ErrUnauthorized, gymstable.ErrForbidden, gymstable.ErrStaleView, gymstable.ErrBusy, gymstable.ErrCorrupt, gymstable.ErrCapacity, gymstable.ErrClock, gymstable.ErrSyncRequired, gymstable.ErrConflict, gymstable.ErrPending, gymstable.ErrNotFound, gymstable.ErrCancelled, gymstable.ErrOutcomeUnknown} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "stable_cancelled"
	}
	return "stable_unavailable"
}
func handleStableRequest(ctx context.Context, r StableRequest, c StableControllerAPI) StableResponse {
	result := StableResponse{ID: r.ID, OK: true}
	var err error
	switch r.Command {
	case "stable_snapshot":
		v := c.StableSnapshot()
		result.Snapshot = &v
	case "stable_recent":
		v, e := c.BeginStableRecent(ctx, r.ViewID)
		result.Job, err = &v, e
	case "stable_sync":
		v, e := c.BeginStableSync(ctx, r.ViewID)
		result.Job, err = &v, e
	case "stable_inventory":
		v, e := c.BeginStableInventory(ctx, r.ViewID, r.After, r.Limit)
		result.Job, err = &v, e
	case "stable_roster":
		v, e := c.BeginStableRoster(ctx, r.ViewID, r.FirstSlot, r.Limit)
		result.Job, err = &v, e
	case "stable_prepare_slot":
		v, e := c.BeginStablePrepare(ctx, r.ViewID, gymstable.SlotRequest{ExpectedRevision: r.ExpectedRevision, Slot: r.Slot, ProspectID: r.ProspectID})
		result.Job, err = &v, e
	case "stable_apply_slot":
		v, e := c.BeginStableApply(ctx, r.ViewID, r.OperationID)
		result.Job, err = &v, e
	case "stable_job":
		v, e := c.StableJob(ctx, r.ViewID, r.JobID)
		result.Job, err = &v, e
	case "stable_operation":
		v, e := c.StableOperation(ctx, r.ViewID, r.OperationID)
		result.Operation, err = &v, e
	case "stable_pending":
		v, e := c.StablePending(ctx, r.ViewID)
		result.Operation, err = &v, e
	case "stable_abandon_operation":
		v, e := c.AbandonStableOperation(ctx, r.ViewID, r.OperationID)
		result.Operation, err = &v, e
	case "stable_cancel":
		v, e := c.CancelStableJob(ctx, r.ViewID, r.JobID)
		result.Job, err = &v, e
	default:
		err = gymstable.ErrInvalid
	}
	if err != nil {
		return StableResponse{ID: r.ID, Error: stableProtocolError(err)}
	}
	return result
}

// Bound typed controller output before allocation; private Item receipts are
// not JSON fields. Depth counts containers, not Go pointers/scalar values.
func stableResponseBudget(v reflect.Value, budget *int, depth int) bool {
	spend := func(n int) bool {
		if n < 0 || n > *budget {
			return false
		}
		*budget -= n
		return true
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return spend(4)
		}
		return stableResponseBudget(v.Elem(), budget, depth)
	case reflect.Struct:
		if depth >= 9 || !spend(2) {
			return false
		}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			tag := strings.Split(f.Tag.Get("json"), ",")
			if tag[0] == "-" || len(tag) > 1 && tag[1] == "omitempty" && v.Field(i).IsZero() {
				continue
			}
			if !spend(len(tag[0])+4) || !stableResponseBudget(v.Field(i), budget, depth+1) {
				return false
			}
		}
	case reflect.Array, reflect.Slice:
		maxCount := 8
		if v.Kind() == reflect.Array {
			maxCount = 10
		}
		if depth >= 9 || v.Len() > maxCount || !spend(2) {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !spend(1) || !stableResponseBudget(v.Index(i), budget, depth+1) {
				return false
			}
		}
	case reflect.String:
		s := v.String()
		return len(s) <= 4096 && utf8.ValidString(s) && len(s) <= (*budget-2)/6 && spend(2+6*len(s))
	case reflect.Bool:
		return spend(5)
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		return spend(32)
	default:
		return false
	}
	return true
}
func respondStable(writer io.Writer, r StableResponse) error {
	budget := gymstable.MaxResponseSize - 1
	if !stableResponseBudget(reflect.ValueOf(r), &budget, 0) {
		return gymstable.ErrInvalid
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return gymstable.ErrInvalid
	}
	if _, err = ParseStableResponse(raw); err != nil {
		return err
	}
	if d, ok := writer.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = d.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}
	raw = append(raw, '\n')
	n, err := writer.Write(raw)
	if err == nil && n != len(raw) {
		err = io.ErrShortWrite
	}
	return err
}

// ServeStable dispatches exact old/new command parsers; it never parses a
// permissive old/new response union. Shutdown drains the owned controller.
func ServeStable(ctx context.Context, input io.Reader, output io.Writer, c StableControllerAPI) (resultErr error) {
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
			if strings.HasPrefix(selector.Command, "stable_") {
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
