// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package rpcclient

import (
	"context"
	"sync"
)

// contextMutex is a zero-value-ready connection lock. Context acquisition does
// not leave a goroutine waiting on a sync.Mutex after the caller has canceled.
type contextMutex struct {
	once  sync.Once
	token chan struct{}
}

func (m *contextMutex) init()   { m.once.Do(func() { m.token = make(chan struct{}, 1) }) }
func (m *contextMutex) Lock()   { m.init(); m.token <- struct{}{} }
func (m *contextMutex) Unlock() { <-m.token }
func (m *contextMutex) LockContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.init()
	select {
	case m.token <- struct{}{}:
		if err := ctx.Err(); err != nil {
			m.Unlock()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// connectAttempt lets cancellation cleanup avoid reacquiring connection state.
type connectAttempt struct{ cancel context.CancelFunc }
