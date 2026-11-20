// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/wire/v2"
	"sync"
	"time"
)

type storeCounts struct {
	Admissions   uint32 `json:"admissions"`
	Publications uint32 `json:"publications"`
	Headers      uint32 `json:"headers"`
	Carriers     uint32 `json:"carriers"`
	Witnesses    uint32 `json:"witnesses"`
	States       uint32 `json:"states"`
	Maps         uint32 `json:"maps"`
	Cutoffs      uint32 `json:"cutoffs"`
	Boundaries   uint32 `json:"boundaries"`
	Conflicts    uint32 `json:"conflicts"`
}
type storeMetadata struct {
	Version                   uint32      `json:"version"`
	PinID                     string      `json:"pin_id"`
	StoreID                   string      `json:"store_id"`
	Revision                  uint32      `json:"revision"`
	ClockHighWaterMS          int64       `json:"clock_high_water_ms"`
	Counts                    storeCounts `json:"counts"`
	LogicalBytes              uint32      `json:"logical_bytes"`
	CurrentHead               *HeadRef    `json:"current_head"`
	CurrentMapID              string      `json:"current_map_id"`
	WorkID                    string      `json:"work_id"`
	FinalitySequence          uint32      `json:"finality_sequence"`
	PublishedFinalitySequence uint32      `json:"published_finality_sequence"`
	FinalityHead              string      `json:"finality_head"`
}

// Store owns only public evidence. Persisted heads are unavailable after reopen
// until replay re-establishes required work and exact concrete observation.
type Store struct {
	mu                         sync.Mutex
	gate, projection           chan struct{}
	life                       context.Context
	cancel                     context.CancelFunc
	closeOnce                  sync.Once
	closed, poisoned, readOnly bool
	closeErr                   error
	db                         *leveldb.DB
	pin                        Pin
	pinID                      string
	identity                   StoreIdentity
	meta                       storeMetadata
	head                       HeadRef
	generation                 uint64
	ready                      bool
	source                     *PublicSource
	node                       *OwnedChainSource
	observation                ChainObservation
	effective, states          map[string]EffectiveState
	admissions                 map[uint32]AdmissionStatement
	admissionByProspect        map[string]uint32
	maps                       map[string]OwnershipMap
	cutoffs                    map[string]CutoffRecord
	cutoffByID                 map[string]string
	portable                   map[string]bool
	publications               map[uint32]PublicationEntry
	headers                    map[string]HeaderRow
	canonical                  map[uint32]string
	witnesses                  map[string]CarrierWitness
	frames                     map[uint32]FinalityFrame
	frameBodies                map[uint32]FinalityFrameBody
	finalities                 map[string]FinalityWitness
	boundaries                 map[string]SignedBoundary
	conflicts                  map[string]ConflictRecord
	work                       *replayWork
	replay                     *Replay
	commits                    map[string]TransactionWitness
	keySizes                   map[string]int
	transferCounts             map[string]uint32
	now                        func() time.Time
	writeBatch                 func(*leveldb.Batch, *opt.WriteOptions) error
}

func normal() error {
	if wire.TestPoW {
		return ErrProfile
	}
	return nil
}
func (s *Store) state() error {
	if e := normal(); e != nil {
		return e
	}
	if s == nil || s.closed || s.life == nil || s.life.Err() != nil {
		return ErrClosed
	}
	if s.poisoned {
		return ErrOutcomeUnknown
	}
	return nil
}
func (s *Store) Pin() Pin {
	if s == nil {
		return Pin{}
	}
	return s.pin
}
func (s *Store) Identity() StoreIdentity {
	if s == nil {
		return StoreIdentity{}
	}
	return s.identity
}
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.cancel()
		s.gate <- struct{}{}
		s.projection <- struct{}{}
		s.mu.Lock()
		s.closed = true
		s.ready = false
		s.generation++
		if s.db != nil {
			s.closeErr = s.db.Close()
		}
		s.mu.Unlock()
		<-s.projection
		<-s.gate
	})
	return s.closeErr
}
