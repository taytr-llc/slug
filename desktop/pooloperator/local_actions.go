// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/poolminer"
	"github.com/taytr-llc/slug/registry"
)

var (
	ErrActionBusy           = errors.New("pool operator local action busy")
	ErrActionOutcomeUnknown = errors.New("pool operator local action outcome unknown; recover exact original request")
)

// beginAction is a local process-owner operation only. It neither grants a gym
// session nor accepts a substitute source/owner. The command cannot expose it
// as an HTTP route. Close cancels and joins every admitted operation.
func (r *Runtime) beginAction(ctx context.Context) (context.Context, func(), error) {
	if r == nil || ctx == nil || r.life == nil || r.localGate == nil {
		return nil, nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, nil, e
	}
	r.mu.Lock()
	if r.closing || r.closed || r.life.Err() != nil {
		r.mu.Unlock()
		return nil, nil, ErrUnavailable
	}
	select {
	case r.localGate <- struct{}{}:
	default:
		r.mu.Unlock()
		return nil, nil, ErrActionBusy
	}
	r.calls.Add(1)
	r.mu.Unlock()
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.life, cancel)
	done := func() { stop(); cancel(); <-r.localGate; r.calls.Done() }
	if e := r.checkInputs(work); e != nil {
		done()
		return nil, nil, e
	}
	return work, done, nil
}
func (r *Runtime) actionDisclosure(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	r.mu.Lock()
	closing := r.closing || r.closed
	r.mu.Unlock()
	if closing {
		return ErrUnavailable
	}
	return r.checkInputs(ctx)
}

// RotateCredential is an explicit local provisioning action on the original
// fixed enrollment/payee set. Token plaintext is neither retained nor returned
// here. Exact expected generation/token/expiry supports interrupted retries.
func (r *Runtime) RotateCredential(ctx context.Context, gym, audience string, expected uint32, token string, expiresAtMS int64) (poolminer.Credential, error) {
	if !identifier(gym) || !identifier(token) || audience != poolminer.PoolAudience && audience != poolminer.AdmissionAudience || expiresAtMS < 1 || expiresAtMS > poolminer.MaxTimeMS {
		return poolminer.Credential{}, ErrInvalid
	}
	work, done, e := r.beginAction(ctx)
	if e != nil {
		return poolminer.Credential{}, e
	}
	defer done()
	credential, e := r.miners.Rotate(work, gym, audience, expected, token, expiresAtMS)
	if e != nil {
		if errors.Is(e, poolminer.ErrOutcomeUnknown) {
			return poolminer.Credential{}, errors.Join(ErrActionOutcomeUnknown, e)
		}
		return poolminer.Credential{}, e
	}
	if e = r.actionDisclosure(work); e != nil {
		return poolminer.Credential{}, errors.Join(ErrActionOutcomeUnknown, e)
	}
	return credential, nil
}
func (r *Runtime) RevokeCredential(ctx context.Context, gym, audience string, expected uint32) (poolminer.Credential, error) {
	if !identifier(gym) || expected == 0 || audience != poolminer.PoolAudience && audience != poolminer.AdmissionAudience {
		return poolminer.Credential{}, ErrInvalid
	}
	work, done, e := r.beginAction(ctx)
	if e != nil {
		return poolminer.Credential{}, e
	}
	defer done()
	credential, e := r.miners.Revoke(work, gym, audience, expected)
	if e != nil {
		if errors.Is(e, poolminer.ErrOutcomeUnknown) {
			return poolminer.Credential{}, errors.Join(ErrActionOutcomeUnknown, e)
		}
		return poolminer.Credential{}, e
	}
	if e = r.actionDisclosure(work); e != nil {
		return poolminer.Credential{}, errors.Join(ErrActionOutcomeUnknown, e)
	}
	return credential, nil
}

// ImportCheckpoint accepts only an already-signed public offline envelope under
// the original profile checkpoint key. It never loads/signs with that private
// role, proves canonical membership, advances a Head or substitutes current-tip
// evidence. Normal ownership replay must still authenticate chain conditions.
func (r *Runtime) ImportCheckpoint(ctx context.Context, checkpoint chaincfg.SignedCheckpoint) (registry.PublicPublicationEntry, error) {
	if r == nil || len(checkpoint.Signature) != ed25519.SignatureSize {
		return registry.PublicPublicationEntry{}, ErrInvalid
	}
	checkpoint.Signature = append([]byte(nil), checkpoint.Signature...)
	pub, e := hex.DecodeString(r.setup.Authority.CheckpointKey)
	if e != nil || len(pub) != ed25519.PublicKeySize || uint32(checkpoint.Network) != r.setup.Authority.Source.Network || checkpoint.GenesisHash.String() != r.setup.Authority.Source.Genesis || checkpoint.Verify(&chaincfg.MainNetParams, ed25519.PublicKey(pub)) != nil {
		return registry.PublicPublicationEntry{}, ErrInvalid
	}
	work, done, e := r.beginAction(ctx)
	if e != nil {
		return registry.PublicPublicationEntry{}, e
	}
	defer done()
	entry, e := r.publisher.PublishCheckpoint(work, checkpoint)
	if e == registry.ErrPublicCapacity {
		// Preserve the original ambiguous action result even if draining or
		// its cleanup also fails; never retry the checkpoint automatically.
		e = errors.Join(e, r.updateIntake(work, false, false, "public_capacity"))
	}
	// Once the concrete publisher is entered, a durable pending reference or
	// signed entry may exist. Never blindly retry based on a transport/close error.
	if e != nil {
		return registry.PublicPublicationEntry{}, errors.Join(ErrActionOutcomeUnknown, e)
	}
	if e = r.actionDisclosure(work); e != nil {
		return registry.PublicPublicationEntry{}, errors.Join(ErrActionOutcomeUnknown, e)
	}
	entry.Payload = append([]byte(nil), entry.Payload...)
	return entry, nil
}
