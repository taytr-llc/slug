// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
)

type ownedWireTestWriter struct {
	entered, closed chan struct{}
	once            sync.Once
	writes          atomic.Int32
	block           bool
	writeHook       func()
}

func (w *ownedWireTestWriter) Write(b []byte) (int, error) {
	w.writes.Add(1)
	if w.entered != nil {
		close(w.entered)
	}
	if w.writeHook != nil {
		w.writeHook()
	}
	if w.block {
		<-w.closed
		return 0, io.ErrClosedPipe
	}
	return len(b), nil
}
func (w *ownedWireTestWriter) Close() error { w.once.Do(func() { close(w.closed) }); return nil }
func ownedWireTestBridge(w io.WriteCloser) *OwnedBridge {
	return &OwnedBridge{input: w, failed: make(chan struct{}), lines: make(chan []byte, 1), gate: make(chan struct{}, 1), scopes: map[string]pilot.OwnedGymScope{}, activeFixture: "zero256"}
}
func TestOwnedBridgeCancelledCallerNeverDispatches(t *testing.T) {
	w := &ownedWireTestWriter{closed: make(chan struct{})}
	b := ownedWireTestBridge(w)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := b.exchangeOwned(ctx, pilot.OwnedGymRequest{Command: "snapshot"}, nil, nil)
	if !errors.Is(e, context.Canceled) || w.writes.Load() != 0 {
		t.Fatal("cancelled caller dispatched", e, w.writes.Load())
	}
	select {
	case <-b.failed:
		t.Fatal("no dispatch poisoned helper")
	default:
	}
}
func TestOwnedBridgeCancellationJoinsBlockedWrite(t *testing.T) {
	w := &ownedWireTestWriter{entered: make(chan struct{}), closed: make(chan struct{}), block: true}
	b := ownedWireTestBridge(w)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := b.exchangeOwned(ctx, pilot.OwnedGymRequest{Command: "snapshot"}, nil, nil); done <- e }()
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("write not entered")
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not join pipe write")
	}
	select {
	case <-b.failed:
	default:
		t.Fatal("uncertain helper remained reusable")
	}
}
func TestOwnedBridgeQueuedReplyCannotBeatCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &ownedWireTestWriter{closed: make(chan struct{}), writeHook: cancel}
	b := ownedWireTestBridge(w)
	b.lines <- []byte(`{"version":1,"id":1,"command":"snapshot","view_id":"","ok":false,"result":null,"error":"outcome_unknown"}`)
	_, e := b.exchangeOwned(ctx, pilot.OwnedGymRequest{Command: "snapshot"}, nil, nil)
	if !errors.Is(e, context.Canceled) {
		t.Fatal("accepted reply after cancellation", e)
	}
	if len(b.lines) != 1 {
		t.Fatal("cancelled call consumed reply as acknowledgment")
	}
}
func TestOwnedBridgeOriginalViewAndReviewBeforeTransport(t *testing.T) {
	w := &ownedWireTestWriter{closed: make(chan struct{})}
	b := ownedWireTestBridge(w)
	v := &OwnedContext{helper: b, view: pilot.OwnedGymView{ViewID: strings.Repeat("1", 64)}}
	b.view = v
	b.scopes["zero256"] = pilot.OwnedGymScope{}
	for _, cmd := range []string{"transfer_confirm", "transfer_deliver", "apply_slot", "apply_entry", "stable_page", "history_page"} {
		_, e := b.Request(context.Background(), v, pilot.OwnedGymRequest{Command: cmd, ReviewID: strings.Repeat("2", 64), OperationID: strings.Repeat("3", 64), CaptureID: strings.Repeat("4", 64)})
		if e == nil || w.writes.Load() != 0 {
			t.Fatal("unreviewed action reached pipe", cmd, e)
		}
	}
	old := *v
	b.view = &OwnedContext{helper: b, view: v.view}
	if b.ContextMatches(&old) {
		t.Fatal("same-shaped replacement view adopted")
	}
	if _, e := b.Request(context.Background(), v, pilot.OwnedGymRequest{Command: "stop"}); e == nil || w.writes.Load() != 0 {
		t.Fatal("old concrete view dispatched", e)
	}
}
