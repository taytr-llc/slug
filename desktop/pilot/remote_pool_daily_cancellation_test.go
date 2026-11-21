package pilot

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Conditional guard controls use no fabricated node/wallet readiness. They
// prove only cause selection before any lower authority work or disclosure.
func TestDailyRemoteCancellationCauseAndLegacyParity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	view := strings.Repeat("1", 64)
	for _, version := range []uint32{1, 2} {
		a := &RemotePoolAttachment{config: RemotePoolConfig{Version: version}, view: RemotePoolView{ViewID: view}}
		_, end, e := a.begin(ctx, view)
		if end != nil {
			t.Fatal("canceled operation admitted")
		}
		want := error(ErrRemotePoolUnavailable)
		if version == 2 {
			want = context.Canceled
		}
		if e != want || a.check(ctx) != want {
			t.Fatal("mode/cancellation provenance", version, e)
		}
		if _, _, e = a.begin(ctx, strings.Repeat("2", 64)); e != ErrRemotePoolUnavailable {
			t.Fatal("foreign view normalized", e)
		}
		l := &remotePoolLocal{daily: version == 2}
		if e = l.check(ctx); e != want {
			t.Fatal("local mode/cancellation provenance", version, e)
		}
	}
}
func TestDailyRemoteStartRetainsObservedCloseFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &RemotePoolAttachment{config: RemotePoolConfig{Version: 2}, life: ctx, cancel: cancel, closeDone: make(chan struct{})}
	p, q := &remotePoolPoolLease{}, &remotePoolAdmissionLease{}
	failure := errors.New("conditional original lease close acknowledgement")
	held := a.retainStartCleanup(p, q, failure)
	var original *remotePoolStartCleanup
	if !errors.Is(held, ErrRemotePoolCleanup) || !errors.Is(held, failure) || !errors.As(held, &original) || original.pool != p || original.admission != q {
		t.Fatal("failed lease ownership", held)
	}
	first := a.Close()
	if !errors.Is(first, failure) || a.Close() != first || !errors.As(first, &original) || original.pool != p || original.admission != q {
		t.Fatal("failed original owner lost after Close", first)
	}
	legacy := &RemotePoolAttachment{config: RemotePoolConfig{Version: 1}}
	if legacy.retainStartCleanup(p, q, failure) != nil || legacy.closeErr != nil {
		t.Fatal("legacy error behavior changed")
	}
	if e := legacy.startCleanup(context.Canceled, nil, nil); e != context.Canceled {
		t.Fatal("legacy close ordering/result", e)
	}
}
