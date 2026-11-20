// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/registry"
	"strings"
)

type operatorSigningKey struct{}
type operatorSigning struct {
	store *Store
	op    *operatorauthority.Operation
}

// Only immediate proof/sign/commit work owns an operation. Replay handles and
// prepared transfers retain no dormant operation and cannot extend its lifetime.
func (s *Store) beginOperatorSigning(ctx context.Context, source *PublicSource) (context.Context, func(), error) {
	if !s.codec.operator {
		return ctx, func() {}, nil
	}
	if e := s.selectedSource(ctx, source, nil); e != nil {
		return nil, nil, e
	}
	if source.reader != nil {
		return ctx, func() {}, nil
	}
	if s.readOnly || s.operatorRole != OperatorIssuer || s.operator == nil {
		return nil, nil, ErrSource
	}
	binding, e := operatorBinding(s.codec.profile)
	if e != nil {
		return nil, nil, e
	}
	op, e := s.operator.Begin(ctx, binding)
	if e != nil {
		return nil, nil, e
	}
	return context.WithValue(op.Context(), operatorSigningKey{}, operatorSigning{s, op}), func() { _ = op.Close() }, nil
}
func (s *Store) operatorOperation(ctx context.Context) (*operatorauthority.Operation, error) {
	if ctx == nil || s.operator == nil {
		return nil, ErrSource
	}
	v, ok := ctx.Value(operatorSigningKey{}).(operatorSigning)
	if !ok || v.store != s {
		return nil, ErrSource
	}
	binding, e := operatorBinding(s.codec.profile)
	if e != nil {
		return nil, e
	}
	if e = s.operator.CheckOperation(v.op, binding); e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return v.op, nil
}
func (s *Store) checkOperatorSigning(ctx context.Context) error {
	if !s.codec.operator || s.operatorRole == OperatorReplica {
		return nil
	}
	_, e := s.operatorOperation(ctx)
	return e
}
func (s *Store) signSelectedObject(ctx context.Context, domain string, v any) (string, error) {
	if !s.codec.operator {
		return signObject(domain, v)
	}
	op, e := s.operatorOperation(ctx)
	if e != nil {
		return "", e
	}
	raw, e := json.Marshal(v)
	if e != nil {
		return "", ErrInvalid
	}
	at := strings.LastIndex(string(raw), `,"signature":`)
	if at < 0 {
		return "", ErrInvalid
	}
	raw = append(append([]byte{}, raw[:at]...), '}')
	var signature [64]byte
	switch domain {
	case "SLUG fighter ownership publication statement":
		signature, e = op.SignPublication(raw)
	case "SLUG fighter ownership boundary statement":
		signature, e = op.SignBoundary(raw)
	case "SLUG fighter ownership cutoff statement":
		signature, e = op.SignCutoff(raw)
	case "SLUG fighter ownership finality statement":
		signature, e = op.SignFinalityFrame(raw)
	default:
		return "", ErrInvalid
	}
	if e != nil {
		return "", e
	}
	return hex.EncodeToString(signature[:]), nil
}
func (s *Store) commitSelectedPublication(ctx context.Context, p *registry.PreparedPublicPublication, entry PublicationEntry) (PublicationEntry, error) {
	if !s.codec.operator {
		return p.CommitSigned(ctx, entry)
	}
	op, e := s.operatorOperation(ctx)
	if e != nil {
		return PublicationEntry{}, e
	}
	return p.CommitOperatorSigned(ctx, op, entry)
}
func (s *Store) commitSelectedCutoffPublication(ctx context.Context, p *registry.PreparedPublicPublication, entry PublicationEntry, cutoff CutoffRecord) (PublicationEntry, error) {
	if !s.codec.operator {
		return p.CommitCutoffSigned(ctx, entry, cutoff)
	}
	op, e := s.operatorOperation(ctx)
	if e != nil {
		return PublicationEntry{}, e
	}
	return p.CommitOperatorCutoffSigned(ctx, op, entry, cutoff)
}
