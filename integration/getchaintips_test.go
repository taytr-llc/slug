package integration

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/stretchr/testify/require"
)

func getBlockFromString(t *testing.T, hexStr string) *btcutil.Block {
	t.Helper()

	serializedBlock, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("couldn't decode hex string of %s", hexStr)
	}

	block, err := btcutil.NewBlockFromBytes(serializedBlock)
	if err != nil {
		t.Fatalf("couldn't make a new block from bytes. "+
			"Decoded hex string: %s", hexStr)
	}

	return block
}

// compareMultipleChainTips checks that all the expected chain tips are included in got chain tips and
// verifies that the got chain tip matches the expected chain tip.
func compareMultipleChainTips(t *testing.T, gotChainTips, expectedChainTips []*btcjson.GetChainTipsResult) error {
	if len(gotChainTips) != len(expectedChainTips) {
		return fmt.Errorf("Expected %d chaintips but got %d", len(expectedChainTips), len(gotChainTips))
	}

	gotChainTipsMap := make(map[string]btcjson.GetChainTipsResult)
	for _, gotChainTip := range gotChainTips {
		gotChainTipsMap[gotChainTip.Hash] = *gotChainTip
	}

	for _, expectedChainTip := range expectedChainTips {
		gotChainTip, found := gotChainTipsMap[expectedChainTip.Hash]
		if !found {
			return fmt.Errorf("Couldn't find expected chaintip with hash %s", expectedChainTip.Hash)
		}

		require.Equal(t, gotChainTip, *expectedChainTip)
	}

	return nil
}

func TestGetChainTips(t *testing.T) {
	// block1Hex is a block that builds on top of the regtest genesis block.
	// Has blockhash of "36c056247e8c0589f6307995e4e13acf2b2b79cad9ecd5a4eeab2131ed0ecde5".
	block1Hex := "00000020a6d3a6e36fb423771f58cacd3cfa39c701893cc29c8f2408ba7d814c3fb1bde122ba4c15e9533fa71c9edd26afae9d9ffefc3a54" +
		"3b868edc2fd0cf94f7b23beef8b41d67ffff7f20000000000101000000010000000000000000000000000000000000000000000000000000" +
		"000000000000ffffffff0f51736c756720666978747572652031ffffffff0100ca9a3b00000000015100000000"

	// block2Hex is a block that builds on top of block1Hex.
	// Has blockhash of "664b51334782a4ad16e8471b530dcd0027c75b8c25187b41dfc85ecd353295c6".
	block2Hex := "00000020d6cd039409116767b75145cd7185a6d3cf72c74d72b2e4188b89f0acaeb608477ca72bdae708f8c0d0485590f3637eab7b335a95" +
		"c076595c88110aba9a140eae70b51d67ffff7f20000000000101000000010000000000000000000000000000000000000000000000000000" +
		"000000000000ffffffff0f52736c756720666978747572652032ffffffff0100ca9a3b00000000015100000000"

	// block3Hex is a block that builds on top of block2Hex.
	// Has blockhash of "17a5c5cb90ecde5a46dd195d434eea46b653e35e4517070eade429db3ac83944".
	block3Hex := "00000020d5d0353fde94084aa9cb3e7b90b00f0017993973291a27cd7d4ff5abcc2a5950a7af53f94155edff3ff75a28d53bea98bfbda458" +
		"04417abec6d33f3552167b6be8b51d67ffff7f20010000000101000000010000000000000000000000000000000000000000000000000000" +
		"000000000000ffffffff0f53736c756720666978747572652033ffffffff0100ca9a3b00000000015100000000"

	// block4Hex is a block that builds on top of block3Hex.
	// Has blockhash of "7b357f3073c4397d6d069a32a09141c32560f3c62233ca138eb5e03c5991f45c".
	block4Hex := "0000002077e66e456e3a82de2c293bb6477fee8004bc2e32c194d97405036978c9022e2aced2570fc9119c17654850a43e0754dd8c90ae5a" +
		"c09efe6b45a422efb99687ef60b61d67ffff7f20000000000101000000010000000000000000000000000000000000000000000000000000" +
		"000000000000ffffffff0f54736c756720666978747572652034ffffffff0100ca9a3b00000000015100000000"

	// block2aHex is a block that builds on top of block1Hex.
	// Has blockhash of "5181a4e34cc23ed95c69749dedf4cc7ebd659243bc1683372f8940c8cd8f9b68".
	block2aHex := "00000020d6cd039409116767b75145cd7185a6d3cf72c74d72b2e4188b89f0acaeb608476c7d4c1ef45428b2ce9f615054c4a955c5154c9a" +
		"dc7b18fe10b501bf85f4b2f57fb51d67ffff7f20010000000101000000010000000000000000000000000000000000000000000000000000" +
		"000000000000ffffffff1052736c75672066697874757265203261ffffffff0100ca9a3b00000000015100000000"

	// block3aHex is a block that builds on top of block2aHex.
	// Has blockhash of "0b0216936d1a5c01362256d06a9c9a2b13768fa2f2748549a71008af36dd167f".
	block3aHex := "00000020e15d6a0062c188af40c147ebdd116e0e2fc247bba4eff67173d0a48dea524573841c8d923747c1c7e6ad65a9125b3253ee4bf66b" +
		"08cbf8d5ca977cf218c99847f7b51d67ffff7f20000000000101000000010000000000000000000000000000000000000000000000000000" +
		"000000000000ffffffff1053736c75672066697874757265203361ffffffff0100ca9a3b00000000015100000000"

	// block4aHex is a block that builds on top of block3aHex.
	// Has blockhash of "65a00a026eaa83f6e7a7f4a920faa090f3f9d3565a56df2362db2ab2fa14ccec".
	block4aHex := "00000020aa74c9ca21eaa7b7e6b1fff9b6389e0d5fe1f6a42e4212316cb4ab546522221170638d84be16d889e35d202540d74a55cb9e2412" +
		"e5a1537c3efc446aef3e18996fb61d67ffff7f20010000000101000000010000000000000000000000000000000000000000000000000000" +
		"000000000000ffffffff1054736c75672066697874757265203461ffffffff0100ca9a3b00000000015100000000"

	// block5aHex is a block that builds on top of block4aHex.
	// Has blockhash of "5c8814bc034a4c37fa5ccdc05e09b45a771bd7505d68092f21869a912737ee10".
	block5aHex := "00000020fff4fbbcc75749ff5fe8fdca67937735925db8281f095ab0a264c6d28f24d15170982a57987cf4393145e60e865db26f35adafb5" +
		"f7e35c5835e9c3327bb7b45ae7b61d67ffff7f20000000000101000000010000000000000000000000000000000000000000000000000000" +
		"000000000000ffffffff1055736c75672066697874757265203561ffffffff0100ca9a3b00000000015100000000"

	// block4bHex is a block that builds on top of block3aHex.
	// Has blockhash of "130458e795cc46f2759195e92737426fb0ada2a07f98434551ffb7500b23c161".
	block4bHex := "00000020aa74c9ca21eaa7b7e6b1fff9b6389e0d5fe1f6a42e4212316cb4ab546522221189c80bcb99d23885dd1e48eb27897564c3161cf5" +
		"cce30fa417c0bea21cf5cd828db61d67ffff7f20000000000101000000010000000000000000000000000000000000000000000000000000" +
		"000000000000ffffffff1054736c75672066697874757265203462ffffffff0100ca9a3b00000000015100000000"

	// Set up regtest chain.
	r, err := rpctest.New(&chaincfg.RegressionNetParams, nil, nil, "")
	if err != nil {
		t.Fatal("TestGetChainTips fail. Unable to create primary harness: ", err)
	}
	if err := r.SetUp(true, 0); err != nil {
		t.Fatalf("TestGetChainTips fail. Unable to setup test chain: %v", err)
	}
	defer r.TearDown()

	// Immediately call getchaintips after setting up regtest.
	gotChainTips, err := r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	// We expect a single genesis block.
	expectedChainTips := []*btcjson.GetChainTipsResult{
		{
			Height:    0,
			Hash:      chaincfg.RegressionNetParams.GenesisHash.String(),
			BranchLen: 0,
			Status:    "active",
		},
	}
	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}

	// Submit 4 blocks.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2 -> 3 -> 4
	blockStrings := []string{block1Hex, block2Hex, block3Hex, block4Hex}
	for _, blockString := range blockStrings {
		block := getBlockFromString(t, blockString)
		err = r.Client.SubmitBlock(block, nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	gotChainTips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      getBlockFromString(t, blockStrings[len(blockStrings)-1]).Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
	}
	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}

	// Submit 2 blocks that don't build on top of the current active tip.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4  (active)
	//                    \ -> 2a -> 3a       (valid-fork)
	blockStrings = []string{block2aHex, block3aHex}
	for _, blockString := range blockStrings {
		block := getBlockFromString(t, blockString)
		err = r.Client.SubmitBlock(block, nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	gotChainTips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4Hex).Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
		{
			Height:    3,
			Hash:      getBlockFromString(t, block3aHex).Hash().String(),
			BranchLen: 2,
			Status:    "valid-fork",
		},
	}
	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}

	// Submit a single block that don't build on top of the current active tip.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4   (active)
	//                    \ -> 2a -> 3a -> 4a  (valid-fork)
	block := getBlockFromString(t, block4aHex)
	err = r.Client.SubmitBlock(block, nil)
	if err != nil {
		t.Fatal(err)
	}

	gotChainTips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4Hex).Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4aHex).Hash().String(),
			BranchLen: 3,
			Status:    "valid-fork",
		},
	}
	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}

	// Submit a single block that changes the active branch to 5a.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4         (valid-fork)
	//                    \ -> 2a -> 3a -> 4a -> 5a  (active)
	block = getBlockFromString(t, block5aHex)
	err = r.Client.SubmitBlock(block, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotChainTips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4Hex).Hash().String(),
			BranchLen: 3,
			Status:    "valid-fork",
		},
		{
			Height:    5,
			Hash:      getBlockFromString(t, block5aHex).Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
	}
	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}

	// Submit a single block that builds on top of 3a.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4         (valid-fork)
	//                    \ -> 2a -> 3a -> 4a -> 5a  (active)
	//                                \ -> 4b        (valid-fork)
	block = getBlockFromString(t, block4bHex)
	err = r.Client.SubmitBlock(block, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotChainTips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4Hex).Hash().String(),
			BranchLen: 3,
			Status:    "valid-fork",
		},
		{
			Height:    5,
			Hash:      getBlockFromString(t, block5aHex).Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4bHex).Hash().String(),
			BranchLen: 1,
			Status:    "valid-fork",
		},
	}

	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}
}
