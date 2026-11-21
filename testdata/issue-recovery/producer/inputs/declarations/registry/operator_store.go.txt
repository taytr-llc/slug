// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"context"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/wire/v2"
)

func operatorBinding(profile OperatorProfile) (operatorauthority.Binding, error) {
	if err := profile.Validate(); err != nil {
		return operatorauthority.Binding{}, err
	}
	p := profile.Pin()
	genesis, err := chainhash.NewHashFromStr(p.Source.Genesis)
	if err != nil {
		return operatorauthority.Binding{}, ErrPublicOwnership
	}
	return operatorauthority.Binding{
		Version: 2, PinID: profile.ID(), Network: wire.BitcoinNet(p.Source.Network), Genesis: *genesis,
		Keys: operatorauthority.Keys{Admission: p.AdmissionKey, Publication: p.PublicationKey, Cutoff: p.CutoffKey, Checkpoint: p.CheckpointKey},
	}, nil
}

func selectedOperatorState(profile OperatorProfile, owner *operatorauthority.Owner, readOnly bool) (*publicManifestState, error) {
	binding, err := operatorBinding(profile)
	if err != nil {
		return nil, err
	}
	if readOnly {
		if owner != nil {
			return nil, ErrPublicOwnership
		}
	} else {
		if owner == nil || owner.Capabilities() != (operatorauthority.Capabilities{Admission: true, Publication: true, Cutoff: true}) {
			return nil, ErrPublicOwnership
		}
		if err = owner.Check(context.Background(), binding); err != nil {
			return nil, err
		}
	}
	s := newSelectedPublicState(profile.Pin(), publicEvidenceCodec{operator: true})
	s.operator, s.binding = owner, binding
	return s, nil
}

// CreateOperatorManifestAuthority creates a fresh v2 guard and retains the exact
// already-open online signer. It neither loads keys nor adopts/migrates a v1 root.
// The caller owns the signer and closes this store before releasing that owner.
func CreateOperatorManifestAuthority(path string, d fighterdescriptor.Descriptor, profile OperatorProfile, owner *operatorauthority.Owner) (*ManifestStore, error) {
	selected, err := selectedOperatorState(profile, owner, false)
	if err != nil {
		return nil, err
	}
	return createSelectedPublicManifestAuthority(path, d, selected)
}

func OpenOperatorManifestAuthority(path string, profile OperatorProfile, owner *operatorauthority.Owner) (*ManifestStore, error) {
	selected, err := selectedOperatorState(profile, owner, false)
	if err != nil {
		return nil, err
	}
	return openSelectedPublicManifestAuthority(path, selected, false)
}

// Audit requires independently expected public configuration and no signer.
// Its borrowed publication supports public reads; every mutation still refuses.
func OpenOperatorManifestAuthorityAudit(path string, profile OperatorProfile) (*ManifestStore, error) {
	selected, err := selectedOperatorState(profile, nil, true)
	if err != nil {
		return nil, err
	}
	return openSelectedPublicManifestAuthority(path, selected, true)
}

// OperatorPublication never substitutes an equal replacement signer. Writer
// identity was retained at the explicit store constructor; old handles stay dead.
func (s *ManifestStore) OperatorPublication(expected OperatorProfile) (*ManifestPublication, error) {
	if err := expected.Validate(); err != nil {
		return nil, err
	}
	return s.selectedPublicPublication(expected.Pin(), publicEvidenceCodec{operator: true})
}
