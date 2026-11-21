//go:build linux || (darwin && cgo)

package main

import (
	"context"
	"errors"
	"os"
	"testing"
)

// Real selected descriptors and sticky owner cleanup, not a constructed wallet
// or proof of full controller readiness. The real signed-account tests live in
// accountpilot; the actual CLI acceptance exercises their composition.
func TestDailyHeadlessOriginalFileOwnersJoinBeforeCancellationSuccess(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "joined", true: "failed-close"}[failed], func(t *testing.T) {
			first := remoteLaunchTestSelect(t, remoteLaunchTestFile(t, []byte("PUBLIC CONFIG")))
			secondPath := remoteLaunchTestFile(t, []byte("PUBLIC RPC"))
			second, e := selectRemoteLaunchInput(context.Background(), secondPath, 64)
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithCancel(context.Background())
			o := &remoteLaunchOwner{life: ctx, cancel: cancel, files: [2]*remoteLaunchInput{first, second}}
			t.Cleanup(func() {
				if e := o.Close(); e != nil && !failed {
					t.Error("original selected-file cleanup", e)
				}
			})
			output, e := os.CreateTemp(t.TempDir(), "public-status")
			if e != nil {
				t.Fatal(e)
			}
			defer output.Close()
			w := &dailyLaunchOutput{file: output}
			if failed {
				if e = second.file.Close(); e != nil {
					t.Fatal(e)
				}
			}
			cancel()
			result := finishDailyLaunch(ctx, o, w, context.Canceled)
			if !o.closing || !first.closeDone || !second.closeDone {
				t.Fatal("original owners not joined")
			}
			if !failed {
				if result != nil || o.Close() != nil {
					t.Fatal("clean requested shutdown", result)
				}
			} else {
				var retained *remoteLaunchError
				if !errors.Is(result, errRemoteLaunchCleanup) || !errors.As(result, &retained) || retained.retained != o || o.files[1] != second || o.Close() != o.closeErr {
					t.Fatal("failed original owner lost", result)
				}
			}
		})
	}
}
func TestDailyHeadlessObservedIntegrityIsNotCancellation(t *testing.T) {
	p := remoteLaunchTestFile(t, []byte("PUBLIC ORIGINAL"))
	f := remoteLaunchTestSelect(t, p)
	if e := os.Chmod(p, 0644); e != nil {
		t.Fatal(e)
	}
	observed := f.check(context.Background())
	if !errors.Is(observed, errRemoteLaunchIntegrity) {
		t.Fatal(observed)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if dailyLaunchParentCancellation(ctx, errors.Join(context.Canceled, observed)) {
		t.Fatal("observed replacement erased")
	}
}
