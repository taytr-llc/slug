//go:build darwin || linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Model cancellation immediately after the original pre-write Err sample.
// The underlying real context is canceled; no clock or production hook changes.
type dailyCancelAfterErr struct {
	context.Context
	cancel context.CancelFunc
	once   sync.Once
}

func (c *dailyCancelAfterErr) Err() error {
	e := c.Context.Err()
	if e == nil {
		c.once.Do(c.cancel)
	}
	return e
}

func TestDailyHeadlessParentCancellationInterruptsOriginalPipe(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	// os.Pipe uses a pollable nonblocking descriptor on these platforms. Clear
	// and verify deadline support, then stop only at observed EAGAIN. Returning
	// true from RawConn.Write prevents its poller from waiting for buffer space.
	// No elapsed duration or scheduler progress is used as evidence of fullness.
	if err = writer.SetWriteDeadline(time.Time{}); err != nil {
		t.Fatal("original pipe deadline state", err)
	}
	raw, err := writer.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	frame, err := json.Marshal(strings.Repeat("p", 400))
	if err != nil {
		t.Fatal(err)
	}
	frame = append(frame, '\n')
	full, completed := false, 0
	for i := 0; i < 65536; i++ {
		var n int
		var writeErr error
		if err = raw.Write(func(fd uintptr) bool {
			n, writeErr = syscall.Write(int(fd), frame)
			return true
		}); err != nil {
			t.Fatal("original pipe prefill", err)
		}
		if writeErr == syscall.EINTR {
			continue
		}
		if writeErr == syscall.EAGAIN || writeErr == syscall.EWOULDBLOCK {
			if n > 0 {
				t.Fatal("partial atomic prefill", n)
			}
			full = true
			break
		}
		if writeErr != nil || n != len(frame) {
			t.Fatal("incomplete original prefill", n, writeErr)
		}
		completed++
	}
	if !full || completed == 0 {
		t.Fatal("original pipe fullness not observed")
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &dailyCancelAfterErr{Context: base, cancel: cancel}
	out := &dailyLaunchOutput{file: writer}
	err = out.write(ctx, strings.Repeat("p", 400))
	if err != context.Canceled || !dailyLaunchParentCancellation(ctx, err) {
		t.Fatal("clean original parent cancellation lost", err)
	}
	out.mu.Lock()
	interrupted, cause, closeErr := out.interrupted, out.interruptCause, out.closeErr
	out.mu.Unlock()
	if !interrupted || cause != context.Canceled || closeErr != nil {
		t.Fatal("original parent interrupt/close was not proved", interrupted, cause, closeErr)
	}
	// The interrupt may close before Write begins; this proves its own joined
	// cancellation path, not a timing claim that Write was already blocked.
	got, err := io.ReadAll(io.LimitReader(reader, 1<<20))
	if err != nil || len(got) != completed*len(frame) || len(got) >= 1<<20 {
		t.Fatal("original completed output framing", len(got), err)
	}
	for i := 0; i < completed; i++ {
		if !bytes.Equal(got[i*len(frame):(i+1)*len(frame)], frame) {
			t.Fatal("partial original output was hidden")
		}
	}
}
