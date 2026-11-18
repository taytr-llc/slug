// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import "github.com/taytr-llc/slug/internal/careerstate"

type careerRootMarker struct {
	resultRootMarker
	CareerSchema     uint32                  `json:"career_schema"`
	CareerPhase      string                  `json:"career_phase"`
	CareerActivation *careerstate.Activation `json:"career_activation"`
}

func validateCareerRoot(m careerRootMarker) error {
	if m.Version != 9 || m.CareerSchema != 1 {
		return ErrManifestInitialization
	}
	switch m.CareerPhase {
	case "waiting_source":
		if m.CareerActivation != nil {
			return ErrManifestInitialization
		}
	case "prepared", "ready":
		if m.ResultPhase != "ready" {
			return ErrManifestInitialization
		}
		if m.CareerActivation == nil {
			if m.CareerPhase == "ready" {
				return ErrManifestInitialization
			}
		} else if careerstate.ValidateActivation(*m.CareerActivation) != nil || m.CareerActivation.ResultStoreID != m.ResultStoreID {
			return ErrManifestInitialization
		}
	default:
		return ErrManifestInitialization
	}
	return nil
}
