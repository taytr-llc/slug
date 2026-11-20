// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"encoding/hex"
	"errors"
	"io"
	"strconv"

	"github.com/taytr-llc/slug/btcjson"
)

// This is a public lifetime nonce, not private key material.
func newChainObservationInstance(random io.Reader) (string, error) {
	var id [32]byte
	if _, err := io.ReadFull(random, id[:]); err != nil {
		return "", errors.New("chain observation instance unavailable")
	}
	if id == [32]byte{} {
		return "", errors.New("zero chain observation instance")
	}
	return hex.EncodeToString(id[:]), nil
}

func handleGetChainObservation(s *rpcServer, _ interface{}, _ <-chan struct{}) (interface{}, error) {
	if s == nil || s.chainObservationInstance == "" {
		return nil, btcjson.NewRPCError(btcjson.ErrRPCInternal.Code, "chain observation unavailable")
	}
	observation, err := s.cfg.Chain.BestObservation()
	if err != nil {
		return nil, btcjson.NewRPCError(btcjson.ErrRPCInternal.Code, err.Error())
	}
	return &btcjson.GetChainObservationResult{
		Version: 1, InstanceID: s.chainObservationInstance,
		Revision: strconv.FormatUint(observation.Revision, 10),
		Hash:     observation.Hash.String(), Height: observation.Height,
	}, nil
}
