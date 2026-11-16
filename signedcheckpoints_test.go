package main

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/database"
	_ "github.com/taytr-llc/slug/database/ffldb"
)

// This deterministic key is a test fixture, never an operator authority.
func checkpointFixture(t *testing.T, height int32) (chaincfg.Params, chaincfg.SignedCheckpoint) {
	t.Helper()
	private := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	params := chaincfg.MainNetParams
	params.CheckpointPublicKey = private.Public().(ed25519.PublicKey)
	checkpoint := chaincfg.SignedCheckpoint{
		Version: chaincfg.SignedCheckpointVersion, Network: params.Net,
		GenesisHash: *params.GenesisHash, Height: height,
		Hash: chainhash.Hash{byte(height)},
	}
	message, err := checkpoint.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	checkpoint.Signature = ed25519.Sign(private, message)
	return params, checkpoint
}

func writeCheckpointFixture(t *testing.T, checkpoint chaincfg.SignedCheckpoint) string {
	t.Helper()
	data, err := json.Marshal(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "checkpoint.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSignedCheckpoints(t *testing.T) {
	params, first := checkpointFixture(t, 1)
	_, second := checkpointFixture(t, 2)
	firstPath, secondPath := writeCheckpointFixture(t, first), writeCheckpointFixture(t, second)
	got, err := loadSignedCheckpoints([]string{secondPath, firstPath}, &params)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Height != 1 || got[1].Height != 2 {
		t.Fatalf("unexpected ordering: %+v", got)
	}
	if _, err := loadSignedCheckpoints([]string{firstPath, firstPath}, &params); err == nil {
		t.Fatal("accepted duplicate checkpoint heights")
	}
	// An unconfigured authority cannot be supplied by an untrusted document.
	if _, err := loadSignedCheckpoints([]string{firstPath}, &chaincfg.MainNetParams); err == nil {
		t.Fatal("accepted signed file without compiled authority")
	}
	if records, err := loadSignedCheckpoints(nil, &chaincfg.MainNetParams); err != nil || len(records) != 0 {
		t.Fatalf("normal startup requires no authority: %v", err)
	}
	other := params
	other.Net = chaincfg.RegressionNetParams.Net
	if _, err := loadSignedCheckpoints([]string{firstPath}, &other); err == nil {
		t.Fatal("accepted cross-network replay")
	}
	first.Signature[0] ^= 1
	if _, err := loadSignedCheckpoints([]string{writeCheckpointFixture(t, first)}, &params); err == nil {
		t.Fatal("accepted bad signature")
	}
	oversized := filepath.Join(t.TempDir(), "large.json")
	if err := os.WriteFile(oversized, []byte(strings.Repeat(" ", chaincfg.MaxSignedCheckpointSize+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSignedCheckpoints([]string{oversized}, &params); err == nil {
		t.Fatal("accepted oversized file")
	}
	// The JSON package strips whitespace before calling UnmarshalJSON. A
	// valid prefix must not hide oversized padding or trailing content past
	// the bounded reader's cutoff.
	validJSON, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "{}"} {
		padded := append(append([]byte(nil), validJSON...), []byte(strings.Repeat(" ", chaincfg.MaxSignedCheckpointSize)+suffix)...)
		if err := os.WriteFile(oversized, padded, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadSignedCheckpoints([]string{oversized}, &params); err == nil {
			t.Fatal("accepted a valid JSON prefix hiding oversized/trailing content")
		}
	}
	if _, err := loadSignedCheckpoints([]string{filepath.Join(t.TempDir(), "missing")}, &params); err == nil {
		t.Fatal("ignored missing file")
	}
}

func TestSignedCheckpointRPCIntegration(t *testing.T) {
	params, checkpoint := checkpointFixture(t, 1)
	records, err := loadSignedCheckpoints([]string{writeCheckpointFixture(t, checkpoint)}, &params)
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Create("ffldb", filepath.Join(t.TempDir(), "db"), params.Net)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	chain, err := blockchain.New(&blockchain.Config{
		DB: db, ChainParams: &params, TimeSource: blockchain.NewMedianTime(),
		SignedCheckpoints: records,
	})
	if err != nil {
		t.Fatal(err)
	}
	if chain.HasCheckpoints() || chain.LatestCheckpoint() != nil {
		t.Fatal("signed record entered consensus checkpoint machinery")
	}
	s := &rpcServer{cfg: rpcserverConfig{Chain: chain}}
	response, err := handleGetCheckpointInfo(s, btcjson.NewGetCheckpointInfoCmd(), nil)
	if err != nil {
		t.Fatal(err)
	}
	result := response.(*btcjson.GetCheckpointInfoResult)
	if result.Mode != "report-only" || !result.AuthorityConfigured || result.BestHeight != 0 || result.BestHash != params.GenesisHash.String() || result.Conflict {
		t.Fatalf("bad chain report: %+v", result)
	}
	if len(result.Checkpoints) != 1 || result.Checkpoints[0].Status != "pending" || result.Checkpoints[0].Hash != checkpoint.Hash.String() {
		t.Fatalf("lost pending signed declaration: %+v", result.Checkpoints)
	}
	if _, ok := rpcLimited["getcheckpointinfo"]; !ok {
		t.Fatal("read-only report unavailable to limited RPC users")
	}
}

func TestCheckpointRPCConflict(t *testing.T) {
	params, checkpoint := checkpointFixture(t, 1)
	local := chainhash.Hash{2}
	result := checkpointInfoResult(blockchain.SignedCheckpointSnapshot{
		BestHeight: 100, BestHash: chainhash.Hash{100}, PublicKey: params.CheckpointPublicKey,
		Checkpoints: []blockchain.SignedCheckpointStatus{{SignedCheckpoint: checkpoint, Status: "conflict", LocalHash: &local}},
	})
	if !result.Conflict || len(result.Checkpoints) != 1 || result.Checkpoints[0].Hash != checkpoint.Hash.String() || result.Checkpoints[0].LocalHash != local.String() || result.Checkpoints[0].Signature == "" {
		t.Fatalf("conflict obscures the official signed record: %+v", result)
	}
}
