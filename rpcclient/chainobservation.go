// Copyright (c) 2026 The SLUG developers. ISC license.
package rpcclient

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/taytr-llc/slug/btcjson"
)

var (
	ErrChainObservationBusy    = errors.New("chain observation busy")
	ErrInvalidChainObservation = errors.New("invalid chain observation response")
)

// FutureGetChainObservationResult is a promise for a local atomic observation.
// Callers still own transport deadlines, node identity and any cutoff clock.
type FutureGetChainObservationResult chan *Response

func observationHex(s string, nonzero bool) bool {
	if len(s) != 64 {
		return false
	}
	b, err := hex.DecodeString(s)
	if err != nil || hex.EncodeToString(b) != s {
		return false
	}
	return !nonzero || !bytes.Equal(b, make([]byte, 32))
}

func (r FutureGetChainObservationResult) Receive() (*btcjson.GetChainObservationResult, error) {
	raw, err := ReceiveFuture(r)
	if err != nil {
		var rpcErr *btcjson.RPCError
		if errors.As(err, &rpcErr) && rpcErr.Code == btcjson.ErrRPCInternal.Code && rpcErr.Message == "chain observation busy" {
			return nil, ErrChainObservationBusy
		}
		return nil, err
	}
	if len(raw) > 512 {
		return nil, ErrInvalidChainObservation
	}
	var value btcjson.GetChainObservationResult
	if json.Unmarshal(raw, &value) != nil {
		return nil, ErrInvalidChainObservation
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(canonical, raw) || value.Version != 1 || value.Height < 0 ||
		!observationHex(value.InstanceID, true) || !observationHex(value.Hash, false) {
		return nil, ErrInvalidChainObservation
	}
	revision, err := strconv.ParseUint(value.Revision, 10, 64)
	if err != nil || revision == 0 || strconv.FormatUint(revision, 10) != value.Revision {
		return nil, ErrInvalidChainObservation
	}
	return &value, nil
}

func (c *Client) GetChainObservationAsync() FutureGetChainObservationResult {
	return c.SendCmd(btcjson.NewGetChainObservationCmd())
}

// GetChainObservation returns a strict v1 node observation. A busy response is
// explicit and has no older-tip fallback. This is not ownership or finality.
func (c *Client) GetChainObservation() (*btcjson.GetChainObservationResult, error) {
	return c.GetChainObservationAsync().Receive()
}
