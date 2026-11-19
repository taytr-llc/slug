// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/seednode"
)

// A pointer is the owner epoch. Neither a stale startup nor a cancelled borrower
// may clear the tracked slot or close the processes of a successor.
type resultNodeOwner struct {
	nodes  *pilotNodes
	life   context.Context
	cancel context.CancelFunc
	done   chan struct{}
	ready  bool
}

func (s *StableController) resultNodeLease(ctx context.Context, fn func() error) error {
	lease := s.stableConfig.ResultNodeLease
	if lease == nil {
		return gymresult.ErrUnauthorized
	}
	return lease(ctx, func(a gymaccount.Account, check func() error) error {
		if a.ID != s.account.AccountID || a.Generation != s.view.Generation || check == nil {
			return gymresult.ErrUnauthorized
		}
		if e := check(); e != nil {
			return e
		}
		if e := fn(); e != nil {
			return e
		}
		return check()
	})
}

// ensureResultNodes is invoked only by an explicitly admitted Result action,
// after its source prerequisite has been checked. It never opens a wallet,
// registry, source service, mining worker or block template.
func (s *StableController) ensureResultNodes(ctx context.Context) error {
	if s.sharedPin != nil {
		return s.resultNodeLease(ctx, func() error {
			s.stableMu.Lock()
			defer s.stableMu.Unlock()
			if s.stableClosed || s.stopping || !s.resultAttached || s.resultStore == nil {
				return gymresult.ErrUnavailable
			}
			if s.account.AccountID != s.resultPin.PublisherAccountID {
				return gymresult.ErrForbidden
			}
			if !sharedPoolNodesReady(s.sharedPublisher) {
				return gymresult.ErrNodeOffline
			}
			return nil
		})
	}
	var owner *resultNodeOwner
	var already bool
	e := s.resultNodeLease(ctx, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed || s.stopping || !s.resultAttached || s.resultStore == nil {
			return gymresult.ErrUnavailable
		}
		if s.account.AccountID != s.resultPin.PublisherAccountID {
			return gymresult.ErrForbidden
		}
		if s.seedNodesReadyLocked() {
			already = true
			return nil
		}
		if s.resultNodeOwner != nil {
			return gymresult.ErrBusy
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed || s.cancel != nil || s.manifest.Phase != "ready" {
			return gymresult.ErrBusy
		}
		life, cancel := context.WithCancel(context.Background())
		owner = &resultNodeOwner{life: life, cancel: cancel, done: make(chan struct{})}
		s.resultNodeOwner = owner
		s.cancel, s.done, s.operation = cancel, owner.done, "result_nodes"
		return nil
	})
	if e != nil {
		if owner != nil {
			s.finishResultNodeStartup(owner, nil)
		}
		return e
	}
	if already {
		return nil
	}
	// The owner lifetime is independent after transfer. Until then, both caller
	// and controller cancellation bound startup and partial cleanup stays local.
	life := owner.life
	stopCaller := context.AfterFunc(ctx, func() {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.resultNodeOwner == owner && !owner.ready {
			owner.cancel()
		}
	})
	nodes, e := s.openNodes(life)
	if e == nil {
		e = s.resultNodeLease(ctx, func() error {
			s.stableMu.Lock()
			defer s.stableMu.Unlock()
			if s.resultNodeOwner != owner || s.stableClosed || s.stopping || s.mining || life.Err() != nil {
				return gymresult.ErrCancelled
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.closed || s.done != owner.done || s.operation != "result_nodes" {
				return gymresult.ErrCancelled
			}
			owner.nodes = nodes
			owner.ready = true
			s.seedNodes = nodes
			return nil
		})
	}
	stopCaller()
	if e != nil || life.Err() != nil {
		// No borrower can have been admitted while this lane's startup is active.
		s.finishResultNodeStartup(owner, nodes)
		if e != nil {
			return e
		}
		return gymresult.ErrCancelled
	}
	go s.watchResultNodes(life, owner)
	return nil
}
func (s *StableController) finishResultNodeStartup(owner *resultNodeOwner, nodes *pilotNodes) {
	owner.cancel()
	if nodes != nil {
		if nodes.Close() != nil {
			s.noteShutdown("result node startup cleanup failed")
		}
	}
	s.stableMu.Lock()
	defer s.stableMu.Unlock()
	if s.resultNodeOwner == owner {
		if s.seedNodes == nodes {
			s.seedNodes = nil
		}
		s.resultNodeOwner = nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done == owner.done && s.operation == "result_nodes" {
		s.cancel = nil
		s.operation = ""
	}
	close(owner.done)
}
func (s *StableController) watchResultNodes(ctx context.Context, owner *resultNodeOwner) {
	select {
	case <-ctx.Done():
	case <-owner.nodes.primary.done:
	case <-owner.nodes.peer.done:
	}
	owner.cancel()
	// This coordinator never holds laneGate or an account lease while draining.
	s.unregisterSeedNodes(owner.nodes)
	if owner.nodes.Close() != nil {
		s.noteShutdown("result node shutdown failed")
	}
	s.stableMu.Lock()
	if s.resultNodeOwner == owner {
		s.resultNodeOwner = nil
	}
	s.stableMu.Unlock()
	s.mu.Lock()
	if s.done == owner.done && s.operation == "result_nodes" {
		s.cancel = nil
		s.operation = ""
	}
	close(owner.done)
	s.mu.Unlock()
}
func (s *StableController) stopResultNodes() {
	s.stableMu.Lock()
	o := s.resultNodeOwner
	if o != nil {
		o.cancel()
	}
	s.stableMu.Unlock()
	if o != nil {
		<-o.done
	}
}
func (s *StableController) borrowResultPublisher(ctx context.Context) (*seednode.ResultPublisher, error) {
	s.stableMu.Lock()
	if !s.seedNodesReadyLocked() || s.stableClosed || s.stopping || s.account.AccountID != s.resultPin.PublisherAccountID {
		s.stableMu.Unlock()
		return nil, gymresult.ErrNodeOffline
	}
	if s.sharedPublisher != nil {
		binding := *s.sharedPublisher
		s.stableMu.Unlock()
		return seednode.AttachSharedPoolResult(ctx, binding)
	}
	n := s.seedNodes
	binding := seednode.OwnerBinding{Source: s.resultPin.SourcePin.SourcePin.SourcePin.SourcePin, Home: s.config.DataDir, Primary: seednode.OwnedProcess{Cmd: n.primary.cmd, Done: n.primary.done}, Peer: seednode.OwnedProcess{Cmd: n.peer.cmd, Done: n.peer.done}}
	s.stableMu.Unlock()
	p, e := seednode.AttachOwnedResult(ctx, binding)
	if e != nil {
		return nil, errors.Join(gymresult.ErrNodeOffline, e)
	}
	return p, nil
}
