// Copyright (c) 2026 The SLUG developers. ISC license.

package registry

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

const (
	Version5                uint32 = 5
	StaticBindingVersion1   uint32 = 1
	MaxManifestMarkerSize          = 4096
	MaxManifestMetadataSize        = 4096
	MaxManifestRecordDepth         = 4
	MaxManifestPageSize            = 8
	MaxManifestPageBytes           = MaxManifestPageSize*MaxRecordSize + MaxManifestPageSize + 1
)

var ErrManifestAuthority = errors.New("invalid or mismatched manifest authority")

// ManifestAuthorityPin is explicit trusted configuration, not an issuer
// signature. Marker inspection alone does not authenticate this value.
type ManifestAuthorityPin struct {
	Version      uint32 `json:"version"`
	Network      uint32 `json:"network"`
	Genesis      string `json:"genesis"`
	AuthorityID  string `json:"authority_id"`
	DescriptorID string `json:"descriptor_id"`
}

type ManifestAuthorityInfo struct {
	Pin        ManifestAuthorityPin         `json:"pin"`
	Descriptor fighterdescriptor.Descriptor `json:"descriptor"`
	Records    uint64                       `json:"records"`
	Jobs       uint64                       `json:"jobs"`
	ReadOnly   bool                         `json:"read_only"`
}

// StaticBinding identifies the fixed candidate appearance derived at admission.
// It does not activate an official generator or establish current ownership.
type StaticBinding struct {
	Version        uint32 `json:"version"`
	DescriptorID   string `json:"descriptor_id"`
	ManifestDigest string `json:"manifest_digest"`
	PortraitDigest string `json:"portrait_digest"`
}

// ManifestRecord is immutable local issuance/admission evidence. Its provenance
// comes from verified reads of a pinned store, not from parsing arbitrary JSON.
type ManifestRecord struct {
	Version       uint32                  `json:"version"`
	Sequence      uint64                  `json:"sequence"`
	Context       prospect.Context        `json:"context"`
	Proof         prospect.Proof          `json:"proof"`
	Verified      prospect.Verified       `json:"verified"`
	Authorization gymauth.Authorization   `json:"authorization"`
	Job           prospectjob.ManifestJob `json:"job"`
	Birth         Birth                   `json:"birth"`
	Eligibility   Eligibility             `json:"eligibility"`
	StaticBinding StaticBinding           `json:"static_binding"`
}

type manifestMarker struct {
	Application  string                       `json:"application"`
	Version      uint32                       `json:"version"`
	Network      uint32                       `json:"network"`
	Genesis      string                       `json:"genesis"`
	AuthorityID  string                       `json:"authority_id"`
	DescriptorID string                       `json:"descriptor_id"`
	Descriptor   fighterdescriptor.Descriptor `json:"descriptor"`
}

func (m manifestMarker) pin() ManifestAuthorityPin {
	return ManifestAuthorityPin{m.Version, m.Network, m.Genesis, m.AuthorityID, m.DescriptorID}
}

type manifestMetadata struct {
	Version      uint32                       `json:"version"`
	Network      uint32                       `json:"network"`
	Genesis      string                       `json:"genesis"`
	AuthorityID  string                       `json:"authority_id"`
	DescriptorID string                       `json:"descriptor_id"`
	Descriptor   fighterdescriptor.Descriptor `json:"descriptor"`
	LastSequence uint64                       `json:"last_sequence"`
	JobCount     uint64                       `json:"job_count"`
	LastClockMS  int64                        `json:"last_clock_ms"`
}

func (m manifestMetadata) pin() ManifestAuthorityPin {
	return ManifestAuthorityPin{m.Version, m.Network, m.Genesis, m.AuthorityID, m.DescriptorID}
}

// ManifestStore is an explicitly initialized candidate-only schema-5 authority.
// It never migrates or enriches legacy evidence. Operations and Close serialize
// under one gate; callbacks must not reenter it. Callers bound their own queues.
type ManifestStore struct {
	failedIssueFile *os.File             // exact failed-Close descriptor; never retried
	issue           *issueRegistryState  // fresh explicitly selected recovery mode
	public          *publicManifestState // selected only by the explicit public constructor
	mu              sync.Mutex
	db              *leveldb.DB
	meta            manifestMetadata
	readOnly        bool
	closed          bool
	closeErr        error
	poisoned        error
	writeBatch      func(*leveldb.Batch, *opt.WriteOptions) error
	// Private deterministic/fault seams, never public configuration.
	now        func() time.Time
	random     io.Reader
	clockSeen  int64
	checkpoint func(string)
}

func (s *ManifestStore) mark(stage string) {
	if s.checkpoint != nil {
		s.checkpoint(stage)
	}
}
