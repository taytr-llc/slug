// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

const OperatorIssuer = "issuer"
const OperatorReplica = "replica"

func selectedOperatorCodec(profile registry.OperatorProfile) (ownershipCodec, error) {
	if profile.Validate() != nil {
		return ownershipCodec{}, ErrSource
	}
	return ownershipCodec{operator: true, profile: profile}, nil
}
func operatorBinding(profile registry.OperatorProfile) (operatorauthority.Binding, error) {
	if profile.Validate() != nil {
		return operatorauthority.Binding{}, ErrSource
	}
	p := profile.Pin()
	g, e := chainhash.NewHashFromStr(p.Source.Genesis)
	if e != nil {
		return operatorauthority.Binding{}, ErrSource
	}
	return operatorauthority.Binding{Version: 2, PinID: profile.ID(), Network: wire.BitcoinNet(p.Source.Network), Genesis: *g, Keys: operatorauthority.Keys{Admission: p.AdmissionKey, Publication: p.PublicationKey, Cutoff: p.CutoffKey, Checkpoint: p.CheckpointKey}}, nil
}
func checkOnlineOwner(profile registry.OperatorProfile, owner *operatorauthority.Owner) error {
	binding, e := operatorBinding(profile)
	if e != nil {
		return e
	}
	if owner == nil || owner.Capabilities() != (operatorauthority.Capabilities{Admission: true, Publication: true, Cutoff: true}) {
		return ErrSource
	}
	return owner.Check(context.Background(), binding)
}
func CreateOperator(dir string, profile registry.OperatorProfile, id StoreIdentity, owner *operatorauthority.Owner) (*Store, error) {
	codec, e := selectedOperatorCodec(profile)
	if e != nil {
		return nil, e
	}
	if e = checkOnlineOwner(profile, owner); e != nil {
		return nil, e
	}
	return createSelectedStore(dir, profile.Pin(), id, codec, OperatorIssuer, owner)
}
func OpenOperator(dir string, profile registry.OperatorProfile, id StoreIdentity, owner *operatorauthority.Owner) (*Store, error) {
	codec, e := selectedOperatorCodec(profile)
	if e != nil {
		return nil, e
	}
	if e = checkOnlineOwner(profile, owner); e != nil {
		return nil, e
	}
	return openSelectedStore(dir, profile.Pin(), id, false, codec, OperatorIssuer, owner)
}
func CreateOperatorReplica(dir string, profile registry.OperatorProfile, id StoreIdentity) (*Store, error) {
	codec, e := selectedOperatorCodec(profile)
	if e != nil {
		return nil, e
	}
	return createSelectedStore(dir, profile.Pin(), id, codec, OperatorReplica, nil)
}
func OpenOperatorReplica(dir string, profile registry.OperatorProfile, id StoreIdentity) (*Store, error) {
	codec, e := selectedOperatorCodec(profile)
	if e != nil {
		return nil, e
	}
	return openSelectedStore(dir, profile.Pin(), id, false, codec, OperatorReplica, nil)
}

// Audit expects the persisted role independently; it never borrows a signer.
func OpenOperatorAudit(dir string, profile registry.OperatorProfile, id StoreIdentity, expectedRole string) (*Store, error) {
	codec, e := selectedOperatorCodec(profile)
	if e != nil {
		return nil, e
	}
	return openSelectedStore(dir, profile.Pin(), id, true, codec, expectedRole, nil)
}
func NewOperatorPublicSource(ctx context.Context, profile registry.OperatorProfile, publisher *registry.ManifestPublication, owner *operatorauthority.Owner) (*PublicSource, error) {
	codec, e := selectedOperatorCodec(profile)
	if e != nil {
		return nil, e
	}
	if e = checkOnlineOwner(profile, owner); e != nil {
		return nil, e
	}
	return newSelectedPublicSource(ctx, profile.Pin(), publisher, codec, owner)
}
func NewOperatorRemotePublicSource(ctx context.Context, profile registry.OperatorProfile, reader *PublicReader) (*PublicSource, error) {
	codec, e := selectedOperatorCodec(profile)
	if e != nil {
		return nil, e
	}
	return newSelectedRemotePublicSource(ctx, profile.Pin(), reader, codec)
}
func NewOperatorOwnedChainSource(ctx context.Context, profile registry.OperatorProfile, config prospectnode.RPCConfig, life context.Context) (*OwnedChainSource, error) {
	codec, e := selectedOperatorCodec(profile)
	if e != nil {
		return nil, e
	}
	return newSelectedOwnedChainSource(ctx, profile.Pin(), config, life, codec)
}
func NewOperatorPublicReader(ctx context.Context, profile registry.OperatorProfile, config PublicReadConfig, life context.Context) (*PublicReader, error) {
	codec, e := selectedOperatorCodec(profile)
	if e != nil {
		return nil, e
	}
	return newSelectedPublicReader(ctx, profile.Pin(), config, life, codec)
}
func NewOperatorPublicReadServer(profile registry.OperatorProfile, publisher *registry.ManifestPublication, store *Store, life context.Context) (*PublicReadServer, error) {
	codec, e := selectedOperatorCodec(profile)
	if e != nil {
		return nil, e
	}
	return newSelectedPublicReadServer(profile.Pin(), publisher, store, life, codec)
}

func (s *Store) selectedSource(ctx context.Context, source *PublicSource, node *OwnedChainSource) error {
	if s == nil || source == nil || source.pin != s.pin || source.codec != s.codec {
		return ErrSource
	}
	if node != nil && (node.pin != s.pin || node.codec != s.codec) {
		return ErrSource
	}
	if !s.codec.operator {
		return nil
	}
	if s.operatorRole == OperatorReplica {
		if source.reader == nil || source.publisher != nil || source.operator != nil || s.operator != nil {
			return ErrSource
		}
		return source.Alive()
	}
	if s.operatorRole != OperatorIssuer || s.operator == nil || source.operator != s.operator || source.publisher == nil || source.reader != nil {
		return ErrSource
	}
	return source.publisher.CheckOperatorOwner(ctx, s.operator)
}
func (s *Store) recaptureSource(ctx context.Context, source *PublicSource) (*PublicSource, error) {
	return newSelectedPublicSource(ctx, s.pin, source.publisher, s.codec, s.operator)
}
