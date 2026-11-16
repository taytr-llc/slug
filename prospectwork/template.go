// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectwork

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighter"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/wire/v2"
)

// A sane noncoinbase transaction needs at least one input and output (60
// stripped bytes). This bound precedes per-transaction decoding/allocation.
const maxTransactions = blockchain.MaxBlockBaseSize / 60
const maxMoney = int64(21000000 * 100000000)

type parsedTemplate struct {
	transactions []*wire.MsgTx
	flags        []byte
	witness      []byte
	sigOps       int64
}

func productionOnly() error {
	if wire.TestPoW {
		return errors.New("payable candidates require production PoW; slugtestpow is unsupported")
	}
	return nil
}

func hash(value string, nonzero bool) (chainhash.Hash, error) {
	if len(value) != 64 {
		return chainhash.Hash{}, errors.New("invalid hash length")
	}
	decoded, err := chainhash.NewHashFromStr(value)
	if err != nil || (nonzero && *decoded == (chainhash.Hash{})) {
		return chainhash.Hash{}, errors.New("invalid hash")
	}
	return *decoded, nil
}

func validateContext(context prospect.Context) error {
	if context.Version != prospect.Version1 || context.Network != uint32(chaincfg.MainNetParams.Net) ||
		!strings.EqualFold(context.Genesis, chaincfg.MainNetParams.GenesisHash.String()) || context.DNAVersion != fighter.Version1 ||
		context.ParentHeight > math.MaxInt32-1 || context.MinTimestamp == 0 || context.MaxTimestamp < context.MinTimestamp {
		return errors.New("invalid production SLUG context")
	}
	if _, err := hash(context.ParentHash, true); err != nil {
		return err
	}
	if _, err := hash(context.ProspectTarget, true); err != nil {
		return err
	}
	target := blockchain.CompactToBig(context.ExpectedBits)
	if target.Sign() <= 0 || target.Cmp(chaincfg.MainNetParams.PowLimit) > 0 || blockchain.BigToCompact(target) != context.ExpectedBits {
		return errors.New("invalid canonical block target")
	}
	return nil
}

// ValidateTemplate checks all locally verifiable template assertions. Fee and
// contextual sigop costs still rely on the trusted node's UTXO view; proposal
// validation is needed before treating a constructed candidate as connectable.
func ValidateTemplate(template Template) error {
	_, err := validateTemplate(template)
	return err
}

func validateTemplate(template Template) (parsedTemplate, error) {
	var result parsedTemplate
	if err := productionOnly(); err != nil {
		return result, err
	}
	if err := validateContext(template.Context); err != nil {
		return result, err
	}
	context, block := template.Context, template.Block
	if block.Version < 2 || block.Height != int64(context.ParentHeight)+1 ||
		!strings.EqualFold(block.PreviousHash, context.ParentHash) || block.MinTime != int64(context.MinTimestamp) ||
		block.MaxTime != int64(context.MaxTimestamp) || block.CurTime < block.MinTime || block.CurTime > block.MaxTime ||
		block.NonceRange != "00000000ffffffff" {
		return result, errors.New("template header does not match trusted context")
	}
	if len(block.Bits) == 0 || len(block.Bits) > 8 {
		return result, errors.New("invalid template bits")
	}
	for _, character := range block.Bits {
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return result, errors.New("invalid hexadecimal template bits")
		}
	}
	bits, err := strconv.ParseUint(block.Bits, 16, 32)
	if err != nil || uint32(bits) != context.ExpectedBits {
		return result, errors.New("template bits do not match context")
	}
	target, err := hash(block.Target, true)
	if err != nil || blockchain.HashToBig(&target).Cmp(blockchain.CompactToBig(context.ExpectedBits)) != 0 {
		return result, errors.New("template target does not match bits")
	}
	if block.WeightLimit <= 0 || block.WeightLimit > blockchain.MaxBlockWeight ||
		block.SizeLimit <= 0 || block.SizeLimit > wire.MaxBlockPayload || block.SigOpLimit <= 0 || block.SigOpLimit > blockchain.MaxBlockSigOpsCost {
		return result, errors.New("invalid template resource limits")
	}
	if block.CoinbaseValue == nil || *block.CoinbaseValue < 0 || *block.CoinbaseValue > maxMoney || block.CoinbaseTxn != nil || block.CoinbaseAux == nil ||
		len(block.CoinbaseAux.Flags) > 200 || len(block.CoinbaseAux.Flags)%2 != 0 || len(block.Transactions) > maxTransactions {
		return result, errors.New("invalid coinbasevalue template")
	}
	result.flags, err = hex.DecodeString(block.CoinbaseAux.Flags)
	if err != nil {
		return result, errors.New("invalid coinbase auxiliary data")
	}
	result.transactions = make([]*wire.MsgTx, 0, len(block.Transactions))
	indices := make(map[chainhash.Hash]int, len(block.Transactions))
	var totalBytes, totalBase, totalWeight, fees int64
	var witnessIncluded bool
	for i, entry := range block.Transactions {
		if len(entry.Data)%2 != 0 || len(entry.Data) < 120 || len(entry.Data) > 2*wire.MaxBlockPayload || len(entry.Depends) > len(block.Transactions) {
			return parsedTemplate{}, errors.New("invalid template transaction size")
		}
		totalBytes += int64(len(entry.Data) / 2)
		if totalBytes > block.SizeLimit || totalBytes > wire.MaxBlockPayload {
			return parsedTemplate{}, errors.New("template transactions exceed byte limit")
		}
		raw, err := hex.DecodeString(entry.Data)
		if err != nil {
			return parsedTemplate{}, errors.New("invalid template transaction hexadecimal encoding")
		}
		reader := bytes.NewReader(raw)
		transaction := new(wire.MsgTx)
		if transaction.Deserialize(reader) != nil || reader.Len() != 0 {
			return parsedTemplate{}, errors.New("invalid template transaction encoding")
		}
		var canonical bytes.Buffer
		if transaction.Serialize(&canonical) != nil || !bytes.Equal(canonical.Bytes(), raw) {
			return parsedTemplate{}, errors.New("noncanonical template transaction encoding")
		}
		wrapped := btcutil.NewTx(transaction)
		if blockchain.IsCoinBaseTx(transaction) || blockchain.CheckTransactionSanity(wrapped) != nil {
			return parsedTemplate{}, errors.New("invalid noncoinbase template transaction")
		}
		txid, err := hash(entry.TxID, false)
		if err != nil || txid != transaction.TxHash() {
			return parsedTemplate{}, errors.New("template transaction ID mismatch")
		}
		wtxid, err := hash(entry.Hash, false)
		if err != nil || wtxid != transaction.WitnessHash() {
			return parsedTemplate{}, errors.New("template witness transaction ID mismatch")
		}
		if _, duplicate := indices[txid]; duplicate {
			return parsedTemplate{}, errors.New("duplicate template transaction")
		}
		indices[txid] = i + 1
		weight := blockchain.GetTransactionWeight(wrapped)
		if entry.Weight != weight || entry.SigOps < int64(blockchain.CountSigOps(wrapped))*blockchain.WitnessScaleFactor ||
			entry.SigOps > block.SigOpLimit || entry.Fee < 0 || entry.Fee > maxMoney-fees {
			return parsedTemplate{}, errors.New("invalid transaction weight, sigops or fee")
		}
		totalBase += int64(transaction.SerializeSizeStripped())
		totalWeight += weight
		result.sigOps += entry.SigOps
		fees += entry.Fee
		if totalBase > blockchain.MaxBlockBaseSize || totalWeight > block.WeightLimit || result.sigOps > block.SigOpLimit {
			return parsedTemplate{}, errors.New("template transaction resource limit exceeded")
		}
		witnessIncluded = witnessIncluded || transaction.HasWitness()
		result.transactions = append(result.transactions, transaction)
	}
	// Dependencies are derived from all inputs, not trusted as instructions to
	// reorder transactions. Reject forward/self references and duplicate spends.
	spent := make(map[wire.OutPoint]bool)
	for i, transaction := range result.transactions {
		expected := make(map[int64]bool)
		for _, input := range transaction.TxIn {
			if spent[input.PreviousOutPoint] {
				return parsedTemplate{}, errors.New("template repeats a spent outpoint")
			}
			spent[input.PreviousOutPoint] = true
			if index, found := indices[input.PreviousOutPoint.Hash]; found {
				if index >= i+1 || uint64(input.PreviousOutPoint.Index) >= uint64(len(result.transactions[index-1].TxOut)) {
					return parsedTemplate{}, errors.New("invalid template transaction dependency")
				}
				expected[int64(index)] = true
			}
		}
		seen := make(map[int64]bool)
		for _, dependency := range block.Transactions[i].Depends {
			if !expected[dependency] || seen[dependency] {
				return parsedTemplate{}, errors.New("incorrect template dependency metadata")
			}
			seen[dependency] = true
		}
		if len(seen) != len(expected) {
			return parsedTemplate{}, errors.New("missing template dependency metadata")
		}
	}
	subsidy := blockchain.CalcBlockSubsidy(int32(block.Height), &chaincfg.MainNetParams)
	if fees > maxMoney-subsidy || *block.CoinbaseValue != subsidy+fees {
		return parsedTemplate{}, errors.New("coinbase value does not equal subsidy and template fees")
	}
	if witnessIncluded {
		if len(block.DefaultWitnessCommitment) != 64 {
			return parsedTemplate{}, errors.New("missing raw SLUG witness commitment")
		}
		result.witness, err = hex.DecodeString(block.DefaultWitnessCommitment)
		if err != nil {
			return parsedTemplate{}, errors.New("invalid raw witness commitment")
		}
		leaves := make([]chainhash.Hash, len(result.transactions)+1)
		for i, transaction := range result.transactions {
			leaves[i+1] = transaction.WitnessHash()
		}
		root, _ := merkle(leaves)
		var preimage [64]byte // Coinbase witness reserved value is zero32.
		copy(preimage[:32], root[:])
		if !bytes.Equal(result.witness, chainhash.DoubleHashB(preimage[:])) {
			return parsedTemplate{}, errors.New("template witness commitment mismatch")
		}
	} else if block.DefaultWitnessCommitment != "" {
		return parsedTemplate{}, errors.New("unexpected witness commitment without witness transactions")
	}
	return result, nil
}

// merkle returns a SHA256d transaction root and the index-zero right sibling
// path. Inputs and outputs are hash values; no caller-owned slice is changed.
func merkle(leaves []chainhash.Hash) (chainhash.Hash, []string) {
	level := append([]chainhash.Hash(nil), leaves...)
	branch := make([]string, 0)
	for len(level) > 1 {
		branch = append(branch, level[1].String())
		next := make([]chainhash.Hash, (len(level)+1)/2)
		for i := range next {
			left, right := level[2*i], level[2*i]
			if 2*i+1 < len(level) {
				right = level[2*i+1]
			}
			var preimage [64]byte
			copy(preimage[:32], left[:])
			copy(preimage[32:], right[:])
			next[i] = chainhash.DoubleHashH(preimage[:])
		}
		level = next
	}
	return level[0], branch
}
