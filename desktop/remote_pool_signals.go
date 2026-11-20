// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// One signal owner spans constructor admission and the initialized app. Before
// handoff, signals only cancel construction; they never call a nil/native app.
// After handoff, the same cancellation is followed by the joined quit veto.
type nativeRemoteSignals struct {
	life       context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	quit       func()
	events     chan os.Signal
	stop, done chan struct{}
	once       sync.Once
}

func newNativeRemoteSignals() *nativeRemoteSignals {
	life, cancel := context.WithCancel(context.Background())
	s := &nativeRemoteSignals{life: life, cancel: cancel, events: make(chan os.Signal, 1), stop: make(chan struct{}), done: make(chan struct{})}
	signal.Notify(s.events, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer close(s.done)
		for {
			select {
			case <-s.stop:
				return
			case <-s.events:
				s.mu.Lock()
				s.cancel()
				quit := s.quit
				s.mu.Unlock()
				if quit != nil {
					quit()
				}
			}
		}
	}()
	return s
}
func (s *nativeRemoteSignals) handoff(quit func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.life.Err() != nil || quit == nil || s.quit != nil {
		return false
	}
	s.quit = quit
	return true
}
func (s *nativeRemoteSignals) close() {
	if s == nil {
		return
	}
	s.once.Do(func() { signal.Stop(s.events); close(s.stop); <-s.done; s.cancel() })
}
