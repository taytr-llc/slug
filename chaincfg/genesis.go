// Copyright (c) 2014-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

// genesisCoinbaseTx is the coinbase transaction for the genesis blocks for
// the main network, regression test network, and test network (version 3).
var genesisCoinbaseTx = wire.MsgTx{
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, 0x45, /* |.......E| */
				0x54, 0x68, 0x65, 0x20, 0x54, 0x69, 0x6d, 0x65, /* |The Time| */
				0x73, 0x20, 0x30, 0x33, 0x2f, 0x4a, 0x61, 0x6e, /* |s 03/Jan| */
				0x2f, 0x32, 0x30, 0x30, 0x39, 0x20, 0x43, 0x68, /* |/2009 Ch| */
				0x61, 0x6e, 0x63, 0x65, 0x6c, 0x6c, 0x6f, 0x72, /* |ancellor| */
				0x20, 0x6f, 0x6e, 0x20, 0x62, 0x72, 0x69, 0x6e, /* | on brin| */
				0x6b, 0x20, 0x6f, 0x66, 0x20, 0x73, 0x65, 0x63, /* |k of sec|*/
				0x6f, 0x6e, 0x64, 0x20, 0x62, 0x61, 0x69, 0x6c, /* |ond bail| */
				0x6f, 0x75, 0x74, 0x20, 0x66, 0x6f, 0x72, 0x20, /* |out for |*/
				0x62, 0x61, 0x6e, 0x6b, 0x73, /* |banks| */
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value: 0x12a05f200,
			PkScript: []byte{
				0x41, 0x04, 0x67, 0x8a, 0xfd, 0xb0, 0xfe, 0x55, /* |A.g....U| */
				0x48, 0x27, 0x19, 0x67, 0xf1, 0xa6, 0x71, 0x30, /* |H'.g..q0| */
				0xb7, 0x10, 0x5c, 0xd6, 0xa8, 0x28, 0xe0, 0x39, /* |..\..(.9| */
				0x09, 0xa6, 0x79, 0x62, 0xe0, 0xea, 0x1f, 0x61, /* |..yb...a| */
				0xde, 0xb6, 0x49, 0xf6, 0xbc, 0x3f, 0x4c, 0xef, /* |..I..?L.| */
				0x38, 0xc4, 0xf3, 0x55, 0x04, 0xe5, 0x1e, 0xc1, /* |8..U....| */
				0x12, 0xde, 0x5c, 0x38, 0x4d, 0xf7, 0xba, 0x0b, /* |..\8M...| */
				0x8d, 0x57, 0x8a, 0x4c, 0x70, 0x2b, 0x6b, 0xf1, /* |.W.Lp+k.| */
				0x1d, 0x5f, 0xac, /* |._.| */
			},
		},
	},
	LockTime: 0,
}

// SLUG (change set 1): the genesis hash constants in this file are the
// Argon2id digests of the genesis headers, recomputed after the
// BlockHash swap in wire.
//
// SLUG (change set 2): the main network genesis block below is now
// SLUG's own genesis (new coinbase message, timestamp, and bits; see
// slugGenesisCoinbaseTx). The other networks still use the inherited
// Bitcoin genesis blocks, rehashed by change set 1; regtest and simnet
// keep theirs for good, and testnet3/testnet4/signet cleanup belongs
// to the rebrand change set.

// slugGenesisCoinbaseTx is the coinbase transaction for the SLUG main
// network genesis block. The signature script carries the message
// "SLUG genesis, November 15 2026. Every fighter starts somewhere."
// in the same layout Bitcoin's genesis used: a push of the block bits
// (0x207fffff), a push of 0x04, then a push of the message. The single
// output pays the 10 SLUG genesis subsidy to an OP_RETURN script
// ("SLUG"), which makes it provably unspendable: this is a Phase A
// test-chain genesis and the permanent genesis is decided later.
var slugGenesisCoinbaseTx = wire.MsgTx{
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				0x04, 0xff, 0xff, 0x7f, 0x20, 0x01, 0x04, 0x3f, /* |.......?| */
				0x53, 0x4c, 0x55, 0x47, 0x20, 0x67, 0x65, 0x6e, /* |SLUG gen| */
				0x65, 0x73, 0x69, 0x73, 0x2c, 0x20, 0x4e, 0x6f, /* |esis, No| */
				0x76, 0x65, 0x6d, 0x62, 0x65, 0x72, 0x20, 0x31, /* |vember 1| */
				0x35, 0x20, 0x32, 0x30, 0x32, 0x36, 0x2e, 0x20, /* |5 2026. | */
				0x45, 0x76, 0x65, 0x72, 0x79, 0x20, 0x66, 0x69, /* |Every fi| */
				0x67, 0x68, 0x74, 0x65, 0x72, 0x20, 0x73, 0x74, /* |ghter st| */
				0x61, 0x72, 0x74, 0x73, 0x20, 0x73, 0x6f, 0x6d, /* |arts som| */
				0x65, 0x77, 0x68, 0x65, 0x72, 0x65, 0x2e, /* |ewhere.| */
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value: 0x3b9aca00, // 10 SLUG (1,000,000,000 base units), burned
			PkScript: []byte{
				0x6a, 0x04, 0x53, 0x4c, 0x55, 0x47, // OP_RETURN "SLUG"
			},
		},
	},
	LockTime: 0,
}

// genesisHash is the hash of the first block in the block chain for the main
// network (genesis block). SLUG: Argon2id digest of the SLUG genesis header.
var genesisHash = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0xcd, 0x46, 0xe2, 0x0c, 0x23, 0xa7, 0x99, 0xa9,
	0x26, 0x33, 0xcf, 0x22, 0x24, 0x9f, 0xa1, 0x22,
	0x94, 0x7d, 0x34, 0x6a, 0xfb, 0x8d, 0x19, 0xbb,
	0x85, 0x99, 0xcf, 0x99, 0x60, 0x67, 0x93, 0x78,
})

// genesisMerkleRoot is the hash of the first transaction in the genesis block
// for the main network.
var genesisMerkleRoot = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0x30, 0xaf, 0x33, 0x68, 0x15, 0x30, 0x75, 0x97,
	0x99, 0xbe, 0x9f, 0x57, 0xed, 0x6d, 0xac, 0x59,
	0x2a, 0x21, 0x6f, 0x61, 0x14, 0xea, 0x12, 0xbd,
	0x89, 0x0d, 0x84, 0x20, 0x67, 0x25, 0x88, 0x7e,
})

// genesisBlock defines the genesis block of the block chain which serves as the
// public transaction ledger for the main network.
var genesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},         // 0000000000000000000000000000000000000000000000000000000000000000
		MerkleRoot: genesisMerkleRoot,        // 7e88256720840d89bd12ea14616f212a59ac6ded579fbe99977530156833af30
		Timestamp:  time.Unix(1794700800, 0), // 2026-11-15 00:00:00 +0000 UTC
		Bits:       0x207fffff,               // 545259519 [7fffff0000000000000000000000000000000000000000000000000000000000]
		Nonce:      0,                        // Argon2id satisfied the target on the first attempt
	},
	Transactions: []*wire.MsgTx{&slugGenesisCoinbaseTx},
}

// regTestGenesisHash is the hash of the first block in the block chain for the
// regression test network (genesis block).
var regTestGenesisHash = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0xa6, 0xd3, 0xa6, 0xe3, 0x6f, 0xb4, 0x23, 0x77,
	0x1f, 0x58, 0xca, 0xcd, 0x3c, 0xfa, 0x39, 0xc7,
	0x01, 0x89, 0x3c, 0xc2, 0x9c, 0x8f, 0x24, 0x08,
	0xba, 0x7d, 0x81, 0x4c, 0x3f, 0xb1, 0xbd, 0xe1,
})

// regTestGenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the regression test network.  SLUG: it stays the inherited
// Bitcoin genesis merkle root; it is NOT the SLUG mainnet root.
var regTestGenesisMerkleRoot = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
	0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
	0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
	0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a,
})

// regTestGenesisBlock defines the genesis block of the block chain which serves
// as the public transaction ledger for the regression test network.
var regTestGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},         // 0000000000000000000000000000000000000000000000000000000000000000
		MerkleRoot: regTestGenesisMerkleRoot, // 4a5e1e4baab89f3a32518a88c31bc87f618f76673e2cc77ab2127b7afdeda33b
		Timestamp:  time.Unix(1296688602, 0), // 2011-02-02 23:16:42 +0000 UTC
		Bits:       0x207fffff,               // 545259519 [7fffff0000000000000000000000000000000000000000000000000000000000]
		Nonce:      2,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// testNet3GenesisHash is the hash of the first block in the block chain for the
// test network (version 3).
var testNet3GenesisHash = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0x76, 0xfc, 0x14, 0xc5, 0x47, 0x23, 0x88, 0x4e,
	0xda, 0x4d, 0x86, 0xd7, 0x24, 0xd1, 0x3e, 0xf5,
	0xf3, 0xa5, 0x46, 0x24, 0x7f, 0xaa, 0x8f, 0xd0,
	0x2b, 0xf1, 0xaf, 0xae, 0x2f, 0x3d, 0x5e, 0x90,
})

// testNet3GenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the test network (version 3).  SLUG: it stays the inherited
// Bitcoin genesis merkle root; it is NOT the SLUG mainnet root.
var testNet3GenesisMerkleRoot = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
	0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
	0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
	0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a,
})

// testNet3GenesisBlock defines the genesis block of the block chain which
// serves as the public transaction ledger for the test network (version 3).
var testNet3GenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},          // 0000000000000000000000000000000000000000000000000000000000000000
		MerkleRoot: testNet3GenesisMerkleRoot, // 4a5e1e4baab89f3a32518a88c31bc87f618f76673e2cc77ab2127b7afdeda33b
		Timestamp:  time.Unix(1296688602, 0),  // 2011-02-02 23:16:42 +0000 UTC
		Bits:       0x1d00ffff,                // 486604799 [00000000ffff0000000000000000000000000000000000000000000000000000]
		Nonce:      0x18aea41a,                // 414098458
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// testNet4GenesisTx is the transaction for the genesis blocks for test network (version 4).
var testNet4GenesisTx = wire.MsgTx{
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				// Message: `03/May/2024 000000000000000000001ebd58c244970b3aa9d783bb001011fbe8ea8e98e00e`
				0x4, 0xff, 0xff, 0x0, 0x1d, 0x1, 0x4, 0x4c,
				0x4c, 0x30, 0x33, 0x2f, 0x4d, 0x61, 0x79, 0x2f,
				0x32, 0x30, 0x32, 0x34, 0x20, 0x30, 0x30, 0x30,
				0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
				0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
				0x30, 0x31, 0x65, 0x62, 0x64, 0x35, 0x38, 0x63,
				0x32, 0x34, 0x34, 0x39, 0x37, 0x30, 0x62, 0x33,
				0x61, 0x61, 0x39, 0x64, 0x37, 0x38, 0x33, 0x62,
				0x62, 0x30, 0x30, 0x31, 0x30, 0x31, 0x31, 0x66,
				0x62, 0x65, 0x38, 0x65, 0x61, 0x38, 0x65, 0x39,
				0x38, 0x65, 0x30, 0x30, 0x65},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value: 0x12a05f200,
			PkScript: []byte{
				0x21, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
				0x0, 0x0, 0xac},
		},
	},
	LockTime: 0,
}

// testNet4GenesisHash is the hash of the first block in the block chain for the
// test network (version 4).
var testNet4GenesisHash = chainhash.Hash([chainhash.HashSize]byte{
	0x69, 0x2d, 0xdf, 0x7f, 0x69, 0x0f, 0xcd, 0xd5,
	0x16, 0xb2, 0xec, 0xcf, 0x7c, 0xdc, 0x08, 0x73,
	0x2b, 0x98, 0x05, 0x95, 0x9c, 0xde, 0xd1, 0x0d,
	0x94, 0xe0, 0x45, 0x3d, 0xa5, 0x1f, 0xb6, 0xc5})

// testNet4GenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the test network (version 4).  It is the same as the merkle root
// for the main network.
var testNet4GenesisMerkleRoot = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0x4e, 0x7b, 0x2b, 0x91, 0x28, 0xfe, 0x02, 0x91,
	0xdb, 0x06, 0x93, 0xaf, 0x2a, 0xe4, 0x18, 0xb7,
	0x67, 0xe6, 0x57, 0xcd, 0x40, 0x7e, 0x80, 0xcb,
	0x14, 0x34, 0x22, 0x1e, 0xae, 0xa7, 0xa0, 0x7a,
})

// testNet4GenesisBlock defines the genesis block of the block chain which
// serves as the public transaction ledger for the test network (version 3).
var testNet4GenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},          // 0000000000000000000000000000000000000000000000000000000000000000
		MerkleRoot: testNet4GenesisMerkleRoot, // 4a5e1e4baab89f3a32518a88c31bc87f618f76673e2cc77ab2127b7afdeda33b
		Timestamp:  time.Unix(1714777860, 0),  // 2024-05-03 23:11:00 +0000 UTC
		Bits:       0x1d00ffff,                // 486604799 [00000000ffff0000000000000000000000000000000000000000000000000000]
		Nonce:      0x17780cbb,                // 393743547
	},
	Transactions: []*wire.MsgTx{&testNet4GenesisTx},
}

// simNetGenesisHash is the hash of the first block in the block chain for the
// simulation test network.
var simNetGenesisHash = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0x84, 0xfb, 0xa9, 0xcb, 0x39, 0x41, 0x35, 0xbd,
	0x91, 0x9e, 0x62, 0xc2, 0xe1, 0x5e, 0x6d, 0x99,
	0x6e, 0x60, 0x99, 0x41, 0x41, 0xd5, 0xa4, 0xb7,
	0xfd, 0x9f, 0x32, 0xf2, 0xe0, 0x63, 0x1d, 0x91,
})

// simNetGenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the simulation test network.  SLUG: it stays the inherited
// Bitcoin genesis merkle root; it is NOT the SLUG mainnet root.
var simNetGenesisMerkleRoot = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
	0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
	0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
	0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a,
})

// simNetGenesisBlock defines the genesis block of the block chain which serves
// as the public transaction ledger for the simulation test network.
var simNetGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},         // 0000000000000000000000000000000000000000000000000000000000000000
		MerkleRoot: simNetGenesisMerkleRoot,  // 4a5e1e4baab89f3a32518a88c31bc87f618f76673e2cc77ab2127b7afdeda33b
		Timestamp:  time.Unix(1401292357, 0), // 2014-05-28 15:52:37 +0000 UTC
		Bits:       0x207fffff,               // 545259519 [7fffff0000000000000000000000000000000000000000000000000000000000]
		Nonce:      2,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// sigNetGenesisHash is the hash of the first block in the block chain for the
// signet test network.
var sigNetGenesisHash = chainhash.Hash{
	0xe6, 0x1d, 0xd5, 0xd4, 0x5d, 0xec, 0xe4, 0xb6,
	0x16, 0x85, 0x66, 0xaa, 0x7a, 0x70, 0x26, 0x00,
	0x1f, 0x55, 0x33, 0xcd, 0x47, 0xb5, 0x0a, 0x60,
	0x73, 0xcc, 0x3b, 0x3a, 0xdc, 0x30, 0x37, 0x60,
}

// sigNetGenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the signet test network. SLUG: it stays the inherited
// Bitcoin genesis merkle root; it is NOT the SLUG mainnet root.
var sigNetGenesisMerkleRoot = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
	0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
	0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
	0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a,
})

// sigNetGenesisBlock defines the genesis block of the block chain which serves
// as the public transaction ledger for the signet test network.
var sigNetGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},         // 0000000000000000000000000000000000000000000000000000000000000000
		MerkleRoot: sigNetGenesisMerkleRoot,  // 4a5e1e4baab89f3a32518a88c31bc87f618f76673e2cc77ab2127b7afdeda33b
		Timestamp:  time.Unix(1598918400, 0), // 2020-09-01 00:00:00 +0000 UTC
		Bits:       0x1e0377ae,               // 503543726 [00000377ae000000000000000000000000000000000000000000000000000000]
		Nonce:      52613770,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}
