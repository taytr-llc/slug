package pilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/taytr-llc/slug/registry"
)

type Request struct {
	ID       uint64 `json:"id"`
	Command  string `json:"command"`
	Sequence uint64 `json:"sequence,omitempty"`
	Path     string `json:"path,omitempty"`
	Fixture  string `json:"fixture,omitempty"`
}
type Response struct {
	ID       uint64           `json:"id"`
	OK       bool             `json:"ok"`
	Snapshot *Snapshot        `json:"snapshot,omitempty"`
	Receipt  *registry.Record `json:"receipt,omitempty"`
	Error    string           `json:"error,omitempty"`
}

func parseRequest(raw []byte) (Request, error) {
	var request Request
	if len(raw) > MaxRequestSize || !utf8.Valid(raw) {
		return request, errors.New("invalid_request")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, e := decoder.Token()
	if e != nil || token != json.Delim('{') {
		return request, errors.New("invalid_request")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		key, e := decoder.Token()
		name, ok := key.(string)
		if e != nil || !ok {
			return request, errors.New("invalid_request")
		}
		if _, exists := fields[name]; exists {
			return request, errors.New("invalid_request")
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return request, errors.New("invalid_request")
		}
		fields[name] = value
	}
	if _, e = decoder.Token(); e != nil {
		return request, errors.New("invalid_request")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return request, errors.New("invalid_request")
	}
	if json.Unmarshal(fields["id"], &request.ID) != nil || request.ID == 0 || request.ID > 1<<53-1 || json.Unmarshal(fields["command"], &request.Command) != nil {
		return request, errors.New("invalid_request")
	}
	for key := range fields {
		if key != "id" && key != "command" && key != "sequence" && key != "path" && key != "fixture" {
			return Request{}, errors.New("invalid_request")
		}
	}
	if request.Command == "receipt" {
		if json.Unmarshal(fields["sequence"], &request.Sequence) != nil || request.Sequence == 0 || request.Sequence > 1000 {
			return request, errors.New("invalid_request")
		}
	} else if _, ok := fields["sequence"]; ok {
		return request, errors.New("invalid_request")
	}
	if request.Command == "export_recovery" || request.Command == "verify_backup" {
		if json.Unmarshal(fields["path"], &request.Path) != nil || !ValidRecoveryPath(request.Path) {
			return request, errors.New("invalid_request")
		}
	} else if _, ok := fields["path"]; ok {
		return request, errors.New("invalid_request")
	}
	switch request.Command {
	case "start", "stop", "snapshot", "receipt", "export_recovery", "verify_backup", "login_fixture", "logout", "reset_fixture":
	default:
		return request, errors.New("invalid_request")
	}
	if request.Command == "login_fixture" {
		if json.Unmarshal(fields["fixture"], &request.Fixture) != nil || (request.Fixture != "zero256" && request.Fixture != "incrementing256") {
			return request, errors.New("invalid_request")
		}
	} else if _, ok := fields["fixture"]; ok {
		return request, errors.New("invalid_request")
	}
	return request, nil
}

// ValidRecoveryPath bounds native-selected paths on the private helper pipe.
// The controller additionally checks filesystem ownership and symlink targets.
func ValidRecoveryPath(path string) bool {
	return len(path) <= 1024 && utf8.ValidString(path) && filepath.IsAbs(path) &&
		!strings.ContainsAny(path, "\x00\r\n")
}
func respond(writer io.Writer, response Response) error {
	data, e := json.Marshal(response)
	if e != nil || len(data)+1 > MaxResponseSize {
		return errors.New("response unavailable")
	}
	data = append(data, '\n')
	if deadline, ok := writer.(interface{ SetWriteDeadline(time.Time) error }); ok {
		// Production stdout is a pipe. Bound a stalled reader even when stdin
		// remains open; EOF/cancellation also closes the pipe independently.
		_ = deadline.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}
	n, e := writer.Write(data)
	if e == nil && n != len(data) {
		e = io.ErrShortWrite
	}
	return e
}

// Serve reads a bounded, correlated JSON-lines protocol. It closes and drains
// the controller on EOF, cancellation, malformed framing or output failure.
type ControllerAPI interface {
	Start() error
	Stop()
	Close() error
	Snapshot() Snapshot
	Receipt(uint64) (registry.Record, error)
	ExportRecovery(string) error
	VerifyBackup(string) error
}
type AccountControllerAPI interface {
	LoginFixture(string) error
	Logout() error
	ResetFixtureAccount() error
}

func Serve(ctx context.Context, input io.Reader, output io.Writer, c ControllerAPI) (resultErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type line struct {
		data []byte
		err  error
	}
	lines := make(chan line, 8)
	readerDone := make(chan struct{})
	shutdownDone := make(chan struct{})
	// This watcher must not depend on the response writer making progress.
	// A parent that closes stdin is quitting, even if its stdout pipe is full.
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
	defer func() {
		cancel()
		<-shutdownDone
		resultErr = errors.Join(resultErr, c.Close())
	}()
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
				// The desktop bridge sends one request at a time. A bounded
				// backlog fails closed rather than hiding EOF behind a writer.
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
					_ = respond(output, Response{OK: false, Error: "invalid_frame"})
				}
				return value.err
			}
			req, e := parseRequest(value.data)
			if e != nil {
				if e = respond(output, Response{ID: req.ID, OK: false, Error: "invalid_request"}); e != nil {
					return e
				}
				continue
			}
			response := Response{ID: req.ID, OK: true}
			switch req.Command {
			case "login_fixture", "logout", "reset_fixture":
				account, ok := c.(AccountControllerAPI)
				var err error
				if !ok {
					err = errors.New("account mode unavailable")
				} else {
					switch req.Command {
					case "login_fixture":
						err = account.LoginFixture(req.Fixture)
					case "logout":
						err = account.Logout()
					case "reset_fixture":
						err = account.ResetFixtureAccount()
					}
				}
				if err != nil {
					response.OK = false
					response.Error = "account_unavailable"
				} else {
					s := c.Snapshot()
					response.Snapshot = &s
				}
			case "start":
				if c.Start() != nil {
					response.OK = false
					response.Error = "start_unavailable"
				} else {
					s := c.Snapshot()
					response.Snapshot = &s
				}
			case "stop":
				c.Stop()
				s := c.Snapshot()
				response.Snapshot = &s
			case "snapshot":
				s := c.Snapshot()
				response.Snapshot = &s
			case "receipt":
				record, e := c.Receipt(req.Sequence)
				if e != nil {
					response.OK = false
					response.Error = "receipt_unavailable"
				} else {
					response.Receipt = &record
				}
			case "export_recovery", "verify_backup":
				var operationErr error
				if req.Command == "export_recovery" {
					operationErr = c.ExportRecovery(req.Path)
					response.Error = "export_unavailable"
				} else {
					operationErr = c.VerifyBackup(req.Path)
					response.Error = "backup_unavailable"
				}
				if operationErr != nil {
					response.OK = false
				} else {
					response.Error = ""
					s := c.Snapshot()
					response.Snapshot = &s
				}
			}
			if e = respond(output, response); e != nil {
				return e
			}
		}
	}
}
