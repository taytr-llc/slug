// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

type ManifestRequest struct {
	ID         uint64 `json:"id"`
	Command    string `json:"command"`
	Sequence   uint64 `json:"sequence,omitempty"`
	Path       string `json:"path,omitempty"`
	Fixture    string `json:"fixture,omitempty"`
	ViewID     string `json:"view_id,omitempty"`
	ProspectID string `json:"prospect_id,omitempty"`
}
type ManifestResponse struct {
	ID              uint64            `json:"id"`
	OK              bool              `json:"ok"`
	Snapshot        *ManifestSnapshot `json:"snapshot,omitempty"`
	Portrait        *ManifestPortrait `json:"portrait,omitempty"`
	ManifestReceipt *ManifestReceipt  `json:"manifest_receipt,omitempty"`
	Error           string            `json:"error,omitempty"`
}

// ManifestControllerAPI is separate from the old protocol's controller and
// receipt types. Native-only paths and selections never enter the webview API.
type ManifestControllerAPI interface {
	Start() error
	Stop()
	Close() error
	Snapshot() ManifestSnapshot
	LoginFixture(string) error
	Logout() error
	ResetFixtureAccount() error
	RefreshManifestProspects(context.Context) error
	ManifestPortrait(context.Context, uint64) (ManifestPortrait, error)
	ManifestReceipt(context.Context, uint64) (ManifestReceipt, error)
	ExportManifestReceipt(context.Context, uint64, string, string, string) error
	ReleaseManifestReceipt(context.Context, string, string) error
	ExportRecovery(context.Context, string) error
	VerifyBackup(context.Context, string) error
}

func canonicalManifestID(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func parseManifestRequest(raw []byte) (ManifestRequest, error) {
	var req ManifestRequest
	invalid := errors.New("invalid_request")
	if len(raw) == 0 || len(raw) > MaxRequestSize || !utf8.Valid(raw) {
		return req, invalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return req, invalid
	}
	fields := make(map[string]json.RawMessage, 7)
	for d.More() {
		token, err := d.Token()
		name, ok := token.(string)
		if err != nil || !ok {
			return req, invalid
		}
		if _, exists := fields[name]; exists {
			return req, invalid
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return req, invalid
		}
		fields[name] = value
	}
	if _, err := d.Token(); err != nil || d.Decode(new(any)) != io.EOF {
		return req, invalid
	}
	if json.Unmarshal(fields["id"], &req.ID) != nil || req.ID == 0 || req.ID > 1<<53-1 || json.Unmarshal(fields["command"], &req.Command) != nil {
		return req, invalid
	}
	for name := range fields {
		switch name {
		case "id", "command", "sequence", "path", "fixture", "view_id", "prospect_id":
		default:
			return req, invalid
		}
	}
	allowed := map[string]bool{"id": true, "command": true}
	sequence := false
	switch req.Command {
	case "start", "stop", "snapshot", "logout", "reset_fixture", "refresh_manifest_prospects":
	case "login_fixture":
		allowed["fixture"] = true
	case "manifest_portrait", "manifest_receipt":
		sequence = true
	case "export_manifest_receipt":
		sequence = true
		allowed["path"], allowed["view_id"], allowed["prospect_id"] = true, true, true
	case "release_manifest_receipt":
		allowed["view_id"], allowed["prospect_id"] = true, true
	case "export_recovery", "verify_backup":
		allowed["path"] = true
	default:
		return req, invalid
	}
	if sequence {
		allowed["sequence"] = true
	}
	for name := range fields {
		if !allowed[name] {
			return req, invalid
		}
	}
	if sequence && (json.Unmarshal(fields["sequence"], &req.Sequence) != nil || req.Sequence == 0 || req.Sequence > MaxManifestSequence) {
		return req, invalid
	}
	if allowed["path"] && (json.Unmarshal(fields["path"], &req.Path) != nil || !ValidRecoveryPath(req.Path)) {
		return req, invalid
	}
	if allowed["fixture"] && (json.Unmarshal(fields["fixture"], &req.Fixture) != nil || (req.Fixture != "zero256" && req.Fixture != "incrementing256")) {
		return req, invalid
	}
	if allowed["view_id"] && (json.Unmarshal(fields["view_id"], &req.ViewID) != nil || !canonicalManifestID(req.ViewID)) {
		return req, invalid
	}
	if allowed["prospect_id"] && (json.Unmarshal(fields["prospect_id"], &req.ProspectID) != nil || !canonicalManifestID(req.ProspectID)) {
		return req, invalid
	}
	return req, nil
}

func manifestProtocolError(err error, fallback string) string {
	for _, candidate := range []error{ErrManifestProjection, ErrManifestProjectionCorrupt, ErrManifestProjectionCancelled, ErrManifestProjectionRequired, ErrManifestProspect, ErrManifestPortrait, ErrManifestReceipt, ErrManifestInitialization} {
		if errors.Is(err, candidate) {
			return candidate.Error()
		}
	}
	return fallback
}

func handleManifestRequest(ctx context.Context, req ManifestRequest, c ManifestControllerAPI) ManifestResponse {
	result := ManifestResponse{ID: req.ID, OK: true}
	var err error
	code := "account_unavailable"
	switch req.Command {
	case "snapshot":
	case "start":
		err, code = c.Start(), "start_unavailable"
	case "stop":
		c.Stop()
	case "login_fixture":
		err = c.LoginFixture(req.Fixture)
	case "logout":
		err = c.Logout()
	case "reset_fixture":
		err = c.ResetFixtureAccount()
	case "refresh_manifest_prospects":
		err, code = c.RefreshManifestProspects(ctx), "projection_unavailable"
	case "manifest_portrait":
		value, e := c.ManifestPortrait(ctx, req.Sequence)
		err, code = e, "portrait_unavailable"
		if e == nil {
			result.Portrait = &value
		}
	case "manifest_receipt":
		value, e := c.ManifestReceipt(ctx, req.Sequence)
		err, code = e, "manifest_receipt_unavailable"
		if e == nil {
			result.ManifestReceipt = &value
		}
	case "export_manifest_receipt":
		err, code = c.ExportManifestReceipt(ctx, req.Sequence, req.ViewID, req.ProspectID, req.Path), "export_unavailable"
	case "release_manifest_receipt":
		err, code = c.ReleaseManifestReceipt(ctx, req.ViewID, req.ProspectID), "manifest_receipt_unavailable"
	case "export_recovery":
		err, code = c.ExportRecovery(ctx, req.Path), "export_unavailable"
	case "verify_backup":
		err, code = c.VerifyBackup(ctx, req.Path), "backup_unavailable"
	default:
		err, code = errors.New("invalid_request"), "invalid_request"
	}
	if err != nil {
		return ManifestResponse{ID: req.ID, Error: manifestProtocolError(err, code)}
	}
	if result.Portrait == nil && result.ManifestReceipt == nil {
		value := c.Snapshot()
		result.Snapshot = &value
	}
	return result
}

// Preflight a conservative escaped-JSON byte budget without allocating the
// encoded response. All accepted types have fixed depth; only bounded slices
// and strings vary. Actual encoding is independently checked below.
func manifestResponseBudget(value reflect.Value, budget *int, depth int) bool {
	if depth > 8 || *budget < 0 {
		return false
	}
	spend := func(n int) bool {
		if n < 0 || n > *budget {
			return false
		}
		*budget -= n
		return true
	}
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return spend(4)
		}
		return manifestResponseBudget(value.Elem(), budget, depth+1)
	case reflect.Struct:
		if !spend(2) {
			return false
		}
		t := value.Type()
		for i := 0; i < value.NumField(); i++ {
			tag := strings.Split(t.Field(i).Tag.Get("json"), ",")
			if len(tag) > 1 && tag[1] == "omitempty" && value.Field(i).IsZero() {
				continue
			}
			if !spend(len(tag[0])+4) || !manifestResponseBudget(value.Field(i), budget, depth+1) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if value.Len() > 20 || !spend(2) {
			return false
		}
		for i := 0; i < value.Len(); i++ {
			if !spend(1) || !manifestResponseBudget(value.Index(i), budget, depth+1) {
				return false
			}
		}
		return true
	case reflect.String:
		s := value.String()
		return len(s) <= (*budget-2)/6 && utf8.ValidString(s) && spend(2+6*len(s))
	case reflect.Bool:
		return spend(5)
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Float64:
		return spend(32)
	default:
		return false
	}
}

func respondManifest(writer io.Writer, response ManifestResponse) error {
	budget := MaxResponseSize - 1
	if (response.Snapshot != nil && len(response.Snapshot.Projection.Items) > MaxManifestProspects) ||
		(response.Portrait != nil && len(response.Portrait.SVG) > 32768) ||
		!manifestResponseBudget(reflect.ValueOf(response), &budget, 0) {
		return errors.New("response unavailable")
	}
	raw, err := json.Marshal(response)
	if err != nil || len(raw)+1 > MaxResponseSize {
		return errors.New("response unavailable")
	}
	if deadline, ok := writer.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = deadline.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}
	raw = append(raw, '\n')
	n, err := writer.Write(raw)
	if err == nil && n != len(raw) {
		err = io.ErrShortWrite
	}
	return err
}

// ServeManifest owns actual controller drainage even when stdout is blocked.
// As with the old helper, production input/output are closable process pipes.
func ServeManifest(ctx context.Context, input io.Reader, output io.Writer, c ManifestControllerAPI) (resultErr error) {
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
		if closer, ok := input.(io.Closer); ok {
			_ = closer.Close()
		}
		if closer, ok := output.(io.Closer); ok {
			_ = closer.Close()
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
			if !open {
				return nil
			}
			if value.data == nil {
				if value.err != nil {
					_ = respondManifest(output, ManifestResponse{Error: "invalid_frame"})
				}
				return value.err
			}
			req, err := parseManifestRequest(value.data)
			if err != nil {
				if err = respondManifest(output, ManifestResponse{ID: req.ID, Error: "invalid_request"}); err != nil {
					return err
				}
				continue
			}
			if err = respondManifest(output, handleManifestRequest(ctx, req, c)); err != nil {
				return err
			}
		}
	}
}
