// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"context"
	"encoding/hex"

	"github.com/taytr-llc/slug/internal/operatorauthority"
)

type operatorActionKey struct{}
type operatorAction struct {
	publication *ManifestPublication
	operation   *operatorauthority.Operation
}

// Only immediate concrete publication operations retain this context. Prepared
// handles hold no signer Operation and cannot extend its original lifetime.
func (p *ManifestPublication) beginOperatorAction(ctx context.Context) (context.Context, func(), error) {
	if !p.codec.operator {
		return ctx, func() {}, nil
	}
	if p.store.readOnly || p.operator == nil {
		return nil, nil, ErrReadOnly
	}
	op, err := p.operator.Begin(ctx, p.binding)
	if err != nil {
		return nil, nil, err
	}
	return context.WithValue(op.Context(), operatorActionKey{}, operatorAction{p, op}), func() { _ = op.Close() }, nil
}

func (p *ManifestPublication) operatorAction(ctx context.Context) (*operatorauthority.Operation, error) {
	if ctx == nil {
		return nil, ErrPublicOwnership
	}
	action, ok := ctx.Value(operatorActionKey{}).(operatorAction)
	if !ok || action.publication != p || p.operator == nil {
		return nil, ErrPublicOwnership
	}
	if err := p.operator.CheckOperation(action.operation, p.binding); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return action.operation, nil
}
func (p *ManifestPublication) checkOperatorAction(ctx context.Context) error {
	if !p.codec.operator {
		return nil
	}
	_, err := p.operatorAction(ctx)
	return err
}

func (p *ManifestPublication) signAdmission(ctx context.Context, a PublicAdmissionStatement) (string, error) {
	if !p.codec.operator {
		return publicSign("SLUG fighter ownership admission statement", a)
	}
	op, err := p.operatorAction(ctx)
	if err != nil {
		return "", err
	}
	raw, err := publicUnsigned(a)
	if err != nil {
		return "", err
	}
	sig, err := op.SignAdmission(raw)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sig[:]), nil
}
func (p *ManifestPublication) signPublication(ctx context.Context, e PublicPublicationEntry) (string, error) {
	if !p.codec.operator {
		return publicSign("SLUG fighter ownership publication statement", e)
	}
	op, err := p.operatorAction(ctx)
	if err != nil {
		return "", err
	}
	raw, err := publicUnsigned(e)
	if err != nil {
		return "", err
	}
	sig, err := op.SignPublication(raw)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sig[:]), nil
}

// CommitOperatorSigned accepts only the original concrete online owner's live
// operation, held by the core across its canonical proof, signing and this sync
// commit. This method never signs a caller-supplied chain assertion. It verifies
// exact unsigned intent bytes, signature, original pending/head and cutoff intent
// through the existing commit. The caller closes op only after return.
func (v *PreparedPublicPublication) CommitOperatorSigned(ctx context.Context, op *operatorauthority.Operation, entry PublicPublicationEntry) (PublicPublicationEntry, error) {
	if v == nil {
		return PublicPublicationEntry{}, ErrPublicOwnership
	}
	return v.commitOperatorSigned(ctx, op, entry, v.cutoff)
}

// CommitOperatorCutoffSigned resumes the exact pending reference using the
// already signed cutoff authenticated by the original core. No new cutoff
// signature, boundary timestamp, current-head or event authority is granted.
func (v *PreparedPublicPublication) CommitOperatorCutoffSigned(ctx context.Context, op *operatorauthority.Operation, entry PublicPublicationEntry, cutoff PublicCutoffRecord) (PublicPublicationEntry, error) {
	if v == nil || v.intent.Kind != "cutoff" {
		return PublicPublicationEntry{}, ErrPublicOwnership
	}
	return v.commitOperatorSigned(ctx, op, entry, &cutoff)
}
func (v *PreparedPublicPublication) commitOperatorSigned(ctx context.Context, op *operatorauthority.Operation, entry PublicPublicationEntry, cutoff *PublicCutoffRecord) (PublicPublicationEntry, error) {
	if v.owner == nil || !v.owner.codec.operator || v.owner.operator == nil {
		return PublicPublicationEntry{}, ErrPublicOwnership
	}
	p := v.owner
	if err := p.operator.CheckOperation(op, p.binding); err != nil {
		return PublicPublicationEntry{}, err
	}
	ctx, done, err := p.acquire(ctx)
	if err != nil {
		return PublicPublicationEntry{}, err
	}
	defer done()
	// Both the method caller and original signing operation can cancel this
	// exact commit. A new caller context cannot revive an ended operation.
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(op.Context(), cancel)
	defer func() { stop(); cancel() }()
	work = context.WithValue(work, operatorActionKey{}, operatorAction{p, op})
	if err = p.checkOperatorAction(work); err != nil {
		return PublicPublicationEntry{}, err
	}
	return p.commitSigned(work, v.intent, nil, entry, cutoff)
}
