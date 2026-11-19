// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

type modeledEvent struct {
	Gym         string `json:"gym"`
	Winner      bool   `json:"winner"`
	RewardAtoms int64  `json:"reward_atoms"`
	Late        bool   `json:"late"`
	Sequence    uint64 `json:"sequence"`
	ShareKey    string `json:"share_key"`
	WorkHash    string `json:"work_hash"`
}

type modeledRound struct {
	Sequence      uint64       `json:"sequence"`
	BlockHash     string       `json:"block_hash"`
	FirstAccepted uint64       `json:"first_accepted"`
	LastAccepted  uint64       `json:"last_accepted"`
	ShareKeys     []string     `json:"share_keys"`
	RewardAtoms   int64        `json:"reward_atoms"`
	Allocations   []Allocation `json:"allocations"`
}

func modeledReference(t *testing.T) (poolshare.Config, []poolshare.AccountingEntry, []struct {
	Through uint64         `json:"through"`
	Pending uint64         `json:"pending_shares"`
	Rounds  []modeledRound `json:"rounds"`
}) {
	t.Helper()
	raw, err := os.ReadFile("testdata/rounds_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "83697deac6c14bafc7051f603d232328fbda52dc46466a9db51cc7ce010ec17c" {
		t.Fatal("independent rational corpus changed")
	}
	var corpus struct {
		Version uint32         `json:"version"`
		Events  []modeledEvent `json:"events"`
		Cases   []struct {
			Through uint64         `json:"through"`
			Pending uint64         `json:"pending_shares"`
			Rounds  []modeledRound `json:"rounds"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil || corpus.Version != 1 || len(corpus.Events) != 7 {
		t.Fatal("invalid modeled corpus")
	}
	cfg := poolshare.Config{PoolTarget: strings.Repeat("f", 64), PayoutAddress: "modeled-fixed-payout"}
	entries := make([]poolshare.AccountingEntry, 0, len(corpus.Events))
	for _, event := range corpus.Events {
		job := prospectjob.ManifestJob{ID: fmt.Sprintf("%064x", event.Sequence+1000), GymID: event.Gym, IssuedAtMS: 1000, ExpiresAtMS: 301000, Context: prospect.Context{ParentHeight: 61}}
		header := strings.Repeat("00", 80)
		share := poolshare.Submission{Version: 1, Sequence: event.Sequence * 10, AcceptedSequence: event.Sequence, JobID: job.ID, GymID: job.GymID, PoolTarget: cfg.PoolTarget, Header: header, WorkHash: event.WorkHash, DuplicateKey: event.ShareKey, Pool: true, Block: event.Winner, ReceivedAtMS: 1000 + int64(event.Sequence), AcceptedAtMS: 1000 + int64(event.Sequence), Acceptance: "current-parent"}
		entry := poolshare.AccountingEntry{Share: share, Job: job, RewardAtoms: event.RewardAtoms}
		if event.Winner {
			entry.Share.BlockHeight = 62
			entry.Proof = &prospect.Proof{Version: 2, JobID: job.ID, GymID: job.GymID, Header: header, MerkleBranch: []string{}}
		}
		if event.Late {
			entry.Share.Sequence = 5 // Pending intent existed before later ordinary acceptance.
			entry.Share.ReceivedAtMS = 1001
			entry.Share.AcceptedAtMS = job.ExpiresAtMS + 1
			entry.Share.Acceptance = "canonical-late-winner"
		}
		entries = append(entries, entry)
	}
	return cfg, entries, corpus.Cases
}

// These modeled projections exercise arithmetic only. The store obtains actual
// accepted work exclusively through a concrete, independently tested Pool.
func TestRoundIndependentFractionsAndFixedWindows(t *testing.T) {
	cfg, entries, cases := modeledReference(t)
	var earlier []Round
	for _, tc := range cases {
		got, err := derive(cfg, entries[:tc.Through])
		if err != nil || len(got) != len(tc.Rounds) {
			t.Fatalf("through %d: %v", tc.Through, err)
		}
		var closed uint64
		for i, r := range got {
			want := tc.Rounds[i]
			projection := modeledRound{r.Sequence, r.BlockHash, r.FirstAccepted, r.LastAccepted, r.ShareKeys, r.RewardAtoms, r.Allocations}
			if !reflect.DeepEqual(projection, want) {
				t.Fatalf("through %d round %d differs from Python fractions: got=%+v want=%+v", tc.Through, i+1, projection, want)
			}
			sum := new(big.Rat)
			for _, a := range r.Allocations {
				value, ok := new(big.Rat).SetString(a.Numerator + "/" + a.Denominator)
				if !ok {
					t.Fatal("invalid exact fraction")
				}
				sum.Add(sum, value)
			}
			if sum.Cmp(new(big.Rat).SetInt64(r.RewardAtoms)) != 0 {
				t.Fatal("gross reward was rounded or fee deducted")
			}
			closed = r.LastAccepted
		}
		if tc.Through-closed != tc.Pending {
			t.Fatal("open shares were prematurely assigned")
		}
		for i := range earlier {
			if !reflect.DeepEqual(earlier[i], got[i]) {
				t.Fatal("late appended winner rewrote an earlier round")
			}
		}
		earlier = got
	}
	entries[2].Proof.Header = "mutated"
	if earlier[0].Winner.Proof.Header == "mutated" {
		t.Fatal("round proof aliases input")
	}
}

func TestRoundRejectsDuplicateAndNoncontiguousAcceptance(t *testing.T) {
	cfg, entries, _ := modeledReference(t)
	for _, mode := range []string{"acceptance-gap", "duplicate-share", "duplicate-winner", "different-target", "unaccepted", "delivery-field"} {
		t.Run(mode, func(t *testing.T) {
			input := append([]poolshare.AccountingEntry(nil), entries...)
			switch mode {
			case "acceptance-gap":
				input[1].Share.AcceptedSequence++
			case "duplicate-share":
				input[1].Share.DuplicateKey = input[0].Share.DuplicateKey
			case "duplicate-winner":
				input[4].Share.WorkHash = input[2].Share.WorkHash
			case "different-target":
				input[1].Share.PoolTarget = strings.Repeat("e", 64)
			case "unaccepted":
				input[2].Share.AcceptedSequence = 0
			case "delivery-field":
				input[2].Share.BlockState = "canonical-observed"
			}
			if _, err := derive(cfg, input); err == nil {
				t.Fatal("invalid modeled acceptance stream was allocated")
			}
		})
	}
}
