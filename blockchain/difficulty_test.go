// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/stretchr/testify/require"
)

// difficultyTestChainCtx is a ChainCtx implementation for the difficulty
// tests below.  It mirrors how BlockChain itself derives the retarget
// values in New (all timespans in seconds).  The stubChainCtx in
// regtest_test.go is not usable here: its Min/MaxRetargetTimespan come out
// in nanoseconds, a latent unit mismatch that never fires there because
// regtest sets PoWNoRetargeting and the retarget code returns before
// consulting the bounds.
type difficultyTestChainCtx struct {
	params *chaincfg.Params
}

// ChainParams returns the active chain parameters.
func (c difficultyTestChainCtx) ChainParams() *chaincfg.Params {
	return c.params
}

// BlocksPerRetarget returns the blocks per difficulty retarget.
func (c difficultyTestChainCtx) BlocksPerRetarget() int32 {
	return int32(c.params.TargetTimespan / c.params.TargetTimePerBlock)
}

// MinRetargetTimespan returns the lower bound of the retarget timespan.
func (c difficultyTestChainCtx) MinRetargetTimespan() int64 {
	targetTimespan := int64(c.params.TargetTimespan / time.Second)
	return targetTimespan / c.params.RetargetAdjustmentFactor
}

// MaxRetargetTimespan returns the upper bound of the retarget timespan.
func (c difficultyTestChainCtx) MaxRetargetTimespan() int64 {
	targetTimespan := int64(c.params.TargetTimespan / time.Second)
	return targetTimespan * c.params.RetargetAdjustmentFactor
}

// VerifyCheckpoint reports whether a checkpoint matches.
func (c difficultyTestChainCtx) VerifyCheckpoint(_ int32, _ *chainhash.Hash) bool {
	return true
}

// FindPreviousCheckpoint returns the last known checkpoint.
func (c difficultyTestChainCtx) FindPreviousCheckpoint() (HeaderCtx, error) {
	return nil, nil
}

// fakeHeaderCtx is a minimal HeaderCtx implementation for difficulty
// tests.  It carries no block hash, so building a synthetic chain of a few
// thousand headers costs no proof-of-work hashing (at ~340 ms per Argon2id
// hash, real blockNodes would make a 2,016-header chain take minutes).
type fakeHeaderCtx struct {
	height    int32
	bits      uint32
	timestamp int64
	parent    *fakeHeaderCtx
}

// Height returns the header's height.
func (f *fakeHeaderCtx) Height() int32 { return f.height }

// Bits returns the header's bits.
func (f *fakeHeaderCtx) Bits() uint32 { return f.bits }

// Timestamp returns the header's timestamp.
func (f *fakeHeaderCtx) Timestamp() int64 { return f.timestamp }

// Parent returns the header's parent.
func (f *fakeHeaderCtx) Parent() HeaderCtx {
	if f.parent == nil {
		return nil
	}
	return f.parent
}

// RelativeAncestorCtx returns the header's ancestor that is distance
// blocks before it in the chain.
func (f *fakeHeaderCtx) RelativeAncestorCtx(distance int32) HeaderCtx {
	node := f
	for i := int32(0); i < distance && node != nil; i++ {
		node = node.parent
	}
	if node == nil {
		return nil
	}
	return node
}

// buildFakeHeaderChain returns the tip of a synthetic chain of numHeaders
// headers (heights 0 through numHeaders-1) starting at startTime with the
// given per-block spacing in seconds, all carrying the given bits.
func buildFakeHeaderChain(numHeaders int32, startTime, spacing int64,
	bits uint32) *fakeHeaderCtx {

	var parent *fakeHeaderCtx
	for height := int32(0); height < numHeaders; height++ {
		parent = &fakeHeaderCtx{
			height:    height,
			bits:      bits,
			timestamp: startTime + int64(height)*spacing,
			parent:    parent,
		}
	}
	return parent
}

// TestMainNetRetargetWindow pins SLUG decision D67: on mainnet params the
// difficulty retarget window is 2,016 blocks (a TargetTimespan of 2,016
// blocks at 2 minutes each, ~2.8 days), not the 10,080 blocks that
// Bitcoin's 14-day timespan would produce at 2-minute blocks.
func TestMainNetRetargetWindow(t *testing.T) {
	params := chaincfg.MainNetParams
	ctx := difficultyTestChainCtx{params: &params}

	// The params themselves encode the D67 window.
	require.Equal(t, int64(2016*2*60),
		int64(params.TargetTimespan/time.Second))
	require.Equal(t, int32(2016), ctx.BlocksPerRetarget())

	genesisTime := params.GenesisBlock.Header.Timestamp.Unix()

	// Blocks arriving every 60 seconds come twice as fast as the
	// 2-minute target, so a retarget must make the next block harder
	// (a strictly smaller target than the previous block's).
	const fastSpacing = int64(60)

	// One block short of the boundary there is no retarget: the
	// required difficulty for block 2015 is simply the previous
	// block's bits, however fast the blocks arrived.
	tipBefore := buildFakeHeaderChain(2015, genesisTime, fastSpacing,
		params.PowLimitBits)
	require.Equal(t, int32(2014), tipBefore.Height())
	bits, err := calcNextRequiredDifficulty(tipBefore,
		time.Unix(tipBefore.timestamp+fastSpacing, 0), ctx)
	require.NoError(t, err)
	require.Equal(t, params.PowLimitBits, bits,
		"expected no retarget before block 2016")

	// At the boundary the retarget fires: the required difficulty for
	// block 2016 is recomputed from the timespan of the first 2,016
	// blocks and, because they arrived twice too fast, the target
	// drops (difficulty rises) instead of passing the old bits
	// through.  Under the old 10,080-block window this call would
	// have returned the previous bits unchanged.
	tipAt := buildFakeHeaderChain(2016, genesisTime, fastSpacing,
		params.PowLimitBits)
	require.Equal(t, int32(2015), tipAt.Height())
	bits, err = calcNextRequiredDifficulty(tipAt,
		time.Unix(tipAt.timestamp+fastSpacing, 0), ctx)
	require.NoError(t, err)
	require.NotEqual(t, params.PowLimitBits, bits,
		"expected a retarget at block 2016")
	require.True(t,
		CompactToBig(bits).Cmp(CompactToBig(params.PowLimitBits)) < 0,
		"expected retarget to lower the target (harder), got bits %08x",
		bits)
}
