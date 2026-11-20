// Copyright (c) 2026 The SLUG developers. ISC license.
package btcjson

// GetChainObservationCmd asks for an atomic, revisioned local best-chain view.
type GetChainObservationCmd struct{}

func NewGetChainObservationCmd() *GetChainObservationCmd {
	return &GetChainObservationCmd{}
}

// GetChainObservationResult keeps the uint64 revision as decimal text so JSON
// consumers never round it. InstanceID identifies the concrete RPC lifetime.
type GetChainObservationResult struct {
	Version    uint32 `json:"version"`
	InstanceID string `json:"instance_id"`
	Revision   string `json:"revision"`
	Hash       string `json:"hash"`
	Height     int32  `json:"height"`
}

func init() {
	MustRegisterCmd("getchainobservation", (*GetChainObservationCmd)(nil), 0)
}
