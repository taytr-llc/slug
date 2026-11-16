// Copyright (c) 2014-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package hdkeychain

// References:
//   [BIP32]: BIP0032 - Hierarchical Deterministic Wallets
//   https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	secp_ecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// childHMACMock supplies cancellation outputs without searching for an
// infeasible natural HMAC preimage.
type childHMACMock struct {
	mock.Mock
}

// sum records the child derivation request and returns its arranged HMAC
// output so the real derivation code handles the cancellation.
func (m *childHMACMock) sum(chainCode, data []byte) []byte {
	args := m.Called(chainCode, data)
	return args.Get(0).([]byte)
}

// TestExtendedKeyRejectInvalidChild checks that both derivation methods reject
// final scalar/point cancellation instead of returning an unusable child.
func TestExtendedKeyRejectInvalidChild(t *testing.T) {
	// Arrange: use a valid parent and its matching public key. Negating a
	// copy of the private scalar makes Il cancel either parent's key when
	// added modulo N, producing zero privately and infinity publicly.
	parent, err := NewMaster(
		bytes.Repeat([]byte{1}, MinSeedBytes), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)
	publicParent, err := parent.Neuter()
	require.NoError(t, err)
	privateKey, err := parent.ECPrivKey()
	require.NoError(t, err)
	il := privateKey.Key
	il.Negate()
	ilBytes := il.Bytes()
	forcedILR := make([]byte, 64)
	copy(forcedILR, ilBytes[:])
	originalHMAC := hmacSHA512

	tests := []struct {
		name   string
		parent *ExtendedKey
		derive func(*ExtendedKey, uint32) (*ExtendedKey, error)
	}{
		{
			name:   "standardPrivateZero",
			parent: parent,
			derive: (*ExtendedKey).Derive,
		},
		{
			name:   "standardPublicInfinity",
			parent: publicParent,
			derive: (*ExtendedKey).Derive,
		},
		{
			name:   "nonStandardPrivateZero",
			parent: parent,
			derive: (*ExtendedKey).DeriveNonStandard,
		},
		{
			name:   "nonStandardPublicInfinity",
			parent: publicParent,
			derive: (*ExtendedKey).DeriveNonStandard,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: force the cancelling Il at the hash boundary
			// while the public method performs its normal key math.
			// Run serially; restore the real hash after each case.
			childHMAC := &childHMACMock{}
			childHMAC.On(
				"sum", test.parent.chainCode, mock.Anything,
			).Return(forcedILR).Once()
			hmacSHA512 = childHMAC.sum
			t.Cleanup(func() {
				hmacSHA512 = originalHMAC
				childHMAC.AssertExpectations(t)
			})

			// Act: call the selected public method at index zero.
			// Its final addition must reach the cancellation guard,
			// rather than failing an earlier input check.
			child, err := test.derive(test.parent, 0)

			// Assert: return ErrInvalidChild without a child key,
			// allowing callers to skip this unusable index.
			require.ErrorIs(t, err, ErrInvalidChild)
			require.Nil(t, child)
		})
	}
}

// TestBIP0032Vectors tests the vectors provided by [BIP32] to ensure the
// derivation works as intended.
func TestBIP0032Vectors(t *testing.T) {
	// The master seeds for each of the two test vectors in [BIP32].
	testVec1MasterHex := "000102030405060708090a0b0c0d0e0f"
	testVec2MasterHex := "fffcf9f6f3f0edeae7e4e1dedbd8d5d2cfccc9c6c3c0bdbab7b4b1aeaba8a5a29f9c999693908d8a8784817e7b7875726f6c696663605d5a5754514e4b484542"
	testVec3MasterHex := "4b381541583be4423346c643850da4b320e46a87ae3d2a4e6da11eba819cd4acba45d239319ac14f863b8d5ab5a0d0c64d2e8a1e7d1457df2e5a3c51c73235be"
	hkStart := uint32(0x80000000)

	tests := []struct {
		name     string
		master   string
		path     []uint32
		wantPub  string
		wantPriv string
		net      *chaincfg.Params
	}{
		// Test vector 1
		{
			name:     "test vector 1 chain m",
			master:   testVec1MasterHex,
			path:     []uint32{},
			wantPub:  "JgduaQiYjRcnjcD9szGq1YVnpeCzcrwRJG3KhKhAR78zDTMGbz9iQ4jhxteB2vHL2he9apVCrZFeNYTtFawKyXENEqCscyVQ35AgDK2M1sxnJP6x",
			wantPriv: "RWC8zh2hyeVQ3jDP6u3EzrWzPmLoNhdze3C7h1zf9GbEa5Kr9dySsodrx9GQ9tg6EyLWrJ5QKNxDdP4fhh62ZRN2R7cNsWmmFiQBYa5mi7uNAvxA",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart},
			wantPub:  "JgduaQkp9RL6ChUGs8sxikFfTXF6jBkyee7oi1JbK5ENLRYTtMS1aUyF41nXShYtjSx47phVRRVBoH6xLZLJ7UG6cu6LMB8CHzUaRpxUaBbHfbnL",
			wantPriv: "RWC8zh4yPeChWpUW63eNi4Gs2eNuV2TYzRGbhhc63Egch3X3S1Fk4DsQ3GQkZfwSaP3tp18J1kFYVmRWaQr4H4xJbugTPaeSPxMtSH6QZZAMVUK3",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1},
			wantPub:  "JgduaQnzGd7e66B9vyL1AUWZL7yzPF8xke9XzwdG9fW99NBG6ayWiCmfMyMRg7dMJrtmTnR7UCuduPhSp9e2vwbGWakAHUL39U7ahTh1MUZ8er4k",
			wantPriv: "RWC8zh79WqzFQDBP9t6R9nXkuF7o95qY6RJKzdvkspxPVz9qeEoFBwfpMDyeo5zddyyfrTD3hZ58vr8TzYnaWCJ6agqwq6RQ827fHwtiw64Qpvos",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2},
			wantPub:  "JgduaQqbYfeTwo41LqVoFj6xhUz2XUHqGuh7zHAVpuqSJY3dXzphq1XVnsnybfeY9JmCn8wDCpHcWme1nbyBDegp3eCK84E1HZ4ePBNvVZnPMKgT",
			wantPriv: "RWC8zh9kntX5Fv4EZkGDF38AGc7qHJzQcgquyyTzZ5HgfA2D5eeSJkRen8RCie2rFxDBLxg569Y5rGYHtmzKFnK1mowPVT2xgB3uEZAU9icJmTXY",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2},
			wantPub:  "JgduaQspwW5atyXgKuyGyRNAnNVBqZ781YHSBieP8AnxmrVC9MvLC5JCA5NSXLyXXiZMWcFLNxJ6TpNuuAPXDjzFFMnnNy1ZUPFYDC4qLiQT95Sx",
			wantPriv: "RWC8zhBzBixCD6XuYpjgxjPNMVczbPohMKSEBQwsrLFD8UTmh1k4fpCM9KzfeKMFiQsan2UCnmTKRBMk4FBDhyjZ81KyXrH3mj8iwi8wKEBo9L9s",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2/1000000000",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2, 1000000000},
			wantPub:  "JgduaQuYhymC96f4XSK4RXujWgCBqMLhKMnFDWtmgR5Lv4Zae53jabiSpLUbayoSUZpSE5wcvTe2GMacVkovEqqk6D3wVqYyZBj7r12TFxYsizRa",
			wantPriv: "RWC8zhDhxCdoTDfHkM5UQqvw5oKzbC3Gf8w3DDCGQaXbGgYABisU4Lcbob6phxD2vKnhrdsc3YpJN2QHBFME5WoZr8Hd5Bmozr3UezUVfyGgEL8E",
			net:      &chaincfg.MainNetParams,
		},

		// Test vector 2
		{
			name:     "test vector 2 chain m",
			master:   testVec2MasterHex,
			path:     []uint32{},
			wantPub:  "JgduaQiYjRcnjcD9sytLLees76QiP78ZiwfaX1Gx9Q5j3RjPUH8jbEtUuKYLdGPR1QtWYj3SeTvJXwYPWJj4CeRHPy7sZqRbVufAsdQ289sXinnW",
			wantPriv: "RWC8zh2hyeVQ3jDP6tekKxg4gDYX8wq94ipNWhaSsZXyQ3hy1vxU4yndtaAZkEjsJ9r4wY89k8Eg5fAhtnckfTnR7iyhdyBqVrk6H9LPwUsEoW6G",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 2 chain m/0",
			master:   testVec2MasterHex,
			path:     []uint32{0},
			wantPub:  "JgduaQmpUhLUh6DyxW9vdnSEzAD7uU6Jyee4oZShWZtkVW9FRy7BZp9pyUNusqrNuCyWywMvpL9JHLdEUTtUtXZypJiXr7ULAp1KFkt8nb56bFfy",
			wantPriv: "RWC8zh5yivD61DEDBQvLd6TSZHLvfJntKRnroFkCEjLzr87pycvv3Z3yxj18zpF96nuumwnPa2F5p6L8298PV24vAfCs2yNFZFEG2GkJVrDgjciK",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647},
			wantPub:  "JgduaQnyXwwWDU8A4g28pCtEddYoTrqXtC8EVebn3VHfG9t1pRQR2S3FHgShUTEd1xi85isrMyz9tdqf58ZMEJUTw2rzUeYxEAjaGdGQPRuwK26N",
			wantPriv: "RWC8zh78nAp7Xb8PHanYoWuSCkgcDhY7DyH2VLuGmejucmrbN5E9WAwQGw4vbRbd5XAeFr1iToGHT9Ld8aN8FkSXJEWJbQ6Uu1jzH4Xe2ccEGiok",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H/1",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647, 1},
			wantPub:  "JgduaQqnWMxV9dmTFmdoxGYPCbVjhCNQPJK9ky3GuLMKbTmNG6EHBjYmzaM5rJ2p9kapNK4JupdF8GxxuDMcFDNttJCTPEqA5PH1UZ35LgJLNpkv",
			wantPriv: "RWC8zh9wkaq6TkmgUgQDwaZamidYT34yj5TwkfLmdVoZx5jwok41fUSvypyJyGPphYsqx9y5AYZMqYtJLu4uQoL3gEz7tXs1hrqxfYL4m6L9wEtL",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H/1/2147483646H",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647, 1, hkStart + 2147483646},
			wantPub:  "JgduaQrxYGvRWFAQy4b3Z12Cjy1Rt76skMHnFzjzns521JXdWYac7qDi16sSwvgwewxLpGVxYd8JqKaHkrzmduG1Q1Z9qWjUZwa6tRG9vDFLyW8K",
			wantPriv: "RWC8zhB7nVo2pNAeByMTYK3QK69EdwoT68SaFh3VX2XGMvWD4CQLba7rzMVg4u6Z6BJEEhYMrrP8pwFsviBznKGxAvpo4MoZ213zHsv5HA6SDhV1",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H/1/2147483646H/2",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647, 1, hkStart + 2147483646, 2},
			wantPub:  "JgduaQtKaEMe1mLjCorESj6LRwkorWAQrbchLeucgGrNVwKV6cmwgzrdmHSPx161w5XhWnZAmu3pFDHJkMRq8jMhJKp1R9uJeswPd6ZM6sxEqZzz",
			wantPriv: "RWC8zhCUpTEFKtLxRiceS37Y14tccLrzCNmVLMD7QSJcrZJ4eGbgAjknkY4d4yWE6LKKJjrDb98mePMickjtJ3bKPyoXnAf4X4U81aHQqEcdPZpL",
			net:      &chaincfg.MainNetParams,
		},

		// Test vector 3
		{
			name:     "test vector 3 chain m",
			master:   testVec3MasterHex,
			path:     []uint32{},
			wantPub:  "JgduaQiYjRcnjcD9sxwnWB3MuJPyQymtphXjnicihoNkbhRUGFNxru2isGoa9kNwzEV6Bu8CWHvBYLDDMk2h7V8KqPTXhxfVieDciR7XpgJmdx8Q",
			wantPriv: "RWC8zh2hyeVQ3jDP6siCVV4ZURXnApUUAUgXnQvDRxpzxKQ3ouChLdvsrXRoGijbU6sJGcppVhANBZbSc9GMyhDZYWTCtVUQ94BSFS44J8r89Sot",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 3 chain m/0H",
			master:   testVec3MasterHex,
			path:     []uint32{hkStart},
			wantPub:  "JgduaQkuwAacsHaXEAUsuQBhQMej5g8WCrP9h86xeccSSdjPdWQ6KUXVjMGMs46nVxqgw6VhM4swHBAQrNVdTRNG1xhoedmScC57WuHtjLoNyixo",
			wantPriv: "RWC8zh55BPTEBQakT5FHtiCtyUnXqWq5YdXwgpQTNn4goFhyBADpoDReibtaz2VwpSpdC4yTui7A4DL54np18GS9JGDQWvcKBoLi4Z51x6FN2oco",
			net:      &chaincfg.MainNetParams,
		},

		// Test vector 1 - Testnet
		{
			name:     "test vector 1 chain m - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{},
			wantPub:  "tpubD6NzVbkrYhZ4XgiXtGrdW5XDAPFCL9h7we1vwNCpn8tGbBcgfVYjXyhWo4E1xkh56hjod1RhGjxbaTLV3X4FyWuejifB9jusQ46QzG87VKp",
			wantPriv: "tprv8ZgxMBicQKsPeDgjzdC36fs6bMjGApWDNLR9erAXMs5skhMv36j9MV5ecvfavji5khqjWaWSFhN3YcCUUdiKH6isR4Pwy3U5y5egddBr16m",
			net:      &chaincfg.TestNet3Params,
		},
		{
			name:     "test vector 1 chain m/0H - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart},
			wantPub:  "tpubD8eQVK4Kdxg3gHrF62jGP7dKVCoYiEB8dFSpuTawkL5YxTus5j5pf83vaKnii4bc6v2NVEy81P2gYrJczYne3QNNwMTS53p5uzDyHvnw2jm",
			wantPriv: "tprv8bxNLu25VazNnppTCP4fyhyCvBHcYtzE3wr3cwYeL4HA7yf6TLGEUdS4QC1vLT63TkjRssqJe4CvGNEC8DzW5AoPUw56D1Ayg6HY4oy8QZ9",
			net:      &chaincfg.TestNet3Params,
		},
		{
			name:     "test vector 1 chain m/0H/1 - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1},
			wantPub:  "tpubDApXh6cD2fZ7WjtgpHd8yrWyYaneiFuRZa7fVjMkgxsmC1QzoXW8cgx9zQFJ81Jx4deRGfRE7yXA9A3STsxXj4CKEZJHYgpMYikkas9DBTP",
			wantPriv: "tprv8e8VYgZxtHsSdGrtvdxYaSrryZGiYviWzGWtDDKTGh5NMXAEB8gYSCLHpFCywNs5uqV7ghRjimALQJkRFZnUrLHpzi2pGkwqLtbubgWuQ8q",
			net:      &chaincfg.TestNet3Params,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2},
			wantPub:  "tpubDDRojdS4jYQXNugn4t2WLrZ7mjfAyoVQu7MLk4eurqFCbrc7cHLZX8W5YRS8ZskGR9k9t3PqVv68bVBjAyW4nWM9pTGRddt3GQftg6MVQsm",
			wantPriv: "tprv8gjmbDPpbAirVSezBEMuwSu1Ci9EpUJWKokZTYccSZSomNMLytWyLdtDNHRbucNaRJWWHANf9AzEdWVAqahfyRjVMKbNRhBmxAM8EJr7R15",
			net:      &chaincfg.TestNet3Params,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2 - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2},
			wantPub:  "tpubDFfCa4Z1v25WTPAVm9EbEMiRrYwucPocLbEe12BPBGooxxEUg42vihy1DkRWyftztTsL23snYezF9uXjGGwGW6pQjEpcTpmsH6ajpf4CVPn",
			wantPriv: "tprv8iyAReWmmePqZv8hsVZzpx4KHXRyT4chmHdriW95m11R8Tyi3fDLYDM93bq4NGn1V6eCu5cE3zSQ6hPd31F2ApKXkZgTyn1V78pHjkq1V2v",
			net:      &chaincfg.TestNet3Params,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2/1000000000 - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2, 1000000000},
			wantPub:  "tpubDHNy3kAG39ThyiwwsgoKY4iRenXDRtce8qdCFJZXPMCJg5dsCUHayp84raLTpvyiNA9sXPob5rgqkKvkN8S7MMyXbnEhGJMW64Cf4vFAoaF",
			wantPriv: "tprv8kgvuL81tmn36Fv9z38j8f4K5m1HGZRjZY2QxnXDy5PuqbP6a5TzoKWCgTcGHBu66W3TgSbAu2yX6sPza5FkHmy564Sh6gmCPUNeUt4yj2x",
			net:      &chaincfg.TestNet3Params,
		},
	}

tests:
	for i, test := range tests {
		masterSeed, err := hex.DecodeString(test.master)
		if err != nil {
			t.Errorf("DecodeString #%d (%s): unexpected error: %v",
				i, test.name, err)
			continue
		}

		extKey, err := NewMaster(masterSeed, test.net)
		if err != nil {
			t.Errorf("NewMaster #%d (%s): unexpected error when "+
				"creating new master key: %v", i, test.name,
				err)
			continue
		}

		for _, childNum := range test.path {
			var err error
			extKey, err = extKey.Derive(childNum)
			if err != nil {
				t.Errorf("err: %v", err)
				continue tests
			}
		}

		if extKey.Depth() != uint8(len(test.path)) {
			t.Errorf("Depth of key %d should match fixture path: %v",
				extKey.Depth(), len(test.path))
			continue
		}

		privStr := extKey.String()
		if privStr != test.wantPriv {
			t.Errorf("Serialize #%d (%s): mismatched serialized "+
				"private extended key -- got: %s, want: %s", i,
				test.name, privStr, test.wantPriv)
			continue
		}

		pubKey, err := extKey.Neuter()
		if err != nil {
			t.Errorf("Neuter #%d (%s): unexpected error: %v ", i,
				test.name, err)
			continue
		}

		// Neutering a second time should have no effect.
		pubKey, err = pubKey.Neuter()
		if err != nil {
			t.Errorf("Neuter #%d (%s): unexpected error: %v", i,
				test.name, err)
			return
		}

		pubStr := pubKey.String()
		if pubStr != test.wantPub {
			t.Errorf("Neuter #%d (%s): mismatched serialized "+
				"public extended key -- got: %s, want: %s", i,
				test.name, pubStr, test.wantPub)
			continue
		}
	}
}

// TestPrivateDerivation tests several vectors which derive private keys from
// other private keys works as intended.
func TestPrivateDerivation(t *testing.T) {
	// The private extended keys for test vectors in [BIP32].
	testVec1MasterPrivKey := "RWC8zh2hyeVQ3jDP6u3EzrWzPmLoNhdze3C7h1zf9GbEa5Kr9dySsodrx9GQ9tg6EyLWrJ5QKNxDdP4fhh62ZRN2R7cNsWmmFiQBYa5mi7uNAvxA"
	testVec2MasterPrivKey := "RWC8zh2hyeVQ3jDP6tekKxg4gDYX8wq94ipNWhaSsZXyQ3hy1vxU4yndtaAZkEjsJ9r4wY89k8Eg5fAhtnckfTnR7iyhdyBqVrk6H9LPwUsEoW6G"

	tests := []struct {
		name     string
		master   string
		path     []uint32
		wantPriv string
	}{
		// Test vector 1
		{
			name:     "test vector 1 chain m",
			master:   testVec1MasterPrivKey,
			path:     []uint32{},
			wantPriv: "RWC8zh2hyeVQ3jDP6u3EzrWzPmLoNhdze3C7h1zf9GbEa5Kr9dySsodrx9GQ9tg6EyLWrJ5QKNxDdP4fhh62ZRN2R7cNsWmmFiQBYa5mi7uNAvxA",
		},
		{
			name:     "test vector 1 chain m/0",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0},
			wantPriv: "RWC8zh4yPeChNUoy7tkSLmFdiDBxefyE1GpPT553CqBH6SBbCTuwBhufxhsUz9snEUD8oDurCXabNfekpqx5mnYeaE42JhdCV6hAhAXZMTf73ppx",
		},
		{
			name:     "test vector 1 chain m/0/1",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1},
			wantPriv: "RWC8zh7d5wzF7iThnpkPGYJWycakE6jESnWQXGpU2kkZV7oxCponZx285R3BRXunz1soL9b7pjyFGDjy9tUztqZN2bFps2Z9XqQKDwgKywKDWZxc",
		},
		{
			name:     "test vector 1 chain m/0/1/2",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1, 2},
			wantPriv: "RWC8zh8YbTkhykVMLQqUCfdJU41NUmy846GioBse6ES2BvdD6MgqY1CLnwrjZziaaDingvAp38V85qZf7qVQZxh3gJdrXyvqubjywjh3uKC51tZt",
		},
		{
			name:     "test vector 1 chain m/0/1/2/2",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1, 2, 2},
			wantPriv: "RWC8zhBz6NU4RSYTk3ryAe9eH8as43YssZdsx5rcUr23bjrN2kUdmW378ZQuX9gtXibYXE73Dffz7xb4n7r6TVbqTPPaHbTLbwABunNfoZUypzL9",
		},
		{
			name:     "test vector 1 chain m/0/1/2/2/1000000000",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1, 2, 2, 1000000000},
			wantPriv: "RWC8zhDDffdxZLUB6j3Q3eXvXvLQCop1JuUpGVx4WJtpCSQ5cEcVvQ1Fo9xdQYrrx9sTbSCYLd2mR1GUV4rYbBMMzNhFH3yc8yvTWxzrM2omnP4F",
		},

		// Test vector 2
		{
			name:     "test vector 2 chain m",
			master:   testVec2MasterPrivKey,
			path:     []uint32{},
			wantPriv: "RWC8zh2hyeVQ3jDP6tekKxg4gDYX8wq94ipNWhaSsZXyQ3hy1vxU4yndtaAZkEjsJ9r4wY89k8Eg5fAhtnckfTnR7iyhdyBqVrk6H9LPwUsEoW6G",
		},
		{
			name:     "test vector 2 chain m/0",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0},
			wantPriv: "RWC8zh5yivD61DEDBQvLd6TSZHLvfJntKRnroFkCEjLzr87pycvv3Z3yxj18zpF96nuumwnPa2F5p6L8298PV24vAfCs2yNFZFEG2GkJVrDgjciK",
		},
		{
			name:     "test vector 2 chain m/0/2147483647",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647},
			wantPriv: "RWC8zh78nAp7PFTrKR4c18RQ7WNRET5b35NKQb1TC2sb3aNNTgUMpb3tVhdvtYkEa7aW7jbTC7RLZdG6aDmeGGqWwv1Sniv26d1MBsyS8XuuMYSE",
		},
		{
			name:     "test vector 2 chain m/0/2147483647/1",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647, 1},
			wantPriv: "RWC8zh9ZQAEqsCkqzGq6QD9agJgvF7uCGK12TokJoMewrkDABZSWBUuT6VbvvFNjM4UsbWUQJHhGcmyiwevPykmwGFUAQtyBc4iYL9ueC14CEBui",
		},
		{
			name:     "test vector 2 chain m/0/2147483647/1/2147483646",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647, 1, 2147483646},
			wantPriv: "RWC8zhC2dj9Wi9NJUvMdWD4sDcoDHeTU5a9QGspJAGvy3PKquXhGSJR3gf4juLUskPXwky5DRM7zoWanwvNdXRH6MWqonaUhf8M4nu19TGRiQoxT",
		},
		{
			name:     "test vector 2 chain m/0/2147483647/1/2147483646/2",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647, 1, 2147483646, 2},
			wantPriv: "RWC8zhDp8RS7wwtxD65suwY5jEJoJhXCF5SCNGq8b8VumNpYHZy2MCBDVoeKTza4pWDBY29MQiDRBxpqVB9D3CdZ1spVFFAkbLL9GPmDRc7z38n9",
		},

		// Custom tests to trigger specific conditions.
		{
			// Seed 000000000000000000000000000000da.
			name:     "Derived privkey with zero high byte m/0",
			master:   "RWC8zh2hyeVQ3jDP6utCtNqJh4dcVc77GSd9pVuKAK6dE37R5Qzs2atYgh4HpBPbTrnjhskDf5mjCMSWXgjKHwJfcZhTUinuY1ZTptZxR4hZHNdN",
			path:     []uint32{0},
			wantPriv: "RWC8zh4t3PUtGSX7UTo9GV5X32VoRYDDnUki5oCYiU5qAq4z3Umtu7cjnbFWwKzYgdMn6YskMzvkkgFZB9HS27YsDFj9SaZ22r9wrST3sSRdwU1F",
		},
	}

tests:
	for i, test := range tests {
		extKey, err := NewKeyFromString(test.master)
		if err != nil {
			t.Errorf("NewKeyFromString #%d (%s): unexpected error "+
				"creating extended key: %v", i, test.name,
				err)
			continue
		}

		for _, childNum := range test.path {
			var err error
			extKey, err = extKey.Derive(childNum)
			if err != nil {
				t.Errorf("err: %v", err)
				continue tests
			}
		}

		privStr := extKey.String()
		if privStr != test.wantPriv {
			t.Errorf("Derive #%d (%s): mismatched serialized "+
				"private extended key -- got: %s, want: %s", i,
				test.name, privStr, test.wantPriv)
			continue
		}
	}
}

// TestPublicDerivation tests several vectors which derive public keys from
// other public keys works as intended.
func TestPublicDerivation(t *testing.T) {
	// The public extended keys for test vectors in [BIP32].
	testVec1MasterPubKey := "JgduaQiYjRcnjcD9szGq1YVnpeCzcrwRJG3KhKhAR78zDTMGbz9iQ4jhxteB2vHL2he9apVCrZFeNYTtFawKyXENEqCscyVQ35AgDK2M1sxnJP6x"
	testVec2MasterPubKey := "JgduaQiYjRcnjcD9sytLLees76QiP78ZiwfaX1Gx9Q5j3RjPUH8jbEtUuKYLdGPR1QtWYj3SeTvJXwYPWJj4CeRHPy7sZqRbVufAsdQ289sXinnW"

	tests := []struct {
		name    string
		master  string
		path    []uint32
		wantPub string
	}{
		// Test vector 1
		{
			name:    "test vector 1 chain m",
			master:  testVec1MasterPubKey,
			path:    []uint32{},
			wantPub: "JgduaQiYjRcnjcD9szGq1YVnpeCzcrwRJG3KhKhAR78zDTMGbz9iQ4jhxteB2vHL2he9apVCrZFeNYTtFawKyXENEqCscyVQ35AgDK2M1sxnJP6x",
		},
		{
			name:    "test vector 1 chain m/0",
			master:  testVec1MasterPubKey,
			path:    []uint32{0},
			wantPub: "JgduaQkp9RL64Mojtyz2MTES9649tqGefVfbTNmYUfj2jpD1ep6Chy1WyTFFsBUko9P3jogxUkziw9KCegG8AmxhyBsUXM4H1zoGKz5Gz2nYjSan",
		},
		{
			name:    "test vector 1 chain m/0/1",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1},
			wantPub: "JgduaQoTqj7dobTUZuyyHEHKQVSwUG2f71McXaWyJbJK8VqNfAz46D7y6AQxJZXdjWjyTtsaGxGjyX9GZ2LPKMe63RdHKq9cZRLszXdxAWe4kaiP",
		},
		{
			name:    "test vector 1 chain m/0/1/2",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1, 2},
			wantPub: "JgduaQpPMEt6fdV87W54DMc6tvsZiwGYiK7voVa9N4ymqJedYhs74GJBohEWT2MSEYttSDRuQtUe9KoADMy7qtqD71pLx2RKxHkVVqPdo6P6SoHe",
		},
		{
			name:    "test vector 1 chain m/0/1/2/2",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1, 2, 2},
			wantPub: "JgduaQspr9bT7KYEX96ZBL8Si1T4JCrJXnV5xPZ7kgZoF7snV6euHm8x9JngQBK3oumrZzXLL6abdFvKk1WJJ9TA6otNsHCc45kfxaXyLAfXqRog",
		},
		{
			name:    "test vector 1 chain m/0/1/2/2/1000000000",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1, 2, 2, 1000000000},
			wantPub: "JgduaQu4RSmMFDTwspGz4LWixoCbSy7Ry8L2GoeZn9SZqpRW4anmSf76ouLQHaTzuCYpZ5ZGzGepLBdMywEyKpkn2LXH1raDi6zq4Gj4Ze7mkyuG",
		},

		// Test vector 2
		{
			name:    "test vector 2 chain m",
			master:  testVec2MasterPubKey,
			path:    []uint32{},
			wantPub: "JgduaQiYjRcnjcD9sytLLees76QiP78ZiwfaX1Gx9Q5j3RjPUH8jbEtUuKYLdGPR1QtWYj3SeTvJXwYPWJj4CeRHPy7sZqRbVufAsdQ289sXinnW",
		},
		{
			name:    "test vector 2 chain m/0",
			master:  testVec2MasterPubKey,
			path:    []uint32{0},
			wantPub: "JgduaQmpUhLUh6DyxW9vdnSEzAD7uU6Jyee4oZShWZtkVW9FRy7BZp9pyUNusqrNuCyWywMvpL9JHLdEUTtUtXZypJiXr7ULAp1KFkt8nb56bFfy",
		},
		{
			name:    "test vector 2 chain m/0/2147483647",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647},
			wantPub: "JgduaQnyXwwW58Td6WJC1pQCYPEcUcP1hJDXQthxTsRLgxPnv2edLr9jWT1hmaNtEUe7JBbnngjxEk5pHPgAbs996RwTkAhUH6L9nHNKJwoKiRDK",
		},
		{
			name:    "test vector 2 chain m/0/2147483647/1",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647, 1},
			wantPub: "JgduaQqQ9wNEZ5kcmN4gQu8P7BZ7VHCcvXrEU7Sp5CChW8Eaducmhk1J7EyhoGzz6G2NQdFNGdJEQa3Gx6NibuDnA2PvcYdy7z9ZjveftWp94faa",
		},
		{
			name:    "test vector 2 chain m/0/2147483647/1/2147483646",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647, 1, 2147483646},
			wantPub: "JgduaQssPWGuQ2N5G1bDWu3feVfQXoktjnzcHBWoS7UigmMGMssXxZWthQSWnN5UBtYkp9ApJFNr8HanJzkULyti2XnUzTm9bSQn2v7Dm46hUmt7",
		},
		{
			name:    "test vector 2 chain m/0/2147483647/1/2147483646/2",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647, 1, 2147483646, 2},
			wantPub: "JgduaQuetCZWdptizBKTvdWtA7AzYrpcuJHQNaXdry3fQkqxjv9HsTH4WZ26M2CqRXd5usZDvcYyUBHGbf1TwhrhMw8SwsZFr5bAbhCcxEKuavHU",
		},
	}

tests:
	for i, test := range tests {
		extKey, err := NewKeyFromString(test.master)
		if err != nil {
			t.Errorf("NewKeyFromString #%d (%s): unexpected error "+
				"creating extended key: %v", i, test.name,
				err)
			continue
		}

		for _, childNum := range test.path {
			var err error
			extKey, err = extKey.Derive(childNum)
			if err != nil {
				t.Errorf("err: %v", err)
				continue tests
			}
		}

		pubStr := extKey.String()
		if pubStr != test.wantPub {
			t.Errorf("Derive #%d (%s): mismatched serialized "+
				"public extended key -- got: %s, want: %s", i,
				test.name, pubStr, test.wantPub)
			continue
		}
	}
}

// TestGenerateSeed ensures the GenerateSeed function works as intended.
func TestGenerateSeed(t *testing.T) {
	wantErr := errors.New("seed length must be between 128 and 512 bits")

	tests := []struct {
		name   string
		length uint8
		err    error
	}{
		// Test various valid lengths.
		{name: "16 bytes", length: 16},
		{name: "17 bytes", length: 17},
		{name: "20 bytes", length: 20},
		{name: "32 bytes", length: 32},
		{name: "64 bytes", length: 64},

		// Test invalid lengths.
		{name: "15 bytes", length: 15, err: wantErr},
		{name: "65 bytes", length: 65, err: wantErr},
	}

	for i, test := range tests {
		seed, err := GenerateSeed(test.length)
		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("GenerateSeed #%d (%s): unexpected error -- "+
				"want %v, got %v", i, test.name, test.err, err)
			continue
		}

		if test.err == nil && len(seed) != int(test.length) {
			t.Errorf("GenerateSeed #%d (%s): length mismatch -- "+
				"got %d, want %d", i, test.name, len(seed),
				test.length)
			continue
		}
	}
}

// TestExtendedKeyAPI ensures the API on the ExtendedKey type works as intended.
func TestExtendedKeyAPI(t *testing.T) {
	tests := []struct {
		name       string
		extKey     string
		isPrivate  bool
		parentFP   uint32
		chainCode  []byte
		childNum   uint32
		privKey    string
		privKeyErr error
		pubKey     string
		address    string
	}{
		{
			name:      "test vector 1 master node private",
			extKey:    "RWC8zh2hyeVQ3jDP6u3EzrWzPmLoNhdze3C7h1zf9GbEa5Kr9dySsodrx9GQ9tg6EyLWrJ5QKNxDdP4fhh62ZRN2R7cNsWmmFiQBYa5mi7uNAvxA",
			isPrivate: true,
			parentFP:  0,
			chainCode: []byte{135, 61, 255, 129, 192, 47, 82, 86, 35, 253, 31, 229, 22, 126, 172, 58, 85, 160, 73, 222, 61, 49, 75, 180, 46, 226, 39, 255, 237, 55, 213, 8},
			childNum:  0,
			privKey:   "e8f32e723decf4051aefac8e2c93c9c5b214313817cdb01a1494b917c8436b35",
			pubKey:    "0339a36013301597daef41fbe593a02cc513d0b55527ec2df1050e2e8ff49c85c2",
			address:   "aUpP3bmZjqURPYg5khGrCLoVFSTb6gChoo",
		},
		{
			name:       "test vector 1 chain m/0H/1/2H public",
			extKey:     "JgduaQqbYfeTwo41LqVoFj6xhUz2XUHqGuh7zHAVpuqSJY3dXzphq1XVnsnybfeY9JmCn8wDCpHcWme1nbyBDegp3eCK84E1HZ4ePBNvVZnPMKgT",
			isPrivate:  false,
			parentFP:   3203769081,
			chainCode:  []byte{4, 70, 107, 156, 200, 225, 97, 233, 102, 64, 156, 165, 41, 134, 197, 132, 240, 126, 157, 200, 31, 115, 93, 182, 131, 195, 255, 110, 199, 177, 80, 63},
			childNum:   2147483650,
			privKeyErr: ErrNotPrivExtKey,
			pubKey:     "0357bfe1e341d01c69fe5654309956cbea516822fba8a601743a012a7896ee8dc2",
			address:    "amo2Zbu4WXyWZyh23J9W6aeXdmVSzWjjZG",
		},
	}

	for i, test := range tests {
		key, err := NewKeyFromString(test.extKey)
		if err != nil {
			t.Errorf("NewKeyFromString #%d (%s): unexpected "+
				"error: %v", i, test.name, err)
			continue
		}

		if key.IsPrivate() != test.isPrivate {
			t.Errorf("IsPrivate #%d (%s): mismatched key type -- "+
				"want private %v, got private %v", i, test.name,
				test.isPrivate, key.IsPrivate())
			continue
		}

		parentFP := key.ParentFingerprint()
		if parentFP != test.parentFP {
			t.Errorf("ParentFingerprint #%d (%s): mismatched "+
				"parent fingerprint -- want %d, got %d", i,
				test.name, test.parentFP, parentFP)
			continue
		}

		chainCode := key.ChainCode()
		if !bytes.Equal(chainCode, test.chainCode) {
			t.Errorf("ChainCode #%d (%s): want %v, got %v", i,
				test.name, test.chainCode, chainCode)
			continue
		}

		childIndex := key.ChildIndex()
		if childIndex != test.childNum {
			t.Errorf("ChildIndex #%d (%s): want %d, got %d", i,
				test.name, test.childNum, childIndex)
			continue
		}

		serializedKey := key.String()
		if serializedKey != test.extKey {
			t.Errorf("String #%d (%s): mismatched serialized key "+
				"-- want %s, got %s", i, test.name, test.extKey,
				serializedKey)
			continue
		}

		privKey, err := key.ECPrivKey()
		if !reflect.DeepEqual(err, test.privKeyErr) {
			t.Errorf("ECPrivKey #%d (%s): mismatched error: want "+
				"%v, got %v", i, test.name, test.privKeyErr, err)
			continue
		}
		if test.privKeyErr == nil {
			privKeyStr := hex.EncodeToString(privKey.Serialize())
			if privKeyStr != test.privKey {
				t.Errorf("ECPrivKey #%d (%s): mismatched "+
					"private key -- want %s, got %s", i,
					test.name, test.privKey, privKeyStr)
				continue
			}
		}

		pubKey, err := key.ECPubKey()
		if err != nil {
			t.Errorf("ECPubKey #%d (%s): unexpected error: %v", i,
				test.name, err)
			continue
		}
		pubKeyStr := hex.EncodeToString(pubKey.SerializeCompressed())
		if pubKeyStr != test.pubKey {
			t.Errorf("ECPubKey #%d (%s): mismatched public key -- "+
				"want %s, got %s", i, test.name, test.pubKey,
				pubKeyStr)
			continue
		}

		addr, err := key.Address(&chaincfg.MainNetParams)
		if err != nil {
			t.Errorf("Address #%d (%s): unexpected error: %v", i,
				test.name, err)
			continue
		}
		if addr.EncodeAddress() != test.address {
			t.Errorf("Address #%d (%s): mismatched address -- want "+
				"%s, got %s", i, test.name, test.address,
				addr.EncodeAddress())
			continue
		}
	}
}

// TestNet ensures the network related APIs work as intended.
func TestNet(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		origNet   *chaincfg.Params
		newNet    *chaincfg.Params
		newPriv   string
		newPub    string
		isPrivate bool
	}{
		// Private extended keys.
		{
			name:      "mainnet -> simnet",
			key:       "RWC8zh2hyeVQ3jDP6u3EzrWzPmLoNhdze3C7h1zf9GbEa5Kr9dySsodrx9GQ9tg6EyLWrJ5QKNxDdP4fhh62ZRN2R7cNsWmmFiQBYa5mi7uNAvxA",
			origNet:   &chaincfg.MainNetParams,
			newNet:    &chaincfg.SimNetParams,
			newPriv:   "sprv8Erh3X3hFeKunvVdAGQQtambRPapECWiTDtvsTGdyrhzhbYgnSZajRRWbihzvq4AM4ivm6uso31VfKaukwJJUs3GYihXP8ebhMb3F2AHu3P",
			newPub:    "spub4Tr3T2ab61tD1Qa6GHwRFiiKyRRJdfEZpSpXfqgFYCEyaPsqKysqHDjzSzMJSiUEGbcsG3w2SLMoTqn44B8x6u3MLRRkYfACTUBnHK79THk",
			isPrivate: true,
		},
		{
			name:      "simnet -> mainnet",
			key:       "sprv8Erh3X3hFeKunvVdAGQQtambRPapECWiTDtvsTGdyrhzhbYgnSZajRRWbihzvq4AM4ivm6uso31VfKaukwJJUs3GYihXP8ebhMb3F2AHu3P",
			origNet:   &chaincfg.SimNetParams,
			newNet:    &chaincfg.MainNetParams,
			newPriv:   "RWC8zh2hyeVQ3jDP6u3EzrWzPmLoNhdze3C7h1zf9GbEa5Kr9dySsodrx9GQ9tg6EyLWrJ5QKNxDdP4fhh62ZRN2R7cNsWmmFiQBYa5mi7uNAvxA",
			newPub:    "JgduaQiYjRcnjcD9szGq1YVnpeCzcrwRJG3KhKhAR78zDTMGbz9iQ4jhxteB2vHL2he9apVCrZFeNYTtFawKyXENEqCscyVQ35AgDK2M1sxnJP6x",
			isPrivate: true,
		},
		{
			name:      "mainnet -> regtest",
			key:       "RWC8zh2hyeVQ3jDP6u3EzrWzPmLoNhdze3C7h1zf9GbEa5Kr9dySsodrx9GQ9tg6EyLWrJ5QKNxDdP4fhh62ZRN2R7cNsWmmFiQBYa5mi7uNAvxA",
			origNet:   &chaincfg.MainNetParams,
			newNet:    &chaincfg.RegressionNetParams,
			newPriv:   "tprv8ZgxMBicQKsPeDgjzdC36fs6bMjGApWDNLR9erAXMs5skhMv36j9MV5ecvfavji5khqjWaWSFhN3YcCUUdiKH6isR4Pwy3U5y5egddBr16m",
			newPub:    "tpubD6NzVbkrYhZ4XgiXtGrdW5XDAPFCL9h7we1vwNCpn8tGbBcgfVYjXyhWo4E1xkh56hjod1RhGjxbaTLV3X4FyWuejifB9jusQ46QzG87VKp",
			isPrivate: true,
		},
		{
			name:      "regtest -> mainnet",
			key:       "tprv8ZgxMBicQKsPeDgjzdC36fs6bMjGApWDNLR9erAXMs5skhMv36j9MV5ecvfavji5khqjWaWSFhN3YcCUUdiKH6isR4Pwy3U5y5egddBr16m",
			origNet:   &chaincfg.RegressionNetParams,
			newNet:    &chaincfg.MainNetParams,
			newPriv:   "RWC8zh2hyeVQ3jDP6u3EzrWzPmLoNhdze3C7h1zf9GbEa5Kr9dySsodrx9GQ9tg6EyLWrJ5QKNxDdP4fhh62ZRN2R7cNsWmmFiQBYa5mi7uNAvxA",
			newPub:    "JgduaQiYjRcnjcD9szGq1YVnpeCzcrwRJG3KhKhAR78zDTMGbz9iQ4jhxteB2vHL2he9apVCrZFeNYTtFawKyXENEqCscyVQ35AgDK2M1sxnJP6x",
			isPrivate: true,
		},

		// Public extended keys.
		{
			name:      "mainnet -> simnet",
			key:       "JgduaQiYjRcnjcD9szGq1YVnpeCzcrwRJG3KhKhAR78zDTMGbz9iQ4jhxteB2vHL2he9apVCrZFeNYTtFawKyXENEqCscyVQ35AgDK2M1sxnJP6x",
			origNet:   &chaincfg.MainNetParams,
			newNet:    &chaincfg.SimNetParams,
			newPub:    "spub4Tr3T2ab61tD1Qa6GHwRFiiKyRRJdfEZpSpXfqgFYCEyaPsqKysqHDjzSzMJSiUEGbcsG3w2SLMoTqn44B8x6u3MLRRkYfACTUBnHK79THk",
			isPrivate: false,
		},
		{
			name:      "simnet -> mainnet",
			key:       "spub4Tr3T2ab61tD1Qa6GHwRFiiKyRRJdfEZpSpXfqgFYCEyaPsqKysqHDjzSzMJSiUEGbcsG3w2SLMoTqn44B8x6u3MLRRkYfACTUBnHK79THk",
			origNet:   &chaincfg.SimNetParams,
			newNet:    &chaincfg.MainNetParams,
			newPub:    "JgduaQiYjRcnjcD9szGq1YVnpeCzcrwRJG3KhKhAR78zDTMGbz9iQ4jhxteB2vHL2he9apVCrZFeNYTtFawKyXENEqCscyVQ35AgDK2M1sxnJP6x",
			isPrivate: false,
		},
		{
			name:      "mainnet -> regtest",
			key:       "JgduaQiYjRcnjcD9szGq1YVnpeCzcrwRJG3KhKhAR78zDTMGbz9iQ4jhxteB2vHL2he9apVCrZFeNYTtFawKyXENEqCscyVQ35AgDK2M1sxnJP6x",
			origNet:   &chaincfg.MainNetParams,
			newNet:    &chaincfg.RegressionNetParams,
			newPub:    "tpubD6NzVbkrYhZ4XgiXtGrdW5XDAPFCL9h7we1vwNCpn8tGbBcgfVYjXyhWo4E1xkh56hjod1RhGjxbaTLV3X4FyWuejifB9jusQ46QzG87VKp",
			isPrivate: false,
		},
		{
			name:      "regtest -> mainnet",
			key:       "tpubD6NzVbkrYhZ4XgiXtGrdW5XDAPFCL9h7we1vwNCpn8tGbBcgfVYjXyhWo4E1xkh56hjod1RhGjxbaTLV3X4FyWuejifB9jusQ46QzG87VKp",
			origNet:   &chaincfg.RegressionNetParams,
			newNet:    &chaincfg.MainNetParams,
			newPub:    "JgduaQiYjRcnjcD9szGq1YVnpeCzcrwRJG3KhKhAR78zDTMGbz9iQ4jhxteB2vHL2he9apVCrZFeNYTtFawKyXENEqCscyVQ35AgDK2M1sxnJP6x",
			isPrivate: false,
		},
	}

	for i, test := range tests {
		extKey, err := NewKeyFromString(test.key)
		if err != nil {
			t.Errorf("NewKeyFromString #%d (%s): unexpected error "+
				"creating extended key: %v", i, test.name,
				err)
			continue
		}

		if !extKey.IsForNet(test.origNet) {
			t.Errorf("IsForNet #%d (%s): key is not for expected "+
				"network %v", i, test.name, test.origNet.Name)
			continue
		}

		extKey.SetNet(test.newNet)
		if !extKey.IsForNet(test.newNet) {
			t.Errorf("SetNet/IsForNet #%d (%s): key is not for "+
				"expected network %v", i, test.name,
				test.newNet.Name)
			continue
		}

		if test.isPrivate {
			privStr := extKey.String()
			if privStr != test.newPriv {
				t.Errorf("Serialize #%d (%s): mismatched serialized "+
					"private extended key -- got: %s, want: %s", i,
					test.name, privStr, test.newPriv)
				continue
			}

			extKey, err = extKey.Neuter()
			if err != nil {
				t.Errorf("Neuter #%d (%s): unexpected error: %v ", i,
					test.name, err)
				continue
			}
		}

		pubStr := extKey.String()
		if pubStr != test.newPub {
			t.Errorf("Neuter #%d (%s): mismatched serialized "+
				"public extended key -- got: %s, want: %s", i,
				test.name, pubStr, test.newPub)
			continue
		}
	}
}

// TestErrors performs some negative tests for various invalid cases to ensure
// the errors are handled properly.
func TestErrors(t *testing.T) {
	// Should get an error when seed has too few bytes.
	net := &chaincfg.MainNetParams
	_, err := NewMaster(bytes.Repeat([]byte{0x00}, 15), net)
	if err != ErrInvalidSeedLen {
		t.Fatalf("NewMaster: mismatched error -- got: %v, want: %v",
			err, ErrInvalidSeedLen)
	}

	// Should get an error when seed has too many bytes.
	_, err = NewMaster(bytes.Repeat([]byte{0x00}, 65), net)
	if err != ErrInvalidSeedLen {
		t.Fatalf("NewMaster: mismatched error -- got: %v, want: %v",
			err, ErrInvalidSeedLen)
	}

	// Generate a new key and neuter it to a public extended key.
	seed, err := GenerateSeed(RecommendedSeedLen)
	if err != nil {
		t.Fatalf("GenerateSeed: unexpected error: %v", err)
	}
	extKey, err := NewMaster(seed, net)
	if err != nil {
		t.Fatalf("NewMaster: unexpected error: %v", err)
	}
	pubKey, err := extKey.Neuter()
	if err != nil {
		t.Fatalf("Neuter: unexpected error: %v", err)
	}

	// Deriving a hardened child extended key should fail from a public key.
	_, err = pubKey.Derive(HardenedKeyStart)
	if err != ErrDeriveHardFromPublic {
		t.Fatalf("Derive: mismatched error -- got: %v, want: %v",
			err, ErrDeriveHardFromPublic)
	}

	// NewKeyFromString failure tests.
	tests := []struct {
		name      string
		key       string
		err       error
		neuter    bool
		neuterErr error
	}{
		{
			name: "invalid key length",
			key:  "xpub1234",
			err:  ErrInvalidKeyLen,
		},
		{
			name: "bad checksum",
			key:  "xpub661MyMwAqRbcFtXgS5sYJABqqG9YLmC4Q1Rdap9gSE8NqtwybGhePY2gZ29ESFjqJoCu1Rupje8YtGqsefD265TMg7usUDFdp6W1EBygr15",
			err:  ErrBadChecksum,
		},
		{
			name: "pubkey not on curve",
			key:  "JgduaQiYjRcnjcD9szGq1YVnpeCzcrwRJG3KhKhAR78zDTMGbz9iQ4jhxteB2vGteQpMMFq5W8TejZEHfgwfYfFLMXmEmPmmHbf8T2UsjwfvJ4Xk",
			err:  secp_ecdsa.ErrPubKeyNotOnCurve,
		},
		{
			name:      "unsupported version",
			key:       "xbad4LfUL9eKmA66w2GJdVMqhvDmYGJpTGjWRAtjHqoUY17sGaymoMV9Cm3ocn9Ud6Hh2vLFVC7KSKCRVVrqc6dsEdsTjRV1WUmkK85YEUujAPX",
			err:       nil,
			neuter:    true,
			neuterErr: chaincfg.ErrUnknownHDKeyID,
		},
	}

	for i, test := range tests {
		extKey, err := NewKeyFromString(test.key)
		if !errors.Is(err, test.err) {
			t.Errorf("NewKeyFromString #%d (%s): mismatched error "+
				"-- got: %v, want: %v", i, test.name, err,
				test.err)
			continue
		}

		if test.neuter {
			_, err := extKey.Neuter()
			if !errors.Is(err, test.neuterErr) {
				t.Errorf("Neuter #%d (%s): mismatched error "+
					"-- got: %v, want: %v", i, test.name,
					err, test.neuterErr)
				continue
			}
		}
	}
}

// TestZero ensures that zeroing an extended key works as intended.
func TestZero(t *testing.T) {
	tests := []struct {
		name   string
		master string
		extKey string
		net    *chaincfg.Params
	}{
		// Test vector 1
		{
			name:   "test vector 1 chain m",
			master: "000102030405060708090a0b0c0d0e0f",
			extKey: "RWC8zh2hyeVQ3jDP6u3EzrWzPmLoNhdze3C7h1zf9GbEa5Kr9dySsodrx9GQ9tg6EyLWrJ5QKNxDdP4fhh62ZRN2R7cNsWmmFiQBYa5mi7uNAvxA",
			net:    &chaincfg.MainNetParams,
		},

		// Test vector 2
		{
			name:   "test vector 2 chain m",
			master: "fffcf9f6f3f0edeae7e4e1dedbd8d5d2cfccc9c6c3c0bdbab7b4b1aeaba8a5a29f9c999693908d8a8784817e7b7875726f6c696663605d5a5754514e4b484542",
			extKey: "RWC8zh2hyeVQ3jDP6tekKxg4gDYX8wq94ipNWhaSsZXyQ3hy1vxU4yndtaAZkEjsJ9r4wY89k8Eg5fAhtnckfTnR7iyhdyBqVrk6H9LPwUsEoW6G",
			net:    &chaincfg.MainNetParams,
		},
	}

	// Use a closure to test that a key is zeroed since the tests create
	// keys in different ways and need to test the same things multiple
	// times.
	testZeroed := func(i int, testName string, key *ExtendedKey) bool {
		// Zeroing a key should result in it no longer being private
		if key.IsPrivate() {
			t.Errorf("IsPrivate #%d (%s): mismatched key type -- "+
				"want private %v, got private %v", i, testName,
				false, key.IsPrivate())
			return false
		}

		parentFP := key.ParentFingerprint()
		if parentFP != 0 {
			t.Errorf("ParentFingerprint #%d (%s): mismatched "+
				"parent fingerprint -- want %d, got %d", i,
				testName, 0, parentFP)
			return false
		}

		wantKey := "zeroed extended key"
		serializedKey := key.String()
		if serializedKey != wantKey {
			t.Errorf("String #%d (%s): mismatched serialized key "+
				"-- want %s, got %s", i, testName, wantKey,
				serializedKey)
			return false
		}

		wantErr := ErrNotPrivExtKey
		_, err := key.ECPrivKey()
		if !reflect.DeepEqual(err, wantErr) {
			t.Errorf("ECPrivKey #%d (%s): mismatched error: want "+
				"%v, got %v", i, testName, wantErr, err)
			return false
		}

		wantErr = secp_ecdsa.ErrPubKeyInvalidLen
		_, err = key.ECPubKey()
		if !errors.Is(err, wantErr) {
			t.Errorf("ECPubKey #%d (%s): mismatched error: want "+
				"%v, got %v", i, testName, wantErr, err)
			return false
		}

		wantAddr := "agWBgUmHcdFNzBeAqSdUgaD5cCtQuedkn5"
		addr, err := key.Address(&chaincfg.MainNetParams)
		if err != nil {
			t.Errorf("Address #%d (%s): unexpected error: %v", i,
				testName, err)
			return false
		}
		if addr.EncodeAddress() != wantAddr {
			t.Errorf("Address #%d (%s): mismatched address -- want "+
				"%s, got %s", i, testName, wantAddr,
				addr.EncodeAddress())
			return false
		}

		return true
	}

	for i, test := range tests {
		// Create new key from seed and get the neutered version.
		masterSeed, err := hex.DecodeString(test.master)
		if err != nil {
			t.Errorf("DecodeString #%d (%s): unexpected error: %v",
				i, test.name, err)
			continue
		}
		key, err := NewMaster(masterSeed, test.net)
		if err != nil {
			t.Errorf("NewMaster #%d (%s): unexpected error when "+
				"creating new master key: %v", i, test.name,
				err)
			continue
		}
		neuteredKey, err := key.Neuter()
		if err != nil {
			t.Errorf("Neuter #%d (%s): unexpected error: %v", i,
				test.name, err)
			continue
		}

		// Ensure both non-neutered and neutered keys are zeroed
		// properly.
		key.Zero()
		if !testZeroed(i, test.name+" from seed not neutered", key) {
			continue
		}
		neuteredKey.Zero()
		if !testZeroed(i, test.name+" from seed neutered", key) {
			continue
		}

		// Deserialize key and get the neutered version.
		key, err = NewKeyFromString(test.extKey)
		if err != nil {
			t.Errorf("NewKeyFromString #%d (%s): unexpected "+
				"error: %v", i, test.name, err)
			continue
		}
		neuteredKey, err = key.Neuter()
		if err != nil {
			t.Errorf("Neuter #%d (%s): unexpected error: %v", i,
				test.name, err)
			continue
		}

		// Ensure both non-neutered and neutered keys are zeroed
		// properly.
		key.Zero()
		if !testZeroed(i, test.name+" deserialized not neutered", key) {
			continue
		}
		neuteredKey.Zero()
		if !testZeroed(i, test.name+" deserialized neutered", key) {
			continue
		}
	}
}

// TestMaximumDepth ensures that attempting to retrieve a child key when already
// at the maximum depth is not allowed.  The serialization of a BIP32 key uses
// uint8 to encode the depth.  This implicitly bounds the depth of the tree to
// 255 derivations.  Here we test that an error is returned after 'max uint8'.
func TestMaximumDepth(t *testing.T) {
	net := &chaincfg.MainNetParams
	extKey, err := NewMaster([]byte(`abcd1234abcd1234abcd1234abcd1234`), net)
	if err != nil {
		t.Fatalf("NewMaster: unexpected error: %v", err)
	}

	for i := uint8(0); i < math.MaxUint8; i++ {
		if extKey.Depth() != i {
			t.Fatalf("extendedkey depth %d should match expected value %d",
				extKey.Depth(), i)
		}
		newKey, err := extKey.Derive(1)
		if err != nil {
			t.Fatalf("Derive: unexpected error: %v", err)
		}
		extKey = newKey
	}

	noKey, err := extKey.Derive(1)
	if err != ErrDeriveBeyondMaxDepth {
		t.Fatalf("Derive: mismatched error: want %v, got %v",
			ErrDeriveBeyondMaxDepth, err)
	}
	if noKey != nil {
		t.Fatal("Derive: deriving 256th key should not succeed")
	}
}

// TestCloneWithVersion ensures proper conversion between standard and SLIP132
// extended keys.
//
// The following tool was used for generating the tests:
//
//	https://jlopp.github.io/xpub-converter
func TestCloneWithVersion(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		version []byte
		want    string
		wantErr error
	}{
		{
			name:    "test xpub to zpub",
			key:     "JgduaQiYjRcnjcD9szGq1YVnpeCzcrwRJG3KhKhAR78zDTMGbz9iQ4jhxteB2vHL2he9apVCrZFeNYTtFawKyXENEqCscyVQ35AgDK2M1sxnJP6x",
			version: []byte{0x04, 0xb2, 0x47, 0x46},
			want:    "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
		},
		{
			name:    "test zpub to xpub",
			key:     "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
			version: []byte{0x04, 0x88, 0xb2, 0x1e},
			want:    "xpub661MyMwAqRbcFtXgS5sYJABqqG9YLmC4Q1Rdap9gSE8NqtwybGhePY2gZ29ESFjqJoCu1Rupje8YtGqsefD265TMg7usUDFdp6W1EGMcet8",
		},
		{
			name:    "test xprv to zprv",
			key:     "RWC8zh2hyeVQ3jDP6u3EzrWzPmLoNhdze3C7h1zf9GbEa5Kr9dySsodrx9GQ9tg6EyLWrJ5QKNxDdP4fhh62ZRN2R7cNsWmmFiQBYa5mi7uNAvxA",
			version: []byte{0x04, 0xb2, 0x43, 0x0c},
			want:    "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
		},
		{
			name:    "test zprv to xprv",
			key:     "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			version: []byte{0x04, 0x88, 0xad, 0xe4},
			want:    "xprv9s21ZrQH143K3QTDL4LXw2F7HEK3wJUD2nW2nRk4stbPy6cq3jPPqjiChkVvvNKmPGJxWUtg6LnF5kejMRNNU3TGtRBeJgk33yuGBxrMPHi",
		},
		{
			name:    "test invalid key id",
			key:     "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			version: []byte{0x4B, 0x1D},
			wantErr: chaincfg.ErrUnknownHDKeyID,
		},
	}

	for i, test := range tests {
		extKey, err := NewKeyFromString(test.key)
		if err != nil {
			panic(err) // This is never expected to fail.
		}

		got, err := extKey.CloneWithVersion(test.version)
		if !reflect.DeepEqual(err, test.wantErr) {
			t.Errorf("CloneWithVersion #%d (%s): unexpected error -- "+
				"want %v, got %v", i, test.name, test.wantErr, err)
			continue
		}

		if test.wantErr == nil {
			if k := got.String(); k != test.want {
				t.Errorf("CloneWithVersion #%d (%s): "+
					"got %s, want %s", i, test.name, k, test.want)
				continue
			}
		}
	}
}

// TestLeadingZero ensures that deriving children from keys with a leading zero byte is done according
// to the BIP-32 standard and that the legacy method generates a backwards-compatible result.
func TestLeadingZero(t *testing.T) {
	// The 400th seed results in a m/0' public key with a leading zero, allowing us to test
	// the desired behavior.
	ii := 399
	seed := make([]byte, 32)
	binary.BigEndian.PutUint32(seed[28:], uint32(ii))
	masterKey, err := NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("hdkeychain.NewMaster failed: %v", err)
	}
	child0, err := masterKey.Derive(0 + HardenedKeyStart)
	if err != nil {
		t.Fatalf("masterKey.Derive failed: %v", err)
	}
	if !child0.IsAffectedByIssue172() {
		t.Fatal("expected child0 to be affected by issue 172")
	}
	child1, err := child0.Derive(0 + HardenedKeyStart)
	if err != nil {
		t.Fatalf("child0.Derive failed: %v", err)
	}
	if child1.IsAffectedByIssue172() {
		t.Fatal("did not expect child1 to be affected by issue 172")
	}

	child1nonstandard, err := child0.DeriveNonStandard(0 + HardenedKeyStart)
	if err != nil {
		t.Fatalf("child0.DeriveNonStandard failed: %v", err)
	}

	// This is the correct result based on BIP32
	if hex.EncodeToString(child1.key) != "a9b6b30a5b90b56ed48728c73af1d8a7ef1e9cc372ec21afcc1d9bdf269b0988" {
		t.Error("incorrect standard BIP32 derivation")
	}

	if hex.EncodeToString(child1nonstandard.key) != "ea46d8f58eb863a2d371a938396af8b0babe85c01920f59a8044412e70e837ee" {
		t.Error("incorrect btcutil backwards compatible BIP32-like derivation")
	}

	if !child0.IsAffectedByIssue172() {
		t.Error("child 0 should be affected by issue 172")
	}

	if child1.IsAffectedByIssue172() {
		t.Error("child 1 should not be affected by issue 172")
	}
}
