// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

// Package bridge owns the single bundled supervisor process and its bounded,
// serialized public JSON-lines protocol. It never reads private key material.
package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
)

var (
	ErrUnavailable = errors.New("Local pilot is unavailable. Quit and reopen the app.")
	ErrBusy        = errors.New("The pilot is finishing another request. Try again shortly.")
	ErrClosed      = errors.New("The local pilot is shutting down.")
)

// Bridge has one request in flight, no unbounded request queue, and one reader.
// Closing stdin asks the helper to drain its real worker and child nodes; Close
// waits for the process rather than killing work in the middle of a sync write.
type Bridge struct {
	cmd       *exec.Cmd
	input     io.WriteCloser
	responses chan pilot.Response
	failed    chan struct{}
	done      chan struct{}
	readDone  chan struct{}
	gate      chan struct{}
	once      sync.Once
	mu        sync.Mutex
	failure   error
	waitErr   error
	nextID    uint64
}

// Launch starts the supervisor, which is stopped until Start is requested.
func Launch(helper, dataDir, node string) (*Bridge, error) {
	return start(exec.Command(helper, "--data-dir", dataDir, "--node-binary", node))
}
func start(cmd *exec.Cmd) (*Bridge, error) {
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, ErrUnavailable
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, ErrUnavailable
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, ErrUnavailable
	}
	b := &Bridge{cmd: cmd, input: input, responses: make(chan pilot.Response, 1), failed: make(chan struct{}), done: make(chan struct{}), readDone: make(chan struct{}), gate: make(chan struct{}, 1)}
	go b.read(output)
	go func() {
		err := cmd.Wait()
		b.mu.Lock()
		b.waitErr = err
		b.mu.Unlock()
		b.fail(ErrUnavailable)
		close(b.done)
	}()
	return b, nil
}
func (b *Bridge) fail(err error) {
	b.once.Do(func() {
		b.mu.Lock()
		b.failure = err
		b.mu.Unlock()
		close(b.failed)
		_ = b.input.Close()
	})
}
func (b *Bridge) read(r io.Reader) {
	defer close(b.readDone)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), pilot.MaxResponseSize)
	for scanner.Scan() {
		response, err := decode(scanner.Bytes())
		if err != nil {
			b.fail(ErrUnavailable)
			return
		}
		select {
		case b.responses <- response:
		case <-b.failed:
			return
		default:
			b.fail(ErrUnavailable)
			return
		}
	}
	b.fail(ErrUnavailable)
}
func decode(raw []byte) (pilot.Response, error) {
	var response pilot.Response
	if len(raw) == 0 || len(raw) >= pilot.MaxResponseSize || json.Unmarshal(raw, &response) != nil {
		return response, ErrUnavailable
	}
	// The helper emits canonical json.Marshal output. Exact round-trip rejects
	// duplicate/unknown/case-alias keys, nulls and trailing data at every level.
	canonical, err := json.Marshal(response)
	if err != nil || !bytes.Equal(raw, canonical) || response.ID == 0 || response.ID > 1<<53-1 {
		return response, ErrUnavailable
	}
	if !response.OK {
		if response.Snapshot != nil || response.Receipt != nil {
			return response, ErrUnavailable
		}
		switch response.Error {
		case "invalid_request", "invalid_frame", "start_unavailable", "receipt_unavailable":
		default:
			return response, ErrUnavailable
		}
	} else if response.Error != "" || (response.Snapshot == nil) == (response.Receipt == nil) {
		return response, ErrUnavailable
	}
	if s := response.Snapshot; s != nil {
		switch s.State {
		case "stopped", "starting", "running", "stopping", "failed":
		default:
			return response, ErrUnavailable
		}
		if s.Mode != pilot.Mode || s.Prospects == nil || len(s.Prospects) > 100 || s.ProspectCount < uint64(len(s.Prospects)) || s.HashesPerSecond < 0 {
			return response, ErrUnavailable
		}
	}
	return response, nil
}

func (b *Bridge) Request(ctx context.Context, command string, sequence uint64) (pilot.Response, error) {
	switch command {
	case "snapshot", "start", "stop":
		if sequence != 0 {
			return pilot.Response{}, ErrUnavailable
		}
	case "receipt":
		if sequence == 0 || sequence > 1000 {
			return pilot.Response{}, ErrUnavailable
		}
	default:
		return pilot.Response{}, ErrUnavailable
	}
	select {
	case b.gate <- struct{}{}:
		defer func() { <-b.gate }()
	default:
		return pilot.Response{}, ErrBusy
	}
	select {
	case <-b.failed:
		return pilot.Response{}, b.error()
	default:
	}
	if ctx.Err() != nil {
		return pilot.Response{}, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	b.nextID++
	if b.nextID > 1<<53-1 {
		b.fail(ErrUnavailable)
		return pilot.Response{}, ErrUnavailable
	}
	raw, _ := json.Marshal(pilot.Request{ID: b.nextID, Command: command, Sequence: sequence})
	raw = append(raw, '\n')
	// The request is less than 100 bytes and the one-at-a-time protocol never
	// fills the pipe. A dead/stalled helper is drained on operation timeout.
	if deadline, ok := b.input.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = deadline.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}
	if n, err := b.input.Write(raw); err != nil || n != len(raw) {
		b.fail(ErrUnavailable)
		return pilot.Response{}, ErrUnavailable
	}
	select {
	case response := <-b.responses:
		if response.ID != b.nextID || (response.OK && ((command == "receipt") != (response.Receipt != nil))) {
			b.fail(ErrUnavailable)
			return pilot.Response{}, ErrUnavailable
		}
		if response.Receipt != nil && (response.Receipt.Sequence != sequence || response.Receipt.Version != 4) {
			b.fail(ErrUnavailable)
			return pilot.Response{}, ErrUnavailable
		}
		if !response.OK {
			return pilot.Response{}, errors.New("The local pilot could not complete this request.")
		}
		return response, nil
	case <-b.failed:
		return pilot.Response{}, b.error()
	case <-ctx.Done():
		b.fail(ErrUnavailable)
		return pilot.Response{}, ErrUnavailable
	}
}
func (b *Bridge) error() error { b.mu.Lock(); defer b.mu.Unlock(); return b.failure }

// Close returns only after the helper and its owned nodes have stopped. A
// failed drain is surfaced, never reported as a clean shutdown.
func (b *Bridge) Close() error {
	b.fail(ErrClosed)
	<-b.done
	<-b.readDone
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.waitErr != nil {
		return ErrUnavailable
	}
	return nil
}
