// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
)

type retryTreeSpec struct {
	Leaf string          `json:"leaf"`
	Wrap *retryTreeSpec  `json:"wrap"`
	Join []retryTreeSpec `json:"join"`
}
type retryTreeWrapped struct{ cause error }

func (e retryTreeWrapped) Error() string { return "wrapped fixture error" }
func (e retryTreeWrapped) Unwrap() error { return e.cause }

type retryTreeJoined struct{ causes []error }

func (e retryTreeJoined) Error() string   { return "joined fixture errors" }
func (e retryTreeJoined) Unwrap() []error { return e.causes }

func TestScheduleRetryIndependentErrorTrees(t *testing.T) {
	raw, err := os.ReadFile("testdata/round-busy/retry-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "a7f19670f966d3c54daefec33889e2226b873425e1b0787c2a531b84c7d1748f" {
		t.Fatal("independent error table changed")
	}
	var table struct {
		Version int
		Cases   []struct {
			Name  string
			Error retryTreeSpec
			Retry bool
		}
	}
	if json.Unmarshal(raw, &table) != nil || table.Version != 1 || len(table.Cases) != 41 {
		t.Fatal("error table inventory")
	}
	leaves := map[string]error{
		"nil": nil, "unknown": errors.New("unknown fixture failure"), "cancelled": context.Canceled, "deadline": context.DeadlineExceeded,
		"schedule_busy": ErrBusy, "schedule_unavailable": ErrUnavailable, "schedule_clock": ErrClock, "schedule_invalid": ErrInvalid, "schedule_capacity": ErrCapacity, "schedule_profile": ErrProfile, "schedule_poisoned": ErrPoisoned, "schedule_storage": ErrStorage,
		"settlement_busy": poolsettle.ErrBusy, "settlement_source": poolsettle.ErrSource, "settlement_backend": poolsettle.ErrBackend, "settlement_corrupt": poolsettle.ErrCorrupt, "settlement_closed": poolsettle.ErrClosed,
		"round_busy": poolround.ErrBusy, "round_source_unavailable": poolround.ErrSourceUnavailable, "round_corrupt": poolround.ErrCorrupt, "round_closed": poolround.ErrClosed,
	}
	var build func(retryTreeSpec) error
	build = func(s retryTreeSpec) error {
		if s.Leaf != "" {
			err, ok := leaves[s.Leaf]
			if !ok || s.Wrap != nil || s.Join != nil {
				t.Fatal("unknown/mixed tree leaf", s.Leaf)
			}
			return err
		}
		if s.Wrap != nil {
			if s.Join != nil {
				t.Fatal("mixed tree wrappers")
			}
			return retryTreeWrapped{build(*s.Wrap)}
		}
		if s.Join == nil {
			t.Fatal("missing error tree")
		}
		children := make([]error, len(s.Join))
		for i, c := range s.Join {
			children[i] = build(c)
		}
		return retryTreeJoined{children}
	}
	for _, c := range table.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if got := retryable(build(c.Error)); got != c.Retry {
				t.Fatal("error-tree retry classification", got, c.Retry)
			}
		})
	}
	if errorCode(poolround.ErrBusy) != "busy" {
		t.Fatal("original round contention not identified as busy")
	}
}
