// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectworker

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighter"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func matchesPolicy(job prospectjob.Job, config Config) bool {
	c, p := job.Context, config.Policy
	return c.Version == p.Version && c.Network == p.Network && c.Genesis == p.Genesis && c.DNAVersion == p.DNAVersion && c.ProspectTarget == p.ProspectTarget
}

func validateScan(scan Scan, config Config) error {
	if scan.Version != Version1 || scan.NextNonce > 1<<32 || scan.Job.PublicKey != config.PublicKey || !matchesPolicy(scan.Job, config) {
		return ErrCorrupt
	}
	candidate, err := prospectwork.Build(scan.Job, scan.Template, config.PayoutAddress, scan.ExtraNonce)
	if err != nil || !reflect.DeepEqual(candidate, scan.Candidate) {
		return ErrCorrupt
	}
	return nil
}
func workBlock(job prospectjob.Job, candidate prospectwork.Candidate, config Config) (*wire.MsgBlock, error) {
	if prospectjob.Validate(job) != nil || job.PublicKey != config.PublicKey || !matchesPolicy(job, config) || candidate.Version != 1 || candidate.JobID != job.ID || candidate.PayoutAddress != config.PayoutAddress || candidate.Proof.JobID != job.ID || candidate.Proof.GymID != job.GymID || candidate.Proof.Version != 2 || len(candidate.BlockHex) > 2*wire.MaxBlockPayload || len(candidate.BlockHex) < 162 {
		return nil, ErrCorrupt
	}
	if _, err := gymauth.AdmissionDigest(job.Context, candidate.Proof, job.PublicKey); err != nil {
		return nil, ErrCorrupt
	}
	raw, err := hex.DecodeString(candidate.BlockHex)
	if err != nil {
		return nil, ErrCorrupt
	}
	reader := bytes.NewReader(raw)
	block := new(wire.MsgBlock)
	if block.Deserialize(reader) != nil || reader.Len() != 0 || len(block.Transactions) == 0 || len(block.Transactions) != int(candidate.TransactionCount) {
		return nil, ErrCorrupt
	}
	var encoded, header, stripped bytes.Buffer
	if block.Serialize(&encoded) != nil || !bytes.Equal(raw, encoded.Bytes()) || block.Header.Serialize(&header) != nil || hex.EncodeToString(header.Bytes()) != candidate.Proof.Header {
		return nil, ErrCorrupt
	}
	coinbase := block.Transactions[0]
	if !blockchain.IsCoinBaseTx(coinbase) || blockchain.CheckTransactionSanity(btcutil.NewTx(coinbase)) != nil || blockchain.CheckSerializedHeight(btcutil.NewTx(coinbase), int32(job.Context.ParentHeight+1)) != nil || coinbase.SerializeNoWitness(&stripped) != nil || hex.EncodeToString(stripped.Bytes()) != candidate.Proof.Coinbase || len(coinbase.TxOut) < 2 || len(coinbase.TxOut) > 3 {
		return nil, ErrCorrupt
	}
	destination, err := address.DecodeAddress(config.PayoutAddress, &chaincfg.MainNetParams)
	if err != nil {
		return nil, ErrCorrupt
	}
	script, err := txscript.PayToAddrScript(destination)
	if err != nil {
		return nil, ErrCorrupt
	}
	commitment, err := prospectjob.CommitmentScript(job)
	if err != nil || coinbase.TxOut[0].Value != candidate.CoinbaseValue || !bytes.Equal(coinbase.TxOut[0].PkScript, script) || coinbase.TxOut[1].Value != 0 || !bytes.Equal(coinbase.TxOut[1].PkScript, commitment) || (len(coinbase.TxOut) == 3 && coinbase.TxOut[2].Value != 0) {
		return nil, ErrCorrupt
	}
	if len(coinbase.TxOut) == 3 && (len(coinbase.TxOut[2].PkScript) != blockchain.CoinbaseWitnessPkScriptLength || !bytes.HasPrefix(coinbase.TxOut[2].PkScript, blockchain.WitnessMagicBytes)) {
		return nil, ErrCorrupt
	}
	wrapped := btcutil.NewBlock(block)
	if blockchain.CalcMerkleRoot(wrapped.Transactions(), false) != block.Header.MerkleRoot || blockchain.ValidateWitnessCommitment(wrapped) != nil || blockchain.GetBlockWeight(wrapped) != candidate.Weight || candidate.Weight > blockchain.MaxBlockWeight || block.SerializeSizeStripped() > blockchain.MaxBlockBaseSize || candidate.SigOpCost < 0 || candidate.SigOpCost > blockchain.MaxBlockSigOpsCost {
		return nil, ErrCorrupt
	}
	current := coinbase.TxHash()
	for _, sibling := range candidate.Proof.MerkleBranch {
		if len(sibling) != 64 {
			return nil, ErrCorrupt
		}
		right, err := chainhash.NewHashFromStr(sibling)
		if err != nil {
			return nil, ErrCorrupt
		}
		var pair [64]byte
		copy(pair[:32], current[:])
		copy(pair[32:], right[:])
		current = chainhash.DoubleHashH(pair[:])
	}
	if current != block.Header.MerkleRoot {
		return nil, ErrCorrupt
	}
	return block, nil
}
func classify(hash chainhash.Hash, job prospectjob.Job) (bool, bool) {
	number := blockchain.HashToBig(&hash)
	target, _ := new(big.Int).SetString(job.Context.ProspectTarget, 16)
	return number.Cmp(blockchain.CompactToBig(job.Context.ExpectedBits)) <= 0, number.Cmp(target) <= 0
}
func validateEntry(entry Entry, config Config) error {
	if entry.Version != Version1 || entry.Sequence == 0 || entry.NextAttemptMS < 0 || len(entry.WorkHash) != 64 || (!entry.BlockWin && !entry.ProspectWin) {
		return ErrCorrupt
	}
	digest, err := chainhash.NewHashFromStr(entry.WorkHash)
	if err != nil || digest.String() != entry.WorkHash {
		return ErrCorrupt
	}
	if _, err := workBlock(entry.Job, entry.Candidate, config); err != nil {
		return ErrCorrupt
	}
	block, prospect := classify(*digest, entry.Job)
	if block != entry.BlockWin || prospect != entry.ProspectWin {
		return ErrCorrupt
	}
	switch entry.BlockState {
	case "not-applicable":
		if entry.BlockWin {
			return ErrCorrupt
		}
	case "pending", "submitted", "canonical", "rejected":
		if !entry.BlockWin {
			return ErrCorrupt
		}
	default:
		return ErrCorrupt
	}
	switch entry.AdmissionState {
	case "not-applicable":
		if entry.ProspectWin {
			return ErrCorrupt
		}
	case "needs-signature", "pending", "accepted", "expired", "rejected":
		if !entry.ProspectWin {
			return ErrCorrupt
		}
	default:
		return ErrCorrupt
	}
	if entry.Authorization != nil {
		encoded, _ := json.Marshal(entry.Authorization)
		if len(encoded) > gymauth.MaxAuthorizationSize || !entry.ProspectWin || gymauth.VerifyAuthorization(entry.Job.Context, entry.Candidate.Proof, *entry.Authorization) != nil {
			return ErrCorrupt
		}
	} else if entry.AdmissionState == "pending" || entry.AdmissionState == "accepted" {
		return ErrCorrupt
	}
	if entry.Receipt != nil {
		result := entry.Receipt
		record := result.Record
		encoded, _ := json.Marshal(result)
		if len(encoded) > prospectservice.MaxResponseSize || entry.AdmissionState != "accepted" || result.Version != 2 || result.Authentication != "gym-key-control" || result.BirthRule != "job-issuance" || result.Inserted != result.CurrentNodeChecked || (result.Inserted && result.ContextSource != "trusted-node-snapshot") || (!result.Inserted && result.ContextSource != "historical-receipt") || record.Version != registry.Version4 || record.Sequence == 0 || record.Job == nil || *record.Job != entry.Job || record.Context != entry.Job.Context || record.Verified.WorkHash != entry.WorkHash || record.Birth == nil || record.Birth.RuleVersion != entry.Job.RuleVersion || record.Birth.BornAtMS != entry.Job.IssuedAtMS || record.Birth.AcceptedAtMS < entry.Job.IssuedAtMS || record.Birth.AcceptedAtMS >= entry.Job.ExpiresAtMS || record.Authorization == nil || gymauth.VerifyAuthorization(entry.Job.Context, record.Proof, *record.Authorization) != nil || !equalProof(record.Proof, entry.Candidate.Proof) || record.Eligibility == nil || registry.ValidateEligibility(entry.Job.Context, record.Verified, *record.Eligibility) != nil {
			return ErrCorrupt
		}
		expected, err := expectedVerified(entry)
		if err != nil || record.Verified != expected {
			return ErrCorrupt
		}
		eligibility := record.Eligibility
		switch eligibility.Source {
		case "current-parent":
			if eligibility.Hash != entry.Job.Context.ParentHash || eligibility.Height != entry.Job.Context.ParentHeight {
				return ErrCorrupt
			}
		case "canonical-winning-block":
			if !entry.BlockWin || eligibility.Hash != entry.WorkHash || eligibility.Height != entry.Job.Context.ParentHeight+1 {
				return ErrCorrupt
			}
		default:
			return ErrCorrupt
		}
	} else if entry.AdmissionState == "accepted" {
		return ErrCorrupt
	}
	switch entry.LastError {
	case "", "unavailable", "outcome_unknown", "busy", "capacity", "rate_limited", "expired", "rejected", "unknown_job", "unauthorized", "forbidden", "invalid_request", "too_large", "signer_unavailable", "clock", "stale":
	default:
		return ErrCorrupt
	}
	return nil
}

func equalProof(a, b prospect.Proof) bool {
	if a.Version != b.Version || !strings.EqualFold(a.GymID, b.GymID) || !strings.EqualFold(a.JobID, b.JobID) || !strings.EqualFold(a.Header, b.Header) || !strings.EqualFold(a.Coinbase, b.Coinbase) || len(a.MerkleBranch) != len(b.MerkleBranch) {
		return false
	}
	for i := range a.MerkleBranch {
		if !strings.EqualFold(a.MerkleBranch[i], b.MerkleBranch[i]) {
			return false
		}
	}
	return true
}

// Recompute inexpensive receipt fields from the claimed digest. Opening a
// journal does not rehash history; recovered pending work is hashed before any
// transport, and stored receipts remain conditional evidence from an authority.
func expectedVerified(entry Entry) (prospect.Verified, error) {
	context := entry.Job.Context
	seed, err := fighter.ParseSeed(entry.WorkHash)
	if err != nil {
		return prospect.Verified{}, ErrCorrupt
	}
	dna, err := fighter.Generate(context.DNAVersion, seed)
	if err != nil {
		return prospect.Verified{}, ErrCorrupt
	}
	genesis, _ := hex.DecodeString(context.Genesis)
	target, _ := hex.DecodeString(context.ProspectTarget)
	var integer [4]byte
	binary.LittleEndian.PutUint32(integer[:], context.Network)
	policy := append([]byte("SLUG prospect policy\x00v1\x00"), integer[:]...)
	policy = append(policy, genesis...)
	binary.LittleEndian.PutUint32(integer[:], context.DNAVersion)
	policy = append(policy, integer[:]...)
	policy = append(policy, target...)
	policyHash := sha256.Sum256(policy)
	binary.LittleEndian.PutUint32(integer[:], context.Network)
	identifier := append([]byte("SLUG prospect id\x00v1\x00"), integer[:]...)
	identifier = append(identifier, genesis...)
	identifier = append(identifier, seed[:]...)
	idHash := sha256.Sum256(identifier)
	return prospect.Verified{Version: 2, Verification: "supplied-context", Network: context.Network, Genesis: context.Genesis,
		ProspectID: hex.EncodeToString(idHash[:]), PolicyID: hex.EncodeToString(policyHash[:]), GymID: entry.Job.GymID,
		WorkHash: entry.WorkHash, ParentHash: context.ParentHash, ParentHeight: context.ParentHeight, DNA: dna}, nil
}
