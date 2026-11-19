// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymstable"
	"strings"
)

// Dispatch chooses a closed family before decoding. Each existing codec retains its own bounds.
func poolCycleResponse(ctx context.Context, raw []byte, id uint64, command string, c PoolSurfaceAPI) ([]byte, bool, error) {
	switch {
	case strings.HasPrefix(command, "seed_"):
		if id == 0 || id > gymstable.MaxSafeInteger {
			return nil, true, gymstable.ErrInvalid
		}
		req, err := ParseSeedRequest(raw)
		response := SeedResponse{ID: id, Error: "invalid_request"}
		if err == nil {
			response = handleSeedRequest(ctx, req, c)
		}
		out, err := MarshalSeedResponse(response)
		return out, true, err
	case strings.HasPrefix(command, "card_"):
		if id == 0 || id > gymstable.MaxSafeInteger {
			return nil, true, gymstable.ErrInvalid
		}
		req, err := ParseCardRequest(raw)
		response := CardResponse{ID: id, Error: "invalid_request"}
		if err == nil {
			response = handleCardRequest(ctx, req, c)
		}
		out, err := MarshalCardResponse(response)
		return out, true, err
	case strings.HasPrefix(command, "bout_"):
		if id == 0 || id > gymstable.MaxSafeInteger {
			return nil, true, gymstable.ErrInvalid
		}
		req, err := ParseBoutRequest(raw)
		response := BoutResponse{ID: id, Error: "invalid_request"}
		if err == nil {
			response = handleBoutRequest(ctx, req, c)
		}
		out, err := MarshalBoutResponse(response)
		return out, true, err
	case strings.HasPrefix(command, "result_"):
		if id == 0 || id > gymstable.MaxSafeInteger {
			return nil, true, gymstable.ErrInvalid
		}
		req, err := ParseResultRequest(raw)
		response := ResultResponse{ID: id, Error: "invalid_request"}
		if err == nil {
			response = handleResultRequest(ctx, req, c)
		}
		out, err := MarshalResultResponse(response)
		return out, true, err
	case strings.HasPrefix(command, "career_history_"):
		if id == 0 || id > gymstable.MaxSafeInteger {
			return nil, true, gymstable.ErrInvalid
		}
		req, err := ParseCareerHistoryRequest(raw)
		response := CareerHistoryResponse{ID: id, Error: "invalid_request", Version: 1}
		if err == nil {
			response = handleCareerHistoryRequest(ctx, req, c)
		}
		out, err := MarshalCareerHistoryResponse(response)
		return out, true, err
	case strings.HasPrefix(command, "career_"):
		if id == 0 || id > gymstable.MaxSafeInteger {
			return nil, true, gymstable.ErrInvalid
		}
		req, err := ParseCareerRequest(raw)
		response := CareerResponse{ID: id, Error: "invalid_request"}
		if err == nil {
			response = handleCareerRequest(ctx, req, c)
		}
		out, err := MarshalCareerResponse(response)
		return out, true, err
	}
	return nil, false, nil
}
