package netsync

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/peer"
	"github.com/taytr-llc/slug/wire/v2"
)

// connectedSchedulerPeer exercises the scheduler's real Connected check. The
// outbound protocol negotiation waits on the pipe until test cleanup; no peer
// internals are changed and no external listener or network access is needed.
func connectedSchedulerPeer(t *testing.T, sm *SyncManager) *peer.Peer {
	t.Helper()
	p, err := peer.NewOutboundPeer(&peer.Config{
		ChainParams: sm.chainParams,
	}, "127.0.0.1:29333")
	require.NoError(t, err)
	local, remote := net.Pipe()
	p.AssociateConnection(local)
	t.Cleanup(func() {
		p.Disconnect()
		_ = remote.Close()
	})
	require.True(t, p.Connected())
	p.UpdateLastBlockHeight(100)
	sm.peerStates[p] = &peerSyncState{
		syncCandidate:   true,
		requestedTxns:   make(map[chainhash.Hash]struct{}),
		requestedBlocks: make(map[chainhash.Hash]struct{}),
	}
	return p
}

func TestHeaderSchedulerYieldsWithBusyInput(t *testing.T) {
	sm, cleanup := makeMockSyncManager(t, &chaincfg.MainNetParams)
	t.Cleanup(cleanup)
	p := connectedSchedulerPeer(t, sm)
	sm.syncPeer, sm.ibdMode = p, true
	sm.msgChan = make(chan interface{}, 4)

	// Warm only the digest cache, not the chain index. Header 1 advances the
	// chain on the first turn; header 2 advances it only at batch completion.
	// Replays model the cheap-to-send attack and keep this test bounded even
	// though it exercises a maximum-sized protocol response.
	fixture := loadHeaders(t)
	fixture[0].BlockHash()
	fixture[1].BlockHash()
	headers := wire.NewMsgHeaders()
	for i := 0; i < wire.MaxBlockHeadersPerMsg-1; i++ {
		require.NoError(t, headers.AddBlockHeader(fixture[0]))
	}
	require.NoError(t, headers.AddBlockHeader(fixture[1]))
	completed := make(chan struct{})
	work := &headersMsg{headers: headers, peer: p, reply: completed}
	sm.msgChan <- work

	// Keep ordinary requests ready while header continuations are pending.
	// Replies are unbuffered, just like SyncPeerID, so each service operation
	// is observed instead of allowing the handler to race through responses.
	queries := make(chan chan int32, 4096)
	filled := make(chan struct{}, 1)
	stopProducer := make(chan struct{})
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		for {
			reply := make(chan int32)
			select {
			case queries <- reply:
			case <-stopProducer:
				return
			}
			select {
			case sm.msgChan <- getSyncPeerMsg{reply: reply}:
			case <-stopProducer:
				return
			}
			if len(sm.msgChan) == cap(sm.msgChan) {
				select {
				case filled <- struct{}{}:
				default:
				}
			}
		}
	}()

	// On any assertion failure, release ordinary reply sends before Stop.
	// The final advertised query might not have entered msgChan, so draining
	// also has an independent stop signal.
	var awaitingReply chan int32
	t.Cleanup(func() {
		close(stopProducer)
		<-producerDone
		stopDrain := make(chan struct{})
		drainDone := make(chan struct{})
		go func() {
			defer close(drainDone)
			if awaitingReply != nil {
				select {
				case <-awaitingReply:
				case <-stopDrain:
					return
				}
			}
			for {
				select {
				case reply := <-queries:
					select {
					case <-reply:
					case <-stopDrain:
						return
					}
				case <-stopDrain:
					return
				}
			}
		}()
		_ = sm.Stop()
		close(stopDrain)
		<-drainDone
	})
	select {
	case <-filled:
	case <-time.After(5 * time.Second):
		t.Fatal("ordinary requests did not fill the input queue")
	}
	sm.Start()

	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	queriesDuringBatch := 0
	for {
		select {
		case reply := <-queries:
			awaitingReply = reply
			select {
			case <-reply:
				awaitingReply = nil
			case <-deadline.C:
				t.Fatal("ordinary query starved behind header work")
			}
			_, height := sm.chain.BestHeader()
			if height == 1 {
				queriesDuringBatch++
			}
		case <-completed:
			require.Positive(t, queriesDuringBatch,
				"ordinary queries must run after header work starts and before it completes")
			require.Equal(t, len(headers.Headers), work.next,
				"continuations must finish despite a busy input queue")
			_, height := sm.chain.BestHeader()
			require.Equal(t, int32(2), height)
			require.True(t, p.Connected(), "valid replay must not disconnect the peer")
			return
		case <-deadline.C:
			t.Fatal("header work starved behind ordinary queries")
		}
	}
}

func TestHeaderSchedulerDiscardsDisconnectedPeer(t *testing.T) {
	sm, cleanup := makeMockSyncManager(t, &chaincfg.MainNetParams)
	t.Cleanup(cleanup)
	p := connectedSchedulerPeer(t, sm)
	p.Disconnect()
	sm.Start()
	t.Cleanup(func() { _ = sm.Stop() })

	// Accessing the nil header would panic if a disconnected batch reaches
	// validation. Queue the work directly so the new QueueHeaders disconnect
	// escape cannot hide a scheduler regression by declining the enqueue.
	// Completion must still release an already waiting receive callback.
	completed := make(chan struct{})
	sm.msgChan <- &headersMsg{
		headers: &wire.MsgHeaders{Headers: []*wire.BlockHeader{nil}},
		peer:    p,
		reply:   completed,
	}
	select {
	case <-completed:
	case <-time.After(5 * time.Second):
		t.Fatal("disconnected peer left its header callback blocked")
	}
	_, height := sm.chain.BestHeader()
	require.Zero(t, height)
}
