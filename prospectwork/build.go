// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectwork

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func payout(value string) (address.Address, []byte, error) {
	if len(value) == 0 || len(value) > 128 {
		return nil, nil, errors.New("a mainnet payout address is required")
	}
	destination, err := address.DecodeAddress(value, &chaincfg.MainNetParams)
	if err != nil || !destination.IsForNet(&chaincfg.MainNetParams) {
		return nil, nil, errors.New("invalid mainnet payout address")
	}
	// Accept network-encoded spend destinations only. In particular, a raw
	// pubkey or anyone-can-spend anchor is never a default payout destination.
	switch destination.(type) {
	case *address.AddressPubKeyHash, *address.AddressScriptHash, *address.AddressWitnessPubKeyHash,
		*address.AddressWitnessScriptHash, *address.AddressTaproot:
	default:
		return nil, nil, errors.New("unsupported payout address type")
	}
	script, err := txscript.PayToAddrScript(destination)
	if err != nil {
		return nil, nil, errors.New("invalid payout script")
	}
	return destination, script, nil
}

// Build constructs a nonce-zero candidate without hashing, signing, mining,
// broadcasting, or reading an authority clock. Job validation establishes its
// byte binding, not issuance or remaining lifetime. The payout address comes
// explicitly from the caller's local wallet/operator configuration.
func Build(job prospectjob.Job, template Template, payoutAddress string, extraNonce uint64) (Candidate, error) {
	var candidate Candidate
	if err := productionOnly(); err != nil {
		return candidate, err
	}
	if err := prospectjob.Validate(job); err != nil {
		return candidate, err
	}
	commitment, err := prospectjob.CommitmentScript(job)
	if err != nil {
		return candidate, err
	}
	return buildValidated(job.Context, job.GymID, job.ID, commitment, template, payoutAddress, extraNonce)
}

// BuildManifest constructs a candidate-only manifest-bound job without
// changing the candidate/proof formats or authenticating issuer/birth facts.
// Legacy Build continues to require its distinct version-1 Job type.
func BuildManifest(job prospectjob.ManifestJob, template Template, payoutAddress string, extraNonce uint64) (Candidate, error) {
	if err := productionOnly(); err != nil {
		return Candidate{}, err
	}
	if err := prospectjob.ValidateManifest(job); err != nil {
		return Candidate{}, err
	}
	commitment, err := prospectjob.ManifestCommitmentScript(job)
	if err != nil {
		return Candidate{}, err
	}
	return buildValidated(job.Context, job.GymID, job.ID, commitment, template, payoutAddress, extraNonce)
}

// Both public entries validate their distinct job type and produce the script
// internally. This private assembler is not an arbitrary public JobID bypass.
func buildValidated(initial prospect.Context, gymID, jobID string, commitment []byte, template Template, payoutAddress string, extraNonce uint64) (Candidate, error) {
	var candidate Candidate
	parsed, err := validateTemplate(template)
	if err != nil {
		return candidate, err
	}
	current := template.Context
	if initial.Version != current.Version || initial.Network != current.Network ||
		!strings.EqualFold(initial.Genesis, current.Genesis) || initial.DNAVersion != current.DNAVersion ||
		!strings.EqualFold(initial.ProspectTarget, current.ProspectTarget) || !strings.EqualFold(initial.ParentHash, current.ParentHash) ||
		initial.ParentHeight != current.ParentHeight || initial.ExpectedBits != current.ExpectedBits || initial.MinTimestamp != current.MinTimestamp ||
		template.Block.CurTime < int64(initial.MinTimestamp) || template.Block.CurTime > int64(initial.MaxTimestamp) {
		return candidate, errors.New("job and template contexts do not match")
	}
	destination, payoutScript, err := payout(payoutAddress)
	if err != nil {
		return candidate, err
	}
	var nonce [8]byte
	binary.LittleEndian.PutUint64(nonce[:], extraNonce)
	script, err := txscript.NewScriptBuilder().AddInt64(int64(initial.ParentHeight) + 1).AddData(nonce[:]).Script()
	if err != nil {
		return candidate, errors.New("could not construct coinbase height and extra nonce")
	}
	script = append(script, parsed.flags...)
	if len(script) < blockchain.MinCoinbaseScriptLen || len(script) > blockchain.MaxCoinbaseScriptLen {
		return candidate, errors.New("coinbase signature script exceeds consensus bounds")
	}
	coinbase := wire.NewMsgTx(2)
	coinbase.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: wire.MaxPrevOutIndex}, script, nil))
	coinbase.AddTxOut(wire.NewTxOut(*template.Block.CoinbaseValue, payoutScript))
	coinbase.AddTxOut(wire.NewTxOut(0, commitment))
	if len(parsed.witness) != 0 {
		coinbase.TxIn[0].Witness = wire.TxWitness{make([]byte, 32)}
		witnessScript := append([]byte{0x6a, 0x24, 0xaa, 0x21, 0xa9, 0xed}, parsed.witness...)
		coinbase.AddTxOut(wire.NewTxOut(0, witnessScript))
	}
	if err := blockchain.CheckTransactionSanity(btcutil.NewTx(coinbase)); err != nil {
		return candidate, errors.New("constructed coinbase failed sanity checks")
	}
	transactions := make([]*wire.MsgTx, 1, len(parsed.transactions)+1)
	transactions[0] = coinbase
	transactions = append(transactions, parsed.transactions...)
	leaves := make([]chainhash.Hash, len(transactions))
	for i, transaction := range transactions {
		leaves[i] = transaction.TxHash()
	}
	root, branch := merkle(leaves)
	parent, _ := hash(initial.ParentHash, true)
	block := &wire.MsgBlock{Header: wire.BlockHeader{Version: template.Block.Version, PrevBlock: parent, MerkleRoot: root,
		Timestamp: time.Unix(template.Block.CurTime, 0), Bits: initial.ExpectedBits, Nonce: 0}, Transactions: transactions}
	wrapped := btcutil.NewBlock(block)
	weight := blockchain.GetBlockWeight(wrapped)
	sigOps := parsed.sigOps + int64(blockchain.CountSigOps(btcutil.NewTx(coinbase)))*blockchain.WitnessScaleFactor
	if block.SerializeSizeStripped() > blockchain.MaxBlockBaseSize || int64(block.SerializeSize()) > template.Block.SizeLimit ||
		block.SerializeSize() > wire.MaxBlockPayload || weight > template.Block.WeightLimit || weight > blockchain.MaxBlockWeight ||
		sigOps > template.Block.SigOpLimit || sigOps > blockchain.MaxBlockSigOpsCost {
		return candidate, errors.New("constructed block exceeds template or consensus limits")
	}
	if err := blockchain.ValidateWitnessCommitment(wrapped); err != nil {
		return candidate, errors.New("constructed block witness commitment is invalid")
	}
	var fullBlock, header, stripped bytes.Buffer
	if block.Serialize(&fullBlock) != nil || block.Header.Serialize(&header) != nil || coinbase.SerializeNoWitness(&stripped) != nil {
		return candidate, errors.New("could not serialize candidate")
	}
	return Candidate{
		Version: Version1, JobID: jobID, BlockHex: hex.EncodeToString(fullBlock.Bytes()),
		Proof: prospect.Proof{Version: prospect.Version2, GymID: gymID, JobID: jobID, Header: hex.EncodeToString(header.Bytes()),
			Coinbase: hex.EncodeToString(stripped.Bytes()), MerkleBranch: branch},
		PayoutAddress: destination.EncodeAddress(), CoinbaseValue: *template.Block.CoinbaseValue,
		TransactionCount: uint32(len(transactions)), Weight: weight, SigOpCost: sigOps,
	}, nil
}
