// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestDailyHeadlessOutputFailureSurvivesParentCancellation(t *testing.T) {
	for _, name := range []string{"broken-reader", "preclosed-writer"} {
		t.Run(name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			defer writer.Close()
			want := error(syscall.EPIPE)
			if name == "broken-reader" {
				if err = reader.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				want = os.ErrClosed
				if err = writer.Close(); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := &dailyLaunchOutput{file: writer}
			err = out.write(ctx, "public status")
			cancel() // A later/coincident signal cannot erase the actual Write cause.
			if !errors.Is(err, want) || !errors.Is(err, errRemoteLaunchUnavailable) || dailyLaunchParentCancellation(ctx, err) {
				t.Fatal("actual write failure erased", err)
			}
			out.mu.Lock()
			interrupted := out.interrupted
			out.mu.Unlock()
			if interrupted {
				t.Fatal("failure invented its own interrupt")
			}
		})
	}
}

func TestDailyHeadlessCancellationPreservesOtherCauses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !dailyLaunchParentCancellation(ctx, &remoteLaunchError{cause: errors.Join(context.Canceled, context.Canceled)}) {
		t.Fatal("known original cancellation wrapper refused")
	}
	if dailyLaunchParentCancellation(ctx, fmt.Errorf("unclassified wrapper: %w", context.Canceled)) {
		t.Fatal("unknown wrapper erased")
	}
	for _, cause := range []error{errRemoteLaunchUnavailable, errRemoteLaunchIntegrity, errRemoteLaunchCleanup, context.DeadlineExceeded, syscall.EPIPE, io.ErrShortWrite} {
		if dailyLaunchParentCancellation(ctx, errors.Join(context.Canceled, cause)) {
			t.Fatal("mixed error hidden", cause)
		}
	}
	for _, cause := range []error{nil, &remoteLaunchError{cause: context.Canceled, retained: &remoteLaunchOwner{}}, &dailyLaunchCleanupError{cause: context.Canceled}} {
		if dailyLaunchParentCancellation(ctx, cause) {
			t.Fatal("missing cancellation or retained owner hidden", cause)
		}
	}
	if dailyLaunchParentCancellation(context.Background(), context.Canceled) {
		t.Fatal("foreign cancellation accepted")
	}
	closed := &os.PathError{Op: "write", Path: "published-pipe-fixture", Err: os.ErrClosed}
	cases := []struct {
		name                        string
		n                           int
		writeErr, workErr, closeErr error
	}{
		{"partial-frame", 1, closed, context.Canceled, nil},
		{"independent-epipe", 0, syscall.EPIPE, context.Canceled, nil},
		{"mixed-closed-and-epipe", 0, errors.Join(closed, syscall.EPIPE), context.Canceled, nil},
		{"own-deadline-first", 0, closed, context.DeadlineExceeded, nil},
		{"failed-owned-close", 0, closed, context.Canceled, errRemoteLaunchCleanup},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := dailyLaunchWriteResult(context.Canceled, c.workErr, c.writeErr, c.n, 10, true, context.Canceled, c.closeErr)
			if dailyLaunchParentCancellation(ctx, err) || !errors.Is(err, errRemoteLaunchUnavailable) {
				t.Fatal("conditional independent failure normalized", err)
			}
		})
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	out := &dailyLaunchOutput{file: writer}
	if err = out.write(ctx, "no effect"); err != context.Canceled || out.interrupted {
		t.Fatal("canceled pre-write did not refuse exactly", err)
	}
	if err = out.write(ctx, func() {}); !errors.Is(err, errRemoteLaunchUnavailable) || dailyLaunchParentCancellation(ctx, err) {
		t.Fatal("serialization failure hidden by cancellation", err)
	}
}

func TestDailyHeadlessFixedStatusAtomicFrame(t *testing.T) {
	value := dailyLaunchStatus{Version: 2, ObservedAtMS: int64(^uint64(0) >> 1), ConfigID: strings.Repeat("f", 64), State: "signed_in", WorkerState: "unavailable", Code: "cleanup_incomplete", Records: ^uint64(0), Pending: ^uint64(0)}
	raw, err := json.Marshal(value)
	if err != nil || len(raw)+1 > 270 || len(raw)+1 > 512 {
		t.Fatal("fixed status bound", len(raw), err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	out := &dailyLaunchOutput{file: writer}
	if err = out.write(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(raw)+1)
	if _, err = io.ReadFull(reader, got); err != nil || !bytes.Equal(got, append(raw, '\n')) {
		t.Fatal("exact complete status", err)
	}
	if err = out.write(context.Background(), strings.Repeat("x", 510)); !errors.Is(err, errRemoteLaunchUnavailable) {
		t.Fatal("oversized atomic frame accepted", err)
	}
}
