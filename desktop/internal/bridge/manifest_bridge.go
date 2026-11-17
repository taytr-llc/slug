// Copyright (c) 2026 The SLUG developers. ISC license.
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

type ManifestBridge struct {
	cmd       *exec.Cmd
	input     io.WriteCloser
	responses chan pilot.ManifestResponse
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

// LaunchManifests selects the separately typed candidate helper mode.
func LaunchManifests(helper, dataDir, node string) (*ManifestBridge, error) {
	return startManifest(exec.Command(helper, "--candidate-manifests", "--data-dir", dataDir, "--node-binary", node))
}
func startManifest(cmd *exec.Cmd) (*ManifestBridge, error) {
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
	b := &ManifestBridge{cmd: cmd, input: input, responses: make(chan pilot.ManifestResponse, 1), failed: make(chan struct{}), done: make(chan struct{}), readDone: make(chan struct{}), gate: make(chan struct{}, 1)}
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
func (b *ManifestBridge) fail(err error) {
	b.once.Do(func() {
		b.mu.Lock()
		b.failure = err
		b.mu.Unlock()
		close(b.failed)
		_ = b.input.Close()
	})
}
func (b *ManifestBridge) read(r io.Reader) {
	defer close(b.readDone)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), pilot.MaxResponseSize)
	for scanner.Scan() {
		response, err := decodeManifest(scanner.Bytes())
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

// decodeManifest accepts only the helper's exact canonical typed envelope.
func decodeManifest(raw []byte) (pilot.ManifestResponse, error) {
	var response pilot.ManifestResponse
	if len(raw) == 0 || len(raw) >= pilot.MaxResponseSize || preflightManifest(raw) != nil || json.Unmarshal(raw, &response) != nil {
		return response, ErrUnavailable
	}
	canonical, err := json.Marshal(response)
	if err != nil || !bytes.Equal(raw, canonical) || response.ID == 0 || response.ID > 1<<53-1 {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	count := 0
	if response.Snapshot != nil {
		count++
	}
	if response.Portrait != nil {
		count++
	}
	if response.ManifestReceipt != nil {
		count++
	}
	if !response.OK {
		if count != 0 || !oneOf(response.Error, "invalid_request", "invalid_frame", "start_unavailable", "export_unavailable", "backup_unavailable", "account_unavailable", "projection_unavailable", "projection_corrupt", "projection_cancelled", "projection_required", "prospect_unavailable", "portrait_unavailable", "manifest_receipt_unavailable", "initialization_incomplete") {
			return pilot.ManifestResponse{}, ErrUnavailable
		}
		return response, nil
	}
	if count != 1 || response.Error != "" {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	if response.Snapshot != nil && !validManifestSnapshot(response.Snapshot) {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	if response.Portrait != nil && !validManifestPortrait(response.Portrait) {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	if response.ManifestReceipt != nil && !validManifestReceipt(response.ManifestReceipt) {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	return response, nil
}

// preflightManifest bounds collection allocations before decoding into the
// typed DTO. The frame cap alone would still permit thousands of large items.
// Canonical re-encoding below separately rejects unknown/duplicate fields.
func preflightManifest(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value func(string, int) error
	value = func(path string, depth int) error {
		if depth > 8 {
			return ErrUnavailable
		}
		token, err := decoder.Token()
		if err != nil {
			return ErrUnavailable
		}
		delim, composite := token.(json.Delim)
		if !composite {
			return nil
		}
		switch delim {
		case '{':
			for decoder.More() {
				key, err := decoder.Token()
				name, ok := key.(string)
				if err != nil || !ok || len(name) > 64 {
					return ErrUnavailable
				}
				next := name
				if path != "" {
					next = path + "." + name
				}
				if err := value(next, depth+1); err != nil {
					return err
				}
			}
		case '[':
			limit := 0
			switch path {
			case "snapshot.projection.items":
				limit = pilot.MaxManifestProspects
			case "manifest_receipt.record.proof.merkle_branch":
				limit = 20
			default:
				return ErrUnavailable
			}
			for count := 0; decoder.More(); count++ {
				if count >= limit {
					return ErrUnavailable
				}
				if err := value(path+"[]", depth+1); err != nil {
					return err
				}
			}
		default:
			return ErrUnavailable
		}
		end, err := decoder.Token()
		if err != nil || delim == '{' && end != json.Delim('}') || delim == '[' && end != json.Delim(']') {
			return ErrUnavailable
		}
		return nil
	}
	if err := value("", 1); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrUnavailable
	}
	return nil
}

func (b *ManifestBridge) Request(ctx context.Context, command string, sequence uint64) (pilot.ManifestResponse, error) {
	switch command {
	case "snapshot", "start", "stop", "logout", "reset_fixture", "refresh_manifest_prospects":
		if sequence != 0 {
			return pilot.ManifestResponse{}, ErrUnavailable
		}
	case "manifest_portrait", "manifest_receipt":
		if sequence == 0 || sequence > pilot.MaxManifestSequence {
			return pilot.ManifestResponse{}, ErrUnavailable
		}
	default:
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	return b.request(ctx, pilot.ManifestRequest{Command: command, Sequence: sequence})
}
func (b *ManifestBridge) RequestFixture(ctx context.Context, fixture string) (pilot.ManifestResponse, error) {
	if fixture != "zero256" && fixture != "incrementing256" {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	return b.request(ctx, pilot.ManifestRequest{Command: "login_fixture", Fixture: fixture})
}
func (b *ManifestBridge) RequestPath(ctx context.Context, command, path string) (pilot.ManifestResponse, error) {
	if !oneOf(command, "export_recovery", "verify_backup") || !pilot.ValidRecoveryPath(path) {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	return b.request(ctx, pilot.ManifestRequest{Command: command, Path: path})
}

// ExportManifest receives only a native dialog selection and its captured view.
func (b *ManifestBridge) ExportManifest(ctx context.Context, sequence uint64, viewID, prospectID, path string) (pilot.ManifestResponse, error) {
	if sequence == 0 || sequence > pilot.MaxManifestSequence || !publicHex.MatchString(viewID) || !publicHex.MatchString(prospectID) || !pilot.ValidRecoveryPath(path) {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	return b.request(ctx, pilot.ManifestRequest{Command: "export_manifest_receipt", Sequence: sequence, ViewID: viewID, ProspectID: prospectID, Path: path})
}
func (b *ManifestBridge) ReleaseManifest(ctx context.Context, viewID, prospectID string) (pilot.ManifestResponse, error) {
	if !publicHex.MatchString(viewID) || !publicHex.MatchString(prospectID) {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	return b.request(ctx, pilot.ManifestRequest{Command: "release_manifest_receipt", ViewID: viewID, ProspectID: prospectID})
}
func (b *ManifestBridge) request(ctx context.Context, request pilot.ManifestRequest) (pilot.ManifestResponse, error) {
	select {
	case b.gate <- struct{}{}:
		defer func() { <-b.gate }()
	default:
		return pilot.ManifestResponse{}, ErrBusy
	}
	select {
	case <-b.failed:
		return pilot.ManifestResponse{}, b.error()
	default:
	}
	if ctx.Err() != nil {
		return pilot.ManifestResponse{}, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	b.nextID++
	if b.nextID > 1<<53-1 {
		b.fail(ErrUnavailable)
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	request.ID = b.nextID
	raw, err := json.Marshal(request)
	if err != nil || len(raw) > pilot.MaxRequestSize {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	raw = append(raw, 10)
	if deadline, ok := b.input.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = deadline.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}
	if n, err := b.input.Write(raw); err != nil || n != len(raw) {
		b.fail(ErrUnavailable)
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	select {
	case response := <-b.responses:
		if response.ID != b.nextID {
			b.fail(ErrUnavailable)
			return pilot.ManifestResponse{}, ErrUnavailable
		}
		if response.OK {
			correct := false
			switch request.Command {
			case "manifest_portrait":
				correct = response.Portrait != nil && response.Portrait.Sequence == request.Sequence
			case "manifest_receipt":
				correct = response.ManifestReceipt != nil && response.ManifestReceipt.Record.Sequence == request.Sequence
			default:
				correct = response.Snapshot != nil
			}
			if !correct {
				b.fail(ErrUnavailable)
				return pilot.ManifestResponse{}, ErrUnavailable
			}
		} else {
			return pilot.ManifestResponse{}, errors.New("The local candidate pilot could not complete this request.")
		}
		return response, nil
	case <-b.failed:
		return pilot.ManifestResponse{}, b.error()
	case <-ctx.Done():
		b.fail(ErrUnavailable)
		return pilot.ManifestResponse{}, ErrUnavailable
	}
}
func (b *ManifestBridge) error() error { b.mu.Lock(); defer b.mu.Unlock(); return b.failure }

// Close returns only after the helper and its owned nodes have stopped. A
// failed drain is surfaced, never reported as a clean shutdown.
func (b *ManifestBridge) Close() error {
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
