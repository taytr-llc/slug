// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

type liveModeledCase struct {
	Through uint64         `json:"through"`
	Pending uint64         `json:"pending_shares"`
	Rounds  []modeledRound `json:"rounds"`
}
type liveReference struct {
	Version  uint32 `json:"version"`
	Extended struct {
		Events []modeledEvent    `json:"events"`
		Cases  []liveModeledCase `json:"cases"`
	} `json:"extended_arithmetic"`
	Trace []struct {
		Op      string         `json:"op"`
		Cursor  uint64         `json:"cursor"`
		Records []modeledRound `json:"records"`
	} `json:"trace"`
}

func readLiveReference(t *testing.T) liveReference {
	t.Helper()
	raw, err := os.ReadFile("testdata/live-round/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "e26f0045170f1a25b91663e45d69309a420f310884cdd091fbf92710c37985fc" {
		t.Fatal("independent live-round reference changed")
	}
	var corpus liveReference
	if json.Unmarshal(raw, &corpus) != nil || corpus.Version != 1 || len(corpus.Extended.Events) != 23 || len(corpus.Extended.Cases) != 2 || len(corpus.Trace) != 15 {
		t.Fatal("invalid independent corpus")
	}
	return corpus
}

func liveModeledEntries(c poolshare.Config, events []modeledEvent) []poolshare.AccountingEntry {
	entries := make([]poolshare.AccountingEntry, 0, len(events))
	for _, e := range events {
		job := prospectjob.ManifestJob{ID: fmt.Sprintf("%064x", e.Sequence+1000), GymID: e.Gym, IssuedAtMS: 1000, ExpiresAtMS: 301000, Context: prospect.Context{ParentHeight: 61}}
		header := strings.Repeat("00", 80)
		share := poolshare.Submission{Version: 1, Sequence: e.Sequence * 10, AcceptedSequence: e.Sequence, JobID: job.ID, GymID: job.GymID, PoolTarget: c.PoolTarget, Header: header, WorkHash: e.WorkHash, DuplicateKey: e.ShareKey, Pool: true, Block: e.Winner, ReceivedAtMS: 1000 + int64(e.Sequence), AcceptedAtMS: 1000 + int64(e.Sequence), Acceptance: "current-parent"}
		entry := poolshare.AccountingEntry{Share: share, Job: job, RewardAtoms: e.RewardAtoms}
		if e.Winner {
			entry.Share.BlockHeight = 62
			entry.Proof = &prospect.Proof{Version: 2, JobID: job.ID, GymID: job.GymID, Header: header, MerkleBranch: []string{}}
		}
		if e.Late {
			entry.Share.Sequence = e.Sequence*10 - 1
			entry.Share.ReceivedAtMS = 1001
			entry.Share.AcceptedAtMS = job.ExpiresAtMS + 1
			entry.Share.Acceptance = "canonical-late-winner"
		}
		entries = append(entries, entry)
	}
	return entries
}
func liveRoundProjection(r Round) modeledRound {
	return modeledRound{r.Sequence, r.BlockHash, r.FirstAccepted, r.LastAccepted, r.ShareKeys, r.RewardAtoms, r.Allocations}
}

// These are independently calculated arithmetic and touched-record decoder
// controls, not authenticated work. The private modeled directory cannot pass
// BeginLive, source validation or any financial API. Concrete authority and
// full Sync/Refresh/Close behavior are tested separately in live_test.go.
func TestRoundLiveIndependentModeledPages(t *testing.T) {
	corpus := readLiveReference(t)
	cfg := poolshare.Config{PoolTarget: strings.Repeat("f", 64), PayoutAddress: "modeled-fixed-payout"}
	entries := liveModeledEntries(cfg, corpus.Extended.Events)
	var rounds []Round
	for _, tc := range corpus.Extended.Cases {
		got, err := derive(cfg, entries[:tc.Through])
		if err != nil || len(got) != len(tc.Rounds) {
			t.Fatal(tc.Through, err)
		}
		for i, r := range got {
			if !reflect.DeepEqual(liveRoundProjection(r), tc.Rounds[i]) {
				t.Fatal("independent allocation mismatch", tc.Through, i)
			}
		}
		if tc.Through-got[len(got)-1].LastAccepted != tc.Pending {
			t.Fatal("open tail changed")
		}
		for i, old := range rounds {
			if !reflect.DeepEqual(old, got[i]) {
				t.Fatal("later acceptance rewrote a closed round")
			}
		}
		rounds = got
	}
	db, err := leveldb.OpenFile(filepath.Join(t.TempDir(), "conditional-decoder"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	s := &Store{db: db, meta: metadata{Rounds: uint64(len(rounds))}}
	s.live = &liveDirectory{bySequence: make(map[uint64]string), byHash: make(map[string]liveRecord)}
	batch := new(leveldb.Batch)
	for _, r := range rounds {
		raw, err := recordBytes(Record{Round: r})
		if err != nil {
			t.Fatal(err)
		}
		batch.Put(indexKey(r.Sequence), []byte(r.BlockHash))
		batch.Put(roundKey(r.BlockHash), raw)
		s.live.bySequence[r.Sequence] = r.BlockHash
		s.live.byHash[r.BlockHash] = liveRecord{sequence: r.Sequence, digest: sha256.Sum256(raw)}
	}
	batch.Put([]byte("outside-selected-page"), []byte("not a full-audit authority"))
	if err := db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	pages := 0
	for _, step := range corpus.Trace {
		if step.Op != "live_list" {
			continue
		}
		rows, err := s.listLiveRecordsLocked(context.Background(), step.Cursor)
		if err != nil || len(rows) != len(step.Records) || len(rows) > MaxPage {
			t.Fatal("bounded page", step.Cursor, err)
		}
		for i, r := range rows {
			if !reflect.DeepEqual(liveRoundProjection(r.Round), step.Records[i]) {
				t.Fatal("page differs from independent reference", step.Cursor, i)
			}
		}
		pages++
	}
	if pages != 3 {
		t.Fatal("reference pages missing")
	}
	// A corrupt ninth record is outside the first eight touched pairs. It must
	// fail when selected, with no partial disclosure and immediate revocation.
	if err := db.Delete(roundKey(rounds[8].BlockHash), &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.listLiveRecordsLocked(context.Background(), 0); err != nil || len(rows) != 8 {
		t.Fatal("first page scanned ninth record", err)
	}
	if rows, err := s.listLiveRecordsLocked(context.Background(), 8); !errors.Is(err, ErrCorrupt) || rows != nil || s.live != nil {
		t.Fatal("missing selected record ignored", err)
	}
}
