package blockchain

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/taytr-llc/slug/blockchain/internal/testhelper"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"golang.org/x/crypto/argon2"
)

func nativeTestGenesis(params *chaincfg.Params) *btcutil.Block {
	block := btcutil.NewBlock(params.GenesisBlock)
	block.SetHeight(0)
	return block
}

// nativeTestHeaderHash independently serializes and hashes fixture headers.
// These fixtures are actually mined under the selected compile-time profile;
// they do not reuse Bitcoin PoW, skip validation, or invent expected digests.
func nativeTestHeaderHash(header *wire.BlockHeader) chainhash.Hash {
	var raw [80]byte
	binary.LittleEndian.PutUint32(raw[0:4], uint32(header.Version))
	copy(raw[4:36], header.PrevBlock[:])
	copy(raw[36:68], header.MerkleRoot[:])
	binary.LittleEndian.PutUint32(raw[68:72], uint32(header.Timestamp.Unix()))
	binary.LittleEndian.PutUint32(raw[72:76], header.Bits)
	binary.LittleEndian.PutUint32(raw[76:80], header.Nonce)
	memory := uint32(256 * 1024)
	if wire.TestPoW {
		memory = 64
	}
	digest := argon2.IDKey(raw[:], raw[:], 1, memory, 1, 32)
	var hash chainhash.Hash
	copy(hash[:], digest)
	return hash
}

// newNativeTestBlock creates a deterministic coinbase-only child. The extra
// nonce gives same-height branches distinct coinbase transactions and headers.
func newNativeTestBlock(t testing.TB, params *chaincfg.Params,
	parent *btcutil.Block, extraNonce uint64) *btcutil.Block {

	t.Helper()
	height := parent.Height() + 1
	coinbase := testhelper.CreateCoinbaseTx(height, CalcBlockSubsidy(height, params))
	script, err := testhelper.StandardCoinbaseScript(height, extraNonce)
	if err != nil {
		t.Fatal(err)
	}
	coinbase.TxIn[0].SignatureScript = script
	msg := &wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:    4,
			PrevBlock:  *parent.Hash(),
			MerkleRoot: coinbase.TxHash(),
			Timestamp:  parent.MsgBlock().Header.Timestamp.Add(params.TargetTimePerBlock),
			Bits:       params.PowLimitBits,
		},
		Transactions: []*wire.MsgTx{coinbase},
	}
	target := CompactToBig(msg.Header.Bits)
	for nonce := uint32(0); nonce < 1024; nonce++ {
		msg.Header.Nonce = nonce
		hash := nativeTestHeaderHash(&msg.Header)
		var bigEndian [chainhash.HashSize]byte
		for i := range hash {
			bigEndian[len(hash)-1-i] = hash[i]
		}
		if new(big.Int).SetBytes(bigEndian[:]).Cmp(target) > 0 {
			continue
		}
		block := btcutil.NewBlock(msg)
		block.SetHeight(height)
		if *block.Hash() != hash {
			t.Fatal("fixture digest differs from independent Argon2id derivation")
		}
		return block
	}
	t.Fatal("unable to solve short native fixture within 1024 attempts")
	return nil
}

// nativeShortFork returns genesis, 1, 2, 3, 4, and 3a (forking after 2).
func nativeShortFork(t testing.TB) []*btcutil.Block {
	t.Helper()
	params := &chaincfg.MainNetParams
	blocks := []*btcutil.Block{nativeTestGenesis(params)}
	for height := 1; height <= 4; height++ {
		blocks = append(blocks, newNativeTestBlock(t, params, blocks[height-1], 0))
	}
	return append(blocks, newNativeTestBlock(t, params, blocks[2], 1))
}
