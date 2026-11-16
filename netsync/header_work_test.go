package netsync

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestHeadersYieldAndReplayProgress(t *testing.T) {
	sm, cleanup := makeMockSyncManager(t, &chaincfg.MainNetParams)
	defer cleanup()
	p := newSyncCandidate(t, sm, 100)
	sm.syncPeer, sm.ibdMode = p, true
	headers := wire.NewMsgHeaders()
	for _, header := range loadHeaders(t) {
		require.NoError(t, headers.AddBlockHeader(header))
	}
	msg := &headersMsg{headers: headers, peer: p}
	require.False(t, sm.handleHeadersMsg(msg), "a long reply must yield")
	require.Equal(t, headersPerTurn, msg.next)
	_, height := sm.chain.BestHeader()
	require.Equal(t, int32(headersPerTurn), height)
	require.Empty(t, sm.requestedBlocks, "don't fetch mid-response")
	require.False(t, sm.lastProgressTime.IsZero(), "new headers are progress")
	processAllHeaders(sm, msg)
	_, height = sm.chain.BestHeader()
	require.Equal(t, int32(len(headers.Headers)), height)

	// Known-header replay still reaches current chain validation, but must
	// not keep a stalled sync peer alive by pretending to advance its tip.
	sentinel := time.Unix(1, 0)
	sm.lastProgressTime = sentinel
	processAllHeaders(sm, &headersMsg{headers: headers, peer: p})
	require.Equal(t, sentinel, sm.lastProgressTime)
}

func TestHeadersCancelBeforeHash(t *testing.T) {
	sm, cleanup := makeMockSyncManager(t, &chaincfg.MainNetParams)
	defer cleanup()
	p := newSyncCandidate(t, sm, 100)
	close(sm.quit)
	// Nil would panic if processing reached validation. Shutdown must stop
	// before any hashing or access to an unprocessed header.
	msg := &headersMsg{headers: &wire.MsgHeaders{Headers: []*wire.BlockHeader{nil}}, peer: p}
	require.True(t, sm.handleHeadersMsg(msg))
	require.Zero(t, msg.next)
	_, height := sm.chain.BestHeader()
	require.Zero(t, height)
}

func TestQueueHeadersBackpressureAndShutdown(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		sm := &SyncManager{msgChan: make(chan interface{}, 1), quit: make(chan struct{})}
		returned := make(chan struct{})
		go func() {
			sm.QueueHeaders(wire.NewMsgHeaders(), nil)
			close(returned)
		}()
		var msg *headersMsg
		select {
		case queued := <-sm.msgChan:
			msg = queued.(*headersMsg)
		case <-time.After(5 * time.Second):
			t.Fatal("headers did not queue")
		}
		select {
		case <-returned:
			t.Fatal("receive callback returned before processing")
		default:
		}
		if shutdown {
			close(sm.quit)
		} else {
			close(msg.reply)
		}
		select {
		case <-returned:
		case <-time.After(5 * time.Second):
			t.Fatal("receive callback stayed blocked")
		}
	}

	// Shutdown also unblocks a sender waiting for room in a full queue.
	sm := &SyncManager{msgChan: make(chan interface{}), quit: make(chan struct{})}
	done := make(chan struct{})
	go func() { sm.QueueHeaders(wire.NewMsgHeaders(), nil); close(done) }()
	close(sm.quit)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown stranded a header enqueue")
	}
}
