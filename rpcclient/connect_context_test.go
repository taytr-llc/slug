// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package rpcclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func contextClient(t *testing.T, configure func(*ConnConfig)) *Client {
	t.Helper()
	config := &ConnConfig{Host: "127.0.0.1:1", Endpoint: "ws", DisableAuth: true, DisableTLS: true, DisableAutoReconnect: true, DisableConnectOnNew: true}
	if configure != nil {
		configure(config)
	}
	client, err := New(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Shutdown(); client.WaitForShutdown() })
	return client
}
func requireCancellation(t *testing.T, done <-chan error, want error) {
	t.Helper()
	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("got %v want %v", err, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("connection cancellation did not drain")
	}
}
func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("fixture did not become ready")
	}
}

func TestConnectContextCanceledBeforeDial(t *testing.T) {
	client := contextClient(t, func(c *ConnConfig) {
		c.netDialContext = func(context.Context, string, string) (net.Conn, error) {
			t.Error("dial on canceled context")
			return nil, errors.New("unexpected dial")
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.ConnectContext(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestConnectContextCancelsActualDial(t *testing.T) {
	entered := make(chan struct{})
	exited := make(chan struct{})
	client := contextClient(t, func(c *ConnConfig) {
		c.netDialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			close(entered)
			<-ctx.Done()
			close(exited)
			return nil, ctx.Err()
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.ConnectContext(ctx, 1) }()
	await(t, entered)
	cancel()
	requireCancellation(t, done, context.Canceled)
	await(t, exited)
}
func TestConnectContextCancelsStalledHandshake(t *testing.T) {
	for _, tlsMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "HTTP upgrade", true: "TLS handshake"}[tlsMode], func(t *testing.T) {
			entered := make(chan struct{})
			closed := make(chan struct{})
			client := contextClient(t, func(c *ConnConfig) {
				c.DisableTLS = !tlsMode
				c.netDialContext = func(context.Context, string, string) (net.Conn, error) {
					local, remote := net.Pipe()
					go func() { defer close(closed); defer remote.Close(); close(entered); _, _ = io.Copy(io.Discard, remote) }()
					return local, nil
				}
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- client.ConnectContext(ctx, 1) }()
			await(t, entered)
			cancel()
			requireCancellation(t, done, context.Canceled)
			await(t, closed)
			client.mtx.Lock()
			published := client.wsConn != nil
			client.mtx.Unlock()
			if published {
				t.Fatal("canceled transport published")
			}
		})
	}
}
func TestConnectContextDeadlineAndRetryWait(t *testing.T) {
	attempted := make(chan struct{}, 1)
	client := contextClient(t, func(c *ConnConfig) {
		c.netDialContext = func(context.Context, string, string) (net.Conn, error) {
			attempted <- struct{}{}
			return nil, errors.New("fixture dial failure")
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := client.ConnectContext(ctx, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("retry wait ignored deadline")
	}
	if len(attempted) != 1 {
		t.Fatal("unexpected dial attempts")
	}
}
func TestConnectContextContendedLocks(t *testing.T) {
	client := contextClient(t, nil)
	for _, lock := range []*contextMutex{&client.connectMtx, &client.mtx} {
		lock.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		err := client.ConnectContext(ctx, 1)
		cancel()
		lock.Unlock()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	}
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if client.mtx.LockContext(ctx) == nil {
			t.Fatal("canceled lock acquired")
		}
	}
	client.mtx.Lock()
	client.mtx.Unlock()
}
func TestConnectContextCanceledCleanupDoesNotWaitForStateLock(t *testing.T) {
	entered := make(chan struct{})
	client := contextClient(t, func(c *ConnConfig) {
		c.netDialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			close(entered)
			<-ctx.Done()
			return nil, ctx.Err()
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.ConnectContext(ctx, 1) }()
	await(t, entered)
	client.mtx.Lock()
	cancel()
	// Keep the state lock contended through the complete cancellation return.
	// Cleanup must not reacquire it after a canceled dial.
	select {
	case err := <-done:
		client.mtx.Unlock()
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		client.mtx.Unlock()
		t.Fatal("canceled cleanup waited for state lock")
	}
	if client.connectCancel.Load() != nil {
		t.Fatal("cancellation handle retained")
	}
}
func TestConnectContextSecondCallerCanCancel(t *testing.T) {
	entered := make(chan struct{})
	client := contextClient(t, func(c *ConnConfig) {
		c.netDialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			close(entered)
			<-ctx.Done()
			return nil, ctx.Err()
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.ConnectContext(ctx, 1) }()
	await(t, entered)
	second, stop := context.WithTimeout(context.Background(), 40*time.Millisecond)
	if err := client.ConnectContext(second, 1); !errors.Is(err, context.DeadlineExceeded) {
		stop()
		t.Fatal(err)
	}
	stop()
	cancel()
	requireCancellation(t, done, context.Canceled)
}

func TestConnectContextShutdownCancelsHandshake(t *testing.T) {
	entered := make(chan struct{})
	closed := make(chan struct{})
	client := contextClient(t, func(c *ConnConfig) {
		c.netDialContext = func(context.Context, string, string) (net.Conn, error) {
			local, remote := net.Pipe()
			go func() { defer close(closed); defer remote.Close(); close(entered); _, _ = io.Copy(io.Discard, remote) }()
			return local, nil
		}
	})
	done := make(chan error, 1)
	go func() { done <- client.ConnectContext(context.Background(), 0) }()
	await(t, entered)
	client.Shutdown()
	requireCancellation(t, done, ErrClientShutdown)
	await(t, closed)
}

// This is a real SOCKS server that accepts TCP but withholds its greeting.
// It checks cancellation inside proxy negotiation, before raw dial returns.
func TestConnectContextCancelsStalledSOCKS(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	entered := make(chan struct{})
	closed := make(chan struct{})
	var fixtureErr error
	go func() {
		defer close(closed)
		conn, err := listener.Accept()
		if err != nil {
			fixtureErr = err
			return
		}
		defer conn.Close()
		buf := make([]byte, 3)
		_, err = io.ReadFull(conn, buf)
		if err != nil {
			fixtureErr = err
			return
		}
		close(entered)
		_, _ = io.Copy(io.Discard, conn)
	}()
	client := contextClient(t, func(c *ConnConfig) { c.Proxy = listener.Addr().String() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.ConnectContext(ctx, 1) }()
	await(t, entered)
	cancel()
	requireCancellation(t, done, context.Canceled)
	await(t, closed)
	if fixtureErr != nil {
		t.Fatal(fixtureErr)
	}
}

func TestConnectContextSuccessDetachesCancellationAndConnectCompatibility(t *testing.T) {
	upgrader := websocket.Upgrader{}
	var mutex sync.Mutex
	var peers []*websocket.Conn
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		mutex.Lock()
		peers = append(peers, conn)
		mutex.Unlock()
	}))
	defer server.Close()
	defer func() {
		mutex.Lock()
		defer mutex.Unlock()
		for _, peer := range peers {
			_ = peer.Close()
		}
	}()
	for _, legacy := range []bool{false, true} {
		client := contextClient(t, func(c *ConnConfig) { c.Host = strings.TrimPrefix(server.URL, "http://") })
		ctx, cancel := context.WithCancel(context.Background())
		var err error
		if legacy {
			err = client.Connect(1)
		} else {
			err = client.ConnectContext(ctx, 1)
		}
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		if err = client.ConnectContext(context.Background(), 1); !errors.Is(err, ErrClientAlreadyConnected) {
			t.Fatal(err)
		}
		// Cancellation after a completed handshake must not close the live socket.
		mutex.Lock()
		peer := peers[len(peers)-1]
		mutex.Unlock()
		if err = peer.WriteMessage(websocket.TextMessage, []byte(`{"id":999,"result":null,"error":null}`)); err != nil {
			t.Fatal("connection canceled after success", err)
		}
		select {
		case <-client.disconnect:
			t.Fatal("live connection canceled after success")
		case <-time.After(20 * time.Millisecond):
		}
		client.Shutdown()
		client.WaitForShutdown()
	}
}

func TestConnectContextRetryAfterInterruptedInitialDisconnect(t *testing.T) {
	upgrader := websocket.Upgrader{}
	var accepted atomic.Int32
	handlerDone := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		accepted.Add(1)
		defer peer.Close()
		defer func() { handlerDone <- struct{}{} }()
		for {
			_, raw, err := peer.ReadMessage()
			if err != nil {
				return
			}
			var request struct {
				ID uint64 `json:"id"`
			}
			if json.Unmarshal(raw, &request) != nil {
				return
			}
			if peer.WriteJSON(map[string]any{"id": request.ID, "result": 7, "error": nil}) != nil {
				return
			}
		}
	}))
	defer server.Close()
	entered := make(chan struct{})
	firstClosed := make(chan struct{})
	var attempts atomic.Int32
	client := contextClient(t, func(c *ConnConfig) {
		c.Host = strings.TrimPrefix(server.URL, "http://")
		c.DisableAutoReconnect = false
		c.netDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if attempts.Add(1) == 1 {
				local, remote := net.Pipe()
				go func() {
					defer close(firstClosed)
					defer remote.Close()
					close(entered)
					_, _ = io.Copy(io.Discard, remote)
				}()
				return local, nil
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}
	})
	first := make(chan error, 1)
	go func() { first <- client.ConnectContext(context.Background(), 1) }()
	await(t, entered)
	client.Disconnect()
	requireCancellation(t, first, context.Canceled)
	await(t, firstClosed)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.ConnectContext(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if client.Disconnected() {
		t.Fatal("successful retry still marked disconnected")
	}
	height, err := client.GetBlockCount()
	if err != nil || height != 7 {
		t.Fatalf("retry transport unusable: %d %v", height, err)
	}
	if accepted.Load() != 1 || attempts.Load() != 2 {
		t.Fatalf("unexpected duplicate reconnection: accepted=%d attempts=%d", accepted.Load(), attempts.Load())
	}
	client.Shutdown()
	client.WaitForShutdown()
	await(t, handlerDone)
}
