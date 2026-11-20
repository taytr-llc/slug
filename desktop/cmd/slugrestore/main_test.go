package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercarrier"
)

func TestRestoredCLIArgumentsNeverEchoInput(t *testing.T) {
	for _, args := range [][]string{{"status", "--password", "do-not-echo"}, {"status", "unexpected-private-input"}, {"do-not-echo"}} {
		var out, prompt bytes.Buffer
		e := run(context.Background(), args, nil, &out, &prompt)
		if e == nil || out.Len() != 0 || prompt.Len() != 0 || strings.Contains(e.Error(), "do-not-echo") {
			t.Fatal("invalid input was accepted or exposed")
		}
		if errorCode(e) != "invalid_request" {
			t.Fatal("wrong closed error")
		}
	}
}
func TestRestoredCLIExactReviewApproval(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{{"approve review\n", true}, {"yes\n", false}, {"approve another\n", false}, {"approve review \n", false}, {strings.Repeat("x", 300) + "\n", false}} {
		input, writer, e := os.Pipe()
		if e != nil {
			t.Fatal(e)
		}
		if _, e = writer.WriteString(tc.line); e != nil {
			t.Fatal(e)
		}
		_ = writer.Close()
		e = confirm(context.Background(), input, bufio.NewReaderSize(input, 256), &bytes.Buffer{}, "review", time.Now().Add(time.Minute).UnixMilli())
		_ = input.Close()
		if (e == nil) != tc.want {
			t.Fatal("wrong approval result", e)
		}
	}
}
func TestRestoredCLIApprovalCancellationJoinsInput(t *testing.T) {
	input, writer, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	e = confirm(ctx, input, bufio.NewReaderSize(input, 256), &bytes.Buffer{}, "review", time.Now().Add(time.Minute).UnixMilli())
	if !errors.Is(e, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatal("input cancellation did not join", e)
	}
	if _, e = input.Stat(); !errors.Is(e, os.ErrClosed) {
		t.Fatal("cancelled input owner remained open", e)
	}
}
func TestRestoredCLIPublicOutputBound(t *testing.T) {
	var out bytes.Buffer
	if e := writeJSON(&out, strings.Repeat("x", 65536)); !errors.Is(e, fightercarrier.ErrCapacity) || out.Len() != 0 {
		t.Fatal("oversize output partly published", e)
	}
}

func TestRestoredCLIRejectsPublicInputsBeforeFilesOrOutput(t *testing.T) {
	common := []string{"--directory", "/must-not-open", "--public-config", "/must-not-open", "--node-config", "/must-not-open"}
	for _, tail := range [][]string{{"--request-id", "pasted-private-text"}, {"--request-id", strings.Repeat("A", 64)}, {"--recipient-gym", "pasted-private-text"}, {"--leg", "spend"}} {
		args := append([]string{"status"}, common...)
		args = append(args, tail...)
		var out, prompt bytes.Buffer
		e := run(context.Background(), args, nil, &out, &prompt)
		if !errors.Is(e, errArguments) || out.Len() != 0 || prompt.Len() != 0 {
			t.Fatal("invalid/irrelevant input reached file open or output", errorCode(e))
		}
	}
}
func TestRestoredCLIOutcomeUnknownDominatesOtherFailures(t *testing.T) {
	for _, other := range []error{context.Canceled, context.DeadlineExceeded, pilot.ErrRestoredGymStorage, errors.New("private error")} {
		e := errors.Join(other, fightercarrier.ErrOutcomeUnknown)
		if errorCode(e) != "outcome_unknown" {
			t.Fatal("uncertain publication was downgraded")
		}
	}
}
func TestRestoredCLIBlockedOutputCancellationJoins(t *testing.T) {
	reader, writer, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	join := interruptOutputs(ctx, writer)
	start := time.Now()
	// Several maximum lines fill the concrete pipe without a reader. Cancellation
	// owns that exact descriptor and must unblock the synchronous write.
	var writeErr error
	for n := 0; n < 4096; n++ {
		writeErr = writeJSON(writer, strings.Repeat("x", 60000))
		if writeErr != nil {
			break
		}
	}
	closeErr := join()
	if ctx.Err() == nil || writeErr == nil || closeErr != nil || time.Since(start) > time.Second {
		t.Fatal("blocked output failed to join", writeErr, closeErr)
	}
	if _, e = writer.Stat(); !errors.Is(e, os.ErrClosed) {
		t.Fatal("output descriptor remained open", e)
	}
}

func TestRestoredCLIFinalErrorOutputCancellationJoins(t *testing.T) {
	reader, writer, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	if e = writer.SetWriteDeadline(time.Now().Add(10 * time.Millisecond)); e != nil {
		t.Fatal(e)
	}
	n, e := writer.Write(make([]byte, 1<<20))
	if n == 0 || !errors.Is(e, os.ErrDeadlineExceeded) {
		t.Fatal("could not fill exact output pipe", n, e)
	}
	if e = writer.SetWriteDeadline(time.Time{}); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	join := interruptOutputs(ctx, writer)
	start := time.Now()
	e = finishCommand(ctx, errArguments, writer, join)
	if !errors.Is(e, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatal("final diagnostic escaped cancellation ownership", e)
	}
	if _, e = writer.Stat(); !errors.Is(e, os.ErrClosed) {
		t.Fatal("final output owner not joined", e)
	}
}
