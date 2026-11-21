// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettlenode

// CheckDailyProfile checks original live concrete evidence selection only.
// No RPC, signing authority, operation replacement or profile mutation occurs.
func (b *Client) CheckDailyProfile() error {
	if b == nil || b.life == nil || b.done == nil {
		return ErrInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closing || b.life.Err() != nil {
		return ErrClosed
	}
	if !b.daily {
		return ErrInvalid
	}
	return nil
}
