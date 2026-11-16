// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package blockchain

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

const activationInputValue int64 = 100000

func activationSpend() *wire.MsgTx {
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: chainhash.DoubleHashH([]byte("PUBLIC synthetic activation UTXO")), Index: 0}, nil, nil))
	tx.AddTxOut(wire.NewTxOut(activationInputValue-1000, []byte{txscript.OP_TRUE}))
	return tx
}

// Exercise the same sanity/context/connection pipeline as a block proposal,
// with a fully specified synthetic UTXO snapshot instead of disk lookups.
// Only PoW is excluded here; production mining acceptance is a separate gate.
func checkActivationSpend(t *testing.T, height int32, tx *wire.MsgTx, pkScript []byte, originHeight int32) error {
	t.Helper()
	b := activationChain(height - 1)
	block := activationBlock(t, b, tx)
	if err := checkBlockSanity(block, b.chainParams.PowLimit, b.timeSource, BFNoPoWCheck); err != nil {
		return fmt.Errorf("sanity: %w", err)
	}
	if err := b.checkBlockContext(block, b.bestChain.Tip(), BFNoPoWCheck); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	view := NewUtxoViewpoint()
	view.SetBestHash(&b.bestChain.Tip().hash)
	// BIP30 output lookups are explicitly absent, not bypassed. Every input
	// is present below, so a missing fixture entry cannot silently hit disk.
	for _, transaction := range block.Transactions() {
		for index := range transaction.MsgTx().TxOut {
			view.entries[wire.OutPoint{Hash: *transaction.Hash(), Index: uint32(index)}] = nil
		}
	}
	view.entries[tx.TxIn[0].PreviousOutPoint] = NewUtxoEntry(wire.NewTxOut(activationInputValue, pkScript), originHeight, false)
	node := newBlockNode(&block.MsgBlock().Header, b.bestChain.Tip())
	if err := b.checkConnectBlock(node, block, view, nil); err != nil {
		return fmt.Errorf("connection: %w", err)
	}
	return nil
}

func assertActivationRule(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	var rule RuleError
	require.ErrorAs(t, err, &rule)
	require.Equal(t, want, rule.ErrorCode)
}

func TestPhaseADERActivation(t *testing.T) {
	// Explicit public fixture scalar1. This tests rule routing, not an
	// independently derived cryptographic golden.
	key, _ := btcec.PrivKeyFromBytes([]byte{1})
	defer key.Zero()
	pk, err := txscript.NewScriptBuilder().AddData(key.PubKey().SerializeCompressed()).AddOp(txscript.OP_CHECKSIG).Script()
	require.NoError(t, err)
	for _, strict := range []bool{true, false} {
		tx := activationSpend()
		sig, err := txscript.RawTxInSignature(tx, 0, pk, txscript.SigHashAll, key)
		require.NoError(t, err)
		if !strict {
			// Same ECDSA integers, redundant R padding: historical BER accepts,
			// BIP66's strict DER rejects. Keep the sighash suffix unchanged.
			sig = append([]byte{0x30, sig[1] + 1, 0x02, sig[3] + 1, 0}, sig[4:]...)
		}
		tx.TxIn[0].SignatureScript, err = txscript.NewScriptBuilder().AddData(sig).Script()
		require.NoError(t, err)
		require.NoError(t, checkActivationSpend(t, 61, tx, pk, 1))
		err = checkActivationSpend(t, 62, tx, pk, 1)
		if strict {
			require.NoError(t, err)
		} else {
			assertActivationRule(t, err, ErrScriptValidation)
		}
	}
}

func TestPhaseACLTVAndCSVScriptActivation(t *testing.T) {
	for _, test := range []struct {
		name  string
		op    byte
		valid bool
	}{
		{"CLTV-valid", txscript.OP_CHECKLOCKTIMEVERIFY, true}, {"CLTV-invalid", txscript.OP_CHECKLOCKTIMEVERIFY, false},
		{"CSV-valid", txscript.OP_CHECKSEQUENCEVERIFY, true}, {"CSV-invalid", txscript.OP_CHECKSEQUENCEVERIFY, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := activationSpend()
			threshold := int64(1)
			if !test.valid {
				threshold = 2
			}
			pk, err := txscript.NewScriptBuilder().AddInt64(threshold).AddOp(test.op).AddOp(txscript.OP_DROP).AddOp(txscript.OP_TRUE).Script()
			require.NoError(t, err)
			if test.op == txscript.OP_CHECKLOCKTIMEVERIFY {
				tx.LockTime = 1
				tx.TxIn[0].Sequence = wire.MaxTxInSequenceNum - 1
			} else {
				tx.TxIn[0].Sequence = 1
			}
			require.NoError(t, checkActivationSpend(t, 61, tx, pk, 1))
			err = checkActivationSpend(t, 62, tx, pk, 1)
			if test.valid {
				require.NoError(t, err)
			} else {
				assertActivationRule(t, err, ErrScriptValidation)
			}
		})
	}
}

func TestPhaseACSVRelativeLocks(t *testing.T) {
	for _, test := range []struct {
		name     string
		sequence uint32
		origin   int32
		valid    bool
	}{
		{"height-valid", 2, 60, true}, {"height-invalid", 3, 60, false},
		{"time-valid", wire.SequenceLockTimeIsSeconds | 1, 55, true},
		{"time-invalid", wire.SequenceLockTimeIsSeconds | 1, 60, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := activationSpend()
			tx.TxIn[0].Sequence = test.sequence
			pk := []byte{txscript.OP_TRUE}
			require.NoError(t, checkActivationSpend(t, 61, tx, pk, test.origin))
			err := checkActivationSpend(t, 62, tx, pk, test.origin)
			if test.valid {
				require.NoError(t, err)
			} else {
				assertActivationRule(t, err, ErrUnfinalizedTx)
			}
		})
	}
}

func TestPhaseAMedianTimeFinality(t *testing.T) {
	for _, valid := range []bool{false, true} {
		tx := activationSpend()
		tx.TxIn[0].Sequence = wire.MaxTxInSequenceNum - 1
		parent := activationChain(60).bestChain.Tip()
		if valid {
			tx.LockTime = uint32(CalcPastMedianTime(parent).Unix() - 1)
		} else {
			tx.LockTime = uint32(parent.timestamp - 1)
		}
		require.NoError(t, checkActivationSpend(t, 61, tx, []byte{txscript.OP_TRUE}, 1))
		err := checkActivationSpend(t, 62, tx, []byte{txscript.OP_TRUE}, 1)
		if valid {
			require.NoError(t, err)
		} else {
			assertActivationRule(t, err, ErrUnfinalizedTx)
		}
	}
}

func activationWitnessSpend(t *testing.T, kind string, valid bool) (*wire.MsgTx, []byte) {
	t.Helper()
	key, _ := btcec.PrivKeyFromBytes([]byte{1}) // Public fixture scalar1 only.
	defer key.Zero()
	tx := activationSpend()
	var pk, program []byte
	var err error
	if kind == "taproot" {
		pk, err = txscript.PayToTaprootScript(txscript.ComputeTaprootKeyNoScript(key.PubKey()))
	} else {
		program, err = txscript.NewScriptBuilder().AddOp(txscript.OP_0).AddData(address.Hash160(key.PubKey().SerializeCompressed())).Script()
		pk = program
		if kind == "nested" {
			pk, err = txscript.NewScriptBuilder().AddOp(txscript.OP_HASH160).AddData(address.Hash160(program)).AddOp(txscript.OP_EQUAL).Script()
			require.NoError(t, err)
			tx.TxIn[0].SignatureScript, err = txscript.NewScriptBuilder().AddData(program).Script()
		}
	}
	require.NoError(t, err)
	fetcher := txscript.NewCannedPrevOutputFetcher(pk, activationInputValue)
	hashes := txscript.NewTxSigHashes(tx, fetcher)
	if kind == "taproot" {
		tx.TxIn[0].Witness, err = txscript.TaprootWitnessSignature(tx, hashes, 0, activationInputValue, pk, txscript.SigHashDefault, key)
	} else {
		tx.TxIn[0].Witness, err = txscript.WitnessSignature(tx, hashes, 0, activationInputValue, program, txscript.SigHashAll, key, true)
	}
	require.NoError(t, err)
	if !valid {
		tx.TxIn[0].Witness[0][len(tx.TxIn[0].Witness[0])-2] ^= 1
	}
	return tx, pk
}

func TestPhaseAWitnessAndTaprootScriptActivation(t *testing.T) {
	for _, kind := range []string{"native", "nested", "taproot"} {
		for _, valid := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", kind, valid), func(t *testing.T) {
				tx, pk := activationWitnessSpend(t, kind, valid)
				// Before activation these witness programs have legacy semantics;
				// their key authorization is not yet enforced by consensus.
				require.NoError(t, checkActivationSpend(t, 61, tx, pk, 1))
				err := checkActivationSpend(t, 62, tx, pk, 1)
				if valid {
					require.NoError(t, err)
				} else {
					assertActivationRule(t, err, ErrScriptValidation)
				}
			})
		}
	}
}

func TestPhaseAWitnessCommitmentActivation(t *testing.T) {
	for _, height := range []int32{61, 62} {
		for _, mutation := range []string{"missing", "wrong", "reserved"} {
			t.Run(fmt.Sprintf("%d/%s", height, mutation), func(t *testing.T) {
				b := activationChain(height - 1)
				tx, _ := activationWitnessSpend(t, "native", true)
				block := activationBlock(t, b, tx)
				coinbase := block.MsgBlock().Transactions[0]
				switch mutation {
				case "missing":
					coinbase.TxOut = coinbase.TxOut[:1]
				case "wrong":
					coinbase.TxOut[1].PkScript[6] ^= 1
				case "reserved":
					coinbase.TxIn[0].Witness[0] = []byte{0}
				}
				err := b.checkBlockContext(block, b.bestChain.Tip(), BFNone)
				if height < 62 {
					require.NoError(t, err)
				} else {
					want := ErrUnexpectedWitness
					if mutation == "wrong" {
						want = ErrWitnessCommitmentMismatch
					}
					if mutation == "reserved" {
						want = ErrInvalidWitnessCommitment
					}
					assertActivationRule(t, err, want)
				}
			})
		}
	}
}
