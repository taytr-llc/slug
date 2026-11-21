// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"

	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

// Pure completed normal-PoW scalars only. No candidate bytes, mutable receipt
// state, canonicality, expiry, readiness or spend authority can be retained.
type nonceMemoValue struct {
	JobID, GymID                   string
	Nonce                          uint32
	Header, WorkHash, DuplicateKey string
	Pool, Prospect, Block          bool
}

func (p *Pool) nonceMemoKey(ctx context.Context, work savedWork, nonce uint32) ([32]byte, error) {
	if ctx == nil {
		return [32]byte{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return [32]byte{}, err
	}
	if p.config.Version != Version2 || !templateBounds(work.Template) {
		return [32]byte{}, ErrCorrupt
	}
	raw, err := json.Marshal(struct {
		Config Config    `json:"config"`
		Work   savedWork `json:"work"`
		Nonce  uint32    `json:"nonce"`
	}{p.config, work, nonce})
	if err != nil || len(raw) > maxMetadataSize+maxWorkSize+128 {
		return [32]byte{}, ErrCorrupt
	}
	if err := ctx.Err(); err != nil {
		return [32]byte{}, err
	}
	key := sha256.Sum256(append([]byte("SLUG pool share nonce memo\x00v1\x00"), raw...))
	if err := ctx.Err(); err != nil {
		return [32]byte{}, err
	}
	return key, nil
}

// Caller already reread and prepared exact original work. Cache hits reconstruct
// an owned candidate using the unchanged CheckNonce header-prefix rule only.
func (p *Pool) checkRetainedNonce(ctx context.Context, work savedWork, prepared *prospectwork.ManifestWork, nonce uint32) (prospectwork.ShareCheck, error) {
	if p.config.Version != Version2 {
		return prepared.CheckNonce(ctx, nonce)
	}
	key, err := p.nonceMemoKey(ctx, work, nonce)
	if err != nil {
		return prospectwork.ShareCheck{}, err
	}
	v, ok := p.nonceMemo[key]
	if !ok {
		return prepared.CheckNonce(ctx, nonce)
	}
	candidate := prepared.Candidate()
	header, err := hex.DecodeString(candidate.Proof.Header)
	if err != nil || len(header) != wire.MaxBlockHeaderPayload || len(candidate.BlockHex) < 160 || candidate.BlockHex[:160] != candidate.Proof.Header || v.JobID != work.Job.ID || v.GymID != work.Job.GymID || v.Nonce != nonce {
		return prospectwork.ShareCheck{}, ErrCorrupt
	}
	binary.LittleEndian.PutUint32(header[76:80], nonce)
	if hex.EncodeToString(header) != v.Header {
		return prospectwork.ShareCheck{}, ErrCorrupt
	}
	check := prospectwork.ShareCheck{JobID: v.JobID, GymID: v.GymID, Nonce: v.Nonce, Header: v.Header, WorkHash: v.WorkHash, DuplicateKey: v.DuplicateKey, Pool: v.Pool, Prospect: v.Prospect, Block: v.Block}
	if v.Pool || v.Prospect || v.Block {
		candidate.Proof.Header = v.Header
		candidate.BlockHex = v.Header + candidate.BlockHex[160:]
		check.Candidate = &candidate
	}
	if err := ctx.Err(); err != nil {
		return prospectwork.ShareCheck{}, err
	}
	return check, nil
}

// Called only after complete retained-row validation or a successful durable
// new-row commit. No unretained nonce can consume an entry. This cache is optional:
// unexpected exhaustion cannot discard history or grant an admission extension.
func (p *Pool) rememberRetainedNonce(ctx context.Context, work savedWork, check prospectwork.ShareCheck) error {
	if p.config.Version != Version2 {
		return nil
	}
	key, err := p.nonceMemoKey(ctx, work, check.Nonce)
	if err != nil {
		return err
	}
	if _, exists := p.nonceMemo[key]; !exists && (uint64(len(p.nonceMemo)) >= p.config.MaxSubmissions || len(p.nonceMemo) >= dailyMaxSubmissions) {
		return nil
	}
	if p.nonceMemo == nil {
		p.nonceMemo = make(map[[32]byte]nonceMemoValue)
	}
	p.nonceMemo[key] = nonceMemoValue{JobID: check.JobID, GymID: check.GymID, Nonce: check.Nonce, Header: check.Header, WorkHash: check.WorkHash, DuplicateKey: check.DuplicateKey, Pool: check.Pool, Prospect: check.Prospect, Block: check.Block}
	return nil
}
