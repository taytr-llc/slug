// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"

	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
)

// RecoverablePoolConfig is an explicit fresh selection, never an interpretation
// of the unidentified DAILY issuance hold. Base keeps its original V2 bytes.
type RecoverablePoolConfig struct {
	Version            uint32                `json:"version"`
	Base               PoolConfig            `json:"base"`
	IssueProfile       registry.IssueProfile `json:"issue_profile"`
	SettlementConfigID string                `json:"settlement_config_id"`
	RecipientScript    string                `json:"recipient_script"`
	TransportTrustID   string                `json:"transport_trust_id"`
}

type RecoverableDailyAuthority interface {
	poolservice.Authority
	ResumeIssue(context.Context, string) (poolservice.IssueResponse, error)
	LookupIssue(context.Context, string) (poolservice.IssueResponse, error)
}

func ValidateRecoverableDailyPoolConfig(c RecoverablePoolConfig) (RecoverablePoolConfig, error) {
	if c.Version != 1 {
		return RecoverablePoolConfig{}, ErrCorrupt
	}
	base, e := ValidateDailyPoolConfig(c.Base)
	if e != nil || base != c.Base || !poolRequestID(c.SettlementConfigID) || len(c.RecipientScript) != 44 || c.RecipientScript[:4] != "0014" {
		return RecoverablePoolConfig{}, ErrCorrupt
	}
	raw, e := hex.DecodeString(c.RecipientScript)
	if e != nil || hex.EncodeToString(raw) != c.RecipientScript {
		return RecoverablePoolConfig{}, ErrCorrupt
	}
	pid, e := registry.IssueProfileID(c.IssueProfile)
	if e != nil || c.IssueProfile.MaxRequests != c.Base.Source.MaxWorks {
		return RecoverablePoolConfig{}, ErrCorrupt
	}
	// Source identity is validated by the pure service selection against its
	// complete original source. No credential/client is constructed here.
	if c.IssueProfile.SourceConfigID != issueSourceID(c.Base) {
		return RecoverablePoolConfig{}, ErrCorrupt
	}
	payload := []byte(c.Base.TransportTrustID + pid)
	h := sha256.New()
	h.Write([]byte("SLUG pool issue recovery transport\x00v1\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(payload)))
	h.Write(n[:])
	h.Write(payload)
	if c.TransportTrustID != hex.EncodeToString(h.Sum(nil)) {
		return RecoverablePoolConfig{}, ErrCorrupt
	}
	return c, nil
}

func (c RecoverablePoolConfig) selection() poolservice.RecoverableDailySelection {
	return poolservice.RecoverableDailySelection{Version: 1, BaseVersion: 2, URL: c.Base.AuthorityOrigin, PublicKey: c.Base.PublicKey, Source: c.Base.Source, IssueProfile: c.IssueProfile, SettlementConfigID: c.SettlementConfigID, RecipientScript: c.RecipientScript, BaseTransportTrustID: c.Base.TransportTrustID, TransportTrustID: c.TransportTrustID}
}

func NewRecoverableDailyPool(c RecoverablePoolConfig, client *poolservice.Client, authority RecoverableDailyAuthority, admission ManifestAuthority, node Node, signer Signer, outbox *PoolOutbox) (*PoolWorker, error) {
	if e := manifestProduction(); e != nil {
		return nil, e
	}
	c, e := ValidateRecoverableDailyPoolConfig(c)
	if e != nil {
		return nil, e
	}
	if client == nil || authority == nil || admission == nil || node == nil || signer == nil || outbox == nil || client.CheckRecoverableDailySelection(c.selection()) != nil {
		return nil, ErrCorrupt
	}
	w := &PoolWorker{config: c.Base, authority: authority, admission: admission, node: node, signer: signer, outbox: outbox, now: time.Now, wait: waitFor, status: Status{State: "ready"}, recovery: &poolRecoveryWorker{c, client, authority}, check: func(ctx context.Context, work *prospectwork.ManifestWork, n uint32) (prospectwork.ShareCheck, error) {
		return work.CheckNonce(ctx, n)
	}}
	if !w.authorityMatches() {
		return nil, ErrCorrupt
	}
	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if outbox.state() != nil || outbox.recovery == nil || outbox.recovery.readOnly || outbox.recovery.config != c || outbox.config != c.Base {
		return nil, ErrCorrupt
	}
	return w, nil
}

type poolRecoveryWorker struct {
	config    RecoverablePoolConfig
	client    *poolservice.Client
	authority RecoverableDailyAuthority
}

func (w *PoolWorker) recoverableAuthorityMatches() bool {
	r := w.recovery
	if r == nil || r.client.CheckRecoverableDailySelection(r.config.selection()) != nil {
		return false
	}
	c, a, m := r.config, w.authority, w.admission
	return a.ProtocolVersion() == 3 && a.AuthorityOrigin() == c.Base.AuthorityOrigin && a.SourceConfig() == c.Base.Source && a.PublicKey() == c.Base.PublicKey && a.TransportTrustID() == c.TransportTrustID && m.ProtocolVersion() == 3 && m.AuthorityOrigin() == c.Base.AdmissionOrigin && m.AuthorityPin() == c.Base.Source.AuthorityPin && m.TransportTrustID() == c.Base.AdmissionTrustID
}

func (b *PoolOutbox) recoveryFits(m poolOutboxMetadata) bool {
	c := b.recovery.config
	if m.IssuePending || m.Config != c.Base || m.Requests > c.IssueProfile.MaxRequests || m.Requests > c.Base.MaxRecords || m.Records > c.Base.MaxRecords-m.Requests || m.Bytes > c.Base.MaxBytes || m.Reserved > c.Base.MaxBytes-m.Bytes {
		return false
	}
	slot := uint64(0)
	if m.ActiveRequestID != "" || m.FirstReserved != 0 {
		slot = 1
	}
	return slot <= c.Base.MaxRecords-m.Requests-m.Records
}

func (w *PoolWorker) recoveryIssueFull() bool {
	b := w.outbox
	if b.meta.ActiveRequestID != "" {
		return false
	}
	return b.meta.Requests >= b.recovery.config.IssueProfile.MaxRequests || b.meta.Requests > b.config.MaxRecords || b.meta.Records > b.config.MaxRecords-b.meta.Requests || b.config.MaxRecords-b.meta.Requests-b.meta.Records < 2
}

var errPoolIssueTerminal = errors.New("original pool issuance terminal")
