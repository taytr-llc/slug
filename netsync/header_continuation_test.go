package netsync

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/peer"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestHeaderContinuationOnlyFromSyncPeer(t *testing.T) {
	sm, cleanup := makeMockSyncManager(t, &chaincfg.MainNetParams)
	t.Cleanup(cleanup)
	requests := make(chan *wire.MsgGetHeaders, 8)
	negotiated := make(chan struct{}, 1)
	cfg := peer.Config{
		ChainParams:     sm.chainParams,
		AllowSelfConns:  true,
		ProtocolVersion: wire.BIP0037Version,
		Listeners: peer.MessageListeners{
			OnVerAck: func(*peer.Peer, *wire.MsgVerAck) { negotiated <- struct{}{} },
		},
	}
	cfg.Listeners.OnWrite = func(_ *peer.Peer, _ int, msg wire.Message, err error) {
		if request, ok := msg.(*wire.MsgGetHeaders); ok && err == nil {
			requests <- request
		}
	}
	syncPeer, err := peer.NewOutboundPeer(&cfg, "127.0.0.1:29333")
	require.NoError(t, err)
	localConn, remoteConn := net.Pipe()
	remoteDone := make(chan error, 1)
	go func() {
		// Explicit handshake order avoids two peers simultaneously writing
		// verack into an unbuffered pipe. Afterwards, drain outgoing messages.
		run := func() error {
			msg, _, err := wire.ReadMessage(remoteConn, wire.BIP0037Version, sm.chainParams.Net)
			if err != nil {
				return err
			}
			if _, ok := msg.(*wire.MsgVersion); !ok {
				return fmt.Errorf("expected version, got %T", msg)
			}
			address := wire.NewNetAddressIPPort(net.IPv4(127, 0, 0, 1), 29333, wire.SFNodeNetwork)
			version := wire.NewMsgVersion(address, address, 1, 100)
			version.ProtocolVersion = int32(wire.BIP0037Version)
			if err := wire.WriteMessage(remoteConn, version, wire.BIP0037Version, sm.chainParams.Net); err != nil {
				return err
			}
			msg, _, err = wire.ReadMessage(remoteConn, wire.BIP0037Version, sm.chainParams.Net)
			if err != nil {
				return err
			}
			if _, ok := msg.(*wire.MsgVerAck); !ok {
				return fmt.Errorf("expected verack, got %T", msg)
			}
			if err := wire.WriteMessage(remoteConn, wire.NewMsgVerAck(), wire.BIP0037Version, sm.chainParams.Net); err != nil {
				return err
			}
			for {
				if _, _, err := wire.ReadMessage(remoteConn, wire.BIP0037Version, sm.chainParams.Net); err != nil {
					return err
				}
			}
		}
		remoteDone <- run()
	}()
	t.Cleanup(func() { syncPeer.Disconnect(); _ = remoteConn.Close() })
	syncPeer.AssociateConnection(localConn)
	select {
	case <-negotiated:
	case err := <-remoteDone:
		t.Fatalf("remote handshake failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("peer handshake did not complete")
	}
	syncPeer.UpdateLastBlockHeight(100)
	sm.peerStates[syncPeer] = &peerSyncState{
		syncCandidate:   true,
		requestedTxns:   make(map[chainhash.Hash]struct{}),
		requestedBlocks: make(map[chainhash.Hash]struct{}),
	}
	sm.syncPeer, sm.ibdMode = syncPeer, true
	otherPeer := newSyncCandidate(t, sm, 100)

	// A completed marker write proves all earlier queued getheaders writes
	// reached OnWrite. This tests absence without timing-based sleeps.
	flush := func() {
		done := make(chan struct{}, 1)
		syncPeer.QueueMessage(wire.NewMsgPing(1), done)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("peer output did not flush")
		}
		require.True(t, syncPeer.Connected())
	}

	fixture := loadHeaders(t)
	response := &headersMsg{
		headers: &wire.MsgHeaders{Headers: fixture},
		peer:    syncPeer,
	}
	require.False(t, sm.handleHeadersMsg(response), "sync response must remain pending")
	flush()
	require.Empty(t, requests, "do not request continuation between turns")

	announcement := &headersMsg{
		headers: &wire.MsgHeaders{Headers: fixture[:1]},
		peer:    otherPeer,
	}
	require.True(t, sm.handleHeadersMsg(announcement))
	flush()
	require.Empty(t, requests, "another peer must not request an overlapping continuation")

	require.True(t, sm.handleHeadersMsg(response))
	flush()
	require.Len(t, requests, 1, "the selected peer's completed response must continue IBD")
	request := <-requests
	bestHash, _ := sm.chain.BestHeader()
	require.Equal(t, []*chainhash.Hash{&bestHash}, request.BlockLocatorHashes)
	require.Equal(t, zeroHash, request.HashStop)

	require.True(t, sm.handleHeadersMsg(&headersMsg{
		headers: wire.NewMsgHeaders(), peer: syncPeer,
	}))
	sm.syncPeer = nil
	announcement.next = 0
	require.True(t, sm.handleHeadersMsg(announcement))
	flush()
	require.Empty(t, requests, "empty replies or a missing sync peer must not continue IBD")

	// Outside IBD, other peers' normal announcements still request bodies.
	sm.ibdMode = false
	announcement.next = 0
	require.Empty(t, sm.requestedBlocks)
	require.True(t, sm.handleHeadersMsg(announcement))
	require.Len(t, sm.peerStates[otherPeer].requestedBlocks, len(fixture))
}
