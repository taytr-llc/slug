// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"encoding/hex"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func transaction(inputs []Coin, payments []Payment, change *Coin) (*wire.MsgTx, error) {
	if len(inputs) == 0 || len(inputs) > MaxInputs || len(payments) == 0 || len(payments) > 256 {
		return nil, ErrInvalid
	}
	tx := wire.NewMsgTx(2)
	seen := make(map[string]bool)
	var inTotal, outTotal int64
	for _, coin := range inputs {
		if validCoin(coin, false) != nil || coin.Atoms <= 0 || coin.Atoms > MaxMoney-inTotal || seen[coinKey(coin)] {
			return nil, ErrInvalid
		}
		seen[coinKey(coin)] = true
		inTotal += coin.Atoms
		hash, _ := chainhash.NewHashFromStr(coin.TxID)
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, coin.Index), nil, nil))
	}
	seenGyms, seenScripts := make(map[string]bool), make(map[string]bool)
	for i, payment := range payments {
		script, ok := nativeScript(payment.Script)
		if _, gymOK := hexBytes(payment.GymID, 32); !gymOK || !ok || payment.Atoms < MinimumAtoms || payment.Atoms > MaxMoney-outTotal || seenGyms[payment.GymID] || seenScripts[payment.Script] || i > 0 && payment.GymID <= payments[i-1].GymID {
			return nil, ErrInvalid
		}
		seenGyms[payment.GymID], seenScripts[payment.Script] = true, true
		outTotal += payment.Atoms
		tx.AddTxOut(wire.NewTxOut(payment.Atoms, script))
	}
	if change != nil {
		if validCoin(*change, true) != nil || change.Coinbase || change.Index != uint32(len(payments)) || change.Atoms < minimumChangeAtoms || change.Atoms > MaxMoney-outTotal || seenScripts[change.Script] {
			return nil, ErrInvalid
		}
		script, _ := hex.DecodeString(change.Script)
		outTotal += change.Atoms
		tx.AddTxOut(wire.NewTxOut(change.Atoms, script))
	}
	if inTotal < outTotal {
		return nil, ErrInvalid
	}
	return tx, nil
}

func unsignedTransaction(inputs []Coin, payments []Payment, change *Coin) ([]byte, error) {
	tx, err := transaction(inputs, payments, change)
	if err != nil {
		return nil, err
	}
	var raw bytes.Buffer
	if tx.Serialize(&raw) != nil || raw.Len() > MaxTransactionBytes {
		return nil, ErrInvalid
	}
	return raw.Bytes(), nil
}

// validateSigned checks the actual retained witness bytes. Signing callbacks
// cannot change destinations, inputs, fees, locktime, sequence or sighash mode.
func validateSigned(inputs []Coin, payments []Payment, change *Coin, fee int64, raw []byte) (string, error) {
	if fee < 0 || fee > MaxMoney || len(raw) == 0 || len(raw) > MaxTransactionBytes {
		return "", ErrInvalid
	}
	expected, err := transaction(inputs, payments, change)
	if err != nil {
		return "", err
	}
	reader := bytes.NewReader(raw)
	tx := new(wire.MsgTx)
	if tx.Deserialize(reader) != nil || reader.Len() != 0 || tx.Version != 2 || tx.LockTime != 0 || len(tx.TxIn) != len(expected.TxIn) || len(tx.TxOut) != len(expected.TxOut) {
		return "", ErrInvalid
	}
	var canonical bytes.Buffer
	if tx.Serialize(&canonical) != nil || !bytes.Equal(canonical.Bytes(), raw) {
		return "", ErrInvalid
	}
	var inTotal, outTotal int64
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	for i, coin := range inputs {
		input := tx.TxIn[i]
		if input.PreviousOutPoint != expected.TxIn[i].PreviousOutPoint || input.Sequence != wire.MaxTxInSequenceNum || len(input.SignatureScript) != 0 || len(input.Witness) != 2 || len(input.Witness[0]) < 2 || input.Witness[0][len(input.Witness[0])-1] != byte(txscript.SigHashAll) || len(input.Witness[1]) != 33 {
			return "", ErrInvalid
		}
		inTotal += coin.Atoms
		script, _ := hex.DecodeString(coin.Script)
		fetcher.AddPrevOut(input.PreviousOutPoint, wire.NewTxOut(coin.Atoms, script))
	}
	for i, output := range tx.TxOut {
		if output.Value != expected.TxOut[i].Value || !bytes.Equal(output.PkScript, expected.TxOut[i].PkScript) {
			return "", ErrInvalid
		}
		outTotal += output.Value
	}
	if inTotal-outTotal != fee {
		return "", ErrInvalid
	}
	hashes := txscript.NewTxSigHashes(tx, fetcher)
	for i, coin := range inputs {
		script, _ := hex.DecodeString(coin.Script)
		engine, err := txscript.NewEngine(script, tx, i, txscript.StandardVerifyFlags, nil, hashes, coin.Atoms, fetcher)
		if err != nil || engine.Execute() != nil {
			return "", ErrInvalid
		}
	}
	txid := tx.TxHash().String()
	if change != nil && change.TxID != "" && change.TxID != txid {
		return "", ErrInvalid
	}
	return txid, nil
}
